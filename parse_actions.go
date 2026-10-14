// 手动解析三动作：单次赋值（会话态知识写入，不升格规则）、泛化成规则（按
// 文件名形态/目录层/序列形态三维度选择）、冲突裁决生成（双命中 And 组合/单侧
// 缺席改绑的谓词条件策略）。生成物一律为最简充分形式——不生成超出必要的模式
// 复杂度与条件复杂度；每次生成规则后立即触发强制验证轮（重识别+差异集高亮，
// 过窄=差异里仍有未覆盖、过泛=差异里错套）。档案写一律经会话管理器
// UpdateProfile 单点串行，会话赋值写经 mutateSession 同点串行。
package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// 泛化维度词汇（generalize 动作的模式分支）。
const (
	generalizeModeFilename = "filename"  // 按文件名形态：从文件名提取模式（如方括号作者）
	generalizeModeDirLayer = "dirLayer"  // 按目录层：目录含义规则
	generalizeModeSequence = "sequence"  // 按序列形态：定群规则
)

// 裁决方式词汇（adjudicate 动作的模式分支）。
const (
	adjudicateModePick        = "pick"        // 双命中择一：模式=A∧B + 合成=裁决结果
	adjudicateModeConditional = "conditional" // 单侧缺席改绑：谓词条件策略
)

// GeneratedRuleView 生成规则的前端呈现摘要。
type GeneratedRuleView struct {
	// ID / Description / Source 规则身份与来源（qna=泛化、adjudication=裁决）。
	ID          string `json:"id"`
	Description string `json:"description"`
	Source      string `json:"source"`
	// Pattern 模式的人话摘要（如「文件名匹配 ^【…」或「A 与 B」组合形态）。
	Pattern string `json:"pattern"`
	// Condition 为条件动作生成物的条件 JSON 数组（各条件动作的 When 依次序列
	// 化，无压缩单行；无条件动作为空）——手写条件 JSON 片段的进阶参照入口。
	Condition string `json:"condition,omitempty"`
}

// GenerationResult 生成动作的结果：入档的规则摘要与验证轮启动结论。
type GenerationResult struct {
	Rule                 *GeneratedRuleView
	VerificationStarted bool
}

// —— 单次赋值 ——

// ApplyAssignment 单次赋值动作（经 SetAssignment 同点串行）：显式维度覆盖该文件
// 档案求值的同维度终选、跨重识别持续生效；不指定任何维度时等价清除。返回会话
// 与该文件的赋值呈现（清除后为 nil）。
func (m *SessionManager) ApplyAssignment(root, relPath string, assign SessionAssignment) (*WorkbenchSession, *workbenchAssignmentView, error) {
	sess, err := m.SetAssignment(root, relPath, assign)
	if err != nil {
		return nil, nil, err
	}
	relPath = filepath.ToSlash(relPath)
	if current, ok := sess.Assignments[relPath]; ok {
		return sess, &workbenchAssignmentView{
			RelPath: relPath, Author: current.Author, AuthorRole: current.AuthorRole,
			Title: current.Title, Tags: current.Tags, Collection: current.Collection,
			Generalized: current.Generalized, UpdatedAtMs: current.UpdatedAtMs,
		}, nil
	}
	return sess, nil, nil
}

// —— 泛化成规则 ——

// ApplyGeneralization 泛化动作：从单次赋值样例与该文件的识别上下文（快照记录）
// 出发生成对应 PatternKind 的规则（来源 qna）入档案，置赋值的已泛化标记，并立即
// 启动强制验证轮。mode 为空时按文件名形态（缺省维度）。
func (m *SessionManager) ApplyGeneralization(root, relPath, mode string) (*GenerationResult, error) {
	if mode == "" {
		mode = generalizeModeFilename
	}
	relPath = filepath.ToSlash(relPath)

	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	assign, ok := sess.Assignments[relPath]
	if !ok {
		return nil, fmt.Errorf("文件 %s 无会话赋值，请先完成单次赋值再泛化", relPath)
	}
	if len(assign.coveredDimensions()) == 0 {
		return nil, fmt.Errorf("文件 %s 的赋值未指定任何维度，无可泛化内容", relPath)
	}
	if sess.Snapshot == nil {
		return nil, fmt.Errorf("该会话尚未完成全量识别，缺少泛化所需的文件上下文，请先识别")
	}
	fileRec := snapshotFileOf(sess.Snapshot, relPath)
	if fileRec == nil {
		return nil, fmt.Errorf("识别全景中无文件 %s 的记录（磁盘可能与快照不一致），请重新识别", relPath)
	}

	var rule RuleEntry
	var entry ArchiveWriteEntry
	switch mode {
	case generalizeModeFilename:
		rule, entry, err = buildFilenameShapeRule(relPath, fileRec.FileName, &assign)
	case generalizeModeDirLayer:
		rule, entry, err = buildDirLayerRule(relPath, fileRec.DirRelKey, &assign)
	case generalizeModeSequence:
		rule, entry, err = buildSequenceRule(relPath, fileRec.GroupKey)
	default:
		return nil, fmt.Errorf("未知的泛化维度 %q（可用：%s / %s / %s）", mode,
			generalizeModeFilename, generalizeModeDirLayer, generalizeModeSequence)
	}
	if err != nil {
		return nil, err
	}

	if err := m.commitGeneratedRule(entry, &rule); err != nil {
		return nil, err
	}
	if _, err := m.MarkAssignmentGeneralized(root, relPath, true); err != nil {
		m.warnf("置已泛化标记失败（规则已入档）: %v", err)
	}
	started := m.startVerificationRound(root)
	return &GenerationResult{
		Rule:                generatedRuleViewOf(&rule),
		VerificationStarted: started,
	}, nil
}

// buildFilenameShapeRule 按文件名形态泛化：赋值各维度的指定值在文件名（去末段
// 扩展名的正文）中定位，值位升格为命名捕获组、其余文本保持字面，产出锚定全匹配
// 的文件名正则规则——捕获组形态取最简：值位紧邻右分隔符（闭括号类）时用排除该
// 分隔符的非空类，其余用非点起头的非空正文段（与内置惯例同形）。仅泛化值出现
// 在文件名中的维度（未出现在文件名中的维度维持会话赋值承载，不作为常量烧入
// 文件名形态规则——避免把特异性值普适化）。
func buildFilenameShapeRule(relPath, fileName string, assign *SessionAssignment) (RuleEntry, ArchiveWriteEntry, error) {
	base := BaseNameWithoutExt(fileName)

	// 值位收集：标量维度 + 标签逐值（各自独立捕获组）
	type valueSlot struct {
		start, end int
		groupName  string
		dim        MetadataDimension
		roleName   string
	}
	type valueRef struct {
		value     string
		groupName string
		dim       MetadataDimension
		roleName  string
	}
	refs := []valueRef{
		{value: assign.Author, groupName: "author", dim: DimAuthor, roleName: assign.AuthorRole},
		{value: assign.Title, groupName: "title", dim: DimTitle},
		{value: assign.Collection, groupName: "collection", dim: DimCollection},
	}
	for i, tag := range assign.Tags {
		refs = append(refs, valueRef{value: tag, groupName: "tag" + strconv.Itoa(i+1), dim: DimTag})
	}
	var slots []valueSlot
	for _, ref := range refs {
		if ref.value == "" {
			continue
		}
		idx := strings.Index(base, ref.value)
		if idx < 0 {
			continue
		}
		slots = append(slots, valueSlot{start: idx, end: idx + len(ref.value), groupName: ref.groupName, dim: ref.dim, roleName: ref.roleName})
	}
	if len(slots) == 0 {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("赋值各维度的值均未出现在文件名 %q 中，无法按文件名形态泛化（可改按目录层或序列形态）", fileName)
	}
	// 按出现位置排序并剔除重叠值位（保留靠前者）
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].start != slots[j].start {
			return slots[i].start < slots[j].start
		}
		return slots[i].end < slots[j].end
	})
	kept := slots[:0]
	var occupiedEnd int
	for _, slot := range slots {
		if len(kept) > 0 && slot.start < occupiedEnd {
			continue
		}
		kept = append(kept, slot)
		occupiedEnd = slot.end
	}
	slots = kept

	var expr strings.Builder
	expr.WriteString("^")
	cursor := 0
	captures := make([]string, 0, len(slots))
	extractions := make([]ExtractionAction, 0, len(slots))
	for _, slot := range slots {
		expr.WriteString(regexp.QuoteMeta(base[cursor:slot.start]))
		expr.WriteString("(?P<" + slot.groupName + ">" + filenameSlotClass(base, slot.start, slot.end) + ")")
		cursor = slot.end
		captures = append(captures, slot.groupName)
		extractions = append(extractions, ExtractionAction{
			Dimension: slot.dim, From: FromCapture, CaptureName: slot.groupName, RoleName: slot.roleName,
		})
	}
	expr.WriteString(regexp.QuoteMeta(base[cursor:]))
	expr.WriteString(`(?:\.[^.]*)?$`)

	pattern, err := NewRegexPattern(string(TargetFileName), expr.String(), captures)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造文件名形态规则失败: %w", err)
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-file", BaseNameWithoutExt(fileName)),
		Description: fmt.Sprintf("泛化（文件名形态）：由「%s」的赋值生成——%s", relPath, assignmentSummary(assign)),
		Source:      RuleSourceQna,
		Enabled:     true,
		Pattern:     pattern,
		Extractions: extractions,
	}
	return rule, ArchiveWriteGeneralize, nil
}

// filenameSlotClass 值位的捕获组形态（最简充分）：值位紧邻右闭分隔符（】」』
// 》〉）)］]等）时用排除该分隔符的非空类；值位在正文末尾或紧邻普通字符时用
// 非点起头的非空正文段（与内置惯例的标题段同形，防把纯扩展名段误纳值位）。
// start/end 为值位在去扩展名正文中的字节区间 [start, end)。
func filenameSlotClass(base string, start, end int) string {
	if end < len(base) {
		if r, size := utf8.DecodeRuneInString(base[end:]); size > 0 {
			if class, ok := closerExclusionClass(r); ok {
				return class
			}
		}
	}
	return `[^.].*?`
}

// closerExclusionClass 右闭分隔符 → 排除该字符的非空捕获类；非闭分隔符返回假。
func closerExclusionClass(c rune) (string, bool) {
	switch c {
	case ']', '\\', '^':
		return `[\` + string(c) + `]+`, true
	case '】', '」', '』', '》', '〉', '）', ')', '］', '”', '"', '’', '\'':
		return "[^" + string(c) + "]+", true
	}
	return "", false
}

// buildDirLayerRule 按目录层泛化：直接父目录名锚定一条位置规则（MinDepth=
// MaxDepth=父目录的 DirNames 索引）——赋值值与父目录名相同的维度按目录名取值
//（ancestorDir），其余维度作为该目录层的常量声明（目录含义规则）。
func buildDirLayerRule(relPath, dirRelKey string, assign *SessionAssignment) (RuleEntry, ArchiveWriteEntry, error) {
	if dirRelKey == "" {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("文件 %s 位于库根直下，无目录层可泛化（可改按文件名形态）", relPath)
	}
	segments := strings.Split(dirRelKey, "/")
	parent := segments[len(segments)-1]
	depth := len(segments) - 1

	ancestor, err := NewRegexPattern(string(TargetFileName), "^"+regexp.QuoteMeta(parent)+"$", nil)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造目录层规则失败: %w", err)
	}
	location, err := NewLocationPattern(depth, depth, ancestor)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造目录层位置模式失败: %w", err)
	}

	type dimValue struct {
		dim      MetadataDimension
		value    string
		roleName string
	}
	values := []dimValue{
		{dim: DimAuthor, value: assign.Author, roleName: assign.AuthorRole},
		{dim: DimTitle, value: assign.Title},
		{dim: DimCollection, value: assign.Collection},
	}
	for _, tag := range assign.Tags {
		values = append(values, dimValue{dim: DimTag, value: tag})
	}
	extractions := make([]ExtractionAction, 0, len(values))
	for _, v := range values {
		if v.value == "" {
			continue
		}
		if v.value == parent {
			// 值即目录名：按目录名取值（目录层含义的本源形态）
			extractions = append(extractions, ExtractionAction{
				Dimension: v.dim, From: FromAncestorDir, RoleName: v.roleName,
			})
			continue
		}
		extractions = append(extractions, ExtractionAction{
			Dimension: v.dim, From: FromConstant, Constant: v.value, RoleName: v.roleName,
		})
	}
	if len(extractions) == 0 {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("赋值未指定任何维度，无可泛化内容")
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-dir", parent),
		Description: fmt.Sprintf("泛化（目录层）：第 %d 层目录「%s」声明%s（源自「%s」的赋值）", depth+1, parent, assignmentSummary(assign), relPath),
		Source:      RuleSourceQna,
		Enabled:     true,
		Pattern:     location,
		Extractions: extractions,
	}
	return rule, ArchiveWriteGeneralize, nil
}

// buildSequenceRule 按序列形态泛化：该文件所属兄弟群的公共前缀锚定一条定群规则
//（分组键=前缀字面、序号=扩展名前的尾缀数字、≥2 成群、归属作品集）——纯定群，
// 不携带提取动作（顶层兄弟群组规则不参与提取段，元数据由其余规则与赋值承载）。
func buildSequenceRule(relPath, groupKey string) (RuleEntry, ArchiveWriteEntry, error) {
	if groupKey == "" {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("文件 %s 不属于任何兄弟序列群，无法按序列形态泛化", relPath)
	}
	sibling, err := NewSiblingPattern(
		"^("+regexp.QuoteMeta(groupKey)+")",
		`(\d+)(?:\.[^.]*)?$`,
		2, OutcomeCollection,
	)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造序列定群规则失败: %w", err)
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-seq", groupKey),
		Description: fmt.Sprintf("泛化（序列形态）：前缀「%s」+尾缀序号 ≥2 成群归入同一作品集（源自「%s」的赋值）", groupKey, relPath),
		Source:      RuleSourceQna,
		Enabled:     true,
		Pattern:     sibling,
	}
	return rule, ArchiveWriteGeneralize, nil
}

// —— 冲突裁决生成 ——

// adjudicationInput 裁决动作的决策载荷。
type adjudicationInput struct {
	// Mode 裁决方式：pick / conditional。
	Mode string
	// PickIndex pick 模式采纳的候选下标。
	PickIndex int
	// PredicateSource / PredicateDimension conditional 模式的条件谓词
	//（某来源在某维度上的声明是否在场）。
	PredicateSource    SourceKind
	PredicateDimension MetadataDimension
	// PresentIndex / AbsentIndex conditional 模式谓词在场/缺席时采纳的候选下标。
	PresentIndex int
	AbsentIndex  int
}

// ApplyAdjudication 裁决动作：按当前识别快照的待裁决项（relPath+dimension 寻址）
// 呈双方候选规则，用户裁决后生成组合规则（来源 adjudication）入档案并立即启动
// 强制验证轮。双命中择一生成「模式=候选规则模式的 And 组合 + 合成=裁决结果常量」；
// 单侧缺席改绑生成「同一组合模式 + 在场/缺席谓词条件双分支常量」的谓词条件策略。
func (m *SessionManager) ApplyAdjudication(root, relPath string, dim MetadataDimension, in adjudicationInput) (*GenerationResult, error) {
	relPath = filepath.ToSlash(relPath)

	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	if sess.Snapshot == nil {
		return nil, fmt.Errorf("该会话尚未完成全量识别，无待裁决项，请先识别")
	}
	var item *SnapPending
	for i := range sess.Snapshot.Pending {
		p := &sess.Snapshot.Pending[i]
		if p.RelPath == relPath && MetadataDimension(p.Dimension) == dim {
			item = p
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("文件 %s 在维度 %s 上无待裁决项（可能已被处置或规则已变化），请刷新识别全景", relPath, dim)
	}
	if len(item.Candidates) < 2 {
		return nil, fmt.Errorf("待裁决项候选不足双方，无法裁决: %+v", item)
	}

	// 候选规则解析：档案规则 ∪ 内置表内按 RuleID 取模式
	profile, errs := LoadProfile(m.store)
	for _, err := range errs {
		m.warnf("规则档案载入异常（裁决）: %v", err)
	}
	if profile == nil {
		return nil, fmt.Errorf("规则档案不可载入，拒绝裁决生成")
	}
	allRules := allRulesWithBuiltins(profile)
	patternByID := func(ruleID string) (Pattern, bool) {
		for i := range allRules {
			if allRules[i].ID == ruleID {
				return allRules[i].Pattern, true
			}
		}
		return nil, false
	}
	candidateRuleIDs := make([]string, 0, 2)
	seenRule := make(map[string]bool)
	for _, cand := range item.Candidates {
		if cand.RuleID == "" || seenRule[cand.RuleID] {
			continue
		}
		seenRule[cand.RuleID] = true
		candidateRuleIDs = append(candidateRuleIDs, cand.RuleID)
	}
	if len(candidateRuleIDs) < 1 {
		return nil, fmt.Errorf("待裁决候选均无规则出处，无法组合（候选: %+v）", item.Candidates)
	}
	subs := make([]Pattern, 0, len(candidateRuleIDs))
	for _, ruleID := range candidateRuleIDs {
		pattern, ok := patternByID(ruleID)
		if !ok || pattern == nil {
			return nil, fmt.Errorf("候选规则 %q 在档案与内置表中均不存在（可能已被删除），无法组合", ruleID)
		}
		subs = append(subs, pattern)
	}
	// 最简充分形式：单一规则产出的冲突不叠加 And（A∧A≡A）
	combined := combinePatterns(subs)

	candValue := func(index int) (string, error) {
		if index < 0 || index >= len(item.Candidates) {
			return "", fmt.Errorf("候选下标 %d 超出范围（共 %d 个候选）", index, len(item.Candidates))
		}
		return item.Candidates[index].Value, nil
	}

	var extractions []ExtractionAction
	switch in.Mode {
	case adjudicateModePick:
		value, err := candValue(in.PickIndex)
		if err != nil {
			return nil, err
		}
		extractions = []ExtractionAction{{
			Dimension: dim, From: FromAdjudication, Constant: value,
		}}
	case adjudicateModeConditional:
		if !knownSourceKinds[in.PredicateSource] {
			return nil, fmt.Errorf("条件谓词引用了未知的声明来源 %q", in.PredicateSource)
		}
		if !knownDimensions[in.PredicateDimension] {
			return nil, fmt.Errorf("条件谓词引用了未知的含义维度 %q", in.PredicateDimension)
		}
		presentValue, err := candValue(in.PresentIndex)
		if err != nil {
			return nil, err
		}
		absentValue, err := candValue(in.AbsentIndex)
		if err != nil {
			return nil, err
		}
		pred := DeclPredicate{Source: in.PredicateSource, Dimension: in.PredicateDimension}
		// 最简充分形式：两分支各恰一条谓词节点（在场 / 在场取反）
		presentWhen := predicateExpr(pred)
		extractions = []ExtractionAction{
			{Dimension: dim, From: FromAdjudication, Constant: presentValue,
				When: presentWhen},
			{Dimension: dim, From: FromAdjudication, Constant: absentValue,
				When: &CondExpr{Kind: ExprNot, Inner: presentWhen}},
		}
	default:
		return nil, fmt.Errorf("未知的裁决方式 %q（可用：%s / %s）", in.Mode, adjudicateModePick, adjudicateModeConditional)
	}

	rule := RuleEntry{
		ID:          generatedRuleID("adj", BaseNameWithoutExt(item.FileName)),
		Description: adjudicationDescription(item, dim, in, extractions),
		Source:      RuleSourceAdjudication,
		Enabled:     true,
		Pattern:     combined,
		Extractions: extractions,
	}
	if err := m.commitGeneratedRule(ArchiveWriteAdjudicate, &rule); err != nil {
		return nil, err
	}
	started := m.startVerificationRound(root)
	return &GenerationResult{
		Rule:                generatedRuleViewOf(&rule),
		VerificationStarted: started,
	}, nil
}

// startVerificationRound 生成规则后的强制验证轮入口：立即启动重识别；若同库根
// 已有识别在途（其装载的可能是旧档案），后台等其在途结束后补启动一轮，保证
// 新规则的效果与差异集必然入快照（不阻塞生成动作的应答）。返回验证轮是否已
// 启动或已排定。
func (m *SessionManager) startVerificationRound(root string) bool {
	started, running, err := m.StartRecognition(root)
	if err != nil {
		m.warnf("验证轮启动失败（规则已入档，可手动重新识别）: %v", err)
		return false
	}
	if !running {
		return started
	}
	go func() {
		deadline := time.Now().Add(verificationFollowupTimeout)
		for time.Now().Before(deadline) {
			m.recogMu.Lock()
			idle := !m.recogRunning[root]
			m.recogMu.Unlock()
			if idle {
				if _, _, err := m.StartRecognition(root); err != nil {
					m.warnf("验证轮补启动失败: %v", err)
				}
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		m.warnf("验证轮补启动等待在途识别超时（root=%s）", root)
	}()
	return true
}

// verificationFollowupTimeout 在途识别等待补启动的上限。
const verificationFollowupTimeout = 30 * time.Second

// combinePatterns 候选规则模式的最简组合：单一模式原样返回，多个模式 And 组合
//（全部命中才命中——双命中冲突的锚定形态）。
func combinePatterns(subs []Pattern) Pattern {
	if len(subs) == 1 {
		return subs[0]
	}
	return And(subs)
}

// adjudicationDescription 裁决生成规则的人话描述：从裁决内容派生。
func adjudicationDescription(item *SnapPending, dim MetadataDimension, in adjudicationInput, extractions []ExtractionAction) string {
	dimLabel := qnaDimensionLabel(dim)
	rules := make([]string, 0, 2)
	seen := make(map[string]bool)
	for _, cand := range item.Candidates {
		if cand.RuleID == "" || seen[cand.RuleID] {
			continue
		}
		seen[cand.RuleID] = true
		rules = append(rules, cand.RuleID)
	}
	combined := strings.Join(rules, " 与 ")
	if in.Mode == adjudicateModePick {
		return fmt.Sprintf("裁决生成：「%s」的%s冲突（%s）裁决为 %s=%s", item.RelPath, dimLabel, combined, dimLabel, extractions[0].Constant)
	}
	return fmt.Sprintf("裁决生成：「%s」的%s冲突（%s）按条件改绑——%s的%s在场时 %s=%s、缺席时 %s=%s",
		item.RelPath, dimLabel, combined,
		sourceKindLabel(in.PredicateSource), qnaDimensionLabel(in.PredicateDimension),
		dimLabel, extractions[0].Constant, dimLabel, extractions[1].Constant)
}

// —— 生成物公共件 ——

// commitGeneratedRule 生成规则经档案写单点串行入口落库（ID 去重后在位追加）。
func (m *SessionManager) commitGeneratedRule(entry ArchiveWriteEntry, rule *RuleEntry) error {
	return m.UpdateProfile(entry, func(p *RuleProfile) error {
		rule.ID = ensureRuleIDUnique(p, rule.ID)
		p.Rules = append(p.Rules, *rule)
		return nil
	})
}

// ensureRuleIDUnique 生成规则 ID 去重：与档案既有 ID 冲突时追加序号后缀。
func ensureRuleIDUnique(p *RuleProfile, id string) string {
	exists := func(candidate string) bool {
		for _, rule := range p.Rules {
			if rule.ID == candidate {
				return true
			}
		}
		return false
	}
	if !exists(id) {
		return id
	}
	for i := 2; ; i++ {
		alt := fmt.Sprintf("%s-%d", id, i)
		if !exists(alt) {
			return alt
		}
	}
}

// generatedRuleID 生成规则 ID：来源前缀 + 样例 slug + 时间戳去重
//（如 qna-file-<slug>-1718…、adj-<slug>-1718…）。
func generatedRuleID(prefix, sample string) string {
	return prefix + "-" + ruleSlug(sample) + "-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
}

// ruleSlug 样例文本 → ID 片段：保留字母/数字/汉字，其余折叠为连字符，限长 32。
func ruleSlug(sample string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range sample {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(builder.String(), "-")
	if runes := []rune(out); len(runes) > 32 {
		out = string(runes[:32])
	}
	if out == "" {
		out = "rule"
	}
	return out
}

// assignmentSummary 赋值的人话摘要（描述用）：作者=甲、作品名=乙…。
func assignmentSummary(assign *SessionAssignment) string {
	parts := make([]string, 0, 4)
	if assign.Author != "" {
		parts = append(parts, "作者="+assign.Author)
	}
	if assign.Title != "" {
		parts = append(parts, "作品名="+assign.Title)
	}
	if len(assign.Tags) > 0 {
		parts = append(parts, "标签="+strings.Join(assign.Tags, "、"))
	}
	if assign.Collection != "" {
		parts = append(parts, "作品集="+assign.Collection)
	}
	return strings.Join(parts, "、")
}

// sourceKindLabel 来源载体的人话标签。
func sourceKindLabel(source SourceKind) string {
	switch source {
	case SourceDir:
		return "目录源"
	case SourceFilename:
		return "文件名源"
	case SourceConstant:
		return "常量"
	case SourceGroup:
		return "群组前缀源"
	case SourcePredicate:
		return "库内命中源"
	case SourceAssignment:
		return "手工赋值"
	case SourceAdjudication:
		return "裁决"
	default:
		return string(source)
	}
}

// snapshotFileOf 快照内按相对路径取文件记录。
func snapshotFileOf(snap *RecognitionSnapshot, relPath string) *SnapFile {
	for i := range snap.Files {
		if snap.Files[i].RelPath == relPath {
			return &snap.Files[i]
		}
	}
	return nil
}

// generatedRuleViewOf 规则 → 前端呈现摘要（条件动作的 When 逐条序列化为
// JSON 后拼为数组形态透出）。
func generatedRuleViewOf(rule *RuleEntry) *GeneratedRuleView {
	view := &GeneratedRuleView{
		ID:          rule.ID,
		Description: rule.Description,
		Source:      string(rule.Source),
		Pattern:     patternSummary(rule.Pattern),
	}
	conds := make([]string, 0, 2)
	for i := range rule.Extractions {
		if rule.Extractions[i].When == nil {
			continue
		}
		data, err := json.Marshal(rule.Extractions[i].When)
		if err != nil {
			continue
		}
		conds = append(conds, string(data))
	}
	if len(conds) > 0 {
		view.Condition = "[" + strings.Join(conds, ", ") + "]"
	}
	return view
}

// patternSummary 模式的人话摘要（生成结果呈现用）。
func patternSummary(p Pattern) string {
	switch t := p.(type) {
	case nil:
		return ""
	case *RegexPattern:
		target := "文件名"
		switch t.Target {
		case TargetDirName:
			target = "目录名"
		case TargetRelPath:
			target = "相对路径"
		}
		return fmt.Sprintf("%s匹配 %s", target, t.Expr)
	case *LocationPattern:
		return fmt.Sprintf("第 %d 层目录（%s）", t.MinDepth+1, patternSummary(t.Ancestor))
	case *SiblingPattern:
		key := t.KeyExpr
		if key == "" {
			key = "去扩展名文件名"
		}
		return fmt.Sprintf("兄弟群组：分组键 %s + 序号 %s", key, t.SeqExpr)
	case *PredicatePattern:
		kind := "作者"
		if t.Kind == PredicateTag {
			kind = "标签"
		}
		return "库内" + kind + "名命中"
	case And:
		return joinPatternSummaries(t, " 与 ")
	case *And:
		return joinPatternSummaries(*t, " 与 ")
	case Or:
		return joinPatternSummaries(t, " 或 ")
	case *Or:
		return joinPatternSummaries(*t, " 或 ")
	case Not:
		return "非（" + patternSummary(t.Inner) + "）"
	case *Not:
		return "非（" + patternSummary(t.Inner) + "）"
	default:
		return fmt.Sprintf("%T", p)
	}
}

// joinPatternSummaries 子模式摘要的连接。
func joinPatternSummaries(subs []Pattern, sep string) string {
	summaries := make([]string, 0, len(subs))
	for _, sub := range subs {
		summaries = append(summaries, patternSummary(sub))
	}
	return strings.Join(summaries, sep)
}
