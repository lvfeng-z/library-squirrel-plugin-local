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

// TestStripShellTable 剥壳表逐条：命中/内名/未命中形态。
func TestStripShellTable(t *testing.T) {
	if _, inner, ok := stripShell("Artist[Jake--Jaccobus]"); !ok || inner != "Jake--Jaccobus" {
		t.Fatalf("括号剥壳不符: %q %v", inner, ok)
	}
	if _, inner, ok := stripShell("Artist Jake--Jaccobus"); !ok || inner != "Jake--Jaccobus" {
		t.Fatalf("空格剥壳不符: %q %v", inner, ok)
	}
	if _, _, ok := stripShell("Artist[]"); ok {
		t.Fatal("空内名不应命中剥壳")
	}
	if _, _, ok := stripShell("Circle(X)"); ok {
		t.Fatal("未入表的包裹形态不应命中")
	}
	if _, _, ok := stripShell("普通目录"); ok {
		t.Fatal("普通目录名不应命中剥壳")
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
