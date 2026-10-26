// 规则组合引用的提取与规范名（规则组合引用方案阶段3）：Apply 的 refCapture
// 取值分支（限定键命中、#canon 同序回退、Row≤0 第 1 行兜底、剥壳适用、成员
// 多行多同名组的行限定、未绑定零产出、pattern 形态成员裸绑定终备）与 when
// 条件操作数的限定名解析（求值侧 conditionOperandValue 与构造校验侧
// validateExpr 成对——限定键形按 refs 放行、非限定名照旧走自有捕获），以及
// 发现6 回归锚（跨规则捕获仍隔离、限定键为唯一通道、非组合规则零变化）。
// 锚定测试矩阵第 2/3 项的阶段3 面。
package main

import (
	"strings"
	"testing"
)

// refDeclOf 取决策内某规则在某维度上的首条完整声明（供来源与归属断言）。
func refDeclOf(decision *FileDecision, ruleID string, dim MetadataDimension) (MetadataDeclaration, bool) {
	for i := range decision.Declarations {
		if decision.Declarations[i].RuleID == ruleID && decision.Declarations[i].Dimension == dim {
			return decision.Declarations[i], true
		}
	}
	return MetadataDeclaration{}, false
}

// refEqExpr 构造「捕获组 == 字符串字面量」相等比较（when 限定名用例的操作面）。
func refEqExpr(capture, str string) *CondExpr {
	return &CondExpr{Kind: ExprEqual,
		Left:  &CondExpr{Kind: ExprCapture, CaptureName: capture},
		Right: &CondExpr{Kind: ExprString, Str: str}}
}

// refNeExpr 构造「捕获组 != 字符串字面量」不等比较。
func refNeExpr(capture, str string) *CondExpr {
	return &CondExpr{Kind: ExprNotEqual,
		Left:  &CondExpr{Kind: ExprCapture, CaptureName: capture},
		Right: &CondExpr{Kind: ExprString, Str: str}}
}

// —— 测试矩阵第 2 项：refCapture 提取取值 ——

// TestRefCaptureApplyValue 限定键命中：组合规则经 refCapture 取到成员捕获值，
// 产出声明来源=SourceFilename（值来自成员规则对文件名的解析捕获，与
// FromCapture 同款）、归属组合规则自身 ID；成员独立产出照旧（并集语义）。
// 成员未绑定的组名＝来源缺失零产出不报错（保守取向）。
func TestRefCaptureApplyValue(t *testing.T) {
	t.Run("限定键命中与声明归属", func(t *testing.T) {
		memberA, memberB := refEvalMembers(t)
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1},
		})
		decision := runRefEval(t, []RuleEntry{memberA, memberB, combo}, refHitIn(), nil)
		decl, ok := refDeclOf(decision, "combo-1", DimAuthor)
		if !ok || decl.Value != "甲" {
			t.Fatalf("限定键取值不符: %+v（在场=%v，期望 甲）", decl, ok)
		}
		if decl.Source != SourceFilename {
			t.Fatalf("refCapture 产出声明来源应为 SourceFilename，实际 %q", decl.Source)
		}
		if decl.RuleID != "combo-1" {
			t.Fatalf("refCapture 产出应归属组合规则自身 ID，实际 %q", decl.RuleID)
		}
		if v, ok := declOf(decision, "member-a", DimAuthor); !ok || v != "甲" {
			t.Fatalf("成员独立作者声明应保留: %q %v", v, ok)
		}
	})

	t.Run("未绑定组零产出不报错", func(t *testing.T) {
		memberA, memberB := refEvalMembers(t)
		combo := refComboRule(t, "combo-1", []string{"member-a", "member-b"}, nil, []ExtractionAction{
			{Dimension: DimTag, From: FromRefCapture, RefRule: "member-a", CaptureName: "tag", Row: 1},
		})
		decision := runRefEval(t, []RuleEntry{memberA, memberB, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a", "member-b", "combo-1")
		if _, ok := declOf(decision, "combo-1", DimTag); ok {
			t.Fatalf("成员未绑定的组名应来源缺失零产出（不报错）")
		}
	})

	t.Run("pattern形态成员裸绑定终备", func(t *testing.T) {
		// pattern 形态成员规则绑定裸组名（无行号前缀）：Row 缺省的 refCapture
		// 经裸限定键终备取值——对齐 FromCapture 分支的裸名终备。
		member := RuleEntry{
			ID: "member-p", Description: "pattern 形态成员", Source: RuleSourceManual, Enabled: true,
			Pattern: mustRegex(t, string(TargetFileName), `^(?P<author>[^_]+)_.+$`, []string{"author"}),
		}
		combo := refComboRule(t, "combo-1", []string{"member-p"}, nil, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-p", CaptureName: "author"},
		})
		in := &MatchInput{FileName: "甲_乙.jpg", RelPath: "甲_乙.jpg"}
		decision := runRefEval(t, []RuleEntry{member, combo}, in, nil)
		if v, ok := declOf(decision, "combo-1", DimAuthor); !ok || v != "甲" {
			t.Fatalf("裸绑定终备取值不符: %q %v（期望 甲）", v, ok)
		}
	})
}

// TestRefCaptureCanonFallback #canon 同序回退：先规范名限定键
//（成员ID.rN.组名#canon，库名单形变归一命中时成员另存的规范名绑定）后原始
// 限定键——规范名优先；无小标或形变归一未命中时回落原始捕获值。
func TestRefCaptureCanonFallback(t *testing.T) {
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
	run := func(member RuleEntry, fileName string) *FileDecision {
		combo := refComboRule(t, "combo-1", []string{"member-a"}, nil, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1},
		})
		in := &MatchInput{FileName: fileName, RelPath: fileName}
		return runRefEval(t, []RuleEntry{member, combo}, in, lib)
	}

	t.Run("规范名优先", func(t *testing.T) {
		badge := refFileRowRule(t, "member-a", "{作者}_{标题}", []CondxBadge{
			{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true},
		}, nil)
		decision := run(badge, "jake__jaccobus_乙.jpg")
		if v, ok := declOf(decision, "combo-1", DimAuthor); !ok || v != "Jake_Jaccobus" {
			t.Fatalf("库名单命中时应优先取规范名: %q %v（期望 Jake_Jaccobus）", v, ok)
		}
	})
	t.Run("无小标回落原始捕获", func(t *testing.T) {
		plain := refFileRowRule(t, "member-a", "{作者}_{标题}", nil, nil)
		decision := run(plain, "jake__jaccobus_乙.jpg")
		if v, ok := declOf(decision, "combo-1", DimAuthor); !ok || v != "jake__jaccobus" {
			t.Fatalf("无小标应回落原始捕获: %q %v（期望 jake__jaccobus）", v, ok)
		}
	})
	t.Run("小标无规范名绑定回落原始值", func(t *testing.T) {
		// fold=false 的精确命中小标：行命中但只记谓词命中、不另存 #canon
		// 规范名绑定（规范名绑定仅形变归一 fold 命中产出）——refCapture
		// 无规范名可取，回落原始捕获值。
		badge := refFileRowRule(t, "member-a", "{作者}_{标题}", []CondxBadge{
			{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author"},
		}, nil)
		decision := run(badge, "Jake_Jaccobus_乙.jpg")
		if v, ok := declOf(decision, "combo-1", DimAuthor); !ok || v != "Jake_Jaccobus" {
			t.Fatalf("无规范名绑定时应回落原始捕获: %q %v（期望 Jake_Jaccobus）", v, ok)
		}
	})
}

// TestRefCaptureRowDiscrimination 成员多行多同名组的行限定：r1/r2 各自独立
//（行号前缀天然区分同名组）；Row 缺省（0/负）按第 1 行兜底；越界行＝来源
// 缺失零产出。
func TestRefCaptureRowDiscrimination(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]")}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}")}},
	}
	member := refRowsRule(t, "member-a", rows, nil, nil)
	in := &MatchInput{
		FileName: "甲_乙.jpg", DirNames: []string{"Artist[丙]"},
		RelPath: "Artist[丙]/甲_乙.jpg", Siblings: []string{"甲_乙.jpg"},
	}

	combo := refComboRule(t, "combo-1", []string{"member-a"}, nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1},
		{Dimension: DimTag, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 2},
	})
	decision := runRefEval(t, []RuleEntry{member, combo}, in, nil)
	if v, ok := declOf(decision, "combo-1", DimAuthor); !ok || v != "丙" {
		t.Fatalf("r1 行限定应取目录行作者: %q %v（期望 丙）", v, ok)
	}
	if v, ok := declOf(decision, "combo-1", DimTag); !ok || v != "甲" {
		t.Fatalf("r2 行限定应取文件行作者: %q %v（期望 甲）", v, ok)
	}

	for _, row := range []int{0, -1} {
		comboDefault := refComboRule(t, "combo-default", []string{"member-a"}, nil, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: row},
		})
		decision := runRefEval(t, []RuleEntry{member, comboDefault}, in, nil)
		if v, ok := declOf(decision, "combo-default", DimAuthor); !ok || v != "丙" {
			t.Fatalf("Row=%d 应按第 1 行兜底取 丙: %q %v", row, v, ok)
		}
	}

	comboOOB := refComboRule(t, "combo-oob", []string{"member-a"}, nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 3},
	})
	decision = runRefEval(t, []RuleEntry{member, comboOOB}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a", "combo-oob")
	if _, ok := declOf(decision, "combo-oob", DimAuthor); ok {
		t.Fatalf("越界行引用应来源缺失零产出")
	}
}

// TestRefCaptureStrip 剥壳变换适用：StripPrefix/StripSuffix 对 refCapture 取得
// 的成员捕获值照既有变换逻辑剥离（逐侧独立 Trim，值不具该侧时无操作）。
func TestRefCaptureStrip(t *testing.T) {
	member := refFileRowRule(t, "member-a", "{作者}_{标题}", nil, nil)
	combo := refComboRule(t, "combo-1", []string{"member-a"}, nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1,
			StripPrefix: "【", StripSuffix: "】"},
	})
	in := &MatchInput{FileName: "【甲】_乙.jpg", RelPath: "【甲】_乙.jpg"}
	decision := runRefEval(t, []RuleEntry{member, combo}, in, nil)
	if v, ok := declOf(decision, "combo-1", DimAuthor); !ok || v != "甲" {
		t.Fatalf("剥壳后取值不符: %q %v（期望 甲）", v, ok)
	}
}

// —— when 限定名（求值侧 + 构造校验侧成对）——

// TestRefCaptureWhenQualified 求值侧：限定键形操作数（显式形「成员ID.rN.组名」）
// 从注入区取值——相等/不等比较可用；未注入/未绑定＝取值不可用、比较为假
//（保守取向）；#canon 规范名变体按书写名直接命中。非限定名照旧走自有捕获：
// 组合自身行声明的裸名读自身绑定，读不到成员捕获（发现6：限定键为唯一通道）。
func TestRefCaptureWhenQualified(t *testing.T) {
	t.Run("限定名相等与未绑定保守为假", func(t *testing.T) {
		memberA, _ := refEvalMembers(t)
		combo := refComboRule(t, "combo-1", []string{"member-a"}, nil, []ExtractionAction{
			{Dimension: DimTitle, From: FromConstant, Constant: "限定名相等产出",
				When: refEqExpr("member-a.r1.author", "甲")},
			{Dimension: DimCollection, From: FromConstant, Constant: "不应产出",
				When: refEqExpr("member-a.r1.author", "乙")},
			{Dimension: DimTag, From: FromConstant, Constant: "不应产出",
				When: refEqExpr("member-a.r2.author", "甲")},
		})
		decision := runRefEval(t, []RuleEntry{memberA, combo}, refHitIn(), nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-a", "combo-1")
		if v, ok := declOf(decision, "combo-1", DimTitle); !ok || v != "限定名相等产出" {
			t.Fatalf("限定名相等命中应产出: %q %v", v, ok)
		}
		if _, ok := declOf(decision, "combo-1", DimCollection); ok {
			t.Fatalf("限定名不相等不应产出")
		}
		if _, ok := declOf(decision, "combo-1", DimTag); ok {
			t.Fatalf("限定名未绑定应取值不可用、比较为假不产出")
		}
	})

	t.Run("canon书写名与不等比较", func(t *testing.T) {
		badge := refFileRowRule(t, "member-c", "{作者}_{标题}", []CondxBadge{
			{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true},
		}, nil)
		combo := refComboRule(t, "combo-2", []string{"member-c"}, nil, []ExtractionAction{
			{Dimension: DimTitle, From: FromConstant, Constant: "canon相等产出",
				When: refEqExpr("member-c.r1.author#canon", "Jake_Jaccobus")},
			{Dimension: DimTag, From: FromConstant, Constant: "原始值不等产出",
				When: refNeExpr("member-c.r1.author", "someone_else")},
		})
		lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
		in := &MatchInput{FileName: "jake__jaccobus_乙.jpg", RelPath: "jake__jaccobus_乙.jpg"}
		decision := runRefEval(t, []RuleEntry{badge, combo}, in, lib)
		if v, ok := declOf(decision, "combo-2", DimTitle); !ok || v != "canon相等产出" {
			t.Fatalf("canon 书写名应直接命中: %q %v", v, ok)
		}
		if v, ok := declOf(decision, "combo-2", DimTag); !ok || v != "原始值不等产出" {
			t.Fatalf("限定名不等比较应产出: %q %v", v, ok)
		}
	})

	t.Run("非限定名照旧走自有捕获", func(t *testing.T) {
		// 组合携带自身行（Vol[{标签}]）并引用成员B（Artist[{作者}]）：裸名
		// r1.tag 读自身绑定；裸名 r1.author 读不到成员捕获（隔离）；成员值
		// 经限定键 member-b.r1.author 可读——限定键为唯一通道。
		_, memberB := refEvalMembers(t)
		ownRows := []CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Vol[{标签}]")}}}
		combo := refComboRule(t, "combo-3", []string{"member-b"}, ownRows, []ExtractionAction{
			{Dimension: DimTitle, From: FromConstant, Constant: "自有捕获产出",
				When: refEqExpr("r1.tag", "科幻")},
			{Dimension: DimCollection, From: FromConstant, Constant: "不应产出",
				When: refEqExpr("r1.author", "甲")},
			{Dimension: DimTag, From: FromConstant, Constant: "成员限定键产出",
				When: refEqExpr("member-b.r1.author", "甲")},
		})
		in := &MatchInput{
			FileName: "甲_乙.jpg", DirNames: []string{"Vol[科幻]", "Artist[甲]"},
			RelPath:  "Vol[科幻]/Artist[甲]/甲_乙.jpg", Siblings: []string{"甲_乙.jpg"},
		}
		decision := runRefEval(t, []RuleEntry{memberB, combo}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-b", "combo-3")
		if v, ok := declOf(decision, "combo-3", DimTitle); !ok || v != "自有捕获产出" {
			t.Fatalf("自身行裸名应读自身绑定: %q %v", v, ok)
		}
		if _, ok := declOf(decision, "combo-3", DimCollection); ok {
			t.Fatalf("裸名不应读到成员捕获（发现6 隔离）")
		}
		if v, ok := declOf(decision, "combo-3", DimTag); !ok || v != "成员限定键产出" {
			t.Fatalf("成员限定键应可读成员捕获: %q %v", v, ok)
		}
	})
}

// TestRefCaptureWhenValidation 构造校验侧（与求值侧成对放行）：限定键形操作数
// 的成员在本条 refs 内即放行；不在 refs 内（含非组合规则 refs 空）拒绝——
// 限定名不是自有捕获的旁路；非限定名照旧按本规则声明捕获集校验（组合自身行
// 声明的裸名放行、未声明的裸名拒绝），既有行为零变化。
func TestRefCaptureWhenValidation(t *testing.T) {
	member := refsTestRule(t, "member-a", nil, nil)
	qualifiedAction := func(capture string) []ExtractionAction {
		return []ExtractionAction{
			{Dimension: DimAuthor, From: FromConstant, Constant: "x", When: refEqExpr(capture, "甲")},
		}
	}

	t.Run("限定名成员在refs内放行", func(t *testing.T) {
		combo := refsTestRule(t, "combo-1", []string{"member-a"}, qualifiedAction("member-a.r1.author"))
		profile, errs := ParseProfile(refsTestProfileJSON(t, member, combo))
		if len(errs) != 0 {
			t.Fatalf("refs 内限定名应放行，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "member-a", "combo-1")
	})

	t.Run("限定名成员不在refs内拒载", func(t *testing.T) {
		combo := refsTestRule(t, "combo-bad", []string{"member-a"}, qualifiedAction("ghost.r1.author"))
		profile, errs := ParseProfile(refsTestProfileJSON(t, member, combo))
		if len(errs) != 1 {
			t.Fatalf("refs 外限定名应单条告警，实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"combo-bad", "ghost", "refs"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "member-a")
	})

	t.Run("非组合规则限定名恒拒", func(t *testing.T) {
		plain := refsTestRule(t, "plain-1", nil, qualifiedAction("member-a.r1.author"))
		profile, errs := ParseProfile(refsTestProfileJSON(t, member, plain))
		if len(errs) != 1 {
			t.Fatalf("非组合规则的限定名应单条告警，实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"plain-1", "refs"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "member-a")
	})

	t.Run("非限定名照旧走自有声明集", func(t *testing.T) {
		ownRows := []CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Vol[{标签}]")}}}
		okCombo := refRowsRule(t, "combo-ok", ownRows, []string{"member-a"}, []ExtractionAction{
			{Dimension: DimTag, From: FromConstant, Constant: "x", When: refEqExpr("r1.tag", "科幻")},
		})
		profile, errs := ParseProfile(refsTestProfileJSON(t, member, okCombo))
		if len(errs) != 0 {
			t.Fatalf("自身行声明的裸名应照旧放行，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "member-a", "combo-ok")

		badCombo := refRowsRule(t, "combo-bad-own", ownRows, []string{"member-a"}, []ExtractionAction{
			{Dimension: DimAuthor, From: FromConstant, Constant: "x", When: refEqExpr("r1.author", "甲")},
		})
		profile, errs = ParseProfile(refsTestProfileJSON(t, member, badCombo))
		if len(errs) != 1 {
			t.Fatalf("未声明裸名应照旧拒载（单条告警），实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"combo-bad-own", "未声明", "r1.author"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "member-a")
	})
}
