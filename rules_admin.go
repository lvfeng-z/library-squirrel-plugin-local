// 规则管理面（工作台「规则」子页）的编辑事务与前端视图：档案条目的保存/启停/
// 删除、内置规则的禁用清单维护与档案备份回退。全部档案写经会话管理器的
// UpdateProfile 单点串行；条目保存复用构造期校验管线单条即时报错，不落盘坏条目。
package main

import (
	"encoding/json"
	"errors"
	"fmt"
)

// rulesAdminView 规则管理面的前端视图：档案条目（ruleEntryJSON 形态，pattern/
// extractions 即 JSON 片段编辑的原文）、例子模板表、档案版本、载入告警与
// 写前备份在场。v4 起内置规则退休为例子模板（不参与求值、无禁用清单）。
type rulesAdminView struct {
	// SchemaVersion 档案当前结构版本。
	SchemaVersion int `json:"schemaVersion"`
	// Rules 档案条目列表（呈现顺序即档案顺序）。
	Rules []*ruleEntryJSON `json:"rules"`
	// Warnings 档案载入告警（坏条目被跳过等，逐条人话——呈现给用户核对档案）。
	Warnings []string `json:"warnings,omitempty"`
	// Examples 例子模板表（v4 词条演示——格式模板写法参考，不参与求值）。
	Examples []exampleTemplate `json:"examples"`
	// BackupAvailable 写前备份键是否有可回退的一代档案。
	BackupAvailable bool `json:"backupAvailable"`
}

// errorTexts 错误列表 → 文本列表（视图告警用）。
func errorTexts(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

// rulesAdminViewOf 档案 → 规则管理视图（不含备份在场——需查存储，由调用方填充）。
func rulesAdminViewOf(profile *RuleProfile, warnings []string) (*rulesAdminView, error) {
	view := &rulesAdminView{
		SchemaVersion: profile.SchemaVersion,
		Rules:         make([]*ruleEntryJSON, 0, len(profile.Rules)),
		Warnings:      warnings,
		Examples:      exampleTemplates(),
	}
	for i := range profile.Rules {
		rj, err := ruleToJSON(profile.Rules[i])
		if err != nil {
			return nil, fmt.Errorf("规则 %q 转为编辑形态失败: %w", profile.Rules[i].ID, err)
		}
		view.Rules = append(view.Rules, rj)
	}
	return view, nil
}

// RulesAdminView 载入规则管理视图：档案（坏条目跳过并转告警透出）+ 内置表禁用态
// + 写前备份在场。档案不可载入（损坏/版本不符）时返回错误。只读，不加写锁。
func (m *SessionManager) RulesAdminView() (*rulesAdminView, error) {
	profile, errs := LoadProfile(m.store)
	if profile == nil {
		return nil, fmt.Errorf("规则档案不可载入: %w", errors.Join(errs...))
	}
	view, err := rulesAdminViewOf(profile, errorTexts(errs))
	if err != nil {
		return nil, err
	}
	view.BackupAvailable = ProfileBackupExists(m.store)
	return view, nil
}

// knownRuleSources 档案条目的合法来源集合。builtin 不在其中——内置规则在代码
// 内置表、以禁用清单承载启停，档案内出现同 ID 条目会同份规则双路参与求值。
var knownRuleSources = map[RuleSource]bool{
	RuleSourceManual: true, RuleSourceQna: true, RuleSourceAdjudication: true,
}

// ApplySaveRule 规则条目保存事务（规则编辑面入口）：ruleJSON 为一条规则条目的
// 完整 JSON（ruleEntryJSON 形态），经构造期校验管线（ruleFromJSON：模式构造 +
// 提取动作与条件表达式静态校验）单条即时报错——非法条目拒绝且不落盘。ID 命中
// 档案既有条目时原位替换（保持档案顺序），否则追加；ID 缺省时按 manual 来源
// 自动生成去重。返回保存后的规则管理视图。
func (m *SessionManager) ApplySaveRule(ruleJSON []byte) (*rulesAdminView, error) {
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
	if err := m.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error {
		if entry.ID == "" {
			entry.ID = ensureRuleIDUnique(p, generatedRuleID("manual", entry.Description))
		}
		for i := range p.Rules {
			if p.Rules[i].ID == entry.ID {
				p.Rules[i] = *entry
				return nil
			}
		}
		p.Rules = append(p.Rules, *entry)
		return nil
	}); err != nil {
		return nil, err
	}
	return m.RulesAdminView()
}

// validateCaptureExtractionDeclared 校验取值来源为捕获组的提取动作所引用的捕获组
// 名在规则模式的声明集合内：只有 regex 节点 captures 数组显式声明（且目标为
// filename / relpath）的捕获组会绑定进解析上下文，未声明的引用在运行期恒取值
// 失败——规则照常命中但零声明。属编辑面的构造期即时报错。
func validateCaptureExtractionDeclared(rule *RuleEntry) error {
	declared := declaredPatternCaptures(rule.Pattern)
	for i := range rule.Extractions {
		action := &rule.Extractions[i]
		if action.From == FromCapture && action.CaptureName != "" && !declared[action.CaptureName] {
			return fmt.Errorf(
				"规则 %q 的第 %d 条提取动作引用捕获组 %q，但该组未在模式 regex 节点的 captures 数组内显式声明（仅 filename / relpath 目标的捕获组参与绑定），未声明的引用不会产出任何声明",
				rule.ID, i+1, action.CaptureName)
		}
	}
	return nil
}

// ApplyDeleteRule 删除档案条目：按 ID 寻址，未命中报错。写前备份照常产生——误删
// 可经回退一代恢复。返回删除后的规则管理视图。
func (m *SessionManager) ApplyDeleteRule(ruleID string) (*rulesAdminView, error) {
	if ruleID == "" {
		return nil, fmt.Errorf("删除规则缺少规则 ID")
	}
	if err := m.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error {
		for i := range p.Rules {
			if p.Rules[i].ID == ruleID {
				p.Rules = append(p.Rules[:i], p.Rules[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("档案中不存在规则 %q，可能已被删除", ruleID)
	}); err != nil {
		return nil, err
	}
	return m.RulesAdminView()
}

// ApplySetRuleEnabled 档案条目启停：按 ID 寻址改 Enabled，未命中报错。返回更新
// 后的规则管理视图。
func (m *SessionManager) ApplySetRuleEnabled(ruleID string, enabled bool) (*rulesAdminView, error) {
	if ruleID == "" {
		return nil, fmt.Errorf("启停规则缺少规则 ID")
	}
	if err := m.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error {
		for i := range p.Rules {
			if p.Rules[i].ID == ruleID {
				p.Rules[i].Enabled = enabled
				return nil
			}
		}
		return fmt.Errorf("档案中不存在规则 %q", ruleID)
	}); err != nil {
		return nil, err
	}
	return m.RulesAdminView()
}

// ApplySetBuiltinDisabled 内置规则启停（已退役）：v4 起内置规则退休为例子
// 模板（exampleTemplates——仅展示参考，不参与求值、无禁用态）。接口保留
// 返回退休错误，前端旧调用得到可读回应（期2 前端三步漏斗同步撤除该入口）。
func (m *SessionManager) ApplySetBuiltinDisabled(builtinID string, disabled bool) (*rulesAdminView, error) {
	return nil, fmt.Errorf("内置规则已退休为例子模板（不参与求值），规则 %q 无启停可言；如需该形态请从例子模板复制为档案条目", builtinID)
}

// ApplyRollbackProfile 档案回退一代（单点串行）：备份键内容写回档案键、现行档案
// 换位进备份键。备份缺席时返回错误。持写锁完成回退与视图载入，避免与并发档案写
// 竞争。返回回退后的规则管理视图。
func (m *SessionManager) ApplyRollbackProfile() (*rulesAdminView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := RollbackProfile(m.store); err != nil {
		return nil, err
	}
	m.logf("规则档案已回退一代（写前备份换位）")
	return m.RulesAdminView()
}
