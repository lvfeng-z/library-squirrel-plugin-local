// 贪婪识别（库内确认）提取来源 FromScan 的求值实现：候选分段 × 库内集合比对。
// 语义锚定「规则编辑图形帧收敛档」（2026-10-23，帧-05 四可裁点全按推荐）：
//   - 确认口径：仅库内已存在值命中才产出（查不到不提取，不做库外约定解析）；
//   - 扫描范围：path=文件名去扩展名+各级目录名；uncovered=同左但排除与本规则
//     捕获区间（同分段内）重叠的候选（「仅上半未覆盖片段」）；
//   - 多值命中：标签=全部命中（去重）；作者/作品集=唯一命中才产出，多命中
//     （歧义）不填并记 ScanNote（宁缺勿错）；
//   - 优先级：显式行（capture/constant 等）先产出，scan 只补本规则尚未产出
//     的维度（由调用方传入 filled 集合）。
package main

import "strings"

// 贪婪识别扫描范围取值。
const (
	// ScanScopePath 为全扫：文件名去扩展名+各级目录名（空串默认同值）。
	ScanScopePath = "path"
	// ScanScopeUncovered 为未覆盖扫描：在全扫基础上排除与本规则捕获区间
	//（同分段内）重叠的候选。
	ScanScopeUncovered = "uncovered"
)

// scanBracketPairs 为候选分段识别的括号对（内容作为一个候选）；与
// structure_clue 的 bracketPairs（结构线索拆分）分立，避免语义耦合。
var scanBracketPairs = [][2]rune{
	{'[', ']'}, {'(', ')'}, {'【', '】'}, {'（', '）'},
}

// scanCandidate 为一个候选分段及其在所属分段内的字节区间。
type scanCandidate struct {
	value string
	start int
	end   int
}

// isScanSeparator 判定分隔符（_、空格、连字符）。
func isScanSeparator(r rune) bool {
	return r == '_' || r == ' ' || r == '-'
}

// scanCandidatesOfSegment 生成分段 seg 的候选：整段、括号内容（按位置序）、
// 分隔符切分词（按位置序）；按值去重保序。
func scanCandidatesOfSegment(seg string) []scanCandidate {
	var out []scanCandidate
	seen := map[string]bool{}
	add := func(value string, start, end int) {
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, scanCandidate{value: value, start: start, end: end})
	}
	add(seg, 0, len(seg))
	runes := []rune(seg)
	// 括号内容：遇开括号找同型闭括号（不嵌套），内容整体为一个候选。
	for i := 0; i < len(runes); i++ {
		closeR, opened := bracketCloseOf(runes[i])
		if !opened {
			continue
		}
		for j := i + 1; j < len(runes); j++ {
			if runes[j] == closeR {
				add(string(runes[i+1:j]), len(string(runes[:i+1])), len(string(runes[:j])))
				i = j
				break
			}
		}
	}
	// 分隔符切分词。
	wordStart := -1
	for i := 0; i <= len(runes); i++ {
		if i < len(runes) && !isScanSeparator(runes[i]) {
			if wordStart < 0 {
				wordStart = i
			}
			continue
		}
		if wordStart >= 0 {
			add(string(runes[wordStart:i]), len(string(runes[:wordStart])), len(string(runes[:i])))
			wordStart = -1
		}
	}
	return out
}

// bracketCloseOf 返回开括号对应的同型闭括号。
func bracketCloseOf(r rune) (rune, bool) {
	for _, pair := range scanBracketPairs {
		if r == pair[0] {
			return pair[1], true
		}
	}
	return 0, false
}

// scanSegmentsOf 返回待扫描分段：文件名去扩展名在前，其后各级目录名（浅→深）。
func scanSegmentsOf(in *MatchInput) []string {
	segs := make([]string, 0, len(in.DirNames)+1)
	segs = append(segs, BaseNameWithoutExt(in.FileName))
	segs = append(segs, in.DirNames...)
	return segs
}

// scanLibrarySet 返回维度对应的库内集合（作者/标签/作品集；其余维度 nil）。
func scanLibrarySet(lib *LibraryIndex, dim MetadataDimension) map[string]struct{} {
	if lib == nil {
		return nil
	}
	switch dim {
	case DimAuthor:
		return lib.AuthorNames
	case DimTag:
		return lib.TagNames
	case DimCollection:
		return lib.WorkSetNames
	default:
		return nil
	}
}

// scanSpansOn 收集 ctx 内作用于分段 seg 的捕获区间：Segment 即 seg 者；以及
// 作用于完整文件名者（扫描用去扩展名基名，是其前缀——区间按基名长度截尾换算）。
// 相对路径（relpath）目标的捕获区间无法换算到分段坐标系，不参与排除。
func scanSpansOn(ctx *ParseContext, in *MatchInput, seg string) [][2]int {
	var out [][2]int
	base := BaseNameWithoutExt(in.FileName)
	for _, b := range ctx.Captures {
		switch b.Segment {
		case seg:
			out = append(out, [2]int{b.Start, b.End})
		case in.FileName:
			if seg != base || !strings.HasPrefix(in.FileName, base) {
				continue
			}
			end := b.End
			if end > len(base) {
				end = len(base)
			}
			if b.Start < len(base) && end > b.Start {
				out = append(out, [2]int{b.Start, end})
			}
		}
	}
	return out
}

// applyScanActions 执行规则内全部贪婪识别动作（第三遍提取：显式行已先产出）。
// filled 为本规则已产出维度集合（随产出更新：标签/唯一命中的作者/作品集产出后
// 记入，同维度后续 scan 行不再补）；返回追加声明与歧义记录。
func applyScanActions(rule *RuleEntry, in *MatchInput, ctx *ParseContext, lib *LibraryIndex, filled map[MetadataDimension]bool) ([]MetadataDeclaration, []ScanNote) {
	var decls []MetadataDeclaration
	var notes []ScanNote
	for _, action := range rule.Extractions {
		if action.From != FromScan {
			continue
		}
		if filled[action.Dimension] {
			continue
		}
		set := scanLibrarySet(lib, action.Dimension)
		if len(set) == 0 {
			continue
		}
		uncovered := action.ScanScope == ScanScopeUncovered
		var hits []MetadataDeclaration
		seen := map[string]bool{}
		var distinct []string
		for segIdx, seg := range scanSegmentsOf(in) {
			spans := [][2]int(nil)
			if uncovered {
				spans = scanSpansOn(ctx, in, seg)
			}
			for _, cand := range scanCandidatesOfSegment(seg) {
				if uncovered {
					covered := false
					for _, sp := range spans {
						if cand.start < sp[1] && sp[0] < cand.end {
							covered = true
							break
						}
					}
					if covered {
						continue
					}
				}
				if _, ok := set[cand.value]; !ok {
					continue
				}
				if seen[cand.value] {
					continue
				}
				seen[cand.value] = true
				distinct = append(distinct, cand.value)
				source := SourceDir
				if segIdx == 0 {
					source = SourceFilename
				}
				hits = append(hits, MetadataDeclaration{
					Dimension: action.Dimension,
					Value:     cand.value,
					Source:    source,
					RoleName:  action.RoleName,
					RuleID:    rule.ID,
				})
			}
		}
		if len(distinct) == 0 {
			continue
		}
		if action.Dimension == DimTag {
			// 标签天然多值：全部命中收下。
			decls = append(decls, hits...)
			filled[DimTag] = true
			continue
		}
		// 作者/作品集：唯一命中产出；多命中=歧义，宁缺勿错。
		if len(distinct) == 1 {
			decls = append(decls, hits[0])
			filled[action.Dimension] = true
		} else {
			notes = append(notes, ScanNote{Dimension: action.Dimension, Values: distinct, RuleID: rule.ID})
		}
	}
	return decls, notes
}

// hasScanAction 判定动作序列是否含贪婪识别者（提取段第三遍的分拣依据）。
func hasScanAction(actions []ExtractionAction) bool {
	for _, action := range actions {
		if action.From == FromScan {
			return true
		}
	}
	return false
}
