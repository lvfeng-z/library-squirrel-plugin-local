// 会话载荷三件套持久化的格式与索引层：轻 meta（信封）+ gzip 分片载荷 + 会话索引。
// 本文件承载信封与分片的编解码、分片键/批次推导、旧 meta 簿记读取与索引维护——
// 全部经 SessionManager 注入的 store（SessionStore）读写，可单测注入内存存储。
// 会话增删改查入口与轻/全保存语义见 session.go。
//
// 键布局（三组独立键，前缀互不重叠）：
//   - meta 键：workbench-session:{归一化库根}（沿用旧格式键名），值=信封 JSON；
//   - 分片键：workbench-session-snap:{归一化库根}@{批次号}#{i}（i 从 0 起）；
//   - 索引键：workbench-session-index，值=归一化库根的 JSON 数组（升序）。
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// workbenchSessionSnapKeyPrefix 分片键前缀：独立于 meta 键前缀（workbench-session:），
// meta 前缀扫描不会命中分片键；回退扫描仍显式排除本前缀（防键布局未来漂移）。
const workbenchSessionSnapKeyPrefix = "workbench-session-snap:"

// workbenchSessionIndexKey 会话索引键：值=归一化库根的 JSON 数组（升序）。
const workbenchSessionIndexKey = "workbench-session-index"

const (
	// sessionEnvelopeVersion 信封结构版本（自管；不符按未命中处理）。
	sessionEnvelopeVersion = 1
	// sessionPayloadCodecGzip 载荷压缩编码。
	sessionPayloadCodecGzip = "gzip"
	// sessionShardMaxValue 单片最终文本值（base64 后）在 KV 中的尺寸上界（3MB）：
	// 单条 SetValue/GetValue 消息由此保持在 gRPC 默认 4MB 接收限内——本方案独立于
	// SDK 消息限额兜底也成立。
	sessionShardMaxValue = 3 << 20
	// sessionShardRawUnit 每片承载的压缩字节数（3MB × 3/4）：KV 通道为 proto3
	// string 字段（须为合法 UTF-8），分片值经 base64 编码（4/3 膨胀）落存——本值
	// 被 3 整除（无填充），编码后长度恰不超过 3MB。
	sessionShardRawUnit = sessionShardMaxValue / 4 * 3
)

// sessionEnvelope meta 键的值形态（信封）：持有会话轻字段（Snapshot/LastDiff 置
// nil 的会话投影）与载荷簿记。顶层 shardSet 字段的在场性是新信封与旧「单键单
// JSON」格式的探测判据（旧格式 WorkbenchSession JSON 无该字段）。
type sessionEnvelope struct {
	// V 信封结构版本（自管）。
	V int `json:"v"`
	// Session 载荷置空的完整会话（轻字段投影）。
	Session *WorkbenchSession `json:"session"`
	// ShardSet 分片批次号（保存时点 UnixMilli，同批分片共享；批次间保持单调，
	// 见 nextSessionShardBatch）。不带 omitempty：字段在场性是新/旧格式探测判据
	//（无快照会话序列化为空串但字段仍在场）。
	ShardSet string `json:"shardSet"`
	// Shards 分片数（0=无快照会话）。
	Shards int `json:"shards"`
	// Codec 载荷压缩编码（"gzip"；无快照为空）。
	Codec string `json:"codec,omitempty"`
	// Digest 压缩载荷字节的 sha256 hex（读侧完整性校验；无快照为空）。
	Digest string `json:"digest,omitempty"`
}

// probeSessionEnvelope 值是否为信封形态：顶层 shardSet 字段在场（旧格式整值
// JSON 与非 JSON 脏值均判否，走旧格式/损坏路径）。
func probeSessionEnvelope(value string) bool {
	var probe struct {
		ShardSet json.RawMessage `json:"shardSet"`
	}
	if err := json.Unmarshal([]byte(value), &probe); err != nil {
		return false
	}
	return len(probe.ShardSet) > 0
}

// parseSessionEnvelope 解析信封值。
func parseSessionEnvelope(value string) (*sessionEnvelope, error) {
	var env sessionEnvelope
	if err := json.Unmarshal([]byte(value), &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// sessionShardKey 归一化库根 + 批次号 + 片序 → 分片 KV 键。
func sessionShardKey(normRoot, batch string, i int) string {
	return workbenchSessionSnapKeyPrefix + normRoot + "@" + batch + "#" + strconv.Itoa(i)
}

// sessionLightCopy 会话 → 轻字段投影（重载荷置 nil）——信封 Session 字段的入存形态。
func sessionLightCopy(sess *WorkbenchSession) *WorkbenchSession {
	light := *sess
	light.Snapshot, light.LastDiff = nil, nil
	return &light
}

// sessionPayload 分片承载的重载荷：全景快照与最近一次重识别差异集。
type sessionPayload struct {
	Snapshot *RecognitionSnapshot `json:"snapshot,omitempty"`
	LastDiff *SnapDiff            `json:"lastDiff,omitempty"`
}

// marshalSessionPayload 载荷 → JSON → gzip 压缩字节，及其 sha256 hex（digest）。
func marshalSessionPayload(snapshot *RecognitionSnapshot, lastDiff *SnapDiff) (compressed []byte, digest string, err error) {
	raw, err := json.Marshal(sessionPayload{Snapshot: snapshot, LastDiff: lastDiff})
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, "", err
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	compressed = buf.Bytes()
	sum := sha256.Sum256(compressed)
	return compressed, hex.EncodeToString(sum[:]), nil
}

// unmarshalSessionPayload gzip 字节 → JSON → 载荷。
func unmarshalSessionPayload(compressed []byte) (*sessionPayload, error) {
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if err := zr.Close(); err != nil {
		return nil, err
	}
	var payload sessionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

// splitSessionShards 压缩字节按 3MB 边界（sessionShardRawUnit）切片。
func splitSessionShards(compressed []byte) [][]byte {
	if len(compressed) == 0 {
		return nil
	}
	shards := make([][]byte, 0, (len(compressed)+sessionShardRawUnit-1)/sessionShardRawUnit)
	for start := 0; start < len(compressed); start += sessionShardRawUnit {
		end := start + sessionShardRawUnit
		if end > len(compressed) {
			end = len(compressed)
		}
		shards = append(shards, compressed[start:end])
	}
	return shards
}

// parseSessionShardBatch 批次号字符串 → 数值（空/畸形=0）。
func parseSessionShardBatch(batch string) int64 {
	n, _ := strconv.ParseInt(batch, 10, 64)
	return n
}

// nextSessionShardBatch 下一批次号：取当前 UnixMilli 与旧批次 +1 的较大者——同一
// 毫秒内的两次全保存也不复用批次，防止全保存末步的旧批次清理误删刚写入的分片。
func nextSessionShardBatch(oldBatch int64) string {
	next := time.Now().UnixMilli()
	if oldBatch+1 > next {
		next = oldBatch + 1
	}
	return strconv.FormatInt(next, 10)
}

// readSessionEnvelope 读 meta 键并解析信封（store 注入）。返回值：
//   - env 非 nil：现值为信封；
//   - env 为 nil 且 raw 非空：旧格式整值（或不可解析的脏值，由调用方按旧格式
//     路径自行处理——旧格式解析失败即损坏语义）；
//   - env 为 nil 且 raw 为空：meta 缺席；
//   - err 非 nil：读取传输层失败。
func (m *SessionManager) readSessionEnvelope(root string) (env *sessionEnvelope, raw string, err error) {
	stored, err := m.store.GetValue(workbenchSessionKey(root))
	if err != nil {
		return nil, "", err
	}
	if stored == nil || stored.Value == "" {
		return nil, "", nil
	}
	if !probeSessionEnvelope(stored.Value) {
		return nil, stored.Value, nil
	}
	parsed, perr := parseSessionEnvelope(stored.Value)
	if perr != nil {
		return nil, stored.Value, nil
	}
	return parsed, stored.Value, nil
}

// deleteStaleShards 删除被替换旧批次的全部分片键（旧批次簿记取自替换前的旧 meta，
// 全保存原子切换后调用；旧格式/无 meta=无操作）。删除失败仅告警——残留垃圾分片键
// 不影响正确性（方案自曝风险3：meta 已切换至新批次，垃圾键不可达）。
func (m *SessionManager) deleteStaleShards(normRoot string, oldEnv *sessionEnvelope, newBatch string) {
	if oldEnv == nil || oldEnv.ShardSet == "" || oldEnv.ShardSet == newBatch || oldEnv.Shards <= 0 {
		return
	}
	for i := 0; i < oldEnv.Shards; i++ {
		key := sessionShardKey(normRoot, oldEnv.ShardSet, i)
		if err := m.store.DeleteValue(key); err != nil {
			m.warnf("删除旧会话分片失败（键 %q，残留垃圾不影响正确性）: %v", key, err)
		}
	}
}

// —— 会话索引（加速层，非真相源：缺席/损坏由回退扫描自愈）——

// readSessionIndexRoots 读索引；缺席/损坏返回 ok=false（调用方回退扫描）。防备性
// 清理空串与重复项并恢复升序（修剪重写的基线）。
func readSessionIndexRoots(store SessionStore) ([]string, bool) {
	stored, err := store.GetValue(workbenchSessionIndexKey)
	if err != nil || stored == nil || stored.Value == "" {
		return nil, false
	}
	var roots []string
	if err := json.Unmarshal([]byte(stored.Value), &roots); err != nil {
		return nil, false
	}
	seen := make(map[string]bool, len(roots))
	uniq := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		uniq = append(uniq, root)
	}
	sort.Strings(uniq)
	return uniq, true
}

// writeSessionIndex 写索引（升序）。
func writeSessionIndex(store SessionStore, roots []string) error {
	sorted := append([]string(nil), roots...)
	sort.Strings(sorted)
	data, err := json.Marshal(sorted)
	if err != nil {
		return err
	}
	return store.SetValue(workbenchSessionIndexKey, string(data))
}

// scanSessionRootsFromValues 全量 KV 枚举 → 归一化库根清单：按 meta 键前缀识别、
// 显式排除分片前缀（索引缺失/损坏时的回退扫描路径）。
func scanSessionRootsFromValues(all map[string]*sdkdto.StorageValue) []string {
	roots := make([]string, 0, len(all))
	for key := range all {
		if strings.HasPrefix(key, workbenchSessionSnapKeyPrefix) {
			continue
		}
		if !strings.HasPrefix(key, workbenchSessionKeyPrefix) {
			continue
		}
		roots = append(roots, strings.TrimPrefix(key, workbenchSessionKeyPrefix))
	}
	sort.Strings(roots)
	return roots
}

// ensureSessionIndexed 保证库根已登记进索引（幂等）。索引缺席/损坏时先经全量 KV
// 扫描重建——不可只写单项索引（会使 ListSessions 漏掉其他会话）。失败向上返回由
// 调用方告警：索引是加速层，索引登记失败不使已成功的会话保存失效，缺席由
// ListSessions 回退扫描自愈。
func (m *SessionManager) ensureSessionIndexed(normRoot string) error {
	roots, ok := readSessionIndexRoots(m.store)
	if !ok {
		all, err := m.store.GetAllValues()
		if err != nil {
			return fmt.Errorf("索引重建扫描失败: %w", err)
		}
		roots = scanSessionRootsFromValues(all)
	}
	pos := slicePos(roots, normRoot)
	if pos >= 0 && ok {
		return nil // 索引在场且已含该根：无需写
	}
	if pos < 0 {
		roots = append(roots, normRoot)
	}
	// 索引此前缺席（ok=false）时即使成员已在扫描结果中也须落写——否则索引
	// 永久缺席，每次保存都触发全量扫描。
	return writeSessionIndex(m.store, roots)
}

// unregisterSessionIndexed 从索引移除库根（幂等）。索引缺席/损坏时静默跳过——
// ListSessions 的扫描/修剪自愈。
func (m *SessionManager) unregisterSessionIndexed(normRoot string) {
	roots, ok := readSessionIndexRoots(m.store)
	if !ok {
		return
	}
	pos := slicePos(roots, normRoot)
	if pos < 0 {
		return
	}
	roots = append(roots[:pos], roots[pos+1:]...)
	if err := writeSessionIndex(m.store, roots); err != nil {
		m.warnf("会话索引移除失败（root=%s）: %v", normRoot, err)
	}
}
