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
	// identity 为作品标识声明。
	identity []IdentityPart
	// tracks 为分轨入口（条件行→角色分配）。
	tracks []TrackAssign
	// requiredRow 为必含轨行号集合（所分配角色在类型规约中 Min≥1）。
	requiredRow map[int]bool
	// bindingRows 为被内容绑定勾选的行号集合（聚合成员判定的绑定行面：仅
	// 绑定行命中的文件=说明成员——归进作品、无轨、不计必含轨单位；轨行兼
	// 绑定的双重身份成员照常落轨）。
	bindingRows map[int]bool
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
			identity:    rule.Identity,
			tracks:      rule.Aggregate.Tracks,
			requiredRow: required,
			bindingRows: metadataAssignedRows(rule),
			seq:         i,
		})
	}
	return plans
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

// —— 单文件的条件行命中面与分组键解析（统一分组求值器）——
//
// 分组键求值的单一实现：聚合标识（规则级 Identity）与元数据文件分组绑定
// （MetadataFileProduction.Grouping）共用同一取值面——普通捕获名按行序跨行
// 汇流（首个能供值的命中行）、序列群键专用名只对写入了群组产物的序列行
// 取值、所在目录形态=目录聚合键。聚合侧与扇出侧的等值要求（三向回放全等
// 锚）：两消费面调同一实现，语义零分叉。

// aggFileRowHits 单文件在一条聚合规则下的条件行命中面：按行号下标存放该行独立
// 求值的作用域上下文（捕获绑定与序列群组产物；nil=该行未命中）。行间捕获不
// 互通——每行各自在独立作用域求值后原样保留，供标识与分轨按行号取用。
type aggFileRowHits []*ParseContext

// identityValue 解析该文件在本规则下的标识值：各部分按声明序以 NUL 分隔拼接
// （多部分复合标识防歧义）；任一部分取不到值即整体不可得（该文件不入组）。
// 聚合标识与元数据文件分组绑定（标识分组形态）共用本实现。
func (h aggFileRowHits) identityValue(parts []IdentityPart) (string, bool) {
	if len(parts) == 0 {
		return "", false
	}
	var sb strings.Builder
	for i := range parts {
		v, ok := h.identityPart(&parts[i])
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
// （组内成员共享群键）。
func (h aggFileRowHits) identityPart(part *IdentityPart) (string, bool) {
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
	return "", false
}

// groupingKeyOf 按分组绑定声明求该文件的分组键（统一分组求值器的扇出侧
// 入口）：标识分组=键部分的复合值（identityValue——与聚合标识同一实现，
// 含序列群键保留名的取值面）；所在目录分组=目录聚合键（同目录文件同键，
// 任意名说明文件与资源文件同组）。任一部分取不到值即整体不可得（调用方
// 按不属任何分组处理——孤文件回退的分组键不可得形态）。
func (h aggFileRowHits) groupingKeyOf(binding *GroupingBinding, dirNames []string) (string, bool) {
	if binding.Mode == GroupingModeDirectory {
		return dirGroupKey(dirNames), true
	}
	return h.identityValue(binding.Identity)
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
//     未分轨=说明成员——归进作品、无轨、不计必含轨单位）；聚合分组键
//     （标识）取值面来自轨行命中与行间捕获汇流、口径不变；
//   - 标识值相等者并成一件作品（文件名同名/序列成组/目录成组三种形态同一语法）；
//   - 必含轨行每组恰一命中才成立：文件/目录行按命中文件数计，序列行按相异群键
//     数计（集合轨=整序列组 N 存储位计 1 单位）；≠1 即歧义（含零命中），该组
//     不成立、成员维持独立作品（宁缺勿错）；
//   - 主导文件=首个必含命中单位的首个成员（扫描序），作品键=其相对路径；
//   - 规则间按档案序先到先得：已被在先规则归属的文件不再改归属（与分组段首个
//     命中定群同款取向）；歧义组成员仍可被后续规则接纳。
//
// 非必含轨行零命中=该轨缺席、作品仍成立（单轨形态）；角色基数由类型规约定、
// 并组不按 Max 截断（聚合表达力不被消费面拖住）。聚合段之后接扇出归属段
// （一切含「元数据文件」产出与绑定的启用规则的内容扇出与元数据源标记——
// 单用/并用同通道，见 fanoutAttribution）：聚合归属优先（扇出不夺已归属
// 成员）、扇出不并作品。
func aggregateGrouping(surface *aggEvalSurface, rules []RuleEntry, lib *LibraryIndex, types *resourceTypeIndex) []aggAssignment {
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
		// 成员收集：标识可得、命中分轨行或绑定行（仅绑定行命中=说明成员面
		// ——归进作品、无轨、不计必含轨单位）、且尚未被在先规则归属；组序=
		// 标识值首成员出现序，组内成员按扫描序（确定性）。
		type aggMemberGroup struct {
			members []int
		}
		groups := make(map[string]*aggMemberGroup)
		var order []string
		for i := range surface.files {
			if out[i].WorkKey != "" {
				continue
			}
			role := hits[i].trackRoleOf(plan.tracks)
			if role == "" && !hits[i].hitsAnyRow(plan.bindingRows) {
				continue
			}
			v, ok := hits[i].identityValue(plan.identity)
			if !ok {
				continue
			}
			g := groups[v]
			if g == nil {
				g = &aggMemberGroup{}
				groups[v] = g
				order = append(order, v)
			}
			g.members = append(g.members, i)
		}
		// 逐组判定必含轨恰一并落轨（作品键+轨道角色写入各成员归属）。
		for _, v := range order {
			g := groups[v]
			units := 0
			lead := -1
			for row := 1; row <= len(plan.rows) && units < 2; row++ {
				if !plan.requiredRow[row] {
					continue
				}
				seenGroupKeys := make(map[string]bool)
				for _, i := range g.members {
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
				continue
			}
			workKey := "f:" + surface.files[lead].RelPath
			workType := plan.rule.Aggregate.WorkType
			for _, i := range g.members {
				// 成员=轨行命中（照常落轨导入；轨行兼绑定的双重身份成员照常
				// 落轨）或仅绑定行命中的说明成员（归进作品、无轨、不导入、
				// 不计必含轨单位——说明成员标记承载处置）。
				role := hits[i].trackRoleOf(plan.tracks)
				out[i] = aggAssignment{WorkKey: workKey, TrackRole: role, WorkType: workType, Companion: role == ""}
				owner[i] = plan.seq
			}
		}
	}
	fanoutAttribution(surface, rules, lib, out, owner)
	return out
}

// —— 扇出归属段（一切含「元数据文件」产出与内容绑定的启用规则的内容扇出
// 与元数据源标记；与聚合段同位挂汇流后、居其后——单用/并用同通道）——

// fanoutRulePlan 扇出归属求值预案：全部条件行（先序展平叶子行）编译为独立
// 求值的行包装模式（与聚合段条件行同一编译口径——分组键的取值面按捕获名在
// 命中行间汇流，见 aggFileRowHits.groupingKeyOf），加上绑定行集（内容源候选
// 的命中面）与目标行集（作品键推导面：非绑定的正向文件/序列行——目录行只
// 作分组键源，不推导作品键）与生效分组绑定；布尔组合形态（括号子组/行级非
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
	// grouping 为生效分组绑定：显式声明在场即用之；缺省（并用且聚合标识
	// 在场）=聚合标识的等值形态（标识分组、键部分=聚合标识——求值期便利
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
			if rule.Aggregate == nil || len(rule.Identity) == 0 {
				continue
			}
			grouping = &GroupingBinding{Mode: GroupingModeIdentity, Identity: rule.Identity}
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
// 档案序）：
//
//   - 参与判定：布尔组合形态（括号子组/行级非在场）先过整条求值门——门不过
//     =不在本规则命中集；命中绑定行或目标行、且分组键可得的文件入组（分组
//     键取不到=不属任何分组，按独立作品导入——孤文件回退的分组键不可得
//     形态）；
//   - 分组键：标识分组=绑定键部分的复合值（捕获值相等即一组，共享捕获跨行
//     汇流——与聚合并组的标识汇流同一实现）；所在目录分组=所在目录（任意
//     名说明文件与资源文件同目录即同组）；
//   - 键集推导：组内目标行命中的文件各推出一个作品键（已被聚合收编=其聚合
//     作品键；未收编=自身路径键，即该文件自己成一件独立作品）——键集=组内
//     全部作品的键（按文件序去重）。组内无目标行命中文件（键集空=内容无
//     落点）→ 绑定行文件不作元数据源、按独立作品导入（孤文件回退——防误
//     把资源吞成说明文件）；
//   - 元数据源标记与扇出：组内绑定行命中的文件其内容声明扇出到键集全部
//     作品（各自成件=各带一份、被聚合成一件=那件带一份；轨行兼绑定的双重
//     身份文件内容同样扇出，作品承载身份不受影响）。仅绑定行命中的未收编
//     文件=元数据源（不导入、不独立成件、带说明成员标记）；归属竞争按先
//     到先得：已被在先规则认领为内容源（扇出键集在场）或被他规则聚合收编
//     的文件不再改判；本规则聚合段所赋的归属（并用绑定行成员=聚合成作品
//     组成员、无轨成员）照常保留，只补扇出键集。
func fanoutAttribution(surface *aggEvalSurface, rules []RuleEntry, lib *LibraryIndex, out []aggAssignment, owner []int) {
	for _, plan := range buildFanoutPlans(rules) {
		// 逐文件逐行求命中（每行独立作用域，捕获与群组产物留在本行上下文）；
		// 布尔门先裁（门不过=该文件不在本规则命中集，行求值跳过）。
		hits := make([]aggFileRowHits, len(surface.files))
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
		type fanoutMemberGroup struct {
			members []int
		}
		groups := make(map[string]*fanoutMemberGroup)
		var order []string
		for i := range surface.files {
			if !hits[i].hitsAnyRow(plan.bindingRows) && !hits[i].hitsAnyRow(plan.targetRows) {
				continue
			}
			key, ok := hits[i].groupingKeyOf(plan.grouping, surface.files[i].DirNames)
			if !ok {
				continue
			}
			g := groups[key]
			if g == nil {
				g = &fanoutMemberGroup{}
				groups[key] = g
				order = append(order, key)
			}
			g.members = append(g.members, i)
		}
		for _, key := range order {
			g := groups[key]
			// 键集=组内目标行命中文件推出的作品键全集（已被聚合收编=其聚合
			// 作品键；未收编=自身路径键），按组内文件序去重（确定性）。
			var keys []string
			seen := make(map[string]bool, len(g.members))
			for _, i := range g.members {
				if !hits[i].hitsAnyRow(plan.targetRows) {
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
			for _, i := range g.members {
				if !hits[i].hitsAnyRow(plan.bindingRows) {
					continue
				}
				if len(out[i].FanoutWorkKeys) > 0 || (out[i].WorkKey != "" && owner[i] != plan.seq) {
					// 已被在先规则认领为内容源或被他规则聚合收编——先到
					// 先得，不再改判。
					continue
				}
				out[i].FanoutWorkKeys = keys
				if out[i].WorkKey == "" && !hits[i].hitsAnyRow(plan.targetRows) {
					// 仅绑定行命中的未收编文件=元数据源：不导入、不独立成件
					//（说明成员标记承载处置）；双重身份文件照常承载作品。
					out[i].Companion = true
					owner[i] = plan.seq
				}
			}
		}
	}
}
