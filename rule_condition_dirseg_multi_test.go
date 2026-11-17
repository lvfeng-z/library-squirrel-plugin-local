package main

// 多段目录模板 · 阶段1 内核用例（目录层级表达与规则引用重定位方案·技术要点一；
// 决策1 分隔符形态 / 决策2 跨段位次 / 决策6 dirLevel 退役断言）：词法切分
//（首尾分隔符与空段拒绝、层距值域）、三段固定层距命中/不命中、任意层距多链
// 取最深（含固定×任意混合的回溯链）、跨段同词位次寻址（提取引用第 2 处）、
// file/seq/contains 含 `/` 拒绝（自决3）、落地段 DirHit（自决6）。
import (
	"strings"
	"testing"
)

// TestDirSegSplitSegments 段界词法（决策1）：`/`=紧邻、`//`=任意层距、
// `/{N}`=显式层距 N≥1；首尾分隔符、空段（连斜杠/相邻分隔符）、层距 0 与
// 越界记号拒绝；单段（无分隔符）切为单段、零段距。跨段出现计数（决策2）
// 在推导层连续编号（tag → tag__2）。
func TestDirSegSplitSegments(t *testing.T) {
	type want struct {
		segs []string
		gaps []dirSegGap
	}
	cases := []struct {
		tpl  string
		want want
	}{
		{tpl: "甲壳/{集名}", want: want{segs: []string{"甲壳", "{集名}"}, gaps: []dirSegGap{{N: 1}}}},
		{tpl: "甲壳//{集名}", want: want{segs: []string{"甲壳", "{集名}"}, gaps: []dirSegGap{{Any: true}}}},
		{tpl: "Artist[{作者}]/{2}{集名}", want: want{segs: []string{"Artist[{作者}]", "{集名}"}, gaps: []dirSegGap{{N: 2}}}},
		{tpl: "甲/{2}乙/{3}丙", want: want{segs: []string{"甲", "乙", "丙"}, gaps: []dirSegGap{{N: 2}, {N: 3}}}},
		// 任意与固定段距混排（`//` 之后接 `/{N}` 的形态不作层距——见 bad 清单）。
		{tpl: "甲//乙/{2}丙", want: want{segs: []string{"甲", "乙", "丙"}, gaps: []dirSegGap{{Any: true}, {N: 2}}}},
		// 单段：无分隔符 → 单段切片、零段距（现行单段路径零变化的前提）。
		{tpl: "甲[{标签}]", want: want{segs: []string{"甲[{标签}]"}, gaps: nil}},
	}
	for _, tc := range cases {
		segs, gaps, err := splitDirTplSegments(tc.tpl)
		if err != nil {
			t.Fatalf("%s: 切分失败: %v", tc.tpl, err)
		}
		if len(segs) != len(tc.want.segs) {
			t.Fatalf("%s: 段数 = %d（%v），期望 %d", tc.tpl, len(segs), segs, len(tc.want.segs))
		}
		for i := range segs {
			if segs[i] != tc.want.segs[i] {
				t.Fatalf("%s: 第 %d 段 = %q，期望 %q", tc.tpl, i+1, segs[i], tc.want.segs[i])
			}
		}
		if len(gaps) != len(tc.want.gaps) {
			t.Fatalf("%s: 段距数 = %d，期望 %d", tc.tpl, len(gaps), len(tc.want.gaps))
		}
		for i := range gaps {
			if gaps[i].Any != tc.want.gaps[i].Any || (!tc.want.gaps[i].Any && gaps[i].N != tc.want.gaps[i].N) {
				t.Fatalf("%s: 第 %d 段距 = %+v，期望 %+v", tc.tpl, i+1, gaps[i], tc.want.gaps[i])
			}
		}
	}
	// 跨段出现计数（决策2）：整条模板一次出现序流，同词跨段连续编号。
	segs, _, err := deriveDirTplSegments("Artist[{标签}]//{标签}")
	if err != nil {
		t.Fatalf("跨段推导失败: %v", err)
	}
	if got := segs[0].captures; len(got) != 1 || got[0] != "tag" {
		t.Fatalf("首段捕获应为裸名 tag: %v", got)
	}
	if got := segs[1].captures; len(got) != 1 || got[0] != "tag__2" {
		t.Fatalf("次段同词捕获应为位次名 tag__2: %v", got)
	}

	bad := []struct{ tpl, wantErr string }{
		{"/甲", "不得以分隔符开头"},
		{"//甲", "不得以分隔符开头"},
		{"甲/", "不得以分隔符结尾"},
		{"甲//", "不得以分隔符结尾"},
		{"甲///乙", "空段"},
		{"甲//{2}乙", "不在词表"}, // //之后接 /{N} 记号：不作层距、回到段内词法由词表校验拦下
		{"甲/{2}/{3}乙", "空段"},
		{"甲/{0}乙", "层距"},
		{"甲/{00}乙", "层距"},
		{"甲/{99999999999999999999}乙", "层距"},
	}
	for _, tc := range bad {
		_, _, err := deriveDirTplSegments(tc.tpl)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s: 期望错误含 %q，得到 %v", tc.tpl, tc.wantErr, err)
		}
	}
}

// TestDirSubjectMultiSegFixedDistances 三段固定层距（`甲壳/{2}{集名}/尾页`：
// 壳段隔 2 级、集名段紧邻尾段）命中/不命中与落地段 DirHit：段链按 c−Σ(后方
// 段距) 唯一确定；层距不满足（首段越浅、字面不符）整行不中；壳段可处任意
// 绝对深度（相对层距表达——dirLevel 绝对锚退役后的分工）。
func TestDirSubjectMultiSegFixedDistances(t *testing.T) {
	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetDir,
		Format: condxTpl("甲壳/{2}{集名}/尾页"),
	}}}
	p, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译三段模板失败: %v", err)
	}
	eval := func(dirNames []string) *ParseContext {
		t.Helper()
		ctx := &ParseContext{}
		p.Evaluate(&MatchInput{FileName: "x.png", DirNames: dirNames}, nil, ctx)
		return ctx
	}

	// 命中：甲壳(0) —隔2→ 集名A(2) —紧邻→ 尾页(3)；捕获绑落地链各段、
	// DirHit 恰一条记落地段层。
	ctx := eval([]string{"甲壳", "中转", "集名A", "尾页"})
	if v, ok := ctx.Capture("r1.setname"); !ok || v != "集名A" {
		t.Fatalf("中段捕获应绑第 3 层目录名: %q ok=%v", v, ok)
	}
	if len(ctx.DirHits) != 1 || ctx.DirHits[0].Depth != 3 || ctx.DirHits[0].Name != "尾页" {
		t.Fatalf("DirHit 应恰一条记落地段层（第 4 层/尾页）: %+v", ctx.DirHits)
	}

	// 壳段不在绝对第 1 级也能命中（相对层距，非绝对锚）。
	ctx = eval([]string{"库", "甲壳", "中转", "集名B", "尾页"})
	if v, _ := ctx.Capture("r1.setname"); v != "集名B" {
		t.Fatalf("壳段下沉后应照常命中并绑集名B: %q", v)
	}
	if len(ctx.DirHits) != 1 || ctx.DirHits[0].Depth != 4 {
		t.Fatalf("下沉链的 DirHit 应记落地段层（第 5 层）: %+v", ctx.DirHits)
	}

	// 不命中：隔 2 级处无壳段（甲壳只隔 1 级）。
	if ctx := eval([]string{"甲壳", "集名A", "尾页"}); len(ctx.DirHits) != 0 {
		t.Fatalf("层距不符（首段仅隔 1 级）不应命中: %+v", ctx.DirHits)
	}
	// 不命中：固定层深处字面不符。
	if ctx := eval([]string{"乙壳", "中转", "集名A", "尾页"}); len(ctx.DirHits) != 0 {
		t.Fatalf("壳段字面不符不应命中: %+v", ctx.DirHits)
	}
	// 不命中：落地段字面不符（无更浅候选可回退）。
	if ctx := eval([]string{"甲壳", "中转", "集名A", "封面"}); len(ctx.DirHits) != 0 {
		t.Fatalf("落地段字面不符不应命中: %+v", ctx.DirHits)
	}
}

// TestDirSubjectMultiSegAnyGapDeepestChain 任意层距（`//`）多链取最深：
// 落地层自深向浅先试（最深候选优先），上段在可行链内取最深匹配层；固定×
// 任意混合段距需要回溯——贪心最深放段可能令上方固定段距无解，须回退浅层
// 再试（存在即成立）。
func TestDirSubjectMultiSegAnyGapDeepestChain(t *testing.T) {
	// 两段全捕获：`{标签}//{集名}`——多链并存（锚A@0 / 锚B@2 均可作上段，
	// 集X@1 / 集Y@3 均可作落地）时取最深：落地=集Y、上段=锚B。
	rows := []CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("{标签}//{集名}")}}}
	p, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译两段任意层距模板失败: %v", err)
	}
	ctx := &ParseContext{}
	in := &MatchInput{FileName: "x.png", DirNames: []string{"锚A", "集X", "锚B", "集Y"}}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatal("任意层距两段链应命中")
	}
	if v, _ := ctx.Capture("r1.setname"); v != "集Y" {
		t.Fatalf("落地段应取最深候选 集Y: %q", v)
	}
	if v, _ := ctx.Capture("r1.tag"); v != "锚B" {
		t.Fatalf("上段应取可行链内最深匹配 锚B（非 锚A）: %q", v)
	}
	if len(ctx.DirHits) != 1 || ctx.DirHits[0].Depth != 3 {
		t.Fatalf("DirHit 应记最深落地层: %+v", ctx.DirHits)
	}

	// 固定×任意混合回溯：`甲/{2}{集名}//丙`——丙(4) 之下 {集名} 最深可放
	// 「丙集」(3)，但甲 隔 2 级只到「隔层」(1)（不匹配）；须回退 {集名} 到
	// (2) 使甲 落 (0)——链 [0,2,4] 才成立（贪心不回退即漏判）。
	rows2 := []CondxRow{{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("甲/{2}{集名}//丙")}}}
	p2, err := CompileRuleConditions(rows2)
	if err != nil {
		t.Fatalf("编译混合段距模板失败: %v", err)
	}
	ctx2 := &ParseContext{}
	in2 := &MatchInput{FileName: "x.png", DirNames: []string{"甲", "隔层", "乙集", "丙集", "丙"}}
	if !p2.Evaluate(in2, nil, ctx2) {
		t.Fatal("混合段距的可行链须经回溯找到（存在即成立）")
	}
	if v, _ := ctx2.Capture("r1.setname"); v != "乙集" {
		t.Fatalf("回溯链的中段应取 乙集(2)（丙集(3) 令固定段距无解）: %q", v)
	}
	// 对照：甲 不在场时固定段距无解——任意段虽可放，整行不中。
	ctx3 := &ParseContext{}
	if p2.Evaluate(&MatchInput{FileName: "x.png", DirNames: []string{"别", "隔层", "乙集", "丙集", "丙"}}, nil, ctx3) {
		t.Fatal("固定段距无解时应整行不中")
	}
}

// TestDirSubjectMultiSegCrossSegOccurrence 跨段同词位次寻址（决策2）：同一
// 占位词跨段出现按整条模板出现序连续编号，提取动作以位次名（tag__2）引用
// 第 2 处——声明集（declaredConditionCaptures）按整条模板口径收入两个位次，
// 提取段取第 2 段的命中值。
func TestDirSubjectMultiSegCrossSegOccurrence(t *testing.T) {
	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetDir,
		Format: condxTpl("A[{标签}]/{标签}集"),
	}}}
	declared := declaredConditionCaptures(rows)
	for _, name := range []string{"r1.tag", "r1.tag__2"} {
		if !declared[name] {
			t.Fatalf("声明集应收跨段两个位次（含 %s）: %v", name, declared)
		}
	}
	p, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译跨段同词模板失败: %v", err)
	}
	rule := RuleEntry{
		ID: "cross-occur", Description: "测试：跨段位次寻址",
		Source: RuleSourceManual, Enabled: true, Conditions: rows, Pattern: p,
		Extractions: []ExtractionAction{{Dimension: DimTag, From: FromCapture, CaptureName: "tag__2", Row: 1}},
	}
	in := &MatchInput{FileName: "x.png", DirNames: []string{"A[甲]", "乙集"}, RelPath: "A[甲]/乙集/x.png"}
	// 直接求值断言双位次绑定：位 1=首段（裸名 tag）、位 2=次段（tag__2）。
	dctx := &ParseContext{}
	if !p.Evaluate(in, nil, dctx) {
		t.Fatal("跨段同词模板应命中")
	}
	if v, _ := dctx.Capture("r1.tag"); v != "甲" {
		t.Fatalf("位 1 应绑首段值 甲: %q", v)
	}
	if v, _ := dctx.Capture("r1.tag__2"); v != "乙" {
		t.Fatalf("位 2 应绑次段值 乙: %q", v)
	}
	// 提取段走规则私有作用域，引用位次名取第 2 处。
	decision := &FileDecision{RelPath: in.RelPath, FileName: in.FileName}
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	runExtractionStage(decision, []RuleEntry{rule}, in, nil, ctx)
	found := ""
	for i := range decision.Declarations {
		if decision.Declarations[i].RuleID == "cross-occur" && decision.Declarations[i].Dimension == DimTag {
			found = decision.Declarations[i].Value
		}
	}
	if found != "乙" {
		t.Fatalf("提取引用第 2 处应取次段位值 乙: %q decls=%+v", found, decision.Declarations)
	}
}

// TestDirSubjectMultiSegValidationRejects 校验期拒绝面（自决3 与边界条件）：
// file/seq 模板与 contains 问法含 `/` 保存期拒绝；目录模板首尾分隔符、空段、
// 层距 0 拒绝；段内词法错误照常穿透（多段不放宽单段校验）。
func TestDirSubjectMultiSegValidationRejects(t *testing.T) {
	dir := func(tpl string) *Condx {
		return &Condx{Target: CondxTargetDir, Format: condxTpl(tpl)}
	}
	cases := []struct {
		name    string
		c       *Condx
		wantErr string
	}{
		{name: "文件模板含斜杠", c: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}/{序号}")}, wantErr: "不可含「/」"},
		{name: "序列模板含斜杠", c: &Condx{Target: CondxTargetSeq, Format: condxTpl("{键}/{序号}")}, wantErr: "不可含「/」"},
		{name: "文件包含词含斜杠", c: &Condx{Target: CondxTargetFile, Format: CondxFormat{Mode: CondxFormatContains, Word: "a/b"}}, wantErr: "不可含「/」"},
		{name: "目录包含词含斜杠", c: &Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatContains, Word: "a/b"}}, wantErr: "不可含「/」"},
		{name: "首段前分隔符", c: dir("/甲壳"), wantErr: "不得以分隔符开头"},
		{name: "末段后分隔符", c: dir("甲壳/"), wantErr: "不得以分隔符结尾"},
		{name: "末段后双斜杠", c: dir("甲壳//"), wantErr: "不得以分隔符结尾"},
		{name: "连斜杠空段", c: dir("甲壳///集名"), wantErr: "空段"},
		{name: "相邻层距记号空段", c: dir("甲壳/{2}/{3}集名"), wantErr: "空段"},
		{name: "层距零不可书写", c: dir("甲壳/{0}集名"), wantErr: "层距"},
		{name: "段内词表错误照常穿透", c: dir("甲壳/{无此词}"), wantErr: "不在词表"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCondx(tc.c)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
			}
		})
	}
	// 编译入口同口径拒绝（行编译先走校验）。
	if _, err := CompileRuleConditions([]CondxRow{{Condition: &Condx{
		Target: CondxTargetFile, Format: condxTpl("{标题}/{序号}"),
	}}}); err == nil || !strings.Contains(err.Error(), "不可含「/」") {
		t.Fatalf("编译应拒绝含斜杠的文件模板: %v", err)
	}
}

// TestDirLevelRetiredAssertions dirLevel（「位于第 N 级」绝对层级锚）退役
// 断言（决策6 改判·内核面）：白名单与必填字段族无该词条；携带该词条的条件
// 行校验拒绝（载入即跳过的先决条件，跳过用例见 rule_refs_test.go
// TestDirLevelAttachEntrySkippedOnLoad）；第 1 级互斥校验随
// 词条删除——父目录谓词组合不再有锚定互斥分支。
func TestDirLevelRetiredAssertions(t *testing.T) {
	if attachAllowed[CondxTargetDir]["dirLevel"] {
		t.Fatal("dir 目标附加条件白名单不应再含 dirLevel 词条")
	}
	if attachAllowed[CondxTargetFile]["dirLevel"] || attachAllowed[CondxTargetSeq]["dirLevel"] {
		t.Fatal("file/seq 目标附加条件白名单不应含 dirLevel 词条")
	}
	if attachNeedsCount["dirLevel"] {
		t.Fatal("附加条件必填字段族不应再登记 dirLevel")
	}
	// 携带该词条：校验拒绝（封闭集拦截）。
	err := validateCondx(&Condx{Target: CondxTargetDir, Format: condxTpl("{标签}"),
		Attach: []CondxAttach{{Kind: "dirLevel", MinCount: 2}}})
	if err == nil || !strings.Contains(err.Error(), "不适用于目标") {
		t.Fatalf("携带 dirLevel 的目录行应被封闭集校验拒绝: %v", err)
	}
	// 行编译入口同口径拒绝。
	if _, cerr := CompileRuleConditions([]CondxRow{{Condition: &Condx{
		Target: CondxTargetDir, Format: condxTpl("{标签}"),
		Attach: []CondxAttach{{Kind: "dirLevel", MinCount: 1}},
	}}}); cerr == nil || !strings.Contains(cerr.Error(), "不适用于目标") {
		t.Fatalf("行编译应拒绝携带 dirLevel 的条件行: %v", cerr)
	}
	// 第 1 级互斥校验不存在：父目录两谓词同挂照常合法（互斥分支的唯一触发
	// 形态携带 dirLevel，已在上游封闭集拦截，不存在残余互斥校验面）。
	if verr := validateCondx(&Condx{Target: CondxTargetDir, Format: condxTpl("[{标签}]"),
		Attach: []CondxAttach{
			{Kind: CondxAttachParentFormat, Tpl: "monster girl"},
			{Kind: CondxAttachParentContains, Word: "pixiv"},
		}}); verr != nil {
		t.Fatalf("父目录谓词组合不应再触发互斥校验: %v", verr)
	}
}
