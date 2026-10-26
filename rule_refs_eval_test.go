// 规则组合引用的求值两段化（规则组合引用方案阶段2）：门段先行——被引用成员
// 全体先求值、命中单点登记（MatchedRuleIDs 不重复计数、追加序保持规则序）、
// 成员捕获快照以限定键（成员ID.rN.组名，含 #canon 变体）注入仅组合规则可见的
// 注入区。锚定测试矩阵第 1/3/5/7 项：门控矩阵（全命中→组合命中；任一未命中/
// 停用/被跳过/悬空→组合零产出不进 MatchedRuleIDs）、非组合路径零变化与捕获
// 隔离（发现6：限定键为唯一通道）、三通道一致性（批量识别/增量识别/执行期
// 单文件重算）与门段双求值去重（风险5）。
package main

import (
	"os"
	"testing"
)

// —— 测试构造件 ——

// refFileRowRule 构造一条 v4 文件行成员规则（模板对文件名全锚定匹配）。
func refFileRowRule(t *testing.T, id, tpl string, badges []CondxBadge, actions []ExtractionAction) RuleEntry {
	t.Helper()
	rows := []CondxRow{{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl(tpl), Badges: badges}}}
	return refRowsRule(t, id, rows, nil, actions)
}

// refDirRowRule 构造一条 v4 目录行成员规则（模板匹配任一层祖先目录名）。
func refDirRowRule(t *testing.T, id, tpl string) RuleEntry {
	t.Helper()
	rows := []CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl(tpl)}}}
	return refRowsRule(t, id, rows, nil, nil)
}

// refRowsRule 构造一条 v4 行条件规则并编译模式树；refs 非空即组合规则。
func refRowsRule(t *testing.T, id string, rows []CondxRow, refs []string, actions []ExtractionAction) RuleEntry {
	t.Helper()
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译条件行失败（%s）: %v", id, err)
	}
	return RuleEntry{
		ID: id, Description: "测试规则 " + id, Source: RuleSourceManual, Enabled: true,
		Refs: refs, Conditions: rows, Pattern: pattern, Extractions: actions,
	}
}

// refComboRule 构造一条组合规则：ownRows 为 nil 时模式为空合取（恒真——门
// 单独决定命中，锚定「组合可无自身行」形态）；ownRows 非空时按决策1 并存
// 自身行（照旧私有语义）。
func refComboRule(t *testing.T, id string, refs []string, ownRows []CondxRow, actions []ExtractionAction) RuleEntry {
	t.Helper()
	if ownRows == nil {
		return RuleEntry{
			ID: id, Description: "测试组合 " + id, Source: RuleSourceManual, Enabled: true,
			Refs: refs, Pattern: And{}, Extractions: actions,
		}
	}
	return refRowsRule(t, id, ownRows, refs, actions)
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

// refHitIn 门全命中形态：文件名命中成员A（{作者}_{标题}），祖先目录命中成员B
//（Artist[{作者}]）。
func refHitIn() *MatchInput {
	return &MatchInput{
		FileName: "甲_乙.jpg", DirNames: []string{"Artist[甲]"},
		RelPath: "Artist[甲]/甲_乙.jpg", Siblings: []string{"甲_乙.jpg"},
	}
}

// refMissIn 成员B 未命中形态：无 Artist 壳祖先目录。
func refMissIn() *MatchInput {
	return &MatchInput{
		FileName: "甲_乙.jpg", DirNames: []string{"Plain"},
		RelPath: "Plain/甲_乙.jpg", Siblings: []string{"甲_乙.jpg"},
	}
}

// refEvalMembers 标准成员对：A=文件行（作者声明产出），B=目录行（纯命中无提取）。
func refEvalMembers(t *testing.T) (RuleEntry, RuleEntry) {
	t.Helper()
	memberA := refFileRowRule(t, "member-a", "{作者}_{标题}", nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1},
	})
	memberB := refDirRowRule(t, "member-b", "Artist[{作者}]")
	return memberA, memberB
}

// —— 测试矩阵第 1 项：门控矩阵 ——

// TestRefGateMatrix 门控矩阵：全部成员命中→组合 ID 进 MatchedRuleIDs；任一
// 成员未命中/停用/被跳过（模式缺失）/悬空（不在规则集内）→组合零产出、不进
// MatchedRuleIDs（严格语义，不降级）；门过但组合自身行未命中→同样不计入。
func TestRefGateMatrix(t *testing.T) {
	t.Run("全命中组合计入", func(t *testing.T) {
		memberA, memberB := refEvalMembers(t)
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, nil)
		decision := runRefEval(t, []RuleEntry{memberA, memberB, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a", "member-b", "combo-1")
		// 成员独立产出照旧（成员A 的作者声明不受组合在场影响）。
		if v, ok := declOf(decision, "member-a", DimAuthor); !ok || v != "甲" {
			t.Fatalf("成员A 独立作者声明不符: %q %v", v, ok)
		}
	})

	t.Run("任一成员未命中组合零产出", func(t *testing.T) {
		memberA, memberB := refEvalMembers(t)
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, nil)
		decision := runRefEval(t, []RuleEntry{memberA, memberB, combo}, refMissIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a")
	})

	t.Run("成员停用组合零产出", func(t *testing.T) {
		memberA, memberB := refEvalMembers(t)
		memberB.Enabled = false
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, nil)
		decision := runRefEval(t, []RuleEntry{memberA, memberB, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a")
	})

	t.Run("成员被跳过组合零产出", func(t *testing.T) {
		memberA, _ := refEvalMembers(t)
		// 模式缺失的成员＝主循环被跳过，门段按未命中。
		skipped := RuleEntry{ID: "member-b", Description: "无模式成员", Source: RuleSourceManual, Enabled: true}
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, nil)
		decision := runRefEval(t, []RuleEntry{memberA, skipped, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a")
	})

	t.Run("悬空成员组合零产出", func(t *testing.T) {
		memberA, _ := refEvalMembers(t)
		combo := refComboRule(t, "combo-1", []string{"member-a", "ghost"}, nil, nil)
		decision := runRefEval(t, []RuleEntry{memberA, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a")
	})

	t.Run("门过但自身行未命中不计入", func(t *testing.T) {
		memberA, memberB := refEvalMembers(t)
		// 自身行（Vol{序号} 数字段）不命中测试文件名——门过但自身模式未中。
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"},
			[]CondxRow{{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("Vol{序号}")}}}, nil)
		decision := runRefEval(t, []RuleEntry{memberA, memberB, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a", "member-b")
	})

	t.Run("门过refCapture动作按限定键取值产出", func(t *testing.T) {
		memberA, memberB := refEvalMembers(t)
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1},
		})
		decision := runRefEval(t, []RuleEntry{memberA, memberB, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a", "member-b", "combo-1")
		// 阶段3 起 Apply 的 refCapture 分支取限定键值产出（完整取值矩阵——
		// #canon 回退/行限定/Row≤0 兜底/剥壳——见 rule_refs_capture_test.go）。
		if v, ok := declOf(decision, "combo-1", DimAuthor); !ok || v != "甲" {
			t.Fatalf("组合规则的 refCapture 动作应取成员限定键值产出: %q %v", v, ok)
		}
	})
}

// —— 测试矩阵第 7 项：门段双求值去重与追加序稳定（风险5/风险2）——

// TestRefGateDedupAndOrder 成员被（多条）组合引用时 MatchedRuleIDs 只计一次；
// 组合先于成员排列时门段先行仍成立，追加序保持规则传入序（命中列统计与任务
// 树子行序消费者的顺序面不变）。
func TestRefGateDedupAndOrder(t *testing.T) {
	memberA, memberB := refEvalMembers(t)
	combo1 := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, nil)
	combo2 := refComboRule(t, "combo-2", []string{"member-a"}, nil, nil)
	rules := []RuleEntry{combo1, memberA, memberB, combo2}
	decision := runRefEval(t, rules, refHitIn(), nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "combo-1", "member-a", "member-b", "combo-2")
	for _, id := range []string{"combo-1", "member-a", "member-b", "combo-2"} {
		if got := countRefMatched(decision.MatchedRuleIDs, id); got != 1 {
			t.Fatalf("规则 %s 命中计数应为 1（门段双求值不重复计数），实际 %d", id, got)
		}
	}
	// 门失败侧同样不重复，且门控按各组合的 refs 独立分叉：memberB 未命中
	// 文件上 combo-1（引用 A+B）门失败零产出，combo-2（只引用 member-a）
	// 门过照常计入；被引的 memberA 仍只计一次。
	decisionMiss := runRefEval(t, rules, refMissIn(), nil)
	assertRefMatchedIDs(t, decisionMiss.MatchedRuleIDs, "member-a", "combo-2")
}

// —— 捕获快照的限定键形态与注入区隔离（发现6 回归锚）——

// TestRefCaptureSnapshotQualifiedKeys 门段快照以限定键「成员ID.rN.组名」落位
//（#canon 规范名变体一并快照）；注入只挂 RefCaptures 注入区、不并入组合规则
// 私有捕获（跨规则捕获仍隔离，限定键为唯一通道）；取值为取最后语义。
func TestRefCaptureSnapshotQualifiedKeys(t *testing.T) {
	// 成员A：文件行 + 库名单折叠小标（命中后另绑 r1.author#canon 规范名）。
	memberA := refFileRowRule(t, "member-a", "{作者}_{标题}", []CondxBadge{
		{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true},
	}, nil)
	combo := refComboRule(t, "combo-1", []string{"member-a"}, nil, nil)
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
	in := &MatchInput{FileName: "jake__jaccobus_乙.jpg", RelPath: "jake__jaccobus_乙.jpg"}
	ctx := &ParseContext{Depth: 0}

	gate := runRefGateStage([]RuleEntry{memberA, combo}, in, lib, ctx)
	if !gate.hits["member-a"] {
		t.Fatalf("门段应判定成员命中")
	}
	snap := gate.snapshot("member-a")
	byName := map[string]string{}
	for _, binding := range snap {
		byName[binding.Name] = binding.Value
	}
	// 限定键全量断言：行捕获（author/title——文件主体对去扩展名基名匹配，
	// title 为 "乙"）+ 库名单小标的 #canon 规范名变体，一并冠以成员 ID 前缀快照。
	wantBindings := map[string]string{
		"member-a.r1.author":       "jake__jaccobus",
		"member-a.r1.title":        "乙",
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

	// 注入区：组合规则作用域经注入可按限定键取值；私有捕获保持空（隔离）。
	ruleCtx := ctx.ruleScopedContext()
	injectRefSnapshots(ruleCtx, gate, []string{"member-a"})
	if len(ruleCtx.Captures) != 0 {
		t.Fatalf("快照不得并入组合规则私有捕获（发现6 隔离），实际 %+v", ruleCtx.Captures)
	}
	if v, ok := ruleCtx.refCaptureValue("member-a", "r1.author#canon"); !ok || v != "Jake_Jaccobus" {
		t.Fatalf("#canon 限定键取值不符: %q %v", v, ok)
	}
	if v, ok := ruleCtx.refCaptureValue("member-a", "r1.author"); !ok || v != "jake__jaccobus" {
		t.Fatalf("原始限定键取值不符: %q %v", v, ok)
	}
	if _, ok := ruleCtx.refCaptureValue("member-a", "r2.author"); ok {
		t.Fatalf("未绑定行的限定键不应取到值")
	}
	if _, ok := ruleCtx.refCaptureValue("ghost", "r1.author"); ok {
		t.Fatalf("悬空成员限定键不应取到值")
	}
	// 非组合规则作用域：零注入（限定键通道对非组合路径不可见）。
	plain := ctx.ruleScopedContext()
	if _, ok := plain.refCaptureValue("member-a", "r1.author"); ok || len(plain.RefCaptures) != 0 {
		t.Fatalf("非组合规则作用域不应有注入区内容")
	}
}

// —— 测试矩阵第 3 项：成员独立产出与非组合路径零变化 ——

// TestRefGateNonComboZeroChange 组合规则在场不改变成员与非组合规则的命中与
// 产出（解耦本义）；组合规则自身行照旧私有语义——无自身行时其普通 capture
// 取值读不到成员捕获（隔离），有自身行时读自身绑定。
func TestRefGateNonComboZeroChange(t *testing.T) {
	memberA, memberB := refEvalMembers(t)
	third := refFileRowRule(t, "plain-c", "{标题}_{标签}", nil, []ExtractionAction{
		{Dimension: DimTitle, From: FromCapture, CaptureName: "title", Row: 1},
	})
	declKey := func(d *FileDecision) []string {
		keys := make([]string, 0, len(d.Declarations))
		for _, decl := range d.Declarations {
			keys = append(keys, string(decl.RuleID)+"|"+string(decl.Dimension)+"="+decl.Value)
		}
		return keys
	}

	baseline := runRefEval(t, []RuleEntry{memberA, memberB, third}, refHitIn(), nil)
	withCombo := runRefEval(t, []RuleEntry{memberA, memberB, third,
		refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, nil)}, refHitIn(), nil)

	assertRefMatchedIDs(t, baseline.MatchedRuleIDs, "member-a", "member-b", "plain-c")
	assertRefMatchedIDs(t, withCombo.MatchedRuleIDs, "member-a", "member-b", "plain-c", "combo-1")
	if len(baseline.Declarations) != len(withCombo.Declarations) {
		t.Fatalf("组合在场不应改变既有声明集: %v vs %v", declKey(baseline), declKey(withCombo))
	}
	for i := range baseline.Declarations {
		if baseline.Declarations[i] != withCombo.Declarations[i] {
			t.Fatalf("声明 %d 因组合在场而变化: %+v vs %+v", i, baseline.Declarations[i], withCombo.Declarations[i])
		}
	}

	// 隔离锚：无自身行的组合规则其普通 capture 取值读不到成员捕获（限定键为
	// 唯一通道）——命中计数在、声明零产出。
	isolated := refComboRule(t, "combo-iso", []string{"member-a"}, nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1},
	})
	decision := runRefEval(t, []RuleEntry{memberA, isolated}, refHitIn(), nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a", "combo-iso")
	if _, ok := declOf(decision, "combo-iso", DimAuthor); ok {
		t.Fatalf("组合规则的普通 capture 取值不应读到成员捕获（发现6 隔离）")
	}

	// 自身行锚（决策1）：组合携带自身行时普通 capture 读自身绑定（非成员值）。
	selfRow := refComboRule(t, "combo-self", []string{"member-b"},
		[]CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Vol[{标签}]")}}}, []ExtractionAction{
			{Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1},
		})
	in := &MatchInput{
		FileName: "甲_乙.jpg", DirNames: []string{"Vol[科幻]", "Artist[甲]"},
		RelPath:  "Vol[科幻]/Artist[甲]/甲_乙.jpg", Siblings: []string{"甲_乙.jpg"},
	}
	decision = runRefEval(t, []RuleEntry{memberB, selfRow}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "combo-self")
	if v, ok := declOf(decision, "combo-self", DimTag); !ok || v != "科幻" {
		t.Fatalf("组合自身行 capture 应读自身绑定: %q %v", v, ok)
	}
}

// —— 测试矩阵第 5 项：三通道一致性 ——

// TestRefGateThreeChannelConsistency 批量识别（MergedRecognition）、增量识别
//（MergedRecognitionWithTree：本批子集 + 全量树——增量轮的求值形态）与执行期
// 单文件重算（recomputeFileDecision）三通道对组合门控的口径一致：成员全命中
// 文件上组合入 MatchedRuleIDs、成员缺失文件上组合缺席，且三通道命中序列全等。
// 增量识别通道的调用链收敛已核实：runFullRecognition（recognition.go 增量轮）
// 经 MergedRecognitionWithTree → mergedFileConclusionWithStructural，与批量入口
// 和 recomputeFileDecision（mergedFileConclusionOf）同一实现（风险3）。
func TestRefGateThreeChannelConsistency(t *testing.T) {
	root := t.TempDir()
	files := []struct {
		dir, name string
		comboHit  bool
	}{
		{"Artist[甲]", "甲_乙.jpg", true},  // 成员A（文件行）+成员B（Artist 壳目录）全命中
		{"Plain", "甲_乙.jpg", false},      // 成员B 无 Artist 壳祖先目录→门失败
	}
	scanFiles := make([]ScanFile, 0, len(files))
	for _, f := range files {
		dir := root + "/" + f.dir
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("建目录失败: %v", err)
		}
		full := dir + "/" + f.name
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
		scanFiles = append(scanFiles, ScanFile{
			Path: full, FileName: f.name, DirNames: []string{f.dir}, RelPath: f.dir + "/" + f.name,
		})
	}

	memberA, memberB := refEvalMembers(t)
	combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1},
	})
	rules := []RuleEntry{memberA, memberB, combo}

	// 通道一：批量识别（全集自聚合）。
	batch := MergedRecognition(scanFiles, rules, nil, nil)
	// 通道二：增量识别形态（本批仅首个文件 + 全量树聚合）。
	incremental := MergedRecognitionWithTree(scanFiles[:1], scanFiles, rules, nil, nil)
	// 通道三：执行期单文件重算（磁盘重扫兄弟全集）。
	h := &LocalImportWorkFetcher{}

	for i, f := range files {
		fp := &FilePluginData{FullPath: scanFiles[i].Path, RelPath: scanFiles[i].RelPath}
		recomputed, _ := h.recomputeFileDecision(fp, rules, nil)

		wantCombo := f.comboHit
		if got := refIDsContain(recomputed.MatchedRuleIDs, "combo-1"); got != wantCombo {
			t.Fatalf("%s：重算通道组合门控不符（期望含组合=%v）: %v", f.dir, wantCombo, recomputed.MatchedRuleIDs)
		}
		if got := refIDsContain(batch[i].MatchedRuleIDs, "combo-1"); got != wantCombo {
			t.Fatalf("%s：批量通道组合门控不符（期望含组合=%v）: %v", f.dir, wantCombo, batch[i].MatchedRuleIDs)
		}
		// 三通道命中序列全等（顺序与成员单点登记口径一并锚定）。
		if i == 0 {
			if !refIDsEqual(incremental[0].MatchedRuleIDs, batch[0].MatchedRuleIDs) {
				t.Fatalf("增量通道与批量通道命中序列不一致: %v vs %v", incremental[0].MatchedRuleIDs, batch[0].MatchedRuleIDs)
			}
		}
		if !refIDsEqual(recomputed.MatchedRuleIDs, batch[i].MatchedRuleIDs) {
			t.Fatalf("%s：重算通道与批量通道命中序列不一致: %v vs %v", f.dir, recomputed.MatchedRuleIDs, batch[i].MatchedRuleIDs)
		}
	}

	// 全命中文件的三通道终局序列锚（成员单点登记 + 组合殿后）。
	assertRefMatchedIDs(t, batch[0].MatchedRuleIDs, "member-a", "member-b", "combo-1")
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
