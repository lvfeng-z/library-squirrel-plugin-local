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
//   - filenameSeg[i]      文件名分隔段（去扩展名正文按 __ 分段，i 从 0 起）
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
				Label: dirLevelLabel(n) + "(剥壳)", Value: inner,
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

// splitFilenameSegments 文件名正文（去扩展名）→ 分隔段序列：`__` 为段分隔、
// `_` 为词内连接（作品名可含单下划线词）。已知名（库根名 / 目录名 / 剥壳变体
// 归一化）在正文中出现时保持为一个整段——覆盖跨 `__` 边界的混合形态（如
// by_jake__jaccobus_delithw 中的 jake__jaccobus：名字两侧以单个 `_` 粘接）。
func splitFilenameSegments(base string, hints []string) []string {
	// 归一化提示词，长名优先（避免子名遮蔽全名），过短提示词易误粘不入表。
	norm := make([]string, 0, len(hints))
	for _, h := range hints {
		if n := normalizeNameHint(h); len(n) >= 3 {
			norm = append(norm, n)
		}
	}
	sort.Slice(norm, func(i, j int) bool { return len(norm[i]) > len(norm[j]) })

	// 归一化正文（与原文等长时按原文切段保留大小写；罕见不等长时退用归一化文）。
	text := normalizeNameHint(base)
	src := base
	if len(text) != len(base) {
		src = text
	}

	// 定位互不重叠的名字出现区间（长名优先、区间内左优先）。
	type span struct{ start, end int }
	var spans []span
	taken := make([]bool, len(text))
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

	// 按 `__` 分段；名字区间为独立段，其两侧的单个 `_` 粘接符剥离。
	var segs []string
	pos := 0
	emit := func(piece string) {
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
	}
	for _, sp := range spans {
		emit(text[pos:sp.start])
		segs = append(segs, src[sp.start:sp.end])
		pos = sp.end
	}
	emit(text[pos:])
	return segs
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
