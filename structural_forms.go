// 结构群形态表：规则无关的兄弟群结构形态判定与键/序号提取表。每条目=一个
// 「公共键+序号」的文件名结构形态，形态命中 ⇔ 键与序号双提取同时成立；
// 条目序即求值优先级，detectStructuralGroups 据此对每个文件取首个命中形态
// 定归属（一个文件至多命中一个形态、入一个结构群）。
package main

import (
	"regexp"
	"sort"
	"strings"
)

// structuralForm 结构形态表条目。
type structuralForm struct {
	// name 形态名：结构群携带的形态标识，群卡呈现与序列规则泛化据以命名。
	name string
	// keyExpr 分群键提取式：整串结构锚定，捕获组1 = 分群键。键提取不成立即
	// 形态不命中——结构锚定保证键的成立范围与形态的成立范围一致（如直连
	// 序号的键式要求前缀以非数字结尾，天然拒绝纯数字名与尾部数字流超 4 位
	// 的文件名）。
	keyExpr string
	// seqExpr 序号提取式：捕获组1 = 序号原文（1-4 位，前导零保留零填充形态）。
	seqExpr string
	// keyRe / seqRe 两提取式的编译缓存（init 时由表达式原文编译，表达式与
	// 编译产物单一来源）。
	keyRe *regexp.Regexp
	seqRe *regexp.Regexp
}

// structuralForms 结构形态表：求值顺序 = 括号序号 → 中段序号 → 尾缀数字 →
// 直连序号。页级站点惯例形态由内置定群规则负责，不入本表。
var structuralForms = []*structuralForm{
	// 括号序号：富信息前缀 + 尾部 `_[NN]` 零填充括号序号；键剥去可选的尾
	// `_`，带与不带 `_` 的同族文件归同键。
	{name: "括号序号", keyExpr: `^(.+?)_?\[\d{1,4}\]$`, seqExpr: `\[(\d{1,4})\]$`},
	// 中段序号：页号在 `_page_N_` 中段、`_by_` 后为各不相同的作者与随机码；
	// 键取 page 前标题，随机码不入键，同标题不同页归一群。
	{name: "中段序号", keyExpr: `^(.+?)_page_`, seqExpr: `_page_(\d{1,4})_by_`},
	// 尾缀数字：公共前缀 + 分隔符（空格/_-·.．）+ 1-4 位尾缀数字。
	{name: "尾缀数字", keyExpr: `^(.+?)[ _\-·.．]+\d{1,4}$`, seqExpr: `(\d{1,4})$`},
	// 直连序号：前缀与序号间无分隔符（键非空且以非数字结尾）。
	{name: "直连序号", keyExpr: `^(.+?[^\d])\d{1,4}$`, seqExpr: `(\d{1,4})$`},
}

// init 编译形态表全部提取式：表达式非法属编程错误，进程启动即失败暴露。
func init() {
	for _, f := range structuralForms {
		f.keyRe = regexp.MustCompile(f.keyExpr)
		f.seqRe = regexp.MustCompile(f.seqExpr)
	}
}

// matchStructuralForm 按形态表优先级对去扩展名文件名求值：返回首个键/序号
// 双提取同时成立的形态条目及其分群键与序号原文；无命中时形态条目为 nil。
func matchStructuralForm(baseName string) (key, seqText string, form *structuralForm) {
	for _, f := range structuralForms {
		km := f.keyRe.FindStringSubmatch(baseName)
		if km == nil {
			continue
		}
		sm := f.seqRe.FindStringSubmatch(baseName)
		if sm == nil {
			continue
		}
		return km[1], sm[1], f
	}
	return "", "", nil
}

// —— 序列块区间（作品集识别强化 批次三阶段5）——

// SnapSeg 文件名内序列块区间（0-based 半开区间 [Start,End)，字节偏移）：
// Kind 取 "author"（作者括号块）/"workId"（作品号块）/"seq"（序号块），非
// 序列段不产生区间。单一事实源=形态表提取式（键/序号两式的捕获组定位块
// 位），前端按区间渲染块底色。区间相对去扩展名正文——正文为含扩展名文件名
// 的前缀，区间对两者均成立。
type SnapSeg struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Kind  string `json:"kind"`
}

// workNumberPattern 作品号的保守提取式：首个 7 位及以上连续数字流。pixiv 等
// 站点作品号现行为 8-10 位，更短数字流（流水号/页号/C100 类会展号/集内序号）
// 一律不误纳——宁缺勿滥的保守取向（5-6 位历史作品号接受漏检）。
var workNumberPattern = regexp.MustCompile(`\d{7,}`)

// workNumberOf 提取名字内的作品号（A 档库内集比对与序列块区间共用的单一
// 提取源）；无命中返回空串。
func workNumberOf(s string) string {
	return workNumberPattern.FindString(s)
}

// authorBracketPattern 作者括号块（保守）：半角/全角方括号包裹组。
var authorBracketPattern = regexp.MustCompile(`\[[^\[\]]*\]|【[^【】]*】`)

// segmentsOf 按形态表条目对去扩展名正文定位序列块区间：序号块=序号提取式
// 捕获组1；作者括号块与作品号块在键提取式捕获组1（公共前缀）内保守定位——
// 首个方括号组的内名非空且非纯数字即作者块，首个长数字流即作品号块。
// 保守性（文档化约定）：1) 两提取式任一不命中即零区间（形态不成立的文件名
// 无块可标）；2) 首括号内名为纯数字（作品号括号形态）时不再向后找作者块
//（后续括号更可能是标题）；3) 键内无 7 位以上数字流即无作品号块。区间按
// 起始偏移稳定排序返回。
func segmentsOf(form *structuralForm, name string) []SnapSeg {
	if form == nil {
		return nil
	}
	return segsFromExprs(form.keyRe, form.seqRe, name)
}

// segsFromExprs 键/序号两提取式（编译产物）→ 序列块区间：segmentsOf（形态
// 表条目，name=去扩展名正文）与规则定群群的成员区间（SiblingPattern 提取
// 式，name=含扩展名文件名）共用的纯实现——区间均对传入原文成立。
func segsFromExprs(keyRe, seqRe *regexp.Regexp, name string) []SnapSeg {
	if seqRe == nil {
		return nil
	}
	sm := seqRe.FindStringSubmatchIndex(name)
	if sm == nil || len(sm) < 4 {
		return nil
	}
	var segs []SnapSeg
	if keyRe != nil {
		if km := keyRe.FindStringSubmatchIndex(name); km != nil && len(km) >= 4 {
			key, keyStart := name[km[2]:km[3]], km[2]
			if m := authorBracketPattern.FindStringIndex(key); m != nil {
				inner := key[m[0]:m[1]]
				if strings.HasPrefix(inner, "【") {
					inner = strings.TrimSuffix(strings.TrimPrefix(inner, "【"), "】")
				} else {
					inner = strings.TrimSuffix(strings.TrimPrefix(inner, "["), "]")
				}
				if inner != "" && !allDigits(inner) {
					segs = append(segs, SnapSeg{Start: keyStart + m[0], End: keyStart + m[1], Kind: "author"})
				}
			}
			if m := workNumberPattern.FindStringIndex(key); m != nil {
				segs = append(segs, SnapSeg{Start: keyStart + m[0], End: keyStart + m[1], Kind: "workId"})
			}
		}
	}
	segs = append(segs, SnapSeg{Start: sm[2], End: sm[3], Kind: "seq"})
	sort.SliceStable(segs, func(i, j int) bool { return segs[i].Start < segs[j].Start })
	return segs
}

// allDigits 判定非空文本是否纯数字（作者括号内名纯数字=作品号括号形态）。
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
