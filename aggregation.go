// 聚合求值内核的并组算法：聚合规则（轨行+作品标识声明+分轨入口）对求值面做
// 标识等值并组——轨行复用既有条件行求值机制（文件名/目录名/序列/引用四种源
// 的行包装求值与捕获绑定），并组判定为纯函数（输入=扫描事实+规则档案+资源
// 类型注册表投影，无 IO、输入序确定）。求值挂点见 pipeline.go 的聚合段。
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
// 聚合段的轨行求值由此取得与逐文件求值循环相同的输入面。
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

// aggRulePlan 单条聚合规则的并组求值预案：每轨行编译为独立求值的行包装模式
// （轨行是平行选择器，不做布尔组合），加上标识声明、分轨入口与按类型规约判定
// 的必含轨行集合。
type aggRulePlan struct {
	// rule 为源规则（引用传入规则序列的条目）。
	rule *RuleEntry
	// rows 为轨行模式（下标+1=行号；聚合规则只收普通条件行，行号即顶层序号）。
	rows []Pattern
	// rowIsSeq 为各轨行是否序列目标行（集合轨：整序列组=N 存储位，必含轨
	// 计数按相异群键计 1 单位）。
	rowIsSeq []bool
	// identity 为作品标识声明。
	identity []IdentityPart
	// tracks 为分轨入口（轨行→角色分配）。
	tracks []TrackAssign
	// requiredRow 为必含轨行号集合（所分配角色在类型规约中 Min≥1）。
	requiredRow map[int]bool
}

// buildAggregationPlans 过滤启用聚合规则并编译预案。跳过形态（求值侧的宁缺
// 勿错，与档案载入侧的停用+告警互补——载入已拦的形态在此双保险）：
//   - 类型悬空（WorkType 不在投影，含索引缺席）——识别求值同样跳过；
//   - 行结构非法（空条件表、括号子组、行级非、行编译失败）；
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
		if len(rule.Conditions) == 0 {
			continue
		}
		rows := make([]Pattern, 0, len(rule.Conditions))
		rowIsSeq := make([]bool, 0, len(rule.Conditions))
		valid := true
		for j := range rule.Conditions {
			row := &rule.Conditions[j]
			if row.Negated || row.Condition == nil {
				valid = false
				break
			}
			p, err := compileCondx(row.Condition, j+1)
			if err != nil {
				valid = false
				break
			}
			rows = append(rows, p)
			rowIsSeq = append(rowIsSeq, row.Condition.Target == CondxTargetSeq)
		}
		if !valid {
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
			identity:    rule.Identity,
			tracks:      rule.Aggregate.Tracks,
			requiredRow: required,
		})
	}
	return plans
}

// —— 单文件的轨行命中面与标识解析 ——

// aggFileRowHits 单文件在一条聚合规则下的轨行命中面：按行号下标存放该行独立
// 求值的作用域上下文（捕获绑定与序列群组产物；nil=该行未命中）。行间捕获不
// 互通——每行各自在独立作用域求值后原样保留，供标识与分轨按行号取用。
type aggFileRowHits []*ParseContext

// identityValue 解析该文件在本规则下的标识值：各部分按声明序以 NUL 分隔拼接
// （多部分复合标识防歧义）；任一部分取不到值即整体不可得（该文件不入组）。
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
// 取值面按捕获名在命中行间汇流，视频各扩展行/字幕各扩展行取到同一标识）；
// 序列群键专用名只对写入了群组产物的序列行取值（组内成员共享群键）。
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

// —— 并组主算法 ——

// aggAssignment 单文件的聚合归属结论：WorkKey 为聚合作品键（"f:"+主导文件
// 相对路径，与选择域的作品键同形——键前缀维持单一）；TrackRole 为本文件的
// 轨道角色；WorkType 为选中规则声明的作品类型（聚合定义的作品类型，消费面=
// 创建链子行 ResourceType）；WorkKey 空=无聚合（独立作品，其余字段同空）。
type aggAssignment struct {
	WorkKey   string
	TrackRole string
	WorkType  string
}

// aggregateGrouping 标识等值并组（纯函数、输入序确定）：
//
//   - 轨行按既有行求值机制逐文件求命中（捕获绑定、目录命中、序列群组分析全部
//     走同一 Pattern.Evaluate——目录捕获的目录间共享与序列群键的组内共享由此
//     自然成立：同目录文件解析出同名目录捕获值、同群组成员解析出同群键）；
//   - 标识值相等者并成一件作品（文件名同名/序列成组/目录成组三种形态同一语法）；
//   - 必含轨行每组恰一命中才成立：文件/目录行按命中文件数计，序列行按相异群键
//     数计（集合轨=整序列组 N 存储位计 1 单位）；≠1 即歧义（含零命中），该组
//     不成立、成员维持独立作品（宁缺勿错）；
//   - 主导文件=首个必含命中单位的首个成员（扫描序），作品键=其相对路径；
//   - 规则间按档案序先到先得：已被在先规则归属的文件不再改归属（与分组段首个
//     命中定群同款取向）；歧义组成员仍可被后续规则接纳。
//
// 非必含轨行零命中=该轨缺席、作品仍成立（单轨形态）；角色基数由类型规约定、
// 并组不按 Max 截断（聚合表达力不被消费面拖住）。
func aggregateGrouping(surface *aggEvalSurface, rules []RuleEntry, lib *LibraryIndex, types *resourceTypeIndex) []aggAssignment {
	n := len(surface.files)
	out := make([]aggAssignment, n)
	plans := buildAggregationPlans(rules, types)
	if len(plans) == 0 {
		return out
	}
	for _, plan := range plans {
		// 逐文件逐轨行求命中（每行独立作用域，捕获与群组产物留在本行上下文）。
		hits := make([]aggFileRowHits, n)
		for i := range surface.files {
			file := surface.files[i]
			in := surface.matchInputOf(file)
			rowHits := make(aggFileRowHits, len(plan.rows))
			for j, row := range plan.rows {
				ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
				if row.Evaluate(in, lib, ctx) {
					rowHits[j] = ctx
				}
			}
			hits[i] = rowHits
		}
		// 成员收集：标识可得、命中至少一条分轨行、且尚未被在先规则归属；
		// 组序=标识值首成员出现序，组内成员按扫描序（确定性）。
		type aggMemberGroup struct {
			members []int
		}
		groups := make(map[string]*aggMemberGroup)
		var order []string
		for i := range surface.files {
			if out[i].WorkKey != "" {
				continue
			}
			if hits[i].trackRoleOf(plan.tracks) == "" {
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
				out[i] = aggAssignment{WorkKey: workKey, TrackRole: hits[i].trackRoleOf(plan.tracks), WorkType: workType}
			}
		}
	}
	return out
}
