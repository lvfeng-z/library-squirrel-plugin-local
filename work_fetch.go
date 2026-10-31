package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
	"github.com/lvfeng-z/library-squirrel-sdk/transport"
)

// currentPluginDataVersion 当前插件支持的 task PluginData 格式版本
// v2：FilePluginData 增加可选归属字段（siteKey/siteWorkId，扫描期归属裁决结果）；
// v3：FilePluginData 增加任务级赋值快照（assignment，工作台创建时固化的会话赋值，
// 执行期重算优先消费）；v0/v1/v2 旧数据无对应字段，读侧按零值走既有路径
// （无归属=local 回退、无赋值快照=纯档案求值）
const currentPluginDataVersion = 3

// checkPluginDataVersion 校验 task PluginData 格式版本：
// 0(旧数据，引入版本约定前写入)与当前支持版本走现有逻辑；高于当前=由更新版本插件写入，拒绝执行防静默数据损坏。
// 主程序不解析 PluginData 内容，版本兼容性由插件自负（见 doc/plugin-dev-guide.md「plugin_data 格式版本约定」）。
func checkPluginDataVersion(v int) error {
	if v > currentPluginDataVersion {
		return fmt.Errorf("任务 PluginData 由更高版本插件创建(schemaVersion=%d，当前支持 %d)，请升级插件或重建任务", v, currentPluginDataVersion)
	}
	return nil
}

// FilePluginData 文件级 PluginData
type FilePluginData struct {
	SchemaVersion int           `json:"schemaVersion,omitempty"` // plugin_data 格式版本；0=旧数据(引入版本约定前写入)，1=引入版本约定，2=增加归属字段，3=增加任务级赋值快照
	FullPath      string        `json:"fullPath"`
	RelPath       string        `json:"relPath"`
	Hash          string        `json:"hash"`
	Size          int64         `json:"size"`
	Metadata      []PathMeaning `json:"metadata,omitempty"`
	// SiteKey/SiteWorkId 真实域归属（扫描期站点分类与文件名形态解析的裁决结果）：
	// SiteKey=归属站点键、SiteWorkId=页级作品 ID。两字段空=local 回退，作品身份=Hash
	SiteKey    string `json:"siteKey,omitempty"`
	SiteWorkId string `json:"siteWorkId,omitempty"`
	// Assignment 任务级赋值快照：工作台创建时该文件的会话赋值固化进任务载荷
	//（插件自有字段，零契约）。执行期重算优先按它做「档案∪赋值」合并求值的
	// 赋值覆盖——已建任务自包含，会话删除或赋值清理不影响在途任务的元数据。
	Assignment *SessionAssignment `json:"assignment,omitempty"`
}

// hasRealAttribution PluginData 是否携带真实域归属（裁决出真实站点与页级作品 ID）
func (fp *FilePluginData) hasRealAttribution() bool {
	return fp.SiteKey != "" && fp.SiteWorkId != ""
}

// DirPluginData 目录级 PluginData（用于 parent task）
type DirPluginData struct {
	SchemaVersion int           `json:"schemaVersion,omitempty"` // plugin_data 格式版本；0=旧数据(引入版本约定前写入)，1=当前
	DirRelPath    string        `json:"dirRelPath"`
	Metadata      []PathMeaning `json:"metadata"`
}

// LocalImportWorkFetcher 本地文件导入作品拉取扩展
type LocalImportWorkFetcher struct {
	ctx        sdkdto.PluginContext
	classifier *PathClassifier
	readers    sync.Map // taskID → *os.File

	siteKeyMu   sync.Mutex
	siteKeyByID map[int64]string // 站点 DB 行 id → 站点键（ListSites 注册表投影一次拉取的缓存）

	// sessions 工作台会话管理器（Activate 装配）；Create 入口按会话分派三分支
	sessions *SessionManager
	// entryConfirm Create 流内入口确认弹窗协调器（Activate 装配）
	entryConfirm *entryConfirmCoordinator

	// hostContractFn 宿主契约版本读取（握手降级判定用）：缺省走 SDK 进程级访问器
	// transport.HostContractVersion——Activate 协商值（ActivateRequest.host_contract_version）
	// 已由 SDK 侧在 Activate 时落进程级原子值，插件无需自记；单测注入模拟旧宿主（<21）
	hostContractFn func() uint32

	// 执行期重算的库内谓词索引缓存（CreateWorkInfo 按任务逐个调用，逐次全量
	// 分页拉取四轨会放大库查询；TTL 窗口内复用，见 libraryIndexCached）
	libMu      sync.Mutex
	libIndex   *LibraryIndex
	libBuiltAt time.Time

	// —— 大选择集创建链（阶段4）单测注入点（零值=生产默认）——
	// progressNow 进度时间节流时钟（nil=time.Now；节流时序锚单测注入步进时钟）。
	progressNow func() time.Time
	// verifyHeartbeatTick verify 期心跳节拍（0=生产 20s；心跳在场单测注入短节拍）。
	verifyHeartbeatTick time.Duration
	// childrenBatchByteLimit / childrenBatchCountLimit 单批子行双界（0=生产
	// 16MB / 2 万条；流水线重叠期单测注入小值驱动多批）。
	childrenBatchByteLimit int
	childrenBatchCountLimit int
}

// libraryIndexTTL 谓词索引缓存时长。
const libraryIndexTTL = 30 * time.Second

// libraryIndexCached 库内谓词索引（带 TTL 缓存）：识别干跑（runFullRecognition
// 一次性构造）与执行期重算（recomputeFileDecision 经 CreateWorkInfo 按任务调用）
// 共用 BuildLibraryIndex 单一构造——索引注入对齐风险12（全景/创建/重算三点同一
// 谓词求值基面）。上下文不可用时返回 nil（谓词规则不命中，与现状降级一致）。
func (h *LocalImportWorkFetcher) libraryIndexCached() *LibraryIndex {
	h.libMu.Lock()
	defer h.libMu.Unlock()
	if h.libIndex != nil && time.Since(h.libBuiltAt) < libraryIndexTTL {
		return h.libIndex
	}
	if h.ctx == nil {
		return nil
	}
	lib := BuildLibraryIndex(h.ctx, func(format string, args ...any) {
		h.ctx.Warnf(format, args...)
	})
	h.libIndex, h.libBuiltAt = lib, time.Now()
	return lib
}

// resolveSiteKeyByID 站点 DB 行 id → 站点键。面板站点下拉选择项的 value 是站点 DB 行 id，
// 而跨库身份是站点键，经 ListSites 注册表投影解析；不可解析（空 id/查询失败/查无此行）
// 一律按无站点分类处理，作品走 local 回退。
func (h *LocalImportWorkFetcher) resolveSiteKeyByID(idStr string) string {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return ""
	}
	h.siteKeyMu.Lock()
	defer h.siteKeyMu.Unlock()
	if h.siteKeyByID == nil {
		if h.ctx == nil {
			return ""
		}
		resp, err := h.ctx.ListSites()
		if err != nil {
			h.ctx.Warnf("查询站点列表失败，站点分类含义按无站点分类处理: %v", err)
			return ""
		}
		byID := make(map[int64]string, len(resp.Items))
		for _, s := range resp.Items {
			if s != nil && s.SiteKey != "" {
				byID[s.Id] = s.SiteKey
			}
		}
		h.siteKeyByID = byID
	}
	return h.siteKeyByID[id]
}

// localImportPathOf 从创建 URL 提取本地路径：剥离 local:// 前缀，余者原样。
func localImportPathOf(url string) string {
	if len(url) >= 8 && url[:8] == "local://" {
		return url[8:]
	}
	return url
}

// intentOnlyResult 构造零任务、以意图信号收尾的创建结果。
func intentOnlyResult(kind sdkdto.CreateIntentKind, message string) *sdkdto.TaskCreateResult {
	result := sdkdto.BatchResult(nil)
	result.SetIntent(kind, message)
	return result
}

// 编译期锚定：实现 Create 流等待期心跳接入接口（HeartbeatCreateFetcher）——
// 入口确认弹窗等待期经 reporter 周期上报心跳维持宿主 60 秒空闲窗。
var _ sdkdto.HeartbeatCreateFetcher = (*LocalImportWorkFetcher)(nil)

// CreateWithHeartbeat 工作台入口的创建分派（三分支）：
//
//  1. 工作台发起（会话带发起标记）→ 过滤产出分支：按会话识别快照推导选择域、
//     只产出所选作品（集父行按所选成员生成、任务级赋值快照固化），零弹问；
//     verify 哈希循环按 reporter 心跳保活（20s 节拍，甲4——工作台长创建不再
//     误触宿主 60s 空闲窗），取消位经 cancelCreate 工作台动作置位（甲3）；
//  2. 库根命中既有会话 → 不重问，意图信号「转入解析」收尾（零任务）；
//  3. 无会话 → Create 流内弹确认（二选：开始解析〔默认〕/取消），等待期心跳
//     保活 + 插件侧 5 分钟超时（超时=取消）；采纳即建会话并以「转入解析」收尾，
//     取消/超时以「已取消」收尾——两态均零任务零落库。
//
// 全量扫描直出的旧创建路径已随工作台过滤产出接线退役删除（决策6：localImport
// 创建唯一路径=工作台会话模式）；目录问答分类（ClassifyDir）在会话模式下结构性
// 不可达——过滤产出分支只消费识别快照内规则回放结论，不经问答取含义。
func (h *LocalImportWorkFetcher) CreateWithHeartbeat(url string, reporter sdkdto.HeartbeatReporter) (*sdkdto.TaskCreateResult, error) {
	if h.sessions == nil {
		return nil, fmt.Errorf("会话管理器未装配，无法处理创建: %s", url)
	}
	root := localImportPathOf(url)

	// 分支 1：工作台发起的创建（会话+选择集在）→ 过滤产出（reporter 随传：
	// verify 循环心跳接线，声明7 的两参调用缺口在此闭合）
	if h.sessions.ConsumeWorkbenchCreateMarker(root) {
		return h.createWorkbenchFiltered(url, root, reporter)
	}

	// 分支 2：库根命中既有活动会话 → 直接进既有会话，不重问
	if _, err := h.sessions.SessionOf(root); err == nil {
		return intentOnlyResult(sdkdto.CreateIntentToParse,
			"该库根已有进行中的解析会话：请从侧栏「本地导入工作台」进入继续"), nil
	} else if !errors.Is(err, ErrWorkbenchSessionNotFound) {
		return nil, err
	}

	// 分支 3：无会话 → Create 流内弹确认（等待期心跳保活）
	outcome := h.entryConfirm.Await(root, reporter)
	if !outcome.accepted {
		return intentOnlyResult(sdkdto.CreateIntentCancelled, outcome.cancelMessage()), nil
	}
	if _, err := h.sessions.CreateSession(root); err != nil {
		return nil, fmt.Errorf("建立解析会话失败: %w", err)
	}
	return intentOnlyResult(sdkdto.CreateIntentToParse,
		"已开始解析：解析会话已建立，正在进入解析视图（亦可从侧栏「本地导入工作台」进入）"), nil
}

// createWorkbenchFiltered 工作台发起创建的过滤产出分支：从会话识别快照推导
// 选择域（作品原子结构——集成员单文件作品/目录聚合多轨作品），按选择集产出批次
// 应答组（emitSelectedWorks——按站点分组的单父多子行，一次创建=一个父任务）；
// 任务名取编排动作暂存的原文（ApplyCreateTasks 经宿主 CreateTask RPC 递归进入本
// 分支时在位），缺省/全空白按默认名兜底（与编排侧同一裁决函数，前端预填与后端
// 兜底同规则）；所选文件依赖的会话赋值固化为任务级赋值快照（FilePluginData.
// Assignment，执行期重算优先消费）；目录含义取快照规则回放结论（dirMeaningChains），
// 全程零弹问——对 ClassifyDir 结构性零调用。作品结构（分组归属）以识别快照为
// 求值基面（干跑 MergedRecognition 注入索引与赋值的结论缓存，创建前经编排动作
// 对账核验磁盘与档案/赋值修订一致）；最终元数据在执行期重算（recomputeFileDecision
// 注入同一索引与赋值快照）产出——三点同一谓词与合并基面（风险12/13 闭环）。
// 流式逐组产出（内容哈希只对所选作品计算，未选资源零哈希零任务）；verify 并行
// 化、取消收流与心跳保活在 emitSelectedWorks（reporter 随传）。
// 取消收尾形态（甲3/决策3）：取消位置位后发射循环提前收流（末批 last 不发），
// 宿主未收批统一回滚在途行，本流以意图块「已取消」收尾（零任务意图先例 :205
// 同款）；零发射阶段取消则无开批父无行可删、直接终态。
func (h *LocalImportWorkFetcher) createWorkbenchFiltered(url, root string, reporter sdkdto.HeartbeatReporter) (*sdkdto.TaskCreateResult, error) {
	sess, err := h.sessions.SessionOf(root)
	if err != nil {
		return nil, fmt.Errorf("读取解析会话失败: %w", err)
	}
	if sess.Snapshot == nil {
		result := sdkdto.BatchResult(nil)
		result.SetReason("该会话尚未完成全量识别，工作台发起的创建无选择集可产出")
		return result, nil
	}
	if len(sess.Selection) == 0 {
		result := sdkdto.BatchResult(nil)
		result.SetReason("工作台发起的创建未携带有效选择集（选择集为空），本次未产出任务")
		return result, nil
	}
	// 选择域求值接线缓存条目（守卫链与会话瘦身四刀之三）：会话是缓存共享实例，
	// 经 workDomainOf 复用全景载入时推导过、条目内驻留的结果（零重推导）。
	domain := h.sessions.workDomainOf(sess)
	works, missing := domain.ResolveAll(sess.Selection)
	if len(missing) > 0 {
		result := sdkdto.BatchResult(nil)
		result.SetReason(fmt.Sprintf("选择集中 %d 项已不在当前识别全景内（磁盘或识别结构可能已变化），本次未产出任务；请重新识别并重选", len(missing)))
		return result, nil
	}

	// 任务名：编排动作暂存的原文经本分支消费（宿主 CreateTask RPC 同步递归覆盖）；
	// 缺席（测试直驱等未暂存路径）或空白按默认名兜底——单一裁决点见 resolveCreateTaskName
	taskName := resolveCreateTaskName(h.sessions.takeStagedCreateTaskName(root), root, len(works))

	// B1+ 分帧发射（条目形态流：task 块+子行批次块经 CreateStreamItem 装配）；
	// 中止原因（握手降级的载荷超限/磁盘对账未通过）经 SetReason 以 error 块收流、
	// 取消经 SetIntent 以意图块收尾（取消与完成竞态下意图不成立——末批 last 已
	// 发即原子提交点，取消请求在编排侧应答「已完成」，风险10）——两者均须在
	// close(ch) 前调用（消费侧的读取依赖 channel close 的 happens-before 顺序）
	ch := make(chan *sdkdto.CreateStreamItem, 16)
	result := sdkdto.StreamItemsResult(ch)
	go func() {
		reason, cancelled := h.emitSelectedWorks(root, sess, works, taskName, ch, reporter)
		switch {
		case cancelled:
			result.SetIntent(sdkdto.CreateIntentCancelled,
				"已取消本次创建：未发射的子任务不再产出，已发射的在途任务由主程序统一回滚清理")
		case reason != "":
			result.SetReason(reason)
		}
		close(ch)
	}()
	return result, nil
}

// batchFileItem 批次应答的子行素材：带归属裁决的文件与其所属目录的含义链。
// 批次父行纯容器（目录含义链不呈现于父行），含义链随子行各自的目录走。
type batchFileItem struct {
	af       attributedFile
	metadata []PathMeaning
}

// —— 大批次子行分帧（契约 21 B1+，大载荷分页方案·设计 B）——

const (
	// createChildrenBatchMinHostContract 子行批次块（children_batch）所需的最低
	// 宿主契约版本：握手协商值低于此（含未 Activate 的 0）时降级为现状单块全量
	// 内联形态——捆绑发布形态下宿主恒 ≥21，降级路径仅兜异常组合（旧宿主手装等）。
	createChildrenBatchMinHostContract = 21
	// createChildrenBatchByteTarget 单批子行的序列化字节目标（16MB）：对 64MB
	// 断路器留 4 倍余量；估和为保守上界（childWireSizeUpper），实际线尺寸只会更小。
	createChildrenBatchByteTarget = 16 << 20
	// createChildrenBatchCountLimit 单批子行条数上限（2 万）：字节目标之外的
	// 第二道界，约束宿主侧单父聚合的峰值行数。
	createChildrenBatchCountLimit = 20000
	// degradeChildrenInlineLimit 握手降级（宿主契约 <21）时单块全量内联的序列化
	// 字节上限（60MB）：超限返回中止原因明确报错（宿主契约过旧且载荷超限），
	// 不静默撞 64MB 断路器。
	degradeChildrenInlineLimit = 60 << 20
	// createProgressMinInterval createProgress 事件的时间基节流间隔（甲1）：两相
	// 各自 ≥2s 推一档、终值恒发（末发必须恒发且晚于一切中间值，风险7）；事件
	// 总量上限≈全程秒数/2，通道负载可控。计数步长节流（原 1000 步长）已退役
	//（发射进度槽仍在 noteBatch 内逐批落账，不受节流影响）。
	createProgressMinInterval = 2 * time.Second
	// createVerifyHeartbeatTick verify 相心跳节拍（甲4）：宿主泵首块后 60s 空闲窗
	// 豁免心跳块（心跳 ≥20s 节拍即保活）；SDK 上报器内置 20s 限频，本节拍恰每窗
	// 一块——插件死亡/读挂起时 60s 内失败终态。残余已知：读挂起的单个 goroutine
	// Windows 无强制中断原语，泄漏至进程重启（闸门与终态不受影响）。
	createVerifyHeartbeatTick = 20 * time.Second
)

// errCreateCancelled 取消位在哈希单元内被观察到的中止哨兵（甲3：verify 循环
// 作品间与 4MB 分块处的检查点统一映射为本哨兵，放行循环据此转取消收尾）。
var errCreateCancelled = errors.New("创建已取消")

// hostContractVersion 宿主契约版本（Activate 协商值，SDK 进程级记录；
// hostContractFn 缺省即取访问器——未 Activate 时为 0，按保守降级处理）。
func (h *LocalImportWorkFetcher) hostContractVersion() uint32 {
	if h.hostContractFn != nil {
		return h.hostContractFn()
	}
	return transport.HostContractVersion()
}

// childWireSizeUpper 子行序列化字节的保守上界（分帧估和用）：proto 线形态为逐
// 字符串字段「tag+varint 长度+内容」（Go 字符串 len 即 UTF-8 字节数，与线编码
// 内容一致），估和=全部字符串字段字节和+每字段垫头+消息级固定头——估和恒 ≥
// 实际线尺寸，按此切块的批不会超出字节目标。
func childWireSizeUpper(child *sdkdto.TaskCreateChildResponse) int {
	const perFieldOverhead = 8 // tag(≤2B)+长度 varint(≤5B)+余量
	size := len(child.TaskName) + len(child.SiteWorkId) + len(child.Url) + len(child.PluginData) +
		len(child.SiteName) + len(child.ResourceType) + len(child.SiteKey) + 7*perFieldOverhead
	for _, role := range child.InvolvedRoles {
		size += len(role) + perFieldOverhead
	}
	return size + 16
}

// splitChildBatches 子行分帧切批：每批同时受序列化字节目标（createChildrenBatchByteTarget）
// 与条数上限（createChildrenBatchCountLimit）双界约束；首条必装保证推进（单条超目标的
// 巨子行独立成批）。恰一批=单批装得下（纯内联形态，无批次块）。
func splitChildBatches(children []*sdkdto.TaskCreateChildResponse) [][]*sdkdto.TaskCreateChildResponse {
	batches := make([][]*sdkdto.TaskCreateChildResponse, 0, 1)
	var cur []*sdkdto.TaskCreateChildResponse
	var curBytes int
	for _, child := range children {
		if len(cur) > 0 && (len(cur) >= createChildrenBatchCountLimit || curBytes+childWireSizeUpper(child) > createChildrenBatchByteTarget) {
			batches = append(batches, cur)
			cur, curBytes = nil, 0
		}
		cur = append(cur, child)
		curBytes += childWireSizeUpper(child)
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

// emitChildBatches 发射子行批次序列（B1+ 单一代码路径）：父块（TaskItem）必带
// 首批子行内联（执行期裁决 2026-10-29：保住宿主「无 children→独立叶」既有判定，
// 不引入空 children 父声明），余批经 ChildrenBatchItem 发块（pluginTaskId 指称
// 父声明所用 ID，末批 last=true）；恰一批时循环体不执行=纯内联形态（pixiv/
// bilibili 单块内联同构）。onBatch 在每批发射后回调（emitted/total=累计已发射
// 子行/计划子行总数——创建相进度的数据源）。
func emitChildBatches(ch chan<- *sdkdto.CreateStreamItem, parentID, taskName, parentURL, siteKey string,
	batches [][]*sdkdto.TaskCreateChildResponse, onBatch func(emitted, total int)) {
	if len(batches) == 0 {
		return
	}
	total := 0
	for _, batch := range batches {
		total += len(batch)
	}
	ch <- sdkdto.TaskItem(buildBatchParentResponse(parentID, taskName, parentURL, siteKey, batches[0]))
	emitted := len(batches[0])
	if onBatch != nil {
		onBatch(emitted, total)
	}
	for i := 1; i < len(batches); i++ {
		ch <- sdkdto.ChildrenBatchItem(parentID, batches[i], i == len(batches)-1)
		emitted += len(batches[i])
		if onBatch != nil {
			onBatch(emitted, total)
		}
	}
}

// unitHashOutcome 单件作品哈希三态结论（verify 哈希循环作品粒度）。
type unitHashOutcome struct {
	// files / ok ok=true=全部轨道哈希完成；ok=false 且 err=nil=哈希计算失败整件
	// 放弃（多轨作品轨道集原子，结构上不产出半作品；单轨集成员作品同口径，
	// 失败仅弃该成员自身——v1 口径），dropErr 携带首失败原因（聚合上报样本）。
	files   []FileEntry
	ok      bool
	dropErr error
	// err 非 nil=整次创建中止：轨道 Stat 失败=所选文件已不存在（磁盘对账未
	// 通过——对账拆分·甲2 刀4 的逐文件存在性核对在本循环，文案经 err 由调用
	// 方收流），或取消位（errCreateCancelled/context.Canceled）。
	err error
}

// indexedUnitOutcome 重排缓冲条目：域序下标 + 单件结论（按下标逐件放行——
// 保序不变量：并发度只影响速度不影响子行序）。
type indexedUnitOutcome struct {
	idx int
	unitHashOutcome
}

// hashDropSample 哈希失败作品样本（风险6：并行下逐件告警会洪泛日志，聚合成批
// 上报的载荷）。
type hashDropSample struct {
	workID string
	err    error
}

// progressThrottle createProgress 事件的时间基节流器（甲1）：间隔内不推、终值
// 恒推（末发必须恒发且晚于一切中间值，风险7）。发布侧只在放行循环单线程调用，
// 无并发；now 可注入（时序锚单测）。
type progressThrottle struct {
	interval time.Duration
	now      func() time.Time
	started  bool
	last     time.Time
}

// allow 是否放行一次发布；final=终值恒放行（不做时钟读写，终值不受时钟注入影响）。
func (p *progressThrottle) allow(final bool) bool {
	if final {
		return true
	}
	now := p.now()
	if !p.started || now.Sub(p.last) >= p.interval {
		p.started, p.last = true, now
		return true
	}
	return false
}

// emitPipeline 发射流水线（乙3）：verify 按域序放行的子行逐批组装发射——双界
//（16MB/2 万条）满批即发，落库与剩余哈希重叠；批界裁决与 splitChildBatches
// 同款（既有分帧函数不动，此处为流式增量的同一规则）。父块必带首批子行内联
//（与 emitChildBatches 同形态，保宿主「无 children→独立叶」判定）；末批
// last=true（契约 21 收批声明）。取消在场（ctx 已取消）时丢弃后续子行并置
// aborted——末批 last 不发=宿主「流结束未收批」统一回滚（决策3/决策4）。
type emitPipeline struct {
	ctx        context.Context
	ch         chan<- *sdkdto.CreateStreamItem
	taskName   string
	parentURL  string
	siteKey    string
	byteTarget int
	countLimit int
	// note 每批发射回调（emitted=累计已发射子行；final=末批）：进度槽逐批落账
	// 不受节流（部分完成信息源），事件发布经 createPace 时间基节流。
	note func(emitted int, final bool)

	cur      []*sdkdto.TaskCreateChildResponse
	curBytes int
	batchIdx int // 已发批数（0=父块未发）
	emitted  int
	batchMs  int64 // 父身份与集序记录同源毫秒时间戳（首批发射时落定）
	aborted  bool
}

// add 追加一子行：双界满批即先发一批（非末批，last=false——触发本调用的子行
// 保证后续仍有内容，末批恒由 finish 以 last=true 发出）。
func (p *emitPipeline) add(child *sdkdto.TaskCreateChildResponse) {
	if p.aborted {
		return
	}
	if p.ctx.Err() != nil {
		p.aborted = true
		return
	}
	if len(p.cur) > 0 && (len(p.cur) >= p.countLimit || p.curBytes+childWireSizeUpper(child) > p.byteTarget) {
		if !p.flush(false) {
			return
		}
	}
	p.cur = append(p.cur, child)
	p.curBytes += childWireSizeUpper(child)
}

// flush 发射当前批：首批随父块内联、余批经 children_batch（last 声明末批）。
// 返回假=取消在场已中止。空批为无操作（零子行或界满批恰发完）。发射通道无
// 消费方（宿主创建流已终结且收流侧停摆）时背压阻塞可被取消解除——取消在场
// 按中止收流，发射不随通道滞留。
func (p *emitPipeline) flush(last bool) bool {
	if p.ctx.Err() != nil {
		p.aborted = true
		return false
	}
	if len(p.cur) == 0 {
		return true
	}
	if p.batchMs == 0 {
		p.batchMs = time.Now().UnixMilli()
	}
	parentID := fmt.Sprintf("local-batch-%d", p.batchMs)
	item := sdkdto.TaskItem(buildBatchParentResponse(parentID, p.taskName, p.parentURL, p.siteKey, p.cur))
	if p.batchIdx > 0 {
		item = sdkdto.ChildrenBatchItem(parentID, p.cur, last)
	}
	select {
	case p.ch <- item:
	case <-p.ctx.Done():
		p.aborted = true
		return false
	}
	p.emitted += len(p.cur)
	p.cur, p.curBytes = nil, 0
	p.batchIdx++
	if p.note != nil {
		p.note(p.emitted, last)
	}
	return true
}

// finish 收尾：待发批以末批（last=true）发出；取消在场或已中止返回假（零末批
// 收流=宿主回滚）。零子流（零发射）为合法收尾。
func (p *emitPipeline) finish() bool {
	if p.aborted || p.ctx.Err() != nil {
		p.aborted = true
		return false
	}
	return p.flush(true)
}

// progressClock 进度节流时钟（注入点；缺省 time.Now）。
func (h *LocalImportWorkFetcher) progressClock() func() time.Time {
	if h.progressNow != nil {
		return h.progressNow
	}
	return time.Now
}

// verifyHeartbeatTickOrDefault verify 期心跳节拍（注入点；缺省 20s）。
func (h *LocalImportWorkFetcher) verifyHeartbeatTickOrDefault() time.Duration {
	if h.verifyHeartbeatTick > 0 {
		return h.verifyHeartbeatTick
	}
	return createVerifyHeartbeatTick
}

// childrenBatchBounds 单批子行双界（注入点；缺省 16MB / 2 万条）。
func (h *LocalImportWorkFetcher) childrenBatchBounds() (byteTarget, countLimit int) {
	byteTarget, countLimit = createChildrenBatchByteTarget, createChildrenBatchCountLimit
	if h.childrenBatchByteLimit > 0 {
		byteTarget = h.childrenBatchByteLimit
	}
	if h.childrenBatchCountLimit > 0 {
		countLimit = h.childrenBatchCountLimit
	}
	return byteTarget, countLimit
}

// emitSelectedWorks 按选择集产出批次应答（一次创建=一个父任务，B1+ 分帧形态）：
// 文件按选择域确定序（目录按域序，目录内先目录聚合作品后集成员〔域序=集内序〕）
// 逐个裁决归属后归集——站点键=identity.Local.Key（创建域），子行归属经各自
// siteKey 字段声明：真实域归属子行携归属站点键（作品身份落库域，仍受宿主站点
// 表存在性校验），local 回退子行留空（继承父应答站点键）。
//
// 产出形态（单一代码路径，契约 21）：父块必带首批子行内联，余下子行按「序列化
// 字节目标+条数上限」切 children_batch 块（plugin_task_id=父声明所用 ID，末批
// last=true）；单批装得下时=纯内联形态（无批次块）。握手降级：宿主契约协商值
// <21（含未 Activate 的 0）时不分帧、按现状单块全量内联（子行序列化 ≤60MB 时），
// 超限返回中止原因「宿主契约过旧且载荷超限」由调用方以 error 块收流。
//
// 阶段4 结构（乙1 并行 + 乙3 流水线 + 甲1 进度 + 甲3 取消 + 甲4 心跳）：
//   - verify 并行化：作品粒度 worker 池消费域序单元序列（并发度=爬坡控制器，
//     决策1 三层），完成件进重排缓冲、放行循环按域序逐件放行——进度计数=放行
//     计数，并行输出与串行基准逐字节全等（保序不变量）；
//   - 发射流水线化：契约 ≥21 时放行即逐批组装发射（emitPipeline 双界切块），
//     落库与剩余哈希重叠；进度升级双计数模型（核对行 X/N+速率 与发射行子行数
//     并存，事件按 phase 维度分道，前端阶段6 消费）；
//   - 进度首反馈与时间节流：works 解析完成即发 createProgress(verify, 0, N)
//     （真分母即刻可见）；节流时间基 ≥2s 且必发终值（风险7）；速率字段=当前
//     窗口读盘字节/秒（爬坡控制器同一窗口统计，风险5 口径：只计真实读盘）；
//   - 取消通路：cancelCreate 工作台动作置位（经运行态登记表传播为本循环 runCtx
//     取消）——verify 循环作品间与 4MB 分块处（ComputeFileHashCtx）检查、发射
//     批间检查，取消即提前收流（末批 last 不发→宿主未收批统一回滚，决策3/4）、
//     意图块「已取消」收尾；零发射阶段取消则无开批父无行可删、直接终态；
//   - 挂死检测：放行循环按 20s 节拍上报心跳（甲4）——配合宿主 60s 空闲窗，插件
//     读挂起 60s 内失败终态（残余已知：挂起读的单 goroutine 无法中断，泄漏至
//     进程重启，闸门与终态不受影响）；
//   - 哈希缓存（乙2/决策2）：verify 开始一次载入内存（loadHashCache）、创建
//     结束一次回写+压实（saveHashCache，defer 覆盖全部收尾路径）——正常收尾按
//     本轮流见集合压实（流见集合即存在性事实），中止/取消合并保留旧条目并
//     叠加新算记录（取消→重试的命中面，交互剧本帧4）；命中判据与零读盘口径
//     见 hashWorkFilesAllOrNothing（风险5）。
//
// 逐文件存在性核对在本循环（对账拆分·甲2 刀4）：任一轨道 Stat 失败=所选文件
// 已不存在，整次创建立即中断、对账文案经返回值（中止原因→SetReason 以 error 块
// 收流——并行/取消路径下语义不回退：早放行单元已发射的批次无 last 末批，由
// 宿主未收批统一回滚，用户可见结局与中断一致）；哈希计算失败仍为整件跳过
//（半作品结构性不产出，v1 口径），失败样本聚合上报（风险6）。产出完成后向会话
// 管理器记录批次摘要（子行总数与非 local 子行的归属分布，编排动作通知文案的
// 数据源——宿主 CreateTask RPC 同步消费完整产出流后才返回，记录必先于编排
// 读取）；降级超限中止与取消中止时不产出、零簿记。
//
// 返回 (reason, cancelled)：reason 非空=中止原因（error 块收流）；cancelled 真=
// 取消收尾（意图块收尾）；两者皆空=正常收尾（哈希缓存按流见集合压实回写，
// 其余收尾形态合并保留——见函数内 defer）。
func (h *LocalImportWorkFetcher) emitSelectedWorks(root string, sess *WorkbenchSession, works []*SelectableWork, taskName string, ch chan<- *sdkdto.CreateStreamItem, reporter sdkdto.HeartbeatReporter) (reason string, cancelled bool) {
	chains := dirMeaningChains(sess.Snapshot)

	// 进度推送（createProgress）：经会话管理器发布事件（上下文不可用静默）；发射
	// 相进度同时在 noteBatch 内逐批落运行态进度槽（失败路径部分完成信息源）
	progress := func(phase string, done, total int, bytesPerSec int64) {
		if h.sessions != nil {
			h.sessions.noteCreateProgress(root, phase, done, total, bytesPerSec)
		}
	}
	// 甲1 进度首反馈：works 解析完成后立即发核对相首事件（真分母即刻可见，
	// 横幅不再「0 / 0」静默）
	progress(createPhaseVerify, 0, len(works), 0)

	// 乙2 哈希缓存（决策2）：verify 开始一次载入内存——置于首进度事件之后
	//（载入是分片 KV 读，15 万级清单可达秒级，不得挤占横幅首反馈）；创建
	// 结束一次回写+压实，defer 覆盖全部收尾路径（正常/中止/取消），压实口径
	// 按收尾形态裁决：正常收尾按本轮流见集合压实，中止/取消合并保留旧条目
	// 并叠加新算记录（取消→重试的命中面，交互剧本帧4）。
	var hcache *hashCache
	if h.sessions != nil {
		hcache = h.sessions.loadHashCache(root)
		defer func() {
			h.sessions.saveHashCache(root, hcache, reason == "" && !cancelled)
		}()
	}

	// 甲1 时间基节流：两相各自独立节流、共用同一可注入时钟（≥2s 且必发终值）
	progressNow := h.progressClock()
	verifyPace := &progressThrottle{interval: createProgressMinInterval, now: progressNow}
	createPace := &progressThrottle{interval: createProgressMinInterval, now: progressNow}
	verifyDone := 0
	noteVerify := func(rate int64) {
		verifyDone++
		if verifyPace.allow(verifyDone == len(works)) {
			progress(createPhaseVerify, verifyDone, len(works), rate)
		}
	}

	// 按目录组织产出计划：目录聚合作品与集成员分桶，目录序=选择域序
	type dirPlan struct {
		dirWork    *SelectableWork
		setMembers []*SelectableWork
	}
	plans := make(map[string]*dirPlan)
	var dirOrder []string
	for _, work := range works {
		plan := plans[work.DirRelKey]
		if plan == nil {
			plan = &dirPlan{}
			plans[work.DirRelKey] = plan
			dirOrder = append(dirOrder, work.DirRelKey)
		}
		if work.Kind == WorkKindDirTracks {
			plan.dirWork = work
		} else {
			plan.setMembers = append(plan.setMembers, work)
		}
	}

	// 集成员按选择域集内序排入批次：集间按集键字典序（与选择域集视图序一致）、
	// 集内按快照群组序号升序（群组序号从 1 起）。群组序号取值与集序记录同口径
	//（buildGroupSeqIndex：快照群卡按成员路径反查为主、文件行群组序兜底）——
	// 已识别成员的文件行不落群组键，只读文件行会取零而丢集内序。批次子行按
	// 集内序组织——任务树子项序与识别序一致；快照查不到群组序的成员（游标
	// 异常形态）序取零，排最前并保持传入稳定序。
	groupSeqByPath := buildGroupSeqIndex(sess.Snapshot)
	for _, dirKey := range dirOrder {
		setMembers := plans[dirKey].setMembers
		sort.SliceStable(setMembers, func(i, j int) bool {
			if setMembers[i].SetKey != setMembers[j].SetKey {
				return setMembers[i].SetKey < setMembers[j].SetKey
			}
			return groupSeqByPath[setMembers[i].Files[0]].seq < groupSeqByPath[setMembers[j].Files[0]].seq
		})
	}

	// 域序单元序列（乙1 作品粒度消费面）：目录序内先目录聚合作品后集成员
	//（集成员按上方集内序）——与串行基准同一序，单元只携计划素材（哈希在
	// worker 池内做，归属裁决与子行构造在放行侧做，保序不变量锚定放行序）
	type emitUnit struct {
		work              *SelectableWork
		metadata          []PathMeaning
		classifiedSiteKey string
	}
	units := make([]emitUnit, 0, len(works))
	for _, dirKey := range dirOrder {
		plan := plans[dirKey]
		metadata := chains[dirKey]
		// 站点分类含义 → 子树归属站点键（快照回放含义不含 site 型——site 仅经
		// 目录问答产生，会话模式问答不可达，此处恒走文件名形态归属裁决）
		classifiedSiteKey := resolveClassifiedSiteKey(metadata, h.resolveSiteKeyByID)
		if plan.dirWork != nil {
			units = append(units, emitUnit{work: plan.dirWork, metadata: metadata, classifiedSiteKey: classifiedSiteKey})
		}
		for _, member := range plan.setMembers {
			units = append(units, emitUnit{work: member, metadata: metadata, classifiedSiteKey: classifiedSiteKey})
		}
	}

	// 取消通路（甲3/决策3）：runCtx 的取消源有两个——cancelCreate 工作台动作经
	// 运行态登记表传播取消（requestCreateCancel 置位即调用登记的 cancelFn），与
	// 宿主创建流提前终结后编排侧的撤回（haltCreateRunEmission 直接调用同一
	// cancelFn，不置取消位）；原子提交点（末批 last 已发，markCreateCommitted）
	// 后的用户取消在请求侧即应答「已完成」，本循环不再回退已收批产出（风险10）
	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	armCreateRunCancel(root, runCancel)
	defer disarmCreateRunCancel(root)

	// 收尾信号：本循环全部退出路径（完成/中止/取消）经 defer 关闭——编排侧在
	// 宿主创建流提前终结后据此等待发射侧落定（终态事件发布前进度停摆、发射
	// 计数定格）；关闭先于哈希缓存回写（defer 后进先出），等待不覆盖缓存落盘
	runDone := armCreateRunDone(root)
	defer closeCreateRunDone(root, runDone)

	// 心跳独立节拍 goroutine：与放行循环解耦——释放环阻塞在发射通道背压时
	// 放行循环不排水，随循环驱动的节拍会随发射停摆；独立 goroutine 按节拍
	// 周期上报，不受释放环与通道背压影响（上报器并发安全，限频由上报器自持）。
	// Create 延迟发射形态下 SDK 上报器随 handler 返回即关闭，发射期上报在其
	// 关闭后为 no-op——发射期喂窗需上报器生命周期覆盖条目流排空（SDK 侧语义）
	hbStop := make(chan struct{})
	go func() {
		tick := time.NewTicker(h.verifyHeartbeatTickOrDefault())
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				if reporter != nil {
					reporter.Heartbeat()
				}
			case <-hbStop:
				return
			}
		}
	}()
	defer close(hbStop)

	// 爬坡控制器 + 动态信号量（乙1/决策1 三层：偏好钉死 > 按盘记忆 > 会话内
	// 自适应；控制器窗口只计真实读盘字节——风险5，速率字段同一窗口统计）
	throttle := h.resolveHashThrottle(root)
	sem := newDynSemaphore(throttle.limit())
	throttle.bindAdjust(sem.setLimit)

	// 供件与哈希 worker 池（乙1）：feeder 按域序供件、令牌闸门限并发（并发度
	// 随控制器收放）；单件哈希结论带域序下标进结果通道，由下方放行循环重排
	var wg sync.WaitGroup
	results := make(chan indexedUnitOutcome, hashConcurrencyPinMax)
	wg.Add(1)
	go func() { // feeder：域序供件（取消即止，不抢占在途件）
		defer wg.Done()
		for idx := range units {
			if !sem.acquire(runCtx) {
				return
			}
			if runCtx.Err() != nil { // 取令牌阻塞期间取消已到：归还退出
				sem.release()
				return
			}
			unit := units[idx]
			wg.Add(1)
			go func() { // 哈希 worker：单件 all-or-nothing，完成即投递（取消后弃置）
				defer wg.Done()
				defer sem.release()
				outcome := h.hashWorkFilesAllOrNothing(runCtx, root, unit.work, throttle, hcache)
				select {
				case results <- indexedUnitOutcome{idx: idx, unitHashOutcome: outcome}:
				case <-runCtx.Done():
				}
			}()
		}
	}()
	go func() { // 结果通道关闭哨（放行循环退出后 worker 仍可自然收尾）
		wg.Wait()
		close(results)
	}()

	// 发射形态裁决：契约 ≥21 走流水线（乙3 放行即逐批发射，落库与剩余哈希
	// 重叠）；<21（含未 Activate 的 0）降级为全量内联（verify 完成后单块发射，
	// 批界与超限中止语义不动）
	pipelined := h.hostContractVersion() >= createChildrenBatchMinHostContract
	byteTarget, countLimit := h.childrenBatchBounds()

	// 计划子行总数上界（发射行 total 的初值=全部轨道数；整件放弃时递减修正，
	// 终值收敛到实际产出）
	plannedTotal := 0
	for _, u := range units {
		plannedTotal += len(u.work.Files)
	}
	var allItems []batchFileItem     // 子行素材全集（集序记录面）
	var degradedChildren []*sdkdto.TaskCreateChildResponse // 降级形态的全量子行
	var dropSamples []hashDropSample // 哈希失败样本（风险6 聚合上报）
	totalBuilt := 0                  // 实际构造子行数（批次摘要 total）
	var nonLocalSiteKeys []string    // 真实域归属子行键序列（批次摘要归属分布）

	// 发射行进度：进度槽逐批落账（不受节流——部分完成信息源），事件经时间基
	// 节流（终值恒发，风险7）
	noteBatch := func(emitted int, final bool) {
		recordCreateEmitProgress(root, emitted, max(plannedTotal, emitted))
		if createPace.allow(final) && h.sessions != nil {
			h.sessions.publishCreateProgress(root, createPhaseCreate, emitted, max(plannedTotal, emitted), 0)
		}
	}
	var pipeline *emitPipeline
	if pipelined {
		pipeline = &emitPipeline{
			ctx: runCtx, ch: ch, taskName: taskName, parentURL: "local://" + root,
			siteKey: identity.Local.Key, byteTarget: byteTarget, countLimit: countLimit,
			note: noteBatch,
		}
	}

	// 放行循环：重排缓冲按域序逐件放行（进度计数=放行计数）——归属裁决、子行
	// 构造、流水线发射与核对相进度都在此单线程串行（确定性锚定）；心跳按 20s
	// 节拍上报（甲4，SDK 上报器内置 20s 限频）
	releaseOne := func(out indexedUnitOutcome) (string, bool) {
		unit := units[out.idx]
		if out.err != nil {
			if errors.Is(out.err, errCreateCancelled) || errors.Is(out.err, context.Canceled) {
				return "", true
			}
			return out.err.Error(), false
		}
		if !out.ok {
			// 整件放弃：计划总数修正 + 样本聚合（风险6：不逐件告警）
			plannedTotal -= len(unit.work.Files)
			dropSamples = append(dropSamples, hashDropSample{workID: unit.work.WorkID, err: out.dropErr})
		} else {
			// 逐文件裁决归属后按原序扁平进批次（目录序内先目录聚合作品后集
			// 成员——同目录文件不按归属重排）
			unitItems := make([]batchFileItem, 0, len(out.files))
			for _, f := range out.files {
				attr := resolveWorkAttribution(filepath.Base(f.FullPath), unit.classifiedSiteKey)
				unitItems = append(unitItems, batchFileItem{af: attributedFile{entry: f, attr: attr}, metadata: unit.metadata})
			}
			allItems = append(allItems, unitItems...)
			children := buildChildResponses(unitItems, sess.Assignments)
			totalBuilt += len(children)
			for _, child := range children {
				if child.SiteKey != "" {
					nonLocalSiteKeys = append(nonLocalSiteKeys, child.SiteKey)
				}
			}
			if pipelined {
				for _, child := range children {
					pipeline.add(child) // 双界满批即发（内部含取消检查）
					if pipeline.aborted {
						return "", true
					}
				}
			} else {
				degradedChildren = append(degradedChildren, children...)
			}
		}
		noteVerify(throttle.windowRate())
		return "", false
	}

	buffered := make(map[int]indexedUnitOutcome, hashConcurrencyPinMax)
	released := 0
	for released < len(units) {
		select {
		case out, ok := <-results:
			if !ok {
				// 结果通道关闭而放行未齐：仅取消路径可达（feeder/worker 因取消
				// 早退）——按取消收尾
				return "", true
			}
			buffered[out.idx] = out
		case <-runCtx.Done():
			// 取消源收流（cancelCreate 工作台动作或宿主创建流提前终结的撤回）：
			// 核对相/发射相（含重叠期）均在此收流——不发末批 last，宿主未收批
			// 统一回滚（决策3/4）；心跳节拍在独立 goroutine，随本返回关闭
			return "", true
		}
		for {
			next, ok := buffered[released]
			if !ok {
				break
			}
			delete(buffered, released)
			if reason, cancelled := releaseOne(next); reason != "" || cancelled {
				return reason, cancelled
			}
			released++
		}
	}

	// 哈希失败聚合上报（风险6）：并行下逐件告警会并发洪泛日志，聚合成批一次上报
	if len(dropSamples) > 0 && h.ctx != nil {
		sample := dropSamples[0]
		h.ctx.Warnf("所选文件哈希失败，已跳过其所属作品 %d 件（样本 %s: %v；完整清单见逐件排查）",
			len(dropSamples), sample.workID, sample.err)
	}

	// 计划总数收敛到实际产出（终值事件 X=N；发射槽终值同步）
	plannedTotal = totalBuilt
	if pipelined {
		// 末批收尾（last=true）；取消在场（收尾窗口内置位）则中止=未收批回滚
		if !pipeline.finish() {
			return "", true
		}
		if pipeline.batchMs > 0 {
			// 原子提交点（风险10）：末批 last 已发——其后到达的取消请求由请求
			// 侧应答「已完成」，创建照常成功收尾
			markCreateCommitted(root)
			// 集序记录：为有集归属的子行按集归集落记录（集内原站序作答的数据
			// 源，批次号=父行身份同源毫秒时间戳）；写入失败不阻断批次产出——
			// 记录缺席时作答为空，集原站序维持现状
			h.recordWorkSetOrders(sess.Snapshot, allItems, pipeline.batchMs)
		}
	} else if len(degradedChildren) > 0 {
		// 降级形态：单块全量内联（超限中止零产出零簿记）；纯内联整体发射即
		// 原子提交点（无批次块=无回滚面）
		var totalBytes int
		for _, child := range degradedChildren {
			totalBytes += childWireSizeUpper(child)
		}
		if totalBytes > degradeChildrenInlineLimit {
			return fmt.Sprintf("宿主契约过旧（协商版本 %d < %d）且子行载荷超限（约 %.0fMB ≥ %dMB 上限），无法以单块内联形态创建；请升级宿主后重试",
				h.hostContractVersion(), createChildrenBatchMinHostContract,
				float64(totalBytes)/(1<<20), degradeChildrenInlineLimit>>20), false
		}
		if runCtx.Err() != nil {
			return "", true
		}
		batchMs := time.Now().UnixMilli()
		parentID := fmt.Sprintf("local-batch-%d", batchMs)
		emitChildBatches(ch, parentID, taskName, "local://"+root, identity.Local.Key,
			[][]*sdkdto.TaskCreateChildResponse{degradedChildren},
			func(emitted, total int) { noteBatch(emitted, emitted == total) })
		markCreateCommitted(root)
		h.recordWorkSetOrders(sess.Snapshot, allItems, batchMs)
	}

	// 批次摘要：按实际产出子行的 siteKey 统计归属分布——local 子行键空（继承
	// 语义）、真实域子行键=归属站点，与产出形态严格一致
	summary := createBatchSummary{total: totalBuilt}
	for _, siteKey := range nonLocalSiteKeys {
		summary.addNonLocal(siteDisplayName(siteKey))
	}
	h.sessions.recordCreateBatchSummary(root, summary)
	return "", false
}

// hashWorkFilesAllOrNothing 对作品全部轨道计算内容哈希（verify 哈希循环的单件
// 入口，worker 池内并发调用）。三态结论见 unitHashOutcome：ok=true=全部轨道
// 哈希完成；ok=false 且 err=nil=哈希计算失败整件放弃（多轨作品的轨道集原子，
// 结构上不产出半作品；单轨集成员作品同口径，失败仅弃该成员自身——v1 口径，
// 失败原因聚合上报见放行循环，风险6）；err 非 nil=整次中止——轨道 Stat 失败=
// 所选文件已不存在（磁盘对账未通过——对账拆分·甲2 刀4：逐文件存在性核对自
// 守卫链并入本循环，守卫链只留目录级 mtime 游标抽查；Stat 失败即整次创建中断，
// 对账文案经 err 由调用方收流，与原守卫链拒绝文案同一句，并行/取消路径下语义
// 不回退），或取消位（甲3：作品间检查点——ctx 取消映射 errCreateCancelled）。
// throttle 为爬坡控制器（读盘字节进窗口统计；nil=不统计）。hcache 为乙2 哈希
// 缓存（nil=不缓存）：命中（relPath+size+mtime 全符，决策2）直接复用 hash、
// 跳过 ComputeFileHashCtx——零读盘字节进爬坡窗口与速率字段（风险5：noteReadBytes
// 是窗口唯一进数口，命中不产生读 IO 不得进数）；未命中算后记入回写集
//（record 先于结论投递——放行序保证中止前已放行单元的记录必已落账）。
func (h *LocalImportWorkFetcher) hashWorkFilesAllOrNothing(ctx context.Context, root string, work *SelectableWork, throttle *hashThrottle, hcache *hashCache) unitHashOutcome {
	var onBytes func(int64)
	if throttle != nil {
		onBytes = throttle.noteReadBytes
	}
	out := make([]FileEntry, 0, len(work.Files))
	for _, rel := range work.Files {
		// 甲3 取消检查点（作品间）：置位即整次收流（哨兵由放行循环转取消收尾）
		if ctx.Err() != nil {
			return unitHashOutcome{err: errCreateCancelled}
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		fi, err := os.Stat(full)
		if err != nil {
			return unitHashOutcome{err: fmt.Errorf("磁盘对账未通过：所选文件 %s 已不存在（可能被移动或删除），请重新识别受影响子树后再创建", rel)}
		}
		size, mtimeNs := fi.Size(), fi.ModTime().UnixNano()
		// 乙2 缓存命中：直接复用 hash、跳过 ComputeFileHashCtx——零读盘字节
		//（风险5 seam：命中不产生读 IO，onBytes 不因命中进数）
		if hcache != nil {
			if hash, hit := hcache.lookup(rel, size, mtimeNs); hit {
				out = append(out, FileEntry{FullPath: full, RelPath: rel, Hash: hash})
				continue
			}
		}
		// 甲3 取消检查点（4MB 分块处，读循环内）；真实读盘字节逐块进爬坡窗口
		hash, err := ComputeFileHashCtx(ctx, full, onBytes)
		if err != nil {
			if ctx.Err() != nil {
				return unitHashOutcome{err: errCreateCancelled}
			}
			return unitHashOutcome{dropErr: err}
		}
		if hcache != nil {
			hcache.record(rel, size, mtimeNs, hash) // 未命中算后记入回写集
		}
		out = append(out, FileEntry{FullPath: full, RelPath: rel, Hash: hash})
	}
	return unitHashOutcome{files: out, ok: true}
}

// Create WorkFetcher 接口原方法：委托 CreateWithHeartbeat 的同一分派路径，上报器
// 传空实现（等待期不上报心跳）。SDK 服务端对实现了心跳接入接口的拉取器调用
// CreateWithHeartbeat，本方法保留接口完整性与进程内直调路径。
func (h *LocalImportWorkFetcher) Create(url string) (*sdkdto.TaskCreateResult, error) {
	return h.CreateWithHeartbeat(url, sdkdto.NoopHeartbeat{})
}

// buildScanFiles 把扫描条目转为流水线输入：文件名含扩展名，目录层级取相对路径的
// 目录段（索引即深度），Path 保留完整路径。
func buildScanFiles(files []FileEntry) []ScanFile {
	out := make([]ScanFile, 0, len(files))
	for _, f := range files {
		out = append(out, ScanFile{
			Path:     f.FullPath,
			FileName: filepath.Base(f.FullPath),
			DirNames: ExtractDirLevels(f.RelPath),
			RelPath:  f.RelPath,
		})
	}
	return out
}

// resolvePipelineRules 解析流水线规则来源：插件上下文可用时经 KV 载入用户规则档案
// 并与内置规则合并（用户规则在前）；个别坏规则被跳过时记录日志、其余规则照常参与；
// 档案整体不可载入（读取失败/结构版本不符）降级为仅内置并记录日志；上下文不可用时
// 仅内置。
func (h *LocalImportWorkFetcher) resolvePipelineRules() []RuleEntry {
	if h.ctx == nil {
		return EffectiveRules(nil)
	}
	profile, errs := LoadProfile(NewContextProfileStore(h.ctx))
	for _, err := range errs {
		h.ctx.Warnf("规则档案载入异常: %v", err)
	}
	if profile == nil {
		return EffectiveRules(nil)
	}
	return EffectiveRules(profile)
}

// scanFileGroup 同目录扫描条目桶：dirRelPath 为以 '/' 连接的目录相对路径
// （扫描根直下为空串），files 保持扫描顺序。
type scanFileGroup struct {
	dirRelPath string
	files      []FileEntry
}

// groupEntriesByDir 按直接父目录聚合扫描条目：聚合键与流水线一致（目录名整序列），
// 桶序为目录首次出现序。
func groupEntriesByDir(files []FileEntry) []scanFileGroup {
	index := make(map[string]int, len(files))
	groups := make([]scanFileGroup, 0, len(files))
	for _, f := range files {
		dirNames := ExtractDirLevels(f.RelPath)
		key := dirGroupKey(dirNames)
		idx, ok := index[key]
		if !ok {
			idx = len(groups)
			index[key] = idx
			groups = append(groups, scanFileGroup{dirRelPath: strings.Join(dirNames, "/")})
		}
		groups[idx].files = append(groups[idx].files, f)
	}
	return groups
}

// buildChildResponses 批次应答的子行构造（逐文件形态）：作品身份真实域=解析
// 出的页级作品 ID、local 回退=文件内容哈希；子行归属键与 FilePluginData 归属
// 字段同构——真实域归属子行携 siteKey（作品身份落库域），local 回退子行留空
// （继承父应答站点键）；FilePluginData 携带该文件所属目录的含义链
// （item.metadata）与任务级赋值快照（assigns——工作台创建固化进任务载荷，
// nil=不固化）。不可访问文件（os.Stat 失败）逐个跳过。
func buildChildResponses(items []batchFileItem, assigns map[string]SessionAssignment) []*sdkdto.TaskCreateChildResponse {
	children := make([]*sdkdto.TaskCreateChildResponse, 0, len(items))
	for _, item := range items {
		af := item.af
		fi, err := os.Stat(af.entry.FullPath)
		if err != nil {
			continue
		}

		// 作品身份：真实域=解析出的页级作品 ID，local 回退=文件内容哈希
		workID := af.entry.Hash
		childSiteKey := ""
		fp := &FilePluginData{
			SchemaVersion: currentPluginDataVersion,
			FullPath:      af.entry.FullPath,
			RelPath:       af.entry.RelPath,
			Hash:          af.entry.Hash,
			Size:          fi.Size(),
			Metadata:      item.metadata,
		}
		if af.attr.isReal() {
			workID = af.attr.PageWorkID
			childSiteKey = af.attr.SiteKey
			fp.SiteKey = af.attr.SiteKey
			fp.SiteWorkId = af.attr.PageWorkID
		}
		// 任务级赋值快照：所选文件依赖的会话赋值固化进任务载荷（执行期重算优先
		// 消费，任务自包含——会话删除不影响在途任务）
		if assign, ok := assigns[af.entry.RelPath]; ok {
			snapshot := assign
			fp.Assignment = &snapshot
		}
		fpJSON, _ := json.Marshal(fp)

		children = append(children, &sdkdto.TaskCreateChildResponse{
			TaskName:     filepath.Base(af.entry.FullPath),
			SiteWorkId:   workID,
			Url:          "local://" + af.entry.FullPath,
			PluginData:   string(fpJSON),
			ResourceType: classifyResourceType(af.entry.FullPath),
			SiteKey:      childSiteKey,
		})
	}
	return children
}

// buildBatchParentResponse 批次应答的父行构造：纯容器——PluginData=
// DirPluginData{SchemaVersion, DirRelPath:"", Metadata:nil}（目录含义链不
// 呈现于父行，执行语义全在子行）；单子行亦为父行（「一次创建为一个父任务」，
// 对齐宿主「有 Children→parent+children 不折叠」契约）。parentID/taskName/
// parentURL/siteKey 由调用方给定（emitSelectedWorks 的命名与创建域裁决）。
func buildBatchParentResponse(parentID, taskName, parentURL, siteKey string, children []*sdkdto.TaskCreateChildResponse) *sdkdto.TaskCreateResponse {
	dp := &DirPluginData{SchemaVersion: currentPluginDataVersion}
	dpJSON, _ := json.Marshal(dp)

	return &sdkdto.TaskCreateResponse{
		PluginTaskId: parentID,
		TaskName:     taskName,
		SiteWorkId:   parentID,
		Url:          parentURL,
		PluginData:   string(dpJSON),
		SiteKey:      siteKey,
		ResourceType: "", // 有 children 时由各 child 声明(parent 不声明)
		Children:     children,
	}
}

// recomputeFileDecision 按 PluginData 携带的原始路径重算单文件的「档案∪赋值」
// 合并结论：FileName/DirNames/RelPath 由路径分解重建（与识别干跑 buildScanFiles
// 同构），Siblings 经父目录 os.ReadDir 重扫（与干跑扫描同源同键——同目录常规
// 文件名全集、字典序）；各层祖先直接子文件与子目录数经 rescanAncestorView
// 磁盘重扫注入（v4 目录终结谓词消费——子女数据缺失时保守不命中，识别批量
// 通路经 MergedRecognitionWithTree 注入全量树，此处为单文件同义磁盘版；
// 子目录计数为磁盘真值〔含空目录〕，与识别通路的清单聚合〔仅含有文件子目录〕
// 在空子目录场景可能分叉，记谱系已知差异）；库内谓词索引注入（lib 参数——与
// 识别干跑同一 BuildLibraryIndex 产物，风险12 闭环：全景/创建/重算三点同一
// 谓词求值基面）；任务级赋值快照（fp.Assignment）优先作为该文件的会话赋值参与
// 合并覆盖（mergedFileConclusionOf——与 MergedRecognition 同一实现，风险13），
// 缺失时回退纯档案求值。返回合并结论与匹配输入（匹配输入供归属裁决重放规则
// 取捕获值）。
func (h *LocalImportWorkFetcher) recomputeFileDecision(fp *FilePluginData, rules []RuleEntry, lib *LibraryIndex) (FileConclusion, *MatchInput) {
	dirNames := ExtractDirLevels(fp.RelPath)
	siblings := h.rescanSiblingNames(filepath.Dir(fp.FullPath))
	ancestorFiles, childDirCounts := h.rescanAncestorView(filepath.Dir(fp.FullPath), dirNames, siblings)
	in := &MatchInput{
		FileName:               filepath.Base(fp.FullPath),
		DirNames:               dirNames,
		RelPath:                fp.RelPath,
		Siblings:               siblings,
		AncestorFiles:          ancestorFiles,
		AncestorChildDirCounts: childDirCounts,
	}
	return mergedFileConclusionOf(in, rules, lib, fp.Assignment), in
}

// rescanAncestorView 各层祖先目录的直接子文件与直接子目录数（磁盘 ReadDir 重
// 扫）：parentDir 为文件父目录（最深层的真值直接复用 siblings，避免二次读盘）；
// 中间层逐级上溯读盘，某层不可读（路径已移动/删除）该层子文件空、计数 0——
// 子女类终结谓词按数据缺失保守不命中，不中断整体重算。与 ancestorFilesLevels/
// ancestorChildDirCounts（清单聚合版）同键同序。
func (h *LocalImportWorkFetcher) rescanAncestorView(parentDir string, dirNames []string, siblings []string) ([][]string, []int) {
	ancestorFiles := make([][]string, len(dirNames))
	childDirCounts := make([]int, len(dirNames))
	if len(dirNames) == 0 {
		return ancestorFiles, childDirCounts
	}
	ancestorFiles[len(dirNames)-1] = siblings
	dir := parentDir
	for d := len(dirNames) - 2; d >= 0; d-- {
		dir = filepath.Dir(dir) // 先上溯：d 层目录是 d+1 层目录的父
		entries, err := os.ReadDir(dir)
		if err != nil {
			if h.ctx != nil {
				h.ctx.Warnf("重扫祖先目录失败，该层子女类谓词保守不命中: %s: %v", dir, err)
			}
			continue
		}
		files := make([]string, 0, len(entries))
		subdirs := 0
		for _, e := range entries {
			if e.IsDir() {
				subdirs++
				continue
			}
			files = append(files, e.Name())
		}
		ancestorFiles[d] = files
		childDirCounts[d] = subdirs
	}
	return ancestorFiles, childDirCounts
}

// rescanSiblingNames 重扫父目录取同目录常规文件名全集（ReadDir 按名字典序返回，
// 与扫描器的目录遍历同序）；父目录不可读（路径已移动/删除）时记日志并返回空集，
// 兄弟群组规则按落单求值，不中断整体重算。
func (h *LocalImportWorkFetcher) rescanSiblingNames(parentDir string) []string {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if h.ctx != nil {
			h.ctx.Warnf("重扫父目录取兄弟文件失败，兄弟群组规则按落单求值: %s: %v", parentDir, err)
		}
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// logPendingSynthesis 逐条记录合成待裁决项（归因观测面）：维度、候选值与冲突
// 原因进日志，供人工裁决后调整规则或合成策略重跑；上下文不可用时静默跳过。
func (h *LocalImportWorkFetcher) logPendingSynthesis(fullPath string, pending []PendingItem) {
	if len(pending) == 0 || h.ctx == nil {
		return
	}
	for _, item := range pending {
		values := make([]string, 0, len(item.Candidates))
		for _, decl := range item.Candidates {
			values = append(values, decl.Value)
		}
		h.ctx.Warnf("文件 %s 的 %s 维度声明无法自动裁决，候选值 %v：%s", fullPath, item.Dimension, values, item.Reason)
	}
}

// CreateWorkInfo 从 PluginData 反序列化路径元数据，并按原始路径重算「档案∪赋值」
// 合并求值合成产出作品信息：规则解析与创建期共用同一入口（resolvePipelineRules），
// 库内谓词索引注入（libraryIndexCached——与识别干跑同一构造，风险12 闭环），
// 任务级赋值快照（PluginData.Assignment）优先参与合并覆盖（recomputeFileDecision
// 内经 mergedFileConclusionOf 与干跑同源，风险13），提取声明合成终选与待裁决——
// 待裁决项逐条记日志（维度/候选值/原因，归因观测面）。终选映射：标题→作品名
// （无标题声明回退整个文件名去扩展名，扩展名由 Resource.Format 单独提供）、
// 作者→本地作者声明（带声明级角色）、标签→本地标签声明、作品集→作品集声明
// （集名=声明值，集内序不传契约——宿主续排近似，本期既定裁决）；路径含义
// （PathMeaning）映射维持现状。归属裁决按内置 pixiv 规则重算：命中即真实域键
// （捕获值=页级作品 ID），未命中走非 pixiv 路径（PluginData 携带的创建期归属，
// 无归属=文件内容哈希）。
func (h *LocalImportWorkFetcher) CreateWorkInfo(task *sdkdto.TaskDTO) (*sdkdto.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}
	if err := checkPluginDataVersion(fp.SchemaVersion); err != nil {
		return nil, err
	}

	// 单任务合并求值重算（注入库内索引与任务级赋值快照，与识别干跑同源）
	rules := h.resolvePipelineRules()
	conclusion, _ := h.recomputeFileDecision(&fp, rules, h.libraryIndexCached())
	h.logPendingSynthesis(fp.FullPath, conclusion.Pending)
	if h.ctx != nil {
		h.ctx.Infof("CreateWorkInfo 观测: file=%s matched=%v final=%d", fp.RelPath, conclusion.MatchedRuleIDs, len(conclusion.Final))
	}
	final := conclusion.Final

	resp := &sdkdto.WorkResponse{Work: &sdkdto.WorkDTO{}}
	workName := stripExt(filepath.Base(fp.FullPath))
	titleDeclared := false
	for _, decl := range final {
		switch decl.Dimension {
		case DimTitle:
			// 标题取首个终选声明（highTrust 择一常为单值，用户策略并集多值时保首个）
			if !titleDeclared {
				workName = decl.Value
				titleDeclared = true
			}
		case DimAuthor:
			// 本地作者按名声明（ID=0 名称模式，宿主 find-or-create），携带声明级角色
			author := &sdkdto.LocalAuthorDTO{AuthorName: new(decl.Value)}
			if decl.RoleName != "" {
				author.RoleName = new(decl.RoleName)
			}
			resp.LocalAuthors = append(resp.LocalAuthors, author)
		case DimTag:
			resp.LocalTags = append(resp.LocalTags, &sdkdto.LocalTagDTO{LocalTagName: new(decl.Value)})
		case DimCollection:
			// 集维度声明 → 作品集按名声明：SiteWorkSetId 传 local 站点的业务键
			// "workSet:{名}"——宿主按 (site_id, site_work_set_id) 业务键 upsert
			//（library-squirrel/backend/work/service.go upsertWorkSets：同名键
			// 复用既有行、缺席才新建），同名集幂等归并。库内既有集「行 id」不
			// 得作此键传入：宿主无任何按行 id 寻址的消费路径，行 id 字符串会
			// 命中不到既有行而另造业务键="{行id}" 的新行（批次三阶段5 宿主语义
			// 核验结论）；既有集行 id 只经会话旁路簿记随快照 confirmedCandidates
			// 呈现，不进创建链。
			resp.WorkSets = append(resp.WorkSets, &sdkdto.TaskWorkSetDTO{
				SiteWorkSetId: workSetBizKey(decl.Value),
				WorkSetName:   decl.Value,
			})
		}
	}

	// 作品身份：创建期归属（真实域页级作品 ID）或 local 回退的文件内容哈希
	//（v4 起内置 pixiv 页级规则退休为例子模板、不参与求值——页级域键形态
	// 经用户档案规则覆盖，未配置时走哈希回退）。
	siteWorkID := fp.Hash
	if fp.hasRealAttribution() {
		siteWorkID = fp.SiteWorkId
	}
	resp.Work.SiteWorkId = new(siteWorkID)
	resp.Work.SiteWorkName = new(workName)

	for _, m := range fp.Metadata {
		switch m.Type {
		case "localAuthor":
			id, _ := strconv.ParseInt(m.ID, 10, 64)
			if id > 0 {
				resp.LocalAuthors = append(resp.LocalAuthors, &sdkdto.LocalAuthorDTO{Id: id})
			}
		case "siteAuthor":
			// 用户供数只有名字：按名落本地域（ID=0 名称模式，宿主 find-or-create）。
			// 站点域行的身份只能来自站点侧 ID，名字不造站点域行
			if m.Name != "" {
				name := m.Name
				resp.LocalAuthors = append(resp.LocalAuthors, &sdkdto.LocalAuthorDTO{AuthorName: &name})
			}
		case "localTag":
			id, _ := strconv.ParseInt(m.ID, 10, 64)
			if id > 0 {
				resp.LocalTags = append(resp.LocalTags, &sdkdto.LocalTagDTO{Id: id})
			}
		case "siteTag":
			// 同 siteAuthor：按名落本地域
			if m.Name != "" {
				name := m.Name
				resp.LocalTags = append(resp.LocalTags, &sdkdto.LocalTagDTO{LocalTagName: &name})
			}
		case "workSet":
			// 作品集按名 upsert 落作品所属站点域：local 归属作品落 local 域
			// （workSet:{名} 即 local 站点的作品集 ID 约定形态；业务键语义同上
			// DimCollection 注释——宿主按业务键 upsert，集行 id 不作键）。真实域
			// 归属作品的站点侧作品集 ID 无法从名字派生（键即身份，无 ID 不造
			// 行），跳过该含义
			if fp.hasRealAttribution() {
				if h.ctx != nil {
					h.ctx.Infof("作品集含义按名仅支持 local 归属，真实域归属(%s)跳过: %s", fp.SiteKey, m.Name)
				}
				continue
			}
			resp.WorkSets = append(resp.WorkSets, &sdkdto.TaskWorkSetDTO{
				SiteWorkSetId: workSetBizKey(m.Name),
				WorkSetName:   m.Name,
			})
		}
	}

	return resp, nil
}

// Start 打开文件,按 storeRoles 选择性返回 StoreSpec 流集合(主资源 downloaded + 缩略图 derived)与作品信息
func (h *LocalImportWorkFetcher) Start(ctx context.Context, task *sdkdto.TaskDTO, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}
	if err := checkPluginDataVersion(fp.SchemaVersion); err != nil {
		return nil, nil, err
	}

	ext := filepath.Ext(fp.FullPath)
	workName := stripExt(filepath.Base(fp.FullPath))
	taskID := fmt.Sprintf("%d", task.Id)

	// 主资源 role 按文件类型派生:video→videoMain(可播放主体,downloaded);image/document 对应;unknown→image 兜底
	mainRole := mainStoreRole(classifyResourceType(fp.FullPath))

	var specs []*sdkdto.StoreSpec

	// 主资源(downloaded):本地文件流,可断点续传
	if wantsRole(storeRoles, mainRole) {
		f, err := os.Open(fp.FullPath)
		if err != nil {
			return nil, nil, fmt.Errorf("打开文件失败: %w", err)
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("获取文件信息失败: %w", err)
		}
		h.readers.Store(taskID, f)
		specs = append(specs, &sdkdto.StoreSpec{
			Role:        mainRole,
			Generation:  sdkdto.GenerationDownloaded,
			ReadCloser:  f,
			Format:      ext,
			Size:        fi.Size(),
			Continuable: new(true),
			// 期望哈希声明(决策1 选项A):声明扫描期哈希,源文件扫描后被改动将触发
			// 主程序侧「资源完整性校验失败」而非静默导入新内容
			ExpectedSha256: expectedSha256Of(fp.Hash),
		})
	}

	// 缩略图(derived):按扩展名生成;无生成器或不适用则不产出该轨
	if wantsRole(storeRoles, sdkdto.StoreRoleThumbnail) {
		if thumbData, thumbFormat, _ := generateThumbnail(fp.FullPath); len(thumbData) > 0 {
			if thumbFormat == "" {
				thumbFormat = "jpg"
			}
			specs = append(specs, &sdkdto.StoreSpec{
				Role:        sdkdto.StoreRoleThumbnail,
				Generation:  sdkdto.GenerationDerived,
				ReadCloser:  io.NopCloser(bytes.NewReader(thumbData)),
				Format:      thumbFormat,
				Size:        int64(len(thumbData)),
				Continuable: new(false),
				// 生成产物轨对内存字节直接算哈希声明,与下载轨声明形态统一
				ExpectedSha256: new(sha256Hex(thumbData)),
			})
		}
	}

	if len(specs) == 0 {
		return nil, nil, fmt.Errorf("所选资源角色 %v 均无可产出 store", storeRoles)
	}

	resp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkName: &workName,
		},
	}

	return specs, resp, nil
}

// wantsRole storeRoles 为空(全量)或包含 role 时返回 true
func wantsRole(storeRoles []string, role string) bool {
	if len(storeRoles) == 0 {
		return true
	}
	return slices.Contains(storeRoles, role)
}

// Retry 委托到 Start
func (h *LocalImportWorkFetcher) Retry(task *sdkdto.TaskDTO) (*sdkdto.WorkResponse, error) {
	return nil, fmt.Errorf("retry 不支持，请使用 start")
}

// Pause 关闭文件句柄
func (h *LocalImportWorkFetcher) Pause(param *sdkdto.TaskResParam) error {
	return h.closeReader(param)
}

// Stop 关闭文件句柄
func (h *LocalImportWorkFetcher) Stop(param *sdkdto.TaskResParam) error {
	return h.closeReader(param)
}

// Resume 恢复下载:按 TaskResumeParam.StreamOffsets 续传未完成的主资源轨
// 缩略图(derived)为一次性产物,暂停的任务意味着其已完成,故 Resume 不再产出
func (h *LocalImportWorkFetcher) Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	if param.Task == nil || param.Task.PluginData == nil {
		return nil, nil, fmt.Errorf("task 或 pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*param.Task.PluginData), &fp); err != nil {
		return nil, nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}
	if err := checkPluginDataVersion(fp.SchemaVersion); err != nil {
		return nil, nil, err
	}

	// 主资源 role 按文件类型派生(与 Start 一致)
	mainRole := mainStoreRole(classifyResourceType(fp.FullPath))

	offset, hasMain := param.OffsetForRole(mainRole)
	// 主资源已完成或未选:无需续传
	if !hasMain {
		return nil, nil, nil
	}

	f, err := os.Open(fp.FullPath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开文件失败: %w", err)
	}

	// 从已下载位置继续
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("seek 到偏移量 %d 失败: %w", offset, err)
		}
	}

	taskID := fmt.Sprintf("%d", param.Task.Id)
	h.readers.Store(taskID, f)

	ext := filepath.Ext(fp.FullPath)
	workName := stripExt(filepath.Base(fp.FullPath))

	spec := &sdkdto.StoreSpec{
		Role:        mainRole,
		Generation:  sdkdto.GenerationDownloaded,
		ReadCloser:  f,
		Format:      ext,
		Size:        fp.Size,
		Continuable: new(true),
		// 与 Start 主资源轨同声明(扫描期哈希);跨会话实测哈希一致由主程序暂存前缀回读保证
		ExpectedSha256: expectedSha256Of(fp.Hash),
	}

	resp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkName: &workName,
		},
	}

	return []*sdkdto.StoreSpec{spec}, resp, nil
}

func (h *LocalImportWorkFetcher) closeReader(param *sdkdto.TaskResParam) error {
	if param.Task == nil {
		return nil
	}
	taskID := fmt.Sprintf("%d", param.Task.Id)
	if v, ok := h.readers.LoadAndDelete(taskID); ok {
		return v.(*os.File).Close()
	}
	return nil
}

// boolPtr 返回 bool 值的指针
//
//go:fix inline
func boolPtr(b bool) *bool { return new(b) }

// strPtr 返回字符串值的指针
//
//go:fix inline
func strPtr(s string) *string { return new(s) }

// expectedSha256Of 期望 SHA-256 声明(决策1 选项A:声明扫描时点哈希)。
// 防御:扫描期哈希为空(异常数据)不声明(nil)——不构造空串声明
// (空串在主程序侧语义=跳过比对,显式 nil 更诚实)
func expectedSha256Of(hash string) *string {
	if hash == "" {
		return nil
	}
	return new(hash)
}

// sha256Hex 计算内存字节的 SHA-256(十六进制):
// 插件生成产物轨(如缩略图)对内存字节直接声明期望哈希
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// stripExt 去除文件名扩展名(扩展名由 StoreSpec.Format 单独提供)
func stripExt(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		return strings.TrimSuffix(name, ext)
	}
	return name
}

// —— Create 流内入口确认协调器 ——

// 入口确认等待上限与心跳节拍。等待上限 5 分钟与分类问答降级时长一致：弹窗静置
// 超时即视为取消（意图信号·已取消）；心跳节拍 1 秒（SDK 上报器内置 20 秒限频，
// 等待线上实际每 20 秒一块）维持宿主 Create 流 60 秒空闲窗。
const (
	entryConfirmWaitLimit     = 5 * time.Minute
	entryConfirmHeartbeatTick = time.Second
)

// 入口确认事件种类（workbenchTopicEvent 通道的 kind 词汇）。
const (
	workbenchEventEntryConfirm        = "entryConfirm"        // 拉起确认弹窗（携带库根与确认配对序号）
	workbenchEventEntryConfirmDismiss = "entryConfirmDismiss" // 关窗指令（等待超时，组件收令关窗不补发答复）
)

// workbenchFrontendEvent 插件→前端的事件载荷（无配对序号的主动通知；入口确认
// 携带自身的确认配对序号供答复配对，识别/创建进度与终态携带阶段与计数）。
type workbenchFrontendEvent struct {
	Kind string `json:"kind"`
	// Seq 入口确认的配对序号（entryConfirm/entryConfirmDismiss 携带）。
	Seq int64 `json:"seq,omitempty"`
	// Root 库根（entryConfirm 与识别/创建进度、终态携带）。
	Root string `json:"root,omitempty"`
	// Phase / Done / Total 进度事件的阶段与计数（recognitionProgress/
	// createProgress 携带；Done/Total 的口径随阶段：识别 scan=待求值/全部文件、
	// evaluate=已求值/待求值文件；创建 verify=已核对/全部作品（逐作品哈希）、
	// create=已发射/计划子行总数（发射循环计数）——乙3 双计数模型：两相事件
	// 按 phase 分道、可并存重叠（核对行核对完成即止、发射行自首批发射起出现，
	// 前端阶段6 按相位消费）。
	Phase string `json:"phase,omitempty"`
	Done  int    `json:"done,omitempty"`
	Total int    `json:"total,omitempty"`
	// BytesPerSec 核对相当前窗口读盘字节/秒（甲1 速率字段：取自爬坡控制器同一
	// 窗口统计，只计真实读盘字节——风险5 口径；发射相与首事件不带）。
	BytesPerSec int64 `json:"bytesPerSec,omitempty"`
	// Success / Message 终态事件的成败与用户可读摘要（recognitionDone/
	// createDone 携带；createDone 失败态携带部分完成信息「已创建 X/N 个子任务」
	// 于文案，X/N 同时镜像到 Done/Total）。
	Success bool   `json:"success,omitempty"`
	Message string `json:"message,omitempty"`
}

// entryConfirmOutcome 入口确认终态：采纳（accepted 真）或未采纳（携带原因）。
type entryConfirmOutcome struct {
	accepted bool
	// reason 未采纳原因：user-cancel / timeout / publish-fail 之一。
	reason string
}

// cancelMessage 未采纳时意图信号（已取消）的用户可读文案。
func (o entryConfirmOutcome) cancelMessage() string {
	switch o.reason {
	case "timeout":
		return "确认超时已自动取消：本次未创建任何任务，需要时可重新发起导入"
	case "publish-fail":
		return "无法弹出确认窗口，本次导入已取消：请确认插件页面可用后重试"
	default:
		return "已取消：本次未开始解析，未创建任何任务"
	}
}

// entryConfirmDeps 入口确认编排所需能力（消费侧窄接口，便于单测注入桩）。
type entryConfirmDeps interface {
	PublishToFrontend(topic string, data []byte) error
}

// entryConfirmCoordinator 入口确认弹窗编排：Create 流内弹确认的拉起、答复配对
// 与超时关窗。同库根并发创建合流共等一次确认（按确认序号配对，迟到答复丢弃）。
type entryConfirmCoordinator struct {
	deps      entryConfirmDeps
	logger    sdkdto.Logger
	waitLimit time.Duration // 等待上限（生产 5 分钟，单测注入短值）
	tick      time.Duration // 心跳上报节拍（生产 1 秒，单测注入短值）

	mu      sync.Mutex
	nextSeq int64
	// waiters 确认序号 → 答复通道（缓冲 1，每通道恰投递一次）。
	waiters map[int64]chan bool
}

// newEntryConfirmCoordinator 创建协调器（等待上限可注入）。
func newEntryConfirmCoordinator(deps entryConfirmDeps, logger sdkdto.Logger, waitLimit time.Duration) *entryConfirmCoordinator {
	return newEntryConfirmCoordinatorWithTick(deps, logger, waitLimit, entryConfirmHeartbeatTick)
}

// newEntryConfirmCoordinatorWithTick 创建等待上限与心跳节拍均可注入的协调器（单测用）。
func newEntryConfirmCoordinatorWithTick(deps entryConfirmDeps, logger sdkdto.Logger, waitLimit, tick time.Duration) *entryConfirmCoordinator {
	return &entryConfirmCoordinator{
		deps:      deps,
		logger:    logger,
		waitLimit: waitLimit,
		tick:      tick,
		waiters:   make(map[int64]chan bool),
	}
}

// publishEvent 向前端发布工作台事件。
func (c *entryConfirmCoordinator) publishEvent(evt *workbenchFrontendEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return c.deps.PublishToFrontend(workbenchTopicEvent, data)
}

// Await 拉起确认弹窗并阻塞等待答复：等待期按节拍经 reporter 上报心跳维持宿主
// 空闲窗；用户答复（经前端 confirmAnswer 请求 → HandleAnswer）投递通道解除等待；
// 超时向前端发关窗指令并按取消收尾。发送拉起事件失败直接按未采纳收尾（无窗口可答）。
func (c *entryConfirmCoordinator) Await(root string, reporter sdkdto.HeartbeatReporter) entryConfirmOutcome {
	ch, seq := c.register()
	evt := &workbenchFrontendEvent{Kind: workbenchEventEntryConfirm, Seq: seq, Root: root}
	if err := c.publishEvent(evt); err != nil {
		c.remove(seq)
		if c.logger != nil {
			c.logger.Warnf("发送入口确认拉起事件失败: %v", err)
		}
		return entryConfirmOutcome{reason: "publish-fail"}
	}

	ticker := time.NewTicker(c.tick)
	defer ticker.Stop()
	timer := time.NewTimer(c.waitLimit)
	defer timer.Stop()
	for {
		select {
		case accepted := <-ch:
			return entryConfirmOutcome{accepted: accepted}
		case <-ticker.C:
			reporter.Heartbeat()
		case <-timer.C:
			// 超时：登记在表者负责关窗指令；已被答复解除时补收结果
			if c.remove(seq) {
				evt := &workbenchFrontendEvent{Kind: workbenchEventEntryConfirmDismiss, Seq: seq}
				if err := c.publishEvent(evt); err != nil && c.logger != nil {
					c.logger.Errorf("发送入口确认关窗指令失败: %v", err)
				}
			}
			select {
			case accepted := <-ch:
				return entryConfirmOutcome{accepted: accepted}
			default:
				return entryConfirmOutcome{reason: "timeout"}
			}
		}
	}
}

// register 登记等待者并分配确认序号。
func (c *entryConfirmCoordinator) register() (<-chan bool, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextSeq++
	ch := make(chan bool, 1)
	c.waiters[c.nextSeq] = ch
	return ch, c.nextSeq
}

// remove 移除等待者；返回移除前是否在表（超时方据此判定关窗指令归属——
// 已被答复解除的会话不重发关窗）。
func (c *entryConfirmCoordinator) remove(seq int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.waiters[seq]
	delete(c.waiters, seq)
	return ok
}

// HandleAnswer 处理前端确认答复（采纳/取消）：命中在等确认即投递并返回真；
// 无在等确认（超时后迟到/重复答复）返回假，由请求处理层回告失效。
func (c *entryConfirmCoordinator) HandleAnswer(confirmSeq int64, accepted bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.waiters[confirmSeq]
	if !ok {
		return false
	}
	delete(c.waiters, confirmSeq)
	ch <- accepted
	return true
}
