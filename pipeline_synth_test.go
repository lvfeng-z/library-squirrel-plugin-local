// 解析内核流水线合成段单元测试：策略聚合的先手与缺省回退、union 按值去重与
// 双来源角色并存、highTrust 可信度择一真值表、同层冲突待裁决、角色硬绑定条件。
package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestSynthesizeUnionCoexistWithRoles 覆盖双来源作者并存场景：目录源作者（画师）
// 与文件名源作者（委托方）两条声明在 union 下各自保留，并由角色硬绑定分别挂上
// 对应角色——并存不合并，无待裁决项。
func TestSynthesizeUnionCoexistWithRoles(t *testing.T) {
	decls := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "画师甲", Source: SourceDir, RuleID: "r-dir"},
		{Dimension: DimAuthor, Value: "委托方乙", Source: SourceFilename, RuleID: "r-file"},
	}
	policy := SynthesisPolicy{
		DimPolicies: []DimPolicy{{Dimension: DimAuthor, Strategy: StrategyUnion}},
		Roles: []RoleBinding{
			{Source: SourceDir, RoleName: "画师"},
			{Source: SourceFilename, RoleName: "委托方"},
		},
	}
	final, pending := Synthesize(decls, policy)
	if len(pending) != 0 {
		t.Fatalf("并集并存不应有待裁决项，实际 %+v", pending)
	}
	want := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "画师甲", Source: SourceDir, RoleName: "画师", RuleID: "r-dir"},
		{Dimension: DimAuthor, Value: "委托方乙", Source: SourceFilename, RoleName: "委托方", RuleID: "r-file"},
	}
	if !reflect.DeepEqual(final, want) {
		t.Errorf("合成结果为 %+v，期望 %+v", final, want)
	}
}

// TestSynthesizeUnionDedupByValue 覆盖 union 的按值去重：同值声明只保留首个
//（来源与规则归属取先到者），不同值全部保留。
func TestSynthesizeUnionDedupByValue(t *testing.T) {
	decls := []MetadataDeclaration{
		{Dimension: DimTag, Value: "同人志", Source: SourceFilename, RuleID: "r1"},
		{Dimension: DimTag, Value: "同人志", Source: SourceConstant, RuleID: "r2"},
		{Dimension: DimTag, Value: "漫画", Source: SourceFilename, RuleID: "r3"},
	}
	final, pending := Synthesize(decls, SynthesisPolicy{
		DimPolicies: []DimPolicy{{Dimension: DimTag, Strategy: StrategyUnion}},
	})
	if len(pending) != 0 {
		t.Fatalf("并集不应有待裁决项，实际 %+v", pending)
	}
	want := []MetadataDeclaration{
		{Dimension: DimTag, Value: "同人志", Source: SourceFilename, RuleID: "r1"},
		{Dimension: DimTag, Value: "漫画", Source: SourceFilename, RuleID: "r3"},
	}
	if !reflect.DeepEqual(final, want) {
		t.Errorf("去重结果为 %+v，期望 %+v（同值保首个）", final, want)
	}
}

// TestSynthesizeHighTrustPick 覆盖 highTrust 可信度择一真值表：可信度序
// dir > predicate > filename > group > constant；同层同值取首个不作冲突。
func TestSynthesizeHighTrustPick(t *testing.T) {
	cases := []struct {
		name      string
		decls     []MetadataDeclaration
		wantValue string
	}{
		{
			name: "目录胜文件名",
			decls: []MetadataDeclaration{
				{Dimension: DimTitle, Value: "目录标题", Source: SourceDir},
				{Dimension: DimTitle, Value: "文件名标题", Source: SourceFilename},
			},
			wantValue: "目录标题",
		},
		{
			name: "文件名胜常量",
			decls: []MetadataDeclaration{
				{Dimension: DimTitle, Value: "常量标题", Source: SourceConstant},
				{Dimension: DimTitle, Value: "文件名标题", Source: SourceFilename},
			},
			wantValue: "文件名标题",
		},
		{
			name: "谓词胜文件名",
			decls: []MetadataDeclaration{
				{Dimension: DimTitle, Value: "文件名标题", Source: SourceFilename},
				{Dimension: DimTitle, Value: "库内确认标题", Source: SourcePredicate},
			},
			wantValue: "库内确认标题",
		},
		{
			name: "群组胜常量",
			decls: []MetadataDeclaration{
				{Dimension: DimTitle, Value: "常量标题", Source: SourceConstant},
				{Dimension: DimTitle, Value: "群组标题", Source: SourceGroup},
			},
			wantValue: "群组标题",
		},
		{
			name: "目录胜谓词",
			decls: []MetadataDeclaration{
				{Dimension: DimTitle, Value: "库内确认标题", Source: SourcePredicate},
				{Dimension: DimTitle, Value: "目录标题", Source: SourceDir},
			},
			wantValue: "目录标题",
		},
		{
			name: "仅常量取常量",
			decls: []MetadataDeclaration{
				{Dimension: DimTitle, Value: "常量标题", Source: SourceConstant},
			},
			wantValue: "常量标题",
		},
		{
			name: "同层同值保首个",
			decls: []MetadataDeclaration{
				{Dimension: DimTitle, Value: "同值标题", Source: SourceFilename, RuleID: "r1"},
				{Dimension: DimTitle, Value: "同值标题", Source: SourceFilename, RuleID: "r2"},
			},
			wantValue: "同值标题",
		},
	}
	policy := SynthesisPolicy{DimPolicies: []DimPolicy{{Dimension: DimTitle, Strategy: StrategyHighTrust}}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			final, pending := Synthesize(tc.decls, policy)
			if len(pending) != 0 {
				t.Fatalf("唯一最高可信级不应有待裁决项，实际 %+v", pending)
			}
			if len(final) != 1 {
				t.Fatalf("产出 %d 条声明，期望 1 条：%+v", len(final), final)
			}
			if final[0].Value != tc.wantValue {
				t.Errorf("择中值 %q，期望 %q", final[0].Value, tc.wantValue)
			}
		})
	}
}

// TestSynthesizeHighTrustSameLevelConflict 覆盖同层冲突：两条目录声明取值不同时
// 该维度不产出，记唯一待裁决项；候选只含冲突层（低可信的文件名声明不参与），
// Reason 为点名维度与冲突原因的可读中文。
func TestSynthesizeHighTrustSameLevelConflict(t *testing.T) {
	decls := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "作者甲", Source: SourceDir, RuleID: "r1"},
		{Dimension: DimAuthor, Value: "作者乙", Source: SourceDir, RuleID: "r2"},
		{Dimension: DimAuthor, Value: "文件名作者", Source: SourceFilename, RuleID: "r3"},
	}
	final, pending := Synthesize(decls, SynthesisPolicy{
		DimPolicies: []DimPolicy{{Dimension: DimAuthor, Strategy: StrategyHighTrust}},
	})
	if len(final) != 0 {
		t.Errorf("同层冲突维度不应产出声明，实际 %+v", final)
	}
	if len(pending) != 1 {
		t.Fatalf("待裁决项数 %d，期望 1", len(pending))
	}
	item := pending[0]
	if item.Dimension != DimAuthor {
		t.Errorf("待裁决维度为 %q，期望 %q", item.Dimension, DimAuthor)
	}
	wantCandidates := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "作者甲", Source: SourceDir, RuleID: "r1"},
		{Dimension: DimAuthor, Value: "作者乙", Source: SourceDir, RuleID: "r2"},
	}
	if !reflect.DeepEqual(item.Candidates, wantCandidates) {
		t.Errorf("候选为 %+v，期望 %+v（不含低可信层）", item.Candidates, wantCandidates)
	}
	if !strings.Contains(item.Reason, "同层冲突") || !strings.Contains(item.Reason, string(DimAuthor)) {
		t.Errorf("Reason %q 应为点名维度与同层冲突的可读描述", item.Reason)
	}
}

// TestSynthesizeNoDeclarationNoOutput 覆盖维度无声明与缺省回退：无声明的维度
// 不产出也不记待裁决；空策略下 author 回退 union 保留声明。
func TestSynthesizeNoDeclarationNoOutput(t *testing.T) {
	decls := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "作者甲", Source: SourceFilename, RuleID: "r1"},
	}
	final, pending := Synthesize(decls, SynthesisPolicy{})
	if len(final) != 1 || final[0].Value != "作者甲" {
		t.Errorf("空策略下 author 应回退 union 保留声明，实际 %+v", final)
	}
	if len(pending) != 0 {
		t.Errorf("无冲突不应有待裁决项，实际 %+v", pending)
	}
}

// TestAggregateSynthesisUserFirstThenDefault 覆盖跨规则聚合：用户维度的首个
// 策略先于内置生效，内置配置中用户未覆盖的维度照常取用，全部未配置的维度回退
// 缺省策略，角色绑定按规则序依序拼接。
func TestAggregateSynthesisUserFirstThenDefault(t *testing.T) {
	rules := []RuleEntry{
		{
			ID: "user-policy",
			Synthesis: SynthesisPolicy{
				DimPolicies: []DimPolicy{{Dimension: DimAuthor, Strategy: StrategyHighTrust}},
				Roles:       []RoleBinding{{Source: SourceDir, RoleName: "画师"}},
			},
		},
		{
			ID: "builtin-policy",
			Synthesis: SynthesisPolicy{
				DimPolicies: []DimPolicy{
					{Dimension: DimAuthor, Strategy: StrategyUnion},
					{Dimension: DimTitle, Strategy: StrategyUnion},
				},
				Roles: []RoleBinding{{Source: SourceFilename, RoleName: "委托方"}},
			},
		},
	}
	policy := AggregateSynthesis(rules)
	wantStrategies := []DimPolicy{
		{Dimension: DimAuthor, Strategy: StrategyHighTrust},
		{Dimension: DimTitle, Strategy: StrategyUnion},
		{Dimension: DimTag, Strategy: StrategyUnion},
		{Dimension: DimCollection, Strategy: StrategyUnion},
	}
	if !reflect.DeepEqual(policy.DimPolicies, wantStrategies) {
		t.Errorf("维度策略为 %+v，期望 %+v", policy.DimPolicies, wantStrategies)
	}
	wantRoles := []RoleBinding{
		{Source: SourceDir, RoleName: "画师"},
		{Source: SourceFilename, RoleName: "委托方"},
	}
	if !reflect.DeepEqual(policy.Roles, wantRoles) {
		t.Errorf("角色绑定为 %+v，期望 %+v（按规则序拼接）", policy.Roles, wantRoles)
	}
}

// TestAggregateSynthesisDefaultsOnly 覆盖无任何配置的缺省回退：title 取
// highTrust，author/tag/collection 取 union，无角色绑定。
func TestAggregateSynthesisDefaultsOnly(t *testing.T) {
	policy := AggregateSynthesis(nil)
	want := []DimPolicy{
		{Dimension: DimTitle, Strategy: StrategyHighTrust},
		{Dimension: DimAuthor, Strategy: StrategyUnion},
		{Dimension: DimTag, Strategy: StrategyUnion},
		{Dimension: DimCollection, Strategy: StrategyUnion},
	}
	if !reflect.DeepEqual(policy.DimPolicies, want) {
		t.Errorf("缺省维度策略为 %+v，期望 %+v", policy.DimPolicies, want)
	}
	if len(policy.Roles) != 0 {
		t.Errorf("无配置时不应有角色绑定，实际 %+v", policy.Roles)
	}
}

// TestSynthesizeRoleBindingOnlyWhenEmpty 覆盖角色硬绑定条件：Source 匹配且
// RoleName 为空才绑定，声明已带 RoleName 不覆盖，Source 不匹配不绑定。
func TestSynthesizeRoleBindingOnlyWhenEmpty(t *testing.T) {
	decls := []MetadataDeclaration{
		{Dimension: DimTag, Value: "标签甲", Source: SourceFilename, RuleID: "r1"},
		{Dimension: DimTag, Value: "标签乙", Source: SourceFilename, RoleName: "既有角色", RuleID: "r2"},
		{Dimension: DimTag, Value: "标签丙", Source: SourceConstant, RuleID: "r3"},
	}
	final, pending := Synthesize(decls, SynthesisPolicy{
		DimPolicies: []DimPolicy{{Dimension: DimTag, Strategy: StrategyUnion}},
		Roles:       []RoleBinding{{Source: SourceFilename, RoleName: "文件名角色"}},
	})
	if len(pending) != 0 {
		t.Fatalf("并集不应有待裁决项，实际 %+v", pending)
	}
	if len(final) != 3 {
		t.Fatalf("产出 %d 条声明，期望 3 条：%+v", len(final), final)
	}
	if final[0].RoleName != "文件名角色" {
		t.Errorf("空 RoleName 未绑定，实际 %q", final[0].RoleName)
	}
	if final[1].RoleName != "既有角色" {
		t.Errorf("已有 RoleName 被覆盖，实际 %q", final[1].RoleName)
	}
	if final[2].RoleName != "" {
		t.Errorf("Source 不匹配不应绑定，实际 %q", final[2].RoleName)
	}
}
