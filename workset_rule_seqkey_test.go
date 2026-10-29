package main

// 集名默认跟随识别形态（迭代五 · 剧本E）的单元测试：序列行（seqKey 词汇）
// 列出与默认集名指派=群键、目录形态默认不变、两形态同场默认=候选产出形态那
// 族（群卡/目录轨来源判定）、目录段多段连续组合（段1+段2、段1..段3——段间
// 原文保留）、不连续组合可读错误、「序列」指给集名的规则构造与消费
//（sibling 形态键=集名捕获，FromGroupPrefix 端到端）、集名三族互斥校验。
import (
	"strings"
	"testing"
)

// workSetSeqKeyFixture 序列前缀指派测试环境：`集盒/` 内结构群两成员（括号
// 序号形态，群键=[画师]_[123456789]_[集名]），确认集名=群键（集名=整键形态
// ——成员赋值可被新规则复现消费）；`他盒/` 同形态异键两成员（跨目录消费样本
// ——各群各自前缀）。
func workSetSeqKeyFixture(t *testing.T, root string) (*SessionManager, string) {
	t.Helper()
	m, _ := newSeqRecognitionFixture(t, root)
	for _, n := range []string{"01", "02"} {
		writeTestFile(t, root+"/集盒/[画师]_[123456789]_[集名]_["+n+"].png", "a"+n)
		writeTestFile(t, root+"/他盒/[画师乙]_[987654321]_[他集]_["+n+"].png", "b"+n)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, confirmGroupCandidate(t, m, root, "集盒", "[画师]_[123456789]_[集名]")
}

// TestWorkSetDraftFollowsCandidateSource 两形态同场默认=候选产出形态那族
// （剧本E 步1/步3）：群卡产出的候选（文件名共同形态+目录名稳定格式都在）→
// 草稿与默认集名指派跟随序列族（seqKey）；目录轨产出的候选（成员另构成同形
// 序列群——两形态同场）→ 跟随目录族（标题段，默认不变）；两族线索行同场
// 都在（目录段行+序列行+文件名段行）。
func TestWorkSetDraftFollowsCandidateSource(t *testing.T) {
	// 群卡来源：workSetRuleFixture 的集盒目录名「合集盒」呈稳定格式（单标
	// 题段）——目录侧可推导；文件名共同形态在——序列侧可推导；候选出自群卡
	// → 序列族。
	root := t.TempDir()
	m, target := workSetRuleFixture(t, root)
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	ws, err := loadWorkSetContext(sess, target)
	if err != nil {
		t.Fatalf("读取集上下文失败: %v", err)
	}
	if _, seqFail := deriveWorkSetSeqDraft(sess.Snapshot, ws); seqFail != "" {
		t.Fatalf("群卡来源场景序列形态应可推导: %s", seqFail)
	}
	if _, dirFail := deriveWorkSetDirDraft(sess.Snapshot, ws); dirFail != "" {
		t.Fatalf("群卡来源场景目录格式应同时可推导（两形态同场）: %s", dirFail)
	}
	sugg, err := m.ApplyWorkSetRuleSummary(root, target)
	if err != nil {
		t.Fatalf("集规则草稿推导失败: %v", err)
	}
	if sugg.Failed {
		t.Fatalf("两形态同场不应失败态: %s", sugg.FailReason)
	}
	if len(sugg.DefaultCollectionSegRefs) != 1 || sugg.DefaultCollectionSegRefs[0] != setClueSeqRef {
		t.Fatalf("群卡来源默认集名应=序列族 seqKey: %v", sugg.DefaultCollectionSegRefs)
	}
	if !sugg.DefaultMemberOnly {
		t.Fatal("序列族草稿应默认勾选仅序列成员产出")
	}
	kinds := map[string]bool{}
	for _, c := range sugg.Clues {
		kinds[c.Kind] = true
	}
	if !kinds[setClueKindDirSeg] || !kinds[setClueKindSeqKey] || !kinds[setClueKindSeg] {
		t.Fatalf("两形态同场应两族线索行同场（目录段+序列+文件名段）: %+v", sugg.Clues)
	}

	// 目录轨来源：成员文件名另构成同形序列群（尾缀数字形态，群键=titulo——
	// 序列侧可推导）、目录名含作品号呈稳定格式（目录侧可推导）；候选出自
	// 目录轨 → 目录族（默认=标题段，目录形态默认不变）。
	root2 := t.TempDir()
	m2, _ := newSeqRecognitionFixture(t, root2)
	writeTestFile(t, root2+"/Artist[P]/[P] [123456789] [Titulo]/titulo_01.png", "a")
	writeTestFile(t, root2+"/Artist[P]/[P] [123456789] [Titulo]/titulo_02.png", "b")
	if err := m2.runFullRecognition(root2); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	dirTarget := confirmDirCandidate(t, m2, root2, "Artist[P]/[P] [123456789] [Titulo]", "titulo")
	sess2, err := m2.SessionOf(root2)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	ws2, err := loadWorkSetContext(sess2, dirTarget)
	if err != nil {
		t.Fatalf("读取集上下文失败: %v", err)
	}
	if _, seqFail := deriveWorkSetSeqDraft(sess2.Snapshot, ws2); seqFail != "" {
		t.Fatalf("目录轨来源场景序列形态应可推导（两形态同场）: %s", seqFail)
	}
	draft, failReason := deriveWorkSetDraft(sess2.Snapshot, ws2)
	if failReason != "" || draft.kind != draftDir {
		t.Fatalf("目录轨来源应选目录格式草稿: kind=%d fail=%q", draft.kind, failReason)
	}
	sugg2, err := m2.ApplyWorkSetRuleSummary(root2, dirTarget)
	if err != nil {
		t.Fatalf("集规则草稿推导失败: %v", err)
	}
	if sugg2.Failed {
		t.Fatalf("两形态同场不应失败态: %s", sugg2.FailReason)
	}
	if len(sugg2.DefaultCollectionSegRefs) != 1 || sugg2.DefaultCollectionSegRefs[0] != "setDirSeg3" {
		t.Fatalf("目录轨来源默认集名应=目录族标题段（默认不变）: %v", sugg2.DefaultCollectionSegRefs)
	}
	if sugg2.DefaultMemberOnly {
		t.Fatal("目录族草稿不应默认勾选仅序列成员产出")
	}
	foundSeqRow := false
	for _, c := range sugg2.Clues {
		if c.Kind == setClueKindSeqKey {
			foundSeqRow = true
			if c.Value != "titulo" {
				t.Fatalf("序列行值应为群键: %+v", c)
			}
		}
	}
	if !foundSeqRow {
		t.Fatalf("两形态同场应列出序列行（可改指另一族）: %+v", sugg2.Clues)
	}
}

// TestWorkSetSeqKeyCollectionAdjust 「序列」指给集名的规则构造与赋值保留（剧本E
// 实施要点）：调整面集名=seqKey → sibling 定群条目 + 序列前缀条目（提取
// FromGroupPrefix，sibling 形态键即集名捕获）；入档+验证轮后成员集名声明由
// 赋值覆盖承载、作者与跨目录同形态文件的声明=各自群组公共前缀、来源=新规则
// （2026-10-25 裁决：赋值保留双源互证，废除消费剥离）。作者←seqKey 同条目
// 共载（与文件名段行同权限）。
func TestWorkSetSeqKeyCollectionAdjust(t *testing.T) {
	root := t.TempDir()
	m, target := workSetSeqKeyFixture(t, root)

	result, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{setClueSeqRef},
		AuthorClueRef:     setClueSeqRef,
		SeqSegRef:         "setSeg4",
		MemberOnly:        true,
	})
	if err != nil {
		t.Fatalf("调整面保存失败: %v", err)
	}
	if len(result.Rules) != 2 || !result.VerificationStarted {
		t.Fatalf("序列指派应恰两条（sibling 定群+序列前缀）且验证轮启动: %+v", result.Rules)
	}
	profile, _ := LoadProfile(m.store)
	seqKeyRule := findRuleByID(t, profile, result.Rules[1].ID)
	if !strings.HasPrefix(seqKeyRule.ID, "qna-setgrp-") {
		t.Fatalf("序列前缀条目 ID 前缀不符: %q", seqKeyRule.ID)
	}
	if len(seqKeyRule.Extractions) != 2 ||
		seqKeyRule.Extractions[0].Dimension != DimCollection || seqKeyRule.Extractions[0].From != FromGroupPrefix ||
		seqKeyRule.Extractions[1].Dimension != DimAuthor || seqKeyRule.Extractions[1].From != FromGroupPrefix {
		t.Fatalf("序列前缀条目应为集名+作者两条 FromGroupPrefix 提取: %+v", seqKeyRule.Extractions)
	}
	// v4：序列前缀条目=序列行模板（与定群条目同款 seqMemberTemplateOf）。
	if _, ok := seqKeyRule.Pattern.(*RowSubjectPattern); !ok || seqKeyRule.Conditions == nil ||
		seqKeyRule.Conditions[0].Condition.Target != CondxTargetSeq {
		t.Fatalf("序列前缀条目应为 v4 序列行条件规则: %T", seqKeyRule.Pattern)
	}
	siblingRule := findRuleByID(t, profile, result.Rules[0].ID)
	if !isGroupingRule(siblingRule) || siblingRule.Conditions == nil {
		t.Fatalf("首条应为 v4 序列条件定群条目（FromGroupPrefix 的定群供给）: %T", siblingRule.Pattern)
	}

	// 验证轮后：成员集名声明由赋值覆盖承载（2026-10-25 裁决赋值保留）、作者由
	// 新规则承载；跨目录同形态文件（各群各自前缀）两维均=新规则。等待条件须
	// 绑定泛化后才成立的跨目录规则命中（成员按值等待会抓到确认轮旧快照）。
	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "他盒/[画师乙]_[987654321]_[他集]_[01].png")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.RuleID == seqKeyRule.ID && d.Dimension == string(DimCollection) {
				return true
			}
		}
		return false
	})
	cases := []struct {
		relPath, collection, author string
		fromRule                    bool
	}{
		{"集盒/[画师]_[123456789]_[集名]_[01].png", "[画师]_[123456789]_[集名]", "[画师]_[123456789]_[集名]", false},
		{"集盒/[画师]_[123456789]_[集名]_[02].png", "[画师]_[123456789]_[集名]", "[画师]_[123456789]_[集名]", false},
		// 跨目录同形态接管：各群键=各自公共前缀。
		{"他盒/[画师乙]_[987654321]_[他集]_[01].png", "[画师乙]_[987654321]_[他集]", "[画师乙]_[987654321]_[他集]", true},
	}
	for _, c := range cases {
		file := snapshotFileOf(sess.Snapshot, c.relPath)
		if file == nil {
			t.Fatalf("快照内无文件 %s", c.relPath)
		}
		collection, author := "", ""
		for _, d := range file.Final {
			if c.fromRule && d.RuleID != seqKeyRule.ID {
				continue
			}
			// 成员（fromRule=false）：两维均走赋值覆盖（赋值保留）
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
	// 赋值保留（废除消费剥离）：成员赋值在场并置已泛化标记。
	for _, rel := range []string{
		"集盒/[画师]_[123456789]_[集名]_[01].png",
		"集盒/[画师]_[123456789]_[集名]_[02].png",
	} {
		assign, kept := sess.Assignments[rel]
		if !kept || !assign.Generalized {
			t.Fatalf("成员 %s 的赋值应保留并置已泛化标记: %+v", rel, assign)
		}
	}

	// 集名三族互斥（剧本E 步4）：序列与文件名段/目录段混指回可读错误。
	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{setClueSeqRef, "setSeg1"},
	}); err == nil || !strings.Contains(err.Error(), "一个维度只能有一个来源") {
		t.Fatalf("序列+文件名段混指应回可读错误: %v", err)
	}
	root2 := t.TempDir()
	m2, dirTarget := workSetDirFixture(t, root2)
	if _, err := m2.ApplyWorkSetRuleAdjust(root2, dirTarget, setRuleAdjustInput{
		CollectionSegRefs: []string{setClueSeqRef, "setDirSeg1"},
	}); err == nil || !strings.Contains(err.Error(), "一个维度只能有一个来源") {
		t.Fatalf("序列+目录段混指应回可读错误: %v", err)
	}
}

// workSetDirComboFixture 目录段组合测试环境（workSetDirFixture 同款样例树）：
// 确认集名=组合期望值（段组合值与确认集名一致 → 成员赋值可被新规则复现
// 消费，终选声明来源=新规则——端到端断言口径）。
func workSetDirComboFixture(t *testing.T, root, confirmName string) (*SessionManager, string) {
	t.Helper()
	m, _ := newSeqRecognitionFixture(t, root)
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "a")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/おまけ.jpg", "b")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242474] [別の題]/表紙.png", "c")
	writeTestFile(t, root+"/Artist[Isel]/[Isel] [92242474] [別の題]/おまけ.jpg", "d")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, confirmDirCandidate(t, m, root, "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]", confirmName)
}

// TestWorkSetDirMultiSegCollection 目录段多段连续组合（剧本E 步2）：段1+段2
// 与段1..段3 两形态——捕获组覆盖连续段组合（含段间分隔原文，镜像文件名段
// span 语义：边界括号一并入组、惰性类+尾部段类 `$` 锚收界）；入档+验证轮后
// 成员与跨目录同格式目录按各自目录原文成集；作者段与组合段互斥校验。
func TestWorkSetDirMultiSegCollection(t *testing.T) {
	root := t.TempDir()
	m, target := workSetDirComboFixture(t, root, "[Isel] [92242473]")

	// 段1+段2：捕获值=「[Isel] [92242473]」（段间空格保留）。
	result, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{"setDirSeg1", "setDirSeg2"},
	})
	if err != nil {
		t.Fatalf("调整面保存失败: %v", err)
	}
	if len(result.Rules) != 1 || !result.VerificationStarted {
		t.Fatalf("目录段组合指派应恰一条目录格式规则且验证轮启动: %+v", result.Rules)
	}
	profile, _ := LoadProfile(m.store)
	rule := findRuleByID(t, profile, result.Rules[0].ID)
	// v4：组合 span {集名} 一位盖段 1..2 区间原文（边界括号与段间空格一并
	// 入位），第 3 段泛化消费（剥壳括号留字面）。
	if _, ok := rule.Pattern.(*RowSubjectPattern); !ok || rule.Conditions == nil {
		t.Fatalf("目录段组合规则应为 v4 目录行条件规则，实际 %T", rule.Pattern)
	}
	if tpl := rule.Conditions[0].Condition.Format.Tpl; tpl != "{集名} [{任意}]" {
		t.Fatalf("段1+段2 组合模板不符: %q", tpl)
	}
	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "Artist[Isel]/[Isel] [92242474] [別の題]/おまけ.jpg")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.RuleID == rule.ID && d.Dimension == string(DimCollection) {
				return d.Value == "[Isel] [92242474]"
			}
		}
		return false
	})
	// 成员集名由赋值覆盖承载（赋值保留）；跨目录同格式接管=新规则承载。
	cases := []struct {
		relPath, collection string
		fromRule            bool
	}{
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png", "[Isel] [92242473]", false},
		{"Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/おまけ.jpg", "[Isel] [92242473]", false},
		// 跨目录同格式接管：各自目录段 1..2 原文成集。
		{"Artist[Isel]/[Isel] [92242474] [別の題]/おまけ.jpg", "[Isel] [92242474]", true},
	}
	for _, c := range cases {
		file := snapshotFileOf(sess.Snapshot, c.relPath)
		if file == nil {
			t.Fatalf("快照内无文件 %s", c.relPath)
		}
		found := false
		for _, d := range file.Final {
			if MetadataDimension(d.Dimension) != DimCollection {
				continue
			}
			if c.fromRule != (d.RuleID == rule.ID) {
				continue
			}
			found = true
			if d.Value != c.collection {
				t.Fatalf("%s 集名应=%q（段间原文保留）: %q", c.relPath, c.collection, d.Value)
			}
		}
		if !found {
			t.Fatalf("%s 应有集名声明（终选: %+v）", c.relPath, file.Final)
		}
	}

	// 段1..段3：整目录名捕获（v4：{集名} 一位盖整名），确认集名=整目录名
	//（端到端同口径）。
	root2 := t.TempDir()
	m2, target2 := workSetDirComboFixture(t, root2, "[Isel] [92242473] [瞬間的に絡み合う]")
	result2, err := m2.ApplyWorkSetRuleAdjust(root2, target2, setRuleAdjustInput{
		CollectionSegRefs: []string{"setDirSeg1", "setDirSeg2", "setDirSeg3"},
	})
	if err != nil {
		t.Fatalf("调整面保存失败: %v", err)
	}
	rule2 := findRuleByID(t, LoadProfileOrFail(t, m2), result2.Rules[0].ID)
	if _, ok := rule2.Pattern.(*RowSubjectPattern); !ok || rule2.Conditions == nil ||
		rule2.Conditions[0].Condition.Format.Tpl != "{集名}" {
		t.Fatalf("段1..段3 组合模板不符: %T %+v", rule2.Pattern, rule2.Conditions)
	}
	sess2 := waitWorkSetCond(t, m2, root2, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "Artist[Isel]/[Isel] [92242474] [別の題]/表紙.png")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.RuleID == rule2.ID && d.Dimension == string(DimCollection) {
				return d.Value == "[Isel] [92242474] [別の題]"
			}
		}
		return false
	})
	// 主体集成员：集名由赋值覆盖承载（赋值保留）
	file2 := snapshotFileOf(sess2.Snapshot, "Artist[Isel]/[Isel] [92242473] [瞬間的に絡み合う]/表紙.png")
	if file2 == nil {
		t.Fatal("快照内无主体集成员")
	}
	found2 := false
	for _, d := range file2.Final {
		if MetadataDimension(d.Dimension) != DimCollection {
			continue
		}
		found2 = true
		if d.Value != "[Isel] [92242473] [瞬間的に絡み合う]" {
			t.Fatalf("主体集集名应=整目录名: %q", d.Value)
		}
	}
	if !found2 {
		t.Fatalf("主体集成员应有集名声明（终选: %+v）", file2.Final)
	}

	// 作者段落在组合区间内 → 互斥可读错误（一个维度只能有一个来源）。
	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{"setDirSeg1", "setDirSeg2"},
		AuthorClueRef:     "setDirSeg2",
	}); err == nil || !strings.Contains(err.Error(), "一个维度只能有一个来源") {
		t.Fatalf("组合段与作者段同段应回互斥错误: %v", err)
	}
}

// TestWorkSetSeqKeyAdjustAfterGeneralize 泛化入档后的集再调整「序列」指派
// （live-test 发现 1 回归锚）：泛化入档（C 链路，结构群 FORM 分支）→ 验证轮后
// 同形态集的快照群变规则定群（ruleId 在场、形态标识全空）→ 调整面 seqKey 指
// 集名经 buildSequenceRule 回退分支构造——序号提取式须容忍括号包裹（`]` 阻断
// 数字与扩展名相邻曾令定群门全员 false、保存被拒）；入档+验证轮后集名=群键
// 的声明链路照常（本集与跨目录各群各自键）。
func TestWorkSetSeqKeyAdjustAfterGeneralize(t *testing.T) {
	root := t.TempDir()
	m, target := workSetSeqKeyFixture(t, root)

	// 泛化（C 链路）：结构群形态的序列草稿入档，验证轮启动。
	genResult, err := m.ApplyWorkSetRuleGeneralize(root, target)
	if err != nil {
		t.Fatalf("泛化入档失败: %v", err)
	}
	if len(genResult.Rules) != 2 || !genResult.VerificationStarted {
		t.Fatalf("泛化应入档两条（sibling 定群+集名捕获）且验证轮启动: %+v", genResult.Rules)
	}
	// 验证轮后：快照群变规则定群（泛化规则定群产出，形态标识全空）——回退
	// 分支前提成立。
	sess := waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		ws, err := loadWorkSetContext(s, target)
		if err != nil {
			return false
		}
		g := snapshotGroupOf(s.Snapshot, ws.cand.Members[ws.repIdx].RelPath)
		return g != nil && g.RuleID != "" && g.FormName == ""
	})
	ws, err := loadWorkSetContext(sess, target)
	if err != nil {
		t.Fatalf("读取集上下文失败: %v", err)
	}
	ruleGroup := snapshotGroupOf(sess.Snapshot, ws.cand.Members[ws.repIdx].RelPath)
	if ruleGroup == nil || ruleGroup.FormName != "" || ruleGroup.RuleID == "" {
		t.Fatalf("泛化入档后应为规则定群（回退分支前提）: formName=%q ruleId=%q", ruleGroup.FormName, ruleGroup.RuleID)
	}

	// 调整面：seqKey 指集名（E1 默认链路）——规则定群态，此前在定群门被拒。
	adjResult, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{setClueSeqRef},
		SeqSegRef:         "setSeg4",
		MemberOnly:        true,
	})
	if err != nil {
		t.Fatalf("规则定群态的序列指派应可通过定群门: %v", err)
	}
	if len(adjResult.Rules) != 2 || !adjResult.VerificationStarted {
		t.Fatalf("序列指派应恰两条（回退式 sibling 定群+序列前缀）且验证轮启动: %+v", adjResult.Rules)
	}
	profile, _ := LoadProfile(m.store)
	siblingRule := findRuleByID(t, profile, adjResult.Rules[0].ID)
	if !isGroupingRule(siblingRule) || siblingRule.Conditions == nil {
		t.Fatalf("回退式定群应为 v4 序列条件规则，实际 %T", siblingRule.Pattern)
	}
	if tpl := siblingRule.Conditions[0].Condition.Format.Tpl; tpl != "{公共段}_[{序号}]" {
		t.Fatalf("回退式成员模板不符（包裹符入字面段）: %q", tpl)
	}

	// 验证轮后：本集与跨目录同形态文件的集名声明=各自群键（回退式定群供给
	// FromGroupPrefix 提取）。
	sess = waitWorkSetCond(t, m, root, func(s *WorkbenchSession) bool {
		f := snapshotFileOf(s.Snapshot, "他盒/[画师乙]_[987654321]_[他集]_[01].png")
		if f == nil {
			return false
		}
		for _, d := range f.Final {
			if d.Dimension == string(DimCollection) && d.Value == "[画师乙]_[987654321]_[他集]" {
				return true
			}
		}
		return false
	})
	for _, c := range []struct{ relPath, collection string }{
		{"集盒/[画师]_[123456789]_[集名]_[01].png", "[画师]_[123456789]_[集名]"},
		{"集盒/[画师]_[123456789]_[集名]_[02].png", "[画师]_[123456789]_[集名]"},
		{"他盒/[画师乙]_[987654321]_[他集]_[01].png", "[画师乙]_[987654321]_[他集]"},
	} {
		file := snapshotFileOf(sess.Snapshot, c.relPath)
		if file == nil {
			t.Fatalf("快照内无文件 %s", c.relPath)
		}
		found := false
		for _, d := range file.Final {
			if MetadataDimension(d.Dimension) == DimCollection {
				found = true
				if d.Value != c.collection {
					t.Fatalf("%s 集名应=%q（群键），实际 %q", c.relPath, c.collection, d.Value)
				}
			}
		}
		if !found {
			t.Fatalf("%s 应有集名声明（终选: %+v）", c.relPath, file.Final)
		}
	}
}

// TestWorkSetSeqKeySingleCharKeyAdjust 单字符键直连序号集的序列指派（live
// 发现 1-fix 回归锚）：识别侧元素对齐建群（键=m、形态=直连序号）不受影响；
// v4 语法换代后规则侧对无分隔直连序号（键段与序号段相邻即模板相邻占位词）
// 如实拒绝——词法禁相邻占位词（切分歧义），调整面保存报可读错误，宁缺勿错。
func TestWorkSetSeqKeySingleCharKeyAdjust(t *testing.T) {
	root := t.TempDir()
	m, _ := newSeqRecognitionFixture(t, root)
	writeTestFile(t, root+"/单键集/m1.jpg", "a")
	writeTestFile(t, root+"/单键集/m2.jpg", "b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	group := snapshotGroupOf(sess.Snapshot, "单键集/m1.jpg")
	if group == nil || group.Key != "m" || group.FormName != "直连序号" {
		t.Fatalf("识别侧应建单字符键直连序号群: %+v", group)
	}
	target := confirmGroupCandidate(t, m, root, "单键集", "m")

	if _, err := m.ApplyWorkSetRuleAdjust(root, target, setRuleAdjustInput{
		CollectionSegRefs: []string{setClueSeqRef},
		SeqSegRef:         "setSeg2",
		MemberOnly:        true,
	}); err == nil || !strings.Contains(err.Error(), "相邻占位词") {
		t.Fatalf("无分隔直连序号应拒绝模板化（v4 词法禁相邻占位词，识别侧建群不受影响、规则侧拒绝歧义形态）: %v", err)
	}
}

// LoadProfileOrFail 测试便捷：载入规则档案（载入异常直接失败）。
func LoadProfileOrFail(t *testing.T, m *SessionManager) *RuleProfile {
	t.Helper()
	profile, errs := LoadProfile(m.store)
	for _, err := range errs {
		t.Fatalf("载入规则档案异常: %v", err)
	}
	return profile
}
