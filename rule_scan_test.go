// 贪婪识别（FromScan）内核测试：真实规则 JSON 经反序列化驱动，锚定收敛档
// 四可裁点——确认口径（仅库内命中）、扫描范围（path/uncovered 的区间排除）、
// 多值裁决（标签全收/作者歧义不填记观察）、显式优先（scan 只补空维度）；
// 另锚定捕获区间绑定（Segment/Start/End）与构造期校验拒绝面。
package main

import (
	"fmt"
	"strings"
	"testing"
)

// scanRuleFromJSON 从规则 JSON 经档案层（ParseProfile，含构造期校验管线）
// 构造 RuleEntry。
func scanRuleFromJSON(t *testing.T, ruleJSON string) *RuleEntry {
	t.Helper()
	profile, errs := ParseProfile([]byte(fmt.Sprintf(`{"schemaVersion":3,"rules":[%s]}`, ruleJSON)))
	if len(errs) > 0 {
		t.Fatalf("规则载入失败: %v", errs)
	}
	if len(profile.Rules) != 1 {
		t.Fatalf("载入后规则数应为 1，实际 %d", len(profile.Rules))
	}
	return &profile.Rules[0]
}

// runScanRule 对单一规则执行提取段（与 mergedFileConclusionWithStructural 同构
// 的最小驱动），返回决策与规则私有上下文。
func runScanRule(t *testing.T, rule *RuleEntry, in *MatchInput, lib *LibraryIndex) *FileDecision {
	t.Helper()
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	decision := &FileDecision{RelPath: in.RelPath, FileName: in.FileName}
	runExtractionStage(decision, []RuleEntry{*rule}, in, lib, ctx)
	return decision
}

// scanValues 收集决策中指定维度的声明值（按产出序）。
func scanValues(decision *FileDecision, dim MetadataDimension) []string {
	var out []string
	for _, decl := range decision.Declarations {
		if decl.Dimension == dim {
			out = append(out, decl.Value)
		}
	}
	return out
}

// scanInput 为共享测试输入：文件名与上级目录各含库内可识别片段。
func scanInput(withBeko bool) *MatchInput {
	name := "Isel_瞬間的に絡み合う_01_[中文翻譯][高清].jpg"
	if withBeko {
		name = "Isel_瞬間的に絡み合う_01_[中文翻譯][高清][Beko].jpg"
	}
	return &MatchInput{
		FileName: name,
		DirNames: []string{"Artist[Isel]"},
		RelPath:  "Artist[Isel]/" + name,
	}
}

func scanTestLib() *LibraryIndex {
	return &LibraryIndex{
		AuthorNames:  map[string]struct{}{"Isel": {}, "Beko": {}},
		TagNames:     map[string]struct{}{"中文翻譯": {}, "高清": {}, "瞬間的に絡み合う": {}},
		WorkSetNames: map[string]struct{}{},
	}
}

// scanRuleJSONPrefix 构造带指定提取段的正则规则 JSON：setname 覆盖集名段，
// 尾部 [中文翻譯][高清] 落在捕获区间之外（uncovered 范围的分界素材）。
func scanRuleJSON(extractions string) string {
	return fmt.Sprintf(`{
		"id": "scan-t", "description": "贪婪识别测试", "source": "manual", "enabled": true,
		"pattern": {"kind": "regex", "target": "filename", "expr": "^Isel_(?P<setname>.+)_[0-9]+", "captures": ["setname"]},
		"extractions": [%s]
	}`, extractions)
}

// TestScanTagAllHits 确认口径+标签全收：path 范围收全部库内命中
// （集名段词 瞬間的に絡み合う + 括号内容 中文翻譯/高清，均命中库内标签集）。
func TestScanTagAllHits(t *testing.T) {
	rule := scanRuleFromJSON(t, scanRuleJSON(`{"dimension": "tag", "from": "scan", "scanScope": "path"}`))
	decision := runScanRule(t, rule, scanInput(false), scanTestLib())
	got := scanValues(decision, DimTag)
	// 候选发现序：同分段内括号内容先于分隔符切分词（中文翻譯/高清 为括号、
	// 瞬間的に絡み合う 为切分词）。
	want := []string{"中文翻譯", "高清", "瞬間的に絡み合う"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("标签贪婪识别应全收: got %v want %v", got, want)
	}
	if decision.MatchedRuleIDs[0] != "scan-t" {
		t.Fatalf("规则应命中: %v", decision.MatchedRuleIDs)
	}
}

// TestScanTagUncoveredExcludesCapturedSpan uncovered 范围：排除与捕获区间重叠
// 的候选（集名段词被 setname 覆盖→剔除；括号标签在区间外→保留）。
func TestScanTagUncoveredExcludesCapturedSpan(t *testing.T) {
	rule := scanRuleFromJSON(t, scanRuleJSON(`{"dimension": "tag", "from": "scan", "scanScope": "uncovered"}`))
	decision := runScanRule(t, rule, scanInput(false), scanTestLib())
	got := scanValues(decision, DimTag)
	want := []string{"中文翻譯", "高清"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("未覆盖范围应剔除捕获段内候选: got %v want %v", got, want)
	}
}

// TestScanAuthorUniqueAndAmbiguous 作者扫描：目录 Artist[Isel] 唯一命中→产出；
// 文件名再含 [Beko]→歧义不填并记 ScanNote（宁缺勿错）。
func TestScanAuthorUniqueAndAmbiguous(t *testing.T) {
	rule := scanRuleFromJSON(t, scanRuleJSON(`{"dimension": "author", "from": "scan", "scanScope": "path"}`))

	decision := runScanRule(t, rule, scanInput(false), scanTestLib())
	got := scanValues(decision, DimAuthor)
	if fmt.Sprint(got) != fmt.Sprint([]string{"Isel"}) {
		t.Fatalf("唯一命中应产出: %v", got)
	}
	if len(decision.ScanNotes) != 0 {
		t.Fatalf("唯一命中不应记歧义: %v", decision.ScanNotes)
	}

	decision = runScanRule(t, rule, scanInput(true), scanTestLib())
	if got := scanValues(decision, DimAuthor); len(got) != 0 {
		t.Fatalf("歧义不应产出: %v", got)
	}
	if len(decision.ScanNotes) != 1 || decision.ScanNotes[0].Dimension != DimAuthor ||
		fmt.Sprint(decision.ScanNotes[0].Values) != fmt.Sprint([]string{"Beko", "Isel"}) {
		t.Fatalf("歧义应记 ScanNote(Beko,Isel): %+v", decision.ScanNotes)
	}
}

// TestScanExplicitFirst 显式优先：显式捕获行先产出作者维度，scan 行只补空维度
// （同维度 scan 不再产出）。
func TestScanExplicitFirst(t *testing.T) {
	rule := scanRuleFromJSON(t, scanRuleJSON(
		`{"dimension": "author", "from": "capture", "captureName": "setname"},
		 {"dimension": "author", "from": "scan", "scanScope": "path"}`))
	decision := runScanRule(t, rule, scanInput(false), scanTestLib())
	got := scanValues(decision, DimAuthor)
	if fmt.Sprint(got) != fmt.Sprint([]string{"瞬間的に絡み合う"}) {
		t.Fatalf("显式行优先且 scan 不抢维度: %v", got)
	}
}

// TestScanDirSourceIsDir 目录段命中的声明来源应为 SourceDir（文件名段为
// SourceFilename）：作者仅出现在目录段（库内无文件名段的任何作者候选）时锚定。
func TestScanDirSourceIsDir(t *testing.T) {
	rule := scanRuleFromJSON(t, scanRuleJSON(`{"dimension": "author", "from": "scan", "scanScope": "path"}`))
	in := &MatchInput{
		FileName: "Isel_瞬間的に絡み合う_01_[中文翻譯][高清].jpg",
		DirNames: []string{"Artist[Beko]"},
		RelPath:  "Artist[Beko]/Isel_瞬間的に絡み合う_01_[中文翻譯][高清].jpg",
	}
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Beko": {}}}
	decision := runScanRule(t, rule, in, lib)
	if len(decision.Declarations) != 1 || decision.Declarations[0].Source != SourceDir {
		t.Fatalf("目录段命中来源应为 SourceDir: %+v", decision.Declarations)
	}
}

// TestScanCaptureSpanBinding 捕获区间绑定：setname 绑定携带 Segment=完整文件名
// 与字节区间（uncovered 排除的数据基座）。
func TestScanCaptureSpanBinding(t *testing.T) {
	rule := scanRuleFromJSON(t, scanRuleJSON(`{"dimension": "tag", "from": "scan"}`))
	in := scanInput(false)
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	if !rule.Pattern.Evaluate(in, scanTestLib(), ctx) {
		t.Fatalf("规则应命中")
	}
	if len(ctx.Captures) != 1 {
		t.Fatalf("应有一条捕获绑定: %+v", ctx.Captures)
	}
	b := ctx.Captures[0]
	if b.Name != "setname" || b.Value != "瞬間的に絡み合う" || b.Segment != in.FileName {
		t.Fatalf("绑定内容不符: %+v", b)
	}
	if b.Start != len("Isel_") || b.End != len("Isel_瞬間的に絡み合う") {
		t.Fatalf("区间不符: [%d,%d)", b.Start, b.End)
	}
}

// TestScanValidation 构造期校验拒绝面：无库集合维度 / 非法范围 / when /
// 携带捕获组名。
func TestScanValidation(t *testing.T) {
	cases := []struct {
		name, extractions, wantErr string
	}{
		{"无库集合维度", `{"dimension": "title", "from": "scan"}`, "无库内集合"},
		{"非法范围", `{"dimension": "tag", "from": "scan", "scanScope": "junk"}`, "范围非法"},
		{"挂条件", `{"dimension": "tag", "from": "scan", "when": {"kind": "and", "subs": []}}`, "暂不支持生效条件"},
		{"带捕获组名", `{"dimension": "tag", "from": "scan", "captureName": "setname"}`, "不应携带捕获组名"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := ParseProfile([]byte(fmt.Sprintf(`{"schemaVersion":3,"rules":[%s]}`, scanRuleJSON(tc.extractions))))
			if len(errs) == 0 {
				t.Fatalf("应被校验拒绝")
			}
			if !strings.Contains(fmt.Sprint(errs), tc.wantErr) {
				t.Fatalf("错误文案不符: %v", errs)
			}
		})
	}
}
