// 规则档案模型：含义维度 / 来源 / 提取 / 合成的类型定义，档案的多态 JSON 序列化与载入校验；纯模型，无 IO 与日志。
package main

import (
	"encoding/json"
	"fmt"
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
	// RuleSourceQna 表示问答向导生成的规则。
	RuleSourceQna RuleSource = "qna"
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

// ExtractionAction 为一条提取动作：规则命中后从解析上下文取值并声明到某含义维度。
type ExtractionAction struct {
	// Dimension 为声明到的含义维度。
	Dimension MetadataDimension `json:"dimension"`
	// From 为取值来源。
	From ExtractionFrom `json:"from"`
	// CaptureName 为 From 为 capture 时的捕获组名。
	CaptureName string `json:"captureName"`
	// Constant 为 From 为 constant 时的常量值。
	Constant string `json:"constant"`
	// RoleName 为声明级角色绑定，可为空；原样透传到产出声明。
	RoleName string `json:"roleName"`
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
const CurrentSchemaVersion = 1

// RuleProfile 为规则档案：一批规则的容器。
type RuleProfile struct {
	// SchemaVersion 为档案结构版本，载入时校验。
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
	// PredKind 为 predicate 的谓词类别（author / tag）；判别字段已占用 kind，故另名序列化。
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
		case PredicateAuthor, PredicateTag:
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

// ruleFromJSON 把规则中间结构构造回 RuleEntry：模式缺失或构造校验失败返回中文错误。
func ruleFromJSON(rj *ruleEntryJSON) (*RuleEntry, error) {
	if rj.Pattern == nil {
		return nil, fmt.Errorf("规则 %q 缺少模式（pattern）", rj.ID)
	}
	pat, err := patternFromJSON(rj.Pattern)
	if err != nil {
		return nil, fmt.Errorf("规则 %q 的模式非法: %w", rj.ID, err)
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

// MarshalJSON 把档案序列化为带 kind 判别字段的多态 JSON。
func (p RuleProfile) MarshalJSON() ([]byte, error) {
	out := ruleProfileJSON{SchemaVersion: p.SchemaVersion, Rules: make([]ruleEntryJSON, 0, len(p.Rules))}
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
	profile := RuleProfile{SchemaVersion: in.SchemaVersion, Rules: make([]RuleEntry, 0, len(in.Rules))}
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
// 数据不可解析、或结构版本与当前版本不一致时返回 nil 档案与单个错误；
// 个别规则构造校验失败（正则编译、深度约束等）只产生含规则 ID 与描述的
// 中文错误并跳过该规则，不阻塞其余规则载入；无错误时错误列表为空。
func ParseProfile(data []byte) (*RuleProfile, []error) {
	var in ruleProfileJSON
	if err := json.Unmarshal(data, &in); err != nil {
		return nil, []error{fmt.Errorf("规则档案反序列化失败: %w", err)}
	}
	if in.SchemaVersion != CurrentSchemaVersion {
		return nil, []error{fmt.Errorf("规则档案结构版本为 %d，与当前支持的版本 %d 不一致", in.SchemaVersion, CurrentSchemaVersion)}
	}
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: make([]RuleEntry, 0, len(in.Rules))}
	var errs []error
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
