// 规则管理面的单元测试：条目保存/启停/删除的档案往返（改→存→读回一致）、非法
// 条目拒绝并报可读错误且不落盘、内置禁用清单在合并求值点的应用（含禁用后恢复）、
// 写前备份与回退一代（含换位语义），以及前端事件协议的规则管理族分派。
package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// adminRuleJSON 构造一条 manual 规则条目的 JSON（ruleEntryJSON 形态）：模式为
// 带命名捕获组的文件名正则，提取动作含捕获组取值与条件常量，合成策略含维度策略
// 与角色绑定——覆盖编辑面全部字段族。
func adminRuleJSON(t *testing.T, id, description string, enabled bool) []byte {
	t.Helper()
	return []byte(`{
		"id": "` + id + `",
		"description": "` + description + `",
		"source": "manual",
		"enabled": ` + jsonBool(enabled) + `,
		"pattern": {"kind": "regex", "target": "filename", "expr": "^【(?P<author>[^】]+)】", "captures": ["author"]},
		"extractions": [
			{"dimension": "author", "from": "capture", "captureName": "author", "constant": "", "roleName": "原画"},
			{"dimension": "title", "from": "constant", "constant": "无题", "when": {"kind": "predicate", "source": "dir", "dimension": "title"}}
		],
		"synthesis": {
			"dimPolicies": [{"dimension": "title", "strategy": "highTrust"}],
			"roles": [{"source": "filename", "roleName": "原画"}]
		}
	}`)
}

// jsonBool 布尔值的 JSON 文本。
func jsonBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// loadAdminProfile 从存储载入档案（失败即测试中止）。
func loadAdminProfile(t *testing.T, store *fakeSessionStore) *RuleProfile {
	t.Helper()
	profile, errs := LoadProfile(store)
	if profile == nil {
		t.Fatalf("档案载入失败: %v", errs)
	}
	if len(errs) != 0 {
		t.Fatalf("档案载入不应告警: %v", errs)
	}
	return profile
}

// TestRuleAdminSaveToggleDeleteRoundTrip 退出标准1：新建→读回一致→编辑原位替换
// →启停→删除，全程档案往返正确（经会话管理器单点串行）。
func TestRuleAdminSaveToggleDeleteRoundTrip(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	// 新建（ID 缺省由后端按 manual 前缀生成）
	view, err := mgr.ApplySaveRule(adminRuleJSON(t, "", "手写的括号作者规则", true))
	if err != nil {
		t.Fatalf("新建规则失败: %v", err)
	}
	if len(view.Rules) != 1 {
		t.Fatalf("新建后视图应有 1 条规则，实际 %d 条", len(view.Rules))
	}
	newID := view.Rules[0].ID
	if !strings.HasPrefix(newID, "manual-") {
		t.Fatalf("缺省 ID 应按 manual 前缀生成，实际 %q", newID)
	}

	// 改→存→读回一致：提交 JSON 与档案读回条目（经编辑形态往返）深度相等
	submitted := adminRuleJSON(t, newID, "手写的括号作者规则", true)
	var want ruleEntryJSON
	if err := json.Unmarshal(submitted, &want); err != nil {
		t.Fatalf("解析提交规则失败: %v", err)
	}
	profile := loadAdminProfile(t, store)
	if len(profile.Rules) != 1 {
		t.Fatalf("档案应有 1 条规则，实际 %d 条", len(profile.Rules))
	}
	got, err := ruleToJSON(profile.Rules[0])
	if err != nil {
		t.Fatalf("读回规则转编辑形态失败: %v", err)
	}
	if !reflect.DeepEqual(got, &want) {
		t.Fatalf("读回规则与提交不一致\n读回: %s\n提交: %s", mustMarshal(t, got), mustMarshal(t, &want))
	}

	// 追加第二条后编辑首条：原位替换（顺序保持、条数不变）
	if _, err := mgr.ApplySaveRule(adminRuleJSON(t, "second-rule", "第二条规则", true)); err != nil {
		t.Fatalf("追加第二条规则失败: %v", err)
	}
	edited := adminRuleJSON(t, newID, "手写的括号作者规则（改）", true)
	if _, err := mgr.ApplySaveRule(edited); err != nil {
		t.Fatalf("编辑保存失败: %v", err)
	}
	profile = loadAdminProfile(t, store)
	if len(profile.Rules) != 2 || profile.Rules[0].ID != newID || profile.Rules[0].Description != "手写的括号作者规则（改）" {
		t.Fatalf("编辑应原位替换：实际条目 %v", []string{profile.Rules[0].ID, profile.Rules[0].Description})
	}

	// 启停：停用后读回 Enabled=false
	if _, err := mgr.ApplySetRuleEnabled(newID, false); err != nil {
		t.Fatalf("停用规则失败: %v", err)
	}
	profile = loadAdminProfile(t, store)
	if profile.Rules[0].Enabled {
		t.Fatalf("停用后读回仍为启用")
	}

	// 删除：条目消失；再删同 ID 报错
	if _, err := mgr.ApplyDeleteRule(newID); err != nil {
		t.Fatalf("删除规则失败: %v", err)
	}
	profile = loadAdminProfile(t, store)
	if len(profile.Rules) != 1 || profile.Rules[0].ID != "second-rule" {
		t.Fatalf("删除后档案应只剩 second-rule，实际 %d 条", len(profile.Rules))
	}
	if _, err := mgr.ApplyDeleteRule(newID); err == nil {
		t.Fatalf("删除不存在的规则应报错")
	}
}

// mustMarshal 序列化任意值为 JSON 文本（差异展示用）。
func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	return string(data)
}

// TestRuleAdminRejectsInvalidEntries 退出标准2：非法 JSON 条目（语法坏/缺模式/
// 坏正则/条件引用未声明捕获组/提取动作引用未声明捕获组/来源非法）单条即时报
// 可读中文错误，且档案字节不变（不落盘坏条目、不产生备份）。
func TestRuleAdminRejectsInvalidEntries(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	if _, err := mgr.ApplySaveRule(adminRuleJSON(t, "seed", "基线规则", true)); err != nil {
		t.Fatalf("写入基线规则失败: %v", err)
	}
	baseline := store.values[RuleProfileKVKey]

	cases := []struct {
		name      string
		ruleJSON  string
		wantError string
	}{
		{name: "JSON 语法坏", ruleJSON: `{"id": "x",`, wantError: "无法解析"},
		{name: "缺少条件行", ruleJSON: `{"id": "x", "description": "d", "source": "manual", "enabled": true}`, wantError: "缺少条件行"},
		{name: "正则不可编译", ruleJSON: `{"id": "x", "source": "manual", "enabled": true, "pattern": {"kind": "regex", "target": "filename", "expr": "("}, "extractions": []}`, wantError: "模式非法"},
		{name: "条件引用未声明捕获组", ruleJSON: `{"id": "x", "source": "manual", "enabled": true, "pattern": {"kind": "regex", "target": "filename", "expr": "^a$", "captures": []}, "extractions": [{"dimension": "title", "from": "constant", "constant": "v", "when": {"kind": "capture", "captureName": "nope"}}]}`, wantError: "未声明的捕获组名"},
		{name: "提取动作引用未声明捕获组", ruleJSON: `{"id": "x", "source": "manual", "enabled": true, "pattern": {"kind": "regex", "target": "filename", "expr": "^【(?P<author>[^】]+)】", "captures": ["author"]}, "extractions": [{"dimension": "author", "from": "capture", "captureName": "author2"}]}`, wantError: "未声明的捕获组名"},
		{name: "来源为内置", ruleJSON: `{"id": "x", "source": "builtin", "enabled": true, "pattern": {"kind": "regex", "target": "filename", "expr": "^a$"}, "extractions": []}`, wantError: "内置"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			view, err := mgr.ApplySaveRule([]byte(tc.ruleJSON))
			if err == nil {
				t.Fatalf("非法条目应被拒绝，实际保存成功（规则数 %d）", len(view.Rules))
			}
			if !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("错误应含 %q，实际: %v", tc.wantError, err)
			}
			if store.values[RuleProfileKVKey] != baseline {
				t.Fatalf("拒绝后档案字节不应变化（坏条目落盘）")
			}
		})
	}
}

// TestBuiltinRetiredAtMergePoint 内置退休断言（v4）：ApplySetBuiltinDisabled 恒
// 回退休错误（不落库）；EffectiveRules 收敛为纯用户档案（nil=空序列）。
func TestBuiltinRetiredAtMergePoint(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	if view, err := mgr.ApplySetBuiltinDisabled("builtin-bracket-author-title", true); err == nil || view != nil {
		t.Fatalf("内置启停应回退休错误，实际 view=%v err=%v", view, err)
	}
	if !strings.Contains(mustErrText(t, mgr, "builtin-bracket-author-title"), "已退休") {
		t.Fatal("退休错误应含「已退休」")
	}
	profile := loadAdminProfile(t, store)
	if len(profile.Rules) != 0 {
		t.Fatalf("退休调用不应落库，实际 %d 条", len(profile.Rules))
	}
	if rules := EffectiveRules(nil); len(rules) != 0 {
		t.Fatalf("无档案时合并求值序列应为空，实际 %d 条", len(rules))
	}
}

// mustErrText 内置启停错误文本（nil 错误即测试失败）。
func mustErrText(t *testing.T, mgr *SessionManager, id string) string {
	t.Helper()
	_, err := mgr.ApplySetBuiltinDisabled(id, true)
	if err == nil {
		t.Fatal("内置启停应恒回错误")
	}
	return err.Error()
}

// TestProfileBackupAndRollback 退出标准4：改→备份存在→回退→档案恢复前值；回退
// 换位（再回退撤销回退）；无备份时回退报错；禁用清单写同样产生写前备份。
func TestProfileBackupAndRollback(t *testing.T) {
	// 存储层直连：改→备份存在→回退→恢复前值
	store := newFakeProfileStore()
	first := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{testRule(t, "gen-one", true)}}
	if err := SaveProfile(store, first); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}
	if ProfileBackupExists(store) {
		t.Fatalf("首次写入（无现行档案）不应产生备份")
	}
	firstJSON := store.values[RuleProfileKVKey]
	second := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{testRule(t, "gen-two", true)}}
	if err := SaveProfile(store, second); err != nil {
		t.Fatalf("第二次保存失败: %v", err)
	}
	if !ProfileBackupExists(store) {
		t.Fatalf("覆写后备份键应存在")
	}
	if store.values[RuleProfileBackupKVKey] != firstJSON {
		t.Fatalf("备份内容应为覆写前的档案原文")
	}
	if err := RollbackProfile(store); err != nil {
		t.Fatalf("回退失败: %v", err)
	}
	if store.values[RuleProfileKVKey] != firstJSON {
		t.Fatalf("回退后档案应恢复前值")
	}
	// 回退换位：现行（第二次）档案换入备份，可再回退一次撤销回退
	if err := RollbackProfile(store); err != nil {
		t.Fatalf("二次回退（换位撤销）失败: %v", err)
	}
	restored, errs := LoadProfile(store)
	if restored == nil || len(errs) != 0 || len(restored.Rules) != 1 || restored.Rules[0].ID != "gen-two" {
		t.Fatalf("二次回退后档案应为第二次保存的内容，实际 %v（告警 %v）", restored, errs)
	}

	// 无备份时回退报错
	empty := newFakeProfileStore()
	if err := RollbackProfile(empty); err == nil || !strings.Contains(err.Error(), "无可回退") {
		t.Fatalf("无备份时回退应报可读错误，实际: %v", err)
	}

	// 会话管理器路径：条目写同样经写前备份；ApplyRollbackProfile 回退后
	// 视图恢复前值
	sessionStore := newFakeSessionStore()
	mgr, _ := newTestManager(t, sessionStore)
	if _, err := mgr.ApplySaveRule(adminRuleJSON(t, "before-rollback", "回退前规则", true)); err != nil {
		t.Fatalf("写入回退前规则失败: %v", err)
	}
	if _, err := mgr.ApplySaveRule(adminRuleJSON(t, "after-first", "禁用位之后的规则", true)); err != nil {
		t.Fatalf("写入第二条规则失败: %v", err)
	}
	view, err := mgr.ApplyRollbackProfile()
	if err != nil {
		t.Fatalf("档案回退失败: %v", err)
	}
	if len(view.Rules) != 1 || view.Rules[0].ID != "before-rollback" {
		t.Fatalf("回退后视图应恢复第二条写之前的档案，实际 %d 条", len(view.Rules))
	}
}

// TestRulesActionDispatch 前端事件协议的规则管理族分派：getRules / saveRule（含
// 非法条目错误回告）/ setRuleEnabled / setBuiltinDisabled / rollbackProfile 逐动作
// 经 HandleFrontendRequest 往返，应答按序号配对并携带规则管理视图。
// TestApplySaveRuleCondxCaptureRoundTrip v4 条件行条目保存（live-test 批次一
// 抓到的回归：编辑面双保险校验曾误用 v3 pattern 声明集，v4 条目的行号前缀
// 捕获引用被拒）。锚定：conditions 形态条目可存、capture 提取引用英文组名+
// 行号、档案往返 conditions 保真。
func TestApplySaveRuleCondxCaptureRoundTrip(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	ruleJSON := []byte(`{
		"id": "", "description": "T3 散页成套", "source": "manual", "enabled": true,
		"conditions": [
			{"condition": {"target": "seq", "format": {"mode": "template", "tpl": "{键}_{序号}"},
				"attach": [{"kind": "seqMinLength", "minCount": 2}, {"kind": "seqMemberType", "types": ["image"]}]}}
		],
		"extractions": [
			{"dimension": "collection", "from": "capture", "captureName": "key", "row": 1}
		],
		"synthesis": {"dimPolicies": [], "roles": []}
	}`)
	view, err := mgr.ApplySaveRule(ruleJSON)
	if err != nil {
		t.Fatalf("v4 条件行条目保存失败: %v", err)
	}
	if len(view.Rules) != 1 {
		t.Fatalf("保存后视图应有 1 条规则，实际 %d 条", len(view.Rules))
	}
	id := view.Rules[0].ID
	if !strings.HasPrefix(id, "manual-") {
		t.Fatalf("缺省 ID 应按 manual 前缀生成，实际 %q", id)
	}

	// 档案往返：conditions 原样保真（含 attach/badges）
	profile := loadAdminProfile(t, store)
	if len(profile.Rules) != 1 {
		t.Fatalf("档案应有 1 条规则，实际 %d 条", len(profile.Rules))
	}
	entry := profile.Rules[0]
	if entry.Conditions == nil || len(entry.Conditions) != 1 || entry.Conditions[0].Condition == nil {
		t.Fatalf("conditions 应保真往返，实际 %+v", entry.Conditions)
	}
	cond := entry.Conditions[0].Condition
	if cond.Target != CondxTargetSeq || cond.Format.Tpl != "{键}_{序号}" || len(cond.Attach) != 2 {
		t.Fatalf("条件行内容不符: %+v", cond)
	}

	// 未声明组引用仍被拒（行号前缀口径）：第 1 行无 {作者} 却引用 author
	bad := []byte(`{
		"id": "bad-ref", "description": "错行引用", "source": "manual", "enabled": true,
		"conditions": [
			{"condition": {"target": "seq", "format": {"mode": "template", "tpl": "{键}_{序号}"}}}
		],
		"extractions": [
			{"dimension": "author", "from": "capture", "captureName": "author", "row": 1}
		],
		"synthesis": {"dimPolicies": [], "roles": []}
	}`)
	if _, err := mgr.ApplySaveRule(bad); err == nil || !strings.Contains(err.Error(), "未声明") {
		t.Fatalf("未声明组引用应被构造期拒绝，实际 err=%v", err)
	}
}

func TestRulesActionDispatch(t *testing.T) {
	store := newFakeSessionStore()
	mgr, ctx := newTestManager(t, store)

	// getRules：空档案 + 例子模板表在场 + 无备份
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 1, Action: workbenchActionGetRules}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.RulesView == nil || len(resp.RulesView.Rules) != 0 || len(resp.RulesView.Examples) != len(exampleTemplates()) || resp.RulesView.BackupAvailable {
		t.Fatalf("getRules 空档案应答不符: %+v", resp.RulesView)
	}

	// saveRule：非法条目回告可读错误（载荷经事件协议传输须为合法 JSON，语法级
	// 坏字节由直连单测覆盖，此处用类型错形态）
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 2, Action: workbenchActionSaveRule, Rule: json.RawMessage(`"不是规则对象"`)}))
	resp = ctx.lastResponse(t)
	if resp.OK || !strings.Contains(resp.Error, "无法解析") {
		t.Fatalf("非法规则保存应回告解析错误: %+v", resp)
	}

	// saveRule：合法条目入档
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 3, Action: workbenchActionSaveRule, Rule: adminRuleJSON(t, "dispatch-rule", "分派测试规则", true)}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.RulesView == nil || len(resp.RulesView.Rules) != 1 || resp.RulesView.Rules[0].ID != "dispatch-rule" {
		t.Fatalf("合法规则保存应答不符: %+v", resp.RulesView)
	}

	// setRuleEnabled：停用
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 4, Action: workbenchActionSetRuleEnabled, RuleID: "dispatch-rule", Enabled: false}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.RulesView.Rules[0].Enabled {
		t.Fatalf("停用应答不符: %+v", resp.RulesView)
	}

	// setBuiltinDisabled：内置退休——恒回错误（不落库、应答不带视图）
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 5, Action: workbenchActionSetBuiltinDisabled, BuiltinID: "builtin-bracket-author-title", Disabled: true}))
	resp = ctx.lastResponse(t)
	if resp.OK || !strings.Contains(resp.Error, "已退休") || resp.RulesView != nil {
		t.Fatalf("内置启停应回退休错误且不带视图: %+v", resp)
	}

	// rollbackProfile：回退撤销的是 seq4 的停用写——dispatch-rule 恢复启用
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 6, Action: workbenchActionRollbackProfile}))
	resp = ctx.lastResponse(t)
	if !resp.OK || !resp.RolledBack || len(resp.RulesView.Rules) != 1 || !resp.RulesView.Rules[0].Enabled {
		t.Fatalf("档案回退应答不符: %+v", resp)
	}

	// deleteRule：删除后档案空
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 7, Action: workbenchActionDeleteRule, RuleID: "dispatch-rule"}))
	resp = ctx.lastResponse(t)
	if !resp.OK || len(resp.RulesView.Rules) != 0 {
		t.Fatalf("删除规则应答不符: %+v", resp.RulesView)
	}
}
