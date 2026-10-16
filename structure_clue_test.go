package main

// 结构分段线索集的单元测试：真实样例分段（含 Artist[...] 剥壳变体 / __ 分隔段 /
// 已知名跨段混合形态 / 尾部码标记的保守判别）与剥壳集表驱动形态。
import (
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
		"rootName":         {"库根名", "big belly"},
		"dirLevel1":        {"一级目录", "Artist[Jake--Jaccobus]"},
		"dirLevelStripped1": {"一级目录(剥壳)", "Jake--Jaccobus"},
		"filenameSeg0":     {"文件名段", "atago"},
		"filenameSeg1":     {"文件名段", "commission"},
		"filenameSeg2":     {"文件名段", "by"},
		"filenameSeg3":     {"文件名段", "jake__jaccobus"},
		"tailCode":         {"尾部码", "delithw"},
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
