// 资源选择与任务创建（阶段4）的单元测试：作品原子选择域推导与可选性校验、
// 选择集持久化（未识别/待裁决不可选）、工作台发起创建的过滤产出（只产出所选
// 作品、集父行按所选成员生成、任务级赋值快照固化、零弹问）、创建编排动作
//（磁盘对账阻断、CreateTask 一次调用、选择集清理）与执行期重算的同源断言
//（干跑/创建/重算三点注入同一谓词索引与合并实现——风险12/13 闭环）。
package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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
			Pattern: mustRegexPattern(t, `^.+$`, nil),
			Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromGroupPrefix}},
		},
		{
			ID: "test-conflict-a", Description: "测试：标题冲突 A", Source: RuleSourceManual, Enabled: true,
			Pattern: conflictA,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c1"}},
		},
		{
			ID: "test-conflict-b", Description: "测试：标题冲突 B", Source: RuleSourceManual, Enabled: true,
			Pattern: conflictB,
			Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromCapture, CaptureName: "c2"}},
		},
		{
			ID: "test-predicate-author", Description: "测试：文件名命中库内作者", Source: RuleSourceManual, Enabled: true,
			Pattern: &PredicatePattern{Kind: PredicateAuthor},
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
		"d:未识别":  false,
		"d:序列":   false, // 落单_009 为 plain → 整件不可选
		"d:冲突":   false, // 待裁决 → 整件不可选
		"d:":     true,  // 库内作者名.jpg 谓词识别 → 可选
		"d:已识别":  true,
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

// collectWorkbenchCreate 经发起标记驱动 Create 的过滤产出分支并收集全部应答。
func collectWorkbenchCreate(t *testing.T, m *SessionManager, ctx *workbenchTestCtx, root string) []*sdkdto.TaskCreateResponse {
	t.Helper()
	h := &LocalImportWorkFetcher{sessions: m, ctx: ctx}
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

// responseChildNames 应答的子任务文件名集合（集父行按所选成员生成的断言载体）。
func responseChildNames(resp *sdkdto.TaskCreateResponse) []string {
	names := make([]string, 0, len(resp.Children))
	for _, child := range resp.Children {
		names = append(names, child.TaskName)
	}
	return names
}

// findResponseByParent 按父任务身份定位应答（未命中 nil）。
func findResponseByParent(resps []*sdkdto.TaskCreateResponse, parentID string) *sdkdto.TaskCreateResponse {
	for _, r := range resps {
		if r.PluginTaskId == parentID {
			return r
		}
	}
	return nil
}

// TestCreateWorkbenchFilteredEmitsSelectedWorksOnly 会话模式 Create 过滤产出：
// 选择=「d:多轨 双轨作品」+「集内 2/3 成员显式拆选」→ 只产出所选作品：目录聚合
// 作品父行含全部双轨（原子），集父行按所选成员生成（仅 2 子任务），未选作品
// （d:已识别、第三名集成员）零任务，未识别/待裁决作品即使混入选择集也非可选
//（写入即拒）。全程零前端事件（零 ClassifyDir 弹问——目录含义取快照回放，
// 结构性不触问答）。PluginData 携带内容哈希与赋值快照。
func TestCreateWorkbenchFilteredEmitsSelectedWorksOnly(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, map[string]SessionAssignment{
		"多轨/【作者C】标题3.jpg": {Title: "手工标题", UpdatedAtMs: 1},
	})
	if _, err := m.SetSelection(root, []string{"d:多轨", "f:序列/前缀名_001.jpg", "f:序列/前缀名_002.jpg"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}

	before := ctx.publishedCount()
	resps := collectWorkbenchCreate(t, m, ctx, root)
	if len(resps) != 2 {
		t.Fatalf("所选=双轨作品+缺员集（2 应答：目录父行+集父行），实得 %d 个: %+v", len(resps), resps)
	}

	// 目录聚合作品：父行 + 全部双轨（原子，轨道不可拆）
	dirResp := findResponseByParent(resps, "local-dir-多轨")
	if dirResp == nil {
		t.Fatalf("缺少目录聚合作品父行 local-dir-多轨（应答: %+v）", resps)
	}
	if len(dirResp.Children) != 2 || strings.Join(responseChildNames(dirResp), ",") != "【作者B】标题2.jpg,【作者C】标题3.jpg" {
		t.Fatalf("目录聚合作品父行应含全部双轨，实际 %v", responseChildNames(dirResp))
	}
	// 赋值快照固化：标题3 轨携带手工标题、标题2 轨无快照
	for _, child := range dirResp.Children {
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

	// 集父行按所选成员生成：3 员集选 2 → 父行恰含所选 2 员（序号序）
	setResp := findResponseByParent(resps, "local-set-序列/前缀名")
	if setResp == nil {
		t.Fatalf("缺员集应生成集父行 local-set-序列/前缀名（应答: %+v）", resps)
	}
	if setResp.TaskName != "前缀名" {
		t.Fatalf("集父行任务名应为集名候选（分组键），实际 %q", setResp.TaskName)
	}
	if strings.Join(responseChildNames(setResp), ",") != "前缀名_001.jpg,前缀名_002.jpg" {
		t.Fatalf("集父行应按所选成员生成（2/3），实际 %v", responseChildNames(setResp))
	}

	// 未选项零任务：d:已识别 与 第三名集成员不在任何应答中
	for _, r := range resps {
		for _, name := range responseChildNames(r) {
			if name == "【作者A】标题1.jpg" || name == "前缀名_003.jpg" {
				t.Fatalf("未选作品出现在产出中: %s", name)
			}
		}
		if r.PluginTaskId == "【作者A】标题1.jpg" {
			t.Fatalf("未选单轨作品不应成叶任务")
		}
	}

	// 零弹问（退出标准 6）：创建全程零新增前端事件——ClassifyDir 的问答与入口
	// 确认弹窗均不触发（过滤产出分支对分类器结构性零引用，目录含义取快照回放）
	if after := ctx.publishedCount(); after != before {
		t.Fatalf("会话模式创建应零前端事件（零 ClassifyDir 弹问），实得创建期新增 %d 条", after-before)
	}
}

// TestCreateWorkbenchFilteredLoneSetMemberLeaf 集内仅选一名成员：单文件成叶任务
//（身份=内容哈希，落 local 域），不生成集父行。
func TestCreateWorkbenchFilteredLoneSetMemberLeaf(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, nil)
	if _, err := m.SetSelection(root, []string{"f:序列/前缀名_003.jpg"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	resps := collectWorkbenchCreate(t, m, ctx, root)
	if len(resps) != 1 || len(resps[0].Children) != 0 {
		t.Fatalf("单名集成员应成一叶任务（1 应答零子任务），实得 %+v", resps)
	}
	if resps[0].TaskName != "前缀名_003.jpg" || resps[0].SiteWorkId == "" {
		t.Fatalf("叶任务应为所选文件（哈希身份），实际 %+v", resps[0])
	}
}

// —— 退出标准 8：识别后磁盘变动触发对账阻断 —— //

// TestApplyCreateTasksGuards 创建编排动作的守卫链：空选择集、磁盘变动（目录
// 新增文件=识别后变化；所选文件被删除）与赋值修订漂移均阻断（零 CreateTask
// 调用、零标记残留）。
func TestApplyCreateTasksGuards(t *testing.T) {
	t.Run("空选择集", func(t *testing.T) {
		root, m, ctx := recognizedSelectionFixture(t, nil)
		if _, err := m.ApplyCreateTasks(root); err == nil || !strings.Contains(err.Error(), "未选择") {
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
		if _, err := m.ApplyCreateTasks(root); err == nil || !strings.Contains(err.Error(), "磁盘对账未通过") {
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
		if _, err := m.ApplyCreateTasks(root); err == nil || !strings.Contains(err.Error(), "磁盘对账未通过") {
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
		if _, err := m.ApplyCreateTasks(root); err == nil || !strings.Contains(err.Error(), "重新识别") {
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

// TestApplyCreateTasksOrchestration 创建编排动作全链：对账通过 → 置发起标记 →
// 经宿主 CreateTask 一次调用（local:// 库根）→ 按宿主结论回告创建数 → 清理
// 选择集（赋值与快照保留）。CreateTask 由桩应答（真实 Create 过滤产出分支由
// 前述用例直接驱动）。
func TestApplyCreateTasksOrchestration(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, map[string]SessionAssignment{
		"多轨/【作者C】标题3.jpg": {Title: "手工标题", UpdatedAtMs: 1},
	})
	if _, err := m.SetSelection(root, []string{"d:多轨", "d:已识别", "f:序列/前缀名_001.jpg"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	outcome, err := m.ApplyCreateTasks(root)
	if err != nil {
		t.Fatalf("创建编排失败: %v", err)
	}
	if outcome.Works != 3 || outcome.Tasks != 3 {
		t.Fatalf("创建结论应为 3 作品/宿主 3 任务，实际 %+v", outcome)
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

// —— 退出标准 3/5：赋值快照自包含与合并同源 —— //

// TestRecomputeSharesMergedRecognitionLogic 执行期重算与识别干跑的合并结论同源
//（风险13）：同一规则+库内索引+赋值输入下，recomputeFileDecision（消费任务级
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
	key := func(d MetadataDeclaration) string { return string(d.Dimension) + "=" + d.Value + "|" + string(d.Source) }
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
	resps := collectWorkbenchCreate(t, m, ctx, root)
	dirResp := findResponseByParent(resps, "local-dir-多轨")
	if dirResp == nil {
		t.Fatalf("缺少目录聚合作品父行: %+v", resps)
	}
	var target *sdkdto.TaskCreateChildResponse
	for _, child := range dirResp.Children {
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

	// ② 创建侧：过滤产出该作品任务
	resps := collectWorkbenchCreate(t, m, ctx, root)
	if len(resps) != 1 || resps[0].TaskName != "库内作者名.jpg" {
		t.Fatalf("所选谓词识别作品应产出单叶任务，实得 %+v", resps)
	}
	leaf := resps[0]

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
// setSelection 落库并回告决策态；createTasks 走编排（桩 CreateTask 成功）并
// 回告创建数与摘要；未知动作回告错误。
func TestWorkbenchSelectionActionsDispatch(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, nil)

	m.HandleFrontendRequest([]byte(`{"seq":41,"action":"setSelection","root":` + jsonQuote(root) + `,"works":["d:多轨","d:已识别"]}`))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Decisions == nil || len(resp.Decisions.Selection) != 2 {
		t.Fatalf("setSelection 应回告已落库决策态，实际 %+v", resp)
	}

	m.HandleFrontendRequest([]byte(`{"seq":42,"action":"createTasks","root":` + jsonQuote(root) + `}`))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.Created != 2 || resp.CreateMsg == "" {
		t.Fatalf("createTasks 应回告创建结论，实际 %+v", resp)
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
