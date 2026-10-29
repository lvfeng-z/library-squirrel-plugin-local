// 本地导入解析内核的流水线：按父目录聚合扫描条目并构造匹配输入，先在分组段求值
// 兄弟群组模式定群，再在提取段依序求值其余规则产出元数据声明；纯函数，无 IO 与日志。
package main

import (
	"fmt"
	"strings"
)

// ScanFile 为 Create 扫描产出的单文件条目。
type ScanFile struct {
	// Path 为文件的完整路径，供后续阶段消费，本阶段不参与匹配。
	Path string
	// FileName 为文件名（含扩展名），不含目录部分。
	FileName string
	// DirNames 为从扫描根到直接父目录的各层目录名，索引即深度。
	DirNames []string
	// RelPath 为以 '/' 分隔的相对路径（相对扫描根）。
	RelPath string
}

// FileDecision 为单文件经过分组与提取两段后的流水线结论。
type FileDecision struct {
	// RelPath 为以 '/' 分隔的相对路径，回填自输入条目。
	RelPath string
	// FileName 为文件名（含扩展名），回填自输入条目。
	FileName string
	// Group 为所属群组的分析产物：群组成员非 nil（Key/Prefix/Seq/Size），
	// 落单条目为 nil——与「同目录多文件拆独立作品」的现状语义等价。
	Group *GroupAnalysis
	// Declarations 为提取阶段产出的元数据声明，按规则序与动作序追加。
	Declarations []MetadataDeclaration
	// MatchedRuleIDs 为全部命中规则的 ID（含 Extractions 为空的规则，
	// 归属接线由后续阶段消费），按规则传入顺序追加。组合规则（refs 非空）
	// 额外要求门段全体成员命中才计入（见 runExtractionStage 门段）。
	MatchedRuleIDs []string
	// ScanNotes 为贪婪识别动作的歧义记录（作者/作品集扫描出多个互异库内值，
	// 宁缺勿错不产出声明时记此），供上层观察呈现。
	ScanNotes []ScanNote
}

// ScanNote 为一条贪婪识别歧义记录。
type ScanNote struct {
	// Dimension 为发生歧义的维度（作者/作品集）。
	Dimension MetadataDimension
	// Values 为扫描出的互异库内值（按发现序）。
	Values []string
	// RuleID 为产出记录的规则 ID。
	RuleID string
}

// dirGroupKey 把目录名序列编码为同目录聚合键：目录名不可包含 NUL 字节，
// 以 NUL 连接不引入歧义；空序列（扫描根直下）聚合为空串。
func dirGroupKey(dirNames []string) string {
	return strings.Join(dirNames, "\x00")
}

// ancestorFilesLevels 从同批文件聚合派生各层祖先目录的直接子文件名列表
// （深度 d = 目录链 dirNames[0..d] 的直接子文件；该目录本批无文件时对应层为
// nil，目录子女谓词对此层保守不命中）。
func ancestorFilesLevels(dirNames []string, siblingsByDir map[string][]string) [][]string {
	levels := make([][]string, len(dirNames))
	for d := range dirNames {
		levels[d] = siblingsByDir[dirGroupKey(dirNames[:d+1])]
	}
	return levels
}

// ancestorChildDirCounts 从同批文件派生各层祖先目录的直接子目录数（仅计数
// 不含文件；空目录本批不可见，不计数）。
func ancestorChildDirCounts(dirNames []string, childDirsByParent map[string]map[string]struct{}) []int {
	counts := make([]int, len(dirNames))
	for d := range dirNames {
		counts[d] = len(childDirsByParent[dirGroupKey(dirNames[:d+1])])
	}
	return counts
}

// RunPipeline 对一批扫描条目执行解析内核流水线，返回与输入同序的单文件结论：
//  1. 按「父目录」（DirNames 整序列相同）聚合，构造各文件的 MatchInput，
//     Siblings 为同目录全部文件名（含自身，按输入顺序），并派生各层祖先的
//     直接子文件/子目录数（v4 目录终结谓词消费）；
//  2. 分组段：顶层 Pattern 为 SiblingPattern 的启用规则逐条求值，首个命中
//     定群（Group 填充，用户规则先于内置由调用方 EffectiveRules 保证），
//     无任何分组规则命中即独立文件（Group=nil）；
//  3. 提取段：其余规则按传入顺序对每文件求值 Pattern，命中即记入
//     MatchedRuleIDs 并依序执行该规则 Extractions 追加声明（声明 RuleID
//     归属产出规则）；
//  4. 每文件一份 ParseContext，基础字段 DirNames/Depth 按文件预先填入；
//     分组段写入的群组分析保留在同一 ctx 内，供 FromGroupPrefix 提取消费。
//
// 求值自下而上、逐文件进行（反向于「先命中目录或序列实体、再覆盖其下文件」的
// 常规心智）：不存在实体级命中；每个文件各自构造 MatchInput（文件名、祖先目录
// 链、同目录兄弟），各规则对该文件独立求值，结论回填该文件。目录目标的「整个
// 子树」与序列目标的「成套」，都是逐文件结论在呈现层的汇出形态。
func RunPipeline(files []ScanFile, rules []RuleEntry, lib *LibraryIndex) []FileDecision {
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
	decisions := make([]FileDecision, 0, len(files))
	for _, file := range files {
		ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
		in := &MatchInput{
			FileName:               file.FileName,
			DirNames:               file.DirNames,
			RelPath:                file.RelPath,
			Siblings:               siblingsByDir[dirGroupKey(file.DirNames)],
			AncestorFiles:          ancestorFilesLevels(file.DirNames, siblingsByDir),
			AncestorChildDirCounts: ancestorChildDirCounts(file.DirNames, childDirsByParent),
		}
		decision := FileDecision{RelPath: file.RelPath, FileName: file.FileName}
		decision.Group, _ = runGroupStage(rules, in, lib, ctx)
		runExtractionStage(&decision, rules, in, lib, ctx)
		decisions = append(decisions, decision)
	}
	return decisions
}

// isGroupingRule 该规则是否为兄弟群定群规则：v3 = 顶层 SiblingPattern；v4 =
// 无提取动作且单一序列条件行的规则（定群语义由序列主体承载：同键≥N 记
// GroupAnalysis，群组行包装求值与 SiblingPattern 同写入群组上下文；带提取
// 动作的单 seq 行规则=序列前缀提取条目〔FromGroupPrefix〕，非定群）。
func isGroupingRule(rule *RuleEntry) bool {
	if rule == nil {
		return false
	}
	if rule.Conditions != nil {
		return len(rule.Extractions) == 0 && len(rule.Conditions) == 1 &&
			rule.Conditions[0].Condition != nil &&
			rule.Conditions[0].Condition.Target == CondxTargetSeq
	}
	_, ok := rule.Pattern.(*SiblingPattern)
	return ok
}

// groupSeqTextOf 取定群规则对某成员文件名的序号原文（识别全景的群序号文本
// 消费）：v3 = SiblingPattern 提取式；v4 = 序列主体对去扩展名基名重演成员模板
// 取 {序号} 段。
func groupSeqTextOf(rule *RuleEntry, fileName string) (string, bool) {
	if rule == nil {
		return "", false
	}
	if sibling, ok := rule.Pattern.(*SiblingPattern); ok {
		return sibling.seqTextOf(fileName)
	}
	seq := condxSeqSubjectOf(rule)
	if seq == nil {
		return "", false
	}
	_, text, ok := seq.seqMemberKey(BaseNameWithoutExt(fileName))
	return text, ok
}

// condxSeqSubjectOf 从规则编译缓存中取出序列主体模式（行包装内层，可为
// And 首子式）；非序列规则返回 nil。
func condxSeqSubjectOf(rule *RuleEntry) *SeqSubjectPattern {
	if rule.Pattern == nil {
		return nil
	}
	inner := rule.Pattern
	if row, ok := inner.(*RowSubjectPattern); ok {
		inner = row.Inner
	}
	switch t := inner.(type) {
	case *SeqSubjectPattern:
		return t
	case And:
		if len(t) > 0 {
			if s, ok := t[0].(*SeqSubjectPattern); ok {
				return s
			}
		}
	case *And:
		if len(*t) > 0 {
			if s, ok := (*t)[0].(*SeqSubjectPattern); ok {
				return s
			}
		}
	}
	return nil
}

// runGroupStage 分组段：对启用规则逐条求值，首个命中的规则把群组分析写入
// ctx 并作为定群结果返回（同时返回定群规则 ID，供识别全景的序号形态重放与
// 规则命中统计消费）；全部不命中返回 nil（独立文件）。禁用规则与
// 非定群形态（v3 SiblingPattern / v4 序列条件行）的规则不参与本段。
// 求值上下文的群组字段另有一类写入方：规则未定群时由合并求值路径的前置
// 结构群对齐补位（见 recognition.go 的 mergedFileConclusionWithStructural）
// ——两类群共用同一字段，群组操作数与群组前缀提取对二者统一生效。
func runGroupStage(rules []RuleEntry, in *MatchInput, lib *LibraryIndex, ctx *ParseContext) (*GroupAnalysis, string) {
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled {
			continue
		}
		if !isGroupingRule(rule) {
			continue
		}
		if rule.Pattern.Evaluate(in, lib, ctx) {
			return ctx.Group, rule.ID
		}
	}
	return nil, ""
}

// runExtractionStage 提取段：对顶层 Pattern 非 SiblingPattern 的启用规则按传入
// 顺序逐条求值，命中即把规则 ID 追加进 decision.MatchedRuleIDs（含 Extractions
// 为空的规则），并依序执行该规则的 Extractions，产出声明追加进
// decision.Declarations；来源缺失的动作不产出声明，不中断其余动作。
//
// 捕获组与目录/谓词命中按规则隔离：每规则在私有作用域内求值与提取（群组
// 共享）——同名捕获组跨规则互不遮蔽，FromAncestorDir/FromPredicate 只读自身
// 规则的锚定/谓词命中（跨规则不再共享累积：共享单上下文时代「先绑入者胜」
// 令后入库规则的同名捕获被占位，后求值规则又会读到先求值规则更深的目录
// 命中而产出他目录剥壳的垃圾值——泛化后再调整的同系规则并存即触雷）；
// 「取最深/取最后」语义保留在规则作用域内。
//
// 带生效条件（When，通用条件表达式树）的动作两遍执行：第一遍只跑无条件动作
// （全部规则按序），第二遍按规则序补跑条件动作——条件对该规则命中时的解析
// 上下文（其私有作用域）与求值时点已产出的声明集求值（见 CondExpr.Holds），
// 两遍执行使条件可见全部无条件声明（裁决生成的条件规则居档案尾，其条件谓词
// 引用的声明在第一遍已全部在场）。贪婪识别（FromScan）动作第三遍执行：显式
// 行先产出、scan 只补本规则尚未产出的维度，多值/歧义语义见 applyScanActions。
//
// 组合规则（refs 非空）门段先行：主循环前先对会被求值的组合规则的被引用成员
// 全体求值（runRefGateStage），成员全体命中是组合规则的求值前提——任一未命中
// （含停用/被跳过/不在场）即整体零产出、不进 MatchedRuleIDs（严格语义，不降
// 级）；门段不写 MatchedRuleIDs，成员命中由其自身独立段单点登记（不重复计数），
// 追加序保持既有规则序面。门过后成员捕获快照以限定键挂入组合规则私有作用域
// 的注入区（ParseContext.RefCaptures，仅组合规则可见；refCapture 提取取值见
// Apply 的 FromRefCapture 分支，when 限定名解析见 conditionOperandValue）。
// 组合自身条件含目录行时另有「之下不含其身」约束（2026-10-29 用户拍板）：
// 自身目录行锚定的层与被引目录成员的命中层重合（锚定目录即被引规则命中的
// 目录）时整体不中——「位于被引规则命中的目录之下」不含其身；纯门组合与
// 自身行非目录形态（无目录锚定）不受影响。非组合规则与非组合路径零变化。
func runExtractionStage(decision *FileDecision, rules []RuleEntry, in *MatchInput, lib *LibraryIndex, ctx *ParseContext) {
	type ruleMatch struct {
		rule    *RuleEntry
		context *ParseContext
	}
	var conditional []ruleMatch
	var scans []ruleMatch
	gate := runRefGateStage(rules, in, lib, ctx)
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.Pattern == nil {
			continue
		}
		if isGroupingRule(rule) {
			continue
		}
		ruleCtx := ctx.ruleScopedContext()
		if len(rule.Refs) > 0 {
			// refs 门＝组合规则求值前提：任一成员门段未命中→整体零产出、
			// 不进 MatchedRuleIDs（严格语义，不降级）。
			if !gate.allHit(rule.Refs) {
				continue
			}
			// 门过：成员捕获快照以限定键挂入本规则可见的注入区（仅组合规则）。
			injectRefSnapshots(ruleCtx, gate, rule.Refs)
		}
		hit := rule.Pattern.Evaluate(in, lib, ruleCtx)
		ctx.absorbScoped(ruleCtx)
		if !hit {
			continue
		}
		if anchor, ok := ruleCtx.LastDirHit(); ok && gate.anchorCoincides(rule.Refs, anchor.Depth) {
			// 之下不含其身（2026-10-29 用户拍板）：组合自身目录行锚定的目录
			// 同时是被引目录成员命中的目录（同链同层即同目录）→整体不中、
			// 不进 MatchedRuleIDs。纯门组合与自身行非目录形态（文件/序列行，
			// 无目录锚定）不受影响。
			continue
		}
		decision.MatchedRuleIDs = append(decision.MatchedRuleIDs, rule.ID)
		if hasConditionalAction(rule.Extractions) {
			conditional = append(conditional, ruleMatch{rule: rule, context: ruleCtx})
		}
		if hasScanAction(rule.Extractions) {
			scans = append(scans, ruleMatch{rule: rule, context: ruleCtx})
		}
		for _, action := range rule.Extractions {
			if action.When != nil || action.From == FromScan {
				continue
			}
			if decl, ok := action.Apply(rule.ID, ruleCtx); ok {
				decision.Declarations = append(decision.Declarations, decl)
			}
		}
	}
	for _, match := range conditional {
		for _, action := range match.rule.Extractions {
			if action.When == nil {
				continue
			}
			if !action.When.Holds(match.context, decision.Declarations) {
				continue
			}
			if decl, ok := action.Apply(match.rule.ID, match.context); ok {
				decision.Declarations = append(decision.Declarations, decl)
			}
		}
	}
	// 第三遍：贪婪识别（scan）动作——显式行（无条件+条件）先产出，scan 只补
	// 本规则尚未产出的维度（可裁点④：显式优先）；多值/歧义语义见 applyScanActions。
	for _, match := range scans {
		filled := map[MetadataDimension]bool{}
		for i := range decision.Declarations {
			if decision.Declarations[i].RuleID == match.rule.ID {
				filled[decision.Declarations[i].Dimension] = true
			}
		}
		decls, notes := applyScanActions(match.rule, in, match.context, lib, filled)
		decision.Declarations = append(decision.Declarations, decls...)
		decision.ScanNotes = append(decision.ScanNotes, notes...)
	}
}

// hasConditionalAction 判定动作序列是否含带生效条件者（两遍执行的分拣依据）。
func hasConditionalAction(actions []ExtractionAction) bool {
	for _, action := range actions {
		if action.When != nil {
			return true
		}
	}
	return false
}

// refGateState 门段产物：被引用成员规则的先行求值结论——命中事实与限定键捕获
// 快照。命中只在此登记为组合规则的门判据，不写 decision.MatchedRuleIDs（命中
// 单点登记＝成员自身独立段，风险5：门段双求值不重复计数）；捕获快照只注入
// 组合规则的私有作用域（捕获私有不变式不动，限定名通道为加法式注入——发现6）。
type refGateState struct {
	// hits 为成员 ID → 门段命中结论；缺席（不在场/停用/被跳过/未求值）按未命中。
	hits map[string]bool
	// snapshots 为成员 ID → 限定键捕获快照（命中成员私有作用域捕获的副本，
	// 键形态「成员ID.rN.组名」，#canon 规范名变体一并快照）。
	snapshots map[string][]CaptureBinding
	// dirHitDepths 为成员 ID → 该成员目录行实际命中的层深集合（同链同层即
	// 同目录；Or 行短路只记实际发生命中的行）——「之下不含其身」判据的消费
	// 数据。无目录行的成员（文件/序列行形态）缺席或空集，不构成约束。
	dirHitDepths map[string]map[int]bool
}

// allHit 门判据：refs 全体成员门段命中（严格语义，不降级——决策2）：任一
// 未命中即组合规则整体零产出、不进 MatchedRuleIDs。
func (g *refGateState) allHit(refs []string) bool {
	for _, ref := range refs {
		if !g.hits[ref] {
			return false
		}
	}
	return true
}

// anchorCoincides 判定组合自身目录行的锚定层（anchorDepth）是否与某被引目录
// 成员的命中层重合——重合即锚定目录同时是该成员命中的目录（「之下不含其身」
// 的排除判据，2026-10-29 用户拍板）：组合以目录行为自身条件时，集名等锚定
// 不得落在被引规则命中的目录身上。无目录行的成员不参与判定。
func (g *refGateState) anchorCoincides(refs []string, anchorDepth int) bool {
	for _, ref := range refs {
		if g.dirHitDepths[ref][anchorDepth] {
			return true
		}
	}
	return false
}

// snapshot 取某成员的限定键捕获快照（成员未命中时为空）。
func (g *refGateState) snapshot(memberID string) []CaptureBinding {
	return g.snapshots[memberID]
}

// runRefGateStage 门段（求值两段化的第一段，先于提取段主循环——与「定群段
// 先于提取段」同款机制）：对本批规则中会被求值的组合规则（启用、模式在场、
// 非定群形态、refs 非空）的被引用成员全体先求值。
//   - 成员仍在各自私有作用域求值（ruleScopedContext）；求值结论不并回共享
//     上下文（不调 absorbScoped——门段不得扰动主循环的求值次序面，非组合
//     路径零变化；群组基线取门段时点即定群/结构群补位后的状态）；
//   - 成员停用、被跳过（定群形态或模式缺失）或不在本规则集内＝未命中
//     （决策2 严格语义：含成员删除后悬空引用的求值期形态）；
//   - 门段不写 MatchedRuleIDs——成员命中由其自身独立段单点登记，追加序保持
//     既有规则序面（风险2）。
//
// 本批无会被求值的组合规则时零工作（空门，主循环行为与既有完全一致）。
// 成员求值彼此独立纯函数，needed 集合的遍历序不影响结论。
func runRefGateStage(rules []RuleEntry, in *MatchInput, lib *LibraryIndex, ctx *ParseContext) *refGateState {
	gate := &refGateState{hits: map[string]bool{}, snapshots: map[string][]CaptureBinding{}, dirHitDepths: map[string]map[int]bool{}}
	needed := map[string]bool{}
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.Pattern == nil || len(rule.Refs) == 0 || isGroupingRule(rule) {
			continue
		}
		for _, ref := range rule.Refs {
			needed[ref] = true
		}
	}
	if len(needed) == 0 {
		return gate
	}
	byID := make(map[string]*RuleEntry, len(rules))
	for i := range rules {
		if _, seen := byID[rules[i].ID]; !seen {
			byID[rules[i].ID] = &rules[i]
		}
	}
	for memberID := range needed {
		member, ok := byID[memberID]
		if !ok || !member.Enabled || member.Pattern == nil || isGroupingRule(member) {
			continue // 不在场/停用/被跳过＝未命中（hits 缺席即 false）
		}
		memberCtx := ctx.ruleScopedContext()
		hit := member.Pattern.Evaluate(in, lib, memberCtx)
		gate.hits[memberID] = hit
		if hit {
			gate.snapshots[memberID] = refCaptureSnapshot(memberID, memberCtx)
			// 目录行命中层集合留档（「之下不含其身」判据；空集=成员无目录行）。
			for _, dh := range memberCtx.DirHits {
				if gate.dirHitDepths[memberID] == nil {
					gate.dirHitDepths[memberID] = map[int]bool{}
				}
				gate.dirHitDepths[memberID][dh.Depth] = true
			}
		}
	}
	return gate
}

// injectRefSnapshots 按 refs 序把门段捕获快照以限定键挂入组合规则的私有作用域
// （仅本规则可见的注入区；非组合规则零注入）。
func injectRefSnapshots(ruleCtx *ParseContext, gate *refGateState, refs []string) {
	for _, ref := range refs {
		ruleCtx.RefCaptures = append(ruleCtx.RefCaptures, gate.snapshot(ref)...)
	}
}

// Holds 对求值时点已产出的声明集求值：Present 内谓词须全部在场、Absent 内
// 谓词须全部缺席（谓词的 And 组合），两列表皆空为恒真。
//
// 本方法为 v2 谓词组合形态的直读求值，保留作旧语义的等价性参照：载入侧
// 映射产出的表达式树（legacyConditionExpr）与本方法在相同声明集上的求值
// 结果恒等，由单测锚定。运行期求值统一走 CondExpr.Holds。
func (c *SynthCondition) Holds(decls []MetadataDeclaration) bool {
	if c == nil {
		return true
	}
	for _, pred := range c.Present {
		if !pred.Holds(decls) {
			return false
		}
	}
	for _, pred := range c.Absent {
		if pred.Holds(decls) {
			return false
		}
	}
	return true
}

// Holds 判定该声明谓词（某来源在某含义维度上的声明）是否在声明集内在场。
func (p DeclPredicate) Holds(decls []MetadataDeclaration) bool {
	for _, decl := range decls {
		if decl.Source == p.Source && decl.Dimension == p.Dimension {
			return true
		}
	}
	return false
}

// Holds 对解析上下文与求值时点已产出的声明集求值条件表达式树：nil 恒真。
// 操作数取值不可用（捕获组未绑定、限定键形操作数未注入或未绑定、非群组成员
// ——既不在规则定群也不在结构群中——的群组字段）时含该操作数的相等/不等/
// 区间比较一律为假——条件无法确立则动作不产出（保守取向）；未知判别类别在
// 构造期已被拦截，此处防御性为假。
func (e *CondExpr) Holds(ctx *ParseContext, decls []MetadataDeclaration) bool {
	if e == nil {
		return true
	}
	switch e.Kind {
	case ExprPredicate:
		return DeclPredicate{Source: e.Source, Dimension: e.Dimension}.Holds(decls)
	case ExprEqual, ExprNotEqual:
		left := conditionOperandValue(e.Left, ctx)
		right := conditionOperandValue(e.Right, ctx)
		if !left.ok || !right.ok {
			return false
		}
		var equal bool
		if isTextOperand(e.Left) {
			equal = left.text == right.text
		} else {
			equal = left.num == right.num
		}
		if e.Kind == ExprNotEqual {
			return !equal
		}
		return equal
	case ExprRange:
		operand := conditionOperandValue(e.Operand, ctx)
		if !operand.ok {
			return false
		}
		if e.Min != nil && operand.num < *e.Min {
			return false
		}
		if e.Max != nil && operand.num > *e.Max {
			return false
		}
		return true
	case ExprAnd:
		for _, sub := range e.Subs {
			if !sub.Holds(ctx, decls) {
				return false
			}
		}
		return true
	case ExprOr:
		for _, sub := range e.Subs {
			if sub.Holds(ctx, decls) {
				return true
			}
		}
		return false
	case ExprNot:
		if e.Inner == nil {
			return false
		}
		return !e.Inner.Holds(ctx, decls)
	default:
		return false
	}
}

// condOperand 为比较类操作数的求值产出：文本与数值双轨（按操作数判别类别
// 其一有效），ok 为 false 表示取值不可用（捕获组未绑定 / 非群组成员的群组
// 字段）。
type condOperand struct {
	text string
	num  float64
	ok   bool
}

// conditionOperandValue 求值比较类操作数（capture / depth / groupSize /
// groupSeq / 字面量）；capture 操作数为限定键形（组合规则 when 引用成员捕获，
// 显式形「成员ID.rN.组名」）时从注入区取值（refCaptureValue），非限定名走
// 自有捕获。其余判别类别（谓词、比较、逻辑）不是操作数，返回不可用。
func conditionOperandValue(e *CondExpr, ctx *ParseContext) condOperand {
	if e == nil {
		return condOperand{}
	}
	switch e.Kind {
	case ExprCapture:
		if ctx == nil {
			return condOperand{}
		}
		// 限定键形操作数（组合规则 when 引用成员捕获，显式形「成员ID.rN.组名」，
		// 可带 #canon 后缀）：从注入区按书写名直接取值（RefCaptures——规范名
		// 变体是普通限定键、按书写名命中，与自有捕获操作数的直取口径一致，
		// 不做自动 #canon 回退）。限定键未注入/未绑定＝取值不可用（比较为假，
		// 保守取向）；非组合规则的注入区恒空且构造期已拒绝其限定名操作数
		//（validateExpr），两侧口径一致（共用 parseRefCaptureName 解析）。
		// 非限定名照旧走自有捕获，行为零变化。
		if memberID, binding, ok := parseRefCaptureName(e.CaptureName); ok {
			value, found := ctx.refCaptureValue(memberID, binding)
			return condOperand{text: value, ok: found}
		}
		value, ok := ctx.Capture(e.CaptureName)
		return condOperand{text: value, ok: ok}
	case ExprDepth:
		if ctx == nil {
			return condOperand{}
		}
		return condOperand{num: float64(ctx.Depth), ok: true}
	case ExprGroupSize:
		if ctx == nil || ctx.Group == nil {
			return condOperand{}
		}
		return condOperand{num: float64(ctx.Group.Size), ok: true}
	case ExprGroupSeq:
		if ctx == nil || ctx.Group == nil {
			return condOperand{}
		}
		return condOperand{num: float64(ctx.Group.Seq), ok: true}
	case ExprString:
		return condOperand{text: e.Str, ok: true}
	case ExprNumber:
		return condOperand{num: e.Num, ok: true}
	default:
		return condOperand{}
	}
}

// isTextOperand 判定比较操作数是否走文本轨（构造期已保证相等比较两侧同型）。
func isTextOperand(e *CondExpr) bool {
	return e != nil && (e.Kind == ExprCapture || e.Kind == ExprString)
}

// PendingItem 为合成段的待裁决项：策略无法自动裁决的声明组合。不静默丢弃，
// 由调用方记日志或报告，人工裁决后可调整规则或策略重跑。
type PendingItem struct {
	// Dimension 为无法裁决的含义维度。
	Dimension MetadataDimension
	// Candidates 为冲突层的全部候选声明，按输入顺序排列。
	Candidates []MetadataDeclaration
	// Reason 为冲突原因的人类可读描述。
	Reason string
}

// sourceTrust 为来源载体的可信度分层（数字越大越可信）。dir>filename>constant
// 为既定分层；predicate 高于 filename：谓词取值须同时通过文件名内容与库内策展
// 集合双门槛，比纯文件名解析多一道确证；group 低于 filename：群组公共前缀是
// 兄弟文件名的结构推断，未经任何内容解析；assignment 为会话赋值（用户手工
// 指定的特异性结果），高于全部规则推导来源——赋值经合成后按维度覆盖，不参与
// 择一，此处登记信任级仅防赋值声明被再次喂入合成时被误降级；adjudication 为
// 裁决声明（用户对冲突的裁决结果），压过全部规则推导来源（含 dir），使裁决
// 生成规则的常量声明在 highTrust 维度上稳定胜出、原冲突双方声明落选。
var sourceTrust = map[SourceKind]int{
	SourceDir:          5,
	SourcePredicate:    4,
	SourceFilename:     3,
	SourceGroup:        2,
	SourceConstant:     1,
	SourceAssignment:   6,
	SourceAdjudication: 6,
}

// defaultDimStrategies 为含义维度未配置策略时的缺省策略：标题多候选并存会
// 产出重复条目，取高可信择一；作者 / 标签 / 作品集多值为常态，取并集不丢值。
var defaultDimStrategies = []DimPolicy{
	{Dimension: DimTitle, Strategy: StrategyHighTrust},
	{Dimension: DimAuthor, Strategy: StrategyUnion},
	{Dimension: DimTag, Strategy: StrategyUnion},
	{Dimension: DimCollection, Strategy: StrategyUnion},
}

// AggregateSynthesis 跨规则聚合合成策略：按规则传入顺序（用户规则先于内置由
// 调用方排序保证）每个含义维度取首个声明的 DimPolicy；RoleBindings 按规则序
// 依序拼接；未配置的含义维度回退缺省策略（title=highTrust，author/tag/
// collection=union）。
func AggregateSynthesis(rules []RuleEntry) SynthesisPolicy {
	var policy SynthesisPolicy
	seen := make(map[MetadataDimension]bool)
	for _, rule := range rules {
		for _, dim := range rule.Synthesis.DimPolicies {
			if seen[dim.Dimension] {
				continue
			}
			seen[dim.Dimension] = true
			policy.DimPolicies = append(policy.DimPolicies, dim)
		}
		policy.Roles = append(policy.Roles, rule.Synthesis.Roles...)
	}
	for _, def := range defaultDimStrategies {
		if !seen[def.Dimension] {
			policy.DimPolicies = append(policy.DimPolicies, def)
		}
	}
	return policy
}

// Synthesize 对提取段产出的声明做声明级合成，返回终选声明与待裁决项：
//   - union：同维度声明全保留，按值去重保首个；
//   - highTrust：按来源可信度择一，唯一最高可信级取首个声明，同最高可信级
//     取值不同记待裁决项（该维度不产出），维度无声明不产出也不记待裁决；
//   - 角色硬绑定：final 中 Source 与某 RoleBinding 相同且 RoleName 为空的
//     声明绑定该角色的 RoleName，声明已带 RoleName 不覆盖。
//
// 角色绑定与维度策略均为声明级合成，不产生新维度。
func Synthesize(decls []MetadataDeclaration, policy SynthesisPolicy) (final []MetadataDeclaration, pending []PendingItem) {
	configured := make(map[MetadataDimension]SynthStrategy, len(policy.DimPolicies))
	for _, dim := range policy.DimPolicies {
		configured[dim.Dimension] = dim.Strategy
	}
	var dimOrder []MetadataDimension
	byDim := make(map[MetadataDimension][]MetadataDeclaration)
	for _, decl := range decls {
		if _, ok := byDim[decl.Dimension]; !ok {
			dimOrder = append(dimOrder, decl.Dimension)
		}
		byDim[decl.Dimension] = append(byDim[decl.Dimension], decl)
	}
	final = make([]MetadataDeclaration, 0, len(decls))
	for _, dim := range dimOrder {
		group := byDim[dim]
		if strategyOf(dim, configured) == StrategyHighTrust {
			final, pending = pickHighTrust(final, pending, dim, group)
			continue
		}
		// union 分支（含缺省回退与未知维度的保底）：按值去重，保首个声明。
		seenValue := make(map[string]bool, len(group))
		for _, decl := range group {
			if seenValue[decl.Value] {
				continue
			}
			seenValue[decl.Value] = true
			final = append(final, decl)
		}
	}
	for i := range final {
		for _, role := range policy.Roles {
			if final[i].Source == role.Source && final[i].RoleName == "" {
				final[i].RoleName = role.RoleName
			}
		}
	}
	return final, pending
}

// strategyOf 查含义维度的生效合成策略：先查已配置策略，再回退缺省策略表；
// 两者均未覆盖（缺省表之外的维度）回退 union，并集不丢值。
func strategyOf(dim MetadataDimension, configured map[MetadataDimension]SynthStrategy) SynthStrategy {
	if strategy, ok := configured[dim]; ok {
		return strategy
	}
	for _, def := range defaultDimStrategies {
		if def.Dimension == dim {
			return def.Strategy
		}
	}
	return StrategyUnion
}

// pickHighTrust 执行 highTrust 维度的择一：取最高可信级的声明层，层内取值
// 唯一则产出首个声明，取值不唯一则记待裁决项，Reason 点名维度、冲突来源层
// 与取值数，交由调用方裁决。
func pickHighTrust(final []MetadataDeclaration, pending []PendingItem, dim MetadataDimension, group []MetadataDeclaration) ([]MetadataDeclaration, []PendingItem) {
	maxTrust := 0
	for _, decl := range group {
		if trust := sourceTrust[decl.Source]; trust > maxTrust {
			maxTrust = trust
		}
	}
	var top []MetadataDeclaration
	for _, decl := range group {
		if sourceTrust[decl.Source] == maxTrust {
			top = append(top, decl)
		}
	}
	distinct := make(map[string]bool, len(top))
	for _, decl := range top {
		distinct[decl.Value] = true
	}
	if len(distinct) == 1 {
		return append(final, top[0]), pending
	}
	reason := fmt.Sprintf("维度 %s 同层冲突：最高可信来源 %s 存在 %d 个不同取值，无法自动择一", dim, top[0].Source, len(distinct))
	return final, append(pending, PendingItem{Dimension: dim, Candidates: top, Reason: reason})
}
