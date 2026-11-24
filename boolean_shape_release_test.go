// 决策10（终审重梳轮）的内核释放锚：解除提取×聚合互斥（聚合规则携提取动作
// 照常逐文件求值、声明挂各命中文件、经主导文件结论并入聚合作品元数据）+
// 聚合/扇出段放开行结构限制（括号子组/行级非——成员判定消费规则完整求值的
// 命中集，行级命中照常按行）+ 平行行等值回归（存量聚合/映射规则的现状形态
// 行命中池 OR 汇聚逐字节等值，连接词不参与成员判定）。
package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// booleanReleaseRun 对文件面执行合并求值+聚合/扇出段（与 fanoutRun 同款装配）。
func booleanReleaseRun(t *testing.T, files []ScanFile, rules []RuleEntry) []FileConclusion {
	t.Helper()
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex(), nil)
	return conclusions
}

// booleanReleaseFinalOf 取结论终选中某维度的值（缺席返回 false）。
func booleanReleaseFinalOf(c *FileConclusion, dim MetadataDimension) (string, bool) {
	for i := range c.Final {
		if c.Final[i].Dimension == dim {
			return c.Final[i].Value, true
		}
	}
	return "", false
}

// booleanReleaseFileCond 构造文件目标条件（模板 + 可选扩展名谓词）——直接返回
// *Condx（测试内联构造件）。
func booleanReleaseFileCond(tpl, ext string) *Condx {
	c := &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}
	if ext != "" {
		c.Attach = []CondxAttach{{Kind: CondxAttachFileExt, Value: ext}}
	}
	return c
}

// TestAggregationExtractionCoexist 聚合规则+提取动作端到端锚（决策10 解除
// 互斥）：档案载入不再拒绝聚合条目携带提取动作（行引用校验照常走）；识别后
// 提取声明挂各命中文件（规则布尔命中面），主导文件的终选结论携带提取维度
// ——消费面经既有「主导文件结论并入作品元数据」通路自然进聚合作品。
func TestAggregationExtractionCoexist(t *testing.T) {
	// 两行视频/字幕条件行（显式 or 连接——布尔命中面=任一行命中），标识={作品}，
	// 分轨=videoMain+subtitle，另携提取动作：标题←第 1 行 {作品} 捕获。
	rj := &ruleEntryJSON{
		ID:          "agg-with-extraction",
		Description: "测试：聚合规则携带提取动作",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			{Condition: booleanReleaseFileCond("{作品}", ".mp4")},
			{Connector: "or", Condition: booleanReleaseFileCond("{作品}.{任意}", ".srt")},
		},
		Identity: []IdentityPart{{Row: 1, Capture: "work"}},
		Aggregate: &AggregateProduction{
			WorkType: sdkdto.ResourceTypeVideo,
			Tracks: []TrackAssign{
				{Row: 1, Role: sdkdto.StoreRoleVideoMain},
				{Row: 2, Role: contract.StoreTypeSubtitle},
			},
		},
		Extractions: []ExtractionAction{
			{Dimension: DimTitle, From: FromCapture, CaptureName: "work", Row: 1},
		},
	}
	// 载入锚：互斥拒绝已除（此前该条目在 validateAggregateEntry 即拒）。
	rule, err := ruleFromJSON(rj)
	if err != nil {
		t.Fatalf("聚合规则携带提取动作应载入（互斥已解除）: %v", err)
	}
	files := aggScanFiles("inner/a.mp4", "inner/a.zh.srt")
	conclusions := booleanReleaseRun(t, files, []RuleEntry{*rule})

	lead := aggConclusionOf(t, conclusions, "inner/a.mp4")
	if lead.AggWorkKey != "f:inner/a.mp4" || lead.AggTrackRole != sdkdto.StoreRoleVideoMain {
		t.Fatalf("视频主导聚合归属不符: key=%q role=%q", lead.AggWorkKey, lead.AggTrackRole)
	}
	// 提取声明挂主导命中文件、终选携带标题维度（主导文件结论=聚合作品元数据面）。
	if v, ok := booleanReleaseFinalOf(lead, DimTitle); !ok || v != "a" {
		t.Fatalf("主导文件终选应携带提取标题（期望 a）: %q %v", v, ok)
	}
	if !containsString(lead.MatchedRuleIDs, rule.ID) {
		t.Fatalf("聚合规则命中应记入 MatchedRuleIDs（提取段不再按类别跳过）: %v", lead.MatchedRuleIDs)
	}
	// 字幕轨成员：规则布尔命中（or 面）但第 1 行捕获不可得——零提取声明，
	// 轨道归属照常。
	sub := aggConclusionOf(t, conclusions, "inner/a.zh.srt")
	if sub.AggWorkKey != "f:inner/a.mp4" || sub.AggTrackRole != contract.StoreTypeSubtitle {
		t.Fatalf("字幕轨聚合归属不符: key=%q role=%q", sub.AggWorkKey, sub.AggTrackRole)
	}
	if _, ok := booleanReleaseFinalOf(sub, DimTitle); ok {
		t.Fatalf("字幕行第 1 行捕获不可得，不应产标题声明")
	}
}

// TestAggregationBooleanShapeGate 布尔组合（括号子组+行级非）聚合规则锚：括号
// 子组内条件行照常按行寻址（分轨/标识），整条布尔求值为门——门不过的文件
// 不入命中集（维持独立作品）。
func TestAggregationBooleanShapeGate(t *testing.T) {
	// 结构：〔{作品}.mp4 或 {作品}.srt〕 且 非 {任意}.md——括号子组两行为分轨
	// （videoMain 必含+subtitle），行级非行为过滤行（不分轨不绑定）。
	rj := &ruleEntryJSON{
		ID:          "agg-boolean-gate",
		Description: "测试：布尔组合聚合",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			{Group: []CondxRow{
				{Condition: booleanReleaseFileCond("{作品}", ".mp4")},
				{Connector: "or", Condition: booleanReleaseFileCond("{作品}.{任意}", ".srt")},
			}},
			{Connector: "and", Negated: true, Condition: booleanReleaseFileCond("{任意}", ".md")},
		},
		Identity: []IdentityPart{{Row: 1, Capture: "work"}},
		Aggregate: &AggregateProduction{
			WorkType: sdkdto.ResourceTypeVideo,
			Tracks: []TrackAssign{
				{Row: 1, Role: sdkdto.StoreRoleVideoMain},
				{Row: 2, Role: contract.StoreTypeSubtitle},
			},
		},
	}
	rule, err := ruleFromJSON(rj)
	if err != nil {
		t.Fatalf("布尔组合聚合规则应载入（括号/行级非已放开）: %v", err)
	}
	files := aggScanFiles("d/a.mp4", "d/a.zh.srt", "d/草稿.md")
	conclusions := booleanReleaseRun(t, files, []RuleEntry{*rule})

	video := aggConclusionOf(t, conclusions, "d/a.mp4")
	if video.AggWorkKey != "f:d/a.mp4" || video.AggTrackRole != sdkdto.StoreRoleVideoMain {
		t.Fatalf("括号内视频行的主导归属不符: key=%q role=%q", video.AggWorkKey, video.AggTrackRole)
	}
	sub := aggConclusionOf(t, conclusions, "d/a.zh.srt")
	if sub.AggWorkKey != "f:d/a.mp4" || sub.AggTrackRole != contract.StoreTypeSubtitle {
		t.Fatalf("括号内字幕行的轨道归属不符（组内行照常按行寻址）: key=%q role=%q", sub.AggWorkKey, sub.AggTrackRole)
	}
	// 门排除锚：草稿.md 命中行级非行内层（{任意}.md 命中→非=false）→ 整条
	// 求值不过→不入命中集，维持独立作品。
	draft := aggConclusionOf(t, conclusions, "d/草稿.md")
	if draft.AggWorkKey != "" || draft.AggTrackRole != "" {
		t.Fatalf("布尔门不过的文件应维持独立作品: key=%q role=%q", draft.AggWorkKey, draft.AggTrackRole)
	}
}

// TestFanoutBooleanShapeGate 布尔组合（括号子组+行级非）映射规则锚：绑定行/
// 目标行照常按行（括号内层行占号）；整条布尔求值为门——门不过的说明文件
// 不入分组命中集、按独立作品导入（孤文件回退）。
func TestFanoutBooleanShapeGate(t *testing.T) {
	// 结构：〔{作品}.png 或 {任意}〕 且 非 {任意}.md——行1 目标（组内展平）、
	// 行2 绑定、行3 行级非（.md 说明文件被门排除）、行4 目录行=所在目录分组
	// 的产生行（键集=门内命中文件的所在目录）。
	rule := RuleEntry{
		ID:          "map-boolean-gate",
		Description: "测试：布尔组合扇出",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Group: []CondxRow{
				{Condition: booleanReleaseFileCond("{作品}", ".png")},
				{Connector: "or", Condition: booleanReleaseFileCond("{任意}", "")},
			}},
			{Connector: "and", Negated: true, Condition: booleanReleaseFileCond("{任意}", ".md")},
			{Connector: "and", Condition: fanoutAnyDirRow().Condition},
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory, Row: 4},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles("单图/cover.png", "单图/说明.txt", "单图/备注.md")
	conclusions := booleanReleaseRun(t, files, []RuleEntry{rule})

	cover := aggConclusionOf(t, conclusions, "单图/cover.png")
	if cover.AggWorkKey != "" || cover.AggCompanion {
		t.Fatalf("目标文件应维持独立作品: key=%q companion=%v", cover.AggWorkKey, cover.AggCompanion)
	}
	paired := aggConclusionOf(t, conclusions, "单图/说明.txt")
	if paired.AggWorkKey != "" || paired.AggTrackRole != "" || !paired.AggCompanion ||
		!reflect.DeepEqual(paired.FanoutWorkKeys, []string{"f:单图/cover.png"}) {
		t.Fatalf("门内说明文件应为元数据源（零归属键+扇出到组内唯一作品）: key=%q role=%q companion=%v fanout=%v",
			paired.AggWorkKey, paired.AggTrackRole, paired.AggCompanion, paired.FanoutWorkKeys)
	}
	// 门排除锚：备注.md 命中行级非内层（.md）→ 整条求值不过——纯平行语义下
	// 它是行2 命中的说明成员候选（会被扇出收编），布尔门令其按独立作品导入。
	note := aggConclusionOf(t, conclusions, "单图/备注.md")
	if note.AggWorkKey != "" || note.AggCompanion || len(note.FanoutWorkKeys) != 0 {
		t.Fatalf("布尔门不过的说明文件应按独立作品导入: key=%q companion=%v fanout=%v",
			note.AggWorkKey, note.AggCompanion, note.FanoutWorkKeys)
	}
}

// TestGroupParallelRowsEquivalenceAnchor 平行行为等值回归锚（关键回归约束）：
// 纯平行行（无括号无行级非——存量聚合/映射规则的现状形态）的成员判定=行
// 命中池 OR 汇聚，行前连接词（空串/and/or）不参与成员判定——三种连接词
// 书写的同构规则产出逐字节等值的聚合/扇出结论（求值器接线不改变平行语义）。
func TestGroupParallelRowsEquivalenceAnchor(t *testing.T) {
	files := aggScanFiles("x/a.mp4", "x/a.zh.srt", "x/b.mp4")
	build := func(id, secondConnector string) RuleEntry {
		return RuleEntry{
			ID:          id,
			Description: "测试：平行行等值",
			Source:      RuleSourceManual,
			Enabled:     true,
			Category:    RuleCategoryAggregate,
			Conditions: []CondxRow{
				{Condition: booleanReleaseFileCond("{作品}", ".mp4")},
				{Connector: secondConnector, Condition: booleanReleaseFileCond("{作品}.{任意}", ".srt")},
			},
			Identity: []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{
				WorkType: sdkdto.ResourceTypeVideo,
				Tracks: []TrackAssign{
					{Row: 1, Role: sdkdto.StoreRoleVideoMain},
					{Row: 2, Role: contract.StoreTypeSubtitle},
				},
			},
		}
	}
	snapshotOf := func(rule RuleEntry) string {
		conclusions := booleanReleaseRun(t, files, []RuleEntry{rule})
		var sb strings.Builder
		for _, rel := range []string{"x/a.mp4", "x/a.zh.srt", "x/b.mp4"} {
			c := aggConclusionOf(t, conclusions, rel)
			sb.WriteString(rel + "|" + c.AggWorkKey + "|" + c.AggTrackRole + "|" + c.AggWorkType + "\n")
		}
		return sb.String()
	}
	want := snapshotOf(build("agg-parallel-empty", ""))
	// 形态前提：a 组成立（主导 a.mp4 + 字幕轨）、b 单视频成立——与既有行为一致。
	if !strings.Contains(want, "x/a.mp4|f:x/a.mp4|"+sdkdto.StoreRoleVideoMain+"|"+sdkdto.ResourceTypeVideo) ||
		!strings.Contains(want, "x/a.zh.srt|f:x/a.mp4|"+contract.StoreTypeSubtitle) ||
		!strings.Contains(want, "x/b.mp4|f:x/b.mp4|") {
		t.Fatalf("平行形态基准结论不符（应与既有行为一致）: %s", want)
	}
	for _, connector := range []string{"and", "or"} {
		if got := snapshotOf(build("agg-parallel-"+connector, connector)); got != want {
			t.Fatalf("连接词 %q 与空串形态的平行行结论应逐字节等值:\n%s\n---\n%s", connector, got, want)
		}
	}
}
