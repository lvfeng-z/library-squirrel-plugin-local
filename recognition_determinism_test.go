// 识别确定性锚（《识别求值相并行化方案》阶段1·确定性地基）：
// assertSnapshotsCongruent 为快照全等比较助手（确定性基准的断言基元，阶段2 的
// 模式等价锚与跨模式增量轮锚复用）；TestSnapshotSerialDeterminism 在现串行实现
// 上锚定「同输入多轮识别的持久化快照全等」——夹具含平局群形态（同目录同键的
// 规则群与结构群并存），群卡终序由第三决胜键（定群规则 ID）裁决，多轮一致。
// 本文件为并行化车道专属，不与「工作台协议大载荷分页/流式分帧」车道的
// recognition_test.go 未提交改动相交；夹具与助手复用 recognition_test.go 既有
// 定义（newRecognitionFixture / recognitionSampleTree / writeTestFile）。
package main

import (
	"fmt"
	"reflect"
	"testing"
)

// determinismTieDirs 平局形态目录集：每目录同分组键的规则群（下划线形态命中
// 兄弟规则定群，RuleID=test-sibling-set）与结构群（空格形态不落规则定群、由
// 结构对齐成群，RuleID=空串）并存——（目录键, 分组键）双键平局，终序仅由第三
// 决胜键裁决（结构群规则 ID 空串、字典序居先）。
//
// 首项「平局/同键」为语义主形态；其余五项为放大形态，不可或缺：群卡映射
//（aggregatePanorama 的 groups map）超过单桶容量（>8 群扩容分桶）后，map 迭代
// 序才在「相对次序」上真正随机——单桶 map 的迭代只是桶内插入序的随机旋转，
// 相对序恒定，平局对终序会退化为巧合稳定、锚点检不出决胜键缺失。多平局对 +
// 跨桶迭代让「决胜键缺失 → 两轮群卡序漂移」可检（13 张群卡跨两个桶，每轮
// 迭代起始桶随机）。
var determinismTieDirs = []struct{ dir, key string}{
	{"平局", "同键"},
	{"平局甲", "甲键"},
	{"平局乙", "乙键"},
	{"平局丙", "丙键"},
	{"平局丁", "丁键"},
	{"平局戊", "戊键"},
}

// determinismSampleTree 确定性锚夹具树：七类元素样例树（已识别/有线索/序列成群
// /落单/待裁决/目录含义/集名互证各形态，见 recognitionSampleTree）+ 六个平局
// 形态目录（见 determinismTieDirs，每目录 4 文件：`键_01/02.jpg` 规则群 +
// `键 03/04.jpg` 结构群）。上游群集合装配自 map 迭代（序随机），决胜键补齐前
// 两轮即可产出不同平局序（方案声明5），补齐后每轮一致。
func determinismSampleTree(t *testing.T, root string) {
	t.Helper()
	recognitionSampleTree(t, root)
	for _, d := range determinismTieDirs {
		writeTestFile(t, fmt.Sprintf("%s/%s/%s_01.jpg", root, d.dir, d.key), "k")
		writeTestFile(t, fmt.Sprintf("%s/%s/%s_02.jpg", root, d.dir, d.key), "l")
		writeTestFile(t, fmt.Sprintf("%s/%s/%s 03.jpg", root, d.dir, d.key), "m")
		writeTestFile(t, fmt.Sprintf("%s/%s/%s 04.jpg", root, d.dir, d.key), "n")
	}
}

// assertSnapshotsCongruent 快照全等比较助手：两快照深度相等，排除且仅排除两个
// 非结论字段——GeneratedAtMs（快照生成时间，墙钟）与 Stats.SkippedDirs（本轮
// 按 mtime 游标跳过的目录数，轮次事实而非结论）；其余全部字段（文件行/群卡序/
// 待裁决/候选墙/命中统计/集名互证线索/目录含义/增量游标/汇总统计）必须全等。
// 不等时按顶层字段定位首个不一致面，给出可读失败信息。
func assertSnapshotsCongruent(t *testing.T, got, want *RecognitionSnapshot) {
	t.Helper()
	// 值拷贝后改写排除字段（切片/映射共享底层数组，仅改写标量，不触底层）
	a, b := *got, *want
	a.GeneratedAtMs, b.GeneratedAtMs = 0, 0
	a.Stats.SkippedDirs, b.Stats.SkippedDirs = 0, 0
	if reflect.DeepEqual(a, b) {
		return
	}
	check := func(field string, x, y any) {
		t.Helper()
		if reflect.DeepEqual(x, y) {
			return
		}
		t.Fatalf("快照不全等（已排除生成时间/跳过目录数）：%s 不一致\ngot:  %+v\nwant: %+v", field, x, y)
	}
	check("SchemaVersion", a.SchemaVersion, b.SchemaVersion)
	check("RootPath", a.RootPath, b.RootPath)
	check("ProfileDigest", a.ProfileDigest, b.ProfileDigest)
	check("AssignmentRevision", a.AssignmentRevision, b.AssignmentRevision)
	check("Dirs（目录树与层含义）", a.Dirs, b.Dirs)
	check("Files（文件行）", a.Files, b.Files)
	check("Groups（群卡序）", a.Groups, b.Groups)
	check("Candidates（候选墙）", a.Candidates, b.Candidates)
	check("IgnoredCandidates（已忽略候选分仓）", a.IgnoredCandidates, b.IgnoredCandidates)
	check("ConfirmedCandidates（已确认候选分仓）", a.ConfirmedCandidates, b.ConfirmedCandidates)
	check("Pending（待裁决明细）", a.Pending, b.Pending)
	check("RuleStats（规则命中统计）", a.RuleStats, b.RuleStats)
	check("StaleAssignments（死路径赋值）", a.StaleAssignments, b.StaleAssignments)
	check("DirMtimes（增量游标）", a.DirMtimes, b.DirMtimes)
	check("Stats（汇总统计）", a.Stats, b.Stats)
	t.Fatal("快照不全等：深度比较失败但未定位到不一致面（RecognitionSnapshot 字段清单外差异）")
}

// assertTieFixtureShape 平局夹具形态断言：每个平局目录恰有结构群与规则群两张
// 群卡（夹具形态缺损时报错，防锚点静默退化为「无平局可检」）。
func assertTieFixtureShape(t *testing.T, snap *RecognitionSnapshot) {
	t.Helper()
	for _, d := range determinismTieDirs {
		structural, rule := false, false
		for i := range snap.Groups {
			g := &snap.Groups[i]
			if g.DirRelKey != d.dir || g.Key != d.key {
				continue
			}
			switch g.RuleID {
			case "":
				structural = true
			case "test-sibling-set":
				rule = true
			default:
				t.Fatalf("平局目录 %s 出现意外定群规则 %q: %+v", d.dir, g.RuleID, g)
			}
		}
		if !structural || !rule {
			t.Fatalf("平局夹具形态缺损（%s/%s：结构群=%t 规则群=%t，groups: %+v）",
				d.dir, d.key, structural, rule, snap.Groups)
		}
	}
}

// assertGroupCardsTotallyOrdered 群卡终序断言：（目录键, 分组键, 定群规则 ID）
// 三分量严格全序——前两键并列的相邻平局对由第三键裁决（结构群规则 ID 空串、
// 字典序居先），任何相邻两群不得三分量并列。决胜键补齐后比较器为全序，群卡
// 终序与上游 map 迭代序无关；本断言直接钉住该语义。
func assertGroupCardsTotallyOrdered(t *testing.T, snap *RecognitionSnapshot) {
	t.Helper()
	for i := 1; i < len(snap.Groups); i++ {
		a, b := &snap.Groups[i-1], &snap.Groups[i]
		if a.DirRelKey == b.DirRelKey && a.Key == b.Key {
			if b.RuleID == a.RuleID {
				t.Fatalf("群卡 %d/%d 三分量并列（群卡应唯一）: %+v / %+v", i-1, i, a, b)
			}
			if b.RuleID < a.RuleID {
				t.Fatalf("平局对（%s/%s）决胜序不符：结构群（规则 ID 空串）须居先，实际 %q → %q（groups: %+v）",
					a.DirRelKey, a.Key, a.RuleID, b.RuleID, snap.Groups)
			}
			continue
		}
		if a.DirRelKey > b.DirRelKey || (a.DirRelKey == b.DirRelKey && a.Key > b.Key) {
			t.Fatalf("群卡前两键乱序 @%d: %+v → %+v（groups: %+v）", i, a, b, snap.Groups)
		}
	}
}

// TestSnapshotSerialDeterminism 串行确定性锚（测试矩阵第 1/4 项）：同输入三轮
// 识别——轮1/轮2 为同库根两个独立会话（全新存储、同物理树 → DirMtimes 同值）
// 的全量识别，两轮全量装配各自经受群卡 map 迭代随机性；轮3 在轮2 会话上增量
// 重跑（磁盘未变 → 目录全跳过，群卡整组复原后重进 groups 映射再排终序，平局
// 序仍由决胜键裁决）。三轮快照经 assertSnapshotsCongruent 全等（除生成时间/
// 跳过目录数），群卡终序逐轮满足三分量全序，证明第三决胜键消除了上游 map
// 迭代随机性（呈现序确定化，无结论变更）。
func TestSnapshotSerialDeterminism(t *testing.T) {
	root := t.TempDir()
	// 死路径赋值入夹具：StaleAssignments 面也进全等比较
	assigns := map[string]SessionAssignment{
		"不存在的目录/旧文件.jpg": {Author: "旧作者", UpdatedAtMs: 1},
	}
	determinismSampleTree(t, root)

	assertRound := func(label string, snap *RecognitionSnapshot) {
		t.Helper()
		if snap == nil {
			t.Fatalf("%s 识别完成后快照不在场", label)
		}
		assertTieFixtureShape(t, snap)
		assertGroupCardsTotallyOrdered(t, snap)
	}

	// —— 轮1：会话A 全量识别 ——
	mA, _ := newRecognitionFixture(t, root, assigns)
	if err := mA.runFullRecognition(root); err != nil {
		t.Fatalf("轮1 全量识别失败: %v", err)
	}
	first, err := mA.SessionOf(root)
	if err != nil {
		t.Fatalf("读取轮1 会话失败: %v", err)
	}
	if first.Snapshot.Stats.SkippedDirs != 0 {
		t.Fatalf("轮1 全量识别不应跳过目录，实际 %d", first.Snapshot.Stats.SkippedDirs)
	}
	assertRound("轮1", first.Snapshot)

	// —— 轮2：同库根独立会话B（全新存储）再次全量识别——两轮全量 → 快照全等 ——
	mB, _ := newRecognitionFixture(t, root, assigns)
	if err := mB.runFullRecognition(root); err != nil {
		t.Fatalf("轮2 全量识别失败: %v", err)
	}
	second, err := mB.SessionOf(root)
	if err != nil {
		t.Fatalf("读取轮2 会话失败: %v", err)
	}
	if second.Snapshot.Stats.SkippedDirs != 0 {
		t.Fatalf("轮2 应为全量识别（零跳过），实际 %d", second.Snapshot.Stats.SkippedDirs)
	}
	assertRound("轮2", second.Snapshot)
	assertSnapshotsCongruent(t, second.Snapshot, first.Snapshot)

	// —— 轮3：会话B 增量识别（磁盘未变 → 目录全跳过、结论与群卡整组复用）——
	if err := mB.runFullRecognition(root); err != nil {
		t.Fatalf("轮3 增量识别失败: %v", err)
	}
	third, err := mB.SessionOf(root)
	if err != nil {
		t.Fatalf("读取轮3 会话失败: %v", err)
	}
	if third.Snapshot.Stats.SkippedDirs == 0 {
		t.Fatal("轮3 应为增量识别（跳过未变目录），实际跳过 0 个")
	}
	assertRound("轮3", third.Snapshot)
	// SkippedDirs 在此真正参与「排除」语义：轮3 全跳、轮1 全算，其余字段仍全等
	assertSnapshotsCongruent(t, third.Snapshot, first.Snapshot)
}
