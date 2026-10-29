package main

// 定群规则命中计数回归锚（2026-10-29 用户拍板修复存量缺口）：纯定群规则
// （v4 全 seq 条件行、零提取动作）不进提取段/MatchedRuleIDs，命中数应按
// 定群成员文件数计——SnapRuleStat.HitCount 的字段契约一直如此声明，但存量
// 实现只数 MatchedRuleIDs，令一切纯定群规则（含单行）命中计数恒 0。修复=
// 元素6 统计在 MatchedRuleIDs 之外对规则群卡（RuleID 非空）按成员文件数
// 单点汇总；结构群（RuleID 空）不计；定群（分组段独占）与提取（提取段
// 独占）两类通道按规则构造互斥，无重复计数面。
import (
	"path/filepath"
	"testing"
)

func TestRuleStatsCountGroupingRuleMembers(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"たじ_001.jpg", "たじ_002.jpg", "たじ_003.jpg"} {
		writeTestFile(t, filepath.Join(root, "序列", name), "x")
	}

	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetSeq,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{任意}_{序号}"},
		Attach: []CondxAttach{{Kind: CondxAttachSeqMinLength, MinCount: 2}},
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("构造序列定群规则失败: %v", err)
	}

	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{{
		ID: "seq-group-rule", Description: "测试：纯定群序列规则（零提取动作）",
		Source: RuleSourceManual, Enabled: true,
		Conditions: rows, Pattern: pattern,
	}}}
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	snap := sess.Snapshot
	if snap == nil {
		t.Fatal("识别完成后快照不在场")
	}

	// 前置事实：三成员被该规则定群（规则群卡 RuleID 非空、成员 3）。
	var ruleGroup *SnapGroup
	for i := range snap.Groups {
		if snap.Groups[i].RuleID == "seq-group-rule" {
			ruleGroup = &snap.Groups[i]
			break
		}
	}
	if ruleGroup == nil {
		t.Fatalf("应存在规则群卡（RuleID=seq-group-rule），实际群卡: %+v", snap.Groups)
	}
	if len(ruleGroup.Members) != 3 {
		t.Fatalf("规则群成员数 = %d，期望 3: %+v", len(ruleGroup.Members), ruleGroup)
	}

	// 命中统计：定群成员文件数（存量缺陷下恒 0）。
	for _, stat := range snap.RuleStats {
		if stat.ID == "seq-group-rule" {
			if stat.HitCount != 3 {
				t.Fatalf("纯定群规则命中数 = %d，期望 3（按定群成员文件数计）", stat.HitCount)
			}
			return
		}
	}
	t.Fatalf("规则统计缺 seq-group-rule: %+v", snap.RuleStats)
}
