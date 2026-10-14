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
		{name: "缺少模式", ruleJSON: `{"id": "x", "description": "d", "source": "manual", "enabled": true}`, wantError: "缺少模式"},
		{name: "正则不可编译", ruleJSON: `{"id": "x", "source": "manual", "enabled": true, "pattern": {"kind": "regex", "target": "filename", "expr": "("}, "extractions": []}`, wantError: "模式非法"},
		{name: "条件引用未声明捕获组", ruleJSON: `{"id": "x", "source": "manual", "enabled": true, "pattern": {"kind": "regex", "target": "filename", "expr": "^a$", "captures": []}, "extractions": [{"dimension": "title", "from": "constant", "constant": "v", "when": {"kind": "capture", "captureName": "nope"}}]}`, wantError: "未声明的捕获组名"},
		{name: "提取动作引用未声明捕获组", ruleJSON: `{"id": "x", "source": "manual", "enabled": true, "pattern": {"kind": "regex", "target": "filename", "expr": "^【(?P<author>[^】]+)】", "captures": ["author"]}, "extractions": [{"dimension": "author", "from": "capture", "captureName": "author2"}]}`, wantError: "captures 数组内显式声明"},
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

// TestBuiltinDisabledAtMergePoint 退出标准3：内置规则加入禁用清单后合并求值不再
// 产出其声明（EffectiveRules 合并点 + 全管线求值双断言），移除后恢复。
func TestBuiltinDisabledAtMergePoint(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	files := []ScanFile{
		{Path: `E:\lib\混合\【小明】夏天的风.jpg`, FileName: "【小明】夏天的风.jpg", DirNames: []string{"混合"}, RelPath: "混合/【小明】夏天的风.jpg"},
	}

	// 基线：未禁用时内置括号规则产出 author/title 声明
	profile := loadAdminProfile(t, store)
	if decision := RunPipeline(files, EffectiveRules(profile), nil)[0]; len(decision.Declarations) != 2 {
		t.Fatalf("基线应产出 2 条声明（author/title），实际 %+v", decision.Declarations)
	}

	// 禁用：档案禁用清单承载，合并求值点应用——求值不再产出其声明
	if _, err := mgr.ApplySetBuiltinDisabled(builtinRuleIDBracketAuthorTitle, true); err != nil {
		t.Fatalf("禁用内置规则失败: %v", err)
	}
	profile = loadAdminProfile(t, store)
	if !reflect.DeepEqual(profile.DisabledBuiltinIDs, []string{builtinRuleIDBracketAuthorTitle}) {
		t.Fatalf("禁用清单应含该内置 ID，实际 %v", profile.DisabledBuiltinIDs)
	}
	effective := EffectiveRules(profile)
	for _, rule := range effective {
		if rule.ID == builtinRuleIDBracketAuthorTitle {
			t.Fatalf("禁用后的内置规则不应出现在合并求值序列")
		}
	}
	decision := RunPipeline(files, effective, nil)[0]
	if len(decision.Declarations) != 0 || len(decision.MatchedRuleIDs) != 0 {
		t.Fatalf("禁用后求值不应产出该内置规则的声明/命中，实际声明 %+v 命中 %v",
			decision.Declarations, decision.MatchedRuleIDs)
	}

	// 移除恢复：清单移出后声明恢复
	if _, err := mgr.ApplySetBuiltinDisabled(builtinRuleIDBracketAuthorTitle, false); err != nil {
		t.Fatalf("恢复内置规则失败: %v", err)
	}
	profile = loadAdminProfile(t, store)
	if len(profile.DisabledBuiltinIDs) != 0 {
		t.Fatalf("恢复后禁用清单应为空，实际 %v", profile.DisabledBuiltinIDs)
	}
	if decision := RunPipeline(files, EffectiveRules(profile), nil)[0]; len(decision.Declarations) != 2 {
		t.Fatalf("恢复后声明应回归 2 条，实际 %+v", decision.Declarations)
	}

	// 未知内置 ID 拒绝（防清单积入失效 ID）
	if _, err := mgr.ApplySetBuiltinDisabled("builtin-不存在", true); err == nil {
		t.Fatalf("未知内置 ID 应被拒绝")
	}
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

	// 会话管理器路径：内置禁用清单写同样经写前备份；ApplyRollbackProfile 回退后
	// 视图恢复前值
	sessionStore := newFakeSessionStore()
	mgr, _ := newTestManager(t, sessionStore)
	if _, err := mgr.ApplySaveRule(adminRuleJSON(t, "before-rollback", "回退前规则", true)); err != nil {
		t.Fatalf("写入回退前规则失败: %v", err)
	}
	if _, err := mgr.ApplySetBuiltinDisabled(builtinRuleIDDashAuthorTitle, true); err != nil {
		t.Fatalf("禁用内置规则失败: %v", err)
	}
	view, err := mgr.ApplyRollbackProfile()
	if err != nil {
		t.Fatalf("档案回退失败: %v", err)
	}
	if len(view.Rules) != 1 || view.Rules[0].ID != "before-rollback" {
		t.Fatalf("回退后视图应恢复禁用写之前的档案，实际 %d 条", len(view.Rules))
	}
	if len(view.DisabledBuiltinIDs) != 0 {
		t.Fatalf("回退后禁用清单应为空，实际 %v", view.DisabledBuiltinIDs)
	}
}

// TestRulesActionDispatch 前端事件协议的规则管理族分派：getRules / saveRule（含
// 非法条目错误回告）/ setRuleEnabled / setBuiltinDisabled / rollbackProfile 逐动作
// 经 HandleFrontendRequest 往返，应答按序号配对并携带规则管理视图。
func TestRulesActionDispatch(t *testing.T) {
	store := newFakeSessionStore()
	mgr, ctx := newTestManager(t, store)

	// getRules：空档案 + 内置表齐全 + 无备份
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 1, Action: workbenchActionGetRules}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.RulesView == nil || len(resp.RulesView.Rules) != 0 || len(resp.RulesView.Builtins) != len(BuiltinRules()) || resp.RulesView.BackupAvailable {
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

	// setBuiltinDisabled：禁用 + 备份在场（前序写已产生覆写备份）
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 5, Action: workbenchActionSetBuiltinDisabled, BuiltinID: builtinRuleIDBracketAuthorTitle, Disabled: true}))
	resp = ctx.lastResponse(t)
	if !resp.OK || len(resp.RulesView.DisabledBuiltinIDs) != 1 || !resp.RulesView.BackupAvailable {
		t.Fatalf("内置禁用应答不符: %+v", resp.RulesView)
	}

	// rollbackProfile：回退撤销的是紧邻的禁用写——dispatch-rule 仍为停用态（seq4）
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 6, Action: workbenchActionRollbackProfile}))
	resp = ctx.lastResponse(t)
	if !resp.OK || !resp.RolledBack || len(resp.RulesView.DisabledBuiltinIDs) != 0 || resp.RulesView.Rules[0].Enabled {
		t.Fatalf("档案回退应答不符: %+v", resp)
	}

	// deleteRule：删除后档案空
	mgr.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{Seq: 7, Action: workbenchActionDeleteRule, RuleID: "dispatch-rule"}))
	resp = ctx.lastResponse(t)
	if !resp.OK || len(resp.RulesView.Rules) != 0 {
		t.Fatalf("删除规则应答不符: %+v", resp.RulesView)
	}
}
