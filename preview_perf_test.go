// 预览面大库性能方案（阶段1·基线锚）：TestPreviewPathBaseline 在 20 万文件级
// 合成库上对四条预览通路与「赋值变更自动重识别」共五项分别计时留档，产出
// 成本占比数据——后续五个性能阶段（清单缓存/求值复用/闸门细化等）的对照基线。
//
// 五个计时对象（各路真实公开入口直调，不重写内部逻辑）：
//  1. 弹窗预览 PreviewRule（rules_preview.go:70 → previewRuleConclusions:139，
//     全库重建清单+档案∪草稿全量合并求值 rules_preview.go:181-190）；
//  2. 结构建议卡 ApplyStructSummary（rule_summarizer.go:783 → previewStructRules
//     rule_summarizer.go:885，全库×草稿规则串行求值 :888-905——五路中唯一串行面）；
//  3. 序列卡 ApplyStructSummary 序列形态分支（rule_summarizer.go:826 →
//     previewSequenceFormRules rule_summarizer_seq.go:414，全库重建清单+以草稿
//     规则集合并求值 :418-423）；
//  4. 集规则卡 ApplyWorkSetRuleSummary（workset_rule.go:413 → previewWorkSetRules
//     workset_rule.go:1793，全库重建清单+以草稿规则集合并求值 :1797-1802）；
//  5. 赋值变更自动重识别：SetAssignment 写赋值（session.go:1208，修订号自增
//     session.go:1227）令增量闸门失效（recognition.go:2432-2434 三条件之修订号
//     相等），随后计时 runFullRecognition（recognition.go:2388——自动刷新链
//     startRecognitionAuto 的 goroutine 所执行的同体干跑主体，调度入口见
//     parse_actions.go:986-1018）。异步调度外壳（goroutine 派发与在途补启动
//     轮询）不计时：确定性计时须同步执行主体，且外壳成本为微秒级——这是本锚
//     对「真实调用路径」的最小复刻偏离，如实留档。
//
// 计时协议（仿 eval_perf_test.go 文件头）：五路各预热一轮不计时（排除首跑
// 分配/调度冷启动），随后 5 轮交错计时（每轮按 1→5 顺序各执行一次，路前
// runtime.GC() 归一化堆态——上一路结论垃圾不落入下一路预算），取值=各路 5 轮
// 耗时的中位数（perfMedianOf 复用）：交错采样令热节流/后台噪声对五路同向作用，
// 中位数抑制离群轮。子计时（清单重建/名单构建/求值分段）经公开入口+时间戳
// 包围不可分段，本锚整体计时（后续优化阶段如需分段再行专门埋点）。
//
// 门控：20 万级磁盘夹具+多轮全量识别为分钟级重负载，较 eval_perf_test.go
// （1.35 万纯内存夹具、-short 跳过）重一个数量级——按阶段方案以环境变量
// PREVIEW_PERF=1 显式启用，缺省 t.Skip（常规 go test . 零副作用）。
//
// 夹具：真实磁盘树（t.TempDir()——识别干跑以磁盘枚举为输入，scanner.go:148；
// 弹窗/序列/集卡预览只消费快照，不触盘）500 块 × 5 目录 = 2500 目录 /
// 200,000 文件，五形态混合（下划线序号/括号序列/括号集/结构双下划线/无线索），
// 各卡样本与集确认目标均落在块 0；档案一条模板规则 + 弹窗草稿一条（预览时
// 并集求值），序列/结构/集卡规则由各入口现场推导（与生产同构）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// —— 合成库规模常量（20 万文件级；调规模先改此处再同步对账）——

const (
	// previewPerfBlocks 合成块数：每块 5 类目录（定群/序列/合集/结构/无线索）。
	previewPerfBlocks = 500
	// previewPerfSeqPerDir 定群目录文件数：`画师NNN画集_NNNN.jpg` 下划线序号
	// 形态——档案模板规则与弹窗草稿的命中主体（15 万，占 75%）。
	previewPerfSeqPerDir = 300
	// previewPerfBracketSeqPerDir 序列目录文件数：`[作者]_[作品号]_月华_CCC_[NN].png`
	// 括号序号形态——序列卡样本所在结构群。
	previewPerfBracketSeqPerDir = 4
	// previewPerfBracketSetPerDir 合集目录文件数：`[作者]_[作品号]_画集AAA_BBB_[NN].png`
	// ——集规则卡确认目标所在结构群。
	previewPerfBracketSetPerDir = 3
	// previewPerfStructPerDir 结构目录文件数：`atago__x_aXY.png` 双下划线
	// 形态——结构建议卡样本（作者=一级目录剥壳/作品名=文件名首段）。
	previewPerfStructPerDir = 20
	// previewPerfCluelessPerDir 无线索目录文件数：`散图aX.jpg` 无数字/无
	// 下划线/无括号——任何规则与结构对齐均不命中。
	previewPerfCluelessPerDir = 73
	// previewPerfRounds 计时轮数（奇数取中位；见文件头「计时协议」）。
	previewPerfRounds = 5
)

// previewPerfDraftJSON 弹窗预览草稿（与 rules_preview_test.go 草稿同构）：文件
// 名模板对**去扩展名基名**求值（rule_condition_subject.go:15，模板不写扩展名），
// 命中全部定群序号文件（15 万），提取动作=作品名常量。
const previewPerfDraftJSON = `{"id":"pv-draft-seq","description":"性能锚草稿：定群序列文件","source":"manual","enabled":true,` +
	`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{任意}画集_{任意}"}}}],` +
	`"extractions":[{"dimension":"title","from":"constant","constant":"序列作品"}],"synthesis":{}}`

// previewPerfAssignTargets 赋值重识别轮的写入目标（第 0 个供预热轮）：无线索
// 目录内逐轮换文件（每轮一次真实赋值写=修订号自增，session.go:1225-1227）。
var previewPerfAssignTargets = []string{
	"无线索000/散图za.jpg", "无线索000/散图zb.jpg", "无线索000/散图zc.jpg",
	"无线索000/散图zd.jpg", "无线索000/散图ze.jpg", "无线索000/散图zf.jpg",
}

// previewPerfShape 合成库形态计数：生成器与守卫两面对账（防夹具静默退化）。
type previewPerfShape struct {
	dirs, files                        int
	seqGrouped, bracketSeq, bracketSet int
	structural, clueless               int
}

// previewPerfTwoLetters 序号无关的两字母后缀（结构/无线线索文件的确定性变位，
// 无数字流——不参与任何序列成群）。
func previewPerfTwoLetters(i int) string {
	return string(rune('a'+i/26)) + string(rune('a'+i%26))
}

// previewPerfBuildTree 落盘合成库（每目录一次 MkdirAll + 逐文件一字节写，
// 20 万小文件为分钟级 setup，由 PREVIEW_PERF 门控兜底）。
func previewPerfBuildTree(t *testing.T, root string) previewPerfShape {
	t.Helper()
	writeDir := func(rel string, names []string) {
		t.Helper()
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("创建目录 %s 失败: %v", rel, err)
		}
		for _, name := range names {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
				t.Fatalf("写入 %s/%s 失败: %v", rel, name, err)
			}
		}
	}
	names := make([]string, 0, previewPerfSeqPerDir)
	fill := func(dir string, count int, build func(i int) string) {
		names = names[:0]
		for i := 1; i <= count; i++ {
			names = append(names, build(i))
		}
		writeDir(dir, names)
	}
	for b := 0; b < previewPerfBlocks; b++ {
		// ① 定群序号：`画师NNN画集_NNNN.jpg`（每目录一个 300 员结构序列群）
		fill(fmt.Sprintf("定群%03d", b), previewPerfSeqPerDir,
			func(i int) string { return fmt.Sprintf("画师%03d画集_%04d.jpg", b, i) })
		// ② 括号序列：`[作者]_[作品号]_月华_CCC_[NN].png`（序列卡样本群）
		fill(fmt.Sprintf("序列%03d", b), previewPerfBracketSeqPerDir,
			func(i int) string { return fmt.Sprintf("[画师%03d]_[%09d]_月华_CCC_[%02d].png", b, 100000000+b, i) })
		// ③ 括号集：`[作者]_[作品号]_画集AAA_BBB_[NN].png`（集卡确认目标群）
		fill(fmt.Sprintf("合集%03d", b), previewPerfBracketSetPerDir,
			func(i int) string {
				return fmt.Sprintf("[画师%03d]_[%09d]_画集AAA_BBB_[%02d].png", b, 200000000+b, i)
			})
		// ④ 结构双下划线：`atago__x_aXY.png`（结构卡样本目录，作者=目录壳）
		fill(fmt.Sprintf("Artist[画师%03d]", b), previewPerfStructPerDir,
			func(i int) string { return "atago__x_" + previewPerfTwoLetters(i) + ".png" })
		// ⑤ 无线索：`散图aX.jpg`（任何规则与结构对齐均不命中）
		fill(fmt.Sprintf("无线索%03d", b), previewPerfCluelessPerDir,
			func(i int) string { return "散图" + previewPerfTwoLetters(i) + ".jpg" })
	}
	perBlock := previewPerfSeqPerDir + previewPerfBracketSeqPerDir + previewPerfBracketSetPerDir +
		previewPerfStructPerDir + previewPerfCluelessPerDir
	return previewPerfShape{
		dirs:       previewPerfBlocks * 5,
		files:      previewPerfBlocks * perBlock,
		seqGrouped: previewPerfBlocks * previewPerfSeqPerDir,
		bracketSeq: previewPerfBlocks * previewPerfBracketSeqPerDir,
		bracketSet: previewPerfBlocks * previewPerfBracketSetPerDir,
		structural: previewPerfBlocks * previewPerfStructPerDir,
		clueless:   previewPerfBlocks * previewPerfCluelessPerDir,
	}
}

// previewPerfGroupTarget 在快照候选主清单内找指定目录的结构群候选目标键
// （workset_rule_test.go confirmGroupCandidate 同款寻址）。
func previewPerfGroupTarget(t *testing.T, m *SessionManager, root, dir string) string {
	t.Helper()
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	for i := range sess.Snapshot.Candidates {
		c := &sess.Snapshot.Candidates[i]
		if c.Source == candSourceGroup && c.Dir == dir {
			return c.target()
		}
	}
	t.Fatalf("候选主清单内无 %s 的群候选（夹具形态退化）", dir)
	return ""
}

// previewPerfWaitRecognitionIdle 等待确认动作触发的异步识别轮完成（在途标记
// 清零；20 万级库一轮为十秒级，等待上限放宽到 10 分钟）。
func previewPerfWaitRecognitionIdle(t *testing.T, m *SessionManager, root string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Minute)
	for {
		m.recogMu.Lock()
		idle := !m.recogRunning[root]
		m.recogMu.Unlock()
		if idle {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("确认归集触发的识别刷新超时未完成（已等 10 分钟）")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// previewPerfFixture 性能锚环境：磁盘树 + 档案（一条模板规则）+ 会话 + 首轮
// 全量识别 + 两卡样本赋值（SetAssignment 不排异步轮——赋值写真实通路、去掉
// 调度外壳）+ 合集群经真实 ConfirmWorkSetCandidate 确认归集（等待其触发的
// 异步识别完成，快照 confirmed 分仓就位）。
func previewPerfFixture(t *testing.T, root string) (*SessionManager, previewPerfShape, string) {
	t.Helper()
	shape := previewPerfBuildTree(t, root)

	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	ctx.library = libraryFixture{
		localAuthors: []string{"库内作者甲"},
		localTags:    []string{"风景"},
	}
	m := NewSessionManagerWithStore(ctx, nil, store)

	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{任意}画集_{任意}"}, // 去扩展名基名求值
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("构造档案模板规则失败: %v", err)
	}
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{{
		ID: "pv-seq-member", Description: "性能锚：定群序列成员（文件名模板）",
		Source: RuleSourceManual, Enabled: true, Conditions: rows, Pattern: pattern,
	}}}); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}

	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	t.Logf("夹具落盘完成（%d 目录 / %d 文件），首轮全量识别……", shape.dirs, shape.files)
	setupStart := time.Now()
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	t.Logf("首轮全量识别耗时 %v（setup，不计入基线）", time.Since(setupStart))

	// 结构卡样本赋值：作者=一级目录剥壳（Artist[画师000]→画师000）、作品名=
	// 文件名首段——与 rule_summarizer_test.go 结构卡用例同构（真实写通路，
	// 不排异步轮）。
	if _, err := m.SetAssignment(root, "Artist[画师000]/atago__x_ab.png", SessionAssignment{
		Author: "画师000", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
	}); err != nil {
		t.Fatalf("写入结构卡样本赋值失败: %v", err)
	}
	// 序列卡样本赋值：作品集=群键中段组合（月华_CCC，跨段含分隔符——用户
	// 实例形态，rule_summarizer_seq_test.go 同款）。
	seqSample := fmt.Sprintf("序列000/[画师000]_[%09d]_月华_CCC_[01].png", 100000000)
	if _, err := m.SetAssignment(root, seqSample, SessionAssignment{Collection: "月华_CCC"}); err != nil {
		t.Fatalf("写入序列卡样本赋值失败: %v", err)
	}

	// 集规则卡确认目标：合集000 的结构群候选经真实确认动作归集（赋值写+
	// 修订号自增+自动排识别轮），等待异步轮完成。
	target := previewPerfGroupTarget(t, m, root, "合集000")
	if _, _, err := m.ConfirmWorkSetCandidate(root, target, "画集AAA_BBB", 0); err != nil {
		t.Fatalf("确认归集失败: %v", err)
	}
	previewPerfWaitRecognitionIdle(t, m, root)
	return m, shape, target
}

// TestPreviewPathBaseline 预览面大库性能基线锚：五路各预热 1 轮 + 5 轮交错
// 计时取中位，形态计数与耗时经 t.Logf 留档（供方案「基线记录」节回填）。
// 五路计数逐轮守恒守卫（计时对象每轮干了等量的真活），分支守卫钉住各路
// 走的是目标通路（序列分支看摘要、结构卡看推导规则在场）。
func TestPreviewPathBaseline(t *testing.T) {
	if os.Getenv("PREVIEW_PERF") != "1" {
		t.Skip("预览面大库性能锚为 20 万文件级磁盘夹具+多轮全量识别（分钟级重负载），缺省跳过；启用：PREVIEW_PERF=1 go test -run TestPreviewPathBaseline -v . -timeout 30m")
	}

	root := t.TempDir()
	m, shape, setTarget := previewPerfFixture(t, root)

	// 夹具守卫：快照文件数对账（夹具任一形态静默退化即报错）。
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if got := len(sess.Snapshot.Files); got != shape.files {
		t.Fatalf("快照文件数 %d ≠ 夹具文件数 %d（夹具形态退化，性能锚输入失真）", got, shape.files)
	}

	const (
		structSample = "Artist[画师000]/atago__x_ab.png"
		seqSample    = "序列000/[画师000]_[100000000]_月华_CCC_[01].png"
	)

	// pathStats 一轮各路形态计数（守恒守卫与留档的数据面）。
	type pathStats struct {
		previewH, previewT int    // 弹窗预览：命中/总数
		structH, structT   int    // 结构卡：命中/总数（除样本外被组合规则完整覆盖）
		structRules        int    // 结构卡推导规则条数（>0=previewStructRules 真跑）
		seqH, seqT         int    // 序列卡：命中/总数
		seqSummary         string // 序列卡摘要（分支守卫：含「同形序列成员」）
		setH, setT         int    // 集规则卡：命中/总数
		snapFiles          int    // 赋值重识别轮完成后的快照文件数
	}
	// pathRun 一轮交错计时的产物：五路耗时 + 形态计数（守恒守卫只比 stats
	// ——耗时天然逐轮浮动，不参与计数对账）。
	type pathRun struct {
		durations [5]time.Duration
		stats     pathStats
	}
	routeLabels := [5]string{
		"弹窗预览 PreviewRule",
		"结构建议卡 ApplyStructSummary（串行面）",
		"序列卡 ApplyStructSummary 序列分支",
		"集规则卡 ApplyWorkSetRuleSummary",
		"赋值重识别 runFullRecognition（闸门失效整库重算）",
	}
	runAll := func(assignRel string) pathRun {
		var out pathRun
		// 1. 弹窗预览（rules_preview.go:181-190）
		runtime.GC()
		start := time.Now()
		pv, err := m.PreviewRule(root, []byte(previewPerfDraftJSON))
		out.durations[0] = time.Since(start)
		if err != nil {
			t.Fatalf("弹窗预览失败: %v", err)
		}
		out.stats.previewH, out.stats.previewT = pv.HitCount, pv.HitTotal
		// 2. 结构建议卡（rule_summarizer.go:885-912 串行全库求值）
		runtime.GC()
		start = time.Now()
		st, err := m.ApplyStructSummary(root, structSample)
		out.durations[1] = time.Since(start)
		if err != nil {
			t.Fatalf("结构建议卡失败: %v", err)
		}
		out.stats.structH, out.stats.structT, out.stats.structRules = st.HitCount, st.HitTotal, len(st.Rules)
		// 3. 序列卡（rule_summarizer_seq.go:414-423，样本/赋值决定走序列分支）
		runtime.GC()
		start = time.Now()
		sq, err := m.ApplyStructSummary(root, seqSample)
		out.durations[2] = time.Since(start)
		if err != nil {
			t.Fatalf("序列卡失败: %v", err)
		}
		out.stats.seqH, out.stats.seqT, out.stats.seqSummary = sq.HitCount, sq.HitTotal, sq.Summary
		// 4. 集规则卡（workset_rule.go:1793-1802）
		runtime.GC()
		start = time.Now()
		ws, err := m.ApplyWorkSetRuleSummary(root, setTarget)
		out.durations[3] = time.Since(start)
		if err != nil {
			t.Fatalf("集规则卡失败: %v", err)
		}
		if ws.Failed {
			t.Fatalf("集规则卡推导失败: %s", ws.FailReason)
		}
		out.stats.setH, out.stats.setT = ws.HitCount, ws.HitTotal
		// 5. 赋值变更自动重识别：写赋值（修订号自增=增量闸门失效，不计入
		//    计时——写为微秒级触发件），计时对象=闸门失效后的整库重算干跑。
		if _, err := m.SetAssignment(root, assignRel, SessionAssignment{Author: "改档作者"}); err != nil {
			t.Fatalf("写入赋值失败: %v", err)
		}
		sess, err := m.SessionOf(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		if sess.AssignRevision != sess.Snapshot.AssignmentRevision+1 {
			t.Fatalf("增量闸门失效前置缺损：会话修订号 %d ≠ 快照修订号 %d+1（本轮将走 mtime 增量而非整库重算，计时对象失真——recognition.go:2432-2434）",
				sess.AssignRevision, sess.Snapshot.AssignmentRevision)
		}
		runtime.GC()
		start = time.Now()
		if err := m.runFullRecognition(root); err != nil {
			t.Fatalf("赋值重识别失败: %v", err)
		}
		out.durations[4] = time.Since(start)
		sess, err = m.SessionOf(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		out.stats.snapFiles = len(sess.Snapshot.Files)
		return out
	}

	// —— 预热轮（不计时）+ 分支/形态守卫 ——
	warm := runAll(previewPerfAssignTargets[0])
	if warm.stats.previewT != shape.files || warm.stats.previewH != shape.seqGrouped {
		t.Fatalf("弹窗预览形态计数失真：命中 %d/总数 %d，期望 %d/%d（模板语义或夹具形态偏离）",
			warm.stats.previewH, warm.stats.previewT, shape.seqGrouped, shape.files)
	}
	if warm.stats.structT != shape.files || warm.stats.structH <= 0 || warm.stats.structRules == 0 {
		t.Fatalf("结构建议卡形态计数失真：命中 %d/总数 %d/推导规则 %d 条（期望 总数 %d、命中>0）",
			warm.stats.structH, warm.stats.structT, warm.stats.structRules, shape.files)
	}
	if !strings.Contains(warm.stats.seqSummary, "同形序列成员") {
		t.Fatalf("序列卡样本未走序列形态分支（摘要 %q）——计时对象失真", warm.stats.seqSummary)
	}
	if warm.stats.seqT != shape.files || warm.stats.seqH <= 0 {
		t.Fatalf("序列卡形态计数失真：命中 %d/总数 %d（期望 总数 %d、命中>0）", warm.stats.seqH, warm.stats.seqT, shape.files)
	}
	if warm.stats.setT != shape.files || warm.stats.setH <= 0 {
		t.Fatalf("集规则卡形态计数失真：命中 %d/总数 %d（期望 总数 %d、命中>0）", warm.stats.setH, warm.stats.setT, shape.files)
	}
	if warm.stats.snapFiles != shape.files {
		t.Fatalf("赋值重识别后快照文件数 %d ≠ %d", warm.stats.snapFiles, shape.files)
	}

	// —— 计时：previewPerfRounds 轮交错，计数逐轮守恒 ——
	runs := make([]pathRun, previewPerfRounds)
	for r := range runs {
		runs[r] = runAll(previewPerfAssignTargets[r+1])
		if runs[r].stats != warm.stats {
			t.Fatalf("第 %d 轮五路形态计数与预热轮不符（计时对象每轮须干等量真活）：预热 %+v / 本轮 %+v", r, warm.stats, runs[r].stats)
		}
	}

	// —— 留档（回填方案「基线记录」节）——
	var medians [5]time.Duration
	for p := range routeLabels {
		ds := make([]time.Duration, previewPerfRounds)
		for r := range runs {
			ds[r] = runs[r].durations[p]
		}
		medians[p] = perfMedianOf(ds)
		t.Logf("%s：各轮 %v；中位 %v", routeLabels[p], ds, medians[p])
	}
	var total time.Duration
	for _, d := range medians {
		total += d
	}
	t.Logf("== 基线锚数据块（回填方案「基线记录」节）==")
	t.Logf("夹具规模：%d 块 × 5 目录 = %d 目录 / %d 文件｜规则：档案 1 条（模板）+ 弹窗草稿 1 条；结构/序列/集卡规则由各入口现场推导（结构 %d 条/序列与集各 2 条）｜GOMAXPROCS=%d",
		previewPerfBlocks, shape.dirs, shape.files, warm.stats.structRules, runtime.GOMAXPROCS(0))
	t.Logf("形态计数：定群序号 %d · 括号序列 %d · 括号集 %d · 结构双下划线 %d · 无线索 %d（合计 %d，无线索占比 %.1f%%）",
		shape.seqGrouped, shape.bracketSeq, shape.bracketSet, shape.structural, shape.clueless,
		shape.files, 100*float64(shape.clueless)/float64(shape.files))
	t.Logf("计时协议：每路预热 1 轮 + %d 轮交错计时取中位（路前 GC；子计时经公开入口不可分段，整体计时）", previewPerfRounds)
	t.Logf("五路中位与成本占比（占五路中位合计）：%s %v（%.1f%%）· %s %v（%.1f%%）· %s %v（%.1f%%）· %s %v（%.1f%%）· %s %v（%.1f%%）｜合计 %v",
		routeLabels[0], medians[0], 100*float64(medians[0])/float64(total),
		routeLabels[1], medians[1], 100*float64(medians[1])/float64(total),
		routeLabels[2], medians[2], 100*float64(medians[2])/float64(total),
		routeLabels[3], medians[3], 100*float64(medians[3])/float64(total),
		routeLabels[4], medians[4], 100*float64(medians[4])/float64(total), total)
	t.Logf("各路形态计数（证明真活）：弹窗 命中/总数=%d/%d；结构卡 %d/%d；序列卡 %d/%d；集卡 %d/%d；赋值重识别后快照文件数=%d",
		warm.stats.previewH, warm.stats.previewT, warm.stats.structH, warm.stats.structT, warm.stats.seqH, warm.stats.seqT,
		warm.stats.setH, warm.stats.setT, warm.stats.snapFiles)
}
