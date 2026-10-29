// 资源选择域：从识别全景快照推导可选作品与集结构（最小选择粒度=作品、多轨原子；
// 集=序列群、默认聚合单元），承载选择集的持久化校验、创建编排动作（异步化：
// 创建前磁盘对账 → 一次 CreateTask 走 Create 过滤产出分支 → 选择集清理，均移
// 独立 goroutine；进度与终态经 createProgress/createDone 事件推送）与会话模式
// 任务产出的目录含义链构造（取快照回放结论，不触发目录问答）。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// —— 作品键与作品类别 ——

// 作品键前缀：区分两类作品键形态，保证键非空且两类键空间无歧义（裸相对路径不是
// 合法作品键——目录聚合作品的键指向目录而非任何单文件，文件级半选在键形态上
// 即不可表达）。
const (
	// workKeyPrefixSetMember 序列群成员作品：单文件作品，键=前缀+相对路径。
	workKeyPrefixSetMember = "f:"
	// workKeyPrefixDirTracks 目录聚合作品：同目录全部落单（未入群）文件合为一件
	// 多轨作品，键=前缀+目录键（库根直下为裸前缀）。
	workKeyPrefixDirTracks = "d:"
)

// 作品类别词汇（作品键的第二段语义，呈现与创建产出分组共用）。
const (
	// WorkKindSetMember 序列群成员：集内单文件作品（集=默认聚合单元，集内允许
	// 作品级显式拆选，每个成员自身完整）。
	WorkKindSetMember = "setMember"
	// WorkKindDirTracks 目录聚合作品：同目录落单文件的全部轨道为一件原子作品
	//（多轨不可拆选——轨道集由作品定义整体持有）。
	WorkKindDirTracks = "dirTracks"
)

// 作品呈现状态词汇（状态过滤与置灰判定用；worst-of-files 口径）。
const (
	workStateIdentified = "identified" // 全部文件已识别且无待裁决
	workStateClueful    = "clueful"    // 含有价值未识别文件
	workStatePlain      = "plain"      // 含无线索未识别文件
	workStatePending    = "pending"    // 含待裁决文件（未定论，不可选）
	workStateIgnored    = "ignored"    // 全部文件已忽略
)

// workSetKey 集（序列群）键：目录键 + NUL + 分组键（目录键为 '/' 路径不含 NUL，
// 连接无歧义）。
func workSetKey(dirRelKey, groupKey string) string {
	return dirRelKey + "\x00" + groupKey
}

// splitWorkSetKey 集键 → 目录键与分组键（集名候选）。
func splitWorkSetKey(setKey string) (dirRelKey, groupKey string) {
	dirRelKey, groupKey, _ = strings.Cut(setKey, "\x00")
	return dirRelKey, groupKey
}

// SelectableWork 可选作品：选择的最小粒度。Files 为作品全部轨道（相对库根的
// '/' 路径）——选择状态只挂作品键，轨道集随作品整体进出选择集，结构上表达
// 不了「半作品」。
type SelectableWork struct {
	// WorkID 作品键（两类前缀之一 + 寻址段）。
	WorkID string `json:"workId"`
	// Kind 作品类别（WorkKindSetMember / WorkKindDirTracks）。
	Kind string `json:"kind"`
	// DirRelKey 所属目录键（'/' 路径，库根直下为空串）。
	DirRelKey string `json:"dirRelKey"`
	// Files 作品全部轨道的相对路径（集成员恰一条；目录聚合为该目录全部落单文件）。
	Files []string `json:"files"`
	// SetKey 所属集键（仅集成员非空；目录聚合作品不可拆、不属任何集）。
	SetKey string `json:"setKey,omitempty"`
	// Selectable 可选=全部文件已识别且无待裁决（未识别与待裁决均不可选——
	// 任务数据=识别结论的闭环保持完整）。
	Selectable bool `json:"selectable"`
	// State 呈现状态（状态过滤与置灰用，worst-of-files 口径）。
	State string `json:"state"`
}

// WorkSetView 集（序列群）的呈现视图：成员作品键按集内序排列。
type WorkSetView struct {
	// SetKey 集键；DirRelKey 所在目录键；Key 分组键（公共前缀=集名候选）。
	SetKey    string `json:"setKey"`
	DirRelKey string `json:"dirRelKey"`
	Key       string `json:"key"`
	// Members 成员作品键（集内序）。
	Members []string `json:"members"`
}

// WorkDomain 识别快照推导的选择域：全部作品与集，及按作品键的索引。
type WorkDomain struct {
	// Works 全部作品（快照文件序=目录主序的确定序）。
	Works []*SelectableWork `json:"works"`
	// Sets 全部集（目录与分组键字典序）。
	Sets []*WorkSetView `json:"sets"`
	// byWork 作品键索引（非序列化）。
	byWork map[string]*SelectableWork
}

// Work 按作品键取作品（未命中 nil）。
func (d *WorkDomain) Work(workID string) *SelectableWork {
	if d == nil {
		return nil
	}
	return d.byWork[workID]
}

// ResolveAll 批量按作品键取作品；未命中的键收集于 missing（快照重识别后选择集
// 悬空项的呈现依据）。
func (d *WorkDomain) ResolveAll(workIDs []string) (works []*SelectableWork, missing []string) {
	if d == nil {
		return nil, append([]string(nil), workIDs...)
	}
	for _, id := range workIDs {
		if work := d.byWork[id]; work != nil {
			works = append(works, work)
			continue
		}
		missing = append(missing, id)
	}
	return works, missing
}

// workVitality 作品的可选性与呈现状态（worst-of-files：任一文件未识别/待裁决
// 即拉低整体；全部文件已忽略呈现 ignored 态）。
func workVitality(fileByPath map[string]*SnapFile, ignored map[string]bool, files []string) (selectable bool, state string) {
	allIgnored := true
	hasPending, hasPlain, hasClueful := false, false, false
	for _, rel := range files {
		rec := fileByPath[rel]
		if rec == nil {
			// 快照无此文件记录（游标异常形态）：按不可选的无线索处理
			hasPlain = true
			allIgnored = false
			continue
		}
		if len(rec.PendingDims) > 0 {
			hasPending = true
		}
		switch rec.State {
		case snapStateIdentified:
			// 已识别文件不拉低作品呈现态
		case snapStateClueful:
			hasClueful = true
		default:
			hasPlain = true
		}
		if !ignored[rel] {
			allIgnored = false
		}
	}
	switch {
	case allIgnored:
		return false, workStateIgnored
	case hasPending:
		return false, workStatePending
	case hasPlain:
		return false, workStatePlain
	case hasClueful:
		return false, workStateClueful
	default:
		return true, workStateIdentified
	}
}

// DeriveWorkDomain 从识别全景快照推导选择域：
//   - GroupKey 非空的文件（规则定群与结构群成员）各自成单文件作品，按
//     「目录+分组键」归集为集（集内序取快照 GroupSeq）；
//   - 其余文件按所属目录聚合为一件多轨作品（轨道集=该目录全部落单文件，原子）。
//
// ignored 为会话已忽略项（相对路径集合，呈现态用；忽略不剥夺可选性——已识别
// 的忽略项仍可被显式选为任务）。
func DeriveWorkDomain(snap *RecognitionSnapshot, ignored map[string]bool) *WorkDomain {
	domain := &WorkDomain{byWork: make(map[string]*SelectableWork), Works: []*SelectableWork{}, Sets: []*WorkSetView{}}
	if snap == nil {
		return domain
	}
	fileByPath := make(map[string]*SnapFile, len(snap.Files))
	for i := range snap.Files {
		fileByPath[snap.Files[i].RelPath] = &snap.Files[i]
	}
	setMembers := make(map[string][]*SelectableWork)
	dirTracks := make(map[string]*SelectableWork)
	appendWork := func(work *SelectableWork) {
		domain.Works = append(domain.Works, work)
		domain.byWork[work.WorkID] = work
	}
	for i := range snap.Files {
		f := &snap.Files[i]
		if f.GroupKey != "" {
			work := &SelectableWork{
				WorkID:    workKeyPrefixSetMember + f.RelPath,
				Kind:      WorkKindSetMember,
				DirRelKey: f.DirRelKey,
				Files:     []string{f.RelPath},
				SetKey:    workSetKey(f.DirRelKey, f.GroupKey),
			}
			appendWork(work)
			setMembers[work.SetKey] = append(setMembers[work.SetKey], work)
			continue
		}
		work := dirTracks[f.DirRelKey]
		if work == nil {
			work = &SelectableWork{
				WorkID:    workKeyPrefixDirTracks + f.DirRelKey,
				Kind:      WorkKindDirTracks,
				DirRelKey: f.DirRelKey,
			}
			dirTracks[f.DirRelKey] = work
			appendWork(work)
		}
		work.Files = append(work.Files, f.RelPath)
	}
	for _, work := range domain.Works {
		work.Selectable, work.State = workVitality(fileByPath, ignored, work.Files)
	}
	for _, setKey := range sortedKeys(setMembers) {
		members := setMembers[setKey]
		sort.SliceStable(members, func(i, j int) bool {
			return fileByPath[members[i].Files[0]].GroupSeq < fileByPath[members[j].Files[0]].GroupSeq
		})
		dirKey, groupKey := splitWorkSetKey(setKey)
		ids := make([]string, 0, len(members))
		for _, member := range members {
			ids = append(ids, member.WorkID)
		}
		domain.Sets = append(domain.Sets, &WorkSetView{SetKey: setKey, DirRelKey: dirKey, Key: groupKey, Members: ids})
	}
	return domain
}

// domainViewOf 选择域的前端投影（ Works/Sets 已是非 nil 切片，直接透传）。
func domainViewOf(domain *WorkDomain) *WorkDomain {
	if domain == nil {
		return nil
	}
	view := &WorkDomain{Works: domain.Works, Sets: domain.Sets}
	return view
}

// ignoredSetOf 会话忽略项 → 相对路径集合。
func ignoredSetOf(sess *WorkbenchSession) map[string]bool {
	ignored := make(map[string]bool, len(sess.Ignored))
	for _, rel := range sess.Ignored {
		ignored[rel] = true
	}
	return ignored
}

// dirMeaningChains 快照目录树 → 各目录的层含义链（根至该目录逐层累积；根层深 0
// 无规则索引、不承载含义）。会话模式任务产出的元数据取本链（快照内规则回放
// 结论），不经目录分类问答——会话模式对 ClassifyDir 结构性零调用。
func dirMeaningChains(snap *RecognitionSnapshot) map[string][]PathMeaning {
	chains := make(map[string][]PathMeaning)
	if snap == nil {
		return chains
	}
	byKey := make(map[string]*SnapDir, len(snap.Dirs))
	for i := range snap.Dirs {
		byKey[snap.Dirs[i].RelKey] = &snap.Dirs[i]
	}
	var chainOf func(dirKey string) []PathMeaning
	chainOf = func(dirKey string) []PathMeaning {
		if cached, ok := chains[dirKey]; ok {
			return cached
		}
		var chain []PathMeaning
		if node := byKey[dirKey]; node != nil {
			if parentKey := dirRelKeyOf(dirKey); parentKey != dirKey {
				chain = append(chain, chainOf(parentKey)...)
			}
			chain = append(chain, node.Meanings...)
		}
		chains[dirKey] = chain
		return chain
	}
	for _, key := range sortedKeys(byKey) {
		chainOf(key)
	}
	return chains
}

// —— 选择集持久化（经会话管理器同点串行）——

// SetSelection 全量替换资源选择集（作品键集合）：逐键校验存在于当前选择域且
// 可选（未识别/含待裁决的作品不可选），键去重后按稳定序落库。作品为原子——
// 本接口只接受作品键，目录聚合作品的轨道不可单独进出选择集。
func (m *SessionManager) SetSelection(root string, workIDs []string) (*WorkbenchSession, error) {
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	if sess.Snapshot == nil {
		return nil, fmt.Errorf("该会话尚未完成全量识别，无选择域可选")
	}
	domain := DeriveWorkDomain(sess.Snapshot, ignoredSetOf(sess))
	seen := make(map[string]bool, len(workIDs))
	ordered := make([]string, 0, len(workIDs))
	for _, id := range workIDs {
		work := domain.Work(id)
		if work == nil {
			return nil, fmt.Errorf("作品键 %q 不在当前识别全景的选择域内（识别结构可能已变化），请刷新后重选", id)
		}
		if !work.Selectable {
			return nil, fmt.Errorf("作品 %s 尚不可选（含未识别或待裁决文件，须先完成解析）", displayOfWork(work))
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	return m.mutateSession(root, func(sess *WorkbenchSession) error {
		sess.Selection = ordered
		return nil
	})
}

// displayOfWork 作品的人话指称（告警与错误文案用）。
func displayOfWork(work *SelectableWork) string {
	if work.Kind == WorkKindDirTracks {
		return fmt.Sprintf("「%s」目录聚合作品（%d 轨）", work.DirRelKey, len(work.Files))
	}
	return fmt.Sprintf("「%s」", strings.TrimPrefix(work.WorkID, workKeyPrefixSetMember))
}

// —— 创建前磁盘对账（守卫）——

// ReconcileSelectionDisk 选择集对磁盘现状复核：所涉文件的存在性（被移动/删除即
// 阻断）+ 所涉目录的 mtime 与识别快照游标比对（识别后有新增/删除/移动即目录
// mtime 漂移、阻断）。阻断文案提示重新识别受影响子树。
func ReconcileSelectionDisk(root string, snap *RecognitionSnapshot, works []*SelectableWork) error {
	for _, work := range works {
		for _, rel := range work.Files {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
				return fmt.Errorf("磁盘对账未通过：所选文件 %s 已不存在（可能被移动或删除），请重新识别受影响子树后再创建", rel)
			}
		}
	}
	seenDir := make(map[string]bool, len(works))
	for _, work := range works {
		if seenDir[work.DirRelKey] {
			continue
		}
		seenDir[work.DirRelKey] = true
		want, ok := snap.DirMtimes[work.DirRelKey]
		if !ok {
			continue
		}
		fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(work.DirRelKey)))
		if err != nil {
			return fmt.Errorf("磁盘对账未通过：所选目录 %s 不可访问，请重新识别后再创建", displayDirKey(work.DirRelKey))
		}
		if fi.ModTime().UnixNano() != want {
			return fmt.Errorf("磁盘对账未通过：目录 %s 在识别后有变动（新增、删除或移动过文件），请重新识别后再创建", displayDirKey(work.DirRelKey))
		}
	}
	return nil
}

// displayDirKey 目录键的展示形态（库根直下显示为「库根」）。
func displayDirKey(dirRelKey string) string {
	if dirRelKey == "" {
		return "库根"
	}
	return dirRelKey
}

// —— 创建编排动作 ——

// defaultCreateTaskName 默认任务名：本地导入 · <库根名> · N 件作品（库根名=库根
// basename；N=选择集作品数）。前端名称确认对话框的预填与后端空名兜底共用本规则。
func defaultCreateTaskName(root string, workCount int) string {
	return fmt.Sprintf("本地导入 · %s · %d 件作品", filepath.Base(root), workCount)
}

// resolveCreateTaskName 任务名裁决：空白输入（空串/全空白）按默认名兜底，非空白
// 输入裁剪首尾空白后采用——编排侧（通知文案）与过滤产出侧（父行命名）共用本
// 函数，单一裁决点（两侧行为不漂移）。
func resolveCreateTaskName(taskName, root string, workCount int) string {
	if name := strings.TrimSpace(taskName); name != "" {
		return name
	}
	return defaultCreateTaskName(root, workCount)
}

// —— 创建编排与过滤产出之间的进程内传值 ——
//
// 宿主 CreateTask RPC 同步覆盖完整产出流（过滤产出分支的流式应答被宿主消费完才
// 返回），编排动作据此在调用前暂存任务名原文、调用后读取批次摘要——两侧值在 RPC
// 返回时均已就绪，无需跨请求持久化（传值槽不落会话存储，重启即弃）。

// stageCreateTaskName 暂存创建任务名原文（ApplyCreateTasks 在宿主 CreateTask 调
// 用前置入）并重置该库根的批次摘要槽（防上一次残留被本次编排误读）。
func (m *SessionManager) stageCreateTaskName(root, taskName string) {
	m.createCarryMu.Lock()
	defer m.createCarryMu.Unlock()
	if m.stagedTaskNames == nil {
		m.stagedTaskNames = make(map[string]string)
	}
	m.stagedTaskNames[root] = taskName
	delete(m.batchSummaries, root)
}

// takeStagedCreateTaskName 取出并清除暂存的创建任务名（过滤产出分支消费；缺席=
// 空串——直驱路径经 resolveCreateTaskName 按默认名兜底）。
func (m *SessionManager) takeStagedCreateTaskName(root string) string {
	m.createCarryMu.Lock()
	defer m.createCarryMu.Unlock()
	name := m.stagedTaskNames[root]
	delete(m.stagedTaskNames, root)
	return name
}

// recordCreateBatchSummary 记录一次过滤产出的批次摘要（emitSelectedWorks 产出
// 完成时回写）。
func (m *SessionManager) recordCreateBatchSummary(root string, summary createBatchSummary) {
	m.createCarryMu.Lock()
	defer m.createCarryMu.Unlock()
	if m.batchSummaries == nil {
		m.batchSummaries = make(map[string]createBatchSummary)
	}
	m.batchSummaries[root] = summary
}

// takeCreateBatchSummary 取出并清除批次摘要（编排动作在宿主 CreateTask 返回后
// 读取；缺席=零值——宿主桩直答等无产出路径无摘要，文案不追加归属附注）。
func (m *SessionManager) takeCreateBatchSummary(root string) createBatchSummary {
	m.createCarryMu.Lock()
	defer m.createCarryMu.Unlock()
	summary := m.batchSummaries[root]
	delete(m.batchSummaries, root)
	return summary
}

// createBatchSummary 一次工作台创建的批次摘要（emitSelectedWorks 产出完成
// 时记录、编排动作在宿主 CreateTask 返回后读取）：子行总数与非 local 子行的
// 归属分布供通知文案的归属附注。
type createBatchSummary struct {
	// total 产出的子行总数。
	total int
	// nonLocalBySite 非 local 子行的归属分布（站点展示名→件数，站点按首见序）。
	nonLocalBySite []siteFileCount
}

// siteFileCount 单站点的非 local 子行计数（批次内归属同一真实域站点的子行数）。
type siteFileCount struct {
	siteName string
	files    int
}

// addNonLocal 记录一件非 local 子行的归属站点（按展示名归并计数）。
func (s *createBatchSummary) addNonLocal(siteName string) {
	for i := range s.nonLocalBySite {
		if s.nonLocalBySite[i].siteName == siteName {
			s.nonLocalBySite[i].files++
			return
		}
	}
	s.nonLocalBySite = append(s.nonLocalBySite, siteFileCount{siteName: siteName, files: 1})
}

// —— 已随 createTasks 异步化（大载荷分页方案·设计 C）退役删除：原 CreateTasksOutcome
//（编排动作的同步返回结构）随编排动作改为立即应答 started 而删除，创建结论经
// createDone 事件推送（runCreateTasks）。

// buildCreateTasksMessage 创建完成的通知文案：已创建任务「名」：N 件作品（M 个
// 子任务，Created 待手动开始）；批次含非 local 归属子行时按归属分布追加
// 「其中 n 件归属 站点名」（逐站点一段、顿号连写）。文案按归属分布组织，与父行
// 形态正交（归属附注分支不随父行分组形态变化）。
func buildCreateTasksMessage(taskName string, works, tasks int, summary createBatchSummary) string {
	message := fmt.Sprintf("已创建任务「%s」：%d 件作品（%d 个子任务，Created 待手动开始）", taskName, works, tasks)
	parts := make([]string, 0, len(summary.nonLocalBySite))
	for _, site := range summary.nonLocalBySite {
		parts = append(parts, fmt.Sprintf("%d 件归属 %s", site.files, site.siteName))
	}
	if len(parts) > 0 {
		message += "；其中 " + strings.Join(parts, "、")
	}
	return message
}

// —— 创建异步事件与运行态（createTasks 立即应答后的进度与终态推送，设计 C）——

// 创建事件种类（workbenchTopicEvent 通道的 kind 词汇）。
const (
	// workbenchEventCreateProgress 创建进度（两相）：核对文件（verify，逐作品
	// 哈希推进）→ 创建子任务（create，发射循环计数）。
	workbenchEventCreateProgress = "createProgress"
	// workbenchEventCreateDone 创建终态：成功携带摘要文案（选择集已清空）；失败
	// 保留选择集并携带部分完成信息「已创建 X/N 个子任务」。
	workbenchEventCreateDone = "createDone"
)

// 创建进度相词汇。
const (
	createPhaseVerify = "verify" // 核对文件（逐作品哈希推进）
	createPhaseCreate = "create" // 创建子任务（发射循环计数）
)

// createRunRegistry 创建运行态登记表（归一化库根 → *createRunState）：同库根
// 创建在跑互斥 + 发射进度槽（失败路径「已创建 X/N 个子任务」的数据源）。落位
// 说明：SessionManager 结构面本阶段禁扩（session.go 限定只改 createTasks case
// 与应答字段），运行态以包级登记表承载——键经 normalizeLibraryRoot 归一（跨
// 管理器实例唯一识别同一库；生产进程单管理器，测试库根互不相交）。
var createRunRegistry sync.Map

// createRunState 单库根的创建运行态。
type createRunState struct {
	mu      sync.Mutex
	running bool
	// emitted / planned 已发射与计划子行总数（发射侧逐批落账；planned=0 表示
	// 发射未开始——失败路径据此区分「零产出失败」与「部分完成中断」）。
	emitted int
	planned int
}

// createRunKey 创建运行态登记键（归一化库根，与会话键同一归一函数）。
func createRunKey(root string) string {
	return normalizeLibraryRoot(root)
}

// beginCreateRun 登记创建启动（重置进度槽）：已在跑返回假——同库根重复
// createTasks 在守卫链以「创建进行中」拒绝。
func beginCreateRun(root string) bool {
	st, _ := createRunRegistry.LoadOrStore(createRunKey(root), &createRunState{})
	state := st.(*createRunState)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.running {
		return false
	}
	state.running, state.emitted, state.planned = true, 0, 0
	return true
}

// endCreateRun 结束创建运行并摘除登记（进度槽如需保留由调用方先行读取）。
func endCreateRun(root string) {
	if st, ok := createRunRegistry.LoadAndDelete(createRunKey(root)); ok {
		state := st.(*createRunState)
		state.mu.Lock()
		state.running = false
		state.mu.Unlock()
	}
}

// recordCreateEmitProgress 落发射进度（发射侧每批回调；与终态读取间以 CreateTask
// RPC 返回为大致界，失败时读到的是最近一批的快照——「已创建 X/N」按此近似）。
func recordCreateEmitProgress(root string, emitted, planned int) {
	st, _ := createRunRegistry.LoadOrStore(createRunKey(root), &createRunState{})
	state := st.(*createRunState)
	state.mu.Lock()
	state.emitted, state.planned = emitted, planned
	state.mu.Unlock()
}

// takeCreateEmitProgress 读取并清零发射进度（失败路径部分完成信息数据源）。
func takeCreateEmitProgress(root string) (emitted, planned int) {
	if st, ok := createRunRegistry.Load(createRunKey(root)); ok {
		state := st.(*createRunState)
		state.mu.Lock()
		emitted, planned, state.emitted, state.planned = state.emitted, state.planned, 0, 0
		state.mu.Unlock()
	}
	return emitted, planned
}

// noteCreateProgress 推送 createProgress 事件并登记发射进度：create 相（发射
// 循环计数）同时落运行态进度槽（abortCreateTasks 的部分完成信息数据源），
// verify 相只推事件。上下文不可用时静默（不影响创建本身）。
func (m *SessionManager) noteCreateProgress(root, phase string, done, total int) {
	if phase == createPhaseCreate {
		recordCreateEmitProgress(root, done, total)
	}
	m.publishCreateEvent(&workbenchFrontendEvent{
		Kind: workbenchEventCreateProgress, Root: root,
		Phase: phase, Done: done, Total: total,
	})
}

// publishCreateEvent 创建事件经工作台事件通道推送前端（与识别事件同通道同载荷
// 结构，见 work_fetch.go workbenchFrontendEvent；序列化/发布失败仅告警）。
func (m *SessionManager) publishCreateEvent(evt *workbenchFrontendEvent) {
	if m.ctx == nil {
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		m.warnf("创建事件序列化失败: %v", err)
		return
	}
	if err := m.ctx.PublishToFrontend(workbenchTopicEvent, data); err != nil {
		m.warnf("创建事件推送失败(kind=%s): %v", evt.Kind, err)
	}
}

// ApplyCreateTasks 创建编排动作（异步化，大载荷分页方案·设计 C）：守卫链照旧
// （选择集解析 → 创建前对账〔磁盘存在性/目录 mtime 游标 + 档案指纹与会话赋值
// 修订号对快照基准一致〕，另置同库根「创建进行中」并发拒绝于链首）→ 置工作台
// 发起标记并暂存任务名原文后立即返回 started=true（工作台订阅循环不再被长创建
// 阻塞；前端经 createProgress/createDone 事件感知进度与终态）→ 创建 RPC（含
// 完整 Create 流消费）在独立 goroutine 执行（runCreateTasks）。taskName 为可选
// 自定义任务名（空/全空白=默认名兜底）。任一守卫未过即阻断（零任务零标记零
// 运行态残留）。
func (m *SessionManager) ApplyCreateTasks(root, taskName string) (bool, error) {
	// 并发防护置首：同库根重复 createTasks 以「创建进行中」拒绝——先于其余守卫，
	// 保证创建在途时的重复发起必中此拒因
	if !beginCreateRun(root) {
		return false, fmt.Errorf("创建进行中：该库根已有一次创建在途，请等待完成通知后再发起")
	}
	fail := func(err error) (bool, error) {
		endCreateRun(root)
		return false, err
	}
	sess, err := m.loadSession(root)
	if err != nil {
		return fail(err)
	}
	if sess.Snapshot == nil {
		return fail(fmt.Errorf("该会话尚未完成全量识别，无法创建任务"))
	}
	if len(sess.Selection) == 0 {
		return fail(fmt.Errorf("未选择任何作品：请在识别全景勾选要导入的作品后再创建"))
	}
	domain := DeriveWorkDomain(sess.Snapshot, ignoredSetOf(sess))
	works, missing := domain.ResolveAll(sess.Selection)
	if len(missing) > 0 {
		return fail(fmt.Errorf("选择集中 %d 项已不在当前识别全景内（识别结构可能已变化），请刷新后重选", len(missing)))
	}
	if err := ReconcileSelectionDisk(root, sess.Snapshot, works); err != nil {
		return fail(err)
	}
	if current := m.currentProfileDigest(); current != "" && sess.Snapshot.ProfileDigest != "" && current != sess.Snapshot.ProfileDigest {
		return fail(fmt.Errorf("规则档案在识别后被变更，全景结论可能与最新规则不一致，请重新识别后再创建"))
	}
	if sess.AssignRevision != sess.Snapshot.AssignmentRevision {
		return fail(fmt.Errorf("会话赋值在识别后有变更，全景结论可能过期，请重新识别后再创建"))
	}
	if m.ctx == nil {
		return fail(fmt.Errorf("插件上下文不可用，无法发起创建"))
	}
	// 任务名裁决（空/全空白=默认名）：编排侧文案与过滤产出侧父行命名共用同一函数
	resolvedName := resolveCreateTaskName(taskName, root, len(works))
	if err := m.BeginWorkbenchCreate(root); err != nil {
		return fail(err)
	}
	// 暂存任务名原文并重置摘要槽：过滤产出分支经宿主 CreateTask RPC 同步递归消费
	// 暂存名、产出完成后回写批次摘要——RPC 返回时两侧均已就绪（时序契约不变，
	// 仅外移 goroutine，见 runCreateTasks）
	m.stageCreateTaskName(root, taskName)
	go m.runCreateTasks(root, resolvedName, len(works))
	return true, nil
}

// runCreateTasks 创建编排主体（ApplyCreateTasks 启动的独立 goroutine）：宿主
// CreateTask RPC 同步覆盖完整产出流——返回时过滤产出侧的暂存名消费与批次摘要
// 回写均已就绪（selection.go「创建编排与过滤产出之间的进程内传值」注释的时序
// 契约在此保持）。终态经 createDone 事件推送：成功=清空选择集（赋值与快照保留）
// +摘要文案；失败=abortCreateTasks（保留选择集+部分完成信息）。运行态由 defer
// 摘除。
func (m *SessionManager) runCreateTasks(root, taskName string, workCount int) {
	defer endCreateRun(root)
	resp, err := m.ctx.CreateTask("local://" + root)
	if err != nil {
		m.abortCreateTasks(root, fmt.Sprintf("宿主创建任务失败: %v", err))
		return
	}
	if resp == nil || !resp.Succeed {
		msg := "宿主未说明原因"
		if resp != nil {
			msg = resp.Msg
		}
		m.abortCreateTasks(root, fmt.Sprintf("宿主创建任务失败: %s", msg))
		return
	}
	if _, err := m.mutateSession(root, func(sess *WorkbenchSession) error {
		sess.Selection = nil
		return nil
	}); err != nil {
		m.warnf("创建完成后清理选择集失败（任务已产出）: %v", err)
	}
	summary := m.takeCreateBatchSummary(root)
	tasks := int(resp.AddedQuantity)
	m.publishCreateEvent(&workbenchFrontendEvent{
		Kind: workbenchEventCreateDone, Root: root, Success: true,
		Done: tasks, Total: tasks,
		Message: buildCreateTasksMessage(taskName, workCount, tasks, summary),
	})
}

// abortCreateTasks 创建失败收尾：回收发起标记与暂存任务名（零残留）+ createDone
// 终态事件——保留选择集供重试；发射已开始（进度槽 planned>0）时携带部分完成
// 信息「已创建 X/N 个子任务」（X=发射侧最近一批快照，近似宿主已收行数——
// error 收流时已到子行与父行保留的部分完成语义，宿主按 Created 待开始承载）。
func (m *SessionManager) abortCreateTasks(root, reason string) {
	m.ConsumeWorkbenchCreateMarker(root)
	m.takeStagedCreateTaskName(root)
	emitted, planned := takeCreateEmitProgress(root)
	var message string
	done, total := 0, 0
	if planned > 0 {
		done, total = emitted, planned
		message = fmt.Sprintf("任务创建中断：已创建 %d/%d 个子任务（父任务已入库，可删除后重试；选择集已保留）：%s", emitted, planned, reason)
	} else {
		message = fmt.Sprintf("任务创建失败（选择集已保留，可重试）：%s", reason)
	}
	m.publishCreateEvent(&workbenchFrontendEvent{
		Kind: workbenchEventCreateDone, Root: root, Success: false,
		Done: done, Total: total, Message: message,
	})
}
