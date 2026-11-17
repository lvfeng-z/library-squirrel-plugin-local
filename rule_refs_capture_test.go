// 供给方捕获（refCapture，作用域附件消费规则专用）的提取与规范面：Apply 的
// 取值分支（限定键命中、#canon 同序回退、Row≤0 第 1 行兜底、剥壳由供给方模板
// 字面括号承载、供给方多行多同名组的行限定、未绑定零产出）与 when 条件操作数
// 的限定名解析（求值侧 conditionOperandValue 与构造校验侧 validateExpr 成对
// ——限定键形按本规则供给方集放行、非限定名照旧走自有捕获），以及发现6 回归
// 锚（跨规则捕获仍隔离、限定键为唯一通道、非消费规则零变化）。供给方一律为
// 纯目录行规则（裁决③），消费规则目录行挂 refScope 附件。
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

// refCaptureConsumer 构造消费规则：目录行挂 refScope 附件（供给方
// providerID、层距 gap=1）+ refCapture/when 动作。
func refCaptureConsumer(t *testing.T, id, providerID, tpl string, actions []ExtractionAction) RuleEntry {
	t.Helper()
	return refDirRowRule(t, id, tpl, []CondxAttach{refScopeAttach(providerID, 1)}, actions)
}

// refBadgeDirProvider 构造带值校验小标的供给方目录行规则（小标须在编译前
// 入行——后置改 Conditions 不进已编译模式）。
func refBadgeDirProvider(t *testing.T, id, tpl string, badges []CondxBadge) RuleEntry {
	t.Helper()
	rows := []CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl(tpl), Badges: badges}}}
	return refRowsRule(t, id, rows, nil)
}

// —— refCapture 提取取值 ——

// TestRefCaptureApplyValue 限定键命中：消费规则经 refCapture 取到供给方捕获值，
// 产出声明来源=SourceFilename（值来自供给方规则对目录名的解析捕获，与
// FromCapture 同款）、归属消费规则自身 ID；供给方独立产出照旧（并集语义）。
// 供给方未绑定的组名＝来源缺失零产出不报错（保守取向）。
//
// 【已删除·refs 退役】pattern 形态成员的裸绑定终备用例（旧组合引用产物）随
// 组合规则退役删除：供给方须为 v4 条件行纯目录规则（裸组名形态不可为供给方），
// Apply 分支的裸名终备保留为防御路径、无构造入口。
func TestRefCaptureApplyValue(t *testing.T) {
	t.Run("限定键命中与声明归属", func(t *testing.T) {
		provider := refDirRowRule(t, "provider-a", "Artist[{作者}]", nil, []ExtractionAction{
			{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1},
		})
		consumer := refCaptureConsumer(t, "consumer-1", "provider-a", "{集名}", []ExtractionAction{
			{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1},
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-a", CaptureName: "author", Row: 1},
		})
		in := &MatchInput{
			FileName: "x.jpg", DirNames: []string{"Artist[甲]", "SetA"},
			RelPath: "Artist[甲]/SetA/x.jpg", Siblings: []string{"x.jpg"},
		}
		decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-a", "consumer-1")
		decl, ok := refDeclOf(decision, "consumer-1", DimAuthor)
		if !ok || decl.Value != "甲" {
			t.Fatalf("限定键取值不符: %+v（在场=%v，期望 甲）", decl, ok)
		}
		if decl.Source != SourceFilename {
			t.Fatalf("refCapture 产出声明来源应为 SourceFilename，实际 %q", decl.Source)
		}
		if decl.RuleID != "consumer-1" {
			t.Fatalf("refCapture 产出应归属消费规则自身 ID，实际 %q", decl.RuleID)
		}
		if v, ok := declOf(decision, "provider-a", DimAuthor); !ok || v != "甲" {
			t.Fatalf("供给方独立作者声明应保留: %q %v", v, ok)
		}
	})

	t.Run("未绑定组零产出不报错", func(t *testing.T) {
		provider := refDirRowRule(t, "provider-a", "Artist[{作者}]", nil, nil)
		consumer := refCaptureConsumer(t, "consumer-1", "provider-a", "{集名}", []ExtractionAction{
			{Dimension: DimTag, From: FromRefCapture, RefRule: "provider-a", CaptureName: "tag", Row: 1},
		})
		in := &MatchInput{
			FileName: "x.jpg", DirNames: []string{"Artist[甲]", "SetA"},
			RelPath: "Artist[甲]/SetA/x.jpg", Siblings: []string{"x.jpg"},
		}
		decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-a", "consumer-1")
		if _, ok := declOf(decision, "consumer-1", DimTag); ok {
			t.Fatalf("供给方未绑定的组名应来源缺失零产出（不报错）")
		}
	})
}

// TestRefCaptureCanonFallback #canon 同序回退：先规范名限定键
//（供给方ID.rN.组名#canon，库名单形变归一命中时供给方另存的规范名绑定）后原始
// 限定键——规范名优先；无小标或形变归一未命中时回落原始捕获值。
func TestRefCaptureCanonFallback(t *testing.T) {
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
	run := func(provider RuleEntry, dirName string) *FileDecision {
		consumer := refCaptureConsumer(t, "consumer-1", "provider-a", "{集名}", []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-a", CaptureName: "author", Row: 1},
		})
		in := &MatchInput{
			FileName: "y.jpg", DirNames: []string{dirName, "SetA"},
			RelPath:  dirName + "/SetA/y.jpg", Siblings: []string{"y.jpg"},
		}
		return runRefEval(t, []RuleEntry{provider, consumer}, in, lib)
	}

	t.Run("规范名优先", func(t *testing.T) {
		badge := refBadgeDirProvider(t, "provider-a", "{作者}_{标题}", []CondxBadge{
			{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true},
		})
		decision := run(badge, "jake jaccobus_乙")
		if v, ok := declOf(decision, "consumer-1", DimAuthor); !ok || v != "Jake_Jaccobus" {
			t.Fatalf("库名单命中时应优先取规范名: %q %v（期望 Jake_Jaccobus）", v, ok)
		}
	})
	t.Run("无小标回落原始捕获", func(t *testing.T) {
		plain := refDirRowRule(t, "provider-a", "{作者}_{标题}", nil, nil)
		decision := run(plain, "jake jaccobus_乙")
		if v, ok := declOf(decision, "consumer-1", DimAuthor); !ok || v != "jake jaccobus" {
			t.Fatalf("无小标应回落原始捕获: %q %v（期望 jake jaccobus）", v, ok)
		}
	})
	t.Run("小标无规范名绑定回落原始值", func(t *testing.T) {
		// fold=false 的精确命中小标：行命中但只记谓词命中、不另存 #canon
		// 规范名绑定（规范名绑定仅形变归一 fold 命中产出）——refCapture
		// 无规范名可取，回落原始捕获值。
		badge := refBadgeDirProvider(t, "provider-a", "{作者} {标题}", []CondxBadge{
			{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author"},
		})
		decision := run(badge, "Jake_Jaccobus 乙")
		if v, ok := declOf(decision, "consumer-1", DimAuthor); !ok || v != "Jake_Jaccobus" {
			t.Fatalf("无规范名绑定时应回落原始捕获: %q %v（期望 Jake_Jaccobus）", v, ok)
		}
	})
}

// TestRefCaptureRowDiscrimination 供给方多行多同名组的行限定：r1/r2 各自独立
//（行号前缀天然区分同名组）；Row 缺省（0/负）按第 1 行兜底；越界行＝来源
// 缺失零产出。
func TestRefCaptureRowDiscrimination(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]")}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("{作者}_{标题}")}},
	}
	provider := refRowsRule(t, "provider-a", rows, nil)
	consumer := refCaptureConsumer(t, "consumer-1", "provider-a", "{集名}", []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-a", CaptureName: "author", Row: 1},
		{Dimension: DimTag, From: FromRefCapture, RefRule: "provider-a", CaptureName: "author", Row: 2},
	})
	in := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"Artist[丙]", "甲_乙", "SetC"},
		RelPath: "Artist[丙]/甲_乙/SetC/x.jpg", Siblings: []string{"x.jpg"},
	}
	decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-a", "consumer-1")
	if v, ok := declOf(decision, "consumer-1", DimAuthor); !ok || v != "丙" {
		t.Fatalf("r1 行限定应取目录行作者: %q %v（期望 丙）", v, ok)
	}
	if v, ok := declOf(decision, "consumer-1", DimTag); !ok || v != "甲" {
		t.Fatalf("r2 行限定应取次行作者: %q %v（期望 甲）", v, ok)
	}

	for _, row := range []int{0, -1} {
		consumerDefault := refCaptureConsumer(t, "consumer-default", "provider-a", "{集名}", []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-a", CaptureName: "author", Row: row},
		})
		decision := runRefEval(t, []RuleEntry{provider, consumerDefault}, in, nil)
		if v, ok := declOf(decision, "consumer-default", DimAuthor); !ok || v != "丙" {
			t.Fatalf("Row=%d 应按第 1 行兜底取 丙: %q %v", row, v, ok)
		}
	}

	consumerOOB := refCaptureConsumer(t, "consumer-oob", "provider-a", "{集名}", []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-a", CaptureName: "author", Row: 3},
	})
	decision = runRefEval(t, []RuleEntry{provider, consumerOOB}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-a", "consumer-oob")
	if _, ok := declOf(decision, "consumer-oob", DimAuthor); ok {
		t.Fatalf("越界行引用应来源缺失零产出")
	}
}

// TestRefCaptureShellByTemplate 剥壳语义由供给方行模板的字面括号承载：捕获类
// 括号感知（占位词前字面以开括号收尾时排除配对闭括号），供给方捕获天然取得
// 括号内名，消费规则 refCapture 原样取值——提取动作不再有独立的剥壳变换。
func TestRefCaptureShellByTemplate(t *testing.T) {
	provider := refDirRowRule(t, "provider-a", "【{作者}】_{标题}", nil, nil)
	consumer := refCaptureConsumer(t, "consumer-1", "provider-a", "{集名}", []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-a", CaptureName: "author", Row: 1},
	})
	in := &MatchInput{
		FileName: "z.jpg", DirNames: []string{"【甲】_乙", "SetA"},
		RelPath: "【甲】_乙/SetA/z.jpg", Siblings: []string{"z.jpg"},
	}
	decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
	if v, ok := declOf(decision, "consumer-1", DimAuthor); !ok || v != "甲" {
		t.Fatalf("括号模板捕获应取内名: %q %v（期望 甲）", v, ok)
	}
}

// —— when 限定名（求值侧 + 构造校验侧成对）——

// TestRefCaptureWhenQualified 求值侧：限定键形操作数（显式形「供给方ID.rN.组名」）
// 从注入区取值——相等/不等比较可用；未注入/未绑定＝取值不可用、比较为假
//（保守取向）；#canon 规范名变体按书写名直接命中。非限定名照旧走自有捕获：
// 消费规则自身行声明的裸名读自身绑定，读不到供给方捕获（发现6：限定键为
// 唯一通道）。
func TestRefCaptureWhenQualified(t *testing.T) {
	t.Run("限定名相等与未绑定保守为假", func(t *testing.T) {
		provider := refDirRowRule(t, "provider-a", "Artist[{作者}]", nil, nil)
		consumer := refCaptureConsumer(t, "consumer-1", "provider-a", "{集名}", []ExtractionAction{
			{Dimension: DimTitle, From: FromConstant, Constant: "限定名相等产出",
				When: refEqExpr("provider-a.r1.author", "甲")},
			{Dimension: DimCollection, From: FromConstant, Constant: "不应产出",
				When: refEqExpr("provider-a.r1.author", "乙")},
			{Dimension: DimTag, From: FromConstant, Constant: "不应产出",
				When: refEqExpr("provider-a.r2.author", "甲")},
		})
		in := &MatchInput{
			FileName: "x.jpg", DirNames: []string{"Artist[甲]", "SetA"},
			RelPath: "Artist[甲]/SetA/x.jpg", Siblings: []string{"x.jpg"},
		}
		decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-a", "consumer-1")
		if v, ok := declOf(decision, "consumer-1", DimTitle); !ok || v != "限定名相等产出" {
			t.Fatalf("限定名相等命中应产出: %q %v", v, ok)
		}
		if _, ok := declOf(decision, "consumer-1", DimCollection); ok {
			t.Fatalf("限定名不相等不应产出")
		}
		if _, ok := declOf(decision, "consumer-1", DimTag); ok {
			t.Fatalf("限定名未绑定应取值不可用、比较为假不产出")
		}
	})

	t.Run("canon书写名与不等比较", func(t *testing.T) {
		badge := refBadgeDirProvider(t, "provider-c", "{作者}_{标题}", []CondxBadge{
			{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true},
		})
		consumer := refCaptureConsumer(t, "consumer-2", "provider-c", "{集名}", []ExtractionAction{
			{Dimension: DimTitle, From: FromConstant, Constant: "canon相等产出",
				When: refEqExpr("provider-c.r1.author#canon", "Jake_Jaccobus")},
			{Dimension: DimTag, From: FromConstant, Constant: "原始值不等产出",
				When: refNeExpr("provider-c.r1.author", "someone_else")},
		})
		lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
		in := &MatchInput{
			FileName: "y.jpg", DirNames: []string{"jake jaccobus_乙", "SetA"},
			RelPath: "jake jaccobus_乙/SetA/y.jpg", Siblings: []string{"y.jpg"},
		}
		decision := runRefEval(t, []RuleEntry{badge, consumer}, in, lib)
		if v, ok := declOf(decision, "consumer-2", DimTitle); !ok || v != "canon相等产出" {
			t.Fatalf("canon 书写名应直接命中: %q %v", v, ok)
		}
		if v, ok := declOf(decision, "consumer-2", DimTag); !ok || v != "原始值不等产出" {
			t.Fatalf("限定名不等比较应产出: %q %v", v, ok)
		}
	})

	t.Run("非限定名照旧走自有捕获", func(t *testing.T) {
		// 消费规则自身行（Vol[{标签}]，带 refScope 附件）+ 供给方
		//（Artist[{作者}]）：裸名 r1.tag 读自身绑定；裸名 r1.author 读不到
		// 供给方捕获（隔离）；供给方值经限定键 provider-b.r1.author 可读
		// ——限定键为唯一通道。
		provider := refDirRowRule(t, "provider-b", "Artist[{作者}]", nil, nil)
		consumer := refDirRowRule(t, "consumer-3", "Vol[{标签}]", []CondxAttach{refScopeAttach("provider-b", 1)}, []ExtractionAction{
			{Dimension: DimTitle, From: FromConstant, Constant: "自有捕获产出",
				When: refEqExpr("r1.tag", "科幻")},
			{Dimension: DimCollection, From: FromConstant, Constant: "不应产出",
				When: refEqExpr("r1.author", "甲")},
			{Dimension: DimTag, From: FromConstant, Constant: "成员限定键产出",
				When: refEqExpr("provider-b.r1.author", "甲")},
		})
		in := &MatchInput{
			FileName: "甲_乙.jpg", DirNames: []string{"Artist[甲]", "Vol[科幻]"},
			RelPath: "Artist[甲]/Vol[科幻]/甲_乙.jpg", Siblings: []string{"甲_乙.jpg"},
		}
		decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
		assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-b", "consumer-3")
		if v, ok := declOf(decision, "consumer-3", DimTitle); !ok || v != "自有捕获产出" {
			t.Fatalf("自身行裸名应读自身绑定: %q %v", v, ok)
		}
		if _, ok := declOf(decision, "consumer-3", DimCollection); ok {
			t.Fatalf("裸名不应读到供给方捕获（发现6 隔离）")
		}
		if v, ok := declOf(decision, "consumer-3", DimTag); !ok || v != "成员限定键产出" {
			t.Fatalf("供给方限定键应可读供给方捕获: %q %v", v, ok)
		}
	})
}

// TestRefCaptureWhenValidation 构造校验侧（与求值侧成对放行）：限定键形操作数
// 的供给方在本规则供给方集内即放行；不在集内（含无附件规则——供给方集空）
// 拒绝——限定名不是自有捕获的旁路；非限定名照旧按本规则声明捕获集校验
//（自身行声明的裸名放行、未声明的裸名拒绝），既有行为零变化。
func TestRefCaptureWhenValidation(t *testing.T) {
	provider := refDirRowRule(t, "provider-a", "Artist[{作者}]", nil, nil)
	providerJSON := func(t *testing.T, r RuleEntry) ruleEntryJSON {
		t.Helper()
		rj, err := ruleToJSON(r)
		if err != nil {
			t.Fatalf("序列化规则失败: %v", err)
		}
		return *rj
	}
	qualifiedAction := func(capture string) []ExtractionAction {
		return []ExtractionAction{
			{Dimension: DimAuthor, From: FromConstant, Constant: "x", When: refEqExpr(capture, "甲")},
		}
	}

	t.Run("限定名供给方在集内放行", func(t *testing.T) {
		consumer := scopeRuleJSON("consumer-1", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("provider-a", 1)})},
			qualifiedAction("provider-a.r1.author"))
		profile, errs := ParseProfile(scopeProfileJSON(t, providerJSON(t, provider), consumer))
		if len(errs) != 0 {
			t.Fatalf("供给方集内限定名应放行，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "provider-a", "consumer-1")
	})

	t.Run("限定名供给方不在集内拒载", func(t *testing.T) {
		consumer := scopeRuleJSON("consumer-bad", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("provider-a", 1)})},
			qualifiedAction("ghost.r1.author"))
		profile, errs := ParseProfile(scopeProfileJSON(t, providerJSON(t, provider), consumer))
		if len(errs) != 1 {
			t.Fatalf("集外限定名应单条告警，实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"consumer-bad", "ghost", "供给方集"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "provider-a")
	})

	t.Run("无附件规则限定名恒拒", func(t *testing.T) {
		plain := scopeRuleJSON("plain-1", []CondxRow{scopeDirRow("{集名}", nil)},
			qualifiedAction("provider-a.r1.author"))
		profile, errs := ParseProfile(scopeProfileJSON(t, providerJSON(t, provider), plain))
		if len(errs) != 1 {
			t.Fatalf("无附件规则的限定名应单条告警，实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"plain-1", "供给方集"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "provider-a")
	})

	t.Run("非限定名照旧走自有声明集", func(t *testing.T) {
		okConsumer := scopeRuleJSON("consumer-ok", []CondxRow{scopeDirRow("Vol[{标签}]", []CondxAttach{refScopeAttach("provider-a", 1)})},
			[]ExtractionAction{{Dimension: DimTag, From: FromConstant, Constant: "x", When: refEqExpr("r1.tag", "科幻")}})
		profile, errs := ParseProfile(scopeProfileJSON(t, providerJSON(t, provider), okConsumer))
		if len(errs) != 0 {
			t.Fatalf("自身行声明的裸名应照旧放行，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "provider-a", "consumer-ok")

		badConsumer := scopeRuleJSON("consumer-bad-own", []CondxRow{scopeDirRow("Vol[{标签}]", []CondxAttach{refScopeAttach("provider-a", 1)})},
			[]ExtractionAction{{Dimension: DimAuthor, From: FromConstant, Constant: "x", When: refEqExpr("r1.author", "甲")}})
		profile, errs = ParseProfile(scopeProfileJSON(t, providerJSON(t, provider), badConsumer))
		if len(errs) != 1 {
			t.Fatalf("未声明裸名应照旧拒载（单条告警），实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"consumer-bad-own", "未声明", "r1.author"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "provider-a")
	})
}

// —— 多位出现的限定名消费锚（模板同占位词多次出现表达方案·阶段2）——

// TestMultiOccurRefCaptureQualifiedHit when/作用域接线限定名锚（锚②）：位2 单值
// 词的组名后缀即寻址键——refCapture 提取行写 author__2 取位2 值、写 author 取
// 位1 值；when 限定名「供给方ID.r1.author__2」命中位2 且不误取位1。
func TestMultiOccurRefCaptureQualifiedHit(t *testing.T) {
	provider := refDirRowRule(t, "provider-2", "{作者}_{作者}", nil, nil)
	consumer := refCaptureConsumer(t, "consumer-2", "provider-2", "{集名}", []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-2", CaptureName: "author", Row: 1},
		{Dimension: DimCollection, From: FromRefCapture, RefRule: "provider-2", CaptureName: "author__2", Row: 1},
		{Dimension: DimTitle, From: FromConstant, Constant: "位2限定名产出",
			When: refEqExpr("provider-2.r1.author__2", "Other")},
		{Dimension: DimTag, From: FromConstant, Constant: "不应产出",
			When: refEqExpr("provider-2.r1.author__2", "Isel")},
		{Dimension: DimTag, From: FromConstant, Constant: "位1限定名产出",
			When: refEqExpr("provider-2.r1.author", "Isel")},
	})
	in := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"Isel_Other", "SetA"},
		RelPath: "Isel_Other/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}
	decision := runRefEval(t, []RuleEntry{provider, consumer}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-2", "consumer-2")
	if v, ok := declOf(decision, "consumer-2", DimAuthor); !ok || v != "Isel" {
		t.Fatalf("接线位1 应取位1 值 Isel: %q %v", v, ok)
	}
	if v, ok := declOf(decision, "consumer-2", DimCollection); !ok || v != "Other" {
		t.Fatalf("接线位2 应取位2 值 Other: %q %v", v, ok)
	}
	if v, ok := declOf(decision, "consumer-2", DimTitle); !ok || v != "位2限定名产出" {
		t.Fatalf("位2 限定名相等应命中: %q %v", v, ok)
	}
	if v, ok := declOf(decision, "consumer-2", DimTag); !ok || v != "位1限定名产出" {
		t.Fatalf("限定名取值不符（位2 不应误取位1）: %q %v", v, ok)
	}
}

// TestMultiOccurCanonSuffixIsolation #canon 后缀隔离锚（锚③）：位2 值校验（库名单
// 折叠）产出 r1.author__2#canon 规范名限定键——接线 refCapture 写 author__2 走
// #canon 同序回退取位2 规范名；位1 无值校验＝无 canon 键，回落位1 原始绑定、
// 不受位2 canon 扰动；when 限定名 canon 书写名按书写直接命中，位1 canon 书写名
// 未绑定＝取值不可用（键位次隔离，位1 与位2 键不互扰）。
func TestMultiOccurCanonSuffixIsolation(t *testing.T) {
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}, "Isel_Isel": {}}}
	provider := refBadgeDirProvider(t, "provider-3", "{作者}_{作者}", []CondxBadge{
		{Placeholder: "作者", Occur: 2, Kind: CondxBadgeLibrary, Dim: "author", Fold: true},
	})
	consumer := refCaptureConsumer(t, "consumer-3", "provider-3", "{集名}", []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-3", CaptureName: "author", Row: 1},
		{Dimension: DimCollection, From: FromRefCapture, RefRule: "provider-3", CaptureName: "author__2", Row: 1},
		{Dimension: DimTitle, From: FromConstant, Constant: "不应产出",
			When: refEqExpr("provider-3.r1.author#canon", "Jake_Jaccobus")},
		{Dimension: DimTitle, From: FromConstant, Constant: "位2canon书写名产出",
			When: refEqExpr("provider-3.r1.author__2#canon", "Isel_Isel")},
		{Dimension: DimTag, From: FromConstant, Constant: "不应产出",
			When: refEqExpr("provider-3.r1.author__2#canon", "Jake_Jaccobus")},
		{Dimension: DimTag, From: FromConstant, Constant: "位2原始书写名产出",
			When: refEqExpr("provider-3.r1.author__2", "isel isel")},
	})
	in := &MatchInput{
		FileName: "x.jpg", DirNames: []string{"jake jaccobus_isel isel", "SetA"},
		RelPath: "jake jaccobus_isel isel/SetA/x.jpg", Siblings: []string{"x.jpg"},
	}
	decision := runRefEval(t, []RuleEntry{provider, consumer}, in, lib)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider-3", "consumer-3")
	if v, ok := declOf(decision, "consumer-3", DimAuthor); !ok || v != "jake jaccobus" {
		t.Fatalf("位1 无 canon 应回落位1 原始绑定: %q %v", v, ok)
	}
	if v, ok := declOf(decision, "consumer-3", DimCollection); !ok || v != "Isel_Isel" {
		t.Fatalf("位2 接线应 #canon 优先取位2 规范名: %q %v", v, ok)
	}
	if v, ok := declOf(decision, "consumer-3", DimTitle); !ok || v != "位2canon书写名产出" {
		t.Fatalf("位2 canon 书写名应直接命中（位1 canon 键未绑定为假在前）: %q %v", v, ok)
	}
	if v, ok := declOf(decision, "consumer-3", DimTag); !ok || v != "位2原始书写名产出" {
		t.Fatalf("位2 原始书写名应照旧可读（canon 键不互扰）: %q %v", v, ok)
	}
}
