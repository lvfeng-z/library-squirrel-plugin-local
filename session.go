// 工作台会话管理：多会话（按库根）的 KV 持久化工件、会话状态机、档案与会话写的
// 单点串行、库根获取与档案外部变更对账，以及工作台前端事件协议（请求/应答/事件）
// 的请求侧处理。识别全景快照、单次赋值与资源选择集等会话内容字段的模型与编排
// 见 recognition.go / parse_actions.go / selection.go。
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
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

// workbenchCreateMarkerKeyPrefix 工作台发起创建标记的外置单键前缀（后接库根
// 标识）：标记置位/消费为 O(1) 单键读写，不经会话工件的全量重解码变异（守卫链
// 瘦身四刀之一）；旧会话信封字段 PendingWorkbenchCreate 只存读侧一轮迁移消费
//（见 ConsumeWorkbenchCreateMarker）。前缀与会话/分片键前缀互不重叠，索引扫描
// 与回退扫描均不会误读。
const workbenchCreateMarkerKeyPrefix = "workbench-create-marker:"

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

// workbenchCreateMarkerKey 库根 → 工作台发起创建标记 KV 键（归一与会话键同规则）。
func workbenchCreateMarkerKey(root string) string {
	return workbenchCreateMarkerKeyPrefix + normalizeLibraryRoot(root)
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

// WorkbenchSession 会话持久化工件：轻字段存于 meta 信封（workbench-session:{root}，
// 见 session_payload.go），重载荷（Snapshot/LastDiff）gzip 压缩分片存于独立分片键组。
// 特异性知识（单次赋值、选择集、识别快照与重识别游标）属会话态；普适规律入规则
// 档案，不在会话内。
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
	// PendingWorkbenchCreate 旧工件字段（已退役）：工作台发起创建标记已外置为
	// 独立 KV 键（workbench-create-marker:{库根}，O(1) 单键读写，BeginWorkbenchCreate/
	// ConsumeWorkbenchCreateMarker）。新代码不再置位本字段，仅存读侧一轮迁移
	// 消费——旧信封/旧整值读到 true 时消费并清除（方案风险4 兼容锚定）。
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
	// IgnoredCandidates 已忽略的集名候选目标键（Source+Dir+Key 稳定串，阶段5）：
	// 忽略=会话态，重识别时对应候选移出快照主清单进 ignoredCandidates 分仓。
	IgnoredCandidates []string `json:"ignoredCandidates,omitempty"`
	// ConfirmedCandidates 已确认的集名候选目标键：确认动作对成员逐文件写集
	// 维度赋值后登记，重识别时对应候选进快照 confirmedCandidates 分仓（带
	// 确认集名与既有集行 id）。
	ConfirmedCandidates []string `json:"confirmedCandidates,omitempty"`
	// ConfirmedSetIDs 确认集名 → 库内既有集行 id 的旁路簿记（existingSetID>0
	// 的确认才记）：快照 confirmedCandidates 分仓的既有 id 呈现数据源。行 id
	// 不作建任务的 SiteWorkSetId 业务键——宿主按 (site_id, site_work_set_id)
	// 业务键 upsert，行 id 字符串命中不到既有行而会另造新行（语义核验结论见
	// work_fetch.go DimCollection 注释）。
	ConfirmedSetIDs map[string]int64 `json:"confirmedSetIds,omitempty"`
	// WorkSetRuleLineage 集→生成规则族的谱系簿记（target 键 → 该集经泛化/
	// 调整面保存入档的规则 ID 集）：同一集再次入档=修订意图，commitWorkSetRules
	// 先停用旧族再入档新族（旧规则不再产出旧值遮蔽新值）。会话态工件——
	// 会话重建后簿记缺席，退化为并存（捕获组按规则隔离保证互不窃取，仅同
	// 维度声明竞争进待裁决）。
	WorkSetRuleLineage map[string][]string `json:"workSetRuleLineage,omitempty"`
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

	// createCarryMu 保护工作台创建的进程内传值槽（任务名暂存与批次摘要回写；
	// 语义与用法见 selection.go「创建编排与过滤产出之间的进程内传值」——不落
	// 会话存储，重启即弃）。
	createCarryMu   sync.Mutex
	stagedTaskNames map[string]string             // 库根 → 编排动作暂存的创建任务名原文
	batchSummaries  map[string]createBatchSummary // 库根 → 过滤产出回写的批次摘要

	// sessCacheMu 已解码会话缓存的叶锁：只保护 sessCache 映射与条目字段的
	// 读写，持锁期间绝不调用其他会话方法（锁层级=单向嵌套 m.mu → sessCacheMu
	// → 无：写路径 saveSession 在 m.mu 内更新缓存，读路径 cachedSession 只取
	// sessCacheMu 不取 m.mu，workDomainOf 先释放 sessCacheMu 再取 domainMu，
	// 无反向路径故无死锁）。
	sessCacheMu sync.Mutex
	// sessCache 归一化库根 → 已解码会话缓存条目（读路径共享只读实例；写路径
	// 一致性铁律见 putSessionCache）。
	sessCache map[string]*sessionCacheEntry
	// sessCacheNow 时钟注入点（空闲 TTL 判定；单测覆写驱动过期，nil=time.Now）。
	sessCacheNow func() time.Time
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

// saveSession 轻保存（调用方须持 mu）：只写 meta 信封 + 索引——载荷簿记字段
// （shardSet/shards/codec/digest）沿用当前 meta，重载荷留在旧批次分片键组中，
// 赋值/确认/选择集等会话级变更不再重写整个快照载荷。
//
// ⚠ 轻/全保存划分纪律（方案自曝风险4）：本入口绝不可承载快照/差异更新——轻保存
// 不触碰分片，shards>0 时内存中对 Snapshot/LastDiff 的改动会静默丢失（无法在不
// 重编码载荷的前提下检出内存改动）。触碰 Snapshot/LastDiff 的唯一全保存入口=
// CompleteRecognitionWithDiff（经 mutateSessionFull/saveSessionFull）；ReplaceSession
// 是重建语义（删除旧工件后轻保存新会话），不属更新。新增入口若需改快照，必须走
// mutateSessionFull——误走轻保存由「测试锚定：轻保存不触分片」的单测护栏暴露。
//
// 升级规则：内存载荷在场（Snapshot/LastDiff 非 nil）而当前 meta 无可用载荷簿记
// （旧格式整值、meta 缺席或信封 shards=0）时，升级为 saveSessionFull——旧格式
// 会话「读到即转新格式」在此落笔（首次保存即写分片），并对 shards=0 场景兜住
// 误走轻保存的快照更新（升级后仍写入载荷而非丢弃）。
func (m *SessionManager) saveSession(sess *WorkbenchSession) error {
	sess.SchemaVersion = workbenchSessionSchemaVersion
	env, _, err := m.readSessionEnvelope(sess.RootPath)
	if err != nil {
		return fmt.Errorf("读取旧会话（键 %q）失败: %w", workbenchSessionKey(sess.RootPath), err)
	}
	if (sess.Snapshot != nil || sess.LastDiff != nil) && (env == nil || env.Shards <= 0) {
		return m.saveSessionFull(sess)
	}
	if err := m.writeSessionEnvelope(sess, env); err != nil {
		return err
	}
	m.putSessionCache(sess) // 写路径一致性：保存成功即重建缓存项
	return nil
}

// writeSessionEnvelope 写 meta 信封（轻保存共用段，调用方须持 mu）：载荷簿记
// 字段（shardSet/shards/codec/digest）沿用入参 env（本方法不重读旧 meta），重
// 载荷留在旧批次分片键组中。不触碰缓存——缓存刷新由调用方按路径负责：全会话
// 保存走 putSessionCache（实例含载荷），信封级轻变异走 refreshSessionCacheEnvelope
//（外层重建+载荷指针共享，见 mutateSessionEnvelope）。
func (m *SessionManager) writeSessionEnvelope(sess *WorkbenchSession, env *sessionEnvelope) error {
	sess.SchemaVersion = workbenchSessionSchemaVersion
	saving := &sessionEnvelope{V: sessionEnvelopeVersion, Session: sessionLightCopy(sess)}
	if env != nil {
		saving.ShardSet, saving.Shards = env.ShardSet, env.Shards
		saving.Codec, saving.Digest = env.Codec, env.Digest
	}
	data, err := json.Marshal(saving)
	if err != nil {
		return fmt.Errorf("会话信封序列化失败: %w", err)
	}
	if err := m.store.SetValue(workbenchSessionKey(sess.RootPath), string(data)); err != nil {
		return fmt.Errorf("写入会话（键 %q）失败: %w", workbenchSessionKey(sess.RootPath), err)
	}
	if err := m.ensureSessionIndexed(normalizeLibraryRoot(sess.RootPath)); err != nil {
		m.warnf("会话索引登记失败（root=%s）: %v", normalizeLibraryRoot(sess.RootPath), err)
	}
	return nil
}

// saveSessionFull 全保存（调用方须持 mu；快照/差异变化时）：载荷 {snapshot,
// lastDiff} → gzip → 3MB 边界切片 → 逐片写全新批次键组（不覆写旧片）→ 全部
// 成功后写 meta 信封（原子切换点）→ 删除旧批次全部分片键（旧批次取自替换前的
// 旧 meta）→ 索引补登。任一分片写失败即中止返回错误——旧 meta 未动，会话维持
// 旧快照可读；已写成的分片成为残留垃圾键，不影响正确性（方案自曝风险3）。
func (m *SessionManager) saveSessionFull(sess *WorkbenchSession) error {
	sess.SchemaVersion = workbenchSessionSchemaVersion
	compressed, digest, err := marshalSessionPayload(sess.Snapshot, sess.LastDiff)
	if err != nil {
		return fmt.Errorf("会话载荷序列化失败: %w", err)
	}
	oldEnv, _, err := m.readSessionEnvelope(sess.RootPath)
	if err != nil {
		return fmt.Errorf("读取旧会话（键 %q）失败: %w", workbenchSessionKey(sess.RootPath), err)
	}
	norm := normalizeLibraryRoot(sess.RootPath)
	oldBatch := int64(0)
	if oldEnv != nil {
		oldBatch = parseSessionShardBatch(oldEnv.ShardSet)
	}
	batch := nextSessionShardBatch(oldBatch)
	shards := splitSessionShards(compressed)
	for i, chunk := range shards {
		key := sessionShardKey(norm, batch, i)
		if err := m.store.SetValue(key, base64.StdEncoding.EncodeToString(chunk)); err != nil {
			return fmt.Errorf("写入会话分片（键 %q）失败: %w", key, err)
		}
	}
	env := &sessionEnvelope{
		V: sessionEnvelopeVersion, Session: sessionLightCopy(sess),
		ShardSet: batch, Shards: len(shards),
		Codec: sessionPayloadCodecGzip, Digest: digest,
	}
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("会话信封序列化失败: %w", err)
	}
	if err := m.store.SetValue(workbenchSessionKey(sess.RootPath), string(data)); err != nil {
		return fmt.Errorf("写入会话（键 %q）失败: %w", workbenchSessionKey(sess.RootPath), err)
	}
	m.deleteStaleShards(norm, oldEnv, batch)
	if err := m.ensureSessionIndexed(norm); err != nil {
		m.warnf("会话索引登记失败（root=%s）: %v", norm, err)
	}
	m.putSessionCache(sess) // 写路径一致性：保存成功即重建缓存项
	return nil
}

// loadSession 读单键会话（锁外读亦安全：只读本会话的键组）；未写入返回
// ErrWorkbenchSessionNotFound，载荷损坏或版本不符按未命中处理并告警——含分片
// digest 校验不符（全保存半写窗口的兜底语义：按未命中处理重新识别，同方案自曝
// 风险3）。旧格式「单键单 JSON」整值兼容可读（读到即转：下次保存升级为信封+
// 分片形态）。
func (m *SessionManager) loadSession(root string) (*WorkbenchSession, error) {
	key := workbenchSessionKey(root)
	stored, err := m.store.GetValue(key)
	if err != nil {
		return nil, fmt.Errorf("读取会话（键 %q）失败: %w", key, err)
	}
	if stored == nil || stored.Value == "" {
		return nil, ErrWorkbenchSessionNotFound
	}
	if probeSessionEnvelope(stored.Value) {
		return m.loadSessionFromEnvelope(root, key, stored.Value)
	}
	var sess WorkbenchSession
	if err := json.Unmarshal([]byte(stored.Value), &sess); err != nil {
		m.warnf("会话载荷无法解析，按未命中处理（键 %q）: %v", key, err)
		return nil, ErrWorkbenchSessionNotFound
	}
	if sess.SchemaVersion != workbenchSessionSchemaVersion {
		m.warnf("会话结构版本 %d 与当前支持版本 %d 不符，按未命中处理（键 %q）",
			sess.SchemaVersion, workbenchSessionSchemaVersion, key)
		return nil, ErrWorkbenchSessionNotFound
	}
	return &sess, nil
}

// loadSessionFromEnvelope 从信封读会话：无快照会话（shards=0）直接取轻字段；
// 否则逐片读 → 拼接 → digest 比对（不符=告警+未命中）→ 解压反序列化合入。
func (m *SessionManager) loadSessionFromEnvelope(root, key, raw string) (*WorkbenchSession, error) {
	env, err := parseSessionEnvelope(raw)
	if err != nil || env.Session == nil {
		m.warnf("会话信封无法解析，按未命中处理（键 %q）: %v", key, err)
		return nil, ErrWorkbenchSessionNotFound
	}
	if env.V != sessionEnvelopeVersion {
		m.warnf("会话信封版本 %d 与当前支持版本 %d 不符，按未命中处理（键 %q）",
			env.V, sessionEnvelopeVersion, key)
		return nil, ErrWorkbenchSessionNotFound
	}
	sess := *env.Session
	if sess.SchemaVersion != workbenchSessionSchemaVersion {
		m.warnf("会话结构版本 %d 与当前支持版本 %d 不符，按未命中处理（键 %q）",
			sess.SchemaVersion, workbenchSessionSchemaVersion, key)
		return nil, ErrWorkbenchSessionNotFound
	}
	if env.Shards <= 0 {
		return &sess, nil
	}
	if env.ShardSet == "" || env.Digest == "" || env.Codec != sessionPayloadCodecGzip {
		m.warnf("会话信封载荷簿记不完整（shards=%d, codec=%q），按未命中处理（键 %q）",
			env.Shards, env.Codec, key)
		return nil, ErrWorkbenchSessionNotFound
	}
	norm := normalizeLibraryRoot(root)
	compressed := make([]byte, 0, env.Shards*sessionShardRawUnit)
	for i := 0; i < env.Shards; i++ {
		shardKey := sessionShardKey(norm, env.ShardSet, i)
		shardStored, serr := m.store.GetValue(shardKey)
		if serr != nil {
			return nil, fmt.Errorf("读取会话分片（键 %q）失败: %w", shardKey, serr)
		}
		if shardStored == nil || shardStored.Value == "" {
			m.warnf("会话分片缺失（键 %q），按未命中处理（可能为半写残留）", shardKey)
			return nil, ErrWorkbenchSessionNotFound
		}
		chunk, derr := base64.StdEncoding.DecodeString(shardStored.Value)
		if derr != nil {
			m.warnf("会话分片无法解码（键 %q），按未命中处理: %v", shardKey, derr)
			return nil, ErrWorkbenchSessionNotFound
		}
		compressed = append(compressed, chunk...)
	}
	sum := sha256.Sum256(compressed)
	if hex.EncodeToString(sum[:]) != env.Digest {
		m.warnf("会话载荷 digest 校验不符，按未命中处理（键 %q）", key)
		return nil, ErrWorkbenchSessionNotFound
	}
	payload, perr := unmarshalSessionPayload(compressed)
	if perr != nil {
		m.warnf("会话载荷无法解压反序列化，按未命中处理（键 %q）: %v", key, perr)
		return nil, ErrWorkbenchSessionNotFound
	}
	sess.Snapshot, sess.LastDiff = payload.Snapshot, payload.LastDiff
	return &sess, nil
}

// —— 已解码会话缓存（读路径加速：getPanorama 每页不再全量重载快照）——
//
// 缓存内容=已解码 *WorkbenchSession（轻字段+Snapshot+LastDiff），键=归一化库根，
// 进程内驻留。无缓存时每次读会话都走「逐片 store.GetValue + base64 + sha256 +
// gunzip + 整包 JSON 反序列化」的全量重载——getPanorama files/domain 分节的全链
// ~45-50 页每页重复付这笔全量成本（实测真实库 20.7 万文件的主要载入瓶颈之一）。
//
// 别名陷阱防线（方案 a：变异旁路）：缓存条目内的实例是**共享只读实例**——
// SessionOf/cachedSession 与全部只读校验前置读用它；一切变异路径（mutateSession /
// mutateSessionFull / ConsumeWorkbenchCreateMarker / runFullRecognition）走
// loadSessionFresh 旁路（绕过缓存直读存储，返回私有实例；缓存失效由保存路径
// putSessionCache 单点负责），绝不把缓存实例交给变异方。除「saveSession 成功时把已保存实例放入缓存」外，任何代码不得写缓存
// 实例的字段（快照亦然——识别聚合对 prev 群成员有转置排序/markCommonElements
// 的共享底层数组改写，故识别主体必须整段走旁路）。世代兜底：即使某条缓存项
// 失效被漏检，getPanorama 的游标世代校验（Snapshot.GeneratedAtMs，见
// decodePanoramaCursor）会让过期页以「世代变更，请整轮重拉」自曝，不会静默
// 错答。
//
// 回收：空闲 TTL（sessionCacheTTL）+ 容量 LRU 上限（sessionCacheCapacity，
// 防多库切换驻留堆积）；真实大库单会话解码态约数百 MB，TTL 过期后即不可达
// 由 GC 回收。
const (
	// sessionCacheTTL 缓存条目空闲逐出时限（最近一次命中后未被读的时长）。
	sessionCacheTTL = 10 * time.Minute
	// sessionCacheCapacity 缓存库根数上限（LRU：超出逐出最久未用者）。
	sessionCacheCapacity = 2
)

// sessionCacheEntry 单库根的缓存条目。
type sessionCacheEntry struct {
	// sess 共享只读会话实例（防线纪律见上：入缓存后任何路径不得再写字段）。
	sess *WorkbenchSession
	// domainMu 保护 domain 的惰性推导（推导 O(全库) 毫秒-秒级，不得持
	// sessCacheMu 长跑）；domain 与 sess 同生命周期——缓存项被替换/逐出即
	// 整体作废（世代与忽略集随 sess 一并失效）。
	domainMu sync.Mutex
	domain   *WorkDomain
	// lastUsed 最近一次命中（或写入）时点，TTL 与 LRU 判定基准。
	lastUsed time.Time
}

// cacheNow 缓存时钟（注入点；缺省墙钟）。
func (m *SessionManager) cacheNow() time.Time {
	if m.sessCacheNow != nil {
		return m.sessCacheNow()
	}
	return time.Now()
}

// cachedSession 读路径取会话：缓存命中（未过 TTL）零存储读直接返回共享只读
// 实例；未命中走 loadSession 全量解码后装填。并发未命中可各自解码、后写覆盖
// 先写（两者均为当前存储真值，浪费一次解码但不损正确性）。
func (m *SessionManager) cachedSession(root string) (*WorkbenchSession, error) {
	norm := normalizeLibraryRoot(root)
	now := m.cacheNow()
	m.sessCacheMu.Lock()
	if entry, ok := m.sessCache[norm]; ok {
		if now.Sub(entry.lastUsed) < sessionCacheTTL {
			entry.lastUsed = now
			m.sessCacheMu.Unlock()
			return entry.sess, nil
		}
		delete(m.sessCache, norm) // 空闲超时逐出
	}
	m.sessCacheMu.Unlock()

	sess, err := m.loadSession(root) // 锁外解码：重载荷解压反序列化可秒级
	if err != nil {
		return nil, err
	}
	m.sessCacheMu.Lock()
	if m.sessCache == nil {
		m.sessCache = make(map[string]*sessionCacheEntry)
	}
	m.sessCache[norm] = &sessionCacheEntry{sess: sess, lastUsed: now}
	m.trimSessionCacheLocked(now)
	m.sessCacheMu.Unlock()
	return sess, nil
}

// loadSessionFresh 变异旁路读：绕过缓存直读存储，返回**私有实例**——调用方可在
// 其上原地改字段再经 saveSession/saveSessionFull 落库（落库成功由 putSessionCache
// 重建缓存项），不会污染缓存共享实例。不做事前逐出：缓存失效由保存路径单点
// 负责（变异失败不落库=存储与缓存均未变；变异成功=保存重建条目），事前逐出只会
// 把变异窗口内的读者赶去存储侧，撞上全保存「新 meta 已写、旧批次分片正删」切换
// 瞬态的既有竞态（窗口极窄、真实频率为每识别轮一次，维持原样不追修）。
func (m *SessionManager) loadSessionFresh(root string) (*WorkbenchSession, error) {
	return m.loadSession(root) // 私有实例（全量解码）；缓存交互只在保存侧
}

// evictSessionCache 逐出单库根缓存项（无项为幂等空转）。
func (m *SessionManager) evictSessionCache(norm string) {
	m.sessCacheMu.Lock()
	delete(m.sessCache, norm)
	m.sessCacheMu.Unlock()
}

// putSessionCache 保存成功后的缓存重建（写路径一致性铁律：saveSession/
// saveSessionFull 每次成功必达此处——识别轮保存/赋值与解析动作保存/建会话全
// 覆盖；ReplaceSession/DeleteSession 经 deleteSessionArtifactsLocked 的入口逐出
// 覆盖）。存入的是刚落库的实例：调用方此后不得再改其字段（变异一律重走旁路）。
func (m *SessionManager) putSessionCache(sess *WorkbenchSession) {
	norm := normalizeLibraryRoot(sess.RootPath)
	now := m.cacheNow()
	m.sessCacheMu.Lock()
	defer m.sessCacheMu.Unlock()
	if m.sessCache == nil {
		m.sessCache = make(map[string]*sessionCacheEntry)
	}
	m.sessCache[norm] = &sessionCacheEntry{sess: sess, lastUsed: now}
	m.trimSessionCacheLocked(now)
}

// trimSessionCacheLocked 缓存修剪（调用方须持 sessCacheMu）：先逐空闲超时项，
// 再按 LRU（lastUsed 最旧者）收到容量上限。
func (m *SessionManager) trimSessionCacheLocked(now time.Time) {
	for key, entry := range m.sessCache {
		if now.Sub(entry.lastUsed) >= sessionCacheTTL {
			delete(m.sessCache, key)
		}
	}
	for len(m.sessCache) > sessionCacheCapacity {
		oldestKey, oldest, first := "", time.Time{}, true
		for key, entry := range m.sessCache {
			if first || entry.lastUsed.Before(oldest) {
				oldestKey, oldest, first = key, entry.lastUsed, false
			}
		}
		delete(m.sessCache, oldestKey)
	}
}

// workDomainOf 选择域求值：会话是缓存共享实例时复用条目内的惰性推导结果
//（推导 O(全库)——getPanorama domain 分节全链逐页共摊一次），否则（变异旁路
// 私有实例/测试直驱会话）现推导。返回值按只读消费。
func (m *SessionManager) workDomainOf(sess *WorkbenchSession) *WorkDomain {
	if entry := m.cachedEntryOf(sess); entry != nil {
		entry.domainMu.Lock()
		defer entry.domainMu.Unlock()
		if entry.domain == nil {
			entry.domain = DeriveWorkDomain(entry.sess.Snapshot, ignoredSetOf(entry.sess))
		}
		return entry.domain
	}
	return DeriveWorkDomain(sess.Snapshot, ignoredSetOf(sess))
}

// cachedEntryOf 会话对应的缓存条目（仅当指针同一于缓存共享实例时命中——
// 私有实例不入缓存，恒 miss）。
func (m *SessionManager) cachedEntryOf(sess *WorkbenchSession) *sessionCacheEntry {
	m.sessCacheMu.Lock()
	defer m.sessCacheMu.Unlock()
	entry := m.sessCache[normalizeLibraryRoot(sess.RootPath)]
	if entry == nil || entry.sess != sess {
		return nil
	}
	return entry
}

// SessionOf 按库根取会话（读路径：进程内缓存命中零存储读；返回共享只读实例，
// 调用方不得原地改其字段——变异走 mutateSession 族）。
func (m *SessionManager) SessionOf(root string) (*WorkbenchSession, error) {
	return m.cachedSession(root)
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

	if existing, err := m.cachedSession(root); err == nil { // 只读存在性校验：缓存读路径
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

// ReplaceSession 确认覆盖重建：删除既有会话全部工件（meta+全部分片+索引登记）
// 后原库根重建（既有会话态不保留）。重建不属快照更新——新会话无快照，走轻保存；
// 旧分片随工件删除清理（轻/全保存划分见 saveSession 纪律注释）。
func (m *SessionManager) ReplaceSession(root string) (*WorkbenchSession, error) {
	m.mu.Lock()
	err := m.deleteSessionArtifactsLocked(root)
	m.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return m.CreateSession(root)
}

// DeleteSession 删除会话（幂等：会话不存在视为成功）：读 meta 取旧批次清单 →
// 删全部分片 → 删 meta → 索引移除。分片删除失败仅告警（残留垃圾键不影响正确性，
// 方案自曝风险3）；meta 删除失败返回错误（会话仍可读，删除未完成）。
func (m *SessionManager) DeleteSession(root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.deleteSessionArtifactsLocked(root); err != nil {
		return err
	}
	m.logf("已删除解析会话: root=%s", root)
	return nil
}

// deleteSessionArtifactsLocked 删除会话全部工件（调用方须持 mu）。入口先逐出
// 缓存项——工件一旦开删，缓存实例与存储真值可能分叉（分片删除失败仅告警的
// 半删态），不再可服务。
func (m *SessionManager) deleteSessionArtifactsLocked(root string) error {
	norm := normalizeLibraryRoot(root)
	m.evictSessionCache(norm)
	env, _, err := m.readSessionEnvelope(root)
	if err != nil {
		return fmt.Errorf("读取旧会话（键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	if env != nil {
		for i := 0; i < env.Shards; i++ {
			shardKey := sessionShardKey(norm, env.ShardSet, i)
			if derr := m.store.DeleteValue(shardKey); derr != nil {
				m.warnf("删除会话分片失败（键 %q，残留垃圾不影响正确性）: %v", shardKey, derr)
			}
		}
	}
	if err := m.store.DeleteValue(workbenchSessionKey(root)); err != nil {
		return fmt.Errorf("删除会话（键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	m.unregisterSessionIndexed(norm)
	return nil
}

// ListSessions 枚举全部会话，按最近更新时间倒序。优先走会话索引：索引命中 →
// 逐根读 meta 仅取轻字段（列表呈现只用 RootPath/State/CreatedAtMs/UpdatedAtMs，
// 分片载荷不加载）；某根 meta 缺失/损坏=跳过+告警+索引修剪重写。索引缺失/损坏 →
// GetAllValues 回退扫描（排除分片前缀）→ 成功后重建索引写回。
func (m *SessionManager) ListSessions() ([]WorkbenchSession, error) {
	roots, indexed := readSessionIndexRoots(m.store)
	if !indexed {
		all, err := m.store.GetAllValues()
		if err != nil {
			return nil, fmt.Errorf("枚举插件 KV 失败: %w", err)
		}
		roots = scanSessionRootsFromValues(all)
		if err := writeSessionIndex(m.store, roots); err != nil {
			m.warnf("会话索引重建写回失败: %v", err)
		}
	}
	sessions := make([]WorkbenchSession, 0, len(roots))
	kept := make([]string, 0, len(roots))
	pruned := false
	for _, normRoot := range roots {
		sess, err := m.listSessionOfRoot(normRoot)
		if err != nil {
			return nil, err
		}
		if sess == nil {
			pruned = true
			continue
		}
		kept = append(kept, normRoot)
		sessions = append(sessions, *sess)
	}
	if indexed && pruned {
		if err := writeSessionIndex(m.store, kept); err != nil {
			m.warnf("会话索引修剪重写失败: %v", err)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAtMs != sessions[j].UpdatedAtMs {
			return sessions[i].UpdatedAtMs > sessions[j].UpdatedAtMs
		}
		return sessions[i].RootPath < sessions[j].RootPath
	})
	return sessions, nil
}

// listSessionOfRoot 读单根 meta 仅取轻字段（列表投影，不读分片载荷）。meta 缺失/
// 损坏/版本不符返回 nil（调用方跳过+修剪索引）；读取传输层失败返回错误（整体
// 枚举不可靠，不静默修剪）。
func (m *SessionManager) listSessionOfRoot(normRoot string) (*WorkbenchSession, error) {
	key := workbenchSessionKeyPrefix + normRoot
	stored, err := m.store.GetValue(key)
	if err != nil {
		return nil, fmt.Errorf("读取会话（键 %q）失败: %w", key, err)
	}
	if stored == nil || stored.Value == "" {
		m.warnf("会话 meta 缺失，跳过并自索引修剪（键 %q）", key)
		return nil, nil
	}
	if probeSessionEnvelope(stored.Value) {
		env, perr := parseSessionEnvelope(stored.Value)
		if perr != nil || env.Session == nil || env.V != sessionEnvelopeVersion ||
			env.Session.SchemaVersion != workbenchSessionSchemaVersion {
			m.warnf("会话信封损坏或版本不符，跳过并自索引修剪（键 %q）", key)
			return nil, nil
		}
		return env.Session, nil
	}
	// 旧格式整值：整值反序列化（快照内联其中，列表只消费轻字段）。
	var sess WorkbenchSession
	if uerr := json.Unmarshal([]byte(stored.Value), &sess); uerr != nil {
		m.warnf("会话载荷无法解析，跳过并自索引修剪（键 %q）: %v", key, uerr)
		return nil, nil
	}
	if sess.SchemaVersion != workbenchSessionSchemaVersion {
		m.warnf("会话结构版本 %d 不符，跳过并自索引修剪（键 %q）", sess.SchemaVersion, key)
		return nil, nil
	}
	return &sess, nil
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

// mutateSession 轻保存入口：锁内会话读改写（单点串行）；mutate 返回错误即中止
// 不落库。落库只写 meta 信封+索引（载荷簿记沿用旧 meta，见 saveSession）。读侧
// 为**全会话解码**（含载荷分片，秒级）——只动轻字段的高频变异（如创建成功后的
// 选择集清理）应走 mutateSessionEnvelope（信封级通路，不触大载荷）。
//
// ⚠ 轻/全保存划分纪律（方案自曝风险4，护栏=「轻保存不触分片」单测）：本入口的
// mutate 闭包**禁止修改 sess.Snapshot 与 sess.LastDiff**——轻保存不重写载荷分片，
// 快照改动会被静默丢弃。既有轻入口清单（全部不触快照/差异）：
//   - session.go：MarkRecognized（状态迁移）、CreateSession（新建）、
//     SetAssignment / SetIgnored（赋值与忽略）、SetCandidateIgnored /
//     ConfirmWorkSetCandidate / UnconfirmWorkSetCandidate（集名候选族）；
//   - parse_actions.go：ApplyDirAssignment / ClearDirAssignments（目录赋值族）、
//     markScopedAssignments（泛化标记）；
//   - selection.go：SetSelection（选择集）；
//   - workset_rule.go：集规则谱系簿记。
//
// 已外置/改走路由的入口：
//   - BeginWorkbenchCreate / ConsumeWorkbenchCreateMarker（创建标记）——独立 KV
//     单键 O(1) 读写，不再触会话工件（旧信封字段仅读侧一轮迁移消费）；
//   - 创建成功后的选择集清理（runCreateTasks）与旧标记迁移清除——走
//     mutateSessionEnvelope（信封级轻变异，禁触快照的护栏同上）。
//
// 触碰快照/差异的入口唯有 CompleteRecognitionWithDiff（走 mutateSessionFull）；
// ReplaceSession 为重建语义（删除旧工件+轻保存新会话）。新增改快照的入口必须走
// mutateSessionFull，否则请更新本清单并补对应单测。
func (m *SessionManager) mutateSession(root string, mutate func(*WorkbenchSession) error) (*WorkbenchSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mutateSessionFullDecodeLocked(root, mutate)
}

// mutateSessionFullDecodeLocked 全会话解码变异的锁内体（调用方须持 mu；语义与
// 原 mutateSession 一致）：变异旁路私有实例 → 变异 → 轻保存。
func (m *SessionManager) mutateSessionFullDecodeLocked(root string, mutate func(*WorkbenchSession) error) (*WorkbenchSession, error) {
	sess, err := m.loadSessionFresh(root) // 变异旁路：私有实例，缓存不外泄
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

// mutateSessionEnvelope 信封级轻变异入口（守卫链与会话瘦身四刀之二）：锁内
// 「读 meta 信封（单键小读，不触载荷分片）→ 变异 → 轻保存」。适用只动轻字段
// 的高频变异（创建成功后的选择集清理、旧创建标记迁移清除）——不再付全会话
// 重解码（真实库大载荷解压反序列化秒级）的代价。护栏：mutate 触碰 Snapshot/
// LastDiff 即报错中止（轻保存不重写分片，静默丢弃是数据损坏）——与 mutateSession
// 的纪律清单同源，快照改动必须走 mutateSessionFull。
//
// 旧格式整值（信封化之前的单键单 JSON 会话）无独立信封可走，回落全会话路径
//（读到即转：本次保存升级为信封+分片形态）。返回实例为信封轻字段投影（载荷
// 字段恒 nil）——需要载荷的读侧走 SessionOf（缓存/全量解码装填）。
func (m *SessionManager) mutateSessionEnvelope(root string, mutate func(*WorkbenchSession) error) (*WorkbenchSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mutateSessionEnvelopeLocked(root, mutate)
}

// mutateSessionEnvelopeLocked 信封级轻变异的锁内体（调用方须持 mu）。
func (m *SessionManager) mutateSessionEnvelopeLocked(root string, mutate func(*WorkbenchSession) error) (*WorkbenchSession, error) {
	norm := normalizeLibraryRoot(root)
	env, raw, err := m.readSessionEnvelope(root)
	if err != nil {
		return nil, fmt.Errorf("读取会话（键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	if env == nil || env.Session == nil || env.V != sessionEnvelopeVersion ||
		env.Session.SchemaVersion != workbenchSessionSchemaVersion {
		if env == nil && raw == "" {
			return nil, ErrWorkbenchSessionNotFound
		}
		// 旧格式整值/信封损坏或版本不符：回落全会话路径（沿用 loadSession 的
		// 判定与告警口径——损坏按未命中处理）
		return m.mutateSessionFullDecodeLocked(root, mutate)
	}
	sess := *env.Session // 轻字段投影（Snapshot/LastDiff 恒 nil）
	if err := mutate(&sess); err != nil {
		return nil, err
	}
	if sess.Snapshot != nil || sess.LastDiff != nil {
		return nil, errors.New("信封级变异禁止触碰快照/差异（轻保存不重写载荷分片，改动会被静默丢弃），须走 mutateSessionFull")
	}
	sess.UpdatedAtMs = time.Now().UnixMilli()
	if err := m.writeSessionEnvelope(&sess, env); err != nil {
		return nil, err
	}
	m.refreshSessionCacheEnvelope(norm, &sess)
	return &sess, nil
}

// refreshSessionCacheEnvelope 信封级轻保存后的缓存刷新（调用方须持 mu 且保存已
// 成功）：**外层重建**缓存条目——轻字段取信封新值，载荷指针自旧条目只读共享
//（沿用共享纪律：入缓存后任何路径不得再写字段；绝不改写旧条目实例，防缓存条
// 目与信封双写漂移）。domain 惰性推导随条目重建整体作废（忽略集等影响推导的
// 轻字段可能已变，重推导由下次使用摊销）。旧条目缺席（缓存未装填）时不入新
// 条目——无载荷共享源，宁缺勿错（下次读走全量解码装填），防无载荷条目被读
// 路径当作「未识别会话」误答。
func (m *SessionManager) refreshSessionCacheEnvelope(norm string, light *WorkbenchSession) {
	m.sessCacheMu.Lock()
	defer m.sessCacheMu.Unlock()
	prev, ok := m.sessCache[norm]
	if !ok || prev.sess == nil {
		return
	}
	rebuilt := *light
	rebuilt.Snapshot, rebuilt.LastDiff = prev.sess.Snapshot, prev.sess.LastDiff // 只读共享
	m.sessCache[norm] = &sessionCacheEntry{sess: &rebuilt, lastUsed: m.cacheNow()}
}

// mutateSessionFull 全保存入口：锁内会话读改写后按重载荷全量落库（gzip 分片+
// 批次原子切换，见 saveSessionFull）。生产调用方唯一=CompleteRecognitionWithDiff
// （识别完成/重识别覆写快照与差异）；其余入口禁止经此写会话级轻字段（徒增整个
// 载荷的重写成本）。
func (m *SessionManager) mutateSessionFull(root string, mutate func(*WorkbenchSession) error) (*WorkbenchSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, err := m.loadSessionFresh(root) // 变异旁路：私有实例，缓存不外泄
	if err != nil {
		return nil, err
	}
	if err := mutate(sess); err != nil {
		return nil, err
	}
	sess.UpdatedAtMs = time.Now().UnixMilli()
	if err := m.saveSessionFull(sess); err != nil {
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
// （识别时点基准，后续档案外部变更仍可对账检出）。生产路径经
// CompleteRecognitionWithDiff（携带验证轮差异集）。
func (m *SessionManager) CompleteRecognition(root string, snapshot *RecognitionSnapshot) (*WorkbenchSession, error) {
	return m.CompleteRecognitionWithDiff(root, snapshot, nil)
}

// CompleteRecognitionWithDiff 全量识别完成落库：快照、状态迁移与重识别差异集
// 在锁内单次写完成（差异集随每次重识别覆写——首轮识别无前序快照，差异置空）。
// 触碰 Snapshot/LastDiff 的唯一全保存入口（经 mutateSessionFull，轻/全划分纪律
// 见 mutateSession 注释）。
func (m *SessionManager) CompleteRecognitionWithDiff(root string, snapshot *RecognitionSnapshot, diff *SnapDiff) (*WorkbenchSession, error) {
	if snapshot == nil {
		return nil, errors.New("识别快照为空，无法落库")
	}
	return m.mutateSessionFull(root, func(sess *WorkbenchSession) error {
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
// Create 据此分派过滤产出分支）：独立 KV 单键 O(1) 写——外置自会话工件，不再付
// 全会话重解码变异的秒级成本（守卫链与会话瘦身四刀之一）。不校验会话存在性
//（调用方 ApplyCreateTasks 已在守卫链读得会话；未消费的孤立标记由后续消费自然
// 清除）。旧信封字段不置位（其读侧迁移见 ConsumeWorkbenchCreateMarker）。
func (m *SessionManager) BeginWorkbenchCreate(root string) error {
	key := workbenchCreateMarkerKey(root)
	if err := m.store.SetValue(key, "1"); err != nil {
		return fmt.Errorf("置位工作台发起创建标记（键 %q）失败: %w", key, err)
	}
	return nil
}

// ConsumeWorkbenchCreateMarker 消费工作台发起创建标记：在位则清除并返回真
// （Create 过滤产出分支的分派判据，消费语义防同一标记被后续创建重复命中）。
// 外置单键缺席时读侧迁移消费一轮旧会话信封字段（方案风险4：外置化升级前置位
// 的标记经旧字段读出——消费后经信封级轻保存清除，此后恒走外置键）。
func (m *SessionManager) ConsumeWorkbenchCreateMarker(root string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := workbenchCreateMarkerKey(root)
	stored, err := m.store.GetValue(key)
	if err != nil {
		m.warnf("读取工作台发起创建标记失败（键 %q）: %v", key, err)
		return false
	}
	if stored != nil && stored.Value != "" {
		if derr := m.store.DeleteValue(key); derr != nil {
			m.warnf("清除工作台发起创建标记失败（键 %q）: %v", key, derr)
			return false
		}
		return true
	}
	return m.consumeLegacyWorkbenchCreateMarker(root)
}

// errCreateMarkerLegacyAbsent 旧信封字段迁移消费的「字段不在位」中止信号（与
// 传输/存储错误区分：前者是正常路径，后者按消费失败处理）。
var errCreateMarkerLegacyAbsent = errors.New("旧信封创建标记不在位")

// consumeLegacyWorkbenchCreateMarker 旧信封字段迁移消费（恰一轮，调用方须持
// mu）：PendingWorkbenchCreate=true 时经信封级轻通路清除并返回真；字段假/会话
// 缺席返回假；清除失败按消费失败返回假（标记留在原处，下次消费重试）。
func (m *SessionManager) consumeLegacyWorkbenchCreateMarker(root string) bool {
	_, err := m.mutateSessionEnvelopeLocked(root, func(sess *WorkbenchSession) error {
		if !sess.PendingWorkbenchCreate {
			return errCreateMarkerLegacyAbsent
		}
		sess.PendingWorkbenchCreate = false
		return nil
	})
	switch {
	case err == nil:
		return true
	case errors.Is(err, errCreateMarkerLegacyAbsent), errors.Is(err, ErrWorkbenchSessionNotFound):
		return false
	default:
		m.warnf("迁移消费旧信封创建标记失败（root=%s）: %v", root, err)
		return false
	}
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

// —— 集名候选动作（作品集识别强化 批次三阶段5：归集两段式由前端做，后端供
// 确认动作；忽略/恢复=会话态）——

// slicePos 字符串切片内目标的位置（缺席 -1）。
func slicePos(items []string, want string) int {
	for i, item := range items {
		if item == want {
			return i
		}
	}
	return -1
}

// SetCandidateIgnored 登记或解除集名候选的忽略（幂等会话态）：ignored 真=
// 目标键入忽略清单，假=移出（恢复）。随后排一轮重识别——候选分仓在重识别
// 聚合时按清单移入/移回（成员结论不受影响）。
func (m *SessionManager) SetCandidateIgnored(root, target string, ignored bool) (*WorkbenchSession, error) {
	if target == "" {
		return nil, errors.New("候选目标键为空，无法登记忽略项")
	}
	sess, err := m.mutateSession(root, func(sess *WorkbenchSession) error {
		pos := slicePos(sess.IgnoredCandidates, target)
		if ignored && pos < 0 {
			sess.IgnoredCandidates = append(sess.IgnoredCandidates, target)
		} else if !ignored && pos >= 0 {
			sess.IgnoredCandidates = append(sess.IgnoredCandidates[:pos], sess.IgnoredCandidates[pos+1:]...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	m.scheduleRecognitionRefresh(root)
	return sess, nil
}

// ConfirmWorkSetCandidate 确认集名候选（两段式归集的确认动作）：对候选全部
// 成员逐文件合并写集维度赋值（collection=setName，其余维度不动——复用目录
// 赋值的合并通路），目标键登记进 confirmed 清单并移出 ignored，
// existingSetID>0 记会话旁路簿记（集名→既有集行 id）。赋值写走 mutateSession
// 单点串行与修订号递增（与单次赋值同通路），成功后排一轮增量重识别（成员
// 终选声明与快照 confirmedCandidates 分仓随之刷新）。返回会话与生效成员数。
func (m *SessionManager) ConfirmWorkSetCandidate(root, target, setName string, existingSetID int64) (*WorkbenchSession, int, error) {
	setName = strings.TrimSpace(setName)
	if target == "" || setName == "" {
		return nil, 0, errors.New("候选目标键与确认集名均不可为空")
	}
	sess, err := m.cachedSession(root) // 只读校验前置读：共享只读实例
	if err != nil {
		return nil, 0, err
	}
	if sess.Snapshot == nil {
		return nil, 0, errors.New("该会话尚未完成全量识别，无候选可确认，请先识别")
	}
	var cand *SnapCandidate
	for i := range sess.Snapshot.Candidates {
		if sess.Snapshot.Candidates[i].target() == target {
			cand = &sess.Snapshot.Candidates[i]
			break
		}
	}
	if cand == nil {
		return nil, 0, errors.New("候选不在当前主清单内（可能已忽略、已确认或识别结构已变化），请刷新候选墙后重试")
	}
	now := time.Now().UnixMilli()
	sess, err = m.mutateSession(root, func(sess *WorkbenchSession) error {
		if sess.Assignments == nil {
			sess.Assignments = make(map[string]SessionAssignment)
		}
		for _, mem := range cand.Members {
			merged := mergeDirAssignment(sess.Assignments[mem.RelPath], SessionAssignment{Collection: setName})
			merged.UpdatedAtMs = now
			sess.Assignments[mem.RelPath] = merged
		}
		sess.AssignRevision++
		if pos := slicePos(sess.IgnoredCandidates, target); pos >= 0 {
			sess.IgnoredCandidates = append(sess.IgnoredCandidates[:pos], sess.IgnoredCandidates[pos+1:]...)
		}
		if slicePos(sess.ConfirmedCandidates, target) < 0 {
			sess.ConfirmedCandidates = append(sess.ConfirmedCandidates, target)
		}
		if existingSetID > 0 {
			if sess.ConfirmedSetIDs == nil {
				sess.ConfirmedSetIDs = make(map[string]int64)
			}
			sess.ConfirmedSetIDs[setName] = existingSetID
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	m.scheduleRecognitionRefresh(root)
	return sess, len(cand.Members), nil
}

// UnconfirmWorkSetCandidate 撤销集名候选的归集（剧本2）：目标在快照 confirmed
// 分仓内寻址（与前端已归集卡同源），缺席回可读错误、卡片不动（会话已刷新或
// 识别结构已变）。撤销只清成员的「作品集」维度赋值（collection 置空、出处
// 引用一并清，其余维度原样保留；全维度剥空的赋值整条删除——与单次赋值「空
// 赋值=清除」同语义，含已泛化标记一并移除），目标键移出 confirmed 清单，
// 确认集名的既有集旁路簿记（ConfirmedSetIDs，单键记账）一并清理。赋值写走
// mutateSession 单点串行，修订号递增驱动增量闸门失效（清赋值后成员目录不再
// 由赋值集锚定，靠修订号防止旧结论被 mtime 游标复用）；成功后排一轮增量重
// 识别——完成前快照保持旧态（成员集声明旧值、卡在已归集分仓），完成后候选
// 重新聚合回主清单、成员集声明消失。返回会话与生效成员数。
func (m *SessionManager) UnconfirmWorkSetCandidate(root, target string) (*WorkbenchSession, int, error) {
	if target == "" {
		return nil, 0, errors.New("候选目标键为空，无法撤销归集")
	}
	sess, err := m.cachedSession(root) // 只读校验前置读：共享只读实例
	if err != nil {
		return nil, 0, err
	}
	if sess.Snapshot == nil {
		return nil, 0, errors.New("该会话尚未完成全量识别，无已归集候选可撤销，请先识别")
	}
	var cand *SnapCandidate
	for i := range sess.Snapshot.ConfirmedCandidates {
		if sess.Snapshot.ConfirmedCandidates[i].target() == target {
			cand = &sess.Snapshot.ConfirmedCandidates[i]
			break
		}
	}
	if cand == nil {
		return nil, 0, errors.New("目标不在已归集清单（会话已刷新或识别结构已变），请刷新候选墙后重试")
	}
	// 簿记清理的确认集名：优先 confirmed 分仓卡回带值，缺席（成员赋值此后被
	// 用户清除）时回退成员赋值现值，均缺席则无簿记可清。
	setName := cand.ConfirmedSetName
	if setName == "" {
		setName = confirmedCollectionOf(*cand, sess.Assignments)
	}
	now := time.Now().UnixMilli()
	sess, err = m.mutateSession(root, func(sess *WorkbenchSession) error {
		for _, mem := range cand.Members {
			assign, ok := sess.Assignments[mem.RelPath]
			if !ok {
				continue
			}
			assign.Collection = ""
			assign.CollectionClueRef = ""
			if len(assign.coveredDimensions()) == 0 {
				// 全维度剥空=清除赋值（含已泛化标记一并移除，与单次赋值通路同语义）
				delete(sess.Assignments, mem.RelPath)
				continue
			}
			assign.UpdatedAtMs = now
			sess.Assignments[mem.RelPath] = assign
		}
		// 批量清维整体视为一次赋值变更：修订号递增驱动增量闸门失效
		sess.AssignRevision++
		if pos := slicePos(sess.ConfirmedCandidates, target); pos >= 0 {
			sess.ConfirmedCandidates = append(sess.ConfirmedCandidates[:pos], sess.ConfirmedCandidates[pos+1:]...)
		}
		if setName != "" {
			delete(sess.ConfirmedSetIDs, setName)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	m.scheduleRecognitionRefresh(root)
	return sess, len(cand.Members), nil
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
	workbenchActionListSessions        = "listSessions"
	workbenchActionCreateSession       = "createSession"
	workbenchActionReplaceSession      = "replaceSession"
	workbenchActionDeleteSession       = "deleteSession"
	workbenchActionActivateSession     = "activateSession"
	workbenchActionConfirmAnswer       = "confirmAnswer"
	workbenchActionStartRecognition    = "startRecognition"
	workbenchActionGetPanorama         = "getPanorama"
	workbenchActionAssign              = "assign"              // 单次赋值（会话态知识写入）
	workbenchActionAssignDir           = "assignDir"           // 目录赋值（子树展开逐文件合并写；dirRelKey 寻址）
	workbenchActionClearAssignment     = "clearAssignment"     // 清除单文件赋值
	workbenchActionClearDirAssignments = "clearDirAssignments" // 清除目录子树全部赋值
	workbenchActionSetIgnored          = "setIgnored"          // 登记或解除忽略项
	workbenchActionGeneralize          = "generalize"          // 泛化成规则（档案写+验证轮；mode=struct 为结构映射组合规则的确认入档）
	workbenchActionSummarizeRule       = "summarizeRule"       // 规则总结器：赋值线索出处→组合规则建议（人话+全库命中预览+样例；不写档案）
	workbenchActionAdjudicate          = "adjudicate"          // 冲突裁决生成组合规则
	workbenchActionGetDiff             = "getDiff"             // 最近一次重识别差异集
	workbenchActionSetSelection        = "setSelection"        // 全量替换资源选择集（作品键）
	workbenchActionCreateTasks         = "createTasks"         // 创建编排（异步：对账→置标记→goroutine 创建；立即应答 started，进度与终态经 createProgress/createDone 事件推送）
	workbenchActionCancelCreate        = "cancelCreate"        // 取消在途创建（甲3/决策3：置取消位→verify/发射循环收流→宿主未收批回滚；应答 cancelState=cancelling/completed/idle）

	// —— 集名候选族（作品集识别强化 批次三阶段5）——
	workbenchActionIgnoreCandidate  = "ignoreCandidate"         // 集名候选忽略（会话态分仓；target 寻址）
	workbenchActionRestoreCandidate = "restoreCandidate"        // 集名候选恢复（移回候选墙主清单）
	workbenchActionConfirmCandidate = "confirmWorkSetCandidate" // 集名候选确认（成员逐文件写集维度赋值；target+setName+existingSetId）
	workbenchActionUndoConfirm      = "undoWorkSetCandidate"    // 集名候选撤销归集（清成员集维度赋值、移出 confirmed 分仓；target 寻址）

	// —— 集规则泛化族（已归集泛化入口 · 迭代三 · 剧本C；target 寻址
	// confirmed 分仓卡，泛化主体=集整体——key+members+confirmedSetName）——
	workbenchActionSummarizeWorkSetRule  = "summarizeWorkSetRule"  // 集规则草稿推导（共同形态+规则草稿+命中预览+调整面线索清单；不写档案）
	workbenchActionGeneralizeWorkSetRule = "generalizeWorkSetRule" // 集规则泛化入档（草稿即所见即所入；集成员作用域消费+验证轮）
	workbenchActionSaveWorkSetRuleAdjust = "saveWorkSetRuleAdjust" // 集规则调整面保存（线索指派→规则组装入档；效果与泛化入档相同）

	// —— 规则管理族（规则子页）——
	workbenchActionGetRules           = "getRules"           // 规则管理视图（档案条目+内置表+禁用清单+备份态）
	workbenchActionPreviewRule        = "previewRule"        // 规则草稿命中预览（只读：档案∪草稿对会话识别快照全库合并求值，不写档案）
	workbenchActionSaveRule           = "saveRule"           // 规则条目保存（新建/编辑，构造期单条校验）
	workbenchActionDeleteRule         = "deleteRule"         // 删除档案规则条目
	workbenchActionSetRuleEnabled     = "setRuleEnabled"     // 档案规则条目启停
	workbenchActionSetBuiltinDisabled = "setBuiltinDisabled" // 内置规则禁用/启用（档案禁用清单）
	workbenchActionRollbackProfile    = "rollbackProfile"    // 档案回退一代（写前备份）
)

// getPanorama 的分节分页拉取词汇（大载荷分页方案·设计 A）。无 section 的
// 缺省请求按 overview 应答；旧值 "files" 在新词汇下语义=文件分节（cursor 空
// =首页）。
const (
	// panoramaSectionOverview 概览分节（单页整答，当前量级小）：统计+目录树+
	// 群+规则统计+三清单+待裁决+过期赋值，并随答会话决策态与最近差异
	//（Decisions/Diff 仅本分节随答）。
	panoramaSectionOverview = "overview"
	// panoramaSectionFiles 文件明细分节：按游标（投影数组下标）分页拉取。
	panoramaSectionFiles = "files"
	// panoramaSectionDomain 选择域分节（作品+集）：作品序列在前、集序列在后
	// 共用一个下标空间，按游标分页拉取。
	panoramaSectionDomain = "domain"
)

// workbenchFrontendRequest 前端→插件请求载荷（序号配对）。
type workbenchFrontendRequest struct {
	Seq    int64  `json:"seq"`
	Action string `json:"action"`
	// Root 会话寻址的库根（createSession 为新建库根输入）。
	Root string `json:"root,omitempty"`
	// ConfirmSeq / Accepted 入口确认答复的配对序号与采纳结果。
	ConfirmSeq int64 `json:"confirmSeq,omitempty"`
	Accepted   bool  `json:"accepted,omitempty"`
	// Section getPanorama 的分节拉取（缺省=overview 概览分节）。
	Section string `json:"section,omitempty"`
	// Cursor getPanorama 分节续拉游标（不透明令牌「世代:下标」，空=首页）；
	// PageSizeHint 单页条数上限提示（可选，仅收窄默认条数上限，页字节界不因
	// 提示放宽）。
	Cursor       string `json:"cursor,omitempty"`
	PageSizeHint int    `json:"pageSizeHint,omitempty"`

	// —— 手动解析族载荷（relPath 为相对库根的 '/' 路径）——
	// RelPath 赋值/泛化/裁决/忽略的目标文件相对路径。
	RelPath string `json:"relPath,omitempty"`
	// DirRelKey 目录寻址键（相对库根的 '/' 目录路径，根直下为空串）：assignDir /
	// clearDirAssignments 的目标子树（空串=库根全树）；generalize / summarizeRule
	// 的目录寻址（relPath 缺省时生效，样本文件取子树内首个有赋值的文件）。
	DirRelKey string `json:"dirRelKey,omitempty"`
	// Author / AuthorRole / Title / Tags / Collection 单次赋值的各维度指定
	//（assign 动作；空维度不覆盖档案结论）。
	Author     string   `json:"author,omitempty"`
	AuthorRole string   `json:"authorRole,omitempty"`
	Title      string   `json:"title,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Collection string   `json:"collection,omitempty"`
	// AuthorClueRef / TitleClueRef / CollectionClueRef / TagClueRefs assign 各
	//维度可携带的线索出处引用（结构分段线索集的稳定标识：rootName /
	// dirLevel[n] / dirLevelStripped[n] / filenameSeg[i] / tailCode；TagClueRefs
	// 与 Tags 逐位平行）：随赋值入会话存储，供「自 一级目录(剥壳)」来源小字与
	// 规则总结器；缺省=无出处（旧调用行为不变）。
	AuthorClueRef     string   `json:"authorClueRef,omitempty"`
	TitleClueRef      string   `json:"titleClueRef,omitempty"`
	CollectionClueRef string   `json:"collectionClueRef,omitempty"`
	TagClueRefs       []string `json:"tagClueRefs,omitempty"`
	// Ignored setIgnored 的登记方向（真=登记忽略，假=解除）。
	Ignored bool `json:"ignored,omitempty"`

	// —— 集名候选族载荷（阶段5 候选墙）——
	// Target 候选目标键（Source+Dir+Key 稳定串）：ignoreCandidate /
	// restoreCandidate / confirmWorkSetCandidate 的寻址。
	Target string `json:"target,omitempty"`
	// SetName confirmWorkSetCandidate 的确认集名（成员集维度赋值的 collection）。
	SetName string `json:"setName,omitempty"`
	// ExistingSetID confirmWorkSetCandidate 携带的库内既有集行 id（A 档「归入
	// 既有集」命中时 >0，记会话旁路簿记随快照呈现）。
	ExistingSetID int64 `json:"existingSetId,omitempty"`

	// —— 集规则泛化族载荷（迭代三 · 剧本C 调整面；维度指派复用
	// AuthorClueRef/TitleClueRef/TagClueRefs——可指向目录层或文件名段线索）——
	// CollectionSegRefs 集名捕获的文件名段组合（setSegN 引用，须连续段）。
	CollectionSegRefs []string `json:"collectionSegRefs,omitempty"`
	// VariableSegRefs 标记为可变的文件名段（构造上与未指派段同泛化消费）。
	VariableSegRefs []string `json:"variableSegRefs,omitempty"`
	// SeqSegRef 序号段指派（默认=末段序号元素；须为序号段线索）。
	SeqSegRef string `json:"seqSegRef,omitempty"`
	// DirAnchorRef 目录锚定指派（dirLevel[n]/dirLevelStripped[n]；捕获规则
	// 模式 And 目录层锚）。
	DirAnchorRef string `json:"dirAnchorRef,omitempty"`
	// SetRuleMemberOnlyOff saveWorkSetRuleAdjust 关闭「仅序列成员产出」时为真
	//（缺省假=勾选仅序列成员产出，与草稿默认一致——bool 载荷以关向为极性，
	// 未携带即保持默认）。
	SetRuleMemberOnlyOff bool `json:"setRuleMemberOnlyOff,omitempty"`
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
	// TaskName createTasks 的自定义任务名（可选，空/全空白=后端默认名兜底；
	// 前端阶段2 的名称确认对话框接入，本阶段先通后端链路）。
	TaskName string `json:"taskName,omitempty"`

	// —— 规则管理族载荷（规则子页）——
	// Rule saveRule 的完整规则条目 JSON 与 previewRule 的规则草稿 JSON
	//（ruleEntryJSON 形态：pattern 与 extractions 为 JSON 片段原文，synthesis
	// 为表单化字段组装；预览草稿与保存载荷同形）。
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
	// running=同库根干跑已在途（本次未重复启动）；createTasks 复用 started=已
	// 开始创建（创建在独立 goroutine 执行，进度与终态经 createProgress/createDone
	// 事件推送，选择集清空随 createDone 成功态发生——见 selection.go 设计 C）。
	Started bool `json:"started,omitempty"`
	Running bool `json:"running,omitempty"`
	// CancelState cancelCreate 的受理结论（甲3/决策3）：cancelling=取消已受理
	//（正在收流清理，终态经 createDone 事件）、completed=恰好已完成（原子提交点
	// 已过未执行取消，风险10）、idle=无在途创建（此时应答为错误形态）。
	CancelState string `json:"cancelState,omitempty"`
	// Panorama getPanorama 的识别全景投影（overview 分节=概览裁剪、files 分节
	//=文件明细分页页片；其余分节不携带）。
	Panorama *RecognitionSnapshot `json:"panorama,omitempty"`
	// Decisions getPanorama overview 分节随答的会话决策态（元素7：赋值/忽略/
	// 选择集的活数据）。
	Decisions *workbenchSessionDecisionsView `json:"decisions,omitempty"`
	// Diff getPanorama overview 分节随答的最近一次重识别差异集（getDiff 同源；
	// 生成规则后强制验证轮的高亮载荷）。
	Diff *SnapDiff `json:"diff,omitempty"`
	// Assignment assign/clearAssignment 后该文件的赋值呈现（清除后为空）。
	Assignment *workbenchAssignmentView `json:"assignment,omitempty"`
	// Affected assignDir/clearDirAssignments 的生效文件计数（目录赋值合并写
	// 触及的子树文件数 / 清除的赋值数）。
	Affected int `json:"affected,omitempty"`
	// Rule generalize/adjudicate 生成的规则摘要（已入档案并触发验证轮）。
	Rule *GeneratedRuleView `json:"rule,omitempty"`
	// Rules generalize（mode=struct）入档的组合规则摘要序列（单规则维度仍填
	// Rule，Rules 含全部条目）。
	Rules []*GeneratedRuleView `json:"rules,omitempty"`
	// RuleSuggestion summarizeRule 的总结器产物（建议卡载荷：人话规则+逐维度
	// 映射+命中预览统计+样例应用；未入档，确认走 generalize mode=struct）。
	RuleSuggestion *RuleSuggestion `json:"ruleSuggestion,omitempty"`
	// SetRuleSuggestion summarizeWorkSetRule 的集规则草稿（集整体推导：集名+
	// 成员+共同形态+规则草稿+命中预览+调整面线索清单与默认指派；Failed=true
	// 为剧本步5a 失败态——泛化置灰、调整可用）。
	SetRuleSuggestion *SetRuleSuggestion `json:"setRuleSuggestion,omitempty"`
	// Domain getPanorama 选择域分节随答的资源选择域（阶段4：作品原子结构+
	// 集视图，前端选择交互与选择集键的单一推导源），按游标分页。
	Domain *WorkDomain `json:"domain,omitempty"`
	// Page getPanorama 分节分页回执（三分节统一按共用下标空间分页：overview
	// 重载荷清单六段共用空间，files/domain 各自条目序列）。
	Page *workbenchPageReceipt `json:"page,omitempty"`
	// RulesView getRules 与规则管理族动作随答返回的规则管理视图（档案条目+
	// 内置表+禁用清单+备份态），前端每次动作后整份刷新。
	RulesView *rulesAdminView `json:"rulesView,omitempty"`
	// RulePreview previewRule 的命中预览（命中数/快照文件总数/样例至多 3 条）。
	RulePreview *rulesPreviewResult `json:"rulePreview,omitempty"`
	// RolledBack rollbackProfile 的回退结论（真=已回退一代）。
	RolledBack bool `json:"rolledBack,omitempty"`
}

// workbenchPageReceipt getPanorama 分节分页回执。generation=快照世代
// （GeneratedAtMs）：识别轮次变更后世代前移，拉取中携带的旧游标失效——应答
// 明确报「世代变更，请整轮重拉」由前端整轮重来。
type workbenchPageReceipt struct {
	// Section 本页所属分节（overview / files / domain）。
	Section string `json:"section"`
	// Offset / Count 本页在分节条目序列中的起点与条数；Total 序列总条数
	//（overview 为六段共用下标空间合计）。
	Offset int `json:"offset"`
	Count  int `json:"count"`
	Total  int `json:"total"`
	// NextCursor 续拉游标（末页为空）；Done 本页即末页。
	NextCursor string `json:"nextCursor,omitempty"`
	Done       bool   `json:"done"`
	// Generation 快照世代（GeneratedAtMs）。
	Generation int64 `json:"generation"`
}

// workbenchSessionDecisionsView 会话决策态投影（元素7 的活数据——赋值/忽略/
// 选择集随用户操作即时变化，不入识别快照）。
type workbenchSessionDecisionsView struct {
	// Assignments 已手动解析项（泛化后赋值全量保留——2026-10-25 用户裁决
	// 废除消费剥离，与规则声明双源互证；含已泛化标记）。
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
	// AuthorClueRef / TitleClueRef / CollectionClueRef / TagClueRefs 各维度赋值
	//的线索出处引用（结构分段线索集稳定标识）：赋值表单「自 一级目录(剥壳)」
	// 来源小字的数据源；TagClueRefs 与 Tags 逐位平行。
	AuthorClueRef     string   `json:"authorClueRef,omitempty"`
	TitleClueRef      string   `json:"titleClueRef,omitempty"`
	CollectionClueRef string   `json:"collectionClueRef,omitempty"`
	TagClueRefs       []string `json:"tagClueRefs,omitempty"`
}

// decisionsViewOf 会话 → 决策态投影（按路径稳定序呈现）。
func decisionsViewOf(sess *WorkbenchSession) *workbenchSessionDecisionsView {
	view := &workbenchSessionDecisionsView{}
	for _, relPath := range sortedKeys(sess.Assignments) {
		assign := sess.Assignments[relPath]
		view.Assignments = append(view.Assignments, workbenchAssignmentView{
			RelPath: relPath, Author: assign.Author, AuthorRole: assign.AuthorRole,
			Title: assign.Title, Tags: assign.Tags, Collection: assign.Collection,
			AuthorClueRef: assign.AuthorClueRef, TitleClueRef: assign.TitleClueRef,
			CollectionClueRef: assign.CollectionClueRef, TagClueRefs: assign.TagClueRefs,
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
		// 日志参数经 SDK 通道先字符串化、再由宿主按模板格式化：数字值须用 %s
		// 动词，%d 收到字符串形态会印出 %!d(string=N)
		m.warnf("工作台应答回发失败(seq=%s): %v", resp.Seq, err)
	}
}

// —— getPanorama 分节分页（大载荷分页方案·设计 A）——

const (
	// panoramaPageTargetBytes 单页序列化字节目标（首遍打包提示：估算函数按此
	// 切页；远低于软界与 64MB 断路器，单页往返在 30s 前端超时窗内余量充足）。
	panoramaPageTargetBytes = 8 << 20
	// panoramaPageSoftBytes 单页序列化软界：页组装后对完整应答 json.Marshal
	// 实测，超界即从尾部卸条目直至达标。估算函数只作首遍打包提示，实测硬门
	// 兜底其漂移（条目新增未计数字段或转义放大不再引发撞断路器的超界应答）；
	// 12MB 为组装目标与断路器之间的安全带。
	panoramaPageSoftBytes = 12 << 20
	// panoramaPageMaxEntries 单页条数硬上限（字节估算之外的双保险，防小条目
	// 海量场景单页条数失控）；请求的 PageSizeHint 只能收窄本上限。
	panoramaPageMaxEntries = 50000
)

// errPanoramaGenerationStale 世代失效：识别轮次更新（快照 GeneratedAtMs 前移），
// 拉取中携带的旧游标作废，前端须整轮重拉。
var errPanoramaGenerationStale = errors.New("世代变更，请整轮重拉")

// errPanoramaCursorInvalid 游标无法解析或下标越界（同世代内不应出现），按整轮
// 重拉语义报错。
var errPanoramaCursorInvalid = errors.New("游标无效，请整轮重拉")

// encodePanoramaCursor 游标编码：世代:下标（前端不透明令牌，原样回传）。
func encodePanoramaCursor(generation int64, offset int) string {
	return fmt.Sprintf("%d:%d", generation, offset)
}

// decodePanoramaCursor 游标解析并校验世代（与当前快照世代不一致 → 世代失效）。
func decodePanoramaCursor(cursor string, generation int64) (int, error) {
	genText, offText, ok := strings.Cut(cursor, ":")
	if !ok {
		return 0, errPanoramaCursorInvalid
	}
	gen, genErr := strconv.ParseInt(genText, 10, 64)
	offset, offErr := strconv.Atoi(offText)
	if genErr != nil || offErr != nil || offset < 0 {
		return 0, errPanoramaCursorInvalid
	}
	if gen != generation {
		return 0, errPanoramaGenerationStale
	}
	return offset, nil
}

// snapFileWireBytes SnapFile 条目的序列化字节估算（确定性近似，不做真序列化：
// 键名/标点骨架按定长、字符串按 UTF-8 字节数；JSON 转义增量与骨架余量互抵，
// 在 6-16MB 页界目标下足够安全——简单确定性优先，不追求精确）。
func snapFileWireBytes(f SnapFile) int {
	n := 144 // 骨架：键名/标点/state/数值与布尔字段
	n += len(f.RelPath) + len(f.FileName) + len(f.DirRelKey) + len(f.State) + len(f.GroupKey)
	for _, d := range f.Final {
		n += 72 + len(d.Dimension) + len(d.Value) + len(d.Source) + len(d.RoleName) + len(d.RuleID)
	}
	for _, dim := range f.PendingDims {
		n += 16 + len(dim)
	}
	for _, id := range f.MatchedRuleIDs {
		n += 18 + len(id)
	}
	for _, c := range f.Clues {
		n += 28 + len(c.Kind) + len(c.Detail)
	}
	for _, sc := range f.StructClues {
		n += 44 + len(sc.Ref) + len(sc.Kind) + len(sc.Label) + len(sc.Value)
	}
	return n
}

// selectableWorkWireBytes 选择域作品条目的序列化字节估算（口径同上；轨道按
// 投影层预览上限截断后形态估算——与 domain 分节逐条投影的出线形态一致）。
func selectableWorkWireBytes(w *SelectableWork) int {
	n := 104
	n += len(w.WorkID) + len(w.Kind) + len(w.DirRelKey) + len(w.SetKey) + len(w.State)
	for _, rel := range previewWireMembers(w.Files) {
		n += 14 + len(rel)
	}
	return n
}

// workSetViewWireBytes 选择域集条目的序列化字节估算（口径同上；成员作品键按
// 投影层预览上限截断后形态估算）。
func workSetViewWireBytes(s *WorkSetView) int {
	n := 68
	n += len(s.SetKey) + len(s.DirRelKey) + len(s.Key)
	for _, m := range previewWireMembers(s.Members) {
		n += 12 + len(m)
	}
	return n
}

// snapElWireBytes 文件名元素条目的序列化字节估算（口径同上）。
func snapElWireBytes(el SnapEl) int {
	return 36 + len(el.Text) + len(el.Kind) + len(el.Inner)
}

// snapDirWireBytes 目录树条目的序列化字节估算（口径同上；含投影展开的目录级
// 结构线索集）。
func snapDirWireBytes(d SnapDir) int {
	n := 92 + len(d.RelKey) + len(d.Name) + len(d.MeaningRuleID)
	for _, m := range d.Meanings {
		n += 44 + len(m.Type) + len(m.ID) + len(m.Name)
	}
	for _, c := range d.StructClues {
		n += 44 + len(c.Ref) + len(c.Kind) + len(c.Label) + len(c.Value)
	}
	return n
}

// snapGroupWireBytes 序列群条目的序列化字节估算（口径同上；成员携带文件名
// 元素数组，按投影层预览上限截断后形态估算——与 overview 投影的出线形态一致）。
func snapGroupWireBytes(g SnapGroup) int {
	n := 118 + len(g.DirRelKey) + len(g.Key) + len(g.RuleID) +
		len(g.FormName) + len(g.FormKeyExpr) + len(g.FormSeqExpr)
	for _, m := range previewWireMembers(g.Members) {
		n += 38 + len(m.RelPath) + len(m.SeqText)
		for _, el := range m.Elements {
			n += snapElWireBytes(el)
		}
	}
	return n
}

// snapCandidateWireBytes 集名候选条目的序列化字节估算（口径同上；成员携带
// 文件名元素数组，按投影层预览上限截断后形态估算——与 overview 投影的出线
// 形态一致）。
func snapCandidateWireBytes(c SnapCandidate) int {
	n := 108 + len(c.Key) + len(c.Dir) + len(c.Source) + len(c.FormName) +
		len(c.Reason) + len(c.ExistingSetName) + len(c.ConfirmedSetName)
	for _, m := range previewWireMembers(c.Members) {
		n += 32 + len(m.RelPath) + len(m.SeqText)
		for _, el := range m.Elements {
			n += snapElWireBytes(el)
		}
	}
	for _, el := range c.KeyElements {
		n += snapElWireBytes(el)
	}
	return n
}

// snapPendingWireBytes 待裁决明细条目的序列化字节估算（口径同上）。
func snapPendingWireBytes(p SnapPending) int {
	n := 74 + len(p.RelPath) + len(p.FileName) + len(p.Dimension) + len(p.Reason)
	for _, d := range p.Candidates {
		n += 72 + len(d.Dimension) + len(d.Value) + len(d.Source) + len(d.RoleName) + len(d.RuleID)
	}
	return n
}

// panoramaPackEnd 估算首遍打包：从 offset 起按页字节目标与条数上限装页
//（首条必装保证推进），返回页终点下标；cost 为共用下标空间内单条目的估算
// 字节。估算只作首遍提示，实测硬门兜底其漂移。
func panoramaPackEnd(cost func(i int) int, offset, total, maxEntries int) int {
	end := offset
	for budget := panoramaPageTargetBytes; end < total && end-offset < maxEntries; end++ {
		c := cost(end)
		if end > offset && budget < c {
			break
		}
		budget -= c
	}
	return end
}

// panoramaOverviewSpans overview 重载荷清单的共用下标空间：六段按固定顺序
// 线性拼接（目录树→序列群→候选三仓→待裁决明细），段边界由各段条数确定；
// 与 domain 分节作品/集共用下标空间同构。
type panoramaOverviewSpans struct {
	dirs, groups, candidates, ignored, confirmed, pending int
}

func (s panoramaOverviewSpans) total() int {
	return s.dirs + s.groups + s.candidates + s.ignored + s.confirmed + s.pending
}

// cost 共用空间第 i 条的估算字节（按段分派到对应清单条目）。
func (s panoramaOverviewSpans) cost(view *RecognitionSnapshot, i int) int {
	switch {
	case i < s.dirs:
		return snapDirWireBytes(view.Dirs[i])
	case i < s.dirs+s.groups:
		return snapGroupWireBytes(view.Groups[i-s.dirs])
	case i < s.dirs+s.groups+s.candidates:
		return snapCandidateWireBytes(view.Candidates[i-s.dirs-s.groups])
	case i < s.dirs+s.groups+s.candidates+s.ignored:
		return snapCandidateWireBytes(view.IgnoredCandidates[i-s.dirs-s.groups-s.candidates])
	case i < s.dirs+s.groups+s.candidates+s.ignored+s.confirmed:
		return snapCandidateWireBytes(view.ConfirmedCandidates[i-s.dirs-s.groups-s.candidates-s.ignored])
	default:
		return snapPendingWireBytes(view.Pending[i-s.dirs-s.groups-s.candidates-s.ignored-s.confirmed])
	}
}

// slice 共用空间 [offset,end) 区间拆回各段切片：未涉及的段为 nil，部分涉及
// 的段按区间截取。
func (s panoramaOverviewSpans) slice(view *RecognitionSnapshot, offset, end int) (
	dirs []SnapDir, groups []SnapGroup, candidates, ignored, confirmed []SnapCandidate, pending []SnapPending,
) {
	seg := func(lo, hi int) (int, int, bool) {
		if end <= lo || offset >= hi {
			return 0, 0, false
		}
		return max(offset, lo), min(end, hi), true
	}
	if lo, hi, ok := seg(0, s.dirs); ok {
		dirs = view.Dirs[lo:hi]
	}
	base := s.dirs
	if lo, hi, ok := seg(base, base+s.groups); ok {
		groups = view.Groups[lo-base : hi-base]
	}
	base += s.groups
	if lo, hi, ok := seg(base, base+s.candidates); ok {
		candidates = view.Candidates[lo-base : hi-base]
	}
	base += s.candidates
	if lo, hi, ok := seg(base, base+s.ignored); ok {
		ignored = view.IgnoredCandidates[lo-base : hi-base]
	}
	base += s.ignored
	if lo, hi, ok := seg(base, base+s.confirmed); ok {
		confirmed = view.ConfirmedCandidates[lo-base : hi-base]
	}
	base += s.confirmed
	if lo, hi, ok := seg(base, base+s.pending); ok {
		pending = view.Pending[lo-base : hi-base]
	}
	return
}

// panoramaReceipt 分页回执构造：终点越过 total 即末页，非末页携带续拉游标。
func panoramaReceipt(section string, generation int64, offset, end, total int) *workbenchPageReceipt {
	receipt := &workbenchPageReceipt{
		Section: section, Offset: offset, Count: end - offset, Total: total,
		Done: end >= total, Generation: generation,
	}
	if !receipt.Done {
		receipt.NextCursor = encodePanoramaCursor(generation, end)
	}
	return receipt
}

// panoramaSectionPage getPanorama 分节应答的装配件：分页回执 + 分节数据，
// 未请求的分节恒空。shrink 为测量硬门的卸条目通道——按卸后条目数重建页载荷
// 与回执（n ∈ [1, 回执 Count]；分节恒有条目空间，恒非 nil）。
type panoramaSectionPage struct {
	receipt  *workbenchPageReceipt
	panorama *RecognitionSnapshot
	domain   *WorkDomain
	overview bool // overview 分节（Decisions/Diff 仅该分节首页随答）
	shrink   func(n int)
}

// panoramaPageOf getPanorama 分节分页求值：按 section 词汇裁剪投影、按游标
// （先过世代校验）切页并落 Page 回执。三分节统一按共用下标空间切页——
// overview 的重载荷清单（目录树/序列群/候选三仓/待裁决明细）六段共用一个
// 下标空间逐页（统计/规则档案态等小字段每页随答）；files/domain 语义不变
//（domain 作品序列在前集在后，页可跨界）。
func (m *SessionManager) panoramaPageOf(sess *WorkbenchSession, req workbenchFrontendRequest) (*panoramaSectionPage, error) {
	generation := sess.Snapshot.GeneratedAtMs
	switch req.Section {
	case "", panoramaSectionOverview, panoramaSectionFiles, panoramaSectionDomain:
	default:
		return nil, fmt.Errorf("未知的 getPanorama 分节 %q", req.Section)
	}
	offset := 0
	if req.Cursor != "" {
		var err error
		if offset, err = decodePanoramaCursor(req.Cursor, generation); err != nil {
			return nil, err
		}
	}
	maxEntries := panoramaPageMaxEntries
	if req.PageSizeHint > 0 && req.PageSizeHint < maxEntries {
		maxEntries = req.PageSizeHint
	}
	switch req.Section {
	case "", panoramaSectionOverview:
		view := panoramaOverviewViewOf(sess.Snapshot)
		spans := panoramaOverviewSpans{
			dirs: len(view.Dirs), groups: len(view.Groups), candidates: len(view.Candidates),
			ignored: len(view.IgnoredCandidates), confirmed: len(view.ConfirmedCandidates),
			pending: len(view.Pending),
		}
		total := spans.total()
		if offset > total {
			return nil, errPanoramaCursorInvalid
		}
		end := panoramaPackEnd(func(i int) int { return spans.cost(&view, i) }, offset, total, maxEntries)
		page := &panoramaSectionPage{overview: true}
		apply := func(e int) {
			pan := view
			pan.Dirs, pan.Groups, pan.Candidates, pan.IgnoredCandidates, pan.ConfirmedCandidates, pan.Pending =
				spans.slice(&view, offset, e)
			page.panorama = &pan
			page.receipt = panoramaReceipt(panoramaSectionOverview, generation, offset, e, total)
		}
		apply(end)
		page.shrink = apply
		return page, nil
	case panoramaSectionFiles:
		// 按页投影：不再全量投影 20 万级文件再切片——仅对打包扫描与出页涉及的
		// 页内条目逐条展开结构线索（投影函数与全量投影 panoramaViewOf 的逐文件
		// 投影同一实现，内容逐条等价，语义守卫=既有「逐页拼接与全量投影逐条
		// 全等」测试）。projected 按相对页内下标惰性装填：估算与出页共用一次
		// 投影（估算须计投影展开的线索字节，且收缩硬门 shrink 只取前缀不重投）。
		snap := sess.Snapshot
		if offset > len(snap.Files) {
			return nil, errPanoramaCursorInvalid
		}
		projected := make([]SnapFile, 0, 64)
		projectAt := func(i int) SnapFile {
			f := snap.Files[i]
			f.StructClues = StructCluesOf(snap.RootPath, f.RelPath, f.FileName)
			return f
		}
		end := panoramaPackEnd(func(i int) int {
			for len(projected) <= i-offset {
				projected = append(projected, projectAt(offset+len(projected)))
			}
			return snapFileWireBytes(projected[i-offset])
		}, offset, len(snap.Files), maxEntries)
		page := &panoramaSectionPage{}
		apply := func(e int) {
			files := snap.Files[offset:e] // 切片边界形态与旧实现同源（nil 语义保持）
			if len(files) > 0 {
				files = projected[:len(files)]
			}
			page.panorama = &RecognitionSnapshot{
				SchemaVersion: snap.SchemaVersion, GeneratedAtMs: snap.GeneratedAtMs,
				Files: files,
			}
			page.receipt = panoramaReceipt(panoramaSectionFiles, generation, offset, e, len(snap.Files))
		}
		apply(end)
		page.shrink = apply
		return page, nil
	default:
		// 选择域推导 O(全库)：会话为缓存共享实例时经 workDomainOf 复用条目内
		// 的惰性推导结果（全链逐页共摊一次，与快照同生命周期失效）。页内条目
		// 按投影层成员上限截成员后出页（作品轨道/集成员作品键，与全量投影
		// domainViewOf 同一截断、逐条浅拷贝不动推导域本体——语义守卫=逐页拼接
		// 与全量投影逐条全等，两侧同截断）。
		domain := m.workDomainOf(sess)
		projectWork := func(i int) *SelectableWork {
			w := *domain.Works[i]
			w.Files = previewWireMembers(w.Files)
			return &w
		}
		projectSet := func(i int) *WorkSetView {
			s := *domain.Sets[i]
			s.Members = previewWireMembers(s.Members)
			return &s
		}
		nWorks := len(domain.Works)
		total := nWorks + len(domain.Sets)
		if offset > total {
			return nil, errPanoramaCursorInvalid
		}
		end := panoramaPackEnd(func(i int) int {
			if i < nWorks {
				return selectableWorkWireBytes(domain.Works[i])
			}
			return workSetViewWireBytes(domain.Sets[i-nWorks])
		}, offset, total, maxEntries)
		page := &panoramaSectionPage{}
		apply := func(e int) {
			// 页域投影：两序列恒为非 nil 空切片形态（序列化出 []，与全量域视图
			// 一致）；作品序列在前集在后，页可跨界（与共用下标空间同构）。
			works := make([]*SelectableWork, 0, e-offset)
			for i := offset; i < e && i < nWorks; i++ {
				works = append(works, projectWork(i))
			}
			sets := make([]*WorkSetView, 0, e-offset)
			for i := max(offset, nWorks); i < e; i++ {
				sets = append(sets, projectSet(i-nWorks))
			}
			page.domain = &WorkDomain{Works: works, Sets: sets}
			page.receipt = panoramaReceipt(panoramaSectionDomain, generation, offset, e, total)
		}
		apply(end)
		page.shrink = apply
		return page, nil
	}
}

// boundPanoramaPage 测量硬门：页组装后对完整应答 json.Marshal 实测，超软界
// 即从尾部按超界比例卸条目直至达标（比例步长几何收敛，免逐条卸的重复序列
// 化）；游标随卸后终点回写。首条必装——单条超界仍独立成页并记日志可见。
// 估算函数仅作首遍打包提示，实测兜底其漂移。
func (m *SessionManager) boundPanoramaPage(resp *workbenchFrontendResponse, page *panoramaSectionPage) {
	data, err := json.Marshal(resp)
	if err != nil || len(data) <= panoramaPageSoftBytes || page.shrink == nil || page.receipt.Count <= 1 {
		return
	}
	n := page.receipt.Count
	for len(data) > panoramaPageSoftBytes && n > 1 {
		drop := (n - 1) * (len(data) - panoramaPageSoftBytes) / len(data)
		if drop < 1 {
			drop = 1
		}
		n -= drop
		page.shrink(n)
		resp.Page, resp.Panorama, resp.Domain = page.receipt, page.panorama, page.domain
		if data, err = json.Marshal(resp); err != nil {
			return
		}
	}
	if len(data) > panoramaPageSoftBytes {
		m.warnf("getPanorama 单条目页实测 %s 字节超软界 %s（section=%s offset=%s）",
			len(data), panoramaPageSoftBytes, page.receipt.Section, page.receipt.Offset)
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
		page, err := m.panoramaPageOf(sess, req)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Page = page.receipt
		resp.Panorama = page.panorama
		resp.Domain = page.domain
		if page.overview && page.receipt.Offset == 0 {
			// Decisions/Diff 仅 overview 分节首页随答（会话决策态与差异高亮随概览
			// 首屏一次到达，后续页与明细分节不重复携带）
			resp.Decisions = decisionsViewOf(sess)
			resp.Diff = sess.LastDiff
		}
		// 测量硬门：估算首遍打包后实测完整应答字节，超软界从尾部卸条目
		m.boundPanoramaPage(resp, page)
	case workbenchActionAssign:
		sess, view, err := m.ApplyAssignment(req.Root, req.RelPath, SessionAssignment{
			Author: req.Author, AuthorRole: req.AuthorRole, Title: req.Title,
			Tags: req.Tags, Collection: req.Collection,
			AuthorClueRef: req.AuthorClueRef, TitleClueRef: req.TitleClueRef,
			CollectionClueRef: req.CollectionClueRef, TagClueRefs: req.TagClueRefs,
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
	case workbenchActionAssignDir:
		sess, affected, err := m.ApplyDirAssignment(req.Root, req.DirRelKey, SessionAssignment{
			Author: req.Author, AuthorRole: req.AuthorRole, Title: req.Title,
			Tags: req.Tags, Collection: req.Collection,
			AuthorClueRef: req.AuthorClueRef, TitleClueRef: req.TitleClueRef,
			CollectionClueRef: req.CollectionClueRef, TagClueRefs: req.TagClueRefs,
		})
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.Decisions = decisionsViewOf(sess)
		resp.Affected = affected
	case workbenchActionClearDirAssignments:
		sess, affected, err := m.ClearDirAssignments(req.Root, req.DirRelKey)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.Decisions = decisionsViewOf(sess)
		resp.Affected = affected
	case workbenchActionSetIgnored:
		if _, err := m.SetIgnored(req.Root, req.RelPath, req.Ignored); err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		if sess, err := m.SessionOf(req.Root); err == nil {
			resp.Decisions = decisionsViewOf(sess)
		}
	case workbenchActionIgnoreCandidate, workbenchActionRestoreCandidate:
		// 候选分仓为快照态：动作登记会话清单并排一轮重识别，分仓清单随刷新
		// 到达（应答只回会话摘要与决策态，与赋值族通路一致）
		sess, err := m.SetCandidateIgnored(req.Root, req.Target, req.Action == workbenchActionIgnoreCandidate)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.Decisions = decisionsViewOf(sess)
	case workbenchActionConfirmCandidate:
		sess, affected, err := m.ConfirmWorkSetCandidate(req.Root, req.Target, req.SetName, req.ExistingSetID)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.Decisions = decisionsViewOf(sess)
		resp.Affected = affected
	case workbenchActionUndoConfirm:
		sess, affected, err := m.UnconfirmWorkSetCandidate(req.Root, req.Target)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Session = summarizeSession(sess)
		resp.Decisions = decisionsViewOf(sess)
		resp.Affected = affected
	case workbenchActionSummarizeWorkSetRule:
		sugg, err := m.ApplyWorkSetRuleSummary(req.Root, req.Target)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.SetRuleSuggestion = sugg
	case workbenchActionGeneralizeWorkSetRule:
		result, err := m.ApplyWorkSetRuleGeneralize(req.Root, req.Target)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Rule = result.Rule
		resp.Rules = result.Rules
		resp.Started = result.VerificationStarted
	case workbenchActionSaveWorkSetRuleAdjust:
		result, err := m.ApplyWorkSetRuleAdjust(req.Root, req.Target, setRuleAdjustInput{
			AuthorClueRef:     req.AuthorClueRef,
			TitleClueRef:      req.TitleClueRef,
			TagClueRefs:       req.TagClueRefs,
			CollectionSegRefs: req.CollectionSegRefs,
			VariableSegRefs:   req.VariableSegRefs,
			SeqSegRef:         req.SeqSegRef,
			DirAnchorRef:      req.DirAnchorRef,
			MemberOnly:        !req.SetRuleMemberOnlyOff,
		})
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Rule = result.Rule
		resp.Rules = result.Rules
		resp.Started = result.VerificationStarted
	case workbenchActionGeneralize:
		// relPath 缺省=目录寻址（dirRelKey 子树，样本=首个有赋值的文件）。
		var result *GenerationResult
		var err error
		if req.RelPath == "" {
			result, err = m.ApplyDirGeneralization(req.Root, req.DirRelKey, req.GeneralizeMode)
		} else {
			result, err = m.ApplyGeneralization(req.Root, req.RelPath, req.GeneralizeMode)
		}
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Rule = result.Rule
		resp.Rules = result.Rules
		resp.Started = result.VerificationStarted
	case workbenchActionSummarizeRule:
		// relPath 缺省=目录寻址（建议卡 RelPath 呈现目录键）。
		var sugg *RuleSuggestion
		var err error
		if req.RelPath == "" {
			sugg, err = m.ApplyDirStructSummary(req.Root, req.DirRelKey)
		} else {
			sugg, err = m.ApplyStructSummary(req.Root, req.RelPath)
		}
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RuleSuggestion = sugg
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
		diff := sess.LastDiff
		if diff == nil {
			// 未重识别过（首轮识别后无差异上下文）：空差异集（非 nil 序列化契约）；
			// 取局部变量不改会话——sess 为缓存共享只读实例，原地写会污染缓存
			diff = &SnapDiff{
				Changed: []SnapDiffEntry{}, Added: []SnapDiffEntry{}, Removed: []SnapDiffEntry{},
			}
		}
		resp.Diff = diff
	case workbenchActionSetSelection:
		sess, err := m.SetSelection(req.Root, req.Works)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Decisions = decisionsViewOf(sess)
	case workbenchActionCreateTasks:
		// 异步化（设计 C）：守卫链通过后立即应答「已开始创建」，创建 RPC 在独立
		// goroutine 执行（订阅循环不再被长创建阻塞），进度与终态经
		// createProgress/createDone 事件推送
		started, err := m.ApplyCreateTasks(req.Root, req.TaskName)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.Started = started
		if sess, err := m.SessionOf(req.Root); err == nil {
			resp.Decisions = decisionsViewOf(sess)
		}
	case workbenchActionCancelCreate:
		// 取消在途创建（甲3/决策3）：创建在独立 goroutine，请求循环可自由应答——
		// 受理结论三态：cancelling=取消已受理（终态经 createDone 事件）、completed=
		// 恰好已完成（原子提交点已过，未执行取消，风险10）、idle=无在途创建
		state := requestCreateCancel(req.Root)
		switch state {
		case createCancelIdle:
			resp.OK, resp.Error = false, "无在途创建可取消"
		default:
			resp.CancelState = state
		}
	case workbenchActionGetRules:
		view, err := m.RulesAdminView()
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulesView = view
	case workbenchActionPreviewRule:
		preview, err := m.PreviewRule(req.Root, req.Rule)
		if err != nil {
			resp.OK, resp.Error = false, err.Error()
			break
		}
		resp.RulePreview = preview
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
