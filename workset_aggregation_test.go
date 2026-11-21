// 作品集聚合（产出四）内核测试：档案 schema 的载入校验（无集名产出拒绝、组键
// 绑定形态校验、往返保真）、汇流后求值（同键成组/无分组键孤文件不入集/档案序
// 先到先得/既有聚合归属优先）、集归属事实入识别快照（v4 字段落档与双向兼容）。
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// —— 测试构造件 ——

// workSetAggProfileRules 把内存规则经档案 JSON 往返载入（走生产载入校验管线，
// 返回启用规则序列；载入拒绝即测试中止）。
func workSetAggProfileRules(t *testing.T, rules ...RuleEntry) []RuleEntry {
	t.Helper()
	data, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: rules})
	if err != nil {
		t.Fatalf("序列化测试档案失败: %v", err)
	}
	profile, errs := ParseProfile(data)
	if profile == nil {
		t.Fatalf("测试档案载入失败: %v", errs)
	}
	if len(errs) != 0 {
		t.Fatalf("测试档案载入不应告警: %v", errs)
	}
	if len(profile.Rules) != len(rules) {
		t.Fatalf("测试档案应载入 %d 条规则，实际 %d 条: %v", len(rules), len(profile.Rules), errs)
	}
	return EffectiveRules(profile)
}

// workSetAggCaptureRule 按集名捕获归集的产出四规则：条件 = 文件名模板
// `{集名}_{序号}`（行 1 同时声明集名与序号捕获），提取 = 集名捕获 → 作品集
// 维度；归集依据 = 行捕获复合（行 1 的集名捕获）。
func workSetAggCaptureRule() RuleEntry {
	return RuleEntry{
		ID:          "ws-capture",
		Description: "测试：按集名捕获归集",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{{
			Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{集名}_{序号}")},
		}},
		Extractions: []ExtractionAction{{
			Dimension: DimCollection, From: FromCapture, CaptureName: condxVocab["集名"], Row: 1,
		}},
		WorkSetAggregation: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode:     GroupingModeIdentity,
			Identity: []IdentityPart{{Row: 1, Capture: condxVocab["集名"]}},
		}},
	}
}

// workSetAggExtRule 按扩展名归集的产出四规则（集名取常量）：条件 = 文件名
// 任意名 + 扩展名 `.png`，提取 = 常量集名 → 作品集维度；归集依据 = 所在目录。
func workSetAggExtRule(id, setName string) RuleEntry {
	row := CondxRow{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{集名}")}}
	row.Condition.Attach = []CondxAttach{{Kind: CondxAttachFileExt, Value: ".png"}}
	return RuleEntry{
		ID:          id,
		Description: "测试：按所在目录归集 " + id,
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions:  []CondxRow{row},
		Extractions: []ExtractionAction{{
			Dimension: DimCollection, From: FromConstant, Constant: setName,
		}},
		WorkSetAggregation: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeDirectory,
		}},
	}
}

// workSetAggRuleOf 取预置聚合种子（0=视频+字幕，2=一图一文本→article）。
func workSetAggRuleOf(t *testing.T, index int) RuleEntry {
	t.Helper()
	seeds := PresetAggregateRules()
	if index < 0 || index >= len(seeds) {
		t.Fatalf("预置聚合种子下标越界: %d", index)
	}
	return seeds[index]
}

// —— 求值：同键成组 / 孤文件不入集 ——

// TestWorkSetAggSameKeyGrouping 同键成组正锚：行捕获复合的组键相等者成组——
// 同集名捕获值的文件获得同一组键/集名/规则 ID，各自独立成作品（产出四不并
// 作品）；不同捕获值分属不同集。
func TestWorkSetAggSameKeyGrouping(t *testing.T) {
	rules := workSetAggProfileRules(t, workSetAggCaptureRule())
	files := aggScanFiles(
		"组/甲_01.png", "组/甲_02.png", "组/甲_03.png",
		"组/乙_01.png",
	)
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, nil)

	want := map[string]struct{ key, name string }{
		"组/甲_01.png": {"甲", "甲"},
		"组/甲_02.png": {"甲", "甲"},
		"组/甲_03.png": {"甲", "甲"},
		"组/乙_01.png": {"乙", "乙"},
	}
	for rel, w := range want {
		c := aggConclusionOf(t, conclusions, rel)
		if c.SetGroupKey != w.key || c.SetName != w.name || c.SetRuleID != "ws-capture" {
			t.Fatalf("%s 的集归属应为组键=%q 集名=%q 规则=ws-capture，实际 key=%q name=%q rule=%q",
				rel, w.key, w.name, c.SetGroupKey, c.SetName, c.SetRuleID)
		}
		if c.AggWorkKey != "" || c.AggTrackRole != "" || c.AggCompanion {
			t.Fatalf("%s 应各自独立成作品（产出四不并作品）: key=%q role=%q companion=%v",
				rel, c.AggWorkKey, c.AggTrackRole, c.AggCompanion)
		}
	}
}

// TestWorkSetAggOrphanNotInSet 无分组键孤文件不入集锚：命中规则但任一组键部分
// 取不到值的文件（此处目录捕获行不中）不获集归属、保持独立作品；组键两部分皆
// 可得的文件正常同键成组。
func TestWorkSetAggOrphanNotInSet(t *testing.T) {
	rule := RuleEntry{
		ID:          "ws-composite-key",
		Description: "测试：行捕获+目录捕获复合组键",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{集名}_{序号}")}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("作者{作者}")}},
		},
		Extractions: []ExtractionAction{{
			Dimension: DimCollection, From: FromCapture, CaptureName: condxVocab["集名"], Row: 1,
		}},
		WorkSetAggregation: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeIdentity,
			Identity: []IdentityPart{
				{Row: 1, Capture: condxVocab["集名"]},
				{Row: 2, Capture: condxVocab["作者"]},
			},
		}},
	}
	rules := workSetAggProfileRules(t, rule)
	files := aggScanFiles(
		"作者甲/甲_01.png", "作者甲/甲_02.png",
		"杂项/甲_03.png", // 命中文件行但目录行不中：复合组键缺目录部分
	)
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, nil)

	wantKey := "甲" + "\x00" + "甲"
	for _, rel := range []string{"作者甲/甲_01.png", "作者甲/甲_02.png"} {
		c := aggConclusionOf(t, conclusions, rel)
		if c.SetGroupKey != wantKey || c.SetName != "甲" || c.SetRuleID != "ws-composite-key" {
			t.Fatalf("%s 的复合组键应可得并成组（key=%q name=%q rule=%q），实际 key=%q name=%q rule=%q",
				rel, wantKey, "甲", "ws-composite-key", c.SetGroupKey, c.SetName, c.SetRuleID)
		}
	}
	orphan := aggConclusionOf(t, conclusions, "杂项/甲_03.png")
	if orphan.SetGroupKey != "" || orphan.SetName != "" || orphan.SetRuleID != "" {
		t.Fatalf("分组键不可得的孤文件不应入集: key=%q name=%q rule=%q",
			orphan.SetGroupKey, orphan.SetName, orphan.SetRuleID)
	}
	if orphan.AggWorkKey != "" || orphan.AggCompanion {
		t.Fatalf("孤文件应保持独立作品: key=%q companion=%v", orphan.AggWorkKey, orphan.AggCompanion)
	}
}

// —— 求值：档案序先到先得 / 既有聚合归属优先 ——

// TestWorkSetAggArchiveOrderFirstCome 档案序先到先得锚：两条产出四规则命中同一
// 文件时，按规则在档案中的声明顺序先到先得——在先规则落定后在后规则不再改判
// （反转声明序即反转归属）；未被任何规则命中的文件不入集。
func TestWorkSetAggArchiveOrderFirstCome(t *testing.T) {
	ruleA := workSetAggExtRule("ws-order-a", "集甲")
	ruleB := workSetAggExtRule("ws-order-b", "集乙")
	files := aggScanFiles("同目录/图_01.png", "同目录/图_02.png", "同目录/说明.txt")

	cases := []struct {
		name     string
		order    []RuleEntry
		wantRule string
		wantSet  string
	}{
		{"先声明的规则先归属", []RuleEntry{ruleA, ruleB}, "ws-order-a", "集甲"},
		{"反转档案序反转归属", []RuleEntry{ruleB, ruleA}, "ws-order-b", "集乙"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := workSetAggProfileRules(t, tc.order...)
			conclusions := MergedRecognition(files, rules, nil, nil)
			runAggregationOnConclusions(conclusions, files, files, rules, nil, nil)
			wantKey := dirGroupKey([]string{"同目录"})
			for _, rel := range []string{"同目录/图_01.png", "同目录/图_02.png"} {
				c := aggConclusionOf(t, conclusions, rel)
				if c.SetRuleID != tc.wantRule || c.SetName != tc.wantSet || c.SetGroupKey != wantKey {
					t.Fatalf("%s 的集归属应为规则=%q 集名=%q 组键=%q，实际 rule=%q name=%q key=%q",
						rel, tc.wantRule, tc.wantSet, wantKey, c.SetRuleID, c.SetName, c.SetGroupKey)
				}
			}
			// 未命中产出四条件行的文件不获集归属。
			if stray := aggConclusionOf(t, conclusions, "同目录/说明.txt"); stray.SetGroupKey != "" || stray.SetName != "" {
				t.Fatalf("未命中规则的文件不应入集: key=%q name=%q", stray.SetGroupKey, stray.SetName)
			}
		})
	}
}

// TestWorkSetAggAggregationTakesPriority 既有聚合归属优先锚（并用②：聚合段先
// 归属、产出四按剩余文件归集）：产出四规则声明在聚合规则之前也不夺已被聚合
// 收编的成员（相位序=聚合段在先）；未被聚合收编的文件由产出四正常归集。
// 图像行仅命中图片但组内必含文档轨缺失=聚合组不成立（宁缺勿错），该文件仍可
// 被产出四归集。
func TestWorkSetAggAggregationTakesPriority(t *testing.T) {
	wsRule := workSetAggExtRule("ws-png", "集甲")
	aggRule := workSetAggRuleOf(t, 2)                   // 一图一文本→article（文档必含轨）
	rules := workSetAggProfileRules(t, wsRule, aggRule) // 产出四规则在前：聚合段仍先归属
	files := aggScanFiles("混合/a.txt", "混合/a.png", "混合/b.png")
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex())

	lead := aggConclusionOf(t, conclusions, "混合/a.txt")
	if lead.AggWorkKey != "f:混合/a.txt" {
		t.Fatalf("一图一文本应聚合为一件作品（主导=文档行文件）: %q", lead.AggWorkKey)
	}
	if lead.SetGroupKey != "" || lead.SetName != "" {
		t.Fatalf("已被聚合收编的成员不应入集: key=%q name=%q", lead.SetGroupKey, lead.SetName)
	}
	img := aggConclusionOf(t, conclusions, "混合/a.png")
	if img.AggWorkKey == "" {
		t.Fatal("图片应作为聚合成员收编（与文档同标识）")
	}
	if img.SetGroupKey != "" || img.SetName != "" || img.SetRuleID != "" {
		t.Fatalf("已被聚合收编的图片不进入作品集聚合: key=%q name=%q rule=%q",
			img.SetGroupKey, img.SetName, img.SetRuleID)
	}
	// b.png 的聚合组只有图片、必含文档轨零命中→组不成立；由产出四按目录归集。
	lone := aggConclusionOf(t, conclusions, "混合/b.png")
	wantKey := dirGroupKey([]string{"混合"})
	if lone.AggWorkKey != "" {
		t.Fatalf("必含轨零命中组不应成立: %q", lone.AggWorkKey)
	}
	if lone.SetGroupKey != wantKey || lone.SetName != "集甲" || lone.SetRuleID != "ws-png" {
		t.Fatalf("剩余文件应由产出四归集（key=%q name=%q rule=%q），实际 key=%q name=%q rule=%q",
			wantKey, "集甲", "ws-png", lone.SetGroupKey, lone.SetName, lone.SetRuleID)
	}
}

// —— 档案 schema / 校验 ——

// TestWorkSetAggProfileRejectsNoSetName 无集名产出拒绝锚（决策5）：产出四规则
// 未声明任何作品集维度提取动作时——载入（ParseProfile）跳过该条目、编辑保存
// （ApplySaveRule）即时报错且不落盘。
func TestWorkSetAggProfileRejectsNoSetName(t *testing.T) {
	noName := RuleEntry{
		ID:          "ws-no-name",
		Description: "测试：无集名产出的作品集聚合",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{{
			Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{集名}")},
		}},
		Extractions: []ExtractionAction{{
			Dimension: DimAuthor, From: FromCapture, CaptureName: condxVocab["集名"], Row: 1,
		}},
		WorkSetAggregation: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeDirectory,
		}},
	}

	// 载入侧：规则跳过 + 告警点名集名来源。
	data, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{noName}})
	if err != nil {
		t.Fatalf("序列化档案失败: %v", err)
	}
	profile, errs := ParseProfile(data)
	if profile == nil || len(profile.Rules) != 0 {
		t.Fatalf("无集名产出的产出四条目应被载入拒绝，实际载入 %d 条", len(profile.Rules))
	}
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "集名") {
		t.Fatalf("载入告警应点名集名来源: %v", errs)
	}

	// 保存侧：编辑保存被拦且档案不落盘。
	rj, err := ruleToJSON(noName)
	if err != nil {
		t.Fatalf("规则转编辑形态失败: %v", err)
	}
	saveJSON, err := json.Marshal(rj)
	if err != nil {
		t.Fatalf("序列化编辑规则失败: %v", err)
	}
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	if _, err := mgr.ApplySaveRule(saveJSON); err == nil || !strings.Contains(err.Error(), "集名") {
		t.Fatalf("无集名产出的产出四规则保存应被拦并点名集名: %v", err)
	}
	if _, stored := store.values[RuleProfileKVKey]; stored {
		t.Fatalf("保存被拦不应落盘档案")
	}
}

// TestWorkSetAggProfileRoundTripAndBindingValidation 档案 schema 锚：产出四声明
// 随档案 JSON 往返保真（workSetAggregation 键与组键绑定结构）；组键绑定的形态
// 与引用校验复用统一分组绑定校验（目录形态不得携带键部分、标识形态须有键部分、
// 序列群键保留名只选自序列目标行）。
func TestWorkSetAggProfileRoundTripAndBindingValidation(t *testing.T) {
	good := workSetAggCaptureRule()
	data, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{good}})
	if err != nil {
		t.Fatalf("序列化档案失败: %v", err)
	}
	if !strings.Contains(string(data), `"workSetAggregation"`) {
		t.Fatalf("产出四声明应随档案落档: %s", data)
	}
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || len(profile.Rules) != 1 {
		t.Fatalf("产出四条目应正常载入: errs=%v rules=%d", errs, len(profile.Rules))
	}
	entry := profile.Rules[0]
	if entry.WorkSetAggregation == nil || entry.WorkSetAggregation.Grouping == nil ||
		entry.WorkSetAggregation.Grouping.Mode != GroupingModeIdentity ||
		len(entry.WorkSetAggregation.Grouping.Identity) != 1 ||
		entry.WorkSetAggregation.Grouping.Identity[0].Capture != condxVocab["集名"] {
		t.Fatalf("组键绑定未随档案往返保真: %+v", entry.WorkSetAggregation)
	}
	again, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("读回档案再序列化失败: %v", err)
	}
	if string(again) != string(data) {
		t.Fatalf("产出四档案写读往返应字节等价\n首次: %s\n读回: %s", data, again)
	}

	// 与聚合产出并列零互斥：同一规则同时携带聚合产出与产出四可正常载入。
	combined := workSetAggRuleOf(t, 2) // 一图一文本→article 聚合种子
	combined.ID = "ws-plus-agg"
	combined.Extractions = []ExtractionAction{{Dimension: DimCollection, From: FromConstant, Constant: "集丙"}}
	combined.WorkSetAggregation = &WorkSetAggregationProduction{Grouping: &GroupingBinding{Mode: GroupingModeDirectory}}
	combinedData, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{combined}})
	if err != nil {
		t.Fatalf("序列化聚合+产出四规则失败: %v", err)
	}
	combinedProfile, combinedErrs := ParseProfile(combinedData)
	if len(combinedErrs) != 0 || len(combinedProfile.Rules) != 1 {
		t.Fatalf("聚合产出与产出四应并列零互斥（同规则并用可载入）: errs=%v rules=%d", combinedErrs, len(combinedProfile.Rules))
	}
	if got := combinedProfile.Rules[0]; got.Aggregate == nil || got.WorkSetAggregation == nil {
		t.Fatalf("并用规则应同时保有聚合产出与产出四声明: %+v", got)
	}

	cases := []struct {
		name     string
		grouping *GroupingBinding
		want     string
	}{
		{"目录形态不得携带键部分", &GroupingBinding{
			Mode: GroupingModeDirectory, Identity: []IdentityPart{{Row: 1, Capture: condxVocab["集名"]}},
		}, "不得携带键部分"},
		{"标识形态须有键部分", &GroupingBinding{Mode: GroupingModeIdentity}, "缺少键部分"},
		{"序列群键只选自序列目标行", &GroupingBinding{
			Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}},
		}, "只能选自序列目标行"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := workSetAggCaptureRule()
			rule.WorkSetAggregation = &WorkSetAggregationProduction{Grouping: tc.grouping}
			data, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{rule}})
			if err != nil {
				t.Fatalf("序列化档案失败: %v", err)
			}
			profile, errs := ParseProfile(data)
			if profile == nil || len(profile.Rules) != 0 {
				t.Fatalf("非法组键绑定的产出四条目应被载入拒绝，实际载入 %d 条", len(profile.Rules))
			}
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), tc.want) {
				t.Fatalf("载入告警应含 %q: %v", tc.want, errs)
			}
		})
	}
}

// —— 集归属事实入识别快照（v4） ——

// TestSnapshotWorkSetAttributionFields 集归属字段落快照锚：识别轮求值产出的
// 集归属三字段（setGroupKey/setName/setRuleId）随快照文件记录落档；结构版本
// 为 4；无集归属文件三字段零值（omitempty 不落键）。
func TestSnapshotWorkSetAttributionFields(t *testing.T) {
	if recognitionSnapshotSchemaVersion != 4 {
		t.Fatalf("快照结构版本应为 4: %d", recognitionSnapshotSchemaVersion)
	}
	root := t.TempDir()
	writeTestFile(t, root+"/组/甲_01.png", "x")
	writeTestFile(t, root+"/组/甲_02.png", "y")
	writeTestFile(t, root+"/组/乙_01.png", "z")
	writeTestFile(t, root+"/组/未命中.txt", "w")
	enumRes, err := NewScanner(root).Enumerate(nil)
	if err != nil {
		t.Fatalf("枚举测试树失败: %v", err)
	}
	rules := workSetAggProfileRules(t, workSetAggCaptureRule())
	files := buildScanFiles(enumRes.AllFiles)
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, nil)
	snap := aggregatePanorama(aggregateInput{
		root: root, enumRes: enumRes,
		enabledRules: rules, allRules: rules,
		conclusions: conclusions, prevSkipped: map[string]bool{},
	})
	if snap == nil {
		t.Fatal("快照装配不应为空")
	}
	if snap.SchemaVersion != recognitionSnapshotSchemaVersion {
		t.Fatalf("快照结构版本应为 %d: %d", recognitionSnapshotSchemaVersion, snap.SchemaVersion)
	}
	byPath := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	for _, rel := range []string{"组/甲_01.png", "组/甲_02.png"} {
		f := byPath[rel]
		if f.SetGroupKey != "甲" || f.SetName != "甲" || f.SetRuleID != "ws-capture" {
			t.Fatalf("%s 的集归属三字段应随快照落档（key=甲 name=甲 rule=ws-capture），实际 key=%q name=%q rule=%q",
				rel, f.SetGroupKey, f.SetName, f.SetRuleID)
		}
	}
	if f := byPath["组/乙_01.png"]; f.SetName != "乙" || f.SetGroupKey != "乙" {
		t.Fatalf("不同捕获值的文件应落不同集归属: key=%q name=%q", f.SetGroupKey, f.SetName)
	}
	if f := byPath["组/未命中.txt"]; f.SetGroupKey != "" || f.SetName != "" || f.SetRuleID != "" {
		t.Fatalf("无集归属文件的集归属字段应零值: key=%q name=%q rule=%q",
			f.SetGroupKey, f.SetName, f.SetRuleID)
	}
}

// TestSnapshotWorkSetAttributionV4Compat 快照 v4 双向兼容锚（风险2）：
//   - 新写新读：集归属三字段随快照 JSON 往返保真、结构版本为 4；
//   - 旧读新（降级插件版本后旧代码读取 v4 快照）：v3 时代的快照结构体按 Go
//     JSON 语义忽略未知字段、schemaVersion 前进按原值载入而不触发拒绝（快照
//     读取路径无结构版本守卫）——旧代码不因新字段/新版本号失败；
//   - 新读旧：v3 旧快照（无集归属字段）读入按零值降级，聚合归属等既有字段
//     照常解读。
func TestSnapshotWorkSetAttributionV4Compat(t *testing.T) {
	if recognitionSnapshotSchemaVersion != 4 {
		t.Fatalf("快照结构版本应为 4: %d", recognitionSnapshotSchemaVersion)
	}
	snap := &RecognitionSnapshot{
		SchemaVersion: recognitionSnapshotSchemaVersion,
		Files: []SnapFile{
			{RelPath: "组/甲_01.png", FileName: "甲_01.png", State: snapStateIdentified,
				SetGroupKey: "甲", SetName: "甲", SetRuleID: "ws-capture",
				AggWorkKey: "f:组/甲_01.png", AggTrackRole: "image"},
			{RelPath: "组/散图.jpg", FileName: "散图.jpg", State: snapStatePlain},
		},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("序列化快照失败: %v", err)
	}
	var back RecognitionSnapshot
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("反序列化快照失败: %v", err)
	}
	if back.SchemaVersion != 4 || back.Files[0].SetGroupKey != "甲" || back.Files[0].SetName != "甲" ||
		back.Files[0].SetRuleID != "ws-capture" {
		t.Fatalf("集归属三字段应随快照往返保真: %+v", back.Files)
	}
	if back.Files[1].SetGroupKey != "" || back.Files[1].SetName != "" || back.Files[1].SetRuleID != "" {
		t.Fatalf("无集归属文件的集归属字段应零值: %+v", back.Files[1])
	}

	// 旧读新（v3 代码结构读取 v4 快照）：未知字段忽略、版本号原样载入。
	type legacySnapFileV3 struct {
		RelPath      string `json:"relPath"`
		FileName     string `json:"fileName"`
		State        string `json:"state"`
		AggWorkKey   string `json:"aggWorkKey,omitempty"`
		AggTrackRole string `json:"aggTrackRole,omitempty"`
		AggWorkType  string `json:"aggWorkType,omitempty"`
	}
	type legacySnapshotV3 struct {
		SchemaVersion int                `json:"schemaVersion"`
		Files         []legacySnapFileV3 `json:"files"`
	}
	var legacy legacySnapshotV3
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatalf("旧代码结构读取 v4 快照不应失败（未知字段按 JSON 语义忽略）: %v", err)
	}
	if legacy.SchemaVersion != 4 {
		t.Fatalf("旧代码结构应原样读取 schemaVersion=4（快照读取路径无版本守卫）: %d", legacy.SchemaVersion)
	}
	if len(legacy.Files) != 2 || legacy.Files[0].RelPath != "组/甲_01.png" ||
		legacy.Files[0].AggWorkKey != "f:组/甲_01.png" || legacy.Files[0].AggTrackRole != "image" {
		t.Fatalf("旧代码结构应照常解读既有聚合归属字段: %+v", legacy.Files)
	}

	// 新读旧（v3 快照降级）：无集归属字段按零值载入，既有聚合归属照常解读。
	v3 := []byte(`{"schemaVersion":3,"files":[{"relPath":"组/甲_01.png","fileName":"甲_01.png","state":"identified","aggWorkKey":"f:组/甲_01.png"}]}`)
	var old RecognitionSnapshot
	if err := json.Unmarshal(v3, &old); err != nil {
		t.Fatalf("v3 旧快照读入失败: %v", err)
	}
	if old.SchemaVersion != 3 || old.Files[0].SetGroupKey != "" || old.Files[0].SetName != "" || old.Files[0].SetRuleID != "" {
		t.Fatalf("v3 旧快照应按空集归属降级: %+v", old.Files[0])
	}
	if old.Files[0].AggWorkKey != "f:组/甲_01.png" {
		t.Fatalf("v3 旧快照既有聚合归属应照常解读: %+v", old.Files[0])
	}
}
