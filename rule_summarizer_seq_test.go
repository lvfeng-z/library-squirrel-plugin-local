package main

// 序列形态建议（规则总结器 · 序列泛化路径）的单元测试：多段含下划线集名匹配
//（用户实例形态——`[作者]_[作品号]_集名段组合_[序号]`）、集名=序号前整键的
// 捕获=整段、集名对不上段组合的退回+说明行（目录层映射与纯手打两态）、非群
// 样本完全走既有形态、when 原子「仅序列成员产出」随规则入档且档案 JSON 形态
// 与规则编辑面复选的判读口径一致、泛化即接管的端到端（本目录成员与跨目录
// 同形序列的声明来源=新规则）。
import (
	"encoding/json"
	"strings"
	"testing"
)

// newSeqRecognitionFixture 序列形态测试环境基座：与 newRecognitionFixture 同
// 构但规则档案为空——通用测试定群规则（test-sibling-set）会以「首个下划线段」
// 为键抢占成群（键≠结构形态键、序号取自作品号），序列形态的样本须由结构群
// 对齐成群（真实库无该规则时的形态）。
func newSeqRecognitionFixture(t *testing.T, root string) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	store := newFakeSessionStore()
	ctx := &workbenchTestCtx{store: store}
	ctx.library = libraryFixture{}
	m := NewSessionManagerWithStore(ctx, nil, store)
	if err := SaveProfile(store, &RuleProfile{SchemaVersion: CurrentSchemaVersion}); err != nil {
		t.Fatalf("写入空规则档案失败: %v", err)
	}
	if _, err := m.CreateSession(root); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	return m, ctx
}

// seqFormFixture 序列形态测试环境：样例目录树——
//   - `X_Misskeyまとめ 1_1～1_31/`：结构群三成员（括号序号形态，键=
//     `[hakosaku]_[126794866]_X_Misskeyまとめ 1_1～1_31`；目录名提示使集名
//     段在元素模型下保持整段——用户实例形态）；
//   - `别处/`：跨目录同形序列两成员（不同作者/作品号/集名段）；
//   - `集名目录/`：尾缀数字结构群两成员（集名=目录名、对不上群键段组合）；
//   - `路灯/`：括号序号结构群两成员（集名=序号前整键）；
//   - `散行/`：同形落单文件（非群样本）。
func seqFormFixture(t *testing.T, root string) (*SessionManager, string) {
	t.Helper()
	m, _ := newSeqRecognitionFixture(t, root)
	seq := "X_Misskeyまとめ 1_1～1_31"
	for _, n := range []string{"01", "02", "03"} {
		writeTestFile(t, root+"/"+seq+"/[hakosaku]_[126794866]_"+seq+"_["+n+"].png", "a"+n)
	}
	writeTestFile(t, root+"/别处/[moka]_[987654321]_Y_Misskeyまとめ 2_2～2_8_[05].jpg", "b")
	writeTestFile(t, root+"/别处/[moka]_[987654321]_Y_Misskeyまとめ 2_2～2_8_[06].jpg", "c")
	writeTestFile(t, root+"/集名目录/甲乙丙_01.jpg", "d")
	writeTestFile(t, root+"/集名目录/甲乙丙_02.jpg", "e")
	writeTestFile(t, root+"/路灯/路灯后辈_[01].jpg", "f")
	writeTestFile(t, root+"/路灯/路灯后辈_[02].jpg", "g")
	writeTestFile(t, root+"/散行/孤本_007.jpg", "h")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, seq
}

// seqSuggOf 目录赋值（作品集）→ 目录寻址总结，返回建议卡。
func seqSuggOf(t *testing.T, m *SessionManager, root, dirKey, collection string) *RuleSuggestion {
	t.Helper()
	if _, _, err := m.ApplyDirAssignment(root, dirKey, SessionAssignment{Collection: collection}); err != nil {
		t.Fatalf("目录赋值失败: %v", err)
	}
	sugg, err := m.ApplyDirStructSummary(root, dirKey)
	if err != nil {
		t.Fatalf("目录总结失败: %v", err)
	}
	return sugg
}

// TestSeqFormSuggestionUserShape 用户实例形态：集名=群键第 3 元素起的连续段
// 组合（含分隔符与下划线）→ 序列形态建议卡——sibling 形态跟随（括号序号提取
// 式）+ 集名捕获规则（捕获组=段组合、when 原子=仅序列成员产出、无目录层
// 锚定）；文案按剧本A 步2详（形态短语/段组合值/映射行段位）；命中预览含
// 跨目录样例。文件入口（等价入口）产出同一推导。
func TestSeqFormSuggestionUserShape(t *testing.T) {
	root := t.TempDir()
	m, seq := seqFormFixture(t, root)
	sugg := seqSuggOf(t, m, root, seq, seq)
	sample := seq + "/[hakosaku]_[126794866]_" + seq + "_[01].png"

	wantSummary := "同形序列成员（[作者]_[作品号]_任意段_序号 形态）→ 作品集 = 段组合『" + seq + "』，仅序列成员产出"
	if sugg.Summary != wantSummary {
		t.Fatalf("摘要行不符:\n取回 %q\n期望 %q", sugg.Summary, wantSummary)
	}
	if len(sugg.Mappings) != 1 || sugg.Mappings[0].Dimension != string(DimCollection) ||
		sugg.Mappings[0].From != "文件名第 3..末−1 段（含分隔符）" {
		t.Fatalf("映射行不符: %+v", sugg.Mappings)
	}
	if sugg.RelPath != seq {
		t.Fatalf("目录寻址建议卡 RelPath 应呈现目录键: %q", sugg.RelPath)
	}
	if len(sugg.Rules) != 2 {
		t.Fatalf("序列形态建议应恰两条（sibling 形态跟随 + 集名捕获），实际 %d 条", len(sugg.Rules))
	}

	// 条目模式形态（建议与入档共用构造）：sibling 形态跟随（括号序号提取式）
	// + 集名捕获正则（无目录层锚定：模式即文件名正则，捕获组=段组合——类允许
	// 含分隔符，靠锚定尾段 `_[序号](扩展名)$` 收界）。
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	sampleName := "[hakosaku]_[126794866]_" + seq + "_[01].png"
	out := trySequenceFormSummary(sample, sampleName, &SessionAssignment{Collection: seq}, sess.Snapshot)
	if !out.matched || len(out.rules) != 2 {
		t.Fatalf("序列形态构造应成立且恰两条: matched=%v %d 条", out.matched, len(out.rules))
	}
	sibling, ok := out.rules[0].Pattern.(*SiblingPattern)
	if !ok {
		t.Fatalf("首条应为 sibling 形态跟随定群规则，实际 %T", out.rules[0].Pattern)
	}
	if sibling.KeyExpr != `^(.+?)_?\[\d{1,4}\](?:\.[^.]*)?$` || sibling.SeqExpr != `\[(\d{1,4})\](?:\.[^.]*)?$` {
		t.Fatalf("sibling 形态跟随提取式不符: %+v", sibling)
	}
	capture, ok := out.rules[1].Pattern.(*RegexPattern)
	if !ok {
		t.Fatalf("次条应为集名捕获正则规则，实际 %T", out.rules[1].Pattern)
	}
	wantExpr := `^\[[^\]]*\]_\[[^\]]*\]_(?P<collection>.+?)_\[\d{1,4}\](?:\.[^.]*)?$`
	if capture.Target != TargetFileName || capture.Expr != wantExpr || len(capture.Captures) != 1 || capture.Captures[0] != "collection" {
		t.Fatalf("集名捕获模式不符:\n取回 %s\n期望 %s", capture.Expr, wantExpr)
	}
	if len(out.rules[1].Extractions) != 1 || out.rules[1].Extractions[0].When == nil {
		t.Fatalf("捕获条目应恰一条带 when 原子的提取动作: %+v", out.rules[1].Extractions)
	}

	// 命中预览：本目录其余 2 成员 + 跨目录同形序列 2 成员；样例取跨目录在前。
	if sugg.HitCount != 4 || sugg.HitTotal != 10 {
		t.Fatalf("命中预览计数不符: hit=%d total=%d（期望 4/10）", sugg.HitCount, sugg.HitTotal)
	}
	if len(sugg.Samples) != 3 || sugg.Samples[0].DirRelKey != "别处" ||
		!strings.Contains(sugg.Samples[0].Result, "作品集=Y_Misskeyまとめ 2_2～2_8") {
		t.Fatalf("跨目录样例不符: %+v", sugg.Samples)
	}

	// 等价入口（步4）：点成员文件块进文件表单保存 → 同一张序列形态建议卡。
	if _, _, err := m.ApplyAssignment(root, sample, SessionAssignment{Collection: seq}); err != nil {
		t.Fatalf("单文件赋值失败: %v", err)
	}
	fileSugg, err := m.ApplyStructSummary(root, sample)
	if err != nil {
		t.Fatalf("单文件总结失败: %v", err)
	}
	if fileSugg.Summary != wantSummary || len(fileSugg.Rules) != 2 {
		t.Fatalf("文件入口应产出同一序列形态推导: %q（%d 条）", fileSugg.Summary, len(fileSugg.Rules))
	}
}

// TestSeqFormSuggestionWholeKey 集名=序号前整键（无作者/作品号前缀，如
// `路灯后辈_[01]`）：捕获=序号前整段，规则成立（A4 手工款自动出——形态+
// 捕获集名+提取+仅序列成员产出）。
func TestSeqFormSuggestionWholeKey(t *testing.T) {
	root := t.TempDir()
	m, _ := seqFormFixture(t, root)
	sugg := seqSuggOf(t, m, root, "路灯", "路灯后辈")

	if !strings.Contains(sugg.Summary, "同形序列成员（任意段_序号 形态）→ 作品集 = 段组合『路灯后辈』") {
		t.Fatalf("摘要行不符: %q", sugg.Summary)
	}
	if len(sugg.Mappings) != 1 || sugg.Mappings[0].From != "文件名第 1..末−1 段（含分隔符）" {
		t.Fatalf("映射行不符（整键捕获）: %+v", sugg.Mappings)
	}
	if len(sugg.Rules) != 2 {
		t.Fatalf("序列形态建议应恰两条，实际 %d 条", len(sugg.Rules))
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	out := trySequenceFormSummary("路灯/路灯后辈_[01].jpg", "路灯后辈_[01].jpg", &SessionAssignment{Collection: "路灯后辈"}, sess.Snapshot)
	if !out.matched || len(out.rules) != 2 {
		t.Fatalf("整键捕获构造应成立: matched=%v", out.matched)
	}
	capture, ok := out.rules[1].Pattern.(*RegexPattern)
	if !ok || capture.Expr != `^(?P<collection>.+?)_\[\d{1,4}\](?:\.[^.]*)?$` {
		t.Fatalf("整键捕获模式不符: %+v", out.rules[1].Pattern)
	}
}

// TestSeqFormSuggestionMultiElementSpan 集名=跨多元素的连续段组合（含元素间
// 分隔符——无目录提示保持整段时的常态形态）：组合区间覆盖多元素+分隔 run，
// 捕获类允许含分隔符、映射行呈段区间。
func TestSeqFormSuggestionMultiElementSpan(t *testing.T) {
	root := t.TempDir()
	m, _ := newSeqRecognitionFixture(t, root)
	writeTestFile(t, root+"/组合集/前段_后段_[01].jpg", "x")
	writeTestFile(t, root+"/组合集/前段_后段_[02].jpg", "y")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sugg := seqSuggOf(t, m, root, "组合集", "前段_后段")

	if !strings.Contains(sugg.Summary, "同形序列成员（任意段_序号 形态）→ 作品集 = 段组合『前段_后段』") {
		t.Fatalf("摘要行不符: %q", sugg.Summary)
	}
	if len(sugg.Mappings) != 1 || sugg.Mappings[0].From != "文件名第 1..末−1 段（含分隔符）" {
		t.Fatalf("映射行不符（跨段组合应呈段区间）: %+v", sugg.Mappings)
	}
	if len(sugg.Rules) != 2 {
		t.Fatalf("序列形态建议应恰两条，实际 %d 条", len(sugg.Rules))
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	out := trySequenceFormSummary("组合集/前段_后段_[01].jpg", "前段_后段_[01].jpg", &SessionAssignment{Collection: "前段_后段"}, sess.Snapshot)
	if !out.matched {
		t.Fatal("跨段组合匹配应成立")
	}
	capture, ok := out.rules[1].Pattern.(*RegexPattern)
	if !ok || capture.Expr != `^(?P<collection>.+?)_\[\d{1,4}\](?:\.[^.]*)?$` {
		t.Fatalf("跨段捕获模式不符（捕获类允许含分隔符）: %+v", out.rules[1].Pattern)
	}
}

// TestSeqFormSuggestionUnmatchedNote 集名对不上任何段组合：退回既有形态——
// 值=目录名时目录层映射照旧 + 附说明行「集名无法对应文件名段落，未生成提取式
// 规则」；值纯手打（无任何结构段可映射）时建议卡退回手工指定形态 + 同一说明行。
func TestSeqFormSuggestionUnmatchedNote(t *testing.T) {
	root := t.TempDir()
	m, _ := seqFormFixture(t, root)

	// 值=目录名（目录层映射照旧）+ 说明行；不产出序列形态条目。
	sugg := seqSuggOf(t, m, root, "集名目录", "集名目录")
	if strings.Contains(sugg.Summary, "同形序列成员") {
		t.Fatalf("对不上段组合不应产出序列形态摘要: %q", sugg.Summary)
	}
	if !strings.Contains(sugg.Summary, "一级目录") {
		t.Fatalf("既有目录层映射形态应照旧: %q", sugg.Summary)
	}
	if len(sugg.Notes) != 1 || sugg.Notes[0] != seqNameUnmatchedNote {
		t.Fatalf("应附说明行: %+v", sugg.Notes)
	}
	for _, r := range sugg.Rules {
		if strings.Contains(r.ID, "qna-seqcap") {
			t.Fatalf("不应产出集名捕获条目: %+v", r)
		}
	}

	// 值纯手打：建议卡退回手工指定形态 + 说明行（无将入档条目）。
	sugg = seqSuggOf(t, m, root, "路灯", "纯手打集名")
	if !strings.Contains(sugg.Summary, "手工指定") || len(sugg.Rules) != 0 {
		t.Fatalf("纯手打应退回手工指定形态且无条目: %+v", sugg)
	}
	if len(sugg.Notes) != 1 || sugg.Notes[0] != seqNameUnmatchedNote {
		t.Fatalf("纯手打应附说明行: %+v", sugg.Notes)
	}
	if len(sugg.ManualDims) != 1 || !strings.Contains(sugg.ManualDims[0], "作品集=纯手打集名") {
		t.Fatalf("手工维度呈现不符: %+v", sugg.ManualDims)
	}
}

// TestSeqFormSuggestionNonGroupSample 样本不属任何序列群（散文件）：建议卡=
// 既有形态，完全不变（无序列摘要、无说明行）。
func TestSeqFormSuggestionNonGroupSample(t *testing.T) {
	root := t.TempDir()
	m, _ := seqFormFixture(t, root)
	rel := "散行/孤本_007.jpg"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{Collection: "散行"}); err != nil {
		t.Fatalf("单文件赋值失败: %v", err)
	}
	sugg, err := m.ApplyStructSummary(root, rel)
	if err != nil {
		t.Fatalf("单文件总结失败: %v", err)
	}
	if strings.Contains(sugg.Summary, "同形序列成员") || len(sugg.Notes) != 0 {
		t.Fatalf("非群样本应完全走既有形态: %q %+v", sugg.Summary, sugg.Notes)
	}
	if !strings.Contains(sugg.Summary, "一级目录") {
		t.Fatalf("既有目录层映射形态应照旧: %q", sugg.Summary)
	}
}

// TestSeqFormGeneralizeEndToEnd 泛化入档与接管（剧本A 步3）：generalize
// mode=struct 入档 sibling+集名捕获两条（建议即所见即所入）；when 原子
// 「群组序号 ≥1」随捕获条目入档，档案 JSON 形态与规则编辑面「仅序列成员产出」
// 复选的判读口径一致（kind=range、operand=groupSeq、min=1、无上界）；消费
// 剥离后集名赋值整组删除；验证轮重识别后本目录成员与跨目录样例的集名声明
// 来源=新捕获规则（跨目录接管）。
func TestSeqFormGeneralizeEndToEnd(t *testing.T) {
	root := t.TempDir()
	m, seq := seqFormFixture(t, root)
	if _, _, err := m.ApplyDirAssignment(root, seq, SessionAssignment{Collection: seq}); err != nil {
		t.Fatalf("目录赋值失败: %v", err)
	}

	result, err := m.ApplyDirGeneralization(root, seq, generalizeModeStruct)
	if err != nil {
		t.Fatalf("泛化入档失败: %v", err)
	}
	if len(result.Rules) != 2 {
		t.Fatalf("入档条目应恰两条（sibling+集名捕获），实际 %d", len(result.Rules))
	}

	profile, errs := LoadProfile(m.store)
	if len(errs) > 0 || profile == nil {
		t.Fatalf("载入档案失败: %v", profile)
	}
	var capture *RuleEntry
	siblingCount := 0
	for i := range profile.Rules {
		r := &profile.Rules[i]
		if strings.HasPrefix(r.ID, "qna-seq-") {
			siblingCount++
			if _, ok := r.Pattern.(*SiblingPattern); !ok {
				t.Fatalf("sibling 条目模式不符: %T", r.Pattern)
			}
			continue
		}
		if strings.HasPrefix(r.ID, "qna-seqcap-") {
			capture = r
		}
	}
	if siblingCount != 1 || capture == nil {
		t.Fatalf("档案应含 sibling 与集名捕获各一条: sibling=%d capture=%v", siblingCount, capture != nil)
	}

	// when 原子随规则入档：档案 JSON 形态与编辑面复选判读（checkSeqAtom）一致。
	if len(capture.Extractions) != 1 || capture.Extractions[0].When == nil {
		t.Fatalf("捕获条目应恰一条带 when 原子的提取动作: %+v", capture.Extractions)
	}
	when := capture.Extractions[0].When
	if when.Kind != ExprRange || when.Operand == nil || when.Operand.Kind != ExprGroupSeq ||
		when.Min == nil || *when.Min != 1 || when.Max != nil {
		t.Fatalf("when 原子形态不符（应为 groupSeq ≥1、无上界）: %+v", when)
	}
	raw, err := json.Marshal(when)
	if err != nil {
		t.Fatalf("序列化 when 原子失败: %v", err)
	}
	if string(raw) != `{"kind":"range","operand":{"kind":"groupSeq"},"min":1}` {
		t.Fatalf("when 原子档案 JSON 不符: %s", raw)
	}

	// 消费剥离：本目录三成员的集名赋值被新规则完整复现 → 赋值整组删除。
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	for _, n := range []string{"01", "02", "03"} {
		rel := seq + "/[hakosaku]_[126794866]_" + seq + "_[" + n + "].png"
		if _, hit := sess.Assignments[rel]; hit {
			t.Fatalf("集名赋值应被消费剥离: %s", rel)
		}
	}

	// 验证轮已排定（异步增量轮与 ApplyDirAssignment 排定的上一轮存在时序竞
	// 态，声明来源断言以确定性的全量重识别为准）。
	if !result.VerificationStarted {
		t.Fatal("泛化入档后应排定验证轮")
	}

	// 全量重识别：本目录成员的集名声明来源=新捕获规则；跨目录同形序列的接管
	// （序列形态的本质差异——无目录层锚定）与同形落单文件的不产出（仅序列
	// 成员产出）在同一口径下断言。
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量重识别失败: %v", err)
	}
	sess, err = m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	for rel, want := range map[string]string{
		seq + "/[hakosaku]_[126794866]_" + seq + "_[01].png":  seq,
		"别处/[moka]_[987654321]_Y_Misskeyまとめ 2_2～2_8_[05].jpg": "Y_Misskeyまとめ 2_2～2_8",
		"别处/[moka]_[987654321]_Y_Misskeyまとめ 2_2～2_8_[06].jpg": "Y_Misskeyまとめ 2_2～2_8",
	} {
		file := snapshotFileOf(sess.Snapshot, rel)
		if file == nil {
			t.Fatalf("快照内无文件 %s", rel)
		}
		found := false
		for _, decl := range file.Final {
			if decl.Dimension == string(DimCollection) && decl.Value == want {
				if decl.RuleID != capture.ID {
					t.Fatalf("%s 集名声明来源应为新规则 %s，实际 %q", rel, capture.ID, decl.RuleID)
				}
				found = true
			}
		}
		if !found {
			t.Fatalf("%s 应有集名声明 %q（终选: %+v）", rel, want, file.Final)
		}
	}
	// 同形落单文件（散行/孤本_007.jpg）不产出集名声明——仅序列成员产出。
	lone := snapshotFileOf(sess.Snapshot, "散行/孤本_007.jpg")
	if lone == nil {
		t.Fatalf("快照内无落单文件")
	}
	for _, decl := range lone.Final {
		if decl.RuleID == capture.ID {
			t.Fatalf("落单文件不应有序列集名声明: %+v", decl)
		}
	}
}
