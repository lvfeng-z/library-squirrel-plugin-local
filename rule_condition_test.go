// 通用条件表达式（档案 v3 语义层）的单元测试：操作数与运算符求值（含嵌套）、
// 构造期静态校验（捕获组存在性 / 类型匹配）的可读错误、v1/v2 旧档案兼容回归
// （v2 谓词组合映射为等价表达式树，行为零变化）、读侧版本不符（未来更高版本）
// 告警不中断，以及 v3 表达式档案的序列化往返。
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// equalCondExpr 条件表达式树的递归等值比较（节点含切片字段，不可用 ==）。
func equalCondExpr(a, b *CondExpr) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind || a.Source != b.Source || a.Dimension != b.Dimension ||
		a.CaptureName != b.CaptureName || a.Str != b.Str || a.Num != b.Num {
		return false
	}
	if (a.Min == nil) != (b.Min == nil) || (a.Max == nil) != (b.Max == nil) {
		return false
	}
	if a.Min != nil && *a.Min != *b.Min {
		return false
	}
	if a.Max != nil && *a.Max != *b.Max {
		return false
	}
	if !equalCondExpr(a.Left, b.Left) || !equalCondExpr(a.Right, b.Right) ||
		!equalCondExpr(a.Operand, b.Operand) || !equalCondExpr(a.Inner, b.Inner) {
		return false
	}
	if len(a.Subs) != len(b.Subs) {
		return false
	}
	for i := range a.Subs {
		if !equalCondExpr(a.Subs[i], b.Subs[i]) {
			return false
		}
	}
	return true
}

// —— 退出标准1：操作数与运算符求值正确（含嵌套）——

// TestCondExprEvaluationTable 条件表达式树求值：谓词在场/缺席（经逻辑非表达）、
// 捕获组引用（相等/不等、未绑定不可用）、深度与群组产物字段的数值区间
// （双边/单边）、字面量比较、与/或/非逻辑组合与嵌套。
func TestCondExprEvaluationTable(t *testing.T) {
	dirAuthor := DeclPredicate{Source: SourceDir, Dimension: DimAuthor}
	fileTitle := DeclPredicate{Source: SourceFilename, Dimension: DimTitle}
	num := func(v float64) *float64 { return &v }

	ctx := &ParseContext{
		DirNames: []string{"作者甲", "画集"},
		Depth:    2,
		Captures: []CaptureBinding{{Name: "author", Value: "作者甲"}},
		Group:    &GroupAnalysis{Key: "画集", Prefix: "画集", Seq: 2, Size: 5},
	}
	decls := []MetadataDeclaration{
		{Dimension: DimAuthor, Value: "作者甲", Source: SourceDir},
	}

	cases := []struct {
		name string
		expr *CondExpr
		want bool
	}{
		// 声明在场/缺席谓词（含经逻辑非表达的缺席）
		{"谓词在场", predicateExpr(dirAuthor), true},
		{"谓词缺席", predicateExpr(fileTitle), false},
		{"非谓词=缺席", &CondExpr{Kind: ExprNot, Inner: predicateExpr(fileTitle)}, true},
		// 命名捕获组引用：相等/不等
		{"捕获组相等命中", &CondExpr{Kind: ExprEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "author"},
			Right: &CondExpr{Kind: ExprString, Str: "作者甲"}}, true},
		{"捕获组相等不中", &CondExpr{Kind: ExprEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "author"},
			Right: &CondExpr{Kind: ExprString, Str: "作者乙"}}, false},
		{"捕获组不等", &CondExpr{Kind: ExprNotEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "author"},
			Right: &CondExpr{Kind: ExprString, Str: "作者乙"}}, true},
		{"捕获组未绑定不可用", &CondExpr{Kind: ExprEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "nope"},
			Right: &CondExpr{Kind: ExprString, Str: ""}}, false},
		{"捕获组未绑定的不等也保守为假", &CondExpr{Kind: ExprNotEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "nope"},
			Right: &CondExpr{Kind: ExprString, Str: "任何值"}}, false},
		// 深度与群组产物字段：数值区间（双边/单边）
		{"深度区间内", &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprDepth}, Min: num(2), Max: num(3)}, true},
		{"深度区间外", &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprDepth}, Min: num(3), Max: num(4)}, false},
		{"深度只设上界", &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprDepth}, Max: num(2)}, true},
		{"群组大小区间", &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSize}, Min: num(2)}, true},
		{"群组序号区间外", &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSeq}, Min: num(3)}, false},
		{"数值字面量相等", &CondExpr{Kind: ExprEqual,
			Left: &CondExpr{Kind: ExprDepth}, Right: &CondExpr{Kind: ExprNumber, Num: 2}}, true},
		// 逻辑组合（与/或/非）与嵌套
		{"与全真", &CondExpr{Kind: ExprAnd, Subs: []*CondExpr{
			predicateExpr(dirAuthor),
			&CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSize}, Min: num(2)},
		}}, true},
		{"与一假", &CondExpr{Kind: ExprAnd, Subs: []*CondExpr{
			predicateExpr(fileTitle),
			&CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSize}, Min: num(2)},
		}}, false},
		{"或一真", &CondExpr{Kind: ExprOr, Subs: []*CondExpr{
			predicateExpr(fileTitle),
			&CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprDepth}, Max: num(2)},
		}}, true},
		{"或全假", &CondExpr{Kind: ExprOr, Subs: []*CondExpr{
			predicateExpr(fileTitle),
			&CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprDepth}, Min: num(3)},
		}}, false},
		{"嵌套：非(或(谓词, 与(非谓词, 区间)))", &CondExpr{Kind: ExprNot, Inner: &CondExpr{Kind: ExprOr, Subs: []*CondExpr{
			predicateExpr(dirAuthor),
			&CondExpr{Kind: ExprAnd, Subs: []*CondExpr{
				&CondExpr{Kind: ExprNot, Inner: predicateExpr(dirAuthor)},
				&CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSeq}, Max: num(1)},
			}},
		}}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.expr.Holds(ctx, decls); got != tc.want {
				t.Fatalf("求值 = %v，期望 %v（表达式: %+v）", got, tc.want, tc.expr)
			}
		})
	}

	// 非群组成员：群组产物字段取值不可用，含它的区间比较为假
	lone := &ParseContext{DirNames: []string{}, Depth: 0}
	groupSizeCond := &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSize}, Min: num(2)}
	if groupSizeCond.Holds(lone, nil) {
		t.Fatal("孤立文件的群组大小比较应为假（非群组成员取值不可用）")
	}
	// nil 条件恒真
	var nilCond *CondExpr
	if !nilCond.Holds(ctx, decls) {
		t.Fatal("nil 条件应恒真")
	}
}

// TestCondExprPipelineEvaluation 提取段两遍执行下表达式条件参与合成：
// 捕获组相等比较门控常量声明、孤立文件场景（目录源作者缺席 → 常量作者）、
// 群组字段区间门控（群组成员生效、孤立文件不生效）。
func TestCondExprPipelineEvaluation(t *testing.T) {
	// 目录层规则：命中「作者甲」目录层即产出目录源作者声明（无条件，第一遍执行）
	ancestor := mustRegex(t, string(TargetFileName), `^作者甲$`, nil)
	location, err := NewLocationPattern(0, 0, ancestor)
	if err != nil {
		t.Fatalf("构造目录层模式失败: %v", err)
	}
	dirRule := RuleEntry{
		ID: "cond-eval-dir", Source: RuleSourceManual, Enabled: true, Pattern: location,
		Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromAncestorDir}},
	}
	// 捕获组相等条件：文件名捕获的 author = "作者甲" 时声明集归属
	captureRule := RuleEntry{
		ID: "cond-eval-capture", Source: RuleSourceManual, Enabled: true,
		Pattern: mustRegex(t, string(TargetFileName), `^(?P<author>[^_]+)_\d+\.jpg$`, []string{"author"}),
		Extractions: []ExtractionAction{{Dimension: DimCollection, From: FromConstant, Constant: "集甲",
			When: &CondExpr{Kind: ExprEqual,
				Left:  &CondExpr{Kind: ExprCapture, CaptureName: "author"},
				Right: &CondExpr{Kind: ExprString, Str: "作者甲"}}}},
	}
	// 孤立文件场景：目录源作者缺席时兜底常量作者
	loneRule := RuleEntry{
		ID: "cond-eval-lone", Source: RuleSourceManual, Enabled: true,
		Pattern: mustRegex(t, string(TargetFileName), `^.+\.jpg$`, nil),
		Extractions: []ExtractionAction{{Dimension: DimAuthor, From: FromConstant, Constant: "佚名",
			When: &CondExpr{Kind: ExprNot, Inner: predicateExpr(DeclPredicate{Source: SourceDir, Dimension: DimAuthor})}}},
	}
	// 定群规则：track 前缀（捕获组提取分组键）+ 序号可提取的兄弟成群（分组段写入 ctx.Group）
	groupPattern, err := NewSiblingPattern(`^(track)_`, `_(\d+)\.mp3$`, 2, OutcomeCollection)
	if err != nil {
		t.Fatalf("构造定群模式失败: %v", err)
	}
	siblingRule := RuleEntry{
		ID: "cond-eval-sibling", Source: RuleSourceManual, Enabled: true, Pattern: groupPattern,
	}
	// 群组字段区间条件：群组成员数达 2 时声明集归属（孤立文件不生效）
	minSize := 2.0
	groupRule := RuleEntry{
		ID: "cond-eval-group", Source: RuleSourceManual, Enabled: true,
		Pattern: mustRegex(t, string(TargetFileName), `^track_\d+\.mp3$`, nil),
		Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromConstant, Constant: "多轨作品",
			When: &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSize}, Min: &minSize}}},
	}

	files := []ScanFile{
		{FileName: "作者甲_01.jpg", DirNames: []string{"作者甲"}, RelPath: "作者甲/作者甲_01.jpg"},
		{FileName: "作者乙_01.jpg", RelPath: "作者乙_01.jpg"},
		{FileName: "track_01.mp3", RelPath: "album/track_01.mp3"},
		{FileName: "track_02.mp3", RelPath: "album/track_02.mp3"},
		{FileName: "single.mp3", RelPath: "album/single.mp3"},
	}
	rules := []RuleEntry{dirRule, captureRule, loneRule, siblingRule, groupRule}
	decisions := RunPipeline(files, rules, nil)
	byRel := make(map[string]FileDecision, len(decisions))
	for _, d := range decisions {
		byRel[d.RelPath] = d
	}
	valuesOf := func(rel string, dim MetadataDimension) []string {
		var out []string
		for _, decl := range byRel[rel].Declarations {
			if decl.Dimension == dim {
				out = append(out, decl.Value)
			}
		}
		return out
	}

	// 目录内文件：目录源作者在场 → 兜底不生效；捕获组相等 → 集归属生效
	if got := valuesOf("作者甲/作者甲_01.jpg", DimAuthor); len(got) != 1 || got[0] != "作者甲" {
		t.Fatalf("目录内文件的作者应仅目录源声明: %v", got)
	}
	if got := valuesOf("作者甲/作者甲_01.jpg", DimCollection); len(got) != 1 || got[0] != "集甲" {
		t.Fatalf("捕获组相等条件应产出集归属: %v", got)
	}
	// 孤立文件：目录源作者缺席 → 兜底常量作者生效；捕获组不等于目标 → 不产出
	if got := valuesOf("作者乙_01.jpg", DimAuthor); len(got) != 1 || got[0] != "佚名" {
		t.Fatalf("孤立文件的作者应为缺席分支常量「佚名」: %v", got)
	}
	if got := valuesOf("作者乙_01.jpg", DimCollection); len(got) != 0 {
		t.Fatalf("捕获组相等不中时不应产出集归属: %v", got)
	}
	// 群组字段：成对 track 成群（size=2）生效；single 落单不生效
	if got := valuesOf("album/track_01.mp3", DimTitle); len(got) != 1 || got[0] != "多轨作品" {
		t.Fatalf("群组成员应满足大小区间条件: %v", got)
	}
	if got := valuesOf("album/single.mp3", DimTitle); len(got) != 0 {
		t.Fatalf("孤立文件的群组大小条件应不可用而不产出: %v", got)
	}
}

// —— 退出标准2：构造期静态校验报可读中文错误 ——

// TestCondExprConstructionValidation 构造期逐条校验：捕获组引用不存在、类型
// 不匹配（对字符串字面量做数值区间比较 / 相等比较两侧类型不一致 / 布尔量入
// 比较 / 逻辑组合接非布尔子式）、未知判别类别与残缺结构，均跳过规则并报含
// 规则 ID 的中文错误，不阻塞其余规则。
func TestCondExprConstructionValidation(t *testing.T) {
	pattern := mustRegex(t, string(TargetFileName), `^(?P<author>[^_]+)_\d+\.jpg$`, []string{"author"})
	num := func(v float64) *float64 { return &v }
	cases := []struct {
		name    string
		when    *CondExpr
		wantMsg string
	}{
		{"引用不存在的捕获组名", &CondExpr{Kind: ExprEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "nope"},
			Right: &CondExpr{Kind: ExprString, Str: "x"}}, "未声明的捕获组名"},
		{"对字符串字面量做数值区间比较", &CondExpr{Kind: ExprRange,
			Operand: &CondExpr{Kind: ExprString, Str: "x"}, Min: num(1)}, "要求数值操作数"},
		{"相等比较两侧类型不一致", &CondExpr{Kind: ExprEqual,
			Left:  &CondExpr{Kind: ExprCapture, CaptureName: "author"},
			Right: &CondExpr{Kind: ExprNumber, Num: 1}}, "两侧类型不一致"},
		{"布尔谓词入相等比较", &CondExpr{Kind: ExprEqual,
			Left:  predicateExpr(DeclPredicate{Source: SourceDir, Dimension: DimAuthor}),
			Right: predicateExpr(DeclPredicate{Source: SourceDir, Dimension: DimTitle})}, "不能比较布尔谓词"},
		{"逻辑与接非布尔子式", &CondExpr{Kind: ExprAnd, Subs: []*CondExpr{
			predicateExpr(DeclPredicate{Source: SourceDir, Dimension: DimAuthor}),
			&CondExpr{Kind: ExprCapture, CaptureName: "author"},
		}}, "子表达式须为布尔"},
		{"逻辑非接非布尔内层", &CondExpr{Kind: ExprNot,
			Inner: &CondExpr{Kind: ExprString, Str: "x"}}, "内层表达式须为布尔"},
		{"未知判别类别", &CondExpr{Kind: ExprKind("mystery")}, "未知的判别类别"},
		{"未知声明来源谓词", predicateExpr(DeclPredicate{Source: "nope", Dimension: DimAuthor}), "未知的声明来源"},
		{"数值区间缺界", &CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprDepth}}, "缺少区间界"},
		{"数值区间下界大于上界", &CondExpr{Kind: ExprRange,
			Operand: &CondExpr{Kind: ExprDepth}, Min: num(3), Max: num(2)}, "下界 3 大于上界 2"},
		{"逻辑组合无子式", &CondExpr{Kind: ExprOr}, "至少需要一个子表达式"},
		{"比较缺右侧操作数", &CondExpr{Kind: ExprEqual,
			Left: &CondExpr{Kind: ExprString, Str: "x"}}, "缺少左侧（left）或右侧（right）操作数"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := RuleEntry{
				ID: "bad-cond", Source: RuleSourceManual, Enabled: true, Pattern: pattern,
				Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromConstant, Constant: "值", When: tc.when}},
			}
			data, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{rule}})
			if err != nil {
				t.Fatalf("构造测试档案失败: %v", err)
			}
			profile, errs := ParseProfile(data)
			if len(errs) != 1 {
				t.Fatalf("应恰一条错误（跳过规则），实际 %d 条: %v", len(errs), errs)
			}
			msg := errs[0].Error()
			if !strings.Contains(msg, "bad-cond") || !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("错误应含规则 ID 与 %q: %q", tc.wantMsg, msg)
			}
			if profile == nil || len(profile.Rules) != 0 {
				t.Fatalf("坏条件规则应被跳过且档案可用: %+v", profile)
			}
		})
	}

	// 合法捕获组引用（模式声明内）通过校验，不阻塞载入
	good := RuleEntry{
		ID: "good-cond", Source: RuleSourceManual, Enabled: true, Pattern: pattern,
		Extractions: []ExtractionAction{{Dimension: DimTitle, From: FromConstant, Constant: "值",
			When: &CondExpr{Kind: ExprNotEqual,
				Left:  &CondExpr{Kind: ExprCapture, CaptureName: "author"},
				Right: &CondExpr{Kind: ExprString, Str: "x"}}}},
	}
	data, _ := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: []RuleEntry{good}})
	profile, errs := ParseProfile(data)
	if profile == nil || len(errs) != 0 || len(profile.Rules) != 1 {
		t.Fatalf("声明内捕获组引用应通过构造期校验: %v %v", profile, errs)
	}
}

// —— 退出标准3：无条件旧档案行为零变化（兼容回归）——

// TestLegacyConditionEquivalence v2 谓词组合 → 等价表达式树的等价性锚定：
// 对各形态的谓词组合 × 各声明集，直读求值（SynthCondition.Holds，升级前
// 语义）与映射树求值（CondExpr.Holds）结果恒等。
func TestLegacyConditionEquivalence(t *testing.T) {
	dirAuthor := DeclPredicate{Source: SourceDir, Dimension: DimAuthor}
	fileTitle := DeclPredicate{Source: SourceFilename, Dimension: DimTitle}
	conditions := []SynthCondition{
		{},
		{Present: []DeclPredicate{dirAuthor}},
		{Absent: []DeclPredicate{dirAuthor}},
		{Present: []DeclPredicate{dirAuthor, fileTitle}},
		{Absent: []DeclPredicate{dirAuthor, fileTitle}},
		{Present: []DeclPredicate{dirAuthor}, Absent: []DeclPredicate{fileTitle}},
		{Present: []DeclPredicate{fileTitle}, Absent: []DeclPredicate{dirAuthor}},
	}
	declSets := [][]MetadataDeclaration{
		{},
		{{Dimension: DimAuthor, Value: "甲", Source: SourceDir}},
		{{Dimension: DimTitle, Value: "t", Source: SourceFilename}},
		{{Dimension: DimAuthor, Value: "甲", Source: SourceDir}, {Dimension: DimTitle, Value: "t", Source: SourceFilename}},
	}
	ctx := &ParseContext{Depth: 1, DirNames: []string{"作者甲"}}
	for i, cond := range conditions {
		mapped := legacyConditionExpr(cond)
		if mapped == nil {
			// 恒真形态归一为无条件：等价性以 nil 树恒真承载
			if !cond.Holds(nil) {
				t.Fatalf("条件 %d：空谓词组合应为恒真", i)
			}
			continue
		}
		for j, decls := range declSets {
			want := cond.Holds(decls)
			got := mapped.Holds(ctx, decls)
			if got != want {
				t.Fatalf("条件 %d × 声明集 %d：直读求值 %v 与映射树求值 %v 不等价", i, j, want, got)
			}
		}
	}
}

// v2 条件档案样例（present/absent 谓词组合形态）与等价的 v3 表达式档案：
// 规则一为目录层作者声明（无条件），规则二为目录源作者在场/缺席的条件标题。
const v2ConditionProfile = `{"schemaVersion":2,"rules":[` +
	`{"id":"legacy-dir","description":"目录作者","source":"manual","enabled":true,` +
	`"pattern":{"kind":"location","minDepth":0,"maxDepth":0,"ancestor":{"kind":"regex","target":"filename","expr":"^作者甲$","captures":[]}},` +
	`"extractions":[{"dimension":"author","from":"ancestorDir"}],"synthesis":{}},` +
	`{"id":"legacy-cond","description":"v2 条件规则","source":"manual","enabled":true,` +
	`"pattern":{"kind":"regex","target":"filename","expr":"^.+\\.jpg$","captures":[]},` +
	`"extractions":[{"dimension":"title","from":"constant","constant":"在场值","when":{"present":[{"source":"dir","dimension":"author"}]}},` +
	`{"dimension":"title","from":"constant","constant":"缺席值","when":{"absent":[{"source":"dir","dimension":"author"}]}}],` +
	`"synthesis":{}}]}`

const v3EquivalentProfile = `{"schemaVersion":3,"rules":[` +
	`{"id":"legacy-dir","description":"目录作者","source":"manual","enabled":true,` +
	`"pattern":{"kind":"location","minDepth":0,"maxDepth":0,"ancestor":{"kind":"regex","target":"filename","expr":"^作者甲$","captures":[]}},` +
	`"extractions":[{"dimension":"author","from":"ancestorDir"}],"synthesis":{}},` +
	`{"id":"legacy-cond","description":"v2 条件规则","source":"manual","enabled":true,` +
	`"pattern":{"kind":"regex","target":"filename","expr":"^.+\\.jpg$","captures":[]},` +
	`"extractions":[{"dimension":"title","from":"constant","constant":"在场值","when":{"kind":"predicate","source":"dir","dimension":"author"}},` +
	`{"dimension":"title","from":"constant","constant":"缺席值","when":{"kind":"not","inner":{"kind":"predicate","source":"dir","dimension":"author"}}}],` +
	`"synthesis":{}}]}`

// TestProfileV2ConditionCompat v2 条件档案兼容载入：载入即升格 v3、条件映射
// 为等价表达式树；流水线求值结论与等价 v3 档案逐文件一致（升级前后行为零
// 变化）；重写后落盘为 v3 形态且仍可载入求值。
func TestProfileV2ConditionCompat(t *testing.T) {
	v2Profile, errs := ParseProfile([]byte(v2ConditionProfile))
	if v2Profile == nil || len(errs) != 0 {
		t.Fatalf("v2 条件档案应兼容载入: %v %v", v2Profile, errs)
	}
	if v2Profile.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("载入后应升格 v%d，实际 %d", CurrentSchemaVersion, v2Profile.SchemaVersion)
	}
	if len(v2Profile.Rules) != 2 || len(v2Profile.Rules[1].Extractions) != 2 {
		t.Fatalf("v2 规则应照常载入: %+v", v2Profile.Rules)
	}
	present, absent := v2Profile.Rules[1].Extractions[0].When, v2Profile.Rules[1].Extractions[1].When
	if present == nil || present.Kind != ExprPredicate || present.Source != SourceDir || present.Dimension != DimAuthor {
		t.Fatalf("在场分支应映射为在场谓词节点: %+v", present)
	}
	if absent == nil || absent.Kind != ExprNot || !equalCondExpr(absent.Inner, present) {
		t.Fatalf("缺席分支应映射为谓词取反节点: %+v", absent)
	}

	// 等价 v3 档案的求值结论与 v2 载入逐文件一致
	v3Profile, errs := ParseProfile([]byte(v3EquivalentProfile))
	if v3Profile == nil || len(errs) != 0 {
		t.Fatalf("等价 v3 档案应可载入: %v %v", v3Profile, errs)
	}
	files := []ScanFile{
		{FileName: "x.jpg", DirNames: []string{"作者甲"}, RelPath: "作者甲/x.jpg"},
		{FileName: "y.jpg", RelPath: "y.jpg"},
	}
	for _, file := range files {
		v2Decisions := RunPipeline([]ScanFile{file}, v2Profile.Rules, nil)
		v3Decisions := RunPipeline([]ScanFile{file}, v3Profile.Rules, nil)
		if len(v2Decisions) != 1 || len(v3Decisions) != 1 {
			t.Fatalf("%s：两侧各应一行结论", file.RelPath)
		}
		if !equalDeclarations(v2Decisions[0].Declarations, v3Decisions[0].Declarations) {
			t.Fatalf("%s：v2 载入与等价 v3 的求值结论不一致:\n%+v\n%+v", file.RelPath, v2Decisions[0].Declarations, v3Decisions[0].Declarations)
		}
		// 升级前语义锚定：目录源作者在场取「在场值」、缺席取「缺席值」
		want := "在场值"
		if len(file.DirNames) == 0 {
			want = "缺席值"
		}
		found := false
		for _, decl := range v2Decisions[0].Declarations {
			if decl.Dimension == DimTitle && decl.Value == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s：条件分支值应为 %q，实际声明: %+v", file.RelPath, want, v2Decisions[0].Declarations)
		}
	}

	// 重写落盘为 v3 形态，仍可载入
	rewritten, err := v2Profile.MarshalJSON()
	if err != nil {
		t.Fatalf("重写 v2 载入档案失败: %v", err)
	}
	if bytes.Contains(rewritten, []byte(`"present"`)) || bytes.Contains(rewritten, []byte(`"absent"`)) {
		t.Fatalf("重写后不应再含 v2 谓词组合字段: %s", rewritten)
	}
	if !bytes.Contains(rewritten, []byte(`"kind":"predicate"`)) {
		t.Fatalf("重写后应为 v3 表达式形态: %s", rewritten)
	}
	reloaded, errs := ParseProfile(rewritten)
	if reloaded == nil || len(errs) != 0 || reloaded.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("重写后档案应可载入: %v %v", reloaded, errs)
	}
}

// equalDeclarations 声明序列逐条等值比较（求值结论一致性断言用）。
func equalDeclarations(a, b []MetadataDeclaration) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestProfileV1UnconditionalZeroChange v1 无条件档案行为零变化：载入零错误、
// 动作无条件执行、求值结论与升级前一致。
func TestProfileV1UnconditionalZeroChange(t *testing.T) {
	v1 := `{"schemaVersion":1,"rules":[{"id":"v1-rule","description":"旧版规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^x\\.jpg$","captures":[]},` +
		`"extractions":[{"dimension":"title","from":"constant","constant":"旧值"}],"synthesis":{}}]}`
	profile, errs := ParseProfile([]byte(v1))
	if profile == nil || len(errs) != 0 {
		t.Fatalf("v1 档案应零错误载入: %v %v", profile, errs)
	}
	if len(profile.Rules) != 1 || profile.Rules[0].Extractions[0].When != nil {
		t.Fatalf("v1 规则动作应为无条件: %+v", profile.Rules)
	}
	decisions := RunPipeline([]ScanFile{{FileName: "x.jpg", RelPath: "x.jpg"}}, profile.Rules, nil)
	if len(decisions) != 1 || len(decisions[0].Declarations) != 1 ||
		decisions[0].Declarations[0].Value != "旧值" {
		t.Fatalf("v1 规则求值结论应与升级前一致: %+v", decisions)
	}
}

// TestParseConditionLegacyEmptyJSON v2 恒真形态（when 两列表皆空）归一为无条件。
func TestParseConditionLegacyEmptyJSON(t *testing.T) {
	action := ExtractionAction{}
	if err := json.Unmarshal([]byte(`{"dimension":"title","from":"constant","constant":"v","when":{}}`), &action); err != nil {
		t.Fatalf("恒真 when 应可载入: %v", err)
	}
	if action.When != nil {
		t.Fatalf("恒真 when 应归一为无条件（nil），实际: %+v", action.When)
	}
}

// —— 退出标准4：读侧版本不符（未来更高版本）告警且不中断 ——

// TestProfileFutureVersionBestEffort schemaVersion=99 的档案：记版本告警、按
// 尽力解析载入（可解析规则照常参与、无法构造的规则逐条跳过），不中断整体。
func TestProfileFutureVersionBestEffort(t *testing.T) {
	future := `{"schemaVersion":99,"rules":[` +
		`{"id":"future-good","description":"可解析规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^x\\.jpg$","captures":[]},` +
		`"extractions":[{"dimension":"title","from":"constant","constant":"未来值"}],"synthesis":{}},` +
		`{"id":"future-bad","description":"无法解析规则","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^y\\.jpg$","captures":[]},` +
		`"extractions":[{"dimension":"title","from":"constant","constant":"v","when":{"kind":"mystery"}}],"synthesis":{}}]}`
	profile, errs := ParseProfile([]byte(future))
	if profile == nil {
		t.Fatal("未来更高版本应按尽力解析返回档案，不中断整体")
	}
	if len(errs) != 2 {
		t.Fatalf("应含 1 条版本告警 + 1 条跳过错误，实际 %d 条: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "高于当前支持的版本") {
		t.Fatalf("首条错误应为版本告警: %v", errs[0])
	}
	if !strings.Contains(errs[1].Error(), "future-bad") {
		t.Fatalf("无法解析的规则应逐条跳过并携带规则 ID: %v", errs[1])
	}
	if len(profile.Rules) != 1 || profile.Rules[0].ID != "future-good" {
		t.Fatalf("可解析规则应照常载入: %+v", profile.Rules)
	}
	// 载入的规则照常参与求值
	decisions := RunPipeline([]ScanFile{{FileName: "x.jpg", RelPath: "x.jpg"}}, profile.Rules, nil)
	if len(decisions) != 1 || len(decisions[0].Declarations) != 1 || decisions[0].Declarations[0].Value != "未来值" {
		t.Fatalf("未来版本档案的可解析规则应照常参与求值: %+v", decisions)
	}
}

// TestIsolatedFileConditionStrategyJSON 孤立文件场景的手写 v3 条件策略 JSON
// （目录源作者缺席 → 兜底常量作者并绑定角色）：完整 JSON 经 ParseProfile 构造期
// 校验通过并入档形态参与求值（手写进阶入口的端到端锚定）。
func TestIsolatedFileConditionStrategyJSON(t *testing.T) {
	profileJSON := `{"schemaVersion":3,"rules":[` +
		`{"id":"iso-dir","description":"目录层作者","source":"manual","enabled":true,` +
		`"pattern":{"kind":"location","minDepth":0,"maxDepth":0,"ancestor":{"kind":"regex","target":"filename","expr":"^作者-.*$","captures":[]}},` +
		`"extractions":[{"dimension":"author","from":"ancestorDir"}],"synthesis":{}},` +
		`{"id":"iso-lone","description":"目录源作者缺席时的兜底作者","source":"manual","enabled":true,` +
		`"pattern":{"kind":"regex","target":"filename","expr":"^.+\\.(jpg|png)$","captures":[]},` +
		`"extractions":[{"dimension":"author","from":"constant","constant":"佚名","roleName":"fallback",` +
		`"when":{"kind":"not","inner":{"kind":"predicate","source":"dir","dimension":"author"}}}],"synthesis":{}}]}`
	profile, errs := ParseProfile([]byte(profileJSON))
	if profile == nil || len(errs) != 0 {
		t.Fatalf("手写 v3 条件策略应通过构造期校验: %v %v", profile, errs)
	}
	files := []ScanFile{
		{FileName: "a.jpg", DirNames: []string{"作者-甲"}, RelPath: "作者-甲/a.jpg"},
		{FileName: "b.png", RelPath: "b.png"},
	}
	decisions := RunPipeline(files, profile.Rules, nil)
	authorOf := func(rel string) (string, string) {
		for _, decl := range decisions {
			if decl.RelPath != rel {
				continue
			}
			for _, d := range decl.Declarations {
				if d.Dimension == DimAuthor {
					return d.Value, d.RoleName
				}
			}
		}
		return "", ""
	}
	if value, role := authorOf("作者-甲/a.jpg"); value != "作者-甲" || role != "" {
		t.Fatalf("目录内文件应取目录源作者且无兜底角色，实得 %q/%q", value, role)
	}
	if value, role := authorOf("b.png"); value != "佚名" || role != "fallback" {
		t.Fatalf("孤立文件应取兜底常量作者并带角色，实得 %q/%q", value, role)
	}
}

// —— v3 表达式档案的序列化往返 ——

// TestCondExprProfileRoundTrip 含各操作数与运算符的表达式档案往返：
// MarshalJSON → ParseProfile → 再 MarshalJSON 字节等价，条件树保结构。
func TestCondExprProfileRoundTrip(t *testing.T) {
	num := func(v float64) *float64 { return &v }
	profile := RuleProfile{
		SchemaVersion: CurrentSchemaVersion,
		Rules: []RuleEntry{{
			ID: "expr-rt", Description: "表达式往返", Source: RuleSourceManual, Enabled: true,
			Pattern: mustRegex(t, string(TargetFileName), `^(?P<author>[^_]+)_(?P<seq>\d+)\.jpg$`, []string{"author", "seq"}),
			Extractions: []ExtractionAction{
				{Dimension: DimTitle, From: FromConstant, Constant: "甲",
					When: &CondExpr{Kind: ExprOr, Subs: []*CondExpr{
						&CondExpr{Kind: ExprEqual,
							Left:  &CondExpr{Kind: ExprCapture, CaptureName: "author"},
							Right: &CondExpr{Kind: ExprString, Str: "作者甲"}},
						&CondExpr{Kind: ExprNot, Inner: predicateExpr(DeclPredicate{Source: SourceDir, Dimension: DimAuthor})},
					}}},
				{Dimension: DimCollection, From: FromConstant, Constant: "集",
					When: &CondExpr{Kind: ExprAnd, Subs: []*CondExpr{
						&CondExpr{Kind: ExprRange, Operand: &CondExpr{Kind: ExprGroupSize}, Min: num(2)},
						&CondExpr{Kind: ExprNotEqual,
							Left:  &CondExpr{Kind: ExprNumber, Num: 0},
							Right: &CondExpr{Kind: ExprDepth}},
					}}},
			},
			Synthesis: SynthesisPolicy{},
		}},
	}
	data1, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("首次序列化失败: %v", err)
	}
	parsed, errs := ParseProfile(data1)
	if parsed == nil || len(errs) != 0 {
		t.Fatalf("往返载入失败: %v %v", parsed, errs)
	}
	if len(parsed.Rules) != 1 || len(parsed.Rules[0].Extractions) != 2 ||
		parsed.Rules[0].Extractions[0].When == nil || parsed.Rules[0].Extractions[1].When == nil {
		t.Fatalf("往返后条件树应保结构: %+v", parsed.Rules)
	}
	data2, err := json.Marshal(*parsed)
	if err != nil {
		t.Fatalf("二次序列化失败: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Fatalf("往返两次 JSON 字节不等价:\n首次: %s\n二次: %s", data1, data2)
	}
}
