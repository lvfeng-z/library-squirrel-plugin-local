package main

// 目录赋值通路与泛化赋值保留的单元测试（条目化写点，阶段2）：assignDir 写
// 目录条目+扇出合并进更深条目（非空维度覆盖/空维度不动/出处随维度写入/应答
// 计数=子树文件数）、扇出等价（浅层后作用覆盖更深层既有条目）、空目录与不在
// 全景目录的错误路径、clearDirAssignments 删本层及更深条目+计数=子树内清除前
// 有效赋值非空文件数+应答范围判定增补（survivorDirKey/survivorDims）、泛化后
// 赋值保留（双源互证——2026-10-25 用户裁决废除消费剥离；单文件与子树两作用
// 域；标记置供值条目）、裁决生成物不标记、目录寻址的总结与泛化分派。
// 断言形态：值口径全部经 resolveFileAssignment 就近解析取值（条目化后直查
// 键位不再与写形态对应，解析断言=值语义保持的证明）；另辅条目结构断言锚定
// 新表示形态（目录条目键在场、例外文件键不因目录赋值新造）。
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

// resolveOf 会话赋值的就近解析取值（测试断言简写）。
func resolveOf(sess *WorkbenchSession, relPath string) SessionAssignment {
	return resolveFileAssignment(sess.Assignments, relPath)
}

// TestAssignDirSubtreeMergeWrite 目录赋值（条目化写）：写一个目录条目+扇出合并
// 进更深条目——非空维度覆盖该文件赋值同维度（值+出处整组）、空维度不动既有
// 维度；应答计数=子树文件数；子树外文件不受影响；无例外的子树文件不再逐文件
// 新造条目（目录条目直接供值）；空维度全空/目录不在全景/空目录回可读错误。
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
	a := resolveOf(sess, "Artist[X]/a__x.png")
	if a.Author != "X" || a.AuthorClueRef != "dirLevelStripped1" ||
		len(a.Tags) != 1 || a.Tags[0] != "目录标签" || len(a.TagClueRefs) != 1 || a.TagClueRefs[0] != "rootName" {
		t.Fatalf("子树直下文件赋值不符: %+v", a)
	}
	b := resolveOf(sess, "Artist[X]/sub/b__y.png")
	if b.Title != "已有标题" || b.Author != "X" || len(b.Tags) != 1 {
		t.Fatalf("嵌套文件应合并写（空维度不动既有标题）: %+v", b)
	}
	// 条目结构锚定：目录条目在场供值，无例外的直下文件不再逐文件新造条目
	if entry, ok := sess.Assignments["Artist[X]/"]; !ok || entry.Author != "X" {
		t.Fatalf("目录条目应在场供值: %+v（在场 %v）", entry, ok)
	}
	if _, ok := sess.Assignments["Artist[X]/a__x.png"]; ok {
		t.Fatal("无例外的子树文件不应新造文件条目")
	}
	if _, ok := sess.Assignments["别处/c.png"]; ok {
		t.Fatal("子树外文件不应被目录赋值触及")
	}
	if c := resolveOf(sess, "别处/c.png"); len(c.coveredDimensions()) != 0 {
		t.Fatalf("子树外文件解析应为空: %+v", c)
	}

	// 二次赋值：仅作品集非空 → 作者/标题/标签（空维度）一律不动
	sess, affected, err = m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{
		Collection: "集甲", CollectionClueRef: "dirLevel1",
	})
	if err != nil || affected != 2 {
		t.Fatalf("二次目录赋值不符: %v %d", err, affected)
	}
	b2 := resolveOf(sess, "Artist[X]/sub/b__y.png")
	if b2.Collection != "集甲" || b2.CollectionClueRef != "dirLevel1" ||
		b2.Title != "已有标题" || b2.Author != "X" || len(b2.Tags) != 1 {
		t.Fatalf("空维度应不动既有赋值: %+v", b2)
	}

	// 非空维度整组覆盖：换作者时旧出处一并替换
	sess, _, err = m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{Author: "Y"})
	if err != nil {
		t.Fatalf("目录赋值（换作者）失败: %v", err)
	}
	a3 := resolveOf(sess, "Artist[X]/a__x.png")
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

// TestApplyDirAssignmentFanout 扇出等价（红绿锚定，方案风险1）：目录 D 已有
// 更深条目（子目录目录条目、直下文件例外，均带相异作者值）→ 对 D 施目录赋值
// → 更深条目的作者维被后作用覆盖（现语义：后作用显式维度覆盖全子树每文件），
// 异维（作品名/作品集）不被扇出清除；更深条目本体保留（合并写入而非删除）。
// 无扇出实现下深层条目会以「就近」挡掉浅层覆盖，本用例即红。
func TestApplyDirAssignmentFanout(t *testing.T) {
	root := t.TempDir()
	m, _ := dirAssignFixture(t, root)

	// 深层在前：子目录目录条目 + 直下文件例外（相异作者值）
	if _, _, err := m.ApplyDirAssignment(root, "Artist[X]/sub", SessionAssignment{
		Author: "深作者", Title: "深层标题",
	}); err != nil {
		t.Fatalf("子目录赋值失败: %v", err)
	}
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/a__x.png", SessionAssignment{
		Author: "旧作者", Collection: "例外集",
	}); err != nil {
		t.Fatalf("写入文件例外失败: %v", err)
	}

	sess, affected, err := m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{Author: "新作者"})
	if err != nil {
		t.Fatalf("浅层目录赋值失败: %v", err)
	}
	if affected != 2 {
		t.Fatalf("生效文件数 = %d，期望 2", affected)
	}
	// 深层目录条目被扇出覆盖（无扇出则 b 以就近解析取「深作者」）
	b := resolveOf(sess, "Artist[X]/sub/b__y.png")
	if b.Author != "新作者" || b.Title != "深层标题" {
		t.Fatalf("更深目录条目的作者维应被后作用覆盖、异维保留: %+v", b)
	}
	// 文件例外被扇出覆盖（无扇出则 a 以就近解析取「旧作者」），异维保留
	a := resolveOf(sess, "Artist[X]/a__x.png")
	if a.Author != "新作者" || a.Collection != "例外集" {
		t.Fatalf("文件例外的作者维应被后作用覆盖、异维保留: %+v", a)
	}
	// 更深条目本体保留：扇出是合并写入，深层条目仍是就近供值者
	if entry, ok := sess.Assignments["Artist[X]/sub/"]; !ok || entry.Author != "新作者" || entry.Title != "深层标题" {
		t.Fatalf("更深目录条目应保留并被合并覆盖: %+v（在场 %v）", entry, ok)
	}
	if entry, ok := sess.Assignments["Artist[X]/a__x.png"]; !ok || entry.Author != "新作者" || entry.Collection != "例外集" {
		t.Fatalf("文件例外应保留并被合并覆盖: %+v（在场 %v）", entry, ok)
	}
}

// TestClearDirAssignmentsCounts 清除目录（条目化删）：删本层目录条目与其下所有
// 更深条目并回清除数（子树内清除前有效赋值非空的文件数），子树外赋值保留；
// 重复清除计数为零；目录不在全景回可读错误。
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
	if a := resolveOf(sess, "Artist[X]/a__x.png"); len(a.coveredDimensions()) != 0 {
		t.Fatalf("子树直下文件赋值应被清除: %+v", a)
	}
	if b := resolveOf(sess, "Artist[X]/sub/b__y.png"); len(b.coveredDimensions()) != 0 {
		t.Fatalf("嵌套文件赋值应被清除: %+v", b)
	}
	// 条目结构锚定：本层目录条目一并删除
	if _, ok := sess.Assignments["Artist[X]/"]; ok {
		t.Fatal("本层目录条目应被删除")
	}
	if outer := resolveOf(sess, "别处/c.png"); outer.Author != "外" {
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

// TestClearDirSurvivorResponse 清除目录的应答范围判定增补（方案帧D）：嵌套
// 场景（上级目录赋值在场）清子目录——本层条目删除、上级赋值继续供值，应答
// 携带最近供值上级的目录条目键（survivorDirKey）与在场维度标签（survivorDims）；
// 无上级赋值在场时字段缺省（维持现文案）。清除计数含仅由上级目录条目覆盖的
// 文件（清除前有效赋值非空口径）。
func TestClearDirSurvivorResponse(t *testing.T) {
	root := t.TempDir()
	m, ctx := dirAssignFixture(t, root)

	if _, _, err := m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{Author: "X"}); err != nil {
		t.Fatalf("上级目录赋值失败: %v", err)
	}
	if _, _, err := m.ApplyDirAssignment(root, "Artist[X]/sub", SessionAssignment{Title: "子题"}); err != nil {
		t.Fatalf("子目录赋值失败: %v", err)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 50, Action: workbenchActionClearDirAssignments, Root: root, DirRelKey: "Artist[X]/sub",
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Affected != 1 {
		t.Fatalf("嵌套清除应答不符（子树 1 文件、清除前有上级供值）: %+v", resp)
	}
	if resp.SurvivorDirKey != "Artist[X]/" || len(resp.SurvivorDims) != 1 || resp.SurvivorDims[0] != "作者" {
		t.Fatalf("应答应携带上级目录键与在场维度: key=%q dims=%v", resp.SurvivorDirKey, resp.SurvivorDims)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	// 上级赋值继续供值（行为细化）：作者仍在、本层作品名已剥
	if b := resolveOf(sess, "Artist[X]/sub/b__y.png"); b.Author != "X" || b.Title != "" {
		t.Fatalf("清除后上级赋值应继续供值、本层维度已剥: %+v", b)
	}
	if _, ok := sess.Assignments["Artist[X]/sub/"]; ok {
		t.Fatal("本层目录条目应被删除")
	}

	// 无上级赋值在场：字段缺省（子树整体回到未赋值，维持现文案）
	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 51, Action: workbenchActionClearDirAssignments, Root: root, DirRelKey: "Artist[X]",
	}))
	resp = ctx.lastResponse(t)
	if !resp.OK || resp.Affected != 2 || resp.SurvivorDirKey != "" || resp.SurvivorDims != nil {
		t.Fatalf("无上级在场应答应缺省范围判定字段: %+v", resp)
	}
	if sess, err = m.SessionOf(root); err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	if a := resolveOf(sess, "Artist[X]/a__x.png"); len(a.coveredDimensions()) != 0 {
		t.Fatalf("无上级清除后子树应整体未赋值: %+v", a)
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
	assign := resolveOf(sess, rel)
	if len(assign.coveredDimensions()) == 0 {
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
	if a2 := resolveOf(sess, rel2); a2.Title != "协议标题" || !a2.Generalized {
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
	if seq := resolveOf(sess, rel3); seq.Collection != "库内作者名" || !seq.Generalized {
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
	sample := resolveOf(sess, "Artist[X]/a__title__code.png")
	if sample.Author != "X" || sample.Title != "a" || sample.Collection != "手填集" || !sample.Generalized {
		t.Fatalf("样本赋值应全量保留（含被规则复现维度）并置标记: %+v", sample)
	}
	c := resolveOf(sess, "Artist[X]/c__code.png")
	if c.Author != "X" || !c.Generalized {
		t.Fatalf("仅复现维度的文件赋值也应保留并置标记: %+v", c)
	}
	nested := resolveOf(sess, "Artist[X]/sub/b__code.png")
	if nested.Author != "别的作者" || !nested.Generalized {
		t.Fatalf("子树内异值赋值应保留并置标记: %+v", nested)
	}
	if outer := resolveOf(sess, "其他/out.png"); outer.Author != "外面" || outer.Generalized {
		t.Fatalf("子树外赋值不应被标记: %+v", outer)
	}
}

// TestMarkScopedAssignmentsMarksSupplyingEntries 泛化标记置供值条目（条目化
// 写点）：作用域内每文件——有文件例外则标例外；无例外的文件标覆盖它的最近
// 目录条目（目录条目标记=子树整片「已参与泛化」，方案风险4 口径）。
func TestMarkScopedAssignmentsMarksSupplyingEntries(t *testing.T) {
	root := t.TempDir()
	m, _ := dirAssignFixture(t, root)

	// 目录条目供值全子树；仅样本文件带文件例外（总结器样本寻址仍为逐文件
	// 直查，读消费点阶段3 切解析器）
	if _, _, err := m.ApplyDirAssignment(root, "Artist[X]", SessionAssignment{
		Author: "X", AuthorClueRef: "dirLevelStripped1",
	}); err != nil {
		t.Fatalf("目录赋值失败: %v", err)
	}
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/a__x.png", SessionAssignment{Title: "x"}); err != nil {
		t.Fatalf("写入样本例外失败: %v", err)
	}
	if _, err := m.ApplyDirGeneralization(root, "Artist[X]", generalizeModeFilename); err != nil {
		t.Fatalf("目录寻址泛化失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	// 无例外的子树文件：供值目录条目被标记
	entry, ok := sess.Assignments["Artist[X]/"]
	if !ok || !entry.Generalized {
		t.Fatalf("供值目录条目应置已泛化标记: %+v（在场 %v）", entry, ok)
	}
	if b := resolveOf(sess, "Artist[X]/sub/b__y.png"); b.Author != "X" || !b.Generalized {
		t.Fatalf("目录条目覆盖的文件应呈现已泛化（谱系随供值条目）: %+v", b)
	}
	// 样本文件例外自身标记，且解析合目录供值
	if a := resolveOf(sess, "Artist[X]/a__x.png"); a.Title != "x" || a.Author != "X" || !a.Generalized {
		t.Fatalf("样本例外应保留并标记、作者维由目录条目供值: %+v", a)
	}
	if c := resolveOf(sess, "别处/c.png"); c.Generalized {
		t.Fatalf("子树外文件不应被标记: %+v", c)
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
	assign := resolveOf(sess, rel)
	if assign.Title != "A" || assign.Generalized {
		t.Fatalf("裁决生成物不应消费赋值: %+v", assign)
	}
}

// TestHandleFrontendRequestDirActions 前端协议分派：assignDir 应答带生效文件数
//（decisions 全量投影已从变更类动作应答剥离——大载荷断路器收口，恒不随答）、
// summarizeRule 目录寻址建议卡 RelPath 呈现目录键、clearDirAssignments 应答带
// 清除数（无上级赋值在场时范围判定字段缺省）、generalize 目录寻址目录不在
// 全景时回可读错误。
func TestHandleFrontendRequestDirActions(t *testing.T) {
	root := t.TempDir()
	m, ctx := dirAssignFixture(t, root)

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 40, Action: workbenchActionAssignDir, Root: root, DirRelKey: "Artist[X]",
		Author: "X", AuthorClueRef: "dirLevelStripped1",
	}))
	resp := ctx.lastResponse(t)
	if !resp.OK || resp.Affected != 2 || resp.Decisions != nil {
		t.Fatalf("assignDir 应答不符（decisions 应剥离）: %+v", resp)
	}

	// 总结器样本寻址仍为逐文件直查（读消费点阶段3 切解析器），置文件例外供样本
	if _, _, err := m.ApplyAssignment(root, "Artist[X]/a__x.png", SessionAssignment{Title: "x"}); err != nil {
		t.Fatalf("写入样本例外失败: %v", err)
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
	if !resp.OK || resp.Affected != 2 || resp.Decisions != nil {
		t.Fatalf("clearDirAssignments 应答不符（decisions 应剥离）: %+v", resp)
	}
	if resp.SurvivorDirKey != "" || resp.SurvivorDims != nil {
		t.Fatalf("无上级赋值在场时范围判定字段应缺省: %+v", resp)
	}

	m.HandleFrontendRequest(mustJSON(t, workbenchFrontendRequest{
		Seq: 43, Action: workbenchActionGeneralize, Root: root, DirRelKey: "不存在", GeneralizeMode: generalizeModeStruct,
	}))
	if resp = ctx.lastResponse(t); resp.OK || !strings.Contains(resp.Error, "不在当前识别全景") {
		t.Fatalf("目录寻址泛化目录不在全景应回可读错误: %+v", resp)
	}
}
