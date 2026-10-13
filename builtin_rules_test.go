// 内置惯例规则表单元测试：pixiv 形态与归属解析的求值等价、分隔惯例真值样例、
// 规则表构造健全与档案往返。
package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// builtinRuleByID 从内置规则表中按 ID 取规则；ID 不存在即判定测试失败。
func builtinRuleByID(t *testing.T, id string) RuleEntry {
	t.Helper()
	for _, rule := range BuiltinRules() {
		if rule.ID == id {
			return rule
		}
	}
	t.Fatalf("内置规则表缺少 ID %q", id)
	return RuleEntry{}
}

// TestBuiltinPixivRuleEquivalence pixiv 形态迁移等价：逐例断言内置规则对文件名求值
// 命中与否、捕获值与归属解析入口 parsePixivPageWorkID(stripExt(...)) 三者一致。
// 样例取自归属解析既有用例（去扩展名形态，与原函数调用形态相同），并补齐带扩展名、
// 带前后缀与近似形态的边界。
func TestBuiltinPixivRuleEquivalence(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
		wantHit  bool
		wantID   string
	}{
		// 既有命中样例：{作品号}[-{十六进制补丁}]_p{页号}
		{"页号个位数", "12345678_p0", true, "12345678_p0"},
		{"页号两位数", "12345678_p12", true, "12345678_p12"},
		{"带短十六进制补丁", "12345678-abcdef_p0", true, "12345678-abcdef_p0"},
		{"带长十六进制补丁", "12345678-0123456789abcdef_p3", true, "12345678-0123456789abcdef_p3"},
		// 既有未命中样例
		{"空文件名", "", false, ""},
		{"无数字作品号", "myart_p0", false, ""},
		{"无页号段", "12345678", false, ""},
		{"master 规格名", "12345678_p0_master1200", false, ""},
		{"带前缀非全匹配", "img_12345678_p0", false, ""},
		{"十六进制补丁大写", "12345678-ABCDEF_p0", false, ""},
		{"页号非数字", "12345678_pabc", false, ""},
		{"页号缺失", "12345678_p", false, ""},
		// 自补边界：真实文件名的含扩展名形态
		{"含扩展名 jpg", "12345678_p0.jpg", true, "12345678_p0"},
		{"含扩展名 png 且带补丁", "12345678-abcdef_p0.png", true, "12345678-abcdef_p0"},
		{"含扩展名 webp 且页号两位", "12345678_p00.webp", true, "12345678_p00"},
		{"末尾点号空扩展名", "12345678_p0.", true, "12345678_p0"},
		{"带扩展名的 master 规格名", "12345678_p0_master1200.jpg", false, ""},
		{"带前缀且含扩展名", "pre_12345678_p0.jpg", false, ""},
		{"原图名后接备份扩展名", "12345678_p0.jpg.bak", false, ""},
		{"双段扩展名", "12345678_p0.tar.gz", false, ""},
		{"尾随空白", "12345678_p0 ", false, ""},
		{"前置空白", " 12345678_p0", false, ""},
	}

	rule := builtinRuleByID(t, builtinRuleIDPixivPageWorkID)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &ParseContext{}
			hit := rule.Pattern.Evaluate(&MatchInput{FileName: tc.fileName}, nil, ctx)
			if hit != tc.wantHit {
				t.Fatalf("内置 pixiv 规则对文件名 %q 求值 = %v，期望 %v", tc.fileName, hit, tc.wantHit)
			}
			got, _ := ctx.Capture(builtinPageWorkIDCapture)
			if got != tc.wantID {
				t.Errorf("捕获 %s = %q，期望 %q", builtinPageWorkIDCapture, got, tc.wantID)
			}

			// 与原函数逐例等价：命中与否一致，且捕获值即原函数在去扩展名形态上的返回值。
			stem := stripExt(tc.fileName)
			legacy := parsePixivPageWorkID(stem)
			if (legacy != "") != tc.wantHit {
				t.Errorf("parsePixivPageWorkID(%q) = %q，与规则命中 %v 不一致", stem, legacy, tc.wantHit)
			}
			if legacy != got {
				t.Errorf("parsePixivPageWorkID(%q) = %q，与捕获值 %q 不一致", stem, legacy, got)
			}
		})
	}
}

// TestBuiltinSeparatorRules 分隔惯例规则真值样例：真例命中并解析出作者与标题
// （末段扩展名不计入标题、不产生空标题），假例不命中；命中时提取动作产出作者与
// 标题两条文件名来源的声明，未命中时不产出声明。
func TestBuiltinSeparatorRules(t *testing.T) {
	cases := []struct {
		name       string
		ruleID     string
		fileName   string
		wantHit    bool
		wantAuthor string
		wantTitle  string
	}{
		// 「【作者】标题」
		{"方括号形态", builtinRuleIDBracketAuthorTitle, "【画师甲】夏日海岸.jpg", true, "画师甲", "夏日海岸"},
		{"方括号形态无扩展名", builtinRuleIDBracketAuthorTitle, "【画师甲】夏日海岸", true, "画师甲", "夏日海岸"},
		{"方括号形态标题含点号", builtinRuleIDBracketAuthorTitle, "【画师甲】夏日.海岸.v2.png", true, "画师甲", "夏日.海岸.v2"},
		{"方括号形态标题带末尾点号", builtinRuleIDBracketAuthorTitle, "【画师甲】夏日海岸.", true, "画师甲", "夏日海岸"},
		{"方括号形态标题含方括号", builtinRuleIDBracketAuthorTitle, "【画师甲】【限定】夏日海岸.jpg", true, "画师甲", "【限定】夏日海岸"},
		{"方括号形态作者空", builtinRuleIDBracketAuthorTitle, "【】夏日海岸.jpg", false, "", ""},
		{"方括号形态标题空", builtinRuleIDBracketAuthorTitle, "【画师甲】.jpg", false, "", ""},
		{"方括号形态仅剩扩展名", builtinRuleIDBracketAuthorTitle, "【画师甲】.png", false, "", ""},
		{"方括号形态未闭合", builtinRuleIDBracketAuthorTitle, "【画师甲夏日海岸.jpg", false, "", ""},
		{"方括号形态不在开头", builtinRuleIDBracketAuthorTitle, "画师甲【夏日海岸】.jpg", false, "", ""},
		// 「作者 - 标题」
		{"连字符形态", builtinRuleIDDashAuthorTitle, "画师甲 - 夏日海岸.jpg", true, "画师甲", "夏日海岸"},
		{"连字符形态多词作者", builtinRuleIDDashAuthorTitle, "Studio A - Summer Coast.png", true, "Studio A", "Summer Coast"},
		{"连字符形态标题再含连字符", builtinRuleIDDashAuthorTitle, "画师甲 - 夏日 - 海岸.jpg", true, "画师甲", "夏日 - 海岸"},
		{"连字符形态无扩展名", builtinRuleIDDashAuthorTitle, "画师甲 - 夏日海岸", true, "画师甲", "夏日海岸"},
		{"连字符形态标题带版本点号", builtinRuleIDDashAuthorTitle, "画师甲 - 夏日.v2.jpg", true, "画师甲", "夏日.v2"},
		{"连字符形态分隔符无空白", builtinRuleIDDashAuthorTitle, "画师甲-夏日海岸.jpg", false, "", ""},
		{"连字符形态作者带括号", builtinRuleIDDashAuthorTitle, "(画师甲) - 夏日海岸.jpg", false, "", ""},
		{"连字符形态作者带竖线", builtinRuleIDDashAuthorTitle, "画师甲|画师乙 - 夏日海岸.jpg", false, "", ""},
		{"连字符形态作者带斜杠", builtinRuleIDDashAuthorTitle, "画师甲/画师乙 - 夏日海岸.jpg", false, "", ""},
		{"连字符形态作者空", builtinRuleIDDashAuthorTitle, " - 夏日海岸.jpg", false, "", ""},
		{"连字符形态标题空", builtinRuleIDDashAuthorTitle, "画师甲 - .jpg", false, "", ""},
		{"连字符形态无分隔符", builtinRuleIDDashAuthorTitle, "画师甲夏日海岸.jpg", false, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := builtinRuleByID(t, tc.ruleID)
			ctx := &ParseContext{}
			hit := rule.Pattern.Evaluate(&MatchInput{FileName: tc.fileName}, nil, ctx)
			if hit != tc.wantHit {
				t.Fatalf("规则 %q 对文件名 %q 求值 = %v，期望 %v", tc.ruleID, tc.fileName, hit, tc.wantHit)
			}
			gotAuthor, _ := ctx.Capture(builtinAuthorCapture)
			gotTitle, _ := ctx.Capture(builtinTitleCapture)
			if gotAuthor != tc.wantAuthor || gotTitle != tc.wantTitle {
				t.Errorf("捕获 (author, title) = (%q, %q)，期望 (%q, %q)",
					gotAuthor, gotTitle, tc.wantAuthor, tc.wantTitle)
			}

			var decls []MetadataDeclaration
			for _, action := range rule.Extractions {
				if decl, ok := action.Apply(rule.ID, ctx); ok {
					decls = append(decls, decl)
				}
			}
			if !tc.wantHit {
				if len(decls) != 0 {
					t.Fatalf("未命中时不应产出声明，实得 %+v", decls)
				}
				return
			}
			want := []MetadataDeclaration{
				{Dimension: DimAuthor, Value: tc.wantAuthor, Source: SourceFilename, RuleID: tc.ruleID},
				{Dimension: DimTitle, Value: tc.wantTitle, Source: SourceFilename, RuleID: tc.ruleID},
			}
			if !reflect.DeepEqual(decls, want) {
				t.Errorf("提取声明 = %+v，期望 %+v", decls, want)
			}
		})
	}
}

// TestBuiltinRulesSanity 内置规则表健全性：声明表全部条目构造无错、条数与声明表一致、
// ID 非空且唯一、来源统一为内置、全部启用、描述非空、模式为文件名正则且声明了捕获组。
func TestBuiltinRulesSanity(t *testing.T) {
	if _, err := buildBuiltinRules(); err != nil {
		t.Fatalf("内置规则声明表构造失败: %v", err)
	}
	rules := BuiltinRules()
	specs := builtinRuleSpecs()
	if len(rules) != len(specs) {
		t.Fatalf("内置规则条数 = %d，期望 %d", len(rules), len(specs))
	}

	seen := make(map[string]struct{}, len(rules))
	for i, rule := range rules {
		if rule.ID == "" {
			t.Errorf("规则 %d 的 ID 为空", i)
			continue
		}
		if _, dup := seen[rule.ID]; dup {
			t.Errorf("规则 ID 重复: %q", rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if rule.Source != RuleSourceBuiltin {
			t.Errorf("规则 %q 的来源 = %q，期望 %q", rule.ID, rule.Source, RuleSourceBuiltin)
		}
		if !rule.Enabled {
			t.Errorf("规则 %q 应为启用状态", rule.ID)
		}
		if rule.Description == "" {
			t.Errorf("规则 %q 缺少人类可读描述", rule.ID)
		}
		pattern, ok := rule.Pattern.(*RegexPattern)
		if !ok {
			t.Fatalf("规则 %q 的模式类型 = %T，期望 *RegexPattern", rule.ID, rule.Pattern)
		}
		if pattern.Target != TargetFileName {
			t.Errorf("规则 %q 的匹配目标 = %q，期望 %q", rule.ID, pattern.Target, TargetFileName)
		}
		if pattern.re == nil {
			t.Errorf("规则 %q 的正则未经编译", rule.ID)
			continue
		}
		// 声明的捕获组名必须真实存在于表达式内，否则求值只会绑定空串。
		names := pattern.re.SubexpNames()
		for _, capture := range pattern.Captures {
			found := false
			for i, groupName := range names {
				if i > 0 && groupName == capture {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("规则 %q 声明的捕获组 %q 不在表达式 %q 内", rule.ID, capture, pattern.Expr)
			}
		}
	}
}

// TestBuiltinRulesProfileRoundTrip 内置规则表经规则档案 JSON 往返不丢规则：
// 载入无错误、条数与次序保持、ID 与来源 / 启用 / 描述 / 提取动作逐项一致，
// 二次序列化字节等价。
func TestBuiltinRulesProfileRoundTrip(t *testing.T) {
	profile := RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: BuiltinRules()}
	if len(profile.Rules) == 0 {
		t.Fatal("内置规则表为空")
	}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("内置规则档案序列化失败: %v", err)
	}

	parsed, errs := ParseProfile(data)
	if len(errs) != 0 {
		t.Fatalf("内置规则档案载入报错: %v", errs)
	}
	if parsed == nil {
		t.Fatal("内置规则档案载入返回 nil")
	}
	if len(parsed.Rules) != len(profile.Rules) {
		t.Fatalf("往返后规则数 = %d，期望 %d", len(parsed.Rules), len(profile.Rules))
	}
	for i := range parsed.Rules {
		got, want := parsed.Rules[i], profile.Rules[i]
		if got.ID != want.ID {
			t.Errorf("规则 %d 的 ID 往返后变化: got %q, want %q", i, got.ID, want.ID)
		}
		if got.Source != want.Source || got.Enabled != want.Enabled || got.Description != want.Description {
			t.Errorf("规则 %q 的元信息往返后变化: got (%q, %v, %q), want (%q, %v, %q)",
				want.ID, got.Source, got.Enabled, got.Description, want.Source, want.Enabled, want.Description)
		}
		if !reflect.DeepEqual(got.Extractions, want.Extractions) {
			t.Errorf("规则 %q 的提取动作往返后变化: got %+v, want %+v", want.ID, got.Extractions, want.Extractions)
		}
	}

	data2, err := json.Marshal(*parsed)
	if err != nil {
		t.Fatalf("往返后重新序列化失败: %v", err)
	}
	if !bytes.Equal(data, data2) {
		t.Errorf("往返两次 JSON 字节不等价:\n首次: %s\n二次: %s", data, data2)
	}
}
