// 工作台会话管理：多会话（按库根）的 KV 持久化工件、会话状态机、档案与会话写的
// 单点串行、库根获取与档案外部变更对账，以及工作台前端事件协议（请求/应答/事件）
// 的请求侧处理。识别全景快照、单次赋值与资源选择集等会话内容字段的模型与编排
// 见 recognition.go / parse_actions.go / selection.go。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 工作台前端事件协议通道。请求由前端发出（携带序号），应答按序号配对回发，
// 事件为插件主动通知（入口确认弹窗拉起/关窗指令）。
const (
	workbenchTopicRequest  = "plugin:local-import:workbench:request"
	workbenchTopicResponse = "plugin:local-import:workbench:response"
	workbenchTopicEvent    = "plugin:local-import:workbench:event"
)

// workbenchSessionKeyPrefix 会话工件在插件 KV 中的键前缀，后接库根标识。
const workbenchSessionKeyPrefix = "workbench-session:"

// workbenchSessionSchemaVersion 会话工件 JSON 的结构版本（自管：主程序只存取明文，
// 版本兼容性由插件自负）。
const workbenchSessionSchemaVersion = 1

// 库根与会话标识。
var (
	// ErrWorkbenchSessionExists 对已有活动会话的库根再新建会话。
	ErrWorkbenchSessionExists = errors.New("该库根已有进行中的解析会话")
	// ErrWorkbenchSessionNotFound 按库根取会话未命中。
	ErrWorkbenchSessionNotFound = errors.New("未找到该库根的解析会话")
)

// normalizeLibraryRoot 库根路径 → 会话标识：去空白、Clean、统一小写与正斜杠
// （Windows 盘符路径大小写不敏感，同一库根的不同书写形态须归一到同一键）。
func normalizeLibraryRoot(root string) string {
	cleaned := filepath.Clean(strings.TrimSpace(root))
	return strings.ReplaceAll(strings.ToLower(cleaned), "\\", "/")
}

// workbenchSessionKey 库根 → 会话 KV 键。
func workbenchSessionKey(root string) string {
	return workbenchSessionKeyPrefix + normalizeLibraryRoot(root)
}

// WorkbenchSessionState 会话生命周期状态。
type WorkbenchSessionState string

const (
	// WorkbenchSessionCreated 会话已建立，尚未完成全量识别。
	WorkbenchSessionCreated WorkbenchSessionState = "created"
	// WorkbenchSessionRecognized 全量识别完成（识别全景快照在场）。
	WorkbenchSessionRecognized WorkbenchSessionState = "recognized"
)

// legalSessionTransition 状态机合法迁移判定：唯一前向迁移为 created→recognized
// （全量识别完成时进入）；其余组合（含回退与自迁移）均非法。
func legalSessionTransition(from, to WorkbenchSessionState) bool {
	return from == WorkbenchSessionCreated && to == WorkbenchSessionRecognized
}

// WorkbenchSession 会话持久化工件（KV 单键 JSON）。特异性知识（单次赋值、选择集、
// 识别快照与重识别游标）属会话态；普适规律入规则档案，不在会话内。
type WorkbenchSession struct {
	// SchemaVersion 工件结构版本（自管）。
	SchemaVersion int `json:"schemaVersion"`
	// RootPath 建会话时的库根原始输入（展示用；键标识见 normalizeLibraryRoot）。
	RootPath string `json:"rootPath"`
	// State 生命周期状态。
	State WorkbenchSessionState `json:"state"`
	// CreatedAtMs / UpdatedAtMs 建立与最近更新时间（Unix 毫秒）。
	CreatedAtMs int64 `json:"createdAtMs"`
	UpdatedAtMs int64 `json:"updatedAtMs"`
	// ProfileDigest 最近一次会话写时点的规则档案指纹：恢复会话时与档案当前
	// 指纹比对，不一致即档案在会话外被变更，提示重新识别。
	ProfileDigest string `json:"profileDigest"`
	// AssignRevision 会话赋值修订号：每次赋值写（新增/修改/清除）递增。识别
	// 快照记录其求值所依据的修订号，重识别的增量资格按修订号一致判定——赋值
	// 被清除后其目录不再由赋值集锚定，修订号闸门保证清除前的结论不被 mtime
	// 游标复用（赋值变更一律全量重算）。
	AssignRevision int64 `json:"assignRevision,omitempty"`
	// PendingWorkbenchCreate 工作台发起创建的标记：工作台创建编排动作
	//（ApplyCreateTasks）在调用 CreateTask 前置位，Create 按此分派到「过滤产出」
	// 分支（只产出所选作品，防递归确认），分派时消费。
	PendingWorkbenchCreate bool `json:"pendingWorkbenchCreate"`
	// Snapshot 识别全景快照（裁剪形态，识别干跑完成时写入；含按目录 mtime 的
	// 重识别增量游标）。
	Snapshot *RecognitionSnapshot `json:"snapshot,omitempty"`
	// LastDiff 最近一次重识别的差异集（与上一份快照的文件级结论对比；生成规则
	// 后的强制验证轮据此高亮——重识别完成时覆写，会话恢复后仍可呈现）。
	LastDiff *SnapDiff `json:"lastDiff,omitempty"`
	// Assignments 会话赋值（特异性手工解析结果，相对库根的 '/' 路径寻址）。
	// 赋值创建交互属后续阶段，本阶段承载其数据模型与求值消费。
	Assignments map[string]SessionAssignment `json:"assignments,omitempty"`
	// Ignored 已忽略项（相对路径集合，阶段3 接线）。
	Ignored []string `json:"ignored,omitempty"`
	// Selection 资源选择集（已选定为任务的作品键集合，阶段4 接线）：作品为最小
	// 粒度（多轨原子，键形态见 selection.go 的两类前缀——半作品结构性不可表达），
	// 集为默认聚合单元（集聚合由前端按选择域展开为成员作品键）。创建成功后清空，
	// 赋值与快照保留。
	Selection []string `json:"selection,omitempty"`
}

// SessionStore 会话读写所需的最小存储方法集；签名与 SDK 的 dto.KVStore 同名方法
// 一致，按需收窄依赖。
type SessionStore interface {
	GetValue(key string) (*sdkdto.StorageValue, error)
	SetValue(key string, value string) error
	DeleteValue(key string) error
	GetAllValues() (map[string]*sdkdto.StorageValue, error)
}

// contextSessionStore 把宿主提供的完整插件上下文收窄为会话存储。
type contextSessionStore struct {
	ctx sdkdto.PluginContext
}

// NewContextSessionStore 把插件上下文适配为会话存储；ctx 为 nil 时适配结果仍可
// 构造，读写时返回错误而不静默丢弃。
func NewContextSessionStore(ctx sdkdto.PluginContext) SessionStore {
	return contextSessionStore{ctx: ctx}
}

func (s contextSessionStore) GetValue(key string) (*sdkdto.StorageValue, error) {
	if s.ctx == nil {
		return nil, fmt.Errorf("插件上下文不可用，无法读取键 %q", key)
	}
	return s.ctx.GetValue(key)
}

func (s contextSessionStore) SetValue(key string, value string) error {
	if s.ctx == nil {
		return fmt.Errorf("插件上下文不可用，无法写入键 %q", key)
	}
	return s.ctx.SetValue(key, value)
}

func (s contextSessionStore) DeleteValue(key string) error {
	if s.ctx == nil {
		return fmt.Errorf("插件上下文不可用，无法删除键 %q", key)
	}
	return s.ctx.DeleteValue(key)
}

func (s contextSessionStore) GetAllValues() (map[string]*sdkdto.StorageValue, error) {
	if s.ctx == nil {
		return nil, errors.New("插件上下文不可用，无法枚举键")
	}
	return s.ctx.GetAllValues()
}

// ArchiveWriteEntry 档案写入口标识：全部档案写经会话管理器单点串行，入口标识
// 用于日志观测与桩入口区分。
type ArchiveWriteEntry string

const (
	// ArchiveWriteGeneralize 手动解析泛化成规则。
	ArchiveWriteGeneralize ArchiveWriteEntry = "generalize"
	// ArchiveWriteAdjudicate 冲突裁决生成组合规则。
	ArchiveWriteAdjudicate ArchiveWriteEntry = "adjudicate"
	// ArchiveWriteRuleEdit 规则编辑面（条目保存/启停/删除与内置禁用清单维护）。
	ArchiveWriteRuleEdit ArchiveWriteEntry = "rule-edit"
)

// confirmAnswerHandler 前端确认答复的处理方：Create 流的入口确认协调器在等待期
// 注入；返回是否命中在等的确认（无等待者时答复迟到，按错误回告）。
type confirmAnswerHandler func(confirmSeq int64, accepted bool) bool

// SessionManager 工作台会话管理器：会话增删改查、档案写与会的单点串行、库根
// 获取、档案对账与前端事件协议的请求处理。
//
// 串行模型：mu 覆盖全部会话键写与档案读改写——会话赋值写、档案三入口写（泛化/
// 裁决生成/规则编辑面）与同库根会话增删在同一锁上串行，无双写竞争。
type SessionManager struct {
	ctx    sdkdto.PluginContext
	logger sdkdto.Logger
	store  SessionStore

	// mu 会话写与档案写的单点串行锁。
	mu sync.Mutex

	// confirmMu 保护 confirmAnswer（装配期注入一次，处理 goroutine 只读）。
	confirmMu sync.RWMutex
	confirm   confirmAnswerHandler

	// recogMu 保护 recogRunning：识别干跑在跑互斥（按库根），干跑不持 mu
	//（长跑不阻塞会话读写，落库时经 mutateSession 短暂加锁）。
	recogMu      sync.Mutex
	recogRunning map[string]bool
}

// NewSessionManager 创建会话管理器；ctx 为 nil 时可构造但读写报错（单测经
// NewSessionManagerWithStore 注入存储）。
func NewSessionManager(ctx sdkdto.PluginContext, logger sdkdto.Logger) *SessionManager {
	return NewSessionManagerWithStore(ctx, logger, NewContextSessionStore(ctx))
}

// NewSessionManagerWithStore 创建指定存储后端的会话管理器（单测注入内存存储）。
func NewSessionManagerWithStore(ctx sdkdto.PluginContext, logger sdkdto.Logger, store SessionStore) *SessionManager {
	return &SessionManager{ctx: ctx, logger: logger, store: store}
}

// SetConfirmAnswerHandler 注入入口确认答复的处理方（激活装配期调用）。
func (m *SessionManager) SetConfirmAnswerHandler(h confirmAnswerHandler) {
	m.confirmMu.Lock()
	defer m.confirmMu.Unlock()
	m.confirm = h
}

// logf 日志Shortcut：无 logger 时静默（单测桩免注入）。
func (m *SessionManager) logf(format string, args ...any) {
	if m.logger != nil {
		m.logger.Infof(format, args...)
	}
}

// warnf 警告日志：无 logger 时静默。
func (m *SessionManager) warnf(format string, args ...any) {
	if m.logger != nil {
		m.logger.Warnf(format, args...)
	}
}

// DefaultLibraryRoot 默认库根：优先宿主工作目录（GetWorkDir）；未配置
// （FailedPrecondition）或查询失败时返回未配置，调用方回退用户自选路径。
func (m *SessionManager) DefaultLibraryRoot() (string, bool) {
	if m.ctx == nil {
		return "", false
	}
	resp, err := m.ctx.GetWorkDir()
	if err != nil {
		m.warnf("查询库根未配置或失败，按未配置回退用户选路径: %v", err)
		return "", false
	}
	if resp == nil || resp.Path == "" {
		return "", false
	}
	return resp.Path, true
}

// profileDigestOf 规则档案指纹：序列化字节的 SHA-256（对账用）。
func profileDigestOf(p *RuleProfile) (string, error) {
	data, err := p.MarshalJSON()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// currentProfileDigest 档案当前指纹；档案不可载入（损坏/版本不符）时返回空串
// 并告警——对账按「指纹缺席」处理，不阻断会话恢复。
func (m *SessionManager) currentProfileDigest() string {
	profile, errs := LoadProfile(m.store)
	for _, err := range errs {
		m.warnf("规则档案载入异常: %v", err)
	}
	if profile == nil {
		return ""
	}
	digest, err := profileDigestOf(profile)
	if err != nil {
		m.warnf("计算规则档案指纹失败: %v", err)
		return ""
	}
	return digest
}

// saveSession 锁内序列化落库（调用方须持 mu）。
func (m *SessionManager) saveSession(sess *WorkbenchSession) error {
	sess.SchemaVersion = workbenchSessionSchemaVersion
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("会话序列化失败: %w", err)
	}
	if err := m.store.SetValue(workbenchSessionKey(sess.RootPath), string(data)); err != nil {
		return fmt.Errorf("写入会话（键 %q）失败: %w", workbenchSessionKey(sess.RootPath), err)
	}
	return nil
}

// loadSessionLocked 读单键会话（锁外读亦安全：只读单键）；未写入返回
// ErrWorkbenchSessionNotFound，载荷损坏或版本不符按未命中处理并告警。
func (m *SessionManager) loadSession(root string) (*WorkbenchSession, error) {
	stored, err := m.store.GetValue(workbenchSessionKey(root))
	if err != nil {
		return nil, fmt.Errorf("读取会话（键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	if stored == nil || stored.Value == "" {
		return nil, ErrWorkbenchSessionNotFound
	}
	var sess WorkbenchSession
	if err := json.Unmarshal([]byte(stored.Value), &sess); err != nil {
		m.warnf("会话载荷无法解析，按未命中处理（键 %q）: %v", workbenchSessionKey(root), err)
		return nil, ErrWorkbenchSessionNotFound
	}
	if sess.SchemaVersion != workbenchSessionSchemaVersion {
		m.warnf("会话结构版本 %d 与当前支持版本 %d 不符，按未命中处理（键 %q）",
			sess.SchemaVersion, workbenchSessionSchemaVersion, workbenchSessionKey(root))
		return nil, ErrWorkbenchSessionNotFound
	}
	return &sess, nil
}

// SessionOf 按库根取会话。
func (m *SessionManager) SessionOf(root string) (*WorkbenchSession, error) {
	return m.loadSession(root)
}

// CreateSession 建立会话（同库根至多一活动会话：已有会话时返回
// ErrWorkbenchSessionExists，由前端引导恢复或确认覆盖重建）。建会话即记录档案
// 指纹，作为后续对账基准。
func (m *SessionManager) CreateSession(root string) (*WorkbenchSession, error) {
	if normalizeLibraryRoot(root) == "" || normalizeLibraryRoot(root) == "." {
		return nil, errors.New("库根路径为空，无法建立会话")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, err := m.loadSession(root); err == nil {
		_ = existing
		return nil, ErrWorkbenchSessionExists
	} else if !errors.Is(err, ErrWorkbenchSessionNotFound) {
		return nil, err
	}

	now := time.Now().UnixMilli()
	sess := &WorkbenchSession{
		RootPath:      strings.TrimSpace(root),
		State:         WorkbenchSessionCreated,
		CreatedAtMs:   now,
		UpdatedAtMs:   now,
		ProfileDigest: m.currentProfileDigest(),
	}
	if err := m.saveSession(sess); err != nil {
		return nil, err
	}
	m.logf("已建立解析会话: root=%s", sess.RootPath)
	return sess, nil
}

// ReplaceSession 确认覆盖重建：删除既有会话后原库根重建（既有会话态不保留）。
func (m *SessionManager) ReplaceSession(root string) (*WorkbenchSession, error) {
	m.mu.Lock()
	if err := m.store.DeleteValue(workbenchSessionKey(root)); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("删除既有会话（键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	m.mu.Unlock()
	return m.CreateSession(root)
}

// DeleteSession 删除会话（幂等：会话不存在视为成功）。
func (m *SessionManager) DeleteSession(root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.store.DeleteValue(workbenchSessionKey(root)); err != nil {
		return fmt.Errorf("删除会话（键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	m.logf("已删除解析会话: root=%s", root)
	return nil
}

// ListSessions 枚举全部会话（键前缀扫描），按最近更新时间倒序；单键载荷损坏或
// 版本不符的条目跳过并告警，不阻塞整体。
func (m *SessionManager) ListSessions() ([]WorkbenchSession, error) {
	all, err := m.store.GetAllValues()
	if err != nil {
		return nil, fmt.Errorf("枚举插件 KV 失败: %w", err)
	}
	sessions := make([]WorkbenchSession, 0, len(all))
	for key, stored := range all {
		if !strings.HasPrefix(key, workbenchSessionKeyPrefix) {
			continue
		}
		if stored == nil || stored.Value == "" {
			continue
		}
		var sess WorkbenchSession
		if err := json.Unmarshal([]byte(stored.Value), &sess); err != nil {
			m.warnf("会话载荷无法解析，跳过（键 %q）: %v", key, err)
			continue
		}
		if sess.SchemaVersion != workbenchSessionSchemaVersion {
			m.warnf("会话结构版本 %d 不符，跳过（键 %q）", sess.SchemaVersion, key)
			continue
		}
		sessions = append(sessions, sess)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAtMs != sessions[j].UpdatedAtMs {
			return sessions[i].UpdatedAtMs > sessions[j].UpdatedAtMs
		}
		return sessions[i].RootPath < sessions[j].RootPath
	})
	return sessions, nil
}

// ProfileReconcile 恢复会话时的档案对账结论。
type ProfileReconcile struct {
	// Changed 档案在会话外被变更（会话记录指纹 ≠ 档案当前指纹）。
	Changed bool
	// Message 用户可读的对账提示文案。
	Message string
}

// ActivateSession 恢复会话并做档案对账：识别结论依赖「档案 ∪ 会话赋值」双源，
// 档案被外部变更（停库直写等）时既有识别快照失真，提示重新识别。指纹缺席
// （档案不可载入）不视为变更。
func (m *SessionManager) ActivateSession(root string) (*WorkbenchSession, ProfileReconcile, error) {
	sess, err := m.SessionOf(root)
	if err != nil {
		return nil, ProfileReconcile{}, err
	}
	reconcile := ProfileReconcile{}
	current := m.currentProfileDigest()
	if current != "" && sess.ProfileDigest != "" && current != sess.ProfileDigest {
		reconcile.Changed = true
		reconcile.Message = "规则档案在会话外被变更，既有识别结论可能与最新规则不一致，建议重新识别"
	}
	return sess, reconcile, nil
}

// mutateSession 锁内会话读改写（单点串行）；mutate 返回错误即中止不落库。
func (m *SessionManager) mutateSession(root string, mutate func(*WorkbenchSession) error) (*WorkbenchSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	if err := mutate(sess); err != nil {
		return nil, err
	}
	sess.UpdatedAtMs = time.Now().UnixMilli()
	if err := m.saveSession(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// MarkRecognized 会话进入全量识别完成态（状态机唯一前向迁移 created→recognized）。
// 生产路径经 CompleteRecognition（快照写入与状态迁移单次写原子完成），本方法
// 保留纯状态迁移语义供单测直接驱动状态机。
func (m *SessionManager) MarkRecognized(root string) (*WorkbenchSession, error) {
	return m.mutateSession(root, func(sess *WorkbenchSession) error {
		if !legalSessionTransition(sess.State, WorkbenchSessionRecognized) {
			return fmt.Errorf("会话状态 %s 不允许迁移到 %s", sess.State, WorkbenchSessionRecognized)
		}
		sess.State = WorkbenchSessionRecognized
		return nil
	})
}

// CompleteRecognition 全量识别完成落库（无差异集形态）：识别全景快照入会话与
// 状态迁移在锁内单次写完成——首次识别迁移 created→recognized；重识别（已
// recognized）只覆写快照，状态不回退。会话的档案指纹同步为快照依据指纹
//（识别时点基准，后续档案外部变更仍可对账检出）。生产路径经
// CompleteRecognitionWithDiff（携带验证轮差异集）。
func (m *SessionManager) CompleteRecognition(root string, snapshot *RecognitionSnapshot) (*WorkbenchSession, error) {
	return m.CompleteRecognitionWithDiff(root, snapshot, nil)
}

// CompleteRecognitionWithDiff 全量识别完成落库：快照、状态迁移与重识别差异集
// 在锁内单次写完成（差异集随每次重识别覆写——首轮识别无前序快照，差异置空）。
func (m *SessionManager) CompleteRecognitionWithDiff(root string, snapshot *RecognitionSnapshot, diff *SnapDiff) (*WorkbenchSession, error) {
	if snapshot == nil {
		return nil, errors.New("识别快照为空，无法落库")
	}
	return m.mutateSession(root, func(sess *WorkbenchSession) error {
		if sess.State == WorkbenchSessionCreated {
			if !legalSessionTransition(sess.State, WorkbenchSessionRecognized) {
				return fmt.Errorf("会话状态 %s 不允许迁移到 %s", sess.State, WorkbenchSessionRecognized)
			}
			sess.State = WorkbenchSessionRecognized
		}
		sess.Snapshot = snapshot
		sess.ProfileDigest = snapshot.ProfileDigest
		sess.LastDiff = diff
		return nil
	})
}

// BeginWorkbenchCreate 置位工作台发起创建标记（工作台在调用 CreateTask 前置位，
// Create 据此分派过滤产出分支）。接线属后续阶段。
func (m *SessionManager) BeginWorkbenchCreate(root string) error {
	_, err := m.mutateSession(root, func(sess *WorkbenchSession) error {
		sess.PendingWorkbenchCreate = true
		return nil
	})
	return err
}

// ConsumeWorkbenchCreateMarker 消费工作台发起创建标记：在位则清除并返回真
// （Create 过滤产出分支的分派判据，消费语义防同一标记被后续创建重复命中）。
func (m *SessionManager) ConsumeWorkbenchCreateMarker(root string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, err := m.loadSession(root)
	if err != nil || !sess.PendingWorkbenchCreate {
		return false
	}
	sess.PendingWorkbenchCreate = false
	sess.UpdatedAtMs = time.Now().UnixMilli()
	if err := m.saveSession(sess); err != nil {
		m.warnf("清除工作台发起创建标记失败: %v", err)
		return false
	}
	return true
}

// —— 会话赋值与忽略项写 API（经 mutateSession 与档案写同一锁串行）——

// SetAssignment 写入单文件的手工解析赋值（特异性知识，会话态）：显式维度
// 覆盖档案求值的同维度终选；不指定任何维度的空赋值等价于清除该文件的赋值。
// 相对路径规范化为 '/' 形态后寻址（与识别快照的 relPath 同键）。
func (m *SessionManager) SetAssignment(root, relPath string, assign SessionAssignment) (*WorkbenchSession, error) {
	relPath = filepath.ToSlash(relPath)
	if relPath == "" {
		return nil, errors.New("相对路径为空，无法写入赋值")
	}
	return m.mutateSession(root, func(sess *WorkbenchSession) error {
		if sess.Assignments == nil {
			sess.Assignments = make(map[string]SessionAssignment)
		}
		if len(assign.coveredDimensions()) == 0 {
			// 空赋值=清除（含已泛化标记一并移除）；修订号递增驱动增量闸门失效
			if _, existed := sess.Assignments[relPath]; existed {
				sess.AssignRevision++
			}
			delete(sess.Assignments, relPath)
			return nil
		}
		// 赋值含切片字段不可整体比较，写路径一律视为变更：修订号递增
		assign.UpdatedAtMs = time.Now().UnixMilli()
		sess.AssignRevision++
		sess.Assignments[relPath] = assign
		return nil
	})
}

// ClearAssignment 清除单文件的会话赋值（幂等：无赋值亦成功）。
func (m *SessionManager) ClearAssignment(root, relPath string) (*WorkbenchSession, error) {
	return m.SetAssignment(root, relPath, SessionAssignment{})
}

// MarkAssignmentGeneralized 置赋值的已泛化标记（元素7：已泛化的赋值其档案规则
// 亦会产出同值声明，标记供呈现与清理提示）。
func (m *SessionManager) MarkAssignmentGeneralized(root, relPath string, generalized bool) (*WorkbenchSession, error) {
	relPath = filepath.ToSlash(relPath)
	return m.mutateSession(root, func(sess *WorkbenchSession) error {
		assign, ok := sess.Assignments[relPath]
		if !ok {
			return fmt.Errorf("文件 %s 无会话赋值，无法标记泛化", relPath)
		}
		assign.Generalized = generalized
		assign.UpdatedAtMs = time.Now().UnixMilli()
		sess.Assignments[relPath] = assign
		return nil
	})
}

// SetIgnored 登记或解除文件的忽略项（元素7：已忽略项不再参与后续解析关注；
// 幂等）。ignored 为假时移除登记。
func (m *SessionManager) SetIgnored(root, relPath string, ignored bool) (*WorkbenchSession, error) {
	relPath = filepath.ToSlash(relPath)
	if relPath == "" {
		return nil, errors.New("相对路径为空，无法登记忽略项")
	}
	return m.mutateSession(root, func(sess *WorkbenchSession) error {
		pos := -1
		for i, item := range sess.Ignored {
			if item == relPath {
				pos = i
				break
			}
		}
		if ignored && pos < 0 {
			sess.Ignored = append(sess.Ignored, relPath)
		} else if !ignored && pos >= 0 {
			sess.Ignored = append(sess.Ignored[:pos], sess.Ignored[pos+1:]...)
		}
		return nil
	})
}

// UpdateProfile 档案单点串行读改写：全部档案写（泛化/裁决生成/规则编辑面三入口）
// 一律经此入口，锁内 Load→mutate→Save 原子完成；mutate 返回错误即中止不落库；
// 档案不可载入（损坏/版本不符）时拒绝写，不覆写既有档案。
func (m *SessionManager) UpdateProfile(entry ArchiveWriteEntry, mutate func(p *RuleProfile) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	profile, errs := LoadProfile(m.store)
	for _, err := range errs {
		m.warnf("规则档案载入异常（入口 %s）: %v", entry, err)
	}
	if profile == nil {
		return fmt.Errorf("规则档案不可载入，拒绝写入（入口 %s）", entry)
	}
	if mutate != nil {
		if err := mutate(profile); err != nil {
			return fmt.Errorf("档案变更被中止（入口 %s）: %w", entry, err)
		}
	}
	if err := SaveProfile(m.store, profile); err != nil {
		return fmt.Errorf("写入规则档案失败（入口 %s）: %w", entry, err)
	}
	m.logf("规则档案已更新（入口 %s）: 规则 %d 条", entry, len(profile.Rules))
	return nil
}

// —— 前端事件协议（请求侧）——

// 前端请求动作词汇。
const (
	workbenchActionListSessions     = "listSessions"
	workbenchActionCreateSession    = "createSession"
	workbenchActionReplaceSession   = "replaceSession"
	workbenchActionDeleteSession    = "deleteSession"
	workbenchActionActivateSession  = "activateSession"
	workbenchActionConfirmAnswer    = "confirmAnswer"
	workbenchActionStartRecognition = "startRecognition"
	workbenchActionGetPanorama      = "getPanorama"
	workbenchActionAssign           = "assign"        // 单次赋值（会话态知识写入）
	workbenchActionClearAssignment  = "clearAssignment" // 清除单文件赋值
	workbenchActionSetIgnored       = "setIgnored"    // 登记或解除忽略项
	workbenchActionGeneralize       = "generalize"    // 泛化成规则（档案写+验证轮）
	workbenchActionAdjudicate       = "adjudicate"    // 冲突裁决生成组合规则
	workbenchActionGetDiff          = "getDiff"       // 最近一次重识别差异集
	workbenchActionSetSelection     = "setSelection"  // 全量替换资源选择集（作品键）
	workbenchActionCreateTasks      = "createTasks"   // 创建编排（对账→CreateTask→清理选择集）

	// —— 规则管理族（规则子页）——
	workbenchActionGetRules           = "getRules"           // 规则管理视图（档案条目+内置表+禁用清单+备份态）
	workbenchActionSaveRule           = "saveRule"           // 规则条目保存（新建/编辑，构造期单条校验）
	workbenchActionDeleteRule         = "deleteRule"         // 删除档案规则条目
	workbenchActionSetRuleEnabled     = "setRuleEnabled"     // 档案规则条目启停
	workbenchActionSetBuiltinDisabled = "setBuiltinDisabled" // 内置规则禁用/启用（档案禁用清单）
	workbenchActionRollbackProfile    = "rollbackProfile"    // 档案回退一代（写前备份）
)

// getPanorama 的分节拉取词汇。
const (
	// panoramaSectionFiles 只取文件级清单（元素2/3 载体 + 序列群 + 待裁决明细 +
	// 统计）——大库清单单独拉取，先渲染概览骨架再拉清单。
	panoramaSectionFiles = "files"
)

// workbenchFrontendRequest 前端→插件请求载荷（序号配对）。
type workbenchFrontendRequest struct {
	Seq    int64  `json:"seq"`
	Action string `json:"action"`
	// Root 会话寻址的库根（createSession 为新建库根输入）。
	Root string `json:"root,omitempty"`
	// ConfirmSeq / Accepted 入口确认答复的配对序号与采纳结果。
	ConfirmSeq int64  `json:"confirmSeq,omitempty"`
	Accepted   bool   `json:"accepted,omitempty"`
	// Section getPanorama 的分节拉取（缺省=全量投影）。
	Section string `json:"section,omitempty"`

	// —— 手动解析族载荷（relPath 为相对库根的 '/' 路径）——
	// RelPath 赋值/泛化/裁决/忽略的目标文件相对路径。
	RelPath string `json:"relPath,omitempty"`
	// Author / AuthorRole / Title / Tags / Collection 单次赋值的各维度指定
	//（assign 动作；空维度不覆盖档案结论）。
	Author     string   `json:"author,omitempty"`
	AuthorRole string   `json:"authorRole,omitempty"`
	Title      string   `json:"title,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Collection string   `json:"collection,omitempty"`
	// Ignored setIgnored 的登记方向（真=登记忽略，假=解除）。
	Ignored bool `json:"ignored,omitempty"`
	// GeneralizeMode generalize 的泛化维度：filename（按文件名形态）/
	// dirLayer（按目录层）/ sequence（按序列形态）。
	GeneralizeMode string `json:"generalizeMode,omitempty"`
	// Dimension adjudicate 的裁决维度（待裁决项寻址）。
	Dimension string `json:"dimension,omitempty"`
	// AdjudicateMode adjudicate 的裁决方式：pick（双命中择一）/ conditional
	//（单侧缺席改绑=谓词条件策略）。
	AdjudicateMode string `json:"adjudicateMode,omitempty"`
	// PickIndex pick 模式采纳的候选下标（待裁决项 candidates 序）。
	PickIndex int `json:"pickIndex,omitempty"`
	// PredicateSource / PredicateDimension conditional 模式的条件谓词
	//（某来源在某维度上的声明是否在场）。
	PredicateSource    string `json:"predicateSource,omitempty"`
	PredicateDimension string `json:"predicateDimension,omitempty"`
	// PresentIndex / AbsentIndex conditional 模式谓词在场/缺席时分别采纳的
	// 候选下标。
	PresentIndex int `json:"presentIndex,omitempty"`
	AbsentIndex  int `json:"absentIndex,omitempty"`

	// —— 资源选择族载荷（阶段4）——
	// Works setSelection 的作品键全集（全量替换：集聚合/树递选/过滤全选等批量
	// 组合由前端按选择域展开为成员作品键后整体提交）。
	Works []string `json:"works,omitempty"`

	// —— 规则管理族载荷（规则子页）——
	// Rule saveRule 的完整规则条目 JSON（ruleEntryJSON 形态：pattern 与
	// extractions 为 JSON 片段原文，synthesis 为表单化字段组装）。
	Rule json.RawMessage `json:"rule,omitempty"`
	// RuleID deleteRule / setRuleEnabled 的档案条目寻址。
	RuleID string `json:"ruleId,omitempty"`
	// Enabled setRuleEnabled 的启停方向（假=停用，字段缺省同假）。
	Enabled bool `json:"enabled,omitempty"`
	// BuiltinID setBuiltinDisabled 的内置规则寻址。
	BuiltinID string `json:"builtinId,omitempty"`
	// Disabled setBuiltinDisabled 的方向（真=加入禁用清单，缺省=移出即启用）。
	Disabled bool `json:"disabled,omitempty"`
}

// workbenchSessionSummary 会话的前端呈现摘要。
type workbenchSessionSummary struct {
	RootPath    string `json:"rootPath"`
	State       string `json:"state"`
	CreatedAtMs int64  `json:"createdAtMs"`
	UpdatedAtMs int64  `json:"updatedAtMs"`
}

// workbenchFrontendResponse 插件→前端应答载荷（按请求序号配对）。
type workbenchFrontendResponse struct {
	Seq   int64  `json:"seq"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Sessions / DefaultRoot listSessions 的会话清单与默认库根（未配置为空）。
	Sessions    []workbenchSessionSummary `json:"sessions,omitempty"`
	DefaultRoot string                    `json:"defaultRoot,omitempty"`
	// Session createSession/replaceSession/activateSession 的会话结果。
	Session *workbenchSessionSummary `json:"session,omitempty"`
	// Conflict createSession 命中既有会话时的既有会话摘要（前端引导恢复或确认覆盖）。
	Conflict *workbenchSessionSummary `json:"conflict,omitempty"`
	// ProfileChanged activateSession 的档案对账结论（真=档案被外部变更，提示重新识别）。
	ProfileChanged bool `json:"profileChanged,omitempty"`
	// Started / Running startRecognition 的启动结论：started=本次请求已启动干跑，
	// running=同库根干跑已在途（本次未重复启动）。
	Started bool `json:"started,omitempty"`
	Running bool `json:"running,omitempty"`
	// Panorama getPanorama 的识别全景投影（section=files 时仅含文件级清单）。
	Panorama *RecognitionSnapshot `json:"panorama,omitempty"`
	// Decisions getPanorama 附带的会话决策态（元素7：赋值/忽略/选择集的活数据）。
	Decisions *workbenchSessionDecisionsView `json:"decisions,omitempty"`
	// Diff getPanorama/getDiff 附带的最近一次重识别差异集（生成规则后强制
	// 验证轮的高亮载荷）。
	Diff *SnapDiff `json:"diff,omitempty"`
	// Assignment assign/clearAssignment 后该文件的赋值呈现（清除后为空）。
	Assignment *workbenchAssignmentView `json:"assignment,omitempty"`
	// Rule generalize/adjudicate 生成的规则摘要（已入档案并触发验证轮）。
	Rule *GeneratedRuleView `json:"rule,omitempty"`
	// Domain getPanorama 附带的资源选择域（阶段4：作品原子结构+集视图，前端
	// 选择交互与选择集键的单一推导源）。
	Domain *WorkDomain `json:"domain,omitempty"`
	// Created / CreateMsg createTasks 的创建结论：已提交作品数与用户可读摘要。
	Created  int    `json:"created,omitempty"`
	CreateMsg string `json:"createMsg,omitempty"`
	// RulesView getRules 与规则管理族动作随答返回的规则管理视图（档案条目+
	// 内置表+禁用清单+备份态），前端每次动作后整份刷新。
	RulesView *rulesAdminView `json:"rulesView,omitempty"`
	// RolledBack rollbackProfile 的回退结论（真=已回退一代）。
	RolledBack bool `json:"rolledBack,omitempty"`
}

// workbenchSessionDecisionsView 会话决策态投影（元素7 的活数据——赋值/忽略/
// 选择集随用户操作即时变化，不入识别快照）。
type workbenchSessionDecisionsView struct {
	// Assignments 已手动解析项（含是否已泛化标记）。
	Assignments []workbenchAssignmentView `json:"assignments,omitempty"`
	// Ignored 已忽略项；Selection 已选定为任务的作品键集合（两类前缀见
	// selection.go——作品原子，半作品不可表达）。
	Ignored   []string `json:"ignored,omitempty"`
	Selection []string `json:"selection,omitempty"`
}

// workbenchAssignmentView 会话赋值的前端呈现形态。
type workbenchAssignmentView struct {
	RelPath     string   `json:"relPath"`
	Author      string   `json:"author,omitempty"`
	AuthorRole  string   `json:"authorRole,omitempty"`
	Title       string   `json:"title,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Collection  string   `json:"collection,omitempty"`
	Generalized bool     `json:"generalized,omitempty"`
	UpdatedAtMs int64    `json:"updatedAtMs,omitempty"`
}

// decisionsViewOf 会话 → 决策态投影（按路径稳定序呈现）。
func decisionsViewOf(sess *WorkbenchSession) *workbenchSessionDecisionsView {
	view := &workbenchSessionDecisionsView{}
	for _, relPath := range sortedKeys(sess.Assignments) {
		assign := sess.Assignments[relPath]
		view.Assignments = append(view.Assignments, workbenchAssignmentView{
			RelPath: relPath, Author: assign.Author, AuthorRole: assign.AuthorRole,
			Title: assign.Title, Tags: assign.Tags, Collection: assign.Collection,
			Generalized: assign.Generalized, UpdatedAtMs: assign.UpdatedAtMs,
		})
	}
	view.Ignored = append(view.Ignored, sess.Ignored...)
	view.Selection = append(view.Selection, sess.Selection...)
	return view
}

func summarizeSession(sess *WorkbenchSession) *workbenchSessionSummary {
	return &workbenchSessionSummary{
		RootPath:    sess.RootPath,
		State:       string(sess.State),
		CreatedAtMs: sess.CreatedAtMs,
		UpdatedAtMs: sess.UpdatedAtMs,
	}
}

// sortedKeys 会话赋值表的路径稳定序（呈现顺序确定）。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// PublishToFrontend 事件发布能力（请求处理应答与确认协调器事件共用；窄接口便于单测注入）。
type frontendPublisher interface {
	PublishToFrontend(topic string, data []byte) error
}

// publishResponse 序列化应答并回发前端。
func (m *SessionManager) publishResponse(resp *workbenchFrontendResponse) {
	if m.ctx == nil {
		return
	}
	data, err := json.Marshal(resp)
	if err != nil {
		m.warnf("工作台应答序列化失败: %v", err)
		return
	}
	if err := m.ctx.PublishToFrontend(workbenchTopicResponse, data); err != nil {
		m.warnf("工作台应答回发失败(seq=%d): %v", resp.Seq, err)
	}
}

// HandleFrontendRequest 处理一条前端请求并回发应答（激活期订阅循环逐条调用）。
// 未知动作/非法载荷回告错误应答，不中断订阅循环。
func (m *SessionManager) HandleFrontendRequest(data []byte) {
	var req workbenchFrontendRequest
	if err := json.Unmarshal(data, &req); err != nil {
		m.warnf("工作台请求载荷无法解析: %v", err)
		return
	}
	resp := &workbenchFrontendResponse{Seq: req.Seq, OK: true}
	switch req.Action {
	case workbenchActionListSessions:
		sessions, err := m.ListSessions()
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		for _, sess := range sessions {
			resp.Sessions = append(resp.Sessions, *summarizeSession(&sess))
		}
		if root, ok := m.DefaultLibraryRoot(); ok {
			resp.DefaultRoot = root
		}
	case workbenchActionCreateSession:
		sess, err := m.CreateSession(req.Root)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			if errors.Is(err, ErrWorkbenchSessionExists) {
				if existing, lookupErr := m.SessionOf(req.Root); lookupErr == nil {
					resp.Conflict = summarizeSession(existing)
				}
			}
			break
		}
		resp.Session = summarizeSession(sess)
	case workbenchActionReplaceSession:
		sess, err := m.ReplaceSession(req.Root)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
	case workbenchActionDeleteSession:
		if err := m.DeleteSession(req.Root); err != nil {
			resp.OK, resp.Error = false, err.Error()
		}
	case workbenchActionActivateSession:
		sess, reconcile, err := m.ActivateSession(req.Root)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.ProfileChanged = reconcile.Changed
	case workbenchActionConfirmAnswer:
		m.confirmMu.RLock()
		handler := m.confirm
		m.confirmMu.RUnlock()
		if handler == nil || !handler(req.ConfirmSeq, req.Accepted) {
			resp.OK, resp.Error = false, "确认请求已失效（可能已超时关闭），请重新发起"
		}
	case workbenchActionStartRecognition:
		started, running, err := m.StartRecognition(req.Root)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Started = started
		resp.Running = running
	case workbenchActionGetPanorama:
		sess, err := m.SessionOf(req.Root)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		if sess.Snapshot == nil {
			resp.OK, resp.Error = false, "该会话尚未完成全量识别，无识别全景可读"
			break
		}
		view := panoramaViewOf(sess.Snapshot)
		if req.Section == panoramaSectionFiles {
			// 文件级清单分节：概览小节（目录树/规则态/死路径）不出现在本载荷
			view = RecognitionSnapshot{
				SchemaVersion: view.SchemaVersion, GeneratedAtMs: view.GeneratedAtMs,
				Files: view.Files, Groups: view.Groups, Pending: view.Pending, Stats: view.Stats,
			}
		}
		resp.Panorama = &view
		resp.Decisions = decisionsViewOf(sess)
		resp.Diff = sess.LastDiff
		resp.Domain = domainViewOf(DeriveWorkDomain(sess.Snapshot, ignoredSetOf(sess)))
	case workbenchActionAssign:
		sess, view, err := m.ApplyAssignment(req.Root, req.RelPath, SessionAssignment{
			Author: req.Author, AuthorRole: req.AuthorRole, Title: req.Title,
			Tags: req.Tags, Collection: req.Collection,
		})
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.Assignment = view
	case workbenchActionClearAssignment:
		sess, view, err := m.ApplyAssignment(req.Root, req.RelPath, SessionAssignment{})
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.Assignment = view
	case workbenchActionSetIgnored:
		if _, err := m.SetIgnored(req.Root, req.RelPath, req.Ignored); err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		if sess, err := m.SessionOf(req.Root); err == nil {
			resp.Decisions = decisionsViewOf(sess)
		}
	case workbenchActionGeneralize:
		result, err := m.ApplyGeneralization(req.Root, req.RelPath, req.GeneralizeMode)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Rule = result.Rule
		resp.Started = result.VerificationStarted
	case workbenchActionAdjudicate:
		result, err := m.ApplyAdjudication(req.Root, req.RelPath, MetadataDimension(req.Dimension), adjudicationInput{
			Mode:               req.AdjudicateMode,
			PickIndex:          req.PickIndex,
			PredicateSource:    SourceKind(req.PredicateSource),
			PredicateDimension: MetadataDimension(req.PredicateDimension),
			PresentIndex:       req.PresentIndex,
			AbsentIndex:        req.AbsentIndex,
		})
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Rule = result.Rule
		resp.Started = result.VerificationStarted
	case workbenchActionGetDiff:
		sess, err := m.SessionOf(req.Root)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		if sess.LastDiff == nil {
			// 未重识别过（首轮识别后无差异上下文）：空差异集（非 nil 序列化契约）
			sess.LastDiff = &SnapDiff{
				Changed: []SnapDiffEntry{}, Added: []SnapDiffEntry{}, Removed: []SnapDiffEntry{},
			}
		}
		resp.Diff = sess.LastDiff
	case workbenchActionSetSelection:
		sess, err := m.SetSelection(req.Root, req.Works)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Decisions = decisionsViewOf(sess)
	case workbenchActionCreateTasks:
		outcome, err := m.ApplyCreateTasks(req.Root)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Created = outcome.Works
		resp.CreateMsg = outcome.Message
		if sess, err := m.SessionOf(req.Root); err == nil {
			resp.Decisions = decisionsViewOf(sess)
		}
	case workbenchActionGetRules:
		view, err := m.RulesAdminView()
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulesView = view
	case workbenchActionSaveRule:
		view, err := m.ApplySaveRule(req.Rule)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulesView = view
	case workbenchActionDeleteRule:
		view, err := m.ApplyDeleteRule(req.RuleID)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulesView = view
	case workbenchActionSetRuleEnabled:
		view, err := m.ApplySetRuleEnabled(req.RuleID, req.Enabled)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulesView = view
	case workbenchActionSetBuiltinDisabled:
		view, err := m.ApplySetBuiltinDisabled(req.BuiltinID, req.Disabled)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulesView = view
	case workbenchActionRollbackProfile:
		view, err := m.ApplyRollbackProfile()
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulesView = view
		resp.RolledBack = true
	default:
		resp.OK, resp.Error = false, fmt.Sprintf("未知的工作台请求动作 %q", req.Action)
	}
	m.publishResponse(resp)
}
