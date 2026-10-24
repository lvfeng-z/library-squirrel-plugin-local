// 例子模板表单元测试：词条健全（可编译、目标合法、字段非空）。
package main

import (
	"strings"
	"testing"
)

// TestExampleTemplatesSanity 例子模板表健全性：词条字段非空、模板词全在
// v4 封闭词表内、每个词条按其主体类别可编译为单行条件规则（写法真实可用，
// 而非展示性摆设）。
func TestExampleTemplatesSanity(t *testing.T) {
	templates := exampleTemplates()
	if len(templates) == 0 {
		t.Fatal("例子模板表为空")
	}
	targets := map[string]bool{string(TargetFileName): true, string(TargetDirName): true, "seq": true}
	seen := make(map[string]bool, len(templates))
	for _, tpl := range templates {
		if tpl.Name == "" || tpl.Template == "" || tpl.Description == "" {
			t.Fatalf("词条字段缺失: %+v", tpl)
		}
		if seen[tpl.Name] {
			t.Errorf("词条短名重复: %q", tpl.Name)
		}
		seen[tpl.Name] = true
		if !targets[tpl.Target] {
			t.Errorf("词条 %q 的主体类别 %q 非法", tpl.Name, tpl.Target)
		}
		target := CondxTargetDir
		if tpl.Target == string(TargetFileName) {
			target = CondxTargetFile
		} else if tpl.Target == "seq" {
			target = CondxTargetSeq
		}
		rows := []CondxRow{{Condition: &Condx{
			Target: target,
			Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl.Template},
		}}}
		if _, err := CompileRuleConditions(rows); err != nil {
			t.Errorf("词条 %q 的模板 %q 无法编译: %v", tpl.Name, tpl.Template, err)
		}
		for word := range condxVocab {
			if strings.Contains(tpl.Template, word) && !strings.Contains(tpl.Template, "{"+word+"}") {
				t.Errorf("词条 %q 的模板 %q 内词 %q 未按 {%s} 占位写法出现", tpl.Name, tpl.Template, word, word)
			}
		}
	}
}
