// 解析内核流水线的分组与提取两段单元测试：序号群定群与落单独立、内置分隔惯例
// 的声明产出、无规则命中时的现状等价、用户规则先于内置的声明顺序。
package main

import (
	"reflect"
	"testing"
)

// decisionsByName 把结论切片按文件名索引，重复文件名以最后一条为准（测试数据不含重复）。
func decisionsByName(t *testing.T, decisions []FileDecision) map[string]FileDecision {
	t.Helper()
	byName := make(map[string]FileDecision, len(decisions))
	for _, decision := range decisions {
		byName[decision.FileName] = decision
	}
	return byName
}

// TestRunPipelineSiblingGrouping 覆盖作者目录下的序号群场景：群成员各自得到
// 同键群组分析（Seq 对应、Size 为成员数），落单文件保持独立；同时验证 ctx 的
// DirNames/Depth 基础字段填充（FromParentDir 取到父目录名）与分组段产出的
// 群组分析流入提取段（FromGroupPrefix 取到公共前缀）。
func TestRunPipelineSiblingGrouping(t *testing.T) {
	rules := []RuleEntry{
		{
			ID:          "grp-work",
			Description: "作者目录下的序号群：work_01.jpg 起连续编号归入同一作品集",
			Source:      RuleSourceManual,
			Enabled:     true,
			Pattern:     mustSibling(t, `^(.+)_\d+`, `_(\d+)(?:\.[^.]+)?$`, 2, OutcomeCollection),
		},
		{
			ID:          "decl-collection",
			Description: "群成员以群组公共前缀声明作品集维度",
			Source:      RuleSourceManual,
			Enabled:     true,
			Pattern:     mustRegex(t, string(TargetFileName), `^work_`, nil),
			Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromGroupPrefix}},
		},
		{
			ID:          "decl-parent",
			Description: "全部文件以直接父目录名声明标签维度",
			Source:      RuleSourceManual,
			Enabled:     true,
			Pattern:     mustRegex(t, string(TargetFileName), `.*`, nil),
			Extractions: []ExtractionAction{{Dimension: DimTag, From: FromParentDir}},
		},
	}
	files := []ScanFile{
		{Path: `E:\lib\作者A\work_01.jpg`, FileName: "work_01.jpg", DirNames: []string{"作者A"}, RelPath: "作者A/work_01.jpg"},
		{Path: `E:\lib\作者A\work_02.jpg`, FileName: "work_02.jpg", DirNames: []string{"作者A"}, RelPath: "作者A/work_02.jpg"},
		{Path: `E:\lib\作者A\work_03.jpg`, FileName: "work_03.jpg", DirNames: []string{"作者A"}, RelPath: "作者A/work_03.jpg"},
		{Path: `E:\lib\作者A\cover.png`, FileName: "cover.png", DirNames: []string{"作者A"}, RelPath: "作者A/cover.png"},
	}
	decisions := RunPipeline(files, rules, nil)
	if len(decisions) != len(files) {
		t.Fatalf("结论数 %d 与输入文件数 %d 不一致", len(decisions), len(files))
	}
	for i, decision := range decisions {
		if decision.RelPath != files[i].RelPath || decision.FileName != files[i].FileName {
			t.Fatalf("结论 %d 的身份字段 (%q, %q) 与输入 (%q, %q) 不一致", i, decision.RelPath, decision.FileName, files[i].RelPath, files[i].FileName)
		}
	}
	byName := decisionsByName(t, decisions)
	memberCases := []struct {
		fileName string
		wantSeq  int
	}{
		{"work_01.jpg", 1},
		{"work_02.jpg", 2},
		{"work_03.jpg", 3},
	}
	wantMemberDecls := []MetadataDeclaration{
		{Dimension: DimCollection, Value: "work", Source: SourceGroup, RuleID: "decl-collection"},
		{Dimension: DimTag, Value: "作者A", Source: SourceDir, RuleID: "decl-parent"},
	}
	for _, tc := range memberCases {
		decision := byName[tc.fileName]
		if decision.Group == nil {
			t.Fatalf("%s 应为群成员，实际 Group 为 nil", tc.fileName)
		}
		if decision.Group.Key != "work" || decision.Group.Prefix != "work" {
			t.Errorf("%s 的群组键/前缀为 (%q, %q)，期望均为 %q", tc.fileName, decision.Group.Key, decision.Group.Prefix, "work")
		}
		if decision.Group.Seq != tc.wantSeq {
			t.Errorf("%s 的序号为 %d，期望 %d", tc.fileName, decision.Group.Seq, tc.wantSeq)
		}
		if decision.Group.Size != 3 {
			t.Errorf("%s 的群组成员数为 %d，期望 3", tc.fileName, decision.Group.Size)
		}
		if !reflect.DeepEqual(decision.MatchedRuleIDs, []string{"decl-collection", "decl-parent"}) {
			t.Errorf("%s 的命中规则为 %v，期望 [decl-collection decl-parent]", tc.fileName, decision.MatchedRuleIDs)
		}
		if !reflect.DeepEqual(decision.Declarations, wantMemberDecls) {
			t.Errorf("%s 的声明为 %+v，期望 %+v", tc.fileName, decision.Declarations, wantMemberDecls)
		}
	}
	lone := byName["cover.png"]
	if lone.Group != nil {
		t.Errorf("落单文件 cover.png 的 Group 应为 nil，实际 %+v", lone.Group)
	}
	if !reflect.DeepEqual(lone.MatchedRuleIDs, []string{"decl-parent"}) {
		t.Errorf("落单文件的命中规则为 %v，期望 [decl-parent]", lone.MatchedRuleIDs)
	}
	wantLoneDecls := []MetadataDeclaration{{Dimension: DimTag, Value: "作者A", Source: SourceDir, RuleID: "decl-parent"}}
	if !reflect.DeepEqual(lone.Declarations, wantLoneDecls) {
		t.Errorf("落单文件的声明为 %+v，期望 %+v", lone.Declarations, wantLoneDecls)
	}
}

// TestRunPipelineExampleTemplateDeclarations 覆盖同目录多格式并存场景（内置
// 退休后的等价路径=用户从例子模板复制为档案条目）：v4 模板规则（【{作者}】
// {标题} 与 {作者}_{任意}）对「【作者】标题.ext」与「作者_尾部码.ext」两类
// 文件各自产出 author 与 title 声明，值正确且互不串扰。
func TestRunPipelineExampleTemplateDeclarations(t *testing.T) {
	mustCondRule := func(id, tpl string, extractions ...ExtractionAction) RuleEntry {
		t.Helper()
		rows := []CondxRow{{Condition: &Condx{
			Target: CondxTargetFile,
			Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
		}}}
		pattern, err := CompileRuleConditions(rows)
		if err != nil {
			t.Fatalf("模板 %q 编译失败: %v", tpl, err)
		}
		return RuleEntry{ID: id, Description: "例子模板复制的档案条目", Source: RuleSourceManual,
			Enabled: true, Conditions: rows, Pattern: pattern, Extractions: extractions}
	}
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{
		mustCondRule("tpl-bracket", "【{作者}】{标题}",
			ExtractionAction{Dimension: DimAuthor, From: FromCapture, CaptureName: condxVocab["作者"], Row: 1},
			ExtractionAction{Dimension: DimTitle, From: FromCapture, CaptureName: condxVocab["标题"], Row: 1}),
		mustCondRule("tpl-title-any", "{标题}_{任意}",
			ExtractionAction{Dimension: DimTitle, From: FromCapture, CaptureName: condxVocab["标题"], Row: 1}),
	}}
	files := []ScanFile{
		{Path: `E:\lib\混合\【小明】夏天的风.jpg`, FileName: "【小明】夏天的风.jpg", DirNames: []string{"混合"}, RelPath: "混合/【小明】夏天的风.jpg"},
		{Path: `E:\lib\混合\夜航船_code42.png`, FileName: "夜航船_code42.png", DirNames: []string{"混合"}, RelPath: "混合/夜航船_code42.png"},
	}
	cases := []struct {
		fileName   string
		wantRuleID string
		wantAuthor string
		wantTitle  string
	}{
		{"【小明】夏天的风.jpg", "tpl-bracket", "小明", "夏天的风"},
		{"夜航船_code42.png", "tpl-title-any", "", "夜航船"},
	}
	decisions := RunPipeline(files, EffectiveRules(profile), nil)
	byName := decisionsByName(t, decisions)
	for _, tc := range cases {
		decision := byName[tc.fileName]
		if decision.Group != nil {
			t.Errorf("%s 无分组规则命中，Group 应为 nil，实际 %+v", tc.fileName, decision.Group)
		}
		if !reflect.DeepEqual(decision.MatchedRuleIDs, []string{tc.wantRuleID}) {
			t.Errorf("%s 的命中规则为 %v，期望 [%s]", tc.fileName, decision.MatchedRuleIDs, tc.wantRuleID)
		}
		wantDecls := []MetadataDeclaration{
			{Dimension: DimTitle, Value: tc.wantTitle, Source: SourceFilename, RuleID: tc.wantRuleID},
		}
		if tc.wantAuthor != "" {
			wantDecls = append(wantDecls, MetadataDeclaration{Dimension: DimAuthor, Value: tc.wantAuthor, Source: SourceFilename, RuleID: tc.wantRuleID})
		}
		if len(decision.Declarations) != len(wantDecls) {
			t.Errorf("%s 的声明为 %+v，期望 %+v", tc.fileName, decision.Declarations, wantDecls)
			continue
		}
		for _, want := range wantDecls {
			found := false
			for _, got := range decision.Declarations {
				if got == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s 缺少期望声明 %+v，实得 %+v", tc.fileName, want, decision.Declarations)
			}
		}
	}
}

// TestRunPipelineNoEffectiveMatches 覆盖现状等价断言：无规则或规则全不命中时，
// 同目录多文件全部拆为独立作品，无声明、无命中规则，结论与输入同序。
func TestRunPipelineNoEffectiveMatches(t *testing.T) {
	cases := []struct {
		name  string
		rules []RuleEntry
	}{
		{
			name:  "无规则",
			rules: nil,
		},
		{
			name: "规则全不命中",
			rules: []RuleEntry{{
				ID:          "never-match",
				Description: "任何文件都不命中的规则",
				Source:      RuleSourceManual,
				Enabled:     true,
				Pattern:     mustRegex(t, string(TargetFileName), `^zzz_`, nil),
				Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromConstant, Constant: "不应产出"}},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := []ScanFile{
				{Path: `E:\lib\散页\a.jpg`, FileName: "a.jpg", DirNames: []string{"散页"}, RelPath: "散页/a.jpg"},
				{Path: `E:\lib\散页\b.jpg`, FileName: "b.jpg", DirNames: []string{"散页"}, RelPath: "散页/b.jpg"},
				{Path: `E:\lib\散页\c.png`, FileName: "c.png", DirNames: []string{"散页"}, RelPath: "散页/c.png"},
			}
			decisions := RunPipeline(files, tc.rules, nil)
			if len(decisions) != len(files) {
				t.Fatalf("结论数 %d 与输入文件数 %d 不一致", len(decisions), len(files))
			}
			for i, decision := range decisions {
				if decision.RelPath != files[i].RelPath {
					t.Errorf("结论 %d 的相对路径 %q 与输入 %q 不一致", i, decision.RelPath, files[i].RelPath)
				}
				if decision.Group != nil {
					t.Errorf("%s 应为独立文件，实际 Group 为 %+v", decision.FileName, decision.Group)
				}
				if len(decision.Declarations) != 0 {
					t.Errorf("%s 不应产出声明，实际 %+v", decision.FileName, decision.Declarations)
				}
				if len(decision.MatchedRuleIDs) != 0 {
					t.Errorf("%s 不应命中规则，实际 %v", decision.FileName, decision.MatchedRuleIDs)
				}
			}
		})
	}
}

// TestRunPipelineUserRulePrecedesBuiltin 覆盖同对象多规则命中的顺序语义（内置
// 退休后由档案序承载）：EffectiveRules 保持档案顺序，档案内先出现的规则先求值
// ——命中规则 ID 序与声明序均按档案序排列。
func TestRunPipelineUserRulePrecedesBuiltin(t *testing.T) {
	mustCondRule := func(id, tpl string, extractions ...ExtractionAction) RuleEntry {
		t.Helper()
		rows := []CondxRow{{Condition: &Condx{
			Target: CondxTargetFile,
			Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
		}}}
		pattern, err := CompileRuleConditions(rows)
		if err != nil {
			t.Fatalf("模板 %q 编译失败: %v", tpl, err)
		}
		return RuleEntry{ID: id, Description: "档案序测试条目", Source: RuleSourceManual,
			Enabled: true, Conditions: rows, Pattern: pattern, Extractions: extractions}
	}
	first := mustCondRule("user-bracket-author", "【{作者}】{任意}",
		ExtractionAction{Dimension: DimAuthor, From: FromCapture, CaptureName: condxVocab["作者"], Row: 1})
	second := mustCondRule("user-bracket-full", "【{作者}】{标题}",
		ExtractionAction{Dimension: DimAuthor, From: FromCapture, CaptureName: condxVocab["作者"], Row: 1},
		ExtractionAction{Dimension: DimTitle, From: FromCapture, CaptureName: condxVocab["标题"], Row: 1})
	profile := &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{first, second}}
	files := []ScanFile{
		{Path: `E:\lib\【小明】夏天的风.jpg`, FileName: "【小明】夏天的风.jpg", DirNames: nil, RelPath: "【小明】夏天的风.jpg"},
	}
	decisions := RunPipeline(files, EffectiveRules(profile), nil)
	if len(decisions) != 1 {
		t.Fatalf("结论数 %d，期望 1", len(decisions))
	}
	decision := decisions[0]
	wantIDs := []string{"user-bracket-author", "user-bracket-full"}
	if !reflect.DeepEqual(decision.MatchedRuleIDs, wantIDs) {
		t.Errorf("命中规则为 %v，期望 %v（档案序）", decision.MatchedRuleIDs, wantIDs)
	}
	wantDecls := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "小明", Source: SourceFilename, RuleID: "user-bracket-author"},
		{Dimension: DimAuthor, Value: "小明", Source: SourceFilename, RuleID: "user-bracket-full"},
		{Dimension: DimTitle, Value: "夏天的风", Source: SourceFilename, RuleID: "user-bracket-full"},
	}
	if !reflect.DeepEqual(decision.Declarations, wantDecls) {
		t.Errorf("声明为 %+v，期望 %+v（档案序声明在前）", decision.Declarations, wantDecls)
	}
}
