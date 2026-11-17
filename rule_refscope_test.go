// 作用域附件（refScope·阶段2）的构造期校验与门段行扫描锚：校验五则迁移
//（自引用/悬空/链式/聚合互斥/重复段接线）+ 供给方纯目录合格性（裁决③）+
// 聚合轨行禁附件（自决4）+ 门段 needed 收集的行扫描口径（启用过滤、括号
// 子组递归）+ 提取段与目录含义回放的双通道一致性（裁决⑦）。
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// —— 校验测试构造件 ——

// scopeRuleJSON 构造一条条件行规则条目（JSON 中间形态）：rows 直接给定。
func scopeRuleJSON(id string, rows []CondxRow, actions []ExtractionAction) ruleEntryJSON {
	return ruleEntryJSON{
		ID: id, Description: "测试规则 " + id, Source: RuleSourceManual, Enabled: true,
		Conditions: rows, Extractions: actions,
	}
}

// scopeDirRow 构造一条目录行（可挂附件）。
func scopeDirRow(tpl string, attach []CondxAttach) CondxRow {
	cond := &Condx{Target: CondxTargetDir, Format: condxTpl(tpl)}
	if len(attach) > 0 {
		cond.Attach = attach
	}
	return CondxRow{Condition: cond}
}

// scopeProfileJSON 序列化档案（v4）。
func scopeProfileJSON(t *testing.T, rules ...ruleEntryJSON) []byte {
	t.Helper()
	data, err := json.Marshal(&ruleProfileJSON{SchemaVersion: CurrentSchemaVersion, Rules: rules})
	if err != nil {
		t.Fatalf("序列化档案失败: %v", err)
	}
	return data
}

// scopeErrText 汇总载入告警文本。
func scopeErrText(errs []error) string {
	var sb strings.Builder
	for _, err := range errs {
		sb.WriteString(err.Error())
		sb.WriteString("\n")
	}
	return sb.String()
}

// scopeLoadedIDs 已载入规则 ID 集。
func scopeLoadedIDs(profile *RuleProfile) map[string]bool {
	ids := map[string]bool{}
	if profile == nil {
		return ids
	}
	for i := range profile.Rules {
		ids[profile.Rules[i].ID] = true
	}
	return ids
}

// —— 校验五则与供给方合格性（保存期/载入期同一构造管线）——

// TestRefScopeValidationMatrix 作用域附件的跨规则校验矩阵：自引用/悬空/链式/
// 聚合互斥/重复段接线五则与供给方纯目录合格性——违规条目跳过+告警，合法
// 形态（含括号子组递归目录行、带提取动作的供给方）照常载入。
func TestRefScopeValidationMatrix(t *testing.T) {
	artistRow := scopeDirRow("Artist[{作者}]", nil)

	t.Run("自引用拒绝", func(t *testing.T) {
		self := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("consumer", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, self))
		if got := scopeLoadedIDs(profile)["consumer"]; got {
			t.Fatalf("自引用条目应跳过")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "引用了自身") {
			t.Fatalf("自引用应恰一条告警点名自身: %v", errs)
		}
	})
	t.Run("悬空拒绝", func(t *testing.T) {
		provider := scopeRuleJSON("provider", []CondxRow{artistRow}, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("ghost", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, provider, consumer))
		if scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("悬空引用条目应跳过")
		}
		if !scopeLoadedIDs(profile)["provider"] {
			t.Fatalf("无关条目不应被阻塞")
		}
		msg := scopeErrText(errs)
		if len(errs) != 1 || !strings.Contains(msg, "ghost") || !strings.Contains(msg, "不存在") {
			t.Fatalf("悬空应点名供给方不存在: %v", errs)
		}
	})
	t.Run("链式拒绝", func(t *testing.T) {
		// provider 自身携带 refScope（消费 chainRoot）→ 作为供给方被链式禁止。
		chainRoot := scopeRuleJSON("chain-root", []CondxRow{artistRow}, nil)
		middle := scopeRuleJSON("middle", []CondxRow{scopeDirRow("Mid", []CondxAttach{refScopeAttach("chain-root", 1)})}, nil)
		tail := scopeRuleJSON("tail", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("middle", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, chainRoot, middle, tail))
		ids := scopeLoadedIDs(profile)
		if ids["tail"] {
			t.Fatalf("链式引用条目应跳过（供给方自身携带作用域附件）")
		}
		if !ids["chain-root"] || !ids["middle"] {
			t.Fatalf("合法两环不应被阻塞: %v", ids)
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "链式") {
			t.Fatalf("链式应恰一条告警: %v", errs)
		}
	})
	t.Run("聚合供给方拒绝", func(t *testing.T) {
		agg := aggTailExtEntry("agg-provider", CondxTargetFile, "{标题}_{序号}", nil, "title")
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("agg-provider", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, *agg, consumer))
		if scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("聚合供给方条目应跳过")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "聚合规则") {
			t.Fatalf("聚合供给方应恰一条告警: %v", errs)
		}
	})
	t.Run("聚合轨行禁附件", func(t *testing.T) {
		// 聚合轨行携带 refScope → 构造期即拒（自决4：聚合段不跑门段通道）。
		agg := aggTailExtEntry("agg-scope", CondxTargetDir, "{标题}",
			[]CondxAttach{refScopeAttach("some-provider", 1)}, "title")
		profile, errs := ParseProfile(scopeProfileJSON(t, *agg))
		if scopeLoadedIDs(profile)["agg-scope"] {
			t.Fatalf("聚合轨行携带作用域附件应跳过")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "作用域附件") {
			t.Fatalf("聚合轨行附件应恰一条告警: %v", errs)
		}
	})
	t.Run("供给方非纯目录拒绝", func(t *testing.T) {
		// 供给方混入文件行（目录行+文件行组合）→ 非纯目录。
		mixed := scopeRuleJSON("mixed", []CondxRow{
			artistRow,
			{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}")}},
		}, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("mixed", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, mixed, consumer))
		if scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("非纯目录供给方条目应跳过")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "纯目录") {
			t.Fatalf("非纯目录供给方应恰一条告警: %v", errs)
		}
	})
	t.Run("供给方定群形态拒绝", func(t *testing.T) {
		// 全序列行供给方（定群）天然非纯目录。
		grouping := scopeRuleJSON("grouping", []CondxRow{{Condition: &Condx{
			Target: CondxTargetSeq, Format: condxTpl("{键}_{序号}"),
		}}}, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("grouping", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, grouping, consumer))
		if scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("定群供给方条目应跳过")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "纯目录") {
			t.Fatalf("定群供给方应恰一条告警: %v", errs)
		}
	})
	t.Run("供给方括号子组递归目录行放行", func(t *testing.T) {
		// 括号子组内全目录行（或组合）仍算纯目录（自决2：含括号子组递归）。
		grouped := scopeRuleJSON("grouped", []CondxRow{
			artistRow,
			{Connector: "or", Group: []CondxRow{
				scopeDirRow("Circle({作者})", nil),
				{Connector: "or", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("作者({作者})")}},
			}},
		}, []ExtractionAction{{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1}})
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("grouped", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, grouped, consumer))
		if len(errs) != 0 {
			t.Fatalf("括号子组递归目录行供给方不应报错: %v", errs)
		}
		ids := scopeLoadedIDs(profile)
		if !ids["grouped"] || !ids["consumer"] {
			t.Fatalf("两条均应载入: %v", ids)
		}
	})
	t.Run("供给方子组混文件行拒绝", func(t *testing.T) {
		grouped := scopeRuleJSON("grouped-bad", []CondxRow{
			{Group: []CondxRow{
				artistRow,
				{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}")}},
			}},
		}, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("grouped-bad", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, grouped, consumer))
		if scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("子组混文件行的供给方条目应跳过")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "纯目录") {
			t.Fatalf("子组混文件行应恰一条告警: %v", errs)
		}
	})
	t.Run("重复段接线拒绝·refCapture通道", func(t *testing.T) {
		// 供给方目录行带量词占位词 {标签+}（重复段词，多值产出）；消费规则
		// refCapture 提取引用该重复段词 → 第二遍拒绝。
		repeatProvider := scopeRuleJSON("repeat-provider", []CondxRow{
			scopeDirRow("Tags({标签+})", nil),
		}, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("repeat-provider", 1)})},
			[]ExtractionAction{{Dimension: DimTag, From: FromRefCapture, RefRule: "repeat-provider", CaptureName: "tag", Row: 1}})
		profile, errs := ParseProfile(scopeProfileJSON(t, repeatProvider, consumer))
		if scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("重复段接线条目应跳过")
		}
		if !scopeLoadedIDs(profile)["repeat-provider"] {
			t.Fatalf("供给方自身应照常载入")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "repeat-provider") {
			t.Fatalf("重复段接线应恰一条告警点名供给方: %v", errs)
		}
	})
	t.Run("重复段接线拒绝·when限定名通道", func(t *testing.T) {
		repeatProvider := scopeRuleJSON("repeat-provider", []CondxRow{
			scopeDirRow("Tags({标签+})", nil),
		}, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("repeat-provider", 1)})},
			[]ExtractionAction{{Dimension: DimTag, From: FromConstant, Constant: "x", When: &CondExpr{
				Kind: ExprNotEqual,
				Left: &CondExpr{Kind: ExprCapture, CaptureName: "repeat-provider.r1.tag"},
				Right: &CondExpr{Kind: ExprString, Str: "y"},
			}}})
		profile, errs := ParseProfile(scopeProfileJSON(t, repeatProvider, consumer))
		if scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("when 限定名引用重复段词的条目应跳过")
		}
		if len(errs) != 1 {
			t.Fatalf("应恰一条告警，实际 %d 条: %v", len(errs), errs)
		}
	})
	t.Run("行级非目录行仍算目录行", func(t *testing.T) {
		// 自决2：行级非的目录行仍算目录行（否定语义在求值面自然处理）。
		negProvider := scopeRuleJSON("neg-provider", []CondxRow{
			{Negated: true, Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Trash")}},
			{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}]")}},
		}, nil)
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("neg-provider", 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, negProvider, consumer))
		if len(errs) != 0 {
			t.Fatalf("行级非目录行供给方不应报错: %v", errs)
		}
		ids := scopeLoadedIDs(profile)
		if !ids["neg-provider"] || !ids["consumer"] {
			t.Fatalf("两条均应载入: %v", ids)
		}
	})
	t.Run("字段校验·供给方ID缺省拒绝", func(t *testing.T) {
		bad := scopeRuleJSON("bad-attach", []CondxRow{scopeDirRow("{集名}", []CondxAttach{{Kind: CondxAttachRefScope}})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, bad))
		if scopeLoadedIDs(profile)["bad-attach"] {
			t.Fatalf("供给方 ID 缺省的附件应构造拒绝")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "供给方规则 ID 不能为空") {
			t.Fatalf("字段校验应恰一条告警: %v", errs)
		}
	})
	t.Run("字段校验·负层距拒绝", func(t *testing.T) {
		bad := scopeRuleJSON("bad-gap", []CondxRow{scopeDirRow("{集名}", []CondxAttach{{Kind: CondxAttachRefScope, Value: "p", MinCount: -1}})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, bad))
		if scopeLoadedIDs(profile)["bad-gap"] {
			t.Fatalf("负层距附件应构造拒绝")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "层距无效") {
			t.Fatalf("层距校验应恰一条告警: %v", errs)
		}
	})
	t.Run("词条目标限定·非目录行拒绝", func(t *testing.T) {
		bad := scopeRuleJSON("bad-target", []CondxRow{{Condition: &Condx{
			Target: CondxTargetFile, Format: condxTpl("{标题}"),
			Attach: []CondxAttach{refScopeAttach("p", 1)},
		}}}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, bad))
		if scopeLoadedIDs(profile)["bad-target"] {
			t.Fatalf("文件行携带作用域附件应构造拒绝（词条目录行专属）")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "refScope") {
			t.Fatalf("目标限定应恰一条告警: %v", errs)
		}
	})
}

// —— 门段 needed 收集的行扫描口径 ——

// TestRefScopeGateNeededRowScan 门段 needed 收集=全部启用规则条件行递归扫描
// refScope 附件：括号子组内附件照扫；消费规则停用/聚合类别不参与收集；
// 无附件档案空门零工作（hits/dirHitDepths 全空）。
func TestRefScopeGateNeededRowScan(t *testing.T) {
	provider := refDirRowRule(t, "provider", "Artist[{作者}]", nil, nil)
	in := &MatchInput{FileName: "x.jpg", DirNames: []string{"Artist[甲]", "SetA"}, RelPath: "Artist[甲]/SetA/x.jpg"}

	t.Run("括号子组递归扫描", func(t *testing.T) {
		consumer := refRowsRule(t, "consumer", []CondxRow{
			{Group: []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach("provider", 1)})}},
		}, nil)
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		gate := runRefGateStage([]RuleEntry{provider, consumer}, in, nil, ctx)
		if !gate.hits["provider"] {
			t.Fatalf("括号子组内的附件应被行扫描收集并求值供给方")
		}
		if !gate.dirHitDepths["provider"][0] {
			t.Fatalf("供给方命中层集应含第 0 层: %+v", gate.dirHitDepths)
		}
	})
	t.Run("消费规则停用不收集", func(t *testing.T) {
		consumer := refDirRowRule(t, "consumer", "{集名}", []CondxAttach{refScopeAttach("provider", 1)}, nil)
		consumer.Enabled = false
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		gate := runRefGateStage([]RuleEntry{provider, consumer}, in, nil, ctx)
		if gate.hits["provider"] {
			t.Fatalf("停用消费规则的附件不应收集供给方")
		}
		if len(gate.dirHitDepths) != 0 {
			t.Fatalf("空门不应有层集登记: %+v", gate.dirHitDepths)
		}
	})
	t.Run("无附件档案空门", func(t *testing.T) {
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		gate := runRefGateStage([]RuleEntry{provider}, in, nil, ctx)
		if len(gate.hits) != 0 || len(gate.snapshots) != 0 || len(gate.dirHitDepths) != 0 {
			t.Fatalf("无附件档案应零工作空门: %+v", gate)
		}
	})
}

// —— 提取段与目录含义回放的双通道一致性（裁决⑦）——

// TestRefScopeDirMeaningConsistency 同一供给方/消费规则对在提取段
//（runRefEval）与目录含义回放（dirMeaningReplay）两通道口径一致：供给方
// 命中链上消费规则覆盖目标层、无供给方祖先链两通道同判零覆盖——回放走同
// 一门段入口（needed 行扫描）与同一层距阶梯判据。
func TestRefScopeDirMeaningConsistency(t *testing.T) {
	provider := refDirRowRule(t, "provider", "Artist[{作者}]", nil, nil)
	consumer := refDirRowRule(t, "consumer", "{集名}", []CondxAttach{refScopeAttach("provider", 1)},
		[]ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1}})
	rules := []RuleEntry{provider, consumer}

	// 供给方命中链：提取段消费规则命中；回放作品集含义=目标层目录名。
	chain := []string{"Artist[foo]", "SetA"}
	in := &MatchInput{FileName: "x.jpg", DirNames: chain, RelPath: "Artist[foo]/SetA/x.jpg", Siblings: []string{"x.jpg"}}
	decision := runRefEval(t, rules, in, nil)
	assertRefMatchedIDs(t, decision.MatchedRuleIDs, "provider", "consumer")
	if v, ok := declOf(decision, "consumer", DimCollection); !ok || v != "SetA" {
		t.Fatalf("提取段集名应为 SetA: %q %v", v, ok)
	}
	meanings, firstID := dirMeaningReplay(rules, chain, nil)
	if !meaningContains(meanings, string(PathTypeWorkSet), "SetA") {
		t.Fatalf("回放通道应产出作品集含义 SetA: %+v", meanings)
	}
	if firstID != "consumer" {
		t.Fatalf("回放首个覆盖规则应为消费规则: %q", firstID)
	}

	// 无供给方祖先链：提取段消费规则零产出；回放零含义零覆盖。
	bare := []string{"OtherDir"}
	in2 := &MatchInput{FileName: "x.jpg", DirNames: bare, RelPath: "OtherDir/x.jpg", Siblings: []string{"x.jpg"}}
	decision2 := runRefEval(t, rules, in2, nil)
	assertRefMatchedIDs(t, decision2.MatchedRuleIDs)
	meanings2, firstID2 := dirMeaningReplay(rules, bare, nil)
	for _, m := range meanings2 {
		if m.Type == string(PathTypeWorkSet) {
			t.Fatalf("回放通道不应产出作品集含义: %+v", meanings2)
		}
	}
	if firstID2 != "" {
		t.Fatalf("无覆盖链首个覆盖规则应为空: %q", firstID2)
	}
}
