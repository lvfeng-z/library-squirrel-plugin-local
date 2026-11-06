// 工作台会话管理器的单元测试：KV 键格式与归一、多会话并存与同库根唯一、
// 跨实例（重启）留存往返、状态机迁移判定、档案写三入口并发互斥、档案外部
// 变更对账、工作台发起创建标记的生命周期，以及前端事件协议的请求分派。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// fakeSessionStore 为内存会话存储：实现会话读写所需的全部四个 KV 方法，
// 键不存在时与宿主一致地返回零值存储值而非错误。
type fakeSessionStore struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{values: make(map[string]string)}
}

func (s *fakeSessionStore) GetValue(key string) (*sdkdto.StorageValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return &sdkdto.StorageValue{}, nil
	}
	return &sdkdto.StorageValue{Value: value, SchemaVersion: workbenchSessionSchemaVersion}, nil
}

func (s *fakeSessionStore) SetValue(key string, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *fakeSessionStore) DeleteValue(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

func (s *fakeSessionStore) GetAllValues() (map[string]*sdkdto.StorageValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]*sdkdto.StorageValue, len(s.values))
	for key, value := range s.values {
		out[key] = &sdkdto.StorageValue{Value: value, SchemaVersion: workbenchSessionSchemaVersion}
	}
	return out, nil
}

// publishedEvent 记录一次前端事件发布。
type publishedEvent struct {
	topic string
	data  string
}

// Infof 观测日志桩（CreateWorkInfo 产出观测等合法调用面，非测试缺口）。
func (c *workbenchTestCtx) Infof(string, ...any) {}

// Warnf 告警桩（爬坡控制器非法偏好回退等路径经 h.ctx.Warnf 上报——内嵌接口
// 零值不可解引用，测试桩必须显式提供；静默丢弃即可，断言面在专用桩 warnCountingCtx）。
func (c *workbenchTestCtx) Warnf(string, ...any) {}

// workbenchTestCtx 工作台测试用的插件上下文桩：KV 走内存存储、前端事件发布被
// 记录供断言；其余方法一旦被调用即 panic 暴露测试缺口。
type workbenchTestCtx struct {
	sdkdto.PluginContext
	store *fakeSessionStore

	// library 谓词索引构造的分页数据集（Query* 五方法的桩数据；零值=五轨全空）。
	library libraryFixture
	// workSetIDSeq QueryLocalWorkSets 桩的行 id 发号器（跨页单调递增）；
	// atomic：ApplyAssignment 异步识别刷新与测试自驱识别可并发拉取，
	// 无锁自增曾致 -race 偶发（2026-10-30 AJ 阶段5 归因，pristine 树复现）。
	workSetIDSeq atomic.Int64

	// CreateTask 桩：createTaskStub 为应答（nil=默认成功 AddedQuantity=3），
	// 计数与末次 URL 供创建编排断言。
	createTaskStub    *sdkdto.CreateTaskResult
	createTaskCount   int
	createTaskLastURL string
	// createTaskBlock 可选：CreateTask 桩入口挂起闸（close 放行——createTasks
	// 异步化的立即应答与终态时序控制）。
	createTaskBlock chan struct{}

	// prefs 偏好桩数据（键 → 偏好值；爬坡控制器偏好钉死层注入面）。
	prefs map[string]*sdkdto.PreferenceValue

	// pluginRootDir GetPluginRoot 桩应答（求值器构建标识模拟面）：空串=根路径
	// 未配置（未打标语义，识别增量闸门与恢复对账按缺席放行）；非空=指向测试
	// 预置的插件根目录（内含带 buildId 的 plugin.json，模拟打标构建）。
	pluginRootDir string

	mu        sync.Mutex
	published []publishedEvent
}

// CreateTask 创建编排动作（ApplyCreateTasks）的桩：记录调用并回告预设结论
// （真实 Create 过滤产出分支由测试直驱，不经此桩）；createTaskBlock 在场时先
// 挂起直到闸放行（createTasks 异步化的立即应答与终态时序控制）。
func (c *workbenchTestCtx) CreateTask(url string) (*sdkdto.CreateTaskResult, error) {
	c.mu.Lock()
	block := c.createTaskBlock
	c.mu.Unlock()
	if block != nil {
		<-block
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createTaskCount++
	c.createTaskLastURL = url
	if c.createTaskStub != nil {
		return c.createTaskStub, nil
	}
	return &sdkdto.CreateTaskResult{Succeed: true, AddedQuantity: 3}, nil
}

// GetPreference 偏好读取桩：prefs 预置时按键回告（爬坡控制器偏好钉死层的注入
// 面，create_pipeline_test.go 消费），未预置=无记录；错误路径恒无。
func (c *workbenchTestCtx) GetPreference(key string) (*sdkdto.PreferenceValue, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.prefs == nil {
		return nil, false, nil
	}
	value, ok := c.prefs[key]
	return value, ok, nil
}

func (c *workbenchTestCtx) GetValue(key string) (*sdkdto.StorageValue, error) {
	return c.store.GetValue(key)
}

func (c *workbenchTestCtx) SetValue(key string, value string) error {
	return c.store.SetValue(key, value)
}

func (c *workbenchTestCtx) DeleteValue(key string) error {
	return c.store.DeleteValue(key)
}

func (c *workbenchTestCtx) GetAllValues() (map[string]*sdkdto.StorageValue, error) {
	return c.store.GetAllValues()
}

func (c *workbenchTestCtx) PublishToFrontend(topic string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.published = append(c.published, publishedEvent{topic: topic, data: string(data)})
	return nil
}

// publishedSnapshot 事件发布记录快照（事件等待/收集助手的数据面）。
func (c *workbenchTestCtx) publishedSnapshot() []publishedEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]publishedEvent(nil), c.published...)
}

// workbenchEventReader 工作台事件读取的最小面（workbenchTestCtx 及其包装桩
// ——宿主同步递归桩等——经嵌入提升实现）。
type workbenchEventReader interface {
	publishedSnapshot() []publishedEvent
}

// GetWorkDir 库根未配置形态：返回错误（会话管理器按未配置处理，默认库根缺席）。
func (c *workbenchTestCtx) GetWorkDir() (*sdkdto.GetWorkDirResponse, error) {
	return nil, errors.New("库根未配置（测试桩）")
}

// GetPluginRoot 插件根桩：原样回告 pluginRootDir（isRelative 形态不区分——
// 求值器构建标识读取只走绝对路径取值）。
func (c *workbenchTestCtx) GetPluginRoot(isRelative bool) string {
	return c.pluginRootDir
}

// lastResponse 取最近一条工作台应答并解出载荷。
func (c *workbenchTestCtx) lastResponse(t *testing.T) workbenchFrontendResponse {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.published) - 1; i >= 0; i-- {
		if c.published[i].topic != workbenchTopicResponse {
			continue
		}
		var resp workbenchFrontendResponse
		if err := json.Unmarshal([]byte(c.published[i].data), &resp); err != nil {
			t.Fatalf("解析工作台应答失败: %v", err)
		}
		return resp
	}
	t.Fatalf("没有任何工作台应答发布")
	return workbenchFrontendResponse{}
}

// newTestManager 构造接在内存存储上的会话管理器（同一存储可再建新管理器实例，
// 模拟插件进程重启后的会话重建）。
func newTestManager(t *testing.T, store *fakeSessionStore) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	ctx := &workbenchTestCtx{store: store}
	return NewSessionManagerWithStore(ctx, nil, store), ctx
}

// TestWorkbenchSessionKeyFormat 会话 KV 键格式与库根归一：键 = 前缀 + 库根标识，
// 同一库根的不同书写形态（大小写/斜杠方向/尾斜杠/前后空白）归一到同一键。
func TestWorkbenchSessionKeyFormat(t *testing.T) {
	if got, want := workbenchSessionKey(`E:\Media\Lib\`), "workbench-session:e:/media/lib"; got != want {
		t.Fatalf("会话键 = %q, 期望 %q", got, want)
	}
	same := []string{`E:\Media\Lib`, "e:/media/lib", ` e:\MEDIA\lib `, `E:\media\lib\`}
	for _, form := range same {
		if key := workbenchSessionKey(form); key != workbenchSessionKey(`E:\Media\Lib`) {
			t.Fatalf("库根 %q 应归一到同一键，实际 %q", form, key)
		}
	}
}

// TestSessionMultiCoexistAndSameRootUnique 多会话并存、同库根唯一、跨实例留存：
// 两个库根会话并存且互不覆盖；同库根再建返回 ErrWorkbenchSessionExists；新管理器
// 实例（模拟重启）从同一 KV 枚举恢复全部会话，工件结构版本自管。
func TestSessionMultiCoexistAndSameRootUnique(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	if _, err := mgr.CreateSession(`D:\库甲`); err != nil {
		t.Fatalf("建立会话甲失败: %v", err)
	}
	if _, err := mgr.CreateSession(`D:\库乙`); err != nil {
		t.Fatalf("建立会话乙失败: %v", err)
	}
	if _, err := mgr.CreateSession(`d:/库甲/`); !errors.Is(err, ErrWorkbenchSessionExists) {
		t.Fatalf("同库根再建应返回会话已存在错误，实际: %v", err)
	}

	// 跨实例（重启）留存：同一存储上重建管理器
	mgr2, _ := newTestManager(t, store)
	sessions, err := mgr2.ListSessions()
	if err != nil {
		t.Fatalf("重启后枚举会话失败: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("重启后应恢复 2 个会话，实际 %d 个: %+v", len(sessions), sessions)
	}
	for _, sess := range sessions {
		if sess.SchemaVersion != workbenchSessionSchemaVersion {
			t.Fatalf("会话工件结构版本应为 %d（自管），实际 %d", workbenchSessionSchemaVersion, sess.SchemaVersion)
		}
		if sess.State != WorkbenchSessionCreated {
			t.Fatalf("新会话状态应为 created，实际 %s", sess.State)
		}
	}
	if _, err := mgr2.SessionOf(`D:\库甲`); err != nil {
		t.Fatalf("重启后按库根取会话失败: %v", err)
	}

	// 删除其一：另一会话不受影响
	if err := mgr2.DeleteSession(`D:\库甲`); err != nil {
		t.Fatalf("删除会话失败: %v", err)
	}
	rest, err := mgr2.ListSessions()
	if err != nil {
		t.Fatalf("删除后枚举会话失败: %v", err)
	}
	if len(rest) != 1 || rest[0].RootPath != `D:\库乙` {
		t.Fatalf("删除后应仅余库乙会话，实际: %+v", rest)
	}
}

// TestSessionReplaceOnConfirm 覆盖重建：确认后原库根重建，会话身份刷新（既有态不保留）。
func TestSessionReplaceOnConfirm(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	first, err := mgr.CreateSession(`D:\库丙`)
	if err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	second, err := mgr.ReplaceSession(`d:\库丙`)
	if err != nil {
		t.Fatalf("覆盖重建失败: %v", err)
	}
	if second.CreatedAtMs < first.CreatedAtMs {
		t.Fatalf("覆盖重建应产生新会话（创建时间刷新）")
	}
	sessions, err := mgr.ListSessions()
	if err != nil {
		t.Fatalf("枚举会话失败: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("覆盖重建后该库根应恰一个会话，实际 %d 个", len(sessions))
	}
}

// TestSessionStateMachine 状态机迁移判定：created→recognized 为唯一前向合法迁移，
// 自迁移/回退非法；迁移经会话管理器落库生效。
func TestSessionStateMachine(t *testing.T) {
	if !legalSessionTransition(WorkbenchSessionCreated, WorkbenchSessionRecognized) {
		t.Fatalf("created→recognized 应为合法迁移")
	}
	for _, pair := range [][2]WorkbenchSessionState{
		{WorkbenchSessionRecognized, WorkbenchSessionRecognized},
		{WorkbenchSessionRecognized, WorkbenchSessionCreated},
		{WorkbenchSessionCreated, WorkbenchSessionCreated},
	} {
		if legalSessionTransition(pair[0], pair[1]) {
			t.Fatalf("迁移 %s→%s 应为非法", pair[0], pair[1])
		}
	}

	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	if _, err := mgr.CreateSession(`D:\库丁`); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	sess, err := mgr.MarkRecognized(`D:\库丁`)
	if err != nil {
		t.Fatalf("created→recognized 迁移失败: %v", err)
	}
	if sess.State != WorkbenchSessionRecognized {
		t.Fatalf("迁移后状态应为 recognized，实际 %s", sess.State)
	}
	if _, err := mgr.MarkRecognized(`D:\库丁`); err == nil {
		t.Fatalf("recognized 再迁移应报错（状态机前向唯一）")
	}
	if _, err := mgr.MarkRecognized(`D:\不存在`); !errors.Is(err, ErrWorkbenchSessionNotFound) {
		t.Fatalf("不存在的会话迁移应报未命中，实际: %v", err)
	}
}

// countingStubReporter 计数心跳上报（Create 流等待期心跳观测桩）。
type countingStubReporter struct {
	count atomic.Int64
}

func (r *countingStubReporter) Heartbeat() {
	r.count.Add(1)
}

// testMarkerRule 档案写入口测试的标记规则：可序列化可载入的最小合法规则。
func testMarkerRule(t RuleSource, marker string) (RuleEntry, error) {
	pattern, err := NewRegexPattern(string(TargetFileName), "^"+marker+`$`, nil)
	if err != nil {
		return RuleEntry{}, err
	}
	return RuleEntry{
		ID:          "mark-" + marker,
		Description: "档案写入口标记规则 " + marker,
		Source:      t,
		Enabled:     true,
		Pattern:     pattern,
		Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromConstant, Constant: marker}},
	}, nil
}

// TestArchiveWriteThreeEntriesSerialized 档案写三入口（泛化/裁决生成/规则编辑面）
// 并发互斥：真实档案写入口与串行 API 并发写入时（变更体带在途观测），任意时刻
// 至多一个档案变更在途，全部标记规则落库无丢失。
func TestArchiveWriteThreeEntriesSerialized(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	var inFlight, maxInFlight atomic.Int32
	gate := make(chan struct{})
	// 三真实写入口：泛化/裁决生成入口经 UpdateProfile 携对应入口标识（真实
	// 事务 ApplyGeneralization / ApplyAdjudication 需会话与快照上下文，此处
	// 以同一串行入口承载入口标识语义）；规则编辑面走真实事务 ApplySaveRule
	// 全管线（解析→构造期校验→入档）。
	markerRuleJSON := func(source RuleSource, marker string) ([]byte, error) {
		rule, err := testMarkerRule(source, marker)
		if err != nil {
			return nil, err
		}
		rj, err := ruleToJSON(rule)
		if err != nil {
			return nil, err
		}
		return json.Marshal(rj)
	}
	entryFuncs := []func(string) error{
		func(marker string) error {
			rule, err := testMarkerRule(RuleSourceQna, marker)
			if err != nil {
				return err
			}
			return mgr.UpdateProfile(ArchiveWriteGeneralize, func(p *RuleProfile) error {
				p.Rules = append(p.Rules, rule)
				return nil
			})
		},
		func(marker string) error {
			rule, err := testMarkerRule(RuleSourceAdjudication, marker)
			if err != nil {
				return err
			}
			return mgr.UpdateProfile(ArchiveWriteAdjudicate, func(p *RuleProfile) error {
				p.Rules = append(p.Rules, rule)
				return nil
			})
		},
		func(marker string) error {
			data, err := markerRuleJSON(RuleSourceManual, marker)
			if err != nil {
				return err
			}
			_, err = mgr.ApplySaveRule(data)
			return err
		},
	}
	const stubWritesPerEntry = 4
	const probeWriters = 6
	var wg sync.WaitGroup

	// 三入口并发写（各自标记规则落库）
	for entryIdx, entry := range entryFuncs {
		for i := 0; i < stubWritesPerEntry; i++ {
			wg.Add(1)
			go func(entryIdx, i int, entry func(string) error) {
				defer wg.Done()
				<-gate
				if err := entry(fmtMarker(entryIdx, i)); err != nil {
					t.Errorf("入口 %d 并发写失败: %v", entryIdx, err)
				}
			}(entryIdx, i, entry)
		}
	}

	// 串行 API 直连探测写：变更体内观测在途峰值——未持锁时 sleep 窗口内并发
	// 变更体必然重叠（峰值 >1），持锁时恒为 1
	for i := 0; i < probeWriters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			marker := fmtMarker(len(entryFuncs), i)
			err := mgr.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error {
				cur := inFlight.Add(1)
				for {
					old := maxInFlight.Load()
					if cur <= old || maxInFlight.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				inFlight.Add(-1)
				rule, err := testMarkerRule(RuleSourceManual, marker)
				if err != nil {
					return err
				}
				p.Rules = append(p.Rules, rule)
				return nil
			})
			if err != nil {
				t.Errorf("串行 API 探测写失败: %v", err)
			}
		}(i)
	}
	close(gate)
	wg.Wait()

	if got := maxInFlight.Load(); got != 1 {
		t.Fatalf("档案写并发互斥被破坏：同时在途峰值 = %d，期望恒为 1", got)
	}

	profile, errs := LoadProfile(store)
	if len(errs) != 0 {
		t.Fatalf("并发写后档案载入不应报错: %v", errs)
	}
	total := stubWritesPerEntry*len(entryFuncs) + probeWriters
	if len(profile.Rules) != total {
		t.Fatalf("并发写应无丢失：期望 %d 条标记规则，实际 %d 条", total, len(profile.Rules))
	}
	seen := make(map[string]bool, len(profile.Rules))
	for _, rule := range profile.Rules {
		seen[strings.TrimPrefix(rule.ID, "mark-")] = true
	}
	for entryIdx := 0; entryIdx <= len(entryFuncs); entryIdx++ {
		n := stubWritesPerEntry
		if entryIdx == len(entryFuncs) {
			n = probeWriters
		}
		for i := 0; i < n; i++ {
			if !seen[fmtMarker(entryIdx, i)] {
				t.Fatalf("标记规则 %s 缺失（写丢失）", fmtMarker(entryIdx, i))
			}
		}
	}
}

// fmtMarker 构造标记规则的稳定标记文本。
func fmtMarker(entryIdx, i int) string {
	return "w" + itoa(entryIdx) + "-" + itoa(i)
}

// TestProfileReconcileOnActivate 档案外部变更对账：会话建立后档案在会话管理器外
// 被覆写（停库直写形态），恢复会话时对账检出变更；未变更时无提示。
func TestProfileReconcileOnActivate(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	if _, err := mgr.CreateSession(`D:\库戊`); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}

	// 未变更：对账无提示
	_, reconcile, err := mgr.ActivateSession(`D:\库戊`)
	if err != nil {
		t.Fatalf("恢复会话失败: %v", err)
	}
	if reconcile.Changed {
		t.Fatalf("档案未变更时不应提示重新识别")
	}

	// 外部变更：绕过会话管理器直写档案（停库直写形态）
	profile, errs := LoadProfile(store)
	if len(errs) != 0 || profile == nil {
		t.Fatalf("档案载入异常: %v %+v", errs, profile)
	}
	rule, err := testMarkerRule(RuleSourceManual, "external-direct-write")
	if err != nil {
		t.Fatalf("构造外部变更规则失败: %v", err)
	}
	profile.Rules = append(profile.Rules, rule)
	if err := SaveProfile(store, profile); err != nil {
		t.Fatalf("外部覆写档案失败: %v", err)
	}

	_, reconcile, err = mgr.ActivateSession(`D:\库戊`)
	if err != nil {
		t.Fatalf("恢复会话失败: %v", err)
	}
	if !reconcile.Changed || !strings.Contains(reconcile.Message, "重新识别") {
		t.Fatalf("档案被外部变更后恢复会话应提示重新识别，实际: %+v", reconcile)
	}
}

// TestEvaluatorReconcileOnActivate 求值器构建标识对账：快照铸于另一构建标识的
// 求值器（识别内核换版）时恢复会话提示重识别；同一标识不提示；当前未打标
//（空串）放行；旧快照未带标识（空串）而当前有值（存量库升级）提示。
func TestEvaluatorReconcileOnActivate(t *testing.T) {
	store := newFakeSessionStore()
	mgr, ctx := newTestManager(t, store)

	if _, err := mgr.CreateSession(`D:\库庚`); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	stampEvaluatorBuild(t, ctx, "build-a")
	if _, err := mgr.CompleteRecognitionWithDiff(`D:\库庚`, &RecognitionSnapshot{EvaluatorBuildID: "build-a"}, nil); err != nil {
		t.Fatalf("识别落库失败: %v", err)
	}

	// 同一标识：无提示
	_, reconcile, err := mgr.ActivateSession(`D:\库庚`)
	if err != nil {
		t.Fatalf("恢复会话失败: %v", err)
	}
	if reconcile.Changed {
		t.Fatalf("求值器标识一致时不应提示重新识别，实际: %+v", reconcile)
	}

	// 求值器换版：提示重识别
	stampEvaluatorBuild(t, ctx, "build-b")
	_, reconcile, err = mgr.ActivateSession(`D:\库庚`)
	if err != nil {
		t.Fatalf("恢复会话失败: %v", err)
	}
	if !reconcile.Changed || !strings.Contains(reconcile.Message, "重新识别") {
		t.Fatalf("求值器换版后恢复会话应提示重新识别，实际: %+v", reconcile)
	}

	// 当前未打标（空串）：缺席放行，不提示
	ctx.pluginRootDir = ""
	_, reconcile, err = mgr.ActivateSession(`D:\库庚`)
	if err != nil {
		t.Fatalf("恢复会话失败: %v", err)
	}
	if reconcile.Changed {
		t.Fatalf("当前构建未打标时应放行不提示，实际: %+v", reconcile)
	}

	// 存量库升级：旧快照未带标识（空串）而当前有值 → 提示
	stampEvaluatorBuild(t, ctx, "build-c")
	if _, err := mgr.CompleteRecognitionWithDiff(`D:\库庚`, &RecognitionSnapshot{EvaluatorBuildID: ""}, nil); err != nil {
		t.Fatalf("识别落库失败: %v", err)
	}
	_, reconcile, err = mgr.ActivateSession(`D:\库庚`)
	if err != nil {
		t.Fatalf("恢复会话失败: %v", err)
	}
	if !reconcile.Changed || !strings.Contains(reconcile.Message, "重新识别") {
		t.Fatalf("旧快照空串而当前有值应提示重新识别，实际: %+v", reconcile)
	}
}

// TestWorkbenchCreateMarkerLifecycle 工作台发起创建标记：置位后恰可消费一次，
// 未置位的库根消费返回假（外部创建不进过滤分支）。
func TestWorkbenchCreateMarkerLifecycle(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	if _, err := mgr.CreateSession(`D:\库己`); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if mgr.ConsumeWorkbenchCreateMarker(`D:\库己`) {
		t.Fatalf("未置位标记的消费应返回假")
	}
	if err := mgr.BeginWorkbenchCreate(`D:\库己`); err != nil {
		t.Fatalf("置位标记失败: %v", err)
	}
	if !mgr.ConsumeWorkbenchCreateMarker(`D:\库己`) {
		t.Fatalf("已置位标记的首度消费应返回真")
	}
	if mgr.ConsumeWorkbenchCreateMarker(`D:\库己`) {
		t.Fatalf("同一标记不应被重复消费")
	}
}

// TestWorkbenchCreateMarkerLegacyMigration 标记旧字段迁移消费（阶段3 退出标准②，
// 方案风险4）：外置单键缺席而旧会话信封字段读到 true 时，消费恰迁移一轮——
// 首度消费命中并清除旧字段（此后恒走外置键），再度消费不再命中；旧格式整值
// 会话（信封化之前）同口径迁移；新置位（BeginWorkbenchCreate）只写外置键、
// 不触旧信封字段。
func TestWorkbenchCreateMarkerLegacyMigration(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	// 信封会话的旧字段迁移：外置化升级前置位的标记（信封字段=true，外置键缺席）
	root := `D:\库辛`
	if _, err := mgr.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if _, err := mgr.mutateSession(root, func(sess *WorkbenchSession) error {
		sess.PendingWorkbenchCreate = true // 旧形态注入：模拟外置化前版本置位
		return nil
	}); err != nil {
		t.Fatalf("注入旧信封标记失败: %v", err)
	}
	if !mgr.ConsumeWorkbenchCreateMarker(root) {
		t.Fatalf("旧信封字段读到 true 应被迁移消费一轮")
	}
	sess, err := mgr.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if sess.PendingWorkbenchCreate {
		t.Fatalf("迁移消费后旧信封字段应清除")
	}
	if mgr.ConsumeWorkbenchCreateMarker(root) {
		t.Fatalf("迁移消费恰一轮，再度消费不应命中")
	}

	// 旧格式整值会话（信封化之前的单键单 JSON）同口径迁移
	rootLegacy := `D:\库癸`
	legacy := &WorkbenchSession{
		SchemaVersion: workbenchSessionSchemaVersion, RootPath: rootLegacy,
		State: WorkbenchSessionRecognized, PendingWorkbenchCreate: true,
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("构造旧格式会话失败: %v", err)
	}
	if err := store.SetValue(workbenchSessionKey(rootLegacy), string(data)); err != nil {
		t.Fatalf("写入旧格式会话失败: %v", err)
	}
	if !mgr.ConsumeWorkbenchCreateMarker(rootLegacy) {
		t.Fatalf("旧整值会话的标记应被迁移消费")
	}
	reloaded, err := mgr.loadSession(rootLegacy)
	if err != nil {
		t.Fatalf("回读旧格式会话失败: %v", err)
	}
	if reloaded.PendingWorkbenchCreate {
		t.Fatalf("迁移消费后旧整值会话字段应清除")
	}
	if mgr.ConsumeWorkbenchCreateMarker(rootLegacy) {
		t.Fatalf("旧整值会话迁移消费恰一轮")
	}

	// 新路径不触旧字段：置位只写外置单键，消费恰一次
	if err := mgr.BeginWorkbenchCreate(root); err != nil {
		t.Fatalf("置位标记失败: %v", err)
	}
	marker, err := store.GetValue(workbenchCreateMarkerKey(root))
	if err != nil || marker == nil || marker.Value == "" {
		t.Fatalf("置位应落外置单键 %q: %+v %v", workbenchCreateMarkerKey(root), marker, err)
	}
	sess, err = mgr.loadSession(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if sess.PendingWorkbenchCreate {
		t.Fatalf("新置位不得写旧信封字段")
	}
	if !mgr.ConsumeWorkbenchCreateMarker(root) {
		t.Fatalf("外置键消费应返回真")
	}
	if mgr.ConsumeWorkbenchCreateMarker(root) {
		t.Fatalf("外置键消费恰一次")
	}
}

// TestHandleFrontendRequestDispatch 前端事件协议请求分派：会话清单（附默认库根
// 缺席）、新建冲突回告既有会话摘要、恢复会话携带对账结论、入口确认无等待者时
// 回告失效、未知动作回告错误——应答一律按请求序号配对。
func TestHandleFrontendRequestDispatch(t *testing.T) {
	store := newFakeSessionStore()
	mgr, ctx := newTestManager(t, store)

	// 清单（空）：默认库根未配置为空
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 1, Action: workbenchActionListSessions}))
	if resp := ctx.lastResponse(t); !resp.OK || len(resp.Sessions) != 0 || resp.DefaultRoot != "" {
		t.Fatalf("空清单应答不符: %+v", resp)
	}

	// 新建
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 2, Action: workbenchActionCreateSession, Root: `D:\库庚`}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.Session == nil || resp.Session.RootPath != `D:\库庚` {
		t.Fatalf("新建会话应答不符: %+v", resp)
	}

	// 同库根再建：冲突 + 既有会话摘要
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 3, Action: workbenchActionCreateSession, Root: `d:/库庚`}))
	if resp := ctx.lastResponse(t); resp.OK || resp.Conflict == nil || resp.Conflict.RootPath != `D:\库庚` {
		t.Fatalf("同库根再建应冲突并回告既有会话: %+v", resp)
	}

	// 恢复：档案对账结论
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 4, Action: workbenchActionActivateSession, Root: `D:\库庚`}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.ProfileChanged {
		t.Fatalf("恢复会话应答不符（档案未变更）: %+v", resp)
	}

	// 入口确认答复：无等待者 → 失效回告
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 5, Action: workbenchActionConfirmAnswer, ConfirmSeq: 99, Accepted: true}))
	if resp := ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "失效") {
		t.Fatalf("无等待者的确认答复应回告失效: %+v", resp)
	}

	// 未知动作
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 6, Action: "nonsense"}))
	if resp := ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "未知") {
		t.Fatalf("未知动作应回告错误: %+v", resp)
	}

	// 删除
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 7, Action: workbenchActionDeleteSession, Root: `D:\库庚`}))
	if resp := ctx.lastResponse(t); !resp.OK {
		t.Fatalf("删除会话应答不符: %+v", resp)
	}
	sessions, err := mgr.ListSessions()
	if err != nil || len(sessions) != 0 {
		t.Fatalf("删除后应无会话，实际: %v %v", sessions, err)
	}
}

// mustJSON 序列化请求载荷。
func mustJSON(t *testing.T, req workbenchFrontendRequest) []byte {
	t.Helper()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("序列化请求失败: %v", err)
	}
	return data
}

// TestGetPanoramaDiffConclusionProjection 护栏四·差异巨表装载瘦身：巨差异轮
//（复用 TestGetPanoramaOverviewDecisionsDiffBudget 的夹具构造形态：变更 5 万 +
// 新增/消失各 5 千，条目带前后结论）overview 差异分节走线上传输投影——每清单
// 头 panoramaDiffConclusionHead 条保留前后结论、其余仅相对路径。断言面：
//   - 投影形态：头 32 条含前后结论（含内容抽查）、其余仅路径；路径序列与
//     存储态逐条全等，回执 Total/条数不随投影变化；
//   - 页装配实测：每页整答 json.Marshal ≤ panoramaPageSoftBytes 软界，页数
//     ≤3 且显著低于全量结论形态推演页数（生产估算器 snapDiffEntryWireBytes +
//     打包函数 panoramaPackEnd 对未投影条目空间的同预算推演，夹具量级自检
//     ≥4 页），线上装载字节合计低于全量差异集字节的一半；
//   - 存储保全：重启读回（新管理器实例走 KV 重载路径）LastDiff 与注入形态
//     JSON 逐字节全等——投影只动线上传输，不动存储态。
func TestGetPanoramaDiffConclusionProjection(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)

	// 快照本体保持小量（六段在场即可），差异集巨量：变更 5 万 + 新增/消失各 5 千
	const (
		changedTotal      = 50000
		addedRemovedTotal = 5000
	)
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: 1700000000007,
		RootPath:      root,
		Dirs:          []SnapDir{{RelKey: "库根目录甲", Name: "库根目录甲", Depth: 1, FileCount: changedTotal}},
		Stats:         SnapStats{TotalFiles: changedTotal + 2*addedRemovedTotal},
	}
	diff := &SnapDiff{
		Changed: make([]SnapDiffEntry, 0, changedTotal),
		Added:   make([]SnapDiffEntry, 0, addedRemovedTotal),
		Removed: make([]SnapDiffEntry, 0, addedRemovedTotal),
	}
	for i := 0; i < changedTotal; i++ {
		rel := fmt.Sprintf("库根目录甲/子目录乙/作品集丙长名占位段/卷次目录_%03d/变更文件占位文段甲乙丙_%06d.jpg", i%40, i)
		diff.Changed = append(diff.Changed, SnapDiffEntry{RelPath: rel,
			Before: &SnapConclusion{State: snapStatePlain, PendingDims: []string{"标题", "作者"},
				Final: []SnapDecl{{Dimension: "作者", Value: fmt.Sprintf("旧作者占位_%06d", i), Source: "rule", RoleName: "画师"}}},
			After: &SnapConclusion{State: snapStateIdentified,
				Final: []SnapDecl{{Dimension: "作者", Value: fmt.Sprintf("新作者占位_%06d", i), Source: "assign", RoleName: "画师"},
					{Dimension: "标题", Value: fmt.Sprintf("新标题占位_%06d", i), Source: "assign"}}}})
	}
	for i := 0; i < addedRemovedTotal; i++ {
		diff.Added = append(diff.Added, SnapDiffEntry{
			RelPath: fmt.Sprintf("库根目录甲/子目录乙/新增文件占位文段甲乙丙_%06d.jpg", i),
			After:   &SnapConclusion{State: snapStatePlain}})
		diff.Removed = append(diff.Removed, SnapDiffEntry{
			RelPath: fmt.Sprintf("库根目录甲/子目录乙/消失文件占位文段甲乙丙_%06d.jpg", i),
			Before:  &SnapConclusion{State: snapStateIdentified}})
	}
	if _, err := m.CompleteRecognitionWithDiff(root, snap, diff); err != nil {
		t.Fatalf("注入快照与差异集失败: %v", err)
	}

	// 存储保全：重启读回走 KV 重载路径，全量差异集与注入形态 JSON 逐字节全等。
	rebooted := NewSessionManagerWithStore(ctx, nil, ctx.store)
	sessStored, err := rebooted.SessionOf(root)
	if err != nil {
		t.Fatalf("重启后读会话失败: %v", err)
	}
	if sessStored.LastDiff == nil {
		t.Fatal("差异集应随会话持久化")
	}
	storedJSON, err := json.Marshal(sessStored.LastDiff)
	if err != nil {
		t.Fatalf("存储差异集序列化失败: %v", err)
	}
	injectedJSON, err := json.Marshal(diff)
	if err != nil {
		t.Fatalf("注入差异集序列化失败: %v", err)
	}
	if !bytes.Equal(storedJSON, injectedJSON) {
		t.Fatalf("存储 LastDiff 应全量结论保全（读回 %d vs 注入 %d 字节）", len(storedJSON), len(injectedJSON))
	}

	// overview 默认页大小逐页拉完（页预算/条数上限同生产缺省）
	total := 1 + changedTotal + 2*addedRemovedTotal // 目录 1 条 + 差异三清单
	gotDiff := &SnapDiff{Changed: []SnapDiffEntry{}, Added: []SnapDiffEntry{}, Removed: []SnapDiffEntry{}}
	var wireBytes int
	pages := 0
	cursor := ""
	for page := 0; ; page++ {
		m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
			Seq: int64(950 + page), Action: workbenchActionGetPanorama, Root: root,
			Section: panoramaSectionOverview, Cursor: cursor,
		}))
		resp := ctx.lastResponse(t)
		if !resp.OK || resp.Panorama == nil || resp.Page == nil || resp.Diff == nil {
			t.Fatalf("overview 分页拉取失败（第 %d 页）: %+v", page+1, resp)
		}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("页应答序列化失败: %v", err)
		}
		if len(data) > panoramaPageSoftBytes {
			t.Fatalf("第 %d 页整答实测 %d 字节超软界 %d", page+1, len(data), panoramaPageSoftBytes)
		}
		wireBytes += len(data)
		if resp.Page.Total != total {
			t.Fatalf("页回执 Total = %d，期望 %d（条数不随投影变化）", resp.Page.Total, total)
		}
		gotDiff.Changed = append(gotDiff.Changed, resp.Diff.Changed...)
		gotDiff.Added = append(gotDiff.Added, resp.Diff.Added...)
		gotDiff.Removed = append(gotDiff.Removed, resp.Diff.Removed...)
		pages = page + 1
		if resp.Page.Done {
			if resp.Page.NextCursor != "" {
				t.Fatal("末页不应携带续拉游标")
			}
			break
		}
		if resp.Page.NextCursor == "" {
			t.Fatal("非末页缺续拉游标")
		}
		cursor = resp.Page.NextCursor
	}

	// 投影形态：头 32 条按清单本来的结论侧保全、其余仅路径；路径与存储态逐条全等
	checkProjected := func(name string, got, want []SnapDiffEntry, wantBefore, wantAfter bool) {
		if len(got) != len(want) {
			t.Fatalf("%s 拼接条数 = %d，期望 %d", name, len(got), len(want))
		}
		for i := range got {
			if got[i].RelPath != want[i].RelPath {
				t.Fatalf("%s 第 %d 条路径 %q 与存储态 %q 不符", name, i+1, got[i].RelPath, want[i].RelPath)
			}
			if i < panoramaDiffConclusionHead {
				if wantBefore && got[i].Before == nil {
					t.Fatalf("%s 头界内第 %d 条缺 Before 结论", name, i+1)
				}
				if wantAfter && got[i].After == nil {
					t.Fatalf("%s 头界内第 %d 条缺 After 结论", name, i+1)
				}
				continue
			}
			if got[i].Before != nil || got[i].After != nil {
				t.Fatalf("%s 第 %d 条（超出头 %d 条）应仅携带路径，实际携带结论", name, i+1, panoramaDiffConclusionHead)
			}
		}
	}
	checkProjected("变更清单", gotDiff.Changed, diff.Changed, true, true)
	checkProjected("新增清单", gotDiff.Added, diff.Added, false, true)
	checkProjected("消失清单", gotDiff.Removed, diff.Removed, true, false)
	// 头界内条目的结论内容保全抽查（头界最后一条：Before plain+两待裁决维度 /
	// After identified+两条终选声明）
	if head := gotDiff.Changed[panoramaDiffConclusionHead-1]; head.Before == nil || head.Before.State != snapStatePlain ||
		len(head.Before.PendingDims) != 2 || head.After == nil || head.After.State != snapStateIdentified || len(head.After.Final) != 2 {
		t.Fatalf("变更清单头界最后一条应保全前后结论内容: %+v", head)
	}

	// 页数收益对照：全量结论形态推演——同一条目空间（目录 1 条 + 差异三清单）
	// 喂生产估算器（未投影条目，结论全在场）与打包函数，共用同一页预算与条数
	// 上限；页数差异即瘦身的装载收益。
	fullCost := func(i int) int {
		if i == 0 {
			return snapDirWireBytes(snap.Dirs[0])
		}
		switch j := i - 1; {
		case j < changedTotal:
			return snapDiffEntryWireBytes(diff.Changed[j])
		case j < changedTotal+addedRemovedTotal:
			return snapDiffEntryWireBytes(diff.Added[j-changedTotal])
		default:
			return snapDiffEntryWireBytes(diff.Removed[j-changedTotal-addedRemovedTotal])
		}
	}
	fullPages := 0
	for offset := 0; offset < total; fullPages++ {
		end := panoramaPackEnd(fullCost, offset, total, panoramaPageMaxEntries)
		if end <= offset {
			t.Fatal("全量结论形态推演打包未推进")
		}
		offset = end
	}
	if fullPages < 4 {
		t.Fatalf("全量结论形态推演页数 %d 应 ≥4（夹具量级自检）", fullPages)
	}
	if pages > 3 {
		t.Fatalf("投影后 5 万变更 + 5 千新增/消失应 ≤3 页装载，实际 %d 页", pages)
	}
	if pages >= fullPages {
		t.Fatalf("投影页数 %d 应显著低于全量结论形态 %d 页", pages, fullPages)
	}
	if wireBytes*2 >= len(injectedJSON) {
		t.Fatalf("线上装载实测 %d 字节应低于全量差异集 %d 字节的一半", wireBytes, len(injectedJSON))
	}
	t.Logf("投影装载 %d 页 / 整答合计 %d 字节；全量结论形态推演 %d 页 / 全量差异集 %d 字节",
		pages, wireBytes, fullPages, len(injectedJSON))
}

// TestWireBytesCountAlsoRuleIDs 覆盖页界估算器对附属规则清单的补计：文件
// 条目与结论摘要的字节估算均按逐条口径计入附属规则 ID，差值与清单估算
// helper 一致。
func TestWireBytesCountAlsoRuleIDs(t *testing.T) {
	decl := SnapDecl{Dimension: "author", Value: "作者甲", RuleID: "r1", AlsoRuleIDs: []string{"r2", "r3"}}
	plain := decl
	plain.AlsoRuleIDs = nil
	delta := alsoRuleIDsWireBytes([]string{"r2", "r3"})
	if delta <= 0 {
		t.Fatalf("非空附属清单估算应计正字节，实际 %d", delta)
	}
	fileWith := SnapFile{RelPath: "a.jpg", State: "identified", Final: []SnapDecl{decl}}
	filePlain := SnapFile{RelPath: "a.jpg", State: "identified", Final: []SnapDecl{plain}}
	if got := snapFileWireBytes(fileWith) - snapFileWireBytes(filePlain); got != delta {
		t.Errorf("文件条目估算附属清单增量 = %d，期望 %d", got, delta)
	}
	conWith := SnapConclusion{State: "identified", Final: []SnapDecl{decl}}
	conPlain := SnapConclusion{State: "identified", Final: []SnapDecl{plain}}
	if got := snapConclusionWireBytes(conWith) - snapConclusionWireBytes(conPlain); got != delta {
		t.Errorf("结论摘要估算附属清单增量 = %d，期望 %d", got, delta)
	}
	if got := alsoRuleIDsWireBytes(nil); got != 0 {
		t.Errorf("空附属清单应记零，实际 %d", got)
	}
}
