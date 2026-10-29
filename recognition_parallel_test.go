// 识别求值相并行化（《识别求值相并行化方案》阶段2·求值相并行化）的锚定测试
// （测试矩阵第 2/3/5 项）：
//   - TestMergedRecognitionParallel 模式等价锚：同输入 1 工作子 vs 多工作子两
//     模式——逐文件结论深度相等（reflect.DeepEqual）+ 全量识别快照全等；
//   - TestCrossModeIncremental 跨模式增量轮锚：串行轮1→并行轮2、并行轮1→串行
//     轮2，轮间施混合变更集（部分目录 mtime 变更+新目录）——增量轮快照与变更
//     后全量轮全等、群卡只增不减（「升级前产快照、升级后跑增量轮」的等价性）。
// 两锚在 -race 下复跑即竞态锚（共享态只读声明的运行期验证，方案风险1）。并行度
// 经 evalWorkerCount（求值相并行度单一取值点）覆写注入——同一实现的两模式互证。
// 夹具复用阶段1 助手（assertSnapshotsCongruent / assertTieFixtureShape /
// assertGroupCardsTotallyOrdered / determinismSampleTree）与 recognition_test.go
// 既有构造件（newRecognitionFixture / writeTestFile / mustRegexPattern）。本文件
// 为并行化车道专属，不与「工作台协议大载荷分页/流式分帧」车道相交。
package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
)

// parallelWorkersMulti 多工作子模式的固定工作子数：与机器核数解耦（低核环境
// 不令锚点退化为串行路径）；配合夹具体量（文件数越过分块阈值 128、目录数 ≥4）
// 保证两并行区真正走多工作子分派。
const parallelWorkersMulti = 4

// withEvalWorkers 把求值相并行度覆写为指定工作子数，测试结束自动复原（多次
// 覆写按 LIFO 复原，后调先生效）。
func withEvalWorkers(t *testing.T, n int) {
	t.Helper()
	orig := evalWorkerCount
	evalWorkerCount = func() int { return n }
	t.Cleanup(func() { evalWorkerCount = orig })
}

// parallelAssigns 并行锚的会话赋值集：死路径赋值（StaleAssignments 面）+ 活赋值
// 覆盖（规则群成员覆盖 author / 结构群成员覆盖 title / 网格文件多维度+标签）。
func parallelAssigns() map[string]SessionAssignment {
	return map[string]SessionAssignment{
		"不存在的目录/旧文件.jpg":             {Author: "旧作者", UpdatedAtMs: 1},
		"平局/同键_01.jpg":                 {Author: "手工作者甲", UpdatedAtMs: 2},
		"结构已识别/【作者A】标题 01.jpg": {Title: "手工标题乙", UpdatedAtMs: 3},
		"网格03/网格_001.jpg":             {Author: "网格作者", Title: "网格标题", Tags: []string{"手工标签"}, UpdatedAtMs: 4},
	}
}

// parallelSampleTree 并行锚夹具树：determinismSampleTree（七类元素样例树+六个
// 平局形态目录）之上叠加并行区覆盖面——
//   - 结构群全员已识别：空格形态不入下划线规则定群、由结构对齐成群，括号命名
//     惯例规则对每成员提取 author/title → 全员 identified；
//   - 组合规则门段样板：「门段样板」文件（空格形态，另成一组结构群）命中成员
//     正则、纯门组合（refs 单成员、空条件表——命中前提全部由成员门段承担）；
//   - 体量网格：12 目录 × 60 文件（下划线形态规则定群）——总文件数越过分块
//     阈值（evalFileChunkSize=128）→ 并行区2 真正多块，目录数 ≥ 工作子数 →
//     并行区1 真正多目录（两模式锚非空转的形态保证）。
func parallelSampleTree(t *testing.T, root string) {
	t.Helper()
	determinismSampleTree(t, root)
	for i := 1; i <= 3; i++ {
		writeTestFile(t, fmt.Sprintf("%s/结构已识别/【作者A】标题 %02d.jpg", root, i), "s")
	}
	writeTestFile(t, root+"/组合/门段样板 01.jpg", "t")
	writeTestFile(t, root+"/组合/门段样板 02.jpg", "u")
	writeTestFile(t, root+"/组合/门段样板 03.jpg", "v")
	for d := 0; d < 12; d++ {
		for i := 1; i <= 60; i++ {
			writeTestFile(t, fmt.Sprintf("%s/网格%02d/网格_%03d.jpg", root, d, i), "w")
		}
	}
}

// newParallelFixture 并行锚测试环境：newRecognitionFixture（内存存储+桩上下文+
// 测试档案）之上追加组合规则对——成员=文件名正则，组合=纯门（refs 单成员、空
// 条件表），经 SaveProfile→LoadProfile 与识别干跑同款装载路径往返。
func newParallelFixture(t *testing.T, root string, assigns map[string]SessionAssignment) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	m, ctx := newRecognitionFixture(t, root, assigns)
	profile, errs := LoadProfile(m.store)
	if len(errs) > 0 {
		t.Fatalf("载入基础档案失败: %v", errs)
	}
	profile.Rules = append(profile.Rules,
		RuleEntry{
			ID: "parallel-gate-member", Description: "并行锚：门段成员（文件名正则）", Source: RuleSourceManual, Enabled: true,
			Pattern: mustRegexPattern(t, `^门段样板.+\.jpg$`, nil),
		},
		RuleEntry{
			ID: "parallel-gate-combo", Description: "并行锚：纯门组合（refs 单成员）", Source: RuleSourceManual, Enabled: true,
			Refs: []string{"parallel-gate-member"}, Conditions: []CondxRow{}, Pattern: And{},
		},
	)
	if err := SaveProfile(m.store, profile); err != nil {
		t.Fatalf("写入并行锚档案失败: %v", err)
	}
	return m, ctx
}

// assertParallelConclusionShape 夹具形态断言（结论层）：覆盖面缺损时报错，防
// 等价锚静默退化为「无可比形态」——结构群全员已识别（成员行带 author 终选）、
// 组合规则门段命中（成员与组合双双进 MatchedRuleIDs）、赋值覆盖（author/title
// 按维度改写终选）。
func assertParallelConclusionShape(t *testing.T, conclusions []FileConclusion) {
	t.Helper()
	byRel := make(map[string]FileConclusion, len(conclusions))
	for _, c := range conclusions {
		byRel[c.RelPath] = c
	}
	hasDecl := func(c FileConclusion, dim MetadataDimension, value string) bool {
		for _, decl := range c.Final {
			if decl.Dimension == dim && decl.Value == value {
				return true
			}
		}
		return false
	}
	// 结构群全员已识别：三个成员均为结构群补位（Group 非空且非规则定群）且带
	// 括号规则 author 终选
	for i := 1; i <= 3; i++ {
		rel := fmt.Sprintf("结构已识别/【作者A】标题 %02d.jpg", i)
		c, ok := byRel[rel]
		if !ok {
			t.Fatalf("夹具形态缺损：结论缺 %s", rel)
		}
		if c.Group == nil || c.GroupRuleID != "" {
			t.Fatalf("夹具形态缺损：%s 应为结构群成员（Group=%+v, GroupRuleID=%q）", rel, c.Group, c.GroupRuleID)
		}
		if !hasDecl(c, DimAuthor, "作者A") {
			t.Fatalf("夹具形态缺损：%s 应带 author=作者A 终选（结构群全员已识别形态），实际终选 %+v", rel, c.Final)
		}
	}
	// 组合规则门段：成员命中 + 纯门组合命中（门段在结论层双登记）
	for i := 1; i <= 3; i++ {
		rel := fmt.Sprintf("组合/门段样板 %02d.jpg", i)
		c, ok := byRel[rel]
		if !ok {
			t.Fatalf("夹具形态缺损：结论缺 %s", rel)
		}
		member, combo := false, false
		for _, id := range c.MatchedRuleIDs {
			switch id {
			case "parallel-gate-member":
				member = true
			case "parallel-gate-combo":
				combo = true
			}
		}
		if !member || !combo {
			t.Fatalf("夹具形态缺损：%s 门段命中缺员（member=%t combo=%t, matched=%v）", rel, member, combo, c.MatchedRuleIDs)
		}
	}
	// 赋值覆盖：规则群成员 author 改写；结构群成员 title 改写（未指定维度保留）
	if c := byRel["平局/同键_01.jpg"]; !hasDecl(c, DimAuthor, "手工作者甲") {
		t.Fatalf("夹具形态缺损：赋值覆盖未生效（平局/同键_01.jpg 终选 %+v）", c.Final)
	}
	if c := byRel["结构已识别/【作者A】标题 01.jpg"]; !hasDecl(c, DimTitle, "手工标题乙") || !hasDecl(c, DimAuthor, "作者A") {
		t.Fatalf("夹具形态缺损：赋值按维度覆盖未生效（title 改写/author 保留），终选 %+v", c.Final)
	}
	if c := byRel["网格03/网格_001.jpg"]; !hasDecl(c, DimAuthor, "网格作者") || !hasDecl(c, DimTitle, "网格标题") {
		t.Fatalf("夹具形态缺损：网格赋值多维度覆盖未生效，终选 %+v", c.Final)
	}
}

// mutateMixedRound 混合增量轮的轮间变更集：部分目录 mtime 变更（平局目录加员、
// 网格05 目录换员——目录 mtime 游标失效）+ 新目录（新群形态）。变更后对变更
// 目录补 os.Chtimes 显式推 mtime（不依赖文件系统对条目增删的 mtime 语义）。
func mutateMixedRound(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, root+"/平局/同键_05.jpg", "x")
	if err := os.Remove(root + "/网格05/网格_060.jpg"); err != nil {
		t.Fatalf("网格05 换员（删旧）失败: %v", err)
	}
	writeTestFile(t, root+"/网格05/网格_061.jpg", "y")
	writeTestFile(t, root+"/新目录/新组_01.jpg", "z1")
	writeTestFile(t, root+"/新目录/新组_02.jpg", "z2")
	writeTestFile(t, root+"/新目录/新组 03.jpg", "z3")
	writeTestFile(t, root+"/新目录/新组 04.jpg", "z4")
	now := time.Now()
	for _, dir := range []string{root + "/平局", root + "/网格05", root + "/新目录"} {
		if err := os.Chtimes(dir, now, now); err != nil {
			t.Fatalf("推目录 mtime 失败（%s）: %v", dir, err)
		}
	}
}

// parallelSnapOf 读取识别完成后的会话并要求快照在场。
func parallelSnapOf(t *testing.T, m *SessionManager, root string) *RecognitionSnapshot {
	t.Helper()
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if sess.Snapshot == nil {
		t.Fatal("识别完成后快照不在场")
	}
	return sess.Snapshot
}

// TestMergedRecognitionParallel 模式等价锚（测试矩阵第 2/5 项）：同输入下
// 1 工作子与多工作子两模式互证——(a) 合并求值直调：逐文件结论 reflect.DeepEqual
// 深度相等；(b) 全量识别干跑：两独立会话分别以两模式识别，快照全等（除生成
// 时间/跳过目录数）且群卡终序三分量全序。共享态（兄弟表/聚合表/结构群成员表/
// 规则序列/谓词索引/会话赋值表）在求值期只读的声明由此锚运行期验证。
func TestMergedRecognitionParallel(t *testing.T) {
	root := t.TempDir()
	assigns := parallelAssigns()
	parallelSampleTree(t, root)
	m, _ := newParallelFixture(t, root, assigns)

	// —— (a) 合并求值直调：同一输入两模式，结论深度相等 ——
	profile, errs := LoadProfile(m.store)
	if len(errs) > 0 {
		t.Fatalf("载入并行锚档案失败: %v", errs)
	}
	enabled := EffectiveRules(profile)
	enumRes, err := NewScanner(root).Enumerate(nil)
	if err != nil {
		t.Fatalf("枚举夹具树失败: %v", err)
	}
	files := buildScanFiles(enumRes.Files)
	if len(files) <= evalFileChunkSize {
		t.Fatalf("夹具形态缺损：文件数 %d 未越过分块阈值 %d（并行区2 不会多块）", len(files), evalFileChunkSize)
	}
	lib := BuildLibraryIndex(m.ctx, m.warnf)
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}

	withEvalWorkers(t, 1)
	serial := MergedRecognitionWithTree(files, files, enabled, lib, sess.Assignments)
	withEvalWorkers(t, parallelWorkersMulti)
	par := MergedRecognitionWithTree(files, files, enabled, lib, sess.Assignments)
	if len(serial) != len(par) {
		t.Fatalf("两模式结论长度不等: 串行 %d vs 并行 %d", len(serial), len(par))
	}
	assertParallelConclusionShape(t, serial)
	for i := range serial {
		if !reflect.DeepEqual(serial[i], par[i]) {
			t.Fatalf("结论 %d（%s）两模式深度不等\n串行: %+v\n并行: %+v", i, serial[i].RelPath, serial[i], par[i])
		}
	}

	// —— (b) 全量识别干跑：两独立会话两模式，快照全等 ——
	withEvalWorkers(t, 1)
	mA, _ := newParallelFixture(t, root, assigns)
	if err := mA.runFullRecognition(root); err != nil {
		t.Fatalf("串行模式全量识别失败: %v", err)
	}
	snapA := parallelSnapOf(t, mA, root)
	if snapA.Stats.SkippedDirs != 0 {
		t.Fatalf("串行模式应为全量识别（零跳过），实际 %d", snapA.Stats.SkippedDirs)
	}
	withEvalWorkers(t, parallelWorkersMulti)
	mB, _ := newParallelFixture(t, root, assigns)
	if err := mB.runFullRecognition(root); err != nil {
		t.Fatalf("并行模式全量识别失败: %v", err)
	}
	snapB := parallelSnapOf(t, mB, root)
	if snapB.Stats.SkippedDirs != 0 {
		t.Fatalf("并行模式应为全量识别（零跳过），实际 %d", snapB.Stats.SkippedDirs)
	}
	assertTieFixtureShape(t, snapA)
	assertTieFixtureShape(t, snapB)
	assertGroupCardsTotallyOrdered(t, snapA)
	assertGroupCardsTotallyOrdered(t, snapB)
	assertSnapshotsCongruent(t, snapB, snapA)
}

// TestCrossModeIncremental 跨模式增量轮锚（测试矩阵第 3/5 项）：串行轮1→并行
// 轮2、并行轮1→串行轮2，轮间施同一混合变更集（部分目录 mtime 变更+新目录）。
// 断言：轮2 为真增量（跳过未变目录、变更目录真实重算）；轮2 快照与「变更后
// 全量重算」（全新会话）快照全等——跨模式复用旧快照增量求值与全量求值等价；
// 群卡较首轮只增不减（新目录成群、旧群不丢，群卡不漂移）。
func TestCrossModeIncremental(t *testing.T) {
	for _, tc := range []struct {
		label                       string
		firstWorkers, secondWorkers int
	}{
		{"串行轮1_并行轮2", 1, parallelWorkersMulti},
		{"并行轮1_串行轮2", parallelWorkersMulti, 1},
	} {
		t.Run(tc.label, func(t *testing.T) {
			root := t.TempDir()
			assigns := parallelAssigns()
			parallelSampleTree(t, root)

			withEvalWorkers(t, tc.firstWorkers)
			m, _ := newParallelFixture(t, root, assigns)
			if err := m.runFullRecognition(root); err != nil {
				t.Fatalf("轮1 全量识别失败: %v", err)
			}
			first := parallelSnapOf(t, m, root)
			if first.Stats.SkippedDirs != 0 {
				t.Fatalf("轮1 应为全量识别（零跳过），实际 %d", first.Stats.SkippedDirs)
			}

			mutateMixedRound(t, root)

			withEvalWorkers(t, tc.secondWorkers)
			if err := m.runFullRecognition(root); err != nil {
				t.Fatalf("轮2 增量识别失败: %v", err)
			}
			second := parallelSnapOf(t, m, root)
			if second.Stats.SkippedDirs == 0 {
				t.Fatal("轮2 应为混合增量轮（跳过未变目录），实际跳过 0 个")
			}
			// 变更集生效：新目录文件在场、换员目录旧名退场（变更目录真实重算）
			hasFile := func(rel string) bool {
				for i := range second.Files {
					if second.Files[i].RelPath == rel {
						return true
					}
				}
				return false
			}
			if !hasFile("新目录/新组_01.jpg") || hasFile("网格05/网格_060.jpg") {
				t.Fatalf("混合轮变更集未生效（新组_01 在场=%t、旧网格_060 在场=%t）",
					hasFile("新目录/新组_01.jpg"), hasFile("网格05/网格_060.jpg"))
			}

			// 跨模式参照：变更后全量重算（全新会话，prev 缺席 → 全量）——轮2
			// 增量结论与全量结论在此全等
			mRef, _ := newParallelFixture(t, root, assigns)
			if err := mRef.runFullRecognition(root); err != nil {
				t.Fatalf("参照全量识别失败: %v", err)
			}
			ref := parallelSnapOf(t, mRef, root)
			assertTieFixtureShape(t, second)
			assertGroupCardsTotallyOrdered(t, second)
			assertGroupCardsTotallyOrdered(t, ref)
			assertSnapshotsCongruent(t, second, ref)
			if len(second.Groups) <= len(first.Groups) {
				t.Fatalf("群卡漂移：混合轮后应只增不减（首轮 %d → 轮2 %d）", len(first.Groups), len(second.Groups))
			}
		})
	}
}
