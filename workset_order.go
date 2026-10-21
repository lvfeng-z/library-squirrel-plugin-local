// 集序记录与作答端：批次创建期为有集归属的子行按集归集落 KV 记录（键=集业务键
// 原文，成员值=作品身份键+序号数值与原文+目录键+首见批次号），跨批次读改写合并
// 维护全局总序（目录块按首见拼接、块内序号数值升序、并列按首见稳序）；作品
// 拉取器实现可选能力接口 WorkOrderQuerier，按传入集业务键读记录按总序作答——
// 宿主在作品入库后拉取该全序，写入集成员关系行的原站序。
package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// worksetOrderSchemaVersion 集序记录 JSON 的格式版本（版本字段在记录载荷内；
// 存储层 StorageValue 的 schema_version 戳是插件配置结构版本，两者互不相干）。
const worksetOrderSchemaVersion = 1

// workSetBizKey 集名的业务键形态：workSet:{名}——与作品应答中集声明一致，宿主
// 按（站点, 业务键）upsert 集行。集序记录以该键原文为存储键（键域与集域一致，
// 不带库根与站点维度——同名集在插件侧同一份记录，宿主侧各站点各自成行）。
func workSetBizKey(name string) string {
	return "workSet:" + name
}

// worksetGroupSeq 成员在所属序列群内的序号：数值（比较与排序用）与原文（保留
// 前导零形态，仅快照群卡携带）。
type worksetGroupSeq struct {
	seq     int
	seqText string
}

// buildGroupSeqIndex 从快照构建成员路径 → 集内序号的反查索引，取值口径：快照
// 群卡按成员路径反查为主（群卡覆盖规则群与结构群两类的全部成员，含已识别
// 成员——已识别成员的文件行不落群组键，群归属只在群卡），文件行群组序兜底
// （群卡未覆盖而文件行携带群组键的形态；序号原文只存于群卡，兜底条目原文为
// 空）。集序记录与批次子行排序共用本索引，保证两端集内序取值同口径。同一
// 路径命中多张群卡时取首张（一文件至多一个群组身份，重复仅可能来自异常
// 数据）；群卡条目优先于文件行条目。
func buildGroupSeqIndex(snap *RecognitionSnapshot) map[string]worksetGroupSeq {
	idx := make(map[string]worksetGroupSeq)
	if snap == nil {
		return idx
	}
	for gi := range snap.Groups {
		for _, mem := range snap.Groups[gi].Members {
			if _, ok := idx[mem.RelPath]; !ok {
				idx[mem.RelPath] = worksetGroupSeq{seq: mem.Seq, seqText: mem.SeqText}
			}
		}
	}
	for i := range snap.Files {
		if f := &snap.Files[i]; f.GroupKey != "" && f.GroupSeq > 0 {
			if _, ok := idx[f.RelPath]; !ok {
				idx[f.RelPath] = worksetGroupSeq{seq: f.GroupSeq}
			}
		}
	}
	return idx
}

// collectWorkSetOrderUpdates 从批次子行素材归集各集的本批成员条目（集业务键 →
// 成员列表）。集归属来源两处，与执行期作品应答的集声明同源：快照终选的集维度
// 声明（集名赋值与规则提取产物，全部归属形态适用）与目录含义链的 workSet 项
// （仅 local 归属子行——真实域作品的站点侧集 ID 无法从名字派生，执行期作品
// 应答同样跳过该含义）。成员的集内序号以群卡反查为主、文件行群组序兜底（规则
// 群成员的文件行携带群组序号；序号原文只存于群卡，兜底路径为空）。无群组身份
// （无集内序）的成员不落记录：作答不含该成员，宿主侧其原站序保持空、由本地
// 续排兜底。作品身份键与批次子行同源同值（真实域页级作品 ID 或文件内容哈希）。
func collectWorkSetOrderUpdates(snap *RecognitionSnapshot, items []batchFileItem) map[string][]worksetOrderMember {
	updates := make(map[string][]worksetOrderMember)
	if snap == nil {
		return updates
	}
	fileByPath := make(map[string]*SnapFile, len(snap.Files))
	for i := range snap.Files {
		fileByPath[snap.Files[i].RelPath] = &snap.Files[i]
	}
	groupSeq := buildGroupSeqIndex(snap)
	for _, item := range items {
		rel := item.af.entry.RelPath
		rec := fileByPath[rel]

		// 集归属归集：同文件多来源同名集只落一条
		var setNames []string
		seenSet := make(map[string]bool)
		if rec != nil {
			for _, decl := range rec.Final {
				if decl.Dimension == string(DimCollection) && decl.Value != "" && !seenSet[decl.Value] {
					seenSet[decl.Value] = true
					setNames = append(setNames, decl.Value)
				}
			}
		}
		if !item.af.attr.isReal() {
			for _, m := range item.metadata {
				if m.Type == string(PathTypeWorkSet) && m.Name != "" && !seenSet[m.Name] {
					seenSet[m.Name] = true
					setNames = append(setNames, m.Name)
				}
			}
		}
		if len(setNames) == 0 {
			continue
		}

		// 集内序号裁决：buildGroupSeqIndex 统一口径（群卡反查为主、文件行群组序
		// 兜底），与批次子行排序同源
		gs, hasSeq := groupSeq[rel]
		if !hasSeq {
			continue
		}
		seq, seqText := gs.seq, gs.seqText

		workID := item.af.entry.Hash
		if item.af.attr.isReal() {
			workID = item.af.attr.PageWorkID
		}
		dirKey := dirRelKeyOf(rel)
		if rec != nil {
			dirKey = rec.DirRelKey
		}
		for _, name := range setNames {
			key := workSetBizKey(name)
			updates[key] = append(updates[key], worksetOrderMember{
				SiteWorkId: workID, Seq: seq, SeqText: seqText, DirKey: dirKey,
			})
		}
	}
	return updates
}

// worksetOrderMember 集序记录的单个成员条目：SiteWorkId 为作品身份键（与批次
// 子行同源同值——真实域页级作品 ID 或文件内容哈希）；Seq/SeqText 为集内序号的
// 数值与原文；DirKey 为成员所在目录键（库根内相对键——跨库根同相对路径并入
// 同一目录块，语义一致降级）；Batch 为首见批次号（批次创建时刻的毫秒时间戳，
// 观测与追溯用，不参与排序比较）；Ord 为记录内单调递增的插入序号，承载「并列
// 按首见稳序」的比较末键。
type worksetOrderMember struct {
	SiteWorkId string `json:"siteWorkId"`
	Seq        int    `json:"seq"`
	SeqText    string `json:"seqText,omitempty"`
	DirKey     string `json:"dirKey"`
	Batch      int64  `json:"batch"`
	Ord        int    `json:"ord"`
}

// worksetOrderRecord 单个集的集序记录（按集业务键一条）：DirBlocks 为目录块的
// 首见序（跨目录同名集按目录块拼接、不做跨块序号数值交错——各目录序号各自
// 起算，数值交错会产乱序）；Members 按插入序存放（作答时按比较键重排）；
// NextOrd 为插入序号分配器。
type worksetOrderRecord struct {
	SchemaVersion int                  `json:"schemaVersion"`
	DirBlocks     []string             `json:"dirBlocks,omitempty"`
	NextOrd       int                  `json:"nextOrd"`
	Members       []worksetOrderMember `json:"members,omitempty"`
}

// worksetOrderStore 集序记录读写所需的最小存储方法集；签名与 SDK 的 dto.KVStore
// 同名方法一致，按需收窄依赖。
type worksetOrderStore interface {
	GetValue(key string) (*sdkdto.StorageValue, error)
	SetValue(key string, value string) error
}

// worksetOrderMu 集序记录读改写串行锁：同集多批次合并（读既有记录 → 并入本批
// 成员 → 覆写回）在插件内互斥，防并发批次交错覆写丢成员。
var worksetOrderMu sync.Mutex

// recordWorkSetOrders 批次集序记录写入入口：归集本批有集归属子行的成员条目，
// 逐集读改写合并落库（批次号=批次创建时刻毫秒时间戳，作成员首见批次号）。
// 上下文不可用（无法持久化）或本批无集归属成员时零动作；写入失败仅告警不阻断
// 批次产出——记录缺席时作答为空，集原站序维持现状。
func (h *LocalImportWorkFetcher) recordWorkSetOrders(snap *RecognitionSnapshot, items []batchFileItem, batch int64) {
	if h.ctx == nil {
		return
	}
	updates := collectWorkSetOrderUpdates(snap, items)
	if len(updates) == 0 {
		return
	}
	store := worksetOrderStore(h.ctx)
	worksetOrderMu.Lock()
	defer worksetOrderMu.Unlock()
	for _, setKey := range sortedKeys(updates) {
		rec, writable := loadWorkSetOrderRecord(store, setKey)
		if !writable {
			continue
		}
		if rec == nil {
			rec = &worksetOrderRecord{SchemaVersion: worksetOrderSchemaVersion}
		}
		mergeWorkSetOrderMembers(rec, updates[setKey], batch)
		if err := saveWorkSetOrderRecord(store, setKey, rec); err != nil {
			h.ctx.Warnf("写入集序记录失败（键 %q）: %v", setKey, err)
		}
	}
}

// loadWorkSetOrderRecord 读取一个集的集序记录：键缺席、空值或载荷损坏返回
// （nil, true）——按空记录重起（记录自愈，作答最多缺失旧成员）；载荷版本高于
// 当前支持返回（nil, false）——由更高版本插件写入，不识别不覆写、保留原值。
func loadWorkSetOrderRecord(store worksetOrderStore, key string) (*worksetOrderRecord, bool) {
	sv, err := store.GetValue(key)
	if err != nil || sv == nil || sv.Value == "" {
		return nil, true
	}
	var rec worksetOrderRecord
	if err := json.Unmarshal([]byte(sv.Value), &rec); err != nil {
		return nil, true
	}
	if rec.SchemaVersion > worksetOrderSchemaVersion {
		return nil, false
	}
	return &rec, true
}

// saveWorkSetOrderRecord 序列化覆写一个集的集序记录。
func saveWorkSetOrderRecord(store worksetOrderStore, key string, rec *worksetOrderRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("序列化集序记录失败: %w", err)
	}
	return store.SetValue(key, string(data))
}

// mergeWorkSetOrderMembers 一批新成员并入集序记录（读改写合并的纯函数侧）：
// 已记录的作品身份键=重复导入，保留首见条目不重复追加（首见批次号、序号与
// 目录块序均不被后续批次扰动）；新目录键按本批出现序接续进目录块首见序；插入
// 序号跨批单调递增。本批成员须按批次产出序传入（同批内的首见先后即产出先后）。
func mergeWorkSetOrderMembers(rec *worksetOrderRecord, incoming []worksetOrderMember, batch int64) {
	seenWork := make(map[string]bool, len(rec.Members))
	for _, m := range rec.Members {
		seenWork[m.SiteWorkId] = true
	}
	seenDir := make(map[string]bool, len(rec.DirBlocks))
	for _, dk := range rec.DirBlocks {
		seenDir[dk] = true
	}
	for _, m := range incoming {
		if seenWork[m.SiteWorkId] {
			continue
		}
		seenWork[m.SiteWorkId] = true
		if !seenDir[m.DirKey] {
			seenDir[m.DirKey] = true
			rec.DirBlocks = append(rec.DirBlocks, m.DirKey)
		}
		m.Batch = batch
		m.Ord = rec.NextOrd
		rec.NextOrd++
		rec.Members = append(rec.Members, m)
	}
}

// 编译期锚定：作品拉取器实现可选能力接口——SDK 服务端按类型断言发现本实现并
// 转发 RPC；未实现该接口的拉取器同名 RPC 返回空响应。
var _ sdkdto.WorkOrderQuerier = (*LocalImportWorkFetcher)(nil)

// QueryWorkSetOrder 集内作品原站全序作答：按传入集业务键读集序记录，成员按
// 全局总序（目录块首见序 → 块内序号数值升序 → 并列按首见插入序）排列作答，
// 条目序号=全序位次（集内序从 1 起、RPC 序号从 0 起，恰差一）。记录缺席或无
// 成员返回空切片——宿主保持该集原站序现状不动。siteId 不参与寻址：集序记录
// 按集业务键全局一份，同名集在宿主按「站点+业务键」各自成行、各自拉取，作答
// 内容由宿主按已入库成员映射裁剪。
func (h *LocalImportWorkFetcher) QueryWorkSetOrder(siteId int64, siteWorkSetId string) ([]*sdkdto.WorkOrderEntry, error) {
	if h.ctx == nil || siteWorkSetId == "" {
		return nil, nil
	}
	rec, _ := loadWorkSetOrderRecord(worksetOrderStore(h.ctx), siteWorkSetId)
	if rec == nil {
		return nil, nil
	}
	return answerWorkSetOrderEntries(rec), nil
}

// answerWorkSetOrderEntries 集序记录 → 全序作答条目（记录内成员存放序不动）。
func answerWorkSetOrderEntries(rec *worksetOrderRecord) []*sdkdto.WorkOrderEntry {
	ordered := append([]worksetOrderMember(nil), rec.Members...)
	blockOf := make(map[string]int, len(rec.DirBlocks))
	for i, dk := range rec.DirBlocks {
		blockOf[dk] = i
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		bi, bj := blockOf[ordered[i].DirKey], blockOf[ordered[j].DirKey]
		if bi != bj {
			return bi < bj
		}
		if ordered[i].Seq != ordered[j].Seq {
			return ordered[i].Seq < ordered[j].Seq
		}
		return ordered[i].Ord < ordered[j].Ord
	})
	entries := make([]*sdkdto.WorkOrderEntry, 0, len(ordered))
	for rank, m := range ordered {
		entries = append(entries, &sdkdto.WorkOrderEntry{SiteWorkId: m.SiteWorkId, SortOrder: int64(rank)})
	}
	return entries
}
