// v4 条件语法层测试：模板推导、静态校验、行列表编译、三类主体求值。
package main

import (
	"reflect"
	"strings"
	"testing"
)

func condxTpl(tpl string) CondxFormat {
	return CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl}
}

func TestDeriveCondxTemplate(t *testing.T) {
	cases := []struct {
		name    string
		tpl     string
		opts    deriveOpts
		expr    string
		caps    []string
		wantErr string
	}{
		{
			name: "文件模板·中位贪婪回溯",
			tpl:  "Isel_{集名}_{序号}_[中文翻譯][高清]",
			expr: `^Isel_(?P<setname>.+)_(?P<seq>[0-9]+)_\[中文翻譯\]\[高清\]$`,
			caps: []string{"setname", "seq"},
		},
		{
			name: "首位数字占位词",
			tpl:  "Tumblr{日期}[pixiv_{作品号}_{作者}]_{序号}",
			expr: `^Tumblr(?P<date>[0-9]+)\[pixiv_(?P<workid>[0-9]+)_(?P<author>.+)\]_(?P<seq>[0-9]+)$`,
			caps: []string{"date", "workid", "author", "seq"},
		},
		{
			name: "单独占位词·贪婪段",
			tpl:  "{作者}",
			expr: `^(?P<author>.+)$`,
			caps: []string{"author"},
		},
		{
			name: "目录模板·括号制式",
			tpl:  "[{作者}][{作品号}][{标题}]",
			expr: `^\[(?P<author>.+)\]\[(?P<workid>[0-9]+)\]\[(?P<title>.+)\]$`,
			caps: []string{"author", "workid", "title"},
		},
		{
			name: "序列成员模板·键含下划线可匹配",
			tpl:  "{键}_{序号}",
			opts: deriveOpts{allowKey: true, requireSeq: true},
			expr: `^(?P<key>.+)_(?P<seq>[0-9]+)$`,
			caps: []string{"key", "seq"},
		},
		{
			name: "序列成员模板·新词公共段与旧词同组同义",
			tpl:  "{公共段}_{序号}",
			opts: deriveOpts{allowKey: true, requireSeq: true},
			expr: `^(?P<key>.+)_(?P<seq>[0-9]+)$`,
			caps: []string{"key", "seq"},
		},
		{name: "空模板", tpl: " ", wantErr: "格式模板不能为空"},
		{name: "词表外", tpl: "{不存在}_{序号}", wantErr: "不在词表"},
		{name: "重复占位词", tpl: "{作者}_{作者}", wantErr: "重复"},
		{name: "相邻占位词", tpl: "{作者}{序号}", wantErr: "缺少分隔"},
		{name: "键用于文件模板", tpl: "{键}_{序号}", wantErr: "仅适用于序列"},
		{name: "公共段用于文件模板", tpl: "{公共段}_{序号}", wantErr: "仅适用于序列"},
		{name: "新旧词同模板并用即重复", tpl: "{键}_{公共段}_{序号}", opts: deriveOpts{allowKey: true, requireSeq: true}, wantErr: "重复"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, caps, err := deriveCondxTemplate(tc.tpl, tc.opts)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("推导失败: %v", err)
			}
			if expr != tc.expr {
				t.Fatalf("表达式不符:\n得到 %q\n期望 %q", expr, tc.expr)
			}
			if !reflect.DeepEqual(caps, tc.caps) {
				t.Fatalf("捕获组不符: 得到 %v 期望 %v", caps, tc.caps)
			}
		})
	}
}

// TestCondxVocabDualWord 换词后的双词映射与提示清单：{公共段} 为书写侧新词、
// 与旧词 {键} 同组同义（旧档零迁移）；词表报错提示只列新词。
func TestCondxVocabDualWord(t *testing.T) {
	if condxVocab["公共段"] != "key" || condxVocab["键"] != "key" {
		t.Fatalf("公共段应与旧词「键」同组 key: 作者=%q 公共段=%q", condxVocab["键"], condxVocab["公共段"])
	}
	if strings.Contains(condxVocabHint, "键") || !strings.Contains(condxVocabHint, "公共段") {
		t.Fatalf("词表提示应只列新词「公共段」: %q", condxVocabHint)
	}
}

func TestValidateCondx(t *testing.T) {
	cases := []struct {
		name    string
		c       Condx
		wantErr string
	}{
		{name: "合法文件条件", c: Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}")}},
		{name: "未知目标", c: Condx{Target: "node", Format: condxTpl("{作者}")}, wantErr: "未知的目标类别"},
		{name: "ref 锚预留拒绝", c: Condx{Target: CondxTargetFile, Format: condxTpl("{作者}"), Anchor: &CondxAnchor{Kind: CondxAnchorRef, Row: 1}}, wantErr: "二期能力"},
		{name: "包含词为空", c: Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatContains}}, wantErr: "包含词不能为空"},
		{name: "序列不支持包含问法", c: Condx{Target: CondxTargetSeq, Format: CondxFormat{Mode: CondxFormatContains, Word: "x"}}, wantErr: "只支持整条按模板"},
		{name: "序列缺序号", c: Condx{Target: CondxTargetSeq, Format: condxTpl("{键}")}, wantErr: "必须包含 {序号}"},
		{
			name:    "小标引用不在本行",
			c:       Condx{Target: CondxTargetFile, Format: condxTpl("{作者}"), Badges: []CondxBadge{{Placeholder: "标签", Kind: CondxBadgeLibrary, Dim: "author"}}},
			wantErr: "不在本行格式模板内",
		},
		{
			name:    "小标名单维度无效",
			c:       Condx{Target: CondxTargetFile, Format: condxTpl("{作者}"), Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "title"}}},
			wantErr: "名单维度无效",
		},
		{
			name:    "文件目标挂目录谓词",
			c:       Condx{Target: CondxTargetFile, Format: condxTpl("{作者}"), Attach: []CondxAttach{{Kind: CondxAttachChildDirCount, MinCount: 2}}},
			wantErr: "不适用于目标",
		},
		{
			name:    "子女谓词缺数量",
			c:       Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}]"), Attach: []CondxAttach{{Kind: CondxAttachChildrenAllFormat, Tpl: "{序号}"}}},
			wantErr: "数量门槛",
		},
		{
			name:    "类型列表越界值",
			c:       Condx{Target: CondxTargetSeq, Format: condxTpl("{键}_{序号}"), Attach: []CondxAttach{{Kind: CondxAttachSeqMemberType, Types: []string{"audio"}}}},
			wantErr: "类型无效",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCondx(&tc.c, 1)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过，得到错误: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateCondxRows(t *testing.T) {
	file := func(tpl string) *Condx { return &Condx{Target: CondxTargetFile, Format: condxTpl(tpl)} }
	cases := []struct {
		name    string
		rows    []CondxRow
		wantErr string
	}{
		{name: "空列表", rows: nil, wantErr: "不能为空"},
		{
			name:    "首行带连接词",
			rows:    []CondxRow{{Connector: "and", Condition: file("{作者}")}},
			wantErr: "首行不能带",
		},
		{
			name:    "连接词非法",
			rows:    []CondxRow{{Condition: file("{作者}")}, {Connector: "xor", Condition: file("{标题}")}},
			wantErr: "连接词无效",
		},
		{
			name:    "行同时有条件与子组",
			rows:    []CondxRow{{Condition: file("{作者}"), Group: []CondxRow{{Condition: file("{标题}")}}}},
			wantErr: "之一",
		},
		{
			name: "跨行相等引用早行·通过",
			rows: []CondxRow{
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}")}},
				{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}]"),
					Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeEqual, AgainstRow: 1, AgainstPh: "作者"}}}},
			},
		},
		{
			name: "跨行相等引用自身行·拒绝",
			rows: []CondxRow{
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}")}},
				{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}]"),
					Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeEqual, AgainstRow: 2, AgainstPh: "作者"}}}},
			},
			wantErr: "须早于本行",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCondxRows(tc.rows)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过，得到错误: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
			}
		})
	}
}

func TestCompileCondxRowsTreeShape(t *testing.T) {
	file := func(tpl string) *Condx { return &Condx{Target: CondxTargetFile, Format: condxTpl(tpl)} }
	rows := []CondxRow{
		{Condition: file("{作者}")},
		{Connector: "and", Condition: file("{标题}")},
		{Connector: "or", Condition: file("{集名}")},
		{Connector: "and", Negated: true, Condition: file("{标签}")},
	}
	p, err := compileCondxRows(rows)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	orTree, ok := p.(Or)
	if !ok {
		t.Fatalf("顶层应为 Or，得到 %T", p)
	}
	if len(orTree) != 2 {
		t.Fatalf("或分支数应为 2，得到 %d", len(orTree))
	}
	andTree, ok := orTree[0].(And)
	if !ok || len(andTree) != 2 {
		t.Fatalf("第一或分支应为 And(2)，得到 %T", orTree[0])
	}
	// 第二或分支 = And[C, ¬D]：或切段内 and 链，行级非包裹末行。
	secondAnd, ok := orTree[1].(And)
	if !ok || len(secondAnd) != 2 {
		t.Fatalf("第二或分支应为 And(2)，得到 %T", orTree[1])
	}
	notPat, ok := secondAnd[1].(Not)
	if !ok {
		t.Fatalf("或分支第二子式应为 Not，得到 %T", secondAnd[1])
	}
	if row, ok := notPat.Inner.(*RowSubjectPattern); !ok || row.Ordinal != 4 {
		t.Fatalf("非的内层应为行包装（行4），得到 %T %+v", notPat.Inner, notPat.Inner)
	}
	// 单行归一为行包装。
	single, err := compileCondxRows([]CondxRow{{Condition: file("{作者}")}})
	if err != nil {
		t.Fatalf("单行编译失败: %v", err)
	}
	if row, ok := single.(*RowSubjectPattern); !ok || row.Ordinal != 1 {
		t.Fatalf("单行应归一为行包装（行1），得到 %T", single)
	}
	// 子组最紧：A or (B and C)。
	grouped, err := compileCondxRows([]CondxRow{
		{Condition: file("{作者}")},
		{Connector: "or", Group: []CondxRow{
			{Condition: file("{标题}")},
			{Connector: "and", Condition: file("{集名}")},
		}},
	})
	if err != nil {
		t.Fatalf("子组编译失败: %v", err)
	}
	gOr, ok := grouped.(Or)
	if !ok || len(gOr) != 2 {
		t.Fatalf("子组用例顶层应为 Or(2)，得到 %T", grouped)
	}
	if _, ok := gOr[1].(And); !ok {
		t.Fatalf("子组应编译为 And，得到 %T", gOr[1])
	}
}

func TestCompileCondxRejectsFileSizeMin(t *testing.T) {
	// fileSizeMin 为唯一显式拒绝件：扫描快照尚无文件尺寸（数据缺口），先拒后接。
	c := &Condx{
		Target: CondxTargetFile,
		Format: condxTpl("{作者}"),
		Attach: []CondxAttach{{Kind: CondxAttachFileSizeMin, MinCount: 500}},
	}
	if _, err := compileCondx(c, 1); err == nil || !strings.Contains(err.Error(), "文件尺寸") {
		t.Fatalf("fileSizeMin 应显式拒绝，得到 %v", err)
	}
	// 子女谓词与值级小标本步已接入：应编译成功。
	ok1 := &Condx{
		Target: CondxTargetDir,
		Format: condxTpl("[{作者}][{作品号}][{标题}]"),
		Attach: []CondxAttach{{Kind: CondxAttachChildrenAllFormat, Tpl: "{序号}", MinCount: 2}},
	}
	if _, err := compileCondx(ok1, 1); err != nil {
		t.Fatalf("子女谓词应可编译，得到 %v", err)
	}
	ok2 := &Condx{
		Target: CondxTargetFile,
		Format: condxTpl("{作者}"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true}},
	}
	if _, err := compileCondx(ok2, 1); err != nil {
		t.Fatalf("值级小标应可编译，得到 %v", err)
	}
}

func TestFileSubjectEvaluate(t *testing.T) {
	p, err := compileCondx(&Condx{Target: CondxTargetFile, Format: condxTpl("Isel_{集名}_{序号}_[中文翻譯][高清]")}, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{FileName: "Isel_瞬間的に絡み合う_01_[中文翻譯][高清].jpg"}
	ctx := &ParseContext{}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatalf("应命中")
	}
	// 行作用域：捕获以行号前缀并入主上下文（rN.组名）。
	setname, ok := ctx.Capture("r1.setname")
	if !ok || setname != "瞬間的に絡み合う" {
		t.Fatalf("集名捕获不符: %q %v", setname, ok)
	}
	seq, _ := ctx.Capture("r1.seq")
	if seq != "01" {
		t.Fatalf("序号捕获不符: %q", seq)
	}
	// 扩展名不参与模板：同基名不同扩展名同样命中；序号位非数字（违反格式）不命中
	//——「集名换了个值」不会失配，那是值级小标（库名单）的职责，不是格式的职责。
	if !p.Evaluate(&MatchInput{FileName: "Isel_瞬間的に絡み合う_01_[中文翻譯][高清].png"}, nil, &ParseContext{}) {
		t.Fatalf("扩展名变化不应影响命中")
	}
	if p.Evaluate(&MatchInput{FileName: "Isel_他作_甲_[中文翻譯][高清].jpg"}, nil, &ParseContext{}) {
		t.Fatalf("序号位非数字不应命中")
	}
}

func TestDirSubjectEvaluate(t *testing.T) {
	p, err := compileCondx(&Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}] [{作品号}] [{标题}]")}, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{
		FileName: "01.jpg",
		DirNames: []string{"big belly", "Artist[Isel]", "[Isel] [92242473] [瞬間的に絡み合う]"},
	}
	ctx := &ParseContext{}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatalf("作品层目录应命中")
	}
	author, _ := ctx.Capture("r1.author")
	workid, _ := ctx.Capture("r1.workid")
	if author != "Isel" || workid != "92242473" {
		t.Fatalf("目录捕获不符: author=%q workid=%q", author, workid)
	}
	if len(ctx.DirHits) != 1 || ctx.DirHits[0].Name != "[Isel] [92242473] [瞬間的に絡み合う]" || ctx.DirHits[0].Depth != 2 {
		t.Fatalf("DirHit 不符: %+v", ctx.DirHits)
	}
	// 包含问法：任意一层含词即命中。
	cp, err := compileCondx(&Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatContains, Word: "Isel"}}, 1)
	if err != nil {
		t.Fatalf("包含问法编译失败: %v", err)
	}
	if !cp.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("包含问法应命中")
	}
	if cp.Evaluate(&MatchInput{FileName: "a.jpg", DirNames: []string{"big belly", "DeviantArt"}}, nil, &ParseContext{}) {
		t.Fatalf("无匹配层不应命中")
	}
}

func TestSeqSubjectEvaluate(t *testing.T) {
	p, err := compileCondx(&Condx{Target: CondxTargetSeq, Format: condxTpl("{键}_{序号}"),
		Attach: []CondxAttach{{Kind: CondxAttachSeqMinLength, MinCount: 3}}}, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	row, ok := p.(*RowSubjectPattern)
	if !ok {
		t.Fatalf("应编译为行包装，得到 %T", p)
	}
	seq, ok := row.Inner.(*SeqSubjectPattern)
	if !ok {
		t.Fatalf("行内主体应为序列主体，得到 %T", row.Inner)
	}
	if seq.MinSize != 3 {
		t.Fatalf("MinSize 应取附加条件值 3，得到 %d", seq.MinSize)
	}
	in := &MatchInput{
		FileName: "kpop_dancer_wg_07.jpg",
		Siblings: []string{"kpop_dancer_wg_07.jpg", "kpop_dancer_wg_08.jpg", "kpop_dancer_wg_09.jpg", "其他图_01.jpg"},
	}
	ctx := &ParseContext{}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatalf("三成员同键应成套")
	}
	if ctx.Group == nil || ctx.Group.Key != "kpop_dancer_wg" || ctx.Group.Seq != 7 || ctx.Group.Size != 3 {
		t.Fatalf("GroupAnalysis 不符: %+v", ctx.Group)
	}
	key, _ := ctx.Capture("r1.key")
	if key != "kpop_dancer_wg" {
		t.Fatalf("键捕获不符: %q", key)
	}
	if p.Evaluate(&MatchInput{FileName: "a_01.jpg", Siblings: []string{"a_01.jpg", "a_02.jpg"}}, nil, &ParseContext{}) {
		t.Fatalf("两成员未达门槛不应命中")
	}
	if p.Evaluate(&MatchInput{FileName: "a_x.jpg", Siblings: []string{"a_x.jpg", "a_02.jpg", "a_03.jpg"}}, nil, &ParseContext{}) {
		t.Fatalf("自身无序数不应命中")
	}
}

// TestDirSubjectChildrenAsserts 子女终结谓词：作品目录里全是 {序号} 子女（T1
// 作品目录制的结构核）。AncestorFiles 缺失（旧构造点）时子女谓词保守不命中。
func TestDirSubjectChildrenAsserts(t *testing.T) {
	c := &Condx{
		Target: CondxTargetDir,
		Format: condxTpl("[{作者}] [{作品号}] [{标题}]"),
		Attach: []CondxAttach{{Kind: CondxAttachChildrenAllFormat, Tpl: "{序号}", MinCount: 3}},
	}
	p, err := compileCondx(c, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	work := &MatchInput{
		FileName: "01.jpg",
		DirNames: []string{"Art", "Artist[Isel]", "[Isel] [92242473] [T]"},
		AncestorFiles: [][]string{
			{"index.txt"},
			{"Artist[Isel]", "[Isel] [92242473] [T]"},
			{"01.jpg", "02.jpg", "03.jpg", "info.txt"},
		},
	}
	if p.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("含非序号子女的目录不应命中（childrenAllFormat 全称断言）")
	}
	work.AncestorFiles[2] = []string{"01.jpg", "02.jpg", "03.jpg", "10.png"}
	if !p.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("全序号子女且达门槛应命中")
	}
	work.AncestorFiles[2] = []string{"01.jpg", "02.jpg"}
	if p.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("子女数低于 MinCount 不应命中")
	}
	work.AncestorFiles = nil
	if p.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("子女数据缺失不应命中")
	}
	// childrenAtLeastN：至少 2 个 {序号} 子女即命中（允许其余杂项）。
	c2 := &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}] [{作品号}] [{标题}]"),
		Attach: []CondxAttach{{Kind: CondxAttachChildrenAtLeastN, Tpl: "{序号}", MinCount: 2}}}
	p2, err := compileCondx(c2, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	work.AncestorFiles = [][]string{nil, {"Artist[Isel]"}, {"01.jpg", "02.jpg", "cover.png", "info.txt"}}
	if !p2.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("至少两个序号子女应命中")
	}
	// childTypeIs：全部子女类型属于指定集。
	c3 := &Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}] [{作品号}] [{标题}]"),
		Attach: []CondxAttach{{Kind: CondxAttachChildTypeIs, Types: []string{"image"}}}}
	p3, err := compileCondx(c3, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	work.AncestorFiles[2] = []string{"01.jpg", "cover.png"}
	if !p3.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("全图类子女应命中")
	}
	work.AncestorFiles[2] = []string{"01.jpg", "info.txt"}
	if p3.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("混杂文档子女不应命中")
	}
	// childDirCount：作者壳（含多个作品子目录）——作者目录层子女目录数 ≥2。
	c4 := &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]"),
		Attach: []CondxAttach{{Kind: CondxAttachChildDirCount, MinCount: 2}}}
	p4, err := compileCondx(c4, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	work.AncestorChildDirCounts = []int{0, 2, 0}
	if !p4.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("作者壳两作品子目录应命中")
	}
	work.AncestorChildDirCounts = []int{0, 1, 0}
	if p4.Evaluate(work, nil, &ParseContext{}) {
		t.Fatalf("单作品子目录不应命中 childDirCount≥2")
	}
}

// TestDirSubjectParentAssert dir 目标的父目录谓词作用于**命中层**的父
// （R4：仅上 1 级）：作品目录的父须为 Artist[{作者}] 壳。
func TestDirSubjectParentAssert(t *testing.T) {
	c := &Condx{
		Target: CondxTargetDir,
		Format: condxTpl("[{作者}] [{作品号}] [{标题}]"),
		Attach: []CondxAttach{{Kind: CondxAttachParentFormat, Tpl: "Artist[{作者}]"}},
	}
	p, err := compileCondx(c, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{FileName: "01.jpg", DirNames: []string{"Art", "Artist[Isel]", "[Isel] [92242473] [T]"}}
	if !p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("父为作者壳应命中")
	}
	in.DirNames[1] = "misc"
	if p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("父不是作者壳不应命中")
	}
}

// TestFileParentAndExt file 目标的父目录谓词（直接父）与 fileExt。
func TestFileParentAndExt(t *testing.T) {
	c := &Condx{
		Target: CondxTargetFile,
		Format: condxTpl("{标题}_{序号}"),
		Attach: []CondxAttach{
			{Kind: CondxAttachParentContains, Word: "pixiv"},
			{Kind: CondxAttachFileExt, Value: ".jpg"},
		},
	}
	p, err := compileCondx(c, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{FileName: "sunset_02.JPG", DirNames: []string{"Art", "pixiv_2024"}}
	if !p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("父含 pixiv 且扩展名匹配（大小写不敏感）应命中")
	}
	in.FileName = "sunset_02.png"
	if p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("扩展名不符不应命中")
	}
	in.FileName = "sunset_02.JPG"
	in.DirNames[1] = "tumblr"
	if p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("父不含 pixiv 不应命中")
	}
}

// TestSeqMemberType 序列成员类型限定：散页成套排除目录内说明 txt（T3）。
func TestSeqMemberType(t *testing.T) {
	c := &Condx{Target: CondxTargetSeq, Format: condxTpl("{键}_{序号}"),
		Attach: []CondxAttach{{Kind: CondxAttachSeqMemberType, Types: []string{"image"}}}}
	p, err := compileCondx(c, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{
		FileName: "trip_02.jpg",
		Siblings: []string{"trip_01.jpg", "trip_02.jpg", "trip_03.jpg", "trip_info.txt"},
	}
	ctx := &ParseContext{}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatalf("三图像成员成套应命中（txt 不计数也不否决）")
	}
	if ctx.Group == nil || ctx.Group.Size != 3 {
		t.Fatalf("Group.Size 应只计图像成员，得到 %+v", ctx.Group)
	}
	in2 := &MatchInput{FileName: "trip_info.txt", Siblings: in.Siblings}
	if p.Evaluate(in2, nil, &ParseContext{}) {
		t.Fatalf("文档自身不应按图像序列成员命中")
	}
}

// TestBadgeGates 值级小标三门：库名单（形变归一唯一才命中）、常量；折叠
// 规范名以 #canon 后缀回绑供提取消费。
func TestBadgeGates(t *testing.T) {
	lib := &LibraryIndex{AuthorNames: map[string]struct{}{
		"Jake_Jaccobus": {}, "jake--other": {},
	}}
	c := &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true}}}
	p, err := compileCondx(c, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	ctx := &ParseContext{}
	if !p.Evaluate(&MatchInput{FileName: "jake__jaccobus_sunset.jpg"}, lib, ctx) {
		t.Fatalf("折叠唯一应命中")
	}
	if v, ok := ctx.Capture("r1.author#canon"); !ok || v != "Jake_Jaccobus" {
		t.Fatalf("规范名绑定不符: %q %v", v, ok)
	}
	if len(ctx.PredHits) != 1 || ctx.PredHits[0].Name != "Jake_Jaccobus" {
		t.Fatalf("谓词命中记录不符: %+v", ctx.PredHits)
	}
	if p.Evaluate(&MatchInput{FileName: "jake_moon.jpg"}, lib, &ParseContext{}) {
		t.Fatalf("折叠零命中应不命中")
	}
	// library 精确（不折叠）：值须逐字在名单。
	c2 := &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author"}}}
	p2, _ := compileCondx(c2, 1)
	if !p2.Evaluate(&MatchInput{FileName: "Jake_Jaccobus_sunset.jpg"}, lib, &ParseContext{}) {
		t.Fatalf("精确名单应命中")
	}
	if p2.Evaluate(&MatchInput{FileName: "jake__jaccobus_sunset.jpg"}, lib, &ParseContext{}) {
		t.Fatalf("形变名在精确模式不应命中")
	}
	// constant。
	c3 := &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeConstant, Value: "Isel"}}}
	p3, _ := compileCondx(c3, 1)
	if !p3.Evaluate(&MatchInput{FileName: "Isel_x.jpg"}, nil, &ParseContext{}) {
		t.Fatalf("常量相等应命中")
	}
	if p3.Evaluate(&MatchInput{FileName: "Other_x.jpg"}, nil, &ParseContext{}) {
		t.Fatalf("常量不等不应命中")
	}
}

// TestCrossRowEqualGate T2 作者壳制：两行 + 跨行值级相等——目录行绑定的
// {作者} 与文件行绑定的 {作者} 相等才整体命中（行序即求值序；捕获行号前缀
// 使两行同名占位词互不遮蔽）。
func TestCrossRowEqualGate(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作者}_{标题}_{序号}")}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("Artist[{作者}]"),
			Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeEqual, AgainstRow: 1, AgainstPh: "作者"}}}},
	}
	p, err := compileCondxRows(rows)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{
		FileName: "Isel_sunset_03.jpg",
		DirNames: []string{"Art", "Artist[Isel]"},
	}
	if !p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("两行作者同值应整体命中")
	}
	in.DirNames[1] = "Artist[Other]"
	if p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("作者异值不应命中")
	}
}

// TestAncestorAggregation 流水线派生：各层祖先的直接子文件/子目录数，及
// 目录子女谓词经派生数据可达（W1 全序号子女 ≥2 命中）。
func TestAncestorAggregation(t *testing.T) {
	files := []ScanFile{
		{FileName: "01.jpg", DirNames: []string{"Art", "A", "W1"}, RelPath: "Art/A/W1/01.jpg"},
		{FileName: "02.jpg", DirNames: []string{"Art", "A", "W1"}, RelPath: "Art/A/W1/02.jpg"},
		{FileName: "cover.png", DirNames: []string{"Art", "B", "W2"}, RelPath: "Art/B/W2/cover.png"},
	}
	siblings := map[string][]string{}
	for _, f := range files {
		k := dirGroupKey(f.DirNames)
		siblings[k] = append(siblings[k], f.FileName)
	}
	childDirs := map[string]map[string]struct{}{}
	for _, f := range files {
		for d := 0; d+1 < len(f.DirNames); d++ {
			pk := dirGroupKey(f.DirNames[:d+1])
			set, ok := childDirs[pk]
			if !ok {
				set = map[string]struct{}{}
				childDirs[pk] = set
			}
			set[f.DirNames[d+1]] = struct{}{}
		}
	}
	in := &MatchInput{
		FileName:               "01.jpg",
		DirNames:               files[0].DirNames,
		Siblings:               siblings[dirGroupKey(files[0].DirNames)],
		AncestorFiles:          ancestorFilesLevels(files[0].DirNames, siblings),
		AncestorChildDirCounts: ancestorChildDirCounts(files[0].DirNames, childDirs),
	}
	if got, ok := in.ancestorFilesAt(2); !ok || len(got) != 2 {
		t.Fatalf("作品层子女应 2 文件，得到 %v %v", got, ok)
	}
	if got, ok := in.ancestorFilesAt(1); ok || len(got) != 0 {
		t.Fatalf("作者层无直接子文件（子目录不计入子女文件），得到 %v %v", got, ok)
	}
	if c, _ := in.ancestorChildDirCountAt(0); c != 2 {
		t.Fatalf("根层子目录数应 2（A/B），得到 %d", c)
	}
	if c, _ := in.ancestorChildDirCountAt(1); c != 1 {
		t.Fatalf("作者层子目录数应 1（W1），得到 %d", c)
	}
	c := &Condx{Target: CondxTargetDir, Format: condxTpl("W{作品号}"),
		Attach: []CondxAttach{{Kind: CondxAttachChildrenAllFormat, Tpl: "{序号}", MinCount: 2}}}
	p, err := compileCondx(c, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if !p.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("W1 全序号子女应命中")
	}
}
