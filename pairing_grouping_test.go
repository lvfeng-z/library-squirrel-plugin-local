// 配对机制独立化的单元测试：映射单用（无聚合产出）按配对键分组做说明成员
// 归并（不并作品）、标识配对（共享捕获跨行汇流）与目录配对（唯一主导守卫）、
// 无主导组不成立边界、聚合归属优先（映射不夺已归属成员）、并用时配对键声明
// 不牵连分组、配对键与绑定的载入校验、旧档（aggregate.contentBindings）归一
// 载入零迁移零拒绝与往返等值。
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

// pairingFileRow 构造文件目标条件行（模板 + 可选扩展名谓词）。
func pairingFileRow(tpl, ext string) CondxRow {
	row := CondxRow{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}
	if ext != "" {
		row.Condition.Attach = []CondxAttach{{Kind: CondxAttachFileExt, Value: ext}}
	}
	return row
}

// pairingMappingRule 构造映射单用规则（元数据类别、无聚合产出）：两行条件
//（资源行+说明行），说明行绑定→维度，配对键按入参声明。
func pairingMappingRule(id string, pairing *PairingKey, bindings ...ContentBinding) RuleEntry {
	return RuleEntry{
		ID:          id,
		Description: "测试：映射单用规则",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}_{序号}", ".png"),
			pairingFileRow("{作品}_{序号}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing:         pairing,
			ContentBindings: bindings,
		},
	}
}

// pairingRun 对文件面执行合并求值+聚合段/配对段，返回结论。
func pairingRun(t *testing.T, files []ScanFile, rules []RuleEntry) []FileConclusion {
	t.Helper()
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex())
	return conclusions
}

// —— 标识配对 ——

// TestPairingIdentityCapture 标识配对锚（剧本B 形态）：配对键=资源行声明的
// 复合标识（{作品}+{序号}，共享捕获跨行汇流——说明文件从自己的命中行取同名
// 捕获），捕获值相等的文件归为一组；组内说明成员归并到主导文件的作品（无轨、
// 不导入、不独立成作品），主导候选不获归属（配对不并作品，成员各自成作品）；
// 配对键取不到的说明文件（无 {序号} 可解析）不属任何配对组、维持独立作品；
// 组内无主导候选（成员全部是绑定行命中）组不成立。
func TestPairingIdentityCapture(t *testing.T) {
	rule := pairingMappingRule("map-seq",
		&PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
			{Row: 1, Capture: "work"},
			{Row: 1, Capture: "seq"},
		}},
		ContentBinding{Row: 2, Dimension: DimDescription},
	)
	files := aggScanFiles(
		"无题/无题_01.png", "无题/无题_02.png", "无题/无题_03.png",
		"无题/无题_01.txt", "无题/无题_02.txt", "无题/无题_03.txt",
		"无题/无题_04.txt", // 配对键可得但组内无主导候选（成员全部绑定行命中）
		"无题/无题.txt",    // 不命中任何条件行
	)
	conclusions := pairingRun(t, files, []RuleEntry{rule})

	for _, png := range []string{"无题_01.png", "无题_02.png", "无题_03.png"} {
		got := aggConclusionOf(t, conclusions, "无题/"+png)
		if got.AggWorkKey != "" || got.AggTrackRole != "" || got.AggWorkType != "" || got.AggCompanion {
			t.Fatalf("资源文件 %s 应维持独立作品（配对不并作品）: key=%q role=%q type=%q companion=%v",
				png, got.AggWorkKey, got.AggTrackRole, got.AggWorkType, got.AggCompanion)
		}
	}
	for i := 1; i <= 3; i++ {
		got := aggConclusionOf(t, conclusions, "无题/无题_0"+string(rune('0'+i))+".txt")
		want := "f:无题/无题_0" + string(rune('0'+i)) + ".png"
		if got.AggWorkKey != want || got.AggTrackRole != "" || got.AggWorkType != "" || !got.AggCompanion {
			t.Fatalf("说明文件 %d 应归并到同名主导文件的作品（无轨+说明成员标记+不并作品类型）: key=%q role=%q type=%q companion=%v",
				i, got.AggWorkKey, got.AggTrackRole, got.AggWorkType, got.AggCompanion)
		}
	}
	// 无主导候选的组不成立：无题_04.txt 的配对键（无题,04）无资源文件同组，
	// 维持独立作品。
	lone := aggConclusionOf(t, conclusions, "无题/无题_04.txt")
	if lone.AggWorkKey != "" || lone.AggCompanion {
		t.Fatalf("组内无主导候选应不成立、说明文件维持独立作品: key=%q companion=%v",
			lone.AggWorkKey, lone.AggCompanion)
	}
	// 不命中任何条件行的文件不参与配对（组级说明文件独立成件现状）。
	stray := aggConclusionOf(t, conclusions, "无题/无题.txt")
	if stray.AggWorkKey != "" || stray.AggCompanion {
		t.Fatalf("不命中条件行的文件不应参与配对: key=%q companion=%v",
			stray.AggWorkKey, stray.AggCompanion)
	}
}

// TestGroupCompanionMergeWithoutAggregating 映射单用组的归并语义锚：说明成员
// 归并到主导文件作品（归属键=主导文件相对路径、无轨、无作品类型——作品形态
// 随主导文件自身结论）且不并作品（主导候选不获归属）；兼有主导行与绑定行命中
// 的文件=双重身份——按主导处置（照常独立成件、不带说明成员标记），不作为
// 说明成员归并；无说明成员的组（仅主导候选）无归并产出。
func TestGroupCompanionMergeWithoutAggregating(t *testing.T) {
	// 资源行带扩展名（仅 .png），绑定行不带扩展名谓词（任意扩展名命中）——
	// a.png 同时命中两行=双重身份，a.txt 仅命中绑定行=说明成员。
	rule := RuleEntry{
		ID:          "map-merge",
		Description: "测试：映射单用归并",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".png"),
			pairingFileRow("{作品}", ""),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing: &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
				{Row: 1, Capture: "work"},
			}},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles("画/a.png", "画/a.txt", "画/b.png")
	conclusions := pairingRun(t, files, []RuleEntry{rule})

	lead := aggConclusionOf(t, conclusions, "画/a.png")
	if lead.AggWorkKey != "" || lead.AggCompanion {
		t.Fatalf("双重身份文件（主导行+绑定行命中）应按主导处置、维持独立作品: key=%q companion=%v",
			lead.AggWorkKey, lead.AggCompanion)
	}
	member := aggConclusionOf(t, conclusions, "画/a.txt")
	if member.AggWorkKey != "f:画/a.png" || member.AggTrackRole != "" || member.AggWorkType != "" || !member.AggCompanion {
		t.Fatalf("说明成员应归并到主导文件作品（键=f:主导相对路径、无轨、无作品类型）: key=%q role=%q type=%q companion=%v",
			member.AggWorkKey, member.AggTrackRole, member.AggWorkType, member.AggCompanion)
	}
	solo := aggConclusionOf(t, conclusions, "画/b.png")
	if solo.AggWorkKey != "" || solo.AggCompanion {
		t.Fatalf("无说明成员的组不应有归并产出（主导候选不获归属）: key=%q companion=%v",
			solo.AggWorkKey, solo.AggCompanion)
	}
}

// —— 目录配对与唯一主导守卫 ——

// TestPairingDirectoryGuard 目录配对正反例锚：同目录主导候选唯一时任意的说明
// 文件归并给该主导（异名形态零配置吸收）；一目录多主导=归属歧义，守卫拒绝
// 配对（说明文件维持独立作品，主导候选不受影响）。
func TestPairingDirectoryGuard(t *testing.T) {
	rule := RuleEntry{
		ID:          "map-dir",
		Description: "测试：目录配对",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".jpg"),
			pairingFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing:         &PairingKey{Mode: PairingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles(
		"单图/cover.jpg", "单图/说明.txt", // 正例：目录内唯一主导
		"双图/a.jpg", "双图/b.jpg", "双图/说明.txt", // 反例：两个主导候选
	)
	conclusions := pairingRun(t, files, []RuleEntry{rule})

	cover := aggConclusionOf(t, conclusions, "单图/cover.jpg")
	if cover.AggWorkKey != "" || cover.AggCompanion {
		t.Fatalf("目录配对的主导候选应维持独立作品: key=%q companion=%v",
			cover.AggWorkKey, cover.AggCompanion)
	}
	paired := aggConclusionOf(t, conclusions, "单图/说明.txt")
	if paired.AggWorkKey != "f:单图/cover.jpg" || paired.AggTrackRole != "" || !paired.AggCompanion {
		t.Fatalf("唯一主导目录的说明文件应归并: key=%q role=%q companion=%v",
			paired.AggWorkKey, paired.AggTrackRole, paired.AggCompanion)
	}
	// 反例：多主导目录守卫拒绝——说明文件维持独立作品，主导候选维持独立。
	for _, jpg := range []string{"双图/a.jpg", "双图/b.jpg"} {
		got := aggConclusionOf(t, conclusions, jpg)
		if got.AggWorkKey != "" || got.AggCompanion {
			t.Fatalf("多主导目录的主导候选 %s 应不受配对影响: key=%q companion=%v",
				jpg, got.AggWorkKey, got.AggCompanion)
		}
	}
	rejected := aggConclusionOf(t, conclusions, "双图/说明.txt")
	if rejected.AggWorkKey != "" || rejected.AggCompanion {
		t.Fatalf("多主导目录的说明文件应被守卫拒绝、维持独立作品: key=%q companion=%v",
			rejected.AggWorkKey, rejected.AggCompanion)
	}
}

// TestPairingNoLeadGroupDissolves 无主导边界锚（目录配对面）：目录内全部参与
// 文件都是绑定行命中（无主导候选）→ 配对组不成立，说明文件维持现状独立作品。
func TestPairingNoLeadGroupDissolves(t *testing.T) {
	rule := RuleEntry{
		ID:          "map-nolead",
		Description: "测试：无主导目录",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".jpg"),
			pairingFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing:         &PairingKey{Mode: PairingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles("只有说明/说明.txt", "只有说明/备注.txt")
	conclusions := pairingRun(t, files, []RuleEntry{rule})
	for _, rel := range []string{"只有说明/说明.txt", "只有说明/备注.txt"} {
		got := aggConclusionOf(t, conclusions, rel)
		if got.AggWorkKey != "" || got.AggCompanion {
			t.Fatalf("无主导候选的目录配对应不成立、说明文件维持独立作品: %s key=%q companion=%v",
				rel, got.AggWorkKey, got.AggCompanion)
		}
	}
}

// —— 与聚合段的次序与并用 ——

// TestPairingAggregatePriorityOverMapping 聚合归属优先锚：聚合规则先归属
//（聚合在前），映射规则不夺已归属成员——即便映射规则在档案序居前，已被聚合
// 并组的文件不再参与配对（主导候选与说明成员双双出局），聚合结论原样保持。
func TestPairingAggregatePriorityOverMapping(t *testing.T) {
	mapping := RuleEntry{
		ID:          "map-first",
		Description: "测试：档案序居前的映射规则",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".jpg"),
			pairingFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing:         &PairingKey{Mode: PairingModeDirectory},
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
			pairingFileRow("{作品}", ".txt"),
			pairingFileRow("{作品}", ".jpg"),
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
	conclusions := pairingRun(t, files, []RuleEntry{mapping, article})

	txt := aggConclusionOf(t, conclusions, "d/x.txt")
	if txt.AggWorkKey != "f:d/x.txt" || txt.AggTrackRole != sdkdto.StoreRoleDocument || txt.AggCompanion {
		t.Fatalf("同名对应由聚合归属优先收编（正文轨）: key=%q role=%q companion=%v",
			txt.AggWorkKey, txt.AggTrackRole, txt.AggCompanion)
	}
	jpg := aggConclusionOf(t, conclusions, "d/x.jpg")
	if jpg.AggWorkKey != "f:d/x.txt" || jpg.AggTrackRole != contract.StoreTypeImage || jpg.AggCompanion {
		t.Fatalf("同名对的图片应由聚合收编（内嵌图轨）: key=%q role=%q companion=%v",
			jpg.AggWorkKey, jpg.AggTrackRole, jpg.AggCompanion)
	}
}

// TestPairingDormantKeyAlongsideAggregate 并用时配对键不牵连锚：聚合规则同时
// 携带「元数据文件」产出与配对键声明（键与聚合标识各不相同），分组仍由聚合
// 标识驱动——并用路径与不带配对键的现状全等。
func TestPairingDormantKeyAlongsideAggregate(t *testing.T) {
	rule := RuleEntry{
		ID:          "agg-with-key",
		Description: "测试：并用配对键不牵连",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".mp4"),
			pairingFileRow("{作品}.{任意}", ".srt"),
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
			// 配对键声明与聚合标识的取值形态不同（目录配对）——在场不牵连
			// 分组：并用路径的分组恒由聚合标识驱动。
			Pairing:         &PairingKey{Mode: PairingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles("d/a.mp4", "d/a.zh.srt", "d/b.mp4")
	conclusions := pairingRun(t, files, []RuleEntry{rule})

	video := aggConclusionOf(t, conclusions, "d/a.mp4")
	if video.AggWorkKey != "f:d/a.mp4" || video.AggTrackRole != sdkdto.StoreRoleVideoMain || video.AggCompanion {
		t.Fatalf("并用时视频主导归属应与现状全等: key=%q role=%q companion=%v",
			video.AggWorkKey, video.AggTrackRole, video.AggCompanion)
	}
	sub := aggConclusionOf(t, conclusions, "d/a.zh.srt")
	if sub.AggWorkKey != "f:d/a.mp4" || sub.AggTrackRole != contract.StoreTypeSubtitle || sub.AggCompanion {
		t.Fatalf("并用时字幕轨归属应与现状全等: key=%q role=%q companion=%v",
			sub.AggWorkKey, sub.AggTrackRole, sub.AggCompanion)
	}
	other := aggConclusionOf(t, conclusions, "d/b.mp4")
	if other.AggWorkKey != "f:d/b.mp4" || other.AggTrackRole != sdkdto.StoreRoleVideoMain {
		t.Fatalf("并用时异标识视频应各自成件: key=%q role=%q",
			other.AggWorkKey, other.AggTrackRole)
	}
}

// —— 载入校验 ——

// pairingRuleJSON 构造映射单用条目的中间形态（元数据类别、两行条件、说明行
// 绑定→描述），配对键按入参。
func pairingRuleJSON(id string, pairing *PairingKey) *ruleEntryJSON {
	row2 := pairingFileRow("{作品}_{序号}", ".txt")
	row2.Connector = "and"
	return &ruleEntryJSON{
		ID:          id,
		Description: "测试：映射单用条目",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}_{序号}", ".png"),
			row2,
		},
		MetadataFile: &MetadataFileProduction{
			Pairing:         pairing,
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
}

// TestPairingLoadValidation 配对键声明的载入校验：映射单用须声明配对键；形态
// 枚举合法（identity=标识配对 / directory=目录配对）；标识配对须携带配对键
// 部分、部分行号与捕获名引用存在（序列群键保留名只选自序列目标行）；目录
// 配对不得携带标识部分；并用（聚合在场）时配对键可省略、声明在场须合法。
func TestPairingLoadValidation(t *testing.T) {
	identity := &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
		{Row: 1, Capture: "work"}, {Row: 1, Capture: "seq"},
	}}
	// 正例：映射单用（元数据类别）携配对键与绑定，载入通过。
	entry, err := ruleFromJSON(pairingRuleJSON("map-ok", identity))
	if err != nil {
		t.Fatalf("映射单用条目应载入: %v", err)
	}
	if entry.MetadataFile == nil || entry.MetadataFile.Pairing.Mode != PairingModeIdentity ||
		len(entry.MetadataFile.Pairing.Identity) != 2 || len(entry.MetadataFile.ContentBindings) != 1 {
		t.Fatalf("配对键与绑定应原样载入: %+v", entry.MetadataFile)
	}

	cases := []struct {
		name    string
		pairing *PairingKey
		wantSub string
	}{
		{"映射单用缺配对键", nil, "缺少配对键声明"},
		{"标识配对缺部分", &PairingKey{Mode: PairingModeIdentity}, "缺少配对键部分"},
		{"目录配对带标识部分", &PairingKey{Mode: PairingModeDirectory, Identity: identity.Identity}, "不得携带标识配对部分"},
		{"配对方式非法", &PairingKey{Mode: PairingMode("bogus")}, "配对方式非法"},
		{"部分行号越界", &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{{Row: 3, Capture: "work"}}}, "越界"},
		{"捕获名未声明", &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "bogus"}}}, "未在任何条件行声明"},
		{"序列群键选自非序列行", &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}}}, "只能选自序列目标行"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ruleFromJSON(pairingRuleJSON("map-bad", tc.pairing))
			if err == nil {
				t.Fatal("非法配对键应载入拒绝")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("错误文案应含 %q: %v", tc.wantSub, err)
			}
		})
	}

	// 并用（聚合在场）时配对键可省略（旧档归一形态），声明在场须合法。
	mixed := pairingRuleJSON("map-agg-ok", nil)
	mixed.Category = RuleCategoryAggregate
	mixed.Identity = []IdentityPart{{Row: 1, Capture: "work"}}
	mixed.Aggregate = &AggregateProduction{
		WorkType: sdkdto.ResourceTypeImage,
		Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}},
	}
	if _, err := ruleFromJSON(mixed); err != nil {
		t.Fatalf("并用条目省略配对键应载入: %v", err)
	}
}

// —— 旧档归一 ——

// legacyBindingProfileJSON 旧档原文（绑定住聚合产出内的 contentBindings 键——
// 迁出前的落档形态）。
const legacyBindingProfileJSON = `{"schemaVersion":5,"rules":[
{"id":"legacy-agg","description":"旧档并用规则","source":"manual","enabled":true,"category":"aggregate",
 "conditions":[
  {"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}"},"attach":[{"kind":"fileExt","value":".jpg"}]}},
  {"condition":{"connector":"and","target":"file","format":{"mode":"template","tpl":"{作品}"},"attach":[{"kind":"fileExt","value":".txt"}]}}],
 "identity":[{"row":1,"capture":"work"}],
 "aggregate":{"workType":"image","tracks":[{"row":1,"role":"image"}],
  "contentBindings":[{"row":2,"dimension":"description"}]}}]}`

// TestPairingLegacyArchiveNormalization 旧档归一锚：含 aggregate.contentBindings
// 的规则档案载入归一到规则级「元数据文件」落档位（零迁移零拒绝——不写回不
// 拒收，载入即归一）；归一后条目与全新落档形态往返等值（保存只出新键位、
// 聚合产出不再携带绑定键）；归一条目的求值行为与并用现状全等（说明成员照常
// 归并）；新旧两个落档位同时携带绑定（双写形态）载入拒绝。
func TestPairingLegacyArchiveNormalization(t *testing.T) {
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
	if entry.MetadataFile == nil || len(entry.MetadataFile.ContentBindings) != 1 ||
		entry.MetadataFile.ContentBindings[0].Row != 2 ||
		entry.MetadataFile.ContentBindings[0].Dimension != DimDescription ||
		entry.MetadataFile.Pairing != nil {
		t.Fatalf("旧档绑定应归一到规则级落档位（无配对键=并用形态）: %+v", entry.MetadataFile)
	}

	// 归一载入后的求值行为与并用现状全等：同名图片+文本对=图片聚合作品+伴随
	// 说明成员（说明成员照常归并，无需配对键）。
	files := aggScanFiles("img/无题.jpg", "img/无题.txt")
	conclusions := pairingRun(t, files, profile.Rules)
	lead := aggConclusionOf(t, conclusions, "img/无题.jpg")
	member := aggConclusionOf(t, conclusions, "img/无题.txt")
	if lead.AggWorkKey != "f:img/无题.jpg" || lead.AggTrackRole != contract.StoreTypeImage || lead.AggCompanion {
		t.Fatalf("归一条目的主导归属应与并用现状全等: key=%q role=%q companion=%v",
			lead.AggWorkKey, lead.AggTrackRole, lead.AggCompanion)
	}
	if member.AggWorkKey != "f:img/无题.jpg" || member.AggTrackRole != "" || !member.AggCompanion {
		t.Fatalf("归一条目的说明成员应照常归并: key=%q role=%q companion=%v",
			member.AggWorkKey, member.AggTrackRole, member.AggCompanion)
	}

	// 往返等值：保存只出新键位（绑定住 metadataFile.contentBindings、聚合产出
	// 不再携带绑定键），重载后条目与归一形态等值。
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
	profile2, errs := ParseProfile(data)
	if len(errs) != 0 {
		t.Fatalf("归一保存产物应零拒绝载入: %v", errs)
	}
	if !reflect.DeepEqual(profile.Rules[0].MetadataFile, profile2.Rules[0].MetadataFile) ||
		!reflect.DeepEqual(profile.Rules[0].Aggregate, profile2.Rules[0].Aggregate) {
		t.Fatalf("归一形态与重载形态应等值: %+v / %+v",
			profile.Rules[0].MetadataFile, profile2.Rules[0].MetadataFile)
	}

	// 双写形态（旧键与新键同时携带绑定）载入拒绝。
	dual := strings.Replace(legacyBindingProfileJSON,
		`"contentBindings":[{"row":2,"dimension":"description"}]}}]}`,
		`"contentBindings":[{"row":2,"dimension":"description"}]},`+
			`"metadataFile":{"contentBindings":[{"row":2,"dimension":"tag"}]}}]}`, 1)
	if _, errs := ParseProfile([]byte(dual)); len(errs) == 0 {
		t.Fatal("新旧落档位双写形态应载入拒绝")
	} else if !strings.Contains(errs[0].Error(), "双写") {
		t.Fatalf("双写拒绝文案不符: %v", errs[0])
	}
}

// —— 配对守卫拒绝注记（剧本B 帧4 行详情可见原因的数据位） —— //

// TestPairingGuardNoteAnchors 目录配对守卫拒绝注记的正反例：多主导目录的
// 说明成员候选携带主导候选数（PairingGuardLeads，行详情「目录内有 N 个主导
// 候选，目录配对未成立，说明文件维持独立作品」的数据面），主导候选自身与
// 配对成立/标识配对拒绝/聚合收编形态零注记；注记随快照往返保真（重识别后
// 目录主导归一则随新快照自然清零——零迁移零残留）。
func TestPairingGuardNoteAnchors(t *testing.T) {
	dirRule := RuleEntry{
		ID:          "map-dir-guard",
		Description: "测试：目录配对守卫",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".jpg"),
			pairingFileRow("{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing:         &PairingKey{Mode: PairingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	// 正例：多主导目录（两图+说明文件）——说明成员候选落主导候选数注记、
	// 零归属零说明标记；主导候选不落注（维持独立作品是主导的正常形态）。
	files := aggScanFiles("双图/a.jpg", "双图/b.jpg", "双图/说明.txt")
	conclusions := pairingRun(t, files, []RuleEntry{dirRule})
	for _, jpg := range []string{"双图/a.jpg", "双图/b.jpg"} {
		got := aggConclusionOf(t, conclusions, jpg)
		if got.AggWorkKey != "" || got.PairingGuardLeads != 0 {
			t.Fatalf("多主导目录的主导候选 %s 不应落注: key=%q guard=%d", jpg, got.AggWorkKey, got.PairingGuardLeads)
		}
	}
	rejected := aggConclusionOf(t, conclusions, "双图/说明.txt")
	if rejected.AggWorkKey != "" || rejected.AggCompanion || rejected.PairingGuardLeads != 2 {
		t.Fatalf("多主导目录的说明成员候选应落主导候选数注记（零归属）: key=%q companion=%v guard=%d",
			rejected.AggWorkKey, rejected.AggCompanion, rejected.PairingGuardLeads)
	}
	// 反例一：唯一主导目录（配对成立）零注记。
	solo := aggScanFiles("单图/cover.jpg", "单图/说明.txt")
	soloConclusions := pairingRun(t, solo, []RuleEntry{dirRule})
	if got := aggConclusionOf(t, soloConclusions, "单图/说明.txt"); got.PairingGuardLeads != 0 || !got.AggCompanion {
		t.Fatalf("配对成立的说明成员应零注记照常归并: guard=%d companion=%v", got.PairingGuardLeads, got.AggCompanion)
	}
	// 反例二：标识配对的多主导拒绝不落注（注记词面按目录配对措辞，标识
	// 配对拒绝无对应帧文案——宁缺勿错）。
	idRule := RuleEntry{
		ID:          "map-id-guard",
		Description: "测试：标识配对守卫",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			pairingFileRow("{作品}", ".png"),
			{Connector: "and", Condition: &Condx{Target: CondxTargetFile,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"},
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".jpg"}}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetFile,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"},
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".txt"}}}},
		},
		MetadataFile: &MetadataFileProduction{
			Pairing: &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
				{Row: 1, Capture: "work"},
			}},
			ContentBindings: []ContentBinding{{Row: 3, Dimension: DimDescription}},
		},
	}
	idFiles := aggScanFiles("混/a.png", "混/a.jpg", "混/a.txt")
	idConclusions := pairingRun(t, idFiles, []RuleEntry{idRule})
	if got := aggConclusionOf(t, idConclusions, "混/a.txt"); got.AggWorkKey != "" || got.PairingGuardLeads != 0 {
		t.Fatalf("标识配对的多主导拒绝不应落注: key=%q guard=%d", got.AggWorkKey, got.PairingGuardLeads)
	}
	// 反例三：聚合归属优先（聚合段先收编的说明文件不参与配对）零注记。
	mixed := aggScanFiles("图文/cover.jpg", "图文/说明.txt")
	mixedRules := append([]RuleEntry{PresetAggregateRules()[2]}, dirRule)
	mixedConclusions := pairingRun(t, mixed, mixedRules)
	if got := aggConclusionOf(t, mixedConclusions, "图文/说明.txt"); got.AggWorkKey != "f:图文/说明.txt" || got.PairingGuardLeads != 0 {
		t.Fatalf("聚合收编的说明文件应带聚合归属零注记: key=%q guard=%d", got.AggWorkKey, got.PairingGuardLeads)
	}
	// 快照往返保真：注记随 SnapFile 落档（json 键 pairingGuardLeads），旧快照
	// 零值载入即无注记语义（零迁移）。
	snap := snapFileOf(aggConclusionOf(t, conclusions, "双图/说明.txt"), nil, newAssignmentDirIndex(nil, nil))
	data, err := json.Marshal(&RecognitionSnapshot{SchemaVersion: recognitionSnapshotSchemaVersion, Files: []SnapFile{snap}})
	if err != nil {
		t.Fatalf("序列化快照失败: %v", err)
	}
	var back RecognitionSnapshot
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("反序列化快照失败: %v", err)
	}
	if len(back.Files) != 1 || back.Files[0].PairingGuardLeads != 2 {
		t.Fatalf("守卫注记应随快照往返保真: %+v", back.Files)
	}
	if !strings.Contains(string(data), `"pairingGuardLeads":2`) {
		t.Fatalf("快照 JSON 应携带 pairingGuardLeads 键: %s", data)
	}
}
