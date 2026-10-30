// verify 哈希并发爬坡控制器（大选择集创建链加速 · 决策1 三层）：
// ① 会话内自适应——并发初始 4，5s 吞吐窗口统计聚合读盘字节，较前窗提升则 +2
//（上限 8），跌超 20% 即减半并定格；② 按盘记忆——定格值连同卷序列号
//（GetVolumeInformationW）存插件 KV 独立键，下次同盘直起记忆值；③ 偏好钉死——
// 插件偏好 createHashConcurrency（0=自动、1-16 钉死）。
// 窗口只统计真实读盘字节（方案风险5 的 seam：阶段5 哈希缓存命中的文件不产生
// 读 IO、不调用 noteReadBytes——计入会令控制器被虚高吞吐误导而误升档）；
// 缓存命中占比高的窗口读盘字节自然偏小，控制器按真实 IO 保守降档，属既定口径。
package main

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// —— 决策1 爬坡参数 ——
const (
	// hashConcurrencyDefault 自适应轨初始并发。
	hashConcurrencyDefault = 4
	// hashAdaptiveMax 自适应轨升档上限（决策1：+2 上限 8）。
	hashAdaptiveMax = 8
	// hashConcurrencyPinMax 偏好钉死上限（createHashConcurrency 合法域 1-16），
	// 亦是动态信号量的容量上界。
	hashConcurrencyPinMax = 16
	// hashRampWindow 吞吐窗口时长（窗口时钟可注入，单测决策表用）。
	hashRampWindow = 5 * time.Second
	// hashRampStep 较前窗提升时的升档步长。
	hashRampStep = 2
)

// —— 偏好与按盘记忆键 ——
const (
	// createHashConcurrencyPrefKey 偏好钉死键（插件偏好域；Data=十进制整数字符串：
	// 0=自动，1-16 钉死并发；非法值按自动处理并告警）。
	createHashConcurrencyPrefKey = "createHashConcurrency"
	// hashConcurrencyDiskKeyPrefix 按盘记忆键前缀（定格值独立键：前缀+卷序列号，
	// 值=定格并发十进制；卷序列号经 GetVolumeInformationW 取得，非 Windows/取
	// 失败返回空串=无记忆面，回退纯自适应）。
	hashConcurrencyDiskKeyPrefix = "hash-concurrency:"
)

// hashConcurrencyDiskKey 按盘记忆键（前缀+卷序列号）。
func hashConcurrencyDiskKey(volumeSerial string) string {
	return hashConcurrencyDiskKeyPrefix + volumeSerial
}

// hashThrottleConfig 控制器构造输入：生产经 resolveHashThrottle 按三层裁决填充，
// 单测直接构造注入时钟/回调跑决策表。
type hashThrottleConfig struct {
	// initial 初始并发（自适应轨起点：默认 4 或按盘记忆值）。
	initial int
	// pinned 偏好钉死轨：不自适应（窗口统计照常累计，供速率字段消费）。
	pinned bool
	// now 窗口时钟（缺省 time.Now；决策表单测注入步进时钟）。
	now func() time.Time
	// adjust 并发变化回调（动态信号量收放；缺省丢弃）。
	adjust func(int)
	// remember 定格值按盘记忆回调（KV 持久化；缺省丢弃）。
	remember func(int)
}

// hashThrottle 会话内自适应控制器（生命周期=一次创建的 verify 相）。
type hashThrottle struct {
	mu  sync.Mutex
	cfg hashThrottleConfig

	cur        int       // 当前并发
	frozen     bool      // 自适应定格（降档后本会话不再升档）
	windowByte int64     // 当前窗口累计真实读盘字节
	lastByte   int64     // 上一窗口聚合读盘字节（0=首窗无比较基准）
	windowAt   time.Time // 当前窗口起点（零值=首个字节到达时起窗）
}

// newHashThrottle 构造控制器（initial 钳制到 [1,16]）。
func newHashThrottle(cfg hashThrottleConfig) *hashThrottle {
	if cfg.now == nil {
		cfg.now = time.Now
	}
	cfg.initial = clampInt(cfg.initial, 1, hashConcurrencyPinMax)
	t := &hashThrottle{cfg: cfg, cur: cfg.initial}
	return t
}

// noteReadBytes 累计真实读盘字节（读循环逐块回调；并发安全）。窗口时钟到达时
// 滚动窗口并做升/降/定格裁决（懒滚动：无读盘的窗口不滚动——纯缓存命中窗口
// 不产生裁决，控制器只感知真实 IO）。
func (t *hashThrottle) noteReadBytes(n int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rollLocked()
	t.windowByte += n
}

// rollLocked 滚动到当前时钟所属窗口（固定窗长追赶：长间隙产生若干零字节空窗，
// 各窗独立参与裁决——读停顿即吞吐坍缩，保守定格属既定口径）。
func (t *hashThrottle) rollLocked() {
	now := t.cfg.now()
	if t.windowAt.IsZero() {
		t.windowAt = now
		return
	}
	for now.Sub(t.windowAt) >= hashRampWindow {
		t.evaluateLocked(t.windowByte, t.lastByte)
		t.lastByte = t.windowByte
		t.windowByte = 0
		t.windowAt = t.windowAt.Add(hashRampWindow)
	}
}

// evaluateLocked 单窗裁决（决策1）：较前窗提升→+2（上限 8）；跌超 20%（cur*5 <
// last*4）→减半（下限 1）并定格（记忆值回调）；钉死/已定格/首窗无基准不动。
func (t *hashThrottle) evaluateLocked(cur, last int64) {
	if t.cfg.pinned || t.frozen || last <= 0 {
		return
	}
	switch {
	case cur > last:
		t.setLimitLocked(min(t.cur+hashRampStep, hashAdaptiveMax))
	case cur*5 < last*4:
		t.setLimitLocked(max(t.cur/2, 1))
		t.frozen = true
		if t.cfg.remember != nil {
			t.cfg.remember(t.cur)
		}
	}
}

// setLimitLocked 变更并发并回调信号量收放（adjust 自持锁、不回调本控制器，无反向嵌套）。
func (t *hashThrottle) setLimitLocked(n int) {
	n = clampInt(n, 1, hashConcurrencyPinMax)
	if n == t.cur {
		return
	}
	t.cur = n
	if t.cfg.adjust != nil {
		t.cfg.adjust(n)
	}
}

// limit 当前并发（worker 闸门容量）。
func (t *hashThrottle) limit() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cur
}

// bindAdjust 绑定并发变化回调（构造后、并发使用前调用一次；生产绑动态信号量
// 的 setLimit——控制器裁决即令牌收放）。
func (t *hashThrottle) bindAdjust(fn func(int)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cfg.adjust = fn
}

// windowRate 当前窗口读盘字节/秒（进度事件速率字段的数据源——与爬坡裁决同一
// 窗口统计；窗口未起或时钟未走返回 0）。
func (t *hashThrottle) windowRate() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.windowAt.IsZero() {
		return 0
	}
	elapsed := t.cfg.now().Sub(t.windowAt)
	if elapsed <= 0 {
		return 0
	}
	return int64(float64(t.windowByte) / elapsed.Seconds())
}

// clampInt 钳制到闭区间 [lo, hi]。
func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

// dynSemaphore 可调容量信号量（令牌收放，容量上界 hashConcurrencyPinMax）：
// 缩容时空闲令牌即时回收、在途令牌经 deficit 记账在归还时回收——并发度变化
// 只影响后续取件速度，不抢占在途哈希（保序不变量不受并发度影响）。
type dynSemaphore struct {
	mu      sync.Mutex
	tokens  chan struct{}
	limit   int
	deficit int // 缩容待回收令牌数（归还时优先扣减）
}

// newDynSemaphore 构造信号量并放行 limit 个令牌。
func newDynSemaphore(limit int) *dynSemaphore {
	s := &dynSemaphore{tokens: make(chan struct{}, hashConcurrencyPinMax)}
	s.setLimit(limit)
	return s
}

// setLimit 调整容量：升容补令牌、缩容收令牌（在途令牌经 deficit 延迟回收）。
func (s *dynSemaphore) setLimit(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n = clampInt(n, 1, hashConcurrencyPinMax)
	for s.limit < n {
		s.tokens <- struct{}{}
		s.limit++
	}
	for s.limit > n {
		select {
		case <-s.tokens:
		default:
			s.deficit++
		}
		s.limit--
	}
}

// acquire 取令牌（ctx 取消时返回假——取件方据此退出）。
func (s *dynSemaphore) acquire(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-s.tokens:
		return true
	}
}

// release 归还令牌（缩容记账在场时直接扣减不归还）。
func (s *dynSemaphore) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.deficit > 0 {
		s.deficit--
		return
	}
	s.tokens <- struct{}{}
}

// resolveHashThrottle 三层裁决构造本库根本次创建的控制器（决策1）：
// 偏好钉死 > 按盘记忆 > 默认 4。偏好/记忆读取失败一律静默回退自动轨（加速器
// 永不阻断创建）。
func (h *LocalImportWorkFetcher) resolveHashThrottle(root string) *hashThrottle {
	cfg := hashThrottleConfig{initial: hashConcurrencyDefault}
	// ③ 偏好钉死：createHashConcurrency（0=自动、1-16 钉死）
	if pinned := h.preferenceHashConcurrency(); pinned > 0 {
		cfg.initial, cfg.pinned = pinned, true
		return newHashThrottle(cfg)
	}
	// ② 按盘记忆：定格值连同卷序列号存独立 KV 键，下次同盘直起记忆值
	//（记忆值即定格值——含低于默认值的定格，直起记忆是定格的语义本体）
	if serial := volumeSerialOfPath(root); serial != "" {
		if remembered := h.rememberedHashConcurrency(serial); remembered > 0 {
			cfg.initial = remembered
		}
		cfg.remember = func(n int) { h.rememberHashConcurrency(serial, n) }
	}
	return newHashThrottle(cfg)
}

// preferenceHashConcurrency 读偏好钉死值：缺席/0=自动（返回 0），1-16=钉死值；
// 非法值告警并按自动。上下文不可用=自动。
func (h *LocalImportWorkFetcher) preferenceHashConcurrency() int {
	if h.ctx == nil {
		return 0
	}
	value, ok, err := h.ctx.GetPreference(createHashConcurrencyPrefKey)
	if err != nil {
		h.ctx.Warnf("读取创建哈希并发偏好失败，按自动爬坡处理: %v", err)
		return 0
	}
	if !ok || value == nil {
		return 0
	}
	v, err := strconv.Atoi(value.Data)
	if err != nil || v < 0 || v > hashConcurrencyPinMax {
		h.ctx.Warnf("创建哈希并发偏好值非法（%q），按自动爬坡处理（合法域 0-16，0=自动）", value.Data)
		return 0
	}
	if v == 0 {
		return 0
	}
	return v
}

// rememberedHashConcurrency 读按盘记忆的定格并发（无记录/非法/读取失败=0；
// 记忆值只能来自自适应定格，域 [1,8]）。
func (h *LocalImportWorkFetcher) rememberedHashConcurrency(volumeSerial string) int {
	if h.ctx == nil {
		return 0
	}
	value, err := h.ctx.GetValue(hashConcurrencyDiskKey(volumeSerial))
	if err != nil {
		h.ctx.Warnf("读取按盘哈希并发记忆失败，按默认并发冷启: %v", err)
		return 0
	}
	if value == nil || value.Value == "" {
		return 0
	}
	v, err := strconv.Atoi(value.Value)
	if err != nil || v < 1 || v > hashAdaptiveMax {
		return 0
	}
	return v
}

// rememberHashConcurrency 落定格值到按盘记忆键（尽力而为，失败仅告警）。
func (h *LocalImportWorkFetcher) rememberHashConcurrency(volumeSerial string, concurrency int) {
	if h.ctx == nil {
		return
	}
	if err := h.ctx.SetValue(hashConcurrencyDiskKey(volumeSerial), strconv.Itoa(concurrency)); err != nil {
		h.ctx.Warnf("写入按盘哈希并发记忆失败（下次同盘按默认并发冷启）: %v", err)
	}
}
