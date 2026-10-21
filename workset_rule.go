package main

// 集规则泛化入口（已归集泛化入口 · 迭代三 · 剧本C）：泛化主体=已归集的集
// 本身（ConfirmedCandidates 卡整体——target 寻址，key+members+confirmedSetName
// 为推导输入），不再绕道集里单个作品的表单。三动作：
//   - ApplyWorkSetRuleSummary  集规则草稿推导（summarizeWorkSetRule）：多成员
//     取共同形态（代表成员出形态、全员校验复现——非单样本），复用迭代二
//     rule_summarizer_seq.go 机制（snapshotGroupOf 反查、seqNameSpanOf 跨多段
//     含分隔符/剥壳）；命中预览走既有 MergedRecognition 全库求值；同时组装
//     调整面线索清单（目录各层〔含剥壳变体〕+成员文件名段+序号段+库根名）。
//     推导失败（集名对不上段组合等）如实返回原因（Failed 态，线索清单照发
//     ——调整规则仍可用）。
//   - ApplyWorkSetRuleGeneralize 泛化入档（generalizeWorkSetRule）：草稿即
//     所见即所入（同一推导复跑），复用 generalize 档案写（ArchiveWriteGeneralize）
//     与消费判据（consumeScopedAssignments，作用域=集成员）+ 验证轮。
//   - ApplyWorkSetRuleAdjust   调整面保存（saveWorkSetRuleAdjust）：线索逐项
//     指派（维度←线索 / 集名捕获段组合 / 可变段 / 目录锚定 / 不用；末段序号
//     默认已指）→既有规则构造器组装（buildSequenceRule / structDirPlan / 集
//     名捕获规则构造），全员自检后入档（效果与泛化入档相同：验证轮+声明接管）。

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// setClueSegPrefix 集规则调整面的成员文件名段线索引用前缀（setSegN，N 为非
// 分隔元素位序、1 起——与呈现位序一致）；序号元素同编号空间、Kind=setSeq。
const setClueSegPrefix = "setSeg"

// setClueKindSeg / setClueKindSeq 文件名段 / 序号段的线索类别。
const (
	setClueKindSeg = "setSeg"
	setClueKindSeq = "setSeq"
)

// SetRuleMember 集规则草稿的成员呈现（成员列表点击跳文件表单的数据源）。
type SetRuleMember struct {
	RelPath  string `json:"relPath"`
	FileName string `json:"fileName"`
	// SeqText 集内序号原文（#N 高亮呈现）。
	SeqText string `json:"seqText,omitempty"`
}

// SetRuleClue 调整面线索清单条目（剧本步4）：目录各层（含剥壳变体）/ 成员
// 文件名段 / 序号段 / 库根名。Ref 词汇：rootName / dirLevel[n] /
// dirLevelStripped[n]（与结构分段线索集同一空间）+ setSegN（集面板自有）。
type SetRuleClue struct {
	Ref   string `json:"ref"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Value string `json:"value"`
	// Ordinal 文件名段的非分隔元素位序（1 起；序号段同编号空间）。
	Ordinal int `json:"ordinal,omitempty"`
	// Shelled 括号组段标记（Value 含括号原文）；Inner 剥壳内名。
	Shelled bool   `json:"shelled,omitempty"`
	Inner   string `json:"inner,omitempty"`
}

// SetRuleSuggestion summarizeWorkSetRule 的应答：集整体推导的规则草稿。失败
// 态（Failed=true，剧本步5a）时 Shape/Summary/Rules 缺席、FailReason 如实呈
// 现（「泛化成规则」置灰、「调整规则」仍可用——Clues 与默认指派照发）。
type SetRuleSuggestion struct {
	Target      string          `json:"target"`
	SetName     string          `json:"setName"`
	MemberCount int             `json:"memberCount"`
	Members     []SetRuleMember `json:"members"`
	// Shape 共同形态人话（如「[作者]_[作品号]_任意段_序号」）。
	Shape string `json:"shape,omitempty"`
	// Summary 人话摘要（规则草稿首行）。
	Summary string `json:"summary,omitempty"`
	// Mappings 逐维度映射说明（集名捕获行）。
	Mappings []RuleSuggestionMapping `json:"mappings,omitempty"`
	// Rules 规则草稿表达式（sibling 形态跟随 + 集名捕获）。
	Rules []*GeneratedRuleView `json:"rules,omitempty"`
	// MemberOnly 「仅序列成员产出」已勾（草稿恒真）。
	MemberOnly bool `json:"memberOnly,omitempty"`
	// HitCount / HitTotal / Samples 命中预览（全库求值，含本集成员；样例偏
	// 好跨目录 3 条，不足回落同目录）。
	HitCount int                    `json:"hitCount,omitempty"`
	HitTotal int                    `json:"hitTotal,omitempty"`
	Samples  []RuleSuggestionSample `json:"samples,omitempty"`
	// Failed / FailReason 剧本步5a 失败态（如实呈现推导不出的原因）。
	Failed     bool   `json:"failed,omitempty"`
	FailReason string `json:"failReason,omitempty"`
	// Clues 调整面线索清单（成功/失败两态均携带——失败时调整规则仍可用）。
	Clues []SetRuleClue `json:"clues,omitempty"`
	// 调整面默认指派（草稿已带入的初值：集名捕获=自动推导的组合、末段序号
	// 默认已指、非捕获段标可变）。
	DefaultCollectionSegRefs []string `json:"defaultCollectionSegRefs,omitempty"`
	DefaultVariableSegRefs   []string `json:"defaultVariableSegRefs,omitempty"`
	DefaultSeqSegRef         string   `json:"defaultSeqSegRef,omitempty"`
	DefaultMemberOnly        bool     `json:"defaultMemberOnly,omitempty"`
}

// setRuleAdjustInput 调整面保存的指派载荷（saveWorkSetRuleAdjust）：每条线索
// 指派到「维度←线索 / 集名捕获段组合 / 可变 / 目录锚定 / 不用」之一——维度
// 指派复用请求的 AuthorClueRef/TitleClueRef/TagClueRefs 字段（可指向目录层
// 或文件名段线索）；集名捕获为段组合（多条 setSeg 引用，须连续）；可变段与
// 不用段在构造上同为泛化消费（呈现意图差异）；目录锚定限定目录层线索。
type setRuleAdjustInput struct {
	AuthorClueRef     string
	TitleClueRef      string
	TagClueRefs       []string
	CollectionSegRefs []string
	VariableSegRefs   []string
	SeqSegRef         string
	DirAnchorRef      string
	MemberOnly        bool
}

// errSetNotConfirmed target 缺席已归集分仓的统一错误（剧本步5c 文案）。
var errSetNotConfirmed = errors.New("集不在已归集清单内（可能已被识别结构变化移除）")

// confirmedCandidateOf 快照 confirmed 分仓内按目标键取已归集候选卡（集规则
// 泛化的寻址口径，与前端已归集卡同源）。缺席=会话已刷新或识别结构已变化。
func confirmedCandidateOf(snap *RecognitionSnapshot, target string) *SnapCandidate {
	if snap == nil || target == "" {
		return nil
	}
	for i := range snap.ConfirmedCandidates {
		if snap.ConfirmedCandidates[i].target() == target {
			return &snap.ConfirmedCandidates[i]
		}
	}
	return nil
}

// workSetContext 集规则的推导上下文：已归集卡 + 确认集名 + 代表成员（首个
// 携带元素数组的成员——形态与线索的代表）。
type workSetContext struct {
	cand    *SnapCandidate
	setName string
	repIdx  int
}

// loadWorkSetContext 载入集上下文：target 寻址 confirmed 分仓 → 确认集名
// （卡面回带 → 成员赋值现值 → 候选键兜底）→ 代表成员。
func loadWorkSetContext(sess *WorkbenchSession, target string) (*workSetContext, error) {
	if sess.Snapshot == nil {
		return nil, errors.New("该会话尚未完成全量识别，无已归集集可寻址，请先识别")
	}
	cand := confirmedCandidateOf(sess.Snapshot, target)
	if cand == nil {
		return nil, errSetNotConfirmed
	}
	if len(cand.Members) == 0 {
		return nil, errors.New("该集的成员清单为空（可能已被识别结构变化移除），请刷新后重试")
	}
	setName := cand.ConfirmedSetName
	if setName == "" {
		setName = confirmedCollectionOf(*cand, sess.Assignments)
	}
	if setName == "" {
		setName = cand.Key
	}
	repIdx := -1
	for i := range cand.Members {
		if len(cand.Members[i].Elements) > 0 {
			repIdx = i
			break
		}
	}
	if repIdx < 0 {
		return nil, errors.New("集成员无文件名元素记录（可能已被识别结构变化移除），请重新识别后重试")
	}
	return &workSetContext{cand: cand, setName: setName, repIdx: repIdx}, nil
}

// workSetDraft 集整体序列形态草稿（推导成功态产物；泛化入档复跑同一构造——
// 建议即所见即所入）。span 供调整面默认指派（集名捕获组合）换算。
type workSetDraft struct {
	setName string
	shape   string
	summary string
	mapping string
	span    seqNameSpan
	rules   []RuleEntry
}

// deriveWorkSetSeqDraft 集整体草稿推导（纯推导，不落库）：代表成员出共同形态
// （群归属 snapshotGroupOf 反查、段组合 seqNameSpanOf 跨多段含分隔符/剥壳——
// 迭代二机制复用），随后以集全体成员校验（每成员段组合可命中 + 捕获规则复
// 现集名值）——非单样本：任一成员不合形即失败并如实返回原因（剧本步5a）。
func deriveWorkSetSeqDraft(snap *RecognitionSnapshot, ws *workSetContext) (workSetDraft, string) {
	cand := ws.cand
	rep := &cand.Members[ws.repIdx]
	repFile := snapshotFileOf(snap, rep.RelPath)
	if repFile == nil {
		return workSetDraft{}, fmt.Sprintf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", rep.RelPath)
	}
	group := snapshotGroupOf(snap, rep.RelPath)
	if group == nil {
		return workSetDraft{}, "集成员不构成同形序列群，无法按共同形态推导（可在调整规则中手动指派）"
	}
	repMember := &SnapGroupMember{RelPath: rep.RelPath, SeqText: rep.SeqText, Elements: rep.Elements}
	span, ok := seqNameSpanOf(repMember, group.Key, ws.setName)
	if !ok {
		return workSetDraft{}, fmt.Sprintf("无法从集名推导捕获：集名「%s」无法对应成员文件名的段组合（可在调整规则中手动指派集名捕获段）", ws.setName)
	}
	siblingRule, _, err := buildSequenceRule(rep.RelPath, group.Key, group)
	if err != nil {
		return workSetDraft{}, fmt.Sprintf("无法构造序列定群规则：%v", err)
	}
	captureRule, ok := buildSeqNameCaptureRule(rep.RelPath, repFile.FileName, repMember, span, ws.setName)
	if !ok {
		return workSetDraft{}, fmt.Sprintf("无法从集名推导捕获：集名「%s」在成员「%s」上无法构造捕获规则", ws.setName, rep.RelPath)
	}
	// 全员校验（集为主体，非首样本）：每成员段组合可命中且捕获规则复现集名。
	for i := range cand.Members {
		if i == ws.repIdx {
			continue // 代表成员已由构造自检覆盖
		}
		mem := &cand.Members[i]
		file := snapshotFileOf(snap, mem.RelPath)
		if file == nil {
			return workSetDraft{}, fmt.Sprintf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", mem.RelPath)
		}
		memMember := &SnapGroupMember{RelPath: mem.RelPath, SeqText: mem.SeqText, Elements: mem.Elements}
		if _, ok := seqNameSpanOf(memMember, group.Key, ws.setName); !ok {
			return workSetDraft{}, fmt.Sprintf("无法从集名推导捕获：成员「%s」的文件名无法按同一形态提取集名「%s」（非首样本校验）", mem.RelPath, ws.setName)
		}
		ctx := &ParseContext{}
		if !captureRule.Pattern.Evaluate(structSampleInput(mem.RelPath, file.FileName), nil, ctx) {
			return workSetDraft{}, fmt.Sprintf("共同形态校验失败：捕获规则未命中成员「%s」（非首样本校验）", mem.RelPath)
		}
		decl, ok := captureRule.Extractions[0].Apply(captureRule.ID, ctx)
		if !ok || decl.Value != ws.setName {
			return workSetDraft{}, fmt.Sprintf("共同形态校验失败：成员「%s」的集名提取结果与「%s」不一致", mem.RelPath, ws.setName)
		}
	}
	shape := seqShapePhrase(rep.Elements, span)
	return workSetDraft{
		setName: ws.setName,
		shape:   shape,
		summary: fmt.Sprintf("同形序列成员（%s 形态）→ 作品集 = 段组合『%s』，仅序列成员产出", shape, ws.setName),
		mapping: seqMappingPhrase(rep.Elements, span),
		span:    span,
		rules:   []RuleEntry{siblingRule, captureRule},
	}, ""
}

// ApplyWorkSetRuleSummary 集规则草稿推导动作（summarizeWorkSetRule）：target
// 寻址 confirmed 分仓卡，以集整体推导序列形态草稿 + 命中预览（全库求值）+
// 调整面线索清单与默认指派。推导失败返回 Failed 态应答（非错误——面板呈现
// 失败文案、「泛化成规则」置灰、「调整规则」仍可用）；target 缺席已归集
// （剧本步5c）回错误。
func (m *SessionManager) ApplyWorkSetRuleSummary(root, target string) (*SetRuleSuggestion, error) {
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	ws, err := loadWorkSetContext(sess, target)
	if err != nil {
		return nil, err
	}
	sugg := &SetRuleSuggestion{
		Target: target, SetName: ws.setName, MemberCount: len(ws.cand.Members),
		DefaultMemberOnly: true,
	}
	for i := range ws.cand.Members {
		mem := &ws.cand.Members[i]
		file := snapshotFileOf(sess.Snapshot, mem.RelPath)
		if file == nil {
			return nil, fmt.Errorf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", mem.RelPath)
		}
		sugg.Members = append(sugg.Members, SetRuleMember{RelPath: mem.RelPath, FileName: file.FileName, SeqText: mem.SeqText})
	}
	clues, segPlan := workSetRuleClues(sess.RootPath, ws)
	sugg.Clues = clues
	sugg.DefaultSeqSegRef = segPlan.seqRef()
	draft, failReason := deriveWorkSetSeqDraft(sess.Snapshot, ws)
	if failReason != "" {
		sugg.Failed, sugg.FailReason = true, failReason
		// 失败态默认指派仍给出可调初值：序号段已指、集名捕获留空（手动起建）。
		return sugg, nil
	}
	sugg.Shape = draft.shape
	sugg.Summary = draft.summary
	sugg.Mappings = []RuleSuggestionMapping{{Dimension: string(DimCollection), From: draft.mapping}}
	sugg.MemberOnly = true
	for i := range draft.rules {
		sugg.Rules = append(sugg.Rules, generatedRuleViewOf(&draft.rules[i]))
	}
	sugg.DefaultCollectionSegRefs = segPlan.spanRefs(draft.span)
	sugg.DefaultVariableSegRefs = segPlan.otherSegRefs(sugg.DefaultCollectionSegRefs)
	sugg.HitCount, sugg.HitTotal, sugg.Samples = previewWorkSetRules(
		draft.rules, sess.Snapshot, ws.cand.Dir, draft.rules[len(draft.rules)-1].ID)
	return sugg, nil
}

// ApplyWorkSetRuleGeneralize 集规则泛化入档动作（generalizeWorkSetRule）：
// 复跑草稿推导（建议即所见即所入）→ generalize 档案写单点入档 → 集成员
// 作用域消费剥离（消费判据与 ad6a748 版同一实现）→ 验证轮排定。
func (m *SessionManager) ApplyWorkSetRuleGeneralize(root, target string) (*GenerationResult, error) {
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	ws, err := loadWorkSetContext(sess, target)
	if err != nil {
		return nil, err
	}
	draft, failReason := deriveWorkSetSeqDraft(sess.Snapshot, ws)
	if failReason != "" {
		return nil, errors.New(failReason)
	}
	return m.commitWorkSetRules(root, ws, sess.Snapshot, draft.rules)
}

// commitWorkSetRules 集规则入档共用落地（泛化入档与调整面保存同终点）：
// 档案写（ArchiveWriteGeneralize，单点串行）→ 集成员作用域消费剥离 → 验证
// 轮排定 → 生成结果摘要（Rules=组合规则全部条目）。
func (m *SessionManager) commitWorkSetRules(root string, ws *workSetContext, snap *RecognitionSnapshot, rules []RuleEntry) (*GenerationResult, error) {
	scope := make([]*SnapFile, 0, len(ws.cand.Members))
	for i := range ws.cand.Members {
		if f := snapshotFileOf(snap, ws.cand.Members[i].RelPath); f != nil {
			scope = append(scope, f)
		}
	}
	for i := range rules {
		if err := m.commitGeneratedRule(ArchiveWriteGeneralize, &rules[i]); err != nil {
			return nil, err
		}
	}
	if err := m.consumeScopedAssignments(root, scope, snap, rules); err != nil {
		m.warnf("集规则泛化消费赋值失败（规则已入档）: %v", err)
	}
	started := m.scheduleRecognitionRefresh(root)
	views := make([]*GeneratedRuleView, 0, len(rules))
	for i := range rules {
		views = append(views, generatedRuleViewOf(&rules[i]))
	}
	return &GenerationResult{Rule: views[0], Rules: views, VerificationStarted: started}, nil
}

// —— 调整面（剧本步4）：线索清单组装与指派→规则构造 ——

// workSetSegPlan 集成员文件名段的位序表：代表成员的非分隔元素依序编号
// （1 起，序号元素同编号空间、Kind=setSeq），支撑 setSegN 引用 ↔ 元素下标
// 换算与默认指派初值。
type workSetSegPlan struct {
	els      []SnapEl
	ordinals []int // 位序（1 起）→ 元素下标（全数组下标，含分隔元素）
}

// newWorkSetSegPlan 代表成员元素数组 → 位序表。
func newWorkSetSegPlan(els []SnapEl) *workSetSegPlan {
	plan := &workSetSegPlan{els: els}
	for i := range els {
		if els[i].Kind != elKindSep {
			plan.ordinals = append(plan.ordinals, i)
		}
	}
	return plan
}

// seqRef 序号段引用（首个序号元素；无序号元素为空）。
func (p *workSetSegPlan) seqRef() string {
	for i := range p.els {
		if p.els[i].Kind == elKindSeq {
			return setSegRefOf(p.ordinalOf(i))
		}
	}
	return ""
}

// ordinalOf 元素下标 → 位序（1 起；分隔元素返回 0）。
func (p *workSetSegPlan) ordinalOf(elIdx int) int {
	ord := 0
	for i := 0; i <= elIdx && i < len(p.els); i++ {
		if p.els[i].Kind != elKindSep {
			ord++
		}
	}
	if elIdx >= len(p.els) || p.els[elIdx].Kind == elKindSep {
		return 0
	}
	return ord
}

// elIndexOfRef setSegN 引用 → 元素下标（非法/越界返回 -1）。
func (p *workSetSegPlan) elIndexOfRef(ref string) int {
	n, ok := setSegOrdinalOf(ref)
	if !ok || n < 1 || n > len(p.ordinals) {
		return -1
	}
	return p.ordinals[n-1]
}

// spanRefs 自动草稿 span 的集名捕获段组合引用（区间内非分隔元素，元素序）。
func (p *workSetSegPlan) spanRefs(span seqNameSpan) []string {
	var refs []string
	for i := span.startEl; i <= span.endEl && i < len(p.els); i++ {
		if p.els[i].Kind != elKindSep {
			refs = append(refs, setSegRefOf(p.ordinalOf(i)))
		}
	}
	return refs
}

// otherSegRefs 除给定引用外、非序号的非分隔元素引用（默认标为可变段）。
func (p *workSetSegPlan) otherSegRefs(exclude []string) []string {
	skip := make(map[string]bool, len(exclude))
	for _, ref := range exclude {
		skip[ref] = true
	}
	var refs []string
	for i := range p.els {
		if p.els[i].Kind == elKindSep || p.els[i].Kind == elKindSeq {
			continue
		}
		ref := setSegRefOf(p.ordinalOf(i))
		if !skip[ref] {
			refs = append(refs, ref)
		}
	}
	return refs
}

// setSegRefOf 位序 → setSegN 引用。
func setSegRefOf(ordinal int) string {
	return fmt.Sprintf("%s%d", setClueSegPrefix, ordinal)
}

// setSegOrdinalOf 引用 → 位序（非法引用返回假）。
func setSegOrdinalOf(ref string) (int, bool) {
	if !strings.HasPrefix(ref, setClueSegPrefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(ref, setClueSegPrefix))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// workSetRuleClues 组装调整面线索清单（剧本步4 形态）：目录各层（含剥壳
// 变体——DirStructCluesOf 同源）+ 成员文件名各段（代表成员元素、序号段单列
// 类别）+ 库根名。返回线索与位序表。
func workSetRuleClues(rootPath string, ws *workSetContext) ([]SetRuleClue, *workSetSegPlan) {
	var clues []SetRuleClue
	for _, c := range DirStructCluesOf(rootPath, ws.cand.Dir) {
		clues = append(clues, SetRuleClue{Ref: c.Ref, Kind: c.Kind, Label: c.Label, Value: c.Value})
	}
	els := ws.cand.Members[ws.repIdx].Elements
	plan := newWorkSetSegPlan(els)
	for i := range els {
		if els[i].Kind == elKindSep {
			continue
		}
		ord := plan.ordinalOf(i)
		clue := SetRuleClue{
			Ref: setSegRefOf(ord), Ordinal: ord, Value: els[i].Text,
			Shelled: els[i].Shelled, Inner: els[i].Inner,
		}
		if els[i].Kind == elKindSeq {
			clue.Kind, clue.Label = setClueKindSeq, "序号段"
		} else {
			clue.Kind, clue.Label = setClueKindSeg, fmt.Sprintf("文件名段 %d", ord)
		}
		clues = append(clues, clue)
	}
	return clues, plan
}

// setRuleDimAssign 单条「维度←线索」指派。
type setRuleDimAssign struct {
	dim   MetadataDimension
	clue  SetRuleClue
	group string // 文件名段指派的捕获组名（目录层/库根名指派为空）
}

// ApplyWorkSetRuleAdjust 调整面保存动作（saveWorkSetRuleAdjust）：线索指派
// 结果 → 既有规则构造器组装（维度←目录层线索=structDirPlan 目录条目 /
// 维度←文件名段=捕获组折入文件名规则 / 集名捕获段组合=集名捕获规则 /
// 目录锚定=捕获规则模式 And 目录层锚 / 序号段=数字类消费）→ 全员自检 →
// 入档+消费+验证轮（效果与泛化入档相同）。
func (m *SessionManager) ApplyWorkSetRuleAdjust(root, target string, in setRuleAdjustInput) (*GenerationResult, error) {
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	ws, err := loadWorkSetContext(sess, target)
	if err != nil {
		return nil, err
	}
	clues, plan := workSetRuleClues(sess.RootPath, ws)
	byRef := make(map[string]SetRuleClue, len(clues))
	for _, c := range clues {
		byRef[c.Ref] = c
	}
	refOf := func(ref, field string) (SetRuleClue, error) {
		c, ok := byRef[ref]
		if !ok {
			return SetRuleClue{}, fmt.Errorf("%s 指派的线索 %q 不在本集线索清单内", field, ref)
		}
		return c, nil
	}

	// 序号段指派校验（须为序号段线索）。
	if in.SeqSegRef != "" {
		c, err := refOf(in.SeqSegRef, "序号段")
		if err != nil {
			return nil, err
		}
		if c.Kind != setClueKindSeq {
			return nil, fmt.Errorf("序号段指派须指向序号段线索，%q 不是序号段", in.SeqSegRef)
		}
	}
	// 目录锚定指派校验（须为目录层线索，含剥壳变体）。
	var anchor *SetRuleClue
	if in.DirAnchorRef != "" {
		c, err := refOf(in.DirAnchorRef, "目录锚定")
		if err != nil {
			return nil, err
		}
		if c.Kind != structClueDirLevel && c.Kind != structClueDirLevelStripped {
			return nil, fmt.Errorf("目录锚定须指向目录层线索，%q 不是目录层线索", in.DirAnchorRef)
		}
		anchor = &c
	}

	// 维度←线索指派解析（文件名段线索折入文件名规则捕获组；目录层/库根名
	// 线索单独条目）。标签逐条独立成组（tag1、tag2…——structGroupName 同形）。
	var dims []setRuleDimAssign
	appendDim := func(dim MetadataDimension, ref string, tagIndex int) error {
		if ref == "" {
			return nil
		}
		c, err := refOf(ref, qnaDimensionLabel(dim))
		if err != nil {
			return err
		}
		if c.Kind == setClueKindSeq {
			return fmt.Errorf("%s 不能指派到序号段（序号段仅可指派为序号）", qnaDimensionLabel(dim))
		}
		assign := setRuleDimAssign{dim: dim, clue: c}
		if c.Kind == setClueKindSeg {
			assign.group = structGroupName(dim, tagIndex)
		}
		dims = append(dims, assign)
		return nil
	}
	if err := appendDim(DimAuthor, in.AuthorClueRef, -1); err != nil {
		return nil, err
	}
	if err := appendDim(DimTitle, in.TitleClueRef, -1); err != nil {
		return nil, err
	}
	for i, ref := range in.TagClueRefs {
		if err := appendDim(DimTag, ref, i); err != nil {
			return nil, err
		}
	}
	// 可变段引用校验（在清单内即受；构造上与非捕获段同泛化消费）。
	for _, ref := range in.VariableSegRefs {
		if _, err := refOf(ref, "可变段"); err != nil {
			return nil, err
		}
	}

	rules, err := buildWorkSetAdjustedRules(ws, sess.Snapshot, plan, anchor, dims, in)
	if err != nil {
		return nil, err
	}
	return m.commitWorkSetRules(root, ws, sess.Snapshot, rules)
}

// buildWorkSetAdjustedRules 指派结果 → 规则集组装（纯构造，不落库）：
//   - sibling 定群条目（MemberOnly 且代表成员属序列群时——「仅序列成员产出」
//     条件的取值载体；复用 buildSequenceRule）；
//   - 目录层维度条目（维度←dirLevel/dirLevelStripped——structDirPlan 既有
//     构造器逐层独立成条）；
//   - 库根名维度常量（维度←rootName——独立条目，锚=至少一层目录）；
//   - 文件名规则（集名捕获段组合 + 维度捕获组 + 序号段数字类消费 + 其余
//     段泛化消费；目录锚定时模式=And[目录层锚, 文件名正则]）。
//
// 全员自检（集为主体）：每成员上文件名规则捕获值须复现集名（有组合时）
// 与各文件名段维度的对齐取值。
func buildWorkSetAdjustedRules(ws *workSetContext, snap *RecognitionSnapshot, plan *workSetSegPlan, anchor *SetRuleClue, dims []setRuleDimAssign, in setRuleAdjustInput) ([]RuleEntry, error) {
	cand := ws.cand
	rep := &cand.Members[ws.repIdx]
	repFile := snapshotFileOf(snap, rep.RelPath)
	if repFile == nil {
		return nil, fmt.Errorf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", rep.RelPath)
	}

	// 集名捕获段组合：引用 → 元素下标（排序去重；须连续非分隔段）。
	spanSet := make(map[int]bool)
	var spanIdx []int
	for _, ref := range in.CollectionSegRefs {
		idx := plan.elIndexOfRef(ref)
		if idx < 0 {
			return nil, fmt.Errorf("集名捕获段组合中的线索 %q 不是本集的文件名段线索", ref)
		}
		if !spanSet[idx] {
			spanSet[idx] = true
			spanIdx = append(spanIdx, idx)
		}
	}
	if len(spanIdx) > 1 {
		sort.Ints(spanIdx)
		for k := 1; k < len(spanIdx); k++ {
			for i := spanIdx[k-1] + 1; i < spanIdx[k]; i++ {
				if rep.Elements[i].Kind != elKindSep {
					return nil, errors.New("集名捕获段组合须为连续的文件名段")
				}
			}
		}
	}

	// 维度←文件名段指派的捕获组规划（元素下标 → 组名；一段一组不可重复）。
	captureByEl := make(map[int]string)
	groupOrdinal := make(map[string]int)
	for _, d := range dims {
		if d.clue.Kind != setClueKindSeg {
			continue
		}
		idx := plan.elIndexOfRef(d.clue.Ref)
		if idx < 0 {
			return nil, fmt.Errorf("指派线索 %q 无法定位到成员文件名段", d.clue.Ref)
		}
		if _, taken := captureByEl[idx]; taken {
			return nil, fmt.Errorf("文件名段 %s 被重复指派", d.clue.Ref)
		}
		if _, clash := groupOrdinal[d.group]; clash {
			return nil, fmt.Errorf("维度组 %s 被重复指派", d.group)
		}
		captureByEl[idx] = d.group
		groupOrdinal[d.group] = plan.ordinalOf(idx)
	}

	var rules []RuleEntry
	// ① sibling 定群（仅序列成员产出的条件载体）。
	if in.MemberOnly {
		if group := snapshotGroupOf(snap, rep.RelPath); group != nil {
			sibling, _, err := buildSequenceRule(rep.RelPath, group.Key, group)
			if err != nil {
				return nil, err
			}
			rules = append(rules, sibling)
		}
	}

	// ② 目录层维度条目（同层合一条，复用 structDirPlan；锚定名取实际目录名）。
	dirPlans := make(map[string]*structDirPlan)
	var dirOrder []string
	for _, d := range dims {
		if d.clue.Kind != structClueDirLevel && d.clue.Kind != structClueDirLevelStripped {
			continue
		}
		n := structClueLevelOf(d.clue.Ref)
		if n <= 0 {
			return nil, fmt.Errorf("目录层线索 %q 的层级无法解析", d.clue.Ref)
		}
		dirName := dirNameAtLevel(rep.RelPath, n)
		if dirName == "" {
			return nil, fmt.Errorf("成员路径上无第 %d 层目录，线索 %q 无法指派", n, d.clue.Ref)
		}
		key := strconv.Itoa(n) + "\x00"
		if d.clue.Kind == structClueDirLevelStripped {
			shell, inner, ok := stripShell(dirName)
			if !ok || inner != d.clue.Value {
				return nil, fmt.Errorf("目录层剥壳线索 %q 与成员所在目录 %q 不符", d.clue.Ref, dirName)
			}
			key += shell.Name
		}
		entry, ok := dirPlans[key]
		if !ok {
			entry = &structDirPlan{level: n, name: dirName}
			if d.clue.Kind == structClueDirLevelStripped {
				shell, _, _ := stripShell(dirName)
				entry.shell = &shell
			}
			dirPlans[key] = entry
			dirOrder = append(dirOrder, key)
		}
		entry.slots = append(entry.slots, &structMappingSlot{dim: d.dim, value: d.clue.Value})
	}
	for _, key := range dirOrder {
		rule, ok := dirPlans[key].build(rep.RelPath, repFile.FileName)
		if !ok {
			return nil, fmt.Errorf("目录层指派（%s）无法在成员文件上自检通过", key)
		}
		rules = append(rules, rule)
	}

	// ③ 库根名维度常量条目（独立成条，锚=至少一层目录——buildStructMapRules 同款形态）。
	for _, d := range dims {
		if d.clue.Kind != structClueRootName {
			continue
		}
		anyDir, err := NewRegexPattern(string(TargetFileName), "^.*$", nil)
		if err != nil {
			return nil, err
		}
		location, err := NewLocationPattern(0, -1, anyDir)
		if err != nil {
			return nil, err
		}
		rules = append(rules, RuleEntry{
			ID:          generatedRuleID("qna-set", d.clue.Value),
			Description: fmt.Sprintf("泛化（集规则调整·库根名）：库根名「%s」声明%s（源自已归集集「%s」的调整）", d.clue.Value, qnaDimensionLabel(d.dim), ws.setName),
			Source:      RuleSourceQna,
			Enabled:     true,
			Pattern:     location,
			Extractions: []ExtractionAction{{Dimension: d.dim, From: FromConstant, Constant: d.clue.Value}},
		})
	}

	// ④ 文件名规则（集名捕获 + 维度捕获组 + 序号/其余段消费）。
	fileRule, err := buildWorkSetFilenameRule(ws, rep, repFile, plan, spanIdx, captureByEl, anchor, in)
	if err != nil {
		return nil, err
	}
	rules = append(rules, fileRule)

	// 全员自检（集为主体，非代表成员单样本）：逐成员复现维度取值；集名捕获
	// 以成员间一致为判据——组合可不同于确认集名（调整面正是「改集名取哪」
	// 的入口），全员同值即形态自洽。
	setNameValue := ""
	for i := range cand.Members {
		mem := &cand.Members[i]
		file := snapshotFileOf(snap, mem.RelPath)
		if file == nil {
			return nil, fmt.Errorf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", mem.RelPath)
		}
		ctx := &ParseContext{}
		if !fileRule.Pattern.Evaluate(structSampleInput(mem.RelPath, file.FileName), nil, ctx) {
			return nil, fmt.Errorf("调整指派未能在成员「%s」上命中（形态或锚定过窄）", mem.RelPath)
		}
		for _, action := range fileRule.Extractions {
			decl, ok := action.Apply(fileRule.ID, ctx)
			if !ok {
				return nil, fmt.Errorf("调整指派未能在成员「%s」上提取%s", mem.RelPath, qnaDimensionLabel(action.Dimension))
			}
			if action.Dimension == DimCollection {
				if decl.Value == "" {
					return nil, fmt.Errorf("调整指派的集名捕获在成员「%s」上提取为空", mem.RelPath)
				}
				if setNameValue == "" {
					setNameValue = decl.Value
				} else if decl.Value != setNameValue {
					return nil, fmt.Errorf("调整指派的集名捕获在成员间不一致：「%s」≠「%s」（组合须取全员同值段）", decl.Value, setNameValue)
				}
				continue
			}
			want := ""
			if ord, has := groupOrdinal[action.CaptureName]; has {
				want = segValueAtOrdinal(mem.Elements, ord)
			}
			if want != "" && decl.Value != want {
				return nil, fmt.Errorf("调整指派未能在成员「%s」上复现%s：提取「%s」≠ 期望「%s」",
					mem.RelPath, qnaDimensionLabel(action.Dimension), decl.Value, want)
			}
		}
	}
	return rules, nil
}

// segValueAtOrdinal 成员元素数组上按位序取对齐值（括号组取剥壳内名——elValue
// 口径；位序越界返回空串）。
func segValueAtOrdinal(els []SnapEl, ordinal int) string {
	ord := 0
	for i := range els {
		if els[i].Kind == elKindSep {
			continue
		}
		ord++
		if ord == ordinal {
			return elValue(els[i])
		}
	}
	return ""
}

// buildWorkSetFilenameRule 文件名规则构造（调整面）：按代表成员元素形态展开
// 正则——集名捕获组合覆盖为一个捕获组（类与 buildSeqNameCaptureRule 同款，
// 紧邻序号无分隔时排除尾随数字）；维度指派段各自成捕获组（括号组开/闭字面
// +内名类，取剥壳内名；普通段非下划线非空类）；序号段=数字类、其余段=泛化
// 消费（writeSeqElementConsumption 同款）；目录锚定时模式=And[目录层锚,
// 文件名正则]（组合规则须全部命中——消费判据与全库求值同构）。自检由调用
// 方的全员校验完成。
func buildWorkSetFilenameRule(ws *workSetContext, rep *SnapCandMember, repFile *SnapFile, plan *workSetSegPlan, spanIdx []int, captureByEl map[int]string, anchor *SetRuleClue, in setRuleAdjustInput) (RuleEntry, error) {
	els := rep.Elements
	base, off := seqElementBase(els)
	hasSpan := len(spanIdx) > 0
	groups := make([]string, 0, len(captureByEl)+1)
	var expr strings.Builder
	expr.WriteString("^")
	if hasSpan {
		cs, ce := off[spanIdx[0]], off[spanIdx[len(spanIdx)-1]+1]
		class := ".+?"
		end := spanIdx[len(spanIdx)-1]
		if end+1 < len(els) && els[end+1].Kind == elKindSeq && off[end+1] == ce && !els[end+1].Shelled {
			class = "[^0-9](?:.*?[^0-9])?"
		}
		for i := range els {
			if off[i+1] > cs {
				break
			}
			writeSeqElementCapture(&expr, els, i, captureByEl, &groups)
		}
		expr.WriteString("(?P<" + seqCaptureGroup + ">" + class + ")")
		groups = append(groups, seqCaptureGroup)
		for i := range els {
			if off[i] < ce {
				continue
			}
			if off[i] > ce {
				expr.WriteString(regexp.QuoteMeta(base[ce:off[i]]))
			}
			writeSeqElementCapture(&expr, els, i, captureByEl, &groups)
			ce = off[i+1]
		}
		if ce < len(base) {
			expr.WriteString(regexp.QuoteMeta(base[ce:]))
		}
	} else {
		for i := range els {
			writeSeqElementCapture(&expr, els, i, captureByEl, &groups)
		}
	}
	expr.WriteString(builtinTrailingExtExpr)
	regex, err := NewRegexPattern(string(TargetFileName), expr.String(), groups)
	if err != nil {
		return RuleEntry{}, fmt.Errorf("构造集规则文件名正则失败: %w", err)
	}
	extractions := make([]ExtractionAction, 0, len(groups))
	if hasSpan {
		collection := ExtractionAction{Dimension: DimCollection, From: FromCapture, CaptureName: seqCaptureGroup}
		if in.MemberOnly {
			collection.When = seqMemberOnlyCond()
		}
		extractions = append(extractions, collection)
	}
	for i := range els {
		if group, ok := captureByEl[i]; ok {
			extractions = append(extractions, ExtractionAction{Dimension: dimOfGroup(group), From: FromCapture, CaptureName: group})
		}
	}
	if len(extractions) == 0 {
		return RuleEntry{}, errors.New("调整指派未产生任何提取（集名捕获与维度指派至少须有其一）")
	}
	desc := fmt.Sprintf("泛化（集规则调整·集名提取）：已归集集「%s」的成员文件名段组合→作品集", ws.setName)
	if !hasSpan {
		desc = fmt.Sprintf("泛化（集规则调整）：已归集集「%s」的成员文件名形态→维度提取", ws.setName)
	}
	if in.MemberOnly {
		desc += "（仅序列成员产出）"
	}
	pattern := Pattern(regex)
	if anchor != nil {
		location, err := workSetAnchorPattern(rep.RelPath, anchor)
		if err != nil {
			return RuleEntry{}, err
		}
		pattern = And{location, regex}
		desc += "；目录锚定「" + anchor.Value + "」"
	}
	return RuleEntry{
		ID:          generatedRuleID("qna-setcap", repFile.FileName),
		Description: desc,
		Source:      RuleSourceQna,
		Enabled:     true,
		Pattern:     pattern,
		Extractions: extractions,
	}, nil
}

// dimOfGroup 捕获组名 → 含义维度（tagN 逐位归标签，其余即维度名——
// structGroupName 的反向）。
func dimOfGroup(group string) MetadataDimension {
	if strings.HasPrefix(group, "tag") {
		if _, err := strconv.Atoi(strings.TrimPrefix(group, "tag")); err == nil {
			return DimTag
		}
	}
	return MetadataDimension(group)
}

// writeSeqElementCapture 单元素的正则展开：维度指派段→捕获组（括号组取剥壳
// 内名、开/闭括号字面包裹；普通段非下划线非空类）；其余段按
// writeSeqElementConsumption 泛化消费。
func writeSeqElementCapture(b *strings.Builder, els []SnapEl, i int, captureByEl map[int]string, groups *[]string) {
	if group, ok := captureByEl[i]; ok {
		el := els[i]
		*groups = append(*groups, group)
		if el.Shelled {
			if p, ok := seqShellPairOf(el); ok {
				b.WriteString(regexp.QuoteMeta(p.open))
				b.WriteString("(?P<" + group + ">[^" + escapeForClass([]rune(p.close)[0]) + "]*)")
				b.WriteString(regexp.QuoteMeta(p.close))
				return
			}
		}
		b.WriteString("(?P<" + group + ">[^_]+?)")
		return
	}
	writeSeqElementConsumption(b, els, i)
}

// workSetAnchorPattern 目录锚定线索 → 目录层锚模式（structDirPlan 同款形态：
// 剥壳变体=壳形匹配、原文名=字面匹配；层深-1 换算同 qnaDirDepth）。锚定名
// 取成员实际所在目录名（线索值与目录不符时回可读错误）。
func workSetAnchorPattern(repRelPath string, anchor *SetRuleClue) (Pattern, error) {
	n := structClueLevelOf(anchor.Ref)
	if n <= 0 {
		return nil, fmt.Errorf("目录锚定线索 %q 的层级无法解析", anchor.Ref)
	}
	dirName := dirNameAtLevel(repRelPath, n)
	if dirName == "" {
		return nil, fmt.Errorf("成员路径上无第 %d 层目录，目录锚定 %q 无法指派", n, anchor.Ref)
	}
	expr := "^" + regexp.QuoteMeta(dirName) + "$"
	if anchor.Kind == structClueDirLevelStripped {
		shell, inner, ok := stripShell(dirName)
		if !ok || inner != anchor.Value {
			return nil, fmt.Errorf("目录锚定的剥壳线索 %q 与成员所在目录 %q 不符", anchor.Ref, dirName)
		}
		innerClass := ".+"
		if shell.Suffix != "" {
			innerClass = "[^" + escapeForClass(rune(shell.Suffix[0])) + "]*"
		}
		expr = "^" + regexp.QuoteMeta(shell.Prefix) + innerClass + regexp.QuoteMeta(shell.Suffix) + "$"
	}
	ancestor, err := NewRegexPattern(string(TargetFileName), expr, nil)
	if err != nil {
		return nil, err
	}
	return NewLocationPattern(n-1, n-1, ancestor)
}

// previewWorkSetRules 集规则命中预览（全库求值，与 previewSequenceFormRules
// 同口径——MergedRecognition 合并求值）：统计产出 captureID 声明的文件数
// （含本集成员——集为主体）与全库总数；样例偏好跨目录（相对 setDir）前 3
// 条，不足回落同目录。
func previewWorkSetRules(rules []RuleEntry, snap *RecognitionSnapshot, setDir, captureID string) (int, int, []RuleSuggestionSample) {
	if snap == nil || captureID == "" {
		return 0, 0, nil
	}
	files := make([]ScanFile, 0, len(snap.Files))
	for i := range snap.Files {
		f := &snap.Files[i]
		files = append(files, ScanFile{FileName: f.FileName, DirNames: ExtractDirLevels(f.RelPath), RelPath: f.RelPath})
	}
	conclusions := MergedRecognition(files, rules, nil, nil)
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
		if !hit {
			continue
		}
		hitCount++
		sample := RuleSuggestionSample{FileName: c.FileName, DirRelKey: c.DirRelKey, Result: strings.Join(parts, "，")}
		if c.DirRelKey != setDir {
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
