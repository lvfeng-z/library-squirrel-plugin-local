package main

// 目录赋值通路与泛化赋值保留的单元测试：assignDir 子树展开合并写（非空维度
// 覆盖/空维度不动/出处随维度写入/应答计数）、空目录与不在全景目录的错误路径、
// clearDirAssignments 计数与清空、泛化后赋值保留（双源互证——2026-10-25 用户
// 裁决废除消费剥离；单文件与子树两作用域）、裁决生成物不标记、
// 目录寻址的总结与泛化分派。
import (
	"os"
	"strings"
	"testing"
)

// dirAssignFixture 目录赋值测试环境：样例树 + 目录子树（Artist[X] 直下与嵌套
// sub）+ 子树外目录 + 空目录，全量识别完成。
func dirAssignFixture(t *testing.T, root string) (*SessionManager, *workbenchTestCtx) {
	t.Helper()
	m, ctx := newAdjudicationFixture(t, root)
	writeTestFile(t, root+"/Artist[X]/a__x.png", "a")
	writeTestFile(t, root+"/Artist[X]/sub/b__y.png", "b")
	writeTestFile(t, root+"/别处/c.png", "c")
	if err := os.MkdirAll(root+"/空目录", 0o755); err != nil {
		t.Fatalf("创建空目录失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	return m, ctx
}

// TestAssignDirSubtreeMergeWrite 目录赋值：子树展开逐文件合并写——非空维度
// 覆盖该文件赋值同维度（值+出处整组）、空维度不动既有维度；应答计数=子树
// 文件数；子树外文件不受影响；空维度全空/目录不在全景/空目录回可读错误。
func TestAssignDirSubtreeMergeWrite(t *testing.T) {
	root := t.TempDir()
	m, _ := dirAssignFixture(t, root)

	// 子目录文件先有标题赋值（合并写不得动它）
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/sub/b__y.png", SessionAssignment{Title: "已有标题"}); err != nil {
		t.Fatalf("写入既有赋值失败: %v", err)
	}

	sess, affected, err := m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{
		Author: "X", AuthorClueRef: "dirLevelStripped1",
		Tags: []string{"目录标签"}, TagClueRefs: []string{"rootName"},
	})
	if err != nil {
		t.Fatalf("目录赋值失败: %v", err)
	}
	if affected != 2 {
		t.Fatalf("生效文件数 = %d，期望 2（直下+嵌套）", affected)
	}
	a := sess.Assignments["Artist[X]/a__x.png"]
	if a.Author != "X" || a.AuthorClueRef != "dirLevelStripped1" ||
		len(a.Tags) != 1 || a.Tags[0] != "目录标签" || len(a.TagClueRefs) != 1 || a.TagClueRefs[0] != "rootName" {
		t.Fatalf("子树直下文件赋值不符: %+v", a)
	}
	b := sess.Assignments["Artist[X]/sub/b__y.png"]
	if b.Title != "已有标题" || b.Author != "X" || len(b.Tags) != 1 {
		t.Fatalf("嵌套文件应合并写（空维度不动既有标题）: %+v", b)
	}
	if _, hit := sess.Assignments["别处/c.png"]; hit {
		t.Fatal("子树外文件不应被目录赋值触及")
	}

	// 二次赋值：仅作品集非空 → 作者/标题/标签（空维度）一律不动
	sess, affected, err = m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{
		Collection: "集甲", CollectionClueRef: "dirLevel1",
	})
	if err != nil || affected != 2 {
		t.Fatalf("二次目录赋值不符: %v %d", err, affected)
	}
	b2 := sess.Assignments["Artist[X]/sub/b__y.png"]
	if b2.Collection != "集甲" || b2.CollectionClueRef != "dirLevel1" ||
		b2.Title != "已有标题" || b2.Author != "X" || len(b2.Tags) != 1 {
		t.Fatalf("空维度应不动既有赋值: %+v", b2)
	}

	// 非空维度整组覆盖：换作者时旧出处一并替换
	sess, _, err = m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{Author: "Y"})
	if err != nil {
		t.Fatalf("目录赋值（换作者）失败: %v", err)
	}
	a3 := sess.Assignments["Artist[X]/a__x.png"]
	if a3.Author != "Y" || a3.AuthorClueRef != "" || len(a3.Tags) != 1 || a3.Collection != "集甲" {
		t.Fatalf("作者维度应整组覆盖、其余维度不动: %+v", a3)
	}

	// 错误路径：全空维度 / 目录不在全景 / 空目录
	if _, _, err := m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{}); err == nil ||
		!strings.Contains(err.Error(), "未指定任何维度") {
		t.Fatalf("全空维度应回可读错误: %v", err)
	}
	if _, _, err := m.ApplyDirAssignment(root, "不存在", SessionAssignment{Author: "甲"}); err == nil ||
		!strings.Contains(err.Error(), "不在当前识别全景") {
		t.Fatalf("不在全景的目录应回可读错误: %v", err)
	}
	if _, _, err := m.ApplyDirAssignment(root, "空目录", SessionAssignment{Author: "甲"}); err == nil ||
		!strings.Contains(err.Error(), "该目录下没有文件") {
		t.Fatalf("空目录应回可读错误: %v", err)
	}
}

// TestClearDirAssignmentsCounts 清除目录：删除子树内全部赋值并回清除数，子树外
// 赋值保留；重复清除计数为零；目录不在全景回可读错误。
func TestClearDirAssignmentsCounts(t *testing.T) {
	root := t.TempDir()
	m, _ := dirAssignFixture(t, root)

	if _, _, err := m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{Author: "X"}); err != nil {
		t.Fatalf("目录赋值失败: %v", err)
	}
	if _, _, err := m.ApplyAssignment(root, "别处/c.png", SessionAssignment{Author: "外"}); err != nil {
		t.Fatalf("子树外赋值失败: %v", err)
	}

	sess, cleared, err := m.ClearDirAssignments(root, "Artist[X]")
	if err != nil {
		t.Fatalf("清除目录赋值失败: %v", err)
	}
	if cleared != 2 {
		t.Fatalf("清除数 = %d，期望 2", cleared)
	}
	if _, hit := sess.Assignments["Artist[X]/a__x.png"]; hit {
		t.Fatal("子树直下文件赋值应被清除")
	}
	if _, hit := sess.Assignments["Artist[X]/sub/b__y.png"]; hit {
		t.Fatal("嵌套文件赋值应被清除")
	}
	if outer := sess.Assignments["别处/c.png"]; outer.Author != "外" {
		t.Fatalf("子树外赋值应保留: %+v", outer)
	}

	if _, cleared, err = m.ClearDirAssignments(root, "Artist[X]"); err != nil || cleared != 0 {
		t.Fatalf("重复清除应为零计数: %v %d", err, cleared)
	}
	if _, _, err := m.ClearDirAssignments(root, "不存在"); err == nil ||
		!strings.Contains(err.Error(), "不在当前识别全景") {
		t.Fatalf("不在全景的目录应回可读错误: %v", err)
	}
}

// TestGeneralizeKeepsAssignments 泛化后赋值保留（2026-10-25 用户裁决：废除
// 「泛化即接管」消费剥离）：规则入档但赋值全量保留（双源互证——赋值显式维度
// 继续覆盖规则终选），Generalized 仅作已参与泛化标记；序列定群同样保留。
func TestGeneralizeKeepsAssignments(t *testing.T) {
	root := t.TempDir()
	m, _ := dirAssignFixture(t, root)

	// 复现维度保留 + 标记：文件名形态规则捕获复现作者/标题
	rel := "已识别/【作者A】标题1.jpg"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{
		Author: "作者A", Title: "标题1", Collection: "手工集",
	}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if _, err := m.ApplyGeneralization(root, rel, generalizeModeFilename); err != nil {
		t.Fatalf("文件名形态泛化失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	assign, ok := sess.Assignments[rel]
	if !ok {
		t.Fatal("泛化后赋值应全量保留（不剥离不删除）")
	}
	if assign.Author != "作者A" || assign.Title != "标题1" || assign.Collection != "手工集" {
		t.Fatalf("全部维度（含被规则复现者）应保留: %+v", assign)
	}
	if !assign.Generalized {
		t.Fatalf("赋值应置已泛化标记: %+v", assign)
	}

	// 目录层泛化：同样保留（原「全剥空→删除」语义废除）
	rel2 := "冲突/AB.jpg"
	if _, _, err := m.ApplyAssignment(root, rel2, SessionAssignment{Title: "协议标题"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if _, err := m.ApplyGeneralization(root, rel2, generalizeModeDirLayer); err != nil {
		t.Fatalf("目录层泛化失败: %v", err)
	}
	if sess, err = m.SessionOf(root); err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if a2 := sess.Assignments[rel2]; a2.Title != "协议标题" || !a2.Generalized {
		t.Fatalf("目录层泛化后赋值应保留并标记: %+v", a2)
	}

	// 序列定群：保留 + 标记
	rel3 := "序列/库内作者名_002.jpg"
	if _, _, err := m.ApplyAssignment(root, rel3, SessionAssignment{Collection: "库内作者名"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if _, err := m.ApplyGeneralization(root, rel3, generalizeModeSequence); err != nil {
		t.Fatalf("序列形态泛化失败: %v", err)
	}
	if sess, err = m.SessionOf(root); err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if seq := sess.Assignments[rel3]; seq.Collection != "库内作者名" || !seq.Generalized {
		t.Fatalf("定群泛化后赋值应保留并标记: %+v", seq)
	}
}

// TestGeneralizeDirScopeMarksAssignments 泛化（子树作用域）：目录寻址泛化以
// 子树首个有赋值文件为样本，标记作用于子树全部文件——赋值全量保留（双源互
// 证）、子树外赋值不受影响（无标记）。
func TestGeneralizeDirScopeMarksAssignments(t *testing.T) {
	root := t.TempDir()
	m, _ := newAdjudicationFixture(t, root)
	writeTestFile(t, root+"/Artist[X]/a__title__code.png", "a")
	writeTestFile(t, root+"/Artist[X]/c__code.png", "c")
	writeTestFile(t, root+"/Artist[X]/sub/b__code.png", "b")
	writeTestFile(t, root+"/其他/out.png", "d")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}

	// 样本（子树首个有赋值文件）作者=剥壳目录内名、作品名=文件名首段、手工集不参与
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/a__title__code.png", SessionAssignment{
		Author: "X", AuthorClueRef: "dirLevelStripped1",
		Title: "a", TitleClueRef: "filenameSeg0",
		Collection: "手填集",
	}); err != nil {
		t.Fatalf("写入样本赋值失败: %v", err)
	}
	// 子树内其余文件：c 仅作者（将被规则复现）、嵌套 b 作者异值
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/c__code.png", SessionAssignment{Author: "X"}); err != nil {
		t.Fatalf("写入 c 赋值失败: %v", err)
	}
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/sub/b__code.png", SessionAssignment{Author: "别的作者"}); err != nil {
		t.Fatalf("写入 b 赋值失败: %v", err)
	}
	if _, _, err := m.ApplyAssignment(root, "其他/out.png", SessionAssignment{Author: "外面"}); err != nil {
		t.Fatalf("写入子树外赋值失败: %v", err)
	}

	result, err := m.ApplyDirGeneralization(root, "Artist[X]", generalizeModeStruct)
	if err != nil {
		t.Fatalf("目录寻址泛化失败: %v", err)
	}
	if result.Rule == nil || len(result.Rules) < 2 {
		t.Fatalf("组合规则应至少两条（目录层+文件名）: %+v", result)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	sample := sess.Assignments["Artist[X]/a__title__code.png"]
	if sample.Author != "X" || sample.Title != "a" || sample.Collection != "手填集" || !sample.Generalized {
		t.Fatalf("样本赋值应全量保留（含被规则复现维度）并置标记: %+v", sample)
	}
	c := sess.Assignments["Artist[X]/c__code.png"]
	if c.Author != "X" || !c.Generalized {
		t.Fatalf("仅复现维度的文件赋值也应保留并置标记: %+v", c)
	}
	nested := sess.Assignments["Artist[X]/sub/b__code.png"]
	if nested.Author != "别的作者" || !nested.Generalized {
		t.Fatalf("子树内异值赋值应保留并置标记: %+v", nested)
	}
	if outer := sess.Assignments["其他/out.png"]; outer.Author != "外面" || outer.Generalized {
		t.Fatalf("子树外赋值不应被标记: %+v", outer)
	}
}

// TestAdjudicateDoesNotConsumeAssignments 裁决生成物不消费赋值（负断言）：裁决
// 规则声明的值与赋值一致时，赋值仍在场且不带已泛化标记。
func TestAdjudicateDoesNotConsumeAssignments(t *testing.T) {
	root := t.TempDir()
	m, _ := dirAssignFixture(t, root)

	rel := "冲突/AB.jpg"
	if _, _, err := m.ApplyAssignment(root, rel, SessionAssignment{Title: "A"}); err != nil {
		t.Fatalf("写入赋值失败: %v", err)
	}
	if _, err := m.ApplyAdjudication(root, rel, DimTitle, adjudicationInput{
		Mode: adjudicateModePick, PickIndex: 0,
	}); err != nil {
		t.Fatalf("裁决失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	assign, ok := sess.Assignments[rel]
	if !ok || assign.Title != "A" || assign.Generalized {
		t.Fatalf("裁决生成物不应消费赋值: %+v（在场 %v）", assign, ok)
	}
}

// TestHandleFrontendRequestDirActions 前端协议分派：assignDir 应答带生效文件数
// 与决策态、summarizeRule 目录寻址建议卡 RelPath 呈现目录键、clearDirAssignments
// 应答带清除数、generalize 目录寻址目录不在全景时回可读错误。
func TestHandleFrontendRequestDirActions(t *testing.T) {
	root := t.TempDir()
	m, ctx := dirAssignFixture(t, root)

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 40, Action: workbenchActionAssignDir, Root: root, DirRelKey: "Artist[X]",
		Author: "X", AuthorClueRef: "dirLevelStripped1",
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Affected != 2 || resp.Decisions == nil || len(resp.Decisions.Assignments) != 2 {
		t.Fatalf("assignDir 应答不符: %+v", resp)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 41, Action: workbenchActionSummarizeRule, Root: root, DirRelKey: "Artist[X]",
	}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.RuleSuggestion == nil {
		t.Fatalf("summarizeRule 目录寻址应答不符: %+v", resp)
	}
	if resp.RuleSuggestion.RelPath != "Artist[X]" {
		t.Fatalf("目录寻址建议卡 RelPath 应呈现目录键: %q", resp.RuleSuggestion.RelPath)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 42, Action: workbenchActionClearDirAssignments, Root: root, DirRelKey: "Artist[X]",
	}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.Affected != 2 || resp.Decisions == nil || len(resp.Decisions.Assignments) != 0 {
		t.Fatalf("clearDirAssignments 应答不符: %+v", resp)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 43, Action: workbenchActionGeneralize, Root: root, DirRelKey: "不存在", GeneralizeMode: generalizeModeStruct,
	}))
	if resp = ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "不在当前识别全景") {
		t.Fatalf("目录寻址泛化目录不在全景应回可读错误: %+v", resp)
	}
}
