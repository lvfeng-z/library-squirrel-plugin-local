// v4 规则构造基础件测试：条件编译入口、提取动作行引用解析（含库名单规范名
// 回落）、条件声明捕获名收集（行号前缀形态）。
package main

import (
	"reflect"
	"strings"
	"testing"
)

// TestCompileRuleConditions 规则级条件编译入口 = 行列表编译（T1 单行冒烟）。
func TestCompileRuleConditions(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}][{作品号}][{标题}]"),
			Attach: []CondxAttach{{Kind: CondxAttachChildrenAllFormat, Tpl: "{序号}", MinCount: 2}}}},
	}
	p, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	row, ok := p.(*RowSubjectPattern)
	if !ok || row.Ordinal != 1 {
		t.Fatalf("应编译为行包装（行1），得到 %T", p)
	}
}

// TestRuleConditionsRoundTrip v4 条目档案序列化往返：conditions 落 JSON 后
// 载入还原（条件行等价、模式树为编译缓存、提取行引用校验过）；同时锚定
// 行引用指向未声明行时报错。
func TestRuleConditionsRoundTrip(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("【{作者}】{标题}"),
			Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".jpg"}}}},
	}
	pat, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	rule := RuleEntry{
		ID: "rt-1", Description: "往返", Source: RuleSourceManual, Enabled: true,
		Conditions: rows, Pattern: pat,
		Extractions: []ExtractionAction{
			{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1},
			{Dimension: DimTitle, From: FromCapture, CaptureName: "title", Row: 1},
		},
	}
	rj, err := ruleToJSON(rule)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if rj.Pattern != nil || rj.Conditions == nil {
		t.Fatalf("v4 条目应只落 conditions（模式树为缓存不落档）: %+v", rj)
	}
	back, err := ruleFromJSON(rj)
	if err != nil {
		t.Fatalf("载入失败: %v", err)
	}
	if !reflect.DeepEqual(back.Conditions, rows) {
		t.Fatalf("条件行往返不等:\n取回 %+v\n期望 %+v", back.Conditions, rows)
	}
	if back.Pattern == nil {
		t.Fatal("载入应重建编译缓存")
	}
	ctx := &ParseContext{}
	if !back.Pattern.Evaluate(&MatchInput{FileName: "【甲】乙.jpg"}, nil, ctx) {
		t.Fatalf("往返后求值应命中")
	}
	if v, _ := ctx.Capture("r1.author"); v != "甲" {
		t.Fatalf("往返后捕获不符: %q", v)
	}
	// 行引用指向未声明行：拒绝。
	bad := *rj
	bad.ID = "rt-2"
	bad.Extractions = []ExtractionAction{{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 2}}
	if _, err := ruleFromJSON(&bad); err == nil || !strings.Contains(err.Error(), "未声明的条件捕获") {
		t.Fatalf("未声明行引用应拒绝: %v", err)
	}
}

// TestExtractionRowReference 提取动作行引用：Row>0 解析 rN.组名；库名单小标
// 命中且原始绑定缺席时回落 #canon 规范名。
func TestExtractionRowReference(t *testing.T) {
	// 行1 目录条件绑定 r1.author；提取 {Row:1, CaptureName:"author"} 取前缀绑定。
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]")}},
	}
	p, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	ctx := &ParseContext{DirNames: []string{"Art", "Artist[Isel]"}}
	if !p.Evaluate(&MatchInput{FileName: "01.jpg", DirNames: ctx.DirNames}, nil, ctx) {
		t.Fatalf("目录行应命中")
	}
	action := ExtractionAction{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1}
	decl, ok := action.Apply("rule-x", ctx)
	if !ok || decl.Value != "Isel" {
		t.Fatalf("行引用取值不符: %+v %v", decl, ok)
	}
	// #canon 回落：库内规范名 Jake_Jaccobus，行捕获原始形变值缺席时取规范名。
	// 2026-11-04 终审拍板（惰性化）：{作者}_{标题} 的作者段取最短（首 _ 前），
	// 载体名由双下划线形变换为空格形变（jake jaccobus 仍折叠命中 Jake_Jaccobus）。
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{"Jake_Jaccobus": {}}}
	rows2 := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}"),
			Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true}}}},
	}
	p2, err := CompileRuleConditions(rows2)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	ctx2 := &ParseContext{}
	if !p2.Evaluate(&MatchInput{FileName: "jake jaccobus_sunset.jpg"}, lib, ctx2) {
		t.Fatalf("折叠唯一应命中")
	}
	action2 := ExtractionAction{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 1}
	decl2, ok := action2.Apply("rule-y", ctx2)
	if !ok || decl2.Value != "Jake_Jaccobus" {
		t.Fatalf("规范名回落取值不符: %+v %v", decl2, ok)
	}
	// 行号越界（行2 未绑定）不产出。
	action3 := ExtractionAction{Dimension: DimAuthor, From: FromCapture, CaptureName: "author", Row: 2}
	if _, ok := action3.Apply("rule-z", ctx2); ok {
		t.Fatalf("未绑定行不应产出")
	}
}

// TestDeclaredConditionCaptures 条件捕获名收集：行号前缀 + 子组接续编号 +
// 库名单小标的 #canon。
func TestDeclaredConditionCaptures(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}_{序号}"),
			Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true}}}},
		{Connector: "and", Group: []CondxRow{
			{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]")}},
			{Connector: "or", Condition: &Condx{Target: CondxTargetSeq, Format: condxTpl("{键}_{序号}")}},
		}},
	}
	got := declaredConditionCaptures(rows)
	want := map[string]bool{
		"r1.author": true, "r1.title": true, "r1.seq": true, "r1.author#canon": true,
		"r2.author": true,
		"r3.key":    true, "r3.seq": true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("捕获名收集不符:\n得到 %v\n期望 %v", got, want)
	}
}

// TestBadgeRepeatWordForbidden 值校验三断言（库名单/相等/常量）引用重复段
// 占位词一律拒绝：模板 {标签+;_} 的整段是多值产出，小标的单一值断言无定义；
// 错误文案与生效条件/组合接线两面同形（关键词「重复段占位词」「多值产出」）。
func TestBadgeRepeatWordForbidden(t *testing.T) {
	badges := map[string]CondxBadge{
		"library":  {Placeholder: "标签", Kind: CondxBadgeLibrary, Dim: "tag"},
		"equal":    {Placeholder: "标签", Kind: CondxBadgeEqual, AgainstRow: 1, AgainstPh: "标题"},
		"constant": {Placeholder: "标签", Kind: CondxBadgeConstant, Value: "x"},
	}
	for name, badge := range badges {
		t.Run(name, func(t *testing.T) {
			rows := []CondxRow{
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}_{标签+;_}"),
					Badges: []CondxBadge{badge}}},
			}
			_, err := CompileRuleConditions(rows)
			if err == nil {
				t.Fatalf("重复段占位词上的值校验应拒绝")
			}
			for _, want := range []string{"重复段占位词", "多值产出"} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("错误应含 %q，实际: %v", want, err)
				}
			}
		})
	}
	// 普通词回归：{标签}（无量词）上的库名单小标照常可编译。
	ok := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}_{标签}"),
			Badges: []CondxBadge{{Placeholder: "标签", Kind: CondxBadgeLibrary, Dim: "tag"}}}},
	}
	if _, err := CompileRuleConditions(ok); err != nil {
		t.Fatalf("普通词上的值校验应可编译: %v", err)
	}
}

// TestValidateDeclaredConditionCaptureSets 声明集合双口径：提取口径并入重复段词
// （阶段1 位置组设计使 En 不进捕获组名清单，此处声明层显式补入）、条件口径
// 剔除重复段词、重复段词打标集单列；提取段引用重复段词照常放行（绿）。
func TestValidateDeclaredConditionCaptureSets(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}_{标签+;_}")}},
	}
	all := declaredConditionCaptures(rows)
	cond := declaredConditionOnlyCaptures(rows)
	repeat := declaredConditionRepeatCaptures(rows)
	if !all["r1.tag"] || !all["r1.title"] {
		t.Fatalf("提取口径应同时含普通词与重复段词: %v", all)
	}
	if cond["r1.tag"] || !cond["r1.title"] {
		t.Fatalf("条件口径应剔除重复段词: %v", cond)
	}
	if !repeat["r1.tag"] || repeat["r1.title"] {
		t.Fatalf("重复段词打标集不符: %v", repeat)
	}
	// 提取段引用重复段词：FromCapture 引用 r1.tag 的规则构造校验照常放行。
	pat, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("重复段词条件行应可编译: %v", err)
	}
	rule := RuleEntry{
		ID: "repeat-extract", Description: "提取引用重复段词", Source: RuleSourceManual, Enabled: true,
		Conditions: rows, Pattern: pat,
		Extractions: []ExtractionAction{{Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1}},
	}
	rj, err := ruleToJSON(rule)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if _, err := ruleFromJSON(rj); err != nil {
		t.Fatalf("提取动作引用重复段词应放行: %v", err)
	}
}
