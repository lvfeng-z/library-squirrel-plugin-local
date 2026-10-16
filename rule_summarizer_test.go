package main

// 规则总结器的单元测试：真实样例的组合规则构造（根名→标签 / 一级目录剥壳→
// 作者 / 文件名首段→作品名 / 尾部码丢弃）、手填值自动匹配兜底与手工维度降级、
// 剥壳变换的求值、命中预览统计与样例、赋值出处存取（会话态+协议透传）、
// summarizeRule 不写档与 generalize mode=struct 确认入档。
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
//（作者=剥壳内名、作品名=首段）；不对应任何段的值降级「手工指定（不参与泛化）」。
func TestBuildStructMapRulesManualFallback(t *testing.T) {
	root, rel, file := realSampleStructFixture()
	assign := &SessionAssignment{
		Author:     "Jake--Jaccobus", // 自动匹配 dirLevelStripped1
		Title:      "atago",          // 自动匹配 filenameSeg0
		Collection: "手填集",           // 无对应段
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
		Tags:  []string{"big belly"}, TagClueRefs: []string{"rootName"},
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
