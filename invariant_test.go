// 防复发不变量（彻底解耦轮）的内核测试：内容声明只按分组扇出键集流动、
// 永不按作品归属键流动（单目标挂键=耦合复发形态）；元数据源文件不获聚合
// 作品键（其同时为聚合成作品组成员时除外——成员化只属于聚合产出本职）。
// 同一场景在映射单用与并用两形态下核验：扇出键集唯一驱动内容落点，作品
// 归属键不参与内容路由。
package main

import (
	"reflect"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// invariantMappingRule 构造同一场景（资源行+绑定行+分组绑定）的两形态规则：
// pairWithAggregate 为真时加聚合成作品产出（图片轨+标识=work 捕获）成为
// 并用形态——其余声明（条件行、内容绑定、分组绑定）逐字段相同。
func invariantMappingRule(id string, pairWithAggregate bool) RuleEntry {
	rule := RuleEntry{
		ID:          id,
		Description: "测试：不变量映射规则",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}", ".png"),
			fanoutFileRow("{作品}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
				{Row: 1, Capture: "work"},
			}},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	if !pairWithAggregate {
		return rule
	}
	rule.Category = RuleCategoryAggregate
	rule.Identity = []IdentityPart{{Row: 1, Capture: "work"}}
	rule.Aggregate = &AggregateProduction{
		WorkType: sdkdto.ResourceTypeImage,
		Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}},
	}
	return rule
}

// TestInvariantFanoutKeySetDrivesContent 不变量（内容按扇出键集流动）：同一
// 场景（资源行+绑定行+分组绑定）在单用与并用（加聚合成作品产出）两形态下，
// 说明文件的内容声明按同一扇出键集流动（键集=分组内作品的键，两形态逐一
// 相等——扇出键集唯一驱动内容落点）；目标文件的聚合归属（并用形态在、单用
// 形态无）不参与内容路由。
func TestInvariantFanoutKeySetDrivesContent(t *testing.T) {
	files := aggScanFiles("画/a.png", "画/a.txt", "画/b.png", "画/b.txt")

	// 单用：说明文件=元数据源（零归属键+说明成员标记），内容扇出到目标
	// 文件各自的作品键（目标不获归属——扇出不并作品）。
	solo := fanoutRun(t, files, []RuleEntry{invariantMappingRule("inv-solo", false)})
	soloTxtA := aggConclusionOf(t, solo, "画/a.txt")
	if soloTxtA.AggWorkKey != "" || soloTxtA.AggTrackRole != "" ||
		soloTxtA.AggWorkType != "" || !soloTxtA.AggCompanion ||
		!reflect.DeepEqual(soloTxtA.FanoutWorkKeys, []string{"f:画/a.png"}) {
		t.Fatalf("单用形态 a.txt 应为元数据源（零归属键+说明成员标记+扇出到 a.png 作品）: key=%q role=%q type=%q companion=%v fanout=%v",
			soloTxtA.AggWorkKey, soloTxtA.AggTrackRole, soloTxtA.AggWorkType, soloTxtA.AggCompanion, soloTxtA.FanoutWorkKeys)
	}
	soloTxtB := aggConclusionOf(t, solo, "画/b.txt")
	if soloTxtB.AggWorkKey != "" || !soloTxtB.AggCompanion ||
		!reflect.DeepEqual(soloTxtB.FanoutWorkKeys, []string{"f:画/b.png"}) {
		t.Fatalf("单用形态 b.txt 应扇出到 b.png 作品: key=%q companion=%v fanout=%v",
			soloTxtB.AggWorkKey, soloTxtB.AggCompanion, soloTxtB.FanoutWorkKeys)
	}
	for _, png := range []string{"画/a.png", "画/b.png"} {
		got := aggConclusionOf(t, solo, png)
		if got.AggWorkKey != "" || got.AggCompanion {
			t.Fatalf("单用形态目标 %s 不获归属（扇出不并作品）: key=%q companion=%v",
				png, got.AggWorkKey, got.AggCompanion)
		}
	}

	// 并用：png 各自被聚合收编为 image 作品（work=a / work=b 两件），txt 经
	// 聚合标识入组为聚合成作品的说明成员（归进作品、无轨、随作品类型）；
	// 内容扇出键集与单用形态逐一相等（内容落点=扇出键集，与归属形态无关）。
	comb := fanoutRun(t, files, []RuleEntry{invariantMappingRule("inv-comb", true)})
	pngA := aggConclusionOf(t, comb, "画/a.png")
	if pngA.AggWorkKey != "f:画/a.png" || pngA.AggTrackRole != contract.StoreTypeImage ||
		pngA.AggWorkType != sdkdto.ResourceTypeImage || pngA.AggCompanion {
		t.Fatalf("并用形态 a.png 应被聚合收编为图片作品主导: key=%q role=%q type=%q companion=%v",
			pngA.AggWorkKey, pngA.AggTrackRole, pngA.AggWorkType, pngA.AggCompanion)
	}
	combTxtA := aggConclusionOf(t, comb, "画/a.txt")
	if combTxtA.AggWorkKey != "f:画/a.png" || combTxtA.AggTrackRole != "" ||
		combTxtA.AggWorkType != sdkdto.ResourceTypeImage || !combTxtA.AggCompanion {
		t.Fatalf("并用形态 a.txt 应为聚合成作品的说明成员（归进作品+无轨+随作品类型）: key=%q role=%q type=%q companion=%v",
			combTxtA.AggWorkKey, combTxtA.AggTrackRole, combTxtA.AggWorkType, combTxtA.AggCompanion)
	}
	combTxtB := aggConclusionOf(t, comb, "画/b.txt")
	if combTxtB.AggWorkKey != "f:画/b.png" || !combTxtB.AggCompanion {
		t.Fatalf("并用形态 b.txt 应归进 b.png 聚合作品: key=%q companion=%v",
			combTxtB.AggWorkKey, combTxtB.AggCompanion)
	}

	// 核心断言：两形态的说明文件扇出键集逐一相等（内容按扇出键集流动，
	// 单用形态的零归属键与并用形态的聚合归属键均不改变内容落点）。
	if !reflect.DeepEqual(soloTxtA.FanoutWorkKeys, combTxtA.FanoutWorkKeys) ||
		!reflect.DeepEqual(soloTxtB.FanoutWorkKeys, combTxtB.FanoutWorkKeys) {
		t.Fatalf("单用/并用两形态的扇出键集应逐一相等（扇出键集唯一驱动）: %v/%v vs %v/%v",
			soloTxtA.FanoutWorkKeys, soloTxtB.FanoutWorkKeys, combTxtA.FanoutWorkKeys, combTxtB.FanoutWorkKeys)
	}
}

// TestInvariantMetadataSourceNoWorkKey 不变量（元数据源不获聚合作品键）：
// 仅绑定行命中、组内有落点的文件=元数据源——不获聚合作品键（单目标挂键=
// 耦合复发形态）；其同时为聚合成作品组成员时除外（并用绑定行成员照常归进
// 聚合作品）。本场景：分组绑定取 {标签}（跨作品共享取值）、聚合标识取
// {作品}（每图一件作品）——同键说明文件扇出到键集内全部作品。
func TestInvariantMetadataSourceNoWorkKey(t *testing.T) {
	newRule := func() RuleEntry {
		txtRow := fanoutFileRow("{标签}", ".txt")
		txtRow.Connector = "and"
		return RuleEntry{
			ID:          "inv-nokey",
			Description: "测试：元数据源零归属键",
			Source:      RuleSourceManual,
			Enabled:     true,
			Category:    RuleCategoryAggregate,
			Conditions: []CondxRow{
				fanoutFileRow("{作品}({标签})", ".png"),
				txtRow,
			},
			Identity: []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{
				WorkType: sdkdto.ResourceTypeImage,
				Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}},
			},
			MetadataFile: &MetadataFileProduction{
				Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
					{Row: 1, Capture: "tag"},
				}},
				ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
			},
		}
	}

	// 单主导：x.txt 扇出到唯一主导的聚合作品（键集一键）。
	soloFiles := aggScanFiles("单/a(x).png", "单/x.txt")
	solo := fanoutRun(t, soloFiles, []RuleEntry{newRule()})
	lead := aggConclusionOf(t, solo, "单/a(x).png")
	if lead.AggWorkKey != "f:单/a(x).png" || lead.AggTrackRole != contract.StoreTypeImage {
		t.Fatalf("单主导应收编为图片作品: key=%q role=%q", lead.AggWorkKey, lead.AggTrackRole)
	}
	member := aggConclusionOf(t, solo, "单/x.txt")
	if member.AggWorkKey != "" || member.AggTrackRole != "" || !member.AggCompanion ||
		!reflect.DeepEqual(member.FanoutWorkKeys, []string{"f:单/a(x).png"}) {
		t.Fatalf("元数据源应零聚合作品键（说明成员标记+扇出键集承载落点）: key=%q role=%q companion=%v fanout=%v",
			member.AggWorkKey, member.AggTrackRole, member.AggCompanion, member.FanoutWorkKeys)
	}

	// 两主导分属两件作品（同分组键）：说明文件扇出到两件作品各一份、零
	// 聚合作品键（不并作品、不整组拒绝——内容按键集流动，归属键不参与）。
	spanFiles := aggScanFiles("跨/a(x).png", "跨/b(x).png", "跨/x.txt")
	span := fanoutRun(t, spanFiles, []RuleEntry{newRule()})
	for _, png := range []string{"跨/a(x).png", "跨/b(x).png"} {
		got := aggConclusionOf(t, span, png)
		if got.AggWorkKey != "f:"+png || got.AggTrackRole != contract.StoreTypeImage || got.AggCompanion {
			t.Fatalf("两主导 %s 的聚合结论应各自成件不受扇出影响: key=%q role=%q companion=%v",
				png, got.AggWorkKey, got.AggTrackRole, got.AggCompanion)
		}
	}
	multi := aggConclusionOf(t, span, "跨/x.txt")
	if multi.AggWorkKey != "" || multi.AggTrackRole != "" || !multi.AggCompanion ||
		!reflect.DeepEqual(multi.FanoutWorkKeys, []string{"f:跨/a(x).png", "f:跨/b(x).png"}) {
		t.Fatalf("同键说明文件应扇出到两件作品（零归属键+键集两键）: key=%q role=%q companion=%v fanout=%v",
			multi.AggWorkKey, multi.AggTrackRole, multi.AggCompanion, multi.FanoutWorkKeys)
	}
}

// TestInvariantSeqGroupFanoutSource 序列群键聚合+共享捕获分组的并用形态
// （用户实测场景的组群形态）：序列行（成员类型限定 image）+txt 行（绑定→
// 描述），聚合标识=群键（每序列组一件作品）、分组绑定=txt 行的 {作品标识}
// 捕获——txt 不命中序列行（成员类型限定），经自己的命中行取同名捕获与 png
// 同键入组；序列作品成立时 txt=元数据源（零归属键、扇出到序列作品键），
// png 们并成一件序列作品（主导=首个成员）。
func TestInvariantSeqGroupFanoutSource(t *testing.T) {
	const (
		dir     = "Sovetskaya"
		pngRel  = "Sovetskaya/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]0.png"
		workKey = "f:" + pngRel
	)
	txtRow := fanoutFileRow("{作品标识}]{序号}", ".txt")
	txtRow.Connector = "and"
	rule := RuleEntry{
		ID:          "inv-seq-fanout",
		Description: "测试：序列群聚合+共享捕获分组",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			{Condition: &Condx{
				Target: CondxTargetSeq,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品标识}]{序号}"},
				Attach: []CondxAttach{{Kind: CondxAttachSeqMemberType, Types: []string{"image"}}},
			}},
			txtRow,
		},
		Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}},
		Aggregate: &AggregateProduction{
			WorkType: sdkdto.ResourceTypeImage,
			Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}},
		},
		MetadataFile: &MetadataFileProduction{
			Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
				{Row: 2, Capture: "work"},
			}},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles(
		dir+"/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]0.png",
		dir+"/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]1.png",
		dir+"/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]0.txt",
		dir+"/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]1.txt",
	)
	conclusions := fanoutRun(t, files, []RuleEntry{rule})

	// 序列组并成一件作品（标识=群键，主导=首个成员）。
	for i, png := range []string{
		dir + "/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]0.png",
		dir + "/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]1.png",
	} {
		got := aggConclusionOf(t, conclusions, png)
		if got.AggWorkKey != workKey || got.AggTrackRole != contract.StoreTypeImage || got.AggCompanion {
			t.Fatalf("序列成员 %d 应并成一件图片作品（主导=首个成员）: key=%q role=%q companion=%v",
				i, got.AggWorkKey, got.AggTrackRole, got.AggCompanion)
		}
	}
	// txt 经自己的命中行取 {作品标识} 与 png 同键入组=元数据源（零归属键、
	// 扇出到序列作品——分组绑定驱动，聚合标识（群键）在 txt 行不可得不碍）。
	for i, txt := range []string{
		dir + "/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]0.txt",
		dir + "/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]1.txt",
	} {
		got := aggConclusionOf(t, conclusions, txt)
		if got.AggWorkKey != "" || got.AggTrackRole != "" ||
			got.AggWorkType != "" || !got.AggCompanion ||
			!reflect.DeepEqual(got.FanoutWorkKeys, []string{workKey}) {
			t.Fatalf("说明文件 %d 应为元数据源（零归属键+扇出到序列作品）: key=%q role=%q type=%q companion=%v fanout=%v",
				i, got.AggWorkKey, got.AggTrackRole, got.AggWorkType, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
}
