// 全量识别引擎的单元测试：七类元素聚合（构造目录树：已识别/未识别有线索/
// 序列成群落单/待裁决各形态齐备）、结构形态表的形态判定与结构群聚合、
// 会话赋值按维度覆盖、谓词索引注入求值与分页拉全、死路径赋值报告、快照
// 往返、目录 mtime 增量、以及前端协议的 startRecognition / getPanorama 分派。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// —— 库查询桩（谓词索引构造的分页数据源）——

// libraryFixture 分页查询桩的数据集：五轨名字全集。
type libraryFixture struct {
	localAuthors     []string
	siteAuthors      []string
	localTags        []string
	siteTags         []string
	localWorkSets    []string
	failSiteTrack    bool
	failWorkSetTrack bool
}

// errIndexTrackFailure 桩的失败轨错误。
var errIndexTrackFailure = &plainError{"站点作者轨查询失败（测试桩）"}

// plainError 简单错误类型。
type plainError struct{ msg string }

func (e *plainError) Error() string { return e.msg }

// pageOf 按分页参数切片名字全集（页码从 1 起、页大小按请求生效）。
func pageOf(names []string, req *sdkdto.PageRequest) []string {
	page := int(req.GetPage())
	if page < 1 {
		page = 1
	}
	size := int(req.GetPageSize())
	if size < 1 {
		size = libraryIndexPageSize
	}
	start := (page - 1) * size
	if start >= len(names) {
		return nil
	}
	end := start + size
	if end > len(names) {
		end = len(names)
	}
	return names[start:end]
}

func (c *workbenchTestCtx) QueryLocalAuthors(req *sdkdto.QueryLocalAuthorsRequest) (*sdkdto.QueryLocalAuthorsResponse, error) {
	names := pageOf(c.library.localAuthors, req.Page)
	items := make([]*sdkdto.LocalAuthorDTO, 0, len(names))
	for _, name := range names {
		value := name
		items = append(items, &sdkdto.LocalAuthorDTO{AuthorName: &value})
	}
	return &sdkdto.QueryLocalAuthorsResponse{Items: items}, nil
}

func (c *workbenchTestCtx) QuerySiteAuthors(req *sdkdto.QuerySiteAuthorsRequest) (*sdkdto.QuerySiteAuthorsResponse, error) {
	if c.library.failSiteTrack {
		return nil, errIndexTrackFailure
	}
	names := pageOf(c.library.siteAuthors, req.Page)
	items := make([]*sdkdto.SiteAuthorInfo, 0, len(names))
	for _, name := range names {
		items = append(items, &sdkdto.SiteAuthorInfo{AuthorName: name})
	}
	return &sdkdto.QuerySiteAuthorsResponse{Items: items}, nil
}

func (c *workbenchTestCtx) QueryLocalTags(req *sdkdto.QueryLocalTagsRequest) (*sdkdto.QueryLocalTagsResponse, error) {
	names := pageOf(c.library.localTags, req.Page)
	items := make([]*sdkdto.LocalTagDTO, 0, len(names))
	for _, name := range names {
		value := name
		items = append(items, &sdkdto.LocalTagDTO{LocalTagName: &value})
	}
	return &sdkdto.QueryLocalTagsResponse{Items: items}, nil
}

func (c *workbenchTestCtx) QuerySiteTags(req *sdkdto.QuerySiteTagsRequest) (*sdkdto.QuerySiteTagsResponse, error) {
	names := pageOf(c.library.siteTags, req.Page)
	items := make([]*sdkdto.SiteTagInfo, 0, len(names))
	for _, name := range names {
		items = append(items, &sdkdto.SiteTagInfo{TagName: name})
	}
	return &sdkdto.QuerySiteTagsResponse{Items: items}, nil
}

func (c *workbenchTestCtx) QueryLocalWorkSets(req *sdkdto.QueryLocalWorkSetsRequest) (*sdkdto.QueryLocalWorkSetsResponse, error) {
	if c.library.failWorkSetTrack {
		return nil, errIndexTrackFailure
	}
	names := pageOf(c.library.localWorkSets, req.Page)
	items := make([]*sdkdto.WorkSetNameEntry, 0, len(names))
	for _, name := range names {
		// 行 id 跨页单调递增（同桩多次拉取也稳定唯一），供 WorkSetIDs 断言
		c.workSetIDSeq++
		items = append(items, &sdkdto.WorkSetNameEntry{Id: c.workSetIDSeq, Name: name})
	}
	return &sdkdto.QueryLocalWorkSetsResponse{Items: items}, nil
}

// —— 测试用规则构造 ——

// recognitionTestRules 构造识别测试的档案规则：兄弟群组定群（非下划线前缀 +
// 序号）、双 filename 命中的同层冲突（title 两值）、谓词作者规则（吃库内索引）、
// 目录层含义规则（「线索」层=标签）。
func recognitionTestRules(t *testing.T) []RuleEntry {
	t.Helper()
	siblingPattern, err := NewSiblingPattern(`^([^_]+)_`, `(\d+)`, 2, OutcomeCollection)
	if err != nil {
		t.Fatalf("构造兄弟群组规则失败: %v", err)
	}
	conflictA := mustRegexPattern(t, `^(?P<c1>A).+\.jpg$`, []string{"c1"})
	conflictB := mustRegexPattern(t, `^.+(?P<c2>B)\.jpg$`, []string{"c2"})
	dirAncestor := mustRegexPattern(t, `^线索$`, nil)
	dirPattern, err := NewLocationPattern(0, 0, dirAncestor)
	if err != nil {
		t.Fatalf("构造目录层位置模式失败: %v", err)
	}
	return []RuleEntry{
		{
			ID: "test-sibling-set", Description: "测试：下划线前缀+序号成群", Source: RuleSourceManual, Enabled: true,
			Pattern: siblingPattern,
		},
		{
			ID: "test-conflict-a", Description: "测试：标题冲突 A", Source: RuleSourceManual, Enabled: true,
			Pattern: conflictA,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c1"}},
		},
		{
			ID: "test-conflict-b", Description: "测试：标题冲突 B", Source: RuleSourceManual, Enabled: true,
			Pattern: conflictB,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c2"}},
		},
		{
			ID: "test-predicate-author", Description: "测试：文件名命中库内作者", Source: RuleSourceManual, Enabled: true,
			Pattern: &PredicatePattern{Kind: PredicateAuthor},
			Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromPredicate}},
		},
		{
			ID: "test-dir-clue-tag", Description: "测试：线索层目录=标签", Source: RuleSourceManual, Enabled: true,
			Pattern: dirPattern,
			Extractions: []ExtractionAction{{Dimension: DimTag, From: FromAncestorDir}},
		},
	}
}

// newRecognitionFixture 构造识别干跑测试环境：内存存储 + 桩上下文 + 测试档案
//（含禁用规则一条，供元素6 启用态呈现）+ 指定库根会话与赋值。
func newRecognitionFixture(t *testing.T, root string, assigns map[string]SessionAssignment) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	ctx.library = libraryFixture{
		localAuthors: []string{"库内作者名"},
		siteAuthors:  []string{"站点作者甲"},
		localTags:    []string{"风景"},
		// 集名互证：与序列群分组键同名（群组键「库内作者名」同时是库内既有集名）
		localWorkSets: []string{"库内作者名"},
	}
	m := NewSessionManagerWithStore(ctx, nil, store)

	rules := recognitionTestRules(t)
	rules = append(rules, RuleEntry{
		ID: "test-disabled", Description: "测试：禁用规则不参与求值", Source: RuleSourceManual, Enabled: false,
		Pattern: mustRegexPattern(t, `^never_.+\.jpg$`, nil),
	})
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: rules}
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}

	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if len(assigns) > 0 {
		sess, err := m.loadSession(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		sess.Assignments = assigns
		if err := m.saveSession(sess); err != nil {
			t.Fatalf("注入会话赋值失败: %v", err)
		}
	}
	return m, ctx
}

// mustRegexPattern 测试内构造正则模式（失败即中止）。
func mustRegexPattern(t *testing.T, expr string, captures []string) *RegexPattern {
	t.Helper()
	pattern, err := NewRegexPattern(string(TargetFileName), expr, captures)
	if err != nil {
		t.Fatalf("构造正则模式 %q 失败: %v", expr, err)
	}
	return pattern
}

// writeTestFile 构造测试文件（父目录自动创建）。
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}

// recognitionSampleTree 构造样例目录树（七类元素各形态齐备；群组前缀
//「库内作者名」命中库内索引，验证群组前缀侧的谓词线索）：
//
//	<root>/已识别/【作者A】标题1.jpg      内置规则命中 → identified
//	<root>/线索/任意名.jpg                目录层含义规则（线索层=标签）→ identified
//	<root>/12345678_p0.jpg               pixiv 形态 → clueful(pixiv-id)
//	<root>/库内作者名.jpg                 谓词规则注入索引后命中 → identified(predicate 源)
//	<root>/序列/库内作者名_001..003.jpg   兄弟群组成群：前缀命中库内作者（谓词线索）
//	                                     + 集名候选（群组前缀线索），零填充序号
//	<root>/序列/lone_007.jpg             序号形态但组大小 1 → 落单（plain）
//	<root>/冲突/AB.jpg                   双规则同层 title 冲突 → 待裁决（元素5）
//	<root>/顶层说明.txt                  无任何特征 → plain
func recognitionSampleTree(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, root+"/已识别/【作者A】标题1.jpg", "a")
	writeTestFile(t, root+"/线索/任意名.jpg", "b")
	writeTestFile(t, root+"/12345678_p0.jpg", "c")
	writeTestFile(t, root+"/库内作者名.jpg", "d")
	writeTestFile(t, root+"/序列/库内作者名_001.jpg", "e")
	writeTestFile(t, root+"/序列/库内作者名_002.jpg", "f")
	writeTestFile(t, root+"/序列/库内作者名_003.jpg", "g")
	writeTestFile(t, root+"/序列/lone_007.jpg", "h")
	writeTestFile(t, root+"/冲突/AB.jpg", "i")
	writeTestFile(t, root+"/顶层说明.txt", "j")
}

// recognitionSampleRelPaths 样例树的全量相对路径清单（总数断言用）。
var recognitionSampleRelPaths = []string{
	"已识别/【作者A】标题1.jpg",
	"线索/任意名.jpg",
	"12345678_p0.jpg",
	"库内作者名.jpg",
	"序列/库内作者名_001.jpg",
	"序列/库内作者名_002.jpg",
	"序列/库内作者名_003.jpg",
	"序列/lone_007.jpg",
	"冲突/AB.jpg",
	"顶层说明.txt",
}

// TestRunFullRecognitionSevenElements 全量识别干跑的七类元素聚合断言。
func TestRunFullRecognitionSevenElements(t *testing.T) {
	root := t.TempDir()
	// 死路径赋值：引用不存在的相对路径
	m, _ := newRecognitionFixture(t, root, map[string]SessionAssignment{
		"不存在的目录/旧文件.jpg": {Author: "旧作者", UpdatedAtMs: 1},
	})
	recognitionSampleTree(t, root)

	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if sess.State != WorkbenchSessionRecognized {
		t.Fatalf("识别完成后会话状态 = %s，期望 recognized", sess.State)
	}
	snap := sess.Snapshot
	if snap == nil {
		t.Fatal("识别完成后快照不在场")
	}

	fileByRel := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		fileByRel[f.RelPath] = f
	}
	assertHasDecl := func(f SnapFile, dim, value string) {
		t.Helper()
		for _, decl := range f.Final {
			if decl.Dimension == dim && decl.Value == value {
				return
			}
		}
		t.Fatalf("文件 %s 缺少 %s=%s 声明（终选: %+v）", f.RelPath, dim, value, f.Final)
	}
	assertHasClue := func(f SnapFile, kind string) {
		t.Helper()
		for _, clue := range f.Clues {
			if clue.Kind == kind {
				return
			}
		}
		t.Fatalf("文件 %s 缺少 %s 线索（线索: %+v）", f.RelPath, kind, f.Clues)
	}

	// —— 元素2：已识别资源（终选声明 + 分组归属）——
	identified := fileByRel["已识别/【作者A】标题1.jpg"]
	if identified.State != snapStateIdentified {
		t.Fatalf("内置规则命中文件状态 = %s，期望 identified（终选声明: %+v）", identified.State, identified.Final)
	}
	assertHasDecl(identified, "author", "作者A")
	assertHasDecl(identified, "title", "标题1")

	// 谓词索引注入求值：库内作者名命中谓词规则 → author 声明（predicate 源）
	predHit := fileByRel["库内作者名.jpg"]
	if predHit.State != snapStateIdentified {
		t.Fatalf("谓词规则命中文件状态 = %s，期望 identified（终选: %+v）", predHit.State, predHit.Final)
	}
	assertHasDecl(predHit, "author", "库内作者名")
	foundPredicate := false
	for _, decl := range predHit.Final {
		if decl.Source == string(SourcePredicate) {
			foundPredicate = true
		}
	}
	if !foundPredicate {
		t.Fatalf("谓词声明未产出（终选: %+v）", predHit.Final)
	}

	// —— 元素3：有价值未识别（线索类别齐备）——
	pixiv := fileByRel["12345678_p0.jpg"]
	if pixiv.State != snapStateClueful {
		t.Fatalf("pixiv 形态文件状态 = %s，期望 clueful（线索: %+v）", pixiv.State, pixiv.Clues)
	}
	assertHasClue(pixiv, cluePixivID)

	// —— 目录层含义规则验证（元素1 的正向覆盖）：线索层文件带 tag 声明 ——
	dirCovered := fileByRel["线索/任意名.jpg"]
	if dirCovered.State != snapStateIdentified {
		t.Fatalf("目录含义覆盖文件状态 = %s，期望 identified（终选: %+v）", dirCovered.State, dirCovered.Final)
	}
	assertHasDecl(dirCovered, "tag", "线索")

	// —— 元素4：序列群（公共前缀/组大小/零填充序号原文/成群落单）——
	if len(snap.Groups) != 1 {
		t.Fatalf("序列群数量 = %d，期望 1（groups: %+v）", len(snap.Groups), snap.Groups)
	}
	group := snap.Groups[0]
	if group.DirRelKey != "序列" || group.Key != "库内作者名" || group.Size != 3 {
		t.Fatalf("序列群形态不符: %+v", group)
	}
	if group.RuleID != "test-sibling-set" {
		t.Fatalf("定群规则 = %s，期望 test-sibling-set", group.RuleID)
	}
	if len(group.Members) != 3 {
		t.Fatalf("群组成员数 = %d，期望 3", len(group.Members))
	}
	for i, member := range group.Members {
		if wantSeq := i + 1; member.Seq != wantSeq {
			t.Fatalf("成员 %s 集内序 = %d，期望 %d", member.RelPath, member.Seq, wantSeq)
		}
		if wantText := []string{"001", "002", "003"}[i]; member.SeqText != wantText {
			t.Fatalf("成员 %s 序号原文 = %q，期望 %q（零填充形态）", member.RelPath, member.SeqText, wantText)
		}
	}
	// 群组成员无元数据声明 → 群组前缀谓词命中 + 集名候选双线索；分组键命中库内
	// 既有集名（互证索引）→ 集名互证线索（挂点验证：attachSetNameCrossCheck 在
	// aggregatePanorama 内生效）
	setMember := fileByRel["序列/库内作者名_001.jpg"]
	if setMember.State != snapStateClueful {
		t.Fatalf("群组成员状态 = %s，期望 clueful（线索: %+v）", setMember.State, setMember.Clues)
	}
	assertHasClue(setMember, cluePredicateAuthor)
	assertHasClue(setMember, clueGroupPrefix)
	assertHasClue(setMember, clueSetMatch)
	if setMember.GroupKey != "库内作者名" || setMember.GroupSeq != 1 {
		t.Fatalf("群组成员分组归属不符: key=%s seq=%d", setMember.GroupKey, setMember.GroupSeq)
	}
	// 落单：同序号形态但组大小 1，不成群
	lone := fileByRel["序列/lone_007.jpg"]
	if lone.GroupKey != "" {
		t.Fatalf("落单文件带群组键 %q", lone.GroupKey)
	}
	if lone.State != snapStatePlain {
		t.Fatalf("落单无特征文件状态 = %s，期望 plain", lone.State)
	}

	// —— 元素5：待裁决项（冲突维度与候选对照）——
	conflict := fileByRel["冲突/AB.jpg"]
	if len(conflict.PendingDims) != 1 || conflict.PendingDims[0] != "title" {
		t.Fatalf("冲突文件待裁决维度 = %+v，期望 [title]", conflict.PendingDims)
	}
	if len(snap.Pending) != 1 {
		t.Fatalf("待裁决明细数 = %d，期望 1（%+v）", len(snap.Pending), snap.Pending)
	}
	pending := snap.Pending[0]
	if pending.RelPath != "冲突/AB.jpg" || pending.Dimension != "title" {
		t.Fatalf("待裁决明细形态不符: %+v", pending)
	}
	if len(pending.Candidates) != 2 {
		t.Fatalf("待裁决候选数 = %d，期望 2（双方规则声明对照）", len(pending.Candidates))
	}

	// —— 元素1：目录结构与层含义（规则覆盖与未覆盖）——
	dirByRel := make(map[string]SnapDir, len(snap.Dirs))
	for _, d := range snap.Dirs {
		dirByRel[d.RelKey] = d
	}
	rootDir, ok := dirByRel[""]
	if !ok || rootDir.Depth != 0 {
		t.Fatalf("目录树缺根节点: %+v", snap.Dirs)
	}
	clueDir, ok := dirByRel["线索"]
	if !ok {
		t.Fatal("目录树缺「线索」层")
	}
	if len(clueDir.Meanings) != 1 || clueDir.Meanings[0].Type != string(PathTypeLocalTag) || clueDir.Meanings[0].Name != "线索" {
		t.Fatalf("「线索」层含义不符: %+v（期望 localTag:线索）", clueDir.Meanings)
	}
	if clueDir.MeaningRuleID != "test-dir-clue-tag" {
		t.Fatalf("「线索」层覆盖规则 = %s", clueDir.MeaningRuleID)
	}
	uncovered, ok := dirByRel["序列"]
	if !ok || len(uncovered.Meanings) != 0 {
		t.Fatalf("「序列」层应为未覆盖（无含义），实际: %+v", uncovered.Meanings)
	}

	// —— 元素6：规则档案态（来源/启用态/命中数）——
	statByID := make(map[string]SnapRuleStat, len(snap.RuleStats))
	for _, stat := range snap.RuleStats {
		statByID[stat.ID] = stat
	}
	if stat := statByID["test-conflict-a"]; !stat.Enabled || stat.HitCount != 1 {
		t.Fatalf("冲突规则 A 统计不符: %+v", stat)
	}
	if stat := statByID["test-disabled"]; stat.Enabled || stat.HitCount != 0 {
		t.Fatalf("禁用规则统计不符: %+v", stat)
	}
	if stat := statByID["builtin-bracket-author-title"]; stat.HitCount != 1 {
		t.Fatalf("内置规则命中数不符: %+v", stat)
	}

	// —— 元素7：会话决策态骨架（死路径赋值报告；赋值/忽略/选择集计数活数据面
	// 由 getPanorama 协议测试覆盖）——
	if len(snap.StaleAssignments) != 1 || snap.StaleAssignments[0] != "不存在的目录/旧文件.jpg" {
		t.Fatalf("死路径赋值报告不符: %+v", snap.StaleAssignments)
	}

	// —— 汇总统计一致性 ——
	if snap.Stats.TotalFiles != len(recognitionSampleRelPaths) {
		t.Fatalf("总文件数 = %d，期望 %d", snap.Stats.TotalFiles, len(recognitionSampleRelPaths))
	}
	if snap.Stats.Identified != 3 {
		t.Fatalf("已识别计数 = %d，期望 3（内置 + 目录含义 + 谓词）", snap.Stats.Identified)
	}
	if snap.Stats.Clueful != 4 {
		t.Fatalf("有线索计数 = %d，期望 4（pixiv + 三名群组成员）", snap.Stats.Clueful)
	}
	if snap.Stats.Plain != 3 {
		t.Fatalf("无线索计数 = %d，期望 3（落单 + 顶层 txt + 冲突文件）", snap.Stats.Plain)
	}
	if snap.Stats.PendingFiles != 1 || snap.Stats.GroupCount != 1 || snap.Stats.StaleAssignments != 1 {
		t.Fatalf("待裁决/群组/死路径计数不符: %+v", snap.Stats)
	}
}

// TestRunFullRecognitionAssignmentOverride 会话赋值参与求值：显式维度覆盖档案
// 结论、未指定维度保留。
func TestRunFullRecognitionAssignmentOverride(t *testing.T) {
	root := t.TempDir()
	// 对内置规则命中的文件赋值：覆盖 author 维度（识别错的修正），title 未指定保留
	m, _ := newRecognitionFixture(t, root, map[string]SessionAssignment{
		"已识别/【作者A】标题1.jpg": {Author: "修正作者", UpdatedAtMs: 9},
	})
	writeTestFile(t, root+"/已识别/【作者A】标题1.jpg", "a")

	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, _ := m.SessionOf(root)
	var target SnapFile
	for _, f := range sess.Snapshot.Files {
		if f.RelPath == "已识别/【作者A】标题1.jpg" {
			target = f
		}
	}
	if !target.Assigned {
		t.Fatal("赋值文件缺 assigned 标记")
	}
	var authorValues, titleValues []string
	for _, decl := range target.Final {
		switch decl.Dimension {
		case "author":
			authorValues = append(authorValues, decl.Value)
		case "title":
			titleValues = append(titleValues, decl.Value)
		}
	}
	if len(authorValues) != 1 || authorValues[0] != "修正作者" {
		t.Fatalf("赋值后作者声明 = %v，期望仅 [修正作者]（档案同维度被覆盖）", authorValues)
	}
	if len(titleValues) != 1 || titleValues[0] != "标题1" {
		t.Fatalf("未指定维度应保留档案结论，title = %v，期望 [标题1]", titleValues)
	}
}

// TestRunFullRecognitionIncremental 目录 mtime 增量：档案指纹一致时未变目录跳过
// 求值并复用旧结论；目录内容变化后重算该目录。
func TestRunFullRecognitionIncremental(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)

	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮识别失败: %v", err)
	}
	first, _ := m.SessionOf(root)
	if first.Snapshot.Stats.SkippedDirs != 0 {
		t.Fatalf("首轮识别不应有跳过目录，实际 %d", first.Snapshot.Stats.SkippedDirs)
	}

	// 二轮：磁盘未变、档案未变 → 全部目录按游标跳过，结论复用
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("增量识别失败: %v", err)
	}
	second, _ := m.SessionOf(root)
	if second.Snapshot.Stats.SkippedDirs == 0 {
		t.Fatal("增量识别应跳过未变目录，实际跳过 0 个")
	}
	// 结论性统计不漂移（SkippedDirs 是本轮增量事实，不参与对比）
	first.Snapshot.Stats.SkippedDirs, second.Snapshot.Stats.SkippedDirs = 0, 0
	if !reflect.DeepEqual(first.Snapshot.Stats, second.Snapshot.Stats) {
		t.Fatalf("增量轮统计漂移: 首轮 %+v，增量轮 %+v", first.Snapshot.Stats, second.Snapshot.Stats)
	}
	if second.Snapshot.GeneratedAtMs == first.Snapshot.GeneratedAtMs {
		t.Fatal("增量轮应产出新快照")
	}

	// 磁盘变化：新目录写入 → 总数演进、新增文件有结论
	time.Sleep(2 * time.Millisecond)
	writeTestFile(t, root+"/新目录/新增.txt", "new")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("变化后识别失败: %v", err)
	}
	third, _ := m.SessionOf(root)
	if third.Snapshot.Stats.TotalFiles != len(recognitionSampleRelPaths)+1 {
		t.Fatalf("变化后总文件数 = %d", third.Snapshot.Stats.TotalFiles)
	}
	var newFile *SnapFile
	for i := range third.Snapshot.Files {
		if third.Snapshot.Files[i].RelPath == "新目录/新增.txt" {
			newFile = &third.Snapshot.Files[i]
		}
	}
	if newFile == nil || newFile.State != snapStatePlain {
		t.Fatalf("新增文件结论缺失或状态不符: %+v", newFile)
	}
}

// TestScannerEnumerateMtimeIncrement 轻量枚举的目录级增量语义。
func TestScannerEnumerateMtimeIncrement(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root+"/a/one.txt", "1")
	writeTestFile(t, root+"/b/sub/two.txt", "2")

	scanner := NewScanner(root)
	first, err := scanner.Enumerate(nil)
	if err != nil {
		t.Fatalf("首轮枚举失败: %v", err)
	}
	if len(first.Files) != 2 || len(first.AllFiles) != 2 {
		t.Fatalf("首轮应全量求值: files=%d all=%d", len(first.Files), len(first.AllFiles))
	}

	second, err := scanner.Enumerate(first.DirMtimes)
	if err != nil {
		t.Fatalf("增量枚举失败: %v", err)
	}
	if len(second.Files) != 0 {
		t.Fatalf("未变目录不应重求值，实际 %d 个文件", len(second.Files))
	}
	if len(second.AllFiles) != 2 {
		t.Fatalf("全量清单应保持: %d", len(second.AllFiles))
	}
	// 子目录内部变化只改子目录 mtime：父目录仍跳过、变化子目录的全部直接文件重求值
	time.Sleep(2 * time.Millisecond)
	writeTestFile(t, root+"/b/sub/three.txt", "3")
	third, err := scanner.Enumerate(second.DirMtimes)
	if err != nil {
		t.Fatalf("变化后枚举失败: %v", err)
	}
	if len(third.Files) != 2 || third.Files[0].RelPath != "b/sub/three.txt" || third.Files[1].RelPath != "b/sub/two.txt" {
		t.Fatalf("变化子目录应整目录重求值（two+three）: %+v", third.Files)
	}
	if !containsStr(third.SkippedDirKeys, "") || !containsStr(third.SkippedDirKeys, "a") {
		t.Fatalf("未变目录应保持跳过: %+v", third.SkippedDirKeys)
	}
}

// containsStr 切片包含判定。
func containsStr(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// TestRecognitionSnapshotRoundTrip 快照序列化往返等价（会话 KV 裁剪形态的存取）。
func TestRecognitionSnapshotRoundTrip(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, _ := m.SessionOf(root)

	// 会话工件整体往返（快照嵌在会话 KV 内）
	data, err := json.Marshal(sess)
	if err != nil {
		t.Fatalf("会话序列化失败: %v", err)
	}
	var restored WorkbenchSession
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("会话反序列化失败: %v", err)
	}
	if !reflect.DeepEqual(sess.Snapshot, restored.Snapshot) {
		t.Fatal("快照往返不等价")
	}

	// 经存储层重读（loadSession 路径）
	loaded, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("重读会话失败: %v", err)
	}
	if !reflect.DeepEqual(sess.Snapshot, loaded.Snapshot) {
		t.Fatal("快照经 KV 往返不等价")
	}
}

// TestBuildLibraryIndexPagingAndFallback 谓词索引构造：分页拉全（条数跨页）与
// 失败轨降级、查询源不可用降 nil。
func TestBuildLibraryIndexPagingAndFallback(t *testing.T) {
	// 跨页：本地作者 405 名（页大小 200 → 3 页）
	localAuthors := make([]string, 405)
	for i := range localAuthors {
		localAuthors[i] = "作者" + string(rune('A'+i%26)) + string(rune('0'+i%10)) + string(rune('a'+i%26)) + string(rune('0'+(i/10)%10))
	}
	// 跨页：库内作品集 205 名（第五轨，页大小 200 → 2 页）
	localWorkSets := make([]string, 205)
	for i := range localWorkSets {
		localWorkSets[i] = "作品集" + string(rune('A'+i%26)) + string(rune('0'+i%10)) + string(rune('a'+i%26)) + string(rune('0'+(i/10)%10))
	}
	ctx := &workbenchTestCtx{store: newFakeSessionStore()}
	ctx.library = libraryFixture{
		localAuthors:  localAuthors,
		localTags:     []string{"风景"},
		siteAuthors:   []string{"站点作者甲"},
		localWorkSets: localWorkSets,
		failSiteTrack: false,
	}
	lib := BuildLibraryIndex(ctx, nil)
	if lib == nil {
		t.Fatal("索引不应为 nil")
	}
	if len(lib.AuthorNames) != len(localAuthors)+1 {
		t.Fatalf("跨页拉全后作者名 = %d，期望 %d", len(lib.AuthorNames), len(localAuthors)+1)
	}
	if _, ok := lib.TagNames["风景"]; !ok {
		t.Fatal("本地标签未入索引")
	}
	if len(lib.WorkSetNames) != len(localWorkSets) {
		t.Fatalf("跨页拉全后作品集名 = %d，期望 %d", len(lib.WorkSetNames), len(localWorkSets))
	}
	if _, ok := lib.WorkSetNames[localWorkSets[0]]; !ok {
		t.Fatal("库内作品集名未入索引")
	}
	// 集 id 索引与集名全集同轨同源：拉全后逐名有行 id（桩按页序发号，首名=1、
	// 跨页末名=205——跨页条目的 id 不因翻页重置）
	if len(lib.WorkSetIDs) != len(localWorkSets) {
		t.Fatalf("跨页拉全后集名→行 id = %d，期望 %d", len(lib.WorkSetIDs), len(localWorkSets))
	}
	if id := lib.WorkSetIDs[localWorkSets[0]]; id != 1 {
		t.Fatalf("首个集名的行 id = %d，期望 1", id)
	}
	if id := lib.WorkSetIDs[localWorkSets[len(localWorkSets)-1]]; id != int64(len(localWorkSets)) {
		t.Fatalf("跨页末名的行 id = %d，期望 %d", id, len(localWorkSets))
	}

	// 失败轨（站点作者 / 库内作品集）按空集降级：不污染其余轨道、不返回 nil
	failCtx := &workbenchTestCtx{store: newFakeSessionStore()}
	failCtx.library = libraryFixture{
		localAuthors:     []string{"仅本地"},
		localWorkSets:    []string{"不该出现的集名"},
		failSiteTrack:    true,
		failWorkSetTrack: true,
	}
	failLib := BuildLibraryIndex(failCtx, nil)
	if failLib == nil || len(failLib.AuthorNames) != 1 {
		t.Fatalf("失败轨降级形态不符: %+v", failLib.AuthorNames)
	}
	if len(failLib.WorkSetNames) != 0 {
		t.Fatalf("作品集失败轨应按空集降级，实际: %+v", failLib.WorkSetNames)
	}
	if len(failLib.WorkSetIDs) != 0 {
		t.Fatalf("作品集失败轨的集 id 索引应按空集降级，实际: %+v", failLib.WorkSetIDs)
	}

	// 查询源不可用 → nil 索引（谓词恒不命中）
	if got := BuildLibraryIndex(nil, nil); got != nil {
		t.Fatal("查询源不可用应返回 nil 索引")
	}
}

// TestHandleFrontendRequestRecognitionActions 前端协议：getPanorama 未识别错误
// 应答、识别后全量/分节拉取与决策态附带、startRecognition 启动应答与无会话错误。
func TestHandleFrontendRequestRecognitionActions(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)

	// getPanorama：未识别时错误应答
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 101, Action: workbenchActionGetPanorama, Root: root}))
	if resp := ctx.lastResponse(t); resp.OK || resp.Seq != 101 {
		t.Fatalf("未识别会话的 getPanorama 应错误应答: %+v", resp)
	}

	// 同步跑完识别（协议层 startRecognition 为异步包装，此处直调主体保证确定性）
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 102, Action: workbenchActionGetPanorama, Root: root}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Seq != 102 || resp.Panorama == nil {
		t.Fatalf("getPanorama 失败: %+v", resp)
	}
	if resp.Panorama.DirMtimes != nil {
		t.Fatal("前端投影不应携带增量游标表")
	}
	if len(resp.Panorama.Dirs) == 0 || len(resp.Panorama.Files) == 0 || len(resp.Panorama.RuleStats) == 0 {
		t.Fatalf("全量投影缺节: dirs=%d files=%d rules=%d",
			len(resp.Panorama.Dirs), len(resp.Panorama.Files), len(resp.Panorama.RuleStats))
	}
	if resp.Decisions == nil {
		t.Fatal("getPanorama 应附带会话决策态（元素7 骨架）")
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 103, Action: workbenchActionGetPanorama, Root: root, Section: panoramaSectionFiles}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.Seq != 103 || resp.Panorama == nil {
		t.Fatalf("分节 getPanorama 失败: %+v", resp)
	}
	if len(resp.Panorama.Files) != len(recognitionSampleRelPaths) || len(resp.Panorama.Groups) != 1 {
		t.Fatalf("文件级分节形态不符: files=%d groups=%d", len(resp.Panorama.Files), len(resp.Panorama.Groups))
	}
	if len(resp.Panorama.Dirs) != 0 || len(resp.Panorama.RuleStats) != 0 {
		t.Fatal("文件级分节不应携带目录树与规则态")
	}

	// startRecognition：完成态再启动=合法重识别请求（started=true，异步执行）
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 104, Action: workbenchActionStartRecognition, Root: root}))
	if resp := ctx.lastResponse(t); !resp.OK || !resp.Started {
		t.Fatalf("startRecognition 应答不符: %+v", resp)
	}

	// 无会话库根：错误应答
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 105, Action: workbenchActionStartRecognition, Root: root + "/不存在"}))
	if resp := ctx.lastResponse(t); resp.OK {
		t.Fatal("无会话库根的 startRecognition 不应成功")
	}
}


// TestDetectStructuralGroups 验证规则无关的兄弟群结构探测（元素4 结构面与
// 元素3 集名候选线索）：尾缀数字形态成群、序号形态单成员不成群、已识别成员
// 入群不加线索、规则群成员不参与。
func TestDetectStructuralGroups(t *testing.T) {
	snap := &RecognitionSnapshot{Files: []SnapFile{
		{RelPath: "画集 01.jpg", FileName: "画集 01.jpg", DirRelKey: "d", State: snapStatePlain},
		{RelPath: "画集 02.jpg", FileName: "画集 02.jpg", DirRelKey: "d", State: snapStatePlain},
		{RelPath: "fill_0001.jpg", FileName: "fill_0001.jpg", DirRelKey: "d", State: snapStatePlain},
		{RelPath: "作品甲 v1.jpg", FileName: "作品甲 v1.jpg", DirRelKey: "d", State: snapStatePlain},
		{RelPath: "成书 01.jpg", FileName: "成书 01.jpg", DirRelKey: "d", State: snapStateIdentified, Final: []SnapDecl{{Dimension: "title", Value: "x"}}},
		{RelPath: "规则群 03.jpg", FileName: "规则群 03.jpg", DirRelKey: "d", State: snapStatePlain, GroupKey: "已有"},
	}}
	detectStructuralGroups(snap)
	if len(snap.Groups) != 1 {
		t.Fatalf("结构群数量=%d，期望 1（画集 01/02）", len(snap.Groups))
	}
	g := snap.Groups[0]
	if g.Key != "画集" || g.Size != 2 || g.RuleID != "" || len(g.Members) != 2 {
		t.Fatalf("结构群字段不符：key=%q size=%d rule=%q members=%d", g.Key, g.Size, g.RuleID, len(g.Members))
	}
	if g.FormName != "尾缀数字" || g.FormKeyExpr != `^(.+?)[ _\-·.．]+\d{1,4}$` || g.FormSeqExpr != `(\d{1,4})$` {
		t.Fatalf("结构群形态标识不符：name=%q keyExpr=%q seqExpr=%q", g.FormName, g.FormKeyExpr, g.FormSeqExpr)
	}
	if g.Members[0].SeqText != "01" || g.Members[0].Seq != 1 || g.Members[1].Seq != 2 {
		t.Fatalf("成员序号形态不符：%+v", g.Members)
	}
	byPath := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	f := byPath["画集 01.jpg"]
	if f.State != snapStateClueful || f.GroupKey != "画集" || len(f.Clues) != 1 || f.Clues[0].Kind != clueGroupPrefix || f.Clues[0].Detail != "画集" {
		t.Fatalf("群组成员线索/升格不符：%+v", f)
	}
	if p := byPath["作品甲 v1.jpg"]; p.GroupKey != "" || p.State != snapStatePlain {
		t.Fatalf("直连序号形态单成员不应成群或升格：%+v", p)
	}
	if p := byPath["fill_0001.jpg"]; p.GroupKey != "" {
		t.Fatalf("落单文件不应入群：%+v", p)
	}
	if p := byPath["规则群 03.jpg"]; p.GroupKey != "已有" {
		t.Fatalf("规则群成员不应被结构群改写：%+v", p)
	}
	if p := byPath["成书 01.jpg"]; p.State != snapStateIdentified || len(p.Clues) != 0 {
		t.Fatalf("已识别成员仅入群呈现、不加线索：%+v", p)
	}
}

// newCrossCheckSnap 集名互证测试快照：规则群（键命中库内集名）、结构群（键未命中）、
// 已识别成员的命中群三群与对应文件记录。
func newCrossCheckSnap() *RecognitionSnapshot {
	plain := func(relPath, dirKey string) SnapFile {
		return SnapFile{RelPath: relPath, FileName: fileNameOfRel(relPath), DirRelKey: dirKey, State: snapStatePlain}
	}
	return &RecognitionSnapshot{
		Files: []SnapFile{
			plain("甲/既有画集_01.jpg", "甲"),
			plain("甲/既有画集_02.jpg", "甲"),
			plain("乙/新画集_01.jpg", "乙"),
			plain("乙/新画集_02.jpg", "乙"),
			{RelPath: "丙/既有画集_01.jpg", FileName: "既有画集_01.jpg", DirRelKey: "丙",
				State: snapStateIdentified, Final: []SnapDecl{{Dimension: "title", Value: "x"}}},
		},
		Groups: []SnapGroup{
			// 规则定群（RuleID 非空）：分群键命中库内集名
			{DirRelKey: "甲", Key: "既有画集", RuleID: "rule-x", Members: []SnapGroupMember{
				{RelPath: "甲/既有画集_01.jpg", Seq: 1, SeqText: "01"},
				{RelPath: "甲/既有画集_02.jpg", Seq: 2, SeqText: "02"},
			}},
			// 结构群（RuleID 空）：分群键未命中
			{DirRelKey: "乙", Key: "新画集", Members: []SnapGroupMember{
				{RelPath: "乙/新画集_01.jpg", Seq: 1, SeqText: "01"},
				{RelPath: "乙/新画集_02.jpg", Seq: 2, SeqText: "02"},
			}},
			// 已识别成员的命中群：补线索但不降格
			{DirRelKey: "丙", Key: "既有画集", Members: []SnapGroupMember{
				{RelPath: "丙/既有画集_01.jpg", Seq: 1, SeqText: "01"},
			}},
		},
	}
}

// TestAttachSetNameCrossCheck 集名互证三态：a) 分群键命中库内集名 → 全员（规则群与
// 结构群同权）补 set-name-match 线索、plain 升格 clueful、已识别成员补线索不降格、
// 同 Detail 去重；b) 未命中 → 无线索零作为；c) lib 为 nil 或集名索引为空 → 零作为。
func TestAttachSetNameCrossCheck(t *testing.T) {
	lib := &LibraryIndex{WorkSetNames: map[string]struct{}{"既有画集": {}}}

	// a) 命中
	snap := newCrossCheckSnap()
	attachSetNameCrossCheck(snap, lib)
	byPath := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	for _, rel := range []string{"甲/既有画集_01.jpg", "甲/既有画集_02.jpg"} {
		f := byPath[rel]
		if f.State != snapStateClueful {
			t.Fatalf("%s 应升格 clueful（plain 补线索），实际 %s（线索: %+v）", rel, f.State, f.Clues)
		}
		if len(f.Clues) != 1 || f.Clues[0].Kind != clueSetMatch || f.Clues[0].Detail != "既有画集" {
			t.Fatalf("%s 集名互证线索不符: %+v", rel, f.Clues)
		}
	}
	identified := byPath["丙/既有画集_01.jpg"]
	if identified.State != snapStateIdentified || len(identified.Clues) != 1 || identified.Clues[0].Kind != clueSetMatch {
		t.Fatalf("已识别成员应补线索不降格: state=%s clues=%+v", identified.State, identified.Clues)
	}
	// 去重：重复调用同 Detail 不再追加
	attachSetNameCrossCheck(snap, lib)
	if f := byPath["甲/既有画集_01.jpg"]; len(f.Clues) != 1 {
		t.Fatalf("同 Detail 的集名互证线索不应重复追加: %+v", f.Clues)
	}

	// b) 未命中 → 无线索、状态不动
	if f := byPath["乙/新画集_01.jpg"]; f.State != snapStatePlain || len(f.Clues) != 0 {
		t.Fatalf("分群键未命中不应加线索或升格: state=%s clues=%+v", f.State, f.Clues)
	}

	// c) lib 为 nil / 集名索引为空 → 零作为（全员无新线索、状态保持构造态）
	nilSnap := newCrossCheckSnap()
	attachSetNameCrossCheck(nilSnap, nil)
	emptySnap := newCrossCheckSnap()
	attachSetNameCrossCheck(emptySnap, &LibraryIndex{WorkSetNames: map[string]struct{}{}})
	for name, s := range map[string]*RecognitionSnapshot{"lib=nil": nilSnap, "空集名索引": emptySnap} {
		for _, f := range s.Files {
			if len(f.Clues) != 0 || (f.State != snapStatePlain && f.State != snapStateIdentified) {
				t.Fatalf("%s 时文件 %s 应零作为: state=%s clues=%+v", name, f.RelPath, f.State, f.Clues)
			}
		}
	}
}

// formBracketKey 乙（括号序号）样本的期望分群键（各成员剥尾 `_[NN]` 后一致）。
const formBracketKey = "[くろニャン＠仕事募集中]_[100423498]_C100新刊告知"

// formDirectKey 丙（直连序号）样本的期望分群键（`]` 后直连数字不入键）。
const formDirectKey = "[Isel] [92242473] [瞬間的に絡み合う]"

// formMidTitle 丁（中段序号）样本的期望分群键（page 前标题，随机码不入键）。
const formMidTitle = "industrial_becca"

// TestStructuralFormMatch 结构形态表逐条目的键/序号提取判定：各形态实测样本
// 提取正确、排除样本零命中、同文件命中多形态时按表序（括号 → 中段 → 尾缀 →
// 直连）取先者。
func TestStructuralFormMatch(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		form    string // 期望命中的形态名；空串 = 零命中
		key     string
		seqText string
	}{
		{"括号序号零填充", formBracketKey + "_[01]", "括号序号", formBracketKey, "01"},
		{"括号序号无下划线", "同族画集[12]", "括号序号", "同族画集", "12"},
		{"中段序号", formMidTitle + "_page_10_by_jake__jaccobus_deq5g43", "中段序号", formMidTitle, "10"},
		{"尾缀数字", "00133_73989691_p2_1", "尾缀数字", "00133_73989691_p2", "1"},
		// 尾缀数字：键与序号间的分隔 run 吞并中段的 `...` 与 `_`（惰性键止于
		// 首个可成分隔段的切分点）。
		{"尾缀数字多分隔符", "[TsukijiTums]Finished 8$..._01", "尾缀数字", "[TsukijiTums]Finished 8$", "01"},
		{"直连序号", formDirectKey + "0", "直连序号", formDirectKey, "0"},
		{"直连序号两位", formDirectKey + "11", "直连序号", formDirectKey, "11"},

		// 优先级：同一文件命中多形态时取表序先者。
		{"中段先于直连", formMidTitle + "_page_10_by_jake__jaccobus_deq5g43", "中段序号", formMidTitle, "10"},
		{"中段先于尾缀", "a_page_3_by_b_02", "中段序号", "a", "3"},
		{"尾缀先于直连", "work_01", "尾缀数字", "work", "01"},

		// 排除：浏览器重名冲突的圆括号序号（末字符 `)`）零命中。
		{"圆括号序号", "(15)", "", "", ""},
		{"圆括号序号二", "(38)", "", "", ""},
		// 排除：纯数字名与尾部数字流超 4 位（直连键式要求前缀以非数字结尾）。
		{"纯数字名", "12345", "", "", ""},
		{"超长尾数流", "abc12345", "", "", ""},
		// 页级流水号形态命中直连序号但键含流水号：不同流水号键不同，天然不成群
		//（页级定群由内置规则负责）。
		{"页级流水号一", "00001_72019841_p0", "直连序号", "00001_72019841_p", "0"},
		{"页级流水号二", "00002_72019841_p1", "直连序号", "00002_72019841_p", "1"},
	}
	for _, tc := range cases {
		key, seqText, form := matchStructuralForm(tc.base)
		if tc.form == "" {
			if form != nil {
				t.Fatalf("%s：不应命中形态，实际 %s（key=%q seq=%q）", tc.name, form.name, key, seqText)
			}
			continue
		}
		if form == nil {
			t.Fatalf("%s：应命中形态 %s，实际零命中", tc.name, tc.form)
		}
		if form.name != tc.form || key != tc.key || seqText != tc.seqText {
			t.Fatalf("%s：提取不符 form=%s key=%q seq=%q，期望 form=%s key=%q seq=%q",
				tc.name, form.name, key, seqText, tc.form, tc.key, tc.seqText)
		}
	}
}

// TestStructuralFormGroups 结构形态表驱动的结构群聚合：乙/丙/丁/戊四形态样本
// 各自成群（群键、序号原文零填充、群携带形态标识），排除样本零成群。
func TestStructuralFormGroups(t *testing.T) {
	plain := func(relPath, dirKey string) SnapFile {
		return SnapFile{RelPath: relPath, FileName: fileNameOfRel(relPath), DirRelKey: dirKey, State: snapStatePlain}
	}
	snap := &RecognitionSnapshot{Files: []SnapFile{
		// 乙 括号序号：4 成员，零填充序号原文。
		plain("乙/[くろニャン＠仕事募集中]_[100423498]_C100新刊告知_[01].jpg", "乙"),
		plain("乙/[くろニャン＠仕事募集中]_[100423498]_C100新刊告知_[02].jpg", "乙"),
		plain("乙/[くろニャン＠仕事募集中]_[100423498]_C100新刊告知_[03].jpg", "乙"),
		plain("乙/[くろニャン＠仕事募集中]_[100423498]_C100新刊告知_[04].jpg", "乙"),
		// 丙 直连序号：`]` 后直连数字，无分隔符。
		plain("丙/"+formDirectKey+"0.jpg", "丙"),
		plain("丙/"+formDirectKey+"1.jpg", "丙"),
		plain("丙/"+formDirectKey+"11.jpg", "丙"),
		// 丁 中段序号：尾部随机码各不相同，键取 page 前标题（若误取直连/尾缀
		// 形态，随机码入键即散群，成群本身即中段形态胜出的证明）。
		plain("丁/industrial_becca_page_10_by_jake__jaccobus_deq5g43.jpg", "丁"),
		plain("丁/industrial_becca_page_11_by_jake__jaccobus_d7abc12.jpg", "丁"),
		plain("丁/industrial_becca_page_16_by_jake__jaccobus_d9xyz98.jpg", "丁"),
		// 戊 尾缀数字：用户手工加尾缀的副本对成群（语义上是同图手工副本，
		// 成群可接受的既定边界）。
		plain("戊/[TsukijiTums]Finished 8$..._01.jpg", "戊"),
		plain("戊/[TsukijiTums]Finished 8$..._02.jpg", "戊"),
		plain("戊/00133_73989691_p2_1.jpg", "戊"),
		plain("戊/00133_73989691_p2_2.jpg", "戊"),

		// 排除：浏览器重名冲突的圆括号序号（末字符 `)` 零命中）。
		plain("排1/(15).jpg", "排1"),
		plain("排1/(38).jpg", "排1"),
		// 排除：DeviantArt 散图（无 `_page_N_` 中段序号，尾部随机码各不相同
		// → 直连形态键各异，各自落单）。
		plain("排2/landscape_by_jake__jaccobus_d8xq2v1.jpg", "排2"),
		plain("排2/portrait_by_jake__jaccobus_d7hk3m9.jpg", "排2"),
		// 排除：页级流水号（直连形态键含流水号 00001/00002 各异 → 天然不群；
		// 页级成群由内置规则负责）。
		plain("排3/00001_72019841_p0.jpg", "排3"),
		plain("排3/00002_72019841_p1.jpg", "排3"),

		// 直连形态的已知误群边界：无分隔符直连数字不做防误群守卫，`abc1` 与
		// `abc12` 同键 abc 成一群；误群由用户在解析面板修正处理。
		plain("边/abc1.jpg", "边"),
		plain("边/abc12.jpg", "边"),
	}}
	detectStructuralGroups(snap)

	groupByKey := make(map[string]*SnapGroup, len(snap.Groups))
	for i := range snap.Groups {
		groupByKey[snap.Groups[i].DirRelKey+"\x00"+snap.Groups[i].Key] = &snap.Groups[i]
	}
	mustGroup := func(dirKey, key string) *SnapGroup {
		g := groupByKey[dirKey+"\x00"+key]
		if g == nil {
			t.Fatalf("目录 %s 缺少键为 %q 的结构群（现有群: %+v）", dirKey, key, snap.Groups)
		}
		return g
	}

	// 乙：键=剥尾 `_[NN]` 的全前缀，序号原文保留零填充，形态标识=括号序号。
	bracket := mustGroup("乙", formBracketKey)
	if bracket.Size != 4 || bracket.RuleID != "" || bracket.FormName != "括号序号" ||
		bracket.FormKeyExpr != `^(.+?)_?\[\d{1,4}\]$` || bracket.FormSeqExpr != `\[(\d{1,4})\]$` {
		t.Fatalf("乙形态群不符：%+v", bracket)
	}
	if want := []string{"01", "02", "03", "04"}; len(bracket.Members) != 4 {
		t.Fatalf("乙形态群成员数 = %d，期望 4", len(bracket.Members))
	} else {
		for i, m := range bracket.Members {
			if m.SeqText != want[i] || m.Seq != i+1 {
				t.Fatalf("乙形态群成员 %d 序号不符：%+v（期望原文 %s）", i, m, want[i])
			}
		}
	}

	// 丙：键=直连数字前的全前缀，形态标识=直连序号。
	direct := mustGroup("丙", formDirectKey)
	if direct.Size != 3 || direct.FormName != "直连序号" ||
		direct.FormKeyExpr != `^(.+?[^\d])\d{1,4}$` || direct.FormSeqExpr != `(\d{1,4})$` {
		t.Fatalf("丙形态群不符：%+v", direct)
	}
	if len(direct.Members) != 3 || direct.Members[0].SeqText != "0" || direct.Members[2].SeqText != "11" {
		t.Fatalf("丙形态群成员序号不符：%+v", direct.Members)
	}

	// 丁：键=page 前标题，形态标识=中段序号（成员按集内序排列）。
	mid := mustGroup("丁", formMidTitle)
	if mid.Size != 3 || mid.FormName != "中段序号" ||
		mid.FormKeyExpr != `^(.+?)_page_` || mid.FormSeqExpr != `_page_(\d{1,4})_by_` {
		t.Fatalf("丁形态群不符：%+v", mid)
	}
	if len(mid.Members) != 3 || mid.Members[0].SeqText != "10" || mid.Members[1].SeqText != "11" || mid.Members[2].SeqText != "16" {
		t.Fatalf("丁形态群成员序号不符：%+v", mid.Members)
	}

	// 戊：两对手工副本各自成群，形态标识=尾缀数字（分隔 run 吞并 `..._`，键
	// 止于首个可成分隔段的切分点）。
	suffixSep := mustGroup("戊", "[TsukijiTums]Finished 8$")
	if suffixSep.Size != 2 || suffixSep.FormName != "尾缀数字" ||
		suffixSep.FormKeyExpr != `^(.+?)[ _\-·.．]+\d{1,4}$` || suffixSep.FormSeqExpr != `(\d{1,4})$` {
		t.Fatalf("戊形态群（手工副本）不符：%+v", suffixSep)
	}
	if len(suffixSep.Members) != 2 || suffixSep.Members[0].SeqText != "01" || suffixSep.Members[1].SeqText != "02" {
		t.Fatalf("戊形态群成员序号不符：%+v", suffixSep.Members)
	}
	if dup := mustGroup("戊", "00133_73989691_p2"); dup.Size != 2 || dup.FormName != "尾缀数字" {
		t.Fatalf("戊形态群（页级手工副本对）不符：%+v", dup)
	}

	// 边界：直连形态把 `abc1`/`abc12` 聚为同键 abc 的一群（已知误群边界，
	// 由用户在解析面板处理）。
	abc := mustGroup("边", "abc")
	if abc.Size != 2 || abc.FormName != "直连序号" {
		t.Fatalf("直连误群边界不符：%+v", abc)
	}

	// 排除样本零成群。
	if got := len(snap.Groups); got != 6 {
		t.Fatalf("结构群总数 = %d，期望 6（乙/丙/丁/戊×2/边界；排除目录零成群）：%+v", got, snap.Groups)
	}
	byPath := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	for _, rel := range []string{
		"排1/(15).jpg", "排1/(38).jpg",
		"排2/landscape_by_jake__jaccobus_d8xq2v1.jpg", "排2/portrait_by_jake__jaccobus_d7hk3m9.jpg",
		"排3/00001_72019841_p0.jpg", "排3/00002_72019841_p1.jpg",
	} {
		if p := byPath[rel]; p.GroupKey != "" || p.State != snapStatePlain {
			t.Fatalf("排除样本 %s 不应入群或升格：%+v", rel, p)
		}
	}

	// 成员侧效应：群组成员带「群组前缀=集名候选」线索并升格有线索态。
	member := byPath["乙/"+formBracketKey+"_[01].jpg"]
	if member.State != snapStateClueful || member.GroupKey != formBracketKey || member.GroupSeq != 1 {
		t.Fatalf("群组成员归属/升格不符：%+v", member)
	}
	if len(member.Clues) != 1 || member.Clues[0].Kind != clueGroupPrefix || member.Clues[0].Detail != formBracketKey {
		t.Fatalf("群组成员集名候选线索不符：%+v", member.Clues)
	}

	// 成员区间：结构群成员携带序列块区间（提取式单一事实源，乙=括号序号三块）
	if g := mustGroup("乙", formBracketKey); len(g.Members) == 0 {
		t.Fatal("乙群成员缺席")
	} else {
		segs := g.Members[0].Segs
		if len(segs) != 3 || segs[0].Kind != "author" || segs[1].Kind != "workId" || segs[2].Kind != "seq" {
			t.Fatalf("乙群成员区间不符（作者/作品号/序号三块）：%+v", segs)
		}
	}
}

// —— 作品集识别强化 批次三阶段5：序列块区间 / 目录候选 / A 档库内集识别 / 候选动作 ——

// TestSegmentsOf 序列块区间提取：各形态命中时作者括号块/作品号块/序号块的
// 区间与类别、无块形态只出序号块、未命中形态零区间；保守性锚点（首括号纯
// 数字不回溯找作者块、5 位流水号不作作品号、区间文本即块原文）。
func TestSegmentsOf(t *testing.T) {
	formOf := func(name string) *structuralForm {
		for _, f := range structuralForms {
			if f.name == name {
				return f
			}
		}
		t.Fatalf("形态 %q 不在形态表", name)
		return nil
	}
	assertSegs := func(base string, segs []SnapSeg, want ...[2]string) {
		t.Helper()
		if len(segs) != len(want) {
			t.Fatalf("%s 的块数 = %d，期望 %d：%+v", base, len(segs), len(want), segs)
		}
		for i, seg := range segs {
			if seg.Kind != want[i][0] || base[seg.Start:seg.End] != want[i][1] {
				t.Fatalf("%s 第 %d 块不符：%q[%d:%d]=%q kind=%s，期望 {kind=%s,text=%s}",
					base, i, base, seg.Start, seg.End, base[seg.Start:seg.End], seg.Kind, want[i][0], want[i][1])
			}
		}
	}
	// 括号序号：作者括号块 + 作品号块 + 序号块
	bracket := formBracketKey + "_[01]"
	assertSegs(bracket, segmentsOf(formOf("括号序号"), bracket),
		[2]string{"author", "[くろニャン＠仕事募集中]"},
		[2]string{"workId", "100423498"},
		[2]string{"seq", "01"})
	// 全角作者括号 + 无作品号 → 只出作者块与序号块
	fwBase := "【作者乙】画集_[03]"
	assertSegs(fwBase, segmentsOf(formOf("括号序号"), fwBase),
		[2]string{"author", "【作者乙】"},
		[2]string{"seq", "03"})
	// 直连序号：作者括号 + 作品号 + 序号
	direct := formDirectKey + "0"
	assertSegs(direct, segmentsOf(formOf("直连序号"), direct),
		[2]string{"author", "[Isel]"},
		[2]string{"workId", "92242473"},
		[2]string{"seq", "0"})
	// 尾缀数字：5 位流水号不作作品号、8 位页作品号命中、尾缀序号成块
	tail := "00133_73989691_p2_1"
	assertSegs(tail, segmentsOf(formOf("尾缀数字"), tail),
		[2]string{"workId", "73989691"},
		[2]string{"seq", "1"})
	// 中段序号：无作者括号与作品号 → 只出序号块
	mid := formMidTitle + "_page_10_by_jake__jaccobus_deq5g43"
	assertSegs(mid, segmentsOf(formOf("中段序号"), mid),
		[2]string{"seq", "10"})
	// 首括号纯数字（作品号括号形态）：不向后找作者块（保守）
	digitFirst := "[100423498]_标题_[02]"
	assertSegs(digitFirst, segmentsOf(formOf("括号序号"), digitFirst),
		[2]string{"workId", "100423498"},
		[2]string{"seq", "02"})
	// 形态不命中 / nil 形态 → 零区间
	if segs := segmentsOf(formOf("括号序号"), "(15)"); len(segs) != 0 {
		t.Fatalf("形态不命中应零区间：%+v", segs)
	}
	if segs := segmentsOf(nil, "任意名"); len(segs) != 0 {
		t.Fatalf("nil 形态应零区间：%+v", segs)
	}
}

// TestDirCollectionCandidate 目录候选探测：目录名含作品号命中、单一结构群键
// 目录命中（理由「已成一集」）、剥壳目录名取内名为候选键、无信号与单文件目录
// 不出、规则定群覆盖目录不重复出（群来源已有卡）、成员=目录直接子文件全部且
// 区间现算。
func TestDirCollectionCandidate(t *testing.T) {
	plainFile := func(relPath, dirKey, name string) SnapFile {
		return SnapFile{RelPath: relPath, FileName: name, DirRelKey: dirKey, State: snapStatePlain}
	}
	snap := &RecognitionSnapshot{
		Dirs: []SnapDir{
			{RelKey: "", Name: "根", Depth: 0},
			{RelKey: "100423498", Name: "100423498", Depth: 1},
			{RelKey: "Artist[100423501]", Name: "Artist[100423501]", Depth: 1},
			{RelKey: "无信号目录", Name: "无信号目录", Depth: 1},
			{RelKey: "单文件目录", Name: "100423499", Depth: 1},
			{RelKey: "规则群目录", Name: "100423500", Depth: 1},
			{RelKey: "结构群目录", Name: "结构群目录", Depth: 1},
		},
		Files: []SnapFile{
			// 作品号目录：4 个散文件（无群归属）→ 命中
			plainFile("100423498/a.jpg", "100423498", "a.jpg"),
			plainFile("100423498/b_01.jpg", "100423498", "b_01.jpg"),
			plainFile("100423498/c_02.jpg", "100423498", "c_02.jpg"),
			plainFile("100423498/d.jpg", "100423498", "d.jpg"),
			// 剥壳目录名含作品号：候选键取内名
			plainFile("Artist[100423501]/x.jpg", "Artist[100423501]", "x.jpg"),
			plainFile("Artist[100423501]/y.jpg", "Artist[100423501]", "y.jpg"),
			// 无信号目录：不出
			plainFile("无信号目录/x.jpg", "无信号目录", "x.jpg"),
			plainFile("无信号目录/y.jpg", "无信号目录", "y.jpg"),
			// 直接子文件 <2：不出
			plainFile("单文件目录/only.jpg", "单文件目录", "only.jpg"),
			// 规则定群覆盖：不重复出（群来源已有卡）
			{RelPath: "规则群目录/g_1.jpg", FileName: "g_1.jpg", DirRelKey: "规则群目录", State: snapStatePlain, GroupKey: "g", GroupSeq: 1},
			{RelPath: "规则群目录/g_2.jpg", FileName: "g_2.jpg", DirRelKey: "规则群目录", State: snapStatePlain, GroupKey: "g", GroupSeq: 2},
			// 单一结构群键成集：出目录卡
			{RelPath: "结构群目录/散集 01.jpg", FileName: "散集 01.jpg", DirRelKey: "结构群目录", State: snapStatePlain, GroupKey: "散集", GroupSeq: 1},
			{RelPath: "结构群目录/散集 02.jpg", FileName: "散集 02.jpg", DirRelKey: "结构群目录", State: snapStatePlain, GroupKey: "散集", GroupSeq: 2},
		},
		Groups: []SnapGroup{
			{DirRelKey: "规则群目录", Key: "g", Size: 2, RuleID: "test-sibling-set"},
			{DirRelKey: "结构群目录", Key: "散集", Size: 2, FormName: "尾缀数字"},
		},
	}
	cands := detectDirCandidates(snap)
	if len(cands) != 3 {
		t.Fatalf("目录候选 = %d，期望 3（作品号目录/剥壳目录/单一结构群目录）：%+v", len(cands), cands)
	}
	byDir := map[string]SnapCandidate{}
	for _, c := range cands {
		byDir[c.Dir] = c
	}
	// 作品号目录：理由含作品号与文件数；成员=直接子文件全部、区间现算
	c1 := byDir["100423498"]
	if c1.Source != candSourceDir || c1.Key != "100423498" || c1.Size != 4 ||
		!strings.Contains(c1.Reason, "目录名含作品号 100423498") || !strings.Contains(c1.Reason, "4 个文件") {
		t.Fatalf("作品号目录候选不符: %+v", c1)
	}
	if len(c1.Members) != 4 {
		t.Fatalf("成员应=目录直接子文件全部，实际: %+v", c1.Members)
	}
	seqText, hasSeqSeg := "", false
	for _, mem := range c1.Members {
		if mem.RelPath != "100423498/b_01.jpg" {
			continue
		}
		seqText = mem.SeqText
		for _, s := range mem.Segs {
			if s.Kind == "seq" {
				hasSeqSeg = true
			}
		}
	}
	if seqText != "01" || !hasSeqSeg {
		t.Fatalf("目录候选成员区间应现算（b_01 序号 01），实际 seqText=%q hasSeqSeg=%v", seqText, hasSeqSeg)
	}
	// 剥壳目录：候选键取内名
	if c2 := byDir["Artist[100423501]"]; c2.Key != "100423501" || c2.Size != 2 {
		t.Fatalf("剥壳目录候选键应取内名: %+v", c2)
	}
	// 单一结构群键目录：理由「已成一集」
	if c3 := byDir["结构群目录"]; c3.Key != "结构群目录" || !strings.Contains(c3.Reason, "2 个文件已成一集") {
		t.Fatalf("单一结构群键目录候选不符: %+v", c3)
	}
}

// TestLibSetCandidate A 档库内集识别：候选键与库内集名全等 → 互证；候选键作品
// 号与集名内作品号唯一相等 → 既有集行 id+集名；零命中/多命中/键无作品号 →
// 留空；索引缺席零命中；注记经 buildWorkSetCandidates 落卡（群候选成员含区间）。
func TestLibSetCandidate(t *testing.T) {
	lib := &LibraryIndex{
		WorkSetNames: map[string]struct{}{
			"既有画集": {}, "100423498 C100新刊告知": {}, "100423498 别名": {},
		},
		WorkSetIDs: map[string]int64{"既有画集": 9, "100423498 C100新刊告知": 57, "100423498 别名": 88},
	}
	// 集名全等（键无作品号）→ 只出互证
	cross, id, name := matchExistingWorkSet("既有画集", lib)
	if !cross || id != 0 || name != "" {
		t.Fatalf("集名全等应只出互证: cross=%v id=%d name=%q", cross, id, name)
	}
	// 作品号零命中 → 留空
	if cross, id, name = matchExistingWorkSet("[作者]_[100423499]_标题", lib); cross || id != 0 || name != "" {
		t.Fatalf("作品号零命中应留空: cross=%v id=%d name=%q", cross, id, name)
	}
	// 作品号多命中（两集名同作品号）→ 留空
	if cross, id, name = matchExistingWorkSet("[作者]_[100423498]_标题", lib); cross || id != 0 || name != "" {
		t.Fatalf("作品号多命中应留空: cross=%v id=%d name=%q", cross, id, name)
	}
	// 作品号唯一命中 → 既有集行 id + 集名
	unique := &LibraryIndex{
		WorkSetNames: map[string]struct{}{"100423498 C100新刊告知": {}, "无关集": {}},
		WorkSetIDs:   map[string]int64{"100423498 C100新刊告知": 57},
	}
	if cross, id, name = matchExistingWorkSet("[作者]_[100423498]_标题", unique); cross || id != 57 || name != "100423498 C100新刊告知" {
		t.Fatalf("作品号唯一命中应出既有集: cross=%v id=%d name=%q", cross, id, name)
	}
	// 索引缺席 → 零命中
	if cross, id, name = matchExistingWorkSet("既有画集", nil); cross || id != 0 || name != "" {
		t.Fatalf("索引缺席应零命中: cross=%v id=%d name=%q", cross, id, name)
	}

	// 落卡：群候选键命中库内集名 → CrossMatch、成员（含区间）随卡携带
	snap := &RecognitionSnapshot{
		Dirs: []SnapDir{{RelKey: "", Name: "根", Depth: 0}, {RelKey: "序列", Name: "序列", Depth: 1}},
		Groups: []SnapGroup{{DirRelKey: "序列", Key: "既有画集", Size: 2, Members: []SnapGroupMember{
			{RelPath: "序列/a_01.jpg", Seq: 1, SeqText: "01", Segs: []SnapSeg{{Start: 2, End: 4, Kind: "seq"}}},
			{RelPath: "序列/a_02.jpg", Seq: 2, SeqText: "02"},
		}}},
	}
	cands := buildWorkSetCandidates(snap, lib)
	if len(cands) != 1 {
		t.Fatalf("群候选应出 1 卡，实际 %d", len(cands))
	}
	got := cands[0]
	if !got.CrossMatch || got.Source != candSourceGroup || got.Key != "既有画集" || got.Size != 2 {
		t.Fatalf("群候选注记不符: %+v", got)
	}
	if len(got.Members) != 2 || len(got.Members[0].Segs) != 1 {
		t.Fatalf("群候选成员应复用群成员（含区间）: %+v", got.Members)
	}
}

// TestWorkSetCandidateActions 候选动作三态：忽略 → 移出主清单进 ignored 分仓；
// 恢复 → 回主清单；确认（带既有集行 id）→ 成员逐文件写集维度赋值、终选声明
// 携带 collection、进 confirmed 分仓（带确认集名与既有 id）；已确认候选再确认
// 回可读错误。候选动作经识别刷新调度异步落快照，测试内等在途识别清零后断言
//（确定性锚点：StartRecognition 返回前同步置在途标记）。
func TestWorkSetCandidateActions(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	snapshotOf := func() *RecognitionSnapshot {
		t.Helper()
		sess, err := m.SessionOf(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		if sess.Snapshot == nil {
			t.Fatal("快照缺席")
		}
		return sess.Snapshot
	}
	// 样例树唯一候选：序列目录的规则群（键=库内作者名，A 档互证命中）
	snap := snapshotOf()
	if len(snap.Candidates) != 1 {
		t.Fatalf("候选主清单应恰 1 条，实际: %+v", snap.Candidates)
	}
	target, members := snap.Candidates[0].target(), snap.Candidates[0].Members
	if target != "group\x00序列\x00库内作者名" || !snap.Candidates[0].CrossMatch {
		t.Fatalf("候选目标键/注记不符: %q %+v", target, snap.Candidates[0])
	}
	if snap.IgnoredCandidates != nil || snap.ConfirmedCandidates != nil {
		t.Fatal("初始态分仓清单应为空")
	}

	// 候选动作触发的识别刷新在途标记清零（同步锚点后轮询）
	waitIdle := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			m.recogMu.Lock()
			idle := !m.recogRunning[root]
			m.recogMu.Unlock()
			if idle {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("候选动作触发的识别刷新超时未完成")
			}
			time.Sleep(2 * time.Millisecond)
		}
	}

	// 忽略 → 分仓（分仓卡携带原候选成员）
	if _, err := m.SetCandidateIgnored(root, target, true); err != nil {
		t.Fatalf("忽略候选失败: %v", err)
	}
	waitIdle()
	snap = snapshotOf()
	if len(snap.Candidates) != 0 || len(snap.IgnoredCandidates) != 1 {
		t.Fatalf("忽略后应移出主清单进分仓: main=%d ignored=%d", len(snap.Candidates), len(snap.IgnoredCandidates))
	}
	if snap.IgnoredCandidates[0].target() != target || len(snap.IgnoredCandidates[0].Members) != len(members) {
		t.Fatalf("分仓卡应携带原候选成员: %+v", snap.IgnoredCandidates[0])
	}

	// 恢复 → 回主清单
	if _, err := m.SetCandidateIgnored(root, target, false); err != nil {
		t.Fatalf("恢复候选失败: %v", err)
	}
	waitIdle()
	snap = snapshotOf()
	if len(snap.Candidates) != 1 || len(snap.IgnoredCandidates) != 0 {
		t.Fatalf("恢复后应回主清单: main=%d ignored=%d", len(snap.Candidates), len(snap.IgnoredCandidates))
	}

	// 确认（带既有集行 id）→ 成员赋值 + confirmed 分仓
	sess, affected, err := m.ConfirmWorkSetCandidate(root, target, "确认集名", 57)
	if err != nil || affected != len(members) {
		t.Fatalf("确认候选不符: %v %d", err, affected)
	}
	for _, mem := range members {
		if a := sess.Assignments[mem.RelPath]; a.Collection != "确认集名" {
			t.Fatalf("成员 %s 的集维度赋值未生效: %+v", mem.RelPath, a)
		}
	}
	if id := sess.ConfirmedSetIDs["确认集名"]; id != 57 {
		t.Fatalf("既有集行 id 旁路簿记不符: %d", id)
	}
	waitIdle()
	snap = snapshotOf()
	if len(snap.Candidates) != 0 || len(snap.ConfirmedCandidates) != 1 || len(snap.IgnoredCandidates) != 0 {
		t.Fatalf("确认后应进 confirmed 分仓: main=%d confirmed=%d ignored=%d",
			len(snap.Candidates), len(snap.ConfirmedCandidates), len(snap.IgnoredCandidates))
	}
	confirmed := snap.ConfirmedCandidates[0]
	if confirmed.ConfirmedSetName != "确认集名" || confirmed.ExistingSetID != 57 || !confirmed.CrossMatch {
		t.Fatalf("confirmed 卡应带确认集名与既有集行 id: %+v", confirmed)
	}
	// 成员终选声明含 collection=确认集名（赋值覆盖档案结论）且全员已识别
	byPath := map[string]SnapFile{}
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	found := false
	for _, mem := range members {
		f := byPath[mem.RelPath]
		if f.State != snapStateIdentified || !f.Assigned {
			t.Fatalf("确认后成员应已识别并带赋值标记: %+v", f)
		}
		for _, d := range f.Final {
			if d.Dimension == string(DimCollection) && d.Value == "确认集名" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("成员终选声明未携带确认集名的 collection 声明")
	}

	// 已确认候选不在主清单：再确认回可读错误
	if _, _, err := m.ConfirmWorkSetCandidate(root, target, "再确认", 0); err == nil {
		t.Fatal("已确认候选再确认应回可读错误")
	}
}
