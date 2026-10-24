package main

// 序列形态建议（规则总结器 · 序列泛化路径）：总结样本（文件入口=该文件；目录
// 入口=子树首个有赋值文件——由 applyStructSummary 统一解析）属某序列群且其
// collection 赋值能对上「群键元素的连续段组合」（含元素间分隔 run、可跨多段、
// 边界元素含剥壳内名变体）时，产出序列形态规则——条目=sibling 形态跟随该群
//（复用 buildSequenceRule，含序号捕获）+ 集名捕获规则（捕获组=对上的段组合、
// 提取 collection←捕获组、when 原子「群组序号 ≥1」=仅序列成员产出）。不加
// 目录层 LocationPattern 锚定——捕获规则只匹配文件名形态，跨目录同形序列天然
// 接管（与目录层映射的本质差异）。集名对不上任何段组合：matched=false 且
// unmatched=true，调用方退回既有结构映射形态并附说明行；样本不属序列群或无
// 集名赋值：完全走既有形态（与现状零差异）。

import (
	"fmt"
	"regexp"
	"strings"
)

// seqNameUnmatchedNote 集名对不上段组合时的建议卡附注行（剧本A 边界行原文）。
const seqNameUnmatchedNote = "集名无法对应文件名段落，未生成提取式规则"

// seqCaptureGroup 序列形态集名捕获组名（提取 collection←捕获组）。
const seqCaptureGroup = "collection"

// seqMemberOnlyCond 「仅序列成员产出」when 原子（A4 手工款同款语义）：数值
// 区间式、操作数=群组序号、下界 1、无上界——落单文件群组序号取值不可用，含它
// 的比较恒假。规则编辑面的「仅序列成员产出」复选按此形态判读回显。
func seqMemberOnlyCond() *CondExpr {
	min := 1.0
	return &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSeq}, Min: &min}
}

// seqFormTry 序列形态总结尝试的产物：matched 与 unmatched 互斥——前者条目已
// 构造且自检通过（建议卡即所见即所入，generalize mode=struct 复用同一构造），
// 后者表示属序列群且有集名赋值但对不上段组合（退回既有形态+附注行）。
type seqFormTry struct {
	sugg      *RuleSuggestion
	rules     []RuleEntry
	matched   bool
	unmatched bool
}

// trySequenceFormSummary 序列形态建议尝试（summarize 与 generalize mode=struct
// 共用入口）。群归属经快照群卡反查（snapshotGroupOf——已识别成员的文件行不落
// 群组键，与集序记录 buildGroupSeqIndex 同一缺口口径），规则群与结构群均认。
func trySequenceFormSummary(relPath, fileName string, assign *SessionAssignment, snap *RecognitionSnapshot) seqFormTry {
	if snap == nil || assign == nil || assign.Collection == "" {
		return seqFormTry{}
	}
	group := snapshotGroupOf(snap, relPath)
	if group == nil {
		return seqFormTry{}
	}
	member := seqGroupMemberOf(group, relPath)
	if member == nil || len(member.Elements) == 0 || member.SeqText == "" {
		return seqFormTry{}
	}
	span, ok := seqNameSpanOf(member, group.Key, assign.Collection)
	if !ok {
		return seqFormTry{unmatched: true}
	}
	siblingRule, _, err := buildSequenceRule(relPath, fileName, group.Key, group)
	if err != nil {
		return seqFormTry{unmatched: true}
	}
	captureRule, ok := buildSeqNameCaptureRule(relPath, fileName, member, span, assign.Collection)
	if !ok {
		return seqFormTry{unmatched: true}
	}
	rules := []RuleEntry{siblingRule, captureRule}

	// 建议卡文案（剧本A 步2详）：摘要行=形态短语+段组合值+仅序列成员产出；
	// 映射行=文件名段位（含分隔符）。其余维度不参与序列形态条目（保持会话
	// 赋值承载，卡片按手工指定呈现）。
	mappings := []RuleSuggestionMapping{{
		Dimension: string(DimCollection),
		From:      seqMappingPhrase(member.Elements, span),
	}}
	var manualDims []string
	manual := func(dim MetadataDimension, value string) {
		manualDims = append(manualDims, fmt.Sprintf("%s=%s（手工指定，不参与泛化）", qnaDimensionLabel(dim), value))
		mappings = append(mappings, RuleSuggestionMapping{Dimension: string(dim), From: "手工指定"})
	}
	if assign.Author != "" {
		manual(DimAuthor, assign.Author)
	}
	if assign.Title != "" {
		manual(DimTitle, assign.Title)
	}
	for _, tag := range assign.Tags {
		manual(DimTag, tag)
	}
	sugg := &RuleSuggestion{
		RelPath:    relPath,
		Summary:    fmt.Sprintf("同形序列成员（%s 形态）→ 作品集 = 段组合『%s』，仅序列成员产出", seqShapePhrase(member.Elements, span), assign.Collection),
		Mappings:   mappings,
		ManualDims: manualDims,
	}
	for i := range rules {
		sugg.Rules = append(sugg.Rules, generatedRuleViewOf(&rules[i]))
	}
	return seqFormTry{sugg: sugg, rules: rules, matched: true}
}

// seqGroupMemberOf 群内按相对路径取成员记录。
func seqGroupMemberOf(group *SnapGroup, relPath string) *SnapGroupMember {
	for i := range group.Members {
		if group.Members[i].RelPath == relPath {
			return &group.Members[i]
		}
	}
	return nil
}

// seqNameSpan 集名值在成员元素数组上的连续组合区间：[startEl, endEl] 含两端
// （其间分隔 run 计入组合文本）；startInner/endInner=边界元素取剥壳内名变体。
type seqNameSpan struct {
	startEl    int
	endEl      int
	startInner bool
	endInner   bool
}

// seqElementBase 元素数组的字节边界（len+1 个）与去扩展名正文拼接（按序拼接
// 元素 text 即还原正文——元素模型的呈现约定）。
func seqElementBase(els []SnapEl) (string, []int) {
	off := make([]int, len(els)+1)
	var b strings.Builder
	for i := range els {
		b.WriteString(els[i].Text)
		off[i+1] = b.Len()
	}
	return b.String(), off
}

// seqNameSpanOf 集名值 ↔ 群键元素的连续段组合匹配：群键文本在成员正文内逐
// 出现处定位，完整落在键区间内且全部先于序号元素的连续元素组合（含元素间
// 分隔 run、可跨多段、边界元素可用剥壳内名变体）恰等于集名值时命中。键边界
// 切进元素内部（无元素对齐口径）或序号元素不在时视为对不上。
func seqNameSpanOf(member *SnapGroupMember, groupKey, value string) (seqNameSpan, bool) {
	if groupKey == "" || value == "" {
		return seqNameSpan{}, false
	}
	els := member.Elements
	seqIdx := -1
	for i := range els {
		if els[i].Kind == elKindSeq {
			seqIdx = i
			break
		}
	}
	if seqIdx < 0 {
		return seqNameSpan{}, false
	}
	base, off := seqElementBase(els)
	for from := 0; ; {
		idx := strings.Index(base[from:], groupKey)
		if idx < 0 {
			return seqNameSpan{}, false
		}
		ks, ke := from+idx, from+idx+len(groupKey)
		// 键区间元素收集：键可始于文件名中段（规则定群键不必然前缀），区间前
		// 的元素跳过；边界切进元素内部即失效。
		var keyEls []int
		valid := true
		for i := 0; i < seqIdx; i++ {
			if els[i].Kind == elKindSep {
				continue
			}
			if off[i+1] <= ks {
				continue
			}
			if off[i] < ks || off[i+1] > ke {
				valid = false
				break
			}
			keyEls = append(keyEls, i)
		}
		if valid && len(keyEls) > 0 {
			for s := 0; s < len(keyEls); s++ {
				for t := s; t < len(keyEls); t++ {
					if span, ok := seqSpanMatch(els, keyEls[s], keyEls[t], value); ok {
						return span, true
					}
				}
			}
		}
		from = ks + 1
	}
}

// seqSpanMatch 组合区间 [s..t] 的候选拼接（全文本 / 起点剥壳 / 终点剥壳 /
// 双剥壳——Shelled 元素才有内名变体）与集名值比对。
func seqSpanMatch(els []SnapEl, s, t int, value string) (seqNameSpan, bool) {
	if seqSliceText(els, s, t, false, false) == value {
		return seqNameSpan{startEl: s, endEl: t}, true
	}
	if els[s].Shelled && els[s].Inner != "" && seqSliceText(els, s, t, true, false) == value {
		return seqNameSpan{startEl: s, endEl: t, startInner: true}, true
	}
	if t != s && els[t].Shelled && els[t].Inner != "" {
		if seqSliceText(els, s, t, false, true) == value {
			return seqNameSpan{startEl: s, endEl: t, endInner: true}, true
		}
		if els[s].Shelled && els[s].Inner != "" && seqSliceText(els, s, t, true, true) == value {
			return seqNameSpan{startEl: s, endEl: t, startInner: true, endInner: true}, true
		}
	}
	return seqNameSpan{}, false
}

// seqSliceText 组合区间的拼接文本：边界元素可取剥壳内名，中间元素（含分隔
// run）按原文保留。
func seqSliceText(els []SnapEl, s, t int, startInner, endInner bool) string {
	var b strings.Builder
	for i := s; i <= t; i++ {
		switch {
		case i == s && startInner:
			b.WriteString(els[i].Inner)
		case i == t && endInner:
			b.WriteString(els[i].Inner)
		default:
			b.WriteString(els[i].Text)
		}
	}
	return b.String()
}

// buildSeqNameCaptureRule 集名捕获规则构造（v4 模板）：按样本成员元素形态
// 展开——组合区间由 {集名} 一位占位（段间分隔与边界括号一并入位），其余
// 非分隔元素泛化消费（数字段取 {序号}/数字类词、普通段借词占位、剥壳段
// 括号留字面），分隔 run 按原文字面；同形异值成员可命中（跨目录接管）。
// 特化段类（尾随数字排除等）废止——贪婪回溯+字面锚定承载切分；集名段与
// 序号段无分隔直连（相邻占位词）等词法失配返回假，调用方降级。不含目录
// 层锚定（跨目录接管）。自检：样例文件上捕获值必须复现集名赋值。
func buildSeqNameCaptureRule(relPath, fileName string, member *SnapGroupMember, span seqNameSpan, value string) (RuleEntry, bool) {
	els := member.Elements
	base, off := seqElementBase(els)
	// 捕获区间字节界（剥壳边界取内名起止；剥壳开/闭括号字节留字面）。
	cs := off[span.startEl]
	if span.startInner {
		cs += strings.Index(els[span.startEl].Text, els[span.startEl].Inner)
	}
	ce := off[span.endEl+1]
	if span.endInner {
		inner := els[span.endEl].Inner
		ce = off[span.endEl] + strings.Index(els[span.endEl].Text, inner) + len(inner)
	}
	pool := newCondxWordPool()
	if !pool.Take("集名") {
		return RuleEntry{}, false
	}
	tpl, err := condxTplFromEls(base, els, off, cs, ce, "集名", nil, pool)
	if err != nil {
		return RuleEntry{}, false
	}
	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		return RuleEntry{}, false
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-seqcap", fileName),
		Description: fmt.Sprintf("泛化（序列形态·集名提取）：文件名按「%s」模板，集名段→作品集（仅序列成员产出；源自「%s」的赋值）", tpl, relPath),
		Source:      RuleSourceQna,
		Enabled:     true,
		Conditions:  rows,
		Pattern:     pattern,
		Extractions: []ExtractionAction{{
			Dimension: DimCollection, From: FromCapture, CaptureName: condxVocab["集名"], Row: 1,
			When: seqMemberOnlyCond(),
		}},
	}
	ctx := &ParseContext{}
	if !pattern.Evaluate(structSampleInput(relPath, fileName), nil, ctx) {
		return RuleEntry{}, false
	}
	decl, ok := rule.Extractions[0].Apply(rule.ID, ctx)
	if !ok || decl.Value != value {
		return RuleEntry{}, false
	}
	return rule, true
}

// writeSeqElementConsumption 单元素的泛化消费（非捕获）：分隔 run 按原文字面；
// 括号组元素=开/闭括号字面 + 内名类（序号元素收窄为数字类）；普通词元素=非
// 下划线非空类（同形异值可命中——跨目录接管的形态泛化面）。
func writeSeqElementConsumption(b *strings.Builder, els []SnapEl, i int) {
	el := els[i]
	if el.Kind == elKindSep {
		b.WriteString(regexp.QuoteMeta(el.Text))
		return
	}
	if el.Shelled {
		if p, ok := seqShellPairOf(el); ok {
			b.WriteString(regexp.QuoteMeta(p.open))
			if el.Kind == elKindSeq {
				b.WriteString(seqDigitsExpr(el))
			} else {
				b.WriteString("[^" + escapeForClass([]rune(p.close)[0]) + "]*")
			}
			b.WriteString(regexp.QuoteMeta(p.close))
			return
		}
	}
	if el.Kind == elKindSeq {
		b.WriteString(seqDigitsExpr(el))
		return
	}
	b.WriteString(`[^_]+?`)
}

// seqDigitsExpr 序号元素的数字类：上界按序号原文位数与形态表序号宽度的较大者
// （容忍零填充宽度差异；形态序号 1-4 位，规则定群序号可更长）。
func seqDigitsExpr(el SnapEl) string {
	n := len(elValue(el))
	if n < 4 {
		n = 4
	}
	return fmt.Sprintf(`\d{1,%d}`, n)
}

// seqShellPairOf 括号组元素的包裹括号对（text = open+inner+close 完整成立时）。
func seqShellPairOf(el SnapEl) (bracketPair, bool) {
	for _, p := range bracketPairs {
		if el.Inner != "" && el.Text == p.open+el.Inner+p.close {
			return p, true
		}
	}
	return bracketPair{}, false
}

// seqShapePhrase 形态短语（摘要行括注）：非分隔元素依序映射为形态词元——作者
// 括号块=[作者]、作品号=[作品号]、序号=序号、组合区间（整体一个词元）与其余
// 变化位=任意段，以间隔 run 原文连接（如 `[作者]_[作品号]_任意段_序号`）。
func seqShapePhrase(els []SnapEl, span seqNameSpan) string {
	var b strings.Builder
	pending := ""
	spanDone := false
	for i := range els {
		if i >= span.startEl && i <= span.endEl {
			// 组合区间整体一个「任意段」词元（区间内分隔 run 并入词元）
			if !spanDone {
				spanDone = true
				b.WriteString(pending)
				pending = ""
				b.WriteString("任意段")
			}
			continue
		}
		el := els[i]
		if el.Kind == elKindSep {
			if b.Len() > 0 {
				pending += el.Text
			}
			continue
		}
		token := "任意段"
		switch el.Kind {
		case elKindAuthor:
			token = "[作者]"
		case elKindWorkId:
			token = "[作品号]"
		case elKindSeq:
			token = "序号"
		}
		b.WriteString(pending)
		pending = ""
		b.WriteString(token)
	}
	return b.String()
}

// seqMappingPhrase 映射行来源短语：组合区间的非分隔元素位序（1 起，含分隔
// 符）。序号为末位非分隔元素且组合终于其前一位时呈现「第 i..末−1 段」（剧本A
// 步2详样例措辞），否则「第 i..j 段」；单元素区间省略区间上界。
func seqMappingPhrase(els []SnapEl, span seqNameSpan) string {
	ord, startOrd, endOrd, seqOrd := 0, 0, 0, 0
	seqSeen := false
	for i := range els {
		if els[i].Kind == elKindSep {
			continue
		}
		ord++
		if i == span.startEl {
			startOrd = ord
		}
		if i == span.endEl {
			endOrd = ord
		}
		if els[i].Kind == elKindSeq && !seqSeen {
			seqSeen = true
			seqOrd = ord
		}
	}
	if seqSeen && seqOrd == ord && endOrd == seqOrd-1 {
		return fmt.Sprintf("文件名第 %d..末−1 段（含分隔符）", startOrd)
	}
	if startOrd == endOrd {
		return fmt.Sprintf("文件名第 %d 段（含分隔符）", startOrd)
	}
	return fmt.Sprintf("文件名第 %d..%d 段（含分隔符）", startOrd, endOrd)
}

// previewSequenceFormRules 序列形态命中预览（全库求值）：以候选规则集对全库
// 快照跑一遍合并求值（同目录兄弟全集 + 规则定群/结构群对齐 + when 原子求值
// ——与入档后重识别同一口径），统计除样本外产出集名声明的文件数与全库总数；
// 样例取跨目录（快照序）前 3 个求值结论（sibling 无目录锚定，跨目录样例天然
// 在场；不足时回落同目录样例）。
func previewSequenceFormRules(rules []RuleEntry, snap *RecognitionSnapshot, selfRelPath string) (int, int, []RuleSuggestionSample) {
	if snap == nil {
		return 0, 0, nil
	}
	files := make([]ScanFile, 0, len(snap.Files))
	for i := range snap.Files {
		f := &snap.Files[i]
		files = append(files, ScanFile{FileName: f.FileName, DirNames: ExtractDirLevels(f.RelPath), RelPath: f.RelPath})
	}
	conclusions := MergedRecognition(files, rules, nil, nil)
	captureID := ""
	for i := range rules {
		if _, isSibling := rules[i].Pattern.(*SiblingPattern); !isSibling {
			captureID = rules[i].ID
		}
	}
	selfDir := dirRelKeyOf(selfRelPath)
	hitCount, hitTotal := 0, len(snap.Files)
	var samples, fallback []RuleSuggestionSample
	for i := range conclusions {
		c := &conclusions[i]
		var parts []string
		hit := false
		for _, decl := range c.Final {
			if decl.RuleID == captureID {
				hit = true
				parts = append(parts, qnaDimensionLabel(decl.Dimension)+"="+decl.Value)
			}
		}
		if !hit || c.RelPath == selfRelPath {
			continue
		}
		hitCount++
		sample := RuleSuggestionSample{FileName: c.FileName, DirRelKey: c.DirRelKey, Result: strings.Join(parts, "，")}
		if c.DirRelKey != selfDir {
			samples = append(samples, sample)
		} else {
			fallback = append(fallback, sample)
		}
	}
	for len(samples) > 3 {
		fallback = append([]RuleSuggestionSample{samples[len(samples)-1]}, fallback...)
		samples = samples[:len(samples)-1]
	}
	for len(samples) < 3 && len(fallback) > 0 {
		samples = append(samples, fallback[0])
		fallback = fallback[1:]
	}
	return hitCount, hitTotal, samples
}
