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
	// FromRefCapture 表示从被引用成员规则的捕获值取值（组合规则专用）：被引
	// 成员规则由本规则 refs 声明、动作以 RefRule 指明成员——取值经限定键
	//「成员ID.rN.组名」注入，成员捕获私有作用域不变（语义见规则组合引用方案
	// 阶段3）。
	FromRefCapture ExtractionFrom = "refCapture"
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
	// RefRule 为 From 为 refCapture 时的被引成员规则 ID：须为本规则 refs 列表
	// 内的成员（组合规则专用；成员捕获以限定键「成员ID.rN.组名」注入，取值
	// 见规则组合引用方案阶段3）。空串在非 refCapture 动作上无意义，载入时
	// 对 refCapture 动作要求非空且属本条 refs。
	RefRule string `json:"refRule,omitempty"`
	// CaptureName 为 From 为 capture 时的捕获组名；v4 条件规则（Row>0）中为
	// 占位词的英文组名，实际绑定名为行号前缀形态 rN.组名（见 Row 字段）。
	CaptureName string `json:"captureName"`
	// Row 为 v4 条件行引用（1 起先序展平行号）：From 为 capture 时取该行的
	// 前缀绑定；库名单小标（形变归一）命中时优先取 rN.组名#canon 规范名绑定
	//（T1 的作者产出即库内规范名），无小标回落原始绑定。0 = 旧式裸组名
	//（机器构造的内部规则未迁移前的过渡形态）。
	Row int `json:"row,omitempty"`
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
		name := a.CaptureName
		captured, ok := "", false
		if a.Row > 0 {
			// 行引用优先取库名单小标的规范名绑定（形变归一命中时产出——
			// T1 的作者产出即库内规范名）；无小标时回落原始绑定。
			captured, ok = ctx.Capture(rowCaptureName(a.Row, a.CaptureName) + "#canon")
			if !ok {
				captured, ok = ctx.Capture(rowCaptureName(a.Row, a.CaptureName))
			}
		} else {
			// Row 缺省（0/负）按第 1 行兜底取绑定——与 validateExtractions
			// 的 rowPrefixed 口径一致（前端编辑面第 1 行捕获引用合法省略
			// row 字段；v4 求值上下文的捕获键恒为 rN. 前缀形态，裸名直取
			// 恒落空曾令编辑保存后的规则「命中但零声明」）。裸名终备兼容
			// 历史无前缀上下文。
			captured, ok = ctx.Capture(rowCaptureName(1, a.CaptureName) + "#canon")
			if !ok {
				captured, ok = ctx.Capture(rowCaptureName(1, a.CaptureName))
			}
			if !ok {
				captured, ok = ctx.Capture(name)
			}
		}
		if !ok {
			return MetadataDeclaration{}, false
		}
		value, source = captured, SourceFilename
	case FromRefCapture:
		// 被引规则捕获（组合规则专用）：取值序对齐 FromCapture——先库名单
		// 小标的规范名限定键（成员ID.rN.组名#canon）后原始限定键；Row 缺省
		//（0/负）按第 1 行兜底（validateExtractions 对 refCapture 不做行
		// 存在性核——成员声明在该管线时点不可得，求值侧兜底与 FromCapture
		// 分支同型）。限定键未注入/未绑定＝来源缺失，零产出不报错（保守
		// 取向）；值来自成员规则对文件名/目录名的解析捕获，产出声明来源与
		// FromCapture 同款 SourceFilename，归属组合规则自身 ID。
		row := a.Row
		if row <= 0 {
			row = 1
		}
		captured, ok := ctx.refCaptureValue(a.RefRule, rowCaptureName(row, a.CaptureName)+"#canon")
		if !ok {
			captured, ok = ctx.refCaptureValue(a.RefRule, rowCaptureName(row, a.CaptureName))
		}
		if !ok && a.Row <= 0 {
			// 裸绑定终备：Row 缺省形态兼容无行号前缀上下文的成员（pattern
			// 形态成员规则绑定裸组名），对齐 FromCapture 分支的裸名终备。
			captured, ok = ctx.refCaptureValue(a.RefRule, a.CaptureName)
		}
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
	// Refs 为被引用的成员规则 ID 列表（组合规则）：非空即组合规则——求值前提
	// 为全部成员规则命中，提取动作可经 refCapture 引用成员捕获值。空/缺省=
	// 普通规则（omitempty 保证旧档零迁移、JSON 往返等价；档案结构版本维持 4）。
	// 成员须在本档案规则集内、不得自引用、自身不得再携带 refs（一期禁止嵌套
	// 组合引用），违规条目载入时跳过并告警。
	//
	// 门语义边界：门为严格门——全部成员命中才放行，任一成员未命中即组合规则
	// 整体零产出、不进命中清单；成员停用、不在场、或属定群形态均按未命中。定群
	// 形态的成员（恰一行序列条件且无提取动作，只把成套文件聚成群卡、不产生文件
	// 级命中）在门段被跳过，组合引用它恒不命中。成员仍独立求值：自身提取动作
	// 照常执行并产出声明、命中照常计入自身命中清单；要让它只作引用条件，须清空
	// 其提取动作。
	Refs []string `json:"refs,omitempty"`
	// Conditions 为 v4 条件行列表（目标三分+格式+附加+小标）：v4 档案的
	// 持久化语法。机器规则生产方（问答/泛化/裁决）与档案装载同一来源；
	// Pattern 为其编译缓存。过渡期（生产方迁移完成为止）直接设置 Pattern
	// 的内部构造仍可求值，但不可落档。
	Conditions []CondxRow `json:"-"`
	// Pattern 为命中判定的模式树；Conditions 存在时为其编译缓存。
	Pattern Pattern `json:"pattern"`
	// Extractions 为命中后依序执行的提取动作。
	Extractions []ExtractionAction `json:"extractions"`
	// Synthesis 为该规则产出声明的合成策略。
	Synthesis SynthesisPolicy `json:"synthesis"`
}

// CurrentSchemaVersion 为规则档案当前的结构版本。
// v4：机器规则一律为条件行组合形态（CondxRow 树：目标主体〔文件/目录/序列〕
// +格式模板/包含词 +终结谓词与锚），生产方（泛化/裁决/问答/集规则）全部直出
// v4，内置规则退休为例子模板、不再参与求值。v1..v3 旧档停用：结构版本不是 4
// 的档案整体拒载（零迁移——旧机器规则对用户无保留价值，重置后由新一代生产
// 方重新生成）。
const CurrentSchemaVersion = 4

// RuleProfile 为规则档案：一批规则的容器。
type RuleProfile struct {
	// SchemaVersion 为档案结构版本，载入时校验（非 v4 档案整体拒载）。
	SchemaVersion int `json:"schemaVersion"`
	// Rules 为档案内的规则列表。
	Rules []RuleEntry `json:"rules"`
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

// ruleEntryJSON 为 RuleEntry 的序列化中间结构：v4 条目以 conditions（条件行
// 列表）为唯一语法（pattern 缺席）；pattern（模式树多态中间表示）为 v3 及
// 更早条目的旧形态，档案版本翻转为仅认 4 后载入侧即拒绝。
type ruleEntryJSON struct {
	ID          string             `json:"id"`
	Description string             `json:"description"`
	Source      RuleSource         `json:"source"`
	Enabled     bool               `json:"enabled"`
	Refs        []string           `json:"refs,omitempty"`
	Conditions  []CondxRow         `json:"conditions,omitempty"`
	Pattern     *patternJSON       `json:"pattern,omitempty"`
	Extractions []ExtractionAction `json:"extractions"`
	Synthesis   SynthesisPolicy    `json:"synthesis"`
}

// ruleProfileJSON 为 RuleProfile 的序列化中间结构。
type ruleProfileJSON struct {
	SchemaVersion int             `json:"schemaVersion"`
	Rules         []ruleEntryJSON `json:"rules"`
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
		RefRule     string            `json:"refRule"`
		CaptureName string            `json:"captureName"`
		Row         int               `json:"row,omitempty"`
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
		RefRule:     in.RefRule,
		CaptureName: in.CaptureName,
		Row:         in.Row,
		Constant:    in.Constant,
		ScanScope:   in.ScanScope,
		RoleName:    in.RoleName,
		StripPrefix: in.StripPrefix,
		StripSuffix: in.StripSuffix,
		When:        cond,
	}
	return nil
}

// ruleToJSON 把规则转为序列化中间结构：v4 条目（Conditions 在场）只落
// conditions——模式树是编译缓存不落档；两者皆缺（直接设置 Pattern 的内部
// 构造，如未迁移的调整面条目）不可落档，返回中文错误。
func ruleToJSON(rule RuleEntry) (*ruleEntryJSON, error) {
	if rule.Conditions != nil {
		return &ruleEntryJSON{
			ID:          rule.ID,
			Description: rule.Description,
			Source:      rule.Source,
			Enabled:     rule.Enabled,
			Refs:        rule.Refs,
			Conditions:  rule.Conditions,
			Extractions: rule.Extractions,
			Synthesis:   rule.Synthesis,
		}, nil
	}
	pj, err := patternToJSON(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("规则 %q 的模式序列化失败: %w", rule.ID, err)
	}
	return &ruleEntryJSON{
		ID:          rule.ID,
		Description: rule.Description,
		Source:      rule.Source,
		Enabled:     rule.Enabled,
		Refs:        rule.Refs,
		Pattern:     pj,
		Extractions: rule.Extractions,
		Synthesis:   rule.Synthesis,
	}, nil
}

// ruleFromJSON 把规则中间结构构造回 RuleEntry：conditions 形态（v4）编译为
// 模式树缓存并对提取动作做行引用存在性校验；pattern 形态（旧）走模式树
// 载入。两形态皆缺或构造校验失败返回中文错误。
func ruleFromJSON(rj *ruleEntryJSON) (*RuleEntry, error) {
	// 组合规则（refs 非空）的空条件表落档形态：omitempty 序列化把空数组写成
	// 无 conditions 键（与 pattern 缺席同形），构造时补回空表走同款处理——
	// 否则空条件组合「可存不可载」（2026-10-26 live-test 抓出的二次缺陷）。
	if rj.Conditions == nil && rj.Pattern == nil && len(rj.Refs) > 0 {
		rj.Conditions = []CondxRow{}
	}
	if rj.Conditions != nil {
		if rj.Pattern != nil {
			return nil, fmt.Errorf("规则 %q 同时携带 conditions 与 pattern（非法形态）", rj.ID)
		}
		// 组合规则（refs 非空）允许空条件表：命中前提全部由成员门段承担
		//（剧本一·帧6「引用 N 条成员规则；（无条件行）」，前端编辑面同口径
		// 放行），模式编译为恒真合取（And 零子式全真）；非组合规则空条件表
		// 仍拒（无条件行的普通规则=恒命中，违背 v4 语义）。行引用存在性
		// 校验照走（空声明集——组合的捕获引用只能走 refCapture 限定键）。
		if len(rj.Conditions) == 0 {
			if len(rj.Refs) == 0 {
				return nil, fmt.Errorf("规则 %q 的条件行非法: 条件行列表不能为空", rj.ID)
			}
			if err := validateExtractions(rj.ID, rj.Refs, And{}, rj.Extractions, map[string]bool{}, true); err != nil {
				return nil, err
			}
			return &RuleEntry{
				ID:          rj.ID,
				Description: rj.Description,
				Source:      rj.Source,
				Enabled:     rj.Enabled,
				Refs:        rj.Refs,
				Conditions:  rj.Conditions,
				Pattern:     And{},
				Extractions: rj.Extractions,
				Synthesis:   rj.Synthesis,
			}, nil
		}
		pat, err := CompileRuleConditions(rj.Conditions)
		if err != nil {
			return nil, fmt.Errorf("规则 %q 的条件行非法: %w", rj.ID, err)
		}
		if err := validateExtractions(rj.ID, rj.Refs, pat, rj.Extractions, declaredConditionCaptures(rj.Conditions), true); err != nil {
			return nil, err
		}
		return &RuleEntry{
			ID:          rj.ID,
			Description: rj.Description,
			Source:      rj.Source,
			Enabled:     rj.Enabled,
			Refs:        rj.Refs,
			Conditions:  rj.Conditions,
			Pattern:     pat,
			Extractions: rj.Extractions,
			Synthesis:   rj.Synthesis,
		}, nil
	}
	if rj.Pattern == nil {
		return nil, fmt.Errorf("规则 %q 缺少条件行（conditions）", rj.ID)
	}
	pat, err := patternFromJSON(rj.Pattern)
	if err != nil {
		return nil, fmt.Errorf("规则 %q 的模式非法: %w", rj.ID, err)
	}
	if err := validateExtractions(rj.ID, rj.Refs, pat, rj.Extractions, declaredPatternCaptures(pat), false); err != nil {
		return nil, err
	}
	return &RuleEntry{
		ID:          rj.ID,
		Description: rj.Description,
		Source:      rj.Source,
		Enabled:     rj.Enabled,
		Refs:        rj.Refs,
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

// validateExtractions 逐条校验提取动作的承载字段（构造期静态校验，与条件
// 编译同管线）：裁决取值缺常量、捕获引用不存在（v4 行引用指向未声明行/组名
// ——Row 缺省按第 1 行兜底，与 validateCaptureExtractionDeclared 同口径；
// 前端编辑面第 1 行省略 row 字段为合法形态）、被引规则捕获（refCapture）的
// refRule 缺失/不在本规则 refs 内或缺少 captureName、条件表达式引用不存在
// （谓词来源/维度、捕获组名不在声明集合内；限定键形操作数「成员ID.rN.组名」
// 按本条 refs 放行，见 validateExpr）或类型不匹配时返回含规则 ID 与动作序号
// 的中文错误。refs 为本条规则的被引成员 ID 列表（组合规则；普通
// 规则为空），declaredCaptures 为该规则形态（条件行或模式树）声明的捕获组
// 名集合（v4 为行号前缀形态，rowPrefixed=true）。
func validateExtractions(ruleID string, refs []string, pattern Pattern, actions []ExtractionAction, declaredCaptures map[string]bool, rowPrefixed bool) error {
	for i := range actions {
		action := &actions[i]
		if action.From == FromRefCapture {
			// 被引规则捕获（组合规则专用）：refRule 必填且须属本条 refs；
			// captureName 必填（对齐 FromCapture 的必填口径）。
			if action.RefRule == "" {
				return fmt.Errorf("规则 %q 的第 %d 条提取动作为被引规则捕获（refCapture）但缺少被引规则 ID（refRule）",
					ruleID, i+1)
			}
			if !containsString(refs, action.RefRule) {
				return fmt.Errorf("规则 %q 的第 %d 条提取动作引用的规则 %q 不在本规则的 refs 列表内",
					ruleID, i+1, action.RefRule)
			}
			if action.CaptureName == "" {
				return fmt.Errorf("规则 %q 的第 %d 条提取动作为被引规则捕获（refCapture）但缺少捕获组名（captureName）",
					ruleID, i+1)
			}
		}
		if action.From == FromCapture && action.CaptureName != "" {
			if rowPrefixed {
				// v4 条件行形态：Row 缺省（0/负）按第 1 行兜底——第 1 行
				// 引用省略 row 是前端编辑面的合法产物。
				row := action.Row
				if row <= 0 {
					row = 1
				}
				if !declaredCaptures[rowCaptureName(row, action.CaptureName)] &&
					!declaredCaptures[rowCaptureName(row, action.CaptureName)+"#canon"] {
					return fmt.Errorf("规则 %q 的第 %d 条提取动作引用了未声明的条件捕获（第 %d 行 %q）",
						ruleID, i+1, row, action.CaptureName)
				}
			} else if !declaredCaptures[action.CaptureName] {
				return fmt.Errorf("规则 %q 的第 %d 条提取动作引用了模式未声明的捕获组名 %q",
					ruleID, i+1, action.CaptureName)
			}
		}
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
		if _, err := validateExpr(ruleID, action.When, declaredCaptures, refs); err != nil {
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
// declaredCaptures 为规则模式声明的捕获组名集合；refs 为本条规则的被引成员
// ID 列表（组合规则）——限定键形捕获操作数「成员ID.rN.组名」（显式限定形，
// 可带 #canon 后缀）按其放行为字符串操作数（求值侧 conditionOperandValue 从
// 注入区取值，两侧共用 parseRefCaptureName 解析、口径一致），被引成员不在
// refs 内即拒绝。行/组在被引成员声明中的存在性不做核验——成员声明在本管线
// 时点不可得（逐规则构造先于跨规则汇集），求值侧未绑定即取值不可用、含该
// 操作数的比较为假（保守取向，与 refCapture 提取动作的形核口径一致）。
func validateExpr(ruleID string, e *CondExpr, declaredCaptures map[string]bool, refs []string) (exprValueType, error) {
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
		if memberID, _, ok := parseRefCaptureName(e.CaptureName); ok {
			// 限定键形操作数（组合规则 when 引用成员捕获，显式形）：成员在
			// 本条 refs 内即放行为字符串操作数；非组合规则（refs 空）恒拒绝
			//——限定名不是自有捕获的旁路（发现6：限定键为引用成员捕获值的
			// 唯一通道）。
			if !containsString(refs, memberID) {
				return "", fmt.Errorf("规则 %q 的条件表达式引用的成员规则 %q 不在本规则的 refs 列表内（限定名操作数「成员ID.rN.组名」只能引用被引成员的捕获）", ruleID, memberID)
			}
			return exprTypeString, nil
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
		leftType, err := validateExpr(ruleID, e.Left, declaredCaptures, refs)
		if err != nil {
			return "", err
		}
		rightType, err := validateExpr(ruleID, e.Right, declaredCaptures, refs)
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
		operandType, err := validateExpr(ruleID, e.Operand, declaredCaptures, refs)
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
			subType, err := validateExpr(ruleID, sub, declaredCaptures, refs)
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
		innerType, err := validateExpr(ruleID, e.Inner, declaredCaptures, refs)
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
// 结构版本不是当前版本（v4）的档案**整体拒载**——v1..v3 旧档已停用（机器
// 规则一律由新一代生产方直出 v4，旧机器规则对用户无保留价值）；拒载档
// 案经 LoadProfile 自动重置（原文备份+空档覆写+知会警告），用户无需手动
// 处置。结构版本高于当前版本（未来版本）按尽力解析载入——记一条版本告警
// 错误（调用方落日志）、未知字段忽略、无法构造的规则逐条跳过，不中断整
// 体。个别规则构造校验失败（条件行编译、正则、深度约束、提取引用不存在、
// refCapture 的 refRule 缺失或不在本条 refs 内等）只产生含规则 ID 与描述的
// 中文错误并跳过该规则，不阻塞其余规则载入；组合规则的跨规则引用校验（悬空
// / 自引用 / 嵌套组合引用）同挂该出口——逐规则构造完成后对全部已构造规则
// 统一校验，违规条目同样跳过并返回带规则 ID 的错误。无错误时错误列表为空。
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
	case in.SchemaVersion != CurrentSchemaVersion:
		// v1..v3 旧档整体停用：零迁移拒载（含可兼容升格的历史版本——机器
		// 规则对用户无保留价值，重置后由新一代生产方重新生成）。
		return nil, []error{fmt.Errorf(
			"规则档案结构版本为 %d（当前 %d），旧档案已停用",
			in.SchemaVersion, CurrentSchemaVersion)}
	}
	profile := &RuleProfile{
		SchemaVersion: CurrentSchemaVersion, Rules: make([]RuleEntry, 0, len(in.Rules)),
	}
	// 第一遍：逐规则构造（模式/条件行编译 + 提取动作校验，含 refCapture 的
	// refRule∈refs、captureName 必填）。失败条目跳过并告警。
	constructed := make([]RuleEntry, 0, len(in.Rules))
	for i := range in.Rules {
		rj := &in.Rules[i]
		rule, err := ruleFromJSON(rj)
		if err != nil {
			errs = append(errs, fmt.Errorf("跳过规则 %q（%s）: %w", rj.ID, rj.Description, err))
			continue
		}
		constructed = append(constructed, *rule)
	}
	// 第二遍：组合规则跨规则引用校验（悬空 / 自引用 / 嵌套组合引用）。以
	// 第一遍已构造规则集为「本档案规则集」——被逐条构造拒收的规则不在场，
	// 指向它的引用即按悬空处理。违规条目同走坏条目出口（跳过 + 带 ID 告警）。
	byID := make(map[string]int, len(constructed))
	for i := range constructed {
		if _, seen := byID[constructed[i].ID]; !seen {
			byID[constructed[i].ID] = i
		}
	}
	for i := range constructed {
		rule := &constructed[i]
		if len(rule.Refs) > 0 {
			if err := validateRuleRefs(rule, byID, constructed); err != nil {
				// 告警点名组合规则（描述优先，缺失退回 ID）：前端「档案载入告警」
				// 逐条呈现，剧本二·帧5 文案＝组合规则『<描述>』引用的成员规则
				// <成员ID> 不存在，已跳过。
				label := rule.Description
				if label == "" {
					label = rule.ID
				}
				errs = append(errs, fmt.Errorf("组合规则『%s』%w", label, err))
				continue
			}
		}
		profile.Rules = append(profile.Rules, *rule)
	}
	return profile, errs
}

// validateRuleRefs 校验组合规则的 refs 引用（构造期跨规则静态校验）：自引用、
// 悬空引用（被引成员 ID 不在本档案已构造规则集内）、嵌套组合引用（被引成员
// 自身携带 refs，一期禁止组合引用组合）三者任一违规即返回中文错误，调用方
// 跳过该条并告警。byID 为已构造规则 ID 到 all 下标的索引（重复 ID 取先出现
// 者）；all 供查看被引规则自身是否携带 refs。
func validateRuleRefs(rule *RuleEntry, byID map[string]int, all []RuleEntry) error {
	for _, ref := range rule.Refs {
		if ref == rule.ID {
			return fmt.Errorf("引用了自身（refs 含本条 ID %q）", ref)
		}
		idx, ok := byID[ref]
		if !ok {
			return fmt.Errorf("引用的成员规则 %s 不存在，已跳过", ref)
		}
		if len(all[idx].Refs) > 0 {
			return fmt.Errorf("引用的成员规则 %q 自身为组合规则（一期禁止嵌套组合引用）", ref)
		}
	}
	return nil
}

// containsString 判断字符串切片是否含有目标值。
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
