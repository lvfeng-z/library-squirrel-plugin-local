// 手动解析动作族：单次赋值与目录赋值（会话态知识写入，不升格规则）、泛化成
// 规则（按文件名形态/目录层/序列形态/结构映射四维度选择；入档后赋值保留
// ——与新规则双源互证，2026-10-25 用户裁决废除「泛化即接管」消费剥离）、
// 冲突裁决生成（双命中 And 组合/单侧缺席
// 改绑的谓词条件策略，生成物不消费赋值）。生成物一律为最简充分形式——不生成
// 超出必要的模式复杂度与条件复杂度；每次生成规则或赋值变更后经识别刷新调度
// 自动排定一轮重识别（差异集高亮，过窄=差异里仍有未覆盖、过泛=差异里错套）。
// 档案写一律经会话管理器 UpdateProfile 单点串行，会话赋值写经 mutateSession
// 同点串行。
package main

import (
	"encoding/json"
	"errors"
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
	generalizeModeFilename = "filename" // 按文件名形态：从文件名提取模式（如方括号作者）
	generalizeModeDirLayer = "dirLayer" // 按目录层：目录含义规则
	generalizeModeSequence = "sequence" // 按序列形态：定群规则
	generalizeModeStruct   = "struct"   // 按结构映射：赋值线索出处→组合规则（规则总结器的确认入档动作）
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

// GenerationResult 生成动作的结果：入档的规则摘要与验证轮启动结论。Rules 为
// 组合规则（结构映射维度）的全部入档条目；单规则维度仅填 Rule。
type GenerationResult struct {
	Rule                *GeneratedRuleView
	Rules               []*GeneratedRuleView
	VerificationStarted bool
}

// —— 单次赋值 ——

// ApplyAssignment 单次赋值动作（经 SetAssignment 同点串行）：显式维度覆盖该文件
// 档案求值的同维度终选、跨重识别持续生效；不指定任何维度时等价清除。成功后经
// 识别刷新调度自动排定一轮重识别（同库根在途则等其结束后补启动），快照最终
// 反映变更、无需前端再发识别请求。返回会话与该文件的赋值呈现（清除后为 nil）。
func (m *SessionManager) ApplyAssignment(root, relPath string, assign SessionAssignment) (*WorkbenchSession, *workbenchAssignmentView, error) {
	sess, err := m.SetAssignment(root, relPath, assign)
	if err != nil {
		return nil, nil, err
	}
	m.scheduleRecognitionRefresh(root)
	relPath = filepath.ToSlash(relPath)
	if current, ok := sess.Assignments[relPath]; ok {
		return sess, &workbenchAssignmentView{
			RelPath: relPath, Author: current.Author, AuthorRole: current.AuthorRole,
			Title: current.Title, Tags: current.Tags, Collection: current.Collection,
			AuthorClueRef: current.AuthorClueRef, TitleClueRef: current.TitleClueRef,
			CollectionClueRef: current.CollectionClueRef, TagClueRefs: current.TagClueRefs,
			Generalized: current.Generalized, UpdatedAtMs: current.UpdatedAtMs,
		}, nil
	}
	return sess, nil, nil
}

// —— 目录赋值（子树展开合并写）——

// dirSubtreeFilesOf 快照内展开目录子树的文件集（快照序）：DirRelKey==dirRelKey
// 或以 dirRelKey+"/" 为前缀（dirRelKey 空串=库根全树）。第二返回值=目录键是否
// 在当前快照的目录树内（含空目录；不在=前端全景已过期）。
func dirSubtreeFilesOf(snap *RecognitionSnapshot, dirRelKey string) ([]*SnapFile, bool) {
	dirRelKey = strings.Trim(filepath.ToSlash(dirRelKey), "/")
	inTree := false
	for i := range snap.Dirs {
		if snap.Dirs[i].RelKey == dirRelKey {
			inTree = true
			break
		}
	}
	if !inTree {
		return nil, false
	}
	var files []*SnapFile
	if dirRelKey == "" {
		for i := range snap.Files {
			files = append(files, &snap.Files[i])
		}
		return files, true
	}
	prefix := dirRelKey + "/"
	for i := range snap.Files {
		f := &snap.Files[i]
		if f.DirRelKey == dirRelKey || strings.HasPrefix(f.DirRelKey, prefix) {
			files = append(files, f)
		}
	}
	return files, true
}

// dirScopedFiles 目录寻址的作用域解析（目录赋值外的动作共用）：返回子树文件集
// （快照序）与样本文件（子树内首个有赋值的文件）。目录键不在快照、子树无文件、
// 子树无赋值文件分别回可读错误；actionNoun 为动作名（泛化/总结规则）。
func dirScopedFiles(sess *WorkbenchSession, dirRelKey, actionNoun string) ([]*SnapFile, string, error) {
	if sess.Snapshot == nil {
		return nil, "", fmt.Errorf("该会话尚未完成全量识别，缺少%s所需的文件上下文，请先识别", actionNoun)
	}
	files, ok := dirSubtreeFilesOf(sess.Snapshot, dirRelKey)
	if !ok {
		return nil, "", errors.New("目录不在当前识别全景内，请重新识别")
	}
	if len(files) == 0 {
		return nil, "", errors.New("该目录下没有文件")
	}
	for _, f := range files {
		if _, has := sess.Assignments[f.RelPath]; has {
			return files, f.RelPath, nil
		}
	}
	return nil, "", fmt.Errorf("目录 %s 子树内没有已赋值文件，请先完成赋值再%s", dirRelKey, actionNoun)
}

// mergeDirAssignment 目录赋值与文件既有赋值的逐维度合并：非空维度覆盖该文件
// 赋值的同维度（值与出处引用整组替换，作者维含 roleName），空维度不动——目录
// 面板只填部分维度时不清空子树文件的其余维度。
func mergeDirAssignment(current, overlay SessionAssignment) SessionAssignment {
	if overlay.Author != "" {
		current.Author = overlay.Author
		current.AuthorRole = overlay.AuthorRole
		current.AuthorClueRef = overlay.AuthorClueRef
	}
	if overlay.Title != "" {
		current.Title = overlay.Title
		current.TitleClueRef = overlay.TitleClueRef
	}
	if overlay.Collection != "" {
		current.Collection = overlay.Collection
		current.CollectionClueRef = overlay.CollectionClueRef
	}
	if len(overlay.Tags) > 0 {
		current.Tags = overlay.Tags
		current.TagClueRefs = overlay.TagClueRefs
	}
	return current
}

// ApplyDirAssignment 目录赋值动作：以当前识别快照展开 dirRelKey 子树（空串=
// 库根全树），逐文件合并写 SessionAssignment（非空维度覆盖、空维度不动；目录级
// 出处引用对子树所有文件有效），单锁单次落库后经识别刷新调度排定一轮重识别。
// 返回会话与生效文件数（子树文件数）。
func (m *SessionManager) ApplyDirAssignment(root, dirRelKey string, overlay SessionAssignment) (*WorkbenchSession, int, error) {
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, 0, err
	}
	if sess.Snapshot == nil {
		return nil, 0, errors.New("该会话尚未完成全量识别，无目录可寻址，请先识别")
	}
	files, ok := dirSubtreeFilesOf(sess.Snapshot, dirRelKey)
	if !ok {
		return nil, 0, errors.New("目录不在当前识别全景内，请重新识别")
	}
	if len(files) == 0 {
		return nil, 0, errors.New("该目录下没有文件")
	}
	if len(overlay.coveredDimensions()) == 0 {
		return nil, 0, errors.New("目录赋值未指定任何维度，无法应用")
	}
	now := time.Now().UnixMilli()
	sess, err = m.mutateSession(root, func(sess *WorkbenchSession) error {
		if sess.Assignments == nil {
			sess.Assignments = make(map[string]SessionAssignment)
		}
		for _, f := range files {
			merged := mergeDirAssignment(sess.Assignments[f.RelPath], overlay)
			merged.UpdatedAtMs = now
			sess.Assignments[f.RelPath] = merged
		}
		// 批量合并写整体视为一次赋值变更：修订号递增驱动增量闸门失效
		sess.AssignRevision++
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	m.scheduleRecognitionRefresh(root)
	return sess, len(files), nil
}

// ClearDirAssignments 清除目录动作：删除 dirRelKey 子树内全部文件的赋值（含
// 已泛化标记），排自动增量重识别。返回会话与清除数。
func (m *SessionManager) ClearDirAssignments(root, dirRelKey string) (*WorkbenchSession, int, error) {
	sess, err := m.loadSession(root)
	if err != nil {
		return nil, 0, err
	}
	if sess.Snapshot == nil {
		return nil, 0, errors.New("该会话尚未完成全量识别，无目录可寻址，请先识别")
	}
	files, ok := dirSubtreeFilesOf(sess.Snapshot, dirRelKey)
	if !ok {
		return nil, 0, errors.New("目录不在当前识别全景内，请重新识别")
	}
	if len(files) == 0 {
		return nil, 0, errors.New("该目录下没有文件")
	}
	cleared := 0
	sess, err = m.mutateSession(root, func(sess *WorkbenchSession) error {
		for _, f := range files {
			if _, existed := sess.Assignments[f.RelPath]; !existed {
				continue
			}
			delete(sess.Assignments, f.RelPath)
			cleared++
		}
		if cleared > 0 {
			sess.AssignRevision++
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	m.scheduleRecognitionRefresh(root)
	return sess, cleared, nil
}

// —— 泛化成规则（入档后赋值保留：双源互证）——

// ApplyGeneralization 泛化动作（单文件寻址）：从单次赋值样例与该文件的识别上下文
// （快照记录）出发生成对应 PatternKind 的规则（来源 qna）入档案，作用域内赋值
// 全量保留（用户裁决 2026-10-25：废除「泛化即接管」消费剥离——赋值显式维度
// 覆盖规则求值终选的合并语义本就存在，双源共存互为印证；Generalized 标记仅作
// 「已参与泛化」记录），随即启动强制验证轮。mode 为空时按文件名形态（缺省维度）。
func (m *SessionManager) ApplyGeneralization(root, relPath, mode string) (*GenerationResult, error) {
	return m.applyGeneralization(root, relPath, "", mode)
}

// ApplyDirGeneralization 泛化动作（目录寻址）：样本文件取子树内首个有赋值的
// 文件，赋值标记作用域=子树全部文件。
func (m *SessionManager) ApplyDirGeneralization(root, dirRelKey, mode string) (*GenerationResult, error) {
	return m.applyGeneralization(root, "", dirRelKey, mode)
}

// applyGeneralization 泛化落地单一实现：作用域解析（单文件=该文件；目录=子树
// 全部文件、样本=首个有赋值文件）→ 规则构造（各模式分支）→ 档案写单点入档 →
// 作用域赋值标记（赋值保留：双源互证，见 markScopedAssignments）→ 验证轮排定。
func (m *SessionManager) applyGeneralization(root, relPath, dirRelKey, mode string) (*GenerationResult, error) {
	if mode == "" {
		mode = generalizeModeFilename
	}
	relPath = filepath.ToSlash(relPath)

	sess, err := m.loadSession(root)
	if err != nil {
		return nil, err
	}
	var scope []*SnapFile
	if relPath == "" {
		scope, relPath, err = dirScopedFiles(sess, dirRelKey, "泛化")
		if err != nil {
			return nil, err
		}
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
	if scope == nil {
		scope = []*SnapFile{fileRec}
	}

	var rules []RuleEntry
	switch mode {
	case generalizeModeFilename:
		rule, _, err := buildFilenameShapeRule(relPath, fileRec.FileName, &assign)
		if err != nil {
			return nil, err
		}
		rules = []RuleEntry{rule}
	case generalizeModeDirLayer:
		rule, _, err := buildDirLayerRule(relPath, fileRec.DirRelKey, &assign)
		if err != nil {
			return nil, err
		}
		rules = []RuleEntry{rule}
	case generalizeModeSequence:
		rule, _, err := buildSequenceRule(relPath, fileRec.FileName, fileRec.GroupKey, snapshotGroupOf(sess.Snapshot, relPath))
		if err != nil {
			return nil, err
		}
		rules = []RuleEntry{rule}
	case generalizeModeStruct:
		// 序列形态路径与总结器共用同一构造（建议即所见即所入）：样本属序列群
		// 且集名对得上群键元素段组合时入档序列形态条目（sibling 形态跟随 +
		// 集名捕获 + 仅序列成员产出 when 原子）；否则走既有结构映射构造。
		if out := trySequenceFormSummary(relPath, fileRec.FileName, &assign, sess.Snapshot); out.matched {
			rules = out.rules
		} else {
			_, rs, err := buildStructMapRules(sess.RootPath, relPath, fileRec.FileName, &assign)
			if err != nil {
				return nil, err
			}
			rules = rs
		}
	default:
		return nil, fmt.Errorf("未知的泛化维度 %q（可用：%s / %s / %s / %s）", mode,
			generalizeModeFilename, generalizeModeDirLayer, generalizeModeSequence, generalizeModeStruct)
	}

	for i := range rules {
		if err := m.commitGeneratedRule(ArchiveWriteGeneralize, &rules[i]); err != nil {
			return nil, err
		}
	}
	if err := m.markScopedAssignments(root, scope); err != nil {
		m.warnf("泛化标记赋值失败（规则已入档）: %v", err)
	}
	started := m.scheduleRecognitionRefresh(root)
	if len(rules) == 1 {
		return &GenerationResult{
			Rule:                generatedRuleViewOf(&rules[0]),
			VerificationStarted: started,
		}, nil
	}
	views := make([]*GeneratedRuleView, 0, len(rules))
	for i := range rules {
		views = append(views, generatedRuleViewOf(&rules[i]))
	}
	return &GenerationResult{Rule: views[0], Rules: views, VerificationStarted: started}, nil
}

// markScopedAssignments 泛化入档后的赋值标记（用户裁决 2026-10-25：废除「泛化
// 即接管」消费剥离）：作用域内赋值**全量保留**——赋值显式维度覆盖规则求值终选
// 的合并语义本就存在，赋值（手动知识）与规则声明双源共存互为印证，手动赋值
// 永不因泛化被自动剥离或删除（用户不再需要时自行清除）；Generalized 标记仅作
// 「该文件赋值已参与过泛化」的谱系记录（识别与泛化观测面用）。
func (m *SessionManager) markScopedAssignments(root string, scope []*SnapFile) error {
	if len(scope) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	_, err := m.mutateSession(root, func(sess *WorkbenchSession) error {
		for _, f := range scope {
			assign, ok := sess.Assignments[f.RelPath]
			if !ok || assign.Generalized {
				continue
			}
			assign.Generalized = true
			assign.UpdatedAtMs = now
			sess.Assignments[f.RelPath] = assign
		}
		return nil
	})
	return err
}

// buildFilenameShapeRule 按文件名形态泛化：赋值各维度的指定值在文件名（去末段
// 扩展名的正文）中定位，值位升格为命名捕获组、其余文本保持字面，产出锚定全匹配
// 的文件名正则规则——捕获组形态取最简：值位紧邻右分隔符（闭括号类）时用排除该
// 分隔符的非空类，其余用非点起头的非空正文段（与内置惯例同形）。仅泛化值出现
// 在文件名中的维度（未出现在文件名中的维度维持会话赋值承载，不作为常量烧入
// 文件名形态规则——避免把特异性值普适化）。
// condxTemplateBraceOK 模板词法守卫：字面段/值含花括号会被占位词词法误读，
// 返回假（调用方报「无法模板化」拒绝生成，宁缺勿错）。
func condxTemplateBraceOK(s string) bool {
	return !strings.ContainsAny(s, "{}")
}

// condxWordOfDim 赋值维度 → v4 占位词（词表封闭：标签仅一位）。
func condxWordOfDim(dim MetadataDimension) (string, bool) {
	switch dim {
	case DimAuthor:
		return "作者", true
	case DimTitle:
		return "标题", true
	case DimCollection:
		return "集名", true
	case DimTag:
		return "标签", true
	}
	return "", false
}

func buildFilenameShapeRule(relPath, fileName string, assign *SessionAssignment) (RuleEntry, ArchiveWriteEntry, error) {
	base := BaseNameWithoutExt(fileName)

	// 值位收集：标量维度 + 标签逐值（各自独立捕获组）
	type valueSlot struct {
		start, end int
		word       string
		dim        MetadataDimension
		roleName   string
	}
	type valueRef struct {
		value    string
		word     string
		dim      MetadataDimension
		roleName string
	}
	if len(assign.Tags) > 1 {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf(
			"赋值含 %d 个标签值，新语法词表仅一位 {标签}，暂不能按文件名形态泛化（可先精简为单标签或改按目录层形态）", len(assign.Tags))
	}
	refs := []valueRef{
		{value: assign.Author, word: "作者", dim: DimAuthor, roleName: assign.AuthorRole},
		{value: assign.Title, word: "标题", dim: DimTitle},
		{value: assign.Collection, word: "集名", dim: DimCollection},
	}
	for _, tag := range assign.Tags {
		refs = append(refs, valueRef{value: tag, word: "标签", dim: DimTag})
	}
	var slots []valueSlot
	for _, ref := range refs {
		if ref.value == "" {
			continue
		}
		if !condxTemplateBraceOK(ref.value) {
			return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf(
				"赋值值 %q 含花括号，与格式模板词法冲突，无法按文件名形态泛化", ref.value)
		}
		idx := strings.Index(base, ref.value)
		if idx < 0 {
			continue
		}
		slots = append(slots, valueSlot{start: idx, end: idx + len(ref.value), word: ref.word, dim: ref.dim, roleName: ref.roleName})
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

	// v4 模板拼接：字面段原样、值位落占位词（行1=文件目标行；捕获段语义由
	// 贪婪回溯承载——右闭分隔符不再需要排除类，字面段锚定即切分）。
	var tpl strings.Builder
	cursor := 0
	extractions := make([]ExtractionAction, 0, len(slots))
	for _, slot := range slots {
		literal := base[cursor:slot.start]
		if !condxTemplateBraceOK(literal) {
			return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf(
				"文件名 %q 的字面段 %q 含花括号，与格式模板词法冲突，无法泛化", fileName, literal)
		}
		tpl.WriteString(literal)
		tpl.WriteString("{" + slot.word + "}")
		cursor = slot.end
		extractions = append(extractions, ExtractionAction{
			Dimension: slot.dim, From: FromCapture, CaptureName: condxVocab[slot.word], Row: 1, RoleName: slot.roleName,
		})
	}
	tail := base[cursor:]
	if !condxTemplateBraceOK(tail) {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf(
			"文件名 %q 的字面段 %q 含花括号，与格式模板词法冲突，无法泛化", fileName, tail)
	}
	tpl.WriteString(tail)

	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl.String()},
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造文件名形态规则失败: %w", err)
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-file", BaseNameWithoutExt(fileName)),
		Description: fmt.Sprintf("泛化（文件名形态）：文件名按「%s」模板，由「%s」的赋值生成——%s", tpl.String(), relPath, assignmentSummary(assign)),
		Source:      RuleSourceQna,
		Enabled:     true,
		Conditions:  rows,
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

// buildDirLayerRule 按目录层泛化：v4 目录条件行（整名字面模板；任意祖先层
// 同名命中=子树包含语义，目录名通常整树唯一，深度漂移可忽略）——赋值值与该
// 目录名相同的维度按目录名取值（ancestorDir，取命中层名），其余维度作为该
// 目录层的常量声明（目录含义规则）。
func buildDirLayerRule(relPath, dirRelKey string, assign *SessionAssignment) (RuleEntry, ArchiveWriteEntry, error) {
	if dirRelKey == "" {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("文件 %s 位于库根直下，无目录层可泛化（可改按文件名形态）", relPath)
	}
	segments := strings.Split(dirRelKey, "/")
	parent := segments[len(segments)-1]
	if !condxTemplateBraceOK(parent) {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf(
			"目录名 %q 含花括号，与格式模板词法冲突，无法按目录层泛化", parent)
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
	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetDir,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: parent},
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造目录层规则失败: %w", err)
	}
	rule := RuleEntry{
		ID:          generatedRuleID("qna-dir", parent),
		Description: fmt.Sprintf("泛化（目录层）：目录「%s」（任意祖先层同名命中）声明%s（源自「%s」的赋值）", parent, assignmentSummary(assign), relPath),
		Source:      RuleSourceQna,
		Enabled:     true,
		Conditions:  rows,
		Pattern:     pattern,
		Extractions: extractions,
	}
	return rule, ArchiveWriteGeneralize, nil
}

// buildSequenceRule 按序列形态泛化：该文件所属兄弟群固化一条 v4 序列条件行
// （成员模板 + ≥2 成群门槛，纯定群不携带提取动作——群组规则不参与提取段，
// 元数据由其余规则与赋值承载）。模板由**样本成员名**推导：{键}盖住键段
// （结构形态群的形态键提取式命中段，或规则定群群的公共前缀字面）、{序号}盖住
// 尾缀数字段（包裹符 [ ] ( ) 原样入字面段——同包裹制的同形态新文件可复现
// 定群；跨包裹制混编的群成员将不再互认，v3 的包裹容忍提取式随语法换代废止）。
func buildSequenceRule(relPath, fileName, groupKey string, group *SnapGroup) (RuleEntry, ArchiveWriteEntry, error) {
	if groupKey == "" {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("文件 %s 不属于任何兄弟序列群，无法按序列形态泛化", relPath)
	}
	base := BaseNameWithoutExt(fileName)
	tpl, formName, err := seqMemberTemplateOf(base, groupKey, group)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造序列定群模板失败: %w", err)
	}
	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetSeq,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
		Attach: []CondxAttach{{Kind: CondxAttachSeqMinLength, MinCount: 2}},
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		return RuleEntry{}, ArchiveWriteGeneralize, fmt.Errorf("构造序列定群规则失败: %w", err)
	}
	desc := fmt.Sprintf("泛化（序列形态）：成员按「%s」模板、同键≥2 成群归入同一作品集（源自「%s」的赋值）", tpl, relPath)
	if formName != "" {
		desc = fmt.Sprintf("泛化（序列形态·%s）：成员按「%s」模板、同键≥2 成群归入同一作品集（源自「%s」的赋值）", formName, tpl, relPath)
	}
	return RuleEntry{
		ID:          generatedRuleID("qna-seq", groupKey),
		Description: desc,
		Source:      RuleSourceQna,
		Enabled:     true,
		Conditions:  rows,
		Pattern:     pattern,
	}, ArchiveWriteGeneralize, nil
}

// seqMemberTemplateOf 由样本成员名推导 v4 序列成员模板：返回 (模板, 形态名, 错误)。
// 结构形态群：键段=形态键提取式在样本上的命中段、序号段=形态序号提取式命中段，
// 段外文本原样入字面段；规则定群群：{键}盖住公共前缀、尾缀数字段（含可选包裹
// 符）入 {序号}。字面段含花括号、两段重叠或提取式不中样本时回退前缀路径；前缀
// 路径再失败（无尾缀数字/花括号冲突）返回错误。
func seqMemberTemplateOf(base, groupKey string, group *SnapGroup) (string, string, error) {
	if group != nil && group.FormName != "" && group.FormKeyExpr != "" && group.FormSeqExpr != "" {
		if tpl, ok, err := seqTemplateFromForm(base, group.FormKeyExpr, group.FormSeqExpr); err != nil {
			return "", "", err
		} else if ok {
			return tpl, group.FormName, nil
		}
	}
	tpl, err := seqTemplateFromPrefix(base, groupKey)
	return tpl, "", err
}

// seqTemplateFromForm 形态路径：提取式定位键/序号段；不中样本或段重叠回退（ok=false）。
func seqTemplateFromForm(base, keyExpr, seqExpr string) (string, bool, error) {
	keyRe, err := regexp.Compile(keyExpr)
	if err != nil {
		return "", false, nil
	}
	seqRe, err := regexp.Compile(seqExpr)
	if err != nil {
		return "", false, nil
	}
	km := keyRe.FindStringSubmatchIndex(base)
	sm := seqRe.FindStringSubmatchIndex(base)
	if km == nil || sm == nil {
		return "", false, nil
	}
	// 提取式捕获组1为段；无捕获组时整匹为段。
	keySpan := submatchSpan(km, len(keyRe.SubexpNames()))
	seqSpan := submatchSpan(sm, len(seqRe.SubexpNames()))
	if keySpan[1] > seqSpan[0] {
		// 键序重叠（或键空段）：回退前缀路径
		return "", false, nil
	}
	tpl, err := condxTemplateFromSpans(base, [][2]int{keySpan, {seqSpan[0], seqSpan[1]}}, []string{"键", "序号"})
	if err != nil {
		return "", false, nil
	}
	return tpl, true, nil
}

// seqTemplateFromPrefix 前缀路径：{键}盖住公共前缀字面（群组键=前缀，集命名
// 沿用），尾缀数字（可含单侧包裹符）为 {序号}。
func seqTemplateFromPrefix(base, groupKey string) (string, error) {
	if !condxTemplateBraceOK(groupKey) {
		return "", fmt.Errorf("群前缀 %q 含花括号，与格式模板词法冲突", groupKey)
	}
	if !strings.HasPrefix(base, groupKey) {
		return "", fmt.Errorf("样本 %q 不以群前缀 %q 开头，无法推导成员模板", base, groupKey)
	}
	rest := base[len(groupKey):]
	m := regexp.MustCompile(`^(.*?)([\[\(]?)([0-9]+)([\]\)]?)$`).FindStringSubmatchIndex(rest)
	if m == nil {
		return "", fmt.Errorf("样本 %q 在群前缀后无尾缀数字段，无法推导成员模板", base)
	}
	span := func(i int) (int, int) { return m[2*i], m[2*i+1] }
	midS, midE := span(1)
	openS, openE := span(2)
	closeS, closeE := span(4)
	var tpl strings.Builder
	tpl.WriteString("{键}")
	tpl.WriteString(rest[midS:midE])
	tpl.WriteString(rest[openS:openE])
	tpl.WriteString("{序号}")
	tpl.WriteString(rest[closeS:closeE])
	return tpl.String(), nil
}

// condxTemplateFromSpans 按给定段切样本为字面段+占位词模板（words 与 spans
// 一一对应，段须按位置升序且互不重叠）。
func condxTemplateFromSpans(base string, spans [][2]int, words []string) (string, error) {
	var tpl strings.Builder
	cursor := 0
	for i, span := range spans {
		literal := base[cursor:span[0]]
		if !condxTemplateBraceOK(literal) {
			return "", fmt.Errorf("字面段 %q 含花括号，与格式模板词法冲突", literal)
		}
		tpl.WriteString(literal)
		tpl.WriteString("{" + words[i] + "}")
		cursor = span[1]
	}
	tail := base[cursor:]
	if !condxTemplateBraceOK(tail) {
		return "", fmt.Errorf("字面段 %q 含花括号，与格式模板词法冲突", tail)
	}
	tpl.WriteString(tail)
	return tpl.String(), nil
}

// submatchSpan 取子匹配组1的区间；无组1时取整匹区间。
func submatchSpan(m []int, groupCount int) [2]int {
	if groupCount > 1 && len(m) >= 4 && m[2] >= 0 {
		return [2]int{m[2], m[3]}
	}
	return [2]int{m[0], m[1]}
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
	started := m.scheduleRecognitionRefresh(root)
	return &GenerationResult{
		Rule:                generatedRuleViewOf(&rule),
		VerificationStarted: started,
	}, nil
}

// scheduleRecognitionRefresh 识别刷新调度（规则生成与赋值变更共用入口）：立即
// 启动一轮重识别；若同库根已有识别在途（其装载的可能是旧档案/旧赋值集），后台
// 等其在途结束后补启动一轮，保证新规则/新赋值的效果与差异集必然入快照（不阻塞
// 调用方的应答；在途识别受修订号闸门约束不会用旧赋值复用结论，补启动轮必然
// 装载最新赋值集）。返回刷新轮是否已启动或已排定。
func (m *SessionManager) scheduleRecognitionRefresh(root string) bool {
	started, running, err := m.StartRecognition(root)
	if err != nil {
		m.warnf("识别刷新启动失败（变更已落盘，可手动重新识别）: %v", err)
		return false
	}
	if !running {
		return started
	}
	go func() {
		deadline := time.Now().Add(recognitionFollowupTimeout)
		for time.Now().Before(deadline) {
			m.recogMu.Lock()
			idle := !m.recogRunning[root]
			m.recogMu.Unlock()
			if idle {
				if _, _, err := m.StartRecognition(root); err != nil {
					m.warnf("识别刷新补启动失败: %v", err)
				}
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		m.warnf("识别刷新补启动等待在途识别超时（root=%s）", root)
	}()
	return true
}

// recognitionFollowupTimeout 在途识别等待补启动的上限。
const recognitionFollowupTimeout = 30 * time.Second

// combinePatterns 候选规则模式的最简组合：单一模式原样返回，多个模式 And 组合
// （全部命中才命中——双命中冲突的锚定形态）。
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
// （如 qna-file-<slug>-1718…、adj-<slug>-1718…）。
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

// snapshotGroupOf 快照内按成员相对路径取所属序列群（结构形态群或规则定群群）；
// 非群组成员返回 nil。
func snapshotGroupOf(snap *RecognitionSnapshot, relPath string) *SnapGroup {
	for i := range snap.Groups {
		for _, mem := range snap.Groups[i].Members {
			if mem.RelPath == relPath {
				return &snap.Groups[i]
			}
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
		switch t.Kind {
		case PredicateTag:
			kind = "标签"
		case PredicateWorkSet:
			kind = "作品集"
		}
		return "库内" + kind + "名命中"
	case *RowSubjectPattern:
		summary := patternSummary(t.Inner)
		if len(t.Gates) > 0 {
			summary += "（值级小标 " + strconv.Itoa(len(t.Gates)) + " 项）"
		}
		return summary
	case *FileSubjectPattern:
		return fmt.Sprintf("文件名按模板 %s", t.Expr)
	case *DirSubjectPattern:
		if len(t.Asserts) > 0 {
			return fmt.Sprintf("目录名按模板 %s（终结谓词 %d 项）", t.Expr, len(t.Asserts))
		}
		return fmt.Sprintf("目录名按模板 %s", t.Expr)
	case *SeqSubjectPattern:
		return fmt.Sprintf("序列成员按模板 %s（同键≥%d）", t.Expr, t.MinSize)
	case *ParentNamePattern:
		return fmt.Sprintf("父目录名匹配 %s", t.Expr)
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
