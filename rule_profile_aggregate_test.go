// 聚合规则文件轨行「模板尾部误写扩展名」的载入校验锚：文件主体模板对去
// 扩展名的文件名求值（FileSubjectPattern 契约，rule_condition_subject.go），
// 尾字面 .ext（ext ∈ 白名单 knownFileExtsForTplTail）恒不命中——载入拒收并
// 指路 fileExt 谓词；白名单外点段（.part1）、正确拆分形态与 dir 目标行不受
// 影响。档案级断言锚「条目跳过 + 带规则 ID 告警」的既有坏条目呈现。
//
// 另附「作品集聚合」产出四的阶段1 档案结构锚：多条目（各自分组绑定+集名来源）、
// 逐条校验与 1:1（含跨「聚合成作品」）、单条对象旧形态拒收（不做兼容）、无集名
// 来源的保存闸门，见文件末节。
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// aggTailExtEntry 构造单轨行聚合规则条目（JSON 中间形态）：target/tpl/attach
// 定轨行，identityCapture 定标识声明（取行 1 捕获）。
func aggTailExtEntry(id string, target CondxTargetKind, tpl string, attach []CondxAttach, identityCapture string) *ruleEntryJSON {
	return &ruleEntryJSON{
		ID:          id,
		Description: "文件轨行模板尾部扩展名测试",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{{Condition: &Condx{
			Target: target,
			Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
			Attach: attach,
		}}},
		Identity:  []IdentityPart{{Row: 1, Capture: identityCapture}},
		Aggregate: &AggregateProduction{WorkType: "video", Tracks: []TrackAssign{{Row: 1, Role: "videoMain"}}},
	}
}

// TestValidateAggregateFileTplTailExt 尾部扩展名白名单判定的五形态：用户真实
// 形态与带锚字幕形态拒；白名单外点段、正确拆分形态（模板无尾扩展名+fileExt
// 谓词）、dir 目标行过。
func TestValidateAggregateFileTplTailExt(t *testing.T) {
	// 用户真实形态：{任意} - {任意}({标题}).mp4——尾字面 ).mp4 命中白名单，拒
	err := validateAggregateEntry(aggTailExtEntry("agg-dead-mp4", CondxTargetFile, "{任意} - {任意}({标题}).mp4", nil, "title"))
	if err == nil || !strings.Contains(err.Error(), "模板尾部误写扩展名 .mp4") || !strings.Contains(err.Error(), "「扩展名是」附加条件承载扩展名") {
		t.Fatalf("用户真实形态应拒并指路扩展名谓词: %v", err)
	}
	// {作品}.{任意}.srt——尾字面 .srt 命中白名单，拒
	if err = validateAggregateEntry(aggTailExtEntry("agg-dead-srt", CondxTargetFile, "{作品}.{任意}.srt", nil, "work")); err == nil || !strings.Contains(err.Error(), "模板尾部误写扩展名 .srt") {
		t.Fatalf("{作品}.{任意}.srt 应拒: %v", err)
	}
	// {标题}.part1——白名单外点段（stem 内合法点段），过
	if err = validateAggregateEntry(aggTailExtEntry("agg-part1", CondxTargetFile, "{标题}.part1", nil, "title")); err != nil {
		t.Fatalf("{标题}.part1 不应误报: %v", err)
	}
	// 正确拆分形态：模板无尾扩展名 + fileExt 谓词承载扩展名，过
	rj := aggTailExtEntry("agg-split", CondxTargetFile, "{作品}.{任意}",
		[]CondxAttach{{Kind: CondxAttachFileExt, Value: ".srt"}}, "work")
	if err = validateAggregateEntry(rj); err != nil {
		t.Fatalf("正确拆分形态不应拒: %v", err)
	}
	// dir 目标行不受影响：目录名不去扩展名，模板带点尾合法，过
	if err = validateAggregateEntry(aggTailExtEntry("agg-dir-tail", CondxTargetDir, "{任意} - {任意}({标题}).mp4", nil, "title")); err != nil {
		t.Fatalf("dir 目标行不应受影响: %v", err)
	}
}

// TestValidateAggregateTailExtLoadSkip 档案载入呈现：死形态条目跳过（不阻塞
// 合法条目）+ 跳过告警携带规则 ID 与指路文案——既有坏条目出口（ParseProfile
// 逐条跳过+告警、规则管理视图 Warnings 透出）。
func TestValidateAggregateTailExtLoadSkip(t *testing.T) {
	dead := aggTailExtEntry("agg-dead-mp4", CondxTargetFile, "{任意} - {任意}({标题}).mp4", nil, "title")
	good := aggTailExtEntry("agg-split", CondxTargetFile, "{作品}.{任意}",
		[]CondxAttach{{Kind: CondxAttachFileExt, Value: ".srt"}}, "work")
	data, mErr := json.Marshal(&ruleProfileJSON{SchemaVersion: CurrentSchemaVersion, Rules: []ruleEntryJSON{*dead, *good}})
	if mErr != nil {
		t.Fatalf("序列化档案失败: %v", mErr)
	}
	profile, errs := ParseProfile(data)
	if profile == nil || len(profile.Rules) != 1 || profile.Rules[0].ID != "agg-split" {
		t.Fatalf("死形态条目应跳过、合法条目照常载入: %+v", profile)
	}
	if len(errs) != 1 {
		t.Fatalf("应恰一条跳过告警，实际 %d 条: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !strings.Contains(msg, `跳过规则 "agg-dead-mp4"`) || !strings.Contains(msg, "模板尾部误写扩展名 .mp4") {
		t.Fatalf("跳过告警应携带规则 ID 与尾部扩展名文案: %s", msg)
	}
}

// —— 元数据类别行式底盘的模板尾内联扩展名守卫（决策8 甲案）——

// metaTailExtEntry 构造元数据类别规则条目（JSON 中间形态）：行式与决策8 剧本
// 形态对齐（行1 资源行+行2 说明行带显式连接词）。
func metaTailExtEntry(id string, row2Tpl string, row2Attach []CondxAttach) *ruleEntryJSON {
	return &ruleEntryJSON{
		ID:          id,
		Description: "元数据行模板尾部扩展名测试",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{
				Target: CondxTargetSeq,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品标识}_{序号}"},
				Attach: []CondxAttach{{Kind: CondxAttachSeqMemberType, Types: []string{"image"}}},
			}},
			{Connector: "and", Condition: &Condx{
				Target: CondxTargetFile,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: row2Tpl},
				Attach: row2Attach,
			}},
		},
	}
}

// TestValidateMetadataFileTplTailExt 元数据类别守卫三形态（决策8 甲案·与聚合
// 轨行守卫同口径）：内联扩展名（剧本A 原行2 写法 {作品标识}_{序号}.txt）拒收
// 并指路「扩展名是」附加条件；正确拆分形态（模板+fileExt 附加）放行；聚合类
// 别路径零变化（聚合守卫文案含「文件轨行」措辞、由 validateAggregateEntry 拦）。
func TestValidateMetadataFileTplTailExt(t *testing.T) {
	// 内联形态拒：{作品标识}_{序号}.txt（尾字面 .txt 命中白名单）
	_, err := ruleFromJSON(metaTailExtEntry("meta-dead-txt", "{作品标识}_{序号}.txt", nil))
	if err == nil || !strings.Contains(err.Error(), "第 2 行的文件名行模板尾部误写扩展名 .txt") ||
		!strings.Contains(err.Error(), "改用「扩展名是」附加条件承载扩展名") {
		t.Fatalf("内联扩展名应拒并指路扩展名谓词: %v", err)
	}
	// 正确拆分形态放行：模板无尾扩展名 + fileExt 谓词承载（决策8 后剧本行式）
	entry := metaTailExtEntry("meta-split", "{作品标识}_{序号}",
		[]CondxAttach{{Kind: CondxAttachFileExt, Value: ".txt"}})
	entry.MetadataFile = &MetadataFileProduction{
		Grouping:        &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}, {Row: 1, Capture: "seq"}}},
		ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
	}
	if _, err = ruleFromJSON(entry); err != nil {
		t.Fatalf("正确拆分形态不应拒: %v", err)
	}
	// dir/seq 目标行带点尾合法（名不去扩展名）——行1 序列模板不动、直接复用
	// 上面放行结论；dir 行单独验证：
	dirEntry := metaTailExtEntry("meta-dir-tail", "{作品标识}.mp4", nil)
	dirEntry.Conditions[1].Condition.Target = CondxTargetDir
	if _, err = ruleFromJSON(dirEntry); err != nil {
		t.Fatalf("dir 目标行带点尾不应误报: %v", err)
	}
	// 聚合路径零变化：聚合条目同形态由聚合守卫拦截（「文件条件行」措辞——
	// 决策10 词汇 sweep：轨行→条件行）
	aggErr := validateAggregateEntry(aggTailExtEntry("agg-dead-mp4", CondxTargetFile, "{任意} - {任意}({标题}).mp4", nil, "title"))
	if aggErr == nil || !strings.Contains(aggErr.Error(), "文件条件行模板尾部误写扩展名 .mp4") {
		t.Fatalf("聚合守卫应维持原措辞拦截: %v", aggErr)
	}
}

// —— 作品集聚合（产出四）多条档案结构（阶段1） ——

// wsAggConditions 两条条件行底盘：行1 文件「{集名}_{序号}」（声明集名/序号捕获）、
// 行2 目录「作者{作者}」（声明作者捕获）——多条目、复合分组键与目录行集名共用。
func wsAggConditions() []CondxRow {
	return []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{集名}_{序号}"}}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "作者{作者}"}}},
	}
}

// wsMultiEntryRule 两条目的作品集聚合规则：条目1＝行1 集名捕获同时作分组键与
// 集名；条目2＝行2 目录行产生的所在目录分组 + 行2 作者捕获作集名。
func wsMultiEntryRule() RuleEntry {
	return RuleEntry{
		ID:          "ws-multi",
		Description: "测试：多条作品集聚合",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions:  wsAggConditions(),
		Extractions: []ExtractionAction{{
			Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1,
		}},
		WorkSetAggregation: []WorkSetAggregationProduction{
			{
				Grouping: &GroupingBinding{
					Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "setname"}},
				},
				NameExtraction: &NameExtraction{Row: 1, Capture: "setname"},
			},
			{
				Grouping:       &GroupingBinding{Mode: GroupingModeDirectory, Row: 2},
				NameExtraction: &NameExtraction{Row: 2, Capture: "author"},
			},
		},
	}
}

// wsProfileData 把规则序列化为单条档案 JSON（走生产写档形态）。
func wsProfileData(t *testing.T, rule RuleEntry) []byte {
	t.Helper()
	data, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{rule}})
	if err != nil {
		t.Fatalf("序列化测试档案失败: %v", err)
	}
	return data
}

// wsRejectOne 断言单条规则档案载入被拒（规则跳过）且告警文本含全部目标子串。
func wsRejectOne(t *testing.T, data []byte, wants ...string) {
	t.Helper()
	profile, errs := ParseProfile(data)
	if profile == nil || len(profile.Rules) != 0 {
		t.Fatalf("非法作品集聚合条目应被载入拒绝，实际载入 %d 条: %v", len(profile.Rules), errs)
	}
	if len(errs) == 0 {
		t.Fatal("载入拒绝应带告警文本")
	}
	for _, want := range wants {
		if !strings.Contains(errs[0].Error(), want) {
			t.Fatalf("载入告警应含 %q: %v", want, errs[0])
		}
	}
}

// TestWorkSetAggMultiEntryRoundTrip 多条目档案结构锚：新档以条目数组落档
// （workSetAggregation:[{…}]），每条各自携带分组绑定与集名来源；载入后两条目
// 原样保真、再序列化字节等价。
func TestWorkSetAggMultiEntryRoundTrip(t *testing.T) {
	data := wsProfileData(t, wsMultiEntryRule())
	if !strings.Contains(string(data), `"workSetAggregation":[{`) {
		t.Fatalf("多条目应以数组形态落档: %s", data)
	}
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 {
		t.Fatalf("多条目规则应正常载入: errs=%v", errs)
	}
	entries := profile.Rules[0].WorkSetAggregation
	if len(entries) != 2 {
		t.Fatalf("载入后应保有两条目，实际 %d: %+v", len(entries), entries)
	}
	first, second := entries[0], entries[1]
	if first.Grouping == nil || first.Grouping.Mode != GroupingModeIdentity ||
		len(first.Grouping.Identity) != 1 || first.Grouping.Identity[0].Row != 1 ||
		first.Grouping.Identity[0].Capture != "setname" ||
		first.NameExtraction == nil || first.NameExtraction.Row != 1 || first.NameExtraction.Capture != "setname" {
		t.Fatalf("条目1 的分组绑定/集名来源未随档案往返保真: %+v", first)
	}
	if second.Grouping == nil || second.Grouping.Mode != GroupingModeDirectory || second.Grouping.Row != 2 ||
		len(second.Grouping.Identity) != 0 ||
		second.NameExtraction == nil || second.NameExtraction.Row != 2 || second.NameExtraction.Capture != "author" {
		t.Fatalf("条目2 的分组绑定（含产生行）/集名来源未随档案往返保真: %+v", second)
	}
	if !strings.Contains(string(data), `"grouping":{"mode":"directory","row":2}`) {
		t.Fatalf("所在目录分组应以产生行落档（row）: %s", data)
	}
	again := wsProfileData(t, profile.Rules[0])
	if string(again) != string(data) {
		t.Fatalf("多条目档案写读往返应字节等价\n首次: %s\n读回: %s", data, again)
	}
}

// TestWorkSetAggEntryValidation 逐条校验锚（校验①②）：每条须有分组绑定、每条须
// 能从 nameExtraction 指到的条件行捕获取到集名（行号在范围内、捕获名非空且在该
// 行声明）——违规条目载入跳过+告警（编辑保存同拒）。
func TestWorkSetAggEntryValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*RuleEntry)
		wants  []string
	}{
		{"缺归集依据", func(r *RuleEntry) { r.WorkSetAggregation[1].Grouping = nil },
			[]string{"第 2 条", "归集依据"}},
		{"缺集名来源", func(r *RuleEntry) { r.WorkSetAggregation[1].NameExtraction = nil },
			[]string{"第 2 条", "集名"}},
		{"集名行号越界", func(r *RuleEntry) {
			r.WorkSetAggregation[0].NameExtraction = &NameExtraction{Row: 9, Capture: "setname"}
		}, []string{"行号 9 越界", "共 2 行"}},
		{"集名捕获未在该行声明", func(r *RuleEntry) {
			r.WorkSetAggregation[0].NameExtraction = &NameExtraction{Row: 2, Capture: "setname"}
		}, []string{"未在第 2 行声明"}},
		{"集名捕获名缺失", func(r *RuleEntry) {
			r.WorkSetAggregation[0].NameExtraction = &NameExtraction{Row: 1}
		}, []string{"缺少捕获名"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := wsMultiEntryRule()
			tc.mutate(&rule)
			wsRejectOne(t, wsProfileData(t, rule), tc.wants...)
		})
	}
}

// TestWorkSetAggGroupingOneToOne 1:1 校验锚（校验③）：同一分组项不得被两个聚合
// 条目选中（同规则两条目互斥——标识分组按键部分、所在目录分组按产生行），也不得
// 与「聚合成作品」的分组绑定相同（标识声明 identity 的等值形态，或目录分组绑定
// aggregate.grouping 的产生行）；两条目录行可各绑一个条目（1:1 按产生行区分）；
// 分组不同的并用形态照常载入。
func TestWorkSetAggGroupingOneToOne(t *testing.T) {
	t.Run("两条目同一标识分组被拒", func(t *testing.T) {
		rule := wsMultiEntryRule()
		rule.WorkSetAggregation[1].Grouping = &GroupingBinding{
			Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "setname"}},
		}
		rule.WorkSetAggregation[1].NameExtraction = &NameExtraction{Row: 1, Capture: "setname"}
		wsRejectOne(t, wsProfileData(t, rule), "第 1 条", "第 2 条", "同一分组", "1:1")
	})
	t.Run("两条目绑同一目录行被拒", func(t *testing.T) {
		rule := wsMultiEntryRule()
		rule.WorkSetAggregation[0].Grouping = &GroupingBinding{Mode: GroupingModeDirectory, Row: 2}
		rule.WorkSetAggregation[0].NameExtraction = &NameExtraction{Row: 2, Capture: "author"}
		wsRejectOne(t, wsProfileData(t, rule), "第 1 条", "第 2 条", "同一分组", "1:1")
	})
	t.Run("两条目录行可各绑一个条目", func(t *testing.T) {
		rule := wsMultiEntryRule()
		rule.Conditions = append(rule.Conditions, CondxRow{Connector: "or", Condition: &Condx{
			Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "刊物{标签}"},
		}})
		rule.WorkSetAggregation[0].Grouping = &GroupingBinding{Mode: GroupingModeDirectory, Row: 3}
		rule.WorkSetAggregation[0].NameExtraction = &NameExtraction{Row: 3, Capture: "tag"}
		data := wsProfileData(t, rule)
		profile, errs := ParseProfile(data)
		if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 {
			t.Fatalf("各绑一条目录行的两条目应正常载入: errs=%v", errs)
		}
		entries := profile.Rules[0].WorkSetAggregation
		if len(entries) != 2 || entries[0].Grouping.Row != 3 || entries[1].Grouping.Row != 2 {
			t.Fatalf("两条目应各保其产生行（第 3 行/第 2 行）: %+v", entries)
		}
		if again := wsProfileData(t, profile.Rules[0]); string(again) != string(data) {
			t.Fatalf("各绑一条目录行的档案写读往返应字节等价\n首次: %s\n读回: %s", data, again)
		}
	})

	aggConditions := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}_{序号}"}}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "作者{作者}"}}},
		{Connector: "or", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "刊物{标签}"}}},
	}
	aggPlusWorkset := func(grouping *GroupingBinding, name *NameExtraction) RuleEntry {
		return RuleEntry{
			ID:          "ws-cross",
			Description: "测试：聚合产出与作品集聚合并用",
			Source:      RuleSourceManual,
			Enabled:     true,
			Category:    RuleCategoryAggregate,
			Conditions:  aggConditions,
			Identity:    []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{
				WorkType:     "video",
				Tracks:       []TrackAssign{{Row: 1, Role: "videoMain"}},
				CompanionUse: CompanionUseTrack,
			},
			WorkSetAggregation: []WorkSetAggregationProduction{{Grouping: grouping, NameExtraction: name}},
		}
	}
	t.Run("与聚合成作品的分组绑定相同被拒", func(t *testing.T) {
		rule := aggPlusWorkset(
			&GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}}},
			&NameExtraction{Row: 1, Capture: "work"},
		)
		wsRejectOne(t, wsProfileData(t, rule), "聚合成作品", "同一分组不得同时供两种聚合使用")
	})
	t.Run("分组不同可并用", func(t *testing.T) {
		rule := aggPlusWorkset(
			&GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 2, Capture: "author"}}},
			&NameExtraction{Row: 2, Capture: "author"},
		)
		profile, errs := ParseProfile(wsProfileData(t, rule))
		if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 {
			t.Fatalf("聚合产出与分组不同的作品集聚合条目应正常并用: errs=%v", errs)
		}
		entries := profile.Rules[0].WorkSetAggregation
		if len(entries) != 1 || entries[0].Grouping == nil ||
			entries[0].Grouping.Mode != GroupingModeIdentity ||
			entries[0].NameExtraction == nil || entries[0].NameExtraction.Row != 2 {
			t.Fatalf("并用条目的分组绑定/集名来源未保真: %+v", entries)
		}
	})

	// 「聚合成作品」绑目录分组（aggregate.grouping）时的跨聚合 1:1：判据取该目录
	// 分组绑定的产生行——作品集聚合条目绑同一目录行即冲突，绑另一条目录行可并用。
	dirAggPlusWorkset := func(entryRow int, name *NameExtraction) RuleEntry {
		rule := aggPlusWorkset(&GroupingBinding{Mode: GroupingModeDirectory, Row: entryRow}, name)
		rule.Identity = nil
		rule.Aggregate.Grouping = &GroupingBinding{Mode: GroupingModeDirectory, Row: 2}
		return rule
	}
	t.Run("与聚合成作品的目录分组同产生行被拒", func(t *testing.T) {
		wsRejectOne(t, wsProfileData(t, dirAggPlusWorkset(2, &NameExtraction{Row: 2, Capture: "author"})),
			"聚合成作品", "同一分组不得同时供两种聚合使用")
	})
	t.Run("与聚合成作品的目录分组产生行不同可并用", func(t *testing.T) {
		profile, errs := ParseProfile(wsProfileData(t, dirAggPlusWorkset(3, &NameExtraction{Row: 3, Capture: "tag"})))
		if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 {
			t.Fatalf("产生行不同的目录分组应可分供两种聚合: errs=%v", errs)
		}
		if got := profile.Rules[0]; len(got.Identity) != 0 || got.Aggregate.Grouping == nil || got.Aggregate.Grouping.Row != 2 ||
			len(got.WorkSetAggregation) != 1 || got.WorkSetAggregation[0].Grouping.Row != 3 {
			t.Fatalf("两种聚合应各保其目录分组产生行: identity=%+v aggregate=%+v workset=%+v",
				got.Identity, got.Aggregate, got.WorkSetAggregation)
		}
	})
}

// TestWorkSetAggSingleObjectRejected 旧形态拒收：workSetAggregation 写成单条对象
// （{…}）不做兼容转换，载入按明确错误拒收（须为条目数组）。
func TestWorkSetAggSingleObjectRejected(t *testing.T) {
	doc := []byte(fmt.Sprintf(`{"schemaVersion":%d,"rules":[{"id":"ws-single","description":"单条对象","source":"manual","enabled":true,"workSetAggregation":{"grouping":{"mode":"directory"}},"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}"}}}],"extractions":[]}]}`, CurrentSchemaVersion))
	profile, errs := ParseProfile(doc)
	loaded := 0
	if profile != nil {
		loaded = len(profile.Rules)
	}
	if loaded != 0 {
		t.Fatalf("单条对象形态应被载入拒绝，实际载入 %d 条: %v", loaded, errs)
	}
	if len(errs) == 0 {
		t.Fatal("单条对象形态拒收应带告警文本")
	}
	if !strings.Contains(errs[0].Error(), "工作集聚合须为条目数组") {
		t.Fatalf("拒收告警应点名须为条目数组: %v", errs[0])
	}
	// 拒收发生在反序列化层：整份档案按反序列化失败拒载（不是逐条跳过）。
	if !strings.Contains(errs[0].Error(), "规则档案反序列化失败") {
		t.Fatalf("单条对象拒收应走档案反序列化失败通道: %v", errs[0])
	}
}

// TestWorkSetAggSaveRejectsMissingNameExtraction 保存闸门：无集名来源（条目缺
// NameExtraction）的条目编辑保存被拦且不落盘（集名只有卡内一个出口）。条目的
// 所在目录分组绑定指向行 2 目录行。
func TestWorkSetAggSaveRejectsMissingNameExtraction(t *testing.T) {
	rj := &ruleEntryJSON{
		ID:          "ws-no-name",
		Description: "测试：条目无集名来源",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{集名}"}}},
		},
		Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromCapture, CaptureName: "作品", Row: 1}},
		WorkSetAggregation: &workSetAggregationField{Entries: []WorkSetAggregationProduction{{
			Grouping: &GroupingBinding{Mode: GroupingModeDirectory, Row: 2},
		}}},
	}
	saveJSON, err := json.Marshal(rj)
	if err != nil {
		t.Fatalf("序列化编辑规则失败: %v", err)
	}
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	if _, err := mgr.ApplySaveRule(saveJSON); err == nil || !strings.Contains(err.Error(), "集名") {
		t.Fatalf("无集名来源的条目保存应被拦并点名集名: %v", err)
	}
	if _, stored := store.values[RuleProfileKVKey]; stored {
		t.Fatal("保存被拦不应落盘档案")
	}
}

// TestWorkSetAggFieldAbsence 现行语义锚（非旧档兼容）：无作品集聚合产出的规则
// 不落 workSetAggregation 键、往返字节等价；null 与空数组两形态按无条目读入、
// 不报错（null/字段缺席＝无条目是本产出的一等语义）。
func TestWorkSetAggFieldAbsence(t *testing.T) {
	plain := RuleEntry{
		ID:          "plain-meta",
		Description: "测试：无作品集聚合产出",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"}}},
		},
		Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromCapture, CaptureName: "work", Row: 1}},
	}
	data := wsProfileData(t, plain)
	if strings.Contains(string(data), "workSetAggregation") {
		t.Fatalf("无产出四的规则不应落 workSetAggregation 键: %s", data)
	}
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 || len(profile.Rules[0].WorkSetAggregation) != 0 {
		t.Fatalf("无产出四的规则应照常载入且无条目: errs=%v", errs)
	}
	if again := wsProfileData(t, profile.Rules[0]); string(again) != string(data) {
		t.Fatalf("无产出四的规则写读往返应字节等价\n首次: %s\n读回: %s", data, again)
	}

	for _, raw := range []string{"null", "[]"} {
		doc := []byte(fmt.Sprintf(`{"schemaVersion":%d,"rules":[{"id":"ws-empty","description":"空卡","source":"manual","enabled":true,"workSetAggregation":%s,"conditions":[{"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}"}}}],"extractions":[{"dimension":"author","from":"capture","captureName":"work","row":1}]}]}`, CurrentSchemaVersion, raw))
		loaded, loadErrs := ParseProfile(doc)
		if len(loadErrs) != 0 || loaded == nil || len(loaded.Rules) != 1 || len(loaded.Rules[0].WorkSetAggregation) != 0 {
			t.Fatalf("workSetAggregation=%s 应按无条目兼容载入: errs=%v", raw, loadErrs)
		}
	}
}

// —— 所在目录分组绑定的产生行（档案结构 / 校验 / 存量归一）与「聚合成作品」的
// 分组绑定 ——

// rjProfileData 把规则中间结构序列化为档案 JSON（不经生产写档校验，可表达存量
// 落档形态）。
func rjProfileData(t *testing.T, rules ...ruleEntryJSON) []byte {
	t.Helper()
	data, err := json.Marshal(&ruleProfileJSON{SchemaVersion: CurrentSchemaVersion, Rules: rules})
	if err != nil {
		t.Fatalf("序列化测试档案失败: %v", err)
	}
	return data
}

// errTexts 把告警列表拼成一段文本（子串断言用）。
func errTexts(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n")
}

// dirRowMappingRule 构造映射单用的「元数据文件」规则（JSON 中间形态）：行1 文件
// `{作品}` + `.png`、行2 目录 `{集名}`、行3 文件 `{任意}` + `.txt`（行间 and）；内容
// 绑定=行3 → 描述；分组绑定由调用方给出。
func dirRowMappingRule(id string, grouping *GroupingBinding) *ruleEntryJSON {
	return &ruleEntryJSON{
		ID:          id,
		Description: "测试：所在目录分组产生行",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"},
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".png"}}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{集名}"}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{任意}"},
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".txt"}}}},
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        grouping,
			ContentBindings: []ContentBinding{{Row: 3, Dimension: DimDescription}},
		},
	}
}

// TestGroupingDirectoryRowShape 所在目录分组绑定的产生行校验（严格形态：编辑保存/
// 命中预览入口）：未指定产生行、产生行指向非目录行、产生行越界三种形态拒收并点名
// 原因；产生行指向目录行即合法——以 row 落档、载入零告警、写读往返字节等价；标识
// 分组不落 row 键。
func TestGroupingDirectoryRowShape(t *testing.T) {
	strictErr := func(grouping *GroupingBinding) string {
		t.Helper()
		_, err := ruleFromJSON(dirRowMappingRule("map-dir", grouping))
		if err == nil {
			t.Fatalf("分组绑定 %+v 应被严格形态拒绝", grouping)
		}
		return err.Error()
	}
	if msg := strictErr(&GroupingBinding{Mode: GroupingModeDirectory}); !strings.Contains(msg, "缺少产生行") {
		t.Fatalf("未指定产生行应拒并点名: %s", msg)
	}
	if msg := strictErr(&GroupingBinding{Mode: GroupingModeDirectory, Row: 1}); !strings.Contains(msg, "不是目录条件行") {
		t.Fatalf("产生行指向文件行应拒并点名: %s", msg)
	}
	if msg := strictErr(&GroupingBinding{Mode: GroupingModeDirectory, Row: 4}); !strings.Contains(msg, "越界") {
		t.Fatalf("产生行越界应拒并点名: %s", msg)
	}
	entry, err := ruleFromJSON(dirRowMappingRule("map-dir", &GroupingBinding{Mode: GroupingModeDirectory, Row: 2}))
	if err != nil {
		t.Fatalf("产生行指向目录行应载入: %v", err)
	}
	data := wsProfileData(t, *entry)
	if !strings.Contains(string(data), `"grouping":{"mode":"directory","row":2}`) {
		t.Fatalf("所在目录分组应以 row 落档: %s", data)
	}
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || len(profile.Rules) != 1 || profile.Rules[0].MetadataFile.Grouping.Row != 2 {
		t.Fatalf("新档载入应零告警并保真产生行: errs=%v", errs)
	}
	if again := wsProfileData(t, profile.Rules[0]); string(again) != string(data) {
		t.Fatalf("新档写读往返应字节等价\n首次: %s\n读回: %s", data, again)
	}
	identityEntry, err := ruleFromJSON(dirRowMappingRule("map-identity", &GroupingBinding{
		Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}},
	}))
	if err != nil {
		t.Fatalf("标识分组应载入: %v", err)
	}
	if data := wsProfileData(t, *identityEntry); strings.Contains(string(data), `"row":0`) || !strings.Contains(string(data), `"mode":"identity"`) {
		t.Fatalf("标识分组不应落 row 键: %s", data)
	}
}

// TestGroupingDirectoryRowLegacyLoad 存量所在目录分组绑定（只写 mode=directory、无
// row）的载入期归一：有目录行——补为首个目录行的行号并告警（提示核对该行是否还会
// 命中；多目录行时点名只继承第 1 行），补行结果随档落定（再载入零告警）；无目录
// 行——跳过所附产出（「元数据文件」整项 / 作品集聚合该条目）并告警点名「请补一条
// 目录行，或改用配对分组」，规则本身照常载入、落档不再写该产出。旧配对键
// （metadataFile.pairing）的所在目录分组走同一条归一。
func TestGroupingDirectoryRowLegacyLoad(t *testing.T) {
	t.Run("单目录行补行号", func(t *testing.T) {
		profile, errs := ParseProfile(rjProfileData(t, *dirRowMappingRule("map-legacy", &GroupingBinding{Mode: GroupingModeDirectory})))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 {
			t.Fatalf("存量所在目录分组应补行号后载入并恰给一条告警: rules=%d errs=%v", len(profile.Rules), errs)
		}
		msg := errs[0].Error()
		if !strings.Contains(msg, `规则 "map-legacy"`) || !strings.Contains(msg, "已按首个目录行补为第 2 行") ||
			!strings.Contains(msg, "请检查该目录行是否还会命中") || strings.Contains(msg, "只继承第 1 行") {
			t.Fatalf("补行告警应点名规则、补到的行号与核对提示（单目录行不提收窄）: %s", msg)
		}
		if mf := profile.Rules[0].MetadataFile; mf == nil || mf.Grouping == nil || mf.Grouping.Row != 2 {
			t.Fatalf("载入后分组绑定应带产生行 2: %+v", mf)
		}
		again := wsProfileData(t, profile.Rules[0])
		if !strings.Contains(string(again), `"grouping":{"mode":"directory","row":2}`) {
			t.Fatalf("补行结果应随档落定: %s", again)
		}
		if _, errs2 := ParseProfile(again); len(errs2) != 0 {
			t.Fatalf("补行后的档案再载入应零告警: %v", errs2)
		}
	})
	t.Run("多目录行只继承第 1 行", func(t *testing.T) {
		rj := dirRowMappingRule("map-legacy-multi", &GroupingBinding{Mode: GroupingModeDirectory})
		rj.Conditions = append(rj.Conditions, CondxRow{Connector: "or", Condition: &Condx{
			Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "作者{作者}"},
		}})
		profile, errs := ParseProfile(rjProfileData(t, *rj))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 {
			t.Fatalf("多目录行的存量绑定应补行号后载入并恰给一条告警: rules=%d errs=%v", len(profile.Rules), errs)
		}
		msg := errs[0].Error()
		if !strings.Contains(msg, "补为第 2 行") || !strings.Contains(msg, "本规则共 2 条目录行") ||
			!strings.Contains(msg, "只继承第 1 行") || !strings.Contains(msg, "第 4 行的目录不入该分组") {
			t.Fatalf("多目录行的补行告警应点名收窄: %s", msg)
		}
		if row := profile.Rules[0].MetadataFile.Grouping.Row; row != 2 {
			t.Fatalf("多目录行应只继承第 1 条目录行，实际产生行 %d", row)
		}
	})
	t.Run("无目录行跳过元数据文件产出", func(t *testing.T) {
		rj := dirRowMappingRule("map-legacy-nodir", &GroupingBinding{Mode: GroupingModeDirectory})
		rj.Conditions = []CondxRow{rj.Conditions[0], rj.Conditions[2]}
		rj.MetadataFile.ContentBindings = []ContentBinding{{Row: 2, Dimension: DimDescription}}
		profile, errs := ParseProfile(rjProfileData(t, *rj))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 {
			t.Fatalf("无目录行的存量绑定应跳过产出、规则照常载入并恰给一条告警: rules=%d errs=%v", len(profile.Rules), errs)
		}
		msg := errs[0].Error()
		if !strings.Contains(msg, "已跳过该产出") || !strings.Contains(msg, "请补一条目录行，或改用配对分组") {
			t.Fatalf("跳过产出的告警应点名处置: %s", msg)
		}
		if profile.Rules[0].MetadataFile != nil {
			t.Fatalf("被跳过的产出不应载入: %+v", profile.Rules[0].MetadataFile)
		}
		again := wsProfileData(t, profile.Rules[0])
		if strings.Contains(string(again), "metadataFile") {
			t.Fatalf("被跳过的产出不应再落档: %s", again)
		}
		if _, errs2 := ParseProfile(again); len(errs2) != 0 {
			t.Fatalf("跳过产出后的档案再载入应零告警: %v", errs2)
		}
	})
	t.Run("旧配对键的所在目录分组同样补行", func(t *testing.T) {
		data := string(rjProfileData(t, *dirRowMappingRule("map-legacy-pairing", &GroupingBinding{Mode: GroupingModeDirectory})))
		data = strings.Replace(data, `"grouping":{"mode":"directory"}`, `"pairing":{"mode":"directory"}`, 1)
		profile, errs := ParseProfile([]byte(data))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 || !strings.Contains(errs[0].Error(), "补为第 2 行") {
			t.Fatalf("旧配对键的所在目录分组应经键位归一后补行: rules=%d errs=%v", len(profile.Rules), errs)
		}
		if mf := profile.Rules[0].MetadataFile; mf == nil || mf.Grouping == nil || mf.Grouping.Row != 2 {
			t.Fatalf("旧配对键归一后应落分组绑定并带产生行 2: %+v", mf)
		}
	})
	t.Run("存量作品集聚合条目补行号", func(t *testing.T) {
		rj := dirRowMappingRule("map-legacy-ws", &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}}})
		rj.WorkSetAggregation = &workSetAggregationField{Entries: []WorkSetAggregationProduction{{
			Grouping:       &GroupingBinding{Mode: GroupingModeDirectory},
			NameExtraction: &NameExtraction{Row: 2, Capture: "setname"},
		}}}
		profile, errs := ParseProfile(rjProfileData(t, *rj))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 || !strings.Contains(errs[0].Error(), "作品集聚合第 1 条") ||
			!strings.Contains(errs[0].Error(), "补为第 2 行") {
			t.Fatalf("存量条目应补行号后保留并恰给一条点名条目的告警: rules=%d errs=%v", len(profile.Rules), errs)
		}
		entries := profile.Rules[0].WorkSetAggregation
		if len(entries) != 1 || entries[0].Grouping.Row != 2 {
			t.Fatalf("存量条目应带产生行 2 载入: %+v", entries)
		}
	})
	t.Run("集名不在产生行的存量条目被跳过", func(t *testing.T) {
		rj := dirRowMappingRule("map-legacy-ws-drop", &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}}})
		rj.WorkSetAggregation = &workSetAggregationField{Entries: []WorkSetAggregationProduction{
			{Grouping: &GroupingBinding{Mode: GroupingModeDirectory}, NameExtraction: &NameExtraction{Row: 1, Capture: "work"}},
			{Grouping: &GroupingBinding{Mode: GroupingModeDirectory, Row: 2}, NameExtraction: &NameExtraction{Row: 2, Capture: "setname"}},
		}}
		profile, errs := ParseProfile(rjProfileData(t, *rj))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 2 {
			t.Fatalf("集名不在产生行的存量条目应被跳过（补行+跳过两条告警），其余条目保留: rules=%d errs=%v", len(profile.Rules), errs)
		}
		msg := errTexts(errs)
		if !strings.Contains(msg, "第 1 条的集名来源（第 1 行·work）不在所绑目录分组的产生行上（产生行=第 2 行）") ||
			!strings.Contains(msg, "已跳过该条目") {
			t.Fatalf("跳过告警应点名条目、集名位置与产生行: %s", msg)
		}
		entries := profile.Rules[0].WorkSetAggregation
		if len(entries) != 1 || entries[0].Grouping.Row != 2 || entries[0].NameExtraction.Capture != "setname" {
			t.Fatalf("只应保留集名在产生行上的条目: %+v", entries)
		}
		again := wsProfileData(t, profile.Rules[0])
		if _, errs2 := ParseProfile(again); len(errs2) != 0 {
			t.Fatalf("跳过条目后的档案再载入应零告警: %v", errs2)
		}
	})
	t.Run("无目录行的存量条目被跳过", func(t *testing.T) {
		rj := dirRowMappingRule("map-legacy-ws-nodir", &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}}})
		rj.Conditions = []CondxRow{rj.Conditions[0], rj.Conditions[2]}
		rj.MetadataFile.ContentBindings = []ContentBinding{{Row: 2, Dimension: DimDescription}}
		rj.WorkSetAggregation = &workSetAggregationField{Entries: []WorkSetAggregationProduction{{
			Grouping: &GroupingBinding{Mode: GroupingModeDirectory}, NameExtraction: &NameExtraction{Row: 1, Capture: "work"},
		}}}
		profile, errs := ParseProfile(rjProfileData(t, *rj))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 {
			t.Fatalf("无目录行的存量条目应被跳过、规则照常载入并恰给一条告警: rules=%d errs=%v", len(profile.Rules), errs)
		}
		msg := errs[0].Error()
		if !strings.Contains(msg, "作品集聚合第 1 条") || !strings.Contains(msg, "请补一条目录行，或改用配对分组") ||
			!strings.Contains(msg, "已跳过该条目") {
			t.Fatalf("跳过条目的告警应点名条目与处置: %s", msg)
		}
		if entries := profile.Rules[0].WorkSetAggregation; len(entries) != 0 {
			t.Fatalf("被跳过的条目不应载入: %+v", entries)
		}
		if again := wsProfileData(t, profile.Rules[0]); strings.Contains(string(again), "workSetAggregation") {
			t.Fatalf("全部条目被跳过即无该产出、不再落档: %s", again)
		}
	})
}

// setReachRule 三行底盘（JSON 中间形态）：行1 序列 `{作品标识}_{序号}`（图片成员）、
// 行2 文件 `{集名}_{序号}` + `.png`、行3 目录 `作者{作者}`——覆盖集名可得性判定的
// 三种分组依据（序列群键 / 手写拼接 / 所在目录）。
func setReachRule(entries ...WorkSetAggregationProduction) *ruleEntryJSON {
	return &ruleEntryJSON{
		ID:          "ws-reach",
		Description: "测试：集名可得性",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetSeq, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品标识}_{序号}"},
				Attach: []CondxAttach{{Kind: CondxAttachSeqMemberType, Types: []string{"image"}}}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{集名}_{序号}"},
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".png"}}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "作者{作者}"}}},
		},
		WorkSetAggregation: &workSetAggregationField{Entries: entries},
	}
}

// TestGroupingSetNameReachable 集名必须由所绑分组项保证可得（键可得则集名必可得）：
// 所在目录分组与单序列群键分组的集名行号须等于产生行，手写拼接分组的（行号·捕获
// 名）须是分组键的一个部分。满足者载入；不满足者——严格形态（编辑保存）拒绝并
// 点名集名现指位置与应取位置，载入形态跳过该条目并告警（规则其余部分照常载入）。
func TestGroupingSetNameReachable(t *testing.T) {
	seqKey := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}}}
	composite := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 2, Capture: "setname"}, {Row: 3, Capture: "author"},
	}}
	directory := &GroupingBinding{Mode: GroupingModeDirectory, Row: 3}
	cases := []struct {
		name     string
		grouping *GroupingBinding
		nameRow  int
		capture  string
		want     string // 空=满足（载入通过）
	}{
		{"目录分组集名在产生行", directory, 3, "author", ""},
		{"目录分组集名不在产生行", directory, 2, "setname", "的集名来源（第 2 行·setname）不在所绑目录分组的产生行上（产生行=第 3 行）"},
		{"序列群键分组集名在产生行", seqKey, 1, "work", ""},
		{"序列群键分组集名不在产生行", seqKey, 2, "setname", "不在所绑序列分组的产生行上（产生行=第 1 行）"},
		{"手写拼接集名是键的一部分", composite, 3, "author", ""},
		{"手写拼接集名不是键的部分", composite, 1, "work", "不是所绑分组键的一个部分（键部分：第 2 行·setname、第 3 行·author）"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := &NameExtraction{Row: tc.nameRow, Capture: tc.capture}
			rj := setReachRule(WorkSetAggregationProduction{Grouping: tc.grouping, NameExtraction: name})
			entry, err := ruleFromJSON(rj)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("集名由分组项保证可得的条目应通过严格形态: %v", err)
				}
				if len(entry.WorkSetAggregation) != 1 {
					t.Fatalf("条目应保真: %+v", entry.WorkSetAggregation)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("严格形态应拒并点名原因（含 %q）: %v", tc.want, err)
			}
			profile, errs := ParseProfile(rjProfileData(t, *setReachRule(WorkSetAggregationProduction{Grouping: tc.grouping, NameExtraction: name})))
			if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 {
				t.Fatalf("载入形态应跳过该条目（规则照常载入、恰一条告警）: rules=%d errs=%v", len(profile.Rules), errs)
			}
			if !strings.Contains(errs[0].Error(), tc.want) || !strings.Contains(errs[0].Error(), "已跳过该条目") {
				t.Fatalf("跳过告警应点名原因: %v", errs[0])
			}
			if entries := profile.Rules[0].WorkSetAggregation; len(entries) != 0 {
				t.Fatalf("被跳过的条目不应载入: %+v", entries)
			}
		})
	}
	// 保存闸门：集名不在产生行的条目编辑保存被拦且不落盘。
	saveJSON, err := json.Marshal(setReachRule(WorkSetAggregationProduction{
		Grouping: directory, NameExtraction: &NameExtraction{Row: 2, Capture: "setname"},
	}))
	if err != nil {
		t.Fatalf("序列化编辑规则失败: %v", err)
	}
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	if _, err := mgr.ApplySaveRule(saveJSON); err == nil || !strings.Contains(err.Error(), "不在所绑目录分组的产生行上") {
		t.Fatalf("集名不在产生行的条目保存应被拦并点名: %v", err)
	}
	if _, stored := store.values[RuleProfileKVKey]; stored {
		t.Fatal("保存被拦不应落盘档案")
	}
}

// dirAggRule 构造聚合规则（JSON 中间形态）：行1 文件 `{作品}` + `.mp4`、行2 目录
// `{集名}`（and）；分轨入口=行1 videoMain；分组依据由调用方经 identity / grouping 给出。
func dirAggRule(id string, identity []IdentityPart, grouping *GroupingBinding) *ruleEntryJSON {
	return &ruleEntryJSON{
		ID:          id,
		Description: "测试：聚合成作品的分组绑定",
		Source:      RuleSourceManual,
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"},
				Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".mp4"}}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{集名}"}}},
		},
		Identity: identity,
		Aggregate: &AggregateProduction{
			WorkType:     "video",
			Grouping:     grouping,
			Tracks:       []TrackAssign{{Row: 1, Role: "videoMain"}},
			CompanionUse: CompanionUseTrack,
		},
	}
}

// TestAggregateGroupingDirectoryBinding 「聚合成作品」的分组绑定（aggregate.grouping）：
// 绑所在目录分组（产生行=目录行）且无标识声明的规则两形态皆载入，grouping 随
// aggregate 落档与读回（自定义反序列化扩读）、往返字节等价；与标识声明同在场即拒
// （点名「两种分组绑定只能选一种」）；两者皆缺即拒（点名分组依据二选一）；产生行
// 缺行/非目录行/越界即拒（聚合绑定不经存量补行归一，载入形态同样拒绝）；轨行分配
// 照常按行号校验；标识声明形态的聚合不落 grouping 键。
func TestAggregateGroupingDirectoryBinding(t *testing.T) {
	directory := &GroupingBinding{Mode: GroupingModeDirectory, Row: 2}
	identity := []IdentityPart{{Row: 1, Capture: "work"}}

	entry, err := ruleFromJSON(dirAggRule("agg-dir", nil, directory))
	if err != nil {
		t.Fatalf("绑目录分组的聚合规则应通过严格形态: %v", err)
	}
	if len(entry.Identity) != 0 || entry.Aggregate.Grouping == nil || entry.Aggregate.Grouping.Row != 2 {
		t.Fatalf("目录分组绑定应载入聚合产出: identity=%+v aggregate=%+v", entry.Identity, entry.Aggregate)
	}
	data := wsProfileData(t, *entry)
	if !strings.Contains(string(data), `"aggregate":{"workType":"video","grouping":{"mode":"directory","row":2},"tracks":[{"row":1,"role":"videoMain"}],"companionUse":"track"}`) {
		t.Fatalf("grouping 应随 aggregate 落档: %s", data)
	}
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || len(profile.Rules) != 1 || profile.Rules[0].Aggregate.Grouping == nil || profile.Rules[0].Aggregate.Grouping.Row != 2 {
		t.Fatalf("grouping 应随 aggregate 读回（自定义反序列化扩读）: errs=%v", errs)
	}
	if again := wsProfileData(t, profile.Rules[0]); string(again) != string(data) {
		t.Fatalf("绑目录分组的聚合档案写读往返应字节等价\n首次: %s\n读回: %s", data, again)
	}

	reject := func(id string, identity []IdentityPart, grouping *GroupingBinding, tracks []TrackAssign, wants ...string) {
		t.Helper()
		build := func() *ruleEntryJSON {
			rj := dirAggRule(id, identity, grouping)
			if tracks != nil {
				rj.Aggregate.Tracks = tracks
			}
			return rj
		}
		_, err := ruleFromJSON(build())
		if err == nil {
			t.Fatalf("%s 应被严格形态拒绝", id)
		}
		for _, want := range wants {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%s 的拒绝文案应含 %q: %v", id, want, err)
			}
		}
		wsRejectOne(t, rjProfileData(t, *build()), wants...)
	}
	reject("agg-both", identity, directory, nil, "两种分组绑定只能选一种")
	reject("agg-none", nil, nil, nil, "缺少分组依据")
	reject("agg-dir-norow", nil, &GroupingBinding{Mode: GroupingModeDirectory}, nil, "缺少产生行")
	reject("agg-dir-filerow", nil, &GroupingBinding{Mode: GroupingModeDirectory, Row: 1}, nil, "不是目录条件行")
	reject("agg-dir-overflow", nil, &GroupingBinding{Mode: GroupingModeDirectory, Row: 3}, nil, "越界")
	reject("agg-dir-tracks", nil, directory, []TrackAssign{{Row: 9, Role: "videoMain"}}, "分轨入口第 1 项行号 9 越界")

	// 标识声明形态：不落 grouping 键（缺省分组=标识声明，落档零变化）。
	identityEntry, err := ruleFromJSON(dirAggRule("agg-identity", identity, nil))
	if err != nil {
		t.Fatalf("标识声明形态的聚合规则应载入: %v", err)
	}
	if data := wsProfileData(t, *identityEntry); strings.Contains(string(data), "grouping") {
		t.Fatalf("标识声明形态不应落 grouping 键: %s", data)
	}

	// 元数据文件的缺省分组保留：有聚合产出（绑目录分组）而未写分组绑定时照常
	// 载入、不在载入侧物化分组绑定。
	withDefault := dirAggRule("agg-default-fanout", nil, directory)
	withDefault.MetadataFile = &MetadataFileProduction{ContentBindings: []ContentBinding{{Row: 1, Dimension: DimDescription}}}
	defaultProfile, defaultErrs := ParseProfile(rjProfileData(t, *withDefault))
	if len(defaultErrs) != 0 || len(defaultProfile.Rules) != 1 || defaultProfile.Rules[0].MetadataFile.Grouping != nil {
		t.Fatalf("并用缺省分组应保留缺省（不物化）: errs=%v", defaultErrs)
	}
}

// TestGroupingSequenceBindingNotMigrated 序列群键分组绑定不迁移：identity + groupKey
// 的绑定就地按产生行（该群键部分的行号）生效，row 不落档、载入零告警、往返字节
// 等价；聚合规则的作品标识声明（identity）同样不迁移。
func TestGroupingSequenceBindingNotMigrated(t *testing.T) {
	seqKey := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}}}
	rj := setReachRule(WorkSetAggregationProduction{Grouping: seqKey, NameExtraction: &NameExtraction{Row: 1, Capture: "work"}})
	data := rjProfileData(t, *rj)
	if !strings.Contains(string(data), `"grouping":{"mode":"identity","identity":[{"row":1,"capture":"groupKey"}]}`) {
		t.Fatalf("序列群键绑定的落档形态: %s", data)
	}
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || len(profile.Rules) != 1 || profile.Rules[0].WorkSetAggregation[0].Grouping.Row != 0 {
		t.Fatalf("序列群键绑定应就地载入、不补 row: errs=%v", errs)
	}
	if again := wsProfileData(t, profile.Rules[0]); string(again) != string(data) {
		t.Fatalf("序列群键绑定档案写读往返应字节等价\n首次: %s\n读回: %s", data, again)
	}
	identityRule, err := ruleFromJSON(dirAggRule("agg-identity-keep", []IdentityPart{{Row: 1, Capture: "work"}}, nil))
	if err != nil {
		t.Fatalf("标识声明的聚合规则应载入: %v", err)
	}
	if len(identityRule.Identity) != 1 || identityRule.Aggregate.Grouping != nil {
		t.Fatalf("标识声明应原样保留、不迁入分组绑定: identity=%+v aggregate=%+v", identityRule.Identity, identityRule.Aggregate)
	}
}

// —— 预置种子「单图+说明文本」配对形态 ——

// presetImageCompanionSeed 取预置种子「单图+说明文本→图片作品元数据」（按稳定
// ID 取，不依赖种子表下标）。
func presetImageCompanionSeed(t *testing.T) RuleEntry {
	t.Helper()
	for _, seed := range PresetAggregateRules() {
		if seed.ID == presetAggImageCompanionTextID {
			return seed
		}
	}
	t.Fatalf("预置种子表应有 %q", presetAggImageCompanionTextID)
	return RuleEntry{}
}

// TestPresetImageCompanionPairingShape 配对形态落档锚：5 行=3 条图片行
// （{作品}+jpg/png/webp）+2 条说明行（带点行 {作品}.{任意} 在前、不带点行 {作品}
// 在后，皆 .txt），非首行显式带连接词；无聚合产出、无标识声明；「元数据文件」
// 产出绑标识分组（单部分、行 1、捕获 {作品}=work，不落产生行 row）、两条说明行
// 各一条内容绑定→描述。经载入校验零告警、写读往返字节等价。
func TestPresetImageCompanionPairingShape(t *testing.T) {
	seed := presetImageCompanionSeed(t)
	if seed.Category != RuleCategoryMetadata || seed.Aggregate != nil || len(seed.Identity) != 0 ||
		seed.Source != RuleSourcePreset || !seed.Enabled {
		t.Fatalf("种子应为启用的预置来源元数据规则（无聚合产出、无标识声明）: %+v", seed)
	}
	rows := seed.Conditions
	if len(rows) != 5 {
		t.Fatalf("配对形态应为 5 行（3 图片行+2 说明行）: %d", len(rows))
	}
	wantRows := []struct{ tpl, ext string }{
		{"{作品}", ".jpg"}, {"{作品}", ".png"}, {"{作品}", ".webp"},
		{"{作品}.{任意}", ".txt"}, {"{作品}", ".txt"},
	}
	for i, want := range wantRows {
		cond := rows[i].Condition
		if cond == nil || cond.Target != CondxTargetFile || cond.Format.Mode != CondxFormatTemplate ||
			cond.Format.Tpl != want.tpl || len(cond.Attach) != 1 ||
			cond.Attach[0].Kind != CondxAttachFileExt || cond.Attach[0].Value != want.ext {
			t.Fatalf("第 %d 行应为文件行 %q+%q: %+v", i+1, want.tpl, want.ext, rows[i])
		}
		if i > 0 && rows[i].Connector != "and" {
			t.Fatalf("非首行应显式带连接词（行式底盘落档语法）: 第 %d 行 %q", i+1, rows[i].Connector)
		}
	}
	if !strings.Contains(rows[3].Condition.Format.Tpl, ".") || strings.Contains(rows[4].Condition.Format.Tpl, ".") {
		t.Fatalf("带点说明行必须排在前面（不带点行在前会把 001.zh.txt 取成 001.zh）: %+v", rows[3:])
	}
	mf := seed.MetadataFile
	if mf == nil || mf.Grouping == nil || mf.Grouping.Mode != GroupingModeIdentity ||
		mf.Grouping.Row != 0 || len(mf.Grouping.Identity) != 1 ||
		mf.Grouping.Identity[0] != (IdentityPart{Row: 1, Capture: "work"}) {
		t.Fatalf("分组绑定应为标识分组（单部分、行 1、捕获 {作品}=work、不落产生行）: %+v", mf)
	}
	wantBindings := []ContentBinding{{Row: 4, Dimension: DimDescription}, {Row: 5, Dimension: DimDescription}}
	if !reflect.DeepEqual(mf.ContentBindings, wantBindings) {
		t.Fatalf("两条说明行应各有一条内容绑定→描述: %+v", mf.ContentBindings)
	}
	data := wsProfileData(t, seed)
	if !strings.Contains(string(data), `"grouping":{"mode":"identity","identity":[{"row":1,"capture":"work"}]}`) ||
		strings.Contains(string(data), `"mode":"directory"`) {
		t.Fatalf("分组绑定应以配对形态落档: %s", data)
	}
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 {
		t.Fatalf("配对形态预置经载入校验应零告警: errs=%v", errs)
	}
	if again := wsProfileData(t, profile.Rules[0]); string(again) != string(data) {
		t.Fatalf("配对形态预置写读往返应字节等价\n首次: %s\n读回: %s", data, again)
	}
}

// TestPresetImageCompanionPairingFanout 配对扇出锚（预置种子实跑）：001.jpg 与
// 001.txt（不带点行命中）、001.zh.txt（带点行命中）各按同名键 001 配对——说明文
// 件为元数据源（不导入：零归属键+说明成员标记），说明内容扇出到同名图片作品
// （作品级归集得到描述声明、来源=说明文件名）；图片维持独立作品、不参与扇出；
// 无同名图片的说明文件（说明.txt）按孤文件回退为独立作品，无同名说明的图片
// （002.jpg）照常独立成件。
func TestPresetImageCompanionPairingFanout(t *testing.T) {
	seed := presetImageCompanionSeed(t)
	files := aggScanFiles("p/001.jpg", "p/001.txt", "p/001.zh.txt", "p/002.jpg", "p/说明.txt")
	files[1].CompanionText = "简述：第一版说明"
	files[2].CompanionText = "简述：中文说明"
	rules := []RuleEntry{seed}
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex(), nil)

	// 说明文件：元数据源——零归属键、说明成员标记、扇出键集=同名图片作品键。
	for _, txt := range []string{"p/001.txt", "p/001.zh.txt"} {
		got := aggConclusionOf(t, conclusions, txt)
		if got.AggWorkKey != "" || got.AggTrackRole != "" || got.AggWorkType != "" || !got.AggCompanion ||
			!reflect.DeepEqual(fanoutKeysOf(got), []string{"f:p/001.jpg"}) {
			t.Fatalf("同名说明文件 %s 应为元数据源并扇出到同名图片作品（键=001 而非 001.zh）: key=%q role=%q type=%q companion=%v fanout=%v",
				txt, got.AggWorkKey, got.AggTrackRole, got.AggWorkType, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
	// 图片：维持独立作品，不参与内容扇出。
	for _, jpg := range []string{"p/001.jpg", "p/002.jpg"} {
		got := aggConclusionOf(t, conclusions, jpg)
		if got.AggWorkKey != "" || got.AggTrackRole != "" || got.AggCompanion || len(fanoutKeysOf(got)) != 0 {
			t.Fatalf("图片 %s 应维持独立作品、不参与内容扇出: key=%q role=%q companion=%v fanout=%v",
				jpg, got.AggWorkKey, got.AggTrackRole, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
	// 孤文件回退：无同名图片的说明文件按独立作品导入（不作元数据源）。
	orphan := aggConclusionOf(t, conclusions, "p/说明.txt")
	if orphan.AggWorkKey != "" || orphan.AggTrackRole != "" || orphan.AggCompanion || len(fanoutKeysOf(orphan)) != 0 {
		t.Fatalf("无同名图片的说明文件应按孤文件回退（独立作品）: key=%q role=%q companion=%v fanout=%v",
			orphan.AggWorkKey, orphan.AggTrackRole, orphan.AggCompanion, orphan.FanoutWorkKeys)
	}

	// 作品带描述：内容声明按扇出键集归集——图片作品键收两条描述声明（两份
	// 同名说明各一条，内容与来源=各自说明文件名）；元数据源不产生自身作品位。
	applyCompanionContentDeclarations(conclusions, files, rules, nil)
	assignIdx := newAssignmentDirIndex(nil, nil)
	snap := &RecognitionSnapshot{SchemaVersion: recognitionSnapshotSchemaVersion}
	for i := range conclusions {
		snap.Files = append(snap.Files, snapFileOf(&conclusions[i], nil, assignIdx))
	}
	contentDecls := workContentDeclsByWork(snap)
	decls := contentDecls["f:p/001.jpg"]
	if len(decls) != 2 {
		t.Fatalf("同名图片作品应经扇出面得到两条描述声明（001.txt 与 001.zh.txt 各一条）: %+v", decls)
	}
	wantTexts := map[string]string{"001.txt": "简述：第一版说明", "001.zh.txt": "简述：中文说明"}
	for _, d := range decls {
		want, ok := wantTexts[d.OriginName]
		if !ok || d.Dimension != string(DimDescription) || d.Value != want || d.Source != string(SourceContent) {
			t.Fatalf("图片作品的描述声明应为对应说明文件的整段内容: %+v", d)
		}
		delete(wantTexts, d.OriginName)
	}
	if len(wantTexts) != 0 {
		t.Fatalf("图片作品的描述声明应来自两份同名说明文件: %+v", decls)
	}
	if _, ok := contentDecls["f:p/001.txt"]; ok {
		t.Fatalf("元数据源文件不应产生自身作品位的内容归集: %+v", contentDecls["f:p/001.txt"])
	}
}

// TestPresetImageCompanionPairingWhenArticleSeedOff 收编边界反面锚：图文种子停用
// 时，同名图片+文本对不再被聚合收编，改由配对形态的「单图+说明文本」种子配
// 对——txt 为元数据源扇出到图片作品，图片维持独立图片作品（非 article 内嵌
// 图轨）。
func TestPresetImageCompanionPairingWhenArticleSeedOff(t *testing.T) {
	var rules []RuleEntry
	for _, seed := range PresetAggregateRules() {
		if seed.ID == presetAggArticleImageTextID {
			seed.Enabled = false
		}
		rules = append(rules, seed)
	}
	files := aggScanFiles("q/cover.jpg", "q/cover.txt")
	effective := EffectiveRules(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: rules})
	conclusions := MergedRecognition(files, effective, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, effective, nil, testAggTypeIndex(), nil)

	txt := aggConclusionOf(t, conclusions, "q/cover.txt")
	if txt.AggWorkKey != "" || txt.AggTrackRole != "" || !txt.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(txt), []string{"f:q/cover.jpg"}) {
		t.Fatalf("图文种子停用时同名 txt 应由配对形态种子收为元数据源并扇出到图片作品: key=%q role=%q companion=%v fanout=%v",
			txt.AggWorkKey, txt.AggTrackRole, txt.AggCompanion, txt.FanoutWorkKeys)
	}
	jpg := aggConclusionOf(t, conclusions, "q/cover.jpg")
	if jpg.AggWorkKey != "" || jpg.AggTrackRole != "" || jpg.AggCompanion || len(fanoutKeysOf(jpg)) != 0 {
		t.Fatalf("图文种子停用时同名图片应维持独立图片作品: key=%q role=%q companion=%v fanout=%v",
			jpg.AggWorkKey, jpg.AggTrackRole, jpg.AggCompanion, jpg.FanoutWorkKeys)
	}
}

// legacyImageCompanionEntry 构造「单图+说明文本」预置的旧形态落档（JSON 中间
// 形态）：3 条图片行 + 1 条说明行（{任意}+.txt），「元数据文件」产出绑未指定产
// 生行的所在目录分组、内容绑定 1 条→行 4。
func legacyImageCompanionEntry() *ruleEntryJSON {
	fileRow := func(connector, tpl, ext string) CondxRow {
		return CondxRow{Connector: connector, Condition: &Condx{
			Target: CondxTargetFile,
			Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
			Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ext}},
		}}
	}
	return &ruleEntryJSON{
		ID:          presetAggImageCompanionTextID,
		Description: presetAggImageCompanionTextDesc,
		Source:      RuleSourcePreset,
		Enabled:     true,
		Category:    RuleCategoryMetadata,
		Conditions: []CondxRow{
			fileRow("", "{作品}", ".jpg"),
			fileRow("and", "{作品}", ".png"),
			fileRow("and", "{作品}", ".webp"),
			fileRow("and", "{任意}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping:        &GroupingBinding{Mode: GroupingModeDirectory},
			ContentBindings: []ContentBinding{{Row: 4, Dimension: DimDescription}},
		},
	}
}

// storeWithLegacyImageCompanionPreset 写出「三条预置 ID 皆在场、图伴条目为旧形
// 态」的档案（模拟升级前首次初始化落盘、此后未动过的用户库），返回内存存储
// 与写出的档案字节。
func storeWithLegacyImageCompanionPreset(t *testing.T) (*fakeProfileStore, string) {
	t.Helper()
	entries := []ruleEntryJSON{*legacyImageCompanionEntry()}
	for _, seed := range PresetAggregateRules() {
		if seed.ID == presetAggImageCompanionTextID {
			continue
		}
		rj, err := ruleToJSON(seed)
		if err != nil {
			t.Fatalf("预置种子转落档形态失败: %v", err)
		}
		entries = append(entries, *rj)
	}
	data := string(rjProfileData(t, entries...))
	store := newFakeProfileStore()
	if err := store.SetValue(RuleProfileKVKey, data); err != nil {
		t.Fatalf("写入档案失败: %v", err)
	}
	return store, data
}

// TestPresetLegacyImageCompanionLoadHintAndRestore 旧形态预置条目的载入告警与
// 恢复链路锚：用户库里在场的旧形态「单图+说明文本」预置条目（所在目录分组、无
// 目录行）载入时跳过「元数据文件」产出、规则本身照常载入，告警点名规则并给出
// 处置指引「该预置规则是旧形态，已停用：删除它后点「恢复预置」即得新形态」；
// 同形态的用户自建条目不给该指引（没有出厂形态可恢复）；在场旧条目不被「恢复
// 预置」覆写（补 0 条、档案字节不动、再载入再现同一告警）；删除后恢复预置补
// 回配对新形态（与出厂种子落档等价），再载入零告警。
func TestPresetLegacyImageCompanionLoadHintAndRestore(t *testing.T) {
	const disposal = "该预置规则是旧形态，已停用：删除它后点「恢复预置」即得新形态"
	t.Run("在场旧条目跳过产出并给处置指引", func(t *testing.T) {
		profile, errs := ParseProfile(rjProfileData(t, *legacyImageCompanionEntry()))
		if profile == nil || len(profile.Rules) != 1 || len(errs) != 1 {
			t.Fatalf("旧形态预置条目应跳过产出、规则照常载入并恰给一条告警: rules=%d errs=%v", len(profile.Rules), errs)
		}
		msg := errs[0].Error()
		for _, want := range []string{`"` + presetAggImageCompanionTextID + `"`, "已跳过该产出", disposal} {
			if !strings.Contains(msg, want) {
				t.Fatalf("旧形态预置的载入告警应含 %q: %s", want, msg)
			}
		}
		if profile.Rules[0].MetadataFile != nil {
			t.Fatalf("被跳过的产出不应载入: %+v", profile.Rules[0].MetadataFile)
		}
	})
	t.Run("用户自建条目同形态不给处置指引", func(t *testing.T) {
		rj := legacyImageCompanionEntry()
		rj.ID, rj.Source, rj.Description = "my-image-notes", RuleSourceManual, "我的图片说明"
		_, errs := ParseProfile(rjProfileData(t, *rj))
		if len(errs) != 1 {
			t.Fatalf("用户自建条目的同形态应恰给一条告警: %v", errs)
		}
		if msg := errs[0].Error(); !strings.Contains(msg, "请补一条目录行，或改用配对分组") || strings.Contains(msg, disposal) {
			t.Fatalf("用户自建条目的告警应保持通用处置、不含预置处置指引: %s", msg)
		}
	})
	t.Run("在场旧条目不被恢复预置覆写", func(t *testing.T) {
		store, data := storeWithLegacyImageCompanionPreset(t)
		added, err := RestorePresetRules(store)
		if err != nil || added != 0 {
			t.Fatalf("预置 ID 皆在场（含旧形态）时恢复应补 0 条: added=%d err=%v", added, err)
		}
		if got, _ := store.GetValue(RuleProfileKVKey); got.Value != data {
			t.Fatalf("补 0 条时档案不应被改写\n原有: %s\n现有: %s", data, got.Value)
		}
		profile, errs := LoadProfile(store)
		if profile == nil || len(profile.Rules) != 3 || len(errs) != 1 || !strings.Contains(errs[0].Error(), disposal) {
			t.Fatalf("在场的旧形态条目应原样留存并再现处置指引告警: rules=%d errs=%v", len(profile.Rules), errs)
		}
		for _, rule := range profile.Rules {
			if rule.ID == presetAggImageCompanionTextID && (len(rule.Conditions) != 4 || rule.MetadataFile != nil) {
				t.Fatalf("在场的旧形态条目不应被覆写为新形态（仍为 4 行、产出被跳过）: %+v", rule)
			}
		}
	})
	t.Run("删除后恢复预置补回新形态", func(t *testing.T) {
		store, _ := storeWithLegacyImageCompanionPreset(t)
		profile, _ := LoadProfile(store)
		var kept []RuleEntry
		for _, rule := range profile.Rules {
			if rule.ID != presetAggImageCompanionTextID {
				kept = append(kept, rule)
			}
		}
		if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: kept}); err != nil {
			t.Fatalf("删除旧形态条目失败: %v", err)
		}
		added, err := RestorePresetRules(store)
		if err != nil || added != 1 {
			t.Fatalf("删除后恢复预置应补回 1 条: added=%d err=%v", added, err)
		}
		restored, errs := LoadProfile(store)
		if restored == nil || len(errs) != 0 || len(restored.Rules) != 3 {
			t.Fatalf("恢复后档案应回到三条种子且零告警: rules=%d errs=%v", len(restored.Rules), errs)
		}
		var seed *RuleEntry
		for i := range restored.Rules {
			if restored.Rules[i].ID == presetAggImageCompanionTextID {
				seed = &restored.Rules[i]
			}
		}
		if seed == nil || len(seed.Conditions) != 5 || seed.MetadataFile == nil ||
			seed.MetadataFile.Grouping == nil || seed.MetadataFile.Grouping.Mode != GroupingModeIdentity ||
			len(seed.MetadataFile.ContentBindings) != 2 {
			t.Fatalf("恢复预置应补回配对新形态（5 行+配对分组+两条内容绑定）: %+v", seed)
		}
		if got, want := wsProfileData(t, *seed), wsProfileData(t, presetImageCompanionSeed(t)); string(got) != string(want) {
			t.Fatalf("恢复回的条目应与出厂种子落档等价\n恢复: %s\n出厂: %s", got, want)
		}
	})
}
