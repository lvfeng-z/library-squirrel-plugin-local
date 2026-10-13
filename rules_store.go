// 规则档案的存取层：KV 最小接口与插件上下文适配器、档案载入（坏规则逐条跳过）、
// 档案保存（多态 JSON 落库），以及用户规则与内置规则的求值顺序合并。
package main

import (
	"fmt"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// RuleProfileKVKey 为全库单份规则档案在插件 KV 存储中的键。
const RuleProfileKVKey = "rule-profile"

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
// 档案且无错误；数据不可解析或结构版本与当前版本不符时返回 nil 档案与单个错误；
// 个别规则构造校验失败时经 ParseProfile 跳过该规则，并返回带规则 ID 的错误，
// 其余规则照常载入，不阻塞整体。
func LoadProfile(store ProfileStore) (*RuleProfile, []error) {
	if store == nil {
		return nil, []error{fmt.Errorf("规则档案存储不可用，无法载入档案")}
	}
	stored, err := store.GetValue(RuleProfileKVKey)
	if err != nil {
		return nil, []error{fmt.Errorf("读取规则档案（键 %q）失败: %w", RuleProfileKVKey, err)}
	}
	if stored == nil || stored.Value == "" {
		return emptyProfile(), nil
	}
	return ParseProfile([]byte(stored.Value))
}

// SaveProfile 把规则档案序列化为多态 JSON 后写入 KV；档案为 nil 时按空档案写入。
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
	if err := store.SetValue(RuleProfileKVKey, string(data)); err != nil {
		return fmt.Errorf("写入规则档案（键 %q）失败: %w", RuleProfileKVKey, err)
	}
	return nil
}

// EffectiveRules 返回参与求值的规则序列：用户档案中启用的规则在前、内置规则表中
// 启用的规则在后（两段内部各自保持原有顺序）。同一对象同时命中用户规则与内置规则
// 时，用户规则优先由「用户在前」的求值顺序承载，执行面的顺序接线在流水阶段完成；
// user 为 nil 时仅返回内置规则。
func EffectiveRules(user *RuleProfile) []RuleEntry {
	rules := make([]RuleEntry, 0)
	if user != nil {
		for _, rule := range user.Rules {
			if rule.Enabled {
				rules = append(rules, rule)
			}
		}
	}
	for _, rule := range BuiltinRules() {
		if rule.Enabled {
			rules = append(rules, rule)
		}
	}
	return rules
}
