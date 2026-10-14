// 全量识别引擎：识别干跑（零任务产出、零弹问的纯求值）的合并求值单一实现、
// 库内谓词索引构造、七类识别全景元素聚合与快照裁剪模型、增量游标编排。
package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// fileNameOfRel 相对路径尾段即文件名（'/' 路径不含目录时整体即文件名）。
func fileNameOfRel(relPath string) string {
	if idx := strings.LastIndexByte(relPath, '/'); idx >= 0 {
		return relPath[idx+1:]
	}
	return relPath
}

// —— 会话赋值（特异性手工解析知识）——

// SessionAssignment 单文件的手工解析赋值（会话态知识，relPath 寻址、按库根持久）：
// 非空维度在合成结论上覆盖档案求值的同维度终选声明，未指定维度保留档案结论。
// 赋值创建的交互属后续阶段，本文件承载其数据模型与求值消费。
type SessionAssignment struct {
	// Author 指定的作者名（覆盖 author 维度）。
	Author string `json:"author,omitempty"`
	// AuthorRole 指定的作者角色（随 Author 一起声明，如「原画」）。
	AuthorRole string `json:"authorRole,omitempty"`
	// Title 指定的作品名（覆盖 title 维度）。
	Title string `json:"title,omitempty"`
	// Tags 指定的标签名集合（覆盖 tag 维度）。
	Tags []string `json:"tags,omitempty"`
	// Collection 指定的作品集名（覆盖 collection 维度，即集归属）。
	Collection string `json:"collection,omitempty"`
	// Generalized 该赋值是否已泛化为档案规则（全景呈现标记：已泛化的赋值其
	// 档案规则亦会产出同值声明，保留标记供清理提示）。
	Generalized bool `json:"generalized,omitempty"`
	// UpdatedAtMs 最近更新时间（Unix 毫秒）。
	UpdatedAtMs int64 `json:"updatedAtMs,omitempty"`
}

// coveredDimensions 赋值显式指定的含义维度集合（非空字段对应的维度）。
func (a *SessionAssignment) coveredDimensions() map[MetadataDimension]bool {
	dims := make(map[MetadataDimension]bool, 4)
	if a == nil {
		return dims
	}
	if a.Author != "" {
		dims[DimAuthor] = true
	}
	if a.Title != "" {
		dims[DimTitle] = true
	}
	if len(a.Tags) > 0 {
		dims[DimTag] = true
	}
	if a.Collection != "" {
		dims[DimCollection] = true
	}
	return dims
}

// assignmentDeclarations 赋值展开为终选声明序列（来源标记 assignment；仅含
// 显式指定维度）。
func (a *SessionAssignment) assignmentDeclarations() []MetadataDeclaration {
	if a == nil {
		return nil
	}
	var decls []MetadataDeclaration
	if a.Author != "" {
		decls = append(decls, MetadataDeclaration{
			Dimension: DimAuthor, Value: a.Author, Source: SourceAssignment, RoleName: a.AuthorRole,
		})
	}
	if a.Title != "" {
		decls = append(decls, MetadataDeclaration{
			Dimension: DimTitle, Value: a.Title, Source: SourceAssignment,
		})
	}
	for _, tag := range a.Tags {
		decls = append(decls, MetadataDeclaration{
			Dimension: DimTag, Value: tag, Source: SourceAssignment,
		})
	}
	if a.Collection != "" {
		decls = append(decls, MetadataDeclaration{
			Dimension: DimCollection, Value: a.Collection, Source: SourceAssignment,
		})
	}
	return decls
}

// —— 「档案规则 ∪ 会话赋值」合并求值（单一实现）——

// FileConclusion 单文件的合并求值结论：流水线结论（分组归属+命中规则+原始声明）
// 加合成产物（终选声明+待裁决项）与赋值覆盖结果，以及定群规则与序号原文
//（供识别全景的序列形态呈现）。
type FileConclusion struct {
	// RelPath / FileName 条目寻址；DirRelKey 为所属目录键（相对库根的 '/' 目录
	// 路径，根直下为空串）。
	RelPath   string
	FileName  string
	DirRelKey string
	// Group 为兄弟群组定群产物（群组成员非 nil，落单为 nil）；GroupRuleID 为
	// 定群规则 ID；GroupSeqText 为本成员序号的原文（前导零体现零填充形态）。
	Group        *GroupAnalysis
	GroupRuleID  string
	GroupSeqText string
	// MatchedRuleIDs 为提取段命中的规则 ID（与流水线结论一致）。
	MatchedRuleIDs []string
	// Final 为合成终选声明（会话赋值的指定维度已覆盖档案结论）。
	Final []MetadataDeclaration
	// Pending 为合成待裁决项（赋值指定维度的冲突随覆盖消解）。
	Pending []PendingItem
}

// MergedRecognition 对一批扫描条目执行「档案规则 ∪ 会话赋值」合并求值：
// 先按规则流水线求值（注入库内谓词索引；扫描条目构造与 Create 期 buildScanFiles
// 同构），再逐文件合成终选与待裁决，最后会话赋值按维度覆盖终选声明（特异性
// 结果压过普适规则）。识别干跑与会话模式的任务创建共用本入口（合并逻辑唯一
// 实现，两处结论同源）。
func MergedRecognition(files []ScanFile, rules []RuleEntry, lib *LibraryIndex, assigns map[string]SessionAssignment) []FileConclusion {
	decisions := RunPipeline(files, rules, lib)
	policy := AggregateSynthesis(rules)
	// 定群规则与序号原文重放需要同目录兄弟全集——与 RunPipeline 内部同键同序构造
	siblingsByDir := make(map[string][]string, len(files))
	for _, file := range files {
		key := dirGroupKey(file.DirNames)
		siblingsByDir[key] = append(siblingsByDir[key], file.FileName)
	}
	conclusions := make([]FileConclusion, 0, len(decisions))
	for i, d := range decisions {
		final, pending := Synthesize(d.Declarations, policy)
		if assign, ok := assigns[d.RelPath]; ok {
			final, pending = applyAssignmentToConclusion(final, pending, &assign)
		}
		c := FileConclusion{
			RelPath: d.RelPath, FileName: d.FileName, DirRelKey: dirRelKeyOf(d.RelPath),
			Group: d.Group, MatchedRuleIDs: d.MatchedRuleIDs,
			Final: final, Pending: pending,
		}
		if d.Group != nil {
			in := &MatchInput{
				FileName: d.FileName, DirNames: files[i].DirNames, RelPath: d.RelPath,
				Siblings: siblingsByDir[dirGroupKey(files[i].DirNames)],
			}
			ctx := &ParseContext{DirNames: files[i].DirNames, Depth: len(files[i].DirNames)}
			if group, ruleID := runGroupStage(rules, in, lib, ctx); group != nil {
				c.Group, c.GroupRuleID = group, ruleID
				if sibling, ok := ruleByID(rules, ruleID).(*SiblingPattern); ok {
					if text, has := sibling.seqTextOf(d.FileName); has {
						c.GroupSeqText = text
					}
				}
			}
		}
		conclusions = append(conclusions, c)
	}
	return conclusions
}

// ruleByID 在规则序列中按 ID 取顶层模式（未命中返回 nil）。
func ruleByID(rules []RuleEntry, id string) Pattern {
	for _, rule := range rules {
		if rule.ID == id {
			return rule.Pattern
		}
	}
	return nil
}

// applyAssignmentToConclusion 会话赋值按维度覆盖合成结论：赋值显式指定维度的
// 档案终选声明与待裁决项全部清场，替换为赋值声明（用户手工指定即终选，不再
// 进入待裁决）；未指定维度原样保留。
func applyAssignmentToConclusion(final []MetadataDeclaration, pending []PendingItem, assign *SessionAssignment) ([]MetadataDeclaration, []PendingItem) {
	covered := assign.coveredDimensions()
	kept := make([]MetadataDeclaration, 0, len(final))
	for _, decl := range final {
		if covered[decl.Dimension] {
			continue
		}
		kept = append(kept, decl)
	}
	kept = append(kept, assign.assignmentDeclarations()...)
	keptPending := make([]PendingItem, 0, len(pending))
	for _, item := range pending {
		if covered[item.Dimension] {
			continue
		}
		keptPending = append(keptPending, item)
	}
	return kept, keptPending
}

// —— 库内谓词索引构造 ——

// libraryIndexQuerySource 谓词索引构造所需的库查询方法集（PluginContext 的收窄）。
type libraryIndexQuerySource interface {
	QueryLocalAuthors(req *sdkdto.QueryLocalAuthorsRequest) (*sdkdto.QueryLocalAuthorsResponse, error)
	QuerySiteAuthors(req *sdkdto.QuerySiteAuthorsRequest) (*sdkdto.QuerySiteAuthorsResponse, error)
	QueryLocalTags(req *sdkdto.QueryLocalTagsRequest) (*sdkdto.QueryLocalTagsResponse, error)
	QuerySiteTags(req *sdkdto.QuerySiteTagsRequest) (*sdkdto.QuerySiteTagsResponse, error)
}

// libraryIndexPageSize 库查询分页大小（宿主数据库单连接与 UI 共享，取插件开发
// 指南的上限指导值）。
const libraryIndexPageSize = 200

// BuildLibraryIndex 经只读库查询构造谓词索引：本地/站点两轨的作者名与标签名全集
//（分页拉全）。任一轨查询失败时该轨按空集继续并告警（谓词规则对空集恒不命中，
// 不阻断识别）；查询源不可用返回 nil 索引。
func BuildLibraryIndex(q libraryIndexQuerySource, warnf func(format string, args ...any)) *LibraryIndex {
	if q == nil {
		return nil
	}
	lib := &LibraryIndex{
		AuthorNames: make(map[string]struct{}),
		TagNames:    make(map[string]struct{}),
	}
	addNames := func(names []string, into map[string]struct{}) {
		for _, name := range names {
			if name != "" {
				into[name] = struct{}{}
			}
		}
	}
	// 每轨分页拉全：按页推进到本页不满为止（页大小固定，不满即到尾页）
	pull := func(label string, fetch func(page int32) ([]string, error), into map[string]struct{}) {
		for page := int32(1); ; page++ {
			names, err := fetch(page)
			if err != nil {
				if warnf != nil {
					warnf("构造谓词索引时查询%s失败，该轨按空集处理（谓词规则对该轨恒不命中）: %v", label, err)
				}
				return
			}
			addNames(names, into)
			if len(names) < libraryIndexPageSize {
				return
			}
		}
	}
	pull("本地作者", func(page int32) ([]string, error) {
		resp, err := q.QueryLocalAuthors(&sdkdto.QueryLocalAuthorsRequest{
			Page: &sdkdto.PageRequest{Page: page, PageSize: libraryIndexPageSize},
		})
		if err != nil {
			return nil, err
		}
		return authorNamesOf(resp.Items), nil
	}, lib.AuthorNames)
	pull("站点作者", func(page int32) ([]string, error) {
		resp, err := q.QuerySiteAuthors(&sdkdto.QuerySiteAuthorsRequest{
			Page: &sdkdto.PageRequest{Page: page, PageSize: libraryIndexPageSize},
		})
		if err != nil {
			return nil, err
		}
		return siteAuthorNamesOf(resp.Items), nil
	}, lib.AuthorNames)
	pull("本地标签", func(page int32) ([]string, error) {
		resp, err := q.QueryLocalTags(&sdkdto.QueryLocalTagsRequest{
			Page: &sdkdto.PageRequest{Page: page, PageSize: libraryIndexPageSize},
		})
		if err != nil {
			return nil, err
		}
		return tagNamesOf(resp.Items), nil
	}, lib.TagNames)
	pull("站点标签", func(page int32) ([]string, error) {
		resp, err := q.QuerySiteTags(&sdkdto.QuerySiteTagsRequest{
			Page: &sdkdto.PageRequest{Page: page, PageSize: libraryIndexPageSize},
		})
		if err != nil {
			return nil, err
		}
		return siteTagNamesOf(resp.Items), nil
	}, lib.TagNames)
	return lib
}

// authorNamesOf / tagNamesOf 等四函数把查询条目投影为名字集（空名剔除）。
func authorNamesOf(items []*sdkdto.LocalAuthorDTO) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name := item.GetAuthorName(); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func siteAuthorNamesOf(items []*sdkdto.SiteAuthorInfo) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil && item.AuthorName != "" {
			names = append(names, item.AuthorName)
		}
	}
	return names
}

func tagNamesOf(items []*sdkdto.LocalTagDTO) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name := item.GetLocalTagName(); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func siteTagNamesOf(items []*sdkdto.SiteTagInfo) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item != nil && item.TagName != "" {
			names = append(names, item.TagName)
		}
	}
	return names
}

// —— 识别全景快照（会话 KV 裁剪形态）——

// 识别全景文件记录的呈现状态类别。
const (
	// snapStateIdentified 已识别：合成终选产出至少一条元数据声明。
	snapStateIdentified = "identified"
	// snapStateClueful 有价值未识别：无终选声明但探测到疑似线索。
	snapStateClueful = "clueful"
	// snapStatePlain 未识别且无已知线索。
	snapStatePlain = "plain"
)

// 线索类别（元素3 的疑似线索，全部由既有探测器产出）。
const (
	cluePredicateAuthor = "predicate-author" // 文件名或群组前缀命中库内作者名（谓词原子）
	cluePredicateTag    = "predicate-tag"    // 文件名或群组前缀命中库内标签名（谓词原子）
	cluePixivID         = "pixiv-id"         // pixiv 页级作品 ID 形态（内置惯例探测器）
	clueGroupPrefix     = "group-prefix"     // 群组成员无元数据声明：公共前缀为集名候选
)

// recognitionSnapshotSchemaVersion 快照结构版本（自管）。
const recognitionSnapshotSchemaVersion = 1

// RecognitionSnapshot 识别全景快照：识别时点的全景结论缓存（可按「原始路径+档案」
// 重算），以裁剪形态入会话 KV——只存呈现与增量复用所需字段：不含文件完整路径
//（可由库根+相对路径重建）、不含内容哈希（干跑不读内容）、不含同目录兄弟全集
//（可由快照文件清单重建）、不含提取段原始中间声明（终选与待裁决候选已足呈现）。
type RecognitionSnapshot struct {
	// SchemaVersion 快照结构版本（自管）。
	SchemaVersion int `json:"schemaVersion"`
	// GeneratedAtMs 快照生成时间（Unix 毫秒）。
	GeneratedAtMs int64 `json:"generatedAtMs"`
	// RootPath 库根原始输入。
	RootPath string `json:"rootPath"`
	// ProfileDigest 生成本快照所依据的规则档案指纹（重识别的增量资格判定）。
	ProfileDigest string `json:"profileDigest"`

	// Dirs 目录结构（元素1）：层含义覆盖状态随目录树节点呈现。
	Dirs []SnapDir `json:"dirs"`
	// Files 文件级记录（元素2/3 与元素7 赋值标记）。
	Files []SnapFile `json:"files"`
	// Groups 序列群（元素4）：公共前缀、组大小与成员序号形态。
	Groups []SnapGroup `json:"groups"`
	// Pending 待裁决明细（元素5）：冲突维度与候选声明对照。
	Pending []SnapPending `json:"pending,omitempty"`
	// RuleStats 规则档案态（元素6）：全部规则（含禁用）的来源、启用态与命中数。
	RuleStats []SnapRuleStat `json:"ruleStats"`
	// StaleAssignments 死路径赋值（识别时点判定）：会话赋值引用的相对路径已不存在。
	StaleAssignments []string `json:"staleAssignments,omitempty"`
	// DirMtimes 增量游标：目录键 → mtime（UnixNano），重识别时未变目录跳过求值。
	DirMtimes map[string]int64 `json:"dirMtimes"`
	// Stats 全库汇总统计。
	Stats SnapStats `json:"stats"`
}

// SnapDir 目录树节点（元素1）。
type SnapDir struct {
	// RelKey 相对库根的 '/' 目录路径（根为空串）；Name 目录名（根为库根基名）。
	RelKey string `json:"relKey"`
	Name   string `json:"name"`
	// Depth 层深（根为 0，与目录问答层深编号一致）。
	Depth int `json:"depth"`
	// FileCount 直接子文件数。
	FileCount int `json:"fileCount"`
	// Meanings 该层目录含义的规则回放结论（nil = 规则未覆盖该层）；MeaningRuleID
	// 为产出含义的规则（首个命中）。
	Meanings      []PathMeaning `json:"meanings,omitempty"`
	MeaningRuleID string        `json:"meaningRuleId,omitempty"`
}

// SnapFile 文件级记录。
type SnapFile struct {
	// RelPath 相对库根的 '/' 路径；FileName 文件名（含扩展名）；DirRelKey 所属目录键。
	RelPath   string `json:"relPath"`
	FileName  string `json:"fileName"`
	DirRelKey string `json:"dirRelKey"`
	// State 呈现状态：identified / clueful / plain。
	State string `json:"state"`
	// Final 合成终选声明（含 role）。
	Final []SnapDecl `json:"final,omitempty"`
	// PendingDims 待裁决维度（明细在快照 Pending 清单）。
	PendingDims []string `json:"pendingDims,omitempty"`
	// MatchedRuleIDs 命中规则 ID（元素6 命中统计与元素2 呈现用）。
	MatchedRuleIDs []string `json:"matchedRuleIds,omitempty"`
	// GroupKey 所属序列群的分组键（群组成员非空）；GroupSeq 集内序（从 1 起）。
	GroupKey string `json:"groupKey,omitempty"`
	GroupSeq int    `json:"groupSeq,omitempty"`
	// Clues 疑似线索（元素3）：类别 + 呈现摘要。
	Clues []SnapClue `json:"clues,omitempty"`
	// Assigned 本文件存在会话赋值（元素7 标记；赋值明细为会话活数据不入快照）。
	Assigned bool `json:"assigned,omitempty"`
}

// SnapDecl 声明的紧凑形态（终选与待裁决候选共用）。
type SnapDecl struct {
	Dimension string `json:"dimension"`
	Value     string `json:"value"`
	Source    string `json:"source"`
	RoleName  string `json:"roleName,omitempty"`
	RuleID    string `json:"ruleId,omitempty"`
}

// SnapClue 疑似线索：类别 + 摘要（如命中的库内名字、pixiv ID、群组前缀）。
type SnapClue struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// SnapGroup 序列群（元素4）：同目录由兄弟群组规则定群的成员集合。
type SnapGroup struct {
	// DirRelKey 群组所在目录键；Key 分组键（=公共前缀，集名候选）；Size 组大小。
	DirRelKey string `json:"dirRelKey"`
	Key       string `json:"key"`
	Size      int    `json:"size"`
	// RuleID 定群规则。
	RuleID string `json:"ruleId"`
	// Members 成员（集内序号与原文——原文前导零体现零填充形态）。
	Members []SnapGroupMember `json:"members"`
}

// SnapGroupMember 序列群成员。
type SnapGroupMember struct {
	RelPath string `json:"relPath"`
	Seq     int    `json:"seq"`
	SeqText string `json:"seqText"`
}

// SnapPending 待裁决明细（元素5）：合成层声明冲突的维度、候选对照与原因。
type SnapPending struct {
	RelPath    string     `json:"relPath"`
	FileName   string     `json:"fileName"`
	Dimension  string     `json:"dimension"`
	Reason     string     `json:"reason"`
	Candidates []SnapDecl `json:"candidates"`
}

// SnapRuleStat 规则档案态条目（元素6）。
type SnapRuleStat struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Enabled     bool   `json:"enabled"`
	// HitCount 命中数：提取段按文件计（记入 MatchedRuleIDs 的文件数），分组段
	// 按定群成员文件数计。
	HitCount int `json:"hitCount"`
}

// SnapStats 全库汇总统计。
type SnapStats struct {
	// TotalFiles 全部常规文件；Identified / Clueful / Plain 按呈现状态计数。
	TotalFiles int `json:"totalFiles"`
	Identified int `json:"identified"`
	Clueful    int `json:"clueful"`
	Plain      int `json:"plain"`
	// GroupCount 序列群数；PendingFiles 含待裁决项的文件数。
	GroupCount   int `json:"groupCount"`
	PendingFiles int `json:"pendingFiles"`
	// SkippedDirs 本次识别按 mtime 游标跳过求值的目录数（增量复用）。
	SkippedDirs int `json:"skippedDirs"`
	// StaleAssignments 死路径赋值数。
	StaleAssignments int `json:"staleAssignments"`
}

// —— 七类元素聚合 ——

// aggregateInput 七类元素聚合的输入。
type aggregateInput struct {
	// root 库根；enumRes 轻量枚举产物（目录树与全量文件清单）。
	root    string
	enumRes *EnumerateResult
	// enabledRules 参与求值的规则序列（档案启用 ∪ 内置）；allRules 全部规则
	//（含禁用，元素6 呈现）。
	enabledRules []RuleEntry
	allRules     []RuleEntry
	// lib 谓词索引（线索探测用，与求值注入同源）。
	lib *LibraryIndex
	// conclusions 变更目录文件的合并求值结论。
	conclusions []FileConclusion
	// prev 上次快照（增量复用未变目录的文件记录与序列群）；prevSkipped 本次
	// 按游标跳过且无赋值文件、直接复用旧结论的目录键集合。
	prev        *RecognitionSnapshot
	prevSkipped map[string]bool
	// assigns 会话赋值（死路径判定与元素7 标记）。
	assigns map[string]SessionAssignment
	// profileDigest 本轮档案指纹。
	profileDigest string
}

// aggregatePanorama 把合并求值结论聚合为识别全景快照（七类元素）：
//
//	1 目录结构：目录树节点 + 各层含义的规则回放（未覆盖呈现空）；
//	2 已识别资源：终选声明（含 role）+ 分组归属（群组键/集内序/落单）；
//	3 有价值未识别：谓词原子（文件名与群组前缀对库内名字）、pixiv ID 形态、
//	  群组前缀（集名候选）三类线索；
//	4 序列：兄弟群的公共前缀/组大小/成员序号形态（零填充由原文前导零呈现）；
//	5 待裁决项：合成层声明冲突的维度与候选对照；
//	6 规则档案态：全部规则的来源/启用态/命中数；
//	7 会话决策态：赋值标记入文件记录（计数为会话活数据，经查询响应另呈）。
func aggregatePanorama(in aggregateInput) *RecognitionSnapshot {
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: time.Now().UnixMilli(),
		RootPath:      in.root,
		ProfileDigest: in.profileDigest,
		DirMtimes:     in.enumRes.DirMtimes,
		// 序列化契约：快照经 JSON 落会话 KV 并直接驱动前端渲染，切片字段
		// 一律非 nil（nil 序列化为 null，前端 .length 即崩）；Pending 与
		// StaleAssignments 带 omitempty（缺席由前端守卫兜底）。
		Dirs:      make([]SnapDir, 0, len(in.enumRes.Dirs)),
		Files:     make([]SnapFile, 0, len(in.enumRes.AllFiles)),
		Groups:    make([]SnapGroup, 0),
		RuleStats: make([]SnapRuleStat, 0, len(in.enabledRules)),
	}

	// —— 元素1：目录树与层含义（根层深 0 无规则索引，不参与回放）——
	for _, dir := range in.enumRes.Dirs {
		node := SnapDir{RelKey: dir.RelKey, Name: dir.Name, Depth: dir.Depth, FileCount: dir.FileCount}
		if dir.Depth > 0 {
			node.Meanings = dirMeaningsFromRules(in.enabledRules, dir.Depth, dir.Name)
			node.MeaningRuleID = firstMeaningRuleID(in.enabledRules, dir.Depth, dir.Name)
		}
		snap.Dirs = append(snap.Dirs, node)
	}

	// —— 文件级记录：重求值结论生成；跳过目录整目录复用旧记录 ——
	prevFilesByDir := make(map[string][]SnapFile)
	if in.prev != nil {
		for _, f := range in.prev.Files {
			prevFilesByDir[f.DirRelKey] = append(prevFilesByDir[f.DirRelKey], f)
		}
	}
	prevFileByPath := make(map[string]SnapFile, len(prevFilesByDir))
	for dirKey, files := range prevFilesByDir {
		if !in.prevSkipped[dirKey] {
			continue
		}
		for _, f := range files {
			prevFileByPath[f.RelPath] = f
		}
	}
	// 旧待裁决明细按文件预分组：复用文件的明细整组拷贝
	prevPendingByPath := make(map[string][]SnapPending)
	if in.prev != nil {
		for _, p := range in.prev.Pending {
			prevPendingByPath[p.RelPath] = append(prevPendingByPath[p.RelPath], p)
		}
	}
	conclusionByRel := make(map[string]*FileConclusion, len(in.conclusions))
	for i := range in.conclusions {
		conclusionByRel[in.conclusions[i].RelPath] = &in.conclusions[i]
	}
	allRelPaths := make(map[string]bool, len(in.enumRes.AllFiles))
	for _, f := range in.enumRes.AllFiles {
		allRelPaths[f.RelPath] = true
	}

	groups := make(map[string]*SnapGroup)
	reusedGroups := make(map[string]bool)

	appendConclusion := func(c *FileConclusion) {
		file := snapFileOf(c, in.lib, in.assigns)
		snap.Files = append(snap.Files, file)
		if len(c.Pending) > 0 {
			for _, item := range c.Pending {
				snap.Pending = append(snap.Pending, snapPendingOf(c, item))
			}
		}
		if c.Group != nil {
			gkey := c.DirRelKey + "\x00" + c.Group.Key
			group := groups[gkey]
			if group == nil {
				group = &SnapGroup{DirRelKey: c.DirRelKey, Key: c.Group.Key, Size: c.Group.Size, RuleID: c.GroupRuleID}
				groups[gkey] = group
			}
			group.Members = append(group.Members, SnapGroupMember{
				RelPath: c.RelPath, Seq: c.Group.Seq, SeqText: c.GroupSeqText,
			})
		}
	}

	for _, f := range in.enumRes.AllFiles {
		if c, ok := conclusionByRel[f.RelPath]; ok {
			appendConclusion(c)
			continue
		}
		// 未重求值的文件必属跳过复用目录：整记录复用（含待裁决与命中规则计数）
		prevFile, ok := prevFileByPath[f.RelPath]
		if !ok {
			// 旧快照无此记录（游标异常时的兜底）：按未识别无线索落位，不中断
			prevFile = SnapFile{RelPath: f.RelPath, FileName: fileNameOfRel(f.RelPath), DirRelKey: dirRelKeyOf(f.RelPath), State: snapStatePlain}
		}
		snap.Pending = append(snap.Pending, prevPendingByPath[prevFile.RelPath]...)
		snap.Files = append(snap.Files, prevFile)
		if prevFile.GroupKey != "" && in.prev != nil {
			gkey := prevFile.DirRelKey + "\x00" + prevFile.GroupKey
			if !reusedGroups[gkey] {
				reusedGroups[gkey] = true
				for i := range in.prev.Groups {
					g := in.prev.Groups[i]
					if g.DirRelKey == prevFile.DirRelKey && g.Key == prevFile.GroupKey {
						groups[gkey] = &g
						break
					}
				}
			}
		}
	}

	// —— 元素4 结构性兄弟群（规则无关）：同目录「公共前缀+分隔符+尾缀序号」
	// 形态且未被规则定群的文件聚为结构群（≥2 成员，序号原文保留零填充形态）；
	// 无终选声明的成员记「群组前缀=集名候选」线索（元素3）并升格有线索态。——
	detectStructuralGroups(snap)

	// 序列群按集内序排列成员，群间按目录与分组键排序（呈现稳定序）
	for _, group := range groups {
		sort.SliceStable(group.Members, func(i, j int) bool {
			return group.Members[i].Seq < group.Members[j].Seq
		})
		snap.Groups = append(snap.Groups, *group)
	}
	sort.Slice(snap.Groups, func(i, j int) bool {
		if snap.Groups[i].DirRelKey != snap.Groups[j].DirRelKey {
			return snap.Groups[i].DirRelKey < snap.Groups[j].DirRelKey
		}
		return snap.Groups[i].Key < snap.Groups[j].Key
	})

	// —— 元素6：规则命中统计（含禁用规则；复用文件的命中从旧记录累计）——
	hitByID := make(map[string]int)
	for _, f := range snap.Files {
		for _, ruleID := range f.MatchedRuleIDs {
			hitByID[ruleID]++
		}
	}
	for _, rule := range in.allRules {
		snap.RuleStats = append(snap.RuleStats, SnapRuleStat{
			ID: rule.ID, Description: rule.Description, Source: string(rule.Source),
			Enabled: rule.Enabled, HitCount: hitByID[rule.ID],
		})
	}

	// —— 死路径赋值（识别时点判定，供清理提示）——
	for relPath := range in.assigns {
		if !allRelPaths[relPath] {
			snap.StaleAssignments = append(snap.StaleAssignments, relPath)
		}
	}
	sort.Strings(snap.StaleAssignments)

	// —— 汇总统计 ——
	snap.Stats.TotalFiles = len(snap.Files)
	for _, f := range snap.Files {
		switch f.State {
		case snapStateIdentified:
			snap.Stats.Identified++
		case snapStateClueful:
			snap.Stats.Clueful++
		default:
			snap.Stats.Plain++
		}
		if len(f.PendingDims) > 0 {
			snap.Stats.PendingFiles++
		}
	}
	snap.Stats.GroupCount = len(snap.Groups)
	snap.Stats.SkippedDirs = len(in.enumRes.SkippedDirKeys)
	snap.Stats.StaleAssignments = len(snap.StaleAssignments)
	return snap
}

// snapFileOf 单文件结论 → 快照文件记录（元素2/3/5/7 的文件粒度聚合）。
// 线索探测仅对未识别文件执行（已识别文件的命中规则与终选声明已足呈现），
// 探测器全部复用既有机制：谓词原子与内置 pixiv 形态探测器。
func snapFileOf(c *FileConclusion, lib *LibraryIndex, assigns map[string]SessionAssignment) SnapFile {
	file := SnapFile{
		RelPath: c.RelPath, FileName: c.FileName, DirRelKey: c.DirRelKey,
		MatchedRuleIDs: c.MatchedRuleIDs,
	}
	if _, ok := assigns[c.RelPath]; ok {
		file.Assigned = true
	}
	for _, decl := range c.Final {
		file.Final = append(file.Final, snapDeclOf(decl))
	}
	for _, item := range c.Pending {
		file.PendingDims = append(file.PendingDims, string(item.Dimension))
	}
	if c.Group != nil {
		file.GroupKey = c.Group.Key
		file.GroupSeq = c.Group.Seq
	}
	if len(c.Final) > 0 {
		file.State = snapStateIdentified
		return file
	}
	file.Clues = detectClues(c, lib)
	if len(file.Clues) > 0 {
		file.State = snapStateClueful
		return file
	}
	file.State = snapStatePlain
	return file
}

// snapDeclOf 声明 → 紧凑形态。
func snapDeclOf(decl MetadataDeclaration) SnapDecl {
	return SnapDecl{
		Dimension: string(decl.Dimension), Value: decl.Value, Source: string(decl.Source),
		RoleName: decl.RoleName, RuleID: decl.RuleID,
	}
}

// snapPendingOf 待裁决项 → 快照明细。
func snapPendingOf(c *FileConclusion, item PendingItem) SnapPending {
	out := SnapPending{
		RelPath:   c.RelPath,
		FileName:  c.FileName,
		Dimension: string(item.Dimension),
		Reason:    item.Reason,
	}
	for _, decl := range item.Candidates {
		out.Candidates = append(out.Candidates, snapDeclOf(decl))
	}
	return out
}

// detectClues 对未识别文件探测疑似线索（元素3）：谓词原子以文件名去扩展名与
// 群组公共前缀分别对库内作者/标签名集合求值；pixiv 页级 ID 形态用内置惯例
// 探测器；群组成员无元数据声明时公共前缀即集名候选线索。
func detectClues(c *FileConclusion, lib *LibraryIndex) []SnapClue {
	var clues []SnapClue
	baseName := BaseNameWithoutExt(c.FileName)
	names := []string{baseName}
	if c.Group != nil && c.Group.Prefix != "" && c.Group.Prefix != baseName {
		names = append(names, c.Group.Prefix)
	}
	seen := make(map[string]bool)
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		for _, kind := range []PredicateKind{PredicateAuthor, PredicateTag} {
			pattern := &PredicatePattern{Kind: kind}
			ctx := &ParseContext{}
			if !pattern.Evaluate(&MatchInput{FileName: name}, lib, ctx) {
				continue
			}
			kindStr := cluePredicateAuthor
			if kind == PredicateTag {
				kindStr = cluePredicateTag
			}
			detail := name
			if name != baseName {
				detail = name + "（群组前缀）"
			}
			clues = append(clues, SnapClue{Kind: kindStr, Detail: detail})
		}
	}
	if pageID := parsePixivPageWorkID(baseName); pageID != "" {
		clues = append(clues, SnapClue{Kind: cluePixivID, Detail: pageID})
	}
	if c.Group != nil && c.Group.Prefix != "" {
		clues = append(clues, SnapClue{Kind: clueGroupPrefix, Detail: c.Group.Prefix})
	}
	return clues
}

// structuralSeqRe 结构性兄弟群的「公共前缀+尾缀序号」形态：前缀与序号间必须
// 有分隔符（空格/_-·.．），纯数字名与前缀直连数字（如 v1）不视为序号群。
var structuralSeqRe = regexp.MustCompile(`^(.+?)[ _\-·.．]+(\d{1,4})$`)

// detectStructuralGroups 在全景聚合后探测规则无关的兄弟群结构（元素4「成群/
// 落单」判定的结构面）：对未被规则定群（GroupKey 为空）的文件按目录与公共
// 前缀分桶，桶内 ≥2 成员即成结构群（RuleID 留空=非规则产物，呈现层标注
// 「结构」）；无终选声明且无线索冲突的成员补「群组前缀」线索并升格有线索
// 态（元素3 集名候选）。已识别成员仅入群呈现，不加线索不升格。
func detectStructuralGroups(snap *RecognitionSnapshot) {
	type member struct {
		relPath string
		seq     int
		seqText string
	}
	buckets := make(map[string][]member)
	fileByPath := make(map[string]*SnapFile, len(snap.Files))
	for i := range snap.Files {
		f := &snap.Files[i]
		fileByPath[f.RelPath] = f
		if f.GroupKey != "" {
			continue
		}
		m := structuralSeqRe.FindStringSubmatch(BaseNameWithoutExt(f.FileName))
		if m == nil {
			continue
		}
		seq, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		key := f.DirRelKey + "\x00" + m[1]
		buckets[key] = append(buckets[key], member{relPath: f.RelPath, seq: seq, seqText: m[2]})
	}
	for key, members := range buckets {
		if len(members) < 2 {
			continue
		}
		dirKey, prefix, _ := strings.Cut(key, "\x00")
		group := SnapGroup{DirRelKey: dirKey, Key: prefix, Size: len(members)}
		for _, mem := range members {
			group.Members = append(group.Members, SnapGroupMember{RelPath: mem.relPath, Seq: mem.seq, SeqText: mem.seqText})
			f := fileByPath[mem.relPath]
			if f == nil || len(f.Final) > 0 {
				continue
			}
			f.GroupKey = prefix
			f.GroupSeq = mem.seq
			dup := false
			for _, c := range f.Clues {
				if c.Kind == clueGroupPrefix {
					dup = true
					break
				}
			}
			if !dup {
				f.Clues = append(f.Clues, SnapClue{Kind: clueGroupPrefix, Detail: prefix})
			}
			if f.State == snapStatePlain {
				f.State = snapStateClueful
			}
		}
		sort.SliceStable(group.Members, func(i, j int) bool { return group.Members[i].Seq < group.Members[j].Seq })
		snap.Groups = append(snap.Groups, group)
	}
}

// firstMeaningRuleID 目录层含义回放中首个命中规则的 ID（元素1 的「被哪条规则
// 覆盖」呈现；与 dirMeaningsFromRules 同输入同序求值，仅取规则侧结论）。
func firstMeaningRuleID(rules []RuleEntry, level int, dirName string) string {
	depth := qnaDirDepth(level)
	if depth < 0 {
		return ""
	}
	dirNames := make([]string, depth+1)
	dirNames[depth] = dirName
	in := &MatchInput{DirNames: dirNames}
	for _, rule := range rules {
		if !rule.Enabled || rule.Pattern == nil {
			continue
		}
		if _, ok := rule.Pattern.(*SiblingPattern); ok {
			continue
		}
		ctx := &ParseContext{DirNames: dirNames, Depth: depth + 1}
		if !rule.Pattern.Evaluate(in, nil, ctx) {
			continue
		}
		if hit, ok := ctx.LastDirHit(); ok && hit.Depth == depth && hit.Name == dirName {
			return rule.ID
		}
	}
	return ""
}

// panoramaViewOf 快照的前端投影：游标表为后端增量态，不出前端载荷。
func panoramaViewOf(snap *RecognitionSnapshot) RecognitionSnapshot {
	view := *snap
	view.DirMtimes = nil
	return view
}

// —— 识别干跑编排 ——

// 识别事件种类（workbenchTopicEvent 通道的 kind 词汇）。
const (
	// workbenchEventRecognitionProgress 全量识别进度（阶段/计数流式推送）。
	workbenchEventRecognitionProgress = "recognitionProgress"
	// workbenchEventRecognitionDone 全量识别终态（成功携带统计摘要，失败携带
	// 用户可读错误）。
	workbenchEventRecognitionDone = "recognitionDone"
)

// 识别进度阶段词汇。
const (
	recogPhaseScan     = "scan"     // 轻量枚举（含增量跳过）
	recogPhaseIndex    = "index"    // 库内谓词索引构造
	recogPhaseEvaluate = "evaluate" // 合并求值（档案 ∪ 会话赋值）
	recogPhaseSave     = "save"     // 快照入会话与状态迁移
)

// publishRecognitionEvent 识别事件经工作台事件通道推送前端（上下文不可用时静默，
// 不影响识别本身）。
func (m *SessionManager) publishRecognitionEvent(evt *workbenchFrontendEvent) {
	if m.ctx == nil {
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		m.warnf("识别事件序列化失败: %v", err)
		return
	}
	if err := m.ctx.PublishToFrontend(workbenchTopicEvent, data); err != nil {
		m.warnf("识别事件推送失败(kind=%s): %v", evt.Kind, err)
	}
}

// StartRecognition 异步启动全量识别干跑：同库根已在途时不重复启动（返回
// running=true）；会话不存在时返回错误。干跑完成经 recognitionDone 事件通知，
// 快照与状态迁移由干跑主体完成。
func (m *SessionManager) StartRecognition(root string) (started, running bool, err error) {
	if _, err := m.loadSession(root); err != nil {
		return false, false, err
	}
	m.recogMu.Lock()
	if m.recogRunning == nil {
		m.recogRunning = make(map[string]bool)
	}
	if m.recogRunning[root] {
		m.recogMu.Unlock()
		return false, true, nil
	}
	m.recogRunning[root] = true
	m.recogMu.Unlock()
	go func() {
		defer func() {
			m.recogMu.Lock()
			delete(m.recogRunning, root)
			m.recogMu.Unlock()
		}()
		if err := m.runFullRecognition(root); err != nil {
			m.warnf("全量识别失败（root=%s）: %v", root, err)
			m.publishRecognitionEvent(&workbenchFrontendEvent{
				Kind: workbenchEventRecognitionDone, Root: root,
				Success: false, Message: fmt.Sprintf("全量识别失败：%v", err),
			})
			return
		}
	}()
	return true, false, nil
}

// runFullRecognition 全量识别干跑主体（零任务产出、零弹问的纯求值）：
// 轻量枚举（mtime 增量）→ 库内谓词索引构造 → 「档案 ∪ 会话赋值」合并求值 →
// 七类元素聚合 → 快照入会话与状态迁移（CompleteRecognition，单次写）。
// 增量资格：上次快照在场且其档案指纹与当前一致——档案外部变更或首次识别一律
// 全量重算；赋值文件所在目录即使 mtime 未变也强制重算（赋值按维度覆盖结论）。
func (m *SessionManager) runFullRecognition(root string) error {
	sess, err := m.loadSession(root)
	if err != nil {
		return err
	}

	// 规则装配：档案载入（坏规则逐条告警跳过，整体不可载入降级仅内置）
	profile, errs := LoadProfile(m.store)
	for _, err := range errs {
		m.warnf("规则档案载入异常（识别）: %v", err)
	}
	enabledRules := EffectiveRules(profile)
	allRules := allRulesWithBuiltins(profile)
	digest := ""
	if profile != nil {
		if d, err := profileDigestOf(profile); err == nil {
			digest = d
		}
	}

	// 增量资格：上次快照的档案指纹与当前一致才可按目录 mtime 游标跳过
	prev := sess.Snapshot
	incremental := prev != nil && prev.ProfileDigest == digest && digest != ""
	prevMtimes := map[string]int64(nil)
	if incremental {
		prevMtimes = prev.DirMtimes
	}

	enumRes, err := NewScanner(root).Enumerate(prevMtimes)
	if err != nil {
		return err
	}
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseScan,
		Done: len(enumRes.Files), Total: len(enumRes.AllFiles),
	})

	// 跳过集修正：含赋值文件的跳过目录强制重算（赋值覆盖维度需重求值）
	assignDirs := make(map[string]bool, len(sess.Assignments))
	for relPath := range sess.Assignments {
		assignDirs[dirRelKeyOf(relPath)] = true
	}
	prevSkipped := make(map[string]bool, len(enumRes.SkippedDirKeys))
	for _, dirKey := range enumRes.SkippedDirKeys {
		prevSkipped[dirKey] = true
	}
	evalFiles := enumRes.Files
	for _, f := range enumRes.AllFiles {
		dirKey := dirRelKeyOf(f.RelPath)
		if prevSkipped[dirKey] && assignDirs[dirKey] {
			evalFiles = append(evalFiles, f)
		}
	}
	for dirKey := range assignDirs {
		delete(prevSkipped, dirKey)
	}

	// 库内谓词索引（识别干跑注入求值：谓词规则与名字命中线索都吃它）
	var lib *LibraryIndex
	if m.ctx != nil {
		lib = BuildLibraryIndex(m.ctx, m.warnf)
	}
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseIndex,
		Done: indexSize(lib), Total: indexSize(lib),
	})

	conclusions := MergedRecognition(buildScanFiles(evalFiles), enabledRules, lib, sess.Assignments)
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseEvaluate,
		Done: len(conclusions), Total: len(evalFiles),
	})

	snapshot := aggregatePanorama(aggregateInput{
		root: root, enumRes: enumRes,
		enabledRules: enabledRules, allRules: allRules, lib: lib,
		conclusions: conclusions, prev: prev, prevSkipped: prevSkipped,
		assigns: sess.Assignments, profileDigest: digest,
	})

	if _, err := m.CompleteRecognition(root, snapshot); err != nil {
		return err
	}
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseSave,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
	})
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionDone, Root: root, Success: true,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
		Message: fmt.Sprintf("识别完成：共 %d 个文件（已识别 %d、有线索 %d、无线索 %d），序列群 %d，待裁决文件 %d",
			snapshot.Stats.TotalFiles, snapshot.Stats.Identified, snapshot.Stats.Clueful,
			snapshot.Stats.Plain, snapshot.Stats.GroupCount, snapshot.Stats.PendingFiles),
	})
	return nil
}

// indexSize 谓词索引的名字总量（进度呈现用；nil 索引为 0）。
func indexSize(lib *LibraryIndex) int {
	if lib == nil {
		return 0
	}
	return len(lib.AuthorNames) + len(lib.TagNames)
}

// allRulesWithBuiltins 全部规则（含禁用）：档案规则在前、内置规则在后，与
// EffectiveRules 的两段顺序一致（元素6 按此序呈现）。
func allRulesWithBuiltins(profile *RuleProfile) []RuleEntry {
	var rules []RuleEntry
	if profile != nil {
		rules = append(rules, profile.Rules...)
	}
	rules = append(rules, BuiltinRules()...)
	return rules
}
