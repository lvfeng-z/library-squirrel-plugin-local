// 规则档案模型：含义维度 / 来源 / 提取 / 合成的类型定义，档案的多态 JSON 序列化
// 与载入校验，以及旧剥壳字段的载入迁移；纯模型，无 IO 与日志。
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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
	// DimDescription 表示作品描述维度（site_work_description；取值词 {描述}，
	// 创建期接线取首个声明）。
	DimDescription MetadataDimension = "description"
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
	// SourceContent 表示伴随说明内容：值来自识别轮按内容绑定命中集采集注入
	// 的说明文件内容片段整段。内容来源只经聚合规则的内容绑定面成立（元数据
	// 规则的提取动作携带内容取值时载入即拒）；声明在场判定（when 谓词）不
	// 消费该来源——条件引用按未知来源拒绝（knownSourceKinds 不收录）。
	SourceContent SourceKind = "content"
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
	// FromRefCapture 表示从供给方规则的捕获值取值（作用域附件消费规则专用）：
	// 供给方由本规则目录行的 refScope 附件声明、动作以 RefRule 指明——取值经
	// 限定键「供给方ID.rN.组名」注入，供给方捕获私有作用域不变。
	FromRefCapture ExtractionFrom = "refCapture"
	// FromContent 表示取伴随说明文件的内容片段整段（识别轮按聚合规则内容绑定
	// 的命中集采集注入的数据字段）。内容取值只经聚合规则的内容绑定面成立：
	// 元数据规则的提取动作携带该来源时载入即拒（内容绑定是聚合规则的窄产出），
	// 绑定行的整段取值不走提取动作分支（见 pipeline.go 的内容声明产出）。
	FromContent ExtractionFrom = "content"
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
	// RuleSourceAdjudication 表示冲突裁决来源（历史词汇：裁决流已改写会话赋值、
	// 不再产出规则；来源值保留供存量 v4 档条目载入解析）。
	RuleSourceAdjudication RuleSource = "adjudication"
	// RuleSourcePreset 表示预置种子规则：首次档案初始化写入的出厂条目，此后与
	// 用户条目同权（可见、可启停、可编辑、可复制），「恢复预置」入口兜底误删。
	RuleSourcePreset RuleSource = "preset"
)

// RuleCategory 为规则的两级综合类别：同一条件行机制（四种源）上的产出分野。
type RuleCategory string

const (
	// RuleCategoryMetadata 表示元数据规则：条件行+元数据产出（既有「条件+提取
	// 动作」路径的综合命名，求值零改动）。缺省值——旧档与既有条目不带类别
	// 字段即属此类，零迁移。
	RuleCategoryMetadata RuleCategory = "metadata"
	// RuleCategoryAggregate 表示聚合规则：条件行（四种源任选，模板可带捕获；
	// 布尔自由——括号子组/行级非放开，决策10）+作品标识声明+分轨入口（聚合成
	// 作品产出）。条件行照常进逐文件求值循环（可携提取动作），并组判定挂汇流
	// 后的聚合段（见 pipeline.go）。
	RuleCategoryAggregate RuleCategory = "aggregate"
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
	// AlsoRuleIDs 为与本声明同值并入的其他规则 ID（按并入先后保序；
	// 并入只补记出处，本声明的维度、值、来源与角色不受影响）。
	AlsoRuleIDs []string `json:"alsoRuleIds,omitempty"`
	// Truncated 标记声明值取自超采集上限被截断的伴随说明内容片段（来源=内容
	// 的声明专属）：内容不完整的事实随声明携带，供来源呈现标注。
	Truncated bool `json:"truncated,omitempty"`
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
	// RefRule 为 From 为 refCapture 时的供给方规则 ID：须为本规则条件行
	// refScope 附件引用的供给方（作用域附件消费规则专用；供给方捕获以限定键
	// 「供给方ID.rN.组名」注入）。空串在非 refCapture 动作上无意义，载入时
	// 对 refCapture 动作要求非空且属本规则供给方集。
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
	// legacyStripPrefix / legacyStripSuffix 为旧档案剥壳前后缀（stripPrefix /
	// stripSuffix 键）的载入暂存（迁移检测位）：反序列化镜像读入非空旧键时
	// 填充，供档案载入路径的迁移器检测与处置（等价改写为字面括号模板或
	// 丢弃+描述注记）；不参与求值与序列化，处置后清零。
	legacyStripPrefix string
	legacyStripSuffix string
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
		// 按名取全部绑定（重复段同名多条=扇出面），Apply 单返回签名维持末值
		// 语义（既有调用面零扰动）；逐条产声明由 ApplyDeclarations 承载。
		values := a.captureValues(ctx)
		if len(values) == 0 {
			return MetadataDeclaration{}, false
		}
		value, source = values[len(values)-1], SourceFilename
	case FromRefCapture:
		// 供给方捕获（作用域附件消费规则专用）：取值序对齐 FromCapture——先
		// 库名单小标的规范名限定键（供给方ID.rN.组名#canon）后原始限定键；
		// Row 缺省（0/负）按第 1 行兜底（validateExtractions 对 refCapture
		// 不做行存在性核——供给方声明在该管线时点不可得，求值侧兜底与
		// FromCapture 分支同型）。限定键未注入/未绑定＝来源缺失，零产出不
		// 报错（保守取向）；值来自供给方规则对文件名/目录名的解析捕获，
		// 产出声明来源与 FromCapture 同款 SourceFilename，归属消费规则自身 ID。
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
	return MetadataDeclaration{
		Dimension: a.Dimension,
		Value:     value,
		Source:    source,
		RoleName:  a.RoleName,
		RuleID:    ruleID,
	}, true
}

// captureValues 解析 FromCapture 的候选取值面（#canon 回退序维持）：行号兜底
// 口径与既有 Apply 分支一致——Row>0 用该行前缀、Row 缺省（0/负）按第 1 行
// 兜底、裸名终备兼容历史无前缀上下文。库名单小标的规范名绑定（#canon，形变
// 归一命中时产出）优先且单值；无 #canon 时按名取全部原始绑定（重复段同名
// 多条=扇出面，按绑定序）。零绑定=来源缺失。
func (a ExtractionAction) captureValues(ctx *ParseContext) []string {
	row := a.Row
	if row <= 0 {
		row = 1
	}
	if v, ok := ctx.Capture(rowCaptureName(row, a.CaptureName) + "#canon"); ok {
		return []string{v}
	}
	var values []string
	for _, binding := range ctx.CaptureAll(rowCaptureName(row, a.CaptureName)) {
		values = append(values, binding.Value)
	}
	if len(values) == 0 && a.Row <= 0 {
		for _, binding := range ctx.CaptureAll(a.CaptureName) {
			values = append(values, binding.Value)
		}
	}
	return values
}

// ApplyDeclarations 执行提取动作并返回全部产出声明（Apply 的多值扇出面）：
// FromCapture 来源按 captureValues 逐条产声明——普通词词法保证一名一绑定、
// 恒单声明零扰动；重复段同名 N 绑定 → N 条声明（值序=绑定序）。#canon 命中
// 时单值单声明（规范名不扇出）。其余来源语义与 Apply 完全一致（单声明），
// 来源缺失/ctx 为 nil 返回 nil 零报错。既有单返回调用面继续用 Apply（维持
// 末值语义），扇出消费面（提取段逐动作产多声明）用本方法。
func (a ExtractionAction) ApplyDeclarations(ruleID string, ctx *ParseContext) []MetadataDeclaration {
	if ctx == nil {
		return nil
	}
	if a.From != FromCapture {
		if decl, ok := a.Apply(ruleID, ctx); ok {
			return []MetadataDeclaration{decl}
		}
		return nil
	}
	values := a.captureValues(ctx)
	if len(values) == 0 {
		return nil
	}
	out := make([]MetadataDeclaration, 0, len(values))
	for _, v := range values {
		out = append(out, MetadataDeclaration{
			Dimension: a.Dimension,
			Value:     v,
			Source:    SourceFilename,
			RoleName:  a.RoleName,
			RuleID:    ruleID,
		})
	}
	return out
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

// IdentityPart 为作品标识声明的一个部分：选自「条件行号·捕获名」——文件名捕获
// 与目录名捕获按行内命名捕获组取值（目录捕获被选=同目录文件共享该值，目录
// 成组）；序列群键用保留名 identityGroupKeyCapture（非序号捕获按序拼接，每
// 序列组一作品，聚合规则该行须为序列目标）。多个部分按声明序拼成复合标识。
type IdentityPart struct {
	// Row 为条件行号（1 起先序展平——括号子组内层行照常占号，组行占组内首
	// 行号；纯平行行的展平行号即顶层序号）。
	Row int `json:"row"`
	// Capture 为捕获名（英文组名，如 {作品}→work）；保留名 groupKey=取该行
	// 序列求值写入的群组键。
	Capture string `json:"capture"`
}

// TrackAssign 为分轨入口的单行角色分配：条件行→结构角色。
type TrackAssign struct {
	// Row 为条件行号（1 起先序展平，与 IdentityPart.Row 同口径）。序列行=集合
	// 轨（整序列组=N 存储位）；目录行可作标识源或集合轨（子树命中文件共享同
	// 一角色）。
	Row int `json:"row"`
	// Role 为结构角色（如 video 的 videoMain、subtitle；article 的 document
	// 与内嵌 image）：必含轨所在行（角色在类型规约中 Min≥1）承载聚合作品的
	// 主导文件，角色选项按所选类型规约过滤。
	Role string `json:"role"`
}

// 处置维度取值：伴随轨行处置的规则级历史两值。处置语义由行级指派合成表达
// ——分轨入口指派角色=轨道行（导入为资源）；内容绑定勾选=内容产出元数据；
// 两者兼有=双重身份（轨道行照常导入+内容来源，导入身份不受绑定影响）；仅
// 绑定未分轨=伴随说明文件处置（归进作品、无轨、不导入、不独立成作品）；两
// 者皆无=不参与成员判定。规则级字段仅为旧档兼容归一保留（空串归一 track）。
const (
	// CompanionUseTrack 表示轨道承载：随作品导入成为资源的轨（规则级唯一可写
	// 值，空串载入归一为此值）。
	CompanionUseTrack = "track"
	// CompanionUseMetadata 表示纯元数据承载：该处置由行级指派合成表达（仅被
	// 内容绑定勾选且未分配轨道的行）；规则级此值不再使用——档案载入校验拒绝
	// 并指路行级指派。
	CompanionUseMetadata = "metadata"
)

// ContentBinding 为「元数据文件」产出的一条内容绑定（行→维度指派）：把本
// 规则一条条件行命中文件的伴随说明内容整段声明到某含义维度。绑定行可为轨
// 行（照常导入，内容叠加产出——双重身份）或说明行（仅绑定行命中=元数据源
// 文件，不导入——内容按分组绑定扇出给组内全部作品）；一期零参数（整段→
// 单维度），「单文件解析逻辑→多维度」为该结构的参数化扩展位、本批不设计；
// 一行内容可整段同时映射多个维度（多目标=同行多条绑定），同一行同一维度
// 唯一。
type ContentBinding struct {
	// Row 为被指派的条件行号（1 起先序展平，须在本规则条件行范围内——括号
	// 子组内层行照常占号）。
	Row int `json:"row"`
	// Dimension 为内容整段声明到的含义维度。
	Dimension MetadataDimension `json:"dimension"`
}

// GroupingMode 为「元数据文件」产出分组绑定的键形态。
type GroupingMode string

const (
	// GroupingModeIdentity 表示标识分组：分组键=若干「行·捕获名」部分按声明
	// 序拼成的复合标识（序列群键保留名可用），捕获值相等（共享捕获跨行汇流）
	// 的文件归为一组——说明文件与资源文件各自从自己的命中行取同名捕获
	// （如 无题_01.txt↔无题_01.png 经同名 {作品标识} 同键入组）。
	GroupingModeIdentity GroupingMode = "identity"
	// GroupingModeDirectory 表示所在目录分组：分组键=文件所在目录——同目录
	// 的文件归为一组，说明内容扇出给组内全部作品（组内无其他文件时说明
	// 文件按独立作品导入——孤文件回退）。
	GroupingModeDirectory GroupingMode = "directory"
)

// GroupingBinding 为产出的分组绑定声明：产出声明自己消费哪个分组。「元数据
// 文件」产出、「作品集聚合」条目与「聚合成作品」产出的目录分组共用本结构，
// 一个绑定指向一个分组项——所在目录分组以产生行（Row，目录条件行）表达，
// 标识分组以键部分表达；分组键求值与聚合标识共用同一实现（统一分组求值器，
// 见 aggregation.go 的 groupingMembersOf 与 aggFileRowHits.identityValueWithSource）。
//
// 取值源约定：标识分组的序列群键保留名（groupKey）只读行内序列行群产物——
// 本结构（「元数据文件」产出的分组绑定）不启库级取值源，行内无群产物的文件
// 不入组语义保持不变。产出四「作品集聚合」的组键绑定复用本结构形态，但其
// 求值由产出形态授权库级取值源（行内优先、库级群卡键兜底）；取值源由消费
// 产出形态静态决定，不在本声明上落档。
type GroupingBinding struct {
	// Mode 为分组键形态（identity=标识分组 | directory=所在目录分组）。
	Mode GroupingMode `json:"mode"`
	// Row 为所在目录分组绑定的产生行行号（口径同 IdentityPart.Row：1 起先序
	// 展平，括号子组内层行照常占号）：必填，须指向本规则的一条目录条件行——
	// 该分组的键集只由这一行命中文件的所在目录构成，集名也只取这一行的捕获。
	// 标识分组不使用本字段（不落档）：单序列群键部分的产生行即该部分的行
	// 号，手写拼接的产生行由键部分决定。
	Row int `json:"row,omitempty"`
	// Identity 为标识分组的键部分（形态同聚合的标识声明：行号+捕获名，序列
	// 群键保留名可用）；所在目录分组不携带（在场即载入拒绝）。
	Identity []IdentityPart `json:"identity,omitempty"`
}

// MetadataFileProduction 为「元数据文件」产出的独立落档位（规则级产出、与
// 聚合成作品产出平级，可各自单独使用或同规则并用）：内容绑定列表（哪些条
// 件行的文件内容→哪个元数据维度）+ 分组绑定声明（说明内容按哪个分组内的
// 作品集扇出）。绑定行命中文件的内容声明扇出给分组内全部作品（各自成件=
// 各带一份、被聚合成一件=那件带一份；轨行兼绑定的双重身份文件内容同样
// 扇出）；仅绑定行命中的未收编文件=元数据源文件（组内有落点时成立：不导入、
// 不独立成件），组内无落点（全为绑定行命中或分组键不可得）按独立作品导入
// （孤文件回退）。并用（有聚合产出）而分组绑定缺省时，求值期缺省绑聚合成
// 作品所绑分组（便利缺省，显式可换——本结构不落档物化）。
type MetadataFileProduction struct {
	// Grouping 为分组绑定声明：映射单用（本规则无聚合产出）时必填——说明
	// 内容无分组可依即无落点；并用（有聚合产出）时可缺省（缺省=聚合成作品
	// 所绑分组，求值期取用、载入不物化）。
	Grouping *GroupingBinding `json:"grouping,omitempty"`
	// ContentBindings 为内容绑定列表（行→维度，一行可多条目=多目标）：绑定
	// 行命中文件的伴随说明内容产声明。空=无内容产出（该形态载入拒绝）。
	ContentBindings []ContentBinding `json:"contentBindings,omitempty"`
	// legacyPairing 为旧档配对键（metadataFile.pairing）的载入暂存位：分组
	// 绑定改以 metadataFile.grouping 落档，旧档读入即归一（键位迁移，见
	// normalizeLegacyPairingKey），不参与序列化（非导出字段）。
	legacyPairing *GroupingBinding
}

// UnmarshalJSON 从 JSON 载入「元数据文件」产出：字段直读之外，另把旧档的
// 配对键（pairing）读入载入暂存位，供条目构造路径归一到分组绑定落档位
// （载入即归一、不拒收——两形态 JSON 同构，归一只是键位迁移）；序列化方向
// 按结构体标签展开，暂存位非导出、不再随档落旧键。
func (m *MetadataFileProduction) UnmarshalJSON(data []byte) error {
	type metadataFileJSON struct {
		Grouping        *GroupingBinding `json:"grouping,omitempty"`
		Pairing         *GroupingBinding `json:"pairing,omitempty"`
		ContentBindings []ContentBinding `json:"contentBindings,omitempty"`
	}
	var in metadataFileJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("元数据文件产出反序列化失败: %w", err)
	}
	m.Grouping = in.Grouping
	m.legacyPairing = in.Pairing
	m.ContentBindings = in.ContentBindings
	return nil
}

// NameExtraction 为「作品集聚合」条目的集名来源声明：集名取自本规则条件行
// （匹配区）里的命名捕获——行号+捕获名（行号口径同 IdentityPart：1 起先序
// 展平，括号子组内层行照常占号、组行占组内首行号），该行的捕获值即本条归集
// 出的作品集的集名。集名的唯一来源就是本声明（决策7 甲案）：不再从提取动作
// 的作品集维度取、目录含义也不产集名。
type NameExtraction struct {
	// Row 为条件行号（1 起先序展平——括号子组内层行照常占号，组行占组内首行号）。
	Row int `json:"row"`
	// Capture 为捕获名（英文组名，如 {集名}→setname），须在该行声明。
	Capture string `json:"capture"`
}

// WorkSetAggregationProduction 为「作品集聚合」产出的单个条目（规则级产出，与
// 聚合产出、「元数据文件」产出、提取声明并列，可各自单独使用或同规则并用，
// 与三者零互斥）：一条 = 一个分组绑定 + 一个集名来源——命中文件各自成作品、
// 按 Grouping 声明的组键归入同一作品集（同键成组；无分组键的文件不入集、独立
// 成作品），集名取自 NameExtraction 指到的条件行捕获。规则可携带多条：多条
// 各自绑一个分组、各取一个集名；分组与聚合条目 1:1——同一分组不得被两个
// 条目选中，也不得与「聚合成作品」的分组绑定（标识声明 identity）相同。
//
// 落档形态：档案只落条目数组（workSetAggregation: [{…}]）；旧形态（规则级单条
// 对象 {…}）不做兼容——读到即按明确错误拒收（须为条目数组）。
type WorkSetAggregationProduction struct {
	// Grouping 为归集依据（组键绑定，形态复用 GroupingBinding）：identity=行捕获
	// 复合（序列群键保留名可用）、directory=目录分组（产生行为 Row 指向的目录
	// 条件行）。必填——无归集依据即无成组语义（载入/保存拒绝）。组键取值源由
	// 本产出形态静态授权库级来源：序列群键行内群产物优先、库级群卡键兜底
	// （取值源以 groupKeySource 入参随求值传入，兜底分支见 aggregation.go 的
	// identityPart）。
	Grouping *GroupingBinding `json:"grouping,omitempty"`
	// NameExtraction 为集名来源（行号+捕获名，指本规则条件行的命名捕获）：
	// 必填——取不到集名的条目无集名可声明（载入跳过+告警、编辑保存被拦）。
	// 集名只能取自所绑分组的产生行：所在目录分组与单序列群键分组要求行号
	// 等于产生行，手写拼接分组要求（行号·捕获名）是该分组键的一个部分——
	// 键可得则集名必可得。
	NameExtraction *NameExtraction `json:"nameExtraction,omitempty"`
}

// workSetAggregationField 为规则级「作品集聚合」条目列表的落档中间形态：档案只
// 认条目数组（workSetAggregation: [{…}]）——单条对象等旧形态不做兼容，读到即
// 拒收。序列化恒写数组。
type workSetAggregationField struct {
	// Entries 为条目列表。
	Entries []WorkSetAggregationProduction
}

// UnmarshalJSON 只接受条目数组：null 视为无条目（字段缺席同义）；单条对象
// （{…}）不做兼容转换，返回明确错误。
func (f *workSetAggregationField) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if trimmed[0] == '{' {
		return fmt.Errorf("工作集聚合须为条目数组（workSetAggregation: [{…}]）：不支持单条对象旧形态")
	}
	var list []WorkSetAggregationProduction
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return fmt.Errorf("作品集聚合产出反序列化失败: %w", err)
	}
	f.Entries = list
	return nil
}

// MarshalJSON 恒按条目数组落档。
func (f workSetAggregationField) MarshalJSON() ([]byte, error) {
	if f.Entries == nil {
		return []byte("null"), nil
	}
	return json.Marshal(f.Entries)
}

// workSetAggregationJSON 把内存条目列表转为落档中间形态：无条目即无该产出
// （nil，随档省略）。
func workSetAggregationJSON(entries []WorkSetAggregationProduction) *workSetAggregationField {
	if len(entries) == 0 {
		return nil
	}
	return &workSetAggregationField{Entries: entries}
}

// workSetAggregationEntries 把落档中间形态还原为内存条目列表：字段缺席或空
// 列表=nil（无该产出）。
func workSetAggregationEntries(f *workSetAggregationField) []WorkSetAggregationProduction {
	if f == nil || len(f.Entries) == 0 {
		return nil
	}
	return f.Entries
}

// AggregateProduction 为聚合规则的分轨入口（聚合成作品产出）：类型先行——
// 作品类型选自运行时资源类型注册表投影（内置 6 类+插件自定义类型），轨行→
// 角色分配按所选类型的轨规约过滤。
type AggregateProduction struct {
	// WorkType 为作品类型（资源类型值；引用的类型不在册=悬空，档案载入停用
	// 该条目并告警、识别求值跳过）。
	WorkType string `json:"workType"`
	// Grouping 为「聚合成作品」的分组绑定（可选）：在场时即本规则作品的分组
	// 依据——作品按该分组项归并（所在目录分组的产生行须为目录条件行）；与规则
	// 级作品标识声明（identity）互斥——两种分组绑定只能选一种，同时在场载入
	// 拒绝；缺席时作品键=作品标识声明的复合值。
	Grouping *GroupingBinding `json:"grouping,omitempty"`
	// Tracks 为各轨行的角色分配。
	Tracks []TrackAssign `json:"tracks"`
	// CompanionUse 为伴随轨行的处置维度（track|metadata；空串按 track 载入）。
	// 处置语义由行级指派合成表达（分轨入口+内容绑定），本字段仅为旧档兼容
	// 归一（metadata 载入即拒并指路行级指派）。
	CompanionUse string `json:"companionUse,omitempty"`
	// legacyContentBindings 为旧档绑定键（aggregate.contentBindings）的载入
	// 暂存位：绑定已迁至规则级「元数据文件」产出落档，旧档载入归一时上缴
	//（见 normalizeLegacyAggregateBindings），不参与序列化（非导出字段）。
	legacyContentBindings []ContentBinding
}

// UnmarshalJSON 从 JSON 载入聚合产出：字段直读之外，另把旧档的绑定键
// （contentBindings）读入载入暂存位，供条目构造路径归一到规则级落档位
// （零迁移零拒绝——载入即归一，不拒收）；序列化方向按结构体标签展开，
// 暂存位非导出、不再随聚合产出落档。字段逐个手工列举——聚合产出新增落档
// 字段时必须同步列入 aggregateJSON，否则该字段能保存、载入被静默丢弃。
func (a *AggregateProduction) UnmarshalJSON(data []byte) error {
	type aggregateJSON struct {
		WorkType        string           `json:"workType"`
		Grouping        *GroupingBinding `json:"grouping,omitempty"`
		Tracks          []TrackAssign    `json:"tracks"`
		CompanionUse    string           `json:"companionUse,omitempty"`
		ContentBindings []ContentBinding `json:"contentBindings,omitempty"`
	}
	var in aggregateJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("聚合产出反序列化失败: %w", err)
	}
	a.WorkType = in.WorkType
	a.Grouping = in.Grouping
	a.Tracks = in.Tracks
	a.CompanionUse = in.CompanionUse
	a.legacyContentBindings = in.ContentBindings
	return nil
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
	// Category 为规则的两级综合类别（metadata|aggregate）：空串即 metadata
	//（缺省类别，序列化省略——旧档与既有条目零迁移）。聚合规则的产出为聚合
	// 归属（无提取动作），求值挂汇流后的聚合段。
	Category RuleCategory `json:"category,omitempty"`
	// Identity 为作品标识声明（聚合规则专属）：标识的每一部分选自「轨行号·
	// 捕获名」。元数据规则不得携带（载入拒绝）；与聚合产出的目录分组绑定
	// （Aggregate.Grouping）互斥——两种分组绑定只能选一种。
	Identity []IdentityPart `json:"identity,omitempty"`
	// Aggregate 为分轨入口（聚合规则专属）：类型先行+轨行角色分配+处置维度，
	// 另可携带目录分组绑定（Grouping）。
	// 元数据规则不得携带（载入拒绝）。
	Aggregate *AggregateProduction `json:"aggregate,omitempty"`
	// MetadataFile 为「元数据文件」产出（规则级独立落档位，与聚合产出平级、
	// 各自可单独使用或并用）：内容绑定+分组绑定声明。元数据规则与聚合规则均可
	// 携带（映射单用=无聚合产出时内容按分组绑定扇出给组内全部作品、扇出
	// 不并作品；并用=分组绑定可独立于聚合标识声明，缺省=聚合成作品所绑分组
	//〔求值期取用，档案不物化〕）。
	MetadataFile *MetadataFileProduction `json:"metadataFile,omitempty"`
	// WorkSetAggregation 为「作品集聚合」产出的条目列表（规则级独立落档位，
	// 与聚合产出、「元数据文件」产出、提取声明并列、并用零互斥）：规则命中的
	// 文件各自成作品并按各条目的组键归入作品集。一条 = 分组绑定 + 集名来源
	// （NameExtraction，指条件行的命名捕获——集名的唯一来源）；条目间与
	// 「聚合成作品」的分组绑定 1:1。档案只落条目数组；旧形态（单条对象）不做
	// 兼容，载入即拒。
	WorkSetAggregation []WorkSetAggregationProduction `json:"workSetAggregation,omitempty"`
	// Conditions 为 v4 条件行列表（目标三分+格式+附加+小标）：v4 档案的
	// 持久化语法。机器规则生产方（问答/泛化）与档案装载同一来源；
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
// v5：目录条件行的多段模板与作用域附件（refScope）词汇落地（段界 `/`·`//`·
// `/{N}`、目录行作用域附件）；规则级引用（refs/组合规则）退役——v4 档内 refs
// 非空条目读入跳过+告警，「位于第 N 级」词条（dirLevel）随多段模板落地废弃，
// 携带该词条的条目按未知词条先例载入即拒。v4 档案读入保留（升级用户自身存量
// 的读入迁移）：读入照常、保存归一 v5；v1..v3 旧档停用：结构版本双读集外的
// 档案整体拒载（零迁移——旧机器规则对用户无保留价值，重置后由新一代生产
// 方重新生成）。
const CurrentSchemaVersion = 5

// RuleProfile 为规则档案：一批规则的容器。
type RuleProfile struct {
	// SchemaVersion 为档案结构版本，载入时校验（v4/v5 双读，其余整体拒载）。
	SchemaVersion int `json:"schemaVersion"`
	// Rules 为档案内的规则列表。
	Rules []RuleEntry `json:"rules"`
	// preservedSkipRaw 为载入跳过条目的原文字节（逐条 json.RawMessage）——写档
	// 保真通道（写档往返不物理丢弃跳过条目）：跳过条目不回填 Rules（不复活、
	// 不参与求值），凭此字段随档案内存对象存续，SaveProfile 时原样回写档案，
	// 后续载入照常跳过+告警，直到用户显式删除（原文层手术）或以同 ID 重写该
	// 条目（写档侧顶替退场）。仅载入路径填充；MarshalJSON 不含（序列化形态
	// 由 SaveProfile 的合并步骤决定）。
	preservedSkipRaw []json.RawMessage
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

// ruleEntryJSON 为 RuleEntry 的序列化中间结构：v4 起条目以 conditions（条件行
// 列表）为唯一语法（pattern 缺席）；pattern（模式树多态中间表示）为 v3 及
// 更早条目的旧形态，档案版本翻转为仅认 v4+ 后载入侧即拒绝。
type ruleEntryJSON struct {
	ID           string                  `json:"id"`
	Description  string                  `json:"description"`
	Source       RuleSource              `json:"source"`
	Enabled      bool                    `json:"enabled"`
	Category     RuleCategory            `json:"category,omitempty"`
	Identity     []IdentityPart          `json:"identity,omitempty"`
	Aggregate    *AggregateProduction    `json:"aggregate,omitempty"`
	MetadataFile *MetadataFileProduction `json:"metadataFile,omitempty"`
	// WorkSetAggregation 为「作品集聚合」产出条目列表的落档位（与既有三产出
	// 并列、并用零互斥）。档案只落条目数组；旧形态（单条对象）不做兼容。
	WorkSetAggregation *workSetAggregationField `json:"workSetAggregation,omitempty"`
	Conditions         []CondxRow               `json:"conditions,omitempty"`
	Pattern            *patternJSON             `json:"pattern,omitempty"`
	Extractions        []ExtractionAction       `json:"extractions"`
	Synthesis          SynthesisPolicy          `json:"synthesis"`
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
// 已废止的旧剥壳键（stripPrefix / stripSuffix）仍读入载入暂存（迁移检测位，
// 供档案载入路径的迁移器处置），不进求值面；序列化方向直接按结构体标签
// 展开，无自定义 MarshalJSON（旧键随之不再落档）。
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
		When:        cond,
	}
	a.legacyStripPrefix = in.StripPrefix
	a.legacyStripSuffix = in.StripSuffix
	return nil
}

// ruleToJSON 把规则转为序列化中间结构：v4 条目（Conditions 在场）只落
// conditions——模式树是编译缓存不落档；两者皆缺（直接设置 Pattern 的内部
// 构造，如未迁移的调整面条目）不可落档，返回中文错误。
func ruleToJSON(rule RuleEntry) (*ruleEntryJSON, error) {
	if rule.Conditions != nil {
		return &ruleEntryJSON{
			ID:                 rule.ID,
			Description:        rule.Description,
			Source:             rule.Source,
			Enabled:            rule.Enabled,
			Category:           rule.Category,
			Identity:           rule.Identity,
			Aggregate:          rule.Aggregate,
			MetadataFile:       rule.MetadataFile,
			WorkSetAggregation: workSetAggregationJSON(rule.WorkSetAggregation),
			Conditions:         rule.Conditions,
			Extractions:        rule.Extractions,
			Synthesis:          rule.Synthesis,
		}, nil
	}
	pj, err := patternToJSON(rule.Pattern)
	if err != nil {
		return nil, fmt.Errorf("规则 %q 的模式序列化失败: %w", rule.ID, err)
	}
	return &ruleEntryJSON{
		ID:                 rule.ID,
		Description:        rule.Description,
		Source:             rule.Source,
		Enabled:            rule.Enabled,
		Category:           rule.Category,
		Identity:           rule.Identity,
		Aggregate:          rule.Aggregate,
		MetadataFile:       rule.MetadataFile,
		WorkSetAggregation: workSetAggregationJSON(rule.WorkSetAggregation),
		Pattern:            pj,
		Extractions:        rule.Extractions,
		Synthesis:          rule.Synthesis,
	}, nil
}

// ruleFromJSON 把规则中间结构构造回 RuleEntry（严格形态：规则编辑保存、命中
// 预览与直接反序列化入口）：conditions 形态（v4）编译为模式树缓存并对提取动作
// 做行引用存在性校验；pattern 形态（旧）走模式树载入。两形态皆缺或构造校验
// 失败返回中文错误。头部先做声明准备（类别归一、旧档归一、产出声明校验，见
// prepareRuleEntryJSON）。所在目录分组绑定的载入期补行号与「集名来源不在产生
// 行」的条目级跳过只在载入形态（ruleFromJSONOnLoad）发生——本入口对这两种
// 形态按校验错误拒绝。
func ruleFromJSON(rj *ruleEntryJSON) (*RuleEntry, error) {
	if _, err := prepareRuleEntryJSON(rj, false); err != nil {
		return nil, err
	}
	return compileRuleEntry(rj)
}

// ruleFromJSONOnLoad 把规则中间结构构造回 RuleEntry（载入形态：档案载入的
// 逐条构造）：声明准备期执行只发生在载入期的归一，归一中补行号的分组绑定与
// 被跳过的产出/条目以告警返回（notes）。构造失败时 notes 无意义（该规则整体
// 跳过，由调用方按跳过告警呈现）。
func ruleFromJSONOnLoad(rj *ruleEntryJSON) (*RuleEntry, []error, error) {
	notes, err := prepareRuleEntryJSON(rj, true)
	if err != nil {
		return nil, nil, err
	}
	rule, err := compileRuleEntry(rj)
	if err != nil {
		return nil, nil, err
	}
	return rule, notes, nil
}

// prepareRuleEntryJSON 规则构造的声明准备：类别校验（两级类别归一：空串即
// metadata）、旧档归一（聚合产出内的绑定键与元数据文件旧配对键上缴到现行
// 落档位）、聚合声明/元数据文件/作品集聚合三种产出的载入校验。loading 为真
// 即载入形态：在旧档归一之后、产出校验之前执行只在载入期发生的归一——所在
// 目录分组绑定未指定产生行的补行号或跳过（normalizeLegacyDirectoryGroupings）、
// 集名来源不在所绑分组产生行的作品集聚合条目跳过
// （dropWorkSetEntriesWithUnreachableSetName）——并返回其告警；loading 为假
// （严格形态）不执行这两种归一，同形态由产出校验拒绝。通过后 rj 即合法的
// 规则中间结构，交 compileRuleEntry 编译。
func prepareRuleEntryJSON(rj *ruleEntryJSON, loading bool) ([]error, error) {
	// 类别归一不物化：空串即 metadata（缺省类别），序列化侧 omitempty 省略
	// ——旧档与既有条目零迁移、往返字节等价不变。
	if rj.Category != "" && rj.Category != RuleCategoryMetadata && rj.Category != RuleCategoryAggregate {
		return nil, fmt.Errorf("规则 %q 的类别非法: %q（metadata / aggregate）", rj.ID, string(rj.Category))
	}
	if rj.Category != RuleCategoryAggregate && (len(rj.Identity) > 0 || rj.Aggregate != nil) {
		return nil, fmt.Errorf("规则 %q 为元数据规则，不得携带聚合声明（identity / aggregate）", rj.ID)
	}
	// 旧档归一：聚合产出内携带绑定键（aggregate.contentBindings）的条目把
	// 绑定上缴到规则级「元数据文件」产出落档位——载入即归一、零拒绝零迁移。
	if err := normalizeLegacyAggregateBindings(rj); err != nil {
		return nil, err
	}
	if rj.Category == RuleCategoryAggregate {
		if err := validateAggregateEntry(rj); err != nil {
			return nil, err
		}
	}
	// 旧档归一：元数据文件产出的旧配对键（metadataFile.pairing）上缴到分组
	// 绑定落档位（metadataFile.grouping）——载入即归一、零拒绝零迁移；并用
	// 缺省绑定不在此物化（求值期按聚合成作品所绑分组取用）。
	if err := normalizeLegacyPairingKey(rj); err != nil {
		return nil, err
	}
	var notes []error
	if loading {
		notes = normalizeLegacyDirectoryGroupings(rj)
	}
	if err := validateMetadataFileProduction(rj); err != nil {
		return nil, err
	}
	if loading {
		notes = append(notes, dropWorkSetEntriesWithUnreachableSetName(rj)...)
	}
	if err := validateWorkSetAggregationProduction(rj); err != nil {
		return nil, err
	}
	return notes, nil
}

// compileRuleEntry 把通过声明准备的规则中间结构编译为 RuleEntry：conditions
// 形态（v4）编译为模式树缓存并对提取动作做行引用存在性校验；pattern 形态
// （旧）走模式树载入。两形态皆缺或编译校验失败返回中文错误。
func compileRuleEntry(rj *ruleEntryJSON) (*RuleEntry, error) {
	// 组合规则（refs 非空）的空条件表落档形态已随 refs 退役消亡：空条件表
	// 一律拒绝（无条件行的普通规则=恒命中，违背 v4 语义）。
	if rj.Conditions != nil {
		if rj.Pattern != nil {
			return nil, fmt.Errorf("规则 %q 同时携带 conditions 与 pattern（非法形态）", rj.ID)
		}
		if len(rj.Conditions) == 0 {
			return nil, fmt.Errorf("规则 %q 的条件行非法: 条件行列表不能为空", rj.ID)
		}
		// 元数据类别行式底盘的模板尾内联扩展名守卫（决策8 甲案·与聚合轨行
		// 守卫同口径同风格）：文件名行模板对去扩展名的文件名求值，尾字面
		// 白名单扩展名恒不命中——此前元数据类别无此守卫（静默死行），补齐
		// 拒收+指路；聚合类别行由 validateAggregateEntry 先行拦截（文案含
		// 「文件轨行」措辞），此处不重复覆盖、聚合路径零变化。
		if rj.Category != RuleCategoryAggregate {
			for i := range rj.Conditions {
				row := &rj.Conditions[i]
				if row.Condition == nil || row.Condition.Target != CondxTargetFile || row.Condition.Format.Mode == CondxFormatContains {
					continue
				}
				if ext, hit := condxTplTailKnownExt(row.Condition.Format.Tpl); hit {
					return nil, fmt.Errorf("规则 %q 第 %d 行的文件名行模板尾部误写扩展名 %s：模板只匹配去扩展名的文件名，该形态恒不命中，请删除模板尾部的 %s，改用「扩展名是」附加条件承载扩展名", rj.ID, i+1, ext, ext)
				}
			}
		}
		pat, err := CompileRuleConditions(rj.Conditions)
		if err != nil {
			return nil, fmt.Errorf("规则 %q 的条件行非法: %w", rj.ID, err)
		}
		// 限定键/refCapture 的成员资格口径：本规则供给方集（refScope 行扫描）
		// ——validateExpr 的限定名放行与 refCapture 的 refRule∈供给方集校验
		// 共用；跨规则校验在第二遍（validateRefScopeWiring）。
		providers := refScopeProvidersOfRows(rj.Conditions)
		if err := validateExtractionsCore(rj.ID, providers, pat, rj.Extractions,
			declaredConditionCaptures(rj.Conditions), declaredConditionRepeatCaptures(rj.Conditions), true); err != nil {
			return nil, err
		}
		return &RuleEntry{
			ID:                 rj.ID,
			Description:        rj.Description,
			Source:             rj.Source,
			Enabled:            rj.Enabled,
			Category:           rj.Category,
			Identity:           rj.Identity,
			Aggregate:          rj.Aggregate,
			MetadataFile:       rj.MetadataFile,
			WorkSetAggregation: workSetAggregationEntries(rj.WorkSetAggregation),
			Conditions:         rj.Conditions,
			Pattern:            pat,
			Extractions:        rj.Extractions,
			Synthesis:          rj.Synthesis,
		}, nil
	}
	if rj.Pattern == nil {
		return nil, fmt.Errorf("规则 %q 缺少条件行（conditions）", rj.ID)
	}
	pat, err := patternFromJSON(rj.Pattern)
	if err != nil {
		return nil, fmt.Errorf("规则 %q 的模式非法: %w", rj.ID, err)
	}
	if err := validateExtractions(rj.ID, nil, pat, rj.Extractions, declaredPatternCaptures(pat), false); err != nil {
		return nil, err
	}
	return &RuleEntry{
		ID:                 rj.ID,
		Description:        rj.Description,
		Source:             rj.Source,
		Enabled:            rj.Enabled,
		Category:           rj.Category,
		Identity:           rj.Identity,
		Aggregate:          rj.Aggregate,
		MetadataFile:       rj.MetadataFile,
		WorkSetAggregation: workSetAggregationEntries(rj.WorkSetAggregation),
		Pattern:            pat,
		Extractions:        rj.Extractions,
		Synthesis:          rj.Synthesis,
	}, nil
}

// knownFileExtsForTplTail 为文件轨行「模板尾部误写扩展名」的识别白名单（小写
// 带点形）：文件主体模板对去扩展名的文件名求值（FileSubjectPattern 的词汇
// 分工——扩展名由 fileExt 终结谓词承载），尾字面以本表扩展名收尾即恒不命中
// 形态，载入拒收并指引用户改走谓词。白名单外的点段（如 .part1）是文件名正文
// 内的合法点段，不拦。
var knownFileExtsForTplTail = map[string]bool{
	".mp4": true, ".mkv": true, ".avi": true, ".mov": true, ".wmv": true,
	".m4v": true, ".webm": true, ".flv": true, ".ts": true, ".m2ts": true,
	".srt": true, ".ass": true, ".ssa": true, ".sub": true, ".vtt": true,
	".txt": true, ".md": true, ".xml": true, ".nfo": true, ".rtf": true,
	".odt": true, ".pdf": true, ".doc": true, ".docx": true, ".epub": true,
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
	".bmp": true, ".avif": true, ".heic": true, ".tiff": true,
	".mp3": true, ".flac": true, ".wav": true, ".m4a": true, ".ogg": true,
	".aac": true, ".mka": true, ".opus": true,
	".zip": true, ".rar": true, ".7z": true, ".tar": true, ".gz": true,
	".bz2": true, ".cbz": true, ".cbr": true,
}

// condxTplTailKnownExt 报告模板尾字面段是否以白名单扩展名收尾：尾字面段取
// 词法切分的末段（最后一个占位词之后的字面文本；无占位词即整条模板），其自
// 最后一个点起的小写后缀命中白名单时返回该扩展名（带点形）。模板以占位词
// 收尾（无尾字面）或点段不在白名单时第二个返回值为 false。
func condxTplTailKnownExt(tpl string) (string, bool) {
	tokens, err := tokenizeCondx(tpl)
	if err != nil || len(tokens) == 0 {
		return "", false
	}
	tail := tokens[len(tokens)-1].literal
	dot := strings.LastIndexByte(tail, '.')
	if dot < 0 || dot == len(tail)-1 {
		return "", false
	}
	ext := strings.ToLower(tail[dot:])
	return ext, knownFileExtsForTplTail[ext]
}

// validateAggregateEntry 聚合规则条目的载入校验：三要件齐备（条件行+分组依据
// +分轨入口；分组依据=作品标识声明（identity）或目录分组绑定
// （aggregate.grouping）二选一，同时在场即拒）、条件行布尔自由（括号子组/行级
// 非放开——决策10：成员判定
// 消费规则完整求值的命中集，行级命中照常按行）、提取动作可携带（决策10 解除
// 互斥——提取声明挂各命中文件，行引用校验照常走 validateExtractions；内容
// 绑定为规则级「元数据文件」产出、不占提取动作——除外条款见
// validateMetadataFileProduction）、聚合条件行禁作用域附件（聚合段不跑门段
// 通道）与跨行相等值校验（行级命中独立求值，相等门引用他行绑定恒不可
// 得）、标识与分轨的行号及捕获名引用存在（行号=先序展平，组行占组内首行
// 号）、目录分组绑定的形态与引用合法（转交 validateGroupingBindingShape：
// 产生行须为本规则的目录条件行）、处置维度按行级指派口径归一
// （空串/track 归一 track；metadata 为被行级指派取代的拼写，载入即拒并指
// 路）、文件条件行模板尾字面误写扩展名拒收（模板对去扩展名的文件名求值，
// 该形态恒不命中——见 knownFileExtsForTplTail）。
func validateAggregateEntry(rj *ruleEntryJSON) error {
	if rj.Aggregate == nil {
		return fmt.Errorf("聚合规则 %q 缺少分轨入口（aggregate）", rj.ID)
	}
	// 分组依据二选一：目录分组绑定与作品标识声明同时在场时无法判定唯一的
	// 分组依据，载入拒绝；两者皆缺时聚合无键可依。
	if rj.Aggregate.Grouping != nil && len(rj.Identity) > 0 {
		return fmt.Errorf("聚合规则 %q 同时携带作品标识声明（identity）与目录分组绑定（aggregate.grouping）：两种分组绑定只能选一种", rj.ID)
	}
	if rj.Aggregate.Grouping == nil && len(rj.Identity) == 0 {
		return fmt.Errorf("聚合规则 %q 缺少分组依据（作品标识声明 identity 或目录分组绑定 aggregate.grouping，须选其一）", rj.ID)
	}
	switch rj.Aggregate.CompanionUse {
	case "", CompanionUseTrack:
		rj.Aggregate.CompanionUse = CompanionUseTrack
	case CompanionUseMetadata:
		return fmt.Errorf("聚合规则 %q 的处置维度 metadata（纯元数据承载）由行级指派合成表达（分轨入口+内容绑定勾选），规则级该值不再使用", rj.ID)
	default:
		return fmt.Errorf("聚合规则 %q 的处置维度非法: %q（可用值 track）", rj.ID, rj.Aggregate.CompanionUse)
	}
	if rj.Conditions == nil || rj.Pattern != nil {
		return fmt.Errorf("聚合规则 %q 须为条件行形态（conditions）", rj.ID)
	}
	if len(rj.Conditions) == 0 {
		return fmt.Errorf("聚合规则 %q 的条件行非法: 条件行列表不能为空", rj.ID)
	}
	// 行前连接词缺省归一：行式底盘的落档语法要求（非首行须带连接词）；平行
	// 行（存量聚合规则的现状形态）的连接词不参与成员判定——命中集接线见
	// aggregation.go 的 aggregateBooleanGate（括号/行级非在场才设门）。
	for i := range rj.Conditions {
		if i > 0 && rj.Conditions[i].Connector == "" {
			rj.Conditions[i].Connector = "and"
		}
	}
	// 条件行布尔自由（决策10）：括号子组/行级非放开，行号=先序展平（叶子行
	// 逐行编号，组行占组内首行号）。仍禁两类构造：作用域附件（聚合段不跑
	// 门段通道，附件无处求值——载入即拒，不留静默死形态）与跨行相等值校验
	// （行级命中独立求值，相等门引用他行绑定恒不可得）。
	leaves := condxLeafRowsOf(rj.Conditions)
	total := len(leaves)
	for _, leaf := range leaves {
		for _, a := range leaf.row.Condition.Attach {
			if a.Kind == CondxAttachRefScope {
				return fmt.Errorf("聚合规则 %q 第 %d 行携带作用域附件（refScope）：聚合段不跑门段通道，附件无处求值", rj.ID, leaf.no)
			}
		}
		for _, b := range leaf.row.Condition.Badges {
			if b.Kind == CondxBadgeEqual {
				return fmt.Errorf("聚合规则 %q 第 %d 行携带相等值校验：条件行独立求值，不支持跨行相等", rj.ID, leaf.no)
			}
		}
		// 文件条件行模板尾字面误写扩展名（仅 file 目标行；dir/seq 目标名不去
		// 扩展名，模板带点尾合法）：载入即拒并指路 fileExt 终结谓词。
		if leaf.row.Condition.Target == CondxTargetFile && leaf.row.Condition.Format.Mode != CondxFormatContains {
			if ext, hit := condxTplTailKnownExt(leaf.row.Condition.Format.Tpl); hit {
				return fmt.Errorf("聚合规则 %q 第 %d 行的文件条件行模板尾部误写扩展名 %s：模板只匹配去扩展名的文件名，该形态恒不命中，请删除模板尾部的 %s，改用「扩展名是」附加条件承载扩展名", rj.ID, leaf.no, ext, ext)
			}
		}
	}
	rowCaptures := make([]map[string]bool, total)
	for i, leaf := range leaves {
		caps, _, err := condxCapturesOf(leaf.row.Condition)
		if err != nil {
			return fmt.Errorf("聚合规则 %q 第 %d 行: %w", rj.ID, leaf.no, err)
		}
		set := make(map[string]bool, len(caps))
		for _, c := range caps {
			set[c] = true
		}
		rowCaptures[i] = set
	}
	// 目录分组绑定（aggregate.grouping 在场、标识声明缺席的形态）：形态与引用
	// 校验与另两种产出的分组绑定同一口径——所在目录分组的产生行须为本规则的
	// 目录条件行。轨行分配（Tracks）独立按行号校验，不随分组绑定形态变化。
	if rj.Aggregate.Grouping != nil {
		if err := validateGroupingBindingShape(rj.ID, rj.Aggregate.Grouping, rj.Conditions); err != nil {
			return fmt.Errorf("聚合规则 %q 的分组绑定（aggregate.grouping）: %w", rj.ID, err)
		}
	}
	for i := range rj.Identity {
		part := &rj.Identity[i]
		if part.Row < 1 || part.Row > total {
			return fmt.Errorf("聚合规则 %q 的标识声明第 %d 部分行号 %d 越界（共 %d 行）", rj.ID, i+1, part.Row, total)
		}
		if part.Capture == "" {
			return fmt.Errorf("聚合规则 %q 的标识声明第 %d 部分缺少捕获名", rj.ID, i+1)
		}
		if part.Capture == identityGroupKeyCapture {
			if leaves[part.Row-1].row.Condition.Target != CondxTargetSeq {
				return fmt.Errorf("聚合规则 %q 的标识声明第 %d 部分：序列群键（%s）只能选自序列目标行", rj.ID, i+1, identityGroupKeyCapture)
			}
			continue
		}
		declared := false
		for _, set := range rowCaptures {
			if set[part.Capture] {
				declared = true
				break
			}
		}
		if !declared {
			return fmt.Errorf("聚合规则 %q 的标识声明第 %d 部分引用的捕获名 %q 未在任何条件行声明", rj.ID, i+1, part.Capture)
		}
	}
	for i := range rj.Aggregate.Tracks {
		ta := &rj.Aggregate.Tracks[i]
		if ta.Row < 1 || ta.Row > total {
			return fmt.Errorf("聚合规则 %q 的分轨入口第 %d 项行号 %d 越界（共 %d 行）", rj.ID, i+1, ta.Row, total)
		}
		if ta.Role == "" {
			return fmt.Errorf("聚合规则 %q 的分轨入口第 %d 项缺少角色", rj.ID, i+1)
		}
	}
	return nil
}

// knownBindingDimensions 为内容绑定可声明的含义维度合法取值集（五个维度
// 枚举全值——内容整段可声明到任一维度；与 when 谓词的维度口径互不相干）。
var knownBindingDimensions = map[MetadataDimension]bool{
	DimAuthor: true, DimTag: true, DimTitle: true, DimCollection: true,
	DimDescription: true,
}

// normalizeLegacyAggregateBindings 旧档归一：聚合产出携带的旧绑定键
// （aggregate.contentBindings，由 AggregateProduction.UnmarshalJSON 读入暂存
// 位）上缴到规则级「元数据文件」产出落档位——载入即归一、零迁移零拒绝
// （归一后绑定照常供给两相提取与成员判定，分组由聚合标识驱动，行为与升级
// 前一致；保存按新落档位落档）。新旧两个落档位同时携带绑定为双写形态
// （无法判定唯一意图），载入拒绝。
func normalizeLegacyAggregateBindings(rj *ruleEntryJSON) error {
	if rj.Aggregate == nil || len(rj.Aggregate.legacyContentBindings) == 0 {
		return nil
	}
	legacy := rj.Aggregate.legacyContentBindings
	rj.Aggregate.legacyContentBindings = nil
	if rj.MetadataFile != nil && len(rj.MetadataFile.ContentBindings) > 0 {
		return fmt.Errorf("规则 %q 同时携带 aggregate.contentBindings 与 metadataFile.contentBindings（双写形态）：内容绑定只落规则级「元数据文件」产出", rj.ID)
	}
	if rj.MetadataFile == nil {
		rj.MetadataFile = &MetadataFileProduction{}
	}
	rj.MetadataFile.ContentBindings = legacy
	return nil
}

// normalizeLegacyPairingKey 旧档归一：元数据文件产出携带旧档配对键
// （metadataFile.pairing，由 UnmarshalJSON 读入暂存位）上缴到分组绑定落档位
// （metadataFile.grouping）——载入即归一、零迁移零拒绝（两形态 JSON 同构：
// mode=identity|directory、identity=键部分，归一只是键位迁移，归一后分组
// 求值照常）。新旧两个落档位同时携带（双写形态——无法判定唯一意图）载入
// 拒绝；保存只写新键位（归一形态随档）。并用缺省（分组绑定缺省而聚合产出
// 在场）不在此物化——求值期按聚合成作品所绑分组取用（便利缺省不落档）。
func normalizeLegacyPairingKey(rj *ruleEntryJSON) error {
	if rj.MetadataFile == nil || rj.MetadataFile.legacyPairing == nil {
		return nil
	}
	legacy := rj.MetadataFile.legacyPairing
	rj.MetadataFile.legacyPairing = nil
	if rj.MetadataFile.Grouping != nil {
		return fmt.Errorf("规则 %q 同时携带 metadataFile.pairing 与 metadataFile.grouping（双写形态）：分组绑定只落 metadataFile.grouping", rj.ID)
	}
	rj.MetadataFile.Grouping = legacy
	return nil
}

// isRowLessDirectoryBinding 报告分组绑定是否为未指定产生行的所在目录分组（只写
// mode=directory、没有 row、不带键部分的存量落档形态）；其余形态（已指定行号、
// 标识分组、所在目录分组却携带键部分的非法形态）均为 false。
func isRowLessDirectoryBinding(binding *GroupingBinding) bool {
	return binding != nil && binding.Mode == GroupingModeDirectory && binding.Row < 1 && len(binding.Identity) == 0
}

// legacyPresetDisposalHint 返回预置种子条目的旧形态处置指引：因所在目录分组
// 无目录行可依而被跳过「元数据文件」产出的条目若是预置种子（按稳定 ID 识别
// ——「恢复预置」按同一 ID 判缺席补回，恢复只补缺席不覆写在场条目），点名
// 「删除后恢复」即得新形态；非预置条目返回空串——用户自建条目没有出厂形态
// 可恢复，处置仍是补目录行或改用配对分组。
func legacyPresetDisposalHint(ruleID string) string {
	switch ruleID {
	case presetAggVideoSubtitleID, presetAggImageCompanionTextID, presetAggArticleImageTextID:
		return "——该预置规则是旧形态，已停用：删除它后点「恢复预置」即得新形态"
	}
	return ""
}

// normalizeLegacyDirectoryGroupings 载入期归一：未指定产生行的所在目录分组绑定
// 补为该规则首个目录条件行的行号；规则没有目录条件行时该绑定无键集可产，跳过
// 其所附产出并告警。处置两处落档位——「元数据文件」产出的分组绑定（无目录行
// 即整项产出跳过）与「作品集聚合」条目的分组绑定（无目录行即跳过该条目，全部
// 条目被跳过即无该产出）。补行与跳过都逐条返回点名规则与产出的告警：补行告警
// 提示核对首个目录行是否还会命中（该分组的键集只由这一行命中文件的所在目录
// 构成），规则有多条目录行时点名只继承第 1 行、其余目录行不入该分组；被跳过
// 的「元数据文件」产出若属于预置种子条目，告警追加出厂形态的处置指引（见
// legacyPresetDisposalHint）。
// 「聚合成作品」的分组绑定（aggregate.grouping）只以指定产生行的形态落档，缺行
// 由聚合条目校验拒绝，不经本归一。
func normalizeLegacyDirectoryGroupings(rj *ruleEntryJSON) []error {
	var dirRows []int
	for _, leaf := range condxLeafRowsOf(rj.Conditions) {
		if leaf.row.Condition.Target == CondxTargetDir {
			dirRows = append(dirRows, leaf.no)
		}
	}
	if rj.MetadataFile == nil && rj.WorkSetAggregation == nil {
		return nil
	}
	label := fmt.Sprintf("规则 %q", rj.ID)
	if rj.Description != "" {
		label += fmt.Sprintf("（%s）", rj.Description)
	}
	var narrowing string
	if len(dirRows) > 1 {
		others := make([]string, 0, len(dirRows)-1)
		for _, no := range dirRows[1:] {
			others = append(others, fmt.Sprintf("第 %d 行", no))
		}
		narrowing = fmt.Sprintf("（本规则共 %d 条目录行，该分组只继承第 1 行：%s的目录不入该分组）", len(dirRows), strings.Join(others, "、"))
	}
	noDirRow := "绑定了目录分组，但本规则没有目录条件行（目录分组的键集由目录行产出）——请补一条目录行，或改用配对分组"
	var warns []error
	if mf := rj.MetadataFile; mf != nil && isRowLessDirectoryBinding(mf.Grouping) {
		if len(dirRows) == 0 {
			rj.MetadataFile = nil
			warns = append(warns, fmt.Errorf("%s的「元数据文件」产出%s，已跳过该产出%s", label, noDirRow, legacyPresetDisposalHint(rj.ID)))
		} else {
			mf.Grouping.Row = dirRows[0]
			warns = append(warns, fmt.Errorf("%s的「元数据文件」产出的目录分组未指定产生行，已按首个目录行补为第 %d 行——请检查该目录行是否还会命中%s", label, dirRows[0], narrowing))
		}
	}
	if ws := rj.WorkSetAggregation; ws != nil {
		kept := make([]WorkSetAggregationProduction, 0, len(ws.Entries))
		for i := range ws.Entries {
			entry := &ws.Entries[i]
			if !isRowLessDirectoryBinding(entry.Grouping) {
				kept = append(kept, *entry)
				continue
			}
			subject := fmt.Sprintf("%s的作品集聚合第 %d 条", label, i+1)
			if len(dirRows) == 0 {
				warns = append(warns, fmt.Errorf("%s%s，已跳过该条目", subject, noDirRow))
				continue
			}
			entry.Grouping.Row = dirRows[0]
			kept = append(kept, *entry)
			warns = append(warns, fmt.Errorf("%s的目录分组未指定产生行，已按首个目录行补为第 %d 行——请检查该目录行是否还会命中%s", subject, dirRows[0], narrowing))
		}
		if len(kept) == 0 {
			rj.WorkSetAggregation = nil
		} else {
			ws.Entries = kept
		}
	}
	return warns
}

// validateMetadataFileProduction 「元数据文件」产出（规则级落档位）的载入
// 校验：绑定行号在本规则条件行范围内、维度为合法维度枚举、同一行同一维度
// 唯一（一行内容整段可同时映射多个维度=同行多条目，同一维度重复勾选无效果
// 即拒绝）。分组绑定按形态校验：标识分组=键部分（行·捕获名）的行号与捕获
// 名引用存在（序列群键保留名只选自序列目标行，与聚合标识声明同口径）；
// 所在目录分组=不携带键部分（目录即分组键）；映射单用（本规则无聚合产出）
// 时分组绑定必填——否则说明内容无分组可依即无落点；并用（有聚合产出）时
// 可缺省（求值期缺省绑聚合成作品所绑分组，档案不物化）。提取动作面：聚合
// 并存已放开（提取声明挂各命中文件，行引用校验照常走 validateExtractions）；
// 内容来源取值拒绝照旧（内容通路唯一面=「元数据文件」产出的内容绑定）。
func validateMetadataFileProduction(rj *ruleEntryJSON) error {
	mf := rj.MetadataFile
	if mf == nil {
		return nil
	}
	total := countCondxRows(rj.Conditions)
	if len(mf.ContentBindings) == 0 {
		return fmt.Errorf("规则 %q 的元数据文件产出缺少内容绑定（行→维度指派）", rj.ID)
	}
	seenRowDims := make(map[ContentBinding]bool, len(mf.ContentBindings))
	for i := range mf.ContentBindings {
		b := &mf.ContentBindings[i]
		if b.Row < 1 || b.Row > total {
			return fmt.Errorf("规则 %q 的内容绑定第 %d 项行号 %d 越界（共 %d 行）", rj.ID, i+1, b.Row, total)
		}
		if seenRowDims[*b] {
			return fmt.Errorf("规则 %q 的内容绑定第 %d 项与在先项重复指派第 %d 行的同一维度 %q（同一行同一维度唯一）", rj.ID, i+1, b.Row, b.Dimension)
		}
		seenRowDims[*b] = true
		if !knownBindingDimensions[b.Dimension] {
			return fmt.Errorf("规则 %q 的内容绑定第 %d 项维度非法: %q（作者/标签/标题/作品集/描述）", rj.ID, i+1, b.Dimension)
		}
	}
	return validateGroupingBinding(rj)
}

// validateGroupingBinding 「元数据文件」产出的分组绑定校验（validateMetadataFileProduction
// 的分组绑定面子程序）：映射单用（无聚合产出）时分组绑定必填、并用（有聚合
// 产出）的缺省绑定不落档（求值期取聚合成作品所绑分组，此处对缺省形态直接
// 放行）；形态与引用校验转交 validateGroupingBindingShape（与产出四「作品集
// 聚合」的组键绑定共用同一口径）。
func validateGroupingBinding(rj *ruleEntryJSON) error {
	mf := rj.MetadataFile
	if mf.Grouping == nil {
		if rj.Aggregate == nil {
			return fmt.Errorf("规则 %q 的元数据文件产出未绑定分组（无聚合产出时分组绑定必填：说明内容按分组扇出，无分组即无落点）", rj.ID)
		}
		return nil
	}
	return validateGroupingBindingShape(rj.ID, mf.Grouping, rj.Conditions)
}

// validateGroupingBindingShape 分组绑定声明的形态与引用校验（「元数据文件」
// 分组绑定、产出四「作品集聚合」的组键绑定与「聚合成作品」的目录分组绑定
// 共用）：形态枚举合法、标识分组的键部分非空且行号与捕获名引用存在（捕获名
// 按本规则全部条件行的声明面汇流查证——共享捕获跨行同名，与聚合并组的取值
// 口径一致；序列群键保留名只选自序列目标行）、所在目录分组不携带键部分且
// 必须指定产生行（Row：行号在条件行范围内、该行目标为目录——该目录行命中
// 文件的所在目录即分组的键集）。
func validateGroupingBindingShape(ruleID string, binding *GroupingBinding, conditions []CondxRow) error {
	if binding == nil {
		return fmt.Errorf("规则 %q 的分组绑定声明缺席", ruleID)
	}
	leaves := condxLeafRowsOf(conditions)
	total := len(leaves)
	switch binding.Mode {
	case GroupingModeIdentity:
		if len(binding.Identity) == 0 {
			return fmt.Errorf("规则 %q 的分组绑定为标识分组但缺少键部分（行·捕获名）", ruleID)
		}
	case GroupingModeDirectory:
		if len(binding.Identity) > 0 {
			return fmt.Errorf("规则 %q 的分组绑定为目录分组，不得携带键部分（目录分组以文件所在目录为分组键）", ruleID)
		}
		if binding.Row < 1 {
			return fmt.Errorf("规则 %q 的分组绑定为目录分组但缺少产生行（row：须指向本规则的一条目录条件行）", ruleID)
		}
		if binding.Row > total {
			return fmt.Errorf("规则 %q 的分组绑定的产生行行号 %d 越界（共 %d 行）", ruleID, binding.Row, total)
		}
		if leaves[binding.Row-1].row.Condition.Target != CondxTargetDir {
			return fmt.Errorf("规则 %q 的分组绑定（目录分组 · 第 %d 行）的产生行不是目录条件行（目录分组的产生行须为目标为目录的条件行）", ruleID, binding.Row)
		}
		return nil
	default:
		return fmt.Errorf("规则 %q 的分组绑定形态非法: %q（identity=标识分组 / directory=目录分组）", ruleID, string(binding.Mode))
	}
	rowCaptures := make([]map[string]bool, total)
	for i, leaf := range leaves {
		set := map[string]bool{}
		if caps, _, err := condxCapturesOf(leaf.row.Condition); err != nil {
			return fmt.Errorf("规则 %q 第 %d 行: %w", ruleID, leaf.no, err)
		} else {
			for _, c := range caps {
				set[c] = true
			}
		}
		rowCaptures[i] = set
	}
	for i := range binding.Identity {
		part := &binding.Identity[i]
		if part.Row < 1 || part.Row > total {
			return fmt.Errorf("规则 %q 的分组绑定第 %d 部分行号 %d 越界（共 %d 行）", ruleID, i+1, part.Row, total)
		}
		if part.Capture == "" {
			return fmt.Errorf("规则 %q 的分组绑定第 %d 部分缺少捕获名", ruleID, i+1)
		}
		if part.Capture == identityGroupKeyCapture {
			if leaves[part.Row-1].row.Condition.Target != CondxTargetSeq {
				return fmt.Errorf("规则 %q 的分组绑定第 %d 部分：序列群键（%s）只能选自序列目标行", ruleID, i+1, identityGroupKeyCapture)
			}
			continue
		}
		declared := false
		for _, set := range rowCaptures {
			if set[part.Capture] {
				declared = true
				break
			}
		}
		if !declared {
			return fmt.Errorf("规则 %q 的分组绑定第 %d 部分引用的捕获名 %q 未在任何条件行声明", ruleID, i+1, part.Capture)
		}
	}
	return nil
}

// groupingBindingsEqual 报告两个分组绑定是否选中同一分组（1:1 判据）：形态相同且
// 标识分组的键部分（行号·捕获名）逐项相等——所在目录分组无键部分，按产生行
// （Row）判等：产生行相同即同一分组（两条目录行各自是一个分组项，可各绑一个
// 产物）。nil 不与任何绑定成组（返回 false，缺归集依据由条目校验先行拦截）。
func groupingBindingsEqual(a, b *GroupingBinding) bool {
	if a == nil || b == nil || a.Mode != b.Mode {
		return false
	}
	if a.Mode != GroupingModeIdentity {
		return a.Row == b.Row
	}
	if len(a.Identity) != len(b.Identity) {
		return false
	}
	for i := range a.Identity {
		if a.Identity[i] != b.Identity[i] {
			return false
		}
	}
	return true
}

// validateNameExtraction 校验条目集名来源的引用存在性：行号在本规则条件行范围内、
// 捕获名非空且在该行声明（集名只取匹配区的命名捕获——行号+捕获名，捕获值即集名）。
// subject 为错误文案的条目主语（含规则 ID 与条目序）。
func validateNameExtraction(subject string, name *NameExtraction, conditions []CondxRow) error {
	if name == nil {
		return fmt.Errorf("%s没有集名来源（nameExtraction：行号+捕获名，须指向本规则条件行的命名捕获）", subject)
	}
	total := countCondxRows(conditions)
	if name.Row < 1 || name.Row > total {
		return fmt.Errorf("%s的集名来源行号 %d 越界（共 %d 行）", subject, name.Row, total)
	}
	if name.Capture == "" {
		return fmt.Errorf("%s的集名来源缺少捕获名（capture）", subject)
	}
	cond := condxRowAt(conditions, name.Row)
	if cond == nil {
		return fmt.Errorf("%s的集名来源行号 %d 不存在", subject, name.Row)
	}
	caps, _, err := condxCapturesOf(cond)
	if err != nil {
		return fmt.Errorf("%s的集名来源第 %d 行: %w", subject, name.Row, err)
	}
	for _, c := range caps {
		if c == name.Capture {
			return nil
		}
	}
	return fmt.Errorf("%s的集名来源引用的捕获名 %q 未在第 %d 行声明", subject, name.Capture, name.Row)
}

// validateWorkSetAggregationProduction 产出四「作品集聚合」的载入/保存校验（逐条
// 条目 + 条目间 1:1）：
//
//	① 每条必须有归集依据（grouping）且形态与引用合法（复用统一分组绑定校验）；
//	② 每条必须能取到集名——NameExtraction 指到的捕获须在该条件行声明（行号在本
//	   规则条件行范围内、捕获名非空）——无集名来源的条目载入跳过+告警、编辑保存
//	   被拦（集名只有卡内一个出口）；
//	③ 1:1——同一分组不得被两个条目选中（含跨聚合：与「聚合成作品」的分组绑定
//	   互斥——aggregate.grouping 在场取之，否则取标识声明 identity 的等值形态），
//	   报错点名冲突的两处条目；
//	④ 集名必须由所绑分组项保证可得（键可得则集名必可得，见
//	   groupingBindingGuaranteesSetName）——不满足的条目载入跳过+告警、编辑保存
//	   被拦。④ 先于 ③ 判定：集名不可得的条目不参与 1:1。
func validateWorkSetAggregationProduction(rj *ruleEntryJSON) error {
	ws := rj.WorkSetAggregation
	if ws == nil || len(ws.Entries) == 0 {
		return nil
	}
	if rj.Conditions == nil {
		// 归集求值消费条件行命中面（逐行独立求值+布尔门）：模式树形态无行
		// 可求值，落档即静默零产出——显式拒绝不静默。
		return fmt.Errorf("规则 %q 的作品集聚合产出须为条件行形态（conditions）", rj.ID)
	}
	// 「聚合成作品」的分组绑定——跨聚合 1:1 判据：目录分组绑定（aggregate.
	// grouping）在场即它，否则为本规则的标识声明（identity）的等值形态。
	var aggGrouping *GroupingBinding
	if rj.Aggregate != nil {
		if rj.Aggregate.Grouping != nil {
			aggGrouping = rj.Aggregate.Grouping
		} else if len(rj.Identity) > 0 {
			aggGrouping = &GroupingBinding{Mode: GroupingModeIdentity, Identity: rj.Identity}
		}
	}
	for i := range ws.Entries {
		entry := &ws.Entries[i]
		subject := fmt.Sprintf("规则 %q 的作品集聚合第 %d 条", rj.ID, i+1)
		// ① 归集依据必填且形态/引用合法。
		if entry.Grouping == nil {
			return fmt.Errorf("%s缺少归集依据（grouping：identity=行捕获复合/序列群键、directory=目录分组）", subject)
		}
		if err := validateGroupingBindingShape(rj.ID, entry.Grouping, rj.Conditions); err != nil {
			return fmt.Errorf("作品集聚合第 %d 条: %w", i+1, err)
		}
		// ② 集名来源必填且引用存在。
		if err := validateNameExtraction(subject, entry.NameExtraction, rj.Conditions); err != nil {
			return err
		}
		// ④ 集名由所绑分组项保证可得。
		if !groupingBindingGuaranteesSetName(entry.Grouping, entry.NameExtraction) {
			return fmt.Errorf("%s%s", subject, setNameUnreachableReason(entry.Grouping, entry.NameExtraction))
		}
		// ③ 1:1：条目间同一分组互斥。
		for j := 0; j < i; j++ {
			if groupingBindingsEqual(ws.Entries[j].Grouping, entry.Grouping) {
				return fmt.Errorf("%s与第 %d 条选中了同一分组（1:1：一个分组只能被一个聚合条目使用）", subject, j+1)
			}
		}
		// ③ 1:1 跨聚合：与「聚合成作品」的分组绑定互斥。
		if aggGrouping != nil && groupingBindingsEqual(aggGrouping, entry.Grouping) {
			return fmt.Errorf("%s与「聚合成作品」的分组绑定相同（1:1：同一分组不得同时供两种聚合使用）", subject)
		}
	}
	return nil
}

// groupingBindingGuaranteesSetName 报告作品集聚合条目的集名来源是否由所绑分组项
// 保证可得（组键可得则集名必可得——杜绝「组在而名不可得」的静默无集）：
//   - 所在目录分组：集名行号等于产生行（Row）；
//   - 单序列群键分组：集名行号等于该群键部分的行号（产生行）；
//   - 手写拼接分组：（行号·捕获名）是该分组键的一个部分。
//
// 调用前分组绑定与集名来源须已通过各自的形态/引用校验（非法形态恒返回
// false）。
func groupingBindingGuaranteesSetName(binding *GroupingBinding, name *NameExtraction) bool {
	if binding == nil || name == nil {
		return false
	}
	if binding.Mode == GroupingModeDirectory {
		return name.Row == binding.Row
	}
	if len(binding.Identity) == 1 && binding.Identity[0].Capture == identityGroupKeyCapture {
		return name.Row == binding.Identity[0].Row
	}
	for _, part := range binding.Identity {
		if part.Row == name.Row && part.Capture == name.Capture {
			return true
		}
	}
	return false
}

// setNameUnreachableReason 生成「集名来源不由所绑分组项保证可得」的原因文案：
// 以「的集名来源（第 N 行·捕获名）」开头、可直接接在条目主语之后，点名集名
// 现指的位置与应取的位置（产生行或分组键部分）。调用前提同
// groupingBindingGuaranteesSetName。
func setNameUnreachableReason(binding *GroupingBinding, name *NameExtraction) string {
	at := fmt.Sprintf("的集名来源（第 %d 行·%s）", name.Row, name.Capture)
	if binding.Mode == GroupingModeDirectory {
		return fmt.Sprintf("%s不在所绑目录分组的产生行上（产生行=第 %d 行）——集名只能取自该分组所在的那一行", at, binding.Row)
	}
	if len(binding.Identity) == 1 && binding.Identity[0].Capture == identityGroupKeyCapture {
		return fmt.Sprintf("%s不在所绑序列分组的产生行上（产生行=第 %d 行）——集名只能取自该分组所在的那一行", at, binding.Identity[0].Row)
	}
	parts := make([]string, 0, len(binding.Identity))
	for _, part := range binding.Identity {
		parts = append(parts, fmt.Sprintf("第 %d 行·%s", part.Row, part.Capture))
	}
	return fmt.Sprintf("%s不是所绑分组键的一个部分（键部分：%s）——集名只能取自分组键的部分", at, strings.Join(parts, "、"))
}

// dropWorkSetEntriesWithUnreachableSetName 载入期处置：作品集聚合条目的集名来源
// 不由所绑分组项保证可得时跳过该条目（该条归集的作品集随之消失，不做自动
// 改写——集名该取哪个捕获只有用户能定），逐条返回点名条目的告警。只处置
// 分组绑定形态与集名来源引用均已合法的条目；这两者非法的条目原样保留，交
// 严格校验拒绝整条规则。全部条目被跳过即无该产出（落档不写该键）。
func dropWorkSetEntriesWithUnreachableSetName(rj *ruleEntryJSON) []error {
	ws := rj.WorkSetAggregation
	if ws == nil || rj.Conditions == nil {
		return nil
	}
	var warns []error
	kept := make([]WorkSetAggregationProduction, 0, len(ws.Entries))
	for i := range ws.Entries {
		entry := &ws.Entries[i]
		if validateGroupingBindingShape(rj.ID, entry.Grouping, rj.Conditions) != nil ||
			validateNameExtraction("", entry.NameExtraction, rj.Conditions) != nil ||
			groupingBindingGuaranteesSetName(entry.Grouping, entry.NameExtraction) {
			kept = append(kept, *entry)
			continue
		}
		subject := fmt.Sprintf("规则 %q 的作品集聚合第 %d 条", rj.ID, i+1)
		warns = append(warns, fmt.Errorf("%s%s，已跳过该条目", subject, setNameUnreachableReason(entry.Grouping, entry.NameExtraction)))
	}
	if len(kept) == 0 {
		rj.WorkSetAggregation = nil
		return warns
	}
	ws.Entries = kept
	return warns
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
// 前端编辑面第 1 行省略 row 字段为合法形态）、供给方捕获（refCapture）的
// refRule 缺失/不在本规则供给方集内或缺少 captureName、条件表达式引用不存在
// （谓词来源/维度、捕获组名不在声明集合内；限定键形操作数「供给方ID.rN.组名」
// 按本规则供给方集放行，见 validateExpr）或类型不匹配时返回含规则 ID 与动作序号
// 的中文错误。providers 为本条规则的作用域供给方 ID 集（refScope 行扫描，按
// 出现序去重；无附件规则为空），declaredCaptures 为该规则形态（条件行或模式
// 树）声明的捕获组名集合（v4 为行号前缀形态，rowPrefixed=true）。
func validateExtractions(ruleID string, providers []string, pattern Pattern, actions []ExtractionAction, declaredCaptures map[string]bool, rowPrefixed bool) error {
	return validateExtractionsCore(ruleID, providers, pattern, actions, declaredCaptures, nil, rowPrefixed)
}

// validateExtractionsCore 同 validateExtractions，另收 repeatCaptures（本条规则
// 自身条件行的重复段词捕获名集合，来源 declaredConditionRepeatCaptures）：提取
// 动作引用重复段词照常放行（提取口径含重复段词），但生效条件（when）引用时按
// 三面引用禁区的统一文案拒绝——条件口径=提取口径剔除重复段词。nil 视作无重复
// 段词（v3 模式树等形态）。
func validateExtractionsCore(ruleID string, providers []string, pattern Pattern, actions []ExtractionAction, declaredCaptures, repeatCaptures map[string]bool, rowPrefixed bool) error {
	// 条件可引用口径：提取口径剔除重复段词（when 的声明存在性校验用）。
	conditionCaptures := declaredCaptures
	if len(repeatCaptures) > 0 {
		conditionCaptures = make(map[string]bool, len(declaredCaptures))
		for name := range declaredCaptures {
			if !repeatCaptures[name] {
				conditionCaptures[name] = true
			}
		}
	}

	for i := range actions {
		action := &actions[i]
		if action.From == FromContent {
			// 内容来源只经「元数据文件」产出的内容绑定面成立：元数据规则的
			// 提取动作携带内容取值即拒绝（不留静默零产出形态）。
			return fmt.Errorf("规则 %q 的第 %d 条提取动作为内容取值：元数据规则不得声明内容来源（内容来源只经「元数据文件」产出的内容绑定面成立）",
				ruleID, i+1)
		}
		if action.From == FromRefCapture {
			// 供给方捕获（作用域附件消费规则专用）：refRule 必填且须属本条
			// 供给方集；captureName 必填（对齐 FromCapture 的必填口径）。
			if action.RefRule == "" {
				return fmt.Errorf("规则 %q 的第 %d 条提取动作为供给方捕获（refCapture）但缺少被引规则 ID（refRule）",
					ruleID, i+1)
			}
			if !containsString(providers, action.RefRule) {
				return fmt.Errorf("规则 %q 的第 %d 条提取动作引用的规则 %q 不在本规则的作用域供给方集内（refScope 附件未引用它）",
					ruleID, i+1, action.RefRule)
			}
			if action.CaptureName == "" {
				return fmt.Errorf("规则 %q 的第 %d 条提取动作为供给方捕获（refCapture）但缺少捕获组名（captureName）",
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
		if _, err := validateExpr(ruleID, action.When, conditionCaptures, repeatCaptures, providers); err != nil {
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
// declaredCaptures 为条件可引用口径的捕获组名集合（提取口径剔除重复段词）；
// repeatCaptures 为重复段词捕获名标记集（其值多值产出、暂不能用于生效条件）；
// providers 为本条规则的作用域供给方 ID 集（refScope 行扫描）——限定键形
// 捕获操作数「供给方ID.rN.组名」（显式限定形，可带 #canon 后缀）按其放行
// 为字符串操作数（求值侧 conditionOperandValue 从注入区取值，两侧共用
// parseRefCaptureName 解析、口径一致），被引供给方不在集内即拒绝。行/组在
// 被引供给方声明中的存在性不做核验——供给方声明在本管线时点不可得（逐规则
// 构造先于跨规则汇集），求值侧未绑定即取值不可用、含该操作数的比较为假
// （保守取向，与 refCapture 提取动作的形核口径一致）；供给方侧的重复段词
// 引用由跨规则校验（validateRefScopeWiring）在第二遍拒绝。
func validateExpr(ruleID string, e *CondExpr, declaredCaptures, repeatCaptures map[string]bool, providers []string) (exprValueType, error) {
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
		if repeatCaptures[e.CaptureName] {
			// 重复段占位词多值产出（单元值序列），生效条件只认单值坐标：
			// 三面引用禁区之一（统一文案，见 condxRepeatRefError）。
			return "", fmt.Errorf("规则 %q 的条件表达式: %w", ruleID,
				condxRepeatRefError(condxRepeatEnOfCapture(e.CaptureName)))
		}
		if memberID, _, ok := parseRefCaptureName(e.CaptureName); ok {
			// 限定键形操作数（消费规则 when 引用供给方捕获，显式形）：供给方
			// 在本条供给方集内即放行为字符串操作数；无附件规则（供给方集空）
			// 恒拒绝——限定名不是自有捕获的旁路（发现6：限定键为引用供给方
			// 捕获值的唯一通道）。
			if !containsString(providers, memberID) {
				return "", fmt.Errorf("规则 %q 的条件表达式引用的供给方规则 %q 不在本规则的作用域供给方集内（限定名操作数「供给方ID.rN.组名」只能引用 refScope 附件所引供给方的捕获）", ruleID, memberID)
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
		leftType, err := validateExpr(ruleID, e.Left, declaredCaptures, repeatCaptures, providers)
		if err != nil {
			return "", err
		}
		rightType, err := validateExpr(ruleID, e.Right, declaredCaptures, repeatCaptures, providers)
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
		operandType, err := validateExpr(ruleID, e.Operand, declaredCaptures, repeatCaptures, providers)
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
			subType, err := validateExpr(ruleID, sub, declaredCaptures, repeatCaptures, providers)
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
		innerType, err := validateExpr(ruleID, e.Inner, declaredCaptures, repeatCaptures, providers)
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

// skippedRuleEntry 载入时被跳过的档案条目（结构化形态）：规则管理面错误态行
// 的数据源——跳过条目对用户可见、可按 ID 删除（跳过条目无编辑出口的死锁收口），
// 跳过原因就地呈现。
type skippedRuleEntry struct {
	// ID 为条目原文的规则 ID（管理面删除寻址）。
	ID string
	// Description 为条目原文的描述。
	Description string
	// Reason 为跳过原因：不含「跳过规则」外层包装的原始校验错误文本。
	Reason string
	// Message 为完整告警文本：与既有告警渠道（ParseProfile 返回的 errs）逐字
	// 同源，供需要文本形态的调用方复用。
	Message string
}

// ParseProfile 从 JSON 数据载入规则档案：反序列化 + 逐条构造校验。
// 结构版本不在双读集（v4/v5）内的档案**整体拒载**——v1..v3 旧档已停用（机器
// 规则一律由新一代生产方直出，旧机器规则对用户无保留价值）；拒载档
// 案经 LoadProfile 自动重置（原文备份+空档覆写+知会警告），用户无需手动
// 处置。结构版本高于当前版本（未来版本）按尽力解析载入——记一条版本告警
// 错误（调用方落日志）、未知字段忽略、无法构造的规则逐条跳过，不中断整
// 体。v4 存量档案照常读入（升级用户自身的读入迁移），其内 refs 非空条目
// （组合规则旧词汇）跳过+告警、携带「位于第 N 级」词条的条目按未知词条
// 先例构造拒绝——同为跳过+告警。个别规则构造校验失败（条件行编译、正则、
// 深度约束、提取引用不存在、refCapture 的 refRule 缺失或不在本规则供给方集
// 内等）只产生含规则 ID 与描述的中文错误并跳过该规则，不阻塞其余规则载入；
// 作用域附件的跨规则校验（自引用/悬空/环/聚合互斥/重复段接线/供给方合格
// 性）同挂该出口——逐规则构造完成后对全部已构造规则统一校验，违规条目
// 同样跳过并返回带规则 ID 的错误。载入期归一（存量所在目录分组绑定补产生行
// 或跳过无目录行的产出、集名来源不在所绑分组产生行的作品集聚合条目跳过，
// 见 ruleFromJSONOnLoad）的告警同随该错误列表返回，规则本身照常载入。
// 无错误时错误列表为空。
func ParseProfile(data []byte) (*RuleProfile, []error) {
	profile, skips, errs := parseProfile(data)
	for _, s := range skips {
		errs = append(errs, errors.New(s.Message))
	}
	return profile, errs
}

// parseProfile 载入主体（ParseProfile 的结构化变体）：被跳过的条目（逐条构造
// 失败与作用域附件跨引用违规两处出口）以 skippedRuleEntry 结构化返回，errs
// 不含其告警文本——呈现形态由调用方决定：ParseProfile 回填文本告警保持既有
// 口径（日志等文本消费面不变），规则管理视图以错误态行呈现（不双报）。
func parseProfile(data []byte) (*RuleProfile, []*skippedRuleEntry, []error) {
	var in ruleProfileJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, nil, []error{fmt.Errorf("规则档案反序列化失败: %w", err)}
	}
	var errs []error
	switch {
	case in.SchemaVersion > CurrentSchemaVersion:
		errs = append(errs, fmt.Errorf(
			"规则档案结构版本为 %d，高于当前支持的版本 %d，按尽力解析载入：未知字段将被忽略、无法解析的规则将被跳过，重写后落盘为版本 %d",
			in.SchemaVersion, CurrentSchemaVersion, CurrentSchemaVersion))
	case in.SchemaVersion != CurrentSchemaVersion && in.SchemaVersion != legacyRefSchemaVersion:
		// v1..v3 旧档整体停用：零迁移拒载（含可兼容升格的历史版本——机器
		// 规则对用户无保留价值，重置后由新一代生产方重新生成）。
		return nil, nil, []error{fmt.Errorf(
			"规则档案结构版本为 %d（当前 %d），旧档案已停用",
			in.SchemaVersion, CurrentSchemaVersion)}
	}
	profile := &RuleProfile{
		SchemaVersion: CurrentSchemaVersion, Rules: make([]RuleEntry, 0, len(in.Rules)),
	}
	// 存量剥壳字段迁移（反序列化后、规则构造校验前逐条过）：可等价改写者
	// 行模板改写为字面括号形态，其余丢弃并留描述注记——改写产物随正常
	// 管线编译校验。
	migrateLegacyStripRules(in.Rules)
	// v4 存量 refs 条目防御（组合规则词汇已退役）：refs 非空条目跳过+告警
	// ——条目形态（空条件行+成员门）在新语法中不可表达，载入侧按值跳过。
	legacyRefs := legacyProfileRefsByIndex(data, in.SchemaVersion, len(in.Rules))
	// 第一遍：逐规则构造（模式/条件行编译 + 提取动作校验，含 refCapture 的
	// refRule∈供给方集、captureName 必填）。失败条目结构化跳过（原文条目携带
	// ID/描述，管理面错误态行与删除出口的数据源）。
	var skips []*skippedRuleEntry
	// skipIdx 与 skips 平行：各跳过条目在原文 rules 数组中的下标（写档保真的
	// 原文定位）。
	var skipIdx []int
	constructed := make([]RuleEntry, 0, len(in.Rules))
	// constructedIdx 与 constructed 平行：已构造规则在原文 rules 数组中的下标
	//（第二遍跨引用校验的跳过条目凭此回指原文）。
	constructedIdx := make([]int, 0, len(in.Rules))
	for i := range in.Rules {
		rj := &in.Rules[i]
		if legacyRefs != nil && legacyRefs[i] {
			skips = append(skips, &skippedRuleEntry{
				ID:          rj.ID,
				Description: rj.Description,
				Reason:      refsRetiredReason,
				Message:     fmt.Sprintf("跳过规则 %q（%s）: %s", rj.ID, rj.Description, refsRetiredReason),
			})
			skipIdx = append(skipIdx, i)
			continue
		}
		// 载入形态构造：存量形态的载入期归一（所在目录分组补产生行/跳过产出、
		// 集名来源不在产生行的条目跳过）以告警随 errs 返回，规则本身照常载入。
		rule, notes, err := ruleFromJSONOnLoad(rj)
		if err != nil {
			skips = append(skips, &skippedRuleEntry{
				ID:          rj.ID,
				Description: rj.Description,
				Reason:      err.Error(),
				Message:     fmt.Sprintf("跳过规则 %q（%s）: %v", rj.ID, rj.Description, err),
			})
			skipIdx = append(skipIdx, i)
			continue
		}
		errs = append(errs, notes...)
		constructed = append(constructed, *rule)
		constructedIdx = append(constructedIdx, i)
	}
	// 第二遍：跨规则引用校验——作用域附件 refScope 五则（自引用/悬空/环/
	// 聚合互斥/重复段接线 + 供给方纯目录合格性）。以第一遍已构造规则集为
	//「本档案规则集」——被逐条构造拒收的规则不在场，指向它的引用即按悬空
	// 处理。违规条目同走坏条目出口（结构化跳过，告警点名消费规则——描述
	// 优先，缺失退回 ID）。
	byID := make(map[string]int, len(constructed))
	for i := range constructed {
		if _, seen := byID[constructed[i].ID]; !seen {
			byID[constructed[i].ID] = i
		}
	}
	// 供给方图环检测（校验③环化）：节点=已构造规则、边=规则 → 其规则来源
	// 供给方（滤除库根 sentinel——非规则 ID 不成依赖边；悬空引用不在 byID、
	// 不成边）。一次建图一次 DFS 判定，逐消费条目在 validateRefScopeWiring
	// 的③分支消费各自参与的环路路径。
	cyclePaths := refScopeCyclePaths(constructed, byID)
	for i := range constructed {
		rule := &constructed[i]
		if err := validateRefScopeWiring(rule, byID, constructed, cyclePaths); err != nil {
			label := rule.Description
			if label == "" {
				label = rule.ID
			}
			skips = append(skips, &skippedRuleEntry{
				ID:          rule.ID,
				Description: rule.Description,
				Reason:      err.Error(),
				Message:     fmt.Sprintf("规则『%s』%v", label, err),
			})
			skipIdx = append(skipIdx, constructedIdx[i])
			continue
		}
		profile.Rules = append(profile.Rules, *rule)
	}
	// 写档保真：跳过条目的原文字节随档案对象存续（SaveProfile 回写档案、后续
	// 载入照常跳过+告警——写档往返不再物理丢弃）。原文对不上（防御：data 已
	// 成功反序列化，正常不可达）时无原文可保真，跳过条目维持既告警口径。
	if len(skips) > 0 {
		if raws := rawProfileRuleEntries(data, len(in.Rules)); raws != nil {
			for _, i := range skipIdx {
				if i >= 0 && i < len(raws) {
					profile.preservedSkipRaw = append(profile.preservedSkipRaw, raws[i])
				}
			}
		}
	}
	return profile, skips, errs
}

// legacyRefSchemaVersion 为读入保留的历史结构版本（升级用户自身存量档案的
// 读入迁移，非旧插件兼容）：该版本档案读入照常、保存归一为当前版本。
const legacyRefSchemaVersion = 4

// refsRetiredReason v4 存量 refs 条目（组合规则）的跳过原因文案。
const refsRetiredReason = "条目携带已退役的规则引用（refs）——组合规则类别已消亡，引用复用请改用目录行作用域附件（refScope）重建规则"

// legacyProfileRefsByIndex 探测档案原文内各条目是否携带非空 refs（组合规则
// 旧词汇，反序列化中间结构已无该字段）：仅对历史版本档案（legacyRefSchema
// Version）执行一次性探测——当前版本档案由新二进制写出，不含该键；其余版本
// 返回 nil（零探测）。返回值按下标与 rules 数组对齐（true=该条携带非空 refs）。
func legacyProfileRefsByIndex(data []byte, schemaVersion, ruleCount int) []bool {
	if schemaVersion != legacyRefSchemaVersion || ruleCount == 0 {
		return nil
	}
	var probe struct {
		Rules []struct {
			LegacyRefs []string `json:"refs"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || len(probe.Rules) != ruleCount {
		return nil
	}
	out := make([]bool, ruleCount)
	for i := range probe.Rules {
		out[i] = len(probe.Rules[i].LegacyRefs) > 0
	}
	return out
}

// migrateLegacyStripRules 存量档案剥壳字段（stripPrefix / stripSuffix，反序列
// 化暂存于动作的 legacyStrip*）的载入迁移，逐规则过：可等价改写者（From 为
// capture、动作引用的条件行模板为裸单占位词 {词}、剥壳前后缀恰为同对括号
// 的开/闭字面）行模板改写为「开括号{词}闭括号{任意}」——剥壳语义由模板字面
// 括号与括号感知捕获类承载，产出从跨界捕获收敛为首组内名；其余带剥壳组合
// 不可等价改写，剥壳丢弃（求值不再剥壳，产出退回原始捕获值）。两类处置都
// 在规则描述追加迁移注记（查重防重复）。先于规则构造校验执行，改写产物按
// 正常管线编译。
func migrateLegacyStripRules(rules []ruleEntryJSON) {
	for i := range rules {
		migrateLegacyStripRule(&rules[i])
	}
}

// migrateLegacyStripRule 迁移单条规则的剥壳动作；无剥壳暂存的规则零变化。
func migrateLegacyStripRule(rj *ruleEntryJSON) {
	rewritten, dropped := false, false
	for i := range rj.Extractions {
		action := &rj.Extractions[i]
		if action.legacyStripPrefix == "" && action.legacyStripSuffix == "" {
			continue
		}
		if rewriteStripCaptureAction(rj, action) {
			rewritten = true
		} else {
			dropped = true
		}
		action.legacyStripPrefix, action.legacyStripSuffix = "", ""
	}
	if rewritten {
		rj.Description = appendMigrationNote(rj.Description, "剥壳已迁移为字面括号模板")
	}
	if dropped {
		rj.Description = appendMigrationNote(rj.Description, "剥壳配置已移除，请改用字面括号模板")
	}
}

// rewriteStripCaptureAction 尝试把动作的剥壳配置等价改写进其引用的条件行
// 模板：动作取值来源为 capture、引用行（Row 缺省按第 1 行兜底）格式为模板
// 且恰为裸单占位词 {词}（词与动作捕获组名对应）、剥壳前后缀恰为同对括号的
// 开/闭字面时，模板改写为「开括号{词}闭括号{任意}」并返回 true；任一条件
// 不满足（不可等价改写）返回 false，模板不动。
func rewriteStripCaptureAction(rj *ruleEntryJSON, action *ExtractionAction) bool {
	if action.From != FromCapture {
		return false
	}
	open := []rune(action.legacyStripPrefix)
	closing := []rune(action.legacyStripSuffix)
	if len(open) != 1 || len(closing) != 1 || condxOpenBrackets[open[0]] != closing[0] {
		return false
	}
	row := action.Row
	if row <= 0 {
		row = 1
	}
	cond := condxRowAt(rj.Conditions, row)
	if cond == nil || cond.Format.Mode == CondxFormatContains {
		return false
	}
	word, ok := barePlaceholderOf(cond.Format.Tpl)
	if !ok {
		return false
	}
	// 吞位词与序列键词不作引用位：不可能构成合法的裸引用行，按不可等价
	// 改写处置。
	if word == "任意" || word == "键" || word == "公共段" {
		return false
	}
	if condxVocab[word] != action.CaptureName {
		return false
	}
	cond.Format.Tpl = string(open[0]) + "{" + word + "}" + string(closing[0]) + "{任意}"
	return true
}

// barePlaceholderOf 判定模板是否为裸单占位词形态（恰一个占位词、无任何字面
// 段，如 "{标签}"）；是则返回占位词的词表原文，否则返回 false。
func barePlaceholderOf(tpl string) (string, bool) {
	tokens, err := tokenizeCondx(tpl)
	if err != nil || len(tokens) != 1 || tokens[0].placeholder == "" {
		return "", false
	}
	return tokens[0].placeholder, true
}

// condxRowAt 返回条件行列表先序展平的第 n 个（1 起）条件行的行内条件；行号
// 计数与编译及捕获收集同口径（Condition 行计 1、Group 子组按 countCondxRows
// 计入）；越界返回 nil。
func condxRowAt(rows []CondxRow, n int) *Condx {
	prefix := 0
	for i := range rows {
		row := &rows[i]
		if row.Condition != nil {
			if prefix+1 == n {
				return row.Condition
			}
			prefix++
		}
		if len(row.Group) > 0 {
			if c := condxRowAt(row.Group, n-prefix); c != nil {
				return c
			}
			prefix += countCondxRows(row.Group)
		}
	}
	return nil
}

// appendMigrationNote 把迁移注记追加到规则描述（描述已含同文注记时不重复
// 追加）。
func appendMigrationNote(desc, note string) string {
	if strings.Contains(desc, note) {
		return desc
	}
	if desc == "" {
		return note
	}
	return desc + "\n" + note
}

// profileDataHasLegacyStrip 判断档案原文是否携带旧剥壳键（stripPrefix /
// stripSuffix）：文本子串级宽松检测——误报（键名作为字符串值出现）只导致
// 多写一次迁移备份（内容仍为原文，无损）；供载入路径在迁移改写前完成一次
// 性备份编排。
func profileDataHasLegacyStrip(data []byte) bool {
	return bytes.Contains(data, []byte(`"stripPrefix"`)) || bytes.Contains(data, []byte(`"stripSuffix"`))
}

// validateRefRepeatWiring 校验作用域接线引用供给方的捕获不落在重复段词上
// （三面引用禁区的作用域接线面）：refCapture 提取动作（Row 缺省按第 1 行兜底，
// 与 validateExtractions 同口径）与 when 表达式内限定名操作数「供给方ID.rN.组名」
// 两路都查；供给方无条件行重复段词时零工作。取值面（refCaptureValue 从注入区取
// 单值）不动——契约要求构造期即拒，不留运行期空值。
func validateRefRepeatWiring(rule *RuleEntry, member *RuleEntry, ref string) error {
	repeatNames := declaredConditionRepeatCaptures(member.Conditions)
	if len(repeatNames) == 0 {
		return nil
	}
	for i := range rule.Extractions {
		action := &rule.Extractions[i]
		if err := validateExprRefRepeat(action.When, ref, repeatNames); err != nil {
			return fmt.Errorf("第 %d 条提取动作的生效条件: %w", i+1, err)
		}
		if action.From != FromRefCapture || action.RefRule != ref || action.CaptureName == "" {
			continue
		}
		row := action.Row
		if row <= 0 {
			row = 1
		}
		name := rowCaptureName(row, action.CaptureName)
		if !repeatNames[name] {
			continue
		}
		return fmt.Errorf("第 %d 条提取动作引用的供给方规则 %q 的捕获 %q: %w",
			i+1, ref, name, condxRepeatRefError(condxRepeatEnOfCapture(name)))
	}
	return nil
}

// validateExprRefRepeat 递归检查条件表达式树中「供给方ID.rN.组名」限定名操作数
// 是否落在被引供给方的重复段词上（作用域接线的 when 通道）。非限定名操作数不走本
// 校验（自有捕获的重复段词引用由 validateExpr 的 repeatCaptures 分支拒绝）。
func validateExprRefRepeat(e *CondExpr, ref string, repeatNames map[string]bool) error {
	if e == nil {
		return nil
	}
	if e.Kind == ExprCapture {
		if memberID, bindingName, ok := parseRefCaptureName(e.CaptureName); ok && memberID == ref {
			if repeatNames[bindingName] {
				return fmt.Errorf("引用的供给方规则 %q 的捕获 %q: %w",
					ref, bindingName, condxRepeatRefError(condxRepeatEnOfCapture(bindingName)))
			}
		}
	}
	for _, sub := range e.Subs {
		if err := validateExprRefRepeat(sub, ref, repeatNames); err != nil {
			return err
		}
	}
	for _, sub := range []*CondExpr{e.Left, e.Right, e.Operand, e.Inner} {
		if err := validateExprRefRepeat(sub, ref, repeatNames); err != nil {
			return err
		}
	}
	return nil
}

// validateRefScopeWiring 校验作用域附件（refScope）的跨规则接线（构造期跨
// 规则静态校验）：①自引用（供给方=本条 ID）
// ②悬空（供给方不在本档案已构造规则集内）③环（全档案「规则→规则来源
// 供给方」图有环且本条在环上——供给方自身可携带作用域附件〔两链/三链合法〕，
// 环路路径由 cyclePaths 点名；自引用=深度 1 的环，由①先行拒绝、文案不动）
// ④聚合互斥（供给方非聚合；聚合轨行
// 自身禁附件由 validateAggregateEntry 先行拦截——自决4）⑤重复段接线（消费
// 规则的 refCapture 提取与 when 限定名不落供给方重复段词——
// validateRefRepeatWiring）。另校验供给方合格
// 性：纯目录规则（裁决③——全部条件行〔含括号子组递归〕目标=目录；行级非的
// 目录行仍算目录行，其否定语义在求值面自然处理、命中层集只记实际命中层；
// 定群〔全序列行〕与模式树旧形态天然不满足——自决2：判定只看行的目标类
// 别，不猜布尔语义）。任一违规返回中文错误，调用方跳过该条并告警。byID 为
// 已构造规则 ID 到 all 下标的索引（重复 ID 取先出现者）；cyclePaths 为
// refScopeCyclePaths 的输出（成环节点 ID → 以其起讫的环路路径）。无 refScope
// 附件的规则零工作。库根 sentinel（RefScopeLibraryRoot）对①-⑤与供给方合格性
// 全部放行——非供给方规则、无捕获与层集求值形态可判（层集门段常量注入，
// 亦不成图边——取边滤除）。
func validateRefScopeWiring(rule *RuleEntry, byID map[string]int, all []RuleEntry, cyclePaths map[string]string) error {
	providers := refScopeProvidersOf(rule)
	for _, ref := range providers {
		if ref == RefScopeLibraryRoot {
			// 库根 sentinel：非供给方规则，无悬空/自引用/环/纯目录合格性
			// 可判（层集门段常量注入、不成图边），无重复段接线面（无捕获）
			// ——放行。
			continue
		}
		if ref == rule.ID {
			return fmt.Errorf("作用域附件引用了自身（供给方=本条 ID %q）", ref)
		}
		idx, ok := byID[ref]
		if !ok {
			return fmt.Errorf("作用域附件引用的供给方规则 %s 不存在，已跳过", ref)
		}
		member := &all[idx]
		if member.Category == RuleCategoryAggregate {
			return fmt.Errorf("作用域附件引用的供给方 %q 自身为聚合规则（聚合规则不作供给方）", ref)
		}
		if path, cyclic := cyclePaths[rule.ID]; cyclic {
			return fmt.Errorf("作用域附件引用的供给方链成环（%s）", path)
		}
		if !condxRowsAllDir(member.Conditions) {
			return fmt.Errorf("作用域附件引用的供给方 %q 不是纯目录规则（供给方的全部条件行须为目录目标）", ref)
		}
		if err := validateRefRepeatWiring(rule, member, ref); err != nil {
			return err
		}
	}
	return nil
}

// condxRowsAllDir 判定条件行列表（含括号子组递归）是否全部为目录目标行且
// 至少一行（纯目录规则判据，裁决③/自决2）。空表（v3 模式树旧形态）不满足
// ——无法按行目标核验，不作供给方。
func condxRowsAllDir(rows []CondxRow) bool {
	any := false
	for i := range rows {
		row := &rows[i]
		if row.Condition != nil {
			any = true
			if row.Condition.Target != CondxTargetDir {
				return false
			}
		}
		if len(row.Group) > 0 {
			any = true
			if !condxRowsAllDir(row.Group) {
				return false
			}
		}
	}
	return any
}

// refScopeProviderRuleIDs 收集规则的供给方规则 ID 集，滤除库根 sentinel
// （RefScopeLibraryRoot——非规则 ID、不构成依赖边）：供给方图的取边唯一
// 口径，构造期环检测（refScopeCyclePaths）与门段拓扑排序（阶段2）共用。
// 去重与出现序口径随 refScopeProvidersOf。
func refScopeProviderRuleIDs(rule *RuleEntry) []string {
	providers := refScopeProvidersOf(rule)
	ids := make([]string, 0, len(providers))
	for _, id := range providers {
		if id != RefScopeLibraryRoot {
			ids = append(ids, id)
		}
	}
	return ids
}

// refScopeCyclePaths 全档案「规则→规则来源供给方」图的环检测（DFS 三色标记）：
// 节点=rules（已构造规则，按传入序），边=规则 → 其供给方规则下标
// （refScopeProviderRuleIDs 取边滤除库根 sentinel；悬空引用不在 byID、不成
// 边）。返回成环节点 ID → 以该节点起讫的环路路径文本（「A → B → … → A」；
// 同一节点落在多个环上时取 DFS 首个命中）。环上每个成员各得一条以自身起讫
// 的路径——第二遍逐消费条目按此点名其参与的环路（校验③）。自引用=深度 1
// 的环亦被检出，但由校验①先行拒绝（文案不动）。按构造序与附件出现序遍历，
// 同档案结论确定。纯函数。
func refScopeCyclePaths(rules []RuleEntry, byID map[string]int) map[string]string {
	edges := make([][]int, len(rules))
	for i := range rules {
		for _, id := range refScopeProviderRuleIDs(&rules[i]) {
			if j, ok := byID[id]; ok {
				edges[i] = append(edges[i], j)
			}
		}
	}
	const (
		colorWhite = iota // 未访问
		colorGray         // 在当前 DFS 路径上
		colorBlack        // 已完结（其可达子图的环已全部记录）
	)
	color := make([]uint8, len(rules))
	cycles := map[string]string{}
	var path []int                    // 当前 DFS 路径（节点下标）
	onPath := make([]int, len(rules)) // 节点在 path 中的下标（仅 colorGray 时有效）
	var visit func(i int)
	visit = func(i int) {
		color[i] = colorGray
		path = append(path, i)
		onPath[i] = len(path) - 1
		for _, j := range edges[i] {
			switch color[j] {
			case colorWhite:
				visit(j)
			case colorGray:
				// 回边 j：path[onPath[j]:] + j 构成起讫同节点的环路。
				cyc := make([]int, 0, len(path)-onPath[j]+1)
				cyc = append(cyc, path[onPath[j]:]...)
				cyc = append(cyc, j)
				refScopeRecordCyclePath(rules, cyc, cycles)
			}
		}
		path = path[:len(path)-1]
		color[i] = colorBlack
	}
	for i := range rules {
		if color[i] == colorWhite {
			visit(i)
		}
	}
	return cycles
}

// refScopeRecordCyclePath 为环上每个成员各记一条以自身起讫的环路路径
// （cyc=环路下标序列，末位与首位同节点=起讫闭合；已有记录的成员不动——
// 同一节点多环取首个命中）。
func refScopeRecordCyclePath(rules []RuleEntry, cyc []int, cycles map[string]string) {
	members := len(cyc) - 1
	ids := make([]string, members)
	for p, idx := range cyc[:members] {
		ids[p] = rules[idx].ID
	}
	parts := make([]string, 0, members+1)
	for p := 0; p < members; p++ {
		if _, seen := cycles[ids[p]]; seen {
			continue
		}
		parts = append(parts[:0], ids[p:]...)
		parts = append(parts, ids[:p]...)
		parts = append(parts, ids[p])
		cycles[ids[p]] = strings.Join(parts, " → ")
	}
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

// —— 聚合规则的类型锚定与预置种子 ——

// AnchorAggregateRules 聚合规则的类型锚定：引用的作品类型不在资源类型注册表
// 投影（悬空——自定义类型可随其插件停用/卸载而注销）即停用该条目并返回告警
// （识别求值随之跳过；沿悬空引用「跳过+载入告警」先例）。纯函数，
// 改动只落在内存态档案对象、不回写存储——每次载入按当轮投影重新锚定；类型
// 索引缺席（查询失败/上下文不可用）时全部聚合规则按悬空处理。
func AnchorAggregateRules(profile *RuleProfile, types *resourceTypeIndex) []error {
	if profile == nil {
		return nil
	}
	var warns []error
	for i := range profile.Rules {
		rule := &profile.Rules[i]
		if rule.Category != RuleCategoryAggregate || rule.Aggregate == nil || !rule.Enabled {
			continue
		}
		if types.has(rule.Aggregate.WorkType) {
			continue
		}
		rule.Enabled = false
		label := rule.Description
		if label == "" {
			label = rule.ID
		}
		warns = append(warns, fmt.Errorf(
			"聚合规则『%s』引用的作品类型 %q 不在资源类型注册表（可能已随插件停用或卸载注销），已停用该条目",
			label, rule.Aggregate.WorkType))
	}
	return warns
}

// presetAggFileRow 构造预置种子的文件轨行：格式模板 + 扩展名谓词（模板对去
// 扩展名的文件名正文求值，扩展名由 fileExt 终结谓词承载——词汇分工）。
func presetAggFileRow(tpl, ext string) CondxRow {
	row := CondxRow{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}
	if ext != "" {
		row.Condition.Attach = []CondxAttach{{Kind: CondxAttachFileExt, Value: ext}}
	}
	return row
}

// 预置种子规则的稳定 ID（恢复预置按 ID 判缺席）。
const (
	presetAggVideoSubtitleID        = "preset-agg-video-subtitle"
	presetAggImageCompanionTextID   = "preset-agg-image-companion-text"
	presetAggArticleImageTextID     = "preset-agg-article-image-text"
	presetAggVideoDesc              = "视频+字幕聚合为 video 作品（预置）"
	presetAggImageCompanionTextDesc = "单图+说明文本→图片作品元数据（预置）"
	presetAggArticleDesc            = "一图一文本聚合为 article 作品（预置）"
)

// PresetAggregateRules 预置种子规则（三条，无特权机制——就是普通规则）：
//
//   - 种子一「视频+字幕→video」：各视频扩展与各字幕扩展各一行（视频轨行的
//     作业名模板不含扩展名——扩展名由谓词承载；字幕轨行用「{作品}.{任意}」
//     带锚形态吞掉语言段，如 ep01.zh.srt 的标识取 ep01）；
//   - 种子三「单图+说明文本→图片作品元数据」：配对形态——jpg/png/webp 图片
//     行不聚合（各自成作品），txt 说明行两条（带点行在前）捕获 {作品}，分组
//     绑定=标识分组 {作品}：说明文件与同名图片同键配对，说明内容扇出→描述
//     （001.jpg+001.txt、001.jpg+001.zh.txt 皆配对；说明文件不导入；无同名图
//     片时说明文件按独立作品导入——孤文件回退）。与图文种子的收编边界：聚合
//     归属优先（聚合段在先、扇出不夺已归属成员）——同名图片+文本对
//     （002.jpg+002.txt）归图文种子聚合收编为 article 作品，孤立 txt（无同名
//     图片对）在图文种子正文行命中下同样先行聚合收编为单正文作品；图文种子
//     停用时同名对才走本种子的配对扇出（收编边界以正反例测试锚定）；
//   - 种子二「一图一文本→article」：txt 正文行 + jpg/png/webp 内嵌图行。
//
// 首次档案初始化写入（来源=预置），此后与用户条目同权——规则管理面可见、可
// 启停、可编辑、可复制；「恢复预置」入口兜底（见 RestorePresetRules）。
func PresetAggregateRules() []RuleEntry {
	videoExts := []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".m4v", ".webm", ".flv"}
	subtitleExts := []string{".srt", ".ass", ".ssa", ".vtt"}
	videoTracks := make([]TrackAssign, 0, len(videoExts)+len(subtitleExts))
	rows := make([]CondxRow, 0, len(videoExts)+len(subtitleExts))
	for _, ext := range videoExts {
		rows = append(rows, presetAggFileRow("{作品}", ext))
		videoTracks = append(videoTracks, TrackAssign{Row: len(rows), Role: contract.StoreTypeVideoMain})
	}
	for _, ext := range subtitleExts {
		rows = append(rows, presetAggFileRow("{作品}.{任意}", ext))
		videoTracks = append(videoTracks, TrackAssign{Row: len(rows), Role: contract.StoreTypeSubtitle})
	}
	articleExts := []struct {
		ext  string
		role string
	}{
		{".txt", contract.StoreTypeDocument},
		{".jpg", contract.StoreTypeImage}, {".png", contract.StoreTypeImage}, {".webp", contract.StoreTypeImage},
	}
	articleRows := make([]CondxRow, 0, len(articleExts))
	articleTracks := make([]TrackAssign, 0, len(articleExts))
	for _, e := range articleExts {
		articleRows = append(articleRows, presetAggFileRow("{作品}", e.ext))
		articleTracks = append(articleTracks, TrackAssign{Row: len(articleRows), Role: e.role})
	}
	// 单图+说明文本（配对形态）：图片行各扩展一行（作品承载方，各自成作品），
	// txt 说明行两条且带点行在前——说明行以 {作品} 捕获与同名图片配对：
	// 001.zh.txt 由带点行命中、001.txt 由不带点行命中，作品皆取 001。带点行
	// 必须在前：{作品} 的取值可吞点，跨行取值又取首个能供值的命中行，不带点
	// 行在前时 001.zh.txt 会被它先命中并取成 001.zh，与图片行的 001 不同键、
	// 配对失败。两条说明行都内容绑定→描述，按标识分组 {作品} 把说明内容扇出
	// 给同名图片各自成的作品（说明文件不导入不独立成件；无同名图片时说明文
	// 件按独立作品导入——孤文件回退），无聚合产出。
	// 行前连接词显式落 and：聚合规则的空连接词归一是聚合校验的特权（轨行
	// 平行选择器），元数据类别行式底盘要求非首行显式携带。
	imageCompanionExts := []string{".jpg", ".png", ".webp"}
	imageCompanionRows := make([]CondxRow, 0, len(imageCompanionExts)+2)
	for _, ext := range imageCompanionExts {
		row := presetAggFileRow("{作品}", ext)
		if len(imageCompanionRows) > 0 {
			row.Connector = "and"
		}
		imageCompanionRows = append(imageCompanionRows, row)
	}
	for _, tpl := range []string{"{作品}.{任意}", "{作品}"} {
		row := presetAggFileRow(tpl, ".txt")
		row.Connector = "and"
		imageCompanionRows = append(imageCompanionRows, row)
	}
	return []RuleEntry{
		{
			ID:          presetAggVideoSubtitleID,
			Description: presetAggVideoDesc,
			Source:      RuleSourcePreset,
			Enabled:     true,
			Category:    RuleCategoryAggregate,
			Conditions:  rows,
			Identity:    []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{
				WorkType:     sdkdto.ResourceTypeVideo,
				Tracks:       videoTracks,
				CompanionUse: CompanionUseTrack,
			},
		},
		{
			ID:          presetAggImageCompanionTextID,
			Description: presetAggImageCompanionTextDesc,
			Source:      RuleSourcePreset,
			Enabled:     true,
			Category:    RuleCategoryMetadata,
			Conditions:  imageCompanionRows,
			// 配对形态：无聚合产出、无标识声明（聚合专属），「元数据文件」
			// 产出绑标识分组 {作品}（行 1 的捕获名 work；取值按捕获名跨行汇
			// 流，说明行命中的文件从自己的命中行取同名捕获）——两条说明行
			// （末两行 txt）内容绑定→描述，扇出给同名图片各自成的作品。
			MetadataFile: &MetadataFileProduction{
				Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}}},
				ContentBindings: []ContentBinding{
					{Row: len(imageCompanionRows) - 1, Dimension: DimDescription},
					{Row: len(imageCompanionRows), Dimension: DimDescription},
				},
			},
		},
		{
			ID:          presetAggArticleImageTextID,
			Description: presetAggArticleDesc,
			Source:      RuleSourcePreset,
			Enabled:     true,
			Category:    RuleCategoryAggregate,
			Conditions:  articleRows,
			Identity:    []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{
				WorkType:     sdkdto.ResourceTypeArticle,
				Tracks:       articleTracks,
				CompanionUse: CompanionUseTrack,
			},
		},
	}
}

// EnsurePresetSeeded 首次档案初始化的种子入档：档案键缺席（从未写过档案）时
// 写入含全部预置种子的档案并返回 true；已有档案（含用户清空后的空档案——
// 键在场）不动、返回 false。写入走 SaveProfile 常规路径（首次写入不产生
// 备份）。激活期调用（档案初始化点）。
func EnsurePresetSeeded(store ProfileStore) (bool, error) {
	if store == nil {
		return false, fmt.Errorf("规则档案存储不可用，无法初始化预置种子")
	}
	stored, err := store.GetValue(RuleProfileKVKey)
	if err != nil {
		return false, fmt.Errorf("读取规则档案（预置种子初始化）失败: %w", err)
	}
	if stored != nil && stored.Value != "" {
		return false, nil
	}
	profile := emptyProfile()
	profile.Rules = append(profile.Rules, PresetAggregateRules()...)
	if err := SaveProfile(store, profile); err != nil {
		return false, fmt.Errorf("写入预置种子规则失败: %w", err)
	}
	return true, nil
}

// RestorePresetRules 恢复预置：把档案中缺席的预置种子条目按 ID 补回（误删
// 兜底），返回补回条数。在场条目——含用户改写过的——一律不动（恢复动作只
// 补缺席，不覆写用户改造；改坏的条目先删除再恢复即回出厂形态）。档案键
// 缺席时等同首次初始化（全量入档）。载入逐条告警不在本出口重复上报——它们
// 由后续每次常规载入再现。
func RestorePresetRules(store ProfileStore) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("规则档案存储不可用，无法恢复预置")
	}
	profile, _ := LoadProfile(store)
	if profile == nil {
		return 0, fmt.Errorf("规则档案不可载入，无法恢复预置")
	}
	present := make(map[string]bool, len(profile.Rules))
	for _, rule := range profile.Rules {
		present[rule.ID] = true
	}
	added := 0
	for _, seed := range PresetAggregateRules() {
		if present[seed.ID] {
			continue
		}
		profile.Rules = append(profile.Rules, seed)
		added++
	}
	if added == 0 {
		return 0, nil
	}
	if err := SaveProfile(store, profile); err != nil {
		return 0, fmt.Errorf("写入恢复的预置规则失败: %w", err)
	}
	return added, nil
}
