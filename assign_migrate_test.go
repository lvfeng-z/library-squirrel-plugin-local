// 赋值压实迁移的单元测试（阶段4，plan/目录粒度赋值表示方案.md『迁移设计与
// 回滚』）：压实算法（确定性 / 无条目文件=异值边界 / 死路径保持 / 最大塌缩 /
// 全字段相等判定）、版本闸新语义（v1 兼容读取 / v2 正常读 / 未来版本未命中）、
// 懒压实挂钩（备份先行 / AssignRevision 不递增 / 分片零触碰 / 信封级轻变异
// 回落全会话路径）、备份失败中止变异、两崩溃窗口幂等（风险8）、回滚通路，
// 以及迁移前后求值结论与创建链子行赋值维度的逐字节全等锚（净室识别链路）。
// 用户形态夹具（500 目录 × 350 文件 = 175,000 文件，纯内存构造）锚塌缩与写税
// 断言（实测数字随断言注明）。
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// —— 测试基建 ——

// legacySeedSession v1 旧形态会话测试模板：已识别态、快照（relPaths 为文件
// 相对路径清单）与赋值映射、赋值修订号。
func legacySeedSession(root string, relPaths []string, assigns map[string]SessionAssignment, revision int64) *WorkbenchSession {
	files := make([]SnapFile, 0, len(relPaths))
	for _, rel := range relPaths {
		files = append(files, SnapFile{
			RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: dirRelKeyOf(rel),
			State: snapStatePlain,
		})
	}
	return &WorkbenchSession{
		RootPath: root, State: WorkbenchSessionRecognized,
		CreatedAtMs: 1, UpdatedAtMs: 1, ProfileDigest: "digest",
		AssignRevision: revision,
		Snapshot: &RecognitionSnapshot{
			SchemaVersion: recognitionSnapshotSchemaVersion, GeneratedAtMs: 42,
			RootPath: root, Files: files, DirMtimes: map[string]int64{},
		},
		Assignments: assigns,
	}
}

// seedLegacyV1Envelope 把会话以 v1 旧形态（信封 + 分片载荷）写入存储，返回
// 信封原文。bookkeeping 非 nil 时复用其载荷簿记（分片零触碰，仅重写信封
// 轻字段——迁移测试的会话形态注入口）；为 nil 时按会话快照新写全套分片。
func seedLegacyV1Envelope(t *testing.T, store *fakeSessionStore, root string, sess *WorkbenchSession, bookkeeping *sessionEnvelope) string {
	t.Helper()
	var shardSet string
	var shards int
	codec, digest := "", ""
	if bookkeeping != nil {
		shardSet, shards, codec, digest = bookkeeping.ShardSet, bookkeeping.Shards, bookkeeping.Codec, bookkeeping.Digest
	} else {
		compressed, d, err := marshalSessionPayload(sess.Snapshot, sess.LastDiff)
		if err != nil {
			t.Fatalf("构造会话载荷失败: %v", err)
		}
		shardSet = nextSessionShardBatch(0)
		chunks := splitSessionShards(compressed)
		norm := normalizeLibraryRoot(root)
		for i, chunk := range chunks {
			key := sessionShardKey(norm, shardSet, i)
			if err := store.SetValue(key, base64.StdEncoding.EncodeToString(chunk)); err != nil {
				t.Fatalf("写入会话分片失败: %v", err)
			}
		}
		shards, codec, digest = len(chunks), sessionPayloadCodecGzip, d
	}
	light := sessionLightCopy(sess)
	light.SchemaVersion = legacyWorkbenchSessionSchemaVersion
	light.Assignments = sess.Assignments
	data, err := json.Marshal(&sessionEnvelope{
		V: sessionEnvelopeVersion, Session: light,
		ShardSet: shardSet, Shards: shards, Codec: codec, Digest: digest,
	})
	if err != nil {
		t.Fatalf("构造 v1 信封失败: %v", err)
	}
	if err := store.SetValue(workbenchSessionKey(root), string(data)); err != nil {
		t.Fatalf("写入 v1 信封失败: %v", err)
	}
	return string(data)
}

// failingPrefixStore 指定键前缀写入失败注入桩（其余转发内存存储）——备份
// 失败=中止变异路径的崩溃注入。
type failingPrefixStore struct {
	*fakeSessionStore
	failPrefix string
}

func (s *failingPrefixStore) SetValue(key, value string) error {
	if s.failPrefix != "" && strings.HasPrefix(key, s.failPrefix) {
		return errors.New("注入的存储写入失败")
	}
	return s.fakeSessionStore.SetValue(key, value)
}

// assertResolutionEquivalent 压实无信息损失的本地锚：对快照全部文件，压实前
// 后就近解析结果全等（含呈现位 Generalized/UpdatedAtMs）。
func assertResolutionEquivalent(t *testing.T, before, after map[string]SessionAssignment, files []SnapFile) {
	t.Helper()
	for _, f := range files {
		want := resolveFileAssignment(before, f.RelPath)
		got := resolveFileAssignment(after, f.RelPath)
		if !assignmentFullyEqual(want, got) {
			t.Fatalf("文件 %s 压实前后就近解析不等: 前 %+v 后 %+v", f.RelPath, want, got)
		}
	}
}

// —— 退出标准①：压实算法（确定性 / 边界 / 死路径 / 最大塌缩）——

// TestCompactAssignmentsUniformSubtreeCollapse 均匀子树最大塌缩：全库同值→库根
// 目录键 "/"；部分均匀→塌缩链顶条目并消费子树内全部文件条目与子目录产出；
// 呈现位（Generalized/UpdatedAtMs）与线索出处差异均阻断归并（全字段相等判定）。
func TestCompactAssignmentsUniformSubtreeCollapse(t *testing.T) {
	uniform := SessionAssignment{
		Author: "作者甲", AuthorRole: "原画", AuthorClueRef: "dirLevelStripped1",
		Tags: []string{"画师", "水彩"}, TagClueRefs: []string{"dirLevel1", "rootName"},
		Generalized: true, UpdatedAtMs: 1700000000000,
	}
	relPaths := []string{
		"A/x1.png", "A/x2.png", "A/sub/y1.png", "A/sub/y2.png", "A/sub/deep/z.png",
		"B/o.png",
	}
	assigns := make(map[string]SessionAssignment, 6)
	for _, rel := range relPaths {
		assigns[rel] = uniform
	}
	files := legacySeedSession("D:\\压实甲", relPaths, nil, 0).Snapshot.Files

	// 全库同值（B 也同值）→ 塌缩到库根目录键 "/"
	whole := compactLegacyAssignments(assigns, files)
	if len(whole) != 1 {
		t.Fatalf("全库同值应塌缩为单一库根条目，实际 %d 条: %v", len(whole), whole)
	}
	if got := whole["/"]; !assignmentFullyEqual(got, uniform) {
		t.Fatalf("库根条目应携带均匀值: %+v", got)
	}
	assertResolutionEquivalent(t, assigns, whole, files)

	// B 异值 → 库根不可塌缩；A 子树整体均匀 → 塌缩在 A/（消费 sub 与 deep）
	mixed := make(map[string]SessionAssignment, len(assigns))
	for k, v := range assigns {
		mixed[k] = v
	}
	other := uniform
	other.Author = "作者乙"
	mixed["B/o.png"] = other
	out := compactLegacyAssignments(mixed, files)
	if _, ok := out["/"]; ok {
		t.Fatal("存在异值文件时不得塌缩库根")
	}
	if got, ok := out["A/"]; !ok || !assignmentFullyEqual(got, uniform) {
		t.Fatalf("A 子树均匀应塌缩为目录条目: %+v（在场 %v）", got, ok)
	}
	for _, key := range []string{"A/sub/", "A/sub/deep/", "A/x1.png", "A/sub/y2.png"} {
		if _, ok := out[key]; ok {
			t.Fatalf("被塌缩消费的子树条目应消失: %s", key)
		}
	}
	if got, ok := out["B/"]; !ok || got.Author != "作者乙" {
		t.Fatalf("单文件异值目录应塌缩为其值的目录条目: %+v（在场 %v）", got, ok)
	}
	if _, ok := out["B/o.png"]; ok {
		t.Fatal("单文件目录塌缩后文件条目应被消费")
	}
	assertResolutionEquivalent(t, mixed, out, files)

	// 全字段相等判定：值相同而 UpdatedAtMs / Generalized / 线索出处任一不同
	// 均不可归并（呈现位全等是迁移全等锚的必要件）
	for name, mutate := range map[string]func(*SessionAssignment){
		"时间戳差异":  func(a *SessionAssignment) { a.UpdatedAtMs++ },
		"泛化标记差异": func(a *SessionAssignment) { a.Generalized = false },
		"出处差异":   func(a *SessionAssignment) { a.AuthorClueRef = "dirLevel1" },
		"标签出处差异": func(a *SessionAssignment) { a.TagClueRefs = []string{"dirLevel1", "dirLevel2"} },
	} {
		variant := make(map[string]SessionAssignment, len(relPaths))
		for i, rel := range relPaths {
			v := uniform
			if i == 0 {
				mutate(&v)
			}
			variant[rel] = v
		}
		got := compactLegacyAssignments(variant, files)
		if _, ok := got["/"]; ok {
			t.Fatalf("%s 应阻断归并（单文件全字段不等）", name)
		}
		if _, ok := got[relPaths[0]]; !ok {
			t.Fatalf("%s：异值文件应保持文件条目", name)
		}
		assertResolutionEquivalent(t, variant, got, files)
	}
}

// TestCompactAssignmentsUnassignedFileBoundary 无条目文件=异值边界（方案风险2）：
// 目录子树内部分文件无赋值 → 该目录不归并（目录条目不得越权覆盖未赋值文件），
// 更深层均匀子树仍归并；边界目录的直属文件保持文件条目。
func TestCompactAssignmentsUnassignedFileBoundary(t *testing.T) {
	relPaths := []string{
		"A/f1.png", "A/f2.png", "A/f3.png", // f3 无条目
		"A/deep/g1.png", "A/deep/g2.png", // 深层均匀
		"B/h1.png", "B/h2.png", // 全有条目且均匀（对照组：应塌缩）
	}
	value := SessionAssignment{Author: "作者甲", UpdatedAtMs: 7}
	assigns := map[string]SessionAssignment{
		"A/f1.png":       value,
		"A/f2.png":       value,
		"A/deep/g1.png":  value,
		"A/deep/g2.png":  value,
		"B/h1.png":       value,
		"B/h2.png":       value,
	}
	files := legacySeedSession("D:\\压实乙", relPaths, nil, 0).Snapshot.Files

	out := compactLegacyAssignments(assigns, files)
	if _, ok := out["A/"]; ok {
		t.Fatal("A 内存在无条目文件（f3）时不得归并为目录条目（越权覆盖未赋值文件）")
	}
	for _, key := range []string{"A/f1.png", "A/f2.png"} {
		if _, ok := out[key]; !ok {
			t.Fatalf("边界目录的直属文件应保持文件条目: %s", key)
		}
	}
	if _, ok := out["A/f3.png"]; ok {
		t.Fatal("无条目文件不得在压实后凭空出现条目")
	}
	if got, ok := out["A/deep/"]; !ok || !assignmentFullyEqual(got, value) {
		t.Fatalf("更深层均匀子树仍应归并: %+v（在场 %v）", got, ok)
	}
	if got, ok := out["B/"]; !ok || !assignmentFullyEqual(got, value) {
		t.Fatalf("全有条目且均匀的对照目录应塌缩: %+v（在场 %v）", got, ok)
	}
	assertResolutionEquivalent(t, assigns, out, files)
	// f3 就近解析恒为零值（压实前后）
	if got := resolveFileAssignment(out, "A/f3.png"); len(got.coveredDimensions()) != 0 {
		t.Fatalf("无条目文件压实后不得被目录条目供值: %+v", got)
	}
}

// TestCompactAssignmentsDeadPathAndDirKeyKept 死路径键（不在快照文件集）原样
// 保持文件条目、不参与压实；输入既有目录条目键原样保持并钉住其目录不塌缩
//（防御分支：野生 v1 会话无目录键）。
func TestCompactAssignmentsDeadPathAndDirKeyKept(t *testing.T) {
	relPaths := []string{"A/x.png", "B/y.png", "B/c/z.png"}
	value := SessionAssignment{Author: "作者甲", UpdatedAtMs: 9}
	deadValue := SessionAssignment{Author: "旧作者", Title: "旧题", UpdatedAtMs: 1}
	assigns := map[string]SessionAssignment{
		"A/x.png":     value,
		"B/y.png":     value,
		"B/c/z.png":   value,
		"已删/旧.png":  deadValue,
		"B/":          value, // 防御：v1 输入携目录键（野生不可达）
	}
	files := legacySeedSession("D:\\压实丙", relPaths, nil, 0).Snapshot.Files

	out := compactLegacyAssignments(assigns, files)
	if got, ok := out["已删/旧.png"]; !ok || !assignmentFullyEqual(got, deadValue) {
		t.Fatalf("死路径键应原样保持: %+v（在场 %v）", got, ok)
	}
	if got, ok := out["B/"]; !ok || !assignmentFullyEqual(got, value) {
		t.Fatalf("既有目录键应原样保持: %+v（在场 %v）", got, ok)
	}
	if _, ok := out["B/c/"]; !ok {
		t.Fatal("被钉住目录之下更深的均匀子树仍应归并")
	}
	if _, ok := out["B/y.png"]; !ok {
		t.Fatal("被钉住目录的直属文件应保持文件条目")
	}
	if got, ok := out["A/"]; !ok || !assignmentFullyEqual(got, value) {
		t.Fatalf("无钉住的均匀目录应塌缩: %+v（在场 %v）", got, ok)
	}
	assertResolutionEquivalent(t, assigns, out, files)
}

// TestCompactAssignmentsDeterministic 压实确定性：同输入双跑，赋值序列化
// bytes.Equal（含多层嵌套 / 例外 / 无条目 / 死路径 / 泛化标记的混合夹具）。
func TestCompactAssignmentsDeterministic(t *testing.T) {
	build := func() (map[string]SessionAssignment, []SnapFile) {
		relPaths := []string{
			"甲/a1.png", "甲/a2.png", "甲/sub/s1.png", "甲/sub/s2.png", "甲/sub/deep/d.png",
			"乙/b1.png", "乙/b2.png", // b2 时间戳差异
			"丙/c1.png", "丙/c2.png", // c2 无条目
			"丁/d1.png", "丁/d2.png", "丁/deep/e1.png", // 丁 均匀含深层
		}
		uniform := SessionAssignment{
			Author: "甲", AuthorClueRef: "dirLevelStripped1",
			Tags: []string{"画集"}, TagClueRefs: []string{"dirLevel1"},
			Generalized: true, UpdatedAtMs: 1700000000000,
		}
		assigns := make(map[string]SessionAssignment, 16)
		for _, rel := range []string{"甲/a1.png", "甲/a2.png", "甲/sub/s1.png", "甲/sub/s2.png", "甲/sub/deep/d.png"} {
			assigns[rel] = uniform
		}
		assigns["乙/b1.png"] = uniform
		v2 := uniform
		v2.UpdatedAtMs++
		assigns["乙/b2.png"] = v2
		assigns["丙/c1.png"] = uniform
		assigns["丁/d1.png"] = uniform
		assigns["丁/d2.png"] = uniform
		assigns["丁/deep/e1.png"] = uniform
		assigns["已删/x.png"] = SessionAssignment{Author: "旧", UpdatedAtMs: 1}
		return assigns, legacySeedSession("D:\\压实丁", relPaths, nil, 0).Snapshot.Files
	}
	firstAssigns, files := build()
	secondAssigns, _ := build()
	first, err := json.Marshal(compactLegacyAssignments(firstAssigns, files))
	if err != nil {
		t.Fatalf("序列化压实结果失败: %v", err)
	}
	second, err := json.Marshal(compactLegacyAssignments(secondAssigns, files))
	if err != nil {
		t.Fatalf("序列化第二次压实结果失败: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("同输入双跑压实结果应逐字节相等:\n%s\n%s", first, second)
	}
	if bytes.Equal(first, []byte("{}")) {
		t.Fatal("夹具应产出非空压实结果")
	}
}

// —— 退出标准②：版本闸新语义（高拒低容）——

// TestLoadSessionVersionGateSemantics 版本闸三处判读的新语义：v1 旧形态（信封
// 与整值两形态）新代码原生兼容读取且赋值解析正确；v2 会话正常读；伪造未来
// 版本（v3）按未命中处理；ListSessions 对 v1 会话仍列出（不被索引修剪）。
func TestLoadSessionVersionGateSemantics(t *testing.T) {
	store := newFakeSessionStore()
	m, _ := newTestManager(t, store)

	// v1 信封形态（含分片载荷）：兼容读取，赋值就近解析正确
	rootV1 := `D:\库壬`
	legacyAssigns := map[string]SessionAssignment{
		"画师/f1.png": {Author: "画师甲", UpdatedAtMs: 5},
		"画师/f2.png": {Author: "画师甲", UpdatedAtMs: 5},
	}
	sessV1 := legacySeedSession(rootV1, []string{"画师/f1.png", "画师/f2.png"}, legacyAssigns, 3)
	seedLegacyV1Envelope(t, store, rootV1, sessV1, nil)
	loaded, err := m.SessionOf(rootV1)
	if err != nil {
		t.Fatalf("v1 信封会话应兼容读取: %v", err)
	}
	if loaded.SchemaVersion != legacyWorkbenchSessionSchemaVersion || loaded.Snapshot == nil {
		t.Fatalf("v1 会话应原样读出（版本 1、快照在场）: 版本 %d 快照 %v", loaded.SchemaVersion, loaded.Snapshot != nil)
	}
	if got := resolveFileAssignment(loaded.Assignments, "画师/f1.png"); got.Author != "画师甲" {
		t.Fatalf("v1 赋值解析不符: %+v", got)
	}

	// v1 整值形态（信封化之前的单键单 JSON）：同口径兼容读取
	rootLegacy := `D:\库癸`
	legacyWhole := legacySeedSession(rootLegacy, []string{"x.png"}, map[string]SessionAssignment{
		"x.png": {Title: "题", UpdatedAtMs: 6},
	}, 1)
	legacyWhole.SchemaVersion = legacyWorkbenchSessionSchemaVersion
	wholeData, err := json.Marshal(legacyWhole)
	if err != nil {
		t.Fatalf("构造 v1 整值会话失败: %v", err)
	}
	if err := store.SetValue(workbenchSessionKey(rootLegacy), string(wholeData)); err != nil {
		t.Fatalf("写入 v1 整值会话失败: %v", err)
	}
	loadedWhole, err := m.loadSession(rootLegacy)
	if err != nil {
		t.Fatalf("v1 整值会话应兼容读取: %v", err)
	}
	if loadedWhole.Assignments["x.png"].Title != "题" {
		t.Fatalf("v1 整值赋值不符: %+v", loadedWhole.Assignments)
	}

	// v2 会话（当前版本）：正常读
	rootV2 := `D:\库子`
	if _, err := m.CreateSession(rootV2); err != nil {
		t.Fatalf("建立 v2 会话失败: %v", err)
	}
	if _, err := m.SessionOf(rootV2); err != nil {
		t.Fatalf("v2 会话应正常读取: %v", err)
	}

	// 伪造未来版本（v3）：按未命中处理（信封与整值两形态）
	rootV3 := `D:\库丑`
	future := legacySeedSession(rootV3, nil, nil, 0)
	future.SchemaVersion = workbenchSessionSchemaVersion + 1
	envData, err := json.Marshal(&sessionEnvelope{
		V: sessionEnvelopeVersion, Session: sessionLightCopy(future), ShardSet: "1", Shards: 0,
	})
	if err != nil {
		t.Fatalf("构造 v3 信封失败: %v", err)
	}
	if err := store.SetValue(workbenchSessionKey(rootV3), string(envData)); err != nil {
		t.Fatalf("写入 v3 信封失败: %v", err)
	}
	if _, err := m.SessionOf(rootV3); !errors.Is(err, ErrWorkbenchSessionNotFound) {
		t.Fatalf("v3 信封会话应按未命中处理，实际: %v", err)
	}
	wholeFuture, err := json.Marshal(future)
	if err != nil {
		t.Fatalf("构造 v3 整值失败: %v", err)
	}
	if err := store.SetValue(workbenchSessionKey(`D:\库寅`), string(wholeFuture)); err != nil {
		t.Fatalf("写入 v3 整值失败: %v", err)
	}
	if _, err := m.loadSession(`D:\库寅`); !errors.Is(err, ErrWorkbenchSessionNotFound) {
		t.Fatalf("v3 整值会话应按未命中处理，实际: %v", err)
	}

	// ListSessions：v1 会话仍列出（不被修剪），v3 会被跳过
	sessions, err := m.ListSessions()
	if err != nil {
		t.Fatalf("枚举会话失败: %v", err)
	}
	roots := make(map[string]bool, len(sessions))
	for _, sess := range sessions {
		roots[normalizeLibraryRoot(sess.RootPath)] = true
	}
	for _, want := range []string{normalizeLibraryRoot(rootV1), normalizeLibraryRoot(rootLegacy), normalizeLibraryRoot(rootV2)} {
		if !roots[want] {
			t.Fatalf("会话 %q 应在清单内（v1 兼容可列）: %v", want, roots)
		}
	}
	if roots[normalizeLibraryRoot(rootV3)] {
		t.Fatalf("未来版本会话不应在清单内: %v", roots)
	}
}

// —— 退出标准③：懒压实挂钩（备份先行 / 修订号不递增 / 分片零触碰）——

// TestAssignMigrateLazyHookCompactsAndBacksUp 懒压实挂钩：v1 会话首个全解码
// 变异触发压实（备份先行=v1 信封原文；结构版本置 2；AssignRevision 不递增
// 〔开放决策③〕；分片零触碰）；信封级轻变异对 v1 回落全会话路径同样触发；
// 缓存路径（SessionOf）取到压实后实例。
func TestAssignMigrateLazyHookCompactsAndBacksUp(t *testing.T) {
	store := newFakeSessionStore()
	m, _ := newTestManager(t, store)
	root := `D:\库卯`
	assigns := map[string]SessionAssignment{
		"画师/a1.png": {Author: "画师甲", UpdatedAtMs: 5},
		"画师/a2.png": {Author: "画师甲", UpdatedAtMs: 5},
		"例外/b.png":  {Title: "例外题", UpdatedAtMs: 6},
	}
	sess := legacySeedSession(root, []string{"画师/a1.png", "画师/a2.png", "例外/b.png"}, assigns, 12)
	rawV1 := seedLegacyV1Envelope(t, store, root, sess, nil)
	shardsBefore := store.values[sessionShardKey(normalizeLibraryRoot(root), "1", 0)] // 快照小=单分片

	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("首个变异（懒压实）失败: %v", err)
	}
	compacted, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("压实后读会话失败: %v", err)
	}
	if compacted.SchemaVersion != workbenchSessionSchemaVersion {
		t.Fatalf("压实后结构版本应为 %d，实际 %d", workbenchSessionSchemaVersion, compacted.SchemaVersion)
	}
	if compacted.AssignRevision != 12 {
		t.Fatalf("压实不得递增 AssignRevision（开放决策③），实际 %d", compacted.AssignRevision)
	}
	if got, ok := compacted.Assignments["画师/"]; !ok || got.Author != "画师甲" {
		t.Fatalf("均匀子树应塌缩为目录条目: %+v（在场 %v）", got, ok)
	}
	if _, ok := compacted.Assignments["画师/a1.png"]; ok {
		t.Fatal("被塌缩消费的文件条目应消失")
	}
	if got, ok := compacted.Assignments["例外/"]; !ok || got.Title != "例外题" {
		t.Fatalf("单文件例外目录应塌缩为其值的目录条目: %+v（在场 %v）", got, ok)
	}
	if _, ok := compacted.Assignments["例外/b.png"]; ok {
		t.Fatal("单文件目录塌缩后文件条目应被消费")
	}
	// 备份键在场=v1 信封原文；分片零触碰；缓存取压实后实例
	backup, err := store.GetValue(workbenchSessionMigrateBackupKey(root))
	if err != nil || backup == nil || backup.Value != rawV1 {
		t.Fatalf("单代备份应存 v1 信封原文: 在场 %v 等值 %v err %v", backup != nil, backup != nil && backup.Value == rawV1, err)
	}
	if shardsAfter := store.values[sessionShardKey(normalizeLibraryRoot(root), "1", 0)]; shardsAfter != shardsBefore {
		t.Fatal("压实只写信封，分片键值不得变化")
	}
	if cached, err := m.SessionOf(root); err != nil || len(cached.Assignments) != 2 {
		t.Fatalf("缓存应取压实后实例（2 条目），实际 %d 条 err %v", len(cached.Assignments), err)
	}

	// v1 信封级轻变异：回落全会话路径并触发压实
	root2 := `D:\库辰`
	sess2 := legacySeedSession(root2, []string{"c.png", "d.png"}, map[string]SessionAssignment{
		"c.png": {Author: "X", UpdatedAtMs: 1},
		"d.png": {Author: "X", UpdatedAtMs: 1},
	}, 4)
	raw2 := seedLegacyV1Envelope(t, store, root2, sess2, nil)
	if _, err := m.mutateSessionEnvelope(root2, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("v1 信封级轻变异应回落全会话路径成功: %v", err)
	}
	compacted2, err := m.loadSession(root2)
	if err != nil {
		t.Fatalf("回读压实后会话失败: %v", err)
	}
	if compacted2.SchemaVersion != workbenchSessionSchemaVersion || len(compacted2.Assignments) != 1 {
		t.Fatalf("信封级触发同样压实升级: 版本 %d 条目 %v", compacted2.SchemaVersion, compacted2.Assignments)
	}
	if backup2, err := store.GetValue(workbenchSessionMigrateBackupKey(root2)); err != nil || backup2.Value != raw2 {
		t.Fatalf("信封级触发路径的备份应存 v1 原文: %v", err)
	}
}

// TestAssignMigrateBackupFailureAborts 备份失败=中止变异（宁可不压也不失备）：
// 备份键写入失败时变异整体回错（错误文案含用户可见告警锚），信封维持 v1
// 原形态（未压实、未升版），重试（备份恢复）后正常压实。
func TestAssignMigrateBackupFailureAborts(t *testing.T) {
	inner := newFakeSessionStore()
	store := &failingPrefixStore{fakeSessionStore: inner, failPrefix: workbenchSessionMigrateBackupKeyPrefix}
	m := NewSessionManagerWithStore(nil, nil, store)
	root := `D:\库巳`
	assigns := map[string]SessionAssignment{
		"a.png": {Author: "甲", UpdatedAtMs: 1},
		"b.png": {Author: "甲", UpdatedAtMs: 1},
	}
	sess := legacySeedSession(root, []string{"a.png", "b.png"}, assigns, 2)
	rawV1 := seedLegacyV1Envelope(t, inner, root, sess, nil) // 分片与信封直接落内层存储

	_, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "赋值迁移前备份失败，本次操作未生效") {
		t.Fatalf("备份失败应中止变异并回告用户锚文案，实际: %v", err)
	}
	stored, err := inner.GetValue(workbenchSessionKey(root))
	if err != nil || stored.Value != rawV1 {
		t.Fatalf("备份失败后信封应维持 v1 原文不动: 等值 %v err %v", stored != nil && stored.Value == rawV1, err)
	}
	untouched, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("中止后 v1 会话应仍可兼容读取: %v", err)
	}
	if untouched.SchemaVersion != legacyWorkbenchSessionSchemaVersion || len(untouched.Assignments) != 2 {
		t.Fatalf("中止后赋值应维持旧形态: 版本 %d 条目 %d", untouched.SchemaVersion, len(untouched.Assignments))
	}

	// 存储恢复后重试：正常压实升级
	store.failPrefix = ""
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("备份恢复后重试应成功: %v", err)
	}
	fixed, err := m.loadSession(root)
	if err != nil || fixed.SchemaVersion != workbenchSessionSchemaVersion || len(fixed.Assignments) != 1 {
		t.Fatalf("重试后应压实升级（单目录条目）: 版本 %d 条目 %d err %v", fixed.SchemaVersion, len(fixed.Assignments), err)
	}
}

// —— 退出标准④：崩溃窗口幂等（风险8 复核）——

// TestAssignMigrateCrashWindowAfterBackup 崩溃窗口①（备份写成后、信封写回前
// 中止）：旧信封（v1 原形态）仍在，重走迁移幂等——备份键被同内容覆写、
// 压实结果与首程一致。
func TestAssignMigrateCrashWindowAfterBackup(t *testing.T) {
	store := newFakeSessionStore()
	m, _ := newTestManager(t, store)
	root := `D:\库午`
	assigns := map[string]SessionAssignment{
		"a.png": {Author: "甲", UpdatedAtMs: 1},
		"b.png": {Author: "甲", UpdatedAtMs: 1},
	}
	sess := legacySeedSession(root, []string{"a.png", "b.png"}, assigns, 2)
	rawV1 := seedLegacyV1Envelope(t, store, root, sess, nil)

	// 窗口①注入：备份已写成（挂钩内），变异闭包中止 → 信封写回不发生
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return errors.New("注入中止（信封写回前崩溃）") }); err == nil {
		t.Fatal("注入中止应作为变异错误上抛")
	}
	stored, _ := store.GetValue(workbenchSessionKey(root))
	if stored.Value != rawV1 {
		t.Fatal("窗口①中止后旧信封（v1 原形态）应仍在")
	}
	backup, _ := store.GetValue(workbenchSessionMigrateBackupKey(root))
	if backup == nil || backup.Value != rawV1 {
		t.Fatal("窗口①中止后备份键应已写成（v1 原文）")
	}

	// 重试：幂等——备份键同内容覆写、迁移完成、赋值形态与备份原文可对照
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("重试迁移失败: %v", err)
	}
	backup2, _ := store.GetValue(workbenchSessionMigrateBackupKey(root))
	if backup2.Value != rawV1 {
		t.Fatal("重试幂等：备份键应被同内容覆写（仍 v1 原文）")
	}
	done, err := m.loadSession(root)
	if err != nil || done.SchemaVersion != workbenchSessionSchemaVersion || len(done.Assignments) != 1 {
		t.Fatalf("重试后应完成压实: 版本 %d 条目 %d err %v", done.SchemaVersion, len(done.Assignments), err)
	}
}

// TestAssignMigrateCrashWindowAfterEnvelopeWrite 崩溃窗口②（信封写回后）：
// 迁移已完成、备份在场供回滚；后续变异不再重走迁移（备份不被触碰——哨兵
// 探针锚定挂钩只跑一次）。
func TestAssignMigrateCrashWindowAfterEnvelopeWrite(t *testing.T) {
	store := newFakeSessionStore()
	m, _ := newTestManager(t, store)
	root := `D:\库未`
	assigns := map[string]SessionAssignment{
		"a.png": {Author: "甲", UpdatedAtMs: 1},
		"b.png": {Author: "甲", UpdatedAtMs: 1},
	}
	sess := legacySeedSession(root, []string{"a.png", "b.png"}, assigns, 2)
	rawV1 := seedLegacyV1Envelope(t, store, root, sess, nil)

	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("首个变异（懒压实）失败: %v", err)
	}
	stored, _ := store.GetValue(workbenchSessionKey(root))
	if stored.Value == rawV1 {
		t.Fatal("信封写回后应为压实形态（v2）")
	}
	backup, _ := store.GetValue(workbenchSessionMigrateBackupKey(root))
	if backup == nil || backup.Value != rawV1 {
		t.Fatal("迁移完成后备份应在场（v1 原文）")
	}

	// 哨兵覆写备份键：再度变异不得触碰备份（挂钩只对 v1 会话运行）
	if err := store.SetValue(workbenchSessionMigrateBackupKey(root), "哨兵"); err != nil {
		t.Fatalf("写入哨兵失败: %v", err)
	}
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("后续变异失败: %v", err)
	}
	sentinel, _ := store.GetValue(workbenchSessionMigrateBackupKey(root))
	if sentinel.Value != "哨兵" {
		t.Fatal("迁移完成后后续变异不得再触碰备份键")
	}
}

// —— 退出标准⑤：回滚通路 ——

// TestAssignMigrateRollbackRestoresLegacy 回滚=备份原文写回信封键：回写后
// v1 旧形态可读（旧版插件亦可读的降级面）；再变异即再压实（回滚-迁移循环
// 幂等）；备份缺席（未曾压实）时回滚报错。
func TestAssignMigrateRollbackRestoresLegacy(t *testing.T) {
	store := newFakeSessionStore()
	m, _ := newTestManager(t, store)
	root := `D:\库申`
	assigns := map[string]SessionAssignment{
		"a.png": {Author: "甲", UpdatedAtMs: 1},
		"b.png": {Author: "甲", UpdatedAtMs: 1},
	}
	sess := legacySeedSession(root, []string{"a.png", "b.png"}, assigns, 2)
	rawV1 := seedLegacyV1Envelope(t, store, root, sess, nil)

	if err := m.RollbackAssignMigrateBackup(root); err == nil {
		t.Fatal("未曾压实（备份缺席）时回滚应报错")
	}
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("首个变异（懒压实）失败: %v", err)
	}
	if err := m.RollbackAssignMigrateBackup(root); err != nil {
		t.Fatalf("回滚失败: %v", err)
	}
	rolled, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("回滚后会话应可读: %v", err)
	}
	if rolled.SchemaVersion != legacyWorkbenchSessionSchemaVersion || len(rolled.Assignments) != 2 {
		t.Fatalf("回滚后应为 v1 逐文件旧形态: 版本 %d 条目 %d", rolled.SchemaVersion, len(rolled.Assignments))
	}
	if rolled.Assignments["a.png"].Author != "甲" {
		t.Fatalf("回滚后旧形态赋值应原样: %+v", rolled.Assignments)
	}
	// 回滚-迁移循环：再变异再压实，备份仍 v1 原文
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("回滚后再变异（再压实）失败: %v", err)
	}
	again, _ := m.loadSession(root)
	if again.SchemaVersion != workbenchSessionSchemaVersion || len(again.Assignments) != 1 {
		t.Fatalf("回滚后再变异应再压实: 版本 %d 条目 %d", again.SchemaVersion, len(again.Assignments))
	}
	backup, _ := store.GetValue(workbenchSessionMigrateBackupKey(root))
	if backup.Value != rawV1 {
		t.Fatal("循环后备份应仍为 v1 原文（同内容覆写）")
	}
}

// —— 退出标准⑥：迁移前后求值结论与创建链子行赋值维度逐字节全等 ——

// evalAnchorProjection 求值结论投影：终选声明/待裁决/Assigned/死路径全字段
//（快照生成时间与增量游标为轮次簿记，不入锚；Files 取快照文件记录原序全量，
// 含终选 Final/待裁决维度/命中规则/Assigned 标记）。
type evalAnchorProjection struct {
	Files   []SnapFile    `json:"files"`
	Pending []SnapPending `json:"pending"`
	Stale   []string      `json:"stale"`
	Stats   SnapStats     `json:"stats"`
}

// captureEvalAnchor 识别快照 → 求值结论投影 JSON。
func captureEvalAnchor(t *testing.T, snap *RecognitionSnapshot) []byte {
	t.Helper()
	proj := evalAnchorProjection{
		Files: snap.Files, Pending: snap.Pending,
		Stale: snap.StaleAssignments, Stats: snap.Stats,
	}
	data, err := json.Marshal(&proj)
	if err != nil {
		t.Fatalf("求值锚投影序列化失败: %v", err)
	}
	return data
}

// TestAssignMigrateEvaluationEquivalence 迁移全等锚（核心验收）：同一会话 v1
// 逐文件旧形态求值快照投影 JSON vs 压实后再求值投影 JSON bytes.Equal（终选
// 声明/待裁决/Assigned/死路径全字段，含无条目边界/异值/泛化标记/死路径形
// 态）；创建链子行赋值维度全等（buildChildResponses 产物对照）。
func TestAssignMigrateEvaluationEquivalence(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)
	// 赋值形态夹具：均匀整树（作者甲，最大塌缩）、部分无条目（作者乙直下，
	// 深层均匀）、同目录异值（异值/）、死路径
	writeTestFile(t, root+"/作者甲/a1.png", "a")
	writeTestFile(t, root+"/作者甲/a2.png", "a")
	writeTestFile(t, root+"/作者甲/sub/a3.png", "a")
	writeTestFile(t, root+"/作者乙/b0.png", "b")
	writeTestFile(t, root+"/作者乙/b1.png", "b")
	writeTestFile(t, root+"/作者乙/b2.png", "b")
	writeTestFile(t, root+"/作者乙/sub/b3.png", "b")
	writeTestFile(t, root+"/异值/c1.png", "c")
	writeTestFile(t, root+"/异值/c2.png", "c")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}

	// v1 逐文件旧形态注入（模拟旧版本插件写下的逐文件合并值）
	uniformJia := SessionAssignment{
		Author: "作者甲", AuthorClueRef: "dirLevelStripped1",
		Tags: []string{"画师"}, TagClueRefs: []string{"dirLevel1"},
		UpdatedAtMs: 1700000000000,
	}
	uniformYi := SessionAssignment{
		Title: "画集乙", TitleClueRef: "dirLevel1",
		Generalized: true, UpdatedAtMs: 1700000000001,
	}
	uniformYiSub := SessionAssignment{Author: "作者乙", AuthorClueRef: "dirLevel2", UpdatedAtMs: 1700000000002}
	legacy := map[string]SessionAssignment{
		"作者甲/a1.png":     uniformJia,
		"作者甲/a2.png":     uniformJia,
		"作者甲/sub/a3.png": uniformJia,
		"作者乙/b1.png":     uniformYi,
		"作者乙/b2.png":     uniformYi,
		"作者乙/sub/b3.png": uniformYiSub,
		"异值/c1.png":       {Author: "值一", UpdatedAtMs: 1},
		"异值/c2.png":       {Author: "值二", UpdatedAtMs: 2},
		"不存在/旧文件.jpg":  {Author: "旧作者", UpdatedAtMs: 1},
	}

	// 求值轮 A：v1 旧形态直接求值（兼容读取，未迁移）。重写信封为 v1 形态
	//（载荷簿记沿用，分片零触碰——同一份快照为两轮求值的共同底座）
	sessA, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	envA, _, err := m.readSessionEnvelope(root)
	if err != nil {
		t.Fatalf("读取信封簿记失败: %v", err)
	}
	lightA := sessionLightCopy(sessA)
	lightA.SchemaVersion = legacyWorkbenchSessionSchemaVersion
	lightA.Assignments = legacy
	dataA, err := json.Marshal(&sessionEnvelope{
		V: sessionEnvelopeVersion, Session: lightA,
		ShardSet: envA.ShardSet, Shards: envA.Shards, Codec: envA.Codec, Digest: envA.Digest,
	})
	if err != nil {
		t.Fatalf("构造 v1 信封失败: %v", err)
	}
	if err := m.store.SetValue(workbenchSessionKey(root), string(dataA)); err != nil {
		t.Fatalf("写入 v1 信封失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("v1 形态求值轮失败: %v", err)
	}
	sessRoundA, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("求值轮 A 读会话失败: %v", err)
	}
	anchorA := captureEvalAnchor(t, sessRoundA.Snapshot)

	// 重注入 v1 形态（求值轮 A 的保存已把信封升版）→ 触发懒压实
	sessB, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	envB, _, err := m.readSessionEnvelope(root)
	if err != nil {
		t.Fatalf("读取信封簿记失败: %v", err)
	}
	lightB := sessionLightCopy(sessB)
	lightB.SchemaVersion = legacyWorkbenchSessionSchemaVersion
	lightB.Assignments = legacy
	dataB, err := json.Marshal(&sessionEnvelope{
		V: sessionEnvelopeVersion, Session: lightB,
		ShardSet: envB.ShardSet, Shards: envB.Shards, Codec: envB.Codec, Digest: envB.Digest,
	})
	if err != nil {
		t.Fatalf("构造 v1 信封失败: %v", err)
	}
	if err := m.store.SetValue(workbenchSessionKey(root), string(dataB)); err != nil {
		t.Fatalf("写入 v1 信封失败: %v", err)
	}
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("懒压实变异失败: %v", err)
	}
	compactedSess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取压实后会话失败: %v", err)
	}
	if got, ok := compactedSess.Assignments["作者甲/"]; !ok || got.Author != "作者甲" {
		t.Fatalf("均匀整树应塌缩在 作者甲/: %+v（在场 %v）", got, ok)
	}
	if _, ok := compactedSess.Assignments["作者乙/"]; ok {
		t.Fatal("作者乙 直下有无条目文件（b0），不得归并")
	}
	if got, ok := compactedSess.Assignments["作者乙/sub/"]; !ok || got.Author != "作者乙" {
		t.Fatalf("作者乙 深层均匀子树应归并: %+v（在场 %v）", got, ok)
	}
	if compactedSess.AssignRevision != sessB.AssignRevision {
		t.Fatalf("压实不得递增 AssignRevision: 前 %d 后 %d", sessB.AssignRevision, compactedSess.AssignRevision)
	}

	// 求值轮 B：压实形态再求值 → 结论投影逐字节全等
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("压实形态求值轮失败: %v", err)
	}
	sessRoundB, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("求值轮 B 读会话失败: %v", err)
	}
	anchorB := captureEvalAnchor(t, sessRoundB.Snapshot)
	if !bytes.Equal(anchorA, anchorB) {
		t.Fatalf("迁移前后求值结论应逐字节全等:\nA=%s\nB=%s", anchorA, anchorB)
	}

	// 创建链子行赋值维度全等：buildChildResponses 产物（PluginData）对照
	rels := []string{
		"作者甲/a1.png", "作者甲/sub/a3.png", "作者乙/b0.png", "作者乙/b1.png",
		"作者乙/sub/b3.png", "异值/c1.png", "异值/c2.png", "顶层说明.txt",
	}
	items := make([]batchFileItem, 0, len(rels))
	for _, rel := range rels {
		items = append(items, batchFileItem{af: attributedFile{entry: FileEntry{
			FullPath: filepath.Join(root, filepath.FromSlash(rel)), RelPath: rel, Hash: "h-" + rel,
		}}})
	}
	childrenOld := buildChildResponses(items, legacy)
	childrenNew := buildChildResponses(items, compactedSess.Assignments)
	if len(childrenOld) != len(rels) || len(childrenNew) != len(rels) {
		t.Fatalf("子行构造应全量产出: 旧 %d 新 %d 期望 %d", len(childrenOld), len(childrenNew), len(rels))
	}
	assignedOld, assignedNew := 0, 0
	for i := range childrenOld {
		if childrenOld[i].PluginData != childrenNew[i].PluginData {
			t.Fatalf("子行 PluginData 应逐字节全等（%s）:\n旧=%s\n新=%s",
				rels[i], childrenOld[i].PluginData, childrenNew[i].PluginData)
		}
		var fp FilePluginData
		if err := json.Unmarshal([]byte(childrenOld[i].PluginData), &fp); err != nil {
			t.Fatalf("解析子任务 PluginData 失败: %v", err)
		}
		if fp.Assignment != nil {
			assignedOld++
		}
		var fpNew FilePluginData
		if err := json.Unmarshal([]byte(childrenNew[i].PluginData), &fpNew); err != nil {
			t.Fatalf("解析子任务 PluginData 失败: %v", err)
		}
		if fpNew.Assignment != nil {
			assignedNew++
		}
	}
	if assignedOld != assignedNew || assignedOld != 6 {
		t.Fatalf("子行赋值固化数应全等且覆盖全部已赋值文件（6=甲 3+乙 3+异值 2-1 无赋值行），旧 %d 新 %d", assignedOld, assignedNew)
	}
}

// TestAssignMigrateRecognitionRoundCompacts 识别轮完成路径（mutateSessionFull）
// 的懒压实：v1 旧形态会话不经任何赋值动作、直接重跑一轮识别 → 落库时压实
// （挂钩在 saveSessionFull 前：条目塌缩、版本 2、修订号不变、备份在场=v1
// 原文），无「版本盖 2 而赋值未压」残留；压实形态再识别的求值结论与 v1
// 形态识别轮逐字节全等。
func TestAssignMigrateRecognitionRoundCompacts(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)
	writeTestFile(t, root+"/作者甲/a1.png", "a")
	writeTestFile(t, root+"/作者甲/a2.png", "a")
	writeTestFile(t, root+"/作者甲/sub/a3.png", "a")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}

	// v1 逐文件旧形态注入（载荷簿记沿用、分片零触碰）
	uniform := SessionAssignment{
		Author: "作者甲", AuthorClueRef: "dirLevelStripped1",
		Tags: []string{"画师"}, TagClueRefs: []string{"dirLevel1"},
		UpdatedAtMs: 1700000000000,
	}
	legacy := map[string]SessionAssignment{
		"作者甲/a1.png":     uniform,
		"作者甲/a2.png":     uniform,
		"作者甲/sub/a3.png": uniform,
	}
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	revisionBefore := sess.AssignRevision
	env, _, err := m.readSessionEnvelope(root)
	if err != nil {
		t.Fatalf("读取信封簿记失败: %v", err)
	}
	light := sessionLightCopy(sess)
	light.SchemaVersion = legacyWorkbenchSessionSchemaVersion
	light.Assignments = legacy
	data, err := json.Marshal(&sessionEnvelope{
		V: sessionEnvelopeVersion, Session: light,
		ShardSet: env.ShardSet, Shards: env.Shards, Codec: env.Codec, Digest: env.Digest,
	})
	if err != nil {
		t.Fatalf("构造 v1 信封失败: %v", err)
	}
	if err := m.store.SetValue(workbenchSessionKey(root), string(data)); err != nil {
		t.Fatalf("写入 v1 信封失败: %v", err)
	}
	rawV1 := string(data)

	// 不经赋值动作、直接识别一轮（mutateSessionFull 路径）：求值按 v1 兼容
	// 读取完成，落库时懒压实
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("v1 形态识别轮失败: %v", err)
	}
	after, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("识别轮后读会话失败: %v", err)
	}
	if after.SchemaVersion != workbenchSessionSchemaVersion {
		t.Fatalf("识别轮完成应压实升级版本 2，实际 %d", after.SchemaVersion)
	}
	if got, ok := after.Assignments["作者甲/"]; !ok || got.Author != "作者甲" {
		t.Fatalf("识别轮路径应塌缩目录条目: %+v（在场 %v）", got, ok)
	}
	for _, key := range []string{"作者甲/a1.png", "作者甲/a2.png", "作者甲/sub/a3.png"} {
		if _, ok := after.Assignments[key]; ok {
			t.Fatalf("被塌缩消费的文件条目应消失: %s", key)
		}
	}
	if after.AssignRevision != revisionBefore {
		t.Fatalf("压实不得递增 AssignRevision: 前 %d 后 %d", revisionBefore, after.AssignRevision)
	}
	backup, err := m.store.GetValue(workbenchSessionMigrateBackupKey(root))
	if err != nil || backup == nil || backup.Value != rawV1 {
		t.Fatalf("识别轮路径的备份应存 v1 信封原文: 在场 %v err %v", backup != nil, err)
	}
	anchorV1 := captureEvalAnchor(t, after.Snapshot)

	// 压实形态再识别一轮：求值结论与 v1 形态识别轮逐字节全等
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("压实形态识别轮失败: %v", err)
	}
	after2, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("压实形态识别轮后读会话失败: %v", err)
	}
	if after2.SchemaVersion != workbenchSessionSchemaVersion || len(after2.Assignments) != 1 {
		t.Fatalf("再识别后应维持压实形态（单目录条目）: 版本 %d 条目 %d", after2.SchemaVersion, len(after2.Assignments))
	}
	anchorCompacted := captureEvalAnchor(t, after2.Snapshot)
	if !bytes.Equal(anchorV1, anchorCompacted) {
		t.Fatalf("识别轮迁移前后求值结论应逐字节全等:\nv1=%s\n压实=%s", anchorV1, anchorCompacted)
	}
}

// —— 退出标准⑦：用户形态夹具的塌缩与写税断言 ——

// TestAssignMigrateUserShapeCollapseAndWriteTax 用户形态夹具（500 目录 × 350
// 文件 = 175,000 文件 + 5 例外口袋 + 3 死路径，纯内存构造）：压实确定性双跑；
// 压实后赋值序列化 ≤1MB；压实后单动作信封序列化 ≤100KB。夹具实测：
// 旧形态赋值序列化 ~27MB（175,008 条目）、压实后 73KB（508 条目=500 目录
// 条目+5 例外+3 死路径）、单动作信封 ~74KB。
func TestAssignMigrateUserShapeCollapseAndWriteTax(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式跳过大夹具")
	}
	const dirCount = 500
	const filesPerDir = 350

	buildFixture := func() (*WorkbenchSession, map[string]SessionAssignment) {
		relPaths := make([]string, 0, dirCount*filesPerDir+8)
		assigns := make(map[string]SessionAssignment, dirCount*filesPerDir+8)
		root := `D:\用户形态库`
		for d := 0; d < dirCount; d++ {
			dirName := "画师" + itoa(d)
			value := SessionAssignment{
				Author: dirName, AuthorClueRef: "dirLevelStripped1",
				Tags: []string{"插画"}, TagClueRefs: []string{"dirLevel1"},
				UpdatedAtMs: 1700000000000,
			}
			for f := 0; f < filesPerDir; f++ {
				rel := dirName + "/p_" + itoa(f) + ".png"
				relPaths = append(relPaths, rel)
				assigns[rel] = value
			}
		}
		// 例外口袋（候选确认集名等历史手动动作残余，各值互异→该目录不归并、
		// 例外保持文件条目）与死路径（不在快照文件集）
		for i := 0; i < 5; i++ {
			rel := "特别收藏/e" + itoa(i) + ".png"
			relPaths = append(relPaths, rel)
			assigns[rel] = SessionAssignment{Collection: "星夜" + itoa(i), UpdatedAtMs: 1700000000100 + int64(i)}
		}
		for i := 0; i < 3; i++ {
			assigns["已删/旧" + itoa(i) + ".jpg"] = SessionAssignment{Author: "旧作者", UpdatedAtMs: 1}
		}
		return legacySeedSession(root, relPaths, assigns, 99), assigns
	}

	sess, legacy := buildFixture()
	compactOnce := compactLegacyAssignments(legacy, sess.Snapshot.Files)
	_, legacyAgain := buildFixture()
	compactTwice := compactLegacyAssignments(legacyAgain, sess.Snapshot.Files)
	once, err := json.Marshal(compactOnce)
	if err != nil {
		t.Fatalf("压实结果序列化失败: %v", err)
	}
	twice, err := json.Marshal(compactTwice)
	if err != nil {
		t.Fatalf("第二次压实结果序列化失败: %v", err)
	}
	if !bytes.Equal(once, twice) {
		t.Fatal("用户形态夹具压实确定性：双跑应逐字节相等")
	}
	legacyJSON, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("旧形态赋值序列化失败: %v", err)
	}
	t.Logf("夹具实测：旧形态赋值 %d 字节（%d 条目），压实后 %d 字节（%d 条目）",
		len(legacyJSON), len(legacy), len(once), len(compactOnce))
	if compactBound := 1 << 20; len(once) > compactBound {
		t.Fatalf("压实后赋值序列化应 ≤1MB，实际 %d 字节", len(once))
	}
	if want := dirCount + 5 + 3; len(compactOnce) != want { // 500 目录条目 + 例外口袋不归并（5 例外文件条目）+ 3 死路径
		t.Fatalf("压实后条目数应为 %d（目录 %d + 例外 5 + 死路径 3），实际 %d", want, dirCount, len(compactOnce))
	}

	// 写税断言：真实变异经管理器（装载分片载荷 → 懒压实 → 轻保存信封）
	store := newFakeSessionStore()
	m := NewSessionManagerWithStore(nil, nil, store)
	root := sess.RootPath
	seedLegacyV1Envelope(t, store, root, sess, nil)
	if _, err := m.mutateSession(root, func(*WorkbenchSession) error { return nil }); err != nil {
		t.Fatalf("用户形态变异（懒压实）失败: %v", err)
	}
	stored, err := store.GetValue(workbenchSessionKey(root))
	if err != nil || stored == nil || stored.Value == "" {
		t.Fatalf("读取压实后信封失败: %v", err)
	}
	t.Logf("夹具实测：压实后单动作信封 %d 字节", len(stored.Value))
	if writeTaxBound := 100 << 10; len(stored.Value) > writeTaxBound {
		t.Fatalf("压实后单动作信封序列化应 ≤100KB，实际 %d 字节", len(stored.Value))
	}
}
