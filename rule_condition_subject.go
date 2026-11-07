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
	"unicode/utf8"
)

// FileSubjectPattern 文件主体模式：格式模板匹配**去扩展名的文件名**（扩展名
// 由 fileExt 终结谓词承载，不混入模板——收敛档 R6 后的词汇分工）。
type FileSubjectPattern struct {
	// Expr 为推导出的锚定正则原文。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表。
	Captures []string `json:"captures"`
	// Repeats 为重复段描述列表（量词占位词推导；机械透传存字段，语义消费
	// 在阶段2 绑定层）。
	Repeats []condxRepeatDesc `json:"repeats,omitempty"`
	// Ext 为 fileExt 终结谓词的扩展名（已归一为小写带点形，如 ".jpg"）；
	// 空串 = 无该谓词。
	Ext string `json:"ext,omitempty"`
	// re 为编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// newFileSubjectPattern 构造文件主体模式：正则编译失败返回中文错误。
func newFileSubjectPattern(expr string, captures []string, repeats []condxRepeatDesc) (*FileSubjectPattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("文件格式正则 %q 编译失败: %w", expr, err)
	}
	return &FileSubjectPattern{Expr: expr, Captures: captures, Repeats: repeats, re: re}, nil
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
	return bindCondxCaptures(p.re, name, p.Captures, p.Repeats, ctx)
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
	// childTypeIs / dirLevel / parentFormat / parentContains）。
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
	case CondxAttachDirLevel:
		// 位于第 N 级（MinCount 承载 N）：depth 0 起、根直下=第 1 级，恰为第
		// N 级即过、否则该层不中——深→浅扫描循环不动，非 N 级层被本谓词跳过、
		// 自然落到 N 级层命中绑定（DirHit 记于 N 级层）；N 超深度（祖先链上无
		// 第 N 级）整行不中（严格不回退）。
		return depth+1 == a.MinCount
	default:
		return false
	}
}

// DirSubjectPattern 目录主体模式：格式（模板或包含词）匹配**任一层祖先目录名**
// （子树包含语义，任意深度），且该层目录满足全部终结谓词时命中：绑定该层捕获、
// 记 DirHit。谓词把「格式像」收紧为「结构像」（如 作品目录：作品目录里全是
// {序号} 子女）。
//
// 命中范围是该目录的整个子树与本次求值面的交集：子树内每个文件向上回看时都会
// 命中这一层目录（不限直接父目录）；但只有进入求值面（批量扫描清单或磁盘重扫
// 所得文件）的文件才被求值，不在清单内的文件不计。
type DirSubjectPattern struct {
	// Expr 为推导出的锚定正则原文（模板推导或包含词推导）。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表（contains 问法为空）。
	Captures []string `json:"captures"`
	// Repeats 为重复段描述列表（量词占位词推导；机械透传存字段，语义消费
	// 在阶段2 绑定层；contains 问法为空）。
	Repeats []condxRepeatDesc `json:"repeats,omitempty"`
	// Asserts 为随主体的终结谓词实例列表。
	Asserts []DirAssert `json:"asserts,omitempty"`
	// re 为编译缓存，不参与序列化。
	re *regexp.Regexp `json:"-"`
}

// newDirSubjectPattern 构造目录主体模式：正则编译失败返回中文错误。
func newDirSubjectPattern(expr string, captures []string, repeats []condxRepeatDesc) (*DirSubjectPattern, error) {
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("目录格式正则 %q 编译失败: %w", expr, err)
	}
	return &DirSubjectPattern{Expr: expr, Captures: captures, Repeats: repeats, re: re}, nil
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
		bindCondxCaptures(p.re, name, p.Captures, p.Repeats, ctx)
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
//
// 命中范围：同目录内{公共段}（即上述键）相同的成员数达到下限即成套（同键≥下限）；
// 不足下限的落单文件出局（不命中），序列不跨目录（只数同目录兄弟）。达成下限时写入
// 群组分析，成套文件的群卡呈现归分组段（定群）；群卡归属本身不是文件级命中，
// 只聚组不提取的定群形态不产文件级命中。
type SeqSubjectPattern struct {
	// Expr 为成员格式模板推导出的锚定正则原文。
	Expr string `json:"expr"`
	// Captures 为需要绑定的命名捕获组名列表（含 seq 与键/其他占位词）。
	Captures []string `json:"captures"`
	// Repeats 为重复段描述列表（量词占位词推导；机械透传存字段，语义消费
	// 在阶段2 绑定层；seq 主体行已禁重复词，通常为空）。
	Repeats []condxRepeatDesc `json:"repeats,omitempty"`
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
func newSeqSubjectPattern(expr string, captures []string, repeats []condxRepeatDesc, minSize int) (*SeqSubjectPattern, error) {
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
	return &SeqSubjectPattern{Expr: expr, Captures: captures, Repeats: repeats, SeqName: "seq", KeyParts: keyParts, MinSize: minSize, re: re}, nil
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
	bindCondxCaptures(p.re, selfName, p.Captures, p.Repeats, ctx)
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
// 列出的组按本次命中值追加为绑定，再对 Repeats 各重复段按整段位置组拆出全部
// 单元值、逐条追加同名绑定（{标签+} 一名 N 绑定，阶段2 扇出根基），返回 true；
// 不命中返回 false 且不绑定。未参与本次匹配的命名组（子匹配索引 -1，如可选
// 词位缺席）跳过绑定登记——缺席≠空串绑定，下游按「取不到值」处理；参与匹配
// 但捕获空串的组照常登记（判定只看索引）。表达式内不存在的组名仍绑空串
// （历史行为，与 RegexPattern.matchWithCaptures 同款；两域缺席语义分叉：模板域
// 缺席无绑定、手写 JSON 原生正则域绑空串）。区间供贪婪识别「未覆盖片段」复用；
// v4 主体捕获沿用同一绑定空间。
func bindCondxCaptures(re *regexp.Regexp, text string, names []string, repeats []condxRepeatDesc, ctx *ParseContext) bool {
	idx := re.FindStringSubmatchIndex(text)
	if idx == nil {
		return false
	}
	groupNames := re.SubexpNames()
	for _, name := range names {
		binding := CaptureBinding{Name: name, Segment: text}
		found, participated := false, false
		for i, groupName := range groupNames {
			if i > 0 && groupName == name {
				found = true
				s, e := idx[2*i], idx[2*i+1]
				if s >= 0 && e >= 0 {
					binding.Value = text[s:e]
					binding.Start, binding.End = s, e
					participated = true
				}
				break
			}
		}
		if found && !participated {
			// 组在表达式内但本次匹配未参与：跳过绑定登记（缺席=无绑定）。
			continue
		}
		ctx.Captures = append(ctx.Captures, binding)
	}
	for _, desc := range repeats {
		bindRepeatUnits(text, idx, desc, ctx)
	}
	return true
}

// bindRepeatUnits 对单个重复段拆单元并逐条绑定：整段位置组（GroupIndex）本次
// 匹配区间 [lo,hi) 内的子串跑单元子模式再拆（恰一个捕获组圈住单元值），每单元
// 追加一条 Name=En 的绑定（区间=单元值在全文本中的区间，已含整段偏移）。0+ 段
// 未参与（区间为 -1）时零绑定零报错——「取不到值零产出」语义直接成立。
//
// 包裹形态（[{标签+}]）的整段不含首开括号与末闭括号（1+ 档由组外字面段承载），
// 裸拆覆盖不完备（首末单元缺括号无法终匹配）——先检裸拆覆盖（首起于 0 且末
// 止于段尾），不完备时以整段前后相邻字符复原包裹后重拆，取命中更多的一组；
// 裸词形态（{标签+;_}）整段天然首尾皆值，覆盖恒完备、不经复原。区间按复原
// 偏移平移回全文本坐标（Segment 恒记全文本）。
func bindRepeatUnits(text string, idx []int, desc condxRepeatDesc, ctx *ParseContext) {
	if desc.GroupIndex <= 0 || 2*desc.GroupIndex+1 >= len(idx) {
		return
	}
	lo, hi := idx[2*desc.GroupIndex], idx[2*desc.GroupIndex+1]
	if lo < 0 || hi < 0 {
		return
	}
	unitRe, err := regexp.Compile(desc.UnitPattern)
	if err != nil {
		// 推导管线保证单元子模式可编译；异常保守零绑定不报错。
		return
	}
	seg, base := text[lo:hi], lo
	runs := unitRe.FindAllStringSubmatchIndex(seg, -1)
	if len(runs) == 0 || runs[0][0] != 0 || runs[len(runs)-1][1] != len(seg) {
		if open, ok := runeTailAt(text, lo); ok {
			if closeCh, ok2 := runeHeadAt(text, hi); ok2 {
				wrapped := open + seg + closeCh
				if wRuns := unitRe.FindAllStringSubmatchIndex(wrapped, -1); len(wRuns) > len(runs) {
					seg, base, runs = wrapped, lo-len(open), wRuns
				}
			}
		}
	}
	for _, r := range runs {
		if len(r) < 4 || r[2] < 0 || r[3] < 0 {
			continue
		}
		ctx.Captures = append(ctx.Captures, CaptureBinding{
			Name: desc.En, Value: seg[r[2]:r[3]], Segment: text,
			Start: base + r[2], End: base + r[3],
		})
	}
}

// runeTailAt 返回 s[:pos] 的末个 rune 原文（pos 为字节边界）；pos 越界或解码
// 失败返回 false。
func runeTailAt(s string, pos int) (string, bool) {
	if pos <= 0 || pos > len(s) {
		return "", false
	}
	r, size := utf8.DecodeLastRuneInString(s[:pos])
	if r == utf8.RuneError && size <= 1 {
		return "", false
	}
	return string(r), true
}

// runeHeadAt 返回 s[pos:] 的首个 rune 原文；pos 越界或解码失败返回 false。
func runeHeadAt(s string, pos int) (string, bool) {
	if pos < 0 || pos >= len(s) {
		return "", false
	}
	r, size := utf8.DecodeRuneInString(s[pos:])
	if r == utf8.RuneError && size <= 1 {
		return "", false
	}
	return string(r), true
}
