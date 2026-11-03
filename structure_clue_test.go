package main

// 结构分段线索集的单元测试：真实样例分段（含 Artist[...] 剥壳变体 / __ 分隔段 /
// 括号组原子段 / 已知名跨段混合形态 / 尾部码标记的保守判别）与剥壳集表驱动形态。
import (
	"fmt"
	"testing"
)

// clueByRef 按稳定标识取线索；不存在返回零值与假。
func clueByRef(clues []SnapStructClue, ref string) (SnapStructClue, bool) {
	for _, c := range clues {
		if c.Ref == ref {
			return c, true
		}
	}
	return SnapStructClue{}, false
}

// TestStructCluesOfRealSample 剧本G 真实样例：库根名 big belly +
// Artist[Jake--Jaccobus] 目录 + atago__commission__by_jake__jaccobus_delithw.png
// → 库根名 / 一级目录 / 剥壳变体 / 四个文件名段（已知名 jake__jaccobus 保持
// 整段）/ 尾部码 delithw。
func TestStructCluesOfRealSample(t *testing.T) {
	root := "E:/libs/big belly"
	rel := "Artist[Jake--Jaccobus]/atago__commission__by_jake__jaccobus_delithw.png"
	clues := StructCluesOf(root, rel, "atago__commission__by_jake__jaccobus_delithw.png")

	if len(clues) != 8 {
		t.Fatalf("线索数 = %d（%v），期望 8（库根名1+目录2+段4+尾部码1）", len(clues), clues)
	}
	want := map[string]struct {
		Label string
		Value string
	}{
		"rootName":          {"库根名", "big belly"},
		"dirLevel1":         {"一级目录", "Artist[Jake--Jaccobus]"},
		"dirLevelStripped1": {"一级目录剥壳", "Jake--Jaccobus"},
		"filenameSeg0":      {"文件名段", "atago"},
		"filenameSeg1":      {"文件名段", "commission"},
		"filenameSeg2":      {"文件名段", "by"},
		"filenameSeg3":      {"文件名段", "jake__jaccobus"},
		"tailCode":          {"尾部码", "delithw"},
	}
	for ref, w := range want {
		c, ok := clueByRef(clues, ref)
		if !ok {
			t.Fatalf("缺线索 %s（%v）", ref, clues)
		}
		if c.Label != w.Label || c.Value != w.Value {
			t.Fatalf("线索 %s 不符：Label=%q Value=%q，期望 %q %q", ref, c.Label, c.Value, w.Label, w.Value)
		}
		if c.Kind == "" || c.Ref == "" {
			t.Fatalf("线索 %s 缺类别/标识: %+v", ref, c)
		}
	}
}

// TestStructCluesMixedUnderscoreForm 混合形态样例：作品名可含单下划线词——
// across_the_counter_by 整体为一段（_ 为词内连接）、已知名 jake__jaccobus 跨
// __ 边界保持整段、尾部码 de1qt6f（含数字）。
func TestStructCluesMixedUnderscoreForm(t *testing.T) {
	rel := "Artist[Jake--Jaccobus]/across_the_counter_by_jake__jaccobus_de1qt6f.png"
	clues := StructCluesOf("E:/libs/big belly", rel, "across_the_counter_by_jake__jaccobus_de1qt6f.png")

	for ref, want := range map[string]string{
		"filenameSeg0": "across_the_counter_by",
		"filenameSeg1": "jake__jaccobus",
		"tailCode":     "de1qt6f",
	} {
		if c, ok := clueByRef(clues, ref); !ok || c.Value != want {
			t.Fatalf("线索 %s 应为 %q，实际 %+v（全集 %v）", ref, want, c, clues)
		}
	}
	if _, ok := clueByRef(clues, "filenameSeg2"); ok {
		t.Fatalf("尾部码不应重复计为文件名段: %v", clues)
	}
}

// TestStructCluesBracketAtomicSegments 括号组原子段：目录 Artist[路灯后辈] +
// [路灯后辈]_[136715041]_无题_[01].png → 括号内名各成段、段间正文（无 `__`）
// 按单个 `_` 分段，恰四段 路灯后辈/136715041/无题/01；剥壳线索值=路灯后辈且
// 标签=「一级目录剥壳」；末段 01 长度<5 不判尾部码。
func TestStructCluesBracketAtomicSegments(t *testing.T) {
	file := "[路灯后辈]_[136715041]_无题_[01].png"
	clues := StructCluesOf("E:/libs/图库", "Artist[路灯后辈]/"+file, file)
	for ref, want := range map[string]string{
		"filenameSeg0": "路灯后辈",
		"filenameSeg1": "136715041",
		"filenameSeg2": "无题",
		"filenameSeg3": "01",
	} {
		if c, ok := clueByRef(clues, ref); !ok || c.Value != want || c.Label != "文件名段" {
			t.Fatalf("线索 %s 应为 %q，实际 %+v（全集 %v）", ref, want, c, clues)
		}
	}
	if _, ok := clueByRef(clues, "filenameSeg4"); ok {
		t.Fatalf("分段应恰四段: %v", clues)
	}
	if c, ok := clueByRef(clues, "dirLevelStripped1"); !ok || c.Value != "路灯后辈" || c.Label != "一级目录剥壳" {
		t.Fatalf("剥壳线索应为 路灯后辈/「一级目录剥壳」: %+v", c)
	}
	if _, ok := clueByRef(clues, "tailCode"); ok {
		t.Fatalf("末段 01 长度<5 不应判尾部码: %v", clues)
	}
}

// TestStructCluesBracketMixedForms 混合形态：括号内名成段（内名取原文保留
// 大小写、段间正文取归一化形态——与既有 `__` 分段一致）；内层异对括号留在
// 内名不递归；含括号组的正文片段含 `__` 时仍按 `__` 分段。
func TestStructCluesBracketMixedForms(t *testing.T) {
	cases := []struct {
		file string
		segs []string
	}{
		{"Artist[X]_pic_01.png", []string{"artist", "X", "pic", "01"}},
		{"[作者(画师)]_标题.png", []string{"作者(画师)", "标题"}},
		{"[画师集]_a__b__c.png", []string{"画师集", "a", "b", "c"}},
	}
	for _, c := range cases {
		clues := StructCluesOf("E:/libs/root", "目录/"+c.file, c.file)
		for i, want := range c.segs {
			ref := fmt.Sprintf("filenameSeg%d", i)
			if got, ok := clueByRef(clues, ref); !ok || got.Value != want {
				t.Fatalf("%s 线索 %s 应为 %q，实际 %+v（全集 %v）", c.file, ref, want, got, clues)
			}
		}
		if extra, ok := clueByRef(clues, fmt.Sprintf("filenameSeg%d", len(c.segs))); ok {
			t.Fatalf("%s 分段应恰 %d 段，多出 %q: %v", c.file, len(c.segs), extra.Value, clues)
		}
	}
}

// TestStructCluesShellVariants 剥壳集表驱动：Artist␣...（空格形态）→ 后名；
// 非包裹目录名不产剥壳变体；二级目录层深编号正确。
func TestStructCluesShellVariants(t *testing.T) {
	clues := StructCluesOf("E:/libs/root", "Artist Jake--Jaccobus/漫画集/pic.png", "pic.png")
	if c, ok := clueByRef(clues, "dirLevelStripped1"); !ok || c.Value != "Jake--Jaccobus" {
		t.Fatalf("空格剥壳形态应产内名线索: %+v（全集 %v）", c, clues)
	}
	if _, ok := clueByRef(clues, "dirLevelStripped2"); ok {
		t.Fatalf("非包裹目录名不应产剥壳变体: %v", clues)
	}
	if c, ok := clueByRef(clues, "dirLevel2"); !ok || c.Value != "漫画集" || c.Label != "二级目录" {
		t.Fatalf("二级目录线索不符: %+v", c)
	}
	// 单文件单段：无尾部码
	if _, ok := clueByRef(clues, "tailCode"); ok {
		t.Fatalf("单段文件不应标尾部码: %v", clues)
	}
}

// TestStructCluesBareBracketDir 剧本3 帧1 断言锚：`[...]` 裸括号目录产剥壳
// 线索——目录线索集（工作台目录面板）含「一级目录 [路灯后辈]」与「一级目录
// 剥壳 路灯后辈」（文案逐字）；文件级线索集同款剥壳变体在场；无括号目录名
// 不产剥壳变体（反例保持）。
func TestStructCluesBareBracketDir(t *testing.T) {
	dirClues := DirStructCluesOf("G:/inner world/picture", "[路灯后辈]")
	if len(dirClues) != 3 {
		t.Fatalf("目录线索数 = %d（%v），期望 3（库根名1+目录层1+剥壳1）", len(dirClues), dirClues)
	}
	want := map[string]struct{ Label, Value string }{
		"rootName":          {"库根名", "picture"},
		"dirLevel1":         {"一级目录", "[路灯后辈]"},
		"dirLevelStripped1": {"一级目录剥壳", "路灯后辈"},
	}
	for ref, w := range want {
		c, ok := clueByRef(dirClues, ref)
		if !ok || c.Label != w.Label || c.Value != w.Value {
			t.Fatalf("线索 %s 不符: %+v（期望 %q %q）", ref, c, w.Label, w.Value)
		}
	}
	// 文件级线索集同款剥壳变体
	fileClues := StructCluesOf("G:/inner world/picture", "[路灯后辈]/a.png", "a.png")
	if c, ok := clueByRef(fileClues, "dirLevelStripped1"); !ok || c.Label != "一级目录剥壳" || c.Value != "路灯后辈" {
		t.Fatalf("文件级线索集应含裸括号剥壳变体: %+v（全集 %v）", c, fileClues)
	}
	// 反例保持：无括号目录名不产剥壳变体
	for _, c := range DirStructCluesOf("G:/inner world/picture", "big belly") {
		if c.Kind == structClueDirLevelStripped {
			t.Fatalf("无括号目录名不应产剥壳变体: %+v", c)
		}
	}
}

// TestStructCluesBareBracketFilenameRegression 风险1 回归锚（真实例）：`[...]`
// 内名进入已知名提示集后文件名分段不劣化——[路灯后辈]/[136715041]_无题_[01].png
// 仍恰三段（括号组原子段，内名不在正文出现、零保持区间）；内名在正文出现的
// 形态跨 `__` 边界保持整段（与 Artist[...] 剥壳内名同口径）。
func TestStructCluesBareBracketFilenameRegression(t *testing.T) {
	file := "[136715041]_无题_[01].png"
	clues := StructCluesOf("G:/inner world/picture", "[路灯后辈]/"+file, file)
	for ref, want := range map[string]string{
		"filenameSeg0": "136715041",
		"filenameSeg1": "无题",
		"filenameSeg2": "01",
	} {
		if c, ok := clueByRef(clues, ref); !ok || c.Value != want {
			t.Fatalf("线索 %s 应为 %q，实际 %+v（全集 %v）", ref, want, c, clues)
		}
	}
	if _, ok := clueByRef(clues, "filenameSeg3"); ok {
		t.Fatalf("分段应恰三段: %v", clues)
	}
	if _, ok := clueByRef(clues, "tailCode"); ok {
		t.Fatalf("末段 01 不应判尾部码: %v", clues)
	}

	// 内名作为已知名参与保持：正文中的 jake__jaccobus 跨 `__` 边界整段保持。
	keep := "atago__commission__by_jake__jaccobus_delithw.png"
	keepClues := StructCluesOf("G:/libs/root", "[jake--jaccobus]/"+keep, keep)
	for ref, want := range map[string]string{
		"filenameSeg0": "atago",
		"filenameSeg1": "commission",
		"filenameSeg2": "by",
		"filenameSeg3": "jake__jaccobus",
		"tailCode":     "delithw",
	} {
		if c, ok := clueByRef(keepClues, ref); !ok || c.Value != want {
			t.Fatalf("保持样例线索 %s 应为 %q，实际 %+v（全集 %v）", ref, want, c, keepClues)
		}
	}
}

// TestStructCluesTailCodeConservative 尾部码判别保守性：无数字且无三连辅音的
// 常词末段（picture/commission）不标尾部码；含数字或三连辅音（de1qt6f/
// delithw/dhecdru）标注。含 `_` 的末段（无已知名拆分时）不标——判别保守。
func TestStructCluesTailCodeConservative(t *testing.T) {
	cases := map[string]bool{ // 文件名 → 是否应标尾部码
		"work__picture.png":    false,
		"name__commission.png": false,
		"atago__delithw.png":   true,
		"2b__dhecdru.png":      true,
		"work__de1qt6f.png":    true,
	}
	for name, wantTail := range cases {
		clues := StructCluesOf("E:/libs/root", "目录/"+name, name)
		_, got := clueByRef(clues, "tailCode")
		if got != wantTail {
			t.Fatalf("%s 尾部码判定 = %v，期望 %v（%v）", name, got, wantTail, clues)
		}
	}
}

// TestStripShellTable 剥壳逐条（组界基座语义）：名字含括号组 → 剥壳变体=首个
// 内名非空组的内名（六对括号、同对嵌套按最外层计）；无组名维持空格前缀形态
// （`Artist␣...`→后名）；空内名与非包裹名不命中。
func TestStripShellTable(t *testing.T) {
	if shell, inner, ok := stripShell("Artist[Jake--Jaccobus]"); !ok || inner != "Jake--Jaccobus" ||
		shell.Prefix != "Artist[" || shell.Suffix != "]" {
		t.Fatalf("括号剥壳不符: %+v %q %v", shell, inner, ok)
	}
	if _, inner, ok := stripShell("Artist[Isel]"); !ok || inner != "Isel" {
		t.Fatalf("中缀单组剥壳应为 Isel: %q %v", inner, ok)
	}
	if _, inner, ok := stripShell("Artist Jake--Jaccobus"); !ok || inner != "Jake--Jaccobus" {
		t.Fatalf("空格剥壳不符: %q %v", inner, ok)
	}
	if shell, inner, ok := stripShell("[路灯后辈]"); !ok || inner != "路灯后辈" || shell.Name != "bracket[]" {
		t.Fatalf("裸括号剥壳不符: %+v %q %v", shell, inner, ok)
	}
	// 多组名（组界基座）：剥壳变体=首组内名，不再产出跨界内名。
	multi := "[七尾ゆきじ] 酔いが醒めるその前に (COMIC 阿吽 2019年2月号) [中国翻訳] [DL版]"
	if _, inner, ok := stripShell(multi); !ok || inner != "七尾ゆきじ" {
		t.Fatalf("多组名剥壳应为首组内名 七尾ゆきじ: %q %v", inner, ok)
	}
	// 同对嵌套按最外层计：内层括号留在内名里。
	if _, inner, ok := stripShell("[a[b]c]"); !ok || inner != "a[b]c" {
		t.Fatalf("同对嵌套应按最外层组剥壳: %q %v", inner, ok)
	}
	// 组界基座覆盖全部六对括号：圆括号包裹形态同样剥壳（旧壳表未收录，属
	// 行为变化面——统一后由组界派生）。
	if _, inner, ok := stripShell("Circle(X)"); !ok || inner != "X" {
		t.Fatalf("圆括号包裹形态应按组界剥壳: %q %v", inner, ok)
	}
	if _, _, ok := stripShell("[]"); ok {
		t.Fatal("空内名不应命中剥壳")
	}
	if _, _, ok := stripShell("Artist[]"); ok {
		t.Fatal("空内名不应命中剥壳")
	}
	if _, _, ok := stripShell("普通目录"); ok {
		t.Fatal("普通目录名不应命中剥壳")
	}
}

// TestStructCluesMultiGroupShell 多组名目录的剥壳变体线索（组界基座语义）：
// dirLevelStripped1=首组内名「七尾ゆきじ」，同层恰一条剥壳线索（不再产出
// 跨界内名）。
func TestStructCluesMultiGroupShell(t *testing.T) {
	name := "[七尾ゆきじ] 酔いが醒めるその前に (COMIC 阿吽 2019年2月号) [中国翻訳] [DL版]"
	dirClues := DirStructCluesOf("E:/libs/图库", name)
	stripped := 0
	for _, c := range dirClues {
		if c.Kind != structClueDirLevelStripped {
			continue
		}
		stripped++
		if c.Ref != "dirLevelStripped1" || c.Value != "七尾ゆきじ" || c.Label != "一级目录剥壳" {
			t.Fatalf("多组名剥壳线索应为首组内名: %+v（全集 %v）", c, dirClues)
		}
	}
	if stripped != 1 {
		t.Fatalf("剥壳线索应恰一条: %v", dirClues)
	}
	// 文件级线索集同款剥壳变体。
	file := "page_001.png"
	fileClues := StructCluesOf("E:/libs/图库", name+"/"+file, file)
	if c, ok := clueByRef(fileClues, "dirLevelStripped1"); !ok || c.Value != "七尾ゆきじ" {
		t.Fatalf("文件级线索集多组名剥壳应为首组内名: %+v（全集 %v）", c, fileClues)
	}
}

// TestDirStructCluesOf 目录线索集：库根名 + 各级目录名与剥壳变体（标签
// 「N级目录剥壳」），无文件名段与尾部码；库根（空目录键）仅库根名线索。
func TestDirStructCluesOf(t *testing.T) {
	clues := DirStructCluesOf("E:/libs/big belly", "Artist[Jake--Jaccobus]/子层")
	if len(clues) != 4 {
		t.Fatalf("线索数 = %d（%v），期望 4（库根名1+目录层2+剥壳1）", len(clues), clues)
	}
	want := map[string]struct{ Label, Value string }{
		"rootName":          {"库根名", "big belly"},
		"dirLevel1":         {"一级目录", "Artist[Jake--Jaccobus]"},
		"dirLevelStripped1": {"一级目录剥壳", "Jake--Jaccobus"},
		"dirLevel2":         {"二级目录", "子层"},
	}
	for ref, w := range want {
		c, ok := clueByRef(clues, ref)
		if !ok || c.Label != w.Label || c.Value != w.Value {
			t.Fatalf("线索 %s 不符: %+v（期望 %q %q）", ref, c, w.Label, w.Value)
		}
	}
	for _, c := range clues {
		if c.Kind == structClueFilenameSeg || c.Kind == structClueTailCode {
			t.Fatalf("目录线索集不应含文件名段/尾部码: %+v", c)
		}
	}

	// 库根：仅库根名；无剥壳目录不再多出线索。
	rootOnly := DirStructCluesOf("E:/libs/图库", "")
	if len(rootOnly) != 1 || rootOnly[0].Ref != "rootName" || rootOnly[0].Value != "图库" {
		t.Fatalf("库根目录线索集应仅库根名: %+v", rootOnly)
	}
	// 目录键两端斜杠容忍
	same := DirStructCluesOf("E:/libs/图库", "/Artist[X]/")
	if len(same) != 3 {
		t.Fatalf("带斜杠目录键应归一: %+v", same)
	}
}

// TestPanoramaViewFillsDirStructClues 全景投影的目录条目填充结构线索集：
// 纯呈现态（持久快照记录不带），根节点仅库根名。
func TestPanoramaViewFillsDirStructClues(t *testing.T) {
	snap := &RecognitionSnapshot{
		RootPath: "E:/libs/图库",
		Dirs: []SnapDir{
			{RelKey: "", Name: "图库", Depth: 0},
			{RelKey: "Artist[X]", Name: "Artist[X]", Depth: 1, FileCount: 1},
		},
		Files: []SnapFile{{RelPath: "Artist[X]/a.png", FileName: "a.png", DirRelKey: "Artist[X]", State: snapStatePlain}},
	}
	view := panoramaViewOf(snap)
	if c, ok := clueByRef(view.Dirs[1].StructClues, "dirLevelStripped1"); !ok || c.Value != "X" {
		t.Fatalf("目录条目应透出剥壳线索: %+v", view.Dirs[1].StructClues)
	}
	if len(view.Dirs[0].StructClues) != 1 || view.Dirs[0].StructClues[0].Ref != "rootName" {
		t.Fatalf("根目录条目应仅库根名线索: %+v", view.Dirs[0].StructClues)
	}
	if snap.Dirs[0].StructClues != nil || snap.Dirs[1].StructClues != nil {
		t.Fatalf("持久快照的目录记录不应被写入线索集: %+v", snap.Dirs)
	}
}
