package main

// 规则总结器的单元测试：真实样例的组合规则构造（根名→标签 / 一级目录剥壳
// →作者 / 文件名首段→作品名 / 尾部码丢弃）、多段目录模板合并产物（决策3：
// 逐层目录条目合并单条多段规则、段距=层距差、零绝对层级锚）、手填值自动匹配
// 兜底与手工维度降级、剥壳变换的求值、全库命中预览统计与跨目录样例（含目录
// 键）、赋值出处存取（会话态+协议透传）、summarizeRule 不写档与 generalize
// mode=struct 确认入档（单文件与目录寻址）。
import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// realSampleStructFixture 剧本G 真实样例的三件输入。
func realSampleStructFixture() (root, rel, file string) {
	return "E:/libs/big belly",
		"Artist[Jake--Jaccobus]/atago__commission__by_jake__jaccobus_delithw.png",
		"atago__commission__by_jake__jaccobus_delithw.png"
}

// TestBuildStructMapRulesRealSample 退出标准样例（决策3 形态）：根名→标签
//（常量折入目录层级条目）、一级目录壳形态 → 目录行模板 Artist[{作者}]（剥壳
// 语义由捕获天然承载、不附绝对层级锚）、文件名全段展开模板（首段 {标题}、
// 未引用段/尾部 {任意} 吞位——尾部码丢弃语义）。
func TestBuildStructMapRulesRealSample(t *testing.T) {
	root, rel, file := realSampleStructFixture()
	assign := &SessionAssignment{
		Author: "Jake--Jaccobus", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
		Tags: []string{"big belly"}, TagClueRefs: []string{"rootName"},
	}
	sugg, rules, err := buildStructMapRules(root, rel, file, assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 2 || len(sugg.Rules) != 2 {
		t.Fatalf("组合规则应恰两条（目录层级+文件名），实际 %d 条", len(rules))
	}

	// 目录层级条目（单层样本=单段模板）：Artist[{作者}]、零附加（无绝对
	// 层级锚）、作者=行 1 捕获、标签=库根名常量折入。
	if _, ok := rules[0].Pattern.(*RowSubjectPattern); !ok || rules[0].Conditions == nil {
		t.Fatalf("首条应为目录行条件规则，实际 %T", rules[0].Pattern)
	}
	dirCond := rules[0].Conditions[0].Condition
	if dirCond.Target != CondxTargetDir || dirCond.Format.Tpl != "Artist[{作者}]" {
		t.Fatalf("目录层模板不符: %+v", dirCond)
	}
	if len(dirCond.Attach) != 0 {
		t.Fatalf("目录行不应附任何层级锚（dirLevel 已废弃）: %+v", dirCond.Attach)
	}
	if len(rules[0].Extractions) != 2 {
		t.Fatalf("目录条目提取应两维（作者+标签），实际 %d 条", len(rules[0].Extractions))
	}
	auth := rules[0].Extractions[0]
	if auth.Dimension != DimAuthor || auth.From != FromCapture ||
		auth.CaptureName != condxVocab["作者"] || auth.Row != 1 {
		t.Fatalf("作者提取应为目录行捕获: %+v", auth)
	}
	tagAct := rules[0].Extractions[1]
	if tagAct.Dimension != DimTag || tagAct.From != FromConstant || tagAct.Constant != "big belly" {
		t.Fatalf("标签提取应为库根名常量: %+v", tagAct)
	}

	// 文件名条目（v4 文件行模板）：首段 {标题}、未引用段与尾部 {任意} 吞位。
	if _, ok := rules[1].Pattern.(*RowSubjectPattern); !ok || rules[1].Conditions == nil {
		t.Fatalf("次条应为 v4 文件行条件规则，实际 %T", rules[1].Pattern)
	}
	fileCond := rules[1].Conditions[0].Condition
	wantTpl := "{标题}__{任意}__{任意}_{任意}_{任意}"
	if fileCond.Target != CondxTargetFile || fileCond.Format.Tpl != wantTpl {
		t.Fatalf("文件名模板不符:\n取回 %s\n期望 %s", fileCond.Format.Tpl, wantTpl)
	}
	if len(rules[1].Extractions) != 1 || rules[1].Extractions[0].Dimension != DimTitle ||
		rules[1].Extractions[0].From != FromCapture || rules[1].Extractions[0].CaptureName != condxVocab["标题"] ||
		rules[1].Extractions[0].Row != 1 {
		t.Fatalf("作品名提取应为首段捕获: %+v", rules[1].Extractions)
	}

	// 人话规则与映射（决策7 多段句式——单段样本同款「目录层级」句式）。
	for _, piece := range []string{
		"目录层级 Artist[{作者}]（自上而下逐级紧邻）", "作者=壳内名",
		"文件名首段→作品名", "库根名→标签", "尾部码丢弃",
	} {
		if !strings.Contains(sugg.Summary, piece) {
			t.Fatalf("人话规则缺片段 %q: %q", piece, sugg.Summary)
		}
	}
	if !sugg.DropsTailCode || len(sugg.ManualDims) != 0 {
		t.Fatalf("尾部码标记/手工维度不符: %+v", sugg)
	}
	if len(sugg.Mappings) != 3 {
		t.Fatalf("映射说明应三条，实际 %v", sugg.Mappings)
	}
	if sugg.Mappings[0].From != "目录段壳内名" {
		t.Fatalf("目录段来源标签应为壳内名: %v", sugg.Mappings)
	}
}

// TestBuildStructMapRulesMultiLevelMerge 多级目录条目合并（决策3 主例）：作者=
// 一级目录壳内名、集名=二级目录整名 → 单条多段模板规则 Artist[{作者}]/{集名}
//（段距紧邻缺省 /），两槽提取各引本行捕获（跨段捕获 Row 指向该行），零绝对
// 层级锚；跳层（一、三级取值）合并为显式段距 /{2}；建议卡句式与来源标签随
// 多段形态（决策7）。
func TestBuildStructMapRulesMultiLevelMerge(t *testing.T) {
	t.Run("紧邻两层合并", func(t *testing.T) {
		rel := "Artist[七尾ゆきじ]/酔いがさめない/x.png"
		assign := &SessionAssignment{
			Author:     "七尾ゆきじ", AuthorClueRef: "dirLevelStripped1",
			Collection: "酔いがさめない", CollectionClueRef: "dirLevel2",
		}
		sugg, rules, err := buildStructMapRules("E:/libs/漫画库", rel, "x.png", assign)
		if err != nil {
			t.Fatalf("结构映射规则构造失败: %v", err)
		}
		if len(rules) != 1 || len(sugg.Rules) != 1 {
			t.Fatalf("两级目录条目应合并为单条规则，实际 %d 条", len(rules))
		}
		cond := rules[0].Conditions[0].Condition
		if cond.Target != CondxTargetDir || cond.Format.Tpl != "Artist[{作者}]/{集名}" {
			t.Fatalf("多段模板不符: %+v", cond.Format)
		}
		if len(cond.Attach) != 0 {
			t.Fatalf("多段目录行不应附任何层级锚: %+v", cond.Attach)
		}
		if len(rules[0].Extractions) != 2 {
			t.Fatalf("合并条目应两维提取，实际 %d 条", len(rules[0].Extractions))
		}
		auth, set := rules[0].Extractions[0], rules[0].Extractions[1]
		if auth.Dimension != DimAuthor || auth.From != FromCapture ||
			auth.CaptureName != condxVocab["作者"] || auth.Row != 1 {
			t.Fatalf("作者提取应为第 1 段捕获: %+v", auth)
		}
		if set.Dimension != DimCollection || set.From != FromCapture ||
			set.CaptureName != condxVocab["集名"] || set.Row != 1 {
			t.Fatalf("集名提取应为末段捕获: %+v", set)
		}
		for _, piece := range []string{
			"目录层级 Artist[{作者}]/{集名}（自上而下逐级紧邻）", "作者=壳内名", "集名=末段",
		} {
			if !strings.Contains(sugg.Summary, piece) {
				t.Fatalf("人话规则缺片段 %q: %q", piece, sugg.Summary)
			}
		}
		if len(sugg.Mappings) != 2 ||
			sugg.Mappings[0].From != "目录段壳内名" || sugg.Mappings[1].From != "目录段末段" {
			t.Fatalf("来源标签不符: %v", sugg.Mappings)
		}
		// 自检复现：同链样例输入上两维取值=赋值值。
		in := &MatchInput{FileName: "x.png", DirNames: []string{"Artist[七尾ゆきじ]", "酔いがさめない"}, RelPath: rel}
		ctx := &ParseContext{}
		if !rules[0].Pattern.Evaluate(in, nil, ctx) {
			t.Fatal("样例输入应命中合并规则")
		}
		for i, want := range []string{"七尾ゆきじ", "酔いがさめない"} {
			decl, ok := rules[0].Extractions[i].Apply(rules[0].ID, ctx)
			if !ok || decl.Value != want {
				t.Fatalf("提取 %d 应复现赋值值 %q: %+v", i, want, decl)
			}
		}
	})

	t.Run("跳层显式段距", func(t *testing.T) {
		rel := "Artist[七尾ゆきじ]/中间层/酔いがさめない/x.png"
		assign := &SessionAssignment{
			Author:     "七尾ゆきじ", AuthorClueRef: "dirLevelStripped1",
			Collection: "酔いがさめない", CollectionClueRef: "dirLevel3",
		}
		sugg, rules, err := buildStructMapRules("E:/libs/漫画库", rel, "x.png", assign)
		if err != nil {
			t.Fatalf("结构映射规则构造失败: %v", err)
		}
		if len(rules) != 1 {
			t.Fatalf("两级目录条目应合并为单条规则，实际 %d 条", len(rules))
		}
		cond := rules[0].Conditions[0].Condition
		if cond.Format.Tpl != "Artist[{作者}]/{2}{集名}" {
			t.Fatalf("跳层合并应为显式段距 /{2}: %+v", cond.Format)
		}
		if !strings.Contains(sugg.Summary, "目录层级 Artist[{作者}]（隔2级）/{集名}（自上而下）") {
			t.Fatalf("跳层句式应夹注隔2级: %q", sugg.Summary)
		}
	})
}

// TestBuildStructMapRulesManualFallback 手填兜底：无显式出处时值=某段自动匹配
//（作者=剥壳内名、作品名=首段）；不对应任何段的值降级「手工指定（不参与泛化）」。
func TestBuildStructMapRulesManualFallback(t *testing.T) {
	root, rel, file := realSampleStructFixture()
	assign := &SessionAssignment{
		Author:     "Jake--Jaccobus", // 自动匹配 dirLevelStripped1
		Title:      "atago",          // 自动匹配 filenameSeg0
		Collection: "手填集",          // 无对应段
	}
	sugg, rules, err := buildStructMapRules(root, rel, file, assign)
	if err != nil {
		t.Fatalf("构造失败: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("应构造目录+文件名两条，实际 %d 条", len(rules))
	}
	if len(sugg.ManualDims) != 1 || !strings.Contains(sugg.ManualDims[0], "手填集") ||
		!strings.Contains(sugg.ManualDims[0], "手工指定") {
		t.Fatalf("作品集应降级手工指定: %v", sugg.ManualDims)
	}
	if strings.Contains(sugg.Summary, "作品集") || strings.Contains(sugg.Summary, "手填集") {
		t.Fatalf("手工维度不应入人话规则: %q", sugg.Summary)
	}
	// 目录条目不含作品集常量
	for _, act := range rules[0].Extractions {
		if act.Dimension == DimCollection {
			t.Fatalf("手工维度不应烧入规则: %+v", act)
		}
	}
}

// TestBuildStructMapRulesBracketUnderscore 括号组 + 单 _ 分隔（v4 模板）：目录
// 层壳形态条目与文件名全段展开条目齐备（括号段以开/闭括号字面包裹、未引用段
// {任意} 吞位、引用段占维度词）；同形异值文件上以样例输入直接求值，作者=
// 目录行捕获、作品名=第 3 段捕获均复现。
func TestBuildStructMapRulesBracketUnderscore(t *testing.T) {
	rel := "Artist[路灯后辈]/[路灯后辈]_[136715041]_无题_[01].png"
	file := "[路灯后辈]_[136715041]_无题_[01].png"
	assign := &SessionAssignment{
		Author: "路灯后辈", AuthorClueRef: "dirLevelStripped1",
		Title: "无题", TitleClueRef: "filenameSeg2",
	}
	sugg, rules, err := buildStructMapRules("E:/libs/图库", rel, file, assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 2 || len(sugg.Rules) != 2 {
		t.Fatalf("组合规则应恰两条（目录层级+文件名），实际 %d 条", len(rules))
	}
	dirRule, ok := rules[0].Pattern.(*RowSubjectPattern)
	if !ok || rules[0].Conditions == nil || rules[0].Conditions[0].Condition.Format.Tpl != "Artist[{作者}]" {
		t.Fatalf("首条应为目录行条件规则（壳模板），实际 %T", rules[0].Pattern)
	}
	fileRule, ok := rules[1].Pattern.(*RowSubjectPattern)
	if !ok || rules[1].Conditions == nil {
		t.Fatalf("次条应为 v4 文件行条件规则，实际 %T", rules[1].Pattern)
	}
	// 文件名模板：前两括号段字面包裹 + 第 3 段捕获（可选括号包裹）+ 尾段吞位。
	wantTpl := "[{任意}]_[{任意}]_{标题}_[{任意}]"
	if got := rules[1].Conditions[0].Condition.Format.Tpl; got != wantTpl {
		t.Fatalf("文件名形态模板不符:\n取回 %s\n期望 %s", got, wantTpl)
	}

	// 来源标签（壳内名标签）与人话措辞（首段/第 N 段措辞不变）。
	fromBy := map[string]string{}
	for _, m := range sugg.Mappings {
		fromBy[m.Dimension] = m.From
	}
	if fromBy["author"] != "目录段壳内名" || fromBy["title"] != "文件名第3段" {
		t.Fatalf("来源标签不符: %v", sugg.Mappings)
	}
	for _, piece := range []string{"作者=壳内名", "文件名第3段→作品名"} {
		if !strings.Contains(sugg.Summary, piece) {
			t.Fatalf("人话规则缺片段 %q: %q", piece, sugg.Summary)
		}
	}

	// 同形异值复现：样例输入构造 MatchInput 直接求值（author=目录行捕获、
	// title=文件行捕获）。已知 v4 差异：v3 的括号感知段类（段值排除括号字
	// 符、可选括号包裹剥壳）废止——纯贪婪切分下异值段的括号并入捕获值
	//（如 "[标题]" 整值；样例自身无括号时自检不受影响），差异记录于
	// grammar 差异清单，期3 实测定夺是否收敛。
	sameShape := "[X]_[9527]_[标题]_[02].png"
	in := &MatchInput{
		FileName: sameShape,
		DirNames: []string{"Artist[X]"},
		RelPath:  "Artist[X]/" + sameShape,
	}
	ctxFile := &ParseContext{}
	if !fileRule.Evaluate(in, nil, ctxFile) {
		t.Fatal("同形异值文件应命中文件名条目")
	}
	decl, ok := rules[1].Extractions[0].Apply(rules[1].ID, ctxFile)
	if !ok || decl.Dimension != DimTitle || decl.Value != "[标题]" {
		t.Fatalf("同形异值作品名捕获不符（v4 贪婪切分含壳）: %+v", decl)
	}
	ctxDir := &ParseContext{}
	if !dirRule.Evaluate(in, nil, ctxDir) {
		t.Fatal("同形异值目录应命中目录层条目")
	}
	decl, ok = rules[0].Extractions[0].Apply(rules[0].ID, ctxDir)
	if !ok || decl.Dimension != DimAuthor || decl.Value != "X" {
		t.Fatalf("同形异值作者剥壳取值不符: %+v", decl)
	}
}

// 【已删除·决策3/6】TestStructSummaryDirLevelAnchor（泛化目录条目层深锚用例，
// AE 阶段2 决策2）随 dirLevel 词条退役与泛化器目录层合并删除：泛化器改产
// 多段模板规则（零绝对层级锚），合并产物断言见
// TestBuildStructMapRulesMultiLevelMerge、单段零附锚见
// TestBuildStructMapRulesRealSample。

// TestStructSummaryBareBracketDirAnchor `[…]` 裸括号一级目录赋 标签=剥壳内名
// → 泛化目录条目壳模板=字面 `[`+{标签}+字面 `]`（前缀字面+占位词+后缀字面，7
// 壳形态机制共用），零附加（无层级锚——任意祖先层命中，深层同名目录照常
// 接管=决策6 放宽面）。
func TestStructSummaryBareBracketDirAnchor(t *testing.T) {
	rel := "[路灯后辈]/pic_01.png"
	assign := &SessionAssignment{Tags: []string{"路灯后辈"}, TagClueRefs: []string{"dirLevelStripped1"}}
	sugg, rules, err := buildStructMapRules("G:/inner world/picture", rel, "pic_01.png", assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 1 || len(sugg.Rules) != 1 {
		t.Fatalf("应恰一条目录层条目，实际 %d 条", len(rules))
	}
	dirCond := rules[0].Conditions[0].Condition
	if dirCond.Format.Tpl != "[{标签}]" {
		t.Fatalf("裸括号目录模板应为字面 [ + {标签} + 字面 ]: %+v", dirCond.Format)
	}
	if len(dirCond.Attach) != 0 {
		t.Fatalf("目录行不应附任何层级锚: %+v", dirCond.Attach)
	}
	// 产物 JSON（与手写条件 JSON 片段同源序列化）：无 attach 键。
	data, err := json.Marshal(rules[0].Conditions)
	if err != nil {
		t.Fatalf("序列化条件失败: %v", err)
	}
	if strings.Contains(string(data), "attach") {
		t.Fatalf("目录条目 JSON 不应含 attach: %s", data)
	}
	// 标签提取=目录行 {标签} 位捕获（壳形态语义由捕获天然承载）。
	tagAct := rules[0].Extractions[0]
	if tagAct.Dimension != DimTag || tagAct.From != FromCapture ||
		tagAct.CaptureName != condxVocab["标签"] || tagAct.Row != 1 {
		t.Fatalf("标签提取应为目录行 {标签} 位捕获: %+v", tagAct)
	}
	if piece := "目录层级 [{标签}]（自上而下逐级紧邻）→ 标签=壳内名"; !strings.Contains(sugg.Summary, piece) {
		t.Fatalf("人话规则缺片段 %q: %q", piece, sugg.Summary)
	}
	if len(sugg.Mappings) != 1 || sugg.Mappings[0].From != "目录段壳内名" {
		t.Fatalf("映射来源应为目录段壳内名: %v", sugg.Mappings)
	}

	// 同层异值壳照常命中且捕获泛化；深层同名 `[…]` 无层级锚同样接管（决策6
	// 放宽面——绝对层级锚退役）。
	pattern := rules[0].Pattern
	sameLevel := &MatchInput{
		FileName: "pic_02.png", DirNames: []string{"[X]"}, RelPath: "[X]/pic_02.png",
	}
	ctx := &ParseContext{}
	if !pattern.Evaluate(sameLevel, nil, ctx) {
		t.Fatal("第 1 级同形异值目录应照常命中")
	}
	decl, ok := tagAct.Apply(rules[0].ID, ctx)
	if !ok || decl.Value != "X" {
		t.Fatalf("同形异值标签捕获应为壳内名: %+v", decl)
	}
	deep := &MatchInput{
		FileName: "pic_03.png", DirNames: []string{"别处", "更深", "[路灯后辈]"}, RelPath: "别处/更深/[路灯后辈]/pic_03.png",
	}
	if !pattern.Evaluate(deep, nil, &ParseContext{}) {
		t.Fatal("深层同名 [..] 目录应照常命中（无绝对层级锚）")
	}
}

// TestSummarizeMultiGroupDirFirstGroupTemplate 多组名目录条目泛化收敛：赋
// 标签=首组内名（显式出处 dirLevelStripped1，剥壳变体=首组内名）→ 目录行
// 模板=前缀字面+[{标签}]+首组后间隙字面+{任意}（首组后正文全部由吞位段
// 承接），标签提取=行 1 捕获且自检复现首组内名；人话包裹形态与来源标签随
// 新语义措辞（首组内名）。单组收尾形态产物零漂移由既有断言锚定
//（Artist[{作者}]/[{标签}]，见本文件其余壳形用例）。
func TestSummarizeMultiGroupDirFirstGroupTemplate(t *testing.T) {
	const dir = "[七尾ゆきじ] 酔いが醒めるその前に (COMIC 阿吽 2019年2月号) [中国翻訳] [DL版]"
	rel := dir + "/001.png"
	assign := &SessionAssignment{Tags: []string{"七尾ゆきじ"}, TagClueRefs: []string{"dirLevelStripped1"}}
	sugg, rules, err := buildStructMapRules("E:/libs/漫画库", rel, "001.png", assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 1 || len(sugg.Rules) != 1 {
		t.Fatalf("应恰一条目录层条目，实际 %d 条", len(rules))
	}
	dirCond := rules[0].Conditions[0].Condition
	if dirCond.Target != CondxTargetDir || dirCond.Format.Tpl != "[{标签}] {任意}" {
		t.Fatalf("多组名目录模板应收敛为 前缀+[{标签}]+间隙+{任意}: %+v", dirCond.Format)
	}
	if len(dirCond.Attach) != 0 {
		t.Fatalf("目录行不应附任何层级锚: %+v", dirCond.Attach)
	}
	tagAct := rules[0].Extractions[0]
	if tagAct.Dimension != DimTag || tagAct.From != FromCapture ||
		tagAct.CaptureName != condxVocab["标签"] || tagAct.Row != 1 {
		t.Fatalf("标签提取应为目录行 {标签} 位捕获: %+v", tagAct)
	}

	// 同形异值命中：首组捕获泛化、首组后正文由 {任意} 吞位。
	pattern := rules[0].Pattern
	sameShape := &MatchInput{
		FileName: "002.png",
		DirNames: []string{"[別の作者] その他の作品 (別冊) [汉化] [DL版]"},
		RelPath:  "[別の作者] その他の作品 (別冊) [汉化] [DL版]/002.png",
	}
	ctx := &ParseContext{}
	if !pattern.Evaluate(sameShape, nil, ctx) {
		t.Fatal("同形异值多组名目录应命中")
	}
	decl, ok := tagAct.Apply(rules[0].ID, ctx)
	if !ok || decl.Value != "別の作者" {
		t.Fatalf("同形异值标签捕获应为首组内名: %+v", decl)
	}

	// 人话与来源标签：目录段句式 + 壳内名来源、多组名注明首组内名。
	if !strings.Contains(sugg.Summary, "→ 标签=壳内名") {
		t.Fatalf("人话规则应为多段句式: %q", sugg.Summary)
	}
	if len(sugg.Mappings) != 1 || sugg.Mappings[0].From != "目录段壳内名（首组内名）" {
		t.Fatalf("来源标签应注明首组内名: %v", sugg.Mappings)
	}
}

// TestSummarizeStrippedAutoMatchPriority 自动匹配优先序（剥壳变体>目录层>
// 库根名>文件名段）在多组名剥壳值域下的收敛：值=首组内名且同值也在文件名
// 首段在场 → 仍解析到剥壳变体线索（目录层条目承载、ClueRef=dirLevelStripped1），
// 产物同多组名收敛形态。
func TestSummarizeStrippedAutoMatchPriority(t *testing.T) {
	rel := "[X] 画集/X__01.png"
	assign := &SessionAssignment{Author: "X"} // 无显式出处 → 自动匹配
	sugg, rules, err := buildStructMapRules("E:/libs/锚库", rel, "X__01.png", assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("应恰一条目录层条目，实际 %d 条", len(rules))
	}
	dirCond := rules[0].Conditions[0].Condition
	if dirCond.Target != CondxTargetDir || dirCond.Format.Tpl != "[{作者}] {任意}" {
		t.Fatalf("剥壳变体优先：应产出目录层条目且模板收敛: %+v", dirCond.Format)
	}
	authAct := rules[0].Extractions[0]
	if authAct.Dimension != DimAuthor || authAct.From != FromCapture ||
		authAct.CaptureName != condxVocab["作者"] || authAct.Row != 1 {
		t.Fatalf("作者提取应为目录行 {作者} 位捕获: %+v", authAct)
	}
	if len(sugg.Mappings) != 1 || sugg.Mappings[0].ClueRef != "dirLevelStripped1" {
		t.Fatalf("自动匹配应解析到剥壳变体线索: %v", sugg.Mappings)
	}
}

// TestBuildStructMapRulesUnderscoreManualDemotion 纯 _ 模式的段值降级：目录名
// foo_bar 作提示名在段间正文内跨分隔符保持整段，其段值含活跃分隔符 _ → 该维度
// 手工（ManualDims 收录、不生成捕获）。2026-11-04 终审拍板（惰性化）翻新：
// 填充段让位后作者段止于首 _（恰=段值 x），旧贪婪「作者段与相邻未引用段的
// 分隔多解切歧义」消解——自检通过、构造成功产出文件行规则；含 _ 的作品名段
// 仍诚实降级手工（活跃分隔符不可入捕获边界）。
func TestBuildStructMapRulesUnderscoreManualDemotion(t *testing.T) {
	rel := "foo_bar/[a]_x_foo_bar_y_[01].png"
	file := "[a]_x_foo_bar_y_[01].png"
	assign := &SessionAssignment{
		Author: "x", AuthorClueRef: "filenameSeg1",
		Title: "foo_bar", TitleClueRef: "filenameSeg2",
	}
	sugg, rules, err := buildStructMapRules("E:/libs/下划线库", rel, file, assign)
	if err != nil {
		t.Fatalf("惰性化后切歧义消解，应构造成功: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("应恰一条文件行规则，实际 %d 条", len(rules))
	}
	if tpl := rules[0].Conditions[0].Condition.Format.Tpl; tpl != "[{任意}]_{作者}_{任意}_{任意}_[{任意}]" {
		t.Fatalf("作者段模板不符: %q", tpl)
	}
	authAct := rules[0].Extractions[0]
	if authAct.Dimension != DimAuthor || authAct.From != FromCapture || authAct.CaptureName != condxVocab["作者"] {
		t.Fatalf("作者提取应为 {作者} 位捕获: %+v", authAct)
	}
	if len(sugg.ManualDims) != 1 || !strings.Contains(sugg.ManualDims[0], "作品名=foo_bar") {
		t.Fatalf("含活跃分隔符的作品名应保持手工降级: %v", sugg.ManualDims)
	}
}

// TestStructSummaryPreviewAndSessionRoundTrip 会话级链路：赋值出处入会话存储并
// 回透视图、summarizeRule 建议含命中预览与样例且不写档案、getPanorama 透出
// 结构线索集、generalize mode=struct 确认后组合规则入档（条件行模板经档案
// 序列化往返）。
func TestStructSummaryPreviewAndSessionRoundTrip(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	_, rel, _ := realSampleStructFixture()
	writeTestFile(t, root+"/"+rel, "a")
	writeTestFile(t, root+"/Artist[Jake--Jaccobus]/2b__comm__by_jake__jaccobus_dhecdru.png", "b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	// 赋值带出处：会话存储 + 视图回透。
	sess, view, err := m.ApplyAssignment(root, rel, SessionAssignment{
		Author: "Jake--Jaccobus", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
		Tags: []string{"big belly"}, TagClueRefs: []string{"rootName"},
	})
	if err != nil {
		t.Fatalf("赋值失败: %v", err)
	}
	if stored := resolveFileAssignment(sess.Assignments, rel); stored.AuthorClueRef != "dirLevelStripped1" ||
		stored.TitleClueRef != "filenameSeg0" || len(stored.TagClueRefs) != 1 || stored.TagClueRefs[0] != "rootName" {
		t.Fatalf("赋值出处未入会话存储: %+v", stored)
	}
	if view.AuthorClueRef != "dirLevelStripped1" || view.TitleClueRef != "filenameSeg0" ||
		len(view.TagClueRefs) != 1 || view.TagClueRefs[0] != "rootName" {
		t.Fatalf("赋值视图未回透出处: %+v", view)
	}

	// summarizeRule：建议卡（命中预览+样例），不写档案。
	profileBefore, _ := LoadProfile(m.store)
	rulesBefore := 0
	if profileBefore != nil {
		rulesBefore = len(profileBefore.Rules)
	}
	sugg, err := m.ApplyStructSummary(root, rel)
	if err != nil {
		t.Fatalf("规则总结失败: %v", err)
	}
	if sugg.HitTotal != 2 || sugg.HitCount != 1 {
		t.Fatalf("命中预览应=同目录其余 1 个（共 2），实际 %d/%d", sugg.HitCount, sugg.HitTotal)
	}
	if len(sugg.Samples) != 1 || !strings.Contains(sugg.Samples[0].FileName, "2b__") ||
		!strings.Contains(sugg.Samples[0].Result, "作者=Jake--Jaccobus") ||
		!strings.Contains(sugg.Samples[0].Result, "作品名=2b") {
		t.Fatalf("样例应用不符: %+v", sugg.Samples)
	}
	profileAfter, _ := LoadProfile(m.store)
	rulesAfter := 0
	if profileAfter != nil {
		rulesAfter = len(profileAfter.Rules)
	}
	if rulesAfter != rulesBefore {
		t.Fatalf("summarizeRule 不应写档案：前 %d 后 %d", rulesBefore, rulesAfter)
	}

	// getPanorama：文件级结构线索集透出。
	pano := panoramaViewOf(sess.Snapshot)
	rec := snapshotFileOf(&pano, rel)
	if rec == nil {
		t.Fatal("全景缺样例文件记录")
	}
	if c, ok := clueByRef(rec.StructClues, "dirLevelStripped1"); !ok || c.Value != "Jake--Jaccobus" {
		t.Fatalf("全景应透出剥壳线索: %v", rec.StructClues)
	}
	if c, ok := clueByRef(rec.StructClues, "tailCode"); !ok || c.Value != "delithw" {
		t.Fatalf("全景应透出尾部码线索: %v", rec.StructClues)
	}

	// generalize mode=struct：确认入档（组合规则两条，目录行模板经档案序列化往返）。
	result, err := m.ApplyGeneralization(root, rel, generalizeModeStruct)
	if err != nil {
		t.Fatalf("结构映射泛化失败: %v", err)
	}
	if result.Rule == nil || len(result.Rules) != 2 || !result.VerificationStarted {
		t.Fatalf("泛化结果形态不符: %+v", result)
	}
	profile, errs := LoadProfile(m.store)
	if len(errs) > 0 || profile == nil {
		t.Fatalf("载入档案失败: %v", errs)
	}
	found, captureForm := 0, false
	for i := range profile.Rules {
		for _, act := range profile.Rules[i].Extractions {
			// v4：剥壳语义由目录行捕获承载（FromCapture+行 1，无 Strip 字段）。
			if act.From == FromCapture && act.CaptureName == condxVocab["作者"] && act.Row == 1 && act.Dimension == DimAuthor {
				captureForm = true
			}
		}
		for _, r := range result.Rules {
			if profile.Rules[i].ID == r.ID {
				found++
			}
		}
	}
	if found != 2 || !captureForm {
		t.Fatalf("组合规则应两条入档且作者走目录行捕获往返（入档 %d、捕获形 %v）", found, captureForm)
	}
}

// TestPreviewStructRulesAcrossDirs 全库命中预览：HitTotal=全库文件总数、
// HitCount=除样本外被组合规则完整命中的文件数（跨目录，剥壳泛化锚命中同形
// 异名目录）；样例取跨目录前 3 个并附目录键（DirRelKey）。
func TestPreviewStructRulesAcrossDirs(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	writeTestFile(t, root+"/Artist[J]/2b__y_dhecdru.png", "b")
	writeTestFile(t, root+"/Artist[J]/atago__x_delithw.png", "a")
	writeTestFile(t, root+"/Artist[K]/3c__z_code2.png", "c")
	writeTestFile(t, root+"/其他/root.jpg", "d")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	rel := "Artist[J]/atago__x_delithw.png"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{
		Author: "J", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
	}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	sugg, err := m.ApplyStructSummary(root, rel)
	if err != nil {
		t.Fatalf("规则总结失败: %v", err)
	}
	if sugg.HitTotal != 4 {
		t.Fatalf("全库文件总数应 = 4，实际 %d", sugg.HitTotal)
	}
	if sugg.HitCount != 2 {
		t.Fatalf("除样本外命中数应 = 2（2b 与跨目录 3c），实际 %d", sugg.HitCount)
	}
	if len(sugg.Samples) != 2 {
		t.Fatalf("样例应 2 条（其余命中文件），实际 %+v", sugg.Samples)
	}
	byDir := map[string]RuleSuggestionSample{}
	for _, s := range sugg.Samples {
		byDir[s.DirRelKey] = s
	}
	if s, ok := byDir["Artist[J]"]; !ok || !strings.Contains(s.FileName, "2b__") ||
		!strings.Contains(s.Result, "作品名=2b") {
		t.Fatalf("同目录样例不符: %+v", s)
	}
	if s, ok := byDir["Artist[K]"]; !ok || !strings.Contains(s.FileName, "3c__") ||
		!strings.Contains(s.Result, "作者=K") || !strings.Contains(s.Result, "作品名=3c") {
		t.Fatalf("跨目录样例应带目录键且求值按各自目录取值: %+v", s)
	}
}

// TestDirSummarizeRuleAddressing 目录寻址总结：样本=子树内首个有赋值的文件、
// 建议卡 RelPath 呈现目录键；子树无赋值文件回可读错误。
func TestDirSummarizeRuleAddressing(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)
	writeTestFile(t, root+"/Artist[X]/a__t.png", "a")
	writeTestFile(t, root+"/Artist[X]/sub/deep__c.png", "b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	if _, err := m.ApplyDirStructSummary(root, "Artist[X]"); err == nil ||
		!strings.Contains(err.Error(), "子树内没有已赋值文件") {
		t.Fatalf("无赋值子树应回可读错误: %v", err)
	}
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/a__t.png", SessionAssignment{
		Author: "X", AuthorClueRef: "dirLevelStripped1",
		Title: "a", TitleClueRef: "filenameSeg0",
	}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	sugg, err := m.ApplyDirStructSummary(root, "Artist[X]")
	if err != nil {
		t.Fatalf("目录寻址总结失败: %v", err)
	}
	if sugg.RelPath != "Artist[X]" {
		t.Fatalf("建议卡 RelPath 应呈现目录键: %q", sugg.RelPath)
	}
	if !strings.Contains(sugg.Summary, "作者=壳内名") || !strings.Contains(sugg.Summary, "文件名首段→作品名") {
		t.Fatalf("人话规则应由子树首个有赋值文件推导: %q", sugg.Summary)
	}
	if sugg.HitTotal != 12 {
		t.Fatalf("全库文件总数应含样例树（10+2），实际 %d", sugg.HitTotal)
	}
}

// TestHandleFrontendRequestSummarizeRule 前端协议分派：assign 携带出处载荷、
// summarizeRule 应答建议卡。
func TestHandleFrontendRequestSummarizeRule(t *testing.T) {
	root := t.TempDir()
	m, ctx := newRecognitionFixture(t, root, nil)
	_, rel, _ := realSampleStructFixture()
	writeTestFile(t, root+"/"+rel, "a")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 30, Action: workbenchActionAssign, Root: root, RelPath: rel,
		Author: "Jake--Jaccobus", AuthorClueRef: "dirLevelStripped1",
		Title: "atago", TitleClueRef: "filenameSeg0",
	}))
	if resp := ctx.lastResponse(t); !resp.OK || resp.Assignment == nil ||
		resp.Assignment.AuthorClueRef != "dirLevelStripped1" || resp.Assignment.TitleClueRef != "filenameSeg0" {
		t.Fatalf("assign 出处载荷应答不符: %+v", resp)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 31, Action: workbenchActionSummarizeRule, Root: root, RelPath: rel,
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.RuleSuggestion == nil {
		t.Fatalf("summarizeRule 应答不符: %+v", resp)
	}
	if !strings.Contains(resp.RuleSuggestion.Summary, "作者=壳内名") ||
		!strings.Contains(resp.RuleSuggestion.Summary, "文件名首段→作品名") {
		t.Fatalf("建议卡人话规则不符: %q", resp.RuleSuggestion.Summary)
	}

	// 无赋值文件：可读错误。
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 32, Action: workbenchActionSummarizeRule, Root: root, RelPath: "不存在/x.png",
	}))
	if resp := ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "无会话赋值") {
		t.Fatalf("无赋值应回可读错误: %+v", resp)
	}
}

// previewStructRulesSerialRef 结构建议卡命中预览的串行参考实现（阶段4 求值相
// 并行化前的原实现逐行复刻）：并行实现的等值锚基准——并行只改变求值顺序的
// 可并行面，逐字段输出（计数/样例）须与本参考全等。
func previewStructRulesSerialRef(rules []RuleEntry, snap *RecognitionSnapshot, selfRelPath string, sub *evalSubstrate) (int, int, []RuleSuggestionSample) {
	hitCount, hitTotal := 0, 0
	var samples []RuleSuggestionSample
	onSubstrate := sub != nil && len(sub.files) == len(snap.Files)
	for i := range snap.Files {
		f := &snap.Files[i]
		hitTotal++
		dirNames := ExtractDirLevels(f.RelPath)
		if onSubstrate {
			dirNames = sub.files[i].DirNames
		}
		in := &MatchInput{FileName: f.FileName, DirNames: dirNames, RelPath: f.RelPath}
		var decls []MetadataDeclaration
		all := true
		for j := range rules {
			ctx := &ParseContext{}
			if !rules[j].Pattern.Evaluate(in, nil, ctx) {
				all = false
				break
			}
			for _, action := range rules[j].Extractions {
				if decl, ok := action.Apply(rules[j].ID, ctx); ok {
					decls = append(decls, decl)
				}
			}
		}
		if !all || len(decls) == 0 {
			continue
		}
		if f.RelPath == selfRelPath {
			continue
		}
		hitCount++
		if len(samples) < 3 {
			parts := make([]string, 0, len(decls))
			for _, decl := range decls {
				parts = append(parts, qnaDimensionLabel(decl.Dimension)+"="+decl.Value)
			}
			samples = append(samples, RuleSuggestionSample{
				FileName: f.FileName, DirRelKey: f.DirRelKey, Result: strings.Join(parts, "，"),
			})
		}
	}
	return hitCount, hitTotal, samples
}

// TestPreviewStructRulesParallelEquivalence 结构建议卡命中预览求值并行化等值锚
//（预览面大库性能方案·阶段4）：同一夹具（文件数越过分块阈值 evalFileChunkSize
//——并行区真多块）上，串行参考实现（previewStructRulesSerialRef）vs 生产实现
// 在多工作子与 1 工作子（串行回退路径）两模式、基座供给（sub）与现场重建
//（nil）两形态下，计数与样例逐字段全等（reflect.DeepEqual）；夹具形态保证
// 命中数远超样例上限 3（样例原序装配面非空转）且自排样本在命中集内（自排除
// 路径被走）。求值期共享态只读的声明同由本锚在 -race 下运行期验证。
func TestPreviewStructRulesParallelEquivalence(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	// 体量树：5 目录 × 60 同形命中文件（Artist[X] 壳目录 + 标题__任意_任意.png
	// 文件名形态）+ 少量不命中形态（异名目录/异形文件名——未命中路径覆盖），
	// 总数越过分块阈值 128（并行区 ≥3 块）。
	for d := 0; d < 5; d++ {
		for i := 1; i <= 60; i++ {
			writeTestFile(t, fmt.Sprintf("%s/Artist[%c]/t%02d__x_%03d.png", root, 'A'+rune(d), i, i), "x")
		}
	}
	writeTestFile(t, root+"/其他/root.jpg", "d")
	writeTestFile(t, root+"/Artist[A]/notes.txt", "e")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	snap := sess.Snapshot
	if len(snap.Files) <= evalFileChunkSize {
		t.Fatalf("夹具形态缺损：文件数 %d 未越过分块阈值 %d（并行区不会多块）", len(snap.Files), evalFileChunkSize)
	}
	sub := m.evalSubstrateOf(sess)
	if sub == nil {
		t.Fatal("识别完成后预览基座应就位（sub 形态须实际被走）")
	}

	// 组合规则对（与建议卡同款构造）：目录壳模板 + 文件名段模板，命中=全部
	// 5 目录 × 60 文件同形。
	self := "Artist[A]/t01__x_001.png"
	assign := &SessionAssignment{
		Author: "A", AuthorClueRef: "dirLevelStripped1",
		Title: "t01", TitleClueRef: "filenameSeg0",
	}
	_, rules, err := buildStructMapRules(root, self, "t01__x_001.png", assign)
	if err != nil {
		t.Fatalf("结构映射规则构造失败: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("夹具形态缺损：应构造目录+文件名两条规则，实际 %d 条", len(rules))
	}

	assertEqual := func(label string, sub *evalSubstrate) {
		t.Helper()
		gotH, gotTotal, gotSamples := previewStructRules(rules, snap, self, sub)
		wantH, wantTotal, wantSamples := previewStructRulesSerialRef(rules, snap, self, sub)
		if gotH != wantH || gotTotal != wantTotal || !reflect.DeepEqual(gotSamples, wantSamples) {
			t.Fatalf("%s：并行与串行参考输出不等值：%d/%d %+v vs %d/%d %+v",
				label, gotH, gotTotal, gotSamples, wantH, wantTotal, wantSamples)
		}
	}
	// 夹具形态保证：命中数远超样例上限（样例恰 3 条、原序装配敏感）且自排
	// 样本在命中集内（自排除路径实际生效）。
	if wantH, _, _ := previewStructRulesSerialRef(rules, snap, self, nil); wantH < 4 {
		t.Fatalf("夹具形态缺损：命中数 %d 未越样例上限（等值锚空转）", wantH)
	}
	if _, _, wantSamples := previewStructRulesSerialRef(rules, snap, self, nil); len(wantSamples) != 3 {
		t.Fatalf("夹具形态缺损：样例应恰 3 条，实际 %d 条", len(wantSamples))
	}

	// 多工作子模式（并行区真多块）：sub/nil 两形态各与串行参考全等。
	withEvalWorkers(t, parallelWorkersMulti)
	assertEqual("多工作子·现场重建", nil)
	assertEqual("多工作子·基座供给", sub)
	// 1 工作子（生产串行回退路径）同样与串行参考全等。
	withEvalWorkers(t, 1)
	assertEqual("单工作子·现场重建", nil)
	assertEqual("单工作子·基座供给", sub)
}
