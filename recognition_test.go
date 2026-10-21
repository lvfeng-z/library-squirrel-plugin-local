// 全量识别引擎的单元测试：七类元素聚合（构造目录树：已识别/未识别有线索/
// 序列成群落单/待裁决各形态齐备）、结构形态表的形态判定与结构群聚合、
// 会话赋值按维度覆盖、谓词索引注入求值与分页拉全、死路径赋值报告、快照
// 往返、目录 mtime 增量、以及前端协议的 startRecognition / getPanorama 分派。
package main

import (
	"encoding/json"
	"fmt"
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
			Pattern:     conflictA,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c1"}},
		},
		{
			ID: "test-conflict-b", Description: "测试：标题冲突 B", Source: RuleSourceManual, Enabled: true,
			Pattern:     conflictB,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c2"}},
		},
		{
			ID: "test-predicate-author", Description: "测试：文件名命中库内作者", Source: RuleSourceManual, Enabled: true,
			Pattern:     &PredicatePattern{Kind: PredicateAuthor},
			Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromPredicate}},
		},
		{
			ID: "test-dir-clue-tag", Description: "测试：线索层目录=标签", Source: RuleSourceManual, Enabled: true,
			Pattern:     dirPattern,
			Extractions: []ExtractionAction{{Dimension: DimTag, From: FromAncestorDir}},
		},
	}
}

// newRecognitionFixture 构造识别干跑测试环境：内存存储 + 桩上下文 + 测试档案
// （含禁用规则一条，供元素6 启用态呈现）+ 指定库根会话与赋值。
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
// 「库内作者名」命中库内索引，验证群组前缀侧的谓词线索）：
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
// TestElementAlign 元素对齐判定（批次二 F3）：任意位序号泛化正例（第N话/
// vol_N/industrial_N_becca/多数字位决策1 A）与反例（圆括号/纯数字/超长尾数流/
// 日期/随机码/同值序号/页级流水号）；`_?[NN]` 分隔容忍（带与不带 `_` 的同族
// 文件归同键的既有语义）。判定层退役后 matchStructuralForm 的表驱动用例由
// 本用例以群级语义承接。
func TestElementAlign(t *testing.T) {
	plain := func(relPath, dirKey string) SnapFile {
		return SnapFile{RelPath: relPath, FileName: fileNameOfRel(relPath), DirRelKey: dirKey, State: snapStatePlain}
	}
	run := func(files ...SnapFile) []SnapGroup {
		snap := &RecognitionSnapshot{Files: files}
		detectStructuralGroups(snap)
		return snap.Groups
	}

	// 正例：括号序号同族归键（`_?[NN]` 分隔容忍——带与不带 `_` 归同键）。
	g := run(plain("fam/同族画集_[01].jpg", "fam"), plain("fam/同族画集[12].jpg", "fam"))
	if len(g) != 1 || g[0].Key != "同族画集" || g[0].FormName != "括号序号" ||
		g[0].Members[0].SeqText != "01" || g[0].Members[1].SeqText != "12" {
		t.Fatalf("括号序号同族归键不符：%+v", g)
	}
	// 正例：第N话（词内直连数字的任意位序号，现状不识别的泛化目标；形态提取
	// 式不命中 → 键退公共元素拼接兜底）。
	g = run(plain("gen1/画集_第1话.jpg", "gen1"), plain("gen1/画集_第2话.jpg", "gen1"), plain("gen1/画集_第10话.jpg", "gen1"))
	if len(g) != 1 || g[0].FormName != "中段序号" || g[0].Key != "画集_第话" ||
		g[0].Members[0].Seq != 1 || g[0].Members[1].Seq != 2 || g[0].Members[2].Seq != 10 {
		t.Fatalf("第N话泛化成群不符：%+v", g)
	}
	// 正例：vol_N 尾缀（形态键提取式复原键）。
	g = run(plain("gen2/标题_vol_1.jpg", "gen2"), plain("gen2/标题_vol_2.jpg", "gen2"))
	if len(g) != 1 || g[0].FormName != "尾缀数字" || g[0].Key != "标题_vol" {
		t.Fatalf("vol_N 成群不符：%+v", g)
	}
	// 正例：industrial_N_becca（中段、键=公共元素拼接兜底）。
	g = run(plain("gen3/industrial_1_becca.jpg", "gen3"), plain("gen3/industrial_2_becca.jpg", "gen3"), plain("gen3/industrial_12_becca.jpg", "gen3"))
	if len(g) != 1 || g[0].FormName != "中段序号" || g[0].Key != "industrial_becca" ||
		g[0].Members[0].SeqText != "1" || g[0].Members[2].SeqText != "12" {
		t.Fatalf("industrial_N_becca 成群不符：%+v", g)
	}
	// 正例（决策1 A，2026-10-20 拍板）：多数字位并存取互异度最大位为序号、
	// 共同数字位入键（B 案下多数字位即不成群——本例锚定 A 案覆盖面）。
	g = run(plain("gen4/第1话_p7.jpg", "gen4"), plain("gen4/第2话_p7.jpg", "gen4"), plain("gen4/第3话_p7.jpg", "gen4"))
	if len(g) != 1 || g[0].Key != "第话_p7" ||
		g[0].Members[0].Seq != 1 || g[0].Members[2].Seq != 3 {
		t.Fatalf("决策1 A 多数字位成群不符：%+v", g)
	}

	// 反例：序号候选缺席或键互异的各种形态零成群。
	for name, files := range map[string][]SnapFile{
		"圆括号序号（浏览器重名后缀）": {plain("p/(15).jpg", "p"), plain("p/(38).jpg", "p")},
		"纯数字名":          {plain("p/12345.jpg", "p"), plain("p/12346.jpg", "p")},
		"尾部数字流超 4 位":    {plain("p/abc12345.jpg", "p"), plain("p/abc12346.jpg", "p")},
		"八位日期":          {plain("p/photo_20231001.jpg", "p"), plain("p/photo_20231002.jpg", "p")},
		"字母随机码":         {plain("p/img_deq5g43.jpg", "p"), plain("p/img_d8xq2v1.jpg", "p")},
		"同值序号（同基名异扩展名）": {plain("p/a_01.jpg", "p"), plain("p/a_01.png", "p")},
		"页级流水号":         {plain("p/00001_72019841_p0.jpg", "p"), plain("p/00002_72019841_p1.jpg", "p")},
	} {
		if g := run(files...); len(g) != 0 {
			t.Fatalf("%s：不应成群，实际 %+v", name, g)
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
		els := g.Members[0].Elements
		kinds := make(map[string]int)
		for _, el := range els {
			if el.Kind != elKindSep {
				kinds[el.Kind]++
			}
		}
		if kinds[elKindAuthor] != 1 || kinds[elKindWorkId] != 1 || kinds[elKindSeq] != 1 || kinds[elKindTitle] != 3 {
			t.Fatalf("乙群成员元素类别不符（作者/作品号/公共×3/序号）：%+v（%v）", els, kinds)
		}
	}
}

// —— 结构群前置对齐与序列成员条件（规则群与结构群统一求值）—— //

// structuralCondRules 序列成员条件测试规则集：集名规则（捕获集名，群组序号
// ≥1 才产出——「仅序列成员产出」形态）与多成员规则（群组规模 ≥2 才产出常量
// 标题）。
func structuralCondRules(t *testing.T) []RuleEntry {
	t.Helper()
	num := func(v float64) *float64 { return &v }
	return []RuleEntry{
		{
			ID: "struct-cond-set", Description: "测试：仅序列成员产出集名", Source: RuleSourceManual, Enabled: true,
			Pattern: mustRegexPattern(t, `^(?P<setname>[^_]+)_(\d{1,4})\.jpg$`, []string{"setname"}),
			Extractions: []ExtractionAction{{
				Dimension: DimCollection, From: FromCapture, CaptureName: "setname",
				When: &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSeq}, Min: num(1)},
			}},
		},
		{
			ID: "struct-cond-size", Description: "测试：多成员才产出标题", Source: RuleSourceManual, Enabled: true,
			Pattern: mustRegexPattern(t, `^(?P<setname>[^_]+)_(\d{1,4})\.jpg$`, []string{"setname"}),
			Extractions: []ExtractionAction{{
				Dimension: DimTitle, From: FromConstant, Constant: "多成员作品",
				When: &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSize}, Min: num(2)},
			}},
		},
	}
}

// structuralCondFiles 序列成员条件测试文件：同目录结构群三成员 + 另一目录的
// 同形落单文件（同一条集名规则的匹配形态，目录内无同形兄弟）。
func structuralCondFiles() []ScanFile {
	return []ScanFile{
		{FileName: "路灯下的她们_01.jpg", DirNames: []string{"路灯"}, RelPath: "路灯/路灯下的她们_01.jpg"},
		{FileName: "路灯下的她们_02.jpg", DirNames: []string{"路灯"}, RelPath: "路灯/路灯下的她们_02.jpg"},
		{FileName: "路灯下的她们_03.jpg", DirNames: []string{"路灯"}, RelPath: "路灯/路灯下的她们_03.jpg"},
		{FileName: "孤行画集_07.jpg", DirNames: []string{"别处"}, RelPath: "别处/孤行画集_07.jpg"},
	}
}

// conclusionByRel 结论按相对路径索引。
func conclusionByRel(conclusions []FileConclusion) map[string]FileConclusion {
	byRel := make(map[string]FileConclusion, len(conclusions))
	for _, c := range conclusions {
		byRel[c.RelPath] = c
	}
	return byRel
}

// declKey 声明的多重集比对键（维度=值|来源）。
func declKey(d MetadataDeclaration) string {
	return string(d.Dimension) + "=" + d.Value + "|" + string(d.Source)
}

// assertSameDeclSet 断言两组声明为同一多重集。
func assertSameDeclSet(t *testing.T, label string, want, got []MetadataDeclaration) {
	t.Helper()
	seen := make(map[string]int, len(want))
	for _, d := range want {
		seen[declKey(d)]++
	}
	for _, d := range got {
		seen[declKey(d)]--
	}
	for k, count := range seen {
		if count != 0 {
			t.Fatalf("%s 声明集不一致（%s 差 %d；期望 %+v，实际 %+v）", label, k, count, want, got)
		}
	}
}

// TestStructuralGroupMemberCondition 结构群前置后的序列成员条件：结构群成员
// 的群组序号/规模操作数可取值（序号≥1 与规模≥2 的条件成立、集名与标题声明
// 产出，结论携带结构群键/序号/规模与序号原文）；同形落单文件的群组操作数取值
// 不可用——含它的区间比较一律为假（两条规则照常命中计数但不产出声明）。
func TestStructuralGroupMemberCondition(t *testing.T) {
	conclusions := MergedRecognition(structuralCondFiles(), structuralCondRules(t), nil, nil)
	byRel := conclusionByRel(conclusions)
	if len(conclusions) != 4 {
		t.Fatalf("结论数 = %d，期望 4", len(conclusions))
	}

	// 结构群成员：群组字段补位注入（无定群规则 ID）、条件成立、声明产出
	for rel, wantSeq := range map[string]int{
		"路灯/路灯下的她们_01.jpg": 1, "路灯/路灯下的她们_02.jpg": 2, "路灯/路灯下的她们_03.jpg": 3,
	} {
		c := byRel[rel]
		if c.Group == nil || c.GroupRuleID != "" || c.Group.Key != "路灯下的她们" ||
			c.Group.Seq != wantSeq || c.Group.Size != 3 || c.GroupSeqText != fmt.Sprintf("%02d", wantSeq) {
			t.Fatalf("%s 结构群补位字段不符：group=%+v seqText=%q", rel, c.Group, c.GroupSeqText)
		}
		var collection, title int
		for _, d := range c.Final {
			if d.Dimension == DimCollection && d.Value == "路灯下的她们" {
				collection++
			}
			if d.Dimension == DimTitle && d.Value == "多成员作品" {
				title++
			}
		}
		if collection != 1 || title != 1 {
			t.Fatalf("%s 序列成员条件应全部成立（集名与标题各一条声明），实际集名 %d 条、标题 %d 条（终选: %+v）", rel, collection, title, c.Final)
		}
	}

	// 同形落单文件：规则命中计数照常，群组操作数不可用 → 条件为假、零声明
	lone := byRel["别处/孤行画集_07.jpg"]
	if lone.Group != nil {
		t.Fatalf("落单文件不应有群组身份：%+v", lone.Group)
	}
	if len(lone.MatchedRuleIDs) != 2 {
		t.Fatalf("落单文件的两条规则应照常命中（计数不受条件影响），实际命中 %+v", lone.MatchedRuleIDs)
	}
	if len(lone.Final) != 0 {
		t.Fatalf("落单文件含群组操作数的比较为假，不应产出任何声明：%+v", lone.Final)
	}
}

// TestGroupFreeRulesUnaffectedByStructuralFill 无群组引用的规则零行为变化：
// 条件与动作均不引用群组操作数/群组前缀来源的规则，在结构群补位注入的合并
// 求值（批量干跑入口）与不含结构群补位的纯流水线（RunPipeline）下对同批
// 文件产出同一声明集——前置对齐只扩大群组引用规则的可表达面，不波及其余。
func TestGroupFreeRulesUnaffectedByStructuralFill(t *testing.T) {
	capture := func(id string, dim MetadataDimension, when *CondExpr) RuleEntry {
		return RuleEntry{
			ID: id, Description: "测试：无群组引用的提取", Source: RuleSourceManual, Enabled: true,
			Pattern: mustRegexPattern(t, `^(?P<setname>[^_]+)_(\d{1,4})\.jpg$`, []string{"setname"}),
			Extractions: []ExtractionAction{{
				Dimension: dim, From: FromCapture, CaptureName: "setname", When: when,
			}},
		}
	}
	// 两条规则分别声明不同维度（无条件标题 + 捕获相等条件的作者），合成层无冲突
	rules := []RuleEntry{
		capture("group-free-uncond", DimTitle, nil),
		capture("group-free-capture-eq", DimAuthor, &CondExpr{Kind: ExprEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "setname"},
			Right: &CondExpr{Kind: ExprString, Str: "路灯下的她们"}}),
	}
	files := structuralCondFiles()
	decisions := RunPipeline(files, rules, nil)
	conclusions := MergedRecognition(files, rules, nil, nil)
	byRel := conclusionByRel(conclusions)
	for _, d := range decisions {
		c, ok := byRel[d.RelPath]
		if !ok {
			t.Fatalf("合并求值缺少 %s 的结论", d.RelPath)
		}
		assertSameDeclSet(t, "文件 "+d.RelPath, d.Declarations, c.Final)
		if fmt.Sprint(c.MatchedRuleIDs) != fmt.Sprint(d.MatchedRuleIDs) {
			t.Fatalf("文件 %s 命中规则不一致：%v vs %v", d.RelPath, d.MatchedRuleIDs, d.MatchedRuleIDs)
		}
		// 条件取值与群组在场与否无关：三份群成员与落单文件的捕获相等条件同判
		want := 1
		if strings.HasPrefix(d.FileName, "路灯下的她们_") {
			want = 2
		}
		if len(c.Final) != want {
			t.Fatalf("文件 %s 声明数 = %d，期望 %d（%+v）", d.RelPath, len(c.Final), want, c.Final)
		}
	}
}

// TestStructuralRecomputeMatchesBatch 单文件重算与批量干跑对同一目录结论一致：
// 重算通道以磁盘重扫的兄弟名单现场跑同一前置对齐，结构群成员身份与条件声明
// 与批量入口（每目录预计算）逐文件等价。
func TestStructuralRecomputeMatchesBatch(t *testing.T) {
	root := t.TempDir()
	rels := []string{
		"路灯/路灯下的她们_01.jpg", "路灯/路灯下的她们_02.jpg", "路灯/路灯下的她们_03.jpg",
		"别处/孤行画集_07.jpg",
	}
	entries := make([]FileEntry, 0, len(rels))
	for _, rel := range rels {
		writeTestFile(t, root+"/"+rel, "x")
		entries = append(entries, FileEntry{FullPath: root + "/" + rel, RelPath: rel})
	}
	rules := structuralCondRules(t)
	lib := BuildLibraryIndex(nil, nil)
	batch := MergedRecognition(buildScanFiles(entries), rules, lib, nil)
	byRel := conclusionByRel(batch)
	h := &LocalImportWorkFetcher{}
	for _, e := range entries {
		fp := &FilePluginData{FullPath: e.FullPath, RelPath: e.RelPath}
		recomputed, _ := h.recomputeFileDecision(fp, rules, lib)
		want := byRel[e.RelPath]
		if want.Group == nil {
			if recomputed.Group != nil {
				t.Fatalf("%s 重算不应有群组身份：%+v", e.RelPath, recomputed.Group)
			}
		} else if recomputed.Group == nil || recomputed.Group.Key != want.Group.Key ||
			recomputed.Group.Seq != want.Group.Seq || recomputed.Group.Size != want.Group.Size ||
			recomputed.GroupSeqText != want.GroupSeqText {
			t.Fatalf("%s 重算结构群身份与干跑不符：%+v/%q vs %+v/%q",
				e.RelPath, recomputed.Group, recomputed.GroupSeqText, want.Group, want.GroupSeqText)
		}
		assertSameDeclSet(t, "文件 "+e.RelPath, want.Final, recomputed.Final)
	}
}

// TestRunFullRecognitionStructuralPresentation 全量识别下结构群的快照呈现：
// 对齐判定前置到求值段后，快照组装消费结论携带的群组信息——群卡（键/规模/
// 序号原文/命中形态标识）、文件行群组键与「群组前缀=集名候选」线索、未识别
// 成员升格有线索态；异形落单文件不入群不升格。
func TestRunFullRecognitionStructuralPresentation(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	// 空格分隔形态：不落测试档案兄弟规则的下划线键形态，由结构群承接
	writeTestFile(t, root+"/路灯/路灯下的她们 01.jpg", "a")
	writeTestFile(t, root+"/路灯/路灯下的她们 02.jpg", "b")
	writeTestFile(t, root+"/路灯/路灯下的她们 03.jpg", "c")
	writeTestFile(t, root+"/路灯/孤本.jpg", "d")

	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, _ := m.SessionOf(root)
	snap := sess.Snapshot
	if len(snap.Groups) != 1 {
		t.Fatalf("序列群数量 = %d，期望 1（groups: %+v）", len(snap.Groups), snap.Groups)
	}
	g := snap.Groups[0]
	if g.DirRelKey != "路灯" || g.Key != "路灯下的她们" || g.Size != 3 || g.RuleID != "" ||
		g.FormName != "尾缀数字" || g.FormKeyExpr != `^(.+?)[ _\-·.．]+\d{1,4}$` || g.FormSeqExpr != `(\d{1,4})$` {
		t.Fatalf("结构群群卡不符: %+v", g)
	}
	if len(g.Members) != 3 {
		t.Fatalf("结构群成员数 = %d，期望 3（%+v）", len(g.Members), g.Members)
	}
	for i, mem := range g.Members {
		if wantSeq := i + 1; mem.Seq != wantSeq || mem.SeqText != fmt.Sprintf("%02d", wantSeq) {
			t.Fatalf("成员 %s 序号不符：%d/%q（期望 %d/%02d）", mem.RelPath, mem.Seq, mem.SeqText, wantSeq, wantSeq)
		}
		if len(mem.Elements) == 0 {
			t.Fatalf("成员 %s 缺少元素数组", mem.RelPath)
		}
	}
	fileByRel := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		fileByRel[f.RelPath] = f
	}
	member := fileByRel["路灯/路灯下的她们 01.jpg"]
	if member.State != snapStateClueful || member.GroupKey != "路灯下的她们" || member.GroupSeq != 1 {
		t.Fatalf("未识别成员文件行呈现不符：%+v", member)
	}
	if len(member.Clues) != 1 || member.Clues[0].Kind != clueGroupPrefix || member.Clues[0].Detail != "路灯下的她们" {
		t.Fatalf("未识别成员线索不符：%+v", member.Clues)
	}
	if lone := fileByRel["路灯/孤本.jpg"]; lone.State != snapStatePlain || lone.GroupKey != "" || len(lone.Clues) != 0 {
		t.Fatalf("异形落单文件不应入群或升格：%+v", lone)
	}
	if snap.Stats.TotalFiles != 4 || snap.Stats.Identified != 0 || snap.Stats.Clueful != 3 || snap.Stats.Plain != 1 || snap.Stats.GroupCount != 1 {
		t.Fatalf("汇总统计不符：%+v", snap.Stats)
	}
}

// —— 元素级内核 批次二：元素拆分基座 / 元素对齐判定 / 目录候选 / A 档库内集识别 / 候选动作 ——

// TestSplitElements 元素拆分基座：分隔 run/括号组双态/已知名保持/词内数字子段
// 与特征级类别推断（author/workId/seq/plain）；拼接还原正文（CJK 量纲根治
// 锚点——阶段0 的 TestSegProbeCJK 由本用例继承：标题括号组与直连尾缀数字都
// 是显式元素，无偏移可丢）。
func TestSplitElements(t *testing.T) {
	assert := func(base string, hints []string, want ...SnapEl) {
		t.Helper()
		got := splitFilenameElements(base, hints)
		var concat strings.Builder
		for i := range got {
			concat.WriteString(got[i].Text)
		}
		if concat.String() != base {
			t.Fatalf("%s：元素拼接不还原正文（%q）：%+v", base, concat.String(), got)
		}
		if len(got) != len(want) {
			t.Fatalf("%s：元素数 = %d，期望 %d：%+v", base, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s：第 %d 元素不符 %+v，期望 %+v", base, i, got[i], want[i])
			}
		}
	}
	// CJK 实测文件名（用户实例）：作者/作品号/标题括号组/直连尾缀数字全显式携带。
	assert("[Isel] [92242473] [瞬間的に絡み合う]0", nil,
		SnapEl{Text: "[Isel]", Kind: "author", Shelled: true, Inner: "Isel"},
		SnapEl{Text: " ", Kind: "sep"},
		SnapEl{Text: "[92242473]", Kind: "workId", Shelled: true, Inner: "92242473"},
		SnapEl{Text: " ", Kind: "sep"},
		SnapEl{Text: "[瞬間的に絡み合う]", Kind: "plain", Shelled: true, Inner: "瞬間的に絡み合う"},
		SnapEl{Text: "0", Kind: "seq"},
	)
	// 词内数字子段（`abc1`→abc+1）与位数判类（5 位流水号 plain、8 位作品号
	// workId——宁缺勿滥口径）。
	assert("abc1", nil, SnapEl{Text: "abc", Kind: "plain"}, SnapEl{Text: "1", Kind: "seq"})
	assert("00133_73989691_p2_1", nil,
		SnapEl{Text: "00133", Kind: "plain"}, SnapEl{Text: "_", Kind: "sep"},
		SnapEl{Text: "73989691", Kind: "workId"}, SnapEl{Text: "_", Kind: "sep"},
		SnapEl{Text: "p", Kind: "plain"}, SnapEl{Text: "2", Kind: "seq"}, SnapEl{Text: "_", Kind: "sep"},
		SnapEl{Text: "1", Kind: "seq"})
	// 括号组保守性：首括号内名纯数字=作品号括号形态（无作者块）；圆括号数字
	// 不参与序号候选（浏览器重名后缀排除语义）。
	assert("[100423498]_标题_[02]", nil,
		SnapEl{Text: "[100423498]", Kind: "workId", Shelled: true, Inner: "100423498"},
		SnapEl{Text: "_", Kind: "sep"}, SnapEl{Text: "标题", Kind: "plain"},
		SnapEl{Text: "_", Kind: "sep"}, SnapEl{Text: "[02]", Kind: "seq", Shelled: true, Inner: "02"})
	assert("(15)", nil, SnapEl{Text: "(15)", Kind: "plain", Shelled: true, Inner: "15"})
	// 已知名保持：目录名提示的跨分隔符区间保持整段（jake__jaccobus 不拆）。
	assert("art_by_jake__jaccobus_d1", []string{"jake--jaccobus"},
		SnapEl{Text: "art", Kind: "plain"}, SnapEl{Text: "_", Kind: "sep"}, SnapEl{Text: "by", Kind: "plain"},
		SnapEl{Text: "_", Kind: "sep"}, SnapEl{Text: "jake__jaccobus", Kind: "plain"},
		SnapEl{Text: "_", Kind: "sep"}, SnapEl{Text: "d", Kind: "plain"}, SnapEl{Text: "1", Kind: "seq"})
	// 句点族分隔：`...` 与 `_` 聚为一个分隔 run（与形态表尾缀数字分隔类对齐）。
	assert("Finished 8$..._01", nil,
		SnapEl{Text: "Finished", Kind: "plain"}, SnapEl{Text: " ", Kind: "sep"},
		SnapEl{Text: "8", Kind: "seq"}, SnapEl{Text: "$", Kind: "plain"},
		SnapEl{Text: "..._", Kind: "sep"}, SnapEl{Text: "01", Kind: "seq"})
}

// （阶段0 的 utf16Slice/TestSegProbeCJK 已随 SnapSeg 区间退役——CJK 量纲
// 锚点由 TestSplitElements 与 TestElementAlign 的元素级断言继承：元素显式
// 携带原文文本，无偏移可丢。）

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
	seqText, hasSeqEl := "", false
	for _, mem := range c1.Members {
		if mem.RelPath != "100423498/b_01.jpg" {
			continue
		}
		seqText = mem.SeqText
		for _, el := range mem.Elements {
			if el.Kind == elKindSeq {
				hasSeqEl = true
			}
		}
	}
	if seqText != "01" || !hasSeqEl {
		t.Fatalf("目录候选成员元素应现算（b_01 序号 01），实际 seqText=%q hasSeqEl=%v", seqText, hasSeqEl)
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
			{RelPath: "序列/a_01.jpg", Seq: 1, SeqText: "01", Elements: []SnapEl{{Text: "a"}, {Text: "_", Kind: "sep"}, {Text: "01", Kind: "seq"}}},
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
	if len(got.Members) != 2 || len(got.Members[0].Elements) != 3 {
		t.Fatalf("群候选成员应复用群成员（含区间）: %+v", got.Members)
	}
}

// TestWorkSetCandidateActions 候选动作三态：忽略 → 移出主清单进 ignored 分仓；
// 恢复 → 回主清单；确认（带既有集行 id）→ 成员逐文件写集维度赋值、终选声明
// 携带 collection、进 confirmed 分仓（带确认集名与既有 id）；已确认候选再确认
// 回可读错误。候选动作经识别刷新调度异步落快照，测试内等在途识别清零后断言
// （确定性锚点：StartRecognition 返回前同步置在途标记）。
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

// TestUnconfirmWorkSetCandidate 撤销归集（剧本2）：确认（带既有集行 id）后撤销——
// 只清成员的集维度赋值（其余维度原样保留、全维度剥空的赋值整条删除），目标移出
// confirmed 清单、既有集旁路簿记一并清理；随自动重识别候选回主清单（分仓重建按
// 会话清单）、成员终选声明不再携带 collection；目标不在已归集清单（会话已刷新/
// 结构已变）时回可读错误、状态不动。撤销动作经识别刷新调度异步落快照，测试内等
// 在途识别清零后断言（确定性锚点：StartRecognition 返回前同步置在途标记）。
func TestUnconfirmWorkSetCandidate(t *testing.T) {
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
				t.Fatal("撤销动作触发的识别刷新超时未完成")
			}
			time.Sleep(2 * time.Millisecond)
		}
	}

	// 样例树唯一候选：序列目录的结构群（键=库内作者名）；先给首成员预置作者维度
	// 赋值（撤销清维语义的保留侧锚点）
	snap := snapshotOf()
	if len(snap.Candidates) != 1 {
		t.Fatalf("候选主清单应恰 1 条，实际: %+v", snap.Candidates)
	}
	target, members := snap.Candidates[0].target(), snap.Candidates[0].Members
	if _, err := m.SetAssignment(root, members[0].RelPath, SessionAssignment{Author: "保留作者"}); err != nil {
		t.Fatalf("预置作者维度赋值失败: %v", err)
	}
	if _, _, err := m.ConfirmWorkSetCandidate(root, target, "确认集名", 57); err != nil {
		t.Fatalf("确认候选失败: %v", err)
	}
	waitIdle()
	snap = snapshotOf()
	if len(snap.Candidates) != 0 || len(snap.ConfirmedCandidates) != 1 {
		t.Fatalf("确认后应进 confirmed 分仓: main=%d confirmed=%d",
			len(snap.Candidates), len(snap.ConfirmedCandidates))
	}
	if snap.ConfirmedCandidates[0].target() != target || len(snap.ConfirmedCandidates[0].Members) != len(members) {
		t.Fatalf("confirmed 分仓卡应携带完整成员: %+v", snap.ConfirmedCandidates[0])
	}

	// 撤销 → 只清集维度、清单移动、旁路簿记清理
	sess, affected, err := m.UnconfirmWorkSetCandidate(root, target)
	if err != nil || affected != len(members) {
		t.Fatalf("撤销归集不符: %v %d", err, affected)
	}
	if pos := slicePos(sess.ConfirmedCandidates, target); pos >= 0 {
		t.Fatalf("目标应移出 confirmed 清单: %+v", sess.ConfirmedCandidates)
	}
	if _, has := sess.ConfirmedSetIDs["确认集名"]; has {
		t.Fatal("既有集行 id 旁路簿记应一并清理")
	}
	for _, mem := range members {
		if assign := sess.Assignments[mem.RelPath]; assign.Collection != "" {
			t.Fatalf("成员 %s 的集维度赋值应清空: %+v", mem.RelPath, assign)
		}
	}
	if a := sess.Assignments[members[0].RelPath]; a.Author != "保留作者" {
		t.Fatalf("撤销只清集维度，其余维度应原样保留: %+v", a)
	}
	if _, has := sess.Assignments[members[1].RelPath]; has {
		t.Fatalf("全维度剥空的成员赋值应整条删除: %+v", sess.Assignments[members[1].RelPath])
	}

	// 随自动重识别：候选回主清单、成员终选声明不再携带 collection
	waitIdle()
	snap = snapshotOf()
	if len(snap.Candidates) != 1 || len(snap.ConfirmedCandidates) != 0 || len(snap.IgnoredCandidates) != 0 {
		t.Fatalf("撤销后候选应回主清单: main=%d confirmed=%d ignored=%d",
			len(snap.Candidates), len(snap.ConfirmedCandidates), len(snap.IgnoredCandidates))
	}
	if snap.Candidates[0].target() != target {
		t.Fatalf("回流候选目标键不符: %q（期望 %q）", snap.Candidates[0].target(), target)
	}
	byPath := map[string]SnapFile{}
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	for _, mem := range members {
		for _, d := range byPath[mem.RelPath].Final {
			if d.Dimension == string(DimCollection) {
				t.Fatalf("撤销重识别后成员 %s 不应再携带 collection 声明: %+v",
					mem.RelPath, byPath[mem.RelPath].Final)
			}
		}
	}

	// 错误路径：目标不在已归集清单（撤销后重识别已完成、分仓已重建）→ 可读错误
	if _, _, err := m.UnconfirmWorkSetCandidate(root, target); err == nil {
		t.Fatal("目标不在已归集清单时撤销应回可读错误")
	}
}
