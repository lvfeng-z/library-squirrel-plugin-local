// 作用域附件（refScope）的求值两段化锚定测试：
// needed 收集自「全部启用规则条件行递归扫描 refScope 附件」；供给方命中层集
//（dirHitDepths）
// 经 ParseContext.RefScopeDepths 透传，消费规则目录行逐层槽位按层距阶梯判据
// 核对（裁决⑥）；供给方捕获快照以限定键注入消费规则私有作用域（发现6：
// 限定键为唯一通道）。锚定矩阵：层距阶梯（紧邻/任意/N 三档正反例+其身）、
// 供给方未命中（停用/悬空/被跳过/above-form）行不中、限定键注入可被提取
// （refCapture）与 when 引用、命中单点登记与追加序稳定（风险2/5）、三通道
// 一致性（批量识别/增量识别/执行期单文件重算）。
package main

import (
	"os"
	"reflect"
	"testing"
)

// —— 测试构造件 ——

// refScopeAttach 构造一条作用域附件（provider=供给方规则 ID；gap=层距：
// 1=紧邻、0=任意深度、N≥2 显式）。
func refScopeAttach(provider string, gap int) CondxAttach {
	return CondxAttach{Kind: CondxAttachRefScope, Value: provider, MinCount: gap}
}

// refDirRowRule 构造一条 v4 目录行规则（模板匹配任一层祖先目录名），可携带
// 附加条件与提取动作——供给方/消费规则的通用构造件。
func refDirRowRule(t *testing.T, id, tpl string, attach []CondxAttach, actions []ExtractionAction) RuleEntry {
	t.Helper()
	cond := &Condx{Target: CondxTargetDir, Format: condxTpl(tpl)}
	if len(attach) > 0 {
		cond.Attach = attach
	}
	rows := []CondxRow{{Condition: cond}}
	return refRowsRule(t, id, rows, actions)
}

// refFileRowRule 构造一条 v4 文件行规则（模板对文件名全锚定匹配），可携
// 带值校验（构造件：rule_condition_optional_absent_test.go /
// rule_refs_capture_test.go 沿用）。
func refFileRowRule(t *testing.T, id, tpl string, badges []CondxBadge, actions []ExtractionAction) RuleEntry {
	t.Helper()
	rows := []CondxRow{{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl(tpl), Badges: badges}}}
	return refRowsRule(t, id, rows, actions)
}

// refRowsRule 构造一条 v4 行条件规则并编译模式树。
func refRowsRule(t *testing.T, id string, rows []CondxRow, actions []ExtractionAction) RuleEntry {
	t.Helper()
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译条件行失败（%s）: %v", id, err)
	}
	return RuleEntry{
		ID: id, Description: "测试规则 " + id, Source: RuleSourceManual, Enabled: true,
		Conditions: rows, Pattern: pattern, Extractions: actions,
	}
}

// runRefEval 对单文件输入执行提取段（含门段），返回决策。
func runRefEval(t *testing.T, rules []RuleEntry, in *MatchInput, lib *LibraryIndex) *FileDecision {
	t.Helper()
	decision := &FileDecision{RelPath: in.RelPath, FileName: in.FileName}
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	runExtractionStage(decision, rules, in, lib, ctx)
	return decision
}

// assertRefMatchedIDs 断言命中规则 ID 全序列（含顺序；等长比较兼锚定无重复）。
func assertRefMatchedIDs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("命中规则 ID 序列不符: 得到 %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("命中规则 ID 序列不符: 得到 %v，期望 %v", got, want)
		}
	}
}

// countRefMatched 命中列统计口径：某规则 ID 在 MatchedRuleIDs 中的出现次数。
func countRefMatched(ids []string, id string) int {
	count := 0
	for _, v := range ids {
		if v == id {
			count++
		}
	}
	return count
}

// —— 层距阶梯（裁决⑥：紧邻/任意/N 三档正反例，其身与 above-form 天然不成立）——

// TestRefGateScopeLadder 层距阶梯判据矩阵：供给方（Artist[{作者}] 目录行）
// 命中层集固定后，消费行（Set{集名}）候选层按层距核对——紧邻(1)：恰下一级
// 命中、隔层不中；任意(0)：之下任一层命中；N=2：恰隔一级命中、紧邻不中；
// 其身（候选层=供给方命中层）在任意档位下均不可成立。
func TestRefGateScopeLadder(t *testing.T) {
	setAction := []ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1}}
	chain2 := &MatchInput{ // Artist 壳 → 集名目录（紧邻）
		FileName: "x.jpg", DirNames: []string{"Artist[甲]", "SetA"},
		RelPath: "Artist[甲]/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}
	chain3 := &MatchInput{ // Artist 壳 → 中隔目录 → 集名目录（隔一级）
		FileName: "x.jpg", DirNames: []string{"Artist[甲]", "Mid", "SetA"},
		RelPath: "Artist[甲]/Mid/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}
	shellOnly := &MatchInput{ // 文件直属 Artist 壳（候选层=供给方命中层——其身）
		FileName: "x.jpg", DirNames: []string{"Artist[甲]"},
		RelPath: "Artist[甲]/x.jpg", Siblings: []string{"x.jpg"},
	}

	t.Run("紧邻正例", func(t *testing.T) {
		provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
		consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 1)}, setAction)
		decision := runRefEval(t, []RuleEntry{provider, consumer}, chain2, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "consumer")
		if v, ok := declOf(decision, "consumer", DimCollection); !ok || v != "A" {
			t.Fatalf("紧邻集名应为 A: %q %v", v, ok)
		}
	})
	t.Run("紧邻反例·隔层不中", func(t *testing.T) {
		provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
		consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 1)}, nil)
		decision := runRefEval(t, []RuleEntry{provider, consumer}, chain3, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b")
	})
	t.Run("任意深度", func(t *testing.T) {
		provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
		consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 0)}, setAction)
		decision := runRefEval(t, []RuleEntry{provider, consumer}, chain3, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "consumer")
		if v, ok := declOf(decision, "consumer", DimCollection); !ok || v != "A" {
			t.Fatalf("任意深度集名应为 A: %q %v", v, ok)
		}
		// 其身在任意档下同样不可成立：直属壳文件的候选层=供给方命中层。
		decision = runRefEval(t, []RuleEntry{provider, consumer}, shellOnly, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b")
	})
	t.Run("显式层距二", func(t *testing.T) {
		provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
		consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 2)}, setAction)
		decision := runRefEval(t, []RuleEntry{provider, consumer}, chain3, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "consumer")
		if v, ok := declOf(decision, "consumer", DimCollection); !ok || v != "A" {
			t.Fatalf("隔一级集名应为 A: %q %v", v, ok)
		}
		// 紧邻链在层距二下不中（c−D=1≠2）。
		decision = runRefEval(t, []RuleEntry{provider, consumer}, chain2, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b")
	})
	t.Run("供给方多命中层取可行层", func(t *testing.T) {
		// 供给方两行合取（Or 行短路只记首个命中行，And 行各自记层）：Artist 壳
		// 与 Mid 层都命中——层距一在三层链上经 Mid 层（D=1）成立。
		provider := refRowsRule(t, "member-b", []CondxRow{
			{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]")}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Mid")}},
		}, nil)
		consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 1)}, nil)
		decision := runRefEval(t, []RuleEntry{provider, consumer}, chain3, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "consumer")
	})
}

// —— 供给方未命中矩阵（边界：严格语义，不降级）——

// TestRefGateScopeProviderMiss 供给方未命中（停用/悬空/被跳过〔模式缺失〕/
// above-form〔候选层在供给方命中层之上〕）→ 附件谓词恒不满足 → 消费行不中
//（消费规则零产出、不进 MatchedRuleIDs）。
func TestRefGateScopeProviderMiss(t *testing.T) {
	in := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"Artist[甲]", "SetA"},
		RelPath: "Artist[甲]/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}
	consumerOf := func(providerID string) RuleEntry {
		return refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach(providerID, 1)}, nil)
	}

	t.Run("供给方停用", func(t *testing.T) {
		provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
		provider.Enabled = false
		decision := runRefEval(t, []RuleEntry{provider, consumerOf("member-b")}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs)
	})
	t.Run("供给方悬空", func(t *testing.T) {
		decision := runRefEval(t, []RuleEntry{consumerOf("ghost")}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs)
	})
	t.Run("供给方被跳过", func(t *testing.T) {
		skipped := RuleEntry{ID: "member-b", Description: "无模式供给方", Source: RuleSourceManual, Enabled: true}
		decision := runRefEval(t, []RuleEntry{skipped, consumerOf("member-b")}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs)
	})
	t.Run("消费候选层在供给方命中层之上", func(t *testing.T) {
		// 供给方命中深层（Mid 的子层）；消费行候选层只有更浅层——above-form
		// 在阶梯判据下天然不可成立（层距 N 要求 c−D=N>0、任意深度要求 c>D）。
		provider := refDirRowRule(t, "member-b", "Set{集名}", nil, nil)
		consumer := refDirRowRule(t, "consumer", "{集名}", []CondxAttach{refScopeAttach("member-b", 0)}, nil)
		in2 := &MatchInput{
			FileName: "y.jpg", DirNames: []string{"SetA"},
			RelPath: "SetA/y.jpg", Siblings: []string{"y.jpg"},
		}
		decision := runRefEval(t, []RuleEntry{provider, consumer}, in2, nil)
		// 供给方在单层链命中第 0 层；消费候选层=同层（其身）→ 附件不满足。
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b")
	})
}

// —— 限定键注入：提取（refCapture）与 when 限定名双通道 ——

// TestRefGateScopeCaptureInjection 供给方捕获经限定键注入消费规则私有作用域：
// refCapture 提取按「供给方ID.rN.组名」取值产出（#canon 规范名回退序对齐
// FromCapture）；when 条件的限定名操作数（conditionOperandValue）同源取值；
// 普通自有捕获通道读不到供给方捕获（隔离，发现6）。
func TestRefGateScopeCaptureInjection(t *testing.T) {
	provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
	providerLib := refRowsRule(t, "member-c", []CondxRow{{Condition: &Condx{
		Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true}},
	}}}, nil)
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
	in := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"Artist[jake jaccobus]", "SetA"},
		RelPath: "Artist[jake jaccobus]/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}

	t.Run("refCapture提取与when限定名", func(t *testing.T) {
		consumer := refDirRowRule(t, "consumer", "Set{集名}",
			[]CondxAttach{refScopeAttach("member-b", 1)},
			[]ExtractionAction{
				// 无条件动作：供给方作者经限定键产出。
				{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-b", CaptureName: "author", Row: 1},
				// 条件动作：when 限定名比较供给方捕获值，成立才产出标签常量。
				{Dimension: DimTag, From: FromConstant, Constant: "壳内作者",
					When: &CondExpr{Kind: ExprEqual,
						Left:  &CondExpr{Kind: ExprCapture, CaptureName: "member-b.r1.author"},
						Right: &CondExpr{Kind: ExprString, Str: "jake jaccobus"}}},
			})
		decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "consumer")
		if v, ok := declOf(decision, "consumer", DimAuthor); !ok || v != "jake jaccobus" {
			t.Fatalf("refCapture 应取供给方限定键值: %q %v", v, ok)
		}
		if v, ok := declOf(decision, "consumer", DimTag); !ok || v != "壳内作者" {
			t.Fatalf("when 限定名条件应取注入区值成立: %q %v", v, ok)
		}
		// when 不成立的对照：限定名比较他值 → 条件动作不产出。
		consumerReject := refDirRowRule(t, "consumer-r", "Set{集名}",
			[]CondxAttach{refScopeAttach("member-b", 1)},
			[]ExtractionAction{
				{Dimension: DimTag, From: FromConstant, Constant: "不该出现",
					When: &CondExpr{Kind: ExprEqual,
						Left:  &CondExpr{Kind: ExprCapture, CaptureName: "member-b.r1.author"},
						Right: &CondExpr{Kind: ExprString, Str: "别人"}}},
			})
		decision2 := runRefEval(t, []RuleEntry{provider, consumerReject}, in, nil)
		if _, ok := declOf(decision2, "consumer-r", DimTag); ok {
			t.Fatalf("when 不成立时条件动作不应产出")
		}
	})
	t.Run("canon规范名回退", func(t *testing.T) {
		// 供给方带在库名单小标：#canon 规范名变体一并快照，refCapture 的
		// #canon→原始键回退序产出规范名。
		consumer := refDirRowRule(t, "consumer-c", "Set{集名}",
			[]CondxAttach{refScopeAttach("member-c", 1)},
			[]ExtractionAction{{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-c", CaptureName: "author", Row: 1}})
		decision := runRefEval(t, []RuleEntry{providerLib, consumer}, in, lib)
		if v, ok := declOf(decision, "consumer-c", DimAuthor); !ok || v != "Jake_Jaccobus" {
			t.Fatalf("refCapture 应经 #canon 回退取规范名: %q %v", v, ok)
		}
	})
	t.Run("自有捕获隔离", func(t *testing.T) {
		// 消费规则的普通 capture 动作读不到供给方捕获（限定键为唯一通道）。
		consumer := refDirRowRule(t, "consumer-iso", "Set{集名}",
			[]CondxAttach{refScopeAttach("member-b", 1)},
			[]ExtractionAction{{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1}})
		decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "consumer-iso")
		if _, ok := declOf(decision, "consumer-iso", DimAuthor); ok {
			t.Fatalf("普通 capture 不应读到供给方捕获（发现6 隔离）")
		}
	})
}

// —— 命中单点登记与追加序稳定（风险2/5）——

// TestRefGateDedupAndOrder 供给方被多条消费规则引用时 MatchedRuleIDs 只计
// 一次（供给方自身独立段单点登记，门段双求值不重复计数）；消费规则先于
// 供给方排列时门段先行仍成立，追加序保持规则传入序。
func TestRefGateDedupAndOrder(t *testing.T) {
	provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
	consumer1 := refDirRowRule(t, "consumer-1", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 1)}, nil)
	consumer2 := refDirRowRule(t, "consumer-2", "{集名}", []CondxAttach{refScopeAttach("member-b", 0)}, nil)
	rules := []RuleEntry{consumer1, provider, consumer2}
	in := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"Artist[甲]", "SetA"},
		RelPath: "Artist[甲]/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}
	decision := runRefEval(t, rules, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "consumer-1", "member-b", "consumer-2")
	for _, id := range []string{"consumer-1", "member-b", "consumer-2"} {
		if got := countRefMatched(decision.MatchedRuleIDs, id); got != 1 {
			t.Fatalf("规则 %s 命中计数应为 1（门段双求值不重复计数），实际 %d", id, got)
		}
	}
	// 供给方未命中文件上（无 Artist 壳祖先）：全部消费行不中，仅无关供给方缺席。
	inMiss := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"Plain"},
		RelPath: "Plain/x.jpg", Siblings: []string{"x.jpg"},
	}
	decisionMiss := runRefEval(t, rules, inMiss, nil)
	assertRefMatchedIDs(t, decisionMiss.MatchedRuleIDs)
}

// —— 消费规则在场不改供给方与非消费路径（解耦本义）——

// TestRefGateNonComboZeroChange 消费规则在场不改变供给方与普通规则的命中与
// 产出（消费规则自身零提取——只命中计入清单）；解耦本义。
func TestRefGateNonComboZeroChange(t *testing.T) {
	provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1},
	})
	third := refFileRowRule(t, "plain-c", "{标题}_{标签}", nil, []ExtractionAction{
		{Dimension: DimTitle, From: FromCapture, CaptureName: "title", Row: 1},
	})
	consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 1)}, nil)
	declKey := func(d *FileDecision) []string {
		keys := make([]string, 0, len(d.Declarations))
		for _, decl := range d.Declarations {
			keys = append(keys, string(decl.RuleID)+"|"+string(decl.Dimension)+"="+decl.Value)
		}
		return keys
	}
	in := &MatchInput{
		FileName: "乙_科幻.jpg", DirNames: []string{"Artist[甲]", "SetA"},
		RelPath: "Artist[甲]/SetA/乙_科幻.jpg", Siblings: []string{"乙_科幻.jpg"},
	}

	baseline := runRefEval(t, []RuleEntry{provider, third}, in, nil)
	withConsumer := runRefEval(t, []RuleEntry{provider, third, consumer}, in, nil)

	assertRefMatchedIDs(t, baseline.MatchedRuleIDs, "member-b", "plain-c")
	assertRefMatchedIDs(t, withConsumer.MatchedRuleIDs, "member-b", "plain-c", "consumer")
	if len(baseline.Declarations) != len(withConsumer.Declarations) {
		t.Fatalf("消费规则在场不应改变既有声明集: %v vs %v", declKey(baseline), declKey(withConsumer))
	}
	for i := range baseline.Declarations {
		if !reflect.DeepEqual(baseline.Declarations[i], withConsumer.Declarations[i]) {
			t.Fatalf("声明 %d 因消费规则在场而变化: %+v vs %+v", i, baseline.Declarations[i], withConsumer.Declarations[i])
		}
	}
}

// —— 三通道一致性 ——

// TestRefGateThreeChannelConsistency 批量识别（MergedRecognition）、增量识别
//（MergedRecognitionWithTree：本批子集 + 全量树——增量轮的求值形态）与执行期
// 单文件重算（recomputeFileDecision）三通道对作用域附件的口径一致：供给方
// 命中文件上消费规则入 MatchedRuleIDs、供给方缺席文件上消费规则缺席，且三
// 通道命中序列全等。
func TestRefGateThreeChannelConsistency(t *testing.T) {
	root := t.TempDir()
	files := []struct {
		dir         string
		dirNames    []string
		consumerHit bool
	}{
		{"Artist[甲]/SetA", []string{"Artist[甲]", "SetA"}, true}, // 供给方命中（Artist 壳）+ 集名目录紧邻其下
		{"Plain", []string{"Plain"}, false},                      // 无 Artist 壳祖先 → 附件谓词不满足
	}
	scanFiles := make([]ScanFile, 0, len(files))
	for _, f := range files {
		dir := root + "/" + f.dir
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}
		full := dir + "/x.jpg"
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
		scanFiles = append(scanFiles, ScanFile{
			Path: full, FileName: "x.jpg", DirNames: f.dirNames, RelPath: f.dir + "/x.jpg",
		})
	}

	provider := refDirRowRule(t, "member-b", "Artist[{作者}]", nil, nil)
	consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach("member-b", 1)}, []ExtractionAction{
		{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1},
	})
	rules := []RuleEntry{provider, consumer}

	batch := MergedRecognition(scanFiles, rules, nil, nil)
	incremental := MergedRecognitionWithTree(scanFiles[:1], scanFiles, rules, nil, nil)
	h := &LocalImportWorkFetcher{}

	for i, f := range files {
		fp := &FilePluginData{FullPath: scanFiles[i].Path, RelPath: scanFiles[i].RelPath}
		recomputed, _ := h.recomputeFileDecision(fp, rules, nil)

		if got := refIDsContain(recomputed.MatchedRuleIDs, "consumer"); got != f.consumerHit {
			t.Fatalf("%s：重算通道作用域门控不符（期望消费命中=%v）: %v", f.dir, f.consumerHit, recomputed.MatchedRuleIDs)
		}
		if got := refIDsContain(batch[i].MatchedRuleIDs, "consumer"); got != f.consumerHit {
			t.Fatalf("%s：批量通道作用域门控不符（期望消费命中=%v）: %v", f.dir, f.consumerHit, batch[i].MatchedRuleIDs)
		}
		if !refIDsEqual(recomputed.MatchedRuleIDs, batch[i].MatchedRuleIDs) {
			t.Fatalf("%s：重算通道与批量通道命中序列不一致: %v vs %v", f.dir, recomputed.MatchedRuleIDs, batch[i].MatchedRuleIDs)
		}
	}
	if !refIDsEqual(incremental[0].MatchedRuleIDs, batch[0].MatchedRuleIDs) {
		t.Fatalf("增量通道与批量通道命中序列不一致: %v vs %v", incremental[0].MatchedRuleIDs, batch[0].MatchedRuleIDs)
	}
	// 全命中文件的三通道终局序列锚（供给方单点登记 + 消费规则殿后）。
	assertRefMatchedIDs(t, batch[0].MatchedRuleIDs, "member-b", "consumer")
}

// —— 捕获快照的限定键形态与注入区隔离（发现6 回归锚·作用域版）——

// TestRefGateCaptureSnapshotQualifiedKeys 门段快照以限定键「供给方ID.rN.组名」
// 落位（#canon 规范名变体一并快照）；注入只挂 RefCaptures 注入区、不并入消费
// 规则私有捕获（跨规则捕获仍隔离，限定键为唯一通道）；供给方命中层集
//（dirHitDepths）即层距阶梯判据的数据源（RefScopeDepths 透传面）。
func TestRefGateCaptureSnapshotQualifiedKeys(t *testing.T) {
	// 供给方：目录行 + 库名单折叠小标（命中后另绑 r1.author#canon 规范名）。
	provider := refRowsRule(t, "member-a", []CondxRow{{Condition: &Condx{
		Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true}},
	}}}, nil)
	consumer := refDirRowRule(t, "consumer", "{集名}", []CondxAttach{refScopeAttach("member-a", 1)}, nil)
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
	in := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"Artist[jake jaccobus]", "SetA"},
		RelPath: "Artist[jake jaccobus]/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}

	gate := runRefGateStage([]RuleEntry{provider, consumer}, in, lib, ctx)
	if !gate.hits["member-a"] {
		t.Fatalf("门段应判定供给方命中")
	}
	if !gate.dirHitDepths["member-a"][0] || len(gate.dirHitDepths["member-a"]) != 1 {
		t.Fatalf("供给方命中层集应恰含第 0 层: %+v", gate.dirHitDepths)
	}
	snap := gate.snapshot("member-a")
	byName := map[string]string{}
	for _, binding := range snap {
		byName[binding.Name] = binding.Value
	}
	// 限定键全量断言：行捕获（author）+ 库名单小标的 #canon 规范名变体，一并
	// 冠以供给方 ID 前缀快照。
	wantBindings := map[string]string{
		"member-a.r1.author":       "jake jaccobus",
		"member-a.r1.author#canon": "Jake_Jaccobus",
	}
	if len(snap) != len(wantBindings) {
		t.Fatalf("限定键快照条数不符: 得到 %v，期望键集 %v", byName, wantBindings)
	}
	for name, want := range wantBindings {
		if got, ok := byName[name]; !ok || got != want {
			t.Fatalf("限定键 %s 取值不符: 得到 %q（在场=%v），期望 %q", name, got, ok, want)
		}
	}

	// 注入区：消费规则作用域经注入可按限定键取值；私有捕获保持空（隔离）。
	ruleCtx := ctx.ruleScopedContext()
	injectRefSnapshots(ruleCtx, gate, refScopeProvidersOf(&consumer))
	if len(ruleCtx.Captures) != 0 {
		t.Fatalf("快照不得并入消费规则私有捕获（发现6 隔离），实际 %+v", ruleCtx.Captures)
	}
	if v, ok := ruleCtx.refCaptureValue("member-a", "r1.author#canon"); !ok || v != "Jake_Jaccobus" {
		t.Fatalf("#canon 限定键取值不符: %q %v", v, ok)
	}
	if v, ok := ruleCtx.refCaptureValue("member-a", "r1.author"); !ok || v != "jake jaccobus" {
		t.Fatalf("原始限定键取值不符: %q %v", v, ok)
	}
	if _, ok := ruleCtx.refCaptureValue("ghost", "r1.author"); ok {
		t.Fatalf("悬空供给方限定键不应取到值")
	}
	// 供给方命中层集透传（提取段同款）：ruleScopedContext 继承后阶梯判据可读。
	ctx.RefScopeDepths = gate.dirHitDepths
	scoped := ctx.ruleScopedContext()
	if !scoped.RefScopeDepths["member-a"][0] {
		t.Fatalf("ruleScopedContext 应继承供给方命中层集（风险6 核对点）")
	}
	if _, ok := scoped.RefScopeDepths["ghost"]; ok {
		t.Fatalf("未求值供给方不应有层集条目")
	}
	// 非消费规则作用域：零注入（限定键通道对非消费路径不可见）。
	plain := ctx.ruleScopedContext()
	if _, ok := plain.refCaptureValue("member-a", "r1.author"); ok || len(plain.RefCaptures) != 0 {
		t.Fatalf("未注入的作用域不应有注入区内容")
	}
}

// refIDsContain 判断命中 ID 序列是否含目标值。
func refIDsContain(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// refIDsEqual 判断两个 ID 序列逐元素相等（顺序敏感）。
func refIDsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// —— 多级链：依赖拓扑序求值与层集逐级传递 ——

// chainDepthRules 构造三级链规则（各层模板互异、每层挂紧邻层距附件）：
// 首层挂库根附件（根直下第 1 级）、二级挂首层、三级挂二级——首层供给方自身
// 携带库根附件，正是「供给方可携带作用域附件」的多级链形态。直接构造规则
// 对象（不经档案构造管线）。
func chainDepthRules(t *testing.T) (l1, l2, l3 RuleEntry) {
	t.Helper()
	capAction := []ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1}}
	l1 = refDirRowRule(t, "chain-l1", "L1[{集名}]", []CondxAttach{refScopeAttach(RefScopeLibraryRoot, 1)}, capAction)
	l2 = refDirRowRule(t, "chain-l2", "L2[{集名}]", []CondxAttach{refScopeAttach("chain-l1", 1)}, capAction)
	l3 = refDirRowRule(t, "chain-l3", "L3[{集名}]", []CondxAttach{refScopeAttach("chain-l2", 1)}, capAction)
	return l1, l2, l3
}

// chainDepthInput 三级链目录输入（文件直属第三级）。
func chainDepthInput() *MatchInput {
	return &MatchInput{
		FileName: "x.jpg", DirNames: []string{"L1[a]", "L2[b]", "L3[c]"},
		RelPath: "L1[a]/L2[b]/L3[c]/x.jpg", Siblings: []string{"x.jpg"},
	}
}

// TestRefGateChainDepthPropagation 三级链层集逐级传递：库根常量 {-1} → 首层
// 命中 D=0 → 二级候选层 c=1 → 三级候选层 c=2——门段按依赖拓扑序求值，先求值
// 供给方登记的命中层集经共享映射对后求值者可见；规则传入序倒置（消费方在前、
// 供给方殿后）不改变结论，各层取值锚定各自目录层。
func TestRefGateChainDepthPropagation(t *testing.T) {
	l1, l2, l3 := chainDepthRules(t)
	in := chainDepthInput()

	// 门段级锚：needed=被引用的供给方（l1、l2），层集恰为 {0}/{1}——库根
	// 常量 {-1} 使首层命中 D=0、二级紧邻判据 c−D=1 使其命中 D=1；三级
	//（chain-l3）不被引用、不进门段，其行在主循环求值（下方端到端锚）。
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	gate := runRefGateStage([]RuleEntry{l3, l2, l1}, in, nil, ctx)
	for id, wantDepth := range map[string]int{"chain-l1": 0, "chain-l2": 1} {
		if !gate.hits[id] {
			t.Fatalf("链供给方 %s 应在门段命中: %+v", id, gate.hits)
		}
		if !gate.dirHitDepths[id][wantDepth] || len(gate.dirHitDepths[id]) != 1 {
			t.Fatalf("链供给方 %s 命中层集应恰为 {%d}: %+v", id, wantDepth, gate.dirHitDepths)
		}
	}
	if _, evaluated := gate.hits["chain-l3"]; evaluated {
		t.Fatalf("未被引用的三级规则不应进门段求值: %+v", gate.hits)
	}

	// 端到端锚：三级全命中（主循环登记序=规则传入序），取值逐层落位。
	decision := runRefEval(t, []RuleEntry{l3, l2, l1}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "chain-l3", "chain-l2", "chain-l1")
	for id, want := range map[string]string{"chain-l1": "a", "chain-l2": "b", "chain-l3": "c"} {
		if v, ok := declOf(decision, id, DimCollection); !ok || v != want {
			t.Fatalf("链成员 %s 集名应取自本层目录（%q）: %q %v", id, want, v, ok)
		}
	}

	// 链形断链：二级目录缺席（文件直属首层）→ 二/三级不中，首层照常命中。
	inDirect := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"L1[a]"},
		RelPath: "L1[a]/x.jpg", Siblings: []string{"x.jpg"},
	}
	decisionDirect := runRefEval(t, []RuleEntry{l3, l2, l1}, inDirect, nil)
	assertRefMatchedIDs(t, decisionDirect.MatchedRuleIDs, "chain-l1")
}

// TestRefGateChainUpstreamDisabled 链上级停用→下级行不中（严格语义传递）：
// 停用首层（库根附件供给方）→ 二、三级全不中；停用中层 → 三级不中、首层
// 照常命中（库根常量可见与中层无关）。
func TestRefGateChainUpstreamDisabled(t *testing.T) {
	l1, l2, l3 := chainDepthRules(t)
	in := chainDepthInput()

	t.Run("停用首层全链不中", func(t *testing.T) {
		l1.Enabled = false
		defer func() { l1.Enabled = true }()
		decision := runRefEval(t, []RuleEntry{l3, l2, l1}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs)
	})
	t.Run("停用中层三级不中", func(t *testing.T) {
		l2.Enabled = false
		defer func() { l2.Enabled = true }()
		decision := runRefEval(t, []RuleEntry{l3, l2, l1}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "chain-l1")
	})
}

// TestRefGateChainCycleDefensiveMiss 环成员防御性未命中：互引成环（构造校验
// 拒绝的形态，此处直接构造规则对象绕过校验管线下探运行期防御）与自引用
//（深度 1 的环）成员都不进门段求值＝未命中，端到端行不中。
func TestRefGateChainCycleDefensiveMiss(t *testing.T) {
	t.Run("互引环不求值", func(t *testing.T) {
		cycA := refDirRowRule(t, "cyc-a", "A[{集名}]", []CondxAttach{refScopeAttach("cyc-b", 1)}, nil)
		cycB := refDirRowRule(t, "cyc-b", "B[{集名}]", []CondxAttach{refScopeAttach("cyc-a", 1)}, nil)
		in := &MatchInput{
			FileName: "x.jpg", DirNames: []string{"A[m]", "B[n]"},
			RelPath: "A[m]/B[n]/x.jpg", Siblings: []string{"x.jpg"},
		}
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		gate := runRefGateStage([]RuleEntry{cycA, cycB}, in, nil, ctx)
		if len(gate.hits) != 0 || len(gate.dirHitDepths) != 0 {
			t.Fatalf("环成员不应进求值（hits/层集全空）: %+v", gate)
		}
		decision := runRefEval(t, []RuleEntry{cycA, cycB}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs)
	})
	t.Run("自引用不入队", func(t *testing.T) {
		self := refDirRowRule(t, "cyc-self", "A[{集名}]", []CondxAttach{refScopeAttach("cyc-self", 1)}, nil)
		in := &MatchInput{
			FileName: "x.jpg", DirNames: []string{"A[m]"},
			RelPath: "A[m]/x.jpg", Siblings: []string{"x.jpg"},
		}
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		gate := runRefGateStage([]RuleEntry{self}, in, nil, ctx)
		if len(gate.hits) != 0 || len(gate.dirHitDepths) != 0 {
			t.Fatalf("自引用成员不应进求值: %+v", gate)
		}
	})
}

// TestRefGateChainThreeChannelConsistency 三级链的三通道一致性（批量识别 /
// 增量识别 / 执行期单文件重算）：全链文件三级全中、中链文件（缺第三级目录）
// 两级中、无链文件全空，三通道命中序列逐文件全等。
func TestRefGateChainThreeChannelConsistency(t *testing.T) {
	root := t.TempDir()
	files := []struct {
		dir      string
		dirNames []string
		want     []string
	}{
		{"L1[a]/L2[b]/L3[c]", []string{"L1[a]", "L2[b]", "L3[c]"}, []string{"chain-l1", "chain-l2", "chain-l3"}},
		{"L1[a]/L2[b]", []string{"L1[a]", "L2[b]"}, []string{"chain-l1", "chain-l2"}},
		{"Plain", []string{"Plain"}, nil},
	}
	scanFiles := make([]ScanFile, 0, len(files))
	for _, f := range files {
		dir := root + "/" + f.dir
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}
		full := dir + "/x.jpg"
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
		scanFiles = append(scanFiles, ScanFile{
			Path: full, FileName: "x.jpg", DirNames: f.dirNames, RelPath: f.dir + "/x.jpg",
		})
	}

	l1, l2, l3 := chainDepthRules(t)
	rules := []RuleEntry{l1, l2, l3}

	batch := MergedRecognition(scanFiles, rules, nil, nil)
	incremental := MergedRecognitionWithTree(scanFiles[:1], scanFiles, rules, nil, nil)
	h := &LocalImportWorkFetcher{}

	for i, f := range files {
		fp := &FilePluginData{FullPath: scanFiles[i].Path, RelPath: scanFiles[i].RelPath}
		recomputed, _ := h.recomputeFileDecision(fp, rules, nil)
		assertRefMatchedIDs(t, batch[i].MatchedRuleIDs, f.want...)
		if !refIDsEqual(recomputed.MatchedRuleIDs, batch[i].MatchedRuleIDs) {
			t.Fatalf("%s：重算通道与批量通道命中序列不一致: %v vs %v", f.dir, recomputed.MatchedRuleIDs, batch[i].MatchedRuleIDs)
		}
	}
	if !refIDsEqual(incremental[0].MatchedRuleIDs, batch[0].MatchedRuleIDs) {
		t.Fatalf("增量通道与批量通道命中序列不一致: %v vs %v", incremental[0].MatchedRuleIDs, batch[0].MatchedRuleIDs)
	}
}
