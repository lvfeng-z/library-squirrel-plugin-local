// 可选词位（{词?}）缺席语义锚定：绑定层对未参与本次匹配的命名组跳过绑定登记，
// 四个消费面按既有「取不到值」语义零改动——提取 FromCapture 缺席零声明、值级
// 小标缺席门为假、when 引用缺席条件不成立、组合接线限定名缺席无声明。缺席≠
// 空串绑定：手写 JSON 原生正则域（RegexPattern.matchWithCaptures）仍按历史行为
// 绑定空串，两域缺席语义有意分叉。
package main

import (
	"regexp"
	"testing"
)

// TestBindCondxCapturesAbsentSkip 绑定层单点锚：未参与本次匹配的命名组（子匹配
// 索引 -1）跳过绑定登记；参与匹配但捕获空串的组照常登记（判定只看索引，不看
// 值是否为空，避免误伤捕获类改可空后的空串命中）。
func TestBindCondxCapturesAbsentSkip(t *testing.T) {
	t.Run("缺席可选词零绑定", func(t *testing.T) {
		re := regexp.MustCompile(`^x(?:(?P<author>.+?))?y$`)
		ctx := &ParseContext{}
		if !bindCondxCaptures(re, "xy", []string{"author"}, nil, ctx) {
			t.Fatalf("xy 应命中可选词位模板正则")
		}
		if got := ctx.CaptureAll("author"); len(got) != 0 {
			t.Fatalf("缺席可选词不应登记绑定，得到 %+v", got)
		}
		if _, ok := ctx.Capture("author"); ok {
			t.Fatalf("缺席可选词按名取值应返回不存在")
		}
	})

	t.Run("在场可选词照常绑定", func(t *testing.T) {
		re := regexp.MustCompile(`^x(?:(?P<author>.+?))?y$`)
		ctx := &ParseContext{}
		if !bindCondxCaptures(re, "x甲y", []string{"author"}, nil, ctx) {
			t.Fatalf("x甲y 应命中可选词位模板正则")
		}
		value, ok := ctx.Capture("author")
		if !ok || value != "甲" {
			t.Fatalf("在场可选词绑定不符: %q %v（期望 甲）", value, ok)
		}
	})

	t.Run("参与匹配但捕获空串照常绑定", func(t *testing.T) {
		// 表达式内组 a 参与匹配且捕获空串（索引 0,0 非 -1）：不得按「值为空」
		// 误判为缺席而跳过绑定。
		re := regexp.MustCompile(`^(?P<a>)(?P<b>x)$`)
		ctx := &ParseContext{}
		if !bindCondxCaptures(re, "x", []string{"a", "b"}, nil, ctx) {
			t.Fatalf("x 应命中空串捕获正则")
		}
		got := ctx.CaptureAll("a")
		if len(got) != 1 || got[0].Value != "" || got[0].Start != 0 || got[0].End != 0 {
			t.Fatalf("参与匹配的空串捕获应登记为空值零区间绑定，得到 %+v", got)
		}
		if value, ok := ctx.Capture("b"); !ok || value != "x" {
			t.Fatalf("同次匹配的其余组绑定不符: %q %v（期望 x）", value, ok)
		}
	})
}

// TestCondxOptionalAbsentExtraction 提取面：FromCapture 引用缺席可选词 → 零绑定
// → 零声明（缺席不产空值声明）。在场形态对照产出取值。
func TestCondxOptionalAbsentExtraction(t *testing.T) {
	rule := refFileRowRule(t, "rule-absent", "x{作者?}y", nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1},
	})
	in := &MatchInput{FileName: "xy.jpg", RelPath: "xy.jpg"}
	decision := runRefEval(t, []RuleEntry{rule}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "rule-absent")
	if _, ok := declOf(decision, "rule-absent", DimAuthor); ok {
		t.Fatalf("缺席可选词不应产出提取声明")
	}

	inPresent := &MatchInput{FileName: "x甲y.jpg", RelPath: "x甲y.jpg"}
	decisionPresent := runRefEval(t, []RuleEntry{rule}, inPresent, nil)
	if v, ok := declOf(decisionPresent, "rule-absent", DimAuthor); !ok || v != "甲" {
		t.Fatalf("在场可选词应产出作者声明: %q %v（期望 甲）", v, ok)
	}
}

// TestCondxOptionalAbsentBadgeGate 值校验面：跨行相等小标的左值缺席 → 取不到
// 值 → 门为假（不得把两个「缺席」判为相等）。构造两行：目录行可选标签缺席、
// 文件行可选作者缺席，文件行相等小标把作者与目录行标签比对。
func TestCondxOptionalAbsentBadgeGate(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{标签?}]")}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("x{作者?}y"),
			Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeEqual, AgainstRow: 1, AgainstPh: "标签"}}}},
	}
	p, err := compileCondxRows(rows)
	if err != nil {
		t.Fatalf("编译条件行失败: %v", err)
	}
	in := &MatchInput{FileName: "xy.jpg", DirNames: []string{"Artist"}}
	if p.Evaluate(in, nil, &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}) {
		t.Fatalf("两行可选词均缺席时相等小标应门为假")
	}
}

// TestCondxOptionalAbsentWhenCondition when 面：引用缺席可选词的条件操作数取不到
// 值（ok=false）→ 比较为假 → 动作不产出；不能把缺席当空串去参与相等比较。
func TestCondxOptionalAbsentWhenCondition(t *testing.T) {
	rule := refFileRowRule(t, "rule-when", "x{作者?}y", nil, []ExtractionAction{
		{Dimension: DimTitle, From: FromConstant, Constant: "when产出",
			When: refEqExpr("r1.author", "")},
	})
	in := &MatchInput{FileName: "xy.jpg", RelPath: "xy.jpg"}
	decision := runRefEval(t, []RuleEntry{rule}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "rule-when")
	if _, ok := declOf(decision, "rule-when", DimTitle); ok {
		t.Fatalf("when 引用缺席可选词应条件不成立、不产出声明")
	}
}

// TestCondxOptionalAbsentRefCapture 组合接线面：被引成员的可选词缺席 → 成员捕获
// 快照无该限定键 → 组合 refCapture 来源缺失零声明。
func TestCondxOptionalAbsentRefCapture(t *testing.T) {
	member := refFileRowRule(t, "member-absent", "x{作者?}y", nil, nil)
	combo := refComboRule(t, "combo-absent", []string{"member-absent"}, nil, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-absent", CaptureName: "author", Row: 1},
	})
	in := &MatchInput{FileName: "xy.jpg", RelPath: "xy.jpg"}
	decision := runRefEval(t, []RuleEntry{member, combo}, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "member-absent", "combo-absent")
	if _, ok := declOf(decision, "combo-absent", DimAuthor); ok {
		t.Fatalf("组合 refCapture 引用缺席可选词应来源缺失零声明")
	}
}
