// 聚合规则文件轨行「模板尾部误写扩展名」的载入校验锚：文件主体模板对去
// 扩展名的文件名求值（FileSubjectPattern 契约，rule_condition_subject.go），
// 尾字面 .ext（ext ∈ 白名单 knownFileExtsForTplTail）恒不命中——载入拒收并
// 指路 fileExt 谓词；白名单外点段（.part1）、正确拆分形态与 dir 目标行不受
// 影响。档案级断言锚「条目跳过 + 带规则 ID 告警」的既有坏条目呈现。
//
// 另附「作品集聚合」产出四的阶段1 档案结构锚：多条目（各自分组绑定+集名来源）、
// 逐条校验与 1:1（含跨「聚合成作品」）、旧档单条对象载入归一（含目录行捕获绑
// 目录分组、无可用捕获告警），见文件末节。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// —— 作品集聚合（产出四）多条档案结构 + 旧档归一（阶段1） ——

// wsAggConditions 两条条件行底盘：行1 文件「{集名}_{序号}」（声明集名/序号捕获）、
// 行2 目录「作者{作者}」（声明作者捕获）——多条目、复合分组键与目录行集名共用。
func wsAggConditions() []CondxRow {
	return []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{集名}_{序号}"}}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "作者{作者}"}}},
	}
}

// wsMultiEntryRule 两条目的作品集聚合规则：条目1＝行1 集名捕获同时作分组键与
// 集名；条目2＝所在目录分组 + 行2 作者捕获作集名。
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
				Grouping:       &GroupingBinding{Mode: GroupingModeDirectory},
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
	if second.Grouping == nil || second.Grouping.Mode != GroupingModeDirectory ||
		len(second.Grouping.Identity) != 0 ||
		second.NameExtraction == nil || second.NameExtraction.Row != 2 || second.NameExtraction.Capture != "author" {
		t.Fatalf("条目2 的分组绑定/集名来源未随档案往返保真: %+v", second)
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

// TestWorkSetAggGroupingOneToOne 1:1 校验锚（校验③）：同一分组不得被两个聚合
// 条目选中（同规则两条目互斥），也不得与「聚合成作品」的分组绑定（标识声明
// identity）相同；分组不同的并用形态照常载入。
func TestWorkSetAggGroupingOneToOne(t *testing.T) {
	t.Run("两条目同一标识分组被拒", func(t *testing.T) {
		rule := wsMultiEntryRule()
		rule.WorkSetAggregation[1].Grouping = &GroupingBinding{
			Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "setname"}},
		}
		wsRejectOne(t, wsProfileData(t, rule), "第 1 条", "第 2 条", "同一分组", "1:1")
	})
	t.Run("两条目同为目录分组被拒", func(t *testing.T) {
		rule := wsMultiEntryRule()
		rule.WorkSetAggregation[0].Grouping = &GroupingBinding{Mode: GroupingModeDirectory}
		wsRejectOne(t, wsProfileData(t, rule), "同一分组", "1:1")
	})

	aggConditions := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}_{序号}"}}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "作者{作者}"}}},
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
}

// legacyWSRule 旧档单条形态的规则 JSON 载体：workSetAggregation 为单对象（新结构
// 序列化恒出数组，旧形态只能显式声明本载体构造）。
type legacyWSRule struct {
	ID          string                        `json:"id"`
	Description string                        `json:"description,omitempty"`
	Source      RuleSource                    `json:"source"`
	Enabled     bool                          `json:"enabled"`
	Category    RuleCategory                  `json:"category,omitempty"`
	Identity    []IdentityPart                `json:"identity,omitempty"`
	Aggregate   *AggregateProduction          `json:"aggregate,omitempty"`
	WorkSet     *WorkSetAggregationProduction `json:"workSetAggregation,omitempty"`
	Conditions  []CondxRow                    `json:"conditions,omitempty"`
	Extractions []ExtractionAction            `json:"extractions"`
}

// legacyWSProfileData 序列化旧档形态档案（单条 workSetAggregation 对象）。
func legacyWSProfileData(t *testing.T, rule legacyWSRule) []byte {
	t.Helper()
	data, err := json.Marshal(&struct {
		SchemaVersion int            `json:"schemaVersion"`
		Rules         []legacyWSRule `json:"rules"`
	}{CurrentSchemaVersion, []legacyWSRule{rule}})
	if err != nil {
		t.Fatalf("序列化旧档测试档案失败: %v", err)
	}
	return data
}

// TestWorkSetAggLegacySingleObjectNormalizes 旧档归一锚（甲案）：旧档单条对象
// （workSetAggregation:{grouping}）+ 条件行的作品集 collection 提取行，载入即把
// 提取行搬进首条目作 NameExtraction（行号+捕获名）——不拒收；捕获来自文件行时
// 分组绑定保留；提取行搬走后从条件行清掉（collection 维度已退役，不留第二条集
// 名声明通路）。保存形态归一为条目数组。
func TestWorkSetAggLegacySingleObjectNormalizes(t *testing.T) {
	data := legacyWSProfileData(t, legacyWSRule{
		ID:          "ws-legacy",
		Description: "测试：旧档单条归一",
		Source:      RuleSourceManual,
		Enabled:     true,
		WorkSet: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "setname"}},
		}},
		Conditions:  wsAggConditions(),
		Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1}},
	})
	// 前置断言：本档确为旧档单对象形态（不是新数组）。
	var probe struct {
		Rules []struct {
			WorkSet json.RawMessage `json:"workSetAggregation"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("旧档测试档解析失败: %v", err)
	}
	if len(probe.Rules) != 1 || !bytes.HasPrefix(bytes.TrimSpace(probe.Rules[0].WorkSet), []byte("{")) {
		t.Fatalf("旧档测试档应携带单对象形态的 workSetAggregation: %s", data)
	}

	profile, errs := ParseProfile(data)
	if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 {
		t.Fatalf("旧档旧形态应载入即归一、不拒收: errs=%v", errs)
	}
	rule := profile.Rules[0]
	if len(rule.WorkSetAggregation) != 1 {
		t.Fatalf("旧档归一后应恰一条目，实际 %d: %+v", len(rule.WorkSetAggregation), rule.WorkSetAggregation)
	}
	entry := rule.WorkSetAggregation[0]
	if entry.NameExtraction == nil || entry.NameExtraction.Row != 1 || entry.NameExtraction.Capture != "setname" {
		t.Fatalf("集名提取行应搬进条目作集名来源（行1 setname）: %+v", entry.NameExtraction)
	}
	if entry.Grouping == nil || entry.Grouping.Mode != GroupingModeIdentity ||
		len(entry.Grouping.Identity) != 1 || entry.Grouping.Identity[0].Capture != "setname" {
		t.Fatalf("捕获来自文件行时分组绑定应保留: %+v", entry.Grouping)
	}
	// 搬走而非复制：集名提取行归一后从条件行清掉（collection 维度已退役，集名
	// 只剩条目 NameExtraction 一个出口）。
	if len(rule.Extractions) != 0 {
		t.Fatalf("归一后旧档作品集提取行应从条件行清掉: %+v", rule.Extractions)
	}
	// 保存形态归一：再序列化落数组。
	again := wsProfileData(t, rule)
	if !strings.Contains(string(again), `"workSetAggregation":[{`) {
		t.Fatalf("归一后保存应落条目数组形态: %s", again)
	}
}

// TestWorkSetAggLegacyDirRowCaptureBindsDirectoryGrouping 旧档归一锚（目录行捕获）：
// 集名提取行的捕获原来自目录行时，该条目分组同时绑成「目录分组 · 所在目录」
// （同目录文件共享该集名），即便旧档分组绑定声明的是标识分组。
func TestWorkSetAggLegacyDirRowCaptureBindsDirectoryGrouping(t *testing.T) {
	data := legacyWSProfileData(t, legacyWSRule{
		ID:          "ws-legacy-dir",
		Description: "测试：旧档目录行集名归一",
		Source:      RuleSourceManual,
		Enabled:     true,
		WorkSet: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}},
		}},
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"}}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "作者{作者}"}}},
		},
		Extractions: []ExtractionAction{
			{Dimension: DimCollection, From: FromCapture, CaptureName: "author", Row: 2},
			// 非 collection 维度的提取动作不属于退役面：归一时应原样保留。
			{Dimension: DimAuthor, From: FromCapture, CaptureName: "work", Row: 1},
		},
	})
	profile, errs := ParseProfile(data)
	if len(errs) != 0 || profile == nil || len(profile.Rules) != 1 {
		t.Fatalf("目录行集名的旧档应载入即归一: errs=%v", errs)
	}
	entry := profile.Rules[0].WorkSetAggregation[0]
	if entry.NameExtraction == nil || entry.NameExtraction.Row != 2 || entry.NameExtraction.Capture != "author" {
		t.Fatalf("目录行捕获应搬进条目作集名来源（行2 author）: %+v", entry.NameExtraction)
	}
	if entry.Grouping == nil || entry.Grouping.Mode != GroupingModeDirectory || len(entry.Grouping.Identity) != 0 {
		t.Fatalf("目录行捕获的条目应绑「目录分组 · 所在目录」: %+v", entry.Grouping)
	}
	// 搬走而非复制：collection 提取行清掉，其余维度提取动作原样保留。
	extractions := profile.Rules[0].Extractions
	if len(extractions) != 1 || extractions[0].Dimension != DimAuthor || extractions[0].CaptureName != "work" {
		t.Fatalf("归一后应仅清掉 collection 提取行（作者行保留）: %+v", extractions)
	}
}

// TestWorkSetAggLegacyNoUsableCaptureWarns 旧档归一锚（无可用捕获）：集名提取行
// 来源不产捕获（此处常量）时保留分组绑定、集名置空——载入按无集名来源告警
// （条目跳过），编辑保存同拒且档案不落盘。
func TestWorkSetAggLegacyNoUsableCaptureWarns(t *testing.T) {
	data := legacyWSProfileData(t, legacyWSRule{
		ID:          "ws-legacy-const",
		Description: "测试：旧档常量集名无捕获",
		Source:      RuleSourceManual,
		Enabled:     true,
		WorkSet:     &WorkSetAggregationProduction{Grouping: &GroupingBinding{Mode: GroupingModeDirectory}},
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"}}},
		},
		Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromConstant, Constant: "集甲"}},
	})
	wsRejectOne(t, data, "第 1 条", "集名")

	// 保存侧：同形态的新档条目（条目数组、无集名来源）编辑保存被拦且不落盘。
	rj := &ruleEntryJSON{
		ID:          "ws-legacy-const",
		Description: "测试：旧档常量集名无捕获",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			{Condition: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{作品}"}}},
		},
		Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromConstant, Constant: "集甲"}},
		WorkSetAggregation: &workSetAggregationField{Entries: []WorkSetAggregationProduction{{
			Grouping: &GroupingBinding{Mode: GroupingModeDirectory},
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

// TestWorkSetAggFieldAbsenceCompat 向后兼容锚：无作品集聚合产出的规则不落
// workSetAggregation 键、往返字节等价；旧档 null 与空数组两形态按无条目读入、
// 不报错（字段类型改列表不得让旧档解析失败）。
func TestWorkSetAggFieldAbsenceCompat(t *testing.T) {
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
