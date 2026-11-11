// 规则草稿命中预览（previewRule 只读动作）：编辑面的未入档草稿按保存同款
// 构造管线转为规则条目，并入现行档案（同 ID 条目原位替换、新建追加、缺省 ID
// 按保存同款生成去重），对该会话最近一次识别快照的全库文件跑一遍合并求值。
// 只读：不写档案、不改会话状态。
//
// 求值双路（预览面大库性能方案阶段3）：「新增条目且未被任何现行规则引用且
// 自身非组合非定群」的草稿走快路径——基座（识别轮铸造的规则相关件，守卫=
// 快照指针+档案指纹+求值器构建标识）逐文件原始声明作底，草稿单独逐文件求值
// 后并集，再走同一 Synthesize 纯函数合成重放（受影响集={草稿} 时与整库全量
// 逐字节同构）；其余形态（替换/组合/被引用/定群及等值护栏失配）一律回退现行
// 整库全量路径，结果一致。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// previewSampleMax 命中样例的呈现条数上限。
const previewSampleMax = 3

// ErrPreviewRuleNoSnapshot 命中预览的降级错误：库根无活动会话，或会话尚无
// 识别快照（预览的求值面=最近一次识别的文件清单）。文案与前端预览按钮
// 禁用态的说明小字同源。
var ErrPreviewRuleNoSnapshot = errors.New("预览需要最近一次识别的文件清单——请先在解析视图执行识别")

// rulesPreviewSample 命中样例：文件名 + 所在目录 + 草稿规则产出的声明。
type rulesPreviewSample struct {
	// FileName 文件名（含扩展名）。
	FileName string `json:"fileName"`
	// DirRelKey 所在目录键（相对库根的 '/' 目录路径，根直下为空串）。
	DirRelKey string `json:"dirRelKey,omitempty"`
	// Result 该规则产出的声明（「作者=甲，作品名=乙」形）；纯门组合（自身
	// 零产出）命中时缺省——样例仅文件名+目录。
	Result string `json:"result,omitempty"`
}

// rulesPreviewResult previewRule 动作的应答：命中统计与样例。
type rulesPreviewResult struct {
	// HitCount 命中文件数（命中=该文件命中规则清单含草稿规则 ID，与工作台
	// 命中列同源口径）。
	HitCount int `json:"hitCount"`
	// HitTotal 求值面文件总数（=最近一次识别快照的文件总数）。
	HitTotal int `json:"hitTotal"`
	// Samples 命中样例（至多 previewSampleMax 条；跨目录样例优先、不足回落
	// 同目录；恒非 nil）。
	Samples []rulesPreviewSample `json:"samples"`
}

// previewDraftEntry 草稿 JSON → 规则条目：与条目保存入口同一构造管线——来源
// 校验、ruleFromJSON（模式构造 + 提取动作与条件表达式静态校验）与捕获声明
// 校验，非法草稿即时报错不进预览。预览与保存同形同校验，「预览即保存后」
// 口径的前提。
func previewDraftEntry(ruleJSON []byte) (*RuleEntry, error) {
	var rj ruleEntryJSON
	if err := json.Unmarshal(ruleJSON, &rj); err != nil {
		return nil, fmt.Errorf("规则条目 JSON 无法解析: %w", err)
	}
	if !knownRuleSources[rj.Source] {
		return nil, fmt.Errorf("规则 %q 的来源 %q 非法（合法来源：manual / qna / adjudication；内置规则在代码内置表，启停经内置区开关，不能作为档案条目保存）", rj.ID, rj.Source)
	}
	entry, err := ruleFromJSON(&rj)
	if err != nil {
		return nil, err
	}
	if err := validateCaptureExtractionDeclared(entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// PreviewRule 规则草稿的命中预览（只读，不写档案）：返回命中数、快照文件总数
// 与样例。库根无活动会话或会话尚无识别快照时返回 ErrPreviewRuleNoSnapshot。
// 求值口径见 previewRuleConclusions。
func (m *SessionManager) PreviewRule(root string, ruleJSON []byte) (*rulesPreviewResult, error) {
	conclusions, entry, err := m.previewRuleConclusions(root, ruleJSON)
	if err != nil {
		return nil, err
	}
	result := &rulesPreviewResult{HitTotal: len(conclusions), Samples: []rulesPreviewSample{}}
	var samples, fallback []rulesPreviewSample
	seenDirs := make(map[string]bool)
	for i := range conclusions {
		c := &conclusions[i]
		hit := false
		for _, id := range c.MatchedRuleIDs {
			if id == entry.ID {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		result.HitCount++
		// 该规则产出的声明：终选声明中归属草稿规则者（会话赋值已按维度
		// 覆盖终选——携带赋值求值与保存后识别同口径，被覆盖维度的声明
		// 同样不出现在两侧）。
		var parts []string
		for _, decl := range c.Final {
			if decl.RuleID == entry.ID {
				parts = append(parts, qnaDimensionLabel(decl.Dimension)+"="+decl.Value)
			}
		}
		sample := rulesPreviewSample{FileName: c.FileName, DirRelKey: c.DirRelKey, Result: strings.Join(parts, "，")}
		if seenDirs[c.DirRelKey] {
			fallback = append(fallback, sample)
			continue
		}
		seenDirs[c.DirRelKey] = true
		samples = append(samples, sample)
	}
	// 样例收口：跨目录优先（每目录首条命中，快照序），超出上限的溢出转入
	// 同目录回落池；不足上限时按快照序从回落池补足。
	for len(samples) > previewSampleMax {
		fallback = append([]rulesPreviewSample{samples[len(samples)-1]}, fallback...)
		samples = samples[:len(samples)-1]
	}
	for len(samples) < previewSampleMax && len(fallback) > 0 {
		samples = append(samples, fallback[0])
		fallback = fallback[1:]
	}
	result.Samples = append(result.Samples, samples...)
	return result, nil
}

// previewRuleConclusions 命中预览的求值主体：草稿并入现行档案（同 ID 条目
// 原位替换、新建追加；ID 缺省按保存同款生成去重）后取启用规则集，对该会话
// 最近一次识别快照的全库文件跑一遍合并求值，返回逐文件结论与构造后的草稿
// 条目（ID 已定，供命中匹配）。
//
// 求值参数面与真实识别入口一致——「保存后全量识别结果与此一致」的实现基座：
//   - 合并求值走唯一实现 MergedRecognitionWithTree，求值面与目录树面都取
//     快照文件全量（同目录兄弟清单、各层子女与子目录数按全量树聚合，与识别
//     时磁盘全枚举同构——序列成员计数与目录子女类终结谓词不因求值面缺省
//     而保守失真）；
//   - 库内谓词索引按识别同款 BuildLibraryIndex 构造并挂靠会话缓存条目复用
//     （建任务收批失效；「在库名单」小标与贪婪识别提取的求值依赖；索引缺席
//     会令这两类规则预览失真——命中数与声明偏离保存后识别）；
//   - 会话赋值携带该会话现行赋值（终选覆盖口径同识别）。
//
// 库根无活动会话或会话尚无识别快照时返回 ErrPreviewRuleNoSnapshot；规则档案
// 存储不可读时返回装载错误。
func (m *SessionManager) previewRuleConclusions(root string, ruleJSON []byte) ([]FileConclusion, *RuleEntry, error) {
	entry, err := previewDraftEntry(ruleJSON)
	if err != nil {
		return nil, nil, err
	}
	sess, err := m.cachedSession(root) // 只读求值前置读：共享只读实例
	if err != nil {
		if errors.Is(err, ErrWorkbenchSessionNotFound) {
			return nil, nil, ErrPreviewRuleNoSnapshot
		}
		return nil, nil, err
	}
	if sess.Snapshot == nil {
		return nil, nil, ErrPreviewRuleNoSnapshot
	}
	profile, errs := LoadProfile(m.store)
	for _, loadErr := range errs {
		m.warnf("规则档案载入异常（命中预览）: %v", loadErr)
	}
	if profile == nil {
		return nil, nil, fmt.Errorf("规则档案不可载入，无法预览: %w", errors.Join(errs...))
	}
	if entry.ID == "" {
		entry.ID = ensureRuleIDUnique(profile, generatedRuleID("manual", entry.Description))
	}
	// 规则集=现行档案以草稿替换同 ID 条目（新建追加），再取启用集——与
	// 保存后重识别同一过滤口径（草稿携带停用标记时两侧同停用）。
	mergedRules := make([]RuleEntry, 0, len(profile.Rules)+1)
	replaced := false
	for i := range profile.Rules {
		if !replaced && profile.Rules[i].ID == entry.ID {
			mergedRules = append(mergedRules, *entry)
			replaced = true
			continue
		}
		mergedRules = append(mergedRules, profile.Rules[i])
	}
	if !replaced {
		mergedRules = append(mergedRules, *entry)
	}
	enabled := EffectiveRules(&RuleProfile{SchemaVersion: profile.SchemaVersion, Rules: mergedRules})

	// 快路径（阶段3）：新增草稿且受影响集={草稿} 时，基座逐文件原始声明 ∪ 草稿
	// 逐文件求值声明 → 同一 Synthesize 合成重放；守卫或形态不符返回未命中，
	// 落到下方现行整库全量路径（结果一致）。
	if !replaced {
		if conclusions, ok := m.previewDraftFastConclusions(sess, profile, enabled, entry); ok {
			return conclusions, entry, nil
		}
	}

	files := make([]ScanFile, 0, len(sess.Snapshot.Files))
	for i := range sess.Snapshot.Files {
		f := &sess.Snapshot.Files[i]
		files = append(files, ScanFile{FileName: f.FileName, DirNames: ExtractDirLevels(f.RelPath), RelPath: f.RelPath})
	}
	// 在库名单索引挂靠会话缓存条目复用（建任务收批失效——收批后名单已变，
	// 下次预览重建）；无条目时现场构造。快路径未命中时本段为现行全量求值
	//（清单重建与档案∪草稿合并求值不变）。
	lib := m.libraryIndexOf(sess)
	conclusions := MergedRecognitionWithTree(files, files, enabled, lib, sess.Assignments)
	return conclusions, entry, nil
}

// —— 弹窗草稿快路径（阶段3：草稿逐文件求值 + 基座合成重放）——

// previewFastPathTaken 快路径命中计数探针（测试断言各形态落点用；生产面只增
// 不读）。等值锚测试以调用前后的增量区分快路径/回退路径被走。
var previewFastPathTaken atomic.Int64

// previewFastPathDisabled 快路径测试开关（生产恒 false）：等值锚以同一会话态
// 分别强制走快路径与整库全量路径，逐字节比对两路应答。
var previewFastPathDisabled bool

// previewDraftFastConclusions 弹窗草稿快路径求值主体。启用条件（任一不符即
// 返回未命中、调用方回退整库全量）：
//   - 基座就位：会话缓存条目持有基座（守卫=条目会话实例×铸造来源快照双指针
//     等值，previewFoundationOf），且规则相关件守卫通过——档案指纹与当前
//     载入档案一致（识别后档案被保存改写即失配回退）、求值器构建标识等值
//     （与识别增量闸门同款空串语义）；
//   - 形态分箱：新增草稿（非替换，调用方守卫）且 refs 空（非组合）、非定群
//     形态、且草稿 ID 不被任何启用档案规则引用（refs 门段/条件限定名/refCapture
//     提取三路反查）——受影响集={草稿}；
//   - 等值护栏：启用档案规则均无生效条件（When）与贪婪识别（scan）动作、
//     模式树不消费库内谓词索引（基座按识别轮名单铸造，名单换代即偏离）——
//     两遍/三遍动作产物与草稿首遍产物在全量求值的声明序列内交错不同序，
//     声明级合成（union 保首个/highTrust 取首个）对序列顺序敏感，不可重放；
//   - 基座逐文件规则段覆盖求值面全部文件（增量携带层的完备性兜底）。
//
// 求值：草稿规则对基座铸造清单逐文件求值（求值输入装配与现行路径同构——
// 兄弟表/目录树聚合复用基座预铸件；群组上下文取基座规则段结论：定群规则群
// 或结构群补位），提取段原始声明以基座逐文件原始声明为底（草稿两遍动作可见
// 与全量求值相同的声明集），随后 base∪draft 声明序列走同一 Synthesize 纯函数
// + 会话赋值按维度覆盖——与「草稿并入档案后的整库全量」逐文件同构（合成终选
// 与待裁决由「逐文件全部原始声明+合成策略」唯一决定）。
func (m *SessionManager) previewDraftFastConclusions(sess *WorkbenchSession, profile *RuleProfile, enabled []RuleEntry, entry *RuleEntry) ([]FileConclusion, bool) {
	if previewFastPathDisabled {
		return nil, false
	}
	f := m.previewFoundationOf(sess)
	if f == nil || f.rule == nil {
		return nil, false
	}
	digest, err := profileDigestOf(profile)
	if err != nil || !f.rule.guardMatches(digest, m.evaluatorBuildID()) {
		return nil, false
	}
	if !previewDraftFastEligible(enabled, entry) {
		return nil, false
	}
	sub, layer := f.indep, f.rule
	files := sub.files
	for i := range files {
		if layer.segmentsByRel[files[i].RelPath] == nil {
			return nil, false
		}
	}
	lib := m.libraryIndexOf(sess)
	policy := AggregateSynthesis(enabled)
	// 会话赋值就近解析预计算：与整库全量路径同一构造（目录集=求值面全部文件
	// 的所属目录键；解析序无关——逐目录 DP，全量路径自身依赖同一性质）。
	var assignIdx *assignmentDirIndex
	if len(sess.Assignments) > 0 {
		dirKeys := make(map[string]struct{}, len(files))
		for _, file := range files {
			dirKeys[dirRelKeyOf(file.RelPath)] = struct{}{}
		}
		dirKeyList := make([]string, 0, len(dirKeys))
		for dirKey := range dirKeys {
			dirKeyList = append(dirKeyList, dirKey)
		}
		assignIdx = newAssignmentDirIndex(sess.Assignments, dirKeyList)
	}
	// 草稿规则序列（单条）：求值输入装配、提取两段与合成均走与现行路径同一
	// 实现；停用草稿由提取段按启用位自然跳过（与全量路径同口径）。
	draftRules := []RuleEntry{*entry}
	conclusions := make([]FileConclusion, len(files))
	evaluateFileAt := func(i int) {
		file := files[i]
		seg := layer.segmentsByRel[file.RelPath]
		in := &MatchInput{
			FileName: file.FileName, DirNames: file.DirNames, RelPath: file.RelPath,
			Siblings:               sub.siblingsByDir[dirGroupKey(file.DirNames)],
			AncestorFiles:          ancestorFilesLevels(file.DirNames, sub.siblingsByDir),
			AncestorChildDirCounts: ancestorChildDirCounts(file.DirNames, sub.childDirsByParent),
		}
		// 群组上下文=基座规则段结论的群组产物（定群规则群或结构群补位——
		// 草稿非定群形态，全量路径的定群段与识别轮等值，直挂即同值）。
		ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames), Group: seg.group}
		decision := FileDecision{RelPath: file.RelPath, FileName: file.FileName}
		// 基座逐文件原始声明作底：全量路径的首遍无条件声明即基座声明（护栏
		// 已排除档案规则的条件/贪婪动作），草稿两遍动作据此可见同一声明集；
		// 追加只写底数组可见长度之外，基座条目不被改写。
		decision.Declarations = seg.declarations
		runExtractionStage(&decision, draftRules, in, lib, ctx)
		// 命中规则清单=基座命中（档案序）++ 草稿命中（全量路径中草稿居序尾），
		// 基座切片不被原地追加（副本拼接）。
		matched := seg.matchedRuleIDs
		if len(decision.MatchedRuleIDs) > 0 {
			merged := make([]string, 0, len(seg.matchedRuleIDs)+len(decision.MatchedRuleIDs))
			merged = append(merged, seg.matchedRuleIDs...)
			merged = append(merged, decision.MatchedRuleIDs...)
			matched = merged
		}
		final, pending := Synthesize(decision.Declarations, policy)
		if assignIdx != nil {
			if a := assignIdx.resolveFile(file.RelPath); len(a.coveredDimensions()) > 0 {
				final, pending = applyAssignmentToConclusion(final, pending, &a)
			}
		}
		// 定群规则群成员的元素数组与现行路径同口径（预铸副本、缺席现场拆分
		// ——公共性终判前的原始类别）：快照装配期的 markCommonElements 终判
		//（plain→title）会就地回流识别轮结论，基座段携带的是终判后值，与全量
		// 路径现拆值不同；结构群成员的终判发生在对齐装配内，基座段与全量路径
		// 同为终判后值，直取。
		groupElements := seg.groupElements
		if seg.groupRuleID != "" {
			if els := sub.elementsOf(file.RelPath); els != nil {
				groupElements = els
			} else {
				groupElements = splitFilenameElements(BaseNameWithoutExt(file.FileName), dirNameHints(file.DirNames))
			}
		}
		conclusions[i] = FileConclusion{
			RelPath: file.RelPath, FileName: file.FileName, DirRelKey: dirRelKeyOf(file.RelPath),
			Group: seg.group, MatchedRuleIDs: matched,
			GroupRuleID: seg.groupRuleID, GroupSeqText: seg.groupSeqText, GroupElements: groupElements,
			GroupFormName: seg.groupFormName, GroupFormKeyExpr: seg.groupFormKeyExpr, GroupFormSeqExpr: seg.groupFormSeqExpr,
			Final: final, Pending: pending, rawDecls: decision.Declarations,
		}
	}
	// 并行区与整库全量路径同款形态：块约百余文件、原子游标取块、结论按下标
	// 写入保持输入序；共享输入（基座两件/规则段/合成策略/赋值预计算表）只读
	//（零锁纪律），工作子 panic 沿串行同款冒泡，严禁 recover。
	if chunks := (len(files) + evalFileChunkSize - 1) / evalFileChunkSize; chunks > 1 && evalWorkerCount() > 1 {
		workers := min(evalWorkerCount(), chunks)
		var cursor atomic.Int64
		var wg sync.WaitGroup
		wg.Add(workers)
		for w := 0; w < workers; w++ {
			go func() {
				defer wg.Done()
				for {
					c := int(cursor.Add(1)) - 1
					if c >= chunks {
						return
					}
					start := c * evalFileChunkSize
					end := min(start+evalFileChunkSize, len(files))
					for i := start; i < end; i++ {
						evaluateFileAt(i)
					}
				}
			}()
		}
		wg.Wait()
		previewFastPathTaken.Add(1)
		return conclusions, true
	}
	for i := range files {
		evaluateFileAt(i)
	}
	previewFastPathTaken.Add(1)
	return conclusions, true
}

// previewDraftFastEligible 快路径形态判定（受影响集={草稿}）：草稿非组合
//（refs 空）、非定群形态，且任何启用档案规则不以草稿 ID 为被引成员。替换
// 形态（草稿 ID 与档案条目同 ID）由调用方先行排除。
func previewDraftFastEligible(enabled []RuleEntry, entry *RuleEntry) bool {
	if len(entry.Refs) > 0 || isGroupingRule(entry) {
		return false
	}
	for i := range enabled {
		rule := &enabled[i]
		if rule.ID == entry.ID {
			continue // 草稿自身（形态判定只针对档案规则）
		}
		if ruleReferencesRuleID(rule, entry.ID) {
			return false // 被引用（形态四）：组合门段语义随草稿在场而变
		}
		if archiveRuleBlocksDraftFastPath(rule) {
			return false // 等值护栏：档案规则产物不可按基座重放
		}
	}
	return true
}

// ruleReferencesRuleID 判定规则是否以 ruleID 为被引成员：refs 门段引用、条件
// 表达式的限定键形操作数（「成员ID.rN.组名」）、refCapture 提取动作三路。
// 构造期校验保证后两路 ⊆ refs（rule_profile.go validateExpr/validateRuleRefs），
// 此处兜底直查防存量档案绕过校验漂移。
func ruleReferencesRuleID(rule *RuleEntry, ruleID string) bool {
	if containsString(rule.Refs, ruleID) {
		return true
	}
	for i := range rule.Extractions {
		action := &rule.Extractions[i]
		if action.From == FromRefCapture && action.RefRule == ruleID {
			return true
		}
		if exprReferencesRuleID(action.When, ruleID) {
			return true
		}
	}
	return false
}

// exprReferencesRuleID 递归检查条件表达式树内限定键形操作数是否引用 ruleID
//（子式与四种单子节点全下潜，与 validateExprRefRepeat 同款走法）。
func exprReferencesRuleID(e *CondExpr, ruleID string) bool {
	if e == nil {
		return false
	}
	if e.Kind == ExprCapture {
		if memberID, _, ok := parseRefCaptureName(e.CaptureName); ok && memberID == ruleID {
			return true
		}
	}
	for _, sub := range e.Subs {
		if exprReferencesRuleID(sub, ruleID) {
			return true
		}
	}
	for _, sub := range []*CondExpr{e.Left, e.Right, e.Operand, e.Inner} {
		if exprReferencesRuleID(sub, ruleID) {
			return true
		}
	}
	return false
}

// archiveRuleBlocksDraftFastPath 判定启用档案规则是否阻断快路径（等值护栏）：
//   - 生效条件（When）动作：两遍执行的第二遍产物并入声明序列，与草稿首遍
//     产物在全量求值内交错不同序——union 去重保首个与 highTrust 取首个对
//     序列顺序敏感，基座「首遍原始声明」无法重放交错序；
//   - 贪婪识别（scan）动作：第三遍产物同上；
//   - 模式树消费库内谓词索引：基座按识别轮的库内名单铸造，建任务收批后
//     名单换代，基座命中/声明与全量求值（当前名单）可能偏离；
//   - 非定群形态规则的模式树含群组写点（SeqSubjectPattern / SiblingPattern
//     求值写 ctx.Group，经 absorbScoped 并回共享上下文）：全量路径中草稿居
//     序尾、看到的是被此类规则改写后的群组上下文，基座规则段只存提取段前
//     的初值——群组操作数/群组前缀提取的草稿结论将偏离。定群形态规则被
//     提取段跳过（isGroupingRule 分支），零共享上下文写，不在此列。
func archiveRuleBlocksDraftFastPath(rule *RuleEntry) bool {
	for i := range rule.Extractions {
		action := &rule.Extractions[i]
		if action.When != nil || action.From == FromScan {
			return true
		}
	}
	if !isGroupingRule(rule) && patternWritesGroupContext(rule.Pattern) {
		return true
	}
	return patternDependsOnLibraryIndex(rule.Pattern)
}

// patternWritesGroupContext 模式树求值是否会写共享上下文的群组字段：
// SeqSubjectPattern 与 SiblingPattern 两处写点（行包装经内层下潜）；已知
// 不写类别显式放行，未知类别保守视为写（新类别默认回退，等值优先）。
func patternWritesGroupContext(p Pattern) bool {
	switch t := p.(type) {
	case nil:
		return false
	case *SiblingPattern, *SeqSubjectPattern:
		return true
	case *RowSubjectPattern:
		return patternWritesGroupContext(t.Inner)
	case And:
		for _, sub := range t {
			if patternWritesGroupContext(sub) {
				return true
			}
		}
		return false
	case Or:
		for _, sub := range t {
			if patternWritesGroupContext(sub) {
				return true
			}
		}
		return false
	case Not:
		return patternWritesGroupContext(t.Inner)
	case *RegexPattern, *LocationPattern, *PredicatePattern, *BadgeGatePattern,
		*FileSubjectPattern, *DirSubjectPattern, *ParentNamePattern:
		return false
	default:
		return true
	}
}

// patternDependsOnLibraryIndex 模式树是否消费库内谓词索引：求值侧仅
// PredicatePattern（rule_pattern.go）与 library 值校验门（BadgeGatePattern）
// 两处读索引；已知的不消费类别显式放行，未知类别保守视为依赖（新类别默认
// 回退，等值优先）。
func patternDependsOnLibraryIndex(p Pattern) bool {
	switch t := p.(type) {
	case nil:
		return false
	case *PredicatePattern:
		return true
	case *BadgeGatePattern:
		return t.Kind == CondxBadgeLibrary
	case *RowSubjectPattern:
		for _, gate := range t.Gates {
			if patternDependsOnLibraryIndex(gate) {
				return true
			}
		}
		return patternDependsOnLibraryIndex(t.Inner)
	case And:
		for _, sub := range t {
			if patternDependsOnLibraryIndex(sub) {
				return true
			}
		}
		return false
	case Or:
		for _, sub := range t {
			if patternDependsOnLibraryIndex(sub) {
				return true
			}
		}
		return false
	case Not:
		return patternDependsOnLibraryIndex(t.Inner)
	case *RegexPattern, *LocationPattern, *SiblingPattern,
		*FileSubjectPattern, *DirSubjectPattern, *SeqSubjectPattern, *ParentNamePattern:
		return false
	default:
		return true
	}
}
