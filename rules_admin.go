// 规则管理面（工作台「规则」子页）的编辑事务与前端视图：档案条目的保存/启停/
// 删除/全序重排、内置规则的禁用清单维护与档案备份回退。全部档案写经会话管理器的
// UpdateProfile 单点串行；条目保存复用构造期校验管线单条即时报错，不落盘坏条目。
package main

import (
	"encoding/json"
	"errors"
	"fmt"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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
	// ResourceTypes 资源类型注册表投影（聚合编辑面分轨入口的类型清单数据源：
	// 内置 6 类+插件自定义类型；查询失败为空——对应告警随 Warnings 透出）。
	// JSON 形态为 camelCase 镜像（resourceTypeSpecJSON）：SDK proto 结构体的
	// json 标签是 snake_case（protoc 生成），前端聚合编辑面按 camelCase 消费
	// （ruleFormModel.ts AggTypeSpec）——直接内嵌 proto 结构体会令分轨入口的
	// 类型清单恒空（2026-11-14 阶段7 live-test 实机发现）。
	ResourceTypes []*resourceTypeSpecJSON `json:"resourceTypes,omitempty"`
	// HitCounts 规则命中统计（会话识别快照 ruleStats 的 ID→命中件数投影；
	// 无会话/无快照时缺省——呈现层按缺席省略命中段）。
	HitCounts map[string]int `json:"hitCounts,omitempty"`
	// Skipped 载入跳过的条目（错误态行）：可见（ID/描述/跳过原因就地呈现）、
	// 仅可删除——不进 Rules（不在已载入集，不可启用/编辑/复制）；顶部 Warnings
	// 不再重复列对应「跳过规则」告警（其余类型告警保持）。
	Skipped []*skippedRuleJSON `json:"skipped,omitempty"`
	// BackupAvailable 写前备份键是否有可回退的一代档案。
	BackupAvailable bool `json:"backupAvailable"`
}

// skippedRuleJSON 规则管理视图的载入跳过条目行（错误态）：仅携带呈现与删除
// 寻址所需字段（id/描述/跳过原因）。
type skippedRuleJSON struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

// skippedRulesAdminJSON 结构化跳过条目批量转 JSON 视图形态（nil 条目跳过）。
func skippedRulesAdminJSON(skips []*skippedRuleEntry) []*skippedRuleJSON {
	out := make([]*skippedRuleJSON, 0, len(skips))
	for _, s := range skips {
		if s == nil {
			continue
		}
		out = append(out, &skippedRuleJSON{ID: s.ID, Description: s.Description, Reason: s.Reason})
	}
	return out
}

// resourceTypeSpecJSON 规则管理视图的资源类型投影 JSON 形态：SDK proto 结构体
// 逐字段镜像为 camelCase 键（对齐前端 ruleFormModel.ts AggTypeSpec/AggRoleSpec
// 的消费口径；proto 生成标签为 snake_case，不可直接序列化出档）。
type resourceTypeSpecJSON struct {
	ResourceType   string                        `json:"resourceType"`
	Roles          []*storeRoleSpecJSON          `json:"roles"`
	PrimaryRoles   []string                      `json:"primaryRoles,omitempty"`
	StoreStandards map[string]*storeStandardJSON `json:"storeStandards,omitempty"`
}

// storeRoleSpecJSON 结构角色规约的 JSON 形态（storeType/min/max）。
type storeRoleSpecJSON struct {
	StoreType string `json:"storeType"`
	Min       int    `json:"min"`
	Max       int    `json:"max"`
}

// storeStandardJSON 角色文件标准的 JSON 形态。
type storeStandardJSON struct {
	Description string   `json:"description"`
	Formats     []string `json:"formats"`
	Generation  string   `json:"generation"`
}

// resourceTypesAdminJSON 投影条目批量转 JSON 视图形态（nil 条目跳过）。
func resourceTypesAdminJSON(items []*sdkdto.ResourceTypeSpec) []*resourceTypeSpecJSON {
	out := make([]*resourceTypeSpecJSON, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		e := &resourceTypeSpecJSON{
			ResourceType: item.ResourceType,
			PrimaryRoles: item.PrimaryRoles,
		}
		if item.Roles != nil {
			e.Roles = make([]*storeRoleSpecJSON, 0, len(item.Roles))
			for _, r := range item.Roles {
				if r == nil {
					continue
				}
				e.Roles = append(e.Roles, &storeRoleSpecJSON{StoreType: r.StoreType, Min: int(r.Min), Max: int(r.Max)})
			}
		}
		if item.StoreStandards != nil {
			e.StoreStandards = make(map[string]*storeStandardJSON, len(item.StoreStandards))
			for k, v := range item.StoreStandards {
				if v == nil {
					continue
				}
				e.StoreStandards[k] = &storeStandardJSON{Description: v.Description, Formats: v.Formats, Generation: v.Generation}
			}
		}
		out = append(out, e)
	}
	return out
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

// adminTypeProjection 规则管理视图的类型投影拉取：一次 ListResourceTypes 同供
// 两处消费——聚合规则的类型锚定（悬空停用+告警，与识别求值轮同口径）与编辑面
// 分轨入口的类型清单（内置 6 类+插件自定义类型，类型先行的单一事实源）。上下文
// 缺席或查询失败返回 nil 索引——锚定按类型悬空处理、类型清单为空。
func (m *SessionManager) adminTypeProjection() ([]*sdkdto.ResourceTypeSpec, *resourceTypeIndex) {
	if m.ctx == nil {
		return nil, nil
	}
	resp, err := m.ctx.ListResourceTypes()
	if err != nil {
		m.warnf("查询资源类型注册表失败（规则管理视图，聚合规则按类型悬空停用处理）: %v", err)
		return nil, nil
	}
	if resp == nil {
		return nil, nil
	}
	return resp.Items, newResourceTypeIndex(resp.Items)
}

// RulesAdminView 载入规则管理视图：档案（坏条目跳过并结构化转错误态行）+ 聚合
// 规则类型锚定（悬空停用+告警，与识别求值侧一致——否则悬空条目在管理面显示为
// 启用而求值已停用）+ 类型清单投影 + 写前备份在场。档案不可载入（损坏/版本不符）
// 时返回错误。只读，不加写锁；锚定只改内存态视图对象、不回写存储。
func (m *SessionManager) RulesAdminView() (*rulesAdminView, error) {
	profile, skips, errs := LoadProfileWithSkips(m.store)
	if profile == nil {
		return nil, fmt.Errorf("规则档案不可载入: %w", errors.Join(errs...))
	}
	items, typeIdx := m.adminTypeProjection()
	warnings := errorTexts(errs)
	warnings = append(warnings, errorTexts(AnchorAggregateRules(profile, typeIdx))...)
	view, err := rulesAdminViewOf(profile, warnings)
	if err != nil {
		return nil, err
	}
	view.Skipped = skippedRulesAdminJSON(skips)
	view.ResourceTypes = resourceTypesAdminJSON(items)
	view.BackupAvailable = ProfileBackupExists(m.store)
	return view, nil
}

// attachRuleHitCounts 规则管理视图补挂命中统计（「来源:X · 命中 N 件」的命中
// 段数据面）：取 root 会话识别快照的 ruleStats（规则 ID → 命中件数）。root 空、
// 无会话或无快照统计时视图不变（缺席=呈现层省略命中段）。只读会话缓存。
func (m *SessionManager) attachRuleHitCounts(view *rulesAdminView, root string) {
	if view == nil || root == "" {
		return
	}
	sess, err := m.cachedSession(root)
	if err != nil || sess == nil || sess.Snapshot == nil || len(sess.Snapshot.RuleStats) == 0 {
		return
	}
	counts := make(map[string]int, len(sess.Snapshot.RuleStats))
	for _, stat := range sess.Snapshot.RuleStats {
		counts[stat.ID] = stat.HitCount
	}
	view.HitCounts = counts
}

// knownRuleSources 档案条目的合法来源集合。builtin 不在其中——内置规则在代码
// 内置表、以禁用清单承载启停，档案内出现同 ID 条目会同份规则双路参与求值。
// preset 为预置种子（首档初始化写入的出厂条目）：与用户条目同权——可启停、
// 可编辑、可复制（编辑保存按原来源原位替换）。
var knownRuleSources = map[RuleSource]bool{
	RuleSourceManual: true, RuleSourceQna: true, RuleSourceAdjudication: true,
	RuleSourcePreset: true,
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
		return nil, fmt.Errorf("规则 %q 的来源 %q 非法（合法来源：manual / qna / adjudication / preset；内置规则在代码内置表、以禁用清单承载启停，不能作为档案条目保存）", rj.ID, rj.Source)
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

// validateCaptureExtractionDeclared 校验取值来源为捕获组的提取动作所引用的捕获
// 组名在规则的捕获声明集合内，未声明的引用在运行期恒取值失败——规则照常命中但
// 零声明。属编辑面的构造期即时报错（ruleFromJSON 内 validateExtractions 的
// 双保险）。v4 条件行条目：声明集=declaredConditionCaptures（rN.组名 行号前缀
// 口径，提取动作 Row 缺省按 1 兜底）；v3 pattern 条目：regex 节点 captures
// （仅 filename / relpath 目标参与绑定）。
func validateCaptureExtractionDeclared(rule *RuleEntry) error {
	v4 := rule.Conditions != nil
	declared := declaredPatternCaptures(rule.Pattern)
	if v4 {
		declared = declaredConditionCaptures(rule.Conditions)
	}
	for i := range rule.Extractions {
		action := &rule.Extractions[i]
		if action.From != FromCapture || action.CaptureName == "" {
			continue
		}
		key := action.CaptureName
		if v4 {
			row := action.Row
			if row <= 0 {
				row = 1
			}
			key = rowCaptureName(row, action.CaptureName)
		}
		if declared[key] || declared[key+"#canon"] {
			continue
		}
		if v4 {
			return fmt.Errorf(
				"规则 %q 的第 %d 条提取动作引用捕获组 %q，但该组未在条件行模板占位词中声明（行号 %d），未声明的引用不会产出任何声明",
				rule.ID, i+1, action.CaptureName, action.Row)
		}
		return fmt.Errorf(
			"规则 %q 的第 %d 条提取动作引用捕获组 %q，但该组未在模式 regex 节点的 captures 数组内显式声明（仅 filename / relpath 目标的捕获组参与绑定），未声明的引用不会产出任何声明",
			rule.ID, i+1, action.CaptureName)
	}
	return nil
}

// ApplyDeleteRule 删除档案条目：按 ID 寻址，作用于档案存储层原文——载入跳过
// 的死条目不在已载入集内（按已载入集删即无出口），按原文删除后重载告警随消；
// 其余条目（含其他死条目与未知字段）字节原样保留。未命中报错。写前备份照常
// 产生——误删可经回退一代恢复。返回删除后的规则管理视图。
func (m *SessionManager) ApplyDeleteRule(ruleID string) (*rulesAdminView, error) {
	if ruleID == "" {
		return nil, fmt.Errorf("删除规则缺少规则 ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := deleteRuleEntryFromStore(m.store, ruleID); err != nil {
		return nil, fmt.Errorf("档案变更被中止（入口 %s）: %w", ArchiveWriteRuleEdit, err)
	}
	m.logf("规则档案已更新（入口 %s）: 删除规则 %q", ArchiveWriteRuleEdit, ruleID)
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

// ApplyReorderRules 规则全序重排事务（排序操作入口）：ruleIDs 为前端一次请求
// 传入的全量规则 ID 新序清单（全序清单协议），经与档案现有规则 ID 集合的多重集
// 精确校验（不漏、不重、无未知 ID）后按清单序重排 Rules 数组一次覆写。校验在
// UpdateProfile 的 mutate 内完成——不一致即中止返回错误，档案字节零改动；一致
// 则落档并产生写前单代备份（复用既有写入链，白得单点串行与回退一代）。重排作用
// 于存储数组序，与启用态无关（禁用行照样占位可移）。返回重排后的规则管理视图。
func (m *SessionManager) ApplyReorderRules(ruleIDs []string) (*rulesAdminView, error) {
	if err := m.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error {
		reordered, err := reorderRuleEntries(p.Rules, ruleIDs)
		if err != nil {
			return err
		}
		p.Rules = reordered
		return nil
	}); err != nil {
		return nil, err
	}
	return m.RulesAdminView()
}

// reorderRuleEntries 按全序清单重排规则数组：清单须与现有规则 ID 的多重集精确
// 一致（逐元素在场+无重复+不漏），未知 ID / 重复 ID / 漏 ID 均报错且不产出新
// 数组；一致时返回按清单序搬运既有条目的新数组（条目内容零变化，仅位置重排）。
func reorderRuleEntries(rules []RuleEntry, ruleIDs []string) ([]RuleEntry, error) {
	pos := make(map[string]int, len(rules))
	for i := range rules {
		pos[rules[i].ID] = i
	}
	reordered := make([]RuleEntry, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for _, id := range ruleIDs {
		i, ok := pos[id]
		if !ok {
			return nil, fmt.Errorf("重排清单含档案中不存在的规则 %q", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("重排清单中规则 %q 重复出现", id)
		}
		seen[id] = true
		reordered = append(reordered, rules[i])
	}
	if len(seen) != len(pos) {
		missing := make([]string, 0)
		for _, r := range rules {
			if !seen[r.ID] {
				missing = append(missing, r.ID)
			}
		}
		return nil, fmt.Errorf("重排清单漏掉档案中的规则 %v（清单须为全部规则 ID 的全序排列，不漏不重）", missing)
	}
	return reordered, nil
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

// ApplyRestorePresetRules 恢复预置种子（误删/改坏兜底）：把档案中缺席的预置聚合
// 种子按稳定 ID 补回——在场条目（含用户改写过的）一律不动，改坏的条目先删除再
// 恢复即回出厂形态。返回补回条数与恢复后的规则管理视图。持写锁（恢复=档案写，
// 与回退一代同款串行口径）。
func (m *SessionManager) ApplyRestorePresetRules() (int, *rulesAdminView, error) {
	m.mu.Lock()
	added, err := RestorePresetRules(m.store)
	m.mu.Unlock()
	if err != nil {
		return 0, nil, err
	}
	if added > 0 {
		m.logf("已补回 %d 条缺席的预置种子规则", added)
	}
	view, err := m.RulesAdminView()
	if err != nil {
		return added, nil, err
	}
	return added, view, nil
}
