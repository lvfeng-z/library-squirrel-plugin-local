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
			if !strings.Contains(tpl.Template, word) {
				continue
			}
			// 占位写法核验：剔除全部占位词记号后的字面残余不得再含词根
			//（出现于占位记号内即占位写法；{作品} 与 {作品号} 一类词根互为
			// 前缀，记号内的出现不算字面误写）。
			tokens, err := tokenizeCondx(tpl.Template)
			if err != nil {
				t.Fatalf("词条 %q 的模板 %q 词法失败: %v", tpl.Name, tpl.Template, err)
			}
			var literal strings.Builder
			for _, tok := range tokens {
				literal.WriteString(tok.literal)
			}
			if strings.Contains(literal.String(), word) {
				t.Errorf("词条 %q 的模板 %q 内词 %q 未按 {%s} 占位写法出现", tpl.Name, tpl.Template, word, word)
			}
		}
	}
	// 「首组内名」词条在场：模板=字面方括号 + 标签词 + 尾位吞位，目标=目录名，
	// 说明含组界统一后的取值口径（多组名取首组内名）。
	var firstInner *exampleTemplate
	for i := range templates {
		if templates[i].Template == "[{标签}]{任意}" {
			firstInner = &templates[i]
			break
		}
	}
	if firstInner == nil {
		t.Fatal("例子模板表应含首组内名词条（模板 [{标签}]{任意}）")
	}
	if firstInner.Target != string(TargetDirName) {
		t.Errorf("首组内名词条主体应为目录名，实际 %q", firstInner.Target)
	}
	for _, piece := range []string{"首组内名（[{标签}]{任意}）", "尾段"} {
		if !strings.Contains(firstInner.Description, piece) {
			t.Errorf("首组内名词条说明缺文案锚 %q: %q", piece, firstInner.Description)
		}
	}
	// 「多处标签」词条在场：同占位词多次出现（位次分立）演示——位1 圆括号组 +
	// 位2 尾部重复段组，说明注记「标签·第2处」=尾部标签组。
	var multiTag *exampleTemplate
	for i := range templates {
		if templates[i].Template == "({标签})[{作者}]{集名}{ ?}[{标签+}]" {
			multiTag = &templates[i]
			break
		}
	}
	if multiTag == nil {
		t.Fatal("例子模板表应含多处标签词条（模板 ({标签})[{作者}]{集名}{ ?}[{标签+}]）")
	}
	if multiTag.Target != string(TargetFileName) {
		t.Errorf("多处标签词条主体应为文件名，实际 %q", multiTag.Target)
	}
	for _, piece := range []string{"位次分立", "标签·第2处"} {
		if !strings.Contains(multiTag.Description, piece) {
			t.Errorf("多处标签词条说明缺文案锚 %q: %q", piece, multiTag.Description)
		}
	}
}
