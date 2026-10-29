// 规则草稿命中预览（previewRule 只读动作）：编辑面的未入档草稿按保存同款
// 构造管线转为规则条目，并入现行档案（同 ID 条目原位替换、新建追加、缺省 ID
// 按保存同款生成去重），对该会话最近一次识别快照的全库文件跑一遍合并求值。
// 只读：不写档案、不改会话状态。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
//   - 库内谓词索引按识别同款 BuildLibraryIndex 构造（「在库名单」小标与
//     贪婪识别提取的求值依赖；索引缺席会令这两类规则预览失真——命中数与
//     声明偏离保存后识别）；
//   - 会话赋值携带该会话现行赋值（终选覆盖口径同识别）。
//
// 库根无活动会话或会话尚无识别快照时返回 ErrPreviewRuleNoSnapshot；规则档案
// 存储不可读时返回装载错误。
func (m *SessionManager) previewRuleConclusions(root string, ruleJSON []byte) ([]FileConclusion, *RuleEntry, error) {
	entry, err := previewDraftEntry(ruleJSON)
	if err != nil {
		return nil, nil, err
	}
	sess, err := m.loadSession(root)
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

	files := make([]ScanFile, 0, len(sess.Snapshot.Files))
	for i := range sess.Snapshot.Files {
		f := &sess.Snapshot.Files[i]
		files = append(files, ScanFile{FileName: f.FileName, DirNames: ExtractDirLevels(f.RelPath), RelPath: f.RelPath})
	}
	var lib *LibraryIndex
	if m.ctx != nil {
		lib = BuildLibraryIndex(m.ctx, m.warnf)
	}
	conclusions := MergedRecognitionWithTree(files, files, enabled, lib, sess.Assignments)
	return conclusions, entry, nil
}
