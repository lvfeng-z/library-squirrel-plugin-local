// 行即分组求值端的扇出/告警/集归属锚：目录分组下只命中目录行的资源文件是容器
// 成员与扇出落点、必含轨单位数零静默与多命中告警、告警文案不依赖日志适配层对
// 数值实参的处理、界面手写声明规则的落档形态对配对的决定作用、作品集聚合成员面
// 的「规则命中」边界。
package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// adapterWarnf 构造模拟会话管理器日志适配层的告警回调：实参先逐个转成字串再套
// 格式串——数值实参套 %d 会得到 %!d(string=N) 形态。告警文案必须在交付前自格
// 式化完毕、以单一字串实参配 %s 交付，才能不依赖适配层对数值实参的处理。
func adapterWarnf(dst *[]string) func(string, ...any) {
	return func(format string, args ...any) {
		strs := make([]any, len(args))
		for i, a := range args {
			strs[i] = fmt.Sprint(a)
		}
		*dst = append(*dst, fmt.Sprintf(format, strs...))
	}
}

// dirOnlyResourceMappingRule 构造「目录行＋说明行」的映射单用规则（元数据文件
// 绑目录分组的产生行）：规则里没有其它文件/序列行供资源文件命中，资源文件只
// 靠目录行（目录行对目录下所有文件成立）进入规则命中集。
func dirOnlyResourceMappingRule(id string) RuleEntry {
	return RuleEntry{
		ID:          id,
		Description: "测试：目录行＋说明行",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}] {标题}")}},
			{Connector: "or", Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}"),
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".txt"}}}},
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory, Row: 1},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
}

// TestFanoutRowGroupDirectoryOnlyResourceIsFanoutTarget 目录分组扇出锚：资源文件
// 只命中目录行（规则＝目录行＋说明行）时，它是目录容器的成员、是说明内容的落
// 点——说明文件为元数据源并扇出到该资源文件；资源文件自身不是内容源、不获作
// 品键（扇出不并作品）。只有说明文件的目录组内无落点，按孤文件回退。
func TestFanoutRowGroupDirectoryOnlyResourceIsFanoutTarget(t *testing.T) {
	rule := dirOnlyResourceMappingRule("map-dir-only-resource")
	files := aggScanFiles(
		"旧单/[作者A] 标题A/[作者A] 标题A.jpg", "旧单/[作者A] 标题A/说明.txt",
		"孤说明/[作者B] 标题B/说明.txt",
	)
	conclusions := fanoutRun(t, files, []RuleEntry{rule})

	resource := aggConclusionOf(t, conclusions, "旧单/[作者A] 标题A/[作者A] 标题A.jpg")
	if resource.AggWorkKey != "" || resource.AggCompanion || len(fanoutKeysOf(resource)) != 0 {
		t.Fatalf("只命中目录行的资源文件应维持独立作品、不作内容源: key=%q companion=%v fanout=%v",
			resource.AggWorkKey, resource.AggCompanion, resource.FanoutWorkKeys)
	}
	paired := aggConclusionOf(t, conclusions, "旧单/[作者A] 标题A/说明.txt")
	if paired.AggWorkKey != "" || !paired.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(paired), []string{"f:旧单/[作者A] 标题A/[作者A] 标题A.jpg"}) {
		t.Fatalf("说明文件应扇出到同目录只命中目录行的资源文件: key=%q companion=%v fanout=%v",
			paired.AggWorkKey, paired.AggCompanion, paired.FanoutWorkKeys)
	}
	orphan := aggConclusionOf(t, conclusions, "孤说明/[作者B] 标题B/说明.txt")
	if orphan.AggWorkKey != "" || orphan.AggCompanion || len(fanoutKeysOf(orphan)) != 0 {
		t.Fatalf("只有说明文件的目录应按孤文件回退: key=%q companion=%v fanout=%v",
			orphan.AggWorkKey, orphan.AggCompanion, orphan.FanoutWorkKeys)
	}
}

// TestFanoutRowGroupDirectoryOnlyResourceBesideAggregate 并用形态锚：聚合成作品
// 与元数据文件同绑目录分组时，只命中目录行的文件不是聚合成员（不获作品键、不
// 带说明成员标记），但仍是目录容器的成员——说明内容扇出到组内全部作品（聚合
// 作品键与各自独立的文件键）。
func TestFanoutRowGroupDirectoryOnlyResourceBesideAggregate(t *testing.T) {
	rule := RuleEntry{
		ID:          "agg-dir-with-plain-file",
		Description: "测试：目录分组并用与只命中目录行的文件",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}] {标题}")}},
			fanoutFileRow("{任意}", ".jpg"),
			fanoutFileRow("{作品}", ".txt"),
		},
		Aggregate: &AggregateProduction{
			WorkType:     sdkdto.ResourceTypeImage,
			Grouping:     &GroupingBinding{Mode: GroupingModeDirectory, Row: 1},
			Tracks:       []TrackAssign{{Row: 2, Role: contract.StoreTypeImage}},
			CompanionUse: CompanionUseTrack,
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory, Row: 1},
			ContentBindings: []ContentBinding{{Row: 3, Dimension: DimDescription}},
		},
	}
	files := aggScanFiles(
		"作品甲/[作者A] 标题A/a.jpg", "作品甲/[作者A] 标题A/说明.txt", "作品甲/[作者A] 标题A/notes.md",
	)
	conclusions := fanoutRun(t, files, []RuleEntry{rule})

	lead := aggConclusionOf(t, conclusions, "作品甲/[作者A] 标题A/a.jpg")
	if lead.AggWorkKey != "f:作品甲/[作者A] 标题A/a.jpg" || lead.AggTrackRole != contract.StoreTypeImage ||
		lead.AggCompanion || len(fanoutKeysOf(lead)) != 0 {
		t.Fatalf("目录分组的图片成员应按目录成件且不作内容源: key=%q role=%q companion=%v fanout=%v",
			lead.AggWorkKey, lead.AggTrackRole, lead.AggCompanion, lead.FanoutWorkKeys)
	}
	plain := aggConclusionOf(t, conclusions, "作品甲/[作者A] 标题A/notes.md")
	if plain.AggWorkKey != "" || plain.AggCompanion || len(fanoutKeysOf(plain)) != 0 {
		t.Fatalf("只命中目录行的文件不获作品键、不作内容源: key=%q companion=%v fanout=%v",
			plain.AggWorkKey, plain.AggCompanion, plain.FanoutWorkKeys)
	}
	note := aggConclusionOf(t, conclusions, "作品甲/[作者A] 标题A/说明.txt")
	if note.AggWorkKey != "f:作品甲/[作者A] 标题A/a.jpg" || !note.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(note), []string{"f:作品甲/[作者A] 标题A/a.jpg", "f:作品甲/[作者A] 标题A/notes.md"}) {
		t.Fatalf("说明内容应扇出到目录容器内全部作品: key=%q companion=%v fanout=%v",
			note.AggWorkKey, note.AggCompanion, note.FanoutWorkKeys)
	}
}

// uiHandwrittenMetadataRuleJSON 界面造出的「手写声明＋元数据文件」规则落档原文
// （实机净室库 rule-profile 条目逐字）：行 1 `{作品标识}`、行 2 `{作品标识}.{任意}`
// （连接词 and）、行 3 `{作品标识}`（连接词 and），元数据文件与作品集聚合都绑
// 手写声明 {第 1 行作品标识}，行 2、行 3 内容绑定→描述。三个占位标记是各行
// 的扩展名附加条件落点（落档原文为空串——该条目每一行都没有扩展名）。
const uiHandwrittenMetadataRuleJSON = `{"schemaVersion":5,"rules":[{"id":"manual-p6-m1","description":"手写声明配对 元数据文件绑手写声明","source":"manual","enabled":true,"workSetAggregation":[{"grouping":{"mode":"identity","identity":[{"row":1,"capture":"work"}]},"nameExtraction":{"row":1,"capture":"work"}}],"metadataFile":{"grouping":{"mode":"identity","identity":[{"row":1,"capture":"work"}]},"contentBindings":[{"row":2,"dimension":"description"},{"row":3,"dimension":"description"}]},"conditions":[
{"condition":{"target":"file","format":{"mode":"template","tpl":"{作品标识}"}@@ROW1EXT@@}},
{"connector":"and","condition":{"target":"file","format":{"mode":"template","tpl":"{作品标识}.{任意}"}@@ROW2EXT@@}},
{"connector":"and","condition":{"target":"file","format":{"mode":"template","tpl":"{作品标识}"}@@ROW3EXT@@}}
],"extractions":[],"synthesis":{"dimPolicies":[],"roles":[]}}]}`

// uiHandwrittenMetadataRule 按 given 扩展名（依次行 1/2/3，空串=不带扩展名）装
// 配界面规则档案并经载入校验解析；任一行载入告警即测试失败。
func uiHandwrittenMetadataRule(t *testing.T, exts [3]string) []RuleEntry {
	t.Helper()
	data := uiHandwrittenMetadataRuleJSON
	for i, ext := range exts {
		frag := ""
		if ext != "" {
			frag = `,"attach":[{"kind":"fileExt","value":"` + ext + `"}]`
		}
		data = strings.ReplaceAll(data, fmt.Sprintf("@@ROW%dEXT@@", i+1), frag)
	}
	profile, errs := ParseProfile([]byte(data))
	if len(errs) != 0 {
		t.Fatalf("界面手写声明规则档案应零拒绝载入: %v", errs)
	}
	return profile.Rules
}

// TestFanoutRowGroupHandwrittenMetadataArchiveShape 手写声明配对对落档形态的
// 依赖锚。标识值取「捕获名跨行汇流、首个能供值的命中行」：行 1 `{作品标识}`
// 不带扩展名时对一切文件成立且排在带点行之前——`200.zh.txt` 被行 1 先命中取成
// `200.zh`（与图片 `200` 不同键、配对不成立），`.txt` 文件同时命中目标行 1
// （扇出键含自身、不作元数据源）；各行带上扩展名（图片行 .jpg、说明行 .txt，
// 与「单图+说明文本」预置同形）时，说明文件只命中说明行，`200.zh.txt` 由带点
// 行先供值得 `200`，与 `200.jpg` 同键配对。
func TestFanoutRowGroupHandwrittenMetadataArchiveShape(t *testing.T) {
	files := aggScanFiles(
		"说明甲/100.jpg", "说明甲/100.txt",
		"说明乙/200.jpg", "说明乙/200.zh.txt",
		"说明丙/300.txt",
	)

	t.Run("各行不带扩展名", func(t *testing.T) {
		conclusions := fanoutRun(t, files, uiHandwrittenMetadataRule(t, [3]string{"", "", ""}))
		dotted := aggConclusionOf(t, conclusions, "说明乙/200.zh.txt")
		if dotted.SetGroupKey != "200.zh" || dotted.AggCompanion ||
			!reflect.DeepEqual(fanoutKeysOf(dotted), []string{"f:说明乙/200.zh.txt"}) {
			t.Fatalf("不带扩展名的首行先供值应取成 200.zh 且 .txt 文件命中目标行: set=%q companion=%v fanout=%v",
				dotted.SetGroupKey, dotted.AggCompanion, dotted.FanoutWorkKeys)
		}
		plain := aggConclusionOf(t, conclusions, "说明甲/100.txt")
		if plain.AggCompanion ||
			!reflect.DeepEqual(fanoutKeysOf(plain), []string{"f:说明甲/100.jpg", "f:说明甲/100.txt"}) {
			t.Fatalf("不带扩展名时 .txt 文件是目标行命中：扇出键含自身、不作元数据源: companion=%v fanout=%v",
				plain.AggCompanion, plain.FanoutWorkKeys)
		}
	})

	t.Run("各行带扩展名", func(t *testing.T) {
		conclusions := fanoutRun(t, files, uiHandwrittenMetadataRule(t, [3]string{".jpg", ".txt", ".txt"}))
		for txt, jpg := range map[string]string{
			"说明甲/100.txt":    "f:说明甲/100.jpg",
			"说明乙/200.zh.txt": "f:说明乙/200.jpg",
		} {
			got := aggConclusionOf(t, conclusions, txt)
			if got.AggWorkKey != "" || !got.AggCompanion || !reflect.DeepEqual(fanoutKeysOf(got), []string{jpg}) {
				t.Fatalf("带扩展名时说明文件 %s 应为元数据源并扇出到同名图片作品: key=%q companion=%v fanout=%v",
					txt, got.AggWorkKey, got.AggCompanion, got.FanoutWorkKeys)
			}
		}
		orphan := aggConclusionOf(t, conclusions, "说明丙/300.txt")
		if orphan.AggCompanion || len(fanoutKeysOf(orphan)) != 0 {
			t.Fatalf("无同名资源的说明文件应按孤文件回退: companion=%v fanout=%v",
				orphan.AggCompanion, orphan.FanoutWorkKeys)
		}
	})
}

// TestAggregationRowGroupRequiredTrackZeroUnitsSilent 必含轨零命中静默锚：组内
// 没有文件命中必含轨行（纯图片文件落在「一图一文本」的标识分组里，正文轨缺席）
// 是必含轨对该组不适用的正常形态——该组不落件、不告警；正文轨在场的组照常成件。
func TestAggregationRowGroupRequiredTrackZeroUnitsSilent(t *testing.T) {
	rules := []RuleEntry{PresetAggregateRules()[2]}
	files := aggScanFiles("配对甲/001.jpg", "配对甲/001.txt", "散图丙/002.jpg")
	conclusions := MergedRecognition(files, rules, nil, nil)
	var warns []string
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex(), adapterWarnf(&warns))

	if len(warns) != 0 {
		t.Fatalf("必含轨零命中的组属正常不适用，不应告警: %v", warns)
	}
	paired := aggConclusionOf(t, conclusions, "配对甲/001.txt")
	if paired.AggWorkKey != "f:配对甲/001.txt" || paired.AggTrackRole != sdkdto.StoreRoleDocument {
		t.Fatalf("正文轨在场的组应照常成件: key=%q role=%q", paired.AggWorkKey, paired.AggTrackRole)
	}
	lone := aggConclusionOf(t, conclusions, "散图丙/002.jpg")
	if lone.AggWorkKey != "" || lone.AggCompanion {
		t.Fatalf("纯图片文件无正文轨不落件（独立作品）: key=%q companion=%v", lone.AggWorkKey, lone.AggCompanion)
	}
}

// TestAggregationRowGroupRequiredTrackMultiUnitsWarns 必含轨多命中告警锚：一
// 目录内多个文件命中同一必含轨行时该组不成立并告警点名规则、分组与单位数；告
// 警经把实参转字串的日志适配层交付时数字仍是数字（不得出现 %!d(string=N)）。
func TestAggregationRowGroupRequiredTrackMultiUnitsWarns(t *testing.T) {
	files := aggScanFiles("甲卷/a.jpg", "甲卷/b.jpg")
	rules := []RuleEntry{aggRowGroupImageRule("agg-dir-multi-units")}
	conclusions := MergedRecognition(files, rules, nil, nil)
	var warns []string
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex(), adapterWarnf(&warns))

	if len(warns) != 1 || !strings.Contains(warns[0], "agg-dir-multi-units") ||
		!strings.Contains(warns[0], "单位数为 2") || strings.Contains(warns[0], "%!") {
		t.Fatalf("必含轨多命中应告警一次且数字打印为数字: %v", warns)
	}
	for _, rel := range []string{"甲卷/a.jpg", "甲卷/b.jpg"} {
		c := aggConclusionOf(t, conclusions, rel)
		if c.AggWorkKey != "" || c.AggCompanion {
			t.Fatalf("必含轨多命中的目录不应成立: %s key=%q companion=%v", rel, c.AggWorkKey, c.AggCompanion)
		}
	}
}

// TestWorkSetAggRowGroupMemberRequiresRuleHit 作品集聚合成员面边界锚：成员必须是
// 所绑规则的命中文件——未命中任何条件行的文件即使带库级群键也不入集；命中平行
// 文件行（非产生行）且带库级群键的文件是规则命中、键落在键集内，入集（连接词
// 不参与聚合面的命中判定，与聚合成作品同一口径）。
func TestWorkSetAggRowGroupMemberRequiresRuleHit(t *testing.T) {
	rule := RuleEntry{
		ID:          "ws-seq-key-parallel-file-row",
		Description: "测试：序列群键条目与平行文件行",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetSeq, Format: condxTpl("{作品标识}_{序号}")}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{任意}"),
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".jpg"}}}},
		},
		WorkSetAggregation: []WorkSetAggregationProduction{{
			Grouping:       &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}}},
			NameExtraction: &NameExtraction{Row: 1, Capture: "work"},
		}},
	}
	files := aggScanFiles("d/作品S_01.jpg", "d/作品S_02.jpg", "d/作品S-03.jpg", "d/未命中.txt")
	conclusions := make([]FileConclusion, len(files))
	for i := range files {
		conclusions[i] = FileConclusion{RelPath: files[i].RelPath, FileName: files[i].FileName, DirRelKey: dirRelKeyOf(files[i].RelPath)}
	}
	conclusions[2].Group = &GroupAnalysis{Key: "作品S"}
	conclusions[3].Group = &GroupAnalysis{Key: "作品S"}
	runAggregationOnConclusions(conclusions, files, files, []RuleEntry{rule}, nil, nil, nil)

	for _, rel := range []string{"d/作品S_01.jpg", "d/作品S_02.jpg", "d/作品S-03.jpg"} {
		c := aggConclusionOf(t, conclusions, rel)
		if c.SetGroupKey != "作品S" || c.SetName != "作品S" || c.SetRuleID != rule.ID {
			t.Fatalf("规则命中且键在键集内的文件应入集: %s set=%q name=%q rule=%q", rel, c.SetGroupKey, c.SetName, c.SetRuleID)
		}
	}
	miss := aggConclusionOf(t, conclusions, "d/未命中.txt")
	if miss.SetGroupKey != "" || miss.SetName != "" || miss.SetRuleID != "" {
		t.Fatalf("未命中规则的文件即使带库级群键也不得入集: set=%q name=%q rule=%q",
			miss.SetGroupKey, miss.SetName, miss.SetRuleID)
	}
}
