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
	"strconv"
	"strings"
	"sync"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// currentPluginDataVersion 当前插件支持的 task PluginData 格式版本
// v2：FilePluginData 增加可选归属字段（siteKey/siteWorkId，扫描期归属裁决结果）；
// v0/v1 旧数据无归属字段，读侧按零值走 local 回退路径（作品身份=内容哈希）
const currentPluginDataVersion = 2

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
	SchemaVersion int           `json:"schemaVersion,omitempty"` // plugin_data 格式版本；0=旧数据(引入版本约定前写入)，1=引入版本约定，2=增加归属字段
	FullPath      string        `json:"fullPath"`
	RelPath       string        `json:"relPath"`
	Hash          string        `json:"hash"`
	Size          int64         `json:"size"`
	Metadata      []PathMeaning `json:"metadata,omitempty"`
	// SiteKey/SiteWorkId 真实域归属（扫描期站点分类与文件名形态解析的裁决结果）：
	// SiteKey=归属站点键、SiteWorkId=页级作品 ID。两字段空=local 回退，作品身份=Hash
	SiteKey    string `json:"siteKey,omitempty"`
	SiteWorkId string `json:"siteWorkId,omitempty"`
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
//  1. 工作台发起（会话带发起标记）→ 过滤产出分支：只产出所选作品防递归确认。
//     本阶段为占位骨架——选择集与会话赋值注入属后续阶段，占位路径零任务收尾；
//  2. 库根命中既有会话 → 不重问，意图信号「转入解析」收尾（零任务）；
//  3. 无会话 → Create 流内弹确认（二选：开始解析〔默认〕/取消），等待期心跳
//     保活 + 插件侧 5 分钟超时（超时=取消）；采纳即建会话并以「转入解析」收尾，
//     取消/超时以「已取消」收尾——两态均零任务零落库。
//
// 全量扫描直出的旧创建路径不再从入口触达（宿主新建任务输路径后必经上述分派），
// 扫描产出实现保留为 streamScanTasks，供工作台过滤产出接线复用。
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

// createWorkbenchFiltered 工作台发起创建的过滤产出分支（占位骨架）：正式实现为
// 注入会话赋值求值 + 按资源选择集只产出所选作品（集父行按所选成员生成），属后续
// 阶段接线；本阶段占位零任务收尾，防同一创建递归进入确认分支。
func (h *LocalImportWorkFetcher) createWorkbenchFiltered(url, root string) (*sdkdto.TaskCreateResult, error) {
	result := sdkdto.BatchResult(nil)
	result.SetReason("工作台发起创建的过滤产出尚未实现（选择集过滤属后续阶段），本次未产出任务")
	return result, nil
}

// Create WorkFetcher 接口原方法：委托 CreateWithHeartbeat 的同一分派路径，上报器
// 传空实现（等待期不上报心跳）。SDK 服务端对实现了心跳接入接口的拉取器调用
// CreateWithHeartbeat，本方法保留接口完整性与进程内直调路径。
func (h *LocalImportWorkFetcher) Create(url string) (*sdkdto.TaskCreateResult, error) {
	return h.CreateWithHeartbeat(url, sdkdto.NoopHeartbeat{})
}

// streamScanTasks 扫描本地路径并流式直出全部可导入任务（无选择集过滤）：
// 扫描条目先经解析内核流水线求值（规则来源=用户档案+内置），兄弟群组规则定群的
// 成员各自声明为单文件作品（叶任务），落单条目按同目录聚合走目录组声明路径
// （多文件=父容器+子任务，单文件=叶任务）。
// 工作台发起创建的过滤产出以本实现为基底（按选择集过滤的接线属后续阶段）。
func (h *LocalImportWorkFetcher) streamScanTasks(url string) (*sdkdto.TaskCreateResult, error) {
	path := localImportPathOf(url)

	scanner := NewScanner(path)
	scanResult, err := scanner.Scan()
	if err != nil {
		return nil, fmt.Errorf("扫描路径失败: %w", err)
	}

	if len(scanResult.Files) == 0 {
		// 扫描器不按扩展名过滤（不识别的类型归 unknown 照常导入），
		// 零文件只可能是目录下无常规文件、或文件均因不可访问被跳过，SetReason 按此语义向用户说明
		result := sdkdto.BatchResult(nil)
		result.SetReason("未发现可导入的文件（目录为空或文件均不可读取）")
		return result, nil
	}

	// 流水线求值：FileDecision 不落 PluginData——后续阶段按原始路径
	// （PluginData 携带 FullPath/RelPath）与同一规则档案重算，无需缓存决策。
	// 库内索引本阶段不注入（nil 时谓词规则恒不命中），索引装配属后续阶段。
	decisions := RunPipeline(buildScanFiles(scanResult.Files), h.resolvePipelineRules(), nil)

	ch := make(chan *sdkdto.TaskCreateResponse, 16)

	go func() {
		defer close(ch)

		pathInfo, _ := os.Stat(path)
		isDir := pathInfo != nil && pathInfo.IsDir()
		var rootMeanings []PathMeaning
		rootDirName := filepath.Base(path)
		if isDir {
			var err error
			rootMeanings, err = h.classifier.ClassifyDir(0, rootDirName)
			if err != nil {
				return
			}
		}

		decisionByRel := make(map[string]FileDecision, len(decisions))
		for _, d := range decisions {
			decisionByRel[d.RelPath] = d
		}

		for _, group := range groupEntriesByDir(scanResult.Files) {
			var metadata []PathMeaning

			if isDir {
				metadata = append(metadata, rootMeanings...)
			}

			if len(group.files) > 0 {
				levelsSlice := ExtractDirLevels(group.files[0].RelPath)
				levelOffset := 0
				if isDir {
					levelOffset = 1
				}
				for i, dirName := range levelsSlice {
					meanings, err := h.classifier.ClassifyDir(i+levelOffset, dirName)
					if err != nil {
						return
					}
					metadata = append(metadata, meanings...)
				}
			}

			taskName := fmt.Sprintf("导入【%s】", path)

			// 站点分类含义 → 子树归属站点键（site 含义 ID=站点 DB 行 id，经注册表投影解析）
			classifiedSiteKey := resolveClassifiedSiteKey(metadata, h.resolveSiteKeyByID)

			// 群成员（兄弟群组规则定群）各自声明为单文件作品（叶任务，归属按
			// 同目录站点分类裁决）；落单条目留在同目录聚合的目录组声明路径
			var singles []FileEntry
			for _, f := range group.files {
				if decisionByRel[f.RelPath].Group == nil {
					singles = append(singles, f)
					continue
				}
				attr := resolveWorkAttribution(filepath.Base(f.FullPath), classifiedSiteKey)
				if resp := buildDirGroupResponse(path, group.dirRelPath, taskName, metadata, []attributedFile{{entry: f, attr: attr}}); resp != nil {
					ch <- resp
				}
			}

			// 按归属裁决拆分：子任务站点继承父应答，同目录混合归属（真实域/local 回退）
			// 须拆为多个应答各自携带站点键
			localFiles, realFiles := partitionByAttribution(singles, classifiedSiteKey)
			if resp := buildDirGroupResponse(path, group.dirRelPath, taskName, metadata, localFiles); resp != nil {
				ch <- resp
			}
			if resp := buildDirGroupResponse(path, group.dirRelPath, taskName, metadata, realFiles); resp != nil {
				ch <- resp
			}
		}
	}()

	return sdkdto.StreamResult(ch), nil
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

// buildDirGroupResponse 为同目录同归属的一组文件构造任务应答：
// 单文件=叶任务（身份=该文件的作品 ID），多文件=父容器+子任务。
// 组内归属一致（调用方按归属裁决拆分）；local 组维持既有任务身份形态
// （local-dir 前缀父任务/哈希身份），真实域组的父任务身份加站点键后缀，
// 区分同目录拆分后的两组任务（PluginTaskId 是同流应答的父任务合并键）。
// 全部文件不可访问（os.Stat 失败）时返回 nil。
func buildDirGroupResponse(path, dirRelPath, taskName string, metadata []PathMeaning, files []attributedFile) *sdkdto.TaskCreateResponse {
	if len(files) == 0 {
		return nil
	}
	siteKey := files[0].attr.SiteKey

	children := make([]*sdkdto.TaskCreateChildResponse, 0, len(files))
	for _, af := range files {
		fi, err := os.Stat(af.entry.FullPath)
		if err != nil {
			continue
		}

		// 作品身份：真实域=解析出的页级作品 ID，local 回退=文件内容哈希
		workID := af.entry.Hash
		fp := &FilePluginData{
			SchemaVersion: currentPluginDataVersion,
			FullPath:      af.entry.FullPath,
			RelPath:       af.entry.RelPath,
			Hash:          af.entry.Hash,
			Size:          fi.Size(),
			Metadata:      metadata,
		}
		if af.attr.isReal() {
			workID = af.attr.PageWorkID
			fp.SiteKey = af.attr.SiteKey
			fp.SiteWorkId = af.attr.PageWorkID
		}
		fpJSON, _ := json.Marshal(fp)

		children = append(children, &sdkdto.TaskCreateChildResponse{
			TaskName:     filepath.Base(af.entry.FullPath),
			SiteWorkId:   workID,
			Url:          "local://" + af.entry.FullPath,
			PluginData:   string(fpJSON),
			ResourceType: classifyResourceType(af.entry.FullPath),
		})
	}

	if len(children) == 0 {
		return nil
	}

	if len(children) == 1 {
		return &sdkdto.TaskCreateResponse{
			PluginTaskId: children[0].SiteWorkId,
			TaskName:     children[0].TaskName,
			SiteWorkId:   children[0].SiteWorkId,
			Url:          children[0].Url,
			PluginData:   children[0].PluginData,
			SiteKey:      siteKey,
			ResourceType: children[0].ResourceType,
		}
	}

	dp := &DirPluginData{
		SchemaVersion: currentPluginDataVersion,
		DirRelPath:    dirRelPath,
		Metadata:      metadata,
	}
	dpJSON, _ := json.Marshal(dp)

	parentID := fmt.Sprintf("local-dir-%s", dirRelPath)
	if files[0].attr.isReal() {
		parentID = parentID + "@" + siteKey
	}

	return &sdkdto.TaskCreateResponse{
		PluginTaskId: parentID,
		TaskName:     taskName,
		SiteWorkId:   parentID,
		Url:          "local://" + filepath.Join(path, dirRelPath),
		PluginData:   string(dpJSON),
		SiteKey:      siteKey,
		ResourceType: "", // 有 children 时由各 child 声明(parent 不声明)
		Children:     children,
	}
}

// recomputeFileDecision 按 PluginData 携带的原始路径重算单文件的流水线结论：
// FileName/DirNames/RelPath 由路径分解重建（与 Create 期 buildScanFiles 同构），
// Siblings 经父目录 os.ReadDir 重扫（与 Create 期扫描同源同键——同目录常规文件
// 名全集、字典序）；库内索引不注入（与 Create 期一致，谓词规则恒不命中）——
// 现状核验结论：本方法与识别干跑（MergedRecognition 注入 BuildLibraryIndex 产物）
// 尚未对齐，谓词维度的执行期重算对齐属阶段4 义务（会话模式 Create 注入后同步）。
// 返回流水线结论与匹配输入（匹配输入供归属裁决重放规则取捕获值）。
func (h *LocalImportWorkFetcher) recomputeFileDecision(fp *FilePluginData, rules []RuleEntry) (FileDecision, *MatchInput) {
	dirNames := ExtractDirLevels(fp.RelPath)
	in := &MatchInput{
		FileName: filepath.Base(fp.FullPath),
		DirNames: dirNames,
		RelPath:  fp.RelPath,
		Siblings: h.rescanSiblingNames(filepath.Dir(fp.FullPath)),
	}
	ctx := &ParseContext{DirNames: dirNames, Depth: len(dirNames)}
	decision := FileDecision{RelPath: fp.RelPath, FileName: in.FileName}
	decision.Group, _ = runGroupStage(rules, in, nil, ctx)
	runExtractionStage(&decision, rules, in, nil, ctx)
	return decision, in
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

// pixivPageWorkIDOf 取内置 pixiv 规则重算的归属域键：流水线结论命中
// （MatchedRuleIDs 含其 ID）时对同一匹配输入重放该规则取捕获值——锚定全匹配下
// 捕获值即页级作品 ID（命中即真实域键）；未命中返回空串。
func pixivPageWorkIDOf(decision FileDecision, rules []RuleEntry, in *MatchInput) string {
	if !slices.Contains(decision.MatchedRuleIDs, builtinRuleIDPixivPageWorkID) {
		return ""
	}
	for _, rule := range rules {
		if rule.ID != builtinRuleIDPixivPageWorkID || rule.Pattern == nil {
			continue
		}
		ctx := &ParseContext{}
		if !rule.Pattern.Evaluate(in, nil, ctx) {
			return ""
		}
		value, _ := ctx.Capture(builtinPageWorkIDCapture)
		return value
	}
	return ""
}

// CreateWorkInfo 从 PluginData 反序列化路径元数据，并按原始路径重算解析内核流水线
// 合成产出作品信息：规则解析与 Create 共用同一入口（resolvePipelineRules），提取
// 声明经 AggregateSynthesis → Synthesize 得终选与待裁决——待裁决项逐条记日志
// （维度/候选值/原因，归因观测面）。终选映射：标题→作品名（无标题声明回退整个
// 文件名去扩展名，扩展名由 Resource.Format 单独提供）、作者→本地作者声明（带
// 声明级角色）、标签→本地标签声明、作品集→作品集声明（集名=声明值，集内序不传
// 契约——宿主续排近似，本期既定裁决）；路径含义（PathMeaning）映射维持现状。
// 归属裁决按内置 pixiv 规则重算：命中即真实域键（捕获值=页级作品 ID，与原
// parsePixivPageWorkID 逐例等价），未命中走现状非 pixiv 路径（PluginData 携带的
// 扫描期归属，无归属=文件内容哈希）。
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

	// 单任务重算流水线并合成终选声明（库内索引与 Create 期一致不注入）
	rules := h.resolvePipelineRules()
	decision, in := h.recomputeFileDecision(&fp, rules)
	final, pending := Synthesize(decision.Declarations, AggregateSynthesis(rules))
	h.logPendingSynthesis(fp.FullPath, pending)

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
			resp.WorkSets = append(resp.WorkSets, &sdkdto.TaskWorkSetDTO{
				SiteWorkSetId: "workSet:" + decl.Value,
				WorkSetName:   decl.Value,
			})
		}
	}

	// 作品身份：内置 pixiv 规则命中即真实域键；未命中=扫描期归属（真实域页级
	// 作品 ID）或 local 回退的文件内容哈希
	siteWorkID := fp.Hash
	if fp.hasRealAttribution() {
		siteWorkID = fp.SiteWorkId
	}
	if pageID := pixivPageWorkIDOf(decision, rules, in); pageID != "" {
		siteWorkID = pageID
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
			// （workSet:{名} 即 local 站点的作品集 ID 约定形态）。真实域归属作品的
			// 站点侧作品集 ID 无法从名字派生（键即身份，无 ID 不造行），跳过该含义
			if fp.hasRealAttribution() {
				if h.ctx != nil {
					h.ctx.Infof("作品集含义按名仅支持 local 归属，真实域归属(%s)跳过: %s", fp.SiteKey, m.Name)
				}
				continue
			}
			resp.WorkSets = append(resp.WorkSets, &sdkdto.TaskWorkSetDTO{
				SiteWorkSetId: "workSet:" + m.Name,
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
