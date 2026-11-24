// 本地导入解析内核的流水线：按父目录聚合扫描条目并构造匹配输入，先在分组段求值
// 兄弟群组模式定群，再在提取段依序求值其余规则产出元数据声明，最后在汇流后的
// 聚合段对聚合规则做标识等值并组；纯函数，无 IO 与日志。
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
	// CompanionText 为伴随说明文件的内容片段（识别轮按规则命中集采集注入，
	// 空串=未采集或不可作为文本呈现——二进制嗅探拒读/读盘失败/不在命中集）；
	// CompanionTruncated 标记片段超采集上限被截断。求值只消费注入的数据
	// 字段，不发起读盘。
	CompanionText      string
	CompanionTruncated bool
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
	// AggWorkKey 为聚合段落定的聚合作品键（"f:"+主导文件相对路径；空=无聚合，
	// 独立作品）；AggTrackRole 为本文件的轨道角色（空=无聚合）；AggWorkType 为
	// 选中规则声明的作品类型（聚合定义的作品类型，创建链子行 ResourceType 的
	// 声明值来源）。三字段由汇流后的聚合段写入（见 runAggregationStage）——
	// 逐文件求值循环不产出。AggCompanion 为说明成员标记（聚合段产的绑定行
	// 成员或扇出归属段产的元数据源文件——归进作品/挂靠作品、无轨、不导入、
	// 不独立成件）；AggFanoutWorkKeys 为内容扇出键集（本文件的内容声明按
	// 分组扇出到达的全部作品键——内容声明只按此键集流动；空=无扇出）。
	AggWorkKey   string
	AggTrackRole string
	AggWorkType  string
	AggCompanion bool
	// AggFanoutWorkKeys 内容扇出键集：扇出归属段落定（与聚合归属同相挂汇流
	// 后），组内目标文件推出的全部作品键（已被聚合收编=其聚合作品键、未
	// 收编=自身路径键）；组内无落点（孤文件回退）与未参与扇出的文件零值。
	AggFanoutWorkKeys []string
	// SetGroupKey / SetName / SetRuleID / SetEntryIndex 为集归属（产出四
	//「作品集聚合」汇流后求值结论）：组键+集名+产出规则 ID+产出条目序
	//（规则内声明序，0 起；同文件多条目按此序先到先得），三字段同空=不入集。
	// 消费面按「SetGroupKey+SetName」识别集、按 SetRuleID+SetEntryIndex
	// 追溯产出条目（字段语义与 FileConclusion 同名四字段一致）。
	SetGroupKey   string
	SetName       string
	SetRuleID     string
	SetEntryIndex int
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

// RunPipeline 对一批扫描条目执行解析内核流水线（无类型规约形态：聚合段按
// 类型索引缺席处理，聚合规则跳过）。求值阶段构成见 RunPipelineWithTypes。
func RunPipeline(files []ScanFile, rules []RuleEntry, lib *LibraryIndex) []FileDecision {
	return RunPipelineWithTypes(files, rules, lib, nil)
}

// RunPipelineWithTypes 对一批扫描条目执行解析内核流水线，返回与输入同序的单文件结论：
//  1. 按「父目录」（DirNames 整序列相同）聚合，构造各文件的 MatchInput，
//     Siblings 为同目录全部文件名（含自身，按输入顺序），并派生各层祖先的
//     直接子文件/子目录数（v4 目录终结谓词消费）；
//  2. 分组段：顶层 Pattern 为 SiblingPattern 的启用规则逐条求值，首个命中
//     定群（Group 填充，用户规则先于内置由调用方 EffectiveRules 保证），
//     无任何分组规则命中即独立文件（Group=nil）；
//  3. 提取段：其余规则（含聚合规则——决策10 解除互斥：携带提取动作时声明
//     挂各命中文件）按传入顺序对每文件求值 Pattern，命中即记入
//     MatchedRuleIDs 并依序执行该规则 Extractions 追加声明（声明 RuleID
//     归属产出规则）；
//  4. 每文件一份 ParseContext，基础字段 DirNames/Depth 按文件预先填入；
//     分组段写入的群组分析保留在同一 ctx 内，供 FromGroupPrefix 提取消费；
//  5. 聚合段：汇流后的独立通道（与分组段同相位、居其后——群归属与元数据
//     结论已定，聚合只消费既有结论不改写），对聚合规则做标识等值并组并把
//     聚合归属落回各文件结论（见 runAggregationStage）。
//
// 求值自下而上、逐文件进行（反向于「先命中目录或序列实体、再覆盖其下所有文件」
// 的常规心智）：不存在实体级命中；每个文件各自构造 MatchInput（文件名、祖先目录
// 链、同目录兄弟），各规则对该文件独立求值，结论回填该文件。目录目标的「整个
// 子树」与序列目标的「成套」，都是逐文件结论在呈现层的汇出形态。
func RunPipelineWithTypes(files []ScanFile, rules []RuleEntry, lib *LibraryIndex, types *resourceTypeIndex) []FileDecision {
	surface := newAggEvalSurface(files, files)
	siblingsByDir := surface.siblingsByDir
	childDirsByParent := surface.childDirsByParent
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
			CompanionText:          file.CompanionText,
			CompanionTruncated:     file.CompanionTruncated,
		}
		decision := FileDecision{RelPath: file.RelPath, FileName: file.FileName}
		decision.Group, _ = runGroupStage(rules, in, lib, ctx)
		runExtractionStage(&decision, rules, in, lib, ctx)
		decisions = append(decisions, decision)
	}
	runAggregationStage(decisions, surface, rules, lib, types)
	return decisions
}

// runAggregationStage 聚合段：汇流后的独立通道（与分组段同相位、居其后）——
// 群归属与元数据结论在分组段/提取段已定，本段只消费既有结论不改写：对求值面
// 执行聚合规则的标识等值并组（算法见 aggregateGrouping），把聚合归属（作品键
// +轨道角色）落回与文件对位序的各条结论。其后接作品集聚合段（产出四）：聚合
// 归属先到先得，本段只对剩余命中文件按组键归集并落集归属（见 workSetAggregation）。
// decisions 与 surface.files 按下标对齐（调用方保证同一清单）。
func runAggregationStage(decisions []FileDecision, surface *aggEvalSurface, rules []RuleEntry, lib *LibraryIndex, types *resourceTypeIndex) {
	assigns := aggregateGrouping(surface, rules, lib, types, nil)
	for i := range decisions {
		if i >= len(assigns) {
			break
		}
		decisions[i].AggWorkKey = assigns[i].WorkKey
		decisions[i].AggTrackRole = assigns[i].TrackRole
		decisions[i].AggWorkType = assigns[i].WorkType
		decisions[i].AggCompanion = assigns[i].Companion
		decisions[i].AggFanoutWorkKeys = assigns[i].FanoutWorkKeys
	}
	// 作品集聚合段（产出四）：居聚合段之后——既有聚合归属优先（已被聚合收编
	// 或说明成员标记的文件不入集），本段只对剩余命中文件按各条目的归集依据求
	// 组键、取该条目的集名来源（NameExtraction 捕获值）并落集归属（组键+集名+
	// 规则ID+条目序）。
	setAssigns := workSetAggregation(surface, rules, lib,
		libraryGroupKeysOfDecisions(decisions),
		workSetTakenOfDecisions(decisions), nil)
	for i := range decisions {
		if i >= len(setAssigns) {
			break
		}
		decisions[i].SetGroupKey = setAssigns[i].GroupKey
		decisions[i].SetName = setAssigns[i].SetName
		decisions[i].SetRuleID = setAssigns[i].RuleID
		decisions[i].SetEntryIndex = setAssigns[i].EntryIndex
	}
}

// metadataRules 过滤出元数据类别的规则序列（保持原序）：聚合规则不表达目录层
// 含义（其条件行的成员判定属聚合段），目录含义回放等「逐规则求值产出含义」
// 的消费面先经本过滤再进入。
func metadataRules(rules []RuleEntry) []RuleEntry {
	out := make([]RuleEntry, 0, len(rules))
	for _, rule := range rules {
		if rule.Category == RuleCategoryAggregate {
			continue
		}
		out = append(out, rule)
	}
	return out
}

// isGroupingRule 该规则是否为兄弟群定群规则：v3 = 顶层 SiblingPattern；v4 =
// 无提取动作且全部条件行均为序列目标行（非行级非；单行与多行〔如同一序列
// 家族的两种分隔形态 or 并列〕同权——定群语义由序列主体承载：同键≥N 记
// GroupAnalysis，群组行包装求值与 SiblingPattern 同写入群组上下文；带提取
// 动作的 seq 行规则=序列前缀提取条目〔FromGroupPrefix〕，非定群；混入非
// seq 行或行级非的规则不定群——多形态合取属规则编辑的高级形态，不在定群
// 语义内）。多行形态曾因「恰一行」判据被漏出定群段（2026-10-29 开发环境
// 实测：两行 or 序列规则命中可见而群卡/序列身份缺席，退化为结构群补位）。
func isGroupingRule(rule *RuleEntry) bool {
	if rule == nil {
		return false
	}
	// 聚合规则不参与定群：定群语义由序列主体承载（成员判定与落轨在汇流后
	// 的聚合段；聚合规则的序列条件行在提取段照常逐文件求值——决策10）。
	if rule.Category == RuleCategoryAggregate {
		return false
	}
	if rule.Conditions != nil {
		if len(rule.Extractions) != 0 || len(rule.Conditions) == 0 {
			return false
		}
		for i := range rule.Conditions {
			row := &rule.Conditions[i]
			if row.Negated || row.Condition == nil || row.Condition.Target != CondxTargetSeq {
				return false
			}
		}
		return true
	}
	_, ok := rule.Pattern.(*SiblingPattern)
	return ok
}

// groupSeqTextOf 取定群规则对某成员文件名的序号原文（识别全景的群序号文本
// 消费）：v3 = SiblingPattern 提取式；v4 = 序列主体对去扩展名基名重演成员模板
// 取 {序号} 段（多行 or 并列时逐主体试配，首个能匹配成员名的主体出序号）。
func groupSeqTextOf(rule *RuleEntry, fileName string) (string, bool) {
	if rule == nil {
		return "", false
	}
	if sibling, ok := rule.Pattern.(*SiblingPattern); ok {
		return sibling.seqTextOf(fileName)
	}
	base := BaseNameWithoutExt(fileName)
	for _, seq := range condxSeqSubjectsOf(rule) {
		if _, text, ok := seq.seqMemberKey(base); ok {
			return text, true
		}
	}
	return "", false
}

// condxSeqSubjectsOf 从规则编译缓存中收集全部序列主体模式（行包装内层；
// Or 分支逐一下潜——多行 or 并列时各形态主体都入列）；非序列规则返回空。
func condxSeqSubjectsOf(rule *RuleEntry) []*SeqSubjectPattern {
	if rule.Pattern == nil {
		return nil
	}
	var out []*SeqSubjectPattern
	var walk func(p Pattern)
	walk = func(p Pattern) {
		switch t := p.(type) {
		case *RowSubjectPattern:
			walk(t.Inner)
		case *SeqSubjectPattern:
			out = append(out, t)
		case And:
			for _, sub := range t {
				walk(sub)
			}
		case *And:
			for _, sub := range *t {
				walk(sub)
			}
		case Or:
			for _, sub := range t {
				walk(sub)
			}
		}
	}
	walk(rule.Pattern)
	return out
}

// runGroupStage 分组段：对启用规则逐条求值，首个命中的规则把群组分析写入
// ctx 并作为定群结果返回（同时返回定群规则 ID，供识别全景的序号形态重放与
// 规则命中统计消费）；全部不命中返回 nil（独立文件）。禁用规则与
// 非定群形态（v3 SiblingPattern / v4 全 seq 条件行规则）的规则不参与本段。
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
// 供给方门段先行（作用域附件）：主循环前对会被求值的规则条件行内
// refScope 附件引用的全部供给方先求值（runRefGateStage——needed 收集为
// 「全部启用规则条件行递归扫描 refScope 附件」）；
// 供给方命中层集（dirHitDepths）透传进共享上下文（ParseContext.RefScopeDepths，
// ruleScopedContext 继承——风险6 核对点），消费规则目录行逐层槽位的 refScope
// 谓词按层距阶梯判据核对（裁决⑥：候选层 c 合格 ⟺ 供给方命中且 ∃D∈供给方
// 命中层集 s.t. 层距=0 时 c>D、层距=N 时 c−D=N——「之下不含其身」与
// above-form 在该判据下天然不可成立）。供给方未命中
// （停用/悬空/被跳过——链式供给合法，环为唯一禁形、保存与载入两期同拒，
// 成环条目不在求值场）→ 附件谓词恒不满足→行不中（严格语义，不降级）。
// 供给方捕获快照以限定键挂入消费规则私有作用域的注入区（ParseContext.
// RefCaptures，按本规则的供给方集注入——injectRefSnapshots；refCapture 提取
// 取值见 Apply 的 FromRefCapture 分支，when 限定名解析见 conditionOperandValue）。
func runExtractionStage(decision *FileDecision, rules []RuleEntry, in *MatchInput, lib *LibraryIndex, ctx *ParseContext) {
	type ruleMatch struct {
		rule    *RuleEntry
		context *ParseContext
	}
	var conditional []ruleMatch
	var scans []ruleMatch
	gate := runRefGateStage(rules, in, lib, ctx)
	// 供给方命中层集挂共享上下文（门段入口已挂同一映射，此处同映射幂等）：
	// 文件级事实（先行求值一次），规则私有作用域经 ruleScopedContext 继承
	// （风险6 核对点）。
	ctx.RefScopeDepths = gate.dirHitDepths
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.Pattern == nil {
			continue
		}
		if isGroupingRule(rule) {
			continue
		}
		// 聚合规则照常逐文件求值（决策10 解除互斥）：携带提取动作时声明挂
		// 各命中文件、经主导文件结论并入聚合作品元数据；并组判定仍在汇流后
		// 的聚合段（runAggregationStage——两段各司其职，扇出/并入不进本循环）。
		ruleCtx := ctx.ruleScopedContext()
		// 作用域附件消费规则：供给方捕获快照按本规则供给方集注入其私有作用域
		//（限定键通道，refCapture 提取与 when 限定名可读；供给方未命中时快照
		// 为空、注入零内容，行级层距谓词自然不满足——严格语义）。
		injectRefSnapshots(ruleCtx, gate, refScopeProvidersOf(rule))
		hit := rule.Pattern.Evaluate(in, lib, ruleCtx)
		ctx.absorbScoped(ruleCtx)
		if !hit {
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
			// ApplyDeclarations 多值扇出面：重复段同名 N 绑定产 N 条声明，
			// 普通词一名一绑定恒单条（与既有 Apply 单返回零差异）。
			decision.Declarations = append(decision.Declarations, action.ApplyDeclarations(rule.ID, ruleCtx)...)
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
			// 扇出面与第一遍一致（条件动作同样可重复段多值）。
			decision.Declarations = append(decision.Declarations, action.ApplyDeclarations(match.rule.ID, match.context)...)
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

// refGateState 门段产物：供给方（作用域附件引用的规则）的先行求值结论——
// 命中事实与限定键捕获快照。命中
// 只在此登记为消费侧判据数据，不写 decision.MatchedRuleIDs（命中单点登记＝
// 供给方自身独立段，风险5：门段双求值不重复计数）；捕获快照只注入消费规则的
// 私有作用域（捕获私有不变式不动，限定名通道为加法式注入——发现6）。
type refGateState struct {
	// hits 为供给方 ID → 门段命中结论；缺席（不在场/停用/被跳过/未求值）按未命中。
	hits map[string]bool
	// snapshots 为供给方 ID → 限定键捕获快照（命中供给方私有作用域捕获的副本，
	// 键形态「供给方ID.rN.组名」，#canon 规范名变体一并快照）。
	snapshots map[string][]CaptureBinding
	// dirHitDepths 为供给方 ID → 该供给方目录行实际命中的层深集合（同链同层
	// 即同目录；Or 行短路只记实际发生命中的行）——refScope 附件层距阶梯判据
	//（裁决⑥）的消费数据（经 ParseContext.RefScopeDepths 透传）。仅命中且
	// 有目录锚定层的供给方在场；供给方未命中＝缺席、命中但无目录行（如全行
	// 级非形态）＝不进场——两者对附件谓词同为不可满足。
	dirHitDepths map[string]map[int]bool
}

// snapshot 取某供给方的限定键捕获快照（供给方未命中时为空）。
func (g *refGateState) snapshot(memberID string) []CaptureBinding {
	return g.snapshots[memberID]
}

// runRefGateStage 门段（求值两段化的第一段，先于提取段主循环——与「定群段
// 先于提取段」同款机制）：对本批规则中会被求值的规则（启用、模式在场、非定
// 群形态、非聚合类别——聚合条件行禁作用域附件，载入侧已拦）的条件行递归
// 扫描 refScope 附件，对引用到的供给方全体先求值（needed 收集自行扫描条件行）。
//   - 供给方仍在各自私有作用域求值（ruleScopedContext）；求值结论不并回共享
//     上下文（不调 absorbScoped——门段不得扰动主循环的求值次序面，非消费
//     路径零变化；群组基线取门段时点即定群/结构群补位后的状态）；
//   - 供给方停用、被跳过（定群形态或模式缺失）或不在本规则集内＝未命中
//     （严格语义：含供给方删除后悬空引用的求值期形态——附件谓词恒不满足）；
//   - 门段不写 MatchedRuleIDs——供给方命中由其自身独立段单点登记，追加序
//     保持既有规则序面（风险2）。
//
// 本批无 refScope 附件时零工作（空门，主循环行为与既有完全一致）。
// 供给方按依赖拓扑序求值：自身携带作用域附件的供给方（多级链成员）经共享
// 层集映射读到库根常量与先求值上游的命中层集；不带附件的独立供给方求值
// 纯函数、次序不影响其结论。环成员（构造校验已拒的防御形态）不求值＝未命中。
// 库根 sentinel（RefScopeLibraryRoot）例外：非供给方规则、无求值，命中层集
// 恒按常量 {-1} 注入（绝对层级语义见 RefScopeLibraryRoot 注释）。
func runRefGateStage(rules []RuleEntry, in *MatchInput, lib *LibraryIndex, ctx *ParseContext) *refGateState {
	gate := &refGateState{hits: map[string]bool{}, snapshots: map[string][]CaptureBinding{}, dirHitDepths: map[string]map[int]bool{}}
	// 层集映射即刻挂共享上下文：门段内求值的供给方（自身携带作用域附件的
	// 链式形态）经 ruleScopedContext 继承同一映射——库根常量与先求值供给方
	// 登记的命中层集对后求值者可见（各调用面逐文件新建上下文，层集无跨文件
	// 残留）；提取段主循环入口的同名赋值为同映射幂等。
	ctx.RefScopeDepths = gate.dirHitDepths
	needed := map[string]bool{}
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.Pattern == nil || isGroupingRule(rule) || rule.Category == RuleCategoryAggregate {
			continue
		}
		collectRefScopeProviders(rule.Conditions, needed)
	}
	// 库根 sentinel（RefScopeLibraryRoot）：命中层集恒为 {-1}（库根之上的
	// 虚拟层），无供给方求值——自 needed 摘除后按常量注入，层距阶梯谓词同一
	// 算术即得绝对层级语义（紧邻=第 1 级、隔 N=第 N 级、任意=恒真）。
	if needed[RefScopeLibraryRoot] {
		gate.dirHitDepths[RefScopeLibraryRoot] = map[int]bool{-1: true}
		delete(needed, RefScopeLibraryRoot)
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
	// 求值序=依赖拓扑序（Kahn）：供给方自身携带作用域附件（多级链）时，其
	// 附件谓词读取上游供给方的命中层集，上游须先于下游求值。子图节点=needed
	// 全体，边=成员→其规则来源供给方（refScopeProviderRuleIDs 取边滤除库根
	// sentinel——非规则 ID 不构成依赖边；悬空引用不在规则集，不产生边）；
	// 按规则序枚举节点与出边，出队序确定。环成员（构造校验已拒的防御形态）
	// 入度恒正、不入队＝不求值＝未命中。
	members := make([]string, 0, len(needed))
	for i := range rules {
		if id := rules[i].ID; needed[id] && !containsString(members, id) {
			members = append(members, id)
		}
	}
	// 边只连到在场节点：悬空引用（在 needed、不在规则集）不产生边——其消费
	// 方照常求值、附件谓词按层集缺席不满足（与停用供给方同判未命中）。
	memberSet := make(map[string]bool, len(members))
	for _, id := range members {
		memberSet[id] = true
	}
	indegree := make(map[string]int, len(members))
	consumers := make(map[string][]string, len(members))
	for _, id := range members {
		member, ok := byID[id]
		if !ok {
			continue
		}
		for _, providerID := range refScopeProviderRuleIDs(member) {
			if !memberSet[providerID] {
				continue
			}
			indegree[id]++
			consumers[providerID] = append(consumers[providerID], id)
		}
	}
	queue := make([]string, 0, len(members))
	for _, id := range members {
		if indegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		memberID := queue[0]
		queue = queue[1:]
		if member, ok := byID[memberID]; ok && member.Enabled && member.Pattern != nil && !isGroupingRule(member) {
			memberCtx := ctx.ruleScopedContext()
			hit := member.Pattern.Evaluate(in, lib, memberCtx)
			gate.hits[memberID] = hit
			if hit {
				gate.snapshots[memberID] = refCaptureSnapshot(memberID, memberCtx)
				// 目录行命中层集合留档（refScope 层距阶梯判据；空集=供给方无
				// 目录锚定层，附件谓词不可满足）。
				for _, dh := range memberCtx.DirHits {
					if gate.dirHitDepths[memberID] == nil {
						gate.dirHitDepths[memberID] = map[int]bool{}
					}
					gate.dirHitDepths[memberID][dh.Depth] = true
				}
			}
		} // 不在场/停用/被跳过＝未命中（hits 缺席即 false），其下游边照常放行
		for _, consumer := range consumers[memberID] {
			indegree[consumer]--
			if indegree[consumer] == 0 {
				queue = append(queue, consumer)
			}
		}
	}
	return gate
}

// injectRefSnapshots 按传入的供给方集把门段捕获快照以限定键挂入消费规则的
// 私有作用域（仅本规则可见的注入区；供给方缺席时快照为空、零注入）。
func injectRefSnapshots(ruleCtx *ParseContext, gate *refGateState, refs []string) {
	for _, ref := range refs {
		ruleCtx.RefCaptures = append(ruleCtx.RefCaptures, gate.snapshot(ref)...)
	}
}

// collectRefScopeProviders 递归扫描条件行列表（含括号子组），把 refScope 附件
// 引用的供给方规则 ID 收进 out（门段 needed 收集的行扫描入口——阶段2 门段
// 入口改造；行级非的行照扫：附件属行、随行求值，求值面语义由谓词槽位自然
// 承接，自决1）。
func collectRefScopeProviders(rows []CondxRow, out map[string]bool) {
	for i := range rows {
		row := &rows[i]
		if row.Condition != nil {
			for _, a := range row.Condition.Attach {
				if a.Kind == CondxAttachRefScope && a.Value != "" {
					out[a.Value] = true
				}
			}
		}
		if len(row.Group) > 0 {
			collectRefScopeProviders(row.Group, out)
		}
	}
}

// refScopeProvidersOf 收集规则的供给方规则 ID 集（条件行递归扫描 refScope 附件，
// 按出现序去重）：门段快照注入、命中预览被引用态判定（ruleReferencesRuleID
// 行扫描）与构造校验（rule_profile.go 的供给方集口径）共用。
func refScopeProvidersOf(rule *RuleEntry) []string {
	if rule == nil {
		return nil
	}
	return refScopeProvidersOfRows(rule.Conditions)
}

// refScopeProvidersOfRows 收集条件行列表（含括号子组递归）引用的供给方规则
// ID 集，按出现序去重——refScopeProvidersOf 的行集直通版（构造校验在规则
// 中间形态〔ruleEntryJSON〕上取供给方集）。
func refScopeProvidersOfRows(rows []CondxRow) []string {
	var providers []string
	var walk func(rows []CondxRow)
	walk = func(rows []CondxRow) {
		for i := range rows {
			row := &rows[i]
			if row.Condition != nil {
				for _, a := range row.Condition.Attach {
					if a.Kind == CondxAttachRefScope && a.Value != "" &&
						!containsString(providers, a.Value) {
						providers = append(providers, a.Value)
					}
				}
			}
			if len(row.Group) > 0 {
				walk(row.Group)
			}
		}
	}
	walk(rows)
	return providers
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
	// 内容为伴随说明文件里人工书写的说明文本，与目录策展同级（高于文件名
	// 解析与谓词确证，低于赋值/裁决）；同层不同值经 highTrust 维度进待裁决
	// ——冲突可见不静默丢失。
	SourceContent: 5,
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
		// union 分支（含缺省回退与未知维度的保底）：按值去重，保首个声明；
		// 被吞并的同值声明若出自其他规则，规则 ID 记入保留声明的附属清单。
		retainedIdx := make(map[string]int, len(group))
		for _, decl := range group {
			if idx, ok := retainedIdx[decl.Value]; ok {
				absorbAlsoRuleID(&final[idx], decl)
				continue
			}
			retainedIdx[decl.Value] = len(final)
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

// absorbAlsoRuleID 同值吞并时登记被吞声明的规则出处：被吞声明无规则 ID
// （内置惯例或纯捕获形态）不记；与保留声明自身同规则（含已登记过的）不重复
// 记。声明按档案序到达，追加顺序即附属清单顺序。
func absorbAlsoRuleID(retained *MetadataDeclaration, absorbed MetadataDeclaration) {
	if absorbed.RuleID == "" || absorbed.RuleID == retained.RuleID {
		return
	}
	for _, id := range retained.AlsoRuleIDs {
		if id == absorbed.RuleID {
			return
		}
	}
	retained.AlsoRuleIDs = append(retained.AlsoRuleIDs, absorbed.RuleID)
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

// —— 伴随说明内容的两相提取（内容绑定 → 命中集登记 → 汇流后整段取值产声明）——

// contentBindingPlan 单条聚合规则内容绑定的求值预案：各绑定行编译为独立求值
// 的行包装模式（与聚合段条件行同一编译口径，行号=先序展平，括号子组内层行
// 照常按行寻址）。
type contentBindingPlan struct {
	// rule 为源规则（引用传入规则序列的条目）。
	rule *RuleEntry
	// bindings 为绑定的先序副本（rows 与之同长对位）。
	bindings []ContentBinding
	// rows 为各绑定行的编译产物（下标与 bindings 对位）。
	rows []Pattern
}

// contentBindingPlans 过滤携带内容绑定（「元数据文件」产出，住规则级落档位）
// 的启用规则并编译绑定行——聚合规则（并用）与映射单用规则同源供给。行号
// 越界或行编译失败按宁缺勿错整条跳过——与聚合段 buildAggregationPlans 同款
// 防御（载入校验已拦的形态在此双保险；括号子组/行级非行照常编译——决策10
// 放开行结构限制，行级命中照常按行）。无绑定时返回空（现状行为零变化）。
func contentBindingPlans(rules []RuleEntry) []*contentBindingPlan {
	var plans []*contentBindingPlan
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.MetadataFile == nil ||
			len(rule.MetadataFile.ContentBindings) == 0 ||
			len(rule.Conditions) == 0 {
			continue
		}
		leaves := condxLeafRowsOf(rule.Conditions)
		byNo := make(map[int]*CondxRow, len(leaves))
		for _, leaf := range leaves {
			byNo[leaf.no] = leaf.row
		}
		bindings := rule.MetadataFile.ContentBindings
		rows := make([]Pattern, len(bindings))
		valid := true
		for j := range bindings {
			rowNo := bindings[j].Row
			leaf, ok := byNo[rowNo]
			if !ok {
				valid = false
				break
			}
			p, err := compileCondx(leaf.Condition, rowNo)
			if err != nil {
				valid = false
				break
			}
			if leaf.Negated {
				p = Not{Inner: p}
			}
			rows[j] = p
		}
		if !valid {
			continue
		}
		plans = append(plans, &contentBindingPlan{rule: rule, bindings: bindings, rows: rows})
	}
	return plans
}

// contentBindingInputs 从同批结论构造内容绑定行的求值输入（两相共用同一基座
// ——第一相命中集登记与第二相整段取值由此取同一批输入，命中面恒一致）。同
// 目录兄弟表按结论同目录聚合、各层祖先直接子文件面按本批结论派生：增量批为
// 求值面子集，目录终结谓词对此保守不命中（与 MatchInput 祖先面缺席的契约
// 同款）。库内谓词索引不注入（nil）：绑定行携带库内谓词时保守不命中。
func contentBindingInputs(conclusions []FileConclusion) []*MatchInput {
	siblingsByDir := make(map[string][]string, len(conclusions))
	for i := range conclusions {
		key := conclusions[i].DirRelKey
		siblingsByDir[key] = append(siblingsByDir[key], conclusions[i].FileName)
	}
	inputs := make([]*MatchInput, len(conclusions))
	for i := range conclusions {
		c := &conclusions[i]
		dirNames := ExtractDirLevels(c.RelPath)
		ancestorFiles := make([][]string, len(dirNames))
		for d := range dirNames {
			ancestorFiles[d] = siblingsByDir[strings.Join(dirNames[:d+1], "/")]
		}
		inputs[i] = &MatchInput{
			FileName:      c.FileName,
			DirNames:      dirNames,
			RelPath:       c.RelPath,
			Siblings:      siblingsByDir[c.DirRelKey],
			AncestorFiles: ancestorFiles,
		}
	}
	return inputs
}

// anyRowHit 报告输入是否命中该预案的任一绑定行（每行独立作用域求值，与聚合
// 段条件行同款纪律；命中只判事实，捕获不外溢）。
func (p *contentBindingPlan) anyRowHit(in *MatchInput) bool {
	for j := range p.rows {
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		if p.rows[j].Evaluate(in, nil, ctx) {
			return true
		}
	}
	return false
}

// contentDeclarationsOf 按该预案的内容绑定对单文件产声明（第二相整段取值）：
// 绑定行命中即把注入的内容片段整段声明到绑定维度，来源=内容、截断事实随声明
// 标注、归属产出规则 ID。内容为空（未采集/拒读）时调用方已先行跳过，此处不
// 再判空；行未命中零产出。
func (p *contentBindingPlan) contentDeclarationsOf(in *MatchInput) []MetadataDeclaration {
	var out []MetadataDeclaration
	for j := range p.rows {
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		if !p.rows[j].Evaluate(in, nil, ctx) {
			continue
		}
		out = append(out, MetadataDeclaration{
			Dimension: p.bindings[j].Dimension,
			Value:     in.CompanionText,
			Source:    SourceContent,
			Truncated: in.CompanionTruncated,
			RuleID:    p.rule.ID,
		})
	}
	return out
}

// applyCompanionContentDeclarations 两相提取的第二相（采集注入完成后、识别
// 装配消费前）：对携带内容片段的文件按内容绑定整段取值产声明，与提取段原始
// 声明合并重合成终选与待裁决，会话赋值按维度覆盖重放（特异性知识压过内容
// 声明，与逐文件求值期的合并语义一致）。空内容/全空白=零声明（取不到值
// 语义，取值面判空白、声明值保留原文整段）；无绑定规则时零工作直接返回。
// conclusions 与 files 按下标对齐（调用方保证同一求值清单）。
func applyCompanionContentDeclarations(conclusions []FileConclusion, files []ScanFile, rules []RuleEntry, assigns map[string]SessionAssignment) {
	plans := contentBindingPlans(rules)
	if len(plans) == 0 {
		return
	}
	hasContent := false
	for i := range files {
		if files[i].CompanionText != "" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return
	}
	inputs := contentBindingInputs(conclusions)
	for i := range files {
		if i >= len(inputs) {
			break
		}
		inputs[i].CompanionText = files[i].CompanionText
		inputs[i].CompanionTruncated = files[i].CompanionTruncated
	}
	policy := AggregateSynthesis(rules)
	for i := range conclusions {
		if i >= len(files) || strings.TrimSpace(files[i].CompanionText) == "" {
			continue
		}
		var contentDecls []MetadataDeclaration
		for _, plan := range plans {
			contentDecls = append(contentDecls, plan.contentDeclarationsOf(inputs[i])...)
		}
		if len(contentDecls) == 0 {
			continue
		}
		merged := append([]MetadataDeclaration{}, conclusions[i].rawDecls...)
		merged = append(merged, contentDecls...)
		final, pending := Synthesize(merged, policy)
		if assign := resolveFileAssignment(assigns, conclusions[i].RelPath); len(assign.coveredDimensions()) > 0 {
			final, pending = applyAssignmentToConclusion(final, pending, &assign)
		}
		conclusions[i].Final, conclusions[i].Pending = final, pending
	}
}
