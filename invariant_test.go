// 决策12（防再偏离轮）的不变量内核测试：I1 成员判定自驱——「元数据文件」
// 产出的分组声明（配对键）在任意形态（单用/并用）下都是其成员判定的唯一
// 驱动，聚合标识不得替代；I3 桥接语义显式——说明成员归属=其分组内主导们
// 所在的作品（同件无歧义/跨件拒绝）；主导跨多作品守卫正反例。
package main

import (
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// invariantMappingRule 构造同一场景（资源行+绑定行+配对键）的两形态规则：
// pairWithAggregate 为真时加聚合成作品产出（图片轨+标识=work 捕获）成为
// 并用形态——其余声明（条件行、内容绑定、配对键）逐字段相同。
func invariantMappingRule(id string, pairWithAggregate bool) RuleEntry {
	rule := RuleEntry{
		ID:          id,
		Description: "测试：不变量映射规则",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".png"),
			pairingFileRow("{作品}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing: &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
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

// TestInvariantPairingDrivesMembership 不变量 I1（成员判定自驱）：同一场景
// （资源行+绑定行+配对键）在单用与并用（加聚合成作品产出）两形态下，配对键
// 均驱动绑定成员归属——单用挂主导文件作品（f:主导路径）、并用挂聚合作品
// （组内主导所在作品，桥接）；两形态的说明成员归属键逐一相等（本场景聚合
// 主导即配对主导，键同值——并用形态另携带作品类型与主导的轨道结论）。并用
// 形态下绑定行不再经聚合标识入组（聚合成员=轨行命中），其归属唯一来源=配对段。
func TestInvariantPairingDrivesMembership(t *testing.T) {
	files := aggScanFiles("画/a.png", "画/a.txt", "画/b.png", "画/b.txt")

	// 单用：配对键驱动——说明成员挂主导文件作品，主导不获归属（不并作品）。
	solo := pairingRun(t, files, []RuleEntry{invariantMappingRule("inv-solo", false)})
	soloTxtA := aggConclusionOf(t, solo, "画/a.txt")
	if soloTxtA.AggWorkKey != "f:画/a.png" || soloTxtA.AggTrackRole != "" ||
		soloTxtA.AggWorkType != "" || !soloTxtA.AggCompanion {
		t.Fatalf("单用形态 a.txt 应经配对键归并到主导文件作品（无轨/无类型+说明成员标记）: key=%q role=%q type=%q companion=%v",
			soloTxtA.AggWorkKey, soloTxtA.AggTrackRole, soloTxtA.AggWorkType, soloTxtA.AggCompanion)
	}
	soloTxtB := aggConclusionOf(t, solo, "画/b.txt")
	if soloTxtB.AggWorkKey != "f:画/b.png" || !soloTxtB.AggCompanion {
		t.Fatalf("单用形态 b.txt 应归并到 b.png 作品: key=%q companion=%v",
			soloTxtB.AggWorkKey, soloTxtB.AggCompanion)
	}
	for _, png := range []string{"画/a.png", "画/b.png"} {
		got := aggConclusionOf(t, solo, png)
		if got.AggWorkKey != "" || got.AggCompanion {
			t.Fatalf("单用形态主导 %s 不获归属（配对不并作品）: key=%q companion=%v",
				png, got.AggWorkKey, got.AggCompanion)
		}
	}

	// 并用：png 各自被聚合收编为 image 作品（work=a / work=b 两件），txt 经
	// 同一配对键归并——配对目标=组内主导所在作品（桥接，携带作品类型）。
	comb := pairingRun(t, files, []RuleEntry{invariantMappingRule("inv-comb", true)})
	pngA := aggConclusionOf(t, comb, "画/a.png")
	if pngA.AggWorkKey != "f:画/a.png" || pngA.AggTrackRole != contract.StoreTypeImage ||
		pngA.AggWorkType != sdkdto.ResourceTypeImage || pngA.AggCompanion {
		t.Fatalf("并用形态 a.png 应被聚合收编为图片作品主导: key=%q role=%q type=%q companion=%v",
			pngA.AggWorkKey, pngA.AggTrackRole, pngA.AggWorkType, pngA.AggCompanion)
	}
	combTxtA := aggConclusionOf(t, comb, "画/a.txt")
	if combTxtA.AggWorkKey != "f:画/a.png" || combTxtA.AggTrackRole != "" ||
		combTxtA.AggWorkType != sdkdto.ResourceTypeImage || !combTxtA.AggCompanion {
		t.Fatalf("并用形态 a.txt 应经配对键桥接归并到聚合作品（无轨+随作品类型+说明成员标记）: key=%q role=%q type=%q companion=%v",
			combTxtA.AggWorkKey, combTxtA.AggTrackRole, combTxtA.AggWorkType, combTxtA.AggCompanion)
	}
	combTxtB := aggConclusionOf(t, comb, "画/b.txt")
	if combTxtB.AggWorkKey != "f:画/b.png" || !combTxtB.AggCompanion {
		t.Fatalf("并用形态 b.txt 应归并到 b.png 聚合作品: key=%q companion=%v",
			combTxtB.AggWorkKey, combTxtB.AggCompanion)
	}

	// I1 核心断言：两形态的说明成员归属键逐一相等（配对键均驱动归属）。
	if soloTxtA.AggWorkKey != combTxtA.AggWorkKey || soloTxtB.AggWorkKey != combTxtB.AggWorkKey {
		t.Fatalf("单用/并用两形态的说明成员归属键应相等（配对键唯一驱动）: %q/%q vs %q/%q",
			soloTxtA.AggWorkKey, soloTxtB.AggWorkKey, combTxtA.AggWorkKey, combTxtB.AggWorkKey)
	}
}

// TestInvariantBridgeSingleWork 不变量 I3（桥接语义显式·用户实测场景）：并用
// 形态+群键聚合标识+共享捕获配对键——序列行 {作品标识}]{序号}（成员类型
// 限定 image）+txt 行 {作品标识}]{序号}（绑定→描述），聚合 identity=groupKey
// （每序列组一件作品）、配对键=work 捕获。txt 不命中序列行（成员类型限定），
// 修复前其经聚合标识（groupKey）入组的通道在 txt 上不可得→维持独立作品
// （偏离）；修复后经配对键归并为该序列作品的说明成员（归属键=序列作品键、
// 无轨、随作品类型）。
func TestInvariantBridgeSingleWork(t *testing.T) {
	const (
		dir     = "Sovetskaya"
		pngRel  = "Sovetskaya/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]0.png"
		workKey = "f:" + pngRel
	)
	txtRow := pairingFileRow("{作品标识}]{序号}", ".txt")
	txtRow.Connector = "and"
	rule := RuleEntry{
		ID:          "inv-bridge",
		Description: "测试：并用群键聚合+配对桥接（用户实测场景）",
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
			Pairing: &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
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
	conclusions := pairingRun(t, files, []RuleEntry{rule})

	// 序列组并成一件作品（identity=groupKey，主导=首个成员）。
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
	// txt 必须归并为该序列作品的说明成员（配对键驱动+桥接，非聚合标识）。
	for i, txt := range []string{
		dir + "/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]0.txt",
		dir + "/[TeeradonSs] [88174380] [Sovetskaya Belorussiya]1.txt",
	} {
		got := aggConclusionOf(t, conclusions, txt)
		if got.AggWorkKey != workKey || got.AggTrackRole != "" ||
			got.AggWorkType != sdkdto.ResourceTypeImage || !got.AggCompanion {
			t.Fatalf("说明文件 %d 应经配对键归并为序列作品的说明成员: key=%q role=%q type=%q companion=%v",
				i, got.AggWorkKey, got.AggTrackRole, got.AggWorkType, got.AggCompanion)
		}
	}
}

// TestInvariantBridgeCrossWorkGuard 主导跨多作品守卫正反例（I3 跨件拒绝）：
// 配对键取 {标签}（跨作品共享取值）、聚合标识取 {作品}（每图一件作品）——
// 正例：单主导收编为一件作品，同键说明文件归并该作品；反例：两个主导分属
// 两件作品（同键），守卫拒绝配对、说明文件维持独立作品，主导的聚合结论
// 原样保持。
func TestInvariantBridgeCrossWorkGuard(t *testing.T) {
	newRule := func() RuleEntry {
		txtRow := pairingFileRow("{标签}", ".txt")
		txtRow.Connector = "and"
		return RuleEntry{
			ID:          "inv-span",
			Description: "测试：主导跨作品守卫",
			Source:      RuleSourceManual,
			Enabled:     true,
			Category:    RuleCategoryAggregate,
			Conditions: []CondxRow{
				pairingFileRow("{作品}({标签})", ".png"),
				txtRow,
			},
			Identity: []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{
				WorkType: sdkdto.ResourceTypeImage,
				Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}},
			},
			MetadataFile: &MetadataFileProduction{
				Pairing: &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
					{Row: 1, Capture: "tag"},
				}},
				ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
			},
		}
	}

	// 正例：唯一主导（a(x).png 收编为 Wa），同键说明文件归并 Wa。
	soloFiles := aggScanFiles("单/a(x).png", "单/x.txt")
	solo := pairingRun(t, soloFiles, []RuleEntry{newRule()})
	lead := aggConclusionOf(t, solo, "单/a(x).png")
	if lead.AggWorkKey != "f:单/a(x).png" || lead.AggTrackRole != contract.StoreTypeImage {
		t.Fatalf("正例主导应收编为图片作品: key=%q role=%q", lead.AggWorkKey, lead.AggTrackRole)
	}
	member := aggConclusionOf(t, solo, "单/x.txt")
	if member.AggWorkKey != "f:单/a(x).png" || member.AggTrackRole != "" || !member.AggCompanion {
		t.Fatalf("正例说明文件应归并到唯一主导所在作品: key=%q role=%q companion=%v",
			member.AggWorkKey, member.AggTrackRole, member.AggCompanion)
	}

	// 反例：两个主导分属两件作品（同配对键），守卫拒绝——说明文件维持独立。
	spanFiles := aggScanFiles("跨/a(x).png", "跨/b(x).png", "跨/x.txt")
	span := pairingRun(t, spanFiles, []RuleEntry{newRule()})
	for _, png := range []string{"跨/a(x).png", "跨/b(x).png"} {
		got := aggConclusionOf(t, span, png)
		if got.AggWorkKey != "f:"+png || got.AggTrackRole != contract.StoreTypeImage || got.AggCompanion {
			t.Fatalf("反例主导 %s 的聚合结论应原样保持（守卫拒绝不扰主导）: key=%q role=%q companion=%v",
				png, got.AggWorkKey, got.AggTrackRole, got.AggCompanion)
		}
	}
	rejected := aggConclusionOf(t, span, "跨/x.txt")
	if rejected.AggWorkKey != "" || rejected.AggCompanion {
		t.Fatalf("反例说明文件应被守卫拒绝、维持独立作品（主导跨多件作品）: key=%q companion=%v",
			rejected.AggWorkKey, rejected.AggCompanion)
	}
}
