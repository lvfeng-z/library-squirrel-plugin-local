// 资源选择域：从识别全景快照推导可选作品与集结构（最小选择粒度=作品、多轨原子；
// 集=序列群、默认聚合单元），承载选择集的持久化校验、创建编排动作（异步化：
// 创建前目录级 mtime 对账〔逐文件存在性核对已并入 verify 哈希循环，见
// hashWorkFilesAllOrNothing〕→ 一次 CreateTask 走 Create 过滤产出分支 → 选择集
// 清理，均移独立 goroutine；进度与终态经 createProgress/createDone 事件推送）与
// 会话模式任务产出的目录含义链构造（取快照回放结论，不触发目录问答）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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

// domainViewOf 选择域的前端投影：条目成员数组按预览上限截断——作品的轨道
//（Files）与集的成员作品键（Members）截前 panoramaMemberPreviewLimit 个（浅拷贝
// 条目，不动推导域本体；getPanorama domain 分节分页按同一截断逐条投影，语义
// 守卫=逐页拼接与本全量投影逐条全等，两侧同截断）。截断仅出线层：创建/守卫链
// 等服务端动作仍消费全量域（workDomainOf），选择/创建按作品键寻址不受影响。
func domainViewOf(domain *WorkDomain) *WorkDomain {
	if domain == nil {
		return nil
	}
	view := &WorkDomain{
		Works: make([]*SelectableWork, len(domain.Works)),
		Sets:  make([]*WorkSetView, len(domain.Sets)),
	}
	for i, w := range domain.Works {
		cw := *w
		cw.Files = previewWireMembers(cw.Files)
		view.Works[i] = &cw
	}
	for i, s := range domain.Sets {
		cs := *s
		cs.Members = previewWireMembers(cs.Members)
		view.Sets[i] = &cs
	}
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
	sess, err := m.cachedSession(root) // 只读校验前置读：共享只读实例
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

// ReconcileSelectionDisk 选择集对磁盘现状复核（守卫链仅目录级）：所涉目录的
// mtime 与识别快照游标比对（识别后有新增/删除/移动即目录 mtime 漂移、阻断），
// 阻断文案提示重新识别受影响子树。
//
// 逐文件存在性核对已并入 verify 哈希循环（守卫链与会话瘦身四刀之四：全库
// 选择集 15 万级文件的守卫链串行 Stat 是 27.7s 的主要构成——文件级核对移至
// hashWorkFilesAllOrNothing 的 Stat，失败即整次创建中断、对账文案经中止原因
// 收流；守卫链只留目录级游标抽查，真实库约 6,210 目录，秒级）。
func ReconcileSelectionDisk(root string, snap *RecognitionSnapshot, works []*SelectableWork) error {
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

// buildCreateTasksMessage 创建完成的通知文案（剧本帧6）：已创建任务「名」：N 件
// 作品，共 M 个子任务，用时 X 分钟；批次含非 local 归属子行时按归属分布追加
// 「其中 n 件归属 站点名」（逐站点一段、顿号连写）。文案按归属分布组织，与父行
// 形态正交（归属附注分支不随父行分组形态变化）。
func buildCreateTasksMessage(taskName string, works, tasks int, elapsed time.Duration, summary createBatchSummary) string {
	message := fmt.Sprintf("已创建任务「%s」：%d 件作品，共 %d 个子任务，%s", taskName, works, tasks, formatCreateElapsed(elapsed))
	parts := make([]string, 0, len(summary.nonLocalBySite))
	for _, site := range summary.nonLocalBySite {
		parts = append(parts, fmt.Sprintf("%d 件归属 %s", site.files, site.siteName))
	}
	if len(parts) > 0 {
		message += "；其中 " + strings.Join(parts, "、")
	}
	return message
}

// formatCreateElapsed 创建用时文案（剧本帧6「用时 14 分钟」形态）：满 1 分钟按
// 四舍五入整分钟（下限 1），不足 1 分钟固定「用时不足 1 分钟」。
func formatCreateElapsed(elapsed time.Duration) string {
	if elapsed < time.Minute {
		return "用时不足 1 分钟"
	}
	minutes := int(elapsed.Minutes() + 0.5)
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("用时 %d 分钟", minutes)
}

// —— 创建异步事件与运行态（createTasks 立即应答后的进度与终态推送，设计 C）——

// 创建事件种类（workbenchTopicEvent 通道的 kind 词汇）。
const (
	// workbenchEventCreateProgress 创建进度（两相）：核对文件（verify，逐作品
	// 哈希推进）→ 创建子任务（create，发射循环计数）。
	workbenchEventCreateProgress = "createProgress"
	// workbenchEventCreateDone 创建终态：成功携带摘要文案（选择集已清空）；失败
	// 保留选择集并按决策4 统一回滚携带在途清理与重试信息（剧本帧5）。
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
	// cancelled 取消位（cancelCreate 工作台动作置位，甲3/决策3）：verify/发射
	// 循环经 runCtx 传播感知（cancelFn 一并取消），收流不发末批 last=宿主未收批
	// 统一回滚；置位后创建侧终态走取消文案（finishCancelledCreate）。
	cancelled bool
	// committed 原子提交点（风险10）：末批 last 已发（或降级纯内联整体发射）
	//——其后到达的取消请求应答「已完成」、不置取消位、不回滚已收批产出。
	committed bool
	// cancelFn 在途创建的 runCtx 取消函数（verify 发射循环注册 arm/收尾 disarm；
	// 置位取消位时同步调用，令读循环 4MB 分块处立即感知）。
	cancelFn context.CancelFunc
	// done 发射侧收尾信号（emitSelectedWorks 全部退出路径关闭）：宿主创建流
	// 提前终结时编排侧等待发射侧落定的观测点——关闭即核对 worker 池已收流、
	// 发射进度槽已定格（哈希缓存回写在其后进行，不纳入等待）。
	done chan struct{}
}

// createRunKey 创建运行态登记键（归一化库根，与会话键同一归一函数）。
func createRunKey(root string) string {
	return normalizeLibraryRoot(root)
}

// beginCreateRun 登记创建启动（重置进度槽与取消/提交位）：已在跑返回假——同库根
// 重复 createTasks 在守卫链以「创建进行中」拒绝。
func beginCreateRun(root string) bool {
	st, _ := createRunRegistry.LoadOrStore(createRunKey(root), &createRunState{})
	state := st.(*createRunState)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.running {
		return false
	}
	state.running, state.emitted, state.planned = true, 0, 0
	state.cancelled, state.committed, state.cancelFn = false, false, nil
	state.done = nil
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

// —— 取消通路（甲3/决策3）：cancelCreate 工作台动作 → 取消位置位 → verify/
// 发射循环收流 → 宿主未收批统一回滚 → 取消终态 —— //

// cancelCreate 动作的受理结论词汇（应答 CancelState 字段值）。
const (
	// createCancelAccepted 取消已受理：取消位置位（runCtx 已取消），创建侧
	// 提前收流（不发末批 last），宿主回滚在途行，终态经 createDone 事件到达。
	createCancelAccepted = "cancelling"
	// createCancelCompleted 恰好已完成：末批 last 已发（原子提交点已过，风险10）
	//——取消语义不成立，不清理、照常发完成通知。
	createCancelCompleted = "completed"
	// createCancelIdle 无在途创建（已收尾或从未发起）。
	createCancelIdle = "idle"
)

// requestCreateCancel 受理取消请求（请求循环调用，创建在独立 goroutine 不受
// 阻塞）：无在途=idle；原子提交点已过=completed（不置位不回滚，风险10）；否则
// 置取消位并同步取消 runCtx（读循环 4MB 分块处感知）=accepted。核对相与发射相
//（含重叠期）均可用；零发射阶段（verify 中）取消则无开批父无行可删、直接终态。
func requestCreateCancel(root string) string {
	st, ok := createRunRegistry.Load(createRunKey(root))
	if !ok {
		return createCancelIdle
	}
	state := st.(*createRunState)
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.running {
		return createCancelIdle
	}
	if state.committed {
		return createCancelCompleted
	}
	state.cancelled = true
	if state.cancelFn != nil {
		state.cancelFn()
	}
	return createCancelAccepted
}

// isCreateCancelled 读取取消位（runCreateTasks 终态分派用；循环内感知走 runCtx）。
func isCreateCancelled(root string) bool {
	if st, ok := createRunRegistry.Load(createRunKey(root)); ok {
		state := st.(*createRunState)
		state.mu.Lock()
		defer state.mu.Unlock()
		return state.cancelled
	}
	return false
}

// createRunCommitted 读取原子提交位（风险10：已收批的创建照常成功收尾）。
func createRunCommitted(root string) bool {
	if st, ok := createRunRegistry.Load(createRunKey(root)); ok {
		state := st.(*createRunState)
		state.mu.Lock()
		defer state.mu.Unlock()
		return state.committed
	}
	return false
}

// markCreateCommitted 落原子提交位（末批 last 已发/纯内联整体发射后由发射侧
// 调用；取消与完成的窄竞态在此收敛——置位后的取消请求一律应答「已完成」）。
func markCreateCommitted(root string) {
	if st, ok := createRunRegistry.Load(createRunKey(root)); ok {
		state := st.(*createRunState)
		state.mu.Lock()
		state.committed = true
		state.mu.Unlock()
	}
}

// armCreateRunCancel 登记在途创建的 runCtx 取消函数（verify 发射循环起始调用；
// 无运行态=直驱路径，无取消面，静默）。登记时取消位已在（置位先于登记的窄窗
//——如 CreateTask RPC 入口即取消）则立即取消，取消位不丢失。
func armCreateRunCancel(root string, cancel context.CancelFunc) {
	st, ok := createRunRegistry.Load(createRunKey(root))
	if !ok {
		return
	}
	state := st.(*createRunState)
	state.mu.Lock()
	state.cancelFn = cancel
	cancelled := state.cancelled && !state.committed
	state.mu.Unlock()
	if cancelled {
		cancel()
	}
}

// disarmCreateRunCancel 摘除取消函数（发射循环收尾调用，防运行态复用误取消）。
func disarmCreateRunCancel(root string) {
	if st, ok := createRunRegistry.Load(createRunKey(root)); ok {
		state := st.(*createRunState)
		state.mu.Lock()
		state.cancelFn = nil
		state.mu.Unlock()
	}
}

// armCreateRunDone 登记在途创建的收尾信号并返回待关闭通道（emitSelectedWorks
// 起始调用、全部退出路径经 defer 关闭）。无运行态=直驱路径，返回 nil（关闭方
// 忽略）。
func armCreateRunDone(root string) chan struct{} {
	st, ok := createRunRegistry.Load(createRunKey(root))
	if !ok {
		return nil
	}
	state := st.(*createRunState)
	done := make(chan struct{})
	state.mu.Lock()
	state.done = done
	state.mu.Unlock()
	return done
}

// closeCreateRunDone 关闭收尾信号（幂等于登记表侧：信号只随本次发射生命周期
// 关闭一次，运行态条目此后由 endCreateRun 摘除）。
func closeCreateRunDone(root string, done chan struct{}) {
	if done == nil {
		return
	}
	close(done)
}

// createHaltEmissionWaitLimit 撤回后等待发射侧落定的时间上限：在途哈希单元按
// 4MB 分块检查取消、通道背压阻塞点即时解除，常态秒级落定；挂起读等不可中断
// 形态超时后照常发终态（发射 goroutine 自行收尾，不阻塞用户可见结局）。
const createHaltEmissionWaitLimit = 10 * time.Second

// haltCreateRunEmission 宿主创建流提前终结（空闲收流/传输中断/error 收流）后
// 撤回插件侧在途核对与发射：调用登记的 runCtx 取消函数（核对 worker 池供件/
// 分块处收流、发射通道背压阻塞点解除），并限时等待发射侧退出——终态事件发布
// 前进度事件停摆、发射进度槽定格（失败文案的已发射计数不再与在途批次竞态）。
// 与 cancelCreate 工作台动作共用取消函数但不置取消位：宿主流终结不是用户取消，
// 终态仍走失败文案；发射侧以取消形态收尾（末批 last 不发，宿主未收批统一回滚）。
func haltCreateRunEmission(root string) {
	st, ok := createRunRegistry.Load(createRunKey(root))
	if !ok {
		return
	}
	state := st.(*createRunState)
	state.mu.Lock()
	cancel, done := state.cancelFn, state.done
	state.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(createHaltEmissionWaitLimit):
	}
}

// noteCreateProgress 推送 createProgress 事件并登记发射进度：create 相（发射
// 循环计数）同时落运行态进度槽（abortCreateTasks 的在途清理量数据源），
// verify 相只推事件；bytesPerSec 为核对相当前窗口读盘字节/秒（甲1 速率字段，
// 爬坡控制器同一窗口统计——风险5 口径：只计真实读盘字节，缓存命中零字节，
// 阶段5 哈希缓存域命中路径不产生读 IO 不计入）。上下文不可用时静默（不影响
// 创建本身）。
func (m *SessionManager) noteCreateProgress(root, phase string, done, total int, bytesPerSec int64) {
	if phase == createPhaseCreate {
		recordCreateEmitProgress(root, done, total)
	}
	m.publishCreateProgress(root, phase, done, total, bytesPerSec)
}

// publishCreateProgress 单发 createProgress 事件（发射相节流发布面：进度槽落账
// 与事件发布解耦——落账逐批恒做，发布经时间基节流、终值恒发，风险7）。
func (m *SessionManager) publishCreateProgress(root, phase string, done, total int, bytesPerSec int64) {
	m.publishCreateEvent(&workbenchFrontendEvent{
		Kind: workbenchEventCreateProgress, Root: root,
		Phase: phase, Done: done, Total: total, BytesPerSec: bytesPerSec,
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
// （选择集解析 → 创建前对账〔仅目录级 mtime 游标——逐文件存在性核对已并入
// verify 哈希循环〕+ 档案指纹与会话赋值修订号对快照基准一致，另置同库根「创建
// 进行中」并发拒绝于链首）→ 置工作台发起标记并暂存任务名原文后立即返回
// started=true（工作台订阅循环不再被长创建阻塞；前端经 createProgress/createDone
// 事件感知进度与终态）→ 创建 RPC（含完整 Create 流消费）在独立 goroutine 执行
//（runCreateTasks）。taskName 为可选自定义任务名（空/全空白=默认名兜底）。任一
// 守卫未过即阻断（零任务零标记零运行态残留）。
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
	sess, err := m.cachedSession(root) // 只读守卫链前置读：共享只读实例
	if err != nil {
		return fail(err)
	}
	if sess.Snapshot == nil {
		return fail(fmt.Errorf("该会话尚未完成全量识别，无法创建任务"))
	}
	if len(sess.Selection) == 0 {
		return fail(fmt.Errorf("未选择任何作品：请在识别全景勾选要导入的作品后再创建"))
	}
	// 选择域求值接线缓存条目（守卫链与会话瘦身四刀之三）：全景载入时已推导过，
	// 此处经 workDomainOf 复用条目内驻留结果——守卫链零重推导（推导 O(全库)）。
	domain := m.workDomainOf(sess)
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
// 契约在此保持）。宿主创建流提前终结（空闲收流/传输中断/error 收流）时 RPC 返回
// 失败：先撤回插件侧在途核对与发射（haltCreateRunEmission——进度事件停摆、发射
// 计数定格）再发终态。终态经 createDone 事件推送：成功=清空选择集（赋值与快照保留）
// +摘要文案；失败=abortCreateTasks（保留选择集+在途清理信息，决策4 统一回滚）；取消（甲3/决策3，
// 判读先于应答形态——取消位与取消意图块的应答形态不保证一致）=finishCancelledCreate
//（保留选择集+清理量文案；原子提交点已过的取消不成立，照常成功收尾，风险10）。
// 运行态由 defer 摘除。
func (m *SessionManager) runCreateTasks(root, taskName string, workCount int) {
	defer endCreateRun(root)
	started := time.Now() // 创建全程计时（含 verify+发射，帧6「用时 X 分钟」文案数据源）
	resp, err := m.ctx.CreateTask("local://" + root)
	// 取消终态优先：取消位置位且未过原子提交点（末批 last 已发的窄竞态下
	// committed=true，创建照常成功——已收批行不回滚，风险10）
	if isCreateCancelled(root) && !createRunCommitted(root) {
		m.finishCancelledCreate(root)
		return
	}
	if err != nil {
		haltCreateRunEmission(root)
		m.abortCreateTasks(root, fmt.Sprintf("宿主创建任务失败: %v", err))
		return
	}
	if resp == nil || !resp.Succeed {
		msg := "宿主未说明原因"
		if resp != nil {
			msg = resp.Msg
		}
		haltCreateRunEmission(root)
		m.abortCreateTasks(root, fmt.Sprintf("宿主创建任务失败: %s", msg))
		return
	}
	// 创建成功清理选择集（轻字段变异走信封级通路，四刀之二）：单键小读→变异
	//→轻保存，不触大载荷 gzip 分片（选择集是信封轻字段；赋值与快照保留）。
	if _, err := m.mutateSessionEnvelope(root, func(sess *WorkbenchSession) error {
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
		Message: buildCreateTasksMessage(taskName, workCount, tasks, time.Since(started), summary),
	})
}

// abortCreateTasks 创建失败收尾（剧本帧5）：回收发起标记与暂存任务名（零残留）
// + createDone 终态事件——保留选择集供重试。发射已开始（进度槽 planned>0）时按
// 决策4 统一回滚语义告知「在途任务已清理」——主失败模式（宿主 60s 空闲窗收流、
// error 收流、流中断）均以未收批回滚收场，已发射子行随父行删除；协议违规类
// host 侧保留行属 localImport 自身不可达形态（悬空引用/收批后再到），不另设分支。
func (m *SessionManager) abortCreateTasks(root, reason string) {
	m.ConsumeWorkbenchCreateMarker(root)
	m.takeStagedCreateTaskName(root)
	emitted, planned := takeCreateEmitProgress(root)
	var message string
	done, total := 0, 0
	if planned > 0 {
		done, total = emitted, planned
		message = fmt.Sprintf("任务创建失败：%s；在途任务已清理（已发射 %d/%d 个子任务随回滚删除），选择集已保留，可重试", reason, emitted, planned)
	} else {
		message = fmt.Sprintf("任务创建失败（选择集已保留，可重试）：%s", reason)
	}
	m.publishCreateEvent(&workbenchFrontendEvent{
		Kind: workbenchEventCreateDone, Root: root, Success: false,
		Done: done, Total: total, Message: message,
	})
}

// finishCancelledCreate 取消终态（甲3/决策3）：显式取消后已落库子行由宿主
// 「未收批统一回滚」清理（阶段1——插件侧提前收流不发末批 last），插件侧只
// 发终态事件：选择集保留（可重新发起）、发起标记与暂存名回收（零残留）、
// 运行态摘除随 runCreateTasks 的 defer（闸门释放）。发射已开始（进度槽
// planned>0）时文案携带清理量（X=发射侧最近一批快照≈宿主已回滚行数）；零发射
// 阶段（verify 中取消）无开批父无行可删、直接终态。
func (m *SessionManager) finishCancelledCreate(root string) {
	m.ConsumeWorkbenchCreateMarker(root)
	m.takeStagedCreateTaskName(root)
	emitted, planned := takeCreateEmitProgress(root)
	var message string
	done, total := 0, 0
	if planned > 0 {
		done, total = emitted, planned
		message = fmt.Sprintf("已取消创建：在途任务已清理（已删 %d 个子任务），选择集已保留，可重新发起", emitted)
	} else {
		message = "已取消创建：尚未发射任何子任务（无在途任务需要清理），选择集已保留，可重新发起"
	}
	m.publishCreateEvent(&workbenchFrontendEvent{
		Kind: workbenchEventCreateDone, Root: root, Success: false,
		Done: done, Total: total, Message: message,
	})
}
