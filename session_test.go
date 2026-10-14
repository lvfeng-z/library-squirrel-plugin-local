// 工作台会话管理器的单元测试：KV 键格式与归一、多会话并存与同库根唯一、
// 跨实例（重启）留存往返、状态机迁移判定、档案写三入口并发互斥、档案外部
// 变更对账、工作台发起创建标记的生命周期，以及前端事件协议的请求分派。
package main

import (
	"encoding/json"
	"errors"
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

// workbenchTestCtx 工作台测试用的插件上下文桩：KV 走内存存储、前端事件发布被
// 记录供断言；其余方法一旦被调用即 panic 暴露测试缺口。
type workbenchTestCtx struct {
	sdkdto.PluginContext
	store *fakeSessionStore

	// library 谓词索引构造的分页数据集（Query* 四方法的桩数据；零值=四轨全空）。
	library libraryFixture

	// CreateTask 桩：createTaskStub 为应答（nil=默认成功 AddedQuantity=3），
	// 计数与末次 URL 供创建编排断言。
	createTaskStub   *sdkdto.CreateTaskResult
	createTaskCount  int
	createTaskLastURL string

	mu        sync.Mutex
	published []publishedEvent
}

// CreateTask 创建编排动作（ApplyCreateTasks）的桩：记录调用并回告预设结论
//（真实 Create 过滤产出分支由测试直驱，不经此桩）。
func (c *workbenchTestCtx) CreateTask(url string) (*sdkdto.CreateTaskResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createTaskCount++
	c.createTaskLastURL = url
	if c.createTaskStub != nil {
		return c.createTaskStub, nil
	}
	return &sdkdto.CreateTaskResult{Succeed: true, AddedQuantity: 3}, nil
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

// GetWorkDir 库根未配置形态：返回错误（会话管理器按未配置处理，默认库根缺席）。
func (c *workbenchTestCtx) GetWorkDir() (*sdkdto.GetWorkDirResponse, error) {
	return nil, errors.New("库根未配置（测试桩）")
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
