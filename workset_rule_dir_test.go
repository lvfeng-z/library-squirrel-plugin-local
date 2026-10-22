package main

// 目录名分段提取 · 泛化面（剧本D）的单元测试：集整体推导的双层优先级——
// 文件名无共同形态 + 目录名稳定格式 → 目录格式草稿（Isel 形态：第 2 层
// `[任意] [数字] [任意]` → 作品集=第 3 段、作者=一级目录 Artist 壳剥壳、
// 数字段可变、默认不勾仅序列成员）；文件名共同形态在→照旧不回归；稳定格式
// 判定（段数/包裹/分隔一致、数字段值可不同、类推断聚合）与格式不一致的
// 如实失败；调整面目录段指派→LocationPattern+FromCapture 构造（含校验）；
// 泛化入档端到端（各目录各自成集+跨目录同格式接管+赋值消费）。
import (
	"strings"
	"testing"
)

// workSetDirFixture 目录名分段测试环境（用户实例形态，Isel）：
//   - `Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/`：两个无共同形态
//     文件（目录名含作品号→目录候选——主体集，确认集名=第 3 段值）；
//   - `Artist[Isel]/[Isel] [92242474] [別の題]/`：同格式他目录两文件（跨目录
//     接管样本）。
//
// 返回管理器与主体集（目录候选）的目标键（已确认归集）。
func workSetDirFixture(t *testing.T, root string) (*SessionManager, string) {
	t.Helper()
	m, _ := newSeqRecognitionFixture(t, root)
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "a")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/おまけ.jpg", "b")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242474] [別の題]/表紙.png", "c")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242474] [別の題]/おまけ.jpg", "d")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, confirmDirCandidate(t, m, root, "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]", "瞬間的に絡み合う")
}

// confirmDirCandidate 找到指定目录的目录来源候选并确认归集，等待自动重识别
// 完成后返回目标键。
func confirmDirCandidate(t *testing.T, m *SessionManager, root, dir, setName string) string {
	t.Helper()
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	var target string
	for i := range sess.Snapshot.Candidates {
		c := &sess.Snapshot.Candidates[i]
		if c.Source == candSourceDir && c.Dir == dir {
			target = c.target()
			break
		}
	}
	if target == "" {
		t.Fatalf("候选主清单内无 %s 的目录候选: %+v", dir, sess.Snapshot.Candidates)
	}
	if _, _, err := m.ConfirmWorkSetCandidate(root, target, setName, 0); err != nil {
		t.Fatalf("确认归集失败: %v", err)
	}
	waitWorkSetIdle(t, m, root)
	return target
}

// dirCaptureExprOf 取规则内 LocationPattern 的 ancestor 表达式（非该形态报错）。
func dirCaptureExprOf(t *testing.T, rule *RuleEntry) (*LocationPattern, string) {
	t.Helper()
	loc, ok := rule.Pattern.(*LocationPattern)
	if !ok {
		t.Fatalf("目录格式条目应为 LocationPattern，实际 %T", rule.Pattern)
	}
	re, ok := loc.Ancestor.(*RegexPattern)
	if !ok {
		t.Fatalf("ancestor 应为正则模式，实际 %T", loc.Ancestor)
	}
	return loc, re.Expr
}

// TestWorkSetDirDraftIselForm 目录格式草稿（剧本D 步1/2）：成员文件名无共同
// 序列形态、所在目录名呈稳定格式 → 自动给目录格式草稿——形态短语、摘要
// （作品集=第 3 段〔末非数字段〕+ 每目录各自成集 + 作者=一级目录 Artist 剥壳）
// 、两规则（一级目录剥壳条目+目录格式条目——后者层深锚定 LocationPattern、
// 段格式正则带命名捕获、数字段 `\d+` 通配〔数字段值不当固定值参与识别〕、
// 无 when 原子）、命中预览=全库同格式目录逐个（含跨目录样例）、调整面默认
// 指派（数字段已标可变、标题段已指集名捕获、作者已指一级目录剥壳）。
func TestWorkSetDirDraftIselForm(t *testing.T) {
	root := t.TempDir()
	m, target := workSetDirFixture(t, root)

	sugg, err := m.ApplyWorkSetRuleSummary(root, target)
	if err != nil {
		t.Fatalf("集规则草稿推导失败: %v", err)
	}
	if sugg.Failed {
		t.Fatalf("目录名稳定格式在→不应失败态: %s", sugg.FailReason)
	}
	if !strings.Contains(sugg.Shape, "成员文件名无共同序列形态") ||
		!strings.Contains(sugg.Shape, "第 2 层目录呈 `[任意] [数字] [任意]` 格式") {
		t.Fatalf("形态短语不符: %q", sugg.Shape)
	}
	want := "第 2 层目录呈 `[任意] [数字] [任意]` 格式 → 作品集=第 3 段（每个这样的目录各自成集）；作者=一级目录 Artist[任意]剥壳"
	if sugg.Summary != want {
		t.Fatalf("摘要不符:\n got %q\nwant %q", sugg.Summary, want)
	}
	if len(sugg.Rules) != 2 || !strings.HasPrefix(sugg.Rules[1].ID, "qna-setdir-") ||
		!strings.HasPrefix(sugg.Rules[0].ID, "qna-struct-") {
		t.Fatalf("草稿应两规则（一级目录剥壳+目录格式）: %+v", sugg.Rules)
	}
	// 草稿规则本体断言（推导纯函数直取——建议即所见即所入，泛化入档复跑同一构造）。
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	ws, err := loadWorkSetContext(sess, target)
	if err != nil {
		t.Fatalf("读取集上下文失败: %v", err)
	}
	draft, failReason := deriveWorkSetDraft(sess.Snapshot, ws)
	if failReason != "" || draft.kind != draftDir {
		t.Fatalf("推导应为目录格式草稿: kind=%d fail=%q", draft.kind, failReason)
	}
	loc, expr := dirCaptureExprOf(t, &draft.rules[1])
	if loc.MinDepth != 1 || loc.MaxDepth != 1 {
		t.Fatalf("目录格式条目应锚定第 2 层（深度区间 1..1）: %+v", loc)
	}
	// 数字段类推断：`[92242473]`→`\[\d+\]` 通配；未捕获段泛化；集名段命名捕获。
	if expr != `^\[[^\]]*\] \[\d+\] \[(?P<collection>[^\]]*)\]$` {
		t.Fatalf("目录名分段正则不符: %q", expr)
	}
	dirRule := &draft.rules[1]
	if len(dirRule.Extractions) != 1 || dirRule.Extractions[0].Dimension != DimCollection ||
		dirRule.Extractions[0].From != FromCapture || dirRule.Extractions[0].CaptureName != seqCaptureGroup {
		t.Fatalf("目录格式条目应恰一条集名捕获提取: %+v", dirRule.Extractions)
	}
	if dirRule.Extractions[0].When != nil {
		t.Fatal("目录格式草稿默认不应带仅序列成员产出 when 原子")
	}
	// 命中预览：全库同格式目录逐个（本集 2+跨目录 2）；样例偏好跨目录。
	if sugg.HitTotal != 4 || sugg.HitCount != 4 {
		t.Fatalf("命中预览应覆盖全库 4 文件: %d/%d", sugg.HitCount, sugg.HitTotal)
	}
	if len(sugg.Samples) == 0 || sugg.Samples[0].DirRelKey == "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]" {
		t.Fatalf("样例应偏好跨目录同格式目录: %+v", sugg.Samples)
	}
	if !strings.Contains(sugg.Samples[0].Result, "別の題") {
		t.Fatalf("跨目录样例应取各自目录第 3 段为集名: %+v", sugg.Samples[0])
	}
	// 调整面默认指派与线索清单。
	if len(sugg.DefaultCollectionSegRefs) != 1 || sugg.DefaultCollectionSegRefs[0] != "setDirSeg3" {
		t.Fatalf("默认集名捕获应=末非数字目录段: %+v", sugg.DefaultCollectionSegRefs)
	}
	if len(sugg.DefaultVariableSegRefs) != 1 || sugg.DefaultVariableSegRefs[0] != "setDirSeg2" {
		t.Fatalf("数字段默认应标可变: %+v", sugg.DefaultVariableSegRefs)
	}
	if sugg.DefaultAuthorClueRef != "dirLevelStripped1" {
		t.Fatalf("默认作者指派应=一级目录剥壳: %q", sugg.DefaultAuthorClueRef)
	}
	if sugg.DefaultSeqSegRef != "" {
		t.Fatalf("无文件名共同形态时序号段不应默认指派: %q", sugg.DefaultSeqSegRef)
	}
	// 缺陷1 回归锚：目录格式草稿不默认勾「仅序列成员产出」（乱名成员非序列
	// 群，when 原子会令规则恒不产出=惰性规则）。
	if sugg.DefaultMemberOnly {
		t.Fatal("目录格式草稿不应默认勾选仅序列成员产出（DefaultMemberOnly 应为 false）")
	}
	byRef := make(map[string]SetRuleClue, len(sugg.Clues))
	for _, c := range sugg.Clues {
		byRef[c.Ref] = c
	}
	seg1, ok := byRef["setDirSeg1"]
	if !ok || seg1.Kind != setClueKindDirSeg || seg1.Label != "二级目录段 1" ||
		seg1.Value != "Isel" || seg1.Ordinal != 1 || !seg1.Shelled || seg1.Inner != "Isel" {
		t.Fatalf("目录名分段线索段 1 不符: %+v", seg1)
	}
	if seg2 := byRef["setDirSeg2"]; !ok || seg2.Value != "92242473" {
		t.Fatalf("目录名分段线索段 2 不符: %+v", byRef["setDirSeg2"])
	}
	if seg3 := byRef["setDirSeg3"]; !ok || seg3.Value != "瞬間的に絡み合う" {
		t.Fatalf("目录名分段线索段 3 不符: %+v", byRef["setDirSeg3"])
	}
}

// TestWorkSetDirDraftFilenameFormPriority 文件名共同形态优先不回归：同形序列
// 成员在场→照旧走序列形态草稿（形态短语/仅序列成员产出已勾/无目录格式条目）
// ——目录名分段仅以调整面线索行在场。
func TestWorkSetDirDraftFilenameFormPriority(t *testing.T) {
	root := t.TempDir()
	m, target := workSetRuleFixture(t, root)

	sugg, err := m.ApplyWorkSetRuleSummary(root, target)
	if err != nil {
		t.Fatalf("集规则草稿推导失败: %v", err)
	}
	if sugg.Failed || sugg.Shape != "[作者]_[作品号]_任意段_序号" {
		t.Fatalf("文件名共同形态在→照旧序列形态草稿: %+v", sugg)
	}
	if !sugg.MemberOnly || !sugg.DefaultMemberOnly {
		t.Fatal("序列形态草稿应默认勾选仅序列成员产出")
	}
	if sugg.DefaultAuthorClueRef != "" {
		t.Fatalf("序列形态草稿不应带默认作者指派: %q", sugg.DefaultAuthorClueRef)
	}
	for _, r := range sugg.Rules {
		if strings.Contains(r.ID, "setdir") {
			t.Fatalf("序列形态草稿不应含目录格式条目: %+v", r)
		}
	}
	found := false
	for _, c := range sugg.Clues {
		if c.Kind == setClueKindDirSeg {
			found = true
		}
	}
	if !found {
		t.Fatalf("调整面应含目录名分段线索行（可手动切换目录格式）: %+v", sugg.Clues)
	}
}

// TestWorkSetDirSegStabilityCheck 稳定格式判定（纯函数）：段数/包裹/分隔一致
// 才成稳定格式（数字段值可不同、类推断对全部目录聚合取交）；不一致时如实
// 返回中文原因。
func TestWorkSetDirSegStabilityCheck(t *testing.T) {
	// 一致：数字段值不同、非数字段值不同——同为稳定格式；数字类推断取交。
	plan, fail := newWorkSetDirSegPlan("a/[A] [92242473] [T]", []string{"a/[B] [123] [T2]"})
	if fail != "" {
		t.Fatalf("同格式目录应判稳定: %s", fail)
	}
	if len(plan.ordinals) != 3 || !plan.digit[plan.ordinals[1]] ||
		plan.digit[plan.ordinals[0]] || plan.digit[plan.ordinals[2]] {
		t.Fatalf("数字类推断不符（中段应数字类、两侧泛化段）: %+v", plan.digit)
	}
	if plan.level != 2 || plan.name != "[A] [92242473] [T]" {
		t.Fatalf("层深/目录名不符: %d %q", plan.level, plan.name)
	}
	if got := dirSegShapePhrase(plan); got != "第 2 层目录呈 `[任意] [数字] [任意]` 格式" {
		t.Fatalf("形态短语不符: %q", got)
	}
	// 类推断取交：同位一个非数字 → 该位归泛化段。
	plan, fail = newWorkSetDirSegPlan("a/[A] [92242473] [T]", []string{"a/[A] [X] [T]"})
	if fail != "" || plan.digit[plan.ordinals[1]] {
		t.Fatalf("数字类应聚合取交: fail=%s digit=%v", fail, plan.digit)
	}
	// 段数不同。
	if _, fail = newWorkSetDirSegPlan("a/[A] [123] [T]", []string{"a/[B] [123]"}); fail == "" ||
		!strings.Contains(fail, "段数不同") {
		t.Fatalf("段数不一致应失败: %q", fail)
	}
	// 分隔不同。
	if _, fail = newWorkSetDirSegPlan("a/[A] [123] [T]", []string{"a/[A]_[123]_[T]"}); fail == "" ||
		!strings.Contains(fail, "分隔不同") {
		t.Fatalf("分隔不一致应失败: %q", fail)
	}
	// 包裹不同。
	if _, fail = newWorkSetDirSegPlan("a/[A] [123] [T]", []string{"a/A 123 [T]"}); fail == "" ||
		!strings.Contains(fail, "包裹不同") {
		t.Fatalf("包裹不一致应失败: %q", fail)
	}
	// 库根（无所在目录名）。
	if _, fail = newWorkSetDirSegPlan("", nil); fail == "" ||
		!strings.Contains(fail, "无法从目录名推导稳定格式") {
		t.Fatalf("库根应失败: %q", fail)
	}
}

// TestWorkSetDirAdjustRuleAssembly 调整面目录段指派→规则组装（剧本D 步4）：
// 作者←目录段 1、集名←目录段 3、数字段可变 → 恰一条目录格式规则
// （LocationPattern 层深锚定+段格式正则带 author/collection 命名捕获、
// 提取 FromCapture）；校验拒绝混指/多段目录集名/目录段指派作品名；入档+
// 验证轮后成员与跨目录文件的集名/作者声明来源=新规则（各自目录各自成集、
// 作者=各自段 1）。
func TestWorkSetDirAdjustRuleAssembly(t *testing.T) {
	root := t.TempDir()
	m, target := workSetDirFixture(t, root)

	result, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		AuthorClueRef:     "setDirSeg1",
		CollectionSegRefs: []string{"setDirSeg3"},
		VariableSegRefs:   []string{"setDirSeg2"},
	})
	if err != nil {
		t.Fatalf("调整面保存失败: %v", err)
	}
	if len(result.Rules) != 1 || !result.VerificationStarted {
		t.Fatalf("目录段指派应恰一条目录格式规则且验证轮启动: %+v", result.Rules)
	}
	profile, _ := LoadProfile(m.store)
	rule := findRuleByID(t, profile, result.Rules[0].ID)
	loc, expr := dirCaptureExprOf(t, rule)
	if loc.MinDepth != 1 || loc.MaxDepth != 1 {
		t.Fatalf("目录格式规则应锚定第 2 层: %+v", loc)
	}
	if expr != `^\[(?P<author>[^\]]*)\] \[\d+\] \[(?P<collection>[^\]]*)\]$` {
		t.Fatalf("段格式正则不符: %q", expr)
	}
	if len(rule.Extractions) != 2 {
		t.Fatalf("应两条提取（集名+作者）: %+v", rule.Extractions)
	}
	for _, act := range rule.Extractions {
		if act.From != FromCapture {
			t.Fatalf("目录段提取应走 FromCapture: %+v", act)
		}
	}

	// 验证轮后：成员与跨目录文件的集名/作者声明来源=新规则。
	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.Dimension == string(DimAuthor) && d.RuleID == rule.ID {
				return true
			}
		}
		return false
	})
	cases := []struct {
		relPath, collection, author string
	}{
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "瞬間的に絡み合う", "Isel"},
		// 跨目录同格式接管：各自目录第 3 段成集、作者=各自段 1。
		{"Artist[Isel]/[Isel] [92242474] [別の題]/おまけ.jpg", "別の題", "Isel"},
	}
	for _, c := range cases {
		file := snapshotFileOf(sess.Snapshot, c.relPath)
		if file == nil {
			t.Fatalf("快照内无文件 %s", c.relPath)
		}
		var collection, author string
		for _, d := range file.Final {
			if d.RuleID != rule.ID {
				continue
			}
			switch MetadataDimension(d.Dimension) {
			case DimCollection:
				collection = d.Value
			case DimAuthor:
				author = d.Value
			}
		}
		if collection != c.collection || author != c.author {
			t.Fatalf("%s 声明不符：集名 %q（期望 %q）、作者 %q（期望 %q）", c.relPath, collection, c.collection, author, c.author)
		}
	}

	// 指派校验：文件名段与目录段混指、目录段多段集名、目录段指作品名。
	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{"setDirSeg3", "setSeg1"},
	}); err == nil || !strings.Contains(err.Error(), "同为") {
		t.Fatalf("混指应回可读错误: %v", err)
	}
	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{"setDirSeg1", "setDirSeg3"},
	}); err == nil || !strings.Contains(err.Error(), "单段") {
		t.Fatalf("目录段多段集名应回可读错误: %v", err)
	}
	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		TitleClueRef: "setDirSeg1",
	}); err == nil || !strings.Contains(err.Error(), "目录名分段") {
		t.Fatalf("目录段指作品名应回可读错误: %v", err)
	}
}

// TestWorkSetDirGeneralizeEndToEnd 目录格式草稿泛化入档端到端（剧本D 步3）：
// 草稿两规则（一级目录剥壳+目录格式）入档、验证轮后各同格式目录下文件声明
// （本集=瞬間的に絡み合う、跨目录=別の題——各自目录第 3 段；作者=各自 Artist
// 剥壳）、成员集维度赋值被消费剥离（集名捕获复现确认集名）。
func TestWorkSetDirGeneralizeEndToEnd(t *testing.T) {
	root := t.TempDir()
	m, target := workSetDirFixture(t, root)

	result, err := m.ApplyWorkSetRuleGeneralize(root, target)
	if err != nil {
		t.Fatalf("泛化入档失败: %v", err)
	}
	if len(result.Rules) != 2 || !result.VerificationStarted {
		t.Fatalf("泛化结果应两规则且验证轮启动: %+v", result.Rules)
	}
	authorID, captureID := result.Rules[0].ID, result.Rules[1].ID

	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.Dimension == string(DimCollection) && d.RuleID == captureID {
				return true
			}
		}
		return false
	})
	cases := []struct {
		relPath, collection, author string
	}{
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "瞬間的に絡み合う", "Isel"},
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/おまけ.jpg", "瞬間的に絡み合う", "Isel"},
		{"Artist[Isel]/[Isel] [92242474] [別の題]/表紙.png", "別の題", "Isel"},
		{"Artist[Isel]/[Isel] [92242474] [別の題]/おまけ.jpg", "別の題", "Isel"},
	}
	for _, c := range cases {
		file := snapshotFileOf(sess.Snapshot, c.relPath)
		if file == nil {
			t.Fatalf("快照内无文件 %s", c.relPath)
		}
		var collection, author string
		for _, d := range file.Final {
			switch {
			case d.RuleID == captureID && MetadataDimension(d.Dimension) == DimCollection:
				collection = d.Value
			case d.RuleID == authorID && MetadataDimension(d.Dimension) == DimAuthor:
				author = d.Value
			}
		}
		if collection != c.collection || author != c.author {
			t.Fatalf("%s 声明不符：集名 %q（期望 %q）、作者 %q（期望 %q）", c.relPath, collection, c.collection, author, c.author)
		}
	}
	// 集名捕获复现确认集名→成员集维度赋值被全消费删除。
	for _, rel := range []string{
		"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png",
		"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/おまけ.jpg",
	} {
		if _, kept := sess.Assignments[rel]; kept {
			t.Fatalf("成员 %s 的赋值应被全消费删除", rel)
		}
	}
}

// TestWorkSetDirAdjustSupersedesLineage 同系覆写（发现2 修复②·D4「效果同步」
// 直接修复）：泛化入档后对同一集再调整=修订意图——旧族规则（壳形作者条目+
// 目录格式条目）停用、新规则入档取代（旧规则不再产出旧值遮蔽新值）；重识别
// 后成员与跨目录文件的集名/作者声明全部来自新规则（声明翻新），谱系簿记翻新
// 为新族 ID。
func TestWorkSetDirAdjustSupersedesLineage(t *testing.T) {
	root := t.TempDir()
	m, target := workSetDirFixture(t, root)

	gen, err := m.ApplyWorkSetRuleGeneralize(root, target)
	if err != nil {
		t.Fatalf("泛化入档失败: %v", err)
	}
	if len(gen.Rules) != 2 {
		t.Fatalf("泛化结果应两规则: %+v", gen.Rules)
	}
	oldAuthorID, oldCaptureID := gen.Rules[0].ID, gen.Rules[1].ID

	// 再调整（作者改从目录段 1 取、集名=目录段 3）= 修订意图：旧族停用、
	// 新目录格式规则（author+collection 双捕获）取代。
	adj, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		AuthorClueRef:     "setDirSeg1",
		CollectionSegRefs: []string{"setDirSeg3"},
		VariableSegRefs:   []string{"setDirSeg2"},
	})
	if err != nil {
		t.Fatalf("调整面保存失败: %v", err)
	}
	if len(adj.Rules) != 1 {
		t.Fatalf("调整结果应恰一条目录格式规则: %+v", adj.Rules)
	}
	newID := adj.Rules[0].ID
	if newID == oldCaptureID || newID == oldAuthorID {
		t.Fatalf("新规则 ID 应为新入档条目: %q", newID)
	}
	profile, errs := LoadProfile(m.store)
	for _, err := range errs {
		t.Fatalf("载入规则档案异常: %v", err)
	}
	if findRuleByID(t, profile, oldAuthorID).Enabled {
		t.Fatalf("旧族作者条目 %s 应被停用", oldAuthorID)
	}
	if findRuleByID(t, profile, oldCaptureID).Enabled {
		t.Fatalf("旧族目录格式条目 %s 应被停用", oldCaptureID)
	}
	if !findRuleByID(t, profile, newID).Enabled {
		t.Fatalf("新规则 %s 应启用", newID)
	}

	// 重识别终态：声明翻新——四文件集名/作者全部来自新规则，旧族零产出。
	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.RuleID == newID && d.Dimension == string(DimAuthor) {
				return true
			}
		}
		return false
	})
	cases := []struct {
		relPath, collection, author string
	}{
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "瞬間的に絡み合う", "Isel"},
		{"Artist[Isel]/[Isel] [92242474] [別の題]/おまけ.jpg", "別の題", "Isel"},
	}
	for _, c := range cases {
		file := snapshotFileOf(sess.Snapshot, c.relPath)
		if file == nil {
			t.Fatalf("快照内无文件 %s", c.relPath)
		}
		var collection, author string
		for _, d := range file.Final {
			if d.RuleID == oldAuthorID || d.RuleID == oldCaptureID {
				t.Fatalf("%s 旧族规则不应再有产出（声明翻新）: %+v", c.relPath, d)
			}
			switch {
			case d.RuleID == newID && MetadataDimension(d.Dimension) == DimCollection:
				collection = d.Value
			case d.RuleID == newID && MetadataDimension(d.Dimension) == DimAuthor:
				author = d.Value
			}
		}
		if collection != c.collection || author != c.author {
			t.Fatalf("%s 声明应来自新规则：集名 %q（期望 %q）、作者 %q（期望 %q）", c.relPath, collection, c.collection, author, c.author)
		}
	}
	// 谱系簿记翻新：target → 新族 ID（旧族不再占据）。
	lineage := sess.WorkSetRuleLineage[target]
	if len(lineage) != 1 || lineage[0] != newID {
		t.Fatalf("谱系簿记应翻新为新族: %+v", lineage)
	}
}
