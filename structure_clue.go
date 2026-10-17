package main

// 结构分段线索集（结构直连规则构造 · 后端底座）：把文件的路径结构（库根名 /
// 各级目录名 / 剥壳变体 / 文件名分隔段 / 尾部码标记）展开为带稳定标识的线索
// 集合。三方共用同一标识空间——getPanorama 透出的线索集、assign 动作的出处
// 引用（clueRef）、规则总结器的输入。
//
// 线索引用（SnapStructClue.Ref）词汇：
//   - rootName            库根基名（无下标）
//   - dirLevel[n]         第 n 级目录名（n 从 1 起，如 dirLevel1）
//   - dirLevelStripped[n] 第 n 级目录的剥壳变体（命中剥壳集时才有）
//   - filenameSeg[i]      文件名分隔段（i 从 0 起；括号组内名与 __/_ 自适应
//                         分段，见 splitFilenameSegments）
//   - tailCode            尾部随机码（至多一条，末段且似随机时标注）

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// 结构线索类别词汇（SnapStructClue.Kind）。
const (
	structClueRootName         = "rootName"
	structClueDirLevel         = "dirLevel"
	structClueDirLevelStripped = "dirLevelStripped"
	structClueFilenameSeg      = "filenameSeg"
	structClueTailCode         = "tailCode"
)

// SnapStructClue 一条结构分段线索：稳定标识（Ref，赋值出处与总结器引用）+
// 类别 + 人话标签 + 段值。
type SnapStructClue struct {
	Ref   string `json:"ref"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// shellStrip 剥壳表条目：目录名（或文件名段）的包裹形态 → 内名。
type shellStrip struct {
	// Name 形态标识（呈现与日志用）。
	Name string
	// Prefix 包裹前缀（如 "Artist["）；必非空。
	Prefix string
	// Suffix 包裹后缀（如 "]"）；可为空（纯前缀形态如 "Artist "）。
	Suffix string
}

// shellStripTable 内置剥壳集（表驱动，可扩展）：Artist[...]→内名、Artist␣...→后名。
var shellStripTable = []shellStrip{
	{Name: "artistBracket", Prefix: "Artist[", Suffix: "]"},
	{Name: "artistSpace", Prefix: "Artist ", Suffix: ""},
}

// stripShell 对名字试剥壳：返回首个命中的剥壳条目与内名；未命中返回假。
// 内名为空（如 "Artist[]"）视为未命中。
func stripShell(name string) (shellStrip, string, bool) {
	for _, s := range shellStripTable {
		if s.Prefix == "" || !strings.HasPrefix(name, s.Prefix) {
			continue
		}
		inner := strings.TrimPrefix(name, s.Prefix)
		if s.Suffix != "" {
			if !strings.HasSuffix(inner, s.Suffix) || len(inner) < len(s.Suffix) {
				continue
			}
			inner = strings.TrimSuffix(inner, s.Suffix)
		}
		if inner == "" {
			continue
		}
		return s, inner, true
	}
	return shellStrip{}, "", false
}

// StructCluesOf 展开单个文件的结构分段线索集（纯函数）。
func StructCluesOf(rootPath, relPath, fileName string) []SnapStructClue {
	var clues []SnapStructClue
	rootName := ""
	if rootPath != "" {
		rootName = filepath.Base(filepath.Clean(filepath.ToSlash(rootPath)))
	}
	hints := make([]string, 0, 4)
	if rootName != "" && rootName != "." && rootName != "/" {
		clues = append(clues, SnapStructClue{
			Ref: structClueRootName, Kind: structClueRootName,
			Label: "库根名", Value: rootName,
		})
		hints = append(hints, rootName)
	}

	dirNames := ExtractDirLevels(relPath)
	for i, name := range dirNames {
		n := i + 1
		clues = append(clues, SnapStructClue{
			Ref:   fmt.Sprintf("%s%d", structClueDirLevel, n),
			Kind:  structClueDirLevel,
			Label: dirLevelLabel(n), Value: name,
		})
		hints = append(hints, name)
		if _, inner, ok := stripShell(name); ok {
			clues = append(clues, SnapStructClue{
				Ref:   fmt.Sprintf("%s%d", structClueDirLevelStripped, n),
				Kind:  structClueDirLevelStripped,
				Label: dirLevelLabel(n) + "剥壳", Value: inner,
			})
			hints = append(hints, inner)
		}
	}

	segs := splitFilenameSegments(BaseNameWithoutExt(fileName), hints)
	tailIdx := -1
	if len(segs) >= 2 && looksLikeTailCode(segs[len(segs)-1]) {
		tailIdx = len(segs) - 1
	}
	for i, seg := range segs {
		if i == tailIdx {
			clues = append(clues, SnapStructClue{
				Ref: structClueTailCode, Kind: structClueTailCode,
				Label: "尾部码", Value: seg,
			})
			continue
		}
		clues = append(clues, SnapStructClue{
			Ref:   fmt.Sprintf("%s%d", structClueFilenameSeg, i),
			Kind:  structClueFilenameSeg,
			Label: "文件名段", Value: seg,
		})
	}
	return clues
}

// DirStructCluesOf 展开单个目录的结构分段线索集（纯函数）：库根名 + dirRelKey
// 各级目录名（dirLevel[n]）与剥壳变体（dirLevelStripped[n]，标签「N级目录
// 剥壳」）——无文件名段与尾部码（目录面板的赋值出处引用与文件级共用同一
// 标识空间）。dirRelKey 为相对库根的 '/' 目录路径（空串=库根，仅库根名线索）。
func DirStructCluesOf(rootPath, dirRelKey string) []SnapStructClue {
	var clues []SnapStructClue
	rootName := ""
	if rootPath != "" {
		rootName = filepath.Base(filepath.Clean(filepath.ToSlash(rootPath)))
	}
	if rootName != "" && rootName != "." && rootName != "/" {
		clues = append(clues, SnapStructClue{
			Ref: structClueRootName, Kind: structClueRootName,
			Label: "库根名", Value: rootName,
		})
	}
	var levels []string
	if dirRelKey = strings.Trim(filepath.ToSlash(dirRelKey), "/"); dirRelKey != "" {
		levels = strings.Split(dirRelKey, "/")
	}
	for i, name := range levels {
		n := i + 1
		clues = append(clues, SnapStructClue{
			Ref:   fmt.Sprintf("%s%d", structClueDirLevel, n),
			Kind:  structClueDirLevel,
			Label: dirLevelLabel(n), Value: name,
		})
		if _, inner, ok := stripShell(name); ok {
			clues = append(clues, SnapStructClue{
				Ref:   fmt.Sprintf("%s%d", structClueDirLevelStripped, n),
				Kind:  structClueDirLevelStripped,
				Label: dirLevelLabel(n) + "剥壳", Value: inner,
			})
		}
	}
	return clues
}

// dirLevelLabel 目录层的中文序数标签（一级目录 / 二级目录 / …，超表退化为
// 「第 n 级目录」）。
func dirLevelLabel(n int) string {
	words := []string{"", "一", "二", "三", "四", "五", "六", "七", "八", "九", "十"}
	if n >= 1 && n < len(words) {
		return words[n] + "级目录"
	}
	return "第" + strconv.Itoa(n) + "级目录"
}

// hintReplacer 名字提示的归一化替换器：'-' 与空格折叠为 '_'（ASCII 1:1 字节，
// 不改变长度）。
var hintReplacer = strings.NewReplacer("-", "_", " ", "_")

// normalizeNameHint 名字 → 文件名段匹配形态：小写 + '-'/' ' 折叠为 '_'。
// 例："Jake--Jaccobus" → "jake__jaccobus"。
func normalizeNameHint(name string) string {
	return hintReplacer.Replace(strings.ToLower(name))
}

// splitFilenameSegments 文件名正文（去扩展名）→ 分隔段序列（v2）：
//   - 括号组原子段：`[]`【】（）()「」『』包裹组按最外层计，内名为一段（括号
//     本身只作分隔符，内名空则跳过该组）；
//   - 段间正文自适应：含括号组的正文，片段含 `__` 时按 `__` 分段、否则按单个
//     `_` 分段；无括号组的正文整体维持仅 `__` 分段（`_` 是词内连接）；
//   - 已知名保持：库根名 / 目录名 / 剥壳变体（归一化）在段间正文内的跨分隔符
//     出现区间保持整段（如 by_jake__jaccobus_delithw 中的 jake__jaccobus）；
//     提示区间与括号组重叠时括号组优先。
func splitFilenameSegments(base string, hints []string) []string {
	// 归一化提示词，长名优先（避免子名遮蔽全名），过短提示词易误粘不入表。
	norm := make([]string, 0, len(hints))
	for _, h := range hints {
		if n := normalizeNameHint(h); len(n) >= 3 {
			norm = append(norm, n)
		}
	}
	sort.Slice(norm, func(i, j int) bool { return len(norm[i]) > len(norm[j]) })

	// 归一化正文（与原文等长时按原文取段值保留大小写；罕见不等长时退用归一化文）。
	text := normalizeNameHint(base)
	src := base
	if len(text) != len(base) {
		src = text
	}

	// 括号组先占位（提示区间与括号组重叠时括号组优先，组内不再匹配提示词）。
	groups := findBracketGroups(text)
	taken := make([]bool, len(text))
	for _, g := range groups {
		for k := g.start; k < g.end; k++ {
			taken[k] = true
		}
	}

	// 定位互不重叠的名字出现区间（长名优先、区间内左优先）。
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

	// 依序输出原子段（括号组内名 + 名字区间），其间的正文片段按是否含括号组
	// 选择分隔：无括号组时 `_` 为词内连接，维持仅 `__` 分段（现状行为）。
	var segs []string
	splitPiece := func(piece string) {
		if len(groups) == 0 {
			piece = strings.TrimPrefix(piece, "__")
			piece = strings.TrimSuffix(piece, "__")
			if strings.HasPrefix(piece, "_") && len(piece) > 1 && !strings.HasPrefix(piece, "__") {
				piece = piece[1:]
			}
			if strings.HasSuffix(piece, "_") && len(piece) > 1 && !strings.HasSuffix(piece, "__") {
				piece = piece[:len(piece)-1]
			}
			for _, s := range strings.Split(piece, "__") {
				if s != "" {
					segs = append(segs, s)
				}
			}
			return
		}
		// 含括号组：片段含 `__` 按 `__` 分、否则按单个 `_` 分；边缘 `_` 为粘接符剥离。
		piece = strings.Trim(piece, "_")
		sep := "_"
		if strings.Contains(piece, "__") {
			sep = "__"
		}
		for _, s := range strings.Split(piece, sep) {
			if s != "" {
				segs = append(segs, s)
			}
		}
	}
	pos, gi, si := 0, 0, 0
	for gi < len(groups) || si < len(spans) {
		useGroup := si >= len(spans) || (gi < len(groups) && groups[gi].start < spans[si].start)
		if useGroup {
			g := groups[gi]
			gi++
			splitPiece(text[pos:g.start])
			if inner := src[g.innerStart:g.innerEnd]; inner != "" {
				segs = append(segs, inner)
			}
			pos = g.end
			continue
		}
		sp := spans[si]
		si++
		splitPiece(text[pos:sp.start])
		segs = append(segs, src[sp.start:sp.end])
		pos = sp.end
	}
	splitPiece(text[pos:])
	return segs
}

// bracketPair 分段用括号对（开/闭括号字面量）。
type bracketPair struct{ open, close string }

// bracketPairs 括号对表：正文中的包裹组按最外层计为一个原子段。
var bracketPairs = []bracketPair{
	{"[", "]"}, {"【", "】"}, {"（", "）"}, {"(", ")"}, {"「", "」"}, {"『", "』"},
}

// bracketGroup 正文中的括号组：含括号区间与内名区间（字节下标）。
type bracketGroup struct {
	start, end           int // 含括号 [start,end)
	innerStart, innerEnd int // 内名 [innerStart,innerEnd)
}

// findBracketGroups 扫描正文中的括号组：同对嵌套按最外层计（内层字符留在
// 内名里，不递归）；异对括号不干扰配对，同样留在内名里。无匹配闭括号的
// 开括号视作普通字符。返回区间按出现顺序排列。
func findBracketGroups(text string) []bracketGroup {
	var groups []bracketGroup
	for i := 0; i < len(text); {
		var pair bracketPair
		found := false
		for _, p := range bracketPairs {
			if strings.HasPrefix(text[i:], p.open) {
				pair, found = p, true
				break
			}
		}
		if !found {
			i++
			continue
		}
		// 同对嵌套计数，找与开括号匹配的闭括号。
		j, depth := i+len(pair.open), 1
		closed := false
	scan:
		for j < len(text) {
			switch {
			case strings.HasPrefix(text[j:], pair.close):
				j += len(pair.close)
				depth--
				if depth == 0 {
					closed = true
					break scan
				}
			case strings.HasPrefix(text[j:], pair.open):
				j += len(pair.open)
				depth++
			default:
				j++
			}
		}
		if !closed {
			i += len(pair.open)
			continue
		}
		groups = append(groups, bracketGroup{
			start: i, end: j,
			innerStart: i + len(pair.open), innerEnd: j - len(pair.close),
		})
		i = j
	}
	return groups
}

// looksLikeTailCode 尾部随机码启发式（保守标注）：段为 5..16 位纯字母数字、
// 且似随机——含数字、或无元音、或含 ≥3 连续辅音（如 delithw 的 thw、dhecdru
// 的 cdr）。仅用于末段的线索标注，不影响分段本身。
func looksLikeTailCode(seg string) bool {
	if len(seg) < 5 || len(seg) > 16 {
		return false
	}
	hasDigit, hasVowel := false, false
	consec, maxConsec := 0, 0
	for _, c := range seg {
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
			consec = 0
		case c == 'a' || c == 'e' || c == 'i' || c == 'o' || c == 'u':
			hasVowel = true
			consec = 0
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			consec++
			if consec > maxConsec {
				maxConsec = consec
			}
		default:
			return false
		}
	}
	return hasDigit || !hasVowel || maxConsec >= 3
}
