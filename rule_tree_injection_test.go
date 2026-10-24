package main

import "testing"

// TestMergedRecognitionTreeInjectionAncestorAsserts v4 目录终结谓词在全量树
// 注入下的合并求值锚（live-test 批次一 T2 抓到的回归：MergedRecognition 此前
// 不注 AncestorFiles/AncestorChildDirCounts，子女类谓词（childrenAllFormat/
// childDirCount）保守不命中——识别主通路 T2 作者壳规则整体零命中）。
// 锚定两点：① 增量批（files 仅星夜集 2 件）+全量树（含月色集）→ childDirCount=2
// 成立、行B 子女格式成立、equal 跨行门成立，作者=Isel；② 无树注入（旧语义
// files 自聚合=目录树不全）→ 子女谓词不命中（保守语义防误判）。
func TestMergedRecognitionTreeInjectionAncestorAsserts(t *testing.T) {
	rule := RuleEntry{
		ID:          "t2-tree",
		Description: "T2 作者壳",
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{
				Target: CondxTargetDir,
				Format: condxTpl("Artist[{作者}]"),
				Attach: []CondxAttach{{Kind: CondxAttachChildDirCount, MinCount: 2}},
			}},
			{Connector: "and", Condition: &Condx{
				Target: CondxTargetDir,
				Format: condxTpl("[{作者}][{作品号}][{标题}]"),
				Attach: []CondxAttach{{Kind: CondxAttachChildrenAllFormat, Tpl: "{标题}_{序号}", MinCount: 2}},
				Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeEqual, AgainstRow: 1, AgainstPh: "作者"}},
			}},
		},
		Extractions: []ExtractionAction{
			{Dimension: DimCollection, From: FromCapture, CaptureName: "title", Row: 2},
			{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 2},
		},
	}
	pat, err := CompileRuleConditions(rule.Conditions)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	rule.Pattern = pat
	if err := validateExtractions(rule.ID, pat, rule.Extractions, declaredConditionCaptures(rule.Conditions)); err != nil {
		t.Fatalf("提取校验失败: %v", err)
	}

	// 增量批：仅星夜集目录 2 件（mtime 游标跳过其余目录）
	evalFiles := []ScanFile{
		{RelPath: "Artist[Isel]/[Isel][73989691][星夜集]/星夜集_001.jpg", FileName: "星夜集_001.jpg", DirNames: []string{"Artist[Isel]", "[Isel][73989691][星夜集]"}},
		{RelPath: "Artist[Isel]/[Isel][73989691][星夜集]/星夜集_002.jpg", FileName: "星夜集_002.jpg", DirNames: []string{"Artist[Isel]", "[Isel][73989691][星夜集]"}},
	}
	// 全量树：星夜集 + 月色集（childDirCount=2 的第二子目录来源）
	treeFiles := append(append([]ScanFile{}, evalFiles...),
		ScanFile{RelPath: "Artist[Isel]/[Isel][73989692][月色集]/月色集_001.jpg", FileName: "月色集_001.jpg", DirNames: []string{"Artist[Isel]", "[Isel][73989692][月色集]"}},
		ScanFile{RelPath: "Artist[Isel]/[Isel][73989692][月色集]/月色集_002.jpg", FileName: "月色集_002.jpg", DirNames: []string{"Artist[Isel]", "[Isel][73989692][月色集]"}},
	)

	concs := MergedRecognitionWithTree(evalFiles, treeFiles, []RuleEntry{rule}, nil, nil)
	if len(concs) != 2 {
		t.Fatalf("应产出 2 条结论，实际 %d", len(concs))
	}
	for _, c := range concs {
		if len(c.MatchedRuleIDs) != 1 || c.MatchedRuleIDs[0] != "t2-tree" {
			t.Fatalf("%s 应命中 t2-tree，实际 %v", c.RelPath, c.MatchedRuleIDs)
		}
		var col, auth string
		for _, d := range c.Final {
			if d.Dimension == DimCollection {
				col = d.Value
			}
			if d.Dimension == DimAuthor {
				auth = d.Value
			}
		}
		if col != "星夜集" || auth != "Isel" {
			t.Fatalf("%s 终选不符: collection=%q author=%q", c.RelPath, col, auth)
		}
	}

	// 旧语义（files 自聚合）：批内只见星夜集一目录 → childDirCount=1 < 2 → 不命中
	legacy := MergedRecognition(evalFiles, []RuleEntry{rule}, nil, nil)
	for _, c := range legacy {
		if len(c.MatchedRuleIDs) != 0 {
			t.Fatalf("批内目录树不全时不应命中（保守语义）: %s %v", c.RelPath, c.MatchedRuleIDs)
		}
	}
}
