// 会话载荷三件套（轻 meta 信封 + gzip 分片载荷 + 会话索引）的单元测试：
// 大载荷分片往返、旧格式兼容（读到即转）、索引路径与回退扫描重建、轻保存不触
// 分片、删除清理三件套、分片篡改的 digest 兜底——对应修复方案「测试锚定」节
// 全部 6 条断言。
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// countingSessionStore 带调用计数的内存会话存储（fakeSessionStore 外包一层）：
// 记录 SetValue/DeleteValue 的逐键调用次数与 GetAllValues 调用数，供「轻保存
// 不触分片」「索引路径不扫描」等断言。
type countingSessionStore struct {
	*fakeSessionStore

	mu           sync.Mutex
	setValueHits map[string]int
	deleteHits   map[string]int
	getAllHits   int
}

func newCountingSessionStore() *countingSessionStore {
	return &countingSessionStore{
		fakeSessionStore: newFakeSessionStore(),
		setValueHits:     map[string]int{},
		deleteHits:       map[string]int{},
	}
}

func (s *countingSessionStore) SetValue(key string, value string) error {
	s.mu.Lock()
	s.setValueHits[key]++
	s.mu.Unlock()
	return s.fakeSessionStore.SetValue(key, value)
}

func (s *countingSessionStore) DeleteValue(key string) error {
	s.mu.Lock()
	s.deleteHits[key]++
	s.mu.Unlock()
	return s.fakeSessionStore.DeleteValue(key)
}

func (s *countingSessionStore) GetAllValues() (map[string]*sdkdto.StorageValue, error) {
	s.mu.Lock()
	s.getAllHits++
	s.mu.Unlock()
	return s.fakeSessionStore.GetAllValues()
}

// resetWriteCounts 清零写侧计数（读侧计数不受影响）。
func (s *countingSessionStore) resetWriteCounts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setValueHits = map[string]int{}
	s.deleteHits = map[string]int{}
}

// resetCounts 清零全部计数（含 GetAllValues）。
func (s *countingSessionStore) resetCounts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setValueHits = map[string]int{}
	s.deleteHits = map[string]int{}
	s.getAllHits = 0
}

// shardSetValueCalls 分片键（workbench-session-snap: 前缀）上的 SetValue 调用总数。
func (s *countingSessionStore) shardSetValueCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for key, n := range s.setValueHits {
		if strings.HasPrefix(key, workbenchSessionSnapKeyPrefix) {
			total += n
		}
	}
	return total
}

// getAllValuesCalls GetAllValues 调用总数。
func (s *countingSessionStore) getAllValuesCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getAllHits
}

// capturingLogger 告警捕获桩（digest 兜底断言用；其余级别静默）。
type capturingLogger struct {
	mu    sync.Mutex
	warns []string
}

func (l *capturingLogger) Debugf(string, ...any) {}
func (l *capturingLogger) Infof(string, ...any)  {}
func (l *capturingLogger) Warnf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns = append(l.warns, fmt.Sprintf(format, args...))
}
func (l *capturingLogger) Errorf(string, ...any) {}

func (l *capturingLogger) Named(string) sdkdto.Logger { return l }

func (l *capturingLogger) warnedContaining(part string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, w := range l.warns {
		if strings.Contains(w, part) {
			return true
		}
	}
	return false
}

// buildLargeSnapshot 构造 gzip 压缩后超过 3MB 的识别快照：文件相对路径携带高熵
// 十六进制段（近不可压缩），保证分片数 ≥2（分片单元=压缩字节 3MB×3/4）。
func buildLargeSnapshot(root string, fileCount, hexSegLen int) *RecognitionSnapshot {
	rng := rand.New(rand.NewSource(20261029))
	files := make([]SnapFile, 0, fileCount)
	for i := 0; i < fileCount; i++ {
		var seg strings.Builder
		for seg.Len() < hexSegLen {
			seg.WriteString(fmt.Sprintf("%08x", rng.Uint32()))
		}
		files = append(files, SnapFile{
			RelPath:   "bulk/" + seg.String() + ".jpg",
			FileName:  seg.String()[:16] + ".jpg",
			DirRelKey: "bulk",
			State:     "identified",
			Final:     []SnapDecl{{Dimension: "author", Value: seg.String()[:8], Source: "builtin"}},
		})
	}
	return &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000000,
		RootPath:      root,
		Files:         files,
	}
}

// smallSnapshot 小体量快照（分片数=1 的常规形态用例）。
func smallSnapshot(root string) *RecognitionSnapshot {
	return &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion, GeneratedAtMs: 1700000000000,
		RootPath: root,
		Files: []SnapFile{{
			RelPath: "画集/人物画.jpg", FileName: "人物画.jpg", DirRelKey: "画集",
			State: "identified", Final: []SnapDecl{{Dimension: "author", Value: "作者甲", Source: "builtin"}},
		}},
	}
}

// mustReadEnvelope 读 meta 键并解析信封（失败即中止）。
func mustReadEnvelope(t *testing.T, store SessionStore, root string) *sessionEnvelope {
	t.Helper()
	stored, err := store.GetValue(workbenchSessionKey(root))
	if err != nil {
		t.Fatalf("读取会话 meta 失败: %v", err)
	}
	if stored == nil || stored.Value == "" {
		t.Fatalf("会话 meta 缺席（键 %q）", workbenchSessionKey(root))
	}
	if !probeSessionEnvelope(stored.Value) {
		t.Fatalf("meta 值应为信封形态，实际: %.120s", stored.Value)
	}
	env, err := parseSessionEnvelope(stored.Value)
	if err != nil {
		t.Fatalf("解析信封失败: %v", err)
	}
	return env
}

// shardKeyCountOfRoot 统计存储中该库根的分片键数。
func shardKeyCountOfRoot(t *testing.T, store SessionStore, root string) int {
	t.Helper()
	all, err := store.GetAllValues()
	if err != nil {
		t.Fatalf("枚举 KV 失败: %v", err)
	}
	norm := normalizeLibraryRoot(root)
	count := 0
	for key := range all {
		if strings.HasPrefix(key, workbenchSessionSnapKeyPrefix+norm+"@") {
			count++
		}
	}
	return count
}

// TestSessionPayloadLargeRoundTrip 锚定1 大载荷往返：gzip 后 >3MB 的快照全保存后
// 每片 ≤3MB、信封 shards 数正确；loadSession 与原会话全等（Snapshot/LastDiff 深比较）。
func TestSessionPayloadLargeRoundTrip(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	root := `D:\大库`

	snapshot := buildLargeSnapshot(root, 9000, 900)
	diff := &SnapDiff{
		Changed: []SnapDiffEntry{{RelPath: "bulk/a.jpg", After: &SnapConclusion{State: "identified"}}},
		Added:   []SnapDiffEntry{},
		Removed: []SnapDiffEntry{},
	}
	// 前置自检：载荷 gzip 后确实超过 3MB（分片必然 ≥2）
	compressed, _, err := marshalSessionPayload(snapshot, diff)
	if err != nil {
		t.Fatalf("载荷序列化失败: %v", err)
	}
	if len(compressed) <= sessionShardMaxValue {
		t.Fatalf("测试夹具载荷 gzip 后应超过 3MB，实际 %d 字节", len(compressed))
	}

	if _, err := mgr.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if _, err := mgr.CompleteRecognitionWithDiff(root, snapshot, diff); err != nil {
		t.Fatalf("全保存失败: %v", err)
	}

	env := mustReadEnvelope(t, store, root)
	if env.Shards < 2 {
		t.Fatalf("超 3MB 载荷应至少 2 片，实际 %d", env.Shards)
	}
	if env.Codec != sessionPayloadCodecGzip || env.Digest == "" {
		t.Fatalf("信封载荷簿记不符: codec=%q digest=%q", env.Codec, env.Digest)
	}
	if got := shardKeyCountOfRoot(t, store, root); got != env.Shards {
		t.Fatalf("分片键数 %d 与信封 shards %d 不符", got, env.Shards)
	}
	all, _ := store.GetAllValues()
	norm := normalizeLibraryRoot(root)
	for key, stored := range all {
		if !strings.HasPrefix(key, workbenchSessionSnapKeyPrefix+norm+"@") {
			continue
		}
		if len(stored.Value) > sessionShardMaxValue {
			t.Fatalf("分片 %q 超过 3MB 上界: %d 字节", key, len(stored.Value))
		}
	}

	loaded, err := mgr.loadSession(root)
	if err != nil {
		t.Fatalf("读回会话失败: %v", err)
	}
	if !reflect.DeepEqual(loaded.Snapshot, snapshot) {
		t.Fatalf("快照往返不相等：文件数 %d vs %d", len(loaded.Snapshot.Files), len(snapshot.Files))
	}
	if !reflect.DeepEqual(loaded.LastDiff, diff) {
		t.Fatalf("差异集往返不相等: %+v vs %+v", loaded.LastDiff, diff)
	}
	if loaded.State != WorkbenchSessionRecognized || loaded.ProfileDigest != snapshot.ProfileDigest {
		t.Fatalf("轻字段往返不符: state=%s profileDigest=%q", loaded.State, loaded.ProfileDigest)
	}
}

// TestSessionPayloadOldFormatCompat 锚定2 旧格式兼容：旧式整值 JSON 直接落 KV →
// loadSession 正常读出；再保存（轻入口触发）后转为信封+分片形态且载荷保留。
func TestSessionPayloadOldFormatCompat(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	root := `D:\旧库`

	snapshot := smallSnapshot(root)
	diff := &SnapDiff{Changed: []SnapDiffEntry{}, Added: []SnapDiffEntry{}, Removed: []SnapDiffEntry{}}
	old := &WorkbenchSession{
		SchemaVersion: workbenchSessionSchemaVersion, RootPath: root,
		State: WorkbenchSessionRecognized, CreatedAtMs: 111, UpdatedAtMs: 111,
		Snapshot: snapshot, LastDiff: diff,
	}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatalf("序列化旧格式会话失败: %v", err)
	}
	if err := store.SetValue(workbenchSessionKey(root), string(data)); err != nil {
		t.Fatalf("直写旧格式会话失败: %v", err)
	}

	loaded, err := mgr.loadSession(root)
	if err != nil {
		t.Fatalf("旧格式会话读取失败: %v", err)
	}
	if !reflect.DeepEqual(loaded.Snapshot, snapshot) || !reflect.DeepEqual(loaded.LastDiff, diff) {
		t.Fatalf("旧格式读取载荷不符")
	}

	// 再保存（轻入口：忽略项登记）——读到即转：升级为信封+分片形态
	if _, err := mgr.SetIgnored(root, "bulk/x.jpg", true); err != nil {
		t.Fatalf("轻保存失败: %v", err)
	}
	env := mustReadEnvelope(t, store, root)
	if env.Shards <= 0 || env.Digest == "" {
		t.Fatalf("旧格式转新后信封应有载荷簿记: shards=%d digest=%q", env.Shards, env.Digest)
	}
	if got := shardKeyCountOfRoot(t, store, root); got != env.Shards {
		t.Fatalf("旧格式转新后分片键数 %d 与信封 shards %d 不符", got, env.Shards)
	}
	reloaded, err := mgr.loadSession(root)
	if err != nil {
		t.Fatalf("转新后读回失败: %v", err)
	}
	if !reflect.DeepEqual(reloaded.Snapshot, snapshot) {
		t.Fatalf("转新后快照丢失")
	}
	if slicePos(reloaded.Ignored, "bulk/x.jpg") < 0 {
		t.Fatalf("转新后轻字段（忽略项）丢失: %v", reloaded.Ignored)
	}
}

// TestSessionPayloadIndexPath 锚定3 索引路径：ListSessions 走索引（GetAllValues
// 调用数为 0）；索引缺失 → 回退扫描成功且索引被重建。
func TestSessionPayloadIndexPath(t *testing.T) {
	store := newCountingSessionStore()
	mgr := NewSessionManagerWithStore(nil, nil, store)

	if _, err := mgr.CreateSession(`D:\索引甲`); err != nil {
		t.Fatalf("建立会话甲失败: %v", err)
	}
	if _, err := mgr.CreateSession(`D:\索引乙`); err != nil {
		t.Fatalf("建立会话乙失败: %v", err)
	}

	store.resetCounts()
	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("索引路径枚举失败: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("应枚举 2 个会话，实际 %d", len(sessions))
	}
	if got := store.getAllValuesCalls(); got != 0 {
		t.Fatalf("索引命中时 GetAllValues 调用数应为 0，实际 %d", got)
	}

	// 索引缺失（外部删除形态）→ 回退扫描 + 重建
	if err := store.DeleteValue(workbenchSessionIndexKey); err != nil {
		t.Fatalf("删除索引失败: %v", err)
	}
	store.resetCounts()
	sessions, err = mgr.ListSessions()
	if err != nil {
		t.Fatalf("回退扫描枚举失败: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("回退扫描应枚举 2 个会话，实际 %d", len(sessions))
	}
	if got := store.getAllValuesCalls(); got == 0 {
		t.Fatalf("索引缺失时应走回退扫描（GetAllValues 调用数 >0）")
	}
	roots, ok := readSessionIndexRoots(store)
	if !ok {
		t.Fatalf("回退扫描后索引应被重建")
	}
	if slicePos(roots, normalizeLibraryRoot(`D:\索引甲`)) < 0 ||
		slicePos(roots, normalizeLibraryRoot(`D:\索引乙`)) < 0 {
		t.Fatalf("重建索引应含两个库根，实际 %v", roots)
	}
}

// TestSessionPayloadLightSaveNoShardWrite 锚定4 轻保存不触分片（轻/全保存划分纪律
// 护栏，方案自曝风险4）：全保存后执行赋值变更 → 分片键 SetValue 调用数为 0、
// meta 时间戳更新、载荷与赋值均保留。
func TestSessionPayloadLightSaveNoShardWrite(t *testing.T) {
	store := newCountingSessionStore()
	mgr := NewSessionManagerWithStore(nil, nil, store)
	root := `D:\轻库`

	if _, err := mgr.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	snapshot := smallSnapshot(root)
	if _, err := mgr.CompleteRecognitionWithDiff(root, snapshot, nil); err != nil {
		t.Fatalf("全保存失败: %v", err)
	}
	before := mustReadEnvelope(t, store, root).Session.UpdatedAtMs

	store.resetWriteCounts()
	time.Sleep(2 * time.Millisecond) // 保证时间戳可区分（UnixMilli 粒度）
	if _, err := mgr.SetAssignment(root, "画集/人物画.jpg", SessionAssignment{Author: "作者乙"}); err != nil {
		t.Fatalf("赋值变更（轻保存）失败: %v", err)
	}

	if got := store.shardSetValueCalls(); got != 0 {
		t.Fatalf("轻保存不应触碰分片键，实际分片 SetValue 调用 %d 次", got)
	}
	after := mustReadEnvelope(t, store, root).Session.UpdatedAtMs
	if after <= before {
		t.Fatalf("轻保存应更新 meta 时间戳: before=%d after=%d", before, after)
	}
	loaded, err := mgr.loadSession(root)
	if err != nil {
		t.Fatalf("轻保存后读回失败: %v", err)
	}
	if !reflect.DeepEqual(loaded.Snapshot, snapshot) {
		t.Fatalf("轻保存后快照应保留在分片中不丢失")
	}
	if assign, ok := loaded.Assignments["画集/人物画.jpg"]; !ok || assign.Author != "作者乙" {
		t.Fatalf("轻保存后赋值不符: %+v", loaded.Assignments)
	}
}

// TestSessionPayloadDeleteCleanup 锚定5 删除清理：全保存后 DeleteSession →
// meta/分片/索引三者全清。
func TestSessionPayloadDeleteCleanup(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	root := `D:\删库`

	if _, err := mgr.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if _, err := mgr.CompleteRecognitionWithDiff(root, smallSnapshot(root), nil); err != nil {
		t.Fatalf("全保存失败: %v", err)
	}
	if err := mgr.DeleteSession(root); err != nil {
		t.Fatalf("删除会话失败: %v", err)
	}

	all, err := store.GetAllValues()
	if err != nil {
		t.Fatalf("枚举 KV 失败: %v", err)
	}
	for key := range all {
		if key == workbenchSessionKey(root) {
			t.Fatalf("删除后 meta 键仍在: %q", key)
		}
		if strings.HasPrefix(key, workbenchSessionSnapKeyPrefix+normalizeLibraryRoot(root)+"@") {
			t.Fatalf("删除后分片键仍在: %q", key)
		}
	}
	if roots, ok := readSessionIndexRoots(store); ok && slicePos(roots, normalizeLibraryRoot(root)) >= 0 {
		t.Fatalf("删除后索引仍含该库根: %v", roots)
	}
}

// TestSessionPayloadTamperDigest 锚定6 半写兜底：人为篡改一分片值 → loadSession
// 按 digest 拒绝、告警并返回未命中语义（同既有损坏语义）。
func TestSessionPayloadTamperDigest(t *testing.T) {
	store := newFakeSessionStore()
	logger := &capturingLogger{}
	mgr := NewSessionManagerWithStore(nil, logger, store)
	root := `D:\篡库`

	if _, err := mgr.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if _, err := mgr.CompleteRecognitionWithDiff(root, smallSnapshot(root), nil); err != nil {
		t.Fatalf("全保存失败: %v", err)
	}

	// 篡改首个分片：base64 值首字符翻转（长度不变、内容变化）
	env := mustReadEnvelope(t, store, root)
	norm := normalizeLibraryRoot(root)
	firstKey := sessionShardKey(norm, env.ShardSet, 0)
	stored, err := store.GetValue(firstKey)
	if err != nil || stored == nil || stored.Value == "" {
		t.Fatalf("读取分片失败: %v %+v", err, stored)
	}
	if _, err := base64.StdEncoding.DecodeString(stored.Value); err != nil {
		t.Fatalf("分片值应为合法 base64: %v", err)
	}
	flip := byte('A')
	if stored.Value[0] == 'A' {
		flip = 'B'
	}
	tampered := string(flip) + stored.Value[1:]
	if tampered == stored.Value {
		t.Fatalf("篡改夹具构造失败（值未变化）")
	}
	if err := store.SetValue(firstKey, tampered); err != nil {
		t.Fatalf("写回篡改分片失败: %v", err)
	}

	if _, err := mgr.loadSession(root); !errors.Is(err, ErrWorkbenchSessionNotFound) {
		t.Fatalf("分片篡改后应按未命中处理，实际: %v", err)
	}
	if !logger.warnedContaining("digest") {
		t.Fatalf("分片篡改后应有 digest 校验告警，实际告警: %v", logger.warns)
	}
}
