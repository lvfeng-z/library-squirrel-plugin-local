// 结构群形态表：规则无关的兄弟群结构形态判定与键/序号提取表。每条目=一个
// 「公共键+序号」的文件名结构形态，形态命中 ⇔ 键与序号双提取同时成立；
// 条目序即求值优先级，detectStructuralGroups 据此对每个文件取首个命中形态
// 定归属（一个文件至多命中一个形态、入一个结构群）。
package main

import "regexp"

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
