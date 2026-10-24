// 全量识别引擎：识别干跑（零任务产出、零弹问的纯求值）的合并求值单一实现、
// 库内谓词索引构造、七类识别全景元素与集名候选墙（批次三阶段5）聚合与快照
// 裁剪模型、增量游标编排。
package main

import (
	"encoding/json"
	"fmt"
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
	// AuthorClueRef / TitleClueRef / CollectionClueRef 各维度赋值的线索出处引用
	//（clueRef，结构分段线索集的稳定标识：rootName / dirLevel[n] /
	// dirLevelStripped[n] / filenameSeg[i] / tailCode）：纯溯源簿记，不参与
	// 求值覆盖语义；规则总结器据此推导结构映射（手填无引用时以值=某段兜底）。
	// TagClueRefs 与 Tags 逐位平行（空串=该标签无出处）。
	AuthorClueRef     string   `json:"authorClueRef,omitempty"`
	TitleClueRef      string   `json:"titleClueRef,omitempty"`
	CollectionClueRef string   `json:"collectionClueRef,omitempty"`
	TagClueRefs       []string `json:"tagClueRefs,omitempty"`
	// Generalized 该赋值是否已参与泛化（泛化即接管语义：新入档规则完整复现的
	// 维度已从赋值剥离、全维度剥空的赋值已删除——标记仅对有残存未消费维度的
	// 赋值保留意义，供残存维度的清理提示）。
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
// （供识别全景的序列形态呈现）。
type FileConclusion struct {
	// RelPath / FileName 条目寻址；DirRelKey 为所属目录键（相对库根的 '/' 目录
	// 路径，根直下为空串）。
	RelPath   string
	FileName  string
	DirRelKey string
	// Group 为兄弟群组定群产物（群组成员非 nil，落单为 nil）；GroupRuleID 为
	// 定群规则 ID；GroupSeqText 为本成员序号的原文（前导零体现零填充形态）；
	// GroupElements 为文件名元素数组（规则定群群成员：与结构群同一拆分基座，
	// 序号元素公共性终判时按序号原文标记；前端按序拼接渲染块底色）。
	Group         *GroupAnalysis
	GroupRuleID   string
	GroupSeqText  string
	GroupElements []SnapEl
	// GroupFormName / GroupFormKeyExpr / GroupFormSeqExpr 为结构群携带的命中
	// 形态标识（形态名与键/序号提取式原文，供快照群卡呈现与序列形态的规则
	// 泛化器消费）；规则定群成员为空。
	GroupFormName    string
	GroupFormKeyExpr string
	GroupFormSeqExpr string
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
	// 同目录兄弟全集按「父目录」批量构造（与 RunPipeline 内部同键同序）；单文件
	// 重算通道（recomputeFileDecision）经磁盘重扫预填兄弟全集后共用同一单文件
	// 合并实现
	siblingsByDir := make(map[string][]string, len(files))
	for _, file := range files {
		key := dirGroupKey(file.DirNames)
		siblingsByDir[key] = append(siblingsByDir[key], file.FileName)
	}
	// 结构群前置对齐每目录只算一次，成员按相对路径登记，各单文件结论取用
	structuralByRel := precomputeStructuralMembers(files, rules, lib, siblingsByDir)
	conclusions := make([]FileConclusion, 0, len(files))
	for _, file := range files {
		in := &MatchInput{
			FileName: file.FileName, DirNames: file.DirNames, RelPath: file.RelPath,
			Siblings: siblingsByDir[dirGroupKey(file.DirNames)],
		}
		var assign *SessionAssignment
		if a, ok := assigns[file.RelPath]; ok {
			assign = &a
		}
		conclusions = append(conclusions, mergedFileConclusionWithStructural(in, rules, lib, assign, structuralByRel[file.RelPath]))
	}
	return conclusions
}

// mergedFileConclusionOf 单文件「档案规则 ∪ 会话赋值」合并求值结论（无批量
// 预计算形态）：结构群补位在函数内现场对齐，其余与批量入口同一实现——
// 执行期单文件重算通道经此与识别干跑共用合并逻辑。
func mergedFileConclusionOf(in *MatchInput, rules []RuleEntry, lib *LibraryIndex, assign *SessionAssignment) FileConclusion {
	return mergedFileConclusionWithStructural(in, rules, lib, assign, nil)
}

// mergedFileConclusionWithStructural 单文件「档案规则 ∪ 会话赋值」合并求值结论
// ——合并逻辑的唯一实现：先流水线两段求值（分组定群+提取声明，注入库内谓词
// 索引），再合成终选与待裁决，最后会话赋值按维度覆盖终选（特异性压过普适）。
// 结构群补位发生在规则定群段之后、提取段之前：规则群优先，规则未定群且前置
// 对齐成群时把结构群分析写入求值上下文的同一群组字段（群组序号/规模操作数
// 与群组前缀提取对规则群与结构群统一可取值；落单文件群组字段保持空，含群组
// 操作数的比较取值不可用恒为假）。pre 为批量入口预计算的结构群成员信息，
// nil 时现场对齐（与批量入口同一实现，两期结论一致）。群组成员附定群规则 ID
// 与序号原文；识别干跑（MergedRecognition 批量入口）与执行期单文件重算
// （recomputeFileDecision，消费任务级赋值快照）共用本函数。
func mergedFileConclusionWithStructural(in *MatchInput, rules []RuleEntry, lib *LibraryIndex, assign *SessionAssignment, pre *structuralMemberInfo) FileConclusion {
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	decision := FileDecision{RelPath: in.RelPath, FileName: in.FileName}
	group, groupRuleID := runGroupStage(rules, in, lib, ctx)
	decision.Group = group
	if group == nil {
		if pre == nil {
			pre = alignStructuralForFile(in, rules, lib)
		}
		if pre != nil {
			ctx.Group = pre.analysis
		}
	} else {
		pre = nil // 规则群优先：已定群文件不再取结构群身份
	}
	runExtractionStage(&decision, rules, in, lib, ctx)
	final, pending := Synthesize(decision.Declarations, AggregateSynthesis(rules))
	if assign != nil {
		final, pending = applyAssignmentToConclusion(final, pending, assign)
	}
	c := FileConclusion{
		RelPath: in.RelPath, FileName: in.FileName, DirRelKey: dirRelKeyOf(in.RelPath),
		Group: decision.Group, MatchedRuleIDs: decision.MatchedRuleIDs,
		Final: final, Pending: pending,
	}
	if decision.Group != nil && groupRuleID != "" {
		c.GroupRuleID = groupRuleID
		var grpRule *RuleEntry
		for i := range rules {
			if rules[i].ID == groupRuleID {
				grpRule = &rules[i]
				break
			}
		}
		if grpRule != nil {
			if text, has := groupSeqTextOf(grpRule, in.FileName); has {
				c.GroupSeqText = text
			}
			// 文件名元素数组（规则定群群成员）：与结构群同一拆分基座；已知名
			// 提示=目录名+剥壳变体（库根提示在求值输入不可得，省略——规则群
			// 成员的元素呈现不依赖它）。
			c.GroupElements = splitFilenameElements(BaseNameWithoutExt(in.FileName), dirNameHints(in.DirNames))
		}
	} else if pre != nil && pre.analysis != nil {
		// 结构群补位成员：群组分析、序号原文、元素数组与命中形态标识随结论
		// 携带（快照组装消费）；定群规则 ID 保持空——结构群非规则产物。
		c.Group = pre.analysis
		c.GroupSeqText = pre.seqText
		c.GroupElements = pre.elements
		c.GroupFormName, c.GroupFormKeyExpr, c.GroupFormSeqExpr = pre.formName, pre.formKeyExpr, pre.formSeqExpr
	}
	return c
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
	QueryLocalWorkSets(req *sdkdto.QueryLocalWorkSetsRequest) (*sdkdto.QueryLocalWorkSetsResponse, error)
}

// libraryIndexPageSize 库查询分页大小（宿主数据库单连接与 UI 共享，取插件开发
// 指南的上限指导值）。
const libraryIndexPageSize = 200

// BuildLibraryIndex 经只读库查询构造谓词索引：本地/站点两轨的作者名与标签名全集、
// 库内作品集轨（分页拉全，集名全集与集名→集行 id 两表同一次拉取填充）。任一轨
// 查询失败时该轨按空集继续并告警（谓词规则对空集恒不命中，不阻断识别）；查询源
// 不可用返回 nil 索引。
func BuildLibraryIndex(q libraryIndexQuerySource, warnf func(format string, args ...any)) *LibraryIndex {
	if q == nil {
		return nil
	}
	lib := &LibraryIndex{
		AuthorNames:  make(map[string]struct{}),
		TagNames:     make(map[string]struct{}),
		WorkSetNames: make(map[string]struct{}),
		WorkSetIDs:   make(map[string]int64),
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
	// 库内作品集轨（单次分页拉取同时填充两表）：集名全集 → 互证索引，
	// 集名 → 集行 id → A 档「归入既有集」的既有 id 来源。任一页失败按
	// 空集降级告警（同其余轨）。
	for page := int32(1); ; page++ {
		resp, err := q.QueryLocalWorkSets(&sdkdto.QueryLocalWorkSetsRequest{
			Page: &sdkdto.PageRequest{Page: page, PageSize: libraryIndexPageSize},
		})
		if err != nil {
			if warnf != nil {
				warnf("构造谓词索引时查询库内作品集失败，该轨按空集处理（谓词规则对该轨恒不命中）: %v", err)
			}
			return lib
		}
		names := make([]string, 0, len(resp.Items))
		for _, item := range resp.Items {
			if item == nil || item.Name == "" {
				continue
			}
			names = append(names, item.Name)
			if item.Id > 0 {
				lib.WorkSetIDs[item.Name] = item.Id
			}
		}
		addNames(names, lib.WorkSetNames)
		if len(names) < libraryIndexPageSize {
			return lib
		}
	}
}

// authorNamesOf / tagNamesOf 等四函数把查询条目投影为名字集（空名剔除）；
// 库内作品集轨的条目投影内联在 BuildLibraryIndex 的拉取循环内（名与行 id
// 同次消费）。
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
	clueSetMatch        = "set-name-match"   // 序列群分群键命中库内既有集名——集名互证
)

// recognitionSnapshotSchemaVersion 快照结构版本（自管）。v2：成员 Segs 区间
// → Elements 元素数组（元素级内核 批次二）、候选增 KeyElements 键元素池；旧
// 快照（v1）反序列化按空元素数组降级——识别重跑即重建。
const recognitionSnapshotSchemaVersion = 2

// RecognitionSnapshot 识别全景快照：识别时点的全景结论缓存（可按「原始路径+档案」
// 重算），以裁剪形态入会话 KV——只存呈现与增量复用所需字段：不含文件完整路径
// （可由库根+相对路径重建）、不含内容哈希（干跑不读内容）、不含同目录兄弟全集
// （可由快照文件清单重建）、不含提取段原始中间声明（终选与待裁决候选已足呈现）。
type RecognitionSnapshot struct {
	// SchemaVersion 快照结构版本（自管）。
	SchemaVersion int `json:"schemaVersion"`
	// GeneratedAtMs 快照生成时间（Unix 毫秒）。
	GeneratedAtMs int64 `json:"generatedAtMs"`
	// RootPath 库根原始输入。
	RootPath string `json:"rootPath"`
	// ProfileDigest 生成本快照所依据的规则档案指纹（重识别的增量资格判定）；
	// AssignmentRevision 求值依据的会话赋值修订号——赋值被清除后其目录不再由
	// 赋值集锚定，修订号闸门保证旧结论不被 mtime 游标复用（赋值变更一律全量
	// 重算）。
	ProfileDigest      string `json:"profileDigest"`
	AssignmentRevision int64  `json:"assignmentRevision"`

	// Dirs 目录结构（元素1）：层含义覆盖状态随目录树节点呈现。
	Dirs []SnapDir `json:"dirs"`
	// Files 文件级记录（元素2/3 与元素7 赋值标记）。
	Files []SnapFile `json:"files"`
	// Groups 序列群（元素4）：公共前缀、组大小与成员序号形态。
	Groups []SnapGroup `json:"groups"`
	// Candidates 集名候选主清单（元素8，批次三阶段5）：目录/序列群两类来源
	// 合流的候选墙单一数据源（成员带序列块区间），A 档库内集识别注记
	//（互证/归入既有集）随卡携带；已忽略/已确认的候选分仓至对应清单、不
	// 重复出现在主清单。
	Candidates []SnapCandidate `json:"candidates"`
	// IgnoredCandidates / ConfirmedCandidates 已忽略/已确认候选分仓（会话态
	// 随重识别重算；confirmed 卡带确认集名与既有集行 id）。
	IgnoredCandidates   []SnapCandidate `json:"ignoredCandidates,omitempty"`
	ConfirmedCandidates []SnapCandidate `json:"confirmedCandidates,omitempty"`
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
	// StructClues 目录级结构分段线索集（库根名/目录层/剥壳变体，无文件名段）：
	// 纯路径推导的呈现态数据，getPanorama 投影时展开（不入持久快照，与文件级
	// 同约定）——目录面板赋值表单的出处引用数据源。
	StructClues []SnapStructClue `json:"structClues,omitempty"`
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
	// StructClues 结构分段线索集（结构直连规则构造）：库根名 / 目录层 / 剥壳
	// 变体 / 文件名分隔段 / 尾部码——纯路径推导，在 getPanorama 投影时展开
	// （不入持久快照，快照只存原始寻址三件）。
	StructClues []SnapStructClue `json:"structClues,omitempty"`
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
	// FormName / FormKeyExpr / FormSeqExpr 结构群携带的命中形态标识：形态名
	// 与键/序号提取式原文（各自捕获组1 = 分群键/序号原文）；规则定群不带
	// 形态。序列形态的规则泛化器据群携带的提取式构造兄弟群规则（提取式与
	// SiblingPattern 的 KeyExpr/SeqExpr 同构）。
	FormName    string `json:"formName,omitempty"`
	FormKeyExpr string `json:"formKeyExpr,omitempty"`
	FormSeqExpr string `json:"formSeqExpr,omitempty"`
	// Members 成员（集内序号与原文——原文前导零体现零填充形态）。
	Members []SnapGroupMember `json:"members"`
}

// SnapGroupMember 序列群成员。
type SnapGroupMember struct {
	RelPath string `json:"relPath"`
	Seq     int    `json:"seq"`
	SeqText string `json:"seqText"`
	// Elements 文件名元素数组（批次二起取代 Segs 区间）：按序拼接 text 还原
	// 去扩展名正文；公共性终判（title/plain）由 markCommonElements 统一落格。
	Elements []SnapEl `json:"elements,omitempty"`
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
	// profileDigest 本轮档案指纹；assignRevision 本轮会话赋值修订号。
	profileDigest  string
	assignRevision int64
	// ignoredTargets / confirmedTargets 候选分仓判据（会话态目标键集合，元素8）；
	// confirmedSetIDs 确认集名 → 既有集行 id 的会话旁路（confirmed 分仓的
	// 既有 id 呈现数据源）。
	ignoredTargets   map[string]bool
	confirmedTargets map[string]bool
	confirmedSetIDs  map[string]int64
}

// aggregatePanorama 把合并求值结论聚合为识别全景快照（七类元素）：
//
//	1 目录结构：目录树节点 + 各层含义的规则回放（未覆盖呈现空）；
//	2 已识别资源：终选声明（含 role）+ 分组归属（群组键/集内序/落单）；
//	3 有价值未识别：谓词原子（文件名与群组前缀对库内名字）、pixiv ID 形态、
//	  群组前缀（集名候选）、集名互证（分群键命中库内既有集名）四类线索；
//	4 序列：兄弟群的公共前缀/组大小/成员序号形态（零填充由原文前导零呈现）；
//	5 待裁决项：合成层声明冲突的维度与候选对照；
//	6 规则档案态：全部规则的来源/启用态/命中数；
//	7 会话决策态：赋值标记入文件记录（计数为会话活数据，经查询响应另呈）；
//	8 集名候选墙（批次三阶段5）：目录/序列群两类来源合流的集名候选 + A 档库内
//	  集识别注记（互证/归入既有集），会话态忽略/确认目标键分仓。
func aggregatePanorama(in aggregateInput) *RecognitionSnapshot {
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		GeneratedAtMs: time.Now().UnixMilli(),
		RootPath:      in.root,
		ProfileDigest: in.profileDigest, AssignmentRevision: in.assignRevision,
		DirMtimes: in.enumRes.DirMtimes,
		// 序列化契约：快照经 JSON 落会话 KV 并直接驱动前端渲染，切片字段
		// 一律非 nil（nil 序列化为 null，前端 .length 即崩）；Pending 与
		// StaleAssignments 带 omitempty（缺席由前端守卫兜底）。
		Dirs:       make([]SnapDir, 0, len(in.enumRes.Dirs)),
		Files:      make([]SnapFile, 0, len(in.enumRes.AllFiles)),
		Groups:     make([]SnapGroup, 0),
		Candidates: make([]SnapCandidate, 0),
		RuleStats:  make([]SnapRuleStat, 0, len(in.enabledRules)),
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
			// 群组键含定群规则 ID：规则群与结构群同目录同键时分立（一文件至多
			// 一个群组身份，两类群不合并呈现）。
			gkey := c.DirRelKey + "\x00" + c.Group.Key + "\x00" + c.GroupRuleID
			group := groups[gkey]
			if group == nil {
				group = &SnapGroup{DirRelKey: c.DirRelKey, Key: c.Group.Key, Size: c.Group.Size, RuleID: c.GroupRuleID}
				if c.GroupRuleID == "" {
					// 结构群：命中形态标识随结论携带（规则定群不带形态）
					group.FormName, group.FormKeyExpr, group.FormSeqExpr = c.GroupFormName, c.GroupFormKeyExpr, c.GroupFormSeqExpr
				}
				groups[gkey] = group
			}
			group.Members = append(group.Members, SnapGroupMember{
				RelPath: c.RelPath, Seq: c.Group.Seq, SeqText: c.GroupSeqText, Elements: c.GroupElements,
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
			for i := range in.prev.Groups {
				g := in.prev.Groups[i]
				if g.DirRelKey == prevFile.DirRelKey && g.Key == prevFile.GroupKey {
					// 复用键与结论侧同构（目录 + 分组键 + 定群规则 ID），命中即
					// 整组拷贝一次
					gkey := prevFile.DirRelKey + "\x00" + prevFile.GroupKey + "\x00" + g.RuleID
					if !reusedGroups[gkey] {
						reusedGroups[gkey] = true
						groups[gkey] = &g
					}
					break
				}
			}
		}
	}

	// —— 元素4 结构性兄弟群（规则无关）的对齐判定已前置到求值段（规则定群段
	// 之后、提取段之前，结论携带成员信息），此处只做呈现标记：未识别成员记
	// 文件行群组键与「群组前缀=集名候选」线索（元素3）并升格有线索态；已识别
	// 成员仅入群呈现，不加线索不升格。跳过复用目录的群零操作（成员文件复用
	// 旧记录时已带标记，重复标记为幂等空转）。——
	structuralMarks := make([]SnapGroup, 0)
	for _, group := range groups {
		if group.RuleID == "" {
			structuralMarks = append(structuralMarks, *group)
		}
	}
	markStructuralPresentation(snap, structuralMarks)

	// 序列群按集内序排列成员、规则定群群落元素公共性终判（结构群的元素终判在
	// 对齐装配内完成），群间按目录与分组键排序（呈现稳定序）
	for _, group := range groups {
		sort.SliceStable(group.Members, func(i, j int) bool {
			return group.Members[i].Seq < group.Members[j].Seq
		})
		if group.RuleID != "" {
			markCommonElements(group)
		}
		snap.Groups = append(snap.Groups, *group)
	}
	sort.Slice(snap.Groups, func(i, j int) bool {
		if snap.Groups[i].DirRelKey != snap.Groups[j].DirRelKey {
			return snap.Groups[i].DirRelKey < snap.Groups[j].DirRelKey
		}
		return snap.Groups[i].Key < snap.Groups[j].Key
	})

	// —— 集名互证（批次二阶段3）：全部序列群（规则定群与结构群——此时群集合已
	// 完整）的分群键对库内作品集名全集精确匹配，命中的群全员补「集名互证」线索
	//（plain 成员升格有线索态，统计在末段按终态汇总）——
	attachSetNameCrossCheck(snap, in.lib)

	// —— 元素8：集名候选墙（批次三阶段5）：三类来源合流 + A 档库内集识别注记
	// + 会话态分仓（ignored/confirmed），见 buildSnapshotCandidates。——
	buildSnapshotCandidates(snap, in.lib, in)

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
	if c.Group != nil && c.GroupRuleID != "" {
		// 文件行群组键只落规则定群成员；结构群成员的文件行群组键由呈现标记步
		// 决定（仅未识别成员落键——已识别成员的群组信息只在群卡呈现）
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
// 探测器；群组成员无元数据声明时公共前缀即集名候选线索。群组前缀的谓词匹配
// 与线索呈现只吃规则定群产物——结构群成员的群组键线索由快照呈现标记步补
// （未识别成员才落线索，与结构群前置前的呈现一致）。
func detectClues(c *FileConclusion, lib *LibraryIndex) []SnapClue {
	var clues []SnapClue
	baseName := BaseNameWithoutExt(c.FileName)
	names := []string{baseName}
	if c.GroupRuleID != "" && c.Group != nil && c.Group.Prefix != "" && c.Group.Prefix != baseName {
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
	if c.GroupRuleID != "" && c.Group != nil && c.Group.Prefix != "" {
		clues = append(clues, SnapClue{Kind: clueGroupPrefix, Detail: c.Group.Prefix})
	}
	return clues
}

// alignEntry 元素对齐判定的参与文件：文件名、相对路径与元素数组、非分隔元素
// 下标序列（求值段前置对齐与快照直检共用同一参与形态）。
type alignEntry struct {
	fileName string
	relPath  string
	base     string
	els      []SnapEl
	nonSep   []int
}

// tailCodeRange 豁免的尾部随机码位区间 [lo,hi]（末词=末个分隔 run 之后拼接
// 而成的词；词内数字子段不破坏码原子性——`deq5g43` 拆为 deq/5/g/43 后整词
// 仍按码豁免）。豁免条件=整词 looksLikeTailCode 且去掉尾部数字流后仍含数字
// （码的随机性在内部数字；纯前缀+尾数字流=直连序号语义，不豁免——`abc12`
// 的 `12` 是序号候选）。豁免位不参与等值也不入键（中段序号「随机码不入键」
// 先例的泛化）；ok=false = 无豁免。
func (e *alignEntry) tailCodeRange() (int, int, bool) {
	n := len(e.nonSep)
	if n == 0 {
		return 0, 0, false
	}
	lo := n - 1
	for lo > 0 && e.nonSep[lo]-e.nonSep[lo-1] == 1 {
		lo-- // 词内子段与前一非分隔元素直连（无分隔 run）→ 同属末词
	}
	var word strings.Builder
	for k := lo; k < n; k++ {
		word.WriteString(e.els[e.nonSep[k]].Text)
	}
	w := word.String()
	if !looksLikeTailCode(w) {
		return 0, 0, false
	}
	i := len(w)
	for i > 0 && w[i-1] >= '0' && w[i-1] <= '9' {
		i--
	}
	if i < len(w) && !strings.ContainsAny(w[:i], "0123456789") {
		return 0, 0, false // 前缀无数字=直连序号语义，不豁免
	}
	return lo, n - 1, true
}

// alignSignature 对齐签名：非分隔元素数 + 除序号位/豁免尾部码位外每位的元素
// 类别类（括号/词）与对齐取值 + 除序号紧邻 gap 外的分隔形态（直连/分隔）。
// 序号前 gap 容忍（`_?[NN]`/`[分隔 run]+` 同族归键的既有语义——不同分隔形态
// 的同族文件归同键）。
func (e *alignEntry) alignSignature(seqPos int) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(len(e.nonSep)))
	exclLo, exclHi, hasExcl := e.tailCodeRange()
	for i, idx := range e.nonSep {
		switch {
		case i == seqPos:
			b.WriteString("\x01S")
		case hasExcl && i >= exclLo && i <= exclHi:
			b.WriteString("\x01X")
		default:
			el := e.els[idx]
			cls := "w"
			if el.Shelled {
				cls = "b"
			}
			b.WriteString("\x01" + cls + "\x02" + elValue(el))
		}
	}
	for i := 0; i+1 < len(e.nonSep); i++ {
		if i == seqPos-1 {
			continue
		}
		if e.nonSep[i]+1 == e.nonSep[i+1] {
			b.WriteString("\x03g")
		} else {
			b.WriteString("\x03s")
		}
	}
	return b.String()
}

// newAlignEntry 构造元素对齐判定的参与文件：去扩展名正文按已知名提示拆元素
// 数组，并记非分隔元素下标序列。
func newAlignEntry(fileName, relPath string, hints []string) *alignEntry {
	base := BaseNameWithoutExt(fileName)
	e := &alignEntry{fileName: fileName, relPath: relPath, base: base, els: splitFilenameElements(base, hints)}
	for j, el := range e.els {
		if el.Kind != elKindSep {
			e.nonSep = append(e.nonSep, j)
		}
	}
	return e
}

// detectStructuralGroups 在快照文件集上直接探测规则无关的兄弟群结构（快照直检
// 入口，原始快照形态的消费方）：对未被规则定群的文件按元素拆分基座分解，同
// 目录文件做元素序列对齐——非分隔元素数一致、除序号位外全部相同（对齐取值=
// 剥壳内名，尾部随机码元素豁免等值）且序号位为 1-4 位纯数字、各成员取值互异
// → 成群。多候选数字位并存时取「各文件间取值互异度最大」的数字位为序号、
// 其余数字位入键，同互异度取最左位；被先选群消费的文件不再参与后续位。
// 群携带命中形态标识（形态名按序号元素上下文映射、提取式取自形态表——命名
// 层；键文本优先由形态键提取式复原，新形态退公共元素拼接兜底）。无终选声明
// 且无线索冲突的成员补「群组前缀」线索并升格有线索态（元素3 集名候选）；
// 已识别成员仅入群呈现，不加线索不升格。识别主流水线的结构群由求值段的前置
// 对齐产出（同一判定实现）、经结论注入快照组装，不走本入口。
func detectStructuralGroups(snap *RecognitionSnapshot) {
	byDir := make(map[string][]*alignEntry)
	for i := range snap.Files {
		f := &snap.Files[i]
		if f.GroupKey != "" {
			continue
		}
		byDir[f.DirRelKey] = append(byDir[f.DirRelKey], newAlignEntry(f.FileName, f.RelPath, elementHints(snap.RootPath, f.RelPath)))
	}
	dirKeys := make([]string, 0, len(byDir))
	for k := range byDir {
		dirKeys = append(dirKeys, k)
	}
	sort.Strings(dirKeys)
	for _, dirKey := range dirKeys {
		groups := alignStructuralDir(dirKey, byDir[dirKey])
		snap.Groups = append(snap.Groups, groups...)
		markStructuralPresentation(snap, groups)
	}
}

// markStructuralPresentation 结构群呈现标记：未识别成员记文件行群组键与集内
// 序、补「群组前缀=集名候选」线索（同类别线索去重）并升格有线索态；已识别
// 成员仅入群卡呈现，不加线索不升格（其群组信息只在群卡）。
func markStructuralPresentation(snap *RecognitionSnapshot, groups []SnapGroup) {
	if len(groups) == 0 {
		return
	}
	fileByPath := make(map[string]*SnapFile, len(snap.Files))
	for i := range snap.Files {
		fileByPath[snap.Files[i].RelPath] = &snap.Files[i]
	}
	for _, group := range groups {
		for _, mem := range group.Members {
			f := fileByPath[mem.RelPath]
			if f == nil || len(f.Final) > 0 {
				continue
			}
			f.GroupKey = group.Key
			f.GroupSeq = mem.Seq
			dup := false
			for _, c := range f.Clues {
				if c.Kind == clueGroupPrefix {
					dup = true
					break
				}
			}
			if !dup {
				f.Clues = append(f.Clues, SnapClue{Kind: clueGroupPrefix, Detail: group.Key})
			}
			if f.State == snapStatePlain {
				f.State = snapStateClueful
			}
		}
	}
}

// alignStructuralDir 同目录候选文件集的结构群对齐判定（唯一实现）：逐候选数字
// 位分桶（桶内签名一致 = 除序号位外全部相同），互异度降序、位次升序贪心选群，
// 被先选群消费的文件不再入群。参与文件先按文件名稳定排序——批量干跑（扫描
// 序）、单文件重算（磁盘重扫序）与快照直检（快照文件序）在此归一为同一顺序，
// 各入口结论一致。候选集由调用方给足（排除规则定群文件）。
func alignStructuralDir(dirKey string, entries []*alignEntry) []SnapGroup {
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].fileName < entries[j].fileName })
	// 逐候选数字位分桶（桶内签名一致 = 除序号位外全部相同）。
	type provisional struct {
		seqPos    int
		members   []*alignEntry
		diversity int
	}
	var provs []provisional
	maxCount := 0
	for _, e := range entries {
		if n := len(e.nonSep); n > maxCount {
			maxCount = n
		}
	}
	for pos := 0; pos < maxCount; pos++ {
		buckets := make(map[string][]*alignEntry)
		order := make([]string, 0)
		for _, e := range entries {
			if pos >= len(e.nonSep) || e.els[e.nonSep[pos]].Kind != elKindSeq {
				continue
			}
			sig := e.alignSignature(pos)
			if _, ok := buckets[sig]; !ok {
				order = append(order, sig)
			}
			buckets[sig] = append(buckets[sig], e)
		}
		for _, sig := range order {
			b := buckets[sig]
			distinct := make(map[string]bool, len(b))
			for _, e := range b {
				distinct[elValue(e.els[e.nonSep[pos]])] = true
			}
			// 序号取值互异（防 `1,1` 假序列——同基名不同扩展名不成群）。
			if len(b) >= 2 && len(distinct) == len(b) {
				provs = append(provs, provisional{seqPos: pos, members: b, diversity: len(distinct)})
			}
		}
	}
	// 互异度降序、位次升序贪心选群；已消费文件不再入群。
	sort.SliceStable(provs, func(i, j int) bool {
		if provs[i].diversity != provs[j].diversity {
			return provs[i].diversity > provs[j].diversity
		}
		return provs[i].seqPos < provs[j].seqPos
	})
	consumed := make(map[*alignEntry]bool)
	var groups []SnapGroup
	for _, pr := range provs {
		members := make([]*alignEntry, 0, len(pr.members))
		for _, e := range pr.members {
			if !consumed[e] {
				members = append(members, e)
			}
		}
		if len(members) < 2 {
			continue
		}
		groups = append(groups, buildAlignedGroup(dirKey, members, pr.seqPos))
		for _, e := range members {
			consumed[e] = true
		}
	}
	return groups
}

// structuralMemberInfo 单文件的结构群前置对齐产物：注入求值上下文的群组分析
// （与规则定群共用群组字段的补位值）与快照组装消费的序号原文、元素数组、命中
// 形态标识。
type structuralMemberInfo struct {
	analysis    *GroupAnalysis
	seqText     string
	elements    []SnapEl
	formName    string
	formKeyExpr string
	formSeqExpr string
}

// dirNameHints 求值输入可得的名字提示：各级目录名 + 剥壳变体（库根名在求值
// 输入不可得，不参与结构群元素拆分——与规则定群群成员的元素拆分同口径）。
func dirNameHints(dirNames []string) []string {
	hints := make([]string, 0, 2*len(dirNames))
	hints = append(hints, dirNames...)
	for _, name := range dirNames {
		if _, inner, ok := stripShell(name); ok {
			hints = append(hints, inner)
		}
	}
	return hints
}

// siblingRelPath 同目录兄弟文件名的相对路径：取本文件相对路径的目录前缀拼接
// 兄弟文件名（即本文件自身时原样返回）。
func siblingRelPath(relPath, fileName string) string {
	idx := strings.LastIndex(relPath, "/")
	if idx < 0 {
		return fileName
	}
	return relPath[:idx+1] + fileName
}

// precomputeStructuralMembers 批量入口的结构群前置对齐：每目录一次——先对目录
// 内各文件求规则定群段（规则群优先，命中者不入候选——与单文件结论内的定群
// 求值同一输入），再对候选文件集跑对齐判定的唯一实现，成员按相对路径登记供
// 各单文件结论取用。
func precomputeStructuralMembers(files []ScanFile, rules []RuleEntry, lib *LibraryIndex, siblingsByDir map[string][]string) map[string]*structuralMemberInfo {
	byDir := make(map[string][]ScanFile, len(siblingsByDir))
	for _, file := range files {
		key := dirGroupKey(file.DirNames)
		byDir[key] = append(byDir[key], file)
	}
	out := make(map[string]*structuralMemberInfo)
	for key, dirFiles := range byDir {
		hints := dirNameHints(dirFiles[0].DirNames)
		var entries []*alignEntry
		for _, file := range dirFiles {
			in := &MatchInput{
				FileName: file.FileName, DirNames: file.DirNames, RelPath: file.RelPath,
				Siblings: siblingsByDir[key],
			}
			if group, _ := runGroupStage(rules, in, lib, &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}); group != nil {
				continue
			}
			entries = append(entries, newAlignEntry(file.FileName, file.RelPath, hints))
		}
		for _, g := range alignStructuralDir(dirRelKeyOf(dirFiles[0].RelPath), entries) {
			for _, mem := range g.Members {
				out[mem.RelPath] = &structuralMemberInfo{
					analysis:    &GroupAnalysis{Key: g.Key, Prefix: g.Key, Seq: mem.Seq, Size: g.Size},
					seqText:     mem.SeqText,
					elements:    mem.Elements,
					formName:    g.FormName,
					formKeyExpr: g.FormKeyExpr,
					formSeqExpr: g.FormSeqExpr,
				}
			}
		}
	}
	return out
}

// alignStructuralForFile 单文件通道的结构群前置对齐：兄弟名单（含自身，与求值
// 输入同一份）先逐名求规则定群段排除已定群者，再跑与批量入口同一实现的对齐
// 判定，取本文件所在群的成员信息；落单（不在任何群）返回 nil。
func alignStructuralForFile(in *MatchInput, rules []RuleEntry, lib *LibraryIndex) *structuralMemberInfo {
	hints := dirNameHints(in.DirNames)
	seen := make(map[string]bool, len(in.Siblings))
	var entries []*alignEntry
	for _, name := range in.Siblings {
		if seen[name] {
			continue
		}
		seen[name] = true
		rel := siblingRelPath(in.RelPath, name)
		sib := &MatchInput{FileName: name, DirNames: in.DirNames, RelPath: rel, Siblings: in.Siblings}
		if group, _ := runGroupStage(rules, sib, lib, &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}); group != nil {
			continue
		}
		entries = append(entries, newAlignEntry(name, rel, hints))
	}
	self := siblingRelPath(in.RelPath, in.FileName)
	for _, g := range alignStructuralDir(dirRelKeyOf(in.RelPath), entries) {
		for _, mem := range g.Members {
			if mem.RelPath != self {
				continue
			}
			return &structuralMemberInfo{
				analysis:    &GroupAnalysis{Key: g.Key, Prefix: g.Key, Seq: mem.Seq, Size: g.Size},
				seqText:     mem.SeqText,
				elements:    mem.Elements,
				formName:    g.FormName,
				formKeyExpr: g.FormKeyExpr,
				formSeqExpr: g.FormSeqExpr,
			}
		}
	}
	return nil
}

// buildAlignedGroup 对齐桶 → 结构群：序号文本=序号元素对齐取值；形态名按
// 序号元素上下文映射（提取式取自形态表）；键文本优先由形态键提取式复原
// （既有形态键语义零漂移——如中段序号键=`_page_` 前标题、随机码不入键），
// 提取式不命中的新形态退公共元素拼接兜底。成员按序号数值升序、随群携带
// 元素副本（公共性终判随后由 markCommonElements 落格）。
func buildAlignedGroup(dirKey string, members []*alignEntry, seqPos int) SnapGroup {
	sort.SliceStable(members, func(i, j int) bool {
		vi, _ := strconv.Atoi(elValue(members[i].els[members[i].nonSep[seqPos]]))
		vj, _ := strconv.Atoi(elValue(members[j].els[members[j].nonSep[seqPos]]))
		return vi < vj
	})
	first := members[0]
	form := formByName(formNameOfSeqContext(first.els, first.nonSep, seqPos))
	key := ""
	if form != nil {
		if km := form.keyRe.FindStringSubmatch(first.base); km != nil && len(km) >= 2 {
			key = km[1]
		}
	}
	if key == "" {
		skip := map[int]bool{first.nonSep[seqPos]: true}
		if lo, hi, ok := first.tailCodeRange(); ok {
			for k := lo; k <= hi; k++ {
				skip[first.nonSep[k]] = true
			}
		}
		key = elementConcatKey(first.els, skip)
	}
	group := SnapGroup{DirRelKey: dirKey, Key: key, Size: len(members)}
	if form != nil {
		group.FormName, group.FormKeyExpr, group.FormSeqExpr = form.name, form.keyExpr, form.seqExpr
	}
	for _, e := range members {
		st := elValue(e.els[e.nonSep[seqPos]])
		sv, _ := strconv.Atoi(st)
		group.Members = append(group.Members, SnapGroupMember{
			RelPath: e.relPath, Seq: sv, SeqText: st,
			Elements: append([]SnapEl(nil), e.els...),
		})
	}
	markCommonElements(&group)
	return group
}

// nonSepIndexOf 第 pos 个非分隔元素在元素数组中的下标（-1 = 越界）。
func nonSepIndexOf(els []SnapEl, pos int) int {
	n := 0
	for i := range els {
		if els[i].Kind == elKindSep {
			continue
		}
		if n == pos {
			return i
		}
		n++
	}
	return -1
}

// markCommonElements 群成员元素公共性终判（F4 呈现色类，规则定群群与结构群
// 同一实现）：全体成员非分隔元素数一致时逐位比对对齐取值——序号元素（对齐
// 取值=成员序号原文的首个命中位）保持 seq；公共位且非 author/workId/序号 →
// title（统一浅灰底）；非公共位（序号、豁免尾部码等变化位）→ plain。数不一致
// 时零作为（保持各自特征级类别）。
func markCommonElements(g *SnapGroup) {
	if len(g.Members) == 0 {
		return
	}
	count := -1
	for _, mem := range g.Members {
		n := 0
		for _, el := range mem.Elements {
			if el.Kind != elKindSep {
				n++
			}
		}
		if count < 0 {
			count = n
		} else if count != n {
			return
		}
	}
	if count <= 0 {
		return
	}
	common := make([]bool, count)
	for pos := 0; pos < count; pos++ {
		ref, started, same := "", false, true
		for _, mem := range g.Members {
			i := nonSepIndexOf(mem.Elements, pos)
			if i < 0 {
				same = false
				break
			}
			v := elValue(mem.Elements[i])
			if !started {
				ref, started = v, true
			} else if ref != v {
				same = false
				break
			}
		}
		common[pos] = same
	}
	for _, mem := range g.Members {
		seqMarked := false
		for i := range mem.Elements {
			el := &mem.Elements[i]
			switch el.Kind {
			case elKindSep, elKindAuthor, elKindWorkId:
				continue
			}
			if !seqMarked && mem.SeqText != "" && elValue(*el) == mem.SeqText {
				el.Kind = elKindSeq
				seqMarked = true
				continue
			}
			ord := countOfNonSepBefore(mem.Elements, i)
			if ord >= 0 && ord < count && common[ord] {
				el.Kind = elKindTitle
			} else {
				el.Kind = elKindPlain
			}
		}
	}
}

// countOfNonSepBefore 下标 i 之前（不含 i）的非分隔元素个数——该元素的位序。
func countOfNonSepBefore(els []SnapEl, i int) int {
	n := 0
	for k := 0; k < i; k++ {
		if els[k].Kind != elKindSep {
			n++
		}
	}
	return n
}

// attachSetNameCrossCheck 集名互证线索（批次二阶段3）：全部序列群（规则定群与结构群）
// 的分群键与库内作品集名全集精确匹配，命中时向该群全部成员补「集名互证」线索
// （去重：同文件已有同 Detail 的该类线索不重复加；lib 为 nil 或集名索引为空时零作为）。
func attachSetNameCrossCheck(snap *RecognitionSnapshot, lib *LibraryIndex) {
	if lib == nil || len(lib.WorkSetNames) == 0 {
		return
	}
	fileByPath := make(map[string]*SnapFile, len(snap.Files))
	for i := range snap.Files {
		fileByPath[snap.Files[i].RelPath] = &snap.Files[i]
	}
	for _, g := range snap.Groups {
		if _, ok := lib.WorkSetNames[g.Key]; !ok {
			continue
		}
		for _, mem := range g.Members {
			f := fileByPath[mem.RelPath]
			if f == nil {
				continue
			}
			dup := false
			for _, c := range f.Clues {
				if c.Kind == clueSetMatch && c.Detail == g.Key {
					dup = true
					break
				}
			}
			if !dup {
				f.Clues = append(f.Clues, SnapClue{Kind: clueSetMatch, Detail: g.Key})
			}
			// 无线索态（plain）升格有线索态；已识别/已有线索态仅补线索不动状态
			//（Clueful/Plain 统计在 aggregatePanorama 末段按终态汇总，此处改态即联动）
			if f.State == snapStatePlain {
				f.State = snapStateClueful
			}
		}
	}
}

// —— 集名候选墙（作品集识别强化 批次三阶段5）——

// 候选来源词汇（SnapCandidate.Source）。
const (
	// candSourceDir 目录候选：目录名含作品号等信号、整目录像一集。
	candSourceDir = "dir"
	// candSourceGroup 序列群候选：既有序列群全部（结构/规则/页级定群）。
	candSourceGroup = "group"
)

// SnapCandidate 集名候选卡：候选墙的单一数据源（前端卡墙只按本清单渲染）。
type SnapCandidate struct {
	// Key 集名候选：目录候选=目录名剥壳段（剥壳未命中时=目录名）、群候选=
	// 分群键（=公共前缀）。
	Key string `json:"key"`
	// Dir 候选所在目录键（相对库根的 '/' 目录路径）。
	Dir string `json:"dir"`
	// Source 来源："dir"（目录信号）|"group"（序列群）。
	Source string `json:"source"`
	// FormName 群来源时携带的命中形态标识（规则定群群为空）。
	FormName string `json:"formName,omitempty"`
	// Size 成员数。
	Size int `json:"size"`
	// Reason 人话理由（如「目录名含作品号 100423498，目录内 4 个文件已成一集」）。
	Reason string `json:"reason"`
	// CrossMatch A 档互证：候选键与库内集名全集精确相等。
	CrossMatch bool `json:"crossMatch,omitempty"`
	// ExistingSetID / ExistingSetName A 档「归入既有集」：候选键作品号与库内
	// 集名内作品号唯一相等时的既有集行 id 与集名（零/多命中留空）。confirmed
	// 分仓时 ExistingSetID 改取会话旁路簿记的确认值。
	ExistingSetID   int64  `json:"existingSetId,omitempty"`
	ExistingSetName string `json:"existingSetName,omitempty"`
	// ConfirmedSetName confirmed 分仓携带：确认动作写入成员集维度赋值的集名。
	ConfirmedSetName string `json:"confirmedSetName,omitempty"`
	// Members 候选成员（群来源复用群成员；目录来源=目录直接子文件、元素现算）。
	Members []SnapCandMember `json:"members"`
	// KeyElements 键元素池（F5 确认区元素插入 chips；决策2 A〔2026-10-20 拍板〕
	//：仅该卡键的各元素 + 所在目录名元素——池小精准）：群来源=首成员序号元素
	// 前的非分隔元素（键前缀），目录来源=目录名（剥壳内名）元素。
	KeyElements []SnapEl `json:"keyElements,omitempty"`
}

// SnapCandMember 候选成员：相对路径 + 序号原文 + 文件名元素数组（目录来源
// 现算，群来源复用群成员——特征级类别，无对齐终判）。
type SnapCandMember struct {
	RelPath  string   `json:"relPath"`
	SeqText  string   `json:"seqText,omitempty"`
	Elements []SnapEl `json:"elements,omitempty"`
}

// target 候选目标键（Source+Dir+Key 的稳定串）：会话忽略/确认清单的寻址形态
// （同目录同键的两形态结构群视为同一目标——忽略/确认按集名候选语义合并处置）。
func (c SnapCandidate) target() string {
	return c.Source + "\x00" + c.Dir + "\x00" + c.Key
}

// buildSnapshotCandidates 候选墙装配（元素8 入口）：三类来源合流为快照主清单
// （buildWorkSetCandidates），再按会话态目标键分仓——ignored 进
// IgnoredCandidates、confirmed 进 ConfirmedCandidates（带确认集名与既有集行
// id：确认集名取成员赋值的 collection〔确认动作的落点〕，既有 id 取会话旁路
// 簿记），分仓候选移出主清单。
func buildSnapshotCandidates(snap *RecognitionSnapshot, lib *LibraryIndex, in aggregateInput) {
	snap.Candidates = buildWorkSetCandidates(snap, lib)
	if len(in.ignoredTargets) == 0 && len(in.confirmedTargets) == 0 {
		return
	}
	kept := make([]SnapCandidate, 0, len(snap.Candidates))
	for _, cand := range snap.Candidates {
		switch target := cand.target(); {
		case in.confirmedTargets[target]:
			setName := confirmedCollectionOf(cand, in.assigns)
			cand.ConfirmedSetName = setName
			cand.ExistingSetID = in.confirmedSetIDs[setName]
			snap.ConfirmedCandidates = append(snap.ConfirmedCandidates, cand)
		case in.ignoredTargets[target]:
			snap.IgnoredCandidates = append(snap.IgnoredCandidates, cand)
		default:
			kept = append(kept, cand)
		}
	}
	snap.Candidates = kept
}

// confirmedCollectionOf 已确认候选的确认集名：确认动作对全员写集维度赋值
// （collection=setName），故取首个携带集维度赋值成员的取值；赋值此后被用户
// 清除时为空串（confirmed 分仓仍在场，呈现按已确认但集名缺席处理）。
func confirmedCollectionOf(cand SnapCandidate, assigns map[string]SessionAssignment) string {
	for _, mem := range cand.Members {
		if a, ok := assigns[mem.RelPath]; ok && a.Collection != "" {
			return a.Collection
		}
	}
	return ""
}

// groupKeyElements 群候选的键元素池（决策2 A：该卡键的各元素）：首成员序号
// 元素（kind=seq 首个非分隔位）前的非分隔元素——对齐构造保证全员一致。
func groupKeyElements(g *SnapGroup) []SnapEl {
	if len(g.Members) == 0 {
		return nil
	}
	var out []SnapEl
	for _, el := range g.Members[0].Elements {
		if el.Kind == elKindSep {
			continue
		}
		if el.Kind == elKindSeq {
			break
		}
		out = append(out, el)
	}
	return out
}

// buildWorkSetCandidates 候选主清单装配（纯函数）：① 目录候选
// （detectDirCandidates：目录名含作品号/整目录单一群键成集）；② 序列群全部
// （快照 Groups 即全集——结构/规则/页级定群同权出卡）；③ A 档库内集识别对
// 全部候选注记（matchExistingWorkSet：键∈集名全集=互证、键作品号与集名内
// 作品号唯一相等=归入既有集）。清单按目录、候选键、来源稳定排序。
func buildWorkSetCandidates(snap *RecognitionSnapshot, lib *LibraryIndex) []SnapCandidate {
	cands := make([]SnapCandidate, 0, len(snap.Groups))
	for _, g := range snap.Groups {
		reason := fmt.Sprintf("同目录 %d 个文件成序列群，公共前缀即集名候选", g.Size)
		if g.FormName != "" {
			reason += fmt.Sprintf("（%s形态）", g.FormName)
		} else if g.RuleID != "" {
			reason += fmt.Sprintf("（规则 %s 定群）", g.RuleID)
		}
		cand := SnapCandidate{
			Key: g.Key, Dir: g.DirRelKey, Source: candSourceGroup,
			FormName: g.FormName, Size: g.Size, Reason: reason,
			Members:     make([]SnapCandMember, 0, len(g.Members)),
			KeyElements: groupKeyElements(&g),
		}
		for _, mem := range g.Members {
			cand.Members = append(cand.Members, SnapCandMember{
				RelPath: mem.RelPath, SeqText: mem.SeqText, Elements: mem.Elements,
			})
		}
		cands = append(cands, cand)
	}
	cands = append(cands, detectDirCandidates(snap)...)
	for i := range cands {
		cands[i].CrossMatch, cands[i].ExistingSetID, cands[i].ExistingSetName =
			matchExistingWorkSet(cands[i].Key, lib)
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Dir != cands[j].Dir {
			return cands[i].Dir < cands[j].Dir
		}
		if cands[i].Key != cands[j].Key {
			return cands[i].Key < cands[j].Key
		}
		return cands[i].Source < cands[j].Source
	})
	return cands
}

// detectDirCandidates 目录候选探测（纯函数）：目录名含作品号（workNumberOf
// 保守提取命中）或目录内直接子文件已单一群键成集 → 整目录归集候选。成员=
// 目录直接子文件全部（不含子目录文件；区间与序号原文按结构形态表现算），
// 直接子文件 <2 不出候选；已被规则定群覆盖的目录（任一直接子文件是规则
// 定群群成员）不重复出——群来源已有候选卡。库根本身（RelKey 空）不出目录
// 候选。
func detectDirCandidates(snap *RecognitionSnapshot) []SnapCandidate {
	ruleGroupKeys := make(map[string]bool, len(snap.Groups))
	for _, g := range snap.Groups {
		if g.RuleID != "" {
			ruleGroupKeys[g.DirRelKey+"\x00"+g.Key] = true
		}
	}
	filesByDir := make(map[string][]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		filesByDir[f.DirRelKey] = append(filesByDir[f.DirRelKey], f)
	}
	var cands []SnapCandidate
	for _, dir := range snap.Dirs {
		if dir.RelKey == "" {
			continue
		}
		members := filesByDir[dir.RelKey]
		if len(members) < 2 {
			continue
		}
		ruleCovered := false
		groupKeys := make(map[string]bool, len(members))
		allGrouped := true
		for _, f := range members {
			if f.GroupKey == "" {
				allGrouped = false
				continue
			}
			groupKeys[f.GroupKey] = true
			if ruleGroupKeys[f.DirRelKey+"\x00"+f.GroupKey] {
				ruleCovered = true
				break
			}
		}
		if ruleCovered {
			continue
		}
		workNo := workNumberOf(dir.Name)
		singleGroup := allGrouped && len(groupKeys) == 1
		if workNo == "" && !singleGroup {
			continue
		}
		key := dir.Name
		if _, inner, ok := stripShell(dir.Name); ok {
			key = inner
		}
		var parts []string
		if workNo != "" {
			parts = append(parts, "目录名含作品号 "+workNo)
		}
		if singleGroup {
			parts = append(parts, fmt.Sprintf("目录内 %d 个文件已成一集", len(members)))
		} else {
			parts = append(parts, fmt.Sprintf("目录内 %d 个文件", len(members)))
		}
		cand := SnapCandidate{
			Key: key, Dir: dir.RelKey, Source: candSourceDir, Size: len(members),
			Reason:      strings.Join(parts, "，"),
			Members:     make([]SnapCandMember, 0, len(members)),
			KeyElements: dirNameElements(key),
		}
		for _, f := range members {
			mem := SnapCandMember{RelPath: f.RelPath}
			if base := BaseNameWithoutExt(f.FileName); base != "" {
				// 元素现算（特征级类别，无对齐终判）；序号原文=最后一个序号类
				// 数字元素（尾缀偏向，与形态表尾缀/直连主导的既有取号一致）。
				els := splitFilenameElements(base, elementHints(snap.RootPath, f.RelPath))
				for i := len(els) - 1; i >= 0; i-- {
					if els[i].Kind == elKindSeq {
						mem.SeqText = elValue(els[i])
						break
					}
				}
				mem.Elements = els
			}
			cand.Members = append(cand.Members, mem)
		}
		cands = append(cands, cand)
	}
	return cands
}

// matchExistingWorkSet A 档库内集识别（纯函数）：候选键对库内集名全集比对——
// 键与某集名精确相等 → 互证（CrossMatch）；键内作品号与集名内作品号
// （workNumberOf 保守提取）唯一相等 → 该集行 id 与集名（「归入既有集」），
// 零或多个命中留空。lib 缺席或集名索引为空时零命中。
func matchExistingWorkSet(key string, lib *LibraryIndex) (crossMatch bool, setID int64, setName string) {
	if lib == nil || len(lib.WorkSetNames) == 0 {
		return false, 0, ""
	}
	if _, ok := lib.WorkSetNames[key]; ok {
		crossMatch = true
	}
	workNo := workNumberOf(key)
	if workNo == "" {
		return crossMatch, 0, ""
	}
	hits := 0
	for name := range lib.WorkSetNames {
		if workNumberOf(name) != workNo {
			continue
		}
		hits++
		setID, setName = lib.WorkSetIDs[name], name
	}
	if hits == 1 {
		return crossMatch, setID, setName
	}
	return crossMatch, 0, ""
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
		if isGroupingRule(&rule) {
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

// panoramaViewOf 快照的前端投影：游标表为后端增量态，不出前端载荷；文件级与
// 目录级记录分别附加结构分段线索集（纯路径推导的呈现态数据，不入持久快照）。
func panoramaViewOf(snap *RecognitionSnapshot) RecognitionSnapshot {
	view := *snap
	view.DirMtimes = nil
	if len(snap.Files) > 0 {
		files := make([]SnapFile, len(snap.Files))
		for i, f := range snap.Files {
			f.StructClues = StructCluesOf(snap.RootPath, f.RelPath, f.FileName)
			files[i] = f
		}
		view.Files = files
	}
	if len(snap.Dirs) > 0 {
		dirs := make([]SnapDir, len(snap.Dirs))
		for i, d := range snap.Dirs {
			d.StructClues = DirStructCluesOf(snap.RootPath, d.RelKey)
			dirs[i] = d
		}
		view.Dirs = dirs
	}
	return view
}

// —— 识别快照差异集（生成规则后的强制验证轮差异高亮载荷）——

// SnapConclusion 文件级结论摘要（差异对比的比较单元）：呈现状态 + 终选声明 +
// 待裁决维度。终选声明的对比不含 RuleID（声明出处是溯源信息，同值换源不构成
// 用户可见的结论变化）。
type SnapConclusion struct {
	// State 呈现状态：identified / clueful / plain。
	State string `json:"state"`
	// Final 终选声明紧凑形态。
	Final []SnapDecl `json:"final,omitempty"`
	// PendingDims 待裁决维度。
	PendingDims []string `json:"pendingDims,omitempty"`
}

// SnapDiffEntry 单文件的结论差异条目：Before/After 为前后快照的结论摘要
// （Before 为 nil=新增文件，After 为 nil=消失文件，双全=结论变化）。
type SnapDiffEntry struct {
	RelPath string          `json:"relPath"`
	Before  *SnapConclusion `json:"before,omitempty"`
	After   *SnapConclusion `json:"after,omitempty"`
}

// SnapDiff 重识别差异集：文件级结论变化分类——过窄（差异里仍有未覆盖）与过泛
// （差异里错套同类）经本差异集呈现给用户核验。
type SnapDiff struct {
	// Changed 结论变化的文件（状态/终选声明/待裁决维度任一变化）。
	Changed []SnapDiffEntry `json:"changed"`
	// Added 新出现文件（前快照无此文件）；Removed 消失文件（新快照无此文件）。
	Added   []SnapDiffEntry `json:"added"`
	Removed []SnapDiffEntry `json:"removed"`
}

// conclusionOf 文件记录 → 结论摘要。
func conclusionOf(f SnapFile) *SnapConclusion {
	return &SnapConclusion{State: f.State, Final: f.Final, PendingDims: f.PendingDims}
}

// conclusionKey 结论摘要的规范比较键（状态 + 排序后的终选声明与待裁决维度）。
func conclusionKey(c *SnapConclusion) string {
	decls := make([]string, 0, len(c.Final))
	for _, decl := range c.Final {
		decls = append(decls, fmt.Sprintf("%s|%s|%s|%s", decl.Dimension, decl.Value, decl.Source, decl.RoleName))
	}
	sort.Strings(decls)
	dims := append([]string(nil), c.PendingDims...)
	sort.Strings(dims)
	return c.State + "\x00" + strings.Join(decls, "\x01") + "\x00" + strings.Join(dims, "\x01")
}

// ComputeSnapshotDiff 计算两份快照的文件级结论差异集（prev vs curr）：结论变化/
// 新增/消失三分类，条目按相对路径稳定排序；prev 为 nil 时返回全量新增形态
// （首轮识别无前序可差异）。切片字段一律非 nil（前端序列化契约）。
func ComputeSnapshotDiff(prev, curr *RecognitionSnapshot) *SnapDiff {
	diff := &SnapDiff{
		Changed: make([]SnapDiffEntry, 0),
		Added:   make([]SnapDiffEntry, 0),
		Removed: make([]SnapDiffEntry, 0),
	}
	if prev == nil {
		for _, f := range curr.Files {
			diff.Added = append(diff.Added, SnapDiffEntry{RelPath: f.RelPath, After: conclusionOf(f)})
		}
		return diff
	}
	prevByPath := make(map[string]SnapFile, len(prev.Files))
	for _, f := range prev.Files {
		prevByPath[f.RelPath] = f
	}
	currByPath := make(map[string]SnapFile, len(curr.Files))
	for _, f := range curr.Files {
		currByPath[f.RelPath] = f
	}
	for _, f := range curr.Files {
		before, ok := prevByPath[f.RelPath]
		if !ok {
			diff.Added = append(diff.Added, SnapDiffEntry{RelPath: f.RelPath, After: conclusionOf(f)})
			continue
		}
		afterKey := conclusionKey(conclusionOf(f))
		beforeKey := conclusionKey(conclusionOf(before))
		if afterKey == beforeKey {
			continue
		}
		b, a := conclusionOf(before), conclusionOf(f)
		diff.Changed = append(diff.Changed, SnapDiffEntry{RelPath: f.RelPath, Before: b, After: a})
	}
	for _, f := range prev.Files {
		if _, ok := currByPath[f.RelPath]; !ok {
			diff.Removed = append(diff.Removed, SnapDiffEntry{RelPath: f.RelPath, Before: conclusionOf(f)})
		}
	}
	sortDiffEntries(diff.Changed)
	sortDiffEntries(diff.Added)
	sortDiffEntries(diff.Removed)
	return diff
}

// sortDiffEntries 差异条目按相对路径稳定排序。
func sortDiffEntries(entries []SnapDiffEntry) {
	sort.Slice(entries, func(i, j int) bool { return entries[i].RelPath < entries[j].RelPath })
}

// diffCounts 差异集的三分类计数（事件摘要用）。
func diffCounts(diff *SnapDiff) (changed, added, removed int) {
	if diff == nil {
		return 0, 0, 0
	}
	return len(diff.Changed), len(diff.Added), len(diff.Removed)
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

	// 增量资格：上次快照的档案指纹与当前一致、且会话赋值修订号未变（赋值
	// 新增/修改/清除都会改修订号——赋值清除后其目录不再由赋值集锚定，靠修订号
	// 闸门防止旧结论被 mtime 游标复用）才可按目录 mtime 游标跳过
	prev := sess.Snapshot
	incremental := prev != nil && prev.ProfileDigest == digest && digest != "" &&
		prev.AssignmentRevision == sess.AssignRevision
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

	// 候选分仓判据（会话态目标键，元素8）：ignored/confirmed 清单与确认旁路
	ignoredTargets := make(map[string]bool, len(sess.IgnoredCandidates))
	for _, t := range sess.IgnoredCandidates {
		ignoredTargets[t] = true
	}
	confirmedTargets := make(map[string]bool, len(sess.ConfirmedCandidates))
	for _, t := range sess.ConfirmedCandidates {
		confirmedTargets[t] = true
	}

	snapshot := aggregatePanorama(aggregateInput{
		root: root, enumRes: enumRes,
		enabledRules: enabledRules, allRules: allRules, lib: lib,
		conclusions: conclusions, prev: prev, prevSkipped: prevSkipped,
		assigns: sess.Assignments, profileDigest: digest, assignRevision: sess.AssignRevision,
		ignoredTargets: ignoredTargets, confirmedTargets: confirmedTargets,
		confirmedSetIDs: sess.ConfirmedSetIDs,
	})

	// 差异集：与上一份快照的文件级结论对比（prev 为 nil 时为全量新增形态）——
	// 生成规则后的强制验证轮据此高亮「过窄仍有未覆盖、过泛错套」的差异
	var diff *SnapDiff
	if prev != nil {
		diff = ComputeSnapshotDiff(prev, snapshot)
	}

	if _, err := m.CompleteRecognitionWithDiff(root, snapshot, diff); err != nil {
		return err
	}
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseSave,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
	})
	changed, added, removed := diffCounts(diff)
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionDone, Root: root, Success: true,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
		Message: fmt.Sprintf("识别完成：共 %d 个文件（已识别 %d、有线索 %d、无线索 %d），序列群 %d，待裁决文件 %d；与上次差异：变更 %d、新增 %d、消失 %d",
			snapshot.Stats.TotalFiles, snapshot.Stats.Identified, snapshot.Stats.Clueful,
			snapshot.Stats.Plain, snapshot.Stats.GroupCount, snapshot.Stats.PendingFiles,
			changed, added, removed),
	})
	return nil
}

// indexSize 谓词索引的名字总量（进度呈现用；nil 索引为 0）。
func indexSize(lib *LibraryIndex) int {
	if lib == nil {
		return 0
	}
	return len(lib.AuthorNames) + len(lib.TagNames) + len(lib.WorkSetNames)
}

// allRulesWithBuiltins 全部规则（含禁用）：v4 起内置规则退休为例子模板、
// 不再参与求值与全景呈现，全部规则即档案规则（元素6 按此序呈现）。
func allRulesWithBuiltins(profile *RuleProfile) []RuleEntry {
	if profile == nil {
		return nil
	}
	return profile.Rules
}
