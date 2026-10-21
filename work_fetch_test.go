package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// stubPluginCtx 测试桩：嵌入 PluginContext 接口，仅实现 Create 链触及的方法
// （日志、站点查询、前端发布、KV 读写）；未实现方法一旦被调用即 panic 暴露测试缺口
type stubPluginCtx struct {
	sdkdto.PluginContext
	sites []*sdkdto.SiteDTO
	kv    map[string]string
}

func (c stubPluginCtx) Infof(string, ...any) {}
func (c stubPluginCtx) Warnf(string, ...any) {}

func (c stubPluginCtx) ListSites() (*sdkdto.ListSitesResponse, error) {
	return &sdkdto.ListSitesResponse{Items: c.sites}, nil
}

func (c stubPluginCtx) PublishToFrontend(string, []byte) error {
	return errors.New("测试环境无前端通道")
}

// GetValue 读桩 KV；键不存在时返回 nil 存储值（等价键未写入，走空档案路径）
func (c stubPluginCtx) GetValue(key string) (*sdkdto.StorageValue, error) {
	if v, ok := c.kv[key]; ok {
		return &sdkdto.StorageValue{Value: v}, nil
	}
	return nil, nil
}

// SetValue 写桩 KV；未初始化 KV 的桩实例不持久化（测试侧只读）
func (c stubPluginCtx) SetValue(key string, value string) error {
	if c.kv == nil {
		return nil
	}
	c.kv[key] = value
	return nil
}

// qnaStubCtx 问答通路测试桩：发布分类问题时按目录名查预置答案，命中即同步回送
// （响应通道缓冲 1，发布返回后分类器必能收到）；未预置答案的目录发布失败，走
// 「发送失败默认规则」回退；published 计数已送达答案的发布次数，供问答触发断言。
type qnaStubCtx struct {
	stubPluginCtx
	answers    map[string]*ClassifyResponse // 目录名 → 预置响应
	published  int                          // 已送达答案的发布次数
	classifier *PathClassifier              // 回送目标分类器
}

// newQnaStubCtx 构造带站点注册表与预置答案的问答通路桩（KV 档案可读写）。
func newQnaStubCtx(sites []*sdkdto.SiteDTO, answers map[string]*ClassifyResponse) *qnaStubCtx {
	ctx := &qnaStubCtx{
		stubPluginCtx: stubPluginCtx{sites: sites, kv: make(map[string]string)},
		answers:       answers,
	}
	ctx.classifier = NewPathClassifier(ctx)
	return ctx
}

// PublishToFrontend 解析问题并按目录名回送预置答案；无预置答案返回错误触发默认回退。
func (c *qnaStubCtx) PublishToFrontend(_ string, data []byte) error {
	var q ClassifyQuestion
	if err := json.Unmarshal(data, &q); err != nil {
		return err
	}
	resp, ok := c.answers[q.DirName]
	if !ok {
		return errors.New("测试环境该目录无预置答案")
	}
	c.published++
	c.classifier.HandleResponse(resp)
	return nil
}

// collectResponses 读取流式创建结果全部应答
func collectResponses(t *testing.T, result *sdkdto.TaskCreateResult) []*sdkdto.TaskCreateResponse {
	t.Helper()
	if !result.IsStream() {
		t.Fatalf("应返回流式结果")
	}
	var out []*sdkdto.TaskCreateResponse
	for resp := range result.Stream() {
		out = append(out, resp)
	}
	return out
}

// findResponse 按站点键查找应答（同键多应答时返回首个）
func findResponse(resps []*sdkdto.TaskCreateResponse, siteKey string) *sdkdto.TaskCreateResponse {
	for _, r := range resps {
		if r.SiteKey == siteKey {
			return r
		}
	}
	return nil
}

// findSpecByRole 按 role 查找 store 规格（同 role 多轨时返回首个）
func findSpecByRole(specs []*sdkdto.StoreSpec, role string) *sdkdto.StoreSpec {
	for _, s := range specs {
		if s.Role == role {
			return s
		}
	}
	return nil
}

// —— 旧直建路径退役说明（决策6，阶段4）——
// 原驱动 streamScanTasks 的用例（空目录 reason、问答 site 分类子树归属、混合
// 归属拆分、群成员叶任务、local 发射形态）已随该路径删除；问答 site 分类在会话
// 模式下结构性不可达（目录含义只取快照规则回放，见 emitSelectedWorks）。混合
// 归属拆分与发射形态断言由 selection_test.go 的会话模式用例承接
//（TestCreateWorkbenchFilteredEmitsSelectedWorksOnly 族）。

// TestClassifyDirAbsorbsAnswerAsQnaRule 问答吸收与回放：作者答案生成一条 qna 规则
// （层深锚定的 LocationPattern + 目录名全匹配正则、ancestorDir→作者维度提取），
// 经 SaveProfile 落键 rule-profile（重启语义由 KV 落地承载）；再次分类时规则回放
// 含义（作者=目录名），不再发起问答；同层同目录锚点重复吸收原位替换不重复追加。
// 锚点取扫描根下首层目录（问答层深 1）：规则的 LocationPattern 层深须为换算后的
// DirNames 索引 0。（旧直建路径退役后经 ClassifyDir 直驱——问答吸收机制保留为
// 不可达历史代码，本测锚定其行为不回归。）
func TestClassifyDirAbsorbsAnswerAsQnaRule(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "作者目录"), 0o755)
	writeFile(t, filepath.Join(root, "作者目录", "a.png"), "a")
	dirName := "作者目录"

	ctx := newQnaStubCtx(nil, map[string]*ClassifyResponse{
		dirName: {Meanings: []PathMeaning{{Type: "localAuthor", Name: "首答作者"}}},
	})
	h := &LocalImportWorkFetcher{ctx: ctx, classifier: ctx.classifier}
	if _, err := h.classifier.ClassifyDir(1, dirName); err != nil {
		t.Fatalf("分类器问答失败: %v", err)
	}

	raw, ok := ctx.kv[RuleProfileKVKey]
	if !ok {
		t.Fatalf("问答响应后应经 SaveProfile 落键 %q", RuleProfileKVKey)
	}
	profile, errs := ParseProfile([]byte(raw))
	if profile == nil || len(errs) != 0 {
		t.Fatalf("落库档案应可整体解析，实得 profile=%v errs=%v", profile, errs)
	}
	if len(profile.Rules) != 1 {
		t.Fatalf("应恰有一条规则，实得 %d 条", len(profile.Rules))
	}
	rule := profile.Rules[0]
	if rule.ID != qnaRuleID(1, dirName) || rule.Source != RuleSourceQna || !rule.Enabled {
		t.Fatalf("规则应为启用的 qna 规则 %q，实得 id=%q source=%q enabled=%v", qnaRuleID(1, dirName), rule.ID, rule.Source, rule.Enabled)
	}
	loc, ok := rule.Pattern.(*LocationPattern)
	if !ok || loc.MinDepth != 0 || loc.MaxDepth != 0 {
		t.Fatalf("问答层深 1（扫描根下首层）应换算为 DirNames 索引 0 的 LocationPattern(0,0)，实得 %#v", rule.Pattern)
	}
	re, ok := loc.Ancestor.(*RegexPattern)
	if !ok || re.Target != TargetFileName || re.Expr != "^"+regexp.QuoteMeta(dirName)+"$" {
		t.Fatalf("Ancestor 应为目录名全匹配正则，实得 %#v", loc.Ancestor)
	}
	if len(rule.Extractions) != 1 || rule.Extractions[0].Dimension != DimAuthor ||
		rule.Extractions[0].From != FromAncestorDir {
		t.Fatalf("提取动作应为 ancestorDir→作者维度，实得 %+v", rule.Extractions)
	}
	if !strings.Contains(rule.Description, dirName) {
		t.Fatalf("规则描述应含问答上下文（目录名），实得 %q", rule.Description)
	}

	// 重启语义：新分类器经同一 KV 档案回放——不再发起问答，作者含义按目录名表达
	// （桩的回送目标切到新分类器，防回放失效时答案误送旧分类器拖满超时）
	before := ctx.published
	replayClassifier := NewPathClassifier(ctx)
	ctx.classifier = replayClassifier
	h2 := &LocalImportWorkFetcher{ctx: ctx, classifier: replayClassifier}
	meanings, err := h2.classifier.ClassifyDir(1, dirName)
	if err != nil {
		t.Fatalf("规则回放分类失败: %v", err)
	}
	if ctx.published != before {
		t.Fatalf("规则已覆盖该层含义，不应再次发起问答（发布 %d → %d）", before, ctx.published)
	}
	found := false
	for _, m := range meanings {
		if m.Type == string(PathTypeLocalAuthor) && m.Name == dirName {
			found = true
		}
	}
	if !found {
		t.Fatalf("规则回放应产出作者含义（名字=目录名），实得 %+v", meanings)
	}

	// 重复吸收（同层同目录锚点）：原位替换为新的维度集合，不重复追加
	ctx.classifier.absorbResponse(1, dirName, &ClassifyResponse{
		Meanings: []PathMeaning{{Type: "localTag", Name: "次答标签"}},
	})
	profile2, errs := ParseProfile([]byte(ctx.kv[RuleProfileKVKey]))
	if profile2 == nil || len(errs) != 0 {
		t.Fatalf("替换后档案应可整体解析，errs=%v", errs)
	}
	if len(profile2.Rules) != 1 || len(profile2.Rules[0].Extractions) != 1 ||
		profile2.Rules[0].Extractions[0].Dimension != DimTag {
		t.Fatalf("重复吸收应原位替换为标签维度且仍只有一条规则，实得 %+v", profile2.Rules)
	}
}

// TestQnaRuleDepthAlignsWithDirNamesIndex 层深编号对齐锁定：问答层深沿用调用方
// 编号（扫描根本身为 0、根下首层为 1），吸收生成的 qna 规则求值须落在 DirNames
// 索引语义的同层目录上——问答指认「扫描根下第 1 层目录为作者」，规则对已知
// DirNames 的 ScanFile 命中 DirNames[0]（作者声明=首层目录名），层深 2 的规则
// 命中 DirNames[1]，互不串层；层深 0（扫描根本身）不入 DirNames，不生成规则。
func TestQnaRuleDepthAlignsWithDirNamesIndex(t *testing.T) {
	ctx := newQnaStubCtx(nil, nil)
	ctx.classifier.absorbResponse(1, "作者甲", &ClassifyResponse{
		Meanings: []PathMeaning{{Type: "localAuthor", Name: "首层作者"}},
	})
	ctx.classifier.absorbResponse(2, "画集", &ClassifyResponse{
		Meanings: []PathMeaning{{Type: "localTag", Name: "第二层标签"}},
	})
	ctx.classifier.absorbResponse(0, "扫描根本身", &ClassifyResponse{
		Meanings: []PathMeaning{{Type: "localTag", Name: "根目录标签"}},
	})

	profile, errs := ParseProfile([]byte(ctx.kv[RuleProfileKVKey]))
	if profile == nil || len(errs) != 0 {
		t.Fatalf("问答规则档案应可整体解析，实得 profile=%v errs=%v", profile, errs)
	}
	if len(profile.Rules) != 2 {
		t.Fatalf("层深 1/2 各生成一条规则、层深 0（扫描根本身）不生成，应恰有 2 条，实得 %d", len(profile.Rules))
	}

	// 与 Create 期 buildScanFiles 同构：DirNames 由相对扫描根的 RelPath 分解，
	// 不含扫描根——首层「作者甲」=DirNames[0]、第二层「画集」=DirNames[1]
	files := buildScanFiles([]FileEntry{{FullPath: `E:\库\作者甲\画集\a.jpg`, RelPath: "作者甲/画集/a.jpg"}})
	decisions := RunPipeline(files, profile.Rules, nil)
	if len(decisions) != 1 {
		t.Fatalf("单文件应产出一行流水线结论，实得 %d 行", len(decisions))
	}
	var authors, tags []string
	for _, decl := range decisions[0].Declarations {
		switch decl.Dimension {
		case DimAuthor:
			authors = append(authors, decl.Value)
		case DimTag:
			tags = append(tags, decl.Value)
		}
	}
	if len(authors) != 1 || authors[0] != "作者甲" {
		t.Fatalf("层深 1 的作者规则应命中 DirNames[0]（作者=首层目录名「作者甲」），实得 %v", authors)
	}
	if len(tags) != 1 || tags[0] != "画集" {
		t.Fatalf("层深 2 的标签规则应命中 DirNames[1]（标签=第二层目录名「画集」），实得 %v", tags)
	}
}

// TestCreateWorkInfoLocalAttributionFieldsUnchanged local 归属的作品信息（回归锚）：
// 作品身份=哈希；站点域列表恒空（伪行停写）；站点作者/标签输入按名落本地域（ID=0 名称模式）；
// 作品集按名保留（local 站点的作品集 ID 约定形态）
func TestCreateWorkInfoLocalAttributionFieldsUnchanged(t *testing.T) {
	h := &LocalImportWorkFetcher{}
	pd := `{"schemaVersion":2,"fullPath":"C:/lib/photo.png","relPath":"photo.png","hash":"abc123","size":10,` +
		`"metadata":[{"type":"localAuthor","id":"5"},{"type":"siteAuthor","name":"作者甲"},` +
		`{"type":"localTag","id":"6"},{"type":"siteTag","name":"标签乙"},{"type":"workSet","name":"合集"}]}`
	resp, err := h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &pd})
	if err != nil {
		t.Fatalf("CreateWorkInfo 不应报错: %v", err)
	}
	if resp.Work.SiteWorkId == nil || *resp.Work.SiteWorkId != "abc123" {
		t.Fatalf("local 归属作品身份应为哈希 abc123，实得 %+v", resp.Work.SiteWorkId)
	}
	if len(resp.SiteAuthors) != 0 || len(resp.SiteTags) != 0 {
		t.Fatalf("站点域周边列表应恒空（伪行停写），实得 SiteAuthors=%d SiteTags=%d", len(resp.SiteAuthors), len(resp.SiteTags))
	}

	if len(resp.LocalAuthors) != 2 {
		t.Fatalf("应产出 2 个本地作者声明，实得 %d", len(resp.LocalAuthors))
	}
	var byName, byID bool
	for _, a := range resp.LocalAuthors {
		if a.Id == 5 {
			byID = true
		}
		if a.Id == 0 && a.AuthorName != nil && *a.AuthorName == "作者甲" {
			byName = true
		}
	}
	if !byID || !byName {
		t.Fatalf("本地作者应含 ID 引用(5)与按名声明(作者甲)，实得 %+v", resp.LocalAuthors)
	}

	if len(resp.LocalTags) != 2 {
		t.Fatalf("应产出 2 个本地标签声明，实得 %d", len(resp.LocalTags))
	}
	tagByName := false
	for _, tg := range resp.LocalTags {
		if tg.Id == 0 && tg.LocalTagName != nil && *tg.LocalTagName == "标签乙" {
			tagByName = true
		}
	}
	if !tagByName {
		t.Fatalf("本地标签应含按名声明(标签乙)，实得 %+v", resp.LocalTags)
	}

	if len(resp.WorkSets) != 1 || resp.WorkSets[0].SiteWorkSetId != "workSet:合集" {
		t.Fatalf("local 归属作品集应按名保留，实得 %+v", resp.WorkSets)
	}
}

// TestCreateWorkInfoRealAttribution 真实域归属的作品信息：作品身份=页级作品 ID（非哈希）；
// 周边按名仍落本地域；作品集含义跳过（真实域作品集 ID 无法从名字派生，无 ID 不造行）
func TestCreateWorkInfoRealAttribution(t *testing.T) {
	h := &LocalImportWorkFetcher{}
	pd := `{"schemaVersion":2,"fullPath":"C:/lib/12345678-abcdef_p0.jpg","relPath":"12345678-abcdef_p0.jpg",` +
		`"hash":"abc123","size":10,"siteKey":"pixiv","siteWorkId":"12345678-abcdef_p0",` +
		`"metadata":[{"type":"siteAuthor","name":"作者甲"},{"type":"siteTag","name":"标签乙"},{"type":"workSet","name":"合集"}]}`
	resp, err := h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &pd})
	if err != nil {
		t.Fatalf("CreateWorkInfo 不应报错: %v", err)
	}
	if resp.Work.SiteWorkId == nil || *resp.Work.SiteWorkId != "12345678-abcdef_p0" {
		t.Fatalf("真实域归属作品身份应为页级 ID，实得 %+v", resp.Work.SiteWorkId)
	}
	if len(resp.SiteAuthors) != 0 || len(resp.SiteTags) != 0 {
		t.Fatalf("站点域周边列表应恒空（伪行停写），实得 SiteAuthors=%d SiteTags=%d", len(resp.SiteAuthors), len(resp.SiteTags))
	}
	if len(resp.LocalAuthors) != 1 || resp.LocalAuthors[0].Id != 0 ||
		resp.LocalAuthors[0].AuthorName == nil || *resp.LocalAuthors[0].AuthorName != "作者甲" {
		t.Fatalf("周边输入应按名落本地域，实得 %+v", resp.LocalAuthors)
	}
	if len(resp.LocalTags) != 1 || resp.LocalTags[0].Id != 0 ||
		resp.LocalTags[0].LocalTagName == nil || *resp.LocalTags[0].LocalTagName != "标签乙" {
		t.Fatalf("周边输入应按名落本地域，实得 %+v", resp.LocalTags)
	}
	if len(resp.WorkSets) != 0 {
		t.Fatalf("真实域归属的作品集含义应跳过，实得 %+v", resp.WorkSets)
	}
}

// TestCreateWorkInfoLegacyPseudoFormDataConvertsByName 旧 PluginData（引入归属字段前写入，
// schemaVersion 0/1）内嵌的 siteAuthor/siteTag 名字声明按新规则转报本地域按名——
// 站点域伪行不再产生，作品身份维持既有哈希（旧任务按创建时归属执行）
func TestCreateWorkInfoLegacyPseudoFormDataConvertsByName(t *testing.T) {
	h := &LocalImportWorkFetcher{}
	for _, version := range []int{0, 1} {
		pd := `{"schemaVersion":` + itoa(version) + `,"fullPath":"C:/lib/photo.png","relPath":"photo.png","hash":"legacyhash","size":10,` +
			`"metadata":[{"type":"siteAuthor","name":"旧作者"},{"type":"siteTag","name":"旧标签"}]}`
		resp, err := h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &pd})
		if err != nil {
			t.Fatalf("schemaVersion=%d 不应报错: %v", version, err)
		}
		if resp.Work.SiteWorkId == nil || *resp.Work.SiteWorkId != "legacyhash" {
			t.Fatalf("schemaVersion=%d 旧任务作品身份应维持哈希，实得 %+v", version, resp.Work.SiteWorkId)
		}
		if len(resp.SiteAuthors) != 0 || len(resp.SiteTags) != 0 {
			t.Fatalf("schemaVersion=%d 旧数据不得复活站点域伪行，实得 %+v/%+v", version, resp.SiteAuthors, resp.SiteTags)
		}
		if len(resp.LocalAuthors) != 1 || resp.LocalAuthors[0].Id != 0 ||
			resp.LocalAuthors[0].AuthorName == nil || *resp.LocalAuthors[0].AuthorName != "旧作者" {
			t.Fatalf("schemaVersion=%d 旧站点作者应转报本地域按名，实得 %+v", version, resp.LocalAuthors)
		}
		if len(resp.LocalTags) != 1 || resp.LocalTags[0].Id != 0 ||
			resp.LocalTags[0].LocalTagName == nil || *resp.LocalTags[0].LocalTagName != "旧标签" {
			t.Fatalf("schemaVersion=%d 旧站点标签应转报本地域按名，实得 %+v", version, resp.LocalTags)
		}
	}
}

// TestBuildChildResponsesSiteKeyAndSingleParent 混归属子行构造与单父行形态：
// 子行归属键与 FilePluginData 归属字段同构——真实域子行携 siteKey 与页级作品
// ID 身份、local 回退子行键留空（继承父应答站点键）与哈希身份；父行为纯容器
// （DirRelPath 空、无含义链），PluginTaskId=local-batch-<毫秒>（无组间后缀）、
// 站点键=创建域 identity.Local.Key；单子行亦为父行（一次创建恒为一个父任务）。
func TestBuildChildResponsesSiteKeyAndSingleParent(t *testing.T) {
	dir := t.TempDir()
	f1 := writeFile(t, filepath.Join(dir, "photo.png"), "a")
	f2 := writeFile(t, filepath.Join(dir, "11111111_p0.jpg"), "b")
	f3 := writeFile(t, filepath.Join(dir, "11111111_p1.jpg"), "c")
	items := []batchFileItem{
		{af: attributedFile{entry: FileEntry{FullPath: f1, RelPath: "sub/photo.png", Hash: "h0"}}},
		{af: attributedFile{entry: FileEntry{FullPath: f2, RelPath: "sub/11111111_p0.jpg", Hash: "h1"}, attr: workAttribution{SiteKey: identity.Pixiv.Key, PageWorkID: "11111111_p0"}}},
		{af: attributedFile{entry: FileEntry{FullPath: f3, RelPath: "sub/11111111_p1.jpg", Hash: "h2"}, attr: workAttribution{SiteKey: identity.Pixiv.Key, PageWorkID: "11111111_p1"}}},
	}
	children := buildChildResponses(items, nil)
	if len(children) != 3 {
		t.Fatalf("应产出 3 子行，实得 %d", len(children))
	}
	// 子行键分布：local 回退首行键空+哈希身份，真实域子行键=pixiv+页级 ID 身份
	if children[0].SiteKey != "" || children[0].SiteWorkId != "h0" {
		t.Fatalf("local 回退子行应键空+哈希身份，实际 %q/%q", children[0].SiteKey, children[0].SiteWorkId)
	}
	for _, child := range children[1:] {
		if child.SiteKey != identity.Pixiv.Key {
			t.Fatalf("真实域子行应携归属站点键，实际 %q", child.SiteKey)
		}
		var fp FilePluginData
		if err := json.Unmarshal([]byte(child.PluginData), &fp); err != nil {
			t.Fatalf("解析子任务 PluginData 失败: %v", err)
		}
		if !fp.hasRealAttribution() || fp.SiteKey != child.SiteKey || fp.SiteWorkId != child.SiteWorkId {
			t.Fatalf("子任务 PluginData 归属字段应与子行键同构，实得 child=%q fp=%+v", child.SiteWorkId, fp)
		}
	}
	resp := buildBatchParentResponse("local-batch-1700000000000", "批次", "local://root", identity.Local.Key, children)
	if resp.PluginTaskId != "local-batch-1700000000000" || resp.SiteWorkId != resp.PluginTaskId {
		t.Fatalf("父任务身份应为 local-batch-<毫秒>（无组间后缀），实得 %q/%q", resp.PluginTaskId, resp.SiteWorkId)
	}
	if resp.SiteKey != identity.Local.Key || resp.TaskName != "批次" {
		t.Fatalf("父行应为创建域站点键与裸任务名，实得 %q/%q", resp.SiteKey, resp.TaskName)
	}
	var dp DirPluginData
	if err := json.Unmarshal([]byte(resp.PluginData), &dp); err != nil {
		t.Fatalf("解析父行 PluginData 失败: %v", err)
	}
	if dp.DirRelPath != "" || dp.Metadata != nil {
		t.Fatalf("批次父行 PluginData 应为纯容器（DirRelPath 空、无含义链），实际 %+v", dp)
	}
	// 单子行亦为父行（一次创建=一个父任务；宿主 parent+children 不折叠契约）
	single := buildBatchParentResponse("local-batch-1700000000001", "单件", "local://root", identity.Local.Key, children[:1])
	if len(single.Children) != 1 {
		t.Fatalf("单子行批次亦应为父行+单子行（不折叠为叶任务），实得 %+v", single)
	}
}

func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写测试文件失败: %v", err)
	}
	return path
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

// stubThumbGenerator 桩缩略图生成器：固定字节，规避单测对 FFmpeg 的环境依赖
type stubThumbGenerator struct{}

func (stubThumbGenerator) generate(string) ([]byte, string, error) {
	return []byte("stub-thumbnail-bytes"), "jpg", nil
}

// pluginDataJSON 构造文件级 PluginData JSON（fullPath 统一斜杠形态，避免 Windows 反斜杠转义）
func pluginDataJSON(fullPath, relPath, hash string, size int64) string {
	b, _ := json.Marshal(FilePluginData{
		SchemaVersion: currentPluginDataVersion,
		FullPath:      filepath.ToSlash(fullPath),
		RelPath:       relPath,
		Hash:          hash,
		Size:          size,
	})
	return string(b)
}

// TestStartDeclaresExpectedSha256 Start 主资源轨期望哈希声明（决策1 选项A：声明扫描期哈希）：
// ①声明值 == PluginData.Hash；④Hash 为空的异常数据不声明（nil，不构造空串声明）
func TestStartDeclaresExpectedSha256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.png")
	writeFile(t, path, "png-bytes")

	hash, err := ComputeFileHash(path)
	if err != nil {
		t.Fatalf("计算测试文件哈希失败: %v", err)
	}

	h := &LocalImportWorkFetcher{}
	task := &sdkdto.TaskDTO{Id: 1, PluginData: new(pluginDataJSON(path, "photo.png", hash, 9))}
	specs, _, err := h.Start(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("Start 不应报错: %v", err)
	}
	main := findSpecByRole(specs, sdkdto.StoreRoleImage)
	if main == nil {
		t.Fatalf("应产出 image 主资源轨，实得 %d 条规格", len(specs))
	}
	if main.ExpectedSha256 == nil || *main.ExpectedSha256 != hash {
		t.Fatalf("①Start 主资源轨 ExpectedSha256 应声明 PluginData.Hash(%q)，实得 %v", hash, main.ExpectedSha256)
	}
	if err := h.Stop(&sdkdto.TaskResParam{Task: task}); err != nil {
		t.Fatalf("关闭源文件句柄失败: %v", err)
	}

	// ④防御分支：Hash 为空时不声明（nil）
	emptyTask := &sdkdto.TaskDTO{Id: 2, PluginData: new(pluginDataJSON(path, "photo.png", "", 9))}
	specs, _, err = h.Start(context.Background(), emptyTask, nil)
	if err != nil {
		t.Fatalf("Start(hash 空) 不应报错: %v", err)
	}
	main = findSpecByRole(specs, sdkdto.StoreRoleImage)
	if main == nil {
		t.Fatalf("hash 空也应产出主资源轨，实得 %d 条规格", len(specs))
	}
	if main.ExpectedSha256 != nil {
		t.Fatalf("④Hash 为空时不应声明期望哈希(nil)，实得 %q", *main.ExpectedSha256)
	}
	if err := h.Stop(&sdkdto.TaskResParam{Task: emptyTask}); err != nil {
		t.Fatalf("关闭源文件句柄失败: %v", err)
	}
}

// TestResumeDeclaresExpectedSha256 ②Resume 主资源轨同声明：声明值 == PluginData.Hash
// （跨会话实测哈希一致由主程序暂存前缀回读保证，插件侧无需额外处理）
func TestResumeDeclaresExpectedSha256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.mp4")
	writeFile(t, path, "mp4-bytes")

	hash, err := ComputeFileHash(path)
	if err != nil {
		t.Fatalf("计算测试文件哈希失败: %v", err)
	}

	h := &LocalImportWorkFetcher{}
	task := &sdkdto.TaskDTO{Id: 7, PluginData: new(pluginDataJSON(path, "movie.mp4", hash, 9))}
	specs, _, err := h.Resume(context.Background(), &sdkdto.TaskResumeParam{
		Task: task,
		StreamOffsets: []*sdkdto.StoreResumeOffset{
			{Role: sdkdto.StoreRoleVideoMain, StoreSeq: 0, Offset: 2},
		},
	})
	if err != nil {
		t.Fatalf("Resume 不应报错: %v", err)
	}
	main := findSpecByRole(specs, sdkdto.StoreRoleVideoMain)
	if main == nil {
		t.Fatalf("应产出 videoMain 续传轨，实得 %d 条规格", len(specs))
	}
	if main.ExpectedSha256 == nil || *main.ExpectedSha256 != hash {
		t.Fatalf("②Resume 主资源轨 ExpectedSha256 应声明 PluginData.Hash(%q)，实得 %v", hash, main.ExpectedSha256)
	}
	if err := h.Stop(&sdkdto.TaskResParam{Task: task}); err != nil {
		t.Fatalf("关闭源文件句柄失败: %v", err)
	}
}

// TestStartThumbnailDeclaresSha256 ③缩略图轨（derived 生成产物轨）期望哈希 == 生成字节的 sha256：
// 桩生成器替换视频生成器规避 FFmpeg 依赖，期望值由测试独立计算（不经被测辅助函数）
func TestStartThumbnailDeclaresSha256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.mp4")
	writeFile(t, path, "mp4-bytes")

	old := extensionGenerators[".mp4"]
	extensionGenerators[".mp4"] = stubThumbGenerator{}
	defer func() { extensionGenerators[".mp4"] = old }()

	h := &LocalImportWorkFetcher{}
	task := &sdkdto.TaskDTO{Id: 3, PluginData: new(pluginDataJSON(path, "clip.mp4", "abc123", 9))}
	specs, _, err := h.Start(context.Background(), task, []string{sdkdto.StoreRoleThumbnail})
	if err != nil {
		t.Fatalf("Start 不应报错: %v", err)
	}
	if len(specs) != 1 || specs[0].Role != sdkdto.StoreRoleThumbnail {
		t.Fatalf("仅选缩略图角色应产出单条 thumbnail 轨，实得 %d 条", len(specs))
	}
	sum := sha256.Sum256([]byte("stub-thumbnail-bytes"))
	want := hex.EncodeToString(sum[:])
	if specs[0].ExpectedSha256 == nil || *specs[0].ExpectedSha256 != want {
		t.Fatalf("③缩略图轨 ExpectedSha256 应为生成字节 sha256(%q)，实得 %v", want, specs[0].ExpectedSha256)
	}
}

// TestEmitSelectedWorksChildOrderFollowsGroupSeq 子行序=选择域集内序：集成员
// 作品的文件名字典序（集名_06 < 集名_1 < 集名_10 < 集名_2）与群组序号数值序
// （1 < 2 < 6 < 10）不同，批次子行按集内序排列（任务树子项序与识别序一致）。
// 夹具混排两类成员形态：已识别成员（文件行不落群组键，群归属只在群卡）与
// 未识别成员（规则群形态，文件行与群卡双携带）——群组序号取值与集序记录同
// 口径（群卡反查为主、文件行兜底），只读文件行时已识别成员序取零、子行退回
// 传入字典序（06,1,10,2 而非 1,2,06,10）。集序记录随批次落库（存储键=集业务
// 键原文），作答身份键与子行身份键同源同值、序号=集内序-1。
func TestEmitSelectedWorksChildOrderFollowsGroupSeq(t *testing.T) {
	root := t.TempDir()
	// 成员按传入序（选择集落库序=文件名字典序）列出：06 → 1 → 10 → 2；
	// 已识别成员 06/10（文件行无群组键）、未识别成员 1/2（文件行携带群组键）
	members := []struct {
		name       string
		seq        int
		seqText    string
		identified bool
	}{
		{"集名_06.jpg", 6, "06", true},
		{"集名_1.jpg", 1, "1", false},
		{"集名_10.jpg", 10, "10", true},
		{"集名_2.jpg", 2, "2", false},
	}
	sess := &WorkbenchSession{Snapshot: &RecognitionSnapshot{}}
	works := make([]*SelectableWork, 0, len(members))
	groupMembers := make([]SnapGroupMember, 0, len(members))
	for _, m := range members {
		writeTestFile(t, root+"/乙/"+m.name, m.name)
		rel := "乙/" + m.name
		sf := SnapFile{
			RelPath: rel, FileName: m.name, DirRelKey: "乙",
			Final: []SnapDecl{{Dimension: string(DimCollection), Value: "集名"}},
		}
		if m.identified {
			// 已识别形态：文件行不落群组键（群归属只在群卡）
			sf.State = snapStateIdentified
		} else {
			// 未识别规则群形态：文件行与群卡双携带群组键与序号
			sf.State = snapStatePlain
			sf.GroupKey = "集名"
			sf.GroupSeq = m.seq
		}
		sess.Snapshot.Files = append(sess.Snapshot.Files, sf)
		groupMembers = append(groupMembers, SnapGroupMember{RelPath: rel, Seq: m.seq, SeqText: m.seqText})
		works = append(works, &SelectableWork{
			WorkID: "f:" + rel, Kind: WorkKindSetMember, DirRelKey: "乙",
			Files: []string{rel}, SetKey: workSetKey("乙", "集名"),
		})
	}
	sess.Snapshot.Groups = append(sess.Snapshot.Groups, SnapGroup{DirRelKey: "乙", Key: "集名", Members: groupMembers})
	m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
	ctx := stubPluginCtx{kv: make(map[string]string)}
	h := &LocalImportWorkFetcher{sessions: m, ctx: ctx}

	resps := collectEmitted(t, h, root, sess, works, "集序批次")
	if len(resps) != 1 {
		t.Fatalf("纯 local 批次应为单应答单父，实得 %d 个", len(resps))
	}
	if want := "集名_1.jpg,集名_2.jpg,集名_06.jpg,集名_10.jpg"; strings.Join(responseChildNames(resps[0]), ",") != want {
		t.Fatalf("子行应按集内序（序号数值序 1,2,06,10）排列，实际 %v", responseChildNames(resps[0]))
	}

	// 集序记录随批次落库：作答全序=集内序、身份键与子行身份键（内容哈希）逐位一致
	entries, err := h.QueryWorkSetOrder(0, "workSet:集名")
	if err != nil {
		t.Fatalf("查询集序作答失败: %v", err)
	}
	if len(entries) != len(members) {
		t.Fatalf("作答应含 %d 条，实得 %d", len(members), len(entries))
	}
	children := resps[0].Children
	for i, e := range entries {
		if e.SortOrder != int64(i) {
			t.Fatalf("作答序号应为集内序-1，第 %d 位实得 %d", i+1, e.SortOrder)
		}
		if e.SiteWorkId != children[i].SiteWorkId {
			t.Fatalf("作答第 %d 位身份键应与子行一致（%s），实得 %s", i+1, children[i].SiteWorkId, e.SiteWorkId)
		}
	}
}
