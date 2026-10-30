// 哈希缓存域（大选择集创建链加速 · 决策2）：按库根打包单键的逐文件哈希缓存。
// 清单=relPath+size+mtime+hash；gzip 分片复用会话分片三件套基建（信封单键+
// gzip 分片键+批次原子切换，session_payload.go——三件套中的会话索引簿记为会话
// 枚举专用加速层，哈希缓存无枚举消费面，不设索引）。禁逐文件键：15 万键 ×
// 单键 RPC=分钟级且 KV 面无前缀扫描（声明10）。
//
// 生命周期（决策2 拍板形态）：verify 开始一次载入内存（loadHashCache，emitSelectedWorks
// 起始调用）；创建结束一次回写+压实（saveHashCache，emitSelectedWorks 的 defer
// 覆盖全部收尾路径）——正常收尾按本轮流见集合压实（本轮流见集合即存在性事实：
// 未流见的路径不保留），中止/取消合并保留旧条目并叠加新算记录（流见集合残缺，
// 压实会误删未核对条目；保留面=取消→重试的命中语义，交互剧本帧4「重试时核对
// 文件相因哈希缓存命中而接近秒级跳过」）。
//
// 命中口径：relPath+size+mtime 全符才复用 hash（size/mtime 任一不符即失效重算）；
// 命中路径不经 ComputeFileHashCtx、零读盘字节进爬坡控制器窗口（风险5：noteReadBytes
// 是窗口唯一进数口，计入会令控制器被虚高吞吐误导而误升档）。分片摘要不符按空
// 缓存冷启（不报错炸链），下次成功回写自愈。
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
)

// —— 键布局（前缀与会话/分片/标记键互不重叠，回退扫描与会话索引均不误读）——

const (
	// hashCacheKeyPrefix 哈希缓存信封键前缀（后接归一化库根）——键命名参照
	// workbench-create-marker: 外置单键先例。
	hashCacheKeyPrefix = "hash-cache:"
	// hashCacheSnapKeyPrefix 分片键前缀：独立于信封键前缀（键内 ':' 的位置区分
	// 两者，与 workbench-session:/workbench-session-snap: 同构），信封前缀扫描
	// 不会命中分片键。
	hashCacheSnapKeyPrefix = "hash-cache-snap:"
	// hashCacheEnvelopeVersion 信封结构版本（自管；不符按空缓存冷启）。
	hashCacheEnvelopeVersion = 1
)

// hashCacheKey 库根 → 哈希缓存信封 KV 键（归一与会话键同规则）。
func hashCacheKey(root string) string {
	return hashCacheKeyPrefix + normalizeLibraryRoot(root)
}

// hashCacheShardKey 归一化库根 + 批次号 + 片序 → 哈希缓存分片 KV 键
//（形态与 sessionShardKey 同构，前缀独立）。
func hashCacheShardKey(normRoot, batch string, i int) string {
	return hashCacheSnapKeyPrefix + normRoot + "@" + batch + "#" + strconv.Itoa(i)
}

// hashCacheEnvelope 信封键的值形态：载荷簿记与条目计数（轻量，无会话字段）。
// ShardSet 不带 omitempty——字段在场性是信封探测判据（probeSessionEnvelope
// 共用：探测顶层 shardSet 字段）。
type hashCacheEnvelope struct {
	// V 信封结构版本（自管）。
	V int `json:"v"`
	// ShardSet 分片批次号（保存时点 UnixMilli；空=零条目形态 shards=0）。
	ShardSet string `json:"shardSet"`
	// Shards 分片数（0=空清单）。
	Shards int `json:"shards"`
	// Codec 载荷压缩编码（"gzip"；空清单为空）。
	Codec string `json:"codec,omitempty"`
	// Digest 压缩载荷字节的 sha256 hex（读侧完整性校验；空清单为空）。
	Digest string `json:"digest,omitempty"`
	// Entries 清单条目数（观测面：载入/回写日志与容量核算）。
	Entries int `json:"entries"`
}

// parseHashCacheEnvelope 解析信封值。
func parseHashCacheEnvelope(value string) (*hashCacheEnvelope, error) {
	var env hashCacheEnvelope
	if err := json.Unmarshal([]byte(value), &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// —— 运行期缓存（生命周期=一次创建的 verify 相，emitSelectedWorks 持有）——

// hashCacheRecord 单文件缓存记录（编码从紧，风险2）：size 与 mtimeNs（UnixNano，
// 保留 FS 100ns 粒度防同毫秒碰撞）为命中判据，hash 为复用值（64 hex）。
type hashCacheRecord struct {
	size    int64
	mtimeNs int64
	hash    string
}

// hashCache 运行期哈希缓存：entries 为载入清单（载入后只读——哈希 worker 无锁
// 并发读）；fresh/seen 由 worker 并发写（mu 保护）。seen=本轮流见集合（经
// Stat+哈希确认在场的路径，压实口径），fresh=本轮新算记录（未命中算后记入的
// 回写集）。
type hashCache struct {
	normRoot string
	entries  map[string]hashCacheRecord
	// staleEnv 载入时点的旧信封簿记（可解析即保留——含 digest 不符的损坏态）：
	// 回写切新批次后据此删除旧批次分片键（半写/损坏残留的自愈清理面）。
	staleEnv *hashCacheEnvelope
	// loadFailed 载入传输层失败：真=放弃回写（半知状态覆写既有好缓存的风险
	// 防线；本轮仍可作空缓存命中判据——只是不回写）。
	loadFailed bool

	mu    sync.Mutex
	fresh map[string]hashCacheRecord
	seen  map[string]struct{}
}

// newHashCache 空缓存（冷启形态）。
func newHashCache() *hashCache {
	return &hashCache{
		entries: make(map[string]hashCacheRecord),
		fresh:   make(map[string]hashCacheRecord),
		seen:    make(map[string]struct{}),
	}
}

// lookup 命中判定：relPath+size+mtime 全符才复用 hash（决策2：size/mtime 任一
// 不符即失效重算）。命中同步把路径记入流见集合（压实口径——命中即在场的
// 存在性证明）。entries 载入后只读，无锁并发读安全。
func (c *hashCache) lookup(rel string, size, mtimeNs int64) (string, bool) {
	rec, ok := c.entries[rel]
	if !ok || rec.size != size || rec.mtimeNs != mtimeNs {
		return "", false
	}
	c.mu.Lock()
	c.seen[rel] = struct{}{}
	c.mu.Unlock()
	return rec.hash, true
}

// record 未命中算后记入：新算记录进回写集、路径进流见集合（哈希 worker 并发
// 调用；记录先于结论投递，放行序保证中止前已放行单元必已记录）。
func (c *hashCache) record(rel string, size, mtimeNs int64, hash string) {
	c.mu.Lock()
	c.fresh[rel] = hashCacheRecord{size: size, mtimeNs: mtimeNs, hash: hash}
	c.seen[rel] = struct{}{}
	c.mu.Unlock()
}

// recordsForWriteBack 回写清单裁决：compact 真=按本轮流见集合压实（流见集合即
// 存在性事实——命中取 entries 记录、新算取 fresh 记录，未流见路径剔除）；假=
// 合并保留（中止/取消路径：流见集合残缺，压实会误删未核对条目；新算记录
// 覆盖同路径旧值）。
func (c *hashCache) recordsForWriteBack(compact bool) map[string]hashCacheRecord {
	c.mu.Lock()
	fresh, seen := c.fresh, c.seen
	c.mu.Unlock()
	if compact {
		out := make(map[string]hashCacheRecord, len(seen))
		for rel := range seen {
			if rec, ok := fresh[rel]; ok {
				out[rel] = rec
				continue
			}
			if rec, ok := c.entries[rel]; ok {
				out[rel] = rec
			}
		}
		return out
	}
	out := make(map[string]hashCacheRecord, len(c.entries)+len(fresh))
	for rel, rec := range c.entries {
		out[rel] = rec
	}
	for rel, rec := range fresh {
		out[rel] = rec
	}
	return out
}

// freshCount 新算记录数（回写 no-op 守卫）。
func (c *hashCache) freshCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.fresh)
}

// seenCount 流见集合大小（回写 no-op 守卫）。
func (c *hashCache) seenCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}

// —— 清单编解码（编码从紧，风险2：逐行定界文本 + 路径转义，流式解码）——

// encodeHashCacheManifest 清单编码：每行 `{size}\t{mtimeNs}\t{hash}\t{转义路径}`，
// 按路径字典序输出（确定性编码；排序后相邻路径前缀共享，对 gzip 友好）。路径
// 域为正斜杠 relPath，仅转义分隔符冲突字符（\\ \t \n \r）——常规路径近零开销。
func encodeHashCacheManifest(entries map[string]hashCacheRecord) []byte {
	rels := make([]string, 0, len(entries))
	for rel := range entries {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	var buf bytes.Buffer
	for _, rel := range rels {
		rec := entries[rel]
		buf.WriteString(strconv.FormatInt(rec.size, 10))
		buf.WriteByte('\t')
		buf.WriteString(strconv.FormatInt(rec.mtimeNs, 10))
		buf.WriteByte('\t')
		buf.WriteString(rec.hash)
		buf.WriteByte('\t')
		writeHashCachePath(&buf, rel)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// writeHashCachePath 转义写路径段：\\ \t \n \r 四字符（制表分隔符与行界冲突面；
// 其余字节原样——含中文与空格）。
func writeHashCachePath(buf *bytes.Buffer, rel string) {
	for i := 0; i < len(rel); i++ {
		switch rel[i] {
		case '\\':
			buf.WriteString(`\\`)
		case '\t':
			buf.WriteString(`\t`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		default:
			buf.WriteByte(rel[i])
		}
	}
}

// readHashCachePath 反转义路径段（writeHashCachePath 的逆；孤立反斜杠按损坏报错）。
func readHashCachePath(s string) (string, error) {
	var b bytes.Buffer
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			return "", fmt.Errorf("路径转义序列不完整: %q", s)
		}
		switch s[i] {
		case '\\':
			b.WriteByte('\\')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			return "", fmt.Errorf("未知路径转义序列 %q", s[:i+1])
		}
	}
	return b.String(), nil
}

// decodeHashCacheManifest 清单解码：逐行流式解析（15 万级清单不整包反序列化，
// 内存峰值=结果映射本身，风险2）；空行容忍（末行换行形态），任一实质行畸形
// 按整体损坏处理（调用方冷启）。
func decodeHashCacheManifest(data []byte) (map[string]hashCacheRecord, error) {
	out := make(map[string]hashCacheRecord)
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20) // 单行上限 1MB（防畸形超长行撑爆内存）
	line := 0
	for sc.Scan() {
		line++
		row := sc.Bytes()
		if len(row) == 0 {
			continue
		}
		parts := bytes.Split(row, []byte{'\t'})
		if len(parts) != 4 {
			return nil, fmt.Errorf("第 %d 行字段数不符（%d）", line, len(parts))
		}
		size, err := strconv.ParseInt(string(parts[0]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行 size 无法解析: %v", line, err)
		}
		mtimeNs, err := strconv.ParseInt(string(parts[1]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行 mtime 无法解析: %v", line, err)
		}
		hash := string(parts[2])
		if len(hash) != 64 {
			return nil, fmt.Errorf("第 %d 行哈希长度不符（%d）", line, len(hash))
		}
		rel, err := readHashCachePath(string(parts[3]))
		if err != nil {
			return nil, fmt.Errorf("第 %d 行路径无法解析: %v", line, err)
		}
		if rel == "" {
			return nil, fmt.Errorf("第 %d 行路径为空", line)
		}
		out[rel] = hashCacheRecord{size: size, mtimeNs: mtimeNs, hash: hash}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// —— 载入与回写（SessionManager 注入的 store 读写；同 create-marker 先例挂
// SessionManager，方法落位本文件）——

// loadHashCache 载入库根的哈希缓存（verify 开始一次调用，决策2）：
//   - 缺席=空缓存冷启（首轮正常形态）；
//   - 损坏（信封不可解析/版本不符/分片缺失/摘要不符/清单畸形）=告警+空缓存
//     冷启，不报错炸链（④态）；可解析的旧信封簿记保留（含摘要不符态）——
//     回写切新批次后据此清理旧分片键；
//   - 传输层读取失败=告警+空缓存且放弃回写（loadFailed：半知状态不得覆写
//     既有好缓存）。
//
// 不取 m.mu：哈希缓存键组独立于会话键组，与会话读写无互斥需求；分片读在
// 锁外逐键进行（单创建在跑，同库根无并发载入）。
func (m *SessionManager) loadHashCache(root string) *hashCache {
	cache := newHashCache()
	cache.normRoot = normalizeLibraryRoot(root)
	key := hashCacheKey(root)
	stored, err := m.store.GetValue(key)
	if err != nil {
		m.warnf("读取哈希缓存失败（键 %q），本轮按空缓存冷启且不回写: %v", key, err)
		cache.loadFailed = true
		return cache
	}
	if stored == nil || stored.Value == "" {
		return cache // 从未写过：空冷启
	}
	if !probeSessionEnvelope(stored.Value) {
		m.warnf("哈希缓存值非信封形态（键 %q），按空缓存冷启（回写自愈）", key)
		return cache
	}
	env, perr := parseHashCacheEnvelope(stored.Value)
	if perr != nil || env.V != hashCacheEnvelopeVersion {
		m.warnf("哈希缓存信封无法解析或版本不符（键 %q），按空缓存冷启: %v", key, perr)
		return cache
	}
	cache.staleEnv = env
	if env.Shards <= 0 {
		return cache // 空清单形态（上次回写零条目）
	}
	if env.Codec != sessionPayloadCodecGzip || env.Digest == "" {
		m.warnf("哈希缓存信封载荷簿记不完整（键 %q，codec=%q），按空缓存冷启", key, env.Codec)
		return cache
	}
	compressed := make([]byte, 0, env.Shards*sessionShardRawUnit)
	for i := 0; i < env.Shards; i++ {
		shardKey := hashCacheShardKey(cache.normRoot, env.ShardSet, i)
		shard, serr := m.store.GetValue(shardKey)
		if serr != nil {
			m.warnf("读取哈希缓存分片失败（键 %q），按空缓存冷启且不回写: %v", shardKey, serr)
			cache.loadFailed = true
			return cache
		}
		if shard == nil || shard.Value == "" {
			m.warnf("哈希缓存分片缺失（键 %q），按空缓存冷启（可能为半写残留）", shardKey)
			return cache
		}
		chunk, derr := base64.StdEncoding.DecodeString(shard.Value)
		if derr != nil {
			m.warnf("哈希缓存分片无法解码（键 %q），按空缓存冷启: %v", shardKey, derr)
			return cache
		}
		compressed = append(compressed, chunk...)
	}
	sum := sha256.Sum256(compressed)
	if hex.EncodeToString(sum[:]) != env.Digest {
		m.warnf("哈希缓存分片摘要不符（键 %q），按空缓存冷启（回写自愈）", key)
		return cache
	}
	raw, gerr := gunzipAll(compressed)
	if gerr != nil {
		m.warnf("哈希缓存载荷解压失败（键 %q），按空缓存冷启: %v", key, gerr)
		return cache
	}
	entries, derr := decodeHashCacheManifest(raw)
	if derr != nil {
		m.warnf("哈希缓存清单无法解析（键 %q），按空缓存冷启: %v", key, derr)
		return cache
	}
	cache.entries = entries
	return cache
}

// saveHashCache 回写哈希缓存（创建结束一次调用，决策2；compact=正常收尾按
// 流见集合压实，假=中止/取消合并保留旧条目）。全程尽力而为：任何失败仅告警
// 不向上传错——加速器永不阻断创建（与爬坡控制器同纪律）。写形态复用会话全
// 保存的批次原子切换：逐片写新批次分片键 → 写信封（原子切换点）→ 删旧批次
// 分片；任一分片写失败即中止（旧信封未动，旧缓存可读；已成分片为残留垃圾键
// 不影响正确性）。零条目回写为合法形态（信封 shards=0，载荷不写分片）。
func (m *SessionManager) saveHashCache(root string, cache *hashCache, compact bool) {
	if cache == nil || cache.loadFailed {
		return
	}
	if !compact && cache.freshCount() == 0 {
		return // 中止且无新算记录：无内容可叠加，免整包重写
	}
	if compact && cache.freshCount() == 0 && len(cache.entries) > 0 && cache.seenCount() == len(cache.entries) {
		return // 全命中零新算且清单无出入：回写产物与载入态全等，免整包重写
		//（纯命中重试〔帧4 秒级路径〕不再付 10MB 级清单重编码与旧批次切换）
	}
	records := cache.recordsForWriteBack(compact)
	norm := normalizeLibraryRoot(root)
	env := &hashCacheEnvelope{V: hashCacheEnvelopeVersion, Entries: len(records)}
	if len(records) > 0 {
		compressed, digest, err := gzipWithDigest(encodeHashCacheManifest(records))
		if err != nil {
			m.warnf("哈希缓存清单编码失败（root=%s）: %v", root, err)
			return
		}
		oldBatch := int64(0)
		if cache.staleEnv != nil {
			oldBatch = parseSessionShardBatch(cache.staleEnv.ShardSet)
		}
		batch := nextSessionShardBatch(oldBatch)
		shards := splitSessionShards(compressed)
		for i, chunk := range shards {
			key := hashCacheShardKey(norm, batch, i)
			if err := m.store.SetValue(key, base64.StdEncoding.EncodeToString(chunk)); err != nil {
				m.warnf("写入哈希缓存分片失败（键 %q，旧缓存不受影响）: %v", key, err)
				return
			}
		}
		env.ShardSet, env.Shards = batch, len(shards)
		env.Codec, env.Digest = sessionPayloadCodecGzip, digest
	}
	data, err := json.Marshal(env)
	if err != nil {
		m.warnf("哈希缓存信封序列化失败（root=%s）: %v", root, err)
		return
	}
	if err := m.store.SetValue(hashCacheKey(root), string(data)); err != nil {
		m.warnf("写入哈希缓存失败（键 %q）: %v", hashCacheKey(root), err)
		return
	}
	// 旧批次分片清理（信封已切换，旧片不可达；删除失败仅告警——残留垃圾键
	// 不影响正确性，下次回写再试）。零条目回写（env.ShardSet 空）同样清理旧片。
	if cache.staleEnv != nil && cache.staleEnv.ShardSet != "" && cache.staleEnv.Shards > 0 &&
		cache.staleEnv.ShardSet != env.ShardSet {
		for i := 0; i < cache.staleEnv.Shards; i++ {
			key := hashCacheShardKey(norm, cache.staleEnv.ShardSet, i)
			if err := m.store.DeleteValue(key); err != nil {
				m.warnf("删除旧哈希缓存分片失败（键 %q，残留垃圾不影响正确性）: %v", key, err)
			}
		}
	}
}
