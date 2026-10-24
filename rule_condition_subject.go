// v4 条件主体模式：文件/目录/序列三类主体的格式段求值（条件构建语法硬化
// 期1 第一步）。终结谓词（子女/父目录断言、类型、扩展名、大小）与值级小标
// （名单形变归一/跨行相等/常量）在后续硬化步并入——编译入口对未实现件显式
// 报错，本文件只承载已实现件。纯模型，无 IO 与日志。
package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// FileSubjectPattern 文件主体模式：格式模板匹配**去扩展名的文件名**（扩展名
// 由 fileExt 终结谓词承载，不混入模板——收敛档 R6 后的词汇分工）。
type FileSubjectPattern struct {
	// Expr 为推导出的锚定正则原文。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表。
	Captures []string `json:"captures"`
	// re 为编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// newFileSubjectPattern 构造文件主体模式：正则编译失败返回中文错误。
func newFileSubjectPattern(expr string, captures []string) (*FileSubjectPattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("文件格式正则 %q 编译失败: %w", expr, err)
	}
	return &FileSubjectPattern{Expr: expr, Captures: captures, re: re}, nil
}

// patternMarker 实现 Pattern 接口标记。
func (p *FileSubjectPattern) patternMarker() {}

// Evaluate 对去扩展名的文件名求锚定匹配，命中时绑定命名捕获组并返回 true。
func (p *FileSubjectPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil {
		return false
	}
	name := BaseNameWithoutExt(in.FileName)
	return bindCondxCaptures(p.re, name, p.Captures, ctx)
}

// DirSubjectPattern 目录主体模式：格式（模板或包含词）匹配**任一层祖先目录名**
// （子树包含语义，任意深度），任一层命中即绑定该层捕获并记 DirHit。
type DirSubjectPattern struct {
	// Expr 为推导出的锚定正则原文（模板推导或包含词推导）。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表（contains 问法为空）。
	Captures []string `json:"captures"`
	// re 为编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// newDirSubjectPattern 构造目录主体模式：正则编译失败返回中文错误。
func newDirSubjectPattern(expr string, captures []string) (*DirSubjectPattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("目录格式正则 %q 编译失败: %w", expr, err)
	}
	return &DirSubjectPattern{Expr: expr, Captures: captures, re: re}, nil
}

// patternMarker 实现 Pattern 接口标记。
func (p *DirSubjectPattern) patternMarker() {}

// Evaluate 逐层（浅→深）试祖先目录名，首个命中层绑定命名捕获组、追加 DirHit
// 并返回 true；全层不中返回 false。
func (p *DirSubjectPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil {
		return false
	}
	for depth, name := range in.DirNames {
		if p.re.MatchString(name) {
			bindCondxCaptures(p.re, name, p.Captures, ctx)
			ctx.DirHits = append(ctx.DirHits, DirHit{Depth: depth, Name: name})
			return true
		}
	}
	return false
}

// SeqSubjectPattern 序列主体模式：成员格式模板（含 {序号}，可选 {键}）逐成员
// 全名匹配；**非{序号}捕获段的拼接文本为群组键**——跨成员键相同即同套（收敛
// 档 R3：模板的其余部分跨成员恒同，这正是「成套」的语义）。达到 MinSize 记
// GroupAnalysis 并绑定捕获（键/序号等）。
type SeqSubjectPattern struct {
	// Expr 为成员格式模板推导出的锚定正则原文。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表（含 seq 与键/其他占位词）。
	Captures []string `json:"captures"`
	// SeqName 为序号捕获组名（恒为 "seq"）。
	SeqName string `json:"seqName"`
	// KeyParts 为键段捕获组名列表（模板中除 {序号} 外的全部占位词，按模板顺序）。
	KeyParts []string `json:"keyParts"`
	// MinSize 为成套最少成员数（含自身，默认 2）。
	MinSize int `json:"minSize"`
	// re 为编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// newSeqSubjectPattern 构造序列主体模式：正则编译失败、模板无 {序号} 或无键段
// （模板仅含 {序号} 时键为空串，任何同型数字名都会成套——允许但语义弱，
// 由校验层要求模板非纯序号）返回中文错误。
func newSeqSubjectPattern(expr string, captures []string, minSize int) (*SeqSubjectPattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("序列成员正则 %q 编译失败: %w", expr, err)
	}
	hasSeq := false
	var keyParts []string
	for _, name := range captures {
		if name == "seq" {
			hasSeq = true
			continue
		}
		keyParts = append(keyParts, name)
	}
	if !hasSeq {
		return nil, fmt.Errorf("序列成员格式缺少 {序号} 捕获组")
	}
	if minSize < 2 {
		minSize = 2
	}
	return &SeqSubjectPattern{Expr: expr, Captures: captures, SeqName: "seq", KeyParts: keyParts, MinSize: minSize, re: re}, nil
}

// patternMarker 实现 Pattern 接口标记。
func (p *SeqSubjectPattern) patternMarker() {}

// seqMemberKey 对单个成员名求模板匹配：命中返回 (键=非序号捕获段按序拼接,
// 序号文本, true)；不匹配或序号非整数返回零值与 false。
func (p *SeqSubjectPattern) seqMemberKey(name string) (string, string, bool) {
	m := p.re.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	groupIdx := make(map[string]int, len(p.re.SubexpNames()))
	for i, g := range p.re.SubexpNames() {
		if i > 0 && g != "" {
			groupIdx[g] = i
		}
	}
	seqIdx, ok := groupIdx[p.SeqName]
	if !ok || seqIdx >= len(m) {
		return "", "", false
	}
	if _, err := strconv.Atoi(m[seqIdx]); err != nil {
		return "", "", false
	}
	var key string
	for _, part := range p.KeyParts {
		if idx, ok := groupIdx[part]; ok && idx < len(m) {
			key += m[idx]
		}
	}
	return key, m[seqIdx], true
}

// Evaluate 先对自身求成员键与序号，再统计 Siblings（含自身）中同键成员数；
// 达到 MinSize 时绑定自身捕获、写入 GroupAnalysis（Key/Prefix 取键、Seq 取
// 序号整数、Size 取同键成员数）并返回 true；落单返回 false。
func (p *SeqSubjectPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil {
		return false
	}
	selfName := BaseNameWithoutExt(in.FileName)
	key, seqText, ok := p.seqMemberKey(selfName)
	if !ok {
		return false
	}
	size := 0
	for _, sibling := range in.Siblings {
		if k, _, ok := p.seqMemberKey(BaseNameWithoutExt(sibling)); ok && k == key {
			size++
		}
	}
	if size < p.MinSize {
		return false
	}
	bindCondxCaptures(p.re, selfName, p.Captures, ctx)
	seq, _ := strconv.Atoi(seqText)
	ctx.Group = &GroupAnalysis{Key: key, Prefix: key, Seq: seq, Size: size}
	return true
}

// bindCondxCaptures 对 text 求一次子匹配并绑定命名捕获组：命中时把 Captures
// 列出的组按本次命中值追加为绑定（不存在的组名绑定空串），返回 true；不命中
// 返回 false 且不绑定。绑定语义与 RegexPattern.matchWithCaptures 一致（区间
// 供贪婪识别「未覆盖片段」复用；v4 主体捕获沿用同一绑定空间）。
func bindCondxCaptures(re *regexp.Regexp, text string, names []string, ctx *ParseContext) bool {
	idx := re.FindStringSubmatchIndex(text)
	if idx == nil {
		return false
	}
	groupNames := re.SubexpNames()
	for _, name := range names {
		binding := CaptureBinding{Name: name, Segment: text}
		for i, groupName := range groupNames {
			if i > 0 && groupName == name {
				s, e := idx[2*i], idx[2*i+1]
				if s >= 0 && e >= 0 {
					binding.Value = text[s:e]
					binding.Start, binding.End = s, e
				}
				break
			}
		}
		ctx.Captures = append(ctx.Captures, binding)
	}
	return true
}
