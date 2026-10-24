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

	// 执行期重算的库内谓词索引缓存（CreateWorkInfo 按任务逐个调用，逐次全量
	// 分页拉取四轨会放大库查询；TTL 窗口内复用，见 libraryIndexCached）
	libMu      sync.Mutex
	libIndex   *LibraryIndex
	libBuiltAt time.Time
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

	// 分支 1：工作台发起的创建（会话+选择集在）→ 过滤产出
	if h.sessions.ConsumeWorkbenchCreateMarker(root) {
		return h.createWorkbenchFiltered(url, root)
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
// 流式逐组产出（内容哈希只对所选作品计算，未选资源零哈希零任务）。
func (h *LocalImportWorkFetcher) createWorkbenchFiltered(url, root string) (*sdkdto.TaskCreateResult, error) {
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
	domain := DeriveWorkDomain(sess.Snapshot, ignoredSetOf(sess))
	works, missing := domain.ResolveAll(sess.Selection)
	if len(missing) > 0 {
		result := sdkdto.BatchResult(nil)
		result.SetReason(fmt.Sprintf("选择集中 %d 项已不在当前识别全景内（磁盘或识别结构可能已变化），本次未产出任务；请重新识别并重选", len(missing)))
		return result, nil
	}

	// 任务名：编排动作暂存的原文经本分支消费（宿主 CreateTask RPC 同步递归覆盖）；
	// 缺席（测试直驱等未暂存路径）或空白按默认名兜底——单一裁决点见 resolveCreateTaskName
	taskName := resolveCreateTaskName(h.sessions.takeStagedCreateTaskName(root), root, len(works))

	ch := make(chan *sdkdto.TaskCreateResponse, 16)
	go func() {
		defer close(ch)
		h.emitSelectedWorks(root, sess, works, taskName, ch)
	}()
	return sdkdto.StreamResult(ch), nil
}

// batchFileItem 批次应答的子行素材：带归属裁决的文件与其所属目录的含义链。
// 批次父行纯容器（目录含义链不呈现于父行），含义链随子行各自的目录走。
type batchFileItem struct {
	af       attributedFile
	metadata []PathMeaning
}

// emitSelectedWorks 按选择集产出批次应答（一次创建=一个父任务）：文件按选择域
// 确定序（目录按域序，目录内先目录聚合作品后集成员〔域序=集内序〕）逐个裁决
// 归属后归集进同一条父应答——站点键=identity.Local.Key（创建域），子行归属经
// 各自 siteKey 字段声明：真实域归属子行携归属站点键（作品身份落库域，仍受宿主
// 站点表存在性校验），local 回退子行留空（继承父应答站点键）。
// 父行形态：
//   - PluginTaskId=local-batch-<unix毫秒>（宿主按应答建树不依赖跨批唯一——
//     同批二次创建产生新批次行，R3）；
//   - TaskName=用户名；
//   - children=全部文件子行（逐文件形态、FilePluginData、赋值快照与确定序
//     保持 v1 形态——子行构造复用 buildChildResponses）；
//   - 父 PluginData=DirPluginData 纯容器（DirRelPath 空、无含义链——执行语义
//     全在子行）。
//
// 任一作品全部轨道不可访问（stat/哈希失败）即整件跳过（半作品结构性不产出，
// v1 口径）。产出完成后向会话管理器记录批次摘要（子行总数与非 local 子行的
// 归属分布，编排动作通知文案的数据源——宿主 CreateTask RPC 同步消费完整产出
// 流后才返回，记录必先于编排读取）。
func (h *LocalImportWorkFetcher) emitSelectedWorks(root string, sess *WorkbenchSession, works []*SelectableWork, taskName string, ch chan<- *sdkdto.TaskCreateResponse) {
	chains := dirMeaningChains(sess.Snapshot)

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

	// 子行素材归集：逐文件裁决归属后按原序扁平进批次（目录序内先目录聚合作品
	// 后集成员——同目录文件不按归属重排）
	items := make([]batchFileItem, 0)
	collect := func(files []FileEntry, metadata []PathMeaning, classifiedSiteKey string) {
		for _, f := range files {
			attr := resolveWorkAttribution(filepath.Base(f.FullPath), classifiedSiteKey)
			items = append(items, batchFileItem{af: attributedFile{entry: f, attr: attr}, metadata: metadata})
		}
	}
	for _, dirKey := range dirOrder {
		plan := plans[dirKey]
		metadata := chains[dirKey]
		// 站点分类含义 → 子树归属站点键（快照回放含义不含 site 型——site 仅经
		// 目录问答产生，会话模式问答不可达，此处恒走文件名形态归属裁决）
		classifiedSiteKey := resolveClassifiedSiteKey(metadata, h.resolveSiteKeyByID)

		if plan.dirWork != nil {
			// 目录聚合作品：全轨道 all-or-nothing 哈希
			if files, ok := h.hashWorkFilesAllOrNothing(root, plan.dirWork); ok {
				collect(files, metadata, classifiedSiteKey)
			}
		}
		// 集成员：单轨同口径（任一轨道失败仅弃该成员自身），成员按域序=集内序进批次
		for _, member := range plan.setMembers {
			if files, ok := h.hashWorkFilesAllOrNothing(root, member); ok {
				collect(files, metadata, classifiedSiteKey)
			}
		}
	}

	// 批次产出：单父应答承载全部子行（空批次不产出父行，v1 口径）
	children := buildChildResponses(items, sess.Assignments)
	if len(children) > 0 {
		batchMs := time.Now().UnixMilli()
		parentID := fmt.Sprintf("local-batch-%d", batchMs)
		ch <- buildBatchParentResponse(parentID, taskName, "local://"+root, identity.Local.Key, children)

		// 集序记录：为有集归属的子行按集归集落记录（集内原站序作答的数据源，
		// 批次号=父行身份同源毫秒时间戳）；写入失败不阻断批次产出——记录缺席
		// 时作答为空，集原站序维持现状
		h.recordWorkSetOrders(sess.Snapshot, items, batchMs)
	}

	// 批次摘要：按实际产出子行的 siteKey 统计归属分布——local 子行键空（继承
	// 语义）、真实域子行键=归属站点，与产出形态严格一致
	summary := createBatchSummary{total: len(children)}
	for _, child := range children {
		if child.SiteKey != "" {
			summary.addNonLocal(siteDisplayName(child.SiteKey))
		}
	}
	h.sessions.recordCreateBatchSummary(root, summary)
}

// hashWorkFilesAllOrNothing 对作品全部轨道计算内容哈希：任一轨道 stat/哈希失败
// 即整件放弃（返回 false）——多轨作品的轨道集原子，结构上不产出半作品；单轨
// 集成员作品同口径（失败仅弃该成员自身）。
func (h *LocalImportWorkFetcher) hashWorkFilesAllOrNothing(root string, work *SelectableWork) ([]FileEntry, bool) {
	out := make([]FileEntry, 0, len(work.Files))
	for _, rel := range work.Files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(full); err != nil {
			if h.ctx != nil {
				h.ctx.Warnf("所选文件不可访问，跳过其所属作品 %s: %v", work.WorkID, err)
			}
			return nil, false
		}
		hash, err := ComputeFileHash(full)
		if err != nil {
			if h.ctx != nil {
				h.ctx.Warnf("所选文件哈希失败，跳过其所属作品 %s: %v", work.WorkID, err)
			}
			return nil, false
		}
		out = append(out, FileEntry{FullPath: full, RelPath: rel, Hash: hash})
	}
	return out, true
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
// 文件名全集、字典序）；库内谓词索引注入（lib 参数——与识别干跑同一
// BuildLibraryIndex 产物，风险12 闭环：全景/创建/重算三点同一谓词求值基面）；
// 任务级赋值快照（fp.Assignment）优先作为该文件的会话赋值参与合并覆盖
// （mergedFileConclusionOf——与 MergedRecognition 同一实现，风险13），缺失时
// 回退纯档案求值。返回合并结论与匹配输入（匹配输入供归属裁决重放规则取捕获值）。
func (h *LocalImportWorkFetcher) recomputeFileDecision(fp *FilePluginData, rules []RuleEntry, lib *LibraryIndex) (FileConclusion, *MatchInput) {
	dirNames := ExtractDirLevels(fp.RelPath)
	in := &MatchInput{
		FileName: filepath.Base(fp.FullPath),
		DirNames: dirNames,
		RelPath:  fp.RelPath,
		Siblings: h.rescanSiblingNames(filepath.Dir(fp.FullPath)),
	}
	return mergedFileConclusionOf(in, rules, lib, fp.Assignment), in
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
// 携带自身的确认配对序号供答复配对，识别进度/终态携带阶段与计数）。
type workbenchFrontendEvent struct {
	Kind string `json:"kind"`
	// Seq 入口确认的配对序号（entryConfirm/entryConfirmDismiss 携带）。
	Seq int64 `json:"seq,omitempty"`
	// Root 库根（entryConfirm 与识别进度/终态携带）。
	Root string `json:"root,omitempty"`
	// Phase / Done / Total 识别进度的阶段（scan/index/evaluate/save）与计数
	//（recognitionProgress 携带；Done/Total 的口径随阶段：scan=待求值/全部文件，
	// evaluate=已求值/待求值文件）。
	Phase string `json:"phase,omitempty"`
	Done  int    `json:"done,omitempty"`
	Total int    `json:"total,omitempty"`
	// Success / Message 识别终态的成败与用户可读摘要（recognitionDone 携带）。
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
