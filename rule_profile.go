// 规则档案模型：含义维度 / 来源 / 提取 / 合成的类型定义，档案的多态 JSON 序列化与载入校验；纯模型，无 IO 与日志。
package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MetadataDimension 为元数据的含义维度：同一份声明值在不同维度下语义不同。
type MetadataDimension string

const (
	// DimAuthor 表示作者维度。
	DimAuthor MetadataDimension = "author"
	// DimTag 表示标签维度。
	DimTag MetadataDimension = "tag"
	// DimTitle 表示标题维度。
	DimTitle MetadataDimension = "title"
	// DimCollection 表示作品集维度。
	DimCollection MetadataDimension = "collection"
)

// SourceKind 为元数据值的来源载体类别。
type SourceKind string

const (
	// SourceDir 表示目录策展：值来自目录名。
	SourceDir SourceKind = "dir"
	// SourceFilename 表示文件名解析：值来自文件名的正则捕获。
	SourceFilename SourceKind = "filename"
	// SourceConstant 表示常量声明：值为规则内声明的固定文本。
	SourceConstant SourceKind = "constant"
	// SourceGroup 表示群组分析：值来自兄弟群组的公共前缀。
	SourceGroup SourceKind = "group"
	// SourcePredicate 表示谓词命中：值来自库内集合命中的名字。
	SourcePredicate SourceKind = "predicate"
	// SourceAssignment 表示会话赋值：值来自用户在解析会话内对该文件的手工指定
	//（特异性知识，合成后按维度覆盖档案求值结论，不参与合成择一）。
	SourceAssignment SourceKind = "assignment"
	// SourceAdjudication 表示裁决声明：值来自用户对声明冲突的裁决结果
	//（经裁决生成的规则以常量动作产出，信任级高于全部规则推导来源，见
	// pipeline.go 的 sourceTrust）。
	SourceAdjudication SourceKind = "adjudication"
)

// ExtractionFrom 为提取动作的取值来源。
type ExtractionFrom string

const (
	// FromCapture 表示从解析上下文的命名捕获组取值。
	FromCapture ExtractionFrom = "capture"
	// FromGroupPrefix 表示从群组分析产物取公共前缀。
	FromGroupPrefix ExtractionFrom = "groupPrefix"
	// FromParentDir 表示取直接父目录名。
	FromParentDir ExtractionFrom = "parentDir"
	// FromAncestorDir 表示取最深一条目录层命中。
	FromAncestorDir ExtractionFrom = "ancestorDir"
	// FromPredicate 表示取最后一条谓词命中。
	FromPredicate ExtractionFrom = "predicate"
	// FromConstant 表示取动作内声明的常量。
	FromConstant ExtractionFrom = "constant"
	// FromAdjudication 表示取动作内声明的裁决结果常量（冲突裁决生成规则的专用
	// 取值来源，产出声明携带裁决来源，合成时压过全部规则推导来源）。
	FromAdjudication ExtractionFrom = "adjudication"
	// FromScan 表示贪婪识别（库内确认）：扫描候选分段（整段/括号内容/分隔符
	// 切分）与库内对应集合精确等值比对，命中者产出声明——查不到不提取
	//（确认非线索口径）；多值与歧义语义见 rule_scan.go（图形帧收敛档 2026-10-23）。
	FromScan ExtractionFrom = "scan"
)

// SynthStrategy 为同一含义维度多来源候选值的合成策略。
type SynthStrategy string

const (
	// StrategyUnion 表示并集：全部候选值都保留。
	StrategyUnion SynthStrategy = "union"
	// StrategyHighTrust 表示高可信源择一：仅从高可信来源中取一个值。
	StrategyHighTrust SynthStrategy = "highTrust"
)

// RuleSource 为规则的产出途径。
type RuleSource string

const (
	// RuleSourceBuiltin 表示内置规则。
	RuleSourceBuiltin RuleSource = "builtin"
	// RuleSourceManual 表示人工手写规则。
	RuleSourceManual RuleSource = "manual"
	// RuleSourceQna 表示问答向导生成的规则；手动解析的泛化生成沿用此来源
	//（「问答=规则生成器」语义：与用户交互产出规则的一切实入口同源标注）。
	RuleSourceQna RuleSource = "qna"
	// RuleSourceAdjudication 表示冲突裁决生成的规则（用户对声明冲突裁决后产出的
	// 组合规则）。
	RuleSourceAdjudication RuleSource = "adjudication"
)

// MetadataDeclaration 为提取产物：一条带来源载体与出处的元数据声明。
type MetadataDeclaration struct {
	// Dimension 为含义维度。
	Dimension MetadataDimension `json:"dimension"`
	// Value 为声明值。
	Value string `json:"value"`
	// Source 为值的来源载体。
	Source SourceKind `json:"source"`
	// RoleName 为声明级角色绑定，可为空。
	RoleName string `json:"roleName"`
	// RuleID 为产出该声明的规则 ID。
	RuleID string `json:"ruleId"`
}

// DeclPredicate 声明谓词：某来源载体在某含义维度上的声明是否在场
// （条件表达式树的布尔叶节点，也是 v2 谓词组合形态的最小操作数）。
type DeclPredicate struct {
	// Source 为待判定的声明来源载体。
	Source SourceKind `json:"source"`
	// Dimension 为待判定的含义维度。
	Dimension MetadataDimension `json:"dimension"`
}

// SynthCondition 声明在场/缺席谓词组合（档案 v2 的条件承载）：Present 内谓词
// 须全部在场、Absent 内谓词须全部缺席——即谓词的 And 组合（最简充分形式通常
// 为单谓词单列表），两列表皆空为恒真。v3 起条件统一为通用表达式树（CondExpr），
// 本类型降为旧档案 JSON 的兼容读中间表示与等价性参照：载入时经
// legacyConditionExpr 映射为等价表达式树，直读求值（Holds）保留供等价性锚定。
type SynthCondition struct {
	// Present 为须在场的声明谓词列表。
	Present []DeclPredicate `json:"present,omitempty"`
	// Absent 为须缺席的声明谓词列表。
	Absent []DeclPredicate `json:"absent,omitempty"`
}

// ExprKind 为条件表达式节点的判别类别。
type ExprKind string

const (
	// ExprPredicate 声明在场谓词操作数：某来源在某含义维度上的声明是否在场。
	ExprPredicate ExprKind = "predicate"
	// ExprCapture 命名捕获组操作数：取该捕获组名最后一次命中的捕获文本。
	ExprCapture ExprKind = "capture"
	// ExprDepth 目录深度操作数：条目所在目录的深度（DirNames 长度，根直下为 0）。
	ExprDepth ExprKind = "depth"
	// ExprGroupSize 群组产物操作数：所属群组的成员总数；非群组成员时取值不可用。
	ExprGroupSize ExprKind = "groupSize"
	// ExprGroupSeq 群组产物操作数：本条目在群组内的序号（从 1 起）；非群组成员时取值不可用。
	ExprGroupSeq ExprKind = "groupSeq"
	// ExprString 字符串字面量操作数。
	ExprString ExprKind = "string"
	// ExprNumber 数值字面量操作数。
	ExprNumber ExprKind = "number"
	// ExprEqual 相等比较：两侧操作数同型（字符串或数值）且取值相等。
	ExprEqual ExprKind = "equal"
	// ExprNotEqual 不等比较：相等比较的否定。
	ExprNotEqual ExprKind = "notEqual"
	// ExprRange 数值区间比较：操作数落在 [min, max] 闭区间内，单边界的另一侧无界。
	ExprRange ExprKind = "range"
	// ExprAnd 逻辑与：全部子表达式为真。
	ExprAnd ExprKind = "and"
	// ExprOr 逻辑或：任一子表达式为真。
	ExprOr ExprKind = "or"
	// ExprNot 逻辑非：内层表达式取反。
	ExprNot ExprKind = "not"
)

// CondExpr 通用条件表达式树节点（合成策略条件绑定的语义层，档案 v3 起承载）：
// 按 Kind 判别共用一个节点结构——操作数（声明谓词 / 命名捕获组 / 目录深度 /
// 群组产物字段 / 字面量）与运算符（比较 equal / notEqual / range，逻辑 and /
// or / not）。求值输入 = 解析上下文 + 求值时点已产出的声明集（见 pipeline.go
// 的 Holds）；构造期静态校验（引用存在性 / 类型匹配）见 validateExpr。v2 的
// 谓词组合（SynthCondition）是本树的节点子集，载入时映射为等价表达式树。
type CondExpr struct {
	// Kind 为节点判别类别。
	Kind ExprKind `json:"kind"`
	// Source / Dimension 为 predicate 的声明来源载体与含义维度。
	Source    SourceKind        `json:"source,omitempty"`
	Dimension MetadataDimension `json:"dimension,omitempty"`
	// CaptureName 为 capture 引用的命名捕获组名（构造期静态校验存在于
	// 规则模式声明的捕获组名列表内）。
	CaptureName string `json:"captureName,omitempty"`
	// Str 为 string 字面量的值。
	Str string `json:"str,omitempty"`
	// Num 为 number 字面量的值。
	Num float64 `json:"num,omitempty"`
	// Left / Right 为 equal / notEqual 的两侧操作数。
	Left  *CondExpr `json:"left,omitempty"`
	Right *CondExpr `json:"right,omitempty"`
	// Operand 为 range 的被比较操作数。
	Operand *CondExpr `json:"operand,omitempty"`
	// Min / Max 为 range 的闭区间界，nil 表示该侧无界。
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
	// Inner 为 not 的内层表达式。
	Inner *CondExpr `json:"inner,omitempty"`
	// Subs 为 and / or 的子表达式列表。
	Subs []*CondExpr `json:"subs,omitempty"`
}

// predicateExpr 声明谓词 → 在场谓词叶节点。
func predicateExpr(pred DeclPredicate) *CondExpr {
	return &CondExpr{Kind: ExprPredicate, Source: pred.Source, Dimension: pred.Dimension}
}

// legacyConditionExpr v2 谓词组合 → 等价表达式树：Present 各谓词原样合取、
// Absent 各谓词取反后合取（p₁∧…∧¬a₁∧…）；单谓词归一为裸节点（最简形式），
// 两列表皆空的恒真形态归一为无条件（nil）。
func legacyConditionExpr(c SynthCondition) *CondExpr {
	subs := make([]*CondExpr, 0, len(c.Present)+len(c.Absent))
	for _, pred := range c.Present {
		subs = append(subs, predicateExpr(pred))
	}
	for _, pred := range c.Absent {
		subs = append(subs, &CondExpr{Kind: ExprNot, Inner: predicateExpr(pred)})
	}
	if len(subs) == 0 {
		return nil
	}
	if len(subs) == 1 {
		return subs[0]
	}
	return &CondExpr{Kind: ExprAnd, Subs: subs}
}

// ExtractionAction 为一条提取动作：规则命中后从解析上下文取值并声明到某含义维度。
type ExtractionAction struct {
	// Dimension 为声明到的含义维度。
	Dimension MetadataDimension `json:"dimension"`
	// From 为取值来源。
	From ExtractionFrom `json:"from"`
	// CaptureName 为 From 为 capture 时的捕获组名。
	CaptureName string `json:"captureName"`
	// Constant 为 From 为 constant / adjudication 时的常量值。
	Constant string `json:"constant"`
	// ScanScope 为 From 为 scan 时的扫描范围："path"=文件名去扩展名+各级目录名
	//（默认，空串同值）；"uncovered"=同左，但排除与本规则捕获区间（同分段内）
	// 重叠的候选（仅上半未覆盖片段）。
	ScanScope string `json:"scanScope,omitempty"`
	// RoleName 为声明级角色绑定，可为空；原样透传到产出声明。
	RoleName string `json:"roleName"`
	// StripPrefix / StripSuffix 取值后的剥壳变换（结构映射泛化产物使用）：值
	// 具有该前缀/后缀时剥离（如 ancestorDir 取得 "Artist[X]" 剥为 "X"）；空=
	// 不剥。逐侧独立 Trim，值不具该侧时为无操作，旧规则（双空）行为不变。
	StripPrefix string `json:"stripPrefix,omitempty"`
	StripSuffix string `json:"stripSuffix,omitempty"`
	// When 为可选生效条件（通用条件表达式树）：条件不满足时该动作不产出声明
	//（规则照常命中计数）。v2 谓词组合形态的 JSON 在载入时映射为等价表达式树
	//（parseConditionJSON），内存中恒为表达式树单一形态。
	When *CondExpr `json:"when,omitempty"`
}

// Apply 在解析上下文 ctx 上执行提取动作，产出归属规则 ruleID 的声明；
// 来源缺失（捕获组不存在 / 非群组成员 / 无目录层 / 无谓词命中）或 ctx 为 nil
// 或 From 取值未知时返回零值与 false，不产出声明。
func (a ExtractionAction) Apply(ruleID string, ctx *ParseContext) (MetadataDeclaration, bool) {
	if ctx == nil {
		return MetadataDeclaration{}, false
	}
	var value string
	var source SourceKind
	switch a.From {
	case FromCapture:
		captured, ok := ctx.Capture(a.CaptureName)
		if !ok {
			return MetadataDeclaration{}, false
		}
		value, source = captured, SourceFilename
	case FromGroupPrefix:
		if ctx.Group == nil {
			return MetadataDeclaration{}, false
		}
		value, source = ctx.Group.Prefix, SourceGroup
	case FromParentDir:
		if len(ctx.DirNames) == 0 {
			return MetadataDeclaration{}, false
		}
		value, source = ctx.DirNames[len(ctx.DirNames)-1], SourceDir
	case FromAncestorDir:
		hit, ok := ctx.LastDirHit()
		if !ok {
			return MetadataDeclaration{}, false
		}
		value, source = hit.Name, SourceDir
	case FromPredicate:
		if len(ctx.PredHits) == 0 {
			return MetadataDeclaration{}, false
		}
		value, source = ctx.PredHits[len(ctx.PredHits)-1].Name, SourcePredicate
	case FromConstant:
		value, source = a.Constant, SourceConstant
	case FromAdjudication:
		value, source = a.Constant, SourceAdjudication
	default:
		return MetadataDeclaration{}, false
	}
	if a.StripPrefix != "" {
		value = strings.TrimPrefix(value, a.StripPrefix)
	}
	if a.StripSuffix != "" {
		value = strings.TrimSuffix(value, a.StripSuffix)
	}
	return MetadataDeclaration{
		Dimension: a.Dimension,
		Value:     value,
		Source:    source,
		RoleName:  a.RoleName,
		RuleID:    ruleID,
	}, true
}

// DimPolicy 为单个含义维度的合成策略。
type DimPolicy struct {
	// Dimension 为策略作用的含义维度。
	Dimension MetadataDimension `json:"dimension"`
	// Strategy 为该维度的候选值合成策略。
	Strategy SynthStrategy `json:"strategy"`
}

// RoleBinding 为源到角色的无条件硬绑定：凡来自该来源的声明一律附加该角色。
type RoleBinding struct {
	// Source 为绑定作用的来源载体。
	Source SourceKind `json:"source"`
	// RoleName 为附加的角色名。
	RoleName string `json:"roleName"`
}

// SynthesisPolicy 为合成策略模型：按含义维度的策略 + 源到角色的无条件硬绑定。
type SynthesisPolicy struct {
	// DimPolicies 为各含义维度的合成策略。
	DimPolicies []DimPolicy `json:"dimPolicies"`
	// Roles 为源到角色的无条件硬绑定列表。
	Roles []RoleBinding `json:"roles"`
}

// RuleEntry 为一条完整规则：模式 + 提取动作 + 合成策略。
type RuleEntry struct {
	// ID 为规则唯一标识。
	ID string `json:"id"`
	// Description 为规则的人类可读描述。
	Description string `json:"description"`
	// Source 为规则的产出途径。
	Source RuleSource `json:"source"`
	// Enabled 为启用开关；禁用的规则不参与匹配。
	Enabled bool `json:"enabled"`
	// Pattern 为命中判定的模式树；序列化时按 kind 判别字段多态展开。
	Pattern Pattern `json:"pattern"`
	// Extractions 为命中后依序执行的提取动作。
	Extractions []ExtractionAction `json:"extractions"`
	// Synthesis 为该规则产出声明的合成策略。
	Synthesis SynthesisPolicy `json:"synthesis"`
}

// CurrentSchemaVersion 为规则档案当前的结构版本。
// v3：提取动作的生效条件升级为通用条件表达式树（CondExpr）；v2 的谓词组合
// 形态与 v1 的无条件形态均可载入（读侧向后兼容，载入即映射/升格为 v3），重写
// 后落盘为 v3。
const CurrentSchemaVersion = 3

// legacySchemaVersions 为可向后兼容载入的历史结构版本
// （v1=无条件形态，v2=谓词组合条件形态——载入时映射为等价表达式树）。
var legacySchemaVersions = map[int]bool{1: true, 2: true}

// RuleProfile 为规则档案：一批规则的容器。
type RuleProfile struct {
	// SchemaVersion 为档案结构版本，载入时校验。
	SchemaVersion int `json:"schemaVersion"`
	// Rules 为档案内的规则列表。
	Rules []RuleEntry `json:"rules"`
	// DisabledBuiltinIDs 为内置规则禁用清单：内置规则在代码内置表、不入档案
	// 条目，其禁用态由本清单承载——清单内的内置规则 ID 在合并求值点
	//（EffectiveRules）被排除、不参与求值。字段可选（omitempty），旧档案缺省
	// 为空清单即全部内置参与求值；清单中不在内置表的 ID 原样保留、无匹配即
	// 无效果。
	DisabledBuiltinIDs []string `json:"disabledBuiltinIDs,omitempty"`
}

// 模式 JSON 的 kind 判别值。
const (
	patternKindRegex     = "regex"
	patternKindLocation  = "location"
	patternKindSibling   = "sibling"
	patternKindPredicate = "predicate"
	patternKindAnd       = "and"
	patternKindOr        = "or"
	patternKindNot       = "not"
)

// patternJSON 为 Pattern 的多态 JSON 中间表示：kind 判别字段 + 各具体模式
// 字段的并集；嵌套子模式（ancestor / patterns / inner）递归同构，任意深度可用。
type patternJSON struct {
	// Kind 为判别字段：regex / location / sibling / predicate / and / or / not。
	Kind string `json:"kind"`
	// Target 为 regex 的匹配目标（filename / dirname / relpath）。
	Target string `json:"target,omitempty"`
	// Expr 为 regex 的正则表达式原文。
	Expr string `json:"expr,omitempty"`
	// Captures 为 regex 的命名捕获组名列表。
	Captures []string `json:"captures,omitempty"`
	// MinDepth 为 location 的最小目录深度（含）。
	MinDepth int `json:"minDepth,omitempty"`
	// MaxDepth 为 location 的最大目录深度（含），小于 0 表示不限上界。
	MaxDepth int `json:"maxDepth,omitempty"`
	// Ancestor 为 location 作用于单层目录名的子模式。
	Ancestor *patternJSON `json:"ancestor,omitempty"`
	// KeyExpr 为 sibling 的分组键正则，空串表示键取去扩展名的文件名。
	KeyExpr string `json:"keyExpr,omitempty"`
	// SeqExpr 为 sibling 的序号提取正则。
	SeqExpr string `json:"seqExpr,omitempty"`
	// MinGroupSize 为 sibling 的成群最少成员数。
	MinGroupSize int `json:"minGroupSize,omitempty"`
	// Outcome 为 sibling 命中群组后的归属类别。
	Outcome string `json:"outcome,omitempty"`
	// PredKind 为 predicate 的谓词类别（author / tag / workSet）；判别字段已占用 kind，故另名序列化。
	PredKind string `json:"predKind,omitempty"`
	// From 为 predicate 的取值来源：空串表示文件名去扩展名，否则为捕获组名。
	From string `json:"from,omitempty"`
	// Patterns 为 and / or 的子模式列表。
	Patterns []*patternJSON `json:"patterns,omitempty"`
	// Inner 为 not 的内层模式。
	Inner *patternJSON `json:"inner,omitempty"`
}

// ruleEntryJSON 为 RuleEntry 的序列化中间结构：Pattern 接口换为多态中间表示。
type ruleEntryJSON struct {
	ID          string             `json:"id"`
	Description string             `json:"description"`
	Source      RuleSource         `json:"source"`
	Enabled     bool               `json:"enabled"`
	Pattern     *patternJSON       `json:"pattern"`
	Extractions []ExtractionAction `json:"extractions"`
	Synthesis   SynthesisPolicy    `json:"synthesis"`
}

// ruleProfileJSON 为 RuleProfile 的序列化中间结构。
type ruleProfileJSON struct {
	SchemaVersion int             `json:"schemaVersion"`
	Rules         []ruleEntryJSON `json:"rules"`
	// DisabledBuiltinIDs 为内置规则禁用清单（可选字段，语义见 RuleProfile 同名字段）。
	DisabledBuiltinIDs []string `json:"disabledBuiltinIDs,omitempty"`
}

// patternToJSON 把模式树递归转为多态中间表示；nil 模式转为 nil。
func patternToJSON(p Pattern) (*patternJSON, error) {
	switch t := p.(type) {
	case nil:
		return nil, nil
	case *RegexPattern:
		return &patternJSON{Kind: patternKindRegex, Target: string(t.Target), Expr: t.Expr, Captures: t.Captures}, nil
	case *LocationPattern:
		ancestor, err := patternToJSON(t.Ancestor)
		if err != nil {
			return nil, err
		}
		return &patternJSON{Kind: patternKindLocation, MinDepth: t.MinDepth, MaxDepth: t.MaxDepth, Ancestor: ancestor}, nil
	case *SiblingPattern:
		return &patternJSON{
			Kind:         patternKindSibling,
			KeyExpr:      t.KeyExpr,
			SeqExpr:      t.SeqExpr,
			MinGroupSize: t.MinGroupSize,
			Outcome:      string(t.Outcome),
		}, nil
	case *PredicatePattern:
		return &patternJSON{Kind: patternKindPredicate, PredKind: string(t.Kind), From: t.From}, nil
	case And:
		return branchToJSON(patternKindAnd, t)
	case *And:
		return branchToJSON(patternKindAnd, *t)
	case Or:
		return branchToJSON(patternKindOr, t)
	case *Or:
		return branchToJSON(patternKindOr, *t)
	case Not:
		return notToJSON(t.Inner)
	case *Not:
		return notToJSON(t.Inner)
	default:
		return nil, fmt.Errorf("不支持序列化的模式类型: %T", p)
	}
}

// branchToJSON 把 and / or 的子模式列表转为中间表示。
func branchToJSON(kind string, subs []Pattern) (*patternJSON, error) {
	out := &patternJSON{Kind: kind, Patterns: make([]*patternJSON, 0, len(subs))}
	for _, sub := range subs {
		pj, err := patternToJSON(sub)
		if err != nil {
			return nil, err
		}
		out.Patterns = append(out.Patterns, pj)
	}
	return out, nil
}

// notToJSON 把 not 的内层模式转为中间表示。
func notToJSON(inner Pattern) (*patternJSON, error) {
	pj, err := patternToJSON(inner)
	if err != nil {
		return nil, err
	}
	return &patternJSON{Kind: patternKindNot, Inner: pj}, nil
}

// patternFromJSON 把多态中间表示递归构造回模式树：正则编译、深度约束等构造
// 校验在各构造函数内完成，失败返回中文错误。
func patternFromJSON(pj *patternJSON) (Pattern, error) {
	if pj == nil {
		return nil, nil
	}
	switch pj.Kind {
	case patternKindRegex:
		return NewRegexPattern(pj.Target, pj.Expr, pj.Captures)
	case patternKindLocation:
		ancestor, err := patternFromJSON(pj.Ancestor)
		if err != nil {
			return nil, err
		}
		return NewLocationPattern(pj.MinDepth, pj.MaxDepth, ancestor)
	case patternKindSibling:
		return NewSiblingPattern(pj.KeyExpr, pj.SeqExpr, pj.MinGroupSize, GroupOutcome(pj.Outcome))
	case patternKindPredicate:
		kind := PredicateKind(pj.PredKind)
		switch kind {
		case PredicateAuthor, PredicateTag, PredicateWorkSet:
		default:
			return nil, fmt.Errorf("未知的谓词类别: %q", pj.PredKind)
		}
		return &PredicatePattern{Kind: kind, From: pj.From}, nil
	case patternKindAnd:
		subs, err := patternsFromJSON(pj.Patterns)
		if err != nil {
			return nil, err
		}
		return And(subs), nil
	case patternKindOr:
		subs, err := patternsFromJSON(pj.Patterns)
		if err != nil {
			return nil, err
		}
		return Or(subs), nil
	case patternKindNot:
		inner, err := patternFromJSON(pj.Inner)
		if err != nil {
			return nil, err
		}
		return Not{Inner: inner}, nil
	default:
		return nil, fmt.Errorf("未知的模式判别 kind: %q", pj.Kind)
	}
}

// patternsFromJSON 逐个构造 and / or 的子模式；子项为 null 视为非法。
func patternsFromJSON(pjs []*patternJSON) ([]Pattern, error) {
	subs := make([]Pattern, 0, len(pjs))
	for _, pj := range pjs {
		if pj == nil {
			return nil, fmt.Errorf("子模式不能为空")
		}
		sub, err := patternFromJSON(pj)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

// conditionJSON 为条件字段的反序列化中间结构：kind 在场为 v3 表达式树节点；
// kind 缺省且带 present / absent 列表为 v2 谓词组合形态（载入即映射为等价
// 表达式树）。字段集与 CondExpr 的序列化标签一一对应。
type conditionJSON struct {
	// Kind 为 v3 表达式的判别类别（v2 形态缺省为空）。
	Kind ExprKind `json:"kind,omitempty"`
	// —— v3 表达式节点字段（与 CondExpr 同名同义）——
	Source      SourceKind        `json:"source,omitempty"`
	Dimension   MetadataDimension `json:"dimension,omitempty"`
	CaptureName string            `json:"captureName,omitempty"`
	Str         string            `json:"str,omitempty"`
	Num         float64           `json:"num,omitempty"`
	Left        *conditionJSON    `json:"left,omitempty"`
	Right       *conditionJSON    `json:"right,omitempty"`
	Operand     *conditionJSON    `json:"operand,omitempty"`
	Min         *float64          `json:"min,omitempty"`
	Max         *float64          `json:"max,omitempty"`
	Inner       *conditionJSON    `json:"inner,omitempty"`
	Subs        []*conditionJSON  `json:"subs,omitempty"`
	// —— v2 谓词组合字段（兼容读）——
	Present []DeclPredicate `json:"present,omitempty"`
	Absent  []DeclPredicate `json:"absent,omitempty"`
}

// parseConditionJSON 解析提取动作的 when 字段：kind 在场为 v3 表达式树；kind
// 缺省为 v2 谓词组合形态，映射为等价表达式树（两列表皆空的恒真形态归一为
// 无条件）；null / 缺省字段为无条件（nil）。
func parseConditionJSON(raw []byte) (*CondExpr, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var in conditionJSON
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("条件反序列化失败: %w", err)
	}
	if in.Kind == "" {
		if len(in.Present) == 0 && len(in.Absent) == 0 {
			return nil, nil
		}
		return legacyConditionExpr(SynthCondition{Present: in.Present, Absent: in.Absent}), nil
	}
	return condExprFromJSON(&in)
}

// condExprFromJSON 把条件中间结构递归构造回表达式树；子节点为 null 视为非法
// （判别类别与引用的静态校验在 validateExpr 完成，此处只做结构装配）。
func condExprFromJSON(in *conditionJSON) (*CondExpr, error) {
	if in == nil {
		return nil, fmt.Errorf("条件子表达式不能为空")
	}
	out := &CondExpr{
		Kind:        in.Kind,
		Source:      in.Source,
		Dimension:   in.Dimension,
		CaptureName: in.CaptureName,
		Str:         in.Str,
		Num:         in.Num,
		Min:         in.Min,
		Max:         in.Max,
	}
	child := func(field string, sub *conditionJSON) (*CondExpr, error) {
		if sub == nil {
			return nil, fmt.Errorf("表达式 %q 缺少 %s 子表达式", in.Kind, field)
		}
		return condExprFromJSON(sub)
	}
	var err error
	if in.Left != nil {
		if out.Left, err = child("left", in.Left); err != nil {
			return nil, err
		}
	}
	if in.Right != nil {
		if out.Right, err = child("right", in.Right); err != nil {
			return nil, err
		}
	}
	if in.Operand != nil {
		if out.Operand, err = child("operand", in.Operand); err != nil {
			return nil, err
		}
	}
	if in.Inner != nil {
		if out.Inner, err = child("inner", in.Inner); err != nil {
			return nil, err
		}
	}
	for _, sub := range in.Subs {
		node, err := child("subs", sub)
		if err != nil {
			return nil, err
		}
		out.Subs = append(out.Subs, node)
	}
	return out, nil
}

// UnmarshalJSON 从 JSON 载入提取动作：条件字段经 parseConditionJSON 兼容
// v3 表达式树与 v2 谓词组合两形态（内存中恒为表达式树），其余字段直读；
// 序列化方向直接按结构体标签展开，无自定义 MarshalJSON。
func (a *ExtractionAction) UnmarshalJSON(data []byte) error {
	type extractionJSON struct {
		Dimension   MetadataDimension `json:"dimension"`
		From        ExtractionFrom    `json:"from"`
		CaptureName string            `json:"captureName"`
		Constant    string            `json:"constant"`
		ScanScope   string            `json:"scanScope"`
		RoleName    string            `json:"roleName"`
		StripPrefix string            `json:"stripPrefix"`
		StripSuffix string            `json:"stripSuffix"`
		When        json.RawMessage   `json:"when,omitempty"`
	}
	var in extractionJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("提取动作反序列化失败: %w", err)
	}
	cond, err := parseConditionJSON(in.When)
	if err != nil {
		return err
	}
	*a = ExtractionAction{
		Dimension:   in.Dimension,
		From:        in.From,
		CaptureName: in.CaptureName,
		Constant:    in.Constant,
		ScanScope:   in.ScanScope,
		RoleName:    in.RoleName,
		StripPrefix: in.StripPrefix,
		StripSuffix: in.StripSuffix,
		When:        cond,
	}
	return nil
}

// ruleToJSON 把规则转为序列化中间结构。
func ruleToJSON(rule RuleEntry) (*ruleEntryJSON, error) {
	pj, err := patternToJSON(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("规则 %q 的模式序列化失败: %w", rule.ID, err)
	}
	return &ruleEntryJSON{
		ID:          rule.ID,
		Description: rule.Description,
		Source:      rule.Source,
		Enabled:     rule.Enabled,
		Pattern:     pj,
		Extractions: rule.Extractions,
		Synthesis:   rule.Synthesis,
	}, nil
}

// ruleFromJSON 把规则中间结构构造回 RuleEntry：模式缺失或构造校验失败（含
// 条件表达式的静态校验，捕获组存在性以构造出的模式树为据）返回中文错误。
func ruleFromJSON(rj *ruleEntryJSON) (*RuleEntry, error) {
	if rj.Pattern == nil {
		return nil, fmt.Errorf("规则 %q 缺少模式（pattern）", rj.ID)
	}
	pat, err := patternFromJSON(rj.Pattern)
	if err != nil {
		return nil, fmt.Errorf("规则 %q 的模式非法: %w", rj.ID, err)
	}
	if err := validateExtractions(rj.ID, pat, rj.Extractions); err != nil {
		return nil, err
	}
	return &RuleEntry{
		ID:          rj.ID,
		Description: rj.Description,
		Source:      rj.Source,
		Enabled:     rj.Enabled,
		Pattern:     pat,
		Extractions: rj.Extractions,
		Synthesis:   rj.Synthesis,
	}, nil
}

// knownSourceKinds / knownDimensions 为提取动作构造期校验用的合法取值集。
var knownSourceKinds = map[SourceKind]bool{
	SourceDir: true, SourceFilename: true, SourceConstant: true, SourceGroup: true,
	SourcePredicate: true, SourceAssignment: true, SourceAdjudication: true,
}

var knownDimensions = map[MetadataDimension]bool{
	DimAuthor: true, DimTag: true, DimTitle: true, DimCollection: true,
}

// validateExtractions 逐条校验提取动作的承载字段（构造期静态校验，与正则
// 编译同管线）：裁决取值缺常量、条件表达式引用不存在（谓词来源/维度、捕获组
// 名不在模式声明内）或类型不匹配时返回含规则 ID 与动作序号的中文错误。
// pattern 为该规则的模式树，供捕获组存在性校验。
func validateExtractions(ruleID string, pattern Pattern, actions []ExtractionAction) error {
	declared := declaredPatternCaptures(pattern)
	for i := range actions {
		action := &actions[i]
		if action.From == FromAdjudication && action.Constant == "" {
			return fmt.Errorf("规则 %q 的第 %d 条提取动作为裁决取值但缺少常量（constant）", ruleID, i+1)
		}
		if action.From == FromScan {
			switch action.Dimension {
			case DimAuthor, DimTag, DimCollection:
			default:
				return fmt.Errorf("规则 %q 的第 %d 条提取动作为贪婪识别但维度 %q 无库内集合（仅作者/标签/作品集可扫描）", ruleID, i+1, action.Dimension)
			}
			switch action.ScanScope {
			case "", ScanScopePath, ScanScopeUncovered:
			default:
				return fmt.Errorf("规则 %q 的第 %d 条提取动作的贪婪识别范围非法: %q（path / uncovered）", ruleID, i+1, action.ScanScope)
			}
			if action.When != nil {
				return fmt.Errorf("规则 %q 的第 %d 条贪婪识别动作暂不支持生效条件（when）", ruleID, i+1)
			}
			if action.CaptureName != "" || action.Constant != "" {
				return fmt.Errorf("规则 %q 的第 %d 条贪婪识别动作不应携带捕获组名或常量", ruleID, i+1)
			}
		}
		if action.When == nil {
			continue
		}
		if _, err := validateExpr(ruleID, action.When, declared); err != nil {
			return fmt.Errorf("规则 %q 的第 %d 条提取动作条件非法: %w", ruleID, i+1, err)
		}
	}
	return nil
}

// exprValueType 为条件表达式操作数的静态取值类型（错误信息用人话标签）。
type exprValueType string

const (
	exprTypeBool   exprValueType = "布尔"
	exprTypeString exprValueType = "字符串"
	exprTypeNumber exprValueType = "数值"
)

// validateExpr 构造期静态校验条件表达式树并推断其静态类型：判别类别未知、
// 操作数引用不存在（谓词的来源/维度、捕获组名不在规则模式声明的捕获组名
// 集合内）、运算符对操作数类型不匹配（如对字符串字面量做数值区间比较、
// 逻辑组合接非布尔子式、相等比较两侧类型不一致）时返回中文错误。
// declaredCaptures 为规则模式声明的捕获组名集合。
func validateExpr(ruleID string, e *CondExpr, declaredCaptures map[string]bool) (exprValueType, error) {
	if e == nil {
		return "", fmt.Errorf("规则 %q 的条件表达式不能为空", ruleID)
	}
	switch e.Kind {
	case ExprPredicate:
		if !knownSourceKinds[e.Source] {
			return "", fmt.Errorf("规则 %q 的条件谓词引用了未知的声明来源 %q", ruleID, e.Source)
		}
		if !knownDimensions[e.Dimension] {
			return "", fmt.Errorf("规则 %q 的条件谓词引用了未知的含义维度 %q", ruleID, e.Dimension)
		}
		return exprTypeBool, nil
	case ExprCapture:
		if e.CaptureName == "" {
			return "", fmt.Errorf("规则 %q 的条件表达式引用捕获组但缺少捕获组名（captureName）", ruleID)
		}
		if !declaredCaptures[e.CaptureName] {
			return "", fmt.Errorf("规则 %q 的条件表达式引用了规则模式未声明的捕获组名 %q", ruleID, e.CaptureName)
		}
		return exprTypeString, nil
	case ExprDepth, ExprGroupSize, ExprGroupSeq:
		return exprTypeNumber, nil
	case ExprString:
		return exprTypeString, nil
	case ExprNumber:
		return exprTypeNumber, nil
	case ExprEqual, ExprNotEqual:
		if e.Left == nil || e.Right == nil {
			return "", fmt.Errorf("规则 %q 的比较表达式（%s）缺少左侧（left）或右侧（right）操作数", ruleID, e.Kind)
		}
		leftType, err := validateExpr(ruleID, e.Left, declaredCaptures)
		if err != nil {
			return "", err
		}
		rightType, err := validateExpr(ruleID, e.Right, declaredCaptures)
		if err != nil {
			return "", err
		}
		if leftType != rightType {
			return "", fmt.Errorf("规则 %q 的比较表达式两侧类型不一致（%s 与 %s）", ruleID, leftType, rightType)
		}
		if leftType == exprTypeBool {
			return "", fmt.Errorf("规则 %q 的比较表达式不能比较布尔谓词（布尔量用逻辑与/或/非组合）", ruleID)
		}
		return exprTypeBool, nil
	case ExprRange:
		if e.Operand == nil {
			return "", fmt.Errorf("规则 %q 的数值区间比较缺少操作数（operand）", ruleID)
		}
		operandType, err := validateExpr(ruleID, e.Operand, declaredCaptures)
		if err != nil {
			return "", err
		}
		if operandType != exprTypeNumber {
			return "", fmt.Errorf("规则 %q 的数值区间比较要求数值操作数，实际为%s操作数", ruleID, operandType)
		}
		if e.Min == nil && e.Max == nil {
			return "", fmt.Errorf("规则 %q 的数值区间比较缺少区间界（min / max 至少其一）", ruleID)
		}
		if e.Min != nil && e.Max != nil && *e.Min > *e.Max {
			return "", fmt.Errorf("规则 %q 的数值区间下界 %g 大于上界 %g", ruleID, *e.Min, *e.Max)
		}
		return exprTypeBool, nil
	case ExprAnd, ExprOr:
		if len(e.Subs) == 0 {
			return "", fmt.Errorf("规则 %q 的逻辑组合（%s）至少需要一个子表达式（subs）", ruleID, e.Kind)
		}
		for _, sub := range e.Subs {
			subType, err := validateExpr(ruleID, sub, declaredCaptures)
			if err != nil {
				return "", err
			}
			if subType != exprTypeBool {
				return "", fmt.Errorf("规则 %q 的逻辑组合（%s）的子表达式须为布尔，实际为%s", ruleID, e.Kind, subType)
			}
		}
		return exprTypeBool, nil
	case ExprNot:
		if e.Inner == nil {
			return "", fmt.Errorf("规则 %q 的逻辑非表达式缺少内层表达式（inner）", ruleID)
		}
		innerType, err := validateExpr(ruleID, e.Inner, declaredCaptures)
		if err != nil {
			return "", err
		}
		if innerType != exprTypeBool {
			return "", fmt.Errorf("规则 %q 的逻辑非的内层表达式须为布尔，实际为%s", ruleID, innerType)
		}
		return exprTypeBool, nil
	default:
		return "", fmt.Errorf("规则 %q 的条件表达式引用了未知的判别类别 %q", ruleID, e.Kind)
	}
}

// declaredPatternCaptures 收集模式树内会绑定进解析上下文的命名捕获组名：
// filename / relpath 目标的正则命中即绑定；LocationPattern 的 Ancestor 命中时
// 其命名捕获组同样绑入主上下文（ ancestor 捕获仅补缺，见 Evaluate），故一并
// 收集——条件与提取动作可引用。dirname 目标的正则只记目录层命中不绑定捕获组，
// 其捕获组声明不参与收集——条件引用这些组名会在构造期被存在性校验拒绝。
func declaredPatternCaptures(p Pattern) map[string]bool {
	out := map[string]bool{}
	collectPatternCaptures(p, out)
	return out
}

// collectPatternCaptures 递归收集会绑定进主解析上下文的捕获组名。
func collectPatternCaptures(p Pattern, out map[string]bool) {
	switch t := p.(type) {
	case nil:
	case *RegexPattern:
		if t.Target == TargetFileName || t.Target == TargetRelPath {
			for _, name := range t.Captures {
				out[name] = true
			}
		}
	case *LocationPattern:
		collectPatternCaptures(t.Ancestor, out)
	case And:
		for _, sub := range t {
			collectPatternCaptures(sub, out)
		}
	case *And:
		for _, sub := range *t {
			collectPatternCaptures(sub, out)
		}
	case Or:
		for _, sub := range t {
			collectPatternCaptures(sub, out)
		}
	case *Or:
		for _, sub := range *t {
			collectPatternCaptures(sub, out)
		}
	case Not:
		collectPatternCaptures(t.Inner, out)
	case *Not:
		collectPatternCaptures(t.Inner, out)
	}
}

// MarshalJSON 把档案序列化为带 kind 判别字段的多态 JSON。
func (p RuleProfile) MarshalJSON() ([]byte, error) {
	out := ruleProfileJSON{
		SchemaVersion: p.SchemaVersion, Rules: make([]ruleEntryJSON, 0, len(p.Rules)),
		DisabledBuiltinIDs: p.DisabledBuiltinIDs,
	}
	for _, rule := range p.Rules {
		rj, err := ruleToJSON(rule)
		if err != nil {
			return nil, err
		}
		out.Rules = append(out.Rules, *rj)
	}
	return json.Marshal(out)
}

// UnmarshalJSON 从带 kind 判别字段的多态 JSON 反序列化档案：逐条规则构造并
// 校验（正则编译、深度约束），任一规则非法即整体失败并返回中文错误。
func (p *RuleProfile) UnmarshalJSON(data []byte) error {
	var in ruleProfileJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("规则档案反序列化失败: %w", err)
	}
	profile := RuleProfile{
		SchemaVersion: in.SchemaVersion, Rules: make([]RuleEntry, 0, len(in.Rules)),
		DisabledBuiltinIDs: in.DisabledBuiltinIDs,
	}
	for i := range in.Rules {
		rule, err := ruleFromJSON(&in.Rules[i])
		if err != nil {
			return err
		}
		profile.Rules = append(profile.Rules, *rule)
	}
	*p = profile
	return nil
}

// ParseProfile 从 JSON 数据载入规则档案：反序列化 + 逐条构造校验。
// 数据不可解析、或结构版本低于已知历史版本时返回 nil 档案与单个错误；历史
// 版本（v1 无条件形态 / v2 谓词组合条件形态）兼容载入并在内存中升格为当前
// 版本（v2 条件映射为等价表达式树）；结构版本高于当前版本（未来版本）按尽力
// 解析载入——记一条版本告警错误（调用方落日志）、未知字段忽略、无法构造的
// 规则逐条跳过，不中断整体。个别规则构造校验失败（正则编译、深度约束、条件
// 表达式引用不存在等）只产生含规则 ID 与描述的中文错误并跳过该规则，不阻塞
// 其余规则载入；无错误时错误列表为空。
func ParseProfile(data []byte) (*RuleProfile, []error) {
	var in ruleProfileJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, []error{fmt.Errorf("规则档案反序列化失败: %w", err)}
	}
	var errs []error
	switch {
	case in.SchemaVersion > CurrentSchemaVersion:
		errs = append(errs, fmt.Errorf(
			"规则档案结构版本为 %d，高于当前支持的版本 %d，按尽力解析载入：未知字段将被忽略、无法解析的规则将被跳过，重写后落盘为版本 %d",
			in.SchemaVersion, CurrentSchemaVersion, CurrentSchemaVersion))
	case in.SchemaVersion != CurrentSchemaVersion && !legacySchemaVersions[in.SchemaVersion]:
		return nil, []error{fmt.Errorf("规则档案结构版本为 %d，与当前支持的版本 %d 不一致", in.SchemaVersion, CurrentSchemaVersion)}
	}
	profile := &RuleProfile{
		SchemaVersion: CurrentSchemaVersion, Rules: make([]RuleEntry, 0, len(in.Rules)),
		DisabledBuiltinIDs: in.DisabledBuiltinIDs,
	}
	for i := range in.Rules {
		rj := &in.Rules[i]
		rule, err := ruleFromJSON(rj)
		if err != nil {
			errs = append(errs, fmt.Errorf("跳过规则 %q（%s）: %w", rj.ID, rj.Description, err))
			continue
		}
		profile.Rules = append(profile.Rules, *rule)
	}
	return profile, errs
}
