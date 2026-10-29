// 规则草稿命中预览（PreviewRule）的单元测试：无会话/无识别快照的降级文案、
// 与保存后全量重识别的命中集一致性、求值参数面对齐（「在库名单」小标与贪婪
// 识别提取——库内谓词索引缺席会令这两类规则的预览命中/声明偏离保存后识别）、
// 纯门组合的样例退化（仅文件名+目录）、序列成员预览的同目录兄弟依赖，以及
// 前端事件协议的 previewRule 分派与只读性（档案字节不变）。
package main

import (
	"strings"
	"testing"
)

// previewFixtureTree 命中预览测试的样例树（相对库根；快照序=扫描的字典序：
// 序列 < 扫描 < 画集 < 静物）：
//
//	<root>/序列/成套_001..004.jpg        {键}_{序号} 成员×4（同键≥2，命中）
//	<root>/序列/落单_005.jpg             序号形态但同键计数 1 → 落单（不命中）
//	<root>/扫描/库内作者名_作品.jpg      文件名分段命中库内作者名（贪婪识别候选）
//	<root>/画集/人物画.jpg               「任意画」成员；标签不在库内名单
//	<root>/画集/风景画.jpg               「任意画」成员；标签「风景」在库内名单
//	<root>/静物/静物画.jpg               「任意画」成员；标签不在库内名单
func previewFixtureTree(t *testing.T, root string) {
	t.Helper()
	for _, name := range []string{"成套_001.jpg", "成套_002.jpg", "成套_003.jpg", "成套_004.jpg", "落单_005.jpg"} {
		writeTestFile(t, root+"/序列/"+name, "s")
	}
	writeTestFile(t, root+"/扫描/库内作者名_作品.jpg", "w")
	writeTestFile(t, root+"/画集/人物画.jpg", "p")
	writeTestFile(t, root+"/画集/风景画.jpg", "p")
	writeTestFile(t, root+"/静物/静物画.jpg", "p")
}

// newPreviewFixture 命中预览测试环境：内存存储 + 桩上下文（库内作者/标签名单）+
// 档案（一条无提取的「任意画」文件规则，作组合引用的成员）+ 样例树 + 已完成
// 全量识别的会话。
func newPreviewFixture(t *testing.T) (*SessionManager, *workbenchTestCtx, *fakeSessionStore, string) {
	t.Helper()
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	ctx.library = libraryFixture{
		localAuthors: []string{"库内作者名"},
		localTags:    []string{"风景"},
	}
	m := NewSessionManagerWithStore(ctx, nil, store)

	memberRows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{任意}画"},
	}}}
	memberPat, err := CompileRuleConditions(memberRows)
	if err != nil {
		t.Fatalf("构造成员规则失败: %v", err)
	}
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{{
		ID: "member-any-hua", Description: "预览测试成员：任意画", Source: RuleSourceManual, Enabled: true,
		Conditions: memberRows, Pattern: memberPat,
	}}}
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	previewFixtureTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, ctx, store, root
}

// previewConclusionsHitSet 预览求值结论中命中规则清单含 ruleID 的文件相对路径集。
func previewConclusionsHitSet(conclusions []FileConclusion, ruleID string) map[string]bool {
	hits := map[string]bool{}
	for i := range conclusions {
		for _, id := range conclusions[i].MatchedRuleIDs {
			if id == ruleID {
				hits[conclusions[i].RelPath] = true
				break
			}
		}
	}
	return hits
}

// relPathSetsEqual 两个相对路径集是否相等。
func relPathSetsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for key := range a {
		if !b[key] {
			return false
		}
	}
	return true
}

// TestPreviewRuleDegradationWithoutSnapshot 降级路径：库根无活动会话、会话尚无
// 识别快照均返回固定中文文案（前端禁用态同文案）；协议分派错误应答同文案。
func TestPreviewRuleDegradationWithoutSnapshot(t *testing.T) {
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)
	draft := []byte(`{"id":"preview-dir","description":"降级测试","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"画集"}}}],` +
		`"extractions":[],"synthesis":{}}`)

	if _, err := m.PreviewRule(root, draft); err == nil || err.Error() != ErrPreviewRuleNoSnapshot.Error() {
		t.Fatalf("无活动会话应返回降级文案，实际: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if _, err := m.PreviewRule(root, draft); err == nil || err.Error() != ErrPreviewRuleNoSnapshot.Error() {
		t.Fatalf("无识别快照应返回降级文案，实际: %v", err)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 201, Action: workbenchActionPreviewRule, Root: root, Rule: draft}))
	resp := ctx.lastResponse(t)
	if resp.OK || resp.RulePreview != nil || resp.Error != ErrPreviewRuleNoSnapshot.Error() {
		t.Fatalf("previewRule 无快照应错误应答且文案一致: OK=%v Error=%q", resp.OK, resp.Error)
	}
}

// TestPreviewRuleHitSetEqualsPostSaveRecognition 一致性锚：草稿预览的命中集与
// 草稿入档后全量重识别的命中集相等；预览只读（档案字节不变）；样例带草稿产出
// 的声明。
func TestPreviewRuleHitSetEqualsPostSaveRecognition(t *testing.T) {
	m, _, store, root := newPreviewFixture(t)
	draft := []byte(`{"id":"preview-dir","description":"预览草稿：画集目录规则","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"画集"}}}],` +
		`"extractions":[{"dimension":"title","from":"constant","constant":"画集文件"}],"synthesis":{}}`)

	before := store.values[RuleProfileKVKey]
	preview, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("命中预览失败: %v", err)
	}
	if store.values[RuleProfileKVKey] != before {
		t.Fatal("命中预览不应改写规则档案")
	}
	if preview.HitTotal != 9 {
		t.Fatalf("求值面总数 = %d，期望 9（快照文件总数）", preview.HitTotal)
	}
	// 目录目标命中整个子树：画集目录下两文件
	if preview.HitCount != 2 {
		t.Fatalf("命中数 = %d，期望 2", preview.HitCount)
	}
	if len(preview.Samples) != 2 {
		t.Fatalf("样例 = %d 条，期望 2", len(preview.Samples))
	}
	for _, s := range preview.Samples {
		if s.DirRelKey != "画集" || s.Result != "作品名=画集文件" {
			t.Fatalf("样例不符（%s / %s）: %+v", s.DirRelKey, s.Result, s)
		}
	}

	conclusions, entry, err := m.previewRuleConclusions(root, draft)
	if err != nil {
		t.Fatalf("预览求值失败: %v", err)
	}
	previewHits := previewConclusionsHitSet(conclusions, entry.ID)

	if _, err := m.ApplySaveRule(draft); err != nil {
		t.Fatalf("草稿入档失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量重识别失败: %v", err)
	}
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	afterHits := map[string]bool{}
	for i := range sess.Snapshot.Files {
		for _, id := range sess.Snapshot.Files[i].MatchedRuleIDs {
			if id == "preview-dir" {
				afterHits[sess.Snapshot.Files[i].RelPath] = true
			}
		}
	}
	want := map[string]bool{"画集/人物画.jpg": true, "画集/风景画.jpg": true}
	if !relPathSetsEqual(afterHits, want) {
		t.Fatalf("保存后重识别命中集不符: %v（期望 %v）", afterHits, want)
	}
	if !relPathSetsEqual(previewHits, afterHits) {
		t.Fatalf("预览命中集与保存后重识别不一致: 预览=%v 重识别=%v", previewHits, afterHits)
	}
}

// TestPreviewRuleLibraryBadgeAlignment 参数面对齐用例一（在库名单小标）：带
// library 小标的规则依赖库内谓词索引——预览命中与声明须与保存后真实识别一致
//（索引缺席时小标门恒不命中，命中数即偏离）。
func TestPreviewRuleLibraryBadgeAlignment(t *testing.T) {
	m, _, _, root := newPreviewFixture(t)
	draft := []byte(`{"id":"preview-badge","description":"预览草稿：标签在库名单","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{标签}画"},` +
		`"badges":[{"placeholder":"标签","kind":"library","dim":"tag"}]}}],` +
		`"extractions":[{"dimension":"tag","from":"capture","captureName":"tag","row":1}],"synthesis":{}}`)

	preview, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("命中预览失败: %v", err)
	}
	// 模板命中人物画/风景画/静物画三文件，库内名单（标签=风景）只放行风景画
	if preview.HitCount != 1 {
		t.Fatalf("命中数 = %d，期望 1（在库名单小标过滤后）", preview.HitCount)
	}
	if len(preview.Samples) != 1 || preview.Samples[0].FileName != "风景画.jpg" ||
		preview.Samples[0].DirRelKey != "画集" || preview.Samples[0].Result != "标签=风景" {
		t.Fatalf("样例不符: %+v", preview.Samples)
	}

	if _, err := m.ApplySaveRule(draft); err != nil {
		t.Fatalf("草稿入档失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量重识别失败: %v", err)
	}
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	afterHits := map[string]bool{}
	for i := range sess.Snapshot.Files {
		for _, id := range sess.Snapshot.Files[i].MatchedRuleIDs {
			if id == "preview-badge" {
				afterHits[sess.Snapshot.Files[i].RelPath] = true
			}
		}
	}
	if !relPathSetsEqual(afterHits, map[string]bool{"画集/风景画.jpg": true}) {
		t.Fatalf("保存后重识别命中集不符: %v", afterHits)
	}
	for i := range sess.Snapshot.Files {
		if !afterHits[sess.Snapshot.Files[i].RelPath] {
			continue
		}
		tagOfDraft := ""
		for _, decl := range sess.Snapshot.Files[i].Final {
			if decl.RuleID == "preview-badge" && decl.Dimension == string(DimTag) {
				tagOfDraft = decl.Value
			}
		}
		if tagOfDraft != "风景" {
			t.Fatalf("保存后识别的草稿声明不符: %q（期望 风景）", tagOfDraft)
		}
	}
}

// TestPreviewRuleScanExtractionAlignment 参数面对齐用例二（贪婪识别提取）：
// FromScan 动作依赖库内集合比对——预览命中与声明须与保存后真实识别一致
//（索引缺席时规则照常命中但零声明，样例声明即偏离）。
func TestPreviewRuleScanExtractionAlignment(t *testing.T) {
	m, _, _, root := newPreviewFixture(t)
	draft := []byte(`{"id":"preview-scan","description":"预览草稿：贪婪识别提取作者","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"库内作者名_{任意}"}}}],` +
		`"extractions":[{"dimension":"author","from":"scan","scanScope":"path"}],"synthesis":{}}`)

	preview, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("命中预览失败: %v", err)
	}
	if preview.HitCount != 1 {
		t.Fatalf("命中数 = %d，期望 1", preview.HitCount)
	}
	if len(preview.Samples) != 1 || preview.Samples[0].FileName != "库内作者名_作品.jpg" ||
		preview.Samples[0].DirRelKey != "扫描" || preview.Samples[0].Result != "作者=库内作者名" {
		t.Fatalf("样例不符（贪婪识别声明依赖库内谓词索引）: %+v", preview.Samples)
	}

	if _, err := m.ApplySaveRule(draft); err != nil {
		t.Fatalf("草稿入档失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量重识别失败: %v", err)
	}
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	for i := range sess.Snapshot.Files {
		if sess.Snapshot.Files[i].RelPath != "扫描/库内作者名_作品.jpg" {
			continue
		}
		authorOfDraft := ""
		for _, decl := range sess.Snapshot.Files[i].Final {
			if decl.RuleID == "preview-scan" && decl.Dimension == string(DimAuthor) {
				authorOfDraft = decl.Value
			}
		}
		if authorOfDraft != "库内作者名" {
			t.Fatalf("保存后识别的草稿声明不符: %q（期望 库内作者名）", authorOfDraft)
		}
	}
}

// TestPreviewRulePureGateComboSampleDegradation 纯门组合：引用成员、自身零提取
// ——命中照常计数，样例退化为仅文件名+目录（无产出声明可显）；跨目录样例
// 优先、不足回落同目录。
func TestPreviewRulePureGateComboSampleDegradation(t *testing.T) {
	m, _, _, root := newPreviewFixture(t)
	draft := []byte(`{"id":"preview-gate-combo","description":"预览草稿：纯门组合","source":"manual","enabled":true,` +
		`"refs":["member-any-hua"],"conditions":[],"extractions":[],"synthesis":{}}`)

	preview, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("命中预览失败: %v", err)
	}
	// 门段=成员（任意画）全体命中：人物画/风景画（画集）+ 静物画（静物）
	if preview.HitCount != 3 {
		t.Fatalf("命中数 = %d，期望 3", preview.HitCount)
	}
	if len(preview.Samples) != 3 {
		t.Fatalf("样例 = %d 条，期望 3（两目录共 3 命中：跨目录 2 + 回落 1）", len(preview.Samples))
	}
	dirs := map[string]int{}
	for _, s := range preview.Samples {
		if s.Result != "" {
			t.Fatalf("纯门组合样例不应携带声明: %+v", s)
		}
		dirs[s.DirRelKey]++
	}
	if dirs["画集"] != 2 || dirs["静物"] != 1 {
		t.Fatalf("样例目录分布不符（跨目录优先、不足回落同目录）: %v", dirs)
	}
	if strings.Contains(mustMarshalPreview(t, preview.Samples), "result") {
		t.Fatalf("纯门组合样例应仅文件名+目录（result 键缺省）: %s", mustMarshalPreview(t, preview.Samples))
	}

	conclusions, entry, err := m.previewRuleConclusions(root, draft)
	if err != nil {
		t.Fatalf("预览求值失败: %v", err)
	}
	previewHits := previewConclusionsHitSet(conclusions, entry.ID)
	if _, err := m.ApplySaveRule(draft); err != nil {
		t.Fatalf("草稿入档失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量重识别失败: %v", err)
	}
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	afterHits := map[string]bool{}
	for i := range sess.Snapshot.Files {
		for _, id := range sess.Snapshot.Files[i].MatchedRuleIDs {
			if id == "preview-gate-combo" {
				afterHits[sess.Snapshot.Files[i].RelPath] = true
			}
		}
	}
	if !relPathSetsEqual(previewHits, afterHits) {
		t.Fatalf("预览命中集与保存后重识别不一致: 预览=%v 重识别=%v", previewHits, afterHits)
	}
}

// TestPreviewRuleSeqMemberNeedsSiblings 序列成员预览不失准：{键}_{序号} 成员的
// 求值输入须含同目录兄弟清单（同键计数≥2 才命中）——兄弟缺席的简化求值面下
// 序列目标恒不命中。落单文件（同键计数 1）不命中；草稿缺省 ID 按保存同款生成。
func TestPreviewRuleSeqMemberNeedsSiblings(t *testing.T) {
	m, _, _, root := newPreviewFixture(t)
	draft := []byte(`{"description":"预览草稿：成套序列成员","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"seq","format":{"mode":"template","tpl":"{键}_{序号}"}}}],` +
		`"extractions":[{"dimension":"collection","from":"capture","captureName":"key","row":1}],"synthesis":{}}`)

	conclusions, entry, err := m.previewRuleConclusions(root, draft)
	if err != nil {
		t.Fatalf("预览求值失败: %v", err)
	}
	if entry.ID == "" || !strings.HasPrefix(entry.ID, "manual-") {
		t.Fatalf("缺省 ID 应按保存同款生成（manual- 前缀），实际 %q", entry.ID)
	}
	previewHits := previewConclusionsHitSet(conclusions, entry.ID)
	want := map[string]bool{
		"序列/成套_001.jpg": true, "序列/成套_002.jpg": true,
		"序列/成套_003.jpg": true, "序列/成套_004.jpg": true,
	}
	if !relPathSetsEqual(previewHits, want) {
		t.Fatalf("预览命中集不符（同键≥2 成套命中、落单出局）: %v", previewHits)
	}
	if previewHits["序列/落单_005.jpg"] {
		t.Fatal("落单文件不应命中（同键计数 1 < 2）")
	}

	preview, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("命中预览失败: %v", err)
	}
	if preview.HitCount != 4 {
		t.Fatalf("命中数 = %d，期望 4", preview.HitCount)
	}
	// 4 命中同目录：跨目录样例 1 条 + 回落补足至 3；每条带 {键} 捕获声明
	if len(preview.Samples) != 3 {
		t.Fatalf("样例 = %d 条，期望 3（同目录回落补足）", len(preview.Samples))
	}
	for _, s := range preview.Samples {
		if s.DirRelKey != "序列" || s.Result != "作品集=成套" {
			t.Fatalf("样例不符: %+v", s)
		}
	}

	// 缺省 ID 草稿入档（保存侧另行生成 ID）→ 重识别命中集仍与预览相等
	view, err := m.ApplySaveRule(draft)
	if err != nil {
		t.Fatalf("草稿入档失败: %v", err)
	}
	savedID := view.Rules[len(view.Rules)-1].ID
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量重识别失败: %v", err)
	}
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	afterHits := map[string]bool{}
	for i := range sess.Snapshot.Files {
		for _, id := range sess.Snapshot.Files[i].MatchedRuleIDs {
			if id == savedID {
				afterHits[sess.Snapshot.Files[i].RelPath] = true
			}
		}
	}
	if !relPathSetsEqual(previewHits, afterHits) {
		t.Fatalf("预览命中集与保存后重识别不一致: 预览=%v 重识别=%v", previewHits, afterHits)
	}
}

// mustMarshalPreview 序列化样例清单（断言 JSON 形态用）。
func mustMarshalPreview(t *testing.T, samples []rulesPreviewSample) string {
	t.Helper()
	return mustMarshal(t, samples)
}
