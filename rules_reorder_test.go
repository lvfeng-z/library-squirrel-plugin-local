// 规则全序重排（reorderRules）的单元测试：全排列重排保序往返（含原序幂等）、
// 全序清单多重集精确校验（漏 ID / 重 ID / 未知 ID 拒绝且档案与备份字节零变化）、
// 禁用行占位重排（存储位次与启用态无关）、写前单代备份随重排产生并可回退，
// 以及前端事件协议的 reorderRules 分派往返。
package main

import (
	"reflect"
	"strings"
	"testing"
)

// seedReorderRules 依序写入若干 manual 规则条目（启用态），失败即测试中止。
func seedReorderRules(t *testing.T, mgr *SessionManager, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := mgr.ApplySaveRule(adminRuleJSON(t, id, "重排测试规则 "+id, true)); err != nil {
			t.Fatalf("写入种子规则 %s 失败: %v", id, err)
		}
	}
}

// assertReorderViewOrder 断言规则管理视图的规则序等于 want。
func assertReorderViewOrder(t *testing.T, view *rulesAdminView, want []string) {
	t.Helper()
	got := make([]string, 0, len(view.Rules))
	for _, r := range view.Rules {
		got = append(got, r.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("视图规则序不符：want %v got %v", want, got)
	}
}

// assertReorderProfileOrder 断言档案存储序等于 want。
func assertReorderProfileOrder(t *testing.T, store *fakeSessionStore, want []string) {
	t.Helper()
	profile := loadAdminProfile(t, store)
	got := make([]string, 0, len(profile.Rules))
	for _, r := range profile.Rules {
		got = append(got, r.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("档案规则序不符：want %v got %v", want, got)
	}
}

// TestReorderRulesFullPermutationRoundTrip 全排列重排保序：任意排列一次请求达成、
// 视图与档案序一致跟随、条目内容零变化（仅位置搬运）、原序重排幂等成功。
func TestReorderRulesFullPermutationRoundTrip(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	seedReorderRules(t, mgr, "rule-a", "rule-b", "rule-c")

	// [a b c] → [c a b]：视图（应答载荷）与档案序均跟随新序
	view, err := mgr.ApplyReorderRules([]string{"rule-c", "rule-a", "rule-b"})
	if err != nil {
		t.Fatalf("全排列重排失败: %v", err)
	}
	assertReorderViewOrder(t, view, []string{"rule-c", "rule-a", "rule-b"})
	assertReorderProfileOrder(t, store, []string{"rule-c", "rule-a", "rule-b"})

	// 条目内容零变化：描述按 ID 对应不变（重排只搬运位置，不改写条目）
	profile := loadAdminProfile(t, store)
	for _, r := range profile.Rules {
		if want := "重排测试规则 " + r.ID; r.Description != want {
			t.Fatalf("规则 %s 的描述被改写：want %q got %q", r.ID, want, r.Description)
		}
	}

	// 排回原序；清单=现序（原序幂等）同样成功
	if _, err := mgr.ApplyReorderRules([]string{"rule-a", "rule-b", "rule-c"}); err != nil {
		t.Fatalf("排回原序失败: %v", err)
	}
	assertReorderProfileOrder(t, store, []string{"rule-a", "rule-b", "rule-c"})
	if _, err := mgr.ApplyReorderRules([]string{"rule-a", "rule-b", "rule-c"}); err != nil {
		t.Fatalf("原序幂等重排应成功: %v", err)
	}
	assertReorderProfileOrder(t, store, []string{"rule-a", "rule-b", "rule-c"})
}

// TestReorderRulesRejectsInconsistentLists 多重集精确校验的拒绝路径：漏 ID /
// 重 ID / 未知 ID / 空清单（漏全部）均报可读中文错误，且档案与写前备份键字节
// 零变化（校验失败在 mutate 内中止，不触达 SaveProfile——显式断言前后字节相等）。
func TestReorderRulesRejectsInconsistentLists(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	seedReorderRules(t, mgr, "rule-a", "rule-b", "rule-c")
	baseline := store.values[RuleProfileKVKey]
	baselineBackup := store.values[RuleProfileBackupKVKey]

	cases := []struct {
		name      string
		ids       []string
		wantError string
	}{
		{name: "漏 ID", ids: []string{"rule-a", "rule-b"}, wantError: "漏掉"},
		{name: "重 ID", ids: []string{"rule-a", "rule-a", "rule-b"}, wantError: "重复出现"},
		{name: "未知 ID", ids: []string{"rule-a", "rule-b", "rule-x"}, wantError: "不存在"},
		{name: "空清单（漏全部）", ids: nil, wantError: "漏掉"},
		{name: "未知 ID 且超长", ids: []string{"rule-a", "rule-b", "rule-c", "rule-x"}, wantError: "不存在"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view, err := mgr.ApplyReorderRules(tc.ids)
			if err == nil {
				t.Fatalf("不一致清单应被拒绝，实际成功（规则数 %d）", len(view.Rules))
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("错误应含 %q，实际: %v", tc.wantError, err)
			}
			if store.values[RuleProfileKVKey] != baseline {
				t.Fatalf("拒绝后档案字节不应变化")
			}
			if store.values[RuleProfileBackupKVKey] != baselineBackup {
				t.Fatalf("拒绝后写前备份键字节不应变化")
			}
		})
	}
	assertReorderProfileOrder(t, store, []string{"rule-a", "rule-b", "rule-c"})
}

// TestReorderRulesDisabledRowKeepsSlot 禁用行占位重排：禁用行照样占存储位次、可
// 被移动到任意位置；重排不改启用态；求值序列按新存储序过滤启用行（既有语义）。
func TestReorderRulesDisabledRowKeepsSlot(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	seedReorderRules(t, mgr, "rule-a", "rule-b", "rule-c")
	if _, err := mgr.ApplySetRuleEnabled("rule-b", false); err != nil {
		t.Fatalf("停用中间行失败: %v", err)
	}

	// 禁用行（rule-b）移到首位：存储序 [b c a]，启用态不变
	view, err := mgr.ApplyReorderRules([]string{"rule-b", "rule-c", "rule-a"})
	if err != nil {
		t.Fatalf("禁用行占位重排失败: %v", err)
	}
	assertReorderViewOrder(t, view, []string{"rule-b", "rule-c", "rule-a"})
	assertReorderProfileOrder(t, store, []string{"rule-b", "rule-c", "rule-a"})

	profile := loadAdminProfile(t, store)
	enabledByID := make(map[string]bool, len(profile.Rules))
	for _, r := range profile.Rules {
		enabledByID[r.ID] = r.Enabled
	}
	want := map[string]bool{"rule-a": true, "rule-b": false, "rule-c": true}
	if !reflect.DeepEqual(enabledByID, want) {
		t.Fatalf("重排不应改变启用态: %v", enabledByID)
	}

	// 求值序列=新存储序过滤启用行（禁用行占位但被跳过——既有合并语义零改动）
	effective := EffectiveRules(profile)
	order := make([]string, 0, len(effective))
	for _, r := range effective {
		order = append(order, r.ID)
	}
	if !reflect.DeepEqual(order, []string{"rule-c", "rule-a"}) {
		t.Fatalf("求值序列应按新存储序过滤启用行: %v", order)
	}
}

// TestReorderRulesPreOverwriteBackup 写前备份生效：重排落档前现行档案原文进单代
// 备份键；回退一代恢复重排前顺序（排序复用既有写入链白得备份与回退）。
func TestReorderRulesPreOverwriteBackup(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	seedReorderRules(t, mgr, "rule-a", "rule-b")

	preReorder := store.values[RuleProfileKVKey]
	if _, err := mgr.ApplyReorderRules([]string{"rule-b", "rule-a"}); err != nil {
		t.Fatalf("重排失败: %v", err)
	}
	if store.values[RuleProfileBackupKVKey] != preReorder {
		t.Fatalf("写前备份应包含重排前的档案原文")
	}
	assertReorderProfileOrder(t, store, []string{"rule-b", "rule-a"})

	view, err := mgr.ApplyRollbackProfile()
	if err != nil {
		t.Fatalf("回退一代失败: %v", err)
	}
	assertReorderViewOrder(t, view, []string{"rule-a", "rule-b"})
}

// TestReorderRulesActionDispatch 前端事件协议分派：reorderRules 携 ruleIds 载荷
// 往返——合法全序清单应答 OK 且携带全量刷新的规则管理视图（新序）；不一致清单
// 错误回告且不携带视图（前端据此回滚本地换位并 toast）。
func TestReorderRulesActionDispatch(t *testing.T) {
	store := newFakeSessionStore()
	mgr, ctx := newTestManager(t, store)

	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 1, Action: workbenchActionSaveRule, Rule: adminRuleJSON(t, "dispatch-a", "分派重排规则A", true)}))
	if resp := ctx.lastResponse(t); !resp.OK {
		t.Fatalf("保存规则 A 失败: %+v", resp)
	}
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 2, Action: workbenchActionSaveRule, Rule: adminRuleJSON(t, "dispatch-b", "分派重排规则B", true)}))
	if resp := ctx.lastResponse(t); !resp.OK {
		t.Fatalf("保存规则 B 失败: %+v", resp)
	}

	// 合法全序清单：应答携带新序视图
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 3, Action: workbenchActionReorderRules, RuleIDs: []string{"dispatch-b", "dispatch-a"}}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.RulesView == nil {
		t.Fatalf("reorderRules 合法清单应答不符: %+v", resp)
	}
	assertReorderViewOrder(t, resp.RulesView, []string{"dispatch-b", "dispatch-a"})
	assertReorderProfileOrder(t, store, []string{"dispatch-b", "dispatch-a"})

	// 不一致清单（漏 ID）：错误回告、不携带视图、档案字节零改动
	before := store.values[RuleProfileKVKey]
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 4, Action: workbenchActionReorderRules, RuleIDs: []string{"dispatch-a"}}))
	resp = ctx.lastResponse(t)
	if resp.OK || resp.RulesView != nil || !strings.Contains(resp.Error, "漏掉") {
		t.Fatalf("reorderRules 不一致清单应错误回告且不带视图: %+v", resp)
	}
	if store.values[RuleProfileKVKey] != before {
		t.Fatalf("拒绝后档案字节不应变化")
	}
}
