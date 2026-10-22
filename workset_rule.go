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

// setClueDirSegPrefix 集规则调整面的目录名分段线索引用前缀（setDirSegN，N 为
// 集所在目录名的非分隔元素位序、1 起）。
const setClueDirSegPrefix = "setDirSeg"

// setClueKindSeg / setClueKindSeq / setClueKindDirSeg 文件名段 / 序号段 /
// 目录名分段的线索类别。
const (
	setClueKindSeg    = "setSeg"
	setClueKindSeq    = "setSeq"
	setClueKindDirSeg = "setDirSeg"
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
	// DefaultAuthorClueRef 默认作者指派的线索引用（目录格式草稿带出：一级
	// 目录 Artist 壳在场时=dirLevelStripped1；空=无默认作者指派）。
	DefaultAuthorClueRef string `json:"defaultAuthorClueRef,omitempty"`
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

// 集草稿类别：draftSeq=文件名共同形态（迭代三路径）；draftDir=目录名稳定
// 格式（迭代四路径——文件名无共同形态时的回退推导）。
const (
	draftSeq = iota
	draftDir
)

// workSetDraft 集整体草稿（推导成功态产物；泛化入档复跑同一构造——
// 建议即所见即所入）。span 供调整面默认指派（集名捕获组合）换算；目录格式
// 草稿的默认指派由 dirCollectionEl/dirAuthorRef 承载。
type workSetDraft struct {
	kind    int
	setName string
	shape   string
	summary string
	mapping string
	span    seqNameSpan
	rules   []RuleEntry
	// dirCollectionEl 目录格式草稿的默认集名捕获段（元素下标；-1=非目录草稿）。
	dirCollectionEl int
	// dirAuthorRef 目录格式草稿的默认作者线索引用（dirLevelStripped[n]；空=
	// 一级目录无 Artist 壳）。
	dirAuthorRef string
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
		kind:    draftSeq,
		setName: ws.setName,
		shape:   shape,
		summary: fmt.Sprintf("同形序列成员（%s 形态）→ 作品集 = 段组合『%s』，仅序列成员产出", shape, ws.setName),
		mapping: seqMappingPhrase(rep.Elements, span),
		span:    span,
		rules:   []RuleEntry{siblingRule, captureRule},
	}, ""
}

// deriveWorkSetDraft 集草稿统一推导入口（自动草稿优先级，剧本D 步1/2）：
// 文件名共同序列形态在→照旧（迭代二/三路径）；否则集所在目录名呈稳定格式
// →目录格式草稿（集名=末非数字段捕获、作者=一级目录 Artist 壳剥壳〔壳在场
// 时〕、数字段可变、每目录各自成集=天然语义——无 sibling/when 原子）；都
// 无→失败态如实返回原因（剧本步5——两路原因并存时并列呈现，文件名侧与
// 目录侧的不可推导事实同时可见）。
func deriveWorkSetDraft(snap *RecognitionSnapshot, ws *workSetContext) (workSetDraft, string) {
	seqDraft, seqFail := deriveWorkSetSeqDraft(snap, ws)
	if seqFail == "" {
		return seqDraft, ""
	}
	dirDraft, dirFail := deriveWorkSetDirDraft(snap, ws)
	if dirFail == "" {
		return dirDraft, ""
	}
	if seqFail != dirFail {
		dirFail = seqFail + "；" + dirFail
	}
	return workSetDraft{}, dirFail
}

// deriveWorkSetDirDraft 目录名稳定格式草稿推导（纯推导，不落库）：成员所在
// 目录名分段（括号段+分隔符感知）取稳定格式（段数/包裹/分隔一致，数字段值
// 可不同），类推断（纯数字段值→数字类 `\d+` 通配、其余→泛化段）；默认集名=
// 末非数字段捕获、作者=一级目录 Artist 壳剥壳（壳在场时）。条目：作者条目
// （structDirPlan 同款：层深锚定+ancestorDir 剥壳）+ 目录格式条目
// （LocationPattern 层深+段格式正则带命名捕获，提取 FromCapture——内核层
// ancestor 捕获绑入通路）。全员自检：每成员目录命中格式且集名捕获非空。
func deriveWorkSetDirDraft(snap *RecognitionSnapshot, ws *workSetContext) (workSetDraft, string) {
	cand := ws.cand
	rep := &cand.Members[ws.repIdx]
	repFile := snapshotFileOf(snap, rep.RelPath)
	if repFile == nil {
		return workSetDraft{}, fmt.Sprintf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", rep.RelPath)
	}
	// 稳定格式判定：全部成员所在目录名（当前候选恒单目录，多目录按同格式
	// 校验——段数/包裹/分隔一致才成稳定格式）。
	memberDirs := make([]string, 0, len(cand.Members))
	seen := make(map[string]bool, len(cand.Members))
	for i := range cand.Members {
		key := dirRelKeyOf(cand.Members[i].RelPath)
		if !seen[key] {
			seen[key] = true
			memberDirs = append(memberDirs, key)
		}
	}
	plan, failReason := newWorkSetDirSegPlan(cand.Dir, memberDirs)
	if failReason != "" {
		return workSetDraft{}, failReason
	}
	// 默认集名捕获 = 末非数字段。
	collectionEl := -1
	for k := len(plan.ordinals) - 1; k >= 0; k-- {
		if !plan.digit[plan.ordinals[k]] {
			collectionEl = plan.ordinals[k]
			break
		}
	}
	if collectionEl < 0 {
		return workSetDraft{}, "无法从目录名推导稳定格式：目录名段全为数字类，无标题段可指集名（可在调整规则中手动指派）"
	}
	var rules []RuleEntry
	// 作者条目：一级目录 Artist 壳在场时（集目录层深 ≥2 才有一级目录先祖）。
	dirAuthorRef := ""
	if plan.level >= 2 {
		if name1 := dirNameAtLevel(rep.RelPath, 1); name1 != "" {
			if shell, inner, ok := stripShell(name1); ok {
				entry := &structDirPlan{level: 1, name: name1, shell: &shell,
					slots: []*structMappingSlot{{dim: DimAuthor, value: inner}}}
				if rule, ok := entry.build(rep.RelPath, repFile.FileName); ok {
					rules = append(rules, rule)
					dirAuthorRef = fmt.Sprintf("%s%d", structClueDirLevelStripped, 1)
				}
			}
		}
	}
	dirRule, err := buildWorkSetDirRule(ws, plan, collectionEl, -1, setRuleAdjustInput{})
	if err != nil {
		return workSetDraft{}, fmt.Sprintf("无法构造目录格式规则：%v", err)
	}
	rules = append(rules, dirRule)
	// 全员自检（集为主体）：每成员目录命中段格式且集名捕获非空复现段值。
	for i := range cand.Members {
		mem := &cand.Members[i]
		file := snapshotFileOf(snap, mem.RelPath)
		if file == nil {
			return workSetDraft{}, fmt.Sprintf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", mem.RelPath)
		}
		if dirRelKeyOf(mem.RelPath) != plan.dirRelKey {
			return workSetDraft{}, fmt.Sprintf("成员「%s」不在集目录 %s 内，无法按目录名格式推导", mem.RelPath, plan.dirRelKey)
		}
		ctx := &ParseContext{}
		if !dirRule.Pattern.Evaluate(structSampleInput(mem.RelPath, file.FileName), nil, ctx) {
			return workSetDraft{}, fmt.Sprintf("共同格式校验失败：目录格式规则未命中成员「%s」（非首样本校验）", mem.RelPath)
		}
		decl, ok := dirRule.Extractions[0].Apply(dirRule.ID, ctx)
		if !ok || decl.Value == "" {
			return workSetDraft{}, fmt.Sprintf("共同格式校验失败：成员「%s」的集名提取结果为空（非首样本校验）", mem.RelPath)
		}
	}
	shape := dirSegShapePhrase(plan)
	summary := fmt.Sprintf("%s → 作品集=第 %d 段（每个这样的目录各自成集）", shape, plan.ordinalOf(collectionEl))
	if dirAuthorRef != "" {
		summary += fmt.Sprintf("；作者=%s %s剥壳", dirLevelLabel(1), structWrapDesc(rep.RelPath, 1))
	}
	return workSetDraft{
		kind:            draftDir,
		setName:         ws.setName,
		shape:           "成员文件名无共同序列形态；" + shape,
		summary:         summary,
		dirCollectionEl: collectionEl,
		dirAuthorRef:    dirAuthorRef,
		rules:           rules,
	}, ""
}

// ApplyWorkSetRuleSummary 集规则草稿推导动作（summarizeWorkSetRule）：target
// 寻址 confirmed 分仓卡，以集整体推导规则草稿（文件名共同形态优先；无共同
// 形态时回退目录名稳定格式——剧本D）+ 命中预览（全库求值）+ 调整面线索清单
// 与默认指派。推导失败返回 Failed 态应答（非错误——面板呈现失败文案、
// 「泛化成规则」置灰、「调整规则」仍可用）；target 缺席已归集
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
	clues, segPlan, dirPlan := workSetRuleClues(sess.RootPath, ws)
	sugg.Clues = clues
	sugg.DefaultSeqSegRef = segPlan.seqRef()
	draft, failReason := deriveWorkSetDraft(sess.Snapshot, ws)
	if failReason != "" {
		sugg.Failed, sugg.FailReason = true, failReason
		// 失败态默认指派仍给出可调初值：序号段已指、集名捕获留空（手动起建）。
		return sugg, nil
	}
	sugg.Shape = draft.shape
	sugg.Summary = draft.summary
	if draft.kind == draftDir {
		// 目录格式草稿：默认集名捕获=末非数字目录段、数字段标可变、作者=
		// 一级目录 Artist 壳（在场时）；「仅序列成员产出」不默认（集的载体
		// 是目录名格式而非序列形态——乱名成员非序列群，when 原子会令规则
		// 恒不产出=惰性规则）。DefaultMemberOnly 复位为 false（构造处预置
		// 的 true 只属于文件名共同形态路径）。
		sugg.DefaultMemberOnly = false
		collectionOrd := dirPlan.ordinalOf(draft.dirCollectionEl)
		sugg.Mappings = []RuleSuggestionMapping{{
			Dimension: string(DimCollection),
			From:      fmt.Sprintf("%s段 %d（末非数字段）", dirLevelLabel(dirPlan.level), collectionOrd),
		}}
		if draft.dirAuthorRef != "" {
			sugg.Mappings = append(sugg.Mappings, RuleSuggestionMapping{
				Dimension: string(DimAuthor), From: dirLevelLabel(1) + "剥壳",
			})
		}
		sugg.DefaultCollectionSegRefs = []string{setDirSegRefOf(collectionOrd)}
		sugg.DefaultVariableSegRefs = dirPlan.digitSegRefs(collectionOrd)
		sugg.DefaultAuthorClueRef = draft.dirAuthorRef
		for i := range draft.rules {
			sugg.Rules = append(sugg.Rules, generatedRuleViewOf(&draft.rules[i]))
		}
		sugg.HitCount, sugg.HitTotal, sugg.Samples = previewWorkSetRules(
			draft.rules, sess.Snapshot, ws.cand.Dir, draft.rules[len(draft.rules)-1].ID)
		return sugg, nil
	}
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
// 复跑草稿推导（建议即所见即所入——文件名形态优先、目录格式回退与草稿面
// 同一判定）→ generalize 档案写单点入档 → 集成员作用域消费剥离（消费判据
// 与 ad6a748 版同一实现）→ 验证轮排定。
func (m *SessionManager) ApplyWorkSetRuleGeneralize(root, target string) (*GenerationResult, error) {
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	ws, err := loadWorkSetContext(sess, target)
	if err != nil {
		return nil, err
	}
	draft, failReason := deriveWorkSetDraft(sess.Snapshot, ws)
	if failReason != "" {
		return nil, errors.New(failReason)
	}
	return m.commitWorkSetRules(root, ws, sess.Snapshot, draft.rules)
}

// commitWorkSetRules 集规则入档共用落地（泛化入档与调整面保存同终点）：
// 同系覆写（同一 target 此前入档的规则族先停用——再次泛化/调整=修订意图，
// 旧规则不再产出旧值遮蔽新值）→ 档案写（ArchiveWriteGeneralize，单点串行）
// → 谱系簿记翻新 → 集成员作用域消费剥离 → 验证轮排定 → 生成结果摘要
// （Rules=组合规则全部条目）。
func (m *SessionManager) commitWorkSetRules(root string, ws *workSetContext, snap *RecognitionSnapshot, rules []RuleEntry) (*GenerationResult, error) {
	scope := make([]*SnapFile, 0, len(ws.cand.Members))
	for i := range ws.cand.Members {
		if f := snapshotFileOf(snap, ws.cand.Members[i].RelPath); f != nil {
			scope = append(scope, f)
		}
	}
	// 同系旧族：会话谱系簿记里该 target 上一代入档的规则 ID。
	var superseded []string
	if sess, err := m.loadSession(root); err == nil {
		superseded = sess.WorkSetRuleLineage[ws.cand.target()]
	}
	if err := m.UpdateProfile(ArchiveWriteGeneralize, func(p *RuleProfile) error {
		for i := range p.Rules {
			if p.Rules[i].Enabled && slicePos(superseded, p.Rules[i].ID) >= 0 {
				p.Rules[i].Enabled = false
			}
		}
		for i := range rules {
			rules[i].ID = ensureRuleIDUnique(p, rules[i].ID)
			p.Rules = append(p.Rules, rules[i])
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// 谱系簿记翻新：新族 ID 取代旧族记录（落库失败仅告警——规则已入档，
	// 退化为并存〔捕获组按规则隔离，互不窃取〕）。
	ids := make([]string, 0, len(rules))
	for i := range rules {
		ids = append(ids, rules[i].ID)
	}
	if _, err := m.mutateSession(root, func(sess *WorkbenchSession) error {
		if sess.WorkSetRuleLineage == nil {
			sess.WorkSetRuleLineage = map[string][]string{}
		}
		sess.WorkSetRuleLineage[ws.cand.target()] = ids
		return nil
	}); err != nil {
		m.warnf("集规则谱系簿记落盘失败（规则已入档）: %v", err)
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

// setDirSegRefOf 位序 → setDirSegN 引用。
func setDirSegRefOf(ordinal int) string {
	return fmt.Sprintf("%s%d", setClueDirSegPrefix, ordinal)
}

// setDirSegOrdinalOf 引用 → 位序（非法引用返回假）。
func setDirSegOrdinalOf(ref string) (int, bool) {
	if !strings.HasPrefix(ref, setClueDirSegPrefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(ref, setClueDirSegPrefix))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// workSetDirSegPlan 集所在目录名的分段位序表（剧本D 目录名分段）：元素数组
// 来自 splitFilenameElements（括号段+分隔符感知，分隔 run 保留原文——空格/
// 连字符经归一折叠为分隔位）；ordinals 为非分隔元素位序（1 起）→ 元素下标；
// digit 按位标记数字类（纯数字段值→`\d+` 通配，值可不同；类推断对全部成员
// 目录名聚合取交——任一目录该位非纯数字即归泛化段）。
type workSetDirSegPlan struct {
	dirRelKey string
	level     int          // 集所在目录层深（1 起）
	name      string       // 集所在目录名（basename）
	els       []SnapEl     // 目录名元素数组（含分隔 run）
	ordinals  []int        // 位序（1 起，非分隔）→ 元素下标
	digit     map[int]bool // 元素下标 → 数字类
}

// newWorkSetDirSegPlan 集目录名 → 分段位序表；memberDirs 为全部成员所在目录
// 键（稳定格式判定输入——段数/包裹/分隔一致才成稳定格式，数字段值可不同；
// 当前候选恒单目录，多目录为一致性兜底）。格式不一致返回中文原因（剧本步5
// 失败态文案来源）。
func newWorkSetDirSegPlan(dirRelKey string, memberDirs []string) (*workSetDirSegPlan, string) {
	levels := ExtractDirLevels(dirRelKey + "/")
	if dirRelKey == "" || len(levels) == 0 {
		return nil, "无法从目录名推导稳定格式：成员位于库根，无所在目录名"
	}
	name := levels[len(levels)-1]
	els := splitFilenameElements(name, nil)
	plan := &workSetDirSegPlan{
		dirRelKey: dirRelKey, level: len(levels), name: name,
		els: els, digit: make(map[int]bool),
	}
	for i := range els {
		if els[i].Kind != elKindSep {
			plan.ordinals = append(plan.ordinals, i)
			plan.digit[i] = allDigits(elValue(els[i]))
		}
	}
	// 稳定格式判定（多目录）：元素数、分隔 run 原文、括号包裹对逐位一致；
	// 数字类对全部目录该位取交。
	for _, dir := range memberDirs {
		if dir == dirRelKey {
			continue
		}
		other := ExtractDirLevels(dir + "/")
		if len(other) == 0 {
			return nil, "无法从目录名推导稳定格式：成员目录格式不一致（有成员位于库根）"
		}
		oels := splitFilenameElements(other[len(other)-1], nil)
		if len(oels) != len(els) {
			return nil, fmt.Sprintf("无法从目录名推导稳定格式：成员所在目录名格式不一致（段数不同：「%s」与「%s」）", name, other[len(other)-1])
		}
		for i := range els {
			if els[i].Kind == elKindSep {
				if oels[i].Text != els[i].Text {
					return nil, fmt.Sprintf("无法从目录名推导稳定格式：成员所在目录名格式不一致（分隔不同：「%s」与「%s」）", name, other[len(other)-1])
				}
				continue
			}
			if els[i].Shelled != oels[i].Shelled {
				return nil, fmt.Sprintf("无法从目录名推导稳定格式：成员所在目录名格式不一致（包裹不同：「%s」与「%s」）", name, other[len(other)-1])
			}
			if els[i].Shelled {
				a, aok := seqShellPairOf(els[i])
				b, bok := seqShellPairOf(oels[i])
				if !aok || !bok || a != b {
					return nil, fmt.Sprintf("无法从目录名推导稳定格式：成员所在目录名格式不一致（包裹不同：「%s」与「%s」）", name, other[len(other)-1])
				}
			}
			if !allDigits(elValue(oels[i])) {
				plan.digit[i] = false
			}
		}
	}
	return plan, ""
}

// ordinalOf 元素下标 → 位序（1 起；分隔元素返回 0）。
func (p *workSetDirSegPlan) ordinalOf(elIdx int) int {
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

// elIndexOfRef setDirSegN 引用 → 元素下标（非法/越界返回 -1）。
func (p *workSetDirSegPlan) elIndexOfRef(ref string) int {
	n, ok := setDirSegOrdinalOf(ref)
	if !ok || n < 1 || n > len(p.ordinals) {
		return -1
	}
	return p.ordinals[n-1]
}

// digitSegRefs 数字类分段引用（排除指名段；数字段默认标可变——呈现意图，
// 构造上数字类恒为 `\d+` 通配）。
func (p *workSetDirSegPlan) digitSegRefs(excludeEl int) []string {
	var refs []string
	for _, idx := range p.ordinals {
		if idx == excludeEl || !p.digit[idx] {
			continue
		}
		refs = append(refs, setDirSegRefOf(p.ordinalOf(idx)))
	}
	return refs
}

// dirSegPlainClass 目录名普通段（非括号包裹）的泛化段类：排除括号开/闭与
// 元素口径分隔字符（`_` `.` `·` `．`——空格/连字符经归一折叠为 `_` 分隔位），
// 同格式目录的该位不得含括号或分隔（包裹/分隔一致的结构性约束）。
var dirSegPlainClass = "[^" + structOpenChars + structCloseChars + `_\.·．` + "]+"

// dirSegClassEl 单段的内名类（不含包裹括号字面）：数字类→`\d+` 通配（值可
// 不同）；括号段→排除闭括号的泛化段（内名不含闭括号——括号组按最外层计的
// 既成口径）；普通段→dirSegPlainClass。
func dirSegClassEl(el SnapEl, digit bool) string {
	if digit {
		return `\d+`
	}
	if el.Shelled {
		if p, ok := seqShellPairOf(el); ok {
			return "[^" + escapeForClass([]rune(p.close)[0]) + "]*"
		}
	}
	return dirSegPlainClass
}

// dirSegShapePhrase 目录格式形态短语（剧本D 步1 摘要头）：「第 N 层目录呈
// `[任意] [数字] [任意]` 格式」——括号段带包裹呈现、普通段裸呈现，以分隔
// run 原文连接。
func dirSegShapePhrase(plan *workSetDirSegPlan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "第 %d 层目录呈 `", plan.level)
	for i := range plan.els {
		el := plan.els[i]
		if el.Kind == elKindSep {
			b.WriteString(el.Text)
			continue
		}
		token := "任意"
		if plan.digit[i] {
			token = "数字"
		}
		if el.Shelled {
			token = "[" + token + "]"
		}
		b.WriteString(token)
	}
	b.WriteString("` 格式")
	return b.String()
}

// workSetRuleClues 组装调整面线索清单（剧本步4 形态）：目录各层（含剥壳
// 变体——DirStructCluesOf 同源）+ 集所在目录名分段（剧本D 新增：setDirSegN，
// 括号段+分隔符感知；数字段默认标可变、标题段默认指集名捕获由默认指派承载）
// + 成员文件名各段（代表成员元素、序号段单列类别）+ 库根名。返回线索、文件
// 名段位序表与目录名分段位序表（目录名无段/库根时后者为仅含计划的空表——
// 分段引用不可解析）。
func workSetRuleClues(rootPath string, ws *workSetContext) ([]SetRuleClue, *workSetSegPlan, *workSetDirSegPlan) {
	var clues []SetRuleClue
	for _, c := range DirStructCluesOf(rootPath, ws.cand.Dir) {
		clues = append(clues, SetRuleClue{Ref: c.Ref, Kind: c.Kind, Label: c.Label, Value: c.Value})
	}
	dirPlan, _ := newWorkSetDirSegPlan(ws.cand.Dir, nil)
	if dirPlan != nil {
		for i := range dirPlan.els {
			el := dirPlan.els[i]
			if el.Kind == elKindSep {
				continue
			}
			ord := dirPlan.ordinalOf(i)
			clues = append(clues, SetRuleClue{
				Ref: setDirSegRefOf(ord), Kind: setClueKindDirSeg,
				Label: fmt.Sprintf("%s段 %d", dirLevelLabel(dirPlan.level), ord),
				Value: elValue(el), Ordinal: ord,
				Shelled: el.Shelled, Inner: el.Inner,
			})
		}
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
	return clues, plan, dirPlan
}

// setRuleDimAssign 单条「维度←线索」指派。
type setRuleDimAssign struct {
	dim   MetadataDimension
	clue  SetRuleClue
	group string // 文件名段/目录名分段指派的捕获组名（目录层/库根名指派为空）
	// dirEl 目录名分段指派的元素下标（-1=非目录段指派——捕获组折入目录
	// 格式规则的 LocationPattern ancestor）。
	dirEl int
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
	clues, plan, dirPlan := workSetRuleClues(sess.RootPath, ws)
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

	// 维度←线索指派解析（文件名段线索折入文件名规则捕获组；目录名分段折入
	// 目录格式规则捕获组；目录层/库根名线索单独条目）。标签逐条独立成组
	//（tag1、tag2…——structGroupName 同形）。
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
		assign := setRuleDimAssign{dim: dim, clue: c, dirEl: -1}
		switch c.Kind {
		case setClueKindSeg:
			assign.group = structGroupName(dim, tagIndex)
		case setClueKindDirSeg:
			// 目录名分段仅支持作者/集名（剧本步4：作者/集名组合/可变/不用）。
			if dim != DimAuthor {
				return fmt.Errorf("目录名分段指派仅支持作者与集名组合，不能指派%s", qnaDimensionLabel(dim))
			}
			if dirPlan == nil {
				return fmt.Errorf("集所在目录名无分段可指派，线索 %q 无法使用", ref)
			}
			assign.dirEl = dirPlan.elIndexOfRef(ref)
			if assign.dirEl < 0 {
				return fmt.Errorf("指派线索 %q 无法定位到集所在目录名的分段", ref)
			}
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

	rules, err := buildWorkSetAdjustedRules(ws, sess.Snapshot, plan, dirPlan, anchor, dims, in)
	if err != nil {
		return nil, err
	}
	return m.commitWorkSetRules(root, ws, sess.Snapshot, rules)
}

// buildWorkSetAdjustedRules 指派结果 → 规则集组装（纯构造，不落库）：
//   - sibling 定群条目（MemberOnly 且代表成员属序列群时——「仅序列成员产出」
//     条件的取值载体；复用 buildSequenceRule；目录格式语义天然每目录各自成
//     集，不勾仅序列成员即无本条目）；
//   - 目录层维度条目（维度←dirLevel/dirLevelStripped——structDirPlan 既有
//     构造器逐层独立成条）；
//   - 库根名维度常量（维度←rootName——独立条目，锚=至少一层目录）；
//   - 文件名规则（集名捕获段组合 + 维度捕获组 + 序号段数字类消费 + 其余
//     段泛化消费；目录锚定时模式=And[目录层锚, 文件名正则]；文件名侧无任何
//     指派时整条缺席）；
//   - 目录格式规则（剧本D：集名捕获/作者←目录名分段——LocationPattern
//     层深+段格式正则带命名捕获，提取 FromCapture 走内核 ancestor 捕获绑入
//     通路；数字段恒 `\d+` 通配、未捕获段泛化消费；目录名分段无捕获指派时
//     整条缺席）。
//
// 全员自检（集为主体）：每成员上文件名规则捕获值须复现集名（有组合时）
// 与各文件名段维度的对齐取值；目录格式规则每成员命中且集名捕获复现该成员
// 所在目录的段值（各目录各自成集——不要求跨目录同值）。
func buildWorkSetAdjustedRules(ws *workSetContext, snap *RecognitionSnapshot, plan *workSetSegPlan, dirPlan *workSetDirSegPlan, anchor *SetRuleClue, dims []setRuleDimAssign, in setRuleAdjustInput) ([]RuleEntry, error) {
	cand := ws.cand
	rep := &cand.Members[ws.repIdx]
	repFile := snapshotFileOf(snap, rep.RelPath)
	if repFile == nil {
		return nil, fmt.Errorf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", rep.RelPath)
	}

	// 集名捕获段组合：文件名段（引用 → 元素下标，排序去重；须连续非分隔段）
	// 或目录名分段（单段；两者不可混指）。
	spanSet := make(map[int]bool)
	var spanIdx []int
	var dirSpanRef string
	for _, ref := range in.CollectionSegRefs {
		if strings.HasPrefix(ref, setClueDirSegPrefix) {
			if len(spanIdx) > 0 {
				return nil, errors.New("集名捕获段组合须同为文件名段或同为目录名分段")
			}
			if dirSpanRef != "" {
				return nil, errors.New("目录名分段的集名捕获须为单段（多段组合暂不支持）")
			}
			dirSpanRef = ref
			continue
		}
		if dirSpanRef != "" {
			return nil, errors.New("集名捕获段组合须同为文件名段或同为目录名分段")
		}
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

	// ④ 文件名规则（集名捕获 + 维度捕获组 + 序号/其余段消费；文件名侧无
	// 任何指派时整条缺席——纯目录段/目录层指派不再强制空文件名条目）。
	var fileRule *RuleEntry
	if len(spanIdx) > 0 || len(captureByEl) > 0 {
		rule, err := buildWorkSetFilenameRule(ws, rep, repFile, plan, spanIdx, captureByEl, anchor, in)
		if err != nil {
			return nil, err
		}
		fileRule = &rule
		rules = append(rules, rule)
	}

	// ⑤ 目录格式规则（集名捕获/作者←目录名分段；无捕获指派时整条缺席——
	// 可变目录段为呈现意图，构造上未捕获段同泛化消费）。
	dirSpanEl := -1
	if dirSpanRef != "" {
		if dirPlan == nil {
			return nil, fmt.Errorf("集所在目录名无分段可指派，线索 %q 无法使用", dirSpanRef)
		}
		dirSpanEl = dirPlan.elIndexOfRef(dirSpanRef)
		if dirSpanEl < 0 {
			return nil, fmt.Errorf("集名捕获段 %q 不是本集所在目录名的分段线索", dirSpanRef)
		}
	}
	dirAuthorEl := -1
	for _, d := range dims {
		if d.dirEl >= 0 {
			dirAuthorEl = d.dirEl
		}
	}
	var dirRule *RuleEntry
	if dirSpanEl >= 0 || dirAuthorEl >= 0 {
		rule, err := buildWorkSetDirRule(ws, dirPlan, dirSpanEl, dirAuthorEl, in)
		if err != nil {
			return nil, err
		}
		dirRule = &rule
		rules = append(rules, rule)
	}

	// 提取总量护栏：任何指派都未产生提取时如实回错（与旧版空文件名规则
	// 同文案——纯锚定/纯可变段指派不构成规则）。
	extractions := 0
	for i := range rules {
		extractions += len(rules[i].Extractions)
	}
	if extractions == 0 {
		return nil, errors.New("调整指派未产生任何提取（集名捕获与维度指派至少须有其一）")
	}

	// 全员自检（集为主体，非代表成员单样本）：逐成员复现维度取值；集名捕获
	// 以成员间一致为判据——组合可不同于确认集名（调整面正是「改集名取哪」
	// 的入口），全员同值即形态自洽。
	if fileRule != nil {
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
	}
	// 目录格式规则自检：每成员所在目录命中段格式、集名捕获复现该成员所在
	// 目录的段值（各目录各自成集——不要求跨目录同值）、作者捕获复现段值。
	if dirRule != nil {
		for i := range cand.Members {
			mem := &cand.Members[i]
			file := snapshotFileOf(snap, mem.RelPath)
			if file == nil {
				return nil, fmt.Errorf("成员「%s」已不在识别全景内（可能已被重识别移除），请刷新后重试", mem.RelPath)
			}
			if dirRelKeyOf(mem.RelPath) != dirPlan.dirRelKey {
				return nil, fmt.Errorf("成员「%s」不在集目录 %s 内，目录分段指派无法自检", mem.RelPath, dirPlan.dirRelKey)
			}
			ctx := &ParseContext{}
			if !dirRule.Pattern.Evaluate(structSampleInput(mem.RelPath, file.FileName), nil, ctx) {
				return nil, fmt.Errorf("目录分段指派未能在成员「%s」上命中（段格式过窄）", mem.RelPath)
			}
			for _, action := range dirRule.Extractions {
				decl, ok := action.Apply(dirRule.ID, ctx)
				if !ok {
					return nil, fmt.Errorf("目录分段指派未能在成员「%s」上提取%s", mem.RelPath, qnaDimensionLabel(action.Dimension))
				}
				want := ""
				switch action.CaptureName {
				case seqCaptureGroup:
					want = elValue(dirPlan.els[dirSpanEl])
				case structGroupName(DimAuthor, -1):
					want = elValue(dirPlan.els[dirAuthorEl])
				}
				if decl.Value == "" || (want != "" && decl.Value != want) {
					return nil, fmt.Errorf("目录分段指派未能在成员「%s」上复现%s：提取「%s」≠ 期望「%s」",
						mem.RelPath, qnaDimensionLabel(action.Dimension), decl.Value, want)
				}
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

// buildWorkSetDirRule 目录格式规则构造（剧本D）：按集所在目录名的分段形态
// 展开 ancestor 正则——数字类段恒 `\d+` 通配（值不当固定值参与识别）、未捕获
// 段泛化消费（括号段=开/闭字面+排除闭括号的内名类、普通段=排除括号与分隔
// 的段类）、分隔 run 按原文字面；集名捕获段/作者段覆写为命名捕获组（括号
// 字面留在组外，捕获值=剥壳内名——与线索值同口径）。模式=LocationPattern
// （层深锚定集目录层，MinDepth=MaxDepth=层深-1），提取 FromCapture 走内核
// ancestor 捕获绑入通路。每目录各自成集=天然语义（无 sibling/when 原子；
// MemberOnly 勾选时集名提取附「仅序列成员产出」when 原子——用户显式收窄）。
// 自检由调用方的全员校验完成。
func buildWorkSetDirRule(ws *workSetContext, plan *workSetDirSegPlan, collectionEl, authorEl int, in setRuleAdjustInput) (RuleEntry, error) {
	collectionGroup := structGroupName(DimCollection, -1)
	authorGroup := structGroupName(DimAuthor, -1)
	groups := make([]string, 0, 2)
	var expr strings.Builder
	expr.WriteString("^")
	for i := range plan.els {
		el := plan.els[i]
		if el.Kind == elKindSep {
			expr.WriteString(regexp.QuoteMeta(el.Text))
			continue
		}
		group := ""
		switch i {
		case collectionEl:
			group = collectionGroup
		case authorEl:
			group = authorGroup
		}
		class := dirSegClassEl(el, plan.digit[i])
		if el.Shelled {
			if p, ok := seqShellPairOf(el); ok {
				expr.WriteString(regexp.QuoteMeta(p.open))
				if group != "" {
					expr.WriteString("(?P<" + group + ">" + class + ")")
					groups = append(groups, group)
				} else {
					expr.WriteString(class)
				}
				expr.WriteString(regexp.QuoteMeta(p.close))
				continue
			}
		}
		if group != "" {
			expr.WriteString("(?P<" + group + ">" + class + ")")
			groups = append(groups, group)
		} else {
			expr.WriteString(class)
		}
	}
	expr.WriteString("$")
	ancestor, err := NewRegexPattern(string(TargetFileName), expr.String(), groups)
	if err != nil {
		return RuleEntry{}, fmt.Errorf("构造目录名分段正则失败: %w", err)
	}
	location, err := NewLocationPattern(plan.level-1, plan.level-1, ancestor)
	if err != nil {
		return RuleEntry{}, err
	}
	extractions := make([]ExtractionAction, 0, 2)
	if collectionEl >= 0 {
		collection := ExtractionAction{Dimension: DimCollection, From: FromCapture, CaptureName: collectionGroup}
		if in.MemberOnly {
			collection.When = seqMemberOnlyCond()
		}
		extractions = append(extractions, collection)
	}
	if authorEl >= 0 {
		extractions = append(extractions, ExtractionAction{Dimension: DimAuthor, From: FromCapture, CaptureName: authorGroup})
	}
	desc := fmt.Sprintf("泛化（集规则·目录名格式）：第 %d 层目录「%s」段格式→作品集", plan.level, plan.name)
	if collectionEl < 0 {
		desc = fmt.Sprintf("泛化（集规则·目录名格式）：第 %d 层目录「%s」段格式→维度提取", plan.level, plan.name)
	}
	desc += "（每个这样的目录各自成集；源自已归集集「" + ws.setName + "」"
	if authorEl >= 0 {
		desc += fmt.Sprintf("；作者=段 %d 捕获", plan.ordinalOf(authorEl))
	}
	if in.MemberOnly {
		desc += "；仅序列成员产出"
	}
	desc += "）"
	return RuleEntry{
		ID:          generatedRuleID("qna-setdir", plan.name),
		Description: desc,
		Source:      RuleSourceQna,
		Enabled:     true,
		Pattern:     location,
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
