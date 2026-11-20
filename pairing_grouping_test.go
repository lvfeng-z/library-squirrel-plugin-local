// 元数据文件扇出归属的单元测试：映射单用（无聚合产出）按分组绑定求值分组，
// 组内目标行命中文件推出作品键集，绑定行文件=元数据源（内容声明扇出到键集
// 全部作品、不导入不独立成件），孤文件回退（组内无落点按独立作品导入）、
// 聚合归属优先（扇出不夺已归属成员）、并用形态（聚合成员面=轨行∪绑定行、
// 双重身份内容同样扇出）、分组绑定的载入校验、旧档归一（aggregate.
// contentBindings 与 metadataFile.pairing 两形态）载入零迁移零拒绝与往返
// 等值、验收1 形态（前缀分组键两 png+txt：两 png 各得描述声明）。
package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// —— 测试构造件 ——

// fanoutFileRow 构造文件目标条件行（模板 + 可选扩展名谓词）。
func fanoutFileRow(tpl, ext string) CondxRow {
	row := CondxRow{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}
	if ext != "" {
		row.Condition.Attach = []CondxAttach{{Kind: CondxAttachFileExt, Value: ext}}
	}
	return row
}

// fanoutMappingRule 构造映射单用规则（元数据类别、无聚合产出）：两行条件
// （资源行+说明行），说明行绑定→维度，分组绑定按入参声明。
func fanoutMappingRule(id string, grouping *GroupingBinding, bindings ...ContentBinding) RuleEntry {
	return RuleEntry{
		ID:          id,
		Description: "测试：映射单用规则",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}_{序号}", ".png"),
			fanoutFileRow("{作品}_{序号}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        grouping,
			ContentBindings: bindings,
		},
	}
}

// fanoutRun 对文件面执行合并求值+聚合段/扇出归属段，返回结论。
func fanoutRun(t *testing.T, files []ScanFile, rules []RuleEntry) []FileConclusion {
	t.Helper()
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex())
	return conclusions
}

// fanoutKeysOf 断言助取：结论的内容扇出键集（nil 安全比较用）。
func fanoutKeysOf(c *FileConclusion) []string {
	if c == nil {
		return nil
	}
	return c.FanoutWorkKeys
}

// —— 标识分组与扇出 ——

// TestFanoutIdentityGrouping 标识分组锚（剧本B 帧1 形态）：分组键=资源行声明
// 的复合标识（{作品}+{序号}，共享捕获跨行汇流——说明文件从自己的命中行取
// 同名捕获），捕获值相等的文件归为一组；组内目标行命中文件各自成一件独立
// 作品（扇出不并作品、不获归属），说明文件=元数据源（不导入、不独立成件、
// 带说明成员标记、内容扇出键集=组内全部作品键）；组内无目标行命中（成员
// 全部绑定行命中）孤文件回退——说明文件按独立作品导入；不命中任何条件行
// 的文件不参与分组（分组键不可得同回退）。
func TestFanoutIdentityGrouping(t *testing.T) {
	rule := fanoutMappingRule("map-seq",
		&GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
			{Row: 1, Capture: "work"},
			{Row: 1, Capture: "seq"},
		}},
		ContentBinding{Row: 2, Dimension: DimDescription},
	)
	files := aggScanFiles(
		"无题/无题_01.png", "无题/无题_02.png", "无题/无题_03.png",
		"无题/无题_01.txt", "无题/无题_02.txt", "无题/无题_03.txt",
		"无题/无题_04.txt", // 分组键可得但组内无目标行命中（成员全部绑定行命中）
		"无题/无题.txt",    // 不命中任何条件行（分组键不可得）
	)
	conclusions := fanoutRun(t, files, []RuleEntry{rule})

	for _, png := range []string{"无题_01.png", "无题_02.png", "无题_03.png"} {
		got := aggConclusionOf(t, conclusions, "无题/"+png)
		if got.AggWorkKey != "" || got.AggTrackRole != "" || got.AggWorkType != "" || got.AggCompanion ||
			len(got.FanoutWorkKeys) != 0 {
			t.Fatalf("目标文件 %s 应各自独立成件（扇出不并作品、不参与内容扇出）: key=%q role=%q type=%q companion=%v fanout=%v",
				png, got.AggWorkKey, got.AggTrackRole, got.AggWorkType, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
	for i := 1; i <= 3; i++ {
		got := aggConclusionOf(t, conclusions, "无题/无题_0"+string(rune('0'+i))+".txt")
		want := []string{"f:无题/无题_0" + string(rune('0'+i)) + ".png"}
		if got.AggWorkKey != "" || got.AggTrackRole != "" || got.AggWorkType != "" || !got.AggCompanion ||
			!reflect.DeepEqual(fanoutKeysOf(got), want) {
			t.Fatalf("说明文件 %d 应为元数据源（零归属键+说明成员标记+内容扇出到组内唯一作品）: key=%q role=%q type=%q companion=%v fanout=%v",
				i, got.AggWorkKey, got.AggTrackRole, got.AggWorkType, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
	// 孤文件回退：无题_04.txt 的分组键（无题,04）无目标文件同组——按独立
	// 作品导入，不作元数据源。
	lone := aggConclusionOf(t, conclusions, "无题/无题_04.txt")
	if lone.AggWorkKey != "" || lone.AggCompanion || len(fanoutKeysOf(lone)) != 0 {
		t.Fatalf("组内无落点的说明文件应按独立作品导入（孤文件回退）: key=%q companion=%v fanout=%v",
			lone.AggWorkKey, lone.AggCompanion, lone.FanoutWorkKeys)
	}
	// 不命中任何条件行的文件不参与分组（组级说明文件独立成件现状）。
	stray := aggConclusionOf(t, conclusions, "无题/无题.txt")
	if stray.AggWorkKey != "" || stray.AggCompanion || len(fanoutKeysOf(stray)) != 0 {
		t.Fatalf("不命中条件行的文件不应参与分组: key=%q companion=%v fanout=%v",
			stray.AggWorkKey, stray.AggCompanion, stray.FanoutWorkKeys)
	}
}

// TestFanoutDoubleIdentityMultiTarget 双重身份与一 txt 多作品扇出锚：分组键
// 取 {作品}（去 {序号}），绑定行不带扩展名谓词（任意扩展名命中）——资源文件
// 兼命中资源行与绑定行=双重身份（照常承载作品、不带说明成员标记，内容同样
// 扇出到组内全部作品）；仅绑定行命中的说明文件扇出到同组两件独立作品（键集
// 两键——内容各带一份）。
func TestFanoutDoubleIdentityMultiTarget(t *testing.T) {
	// 资源行 {作品}_{序号} 限 .png；绑定行 {作品}_{序号} 不限扩展名——两张
	// png 兼命中两行=双重身份，说明 txt 仅命中绑定行=元数据源。
	rule := RuleEntry{
		ID:          "map-multi",
		Description: "测试：一 txt 多作品扇出",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}_{序号}", ".png"),
			{Connector: "and", Condition: &Condx{
				Target: CondxTargetFile,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}_{序号}"},
			}},
		},
		MetadataFile: &MetadataFileProduction{
			Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
				{Row: 1, Capture: "work"},
			}},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	want := []string{"f:集/集_01.png", "f:集/集_02.png"}
	files := aggScanFiles("集/集_01.png", "集/集_02.png", "集/集_A.txt")
	conclusions := fanoutRun(t, files, []RuleEntry{rule})

	// 双重身份资源文件：照常各自承载作品（独立成件）、不带说明成员标记，
	// 内容扇出到组内全部两件作品。
	for _, png := range []string{"集/集_01.png", "集/集_02.png"} {
		got := aggConclusionOf(t, conclusions, png)
		if got.AggWorkKey != "" || got.AggCompanion || !reflect.DeepEqual(fanoutKeysOf(got), want) {
			t.Fatalf("双重身份文件 %s 应照常承载作品+内容扇出到组内全部作品: key=%q companion=%v fanout=%v",
				png, got.AggWorkKey, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
	// 仅绑定行命中的说明文件：元数据源（零归属键+说明成员标记+扇出到两件
	// 作品——各带一份）。
	member := aggConclusionOf(t, conclusions, "集/集_A.txt")
	if member.AggWorkKey != "" || member.AggTrackRole != "" || member.AggWorkType != "" || !member.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(member), want) {
		t.Fatalf("仅绑定行命中的说明文件应为元数据源（零归属键+说明成员标记+扇出到两件作品）: key=%q role=%q type=%q companion=%v fanout=%v",
			member.AggWorkKey, member.AggTrackRole, member.AggWorkType, member.AggCompanion, member.FanoutWorkKeys)
	}
}

// —— 所在目录分组 ——

// TestFanoutDirectoryGrouping 所在目录分组正反例锚：同目录的目标行命中文件
// 各自成作品，说明内容扇出给同目录全部作品——唯一目标的目录=一对一扇出；
// 多目标的目录=一份说明内容各带一份（指向集守卫已随成员化语义退役，多作品
// 组不再整组拒绝），目标候选维持独立作品不受扇出影响。
func TestFanoutDirectoryGrouping(t *testing.T) {
	rule := RuleEntry{
		ID:          "map-dir",
		Description: "测试：所在目录分组",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}", ".jpg"),
			fanoutFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles(
		"单图/cover.jpg", "单图/说明.txt", // 唯一目标：一对一扇出
		"双图/a.jpg", "双图/b.jpg", "双图/说明.txt", // 两目标：各带一份
	)
	conclusions := fanoutRun(t, files, []RuleEntry{rule})

	cover := aggConclusionOf(t, conclusions, "单图/cover.jpg")
	if cover.AggWorkKey != "" || cover.AggCompanion {
		t.Fatalf("目录分组的目标文件应维持独立作品: key=%q companion=%v",
			cover.AggWorkKey, cover.AggCompanion)
	}
	paired := aggConclusionOf(t, conclusions, "单图/说明.txt")
	if paired.AggWorkKey != "" || paired.AggTrackRole != "" || !paired.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(paired), []string{"f:单图/cover.jpg"}) {
		t.Fatalf("唯一目标目录的说明文件应扇出到该作品: key=%q role=%q companion=%v fanout=%v",
			paired.AggWorkKey, paired.AggTrackRole, paired.AggCompanion, paired.FanoutWorkKeys)
	}
	// 多目标目录：说明文件扇出到两件作品（各带一份），目标候选不受影响。
	for _, jpg := range []string{"双图/a.jpg", "双图/b.jpg"} {
		got := aggConclusionOf(t, conclusions, jpg)
		if got.AggWorkKey != "" || got.AggCompanion {
			t.Fatalf("多目标目录的目标候选 %s 应不受扇出影响: key=%q companion=%v",
				jpg, got.AggWorkKey, got.AggCompanion)
		}
	}
	multi := aggConclusionOf(t, conclusions, "双图/说明.txt")
	if multi.AggWorkKey != "" || !multi.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(multi), []string{"f:双图/a.jpg", "f:双图/b.jpg"}) {
		t.Fatalf("多目标目录的说明文件应扇出到组内全部两件作品: key=%q companion=%v fanout=%v",
			multi.AggWorkKey, multi.AggCompanion, multi.FanoutWorkKeys)
	}
}

// TestFanoutNoTargetGroupFallsBack 孤文件回退锚（目录分组面）：目录内全部
// 参与文件都是绑定行命中（组内无目标行命中=内容无落点）→ 说明文件按独立
// 作品导入，不作元数据源。
func TestFanoutNoTargetGroupFallsBack(t *testing.T) {
	rule := RuleEntry{
		ID:          "map-nofall",
		Description: "测试：无落点目录",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}", ".jpg"),
			fanoutFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles("只有说明/说明.txt", "只有说明/备注.txt")
	conclusions := fanoutRun(t, files, []RuleEntry{rule})
	for _, rel := range []string{"只有说明/说明.txt", "只有说明/备注.txt"} {
		got := aggConclusionOf(t, conclusions, rel)
		if got.AggWorkKey != "" || got.AggCompanion || len(fanoutKeysOf(got)) != 0 {
			t.Fatalf("无落点目录的说明文件应按独立作品导入（孤文件回退）: %s key=%q companion=%v fanout=%v",
				rel, got.AggWorkKey, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
}

// —— 与聚合段的次序与并用 ——

// TestFanoutAggregatePriorityOverMapping 聚合归属优先锚：聚合规则先归属
// （聚合段在前），扇出归属不夺已归属成员——即便映射规则在档案序居前，已被
// 聚合收编的文件（含其绑定行命中的说明文件）不再被认领为元数据源，聚合
// 结论原样保持、零扇出键集。
func TestFanoutAggregatePriorityOverMapping(t *testing.T) {
	mapping := RuleEntry{
		ID:          "map-first",
		Description: "测试：档案序居前的映射规则",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}", ".jpg"),
			fanoutFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	article := RuleEntry{
		ID:          "agg-article",
		Description: "测试：图文聚合",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}", ".txt"),
			fanoutFileRow("{作品}", ".jpg"),
		},
		Identity: []IdentityPart{{Row: 1, Capture: "work"}},
		Aggregate: &AggregateProduction{
			WorkType: sdkdto.ResourceTypeArticle,
			Tracks: []TrackAssign{
				{Row: 1, Role: sdkdto.StoreRoleDocument},
				{Row: 2, Role: contract.StoreTypeImage},
			},
		},
	}
	files := aggScanFiles("d/x.jpg", "d/x.txt")
	conclusions := fanoutRun(t, files, []RuleEntry{mapping, article})

	txt := aggConclusionOf(t, conclusions, "d/x.txt")
	if txt.AggWorkKey != "f:d/x.txt" || txt.AggTrackRole != sdkdto.StoreRoleDocument || txt.AggCompanion ||
		len(fanoutKeysOf(txt)) != 0 {
		t.Fatalf("同名对应由聚合归属优先收编（正文轨、零扇出——扇出不夺已归属成员）: key=%q role=%q companion=%v fanout=%v",
			txt.AggWorkKey, txt.AggTrackRole, txt.AggCompanion, txt.FanoutWorkKeys)
	}
	jpg := aggConclusionOf(t, conclusions, "d/x.jpg")
	if jpg.AggWorkKey != "f:d/x.txt" || jpg.AggTrackRole != contract.StoreTypeImage || jpg.AggCompanion {
		t.Fatalf("同名对的图片应由聚合收编（内嵌图轨）: key=%q role=%q companion=%v",
			jpg.AggWorkKey, jpg.AggTrackRole, jpg.AggCompanion)
	}
}

// TestFanoutAlongsideAggregateFanoutAcrossWorks 并用形态锚：聚合成员面=轨行
// ∪绑定行（仅绑定行命中的文件=聚合作品的说明成员——归进作品、无轨）；分组
// 绑定独立于聚合标识（本例显式绑所在目录）时，绑定行文件的内容扇出到分组
// 内全部作品——轨行兼绑定的双重身份轨成员内容同样扇出（本组两件视频作品
// 各收一份），轨道结论与聚合归属不受扇出影响。
func TestFanoutAlongsideAggregateFanoutAcrossWorks(t *testing.T) {
	rule := RuleEntry{
		ID:          "agg-with-grouping",
		Description: "测试：并用扇出跨作品",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}", ".mp4"),
			fanoutFileRow("{作品}.{任意}", ".srt"),
		},
		Identity: []IdentityPart{{Row: 1, Capture: "work"}},
		Aggregate: &AggregateProduction{
			WorkType: sdkdto.ResourceTypeVideo,
			Tracks: []TrackAssign{
				{Row: 1, Role: sdkdto.StoreRoleVideoMain},
				{Row: 2, Role: contract.StoreTypeSubtitle},
			},
		},
		MetadataFile: &MetadataFileProduction{
			// 分组绑定与聚合标识的取值形态不同（所在目录）——绑定行兼为
			// 轨行（双重身份），内容扇出到目录内两件视频作品。
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles("d/a.mp4", "d/a.zh.srt", "d/b.mp4")
	conclusions := fanoutRun(t, files, []RuleEntry{rule})

	video := aggConclusionOf(t, conclusions, "d/a.mp4")
	if video.AggWorkKey != "f:d/a.mp4" || video.AggTrackRole != sdkdto.StoreRoleVideoMain || video.AggCompanion {
		t.Fatalf("并用时视频主导归属应不受扇出影响: key=%q role=%q companion=%v",
			video.AggWorkKey, video.AggTrackRole, video.AggCompanion)
	}
	sub := aggConclusionOf(t, conclusions, "d/a.zh.srt")
	if sub.AggWorkKey != "f:d/a.mp4" || sub.AggTrackRole != contract.StoreTypeSubtitle || sub.AggCompanion {
		t.Fatalf("并用时字幕轨归属应不受扇出影响: key=%q role=%q companion=%v",
			sub.AggWorkKey, sub.AggTrackRole, sub.AggCompanion)
	}
	if !reflect.DeepEqual(fanoutKeysOf(sub), []string{"f:d/a.mp4", "f:d/b.mp4"}) {
		t.Fatalf("双重身份轨成员的内容应扇出到目录分组内全部两件作品: %v", sub.FanoutWorkKeys)
	}
	other := aggConclusionOf(t, conclusions, "d/b.mp4")
	if other.AggWorkKey != "f:d/b.mp4" || other.AggTrackRole != sdkdto.StoreRoleVideoMain {
		t.Fatalf("并用时异标识视频应各自成件: key=%q role=%q",
			other.AggWorkKey, other.AggTrackRole)
	}
}

// —— 载入校验 ——

// fanoutRuleJSON 构造映射单用条目的中间形态（元数据类别、两行条件、说明行
// 绑定→描述），分组绑定按入参。
func fanoutRuleJSON(id string, grouping *GroupingBinding) *ruleEntryJSON {
	row2 := fanoutFileRow("{作品}_{序号}", ".txt")
	row2.Connector = "and"
	return &ruleEntryJSON{
		ID:          id,
		Description: "测试：映射单用条目",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}_{序号}", ".png"),
			row2,
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        grouping,
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
}

// TestFanoutLoadValidation 分组绑定声明的载入校验：映射单用须绑定分组；形态
// 枚举合法（identity=标识分组 / directory=所在目录）；标识分组须携带键
// 部分、部分行号与捕获名引用存在（序列群键保留名只选自序列目标行）；所在
// 目录分组不得携带键部分；并用（聚合在场）时分组绑定可省略——缺省不在载入
// 侧物化（求值期按聚合成作品所绑分组取用），声明在场须合法。
func TestFanoutLoadValidation(t *testing.T) {
	identity := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 1, Capture: "work"}, {Row: 1, Capture: "seq"},
	}}
	// 正例：映射单用（元数据类别）携分组绑定与绑定，载入通过。
	entry, err := ruleFromJSON(fanoutRuleJSON("map-ok", identity))
	if err != nil {
		t.Fatalf("映射单用条目应载入: %v", err)
	}
	if entry.MetadataFile == nil || entry.MetadataFile.Grouping.Mode != GroupingModeIdentity ||
		len(entry.MetadataFile.Grouping.Identity) != 2 || len(entry.MetadataFile.ContentBindings) != 1 {
		t.Fatalf("分组绑定与内容绑定应原样载入: %+v", entry.MetadataFile)
	}

	cases := []struct {
		name     string
		grouping *GroupingBinding
		wantSub  string
	}{
		{"映射单用缺分组绑定", nil, "未绑定分组"},
		{"标识分组缺键部分", &GroupingBinding{Mode: GroupingModeIdentity}, "缺少键部分"},
		{"所在目录分组带键部分", &GroupingBinding{Mode: GroupingModeDirectory, Identity: identity.Identity}, "不得携带键部分"},
		{"分组绑定形态非法", &GroupingBinding{Mode: GroupingMode("bogus")}, "分组绑定形态非法"},
		{"键部分行号越界", &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 3, Capture: "work"}}}, "越界"},
		{"捕获名未声明", &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "bogus"}}}, "未在任何条件行声明"},
		{"序列群键选自非序列行", &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}}}, "只能选自序列目标行"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ruleFromJSON(fanoutRuleJSON("map-bad", tc.grouping))
			if err == nil {
				t.Fatal("非法分组绑定应载入拒绝")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("错误文案应含 %q: %v", tc.wantSub, err)
			}
		})
	}

	// 并用（聚合在场）时分组绑定可省略——载入不物化（缺省绑聚合成作品所绑
	// 分组是求值期便利缺省，档案保持缺省形态）；声明在场须合法。
	mixed := fanoutRuleJSON("map-agg-ok", nil)
	mixed.Category = RuleCategoryAggregate
	mixed.Identity = []IdentityPart{{Row: 1, Capture: "work"}}
	mixed.Aggregate = &AggregateProduction{
		WorkType: sdkdto.ResourceTypeImage,
		Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}},
	}
	mixedEntry, err := ruleFromJSON(mixed)
	if err != nil {
		t.Fatalf("并用条目省略分组绑定应载入（求值期缺省）: %v", err)
	}
	if mixedEntry.MetadataFile == nil || mixedEntry.MetadataFile.Grouping != nil {
		t.Fatalf("并用缺省分组绑定不应在载入侧物化: %+v", mixedEntry.MetadataFile)
	}
	// 缺省取用核验：同一文件面下，缺省形态与显式声明聚合标识的分组绑定产出
	// 等值（求值期缺省=聚合成作品所绑分组——说明文件经聚合标识入组为聚合成
	// 作品的说明成员，内容扇出键集=该聚合作品键）。
	files := aggScanFiles("无题/无题_01.png", "无题/无题_01.txt")
	explicit := fanoutRuleJSON("map-agg-explicit", &GroupingBinding{Mode: GroupingModeIdentity,
		Identity: []IdentityPart{{Row: 1, Capture: "work"}}})
	explicit.Category = RuleCategoryAggregate
	explicit.Identity = []IdentityPart{{Row: 1, Capture: "work"}}
	explicit.Aggregate = mixed.Aggregate
	explicitEntry, err := ruleFromJSON(explicit)
	if err != nil {
		t.Fatalf("并用条目显式声明分组绑定应载入: %v", err)
	}
	defConclusions := fanoutRun(t, files, []RuleEntry{*mixedEntry})
	expConclusions := fanoutRun(t, files, []RuleEntry{*explicitEntry})
	defTxt := aggConclusionOf(t, defConclusions, "无题/无题_01.txt")
	expTxt := aggConclusionOf(t, expConclusions, "无题/无题_01.txt")
	if !defTxt.AggCompanion || defTxt.AggWorkKey != "f:无题/无题_01.png" ||
		defTxt.AggWorkType != sdkdto.ResourceTypeImage ||
		!reflect.DeepEqual(fanoutKeysOf(defTxt), []string{"f:无题/无题_01.png"}) {
		t.Fatalf("并用缺省分组下说明文件应为聚合作品说明成员+扇出: key=%q type=%q companion=%v fanout=%v",
			defTxt.AggWorkKey, defTxt.AggWorkType, defTxt.AggCompanion, defTxt.FanoutWorkKeys)
	}
	if !reflect.DeepEqual(fanoutKeysOf(defTxt), fanoutKeysOf(expTxt)) || defTxt.AggWorkKey != expTxt.AggWorkKey {
		t.Fatalf("缺省分组与显式声明的扇出结论应等值: %+v vs %+v", defTxt, expTxt)
	}
}

// —— 旧档归一 ——

// legacyBindingProfileJSON 旧档原文一（绑定住聚合产出内的 contentBindings 键
// ——绑定迁至规则级落档位前的形态；无分组绑定——并用缺省的旧档形态）。
const legacyBindingProfileJSON = `{"schemaVersion":5,"rules":[
{"id":"legacy-agg","description":"旧档并用规则","source":"manual","enabled":true,"category":"aggregate",
 "conditions":[
  {"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}"},"attach":[{"kind":"fileExt","value":".jpg"}]}},
  {"condition":{"connector":"and","target":"file","format":{"mode":"template","tpl":"{作品}"},"attach":[{"kind":"fileExt","value":".txt"}]}}],
 "identity":[{"row":1,"capture":"work"}],
 "aggregate":{"workType":"image","tracks":[{"row":1,"role":"image"}],
  "contentBindings":[{"row":2,"dimension":"description"}]}}]}`

// legacyPairingProfileJSON 旧档原文二（元数据文件产出携带旧档配对键
// metadataFile.pairing——分组绑定改键落档前的形态；键形与新形态同构）。
const legacyPairingProfileJSON = `{"schemaVersion":5,"rules":[
{"id":"legacy-pairing","description":"旧档配对键规则","source":"manual","enabled":true,
 "conditions":[
  {"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}_{序号}"},"attach":[{"kind":"fileExt","value":".png"}]}},
  {"connector":"and","condition":{"target":"file","format":{"mode":"template","tpl":"{作品}_{序号}"},"attach":[{"kind":"fileExt","value":".txt"}]}}],
 "metadataFile":{"pairing":{"mode":"identity","identity":[{"row":1,"capture":"work"},{"row":1,"capture":"seq"}]},
  "contentBindings":[{"row":2,"dimension":"description"}]}}]}`

// TestFanoutLegacyArchiveNormalization 旧档归一锚：含 aggregate.contentBindings
// 或 metadataFile.pairing 的规则档案载入归一到规则级「元数据文件」新键位
// （contentBindings 与 grouping——零迁移零拒绝，不写回不拒收，载入即归一）；
// 并用缺省分组绑定不物化（旧档绑定行经聚合标识入组=缺省绑聚合所绑分组的
// 效果）；归一后条目与全新落档形态往返等值（保存只出新键位）；求值行为=
// 绑定行成员照常归进聚合作品+内容扇出；新旧键位双写形态载入拒绝。
func TestFanoutLegacyArchiveNormalization(t *testing.T) {
	profile, errs := ParseProfile([]byte(legacyBindingProfileJSON))
	if len(errs) != 0 {
		t.Fatalf("旧档绑定档案应零拒绝载入: %v", errs)
	}
	if len(profile.Rules) != 1 {
		t.Fatalf("应恰载入一条规则: %d", len(profile.Rules))
	}
	entry := &profile.Rules[0]
	if entry.Aggregate == nil || entry.Aggregate.WorkType != sdkdto.ResourceTypeImage {
		t.Fatalf("聚合产出应原样保留: %+v", entry.Aggregate)
	}
	// 旧档绑定上缴到规则级落档位；并用缺省分组绑定不物化（求值期按聚合成
	// 作品所绑分组取用）。
	if entry.MetadataFile == nil || len(entry.MetadataFile.ContentBindings) != 1 ||
		entry.MetadataFile.ContentBindings[0].Row != 2 ||
		entry.MetadataFile.ContentBindings[0].Dimension != DimDescription ||
		entry.MetadataFile.Grouping != nil {
		t.Fatalf("旧档绑定应归一到规则级落档位且分组绑定缺省不物化: %+v", entry.MetadataFile)
	}

	// 归一载入后的求值行为：同名图片+文本对=图片聚合作品+说明成员（绑定行
	// 经聚合标识入组归进作品），内容扇出键集=该聚合作品键。
	files := aggScanFiles("img/无题.jpg", "img/无题.txt")
	conclusions := fanoutRun(t, files, profile.Rules)
	lead := aggConclusionOf(t, conclusions, "img/无题.jpg")
	member := aggConclusionOf(t, conclusions, "img/无题.txt")
	if lead.AggWorkKey != "f:img/无题.jpg" || lead.AggTrackRole != contract.StoreTypeImage || lead.AggCompanion {
		t.Fatalf("归一条目的主导归属应照常: key=%q role=%q companion=%v",
			lead.AggWorkKey, lead.AggTrackRole, lead.AggCompanion)
	}
	if member.AggWorkKey != "f:img/无题.jpg" || member.AggTrackRole != "" || !member.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(member), []string{"f:img/无题.jpg"}) {
		t.Fatalf("归一条目的说明成员应照常归进聚合作品且内容扇出: key=%q role=%q companion=%v fanout=%v",
			member.AggWorkKey, member.AggTrackRole, member.AggCompanion, member.FanoutWorkKeys)
	}

	// 往返等值：保存只出新键位（绑定住 metadataFile.contentBindings、聚合
	// 产出不再携带绑定键、不落旧配对键），重载后条目与归一形态等值。
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("档案序列化失败: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("归一保存产物解析失败: %v", err)
	}
	rules := raw["rules"].([]any)
	aggObj := rules[0].(map[string]any)["aggregate"].(map[string]any)
	if _, ok := aggObj["contentBindings"]; ok {
		t.Fatalf("聚合产出不应再携带绑定键: %v", aggObj)
	}
	mfObj := rules[0].(map[string]any)["metadataFile"].(map[string]any)
	if bindings, ok := mfObj["contentBindings"].([]any); !ok || len(bindings) != 1 {
		t.Fatalf("规则级落档位应携带绑定: %v", mfObj)
	}
	if _, ok := mfObj["pairing"]; ok {
		t.Fatalf("保存产物不应再落旧配对键: %v", mfObj)
	}
	profile2, errs := ParseProfile(data)
	if len(errs) != 0 {
		t.Fatalf("归一保存产物应零拒绝载入: %v", errs)
	}
	if !reflect.DeepEqual(profile.Rules[0].MetadataFile, profile2.Rules[0].MetadataFile) ||
		!reflect.DeepEqual(profile.Rules[0].Aggregate, profile2.Rules[0].Aggregate) {
		t.Fatalf("归一形态与重载形态应等值: %+v / %+v",
			profile.Rules[0].MetadataFile, profile2.Rules[0].MetadataFile)
	}

	// 双写形态一（旧绑定键与新绑定键同时携带）载入拒绝。
	dual := strings.Replace(legacyBindingProfileJSON,
		`"contentBindings":[{"row":2,"dimension":"description"}]}}]}`,
		`"contentBindings":[{"row":2,"dimension":"description"}]},`+
			`"metadataFile":{"contentBindings":[{"row":2,"dimension":"tag"}]}}]}`, 1)
	if _, errs := ParseProfile([]byte(dual)); len(errs) == 0 {
		t.Fatal("绑定新旧落档位双写形态应载入拒绝")
	} else if !strings.Contains(errs[0].Error(), "双写") {
		t.Fatalf("双写拒绝文案不符: %v", errs[0])
	}
}

// TestFanoutLegacyPairingKeyNormalization 旧档配对键归一锚：metadataFile.pairing
// 读入即归一为分组绑定（键形同构——mode/identity 原样迁移），保存只写
// grouping 新键位；新旧分组键位双写形态载入拒绝。
func TestFanoutLegacyPairingKeyNormalization(t *testing.T) {
	profile, errs := ParseProfile([]byte(legacyPairingProfileJSON))
	if len(errs) != 0 {
		t.Fatalf("旧档配对键档案应零拒绝载入: %v", errs)
	}
	if len(profile.Rules) != 1 {
		t.Fatalf("应恰载入一条规则: %d", len(profile.Rules))
	}
	mf := profile.Rules[0].MetadataFile
	if mf == nil || mf.Grouping == nil || mf.Grouping.Mode != GroupingModeIdentity ||
		len(mf.Grouping.Identity) != 2 || mf.Grouping.Identity[0].Capture != "work" ||
		mf.Grouping.Identity[1].Capture != "seq" {
		t.Fatalf("旧档配对键应归一为分组绑定（键部分原样迁移）: %+v", mf)
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("档案序列化失败: %v", err)
	}
	if strings.Contains(string(data), `"pairing"`) {
		t.Fatalf("保存产物不应再落旧配对键: %s", data)
	}
	if !strings.Contains(string(data), `"grouping"`) {
		t.Fatalf("归一分组绑定应随档落新键位: %s", data)
	}
	// 求值行为：归一后的标识分组照常扇出。
	files := aggScanFiles("无题/无题_01.png", "无题/无题_01.txt")
	conclusions := fanoutRun(t, files, profile.Rules)
	txt := aggConclusionOf(t, conclusions, "无题/无题_01.txt")
	if txt.AggWorkKey != "" || !txt.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(txt), []string{"f:无题/无题_01.png"}) {
		t.Fatalf("归一配对键的标识分组应照常扇出: key=%q companion=%v fanout=%v",
			txt.AggWorkKey, txt.AggCompanion, txt.FanoutWorkKeys)
	}
	// 双写形态二（pairing 与 grouping 同时携带）载入拒绝。
	dual := strings.Replace(legacyPairingProfileJSON,
		`"metadataFile":{"pairing":`,
		`"metadataFile":{"grouping":{"mode":"directory"},"pairing":`, 1)
	if _, errs := ParseProfile([]byte(dual)); len(errs) == 0 {
		t.Fatal("分组新旧键位双写形态应载入拒绝")
	} else if !strings.Contains(errs[0].Error(), "双写") {
		t.Fatalf("双写拒绝文案不符: %v", errs[0])
	}
}

// —— 验收1 形态：前缀分组键两 png+txt（内容声明扇出端到端落地）—— //

// TestFanoutAcceptanceOnePrefixGroupContent 扇出语义的验收锚（用户实测定案
// 形态的 Go 级复刻）：前缀分组键把两张 png 与一份 txt 编入同组——两张 png
// 各自成一件独立作品，txt 的整段内容声明扇出给两件作品（作品级归集两键各
// 一条、投影并入各得描述、来源标注=说明文件名），txt 为元数据源（不导入、
// 不独立成件、不获聚合作品键）。
func TestFanoutAcceptanceOnePrefixGroupContent(t *testing.T) {
	const prefixGroupCapture = "work"
	rule := RuleEntry{
		ID:          "map-prefix",
		Description: "测试：前缀分组扇出（验收1 形态）",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}_{序号}", ".png"),
			{Connector: "and", Condition: &Condx{
				Target: CondxTargetFile,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"},
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".txt"}},
			}},
		},
		MetadataFile: &MetadataFileProduction{
			Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
				{Row: 1, Capture: prefixGroupCapture},
			}},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles(
		"画/无题_0.png", "画/无题_1.png", "画/无题.txt",
	)
	files[2].CompanionText = "总说明内容：本序列的公共描述"
	rules := []RuleEntry{rule}
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex())
	applyCompanionContentDeclarations(conclusions, files, rules, nil)

	// txt=元数据源：零聚合作品键+说明成员标记+扇出键集=两 png 作品键。
	txt := aggConclusionOf(t, conclusions, "画/无题.txt")
	want := []string{"f:画/无题_0.png", "f:画/无题_1.png"}
	if txt.AggWorkKey != "" || txt.AggTrackRole != "" || !txt.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(txt), want) {
		t.Fatalf("说明文件应为元数据源（不导入不独立成件、扇出到两件作品）: key=%q role=%q companion=%v fanout=%v",
			txt.AggWorkKey, txt.AggTrackRole, txt.AggCompanion, txt.FanoutWorkKeys)
	}
	// png 各自成件：零归属、内容声明经扇出面到达（自身终选零内容声明）。
	for _, png := range []string{"画/无题_0.png", "画/无题_1.png"} {
		got := aggConclusionOf(t, conclusions, png)
		if got.AggWorkKey != "" || got.AggCompanion {
			t.Fatalf("目标文件 %s 应独立成件: key=%q companion=%v", png, got.AggWorkKey, got.AggCompanion)
		}
		for _, decl := range got.Final {
			if decl.Source == SourceContent {
				t.Fatalf("目标文件 %s 自身终选不应产内容声明（经扇出面到达）: %+v", png, decl)
			}
		}
	}

	// 快照层：内容声明按扇出键集归集——两 png 作品键各收一条描述声明
	//（来源成员文件名随声明附注）；txt 行只补自身结论、不产作品位。
	assignIdx := newAssignmentDirIndex(nil, nil)
	snap := &RecognitionSnapshot{SchemaVersion: recognitionSnapshotSchemaVersion}
	for i := range conclusions {
		snap.Files = append(snap.Files, snapFileOf(&conclusions[i], nil, assignIdx))
	}
	contentDecls := workContentDeclsByWork(snap)
	for _, key := range want {
		decls := contentDecls[key]
		if len(decls) != 1 || decls[0].Dimension != string(DimDescription) ||
			decls[0].Value != "总说明内容：本序列的公共描述" ||
			decls[0].Source != string(SourceContent) || decls[0].OriginName != "无题.txt" {
			t.Fatalf("作品 %s 应经扇出面各得一条描述声明（来源=说明文件 无题.txt）: %+v", key, decls)
		}
	}
	if _, ok := contentDecls["f:画/无题.txt"]; ok {
		t.Fatalf("元数据源文件不应产生自身作品位的内容归集: %+v", contentDecls)
	}

	// 投影层：两 png 的作品级并入各得描述（详情「来源：说明文件」的数据面）。
	for i, png := range []string{"画/无题_0.png", "画/无题_1.png"} {
		merged := projectWorkMergedFile(snap.Files[i], contentDecls)
		found := false
		for _, d := range merged.Final {
			if d.Dimension == string(DimDescription) && d.Value == "总说明内容：本序列的公共描述" &&
				d.Source == string(SourceContent) && d.OriginName == "无题.txt" {
				found = true
			}
		}
		if !found {
			t.Fatalf("png %s 的投影应并入扇出描述声明: %+v", png, merged.Final)
		}
	}
}
