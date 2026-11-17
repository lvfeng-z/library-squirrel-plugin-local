// 写档保真（载入跳过条目在写档往返中原样存续）的回归测试：live-test
// 2026-11-17 bk-dirlevel-refscope 遗留缺陷1——UpdateProfile 锁内 LoadProfile
// （跳过条目不回填内存集）→ mutate → SaveProfile 全量序列化曾把载入跳过的条目
// 从档案物理丢弃（供给方悬空的消费规则在「重建供给方」保存时被静默删除，剧本2
// 帧4「恢复即复效」不可达）。修复语义：跳过条目原文随档案内存对象存续、
// SaveProfile 原样回写（不复活、不再造），后续载入照常跳过+告警（「载入失败
// N 条」UI 语义不变），直到用户显式删除（原文层手术）或以同 ID 重写该条目
// （写档侧顶替退场）；v4→v5 归一化语义不变（v4 内被跳过的 refs/dirLevel 条目
// 在 v5 档中原文存续——防御性跳过、每次载入告警）。
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// writeRefScopeFixture 直写档案：供给方（纯目录行）+消费规则（refScope 附件引用
// 供给方）+旁证规则各一条——供给方删除后消费规则即成载入跳过的悬空条目。
func writeRefScopeFixture(t *testing.T, store *fakeSessionStore) {
	t.Helper()
	provider := scopeRuleJSON("bk-provider", []CondxRow{scopeDirRow("Artist[{作者}]", nil)}, nil)
	consumer := scopeRuleJSON("bk-consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("bk-provider", 1)})}, nil)
	other := scopeRuleJSON("bk-other", []CondxRow{scopeDirRow("Other", nil)}, nil)
	data, err := json.Marshal(&ruleProfileJSON{SchemaVersion: CurrentSchemaVersion, Rules: []ruleEntryJSON{provider, consumer, other}})
	if err != nil {
		t.Fatalf("序列化档案失败: %v", err)
	}
	store.values[RuleProfileKVKey] = string(data)
}

// danglingConsumerFixture 供给方删除后的悬空现场：返回管理器与存储（消费规则
// 已成载入跳过的死条目、旁证规则照常载入）。
func danglingConsumerFixture(t *testing.T) (*SessionManager, *fakeSessionStore) {
	t.Helper()
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	writeRefScopeFixture(t, store)
	if _, err := mgr.ApplyDeleteRule("bk-provider"); err != nil {
		t.Fatalf("删除供给方失败: %v", err)
	}
	profile, skips, _ := LoadProfileWithSkips(store)
	if len(profile.Rules) != 1 || profile.Rules[0].ID != "bk-other" {
		t.Fatalf("删供给方后应仅载入旁证规则: %+v", profile.Rules)
	}
	if len(skips) != 1 || skips[0].ID != "bk-consumer" {
		t.Fatalf("消费规则应成载入跳过的悬空条目: %+v", skips)
	}
	return mgr, store
}

// TestUpdateProfileWriteKeepsSkippedEntryRaw 悬空消费规则在场时任意经
// UpdateProfile 的写档（如启停旁证规则）不再物理丢弃跳过条目：原文（附件
// refScope 与模板 {集名} 字节）存续在档，再次载入仍跳过+告警（「载入失败
// 1 条」语义不变）。
func TestUpdateProfileWriteKeepsSkippedEntryRaw(t *testing.T) {
	mgr, store := danglingConsumerFixture(t)

	if _, err := mgr.ApplySetRuleEnabled("bk-other", false); err != nil {
		t.Fatalf("启停旁证规则（经 UpdateProfile 写档）失败: %v", err)
	}
	raw := store.values[RuleProfileKVKey]
	for _, want := range []string{`"bk-consumer"`, `"refScope"`, `{集名}`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("写档后跳过条目原文应保真存续（缺 %q）: %s", want, raw)
		}
	}
	profile, skips, _ := LoadProfileWithSkips(store)
	if len(profile.Rules) != 1 || profile.Rules[0].ID != "bk-other" || profile.Rules[0].Enabled {
		t.Fatalf("旁证规则启停应生效且不被阻塞: %+v", profile.Rules)
	}
	if len(skips) != 1 || skips[0].ID != "bk-consumer" {
		t.Fatalf("再次载入应仍跳过+告警（供给方仍缺席）: %+v", skips)
	}
}

// TestSkippedConsumerRevivesAfterProviderRebuild 剧本2 帧4 恢复路径：重建同 ID
// 供给方（saveRule=一次经 UpdateProfile 的写档）后，保真存续的消费规则原文
// 再次载入即复效——回到已载入集、错误态行清空。
func TestSkippedConsumerRevivesAfterProviderRebuild(t *testing.T) {
	mgr, _ := danglingConsumerFixture(t)

	rebuilt := scopeRuleJSON("bk-provider", []CondxRow{scopeDirRow("Artist[{作者}]", nil)}, nil)
	payload, err := json.Marshal(rebuilt)
	if err != nil {
		t.Fatalf("序列化重建供给方失败: %v", err)
	}
	view, err := mgr.ApplySaveRule(payload)
	if err != nil {
		t.Fatalf("重建供给方保存失败: %v", err)
	}
	loaded := map[string]bool{}
	for _, r := range view.Rules {
		loaded[r.ID] = true
	}
	if !loaded["bk-provider"] || !loaded["bk-consumer"] || !loaded["bk-other"] {
		t.Fatalf("供给方重建后消费规则应复效（三条均载入）: %+v", view.Rules)
	}
	if len(view.Skipped) != 0 {
		t.Fatalf("复效后错误态行应清空: %+v", view.Skipped)
	}
}

// TestSkippedEntryStillDeletableAfterProfileWrite 载入跳过中的条目在经历写档
// 往返后仍可按 ID 显式删除（错误态行的删除出口可用；原文层手术不被保真通道
// 破坏）。
func TestSkippedEntryStillDeletableAfterProfileWrite(t *testing.T) {
	mgr, store := danglingConsumerFixture(t)

	if _, err := mgr.ApplySetRuleEnabled("bk-other", true); err != nil {
		t.Fatalf("写档往返失败: %v", err)
	}
	view, err := mgr.ApplyDeleteRule("bk-consumer")
	if err != nil {
		t.Fatalf("删除载入跳过中的条目失败: %v", err)
	}
	if len(view.Skipped) != 0 {
		t.Fatalf("删除后错误态行应清空: %+v", view.Skipped)
	}
	if strings.Contains(store.values[RuleProfileKVKey], "bk-consumer") {
		t.Fatalf("删除后档案原文不应再含该条目: %s", store.values[RuleProfileKVKey])
	}
}

// TestRewritingSkippedEntryByIDReplacesRaw 以同 ID 重写跳过条目（用户修复该
// 条目的出口）：新条目入册、被顶替的旧原文退场（不双写），重载稳定。
func TestRewritingSkippedEntryByIDReplacesRaw(t *testing.T) {
	mgr, store := danglingConsumerFixture(t)

	fixed := scopeRuleJSON("bk-consumer", []CondxRow{scopeDirRow("{集名}", nil)}, nil)
	payload, err := json.Marshal(fixed)
	if err != nil {
		t.Fatalf("序列化重写条目失败: %v", err)
	}
	view, err := mgr.ApplySaveRule(payload)
	if err != nil {
		t.Fatalf("同 ID 重写保存失败: %v", err)
	}
	if len(view.Skipped) != 0 {
		t.Fatalf("重写后错误态行应清空: %+v", view.Skipped)
	}
	if got := strings.Count(store.values[RuleProfileKVKey], `"id":"bk-consumer"`); got != 1 {
		t.Fatalf("同 ID 重写应顶替旧原文（恰一条），实际 %d 条: %s", got, store.values[RuleProfileKVKey])
	}
	profile, skips, _ := LoadProfileWithSkips(store)
	if len(profile.Rules) != 2 || len(skips) != 0 {
		t.Fatalf("重载应稳定：两条已载入规则、零跳过: rules=%d skips=%d", len(profile.Rules), len(skips))
	}
}

// TestV4SkippedEntriesSurviveV5Normalize v4→v5 归一化保真：v4 档读入照常、
// 保存归一 v5，其内被跳过的 refs/dirLevel 条目在 v5 档中原文存续——后续每次
// 载入仍防御性跳过+告警；再经一轮写档保真幂等（不增殖）。
func TestV4SkippedEntriesSurviveV5Normalize(t *testing.T) {
	const v4Raw = `{"schemaVersion":4,"rules":[` +
		`{"id":"plain-a","description":"普通规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^a\\.jpg$","captures":[]},` +
		`"extractions":[],"synthesis":{}},` +
		`{"id":"combo-1","description":"组合规则","source":"manual","enabled":true,` +
		`"refs":["plain-a"],"conditions":[],` +
		`"extractions":[{"dimension":"author","from":"refCapture","refRule":"plain-a",` +
		`"captureName":"author"}],"synthesis":{}},` +
		`{"id":"anchored","description":"带层级锚","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"Artist[{作者}]"},` +
		`"attach":[{"kind":"dirLevel","minCount":1}]}}],` +
		`"extractions":[{"dimension":"author","from":"capture","captureName":"author","row":1}],"synthesis":{}}]}`
	store := newFakeSessionStore()
	store.values[RuleProfileKVKey] = v4Raw
	mgr, _ := newTestManager(t, store)

	if err := mgr.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error {
		p.Rules = append(p.Rules, testRule(t, "v5-new", true))
		return nil
	}); err != nil {
		t.Fatalf("经 UpdateProfile 写档失败: %v", err)
	}
	raw := store.values[RuleProfileKVKey]
	if !strings.Contains(raw, `"schemaVersion":5`) {
		t.Fatalf("保存后档案应归一 v5: %s", raw)
	}
	for _, want := range []string{`"refs"`, `"dirLevel"`, `"id":"combo-1"`, `"id":"anchored"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("v5 档应原文存续被跳过的 v4 条目（缺 %q）: %s", want, raw)
		}
	}
	profile, skips, _ := LoadProfileWithSkips(store)
	if len(profile.Rules) != 2 || profile.Rules[0].ID != "plain-a" || profile.Rules[1].ID != "v5-new" {
		t.Fatalf("归一后应载入普通条目+新写条目: %+v", profile.Rules)
	}
	if len(skips) != 2 || skips[0].ID != "combo-1" || skips[1].ID != "anchored" {
		t.Fatalf("v4 跳过条目在 v5 档中应仍防御性跳过+告警: %+v", skips)
	}
	// 第二轮写档：保真幂等（跳过条目原文不增殖）。
	if err := mgr.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error { return nil }); err != nil {
		t.Fatalf("第二轮写档失败: %v", err)
	}
	raw2 := store.values[RuleProfileKVKey]
	if got := strings.Count(raw2, `"id":"combo-1"`); got != 1 {
		t.Fatalf("refs 条目应恰一份（保真幂等），实际 %d 份: %s", got, raw2)
	}
	if got := strings.Count(raw2, `"id":"anchored"`); got != 1 {
		t.Fatalf("dirLevel 条目应恰一份（保真幂等），实际 %d 份: %s", got, raw2)
	}
}
