package main

// 元素拆分基座（元素级识别内核 · 批次二 F2）：把去扩展名正文拆为平铺元素
// 数组——判定（元素对齐判序列）与呈现（块底色渲染）的单一事实源。与
// splitFilenameSegments（结构线索口径）共用同一组基元（括号组定位、已知名
// 保持、归一化折叠），分段维持既有实现不动（消费方零回归的构造性保证）。

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// 元素类别词汇（SnapEl.Kind）。
const (
	// elKindAuthor 作者括号块（首个内名非纯数字的方/全角括号组）。
	elKindAuthor = "author"
	// elKindWorkId 作品号数字流（首个 ≥7 位纯数字）。
	elKindWorkId = "workId"
	// elKindSeq 序号数字（1-4 位纯数字；对齐终判前为候选标记，终判=对齐选定
	// 的序号位或与群序号原文相等的元素）。
	elKindSeq = "seq"
	// elKindTitle 群公共元素（markCommonElements 终判，统一浅灰底）。
	elKindTitle = "title"
	// elKindPlain 非公共/未判定元素（无底色）。
	elKindPlain = "plain"
	// elKindSep 分隔 run（拼接还原用，无底色）。
	elKindSep = "sep"
)

// SnapEl 文件名元素：前端按数组顺序拼接 text 即还原去扩展名正文（分隔 run
// 也是元素）——不再传偏移，阶段0 的 UTF-16 量纲换算随之整体退役。Shelled=
// 括号组元素：text=含括号原文、inner=剥壳内名；其余元素 inner 空。
type SnapEl struct {
	Text    string `json:"text"`
	Kind    string `json:"kind"`
	Shelled bool   `json:"shelled,omitempty"`
	Inner   string `json:"inner,omitempty"`
}

// elValue 元素的对齐取值（用户心智模型「相同元素（分隔符拆分+剥壳后）」）：
// 括号组取剥壳内名，其余取原文。
func elValue(el SnapEl) string {
	if el.Shelled {
		return el.Inner
	}
	return el.Text
}

// isElementSepRune 元素口径的分隔字符（归一化正文）：`_`（含折叠自 '-'/空格）
// 与句点族 `.`/`·`/`．`——与形态表尾缀数字的分隔类 [ _\-·.．] 对齐（分隔 run
// 聚为一个 sep 元素）。
func isElementSepRune(r rune) bool {
	return r == '_' || r == '.' || r == '·' || r == '．'
}

// splitFilenameElements 去扩展名正文 → 平铺元素数组：
//   - 分隔 run：连续的分隔字符聚为一个 sep 元素；
//   - 括号组原子元素（六对括号按最外层计）：Shelled 双态（text 含括号、inner
//     内名）；方/全角括号的 1-4 位纯数字内名是序号候选（圆括号序号=浏览器
//     重名冲突后缀，不参与序号候选——既有排除语义）；
//   - 已知名保持：hints（库根/目录名/剥壳变体）在正文内的跨分隔符出现区间
//     保持整段（长名优先、括号组优先）——路径参与的既成口径；
//   - 词内数字子段：无分隔直连的数字流独立成元素（`abc1`→abc+1——直连序号
//     在元素模型下的等价物），数字流按位数判类（≥7=workId〔仅首个〕、
//     1-4=seq 候选、其余 plain）。
//
// 类别推断与既有块区间口径一致：author 只认首个内名非纯数字的方/全角括号组
// （首括号内名纯数字=作品号括号形态，不再向后找作者块）；workId 只认首个
// ≥7 位数字流。title/plain 的公共性终判归群装配（markCommonElements）。
func splitFilenameElements(base string, hints []string) []SnapEl {
	// 归一化提示词（同分段：≥3、长名优先）。
	norm := make([]string, 0, len(hints))
	for _, h := range hints {
		if n := normalizeNameHint(h); len(n) >= 3 {
			norm = append(norm, n)
		}
	}
	sort.Slice(norm, func(i, j int) bool { return len(norm[i]) > len(norm[j]) })

	// 归一化正文（与原文等长时取段值保留原文大小写；罕见不等长时退用归一化文）。
	text := normalizeNameHint(base)
	src := base
	if len(text) != len(base) {
		src = text
	}

	// 括号组先占位（提示区间与括号组重叠时括号组优先），再定位互不重叠的
	// 已知名区间（长名优先、区间内左优先）——与分段同算法。
	groups := findBracketGroups(text)
	taken := make([]bool, len(text))
	for _, g := range groups {
		for k := g.start; k < g.end; k++ {
			taken[k] = true
		}
	}
	type span struct{ start, end int }
	var spans []span
	for _, n := range norm {
		for from := 0; from+len(n) <= len(text); from++ {
			if taken[from] || text[from:from+len(n)] != n {
				continue
			}
			ok := true
			for k := from; k < from+len(n); k++ {
				if taken[k] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			spans = append(spans, span{start: from, end: from + len(n)})
			for k := from; k < from+len(n); k++ {
				taken[k] = true
			}
			from += len(n) - 1
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	authorDone, workIdDone := false, false
	classifyDigits := func(digits string) string {
		switch {
		case len(digits) >= 7:
			if !workIdDone {
				workIdDone = true
				return elKindWorkId
			}
			return elKindPlain
		case len(digits) <= 4:
			return elKindSeq
		default:
			return elKindPlain
		}
	}
	els := make([]SnapEl, 0, 16)
	emitBracket := func(g bracketGroup) {
		text_ := src[g.start:g.end]
		inner := src[g.innerStart:g.innerEnd]
		squareish := strings.HasPrefix(text_, "[") || strings.HasPrefix(text_, "【")
		kind := elKindPlain
		switch {
		case inner == "":
			// 空内名括号组：平元素（分段口径跳过空组，元素口径保留以拼接还原）。
		case allDigits(inner):
			if len(inner) >= 7 {
				if !workIdDone {
					workIdDone = true
					kind = elKindWorkId
				}
			} else if len(inner) <= 4 && squareish {
				kind = elKindSeq
			}
			if squareish {
				// 首括号内名纯数字=作品号括号形态：作者块不再向后找。
				authorDone = true
			}
		default:
			if squareish && !authorDone {
				authorDone = true
				kind = elKindAuthor
			}
		}
		els = append(els, SnapEl{Text: text_, Kind: kind, Shelled: true, Inner: inner})
	}
	emitWord := func(w string) {
		start, wasDigit := -1, false
		flush := func(end int) {
			if start < 0 {
				return
			}
			piece := w[start:end]
			if piece == "" {
				return
			}
			kind := elKindPlain
			if wasDigit {
				kind = classifyDigits(piece)
			}
			els = append(els, SnapEl{Text: piece, Kind: kind})
		}
		for i := 0; i < len(w); i++ {
			d := w[i] >= '0' && w[i] <= '9'
			if start < 0 {
				start, wasDigit = i, d
				continue
			}
			if d != wasDigit {
				flush(i)
				start, wasDigit = i, d
			}
		}
		flush(len(w))
	}

	gi, si := 0, 0
	i := 0
	for i < len(text) {
		if gi < len(groups) && groups[gi].start == i {
			emitBracket(groups[gi])
			i = groups[gi].end
			gi++
			continue
		}
		if si < len(spans) && spans[si].start == i {
			s := spans[si]
			spanText := src[s.start:s.end]
			kind := elKindPlain
			if allDigits(spanText) && len(spanText) >= 7 && !workIdDone {
				workIdDone = true
				kind = elKindWorkId
			}
			els = append(els, SnapEl{Text: spanText, Kind: kind})
			i = s.end
			si++
			continue
		}
		r, _ := utf8.DecodeRuneInString(text[i:])
		if isElementSepRune(r) {
			j := i
			for j < len(text) {
				r2, s2 := utf8.DecodeRuneInString(text[j:])
				if !isElementSepRune(r2) {
					break
				}
				j += s2
			}
			els = append(els, SnapEl{Text: src[i:j], Kind: elKindSep})
			i = j
			continue
		}
		// 词：直到分隔符/括号组/已知名区间边界。
		j := i
		for j < len(text) {
			if gi < len(groups) && groups[gi].start == j {
				break
			}
			if si < len(spans) && spans[si].start == j {
				break
			}
			r2, s2 := utf8.DecodeRuneInString(text[j:])
			if isElementSepRune(r2) {
				break
			}
			j += s2
		}
		emitWord(src[i:j])
		i = j
	}
	return els
}

// elementHints 元素拆分的已知名提示（路径参与既成口径）：库根名 + 各级目录名
// + 剥壳变体（与 StructCluesOf 的 hints 同源）。
func elementHints(rootPath, relPath string) []string {
	var hints []string
	if rootPath != "" {
		if name := filepath.Base(filepath.Clean(filepath.ToSlash(rootPath))); name != "" && name != "." && name != "/" {
			hints = append(hints, name)
		}
	}
	for _, name := range ExtractDirLevels(relPath) {
		hints = append(hints, name)
		if _, inner, ok := stripShell(name); ok {
			hints = append(hints, inner)
		}
	}
	return hints
}

// dirNameElements 目录名（或剥壳内名）的元素分解（F5 插入池的目录名元素——
// 无壳单态，分隔 run 不入池）。
func dirNameElements(name string) []SnapEl {
	els := splitFilenameElements(name, nil)
	out := make([]SnapEl, 0, len(els))
	for _, el := range els {
		if el.Kind == elKindSep {
			continue
		}
		out = append(out, el)
	}
	return out
}
