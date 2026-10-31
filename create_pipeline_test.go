// 大选择集创建链（阶段4）单元测试：乙1 verify 并行化+爬坡控制器（决策1 三层：
// 会话内自适应/按盘记忆/偏好钉死）、乙3 发射流水线化与双计数进度、甲1 进度首
// 反馈与时间节流、甲3/决策3 取消通路、甲4 verify 期心跳、风险5/6/7/10 锚定与
// 阶段3 磁盘对账语义在并行/取消路径下的等价保持。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// —— 合成直驱夹具（多目录多作品、空链含义=纯 local 批次）——

// stepClock 可步进时钟（进度节流与爬坡窗口的注入面）。
type stepClock struct {
	mu   sync.Mutex
	t    time.Time
	step time.Duration // 每次读取前进的步长（0=冻结）
}

func (c *stepClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(c.step)
	return c.t
}

func (c *stepClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// emitDirectCfg 直驱 emitSelectedWorks 的夹具配置（零值=生产默认行为面）。
type emitDirectCfg struct {
	pinned     int                // 偏好钉死并发（0=自动轨）
	batchCount int                // 单批条数界（0=生产 2 万；小值驱动多批流水线）
	clockStep  time.Duration      // 进度时钟步长（缺省 0=冻结时钟——中间值全被节流）
	fileBytes  int                // 每文件内容字节数（缺省=小内容；心跳测试放大）
	reporter   sdkdto.HeartbeatReporter
}

// newParallelEmitFixture 合成多目录作品环境（dirs 目录 × 每目录 files 个文件，
// 域序=目录迭代序）：works 按入序即域序，会话快照空（无含义链——纯 local 批次，
// 归属走文件名形态裁决），返回直驱所需的全部句柄。
func newParallelEmitFixture(t *testing.T, dirs, files int, cfg emitDirectCfg) (string, *LocalImportWorkFetcher, *SessionManager, *workbenchTestCtx, *WorkbenchSession, []*SelectableWork) {
	t.Helper()
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	if cfg.pinned > 0 {
		ctx.prefs = map[string]*sdkdto.PreferenceValue{
			createHashConcurrencyPrefKey: {Data: strconv.Itoa(cfg.pinned)},
		}
	}
	m := NewSessionManagerWithStore(ctx, nil, store)
	content := "parallel-fixture-content"
	if cfg.fileBytes > 0 {
		content = strings.Repeat("b", cfg.fileBytes)
	}
	var works []*SelectableWork
	for d := 0; d < dirs; d++ {
		dirKey := fmt.Sprintf("目录%02d", d)
		rel := make([]string, 0, files)
		for f := 0; f < files; f++ {
			name := fmt.Sprintf("文件%02d.jpg", f)
			writeTestFile(t, root+"/"+dirKey+"/"+name, content)
			rel = append(rel, dirKey+"/"+name)
		}
		works = append(works, &SelectableWork{
			WorkID: "d:" + dirKey, Kind: WorkKindDirTracks, DirRelKey: dirKey, Files: rel,
		})
	}
	clock := &stepClock{t: time.Now(), step: cfg.clockStep}
	h := &LocalImportWorkFetcher{
		sessions: m, ctx: ctx,
		hostContractFn: func() uint32 { return createChildrenBatchMinHostContract },
		progressNow:    clock.now,
	}
	if cfg.batchCount > 0 {
		h.childrenBatchCountLimit = cfg.batchCount
	}
	return root, h, m, ctx, &WorkbenchSession{Snapshot: &RecognitionSnapshot{}}, works
}

// runEmitDirect 直驱 emitSelectedWorks 并收卷全部发射条目（同步收尾）。
func runEmitDirect(t *testing.T, h *LocalImportWorkFetcher, root string, sess *WorkbenchSession, works []*SelectableWork, reporter sdkdto.HeartbeatReporter) ([]*sdkdto.CreateStreamItem, string, bool) {
	t.Helper()
	ch := make(chan *sdkdto.CreateStreamItem, 64)
	var items []*sdkdto.CreateStreamItem
	var reason string
	var cancelled bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reason, cancelled = h.emitSelectedWorks(root, sess, works, "直驱批次", ch, reporter)
		close(ch)
	}()
	for item := range ch {
		items = append(items, item)
	}
	wg.Wait()
	return items, reason, cancelled
}

// itemFingerprint 发射条目序列的逐字节指纹（父块取内联子行 JSON——父身份含
// 毫秒时间戳跨运行必不同，不参与比对；批次块取子行 JSON+last 位；库根绝对路径
// 归一为 ROOT 占位——跨夹具仅根不同，相对结构与哈希应逐字节全等）。
func itemFingerprint(t *testing.T, root string, items []*sdkdto.CreateStreamItem) []string {
	t.Helper()
	normalize := func(s string) string {
		esc1 := strings.ReplaceAll(root, `\`, `\\`) // JSON 一层转义（url 字段）
		esc2 := strings.ReplaceAll(esc1, `\`, `\\`) // 二层转义（pluginData 内嵌 JSON 字段）
		s = strings.ReplaceAll(s, esc2, "ROOT")
		return strings.ReplaceAll(s, esc1, "ROOT")
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		if task := it.Task(); task != nil {
			b, err := json.Marshal(task.Children)
			if err != nil {
				t.Fatalf("序列化父块子行失败: %v", err)
			}
			out = append(out, "T:"+normalize(string(b)))
			continue
		}
		if batch := it.ChildrenBatch(); batch != nil {
			b, err := json.Marshal(struct {
				Items []*sdkdto.TaskCreateChildResponse `json:"i"`
				Last  bool                             `json:"l"`
			}{batch.Items, batch.Last})
			if err != nil {
				t.Fatalf("序列化批次块失败: %v", err)
			}
			out = append(out, "B:"+normalize(string(b)))
		}
	}
	return out
}

// createProgressEvents 按发布序抽取 createProgress 事件。
func createProgressEvents(t *testing.T, ctx *workbenchTestCtx) []workbenchFrontendEvent {
	t.Helper()
	var out []workbenchFrontendEvent
	for _, evt := range collectWorkbenchEvents(t, ctx) {
		if evt.Kind == workbenchEventCreateProgress {
			out = append(out, evt)
		}
	}
	return out
}

// phaseEvents 按发布序抽取指定相的 createProgress 事件。
func phaseEvents(t *testing.T, ctx *workbenchTestCtx, phase string) []workbenchFrontendEvent {
	t.Helper()
	var out []workbenchFrontendEvent
	for _, evt := range createProgressEvents(t, ctx) {
		if evt.Phase == phase {
			out = append(out, evt)
		}
	}
	return out
}

// 退出标准①：并行输出=串行基准逐字节全等锚（乙1 保序不变量——重排缓冲按域序
// 逐件放行，并发度只影响速度不影响子行序；偏好钉死 1=串行基准、8=并行满档）。
func TestParallelEmitByteIdenticalToSerialBaseline(t *testing.T) {
	const dirs, files = 12, 2
	serialRoot, serialH, _, _, serialSess, works := newParallelEmitFixture(t, dirs, files, emitDirectCfg{pinned: 1, batchCount: 3})
	serialWorks := append([]*SelectableWork(nil), works...)
	parallelRoot, parallelH, _, _, parallelSess, parallelWorks := newParallelEmitFixture(t, dirs, files, emitDirectCfg{pinned: 8, batchCount: 3})

	serialItems, reason, cancelled := runEmitDirect(t, serialH, serialRoot, serialSess, serialWorks, sdkdto.NoopHeartbeat{})
	if reason != "" || cancelled {
		t.Fatalf("串行基准不应中止: reason=%s cancelled=%v", reason, cancelled)
	}
	parallelItems, reason, cancelled := runEmitDirect(t, parallelH, parallelRoot, parallelSess, parallelWorks, sdkdto.NoopHeartbeat{})
	if reason != "" || cancelled {
		t.Fatalf("并行满档不应中止: reason=%s cancelled=%v", reason, cancelled)
	}

	want, got := itemFingerprint(t, serialRoot, serialItems), itemFingerprint(t, parallelRoot, parallelItems)
	if len(want) != len(got) {
		t.Fatalf("并行与串行条目数应全等（%d vs %d）", len(want), len(got))
	}
	for i := range want {
		if want[i] != got[i] {
			t.Fatalf("第 %d 条目并行与串行逐字节不等:\n串行 %s\n并行 %s", i, want[i], got[i])
		}
	}
	// 确证批界真实生效（非单批全内联的平凡全等）：多批次且末批唯一 last
	batches := emittedBatches(parallelItems)
	if len(batches) < 2 {
		t.Fatalf("小批界注入应产生多批次（实得 %d 批），平凡单批形态不构成并行锚定", len(batches))
	}
	lastCount := 0
	for _, b := range batches {
		if b.Last {
			lastCount++
		}
	}
	if lastCount != 1 || !batches[len(batches)-1].Last {
		t.Fatalf("末批 last 应唯一且居末（last 计数 %d）", lastCount)
	}
}

// 退出标准②：爬坡控制器决策表（决策1 会话内自适应）——注入吞吐序列覆盖升
//（+2 上限 8）/降（跌超 20% 减半）/定格（降后不再升）/缓存命中零字节（风险5：
// 缓存命中不产生读 IO 不计入窗口，虚高吞吐不得误升档）与速率 seam（窗口字节/秒
// 与裁决同源）。
func TestHashThrottleDecisionTable(t *testing.T) {
	t.Run("升档+2 上限8、跌超20%减半定格、定格后不再升", func(t *testing.T) {
		clock := &stepClock{t: time.Now()}
		var adjusts []int
		var remembered []int
		th := newHashThrottle(hashThrottleConfig{initial: 4, now: clock.now,
			adjust: func(n int) { adjusts = append(adjusts, n) },
			remember: func(n int) { remembered = append(remembered, n) }})
		// 每窗投喂一次真实读盘字节（时钟步进一个窗长——窗界即投喂边界；裁决在
		// 下一窗滚动时对该窗执行，即断言滞后一拍）
		feed := func(bytes int64) {
			clock.advance(hashRampWindow)
			th.noteReadBytes(bytes)
		}
		feed(100) // 首窗（无基准）
		feed(200) // 首窗裁决：无基准不动；本窗 200
		if th.limit() != 4 {
			t.Fatalf("首窗无基准不得动并发，实际 %d", th.limit())
		}
		feed(300) // 裁决 200>100 → 升 4→6
		if th.limit() != 6 {
			t.Fatalf("较前窗提升应 +2（4→6），实际 %d", th.limit())
		}
		feed(400) // 裁决 300>200 → 升 6→8
		if th.limit() != 8 {
			t.Fatalf("连续提升应到自适应上限 8，实际 %d", th.limit())
		}
		feed(500) // 裁决 400>300 → 已达上限不动
		feed(100) // 裁决 500>400 → 上限不动（仍在高位）
		if th.limit() != 8 {
			t.Fatalf("上限 8 不得再升，实际 %d", th.limit())
		}
		feed(50)  // 裁决 100<500×0.8 → 减半 8→4 并定格
		feed(2000) // 裁决 50<100×0.8：再跌——已定格不动
		feed(3000) // 裁决 2000>50：回升——已定格不动
		if th.limit() != 4 {
			t.Fatalf("定格后吞吐变化不得改并发，实际 %d", th.limit())
		}
		if len(remembered) != 1 || remembered[0] != 4 {
			t.Fatalf("定格值应恰回调一次按盘记忆（4），实际 %v", remembered)
		}
		if len(adjusts) != 3 || adjusts[0] != 6 || adjusts[1] != 8 || adjusts[2] != 4 {
			t.Fatalf("并发变化回调序列应为 [6 8 4]（升两档+减半定格；上限/定格维持不回调），实际 %v", adjusts)
		}
	})
	t.Run("偏好钉死轨：吞吐序列不改并发（升不升、跌不降不定格）", func(t *testing.T) {
		clock := &stepClock{t: time.Now()}
		var adjusts []int
		var remembered []int
		th := newHashThrottle(hashThrottleConfig{initial: 16, pinned: true, now: clock.now,
			adjust: func(n int) { adjusts = append(adjusts, n) },
			remember: func(n int) { remembered = append(remembered, n) }})
		for _, b := range []int64{100, 1000, 1, 10000} { // 连升后骤跌
			clock.advance(hashRampWindow)
			th.noteReadBytes(b)
		}
		if th.limit() != 16 {
			t.Fatalf("钉死轨并发恒定 16，实际 %d", th.limit())
		}
		if len(adjusts) != 0 || len(remembered) != 0 {
			t.Fatalf("钉死轨不应有并发变化/定格记忆，实际 adjusts=%v remembered=%v", adjusts, remembered)
		}
	})
	t.Run("缓存命中零字节不误升档（风险5 seam）", func(t *testing.T) {
		clock := &stepClock{t: time.Now()}
		var adjusts []int
		th := newHashThrottle(hashThrottleConfig{initial: 4, now: clock.now,
			adjust: func(n int) { adjusts = append(adjusts, n) }})
		clock.advance(hashRampWindow)
		th.noteReadBytes(1000) // 前窗真实读盘 1000B
		clock.advance(hashRampWindow)
		// 本窗：真实读盘 1000B + 阶段5 哈希缓存命中 5000B（缓存命中零读盘字节——
		// noteReadBytes 是窗口唯一进数口，命中路径不产生调用）
		th.noteReadBytes(1000)
		if th.limit() != 4 {
			t.Fatalf("缓存命中字节不得计入窗口（计入则 6000>1000 误升 6），实际 %d", th.limit())
		}
		if len(adjusts) != 0 {
			t.Fatalf("本窗不应有并发变化回调，实际 %v", adjusts)
		}
	})
	t.Run("速率字段与裁决同窗口统计（seam）", func(t *testing.T) {
		clock := &stepClock{t: time.Now()}
		th := newHashThrottle(hashThrottleConfig{initial: 4, now: clock.now})
		clock.advance(hashRampWindow)
		th.noteReadBytes(100)
		clock.advance(time.Second)
		if rate := th.windowRate(); rate != 100 {
			t.Fatalf("窗口速率应=窗口字节/流逝秒（100B/1s=100），实际 %d", rate)
		}
	})
}

// 退出标准②补充：三层裁决（决策1）——偏好钉死（1-16）优先、按盘记忆（卷序列号
// 独立键）次之、缺省 4；非法偏好回退自动轨。
func TestHashThrottleThreeLayerResolution(t *testing.T) {
	t.Run("偏好钉死优先（初始并发=偏好值）", func(t *testing.T) {
		_, h, _, _, _, _ := newParallelEmitFixture(t, 1, 1, emitDirectCfg{pinned: 12})
		if got := h.resolveHashThrottle(`D:\whatever`).limit(); got != 12 {
			t.Fatalf("偏好 12 应钉死初始并发 12，实际 %d", got)
		}
	})
	t.Run("按盘记忆：定格值经卷序列号键冷启直起", func(t *testing.T) {
		root, h, _, ctx, _, _ := newParallelEmitFixture(t, 1, 1, emitDirectCfg{})
		serial := volumeSerialOfPath(root)
		if serial == "" {
			t.Skip("本环境取不到卷序列号（非 Windows 兜底），按盘记忆面缺席")
		}
		if err := ctx.store.SetValue(hashConcurrencyDiskKey(serial), "6"); err != nil {
			t.Fatalf("预置按盘记忆失败: %v", err)
		}
		if got := h.resolveHashThrottle(root).limit(); got != 6 {
			t.Fatalf("同盘冷启应直起记忆值 6，实际 %d", got)
		}
		// 定格回写面：remember 落 KV、下次解析读到（记忆值即定格值——含低于默认值）
		h.rememberHashConcurrency(serial, 2)
		if got := h.resolveHashThrottle(root).limit(); got != 2 {
			t.Fatalf("定格回写后应直起 2，实际 %d", got)
		}
	})
	t.Run("缺省自动轨=4", func(t *testing.T) {
		root, h, _, _, _, _ := newParallelEmitFixture(t, 1, 1, emitDirectCfg{})
		if got := h.resolveHashThrottle(root).limit(); got != hashConcurrencyDefault {
			t.Fatalf("无偏好无记忆应缺省 %d，实际 %d", hashConcurrencyDefault, got)
		}
	})
	t.Run("非法偏好回退自动轨", func(t *testing.T) {
		_, h, _, ctx, _, _ := newParallelEmitFixture(t, 1, 1, emitDirectCfg{})
		ctx.mu.Lock()
		ctx.prefs = map[string]*sdkdto.PreferenceValue{
			createHashConcurrencyPrefKey: {Data: "20"},
		}
		ctx.mu.Unlock()
		if got := h.resolveHashThrottle(`D:\whatever`).limit(); got != hashConcurrencyDefault {
			t.Fatalf("超域偏好（20>16）应回退默认 %d，实际 %d", hashConcurrencyDefault, got)
		}
	})
}

// 退出标准⑤：首事件 done=0/total=N 与时间节流时序锚（甲1/风险7——末发必须恒发
// 且晚于一切中间值；节流中间值按 ≥2s 时钟放行）。
func TestCreateProgressFirstFeedbackAndTimeThrottle(t *testing.T) {
	const dirs, files = 6, 2
	t.Run("冻结时钟：中间值节流只剩首反馈与终值", func(t *testing.T) {
		root, h, _, ctx, sess, works := newParallelEmitFixture(t, dirs, files, emitDirectCfg{clockStep: 0})
		items, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{})
		if reason != "" || cancelled {
			t.Fatalf("直驱不应中止: %s / %v", reason, cancelled)
		}
		if len(items) == 0 {
			t.Fatalf("应正常发射子行")
		}
		verify := phaseEvents(t, ctx, createPhaseVerify)
		// 冻结时钟：首反馈 (0,N) → 节流器首放一条中间值 (1,N) → 终值 (N,N) 恒发
		want := []int{0, 1, dirs}
		if len(verify) != len(want) {
			t.Fatalf("冻结时钟下核对相事件应为 %v，实际 %d 条", want, len(verify))
		}
		for i, evt := range verify {
			if evt.Done != want[i] || evt.Total != dirs {
				t.Fatalf("第 %d 条核对事件应为 (%d/%d)，实际 (%d/%d)", i, want[i], dirs, evt.Done, evt.Total)
			}
		}
		final := verify[len(verify)-1]
		if final.Done != dirs || final.Total != dirs {
			t.Fatalf("末发应为终值 %d/%d，实际 %+v", dirs, dirs, final)
		}
	})
	t.Run("步进时钟：逐值放行、单调递增、终值殿后", func(t *testing.T) {
		root, h, _, ctx, sess, works := newParallelEmitFixture(t, dirs, files, emitDirectCfg{clockStep: 3 * time.Second})
		if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
			t.Fatalf("直驱不应中止: %s / %v", reason, cancelled)
		}
		verify := phaseEvents(t, ctx, createPhaseVerify)
		if len(verify) != dirs+1 {
			t.Fatalf("步进时钟（3s≥2s）下核对相事件应逐值放行共 %d 条，实际 %d", dirs+1, len(verify))
		}
		for i, evt := range verify {
			if evt.Done != i || evt.Total != dirs {
				t.Fatalf("核对事件应单调递增（第 %d 条 done=%d）", i, evt.Done)
			}
		}
		last := verify[len(verify)-1]
		if last.Done != last.Total {
			t.Fatalf("末发必须恒发且为终值（%d/%d）", last.Done, last.Total)
		}
	})
}

// 退出标准⑦：重叠期双计数进度事件序列锚（乙3——核对行〔X/N〕与发射行〔子行
// 数〕并存：首批发射起发射行出现、核对终值后发射行仍在推进直至终值；事件按
// phase 分道）。
func TestOverlappingDualCountProgress(t *testing.T) {
	const dirs, files = 5, 2
	root, h, _, ctx, sess, works := newParallelEmitFixture(t, dirs, files,
		emitDirectCfg{batchCount: 1, clockStep: 3 * time.Second})
	items, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{})
	if reason != "" || cancelled {
		t.Fatalf("直驱不应中止: %s / %v", reason, cancelled)
	}
	if len(emittedBatches(items)) < 2 {
		t.Fatalf("小批界应产生多批次驱动重叠期")
	}
	events := createProgressEvents(t, ctx)
	verifyAt, createAt := map[int]int{}, map[int]int{}
	var verifyDone, createDone []int
	for i, evt := range events {
		switch evt.Phase {
		case createPhaseVerify:
			verifyAt[len(verifyDone)] = i
			verifyDone = append(verifyDone, evt.Done)
		case createPhaseCreate:
			createAt[len(createDone)] = i
			createDone = append(createDone, evt.Done)
		}
	}
	// 核对相：首反馈 0 → 终值 N（逐值在场）
	if len(verifyDone) != dirs+1 || verifyDone[0] != 0 || verifyDone[len(verifyDone)-1] != dirs {
		t.Fatalf("核对行应为 0..%d 全序列，实际 %v", dirs, verifyDone)
	}
	// 发射行：自首批发射起出现、递增、终值=实际子行数（核对终值之后仍在场）
	if len(createDone) < 2 || createDone[0] == 0 {
		t.Fatalf("发射行应自首批发射起出现（首事件即非零计数），实际 %v", createDone)
	}
	for i := 1; i < len(createDone); i++ {
		if createDone[i] <= createDone[i-1] {
			t.Fatalf("发射行计数应递增，实际 %v", createDone)
		}
	}
	if want := dirs * files; createDone[len(createDone)-1] != want {
		t.Fatalf("发射行终值应=实际子行数 %d，实际 %v", want, createDone)
	}
	// 重叠锚定：首个发射行事件先于核对终值（核对完成前发射已在推进）；发射
	// 终值晚于核对终值（核对行消失后发射行存活至收尾）
	firstCreateIdx, verifyFinalIdx := createAt[0], verifyAt[len(verifyDone)-1]
	lastCreateIdx := createAt[len(createDone)-1]
	if firstCreateIdx >= verifyFinalIdx {
		t.Fatalf("重叠期锚定失败：首发射行事件（idx=%d）应先于核对终值（idx=%d）", firstCreateIdx, verifyFinalIdx)
	}
	if lastCreateIdx <= verifyFinalIdx {
		t.Fatalf("发射行终值（idx=%d）应晚于核对终值（idx=%d）", lastCreateIdx, verifyFinalIdx)
	}
	// 核对行消失锚：核对终值后不得再出现核对相事件
	for i := verifyFinalIdx + 1; i < len(events); i++ {
		if events[i].Phase == createPhaseVerify {
			t.Fatalf("核对完成后核对行应消失（idx=%d 又现核对事件）", i)
		}
	}
}

// 退出标准④：verify 期心跳在场（甲4——放行循环按节拍上报，注入短节拍 +
// 计数上报器锚定「verify 期有心跳块」；生产节拍=20s 恰配 SDK 限频与宿主 60s
// 空闲窗）。
func TestVerifyHeartbeatDuringEmit(t *testing.T) {
	if createVerifyHeartbeatTick != 20*time.Second {
		t.Fatalf("生产心跳节拍应为 20s（恰配 SDK 限频窗），实际 %v", createVerifyHeartbeatTick)
	}
	// 40 目录 × 256KB：拉长 verify 相确保多个节拍窗；注入 2ms 节拍
	root, h, _, _, sess, works := newParallelEmitFixture(t, 40, 1, emitDirectCfg{fileBytes: 256 << 10})
	h.verifyHeartbeatTick = 2 * time.Millisecond
	reporter := &countingStubReporter{}
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, reporter); reason != "" || cancelled {
		t.Fatalf("直驱不应中止: %s / %v", reason, cancelled)
	}
	if got := reporter.count.Load(); got < 3 {
		t.Fatalf("verify 期应按节拍多次上报心跳（≥3），实际 %d 次", got)
	}
}

// —— 全链夹具（宿主桩驱动：ApplyCreateTasks → CreateTask RPC 桩 → 插件 Create
// 过滤产出分支，钩子注入取消时机）——

// hookedHostCtx 宿主桩：同步递归消费插件过滤产出流（与 recursiveCreateTaskCtx
// 同构），叠加三个注入钩子——preConsume（RPC 入口=零发射阶段取消面）、onItem
//（每收一个批次块=发射中取消面）、onDrained（流收完=已收批取消竞态面，风险10）。
type hookedHostCtx struct {
	workbenchTestCtx
	fetcher      *LocalImportWorkFetcher
	root         string
	preConsume   func()
	onItem       func(batchCount int)
	onDrained    func()
	mu           sync.Mutex
	items        []*sdkdto.CreateStreamItem
	cancelResps  []string
	intentKind   sdkdto.CreateIntentKind
	lastBatch    bool
	childrenSeen int
}

func (c *hookedHostCtx) CreateTask(url string) (*sdkdto.CreateTaskResult, error) {
	if c.preConsume != nil {
		c.preConsume()
	}
	result, err := c.fetcher.Create(url)
	if err != nil {
		return nil, err
	}
	if result.Items() == nil {
		return nil, fmt.Errorf("过滤产出应为条目形态流式结果")
	}
	added := 0
	batchCount := 0
	for item := range result.Items() {
		c.mu.Lock()
		c.items = append(c.items, item)
		c.mu.Unlock()
		if task := item.Task(); task != nil {
			added += len(task.Children)
		} else if batch := item.ChildrenBatch(); batch != nil {
			added += len(batch.Items)
			batchCount++
			if batch.Last {
				c.mu.Lock()
				c.lastBatch = true
				c.mu.Unlock()
			}
			if c.onItem != nil {
				c.onItem(batchCount)
			}
		}
	}
	if c.onDrained != nil {
		c.onDrained()
	}
	c.mu.Lock()
	c.childrenSeen = added
	c.mu.Unlock()
	if kind, _ := result.Intent(); kind != sdkdto.CreateIntentNone {
		c.mu.Lock()
		c.intentKind = kind
		c.mu.Unlock()
		return &sdkdto.CreateTaskResult{Succeed: true, AddedQuantity: 0}, nil
	}
	if reason := result.Reason(); reason != "" {
		return &sdkdto.CreateTaskResult{Succeed: false, Msg: reason}, nil
	}
	return &sdkdto.CreateTaskResult{Succeed: true, AddedQuantity: added}, nil
}

// cancelNow 记录一次取消请求的受理结论。
func (c *hookedHostCtx) cancelNow() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cancelResps = append(c.cancelResps, requestCreateCancel(c.root))
}

// newCreateChainFixture 全链环境（样例树+全量识别+宿主桩）：selection=5 作品
//（d:多轨 2 轨、d:已识别 1 轨、f:序列三成员）→ 6 子行；fetcher 可注入面直改。
func newCreateChainFixture(t *testing.T, onItem func(int), preConsume func(), onDrained func()) (string, *SessionManager, *hookedHostCtx) {
	t.Helper()
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &hookedHostCtx{workbenchTestCtx: workbenchTestCtx{store: store}, root: root,
		preConsume: preConsume, onItem: onItem, onDrained: onDrained}
	ctx.library = libraryFixture{localAuthors: []string{"库内作者名"}}
	m := NewSessionManagerWithStore(ctx, nil, store)
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: selectionTestRules(t)}
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	selectionSampleTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	ctx.fetcher = &LocalImportWorkFetcher{sessions: m, ctx: ctx,
		hostContractFn: func() uint32 { return createChildrenBatchMinHostContract }}
	return root, m, ctx
}

// selectAllFive 写入 5 作品选择集（6 子行）。
func selectAllFive(t *testing.T, m *SessionManager, root string) {
	t.Helper()
	if _, err := m.SetSelection(root, []string{
		"d:多轨", "d:已识别",
		"f:序列/前缀名_001.jpg", "f:序列/前缀名_002.jpg", "f:序列/前缀名_003.jpg",
	}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
}

// waitForRunGateOpen 轮询等待创建闸门释放：重发起不再以「创建进行中」拒绝即视为
// 已释放（成功=新运行已启动；其他错误如「未选择」同样证明闸门不在拒绝位）。
func waitForRunGateOpen(t *testing.T, m *SessionManager, root string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := m.ApplyCreateTasks(root, ""); err == nil || !strings.Contains(err.Error(), "创建进行中") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("终态后闸门应及时释放（创建进行中拒绝持续超 2s）")
}

// 退出标准③：取消单测（甲3/决策3）——置位后流无 last 收流（宿主未收批统一回滚
// 面）+取消意图块+闸门释放；零发射阶段（verify 中）取消直接终态。
func TestCancelCreateMidEmission(t *testing.T) {
	var ctx *hookedHostCtx
	root, m, ctx := newCreateChainFixture(t,
		func(batchCount int) {
			if batchCount == 2 { // 第 2 个批次块到达后置取消（发射相/重叠期）
				ctx.cancelNow()
			}
		}, nil, nil)
	ctx.fetcher.childrenBatchCountLimit = 1 // 单子行成批：首批尽早发射
	selectAllFive(t, m, root)

	started, err := m.ApplyCreateTasks(root, "取消批次")
	if err != nil || !started {
		t.Fatalf("创建编排应立即应答已开始: started=%v err=%v", started, err)
	}
	done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	if done.Success {
		t.Fatalf("取消后终态不应为成功，实际: %s", done.Message)
	}
	if !strings.Contains(done.Message, "已取消创建") || !strings.Contains(done.Message, "已删") {
		t.Fatalf("取消终态应携带清理量文案，实际 %q", done.Message)
	}
	if done.Done == 0 || done.Done >= done.Total {
		t.Fatalf("部分发射后取消应携带部分清理计数（0 < X < N），实际 %+v", done)
	}
	// 受理结论=已受理；流收尾形态=无 last 收流（宿主未收批回滚面）+取消意图块
	ctx.mu.Lock()
	resps := append([]string(nil), ctx.cancelResps...)
	items := append([]*sdkdto.CreateStreamItem(nil), ctx.items...)
	intent := ctx.intentKind
	lastBatch := ctx.lastBatch
	ctx.mu.Unlock()
	if len(resps) != 1 || resps[0] != createCancelAccepted {
		t.Fatalf("发射中取消应受理为 cancelling，实际 %v", resps)
	}
	if lastBatch {
		t.Fatalf("取消收流不得发射末批 last（宿主按未收批统一回滚），实得 last 已发")
	}
	if intent != sdkdto.CreateIntentCancelled {
		t.Fatalf("取消收尾应以意图块「已取消」收流，实际 %v", intent)
	}
	// 选择集保留 + 闸门释放（重发起不被「创建进行中」拒绝）
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if len(sess.Selection) == 0 {
		t.Fatalf("取消终态应保留选择集")
	}
	_ = items
	waitForRunGateOpen(t, m, root)
	// 重发起的创建应收尾（闸门释放后的净空验证），等待其终态避免泄漏干扰后续测试
	waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
}

// 退出标准③（零发射阶段）：verify 中取消——无开批父无行可删、直接终态（部分
// 完成计数不出现）。
func TestCancelCreateZeroEmission(t *testing.T) {
	t.Run("全链：RPC 入口即取消（置位先于 arm 的窄窗）", func(t *testing.T) {
		var ctx *hookedHostCtx
		root, m, ctx := newCreateChainFixture(t, nil, func() { ctx.cancelNow() }, nil)
		selectAllFive(t, m, root)
		if started, err := m.ApplyCreateTasks(root, ""); err != nil || !started {
			t.Fatalf("创建编排应立即应答已开始: %v / %v", started, err)
		}
		done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
		if done.Success || !strings.Contains(done.Message, "已取消创建：尚未发射任何子任务") {
			t.Fatalf("零发射阶段取消应为直接取消终态，实际 %q", done.Message)
		}
		ctx.mu.Lock()
		resps := append([]string(nil), ctx.cancelResps...)
		items := append([]*sdkdto.CreateStreamItem(nil), ctx.items...)
		intent := ctx.intentKind
		ctx.mu.Unlock()
		if len(resps) != 1 || resps[0] != createCancelAccepted || len(items) != 0 || intent != sdkdto.CreateIntentCancelled {
			t.Fatalf("零发射取消应受理+零条目+取消意图，实际 resps=%v items=%d intent=%v", resps, len(items), intent)
		}
		if sess, err := m.SessionOf(root); err != nil || len(sess.Selection) == 0 {
			t.Fatalf("零发射取消应保留选择集")
		}
		waitForRunGateOpen(t, m, root)
		waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	})
	t.Run("直驱：verify 前置位即零发射取消收尾", func(t *testing.T) {
		root, h, _, _, sess, works := newParallelEmitFixture(t, 4, 2, emitDirectCfg{})
		beginCreateRun(root)
		defer endCreateRun(root)
		if resp := requestCreateCancel(root); resp != createCancelAccepted {
			t.Fatalf("在途取消应受理，实际 %q", resp)
		}
		items, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{})
		if !cancelled || reason != "" || len(items) != 0 {
			t.Fatalf("前置取消应零发射取消收尾（cancelled=%v reason=%q items=%d）", cancelled, reason, len(items))
		}
	})
}

// 退出标准⑥：取消×完成竞态（风险10）——末批 last 已发（原子提交点）后收到的
// 取消请求应答「已完成」、不置取消位、不回滚，创建照常成功收尾（选择集清空、
// 成功终态文案）。
func TestCancelAfterCommitPointAnswerCompleted(t *testing.T) {
	var ctx *hookedHostCtx
	root, m, ctx := newCreateChainFixture(t, nil, nil, func() { ctx.cancelNow() }) // 流收完=末批 last 已发后
	selectAllFive(t, m, root)
	if started, err := m.ApplyCreateTasks(root, "竞态批次"); err != nil || !started {
		t.Fatalf("创建编排应立即应答已开始: %v / %v", started, err)
	}
	done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	if !done.Success || !strings.Contains(done.Message, "已创建任务「竞态批次」") {
		t.Fatalf("提交点后的取消不应影响成功收尾，实际 %q", done.Message)
	}
	ctx.mu.Lock()
	resps := append([]string(nil), ctx.cancelResps...)
	childrenSeen := ctx.childrenSeen
	intent := ctx.intentKind
	ctx.mu.Unlock()
	if len(resps) != 1 || resps[0] != createCancelCompleted {
		t.Fatalf("末批已发后的取消请求应答 completed（已完成），实际 %v", resps)
	}
	if childrenSeen != 6 {
		t.Fatalf("原子提交点=全部子行已发射（单批全内联形态无批次块），实得 %d 子行", childrenSeen)
	}
	if intent != sdkdto.CreateIntentNone {
		t.Fatalf("成功收尾不应携带取消意图，实际 %v", intent)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if len(sess.Selection) != 0 {
		t.Fatalf("成功收尾应清空选择集")
	}
	// 取消位未被置起：闸门随终态自然释放（选择集已清空，重发起以「未选择」拒绝
	//——非闸门拒绝即证明不在「创建进行中」位）
	waitForRunGateOpen(t, m, root)
}

// 退出标准⑧：磁盘对账等价保持（阶段3 语义在并行/取消路径下不回退）——多单元
// 并行下后位单元 Stat 失败：早放行单元已发射的批次无 last 末批（宿主未收批
// 统一回滚=用户可见结局与整次中断一致）、对账文案收流、零簿记。
func TestReconcileAbortUnderPipelineKeepsNoLastSemantics(t *testing.T) {
	root, h, m, _, sess, works := newParallelEmitFixture(t, 3, 2, emitDirectCfg{batchCount: 1})
	// 删除末目录一轨（其单元在域序末位）：在先单元照常放行发射、末单元 Stat 中断
	if err := removeFile(t, root+"/目录02/文件01.jpg"); err != nil {
		t.Fatalf("删除所选文件失败: %v", err)
	}
	items, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{})
	if cancelled {
		t.Fatalf("对账中断是失败收尾而非取消收尾")
	}
	if reason == "" || !strings.Contains(reason, "磁盘对账未通过") || !strings.Contains(reason, "目录02/文件01.jpg") {
		t.Fatalf("后位单元 Stat 失败应整次中断并对账文案收流，实际 %q", reason)
	}
	// 已发射批次存在但无 last（未收批回滚面）；父块在场的部分子行同被宿主回滚
	if len(items) == 0 {
		t.Fatalf("流水线下早放行单元应已有部分发射（非平凡路径）")
	}
	for _, batch := range emittedBatches(items) {
		if batch.Last {
			t.Fatalf("对账中断不得发射末批 last（宿主按未收批统一回滚），实得 last 已发")
		}
	}
	if summary := m.takeCreateBatchSummary(root); summary.total != 0 {
		t.Fatalf("对账中止不应记录批次摘要，实际 %+v", summary)
	}
}

// 风险6 锚定：并行哈希下「文件不可访问」告警聚合成批——哈希失败 N 件仅一次
// 聚合告警（含件数与样本），不逐件洪泛。失败注入面：Windows 上 Stat 成功但
// Read 失败的形态=名字即文件路径的目录（os.Open 目录句柄可开、读必败）。
func TestHashFailureWarningAggregated(t *testing.T) {
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &warnCountingCtx{workbenchTestCtx: workbenchTestCtx{store: store}}
	m := NewSessionManagerWithStore(ctx, nil, store)
	var works []*SelectableWork
	for d := 0; d < 5; d++ {
		dirKey := fmt.Sprintf("散%02d", d)
		rel := dirKey + "/独.jpg"
		if d%2 == 0 { // 3 件作品：轨道路径是目录——Stat 通过、哈希读失败 → 整件放弃
			if err := os.MkdirAll(root+"/"+rel, 0o755); err != nil {
				t.Fatalf("构造不可读轨道失败: %v", err)
			}
		} else {
			writeTestFile(t, root+"/"+rel, "ok")
		}
		works = append(works, &SelectableWork{
			WorkID: "d:" + dirKey, Kind: WorkKindDirTracks, DirRelKey: dirKey, Files: []string{rel},
		})
	}
	h := &LocalImportWorkFetcher{sessions: m, ctx: ctx,
		hostContractFn: func() uint32 { return createChildrenBatchMinHostContract }}
	items, reason, cancelled := runEmitDirect(t, h, root, &WorkbenchSession{Snapshot: &RecognitionSnapshot{}}, works, sdkdto.NoopHeartbeat{})
	if reason != "" || cancelled {
		t.Fatalf("哈希失败为整件放弃不中止创建: %s / %v", reason, cancelled)
	}
	ctx.mu.Lock()
	warns := append([]string(nil), ctx.warns...)
	ctx.mu.Unlock()
	if len(warns) != 1 || !strings.Contains(warns[0], "3 件") {
		t.Fatalf("哈希失败应聚合成一次告警（含件数），实际 %d 条: %v", len(warns), warns)
	}
	var names []string
	for _, item := range items {
		if task := item.Task(); task != nil {
			names = append(names, responseChildNames(task)...)
		} else if batch := item.ChildrenBatch(); batch != nil {
			for _, c := range batch.Items {
				names = append(names, c.TaskName)
			}
		}
	}
	if len(names) != 2 {
		t.Fatalf("应只产出可读作品的子行（2），实际 %v", names)
	}
}

// warnCountingCtx 记录 Warnf 调用的桩（聚合告警断言面）。
type warnCountingCtx struct {
	workbenchTestCtx
	warnMu sync.Mutex
	warns  []string
}

func (c *warnCountingCtx) Warnf(format string, args ...any) {
	c.warnMu.Lock()
	defer c.warnMu.Unlock()
	c.warns = append(c.warns, fmt.Sprintf(format, args...))
}

// cancelCreate 工作台动作分派（甲3）：受理三态应答面。
func TestCancelCreateActionDispatch(t *testing.T) {
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)
	// 无在途创建：错误应答
	m.HandleFrontendRequest([]byte(`{"seq":1,"action":"cancelCreate","root":` + jsonQuote(`D:\无在途`) + `}`))
	if resp := ctx.lastResponse(t); resp.OK || resp.CancelState != "" {
		t.Fatalf("无在途取消应错误应答且无 cancelState，实际 %+v", resp)
	}
	// 在途：受理应答 cancelling（登记运行态后直取）
	root := `D:\在途`
	beginCreateRun(root)
	defer endCreateRun(root)
	m.HandleFrontendRequest([]byte(`{"seq":2,"action":"cancelCreate","root":` + jsonQuote(root) + `}`))
	if resp := ctx.lastResponse(t); !resp.OK || resp.CancelState != createCancelAccepted {
		t.Fatalf("在途取消应应答 cancelling，实际 %+v", resp)
	}
	// 已收批（原子提交点后）：应答 completed
	markCreateCommitted(root)
	m.HandleFrontendRequest([]byte(`{"seq":3,"action":"cancelCreate","root":` + jsonQuote(root) + `}`))
	if resp := ctx.lastResponse(t); !resp.OK || resp.CancelState != createCancelCompleted {
		t.Fatalf("提交点后的取消应应答 completed，实际 %+v", resp)
	}
}

// TestEmitHeartbeatSurvivesChannelBackpressure 心跳节拍与发射通道背压解耦的锚定：
// 发射通道无消费方、缓冲填满时释放环阻塞在通道发送处——独立心跳 goroutine 的
// 节拍仍按注入周期在场（随放行循环驱动的节拍在阻塞期全停）。停摆解除后发射
// 完整收尾（无死锁、产出齐全）。
func TestEmitHeartbeatSurvivesChannelBackpressure(t *testing.T) {
	// 25 单文件作品 × 单批条数界 1 = 25 批 > 通道缓冲 16：消费方停摆期释放环
	// 必然阻塞在通道发送处
	root, h, _, _, sess, works := newParallelEmitFixture(t, 25, 1, emitDirectCfg{batchCount: 1})
	h.verifyHeartbeatTick = 20 * time.Millisecond
	reporter := &countingStubReporter{}
	ch := make(chan *sdkdto.CreateStreamItem, 16)
	var reason string
	var cancelled bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		reason, cancelled = h.emitSelectedWorks(root, sess, works, "背压批次", ch, reporter)
		close(ch)
	}()
	<-ch // 首件（父块内联批）到达即停摆消费方，通道填满后释放环阻塞
	time.Sleep(200 * time.Millisecond)
	during := reporter.count.Load()
	items := 1 // 首件已预读，收卷计数自 1 起
	for range ch {
		items++
	}
	wg.Wait()
	if during < 5 {
		t.Fatalf("通道背压期心跳节拍应在场（200ms 停摆 × 20ms 节拍应 ≥5 次），实际 %d 次", during)
	}
	if items != 25 || reason != "" || cancelled {
		t.Fatalf("背压解除后发射应完整收尾：items=%d reason=%q cancelled=%v", items, reason, cancelled)
	}
}
