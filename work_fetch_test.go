package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
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

// GetPreference 偏好读取桩：恒无记录（爬坡控制器偏好钉死层在直驱测试走自动轨；
// 偏好钉死断言见 create_pipeline_test.go 的偏好桩环境）。
func (c stubPluginCtx) GetPreference(string) (*sdkdto.PreferenceValue, bool, error) {
	return nil, false, nil
}

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
	// v4：目录行条件规则（行 1=目录名整名字面模板），提取 ancestorDir→作者。
	rowRule, ok := rule.Pattern.(*RowSubjectPattern)
	if !ok || rule.Conditions == nil {
		t.Fatalf("问答规则应为 v4 目录行条件规则，实得 %#v", rule.Pattern)
	}
	if len(rule.Conditions) != 1 || rule.Conditions[0].Condition == nil ||
		rule.Conditions[0].Condition.Target != CondxTargetDir ||
		rule.Conditions[0].Condition.Format.Tpl != dirName {
		t.Fatalf("行 1 应为目录名 %q 整名字面模板，实得 %+v", dirName, rule.Conditions)
	}
	if rowRule.Ordinal != 1 {
		t.Fatalf("目录行应锚定行 1，实得 %#v", rowRule)
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

// —— 聚合作品全链（选择域聚合分支→单子行→执行面多轨）—— //

// aggTestTypeSpecs 资源类型注册表测试投影（镜像主仓 video/article/image/document
// 内置规约的角色基数；聚合作品类型派生与宿主完整性校验同一容纳口径）。
func aggTestTypeSpecs() []*sdkdto.ResourceTypeSpec {
	return []*sdkdto.ResourceTypeSpec{
		{
			ResourceType: sdkdto.ResourceTypeVideo,
			Roles: []*sdkdto.StoreRoleSpec{
				{StoreType: sdkdto.StoreRoleVideoMain, Min: 1, Max: 1},
				{StoreType: sdkdto.StoreRoleVideoTrack, Min: 0, Max: 1},
				{StoreType: sdkdto.StoreRoleAudioTrack, Min: 0, Max: 1},
				{StoreType: contract.StoreTypeSubtitle, Min: 0, Max: 0},
				{StoreType: sdkdto.StoreRoleThumbnail, Min: 0, Max: 1},
			},
		},
		{
			ResourceType: sdkdto.ResourceTypeArticle,
			Roles: []*sdkdto.StoreRoleSpec{
				{StoreType: sdkdto.StoreRoleDocument, Min: 1, Max: 1},
				{StoreType: sdkdto.StoreRoleImage, Min: 0, Max: 0},
				{StoreType: sdkdto.StoreRoleThumbnail, Min: 0, Max: 1},
			},
		},
		{
			ResourceType: sdkdto.ResourceTypeImage,
			Roles: []*sdkdto.StoreRoleSpec{
				{StoreType: sdkdto.StoreRoleImage, Min: 1, Max: 1},
				{StoreType: sdkdto.StoreRoleThumbnail, Min: 0, Max: 1},
			},
		},
		{
			ResourceType: sdkdto.ResourceTypeDocument,
			Roles: []*sdkdto.StoreRoleSpec{
				{StoreType: sdkdto.StoreRoleDocument, Min: 1, Max: 1},
				{StoreType: sdkdto.StoreRoleThumbnail, Min: 0, Max: 1},
			},
		},
	}
}

// aggTypesStubCtx 带资源类型注册表投影的桩上下文（聚合作品类型在册校验的拉取
// 面）：stubPluginCtx 未实现 ListResourceTypes（调用即 panic 暴露缺口），聚合
// 链测试经本桩供在册类型。
type aggTypesStubCtx struct {
	stubPluginCtx
	types []*sdkdto.ResourceTypeSpec
}

// ListResourceTypes 注册表投影桩：回预置类型集。
func (c aggTypesStubCtx) ListResourceTypes() (*sdkdto.ListResourceTypesResponse, error) {
	return &sdkdto.ListResourceTypesResponse{Items: c.types}, nil
}

// aggRegistryCtx 注册表桩的可控变体：fail 真=拉取失败（信任声明值路径的面），
// types 可注入任意在册集；Warnf 记录调用（整件放弃样本文案的断言面）。
type aggRegistryCtx struct {
	stubPluginCtx
	types []*sdkdto.ResourceTypeSpec
	fail  bool
	mu    sync.Mutex
	warns []string
}

// ListResourceTypes 按 fail/types 应答。
func (c *aggRegistryCtx) ListResourceTypes() (*sdkdto.ListResourceTypesResponse, error) {
	if c.fail {
		return nil, errors.New("测试注入的注册表拉取失败")
	}
	return &sdkdto.ListResourceTypesResponse{Items: c.types}, nil
}

// Warnf 记录告警文案（聚合放弃样本断言用）。
func (c *aggRegistryCtx) Warnf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.warns = append(c.warns, fmt.Sprintf(format, args...))
}

// aggEmitFixture 聚合链直驱夹具：按快照行（含聚合归属与声明类型）落盘文件并
// 推导选择域，返回直驱 emitSelectedWorks 的全部句柄（ctx 供注册表投影桩）。
func aggEmitFixture(t *testing.T, files []SnapFile) (string, *LocalImportWorkFetcher, *SessionManager, *WorkbenchSession, []*SelectableWork) {
	t.Helper()
	return aggEmitFixtureWithCtx(t, files, aggTypesStubCtx{stubPluginCtx: stubPluginCtx{kv: make(map[string]string)}, types: aggTestTypeSpecs()})
}

// aggEmitFixtureWithCtx 指定插件上下文的聚合链直驱夹具（注册表桩变体注入面）。
func aggEmitFixtureWithCtx(t *testing.T, files []SnapFile, ctx sdkdto.PluginContext) (string, *LocalImportWorkFetcher, *SessionManager, *WorkbenchSession, []*SelectableWork) {
	t.Helper()
	root := t.TempDir()
	for i := range files {
		writeTestFile(t, root+"/"+files[i].RelPath, "内容-"+files[i].RelPath)
	}
	sess := &WorkbenchSession{Snapshot: &RecognitionSnapshot{Files: files}}
	works := DeriveWorkDomain(sess.Snapshot, nil).Works
	m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
	h := &LocalImportWorkFetcher{sessions: m, ctx: ctx, hostContractFn: func() uint32 { return createChildrenBatchMinHostContract }}
	return root, h, m, sess, works
}

// TestCreateAggregatedWorksSingleChildAndTracks 聚合作品=单子行：一件聚合作品恰
// 一个子行（不再逐文件扁平），ResourceType=快照声明的作品类型（聚合段落落轨的
// 规则声明值——video/article，不做推断）；PluginData v4 携带成员轨道清单（各
// 文件路径/哈希/角色，首位=主导文件=顶层字段结论），文件数与角色构成（确认呈
// 现「N 个文件」「视频 + 字幕×N」的数据面）由清单直接可得；作品身份 local 回退
// =主导文件内容哈希。
func TestCreateAggregatedWorksSingleChildAndTracks(t *testing.T) {
	videoKey := "f:剧集/ep01.mp4"
	articleKey := "f:图文/手记.txt"
	root, h, m, sess, works := aggEmitFixture(t, []SnapFile{
		{RelPath: "剧集/ep01.en.srt", FileName: "ep01.en.srt", DirRelKey: "剧集", State: snapStatePlain,
			AggWorkKey: videoKey, AggTrackRole: contract.StoreTypeSubtitle, AggWorkType: sdkdto.ResourceTypeVideo},
		{RelPath: "剧集/ep01.mp4", FileName: "ep01.mp4", DirRelKey: "剧集", State: snapStateIdentified,
			AggWorkKey: videoKey, AggTrackRole: sdkdto.StoreRoleVideoMain, AggWorkType: sdkdto.ResourceTypeVideo},
		{RelPath: "剧集/ep01.zh.srt", FileName: "ep01.zh.srt", DirRelKey: "剧集", State: snapStatePlain,
			AggWorkKey: videoKey, AggTrackRole: contract.StoreTypeSubtitle, AggWorkType: sdkdto.ResourceTypeVideo},
		{RelPath: "图文/手记.txt", FileName: "手记.txt", DirRelKey: "图文", State: snapStateIdentified,
			AggWorkKey: articleKey, AggTrackRole: sdkdto.StoreRoleDocument, AggWorkType: sdkdto.ResourceTypeArticle},
		{RelPath: "图文/配图.jpg", FileName: "配图.jpg", DirRelKey: "图文", State: snapStatePlain,
			AggWorkKey: articleKey, AggTrackRole: sdkdto.StoreRoleImage, AggWorkType: sdkdto.ResourceTypeArticle},
	})
	hashOf := func(rel string) string {
		hash, err := ComputeFileHash(root + "/" + rel)
		if err != nil {
			t.Fatalf("计算测试文件哈希失败: %v", err)
		}
		return hash
	}

	if len(works) != 2 {
		t.Fatalf("快照 5 文件应并成 2 件聚合作品，实际 %d: %+v", len(works), works)
	}
	resps := emittedTasks(collectEmitted(t, h, root, sess, works, "聚合批次"))
	if len(resps) != 1 {
		t.Fatalf("纯 local 聚合批次应为单应答单父，实得 %d 个", len(resps))
	}
	children := resps[0].Children
	if len(children) != 2 {
		t.Fatalf("2 件聚合作品应各产 1 个子行（单子行形态），实得 %d: %v", len(children), responseChildNames(resps[0]))
	}

	// 子行一：视频聚合作品（主导 mp4+双语字幕）→ ResourceType=video、v4 轨道清单
	video := children[0]
	if video.TaskName != "ep01.mp4" || video.ResourceType != sdkdto.ResourceTypeVideo {
		t.Fatalf("视频聚合子行应为 mp4 主导+ResourceType=video，实际 %q/%q", video.TaskName, video.ResourceType)
	}
	var vfp FilePluginData
	if err := json.Unmarshal([]byte(video.PluginData), &vfp); err != nil {
		t.Fatalf("解析聚合子行 PluginData 失败: %v", err)
	}
	if vfp.SchemaVersion != 4 || len(vfp.Tracks) != 3 {
		t.Fatalf("聚合子行 PluginData 应为 v4 三轨清单，实际 schema=%d tracks=%+v", vfp.SchemaVersion, vfp.Tracks)
	}
	if vfp.RelPath != "剧集/ep01.mp4" || vfp.Hash != hashOf("剧集/ep01.mp4") {
		t.Fatalf("v4 顶层字段应为主导文件结论，实际 relPath=%q hash 有无=%v", vfp.RelPath, vfp.Hash == hashOf("剧集/ep01.mp4"))
	}
	wantTracks := []struct {
		rel  string
		role string
	}{
		{"剧集/ep01.mp4", sdkdto.StoreRoleVideoMain},
		{"剧集/ep01.en.srt", contract.StoreTypeSubtitle},
		{"剧集/ep01.zh.srt", contract.StoreTypeSubtitle},
	}
	for i, want := range wantTracks {
		track := vfp.Tracks[i]
		if track.RelPath != want.rel || track.Role != want.role || track.Hash != hashOf(want.rel) {
			t.Fatalf("轨道 %d 应为 %s/%s（哈希=文件内容哈希），实际 %s/%s", i, want.rel, want.role, track.RelPath, track.Role)
		}
	}
	if video.SiteKey != "" || video.SiteWorkId != hashOf("剧集/ep01.mp4") {
		t.Fatalf("local 回退聚合子行应键空+主导文件哈希身份，实际 %q/%q", video.SiteKey, video.SiteWorkId)
	}

	// 子行二：图文聚合作品（主导 txt+内嵌图）→ ResourceType=article
	article := children[1]
	if article.TaskName != "手记.txt" || article.ResourceType != sdkdto.ResourceTypeArticle {
		t.Fatalf("图文聚合子行应为 txt 主导+ResourceType=article，实际 %q/%q", article.TaskName, article.ResourceType)
	}
	var afp FilePluginData
	if err := json.Unmarshal([]byte(article.PluginData), &afp); err != nil {
		t.Fatalf("解析聚合子行 PluginData 失败: %v", err)
	}
	if len(afp.Tracks) != 2 || afp.Tracks[0].Role != sdkdto.StoreRoleDocument || afp.Tracks[1].Role != sdkdto.StoreRoleImage {
		t.Fatalf("图文聚合轨道清单应为 document+image，实际 %+v", afp.Tracks)
	}

	// 批次摘要：2 件聚合作品=2 子行
	if summary := m.takeCreateBatchSummary(root); summary.total != 2 {
		t.Fatalf("批次摘要 total 应为 2（聚合作品单子行），实际 %+v", summary)
	}
}

// TestCreateAggregatedDanglingTypeDropped 声明作品类型的悬空处置：注册表已拉到
// 而声明类型不在册（类型已下架/自定义类型注销）=整件放弃+样本上报（宁可不导
// 不错型）；声明值缺席（过期求值器产物）同整件放弃并提示重扫；在册声明照常
// 产出。注册表拉取失败（索引缺席）=信任声明值直发（宿主写入侧严格校验兜底）。
func TestCreateAggregatedDanglingTypeDropped(t *testing.T) {
	t.Run("悬空与声明缺席整件放弃，在册照常产出", func(t *testing.T) {
		ctx := &aggRegistryCtx{stubPluginCtx: stubPluginCtx{kv: make(map[string]string)}, types: aggTestTypeSpecs()}
		root, h, m, sess, works := aggEmitFixtureWithCtx(t, []SnapFile{
			// 悬空：声明类型 manga 不在注册表投影（域序在首——聚合放弃样本的采样位）
			{RelPath: "漫画卷/卷一.db", FileName: "卷一.db", DirRelKey: "漫画卷", State: snapStateIdentified,
				AggWorkKey: "f:漫画卷/卷一.db", AggTrackRole: sdkdto.StoreRoleDocument, AggWorkType: "manga"},
			{RelPath: "漫画卷/卷一.png", FileName: "卷一.png", DirRelKey: "漫画卷", State: snapStatePlain,
				AggWorkKey: "f:漫画卷/卷一.db", AggTrackRole: sdkdto.StoreRoleImage, AggWorkType: "manga"},
			// 在册：video 声明照常单子行产出
			{RelPath: "剧集/ep01.mp4", FileName: "ep01.mp4", DirRelKey: "剧集", State: snapStateIdentified,
				AggWorkKey: "f:剧集/ep01.mp4", AggTrackRole: sdkdto.StoreRoleVideoMain, AggWorkType: sdkdto.ResourceTypeVideo},
			{RelPath: "剧集/ep01.zh.srt", FileName: "ep01.zh.srt", DirRelKey: "剧集", State: snapStatePlain,
				AggWorkKey: "f:剧集/ep01.mp4", AggTrackRole: contract.StoreTypeSubtitle, AggWorkType: sdkdto.ResourceTypeVideo},
			// 声明缺席：过期求值器产物形态
			{RelPath: "无型/手记.txt", FileName: "手记.txt", DirRelKey: "无型", State: snapStateIdentified,
				AggWorkKey: "f:无型/手记.txt", AggTrackRole: sdkdto.StoreRoleDocument},
		}, ctx)
		items, reason, cancelled := collectEmittedWithReason(t, h, root, sess, works, "悬空批次")
		if reason != "" || cancelled {
			t.Fatalf("类型悬空应整件放弃而非中止创建: reason=%s cancelled=%v", reason, cancelled)
		}
		var children []*sdkdto.TaskCreateChildResponse
		for _, item := range items {
			if task := item.Task(); task != nil {
				children = append(children, task.Children...)
			} else if batch := item.ChildrenBatch(); batch != nil {
				children = append(children, batch.Items...)
			}
		}
		if len(children) != 1 || children[0].TaskName != "ep01.mp4" || children[0].ResourceType != sdkdto.ResourceTypeVideo {
			t.Fatalf("仅在册声明的作品应产出（ResourceType=声明值），实际 %d 子行: %+v", len(children), children)
		}
		summary := m.takeCreateBatchSummary(root)
		if summary.total != 1 || summary.dropped != 2 || summary.dropSample != "漫画卷/卷一.db" {
			t.Fatalf("悬空+缺席应各整件放弃（计 2、样本=域序首位），实际 %+v", summary)
		}
		ctx.mu.Lock()
		warns := append([]string(nil), ctx.warns...)
		ctx.mu.Unlock()
		if len(warns) == 0 || !strings.Contains(warns[0], "2 件") || !strings.Contains(warns[0], "作品类型 manga 已不可用") {
			t.Fatalf("放弃样本应说明悬空类型名，实际 %v", warns)
		}
	})
	t.Run("注册表拉取失败信任声明值直发", func(t *testing.T) {
		ctx := &aggRegistryCtx{stubPluginCtx: stubPluginCtx{kv: make(map[string]string)}, fail: true}
		root, h, m, sess, works := aggEmitFixtureWithCtx(t, []SnapFile{
			{RelPath: "剧集/ep01.mp4", FileName: "ep01.mp4", DirRelKey: "剧集", State: snapStateIdentified,
				AggWorkKey: "f:剧集/ep01.mp4", AggTrackRole: sdkdto.StoreRoleVideoMain, AggWorkType: sdkdto.ResourceTypeVideo},
			{RelPath: "剧集/ep01.zh.srt", FileName: "ep01.zh.srt", DirRelKey: "剧集", State: snapStatePlain,
				AggWorkKey: "f:剧集/ep01.mp4", AggTrackRole: contract.StoreTypeSubtitle, AggWorkType: sdkdto.ResourceTypeVideo},
		}, ctx)
		items, reason, cancelled := collectEmittedWithReason(t, h, root, sess, works, "信任批次")
		if reason != "" || cancelled {
			t.Fatalf("注册表拉取失败不应中止创建: reason=%s cancelled=%v", reason, cancelled)
		}
		resps := emittedTasks(items)
		if len(resps) != 1 || len(resps[0].Children) != 1 ||
			resps[0].Children[0].ResourceType != sdkdto.ResourceTypeVideo {
			t.Fatalf("拉取失败应信任声明值直发（ResourceType=video），实际 %+v", resps)
		}
		if summary := m.takeCreateBatchSummary(root); summary.total != 1 || summary.dropped != 0 {
			t.Fatalf("信任路径应零放弃，实际 %+v", summary)
		}
	})
}

// TestWorkFetchAggregatedStartMultiTrackSpecs 聚合作品 Start 多轨：按轨道清单发
// specs（videoMain+subtitle×N）另加缩略图轨（主导文件生成，derived），每轨独立
// ExpectedSha256（=各自成员扫描期哈希）；storeRoles 过滤按角色生效。
func TestWorkFetchAggregatedStartMultiTrackSpecs(t *testing.T) {
	dir := t.TempDir()
	mp4 := writeFile(t, filepath.Join(dir, "clip.mp4"), "mp4-payload")
	zh := writeFile(t, filepath.Join(dir, "clip.zh.srt"), "zh-sub")
	en := writeFile(t, filepath.Join(dir, "clip.en.srt"), "en-sub")
	hashOf := func(path string) string {
		hash, err := ComputeFileHash(path)
		if err != nil {
			t.Fatalf("计算测试文件哈希失败: %v", err)
		}
		return hash
	}
	fp := FilePluginData{
		SchemaVersion: currentPluginDataVersion,
		FullPath:      filepath.ToSlash(mp4), RelPath: "clip.mp4", Hash: hashOf(mp4), Size: 10,
		Tracks: []TrackEntry{
			{FullPath: filepath.ToSlash(mp4), RelPath: "clip.mp4", Hash: hashOf(mp4), Role: sdkdto.StoreRoleVideoMain},
			{FullPath: filepath.ToSlash(zh), RelPath: "clip.zh.srt", Hash: hashOf(zh), Role: contract.StoreTypeSubtitle},
			{FullPath: filepath.ToSlash(en), RelPath: "clip.en.srt", Hash: hashOf(en), Role: contract.StoreTypeSubtitle},
		},
	}
	fpJSON, _ := json.Marshal(fp)
	task := &sdkdto.TaskDTO{Id: 21, PluginData: new(string(fpJSON))}

	old := extensionGenerators[".mp4"]
	extensionGenerators[".mp4"] = stubThumbGenerator{}
	defer func() { extensionGenerators[".mp4"] = old }()

	h := &LocalImportWorkFetcher{}
	specs, _, err := h.Start(context.Background(), task, nil)
	if err != nil {
		t.Fatalf("聚合 Start 不应报错: %v", err)
	}
	// 轨序断言：主导轨+伴随轨×N（清单序）+缩略图轨
	roles := make([]string, len(specs))
	for i, s := range specs {
		roles[i] = s.Role
	}
	if want := []string{sdkdto.StoreRoleVideoMain, contract.StoreTypeSubtitle, contract.StoreTypeSubtitle, sdkdto.StoreRoleThumbnail}; strings.Join(roles, ",") != strings.Join(want, ",") {
		t.Fatalf("聚合轨序应为 %v，实际 %v", want, roles)
	}
	// 每轨独立期望哈希：逐轨声明各自成员的扫描期哈希
	for i, track := range fp.Tracks {
		spec := specs[i]
		if spec.ExpectedSha256 == nil || *spec.ExpectedSha256 != track.Hash {
			t.Fatalf("轨 %d（%s）ExpectedSha256 应独立声明该成员哈希，实际 %v", i, track.Role, spec.ExpectedSha256)
		}
		if spec.Generation != sdkdto.GenerationDownloaded || spec.Continuable == nil || !*spec.Continuable {
			t.Fatalf("成员轨 %d 应为 downloaded 可续传，实际 %+v", i, spec)
		}
	}
	sum := sha256.Sum256([]byte("stub-thumbnail-bytes"))
	if specs[3].ExpectedSha256 == nil || *specs[3].ExpectedSha256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("缩略图轨应为生成字节 sha256，实际 %v", specs[3].ExpectedSha256)
	}
	if err := h.Stop(&sdkdto.TaskResParam{Task: task}); err != nil {
		t.Fatalf("关闭成员文件句柄失败: %v", err)
	}

	// 角色过滤：仅选 subtitle → 只发两条字幕轨
	subSpecs, _, err := h.Start(context.Background(), task, []string{contract.StoreTypeSubtitle})
	if err != nil {
		t.Fatalf("仅选字幕角色不应报错: %v", err)
	}
	if len(subSpecs) != 2 || subSpecs[0].Role != contract.StoreTypeSubtitle || subSpecs[1].Role != contract.StoreTypeSubtitle {
		t.Fatalf("仅选 subtitle 应产两条字幕轨，实际 %d 条", len(subSpecs))
	}
	if err := h.Stop(&sdkdto.TaskResParam{Task: task}); err != nil {
		t.Fatalf("关闭成员文件句柄失败: %v", err)
	}
}

// TestWorkFetchAggregatedResumePerRoleOffset 聚合作品 Resume 按角色偏移续传：
// StreamOffsets 按（角色,store_seq）对位轨道清单（store_seq=清单内同角色出现
// 序），返回序=清单序（与 Start 发射序一致——同角色轨编号对位稳定）；每轨从
// 磁盘已落盘偏移处续读（读取内容=源文件偏移后余段）并声明 ResumeWriteOffset
// 与该轨期望哈希。无待续轨返回空过。
func TestWorkFetchAggregatedResumePerRoleOffset(t *testing.T) {
	dir := t.TempDir()
	mp4 := writeFile(t, filepath.Join(dir, "clip.mp4"), "v-payload")
	zh := writeFile(t, filepath.Join(dir, "clip.zh.srt"), "zh-sub")
	en := writeFile(t, filepath.Join(dir, "clip.en.srt"), "en-sub")
	hashOf := func(path string) string {
		hash, err := ComputeFileHash(path)
		if err != nil {
			t.Fatalf("计算测试文件哈希失败: %v", err)
		}
		return hash
	}
	fp := FilePluginData{
		SchemaVersion: currentPluginDataVersion,
		FullPath:      filepath.ToSlash(mp4), RelPath: "clip.mp4", Hash: hashOf(mp4), Size: 8,
		Tracks: []TrackEntry{
			{FullPath: filepath.ToSlash(mp4), RelPath: "clip.mp4", Hash: hashOf(mp4), Role: sdkdto.StoreRoleVideoMain},
			{FullPath: filepath.ToSlash(zh), RelPath: "clip.zh.srt", Hash: hashOf(zh), Role: contract.StoreTypeSubtitle},
			{FullPath: filepath.ToSlash(en), RelPath: "clip.en.srt", Hash: hashOf(en), Role: contract.StoreTypeSubtitle},
		},
	}
	fpJSON, _ := json.Marshal(fp)
	task := &sdkdto.TaskDTO{Id: 22, PluginData: new(string(fpJSON))}

	h := &LocalImportWorkFetcher{}
	// 偏移乱序给（subtitle/1 在前）：返回序仍按清单序对位
	specs, _, err := h.Resume(context.Background(), &sdkdto.TaskResumeParam{
		Task: task,
		StreamOffsets: []*sdkdto.StoreResumeOffset{
			{Role: contract.StoreTypeSubtitle, StoreSeq: 1, Offset: 3},
			{Role: sdkdto.StoreRoleVideoMain, StoreSeq: 0, Offset: 2},
			{Role: contract.StoreTypeSubtitle, StoreSeq: 0, Offset: 2},
		},
	})
	if err != nil {
		t.Fatalf("聚合 Resume 不应报错: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("应按待续偏移产 3 条续传轨，实得 %d", len(specs))
	}
	type wantTrack struct {
		role   string
		offset int64
		hash   string
		rest   string
	}
	wants := []wantTrack{
		{sdkdto.StoreRoleVideoMain, 2, hashOf(mp4), "payload"},
		{contract.StoreTypeSubtitle, 2, hashOf(zh), "-sub"},
		{contract.StoreTypeSubtitle, 3, hashOf(en), "sub"},
	}
	for i, want := range wants {
		spec := specs[i]
		if spec.Role != want.role {
			t.Fatalf("续传轨 %d 角色应为 %s，实际 %s", i, want.role, spec.Role)
		}
		if spec.ResumeWriteOffset == nil || *spec.ResumeWriteOffset != want.offset {
			t.Fatalf("续传轨 %d ResumeWriteOffset 应为 %d，实际 %v", i, want.offset, spec.ResumeWriteOffset)
		}
		if spec.ExpectedSha256 == nil || *spec.ExpectedSha256 != want.hash {
			t.Fatalf("续传轨 %d 期望哈希应为该成员哈希，实际 %v", i, spec.ExpectedSha256)
		}
		rest, err := io.ReadAll(spec.ReadCloser)
		if err != nil {
			t.Fatalf("读取续传轨 %d 失败: %v", i, err)
		}
		if string(rest) != want.rest {
			t.Fatalf("续传轨 %d 应自偏移 %d 处续读（余段 %q），实际 %q", i, want.offset, want.rest, string(rest))
		}
	}
	if err := h.Stop(&sdkdto.TaskResParam{Task: task}); err != nil {
		t.Fatalf("关闭成员文件句柄失败: %v", err)
	}

	// 无待续轨：清单内角色均无未完成偏移 → 空过（主程序经 Start 重产）
	none, _, err := h.Resume(context.Background(), &sdkdto.TaskResumeParam{Task: task})
	if err != nil {
		t.Fatalf("无待续轨 Resume 不应报错: %v", err)
	}
	if none != nil {
		t.Fatalf("无待续轨应空过，实际 %d 条", len(none))
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
			WorkID: "f:" + rel, DirRelKey: "乙",
			Files: []string{rel}, SetKey: workSetKey("乙", "集名"),
		})
	}
	sess.Snapshot.Groups = append(sess.Snapshot.Groups, SnapGroup{DirRelKey: "乙", Key: "集名", Members: groupMembers})
	m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
	ctx := stubPluginCtx{kv: make(map[string]string)}
	h := &LocalImportWorkFetcher{sessions: m, ctx: ctx}

	resps := emittedTasks(collectEmitted(t, h, root, sess, works, "集序批次"))
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

// —— 大批次子行分帧（契约 21 B1+，大载荷分页方案·设计 B 测试锚定 5/9）—— //

// TestSplitChildBatchesDualBoundAndEmitSequence 拆块序列：构造 3 万子行 → 条数
// 上限主导切批（2 万+1 万）→ 发射序列=父块（首批内联）+批次块（末批 last=true），
// 块序列化估和 ≤16MB、序保持；单条超字节目标的巨子行独立成批（首条必装保证推进）。
func TestSplitChildBatchesDualBoundAndEmitSequence(t *testing.T) {
	children := make([]*sdkdto.TaskCreateChildResponse, 30000)
	for i := range children {
		children[i] = &sdkdto.TaskCreateChildResponse{
			TaskName:   fmt.Sprintf("f%06d.jpg", i),
			SiteWorkId: fmt.Sprintf("h%06d", i),
			Url:        fmt.Sprintf("local://lib/f%06d.jpg", i),
			PluginData: `{"schemaVersion":3}`,
		}
	}
	batches := splitChildBatches(children)
	if len(batches) != 2 || len(batches[0]) != createChildrenBatchCountLimit || len(batches[1]) != 10000 {
		t.Fatalf("3 万子行应按条数上限切 2 批（2万+1万），实得 %d 批", len(batches))
	}
	for bi, batch := range batches {
		if len(batch) > createChildrenBatchCountLimit {
			t.Fatalf("第 %d 批超条数上限: %d", bi, len(batch))
		}
		var size int
		for _, c := range batch {
			size += childWireSizeUpper(c)
		}
		if size > createChildrenBatchByteTarget {
			t.Fatalf("第 %d 批序列化估和超字节目标: ~%dMB", bi, size>>20)
		}
	}

	// 发射序列：父块首批内联 + 批次块（末批 last）+ 跨块序保持
	ch := make(chan *sdkdto.CreateStreamItem, 8)
	go func() {
		emitChildBatches(ch, "local-batch-1", "大批次", "local://lib", identity.Local.Key, batches, nil)
		close(ch)
	}()
	var items []*sdkdto.CreateStreamItem
	for it := range ch {
		items = append(items, it)
	}
	if len(items) != 2 {
		t.Fatalf("发射序列应为父块+1 批次块，实得 %d 条目", len(items))
	}
	parent := items[0].Task()
	if parent == nil || parent.PluginTaskId != "local-batch-1" || len(parent.Children) != createChildrenBatchCountLimit {
		t.Fatalf("父块应带首批 2 万子行内联，实际 parent在场=%v children=%d", parent != nil, len(parent.Children))
	}
	if parent.Children[0].TaskName != "f000000.jpg" || parent.Children[19999].TaskName != "f019999.jpg" {
		t.Fatalf("首批子行应保持输入序（首尾锚点），实际 %q…%q", parent.Children[0].TaskName, parent.Children[19999].TaskName)
	}
	batch := items[1].ChildrenBatch()
	if batch == nil || batch.PluginTaskId != "local-batch-1" || !batch.Last || len(batch.Items) != 10000 {
		t.Fatalf("批次块应指称父 ID、携带余批 1 万行且 last=true，实际在场=%v %+v", batch != nil, batch)
	}
	if batch.Items[0].TaskName != "f020000.jpg" || batch.Items[9999].TaskName != "f029999.jpg" {
		t.Fatalf("批次块子行应保持输入序（首尾锚点），实际 %q…%q", batch.Items[0].TaskName, batch.Items[9999].TaskName)
	}

	// 单条巨子行：超字节目标也独立成批（首条必装保证推进，纯内联形态）
	mega := []*sdkdto.TaskCreateChildResponse{{TaskName: "mega.jpg", PluginData: strings.Repeat("x", createChildrenBatchByteTarget+1)}}
	if got := splitChildBatches(mega); len(got) != 1 || len(got[0]) != 1 {
		t.Fatalf("单条巨子行应独立成批（首条必装），实得 %d 批", len(got))
	}
}

// TestEmitSelectedWorksChildrenBatchSplitting 字节界拆块与握手降级（对齐大载荷
// 分页方案测试锚定 5/9）：目录含义链携带 2MB 大值（每子行 PluginData ≈2.1MB）
// ——契约 ≥21：首批内联父块+批次块（≤16MB、指称父 ID、末批 last、序保持、
// 摘要全量统计）；协商 20：同载荷（~21MB ≤60MB）降级单块全量内联（无批次块）；
// 协商 20 且 ~65MB ≥60MB：中止原因明确报错「宿主契约过旧且载荷超限」、零产出。
func TestEmitSelectedWorksChildrenBatchSplitting(t *testing.T) {
	root := t.TempDir()
	files := make([]string, 10)
	for i := range files {
		files[i] = fmt.Sprintf("bulk/b%02d.bin", i)
		writeTestFile(t, root+"/"+files[i], files[i])
	}
	newSess := func(dirKey string) *WorkbenchSession {
		return &WorkbenchSession{Snapshot: &RecognitionSnapshot{
			Dirs: []SnapDir{{RelKey: dirKey, Meanings: []PathMeaning{{Type: string(PathTypeLocalTag), Name: strings.Repeat("a", 2<<20)}}}},
		}}
	}

	t.Run("字节界拆块", func(t *testing.T) {
		m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
		h := &LocalImportWorkFetcher{sessions: m, hostContractFn: func() uint32 { return createChildrenBatchMinHostContract }}
		works := make([]*SelectableWork, 0, len(files))
		for _, rel := range files {
			works = append(works, &SelectableWork{WorkID: "f:" + rel, DirRelKey: "bulk", Files: []string{rel}})
		}
		items := collectEmitted(t, h, root, newSess("bulk"), works, "大批次")
		tasks, batches := emittedTasks(items), emittedBatches(items)
		if len(tasks) != 1 || len(batches) != 1 {
			t.Fatalf("~21MB 子行应拆为父块+1 批次块，实得 task=%d batch=%d", len(tasks), len(batches))
		}
		parent := tasks[0]
		if len(parent.Children) == 0 || len(parent.Children) >= len(files) {
			t.Fatalf("父块应带首批子行内联（0<首批<全部），实际 %d 条", len(parent.Children))
		}
		var firstBytes int
		for _, c := range parent.Children {
			firstBytes += childWireSizeUpper(c)
		}
		if firstBytes > createChildrenBatchByteTarget {
			t.Fatalf("首批内联序列化估和应 ≤16MB，实际 ~%dMB", firstBytes>>20)
		}
		batch := batches[0]
		if batch.PluginTaskId != parent.PluginTaskId || !batch.Last {
			t.Fatalf("批次块应指称父声明 ID 且 last=true，实际 %q vs %q last=%v", batch.PluginTaskId, parent.PluginTaskId, batch.Last)
		}
		var names []string
		names = append(names, responseChildNames(parent)...)
		for _, c := range batch.Items {
			names = append(names, c.TaskName)
		}
		var want []string
		for _, f := range files {
			want = append(want, filepath.Base(f))
		}
		if strings.Join(names, ",") != strings.Join(want, ",") {
			t.Fatalf("拆块后子行总数应全等且序保持，实际 %v", names)
		}
		if summary := m.takeCreateBatchSummary(root); summary.total != len(files) {
			t.Fatalf("批次摘要应全量统计（%d 子行），实际 %+v", len(files), summary)
		}
	})

	t.Run("降级单块全量内联", func(t *testing.T) {
		m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
		h := &LocalImportWorkFetcher{sessions: m, hostContractFn: func() uint32 { return createChildrenBatchMinHostContract - 1 }}
		works := make([]*SelectableWork, 0, len(files))
		for _, rel := range files {
			works = append(works, &SelectableWork{WorkID: "f:" + rel, DirRelKey: "bulk", Files: []string{rel}})
		}
		items := collectEmitted(t, h, root, newSess("bulk"), works, "大批次")
		if got := len(emittedBatches(items)); got != 0 {
			t.Fatalf("降级形态不应有批次块，实得 %d", got)
		}
		tasks := emittedTasks(items)
		if len(tasks) != 1 || len(tasks[0].Children) != len(files) {
			t.Fatalf("降级应为单块全量内联（%d 子行），实得 %+v", len(files), tasks)
		}
	})

	t.Run("降级超限明确报错", func(t *testing.T) {
		// 31 文件 × ~2.1MB ≈ 65MB > 60MB 降级内联上限
		overFiles := make([]string, 31)
		for i := range overFiles {
			overFiles[i] = fmt.Sprintf("over/o%02d.bin", i)
			writeTestFile(t, root+"/"+overFiles[i], overFiles[i])
		}
		m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
		h := &LocalImportWorkFetcher{sessions: m, hostContractFn: func() uint32 { return createChildrenBatchMinHostContract - 1 }}
		works := make([]*SelectableWork, 0, len(overFiles))
		for _, rel := range overFiles {
			works = append(works, &SelectableWork{WorkID: "f:" + rel, DirRelKey: "over", Files: []string{rel}})
		}
		items, reason, cancelled := collectEmittedWithReason(t, h, root, newSess("over"), works, "超限批次")
		if reason == "" || !strings.Contains(reason, "宿主契约过旧") || !strings.Contains(reason, "载荷超限") {
			t.Fatalf("降级超限应明确报错（宿主契约过旧且载荷超限），实际 %q", reason)
		}
		if cancelled {
			t.Fatalf("降级超限是失败收尾而非取消收尾")
		}
		if len(items) != 0 {
			t.Fatalf("降级超限应零产出，实得 %d 条目", len(items))
		}
	})
}
