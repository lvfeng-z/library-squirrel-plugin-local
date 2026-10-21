package main

// 规则总结器（结构直连规则构造 · 后端）：从单文件赋值的线索出处（clueRef；
// 手填值无出处时以「值=某段」自动匹配兜底）推导结构映射，构造组合规则——
// 目录层锚定+剥壳变换（LocationPattern MinDepth=层深-1）/ 文件名分隔段捕获
//（RegexPattern）/ 尾部码丢弃（正则仅消费不捕获）/ 库根名常量。出参=人话
// 规则+逐维度映射+全库命中预览统计+跨目录样例应用；仅总结不入档，用户确认
//（generalize mode=struct）才经档案写单点（ArchiveWriteGeneralize）入档。

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// structMappingSlot 赋值单维度值与其线索出处决策。
type structMappingSlot struct {
	dim      MetadataDimension
	roleName string
	value    string
	clueRef  string          // 显式出处引用（空=手填）
	clue     *SnapStructClue // 解析后的线索；nil=手工指定（不参与泛化）
	manual   bool
	segIndex int    // 文件名段下标（文件名段映射有效）
	group    string // 文件名捕获组名（文件名段映射有效）
	tagIndex int    // Tags 内下标（非标签维度为 -1）
}

// RuleSuggestionMapping 建议卡的单维度映射说明。
type RuleSuggestionMapping struct {
	// Dimension 含义维度（author/title/tag/collection）。
	Dimension string `json:"dimension"`
	// From 来源人话（如「一级目录剥壳」「文件名首段」「库根名」「手工指定」）。
	From string `json:"from"`
	// ClueRef 解析到的线索引用（手工指定为空）。
	ClueRef string `json:"clueRef,omitempty"`
}

// RuleSuggestionSample 建议卡的样例应用：全库内被组合规则完整命中的其他文件
// 按组合规则求值的结论。
type RuleSuggestionSample struct {
	FileName string `json:"fileName"`
	// DirRelKey 样例文件所在目录键（相对库根的 '/' 目录路径，根直下为空串）：
	// 全库预览的样例跨目录呈现。
	DirRelKey string `json:"dirRelKey,omitempty"`
	// Result 人话结论（如「作者=Jake--Jaccobus，作品名=2b」）。
	Result string `json:"result"`
}

// RuleSuggestion 规则总结器产物（summarizeRule 动作应答；确认后经 generalize
// mode=struct 入档，两路共用同一构造器，建议即所见即所入）。
type RuleSuggestion struct {
	RelPath string `json:"relPath"`
	// Summary 人话规则一行（如「一级目录 Artist[任意] 剥壳→作者；文件名首段→
	// 作品名；库根名→标签；尾部码丢弃」）。
	Summary  string                  `json:"summary"`
	Mappings []RuleSuggestionMapping `json:"mappings"`
	// ManualDims 手工指定维度的说明（值不对应任何段，不参与泛化）。
	ManualDims []string `json:"manualDims,omitempty"`
	// Notes 呈现附注行（如样本属序列群但集名对不上段组合时，退回既有形态
	// 并附「集名无法对应文件名段落，未生成提取式规则」）。
	Notes []string `json:"notes,omitempty"`
	// Rules 将入档的组合规则摘要（目录层/文件名/库根名条目序列）。
	Rules []*GeneratedRuleView `json:"rules"`
	// HitCount 命中预览（全库）：除样本文件外将被组合规则完整覆盖的文件数；
	// HitTotal 全库文件总数（含样本自身）。
	HitCount int `json:"hitCount"`
	HitTotal int `json:"hitTotal"`
	// Samples 样例应用（全库内其余命中文件按快照序的前 3 个，附目录键）。
	Samples []RuleSuggestionSample `json:"samples"`
	// DropsTailCode 样例文件存在尾部码线索且被规则丢弃（人话提示用）。
	DropsTailCode bool `json:"dropsTailCode,omitempty"`
}

// buildStructMapRules 规则总结器构造器：赋值+线索出处 → 建议呈现与将入档的
// 组合规则条目（不落库）。全部维度无法结构映射时返回错误。构造自检：每条
// 规则在样例文件上必须复现赋值值，自检不过的映射降级为手工指定。
func buildStructMapRules(rootPath, relPath, fileName string, assign *SessionAssignment) (*RuleSuggestion, []RuleEntry, error) {
	if assign == nil {
		return nil, nil, fmt.Errorf("赋值为空，无可总结内容")
	}
	clues := StructCluesOf(rootPath, relPath, fileName)
	byRef := make(map[string]*SnapStructClue, len(clues))
	for i := range clues {
		byRef[clues[i].Ref] = &clues[i]
	}
	hasTail := byRef[structClueTailCode] != nil

	// 文件名分段形态走查（v2：分隔符/括号感知构造与段降级判定的底座）。
	segPlan, segPlanOK := structFilenamePlanOf(fileName, structSegValuesOf(clues))

	slots := collectStructSlots(assign)
	for i := range slots {
		resolveStructSlot(&slots[i], clues, byRef)
	}

	// —— 组合规则构造：目录层条目（按层分组）+ 文件名条目 + 库根名常量折入 ——
	dirPlans := make(map[string]*structDirPlan)
	var dirOrder []string
	var fileSlots []*structMappingSlot
	usedSegIdx := make(map[int]bool)
	for i := range slots {
		s := &slots[i]
		if s.manual || s.clue == nil {
			continue
		}
		switch s.clue.Kind {
		case structClueDirLevel, structClueDirLevelStripped:
			n := structClueLevelOf(s.clue.Ref)
			if n <= 0 {
				s.manual = true
				continue
			}
			key := strconv.Itoa(n)
			if s.clue.Kind == structClueDirLevelStripped {
				shell, _, ok := stripShell(dirNameAtLevel(relPath, n))
				if !ok {
					s.manual = true
					continue
				}
				key = key + "\x00" + shell.Name
			} else {
				key = key + "\x00"
			}
			plan, ok := dirPlans[key]
			if !ok {
				plan = &structDirPlan{level: n, name: dirNameAtLevel(relPath, n)}
				if s.clue.Kind == structClueDirLevelStripped {
					shell, _, _ := stripShell(plan.name)
					plan.shell = &shell
				}
				dirPlans[key] = plan
				dirOrder = append(dirOrder, key)
			}
			plan.slots = append(plan.slots, s)
		case structClueFilenameSeg:
			idx := structSegIndexOf(s.clue.Ref)
			// 已被更早维度占用的段、形态走查失配的段 → 手工；段值含活跃分隔符的
			// 段同样无法按段表达（_ 模式段类排除 _；__ 模式普通段排除 __，括号段
			// 以闭括号为界不受限）。
			bad := idx < 0 || usedSegIdx[idx] || !segPlanOK || idx >= len(segPlan.forms)
			if !bad {
				if segPlan.underscore {
					bad = strings.Contains(s.clue.Value, "_")
				} else if segPlan.forms[idx].bracket == nil {
					bad = strings.Contains(s.clue.Value, "__")
				}
			}
			if bad {
				s.manual = true
				continue
			}
			usedSegIdx[idx] = true
			s.segIndex = idx
			s.group = structGroupName(s.dim, s.tagIndex)
			fileSlots = append(fileSlots, s)
		default:
			// rootName 常量折入目录条目；tailCode 引用→手工（尾部码只丢弃）。
			if s.clue.Kind != structClueRootName {
				s.manual = true
			}
		}
	}

	var rules []RuleEntry
	var phrases []string
	var mappings []RuleSuggestionMapping
	var manualDims []string

	// 目录层条目（层序稳定）。firstDirIdx 为常量折入目标（-1=尚无目录条目）；
	// 以下标寻址避免 append 扩容后的悬垂指针。
	firstDirIdx := -1
	for _, key := range dirOrder {
		plan := dirPlans[key]
		rule, ok := plan.build(relPath, fileName)
		if !ok {
			for _, s := range plan.slots {
				s.manual = true
			}
			continue
		}
		rules = append(rules, rule)
		if firstDirIdx < 0 {
			firstDirIdx = len(rules) - 1
		}
		for _, s := range plan.slots {
			phrases = append(phrases, structDirPhrase(s, relPath))
			mappings = append(mappings, RuleSuggestionMapping{
				Dimension: string(s.dim), From: structFromLabel(s), ClueRef: s.clue.Ref,
			})
		}
	}

	// 文件名条目（分隔段捕获，尾部码自然丢弃）。
	if len(fileSlots) > 0 {
		if rule, ok := buildStructFilenameRule(relPath, fileName, segPlan, fileSlots); ok {
			rules = append(rules, rule)
			for _, s := range fileSlots {
				phrases = append(phrases, structFilePhrase(s))
				mappings = append(mappings, RuleSuggestionMapping{
					Dimension: string(s.dim), From: structFromLabel(s), ClueRef: s.clue.Ref,
				})
			}
		} else {
			for _, s := range fileSlots {
				s.manual = true
			}
			fileSlots = nil
		}
	}

	// 库根名常量（折入首个目录条目；无目录条目时独立成条，锚=至少一层目录）。
	for i := range slots {
		s := &slots[i]
		if s.manual || s.clue == nil || s.clue.Kind != structClueRootName {
			continue
		}
		action := ExtractionAction{
			Dimension: s.dim, From: FromConstant, Constant: s.value, RoleName: s.roleName,
		}
		phrase := "库根名→" + qnaDimensionLabel(s.dim)
		if firstDirIdx >= 0 {
			rules[firstDirIdx].Extractions = append(rules[firstDirIdx].Extractions, action)
		} else {
			ancestor, err := NewRegexPattern(string(TargetFileName), "^.*$", nil)
			if err != nil {
				s.manual = true
				continue
			}
			location, err := NewLocationPattern(0, -1, ancestor)
			if err != nil {
				s.manual = true
				continue
			}
			rules = append(rules, RuleEntry{
				ID:          generatedRuleID("qna-struct", rootPath),
				Description: fmt.Sprintf("泛化（结构映射·库根名）：库根名「%s」声明%s（源自「%s」的赋值）", s.clue.Value, qnaDimensionLabel(s.dim), relPath),
				Source:      RuleSourceQna,
				Enabled:     true,
				Pattern:     location,
				Extractions: []ExtractionAction{action},
			})
			firstDirIdx = len(rules) - 1
		}
		phrases = append(phrases, phrase)
		mappings = append(mappings, RuleSuggestionMapping{
			Dimension: string(s.dim), From: "库根名", ClueRef: s.clue.Ref,
		})
	}

	// 手工维度收尾。
	for i := range slots {
		s := &slots[i]
		if s.value == "" || !s.manual {
			continue
		}
		manualDims = append(manualDims, fmt.Sprintf("%s=%s（手工指定，不参与泛化）",
			qnaDimensionLabel(s.dim), s.value))
		mappings = append(mappings, RuleSuggestionMapping{
			Dimension: string(s.dim), From: "手工指定",
		})
	}

	if len(rules) == 0 {
		return nil, nil, fmt.Errorf("赋值各维度均无法映射到结构分段（值不出现在库根名/目录层/文件名段中），无法总结结构映射规则")
	}

	summary := strings.Join(phrases, "；")
	if hasTail {
		summary += "；尾部码丢弃"
	}
	sugg := &RuleSuggestion{
		RelPath:       relPath,
		Summary:       summary,
		Mappings:      mappings,
		ManualDims:    manualDims,
		DropsTailCode: hasTail,
	}
	for i := range rules {
		sugg.Rules = append(sugg.Rules, generatedRuleViewOf(&rules[i]))
	}
	return sugg, rules, nil
}

// structDirPlan 单层目录的结构条目构造计划。
type structDirPlan struct {
	level int    // 1 起层深
	name  string // 样例目录名
	shell *shellStrip
	slots []*structMappingSlot
}

// build 构造目录层规则条目并自检（锚定样例目录层、逐槽提取值=赋值值）。
func (p *structDirPlan) build(relPath, fileName string) (RuleEntry, bool) {
	var expr string
	if p.shell != nil {
		inner := ".+"
		if p.shell.Suffix != "" {
			inner = "[^" + escapeForClass(rune(p.shell.Suffix[0])) + "]*"
		}
		expr = "^" + regexp.QuoteMeta(p.shell.Prefix) + inner + regexp.QuoteMeta(p.shell.Suffix) + "$"
	} else {
		expr = "^" + regexp.QuoteMeta(p.name) + "$"
	}
	ancestor, err := NewRegexPattern(string(TargetFileName), expr, nil)
	if err != nil {
		return RuleEntry{}, false
	}
	location, err := NewLocationPattern(p.level-1, p.level-1, ancestor)
	if err != nil {
		return RuleEntry{}, false
	}
	extractions := make([]ExtractionAction, 0, len(p.slots))
	for _, s := range p.slots {
		action := ExtractionAction{Dimension: s.dim, From: FromAncestorDir, RoleName: s.roleName}
		if p.shell != nil {
			action.StripPrefix = p.shell.Prefix
			action.StripSuffix = p.shell.Suffix
		}
		extractions = append(extractions, action)
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-struct", relPath),
		Description: fmt.Sprintf("泛化（结构映射·目录层）：第 %d 层目录「%s」→%s（源自「%s」的赋值）", p.level, p.name, structDimPhraseOf(p.slots), relPath),
		Source:      RuleSourceQna,
		Enabled:     true,
		Pattern:     location,
		Extractions: extractions,
	}
	// 自检：样例文件上锚定命中且各槽提取值复现赋值值。
	ctx := &ParseContext{}
	if !location.Evaluate(structSampleInput(relPath, fileName), nil, ctx) {
		return RuleEntry{}, false
	}
	for i, s := range p.slots {
		decl, ok := extractions[i].Apply(rule.ID, ctx)
		if !ok || decl.Value != s.value {
			return RuleEntry{}, false
		}
	}
	return rule, true
}

// structFilenamePlan 样例文件名的分段形态走查（buildStructFilenameRule v2 的
// 底座）：段值依序在正文（去扩展名）中定位，重建每段的间隔原文与括号包裹；
// underscore 为「含括号组且段间无 __」的单 _ 分隔模式。
type structFilenamePlan struct {
	forms      []structSegForm
	underscore bool
}

// structSegForm 单段的构造形态：与前一段值之间的间隔原文（括号段已折出开
// 括号）与包裹括号对（nil=普通段）。
type structSegForm struct {
	gap     string
	bracket *bracketPair
}

// structFilenamePlanOf 走查样例文件名的分段形态；段值不按序出现等失配返回
// false（调用方将文件名槽降级手工）。正文取与 splitFilenameSegments 同源的
// 归一规则（等长时保留原文），段值必为正文切片、依序可寻。间隔中括号段前后
// 的开/闭括号折入段自身（段类只裹内名）。
func structFilenamePlanOf(fileName string, segs []string) (structFilenamePlan, bool) {
	base := BaseNameWithoutExt(fileName)
	src := base
	if text := normalizeNameHint(base); len(text) != len(base) {
		src = text
	}
	forms := make([]structSegForm, 0, len(segs))
	pos, hasBracket, hasDouble := 0, false, false
	for _, seg := range segs {
		off := strings.Index(src[pos:], seg)
		if off < 0 {
			return structFilenamePlan{}, false
		}
		start, end := pos+off, pos+off+len(seg)
		form := structSegForm{gap: src[pos:start]}
		for _, p := range bracketPairs {
			if strings.HasSuffix(form.gap, p.open) && strings.HasPrefix(src[end:], p.close) {
				bracket := p
				form.bracket = &bracket
				form.gap = form.gap[:len(form.gap)-len(p.open)]
				end += len(p.close)
				break
			}
		}
		hasBracket = hasBracket || form.bracket != nil
		hasDouble = hasDouble || strings.Contains(form.gap, "__")
		forms = append(forms, form)
		pos = end
	}
	return structFilenamePlan{forms: forms, underscore: hasBracket && !hasDouble}, true
}

// structSegValuesOf 线索集 → 文件名段值序列（filenameSeg 线索按下标序生成，
// 按出现序收集即得段序；尾部码线索不在其列）。
func structSegValuesOf(clues []SnapStructClue) []string {
	var segs []string
	for i := range clues {
		if clues[i].Kind == structClueFilenameSeg {
			segs = append(segs, clues[i].Value)
		}
	}
	return segs
}

// buildStructFilenameRule 构造文件名分隔段规则（v2：分隔符/括号感知）：按样例
// 文件名的实际分段形态逐段展开——段间隔以字面连接，首个捕获组前的未捕获段
// 逐一按形态消费；括号段以开/闭括号字面包裹、段类排除闭括号；_ 模式（含括号
// 组且段间无 __）普通段用 `[^_]` 段类、末段后以 `(?:_.*)?` 吞余部，__ 模式维持
// `[^_].*?`+`__` 既有形态。末槽后的其余段与扩展名整体吞掉（尾部码丢弃）。
// 自检：样例文件上各捕获值必须复现赋值值。
func buildStructFilenameRule(relPath, fileName string, plan structFilenamePlan, slots []*structMappingSlot) (RuleEntry, bool) {
	sorted := make([]*structMappingSlot, len(slots))
	copy(sorted, slots)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1].segIndex > sorted[j].segIndex; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	var expr strings.Builder
	expr.WriteString("^")
	prev := 0
	captures := make([]string, 0, len(sorted))
	extractions := make([]ExtractionAction, 0, len(sorted))
	for i, s := range sorted {
		for j := prev; j < s.segIndex; j++ {
			writeStructSegForm(&expr, plan.forms[j], "", plan.underscore)
		}
		writeStructSegForm(&expr, plan.forms[s.segIndex], s.group, plan.underscore)
		if i == len(sorted)-1 {
			if plan.underscore {
				expr.WriteString("(?:_.*)?(?:\\.[^.]*)?$")
			} else {
				expr.WriteString("(?:__.*)?(?:\\.[^.]*)?$")
			}
		}
		prev = s.segIndex + 1
		captures = append(captures, s.group)
		extractions = append(extractions, ExtractionAction{
			Dimension: s.dim, From: FromCapture, CaptureName: s.group, RoleName: s.roleName,
		})
	}
	pattern, err := NewRegexPattern(string(TargetFileName), expr.String(), captures)
	if err != nil {
		return RuleEntry{}, false
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-struct", fileName),
		Description: fmt.Sprintf("泛化（结构映射·文件名）：文件名分隔段形态→%s（尾部码丢弃）（源自「%s」的赋值）", structDimPhraseOf(sorted), relPath),
		Source:      RuleSourceQna,
		Enabled:     true,
		Pattern:     pattern,
		Extractions: extractions,
	}
	// 自检：样例文件上各捕获值复现赋值值。
	ctx := &ParseContext{}
	if !pattern.Evaluate(structSampleInput(relPath, fileName), nil, ctx) {
		return RuleEntry{}, false
	}
	for _, s := range sorted {
		decl, ok := ExtractionAction{
			Dimension: s.dim, From: FromCapture, CaptureName: s.group, RoleName: s.roleName,
		}.Apply(rule.ID, ctx)
		if !ok || decl.Value != s.value {
			return RuleEntry{}, false
		}
	}
	return rule, true
}

// structBracketClassChars 括号对开（open=true）/闭括号字面集的字符类内容
// （QuoteMeta 后可直接拼入 []）。_ 模式普通段的段类排除集：括号包裹视作同形
// 装饰（可选消费），同形异值文件上该位置的括号不混入段值。
func structBracketClassChars(open bool) string {
	var b strings.Builder
	for _, p := range bracketPairs {
		c := p.close
		if open {
			c = p.open
		}
		b.WriteString(regexp.QuoteMeta(c))
	}
	return b.String()
}

var (
	structOpenChars  = structBracketClassChars(true)
	structCloseChars = structBracketClassChars(false)
)

// writeStructSegForm 写入单段正则形态：段间隔字面 + 括号段的开括号字面 + 捕获
// （group 非空）或消费段类 + 闭括号字面。_ 模式段类排除活跃分隔符 _（括号段
// 连闭括号一并排除），普通段以可选括号包裹容纳同形异值的装饰差异；__ 模式
// 普通段维持 `[^_].*?` 既有形态、括号段仅排除闭括号（内名可含 _ / __，以闭
// 括号收界）。
func writeStructSegForm(b *strings.Builder, f structSegForm, group string, underscore bool) {
	b.WriteString(regexp.QuoteMeta(f.gap))
	if f.bracket == nil {
		if underscore {
			b.WriteString("(?:[" + structOpenChars + "])?")
			if group == "" {
				b.WriteString("[^" + structOpenChars + structCloseChars + "_]*")
			} else {
				b.WriteString("(?P<" + group + ">[^" + structOpenChars + structCloseChars + "_]+?)")
			}
			b.WriteString("(?:[" + structCloseChars + "])?")
			return
		}
		if group == "" {
			b.WriteString("(?:[^_].*?)")
		} else {
			b.WriteString("(?P<" + group + ">[^_].*?)")
		}
		return
	}
	excl := regexp.QuoteMeta(f.bracket.close)
	if underscore {
		excl += "_"
	}
	if group == "" {
		b.WriteString(regexp.QuoteMeta(f.bracket.open) + "[^" + excl + "]*")
	} else {
		b.WriteString(regexp.QuoteMeta(f.bracket.open) + "(?P<" + group + ">[^" + excl + "]+?)")
	}
	b.WriteString(regexp.QuoteMeta(f.bracket.close))
}

// —— 纯辅助 ——

// structSampleInput 样例文件的匹配输入（DirNames 由相对路径重建）。
func structSampleInput(relPath, fileName string) *MatchInput {
	return &MatchInput{
		FileName: fileName,
		DirNames: ExtractDirLevels(relPath),
		RelPath:  relPath,
	}
}

// collectStructSlots 赋值展开为映射槽（维度顺序=作者/作品名/作品集/标签，
// 同段先到先得的决定序）。
func collectStructSlots(assign *SessionAssignment) []structMappingSlot {
	var slots []structMappingSlot
	add := func(dim MetadataDimension, roleName, value, clueRef string, tagIndex int) {
		if value == "" {
			return
		}
		slots = append(slots, structMappingSlot{
			dim: dim, roleName: roleName, value: value, clueRef: clueRef, tagIndex: tagIndex,
		})
	}
	add(DimAuthor, assign.AuthorRole, assign.Author, assign.AuthorClueRef, -1)
	add(DimTitle, "", assign.Title, assign.TitleClueRef, -1)
	add(DimCollection, "", assign.Collection, assign.CollectionClueRef, -1)
	for i, tag := range assign.Tags {
		ref := ""
		if i < len(assign.TagClueRefs) {
			ref = assign.TagClueRefs[i]
		}
		add(DimTag, "", tag, ref, i)
	}
	return slots
}

// resolveStructSlot 解析单槽出处：显式引用优先（须在线索集内），否则按
// 「值=某段」自动匹配（剥壳变体 > 目录层 > 库根名 > 文件名段；尾部码不参与
// 自动匹配）；两路皆空 → 手工指定（不参与泛化）。
func resolveStructSlot(s *structMappingSlot, clues []SnapStructClue, byRef map[string]*SnapStructClue) {
	if s.clueRef != "" {
		if c, ok := byRef[s.clueRef]; ok {
			s.clue = c
			return
		}
	}
	priority := []string{structClueDirLevelStripped, structClueDirLevel, structClueRootName, structClueFilenameSeg}
	for _, kind := range priority {
		for i := range clues {
			if clues[i].Kind == kind && clues[i].Value == s.value {
				s.clue = &clues[i]
				return
			}
		}
	}
	s.manual = true
}

// structClueLevelOf 目录层线索引用（dirLevel[n] / dirLevelStripped[n]）→ 层深 n。
func structClueLevelOf(ref string) int {
	for _, prefix := range []string{structClueDirLevelStripped, structClueDirLevel} {
		if strings.HasPrefix(ref, prefix) {
			n, err := strconv.Atoi(strings.TrimPrefix(ref, prefix))
			if err == nil {
				return n
			}
		}
	}
	return -1
}

// structSegIndexOf 文件名段线索引用（filenameSeg[i]）→ 段下标 i。
func structSegIndexOf(ref string) int {
	if !strings.HasPrefix(ref, structClueFilenameSeg) {
		return -1
	}
	i, err := strconv.Atoi(strings.TrimPrefix(ref, structClueFilenameSeg))
	if err != nil {
		return -1
	}
	return i
}

// dirNameAtLevel 相对路径的第 n 级目录名（1 起；越界返回空）。
func dirNameAtLevel(relPath string, n int) string {
	levels := ExtractDirLevels(relPath)
	if n < 1 || n > len(levels) {
		return ""
	}
	return levels[n-1]
}

// structGroupName 捕获组名（title/author/collection/tagN，与既有泛化同形）。
func structGroupName(dim MetadataDimension, tagIndex int) string {
	if dim == DimTag {
		return "tag" + strconv.Itoa(tagIndex+1)
	}
	return string(dim)
}

// structDirPhrase 目录层映射的人话（如「一级目录 Artist[任意] 剥壳→作者」）。
func structDirPhrase(s *structMappingSlot, relPath string) string {
	n := structClueLevelOf(s.clue.Ref)
	label := qnaDimensionLabel(s.dim)
	if s.clue.Kind == structClueDirLevelStripped {
		return fmt.Sprintf("%s %s剥壳→%s", dirLevelLabel(n), structWrapDesc(relPath, n), label)
	}
	return fmt.Sprintf("%s「%s」→%s", dirLevelLabel(n), s.clue.Value, label)
}

// structWrapDesc 剥壳映射的包裹形态描述（如 "Artist[任意]"；取样例目录名的
// 剥壳表前/后缀 +「任意」内名）。
func structWrapDesc(relPath string, n int) string {
	name := dirNameAtLevel(relPath, n)
	if shell, _, ok := stripShell(name); ok {
		return shell.Prefix + "任意" + shell.Suffix
	}
	return ""
}

// structFilePhrase 文件名段映射的人话（首段/第 N 段）。
func structFilePhrase(s *structMappingSlot) string {
	if s.segIndex == 0 {
		return "文件名首段→" + qnaDimensionLabel(s.dim)
	}
	return fmt.Sprintf("文件名第%d段→%s", s.segIndex+1, qnaDimensionLabel(s.dim))
}

// structFromLabel 映射说明的来源标签。
func structFromLabel(s *structMappingSlot) string {
	switch s.clue.Kind {
	case structClueDirLevelStripped:
		return dirLevelLabel(structClueLevelOf(s.clue.Ref)) + "剥壳"
	case structClueDirLevel:
		return dirLevelLabel(structClueLevelOf(s.clue.Ref))
	case structClueRootName:
		return "库根名"
	case structClueFilenameSeg:
		if s.segIndex == 0 {
			return "文件名首段"
		}
		return fmt.Sprintf("文件名第%d段", s.segIndex+1)
	}
	return "手工指定"
}

// structDimPhraseOf 槽集的维度人话列举（描述用）。
func structDimPhraseOf(slots []*structMappingSlot) string {
	parts := make([]string, 0, len(slots))
	for _, s := range slots {
		parts = append(parts, qnaDimensionLabel(s.dim))
	}
	return strings.Join(parts, "、")
}

// escapeForClass 字符类内的转义（] \ ^ 需转义）。
func escapeForClass(c rune) string {
	switch c {
	case ']', '\\', '^':
		return "\\" + string(c)
	}
	return string(c)
}

// —— 会话动作 ——

// ApplyStructSummary 规则总结器动作（summarizeRule，单文件寻址）：从赋值与线索
// 出处推导组合规则建议（人话规则+全库命中预览+样例应用），不写档案、不触发
// 重识别——用户确认走 generalize（mode=struct）才入档。
func (m *SessionManager) ApplyStructSummary(root, relPath string) (*RuleSuggestion, error) {
	return m.applyStructSummary(root, relPath, "")
}

// ApplyDirStructSummary 规则总结器动作（summarizeRule，目录寻址）：样本文件取
// 子树内首个有赋值的文件；建议卡 RelPath 呈现目录键（供前端目录面板呈现，
// 确认入档走 generalize 的目录寻址）。
func (m *SessionManager) ApplyDirStructSummary(root, dirRelKey string) (*RuleSuggestion, error) {
	return m.applyStructSummary(root, "", dirRelKey)
}

// applyStructSummary 规则总结器落地单一实现：relPath 非空=单文件寻址；空=
// 目录寻址（dirRelKey 子树，样本=首个有赋值的文件）。
func (m *SessionManager) applyStructSummary(root, relPath, dirRelKey string) (*RuleSuggestion, error) {
	relPath = filepath.ToSlash(relPath)
	dirAddressed := relPath == ""
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	if dirAddressed {
		_, relPath, err = dirScopedFiles(sess, dirRelKey, "总结规则")
		if err != nil {
			return nil, err
		}
	}
	assign, ok := sess.Assignments[relPath]
	if !ok {
		return nil, fmt.Errorf("文件 %s 无会话赋值，请先完成单次赋值再总结规则", relPath)
	}
	if len(assign.coveredDimensions()) == 0 {
		return nil, fmt.Errorf("文件 %s 的赋值未指定任何维度，无可总结内容", relPath)
	}
	if sess.Snapshot == nil {
		return nil, fmt.Errorf("该会话尚未完成全量识别，缺少规则总结所需的文件上下文，请先识别")
	}
	fileRec := snapshotFileOf(sess.Snapshot, relPath)
	if fileRec == nil {
		return nil, fmt.Errorf("识别全景中无文件 %s 的记录（磁盘可能与快照不一致），请重新识别", relPath)
	}
	// 序列形态路径（建议即所见即所入）：样本属序列群且集名对得上群键元素段
	// 组合时产出序列形态规则（sibling 形态跟随 + 集名捕获，无目录层锚定）；
	// 属序列群但对不上段组合时退回既有结构映射形态并附说明行；不属序列群
	// 完全走既有形态（与现状零差异）。
	out := trySequenceFormSummary(relPath, fileRec.FileName, &assign, sess.Snapshot)
	if out.matched {
		out.sugg.HitCount, out.sugg.HitTotal, out.sugg.Samples = previewSequenceFormRules(out.rules, sess.Snapshot, relPath)
		if dirAddressed {
			out.sugg.RelPath = dirRelKey
		}
		return out.sugg, nil
	}
	sugg, rules, err := buildStructMapRules(sess.RootPath, relPath, fileRec.FileName, &assign)
	if err != nil {
		if !out.unmatched {
			return nil, err
		}
		// 集名纯手打（无任何结构段可映射）：建议卡退回手工指定形态并附说明行
		//（剧本A 边界行——对不上段组合时如实呈现，不硬造提取式规则）。
		sugg = manualOnlySeqSuggestion(relPath, &assign)
		if dirAddressed {
			sugg.RelPath = dirRelKey
		}
		sugg.HitTotal = len(sess.Snapshot.Files)
		return sugg, nil
	}
	if out.unmatched {
		sugg.Notes = append(sugg.Notes, seqNameUnmatchedNote)
	}
	sugg.HitCount, sugg.HitTotal, sugg.Samples = previewStructRules(rules, sess.Snapshot, relPath)
	if dirAddressed {
		sugg.RelPath = dirRelKey
	}
	return sugg, nil
}

// manualOnlySeqSuggestion 集名对不上段组合且无任何结构段可映射时的退回建议卡：
// 各维度按手工指定呈现（不参与泛化）+ 说明行——无将入档条目（Rules 空）。
func manualOnlySeqSuggestion(relPath string, assign *SessionAssignment) *RuleSuggestion {
	sugg := &RuleSuggestion{RelPath: relPath, Summary: "全部赋值维度为手工指定（不参与泛化）"}
	add := func(dim MetadataDimension, value string) {
		sugg.ManualDims = append(sugg.ManualDims, fmt.Sprintf("%s=%s（手工指定，不参与泛化）", qnaDimensionLabel(dim), value))
		sugg.Mappings = append(sugg.Mappings, RuleSuggestionMapping{Dimension: string(dim), From: "手工指定"})
	}
	if assign.Author != "" {
		add(DimAuthor, assign.Author)
	}
	if assign.Title != "" {
		add(DimTitle, assign.Title)
	}
	if assign.Collection != "" {
		add(DimCollection, assign.Collection)
	}
	for _, tag := range assign.Tags {
		add(DimTag, tag)
	}
	sugg.Notes = append(sugg.Notes, seqNameUnmatchedNote)
	return sugg
}

// previewStructRules 命中预览（全库）：全库文件逐一经组合规则求值（各规则独立
// 上下文，与流水线逐规则求值同构），全部命中才算覆盖；统计除样本文件外的命中
// 数与全库文件总数，样例取跨目录（快照序）前 3 个求值结论（附目录键）。
func previewStructRules(rules []RuleEntry, snap *RecognitionSnapshot, selfRelPath string) (int, int, []RuleSuggestionSample) {
	hitCount, hitTotal := 0, 0
	var samples []RuleSuggestionSample
	for i := range snap.Files {
		f := &snap.Files[i]
		hitTotal++
		in := &MatchInput{FileName: f.FileName, DirNames: ExtractDirLevels(f.RelPath), RelPath: f.RelPath}
		var decls []MetadataDeclaration
		all := true
		for j := range rules {
			ctx := &ParseContext{}
			if !rules[j].Pattern.Evaluate(in, nil, ctx) {
				all = false
				break
			}
			for _, action := range rules[j].Extractions {
				if decl, ok := action.Apply(rules[j].ID, ctx); ok {
					decls = append(decls, decl)
				}
			}
		}
		if !all || len(decls) == 0 {
			continue
		}
		if f.RelPath == selfRelPath {
			continue
		}
		hitCount++
		if len(samples) < 3 {
			parts := make([]string, 0, len(decls))
			for _, decl := range decls {
				parts = append(parts, qnaDimensionLabel(decl.Dimension)+"="+decl.Value)
			}
			samples = append(samples, RuleSuggestionSample{
				FileName: f.FileName, DirRelKey: f.DirRelKey, Result: strings.Join(parts, "，"),
			})
		}
	}
	return hitCount, hitTotal, samples
}
