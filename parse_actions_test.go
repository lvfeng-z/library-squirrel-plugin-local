// 手动解析动作族的单元测试：冲突裁决双模式的最简充分形式生成物结构断言
// （双命中 And 组合 / 单侧缺席改绑的谓词条件策略，生成物不标记赋值）、泛化
// 规则按维度三态生成与泛化后赋值保留（2026-10-25 用户裁决废除「泛化即接
// 管」消费剥离——赋值与新规则双源互证，赋值显式维度覆盖规则终选）、会话
// 赋值写 API 与合并求值生效链路、生成入口的档案写并发无丢失、差异集计算
// 三分类、条件求值（提取段两遍执行）、档案版本向后兼容与条件构造期校验，
// 以及前端协议的 assign / generalize / adjudicate / getDiff 分派。
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// findRuleByID 档案内按 ID 取规则。
func findRuleByID(t *testing.T, profile *RuleProfile, id string) *RuleEntry {
	t.Helper()
	for i := range profile.Rules {
		if profile.Rules[i].ID == id {
			return &profile.Rules[i]
		}
	}
	t.Fatalf("档案内无规则 %q（现有 %d 条）", id, len(profile.Rules))
	return nil
}

// waitForVerificationRound 等待验证轮（异步重识别）完成：LastDiff 在场即终态。
func waitForVerificationRound(t *testing.T, m *SessionManager, root string) *WorkbenchSession {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sess, err := m.SessionOf(root)
		if err == nil && sess.LastDiff != nil {
			return sess
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待验证轮完成超时")
	return nil
}

// newAdjudicationFixture 冲突裁决测试环境：样例树 + 全量识别完成（快照在场）。
func newAdjudicationFixture(t *testing.T, root string) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	m, ctx := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, ctx
}

// —— 退出标准1：双命中冲突裁决产出 And 组合规则（最简充分形式）——

// TestAdjudicatePickGeneratesAndRule 双命中择一：生成规则的模式为候选规则模式的
// And 组合、合成为恰一条裁决常量动作（无条件、无多余维度）；重识别后该文件
// 待裁决消解、终选为裁决值、差异集命中该文件。
func TestAdjudicatePickGeneratesAndRule(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	result, err := m.ApplyAdjudication(root, "冲突/AB.jpg", DimTitle, adjudicationInput{
		Mode: adjudicateModePick, PickIndex: 0,
	})
	if err != nil {
		t.Fatalf("双命中裁决失败: %v", err)
	}
	if result.Rule == nil || !result.VerificationStarted {
		t.Fatalf("裁决结果形态不符: %+v", result)
	}
	if !strings.HasPrefix(result.Rule.ID, "adj-") || result.Rule.Source != string(RuleSourceAdjudication) {
		t.Fatalf("生成规则 ID/来源不符: %+v", result.Rule)
	}

	// 生成物结构断言（档案内）：模式=And[conflictA, conflictB]，提取恰一条
	profile, errs := LoadProfile(m.store)
	if len(errs) > 0 || profile == nil {
		t.Fatalf("载入档案失败: %v", profile)
	}
	rule := findRuleByID(t, profile, result.Rule.ID)
	and, ok := rule.Pattern.(And)
	if !ok {
		t.Fatalf("生成规则模式应为 And 组合，实际 %T", rule.Pattern)
	}
	if len(and) != 2 {
		t.Fatalf("And 组合应恰含双方候选模式（最简充分形式），实际 %d 个子模式", len(and))
	}
	for _, sub := range and {
		if _, isRegex := sub.(*RegexPattern); !isRegex {
			t.Fatalf("And 子模式应为候选规则的正则模式，实际 %T", sub)
		}
	}
	if len(rule.Extractions) != 1 {
		t.Fatalf("生成规则提取动作应恰一条（合成=裁决结果），实际 %d 条", len(rule.Extractions))
	}
	action := rule.Extractions[0]
	if action.Dimension != DimTitle || action.From != FromAdjudication || action.Constant != "A" || action.When != nil {
		t.Fatalf("裁决动作形态不符（应为无条件的 title 常量 A）: %+v", action)
	}
	if rule.Source != RuleSourceAdjudication || !rule.Enabled {
		t.Fatalf("生成规则来源/启用态不符: %+v", rule)
	}

	// 验证轮：重识别后冲突消解（裁决声明信任级压过双方）、差异高亮命中
	sess := waitForVerificationRound(t, m, root)
	file := snapshotFileOf(sess.Snapshot, "冲突/AB.jpg")
	if file == nil {
		t.Fatal("重识别后缺冲突文件记录")
	}
	if len(file.PendingDims) != 0 {
		t.Fatalf("裁决后待裁决维度应消解，实际 %v", file.PendingDims)
	}
	found := false
	for _, decl := range file.Final {
		if decl.Dimension == "title" && decl.Value == "A" && decl.Source == string(SourceAdjudication) {
			found = true
		}
	}
	if !found {
		t.Fatalf("终选应含裁决值 title=A（裁决源），实际 %+v", file.Final)
	}
	if len(sess.Snapshot.Pending) != 0 {
		t.Fatalf("同类冲突不应再出现在待裁决清单，实际 %+v", sess.Snapshot.Pending)
	}
	if len(sess.LastDiff.Changed) != 1 || sess.LastDiff.Changed[0].RelPath != "冲突/AB.jpg" {
		t.Fatalf("差异集应恰含冲突文件的结论变化，实际 %+v", sess.LastDiff.Changed)
	}
	entry := sess.LastDiff.Changed[0]
	if entry.Before == nil || entry.After == nil || len(entry.Before.PendingDims) != 1 {
		t.Fatalf("差异条目前后形态不符: %+v", entry)
	}
}

// TestAdjudicateSameRuleCandidatesCollapse 候选同出一规则（同规则双动作产值）时
// 组合模式退化为该规则模式（A∧A≡A 的最简形式）。
func TestAdjudicateSameRuleCandidatesCollapse(t *testing.T) {
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)

	// 单规则双常量动作 → 同层 title 冲突（两候选同 RuleID）
	dual := RuleEntry{
		ID: "dual-title", Source: RuleSourceManual, Enabled: true,
		Pattern: mustRegexPattern(t, `^D.*\.jpg$`, nil),
		Extractions: []ExtractionAction{
			{Dimension: DimTitle, From: FromConstant, Constant: "甲"},
			{Dimension: DimTitle, From: FromConstant, Constant: "乙"},
		},
	}
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{dual}}); err != nil {
		t.Fatalf("写入档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	writeTestFile(t, root+"/Dual.jpg", "d")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	snap := mustSessionSnapshot(t, m, root)
	if len(snap.Pending) != 1 || snap.Pending[0].RelPath != "Dual.jpg" {
		t.Fatalf("应产出同规则双值待裁决项: %+v", snap.Pending)
	}

	result, err := m.ApplyAdjudication(root, "Dual.jpg", DimTitle, adjudicationInput{
		Mode: adjudicateModePick, PickIndex: 1,
	})
	if err != nil {
		t.Fatalf("同规则候选裁决失败: %v", err)
	}
	profile, _ := LoadProfile(store)
	rule := findRuleByID(t, profile, result.Rule.ID)
	regex, ok := rule.Pattern.(*RegexPattern)
	if !ok || regex.Expr != `^D.*\.jpg$` {
		t.Fatalf("同规则候选的组合模式应退化为该规则模式，实际 %T %+v", rule.Pattern, rule.Pattern)
	}
	if len(rule.Extractions) != 1 || rule.Extractions[0].Constant != "乙" {
		t.Fatalf("裁决动作应恰一条且取所选候选值: %+v", rule.Extractions)
	}
}

// —— 退出标准1：缺席场景产出谓词条件策略（最简充分形式）——

// TestAdjudicateConditionalGeneratesPredicateStrategy 单侧缺席改绑：生成规则的
// 合成带在场/缺席谓词条件双分支，两分支各恰一条谓词；构造期校验拒绝未知来源。
func TestAdjudicateConditionalGeneratesPredicateStrategy(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	result, err := m.ApplyAdjudication(root, "冲突/AB.jpg", DimTitle, adjudicationInput{
		Mode:               adjudicateModeConditional,
		PredicateSource:    SourceDir,
		PredicateDimension: DimAuthor,
		PresentIndex:       0,
		AbsentIndex:        1,
	})
	if err != nil {
		t.Fatalf("缺席改绑裁决失败: %v", err)
	}

	profile, _ := LoadProfile(m.store)
	rule := findRuleByID(t, profile, result.Rule.ID)
	if _, ok := rule.Pattern.(And); !ok {
		t.Fatalf("缺席改绑生成规则模式应仍为 And 组合锚定，实际 %T", rule.Pattern)
	}
	if len(rule.Extractions) != 2 {
		t.Fatalf("谓词条件策略应恰含在场/缺席两分支动作，实际 %d 条", len(rule.Extractions))
	}
	present, absent := rule.Extractions[0], rule.Extractions[1]
	if present.Constant != "A" || absent.Constant != "B" {
		t.Fatalf("条件分支值应取自候选双值，实际 %q / %q", present.Constant, absent.Constant)
	}
	for i, action := range []ExtractionAction{present, absent} {
		if action.From != FromAdjudication || action.Dimension != DimTitle {
			t.Fatalf("条件动作 %d 应为 title 裁决常量: %+v", i, action)
		}
	}
	if present.When == nil || present.When.Kind != ExprPredicate ||
		present.When.Source != SourceDir || present.When.Dimension != DimAuthor {
		t.Fatalf("在场分支条件应为恰一条在场谓词节点（最简形式）: %+v", present.When)
	}
	if absent.When == nil || absent.When.Kind != ExprNot || !equalCondExpr(absent.When.Inner, present.When) {
		t.Fatalf("缺席分支条件应为在场谓词节点的取反（最简形式）: %+v", absent.When)
	}

	// 非法谓词的构造期校验（中文错误 + 拒绝生成）
	if _, err := m.ApplyAdjudication(root, "冲突/AB.jpg", DimTitle, adjudicationInput{
		Mode: adjudicateModeConditional, PredicateSource: "nope", PredicateDimension: DimAuthor,
	}); err == nil || !strings.Contains(err.Error(), "未知的声明来源") {
		t.Fatalf("未知来源谓词应被拒绝: %v", err)
	}
}

// TestConditionalExtractionEvaluation 谓词条件策略的求值：目录源作者在场时取
// 在场分支值、缺席时取缺席分支值（提取段两遍执行——条件可见全部无条件声明）。
func TestConditionalExtractionEvaluation(t *testing.T) {
	ancestor := mustRegexPattern(t, `^作者甲$`, nil)
	location, err := NewLocationPattern(0, 0, ancestor)
	if err != nil {
		t.Fatalf("构造目录层模式失败: %v", err)
	}
	dirRule := RuleEntry{
		ID: "cond-dir", Source: RuleSourceManual, Enabled: true, Pattern: location,
		Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromAncestorDir}},
	}
	condRule := RuleEntry{
		ID: "cond-adj", Source: RuleSourceAdjudication, Enabled: true,
		Pattern: mustRegexPattern(t, `^.+\.jpg$`, nil),
		Extractions: []ExtractionAction{
			{Dimension: DimTitle, From: FromAdjudication, Constant: "在场值",
				When: predicateExpr(DeclPredicate{Source: SourceDir, Dimension: DimAuthor})},
			{Dimension: DimTitle, From: FromAdjudication, Constant: "缺席值",
				When: &CondExpr{Kind: ExprNot, Inner: predicateExpr(DeclPredicate{Source: SourceDir, Dimension: DimAuthor})}},
		},
	}
	files := []ScanFile{
		{FileName: "x.jpg", DirNames: []string{"作者甲"}, RelPath: "作者甲/x.jpg"},
		{FileName: "y.jpg", RelPath: "y.jpg"},
	}
	decisions := RunPipeline(files, []RuleEntry{dirRule, condRule}, nil)
	if len(decisions) != 2 {
		t.Fatalf("结论数 = %d，期望 2", len(decisions))
	}
	byRel := map[string]FileDecision{}
	for _, d := range decisions {
		byRel[d.RelPath] = d
	}
	titleOf := func(rel string) (string, SourceKind) {
		for _, decl := range byRel[rel].Declarations {
			if decl.Dimension == DimTitle {
				return decl.Value, decl.Source
			}
		}
		return "", ""
	}
	value, source := titleOf("作者甲/x.jpg")
	if value != "在场值" || source != SourceAdjudication {
		t.Fatalf("目录源作者在场时应取在场分支值，实际 %q（源 %s）", value, source)
	}
	value, source = titleOf("y.jpg")
	if value != "缺席值" || source != SourceAdjudication {
		t.Fatalf("目录源作者缺席时应取缺席分支值，实际 %q（源 %s）", value, source)
	}
	// 在场谓词依据的目录声明确实由同轮无条件动作产出
	foundDir := false
	for _, decl := range byRel["作者甲/x.jpg"].Declarations {
		if decl.Dimension == DimAuthor && decl.Source == SourceDir && decl.Value == "作者甲" {
			foundDir = true
		}
	}
	if !foundDir {
		t.Fatalf("目录规则声明缺失: %+v", byRel["作者甲/x.jpg"].Declarations)
	}
}

// —— 退出标准2：泛化规则按维度正确生成（三态）——

// TestGeneralizeFilenameShape 按文件名形态（v4 条件行）：赋值值位升为占位词、
// 其余文本保持字面段（方括号作者样例与例子模板同形），来源 qna、ID 前缀
// qna-file-；提取动作带行引用（行1=文件目标行）。
func TestGeneralizeFilenameShape(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	rel := "已识别/【作者A】标题1.jpg"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{Author: "作者A", Title: "标题1"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	result, err := m.ApplyGeneralization(root, rel, generalizeModeFilename)
	if err != nil {
		t.Fatalf("文件名形态泛化失败: %v", err)
	}
	if !strings.HasPrefix(result.Rule.ID, "qna-file-") {
		t.Fatalf("泛化规则 ID 前缀不符: %q", result.Rule.ID)
	}

	profile, _ := LoadProfile(m.store)
	rule := findRuleByID(t, profile, result.Rule.ID)
	if len(rule.Conditions) != 1 || rule.Conditions[0].Condition == nil ||
		rule.Conditions[0].Condition.Target != CondxTargetFile {
		t.Fatalf("文件名形态泛化应为单行文件条件: %+v", rule.Conditions)
	}
	cond := rule.Conditions[0].Condition
	if cond.Format.Mode != CondxFormatTemplate || cond.Format.Tpl != "【{作者}】{标题}" {
		t.Fatalf("模板不符: %+v", cond.Format)
	}
	if len(rule.Extractions) != 2 {
		t.Fatalf("提取动作应按文件名值位生成两维，实际 %d 条", len(rule.Extractions))
	}
	for _, action := range rule.Extractions {
		if action.From != FromCapture || action.Row != 1 {
			t.Fatalf("文件名形态泛化的提取应为行引用取值: %+v", action)
		}
	}
	if rule.Extractions[0].CaptureName != "author" || rule.Extractions[1].CaptureName != "title" {
		t.Fatalf("提取引用的英文组名不符: %+v", rule.Extractions)
	}
	if rule.Source != RuleSourceQna {
		t.Fatalf("泛化规则来源应为 qna: %+v", rule)
	}

	// 求值面：模板行对同形文件名回放捕获（贪婪回溯切分，右闭分隔符免排除类）。
	ctx := &ParseContext{}
	if !rule.Pattern.Evaluate(&MatchInput{FileName: "【作者B】标题2.jpg"}, nil, ctx) {
		t.Fatalf("同形文件名应命中")
	}
	if v, _ := ctx.Capture("r1.author"); v != "作者B" {
		t.Fatalf("作者捕获不符: %q", v)
	}
	if v, _ := ctx.Capture("r1.title"); v != "标题2" {
		t.Fatalf("标题捕获不符: %q", v)
	}

	// 泛化赋值保留（2026-10-25 用户裁决：废除「泛化即接管」消费剥离）：规则
	// 复现赋值两维但赋值全量保留并置已泛化标记；验证轮终选声明继续由赋值
	// 显式维度覆盖（赋值=最强源，双源互证）。
	sess := waitForVerificationRound(t, m, root)
	if kept := sess.Assignments[rel]; kept.Author != "作者A" || kept.Title != "标题1" || !kept.Generalized {
		t.Fatalf("泛化后赋值应全量保留并置标记: %+v", kept)
	}
	file := waitAssignmentRefresh(t, m, root, rel, func(f *SnapFile) bool {
		return finalHasDecl(f, DimAuthor, "作者A", SourceAssignment) &&
			finalHasDecl(f, DimTitle, "标题1", SourceAssignment)
	})
	if !finalHasDecl(file, DimAuthor, "作者A", SourceAssignment) ||
		!finalHasDecl(file, DimTitle, "标题1", SourceAssignment) {
		t.Fatalf("赋值显式维度应继续覆盖终选（双源互证）: %+v", file.Final)
	}

	// 未出现在文件名中的维度不烧入文件名形态规则（由赋值承载）
	if _, _, err := m.ApplyAssignment(root, "线索/任意名.jpg", SessionAssignment{Author: "不在名中"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if _, err := m.ApplyGeneralization(root, "线索/任意名.jpg", generalizeModeFilename); err == nil ||
		!strings.Contains(err.Error(), "均未出现在文件名") {
		t.Fatalf("值不在文件名中时应拒绝泛化: %v", err)
	}
}

// TestGeneralizeDirLayer 按目录层（v4 目录条件行）：目录整名字面模板（任意
// 祖先层同名命中=子树包含），值同目录名的维度按目录名取值（ancestorDir 取
// 命中层名）、异值的作常量声明。
func TestGeneralizeDirLayer(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	rel := "已识别/【作者A】标题1.jpg"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{Author: "作者A"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	result, err := m.ApplyGeneralization(root, rel, generalizeModeDirLayer)
	if err != nil {
		t.Fatalf("目录层泛化失败: %v", err)
	}
	if !strings.HasPrefix(result.Rule.ID, "qna-dir-") {
		t.Fatalf("泛化规则 ID 前缀不符: %q", result.Rule.ID)
	}

	profile, _ := LoadProfile(m.store)
	rule := findRuleByID(t, profile, result.Rule.ID)
	if len(rule.Conditions) != 1 || rule.Conditions[0].Condition == nil ||
		rule.Conditions[0].Condition.Target != CondxTargetDir {
		t.Fatalf("目录层泛化应为单行目录条件: %+v", rule.Conditions)
	}
	cond := rule.Conditions[0].Condition
	if cond.Format.Mode != CondxFormatTemplate || cond.Format.Tpl != "已识别" {
		t.Fatalf("目录模板应为父目录名整名字面: %+v", cond.Format)
	}
	// 求值面：任意祖先层同名命中（子树包含语义）。
	ctx := &ParseContext{}
	if !rule.Pattern.Evaluate(&MatchInput{DirNames: []string{"库", "已识别"}}, nil, ctx) {
		t.Fatalf("祖先层同名目录应命中")
	}
	if len(rule.Extractions) != 1 || rule.Extractions[0].From != FromConstant || rule.Extractions[0].Constant != "作者A" {
		t.Fatalf("异目录名值应作常量声明: %+v", rule.Extractions)
	}

	// 值=目录名：按目录名取值（ancestorDir）
	rel2 := "线索/任意名.jpg"
	if _, _, err := m.ApplyAssignment(root, rel2, SessionAssignment{Tags: []string{"线索"}}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	result2, err := m.ApplyGeneralization(root, rel2, generalizeModeDirLayer)
	if err != nil {
		t.Fatalf("目录层泛化失败: %v", err)
	}
	profile, _ = LoadProfile(m.store)
	rule2 := findRuleByID(t, profile, result2.Rule.ID)
	if len(rule2.Extractions) != 1 || rule2.Extractions[0].From != FromAncestorDir || rule2.Extractions[0].Dimension != DimTag {
		t.Fatalf("值同目录名的维度应按目录名取值: %+v", rule2.Extractions)
	}

	// 库根直下无目录层：拒绝
	if _, _, err := m.ApplyAssignment(root, "12345678_p0.jpg", SessionAssignment{Author: "甲"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if _, err := m.ApplyGeneralization(root, "12345678_p0.jpg", generalizeModeDirLayer); err == nil ||
		!strings.Contains(err.Error(), "无目录层可泛化") {
		t.Fatalf("库根直下应拒绝目录层泛化: %v", err)
	}
}

// TestGeneralizeSequence 按序列形态：规则定群群（测试档案的兄弟群组规则抢先
// 定群、无形态标识）走公共前缀字面锚定路径（分组键=前缀字面、序号=扩展名前
// 尾缀数字、≥2 成群、归入作品集），纯定群无提取动作，产物对同前缀新文件可
// 复现定群；落单文件拒绝。
func TestGeneralizeSequence(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	rel := "序列/库内作者名_002.jpg"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{Collection: "库内作者名"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	result, err := m.ApplyGeneralization(root, rel, generalizeModeSequence)
	if err != nil {
		t.Fatalf("序列形态泛化失败: %v", err)
	}
	if !strings.HasPrefix(result.Rule.ID, "qna-seq-") {
		t.Fatalf("泛化规则 ID 前缀不符: %q", result.Rule.ID)
	}

	profile, _ := LoadProfile(m.store)
	rule := findRuleByID(t, profile, result.Rule.ID)
	if len(rule.Conditions) != 1 || rule.Conditions[0].Condition == nil ||
		rule.Conditions[0].Condition.Target != CondxTargetSeq {
		t.Fatalf("序列形态泛化应为单行序列条件: %+v", rule.Conditions)
	}
	cond := rule.Conditions[0].Condition
	if cond.Format.Mode != CondxFormatTemplate || cond.Format.Tpl != "{键}_{序号}" {
		t.Fatalf("成员模板不符（{键}盖住公共前缀、{序号}盖住尾缀数字）: %+v", cond.Format)
	}
	if len(cond.Attach) != 1 || cond.Attach[0].Kind != CondxAttachSeqMinLength || cond.Attach[0].MinCount != 2 {
		t.Fatalf("成群门槛应为 seqMinLength=2: %+v", cond.Attach)
	}
	if !isGroupingRule(rule) {
		t.Fatalf("单序列条件行规则应判为定群规则")
	}
	if len(rule.Extractions) != 0 {
		t.Fatalf("定群规则不应携带提取动作（最简形式）: %+v", rule.Extractions)
	}

	// 复现定群：同前缀新文件（不同目录）经该规则聚群，异前缀不群。
	replicas := []ScanFile{
		{Path: `E:\lib\新目录\库内作者名_05.jpg`, FileName: "库内作者名_05.jpg", DirNames: []string{"新目录"}, RelPath: "新目录/库内作者名_05.jpg"},
		{Path: `E:\lib\新目录\库内作者名_06.jpg`, FileName: "库内作者名_06.jpg", DirNames: []string{"新目录"}, RelPath: "新目录/库内作者名_06.jpg"},
		{Path: `E:\lib\新目录\另一前缀_01.jpg`, FileName: "另一前缀_01.jpg", DirNames: []string{"新目录"}, RelPath: "新目录/另一前缀_01.jpg"},
	}
	rDecisions := RunPipeline(replicas, []RuleEntry{*rule}, nil)
	rBy := decisionsByName(t, rDecisions)
	for _, name := range []string{"库内作者名_05.jpg", "库内作者名_06.jpg"} {
		d := rBy[name]
		if d.Group == nil || d.Group.Key != "库内作者名" || d.Group.Size != 2 {
			t.Fatalf("%s 应经泛化规则复现定群（键=库内作者名、组大小=2），实际 %+v", name, d.Group)
		}
	}
	if d := rBy["另一前缀_01.jpg"]; d.Group != nil {
		t.Fatalf("异前缀文件不应被前缀字面规则入群，实际 %+v", d.Group)
	}

	// 落单文件不成群：拒绝
	if _, _, err := m.ApplyAssignment(root, "序列/lone_007.jpg", SessionAssignment{Author: "甲"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if _, err := m.ApplyGeneralization(root, "序列/lone_007.jpg", generalizeModeSequence); err == nil ||
		!strings.Contains(err.Error(), "不属于任何兄弟序列群") {
		t.Fatalf("落单文件应拒绝序列形态泛化: %v", err)
	}
}

// TestBuildSequenceRuleFallbackWrappedSeq 规则定群群（无形态标识）回退分支的
// v4 成员模板：包裹符原样入字面段（样本 `[01]` → 模板 `{键}_[{序号}]`，同包
// 裹制的同形态成员可复现定群）。制式换代分歧注记：v3 提取式对括号/方括号
// 跨制混判，v4 模板字面段严格同制——跨包裹制混编群成员不再互认（分离为更
// 小的群或不成群，宁缺勿错）。
func TestBuildSequenceRuleFallbackWrappedSeq(t *testing.T) {
	rule, _, err := buildSequenceRule("集盒/[画师]_[123456789]_[集名]_[01].png",
		"[画师]_[123456789]_[集名]_[01].png", "[画师]_[123456789]_[集名]", nil)
	if err != nil {
		t.Fatalf("回退分支构造失败: %v", err)
	}
	if len(rule.Conditions) != 1 || rule.Conditions[0].Condition.Target != CondxTargetSeq {
		t.Fatalf("回退分支应为单行序列条件: %+v", rule.Conditions)
	}
	if tpl := rule.Conditions[0].Condition.Format.Tpl; tpl != "{键}_[{序号}]" {
		t.Fatalf("回退分支成员模板不符: %q", tpl)
	}
	// 括号序号成员经模板定群（≥2 同键、序号=段内数字、键={键} 捕获=前缀）。
	bracketFiles := []ScanFile{
		{Path: `E:\lib\集盒\[画师]_[123456789]_[集名]_[01].png`, FileName: "[画师]_[123456789]_[集名]_[01].png", DirNames: []string{"集盒"}, RelPath: "集盒/[画师]_[123456789]_[集名]_[01].png"},
		{Path: `E:\lib\集盒\[画师]_[123456789]_[集名]_[02].png`, FileName: "[画师]_[123456789]_[集名]_[02].png", DirNames: []string{"集盒"}, RelPath: "集盒/[画师]_[123456789]_[集名]_[02].png"},
	}
	decisions := RunPipeline(bracketFiles, []RuleEntry{rule}, nil)
	for i, d := range decisions {
		if d.Group == nil || d.Group.Key != "[画师]_[123456789]_[集名]" || d.Group.Seq != i+1 || d.Group.Size != 2 {
			t.Fatalf("括号序号应经模板定群: %+v", d.Group)
		}
	}
	if text, ok := groupSeqTextOf(&rule, "[画师]_[123456789]_[集名]_[02].png"); !ok || text != "02" {
		t.Fatalf("序号原文应保留零填充，实际 %q（ok=%v）", text, ok)
	}
	// 同制无扩展名成员（.png 缺席）不回归：去扩展名基名同判。
	if text, ok := groupSeqTextOf(&rule, "[画师]_[123456789]_[集名]_[03]"); !ok || text != "03" {
		t.Fatalf("无扩展名成员序号原文不符: %q（ok=%v）", text, ok)
	}
	// 跨包裹制（圆括号）成员不再互认（v4 字面段严格同制）。
	ctx := &ParseContext{}
	if rule.Pattern.Evaluate(&MatchInput{FileName: "[画师]_[123456789]_[集名]_(04).png"}, nil, ctx) && ctx.Group != nil {
		t.Fatalf("圆括号包裹成员不应经方括号模板定群（制式换代分歧，见注记）")
	}
	if _, ok := groupSeqTextOf(&rule, "表紙.png"); ok {
		t.Fatal("无尾缀数字的文件不应提取出序号")
	}
}

// TestDirectSeqFormSingleCharKey 直连序号形态键式须容忍单字符键（live 发现
// 1-fix 回归锚：键式 `^(.+?[^\d])\d{1,4}$` 组1最少 2 字符，集键 `m`（成员
// m1.jpg/m2.jpg——识别侧元素对齐可建群）经 FORM 分支构造的定群规则 keyMatch
// false → 序列指派被定群门拒；识别侧能建出的键构造侧都该能复现）；纯数字
// 基不放宽（识别侧该形态键由元素拼接兜底为空串——数字流尽数归序号），多字
// 符键不回归。
func TestDirectSeqFormSingleCharKey(t *testing.T) {
	form := formByName("直连序号")
	if form == nil {
		t.Fatal("形态表缺直连序号条目")
	}
	if m := form.keyRe.FindStringSubmatch("m1"); len(m) != 2 || m[1] != "m" {
		t.Fatalf("单字符键应可提取: %+v", m)
	}
	if m := form.keyRe.FindStringSubmatch("moka1"); len(m) != 2 || m[1] != "moka" {
		t.Fatalf("多字符键不回归: %+v", m)
	}
	for _, base := range []string{"12345", "12", "1"} {
		if m := form.keyRe.FindStringSubmatch(base); m != nil {
			t.Fatalf("纯数字基不应提取出非空键: %q -> %+v", base, m)
		}
	}
	// FORM 分支配新键式复现单字符键定群——v4 语法换代注记：直连序号
	//（键段与序号段无分隔字面）键式 `(.+?[^\d])\d{1,4}` 在模板语法中即相邻
	// 占位词 {键}{序号}，词法禁止（切分歧义），构造侧如实拒绝（宁缺勿错）；
	// 识别侧该形态键由元素拼接兜底的能力不受影响（识别建群与规则定群两码
	// 事，规则侧本就该拒绝歧义形态）。
	if _, _, err := buildSequenceRule("单键集/m1.jpg", "m1.jpg", "m", &SnapGroup{
		FormName: form.name, FormKeyExpr: form.keyExpr, FormSeqExpr: form.seqExpr,
	}); err == nil {
		t.Fatal("无分隔直连序号应拒绝模板化（相邻占位词），实际构造成功")
	} else if !strings.Contains(err.Error(), "相邻") {
		t.Fatalf("拒绝原因应为相邻占位词: %v", err)
	}
}

// TestGeneralizeSequenceFormFollows 结构形态群的端到端形态跟随：空档案环境
// （仅内置规则，测试档案的兄弟群组规则不抢占定群）下括号序号结构群携带形态
// 标识 → 泛化产物按群携带的键/序号提取式定群（提取式补扩展名尾段）→ 同形态
// 新文件复现定群、序号原文保留零填充；无尾锚提取式（中段序号）原样承载。
func TestGeneralizeSequenceFormFollows(t *testing.T) {
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion}); err != nil {
		t.Fatalf("写入空规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	for i := 1; i <= 4; i++ {
		writeTestFile(t, root+fmt.Sprintf("/括号/[作者甲]_[100423498]_新刊_[%02d].jpg", i), "x")
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	rel := "括号/[作者甲]_[100423498]_新刊_[02].jpg"
	group := snapshotGroupOf(sess.Snapshot, rel)
	if group == nil || group.FormName != "括号序号" {
		t.Fatalf("样例文件应属括号序号结构群，实际 %+v", group)
	}

	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{Collection: "新刊"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	result, err := m.ApplyGeneralization(root, rel, generalizeModeSequence)
	if err != nil {
		t.Fatalf("序列形态泛化失败: %v", err)
	}
	profile, _ := LoadProfile(m.store)
	rule := findRuleByID(t, profile, result.Rule.ID)
	if len(rule.Conditions) != 1 || rule.Conditions[0].Condition == nil ||
		rule.Conditions[0].Condition.Target != CondxTargetSeq {
		t.Fatalf("泛化规则应为单行序列条件，实际 %+v", rule.Conditions)
	}
	if tpl := rule.Conditions[0].Condition.Format.Tpl; tpl != "{键}_[{序号}]" {
		t.Fatalf("括号序号形态的成员模板不符: %q", tpl)
	}
	if !strings.Contains(rule.Description, "括号序号") {
		t.Fatalf("泛化规则描述应携带形态名: %q", rule.Description)
	}

	// 复现定群：不同前缀的同类形态新文件经规则聚群，键={键} 捕获=剥尾 `_`
	// 的全前缀。
	replicas := []ScanFile{
		{Path: `E:\lib\别处\[作者乙]_[999888777]_画集_[01].jpg`, FileName: "[作者乙]_[999888777]_画集_[01].jpg", DirNames: []string{"别处"}, RelPath: "别处/[作者乙]_[999888777]_画集_[01].jpg"},
		{Path: `E:\lib\别处\[作者乙]_[999888777]_画集_[02].jpg`, FileName: "[作者乙]_[999888777]_画集_[02].jpg", DirNames: []string{"别处"}, RelPath: "别处/[作者乙]_[999888777]_画集_[02].jpg"},
	}
	rDecisions := RunPipeline(replicas, []RuleEntry{*rule}, nil)
	for _, d := range rDecisions {
		if d.Group == nil || d.Group.Key != "[作者乙]_[999888777]_画集" || d.Group.Size != 2 {
			t.Fatalf("同形态新文件应复现定群（键=前缀剥尾 _、组大小=2），实际 %+v", d.Group)
		}
	}
	if text, ok := groupSeqTextOf(rule, "[作者乙]_[999888777]_画集_[02].jpg"); !ok || text != "02" {
		t.Fatalf("序号原文应保留零填充，实际 %q（ok=%v）", text, ok)
	}

	// 无尾锚提取式（中段序号）形态段照搬入字面段。
	midRule, _, err := buildSequenceRule("丁/industrial_becca_page_10_by_x.jpg", "industrial_becca_page_10_by_x.jpg", "industrial_becca",
		&SnapGroup{FormName: "中段序号", FormKeyExpr: `^(.+?)_page_`, FormSeqExpr: `_page_(\d{1,4})_by_`})
	if err != nil {
		t.Fatalf("中段序号形态泛化失败: %v", err)
	}
	if tpl := midRule.Conditions[0].Condition.Format.Tpl; tpl != "{键}_page_{序号}_by_x" {
		t.Fatalf("中段序号成员模板不符（形态段入字面段）: %q", tpl)
	}
}

// —— 退出标准3：赋值生效链路（创建侧）与档案写串行 ——

// TestAssignmentWriteAPIMergedRecognition 赋值经写 API 创建后由合并求值消费：
// 指定维度覆盖（冲突消解）、未指定维度保留档案结论；清除后回到档案结论。
func TestAssignmentWriteAPIMergedRecognition(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	// 指定维度覆盖：title 赋值消解待裁决冲突
	rel := "冲突/AB.jpg"
	sess, view, err := m.ApplyAssignment(root, rel, SessionAssignment{Title: "手工标题"})
	if err != nil || view == nil {
		t.Fatalf("写入赋值失败: %v %+v", err, view)
	}
	if view.Title != "手工标题" || sess.State != WorkbenchSessionRecognized {
		t.Fatalf("赋值应答形态不符: %+v %+v", view, sess)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("重识别失败: %v", err)
	}
	snap := mustSessionSnapshot(t, m, root)
	file := snapshotFileOf(snap, rel)
	if file == nil || len(file.PendingDims) != 0 || !file.Assigned {
		t.Fatalf("赋值后冲突应消解且带赋值标记: %+v", file)
	}
	found := false
	for _, decl := range file.Final {
		if decl.Dimension == "title" && decl.Value == "手工标题" && decl.Source == string(SourceAssignment) {
			found = true
		}
	}
	if !found {
		t.Fatalf("终选应含赋值 title=手工标题（赋值源）: %+v", file.Final)
	}

	// 未指定维度保留：对已识别文件只补集维度，作者/标题结论保留
	rel2 := "已识别/【作者A】标题1.jpg"
	if _, _, err := m.ApplyAssignment(root, rel2, SessionAssignment{Collection: "手工集"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("重识别失败: %v", err)
	}
	file2 := snapshotFileOf(mustSessionSnapshot(t, m, root), rel2)
	dims := map[string]string{}
	for _, decl := range file2.Final {
		dims[decl.Dimension] = decl.Value
	}
	if dims["author"] != "作者A" || dims["title"] != "标题1" || dims["collection"] != "手工集" {
		t.Fatalf("赋值应只覆盖指定维度: %+v", dims)
	}

	// 清除：回到档案结论（冲突重现）
	if _, view, err := m.ApplyAssignment(root, rel, SessionAssignment{}); err != nil || view != nil {
		t.Fatalf("清除赋值应回空视图: %v %+v", err, view)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("重识别失败: %v", err)
	}
	file = snapshotFileOf(mustSessionSnapshot(t, m, root), rel)
	if len(file.PendingDims) != 1 || file.PendingDims[0] != "title" {
		t.Fatalf("清除赋值后冲突应回到待裁决: %+v", file.PendingDims)
	}

	// 忽略项登记（元素7 接线）
	if _, err := m.SetIgnored(root, rel2, true); err != nil {
		t.Fatalf("登记忽略项失败: %v", err)
	}
	sessAfter, err := m.SessionOf(root)
	if err != nil || len(sessAfter.Ignored) != 1 || sessAfter.Ignored[0] != rel2 {
		t.Fatalf("忽略项形态不符: %v %+v", err, sessAfter.Ignored)
	}
	if _, err := m.SetIgnored(root, rel2, false); err != nil {
		t.Fatalf("解除忽略项失败: %v", err)
	}
	if sessAfter, err = m.SessionOf(root); err != nil || len(sessAfter.Ignored) != 0 {
		t.Fatalf("解除后忽略项应为空: %v %+v", err, sessAfter.Ignored)
	}
}

// TestConcurrentGenerationEntriesNotLost 生成两入口（泛化/裁决）并发写档案：
// 与既有 UpdateProfile 串行测试同构——并发下无规则丢失、无档案损坏。
func TestConcurrentGenerationEntriesNotLost(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	// 双冲突文件（冲突/AB.jpg 与 冲突/AyB.jpg 同时命中两条冲突规则）
	writeTestFile(t, root+"/冲突/AyB.jpg", "z")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := m.ApplyAdjudication(root, "冲突/AB.jpg", DimTitle, adjudicationInput{
			Mode: adjudicateModePick, PickIndex: 0,
		}); err != nil {
			t.Errorf("并发裁决一失败: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, _, err := m.ApplyAssignment(root, "冲突/AyB.jpg", SessionAssignment{Title: "并发标题"}); err != nil {
			t.Errorf("并发赋值失败: %v", err)
			return
		}
		if _, err := m.ApplyGeneralization(root, "冲突/AyB.jpg", generalizeModeFilename); err != nil &&
			!strings.Contains(err.Error(), "均未出现在文件名") {
			// 并发值不在文件名中会拒绝文件名形态（预期内），其余失败为缺陷
			t.Errorf("并发泛化失败: %v", err)
		}
	}()
	wg.Wait()

	profile, errs := LoadProfile(m.store)
	if profile == nil || len(errs) > 0 {
		t.Fatalf("并发后档案不可载入: %v %v", profile, errs)
	}
	adjCount, qnaCount := 0, 0
	for _, rule := range profile.Rules {
		switch rule.Source {
		case RuleSourceAdjudication:
			adjCount++
		case RuleSourceQna:
			qnaCount++
		}
	}
	if adjCount != 1 {
		t.Fatalf("并发裁决应恰留一条 adjudication 规则，实际 %d", adjCount)
	}
	if qnaCount > 1 {
		t.Fatalf("并发泛化至多留一条 qna 规则，实际 %d", qnaCount)
	}
}

// —— 退出标准4：差异集计算 ——

// TestComputeSnapshotDiff 结论变化/新增/消失三分类：状态或终选声明变化计入
// Changed、前快照独有计入 Removed、新快照独有计入 Added、同结论不入差异集。
func TestComputeSnapshotDiff(t *testing.T) {
	decl := func(dim, value, source string) SnapDecl {
		return SnapDecl{Dimension: dim, Value: value, Source: source}
	}
	prev := &RecognitionSnapshot{Files: []SnapFile{
		{RelPath: "变值.jpg", State: snapStateIdentified, Final: []SnapDecl{decl("title", "旧", "filename")}},
		{RelPath: "变态.jpg", State: snapStateClueful, Final: []SnapDecl{decl("title", "同", "filename")}},
		{RelPath: "消失.jpg", State: snapStatePlain},
		{RelPath: "不动.jpg", State: snapStatePlain},
	}}
	curr := &RecognitionSnapshot{Files: []SnapFile{
		{RelPath: "变值.jpg", State: snapStateIdentified, Final: []SnapDecl{decl("title", "新", "filename")}},
		{RelPath: "变态.jpg", State: snapStateIdentified, Final: []SnapDecl{decl("title", "同", "filename")}},
		{RelPath: "新增.jpg", State: snapStateClueful},
		{RelPath: "不动.jpg", State: snapStatePlain},
	}}
	diff := ComputeSnapshotDiff(prev, curr)
	if len(diff.Changed) != 2 {
		t.Fatalf("结论变化数 = %d（含状态变化与声明值变化），期望 2: %+v", len(diff.Changed), diff.Changed)
	}
	changedPaths := map[string]bool{}
	for _, entry := range diff.Changed {
		changedPaths[entry.RelPath] = true
		if entry.Before == nil || entry.After == nil {
			t.Fatalf("变化条目应带前后摘要: %+v", entry)
		}
	}
	if !changedPaths["变值.jpg"] || !changedPaths["变态.jpg"] {
		t.Fatalf("变化集应含声明值变化与状态变化: %v", changedPaths)
	}
	if len(diff.Added) != 1 || diff.Added[0].RelPath != "新增.jpg" || diff.Added[0].Before != nil {
		t.Fatalf("新增分类不符: %+v", diff.Added)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].RelPath != "消失.jpg" || diff.Removed[0].After != nil {
		t.Fatalf("消失分类不符: %+v", diff.Removed)
	}
	for _, entry := range append(append(diff.Changed, diff.Added...), diff.Removed...) {
		if entry.RelPath == "不动.jpg" {
			t.Fatal("同结论文件不应入差异集")
		}
	}

	// prev 为 nil：全量新增形态
	first := ComputeSnapshotDiff(nil, curr)
	if len(first.Added) != len(curr.Files) || len(first.Changed) != 0 || len(first.Removed) != 0 {
		t.Fatalf("首轮差异应全量新增: %+v", first)
	}
}

// —— 档案版本与条件构造期校验 ——
// （v1..v3 旧档拒载断言由 rule_condition_test / rules_store_test 承载。）

// TestConditionalRuleConstructionValidation 条件谓词引用未知来源/维度的规则在
// 构造期被跳过并报含规则 ID 的中文错误；裁决取值缺常量同。
func TestConditionalRuleConstructionValidation(t *testing.T) {
	store := newFakeSessionStore()
	base := func(when string) string {
		return fmt.Sprintf(`{"schemaVersion":%d,"rules":[{"id":"bad-cond","description":"坏条件规则","source":"manual","enabled":true,`, CurrentSchemaVersion) +
			`"pattern":{"kind":"regex","target":"filename","expr":"^x\\.jpg$","captures":[]},` +
			`"extractions":[{"dimension":"title","from":"constant","constant":"值"` + when + `}],"synthesis":{}}]}`
	}
	store.values[RuleProfileKVKey] = base(`,"when":{"present":[{"source":"nope","dimension":"author"}]}`)
	profile, errs := LoadProfile(store)
	if profile == nil || len(profile.Rules) != 0 || len(errs) != 1 {
		t.Fatalf("未知来源谓词应跳过规则并报错: %v %v", profile, errs)
	}
	if !strings.Contains(errs[0].Error(), "bad-cond") || !strings.Contains(errs[0].Error(), "未知的声明来源") {
		t.Fatalf("错误应含规则 ID 与中文原因: %v", errs[0])
	}

	store.values[RuleProfileKVKey] = fmt.Sprintf(`{"schemaVersion":%d,"rules":[{"id":"bad-adj","description":"缺常量","source":"adjudication","enabled":true,`, CurrentSchemaVersion) +
		`"pattern":{"kind":"regex","target":"filename","expr":"^x\\.jpg$","captures":[]},` +
		`"extractions":[{"dimension":"title","from":"adjudication"}],"synthesis":{}}]}`
	profile, errs = LoadProfile(store)
	if profile == nil || len(profile.Rules) != 0 || len(errs) != 1 ||
		!strings.Contains(errs[0].Error(), "缺少常量") {
		t.Fatalf("裁决取值缺常量应跳过规则并报错: %v %v", profile, errs)
	}
}

// —— 前端协议分派 ——

// TestHandleFrontendRequestParseActions assign / generalize / adjudicate / getDiff /
// setIgnored 的请求分派与序号配对应答。
func TestHandleFrontendRequestParseActions(t *testing.T) {
	root := t.TempDir()
	m, ctx := newAdjudicationFixture(t, root)

	// 单次赋值
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 10, Action: workbenchActionAssign, Root: root, RelPath: "冲突/AB.jpg", Title: "协议标题",
	}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.Assignment == nil || resp.Assignment.Title != "协议标题" {
		t.Fatalf("assign 应答不符: %+v", resp)
	}

	// 泛化（值不在文件名中→可读错误应答）
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 11, Action: workbenchActionGeneralize, Root: root, RelPath: "冲突/AB.jpg", GeneralizeMode: generalizeModeFilename,
	}))
	if resp := ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "均未出现在文件名") {
		t.Fatalf("泛化应回可读错误: %+v", resp)
	}

	// 泛化（改按目录层→生成成功且带规则摘要与验证轮结论）
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 12, Action: workbenchActionGeneralize, Root: root, RelPath: "冲突/AB.jpg", GeneralizeMode: generalizeModeDirLayer,
	}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.Rule == nil || !resp.Started {
		t.Fatalf("目录层泛化应答不符: %+v", resp)
	}

	// 等验证轮落地后裁决（赋值已改写冲突 → 用新冲突文件）
	writeTestFile(t, root+"/冲突/AyB.jpg", "z")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 13, Action: workbenchActionAdjudicate, Root: root, RelPath: "冲突/AyB.jpg",
		Dimension: string(DimTitle), AdjudicateMode: adjudicateModePick, PickIndex: 1,
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Rule == nil || !resp.Started {
		t.Fatalf("adjudicate 应答不符: %+v", resp)
	}
	waitForVerificationRound(t, m, root)

	// getDiff：验证轮差异集在场
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 14, Action: workbenchActionGetDiff, Root: root}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.Diff == nil {
		t.Fatalf("getDiff 应答不符: %+v", resp)
	}

	// getPanorama 附带差异集与决策态
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 15, Action: workbenchActionGetPanorama, Root: root}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.Diff == nil || resp.Decisions == nil {
		t.Fatalf("getPanorama 应附带差异集: %+v", resp)
	}

	// setIgnored
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 16, Action: workbenchActionSetIgnored, Root: root, RelPath: "顶层说明.txt", Ignored: true,
	}))
	if resp := ctx.lastResponse(t); !resp.OK || len(resp.Decisions.Ignored) != 1 {
		t.Fatalf("setIgnored 应答不符: %+v", resp)
	}
}

// mustSessionSnapshot 读会话快照（不在场即中止）。
func mustSessionSnapshot(t *testing.T, m *SessionManager, root string) *RecognitionSnapshot {
	t.Helper()
	sess, err := m.SessionOf(root)
	if err != nil || sess.Snapshot == nil {
		t.Fatalf("读会话快照失败: %v", err)
	}
	return sess.Snapshot
}

// —— 会话差异集持久化往返 ——

// TestSessionLastDiffRoundTrip 差异集随会话工件持久化（重启重建后仍在场）。
func TestSessionLastDiffRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	diff := &SnapDiff{
		Changed: []SnapDiffEntry{{RelPath: "a.jpg",
			Before: &SnapConclusion{State: snapStatePlain},
			After:  &SnapConclusion{State: snapStateIdentified, Final: []SnapDecl{{Dimension: "title", Value: "x", Source: "filename"}}}}},
		Added:   []SnapDiffEntry{},
		Removed: []SnapDiffEntry{},
	}
	if _, err := m.CompleteRecognitionWithDiff(root, &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		Files:         []SnapFile{{RelPath: "a.jpg", State: snapStateIdentified}},
		Stats:         SnapStats{TotalFiles: 1, Identified: 1},
	}, diff); err != nil {
		t.Fatalf("落库失败: %v", err)
	}
	rebooted := NewSessionManagerWithStore(ctx, nil, store)
	sess, err := rebooted.SessionOf(root)
	if err != nil {
		t.Fatalf("重启后读会话失败: %v", err)
	}
	if sess.LastDiff == nil || len(sess.LastDiff.Changed) != 1 || sess.LastDiff.Changed[0].RelPath != "a.jpg" {
		t.Fatalf("差异集应随会话持久化: %+v", sess.LastDiff)
	}
	// 空差异集经 JSON 往返后切片仍非 nil
	data, _ := json.Marshal(sess)
	var back WorkbenchSession
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("会话往返失败: %v", err)
	}
	if back.LastDiff == nil || back.LastDiff.Changed == nil || back.LastDiff.Added == nil || back.LastDiff.Removed == nil {
		t.Fatalf("空差异分类应为非 nil 切片: %+v", back.LastDiff)
	}
}

// —— 自动刷新通路：赋值/清除成功后自动排定识别刷新（解析视图重构阶段1）——

// finalHasDecl 终选是否含指定维度/值/来源的声明。
func finalHasDecl(f *SnapFile, dim MetadataDimension, value string, source SourceKind) bool {
	for _, decl := range f.Final {
		if decl.Dimension == string(dim) && decl.Value == value && decl.Source == string(source) {
			return true
		}
	}
	return false
}

// waitAssignmentRefresh 轮询快照直至赋值/清除触发的自动识别刷新完成、该文件
// 终选满足断言（退出标准：快照最终反映变更，无需任何再发的识别请求）。
func waitAssignmentRefresh(t *testing.T, m *SessionManager, root, relPath string, check func(*SnapFile) bool) *SnapFile {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		sess, err := m.SessionOf(root)
		if err == nil && sess.Snapshot != nil {
			if file := snapshotFileOf(sess.Snapshot, relPath); file != nil && check(file) {
				return file
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("等待赋值触发的自动识别刷新超时")
	return nil
}

// TestApplyAssignmentSchedulesRecognitionRefresh 赋值成功后自动排定增量重识别：
// 不再发任何识别请求，快照终选最终出现该赋值的 assignment 源声明。
func TestApplyAssignmentSchedulesRecognitionRefresh(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	sess, view, err := m.ApplyAssignment(root, "顶层说明.txt", SessionAssignment{Author: "手动作者"})
	if err != nil {
		t.Fatalf("单次赋值失败: %v", err)
	}
	if view == nil || view.Author != "手动作者" {
		t.Fatalf("赋值呈现不符: %+v", view)
	}
	// 前置：首轮识别快照在场，且尚未含该赋值声明（plain 文件）
	if sess.Snapshot == nil {
		t.Fatal("前置首轮识别快照缺失")
	}
	if before := snapshotFileOf(sess.Snapshot, "顶层说明.txt"); before == nil ||
		finalHasDecl(before, DimAuthor, "手动作者", SourceAssignment) {
		t.Fatalf("赋值前快照不应已含该作者声明: %+v", before)
	}

	file := waitAssignmentRefresh(t, m, root, "顶层说明.txt", func(f *SnapFile) bool {
		return finalHasDecl(f, DimAuthor, "手动作者", SourceAssignment)
	})
	if len(file.PendingDims) != 0 {
		t.Fatalf("赋值文件不应处于待裁决态: %+v", file.PendingDims)
	}
}

// TestClearAssignmentSchedulesRecognitionRefresh 清除（空维度赋值等价清除）同样
// 自动排定刷新：快照终选最终不再含该赋值声明（修订号闸门防旧结论被复用）。
func TestClearAssignmentSchedulesRecognitionRefresh(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)

	if _, _, err := m.ApplyAssignment(root, "顶层说明.txt", SessionAssignment{Author: "手动作者"}); err != nil {
		t.Fatalf("单次赋值失败: %v", err)
	}
	waitAssignmentRefresh(t, m, root, "顶层说明.txt", func(f *SnapFile) bool {
		return finalHasDecl(f, DimAuthor, "手动作者", SourceAssignment)
	})

	_, view, err := m.ApplyAssignment(root, "顶层说明.txt", SessionAssignment{})
	if err != nil {
		t.Fatalf("清除赋值失败: %v", err)
	}
	if view != nil {
		t.Fatalf("清除后赋值呈现应为 nil，实际 %+v", view)
	}
	waitAssignmentRefresh(t, m, root, "顶层说明.txt", func(f *SnapFile) bool {
		return !finalHasDecl(f, DimAuthor, "手动作者", SourceAssignment)
	})
}
