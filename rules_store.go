// 规则档案的存取层：KV 最小接口与插件上下文适配器、档案载入（坏规则逐条跳过、
// 旧剥壳字段的一次性迁移备份）、档案保存（多态 JSON 落库 + 写前单代备份与
// 回退），以及用户规则与内置规则（含内置禁用清单）的求值顺序合并。
package main

import (
	"encoding/json"
	"errors"
	"fmt"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// RuleProfileKVKey 为全库单份规则档案在插件 KV 存储中的键。
const RuleProfileKVKey = "rule-profile"

// RuleProfileBackupKVKey 为档案单代备份键：每次覆写档案前，现行档案原文先存入
// 本键（写前备份），误删或覆写中断可回退一代（RollbackProfile）。
const RuleProfileBackupKVKey = "rule-profile.backup"

// RuleProfileMigrationBackupKVKey 为存量档案剥壳迁移的一次性备份键：载入
// 检测到含旧剥壳字段（stripPrefix / stripSuffix）的档案、且本键尚无内容时，
// 把迁移前档案原文整体写入本键；此后任何保存与载入都不再覆写——迁移改写
// 若有错可凭本键整体还原旧档（单代写前备份会随保存滚动丢失，故另设）。
const RuleProfileMigrationBackupKVKey = "rule-profile.migration-backup"

// ProfileStore 为规则档案读写所需的最小存储方法集；签名与 SDK 的
// dto.KVStore 同名方法一致，按需收窄依赖以避免触碰无关能力。
type ProfileStore interface {
	// GetValue 读取键对应的存储值；键不存在时返回零值存储值，不报错。
	GetValue(key string) (*sdkdto.StorageValue, error)
	// SetValue 以明文覆写键对应的存储值。
	SetValue(key string, value string) error
}

// contextProfileStore 把宿主提供的完整插件上下文收窄为规则档案存储。
type contextProfileStore struct {
	// ctx 为宿主注入的插件上下文。
	ctx sdkdto.PluginContext
}

// NewContextProfileStore 把插件上下文适配为规则档案存储；
// ctx 为 nil 时适配结果仍可构造，读写时返回错误而不静默丢弃。
func NewContextProfileStore(ctx sdkdto.PluginContext) ProfileStore {
	return contextProfileStore{ctx: ctx}
}

// GetValue 转发到插件上下文的 KV 读取；上下文缺失时返回错误。
func (s contextProfileStore) GetValue(key string) (*sdkdto.StorageValue, error) {
	if s.ctx == nil {
		return nil, fmt.Errorf("插件上下文不可用，无法读取键 %q", key)
	}
	return s.ctx.GetValue(key)
}

// SetValue 转发到插件上下文的 KV 写入；上下文缺失时返回错误。
func (s contextProfileStore) SetValue(key string, value string) error {
	if s.ctx == nil {
		return fmt.Errorf("插件上下文不可用，无法写入键 %q", key)
	}
	return s.ctx.SetValue(key, value)
}

// emptyProfile 返回空档案：结构版本为当前版本、规则列表为空。
func emptyProfile() *RuleProfile {
	return &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: make([]RuleEntry, 0)}
}

// LoadProfile 从 KV 读取规则档案：键不存在（存储值为 nil 或明文为空）时返回空
// 档案且无错误；携带旧剥壳字段（stripPrefix / stripSuffix）的档案在解析前
// 先把原文一次性写入迁移备份键（已有内容不覆写），再经 ParseProfile 的载入
// 迁移把剥壳配置等价改写为字面括号模板或丢弃留描述注记——载入不回写档案
// 键，迁移结果在下次保存时自然落新形态；数据不可解析或结构版本为旧版
// （v1..v3）时**自动重置**——原文备份进单代备份键、覆写为空 v4 档案、返回
// 空档案与一条知会警告（拒载死锁收口：旧档用户无重置入口、保存/裁决/泛化
// 全被「档案不可载入」阻断）；个别规则构造校验失败时经 ParseProfile 跳过
// 该规则，并返回带规则 ID 的错误，其余规则照常载入，不阻塞整体。
func LoadProfile(store ProfileStore) (*RuleProfile, []error) {
	profile, skips, errs := loadProfile(store)
	if profile != nil {
		// 跳过条目的告警文本照既有口径随 errs 返回（日志与既有文本消费面不变）。
		for _, s := range skips {
			errs = append(errs, errors.New(s.Message))
		}
	}
	return profile, errs
}

// LoadProfileWithSkips 载入档案并结构化携带被跳过的条目（规则管理面错误态行
// 数据源）：errs 不含跳过条目的告警文本——跳过条目由管理视图以行呈现，顶部
// 告警不再重复列出（其余类型告警保持）；完整文本告警经 LoadProfile 口径消费。
// 整体拒载（旧版/损坏自动重置）语义与 LoadProfile 一致，拒载时 skips 为空。
func LoadProfileWithSkips(store ProfileStore) (*RuleProfile, []*skippedRuleEntry, []error) {
	return loadProfile(store)
}

// loadProfile 载入主体（LoadProfile 的结构化内核）：被跳过的条目与告警文本
// 分离返回，呈现形态由两个出口各自决定。
func loadProfile(store ProfileStore) (*RuleProfile, []*skippedRuleEntry, []error) {
	if store == nil {
		return nil, nil, []error{fmt.Errorf("规则档案存储不可用，无法载入档案")}
	}
	stored, err := store.GetValue(RuleProfileKVKey)
	if err != nil {
		return nil, nil, []error{fmt.Errorf("读取规则档案（键 %q）失败: %w", RuleProfileKVKey, err)}
	}
	if stored == nil || stored.Value == "" {
		return emptyProfile(), nil, nil
	}
	// 存量剥壳字段的迁移前兜底：原文携带旧剥壳键时，先把迁移前原文一次性
	// 写入迁移备份键（已有内容不覆写）；备份写入失败即中止载入——迁移属
	// 改写操作，无备份不放行（比照整体重置失败的透传语义）。
	if profileDataHasLegacyStrip([]byte(stored.Value)) {
		if err := backupProfileBeforeStripMigration(store, stored.Value); err != nil {
			return nil, nil, []error{err}
		}
	}
	profile, skips, errs := parseProfile([]byte(stored.Value))
	if profile != nil {
		return profile, skips, errs
	}
	// 整体拒载（旧版/损坏）：自动重置。重置自身失败（存储只读故障）时维持
	// 拒载语义透传——不静默吞错，调用方按存储故障处置。
	empty := emptyProfile()
	data, mErr := empty.MarshalJSON()
	if mErr == nil {
		if sErr := store.SetValue(RuleProfileBackupKVKey, stored.Value); sErr == nil {
			if wErr := store.SetValue(RuleProfileKVKey, string(data)); wErr == nil {
				warn := fmt.Errorf("规则档案不可载入（%s），已自动重置为空档案；原档案原文已备份，重新识别后可在工作台重新生成规则", errs[0])
				return empty, nil, []error{warn}
			}
		}
	}
	return nil, nil, errs
}

// SaveProfile 把规则档案序列化为多态 JSON 后写入 KV；档案为 nil 时按空档案写入。
// 写入前先把现行档案原文存入单代备份键（写前备份）：现行档案缺席（首次写入）时
// 不产生备份；备份写入失败即中止保存——覆写前恒有一代档案可回退。
func SaveProfile(store ProfileStore, p *RuleProfile) error {
	if store == nil {
		return fmt.Errorf("规则档案存储不可用，无法保存档案")
	}
	profile := p
	if profile == nil {
		profile = emptyProfile()
	}
	data, err := profile.MarshalJSON()
	if err != nil {
		return fmt.Errorf("规则档案序列化失败: %w", err)
	}
	if err := backupProfileBeforeOverwrite(store); err != nil {
		return err
	}
	if err := store.SetValue(RuleProfileKVKey, string(data)); err != nil {
		return fmt.Errorf("写入规则档案（键 %q）失败: %w", RuleProfileKVKey, err)
	}
	return nil
}

// backupProfileBeforeOverwrite 写前备份：把现行档案原文存入单代备份键；现行
// 档案缺席（键不存在或明文为空）时不产生备份。
func backupProfileBeforeOverwrite(store ProfileStore) error {
	current, err := store.GetValue(RuleProfileKVKey)
	if err != nil {
		return fmt.Errorf("读取现行规则档案（写前备份）失败: %w", err)
	}
	if current == nil || current.Value == "" {
		return nil
	}
	if err := store.SetValue(RuleProfileBackupKVKey, current.Value); err != nil {
		return fmt.Errorf("写入规则档案备份（键 %q）失败: %w", RuleProfileBackupKVKey, err)
	}
	return nil
}

// backupProfileBeforeStripMigration 迁移前一次性备份：迁移备份键无内容时把
// 迁移前档案原文写入；已有内容（上一轮迁移写入）不覆写——单代写前备份会随
// 保存滚动丢失，本键为迁移改写的整体还原兜底。
func backupProfileBeforeStripMigration(store ProfileStore, raw string) error {
	backup, err := store.GetValue(RuleProfileMigrationBackupKVKey)
	if err != nil {
		return fmt.Errorf("读取规则档案迁移备份（键 %q）失败: %w", RuleProfileMigrationBackupKVKey, err)
	}
	if backup != nil && backup.Value != "" {
		return nil
	}
	if err := store.SetValue(RuleProfileMigrationBackupKVKey, raw); err != nil {
		return fmt.Errorf("写入规则档案迁移备份（键 %q）失败: %w", RuleProfileMigrationBackupKVKey, err)
	}
	return nil
}

// RollbackProfile 回退一代档案：把备份键内容写回档案键，同时把覆写前的现行档案
// 换位存入备份键（回退本身可再回退一次撤销）。备份缺席（从未产生过覆写）时返回
// 错误；回退直接以原文写键，不经 SaveProfile（其写前备份会与换位重复）。
func RollbackProfile(store ProfileStore) error {
	if store == nil {
		return fmt.Errorf("规则档案存储不可用，无法回退档案")
	}
	backup, err := store.GetValue(RuleProfileBackupKVKey)
	if err != nil {
		return fmt.Errorf("读取规则档案备份（键 %q）失败: %w", RuleProfileBackupKVKey, err)
	}
	if backup == nil || backup.Value == "" {
		return fmt.Errorf("无可回退的规则档案备份（尚未产生过覆写）")
	}
	current, err := store.GetValue(RuleProfileKVKey)
	if err != nil {
		return fmt.Errorf("读取现行规则档案失败: %w", err)
	}
	if err := store.SetValue(RuleProfileKVKey, backup.Value); err != nil {
		return fmt.Errorf("回写规则档案（键 %q）失败: %w", RuleProfileKVKey, err)
	}
	if current != nil && current.Value != "" {
		if err := store.SetValue(RuleProfileBackupKVKey, current.Value); err != nil {
			return fmt.Errorf("更新规则档案备份（键 %q）失败: %w", RuleProfileBackupKVKey, err)
		}
	}
	return nil
}

// ProfileBackupExists 备份键是否有可回退的一代档案内容。
func ProfileBackupExists(store ProfileStore) bool {
	if store == nil {
		return false
	}
	backup, err := store.GetValue(RuleProfileBackupKVKey)
	return err == nil && backup != nil && backup.Value != ""
}

// rawProfileEntriesJSON 档案原文的最小条目数组形态：rules 逐条保留原文字节
// （json.RawMessage），删除手术只移除目标条目、不改写未涉及条目——载入跳过
// 的死条目与未来版本未知字段零损失。
type rawProfileEntriesJSON struct {
	SchemaVersion int               `json:"schemaVersion"`
	Rules         []json.RawMessage `json:"rules"`
}

// deleteRuleEntryFromStore 按规则 ID 在档案原文层删除条目并落库：载入跳过的
// 死条目不在已载入集内（按已载入集删即无出口），删除须作用于存储层原文；写前
// 备份照常产生（误删可回退一代）。条目不在档（含档案缺席）返回错误。
func deleteRuleEntryFromStore(store ProfileStore, ruleID string) error {
	if store == nil {
		return fmt.Errorf("规则档案存储不可用，无法删除规则")
	}
	stored, err := store.GetValue(RuleProfileKVKey)
	if err != nil {
		return fmt.Errorf("读取规则档案（键 %q）失败: %w", RuleProfileKVKey, err)
	}
	raw := ""
	if stored != nil {
		raw = stored.Value
	}
	rewritten, hit, err := deleteRuleEntryFromRaw(raw, ruleID)
	if err != nil {
		return err
	}
	if !hit {
		return fmt.Errorf("档案中不存在规则 %q，可能已被删除", ruleID)
	}
	if err := backupProfileBeforeOverwrite(store); err != nil {
		return err
	}
	if err := store.SetValue(RuleProfileKVKey, rewritten); err != nil {
		return fmt.Errorf("写入规则档案（键 %q）失败: %w", RuleProfileKVKey, err)
	}
	return nil
}

// deleteRuleEntryFromRaw 在档案原文中移除全部同 ID 条目（ID 重复的档案一次清
// 干净）：逐条目原文过滤、未涉及条目字节原样保留，顶层其余字段按原文值重新
// 序列化。返回改写后的档案原文与是否命中。
func deleteRuleEntryFromRaw(raw, ruleID string) (string, bool, error) {
	if raw == "" {
		return "", false, nil
	}
	var doc rawProfileEntriesJSON
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", false, fmt.Errorf("规则档案原文不可解析: %w", err)
	}
	kept := make([]json.RawMessage, 0, len(doc.Rules))
	hit := false
	for _, item := range doc.Rules {
		var idOnly struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(item, &idOnly) == nil && idOnly.ID == ruleID {
			hit = true
			continue
		}
		kept = append(kept, item)
	}
	if !hit {
		return "", false, nil
	}
	doc.Rules = kept
	data, err := json.Marshal(&doc)
	if err != nil {
		return "", false, fmt.Errorf("规则档案序列化失败: %w", err)
	}
	return string(data), true, nil
}

// EffectiveRules 返回参与求值的规则序列：用户档案中启用的规则（保持原有
// 顺序）。v4 起内置规则退休为例子模板、不再参与求值（rules_admin 仅作展示
// 参考），合并点随之收敛为纯用户档案。
func EffectiveRules(user *RuleProfile) []RuleEntry {
	rules := make([]RuleEntry, 0)
	if user == nil {
		return rules
	}
	for _, rule := range user.Rules {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	return rules
}
