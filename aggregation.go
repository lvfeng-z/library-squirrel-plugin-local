// 聚合求值内核的并组算法：聚合规则（条件行+作品标识声明+分轨入口）对求值面做
// 标识等值并组——条件行复用既有条件行求值机制（文件名/目录名/序列/引用四种源
// 的行包装求值与捕获绑定），并组判定为纯函数（输入=扫描事实+规则档案+资源
// 类型注册表投影，无 IO、输入序确定）。求值挂点见 pipeline.go 的聚合段。
// 聚合段成员判定=命中分轨行或绑定行（仅绑定行命中=说明成员——归进作品、
// 无轨、不计必含轨单位）；其后接扇出归属段（fanoutAttribution，覆盖一切含
// 「元数据文件」产出与绑定的启用规则——内容按分组扇出、元数据源标记，
// 含并用形态）。
package main

import (
	"fmt"
	"strings"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// —— 资源类型注册表投影（类型锚定的单一事实源视图）——

// resourceTypeIndex 资源类型注册表投影索引：ListResourceTypes 投影的一次拉取
// 视图（内置 6 类+插件自定义类型，含轨角色基数与必含标记）。求值器只消费本
// 索引、不发起查询（纯度红线）；拉取发生在识别轮装配处（与库内谓词索引同款
// 轮级取数）。
type resourceTypeIndex struct {
	// byType 为类型值 → 规约。
	byType map[string]*sdkdto.ResourceTypeSpec
}

// newResourceTypeIndex 从投影条目构造索引（nil 条目与空类型值跳过）。
func newResourceTypeIndex(items []*sdkdto.ResourceTypeSpec) *resourceTypeIndex {
	idx := &resourceTypeIndex{byType: make(map[string]*sdkdto.ResourceTypeSpec, len(items))}
	for _, item := range items {
		if item == nil || item.ResourceType == "" {
			continue
		}
		idx.byType[item.ResourceType] = item
	}
	return idx
}

// has 报告作品类型是否在册（索引缺席时恒为不在册——聚合规则按类型悬空处理）。
func (idx *resourceTypeIndex) has(workType string) bool {
	if idx == nil {
		return false
	}
	_, ok := idx.byType[workType]
	return ok
}

// roleRequired 报告类型规约中该结构角色是否必含（Min≥1）：必含轨所在行承载
// 聚合作品的主导文件与作品级结论。
func (idx *resourceTypeIndex) roleRequired(workType, role string) bool {
	if idx == nil {
		return false
	}
	spec, ok := idx.byType[workType]
	if !ok {
		return false
	}
	for _, r := range spec.Roles {
		if r != nil && r.StoreType == role && r.Min >= 1 {
			return true
		}
	}
	return false
}

// fetchResourceTypeIndex 从宿主拉取资源类型注册表投影（插件侧一次拉取缓存消费
// 的轮级形态：识别轮装配处调用，与 BuildLibraryIndex 同款轮级取数；求值器内
// 不缓存不重查）。上下文缺席或查询失败返回 nil 并告警——调用方按无类型规约
// 处置（聚合规则类型悬空停用）。
func fetchResourceTypeIndex(ctx sdkdto.PluginContext, warnf func(string, ...any)) *resourceTypeIndex {
	if ctx == nil {
		return nil
	}
	resp, err := ctx.ListResourceTypes()
	if err != nil {
		if warnf != nil {
			warnf("查询资源类型注册表失败，聚合规则按类型悬空停用处理: %v", err)
		}
		return nil
	}
	if resp == nil {
		return nil
	}
	return newResourceTypeIndex(resp.Items)
}

// —— 聚合段求值面（与逐文件求值循环同一构造口径的聚合视图）——

// aggEvalSurface 聚合段求值面：文件清单（求值序）与同目录兄弟、各层祖先直接
// 子文件/子目录数的聚合表——构造口径与 mergedRecognition / RunPipeline 一致，
// 聚合段的条件行求值由此取得与逐文件求值循环相同的输入面。
type aggEvalSurface struct {
	// files 为求值面文件清单（输入序）。
	files []ScanFile
	// siblingsByDir 为求值面的同目录兄弟全集（键=目录名序列的 NUL 连接）。
	siblingsByDir map[string][]string
	// treeSiblings 为全量树的同目录兄弟全集（增量批时跨被跳目录聚合）。
	treeSiblings map[string][]string
	// childDirsByParent 为各层祖先目录的直接子目录名集合。
	childDirsByParent map[string]map[string]struct{}
}

// newAggEvalSurface 从求值面与全量树清单构造聚合段求值面（两清单同一时聚合
// 表只算一份口径——兄弟表按求值面、树面表与子目录聚合按树面）。
func newAggEvalSurface(files, treeFiles []ScanFile) *aggEvalSurface {
	siblingsByDir := make(map[string][]string, len(files))
	for _, file := range files {
		key := dirGroupKey(file.DirNames)
		siblingsByDir[key] = append(siblingsByDir[key], file.FileName)
	}
	treeSiblings := make(map[string][]string, len(treeFiles))
	childDirsByParent := make(map[string]map[string]struct{})
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
	return &aggEvalSurface{
		files:             files,
		siblingsByDir:     siblingsByDir,
		treeSiblings:      treeSiblings,
		childDirsByParent: childDirsByParent,
	}
}

// matchInputOf 构造单文件的求值输入（与逐文件求值循环同键同序）。
func (s *aggEvalSurface) matchInputOf(file ScanFile) *MatchInput {
	return &MatchInput{
		FileName:               file.FileName,
		DirNames:               file.DirNames,
		RelPath:                file.RelPath,
		Siblings:               s.siblingsByDir[dirGroupKey(file.DirNames)],
		AncestorFiles:          ancestorFilesLevels(file.DirNames, s.treeSiblings),
		AncestorChildDirCounts: ancestorChildDirCounts(file.DirNames, s.childDirsByParent),
	}
}

// —— 并组求值预案（启用聚合规则的编译形态）——

// identityGroupKeyCapture 为标识声明的序列群键专用捕获名（IdentityPart.Capture
// 的保留值）：取该行序列求值写入的群组键（非序号捕获按序拼接），每序列组一件
// 作品；只在序列目标行上可声明（载入校验拒绝其他目标）。
const identityGroupKeyCapture = "groupKey"

// aggRulePlan 单条聚合规则的并组求值预案：各条件行（先序展平叶子行——括号
// 子组内层行照常逐行寻址）编译为独立求值的行包装模式，加上标识声明、分轨
// 入口与按类型规约判定的必含轨行集合；布尔组合形态（括号子组/行级非在场）
// 另编译整条求值门（gate）——成员判定消费规则完整求值的命中集（决策10）。
type aggRulePlan struct {
	// rule 为源规则（引用传入规则序列的条目）。
	rule *RuleEntry
	// rows 为条件行模式（下标+1=先序展平行号；括号子组内层行照常占号）。
	rows []Pattern
	// rowIsSeq 为各条件行是否序列目标行（集合轨：整序列组=N 存储位，必含轨
	// 计数按相异群键计 1 单位）。
	rowIsSeq []bool
	// gate 为布尔组合形态的整条求值门（括号子组/行级非在场才非 nil——见
	// aggregateBooleanGate；纯平行行 nil=不设门，行命中池 OR 汇聚即命中面，
	// 存量聚合规则的既有语义逐字节等值）。
	gate Pattern
	// tracks 为分轨入口（条件行→角色分配）。
	tracks []TrackAssign
	// requiredRow 为必含轨行号集合（所分配角色在类型规约中 Min≥1）。
	requiredRow map[int]bool
	// bindingRows 为被内容绑定勾选的行号集合（聚合成员判定的绑定行面：仅
	// 绑定行命中的文件=说明成员——归进作品、无轨、不计必含轨单位；轨行兼
	// 绑定的双重身份成员照常落轨）。
	bindingRows map[int]bool
	// grouping 为并组所绑的分组项：规则显式绑定「聚合成作品」的目录分组
	// （Aggregate.Grouping）在场即用之，缺省为标识声明的标识分组（键部分=标
	// 识声明，按标识值等值并组）。成员面由此经分组求值两段得出——目录分组
	// 只收键集内目录的成员，键集外的文件不入组。
	grouping *GroupingBinding
	// seq 为规则在规则表中的下标（归属竞争的先到先得凭据：扇出归属段须
	// 辨认「归属是否本规则聚合段所赋」——本规则收编的绑定行成员照常补
	// 扇出键集，他规则收编的不再改判）。
	seq int
}

// aggregateBooleanGate 布尔组合形态的整条求值门（决策10：成员判定消费规则
// 完整求值的命中集）：条件行含括号子组或行级非时，把整行编译为完整布尔树
// （且连接紧于或连接——与提取段同一行式底盘语义；平行余留的空连接词按
// 「或」归一，平行=行命中池 OR）；纯平行行（无括号无行级非——存量聚合/
// 映射规则的现状形态）返回 nil 不设门——行命中池 OR 汇聚即命中面，既有
// 行为逐字节等值（连接词不参与成员判定）。编译失败返回错误（调用方按宁缺
// 勿错整条跳过——与载入侧校验互补的双保险）。
func aggregateBooleanGate(rule *RuleEntry) (Pattern, error) {
	if len(rule.Conditions) == 0 || !condxHasBooleanShape(rule.Conditions) {
		return nil, nil
	}
	rows := make([]CondxRow, len(rule.Conditions))
	copy(rows, rule.Conditions)
	for i := range rows {
		if i > 0 && rows[i].Connector == "" {
			rows[i].Connector = "or"
		}
	}
	return compileCondxRows(rows)
}

// buildAggregationPlans 过滤启用聚合规则并编译预案。跳过形态（求值侧的宁缺
// 勿错，与档案载入侧的停用+告警互补——载入已拦的形态在此双保险）：
//   - 类型悬空（WorkType 不在投影，含索引缺席）——识别求值同样跳过；
//   - 行编译失败或布尔门编译失败（括号子组/行级非行照常入预案——决策10
//     放开行结构限制，行级命中按行、整条求值进门）；
//   - 分轨入口未覆盖任何必含角色（类型规约无 Min≥1 角色被引用）——主导文件
//     无从判定，不产出聚合归属。
func buildAggregationPlans(rules []RuleEntry, types *resourceTypeIndex) []*aggRulePlan {
	var plans []*aggRulePlan
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.Category != RuleCategoryAggregate || rule.Aggregate == nil {
			continue
		}
		if !types.has(rule.Aggregate.WorkType) {
			continue
		}
		leaves := condxLeafRowsOf(rule.Conditions)
		if len(leaves) == 0 {
			continue
		}
		rows := make([]Pattern, 0, len(leaves))
		rowIsSeq := make([]bool, 0, len(leaves))
		valid := true
		for _, leaf := range leaves {
			p, err := compileCondx(leaf.row.Condition, leaf.no)
			if err != nil {
				valid = false
				break
			}
			if leaf.row.Negated {
				p = Not{Inner: p}
			}
			rows = append(rows, p)
			rowIsSeq = append(rowIsSeq, leaf.row.Condition.Target == CondxTargetSeq)
		}
		if !valid {
			continue
		}
		gate, err := aggregateBooleanGate(rule)
		if err != nil {
			continue
		}
		required := make(map[int]bool)
		for _, ta := range rule.Aggregate.Tracks {
			if ta.Row >= 1 && ta.Row <= len(rows) && types.roleRequired(rule.Aggregate.WorkType, ta.Role) {
				required[ta.Row] = true
			}
		}
		if len(required) == 0 {
			continue
		}
		plans = append(plans, &aggRulePlan{
			rule:        rule,
			rows:        rows,
			rowIsSeq:    rowIsSeq,
			gate:        gate,
			tracks:      rule.Aggregate.Tracks,
			requiredRow: required,
			bindingRows: metadataAssignedRows(rule),
			grouping:    groupingOf(rule),
			seq:         i,
		})
	}
	return plans
}

// groupingOf 取聚合规则并组所绑的分组项：「聚合成作品」显式绑定的目录分组
// （Aggregate.Grouping）在场即用之；缺省为标识声明的标识分组——键部分=标识
// 声明，按标识值等值并组，与聚合标识同一取值面。
func groupingOf(rule *RuleEntry) *GroupingBinding {
	if rule.Aggregate.Grouping != nil {
		return rule.Aggregate.Grouping
	}
	return &GroupingBinding{Mode: GroupingModeIdentity, Identity: rule.Identity}
}

// metadataAssignedRows 规则上被指派为元数据文件的行号集合：内容绑定行
// （行→维度配置即「元数据文件」指派——勾选即指派；轨行兼有绑定=双重身份，
// 照常导入）。绑定住规则级「元数据文件」产出落档位；无该产出或无绑定为空
// 集。消费面=聚合段的绑定行成员面（仅绑定行命中的文件=说明成员——归进
// 作品、无轨、不计必含轨单位）、扇出归属段的内容源候选面与内容采集/第二相
// 的行命中面。
func metadataAssignedRows(rule *RuleEntry) map[int]bool {
	if rule == nil || rule.MetadataFile == nil || len(rule.MetadataFile.ContentBindings) == 0 {
		return nil
	}
	rows := make(map[int]bool, len(rule.MetadataFile.ContentBindings))
	for _, b := range rule.MetadataFile.ContentBindings {
		if b.Row >= 1 {
			rows[b.Row] = true
		}
	}
	return rows
}

// —— 单文件的条件行命中面与标识值解析（分组求值两段的取值面）——
//
// 标识值的单一实现：聚合成作品所绑的标识分组、元数据文件扇出所绑的标识分组
// 与作品集聚合条目所绑的标识分组共用同一取值面——普通捕获名按行序跨行汇流
// （首个能供值的命中行）、序列群键专用名只对写入了群组产物的序列行取值。
// 各消费面经 groupingSelfKeyOf 调同一实现（三向回放全等锚：语义零分叉）。
//
// 序列群键保留名的取值源按消费产出形态授权：聚合成作品与「元数据文件」
// 分组绑定只读行内序列行群产物；产出四「作品集聚合」的分组项另可经库级群卡
// 键兜底（行内优先、库级次之）。库级取值源只经 groupKeySourceLibraryFallback
// 授权开启，行内独占入口语义不扩。

// groupKeySource 序列群键保留名的取值源授权形态（由消费产出形态静态选择，
// 不落档案）。
type groupKeySource int

const (
	// groupKeySourceInline 只读行内序列行群产物：聚合标识与「元数据文件」分组
	// 绑定的既有取值面。
	groupKeySourceInline groupKeySource = iota
	// groupKeySourceLibraryFallback 行内群产物优先、取不到时按成员路径反查库级
	// 群卡键：产出四「作品集聚合」的组键绑定专用，既有绑定形态不启。
	groupKeySourceLibraryFallback
)

// libraryGroupKeyIndex 库级序列群键反查索引（成员相对路径 → 群卡分组键）：由
// 分组层群卡按成员路径反查（同 workset_order.go buildGroupSeqIndex 口径：群卡
// 成员覆盖规则定群与结构群两类，含已识别成员——其文件行不落群组键、群归属只
// 在群卡），供产出四组键绑定的库级取值源使用。nil/缺项=该成员无库级群归属。
type libraryGroupKeyIndex map[string]string

// groupKeyOf 按成员相对路径反查库级群卡键。
func (idx libraryGroupKeyIndex) groupKeyOf(relPath string) (string, bool) {
	key, ok := idx[relPath]
	if !ok || key == "" {
		return "", false
	}
	return key, true
}

// put 收录一条成员群键（空路径或空键不计，nil 索引静默忽略）。
func (idx libraryGroupKeyIndex) put(relPath, key string) {
	if idx == nil || relPath == "" || key == "" {
		return
	}
	idx[relPath] = key
}

// aggFileRowHits 单文件在一条聚合规则下的条件行命中面：按行号下标存放该行独立
// 求值的作用域上下文（捕获绑定与序列群组产物；nil=该行未命中）。行间捕获不
// 互通——每行各自在独立作用域求值后原样保留，供标识与分轨按行号取用。
type aggFileRowHits []*ParseContext

// identityValueWithSource 标识值求值的单一实现：各部分按声明序以 NUL 分隔拼接
// （多部分复合标识防歧义）；任一部分取不到值即整体不可得（该文件不入组）。
// 序列群键保留名的取值源按消费形态授权（src）：行内独占或行内优先、库级兜底
// （见 identityPart）。
func (h aggFileRowHits) identityValueWithSource(parts []IdentityPart, src groupKeySource, relPath string, library libraryGroupKeyIndex) (string, bool) {
	if len(parts) == 0 {
		return "", false
	}
	var sb strings.Builder
	for i := range parts {
		v, ok := h.identityPart(&parts[i], src, relPath, library)
		if !ok {
			return "", false
		}
		if i > 0 {
			sb.WriteByte(0)
		}
		sb.WriteString(v)
	}
	return sb.String(), true
}

// identityPart 解析单个标识部分：按行序找首个命中且能供值的行——普通捕获名
// 取该行的前缀绑定（{作品} 一类共享槽跨轨行同名出现，声明行只是名义指向，
// 取值面按捕获名在命中行间汇流，视频各扩展行/字幕各扩展行取到同一标识；
// 仅命中部分行的文件经同名捕获入组——说明文件从自己的命中行取同名捕获，
// 与资源文件同键进分组）；序列群键专用名只对写入了群组产物的序列行取值
// （组内成员共享群键），库级取值源形态下行内取不到再按成员路径反查群卡键。
func (h aggFileRowHits) identityPart(part *IdentityPart, src groupKeySource, relPath string, library libraryGroupKeyIndex) (string, bool) {
	for row := 1; row <= len(h); row++ {
		ctx := h[row-1]
		if ctx == nil {
			continue
		}
		if part.Capture == identityGroupKeyCapture {
			if ctx.Group != nil {
				return ctx.Group.Key, true
			}
			continue
		}
		if v, ok := ctx.Capture(rowCaptureName(row, part.Capture)); ok {
			return v, true
		}
	}
	if part.Capture == identityGroupKeyCapture && src == groupKeySourceLibraryFallback {
		return library.groupKeyOf(relPath)
	}
	return "", false
}

// trackRoleOf 取该文件的轨道角色：分轨入口声明序中首个命中的行所分配的角色。
// 未命中任何分轨行返回空串——该文件不构成轨成员（仅命中标识源行的文件不入组）。
func (h aggFileRowHits) trackRoleOf(tracks []TrackAssign) string {
	for i := range tracks {
		ta := &tracks[i]
		if ta.Row >= 1 && ta.Row <= len(h) && h[ta.Row-1] != nil {
			return ta.Role
		}
	}
	return ""
}

// hitsRequiredRow 报告该文件是否命中任一必含轨行。
func (h aggFileRowHits) hitsRequiredRow(required map[int]bool) bool {
	for row := range required {
		if row >= 1 && row <= len(h) && h[row-1] != nil {
			return true
		}
	}
	return false
}

// hitsAnyRow 报告该文件是否命中行号集合中的任一行（集合为空或行号越界时
// 恒假）——被指派为元数据文件的行的命中面。
func (h aggFileRowHits) hitsAnyRow(rows map[int]bool) bool {
	for row := range rows {
		if row >= 1 && row <= len(h) && h[row-1] != nil {
			return true
		}
	}
	return false
}

// hitsAnyConditionRow 报告该文件是否命中任一条件行（纯平行行规则的规则命中判
// 据；布尔门未过的文件命中面为空，恒假）。
func (h aggFileRowHits) hitsAnyConditionRow() bool {
	for _, ctx := range h {
		if ctx != nil {
			return true
		}
	}
	return false
}

// —— 分组求值两段（键集 → 容器成员）——
//
// 一个分组项取哪些文件分两段算：先由产生行算出键集，再把「规则命中且自身键
// 落在键集内」的文件判为成员。成员不要求自己命中产生行——说明文件与图片同
// 目录即同组、同群键的其它行文件并入同一组；键集之外的文件即使命中规则也
// 不入该分组项。两段必须先后完整执行：键集未算全就判成员，成员面会依赖文件
// 遍历序（先见到的文件决定键集），故键集一次遍历算全后另起一次遍历判成员，
// 成员面与输入序无关。

// groupingGroups 分组求值两段的结论：组键 → 成员文件下标。成员面（哪个文件
// 属于哪个组）与文件输入序无关；组序与组内成员序沿用求值面顺序（组按组内首
// 成员出现序、成员按求值面下标升序），与各产出的既有归集序同一口径。
type groupingGroups struct {
	// order 为组键序列，按组内首成员在求值面的出现序。
	order []string
	// groups 为组键 → 成员文件下标（求值面序）。
	groups map[string][]int
}

// groupingKeySetOf 分组求值两段的第一段：算出所绑分组项的键集。第二个返回
// 值报告键集是否约束成员——约束时只有自身键落在键集内的文件才是成员：
//   - 所在目录分组：产生行=绑定的行号，键集=该行命中文件的所在目录（目录聚
//     合键，同目录文件同键）；
//   - 单序列群键分组（标识分组恰一个部分且为序列群键保留名）：产生行=该部
//     分的行号，键集=该行命中且产出了群组键的文件所产出的群键；
//   - 手写拼接（标识分组的其余形态）：无键集，成员只看自身键是否可得（自
//     产生），返回 nil、false。
//
// 产生行缺席（行号小于 1 或超出该文件命中面的行数）时键集为空而非缺席：
// 没有产生行就没有文件能产出键，空键集之下无成员，不回退到任何其它行。
// 合法形态（产生行必填、在条件行范围内、目标相符）由档案载入校验保证，只有
// 未经载入校验的内存规则才会出现产生行缺席；本函数只保证不越界、不臆造键。
// hits 与 files 按下标对齐；文件下标越界或行命中上下文缺席（布尔门未过、未
// 被求值的文件）不计。
func groupingKeySetOf(binding *GroupingBinding, files []ScanFile, hits []aggFileRowHits) (map[string]bool, bool) {
	row, container := groupingProducingRowOf(binding)
	if !container {
		return nil, false
	}
	keys := make(map[string]bool)
	for i := range files {
		ctx := groupingRowHit(hits, i, row)
		if ctx == nil {
			continue
		}
		if binding.Mode == GroupingModeDirectory {
			keys[dirGroupKey(files[i].DirNames)] = true
		} else if ctx.Group != nil {
			keys[ctx.Group.Key] = true
		}
	}
	return keys, true
}

// groupingProducingRowOf 取分组绑定的产生行号，并报告该分组项是否带键集：
// 所在目录分组取绑定的行号；恰一个序列群键部分的标识分组取该部分的行号——
// 两者都由产生行产出键集；其余标识分组为手写拼接，成员各自产出键（无产生
// 行、无键集），返回 0、false。绑定缺席同样返回 0、false。
func groupingProducingRowOf(binding *GroupingBinding) (int, bool) {
	switch {
	case binding == nil:
		return 0, false
	case binding.Mode == GroupingModeDirectory:
		return binding.Row, true
	case len(binding.Identity) == 1 && binding.Identity[0].Capture == identityGroupKeyCapture:
		return binding.Identity[0].Row, true
	default:
		return 0, false
	}
}

// groupingRowHit 取某文件命中面上产生行的命中上下文：文件下标越界、行号小于
// 1、超出该文件命中面的行数、该行未命中，均返回 nil（命中面在布尔门未过或
// 未被求值的文件上为空）。
func groupingRowHit(hits []aggFileRowHits, file, row int) *ParseContext {
	if row < 1 || file < 0 || file >= len(hits) || row > len(hits[file]) {
		return nil
	}
	return hits[file][row-1]
}

// groupingSelfKeyOf 求文件在所绑分组项下的自身键（带键集分组的第二段与手写
// 拼接共用的取值面）：所在目录分组=所在目录的目录聚合键（恒可得）；标识分组
// =键部分的复合标识值——普通捕获名按行序跨行汇流取首个能供值的命中行（声明
// 行号只是名义指向），序列群键保留名取群组键，取值源按消费产出形态授权（行
// 内独占，或行内优先、库级群卡键兜底）。任一部分取不到值即不可得。
func groupingSelfKeyOf(h aggFileRowHits, binding *GroupingBinding, file ScanFile, src groupKeySource, library libraryGroupKeyIndex) (string, bool) {
	if binding.Mode == GroupingModeDirectory {
		return dirGroupKey(file.DirNames), true
	}
	return h.identityValueWithSource(binding.Identity, src, file.RelPath, library)
}

// groupingMembersOf 分组求值两段的单一实现：按所绑分组项把规则命中的文件
// 归组。hits 与 matched 均与 files 按下标对齐：
//   - hits：各文件在所绑规则下的条件行命中面（键集与自身键的取值面）；
//   - matched：各文件是否命中所绑规则——规则命中集的判据归调用方（纯平行行
//     =行命中池任一行命中；布尔组合形态=整条求值门通过，此时行命中面可全
//     空），越界读取按未命中处理；
//   - src 与 library：序列群键保留名的取值源授权（与标识值求值同一口径）。
//
// 成员判定：规则命中且自身键可得，并且自身键落在键集内（带键集的分组项）
// ——手写拼接无键集，自身键可得即成员、同键成组。键集为空（产生行缺席或从未
// 命中）时无成员；绑定缺席（nil）无分组可依，同样无成员。
func groupingMembersOf(binding *GroupingBinding, files []ScanFile, hits []aggFileRowHits, matched []bool, src groupKeySource, library libraryGroupKeyIndex) groupingGroups {
	res := groupingGroups{groups: make(map[string][]int)}
	if binding == nil {
		return res
	}
	keys, constrained := groupingKeySetOf(binding, files, hits)
	for i := range files {
		if i >= len(matched) || !matched[i] {
			continue
		}
		var h aggFileRowHits
		if i < len(hits) {
			h = hits[i]
		}
		key, ok := groupingSelfKeyOf(h, binding, files[i], src, library)
		if !ok || (constrained && !keys[key]) {
			continue
		}
		if _, seen := res.groups[key]; !seen {
			res.order = append(res.order, key)
		}
		res.groups[key] = append(res.groups[key], i)
	}
	return res
}

// —— 并组主算法 ——

// aggAssignment 单文件的聚合归属与内容扇出结论：WorkKey 为聚合作品键（"f:"+
// 主导文件相对路径，与选择域的作品键同形——键前缀维持单一）；TrackRole 为
// 本文件的轨道角色；WorkType 为选中规则声明的作品类型（聚合定义的作品类型，
// 消费面=创建链子行 ResourceType）；Companion 为说明成员标记（聚合段产：仅
// 绑定行命中而归进聚合作品的成员——归进作品、无轨、不导入；扇出归属段产：
// 仅绑定行命中、组内有落点的元数据源文件——不导入、不独立成件）；轨道成员
// （含轨行兼绑定的双重身份成员）照常导入不带标记。WorkKey 空=无归属（独立
// 作品，其余归属字段同空）；元数据源文件不获聚合作品键（其同时为聚合成
// 作品组成员时除外——成员化只属于聚合产出本职）。FanoutWorkKeys 为内容
// 扇出键集：该文件按分组绑定扇出到达的全部作品键（已被聚合收编=其聚合
// 作品键、未收编=自身路径键，按组内文件序去重）——内容声明只按此键集
// 流动；空=无扇出（内容不经扇出面承载）。
type aggAssignment struct {
	WorkKey        string
	TrackRole      string
	WorkType       string
	Companion      bool
	FanoutWorkKeys []string
}

// aggregateGrouping 标识等值并组+扇出归属解析（纯函数、输入序确定）。两段同位
// 挂汇流后、扇出段居聚合段之后：
//
//   - 条件行按既有行求值机制逐文件求命中（捕获绑定、目录命中、序列群组分析全部
//     走同一 Pattern.Evaluate——目录捕获的目录间共享与序列群键的组内共享由此
//     自然成立：同目录文件解析出同名目录捕获值、同群组成员解析出同群键）；
//     布尔组合形态（括号子组/行级非在场）另在行命中面之上加整条布尔求值门
//     （gate，见 aggregateBooleanGate）——门不过的文件不入命中面（分组
//     作用于匹配求值后的命中集）；纯平行行不设门，行命中池 OR 汇聚即
//     命中面（存量聚合规则的既有语义逐字节等值）；
//   - 成员判定=命中分轨行（轨道行，导入为资源；兼有内容绑定的轨行=双重
//     身份，照常导入+内容来源，导入身份不受绑定影响）或命中绑定行（仅绑定
//     未分轨=说明成员——归进作品、无轨、不计必含轨单位）；
//   - 分组按所绑分组项走分组求值两段（见 groupingMembersOf）：显式绑定目录
//     分组时成员只收键集内目录的文件、键集外的命中文件不入组；缺省绑定标识
//     声明时按标识值等值并组（文件名同名/序列成组/目录成组三种形态同一
//     语法）；
//   - 必含轨行每组恰一命中才成立：文件/目录行按命中文件数计，序列行按相异群键
//     数计（集合轨=整序列组 N 存储位计 1 单位）；零命中=必含轨对该组不适用、
//     静默不落件，多命中（≥2）即歧义——该组不成立、成员维持独立作品（宁缺勿
//     错），并经 warnf 告警点名（nil=不告警）；
//   - 主导文件=首个必含命中单位的首个成员（扫描序），作品键=其相对路径；
//   - 规则间按档案序先到先得：已被在先规则归属的文件不再改归属（与分组段首个
//     命中定群同款取向）；歧义组成员仍可被后续规则接纳。
//
// 非必含轨行零命中=该轨缺席、作品仍成立（单轨形态）；角色基数由类型规约定、
// 并组不按 Max 截断（聚合表达力不被消费面拖住）。聚合段之后接扇出归属段
// （一切含「元数据文件」产出与绑定的启用规则的内容扇出与元数据源标记——
// 单用/并用同通道，见 fanoutAttribution）：聚合归属优先（扇出不夺已归属
// 成员）、扇出不并作品。
func aggregateGrouping(surface *aggEvalSurface, rules []RuleEntry, lib *LibraryIndex, types *resourceTypeIndex, warnf func(string, ...any)) []aggAssignment {
	n := len(surface.files)
	out := make([]aggAssignment, n)
	// owner 为各文件的归属赋与规则下标（-1=未归属）：扇出归属段辨认「归属
	// 是否本规则聚合段所赋」的凭据（先到先得的竞争面=归属与内容源认领）。
	owner := make([]int, n)
	for i := range owner {
		owner[i] = -1
	}
	for _, plan := range buildAggregationPlans(rules, types) {
		// 逐文件逐条件行求命中（每行独立作用域，捕获与群组产物留在本行上下文）；
		// 布尔门先裁（门不过=该文件不在本规则命中集，行求值跳过）。
		hits := make([]aggFileRowHits, n)
		for i := range surface.files {
			file := surface.files[i]
			in := surface.matchInputOf(file)
			if plan.gate != nil {
				gateCtx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
				if !plan.gate.Evaluate(in, lib, gateCtx) {
					continue
				}
			}
			rowHits := make(aggFileRowHits, len(plan.rows))
			for j, row := range plan.rows {
				ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
				if row.Evaluate(in, lib, ctx) {
					rowHits[j] = ctx
				}
			}
			hits[i] = rowHits
		}
		// 规则命中面：命中分轨行（轨道成员）或绑定行（仅绑定行命中=说明成员面
		// ——归进作品、无轨、不计必含轨单位）；已被在先规则归属的文件不再参与
		// 本规则归组（先到先得）。
		matched := make([]bool, n)
		for i := range surface.files {
			if out[i].WorkKey != "" {
				continue
			}
			if hits[i].trackRoleOf(plan.tracks) == "" && !hits[i].hitsAnyRow(plan.bindingRows) {
				continue
			}
			matched[i] = true
		}
		// 分组：按所绑分组项走分组求值两段——目录分组只收键集内目录的成员
		// （键集外的文件不入组、各自成独立作品），标识手写分组按标识值等值
		// 并组。组序=组内首成员出现序，组内成员按扫描序（确定性）。
		groups := groupingMembersOf(plan.grouping, surface.files, hits, matched, groupKeySourceInline, nil)
		// 逐组判定必含轨恰一并落轨（作品键+轨道角色写入各成员归属）。
		for _, v := range groups.order {
			members := groups.groups[v]
			units := 0
			lead := -1
			for row := 1; row <= len(plan.rows) && units < 2; row++ {
				if !plan.requiredRow[row] {
					continue
				}
				seenGroupKeys := make(map[string]bool)
				for _, i := range members {
					ctx := hits[i][row-1]
					if ctx == nil {
						continue
					}
					if plan.rowIsSeq[row-1] {
						// 集合轨：同一序列群的成员共享群键，相异群键各计 1 单位。
						if ctx.Group == nil || seenGroupKeys[ctx.Group.Key] {
							continue
						}
						seenGroupKeys[ctx.Group.Key] = true
					}
					units++
					if lead < 0 {
						lead = i
					}
					if units >= 2 {
						break
					}
				}
			}
			if units != 1 {
				// 必含轨恰一才成立：该组不落件、成员维持独立作品（宁缺勿错）。
				// 零命中=组内没有文件命中必含轨行（纯说明成员成组、或可选轨文
				// 件单独成组），是必含轨对该组不适用的正常形态，静默不落件；
				// 多命中（≥2）才是歧义——目录分组下一目录内多个文件命中同一必
				// 含轨行即落入此处，不静默，告警点名规则、分组与单位数。告警整
				// 句先格式化完毕、以单一字串实参交付：日志适配层会把实参转成
				// 字串再套格式串，数值实参直传 %d 会被打印成 %!d(string=N)。
				if units >= 2 && warnf != nil {
					warnf("%s", fmt.Sprintf("聚合规则 %q 的分组 %q 必含轨命中单位数为 %d（恰一才成立），该组不成立、成员按独立作品导入",
						plan.rule.ID, v, units))
				}
				continue
			}
			workKey := "f:" + surface.files[lead].RelPath
			workType := plan.rule.Aggregate.WorkType
			for _, i := range members {
				// 成员=轨行命中（照常落轨导入；轨行兼绑定的双重身份成员照常
				// 落轨）或仅绑定行命中的说明成员（归进作品、无轨、不导入、
				// 不计必含轨单位——说明成员标记承载处置）。
				role := hits[i].trackRoleOf(plan.tracks)
				out[i] = aggAssignment{WorkKey: workKey, TrackRole: role, WorkType: workType, Companion: role == ""}
				owner[i] = plan.seq
			}
		}
	}
	fanoutAttribution(surface, rules, lib, out, owner, warnf)
	return out
}

// —— 扇出归属段（一切含「元数据文件」产出与内容绑定的启用规则的内容扇出
// 与元数据源标记；与聚合段同位挂汇流后、居其后——单用/并用同通道）——

// fanoutRulePlan 扇出归属求值预案：全部条件行（先序展平叶子行）编译为独立
// 求值的行包装模式（与聚合段条件行同一编译口径——分组键的取值面按捕获名在
// 命中行间汇流，见 groupingMembersOf），加上绑定行集（内容源候选
// 的命中面）与目标行集（作品键推导面：非绑定的正向文件/序列行——目录行不在
// 此列，目录分组下只命中目录行的成员另按容器成员判定落点，见
// fanoutAttribution）与生效分组绑定；布尔组合形态（括号子组/行级非
// 在场）另编译整条求值门（与聚合段同一接线）。
type fanoutRulePlan struct {
	// rule 为源规则（引用传入规则序列的条目）。
	rule *RuleEntry
	// seq 为规则在规则表中的下标（先到先得的认领凭据）。
	seq int
	// rows 为条件行模式（下标+1=先序展平行号）。
	rows []Pattern
	// gate 为布尔组合形态的整条求值门（nil=纯平行行不设门——见
	// aggregateBooleanGate）。
	gate Pattern
	// bindingRows 为被内容绑定勾选的行号集合（命中文件=内容源候选；兼有
	// 目标行命中=双重身份，照常承载作品+内容扇出）。
	bindingRows map[int]bool
	// targetRows 为可推导作品键的行号集合（非绑定的正向文件/序列目标行）。
	targetRows map[int]bool
	// grouping 为生效分组绑定：显式声明在场即用之；缺省（并用且聚合产出在场）
	// =聚合成作品所绑分组（显式目录分组，否则聚合标识的等值形态——求值期便利
	// 缺省，档案不落物化）。
	grouping *GroupingBinding
}

// buildFanoutPlans 过滤扇出规则并编译预案：一切含「元数据文件」产出与内容
// 绑定、条件行在场的启用规则——含并用形态（分组绑定可独立于聚合标识声明：
// 并用缺省绑聚合成作品所绑分组，显式可换）。映射单用缺分组绑定不进预案
// （载入校验已拒，双保险）。行编译或布尔门编译失败按宁缺勿错整条跳过——
// 与聚合段同款防御（载入校验已拦的形态在此双保险；括号子组/行级非照常入
// 预案——行级命中照常按行）。
func buildFanoutPlans(rules []RuleEntry) []*fanoutRulePlan {
	var plans []*fanoutRulePlan
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.MetadataFile == nil ||
			len(rule.MetadataFile.ContentBindings) == 0 ||
			len(rule.Conditions) == 0 {
			continue
		}
		grouping := rule.MetadataFile.Grouping
		if grouping == nil {
			// 并用缺省=聚合成作品所绑分组（求值期解析，档案不物化）：显式绑定
			// 的目录分组在场即用之，否则取标识声明的标识分组；无聚合产出（映射
			// 单用）或聚合产出两种绑定皆无时无缺省可依，不进预案。
			if rule.Aggregate == nil || (rule.Aggregate.Grouping == nil && len(rule.Identity) == 0) {
				continue
			}
			grouping = groupingOf(rule)
		}
		leaves := condxLeafRowsOf(rule.Conditions)
		if len(leaves) == 0 {
			continue
		}
		rows := make([]Pattern, 0, len(leaves))
		valid := true
		for _, leaf := range leaves {
			p, err := compileCondx(leaf.row.Condition, leaf.no)
			if err != nil {
				valid = false
				break
			}
			if leaf.row.Negated {
				p = Not{Inner: p}
			}
			rows = append(rows, p)
		}
		if !valid {
			continue
		}
		gate, err := aggregateBooleanGate(rule)
		if err != nil {
			continue
		}
		bindingRows := metadataAssignedRows(rule)
		targetRows := make(map[int]bool, len(rows))
		for _, leaf := range leaves {
			if bindingRows[leaf.no] || leaf.row.Negated {
				// 行级非行是过滤行（命中=内层未中），不承载作品键推导——
				// 目标行只认正向的非绑定文件/序列行。
				continue
			}
			if target := leaf.row.Condition.Target; target == CondxTargetFile || target == CondxTargetSeq {
				targetRows[leaf.no] = true
			}
		}
		plans = append(plans, &fanoutRulePlan{
			rule:        rule,
			seq:         i,
			rows:        rows,
			gate:        gate,
			bindingRows: bindingRows,
			targetRows:  targetRows,
			grouping:    grouping,
		})
	}
	return plans
}

// fanoutAttribution 扇出归属段的解析主算法（聚合段之后逐预案执行，预案按
// 档案序；warnf 为运行期告警回调，nil=不告警）：
//
//   - 参与判定：布尔组合形态（括号子组/行级非在场）先过整条求值门——门不过
//     =不在本规则命中集；命中绑定行或目标行的文件参与归组；分组带键集（目录
//     分组、单序列群键分组）时规则命中的文件一律参与——成员＝规则命中 ∧ 自身
//     键 ∈ 键集，目录行对目录下所有文件成立，只命中目录行的资源文件也是容器
//     成员（手写拼接分组无键集，参与面不扩、与既有同口径）；
//   - 分组：按生效分组绑定走分组求值两段（见 groupingMembersOf）——标识
//     分组=键部分的复合值（捕获值相等即一组，共享捕获跨行汇流——与聚合并组
//     的标识汇流同一实现）；所在目录分组=目录聚合键，成员只收键集内目录的
//     文件（键集由所绑目录行命中文件的所在目录构成）；分组键不可得=不属任何
//     分组，按独立作品导入（孤文件回退的分组键不可得形态）；
//   - 键集推导：组内作为内容落点的文件各推出一个作品键（已被聚合收编=其聚合
//     作品键；未收编=自身路径键，即该文件自己成一件独立作品）——落点=命中目
//     标行的成员，加上目录分组下命中目录产生行却未命中任何绑定行的成员（只
//     命中目录行的文件没有自己的文件行，仍是目录容器内的资源）；仅绑定行命
//     中的成员是内容源、不是落点。键集=组内全部作品的键（按文件序去重）。
//     组内无落点（键集空）→ 绑定行文件不作元数据源、按独立作品导入（孤文件
//     回退——防误把资源吞成说明文件）；
//   - 元数据源标记与扇出：组内绑定行命中的文件其内容声明扇出到键集全部
//     作品（各自成件=各带一份、被聚合成一件=那件带一份；轨行兼绑定的双重
//     身份文件内容同样扇出，作品承载身份不受影响）。仅绑定行命中的未收编
//     文件=元数据源（不导入、不独立成件、带说明成员标记）；归属竞争按先
//     到先得：已被在先规则认领为内容源（扇出键集在场）或被他规则聚合收编
//     的文件不再改判；本规则聚合段所赋的归属（并用绑定行成员=聚合成作品
//     组成员、无轨成员）照常保留，只补扇出键集。
func fanoutAttribution(surface *aggEvalSurface, rules []RuleEntry, lib *LibraryIndex, out []aggAssignment, owner []int, warnf func(string, ...any)) {
	for _, plan := range buildFanoutPlans(rules) {
		// 逐文件逐行求命中（每行独立作用域，捕获与群组产物留在本行上下文）；
		// 布尔门先裁（门不过=该文件不在本规则命中集，行求值跳过）。
		hits := make([]aggFileRowHits, len(surface.files))
		// ruleHit 为各文件是否命中所绑规则（分组求值内核的规则命中口径：布尔
		// 门通过即命中；纯平行行任一行命中即命中）。
		ruleHit := make([]bool, len(surface.files))
		for i := range surface.files {
			file := surface.files[i]
			in := surface.matchInputOf(file)
			if plan.gate != nil {
				gateCtx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
				if !plan.gate.Evaluate(in, lib, gateCtx) {
					continue
				}
			}
			rowHits := make(aggFileRowHits, len(plan.rows))
			for j, row := range plan.rows {
				ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
				if row.Evaluate(in, lib, ctx) {
					rowHits[j] = ctx
				}
			}
			hits[i] = rowHits
			ruleHit[i] = plan.gate != nil || rowHits.hitsAnyConditionRow()
		}
		// 所绑分组带键集（目录分组、单序列群键分组）时成员面按「规则命中 ∧
		// 自身键 ∈ 键集」收：只命中目录行的资源文件也是容器成员。dirRow 为目
		// 录分组的产生行（其余分组为 0）。
		_, container := groupingProducingRowOf(plan.grouping)
		dirRow := 0
		if plan.grouping != nil && plan.grouping.Mode == GroupingModeDirectory {
			dirRow = plan.grouping.Row
		}
		// 参与面：命中绑定行（内容源候选）或目标行（作品键推导面）的文件参与
		// 归组；带键集的分组另收规则命中的文件。布尔门不过的文件行命中面为
		// 空、也不在规则命中集，自然落选。
		matched := make([]bool, len(surface.files))
		for i := range surface.files {
			if hits[i].hitsAnyRow(plan.bindingRows) || hits[i].hitsAnyRow(plan.targetRows) || (container && ruleHit[i]) {
				matched[i] = true
			}
		}
		// 内容落点：命中目标行的成员（兼有内容绑定的双重身份文件照常是落
		// 点），以及目录分组下命中产生行却未命中任何绑定行的成员。仅绑定行命
		// 中的成员是内容源、不是落点。
		isContentLanding := func(i int) bool {
			if hits[i].hitsAnyRow(plan.targetRows) {
				return true
			}
			return dirRow >= 1 && !hits[i].hitsAnyRow(plan.bindingRows) && groupingRowHit(hits, i, dirRow) != nil
		}
		// 分组：按生效分组绑定走分组求值两段——组序=组内首成员出现序，组内
		// 成员按求值面下标升序（确定性）。说明文件自身键须落在键集内（容器
		// 语义）才算组成员；不在任何组（键不可得或键集外）走孤文件回退。
		groups := groupingMembersOf(plan.grouping, surface.files, hits, matched, groupKeySourceInline, nil)
		for _, key := range groups.order {
			members := groups.groups[key]
			// 键集=组内落点文件推出的作品键全集（已被聚合收编=其聚合作品
			// 键；未收编=自身路径键），按组内文件序去重（确定性）。
			var keys []string
			seen := make(map[string]bool, len(members))
			for _, i := range members {
				if !isContentLanding(i) {
					continue
				}
				k := out[i].WorkKey
				if k == "" {
					k = "f:" + surface.files[i].RelPath
				}
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
			if len(keys) == 0 {
				// 孤文件回退：组内全部为绑定行命中（内容无落点）——绑定行
				// 文件按独立作品导入，不作元数据源。
				continue
			}
			for _, i := range members {
				if !hits[i].hitsAnyRow(plan.bindingRows) {
					continue
				}
				if len(out[i].FanoutWorkKeys) > 0 || (out[i].WorkKey != "" && owner[i] != plan.seq) {
					// 已被在先规则认领为内容源或被他规则聚合收编——先到
					// 先得，不再改判。
					continue
				}
				out[i].FanoutWorkKeys = keys
				if out[i].WorkKey == "" && !isContentLanding(i) {
					// 仅绑定行命中的未收编文件=元数据源：不导入、不独立成件
					//（说明成员标记承载处置）；双重身份文件照常承载作品。
					out[i].Companion = true
					owner[i] = plan.seq
				}
			}
		}
	}
}
