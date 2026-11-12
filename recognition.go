// 全量识别引擎：识别干跑（零任务产出、零弹问的纯求值）的合并求值单一实现、
// 库内谓词索引构造、七类识别全景元素与集名候选墙（批次三阶段5）聚合与快照
// 裁剪模型、增量游标编排。
package main

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// Generalized 该赋值是否已参与泛化（2026-10-25 用户裁决：废除「泛化即接
	// 管」消费剥离——赋值全量保留与规则声明双源互证，显式维度覆盖规则终选
	// 的合并语义不变；本标记仅作「已参与泛化」的谱系记录，不再意味着维度
	// 被剥离或删除）。
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
	// rawDecls 为提取段原始声明（合成前中间产物）：预览基座规则相关件的铸造
	// 原料，快照与呈现不消费（本类型为进程内求值产物，不经序列化）。
	rawDecls []MetadataDeclaration
}

// MergedRecognition 对一批扫描条目执行「档案规则 ∪ 会话赋值」合并求值：
// 先按规则流水线求值（注入库内谓词索引；扫描条目构造与 Create 期 buildScanFiles
// 同构），再逐文件合成终选与待裁决，最后会话赋值按维度覆盖终选声明（特异性
// 结果压过普适规则）。识别干跑与会话模式的任务创建共用本入口（合并逻辑唯一
// 实现，两处结论同源）。聚合视图（同目录兄弟/各层子女/子目录数）按本批 files
// 自聚合——全集语义调用方用 MergedRecognitionWithTree 传全量树。
func MergedRecognition(files []ScanFile, rules []RuleEntry, lib *LibraryIndex, assigns map[string]SessionAssignment) []FileConclusion {
	return MergedRecognitionWithTree(files, files, rules, lib, assigns)
}

// evalWorkerCount 求值相并行度（工作子数）的单一取值点：进程默认
// GOMAXPROCS（=逻辑核数），不设用户配置面（未上线功能不背配置债）。var
// 形态供测试覆写注入——模式等价锚以同一实现的 1 工作子与多工作子两模式互证。
// 工作子 panic 沿串行同款冒泡（识别干跑的串行循环 panic 同样崩溃插件进程），
// 全链严禁 recover：识别结论静默缺失比崩溃更危险。
var evalWorkerCount = func() int { return runtime.GOMAXPROCS(0) }

// evalFileChunkSize 逐文件结论循环的分块大小（块约百余个文件）：块是并行区2
// 的取工单位，块内下标连续、块间不重叠；结论按下标写入预分配切片——输入序
// 保持，汇流消费方（快照装配与命中预览）按文件序索引取用，语义不变。
const evalFileChunkSize = 128

// evalProgressStride 求值相进度步长：逐文件结论循环每累计完成这么多文件向
// 进度钩子发一拍（大库求值分钟级，相内按步长多拍推送）。拍频与工作子数
// 无关——计数按块完成原子累计，跨步长才发拍。
const evalProgressStride = 2000

// MergedRecognitionWithTree 带全量聚合视图的合并求值：treeFiles 为目录树全量
// 文件清单（识别增量批时 evalFiles 仅含新目录文件——跨被跳目录的各层子女/
// 子目录数必须按全量树聚合，否则目录终结谓词（childrenAllFormat/childDirCount
// 等）在增量轮少计误判）。同目录兄弟仍按本批聚合：mtime 游标按目录整跳，本批
// 目录的文件集完整。onEvalProgress 为可选求值进度钩子（变参，缺省零进度
// 拍）：每跨一个步长的累计完成数发一拍，调用串行、done 单调不减、total 恒
// 为本批文件数——识别干跑以此转发求值相内进度事件，其余调用点缺省直通。
func MergedRecognitionWithTree(files, treeFiles []ScanFile, rules []RuleEntry, lib *LibraryIndex, assigns map[string]SessionAssignment, onEvalProgress ...func(done, total int)) []FileConclusion {
	var hook func(done, total int)
	if len(onEvalProgress) > 0 {
		hook = onEvalProgress[0]
	}
	return mergedRecognition(files, treeFiles, rules, lib, assigns, nil, nil, hook)
}

// MergedRecognitionOnSubstrate 预览基座供给形态的合并求值：求值面与聚合视图
// 均取基座铸造清单（sub.files——与预览通路自快照现场重建的清单逐字节同构），
// 兄弟表/目录树聚合/文件名元素拆分复用基座预铸件；规则求值本身（含结构群
// 前置对齐——对齐候选依赖规则定群段，随草稿规则集现场计算）与现行路径同一
// 实现。sub 须非 nil 且与求值面同源（调用方守卫：基座未就位即回退
// MergedRecognition 现场重建）。
func MergedRecognitionOnSubstrate(sub *evalSubstrate, rules []RuleEntry, lib *LibraryIndex, assigns map[string]SessionAssignment) []FileConclusion {
	return mergedRecognition(sub.files, sub.files, rules, lib, assigns, sub, nil, nil)
}

// mergedRecognition 合并求值的唯一实现（公开入口 MergedRecognition /
// MergedRecognitionWithTree / MergedRecognitionOnSubstrate 均经此）：
//   - sub 为基座规则无关件供给（nil=聚合表与文件名元素拆分现场计算，现行
//     路径；非 nil 时 sub.files 须即求值面——基座铸造清单）；
//   - harvest 为识别轮的求值中间产物回收槽（nil=不回收）；
//   - onEvalProgress 为可选进度钩子（nil=零进度拍）。
func mergedRecognition(files, treeFiles []ScanFile, rules []RuleEntry, lib *LibraryIndex, assigns map[string]SessionAssignment, sub *evalSubstrate, harvest *evalHarvest, onEvalProgress func(done, total int)) []FileConclusion {
	// 同目录兄弟全集按「父目录」批量构造（与 RunPipeline 内部同键同序）；基座
	// 供给时直接复用预铸表（求值面=树面=铸造清单，聚合内容与现场计算同键同
	// 序）；单文件重算通道（recomputeFileDecision）经磁盘重扫预填兄弟全集后
	// 共用同一单文件合并实现
	var siblingsByDir, treeSiblings map[string][]string
	var childDirsByParent map[string]map[string]struct{}
	if sub != nil {
		siblingsByDir, treeSiblings, childDirsByParent = sub.siblingsByDir, sub.siblingsByDir, sub.childDirsByParent
	} else {
		siblingsByDir = make(map[string][]string, len(files))
		for _, file := range files {
			key := dirGroupKey(file.DirNames)
			siblingsByDir[key] = append(siblingsByDir[key], file.FileName)
		}
		// 各层子女与子目录数按全量树聚合（v4 目录终结谓词消费；与 RunPipeline
		// 内部同键同序——AncestorFiles 缺失时子女类谓词保守不命中）
		treeSiblings = make(map[string][]string, len(treeFiles))
		childDirsByParent = make(map[string]map[string]struct{})
		for _, file := range treeFiles {
			key := dirGroupKey(file.DirNames)
			treeSiblings[key] = append(treeSiblings[key], file.FileName)
			for d := 0; d+1 < len(file.DirNames); d++ {
				parentKey := dirGroupKey(file.DirNames[:d+1])
				set, ok := childDirsByParent[parentKey]
				if !ok {
					set = make(map[string]struct{})
					childDirsByParent[parentKey] = set
				}
				set[file.DirNames[d+1]] = struct{}{}
			}
		}
	}
	// 结构群前置对齐每目录只算一次，成员按相对路径登记，各单文件结论取用
	structuralByRel := precomputeStructuralMembers(files, rules, lib, siblingsByDir, sub)
	if harvest != nil {
		harvest.structuralByRel = structuralByRel
	}
	// 会话赋值就近解析预计算（目录粒度表示的读路径）：目录集=求值树全部文件
	// 的所属目录键，一次 DP 出每目录解析结果（并行区前完成，构造后只读共享）
	// ——逐文件 O(1) 取用，文件例外条目逐维覆盖。旧形态（全文件键）经同一
	// 解析器原生正确解读：无目录键=逐维上溯恒落空=与直查逐字节同义。
	var assignIdx *assignmentDirIndex
	if len(assigns) > 0 {
		dirKeys := make(map[string]struct{}, len(treeFiles))
		for _, file := range treeFiles {
			dirKeys[dirRelKeyOf(file.RelPath)] = struct{}{}
		}
		for _, file := range files {
			dirKeys[dirRelKeyOf(file.RelPath)] = struct{}{}
		}
		dirKeyList := make([]string, 0, len(dirKeys))
		for dirKey := range dirKeys {
			dirKeyList = append(dirKeyList, dirKey)
		}
		assignIdx = newAssignmentDirIndex(assigns, dirKeyList)
	}
	// 并行区2：逐文件结论按文件分块并行——每文件求值输入装配为兄弟表/聚合表
	// 的只读查找、结论彼此独立，按下标写入预分配切片保输入序。共享输入
	// （兄弟表/聚合表/结构群成员表/规则序列/谓词索引/会话赋值预计算表）在此
	// 只读（零锁纪律）。
	conclusions := make([]FileConclusion, len(files))
	evaluateFileAt := func(i int) {
		file := files[i]
		in := &MatchInput{
			FileName: file.FileName, DirNames: file.DirNames, RelPath: file.RelPath,
			Siblings:               siblingsByDir[dirGroupKey(file.DirNames)],
			AncestorFiles:          ancestorFilesLevels(file.DirNames, treeSiblings),
			AncestorChildDirCounts: ancestorChildDirCounts(file.DirNames, childDirsByParent),
		}
		var assign *SessionAssignment
		if assignIdx != nil {
			// 就近解析取值：解析非空（任一维度有供值条目）才参与合并覆盖，
			// 与旧直查「有条目即覆盖」等值——空赋值对结论本就是空覆盖
			if a := assignIdx.resolveFile(file.RelPath); len(a.coveredDimensions()) > 0 {
				assign = &a
			}
		}
		conclusions[i] = mergedFileConclusionWithStructural(in, rules, lib, assign, structuralByRel[file.RelPath], sub)
	}
	// 进度拍发器（钩子缺省时为空操作）：块完成计数原子累计，每跨一个步长发
	// 一拍 done=累计完成数（块边界值，非步长整数倍）；拍调用串行且 done 单调
	// 不减——多工作子乱序完成块时后到的较小累计值直接丢弃（拍是节流采样，
	// 不承载精确对账）。
	var onChunkDone func(int)
	if onEvalProgress != nil {
		hook := onEvalProgress
		var doneCount atomic.Int64
		var beatMu sync.Mutex
		lastBeat := 0
		onChunkDone = func(n int) {
			prev := doneCount.Add(int64(n))
			if prev/evalProgressStride <= (prev-int64(n))/evalProgressStride {
				return
			}
			beatMu.Lock()
			if int(prev) > lastBeat {
				lastBeat = int(prev)
				hook(int(prev), len(files))
			}
			beatMu.Unlock()
		}
	} else {
		onChunkDone = func(int) {}
	}
	if chunks := (len(files) + evalFileChunkSize - 1) / evalFileChunkSize; chunks > 1 && evalWorkerCount() > 1 {
		// 工作子池取块：原子游标取块号，块间下标不重叠（零共享写），单工作子
		// 或单块时同一求值实现在本 goroutine 内联执行（两模式同一实现）。
		workers := min(evalWorkerCount(), chunks)
		var cursor atomic.Int64
		var wg sync.WaitGroup
		wg.Add(workers)
		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for {
					c := int(cursor.Add(1)) - 1
					if c >= chunks {
						return
					}
					start := c * evalFileChunkSize
					end := min(start+evalFileChunkSize, len(files))
					for i := start; i < end; i++ {
						evaluateFileAt(i)
					}
					onChunkDone(end - start)
				}
			}()
		}
		wg.Wait()
		return conclusions
	}
	// 串行内联路径与并行工作子同一节流形态：按块推进并逐块报完成数（拍点与
	// 并行模式的块边界一致）。
	for start := 0; start < len(files); start += evalFileChunkSize {
		end := min(start+evalFileChunkSize, len(files))
		for i := start; i < end; i++ {
			evaluateFileAt(i)
		}
		onChunkDone(end - start)
	}
	return conclusions
}

// mergedFileConclusionOf 单文件「档案规则 ∪ 会话赋值」合并求值结论（无批量
// 预计算形态）：结构群补位在函数内现场对齐，其余与批量入口同一实现——
// 执行期单文件重算通道经此与识别干跑共用合并逻辑。
func mergedFileConclusionOf(in *MatchInput, rules []RuleEntry, lib *LibraryIndex, assign *SessionAssignment) FileConclusion {
	return mergedFileConclusionWithStructural(in, rules, lib, assign, nil, nil)
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
// （recomputeFileDecision，消费任务级赋值快照）共用本函数。sub 为基座规则
// 无关件供给（nil=现行现场计算形态）：结构群补位与规则定群群成员的元素拆分
// 复用预铸件。
func mergedFileConclusionWithStructural(in *MatchInput, rules []RuleEntry, lib *LibraryIndex, assign *SessionAssignment, pre *structuralMemberInfo, sub *evalSubstrate) FileConclusion {
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	decision := FileDecision{RelPath: in.RelPath, FileName: in.FileName}
	group, groupRuleID := runGroupStage(rules, in, lib, ctx)
	decision.Group = group
	if group == nil {
		if pre == nil {
			pre = alignStructuralForFile(in, rules, lib, sub)
		}
		if pre != nil {
			ctx.Group = pre.analysis
		}
	} else {
		pre = nil // 规则群优先：已定群文件不再取结构群身份
	}
	runExtractionStage(&decision, rules, in, lib, ctx)
	c := FileConclusion{
		RelPath: in.RelPath, FileName: in.FileName, DirRelKey: dirRelKeyOf(in.RelPath),
		Group: decision.Group, MatchedRuleIDs: decision.MatchedRuleIDs,
		Final: nil, Pending: nil,
	}
	// 提取段原始声明在合成前留存（合成不改动入参切片，引用即安全）
	c.rawDecls = decision.Declarations
	final, pending := Synthesize(decision.Declarations, AggregateSynthesis(rules))
	if assign != nil {
		final, pending = applyAssignmentToConclusion(final, pending, assign)
	}
	c.Final, c.Pending = final, pending
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
			// 成员的元素呈现不依赖它）。基座在场取预铸拆分（副本——元素类别
			// 会被群公共性终判改写，缓存原件不得外流），未命中现场拆分。
			if els := sub.elementsOf(in.RelPath); els != nil {
				c.GroupElements = els
			} else {
				c.GroupElements = splitFilenameElements(BaseNameWithoutExt(in.FileName), dirNameHints(in.DirNames))
			}
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

// —— 会话级预览求值基座（识别轮完成时铸造，挂靠会话缓存条目）——

// evalSubstrate 预览求值的规则无关预铸件（基座规则无关层的供给视图）：文件
// 清单重建、同目录兄弟表、目录树子目录聚合与文件名元素拆分——只依赖文件集
// 身份（求值输入的原始路径形态），与规则集、会话赋值均无关。预览通路命中
// 基座时经 MergedRecognitionOnSubstrate / 逐文件对位取用，消灭每次调用的
// 整库重建与元素拆分；未命中时求值内部现场计算（现行路径），行为一致。
// 铸造后整体只读共享（元素取用发副本），并发读零锁。
type evalSubstrate struct {
	// files 求值清单（快照文件序重建，与预览通路现行现场重建逐字节同构）。
	files []ScanFile
	// siblingsByDir 同目录兄弟全集（键=目录名序列的 NUL 连接，与求值内部
	// 同键同序；求值面=树面=全量清单，一张表同供两面）。
	siblingsByDir map[string][]string
	// childDirsByParent 各层祖先目录的直接子目录名集合（v4 目录终结谓词消费）。
	childDirsByParent map[string]map[string]struct{}
	// elementsByRel 文件名元素拆分（相对路径 → 去扩展名正文元素数组；提示=
	// 目录名+剥壳变体，与求值内部拆分同口径）。
	elementsByRel map[string][]SnapEl
}

// elementsOf 取文件的预铸元素拆分副本（未命中返回 nil=调用方现场拆分）。恒发
// 副本：元素类别会被群公共性终判（markCommonElements）改写，缓存原件不得外流。
func (s *evalSubstrate) elementsOf(relPath string) []SnapEl {
	if s == nil {
		return nil
	}
	els, ok := s.elementsByRel[relPath]
	if !ok || len(els) == 0 {
		return nil
	}
	return append([]SnapEl(nil), els...)
}

// evalHarvest 求值相中间产物的回收槽（识别轮铸造基座规则相关件的原料；
// nil=不回收）。求值实现填充，调用方只读。
type evalHarvest struct {
	// structuralByRel 结构群前置对齐结果（本轮求值面的成员登记；增量轮只含
	// 变更目录条目）。
	structuralByRel map[string]*structuralMemberInfo
}

// previewRuleSegment 单文件的规则段结论（与赋值无关——赋值只在合成后按维度
// 覆盖终选，不改写规则段产物）：提取段原始声明（合成前）、命中规则与定群
// 产物。识别快照只存呈现裁剪形态，本件为基座新铸中间产物（不入快照）。
type previewRuleSegment struct {
	declarations     []MetadataDeclaration
	matchedRuleIDs   []string
	group            *GroupAnalysis
	groupRuleID      string
	groupSeqText     string
	groupElements    []SnapEl
	groupFormName    string
	groupFormKeyExpr string
	groupFormSeqExpr string
}

// previewRuleLayer 基座规则相关件：结构群对齐结果与逐文件规则段结论。守卫键=
// 档案指纹+求值器构建标识+文件集身份（与识别增量闸门同款等值比较，赋值修订
// 号不入键——赋值只在合成后按维度覆盖终选，规则段结论与赋值无关）；libDigest
// 为铸造时点库内名单索引的内容键（阶段5：谓词类规则的命中面吃名单，名单换代
// 即令规则段失真——赋值增量重算按此守卫回退整库重求值）；coveredDirKeys
// 登记条目在场的目录键——增量轮上轮基座缺席时只部分携带，消费方按目录缺席
// 回退整段重算。
type previewRuleLayer struct {
	profileDigest    string
	evaluatorBuildID string
	libDigest        string
	structuralByRel  map[string]*structuralMemberInfo
	segmentsByRel    map[string]*previewRuleSegment
	coveredDirKeys   map[string]bool
}

// libDigestMatches 库内名单身份守卫：铸造时点名单内容键与当前一致（nil 名单
// 两侧空串等值——上下文缺席时求值与线索探测本就不消费名单）。
func (l *previewRuleLayer) libDigestMatches(libDigest string) bool {
	if l == nil {
		return false
	}
	return l.libDigest == libDigest
}

// guardMatches 规则相关件守卫（与识别增量闸门的增量资格判定同款等值比较形态）：
// 档案指纹一致，且求值器构建标识一致（当前构建未打标=空串按缺席放行，与闸门
// 空串语义一致）。
func (l *previewRuleLayer) guardMatches(profileDigest, evaluatorBuildID string) bool {
	if l == nil {
		return false
	}
	if l.profileDigest != profileDigest {
		return false
	}
	return evaluatorBuildID == "" || l.evaluatorBuildID == evaluatorBuildID
}

// previewFoundation 会话级预览求值基座：识别轮完成时铸造的整库求值中间产物
// 进程驻留（纯内存态：不入键值存储、不入识别快照、不改会话信封结构），挂靠
// 会话缓存条目同生命周期（条目被逐出/替换即随之消亡）。snapshot 为铸造来源
// 快照实例（指针等值即文件集未变的零成本快路径），fileSetKey 为其文件集身份
// 键（阶段5：赋值写等轻保存重建缓存条目后快照实例换代而文件集未变，值守卫
// 由此放行携带——见 matchesSnapshot）；indep 为规则无关层（本阶段三条建议卡
// 路径消费），rule 为规则相关件（守卫键分层，后续阶段的求值复用增量消费）。
type previewFoundation struct {
	snapshot   *RecognitionSnapshot
	fileSetKey string
	indep      *evalSubstrate
	rule       *previewRuleLayer
}

// snapshotFileSetKey 快照文件集身份键：文件相对路径有序清单的内容哈希（计数
// 前缀防异长清单前缀碰撞；恒非空串——零文件快照也有 "0" 前缀）。基座两层的
// 全部求值输入（求值清单/兄弟表/子目录聚合/元素拆分/规则段结论）都由该清单
// 唯一决定——键等值即基座与新快照的求值面同构。
func snapshotFileSetKey(snap *RecognitionSnapshot) string {
	h := fnv.New128a()
	if snap != nil {
		h.Write([]byte(strconv.Itoa(len(snap.Files))))
		h.Write([]byte{0})
		for i := range snap.Files {
			h.Write([]byte(snap.Files[i].RelPath))
			h.Write([]byte{0})
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// matchesSnapshot 基座与快照的匹配判定（阶段5 携带/守卫改造）：铸造来源快照
// 指针等值（识别轮铸后未换代的零成本快路径）或文件集身份键等值（赋值写等
// 轻保存重建缓存条目后，新快照实例与铸造来源内容同构）。安全依据=规则段结论
// 与赋值无关（赋值只在合成后按维度覆盖终选，recognition.go:349-351 核码事实）
// ——赋值修订号不入守卫键。
func (f *previewFoundation) matchesSnapshot(snap *RecognitionSnapshot) bool {
	if f == nil || f.snapshot == nil || snap == nil {
		return false
	}
	if f.snapshot == snap {
		return true
	}
	return f.fileSetKey != "" && f.fileSetKey == snapshotFileSetKey(snap)
}

// libraryIndexDigest 库内名单索引的内容键（阶段5）：三轨名字集的有序指纹——
// 名单是求值（谓词类规则）与线索探测/集名注记的消费面，名字集等值即索引求值
// 等价。行 id（WorkSetIDs）不入键：真实库行 id 稳定，测试桩逐拉递增，且行 id
// 只在「归入既有集」注记呈现（名字集未变时注记语义不变）。
func libraryIndexDigest(lib *LibraryIndex) string {
	if lib == nil {
		return ""
	}
	h := fnv.New128a()
	for _, names := range []map[string]struct{}{lib.AuthorNames, lib.TagNames, lib.WorkSetNames} {
		sorted := make([]string, 0, len(names))
		for name := range names {
			sorted = append(sorted, name)
		}
		sort.Strings(sorted)
		h.Write([]byte(strconv.Itoa(len(sorted))))
		h.Write([]byte{0})
		for _, name := range sorted {
			h.Write([]byte(name))
			h.Write([]byte{0})
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// buildEvalSubstrate 规则无关层铸造：自快照文件序全量重建求值清单与两张聚合
// 表、预拆全库文件名元素——与预览通路现行现场重建（快照文件 → ScanFile 清单
// + 求值内聚合 + 元素拆分）同键同序同口径，纯组织无求值。
func buildEvalSubstrate(snapshot *RecognitionSnapshot) *evalSubstrate {
	files := make([]ScanFile, len(snapshot.Files))
	for i := range snapshot.Files {
		f := &snapshot.Files[i]
		files[i] = ScanFile{FileName: f.FileName, DirNames: ExtractDirLevels(f.RelPath), RelPath: f.RelPath}
	}
	siblingsByDir := make(map[string][]string, len(files))
	childDirsByParent := make(map[string]map[string]struct{})
	for _, file := range files {
		key := dirGroupKey(file.DirNames)
		siblingsByDir[key] = append(siblingsByDir[key], file.FileName)
		for d := 0; d+1 < len(file.DirNames); d++ {
			parentKey := dirGroupKey(file.DirNames[:d+1])
			set, ok := childDirsByParent[parentKey]
			if !ok {
				set = make(map[string]struct{})
				childDirsByParent[parentKey] = set
			}
			set[file.DirNames[d+1]] = struct{}{}
		}
	}
	elementsByRel := make(map[string][]SnapEl, len(files))
	for _, file := range files {
		elementsByRel[file.RelPath] = splitFilenameElements(BaseNameWithoutExt(file.FileName), dirNameHints(file.DirNames))
	}
	return &evalSubstrate{
		files:             files,
		siblingsByDir:     siblingsByDir,
		childDirsByParent: childDirsByParent,
		elementsByRel:     elementsByRel,
	}
}

// ruleSegmentOf 单文件结论 → 规则段结论条目（引用拷贝：结论为本轮私有产物，
// 合成不改动提取段原始声明切片）。
func ruleSegmentOf(c *FileConclusion) *previewRuleSegment {
	return &previewRuleSegment{
		declarations:     c.rawDecls,
		matchedRuleIDs:   c.MatchedRuleIDs,
		group:            c.Group,
		groupRuleID:      c.GroupRuleID,
		groupSeqText:     c.GroupSeqText,
		groupElements:    c.GroupElements,
		groupFormName:    c.GroupFormName,
		groupFormKeyExpr: c.GroupFormKeyExpr,
		groupFormSeqExpr: c.GroupFormSeqExpr,
	}
}

// coveredDirsOf 规则段条目在场的目录键集（按条目相对路径归属目录推导）。
func coveredDirsOf(segments map[string]*previewRuleSegment) map[string]bool {
	covered := make(map[string]bool, len(segments))
	for rel := range segments {
		covered[dirRelKeyOf(rel)] = true
	}
	return covered
}

// freshRuleLayer 规则相关件全量铸造：全量重算轮（首轮/档案或赋值修订失配/求值
// 器换版）的结论覆盖全库，规则段结论与结构群对齐结果整体建层。
func freshRuleLayer(harvest *evalHarvest, conclusions []FileConclusion, liveRels map[string]bool, profileDigest, evaluatorBuildID, libDigest string) *previewRuleLayer {
	segments := make(map[string]*previewRuleSegment, len(conclusions))
	for i := range conclusions {
		c := &conclusions[i]
		if !liveRels[c.RelPath] {
			continue
		}
		segments[c.RelPath] = ruleSegmentOf(c)
	}
	return &previewRuleLayer{
		profileDigest:    profileDigest,
		evaluatorBuildID: evaluatorBuildID,
		libDigest:        libDigest,
		structuralByRel:  harvest.structuralByRel,
		segmentsByRel:    segments,
		coveredDirKeys:   coveredDirsOf(segments),
	}
}

// carryRuleLayer 规则相关件增量携带：增量轮结论只覆盖变更目录（本轮求值面），
// 以上轮条目为底——变更目录条目整目录替换为本轮结论、快照已消失文件的条目
// 剔除、本轮结论合入；结构群对齐结果同口径（回收槽只含本轮求值面条目）。
func carryRuleLayer(prev *previewRuleLayer, harvest *evalHarvest, conclusions []FileConclusion, evalScan []ScanFile, liveRels map[string]bool, profileDigest, evaluatorBuildID, libDigest string) *previewRuleLayer {
	changedDirs := make(map[string]bool, len(evalScan))
	for _, f := range evalScan {
		changedDirs[dirRelKeyOf(f.RelPath)] = true
	}
	segments := make(map[string]*previewRuleSegment, len(prev.segmentsByRel))
	for rel, seg := range prev.segmentsByRel {
		if changedDirs[dirRelKeyOf(rel)] || !liveRels[rel] {
			continue
		}
		segments[rel] = seg
	}
	structural := make(map[string]*structuralMemberInfo, len(prev.structuralByRel))
	for rel, info := range prev.structuralByRel {
		if changedDirs[dirRelKeyOf(rel)] || !liveRels[rel] {
			continue
		}
		structural[rel] = info
	}
	for i := range conclusions {
		c := &conclusions[i]
		if !liveRels[c.RelPath] {
			continue
		}
		segments[c.RelPath] = ruleSegmentOf(c)
	}
	maps.Copy(structural, harvest.structuralByRel)
	return &previewRuleLayer{
		profileDigest:    profileDigest,
		evaluatorBuildID: evaluatorBuildID,
		libDigest:        libDigest,
		structuralByRel:  structural,
		segmentsByRel:    segments,
		coveredDirKeys:   coveredDirsOf(segments),
	}
}

// mintPreviewFoundation 识别完成处铸造基座：规则无关层自新快照全量组织；规则
// 相关层按增量资格（识别增量闸门通过且上轮基座在场且守卫键一致）携带上轮
// 条目、只更新变更目录，否则全量铸造。浅层组织+引用拷贝，无重求值。
func mintPreviewFoundation(snapshot *RecognitionSnapshot, harvest *evalHarvest, conclusions []FileConclusion, evalScan []ScanFile, profileDigest, evaluatorBuildID, libDigest string, prev *previewFoundation, incremental bool) *previewFoundation {
	liveRels := make(map[string]bool, len(snapshot.Files))
	for i := range snapshot.Files {
		liveRels[snapshot.Files[i].RelPath] = true
	}
	var rule *previewRuleLayer
	if incremental && prev != nil && prev.rule != nil && prev.rule.guardMatches(profileDigest, evaluatorBuildID) {
		rule = carryRuleLayer(prev.rule, harvest, conclusions, evalScan, liveRels, profileDigest, evaluatorBuildID, libDigest)
	} else {
		rule = freshRuleLayer(harvest, conclusions, liveRels, profileDigest, evaluatorBuildID, libDigest)
	}
	return &previewFoundation{snapshot: snapshot, fileSetKey: snapshotFileSetKey(snapshot), indep: buildEvalSubstrate(snapshot), rule: rule}
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
	// AssignmentRevision 求值依据的会话赋值修订号——赋值保存触发的刷新轮在
	// 修订号失配时走赋值增量重算（tryAssignmentIncremental）或整库重算，赋值
	// 被清除后其目录不再由赋值集锚定，修订号闸门保证旧结论不被 mtime 游标
	// 复用；EvaluatorBuildID 铸本快照的识别求值器构建标识（自身插件清单的
	// buildId，空串=未打标构建）：识别内核随构建产物演进，求值器身份不一致
	// 的旧结论失去 mtime 游标复用资格（增量闸门与恢复对账按等值判定）。
	ProfileDigest      string `json:"profileDigest"`
	AssignmentRevision int64  `json:"assignmentRevision"`
	EvaluatorBuildID   string `json:"evaluatorBuildId"`

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
	Dimension   string   `json:"dimension"`
	Value       string   `json:"value"`
	Source      string   `json:"source"`
	RoleName    string   `json:"roleName,omitempty"`
	RuleID      string   `json:"ruleId,omitempty"`
	AlsoRuleIDs []string `json:"alsoRuleIds,omitempty"`
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
	// HitCount 命中数：提取段按文件计（记入 MatchedRuleIDs 的文件数），定群
	// 规则不进提取段，按定群成员文件数计（规则群卡成员数汇总，复用组含）。
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
	// assigns 会话赋值（目录条目+文件例外共居；求值合并经预计算表、死路径
	// 判定与元素7 标记经就近解析消费）。
	assigns map[string]SessionAssignment
	// profileDigest 本轮档案指纹；assignRevision 本轮会话赋值修订号；
	// evaluatorBuildID 本轮求值器构建标识（铸入快照，增量与恢复对账的凭据）。
	profileDigest    string
	assignRevision   int64
	evaluatorBuildID string
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
		EvaluatorBuildID: in.evaluatorBuildID,
		DirMtimes: in.enumRes.DirMtimes,
		// 序列化契约：快照经 JSON 落会话 KV 并直接驱动前端渲染，切片字段
		// 一律非 nil（nil 序列化为 null，前端 .length 即崩）；Pending 与
		// StaleAssignments 带 omitempty（缺席由前端守卫兜底）；标量字符串
		//（如 EvaluatorBuildID）恒序列化（空串即未打标语义），无 null 风险。
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
			// 目录链取真实祖先名（RelKey 按 '/' 拆层——目录名不含分隔符）：
			// 组合规则的门段依赖祖先层求值（如「Artist 壳目录之下」），占位
			// 空串祖先会令门段保守失败、含义面收窄。
			node.Meanings, node.MeaningRuleID = dirMeaningReplay(in.enabledRules, strings.Split(dir.RelKey, "/"), in.lib)
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
	// 会话赋值就近解析预计算：目录集=快照目录树键（enumRes.Dirs），死路径
	// 判定与元素7 Assigned 标记共用（聚合阶段串行，构造后只读）
	snapDirKeys := make([]string, 0, len(in.enumRes.Dirs))
	for _, dir := range in.enumRes.Dirs {
		snapDirKeys = append(snapDirKeys, dir.RelKey)
	}
	assignIdx := newAssignmentDirIndex(in.assigns, snapDirKeys)

	groups := make(map[string]*SnapGroup)
	reusedGroups := make(map[string]bool)

	appendConclusion := func(c *FileConclusion) {
		file := snapFileOf(c, in.lib, assignIdx)
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

	// 跳过复用目录的序列群按目录整组复原：目录级跳过=目录 mtime 未变、文件集
	// 未变，该目录全部群卡（规则定群与结构群）从旧快照整组拷贝。文件行群组键
	// 只覆盖规则定群成员与未识别结构群成员（已识别成员的群组信息只在群卡），
	// 不足以充当复原索引。重求值目录的群由本轮结论装配，不经此通道。
	if in.prev != nil {
		for i := range in.prev.Groups {
			g := in.prev.Groups[i]
			if !in.prevSkipped[g.DirRelKey] {
				continue
			}
			gkey := g.DirRelKey + "\x00" + g.Key + "\x00" + g.RuleID
			if reusedGroups[gkey] {
				continue
			}
			reusedGroups[gkey] = true
			groups[gkey] = &g
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
	}

	// —— 元素4 结构性兄弟群（规则无关）的对齐判定已前置到求值段（规则定群段
	// 之后、提取段之前，结论携带成员信息），此处只做呈现标记：未识别成员记
	// 文件行群组键与「群组前缀=集名候选」线索（元素3）并升格有线索态；已识别
	// 成员仅入群呈现，不加线索不升格。跳过复用目录的复原群重复参与标记为幂等
	// 空转（未识别成员旧记录已带标记）。——
	structuralMarks := make([]SnapGroup, 0)
	for _, group := range groups {
		if group.RuleID == "" {
			structuralMarks = append(structuralMarks, *group)
		}
	}
	markStructuralPresentation(snap, structuralMarks)

	// 序列群按集内序排列成员、规则定群群落元素公共性终判（结构群的元素终判在
	// 对齐装配内完成），群间按目录、分组键与定群规则 ID 排序（呈现稳定序）
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
		if snap.Groups[i].Key != snap.Groups[j].Key {
			return snap.Groups[i].Key < snap.Groups[j].Key
		}
		// 第三决胜键=定群规则 ID：同目录同键多群（规则群与结构群分立、或两条
		// 定群规则同键）时前两键平局——结构群规则 ID 为空串、字典序居先，规则
		// 群间按规则 ID 字典序。三分量即群卡唯一键（上方 groups 映射键），补齐
		// 后比较器为全序，群卡终序不随上游 map 迭代序漂移（呈现序确定化，无
		// 结论变更）。
		return snap.Groups[i].RuleID < snap.Groups[j].RuleID
	})

	// —— 集名互证（批次二阶段3）：全部序列群（规则定群与结构群——此时群集合已
	// 完整）的分群键对库内作品集名全集精确匹配，命中的群全员补「集名互证」线索
	//（plain 成员升格有线索态，统计在末段按终态汇总）——
	attachSetNameCrossCheck(snap, in.lib)

	// —— 元素8：集名候选墙（批次三阶段5）：三类来源合流 + A 档库内集识别注记
	// + 会话态分仓（ignored/confirmed），见 buildSnapshotCandidates。——
	buildSnapshotCandidates(snap, in.lib, in)

	// —— 元素6：规则命中统计（含禁用规则；复用文件的命中从旧记录累计）——
	// 定群规则不进提取段/MatchedRuleIDs，命中按定群成员文件数计：规则群卡
	// （RuleID 非空——复用组随组拷贝、规则组逐成员追加）单点汇总；结构群
	// （RuleID 空）非规则产物不计。定群（分组段独占）与提取（提取段独占）
	// 两类通道按规则构造互斥，同一规则不会两面计数。
	hitByID := make(map[string]int)
	for _, g := range snap.Groups {
		if g.RuleID != "" {
			hitByID[g.RuleID] += len(g.Members)
		}
	}
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
	// 文件键：不在全量路径集=死路径（原样）；目录键：其目录不在快照目录树
	// 或子树无文件=死路径（条目供值面对不上任何活文件——空目录赋值与不
	// 存在目录同样无法生效）。子树有文件判定=逐文件标记祖先链。
	dirInTree := make(map[string]bool, len(in.enumRes.Dirs))
	for _, dir := range in.enumRes.Dirs {
		dirInTree[dir.RelKey] = true
	}
	subtreeHasFiles := make(map[string]bool, len(dirInTree))
	for _, f := range in.enumRes.AllFiles {
		for dir := dirRelKeyOf(f.RelPath); ; dir = parentDirRelKeyOf(dir) {
			subtreeHasFiles[dir] = true
			if dir == "" {
				break
			}
		}
	}
	for key := range in.assigns {
		if isDirAssignmentKey(key) {
			dir := dirRelKeyOfAssignment(key)
			if !dirInTree[dir] || !subtreeHasFiles[dir] {
				snap.StaleAssignments = append(snap.StaleAssignments, key)
			}
			continue
		}
		if !allRelPaths[key] {
			snap.StaleAssignments = append(snap.StaleAssignments, key)
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
// 探测器全部复用既有机制：谓词原子与内置 pixiv 形态探测器。Assigned 标记
// =就近解析非空（仅目录条目覆盖也算——与旧展开形态「有条目」等值）。
func snapFileOf(c *FileConclusion, lib *LibraryIndex, assignIdx *assignmentDirIndex) SnapFile {
	file := SnapFile{
		RelPath: c.RelPath, FileName: c.FileName, DirRelKey: c.DirRelKey,
		MatchedRuleIDs: c.MatchedRuleIDs,
	}
	if resolved := assignIdx.resolveFile(c.RelPath); len(resolved.coveredDimensions()) > 0 {
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
		RoleName: decl.RoleName, RuleID: decl.RuleID, AlsoRuleIDs: decl.AlsoRuleIDs,
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
	return alignEntryOfEls(fileName, relPath, nil, hints)
}

// alignEntryOfEls 元素数组在场的对齐参与文件构造（els=nil 现场按提示拆分）：
// 预览基座的元素预铸件经此复用（对齐判定只读元素数组，群装配另持元素副本，
// 预铸件不受改写）。
func alignEntryOfEls(fileName, relPath string, els []SnapEl, hints []string) *alignEntry {
	base := BaseNameWithoutExt(fileName)
	if els == nil {
		els = splitFilenameElements(base, hints)
	}
	e := &alignEntry{fileName: fileName, relPath: relPath, base: base, els: els}
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
//
// 并行区1：目录键切片后交工作子池、原子游标取目录号——群不跨目录（群聚合
// 语义不变量）保证目录级分区完备，目录间零共享写；每工作子写私有结果 map，
// 收尾按键（=文件相对路径，全库唯一）归并，归并次序无关。工作子 panic 沿串行
// 同款冒泡，严禁 recover。单工作子（或单目录）时同一逐目录实现在本 goroutine
// 内联执行——两模式同一实现。
func precomputeStructuralMembers(files []ScanFile, rules []RuleEntry, lib *LibraryIndex, siblingsByDir map[string][]string, sub *evalSubstrate) map[string]*structuralMemberInfo {
	byDir := make(map[string][]ScanFile, len(siblingsByDir))
	for _, file := range files {
		key := dirGroupKey(file.DirNames)
		byDir[key] = append(byDir[key], file)
	}
	dirKeys := make([]string, 0, len(byDir))
	for key := range byDir {
		dirKeys = append(dirKeys, key)
	}
	out := make(map[string]*structuralMemberInfo)
	if workers := min(evalWorkerCount(), len(dirKeys)); workers > 1 {
		privates := make([]map[string]*structuralMemberInfo, workers)
		for w := range privates {
			privates[w] = make(map[string]*structuralMemberInfo)
		}
		var cursor atomic.Int64
		var wg sync.WaitGroup
		wg.Add(workers)
		for w := 0; w < workers; w++ {
			go func(private map[string]*structuralMemberInfo) {
				defer wg.Done()
				for {
					i := int(cursor.Add(1)) - 1
					if i >= len(dirKeys) {
						return
					}
					alignStructuralDirMembers(dirKeys[i], byDir[dirKeys[i]], rules, lib, siblingsByDir, sub, private)
				}
			}(privates[w])
		}
		wg.Wait()
		for _, private := range privates {
			maps.Copy(out, private)
		}
		return out
	}
	for _, key := range dirKeys {
		alignStructuralDirMembers(key, byDir[key], rules, lib, siblingsByDir, sub, out)
	}
	return out
}

// alignStructuralDirMembers 单目录的结构群前置对齐（并行区1 的工作单元，串行
// 与并行两模式共用同一实现）：dirFiles 为同目录键的参与文件（至少一条），
// 成员信息按文件相对路径写入 out（相对路径全库唯一，调用方保证键不冲突）。
// sub 为基座元素预铸件供给（nil=现场拆分）。
func alignStructuralDirMembers(key string, dirFiles []ScanFile, rules []RuleEntry, lib *LibraryIndex, siblingsByDir map[string][]string, sub *evalSubstrate, out map[string]*structuralMemberInfo) {
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
		entries = append(entries, alignEntryOfEls(file.FileName, file.RelPath, sub.elementsOf(file.RelPath), hints))
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

// alignStructuralForFile 单文件通道的结构群前置对齐：兄弟名单（含自身，与求值
// 输入同一份）先逐名求规则定群段排除已定群者，再跑与批量入口同一实现的对齐
// 判定，取本文件所在群的成员信息；落单（不在任何群）返回 nil。sub 为基座
// 元素预铸件供给（兄弟文件名对应的相对路径逐名取用，nil=现场拆分）。
func alignStructuralForFile(in *MatchInput, rules []RuleEntry, lib *LibraryIndex, sub *evalSubstrate) *structuralMemberInfo {
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
		entries = append(entries, alignEntryOfEls(name, rel, sub.elementsOf(rel), hints))
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

// confirmedCollectionOf 已确认候选的确认集名：确认动作对成员写集维度赋值
// （collection=setName，文件例外形态），故取首个携带集维度赋值成员的取值
// ——成员就近解析取集名（文件例外或目录条目供值均可，首个非空口径保持）；
// 赋值此后被用户清除时为空串（confirmed 分仓仍在场，呈现按已确认但集名
// 缺席处理）。
func confirmedCollectionOf(cand SnapCandidate, assigns map[string]SessionAssignment) string {
	for _, mem := range cand.Members {
		if a := resolveFileAssignment(assigns, mem.RelPath); a.Collection != "" {
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

// panoramaMemberPreviewLimit 全景投影的条目成员预览上限：overview/domain 分节
// 条目内的成员数组（序列群/集名候选的成员、选择域作品的轨道与集的成员作品键）
// 在投影层截前本上限个——存储快照与服务端动作（确认归集/忽略/创建守卫链）仍用
// 全量成员按目标键解析，仅出线载荷截断；Size 等总数字段保持真值，前端由
// size>len(members) 推导截断并加说明。条目最坏 ≈ 200 成员 × ~2KB ≈ 400KB，按
// 构造远离 12MB 页软界——新库首用（清存储=档案空、全员未识别）的巨目录候选
// 单条 ~3.8 万成员 × ~2KB ≈ 79MB 直撞 64MB 应答断路器的形态由此封死，页界
// 首条必装 warn 退化为不可达兜底（测量硬门仍兜底估算漂移）。
const panoramaMemberPreviewLimit = 200

// previewWireMembers 投影层成员预览截断（泛型）：截前 panoramaMemberPreviewLimit
// 个（重切共享底层数组，零复制）；未超限原样返回（小夹具零行为变化）。
func previewWireMembers[T any](members []T) []T {
	if len(members) <= panoramaMemberPreviewLimit {
		return members
	}
	return members[:panoramaMemberPreviewLimit]
}

// previewSnapGroups 群清单的投影层成员截断：超限条目浅拷贝结构体 + 成员重切
//（不动存储快照，Size 保持真总数）；全清单未超限时原切片透传。
func previewSnapGroups(groups []SnapGroup) []SnapGroup {
	copied := false
	for i := range groups {
		if len(groups[i].Members) <= panoramaMemberPreviewLimit {
			continue
		}
		if !copied {
			groups = append([]SnapGroup(nil), groups...)
			copied = true
		}
		groups[i].Members = groups[i].Members[:panoramaMemberPreviewLimit]
	}
	return groups
}

// previewSnapCandidates 候选清单的投影层成员截断（口径同 previewSnapGroups，
// 候选三仓共用）。
func previewSnapCandidates(cands []SnapCandidate) []SnapCandidate {
	copied := false
	for i := range cands {
		if len(cands[i].Members) <= panoramaMemberPreviewLimit {
			continue
		}
		if !copied {
			cands = append([]SnapCandidate(nil), cands...)
			copied = true
		}
		cands[i].Members = cands[i].Members[:panoramaMemberPreviewLimit]
	}
	return cands
}

// panoramaOverviewViewOf 快照的 overview 分节投影：文件明细不随答（files 分节
// 逐页拉取），仅展开目录级结构线索集；候选三仓/序列群/待裁决明细等重载荷
// 清单由分节切页逐段携带，小字段（统计/规则档案态等）每页随答。重载荷条目的
// 成员按投影层预览上限截断（服务端动作仍用全量成员，见 panoramaMemberPreviewLimit）。
func panoramaOverviewViewOf(snap *RecognitionSnapshot) RecognitionSnapshot {
	view := *snap
	view.DirMtimes = nil
	view.Files = nil
	view.Groups = previewSnapGroups(view.Groups)
	view.Candidates = previewSnapCandidates(view.Candidates)
	view.IgnoredCandidates = previewSnapCandidates(view.IgnoredCandidates)
	view.ConfirmedCandidates = previewSnapCandidates(view.ConfirmedCandidates)
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

// 欠计数护栏判定线（两条件同时满足才拒绝覆盖）：新枚举文件数 × 比例因子 <
// 上轮快照文件数（少过半）且 绝对损失 ≥ 下限。日常增删文件远低于该线不触发；
// 合法删除近半库会被拦一次，用户再次触发即强制覆盖。
const (
	// UNDERCOUNT_GUARD_RATIO_FACTOR 相对比例因子（2 = 少过半进入判定）。
	UNDERCOUNT_GUARD_RATIO_FACTOR = 2
	// UNDERCOUNT_GUARD_MIN_ABS_LOSS 绝对损失下限（个）。
	UNDERCOUNT_GUARD_MIN_ABS_LOSS = 5
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

// recognitionTrigger 识别干跑的启动意图。欠计数护栏的放行簿记只被用户动作
// 入口消费：自动刷新链见簿记不消费、护栏照常生效——放行必须是用户的显式点击
// 确认，自动轮不得静默绕过护栏。
type recognitionTrigger int

const (
	// recognitionTriggerUser 用户动作入口（工作台「重新识别」按钮）。
	recognitionTriggerUser recognitionTrigger = iota
	// recognitionTriggerAuto 自动刷新链（规则生成/赋值变更的刷新调度与其
	// 在途结束后的补启动）。
	recognitionTriggerAuto
)

// StartRecognition 用户动作入口（工作台「重新识别」按钮）：异步启动全量识别
// 干跑，同库根已在途时不重复启动（返回 running=true）；会话不存在时返回错误。
// 与自动入口的差异仅在欠计数护栏放行簿记：本入口启动时见簿记即清位，本轮跳过
// 护栏强制覆盖。干跑完成经 recognitionDone 事件通知，快照与状态迁移由干跑主体
// 完成。
func (m *SessionManager) StartRecognition(root string) (started, running bool, err error) {
	return m.startRecognition(root, recognitionTriggerUser)
}

// startRecognitionAuto 自动刷新链入口（识别刷新调度的当轮启动与在途结束后
// 的补启动）：启动行为与用户入口一致，但不消费欠计数护栏放行簿记——自动轮
// 命中欠计数条件照常拒绝，错误经 recognitionDone 事件用户可见。
func (m *SessionManager) startRecognitionAuto(root string) (started, running bool, err error) {
	return m.startRecognition(root, recognitionTriggerAuto)
}

// startRecognition 干跑启动主体（两意图入口共用）：在跑互斥置位后干跑进
// goroutine；意图位仅在放行簿记消费处分岔。
func (m *SessionManager) startRecognition(root string, trigger recognitionTrigger) (started, running bool, err error) {
	if _, err := m.cachedSession(root); err != nil { // 只读存在性校验：缓存读路径
		return false, false, err
	}
	skipUndercountGuard := false
	m.recogMu.Lock()
	if m.recogRunning == nil {
		m.recogRunning = make(map[string]bool)
	}
	if m.recogRunning[root] {
		m.recogMu.Unlock()
		return false, true, nil
	}
	if trigger == recognitionTriggerUser && m.undercountBlocked[root] {
		// 放行簿记消费（仅用户动作）：上次被拦的库根再次触发即跳过护栏。
		delete(m.undercountBlocked, root)
		skipUndercountGuard = true
	}
	m.recogRunning[root] = true
	m.recogMu.Unlock()
	go func() {
		defer func() {
			m.recogMu.Lock()
			delete(m.recogRunning, root)
			m.recogMu.Unlock()
		}()
		if err := m.runFullRecognitionOpt(root, skipUndercountGuard); err != nil {
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

// undercountGuardErr 欠计数护栏判定：上轮快照在场且新枚举全集显著欠收（见
// UNDERCOUNT_GUARD 判定线——移动盘掉线/半可见、路径权限抖动的典型形态）时
// 置放行簿记并返回用户可读拒绝错误，调用方以此中止本轮、会话零写入（既有
// 好快照原样保全）；prev 为空（首轮，无既有结论可保护）或收缩未及判定线时
// 返回 nil 放行。判定基数 = 枚举全集 AllFiles（含未变目录复用者，与上轮
// 快照文件计数同口径），增量轮与强制全量轮同判。
func (m *SessionManager) undercountGuardErr(root string, prev *RecognitionSnapshot, enumCount int) error {
	if prev == nil {
		return nil
	}
	prevCount := prev.Stats.TotalFiles
	if enumCount*UNDERCOUNT_GUARD_RATIO_FACTOR >= prevCount || prevCount-enumCount < UNDERCOUNT_GUARD_MIN_ABS_LOSS {
		return nil
	}
	m.markUndercountBlocked(root)
	loss := prevCount - enumCount
	return fmt.Errorf("欠计数护栏拒绝覆盖：本轮枚举到 %d 个文件，上轮识别快照为 %d 个（少了 %d 个、约 %.0f%%）——磁盘可能掉线或半可见；若文件确已删除，请再次点击重新识别强制覆盖",
		enumCount, prevCount, loss, float64(loss)/float64(prevCount)*100)
}

// markUndercountBlocked 置欠计数护栏放行簿记（库根 → 上次被拦；重复拒绝
// 重复置位，幂等）。
func (m *SessionManager) markUndercountBlocked(root string) {
	m.recogMu.Lock()
	if m.undercountBlocked == nil {
		m.undercountBlocked = make(map[string]bool)
	}
	m.undercountBlocked[root] = true
	m.recogMu.Unlock()
}

// —— 赋值增量重算（预览面大库性能方案·阶段5）——

// assignIncrementalTaken 赋值增量重算命中计数探针（测试断言各形态落点用；
// 生产面只增不读）。
var assignIncrementalTaken atomic.Int64

// assignIncrementalDisabled 赋值增量重算的测试开关（生产恒 false）：等值锚以
// 同一会话态分别强制走赋值增量与整库全量路径，逐字节比对两路快照。
var assignIncrementalDisabled bool

// tryAssignmentIncremental 赋值增量重算：赋值保存触发的识别刷新轮在「自上次
// 快照以来仅会话赋值变更」时，不再整库重算（基线 12.19s@20 万文件的大头=
// 枚举+求值）——受影响面=全部文件的合成重放：规则段结论与赋值无关（赋值只在
// 合成后按维度覆盖终选，recognition.go:349-351 核码事实），基座规则相关件供给
// 逐文件提取段原始声明作底，走与 mergedRecognition 同款「Synthesize 合成 →
// 会话赋值按维度覆盖」链重建全部结论，再喂给同一 aggregatePanorama 装配
// （枚举产物自上次快照重建：目录树/文件序沿用、跳过集为空——与「赋值变更后
// 整库全量轮」的 SkippedDirs=0 同口径；DirMtimes 携带上轮值，磁盘未变时与
// 现场枚举等值），快照与差异集经 CompleteRecognitionWithDiff 落库。合成重放
// 与装配组织为 O(全库) 纯组织（无枚举无重求值），秒级。
//
// 增量资格（任一不符即返回 false，调用方回退现行整库路径，结果一致）：
//   - 上次快照在场，且档案指纹一致（非空）、赋值修订号不一致（赋值写是本路径
//     唯一预期变更源——修订号一致属 mtime 增量轮、指纹失配属档案变更轮）、
//     求值器构建标识等值（空串=未打标语义同闸门）；
//   - 基座就位（缓存条目持有，文件集身份键与上次快照等值——赋值写轻保存后
//     经值守卫携带，见 putSessionCache/matchesSnapshot）且规则相关件守卫通过
//     （档案指纹+构建标识+库内名单内容键——名单换代即失配回退）；
//   - 基座规则段覆盖上次快照全部文件（增量携带层的完备性兜底）。
//
// 并发边界：本路径运行于识别刷新轮内部——在途互斥与「在途结束后补启动」由
// 调度壳 startRecognition/scheduleRecognitionRefresh 既有机制承担（识别在途时
// 赋值写只排定补启轮，不阻塞识别）；期间新的赋值写经修订号闸门排定下一轮
// 增量，收敛到最新赋值态。返回 (true, nil)=快路径完成、(true, err)=快路径
// 失败（错误沿整库路径同款出口上报）、(false, nil)=回退整库。
func (m *SessionManager) tryAssignmentIncremental(root string) (bool, error) {
	if assignIncrementalDisabled {
		return false, nil
	}
	// 资格判定逐条下行；读侧错误一律回退整库路径（由其以原口径报错）。
	sess, err := m.loadSessionFresh(root)
	if err != nil {
		return false, nil
	}
	prev := sess.Snapshot
	if prev == nil {
		return false, nil
	}
	profile, errs := LoadProfile(m.store)
	for _, loadErr := range errs {
		m.warnf("规则档案载入异常（赋值增量重算）: %v", loadErr)
	}
	if profile == nil {
		return false, nil
	}
	digest := ""
	if d, derr := profileDigestOf(profile); derr == nil {
		digest = d
	}
	evalBuildID := m.evaluatorBuildID()
	if digest == "" || prev.ProfileDigest != digest || prev.AssignmentRevision == sess.AssignRevision {
		return false, nil
	}
	if evalBuildID != "" && prev.EvaluatorBuildID != evalBuildID {
		return false, nil
	}
	foundation := m.cachedFoundation(root)
	if foundation == nil || !foundation.matchesSnapshot(prev) {
		return false, nil
	}
	layer := foundation.rule
	if layer == nil || !layer.guardMatches(digest, evalBuildID) {
		return false, nil
	}
	lib := m.libraryIndexForIncremental(root)
	if !layer.libDigestMatches(libraryIndexDigest(lib)) {
		return false, nil
	}
	sub := foundation.indep
	if sub == nil || len(sub.files) != len(prev.Files) {
		return false, nil
	}
	for i := range sub.files {
		if layer.segmentsByRel[sub.files[i].RelPath] == nil {
			return false, nil
		}
	}
	enabledRules := EffectiveRules(profile)
	policy := AggregateSynthesis(enabledRules)
	files := sub.files
	total := len(files)
	assignIncrementalTaken.Add(1)
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseScan,
		Done: total, Total: total,
	})
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseIndex,
		Done: indexSize(lib), Total: indexSize(lib),
	})
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseEvaluate,
		Done: 0, Total: total,
	})
	// 会话赋值就近解析预计算：与 mergedRecognition 同一构造（目录集=求值面全部
	// 文件的所属目录键；解析序无关——逐目录 DP）。
	var assignIdx *assignmentDirIndex
	if len(sess.Assignments) > 0 {
		dirKeys := make(map[string]struct{}, len(files))
		for _, file := range files {
			dirKeys[dirRelKeyOf(file.RelPath)] = struct{}{}
		}
		dirKeyList := make([]string, 0, len(dirKeys))
		for dirKey := range dirKeys {
			dirKeyList = append(dirKeyList, dirKey)
		}
		assignIdx = newAssignmentDirIndex(sess.Assignments, dirKeyList)
	}
	// 逐文件结论重建：基座规则段原始声明 → 同一 Synthesize → 赋值按维度覆盖
	//（与 mergedFileConclusionWithStructural 的合成段同链同序）；定群群成员的
	// 元素数组与快路径同口径（预铸副本、缺席现场拆分——公共性终判前的原始
	// 类别，终判由装配段 markCommonElements 统一落格），结构群成员直取段携带
	// 值（对齐装配内已终判）。
	conclusions := make([]FileConclusion, len(files))
	rebuildAt := func(i int) {
		file := files[i]
		seg := layer.segmentsByRel[file.RelPath]
		final, pending := Synthesize(seg.declarations, policy)
		if assignIdx != nil {
			if a := assignIdx.resolveFile(file.RelPath); len(a.coveredDimensions()) > 0 {
				final, pending = applyAssignmentToConclusion(final, pending, &a)
			}
		}
		groupElements := seg.groupElements
		if seg.groupRuleID != "" {
			if els := sub.elementsOf(file.RelPath); els != nil {
				groupElements = els
			} else {
				groupElements = splitFilenameElements(BaseNameWithoutExt(file.FileName), dirNameHints(file.DirNames))
			}
		}
		conclusions[i] = FileConclusion{
			RelPath: file.RelPath, FileName: file.FileName, DirRelKey: dirRelKeyOf(file.RelPath),
			Group: seg.group, MatchedRuleIDs: seg.matchedRuleIDs,
			GroupRuleID: seg.groupRuleID, GroupSeqText: seg.groupSeqText, GroupElements: groupElements,
			GroupFormName: seg.groupFormName, GroupFormKeyExpr: seg.groupFormKeyExpr, GroupFormSeqExpr: seg.groupFormSeqExpr,
			Final: final, Pending: pending, rawDecls: seg.declarations,
		}
	}
	// 并行区与 mergedRecognition 同款形态：块约百余文件、原子游标取块、结论按
	// 下标写入保持输入序；共享输入（基座两件/规则段/合成策略/赋值预计算表）只
	// 读（零锁纪律），工作子 panic 沿串行同款冒泡，严禁 recover。
	if chunks := (total + evalFileChunkSize - 1) / evalFileChunkSize; chunks > 1 && evalWorkerCount() > 1 {
		workers := min(evalWorkerCount(), chunks)
		var cursor atomic.Int64
		var wg sync.WaitGroup
		wg.Add(workers)
		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for {
					c := int(cursor.Add(1)) - 1
					if c >= chunks {
						return
					}
					start := c * evalFileChunkSize
					end := min(start+evalFileChunkSize, total)
					for i := start; i < end; i++ {
						rebuildAt(i)
					}
				}
			}()
		}
		wg.Wait()
	} else {
		for i := range files {
			rebuildAt(i)
		}
	}
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseEvaluate,
		Done: total, Total: total,
	})
	snapshot := aggregatePanorama(aggregateInput{
		root: root, enumRes: enumerateResultOfSnapshot(prev),
		enabledRules: enabledRules, allRules: allRulesWithBuiltins(profile), lib: lib,
		conclusions: conclusions, prev: prev, prevSkipped: map[string]bool{},
		assigns: sess.Assignments, profileDigest: digest, assignRevision: sess.AssignRevision,
		evaluatorBuildID: evalBuildID,
		ignoredTargets:   targetSetOf(sess.IgnoredCandidates),
		confirmedTargets: targetSetOf(sess.ConfirmedCandidates),
		confirmedSetIDs:  sess.ConfirmedSetIDs,
	})
	diff := ComputeSnapshotDiff(prev, snapshot)
	if _, err := m.CompleteRecognitionWithDiff(root, snapshot, diff); err != nil {
		return true, err
	}
	// 基座不重铸：规则段结论与赋值无关（本轮零重求值），文件集未变——保存路径
	// 重建缓存条目时经文件集身份键携带（putSessionCache），阶段2/3 的预览加速
	// 成果随赋值增量轮延续；文件集/档案/名单换代的重铸由对应整库轮承担。
	m.logf("赋值增量重算完成（root=%s，文件 %d，修订号 %d）", root, snapshot.Stats.TotalFiles, sess.AssignRevision)
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseSave,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
	})
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionDone, Root: root, Success: true,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
		Message: recognitionDoneMessage(snapshot, diff),
		// 完成时点档案对账（与全量轮同口径）：赋值增量轮完成后横幅同样按
		// 完成时点对账撤销/保持。
		ProfileChanged: m.profileChangedSince(digest),
	})
	return true, nil
}

// enumerateResultOfSnapshot 上次快照 → 赋值增量重算的枚举产物重建（纯组织）：
// 目录树（含直接子文件数）、全量文件清单沿用快照序，mtime 游标携带上轮值
//（磁盘未变时与现场枚举等值；磁盘变更由下一轮识别的现场枚举接住），跳过集
// 为空——与「赋值变更后整库全量轮」（闸门三条件失配→全量枚举零跳过）的
// Stats.SkippedDirs=0 同口径。
func enumerateResultOfSnapshot(prev *RecognitionSnapshot) *EnumerateResult {
	dirs := make([]EnumerateDir, len(prev.Dirs))
	for i := range prev.Dirs {
		d := &prev.Dirs[i]
		dirs[i] = EnumerateDir{RelKey: d.RelKey, Name: d.Name, Depth: d.Depth, FileCount: d.FileCount}
	}
	allFiles := make([]FileEntry, len(prev.Files))
	for i := range prev.Files {
		allFiles[i] = FileEntry{RelPath: prev.Files[i].RelPath}
	}
	mtimes := make(map[string]int64, len(prev.DirMtimes))
	maps.Copy(mtimes, prev.DirMtimes)
	return &EnumerateResult{AllFiles: allFiles, DirMtimes: mtimes, Dirs: dirs}
}

// targetSetOf 目标键清单 → 集合（候选分仓判据装配）。
func targetSetOf(targets []string) map[string]bool {
	set := make(map[string]bool, len(targets))
	for _, t := range targets {
		set[t] = true
	}
	return set
}

// recognitionDoneMessage 识别完成消息（整库全量轮与赋值增量重算轮共用同一
// 格式，快照统计与差异计数为唯一数据源）。
func recognitionDoneMessage(snapshot *RecognitionSnapshot, diff *SnapDiff) string {
	changed, added, removed := diffCounts(diff)
	return fmt.Sprintf("识别完成：共 %d 个文件（已识别 %d、有线索 %d、无线索 %d），序列群 %d，待裁决文件 %d；与上次差异：变更 %d、新增 %d、消失 %d",
		snapshot.Stats.TotalFiles, snapshot.Stats.Identified, snapshot.Stats.Clueful,
		snapshot.Stats.Plain, snapshot.Stats.GroupCount, snapshot.Stats.PendingFiles,
		changed, added, removed)
}

// runFullRecognition 全量识别干跑主体（欠计数护栏设防形态）：上轮快照显著
// 欠收时拒绝覆盖既有识别结论。直接调用方（含自动刷新链启动的轮）一律走本
// 入口；用户动作二次触发的强制覆盖轮走 runFullRecognitionOpt(root, true)。
func (m *SessionManager) runFullRecognition(root string) error {
	return m.runFullRecognitionOpt(root, false)
}

// runFullRecognitionOpt 干跑主体（零任务产出、零弹问的纯求值）：
// 轻量枚举（mtime 增量）→ 库内谓词索引构造 → 「档案 ∪ 会话赋值」合并求值 →
// 七类元素聚合 → 快照入会话与状态迁移（CompleteRecognition，单次写）。
// skipUndercountGuard=true 跳过欠计数护栏（用户动作二次触发的强制覆盖轮，
// 完成消息附注说明）。
// 增量资格：上次快照在场且其档案指纹与当前一致——档案外部变更或首次识别一律
// 全量重算；赋值文件所在目录即使 mtime 未变也强制重算（赋值按维度覆盖结论）。
// 赋值修订号失配（赋值保存触发的刷新轮）由函数头的赋值增量重算快分支接住
//（仅赋值变更时秒级重放，见 tryAssignmentIncremental），资格不符回落本整库路径。
func (m *SessionManager) runFullRecognitionOpt(root string, skipUndercountGuard bool) error {
	// 赋值增量重算（阶段5）：「自上次快照以来仅会话赋值变更」的刷新轮走秒级
	// 合成重放路径（不枚举不重求值）；资格不符（返回 false）继续下方整库路径，
	// 结果一致。计时锚（TestPreviewPathBaseline 赋值路）直调本函数即得快路径。
	if taken, err := m.tryAssignmentIncremental(root); taken {
		return err
	}
	// 变异旁路读：识别主体持有会话跨整轮（分钟级），且聚合阶段对 prev 快照的
	// 复用群成员有共享底层数组的转置排序与元素终判改写（aggregatePanorama 的
	// sort.SliceStable/markCommonElements）——必须私有实例，缓存共享实例不得
	// 进入本通路。
	sess, err := m.loadSessionFresh(root)
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
	// 新增/修改/清除都会改修订号——赋值保存触发的刷新轮由函数头的赋值增量
	// 重算快分支接住；落回本路径时〔资格不符〕修订号闸门保证赋值清除后其目录
	// 不再由赋值集锚定的旧结论不被 mtime 游标复用）、且快照铸于同一识别求值器
	// 构建（EvaluatorBuildID 等值）才可按目录 mtime 游标跳过。当前构建标识为
	// 空串（未打标构建，如开发直跑 go test）不参与比对；旧快照空串而当前有值
	//（存量库升级到打标构建）或两值不同（求值器换版）均强制全量——每库根
	// 一次的首轮全量重算是升级后的预期成本。
	evalBuildID := m.evaluatorBuildID()
	prev := sess.Snapshot
	incremental := prev != nil && prev.ProfileDigest == digest && digest != "" &&
		prev.AssignmentRevision == sess.AssignRevision &&
		(evalBuildID == "" || prev.EvaluatorBuildID == evalBuildID)
	prevMtimes := map[string]int64(nil)
	if incremental {
		prevMtimes = prev.DirMtimes
	}
	// 上轮基座预取：保存路径会整体重建缓存条目（基座随旧条目消亡），在此取回
	// 供规则相关层增量携带（守卫在铸造处再判）。
	prevFoundation := m.cachedFoundation(root)

	enumRes, err := NewScanner(root).Enumerate(prevMtimes)
	if err != nil {
		return err
	}
	// 欠计数护栏：枚举成功后、任何会话状态写入之前判定——拒绝即整轮中止，
	// 会话零写入（好快照原样保全），错误经调用方的 recognitionDone{success:
	// false} 出口呈给用户。
	if !skipUndercountGuard {
		if err := m.undercountGuardErr(root, prev, len(enumRes.AllFiles)); err != nil {
			return err
		}
	}
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseScan,
		Done: len(enumRes.Files), Total: len(enumRes.AllFiles),
	})

	// 跳过集修正：赋值影响面的跳过目录整目录强制重算（赋值覆盖维度需重求
	// 值）。等值口径（旧形态目录赋值展开为每文件条目=子树每目录都命中）：
	// 目录 D 复活⟺D 自身或祖先链存在目录条目，或 D 直属文件存在文件例外
	// ——前者覆盖子树全部目录（与旧展开等值），后者锚定单目录（旧形态下
	// 该目录的每文件条目至少含此例外）。
	assignDirs := make(map[string]bool, len(sess.Assignments))
	for key := range sess.Assignments {
		if isDirAssignmentKey(key) {
			continue // 目录条目经祖先链判定覆盖子树全部目录
		}
		assignDirs[dirRelKeyOf(key)] = true
	}
	dirAssignmentActive := func(dirKey string) bool {
		for dir := dirKey; ; dir = parentDirRelKeyOf(dir) {
			if _, ok := sess.Assignments[assignmentDirKeyOf(dir)]; ok {
				return true
			}
			if dir == "" {
				return false
			}
		}
	}
	prevSkipped := make(map[string]bool, len(enumRes.SkippedDirKeys))
	for _, dirKey := range enumRes.SkippedDirKeys {
		prevSkipped[dirKey] = true
	}
	evalFiles := enumRes.Files
	for _, f := range enumRes.AllFiles {
		dirKey := dirRelKeyOf(f.RelPath)
		if prevSkipped[dirKey] && (assignDirs[dirKey] || dirAssignmentActive(dirKey)) {
			evalFiles = append(evalFiles, f)
		}
	}
	for dirKey := range prevSkipped {
		if assignDirs[dirKey] || dirAssignmentActive(dirKey) {
			delete(prevSkipped, dirKey)
		}
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

	// 求值相进度节流闭包：相首先发一拍 done=0（相位切换即时反馈），求值中每
	// 跨一个步长由钩子发一拍，相尾全量拍保留在求值返回后。闭包可能被并行
	// 工作子调用，互斥串行化发布——PublishToFrontend 的并发安全未核验，发布
	// 串行不依赖调用方纪律。
	var evalProgressMu sync.Mutex
	publishEvalProgress := func(done, total int) {
		evalProgressMu.Lock()
		m.publishRecognitionEvent(&workbenchFrontendEvent{
			Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseEvaluate,
			Done: done, Total: total,
		})
		evalProgressMu.Unlock()
	}
	publishEvalProgress(0, len(evalFiles))
	evalScan := buildScanFiles(evalFiles)
	treeScan := buildScanFiles(enumRes.AllFiles)
	var harvest evalHarvest
	conclusions := mergedRecognition(evalScan, treeScan, enabledRules, lib, sess.Assignments, nil, &harvest, publishEvalProgress)
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
		evaluatorBuildID: evalBuildID,
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
	// 基座铸造与置入：识别装配完成处顺手铸造（规则无关层自新快照全量组织，
	// 规则相关层按增量资格携带或全量铸造——浅层组织+引用拷贝，无重求值），
	// 保存成功的新缓存条目内置入；条目被逐出等置入失配时弃置，下次预览回退
	// 现行路径，下一轮识别重铸。
	m.attachPreviewFoundation(root, mintPreviewFoundation(snapshot, &harvest, conclusions, evalScan, digest, evalBuildID, libraryIndexDigest(lib), prevFoundation, incremental))
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionProgress, Root: root, Phase: recogPhaseSave,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
	})
	doneMsg := recognitionDoneMessage(snapshot, diff)
	if skipUndercountGuard {
		doneMsg += "；本轮经再次触发跳过欠计数护栏"
	}
	m.publishRecognitionEvent(&workbenchFrontendEvent{
		Kind: workbenchEventRecognitionDone, Root: root, Success: true,
		Done: snapshot.Stats.TotalFiles, Total: snapshot.Stats.TotalFiles,
		Message: doneMsg,
		// 完成时点档案对账：识别期间档案未被再变更则横幅随本事件撤销（缺陷②
		// 修复——会话指纹已随落库重捕，横幅不再滞留到下次 activateSession）。
		ProfileChanged: m.profileChangedSince(digest),
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
