// 规则组合引用的档案模型与载入校验（规则组合引用方案阶段1）：RuleEntry.refs /
// ExtractionAction.refRule 字段落档与 JSON 往返（旧档零迁移）、From=refCapture
// 来源，以及载入校验五则（refs 悬空 / 自引用 / 嵌套组合引用 / refCapture 的
// refRule∈refs / captureName 必填）各自的正反用例——违规条目一律走既有坏条目
// 出口（跳过该条 + 带规则 ID 的告警），不阻塞其余规则载入。
package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// refsTestRule 构造一条可序列化的成员/组合规则：模式为固定文件名正则（内容
// 与引用校验无关），refs 与提取动作按用例注入。
func refsTestRule(t *testing.T, id string, refs []string, actions []ExtractionAction) RuleEntry {
	t.Helper()
	return RuleEntry{
		ID:          id,
		Description: "测试规则 " + id,
		Source:      RuleSourceManual,
		Enabled:     true,
		Refs:        refs,
		Pattern:     mustRegex(t, string(TargetFileName), "^"+id+`\.jpg$`, nil),
		Extractions: actions,
	}
}

// refsTestProfileJSON 把规则列表序列化为 v4 档案 JSON（与真档案同款多态序列化）。
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

// TestRefsFieldsRoundTrip refs / refRule / from=refCapture 落档与 JSON 往返：
// 组合规则经 MarshalJSON→ParseProfile→MarshalJSON 字节等价，引用字段在场。
func TestRefsFieldsRoundTrip(t *testing.T) {
	member := refsTestRule(t, "member-a", nil, nil)
	combo := refsTestRule(t, "combo-1", []string{"member-a"}, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1},
	})
	data1 := refsTestProfileJSON(t, member, combo)
	text := string(data1)
	for _, want := range []string{`"refs":["member-a"]`, `"refRule":"member-a"`, `"from":"refCapture"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("序列化结果应含 %s，实际: %s", want, text)
		}
	}

	profile, errs := ParseProfile(data1)
	if len(errs) != 0 {
		t.Fatalf("合规组合档案不应报错，实际: %v", errs)
	}
	assertRefsLoadedIDs(t, profile, "member-a", "combo-1")
	if !reflect.DeepEqual(profile.Rules[1].Refs, []string{"member-a"}) {
		t.Fatalf("refs 往返不符: %+v", profile.Rules[1].Refs)
	}
	if got := profile.Rules[1].Extractions[0].RefRule; got != "member-a" {
		t.Fatalf("refRule 往返不符: %q", got)
	}

	data2, err := json.Marshal(*profile)
	if err != nil {
		t.Fatalf("二次序列化失败: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatalf("引用档案往返字节不等价:\n首次: %s\n二次: %s", data1, data2)
	}
}

// TestRefsLegacyNoRefsRoundTrip 无 refs 的旧档案：序列化不落 refs/refRule 字段
// （omitempty）、往返字节不变；手写旧档（仅 pattern 形态）同样零迁移可载。
func TestRefsLegacyNoRefsRoundTrip(t *testing.T) {
	legacy := RuleProfile{
		SchemaVersion: CurrentSchemaVersion,
		Rules: []RuleEntry{
			{ID: "legacy-rule", Description: "旧档普通规则", Source: RuleSourceManual, Enabled: true,
				Pattern:     mustRegex(t, string(TargetFileName), `^legacy\.jpg$`, nil),
				Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromParentDir}}},
		},
	}
	data1, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("序列化旧档失败: %v", err)
	}
	if strings.Contains(string(data1), `"refs"`) || strings.Contains(string(data1), `"refRule"`) {
		t.Fatalf("无引用旧档不应落 refs/refRule 字段（omitempty 保证零迁移）: %s", data1)
	}
	parsed, errs := ParseProfile(data1)
	if len(errs) != 0 {
		t.Fatalf("旧档应零迁移可载，实际: %v", errs)
	}
	if parsed.Rules[0].Refs != nil {
		t.Fatalf("旧档规则 Refs 应为空，实际: %+v", parsed.Rules[0].Refs)
	}
	data2, err := json.Marshal(*parsed)
	if err != nil {
		t.Fatalf("二次序列化失败: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatalf("旧档往返字节不等价:\n首次: %s\n二次: %s", data1, data2)
	}

	// 手写旧档（缺 refs/refRule 键，pattern 形态）：载入不报错、Refs 为空。
	raw := `{"schemaVersion":4,"rules":[{"id":"legacy-pattern","description":"手写旧档",` +
		`"source":"manual","enabled":true,"pattern":{"kind":"regex","target":"filename",` +
		`"expr":"^legacy$","captures":[]},"extractions":[{"dimension":"title","from":"parentDir"}],` +
		`"synthesis":{}}]}`
	parsedRaw, errs := ParseProfile([]byte(raw))
	if len(errs) != 0 {
		t.Fatalf("手写旧档应零迁移可载，实际: %v", errs)
	}
	assertRefsLoadedIDs(t, parsedRaw, "legacy-pattern")
	if parsedRaw.Rules[0].Refs != nil {
		t.Fatalf("手写旧档规则 Refs 应为空，实际: %+v", parsedRaw.Rules[0].Refs)
	}
}

// TestRefsEmptyConditionsCombo 组合规则空条件表（剧本一·帧6「引用 N 条成员
// 规则；（无条件行）」——前端编辑面放行的合法形态）：refs 非空时 conditions
// 空数组可载可存（模式=恒真合取，命中前提全部由成员门段承担）；非组合规则
// 空条件表仍拒（无条件行的普通规则=恒命中，违背 v4 语义）。live-test 实机
// 抓出后端未放行的缺陷（2026-10-26 VERIFY-规则组合引用 A 项），本用例锚定。
func TestRefsEmptyConditionsCombo(t *testing.T) {
	const comboJSON = `{"schemaVersion":4,"rules":[` +
		`{"id":"member-a","description":"成员","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^a\\.jpg$","captures":[]},` +
		`"extractions":[],"synthesis":{}},` +
		`{"id":"combo-1","description":"空条件组合","source":"manual","enabled":true,` +
		`"refs":["member-a"],"conditions":[],` +
		`"extractions":[{"dimension":"author","from":"refCapture","refRule":"member-a",` +
		`"captureName":"author"}],"synthesis":{}}]}`

	profile, errs := ParseProfile([]byte(comboJSON))
	if len(errs) != 0 {
		t.Fatalf("空条件组合档案应可载，实际: %v", errs)
	}
	assertRefsLoadedIDs(t, profile, "member-a", "combo-1")
	combo := profile.Rules[1]
	if combo.Pattern == nil {
		t.Fatalf("空条件组合的模式应为恒真合取（非 nil），实际 nil")
	}
	in := &MatchInput{}
	if !combo.Pattern.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("空条件组合的恒真模式应对任意输入命中")
	}

	// 非组合规则空条件表：拒载该条 + 告警。
	const plainEmptyJSON = `{"schemaVersion":4,"rules":[` +
		`{"id":"plain-bad","description":"普通空条件","source":"manual","enabled":true,` +
		`"conditions":[],"extractions":[],"synthesis":{}}]}`
	profile2, errs2 := ParseProfile([]byte(plainEmptyJSON))
	if len(errs2) != 1 || !strings.Contains(refsErrText(errs2), "条件行列表不能为空") {
		t.Fatalf("非组合空条件表应拒载并告警，实际: %v", errs2)
	}
	if len(profile2.Rules) != 0 {
		t.Fatalf("拒载后不应有余留规则，实际: %d", len(profile2.Rules))
	}

	// 落档形态往返：空条件表经 omitempty 序列化为无 conditions 键（refs 在场
	// 即组合），再载入须等价（可存可载——live-test 抓出的「存后即丢」缺陷锚）。
	data, err := json.Marshal(*profile)
	if err != nil {
		t.Fatalf("空条件组合档案序列化失败: %v", err)
	}
	if !strings.Contains(string(data), `"refs":["member-a"],"extractions":`) {
		t.Fatalf("空条件组合落档应为无 conditions 键形态，实际: %s", data)
	}
	reparsed, errs3 := ParseProfile(data)
	if len(errs3) != 0 {
		t.Fatalf("落档形态（无 conditions 键）应可载，实际: %v", errs3)
	}
	assertRefsLoadedIDs(t, reparsed, "member-a", "combo-1")
	if reparsed.Rules[1].Pattern == nil {
		t.Fatalf("落档形态载入后模式应为恒真合取（非 nil）")
	}
	data2, err := json.Marshal(*reparsed)
	if err != nil {
		t.Fatalf("二次序列化失败: %v", err)
	}
	if !bytes.Equal(data, data2) {
		t.Fatalf("空条件组合档案往返字节不等价:\n首次: %s\n二次: %s", data, data2)
	}
}

// TestRefsDanglingReference 校验 a（refs 悬空）：引用不在本档案规则集内的成员
// → 跳过该条 + 带规则 ID 的告警；引用存在的普通成员 → 照常载入。
func TestRefsDanglingReference(t *testing.T) {
	member := refsTestRule(t, "member-a", nil, nil)

	t.Run("悬空拒载", func(t *testing.T) {
		dangling := refsTestRule(t, "combo-dangling", []string{"member-x"}, nil)
		profile, errs := ParseProfile(refsTestProfileJSON(t, member, dangling))
		if len(errs) != 1 {
			t.Fatalf("悬空引用应返回单条告警，实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"combo-dangling", "member-x", "不存在"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		// 剧本二·帧5 告警文案锚：点名组合规则描述与悬空成员 ID。
		if wantMsg := "组合规则『测试规则 combo-dangling』引用的成员规则 member-x 不存在，已跳过"; !strings.Contains(msg, wantMsg) {
			t.Fatalf("告警应为帧5 文案 %q，实际: %s", wantMsg, msg)
		}
		assertRefsLoadedIDs(t, profile, "member-a")
	})

	t.Run("成员在场可载", func(t *testing.T) {
		ok := refsTestRule(t, "combo-ok", []string{"member-a"}, nil)
		profile, errs := ParseProfile(refsTestProfileJSON(t, member, ok))
		if len(errs) != 0 {
			t.Fatalf("引用在场成员不应报错，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "member-a", "combo-ok")
	})

	t.Run("被构造拒收的成员按悬空处理", func(t *testing.T) {
		// 成员缺裁决常量 → 第一遍构造失败拒收；对它的引用按悬空处理。
		brokenMember := refsTestRule(t, "member-broken", nil, []ExtractionAction{
			{Dimension: DimAuthor, From: FromAdjudication},
		})
		refToBroken := refsTestRule(t, "combo-ref-broken", []string{"member-broken"}, nil)
		profile, errs := ParseProfile(refsTestProfileJSON(t, brokenMember, refToBroken))
		if len(errs) != 2 {
			t.Fatalf("应为两条告警（成员构造失败 + 引用悬空），实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"member-broken", "combo-ref-broken", "不存在"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile)
	})
}

// TestRefsSelfReference 校验 b（自引用）：refs 含本条自身 ID → 跳过 + 告警；
// 引用其它规则为合法组合。
func TestRefsSelfReference(t *testing.T) {
	self := refsTestRule(t, "combo-self", []string{"combo-self"}, nil)
	profile, errs := ParseProfile(refsTestProfileJSON(t, self))
	if len(errs) != 1 {
		t.Fatalf("自引用应返回单条告警，实际 %d 条: %v", len(errs), errs)
	}
	msg := refsErrText(errs)
	for _, want := range []string{"combo-self", "自身"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("告警应含 %q，实际: %s", want, msg)
		}
	}
	assertRefsLoadedIDs(t, profile)

	ok := refsTestRule(t, "combo-ok", []string{"member-a"}, nil)
	profile, errs = ParseProfile(refsTestProfileJSON(t, refsTestRule(t, "member-a", nil, nil), ok))
	if len(errs) != 0 {
		t.Fatalf("引用其它规则不应报错，实际: %v", errs)
	}
	assertRefsLoadedIDs(t, profile, "member-a", "combo-ok")
}

// TestRefsNestedCombination 校验 c（嵌套组合引用，一期禁止）：refs 指向自身
// 携带 refs 的组合规则 → 跳过该条 + 告警；被引组合自身合法时照常载入。
func TestRefsNestedCombination(t *testing.T) {
	member := refsTestRule(t, "member-a", nil, nil)
	inner := refsTestRule(t, "combo-inner", []string{"member-a"}, nil)
	outer := refsTestRule(t, "combo-outer", []string{"combo-inner"}, nil)
	profile, errs := ParseProfile(refsTestProfileJSON(t, member, inner, outer))
	if len(errs) != 1 {
		t.Fatalf("嵌套组合引用应返回单条告警，实际 %d 条: %v", len(errs), errs)
	}
	msg := refsErrText(errs)
	for _, want := range []string{"combo-outer", "combo-inner", "嵌套"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("告警应含 %q，实际: %s", want, msg)
		}
	}
	assertRefsLoadedIDs(t, profile, "member-a", "combo-inner")

	flat := refsTestRule(t, "combo-flat", []string{"member-a"}, nil)
	profile, errs = ParseProfile(refsTestProfileJSON(t, member, flat))
	if len(errs) != 0 {
		t.Fatalf("引用普通成员不应报错，实际: %v", errs)
	}
	assertRefsLoadedIDs(t, profile, "member-a", "combo-flat")
}

// TestRefCaptureRefRuleConstraint 校验 d（refCapture 的 refRule∈本条 refs）：
// refRule 为空或不在 refs 内 → 跳过 + 告警；refRule 属 refs → 载入并保留字段。
func TestRefCaptureRefRuleConstraint(t *testing.T) {
	memberA := refsTestRule(t, "member-a", nil, nil)
	memberB := refsTestRule(t, "member-b", nil, nil)

	t.Run("refRule为空拒载", func(t *testing.T) {
		empty := refsTestRule(t, "combo-empty-ref", []string{"member-a"}, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, CaptureName: "author"},
		})
		profile, errs := ParseProfile(refsTestProfileJSON(t, memberA, empty))
		if len(errs) != 1 {
			t.Fatalf("缺 refRule 应返回单条告警，实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"combo-empty-ref", "refRule"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "member-a")
	})

	t.Run("refRule不在refs内拒载", func(t *testing.T) {
		outside := refsTestRule(t, "combo-outside-ref", []string{"member-a"}, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-b", CaptureName: "author"},
		})
		profile, errs := ParseProfile(refsTestProfileJSON(t, memberA, memberB, outside))
		if len(errs) != 1 {
			t.Fatalf("refRule 不在 refs 内应返回单条告警，实际 %d 条: %v", len(errs), errs)
		}
		msg := refsErrText(errs)
		for _, want := range []string{"combo-outside-ref", "member-b", "refs 列表"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("告警应含 %q，实际: %s", want, msg)
			}
		}
		assertRefsLoadedIDs(t, profile, "member-a", "member-b")
	})

	t.Run("refRule属refs可载", func(t *testing.T) {
		good := refsTestRule(t, "combo-good-ref", []string{"member-a"}, []ExtractionAction{
			{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author", Row: 1},
		})
		profile, errs := ParseProfile(refsTestProfileJSON(t, memberA, good))
		if len(errs) != 0 {
			t.Fatalf("refRule 属 refs 不应报错，实际: %v", errs)
		}
		assertRefsLoadedIDs(t, profile, "member-a", "combo-good-ref")
		if got := profile.Rules[1].Extractions[0].RefRule; got != "member-a" {
			t.Fatalf("refRule 应保留，实际: %q", got)
		}
	})
}

// TestRefCaptureCaptureNameRequired 校验 e（refCapture 的 captureName 必填，
// 对齐 FromCapture 口径）：缺 captureName → 跳过 + 告警；有 captureName → 载入。
func TestRefCaptureCaptureNameRequired(t *testing.T) {
	member := refsTestRule(t, "member-a", nil, nil)
	missing := refsTestRule(t, "combo-no-name", []string{"member-a"}, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a"},
	})
	profile, errs := ParseProfile(refsTestProfileJSON(t, member, missing))
	if len(errs) != 1 {
		t.Fatalf("缺 captureName 应返回单条告警，实际 %d 条: %v", len(errs), errs)
	}
	msg := refsErrText(errs)
	for _, want := range []string{"combo-no-name", "captureName"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("告警应含 %q，实际: %s", want, msg)
		}
	}
	assertRefsLoadedIDs(t, profile, "member-a")

	good := refsTestRule(t, "combo-named", []string{"member-a"}, []ExtractionAction{
		{Dimension: DimAuthor, From: FromRefCapture, RefRule: "member-a", CaptureName: "author"},
	})
	profile, errs = ParseProfile(refsTestProfileJSON(t, member, good))
	if len(errs) != 0 {
		t.Fatalf("captureName 在场不应报错，实际: %v", errs)
	}
	assertRefsLoadedIDs(t, profile, "member-a", "combo-named")
}

// TestLoadProfileSurfacesRefViolations 引用违规挂既有坏条目出口：LoadProfile
// 返回可用档案（仅跳过违规条目）与带规则 ID 的错误，其他规则不受阻塞。
func TestLoadProfileSurfacesRefViolations(t *testing.T) {
	member := refsTestRule(t, "member-a", nil, nil)
	dangling := refsTestRule(t, "combo-dangling", []string{"member-x"}, nil)
	store := &fakeProfileStore{values: map[string]string{
		RuleProfileKVKey: string(refsTestProfileJSON(t, member, dangling)),
	}}
	loaded, errs := LoadProfile(store)
	if loaded == nil {
		t.Fatalf("单条引用违规不应导致整体载入失败")
	}
	assertRefsLoadedIDs(t, loaded, "member-a")
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "combo-dangling") {
		t.Fatalf("应返回带规则 ID 的单条告警，实际: %v", errs)
	}
}
