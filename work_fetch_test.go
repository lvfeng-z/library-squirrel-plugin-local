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

// TestCreateEmptyDirSetsReason 空目录创建任务时零任务返回并经 SetReason 声明用户可读原因；
// 路径不可访问仍走错误返回（经 SDK 的 error 块通道透传，不经 reason）
func TestCreateEmptyDirSetsReason(t *testing.T) {
	dir := t.TempDir()

	h := &LocalImportWorkFetcher{}
	result, err := h.Create(dir)
	if err != nil {
		t.Fatalf("空目录 Create 不应报错: %v", err)
	}
	if result.IsStream() {
		t.Fatalf("空目录应返回批量结果而非流式")
	}
	if tasks := result.Array(); len(tasks) != 0 {
		t.Fatalf("空目录不应产出任务，实际产出 %d 个", len(tasks))
	}
	if result.Reason() == "" {
		t.Fatalf("空目录应经 SetReason 声明原因，Reason() 为空")
	}

	missing := filepath.Join(dir, "不存在的子目录")
	if _, err := h.Create(missing); err == nil {
		t.Fatalf("不可访问路径应保持错误返回，实际无错误")
	}
}

// TestCreateSiteClassifiedSubtreeGoesRealDomain site 分类消费：目录经问答指认 site 含义
// （答案 ID=面板选择项 value=站点 DB 行 id）时，子树内文件解析出的作品归该站点域；
// 解析不出页级 ID 的文件回退 local；层级作用域对照——更深层 site 含义不外溢到浅层文件
func TestCreateSiteClassifiedSubtreeGoesRealDomain(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "12345678_p0.jpg"), "a")
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	writeFile(t, filepath.Join(root, "sub", "99999999_p1.png"), "b")
	os.MkdirAll(filepath.Join(root, "sub2"), 0o755)
	writeFile(t, filepath.Join(root, "sub2", "readme.txt"), "c")

	sites := []*sdkdto.SiteDTO{
		{Id: 7, SiteKey: identity.Pixiv.Key},
		{Id: 9, SiteKey: identity.Bilibili.Key},
	}
	ctx := newQnaStubCtx(sites, map[string]*ClassifyResponse{
		filepath.Base(root): {Meanings: []PathMeaning{{Type: "site", ID: "7", Name: "pixiv"}}},
	})
	h := &LocalImportWorkFetcher{ctx: ctx, classifier: ctx.classifier}

	resps := collectResponses(t, mustCreate(t, h, root))
	if len(resps) != 3 {
		t.Fatalf("三个目录组应各产出一应答（根/sub/sub2 各单文件成叶），实得 %d 个", len(resps))
	}

	// 根组：原图形态文件 → pixiv 页级叶任务
	// （应答按父目录分组 map 随机序产出，须按作品 ID 无序定位，不能取首个 pixiv 键应答）
	var pixivRoot *sdkdto.TaskCreateResponse
	for _, r := range resps {
		if r.SiteWorkId == "12345678_p0" {
			pixivRoot = r
			break
		}
	}
	if pixivRoot == nil || pixivRoot.PluginTaskId != "12345678_p0" {
		t.Fatalf("根组应产出 pixiv 页级叶任务(12345678_p0)，实得 %+v", pixivRoot)
	}
	// 子树生效：根级 site 规则传导到子目录组的文件归属
	foundSub := false
	for _, r := range resps {
		if r.SiteKey == identity.Pixiv.Key && r.SiteWorkId == "99999999_p1" {
			foundSub = true
		}
	}
	if !foundSub {
		t.Fatalf("子目录原图形态文件应归 pixiv 域(99999999_p1)，实得应答 %+v", resps)
	}
	// 站点分类在场但文件名解析不出页级 ID → local 回退
	localResp := findResponse(resps, identity.Local.Key)
	if localResp == nil || localResp.SiteWorkId == "" {
		t.Fatalf("解析不出真实 ID 的文件应回退 local 哈希身份，实得 %+v", localResp)
	}
	expectHash, err := ComputeFileHash(filepath.Join(root, "sub2", "readme.txt"))
	if err != nil || localResp.SiteWorkId != expectHash {
		t.Fatalf("local 回退身份应为内容哈希 %q，实得 %q (err=%v)", expectHash, localResp.SiteWorkId, err)
	}
	// 站点答案无可映射含义维度：不生成规则，不落规则档案键（含义仅本次运行消费）
	if _, ok := ctx.kv[RuleProfileKVKey]; ok {
		t.Fatalf("站点答案无可映射含义维度，不应写入规则档案键 %q", RuleProfileKVKey)
	}

	// 层级作用域对照：更深层（level 1）的 site 含义只作用于该层子树——
	// 根层文件不受其影响（不外溢到浅层），按文件名形态独立锚定 pixiv
	root2 := t.TempDir()
	writeFile(t, filepath.Join(root2, "12345678_p0.jpg"), "d")
	os.MkdirAll(filepath.Join(root2, "deep"), 0o755)
	writeFile(t, filepath.Join(root2, "deep", "99999999_p1.png"), "e")

	ctx2 := newQnaStubCtx(sites, map[string]*ClassifyResponse{
		"deep": {Meanings: []PathMeaning{{Type: "site", ID: "9", Name: "bilibili"}}},
	})
	h2 := &LocalImportWorkFetcher{ctx: ctx2, classifier: ctx2.classifier}
	resps2 := collectResponses(t, mustCreate(t, h2, root2))
	var rootFileSite, deepFileSite string
	for _, r := range resps2 {
		if r.SiteWorkId == "12345678_p0" {
			rootFileSite = r.SiteKey
		}
		if r.SiteWorkId == "99999999_p1" {
			deepFileSite = r.SiteKey
		}
	}
	if rootFileSite != identity.Pixiv.Key {
		t.Fatalf("根层文件不应受 level 1 站点规则影响（应为 pixiv 形态锚定），实得 %q", rootFileSite)
	}
	if deepFileSite != identity.Bilibili.Key {
		t.Fatalf("深层文件应受 level 1 站点规则归 bilibili，实得 %q", deepFileSite)
	}
}

// TestClassifyDirAbsorbsAnswerAsQnaRule 问答吸收与回放：作者答案生成一条 qna 规则
// （层深锚定的 LocationPattern + 目录名全匹配正则、ancestorDir→作者维度提取），
// 经 SaveProfile 落键 rule-profile（重启语义由 KV 落地承载）；再次扫描时规则回放
// 含义（作者=目录名），不再发起问答；同层同目录锚点重复吸收原位替换不重复追加
func TestClassifyDirAbsorbsAnswerAsQnaRule(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.png"), "a")
	rootName := filepath.Base(root)

	ctx := newQnaStubCtx(nil, map[string]*ClassifyResponse{
		rootName: {Meanings: []PathMeaning{{Type: "localAuthor", Name: "首答作者"}}},
	})
	h := &LocalImportWorkFetcher{ctx: ctx, classifier: ctx.classifier}
	collectResponses(t, mustCreate(t, h, root))

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
	if rule.ID != qnaRuleID(0, rootName) || rule.Source != RuleSourceQna || !rule.Enabled {
		t.Fatalf("规则应为启用的 qna 规则 %q，实得 id=%q source=%q enabled=%v", qnaRuleID(0, rootName), rule.ID, rule.Source, rule.Enabled)
	}
	loc, ok := rule.Pattern.(*LocationPattern)
	if !ok || loc.MinDepth != 0 || loc.MaxDepth != 0 {
		t.Fatalf("模式应为层深锚定的 LocationPattern(0,0)，实得 %#v", rule.Pattern)
	}
	re, ok := loc.Ancestor.(*RegexPattern)
	if !ok || re.Target != TargetFileName || re.Expr != "^"+regexp.QuoteMeta(rootName)+"$" {
		t.Fatalf("Ancestor 应为目录名全匹配正则，实得 %#v", loc.Ancestor)
	}
	if len(rule.Extractions) != 1 || rule.Extractions[0].Dimension != DimAuthor ||
		rule.Extractions[0].From != FromAncestorDir {
		t.Fatalf("提取动作应为 ancestorDir→作者维度，实得 %+v", rule.Extractions)
	}
	if !strings.Contains(rule.Description, rootName) {
		t.Fatalf("规则描述应含问答上下文（目录名），实得 %q", rule.Description)
	}

	// 重启语义：新分类器经同一 KV 档案回放——不再发起问答，作者含义按目录名表达
	// （桩的回送目标切到新分类器，防回放失效时答案误送旧分类器拖满超时）
	before := ctx.published
	replayClassifier := NewPathClassifier(ctx)
	ctx.classifier = replayClassifier
	h2 := &LocalImportWorkFetcher{ctx: ctx, classifier: replayClassifier}
	resps2 := collectResponses(t, mustCreate(t, h2, root))
	if ctx.published != before {
		t.Fatalf("规则已覆盖该层含义，不应再次发起问答（发布 %d → %d）", before, ctx.published)
	}
	if len(resps2) != 1 {
		t.Fatalf("单文件应产出一叶应答，实得 %d 个", len(resps2))
	}
	var fp FilePluginData
	if err := json.Unmarshal([]byte(resps2[0].PluginData), &fp); err != nil {
		t.Fatalf("解析叶应答 PluginData 失败: %v", err)
	}
	found := false
	for _, m := range fp.Metadata {
		if m.Type == string(PathTypeLocalAuthor) && m.Name == rootName {
			found = true
		}
	}
	if !found {
		t.Fatalf("规则回放应产出作者含义（名字=目录名），实得 %+v", fp.Metadata)
	}

	// 重复吸收（同层同目录锚点）：原位替换为新的维度集合，不重复追加
	ctx.classifier.absorbResponse(0, rootName, &ClassifyResponse{
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

// TestCreateMixedDirSplitsByAttribution 同目录混合归属（原图形态+普通文件名）：
// 拆为两个应答各自携带站点键（子任务站点继承父应答，无法在同应答内混合归属）
func TestCreateMixedDirSplitsByAttribution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "11111111_p0.jpg"), "a")
	writeFile(t, filepath.Join(root, "plain.png"), "b")

	h := &LocalImportWorkFetcher{ctx: stubPluginCtx{}, classifier: NewPathClassifier(stubPluginCtx{})}
	resps := collectResponses(t, mustCreate(t, h, root))
	if len(resps) != 2 {
		t.Fatalf("同目录混合归属应拆为 2 个应答，实得 %d 个", len(resps))
	}
	var hasPixiv, hasLocal bool
	for _, r := range resps {
		switch r.SiteKey {
		case identity.Pixiv.Key:
			hasPixiv = r.SiteWorkId == "11111111_p0"
		case identity.Local.Key:
			hasLocal = r.SiteWorkId != ""
		}
	}
	if !hasPixiv || !hasLocal {
		t.Fatalf("应同时存在 pixiv(11111111_p0) 与 local 应答，实得 %+v", resps)
	}
}

// TestCreateSiblingGroupMembersBecomeSingleWorks Create 级群成员集成断言：
// 档案内兄弟群组规则（key 正则剥序号、minGroupSize=2）命中同目录序列文件后，
// 两名群成员各自成单文件作品叶任务（身份=内容哈希，落 local 域），落单文件
// 走目录组路径（本例组内仅剩单文件亦成叶）——全叶输出，无目录父任务应答
func TestCreateSiblingGroupMembersBecomeSingleWorks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "photo_1.jpg"), "a")
	writeFile(t, filepath.Join(root, "photo_2.jpg"), "b")
	writeFile(t, filepath.Join(root, "plain.txt"), "c")

	profile := `{"schemaVersion":1,"rules":[{` +
		`"id":"sibling-photo","description":"同前缀序列文件成群","source":"manual","enabled":true,` +
		`"pattern":{"kind":"sibling","keyExpr":"^(.*)_","seqExpr":"_(\\d+)","minGroupSize":2,"outcome":"collection"},` +
		`"extractions":[]}]}`
	ctx := stubPluginCtx{kv: map[string]string{RuleProfileKVKey: profile}}
	h := &LocalImportWorkFetcher{ctx: ctx, classifier: NewPathClassifier(ctx)}

	resps := collectResponses(t, mustCreate(t, h, root))
	if len(resps) != 3 {
		t.Fatalf("两名群成员+一名落单者应各成叶任务（3 应答），实得 %d 个", len(resps))
	}
	seen := map[string]bool{}
	for _, r := range resps {
		if len(r.Children) != 0 || strings.HasPrefix(r.PluginTaskId, "local-dir-") {
			t.Fatalf("不应出现目录父任务应答，实得 PluginTaskId=%q children=%d", r.PluginTaskId, len(r.Children))
		}
		if r.SiteKey != identity.Local.Key {
			t.Fatalf("无站点分类时成员应落 local 域，实得 SiteKey=%q", r.SiteKey)
		}
		expectHash, err := ComputeFileHash(filepath.Join(root, r.TaskName))
		if err != nil || r.SiteWorkId != expectHash {
			t.Fatalf("叶任务身份应为内容哈希 %q，实得 %q (err=%v)", expectHash, r.SiteWorkId, err)
		}
		seen[r.TaskName] = true
	}
	for _, name := range []string{"photo_1.jpg", "photo_2.jpg", "plain.txt"} {
		if !seen[name] {
			t.Fatalf("缺少 %s 的叶任务应答，实见 %v", name, seen)
		}
	}
}

// TestCreateLocalEmissionFormUnchanged local 归属路径字段零变化（回归锚）：
// 无站点分类时父任务身份/local 键/子任务哈希身份维持既有形态，PluginData 不携带归属字段
func TestCreateLocalEmissionFormUnchanged(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.png"), "a")
	writeFile(t, filepath.Join(root, "b.png"), "b")

	h := &LocalImportWorkFetcher{ctx: stubPluginCtx{}, classifier: NewPathClassifier(stubPluginCtx{})}
	resps := collectResponses(t, mustCreate(t, h, root))
	if len(resps) != 1 {
		t.Fatalf("根目录两文件应产出单父任务应答，实得 %d 个", len(resps))
	}
	r := resps[0]
	if r.SiteKey != identity.Local.Key || len(r.Children) != 2 {
		t.Fatalf("应答应为 local 父任务+2 子任务，实得 SiteKey=%q children=%d", r.SiteKey, len(r.Children))
	}
	if r.PluginTaskId != "local-dir-" || r.SiteWorkId != "local-dir-" {
		t.Fatalf("父任务身份应维持 local-dir 前缀形态，实得 %q/%q", r.PluginTaskId, r.SiteWorkId)
	}
	if r.ResourceType != "" {
		t.Fatalf("父任务不应声明 ResourceType，实得 %q", r.ResourceType)
	}
	for _, child := range r.Children {
		expectHash, err := ComputeFileHash(filepath.Join(root, child.TaskName))
		if err != nil || child.SiteWorkId != expectHash {
			t.Fatalf("local 子任务身份应为内容哈希，期望 %q 实得 %q (err=%v)", expectHash, child.SiteWorkId, err)
		}
		var fpMap map[string]any
		if err := json.Unmarshal([]byte(child.PluginData), &fpMap); err != nil {
			t.Fatalf("解析子任务 PluginData 失败: %v", err)
		}
		if _, ok := fpMap["siteKey"]; ok {
			t.Fatalf("local 归属 PluginData 不应携带 siteKey 字段")
		}
		if _, ok := fpMap["siteWorkId"]; ok {
			t.Fatalf("local 归属 PluginData 不应携带 siteWorkId 字段")
		}
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

// TestBuildDirGroupResponseRealParentIdentity 真实域多文件组：父任务身份加站点键后缀
// （PluginTaskId 是同流应答的父任务合并键，同目录拆分的两组须可区分），子任务带页级 ID
// 与归属字段；真实域单文件成叶任务（身份=页级 ID）
func TestBuildDirGroupResponseRealParentIdentity(t *testing.T) {
	dir := t.TempDir()
	f1 := writeFile(t, filepath.Join(dir, "11111111_p0.jpg"), "a")
	f2 := writeFile(t, filepath.Join(dir, "11111111_p1.jpg"), "b")
	files := []attributedFile{
		{entry: FileEntry{FullPath: f1, RelPath: "sub/11111111_p0.jpg", Hash: "h1"}, attr: workAttribution{SiteKey: identity.Pixiv.Key, PageWorkID: "11111111_p0"}},
		{entry: FileEntry{FullPath: f2, RelPath: "sub/11111111_p1.jpg", Hash: "h2"}, attr: workAttribution{SiteKey: identity.Pixiv.Key, PageWorkID: "11111111_p1"}},
	}
	resp := buildDirGroupResponse("root", "sub", "导入【root】", nil, files)
	if resp == nil {
		t.Fatalf("应产出父任务应答")
	}
	if resp.PluginTaskId != "local-dir-sub@"+identity.Pixiv.Key || resp.SiteWorkId != resp.PluginTaskId {
		t.Fatalf("真实域父任务身份应为 local-dir-sub@pixiv，实得 %q/%q", resp.PluginTaskId, resp.SiteWorkId)
	}
	if resp.SiteKey != identity.Pixiv.Key {
		t.Fatalf("真实域组应答站点键应为 pixiv，实得 %q", resp.SiteKey)
	}
	if len(resp.Children) != 2 {
		t.Fatalf("应含 2 子任务，实得 %d", len(resp.Children))
	}
	for _, child := range resp.Children {
		var fp FilePluginData
		if err := json.Unmarshal([]byte(child.PluginData), &fp); err != nil {
			t.Fatalf("解析子任务 PluginData 失败: %v", err)
		}
		if !fp.hasRealAttribution() || fp.SiteKey != identity.Pixiv.Key || fp.SiteWorkId != child.SiteWorkId {
			t.Fatalf("子任务 PluginData 应携带归属字段且与身份一致，实得 child=%q fp=%+v", child.SiteWorkId, fp)
		}
	}

	single := []attributedFile{files[0]}
	leaf := buildDirGroupResponse("root", "sub", "导入【root】", nil, single)
	if leaf == nil || leaf.Children != nil || leaf.PluginTaskId != "11111111_p0" || leaf.SiteWorkId != "11111111_p0" {
		t.Fatalf("真实域单文件应成叶任务(身份=页级 ID)，实得 %+v", leaf)
	}
}

func mustCreate(t *testing.T, h *LocalImportWorkFetcher, path string) *sdkdto.TaskCreateResult {
	t.Helper()
	result, err := h.Create(path)
	if err != nil {
		t.Fatalf("Create 不应报错: %v", err)
	}
	return result
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
