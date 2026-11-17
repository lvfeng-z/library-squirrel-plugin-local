// 或分支行提取接线锚定测试（诊断 2026-11-17，规则 manual-标签-首层标签目录-
// 1794910720657）：两行 or 目录行（[{标签}] / {标签}，均挂库根紧邻附件）只配了
// 一条无 row 的提取动作——Row 缺省按第 1 行兜底（captureValues，rule_profile.go），
// 行2 单独命中时 r1.tag 无绑定 → 零声明（规则照常命中，条件面无缺陷：or 切段
// 编译、库根紧邻层距阶梯、{标签} 惰性段均正常）。补第二条 row:2 提取动作后
// 行2 命中正常产声明；行1 命中受 Or 短路保护（rule_pattern.go）不双重声明。
// 本文件锚定「规则命中（MatchedRuleIDs）与声明产出（Declarations）是两个独立
// 观测面」的接线语义，供编辑面构造多行规则时参照。
package main

import (
	"fmt"
	"testing"
)

// tagFirstLevelRuleJSON 真实库原样规则 JSON（plugin_storage: plugin_id=2,
// key=rule-profile，2026-11-17 只读摘录）：两行 or 目录行 + 单条无 row 的标签
// 提取动作——用户报告「行2 未能识别任意首层目录」的原始构造形态。
const tagFirstLevelRuleJSON = `{"id":"manual-标签-首层标签目录-1794910720657","description":"【标签】首层标签目录","source":"manual","enabled":true,"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"[{标签}]"},"attach":[{"kind":"refScope","minCount":1,"value":"$root"}]}},{"connector":"or","condition":{"target":"dir","format":{"mode":"template","tpl":"{标签}"},"attach":[{"kind":"refScope","minCount":1,"value":"$root"}]}}],"extractions":[{"dimension":"tag","from":"capture","captureName":"tag","constant":"","roleName":""}],"synthesis":{"dimPolicies":[],"roles":[]}}`

// TestOrRowExtractionWiringStoredRule 原样规则（仅行1 接提取）的四面目击：
// 载入零告警（Row 缺省=第 1 行兜底是合法书写面）、括号首层目录命中且产声明、
// 任意首层目录**命中但零声明**（症状机理复现——非条件缺陷而是提取接线缺口）、
// 深层文件经首层目录子树包含命中但同样零声明。
func TestOrRowExtractionWiringStoredRule(t *testing.T) {
	const ruleID = "manual-标签-首层标签目录-1794910720657"
	profile, errs := ParseProfile([]byte(fmt.Sprintf(`{"schemaVersion":%d,"rules":[%s]}`,
		CurrentSchemaVersion, tagFirstLevelRuleJSON)))
	if len(errs) != 0 {
		t.Fatalf("原样规则载入应零告警（Row 缺省=第 1 行兜底为合法书写面）: %v", errs)
	}
	if len(profile.Rules) != 1 || profile.Rules[0].ID != ruleID {
		t.Fatalf("原样规则应载入 1 条: %d 条", len(profile.Rules))
	}
	rules := profile.Rules

	t.Run("括号首层目录-行1命中且产声明", func(t *testing.T) {
		decision := runRefEval(t, rules, rootEvalInput("[标签X]"), nil)
		if !refIDsContain(decision.MatchedRuleIDs, ruleID) {
			t.Fatalf("行1 应命中括号首层目录: %v", decision.MatchedRuleIDs)
		}
		if v, ok := declOf(decision, ruleID, DimTag); !ok || v != "标签X" {
			t.Fatalf("行1 命中应经 r1.tag 产声明（期望 标签X）: %q %v", v, ok)
		}
	})
	t.Run("任意首层目录-命中但零声明", func(t *testing.T) {
		decision := runRefEval(t, rules, rootEvalInput("SomeDir"), nil)
		if !refIDsContain(decision.MatchedRuleIDs, ruleID) {
			t.Fatalf("行2 应命中任意首层目录（or 切段/库根紧邻/惰性段均正常）: %v", decision.MatchedRuleIDs)
		}
		if _, ok := declOf(decision, ruleID, DimTag); ok {
			t.Fatalf("仅接 r1.tag 时行2 单独命中不应产标签声明")
		}
		if len(decision.Declarations) != 0 {
			t.Fatalf("该规则唯一提取动作来源缺失，声明应恰为 0: %+v", decision.Declarations)
		}
	})
	t.Run("深层文件-经首层目录命中仍零声明", func(t *testing.T) {
		// dir 目标=子树包含语义：Shell/Deep/x.jpg 的祖先链上 Shell@0 过库根
		// 紧邻附件（深层 Deep@1 不合格，深→浅回退首层命中）——行2 照常命中，
		// 仅接 r1.tag 时依旧零声明。
		decision := runRefEval(t, rules, rootEvalInput("Shell", "Deep"), nil)
		if !refIDsContain(decision.MatchedRuleIDs, ruleID) {
			t.Fatalf("子树包含语义下经首层目录应命中: %v", decision.MatchedRuleIDs)
		}
		if len(decision.Declarations) != 0 {
			t.Fatalf("仅接 r1.tag 时深层文件同样零声明: %+v", decision.Declarations)
		}
	})
}

// TestOrRowExtractionWiringRow2WiredDeclares 接线修正形态（提取动作补 row:2
// 引用第 2 行标签）的三面锚：任意首层目录经 r2.tag 正常产声明（内核无缺陷）、
// 括号首层目录仅行1 产声明（Or 短路：行1 命中后行2 不求值不绑定，无双重声明）、
// 深层文件取最深合格层（首层目录名）产声明。
func TestOrRowExtractionWiringRow2WiredDeclares(t *testing.T) {
	attach := []CondxAttach{refScopeAttach(RefScopeLibraryRoot, 1)}
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("[{标签}]"), Attach: attach}},
		{Connector: "or", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("{标签}"), Attach: attach}},
	}
	rule := refRowsRule(t, "tag-first-level-fixed", rows, []ExtractionAction{
		{Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1},
		{Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 2},
	})

	t.Run("任意首层目录-r2声明正常产出", func(t *testing.T) {
		decision := runRefEval(t, []RuleEntry{rule}, rootEvalInput("SomeDir"), nil)
		if !refIDsContain(decision.MatchedRuleIDs, rule.ID) {
			t.Fatalf("行2 应命中任意首层目录: %v", decision.MatchedRuleIDs)
		}
		if v, ok := declOf(decision, rule.ID, DimTag); !ok || v != "SomeDir" {
			t.Fatalf("行2 命中且提取接 r2.tag 时声明应正常产出（期望 SomeDir）: %q %v", v, ok)
		}
	})
	t.Run("括号首层目录-仅行1单声明", func(t *testing.T) {
		decision := runRefEval(t, []RuleEntry{rule}, rootEvalInput("[标签X]"), nil)
		if !refIDsContain(decision.MatchedRuleIDs, rule.ID) {
			t.Fatalf("行1 应命中括号首层目录: %v", decision.MatchedRuleIDs)
		}
		if got := len(decision.Declarations); got != 1 {
			t.Fatalf("Or 短路下括号目录应仅行1 一条声明，实际 %d: %+v", got, decision.Declarations)
		}
		if v, ok := declOf(decision, rule.ID, DimTag); !ok || v != "标签X" {
			t.Fatalf("行1 声明值应为去括号段（期望 标签X）: %q %v", v, ok)
		}
	})
	t.Run("深层文件-取最深合格层声明", func(t *testing.T) {
		// 深层目录不合格（Deep@1 非首层），深→浅回退到首层 Shell 命中——
		// 标签值取首层目录名（子树包含语义，最深合格层优先）。
		decision := runRefEval(t, []RuleEntry{rule}, rootEvalInput("Shell", "Deep"), nil)
		if !refIDsContain(decision.MatchedRuleIDs, rule.ID) {
			t.Fatalf("经首层目录应命中: %v", decision.MatchedRuleIDs)
		}
		if v, ok := declOf(decision, rule.ID, DimTag); !ok || v != "Shell" {
			t.Fatalf("标签值应取最深合格层（首层 Shell）: %q %v", v, ok)
		}
	})
}
