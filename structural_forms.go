// 结构群形态表：规则无关的兄弟群结构形态判定与键/序号提取表。每条目=一个
// 「公共键+序号」的文件名结构形态。判定层已退役（元素级内核 批次二：结构群
// 归属由元素对齐判定，detectStructuralGroups 不再按条目正则求值）；形态表
// 保留为命名层——群对齐成群后按序号元素上下文映射形态名，条目的键/序号提取
// 式用于键文本复原（既有形态键语义零漂移）与泛化器构造（buildSequenceRule），
// 亦是形态名归类器与提取式来源（决策3）。
package main

import (
	"regexp"
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

// formByName 按形态名取形态表条目（命名层入口：判定退役后形态表保留为形态
// 名归类与提取式来源——决策3「判定层退役、命名层保留」）。
func formByName(name string) *structuralForm {
	for _, f := range structuralForms {
		if f.name == name {
			return f
		}
	}
	return nil
}

// formNameOfSeqContext 序号元素上下文 → 形态名：壳内=括号序号、尾位有分隔=
// 尾缀数字、尾位无分隔=直连序号、中位=中段序号（`_page_N_by_` 特化由中段
// 序号条目的提取式承载，形态名不另设别名——FormName 语义与泛化产物不断代）。
func formNameOfSeqContext(els []SnapEl, nonSep []int, seqPos int) string {
	if seq := els[nonSep[seqPos]]; seq.Shelled {
		return "括号序号"
	}
	if seqPos == len(nonSep)-1 {
		if seqPos > 0 && nonSep[seqPos]-nonSep[seqPos-1] > 1 {
			return "尾缀数字" // 序号与前一元素间存在分隔 run
		}
		return "直连序号"
	}
	return "中段序号"
}

// elementConcatKey 兜底键文本（形态键提取式不命中的新形态，如 `第N话`/
// `industrial_N_becca`）：公共元素拼接——跳过 skip 命中的元素（序号位/豁免
// 尾部码位），分隔 run 只在两个保留元素之间保留，被跳元素邻接的分隔 run
// 随其吸收（`industrial_1_becca` → `industrial_becca` 而非 `industrial__becca`）。
// 候选键仅为两段式归集的预填，用户可改写。
func elementConcatKey(els []SnapEl, skip map[int]bool) string {
	var b strings.Builder
	pending, emitted := "", false
	for i, el := range els {
		if skip[i] {
			if el.Kind != elKindSep {
				pending = "" // 被跳元素吸收其两侧邻接分隔 run 的匹配点
			}
			continue
		}
		if el.Kind == elKindSep {
			if emitted {
				pending += el.Text
			}
			continue
		}
		b.WriteString(pending)
		pending = ""
		emitted = true
		b.WriteString(el.Text)
	}
	return b.String()
}

// workNumberPattern 作品号的保守提取式：首个 7 位及以上连续数字流。pixiv 等
// 站点作品号现行为 8-10 位，更短数字流（流水号/页号/C100 类会展号/集内序号）
// 一律不误纳——宁缺勿滥的保守取向（5-6 位历史作品号接受漏检）。
var workNumberPattern = regexp.MustCompile(`\d{7,}`)

// workNumberOf 提取名字内的作品号（A 档库内集比对与元素类别推断共用的单一
// 提取源）；无命中返回空串。
func workNumberOf(s string) string {
	return workNumberPattern.FindString(s)
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
