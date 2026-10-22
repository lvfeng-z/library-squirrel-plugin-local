package main

// 目录名分段提取 · 内核层锚定（剧本「内核层」节）：手写规则的位置模式目录名
// 正则可带命名捕获组、提取动作 FromCapture 可引用——ancestor 命中时的命名
// 捕获组绑入主解析上下文（与文件名捕获同空间）：
//   - 手写形态「第 2 层 `^\[(?P<a>[^\]]*)\] \[\d+\] \[(?P<t>[^\]]*)\]$` +
//     集名←capture t」在全库识别上生效（各自目录各自成集、跨目录同格式接管）；
//   - 同名冲突优先级断言：文件名捕获优先、ancestor 捕获仅补缺（And 两种
//     子模式顺序下文件名捕获恒为终值）；
//   - ancestor 捕获组计入 declaredPatternCaptures（条件动作可引用——存在性
//     校验放行）；
//   - 捕获组与目录/谓词命中按规则隔离（发现2 修复①+发现6 修复）：提取段每
//     规则在私有作用域内求值与提取——并存规则的同名捕获互不遮蔽（规则顺序
//     两向锚定）；FromAncestorDir 只读自身规则锚定产生的命中（「取最深」保留
//     在规则作用域内），跨规则共享累积废止（发现6：后求值浅层锚定规则读到
//     先求值规则的更深命中即垃圾值）。
import (
	"testing"
)

// TestLocationPatternAncestorCaptureHandwrittenForm 手写形态在全库识别上生效
// （剧本内核层锚定原文形态）：第 2 层 `[作者] [数字] [标题]` 格式目录 →
// 集名=第 3 段捕获、作者=第 1 段捕获；同格式他目录各自成集（跨目录接管）、
// 不同深度/不同格式目录不命中。
func TestLocationPatternAncestorCaptureHandwrittenForm(t *testing.T) {
	root := t.TempDir()
	ancestor := mustRegexPattern(t, `^\[(?P<a>[^\]]*)\] \[\d+\] \[(?P<t>[^\]]*)\]$`, []string{"a", "t"})
	location, err := NewLocationPattern(1, 1, ancestor)
	if err != nil {
		t.Fatalf("构造目录层位置模式失败: %v", err)
	}
	rule := RuleEntry{
		ID: "hand-dir-cap", Description: "手写：第 2 层目录 [作者] [数字] [标题] → 集名=第 3 段、作者=第 1 段",
		Source: RuleSourceManual, Enabled: true,
		Pattern: location,
		Extractions: []ExtractionAction{
			{Dimension: DimCollection, From: FromCapture, CaptureName: "t"},
			{Dimension: DimAuthor, From: FromCapture, CaptureName: "a"},
		},
	}
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	ctx.library = libraryFixture{}
	m := NewSessionManagerWithStore(ctx, nil, store)
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{rule}}); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "a")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/おまけ.jpg", "b")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242474] [別の題]/表紙.png", "c")
	writeTestFile(t, root+"/无关/散文件.png", "d")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	cases := []struct {
		relPath    string
		collection string
		author     string
	}{
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "瞬間的に絡み合う", "Isel"},
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/おまけ.jpg", "瞬間的に絡み合う", "Isel"},
		// 同格式他目录各自成集：集名取各自目录第 3 段、作者取各自段 1。
		{"Artist[Isel]/[Isel] [92242474] [別の題]/表紙.png", "別の題", "Isel"},
	}
	for _, c := range cases {
		file := snapshotFileOf(sess.Snapshot, c.relPath)
		if file == nil {
			t.Fatalf("快照内无文件 %s", c.relPath)
		}
		var collection, author string
		for _, decl := range file.Final {
			if decl.RuleID != rule.ID {
				continue
			}
			switch MetadataDimension(decl.Dimension) {
			case DimCollection:
				collection = decl.Value
			case DimAuthor:
				author = decl.Value
			}
		}
		if collection != c.collection || author != c.author {
			t.Fatalf("文件 %s 的捕获提取不符：集名 %q（期望 %q）、作者 %q（期望 %q）", c.relPath, collection, c.collection, author, c.author)
		}
	}
	// 不同格式/不同层深的目录不命中。
	if file := snapshotFileOf(sess.Snapshot, "无关/散文件.png"); file == nil {
		t.Fatal("快照内无控制文件")
	} else {
		for _, decl := range file.Final {
			if decl.RuleID == rule.ID {
				t.Fatalf("控制文件不应命中手写目录规则：%+v", decl)
			}
		}
	}
}

// TestLocationPatternAncestorCaptureConflictPriority 同名冲突优先级断言：
// 文件名捕获优先、ancestor 捕获仅补缺——And[文件名正则, 位置模式] 与
// And[位置模式, 文件名正则] 两种求值顺序下，同名捕获组的终值恒为文件名侧
// 命中值（ancestor 命中值被并入时跳过）；无文件名侧绑定时 ancestor 补缺。
func TestLocationPatternAncestorCaptureConflictPriority(t *testing.T) {
	fileRe := mustRegexPattern(t, `^(?P<x>FILE)_\d+`, []string{"x"})
	dirRe := mustRegexPattern(t, `^(?P<x>DIR\w+)$`, []string{"x"})
	location, err := NewLocationPattern(0, 0, dirRe)
	if err != nil {
		t.Fatalf("构造目录层位置模式失败: %v", err)
	}
	in := &MatchInput{FileName: "FILE_01.png", DirNames: []string{"DIRBOX"}, RelPath: "DIRBOX/FILE_01.png"}

	// 顺序一：文件名正则在位置模式之前。
	ctx := &ParseContext{}
	if !(And{fileRe, location}).Evaluate(in, nil, ctx) {
		t.Fatal("组合模式应命中")
	}
	if v, ok := ctx.Capture("x"); !ok || v != "FILE" {
		t.Fatalf("文件名捕获应优先：got %q ok=%v", v, ok)
	}
	// 顺序二：位置模式在文件名正则之前。
	ctx2 := &ParseContext{}
	if !(And{location, fileRe}).Evaluate(in, nil, ctx2) {
		t.Fatal("组合模式应命中")
	}
	if v, ok := ctx2.Capture("x"); !ok || v != "FILE" {
		t.Fatalf("求值顺序不应影响优先级（文件名捕获恒为终值）：got %q ok=%v", v, ok)
	}
	// 补缺路径：无文件名侧绑定时 ancestor 捕获可取。
	ctx3 := &ParseContext{}
	if !location.Evaluate(in, nil, ctx3) {
		t.Fatal("位置模式应命中")
	}
	if v, ok := ctx3.Capture("x"); !ok || v != "DIRBOX" {
		t.Fatalf("ancestor 捕获应补缺绑入：got %q ok=%v", v, ok)
	}
	// 声明集：ancestor 捕获组计入（When 条件可引用）。
	if declared := declaredPatternCaptures(location); !declared["x"] {
		t.Fatalf("ancestor 捕获组应计入声明集: %v", declared)
	}
}

// mustDirCaptureRule 构造「第 depth+1 层目录 `[任意] [数字]` → 集名=第 1 段
// 捕获」的目录格式规则（捕获组名与传入一致——同组名并存即遮蔽场景）。
func mustDirCaptureRule(t *testing.T, id string, depth int, group string) RuleEntry {
	t.Helper()
	ancestor := mustRegexPattern(t, `^\[(?P<`+group+`>[^\]]*)\] \[\d+\]$`, []string{group})
	location, err := NewLocationPattern(depth, depth, ancestor)
	if err != nil {
		t.Fatalf("构造目录层位置模式失败: %v", err)
	}
	return RuleEntry{
		ID: id, Description: "测试：目录格式捕获规则 " + id, Source: RuleSourceManual, Enabled: true,
		Pattern: location,
		Extractions: []ExtractionAction{
			{Dimension: DimCollection, From: FromCapture, CaptureName: group},
		},
	}
}

// TestExtractionStageIsolatesCapturesPerRule 捕获组与目录/谓词命中按规则隔离
// （发现2 修复① + 发现6 修复）：并存规则各带同名命名捕获组（两个不同集各带
// (?P<collection>…) 的目录格式规则、文件名捕获规则与目录格式规则并存）时，
// 每规则的提取只取自身捕获——共享单上下文时代「先绑入者胜」令后入库规则的
// 捕获被占位、提取恒取他规则之值（规则顺序两个方向都锚定）；FromAncestorDir
// 只读自身规则锚定产生的目录命中——跨规则共享累积时代后求值的浅层锚定规则
// 会读到先求值规则的更深命中、产出他目录剥壳的垃圾值（发现6：族A setdir 锚
// L2 × 族B 壳形作者锚 L1）；「取最深」语义保留在规则作用域内（规则内多模式
// 锚定取该规则内最深命中）。
func TestExtractionStageIsolatesCapturesPerRule(t *testing.T) {
	run := func(rules []RuleEntry, in *MatchInput) []MetadataDeclaration {
		t.Helper()
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		decision := &FileDecision{RelPath: in.RelPath, FileName: in.FileName}
		runExtractionStage(decision, rules, in, nil, ctx)
		return decision.Declarations
	}
	collectionOf := func(decls []MetadataDeclaration, ruleID string) (string, bool) {
		t.Helper()
		for i := range decls {
			if decls[i].RuleID == ruleID && decls[i].Dimension == DimCollection {
				return decls[i].Value, true
			}
		}
		return "", false
	}

	// 场景一：文件名捕获规则与目录格式规则同组名并存（泛化后再调整的实机
	// 形态）——两个规则顺序方向各自声明其捕获值。
	fileRule := RuleEntry{
		ID: "file-cap", Description: "测试：文件名捕获规则", Source: RuleSourceManual, Enabled: true,
		Pattern:     mustRegexPattern(t, `^(?P<collection>[a-z]+)_\d+\.png$`, []string{"collection"}),
		Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "collection"}},
	}
	dirRule := mustDirCaptureRule(t, "dir-cap", 0, "collection")
	in := &MatchInput{FileName: "beta_02.png", DirNames: []string{"[Alpha] [123]"}, RelPath: "[Alpha] [123]/beta_02.png"}
	for _, order := range [][]RuleEntry{{fileRule, dirRule}, {dirRule, fileRule}} {
		decls := run(order, in)
		if v, ok := collectionOf(decls, "file-cap"); !ok || v != "beta" {
			t.Fatalf("文件名规则的集名声明应为自身捕获 beta：got %q ok=%v（规则顺序被遮蔽的共享 ctx 时代会取他规则值）", v, ok)
		}
		if v, ok := collectionOf(decls, "dir-cap"); !ok || v != "Alpha" {
			t.Fatalf("目录格式规则的集名声明应为自身捕获 Alpha：got %q ok=%v", v, ok)
		}
	}

	// 场景二：两个不同层深的目录格式规则同组名并存（两个不同集各带
	// (?P<collection>…)）——各自取各自目录层段值，先入库者不占位。
	l0 := mustDirCaptureRule(t, "dir-l0", 0, "collection")
	l1 := mustDirCaptureRule(t, "dir-l1", 1, "collection")
	in2 := &MatchInput{FileName: "x.png", DirNames: []string{"[Alpha] [123]", "[Gamma] [456]"}, RelPath: "[Alpha] [123]/[Gamma] [456]/x.png"}
	for _, order := range [][]RuleEntry{{l0, l1}, {l1, l0}} {
		decls := run(order, in2)
		if v, ok := collectionOf(decls, "dir-l0"); !ok || v != "Alpha" {
			t.Fatalf("第 1 层规则的集名声明应为 Alpha：got %q ok=%v", v, ok)
		}
		if v, ok := collectionOf(decls, "dir-l1"); !ok || v != "Gamma" {
			t.Fatalf("第 2 层规则的集名声明应为 Gamma：got %q ok=%v", v, ok)
		}
	}

	// 场景三（发现6 复现）：族A 目录格式规则（锚第 2 层、集名捕获）× 族B 壳形
	// 作者规则（锚第 1 层、FromAncestorDir 剥壳）双族并存——两个规则顺序方向
	// 下族B 作者恒为各自 Artist 内名，不读族A 的第 2 层命中（共享累积时代
	// 族A 先求值即污染族B 为「[Isel] [92242473] [瞬間的に絡み合う」垃圾值）。
	ancestor3 := mustRegexPattern(t, `^\[[^\]]*\] \[\d+\] \[(?P<collection>[^\]]*)\]$`, []string{"collection"})
	setdirLoc, err := NewLocationPattern(1, 1, ancestor3)
	if err != nil {
		t.Fatalf("构造目录格式位置模式失败: %v", err)
	}
	familyA := RuleEntry{
		ID: "setdir-l2", Description: "测试：族A 目录格式（锚 L2）", Source: RuleSourceManual, Enabled: true,
		Pattern:     setdirLoc,
		Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "collection"}},
	}
	shellLoc, err := NewLocationPattern(0, 0, mustRegexPattern(t, `^Artist\[[^\]]*\]$`, nil))
	if err != nil {
		t.Fatalf("构造壳形位置模式失败: %v", err)
	}
	familyB := RuleEntry{
		ID: "shell-l1", Description: "测试：族B 壳形作者（锚 L1）", Source: RuleSourceManual, Enabled: true,
		Pattern: shellLoc,
		Extractions: []ExtractionAction{
			{Dimension: DimAuthor, From: FromAncestorDir, StripPrefix: "Artist[", StripSuffix: "]"},
		},
	}
	in3 := &MatchInput{
		FileName: "表紙.png",
		DirNames: []string{"Artist[Isel]", "[Isel] [92242473] [瞬間的に絡み合う]"},
		RelPath:  "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png",
	}
	for _, order := range [][]RuleEntry{{familyA, familyB}, {familyB, familyA}} {
		decls := run(order, in3)
		author := ""
		for i := range decls {
			if decls[i].RuleID == "shell-l1" && decls[i].Dimension == DimAuthor {
				author = decls[i].Value
			}
		}
		if author != "Isel" {
			t.Fatalf("族B 作者应为自身第 1 层锚定的剥壳值 Isel（读到他目录命中即垃圾值污染）：got %q", author)
		}
		if v, ok := collectionOf(decls, "setdir-l2"); !ok || v != "瞬間的に絡み合う" {
			t.Fatalf("族A 集名应为自身第 2 层捕获：got %q ok=%v", v, ok)
		}
	}

	// 场景四：FromAncestorDir 规则内锚定仍可用（「取最深」保留在规则作用域
	// 内）——同一规则 And{目录锚定, 文件名} 的 FromAncestorDir 取自身命中；
	// 跨规则不再共享：他规则的锚定命中不为本规则的 FromAncestorDir 所见。
	anchorLoc, err := NewLocationPattern(0, 0, mustRegexPattern(t, `^\[Alpha\] \[\d+\]$`, nil))
	if err != nil {
		t.Fatalf("构造锚定位置模式失败: %v", err)
	}
	selfAnchored := RuleEntry{
		ID: "self-anchored", Description: "测试：规则内锚定+FromAncestorDir", Source: RuleSourceManual, Enabled: true,
		Pattern:     And{anchorLoc, mustRegexPattern(t, `^beta_02\.png$`, nil)},
		Extractions: []ExtractionAction{{Dimension: DimTag, From: FromAncestorDir}},
	}
	decls := run([]RuleEntry{selfAnchored}, in)
	found := false
	for i := range decls {
		if decls[i].RuleID == "self-anchored" && decls[i].Dimension == DimTag && decls[i].Value == "[Alpha] [123]" {
			found = true
		}
	}
	if !found {
		t.Fatalf("规则内锚定的 FromAncestorDir 应取自身目录命中：decls=%+v", decls)
	}
	anchor := RuleEntry{ID: "anchor", Description: "测试：目录锚定（无提取）", Source: RuleSourceManual, Enabled: true, Pattern: anchorLoc}
	consumer := RuleEntry{
		ID: "consumer", Description: "测试：FromAncestorDir 消费（无自身锚定）", Source: RuleSourceManual, Enabled: true,
		Pattern:     mustRegexPattern(t, `^beta_02\.png$`, nil),
		Extractions: []ExtractionAction{{Dimension: DimTag, From: FromAncestorDir}},
	}
	for _, order := range [][]RuleEntry{{anchor, consumer}, {consumer, anchor}} {
		decls := run(order, in)
		for i := range decls {
			if decls[i].RuleID == "consumer" {
				t.Fatalf("无自身锚定的 FromAncestorDir 不应读到他规则的目录命中（跨规则共享已废止）：decls=%+v", decls)
			}
		}
	}
}
