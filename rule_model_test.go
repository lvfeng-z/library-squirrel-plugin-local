// 规则模型单元测试：四类原子模式求值真值样例、And/Or/Not 组合真值表、
// 档案多态 JSON 序列化往返、非法正则（反向引用 / lookahead）双入口拦截。
package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// mustRegex 构造必然合法的正则模式，构造失败即判定测试失败。
func mustRegex(t *testing.T, target, expr string, captures []string) *RegexPattern {
	t.Helper()
	p, err := NewRegexPattern(target, expr, captures)
	if err != nil {
		t.Fatalf("NewRegexPattern(%q, %q) 意外失败: %v", target, expr, err)
	}
	return p
}

// mustLocation 构造必然合法的位置模式，构造失败即判定测试失败。
func mustLocation(t *testing.T, minDepth, maxDepth int, ancestor Pattern) *LocationPattern {
	t.Helper()
	p, err := NewLocationPattern(minDepth, maxDepth, ancestor)
	if err != nil {
		t.Fatalf("NewLocationPattern(%d, %d) 意外失败: %v", minDepth, maxDepth, err)
	}
	return p
}

// mustSibling 构造必然合法的兄弟群组模式，构造失败即判定测试失败。
func mustSibling(t *testing.T, keyExpr, seqExpr string, minGroupSize int, outcome GroupOutcome) *SiblingPattern {
	t.Helper()
	p, err := NewSiblingPattern(keyExpr, seqExpr, minGroupSize, outcome)
	if err != nil {
		t.Fatalf("NewSiblingPattern(%q, %q) 意外失败: %v", keyExpr, seqExpr, err)
	}
	return p
}

// TestRegexPatternEvaluate 覆盖正则模式三类目标的真/假样例：
// filename 命中时命名捕获组绑定进 ctx；dirname 任一层命中追加 DirHit。
func TestRegexPatternEvaluate(t *testing.T) {
	cases := []struct {
		name string
		// 构造参数：目标类别 + 表达式 + 需绑定的捕获组名。
		target   string
		expr     string
		captures []string
		in       MatchInput
		want     bool
		// wantCaptures 为求值后期望的捕获绑定（按追加顺序）；未命中时为 nil。
		wantCaptures []CaptureBinding
		// wantDirHits 为求值后期望的目录层命中记录；未命中时为 nil。
		wantDirHits []DirHit
	}{
		{
			name:         "filename命中并绑定命名捕获组",
			target:       string(TargetFileName),
			expr:         `^(?P<author>[a-z]+)_(\d+)\.jpg$`,
			captures:     []string{"author"},
			in:           MatchInput{FileName: "alice_001.jpg"},
			want:         true,
			wantCaptures: []CaptureBinding{{Name: "author", Value: "alice"}},
		},
		{
			name:     "filename未命中不追加绑定",
			target:   string(TargetFileName),
			expr:     `^(?P<author>[a-z]+)_(\d+)\.jpg$`,
			captures: []string{"author"},
			in:       MatchInput{FileName: "bob.png"},
			want:     false,
		},
		{
			name:         "filename命中但组名不存在时绑定空串",
			target:       string(TargetFileName),
			expr:         `^img$`,
			captures:     []string{"missing"},
			in:           MatchInput{FileName: "img"},
			want:         true,
			wantCaptures: []CaptureBinding{{Name: "missing", Value: ""}},
		},
		{
			name: "dirname任一层命中追加DirHit",
			target:   string(TargetDirName),
			expr:     `^作者-(.+)$`,
			in:       MatchInput{DirNames: []string{"库根", "作者-张三", "作品"}},
			want:     true,
			wantDirHits: []DirHit{{Depth: 1, Name: "作者-张三"}},
		},
		{
			name:   "dirname全部层未命中",
			target: string(TargetDirName),
			expr:   `^作者-(.+)$`,
			in:     MatchInput{DirNames: []string{"库根", "photos"}},
			want:   false,
		},
		{
			name:   "relpath命中",
			target: string(TargetRelPath),
			expr:   `^library/`,
			in:     MatchInput{RelPath: "library/a.jpg"},
			want:   true,
		},
		{
			name:   "relpath未命中",
			target: string(TargetRelPath),
			expr:   `^library/`,
			in:     MatchInput{RelPath: "other/a.jpg"},
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewRegexPattern(tc.target, tc.expr, tc.captures)
			if err != nil {
				t.Fatalf("NewRegexPattern 构造失败: %v", err)
			}
			ctx := &ParseContext{}
			got := p.Evaluate(&tc.in, nil, ctx)
			if got != tc.want {
				t.Fatalf("Evaluate = %v, 期望 %v", got, tc.want)
			}
			if !reflect.DeepEqual(ctx.Captures, tc.wantCaptures) {
				t.Errorf("Captures = %+v, 期望 %+v", ctx.Captures, tc.wantCaptures)
			}
			if !reflect.DeepEqual(ctx.DirHits, tc.wantDirHits) {
				t.Errorf("DirHits = %+v, 期望 %+v", ctx.DirHits, tc.wantDirHits)
			}
			// 命中目录层时顺带校验 LastDirHit 取到最深（此处唯一）一条命中。
			if tc.want && len(tc.wantDirHits) > 0 {
				hit, ok := ctx.LastDirHit()
				want := tc.wantDirHits[len(tc.wantDirHits)-1]
				if !ok || hit != want {
					t.Errorf("LastDirHit = (%+v, %v), 期望 (%+v, true)", hit, ok, want)
				}
			}
		})
	}
}

// TestLocationPatternEvaluate 覆盖位置模式深度区间的含/不含命中层两种结局；
// 并校验 Ancestor 的子绑定不泄漏进主 ctx（只追加 DirHit）。
func TestLocationPatternEvaluate(t *testing.T) {
	// ancestor 带 named 捕获组，用于验证子绑定不进主上下文。
	ancestor := mustRegex(t, string(TargetFileName), `^(?P<kw>漫画)$`, []string{"kw"})
	cases := []struct {
		name             string
		minDepth, maxDepth int
		ancestor         Pattern
		in               MatchInput
		want             bool
		wantDirHits      []DirHit
	}{
		{
			name:       "命中层在深度区间内",
			minDepth:   0,
			maxDepth:   2,
			ancestor:   ancestor,
			in:         MatchInput{DirNames: []string{"库根", "漫画", "第01卷"}},
			want:       true,
			wantDirHits: []DirHit{{Depth: 1, Name: "漫画"}},
		},
		{
			name:       "命中层在深度区间外",
			minDepth:   2,
			maxDepth:   -1, // 不限上界，但起点已越过命中层
			ancestor:   ancestor,
			in:         MatchInput{DirNames: []string{"库根", "漫画", "第01卷"}},
			want:       false,
		},
		{
			name:       "区间内无命中层",
			minDepth:   1,
			maxDepth:   1,
			ancestor:   ancestor,
			in:         MatchInput{DirNames: []string{"库根", "杂项"}},
			want:       false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewLocationPattern(tc.minDepth, tc.maxDepth, tc.ancestor)
			if err != nil {
				t.Fatalf("NewLocationPattern 构造失败: %v", err)
			}
			ctx := &ParseContext{}
			got := p.Evaluate(&tc.in, nil, ctx)
			if got != tc.want {
				t.Fatalf("Evaluate = %v, 期望 %v", got, tc.want)
			}
			if !reflect.DeepEqual(ctx.DirHits, tc.wantDirHits) {
				t.Errorf("DirHits = %+v, 期望 %+v", ctx.DirHits, tc.wantDirHits)
			}
			// LocationPattern 只向主 ctx 追加 DirHit，Ancestor 的子绑定不应泄漏。
			if len(ctx.Captures) != 0 {
				t.Errorf("子模式捕获绑定泄漏进主 ctx: %+v", ctx.Captures)
			}
		})
	}
}

// TestSiblingPatternEvaluate 覆盖兄弟群组模式：三文件序号群达成 MinGroupSize
// 时产出 GroupAnalysis（Key/Seq/Size），落单或序号不可提取时为 false。
func TestSiblingPatternEvaluate(t *testing.T) {
	const (
		keyExpr = `^(.+)_\d+\.jpg$`
		seqExpr = `_(\d+)\.jpg$`
	)
	cases := []struct {
		name         string
		keyExpr      string
		seqExpr      string
		minGroupSize int
		fileName     string
		siblings     []string
		want         bool
		wantGroup    *GroupAnalysis
	}{
		{
			name:         "三文件序号群达成最小规模",
			keyExpr:      keyExpr,
			seqExpr:      seqExpr,
			minGroupSize: 3,
			fileName:     "album_02.jpg",
			siblings:     []string{"album_01.jpg", "album_02.jpg", "album_03.jpg"},
			want:         true,
			wantGroup:    &GroupAnalysis{Key: "album", Prefix: "album", Seq: 2, Size: 3},
		},
		{
			name:         "落单不足最小规模",
			keyExpr:      keyExpr,
			seqExpr:      seqExpr,
			minGroupSize: 3,
			fileName:     "album_09.jpg",
			siblings:     []string{"album_09.jpg", "note.txt"},
			want:         false,
			wantGroup:    nil,
		},
		{
			name:         "自身序号不可提取直接落单",
			keyExpr:      keyExpr,
			seqExpr:      seqExpr,
			minGroupSize: 2,
			fileName:     "album_cover.jpg",
			siblings:     []string{"album_01.jpg", "album_cover.jpg", "album_02.jpg"},
			want:         false,
			wantGroup:    nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewSiblingPattern(tc.keyExpr, tc.seqExpr, tc.minGroupSize, OutcomeCollection)
			if err != nil {
				t.Fatalf("NewSiblingPattern 构造失败: %v", err)
			}
			ctx := &ParseContext{}
			in := MatchInput{FileName: tc.fileName, Siblings: tc.siblings}
			got := p.Evaluate(&in, nil, ctx)
			if got != tc.want {
				t.Fatalf("Evaluate = %v, 期望 %v", got, tc.want)
			}
			if !reflect.DeepEqual(ctx.Group, tc.wantGroup) {
				t.Errorf("Group = %+v, 期望 %+v", ctx.Group, tc.wantGroup)
			}
		})
	}
}

// TestPredicatePatternEvaluate 覆盖谓词模式：文件名（去扩展名）或指定捕获组
// 的值命中/未命中库内作者、标签或作品集集合。
func TestPredicatePatternEvaluate(t *testing.T) {
	lib := &LibraryIndex{
		AuthorNames:  map[string]struct{}{"宫崎骏": {}},
		TagNames:     map[string]struct{}{"治愈": {}},
		WorkSetNames: map[string]struct{}{"画集总集": {}},
	}
	cases := []struct {
		name string
		kind PredicateKind
		from string
		// preCaptures 为预置到 ctx 的上游捕获绑定，模拟先行的正则命中。
		preCaptures []CaptureBinding
		fileName    string
		want        bool
		wantHits    []PredicateHit
	}{
		{
			name:     "文件名命中库内作者",
			kind:     PredicateAuthor,
			fileName: "宫崎骏.jpg",
			want:     true,
			wantHits: []PredicateHit{{Kind: "author", Input: "宫崎骏", Name: "宫崎骏"}},
		},
		{
			name:     "文件名未命中库内作者",
			kind:     PredicateAuthor,
			fileName: "无名氏.jpg",
			want:     false,
		},
		{
			name:         "从捕获组取值命中库内标签",
			kind:         PredicateTag,
			from:         "tag",
			preCaptures:  []CaptureBinding{{Name: "tag", Value: "治愈"}},
			fileName:     "any.png",
			want:         true,
			wantHits:     []PredicateHit{{Kind: "tag", Input: "治愈", Name: "治愈"}},
		},
		{
			name:     "捕获组绑定缺失不命中",
			kind:     PredicateTag,
			from:     "tag",
			fileName: "any.png",
			want:     false,
		},
		{
			name:     "文件名命中库内作品集",
			kind:     PredicateWorkSet,
			fileName: "画集总集.jpg",
			want:     true,
			wantHits: []PredicateHit{{Kind: "workSet", Input: "画集总集", Name: "画集总集"}},
		},
		{
			name:     "文件名未命中库内作品集",
			kind:     PredicateWorkSet,
			fileName: "无此集.jpg",
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &PredicatePattern{Kind: tc.kind, From: tc.from}
			ctx := &ParseContext{Captures: tc.preCaptures}
			in := MatchInput{FileName: tc.fileName}
			got := p.Evaluate(&in, lib, ctx)
			if got != tc.want {
				t.Fatalf("Evaluate = %v, 期望 %v", got, tc.want)
			}
			if !reflect.DeepEqual(ctx.PredHits, tc.wantHits) {
				t.Errorf("PredHits = %+v, 期望 %+v", ctx.PredHits, tc.wantHits)
			}
		})
	}
}

// TestCompositePatternEvaluate 以真值表样例覆盖 And/Or/Not 的组合求值，
// 并用绑定副作用观察 And/Or 的短路求值顺序。
func TestCompositePatternEvaluate(t *testing.T) {
	// 两个基础谓词：A 命中 alpha 前缀，B 命中 jpg 扩展名。
	a := mustRegex(t, string(TargetFileName), `^alpha_`, nil)
	b := mustRegex(t, string(TargetFileName), `\.jpg$`, nil)
	cases := []struct {
		name     string
		pattern  Pattern
		fileName string
		want     bool
	}{
		{"And两真为真", And{a, b}, "alpha_01.jpg", true},
		{"And含假为假", And{a, b}, "alpha_01.png", false},
		{"Or含真为真", Or{a, b}, "beta_02.jpg", true},
		{"Or两假为假", Or{a, b}, "beta_02.png", false},
		{"Not内层真取反为假", Not{Inner: a}, "alpha_01.png", false},
		{"Not内层假取反为真", Not{Inner: a}, "beta_02.png", true},
		{"And空子模式恒真", And{}, "任意.jpg", true},
		{"Or空子模式恒假", Or{}, "任意.jpg", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &ParseContext{}
			got := tc.pattern.Evaluate(&MatchInput{FileName: tc.fileName}, nil, ctx)
			if got != tc.want {
				t.Fatalf("Evaluate(%q) = %v, 期望 %v", tc.fileName, got, tc.want)
			}
		})
	}

	// 短路观察一：And 首个子模式未命中时，后续子模式的绑定不追加。
	t.Run("And短路不追加后续绑定", func(t *testing.T) {
		miss := mustRegex(t, string(TargetFileName), `^never_`, []string{"x"})
		hitLater := mustRegex(t, string(TargetFileName), `^alpha_`, []string{"y"})
		ctx := &ParseContext{}
		if (And{miss, hitLater}).Evaluate(&MatchInput{FileName: "alpha_01.jpg"}, nil, ctx) {
			t.Fatal("And 含未命中子模式，期望 false")
		}
		if len(ctx.Captures) != 0 {
			t.Errorf("And 短路后不应追加绑定，得到 %+v", ctx.Captures)
		}
	})

	// 短路观察二：Or 首个子模式命中后，后续子模式不再求值、不追加绑定。
	t.Run("Or短路只保留首个命中绑定", func(t *testing.T) {
		first := mustRegex(t, string(TargetFileName), `^(?P<x>alpha)_`, []string{"x"})
		second := mustRegex(t, string(TargetFileName), `\.jpg$`, []string{"y"})
		ctx := &ParseContext{}
		if !(Or{first, second}).Evaluate(&MatchInput{FileName: "alpha_01.jpg"}, nil, ctx) {
			t.Fatal("Or 首个子模式命中，期望 true")
		}
		want := []CaptureBinding{{Name: "x", Value: "alpha"}}
		if !reflect.DeepEqual(ctx.Captures, want) {
			t.Errorf("Captures = %+v, 期望 %+v", ctx.Captures, want)
		}
	})
}

// TestRuleProfileJSONRoundTrip 构造含全部 pattern kind 的嵌套档案，
// MarshalJSON → ParseProfile → 再 MarshalJSON，断言两次 JSON 字节等价、
// 规则数一致且规则 ID 保序。
func TestRuleProfileJSONRoundTrip(t *testing.T) {
	profile := RuleProfile{
		SchemaVersion: CurrentSchemaVersion,
		Rules: []RuleEntry{
			{
				ID:          "rule-comic",
				Description: "目录与文件名联合识别漫画",
				Source:      RuleSourceManual,
				Enabled:     true,
				// 覆盖 and / regex / location / not / or / predicate 六种 kind 的嵌套。
				Pattern: And{
					mustRegex(t, string(TargetFileName), `^(?P<series>.+?)_\d+\.jpg$`, []string{"series"}),
					mustLocation(t, 1, 2, mustRegex(t, string(TargetFileName), `^(?P<kw>漫画)$`, []string{"kw"})),
					Not{Inner: Or{
						&PredicatePattern{Kind: PredicateTag, From: "series"},
						mustRegex(t, string(TargetRelPath), `^private/`, nil),
					}},
				},
				Extractions: []ExtractionAction{
					{Dimension: DimTitle, From: FromCapture, CaptureName: "series"},
					{Dimension: DimCollection, From: FromAncestorDir},
					{Dimension: DimAuthor, From: FromConstant, Constant: "佚名", RoleName: "fallback"},
				},
				Synthesis: SynthesisPolicy{
					DimPolicies: []DimPolicy{
						{Dimension: DimTag, Strategy: StrategyUnion},
						{Dimension: DimTitle, Strategy: StrategyHighTrust},
					},
					Roles: []RoleBinding{{Source: SourceDir, RoleName: "curated"}},
				},
			},
			{
				ID:          "rule-author-dir",
				Description: "目录名作者前缀",
				Source:      RuleSourceBuiltin,
				Enabled:     true,
				Pattern:     mustRegex(t, string(TargetDirName), `^作者-(.+)$`, nil),
			},
			{
				ID:          "rule-album",
				Description: "兄弟序号群归集（停用状态）",
				Source:      RuleSourceQna,
				Enabled:     false,
				Pattern:     mustSibling(t, `^(.+?)_\d+\.jpg$`, `_(\d+)\.jpg$`, 2, OutcomeIndependent),
			},
		},
	}

	data1, err := json.Marshal(profile)
	if err != nil {
		t.Fatalf("首次 MarshalJSON 失败: %v", err)
	}
	parsed, errs := ParseProfile(data1)
	if len(errs) != 0 {
		t.Fatalf("ParseProfile 意外报错: %v", errs)
	}
	if parsed == nil {
		t.Fatal("ParseProfile 返回了 nil 档案")
	}
	if got, want := len(parsed.Rules), len(profile.Rules); got != want {
		t.Fatalf("往返后规则数不一致: got %d, want %d", got, want)
	}
	for i := range parsed.Rules {
		if parsed.Rules[i].ID != profile.Rules[i].ID {
			t.Errorf("规则 %d 的 ID 往返后变化: got %q, want %q", i, parsed.Rules[i].ID, profile.Rules[i].ID)
		}
	}

	data2, err := json.Marshal(*parsed)
	if err != nil {
		t.Fatalf("二次 MarshalJSON 失败: %v", err)
	}
	if !bytes.Equal(data1, data2) {
		t.Errorf("往返两次 JSON 字节不等价:\n首次: %s\n二次: %s", data1, data2)
	}
}

// TestInvalidRegexBlocked 验证反向引用与 lookahead 两类 RE2 不支持形态
// 在 NewRegexPattern 与 ParseProfile 双入口被拦截，且错误信息含表达式原文
// （以 %q 引用形态出现）与规则 ID；非法规则被跳过时不阻塞合法规则载入。
func TestInvalidRegexBlocked(t *testing.T) {
	badExprs := []string{`(a)\1`, `a(?=b)`}

	t.Run("NewRegexPattern入口", func(t *testing.T) {
		for _, expr := range badExprs {
			_, err := NewRegexPattern(string(TargetFileName), expr, nil)
			if err == nil {
				t.Errorf("表达式 %q 应被 NewRegexPattern 拦截，却构造成功", expr)
				continue
			}
			// 错误信息以 %q 引用形态携带表达式原文，故用 strconv.Quote 匹配。
			if !strings.Contains(err.Error(), strconv.Quote(expr)) {
				t.Errorf("错误信息 %q 应含表达式原文 %q", err.Error(), strconv.Quote(expr))
			}
		}
	})

	t.Run("ParseProfile入口", func(t *testing.T) {
		for _, expr := range badExprs {
			// 直接以结构体字面量携带非法表达式：序列化只搬运原文，编译拦截留给载入阶段。
			bad := RuleProfile{
				SchemaVersion: CurrentSchemaVersion,
				Rules: []RuleEntry{
					{
						ID:          "rule-bad-regex",
						Description: "含非法正则的规则",
						Source:      RuleSourceQna,
						Enabled:     true,
						Pattern:     &RegexPattern{Target: TargetFileName, Expr: expr},
					},
					{
						ID:          "rule-good",
						Description: "合法对照规则",
						Source:      RuleSourceBuiltin,
						Enabled:     true,
						Pattern:     &RegexPattern{Target: TargetDirName, Expr: `^作者-(.+)$`},
					},
				},
			}
			data, err := json.Marshal(bad)
			if err != nil {
				t.Fatalf("构造测试档案失败: %v", err)
			}
			got, errs := ParseProfile(data)
			if len(errs) == 0 {
				t.Errorf("表达式 %q 应触发 ParseProfile 错误", expr)
				continue
			}
			msg := errs[0].Error()
			if !strings.Contains(msg, "rule-bad-regex") {
				t.Errorf("错误信息 %q 应含规则 ID rule-bad-regex", msg)
			}
			if !strings.Contains(msg, strconv.Quote(expr)) {
				t.Errorf("错误信息 %q 应含表达式原文 %q", msg, strconv.Quote(expr))
			}
			// 非法规则被跳过，合法规则不受阻塞。
			if got == nil || len(got.Rules) != 1 || got.Rules[0].ID != "rule-good" {
				t.Errorf("非法规则应被跳过且不影响合法规则，实际档案: %+v", got)
			}
		}
	})
}
