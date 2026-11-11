// 全量识别引擎的单元测试：七类元素聚合（构造目录树：已识别/未识别有线索/
// 序列成群落单/待裁决各形态齐备）、结构形态表的形态判定与结构群聚合、
// 会话赋值按维度覆盖、谓词索引注入求值与分页拉全、死路径赋值报告、快照
// 往返、目录 mtime 增量、以及前端协议的 startRecognition / getPanorama 分派。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
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
	c.indexBuildSeq.Add(1) // 索引构造观测：首轨必经（名单缓存测试的计数额面）
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
		// 行 id 跨页单调递增（同桩多次拉取也稳定唯一），供 WorkSetIDs 断言；
		// atomic 发号（见 workbenchTestCtx.workSetIDSeq 注释）
		items = append(items, &sdkdto.WorkSetNameEntry{Id: c.workSetIDSeq.Add(1), Name: name})
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
	// 内置退休后的档案等价条目：【作者】标题命名惯例（样例树「已识别」目录
	// 的识别由此承载——v4 文件行模板）。
	bracketRows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "【{作者}】{标题}"},
	}}}
	bracketPattern, err := CompileRuleConditions(bracketRows)
	if err != nil {
		t.Fatalf("构造括号命名惯例模板规则失败: %v", err)
	}
	return []RuleEntry{
		{
			ID: "test-sibling-set", Description: "测试：下划线前缀+序号成群", Source: RuleSourceManual, Enabled: true,
			Pattern: siblingPattern,
		},
		{
			ID: "test-bracket-author-title", Description: "测试：【作者】标题命名惯例", Source: RuleSourceManual, Enabled: true,
			Conditions: bracketRows, Pattern: bracketPattern,
			Extractions: []ExtractionAction{
				{Dimension: DimAuthor, From: FromCapture, CaptureName: condxVocab["作者"], Row: 1},
				{Dimension: DimTitle, From: FromCapture, CaptureName: condxVocab["标题"], Row: 1},
			},
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
	if stat := statByID["test-bracket-author-title"]; !stat.Enabled || stat.HitCount != 1 {
		t.Fatalf("括号命名惯例规则统计不符: %+v", stat)
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

// stampEvaluatorBuild 模拟打标构建：在临时目录落一份带 buildId 的插件清单并
// 把测试桩的插件根指向它（求值器构建标识的读取面走真实清单解析路径）。
func stampEvaluatorBuild(t *testing.T, ctx *workbenchTestCtx, buildID string) {
	t.Helper()
	dir := t.TempDir()
	manifest := `{"buildId": "` + buildID + `"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("写入测试插件清单失败: %v", err)
	}
	ctx.pluginRootDir = dir
}

// TestRunFullRecognitionEvaluatorGate 求值器构建标识的增量闸门三态：相等→增量；
// 当前为空串（未打标构建）→放行；旧快照空串而当前有值（存量库升级到打标
// 构建）或两值不同（求值器换版）→强制全量。快照铸造同步铸入当前标识。
func TestRunFullRecognitionEvaluatorGate(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)

	// 打标首轮：全量，快照铸入标识
	stampEvaluatorBuild(t, ctx, "build-a")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("打标首轮识别失败: %v", err)
	}
	first, _ := m.SessionOf(root)
	if first.Snapshot.EvaluatorBuildID != "build-a" {
		t.Fatalf("快照应铸入求值器构建标识 build-a，实际 %q", first.Snapshot.EvaluatorBuildID)
	}

	// 同一求值器且磁盘未变：增量（未变目录按游标跳过）
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("同求值器二轮识别失败: %v", err)
	}
	same, _ := m.SessionOf(root)
	if same.Snapshot.Stats.SkippedDirs == 0 {
		t.Fatal("求值器标识相等且磁盘未变时应按游标跳过（增量）")
	}

	// 求值器换版：标识不同 → 全量，快照铸入新标识
	stampEvaluatorBuild(t, ctx, "build-b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("换版识别失败: %v", err)
	}
	swapped, _ := m.SessionOf(root)
	if swapped.Snapshot.Stats.SkippedDirs != 0 {
		t.Fatalf("求值器换版应强制全量，实际跳过 %d 个目录", swapped.Snapshot.Stats.SkippedDirs)
	}
	if swapped.Snapshot.EvaluatorBuildID != "build-b" {
		t.Fatalf("换版后快照应铸入 build-b，实际 %q", swapped.Snapshot.EvaluatorBuildID)
	}

	// 当前未打标（开发直跑形态）：不参与比对 → 放行（增量），快照铸出空串
	ctx.pluginRootDir = ""
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("未打标识别失败: %v", err)
	}
	untagged, _ := m.SessionOf(root)
	if untagged.Snapshot.Stats.SkippedDirs == 0 {
		t.Fatal("当前构建未打标时应放行既有游标（增量）")
	}
	if untagged.Snapshot.EvaluatorBuildID != "" {
		t.Fatalf("未打标构建铸出的快照标识应为空串，实际 %q", untagged.Snapshot.EvaluatorBuildID)
	}

	// 存量库升级：旧快照空串而当前有值 → 强制全量
	stampEvaluatorBuild(t, ctx, "build-c")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("升级后识别失败: %v", err)
	}
	upgraded, _ := m.SessionOf(root)
	if upgraded.Snapshot.Stats.SkippedDirs != 0 {
		t.Fatalf("旧快照空串而当前有值应强制全量，实际跳过 %d 个目录", upgraded.Snapshot.Stats.SkippedDirs)
	}
}

// TestRunFullRecognitionIncrementalGroupRestore 全员已识别结构群的增量轮复原：
// 结构群成员全部已识别时，文件行不落群组键（已识别成员的群组信息只在群卡），
// 增量轮按目录整组复原群卡，群聚合计数与全量轮一致。
func TestRunFullRecognitionIncrementalGroupRestore(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	// 三文件仅尾号数字互异：结构对齐成群；括号命名惯例规则全部识别 → 全员
	// 已识别的结构群（群卡在场而成员文件行无群组键）
	writeTestFile(t, root+"/成群已识别/【作者A】标题1.jpg", "a")
	writeTestFile(t, root+"/成群已识别/【作者A】标题2.jpg", "b")
	writeTestFile(t, root+"/成群已识别/【作者A】标题3.jpg", "c")

	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮识别失败: %v", err)
	}
	first, _ := m.SessionOf(root)
	groupOfDir := func(snap *RecognitionSnapshot) *SnapGroup {
		for i := range snap.Groups {
			if snap.Groups[i].DirRelKey == "成群已识别" {
				return &snap.Groups[i]
			}
		}
		return nil
	}
	g := groupOfDir(first.Snapshot)
	if g == nil || g.RuleID != "" || len(g.Members) != 3 {
		t.Fatalf("首轮应产出全员已识别的结构群: %+v", g)
	}
	for _, f := range first.Snapshot.Files {
		if f.DirRelKey == "成群已识别" && (len(f.Final) == 0 || f.GroupKey != "") {
			t.Fatalf("首轮成员应已识别且文件行不落群组键: %+v", f)
		}
	}

	// 二轮：磁盘未变 → 全部目录按游标跳过；群卡应整组复原
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("增量识别失败: %v", err)
	}
	second, _ := m.SessionOf(root)
	if second.Snapshot.Stats.SkippedDirs == 0 {
		t.Fatal("增量识别应跳过未变目录，实际跳过 0 个")
	}
	if g2 := groupOfDir(second.Snapshot); g2 == nil || len(g2.Members) != 3 {
		t.Fatalf("增量轮丢失全员已识别结构群: %+v", g2)
	}
	first.Snapshot.Stats.SkippedDirs, second.Snapshot.Stats.SkippedDirs = 0, 0
	if !reflect.DeepEqual(first.Snapshot.Stats, second.Snapshot.Stats) {
		t.Fatalf("增量轮统计漂移: 首轮 %+v，增量轮 %+v", first.Snapshot.Stats, second.Snapshot.Stats)
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
// 应答、识别后三分节（overview/files/domain）分节裁剪与决策态附带、startRecognition
// 启动应答与无会话错误。
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
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}

	// 缺省请求=overview 概览分节：目录树/群/规则统计在场，文件明细与选择域
	// 不出现在本载荷；随答会话决策态；单页整答回执
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 102, Action: workbenchActionGetPanorama, Root: root}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Seq != 102 || resp.Panorama == nil {
		t.Fatalf("getPanorama 失败: %+v", resp)
	}
	if resp.Panorama.DirMtimes != nil {
		t.Fatal("前端投影不应携带增量游标表")
	}
	if len(resp.Panorama.Dirs) == 0 || len(resp.Panorama.Groups) != 1 || len(resp.Panorama.RuleStats) == 0 {
		t.Fatalf("概览分节缺节: dirs=%d groups=%d rules=%d",
			len(resp.Panorama.Dirs), len(resp.Panorama.Groups), len(resp.Panorama.RuleStats))
	}
	if len(resp.Panorama.Files) != 0 || resp.Domain != nil {
		t.Fatal("概览分节不应携带文件明细与选择域")
	}
	if resp.Decisions == nil {
		t.Fatal("概览分节应附带会话决策态（元素7 骨架）")
	}
	if resp.Page == nil || resp.Page.Section != panoramaSectionOverview || !resp.Page.Done ||
		resp.Page.NextCursor != "" || resp.Page.Generation != sess.Snapshot.GeneratedAtMs {
		t.Fatalf("概览分节分页回执不符: %+v", resp.Page)
	}

	// files 分节：仅文件明细（整树单页承载），不携带目录树/群/规则态/决策态/
	// 差异集/选择域；回执按全量条数落 total/done
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 103, Action: workbenchActionGetPanorama, Root: root, Section: panoramaSectionFiles}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.Seq != 103 || resp.Panorama == nil {
		t.Fatalf("分节 getPanorama 失败: %+v", resp)
	}
	if len(resp.Panorama.Files) != len(recognitionSampleRelPaths) {
		t.Fatalf("文件分节形态不符: files=%d", len(resp.Panorama.Files))
	}
	if len(resp.Panorama.Dirs) != 0 || len(resp.Panorama.Groups) != 0 || len(resp.Panorama.RuleStats) != 0 {
		t.Fatal("文件分节不应携带目录树/群/规则态")
	}
	if resp.Decisions != nil || resp.Diff != nil || resp.Domain != nil {
		t.Fatal("文件分节不应随答决策态/差异集/选择域")
	}
	if resp.Page == nil || resp.Page.Section != panoramaSectionFiles || resp.Page.Offset != 0 ||
		resp.Page.Count != len(recognitionSampleRelPaths) || resp.Page.Total != len(recognitionSampleRelPaths) ||
		!resp.Page.Done || resp.Page.NextCursor != "" {
		t.Fatalf("文件分节分页回执不符: %+v", resp.Page)
	}

	// domain 分节：仅选择域（样例树=8 作品+1 集，合计 9 条目单页承载），
	// 不携带全景（无 Files/Dirs）与决策态/差异集
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 106, Action: workbenchActionGetPanorama, Root: root, Section: panoramaSectionDomain}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.Seq != 106 || resp.Panorama != nil || resp.Domain == nil {
		t.Fatalf("选择域分节 getPanorama 失败: %+v", resp)
	}
	if len(resp.Domain.Works) != 8 || len(resp.Domain.Sets) != 1 {
		t.Fatalf("选择域分节形态不符: works=%d sets=%d", len(resp.Domain.Works), len(resp.Domain.Sets))
	}
	if resp.Decisions != nil || resp.Diff != nil {
		t.Fatal("选择域分节不应随答决策态/差异集")
	}
	if resp.Page == nil || resp.Page.Section != panoramaSectionDomain || resp.Page.Offset != 0 ||
		resp.Page.Count != 9 || resp.Page.Total != 9 || !resp.Page.Done || resp.Page.NextCursor != "" {
		t.Fatalf("选择域分节分页回执不符: %+v", resp.Page)
	}

	// 未知分节词汇：错误应答
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 107, Action: workbenchActionGetPanorama, Root: root, Section: "不存在的分节"}))
	if resp := ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "未知的 getPanorama 分节") {
		t.Fatalf("未知分节应错误应答: %+v", resp)
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

// injectSessionSnapshot 测试直注：经识别完成落库入口写入指定快照（绕过识别
// 干跑，构造分页/世代/页界测试所需的大规模或特定世代快照；重复调用=模拟
// 重识别覆写，世代随新快照前移）。
func injectSessionSnapshot(t *testing.T, m *SessionManager, root string, snap *RecognitionSnapshot) {
	t.Helper()
	if _, err := m.CompleteRecognition(root, snap); err != nil {
		t.Fatalf("注入快照失败: %v", err)
	}
}

// TestGetPanoramaCursorPaging 游标分页：5 万条文件快照按页大小提示逐页拉取，
// 拼接与全量投影逐条全等、Page 回执 offset/count/total/nextCursor/done 逐页
// 正确；选择域分节同口径（作品序列在前集在后，含跨界页）。
func TestGetPanoramaCursorPaging(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)

	const fileTotal = 50000
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000000,
		RootPath:      root,
		Files:         make([]SnapFile, fileTotal),
	}
	for i := range snap.Files {
		rel := fmt.Sprintf("目录甲/文件%06d.jpg", i)
		snap.Files[i] = SnapFile{RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: "目录甲", State: snapStatePlain}
	}
	injectSessionSnapshot(t, m, root, snap)

	// files 分节：提示 1 万条/页 → 5 页拉完
	var got []SnapFile
	cursor := ""
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(200 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionFiles, Cursor: cursor, PageSizeHint: 10000,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Panorama == nil || resp.Page == nil {
			t.Fatalf("files 分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		p := resp.Page
		if p.Section != panoramaSectionFiles || p.Generation != snap.GeneratedAtMs ||
			p.Total != fileTotal || p.Offset != len(got) || p.Count != len(resp.Panorama.Files) {
			t.Fatalf("files 页回执不符（第 %d 页）: %+v（已收 %d 条）", page+1, p, len(got))
		}
		got = append(got, resp.Panorama.Files...)
		if p.Done {
			if p.NextCursor != "" {
				t.Fatal("末页不应携带续拉游标")
			}
			if page+1 != 5 {
				t.Fatalf("提示 1 万条/页应 5 页拉完，实际 %d 页", page+1)
			}
			break
		}
		if p.NextCursor == "" {
			t.Fatal("非末页缺续拉游标")
		}
		cursor = p.NextCursor
	}
	if !reflect.DeepEqual(got, panoramaViewOf(snap).Files) {
		t.Fatal("逐页拼接与全量投影逐条不全等")
	}

	// domain 分节：6 集成员（1 集）+ 6 落单（同目录聚合 1 作品）→ 作品 7 + 集 1
	// = 8 条目共用一个下标空间；提示 3 条/页 → 第 3 页跨界（作品尾+集头同页）
	domSnap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000001,
		RootPath:      root,
	}
	for i := 1; i <= 6; i++ {
		rel := fmt.Sprintf("甲/画集_%02d.jpg", i)
		domSnap.Files = append(domSnap.Files, SnapFile{
			RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: "甲",
			State: snapStatePlain, GroupKey: "画集", GroupSeq: i,
		})
	}
	for i := 1; i <= 6; i++ {
		rel := fmt.Sprintf("乙/落单_%02d.jpg", i)
		domSnap.Files = append(domSnap.Files, SnapFile{
			RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: "乙", State: snapStatePlain,
		})
	}
	injectSessionSnapshot(t, m, root, domSnap)
	wantDomain := domainViewOf(DeriveWorkDomain(domSnap, nil))
	if len(wantDomain.Works) != 7 || len(wantDomain.Sets) != 1 {
		t.Fatalf("选择域测试桩形态不符: works=%d sets=%d", len(wantDomain.Works), len(wantDomain.Sets))
	}

	var gotWorks []*SelectableWork
	var gotSets []*WorkSetView
	cursor = ""
	straddled := false
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(300 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionDomain, Cursor: cursor, PageSizeHint: 3,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Domain == nil || resp.Page == nil || resp.Panorama != nil {
			t.Fatalf("domain 分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		p := resp.Page
		if p.Section != panoramaSectionDomain || p.Generation != domSnap.GeneratedAtMs ||
			p.Total != 8 || p.Offset != len(gotWorks)+len(gotSets) ||
			p.Count != len(resp.Domain.Works)+len(resp.Domain.Sets) {
			t.Fatalf("domain 页回执不符（第 %d 页）: %+v", page+1, p)
		}
		if len(resp.Domain.Works) > 0 && len(resp.Domain.Sets) > 0 {
			straddled = true // 作品尾+集头同页
		}
		gotWorks = append(gotWorks, resp.Domain.Works...)
		gotSets = append(gotSets, resp.Domain.Sets...)
		if p.Done {
			if p.NextCursor != "" {
				t.Fatal("末页不应携带续拉游标")
			}
			break
		}
		cursor = p.NextCursor
	}
	if !straddled {
		t.Fatal("提示 3 条/页的 8 条目序列应出现作品→集跨界页")
	}
	if !reflect.DeepEqual(gotWorks, wantDomain.Works) || !reflect.DeepEqual(gotSets, wantDomain.Sets) {
		t.Fatal("选择域逐页拼接与全量推导不全等")
	}
}

// TestGetPanoramaGenerationStale 世代失效：拉取中途快照世代前移 → 旧游标请求
// 报「世代变更，请整轮重拉」错误应答（复用既有错误应答通路）。
func TestGetPanoramaGenerationStale(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)

	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1000,
		RootPath:      root,
	}
	for i := 0; i < 10; i++ {
		rel := fmt.Sprintf("d/f%d.jpg", i)
		snap.Files = append(snap.Files, SnapFile{RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: "d", State: snapStatePlain})
	}
	injectSessionSnapshot(t, m, root, snap)

	// 首页（提示 3 条/页 → 非末页，携带续拉游标）
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 400, Action: workbenchActionGetPanorama, Root: root,
		Section: panoramaSectionFiles, PageSizeHint: 3,
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Page == nil || resp.Page.Done || resp.Page.NextCursor == "" {
		t.Fatalf("files 首页应答不符: %+v", resp)
	}

	// 拉取中途换世代（模拟重识别产出新快照）→ 旧游标请求报世代变更
	newer := *snap
	newer.GeneratedAtMs = 2000
	injectSessionSnapshot(t, m, root, &newer)
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 401, Action: workbenchActionGetPanorama, Root: root,
		Section: panoramaSectionFiles, Cursor: resp.Page.NextCursor, PageSizeHint: 3,
	}))
	if stale := ctx.lastResponse(t); stale.OK || !strings.Contains(stale.Error, "世代变更") {
		t.Fatalf("旧游标应报世代变更: %+v", stale)
	}

	// 游标解析的世代校验单锚（sentinel 语义）
	if _, err := decodePanoramaCursor("1000:3", 2000); !errors.Is(err, errPanoramaGenerationStale) {
		t.Fatalf("世代不一致应回 errPanoramaGenerationStale: %v", err)
	}
	if _, err := decodePanoramaCursor("残缺游标", 2000); !errors.Is(err, errPanoramaCursorInvalid) {
		t.Fatalf("无法解析的游标应回 errPanoramaCursorInvalid: %v", err)
	}
}

// TestGetPanoramaPageByteBound 页实测硬门：每页断言以 json.Marshal 实测完整
// 应答字节为准（≤ 软界 panoramaPageSoftBytes，非估算值）——估算函数仅作首遍
// 打包提示，实测兜底漂移。夹具三形态：
//   - 估算放大型：RelPath/DirRelKey 含大量 `<&>`（json 序列化为 \u003c 等
//     六字节转义，估算按单字节计数低估约 6 倍）——估算首遍装出的页实测超
//     软界，硬门须从尾部卸条目直至达标（首页回执条数 < 估算首遍装页条数
//     即卸载实证）；
//   - 真实嵌套形态：每条含多条结构分段线索（投影展开）+ 终选声明 + 疑似
//     线索 + 命中规则；
//   - 单条巨条目：独立成页（首条必装的推进保证）。
func TestGetPanoramaPageByteBound(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)

	escapeHeavy := strings.Repeat("<&>", 5667) // ~17KB：实测转义后 ~102KB/字段（×2 字段 ≈ 6 倍低估）
	huge := strings.Repeat("y", 3<<20)         // 3MB 目录名：单条估算 ≈ 9MB > 页目标
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000002,
		RootPath:      root,
	}
	hugeRel := huge + "/单条超界.jpg"
	for i := 0; i < 60; i++ {
		rel := fmt.Sprintf("%s/f%02d.jpg", escapeHeavy, i)
		snap.Files = append(snap.Files, SnapFile{RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: escapeHeavy, State: snapStatePlain})
	}
	for i := 0; i < 80; i++ {
		rel := fmt.Sprintf("作者%02d/画集%02d/【作者名】タイトル文段_汉_%05d(%d).jpg", i%7, i%11, i, i%40)
		f := SnapFile{
			RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: fmt.Sprintf("作者%02d/画集%02d", i%7, i%11),
			State: snapStateClueful, GroupKey: fmt.Sprintf("タイトル文段_%05d", i), GroupSeq: i%40 + 1,
			Final: []SnapDecl{{
				Dimension: "作者", Value: fmt.Sprintf("作者名%02d", i%7), Source: "rule",
				RoleName: "画师", RuleID: "test-bracket-author-title",
			}},
			MatchedRuleIDs: []string{"test-bracket-author-title"},
			Clues:          []SnapClue{{Kind: "库内作者", Detail: fmt.Sprintf("命中库内作者 作者名%02d", i%7)}},
		}
		snap.Files = append(snap.Files, f)
	}
	snap.Files = append(snap.Files, SnapFile{RelPath: hugeRel, FileName: fileNameOfRel(hugeRel), DirRelKey: huge, State: snapStatePlain})
	injectSessionSnapshot(t, m, root, snap)
	view := panoramaViewOf(snap)

	// 估算首遍装页条数（首页）：硬门卸载的对照基线
	firstPassEnd := panoramaPackEnd(func(i int) int { return snapFileWireBytes(view.Files[i]) }, 0, len(view.Files), panoramaPageMaxEntries)

	var got []SnapFile
	hugeAlone := false
	trimmedFirstPage := false
	cursor := ""
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(500 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionFiles, Cursor: cursor,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Panorama == nil || resp.Page == nil {
			t.Fatalf("字节界分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("页应答序列化失败: %v", err)
		}
		if len(data) > panoramaPageSoftBytes {
			t.Fatalf("第 %d 页实测 %d 字节超软界 %d", page+1, len(data), panoramaPageSoftBytes)
		}
		if page == 0 && resp.Page.Count < firstPassEnd {
			trimmedFirstPage = true // 估算放大型条目超软界 → 硬门已卸载
		}
		if len(resp.Panorama.Files) == 1 && resp.Panorama.Files[0].RelPath == hugeRel {
			hugeAlone = true
		}
		got = append(got, resp.Panorama.Files...)
		if resp.Page.Done {
			if page+1 < 2 {
				t.Fatalf("超界规模快照应分多页，实际 %d 页", page+1)
			}
			break
		}
		cursor = resp.Page.NextCursor
	}
	if len(got) != 141 {
		t.Fatalf("逐页合计条数 = %d，期望 141", len(got))
	}
	if !hugeAlone {
		t.Fatal("单条超界条目应独立成页")
	}
	if !trimmedFirstPage {
		t.Fatalf("估算放大型条目应触发硬门卸载（首页回执 %d 条 ≥ 估算首遍 %d 条）", 0, firstPassEnd)
	}
	if !reflect.DeepEqual(got, view.Files) {
		t.Fatal("逐页拼接与全量投影逐条不全等")
	}
}

// TestGetPanoramaOverviewPaging overview 分节分页：应答的全部重载荷清单——
// 全景六段（目录树/序列群/候选三仓/待裁决明细）+决策态三清单（赋值/忽略/
// 选择集）+差异集三清单（变更/新增/消失）——共用一个下标空间逐页预算装配，
// 逐页拼接与全量投影/全量决策态/全量差异集逐条全等（含跨界页：一页同时携带
// 相邻段条目）；统计/规则档案态小字段与决策态/差异集容器每页随答、文件明细
// 恒不随答；每页实测 ≤ 软界。
func TestGetPanoramaOverviewPaging(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, map[string]SessionAssignment{
		"甲/a_1.jpg": {Author: "作者甲", Title: "标题甲", UpdatedAtMs: 1700000000001},
		"乙/b_1.jpg": {Author: "作者乙", Collection: "集乙", UpdatedAtMs: 1700000000002},
	})
	// 决策态另两清单入段：忽略清单与选择集直接注入会话（轻字段）
	{
		sess, err := m.loadSession(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		sess.Ignored = []string{"乙/b_1.jpg"}
		sess.Selection = []string{"f:甲/a_1.jpg"}
		if err := m.saveSession(sess); err != nil {
			t.Fatalf("注入忽略/选择集失败: %v", err)
		}
	}

	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000003,
		RootPath:      root,
		Dirs: []SnapDir{
			{RelKey: "甲", Name: "甲", Depth: 1, FileCount: 2},
			{RelKey: "乙", Name: "乙", Depth: 1, FileCount: 2},
		},
		Groups: []SnapGroup{{DirRelKey: "甲", Key: "群A", Size: 2, RuleID: "test-sibling-set",
			Members: []SnapGroupMember{{RelPath: "甲/a_1.jpg", Seq: 1, SeqText: "1"}}}},
		Candidates:          []SnapCandidate{{Key: "候选甲", Dir: "甲", Source: candSourceDir, Size: 2, Reason: "目录信号", Members: []SnapCandMember{{RelPath: "甲/a_1.jpg"}}}},
		IgnoredCandidates:   []SnapCandidate{{Key: "忽略乙", Dir: "乙", Source: candSourceGroup, Size: 1, Members: []SnapCandMember{{RelPath: "乙/b_1.jpg"}}}},
		ConfirmedCandidates: []SnapCandidate{{Key: "确认丙", Dir: "乙", Source: candSourceDir, Size: 1, ConfirmedSetName: "确认丙", Members: []SnapCandMember{{RelPath: "乙/c_1.jpg"}}}},
		Pending: []SnapPending{{RelPath: "甲/a_1.jpg", FileName: "a_1.jpg", Dimension: "标题",
			Reason: "同层冲突", Candidates: []SnapDecl{{Dimension: "标题", Value: "甲", Source: "rule"}}}},
		RuleStats: []SnapRuleStat{{ID: "test-sibling-set", Description: "测试：下划线前缀+序号成群", Source: string(RuleSourceManual), Enabled: true, HitCount: 2}},
		Stats:     SnapStats{TotalFiles: 4, GroupCount: 1, PendingFiles: 1},
	}
	diff := &SnapDiff{
		Changed: []SnapDiffEntry{{RelPath: "乙/b_1.jpg",
			Before: &SnapConclusion{State: snapStatePlain}, After: &SnapConclusion{State: snapStateClueful}}},
		Removed: []SnapDiffEntry{},
		Added:   []SnapDiffEntry{{RelPath: "甲/a_1.jpg", After: &SnapConclusion{State: snapStatePlain}}},
	}
	if _, err := m.CompleteRecognitionWithDiff(root, snap, diff); err != nil {
		t.Fatalf("注入快照与差异集失败: %v", err)
	}
	sessFull, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取全会话失败: %v", err)
	}
	want := panoramaOverviewViewOf(snap)
	wantDec := decisionsViewOf(sessFull)
	total := len(snap.Dirs) + len(snap.Groups) + len(snap.Candidates) +
		len(snap.IgnoredCandidates) + len(snap.ConfirmedCandidates) + len(snap.Pending) +
		len(wantDec.Assignments) + len(wantDec.Ignored) + len(wantDec.Selection) +
		len(diff.Changed) + len(diff.Added) + len(diff.Removed)

	var gotDirs []SnapDir
	var gotGroups []SnapGroup
	var gotCands, gotIgnored, gotConfirmed []SnapCandidate
	var gotPending []SnapPending
	gotDec := &workbenchSessionDecisionsView{
		Assignments: []workbenchAssignmentView{}, Ignored: []string{}, Selection: []string{},
	}
	gotDiff := &SnapDiff{Changed: []SnapDiffEntry{}, Added: []SnapDiffEntry{}, Removed: []SnapDiffEntry{}}
	straddled := false
	cursor := ""
	pages := 0
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(600 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionOverview, Cursor: cursor, PageSizeHint: 2,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Panorama == nil || resp.Page == nil {
			t.Fatalf("overview 分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("页应答序列化失败: %v", err)
		}
		if len(data) > panoramaPageSoftBytes {
			t.Fatalf("第 %d 页实测 %d 字节超软界 %d", page+1, len(data), panoramaPageSoftBytes)
		}
		p := resp.Page
		received := len(gotDirs) + len(gotGroups) + len(gotCands) + len(gotIgnored) + len(gotConfirmed) +
			len(gotPending) + len(gotDec.Assignments) + len(gotDec.Ignored) + len(gotDec.Selection) +
			len(gotDiff.Changed) + len(gotDiff.Added) + len(gotDiff.Removed)
		count := countOverviewPage(resp.Panorama) + countDecisionsPage(resp.Decisions) + countDiffPage(resp.Diff)
		if p.Section != panoramaSectionOverview || p.Generation != snap.GeneratedAtMs ||
			p.Total != total || p.Offset != received || p.Count != count {
			t.Fatalf("overview 页回执不符（第 %d 页）: %+v（已收 %d 条，页计 %d）", page+1, p, received, count)
		}
		// 小字段每页随答；文件明细恒不随答
		if len(resp.Panorama.RuleStats) == 0 || resp.Panorama.Stats.TotalFiles != 4 || len(resp.Panorama.Files) != 0 {
			t.Fatalf("overview 页应每页随答小字段且不携带文件明细（第 %d 页）", page+1)
		}
		// 决策态/差异集容器每页随答（清单页片由装配器区间落页）
		if resp.Decisions == nil || resp.Diff == nil {
			t.Fatalf("overview 页应每页随答决策态与差异集容器（第 %d 页）", page+1)
		}
		pan := resp.Panorama
		if len(pan.Dirs) > 0 && len(pan.Groups) > 0 || len(pan.Groups) > 0 && len(pan.Candidates) > 0 ||
			len(pan.Candidates) > 0 && len(pan.IgnoredCandidates) > 0 || len(pan.IgnoredCandidates) > 0 && len(pan.ConfirmedCandidates) > 0 ||
			len(pan.ConfirmedCandidates) > 0 && len(pan.Pending) > 0 {
			straddled = true
		}
		if len(pan.Pending) > 0 && len(resp.Decisions.Assignments) > 0 ||
			len(resp.Decisions.Selection) > 0 && len(resp.Diff.Changed) > 0 {
			straddled = true // 决策态/差异集段的跨界页同样成立
		}
		gotDirs = append(gotDirs, pan.Dirs...)
		gotGroups = append(gotGroups, pan.Groups...)
		gotCands = append(gotCands, pan.Candidates...)
		gotIgnored = append(gotIgnored, pan.IgnoredCandidates...)
		gotConfirmed = append(gotConfirmed, pan.ConfirmedCandidates...)
		gotPending = append(gotPending, pan.Pending...)
		gotDec.Assignments = append(gotDec.Assignments, resp.Decisions.Assignments...)
		gotDec.Ignored = append(gotDec.Ignored, resp.Decisions.Ignored...)
		gotDec.Selection = append(gotDec.Selection, resp.Decisions.Selection...)
		gotDiff.Changed = append(gotDiff.Changed, resp.Diff.Changed...)
		gotDiff.Added = append(gotDiff.Added, resp.Diff.Added...)
		gotDiff.Removed = append(gotDiff.Removed, resp.Diff.Removed...)
		pages = page + 1
		if p.Done {
			if p.NextCursor != "" {
				t.Fatal("末页不应携带续拉游标")
			}
			break
		}
		if p.NextCursor == "" {
			t.Fatal("非末页缺续拉游标")
		}
		cursor = p.NextCursor
	}
	if !straddled {
		t.Fatal("提示 2 条/页的清单序列应出现跨段页")
	}
	if pages < 5 {
		t.Fatalf("十二段序列提示 2 条/页应 ≥5 页，实际 %d 页", pages)
	}
	if !reflect.DeepEqual(gotDirs, want.Dirs) || !reflect.DeepEqual(gotGroups, want.Groups) ||
		!reflect.DeepEqual(gotCands, want.Candidates) || !reflect.DeepEqual(gotIgnored, want.IgnoredCandidates) ||
		!reflect.DeepEqual(gotConfirmed, want.ConfirmedCandidates) || !reflect.DeepEqual(gotPending, want.Pending) {
		t.Fatal("overview 逐页拼接与全量投影逐条不全等")
	}
	if !reflect.DeepEqual(gotDec, wantDec) {
		t.Fatalf("决策态逐页拼接与全量决策态不全等: %+v vs %+v", gotDec, wantDec)
	}
	if !reflect.DeepEqual(gotDiff, sessFull.LastDiff) {
		t.Fatalf("差异集逐页拼接与全量差异集不全等: %+v vs %+v", gotDiff, sessFull.LastDiff)
	}
}

// countOverviewPage overview 页回执条数复核：页内全景六段条目合计。
func countOverviewPage(pan *RecognitionSnapshot) int {
	return len(pan.Dirs) + len(pan.Groups) + len(pan.Candidates) +
		len(pan.IgnoredCandidates) + len(pan.ConfirmedCandidates) + len(pan.Pending)
}

// countDecisionsPage overview 页回执条数复核：页内决策态三清单条目合计。
func countDecisionsPage(dec *workbenchSessionDecisionsView) int {
	if dec == nil {
		return 0
	}
	return len(dec.Assignments) + len(dec.Ignored) + len(dec.Selection)
}

// countDiffPage overview 页回执条数复核：页内差异集三清单条目合计。
func countDiffPage(d *SnapDiff) int {
	if d == nil {
		return 0
	}
	return len(d.Changed) + len(d.Added) + len(d.Removed)
}

// TestGetPanoramaOverviewDecisionsDiffBudget 大载荷预算装配实证：decisions/diff
// 巨量条目（用户真实事故形态的 ≥2 倍——长期目录赋值逐文件合并累计的赋值
// 清单 + 重识别差异集；2026-10-31 用户环境 getPanorama overview 首页
// decisions/diff 按小载荷假设整答实测 67,321,044 字节，撞 64MB gRPC 应答
// 断路器致「恢复会话失败」，净室复测同根形态 110,951,071 字节三次。本夹具
// decisions 合成 ≥140MB 级（245k 条赋值 + 忽略/选择集各 4 万条 + 差异集
// 6 万条）。断言面：
//   - 每页整答 json.Marshal 实测 ≤ panoramaPageSoftBytes 软界（装配器页预算
//     设计上 64MB 硬门不可达）；
//   - 跨页拼接与全量决策态逐字节全等（赋值档案全量可达，语义零丢失）；差异集
//     与线上传输投影（头 panoramaDiffConclusionHead 条保留结论、其余仅路径——
//     护栏四瘦身）逐字节全等，存储态全量保全由
//     TestGetPanoramaDiffConclusionProjection 锚定；
//   - 回执 offset/count/total 含 decisions/diff 段合计，末页声明收敛。
func TestGetPanoramaOverviewDecisionsDiffBudget(t *testing.T) {
	root := t.TempDir()

	// 赋值条目形态对齐真实目录赋值（每赋值一目录=每文件一条，五维度+出处引用）；
	// 实测 ~613B/条 × 245k ≈ 150MB ≥ 140MB（用户真实 67MB 的 ≥2 倍）
	const assignTotal = 245000
	assigns := make(map[string]SessionAssignment, assignTotal)
	for i := 0; i < assignTotal; i++ {
		rel := fmt.Sprintf("库根目录甲/子目录乙/作品集丙长名占位段/卷次目录_%03d/文件名前段占位文段甲乙丙丁_%06d.jpg", i%40, i)
		assigns[rel] = SessionAssignment{
			Author:     fmt.Sprintf("作者名占位文段甲乙丙丁_%06d", i),
			AuthorRole: "画师",
			Title:      fmt.Sprintf("标题文段占位甲乙丙丁戊己庚辛_%06d_尾注文段", i),
			Tags: []string{
				fmt.Sprintf("标签甲乙丙_%06d", i), "公共标签占位文段一",
				fmt.Sprintf("标签丙丁戊_%06d", i%97), "公共标签占位文段二",
				fmt.Sprintf("标签己庚辛_%06d", i%31),
			},
			Collection:        fmt.Sprintf("集名占位文段甲乙丙_%06d", i%53),
			AuthorClueRef:     "dirLevelStripped[1]",
			CollectionClueRef: "filenameSeg[0]",
			UpdatedAtMs:       1700000000000 + int64(i),
		}
	}
	m, ctx := newRecognitionFixture(t, root, assigns)
	// 决策态另两清单入段：忽略/选择集各 4 万条（各 ~2MB 级）
	{
		sess, err := m.loadSession(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		ignored := make([]string, 0, 40000)
		selection := make([]string, 0, 40000)
		for i := 0; i < 40000; i++ {
			ignored = append(ignored, fmt.Sprintf("库根目录甲/子目录乙/忽略文件占位_%06d.jpg", i))
			selection = append(selection, fmt.Sprintf("d:库根目录甲/子目录乙/作品集丙长名占位段/卷次目录_%03d", i%40))
		}
		sess.Ignored = ignored
		sess.Selection = selection
		if err := m.saveSession(sess); err != nil {
			t.Fatalf("注入忽略/选择集失败: %v", err)
		}
	}

	// 快照本体保持小量（六段在场即可），差异集巨量：变更 5 万 + 新增/消失各 5 千
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000005,
		RootPath:      root,
		Dirs: []SnapDir{
			{RelKey: "库根目录甲", Name: "库根目录甲", Depth: 1, FileCount: assignTotal},
			{RelKey: "库根目录甲/子目录乙", Name: "子目录乙", Depth: 2, FileCount: assignTotal},
		},
		Stats: SnapStats{TotalFiles: assignTotal},
	}
	diff := &SnapDiff{
		Changed: make([]SnapDiffEntry, 0, 50000),
		Added:   make([]SnapDiffEntry, 0, 5000),
		Removed: make([]SnapDiffEntry, 0, 5000),
	}
	for i := 0; i < 50000; i++ {
		rel := fmt.Sprintf("库根目录甲/子目录乙/作品集丙长名占位段/卷次目录_%03d/变更文件占位文段甲乙丙_%06d.jpg", i%40, i)
		diff.Changed = append(diff.Changed, SnapDiffEntry{RelPath: rel,
			Before: &SnapConclusion{State: snapStatePlain, PendingDims: []string{"标题", "作者"},
				Final: []SnapDecl{{Dimension: "作者", Value: fmt.Sprintf("旧作者占位_%06d", i), Source: "rule", RoleName: "画师"}}},
			After: &SnapConclusion{State: snapStateIdentified,
				Final: []SnapDecl{{Dimension: "作者", Value: fmt.Sprintf("新作者占位_%06d", i), Source: "assign", RoleName: "画师"},
					{Dimension: "标题", Value: fmt.Sprintf("新标题占位_%06d", i), Source: "assign"}}}})
	}
	for i := 0; i < 5000; i++ {
		diff.Added = append(diff.Added, SnapDiffEntry{
			RelPath: fmt.Sprintf("库根目录甲/子目录乙/新增文件占位文段甲乙丙_%06d.jpg", i),
			After:   &SnapConclusion{State: snapStatePlain}})
		diff.Removed = append(diff.Removed, SnapDiffEntry{
			RelPath: fmt.Sprintf("库根目录甲/子目录乙/消失文件占位文段甲乙丙_%06d.jpg", i),
			Before:  &SnapConclusion{State: snapStateIdentified}})
	}
	if _, err := m.CompleteRecognitionWithDiff(root, snap, diff); err != nil {
		t.Fatalf("注入快照与差异集失败: %v", err)
	}
	sessFull, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取全会话失败: %v", err)
	}
	wantDec := decisionsViewOf(sessFull)
	wantDecJSON, err := json.Marshal(wantDec)
	if err != nil {
		t.Fatalf("全量决策态序列化失败: %v", err)
	}
	// 夹具量级自检：decisions ≥140MB（用户真实 67,321,044 字节的 ≥2 倍）
	if len(wantDecJSON) < 140<<20 {
		t.Fatalf("夹具量级不足：全量决策态 %d 字节 < 140MB（%d 条赋值）", len(wantDecJSON), assignTotal)
	}

	wantDirCount := len(snap.Dirs)
	gotDirs := 0
	gotDec := &workbenchSessionDecisionsView{
		Assignments: []workbenchAssignmentView{}, Ignored: []string{}, Selection: []string{},
	}
	gotDiff := &SnapDiff{Changed: []SnapDiffEntry{}, Added: []SnapDiffEntry{}, Removed: []SnapDiffEntry{}}
	total := len(snap.Dirs) + len(wantDec.Assignments) + len(wantDec.Ignored) + len(wantDec.Selection) +
		len(diff.Changed) + len(diff.Added) + len(diff.Removed)
	var lastMeasured int
	cursor := ""
	pages := 0
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(900 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionOverview, Cursor: cursor,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Panorama == nil || resp.Page == nil {
			t.Fatalf("overview 大载荷分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("页应答序列化失败: %v", err)
		}
		// 整答实测尺寸断言：≤12MB 软界（12MB 与 64MB 断路器之间的安全带，
		// 分页预算设计上排除硬门可达性）
		if len(data) > panoramaPageSoftBytes {
			t.Fatalf("第 %d 页整答实测 %d 字节超软界 %d", page+1, len(data), panoramaPageSoftBytes)
		}
		lastMeasured = len(data)
		p := resp.Page
		received := gotDirs + len(gotDec.Assignments) + len(gotDec.Ignored) + len(gotDec.Selection) +
			len(gotDiff.Changed) + len(gotDiff.Added) + len(gotDiff.Removed)
		count := countOverviewPage(resp.Panorama) + countDecisionsPage(resp.Decisions) + countDiffPage(resp.Diff)
		if p.Section != panoramaSectionOverview || p.Generation != snap.GeneratedAtMs ||
			p.Total != total || p.Offset != received || p.Count != count || count < 1 {
			t.Fatalf("overview 页回执不符（第 %d 页）: %+v（已收 %d 条，页计 %d）", page+1, p, received, count)
		}
		if resp.Decisions == nil || resp.Diff == nil {
			t.Fatalf("overview 页应每页随答决策态与差异集容器（第 %d 页）", page+1)
		}
		gotDirs += countOverviewPage(resp.Panorama)
		gotDec.Assignments = append(gotDec.Assignments, resp.Decisions.Assignments...)
		gotDec.Ignored = append(gotDec.Ignored, resp.Decisions.Ignored...)
		gotDec.Selection = append(gotDec.Selection, resp.Decisions.Selection...)
		gotDiff.Changed = append(gotDiff.Changed, resp.Diff.Changed...)
		gotDiff.Added = append(gotDiff.Added, resp.Diff.Added...)
		gotDiff.Removed = append(gotDiff.Removed, resp.Diff.Removed...)
		pages = page + 1
		if p.Done {
			if p.NextCursor != "" {
				t.Fatal("末页不应携带续拉游标")
			}
			break
		}
		if p.NextCursor == "" {
			t.Fatal("非末页缺续拉游标")
		}
		cursor = p.NextCursor
	}
	if pages < 15 {
		t.Fatalf("≥140MB 决策态 + 6 万差异条目按 8MB 页目标应 ≥15 页，实际 %d 页", pages)
	}
	if gotDirs != wantDirCount {
		t.Fatalf("全景六段逐页合计目录条数 = %d，期望 %d", gotDirs, wantDirCount)
	}
	// 跨页拼接语义全等锚：与全量决策态 JSON 逐字节全等（赋值档案全量可达，
	// 语义零丢失）；差异集与线上传输投影逐字节全等（见下，护栏四瘦身）
	gotDecJSON, err := json.Marshal(gotDec)
	if err != nil {
		t.Fatalf("拼接决策态序列化失败: %v", err)
	}
	if !bytes.Equal(gotDecJSON, wantDecJSON) {
		t.Fatalf("决策态跨页拼接与全量决策态 JSON 不逐字节全等（%d vs %d 字节）", len(gotDecJSON), len(wantDecJSON))
	}
	gotDiffJSON, err := json.Marshal(gotDiff)
	if err != nil {
		t.Fatalf("拼接差异集序列化失败: %v", err)
	}
	// 护栏四（差异巨表装载瘦身）：差异集线上传输投影——每清单头
	// panoramaDiffConclusionHead 条保留前后结论、其余仅路径；拼接与投影形态
	// 逐字节全等（路径全量可达；存储态全量保全由
	// TestGetPanoramaDiffConclusionProjection 锚定）。
	wantDiffJSON, err := json.Marshal(&SnapDiff{
		Changed: wireDiffEntriesOf(sessFull.LastDiff.Changed),
		Added:   wireDiffEntriesOf(sessFull.LastDiff.Added),
		Removed: wireDiffEntriesOf(sessFull.LastDiff.Removed),
	})
	if err != nil {
		t.Fatalf("差异集线上投影序列化失败: %v", err)
	}
	if !bytes.Equal(gotDiffJSON, wantDiffJSON) {
		t.Fatalf("差异集跨页拼接与线上投影 JSON 不逐字节全等（%d vs %d 字节）", len(gotDiffJSON), len(wantDiffJSON))
	}
	t.Logf("decisions=%d 字节 diff=%d 字节，%d 页拉完，末页整答 %d 字节",
		len(wantDecJSON), len(wantDiffJSON), pages, lastMeasured)
}

// previewBoundFixture 巨条目夹具（条目成员无界击穿页界的真实形态）：巨群 +
// 候选三仓各一条 giantMembers 成员（每成员带元素数组=目录来源候选的元素级
// 放大），另备落单目录 250 文件（domain 分节的目录聚合多轨作品形态）。
func previewBoundFixture(root string, giantMembers int) (snap *RecognitionSnapshot, dirTracksFiles int) {
	bigElements := func(i int) []SnapEl {
		return []SnapEl{
			{Text: fmt.Sprintf("公共前缀文段_%05d_尾注文段甲乙丙丁", i), Kind: elKindTitle},
			{Text: "_", Kind: elKindSep},
			{Text: fmt.Sprintf("%05d", i), Kind: elKindSeq},
			{Text: "【作者名占位文段】", Kind: elKindAuthor},
			{Text: "（标题文段占位）", Kind: elKindTitle},
		}
	}
	groupMembers := make([]SnapGroupMember, 0, giantMembers)
	candMembers := make([]SnapCandMember, 0, giantMembers)
	files := make([]SnapFile, 0, giantMembers+250)
	for i := 0; i < giantMembers; i++ {
		rel := fmt.Sprintf("巨目录/成员_%05d.jpg", i)
		groupMembers = append(groupMembers, SnapGroupMember{
			RelPath: rel, Seq: i + 1, SeqText: fmt.Sprintf("%05d", i+1), Elements: bigElements(i),
		})
		candMembers = append(candMembers, SnapCandMember{
			RelPath: rel, SeqText: fmt.Sprintf("%05d", i+1), Elements: bigElements(i),
		})
		files = append(files, SnapFile{
			RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: "巨目录",
			State: snapStatePlain, GroupKey: "公共前缀文段", GroupSeq: i + 1,
		})
	}
	const dirTracksTotal = 250 // > panoramaMemberPreviewLimit：目录聚合作品截断实证
	for i := 0; i < dirTracksTotal; i++ {
		rel := fmt.Sprintf("落单目录/散件_%03d.jpg", i)
		files = append(files, SnapFile{
			RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: "落单目录", State: snapStatePlain,
		})
	}
	snap = &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000004,
		RootPath:      root,
		Dirs: []SnapDir{
			{RelKey: "巨目录", Name: "巨目录", Depth: 1, FileCount: giantMembers},
			{RelKey: "落单目录", Name: "落单目录", Depth: 1, FileCount: dirTracksTotal},
		},
		Files:               files,
		Groups:              []SnapGroup{{DirRelKey: "巨目录", Key: "公共前缀文段", Size: giantMembers, Members: groupMembers}},
		Candidates:          []SnapCandidate{{Key: "公共前缀文段", Dir: "巨目录", Source: candSourceGroup, Size: giantMembers, Reason: "巨群候选", Members: candMembers}},
		IgnoredCandidates:   []SnapCandidate{{Key: "忽略巨候选", Dir: "巨目录", Source: candSourceDir, Size: giantMembers, Reason: "忽略分仓", Members: candMembers}},
		ConfirmedCandidates: []SnapCandidate{{Key: "确认巨候选", Dir: "巨目录", Source: candSourceDir, Size: giantMembers, Reason: "确认分仓", Members: candMembers}},
	}
	return snap, dirTracksTotal
}

// TestGetPanoramaMemberPreviewBound 条目成员预览截断（协议级页界修复）：overview
// 六段与 domain 分节的重载荷条目成员原本无界——清存储新库首用时巨目录候选
// 单条 ~3.8 万成员 × ~2KB ≈ 79MB 直撞 64MB 应答断路器。投影层按
// panoramaMemberPreviewLimit 截前 200 个（Size=真总数）：逐页 json.Marshal 实测
// 全 ≤ 软界、条目 Members ≤ 上限、逐页拼接与全量投影逐条全等（语义守卫两侧
// 同截断）、存储快照与推导域保持全量成员（服务端动作按目标键全量解析）。
func TestGetPanoramaMemberPreviewBound(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)

	const giantMembers = 5000
	snap, dirTracksTotal := previewBoundFixture(root, giantMembers)
	injectSessionSnapshot(t, m, root, snap)
	want := panoramaOverviewViewOf(snap)

	// overview：提示 2 条/页逐页拉完（六段跨界页全覆盖）
	var gotGroups []SnapGroup
	var gotCands, gotIgnored, gotConfirmed []SnapCandidate
	cursor := ""
	pages := 0
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(700 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionOverview, Cursor: cursor, PageSizeHint: 2,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Panorama == nil || resp.Page == nil {
			t.Fatalf("overview 分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("页应答序列化失败: %v", err)
		}
		if len(data) > panoramaPageSoftBytes {
			t.Fatalf("第 %d 页实测 %d 字节超软界 %d", page+1, len(data), panoramaPageSoftBytes)
		}
		pan := resp.Panorama
		for _, g := range pan.Groups {
			if len(g.Members) > panoramaMemberPreviewLimit || g.Size != giantMembers {
				t.Fatalf("群条目应截 %d 成员且 Size=真总数: members=%d size=%d",
					panoramaMemberPreviewLimit, len(g.Members), g.Size)
			}
		}
		for _, c := range append(append(append([]SnapCandidate{}, pan.Candidates...),
			pan.IgnoredCandidates...), pan.ConfirmedCandidates...) {
			if len(c.Members) > panoramaMemberPreviewLimit || c.Size != giantMembers {
				t.Fatalf("候选条目应截 %d 成员且 Size=真总数: members=%d size=%d",
					panoramaMemberPreviewLimit, len(c.Members), c.Size)
			}
		}
		gotGroups = append(gotGroups, pan.Groups...)
		gotCands = append(gotCands, pan.Candidates...)
		gotIgnored = append(gotIgnored, pan.IgnoredCandidates...)
		gotConfirmed = append(gotConfirmed, pan.ConfirmedCandidates...)
		pages = page + 1
		if resp.Page.Done {
			if resp.Page.NextCursor != "" {
				t.Fatal("末页不应携带续拉游标")
			}
			break
		}
		cursor = resp.Page.NextCursor
	}
	if pages < 3 {
		t.Fatalf("提示 2 条/页的六段快照应多页，实际 %d 页", pages)
	}
	if len(gotGroups) != 1 || len(gotCands) != 1 || len(gotIgnored) != 1 || len(gotConfirmed) != 1 {
		t.Fatalf("逐页条目合计不符: groups=%d cands=%d ignored=%d confirmed=%d",
			len(gotGroups), len(gotCands), len(gotIgnored), len(gotConfirmed))
	}
	if len(gotGroups[0].Members) != panoramaMemberPreviewLimit {
		t.Fatalf("巨群应恰截 %d 成员，实际 %d", panoramaMemberPreviewLimit, len(gotGroups[0].Members))
	}
	if !reflect.DeepEqual(gotGroups, want.Groups) || !reflect.DeepEqual(gotCands, want.Candidates) ||
		!reflect.DeepEqual(gotIgnored, want.IgnoredCandidates) || !reflect.DeepEqual(gotConfirmed, want.ConfirmedCandidates) {
		t.Fatal("overview 逐页拼接与截断后全量投影逐条不全等")
	}

	// 存储快照成员保持全量（投影截断不回写会话；服务端动作按目标键全量解析）
	after, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if len(after.Snapshot.Groups[0].Members) != giantMembers || len(after.Snapshot.Candidates[0].Members) != giantMembers {
		t.Fatal("存储快照成员被投影截断意外改写")
	}

	// domain：5000 集成员作品 + 1 目录聚合作品（250 轨）+ 1 巨集，共 5002 条目；
	// 提示 2501 条/页 → 末页作品尾+集头同页（跨界页全覆盖）
	fullDomain := DeriveWorkDomain(snap, nil)
	if len(fullDomain.Sets) != 1 || len(fullDomain.Sets[0].Members) != giantMembers {
		t.Fatalf("推导域巨集形态不符: sets=%d members=%d", len(fullDomain.Sets), len(fullDomain.Sets[0].Members))
	}
	dirTracksSeen := false
	for _, w := range fullDomain.Works {
		if w.Kind == WorkKindDirTracks {
			if len(w.Files) != dirTracksTotal || dirTracksSeen {
				t.Fatalf("目录聚合作品形态不符: files=%d", len(w.Files))
			}
			dirTracksSeen = true
		}
	}
	if !dirTracksSeen {
		t.Fatal("推导域应含目录聚合多轨作品")
	}

	wantDomain := domainViewOf(fullDomain)
	var gotWorks []*SelectableWork
	var gotSets []*WorkSetView
	cursor = ""
	straddled := false
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(800 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionDomain, Cursor: cursor, PageSizeHint: 2501,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Domain == nil || resp.Page == nil || resp.Panorama != nil {
			t.Fatalf("domain 分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("页应答序列化失败: %v", err)
		}
		if len(data) > panoramaPageSoftBytes {
			t.Fatalf("domain 第 %d 页实测 %d 字节超软界 %d", page+1, len(data), panoramaPageSoftBytes)
		}
		for _, w := range resp.Domain.Works {
			if len(w.Files) > panoramaMemberPreviewLimit {
				t.Fatalf("作品条目轨道 %d 超预览上限 %d", len(w.Files), panoramaMemberPreviewLimit)
			}
		}
		for _, s := range resp.Domain.Sets {
			if len(s.Members) > panoramaMemberPreviewLimit {
				t.Fatalf("集条目成员 %d 超预览上限 %d", len(s.Members), panoramaMemberPreviewLimit)
			}
		}
		if len(resp.Domain.Works) > 0 && len(resp.Domain.Sets) > 0 {
			straddled = true
		}
		gotWorks = append(gotWorks, resp.Domain.Works...)
		gotSets = append(gotSets, resp.Domain.Sets...)
		if resp.Page.Done {
			break
		}
		cursor = resp.Page.NextCursor
	}
	if !straddled {
		t.Fatal("提示 2501 条/页的 5002 条目序列应出现作品→集跨界页")
	}
	if !reflect.DeepEqual(gotWorks, wantDomain.Works) || !reflect.DeepEqual(gotSets, wantDomain.Sets) {
		t.Fatal("domain 逐页拼接与截断后全量投影逐条不全等")
	}
	truncatedTracks, truncatedSet := false, false
	for _, w := range gotWorks {
		if w.Kind == WorkKindDirTracks && len(w.Files) == panoramaMemberPreviewLimit {
			truncatedTracks = true
		}
	}
	for _, s := range gotSets {
		if len(s.Members) == panoramaMemberPreviewLimit {
			truncatedSet = true
		}
	}
	if !truncatedTracks || !truncatedSet {
		t.Fatalf("巨集与目录聚合作品应截至 %d 成员出线: tracks=%v set=%v",
			panoramaMemberPreviewLimit, truncatedTracks, truncatedSet)
	}
}

// TestCandidateActionsOnPreviewTruncatedCandidate 截断展示下的候选动作全量生效：
// 前端仅见前 panoramaMemberPreviewLimit 个成员，忽略/确认按目标键服务端解析
// 全量成员——忽略分仓携带完整成员、确认对全部成员写集维度赋值（affected=真
// 总数），识别刷新后快照分仓仍为全量成员。
func TestCandidateActionsOnPreviewTruncatedCandidate(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)

	// 真实磁盘树：250 个尾缀数字形态文件在单目录 → 结构群成集（成员=250 > 上限）
	const total = 250
	for i := 1; i <= total; i++ {
		p := filepath.Join(root, "巨目录", fmt.Sprintf("成员_%03d.jpg", i))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("建文件失败: %v", err)
		}
	}
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
	snap := snapshotOf()
	var cand *SnapCandidate
	for i := range snap.Candidates {
		if len(snap.Candidates[i].Members) == total {
			cand = &snap.Candidates[i]
			break
		}
	}
	if cand == nil {
		t.Fatalf("应存在 %d 成员的巨候选，实际: %+v", total, snap.Candidates)
	}
	target := cand.target()

	// 出线仅预览前 200（Size=真总数）
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 901, Action: workbenchActionGetPanorama, Root: root,
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Panorama == nil {
		t.Fatalf("getPanorama 失败: %+v", resp)
	}
	var wire *SnapCandidate
	for i := range resp.Panorama.Candidates {
		if resp.Panorama.Candidates[i].target() == target {
			wire = &resp.Panorama.Candidates[i]
			break
		}
	}
	if wire == nil || len(wire.Members) != panoramaMemberPreviewLimit || wire.Size != total {
		t.Fatalf("出线候选应仅预览 %d 成员且 Size=真总数: %+v", panoramaMemberPreviewLimit, wire)
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
				t.Fatal("候选动作触发的识别刷新超时未完成")
			}
			time.Sleep(2 * time.Millisecond)
		}
	}

	// 忽略 → 分仓携带完整成员（会话全量成员解析，不受出线截断影响）
	if _, err := m.SetCandidateIgnored(root, target, true); err != nil {
		t.Fatalf("忽略候选失败: %v", err)
	}
	waitIdle()
	snap = snapshotOf()
	if len(snap.IgnoredCandidates) != 1 || snap.IgnoredCandidates[0].target() != target ||
		len(snap.IgnoredCandidates[0].Members) != total {
		t.Fatalf("忽略分仓应携带完整 %d 成员: %+v", total, snap.IgnoredCandidates[0])
	}
	if _, err := m.SetCandidateIgnored(root, target, false); err != nil {
		t.Fatalf("恢复候选失败: %v", err)
	}
	waitIdle()

	// 确认 → 全量成员写集维度赋值（affected=真总数）
	sess, affected, err := m.ConfirmWorkSetCandidate(root, target, "确认巨集", 0)
	if err != nil || affected != total {
		t.Fatalf("确认候选应生效 %d 成员: err=%v affected=%d", total, err, affected)
	}
	assigned := 0
	for _, mem := range cand.Members {
		if a := sess.Assignments[mem.RelPath]; a.Collection == "确认巨集" {
			assigned++
		}
	}
	if assigned != total {
		t.Fatalf("确认应对全量 %d 成员写集维度赋值，实际 %d", total, assigned)
	}
	waitIdle()
	snap = snapshotOf()
	if len(snap.ConfirmedCandidates) != 1 || len(snap.ConfirmedCandidates[0].Members) != total {
		t.Fatalf("confirmed 分仓应携带完整 %d 成员: %+v", total, snap.ConfirmedCandidates[0])
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
		direct.FormKeyExpr != `^(.+?[^\d]|[^\d])\d{1,4}$` || direct.FormSeqExpr != `(\d{1,4})$` {
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

// TestDirCandidateBareBracketKey 风险2 行为变化锚（声明11）：`[集名]` 裸括号
// 目录的集名候选键由整名改取内名——候选墙呈现变化（属改善）：键=100423502、
// 与库内集名的互证比对走内名口径（整名口径下 "[100423502]" 不等于集名
// "100423502"，互证不可能命中）。
func TestDirCandidateBareBracketKey(t *testing.T) {
	plainFile := func(relPath, dirKey, name string) SnapFile {
		return SnapFile{RelPath: relPath, FileName: name, DirRelKey: dirKey, State: snapStatePlain}
	}
	snap := &RecognitionSnapshot{
		RootPath: "G:/inner world/picture",
		Dirs: []SnapDir{
			{RelKey: "", Name: "picture", Depth: 0},
			{RelKey: "[100423502]", Name: "[100423502]", Depth: 1},
		},
		Files: []SnapFile{
			plainFile("[100423502]/无题_01.jpg", "[100423502]", "无题_01.jpg"),
			plainFile("[100423502]/无题_02.jpg", "[100423502]", "无题_02.jpg"),
		},
	}
	lib := &LibraryIndex{WorkSetNames: map[string]struct{}{"100423502": {}}}
	cands := buildWorkSetCandidates(snap, lib)
	if len(cands) != 1 {
		t.Fatalf("裸括号作品号目录应出 1 卡，实际 %d: %+v", len(cands), cands)
	}
	c := cands[0]
	if c.Source != candSourceDir || c.Dir != "[100423502]" || c.Key != "100423502" || c.Size != 2 {
		t.Fatalf("候选键应由整名改取内名: %+v", c)
	}
	if !c.CrossMatch {
		t.Fatalf("内名键应与库内集名全等互证: %+v", c)
	}
	if target := c.target(); target != "dir\x00[100423502]\x00100423502" {
		t.Fatalf("目标键不符: %q", target)
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

// TestDirEntryOnlySnapshotConsumption 仅目录条目覆盖形态（无任何文件例外）的
// 快照消费断言（目录粒度表示·阶段3 新增锚）：目录条目供值的子树文件 Assigned
// 标记=就近解析非空（与旧展开形态「有条目」等值）、终选声明被目录条目维度覆
// 盖；无供值文件不标记；死路径判定按两类键分别落——文件键不在全量路径集、
// 目录键=目录不在快照目录树或子树无文件（空目录条目同样判死）；线视图按存储
// 原样直出目录条目（键带尾斜杠），不为被覆盖文件新造条目。
func TestDirEntryOnlySnapshotConsumption(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, map[string]SessionAssignment{
		// 唯一活赋值=目录条目（子树 3 文件全部仅由它覆盖）
		"画师目录/": {Author: "目录作者", Title: "目录标题", UpdatedAtMs: 5},
		// 死路径三形态：文件键路径不存在 / 目录键目录不在树 / 目录键子树无文件
		"不存在目录/旧文件.jpg": {Author: "死文件", UpdatedAtMs: 1},
		"不存在目录/":        {Author: "死目录", UpdatedAtMs: 2},
		"空目录/":          {Author: "空目录", UpdatedAtMs: 3},
	})
	writeTestFile(t, root+"/画师目录/pic_01.jpg", "a")
	writeTestFile(t, root+"/画师目录/pic_02.jpg", "b")
	writeTestFile(t, root+"/画师目录/嵌套/pic_01.jpg", "c")
	writeTestFile(t, root+"/无赋值/pic_01.jpg", "d")
	if err := os.MkdirAll(root+"/空目录", 0o755); err != nil {
		t.Fatalf("创建空目录失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	snap := sess.Snapshot
	byPath := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	authorOf := func(rel string) []string {
		var vals []string
		for _, d := range byPath[rel].Final {
			if d.Dimension == string(DimAuthor) {
				vals = append(vals, d.Value)
			}
		}
		return vals
	}
	// Assigned/终选声明：仅目录条目覆盖=已赋值（等值口径），author/title 维度
	// 由目录条目供值进终选
	for _, rel := range []string{
		"画师目录/pic_01.jpg", "画师目录/pic_02.jpg", "画师目录/嵌套/pic_01.jpg",
	} {
		f := byPath[rel]
		if !f.Assigned {
			t.Fatalf("%s 仅目录条目覆盖应带 assigned 标记", rel)
		}
		if got := authorOf(rel); len(got) != 1 || got[0] != "目录作者" {
			t.Fatalf("%s 终选 author = %v，期望仅 [目录作者]（目录条目供值）", rel, got)
		}
		var title string
		for _, d := range f.Final {
			if d.Dimension == string(DimTitle) {
				title = d.Value
			}
		}
		if title != "目录标题" {
			t.Fatalf("%s 终选 title = %q，期望 目录标题", rel, title)
		}
	}
	// 无供值文件：不标记、无作者声明
	if f := byPath["无赋值/pic_01.jpg"]; f.Assigned {
		t.Fatal("无供值文件不应带 assigned 标记")
	} else if got := authorOf("无赋值/pic_01.jpg"); len(got) != 0 {
		t.Fatalf("无供值文件不应有作者声明: %v", got)
	}
	// 死路径：三形态全部判死、活目录条目不判死（稳定序断言）
	wantStale := []string{"不存在目录/", "不存在目录/旧文件.jpg", "空目录/"}
	if len(snap.StaleAssignments) != len(wantStale) {
		t.Fatalf("死路径赋值报告不符: %+v", snap.StaleAssignments)
	}
	for i, key := range wantStale {
		if snap.StaleAssignments[i] != key {
			t.Fatalf("死路径赋值报告[%d] = %q，期望 %q（全量: %+v）", i, snap.StaleAssignments[i], key, snap.StaleAssignments)
		}
	}
	if snap.Stats.StaleAssignments != len(wantStale) {
		t.Fatalf("死路径计数 = %d，期望 %d", snap.Stats.StaleAssignments, len(wantStale))
	}
	// 线视图：按存储原样直出目录条目（键带尾斜杠），不为被覆盖文件新造条目
	view := decisionsViewOf(sess)
	entryByPath := make(map[string]workbenchAssignmentView, len(view.Assignments))
	for _, v := range view.Assignments {
		entryByPath[v.RelPath] = v
	}
	if entry, ok := entryByPath["画师目录/"]; !ok || entry.Author != "目录作者" {
		t.Fatalf("线视图应直出目录条目 画师目录/: %+v（在场 %v）", entry, ok)
	}
	if _, ok := entryByPath["画师目录/pic_01.jpg"]; ok {
		t.Fatal("线视图不应为仅目录条目覆盖的文件新造条目")
	}
}

// TestIncrementalReviveByAssignmentEntries 增量跳过修正的等值口径（阶段3 新增
// 锚）：mtime 未变目录被游标跳过后，赋值影响面整目录复活重算——目录条目供值
// 复活其子树全部目录（旧形态目录赋值展开为每文件条目=子树每目录都命中的等值
// 口径）、文件例外锚定其直属目录；无赋值影响面的跳过目录继续整目录复用旧结论。
func TestIncrementalReviveByAssignmentEntries(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	writeTestFile(t, root+"/目录A/f1.jpg", "a")
	writeTestFile(t, root+"/目录B/f2.jpg", "b")
	writeTestFile(t, root+"/目录C/f3.jpg", "c")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	// 原位注入赋值（不经公开写点——不递增修订号，保持增量资格，专测跳过
	// 集修正通路）：目录A=目录条目、目录B=直属文件例外、目录C=无赋值
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	sess.Assignments = map[string]SessionAssignment{
		"目录A/":       {Author: "目录作者", UpdatedAtMs: 2},
		"目录B/f2.jpg": {Author: "例外作者", UpdatedAtMs: 3},
	}
	if err := m.saveSession(sess); err != nil {
		t.Fatalf("注入会话赋值失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("增量识别失败: %v", err)
	}
	sess, err = m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	byPath := make(map[string]SnapFile, len(sess.Snapshot.Files))
	for _, f := range sess.Snapshot.Files {
		byPath[f.RelPath] = f
	}
	assertAuthor := func(rel, want string) {
		t.Helper()
		f := byPath[rel]
		if want == "" {
			if f.Assigned {
				t.Fatalf("%s 不应带 assigned 标记", rel)
			}
			for _, d := range f.Final {
				if d.Dimension == string(DimAuthor) {
					t.Fatalf("%s 不应有作者声明: %+v", rel, f.Final)
				}
			}
			return
		}
		if !f.Assigned {
			t.Fatalf("%s 应带 assigned 标记", rel)
		}
		var got []string
		for _, d := range f.Final {
			if d.Dimension == string(DimAuthor) {
				got = append(got, d.Value)
			}
		}
		if len(got) != 1 || got[0] != want {
			t.Fatalf("%s 终选 author = %v，期望 [%s]", rel, got, want)
		}
	}
	// 目录A 经祖先链目录条目复活：终选被目录条目覆盖；目录B 经直属文件例外
	// 复活；目录C 无赋值影响面：继续整目录复用（无作者声明）
	assertAuthor("目录A/f1.jpg", "目录作者")
	assertAuthor("目录B/f2.jpg", "例外作者")
	assertAuthor("目录C/f3.jpg", "")
}

// —— 欠计数护栏（枚举欠计数拒覆盖）——

// undercountFixtureTree 构造欠计数护栏测试树：根 3 文件 + ArtistB 8 png +
// ArtistA/SetX 12 jpg 共 23 文件（对齐真机事故合成库形态），返回全部相对路径
// （'/' 分隔，删除模拟半可见用）。
func undercountFixtureTree(t *testing.T, root string) []string {
	t.Helper()
	rels := make([]string, 0, 23)
	for i := 1; i <= 3; i++ {
		rel := fmt.Sprintf("根文件_%02d.jpg", i)
		writeTestFile(t, root+"/"+rel, "r")
		rels = append(rels, rel)
	}
	for i := 1; i <= 8; i++ {
		rel := fmt.Sprintf("ArtistB/b_%02d.png", i)
		writeTestFile(t, root+"/"+rel, "b")
		rels = append(rels, rel)
	}
	for i := 1; i <= 12; i++ {
		rel := fmt.Sprintf("ArtistA/SetX/a_%03d.jpg", i)
		writeTestFile(t, root+"/"+rel, "a")
		rels = append(rels, rel)
	}
	return rels
}

// undercountRemoveExcept 删除 keep 之外的全部测试文件（模拟移动盘半可见：
// 枚举只能看到幸存者）。
func undercountRemoveExcept(t *testing.T, root string, rels []string, keep string) {
	t.Helper()
	for _, rel := range rels {
		if rel == keep {
			continue
		}
		if err := os.Remove(root + "/" + rel); err != nil {
			t.Fatalf("删除文件 %s 失败: %v", rel, err)
		}
	}
}

// waitUndercountRecognitionIdle 等库根识别在跑标记清零（标记在启动入口
// 返回前同步置位；终态事件先于标记清位发布，清零后事件必已可读）。
func waitUndercountRecognitionIdle(t *testing.T, m *SessionManager, root string) {
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
			t.Fatal("识别干跑超时未完成")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// lastUndercountDoneEvent 取库根最近一条识别终态事件并解出载荷。
func lastUndercountDoneEvent(t *testing.T, ctx *workbenchTestCtx, root string) workbenchFrontendEvent {
	t.Helper()
	events := ctx.publishedSnapshot()
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].topic != workbenchTopicEvent {
			continue
		}
		var evt workbenchFrontendEvent
		if err := json.Unmarshal([]byte(events[i].data), &evt); err != nil {
			t.Fatalf("解析识别事件失败: %v", err)
		}
		if evt.Kind == workbenchEventRecognitionDone && evt.Root == root {
			return evt
		}
	}
	t.Fatalf("库根 %s 没有识别终态事件", root)
	return workbenchFrontendEvent{}
}

// undercountBlockedOf 读欠计数护栏放行簿记的当前位。
func undercountBlockedOf(m *SessionManager, root string) bool {
	m.recogMu.Lock()
	defer m.recogMu.Unlock()
	return m.undercountBlocked[root]
}

// snapshotTotalOf 读会话快照的文件计数（快照缺席即失败）。
func snapshotTotalOf(t *testing.T, m *SessionManager, root string) int {
	t.Helper()
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if sess.Snapshot == nil {
		t.Fatal("快照缺席")
	}
	return sess.Snapshot.Stats.TotalFiles
}

// TestUndercountGuardRejectsOverwrite 欠计数轮（上轮 23 文件 → 枚举 1）返回
// 护栏错误且会话快照未被覆盖：拒绝文案含两轮计数与二次点击指引，放行簿记
// 置位（任何来源的轮命中欠计数条件都拒绝）。
func TestUndercountGuardRejectsOverwrite(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	rels := undercountFixtureTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	if got := snapshotTotalOf(t, m, root); got != 23 {
		t.Fatalf("首轮快照文件数 = %d，期望 23", got)
	}

	undercountRemoveExcept(t, root, rels, "ArtistA/SetX/a_001.jpg")
	err := m.runFullRecognition(root)
	if err == nil || !strings.Contains(err.Error(), "欠计数护栏拒绝覆盖") {
		t.Fatalf("欠计数轮应返回护栏错误，实际: %v", err)
	}
	for _, want := range []string{"23", "1", "再次点击重新识别强制覆盖"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("拒绝文案应含 %q，实际: %v", want, err)
		}
	}
	if got := snapshotTotalOf(t, m, root); got != 23 {
		t.Fatalf("被拦后快照应保全为 23 文件，实际 %d（会话被覆盖）", got)
	}
	if !undercountBlockedOf(m, root) {
		t.Fatal("护栏拒绝后放行簿记应置位")
	}
}

// TestUndercountGuardUserRetryOverrides 用户入口二次触发放行：被拦后再走
// StartRecognition（用户动作入口）——簿记被消费、护栏跳过、快照照常覆盖，
// 完成消息附注跳过说明。
func TestUndercountGuardUserRetryOverrides(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)
	rels := undercountFixtureTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	undercountRemoveExcept(t, root, rels, "ArtistA/SetX/a_001.jpg")
	if err := m.runFullRecognition(root); err == nil {
		t.Fatal("欠计数轮应被护栏拒绝")
	}

	started, running, err := m.StartRecognition(root)
	if err != nil || !started || running {
		t.Fatalf("用户入口二次触发应启动新轮: started=%v running=%v err=%v", started, running, err)
	}
	waitUndercountRecognitionIdle(t, m, root)
	done := lastUndercountDoneEvent(t, ctx, root)
	if !done.Success {
		t.Fatalf("二次触发轮应放行成功，实际失败: %s", done.Message)
	}
	if !strings.Contains(done.Message, "本轮经再次触发跳过欠计数护栏") {
		t.Fatalf("完成消息应含跳过护栏附注: %s", done.Message)
	}
	if got := snapshotTotalOf(t, m, root); got != 1 {
		t.Fatalf("放行轮快照应按残缺枚举覆盖为 1 文件，实际 %d", got)
	}
	if undercountBlockedOf(m, root) {
		t.Fatal("用户入口消费后放行簿记应清位")
	}
}

// TestUndercountGuardAutoRefreshDoesNotConsumeOverride 自动链入口（刷新调度
// 路径）见簿记不消费、欠计数仍被拦：被拦后 scheduleRecognitionRefresh 启动
// 的自动轮再次被拦——快照保全、簿记仍在场。
func TestUndercountGuardAutoRefreshDoesNotConsumeOverride(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)
	rels := undercountFixtureTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	undercountRemoveExcept(t, root, rels, "ArtistA/SetX/a_001.jpg")
	if err := m.runFullRecognition(root); err == nil {
		t.Fatal("欠计数轮应被护栏拒绝")
	}

	if !m.scheduleRecognitionRefresh(root) {
		t.Fatal("自动刷新轮应已启动")
	}
	waitUndercountRecognitionIdle(t, m, root)
	done := lastUndercountDoneEvent(t, ctx, root)
	if done.Success {
		t.Fatalf("自动轮见簿记不消费、欠计数应仍被拦，实际成功: %s", done.Message)
	}
	if !strings.Contains(done.Message, "欠计数护栏拒绝覆盖") {
		t.Fatalf("自动轮失败文案应为护栏拒绝: %s", done.Message)
	}
	if got := snapshotTotalOf(t, m, root); got != 23 {
		t.Fatalf("自动轮被拦后快照应保全为 23 文件，实际 %d", got)
	}
	if !undercountBlockedOf(m, root) {
		t.Fatal("自动轮不消费放行簿记，簿记应仍在场")
	}
}

// TestUndercountGuardSkippedWithoutPrevSnapshot prev 为空不设防：首轮识别
//（无既有结论可保护）即使仅 1 文件也照常完成，簿记不置位。
func TestUndercountGuardSkippedWithoutPrevSnapshot(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	writeTestFile(t, root+"/仅此一件.jpg", "x")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮识别不应设防: %v", err)
	}
	if got := snapshotTotalOf(t, m, root); got != 1 {
		t.Fatalf("首轮快照文件数 = %d，期望 1", got)
	}
	if undercountBlockedOf(m, root) {
		t.Fatal("无快照可保护的轮不应置放行簿记")
	}
}

// TestUndercountGuardModerateShrinkPasses 中等收缩（约 -30%）不拦：23 → 16
// 文件（绝对损失 7 达下限、但未少过半），照常覆盖、簿记不置位。
func TestUndercountGuardModerateShrinkPasses(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	rels := undercountFixtureTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	for _, rel := range rels[:7] { // 根 3 + ArtistB 4，余 16（约 -30%）
		if err := os.Remove(root + "/" + rel); err != nil {
			t.Fatalf("删除文件 %s 失败: %v", rel, err)
		}
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("中等收缩不应被护栏拦截: %v", err)
	}
	if got := snapshotTotalOf(t, m, root); got != 16 {
		t.Fatalf("收缩轮快照文件数 = %d，期望 16", got)
	}
	if undercountBlockedOf(m, root) {
		t.Fatal("中等收缩轮不应置放行簿记")
	}
}

// TestApplyAssignmentToConclusionDropsAlsoRuleIDs 覆盖赋值覆盖对附属来源的
// 整体丢弃：显式维度清场替换后该维度只余赋值声明（无附属规则清单），
// 未覆盖维度的声明连同附属清单原样保留。
func TestApplyAssignmentToConclusionDropsAlsoRuleIDs(t *testing.T) {
	final := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "作者甲", Source: SourceDir, RuleID: "r1", AlsoRuleIDs: []string{"r2", "r3"}},
		{Dimension: DimTag, Value: "标签甲", Source: SourceFilename, RuleID: "r4", AlsoRuleIDs: []string{"r5"}},
	}
	got, _ := applyAssignmentToConclusion(final, nil, &SessionAssignment{Author: "修正作者"})
	var authorDecls, tagDecls []MetadataDeclaration
	for _, d := range got {
		switch d.Dimension {
		case DimAuthor:
			authorDecls = append(authorDecls, d)
		case DimTag:
			tagDecls = append(tagDecls, d)
		}
	}
	if len(authorDecls) != 1 || authorDecls[0].Source != SourceAssignment || authorDecls[0].Value != "修正作者" || len(authorDecls[0].AlsoRuleIDs) != 0 {
		t.Errorf("author 维应为无附属清单的赋值声明，实际 %+v", authorDecls)
	}
	if len(tagDecls) != 1 || tagDecls[0].Value != "标签甲" || len(tagDecls[0].AlsoRuleIDs) != 1 || tagDecls[0].AlsoRuleIDs[0] != "r5" {
		t.Errorf("未覆盖维应连同附属清单原样保留，实际 %+v", tagDecls)
	}
}

// TestConclusionKeyInsensitiveToAlsoRuleIDs 覆盖结论比较键口径：附属规则清单
// 不参与差异比较键，仅附属清单不同的两条结论键相等（差异轮零噪音）。
func TestConclusionKeyInsensitiveToAlsoRuleIDs(t *testing.T) {
	base := SnapConclusion{State: snapStateIdentified, Final: []SnapDecl{{
		Dimension: "author", Value: "作者甲", Source: "rule", RuleID: "r1",
	}}}
	withAlso := SnapConclusion{State: snapStateIdentified, Final: []SnapDecl{{
		Dimension: "author", Value: "作者甲", Source: "rule", RuleID: "r1",
		AlsoRuleIDs: []string{"r2", "r3"},
	}}}
	if conclusionKey(&base) != conclusionKey(&withAlso) {
		t.Errorf("仅附属规则清单不同的结论比较键应相等")
	}
}

// TestSnapDeclOfPassesAlsoRuleIDs 覆盖快照投影透传：声明的附属规则清单
// 原样进入紧凑形态。
func TestSnapDeclOfPassesAlsoRuleIDs(t *testing.T) {
	decl := MetadataDeclaration{
		Dimension: DimAuthor, Value: "作者甲", Source: SourceDir, RuleID: "r1",
		AlsoRuleIDs: []string{"r2"},
	}
	got := snapDeclOf(decl)
	if !reflect.DeepEqual(got.AlsoRuleIDs, []string{"r2"}) {
		t.Errorf("附属规则清单未透传，实际 %+v", got)
	}
}
