// 资源选择与任务创建（阶段4）的单元测试：作品原子选择域推导与可选性校验、
// 选择集持久化（未识别/待裁决不可选）、工作台发起创建的过滤产出（批次应答组：
// 按站点分组的单父多子行、一次创建=一个父任务；任务级赋值快照固化、零弹问）、
// 任务名裁决与默认名兜底、创建编排动作（磁盘对账阻断、CreateTask 一次调用、
// 选择集清理、通知文案）与执行期重算的同源断言（干跑/创建/重算三点注入同一谓词
// 索引与合并实现——风险12/13 闭环）。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// createTaskCalls CreateTask 桩调用计数。
func (c *workbenchTestCtx) createTaskCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.createTaskCount
}

// createTaskURL CreateTask 桩末次调用 URL。
func (c *workbenchTestCtx) createTaskURL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.createTaskLastURL
}

// publishedCount 前端事件发布计数（识别期亦发事件，零弹问断言取创建前后差量）。
func (c *workbenchTestCtx) publishedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.published)
}

// —— 选择测试档案与样例树 ——

// selectionTestRules 构造选择测试的档案规则：兄弟群组定群（前缀任意非空）、
// 群组成员集名=公共前缀（FromGroupPrefix——非群组成员来源缺失不产出）、谓词
// 作者规则（吃库内索引）、双规则同层冲突（待裁决形态）。
func selectionTestRules(t *testing.T) []RuleEntry {
	t.Helper()
	sibling, err := NewSiblingPattern(`^(.+)_`, `(\d+)`, 2, OutcomeCollection)
	if err != nil {
		t.Fatalf("构造兄弟群组规则失败: %v", err)
	}
	conflictA := mustRegexPattern(t, `^(?P<c1>A).+\.jpg$`, []string{"c1"})
	conflictB := mustRegexPattern(t, `^.+(?P<c2>B)\.jpg$`, []string{"c2"})
	return []RuleEntry{
		{
			ID: "sel-sibling-set", Description: "测试：下划线前缀+序号成群", Source: RuleSourceManual, Enabled: true,
			Pattern: sibling,
		},
		{
			ID: "sel-set-collection", Description: "测试：群组成员集名=公共前缀", Source: RuleSourceManual, Enabled: true,
			Pattern:     mustRegexPattern(t, `^.+$`, nil),
			Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromGroupPrefix}},
		},
		{
			ID: "test-conflict-a", Description: "测试：标题冲突 A", Source: RuleSourceManual, Enabled: true,
			Pattern:     conflictA,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c1"}},
		},
		{
			ID: "test-conflict-b", Description: "测试：标题冲突 B", Source: RuleSourceManual, Enabled: true,
			Pattern:     conflictB,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c2"}},
		},
		{
			ID: "test-predicate-author", Description: "测试：文件名命中库内作者", Source: RuleSourceManual, Enabled: true,
			Pattern:     &PredicatePattern{Kind: PredicateAuthor},
			Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromPredicate}},
		},
	}
}

// newSelectionFixture 构造选择域测试环境：内存存储 + 桩上下文（库内索引含
// 「库内作者名」）+ 选择测试档案 + 指定库根会话（赋值在建档后、识别前注入，
// 修订号与快照一致）。返回的 ctx 预置 CreateTask 桩（成功、AddedQuantity=3）。
func newSelectionFixture(t *testing.T, root string, assigns map[string]SessionAssignment) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store, createTaskStub: &sdkdto.CreateTaskResult{Succeed: true, AddedQuantity: 3}}
	ctx.library = libraryFixture{localAuthors: []string{"库内作者名"}}
	m := NewSessionManagerWithStore(ctx, nil, store)

	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: selectionTestRules(t)}
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if len(assigns) > 0 {
		sess, err := m.loadSession(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		sess.Assignments = assigns
		if err := m.saveSession(sess); err != nil {
			t.Fatalf("注入会话赋值失败: %v", err)
		}
	}
	return m, ctx
}

// selectionSampleTree 构造选择测试样例树：
//
//	<root>/已识别/【作者A】标题1.jpg    内置规则命中 → identified（d:已识别 单轨作品）
//	<root>/多轨/【作者B】标题2.jpg      内置规则命中 → identified（d:多轨 双轨作品）
//	<root>/多轨/【作者C】标题3.jpg      内置规则命中 → identified（d:多轨 双轨作品）
//	<root>/未识别/随便.txt              plain（d:未识别 不可选）
//	<root>/序列/前缀名_001..003.jpg     兄弟群组成群+集名声明 → identified（集成员作品×3）
//	<root>/序列/落单_009.jpg            序号形态组大小 1 → plain（d:序列 不可选）
//	<root>/冲突/AB.jpg                  双规则冲突 → 待裁决（d:冲突 不可选）
//	<root>/库内作者名.jpg               谓词规则命中（库内索引）→ identified（d:"" 单轨作品）
func selectionSampleTree(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, root+"/已识别/【作者A】标题1.jpg", "a")
	writeTestFile(t, root+"/多轨/【作者B】标题2.jpg", "b")
	writeTestFile(t, root+"/多轨/【作者C】标题3.jpg", "c")
	writeTestFile(t, root+"/未识别/随便.txt", "d")
	writeTestFile(t, root+"/序列/前缀名_001.jpg", "e")
	writeTestFile(t, root+"/序列/前缀名_002.jpg", "f")
	writeTestFile(t, root+"/序列/前缀名_003.jpg", "g")
	writeTestFile(t, root+"/序列/落单_009.jpg", "h")
	writeTestFile(t, root+"/冲突/AB.jpg", "i")
	writeTestFile(t, root+"/库内作者名.jpg", "j")
}

// recognizedSelectionFixture 样例树 + 全量识别完成的选择测试环境。
func recognizedSelectionFixture(t *testing.T, assigns map[string]SessionAssignment) (string, *SessionManager, *workbenchTestCtx) {
	t.Helper()
	root := t.TempDir()
	m, ctx := newSelectionFixture(t, root, assigns)
	selectionSampleTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return root, m, ctx
}

// —— 退出标准 2：作品原子结构 —— //

// TestDeriveWorkDomainWorkAtomicity 选择域推导：集成员各自单文件作品（集=序列群，
// 成员按集内序）、目录落单文件合为一件多轨作品（轨道集=该目录全部落单文件，
// 整体持有——作品键只指向作品，无文件级键形态）；可选性=全部文件已识别且无
// 待裁决（未识别/待裁决作品整体不可选——结构上表达不了半作品）。
func TestDeriveWorkDomainWorkAtomicity(t *testing.T) {
	root, m, _ := recognizedSelectionFixture(t, nil)
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	domain := DeriveWorkDomain(sess.Snapshot, ignoredSetOf(sess))

	byID := make(map[string]*SelectableWork)
	for _, work := range domain.Works {
		byID[work.WorkID] = work
	}
	// 目录聚合作品：多轨目录两文件为一件作品的完整轨道集
	multi := byID["d:多轨"]
	if multi == nil {
		t.Fatalf("缺少 d:多轨 目录聚合作品（works: %+v）", domain.Works)
	}
	if multi.Kind != WorkKindDirTracks || len(multi.Files) != 2 || multi.SetKey != "" {
		t.Fatalf("d:多轨 应为双轨原子目录聚合作品，实际 %+v", multi)
	}
	if !multi.Selectable || multi.State != workStateIdentified {
		t.Fatalf("全轨已识别的目录聚合作品应可选，实际 selectable=%v state=%s", multi.Selectable, multi.State)
	}

	// 集成员：三名成员各自单文件作品、同属一集
	var set *WorkSetView
	for _, s := range domain.Sets {
		if s.DirRelKey == "序列" && s.Key == "前缀名" {
			set = s
		}
	}
	if set == nil || len(set.Members) != 3 {
		t.Fatalf("应有一集（序列/前缀名，3 成员），实际 sets: %+v", domain.Sets)
	}
	for i, memberID := range set.Members {
		member := byID[memberID]
		if member == nil || member.Kind != WorkKindSetMember || len(member.Files) != 1 {
			t.Fatalf("集成员 %s 应为单文件作品，实际 %+v", memberID, member)
		}
		if !member.Selectable {
			t.Fatalf("集成员 %s 应可选（集名声明已识别）", memberID)
		}
		want := "f:序列/前缀名_00" + string(rune('0'+i+1)) + ".jpg"
		if member.WorkID != want {
			t.Fatalf("集内序 %d 成员应为 %s，实际 %s", i+1, want, member.WorkID)
		}
	}

	// 不可选作品：未识别（含落单 plain）、待裁决；根目录聚合作品（库内作者名.jpg
	// 经谓词识别）可选
	for id, wantSelectable := range map[string]bool{
		"d:未识别": false,
		"d:序列":  false, // 落单_009 为 plain → 整件不可选
		"d:冲突":  false, // 待裁决 → 整件不可选
		"d:":    true,  // 库内作者名.jpg 谓词识别 → 可选
		"d:已识别": true,
	} {
		work := byID[id]
		if work == nil {
			t.Fatalf("缺少作品 %s（works: %+v）", id, domain.Works)
		}
		if work.Selectable != wantSelectable {
			t.Fatalf("作品 %s 可选性 = %v，期望 %v（state=%s）", id, work.Selectable, wantSelectable, work.State)
		}
	}
	// 待裁决呈现态
	if byID["d:冲突"].State != workStatePending {
		t.Fatalf("d:冲突 呈现态应为 pending，实际 %s", byID["d:冲突"].State)
	}
}

// —— 退出标准 9：未识别项不可选 —— //

// TestSetSelectionValidatesSelectability 选择集写入校验：未知作品键（含裸相对
// 路径——文件级键形态结构性不存在）与不可选作品（未识别/待裁决）整体拒绝，
// 不部分落库；合法键去重按稳定序落库并随决策态投影回读。
func TestSetSelectionValidatesSelectability(t *testing.T) {
	root, m, _ := recognizedSelectionFixture(t, nil)

	// 裸相对路径不是作品键：文件级半选不可寻址
	if _, err := m.SetSelection(root, []string{"多轨/【作者B】标题2.jpg"}); err == nil {
		t.Fatalf("裸相对路径不应是合法作品键（文件级半选结构性不可表达）")
	}
	if _, err := m.SetSelection(root, []string{"d:不存在"}); err == nil {
		t.Fatalf("未知作品键应拒绝")
	}
	// 未识别/待裁决作品不可选
	if _, err := m.SetSelection(root, []string{"d:未识别"}); err == nil {
		t.Fatalf("未识别作品不可选")
	}
	if _, err := m.SetSelection(root, []string{"d:序列"}); err == nil {
		t.Fatalf("含未识别文件的目录聚合作品不可选（半作品被结构性防止）")
	}
	if _, err := m.SetSelection(root, []string{"d:冲突"}); err == nil {
		t.Fatalf("待裁决作品不可选（未定论）")
	}
	// 拒绝后零残留
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if len(sess.Selection) != 0 {
		t.Fatalf("被拒绝的写入不应部分落库，实际 %v", sess.Selection)
	}

	// 合法写入：去重 + 稳定序
	sess, err = m.SetSelection(root, []string{"d:多轨", "d:多轨", "d:已识别", "d:"})
	if err != nil {
		t.Fatalf("合法选择集写入失败: %v", err)
	}
	if strings.Join(sess.Selection, "|") != "d:|d:多轨|d:已识别" {
		t.Fatalf("选择集应去重按稳定序落库，实际 %v", sess.Selection)
	}
}

// —— 退出标准 1/6：会话模式只产出所选作品（集父行按所选成员生成）+ 零弹问 —— //

// collectWorkbenchCreate 经发起标记驱动 Create 的过滤产出分支并收集全部应答；
// taskName 模拟编排动作暂存的创建任务名原文（空串=默认名兜底路径——与真实
// 编排 ApplyCreateTasks 的暂存行为同构）。
func collectWorkbenchCreate(t *testing.T, m *SessionManager, ctx *workbenchTestCtx, root, taskName string) []*sdkdto.TaskCreateResponse {
	t.Helper()
	h := &LocalImportWorkFetcher{sessions: m, ctx: ctx}
	m.stageCreateTaskName(root, taskName)
	if err := m.BeginWorkbenchCreate(root); err != nil {
		t.Fatalf("置位发起标记失败: %v", err)
	}
	result, err := h.Create("local://" + root)
	if err != nil {
		t.Fatalf("工作台发起创建不应报错: %v", err)
	}
	if !result.IsStream() {
		t.Fatalf("过滤产出应为流式结果")
	}
	var out []*sdkdto.TaskCreateResponse
	for resp := range result.Stream() {
		out = append(out, resp)
	}
	return out
}

// responseChildNames 应答的子任务文件名集合（批次父行子行形态的断言载体）。
func responseChildNames(resp *sdkdto.TaskCreateResponse) []string {
	names := make([]string, 0, len(resp.Children))
	for _, child := range resp.Children {
		names = append(names, child.TaskName)
	}
	return names
}

// TestCreateWorkbenchFilteredEmitsSelectedWorksOnly 会话模式 Create 过滤产出：
// 选择=「d:多轨 双轨作品」+「集内 2/3 成员显式拆选」→ 只产出所选作品：纯 local
// 批次=单应答单父（批次应答），子行=全部所选文件（确定序：目录序内先目录
// 聚合作品后集成员）；未选作品（d:已识别、第三名集成员）零任务，未识别/待裁决
// 作品即使混入选择集也非可选（写入即拒）。任务名走默认名兜底（未暂存用户名）。
// 全程零前端事件（零 ClassifyDir 弹问——目录含义取快照回放，结构性不触问答）。
// PluginData 携带内容哈希与赋值快照；父行为纯容器（DirRelPath 空、无含义链）。
func TestCreateWorkbenchFilteredEmitsSelectedWorksOnly(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, map[string]SessionAssignment{
		"多轨/【作者C】标题3.jpg": {Title: "手工标题", UpdatedAtMs: 1},
	})
	if _, err := m.SetSelection(root, []string{"d:多轨", "f:序列/前缀名_001.jpg", "f:序列/前缀名_002.jpg"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}

	before := ctx.publishedCount()
	resps := collectWorkbenchCreate(t, m, ctx, root, "")
	if len(resps) != 1 {
		t.Fatalf("纯 local 批次应为单应答单父（批次应答），实得 %d 个: %+v", len(resps), resps)
	}
	batch := resps[0]

	// 批次父行：默认名兜底（本地导入 · 库根名 · N 件作品，N=选择集作品数〔3 作品
	// 4 文件〕）、local-batch- 父身份、创建域站点键、纯容器 PluginData（DirRelPath
	// 空、无含义链）
	if want := fmt.Sprintf("本地导入 · %s · 3 件作品", filepath.Base(root)); batch.TaskName != want {
		t.Fatalf("批次父行任务名应为默认名 %q，实际 %q", want, batch.TaskName)
	}
	if !strings.HasPrefix(batch.PluginTaskId, "local-batch-") || batch.SiteKey != identity.Local.Key {
		t.Fatalf("批次父行身份应为 local-batch-<毫秒> 且创建域站点键，实际 %q/%q", batch.PluginTaskId, batch.SiteKey)
	}
	var dp DirPluginData
	if err := json.Unmarshal([]byte(batch.PluginData), &dp); err != nil {
		t.Fatalf("解析父行 PluginData 失败: %v", err)
	}
	if dp.DirRelPath != "" || dp.Metadata != nil {
		t.Fatalf("批次父行 PluginData 应为纯容器（DirRelPath 空、无含义链），实际 %+v", dp)
	}

	// 子行=全部所选文件（原子轨道集 + 集成员按域序=集内序，确定序保持）
	if want := "【作者B】标题2.jpg,【作者C】标题3.jpg,前缀名_001.jpg,前缀名_002.jpg"; strings.Join(responseChildNames(batch), ",") != want {
		t.Fatalf("批次子行应按确定序含全部所选文件，实际 %v", responseChildNames(batch))
	}
	// 赋值快照固化：标题3 轨携带手工标题、标题2 轨无快照
	for _, child := range batch.Children {
		var fp FilePluginData
		if err := json.Unmarshal([]byte(child.PluginData), &fp); err != nil {
			t.Fatalf("解析子任务 PluginData 失败: %v", err)
		}
		if fp.Hash == "" || fp.RelPath == "" {
			t.Fatalf("子任务 PluginData 应携带内容哈希与相对路径，实际 %+v", fp)
		}
		if child.TaskName == "【作者C】标题3.jpg" {
			if fp.Assignment == nil || fp.Assignment.Title != "手工标题" {
				t.Fatalf("已赋值轨应固化任务级赋值快照，实际 %+v", fp.Assignment)
			}
		} else if fp.Assignment != nil {
			t.Fatalf("未赋值轨不应携带赋值快照，实际 %+v", fp.Assignment)
		}
	}

	// 未选项零任务：d:已识别 与 第三名集成员不在任何子行中
	for _, name := range responseChildNames(batch) {
		if name == "【作者A】标题1.jpg" || name == "前缀名_003.jpg" {
			t.Fatalf("未选作品出现在产出中: %s", name)
		}
	}

	// 零弹问（退出标准 6）：创建全程零新增前端事件——ClassifyDir 的问答与入口
	// 确认弹窗均不触发（过滤产出分支对分类器结构性零引用，目录含义取快照回放）
	if after := ctx.publishedCount(); after != before {
		t.Fatalf("会话模式创建应零前端事件（零 ClassifyDir 弹问），实得创建期新增 %d 条", after-before)
	}
}

// TestCreateWorkbenchFilteredLoneSetMemberSingleChild 集内仅选一名成员：批次应答
// 形态下为单父行+单子行（单文件叶任务不折叠——一次创建恒为一个父任务），
// 子行身份=内容哈希（local 回退）。
func TestCreateWorkbenchFilteredLoneSetMemberSingleChild(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, nil)
	if _, err := m.SetSelection(root, []string{"f:序列/前缀名_003.jpg"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	resps := collectWorkbenchCreate(t, m, ctx, root, "单件批次")
	if len(resps) != 1 {
		t.Fatalf("单名集成员应产出一个批次应答（单父行），实得 %+v", resps)
	}
	resp := resps[0]
	if resp.TaskName != "单件批次" || len(resp.Children) != 1 {
		t.Fatalf("批次父行应为用户名+单子行（不折叠为叶任务），实际 %+v", resp)
	}
	child := resp.Children[0]
	if child.TaskName != "前缀名_003.jpg" || child.SiteWorkId == "" {
		t.Fatalf("子行应为所选文件（哈希身份），实际 %+v", child)
	}
}

// —— 批次应答：纯 local 与混归属单父（emitSelectedWorks 直驱）—— //

// emitFixture 构造直驱 emitSelectedWorks 的最小环境：临时库根 + 指定相对路径的
// 文件集 + 空快照会话（快照无目录记录 → 含义链为空，归属裁决只走文件名形态）。
func emitFixture(t *testing.T, relPaths []string) (string, *LocalImportWorkFetcher, *SessionManager, *WorkbenchSession) {
	t.Helper()
	root := t.TempDir()
	for _, rel := range relPaths {
		writeTestFile(t, root+"/"+rel, rel)
	}
	m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
	h := &LocalImportWorkFetcher{sessions: m}
	return root, h, m, &WorkbenchSession{Snapshot: &RecognitionSnapshot{}}
}

// collectEmitted 直驱 emitSelectedWorks 收集全部应答（同步调用，结束后收卷）。
func collectEmitted(t *testing.T, h *LocalImportWorkFetcher, root string, sess *WorkbenchSession, works []*SelectableWork, taskName string) []*sdkdto.TaskCreateResponse {
	t.Helper()
	ch := make(chan *sdkdto.TaskCreateResponse, 16)
	h.emitSelectedWorks(root, sess, works, taskName, ch)
	close(ch)
	var out []*sdkdto.TaskCreateResponse
	for resp := range ch {
		out = append(out, resp)
	}
	return out
}

// TestEmitSelectedWorksPureLocalSingleParent 纯 local 批次=单应答单父：一次创建
// 的全部所选文件为 local 回退子行（归属键留空=继承父应答站点键），子行数=文件
// 数、确定序保持（目录序内先目录聚合作品后集成员〔域序=集内序〕）、用户名直用
// （默认名两式的另一式由 TestCreateWorkbenchFilteredEmitsSelectedWorksOnly 的
// 空名直驱路径覆盖）。
func TestEmitSelectedWorksPureLocalSingleParent(t *testing.T) {
	root, h, m, sess := emitFixture(t, []string{
		"甲/【作者A】标题1.jpg",
		"甲/【作者A】标题2.jpg",
		"乙/集名_001.jpg",
		"乙/集名_002.jpg",
	})
	works := []*SelectableWork{
		{WorkID: "d:甲", Kind: WorkKindDirTracks, DirRelKey: "甲", Files: []string{"甲/【作者A】标题1.jpg", "甲/【作者A】标题2.jpg"}},
		{WorkID: "f:乙/集名_001.jpg", Kind: WorkKindSetMember, DirRelKey: "乙", Files: []string{"乙/集名_001.jpg"}, SetKey: workSetKey("乙", "集名")},
		{WorkID: "f:乙/集名_002.jpg", Kind: WorkKindSetMember, DirRelKey: "乙", Files: []string{"乙/集名_002.jpg"}, SetKey: workSetKey("乙", "集名")},
	}
	resps := collectEmitted(t, h, root, sess, works, "整批导入")
	if len(resps) != 1 || resps[0].TaskName != "整批导入" {
		t.Fatalf("纯 local 批次应为单应答（用户名直用），实得 %+v", resps)
	}
	resp := resps[0]
	if want := "【作者A】标题1.jpg,【作者A】标题2.jpg,集名_001.jpg,集名_002.jpg"; strings.Join(responseChildNames(resp), ",") != want {
		t.Fatalf("子行应按确定序含全部 4 文件，实际 %v", responseChildNames(resp))
	}
	if resp.SiteKey != identity.Local.Key || !strings.HasPrefix(resp.PluginTaskId, "local-batch-") {
		t.Fatalf("批次父行应为创建域站点键与 local-batch-<毫秒> 身份形态，实际 %q/%q", resp.SiteKey, resp.PluginTaskId)
	}
	for _, child := range resp.Children {
		if child.SiteKey != "" {
			t.Fatalf("local 回退子行归属键应留空（继承父应答站点键），实际 %q", child.SiteKey)
		}
	}
	if summary := m.takeCreateBatchSummary(root); summary.total != 4 || len(summary.nonLocalBySite) != 0 {
		t.Fatalf("纯 local 批次摘要应为 4 子行、无归属分布，实际 %+v", summary)
	}
}

// TestEmitSelectedWorksMixedAttributionSingleParent 混归属批次=单应答单父：pixiv
// 原图形态文件名与普通文件同批，全部子行进同一父应答（站点键=创建域
// identity.Local.Key）；子行归属键分布——真实域子行携归属站点键与页级作品 ID
// 身份、local 回退子行归属键留空（继承父应答站点键）与内容哈希身份；父行裸用
// 用户名、父身份无组间后缀；批次摘要回写（子行总数与归属分布——通知文案数据源）。
func TestEmitSelectedWorksMixedAttributionSingleParent(t *testing.T) {
	root, h, m, sess := emitFixture(t, []string{
		"混合/【作者A】标题1.jpg",
		"混合/11111111_p0.jpg",
	})
	works := []*SelectableWork{{
		WorkID: "d:混合", Kind: WorkKindDirTracks, DirRelKey: "混合",
		Files: []string{"混合/【作者A】标题1.jpg", "混合/11111111_p0.jpg"},
	}}
	resps := collectEmitted(t, h, root, sess, works, "混归属批次")
	if len(resps) != 1 {
		t.Fatalf("混归属批次应为单应答单父（全部子行进同一父），实得 %d: %+v", len(resps), resps)
	}
	resp := resps[0]
	if resp.SiteKey != identity.Local.Key || resp.TaskName != "混归属批次" {
		t.Fatalf("父行应为创建域站点键+裸用户名（无站点名后缀），实际 %q/%q", resp.TaskName, resp.SiteKey)
	}
	if !strings.HasPrefix(resp.PluginTaskId, "local-batch-") || strings.Contains(resp.PluginTaskId, "@") {
		t.Fatalf("父身份应为 local-batch-<毫秒>（无组间后缀），实际 %q", resp.PluginTaskId)
	}
	// 子行确定序：目录内文件原序（不按归属重排）
	if want := "【作者A】标题1.jpg,11111111_p0.jpg"; strings.Join(responseChildNames(resp), ",") != want {
		t.Fatalf("子行应按目录内原序排列，实际 %v", responseChildNames(resp))
	}
	localChild, realChild := resp.Children[0], resp.Children[1]
	if localChild.SiteKey != "" || localChild.SiteWorkId == "11111111_p0" {
		t.Fatalf("local 回退子行归属键应留空且身份=内容哈希，实际 %q/%q", localChild.SiteKey, localChild.SiteWorkId)
	}
	if realChild.SiteKey != identity.Pixiv.Key || realChild.SiteWorkId != "11111111_p0" {
		t.Fatalf("真实域子行应携归属站点键与页级作品 ID 身份，实际 %q/%q", realChild.SiteKey, realChild.SiteWorkId)
	}
	// 子行归属键与 FilePluginData 归属字段同构
	var localFP, realFP FilePluginData
	if err := json.Unmarshal([]byte(localChild.PluginData), &localFP); err != nil {
		t.Fatalf("解析 local 子行 PluginData 失败: %v", err)
	}
	if err := json.Unmarshal([]byte(realChild.PluginData), &realFP); err != nil {
		t.Fatalf("解析真实域子行 PluginData 失败: %v", err)
	}
	if localFP.hasRealAttribution() {
		t.Fatalf("local 子行 PluginData 不应携带归属字段，实际 %+v", localFP)
	}
	if !realFP.hasRealAttribution() || realFP.SiteKey != realChild.SiteKey || realFP.SiteWorkId != realChild.SiteWorkId {
		t.Fatalf("真实域子行 PluginData 归属字段应与子行键同构，实际 fp=%+v child=%+v", realFP, realChild)
	}
	// 批次摘要：2 子行、归属分布 pixiv 1 件
	summary := m.takeCreateBatchSummary(root)
	if summary.total != 2 || len(summary.nonLocalBySite) != 1 ||
		summary.nonLocalBySite[0].siteName != "pixiv" || summary.nonLocalBySite[0].files != 1 {
		t.Fatalf("批次摘要应记 2 子行与 pixiv 1 件归属分布，实际 %+v", summary)
	}
}

// —— 退出标准 8：识别后磁盘变动触发对账阻断 —— //

// TestApplyCreateTasksGuards 创建编排动作的守卫链：空选择集、磁盘变动（目录
// 新增文件=识别后变化；所选文件被删除）与赋值修订漂移均阻断（零 CreateTask
// 调用、零标记残留）。
func TestApplyCreateTasksGuards(t *testing.T) {
	t.Run("空选择集", func(t *testing.T) {
		root, m, ctx := recognizedSelectionFixture(t, nil)
		if _, err := m.ApplyCreateTasks(root, ""); err == nil || !strings.Contains(err.Error(), "未选择") {
			t.Fatalf("空选择集应阻断，实际: %v", err)
		}
		if ctx.createTaskCalls() != 0 {
			t.Fatalf("阻断路径不应调用 CreateTask")
		}
	})
	t.Run("识别后目录新增文件", func(t *testing.T) {
		root, m, ctx := recognizedSelectionFixture(t, nil)
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		writeTestFile(t, root+"/多轨/新文件.jpg", "x")
		if _, err := m.ApplyCreateTasks(root, ""); err == nil || !strings.Contains(err.Error(), "磁盘对账未通过") {
			t.Fatalf("识别后目录新增文件应对账阻断，实际: %v", err)
		}
		if ctx.createTaskCalls() != 0 {
			t.Fatalf("对账阻断不应调用 CreateTask")
		}
	})
	t.Run("所选文件被删除", func(t *testing.T) {
		root, m, ctx := recognizedSelectionFixture(t, nil)
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		if err := removeFile(t, root+"/多轨/【作者B】标题2.jpg"); err != nil {
			t.Fatalf("删除所选文件失败: %v", err)
		}
		if _, err := m.ApplyCreateTasks(root, ""); err == nil || !strings.Contains(err.Error(), "磁盘对账未通过") {
			t.Fatalf("所选文件被删除应对账阻断，实际: %v", err)
		}
		if ctx.createTaskCalls() != 0 {
			t.Fatalf("对账阻断不应调用 CreateTask")
		}
	})
	t.Run("赋值修订漂移", func(t *testing.T) {
		root, m, ctx := recognizedSelectionFixture(t, nil)
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		if _, err := m.SetAssignment(root, "多轨/【作者B】标题2.jpg", SessionAssignment{Title: "识别后新赋值"}); err != nil {
			t.Fatalf("识别后写入赋值失败: %v", err)
		}
		if _, err := m.ApplyCreateTasks(root, ""); err == nil || !strings.Contains(err.Error(), "重新识别") {
			t.Fatalf("赋值修订漂移应阻断并提示重新识别，实际: %v", err)
		}
		if ctx.createTaskCalls() != 0 {
			t.Fatalf("修订漂移阻断不应调用 CreateTask")
		}
	})
}

// removeFile 删除测试文件。
func removeFile(t *testing.T, path string) error {
	t.Helper()
	return os.Remove(path)
}

// —— 创建编排动作：对账通过后一次 CreateTask + 选择集清理 —— //

// TestApplyCreateTasksOrchestration 创建编排动作全链：对账通过 → 置发起标记+暂存
// 任务名 → 经宿主 CreateTask 一次调用（local:// 库根）→ 按宿主结论回告创建数与
// 通知文案（空任务名=默认名兜底；宿主桩不产出=无批次摘要，文案不追加归属
// 附注——真实 Create 过滤产出分支由前述用例直接驱动）→ 清理选择集（赋值与
// 快照保留）。CreateTask 由桩应答。
func TestApplyCreateTasksOrchestration(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, map[string]SessionAssignment{
		"多轨/【作者C】标题3.jpg": {Title: "手工标题", UpdatedAtMs: 1},
	})
	if _, err := m.SetSelection(root, []string{"d:多轨", "d:已识别", "f:序列/前缀名_001.jpg"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	outcome, err := m.ApplyCreateTasks(root, "")
	if err != nil {
		t.Fatalf("创建编排失败: %v", err)
	}
	wantName := fmt.Sprintf("本地导入 · %s · 3 件作品", filepath.Base(root))
	if outcome.TaskName != wantName {
		t.Fatalf("空任务名应按默认名兜底，期望 %q 实际 %q", wantName, outcome.TaskName)
	}
	if outcome.Works != 3 || outcome.Tasks != 3 {
		t.Fatalf("创建结论应为 3 作品/宿主 3 任务，实际 %+v", outcome)
	}
	if want := fmt.Sprintf("已创建任务「%s」：3 件作品（3 个子任务，Created 待手动开始）", wantName); outcome.Message != want {
		t.Fatalf("通知文案应为批次任务形态，期望 %q 实际 %q", want, outcome.Message)
	}
	if got := ctx.createTaskURL(); got != "local://"+root {
		t.Fatalf("CreateTask 应以 local:// 库根调用一次，实际 %q", got)
	}
	if ctx.createTaskCalls() != 1 {
		t.Fatalf("CreateTask 应恰调用一次，实际 %d", ctx.createTaskCalls())
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if len(sess.Selection) != 0 {
		t.Fatalf("创建成功后应清理选择集，实际 %v", sess.Selection)
	}
	if len(sess.Assignments) != 1 || sess.Snapshot == nil {
		t.Fatalf("创建成功后应保留赋值与快照，实际 assigns=%d snapshot=%v", len(sess.Assignments), sess.Snapshot != nil)
	}
	// 发起标记仍为置位态：真实流由 Create 分派消费（桩不经 Create，此处核验标记在位）
	if !m.ConsumeWorkbenchCreateMarker(root) {
		t.Fatalf("CreateTask 前置位的发起标记应在位（真实流由 Create 分派消费）")
	}
}

// TestApplyCreateTasksCustomName 自定义任务名：非空载荷（含首尾空白）裁剪后生效，
// 不走默认名兜底。
func TestApplyCreateTasksCustomName(t *testing.T) {
	root, m, _ := recognizedSelectionFixture(t, nil)
	if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	outcome, err := m.ApplyCreateTasks(root, "  我的批次  ")
	if err != nil {
		t.Fatalf("创建编排失败: %v", err)
	}
	if outcome.TaskName != "我的批次" {
		t.Fatalf("自定义任务名应裁剪首尾空白后生效，实际 %q", outcome.TaskName)
	}
	if !strings.Contains(outcome.Message, "已创建任务「我的批次」") {
		t.Fatalf("通知文案应携带生效任务名，实际 %q", outcome.Message)
	}
}

// TestBuildCreateTasksMessageAttributionNote 通知文案的归属附注：无非 local 子行
// 无附注；有则追加「其中 n 件归属 站点名」（按归属分布逐站点一段、顿号连写，
// 多站点分别列出——文案按归属分布组织，与父行形态正交）。
func TestBuildCreateTasksMessageAttributionNote(t *testing.T) {
	pure := buildCreateTasksMessage("本地导入 · 库 · 3 件作品", 3, 3, createBatchSummary{total: 3})
	if want := "已创建任务「本地导入 · 库 · 3 件作品」：3 件作品（3 个子任务，Created 待手动开始）"; pure != want {
		t.Fatalf("纯 local 批次文案不追加附注，期望 %q 实际 %q", want, pure)
	}
	mixed := buildCreateTasksMessage("混归属批次", 5, 5, createBatchSummary{
		total:          5,
		nonLocalBySite: []siteFileCount{{siteName: "pixiv", files: 2}},
	})
	if want := "已创建任务「混归属批次」：5 件作品（5 个子任务，Created 待手动开始）；其中 2 件归属 pixiv"; mixed != want {
		t.Fatalf("混归属批次文案应追加归属附注，期望 %q 实际 %q", want, mixed)
	}
	multiSite := buildCreateTasksMessage("多站点批次", 4, 4, createBatchSummary{
		total:          4,
		nonLocalBySite: []siteFileCount{{siteName: "pixiv", files: 2}, {siteName: "bilibili", files: 1}},
	})
	if want := "已创建任务「多站点批次」：4 件作品（4 个子任务，Created 待手动开始）；其中 2 件归属 pixiv、1 件归属 bilibili"; multiSite != want {
		t.Fatalf("多站点归属附注应逐站点分别列出，期望 %q 实际 %q", want, multiSite)
	}
}

// TestResolveCreateTaskName 任务名裁决：空串/全空白=默认名兜底（本地导入 · 库根
// 名 · N 件作品），非空白=裁剪首尾空白后采用。
func TestResolveCreateTaskName(t *testing.T) {
	if got := resolveCreateTaskName("", `D:\媒体库`, 3); got != `本地导入 · 媒体库 · 3 件作品` {
		t.Fatalf("空任务名应兜底默认名，实际 %q", got)
	}
	if got := resolveCreateTaskName("  \t\n ", `D:\媒体库`, 3); got != `本地导入 · 媒体库 · 3 件作品` {
		t.Fatalf("全空白任务名应兜底默认名，实际 %q", got)
	}
	if got := resolveCreateTaskName("  我的批次 ", `D:\媒体库`, 3); got != "我的批次" {
		t.Fatalf("非空白任务名应裁剪后采用，实际 %q", got)
	}
}

// —— 退出标准 3/5：赋值快照自包含与合并同源 —— //

// TestRecomputeSharesMergedRecognitionLogic 执行期重算与识别干跑的合并结论同源
// （风险13）：同一规则+库内索引+赋值输入下，recomputeFileDecision（消费任务级
// 赋值快照）与 MergedRecognition（干跑批量入口）产出等价终选声明。
func TestRecomputeSharesMergedRecognitionLogic(t *testing.T) {
	root, m, _ := recognizedSelectionFixture(t, nil)
	store := m.store
	profile, _ := LoadProfile(store)
	rules := EffectiveRules(profile)
	lib := BuildLibraryIndex(nil, nil)

	rel := "多轨/【作者C】标题3.jpg"
	full := root + "/" + rel
	assign := SessionAssignment{Title: "手工标题", Collection: "手工集"}
	assigns := map[string]SessionAssignment{rel: assign}

	// 干跑侧：批量入口（注入索引+赋值）
	scanFiles := buildScanFiles([]FileEntry{{FullPath: full, RelPath: rel}})
	batch := MergedRecognition(scanFiles, rules, lib, assigns)
	if len(batch) != 1 {
		t.Fatalf("干跑批量入口应产出一行结论")
	}
	// 重算侧：PluginData 携带任务级赋值快照（与磁盘重扫兄弟全集）
	h := &LocalImportWorkFetcher{}
	fp := &FilePluginData{FullPath: full, RelPath: rel, Assignment: &assign}
	conclusion, _ := h.recomputeFileDecision(fp, rules, lib)

	if len(conclusion.Final) != len(batch[0].Final) {
		t.Fatalf("重算与干跑终选声明数不一致: %d vs %d（%+v / %+v）", len(conclusion.Final), len(batch[0].Final), conclusion.Final, batch[0].Final)
	}
	key := func(d MetadataDeclaration) string {
		return string(d.Dimension) + "=" + d.Value + "|" + string(d.Source)
	}
	seen := map[string]int{}
	for _, decl := range batch[0].Final {
		seen[key(decl)]++
	}
	for _, decl := range conclusion.Final {
		seen[key(decl)]--
	}
	for k, count := range seen {
		if count != 0 {
			t.Fatalf("重算与干跑终选声明不一致（%s 差 %d）——合并逻辑须同源", k, count)
		}
	}
	// 赋值覆盖生效：标题与集名取赋值值
	var title, collection string
	for _, decl := range conclusion.Final {
		if decl.Dimension == DimTitle {
			title = decl.Value
		}
		if decl.Dimension == DimCollection {
			collection = decl.Value
		}
	}
	if title != "手工标题" || collection != "手工集" {
		t.Fatalf("赋值指定维度应覆盖档案结论，实际 title=%q collection=%q", title, collection)
	}
}

// TestAssignmentSnapshotSelfContainedAfterSessionDeletion 退出标准 5：任务级赋值
// 快照固化后删除会话，执行期重算元数据不回退（快照自包含——回退侧=档案求值
// 只得内置规则标题「标题3」）。
func TestAssignmentSnapshotSelfContainedAfterSessionDeletion(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, map[string]SessionAssignment{
		"多轨/【作者C】标题3.jpg": {Title: "手工标题", UpdatedAtMs: 1},
	})
	if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	resps := collectWorkbenchCreate(t, m, ctx, root, "")
	if len(resps) != 1 {
		t.Fatalf("纯 local 批次应为单应答单父，实得 %+v", resps)
	}
	var target *sdkdto.TaskCreateChildResponse
	for _, child := range resps[0].Children {
		if child.TaskName == "【作者C】标题3.jpg" {
			target = child
		}
	}
	if target == nil {
		t.Fatalf("缺少已赋值轨子任务")
	}

	// 删除会话后重算：任务自包含，赋值快照优先、元数据不回退
	if err := m.DeleteSession(root); err != nil {
		t.Fatalf("删除会话失败: %v", err)
	}
	h := &LocalImportWorkFetcher{ctx: ctx}
	resp, err := h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &target.PluginData})
	if err != nil {
		t.Fatalf("删除会话后 CreateWorkInfo 失败: %v", err)
	}
	if resp.Work == nil || resp.Work.SiteWorkName == nil || *resp.Work.SiteWorkName != "手工标题" {
		t.Fatalf("删除会话后重算应消费任务级赋值快照（作品名=手工标题），实际 %+v", resp.Work)
	}
}

// —— 退出标准 4：干跑与会话模式谓词声明两侧一致（风险12 闭环） —— //

// TestPredicateConsistencyAcrossDryRunCreateAndRecompute 谓词规则（吃库内索引）
// 三点一致：①干跑注入索引 → 谓词识别作品可选（全景所见）；②会话模式 Create
// 产出该作品任务（过滤产出消费同一快照基面）；③删除会话后执行期重算注入
// 同一索引 → 谓词声明再现（任务所得）。任一点漏注入即断言失败。
func TestPredicateConsistencyAcrossDryRunCreateAndRecompute(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, nil)

	// ① 干跑侧：谓词识别的作品（根目录聚合作品 d:""）在域内可选
	if _, err := m.SetSelection(root, []string{"d:"}); err != nil {
		t.Fatalf("谓词识别作品应在干跑结论下可选: %v", err)
	}

	// ② 创建侧：过滤产出该作品任务（批次父行+子行形态）
	resps := collectWorkbenchCreate(t, m, ctx, root, "")
	if len(resps) != 1 || len(resps[0].Children) != 1 || resps[0].Children[0].TaskName != "库内作者名.jpg" {
		t.Fatalf("所选谓词识别作品应产出批次父行+单子行，实得 %+v", resps)
	}
	leaf := resps[0].Children[0]

	// ③ 重算侧：删除会话后 CreateWorkInfo 注入同一库内索引 → 谓词作者声明再现
	if err := m.DeleteSession(root); err != nil {
		t.Fatalf("删除会话失败: %v", err)
	}
	h := &LocalImportWorkFetcher{ctx: ctx}
	resp, err := h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &leaf.PluginData})
	if err != nil {
		t.Fatalf("删除会话后 CreateWorkInfo 失败: %v", err)
	}
	found := false
	for _, author := range resp.LocalAuthors {
		if author.AuthorName != nil && *author.AuthorName == "库内作者名" {
			found = true
		}
	}
	if !found {
		t.Fatalf("执行期重算应注入库内索引使谓词声明再现（作者=库内作者名），实际 %+v", resp.LocalAuthors)
	}
}

// —— 前端动作分派：setSelection / createTasks —— //

// TestWorkbenchSelectionActionsDispatch 前端事件协议的选择族动作分派：
// setSelection 落库并回告决策态；createTasks 携可选 taskName 走编排（桩
// CreateTask 成功）并回告创建数与摘要（任务名生效）；未知动作回告错误。
func TestWorkbenchSelectionActionsDispatch(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, nil)

	m.HandleFrontendRequest([]byte(`{"seq":41,"action":"setSelection","root":` + jsonQuote(root) + `,"works":["d:多轨","d:已识别"]}`))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Decisions == nil || len(resp.Decisions.Selection) != 2 {
		t.Fatalf("setSelection 应回告已落库决策态，实际 %+v", resp)
	}

	m.HandleFrontendRequest([]byte(`{"seq":42,"action":"createTasks","root":` + jsonQuote(root) + `,"taskName":"命名批次"}`))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.Created != 2 || resp.CreateMsg == "" {
		t.Fatalf("createTasks 应回告创建结论，实际 %+v", resp)
	}
	if !strings.Contains(resp.CreateMsg, "已创建任务「命名批次」") {
		t.Fatalf("createTasks 摘要应携带生效任务名（taskName 载荷透传），实际 %q", resp.CreateMsg)
	}
	if resp.Decisions == nil || len(resp.Decisions.Selection) != 0 {
		t.Fatalf("createTasks 后选择集应清理，实际 %+v", resp.Decisions)
	}

	m.HandleFrontendRequest([]byte(`{"seq":43,"action":"setSelection","root":` + jsonQuote(root) + `,"works":["d:未识别"]}`))
	resp = ctx.lastResponse(t)
	if resp.OK || !strings.Contains(resp.Error, "不可选") {
		t.Fatalf("不可选作品的 setSelection 应回告错误，实际 %+v", resp)
	}
}

// jsonQuote 测试内 JSON 字符串转义。
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
