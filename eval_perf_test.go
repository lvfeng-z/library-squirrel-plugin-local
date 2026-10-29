// 识别求值相并行化（《识别求值相并行化方案》阶段3·性能锚与全量回归）的耗时
// 锚（测试矩阵第 7 项）：TestRecognitionEvalSpeedup 在合成大库（万级文件/数百
// 目录/六类混合形态）上以同一输入对求值相两模式计时——evalWorkerCount 覆写为
// 1（串行） vs 逻辑核数（并行，经 MergedRecognitionWithTree 求值入口直调），
// 门限=逻辑核 ≥4 时加速比 ≥1.5（方案风险2 宽门限，实测画像后再收），不足时
// 如实报失败不放宽门限；逻辑核 <4 的环境 t.Skip（形态覆盖验证仍先行完成）。
// 实测两模式耗时/核数/加速比经 t.Logf 留档（阶段4 转述入文档）。
//
// 计时协议（noise 控制）：两模式各预热一轮不计时（排除首跑分配/调度冷启动），
// 随后 5 轮交错计时（每轮先串行后并行、计时前 runtime.GC() 归一化堆态——上一
// 轮结论垃圾不落入本轮预算），取值=各模式 5 轮耗时的中位数：交错采样令热节流/
// 后台噪声对两模式同向作用，中位数抑制离群轮。
//
// 本锚不进 -race 车道（竞态插桩令墙钟失真，加速比不可判）——阶段3 验证命令以
// 无 -race 跑本锚，竞态锚另由模式等价/跨模式增量轮锚承担。夹具形态借
// parallelSampleTree 的形态思路放大、独立生成：求值相输入契约即 []ScanFile
// （MergedRecognitionWithTree 签名），内存合成避免万级磁盘写入的 I/O 噪声与
// 分钟级 setup——计时只覆盖求值相本身（与 TestMergedRecognitionParallel (a)
// 段同一消费方式）。本文件为并行化车道专属，不与「工作台协议大载荷分页/流式
// 分帧」车道相交。
package main

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// —— 合成库规模常量（万级文件/数百目录/六类形态；调规模先改此处再同步对账）——

const (
	// perfBlockCount 合成块数：每块 5 类目录（定群/结构已识别/组合/谓词/无线索）
	// ——150 块 × 5 目录/块 = 750 目录（数百目录档）。
	perfBlockCount = 150
	// perfRuleGroupPerDir 每块规则定群文件数：下划线形态命中兄弟规则成群
	//（真实库主体形态——序列目录数十成员）。
	perfRuleGroupPerDir = 60
	// perfStructuralPerDir 每块结构群全员已识别文件数：两群 × 3（空格形态
	// 结构对齐成群 + 括号命名惯例规则逐成员提取 author/title）。
	perfStructuralPerDir = 6
	// perfComboPerDir 每块组合规则门段文件数：成员正则 + 纯门组合双登记。
	perfComboPerDir = 4
	// perfPredicatePerDir 每块谓词索引命中文件数：本地/站点作者轨各 2（文件名
	// 去扩展名精确等于库内作者名）。
	perfPredicatePerDir = 4
	// perfCluelessPerDir 每块无线索文件数：无数字/无下划线/无括号/非库内名
	// ——任何规则与结构对齐均不命中（占比 ≈18%，在场由形态断言钉住）。
	perfCluelessPerDir = 16
	// perfSpeedupFloor 加速比门限（逻辑核 ≥4 时生效；方案风险2 宽门限）。
	perfSpeedupFloor = 1.5
	// perfRounds 计时轮数（≥3 取中位；见文件头「计时协议」）。
	perfRounds = 5
)

// perfShape 合成库的形态计数：生成器与形态断言两面对账（期望值由规模常量
// 推导，实际值由结论清点）——夹具任一形态静默退化即报错，防性能锚输入失真。
type perfShape struct {
	dirs, files                                 int
	ruleGrouped, structuralIdentified           int
	combo, predicateHits, assigned, clueless    int
}

// syntheticEvalLibrary 合成大库生成器（eval_perf_test 专属，磁盘零依赖）：按块
// 复制五类目录形态，产出求值相三输入——文件清单（[]ScanFile，treeFiles=files
// 全量语义）、会话赋值表（每块 2 条：规则群首员 author 改写 / 结构群首员 title
// 改写）与形态期望计数。命名约束：无线索文件名不含数字与下划线（数字位分桶与
// 兄弟规则双不命中）；谓词文件名去扩展名精确等于库内作者名（本地/站点两轨同
// 入 AuthorNames，BuildLibraryIndex 拉全后精确匹配）。
func syntheticEvalLibrary() ([]ScanFile, map[string]SessionAssignment, perfShape) {
	perBlock := perfRuleGroupPerDir + perfStructuralPerDir + perfComboPerDir + perfPredicatePerDir + perfCluelessPerDir
	files := make([]ScanFile, 0, perfBlockCount*perBlock)
	assigns := make(map[string]SessionAssignment, 2*perfBlockCount)
	add := func(dir, name string) {
		rel := dir + "/" + name
		files = append(files, ScanFile{Path: rel, FileName: name, DirNames: []string{dir}, RelPath: rel})
	}
	for b := 0; b < perfBlockCount; b++ {
		// ① 规则定群：`画师NNN画集_%03d.jpg` 命中 test-sibling-set 兄弟规则成群
		dir := fmt.Sprintf("定群%03d", b)
		prefix := fmt.Sprintf("画师%03d画集", b)
		for i := 1; i <= perfRuleGroupPerDir; i++ {
			add(dir, fmt.Sprintf("%s_%03d.jpg", prefix, i))
		}
		// 赋值覆盖形态 A：规则群首员 author 改写（档案结论被特异性知识覆盖）
		assigns[dir+"/"+prefix+"_001.jpg"] = SessionAssignment{
			Author: fmt.Sprintf("手工作者%03d", b), UpdatedAtMs: int64(b) + 1,
		}
		// ② 结构群全员已识别：空格形态不入规则定群、结构对齐成群，括号命名
		//    惯例规则对每成员提取 author/title → 全员带 author 终选
		dir = fmt.Sprintf("结构已识别%03d", b)
		for i := 1; i <= perfStructuralPerDir/2; i++ {
			add(dir, fmt.Sprintf("【画师乙】画集 %02d.jpg", i))
		}
		for i := 1; i <= perfStructuralPerDir/2; i++ {
			add(dir, fmt.Sprintf("【画师丙】外传 %02d.jpg", i))
		}
		// 赋值覆盖形态 B：结构群首员 title 改写（未指定维度 author 保留）
		assigns[dir+"/【画师乙】画集 01.jpg"] = SessionAssignment{
			Title: fmt.Sprintf("手工标题%03d", b), UpdatedAtMs: int64(b) + 100001,
		}
		// ③ 组合规则门段：空格形态（另成结构群）命中成员正则 + 纯门组合
		dir = fmt.Sprintf("组合%03d", b)
		for i := 1; i <= perfComboPerDir; i++ {
			add(dir, fmt.Sprintf("门段样板 %02d.jpg", i))
		}
		// ④ 谓词索引命中：文件名去扩展名=库内作者名（test-predicate-author）
		dir = fmt.Sprintf("谓词%03d", b)
		add(dir, "库内作者甲.jpg")
		add(dir, "库内作者乙.jpg")
		add(dir, "站点作者丙.jpg")
		add(dir, "站点作者丁.jpg")
		// ⑤ 无线索：任何规则与结构对齐均不命中（`散图a..p.jpg`，无数字流）
		dir = fmt.Sprintf("无线索%03d", b)
		for i := 0; i < perfCluelessPerDir; i++ {
			add(dir, fmt.Sprintf("散图%c.jpg", 'a'+rune(i)))
		}
	}
	want := perfShape{
		dirs:                 perfBlockCount * 5,
		files:                perfBlockCount * perBlock,
		ruleGrouped:          perfBlockCount * perfRuleGroupPerDir,
		structuralIdentified: perfBlockCount * perfStructuralPerDir,
		combo:                perfBlockCount * perfComboPerDir,
		predicateHits:        perfBlockCount * perfPredicatePerDir,
		assigned:             perfBlockCount * 2,
		clueless:             perfBlockCount * perfCluelessPerDir,
	}
	return files, assigns, want
}

// perfEvalRules 性能锚规则集：recognitionTestRules 六条（兄弟定群/括号惯例/双
// 冲突/谓词作者/目录含义）之上追加组合规则对——成员=文件名正则，组合=纯门
//（refs 单成员、空条件表），形态与 newParallelFixture 的组合样板同构。
func perfEvalRules(t *testing.T) []RuleEntry {
	t.Helper()
	rules := recognitionTestRules(t)
	return append(rules,
		RuleEntry{
			ID: "perf-gate-member", Description: "性能锚：门段成员（文件名正则）", Source: RuleSourceManual, Enabled: true,
			Pattern: mustRegexPattern(t, `^门段样板.+\.jpg$`, nil),
		},
		RuleEntry{
			ID: "perf-gate-combo", Description: "性能锚：纯门组合（refs 单成员）", Source: RuleSourceManual, Enabled: true,
			Refs: []string{"perf-gate-member"}, Conditions: []CondxRow{}, Pattern: And{},
		},
	)
}

// perfLibraryIndex 谓词索引（性能锚形态 ④ 的数据源）：库查询桩五轨拉全——本地
// 与站点作者同入 AuthorNames（BuildLibraryIndex 两轨并表），标签轨一条非命中
// 库容（谓词查空轨的 miss 成本在场）。
func perfLibraryIndex() *LibraryIndex {
	ctx := &workbenchTestCtx{store: newFakeSessionStore()}
	ctx.library = libraryFixture{
		localAuthors: []string{"库内作者甲", "库内作者乙"},
		siteAuthors:  []string{"站点作者丙", "站点作者丁"},
		localTags:    []string{"风景"},
	}
	return BuildLibraryIndex(ctx, nil)
}

// perfDeclHas 终选声明按维度+取值精确探测。
func perfDeclHas(decls []MetadataDeclaration, dim MetadataDimension, value string) bool {
	for _, d := range decls {
		if d.Dimension == dim && d.Value == value {
			return true
		}
	}
	return false
}

// assertPerfShape 六类形态在场断言（结论层清点对账，防夹具静默退化）：规则定群
//（Group 非空且带定群规则 ID）/ 结构群全员已识别（Group 非空且规则 ID 空、每
// 成员带 author 终选）/ 组合规则门段（成员与组合双双进 MatchedRuleIDs）/ 赋值
// 覆盖（终选含赋值标记值）/ 谓词索引命中（test-predicate-author + author 终选
// =库内名）/ 无线索（无群、无终选、无命中规则）。另附两条赋值语义探针（author
// 改写 / title 改写且 author 保留）。形态间允许交叠（赋值文件同时是定群/结构
// 群成员），各计数独立对账。
func assertPerfShape(t *testing.T, conclusions []FileConclusion, want perfShape) {
	t.Helper()
	if len(conclusions) != want.files {
		t.Fatalf("结论数 %d ≠ 夹具文件数 %d", len(conclusions), want.files)
	}
	byRel := make(map[string]FileConclusion, len(conclusions))
	ruleGrouped, structural, combo, predicate, assigned, clueless := 0, 0, 0, 0, 0, 0
	for _, c := range conclusions {
		byRel[c.RelPath] = c
		matched := func(id string) bool {
			for _, got := range c.MatchedRuleIDs {
				if got == id {
					return true
				}
			}
			return false
		}
		if c.Group != nil && c.GroupRuleID != "" {
			ruleGrouped++
		}
		if c.Group != nil && c.GroupRuleID == "" &&
			(perfDeclHas(c.Final, DimAuthor, "画师乙") || perfDeclHas(c.Final, DimAuthor, "画师丙")) {
			structural++
		}
		if matched("perf-gate-member") && matched("perf-gate-combo") {
			combo++
		}
		if matched("test-predicate-author") &&
			(perfDeclHas(c.Final, DimAuthor, "库内作者甲") || perfDeclHas(c.Final, DimAuthor, "库内作者乙") ||
				perfDeclHas(c.Final, DimAuthor, "站点作者丙") || perfDeclHas(c.Final, DimAuthor, "站点作者丁")) {
			predicate++
		}
		for _, d := range c.Final {
			if strings.HasPrefix(d.Value, "手工") {
				assigned++
				break
			}
		}
		if c.Group == nil && len(c.Final) == 0 && len(c.MatchedRuleIDs) == 0 {
			clueless++
		}
	}
	check := func(field string, got, wantN int) {
		t.Helper()
		if got != wantN {
			t.Fatalf("夹具形态缺损：%s = %d，期望 %d（六类形态覆盖面退化，性能锚输入失真）", field, got, wantN)
		}
	}
	check("规则定群成员", ruleGrouped, want.ruleGrouped)
	check("结构群全员已识别成员", structural, want.structuralIdentified)
	check("组合规则门段命中", combo, want.combo)
	check("谓词索引命中", predicate, want.predicateHits)
	check("赋值覆盖文件", assigned, want.assigned)
	check("无线索文件", clueless, want.clueless)
	if c, ok := byRel["定群000/画师000画集_001.jpg"]; !ok || !perfDeclHas(c.Final, DimAuthor, "手工作者000") {
		t.Fatalf("赋值覆盖形态缺损：定群首员应改写 author=手工作者000，实际在场=%t 终选 %+v", ok, c.Final)
	}
	if c, ok := byRel["结构已识别000/【画师乙】画集 01.jpg"]; !ok ||
		!perfDeclHas(c.Final, DimTitle, "手工标题000") || !perfDeclHas(c.Final, DimAuthor, "画师乙") {
		t.Fatalf("赋值按维度覆盖形态缺损：title 改写而 author 保留，实际在场=%t 终选 %+v", ok, c.Final)
	}
}

// perfMedianOf 取耗时切片的中位数（奇数轮，排序后取中位元素；轮次与取值法
// 见文件头「计时协议」）。
func perfMedianOf(ds []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), ds...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

// TestRecognitionEvalSpeedup 大库性能锚（测试矩阵第 7 项）：合成大库同输入两
// 模式计时——1 工作子（串行） vs 逻辑核数（并行），对 MergedRecognitionWithTree
// 求值入口直调（两并行区：前置结构对齐按目录 + 逐文件结论循环按文件分块）。断
// 言门限=逻辑核 ≥4 时串行中位/并行中位 ≥1.5；两模式结论数逐轮守恒（求值完备
// 性守卫）。-short 跳过（常规全量与短跑两不误）。
func TestRecognitionEvalSpeedup(t *testing.T) {
	if testing.Short() {
		t.Skip("性能锚为万级合成库多轮计时（重负载），-short 短跑跳过")
	}
	cores := runtime.GOMAXPROCS(0)

	files, assigns, want := syntheticEvalLibrary()
	if want.files < 10000 {
		t.Fatalf("合成库规模退化：%d 文件（万级门未达，性能锚量级失真）", want.files)
	}
	rules := perfEvalRules(t)
	lib := perfLibraryIndex()

	// —— 形态覆盖验证（任何核数环境先行完成，再谈计时）：六类形态在场 ——
	withEvalWorkers(t, 1)
	warm := MergedRecognitionWithTree(files, files, rules, lib, assigns)
	if len(warm) != len(files) {
		t.Fatalf("串行预热轮结论数 %d ≠ 文件数 %d", len(warm), len(files))
	}
	assertPerfShape(t, warm, want)

	if cores < 4 {
		t.Skipf("逻辑核 %d < 4：加速比门限（≥%.1f，方案风险2 宽门限）不判定；六类形态覆盖验证已于上文通过", cores, perfSpeedupFloor)
	}

	// —— 预热：两模式各一轮不计时（首跑分配/调度冷启动排除）——
	MergedRecognitionWithTree(files, files, rules, lib, assigns) // 串行预热（覆写仍为 1）
	withEvalWorkers(t, cores)
	MergedRecognitionWithTree(files, files, rules, lib, assigns) // 并行预热

	// —— 计时：perfRounds 轮交错（每轮先串行后并行，轮前 GC 归一化堆态）——
	serials := make([]time.Duration, perfRounds)
	parallels := make([]time.Duration, perfRounds)
	for r := 0; r < perfRounds; r++ {
		runtime.GC()
		withEvalWorkers(t, 1)
		start := time.Now()
		out := MergedRecognitionWithTree(files, files, rules, lib, assigns)
		serials[r] = time.Since(start)
		if len(out) != len(files) {
			t.Fatalf("串行轮 %d 结论数 %d ≠ 文件数 %d", r, len(out), len(files))
		}

		runtime.GC()
		withEvalWorkers(t, cores)
		start = time.Now()
		out = MergedRecognitionWithTree(files, files, rules, lib, assigns)
		parallels[r] = time.Since(start)
		if len(out) != len(files) {
			t.Fatalf("并行轮 %d 结论数 %d ≠ 文件数 %d", r, len(out), len(files))
		}
	}
	serialMedian, parallelMedian := perfMedianOf(serials), perfMedianOf(parallels)
	speedup := float64(serialMedian) / float64(parallelMedian)

	// —— 留档（阶段4 转述入文档）：库规模与形态/核数/两模式各轮与中位/加速比 ——
	t.Logf("合成大库：%d 块 × 5 目录 = %d 目录 / %d 文件（规则定群 %d · 结构群全员已识别 %d · 组合规则门段 %d · 谓词索引命中 %d · 赋值覆盖 %d · 无线索 %d，无线索占比 %.1f%%）",
		perfBlockCount, want.dirs, want.files, want.ruleGrouped, want.structuralIdentified,
		want.combo, want.predicateHits, want.assigned, want.clueless,
		100*float64(want.clueless)/float64(want.files))
	t.Logf("环境：GOMAXPROCS=%d（逻辑核，并行模式工作子数）", cores)
	t.Logf("串行（1 工作子）各轮耗时：%v；中位 %v", serials, serialMedian)
	t.Logf("并行（%d 工作子）各轮耗时：%v；中位 %v", cores, parallels, parallelMedian)
	t.Logf("求值相加速比 = %.2f（串行中位 / 并行中位；门限 ≥%.1f，方案风险2 宽门限）", speedup, perfSpeedupFloor)

	if speedup < perfSpeedupFloor {
		t.Fatalf("求值相加速比 %.2f 低于门限 %.2f（逻辑核 %d）——如实报失败不放宽门限；串行中位 %v / 并行中位 %v，各轮 串行=%v 并行=%v",
			speedup, perfSpeedupFloor, cores, serialMedian, parallelMedian, serials, parallels)
	}
}
