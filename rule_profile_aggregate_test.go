// 聚合规则文件轨行「模板尾部误写扩展名」的载入校验锚：文件主体模板对去
// 扩展名的文件名求值（FileSubjectPattern 契约，rule_condition_subject.go），
// 尾字面 .ext（ext ∈ 白名单 knownFileExtsForTplTail）恒不命中——载入拒收并
// 指路 fileExt 谓词；白名单外点段（.part1）、正确拆分形态与 dir 目标行不受
// 影响。档案级断言锚「条目跳过 + 带规则 ID 告警」的既有坏条目呈现。
package main

import (
	"encoding/json"
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
		Pairing:         &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{{Row: 1, Capture: "work"}, {Row: 1, Capture: "seq"}}},
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
	// 聚合路径零变化：聚合条目同形态由聚合守卫拦截（「文件轨行」措辞）
	aggErr := validateAggregateEntry(aggTailExtEntry("agg-dead-mp4", CondxTargetFile, "{任意} - {任意}({标题}).mp4", nil, "title"))
	if aggErr == nil || !strings.Contains(aggErr.Error(), "文件轨行模板尾部误写扩展名 .mp4") {
		t.Fatalf("聚合守卫应维持原措辞拦截: %v", aggErr)
	}
}
