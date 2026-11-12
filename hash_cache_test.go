// 大选择集创建链（阶段5）单元测试：乙2 哈希缓存域（决策2）——命中/失效/压实/
// 损坏四态（阶段5 退出标准①-④）、缓存命中路径零读盘字节与控制器窗口联动
//（风险5 锚定，退出标准⑤）、持久化分片布局与旧批次清理、载入损坏形态表与
// 中止合并回写（取消→重试命中面，交互剧本帧4 同源语义）。
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// —— 测试助手 ——

// mustReadHashCacheEnvelope 读哈希缓存信封键并解析（失败即中止）。
func mustReadHashCacheEnvelope(t *testing.T, store SessionStore, root string) *hashCacheEnvelope {
	t.Helper()
	stored, err := store.GetValue(hashCacheKey(root))
	if err != nil {
		t.Fatalf("读取哈希缓存信封失败: %v", err)
	}
	if stored == nil || stored.Value == "" {
		t.Fatalf("哈希缓存信封缺席（键 %q）", hashCacheKey(root))
	}
	if !probeSessionEnvelope(stored.Value) {
		t.Fatalf("信封值应为信封形态，实际: %.120s", stored.Value)
	}
	env, err := parseHashCacheEnvelope(stored.Value)
	if err != nil {
		t.Fatalf("解析哈希缓存信封失败: %v", err)
	}
	return env
}

// hashCacheShardKeysOfRoot 枚举存储中该库根的哈希缓存分片键。
func hashCacheShardKeysOfRoot(t *testing.T, store SessionStore, root string) []string {
	t.Helper()
	all, err := store.GetAllValues()
	if err != nil {
		t.Fatalf("枚举 KV 失败: %v", err)
	}
	prefix := hashCacheSnapKeyPrefix + normalizeLibraryRoot(root) + "@"
	var keys []string
	for key := range all {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys
}

// sameSizeRewriteKeepMtime 命中探针：等长内容改写 + mtime 还原——命中判据
//（relPath+size+mtime）全符而内容已变；若发生真实读盘哈希，hash 必变。返回
// 新内容（供期望 hash 断言）。
func sameSizeRewriteKeepMtime(t *testing.T, full, filler string) string {
	t.Helper()
	fi, err := os.Stat(full)
	if err != nil {
		t.Fatalf("探针前置 Stat 失败: %v", err)
	}
	content := strings.Repeat(filler, 1)
	for len(content) < int(fi.Size()) {
		content += filler
	}
	content = content[:fi.Size()]
	writeTestFile(t, full, content)
	if err := os.Chtimes(full, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatalf("还原探针文件 mtime 失败: %v", err)
	}
	return content
}

// —— 键布局 ——

// TestHashCacheKeyFormat 键格式与库根归一：信封键=前缀+归一化库根（与会话键
// 同一归一规则）；分片键形态与 sessionShardKey 同构；三前缀（信封/分片/会话）
// 互不重叠——会话回退扫描与索引均不误读哈希缓存键。
func TestHashCacheKeyFormat(t *testing.T) {
	if got, want := hashCacheKey(`E:\Media\Lib\`), "hash-cache:e:/media/lib"; got != want {
		t.Fatalf("哈希缓存键 = %q, 期望 %q", got, want)
	}
	for _, form := range []string{`E:\Media\Lib`, "e:/media/lib", ` e:\MEDIA\lib `, `E:\media\lib\`} {
		if key := hashCacheKey(form); key != hashCacheKey(`E:\Media\Lib`) {
			t.Fatalf("库根 %q 应归一到同一键，实际 %q", form, key)
		}
	}
	if got, want := hashCacheShardKey("e:/media/lib", "1700000000000", 2),
		"hash-cache-snap:e:/media/lib@1700000000000#2"; got != want {
		t.Fatalf("分片键 = %q, 期望 %q", got, want)
	}
	// 前缀互不重叠（':' 位置区分信封/分片前缀，与会话键组同构）
	if strings.HasPrefix(hashCacheKey(`D:\库`), workbenchSessionKeyPrefix) ||
		strings.HasPrefix(hashCacheShardKey("d:/库", "1", 0), hashCacheKeyPrefix) ||
		strings.HasPrefix(hashCacheShardKey("d:/库", "1", 0), workbenchSessionSnapKeyPrefix) {
		t.Fatalf("哈希缓存键前缀与会话/分片键前缀发生重叠")
	}
}

// —— 退出标准①+⑤：命中态 + 零读盘字节与控制器窗口联动 ——

// TestHashCacheHitReusesHashZeroReadBytes 命中态单件锚：size+mtime 全符→复用
// hash、零 ComputeFileHashCtx 调用（等长改写+mtime 还原探针——逐字节证明：
// 若读盘重算，hash 必变）；命中路径不向爬坡控制器窗口进数（真控制器 windowRate
// 断言，风险5：noteReadBytes 是窗口唯一进数口），未命中反向锚证明窗口接线在场。
func TestHashCacheHitReusesHashZeroReadBytes(t *testing.T) {
	root, h, m, _, _, works := newParallelEmitFixture(t, 2, 2, emitDirectCfg{})
	work := works[0]

	// 首轮（空缓存）：真实读盘计算并记录 → 压实回写 → 重载
	c1 := newHashCache()
	out1 := h.hashWorkFilesAllOrNothing(context.Background(), root, work, nil, c1)
	if !out1.ok || out1.err != nil || len(out1.files) != 1 {
		t.Fatalf("首轮全量哈希应成功（单轨），实际 ok=%v err=%v files=%d", out1.ok, out1.err, len(out1.files))
	}
	m.saveHashCache(root, c1, true)
	c2 := m.loadHashCache(root)
	if len(c2.entries) != 1 {
		t.Fatalf("回写重载后清单应有 1 条记录，实际 %d", len(c2.entries))
	}

	// 探针：首轨等长改写 + mtime 还原（命中判据全符、内容已变）
	if content := sameSizeRewriteKeepMtime(t, root+"/"+work.Files[0], "X"); content == "" {
		t.Fatalf("探针内容不应为空")
	}

	// ⑤ 风险5 联动：真控制器窗口断言（命中不进数=窗口未起，windowRate 恒 0）
	clock := &stepClock{t: time.Now()}
	th := newHashThrottle(hashThrottleConfig{initial: 4, now: clock.now})
	out2 := h.hashWorkFilesAllOrNothing(context.Background(), root, work, th, c2)
	if !out2.ok || out2.err != nil || len(out2.files) != 1 {
		t.Fatalf("命中轮应成功（单轨），实际 ok=%v err=%v files=%d", out2.ok, out2.err, len(out2.files))
	}
	if rate := th.windowRate(); rate != 0 {
		t.Fatalf("缓存命中不得向控制器窗口进数（风险5），windowRate=%d", rate)
	}
	// ① 命中态：复用缓存 hash（探针内容已变而 hash 不变=零 ComputeFileHashCtx
	// 调用的逐字节证明）
	for i := range out2.files {
		if out2.files[i].Hash != out1.files[i].Hash {
			t.Fatalf("第 %d 轨应复用缓存 hash（探针内容已变仍复用=未读盘重算）", i)
		}
	}

	// 反向锚：未命中路径（次作品不在缓存内）确向窗口进数——窗口接线非平凡在场
	out3 := h.hashWorkFilesAllOrNothing(context.Background(), root, works[1], th, c2)
	if !out3.ok || out3.err != nil {
		t.Fatalf("未命中轮应成功，实际 ok=%v err=%v", out3.ok, out3.err)
	}
	clock.advance(time.Second)
	if rate := th.windowRate(); rate <= 0 {
		t.Fatalf("未命中路径应向控制器窗口进数（反向锚），windowRate=%d", rate)
	}
}

// —— 退出标准②：失效态 ——

// TestHashCacheInvalidationRewrites mtime 变（内容不动）→ 失效重算并按新 mtime
// 更新回写清单（hash 因内容未变而不变）；size 变（内容变）→ 失效重算出新 hash
// 并更新记录。全链驱动（emitSelectedWorks 的载入/命中/回写接线一并覆盖）。
func TestHashCacheInvalidationRewrites(t *testing.T) {
	root, h, m, _, sess, works := newParallelEmitFixture(t, 2, 2, emitDirectCfg{})
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("首轮创建不应中止: %s / %v", reason, cancelled)
	}
	base := m.loadHashCache(root)
	if len(base.entries) != 4 {
		t.Fatalf("首轮回写后清单应有 4 条记录，实际 %d", len(base.entries))
	}

	// mtime 变：内容不动 → 重算 hash 不变、清单记录更新为新 mtime
	rel0, full0 := works[0].Files[0], root+"/"+works[0].Files[0]
	fi, err := os.Stat(full0)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	newTime := fi.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(full0, newTime, newTime); err != nil {
		t.Fatalf("前移 mtime 失败: %v", err)
	}
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("mtime 失效轮不应中止: %s / %v", reason, cancelled)
	}
	c2 := m.loadHashCache(root)
	rec := c2.entries[rel0]
	if rec.mtimeNs != newTime.UnixNano() {
		t.Fatalf("mtime 失效重算后清单应更新为新 mtime（%d），实际 %d", newTime.UnixNano(), rec.mtimeNs)
	}
	if rec.hash != base.entries[rel0].hash {
		t.Fatalf("内容未变时重算 hash 应与旧值一致")
	}

	// size 变：内容变 → 重算出新 hash 并更新 size
	rel1, full1 := works[1].Files[0], root+"/"+works[1].Files[0]
	newContent := "replacement-content-with-different-length"
	writeTestFile(t, full1, newContent)
	fi1, err := os.Stat(full1)
	if err != nil {
		t.Fatalf("Stat 失败: %v", err)
	}
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("size 失效轮不应中止: %s / %v", reason, cancelled)
	}
	c3 := m.loadHashCache(root)
	rec1 := c3.entries[rel1]
	if rec1.size != fi1.Size() {
		t.Fatalf("size 失效重算后清单应更新为新 size（%d），实际 %d", fi1.Size(), rec1.size)
	}
	if want := sha256Hex([]byte(newContent)); rec1.hash != want {
		t.Fatalf("内容已变应重算出新 hash（%s），实际 %s", want, rec1.hash)
	}
	if rec1.hash == base.entries[rel1].hash {
		t.Fatalf("内容变化后 hash 不应仍与旧值一致")
	}
}

// —— 退出标准③：压实态 ——

// TestHashCacheCompactionDropsVanishedPaths 路径消失（整作品移出本轮选择且磁盘
// 文件删除）→ 正常收尾回写后清单不含该路径；本轮流见集合即存在性事实（决策2
// 压实口径——未流见路径不保留）。
func TestHashCacheCompactionDropsVanishedPaths(t *testing.T) {
	root, h, m, _, sess, works := newParallelEmitFixture(t, 3, 2, emitDirectCfg{})
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("首轮全量创建不应中止: %s / %v", reason, cancelled)
	}
	if got := len(m.loadHashCache(root).entries); got != 6 {
		t.Fatalf("首轮回写后清单应有 6 条记录，实际 %d", got)
	}

	// 末作品移出本轮选择 + 其文件从磁盘消失（在场选择不受影响、创建照常成功）
	for _, rel := range works[5].Files {
		if err := removeFile(t, root+"/"+rel); err != nil {
			t.Fatalf("删除消失路径失败: %v", err)
		}
	}
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works[:5], sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("部分选择轮不应中止: %s / %v", reason, cancelled)
	}

	c := m.loadHashCache(root)
	if got := len(c.entries); got != 5 {
		t.Fatalf("压实回写后清单应只含流见 5 条，实际 %d", got)
	}
	for _, rel := range works[5].Files {
		if _, ok := c.entries[rel]; ok {
			t.Fatalf("消失路径 %s 应被压实剔除", rel)
		}
	}
	for _, work := range works[:5] {
		for _, rel := range work.Files {
			if _, ok := c.entries[rel]; !ok {
				t.Fatalf("流见路径 %s 应保留", rel)
			}
		}
	}
}

// —— 退出标准④：损坏态 ——

// TestHashCacheCorruptDigestColdStartAndSelfHeal 分片摘要不符→按空缓存冷启
//（不报错炸链：创建正常收尾）；冷启由探针证明（等长改写+mtime 还原——旧清单
// 若可读必复用旧 hash，冷启必得新内容 hash）；回写自愈=旧批次分片清理+新清单
// 建立，第三轮探针复用二轮 hash（全链命中回路锚，①的 emit 级证明）。
func TestHashCacheCorruptDigestColdStartAndSelfHeal(t *testing.T) {
	root, h, m, ctx, sess, works := newParallelEmitFixture(t, 2, 2, emitDirectCfg{})
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("首轮创建不应中止: %s / %v", reason, cancelled)
	}
	norm := normalizeLibraryRoot(root)
	env := mustReadHashCacheEnvelope(t, ctx.store, root)
	oldBatch := env.ShardSet

	// 篡改首分片（base64 首字符翻转：长度不变、内容变化 → 摘要不符）
	firstKey := hashCacheShardKey(norm, oldBatch, 0)
	stored, err := ctx.store.GetValue(firstKey)
	if err != nil || stored == nil || stored.Value == "" {
		t.Fatalf("读取待篡改分片失败: %v %+v", err, stored)
	}
	flip := byte('A')
	if stored.Value[0] == 'A' {
		flip = 'B'
	}
	if err := ctx.store.SetValue(firstKey, string(flip)+stored.Value[1:]); err != nil {
		t.Fatalf("写回篡改分片失败: %v", err)
	}

	// 探针：等长改写+mtime 还原——冷启与否的判别面
	relA := works[0].Files[0]
	newContent := sameSizeRewriteKeepMtime(t, root+"/"+relA, "Y")

	// ④ 损坏态：按空缓存冷启，不报错炸链（收尾正常、全量重算）
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("损坏冷启轮不应中止: %s / %v", reason, cancelled)
	}
	c := m.loadHashCache(root)
	if got := len(c.entries); got != 4 {
		t.Fatalf("冷启重算后清单应重建 4 条记录，实际 %d", got)
	}
	if got := c.entries[relA].hash; got != sha256Hex([]byte(newContent)) {
		t.Fatalf("冷启应重算探针文件新内容 hash（%s），实际 %s（旧清单被复用=未冷启）",
			sha256Hex([]byte(newContent)), got)
	}

	// 自愈：信封切至新批次，旧批次分片全清（残留垃圾即未自愈）
	env2 := mustReadHashCacheEnvelope(t, ctx.store, root)
	if env2.ShardSet == oldBatch {
		t.Fatalf("回写后信封应切至新批次（%q 仍在指向旧批次）", env2.ShardSet)
	}
	if got := len(hashCacheShardKeysOfRoot(t, ctx.store, root)); got != env2.Shards {
		t.Fatalf("自愈后分片键数 %d 应与信封 shards %d 一致（旧批次应全清）", got, env2.Shards)
	}

	// ① 全链命中回路锚：三轮探针（再次等长改写+mtime 还原）必复用二轮 hash
	sameSizeRewriteKeepMtime(t, root+"/"+relA, "Z")
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("三轮命中轮不应中止: %s / %v", reason, cancelled)
	}
	c3 := m.loadHashCache(root)
	if got := c3.entries[relA].hash; got != sha256Hex([]byte(newContent)) {
		t.Fatalf("三轮应命中二轮清单复用 hash（%s），实际 %s（发生读盘重算）",
			sha256Hex([]byte(newContent)), got)
	}
}

// TestHashCacheLoadCorruptVariants 载入级损坏形态表：分片缺失/信封脏值/版本
// 不符/编码簿记不完整/清单畸形/传输层失败——前五者按空缓存冷启（回写可自愈），
// 传输层失败额外放弃回写（loadFailed：半知状态不得覆写既有缓存）。
func TestHashCacheLoadCorruptVariants(t *testing.T) {
	validManifest := encodeHashCacheManifest(map[string]hashCacheRecord{
		"画集/a.jpg": {size: 10, mtimeNs: 20, hash: strings.Repeat("ab", 32)},
	})
	fabricate := func(t *testing.T, store *fakeSessionStore, root string, payload []byte, mutate func(*hashCacheEnvelope)) *hashCacheEnvelope {
		t.Helper()
		norm := normalizeLibraryRoot(root)
		var compressed []byte
		var digest string
		if payload != nil {
			var err error
			if compressed, digest, err = gzipWithDigest(payload); err != nil {
				t.Fatalf("构造载荷失败: %v", err)
			}
		}
		batch := "1700000000000"
		shards := splitSessionShards(compressed)
		for i, chunk := range shards {
			if err := store.SetValue(hashCacheShardKey(norm, batch, i), base64.StdEncoding.EncodeToString(chunk)); err != nil {
				t.Fatalf("构造分片失败: %v", err)
			}
		}
		env := &hashCacheEnvelope{V: hashCacheEnvelopeVersion, ShardSet: batch,
			Shards: len(shards), Codec: sessionPayloadCodecGzip, Digest: digest, Entries: 1}
		if len(shards) == 0 {
			env.Codec, env.Digest = "", ""
		}
		if mutate != nil {
			mutate(env)
		}
		data, err := json.Marshal(env)
		if err != nil {
			t.Fatalf("构造信封失败: %v", err)
		}
		if err := store.SetValue(hashCacheKey(root), string(data)); err != nil {
			t.Fatalf("写信封失败: %v", err)
		}
		return env
	}

	t.Run("合法往返（正控）", func(t *testing.T) {
		store := newFakeSessionStore()
		m := NewSessionManagerWithStore(nil, nil, store)
		root := `D:\缓存正控`
		fabricate(t, store, root, validManifest, nil)
		c := m.loadHashCache(root)
		if len(c.entries) != 1 || c.loadFailed {
			t.Fatalf("合法清单应载入 1 条且非失败态，实际 entries=%d loadFailed=%v", len(c.entries), c.loadFailed)
		}
		rec := c.entries["画集/a.jpg"]
		if rec.size != 10 || rec.mtimeNs != 20 || rec.hash != strings.Repeat("ab", 32) {
			t.Fatalf("往返记录不符: %+v", rec)
		}
	})
	t.Run("摘要不符 → 空缓存冷启", func(t *testing.T) {
		store := newFakeSessionStore()
		logger := &capturingLogger{}
		m := NewSessionManagerWithStore(nil, logger, store)
		root := `D:\缓存篡改`
		env := fabricate(t, store, root, validManifest, nil)
		firstKey := hashCacheShardKey(normalizeLibraryRoot(root), env.ShardSet, 0)
		stored, _ := store.GetValue(firstKey)
		if err := store.SetValue(firstKey, "B"+stored.Value[1:]); err != nil {
			t.Fatalf("篡改分片失败: %v", err)
		}
		c := m.loadHashCache(root)
		if len(c.entries) != 0 || c.loadFailed {
			t.Fatalf("摘要不符应按空缓存冷启（entries=%d loadFailed=%v）", len(c.entries), c.loadFailed)
		}
		if !logger.warnedContaining("摘要不符") {
			t.Fatalf("摘要不符应有告警，实际: %v", logger.warns)
		}
	})
	t.Run("分片缺失 → 空缓存冷启", func(t *testing.T) {
		store := newFakeSessionStore()
		logger := &capturingLogger{}
		m := NewSessionManagerWithStore(nil, logger, store)
		root := `D:\缓存缺片`
		env := fabricate(t, store, root, validManifest, nil)
		if err := store.DeleteValue(hashCacheShardKey(normalizeLibraryRoot(root), env.ShardSet, 0)); err != nil {
			t.Fatalf("删除分片失败: %v", err)
		}
		c := m.loadHashCache(root)
		if len(c.entries) != 0 || c.loadFailed {
			t.Fatalf("分片缺失应按空缓存冷启（entries=%d loadFailed=%v）", len(c.entries), c.loadFailed)
		}
		if !logger.warnedContaining("分片缺失") {
			t.Fatalf("分片缺失应有告警，实际: %v", logger.warns)
		}
	})
	t.Run("信封版本不符 → 空缓存冷启", func(t *testing.T) {
		store := newFakeSessionStore()
		logger := &capturingLogger{}
		m := NewSessionManagerWithStore(nil, logger, store)
		root := `D:\缓存版本`
		fabricate(t, store, root, validManifest, func(env *hashCacheEnvelope) { env.V = hashCacheEnvelopeVersion + 99 })
		c := m.loadHashCache(root)
		if len(c.entries) != 0 {
			t.Fatalf("版本不符应按空缓存冷启，实际 entries=%d", len(c.entries))
		}
		if !logger.warnedContaining("版本不符") {
			t.Fatalf("版本不符应有告警，实际: %v", logger.warns)
		}
	})
	t.Run("信封脏值（非信封形态）→ 空缓存冷启", func(t *testing.T) {
		store := newFakeSessionStore()
		logger := &capturingLogger{}
		m := NewSessionManagerWithStore(nil, logger, store)
		root := `D:\缓存脏值`
		if err := store.SetValue(hashCacheKey(root), "###not-json###"); err != nil {
			t.Fatalf("写脏值失败: %v", err)
		}
		c := m.loadHashCache(root)
		if len(c.entries) != 0 || c.loadFailed {
			t.Fatalf("脏值应按空缓存冷启（entries=%d loadFailed=%v）", len(c.entries), c.loadFailed)
		}
		if !logger.warnedContaining("非信封形态") {
			t.Fatalf("脏值应有告警，实际: %v", logger.warns)
		}
	})
	t.Run("清单畸形（gzip 合法而内容非清单）→ 空缓存冷启", func(t *testing.T) {
		store := newFakeSessionStore()
		logger := &capturingLogger{}
		m := NewSessionManagerWithStore(nil, logger, store)
		root := `D:\缓存畸形`
		fabricate(t, store, root, []byte("garbage-line-no-tabs"), nil)
		c := m.loadHashCache(root)
		if len(c.entries) != 0 || c.loadFailed {
			t.Fatalf("畸形清单应按空缓存冷启（entries=%d loadFailed=%v）", len(c.entries), c.loadFailed)
		}
		if !logger.warnedContaining("清单无法解析") {
			t.Fatalf("畸形清单应有告警，实际: %v", logger.warns)
		}
	})
	t.Run("传输层读取失败 → 空缓存且放弃回写", func(t *testing.T) {
		store := newFailingHashCacheStore()
		m := NewSessionManagerWithStore(nil, nil, store)
		root := `D:\缓存断连`
		c := m.loadHashCache(root)
		if len(c.entries) != 0 || !c.loadFailed {
			t.Fatalf("传输失败应空缓存且 loadFailed（entries=%d loadFailed=%v）", len(c.entries), c.loadFailed)
		}
		// 放弃回写：带新算记录回写也不落任何键（半知状态不覆写）
		c.record("画集/b.jpg", 1, 2, strings.Repeat("cd", 32))
		m.saveHashCache(root, c, true)
		all, err := store.GetAllValues()
		if err != nil {
			t.Fatalf("枚举 KV 失败: %v", err)
		}
		for key := range all {
			if strings.HasPrefix(key, hashCacheKeyPrefix) || strings.HasPrefix(key, hashCacheSnapKeyPrefix) {
				t.Fatalf("loadFailed 下回写不得落键，实得 %q", key)
			}
		}
	})
}

// newFailingHashCacheStore 信封键读取恒失败的存储桩（传输层失败注入面）。
func newFailingHashCacheStore() *failingHashCacheStore {
	return &failingHashCacheStore{fakeSessionStore: newFakeSessionStore()}
}

type failingHashCacheStore struct {
	*fakeSessionStore
}

func (s *failingHashCacheStore) GetValue(key string) (*sdkdto.StorageValue, error) {
	if strings.HasPrefix(key, hashCacheKeyPrefix) {
		return nil, fmt.Errorf("模拟传输层断连（键 %q）", key)
	}
	return s.fakeSessionStore.GetValue(key)
}

// —— 中止合并回写（取消→重试命中面，交互剧本帧4 同源语义）——

// TestHashCacheAbortMergeKeepsEntries 中止收尾不压实：合并保留未流见旧条目并
// 叠加本轮新算记录。失败注入取确定性形态——并发钉死 1（单元串行），第三作品
// 文件删除 → 域序放行至该单元时 Stat 中断（对账拆分·甲2 刀4），首作品全命中、
// 次作品先哈希入账再中断（记录先于结论投递，放行序保证入账必达）。
func TestHashCacheAbortMergeKeepsEntries(t *testing.T) {
	root, h, m, _, sess, works := newParallelEmitFixture(t, 2, 2, emitDirectCfg{pinned: 1})
	// 基线：仅首作品成功 → 压实回写 1 条
	if _, reason, cancelled := runEmitDirect(t, h, root, sess, works[:1], sdkdto.NoopHeartbeat{}); reason != "" || cancelled {
		t.Fatalf("基线创建不应中止: %s / %v", reason, cancelled)
	}
	if got := len(m.loadHashCache(root).entries); got != 1 {
		t.Fatalf("基线回写后清单应有 1 条记录，实际 %d", got)
	}

	// 中止轮：全量发起，第三作品文件消失 → 整次创建中断（对账文案收流）
	if err := removeFile(t, root+"/"+works[2].Files[0]); err != nil {
		t.Fatalf("删除中断文件失败: %v", err)
	}
	_, reason, cancelled := runEmitDirect(t, h, root, sess, works, sdkdto.NoopHeartbeat{})
	if cancelled || !strings.Contains(reason, "磁盘对账未通过") {
		t.Fatalf("中断作品应整次中止并对账文案收流，实际 cancelled=%v reason=%q", cancelled, reason)
	}

	// 合并回写：基线 1 条保留（未流见不剔除）+ 次作品新算叠加；中断作品文件缺席
	c := m.loadHashCache(root)
	if got := len(c.entries); got != 2 {
		t.Fatalf("中止合并后清单应为基线 1 + 新算 1 = 2 条，实际 %d: %v", got, c.entries)
	}
	for _, rel := range works[0].Files {
		if _, ok := c.entries[rel]; !ok {
			t.Fatalf("未流见基线条目 %s 应合并保留", rel)
		}
	}
	if _, ok := c.entries[works[1].Files[0]]; !ok {
		t.Fatalf("中止前已哈希入账的 %s 应叠加进清单", works[1].Files[0])
	}
	if _, ok := c.entries[works[2].Files[0]]; ok {
		t.Fatalf("中断文件 %s 从未成功哈希，不得入账", works[2].Files[0])
	}
}

// —— 持久化布局 ——

// TestHashCachePersistenceShardLayout 信封单键+gzip 分片键布局（会话分片三件套
// 同构复用）：条目数/分片数簿记一致、二次回写批次原子切换（旧批次清理）；全
// 命中零新算的压实回写为 no-op（不重写清单、不切批次）。
func TestHashCachePersistenceShardLayout(t *testing.T) {
	store := newCountingSessionStore()
	m := NewSessionManagerWithStore(nil, nil, store)
	root := `D:\缓存布局`

	cache := newHashCache()
	for i := 0; i < 3; i++ {
		cache.record(fmt.Sprintf("画集/图%02d.jpg", i), int64(100+i), int64(1000+i),
			strings.Repeat(fmt.Sprintf("%02x", i), 32))
	}
	m.saveHashCache(root, cache, true)

	env := mustReadHashCacheEnvelope(t, store, root)
	if env.Entries != 3 || env.Shards != 1 || env.Codec != sessionPayloadCodecGzip || env.Digest == "" {
		t.Fatalf("信封簿记不符: %+v", env)
	}
	if got := len(hashCacheShardKeysOfRoot(t, store, root)); got != env.Shards {
		t.Fatalf("分片键数 %d 应与信封 shards %d 一致", got, env.Shards)
	}

	// 二次回写：重载+全量命中标注（lookup 即流见记账，真实轮次每文件必经其一）
	//+改一条 → 批次原子切换（旧批次清理、新批次就位、更新生效、未触及条目按
	// 流见保留
	c2 := m.loadHashCache(root)
	for i := 1; i <= 2; i++ {
		rel := fmt.Sprintf("画集/图%02d.jpg", i)
		if hash, hit := c2.lookup(rel, int64(100+i), int64(1000+i)); !hit || hash != strings.Repeat(fmt.Sprintf("%02x", i), 32) {
			t.Fatalf("未触及条目 %s 应命中原值", rel)
		}
	}
	c2.record("画集/图00.jpg", 999, 9999, strings.Repeat("ff", 32))
	m.saveHashCache(root, c2, true)
	env2 := mustReadHashCacheEnvelope(t, store, root)
	if env2.ShardSet == env.ShardSet {
		t.Fatalf("二次回写应切至新批次（仍指向 %q）", env2.ShardSet)
	}
	if got := len(hashCacheShardKeysOfRoot(t, store, root)); got != env2.Shards {
		t.Fatalf("切换后分片键数 %d 应与新信封 shards %d 一致（旧批次应全清）", got, env2.Shards)
	}
	c3 := m.loadHashCache(root)
	if rec := c3.entries["画集/图00.jpg"]; rec.size != 999 || rec.hash != strings.Repeat("ff", 32) {
		t.Fatalf("新算记录应覆盖旧值: %+v", rec)
	}
	if rec, ok := c3.entries["画集/图02.jpg"]; !ok || rec.size != 102 {
		t.Fatalf("未触及条目应原样保留: %+v ok=%v", rec, ok)
	}

	// 全命中零新算的压实回写为 no-op：全量命中标注（真实全命中轮的载入态
	//——每文件均 lookup 命中而零新算）→ 批次不切、分片不写（免 10MB 级整包重写）
	store.resetWriteCounts()
	c4 := m.loadHashCache(root)
	for _, probe := range []struct {
		rel       string
		size      int64
		mtimeNs   int64
	}{{"画集/图00.jpg", 999, 9999}, {"画集/图01.jpg", 101, 1001}, {"画集/图02.jpg", 102, 1002}} {
		if _, hit := c4.lookup(probe.rel, probe.size, probe.mtimeNs); !hit {
			t.Fatalf("条目 %s 应命中（全命中轮标注）", probe.rel)
		}
	}
	m.saveHashCache(root, c4, true)
	if env3 := mustReadHashCacheEnvelope(t, store, root); env3.ShardSet != env2.ShardSet {
		t.Fatalf("全命中零新算的回写不得切换批次")
	}
	store.mu.Lock()
	setCalls := len(store.setValueHits)
	store.mu.Unlock()
	if setCalls != 0 {
		t.Fatalf("全命中零新算的回写应为 no-op（SetValue 调用 %d 次）", setCalls)
	}
}
