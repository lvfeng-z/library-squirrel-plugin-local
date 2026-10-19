// 规则模式类型树：目标 / 归属 / 谓词枚举，Pattern 接口与各具体模式的构造和求值；纯模型，无 IO 与日志。
package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// RegexTarget 为正则模式的匹配目标类别。
type RegexTarget string

const (
	// TargetFileName 表示匹配文件名（含扩展名）。
	TargetFileName RegexTarget = "filename"
	// TargetDirName 表示逐层匹配目录名。
	TargetDirName RegexTarget = "dirname"
	// TargetRelPath 表示匹配以 '/' 分隔的相对路径。
	TargetRelPath RegexTarget = "relpath"
)

// GroupOutcome 为群组成员的归属结果类别。
type GroupOutcome string

const (
	// OutcomeCollection 表示群组归入同一作品集。
	OutcomeCollection GroupOutcome = "collection"
	// OutcomeIndependent 表示落单条目作为独立作品。
	OutcomeIndependent GroupOutcome = "independent"
	// OutcomeMultiStoreWork 表示群组归入单作品多 store；预留位，本阶段不实现。
	OutcomeMultiStoreWork GroupOutcome = "multiStoreWork"
)

// PredicateKind 为谓词类别。
type PredicateKind string

const (
	// PredicateAuthor 表示按作者名查询库内数据。
	PredicateAuthor PredicateKind = "author"
	// PredicateTag 表示按标签名查询库内数据。
	PredicateTag PredicateKind = "tag"
	// PredicateWorkSet 表示按作品集名查询库内数据。
	PredicateWorkSet PredicateKind = "workSet"
)

// Pattern 为规则模式接口：Evaluate 命中返回 true，并把命中的绑定追加进 ctx。
type Pattern interface {
	// Evaluate 对输入 in 求值；命中返回 true 并把绑定追加进 ctx。
	Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool
	// patternMarker 为模式类型树的私有标记方法。
	patternMarker()
}

// RegexPattern 为正则模式：按目标类别匹配文件名 / 目录名 / 相对路径。
type RegexPattern struct {
	// Target 为匹配目标类别。
	Target RegexTarget `json:"target"`
	// Expr 为正则表达式原文。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表。
	Captures []string `json:"captures"`
	// re 为表达式的编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// NewRegexPattern 构造正则模式：目标类别未知、或表达式编译失败（反向引用 / lookahead
// 等 RE2 不支持的形态在此被拦截）时，返回含原文与底层错误的中文错误。
func NewRegexPattern(target, expr string, captures []string) (*RegexPattern, error) {
	switch RegexTarget(target) {
	case TargetFileName, TargetDirName, TargetRelPath:
	default:
		return nil, fmt.Errorf("未知的正则匹配目标: %q", target)
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("正则表达式 %q 编译失败: %w", expr, err)
	}
	return &RegexPattern{Target: RegexTarget(target), Expr: expr, Captures: captures, re: re}, nil
}

// patternMarker 实现 Pattern 接口标记。
func (p *RegexPattern) patternMarker() {}

// Evaluate 按目标类别求值：filename / relpath 命中时追加命名捕获绑定；
// dirname 逐层试目录名，任一层命中追加 DirHit（不追加捕获组）并返回 true。
func (p *RegexPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil {
		return false
	}
	switch p.Target {
	case TargetFileName:
		return p.matchWithCaptures(in.FileName, ctx)
	case TargetRelPath:
		return p.matchWithCaptures(in.RelPath, ctx)
	case TargetDirName:
		for depth, name := range in.DirNames {
			if p.re.MatchString(name) {
				ctx.DirHits = append(ctx.DirHits, DirHit{Depth: depth, Name: name})
				return true
			}
		}
		return false
	default:
		return false
	}
}

// matchWithCaptures 对 text 求一次子匹配：命中时把 Captures 列出的命名捕获组
// 逐个按本次命中值追加为绑定（表达式中不存在的组名绑定空串），返回 true。
func (p *RegexPattern) matchWithCaptures(text string, ctx *ParseContext) bool {
	m := p.re.FindStringSubmatch(text)
	if m == nil {
		return false
	}
	groupNames := p.re.SubexpNames()
	for _, name := range p.Captures {
		value := ""
		for i, groupName := range groupNames {
			if i > 0 && groupName == name {
				value = m[i]
				break
			}
		}
		ctx.Captures = append(ctx.Captures, CaptureBinding{Name: name, Value: value})
	}
	return true
}

// LocationPattern 为位置模式：在目录深度区间内逐层以 Ancestor 匹配目录名。
type LocationPattern struct {
	// MinDepth 为待试目录的最小深度（含）。
	MinDepth int `json:"minDepth"`
	// MaxDepth 为待试目录的最大深度（含）；小于 0 视为不限上界。
	MaxDepth int `json:"maxDepth"`
	// Ancestor 为作用于单层目录名的子模式。
	Ancestor Pattern `json:"ancestor"`
}

// NewLocationPattern 构造位置模式：MinDepth 为负或 Ancestor 为空时返回中文错误。
func NewLocationPattern(minDepth, maxDepth int, ancestor Pattern) (*LocationPattern, error) {
	if minDepth < 0 {
		return nil, fmt.Errorf("MinDepth 不能为负: %d", minDepth)
	}
	if ancestor == nil {
		return nil, fmt.Errorf("Ancestor 不能为空")
	}
	return &LocationPattern{MinDepth: minDepth, MaxDepth: maxDepth, Ancestor: ancestor}, nil
}

// patternMarker 实现 Pattern 接口标记。
func (p *LocationPattern) patternMarker() {}

// Evaluate 对 [MinDepth, MaxDepth] ∩ [0, len(DirNames)-1] 内每层目录构造只含
// 该目录名的子输入，用临时上下文求值 Ancestor（子绑定不进主 ctx）；任一层命中
// 即向主 ctx 追加 DirHit 并返回 true；全部层不中返回 false。
func (p *LocationPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.Ancestor == nil {
		return false
	}
	start := p.MinDepth
	if start < 0 {
		start = 0
	}
	end := p.MaxDepth
	if end < 0 || end >= len(in.DirNames) {
		end = len(in.DirNames) - 1
	}
	for depth := start; depth <= end; depth++ {
		sub := &MatchInput{FileName: in.DirNames[depth]}
		tmp := &ParseContext{}
		if p.Ancestor.Evaluate(sub, lib, tmp) {
			ctx.DirHits = append(ctx.DirHits, DirHit{Depth: depth, Name: in.DirNames[depth]})
			return true
		}
	}
	return false
}

// SiblingPattern 为兄弟群组模式：提取分组键与序号，统计同目录内同键且序号
// 可提取的兄弟数目，达到 MinGroupSize 即视为成群。
type SiblingPattern struct {
	// KeyExpr 为分组键提取正则；空串表示键取去扩展名的文件名。
	KeyExpr string `json:"keyExpr"`
	// SeqExpr 为序号提取正则，至少包含一个捕获组。
	SeqExpr string `json:"seqExpr"`
	// MinGroupSize 为成群的最少成员数（Siblings 含自身）。
	MinGroupSize int `json:"minGroupSize"`
	// Outcome 为命中群组后的归属结果类别；求值阶段只产出群组分析，归属由消费方按此字段决定。
	Outcome GroupOutcome `json:"outcome"`
	// keyRe 为 KeyExpr 的编译缓存；KeyExpr 为空时为 nil。
	keyRe *regexp.Regexp `json:"-"`
	// seqRe 为 SeqExpr 的编译缓存。
	seqRe *regexp.Regexp `json:"-"`
}

// NewSiblingPattern 构造兄弟群组模式：两个正则编译失败、或 SeqExpr 缺少捕获组时返回中文错误。
func NewSiblingPattern(keyExpr, seqExpr string, minGroupSize int, outcome GroupOutcome) (*SiblingPattern, error) {
	seqRe, err := regexp.Compile(seqExpr)
	if err != nil {
		return nil, fmt.Errorf("序号正则 %q 编译失败: %w", seqExpr, err)
	}
	if seqRe.NumSubexp() < 1 {
		return nil, fmt.Errorf("序号正则 %q 必须包含至少一个捕获组", seqExpr)
	}
	var keyRe *regexp.Regexp
	if keyExpr != "" {
		keyRe, err = regexp.Compile(keyExpr)
		if err != nil {
			return nil, fmt.Errorf("分组键正则 %q 编译失败: %w", keyExpr, err)
		}
	}
	return &SiblingPattern{KeyExpr: keyExpr, SeqExpr: seqExpr, MinGroupSize: minGroupSize, Outcome: outcome, keyRe: keyRe, seqRe: seqRe}, nil
}

// patternMarker 实现 Pattern 接口标记。
func (p *SiblingPattern) patternMarker() {}

// Evaluate 先提取自身键与序号（序号不可提取返回 false），再统计 Siblings 中
// 同键且序号可提取者的数目 size；size 达到 MinGroupSize 时写入 GroupAnalysis
// （Prefix 取键）并返回 true，否则返回 false（落单）。
func (p *SiblingPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.seqRe == nil {
		return false
	}
	key := p.groupKey(in.FileName)
	seq, ok := p.seqOf(in.FileName)
	if !ok {
		return false
	}
	size := 0
	for _, sibling := range in.Siblings {
		if p.groupKey(sibling) != key {
			continue
		}
		if _, ok := p.seqOf(sibling); ok {
			size++
		}
	}
	if size < p.MinGroupSize {
		return false
	}
	ctx.Group = &GroupAnalysis{Key: key, Prefix: key, Seq: seq, Size: size}
	return true
}

// groupKey 提取文件名的分组键：KeyExpr 命中且含捕获组时取第一个分组文本；
// 无 KeyExpr 或不匹配时取去扩展名的文件名。
func (p *SiblingPattern) groupKey(fileName string) string {
	if p.keyRe != nil {
		if m := p.keyRe.FindStringSubmatch(fileName); len(m) >= 2 {
			return m[1]
		}
	}
	return BaseNameWithoutExt(fileName)
}

// seqOf 提取文件名的序号：SeqExpr 命中且第一个捕获组可解析为整数时返回 (序号, true)。
func (p *SiblingPattern) seqOf(fileName string) (int, bool) {
	text, ok := p.seqTextOf(fileName)
	if !ok {
		return 0, false
	}
	seq, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}
	return seq, true
}

// seqTextOf 提取文件名序号的原文（第一个捕获组文本）：供识别全景呈现序号形态
// （零填充与否由原文的前导零体现，宽度信息在整数转换中会丢失）。
func (p *SiblingPattern) seqTextOf(fileName string) (string, bool) {
	if p.seqRe == nil {
		return "", false
	}
	m := p.seqRe.FindStringSubmatch(fileName)
	if len(m) < 2 {
		return "", false
	}
	return m[1], true
}

// PredicatePattern 为谓词模式：以文件名去扩展名或某捕获组绑定的值为名字，查询库内集合。
type PredicatePattern struct {
	// Kind 为谓词类别（author / tag / workSet）。
	Kind PredicateKind `json:"kind"`
	// From 为取值来源：空串表示文件名去扩展名，否则为捕获组名。
	From string `json:"from"`
}

// patternMarker 实现 Pattern 接口标记。
func (p *PredicatePattern) patternMarker() {}

// Evaluate 取名字（From 为空时取文件名去扩展名，否则取该捕获组名的最后一条绑定，
// 绑定不存在返回 false），按 Kind 查询 lib 的作者 / 标签 / 作品集集合；命中时向 ctx
// 追加 PredicateHit 并返回 true。
func (p *PredicatePattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	var name string
	if p.From == "" {
		name = BaseNameWithoutExt(in.FileName)
	} else {
		value, ok := ctx.Capture(p.From)
		if !ok {
			return false
		}
		name = value
	}
	if lib == nil {
		return false
	}
	var names map[string]struct{}
	switch p.Kind {
	case PredicateAuthor:
		names = lib.AuthorNames
	case PredicateTag:
		names = lib.TagNames
	case PredicateWorkSet:
		names = lib.WorkSetNames
	default:
		return false
	}
	if _, ok := names[name]; !ok {
		return false
	}
	ctx.PredHits = append(ctx.PredHits, PredicateHit{Kind: string(p.Kind), Input: name, Name: name})
	return true
}

// And 为合取模式：全部子模式命中才命中；短路求值。
type And []Pattern

// patternMarker 实现 Pattern 接口标记。
func (And) patternMarker() {}

// Evaluate 依次求值子模式，遇第一个未命中即返回 false；全真返回 true。
func (p And) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	for _, sub := range p {
		if !sub.Evaluate(in, lib, ctx) {
			return false
		}
	}
	return true
}

// Or 为析取模式：任一子模式命中即命中；短路求值。
type Or []Pattern

// patternMarker 实现 Pattern 接口标记。
func (Or) patternMarker() {}

// Evaluate 依次求值子模式，遇第一个命中即返回 true；全假返回 false。
func (p Or) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	for _, sub := range p {
		if sub.Evaluate(in, lib, ctx) {
			return true
		}
	}
	return false
}

// Not 为取反模式：对内层模式的求值结果取反。
type Not struct {
	// Inner 为被取反的内层模式。
	Inner Pattern `json:"inner"`
}

// patternMarker 实现 Pattern 接口标记。
func (Not) patternMarker() {}

// Evaluate 对内层结果取反；内层为空时恒不命中。
func (p Not) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.Inner == nil {
		return false
	}
	return !p.Inner.Evaluate(in, lib, ctx)
}
