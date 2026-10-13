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
	// 归属接线由后续阶段消费），按规则传入顺序追加。
	MatchedRuleIDs []string
}

// dirGroupKey 把目录名序列编码为同目录聚合键：目录名不可包含 NUL 字节，
// 以 NUL 连接不引入歧义；空序列（扫描根直下）聚合为空串。
func dirGroupKey(dirNames []string) string {
	return strings.Join(dirNames, "\x00")
}

// RunPipeline 对一批扫描条目执行解析内核流水线，返回与输入同序的单文件结论：
//  1. 按「父目录」（DirNames 整序列相同）聚合，构造各文件的 MatchInput，
//     Siblings 为同目录全部文件名（含自身，按输入顺序）；
//  2. 分组段：顶层 Pattern 为 SiblingPattern 的启用规则逐条求值，首个命中
//     定群（Group 填充，用户规则先于内置由调用方 EffectiveRules 保证），
//     无任何分组规则命中即独立文件（Group=nil）；
//  3. 提取段：其余规则按传入顺序对每文件求值 Pattern，命中即记入
//     MatchedRuleIDs 并依序执行该规则 Extractions 追加声明（声明 RuleID
//     归属产出规则）；
//  4. 每文件一份 ParseContext，基础字段 DirNames/Depth 按文件预先填入；
//     分组段写入的群组分析保留在同一 ctx 内，供 FromGroupPrefix 提取消费。
func RunPipeline(files []ScanFile, rules []RuleEntry, lib *LibraryIndex) []FileDecision {
	siblingsByDir := make(map[string][]string, len(files))
	for _, file := range files {
		key := dirGroupKey(file.DirNames)
		siblingsByDir[key] = append(siblingsByDir[key], file.FileName)
	}
	decisions := make([]FileDecision, 0, len(files))
	for _, file := range files {
		ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
		in := &MatchInput{
			FileName: file.FileName,
			DirNames: file.DirNames,
			RelPath:  file.RelPath,
			Siblings: siblingsByDir[dirGroupKey(file.DirNames)],
		}
		decision := FileDecision{RelPath: file.RelPath, FileName: file.FileName}
		decision.Group = runGroupStage(rules, in, lib, ctx)
		runExtractionStage(&decision, rules, in, lib, ctx)
		decisions = append(decisions, decision)
	}
	return decisions
}

// runGroupStage 分组段：对顶层 Pattern 为 SiblingPattern 的启用规则逐条求值，
// 首个命中的规则把群组分析写入 ctx 并作为定群结果返回；全部不命中返回 nil
// （独立文件）。禁用规则与非兄弟群组模式的规则不参与本段。
func runGroupStage(rules []RuleEntry, in *MatchInput, lib *LibraryIndex, ctx *ParseContext) *GroupAnalysis {
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if _, ok := rule.Pattern.(*SiblingPattern); !ok {
			continue
		}
		if rule.Pattern.Evaluate(in, lib, ctx) {
			return ctx.Group
		}
	}
	return nil
}

// runExtractionStage 提取段：对顶层 Pattern 非 SiblingPattern 的启用规则按传入
// 顺序逐条求值，命中即把规则 ID 追加进 decision.MatchedRuleIDs（含 Extractions
// 为空的规则），并依序执行该规则的 Extractions，产出声明追加进
// decision.Declarations；来源缺失的动作不产出声明，不中断其余动作。
func runExtractionStage(decision *FileDecision, rules []RuleEntry, in *MatchInput, lib *LibraryIndex, ctx *ParseContext) {
	for _, rule := range rules {
		if !rule.Enabled || rule.Pattern == nil {
			continue
		}
		if _, ok := rule.Pattern.(*SiblingPattern); ok {
			continue
		}
		if !rule.Pattern.Evaluate(in, lib, ctx) {
			continue
		}
		decision.MatchedRuleIDs = append(decision.MatchedRuleIDs, rule.ID)
		for _, action := range rule.Extractions {
			if decl, ok := action.Apply(rule.ID, ctx); ok {
				decision.Declarations = append(decision.Declarations, decl)
			}
		}
	}
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
// 兄弟文件名的结构推断，未经任何内容解析。
var sourceTrust = map[SourceKind]int{
	SourceDir:       5,
	SourcePredicate: 4,
	SourceFilename:  3,
	SourceGroup:     2,
	SourceConstant:  1,
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
