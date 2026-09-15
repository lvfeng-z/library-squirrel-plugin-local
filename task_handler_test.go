package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// stubPluginCtx 测试桩：嵌入 PluginContext 接口，仅实现 Create 链触及的方法
// （日志、站点查询、前端发布）；未实现方法一旦被调用即 panic 暴露测试缺口
type stubPluginCtx struct {
	sdkdto.PluginContext
	sites []*sdkdto.SiteDTO
}

func (c stubPluginCtx) Infof(string, ...any) {}
func (c stubPluginCtx) Warnf(string, ...any) {}

func (c stubPluginCtx) ListSites() (*sdkdto.ListSitesResponse, error) {
	return &sdkdto.ListSitesResponse{Items: c.sites}, nil
}

func (c stubPluginCtx) PublishToFrontend(string, []byte) error {
	return errors.New("测试环境无前端通道")
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

// TestCreateEmptyDirSetsReason 空目录创建任务时零任务返回并经 SetReason 声明用户可读原因；
// 路径不可访问仍走错误返回（经 SDK 的 error 块通道透传，不经 reason）
func TestCreateEmptyDirSetsReason(t *testing.T) {
	dir := t.TempDir()

	h := &LocalImportTaskHandler{}
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

// TestCreateSiteClassifiedSubtreeGoesRealDomain site 分类消费：目录命中 site 含义
// （面板选择项 value=站点 DB 行 id）时，子树内文件解析出的作品归该站点域；
// 解析不出页级 ID 的文件回退 local；层级作用域对照——更深层 site 规则不外溢到浅层文件
func TestCreateSiteClassifiedSubtreeGoesRealDomain(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "12345678_p0.jpg"), "a")
	os.MkdirAll(filepath.Join(root, "sub"), 0o755)
	writeFile(t, filepath.Join(root, "sub", "99999999_p1.png"), "b")
	os.MkdirAll(filepath.Join(root, "sub2"), 0o755)
	writeFile(t, filepath.Join(root, "sub2", "readme.txt"), "c")

	ctx := stubPluginCtx{sites: []*sdkdto.SiteDTO{
		{Id: 7, SiteKey: identity.Pixiv.Key},
		{Id: 9, SiteKey: identity.Bilibili.Key},
	}}
	classifier := NewPathClassifier(ctx)
	classifier.mu.Lock()
	classifier.learnedRules[0] = []PathMeaning{{Type: "site", ID: "7", Name: "pixiv"}}
	classifier.mu.Unlock()
	h := &LocalImportTaskHandler{ctx: ctx, classifier: classifier}

	resps := collectResponses(t, mustCreate(t, h, root))
	if len(resps) != 3 {
		t.Fatalf("三个目录组应各产出一应答（根/sub/sub2 各单文件成叶），实得 %d 个", len(resps))
	}

	// 根组：原图形态文件 → pixiv 页级叶任务
	pixivRoot := findResponse(resps, identity.Pixiv.Key)
	if pixivRoot == nil || pixivRoot.SiteWorkId != "12345678_p0" || pixivRoot.PluginTaskId != "12345678_p0" {
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

	// 层级作用域对照：更深层（level 1）的 site 规则只作用于该层子树——
	// 根层文件不受其影响（不外溢到浅层），按文件名形态独立锚定 pixiv
	root2 := t.TempDir()
	writeFile(t, filepath.Join(root2, "12345678_p0.jpg"), "d")
	os.MkdirAll(filepath.Join(root2, "deep"), 0o755)
	writeFile(t, filepath.Join(root2, "deep", "99999999_p1.png"), "e")

	classifier2 := NewPathClassifier(ctx)
	classifier2.mu.Lock()
	classifier2.learnedRules[1] = []PathMeaning{{Type: "site", ID: "9", Name: "bilibili"}}
	classifier2.mu.Unlock()
	h2 := &LocalImportTaskHandler{ctx: ctx, classifier: classifier2}
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

// TestCreateMixedDirSplitsByAttribution 同目录混合归属（原图形态+普通文件名）：
// 拆为两个应答各自携带站点键（子任务站点继承父应答，无法在同应答内混合归属）
func TestCreateMixedDirSplitsByAttribution(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "11111111_p0.jpg"), "a")
	writeFile(t, filepath.Join(root, "plain.png"), "b")

	h := &LocalImportTaskHandler{ctx: stubPluginCtx{}, classifier: NewPathClassifier(stubPluginCtx{})}
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

// TestCreateLocalEmissionFormUnchanged local 归属路径字段零变化（回归锚）：
// 无站点分类时父任务身份/local 键/子任务哈希身份维持既有形态，PluginData 不携带归属字段
func TestCreateLocalEmissionFormUnchanged(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.png"), "a")
	writeFile(t, filepath.Join(root, "b.png"), "b")

	h := &LocalImportTaskHandler{ctx: stubPluginCtx{}, classifier: NewPathClassifier(stubPluginCtx{})}
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
	h := &LocalImportTaskHandler{}
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
	h := &LocalImportTaskHandler{}
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
	h := &LocalImportTaskHandler{}
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

func mustCreate(t *testing.T, h *LocalImportTaskHandler, path string) *sdkdto.TaskCreateResult {
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
