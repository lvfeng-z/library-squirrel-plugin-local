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
		{name: "空模板", tpl: " ", wantErr: "格式模板不能为空"},
		{name: "词表外", tpl: "{不存在}_{序号}", wantErr: "不在词表"},
		{name: "重复占位词", tpl: "{作者}_{作者}", wantErr: "重复"},
		{name: "相邻占位词", tpl: "{作者}{序号}", wantErr: "缺少分隔"},
		{name: "键用于文件模板", tpl: "{键}_{序号}", wantErr: "仅适用于序列"},
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
	if _, ok := notPat.Inner.(*FileSubjectPattern); !ok {
		t.Fatalf("非的内层应为文件主体，得到 %T", notPat.Inner)
	}
	// 单行归一。
	single, err := compileCondxRows([]CondxRow{{Condition: file("{作者}")}})
	if err != nil {
		t.Fatalf("单行编译失败: %v", err)
	}
	if _, ok := single.(*FileSubjectPattern); !ok {
		t.Fatalf("单行应归一为主体模式，得到 %T", single)
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

func TestCompileCondxRejectsUnimplemented(t *testing.T) {
	c := &Condx{
		Target: CondxTargetDir,
		Format: condxTpl("[{作者}][{作品号}][{标题}]"),
		Attach: []CondxAttach{{Kind: CondxAttachChildrenAllFormat, Tpl: "{序号}", MinCount: 2}},
	}
	if _, err := compileCondx(c); err == nil || !strings.Contains(err.Error(), "下一硬化步") {
		t.Fatalf("子女谓词应显式拒绝，得到 %v", err)
	}
	c2 := &Condx{
		Target: CondxTargetFile,
		Format: condxTpl("{作者}"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author", Fold: true}},
	}
	if _, err := compileCondx(c2); err == nil || !strings.Contains(err.Error(), "下一硬化步") {
		t.Fatalf("值级小标应显式拒绝，得到 %v", err)
	}
}

func TestFileSubjectEvaluate(t *testing.T) {
	p, err := compileCondx(&Condx{Target: CondxTargetFile, Format: condxTpl("Isel_{集名}_{序号}_[中文翻譯][高清]")})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{FileName: "Isel_瞬間的に絡み合う_01_[中文翻譯][高清].jpg"}
	ctx := &ParseContext{}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatalf("应命中")
	}
	setname, ok := ctx.Capture("setname")
	if !ok || setname != "瞬間的に絡み合う" {
		t.Fatalf("集名捕获不符: %q %v", setname, ok)
	}
	seq, _ := ctx.Capture("seq")
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
	p, err := compileCondx(&Condx{Target: CondxTargetDir, Format: condxTpl("[{作者}] [{作品号}] [{标题}]")})
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
	author, _ := ctx.Capture("author")
	workid, _ := ctx.Capture("workid")
	if author != "Isel" || workid != "92242473" {
		t.Fatalf("目录捕获不符: author=%q workid=%q", author, workid)
	}
	if len(ctx.DirHits) != 1 || ctx.DirHits[0].Name != "[Isel] [92242473] [瞬間的に絡み合う]" || ctx.DirHits[0].Depth != 2 {
		t.Fatalf("DirHit 不符: %+v", ctx.DirHits)
	}
	// 包含问法：任意一层含词即命中。
	cp, err := compileCondx(&Condx{Target: CondxTargetDir, Format: CondxFormat{Mode: CondxFormatContains, Word: "Isel"}})
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
		Attach: []CondxAttach{{Kind: CondxAttachSeqMinLength, MinCount: 3}}})
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	seq, ok := p.(*SeqSubjectPattern)
	if !ok {
		t.Fatalf("应编译为序列主体，得到 %T", p)
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
	key, _ := ctx.Capture("key")
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
