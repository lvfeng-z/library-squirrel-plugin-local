package main

// 集规则泛化入口（迭代三 · 剧本C）的单元测试：集整体推导（多成员取共同
// 形态、命中预览含本集成员——非首样本口径）、调整面线索清单（目录层剥壳
// 变体在场）、调整指派→规则组装（作者←目录剥壳 / 集名捕获选段 / 可变段 /
// 序号段默认指）、推导失败如实返回（含杂成员的目录候选——单首样本会漏）、
// 泛化入档+验证轮+声明接管端到端（本集成员与跨目录样例声明来源=新规则、
// 仅序列成员产出 when 原子入档）、target 缺席已归集的错误（剧本步5c 文案）
// 与前端协议分派。
import (
	"strings"
	"testing"
	"time"
)

// workSetRuleFixture 集规则测试环境（空规则档案——样本由结构群对齐成群）：
//   - `Artist[hakosaku]/合集盒/`：结构群三成员 `[作者]_[作品号]_AAA_BBB_[序号]`
//     （主体集——集名跨两段含分隔符，确认集名 AAA_BBB）；
//   - `别处/`：跨目录同形序列两成员（不同作者/作品号/段值 CCC_DDD）。
//
// 返回管理器与主体集（合集盒内结构群候选）的目标键（已确认归集）。
func workSetRuleFixture(t *testing.T, root string) (*SessionManager, string) {
	t.Helper()
	m, _ := newSeqRecognitionFixture(t, root)
	for _, n := range []string{"01", "02", "03"} {
		writeTestFile(t, root+"/Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_["+n+"].png", "a"+n)
	}
	writeTestFile(t, root+"/别处/[moka]_[987654321]_CCC_DDD_[05].jpg", "b")
	writeTestFile(t, root+"/别处/[moka]_[987654321]_CCC_DDD_[06].jpg", "c")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, confirmGroupCandidate(t, m, root, "Artist[hakosaku]/合集盒", "AAA_BBB")
}

// confirmGroupCandidate 找到指定目录内的群来源候选并确认归集，等待自动重识
// 别完成后返回目标键。
func confirmGroupCandidate(t *testing.T, m *SessionManager, root, dir, setName string) string {
	t.Helper()
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	var target string
	for i := range sess.Snapshot.Candidates {
		c := &sess.Snapshot.Candidates[i]
		if c.Source == candSourceGroup && c.Dir == dir {
			target = c.target()
			break
		}
	}
	if target == "" {
		t.Fatalf("候选主清单内无 %s 的群候选: %+v", dir, sess.Snapshot.Candidates)
	}
	if _, _, err := m.ConfirmWorkSetCandidate(root, target, setName, 0); err != nil {
		t.Fatalf("确认归集失败: %v", err)
	}
	waitWorkSetIdle(t, m, root)
	return target
}

// waitWorkSetIdle 等待确认动作触发的识别刷新完成（在途标记清零）。
func waitWorkSetIdle(t *testing.T, m *SessionManager, root string) {
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
			t.Fatal("确认动作触发的识别刷新超时未完成")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// waitWorkSetCond 轮询直至会话满足给定条件（验证轮在后台完成、快照呈现新
// 终选——不依赖 LastDiff 在场与否：确认动作自身也会产出差异，直接判差异
// 会拿到陈旧快照）。
func waitWorkSetCond(t *testing.T, m *SessionManager, root string, check func(*WorkbenchSession) bool) *WorkbenchSession {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sess, err := m.SessionOf(root); err == nil && check(sess) {
			return sess
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("等待验证轮呈现新终选超时")
	return nil
}

// TestWorkSetRuleSummarySetWideDraft 集整体推导（剧本步1）：共同形态人话、
// 规则草稿恰两条（sibling 形态跟随+集名捕获）、「仅序列成员产出」已勾、命中
// 预览含本集成员全量（3 成员+跨目录 2=5——单文件样本口径会排除样本自身）、
// 样例偏好跨目录；调整面默认指派=自动组合（3..4 段）+末段序号已指+其余段
// 标可变。
func TestWorkSetRuleSummarySetWideDraft(t *testing.T) {
	root := t.TempDir()
	m, target := workSetRuleFixture(t, root)

	sugg, err := m.ApplyWorkSetRuleSummary(root, target)
	if err != nil {
		t.Fatalf("集规则草稿推导失败: %v", err)
	}
	if sugg.Target != target || sugg.SetName != "AAA_BBB" || sugg.MemberCount != 3 {
		t.Fatalf("草稿头不符: %+v", sugg)
	}
	if len(sugg.Members) != 3 || sugg.Members[0].SeqText != "01" ||
		!strings.HasSuffix(sugg.Members[0].FileName, "[01].png") {
		t.Fatalf("成员清单不符: %+v", sugg.Members)
	}
	if sugg.Shape != "[作者]_[作品号]_任意段_序号" {
		t.Fatalf("共同形态不符: %q", sugg.Shape)
	}
	wantSummary := "同形序列成员（[作者]_[作品号]_任意段_序号 形态）→ 作品集 = 段组合『AAA_BBB』，仅序列成员产出"
	if sugg.Summary != wantSummary {
		t.Fatalf("人话摘要不符:\n取回 %q\n期望 %q", sugg.Summary, wantSummary)
	}
	if len(sugg.Mappings) != 1 || sugg.Mappings[0].From != "文件名第 3..末−1 段（含分隔符）" {
		t.Fatalf("映射行不符: %+v", sugg.Mappings)
	}
	if len(sugg.Rules) != 2 || !sugg.MemberOnly {
		t.Fatalf("规则草稿应恰两条且勾选仅序列成员产出: %+v", sugg.Rules)
	}
	if sugg.Failed || sugg.FailReason != "" {
		t.Fatalf("推导不应失败: %q", sugg.FailReason)
	}
	// 命中预览：全库 5 文件全部命中（含本集成员——集为主体，非首样本口径）。
	if sugg.HitCount != 5 || sugg.HitTotal != 5 {
		t.Fatalf("命中预览计数不符: hit=%d total=%d", sugg.HitCount, sugg.HitTotal)
	}
	if len(sugg.Samples) != 3 {
		t.Fatalf("样例应 3 条（跨目录优先+回落）: %+v", sugg.Samples)
	}
	cross := 0
	for _, s := range sugg.Samples {
		if s.DirRelKey == "别处" {
			cross++
		}
	}
	if cross < 2 || !strings.Contains(sugg.Samples[0].Result, "作品集=CCC_DDD") {
		t.Fatalf("样例应含跨目录两条且首条为跨目录求值结论: %+v", sugg.Samples)
	}
	// 调整面默认指派（剧本E：集名默认跟随识别形态——本集为序列形态识别，
	// 默认集名=序列行 seqKey〔群键/公共前缀〕，不再是文件名段组合）；序号=
	// 末段（第 5）；其余段（作者/作品号/集名段）全部标可变（集名不占段）。
	if len(sugg.DefaultCollectionSegRefs) != 1 || sugg.DefaultCollectionSegRefs[0] != "seqKey" {
		t.Fatalf("默认集名指派应为序列行 seqKey: %v", sugg.DefaultCollectionSegRefs)
	}
	if sugg.DefaultSeqSegRef != "setSeg5" {
		t.Fatalf("默认序号段应为末段序号元素: %q", sugg.DefaultSeqSegRef)
	}
	if len(sugg.DefaultVariableSegRefs) != 4 ||
		sugg.DefaultVariableSegRefs[0] != "setSeg1" || sugg.DefaultVariableSegRefs[3] != "setSeg4" {
		t.Fatalf("默认可变段应为全部非序号文件名段: %v", sugg.DefaultVariableSegRefs)
	}
	if !sugg.DefaultMemberOnly {
		t.Fatal("默认应勾选仅序列成员产出")
	}
}

// TestWorkSetRuleCluesIncludeStrippedDir 调整面线索清单（剧本步4 形态）：目录
// 各层（一级目录 Artist[hakosaku] 与剥壳变体 hakosaku 并列在场）、库根名、
// 成员文件名段（序号段单列类别）。
func TestWorkSetRuleCluesIncludeStrippedDir(t *testing.T) {
	root := t.TempDir()
	m, target := workSetRuleFixture(t, root)

	sugg, err := m.ApplyWorkSetRuleSummary(root, target)
	if err != nil {
		t.Fatalf("集规则草稿推导失败: %v", err)
	}
	byRef := make(map[string]SetRuleClue, len(sugg.Clues))
	for _, c := range sugg.Clues {
		byRef[c.Ref] = c
	}
	lvl, ok := byRef["dirLevel1"]
	if !ok || lvl.Value != "Artist[hakosaku]" || lvl.Kind != structClueDirLevel {
		t.Fatalf("一级目录线索不符: %+v", lvl)
	}
	stripped, ok := byRef["dirLevelStripped1"]
	if !ok || stripped.Value != "hakosaku" || stripped.Kind != structClueDirLevelStripped {
		t.Fatalf("一级目录剥壳变体线索应在场: %+v", stripped)
	}
	if _, ok := byRef["dirLevel2"]; !ok || byRef["dirLevel2"].Value != "合集盒" {
		t.Fatalf("二级目录线索应在场: %+v", byRef["dirLevel2"])
	}
	if _, ok := byRef[structClueRootName]; !ok {
		t.Fatal("库根名线索应在场")
	}
	seg, ok := byRef["setSeg3"]
	if !ok || seg.Kind != setClueKindSeg || seg.Value != "AAA" || seg.Ordinal != 3 || seg.Label != "文件名段 3" {
		t.Fatalf("文件名段线索不符: %+v", seg)
	}
	seq, ok := byRef["setSeg5"]
	if !ok || seq.Kind != setClueKindSeq || seq.Label != "序号段" || seq.Value != "[01]" || seq.Inner != "01" {
		t.Fatalf("序号段线索不符: %+v", seq)
	}
	if seg2 := byRef["setSeg1"]; !seg2.Shelled || seg2.Inner != "hakosaku" {
		t.Fatalf("括号组段应带剥壳内名: %+v", seg2)
	}
	// 序列行（剧本E 步1）：值=群键/公共前缀（括号序号形态键=序号元素前全部
	// 正文），序列形态在场（群卡反查成立）才列出。
	seqKey, ok := byRef[setClueSeqRef]
	if !ok || seqKey.Kind != setClueKindSeqKey || seqKey.Label != "序列" ||
		seqKey.Value != "[hakosaku]_[126794866]_AAA_BBB" {
		t.Fatalf("序列线索行不符: %+v", seqKey)
	}
}

// TestWorkSetRuleAdjustRuleAssembly 调整指派→规则组装（剧本步4）：作者←一级
// 目录剥壳（structDirPlan 目录条目：锚定 Artist[任意] + ancestorDir 提取剥壳）、
// 集名捕获选段（第 4 段——不同于确认集名的组合，跨目录段值差异被归一）、
// 可变段与序号段默认指派受认；三规则入档+验证轮后：成员作者声明=hakosaku
// （来源新目录规则）、跨目录文件集名声明=DDD（来源新捕获规则——选段归一）。
func TestWorkSetRuleAdjustRuleAssembly(t *testing.T) {
	root := t.TempDir()
	m, target := workSetRuleFixture(t, root)

	result, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		AuthorClueRef:     "dirLevelStripped1",
		CollectionSegRefs: []string{"setSeg4"},
		VariableSegRefs:   []string{"setSeg1", "setSeg2", "setSeg3"},
		SeqSegRef:         "setSeg5",
		MemberOnly:        true,
	})
	if err != nil {
		t.Fatalf("调整面保存失败: %v", err)
	}
	if len(result.Rules) != 3 || !result.VerificationStarted {
		t.Fatalf("调整规则应三条（sibling+目录层+文件名）且验证轮启动: %+v", result.Rules)
	}

	profile, _ := LoadProfile(m.store)
	dirRule := findRuleByID(t, profile, result.Rules[1].ID)
	// v4：目录层条目=目录行模板 Artist[{作者}]，剥壳语义由行 1 捕获承载。
	if _, ok := dirRule.Pattern.(*RowSubjectPattern); !ok || dirRule.Conditions == nil ||
		dirRule.Conditions[0].Condition.Format.Tpl != "Artist[{作者}]" {
		t.Fatalf("目录层条目应为 v4 目录行模板规则，实际 %T", dirRule.Pattern)
	}
	if len(dirRule.Extractions) != 1 {
		t.Fatalf("目录层条目应恰一条作者提取: %+v", dirRule.Extractions)
	}
	authAct := dirRule.Extractions[0]
	if authAct.Dimension != DimAuthor || authAct.From != FromCapture ||
		authAct.CaptureName != condxVocab["作者"] || authAct.Row != 1 {
		t.Fatalf("作者←目录行捕获动作不符: %+v", authAct)
	}

	captureRule := findRuleByID(t, profile, result.Rules[2].ID)
	if len(captureRule.Extractions) != 1 || captureRule.Extractions[0].CaptureName != seqCaptureGroup {
		t.Fatalf("文件名条目应恰一条集名捕获: %+v", captureRule.Extractions)
	}
	if captureRule.Extractions[0].When == nil {
		t.Fatal("仅序列成员产出条件应在场（MemberOnly=true）")
	}

	// 验证轮后：成员作者声明来源=新目录规则；跨目录集名声明=选段归一值 DDD
	//（来源新捕获规则）。
	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[01].png")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.Dimension == string(DimAuthor) && d.RuleID == dirRule.ID {
				return true
			}
		}
		return false
	})
	member := snapshotFileOf(sess.Snapshot, "Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[01].png")
	if member == nil {
		t.Fatal("验证轮后缺成员文件记录")
	}
	foundAuthor := false
	for _, d := range member.Final {
		if d.Dimension == string(DimAuthor) {
			if d.Value != "hakosaku" || d.RuleID != dirRule.ID {
				t.Fatalf("成员作者声明应=hakosaku 且来源新目录规则: %+v", d)
			}
			foundAuthor = true
		}
	}
	if !foundAuthor {
		t.Fatalf("成员应有作者声明: %+v", member.Final)
	}
	cross := snapshotFileOf(sess.Snapshot, "别处/[moka]_[987654321]_CCC_DDD_[05].jpg")
	if cross == nil {
		t.Fatal("验证轮后缺跨目录文件记录")
	}
	foundCross := false
	for _, d := range cross.Final {
		if d.Dimension == string(DimCollection) {
			if d.Value != "DDD" || d.RuleID != captureRule.ID {
				t.Fatalf("跨目录集名声明应=选段归一值 DDD 且来源新捕获规则: %+v", d)
			}
			foundCross = true
		}
	}
	if !foundCross {
		t.Fatalf("跨目录文件应有集名声明: %+v", cross.Final)
	}
	// 选段捕获值（BBB）≠ 确认集名（AAA_BBB）→ 成员集维度赋值不被消费（特异
	// 性赋值压过普适规则，保持在场）。
	if _, kept := sess.Assignments[member.RelPath]; !kept {
		t.Fatal("选段改变的捕获不复现确认集名时，成员赋值应保留不被消费")
	}
}

// TestWorkSetRuleAdjustInputValidation 调整面指派校验：集名捕获组合须连续段、
// 序号段指派须为序号段线索、目录锚定须为目录层线索。
func TestWorkSetRuleAdjustInputValidation(t *testing.T) {
	root := t.TempDir()
	m, target := workSetRuleFixture(t, root)

	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{"setSeg3", "setSeg5"},
	}); err == nil || !strings.Contains(err.Error(), "连续") {
		t.Fatalf("非连续组合应回可读错误: %v", err)
	}
	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		SeqSegRef: "setSeg3",
	}); err == nil || !strings.Contains(err.Error(), "序号段") {
		t.Fatalf("序号段指派到普通段应回可读错误: %v", err)
	}
	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		DirAnchorRef: "setSeg1",
	}); err == nil || !strings.Contains(err.Error(), "目录层") {
		t.Fatalf("目录锚定指派到文件名段应回可读错误: %v", err)
	}
}

// TestWorkSetRuleDerivationFailsTruthfully 推导失败如实返回（剧本步5a；迭代四
// 起双路都败才成失败态）：目录候选整目录归集（目录名含作品号）带杂成员
// （封面.png 不合同形）——文件名侧全员校验失败；目录名分段侧全为数字类、无
// 标题段可指集名——两路原因并列呈现（点名不合形成员+目录侧原因）。Failed
// 态仍携带线索清单与序号段默认指派（调整规则仍可用），泛化入档回错误
// （前端置灰对应）。
func TestWorkSetRuleDerivationFailsTruthfully(t *testing.T) {
	root := t.TempDir()
	m, _ := newSeqRecognitionFixture(t, root)
	writeTestFile(t, root+"/[126794866]/[画师乙]_[126794866]_合集名_[01].png", "a")
	writeTestFile(t, root+"/[126794866]/[画师乙]_[126794866]_合集名_[02].png", "b")
	writeTestFile(t, root+"/[126794866]/封面.png", "c")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	var target string
	for i := range sess.Snapshot.Candidates {
		c := &sess.Snapshot.Candidates[i]
		if c.Source == candSourceDir && c.Dir == "[126794866]" {
			target = c.target()
			break
		}
	}
	if target == "" {
		t.Fatalf("主清单应含目录候选: %+v", sess.Snapshot.Candidates)
	}
	if _, _, err := m.ConfirmWorkSetCandidate(root, target, "合集名", 0); err != nil {
		t.Fatalf("确认归集失败: %v", err)
	}
	waitWorkSetIdle(t, m, root)

	sugg, err := m.ApplyWorkSetRuleSummary(root, target)
	if err != nil {
		t.Fatalf("推导失败应回 Failed 态而非错误: %v", err)
	}
	if !sugg.Failed || !strings.Contains(sugg.FailReason, "封面.png") ||
		!strings.Contains(sugg.FailReason, "无法从目录名推导稳定格式") {
		t.Fatalf("失败原因应并列点名文件名侧不合形成员与目录侧不可推导原因: %+v", sugg.FailReason)
	}
	if len(sugg.Rules) != 0 || sugg.Summary != "" {
		t.Fatalf("失败态不应携带规则草稿: %+v", sugg)
	}
	// 调整规则仍可用：线索清单与序号段默认指派照发。
	if len(sugg.Clues) == 0 || sugg.DefaultSeqSegRef == "" {
		t.Fatalf("失败态仍应携带调整面线索清单: %+v", sugg.Clues)
	}
	// 泛化入档回错误（面板「泛化成规则」置灰的后端对应）。
	if _, err := m.ApplyWorkSetRuleGeneralize(root, target); err == nil {
		t.Fatal("推导失败的集泛化入档应回错误")
	}
}

// TestWorkSetRuleGeneralizeEndToEnd 泛化入档+验证轮+声明接管端到端（剧本步3）
// ：草稿两规则入档（sibling+集名捕获，仅序列成员产出 when 原子=groupSeq
// range min1 形态）、集成员集维度赋值被消费剥离（全消费删除）、验证轮后本
// 集成员与跨目录样例的集名声明来源=新捕获规则（声明接管）、档案内规则可见。
func TestWorkSetRuleGeneralizeEndToEnd(t *testing.T) {
	root := t.TempDir()
	m, target := workSetRuleFixture(t, root)

	result, err := m.ApplyWorkSetRuleGeneralize(root, target)
	if err != nil {
		t.Fatalf("集规则泛化入档失败: %v", err)
	}
	if len(result.Rules) != 2 || !result.VerificationStarted {
		t.Fatalf("泛化结果应两规则且验证轮启动: %+v", result.Rules)
	}
	captureID := result.Rules[1].ID

	profile, _ := LoadProfile(m.store)
	capture := findRuleByID(t, profile, captureID)
	when := capture.Extractions[0].When
	if when == nil || when.Kind != ExprRange || when.Operand == nil || when.Operand.Kind != ExprGroupSeq ||
		when.Min == nil || *when.Min != 1 {
		t.Fatalf("仅序列成员产出 when 原子形态不符（规则编辑面复选判读口径）: %+v", when)
	}

	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[01].png")
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
	for _, rel := range []string{
		"Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[01].png",
		"Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[02].png",
		"Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[03].png",
		"别处/[moka]_[987654321]_CCC_DDD_[05].jpg",
		"别处/[moka]_[987654321]_CCC_DDD_[06].jpg",
	} {
		file := snapshotFileOf(sess.Snapshot, rel)
		if file == nil {
			t.Fatalf("快照内无文件 %s", rel)
		}
		found := false
		for _, decl := range file.Final {
			if decl.Dimension == string(DimCollection) && decl.RuleID == captureID {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s 的集名声明来源应为新规则 %s（终选: %+v）", rel, captureID, file.Final)
		}
	}
	// 集成员赋值全消费删除（collection 是唯一维度，泛化即接管）。
	for _, rel := range []string{
		"Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[01].png",
		"Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[02].png",
		"Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[03].png",
	} {
		if _, kept := sess.Assignments[rel]; kept {
			t.Fatalf("成员 %s 的赋值应被全消费删除", rel)
		}
	}
}

// TestWorkSetRuleTargetNotConfirmed target 缺席已归集（剧本步5c）：三动作统一
// 回「集不在已归集清单内（可能已被识别结构变化移除）」，会话不动。
func TestWorkSetRuleTargetNotConfirmed(t *testing.T) {
	root := t.TempDir()
	m, _ := workSetRuleFixture(t, root)
	const bogus = "group\x00不存在\x00无此集"

	if _, err := m.ApplyWorkSetRuleSummary(root, bogus); err == nil ||
		!strings.Contains(err.Error(), errSetNotConfirmed.Error()) {
		t.Fatalf("草稿推导应回缺席错误: %v", err)
	}
	if _, err := m.ApplyWorkSetRuleGeneralize(root, bogus); err == nil ||
		!strings.Contains(err.Error(), errSetNotConfirmed.Error()) {
		t.Fatalf("泛化入档应回缺席错误: %v", err)
	}
	if _, err := m.ApplyWorkSetRuleAdjust(root, bogus, setRuleAdjustInput{MemberOnly: true}); err == nil ||
		!strings.Contains(err.Error(), errSetNotConfirmed.Error()) {
		t.Fatalf("调整面保存应回缺席错误: %v", err)
	}
}

// TestHandleFrontendRequestWorkSetRuleActions 前端协议分派：summarizeWorkSetRule
// 应答集规则草稿；缺席 target 回 5c 文案错误。
func TestHandleFrontendRequestWorkSetRuleActions(t *testing.T) {
	root := t.TempDir()
	m, ctx := newSeqRecognitionFixture(t, root)
	writeTestFile(t, root+"/Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[01].png", "a")
	writeTestFile(t, root+"/Artist[hakosaku]/合集盒/[hakosaku]_[126794866]_AAA_BBB_[02].png", "b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	var target string
	for i := range sess.Snapshot.Candidates {
		if c := &sess.Snapshot.Candidates[i]; c.Source == candSourceGroup {
			target = c.target()
			break
		}
	}
	if _, _, err := m.ConfirmWorkSetCandidate(root, target, "AAA_BBB", 0); err != nil {
		t.Fatalf("确认归集失败: %v", err)
	}
	waitWorkSetIdle(t, m, root)

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 71, Action: workbenchActionSummarizeWorkSetRule, Root: root, Target: target,
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.SetRuleSuggestion == nil || resp.SetRuleSuggestion.Failed ||
		resp.SetRuleSuggestion.SetName != "AAA_BBB" {
		t.Fatalf("summarizeWorkSetRule 应答不符: %+v", resp)
	}
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 72, Action: workbenchActionSummarizeWorkSetRule, Root: root, Target: "group\x00无\x00无",
	}))
	if resp := ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, errSetNotConfirmed.Error()) {
		t.Fatalf("缺席 target 应回 5c 文案: %+v", resp)
	}
}
