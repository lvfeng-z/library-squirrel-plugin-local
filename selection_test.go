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
	"reflect"
	"strings"
	"testing"
	"time"

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
// 作者规则（吃库内索引）、双规则同层冲突（待裁决形态）、括号命名惯例
// （内置退休后由档案条目承载——多轨目录两文件靠它完成识别）。
func selectionTestRules(t *testing.T) []RuleEntry {
	t.Helper()
	sibling, err := NewSiblingPattern(`^(.+)_`, `(\d+)`, 2, OutcomeCollection)
	if err != nil {
		t.Fatalf("构造兄弟群组规则失败: %v", err)
	}
	conflictA := mustRegexPattern(t, `^(?P<c1>A).+\.jpg$`, []string{"c1"})
	conflictB := mustRegexPattern(t, `^.+(?P<c2>B)\.jpg$`, []string{"c2"})
	bracketRows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "【{作者}】{标题}"},
	}}}
	bracketPattern, err := CompileRuleConditions(bracketRows)
	if err != nil {
		t.Fatalf("构造括号命名惯例模板规则失败: %v", err)
	}
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
			ID: "sel-bracket-author-title", Description: "测试：【作者】标题命名惯例", Source: RuleSourceManual, Enabled: true,
			Conditions: bracketRows, Pattern: bracketPattern,
			Extractions: []ExtractionAction{
				{Dimension: DimAuthor, From: FromCapture, CaptureName: condxVocab["作者"], Row: 1},
				{Dimension: DimTitle, From: FromCapture, CaptureName: condxVocab["标题"], Row: 1},
			},
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

// collectWorkbenchCreate 经发起标记驱动 Create 的过滤产出分支并收集全部 task 块
// （条目形态流的批次块由专门用例断言，此处取父行断言载体）；taskName 模拟编排
// 动作暂存的创建任务名原文（空串=默认名兜底路径——与真实编排 ApplyCreateTasks
// 的暂存行为同构）；hostContractFn 注入 21（B1+ 分帧路径）。
func collectWorkbenchCreate(t *testing.T, m *SessionManager, ctx *workbenchTestCtx, root, taskName string) []*sdkdto.TaskCreateResponse {
	t.Helper()
	h := &LocalImportWorkFetcher{sessions: m, ctx: ctx, hostContractFn: func() uint32 { return createChildrenBatchMinHostContract }}
	m.stageCreateTaskName(root, taskName)
	if err := m.BeginWorkbenchCreate(root); err != nil {
		t.Fatalf("置位发起标记失败: %v", err)
	}
	result, err := h.Create("local://" + root)
	if err != nil {
		t.Fatalf("工作台发起创建不应报错: %v", err)
	}
	if result.Items() == nil {
		t.Fatalf("过滤产出应为条目形态流式结果")
	}
	var out []*sdkdto.TaskCreateResponse
	for item := range result.Items() {
		if task := item.Task(); task != nil {
			out = append(out, task)
		}
	}
	return out
}

// classifyQuestionCount 分类问答发布计数（零弹问断言的口径：classify:request
// 主题的发布数——createProgress 等进度事件属预期发布，不计入）。
func (c *workbenchTestCtx) classifyQuestionCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, p := range c.published {
		if p.topic == "plugin:local-import:classify:request" {
			n++
		}
	}
	return n
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

	// 零弹问（退出标准 6）：创建全程零分类问答（ClassifyDir 结构性不触问答，
	// 目录含义取快照回放）；createProgress 进度事件属预期发布不计入
	before := ctx.classifyQuestionCount()
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

	// 零弹问（退出标准 6）：创建全程零分类问答与零入口确认事件——ClassifyDir 的
	// 问答与确认弹窗均不触发（过滤产出分支对分类器结构性零引用，目录含义取快照
	// 回放）；createProgress 进度事件属预期发布不计入
	if after := ctx.classifyQuestionCount(); after != before {
		t.Fatalf("会话模式创建应零分类问答（零 ClassifyDir 弹问），实得创建期新增 %d 条", after-before)
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
// 文件集 + 空快照会话（快照无目录记录 → 含义链为空，归属裁决只走文件名形态）；
// hostContractFn 注入 21（B1+ 分帧路径——单批装得下时即纯内联形态）。
func emitFixture(t *testing.T, relPaths []string) (string, *LocalImportWorkFetcher, *SessionManager, *WorkbenchSession) {
	t.Helper()
	root := t.TempDir()
	for _, rel := range relPaths {
		writeTestFile(t, root+"/"+rel, rel)
	}
	m := NewSessionManagerWithStore(nil, nil, newFakeSessionStore())
	h := &LocalImportWorkFetcher{sessions: m, hostContractFn: func() uint32 { return createChildrenBatchMinHostContract }}
	return root, h, m, &WorkbenchSession{Snapshot: &RecognitionSnapshot{}}
}

// collectEmittedWithReason 直驱 emitSelectedWorks 收集全部发射条目与中止结论
// （同步调用，结束后收卷；取消路径返回 cancelled=真——直驱不经编排 goroutine，
// 中止原因/取消收尾由本助手原样转交断言面）。
func collectEmittedWithReason(t *testing.T, h *LocalImportWorkFetcher, root string, sess *WorkbenchSession, works []*SelectableWork, taskName string) ([]*sdkdto.CreateStreamItem, string, bool) {
	t.Helper()
	ch := make(chan *sdkdto.CreateStreamItem, 16)
	reason, cancelled := h.emitSelectedWorks(root, sess, works, taskName, ch, sdkdto.NoopHeartbeat{})
	close(ch)
	var out []*sdkdto.CreateStreamItem
	for item := range ch {
		out = append(out, item)
	}
	return out, reason, cancelled
}

// collectEmitted 直驱 emitSelectedWorks 收集全部发射条目（断言无中止原因与取消）。
func collectEmitted(t *testing.T, h *LocalImportWorkFetcher, root string, sess *WorkbenchSession, works []*SelectableWork, taskName string) []*sdkdto.CreateStreamItem {
	t.Helper()
	items, reason, cancelled := collectEmittedWithReason(t, h, root, sess, works, taskName)
	if reason != "" || cancelled {
		t.Fatalf("emitSelectedWorks 不应中止: reason=%s cancelled=%v", reason, cancelled)
	}
	return items
}

// emittedTasks 发射条目序列中的全部 task 块（按发射序）。
func emittedTasks(items []*sdkdto.CreateStreamItem) []*sdkdto.TaskCreateResponse {
	var out []*sdkdto.TaskCreateResponse
	for _, item := range items {
		if task := item.Task(); task != nil {
			out = append(out, task)
		}
	}
	return out
}

// emittedBatches 发射条目序列中的全部子行批次块（按发射序）。
func emittedBatches(items []*sdkdto.CreateStreamItem) []*sdkdto.TaskChildrenBatch {
	var out []*sdkdto.TaskChildrenBatch
	for _, item := range items {
		if batch := item.ChildrenBatch(); batch != nil {
			out = append(out, batch)
		}
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
	resps := emittedTasks(collectEmitted(t, h, root, sess, works, "整批导入"))
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
	resps := emittedTasks(collectEmitted(t, h, root, sess, works, "混归属批次"))
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

// TestApplyCreateTasksGuards 创建编排动作的守卫链：空选择集、目录级 mtime 游标
// 对账（识别后新增文件=游标漂移；仅推移目录 mtime 亦拒绝；所选文件被删除时其
// 目录 mtime 随之漂移、同被拦截）与赋值修订漂移均阻断（零 CreateTask 调用、零
// 标记残留）。逐文件存在性核对已并入 verify 哈希循环（对账拆分·刀4，中断语义
// 见 TestReconcileSplitFileGoneAbortsInVerify）。
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
	t.Run("仅目录 mtime 漂移（无文件变化）", func(t *testing.T) {
		root, m, ctx := recognizedSelectionFixture(t, nil)
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		dir := root + "/多轨"
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("读取目录失败: %v", err)
		}
		shifted := fi.ModTime().Add(2 * time.Second)
		if err := os.Chtimes(dir, shifted, shifted); err != nil {
			t.Fatalf("推移目录 mtime 失败: %v", err)
		}
		if _, err := m.ApplyCreateTasks(root, ""); err == nil ||
			!strings.Contains(err.Error(), "磁盘对账未通过") || !strings.Contains(err.Error(), "在识别后有变动") {
			t.Fatalf("目录 mtime 游标漂移应对账阻断（对账拆分后守卫链仍拒绝），实际: %v", err)
		}
		if ctx.createTaskCalls() != 0 {
			t.Fatalf("对账阻断不应调用 CreateTask")
		}
	})
	t.Run("所选文件被删除（目录 mtime 随之漂移，目录级对账拦截）", func(t *testing.T) {
		root, m, ctx := recognizedSelectionFixture(t, nil)
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		if err := removeFile(t, root+"/多轨/【作者B】标题2.jpg"); err != nil {
			t.Fatalf("删除所选文件失败: %v", err)
		}
		if _, err := m.ApplyCreateTasks(root, ""); err == nil || !strings.Contains(err.Error(), "磁盘对账未通过") {
			t.Fatalf("所选文件被删除应阻断（其目录 mtime 漂移由目录级对账拦截），实际: %v", err)
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

// —— 对账拆分等价（守卫链与会话瘦身四刀之四，阶段3 退出标准③）—— //

// TestReconcileSplitFileGoneAbortsInVerify 逐文件存在性核对移入 verify 哈希循环
// 后的等价锚定：目录级游标不变（隔离目录对账路径）时守卫链放行，所选文件不可
// 访问在 verify 循环 Stat 处整次创建中断——对账文案经中止原因收流（error 块 →
// 宿主回告失败 → createDone 失败终态），零子行发射、选择集保留可重试（与拆分
// 前守卫链拒绝同一用户可见结局，仅核对时机后移）。
func TestReconcileSplitFileGoneAbortsInVerify(t *testing.T) {
	t.Run("直驱 emitSelectedWorks：对账文案收流且零发射", func(t *testing.T) {
		root, h, m, sess := emitFixture(t, []string{"对账甲/x1.jpg", "对账甲/x2.jpg"})
		works := []*SelectableWork{{
			WorkID: "d:对账甲", Kind: WorkKindDirTracks, DirRelKey: "对账甲",
			Files: []string{"对账甲/x1.jpg", "对账甲/x2.jpg"},
		}}
		if err := removeFile(t, root+"/对账甲/x2.jpg"); err != nil {
			t.Fatalf("删除所选文件失败: %v", err)
		}
		items, reason, cancelled := collectEmittedWithReason(t, h, root, sess, works, "对账批次")
		if reason == "" || !strings.Contains(reason, "磁盘对账未通过") ||
			!strings.Contains(reason, "对账甲/x2.jpg") || !strings.Contains(reason, "已不存在") {
			t.Fatalf("文件缺失应经对账文案中止收流（沿用守卫链拒绝文案），实际 %q", reason)
		}
		if cancelled {
			t.Fatalf("对账中止是失败收尾而非取消收尾")
		}
		if len(items) != 0 {
			t.Fatalf("对账中止应零发射（含已哈希完成的在先轨道），实际 %d 条", len(items))
		}
		if summary := m.takeCreateBatchSummary(root); summary.total != 0 {
			t.Fatalf("对账中止不应记录批次摘要，实际 %+v", summary)
		}
	})
	t.Run("全链：守卫放行 → verify 中断 → 失败终态且选择集保留", func(t *testing.T) {
		root, m, ctx := recursiveCreateFixture(t, nil)
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		// 删除所选一轨并复原目录 mtime 至识别时值：目录级游标对账不动（隔离
		// 逐文件路径），中断只能发生在 verify 哈希循环的 Stat
		dir := root + "/多轨"
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("读取目录失败: %v", err)
		}
		if err := removeFile(t, root+"/多轨/【作者B】标题2.jpg"); err != nil {
			t.Fatalf("删除所选文件失败: %v", err)
		}
		if err := os.Chtimes(dir, fi.ModTime(), fi.ModTime()); err != nil {
			t.Fatalf("复原目录 mtime 失败: %v", err)
		}
		started, err := m.ApplyCreateTasks(root, "")
		if err != nil || !started {
			t.Fatalf("目录级对账通过时守卫链应放行（文件级核对已移入 verify）: started=%v err=%v", started, err)
		}
		done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
		if done.Success {
			t.Fatalf("verify 期文件不可访问应中断创建，实际成功: %+v", done)
		}
		if !strings.Contains(done.Message, "磁盘对账未通过") || !strings.Contains(done.Message, "已不存在") {
			t.Fatalf("中断文案应为对账文案（所选文件已不存在），实际 %q", done.Message)
		}
		sess, err := m.SessionOf(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		if len(sess.Selection) != 1 {
			t.Fatalf("中断后选择集应保留供重试，实际 %v", sess.Selection)
		}
	})
}

// —— 守卫段提速锚定（阶段3 退出标准①）—— //

// syntheticLargeSessionFixture 合成大会话夹具：指定目录数×每目录文件数的已识别
// 快照（全部 identified → 目录聚合作品可选）经 CompleteRecognition 全保存落库
//（真实 gzip 分片载荷），并预演「全景载入完成」形态——缓存条目装填 + 域推导
// 驻留（真实时序=前端拉取 domain 分节）。DirMtimes 与磁盘 ProfileDigest 刻意
// 缺席（目录级对账与档案指纹闸门按缺席放行，计时聚焦守卫链自身通路）。
func syntheticLargeSessionFixture(t *testing.T, dirCount, filesPerDir int) (string, *SessionManager, *workbenchTestCtx, *countingSessionStore) {
	t.Helper()
	root := t.TempDir()
	store := newCountingSessionStore()
	ctx := &workbenchTestCtx{
		store:          store.fakeSessionStore,
		createTaskStub: &sdkdto.CreateTaskResult{Succeed: true, AddedQuantity: 3},
	}
	m := NewSessionManagerWithStore(ctx, nil, store)
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion, GeneratedAtMs: 1000, RootPath: root,
	}
	for d := 0; d < dirCount; d++ {
		dirKey := fmt.Sprintf("目录%05d", d)
		for f := 0; f < filesPerDir; f++ {
			rel := fmt.Sprintf("%s/文件%03d.jpg", dirKey, f)
			snap.Files = append(snap.Files, SnapFile{
				RelPath: rel, FileName: fileNameOfRel(rel), DirRelKey: dirKey, State: snapStateIdentified,
			})
		}
	}
	if _, err := m.CompleteRecognition(root, snap); err != nil {
		t.Fatalf("注入合成快照失败: %v", err)
	}
	shared, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("装填缓存失败: %v", err)
	}
	m.workDomainOf(shared) // 全景 domain 分节的域推导驻留
	return root, m, ctx, store
}

// TestApplyCreateTasksGuardSegmentFastOnSyntheticLargeSession 合成大会话夹具下
// createTasks 动作守卫段 <3s（阶段3 退出标准①）：12 万文件/6 千目录（真实库
// 20.7 万文件量级过半、目录数同量级）全选创建——守卫链走缓存条目读（零分片
// 解码）+ 域推导复用（零重推导）+ 目录级对账（零逐文件 Stat）+ 外置标记 O(1)
// 置位；收尾清理（选择集清空）走信封级轻变异，全程零载荷分片写。
func TestApplyCreateTasksGuardSegmentFastOnSyntheticLargeSession(t *testing.T) {
	root, m, ctx, store := syntheticLargeSessionFixture(t, 6000, 20)
	shared, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	ids := make([]string, 0, len(shared.Snapshot.Files))
	for _, work := range m.workDomainOf(shared).Works {
		ids = append(ids, work.WorkID)
	}
	if _, err := m.SetSelection(root, ids); err != nil {
		t.Fatalf("写入全量选择集失败: %v", err)
	}

	store.resetWriteCounts()
	start := time.Now()
	started, err := m.ApplyCreateTasks(root, "守卫计时批次")
	elapsed := time.Since(start)
	if err != nil || !started {
		t.Fatalf("合成大会话守卫链应放行: started=%v err=%v", started, err)
	}
	if elapsed >= 3*time.Second {
		t.Fatalf("合成大会话（%d 文件）createTasks 守卫段耗时 %v ≥ 3s", 6000*20, elapsed)
	}
	done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	if !done.Success {
		t.Fatalf("桩创建应成功收尾: %s", done.Message)
	}
	if store.shardSetValueCalls() != 0 {
		t.Fatalf("守卫与信封级收尾不得触载荷分片（刀2），实际分片写 %d 次", store.shardSetValueCalls())
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if len(sess.Selection) != 0 || sess.Snapshot == nil {
		t.Fatalf("收尾应经信封级轻变异清空选择集且载荷仍在缓存实例: sel=%d snap=%v", len(sess.Selection), sess.Snapshot == nil)
	}
}

// —— 创建编排动作：异步化（立即应答 started + goroutine 创建 + createDone 终态）—— //

// TestApplyCreateTasksOrchestration 创建编排动作全链（异步化后）：对账通过 → 置
// 发起标记+暂存任务名 → 立即应答 started=true（创建在独立 goroutine）→ 宿主
// CreateTask 一次调用（local:// 库根）→ createDone 成功事件携带默认名兜底通知
// 文案（空任务名；宿主桩不产出=无批次摘要，文案不追加归属附注——真实 Create
// 过滤产出分支由前述用例直接驱动）→ 清理选择集（赋值与快照保留）。CreateTask
// 由桩应答。
func TestApplyCreateTasksOrchestration(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, map[string]SessionAssignment{
		"多轨/【作者C】标题3.jpg": {Title: "手工标题", UpdatedAtMs: 1},
	})
	if _, err := m.SetSelection(root, []string{"d:多轨", "d:已识别", "f:序列/前缀名_001.jpg"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	started, err := m.ApplyCreateTasks(root, "")
	if err != nil || !started {
		t.Fatalf("创建编排应立即应答已开始创建: started=%v err=%v", started, err)
	}
	wantName := fmt.Sprintf("本地导入 · %s · 3 件作品", filepath.Base(root))
	done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	if !done.Success {
		t.Fatalf("桩创建应成功收尾，实际: %s", done.Message)
	}
	if want := fmt.Sprintf("已创建任务「%s」：3 件作品，共 3 个子任务，用时不足 1 分钟", wantName); done.Message != want {
		t.Fatalf("createDone 文案应为批次任务形态（默认名兜底），期望 %q 实际 %q", want, done.Message)
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
// 不走默认名兜底（createDone 文案携带生效名）。
func TestApplyCreateTasksCustomName(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, nil)
	if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	started, err := m.ApplyCreateTasks(root, "  我的批次  ")
	if err != nil || !started {
		t.Fatalf("创建编排应立即应答已开始创建: started=%v err=%v", started, err)
	}
	done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	if !done.Success || !strings.Contains(done.Message, "已创建任务「我的批次」") {
		t.Fatalf("createDone 文案应携带裁剪后生效的任务名，实际: %+v", done)
	}
}

// —— createTasks 异步化：宿主同步递归链的事件序列（大载荷分页方案·设计 C）—— //

// recursiveCreateTaskCtx CreateTask 桩模拟宿主同步递归：调用即同步驱动插件的
// Create 过滤产出分支并消费完整产出流（还原「CreateTask RPC 同步覆盖完整产出
// 流」的真实宿主时序），按实际产出子行数回告 AddedQuantity；transportFail=真
// 返回传输错误（零产出）；afterEmitFail=真在产出消费完毕后回告宿主落库失败
// （部分完成形态——发射进度槽已落账）；block 闸可挂起创建（并发拒绝断言用）。
type recursiveCreateTaskCtx struct {
	workbenchTestCtx
	fetcher       *LocalImportWorkFetcher
	block         chan struct{}
	transportFail bool
	afterEmitFail bool
}

// CreateTask 宿主同步递归桩：真实链=编排 goroutine → m.ctx.CreateTask → 宿主 →
// 插件 Create（过滤产出分支）→ 完整产出流消费 → CreateTask 返回；产出流以
// SetReason 中止（如对账文案收流，对账拆分·刀4）时按宿主口径回告失败——error
// 块收流后 CreateTask RPC 返回 Succeed=false+Msg=reason。
func (c *recursiveCreateTaskCtx) CreateTask(url string) (*sdkdto.CreateTaskResult, error) {
	if c.block != nil {
		<-c.block
	}
	if c.transportFail {
		return nil, fmt.Errorf("宿主连接中断（测试注入）")
	}
	result, err := c.fetcher.Create(url)
	if err != nil {
		return nil, err
	}
	if result.Items() == nil {
		return nil, fmt.Errorf("过滤产出应为条目形态流式结果（B1+）")
	}
	added := 0
	for item := range result.Items() {
		if task := item.Task(); task != nil {
			added += len(task.Children)
		} else if batch := item.ChildrenBatch(); batch != nil {
			added += len(batch.Items)
		}
	}
	if reason := result.Reason(); reason != "" {
		return &sdkdto.CreateTaskResult{Succeed: false, Msg: reason}, nil
	}
	if c.afterEmitFail {
		return &sdkdto.CreateTaskResult{Succeed: false, Msg: "宿主任务落库失败（测试注入）"}, nil
	}
	return &sdkdto.CreateTaskResult{Succeed: true, AddedQuantity: added}, nil
}

// recursiveCreateFixture 「宿主同步递归」创建链测试环境：样例树+全量识别与
// recognizedSelectionFixture 同构，但 CreateTask 桩同步驱动真实过滤产出分支
// （宿主契约注入 21——B1+ 分帧路径）；block 为可选挂起闸。
func recursiveCreateFixture(t *testing.T, block chan struct{}) (string, *SessionManager, *recursiveCreateTaskCtx) {
	t.Helper()
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &recursiveCreateTaskCtx{workbenchTestCtx: workbenchTestCtx{store: store}, block: block}
	ctx.library = libraryFixture{localAuthors: []string{"库内作者名"}}
	m := NewSessionManagerWithStore(ctx, nil, store)
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: selectionTestRules(t)}
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	selectionSampleTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	ctx.fetcher = &LocalImportWorkFetcher{sessions: m, ctx: ctx, hostContractFn: func() uint32 { return createChildrenBatchMinHostContract }}
	return root, m, ctx
}

// collectWorkbenchEvents 收集工作台事件通道的全部事件（按发布序；ctx 为事件
// 记录桩或其包装桩）。
func collectWorkbenchEvents(t *testing.T, ctx workbenchEventReader) []workbenchFrontendEvent {
	t.Helper()
	var out []workbenchFrontendEvent
	for _, p := range ctx.publishedSnapshot() {
		if p.topic != workbenchTopicEvent {
			continue
		}
		var evt workbenchFrontendEvent
		if err := json.Unmarshal([]byte(p.data), &evt); err == nil {
			out = append(out, evt)
		}
	}
	return out
}

// TestApplyCreateTasksAsyncEventSequence createTasks 异步化全链事件序列（对齐
// 大载荷分页方案测试锚定 10）：立即应答 Started → createProgress 两相（核对
// 文件 X/N=逐作品哈希进度 → 创建子任务 X/N=发射循环计数）→ createDone（成功=
// 清空选择集+摘要文案；失败=保留选择集+部分完成信息「已创建 X/N 个子任务」）；
// 进行中重复 createTasks 以「创建进行中」拒绝、收尾后闸解除。
func TestApplyCreateTasksAsyncEventSequence(t *testing.T) {
	t.Run("成功全链清空选择集", func(t *testing.T) {
		root, m, ctx := recursiveCreateFixture(t, nil)
		if _, err := m.SetSelection(root, []string{"d:多轨", "d:已识别"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		started, err := m.ApplyCreateTasks(root, "异步批次")
		if err != nil || !started {
			t.Fatalf("创建编排应立即应答已开始创建: started=%v err=%v", started, err)
		}
		done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
		if !done.Success {
			t.Fatalf("宿主同步递归桩应成功收尾，实际: %s", done.Message)
		}
		if !strings.Contains(done.Message, "已创建任务「异步批次」：2 件作品，共 3 个子任务") {
			t.Fatalf("createDone 文案应携带真实产出计数（2 作品 3 子行），实际 %q", done.Message)
		}
		// 事件序列（阶段4 双计数模型）：createProgress(verify 0/2) 首反馈（甲1：
		// works 解析完成即发、真分母即刻可见）→ createProgress(verify 2/2) 终值
		//（恒发）→ createProgress(create 3/3)（单批不重叠；重叠期双计数锚定见
		// create_pipeline_test.go）→ createDone
		var firstVerifyAt, verifyFinalAt, createAt, doneAt = -1, -1, -1, -1
		var firstVerifyEvt, verifyFinalEvt, createEvt workbenchFrontendEvent
		for i, evt := range collectWorkbenchEvents(t, ctx) {
			switch {
			case evt.Kind == workbenchEventCreateProgress && evt.Phase == createPhaseVerify && firstVerifyAt < 0:
				firstVerifyAt, firstVerifyEvt = i, evt
			case evt.Kind == workbenchEventCreateProgress && evt.Phase == createPhaseVerify:
				verifyFinalAt, verifyFinalEvt = i, evt
			case evt.Kind == workbenchEventCreateProgress && evt.Phase == createPhaseCreate && createAt < 0:
				createAt, createEvt = i, evt
			case evt.Kind == workbenchEventCreateDone:
				doneAt = i
			}
		}
		if firstVerifyAt < 0 || firstVerifyEvt.Done != 0 || firstVerifyEvt.Total != 2 {
			t.Fatalf("首个 createProgress 应为核对相首反馈（0/2，先于一切发射相事件），实际 idx=%d %+v", firstVerifyAt, firstVerifyEvt)
		}
		if verifyFinalAt < 0 || verifyFinalEvt.Done != 2 || verifyFinalEvt.Total != 2 {
			t.Fatalf("核对相终值事件应在场且计数=已核对/全部作品（2/2），实际 idx=%d %+v", verifyFinalAt, verifyFinalEvt)
		}
		if createAt < 0 || createEvt.Done != 3 || createEvt.Total != 3 {
			t.Fatalf("创建相事件应在场且计数=已发射/计划子行（3/3），实际 idx=%d %+v", createAt, createEvt)
		}
		if createAt < firstVerifyAt || doneAt <= createAt {
			t.Fatalf("首反馈应先于发射相事件、createDone 应殿后，实际 first=%d create=%d done=%d", firstVerifyAt, createAt, doneAt)
		}
		sess, err := m.SessionOf(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		if len(sess.Selection) != 0 {
			t.Fatalf("成功终态应清空选择集，实际 %v", sess.Selection)
		}
	})

	t.Run("失败保留选择集并携带部分完成信息", func(t *testing.T) {
		root, m, ctx := recursiveCreateFixture(t, nil)
		ctx.afterEmitFail = true // 产出消费完毕后宿主回告失败（发射进度已落账 2/2）
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		if _, err := m.ApplyCreateTasks(root, ""); err != nil {
			t.Fatalf("创建编排失败: %v", err)
		}
		done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
		if done.Success {
			t.Fatalf("注入失败应回告失败终态，实际 %+v", done)
		}
		if !strings.Contains(done.Message, "已发射 2/2 个子任务随回滚删除") || !strings.Contains(done.Message, "选择集已保留") {
			t.Fatalf("失败终态应携带在途清理信息与选择集保留说明（决策4 统一回滚），实际 %q", done.Message)
		}
		if done.Done != 2 || done.Total != 2 {
			t.Fatalf("失败终态计数应镜像发射进度（2/2），实际 %+v", done)
		}
		sess, err := m.SessionOf(root)
		if err != nil {
			t.Fatalf("读取会话失败: %v", err)
		}
		if len(sess.Selection) != 1 {
			t.Fatalf("失败终态应保留选择集供重试，实际 %v", sess.Selection)
		}
	})

	t.Run("零产出传输失败走通用失败文案", func(t *testing.T) {
		root, m, ctx := recursiveCreateFixture(t, nil)
		ctx.transportFail = true
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		if _, err := m.ApplyCreateTasks(root, ""); err != nil {
			t.Fatalf("创建编排失败: %v", err)
		}
		done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
		if done.Success || !strings.Contains(done.Message, "任务创建失败") || !strings.Contains(done.Message, "宿主连接中断") {
			t.Fatalf("零产出失败应为通用失败文案（不含部分完成计数），实际 %q", done.Message)
		}
		if done.Done != 0 || done.Total != 0 {
			t.Fatalf("零产出失败终态计数应为 0/0，实际 %+v", done)
		}
	})

	t.Run("进行中重复发起以创建进行中拒绝", func(t *testing.T) {
		block := make(chan struct{})
		root, m, ctx := recursiveCreateFixture(t, block)
		if _, err := m.SetSelection(root, []string{"d:多轨"}); err != nil {
			t.Fatalf("写入选择集失败: %v", err)
		}
		if started, err := m.ApplyCreateTasks(root, ""); err != nil || !started {
			t.Fatalf("首次创建应立即应答已开始: started=%v err=%v", started, err)
		}
		// 创建挂在 block 上（RPC 未返回）期间重复发起 → 守卫链「创建进行中」拒绝
		if _, err := m.ApplyCreateTasks(root, ""); err == nil || !strings.Contains(err.Error(), "创建进行中") {
			t.Fatalf("进行中重复 createTasks 应被拒（创建进行中），实际 %v", err)
		}
		close(block)
		done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
		if !done.Success {
			t.Fatalf("放行后创建应成功收尾，实际 %q", done.Message)
		}
		// 收尾后运行态闸摘除：再次发起不再被「创建进行中」拒绝
		if _, err := m.SetSelection(root, []string{"d:已识别"}); err != nil {
			t.Fatalf("重建选择集失败: %v", err)
		}
		ctx.transportFail = true
		if _, err := m.ApplyCreateTasks(root, ""); err != nil {
			t.Fatalf("收尾后再次发起不应再被「创建进行中」拒绝: %v", err)
		}
		waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	})
}

// TestBuildCreateTasksMessageAttributionNote 通知文案的归属附注：无非 local 子行
// 无附注；有则追加「其中 n 件归属 站点名」（按归属分布逐站点一段、顿号连写，
// 多站点分别列出——文案按归属分布组织，与父行形态正交）。
func TestBuildCreateTasksMessageAttributionNote(t *testing.T) {
	pure := buildCreateTasksMessage("本地导入 · 库 · 3 件作品", 3, 3, 14*time.Minute, createBatchSummary{total: 3})
	if want := "已创建任务「本地导入 · 库 · 3 件作品」：3 件作品，共 3 个子任务，用时 14 分钟"; pure != want {
		t.Fatalf("纯 local 批次文案不追加附注，期望 %q 实际 %q", want, pure)
	}
	mixed := buildCreateTasksMessage("混归属批次", 5, 5, 14*time.Minute, createBatchSummary{
		total:          5,
		nonLocalBySite: []siteFileCount{{siteName: "pixiv", files: 2}},
	})
	if want := "已创建任务「混归属批次」：5 件作品，共 5 个子任务，用时 14 分钟；其中 2 件归属 pixiv"; mixed != want {
		t.Fatalf("混归属批次文案应追加归属附注，期望 %q 实际 %q", want, mixed)
	}
	multiSite := buildCreateTasksMessage("多站点批次", 4, 4, 14*time.Minute, createBatchSummary{
		total:          4,
		nonLocalBySite: []siteFileCount{{siteName: "pixiv", files: 2}, {siteName: "bilibili", files: 1}},
	})
	if want := "已创建任务「多站点批次」：4 件作品，共 4 个子任务，用时 14 分钟；其中 2 件归属 pixiv、1 件归属 bilibili"; multiSite != want {
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
// setSelection 落库并回告选择集轻量回执（decisions 全量投影已从变更类动作
// 应答剥离——大载荷断路器收口）；createTasks 携可选 taskName 走异步编排——
// 立即应答 started=true（轻量回执仍携带在选集，创建挂在桩闸上），终态经
// createDone 事件（任务名生效、选择集随成功清空）；未知动作回告错误。
func TestWorkbenchSelectionActionsDispatch(t *testing.T) {
	root, m, ctx := recognizedSelectionFixture(t, nil)
	// 桩闸：立即应答断言前挂起创建 goroutine（时序确定性）
	ctx.mu.Lock()
	ctx.createTaskBlock = make(chan struct{})
	ctx.mu.Unlock()

	m.HandleFrontendRequest([]byte(`{"seq":41,"action":"setSelection","root":` + jsonQuote(root) + `,"works":["d:多轨","d:已识别"]}`))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Decisions != nil || len(resp.Selection) != 2 {
		t.Fatalf("setSelection 应回告选择集轻量回执（decisions 应剥离），实际 %+v", resp)
	}

	m.HandleFrontendRequest([]byte(`{"seq":42,"action":"createTasks","root":` + jsonQuote(root) + `,"taskName":"命名批次"}`))
	resp = ctx.lastResponse(t)
	if !resp.OK || !resp.Started {
		t.Fatalf("createTasks 应回告已开始创建（started），实际 %+v", resp)
	}
	// 立即应答时创建在途：轻量回执的选择集仍在场（清空随 createDone 成功态发生）
	if resp.Decisions != nil || len(resp.Selection) != 2 {
		t.Fatalf("createTasks 立即应答应携带在途选择集轻量回执，实际 selection=%v", resp.Selection)
	}

	// 放行桩闸 → 创建收尾：createDone 成功携带生效任务名，选择集随之清空
	ctx.mu.Lock()
	close(ctx.createTaskBlock)
	ctx.mu.Unlock()
	done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	if !done.Success || !strings.Contains(done.Message, "已创建任务「命名批次」") {
		t.Fatalf("createDone 应为成功终态并携带生效任务名（taskName 载荷透传），实际 %+v", done)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if len(sess.Selection) != 0 {
		t.Fatalf("createDone 成功后选择集应清理，实际 %v", sess.Selection)
	}

	m.HandleFrontendRequest([]byte(`{"seq":43,"action":"setSelection","root":` + jsonQuote(root) + `,"works":["d:未识别"]}`))
	resp = ctx.lastResponse(t)
	if resp.OK || !strings.Contains(resp.Error, "不可选") {
		t.Fatalf("不可选作品的 setSelection 应回告错误，实际 %+v", resp)
	}
}

// TestWorkbenchSelectionActionReplyBound 选择族动作应答尺寸界（延伸收口）：
// 用户真实环境 decisions 67MB 级（长期目录赋值逐文件合并累计，2026-10-31
// 实测 67,321,044 字节）下，变更类动作应答曾整答全量决策态——动作已生效而
// 应答撞 64MB gRPC 断路器丢失、UI 误报失败。收口后断言：67MB 级 decisions
// 夹具下 setSelection/createTasks 立即应答 json.Marshal 实测 ≤ 软界
// panoramaPageSoftBytes、Decisions 恒不随答、选择集轻量回执与会话真值全等。
func TestWorkbenchSelectionActionReplyBound(t *testing.T) {
	// 118k 条 × ~583B ≈ 68MB 级（对齐用户真实 67,321,044 字节量级）
	const assignTotal = 118000
	assigns := make(map[string]SessionAssignment, assignTotal)
	for i := 0; i < assignTotal; i++ {
		rel := fmt.Sprintf("库根目录甲/子目录乙/作品集丙长名占位段/卷次目录_%03d/文件名前段占位文段甲乙丙丁_%06d.jpg", i%40, i)
		assigns[rel] = SessionAssignment{
			Author:     fmt.Sprintf("作者名占位文段甲乙丙丁_%06d", i),
			AuthorRole: "画师",
			Title:      fmt.Sprintf("标题文段占位甲乙丙丁戊己庚辛_%06d_尾注文段", i),
			Tags: []string{
				fmt.Sprintf("标签甲乙丙_%06d", i), "公共标签占位文段一",
				fmt.Sprintf("标签丙丁戊_%06d", i%97), "公共标签占位文段二",
				fmt.Sprintf("标签己庚辛_%06d", i%31),
			},
			Collection:       fmt.Sprintf("集名占位文段甲乙丙_%06d", i%53),
			AuthorClueRef:    "dirLevelStripped[1]",
			CollectionClueRef: "filenameSeg[0]",
			UpdatedAtMs:      1700000000000 + int64(i),
		}
	}
	root, m, ctx := recognizedSelectionFixture(t, assigns)
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	decJSON, err := json.Marshal(decisionsViewOf(sess))
	if err != nil {
		t.Fatalf("全量决策态序列化失败: %v", err)
	}
	if len(decJSON) < 64<<20 {
		t.Fatalf("夹具量级不足：全量决策态 %d 字节 < 64MB（%d 条赋值）", len(decJSON), assignTotal)
	}
	// 桩闸：立即应答断言前挂起创建 goroutine（时序确定性——选择集清空只随
	// createDone 成功态发生）
	ctx.mu.Lock()
	ctx.createTaskBlock = make(chan struct{})
	ctx.mu.Unlock()

	works := []string{"d:多轨", "d:已识别"}
	setSelMeasured := 0
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 51, Action: workbenchActionSetSelection, Root: root, Works: works,
	}))
	resp := ctx.lastResponse(t)
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("setSelection 应答序列化失败: %v", err)
	}
	setSelMeasured = len(data)
	if !resp.OK || resp.Decisions != nil {
		t.Fatalf("setSelection 应答不符（decisions 应剥离）: %+v", resp)
	}
	if len(data) > panoramaPageSoftBytes {
		t.Fatalf("setSelection 应答实测 %d 字节超软界 %d（decisions=%d 字节）", len(data), panoramaPageSoftBytes, len(decJSON))
	}
	if !reflect.DeepEqual(resp.Selection, works) {
		t.Fatalf("setSelection 轻量回执与会话真值不全等: %v vs %v", resp.Selection, works)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 52, Action: workbenchActionCreateTasks, Root: root, TaskName: "大载荷界批次",
	}))
	resp = ctx.lastResponse(t)
	data, err = json.Marshal(resp)
	if err != nil {
		t.Fatalf("createTasks 应答序列化失败: %v", err)
	}
	if !resp.OK || !resp.Started || resp.Decisions != nil {
		t.Fatalf("createTasks 应答不符（decisions 应剥离）: %+v", resp)
	}
	if len(data) > panoramaPageSoftBytes {
		t.Fatalf("createTasks 应答实测 %d 字节超软界 %d（decisions=%d 字节）", len(data), panoramaPageSoftBytes, len(decJSON))
	}
	if !reflect.DeepEqual(resp.Selection, works) {
		t.Fatalf("createTasks 立即应答轻量回执与在途选择集不全等: %v vs %v", resp.Selection, works)
	}

	// 放行桩闸收尾（后台创建 goroutine 收流终态，防测试泄漏挂起）
	ctx.mu.Lock()
	close(ctx.createTaskBlock)
	ctx.mu.Unlock()
	if done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone); !done.Success {
		t.Fatalf("createDone 应为成功终态，实际 %+v", done)
	}
	t.Logf("decisions=%d 字节夹具下 setSelection/createTasks 立即应答实测 %d / %d 字节",
		len(decJSON), setSelMeasured, len(data))
}

// jsonQuote 测试内 JSON 字符串转义。
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// —— 宿主创建流提前终结的撤回链（终态后核对池随撤 + 帧5 计数定格）—— //

// streamDeathCreateTaskCtx CreateTask 桩模拟宿主创建流提前终结：同步驱动插件
// Create 过滤产出分支取得条目流后不消费任何条目（宿主泵建流即收流的形态——
// 首块前或批间隙空闲收流），回告失败。发射通道 16 缓冲填满后释放环阻塞在通道
// 发送处，撤回链经取消解除阻塞收流。
type streamDeathCreateTaskCtx struct {
	workbenchTestCtx
	fetcher *LocalImportWorkFetcher
}

// CreateTask 宿主泵收流桩：等发射通道缓冲填满（释放环阻塞在通道发送处、进度槽
// 已落账 16 批）后不消费即回告空闲收流失败。
func (c *streamDeathCreateTaskCtx) CreateTask(url string) (*sdkdto.CreateTaskResult, error) {
	result, err := c.fetcher.Create(url)
	if err != nil {
		return nil, err
	}
	items := result.Items()
	if items == nil {
		return nil, fmt.Errorf("过滤产出应为条目形态流式结果（B1+）")
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(items) < 16 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if len(items) < 16 {
		return nil, fmt.Errorf("发射通道未按预期填满（len=%d）——夹具失效", len(items))
	}
	return &sdkdto.CreateTaskResult{Succeed: false, Msg: "插件流空闲超时: 1m0s 内未收到数据（测试注入）"}, nil
}

// streamDeathCreateFixture 宿主流断链测试环境：单目录 25 轨聚合作品（全量识别后
// 可选，选择集=该作品 → 25 子行），fetcher 单批条数界 1（每子行一批——发射通道
// 16 缓冲必然填满、释放环阻塞在通道发送处）。
func streamDeathCreateFixture(t *testing.T, tracks int) (string, *SessionManager, *streamDeathCreateTaskCtx) {
	t.Helper()
	root := t.TempDir()
	store := newFakeSessionStore()
	ctx := &streamDeathCreateTaskCtx{workbenchTestCtx: workbenchTestCtx{store: store}}
	ctx.library = libraryFixture{localAuthors: []string{"库内作者名"}}
	m := NewSessionManagerWithStore(ctx, nil, store)
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: selectionTestRules(t)}
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("写入测试规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	for i := 0; i < tracks; i++ {
		writeTestFile(t, fmt.Sprintf("%s/聚合目录/【作者甲】标题%02d.jpg", root, i), fmt.Sprintf("内容%02d", i))
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	ctx.fetcher = &LocalImportWorkFetcher{
		sessions: m, ctx: ctx,
		hostContractFn:          func() uint32 { return createChildrenBatchMinHostContract },
		childrenBatchCountLimit: 1,
	}
	return root, m, ctx
}

// TestStreamDeathHaltsEmissionAndFreezesCount 宿主创建流提前终结后编排侧撤回在途
// 核对与发射：createDone 失败终态（帧5）的已发射计数=撤回时点真实已发射数（发射
// 通道缓冲容纳的 16 子行，计划 25），终态后无残余 createProgress 事件，哈希缓存
// 按中止合并语义落盘（发射 goroutine 退出路径的 defer 已执行——撤回前阻塞在通道
// 发送处的形态永不落盘）。
func TestStreamDeathHaltsEmissionAndFreezesCount(t *testing.T) {
	root, m, ctx := streamDeathCreateFixture(t, 25)
	if _, err := m.SetSelection(root, []string{"d:聚合目录"}); err != nil {
		t.Fatalf("写入选择集失败: %v", err)
	}
	started, err := m.ApplyCreateTasks(root, "流断批次")
	if err != nil || !started {
		t.Fatalf("创建编排应立即应答已开始: started=%v err=%v", started, err)
	}
	done := waitForWorkbenchEvent(t, ctx, workbenchEventCreateDone)
	if done.Success {
		t.Fatalf("宿主收流桩应失败收尾，实际成功: %s", done.Message)
	}
	if !strings.Contains(done.Message, "宿主创建任务失败: 插件流空闲超时") {
		t.Fatalf("帧5 文案应携带宿主收流原因，实际 %q", done.Message)
	}
	if !strings.Contains(done.Message, "已发射 16/25 个子任务随回滚删除") {
		t.Fatalf("帧5 计数应为撤回时点真实已发射数（通道缓冲 16 子行/计划 25），实际 %q", done.Message)
	}
	events := collectWorkbenchEvents(t, ctx)
	for i, evt := range events {
		if evt.Kind == workbenchEventCreateDone && i != len(events)-1 {
			t.Fatalf("createDone 后不应再有创建事件（撤回后核对/发射停摆），残余: %+v", events[i+1:])
		}
	}
	if _, err := ctx.store.GetValue(hashCacheKey(root)); err != nil {
		t.Fatalf("哈希缓存应随发射侧收尾按中止合并语义落盘: %v", err)
	}
}
