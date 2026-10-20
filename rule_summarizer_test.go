package main

// 规则总结器的单元测试：真实样例的组合规则构造（根名→标签 / 一级目录剥壳→
// 作者 / 文件名首段→作品名 / 尾部码丢弃）、手填值自动匹配兜底与手工维度降级、
// 剥壳变换的求值、全库命中预览统计与跨目录样例（含目录键）、赋值出处存取
//（会话态+协议透传）、summarizeRule 不写档与 generalize mode=struct 确认入档
//（单文件与目录寻址）。
import (
	"strings"
	"testing"
)

// realSampleStructFixture 剧本G 真实样例的三件输入。
func realSampleStructFixture() (root, rel, file string) {
	return "E:/libs/big belly",
		"Artist[Jake--Jaccobus]/atago__commission__by_jake__jaccobus_delithw.png",
		"atago__commission__by_jake__jaccobus_delithw.png"
}

// TestBuildStructMapRulesRealSample 退出标准样例：根名→标签（常量折入目录条目）、
// 一级目录剥壳→作者（MinDepth=0 锚 + Artist[任意] 泛化锚 + 剥壳变换）、文件名
// 首段→作品名（分隔段捕获）、尾部码丢弃（人话提示 + 正则只消费不捕获）。
func TestBuildStructMapRulesRealSample(t *testing.T) {
	root, rel, file := realSampleStructFixture()
	assign := &SessionAssignment{
		Author: "Jake--Jaccobus", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
		Tags: []string{"big belly"}, TagClueRefs: []string{"rootName"},
	}
	sugg, rules, err := buildStructMapRules(root, rel, file, assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 2 || len(sugg.Rules) != 2 {
		t.Fatalf("组合规则应恰两条（目录层+文件名），实际 %d 条", len(rules))
	}

	// 目录层条目：MinDepth=MaxDepth=0、锚 ^Artist\[[^\]]*\]$、作者=剥壳 ancestorDir、
	// 标签=库根名常量。
	dirRule, ok := rules[0].Pattern.(*LocationPattern)
	if !ok {
		t.Fatalf("首条应为位置模式，实际 %T", rules[0].Pattern)
	}
	if dirRule.MinDepth != 0 || dirRule.MaxDepth != 0 {
		t.Fatalf("目录层锚应 MinDepth=MaxDepth=0，实际 %d-%d", dirRule.MinDepth, dirRule.MaxDepth)
	}
	anchor, ok := dirRule.Ancestor.(*RegexPattern)
	if !ok || anchor.Expr != `^Artist\[[^\]]*\]$` {
		t.Fatalf("剥壳泛化锚不符: %+v", dirRule.Ancestor)
	}
	if len(rules[0].Extractions) != 2 {
		t.Fatalf("目录条目提取应两维（作者+标签），实际 %d", len(rules[0].Extractions))
	}
	auth := rules[0].Extractions[0]
	if auth.Dimension != DimAuthor || auth.From != FromAncestorDir ||
		auth.StripPrefix != "Artist[" || auth.StripSuffix != "]" {
		t.Fatalf("作者提取应为 ancestorDir+剥壳: %+v", auth)
	}
	tagAct := rules[0].Extractions[1]
	if tagAct.Dimension != DimTag || tagAct.From != FromConstant || tagAct.Constant != "big belly" {
		t.Fatalf("标签提取应为库根名常量: %+v", tagAct)
	}

	// 文件名条目：首段捕获、尾部码只消费不捕获。
	fileRule, ok := rules[1].Pattern.(*RegexPattern)
	if !ok || fileRule.Target != TargetFileName {
		t.Fatalf("次条应为文件名正则模式，实际 %T", rules[1].Pattern)
	}
	wantExpr := `^(?P<title>[^_].*?)(?:__.*)?(?:\.[^.]*)?$`
	if fileRule.Expr != wantExpr {
		t.Fatalf("文件名形态正则不符:\n取回 %s\n期望 %s", fileRule.Expr, wantExpr)
	}
	if len(rules[1].Extractions) != 1 || rules[1].Extractions[0].Dimension != DimTitle ||
		rules[1].Extractions[0].From != FromCapture || rules[1].Extractions[0].CaptureName != "title" {
		t.Fatalf("作品名提取应为首段捕获: %+v", rules[1].Extractions)
	}

	// 人话规则与映射。
	for _, piece := range []string{"一级目录", "Artist[任意]", "剥壳→作者", "文件名首段→作品名", "库根名→标签", "尾部码丢弃"} {
		if !strings.Contains(sugg.Summary, piece) {
			t.Fatalf("人话规则缺片段 %q: %q", piece, sugg.Summary)
		}
	}
	if !sugg.DropsTailCode || len(sugg.ManualDims) != 0 {
		t.Fatalf("尾部码标记/手工维度不符: %+v", sugg)
	}
	if len(sugg.Mappings) != 3 {
		t.Fatalf("映射说明应三条，实际 %v", sugg.Mappings)
	}
}

// TestBuildStructMapRulesManualFallback 手填兜底：无显式出处时值=某段自动匹配
// （作者=剥壳内名、作品名=首段）；不对应任何段的值降级「手工指定（不参与泛化）」。
func TestBuildStructMapRulesManualFallback(t *testing.T) {
	root, rel, file := realSampleStructFixture()
	assign := &SessionAssignment{
		Author:     "Jake--Jaccobus", // 自动匹配 dirLevelStripped1
		Title:      "atago",          // 自动匹配 filenameSeg0
		Collection: "手填集",            // 无对应段
	}
	sugg, rules, err := buildStructMapRules(root, rel, file, assign)
	if err != nil {
		t.Fatalf("构造失败: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("应构造目录+文件名两条，实际 %d", len(rules))
	}
	if len(sugg.ManualDims) != 1 || !strings.Contains(sugg.ManualDims[0], "手填集") ||
		!strings.Contains(sugg.ManualDims[0], "手工指定") {
		t.Fatalf("作品集应降级手工指定: %v", sugg.ManualDims)
	}
	if strings.Contains(sugg.Summary, "作品集") || strings.Contains(sugg.Summary, "手填集") {
		t.Fatalf("手工维度不应入人话规则: %q", sugg.Summary)
	}
	// 目录条目不含作品集常量
	for _, act := range rules[0].Extractions {
		if act.Dimension == DimCollection {
			t.Fatalf("手工维度不应烧入规则: %+v", act)
		}
	}
}

// TestBuildStructMapRulesBracketUnderscore 括号组 + 单 _ 分隔（v2 构造）：目录层
// 剥壳条目与文件名条目齐备；文件名条目按实际分段形态构造（括号段字面包裹消费、
// [^_] 段类、末段后吞余部与扩展名）；同形异值文件上以样例输入直接求值，作者=
// 目录剥壳、作品名=第 3 段捕获均复现。
func TestBuildStructMapRulesBracketUnderscore(t *testing.T) {
	rel := "Artist[路灯后辈]/[路灯后辈]_[136715041]_无题_[01].png"
	file := "[路灯后辈]_[136715041]_无题_[01].png"
	assign := &SessionAssignment{
		Author: "路灯后辈", AuthorClueRef: "dirLevelStripped1",
		Title: "无题", TitleClueRef: "filenameSeg2",
	}
	sugg, rules, err := buildStructMapRules("E:/libs/图库", rel, file, assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 2 || len(sugg.Rules) != 2 {
		t.Fatalf("组合规则应恰两条（目录层+文件名），实际 %d 条", len(rules))
	}
	dirRule, ok := rules[0].Pattern.(*LocationPattern)
	if !ok {
		t.Fatalf("首条应为位置模式，实际 %T", rules[0].Pattern)
	}
	fileRule, ok := rules[1].Pattern.(*RegexPattern)
	if !ok || fileRule.Target != TargetFileName {
		t.Fatalf("次条应为文件名正则模式，实际 %T", rules[1].Pattern)
	}
	// 文件名形态：前两括号段字面消费（段类排除 ] 与 _）+ 第 3 段捕获（可选括号
	// 包裹 + 段类排除括号与 _）+ 吞余部。
	wantExpr := `^\[[^\]_]*\]_\[[^\]_]*\]_(?:[\[【（\(「『])?(?P<title>[^\[【（\(「『\]】）\)」』_]+?)(?:[\]】）\)」』])?(?:_.*)?(?:\.[^.]*)?$`
	if fileRule.Expr != wantExpr {
		t.Fatalf("文件名形态正则不符:\n取回 %s\n期望 %s", fileRule.Expr, wantExpr)
	}

	// 来源标签（剥壳标签无圆括号形态）与人话措辞（首段/第 N 段措辞不变）。
	fromBy := map[string]string{}
	for _, m := range sugg.Mappings {
		fromBy[m.Dimension] = m.From
	}
	if fromBy["author"] != "一级目录剥壳" || fromBy["title"] != "文件名第3段" {
		t.Fatalf("来源标签不符: %v", sugg.Mappings)
	}
	for _, piece := range []string{"Artist[任意]剥壳→作者", "文件名第3段→作品名"} {
		if !strings.Contains(sugg.Summary, piece) {
			t.Fatalf("人话规则缺片段 %q: %q", piece, sugg.Summary)
		}
	}

	// 同形异值复现：样例输入构造 MatchInput 直接求值（author=目录剥壳、title=捕获）。
	sameShape := "[X]_[9527]_[标题]_[02].png"
	in := &MatchInput{
		FileName: sameShape,
		DirNames: []string{"Artist[X]"},
		RelPath:  "Artist[X]/" + sameShape,
	}
	ctxFile := &ParseContext{}
	if !fileRule.Evaluate(in, nil, ctxFile) {
		t.Fatal("同形异值文件应命中文件名条目")
	}
	decl, ok := rules[1].Extractions[0].Apply(rules[1].ID, ctxFile)
	if !ok || decl.Dimension != DimTitle || decl.Value != "标题" {
		t.Fatalf("同形异值作品名捕获不符: %+v", decl)
	}
	ctxDir := &ParseContext{}
	if !dirRule.Evaluate(in, nil, ctxDir) {
		t.Fatal("同形异值目录应命中目录层条目")
	}
	decl, ok = rules[0].Extractions[0].Apply(rules[0].ID, ctxDir)
	if !ok || decl.Dimension != DimAuthor || decl.Value != "X" {
		t.Fatalf("同形异值作者剥壳取值不符: %+v", decl)
	}
}

// TestBuildStructMapRulesUnderscoreManualDemotion 纯 _ 模式的段值降级：目录名
// foo_bar 作提示名在段间正文内跨分隔符保持整段，其段值含活跃分隔符 _ → 该维度
// 手工（ManualDims 收录、不生成捕获），同文件其余段捕获不受牵连。
func TestBuildStructMapRulesUnderscoreManualDemotion(t *testing.T) {
	rel := "foo_bar/[a]_x_foo_bar_y_[01].png"
	file := "[a]_x_foo_bar_y_[01].png"
	assign := &SessionAssignment{
		Author: "x", AuthorClueRef: "filenameSeg1",
		Title: "foo_bar", TitleClueRef: "filenameSeg2",
	}
	sugg, rules, err := buildStructMapRules("E:/libs/下划线库", rel, file, assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("应仅文件名一条（目录层无赋值），实际 %d 条", len(rules))
	}
	if len(sugg.ManualDims) != 1 || !strings.Contains(sugg.ManualDims[0], "foo_bar") ||
		!strings.Contains(sugg.ManualDims[0], "手工指定") {
		t.Fatalf("作品名应降级手工指定: %v", sugg.ManualDims)
	}
	fileRule, ok := rules[0].Pattern.(*RegexPattern)
	if !ok {
		t.Fatalf("应为文件名正则模式，实际 %T", rules[0].Pattern)
	}
	wantExpr := `^\[[^\]_]*\]_(?:[\[【（\(「『])?(?P<author>[^\[【（\(「『\]】）\)」』_]+?)(?:[\]】）\)」』])?(?:_.*)?(?:\.[^.]*)?$`
	if fileRule.Expr != wantExpr {
		t.Fatalf("文件名形态正则不符:\n取回 %s\n期望 %s", fileRule.Expr, wantExpr)
	}
	if len(rules[0].Extractions) != 1 || rules[0].Extractions[0].Dimension != DimAuthor ||
		rules[0].Extractions[0].From != FromCapture || rules[0].Extractions[0].CaptureName != "author" {
		t.Fatalf("降级维度不应生成对应捕获: %+v", rules[0].Extractions)
	}
	fromBy := map[string]string{}
	for _, m := range sugg.Mappings {
		fromBy[m.Dimension] = m.From
	}
	if fromBy["author"] != "文件名第2段" || fromBy["title"] != "手工指定" {
		t.Fatalf("映射来源不符: %v", sugg.Mappings)
	}
}

// TestExtractionActionStripTransform 剥壳变换求值：ancestorDir 全名剥为内名；
// 旧形态（双空字段）行为不变；值不具包裹时为无操作。
func TestExtractionActionStripTransform(t *testing.T) {
	ctx := &ParseContext{DirHits: []DirHit{{Depth: 0, Name: "Artist[Jake--Jaccobus]"}}}
	decl, ok := ExtractionAction{
		Dimension: DimAuthor, From: FromAncestorDir, StripPrefix: "Artist[", StripSuffix: "]",
	}.Apply("r", ctx)
	if !ok || decl.Value != "Jake--Jaccobus" {
		t.Fatalf("剥壳取值不符: %+v", decl)
	}
	decl, ok = ExtractionAction{Dimension: DimAuthor, From: FromAncestorDir}.Apply("r", ctx)
	if !ok || decl.Value != "Artist[Jake--Jaccobus]" {
		t.Fatalf("无剥壳字段应保持全名（旧行为不变）: %+v", decl)
	}
	decl, ok = ExtractionAction{
		Dimension: DimAuthor, From: FromAncestorDir, StripPrefix: "Artist[", StripSuffix: "]",
	}.Apply("r", &ParseContext{DirHits: []DirHit{{Depth: 0, Name: "普通目录"}}})
	if !ok || decl.Value != "普通目录" {
		t.Fatalf("值不具包裹时应无操作: %+v", decl)
	}
}

// TestStructSummaryPreviewAndSessionRoundTrip 会话级链路：赋值出处入会话存储并
// 回透视图、summarizeRule 建议含命中预览与样例且不写档案、getPanorama 透出
// 结构线索集、generalize mode=struct 确认后组合规则入档（剥壳字段经档案
// 序列化往返）。
func TestStructSummaryPreviewAndSessionRoundTrip(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	_, rel, _ := realSampleStructFixture()
	writeTestFile(t, root+"/"+rel, "a")
	writeTestFile(t, root+"/Artist[Jake--Jaccobus]/2b__comm__by_jake__jaccobus_dhecdru.png", "b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	// 赋值带出处：会话存储 + 视图回透。
	sess, view, err := m.ApplyAssignment(root, rel, SessionAssignment{
		Author: "Jake--Jaccobus", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
		Tags: []string{"big belly"}, TagClueRefs: []string{"rootName"},
	})
	if err != nil {
		t.Fatalf("赋值失败: %v", err)
	}
	if stored, ok := sess.Assignments[rel]; !ok || stored.AuthorClueRef != "dirLevelStripped1" ||
		stored.TitleClueRef != "filenameSeg0" || len(stored.TagClueRefs) != 1 || stored.TagClueRefs[0] != "rootName" {
		t.Fatalf("赋值出处未入会话存储: %+v", stored)
	}
	if view.AuthorClueRef != "dirLevelStripped1" || view.TitleClueRef != "filenameSeg0" ||
		len(view.TagClueRefs) != 1 || view.TagClueRefs[0] != "rootName" {
		t.Fatalf("赋值视图未回透出处: %+v", view)
	}

	// summarizeRule：建议卡（命中预览+样例），不写档案。
	profileBefore, _ := LoadProfile(m.store)
	rulesBefore := 0
	if profileBefore != nil {
		rulesBefore = len(profileBefore.Rules)
	}
	sugg, err := m.ApplyStructSummary(root, rel)
	if err != nil {
		t.Fatalf("规则总结失败: %v", err)
	}
	if sugg.HitTotal != 2 || sugg.HitCount != 1 {
		t.Fatalf("命中预览应=同目录其余 1 个（共 2），实际 %d/%d", sugg.HitCount, sugg.HitTotal)
	}
	if len(sugg.Samples) != 1 || !strings.Contains(sugg.Samples[0].FileName, "2b__") ||
		!strings.Contains(sugg.Samples[0].Result, "作者=Jake--Jaccobus") ||
		!strings.Contains(sugg.Samples[0].Result, "作品名=2b") {
		t.Fatalf("样例应用不符: %+v", sugg.Samples)
	}
	profileAfter, _ := LoadProfile(m.store)
	rulesAfter := 0
	if profileAfter != nil {
		rulesAfter = len(profileAfter.Rules)
	}
	if rulesAfter != rulesBefore {
		t.Fatalf("summarizeRule 不应写档案：前 %d 后 %d", rulesBefore, rulesAfter)
	}

	// getPanorama：文件级结构线索集透出。
	pano := panoramaViewOf(sess.Snapshot)
	rec := snapshotFileOf(&pano, rel)
	if rec == nil {
		t.Fatal("全景缺样例文件记录")
	}
	if c, ok := clueByRef(rec.StructClues, "dirLevelStripped1"); !ok || c.Value != "Jake--Jaccobus" {
		t.Fatalf("全景应透出剥壳线索: %v", rec.StructClues)
	}
	if c, ok := clueByRef(rec.StructClues, "tailCode"); !ok || c.Value != "delithw" {
		t.Fatalf("全景应透出尾部码线索: %v", rec.StructClues)
	}

	// generalize mode=struct：确认入档（组合规则两条，剥壳字段序列化往返）。
	result, err := m.ApplyGeneralization(root, rel, generalizeModeStruct)
	if err != nil {
		t.Fatalf("结构映射泛化失败: %v", err)
	}
	if result.Rule == nil || len(result.Rules) != 2 || !result.VerificationStarted {
		t.Fatalf("泛化结果形态不符: %+v", result)
	}
	profile, errs := LoadProfile(m.store)
	if len(errs) > 0 || profile == nil {
		t.Fatalf("载入档案失败: %v", errs)
	}
	found, stripped := 0, false
	for i := range profile.Rules {
		for _, act := range profile.Rules[i].Extractions {
			if act.StripPrefix == "Artist[" && act.StripSuffix == "]" && act.Dimension == DimAuthor {
				stripped = true
			}
		}
		for _, r := range result.Rules {
			if profile.Rules[i].ID == r.ID {
				found++
			}
		}
	}
	if found != 2 || !stripped {
		t.Fatalf("组合规则应两条入档且剥壳字段往返（入档 %d、剥壳 %v）", found, stripped)
	}
}

// TestPreviewStructRulesAcrossDirs 全库命中预览：HitTotal=全库文件总数、
// HitCount=除样本外被组合规则完整命中的文件数（跨目录，剥壳泛化锚命中同形
// 异名目录）；样例取跨目录前 3 个并附目录键（DirRelKey）。
func TestPreviewStructRulesAcrossDirs(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	writeTestFile(t, root+"/Artist[J]/2b__y_dhecdru.png", "b")
	writeTestFile(t, root+"/Artist[J]/atago__x_delithw.png", "a")
	writeTestFile(t, root+"/Artist[K]/3c__z_code2.png", "c")
	writeTestFile(t, root+"/其他/root.jpg", "d")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	rel := "Artist[J]/atago__x_delithw.png"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{
		Author: "J", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
	}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	sugg, err := m.ApplyStructSummary(root, rel)
	if err != nil {
		t.Fatalf("规则总结失败: %v", err)
	}
	if sugg.HitTotal != 4 {
		t.Fatalf("全库文件总数应 = 4，实际 %d", sugg.HitTotal)
	}
	if sugg.HitCount != 2 {
		t.Fatalf("除样本外命中数应 = 2（2b 与跨目录 3c），实际 %d", sugg.HitCount)
	}
	if len(sugg.Samples) != 2 {
		t.Fatalf("样例应 2 条（其余命中文件），实际 %+v", sugg.Samples)
	}
	byDir := map[string]RuleSuggestionSample{}
	for _, s := range sugg.Samples {
		byDir[s.DirRelKey] = s
	}
	if s, ok := byDir["Artist[J]"]; !ok || !strings.Contains(s.FileName, "2b__") ||
		!strings.Contains(s.Result, "作品名=2b") {
		t.Fatalf("同目录样例不符: %+v", s)
	}
	if s, ok := byDir["Artist[K]"]; !ok || !strings.Contains(s.FileName, "3c__") ||
		!strings.Contains(s.Result, "作者=K") || !strings.Contains(s.Result, "作品名=3c") {
		t.Fatalf("跨目录样例应带目录键且求值按各自目录取值: %+v", s)
	}
}

// TestDirSummarizeRuleAddressing 目录寻址总结：样本=子树内首个有赋值的文件、
// 建议卡 RelPath 呈现目录键；子树无赋值文件回可读错误。
func TestDirSummarizeRuleAddressing(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)
	writeTestFile(t, root+"/Artist[X]/a__t.png", "a")
	writeTestFile(t, root+"/Artist[X]/sub/deep__c.png", "b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	if _, err := m.ApplyDirStructSummary(root, "Artist[X]"); err == nil ||
		!strings.Contains(err.Error(), "子树内没有已赋值文件") {
		t.Fatalf("无赋值子树应回可读错误: %v", err)
	}
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/a__t.png", SessionAssignment{
		Author: "X", AuthorClueRef: "dirLevelStripped1",
		Title: "a", TitleClueRef: "filenameSeg0",
	}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	sugg, err := m.ApplyDirStructSummary(root, "Artist[X]")
	if err != nil {
		t.Fatalf("目录寻址总结失败: %v", err)
	}
	if sugg.RelPath != "Artist[X]" {
		t.Fatalf("建议卡 RelPath 应呈现目录键: %q", sugg.RelPath)
	}
	if !strings.Contains(sugg.Summary, "剥壳→作者") || !strings.Contains(sugg.Summary, "文件名首段→作品名") {
		t.Fatalf("人话规则应由子树首个有赋值文件推导: %q", sugg.Summary)
	}
	if sugg.HitTotal != 12 {
		t.Fatalf("全库文件总数应含样例树（10+2），实际 %d", sugg.HitTotal)
	}
}

// TestHandleFrontendRequestSummarizeRule 前端协议分派：assign 携带出处载荷、
// summarizeRule 应答建议卡。
func TestHandleFrontendRequestSummarizeRule(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)
	_, rel, _ := realSampleStructFixture()
	writeTestFile(t, root+"/"+rel, "a")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 30, Action: workbenchActionAssign, Root: root, RelPath: rel,
		Author: "Jake--Jaccobus", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
	}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.Assignment == nil ||
		resp.Assignment.AuthorClueRef != "dirLevelStripped1" || resp.Assignment.TitleClueRef != "filenameSeg0" {
		t.Fatalf("assign 出处载荷应答不符: %+v", resp)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 31, Action: workbenchActionSummarizeRule, Root: root, RelPath: rel,
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.RuleSuggestion == nil {
		t.Fatalf("summarizeRule 应答不符: %+v", resp)
	}
	if !strings.Contains(resp.RuleSuggestion.Summary, "剥壳→作者") ||
		!strings.Contains(resp.RuleSuggestion.Summary, "文件名首段→作品名") {
		t.Fatalf("建议卡人话规则不符: %q", resp.RuleSuggestion.Summary)
	}

	// 无赋值文件：可读错误。
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 32, Action: workbenchActionSummarizeRule, Root: root, RelPath: "不存在/x.png",
	}))
	if resp := ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "无会话赋值") {
		t.Fatalf("无赋值应回可读错误: %+v", resp)
	}
}
