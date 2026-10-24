// v4 规则构造基础件测试：条件编译入口、提取动作行引用解析（含库名单规范名
// 回落）、条件声明捕获名收集（行号前缀形态）。
package main

import (
	"reflect"
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
	if !p2.Evaluate(&MatchInput{FileName: "jake__jaccobus_sunset.jpg"}, lib, ctx2) {
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
