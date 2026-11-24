// 序列群键保留名 groupKey 的库级取值源锚（分组层强化②）：产出四「作品集聚合」
// 的组键绑定可用「行内序列行群产物优先 → 库级群卡键兜底」两级取值源；既有
// 「元数据文件」分组绑定与聚合标识的入口不扩取值源（行内独占）。
// 库级索引由汇流结论的分组归属按成员路径反查群卡键（workset_order.go 群卡
// 反查同口径）。
package main

import (
	"reflect"
	"testing"
)

// TestGroupingLibraryGroupKeySource 库级取值源锚：序列群键保留名在授权取值源
// （groupKeySourceLibraryFallback——产出四「作品集聚合」的分组项专用）按「行内
// 序列行群产物优先、库级群卡键兜底」取值；行内优先压过库级；索引缺项/nil 索
// 引不可得；普通捕获名仍按行内汇流（库级源不参与）；所在目录形态不受库级源
// 影响。
func TestGroupingLibraryGroupKeySource(t *testing.T) {
	groupKeyParts := []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}}
	lib := libraryGroupKeyIndex{
		"d/行内.jpg":  "库级键",
		"d/无行内.jpg": "库级键",
	}
	// 行内序列行群产物在场：两种取值源都取行内键（行内优先）。
	inlineHits := aggFileRowHits{&ParseContext{Group: &GroupAnalysis{Key: "行内键"}}, nil}
	if v, ok := inlineHits.identityValueWithSource(groupKeyParts, groupKeySourceInline, "", nil); !ok || v != "行内键" {
		t.Fatalf("行内群产物应供序列群键: %q %v", v, ok)
	}
	if v, ok := inlineHits.identityValueWithSource(groupKeyParts, groupKeySourceLibraryFallback, "d/行内.jpg", lib); !ok || v != "行内键" {
		t.Fatalf("库级取值源形态下行内键应优先: %q %v", v, ok)
	}
	// 行内无群产物：行内独占取值源不可得；库级取值源按成员路径反查库级键。
	noInline := aggFileRowHits{nil, nil}
	if v, ok := noInline.identityValueWithSource(groupKeyParts, groupKeySourceInline, "d/无行内.jpg", lib); ok {
		t.Fatalf("行内独占取值源不得读库级群卡键: %q", v)
	}
	if v, ok := noInline.identityValueWithSource(groupKeyParts, groupKeySourceLibraryFallback, "d/无行内.jpg", lib); !ok || v != "库级键" {
		t.Fatalf("授权取值源应按成员路径兜底库级群卡键: %q %v", v, ok)
	}
	// 索引缺项 / nil 索引：不可得（该文件不入组）。
	if v, ok := noInline.identityValueWithSource(groupKeyParts, groupKeySourceLibraryFallback, "d/缺项.jpg", lib); ok {
		t.Fatalf("库级索引缺项应不可得: %q", v)
	}
	if v, ok := noInline.identityValueWithSource(groupKeyParts, groupKeySourceLibraryFallback, "d/无行内.jpg", nil); ok {
		t.Fatalf("nil 索引应不可得: %q", v)
	}
	// 普通捕获名不受库级源影响：仍取行内绑定值。
	capHits := aggFileRowHits{&ParseContext{Captures: []CaptureBinding{{Name: rowCaptureName(1, "work"), Value: "标识值"}}}}
	workParts := []IdentityPart{{Row: 1, Capture: "work"}}
	if v, ok := capHits.identityValueWithSource(workParts, groupKeySourceLibraryFallback, "d/无行内.jpg", lib); !ok || v != "标识值" {
		t.Fatalf("普通捕获按行内汇流取值: %q %v", v, ok)
	}
}

// TestGroupingMetadataBindingInlineOnly 既有绑定零漂移锚：同一命中面与
// 同一库级索引下，「元数据文件」分组绑定与聚合成作品（行内独占取值源）的自身
// 键都不得读到库级群卡键——行内无群产物的文件维持「不入组」语义；仅产出四授
// 权的库级取值源可兜底。所在目录分组两取值源同值（目录键恒可得，库级源不参
// 与）。
func TestGroupingMetadataBindingInlineOnly(t *testing.T) {
	lib := libraryGroupKeyIndex{"d/n_01.txt": "序列键"}
	noInline := aggFileRowHits{nil}
	file := ScanFile{DirNames: []string{"d"}, FileName: "n_01.txt", RelPath: "d/n_01.txt"}
	identityBinding := &GroupingBinding{
		Mode:     GroupingModeIdentity,
		Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}},
	}
	if v, ok := groupingSelfKeyOf(noInline, identityBinding, file, groupKeySourceInline, lib); ok {
		t.Fatalf("行内独占取值源不得读库级群卡键（行内无群产物应不入组）: %q", v)
	}
	if v, ok := groupingSelfKeyOf(noInline, identityBinding, file, groupKeySourceLibraryFallback, lib); !ok || v != "序列键" {
		t.Fatalf("产出四库级取值源应兜底库级群卡键: %q %v", v, ok)
	}
	directoryBinding := &GroupingBinding{Mode: GroupingModeDirectory, Row: 1}
	wantDirKey := dirGroupKey([]string{"d"})
	if v, ok := groupingSelfKeyOf(noInline, directoryBinding, file, groupKeySourceInline, lib); !ok || v != wantDirKey {
		t.Fatalf("既有所在目录分组应取目录聚合键: %q（期望 %q）", v, wantDirKey)
	}
	if v, ok := groupingSelfKeyOf(noInline, directoryBinding, file, groupKeySourceLibraryFallback, lib); !ok || v != wantDirKey {
		t.Fatalf("库级取值源的所在目录分组应取目录聚合键（库级源不参与）: %q（期望 %q）", v, wantDirKey)
	}
}

// TestGroupingMetadataBindingZeroDrift 既有绑定用例结果零漂移锚（行为面）：库级存在该说明文件的图形群卡键（兄弟群规则把它收进序列群），元数据
// 文件分组绑定的序列群键保留名仍只读行内群产物——说明文件的行内无群产物即不
// 入组、按独立作品导入（不获说明成员标记、不产生内容扇出）；若库级取值源被误
// 接到既有绑定形态，该说明文件会经库级群卡键并入组、取得扇出键集与说明成员
// 标记。夹具同时断言库级索引确已含该成员（排除键缺席导致的假通过）。
func TestGroupingMetadataBindingZeroDrift(t *testing.T) {
	siblingPattern, err := NewSiblingPattern(`^([^_]+)_`, `(\d+)`, 2, OutcomeCollection)
	if err != nil {
		t.Fatalf("构造兄弟群组规则失败: %v", err)
	}
	groupingRule := RuleEntry{
		ID: "group-seq", Description: "测试：兄弟群规则", Source: RuleSourceManual, Enabled: true,
		Pattern: siblingPattern,
	}
	mappingRule := RuleEntry{
		ID:          "map-pair",
		Description: "测试：元数据文件标识分组（序列群键保留名）",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}_{序号}", ".png"),
			fanoutFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
				{Row: 1, Capture: identityGroupKeyCapture},
			}},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles("无题/无题_01.png", "无题/无题_02.png", "无题/无题_03.txt")
	rules := []RuleEntry{groupingRule, mappingRule}
	conclusions := fanoutRun(t, files, rules)

	// 夹具活性：兄弟群规则已把说明文件收进库级序列群（库级索引可得该键）
	if got := libraryGroupKeysOfConclusions(conclusions)["无题/无题_03.txt"]; got != "无题" {
		t.Fatalf("夹具缺损：说明文件应属库级序列群「无题」，实际库级群键 %q", got)
	}
	note := aggConclusionOf(t, conclusions, "无题/无题_03.txt")
	if note.AggWorkKey != "" || note.AggCompanion || len(fanoutKeysOf(note)) != 0 {
		t.Fatalf("既有元数据文件分组绑定不得经库级群卡键入组（行内无群产物=不入组、独立作品）: key=%q companion=%v fanout=%v",
			note.AggWorkKey, note.AggCompanion, note.FanoutWorkKeys)
	}
}

// 归属按成员路径反查群卡键（规则定群与结构群成员同权，含已识别成员——群归属
// 只在群卡）；无分组归属的成员不入索引；空路径/空键不计；nil 索引查询与写入
// 均安全。
func TestGroupingLibraryGroupKeyIndexFromConclusions(t *testing.T) {
	conclusions := []FileConclusion{
		{RelPath: "d/p_01.jpg", Group: &GroupAnalysis{Key: "p"}},
		{RelPath: "d/p_02.jpg", Group: &GroupAnalysis{Key: "p"}},
		{RelPath: "d/q 01.jpg", Group: &GroupAnalysis{Key: "q"}, GroupRuleID: "rule-seq"},
		{RelPath: "d/lone.jpg"},
		{Group: &GroupAnalysis{Key: "空路径不计"}},
	}
	idx := libraryGroupKeysOfConclusions(conclusions)
	want := libraryGroupKeyIndex{
		"d/p_01.jpg": "p",
		"d/p_02.jpg": "p",
		"d/q 01.jpg": "q",
	}
	if !reflect.DeepEqual(idx, want) {
		t.Fatalf("库级群键索引不符：\ngot:  %+v\nwant: %+v", idx, want)
	}
	for rel, key := range want {
		if v, ok := idx.groupKeyOf(rel); !ok || v != key {
			t.Fatalf("成员 %s 的库级群键应为 %q: %q %v", rel, key, v, ok)
		}
	}
	if v, ok := idx.groupKeyOf("d/lone.jpg"); ok {
		t.Fatalf("无分组归属成员不应入索引: %q", v)
	}
	var nilIdx libraryGroupKeyIndex
	nilIdx.put("d/x.jpg", "键") // nil 索引写入静默忽略
	if v, ok := nilIdx.groupKeyOf("d/x.jpg"); ok {
		t.Fatalf("nil 索引不应反查得到键: %q", v)
	}
	empty := libraryGroupKeyIndex{}
	empty.put("", "键")
	empty.put("d/y.jpg", "")
	if len(empty) != 0 {
		t.Fatalf("空路径/空键不应收录: %+v", empty)
	}
}
