// 会话内存缓存（刀1）与按页投影（刀2）的单元测试：缓存命中零存储读、保存后
// 缓存更新/失效、TTL 过期（注入时钟）、容量 LRU 逐出、变异旁路不污染缓存、
// 删除/重建不复活缓存项、domain 推导随缓存条目复用，以及并发读 + 识别写落库
// 的 -race 锚定（getPanorama 装载性能收口）。
package main

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// newCacheTestManager 建立接在读计数存储（countingSessionStore，session_payload_test.go）
// 上的会话管理器（ctx 同接底层内存存储，供 HandleFrontendRequest 应答发布）；
// 顺带建会话并注入带快照的识别完成态。clock 非 nil 时在建会话前注入（TTL/LRU
// 测试的时钟控制）。
func newCacheTestManager(t *testing.T, root string, gen int64, clock func() time.Time) (*SessionManager, *workbenchTestCtx, *countingSessionStore) {
	t.Helper()
	store := newCountingSessionStore()
	ctx := &workbenchTestCtx{store: store.fakeSessionStore}
	m := NewSessionManagerWithStore(ctx, nil, store)
	if clock != nil {
		m.sessCacheNow = clock
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if _, err := m.CompleteRecognition(root, cacheTestSnapshot(root, gen)); err != nil {
		t.Fatalf("注入快照失败: %v", err)
	}
	return m, ctx, store
}

// cacheTestSnapshot 缓存测试用最小快照（gen 为世代）。
func cacheTestSnapshot(root string, gen int64) *RecognitionSnapshot {
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: gen,
		RootPath:      root,
	}
	for i := 0; i < 6; i++ {
		rel := fmt.Sprintf("甲/文件_%02d.jpg", i)
		snap.Files = append(snap.Files, SnapFile{
			RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: "甲", State: snapStatePlain,
		})
	}
	return snap
}

// TestSessionCacheHitZeroStoreReads 缓存命中零存储读：保存成功即入缓存
//（putSessionCache），随后 SessionOf 连读零存储读且同一共享实例；旁路逐出后
// 首读走全量解码（存储读增加），次读回零。
func TestSessionCacheHitZeroStoreReads(t *testing.T) {
	root := `D:\缓存库甲`
	m, _, store := newCacheTestManager(t, root, 1000, nil)

	// 保存（CompleteRecognition→saveSessionFull）已重建缓存项：首读零存储读
	before := store.readHits()
	first, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if delta := store.readHits() - before; delta != 0 {
		t.Fatalf("保存后首读应命中缓存零存储读，实际 %d 次", delta)
	}
	second, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if delta := store.readHits() - before; delta != 0 {
		t.Fatalf("第二次 SessionOf 应零存储读，实际 %d 次", delta)
	}
	if first != second {
		t.Fatal("缓存命中应返回同一共享只读实例")
	}

	// 显式逐出（删除/容量路径行为）→ 首读全量解码、次读回缓存；变异旁路
	//（loadSessionFresh）不逐出缓存，另见 TestSessionMutationBypassNotPollutingCache
	m.evictSessionCache(normalizeLibraryRoot(root))
	before = store.readHits()
	third, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if delta := store.readHits() - before; delta == 0 {
		t.Fatal("逐出后首读应走存储解码")
	}
	if third == first {
		t.Fatal("逐出后重载应为新实例")
	}
	if fourth, err := m.SessionOf(root); err != nil || fourth != third {
		t.Fatalf("重载后应回缓存共享实例: %v", err)
	}
}

// TestSessionCacheSaveInvalidation 保存后缓存更新：轻保存（赋值）与全保存
//（识别轮落库）成功后，缓存读路径立即可见新值（变异立即可见）。
func TestSessionCacheSaveInvalidation(t *testing.T) {
	root := `D:\缓存库乙`
	m, _, _ := newCacheTestManager(t, root, 1000, nil)

	if _, err := m.SetAssignment(root, "甲/文件_00.jpg", SessionAssignment{Author: "作者X"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if got := sess.Assignments["甲/文件_00.jpg"].Author; got != "作者X" {
		t.Fatalf("轻保存后缓存应立即可见赋值，实际 %q", got)
	}

	// 全保存（识别轮落库）后世代前移即可见
	if _, err := m.CompleteRecognition(root, cacheTestSnapshot(root, 2000)); err != nil {
		t.Fatalf("重识别落库失败: %v", err)
	}
	sess, err = m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if sess.Snapshot.GeneratedAtMs != 2000 {
		t.Fatalf("全保存后缓存世代应前移，实际 %d", sess.Snapshot.GeneratedAtMs)
	}
}

// TestSessionCacheTTLExpiry 空闲 TTL 逐出：注入时钟推进越过 sessionCacheTTL 后
// 缓存条目过期，读路径回存储解码（新实例）。
func TestSessionCacheTTLExpiry(t *testing.T) {
	root := `D:\缓存库丙`
	now := time.UnixMilli(1700000000000)
	m, _, store := newCacheTestManager(t, root, 1000, func() time.Time { return now })

	first, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	before := store.readHits()
	if _, err := m.SessionOf(root); err != nil || store.readHits() != before {
		t.Fatalf("TTL 内应命中缓存（零存储读）: %v", err)
	}

	now = now.Add(sessionCacheTTL + time.Second) // 时钟推进越过 TTL
	second, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if store.readHits() == before {
		t.Fatal("TTL 过期后应回存储解码")
	}
	if second == first {
		t.Fatal("TTL 过期后应为新解码实例")
	}
}

// TestSessionCacheCapacityEviction 容量 LRU 逐出：容量上限 2——第三库根装入时
// 逐出最久未用者；被逐出根再读回存储（新实例），未逐出根仍命中（指针同一）。
func TestSessionCacheCapacityEviction(t *testing.T) {
	clock := time.UnixMilli(1700000000000)
	m, _, _ := newCacheTestManager(t, `D:\库A`, 1000, func() time.Time { return clock })
	normA, normB := normalizeLibraryRoot(`D:\库A`), normalizeLibraryRoot(`D:\库B`)

	// 时钟步进建 B、C（A 已在缓存 @t0；B @t1、C @t2 装入 → LRU 逐出 A）
	clock = clock.Add(time.Minute)
	createdB, err := m.CreateSession(`D:\库B`)
	if err != nil {
		t.Fatalf("建立会话（库B）失败: %v", err)
	}
	clock = clock.Add(time.Minute)
	if _, err := m.CreateSession(`D:\库C`); err != nil {
		t.Fatalf("建立会话（库C）失败: %v", err)
	}
	m.sessCacheMu.Lock()
	_, hasA := m.sessCache[normA]
	m.sessCacheMu.Unlock()
	if hasA {
		t.Fatal("容量上限 2：装入库C 后最久未用的库A 应被 LRU 逐出")
	}

	// 库A 回读（@t3 miss → 装入 → 逐出库B）；再读命中同一实例
	clock = clock.Add(time.Minute)
	a1, err := m.SessionOf(`D:\库A`)
	if err != nil {
		t.Fatalf("SessionOf(库A) 失败: %v", err)
	}
	m.sessCacheMu.Lock()
	_, hasB := m.sessCache[normB]
	m.sessCacheMu.Unlock()
	if hasB {
		t.Fatal("库A 回读装入后，最久未用的库B 应被 LRU 逐出")
	}
	clock = clock.Add(time.Minute)
	if a2, err := m.SessionOf(`D:\库A`); err != nil || a2 != a1 {
		t.Fatalf("库A 回读后应命中同一实例: %v", err)
	}

	// 被逐出的库B 再读：回存储重载（与 CreateSession 返回的旧实例不同指针）
	clock = clock.Add(time.Minute)
	bReloaded, err := m.SessionOf(`D:\库B`)
	if err != nil {
		t.Fatalf("SessionOf(库B) 失败: %v", err)
	}
	if bReloaded == createdB {
		t.Fatal("被逐出后的库B 应为存储重载的新实例")
	}
	m.sessCacheMu.Lock()
	count := len(m.sessCache)
	m.sessCacheMu.Unlock()
	if count > sessionCacheCapacity {
		t.Fatalf("缓存容量超限: %d > %d", count, sessionCacheCapacity)
	}
}

// TestSessionMutationBypassNotPollutingCache 变异旁路不污染缓存：
// loadSessionFresh 返回私有实例，其上改字段不落库不污染缓存；mutate 变异失败
//（mutate 返回错误）时缓存项不被写坏。
func TestSessionMutationBypassNotPollutingCache(t *testing.T) {
	root := `D:\缓存库丁`
	m, _, _ := newCacheTestManager(t, root, 1000, nil)

	shared, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	priv, err := m.loadSessionFresh(root)
	if err != nil {
		t.Fatalf("loadSessionFresh 失败: %v", err)
	}
	if priv == shared {
		t.Fatal("旁路实例不得与缓存共享实例同一")
	}
	priv.State = WorkbenchSessionState("篡改态") // 私有实例上改字段（不落库）

	after, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if after == priv || after.State == WorkbenchSessionState("篡改态") {
		t.Fatal("旁路实例的字段篡改不得污染缓存")
	}
	if after.State != WorkbenchSessionRecognized {
		t.Fatalf("缓存实例应保持存储真值，实际 %s", after.State)
	}

	// 变异中止（mutate 返回错误）不落库：缓存随后读仍为存储真值
	sentinel := errors.New("测试中止")
	if _, err := m.mutateSession(root, func(sess *WorkbenchSession) error {
		sess.State = WorkbenchSessionState("中止态")
		return sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("mutateSession 应回传中止错误: %v", err)
	}
	final, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if final.State != WorkbenchSessionRecognized {
		t.Fatalf("中止的变异不得影响后续读，实际 %s", final.State)
	}
}

// TestSessionCacheDeleteAndReplaceNoResurrect 删除与重建不复活缓存项：
// DeleteSession 后缓存不得答旧会话；ReplaceSession 后读到重建的新会话（无快照）。
func TestSessionCacheDeleteAndReplaceNoResurrect(t *testing.T) {
	root := `D:\缓存库戊`
	m, _, _ := newCacheTestManager(t, root, 1000, nil)

	if _, err := m.SessionOf(root); err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if err := m.DeleteSession(root); err != nil {
		t.Fatalf("删除会话失败: %v", err)
	}
	if _, err := m.SessionOf(root); !errors.Is(err, ErrWorkbenchSessionNotFound) {
		t.Fatalf("删除后缓存不得复活旧会话: %v", err)
	}

	if _, err := m.ReplaceSession(root); err != nil {
		t.Fatalf("重建会话失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if sess.State != WorkbenchSessionCreated || sess.Snapshot != nil {
		t.Fatalf("重建后应为无快照新会话: state=%s snapshot=%v", sess.State, sess.Snapshot != nil)
	}
}

// TestPanoramaDomainDerivationCached domain 推导随缓存条目复用：首个 domain 分节
// 请求后条目内驻留推导结果，后续请求零重复推导（指针同一）；写路径重建条目后
// 推导结果随之作废重推（与快照同生命周期、同世代失效）。
func TestPanoramaDomainDerivationCached(t *testing.T) {
	root := `D:\缓存库己`
	m, ctx, _ := newCacheTestManager(t, root, 1000, nil)

	pullDomain := func(seq int64) {
		t.Helper()
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: seq, Action: workbenchActionGetPanorama, Root: root, Section: panoramaSectionDomain,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Domain == nil || resp.Page == nil || resp.Page.Total == 0 {
			t.Fatalf("domain 分节应答不符: %+v", resp)
		}
	}

	pullDomain(1)
	m.sessCacheMu.Lock()
	entry := m.sessCache[normalizeLibraryRoot(root)]
	m.sessCacheMu.Unlock()
	if entry == nil || entry.domain == nil {
		t.Fatal("首个 domain 请求后缓存条目应驻留推导结果")
	}
	first := entry.domain

	pullDomain(2)
	if entry.domain != first {
		t.Fatal("同一缓存条目内的 domain 推导应复用（指针同一）")
	}

	// 写路径（全保存）重建缓存条目：旧推导结果不随行（世代失效）
	if _, err := m.CompleteRecognition(root, cacheTestSnapshot(root, 2000)); err != nil {
		t.Fatalf("重识别落库失败: %v", err)
	}
	pullDomain(3)
	m.sessCacheMu.Lock()
	renewed := m.sessCache[normalizeLibraryRoot(root)]
	m.sessCacheMu.Unlock()
	if renewed == nil || renewed == entry {
		t.Fatal("写路径应重建缓存条目")
	}
	if renewed.domain == nil || renewed.domain == first {
		t.Fatal("重建条目应重推 domain（与快照同生命周期失效）")
	}
}

// TestSessionCacheConcurrentReadWithRecognitionWrite 并发读 + 识别写落库的
// -race 锚定：读者循环 SessionOf→panoramaPageOf（files/domain 双分节），写者
// 循环 CompleteRecognitionWithDiff（识别轮保存的写通路）。缓存共享实例只读、
// 变异走旁路的防线在竞态检测器下必须零报告；读者每页世代须与所见会话一致。
func TestSessionCacheConcurrentReadWithRecognitionWrite(t *testing.T) {
	root := `D:\缓存库庚`
	m, _, _ := newCacheTestManager(t, root, 1000, nil)

	const readerCount = 3
	const iterations = 200
	var wg sync.WaitGroup
	fail := make(chan string, readerCount+1)

	for r := 0; r < readerCount; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				sess, err := m.SessionOf(root)
				if err != nil {
					fail <- fmt.Sprintf("读者 SessionOf 失败: %v", err)
					return
				}
				if sess.Snapshot == nil {
					fail <- "读者见到无快照会话"
					return
				}
				gen := sess.Snapshot.GeneratedAtMs
				for _, section := range []string{panoramaSectionFiles, panoramaSectionDomain} {
					page, err := m.panoramaPageOf(sess, workbenchFrontendRequest{
						Seq: 1, Action: workbenchActionGetPanorama, Root: root,
						Section: section, PageSizeHint: 2,
					})
					if err != nil {
						fail <- fmt.Sprintf("读者分节求值失败（%s）: %v", section, err)
						return
					}
					if page.receipt.Generation != gen {
						fail <- fmt.Sprintf("读者页世代 %d 与会话世代 %d 不一致", page.receipt.Generation, gen)
						return
					}
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			if _, err := m.CompleteRecognitionWithDiff(root, cacheTestSnapshot(root, 2000+int64(i)), nil); err != nil {
				fail <- fmt.Sprintf("写者识别落库失败: %v", err)
				return
			}
		}
	}()

	wg.Wait()
	close(fail)
	for msg := range fail {
		t.Fatal(msg)
	}
}

// TestMutateSessionEnvelopeLightPath 信封级轻变异通路（守卫链与会话瘦身四刀
// 之二）：只动轻字段的变异（创建成功后的选择集清理同款形态）走「读信封单键
// 小读→变异→轻保存」——零载荷分片写；缓存条目外层重建（轻字段即时可见、
// 载荷指针只读共享、domain 推导随条目作废重推）；mutate 触碰快照即拒（轻保存
// 静默丢掷的防线）；缓存未装填时不造无载荷条目（下次读走全量解码装填）。
func TestMutateSessionEnvelopeLightPath(t *testing.T) {
	root := `D:\缓存库壬`
	m, _, store := newCacheTestManager(t, root, 1000, nil)

	shared, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	payload := shared.Snapshot
	domain1 := m.workDomainOf(shared)

	// 轻变异：写忽略项（影响域推导的轻字段，创建成功清理的同款通路形态）
	store.resetWriteCounts()
	if _, err := m.mutateSessionEnvelope(root, func(sess *WorkbenchSession) error {
		sess.Ignored = []string{"甲/文件_00.jpg"}
		return nil
	}); err != nil {
		t.Fatalf("信封级轻变异失败: %v", err)
	}
	if store.shardSetValueCalls() != 0 {
		t.Fatalf("信封级轻变异不得触载荷分片，实际分片写 %d 次", store.shardSetValueCalls())
	}

	// 缓存条目外层重建：轻字段立即可见、载荷指针只读共享、domain 作废重推
	after, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if len(after.Ignored) != 1 || after.Snapshot != payload {
		t.Fatalf("缓存条目应外层重建且载荷指针只读共享: ignored=%d payloadShared=%v", len(after.Ignored), after.Snapshot == payload)
	}
	if domain2 := m.workDomainOf(after); domain2 == domain1 {
		t.Fatal("domain 推导应随条目重建作废重推（忽略集已变）")
	}

	// 触碰快照的 mutate 拒绝：轻保存不重写分片，静默丢弃即数据损坏
	if _, err := m.mutateSessionEnvelope(root, func(sess *WorkbenchSession) error {
		sess.Snapshot = &RecognitionSnapshot{}
		return nil
	}); err == nil {
		t.Fatal("信封级变异触碰快照应被拒绝")
	}

	// 存储真值核验（跨实例=重启形态）：轻字段已变、载荷经分片往返完整
	m2 := NewSessionManagerWithStore(nil, nil, store)
	reloaded, err := m2.loadSession(root)
	if err != nil {
		t.Fatalf("重启后读取会话失败: %v", err)
	}
	if len(reloaded.Ignored) != 1 || reloaded.Snapshot == nil || reloaded.Snapshot.GeneratedAtMs != 1000 {
		t.Fatalf("重启后会话不符（轻字段已变、载荷完整）: ignored=%d snapshot=%+v", len(reloaded.Ignored), reloaded.Snapshot)
	}

	// 缓存未装填（逐出后）轻变异：不造无载荷条目，随后读走全量解码装填
	m.evictSessionCache(normalizeLibraryRoot(root))
	if _, err := m.mutateSessionEnvelope(root, func(sess *WorkbenchSession) error {
		sess.Selection = []string{"d:甲"} // 值不经验证，只锚通路
		return nil
	}); err != nil {
		t.Fatalf("缓存未装填时的信封级轻变异失败: %v", err)
	}
	warm, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("SessionOf 失败: %v", err)
	}
	if warm.Snapshot == nil {
		t.Fatal("逐出后的重装填应为全量解码实例（含载荷），不得是无载荷轻条目")
	}
	if len(warm.Selection) != 1 {
		t.Fatalf("重装填实例应携带信封级轻变异后的轻字段，实际 %v", warm.Selection)
	}
}
