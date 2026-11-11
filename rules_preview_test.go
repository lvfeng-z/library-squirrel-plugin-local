// 规则草稿命中预览（PreviewRule）的单元测试：无会话/无识别快照的降级文案、
// 与保存后全量重识别的命中集一致性、求值参数面对齐（「在库名单」小标与贪婪
// 识别提取——库内谓词索引缺席会令这两类规则的预览命中/声明偏离保存后识别）、
// 纯门组合的样例退化（仅文件名+目录）、序列成员预览的同目录兄弟依赖，以及
// 前端事件协议的 previewRule 分派与只读性（档案字节不变）。
package main

import (
	"reflect"
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

// —— 弹窗草稿快路径（阶段3）：五形态等值锚与回退分箱 ——

// newPreviewFastFixture 快路径测试环境：样例树 + 结构对齐目录 + 档案（定群
// 规则、【】命名惯例、双冲突规则、目录线索规则——不含库内谓词依赖规则）+
// 冲突文件会话赋值（赋值覆盖面入锚）+ 首轮全量识别（基座就位）。extraArchives
// 追加档案规则（组合引用/条件动作等回退形态的夹具变体）。
func newPreviewFastFixture(t *testing.T, extraArchives ...RuleEntry) (*SessionManager, string) {
	t.Helper()
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	ctx.library = libraryFixture{localAuthors: []string{"库内作者名"}, localTags: []string{"风景"}}
	m := NewSessionManagerWithStore(ctx, nil, store)

	siblingPattern, err := NewSiblingPattern(`^([^_]+)_`, `(\d+)`, 2, OutcomeCollection)
	if err != nil {
		t.Fatalf("构造兄弟群组规则失败: %v", err)
	}
	bracketRows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "【{作者}】{标题}"},
	}}}
	bracketPattern, err := CompileRuleConditions(bracketRows)
	if err != nil {
		t.Fatalf("构造括号命名惯例模板规则失败: %v", err)
	}
	dirPattern, err := NewLocationPattern(0, 0, mustRegexPattern(t, `^线索$`, nil))
	if err != nil {
		t.Fatalf("构造目录层位置模式失败: %v", err)
	}
	rules := []RuleEntry{
		{
			ID: "test-sibling-set", Description: "快路径夹具：下划线前缀+序号成群", Source: RuleSourceManual, Enabled: true,
			Pattern: siblingPattern,
		},
		{
			ID: "test-bracket-author-title", Description: "快路径夹具：【作者】标题命名惯例", Source: RuleSourceManual, Enabled: true,
			Conditions: bracketRows, Pattern: bracketPattern,
			Extractions: []ExtractionAction{
				{Dimension: DimAuthor, From: FromCapture, CaptureName: condxVocab["作者"], Row: 1},
				{Dimension: DimTitle, From: FromCapture, CaptureName: condxVocab["标题"], Row: 1},
			},
		},
		{
			ID: "test-conflict-a", Description: "快路径夹具：标题冲突 A", Source: RuleSourceManual, Enabled: true,
			Pattern:     mustRegexPattern(t, `^(?P<c1>A).+\.jpg$`, []string{"c1"}),
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c1"}},
		},
		{
			ID: "test-conflict-b", Description: "快路径夹具：标题冲突 B", Source: RuleSourceManual, Enabled: true,
			Pattern:     mustRegexPattern(t, `^.+(?P<c2>B)\.jpg$`, []string{"c2"}),
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c2"}},
		},
		{
			ID: "test-dir-clue-tag", Description: "快路径夹具：线索层目录=标签", Source: RuleSourceManual, Enabled: true,
			Pattern:     dirPattern,
			Extractions: []ExtractionAction{{Dimension: DimTag, From: FromAncestorDir}},
		},
	}
	rules = append(rules, extraArchives...)
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: rules}); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	recognitionSampleTree(t, root)
	writeTestFile(t, root+"/对齐/月华01.png", "x")
	writeTestFile(t, root+"/对齐/月华02.png", "x")
	writeTestFile(t, root+"/对齐/月华03.png", "x")
	sess, err := m.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	sess.Assignments = map[string]SessionAssignment{"冲突/AB.jpg": {Author: "手工作者"}}
	if err := m.saveSession(sess); err != nil {
		t.Fatalf("注入会话赋值失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮全量识别失败: %v", err)
	}
	return m, root
}

// mustDraftEntryJSON 草稿 JSON → 规则条目（保存同款构造管线，夹具档案追加用）。
func mustDraftEntryJSON(t *testing.T, ruleJSON string) RuleEntry {
	t.Helper()
	entry, err := previewDraftEntry([]byte(ruleJSON))
	if err != nil {
		t.Fatalf("构造规则条目失败: %v", err)
	}
	return *entry
}

// previewResponseOf 快路径开关就地切换执行一次弹窗预览（探针增量随返回）：
// fn 内先按当前开关态求值，再强制整库全量路径求值。
func previewResponseOf(t *testing.T, m *SessionManager, root string, draft []byte) (probeDelta int64, fast, forcedFull *rulesPreviewResult) {
	t.Helper()
	before := previewFastPathTaken.Load()
	resp, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("命中预览失败: %v", err)
	}
	probeDelta, fast = previewFastPathTaken.Load()-before, resp

	restore := previewFastPathDisabled
	previewFastPathDisabled = true
	defer func() { previewFastPathDisabled = restore }()
	full, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("整库全量路径预览失败: %v", err)
	}
	forcedFull = full
	return
}

// previewConclusionsOf 快路径开关就地切换取预览求值结论（先按当前开关态、再
// 强制整库全量路径），返回两路结论。
func previewConclusionsOf(t *testing.T, m *SessionManager, root string, draft []byte) (fast, forcedFull []FileConclusion) {
	t.Helper()
	fastConclusions, _, err := m.previewRuleConclusions(root, draft)
	if err != nil {
		t.Fatalf("预览求值失败: %v", err)
	}
	restore := previewFastPathDisabled
	previewFastPathDisabled = true
	defer func() { previewFastPathDisabled = restore }()
	fullConclusions, _, err := m.previewRuleConclusions(root, draft)
	if err != nil {
		t.Fatalf("整库全量路径预览求值失败: %v", err)
	}
	return fastConclusions, fullConclusions
}

// assertPreviewResponsesByteEqual 逐字节全等锚：两路应答 JSON 序列化字节相等
//（方案口径「逐字节」——json.Marshal 后整体比对，含样例序与缺省键形态）。
func assertPreviewResponsesByteEqual(t *testing.T, fast, full *rulesPreviewResult) {
	t.Helper()
	fastJSON, fullJSON := mustMarshal(t, fast), mustMarshal(t, full)
	if fastJSON != fullJSON {
		t.Fatalf("快路径与整库全量应答应逐字节全等:\n快路径: %s\n全量:   %s", fastJSON, fullJSON)
	}
}

// TestPreviewRuleFastPathByteEquality 等值锚·形态一（新增草稿：非组合、未被
// 引用、非定群）：快路径 vs 整库全量，同语料逐字节全等。语料覆盖定群规则群
// 成员/结构群成员/同层冲突待裁决/赋值覆盖文件/无特征文件；草稿三型分别命中
// 括号文件（作者并集+标题同值）、目录子树（标签并集）、全库通配。
func TestPreviewRuleFastPathByteEquality(t *testing.T) {
	m, root := newPreviewFastFixture(t)

	drafts := []struct {
		name  string
		json  string
		wantH int
	}{
		{"捕获提取草稿", `{"id":"fast-cap","description":"快路径草稿：捕获+常量","source":"manual","enabled":true,` +
			`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"【{任意}】{标题}"}}}],` +
			`"extractions":[{"dimension":"author","from":"constant","constant":"草稿作者"},` +
			`{"dimension":"title","from":"capture","captureName":"title","row":1}],"synthesis":{}}`, 1},
		{"目录常量草稿", `{"id":"fast-dir","description":"快路径草稿：目录常量标签","source":"manual","enabled":true,` +
			`"conditions":[{"condition":{"target":"dir","format":{"mode":"contains","word":"线"}}}],` +
			`"extractions":[{"dimension":"tag","from":"constant","constant":"草稿标签"}],"synthesis":{}}`, 1},
		{"全库通配草稿", `{"id":"fast-any","description":"快路径草稿：通配全库","source":"manual","enabled":true,` +
			`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{任意}"}}}],` +
			`"extractions":[{"dimension":"collection","from":"constant","constant":"草稿集"}],"synthesis":{}}`, -1},
	}
	for _, d := range drafts {
		probeDelta, fastResp, fullResp := previewResponseOf(t, m, root, []byte(d.json))
		if probeDelta != 1 {
			t.Fatalf("%s：新增草稿应走快路径（探针增量 %d，期望 1）", d.name, probeDelta)
		}
		assertPreviewResponsesByteEqual(t, fastResp, fullResp)
		if d.wantH >= 0 && fastResp.HitCount != d.wantH {
			t.Fatalf("%s：命中数 = %d，期望 %d", d.name, fastResp.HitCount, d.wantH)
		}
		fastConclusions, fullConclusions := previewConclusionsOf(t, m, root, []byte(d.json))
		if !reflect.DeepEqual(fastConclusions, fullConclusions) {
			t.Fatalf("%s：快路径与整库全量结论应逐字段等值", d.name)
		}
	}

	// 语料代表性守卫：结论面覆盖定群规则群成员（GroupRuleID 非空）、结构群
	// 成员（Group 非空且 GroupRuleID 空）、赋值覆盖文件（终选含赋值作者）。
	fastConclusions, _ := previewConclusionsOf(t, m, root, []byte(drafts[2].json))
	hasRuleGroup, hasStructuralGroup, hasAssignment := false, false, false
	for i := range fastConclusions {
		c := &fastConclusions[i]
		if c.GroupRuleID != "" {
			hasRuleGroup = true
		} else if c.Group != nil {
			hasStructuralGroup = true
		}
		if c.RelPath == "冲突/AB.jpg" {
			for _, decl := range c.Final {
				if decl.Source == SourceAssignment && decl.Dimension == DimAuthor && decl.Value == "手工作者" {
					hasAssignment = true
				}
			}
		}
	}
	if !hasRuleGroup || !hasStructuralGroup || !hasAssignment {
		t.Fatalf("等值锚语料应覆盖定群规则群/结构群/赋值覆盖三形态: 规则群=%v 结构群=%v 赋值=%v",
			hasRuleGroup, hasStructuralGroup, hasAssignment)
	}
}

// TestPreviewRuleFastPathFallbackBins 等值锚·形态二/三/四/五：替换条目、组合
// 草稿、被引用（替换与悬空引用新增）、定群草稿——全部显式回退整库全量路径
//（探针增量 0），应答与强制全量路径等值。
func TestPreviewRuleFastPathFallbackBins(t *testing.T) {
	// 形态二：替换条目（草稿 ID 与档案条目同 ID，原位替换）
	m, root := newPreviewFastFixture(t)
	probeDelta, fastResp, fullResp := previewResponseOf(t, m, root, []byte(
		`{"id":"test-bracket-author-title","description":"替换草稿：同 ID 原位替换","source":"manual","enabled":true,`+
			`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"【{作者}】{标题}"}}}],`+
			`"extractions":[{"dimension":"author","from":"capture","captureName":"author","row":1}],"synthesis":{}}`))
	if probeDelta != 0 {
		t.Fatalf("形态二（替换条目）应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 形态三：组合草稿（refs 非空——门段成员先行求值，耦合面整体让给回退）
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, []byte(
		`{"id":"fast-combo-draft","description":"组合草稿：引用冲突 A","source":"manual","enabled":true,`+
			`"refs":["test-conflict-a"],"conditions":[],"extractions":[],"synthesis":{}}`))
	if probeDelta != 0 {
		t.Fatalf("形态三（组合草稿）应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 形态五：定群草稿（全 seq 条件行且零提取——定群改变结构群对齐参与面）
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, []byte(
		`{"id":"fast-grouping-draft","description":"定群草稿：序列成群","source":"manual","enabled":true,`+
			`"conditions":[{"condition":{"target":"seq","format":{"mode":"template","tpl":"{键}_{序号}"}}}],`+
			`"extractions":[],"synthesis":{}}`))
	if probeDelta != 0 {
		t.Fatalf("形态五（定群草稿）应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 形态四甲：被引用替换——草稿 ID 被现行组合规则的 refs 引用
	mReferenced, rootReferenced := newPreviewFastFixture(t, mustDraftEntryJSON(t,
		`{"id":"fast-arch-combo","description":"档案组合规则：引用冲突 A","source":"manual","enabled":true,`+
			`"refs":["test-conflict-a"],"conditions":[],"extractions":[],"synthesis":{}}`))
	probeDelta, fastResp, fullResp = previewResponseOf(t, mReferenced, rootReferenced, []byte(
		`{"id":"test-conflict-a","description":"被引用替换草稿","source":"manual","enabled":true,`+
			`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{任意}"}}}],`+
			`"extractions":[{"dimension":"title","from":"constant","constant":"替换标题"}],"synthesis":{}}`))
	if probeDelta != 0 {
		t.Fatalf("形态四（被引用替换）应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 形态四乙：悬空引用的存量组合规则在档案载入时整条弃置（rule_profile.go
	// 第二遍悬空校验——预览求值面=载入档案），引用面按载入档案即完备：新增
	// 草稿即便恰好取悬空 ID，载入档案内也无引用者，快路径合法且等值。
	mDangle, rootDangle := newPreviewFastFixture(t, mustDraftEntryJSON(t,
		`{"id":"fast-arch-dangle","description":"档案组合规则：悬空引用","source":"manual","enabled":true,`+
			`"refs":["ghost-draft-id"],"conditions":[],"extractions":[],"synthesis":{}}`))
	probeDelta, fastResp, fullResp = previewResponseOf(t, mDangle, rootDangle, []byte(
		`{"id":"ghost-draft-id","description":"悬空引用目标草稿","source":"manual","enabled":true,`+
			`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{任意}"}}}],`+
			`"extractions":[{"dimension":"title","from":"constant","constant":"悬空标题"}],"synthesis":{}}`))
	if probeDelta != 1 {
		t.Fatalf("悬空引用组合载入即弃，引用面=载入档案，新增草稿应走快路径（探针增量 %d，期望 1）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)
}

// TestPreviewRuleFastPathGuardFallbacks 快路径等值护栏与基座守卫的回退分箱：
// 档案条件动作/贪婪识别动作/库内小标/谓词模式（基座产物不可按当前面重放的
// 四型）、基座缺席（逐出）、档案指纹失配（识别后档案被改写）——全部回退整库
// 全量且应答等值。
func TestPreviewRuleFastPathGuardFallbacks(t *testing.T) {
	draft := []byte(`{"id":"fast-guard-draft","description":"护栏回退草稿","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{任意}"}}}],` +
		`"extractions":[{"dimension":"title","from":"constant","constant":"护栏标题"}],"synthesis":{}}`)

	// 护栏①：档案规则带生效条件（When）动作——两遍执行产物与草稿首遍产物
	// 在全量声明序列内交错不同序，合成对序敏感不可重放
	m, root := newPreviewFastFixture(t, RuleEntry{
		ID: "arch-when", Description: "护栏夹具：条件动作", Source: RuleSourceManual, Enabled: true,
		Pattern: mustRegexPattern(t, `^AB.+\.jpg$`, nil),
		Extractions: []ExtractionAction{{
			Dimension: DimTitle, From: FromConstant, Constant: "条件标题",
			When: &CondExpr{Kind: ExprPredicate, Source: SourceFilename, Dimension: DimTitle},
		}},
	})
	probeDelta, fastResp, fullResp := previewResponseOf(t, m, root, draft)
	if probeDelta != 0 {
		t.Fatalf("档案条件动作应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 护栏②：档案规则带贪婪识别（scan）动作——第三遍产物同上不可重放
	m, root = newPreviewFastFixture(t, RuleEntry{
		ID: "arch-scan", Description: "护栏夹具：贪婪识别动作", Source: RuleSourceManual, Enabled: true,
		Pattern:     mustRegexPattern(t, `^库内作者名\.jpg$`, nil),
		Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromScan, ScanScope: "path"}},
	})
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, draft)
	if probeDelta != 0 {
		t.Fatalf("档案贪婪识别动作应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 护栏③：档案规则带库内名单小标（library 值校验门）——基座按识别轮名单
	// 铸造，名单换代后基座命中面与全量求值可能偏离
	badgeRows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{标签}画"},
		Badges: []CondxBadge{{Placeholder: "标签", Kind: CondxBadgeLibrary, Dim: "tag"}},
	}}}
	badgePattern, err := CompileRuleConditions(badgeRows)
	if err != nil {
		t.Fatalf("构造库内小标规则失败: %v", err)
	}
	m, root = newPreviewFastFixture(t, RuleEntry{
		ID: "arch-badge", Description: "护栏夹具：库内小标", Source: RuleSourceManual, Enabled: true,
		Conditions: badgeRows, Pattern: badgePattern,
		Extractions: []ExtractionAction{{Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1}},
	})
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, draft)
	if probeDelta != 0 {
		t.Fatalf("档案库内小标应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 护栏④：档案谓词模式（PredicatePattern）——直接消费库内谓词索引
	m, root = newPreviewFastFixture(t, RuleEntry{
		ID: "arch-predicate", Description: "护栏夹具：谓词模式", Source: RuleSourceManual, Enabled: true,
		Pattern:     &PredicatePattern{Kind: PredicateAuthor},
		Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromPredicate}},
	})
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, draft)
	if probeDelta != 0 {
		t.Fatalf("档案谓词模式应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 护栏⑤：档案非定群规则含序列行（带提取动作故非定群形态）——求值写共享
	// 上下文群组字段，全量路径中草稿居序尾看到被改写后的群组上下文，基座段
	// 只存提取段前初值，群组操作数/群组前缀提取类草稿将偏离
	m, root = newPreviewFastFixture(t, mustDraftEntryJSON(t,
		`{"id":"arch-seq-extract","description":"护栏夹具：序列行+提取（非定群）","source":"manual","enabled":true,`+
			`"conditions":[{"condition":{"target":"seq","format":{"mode":"template","tpl":"{键}_{序号}"}}}],`+
			`"extractions":[{"dimension":"collection","from":"capture","captureName":"key","row":1}],"synthesis":{}}`))
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, []byte(
		`{"id":"fast-groupprefix-draft","description":"护栏草稿：群组前缀提取","source":"manual","enabled":true,`+
			`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{任意}"}}}],`+
			`"extractions":[{"dimension":"collection","from":"groupPrefix"}],"synthesis":{}}`))
	if probeDelta != 0 {
		t.Fatalf("档案非定群序列行规则应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 基座缺席（缓存逐出后重装填）：守卫指针等值失配 → 回退，应答与强制全量等值
	m, root = newPreviewFastFixture(t)
	m.evictSessionCache(normalizeLibraryRoot(root))
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, draft)
	if probeDelta != 0 {
		t.Fatalf("基座缺席应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)

	// 档案指纹失配（识别完成后档案被保存改写、未再识别）：基座按旧档案铸造 → 回退
	m, root = newPreviewFastFixture(t)
	profile, errs := LoadProfile(m.store)
	if len(errs) > 0 || profile == nil {
		t.Fatalf("载入规则档案失败: %v", errs)
	}
	profile.Rules = append(profile.Rules, mustDraftEntryJSON(t,
		`{"id":"late-archive-rule","description":"识别后改写档案","source":"manual","enabled":true,`+
			`"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"never_{任意}"}}}],`+
			`"extractions":[],"synthesis":{}}`))
	if err := SaveProfile(m.store, profile); err != nil {
		t.Fatalf("改写规则档案失败: %v", err)
	}
	probeDelta, fastResp, fullResp = previewResponseOf(t, m, root, draft)
	if probeDelta != 0 {
		t.Fatalf("档案指纹失配应回退整库全量（探针增量 %d，期望 0）", probeDelta)
	}
	assertPreviewResponsesByteEqual(t, fastResp, fullResp)
}

// TestPreviewRuleDirLevelAnchor 命中预览通道（声明5 第 6 路，AE 阶段5 核验）：
// 预览求值走与保存后重识别同一合并求值入口，故带层级锚的草稿其命中数/样例即
// 「锚定层形态命中的文件」——第 1 级 [路灯后辈] 子树两文件计入、样例「在
// 目录/」为锚定层下实际位置；更深层同名形态目录（Art/[路灯后辈]）不命中。
// 组合规则直译句（前端）只按成员目标类别成段、不含层级断言，锚定成员的呈现
// 由成员自身摘要的层级小字承载（ruleFormModel.ts:577/616），本通道无需改动。
func TestPreviewRuleDirLevelAnchor(t *testing.T) {
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	m := NewSessionManagerWithStore(ctx, nil, store)
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{}}); err != nil {
		t.Fatalf("写入空规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	writeTestFile(t, root+"/[路灯后辈]/a.jpg", "a")
	writeTestFile(t, root+"/[路灯后辈]/子/b.jpg", "b")
	writeTestFile(t, root+"/Art/[路灯后辈]/c.jpg", "c")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	draft := []byte(`{"id":"preview-level","description":"预览草稿：第 1 级括号目录→标签","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"[{标签}]"},` +
		`"attach":[{"kind":"dirLevel","minCount":1}]}}],` +
		`"extractions":[{"dimension":"tag","from":"capture","captureName":"tag","row":1}],"synthesis":{}}`)
	preview, err := m.PreviewRule(root, draft)
	if err != nil {
		t.Fatalf("命中预览失败: %v", err)
	}
	if preview.HitTotal != 3 {
		t.Fatalf("求值面总数 = %d，期望 3", preview.HitTotal)
	}
	if preview.HitCount != 2 {
		t.Fatalf("命中数 = %d，期望 2（仅第 1 级括号目录子树）", preview.HitCount)
	}
	if len(preview.Samples) != 2 {
		t.Fatalf("样例 = %d 条，期望 2（两命中分属不同目录）: %+v", len(preview.Samples), preview.Samples)
	}
	wantDirs := map[string]bool{"[路灯后辈]": true, "[路灯后辈]/子": true}
	for _, s := range preview.Samples {
		if !wantDirs[s.DirRelKey] || s.Result != "标签=路灯后辈" {
			t.Fatalf("样例不符（锚定层下实际位置/内名声明）: %+v", s)
		}
	}

	// 预览即保存后：入档并全量重识别，命中集与预览一致（a、b 命中，c 不命中）。
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
			if id == "preview-level" {
				afterHits[sess.Snapshot.Files[i].RelPath] = true
			}
		}
	}
	want := map[string]bool{"[路灯后辈]/a.jpg": true, "[路灯后辈]/子/b.jpg": true}
	if !relPathSetsEqual(afterHits, want) {
		t.Fatalf("保存后重识别命中集不符: %v（期望 %v）", afterHits, want)
	}
	if !relPathSetsEqual(previewHits, afterHits) {
		t.Fatalf("预览命中集与保存后重识别不一致: 预览=%v 重识别=%v", previewHits, afterHits)
	}
}
