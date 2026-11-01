package main

// 赋值就近解析器（目录粒度表示）的单元测试：键判别与根键映射、逐维就近（文件赢
// 父目录/子目录赢祖先/目录未指定维度由上层供值）、纯旧形态等价（全部文件键零目录
// 键=与逐文件直查逐维全等，方案声明12 锚）、死键不误读（文件键 X 与目录键 X/ 互
// 不干扰）、目录预计算 DP（祖先自动解析/表外目录回落现算/与散点现算全等）、呈现
// 位聚合（Generalized 任一供值即真、UpdatedAtMs 取供值条目最近值、被覆盖的供值
// 条目不再计入）。
import (
	"reflect"
	"testing"
)

// TestAssignResolveKeyDiscrimination 键判别与根键映射：以 / 结尾⟺目录条目（库根
// 目录键="/"）；目录寻址键与目录条目键双向互转；根条目对全库文件供值。
func TestAssignResolveKeyDiscrimination(t *testing.T) {
	for _, key := range []string{"/", "Artist/", "Artist/sub/"} {
		if !isDirAssignmentKey(key) {
			t.Fatalf("目录键 %q 应判为目录条目", key)
		}
	}
	for _, key := range []string{"", "a.png", "Artist/a.png", "X"} {
		if isDirAssignmentKey(key) {
			t.Fatalf("文件键 %q 不应判为目录条目", key)
		}
	}
	if got := assignmentDirKeyOf(""); got != "/" {
		t.Fatalf("库根目录条目键 = %q，期望 \"/\"", got)
	}
	if got := assignmentDirKeyOf("Artist"); got != "Artist/" {
		t.Fatalf("一级目录条目键 = %q，期望 \"Artist/\"", got)
	}
	if got := assignmentDirKeyOf("Artist/sub"); got != "Artist/sub/" {
		t.Fatalf("嵌套目录条目键 = %q，期望 \"Artist/sub/\"", got)
	}
	if got := dirRelKeyOfAssignment("/"); got != "" {
		t.Fatalf("根目录条目键的目录寻址键 = %q，期望空串", got)
	}
	if got := dirRelKeyOfAssignment("Artist/"); got != "Artist" {
		t.Fatalf("目录条目键的目录寻址键 = %q，期望 \"Artist\"", got)
	}
	if got := dirRelKeyOfAssignment("Artist/sub/"); got != "Artist/sub" {
		t.Fatalf("嵌套目录条目键的目录寻址键 = %q，期望 \"Artist/sub\"", got)
	}

	// 根条目供值：库根目录键覆盖根直下文件与深层文件（祖先链止于 /）
	assigns := map[string]SessionAssignment{
		"/": {Author: "根作者", AuthorClueRef: "rootName", UpdatedAtMs: 100},
	}
	for _, rel := range []string{"根直下.png", "Artist/a.png", "Artist/sub/b.png"} {
		got := resolveFileAssignment(assigns, rel)
		if got.Author != "根作者" || got.AuthorClueRef != "rootName" || got.UpdatedAtMs != 100 {
			t.Fatalf("根条目应供值 %s: %+v", rel, got)
		}
	}
}

// TestAssignResolveNearestPerDimension 逐维就近：文件例外赢父目录条目、子目录条目
// 赢祖先条目；目录条目未指定的维度由上层（直至库根）继续供值；值与线索出处随维
// 成组走（作者维含 roleName 整组替换，不留深层角色残留）。
func TestAssignResolveNearestPerDimension(t *testing.T) {
	assigns := map[string]SessionAssignment{
		"/":            {Collection: "根集", CollectionClueRef: "rootName", UpdatedAtMs: 100},
		"Artist/":      {Author: "画家甲", AuthorRole: "原画", AuthorClueRef: "dirLevel1", Title: "甲的画集", TitleClueRef: "dirLevel1", UpdatedAtMs: 200, Generalized: true},
		"Artist/sub/":  {Title: "子目录特例", TitleClueRef: "dirLevel2", UpdatedAtMs: 300},
		"Artist/y.png": {Title: "文件例外", TitleClueRef: "filenameSeg0", UpdatedAtMs: 50},
	}

	// 嵌套子树文件（无例外）：标题=子目录赢祖先、作者=子目录未指定由父目录供值、
	// 作品集=全链仅库根指定
	sub := resolveFileAssignment(assigns, "Artist/sub/x.png")
	if sub.Title != "子目录特例" || sub.TitleClueRef != "dirLevel2" {
		t.Fatalf("子目录条目应赢祖先（标题维）: %+v", sub)
	}
	if sub.Author != "画家甲" || sub.AuthorRole != "原画" || sub.AuthorClueRef != "dirLevel1" {
		t.Fatalf("子目录未指定作者维，应由父目录条目供值（含角色与出处成组）: %+v", sub)
	}
	if sub.Collection != "根集" || sub.CollectionClueRef != "rootName" {
		t.Fatalf("作品集维应由库根条目供值: %+v", sub)
	}

	// 文件例外赢父目录（标题维）；其余维度仍就近上溯
	file := resolveFileAssignment(assigns, "Artist/y.png")
	if file.Title != "文件例外" || file.TitleClueRef != "filenameSeg0" {
		t.Fatalf("文件例外应赢父目录条目（标题维）: %+v", file)
	}
	if file.Author != "画家甲" || file.Collection != "根集" {
		t.Fatalf("例外未指定的维度应保持目录供值: %+v", file)
	}

	// 无例外直下文件：父目录条目全维供值
	plain := resolveFileAssignment(assigns, "Artist/z.png")
	if plain.Title != "甲的画集" || plain.TitleClueRef != "dirLevel1" || plain.Author != "画家甲" {
		t.Fatalf("直下文件应由父目录条目供值: %+v", plain)
	}

	// 目录树外文件只受库根供值，其余维度全空
	outside := resolveFileAssignment(assigns, "别处/b.png")
	if outside.Collection != "根集" || outside.Author != "" || outside.Title != "" {
		t.Fatalf("树外文件应仅库根供值: %+v", outside)
	}
}

// TestAssignResolvePureLegacyEquivalent 纯旧形态等价（方案声明12 锚）：全部文件键、
// 零目录键的赋值映射下，散点解析与逐文件直查逐维全等——无目录键=逐维上溯在文件键
// 之外恒落空；覆盖维非空的条目呈现位（Generalized/UpdatedAtMs）原样透传，整体逐
// 字节全等；终选声明序列（求值口径）全等。
func TestAssignResolvePureLegacyEquivalent(t *testing.T) {
	legacy := map[string]SessionAssignment{
		"甲/a.png": {Author: "甲", AuthorRole: "绘", AuthorClueRef: "dirLevel1", Title: "画", TitleClueRef: "filenameSeg0", Tags: []string{"t1", "t2"}, TagClueRefs: []string{"dirLevel1", ""}, Collection: "集", CollectionClueRef: "rootName", Generalized: true, UpdatedAtMs: 1000},
		"乙/b.png": {Author: "乙", UpdatedAtMs: 500},
		"乙/c.png": {Tags: []string{"仅标签"}, TagClueRefs: []string{"rootName"}, Generalized: true},
		"根直下.png": {Title: "根下", UpdatedAtMs: 42},
		// 孤立 roleName（作者主值为空=零覆盖维的旧残留）：不构成任何维度的供值，
		// 解析后各维皆空——与求值口径一致（作者角色只随 Author 维声明）
		"只角色.png": {AuthorRole: "原画", UpdatedAtMs: 900},
	}
	files := []string{"甲/a.png", "乙/b.png", "乙/c.png", "根直下.png", "只角色.png", "无赋值.png"}
	for _, rel := range files {
		want := legacy[rel] // 无条目文件=零值，同直查缺席形态
		got := resolveFileAssignment(legacy, rel)
		if got.Author != want.Author || got.AuthorClueRef != want.AuthorClueRef ||
			got.Title != want.Title || got.TitleClueRef != want.TitleClueRef ||
			got.Collection != want.Collection || got.CollectionClueRef != want.CollectionClueRef ||
			!reflect.DeepEqual(got.Tags, want.Tags) || !reflect.DeepEqual(got.TagClueRefs, want.TagClueRefs) {
			t.Fatalf("纯旧形态逐维不等 %s: 解析=%+v 直查=%+v", rel, got, want)
		}
		// 作者角色随主值成组：主值非空时须同传，主值为空（零覆盖维残留）不独立供值
		if want.Author != "" && got.AuthorRole != want.AuthorRole {
			t.Fatalf("作者角色应随作者维成组 %s: %q ≠ %q", rel, got.AuthorRole, want.AuthorRole)
		}
		if len(want.coveredDimensions()) > 0 {
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("覆盖维非空应整体逐字节全等 %s: 解析=%+v 直查=%+v", rel, got, want)
			}
		}
		if !reflect.DeepEqual(got.assignmentDeclarations(), want.assignmentDeclarations()) {
			t.Fatalf("终选声明序列（求值口径）不等 %s: 解析=%+v 直查=%+v",
				rel, got.assignmentDeclarations(), want.assignmentDeclarations())
		}
	}
}

// TestAssignResolveDeadFileKeyNoBleed 死键不误读：文件键 X（文件已删=死路径残留）
// 与目录键 X/ 共存时互不干扰——解析 X/ 直下文件的祖先链只含目录键（X/ 与 /），
// 不消费键 X；键空间两类形态天然互斥，直查 X 也不受目录条目污染。
func TestAssignResolveDeadFileKeyNoBleed(t *testing.T) {
	assigns := map[string]SessionAssignment{
		"X":  {Title: "死文件残留", TitleClueRef: "filenameSeg0", UpdatedAtMs: 10},
		"X/": {Author: "目录赋值", AuthorClueRef: "dirLevel1", UpdatedAtMs: 20},
	}
	if len(assigns) != 2 || isDirAssignmentKey("X") == isDirAssignmentKey("X/") {
		t.Fatalf("两类键应在键空间互斥共存: %v", assigns)
	}
	// X/ 直下文件：只消费目录键 X/，死文件键 X 的标题不得渗入
	under := resolveFileAssignment(assigns, "X/a.png")
	if under.Author != "目录赋值" || under.AuthorClueRef != "dirLevel1" || under.UpdatedAtMs != 20 {
		t.Fatalf("目录条目应正常供值 X/a.png: %+v", under)
	}
	if under.Title != "" || under.TitleClueRef != "" {
		t.Fatalf("死文件键 X 不得被误读为目录覆盖: %+v", under)
	}
	// 根直下同名文件 X（若仍存在）：直查形态不被目录键反向污染
	self := resolveFileAssignment(assigns, "X")
	if self.Title != "死文件残留" || self.Author != "" || self.UpdatedAtMs != 10 {
		t.Fatalf("文件键 X 自身解析不得消费目录键 X/: %+v", self)
	}
}

// TestAssignResolveDirIndexPrecompute 目录预计算 DP：给定目录集合按父链一次解析，
// 祖先目录（含库根）被顺带入表；表内目录 O(1) 取用、表外目录与文件回落散点现算
// （不写表）；全部文件的预计算结果与散点现算逐字节全等；零维文件例外不供值也不
// 挡目录供值。
func TestAssignResolveDirIndexPrecompute(t *testing.T) {
	assigns := map[string]SessionAssignment{
		"/":                {Collection: "根集", UpdatedAtMs: 100},
		"Artist/":          {Author: "甲", UpdatedAtMs: 200, Generalized: true},
		"Artist/sub/":      {Title: "子集", UpdatedAtMs: 300},
		"Artist/sub/x.png": {Collection: "例外集", UpdatedAtMs: 400},
		"Artist/y.png":     {}, // 零维例外：不构成供值
		"别处/z.png":         {Author: "乙", UpdatedAtMs: 350},
	}
	idx := newAssignmentDirIndex(assigns, []string{"Artist", "Artist/sub"})

	// 祖先自动入表：库根 + 两级目录；表外目录（别处）不占表
	if len(idx.dirs) != 3 {
		t.Fatalf("DP 应入库根与祖先目录（3 个），实得 %d: %v", len(idx.dirs), idx.dirs)
	}
	for _, dirKey := range []string{"", "Artist", "Artist/sub"} {
		if _, ok := idx.dirs[dirKey]; !ok {
			t.Fatalf("目录 %q 应在预计算表内", dirKey)
		}
	}

	// 预计算取用与散点现算全等（含表外目录回落与零维例外）
	for _, rel := range []string{"Artist/sub/x.png", "Artist/y.png", "别处/z.png", "根下.png"} {
		got, want := idx.resolveFile(rel), resolveFileAssignment(assigns, rel)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("预计算与散点现算不等 %s: 预计算=%+v 现算=%+v", rel, got, want)
		}
	}
	deep := idx.resolveFile("Artist/sub/x.png")
	if deep.Title != "子集" || deep.Collection != "例外集" || deep.Author != "甲" ||
		!deep.Generalized || deep.UpdatedAtMs != 400 {
		t.Fatalf("嵌套例外文件解析不符: %+v", deep)
	}

	// 目录级取值：本目录条目盖父链供值；表外目录现算不写表
	for _, tc := range []struct {
		dirKey string
		want   SessionAssignment
	}{
		{"", SessionAssignment{Collection: "根集", UpdatedAtMs: 100}},
		{"Artist", SessionAssignment{Author: "甲", Collection: "根集", Generalized: true, UpdatedAtMs: 200}},
		{"Artist/sub", SessionAssignment{Author: "甲", Title: "子集", Collection: "根集", Generalized: true, UpdatedAtMs: 300}},
		{"别处", SessionAssignment{Collection: "根集", UpdatedAtMs: 100}},
	} {
		if got := idx.resolveDir(tc.dirKey); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("目录 %q 解析不符: %+v，期望 %+v", tc.dirKey, got, tc.want)
		}
	}
	if len(idx.dirs) != 3 {
		t.Fatalf("表外取用不应写表（回落现算保持只读）: %v", idx.dirs)
	}
}

// TestAssignResolvePresentationAggregate 呈现位聚合：Generalized=任一供值条目带标
// 记即真、UpdatedAtMs=供值条目最近值；被更近端覆盖的条目不再供值该维，其标记与时
// 间戳随之不计入（不参与求值终选，仅为呈现口径）。
func TestAssignResolvePresentationAggregate(t *testing.T) {
	assigns := map[string]SessionAssignment{
		"A/":   {Title: "旧标题", UpdatedAtMs: 999, Generalized: true},
		"A/B/": {Title: "新标题", UpdatedAtMs: 10},
		"C/":   {Author: "丙", UpdatedAtMs: 20, Generalized: true},
		"C/d/": {Title: "丁集", UpdatedAtMs: 30},
	}

	// 被覆盖供值者不计入：标题维由 A/B/ 供值，A/ 的 t=999 与泛化标记失效
	overridden := resolveFileAssignment(assigns, "A/B/f.png")
	if overridden.Title != "新标题" || overridden.UpdatedAtMs != 10 || overridden.Generalized {
		t.Fatalf("被覆盖供值者的标记与时间不得计入: %+v", overridden)
	}

	// 任一供值即真 + 供值条目最近值：作者维供值者（C/）带标记，时间取两供值条目最大
	mixed := resolveFileAssignment(assigns, "C/d/g.png")
	if !mixed.Generalized || mixed.UpdatedAtMs != 30 || mixed.Author != "丙" || mixed.Title != "丁集" {
		t.Fatalf("呈现聚合不符（任一供值即真/最近值）: %+v", mixed)
	}

	// 全链无供值=零值（无呈现位）
	none := resolveFileAssignment(assigns, "E/h.png")
	if !reflect.DeepEqual(none, SessionAssignment{}) {
		t.Fatalf("无供值链应得零值: %+v", none)
	}
}
