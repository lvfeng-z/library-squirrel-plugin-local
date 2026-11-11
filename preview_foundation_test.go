// 预览求值基座（会话级中间产物缓存）单测：识别完成铸造在位/守卫失配回退/
// 逐出重建/增量轮规则相关件携带/在库名单缓存生命周期，以及基座供给求值与
// 现行现场重建路径的等值锚（同一规则集逐字段等值——行为零变化的构造性证明）。
package main

import (
	"reflect"
	"testing"
)

// foundationFixture 基座测试环境：样例树 + 结构对齐目录（`月华NN.png` 无
// 下划线——兄弟定群规则的键退化为整名逐文件唯一、不被规则定群，词内数字
// 子段独立成序号元素，前置对齐成结构群）+ 测试档案 + 会话 + 首轮全量识别
//（recognition_test.go 的识别夹具同构，识别完成即基座就位）。
func foundationFixture(t *testing.T, root string) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	m, ctx := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)
	writeTestFile(t, root+"/对齐/月华01.png", "x")
	writeTestFile(t, root+"/对齐/月华02.png", "x")
	writeTestFile(t, root+"/对齐/月华03.png", "x")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	return m, ctx
}

// foundationFilesOf 现行现场重建形态的求值清单（快照文件 → ScanFile，与
// rules_preview.go / 三条建议卡的回退路径同构）。
func foundationFilesOf(snap *RecognitionSnapshot) []ScanFile {
	files := make([]ScanFile, 0, len(snap.Files))
	for i := range snap.Files {
		f := &snap.Files[i]
		files = append(files, ScanFile{FileName: f.FileName, DirNames: ExtractDirLevels(f.RelPath), RelPath: f.RelPath})
	}
	return files
}

// TestPreviewFoundationMintAndHit 识别完成后基座在位与内容对账：规则无关层
// （清单同序/兄弟表/元素拆分）与规则相关层（守卫键/目录覆盖/逐文件原始声明）
// 逐面断言，且基座供给求值与现行路径等值。
func TestPreviewFoundationMintAndHit(t *testing.T) {
	root := t.TempDir()
	m, _ := foundationFixture(t, root)

	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	f := m.previewFoundationOf(sess)
	if f == nil {
		t.Fatal("识别完成后基座应就位（守卫=共享会话实例与铸造快照指针等值）")
	}
	if f.snapshot != sess.Snapshot {
		t.Fatal("基座铸造来源快照应为当前会话快照实例")
	}
	snap := sess.Snapshot
	sub := f.indep
	if sub == nil || len(sub.files) != len(snap.Files) {
		t.Fatalf("规则无关层清单缺席或长度不符：%v / 快照 %d", sub, len(snap.Files))
	}
	// 清单与快照同序对位、目录层与现行重建同构
	for i := range snap.Files {
		want := ExtractDirLevels(snap.Files[i].RelPath)
		got := sub.files[i]
		if got.RelPath != snap.Files[i].RelPath || got.FileName != snap.Files[i].FileName || !reflect.DeepEqual(got.DirNames, want) {
			t.Fatalf("清单第 %d 位与快照不同构：%+v vs %s", i, got, snap.Files[i].RelPath)
		}
	}
	// 兄弟表：样例树「序列」目录 4 文件同键聚合
	if got := len(sub.siblingsByDir[dirGroupKey([]string{"序列"})]); got != 4 {
		t.Fatalf("序列目录兄弟表应含 4 文件，实际 %d", got)
	}
	// 元素拆分预铸件与现行口径等值（抽规则定群成员与根直下文件各一）
	for _, rel := range []string{"序列/库内作者名_001.jpg", "12345678_p0.jpg"} {
		fileName := snap.Files[0].FileName
		for i := range snap.Files {
			if snap.Files[i].RelPath == rel {
				fileName = snap.Files[i].FileName
			}
		}
		want := splitFilenameElements(BaseNameWithoutExt(fileName), dirNameHints(ExtractDirLevels(rel)))
		if got := sub.elementsOf(rel); !reflect.DeepEqual(got, want) {
			t.Fatalf("文件 %s 的预铸元素与现行拆分不等值：%+v vs %+v", rel, got, want)
		}
	}
	// 元素取用发副本：改写取用结果不得污染缓存原件
	rel := "序列/库内作者名_001.jpg"
	got := sub.elementsOf(rel)
	if len(got) > 0 {
		got[0].Kind = elKindTitle
		if again := sub.elementsOf(rel); again[0].Kind == elKindTitle {
			t.Fatal("元素取用应发副本，改写不得回流缓存原件")
		}
	}
	// 规则相关层：全量轮全目录覆盖、守卫键与快照一致、原始声明在场
	if f.rule == nil {
		t.Fatal("规则相关层应随基座铸造")
	}
	if !f.rule.guardMatches(snap.ProfileDigest, snap.EvaluatorBuildID) {
		t.Fatal("规则相关层守卫应与快照三件套一致")
	}
	for _, d := range snap.Dirs {
		if !f.rule.coveredDirKeys[d.RelKey] {
			t.Fatalf("全量轮规则段应覆盖目录 %q", d.RelKey)
		}
	}
	seg := f.rule.segmentsByRel["已识别/【作者A】标题1.jpg"]
	if seg == nil || len(seg.declarations) == 0 {
		t.Fatalf("规则定群成员的规则段结论缺席或无原始声明：%+v", seg)
	}
	if len(f.rule.structuralByRel) == 0 {
		t.Fatal("结构群对齐结果回收缺席（对齐目录应成结构群）")
	}
	if f.rule.structuralByRel["对齐/月华02.png"] == nil {
		t.Fatal("结构群成员的对齐结果应按相对路径登记")
	}

	// —— 等值锚：基座供给求值与现行现场重建路径逐字段等值 ——
	rules := recognitionTestRules(t)
	direct := MergedRecognition(foundationFilesOf(snap), rules, nil, nil)
	onSub := MergedRecognitionOnSubstrate(sub, rules, nil, nil)
	if !reflect.DeepEqual(direct, onSub) {
		t.Fatal("基座供给求值与现行路径结论应逐字段等值")
	}
	// 三条建议卡命中统计与样例的等值（回退 nil vs 基座供给）
	h1, total1, s1 := previewStructRules(rules, snap, "序列/lone_007.jpg", nil)
	h2, total2, s2 := previewStructRules(rules, snap, "序列/lone_007.jpg", sub)
	if h1 != h2 || total1 != total2 || !reflect.DeepEqual(s1, s2) {
		t.Fatalf("结构卡基座供给与回退结果不等值：%d/%d %+v vs %d/%d %+v", h1, total1, s1, h2, total2, s2)
	}
	h1, total1, s1 = previewSequenceFormRules(rules, snap, "序列/lone_007.jpg", nil)
	h2, total2, s2 = previewSequenceFormRules(rules, snap, "序列/lone_007.jpg", sub)
	if h1 != h2 || total1 != total2 || !reflect.DeepEqual(s1, s2) {
		t.Fatalf("序列卡基座供给与回退结果不等值：%d/%d %+v vs %d/%d %+v", h1, total1, s1, h2, total2, s2)
	}
	h1, total1, s1 = previewWorkSetRules(rules, snap, "冲突", "test-conflict-a", nil)
	h2, total2, s2 = previewWorkSetRules(rules, snap, "冲突", "test-conflict-a", sub)
	if h1 != h2 || total1 != total2 || !reflect.DeepEqual(s1, s2) {
		t.Fatalf("集卡基座供给与回退结果不等值：%d/%d %+v vs %d/%d %+v", h1, total1, s1, h2, total2, s2)
	}
}

// TestPreviewFoundationGuardFallback 守卫失配回退：基座铸造来源快照与当前
// 快照失配（换代）或基座条目被清时取用返回 nil，卡路径回退现行重建路径结果
// 正确；规则相关件守卫（档案指纹/求值器构建标识）的等值比较单点锚。
func TestPreviewFoundationGuardFallback(t *testing.T) {
	root := t.TempDir()
	m, _ := foundationFixture(t, root)

	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	norm := normalizeLibraryRoot(root)
	// 扰动①：基座铸造来源快照失配（模拟快照换代后残留的旧基座）
	m.sessCacheMu.Lock()
	if entry, ok := m.sessCache[norm]; ok {
		entry.foundation = &previewFoundation{snapshot: &RecognitionSnapshot{}}
	}
	m.sessCacheMu.Unlock()
	if m.evalSubstrateOf(sess) != nil {
		t.Fatal("快照指针失配时基座不可用")
	}
	// 回退路径结果与基座就位时等值（现行重建路径原样）
	rules := recognitionTestRules(t)
	h1, total1, s1 := previewSequenceFormRules(rules, sess.Snapshot, "序列/lone_007.jpg", nil)
	if total1 != len(sess.Snapshot.Files) {
		t.Fatalf("回退路径求值面应=快照全量 %d，实际 %d", len(sess.Snapshot.Files), total1)
	}
	if h1 == 0 && len(s1) != 0 {
		t.Fatal("回退路径结果形态异常")
	}
	// 扰动②：基座条目清空（逐出/未铸形态）
	m.sessCacheMu.Lock()
	if entry, ok := m.sessCache[norm]; ok {
		entry.foundation = nil
	}
	m.sessCacheMu.Unlock()
	if m.evalSubstrateOf(sess) != nil {
		t.Fatal("基座缺席时取用应返回 nil")
	}
	// 会话实例失配（变异旁路私有实例不入缓存）
	fresh, err := m.loadSessionFresh(root)
	if err != nil {
		t.Fatalf("旁路读会话失败: %v", err)
	}
	if m.previewFoundationOf(fresh) != nil {
		t.Fatal("私有实例对基座的取用应恒 miss")
	}

	// 规则相关件守卫等值比较（与识别增量闸门同款形态）
	layer := &previewRuleLayer{profileDigest: "d1", evaluatorBuildID: "b1"}
	if !layer.guardMatches("d1", "b1") {
		t.Fatal("指纹与构建标识一致应放行")
	}
	if layer.guardMatches("d2", "b1") {
		t.Fatal("档案指纹失配应拒绝")
	}
	if layer.guardMatches("d1", "b2") {
		t.Fatal("求值器构建标识失配应拒绝")
	}
	if !layer.guardMatches("d1", "") {
		t.Fatal("当前构建未打标（空串）应缺席放行")
	}
	if (&previewRuleLayer{profileDigest: "d1", evaluatorBuildID: ""}).guardMatches("d1", "b1") {
		t.Fatal("上轮未打标而当前打标（升级换版）应拒绝")
	}
	var nilLayer *previewRuleLayer
	if nilLayer.guardMatches("d1", "b1") {
		t.Fatal("空层应拒绝")
	}
}

// TestPreviewFoundationIncrementalCarry 增量轮规则相关件携带：mtime 增量轮
// 未变目录条目按指针携带（不重算）、变更目录条目整目录更新；档案变更的
// 全量轮重铸新层（条目指针全新、指纹随新档案）。
func TestPreviewFoundationIncrementalCarry(t *testing.T) {
	root := t.TempDir()
	m, ctx := foundationFixture(t, root)

	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	f1 := m.previewFoundationOf(sess)
	if f1 == nil {
		t.Fatal("首轮基座应就位")
	}
	const oldRel = "已识别/【作者A】标题1.jpg"
	if f1.rule.segmentsByRel[oldRel] == nil {
		t.Fatal("首轮规则段应含样例文件")
	}

	// 增量轮：新目录落新文件，既有目录 mtime 未变
	writeTestFile(t, root+"/新目录/新增_001.jpg", "x")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("增量识别失败: %v", err)
	}
	sess2, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	f2 := m.previewFoundationOf(sess2)
	if f2 == nil {
		t.Fatal("增量轮后基座应就位")
	}
	if f2 == f1 || f2.snapshot != sess2.Snapshot {
		t.Fatal("增量轮应铸造新基座（新快照实例）")
	}
	// 未变目录条目携带（指针同一——不重算）
	if f2.rule.segmentsByRel[oldRel] != f1.rule.segmentsByRel[oldRel] {
		t.Fatal("未变目录的规则段条目应按指针携带上轮产物")
	}
	// 变更目录条目更新：新文件条目在场、覆盖含新旧全部目录
	if f2.rule.segmentsByRel["新目录/新增_001.jpg"] == nil {
		t.Fatal("变更目录的新文件规则段应入场")
	}
	for _, d := range sess2.Snapshot.Dirs {
		if !f2.rule.coveredDirKeys[d.RelKey] {
			t.Fatalf("增量轮规则段覆盖应含目录 %q", d.RelKey)
		}
	}
	if len(f2.indep.files) != len(sess2.Snapshot.Files) {
		t.Fatalf("增量轮规则无关层清单应随新快照全量：%d vs %d", len(f2.indep.files), len(sess2.Snapshot.Files))
	}

	// 全量轮：档案变更令指纹失配，规则段整层重铸
	profile, errs := LoadProfile(ctx.store)
	if len(errs) > 0 || profile == nil {
		t.Fatalf("载入规则档案失败: %v", errs)
	}
	profile.Rules = append(profile.Rules, RuleEntry{
		ID: "foundation-carry-extra", Description: "测试：档案变更触发全量重铸",
		Source: RuleSourceManual, Enabled: true, Pattern: mustRegexPattern(t, `^never_.+\.jpg$`, nil),
	})
	if err := SaveProfile(ctx.store, profile); err != nil {
		t.Fatalf("写入规则档案失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("档案变更后的全量识别失败: %v", err)
	}
	sess3, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	f3 := m.previewFoundationOf(sess3)
	if f3 == nil {
		t.Fatal("全量轮后基座应就位")
	}
	if f3.rule.segmentsByRel[oldRel] == f2.rule.segmentsByRel[oldRel] {
		t.Fatal("档案变更的全量轮应重铸规则段（条目指针全新）")
	}
	if !f3.rule.guardMatches(sess3.Snapshot.ProfileDigest, sess3.Snapshot.EvaluatorBuildID) {
		t.Fatal("重铸层的守卫键应与新快照一致")
	}
}

// TestPreviewFoundationEvictionRebuild 缓存逐出连带基座逐出：逐出后基座取用
// miss、预览回退现行路径不受影响；再识别即重铸在位。
func TestPreviewFoundationEvictionRebuild(t *testing.T) {
	root := t.TempDir()
	m, _ := foundationFixture(t, root)

	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if m.previewFoundationOf(sess) == nil {
		t.Fatal("前置：基座应在位")
	}
	m.evictSessionCache(normalizeLibraryRoot(root))
	sess, err = m.SessionOf(root) // 重装填（全新解码实例）
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if m.previewFoundationOf(sess) != nil {
		t.Fatal("逐出后基座不应随重装填复活")
	}
	// 回退路径预览正常（弹窗走现行全量）
	pv, err := m.PreviewRule(root, []byte(previewPerfDraftJSON))
	if err != nil {
		t.Fatalf("逐出后弹窗预览应回退现行路径正常应答: %v", err)
	}
	if pv.HitTotal != len(sess.Snapshot.Files) {
		t.Fatalf("回退预览求值面应=快照全量 %d，实际 %d", len(sess.Snapshot.Files), pv.HitTotal)
	}
	// 再识别重铸
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("再识别失败: %v", err)
	}
	sess, err = m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if m.previewFoundationOf(sess) == nil {
		t.Fatal("再识别后基座应重铸在位")
	}
}

// TestLibraryIndexCacheLifecycle 在库名单缓存生命周期：弹窗预览首用构造一次、
// 复用不重复构造、轻保存条目重建跨条目携带、收批失效后重建。
func TestLibraryIndexCacheLifecycle(t *testing.T) {
	root := t.TempDir()
	m, ctx := foundationFixture(t, root)

	preview := func() *rulesPreviewResult {
		t.Helper()
		pv, err := m.PreviewRule(root, []byte(previewPerfDraftJSON))
		if err != nil {
			t.Fatalf("弹窗预览失败: %v", err)
		}
		return pv
	}
	// 首用构造一次
	before := ctx.indexBuildSeq.Load()
	first := preview()
	if got := ctx.indexBuildSeq.Load(); got != before+1 {
		t.Fatalf("首次弹窗预览应构造名单索引一次：%d → %d", before, got)
	}
	// 复用不重复构造
	second := preview()
	if got := ctx.indexBuildSeq.Load(); got != before+1 {
		t.Fatalf("缓存命中不应重复构造名单索引：%d → %d", before+1, got)
	}
	if first.HitTotal != second.HitTotal || first.HitCount != second.HitCount {
		t.Fatalf("两次预览结果应稳定：%+v vs %+v", first, second)
	}
	// 轻保存条目重建（赋值写走 saveSession→putSessionCache）跨条目携带
	if _, err := m.SetAssignment(root, "顶层说明.txt", SessionAssignment{Author: "甲"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	preview()
	if got := ctx.indexBuildSeq.Load(); got != before+1 {
		t.Fatalf("轻保存条目重建应携带名单缓存（收批外不变失效）：%d → %d", before+1, got)
	}
	// 收批失效后重建
	m.invalidateLibraryIndexCache()
	preview()
	if got := ctx.indexBuildSeq.Load(); got != before+2 {
		t.Fatalf("收批失效后应重建名单索引：%d → %d", before+1, got)
	}
}

// TestPreviewFoundationCarriesAcrossAssignmentSave 阶段5 携带/守卫改造：赋值写
//（轻保存条目重建）后基座经文件集身份键等值携带——快照实例换代（指针失配）
// 不再丢基座，规则段层仍可取用；赋值增量重算轮（快分支）本身也保持基座在位
//（不重铸——规则段结论与赋值无关）；文件集换代（识别轮换新快照）即失配不携带。
func TestPreviewFoundationCarriesAcrossAssignmentSave(t *testing.T) {
	root := t.TempDir()
	m, _ := foundationFixture(t, root)

	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	f1 := m.previewFoundationOf(sess)
	if f1 == nil {
		t.Fatal("前置：基座应在位")
	}
	// 赋值写（轻保存）→ 缓存条目重建（快照为新解码实例，指针与铸造来源失配）
	// → 基座经文件集身份键携带在位（阶段2 双指针守卫形态下此处即丢）
	if _, err := m.SetAssignment(root, "顶层说明.txt", SessionAssignment{Author: "甲"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	sess, err = m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	f2 := m.previewFoundationOf(sess)
	if f2 == nil {
		t.Fatal("赋值写（轻保存条目重建）后基座应经文件集身份键携带在位")
	}
	if f2 != f1 {
		t.Fatal("携带应为同一基座实例（规则段结论与赋值无关，不重铸）")
	}
	if f2.snapshot == sess.Snapshot {
		t.Fatal("携带形态下铸造来源快照实例应已换代（指针失配、值等值）")
	}
	if !f2.rule.guardMatches(sess.Snapshot.ProfileDigest, sess.Snapshot.EvaluatorBuildID) {
		t.Fatal("携带后的规则相关件守卫应仍与快照一致")
	}
	if f2.rule.segmentsByRel["已识别/【作者A】标题1.jpg"] == nil {
		t.Fatal("携带后的规则段条目应仍在场")
	}
	// 赋值增量重算轮：快分支保存新快照（文件集未变）——基座继续携带在位
	before := assignIncrementalTaken.Load()
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("赋值增量重识别失败: %v", err)
	}
	if got := assignIncrementalTaken.Load(); got != before+1 {
		t.Fatalf("赋值变更刷新轮应走赋值增量：%d → %d", before, got)
	}
	sess, err = m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if m.previewFoundationOf(sess) == nil {
		t.Fatal("赋值增量轮后基座应仍在位（文件集未变，保存路径值守卫携带）")
	}
}
