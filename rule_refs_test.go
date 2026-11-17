// 规则引用（refs）退役后的档案面锚：v4 存量档案双读与载入处置（refs 非空
// 条目跳过+告警、保存归一 v5）、「位于第 N 级」词条（dirLevel）携带条目按
// 未知词条先例载入即拒、普通档案序列化零 refs/refRule 字段，以及重复段词
// 三面引用禁区（when 面 + 作用域接线面：refCapture 动作与 when 限定名两路）
// ——组合规则（refs/空条件门）词汇消亡后的等价承载与防御性处置。
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// refsTestRule 构造一条可序列化的普通规则：模式为固定文件名正则（内容与
// 载入处置无关），提取动作按用例注入。
func refsTestRule(t *testing.T, id string, actions []ExtractionAction) RuleEntry {
	t.Helper()
	return RuleEntry{
		ID:          id,
		Description: "测试规则 " + id,
		Source:      RuleSourceManual,
		Enabled:     true,
		Pattern:     mustRegex(t, string(TargetFileName), "^"+id+`\.jpg$`, nil),
		Extractions: actions,
	}
}

// refsTestProfileJSON 把规则列表序列化为当前版本档案 JSON（与真档案同款多态
// 序列化）。
func refsTestProfileJSON(t *testing.T, rules ...RuleEntry) []byte {
	t.Helper()
	data, err := json.Marshal(RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: rules})
	if err != nil {
		t.Fatalf("构造测试档案失败: %v", err)
	}
	return data
}

// refsErrText 把错误列表拼为单个文本，便于断言规则 ID 与原因关键词。
func refsErrText(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		parts = append(parts, err.Error())
	}
	return strings.Join(parts, "\n")
}

// assertRefsLoadedIDs 断言载入后的规则 ID 序列（档案序）。
func assertRefsLoadedIDs(t *testing.T, profile *RuleProfile, want ...string) {
	t.Helper()
	if profile == nil {
		t.Fatalf("档案不应为 nil，期望规则 %v", want)
	}
	got := make([]string, 0, len(profile.Rules))
	for _, rule := range profile.Rules {
		got = append(got, rule.ID)
	}
	if len(got) != len(want) {
		t.Fatalf("载入规则条数不符: 得到 %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("载入规则序不符: 得到 %v，期望 %v", got, want)
		}
	}
}

// TestLegacyV4RefsEntriesSkippedOnLoad v4 存量档案读入（升级用户自身的读入
// 迁移）：refs 非空条目（组合规则旧词汇）跳过+告警，其余规则照常载入；
// 无 refs 的 v4 条目零迁移可载。
func TestLegacyV4RefsEntriesSkippedOnLoad(t *testing.T) {
	const v4WithRefs = `{"schemaVersion":4,"rules":[` +
		`{"id":"plain-a","description":"普通规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^a\\.jpg$","captures":[]},` +
		`"extractions":[],"synthesis":{}},` +
		`{"id":"combo-1","description":"组合规则","source":"manual","enabled":true,` +
		`"refs":["plain-a"],"conditions":[],` +
		`"extractions":[{"dimension":"author","from":"refCapture","refRule":"plain-a",` +
		`"captureName":"author"}],"synthesis":{}}]}`
	profile, errs := ParseProfile([]byte(v4WithRefs))
	if profile == nil {
		t.Fatalf("v4 档案应可载入: %v", errs)
	}
	assertRefsLoadedIDs(t, profile, "plain-a")
	if len(errs) != 1 {
		t.Fatalf("refs 条目应恰一条告警，实际 %d 条: %v", len(errs), errs)
	}
	msg := refsErrText(errs)
	for _, want := range []string{"combo-1", "refs", "跳过"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("告警应含 %q，实际: %s", want, msg)
		}
	}
}

// TestV4ArchiveLoadNormalizeV5 v4 档案读入照常、保存归一 v5：载入后版本为
// 当前版本（5），保存落盘为 v5，再载入零告警。
func TestV4ArchiveLoadNormalizeV5(t *testing.T) {
	const v4Plain = `{"schemaVersion":4,"rules":[` +
		`{"id":"legacy-pattern","description":"手写旧档","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^legacy$","captures":[]},` +
		`"extractions":[{"dimension":"title","from":"parentDir"}],"synthesis":{}}]}`
	profile, errs := ParseProfile([]byte(v4Plain))
	if len(errs) != 0 {
		t.Fatalf("v4 档案应零告警载入，实际: %v", errs)
	}
	if profile.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("载入后版本应归一为 %d，实际 %d", CurrentSchemaVersion, profile.SchemaVersion)
	}
	store := &fakeProfileStore{values: map[string]string{RuleProfileKVKey: v4Plain}}
	if _, errs := LoadProfile(store); len(errs) != 0 {
		t.Fatalf("v4 档案经 LoadProfile 应零告警: %v", errs)
	}
	loaded, _ := LoadProfile(store)
	if err := SaveProfile(store, loaded); err != nil {
		t.Fatalf("保存归一失败: %v", err)
	}
	saved, _ := store.GetValue(RuleProfileKVKey)
	if !strings.Contains(saved.Value, `"schemaVersion":5`) {
		t.Fatalf("保存后档案应为 v5: %s", saved.Value)
	}
	again, errs := ParseProfile([]byte(saved.Value))
	if len(errs) != 0 || len(again.Rules) != 1 || again.Rules[0].ID != "legacy-pattern" {
		t.Fatalf("v5 归一档案应零告警载入: %v %+v", errs, again)
	}
}

// TestDirLevelAttachEntrySkippedOnLoad 携带「位于第 N 级」词条（dirLevel，
// 已废弃）的条目按未知词条先例载入即拒（跳过+告警）；同档其余规则照常载入。
func TestDirLevelAttachEntrySkippedOnLoad(t *testing.T) {
	const withDirLevel = `{"schemaVersion":4,"rules":[` +
		`{"id":"plain-a","description":"普通规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^a\\.jpg$","captures":[]},` +
		`"extractions":[],"synthesis":{}},` +
		`{"id":"anchored","description":"带层级锚","source":"manual","enabled":true,` +
		`"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"Artist[{作者}]"},` +
		`"attach":[{"kind":"dirLevel","minCount":1}]}}],` +
		`"extractions":[{"dimension":"author","from":"capture","captureName":"author","row":1}],"synthesis":{}}]}`
	profile, errs := ParseProfile([]byte(withDirLevel))
	if profile == nil {
		t.Fatalf("档案应可载入: %v", errs)
	}
	assertRefsLoadedIDs(t, profile, "plain-a")
	if len(errs) != 1 {
		t.Fatalf("dirLevel 条目应恰一条告警，实际 %d 条: %v", len(errs), errs)
	}
	if msg := refsErrText(errs); !strings.Contains(msg, "anchored") {
		t.Fatalf("告警应点名条目 ID，实际: %s", msg)
	}
}

// TestPlainProfileSerializesNoRefs 普通档案序列化不落 refs 键（词汇消亡后
// ruleEntryJSON 无该字段）、refRule 仅在 refCapture 动作上落档（作用域附件
// 消费规则）；往返字节等价。
func TestPlainProfileSerializesNoRefs(t *testing.T) {
	rule := refsTestRule(t, "plain-1", []ExtractionAction{
		{Dimension: DimTitle, From: FromParentDir},
	})
	data1 := refsTestProfileJSON(t, rule)
	if strings.Contains(string(data1), `"refs"`) {
		t.Fatalf("普通档案不应落 refs 键: %s", data1)
	}
	parsed, errs := ParseProfile(data1)
	if len(errs) != 0 {
		t.Fatalf("普通档案应零告警载入，实际: %v", errs)
	}
	data2, err := json.Marshal(*parsed)
	if err != nil {
		t.Fatalf("二次序列化失败: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatalf("普通档案往返字节不等价:\n首次: %s\n二次: %s", data1, data2)
	}
}

// refsRepeatMember 构造带重复段词条件行的规则：模板 {标题}_{标签+;_}，模式由
// CompileRuleConditions 编译（rowPrefixed v4 形态；重复段词声明 r1.tag 供
// 引用禁区用例消费）。
func refsRepeatMember(t *testing.T, id string) RuleEntry {
	t.Helper()
	rows := []CondxRow{{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}_{标签+;_}")}}}
	pat, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("重复段词条件行应可编译: %v", err)
	}
	return RuleEntry{
		ID: id, Description: "测试规则 " + id, Source: RuleSourceManual, Enabled: true,
		Conditions: rows, Pattern: pat,
	}
}

// refsRepeatErrWants 断言重复段占位词引用禁区的统一文案关键词（三面同形：
// 生效条件 / 值校验 / 作用域接线）。
func refsRepeatErrWants(t *testing.T, errs []error) string {
	t.Helper()
	if len(errs) != 1 {
		t.Fatalf("应返回单条告警，实际 %d 条: %v", len(errs), errs)
	}
	msg := refsErrText(errs)
	for _, want := range []string{"重复段占位词", "多值产出"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("告警应含 %q，实际: %s", want, msg)
		}
	}
	return msg
}

// refsProviderConsumerJSON 构造供给方（纯目录行）+ 消费方（目录行挂 refScope
// 附件引用供给方）的档案 JSON——组合接线面的作用域版承载。
func refsProviderConsumerJSON(t *testing.T, provider RuleEntry, consumer ruleEntryJSON) []byte {
	t.Helper()
	pj, err := ruleToJSON(provider)
	if err != nil {
		t.Fatalf("序列化供给方失败: %v", err)
	}
	return scopeProfileJSON(t, *pj, consumer)
}

// TestRefsRepeatWordReferenceForbidden 三面引用禁区的 when 面与作用域接线面：
// 生效条件引用自有重复段词、作用域接线引用供给方重复段词（refCapture 动作与
// when 限定名两路）一律拒载并带统一文案；提取段与普通词引用照常放行。
func TestRefsRepeatWordReferenceForbidden(t *testing.T) {
	t.Run("when引用自有重复段词拒载", func(t *testing.T) {
		rule := refsRepeatMember(t, "repeat-when")
		rule.Extractions = []ExtractionAction{{
			Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1,
			When: &CondExpr{Kind: ExprEqual,
				Left:  &CondExpr{Kind: ExprCapture, CaptureName: "r1.tag"},
				Right: &CondExpr{Kind: ExprString, Str: "x"}},
		}}
		profile, errs := ParseProfile(refsTestProfileJSON(t, rule))
		msg := refsRepeatErrWants(t, errs)
		if !strings.Contains(msg, "repeat-when") {
			t.Fatalf("告警应点名规则 ID，实际: %s", msg)
		}
		if profile == nil || len(profile.Rules) != 0 {
			t.Fatalf("违规规则应被跳过，实际载入 %d 条", len(profile.Rules))
		}
	})

	t.Run("作用域接线refCapture引用供给方重复段词拒载", func(t *testing.T) {
		repeatProvider := refDirRowRule(t, "provider-repeat", "Tags({标签+})", nil, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("provider-repeat", 1)})},
			[]ExtractionAction{{Dimension: DimTag, From: FromRefCapture, RefRule: "provider-repeat", CaptureName: "tag", Row: 1}})
		profile, errs := ParseProfile(refsProviderConsumerJSON(t, repeatProvider, consumer))
		msg := refsRepeatErrWants(t, errs)
		if !strings.Contains(msg, "consumer") || !strings.Contains(msg, "provider-repeat") {
			t.Fatalf("告警应点名消费规则与供给方，实际: %s", msg)
		}
		assertRefsLoadedIDs(t, profile, "provider-repeat")
	})

	t.Run("作用域接线when限定名引用供给方重复段词拒载", func(t *testing.T) {
		repeatProvider := refDirRowRule(t, "provider-repeat", "Tags({标签+})", nil, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("provider-repeat", 1)})},
			[]ExtractionAction{{
				Dimension: DimTitle, From: FromParentDir,
				When: &CondExpr{Kind: ExprEqual,
					Left:  &CondExpr{Kind: ExprCapture, CaptureName: "provider-repeat.r1.tag"},
					Right: &CondExpr{Kind: ExprString, Str: "x"}},
			}})
		profile, errs := ParseProfile(refsProviderConsumerJSON(t, repeatProvider, consumer))
		msg := refsRepeatErrWants(t, errs)
		if !strings.Contains(msg, "consumer") || !strings.Contains(msg, "provider-repeat") {
			t.Fatalf("告警应点名消费规则与供给方，实际: %s", msg)
		}
		assertRefsLoadedIDs(t, profile, "provider-repeat")
	})

	t.Run("普通词三面引用照常放行", func(t *testing.T) {
		// 供给方与消费规则均为普通占位词：refCapture 与 when 限定名引用照常载入。
		provider := refDirRowRule(t, "provider-plain", "Artist[{作者}]", nil, nil)
		consumer := scopeRuleJSON("consumer-plain", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("provider-plain", 1)})},
			[]ExtractionAction{
				{Dimension: DimAuthor, From: FromRefCapture, RefRule: "provider-plain", CaptureName: "author", Row: 1},
				{Dimension: DimTitle, From: FromParentDir,
					When: &CondExpr{Kind: ExprEqual,
						Left:  &CondExpr{Kind: ExprCapture, CaptureName: "provider-plain.r1.author"},
						Right: &CondExpr{Kind: ExprString, Str: "x"}}},
			})
		profile, errs := ParseProfile(refsProviderConsumerJSON(t, provider, consumer))
		if len(errs) != 0 {
			t.Fatalf("普通词引用不应报错，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "provider-plain", "consumer-plain")
	})
}

// multiOccurRepeatMember 构造位次分立的重复段成员规则：模板 {标签}_[{标签+}]
//（位1 单值 tag、位2 重复段 tag__2），供三面禁引按位锚定用例消费。
func multiOccurRepeatMember(t *testing.T, id string) RuleEntry {
	t.Helper()
	rows := []CondxRow{{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标签}_[{标签+}]")}}}
	pat, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("多位重复段条件行应可编译: %v", err)
	}
	return RuleEntry{
		ID: id, Description: "测试规则 " + id, Source: RuleSourceManual, Enabled: true,
		Conditions: rows, Pattern: pat,
	}
}

// TestMultiOccurRepeatThreeFaceForbidden 位为重复段时的三面禁引按位锚定（模板
// 同占位词多次出现表达方案·阶段2 锚④）：位2 为重复段（组名 tag__2）时——生效
// 条件引用自有位2、作用域接线 refCapture 引用供给方位2、作用域接线 when 限定名
// 引用供给方位2 三路一律拒载，统一文案对位次名 r1.tag__2 经词根剥离按「标签」渲染
//（condxRepeatEnOfCapture 扩展）；位2 为单值词时三面照常放行（按位判定）；
// 求值面防御语义——位2 重复段 0+ 缺席＝零绑定，引用取不到值＝条件不成立、
// 提取零产出，不报错。
func TestMultiOccurRepeatThreeFaceForbidden(t *testing.T) {
	t.Run("when引用自有位2重复段拒载（词根文案）", func(t *testing.T) {
		rule := multiOccurRepeatMember(t, "mo-when")
		rule.Extractions = []ExtractionAction{{
			Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1,
			When: refEqExpr("r1.tag__2", "x"),
		}}
		profile, errs := ParseProfile(refsTestProfileJSON(t, rule))
		msg := refsRepeatErrWants(t, errs)
		for _, want := range []string{"mo-when", "「标签」"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q（位次名按词根渲染），实际: %s", want, msg)
			}
		}
		if profile == nil || len(profile.Rules) != 0 {
			t.Fatalf("违规规则应被跳过，实际载入 %d 条", len(profile.Rules))
		}
	})

	t.Run("作用域接线refCapture引用供给方位2重复段拒载", func(t *testing.T) {
		provider := refDirRowRule(t, "mo-provider", "{标签}_[{标签+}]", nil, nil)
		consumer := scopeRuleJSON("mo-consumer-ref", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("mo-provider", 1)})},
			[]ExtractionAction{
				{Dimension: DimTag, From: FromRefCapture, RefRule: "mo-provider", CaptureName: "tag__2", Row: 1},
			})
		profile, errs := ParseProfile(refsProviderConsumerJSON(t, provider, consumer))
		msg := refsRepeatErrWants(t, errs)
		for _, want := range []string{"mo-consumer-ref", "mo-provider", "r1.tag__2", "「标签」"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "mo-provider")
	})

	t.Run("作用域接线when限定名引用供给方位2重复段拒载", func(t *testing.T) {
		provider := refDirRowRule(t, "mo-provider", "{标签}_[{标签+}]", nil, nil)
		consumer := scopeRuleJSON("mo-consumer-when", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("mo-provider", 1)})},
			[]ExtractionAction{{
				Dimension: DimTitle, From: FromConstant, Constant: "x",
				When: refEqExpr("mo-provider.r1.tag__2", "x"),
			}})
		profile, errs := ParseProfile(refsProviderConsumerJSON(t, provider, consumer))
		msg := refsRepeatErrWants(t, errs)
		for _, want := range []string{"mo-consumer-when", "mo-provider", "r1.tag__2", "「标签」"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "mo-provider")
	})

	t.Run("位2单值词三面照常放行（按位判定）", func(t *testing.T) {
		rows := []CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("{标签}_{标签}")}}}
		pat, err := CompileRuleConditions(rows)
		if err != nil {
			t.Fatalf("双位单值条件行应可编译: %v", err)
		}
		provider := RuleEntry{
			ID: "mo-provider-plain", Description: "测试规则 mo-provider-plain", Source: RuleSourceManual, Enabled: true,
			Conditions: rows, Pattern: pat,
		}
		consumer := scopeRuleJSON("mo-consumer-plain", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("mo-provider-plain", 1)})},
			[]ExtractionAction{
				{Dimension: DimTag, From: FromRefCapture, RefRule: "mo-provider-plain", CaptureName: "tag__2", Row: 1},
				{Dimension: DimTitle, From: FromConstant, Constant: "x",
					When: refEqExpr("mo-provider-plain.r1.tag__2", "乙")},
			})
		profile, errs := ParseProfile(refsProviderConsumerJSON(t, provider, consumer))
		if len(errs) != 0 {
			t.Fatalf("位2 单值词三面引用应照常放行，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "mo-provider-plain", "mo-consumer-plain")
	})

	t.Run("求值面位2重复段0+缺席取不到值不成立", func(t *testing.T) {
		// 三面构造期已拒（上路三例）；此处锚求值层防御语义：位2 重复段 0+
		// 缺席＝零绑定，引用取不到值＝条件不成立（when 面）/零产出（提取
		// 面），不报错。
		rows := []CondxRow{{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标签}_[{标签*}]")}}}
		pat, err := CompileRuleConditions(rows)
		if err != nil {
			t.Fatalf("0+ 多位条件行应可编译: %v", err)
		}
		ctx := &ParseContext{}
		if !pat.Evaluate(&MatchInput{FileName: "art_.jpg"}, nil, ctx) {
			t.Fatalf("0+ 位2 缺席应仍命中")
		}
		if bindings := ctx.CaptureAll("r1.tag__2"); len(bindings) != 0 {
			t.Fatalf("0+ 位2 缺席应零绑定: %+v", bindings)
		}
		if refEqExpr("r1.tag__2", "art").Holds(ctx, nil) {
			t.Fatalf("零绑定位引用应取不到值、条件不成立")
		}
		act := ExtractionAction{Dimension: DimTag, From: FromCapture, CaptureName: "tag__2", Row: 1}
		if got := act.ApplyDeclarations("mo-absent", ctx); len(got) != 0 {
			t.Fatalf("零绑定位提取应零产出: %+v", got)
		}
	})
}
