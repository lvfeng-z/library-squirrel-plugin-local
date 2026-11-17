// 库根 sentinel（RefScopeLibraryRoot·作用域附件来源=库根）锚定测试：门段
// {-1} 层集常量注入（无供给方求值）、层距阶梯三档的绝对层级正反例（紧邻=
// 根直下第 1 级、隔 N=自根数起第 N 级、任意=恒真）、深层同名不误命中、
// 构造期校验放行（无悬空/自引用/链式/纯目录形态可判）、聚合轨行禁附件不变、
// 序列化往返（sentinel 值与层距无损）、摘要句式（库根/规则两形态）。
package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// rootEvalInput 构造库根层距测试输入（dirNames 自根直下逐层给出，文件名占位）。
func rootEvalInput(dirNames ...string) *MatchInput {
	return &MatchInput{
		FileName: "x.jpg", DirNames: dirNames,
		RelPath: strings.Join(dirNames, "/") + "/x.jpg", Siblings: []string{"x.jpg"},
	}
}

// rootScopedConsumer 构造库根来源的消费规则（目录行 Set{集名} 挂库根附件，
// 层距 gap），带集名提取动作——命中面即附件层距判据的观测面。
func rootScopedConsumer(t *testing.T, gap int) RuleEntry {
	t.Helper()
	return refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach(RefScopeLibraryRoot, gap)},
		[]ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1}})
}

// TestRefScopeLibraryRootGateInject 门段对库根 sentinel 的处理：命中层集恒按
// 常量 {-1} 注入、无供给方求值（hits/snapshots 全空——sentinel 不进供给方
// 查找路径，不在场≠未命中层集缺席）。
func TestRefScopeLibraryRootGateInject(t *testing.T) {
	consumer := refDirRowRule(t, "consumer", "Set{集名}", []CondxAttach{refScopeAttach(RefScopeLibraryRoot, 1)}, nil)
	in := rootEvalInput("SetA")
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	gate := runRefGateStage([]RuleEntry{consumer}, in, nil, ctx)
	if len(gate.hits) != 0 || len(gate.snapshots) != 0 {
		t.Fatalf("库根 sentinel 无供给方求值（hits/snapshots 应空）: %+v", gate)
	}
	if !gate.dirHitDepths[RefScopeLibraryRoot][-1] || len(gate.dirHitDepths[RefScopeLibraryRoot]) != 1 {
		t.Fatalf("库根 sentinel 命中层集应恰为 {-1}: %+v", gate.dirHitDepths)
	}
}

// TestRefScopeLibraryRootLadder 库根层距三档=绝对层级语义（复用层距阶梯算术、
// 库根命中层 −1）：紧邻(1)只中根直下第 1 级（c=0）、隔 N(N≥2) 只中自根数起
// 第 N 级（c=N−1）、任意(0)恒真；深层同名目录不因同名而误中浅层档。
func TestRefScopeLibraryRootLadder(t *testing.T) {
	consumerHit := func(t *testing.T, gap int, dirNames ...string) (*FileDecision, bool) {
		t.Helper()
		consumer := rootScopedConsumer(t, gap)
		decision := runRefEval(t, []RuleEntry{consumer}, rootEvalInput(dirNames...), nil)
		return decision, refIDsContain(decision.MatchedRuleIDs, "consumer")
	}

	t.Run("紧邻只中根直下", func(t *testing.T) {
		if _, hit := consumerHit(t, 1, "SetA"); !hit {
			t.Fatalf("紧邻应中根直下第 1 级（SetA@0）")
		}
		if _, hit := consumerHit(t, 1, "Shell", "SetA"); hit {
			t.Fatalf("紧邻不应中第 2 级（SetA@1）")
		}
		if _, hit := consumerHit(t, 1, "A", "B", "SetA"); hit {
			t.Fatalf("紧邻不应中第 3 级（SetA@2）")
		}
	})
	t.Run("隔N只中第N级", func(t *testing.T) {
		if _, hit := consumerHit(t, 2, "Shell", "SetA"); !hit {
			t.Fatalf("隔 2 应中自根数起第 2 级（SetA@1）")
		}
		if _, hit := consumerHit(t, 2, "SetA"); hit {
			t.Fatalf("隔 2 不应中第 1 级（SetA@0）")
		}
		if _, hit := consumerHit(t, 3, "A", "B", "SetA"); !hit {
			t.Fatalf("隔 3 应中自根数起第 3 级（SetA@2）")
		}
		if _, hit := consumerHit(t, 3, "Shell", "SetA"); hit {
			t.Fatalf("隔 3 不应中第 2 级（SetA@1）")
		}
	})
	t.Run("任意恒真", func(t *testing.T) {
		for _, chain := range [][]string{
			{"SetA"}, {"Shell", "SetA"}, {"A", "B", "SetA"}, {"A", "B", "C", "SetA"},
		} {
			decision, hit := consumerHit(t, 0, chain...)
			if !hit {
				t.Fatalf("任意档应恒真（库根之下任一层）: %v", chain)
			}
			if v, ok := declOf(decision, "consumer", DimCollection); !ok || v != "A" {
				t.Fatalf("任意档集名应为 A: %q %v", v, ok)
			}
		}
	})
	t.Run("深层同名不误命中", func(t *testing.T) {
		// 两层同形目录（{集名} 模板对两层都「格式像」）：紧邻档下只有根直下
		// 层过附件——命中与取值均落在第 1 级，深层同名层不接管。
		consumer := refDirRowRule(t, "consumer", "{集名}", []CondxAttach{refScopeAttach(RefScopeLibraryRoot, 1)},
			[]ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1}})
		decision := runRefEval(t, []RuleEntry{consumer}, rootEvalInput("Top", "Deep"), nil)
		if !refIDsContain(decision.MatchedRuleIDs, "consumer") {
			t.Fatalf("根直下层（Top@0）应过紧邻附件命中")
		}
		if v, ok := declOf(decision, "consumer", DimCollection); !ok || v != "Top" {
			t.Fatalf("深层同名层不应接管取值（期望 Top）: %q %v", v, ok)
		}
	})
}

// TestRefScopeLibraryRootValidation 库根 sentinel 的构造期校验：跨规则五则与
// 供给方合格性全放行（无供给方规则在场也照常载入——无悬空可判）；聚合轨行
// 禁附件不因来源=库根而放松。
func TestRefScopeLibraryRootValidation(t *testing.T) {
	t.Run("库根放行·无供给方在场", func(t *testing.T) {
		consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("{集名}", []CondxAttach{refScopeAttach(RefScopeLibraryRoot, 1)})}, nil)
		profile, errs := ParseProfile(scopeProfileJSON(t, consumer))
		if len(errs) != 0 {
			t.Fatalf("库根来源不应有跨规则校验告警: %v", errs)
		}
		if !scopeLoadedIDs(profile)["consumer"] {
			t.Fatalf("库根来源条目应照常载入")
		}
	})
	t.Run("聚合轨行禁附件不变", func(t *testing.T) {
		agg := aggTailExtEntry("agg-scope-root", CondxTargetDir, "{标题}",
			[]CondxAttach{refScopeAttach(RefScopeLibraryRoot, 1)}, "title")
		profile, errs := ParseProfile(scopeProfileJSON(t, *agg))
		if scopeLoadedIDs(profile)["agg-scope-root"] {
			t.Fatalf("聚合轨行携带库根附件仍应跳过")
		}
		if len(errs) != 1 || !strings.Contains(scopeErrText(errs), "作用域附件") {
			t.Fatalf("聚合轨行附件应恰一条告警: %v", errs)
		}
	})
}

// TestRefScopeLibraryRootRoundTrip 库根附件的序列化往返：sentinel 值与层距
// 无损落档（JSON → 载入 → 回写 → 再载入），再载入后的条目求值面照常
//（隔 2 只中自根数起第 2 级）。
func TestRefScopeLibraryRootRoundTrip(t *testing.T) {
	consumer := scopeRuleJSON("consumer", []CondxRow{scopeDirRow("Set{集名}", []CondxAttach{refScopeAttach(RefScopeLibraryRoot, 2)})},
		[]ExtractionAction{{Dimension: DimCollection, From: FromCapture, CaptureName: "setname", Row: 1}})
	profile, errs := ParseProfile(scopeProfileJSON(t, consumer))
	if len(errs) != 0 || !scopeLoadedIDs(profile)["consumer"] {
		t.Fatalf("库根附件条目应载入: %v", errs)
	}
	pj, err := ruleToJSON(profile.Rules[0])
	if err != nil {
		t.Fatalf("回写序列化失败: %v", err)
	}
	data2, err := json.Marshal(&ruleProfileJSON{SchemaVersion: CurrentSchemaVersion, Rules: []ruleEntryJSON{*pj}})
	if err != nil {
		t.Fatalf("二次序列化失败: %v", err)
	}
	profile2, errs2 := ParseProfile(data2)
	if len(errs2) != 0 || !scopeLoadedIDs(profile2)["consumer"] {
		t.Fatalf("往返后再载入应零告警: %v", errs2)
	}
	attach := profile2.Rules[0].Conditions[0].Condition.Attach[0]
	if attach.Kind != CondxAttachRefScope || attach.Value != RefScopeLibraryRoot || attach.MinCount != 2 {
		t.Fatalf("往返后附件应无损（refScope/$root/2）: %+v", attach)
	}
	decision := runRefEval(t, profile2.Rules, rootEvalInput("Shell", "SetA"), nil)
	if !refIDsContain(decision.MatchedRuleIDs, "consumer") {
		t.Fatalf("往返后求值面应照常（隔 2 中第 2 级）: %v", decision.MatchedRuleIDs)
	}
	if v, ok := declOf(decision, "consumer", DimCollection); !ok || v != "A" {
		t.Fatalf("往返后集名应为 A: %q %v", v, ok)
	}
}

// TestRefScopeAssertPhrase 库根/规则两形态的摘要句式：库根「位于库根之下
//（紧邻/任意深度/隔N级）」、规则「位于规则 X 之下（…）」（ID 直读形态）。
func TestRefScopeAssertPhrase(t *testing.T) {
	cases := []struct {
		attach DirAssert
		want   string
	}{
		{DirAssert{Kind: CondxAttachRefScope, Value: RefScopeLibraryRoot, MinCount: 1}, "位于库根之下（紧邻）"},
		{DirAssert{Kind: CondxAttachRefScope, Value: RefScopeLibraryRoot, MinCount: 0}, "位于库根之下（任意深度）"},
		{DirAssert{Kind: CondxAttachRefScope, Value: RefScopeLibraryRoot, MinCount: 3}, "位于库根之下（隔3级）"},
		{DirAssert{Kind: CondxAttachRefScope, Value: "provider-a", MinCount: 1}, "位于规则 provider-a 之下（紧邻）"},
		{DirAssert{Kind: CondxAttachRefScope, Value: "provider-a", MinCount: 0}, "位于规则 provider-a 之下（任意深度）"},
		{DirAssert{Kind: CondxAttachRefScope, Value: "provider-a", MinCount: 2}, "位于规则 provider-a 之下（隔2级）"},
	}
	for _, c := range cases {
		if got := refScopeAssertPhrase(c.attach); got != c.want {
			t.Fatalf("摘要句式不符: 得到 %q，期望 %q", got, c.want)
		}
	}
}
