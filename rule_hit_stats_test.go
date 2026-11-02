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

// TestRuleStatsDirLevelAnchor 命中计数通道（声明5 第 5 路，AE 阶段5 核验）：
// 元素6 按 MatchedRuleIDs 逐文件累计，条目命中与否已在求值侧按锚定层判定——
// 计数天然为「锚定层形态命中的文件数」：第 1 级 [路灯后辈] 及其子树文件计入
// （子树经第 1 级锚定命中）；更深层的同名形态目录（Art/[路灯后辈]）不入命中
// 面，命中数不虚增。
func TestRuleStatsDirLevelAnchor(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "[路灯后辈]", "a.jpg"), "a")
	writeTestFile(t, filepath.Join(root, "[路灯后辈]", "子", "b.jpg"), "b")
	writeTestFile(t, filepath.Join(root, "Art", "[路灯后辈]", "c.jpg"), "c")

	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetDir,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "[{标签}]"},
		Attach: []CondxAttach{{Kind: CondxAttachDirLevel, MinCount: 1}},
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译层级锚规则失败: %v", err)
	}
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{{
		ID: "level1-bracket", Description: "测试：第 1 级括号目录 → 标签",
		Source: RuleSourceManual, Enabled: true, Conditions: rows, Pattern: pattern,
		Extractions: []ExtractionAction{{Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1}},
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
	// 命中面：第 1 级目录子树（a、b）命中；更深层同名形态目录（c）不命中。
	wantHit := map[string]bool{"[路灯后辈]/a.jpg": true, "[路灯后辈]/子/b.jpg": true}
	for i := range snap.Files {
		hit := false
		for _, id := range snap.Files[i].MatchedRuleIDs {
			if id == "level1-bracket" {
				hit = true
				break
			}
		}
		if hit != wantHit[snap.Files[i].RelPath] {
			t.Fatalf("%s 命中=%v，期望 %v（锚定第 1 级）", snap.Files[i].RelPath, hit, wantHit[snap.Files[i].RelPath])
		}
	}
	for _, stat := range snap.RuleStats {
		if stat.ID == "level1-bracket" {
			if stat.HitCount != 2 {
				t.Fatalf("层级锚规则命中数 = %d，期望 2（仅第 1 级形态子树）", stat.HitCount)
			}
			return
		}
	}
	t.Fatalf("规则统计缺 level1-bracket: %+v", snap.RuleStats)
}
