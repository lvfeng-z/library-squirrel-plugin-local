// v4 条件主体模式：文件/目录/序列三类主体的格式段与随主体终结谓词求值
// （条件构建语法硬化期1 第二步）。目录谓词作用于**命中层目录**的子女与父目录
// （R4：仅上/下各 1 级；R5：迷你格式为终结格式，只匹配不再挂条件）。纯模型，
// 无 IO 与日志。
package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// FileSubjectPattern 文件主体模式：格式模板匹配**去扩展名的文件名**（扩展名
// 由 fileExt 终结谓词承载，不混入模板——收敛档 R6 后的词汇分工）。
type FileSubjectPattern struct {
	// Expr 为推导出的锚定正则原文。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表。
	Captures []string `json:"captures"`
	// Ext 为 fileExt 终结谓词的扩展名（已归一为小写带点形，如 ".jpg"）；
	// 空串 = 无该谓词。
	Ext string `json:"ext,omitempty"`
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

// Evaluate 对去扩展名的文件名求锚定匹配；fileExt 谓词（如有）同时校验扩展名
// （大小写不敏感，容忍无点写法）。命中时绑定命名捕获组并返回 true。
func (p *FileSubjectPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil {
		return false
	}
	if p.Ext != "" && !extMatches(in.FileName, p.Ext) {
		return false
	}
	name := BaseNameWithoutExt(in.FileName)
	return bindCondxCaptures(p.re, name, p.Captures, ctx)
}

// extMatches 判断文件名扩展名是否等于 want（want 已归一小写带点形）。
func extMatches(fileName, want string) bool {
	ext := strings.ToLower(extOf(fileName))
	if ext == "" {
		return false
	}
	if ext == want {
		return true
	}
	// 无点写法（"jpg"）与带点形等价。
	return "."+ext == want
}

// extOf 返回小写扩展名（带点）；无扩展名返回空串。
func extOf(fileName string) string {
	i := strings.LastIndexByte(fileName, '.')
	if i < 0 {
		return ""
	}
	return strings.ToLower(fileName[i:])
}

// fileTypeClassOf 按扩展名归类为终结谓词的类型词表（image / video / document /
// archive；不可归类返回 "other"，不与任何指定类型匹配）。
func fileTypeClassOf(fileName string) string {
	switch extOf(fileName) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".avif", ".heic", ".tiff":
		return "image"
	case ".mp4", ".mov", ".avi", ".mkv", ".webm", ".flv", ".wmv", ".m4v":
		return "video"
	case ".pdf", ".txt", ".md", ".doc", ".docx", ".rtf", ".odt", ".pages":
		return "document"
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".cbz", ".cbr":
		return "archive"
	default:
		return "other"
	}
}

// DirAssert 为目录主体上的终结谓词实例（编译自 CondxAttach，目标=dir）。
type DirAssert struct {
	// Kind 为谓词类别（childrenAllFormat / childrenAtLeastN / childDirCount /
	// childTypeIs / parentFormat / parentContains）。
	Kind string `json:"kind"`
	// re 为迷你格式推导正则（childrenAllFormat / childrenAtLeastN 匹配子女
	// 去扩展名基名；parentFormat 匹配父目录名）。终结格式不再捕获（R5）。
	re *regexp.Regexp `json:"-"`
	// MinCount 为数量门槛（≥）。
	MinCount int `json:"minCount,omitempty"`
	// Types 为类型词表（childTypeIs）。
	Types []string `json:"types,omitempty"`
	// Word 为包含词（parentContains）。
	Word string `json:"word,omitempty"`
}

// check 对「深度 depth 的命中目录」校验全部谓词；任一不满足返回 false。
// 子女数据缺失（构造点未注入 AncestorFiles）时子女类谓词保守不命中。
func (a DirAssert) check(in *MatchInput, depth int) bool {
	switch a.Kind {
	case CondxAttachChildrenAllFormat, CondxAttachChildrenAtLeastN:
		children, ok := in.ancestorFilesAt(depth)
		if !ok {
			return false
		}
		if a.Kind == CondxAttachChildrenAllFormat {
			if len(children) < a.MinCount {
				return false
			}
			for _, child := range children {
				if !a.re.MatchString(BaseNameWithoutExt(child)) {
					return false
				}
			}
			return true
		}
		count := 0
		for _, child := range children {
			if a.re.MatchString(BaseNameWithoutExt(child)) {
				count++
			}
		}
		return count >= a.MinCount
	case CondxAttachChildDirCount:
		count, ok := in.ancestorChildDirCountAt(depth)
		if !ok {
			return false
		}
		return count >= a.MinCount
	case CondxAttachChildTypeIs:
		children, ok := in.ancestorFilesAt(depth)
		if !ok {
			return false
		}
		allowed := make(map[string]bool, len(a.Types))
		for _, t := range a.Types {
			allowed[t] = true
		}
		for _, child := range children {
			if !allowed[fileTypeClassOf(child)] {
				return false
			}
		}
		return true
	case CondxAttachParentFormat:
		if depth < 1 {
			return false
		}
		return a.re.MatchString(in.DirNames[depth-1])
	case CondxAttachParentContains:
		if depth < 1 {
			return false
		}
		return strings.Contains(in.DirNames[depth-1], a.Word)
	default:
		return false
	}
}

// DirSubjectPattern 目录主体模式：格式（模板或包含词）匹配**任一层祖先目录名**
// （子树包含语义，任意深度），且该层目录满足全部终结谓词时命中：绑定该层捕获、
// 记 DirHit。谓词把「格式像」收紧为「结构像」（如 作品目录：作品目录里全是
// {序号} 子女）。
type DirSubjectPattern struct {
	// Expr 为推导出的锚定正则原文（模板推导或包含词推导）。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表（contains 问法为空）。
	Captures []string `json:"captures"`
	// Asserts 为随主体的终结谓词实例列表。
	Asserts []DirAssert `json:"asserts,omitempty"`
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

// Evaluate 逐层（深→浅——最具体的祖先层优先：全通配目录模板（如整目录名
// 捕获 {集名}）须绑文件直接父层而非更浅的任意匹配层；浅层形态模板在深层
// 不匹配时自然回退浅层命中）试祖先目录名，格式与全部终结谓词同时满足的首
// 个命中层绑定该层捕获、追加 DirHit 并返回 true；全层不中返回 false。
func (p *DirSubjectPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil {
		return false
	}
	for depth := len(in.DirNames) - 1; depth >= 0; depth-- {
		name := in.DirNames[depth]
		if !p.re.MatchString(name) {
			continue
		}
		ok := true
		for _, assert := range p.Asserts {
			if !assert.check(in, depth) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		bindCondxCaptures(p.re, name, p.Captures, ctx)
		ctx.DirHits = append(ctx.DirHits, DirHit{Depth: depth, Name: name})
		return true
	}
	return false
}

// SeqSubjectPattern 序列主体模式：成员格式模板（含 {序号}，可选 {键}）逐成员
// 全名匹配；**非{序号}捕获段的拼接文本为群组键**——跨成员键相同即同套（收敛
// 档 R3：模板的其余部分跨成员恒同，这正是「成套」的语义）。达到 MinSize 记
// GroupAnalysis 并绑定捕获（键/序号等）；MemberTypes 非空时只统计指定类型的
// 成员（seqMemberType 谓词，T3 散页成套排除目录内的说明 txt 等）。
type SeqSubjectPattern struct {
	// Expr 为成员格式模板推导出的锚定正则原文。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表（含 seq 与键/其他占位词）。
	Captures []string `json:"captures"`
	// SeqName 为序号捕获组名（恒为 "seq"）。
	SeqName string `json:"seqName"`
	// KeyParts 为键段捕获组名列表（模板中除 {序号} 外的全部占位词，按模板顺序）。
	KeyParts []string `json:"keyParts"`
	// MinSize 为成套最少成员数（含自身，默认 2；seqMinLength 谓词）。
	MinSize int `json:"minSize"`
	// MemberTypes 为成员类型限定（seqMemberType 谓词；空 = 不限类型）。
	MemberTypes []string `json:"memberTypes,omitempty"`
	// re 为编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// newSeqSubjectPattern 构造序列主体模式：正则编译失败或模板无 {序号} 捕获组
// 返回中文错误。
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

// memberAllowed 判断文件名是否可作为成员参与计数（MemberTypes 限定）。
func (p *SeqSubjectPattern) memberAllowed(fileName string) bool {
	if len(p.MemberTypes) == 0 {
		return true
	}
	class := fileTypeClassOf(fileName)
	for _, t := range p.MemberTypes {
		if t == class {
			return true
		}
	}
	return false
}

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

// Evaluate 先对自身求成员键与序号，再统计 Siblings（含自身）中同键且类型合格
// 的成员数；达到 MinSize 时绑定自身捕获、写入 GroupAnalysis（Key/Prefix 取键、
// Seq 取序号整数、Size 取合格成员数）并返回 true；落单返回 false。
func (p *SeqSubjectPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil {
		return false
	}
	if !p.memberAllowed(in.FileName) {
		return false
	}
	selfName := BaseNameWithoutExt(in.FileName)
	key, seqText, ok := p.seqMemberKey(selfName)
	if !ok {
		return false
	}
	size := 0
	for _, sibling := range in.Siblings {
		if !p.memberAllowed(sibling) {
			continue
		}
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

// ParentNamePattern 父目录名谓词（file/seq 目标的 parentFormat / parentContains）：
// 校验**直接父目录**（DirNames 末层）的名——与 dir 目标的同名谓词不同，后者
// 作用于命中目录的父。R4：仅上 1 级，无递归。
type ParentNamePattern struct {
	// Expr 为锚定正则原文（模板或包含词推导）。
	Expr string `json:"expr"`
	// re 为编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// newParentNamePattern 构造父目录名谓词。
func newParentNamePattern(expr string) (*ParentNamePattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("父目录格式正则 %q 编译失败: %w", expr, err)
	}
	return &ParentNamePattern{Expr: expr, re: re}, nil
}

// patternMarker 实现 Pattern 接口标记。
func (p *ParentNamePattern) patternMarker() {}

// Evaluate 校验直接父目录名（无目录层时不命中）。
func (p *ParentNamePattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	if p.re == nil || len(in.DirNames) == 0 {
		return false
	}
	return p.re.MatchString(in.DirNames[len(in.DirNames)-1])
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
