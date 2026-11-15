// 占位词词汇表重设计（阶段1/阶段2）的锚定测试。阶段1：字母数字类放宽、
// {作品} 改名 {作品标识}（别名书写）、{描述} 新词与描述维度常量、多处出现
// 全集扩容、{作品标识}/{作品号} 同模板互斥与 {公共段} 多次出现的中文报错，
// 以及编辑面保存事务（ApplySaveRule）上的分P 双标题模板往返。阶段2：序列
// 模板可选词位 {词?} 解禁与全可选空键守卫、成套键词根过滤（{序号} 位次变体
// 与可选捕获不进键）、序号自然排序（数值前缀主序、无前缀殿后、Seq 取前缀）。
package main

import (
	"math"
	"regexp"
	"strings"
	"testing"
)

// TestVocabAlnumClassRelax 字母数字类编译断言：{作品号}/{序号} 编译
// [0-9A-Za-z]+——BV 号整词命中、纯数字回归不变；{日期} 维持纯数字类。
func TestVocabAlnumClassRelax(t *testing.T) {
	exprWorkID, caps, _, _, err := deriveCondxTemplate("{作品号}", deriveOpts{})
	if err != nil {
		t.Fatalf("{作品号} 推导失败: %v", err)
	}
	if exprWorkID != `^(?P<workid>[0-9A-Za-z]+)$` {
		t.Fatalf("{作品号} 表达式不符: %q", exprWorkID)
	}
	re := regexp.MustCompile(exprWorkID)
	if !re.MatchString("BV1GJ411x7f7") {
		t.Fatalf("BV 形态应经 {作品号} 整词命中")
	}
	if !re.MatchString("116084711628056") {
		t.Fatalf("纯数字应照常命中")
	}
	if re.MatchString("BV1GJ411x7f7-补丁") {
		t.Fatalf("带连字符整段不应命中（类外字符截断）")
	}
	if len(caps) != 1 || caps[0] != "workid" {
		t.Fatalf("捕获组不符: %v", caps)
	}

	exprSeq, _, _, _, err := deriveCondxTemplate("P{序号}", deriveOpts{})
	if err != nil {
		t.Fatalf("{序号} 推导失败: %v", err)
	}
	if exprSeq != `^P(?P<seq>[0-9A-Za-z]+)$` {
		t.Fatalf("{序号} 表达式不符: %q", exprSeq)
	}
	if !regexp.MustCompile(exprSeq).MatchString("P12a") {
		t.Fatalf("字母混合序号应命中")
	}

	exprDate, _, _, _, err := deriveCondxTemplate("{日期}", deriveOpts{})
	if err != nil {
		t.Fatalf("{日期} 推导失败: %v", err)
	}
	if exprDate != `^(?P<date>[0-9]+)$` {
		t.Fatalf("{日期} 应维持纯数字类: %q", exprDate)
	}
	reDate := regexp.MustCompile(exprDate)
	if !reDate.MatchString("20240101") || reDate.MatchString("2024a") {
		t.Fatalf("{日期} 纯数字回归/字母拒绝不符预期")
	}
}

// TestVocabWorkRenameAlias {作品} 改名 {作品标识}：中文键并存新旧行（同组
// work）、两种书写推导产物一致（合法别名、零迁移）；提示串只列新词。
func TestVocabWorkRenameAlias(t *testing.T) {
	if condxVocab["作品标识"] != "work" || condxVocab["作品"] != "work" {
		t.Fatalf("{作品标识} 应与旧词 {作品} 同组 work: 标识=%q 旧词=%q", condxVocab["作品标识"], condxVocab["作品"])
	}
	exprNew, capsNew, _, _, err := deriveCondxTemplate("{作品标识}.{任意}", deriveOpts{})
	if err != nil {
		t.Fatalf("新词书写推导失败: %v", err)
	}
	exprOld, capsOld, _, _, err := deriveCondxTemplate("{作品}.{任意}", deriveOpts{})
	if err != nil {
		t.Fatalf("旧词书写推导失败: %v", err)
	}
	if exprNew != exprOld || strings.Join(capsNew, ",") != strings.Join(capsOld, ",") {
		t.Fatalf("新旧书写推导应一致:\n新 %q %v\n旧 %q %v", exprNew, capsNew, exprOld, capsOld)
	}
	if strings.Join(capsNew, ",") != "work" {
		t.Fatalf("组名应为 work: %v", capsNew)
	}

	// {键}→{公共段} 别名先例回归锚：同组别名机制两对并存。
	exprKey, capsKey, _, _, err := deriveCondxTemplate("{键}_{序号}", deriveOpts{allowKey: true, requireSeq: true})
	if err != nil {
		t.Fatalf("{键} 书写推导失败: %v", err)
	}
	exprSeg, capsSeg, _, _, err := deriveCondxTemplate("{公共段}_{序号}", deriveOpts{allowKey: true, requireSeq: true})
	if err != nil {
		t.Fatalf("{公共段} 书写推导失败: %v", err)
	}
	if exprKey != exprSeg || strings.Join(capsKey, ",") != strings.Join(capsSeg, ",") {
		t.Fatalf("{键}/{公共段} 别名推导应一致:\n%q %v\n%q %v", exprKey, capsKey, exprSeg, capsSeg)
	}

	if !strings.Contains(condxVocabHint, "作品标识") {
		t.Fatalf("提示串应列新词 {作品标识}: %q", condxVocabHint)
	}
	if strings.Contains(condxVocabHint, "作品/") || strings.HasSuffix(condxVocabHint, "作品") {
		t.Fatalf("提示串不应再列旧词 {作品}: %q", condxVocabHint)
	}
	if strings.Contains(condxVocabHint, "键") {
		t.Fatalf("提示串不应列旧词 {键}: %q", condxVocabHint)
	}
}

// TestVocabDescriptionWord {描述} 入词表与维度常量：description 为第五声明
// 维度（创建期接线归后续阶段，本测试只锚词表与常量）。
func TestVocabDescriptionWord(t *testing.T) {
	if condxVocab["描述"] != "description" {
		t.Fatalf("{描述} 应映射 description: %q", condxVocab["描述"])
	}
	if DimDescription != "description" {
		t.Fatalf("DimDescription 应为 description: %q", DimDescription)
	}
	for _, dim := range []MetadataDimension{DimAuthor, DimTag, DimTitle, DimCollection} {
		if dim == DimDescription {
			t.Fatalf("DimDescription 应与既有四维互异")
		}
	}
	expr, caps, _, _, err := deriveCondxTemplate("{标题}({描述})", deriveOpts{})
	if err != nil {
		t.Fatalf("{描述} 推导失败: %v", err)
	}
	if expr != `^(?P<title>.+?)\((?P<description>[^)]+)\)$` {
		t.Fatalf("{描述} 表达式不符: %q", expr)
	}
	if len(caps) != 2 || caps[1] != "description" {
		t.Fatalf("捕获组不符: %v", caps)
	}
	if m := regexp.MustCompile(expr).FindStringSubmatch("作品甲(简介文本)"); m == nil || m[2] != "简介文本" {
		t.Fatalf("描述段取值不符: %v", m)
	}
	if !strings.Contains(condxVocabHint, "描述") {
		t.Fatalf("提示串应列 {描述}: %q", condxVocabHint)
	}
}

// TestVocabMultiOccurExpanded 多处出现全集扩容：标题/序号第二次出现放行且
// 组名带 __2（title__2/seq__2）；{公共段} 多次出现维持中文拒绝。
func TestVocabMultiOccurExpanded(t *testing.T) {
	const tpl = "{标题} - {序号}.{标题}(Av{作品号},P{序号})"
	expr, caps, _, _, err := deriveCondxTemplate(tpl, deriveOpts{})
	if err != nil {
		t.Fatalf("分P 双标题模板推导失败: %v", err)
	}
	wantExpr := `^(?P<title>.+?) - (?P<seq>[0-9A-Za-z]+)\.(?P<title__2>.+?)\(Av(?P<workid>[0-9A-Za-z]+),P(?P<seq__2>[0-9A-Za-z]+)\)$`
	if expr != wantExpr {
		t.Fatalf("分P 模板表达式不符:\n得到 %q\n期望 %q", expr, wantExpr)
	}
	if strings.Join(caps, ",") != "title,seq,title__2,workid,seq__2" {
		t.Fatalf("捕获组（按位）不符: %v", caps)
	}
	const name = "暗箱工作室祝各位马年大吉，鸿运当头！ - 1.暗箱工作室祝各位马年大吉，鸿运当头！(Av116084711628056,P1)"
	m := regexp.MustCompile(expr).FindStringSubmatch(name)
	if m == nil {
		t.Fatalf("用户实例文件名应命中分P 模板")
	}
	byName := map[string]string{}
	for i, g := range regexp.MustCompile(expr).SubexpNames() {
		if i > 0 && g != "" {
			byName[g] = m[i]
		}
	}
	for g, want := range map[string]string{
		"title": "暗箱工作室祝各位马年大吉，鸿运当头！", "seq": "1",
		"title__2": "暗箱工作室祝各位马年大吉，鸿运当头！", "seq__2": "1",
		"workid": "116084711628056",
	} {
		if byName[g] != want {
			t.Fatalf("命名组 %s 取值不符: 得到 %q 期望 %q", g, byName[g], want)
		}
	}

	// BV 号经 {作品号} 整词命中（位内取值）。
	bv, _, _, _, err := deriveCondxTemplate("{标题}(Av{作品号},P{序号})", deriveOpts{})
	if err != nil {
		t.Fatalf("BV 形态模板推导失败: %v", err)
	}
	mb := regexp.MustCompile(bv).FindStringSubmatch("某投稿(AvBV1GJ411x7f7,P12)")
	if mb == nil {
		t.Fatalf("BV 号应整词命中")
	}
	if mb[2] != "BV1GJ411x7f7" || mb[3] != "12" {
		t.Fatalf("BV 命中取值不符: %v", mb)
	}

	// 其余取值词双写同样放行（日期/作品标识位次分立）。
	for _, tc := range []struct {
		tpl  string
		caps string
	}{
		{"{日期}_{日期}", "date,date__2"},
		{"{作品标识}_{作品标识}", "work,work__2"},
		{"{描述}_{描述}", "description,description__2"},
	} {
		_, caps, _, _, err := deriveCondxTemplate(tc.tpl, deriveOpts{})
		if err != nil {
			t.Fatalf("%q 双写应放行: %v", tc.tpl, err)
		}
		if strings.Join(caps, ",") != tc.caps {
			t.Fatalf("%q 捕获组不符: %v 期望 %s", tc.tpl, caps, tc.caps)
		}
	}

	// {公共段} 维持单次：双写拒绝、组名不带位次。
	_, _, _, _, err = deriveCondxTemplate("{公共段}_{公共段}_{序号}", deriveOpts{allowKey: true, requireSeq: true})
	if err == nil || !strings.Contains(err.Error(), "模板占位词 {公共段} 重复") {
		t.Fatalf("{公共段} 双写应中文拒绝，实际 err=%v", err)
	}
}

// TestVocabWorkIDMutualExclusion {作品标识} 与 {作品号} 同模板并用拒绝
// （整词标识 vs 编号段，语义近邻防混用）：新旧书写、先后顺序、可选词位四种
// 形态全部拒绝（报错点在后见词、文案点名两者）；单用各自照常放行。
func TestVocabWorkIDMutualExclusion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tpl     string
		wantErr string
	}{
		{"新词在前", "{作品标识}_{作品号}", "模板占位词 {作品号} 不可与 {作品标识} 同模板并用"},
		{"旧词别名等同", "{作品}_{作品号}", "模板占位词 {作品号} 不可与 {作品标识} 同模板并用"},
		{"编号段在前", "{作品号}_{作品标识}", "模板占位词 {作品标识} 不可与 {作品号} 同模板并用"},
		{"可选词位形态", "{作品标识?}_{作品号}", "模板占位词 {作品号} 不可与 {作品标识} 同模板并用"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, _, err := deriveCondxTemplate(tc.tpl, deriveOpts{})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
			}
			if !strings.Contains(err.Error(), "语义近邻") {
				t.Fatalf("报错应说明拒绝理由（语义近邻）: %v", err)
			}
		})
	}
	for _, tpl := range []string{"{作品标识}.{任意}", "Av{作品号}_{标题}"} {
		if _, _, _, _, err := deriveCondxTemplate(tpl, deriveOpts{}); err != nil {
			t.Fatalf("单用模板 %q 应放行: %v", tpl, err)
		}
	}
}

// TestApplySaveRuleVocabRedesignRoundTrip 编辑面保存事务上的新词表锚：分P
// 双标题模板（标题/序号双写）可保存、档案往返保真、提取行按位引用
// （title__2/seq__2）；{作品标识} 与 {作品号} 同模板并用在保存事务被拒。
func TestApplySaveRuleVocabRedesignRoundTrip(t *testing.T) {
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)

	ruleJSON := []byte(`{
		"id": "", "description": "分P 双标题（投稿标题/P标题）", "source": "manual", "enabled": true,
		"conditions": [
			{"condition": {"target": "file", "format": {"mode": "template", "tpl": "{标题} - {序号}.{标题}(Av{作品号},P{序号})"}}}
		],
		"extractions": [
			{"dimension": "collection", "from": "capture", "captureName": "title", "row": 1},
			{"dimension": "title", "from": "capture", "captureName": "title__2", "row": 1},
			{"dimension": "description", "from": "capture", "captureName": "title__2", "row": 1}
		],
		"synthesis": {"dimPolicies": [], "roles": []}
	}`)
	view, err := mgr.ApplySaveRule(ruleJSON)
	if err != nil {
		t.Fatalf("分P 双标题条目保存失败: %v", err)
	}
	if len(view.Rules) != 1 {
		t.Fatalf("保存后视图应有 1 条规则，实际 %d 条", len(view.Rules))
	}

	profile := loadAdminProfile(t, store)
	if len(profile.Rules) != 1 {
		t.Fatalf("档案应有 1 条规则，实际 %d 条", len(profile.Rules))
	}
	entry := profile.Rules[0]
	cond := entry.Conditions[0].Condition
	if cond.Target != CondxTargetFile || cond.Format.Tpl != "{标题} - {序号}.{标题}(Av{作品号},P{序号})" {
		t.Fatalf("条件行内容不符: %+v", cond)
	}
	if len(entry.Extractions) != 3 ||
		entry.Extractions[0].CaptureName != "title" ||
		entry.Extractions[1].CaptureName != "title__2" ||
		entry.Extractions[2].Dimension != DimDescription {
		t.Fatalf("按位提取与描述维度往返不符: %+v", entry.Extractions)
	}

	// {描述} 声明（capture 来源）在终选前照常流转（本阶段只锚提取面合法）。
	bad := []byte(`{
		"id": "bad-pair", "description": "标识与编号并用", "source": "manual", "enabled": true,
		"conditions": [
			{"condition": {"target": "file", "format": {"mode": "template", "tpl": "{作品标识}(Av{作品号})"}}}
		],
		"extractions": [],
		"synthesis": {"dimPolicies": [], "roles": []}
	}`)
	if _, err := mgr.ApplySaveRule(bad); err == nil || !strings.Contains(err.Error(), "同模板并用") {
		t.Fatalf("{作品标识}/{作品号} 并用应经保存事务拒绝，实际 err=%v", err)
	}
}

// TestSeqOptionalSlotUnbannedAndGuard 序列模板解禁可选词位 {词?}：可选捕获
// 组名集透出；{公共段?} 维持拒绝（成群依据不可缺席）；含可选词位而 {序号}
// 之外无必选取值词（全可选=空键）中文拒绝；不含可选词位的既有空键书写与
// 泛化器产物（{公共段} 必选）不受守卫影响。
func TestSeqOptionalSlotUnbannedAndGuard(t *testing.T) {
	seqOpts := deriveOpts{allowKey: true, requireSeq: true}
	// 解禁：分P 序列模板（集名+作品号为键、{标题?} 逐件变化段）推导通过，
	// 可选捕获集恰为位1 标题。
	expr, caps, _, optCaps, err := deriveCondxTemplate("{集名} - {序号}.{标题?}(Av{作品号},P{序号})", seqOpts)
	if err != nil {
		t.Fatalf("分P 序列模板推导失败: %v", err)
	}
	wantExpr := `^(?P<setname>.+?) - (?P<seq>[0-9A-Za-z]+)\.(?:(?P<title>.+?))?\(Av(?P<workid>[0-9A-Za-z]+),P(?P<seq__2>[0-9A-Za-z]+)\)$`
	if expr != wantExpr {
		t.Fatalf("分P 序列模板表达式不符:\n得到 %q\n期望 %q", expr, wantExpr)
	}
	if strings.Join(caps, ",") != "setname,seq,title,workid,seq__2" {
		t.Fatalf("分P 序列模板捕获组不符: %v", caps)
	}
	if strings.Join(optCaps, ",") != "title" {
		t.Fatalf("可选捕获集不符: %v", optCaps)
	}

	// 必选键段词（{公共段}）在场的可选词位照常放行（泛化器产物同款形态）。
	if _, _, _, _, err := deriveCondxTemplate("{公共段}_{标题?}_{序号}", seqOpts); err != nil {
		t.Fatalf("必选键段词在场的可选词位应放行: %v", err)
	}
	// 不含可选词位的既有空键书写（{序号} 单写）不受守卫影响。
	if _, _, _, _, err := deriveCondxTemplate("第{序号}话", seqOpts); err != nil {
		t.Fatalf("不含可选词位的存量空键书写应照常放行: %v", err)
	}

	// 全可选（空键）拒绝：单可选与多可选两形态，文案点名成套键。
	for _, tpl := range []string{"{标题?}_{序号}", "{标题?}_{作品号?}_{序号}"} {
		_, _, _, _, err := deriveCondxTemplate(tpl, seqOpts)
		if err == nil || !strings.Contains(err.Error(), "成套键为空") {
			t.Fatalf("%q 应以空键守卫拒绝，实际 err=%v", tpl, err)
		}
	}
	// {公共段?} 维持拒绝。
	if _, _, _, _, err := deriveCondxTemplate("{公共段?}_{序号}", seqOpts); err == nil || !strings.Contains(err.Error(), "成套的成群依据，不可带问号") {
		t.Fatalf("{公共段?} 应维持拒绝，实际 err=%v", err)
	}

	// 保存事务面同款拦截（编辑面新存档）：全可选序列模板在 ApplySaveRule 被拒。
	store := newFakeSessionStore()
	mgr, _ := newTestManager(t, store)
	bad := []byte(`{
		"id": "seq-empty-key", "description": "全可选空键", "source": "manual", "enabled": true,
		"conditions": [
			{"condition": {"target": "seq", "format": {"mode": "template", "tpl": "{标题?}_{序号}"}}}
		],
		"extractions": [],
		"synthesis": {"dimPolicies": [], "roles": []}
	}`)
	if _, err := mgr.ApplySaveRule(bad); err == nil || !strings.Contains(err.Error(), "成套键为空") {
		t.Fatalf("全可选序列模板应经保存事务拒绝，实际 err=%v", err)
	}
	good := []byte(`{
		"id": "seq-optional-title", "description": "分P 序列可选标题段", "source": "manual", "enabled": true,
		"conditions": [
			{"condition": {"target": "seq", "format": {"mode": "template", "tpl": "{集名} - {序号}.{标题?}(Av{作品号},P{序号})"}}}
		],
		"extractions": [
			{"dimension": "collection", "from": "capture", "captureName": "setname", "row": 1},
			{"dimension": "title", "from": "capture", "captureName": "title", "row": 1}
		],
		"synthesis": {"dimPolicies": [], "roles": []}
	}`)
	if _, err := mgr.ApplySaveRule(good); err != nil {
		t.Fatalf("分P 序列模板（含可选标题段）保存应放行: %v", err)
	}
}

// TestSeqKeyExcludeOptionalAndSeqVariants 成套键词根过滤求值锚：{序号} 位次
// 变体（seq__2）与可选捕获（{标题?}）不进成套键——集名+作品号为键，P 序号与
// P 标题逐件变化不拆套；标题缺席成员照常入套且无标题绑定（缺席=无绑定）；
// 异作品号成员异键落单。
func TestSeqKeyExcludeOptionalAndSeqVariants(t *testing.T) {
	p, err := compileCondx(&Condx{Target: CondxTargetSeq, Format: condxTpl("{集名} - {序号}.{标题?}(Av{作品号},P{序号})")}, 1)
	if err != nil {
		t.Fatalf("分P 序列模板编译失败: %v", err)
	}
	siblings := []string{
		"画集 - 1.甲标题(Av100,P1).mp4",
		"画集 - 2.乙标题(Av100,P2).mp4",
		"画集 - 3.(Av100,P3).mp4",    // 标题缺席成员
		"画集 - 4.丁标题(Av200,P4).mp4", // 异作品号→异键落单
	}
	for _, tc := range []struct {
		file    string
		seq     int
		wantHit bool
		title   string
		titleOK bool
	}{
		{"画集 - 1.甲标题(Av100,P1).mp4", 1, true, "甲标题", true},
		{"画集 - 2.乙标题(Av100,P2).mp4", 2, true, "乙标题", true},
		{"画集 - 3.(Av100,P3).mp4", 3, true, "", false},
		{"画集 - 4.丁标题(Av200,P4).mp4", 0, false, "", false},
	} {
		ctx := &ParseContext{}
		if hit := p.Evaluate(&MatchInput{FileName: tc.file, Siblings: siblings}, nil, ctx); hit != tc.wantHit {
			t.Fatalf("%s 命中=%v，期望 %v", tc.file, hit, tc.wantHit)
		}
		if !tc.wantHit {
			continue
		}
		if ctx.Group == nil || ctx.Group.Key != "画集100" || ctx.Group.Size != 3 || ctx.Group.Seq != tc.seq {
			t.Fatalf("%s 群组分析不符: %+v", tc.file, ctx.Group)
		}
		if title, ok := ctx.Capture("r1.title"); ok != tc.titleOK || title != tc.title {
			t.Fatalf("%s 标题绑定不符: %q %v（期望 %q %v）", tc.file, title, ok, tc.title, tc.titleOK)
		}
	}
	// 位次变体与键段词绑定照常在场（可被提取行按位接线）。
	ctx := &ParseContext{}
	if !p.Evaluate(&MatchInput{FileName: "画集 - 1.甲标题(Av100,P1).mp4", Siblings: siblings}, nil, ctx) {
		t.Fatalf("成员 1 应命中")
	}
	for name, want := range map[string]string{
		"r1.setname": "画集", "r1.workid": "100", "r1.seq": "1", "r1.seq__2": "1", "r1.title": "甲标题",
	} {
		if got, ok := ctx.Capture(name); !ok || got != want {
			t.Fatalf("绑定 %s 不符: %q %v（期望 %q）", name, got, ok, want)
		}
	}
}

// TestSeqNaturalOrderAnchor 序号自然排序锚定：数值前缀主序、同前缀按余文字
// 节序；无数值前缀的纯字母序号按字典序排在一切有前缀文本之后；超长数字段
// 饱和 MaxInt；求值面 Seq 取数值前缀、无前缀为 0、字母序号成员照常入套。
func TestSeqNaturalOrderAnchor(t *testing.T) {
	ordered := []string{"1", "1a", "1b", "2", "9", "10", "ova", "sp"}
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			if !seqNaturalLess(ordered[i], ordered[j]) {
				t.Fatalf("自然序不符：%q 应先于 %q", ordered[i], ordered[j])
			}
		}
	}
	if seqNaturalLess("sp", "10") || seqNaturalLess("1a", "1") || seqNaturalLess("10", "10") {
		t.Fatalf("无前缀殿后/同前缀余文次序/相等文本判定不符")
	}
	for text, want := range map[string]int{
		"1a": 1, "10": 10, "07": 7, "sp": 0, "": 0,
		strings.Repeat("9", 30): math.MaxInt,
	} {
		if got := seqNumericPrefix(text); got != want {
			t.Fatalf("seqNumericPrefix(%q)=%d，期望 %d", text, got, want)
		}
	}

	p, err := compileCondx(&Condx{Target: CondxTargetSeq, Format: condxTpl("{公共段}_{序号}")}, 1)
	if err != nil {
		t.Fatalf("序列模板编译失败: %v", err)
	}
	siblings := []string{"画集_1a.png", "画集_1b.png", "画集_2.png", "画集_10.png", "画集_ova.png", "画集_sp.png"}
	for _, tc := range []struct {
		file string
		seq  int
	}{
		{"画集_1a.png", 1}, {"画集_1b.png", 1}, {"画集_2.png", 2},
		{"画集_10.png", 10}, {"画集_ova.png", 0}, {"画集_sp.png", 0},
	} {
		ctx := &ParseContext{}
		if !p.Evaluate(&MatchInput{FileName: tc.file, Siblings: siblings}, nil, ctx) {
			t.Fatalf("%s 应照常入套", tc.file)
		}
		if ctx.Group == nil || ctx.Group.Seq != tc.seq || ctx.Group.Key != "画集" || ctx.Group.Size != 6 {
			t.Fatalf("%s 群组分析不符: %+v", tc.file, ctx.Group)
		}
	}
}
