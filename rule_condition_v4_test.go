// v4 条件语法层测试：模板推导、静态校验、行列表编译、三类主体求值。
package main

import (
	"reflect"
	"regexp"
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
			// 2026-11-04 终审拍板（惰性化）：填充段让位一切占位词——无类
			// 锚定的贪婪段 .+ 全量改编译为惰性 .+?，下列期望表达式随之翻新。
			name: "文件模板·中位惰性让位",
			tpl:  "Isel_{集名}_{序号}_[中文翻譯][高清]",
			expr: `^Isel_(?P<setname>.+?)_(?P<seq>[0-9]+)_\[中文翻譯\]\[高清\]$`,
			caps: []string{"setname", "seq"},
		},
		{
			name: "首位数字占位词",
			tpl:  "Tumblr{日期}[pixiv_{作品号}_{作者}]_{序号}",
			expr: `^Tumblr(?P<date>[0-9]+)\[pixiv_(?P<workid>[0-9]+)_(?P<author>.+?)\]_(?P<seq>[0-9]+)$`,
			caps: []string{"date", "workid", "author", "seq"},
		},
		{
			name: "单独占位词·惰性段",
			tpl:  "{作者}",
			expr: `^(?P<author>.+?)$`,
			caps: []string{"author"},
		},
		{
			name: "目录模板·括号制式（括号感知捕获类）",
			tpl:  "[{作者}][{作品号}][{标题}]",
			expr: `^\[(?P<author>[^]]+)\]\[(?P<workid>[0-9]+)\]\[(?P<title>[^]]+)\]$`,
			caps: []string{"author", "workid", "title"},
		},
		{
			name: "前字面非开括号收尾·惰性段",
			tpl:  "[x{标签}]",
			expr: `^\[x(?P<tag>.+?)\]$`,
			caps: []string{"tag"},
		},
		{
			name: "数字类词前开括号收尾·维持数字类",
			tpl:  "[{作品号}]",
			expr: `^\[(?P<workid>[0-9]+)\]$`,
			caps: []string{"workid"},
		},
		{
			name: "末位 {任意} 可空",
			tpl:  "[{标签}]{任意}",
			expr: `^\[(?P<tag>[^]]+)\](?:.*)$`,
			caps: []string{"tag"},
		},
		{
			name: "中位 {任意} 保持非空·惰性",
			tpl:  "{任意}[{标签}]",
			expr: `^(?:.+?)\[(?P<tag>[^]]+)\]$`,
			caps: []string{"tag"},
		},
		{
			// 2026-11-04 终审修复批：括号包裹的 {任意} 编译为排除配对
			// 闭括号的非空类（通配段不跨括号组吞并）。
			name: "括号包裹 {任意} 排除闭括号类",
			tpl:  "{集名} [{任意}]",
			expr: `^(?P<setname>.+?) \[(?:[^]]+)\]$`,
			caps: []string{"setname"},
		},
		{
			name: "序列成员模板·键含下划线可匹配",
			tpl:  "{键}_{序号}",
			opts: deriveOpts{allowKey: true, requireSeq: true},
			expr: `^(?P<key>.+?)_(?P<seq>[0-9]+)$`,
			caps: []string{"key", "seq"},
		},
		{
			name: "序列成员模板·新词公共段与旧词同组同义",
			tpl:  "{公共段}_{序号}",
			opts: deriveOpts{allowKey: true, requireSeq: true},
			expr: `^(?P<key>.+?)_(?P<seq>[0-9]+)$`,
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
			expr, caps, _, err := deriveCondxTemplate(tc.tpl, tc.opts)
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

// TestDeriveCondxTemplateBracketPairs 组界六对括号的捕获类配对：前字面段以
// 开括号收尾 → 排除各自配对闭括号的非空类（括号感知捕获类的括号词表与组界
// 扫描共用同一六对词汇）。
func TestDeriveCondxTemplateBracketPairs(t *testing.T) {
	cases := []struct{ name, tpl, expr string }{
		{"半角方括号", "[{标签}]", `^\[(?P<tag>[^]]+)\]$`},
		{"黑头方括号", "【{标签}】", `^【(?P<tag>[^】]+)】$`},
		{"全角圆括号", "（{标签}）", `^（(?P<tag>[^）]+)）$`},
		{"半角圆括号", "({标签})", `^\((?P<tag>[^)]+)\)$`},
		{"单直角引号", "「{标签}」", `^「(?P<tag>[^」]+)」$`},
		{"双直角引号", "『{标签}』", `^『(?P<tag>[^』]+)』$`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, caps, _, err := deriveCondxTemplate(tc.tpl, deriveOpts{})
			if err != nil {
				t.Fatalf("推导失败: %v", err)
			}
			if expr != tc.expr {
				t.Fatalf("表达式不符:\n得到 %q\n期望 %q", expr, tc.expr)
			}
			if !reflect.DeepEqual(caps, []string{"tag"}) {
				t.Fatalf("捕获组不符: 得到 %v", caps)
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

// TestCondxBracketAwareCorpus 断句统一验收语料上的模板行为（括号感知捕获类
// + 末位 {任意} 可空的联合效果）：多组名上 [{标签}] 宁缺不命中、
// [{标签}]{任意} 取首组内名；单组整名（尾空）与中缀单组（Artist[Isel]）
// 行为不变；同对嵌套按首闭截断（已知词法边界）。
func TestCondxBracketAwareCorpus(t *testing.T) {
	const multi = "[七尾ゆきじ] 酔いが醒めるその前に (COMIC 阿吽 2019年2月号) [中国翻訳] [DL版]"
	dirHit := func(tpl, dir string) (*ParseContext, bool) {
		p, err := compileCondx(&Condx{Target: CondxTargetDir, Format: condxTpl(tpl)}, 1)
		if err != nil {
			t.Fatalf("模板 %q 编译失败: %v", tpl, err)
		}
		ctx := &ParseContext{}
		ok := p.Evaluate(&MatchInput{FileName: "01.jpg", DirNames: []string{dir}}, nil, ctx)
		return ctx, ok
	}
	// 多组名：[{标签}] 不命中（捕获类排除闭括号后无法跨界吞并后续组）。
	if _, ok := dirHit("[{标签}]", multi); ok {
		t.Fatalf("[{标签}] 对多组名应不命中")
	}
	// 多组名：[{标签}]{任意} 命中且捕获首组内名。
	ctx, ok := dirHit("[{标签}]{任意}", multi)
	if !ok {
		t.Fatalf("[{标签}]{任意} 对多组名应命中")
	}
	if tag, _ := ctx.Capture("r1.tag"); tag != "七尾ゆきじ" {
		t.Fatalf("首组内名捕获不符: %q", tag)
	}
	// 单组整名：尾空命中。
	ctx, ok = dirHit("[{标签}]{任意}", "[路灯后辈]")
	if !ok {
		t.Fatalf("[{标签}]{任意} 对单组整名应命中（尾空）")
	}
	if tag, _ := ctx.Capture("r1.tag"); tag != "路灯后辈" {
		t.Fatalf("单组整名捕获不符: %q", tag)
	}
	// 中缀单组：Artist[{作者}] 行为不变。
	ctx, ok = dirHit("Artist[{作者}]", "Artist[Isel]")
	if !ok {
		t.Fatalf("Artist[{作者}] 对 Artist[Isel] 应命中")
	}
	if author, _ := ctx.Capture("r1.author"); author != "Isel" {
		t.Fatalf("中缀单组捕获不符: %q", author)
	}
	// 同对嵌套：首闭截断（[{标签}]{任意} 捕获 a[b——组界扫描语义为最外层
	// 内名 a[b]，正则无计数能力，属已知词法边界）。
	ctx, ok = dirHit("[{标签}]{任意}", "[a[b]]")
	if !ok {
		t.Fatalf("[{标签}]{任意} 对同对嵌套应命中")
	}
	if tag, _ := ctx.Capture("r1.tag"); tag != "a[b" {
		t.Fatalf("同对嵌套应按首闭截断捕获 a[b: %q", tag)
	}
	if _, ok := dirHit("[{标签}]", "[a[b]]"); ok {
		t.Fatalf("[{标签}] 对同对嵌套（末闭括号后有残留）应不命中")
	}
	// 尾空模板新命中：{标题}_{任意} 对 a_ 捕获空尾。
	fp, err := compileCondx(&Condx{Target: CondxTargetFile, Format: condxTpl("{标题}_{任意}")}, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	fctx := &ParseContext{}
	if !fp.Evaluate(&MatchInput{FileName: "a_"}, nil, fctx) {
		t.Fatalf("{标题}_{任意} 对 a_ 应命中（尾空）")
	}
	if title, _ := fctx.Capture("r1.title"); title != "a" {
		t.Fatalf("尾空命中标题捕获不符: %q", title)
	}
}

// TestCondxLazyYieldAnchors 填充段让位锚定验收（2026-11-04 终审拍板：乙档
// 全面惰性化——无类锚定贪婪段 .+ 一律编译为惰性 .+?，填充段让位一切占位词）：
// a) 终审实例——[{作者}]{集名} {标签*; } 对多组名目录，旧贪婪下 {集名} 吞到
//    「追悔の滓 [中国翻訳] [無修正]」、重复段只盖 [DL版] 末段；惰性化后
//    {集名} 止于首个满足位（前导空格=模板书写责任，不剪），重复段取满
//    [中国翻訳]/[無修正]/[DL版] 三段（含括号=裸词字面语义裁决——单元值类
//    只排除分隔空格）；b) 写对模板 [{作者}] {集名} [{标签*}]：集名干净、
//    包裹重复段剥括号三标签；c) 填充让位：{标题}_{作者} 对 A_B_C → 标题=A、
//    作者吞尾余段 B_C；d) 类词钉界：{标题}_{序号} 对 A_B_123 → 序号=123
//    （尾随数字类类锚定收界，不被惰性化扰动）。a/b 为 dir 目标、c/d 为
//    file 目标——同一推导函数 deriveCondxTemplate 服务双 target，各抽一例即覆盖。
func TestCondxLazyYieldAnchors(t *testing.T) {
	const dir = "[南方ヒトガクシキ (仲村レグラ)] 追悔の滓 [中国翻訳] [無修正] [DL版]"
	dirHit := func(tpl string) (*ParseContext, bool) {
		p, err := compileCondx(&Condx{Target: CondxTargetDir, Format: condxTpl(tpl)}, 1)
		if err != nil {
			t.Fatalf("模板 %q 编译失败: %v", tpl, err)
		}
		ctx := &ParseContext{}
		ok := p.Evaluate(&MatchInput{FileName: "01.jpg", DirNames: []string{dir}}, nil, ctx)
		return ctx, ok
	}
	fileHit := func(tpl, fileName string) (*ParseContext, bool) {
		p, err := compileCondx(&Condx{Target: CondxTargetFile, Format: condxTpl(tpl)}, 1)
		if err != nil {
			t.Fatalf("模板 %q 编译失败: %v", tpl, err)
		}
		ctx := &ParseContext{}
		ok := p.Evaluate(&MatchInput{FileName: fileName}, nil, ctx)
		return ctx, ok
	}

	// a) 终审实例（dir）：3 标签绑定（含括号）+ 集名带前导空格。
	ctx, ok := dirHit("[{作者}]{集名} {标签*; }")
	if !ok {
		t.Fatalf("终审实例模板应命中")
	}
	if author, _ := ctx.Capture("r1.author"); author != "南方ヒトガクシキ (仲村レグラ)" {
		t.Fatalf("作者捕获不符: %q", author)
	}
	if setname, _ := ctx.Capture("r1.setname"); setname != " 追悔の滓" {
		t.Fatalf("集名应止于首个满足位（前导空格=模板书写责任，不剪）: %q", setname)
	}
	tags := ctx.CaptureAll("r1.tag")
	if len(tags) != 3 || tags[0].Value != "[中国翻訳]" || tags[1].Value != "[無修正]" || tags[2].Value != "[DL版]" {
		t.Fatalf("应 3 标签绑定（含括号=裸词字面语义）: %+v", tags)
	}

	// b) 写对模板（dir）：集名干净、包裹重复段剥括号三标签。
	ctx, ok = dirHit("[{作者}] {集名} [{标签*}]")
	if !ok {
		t.Fatalf("写对模板应命中")
	}
	if setname, _ := ctx.Capture("r1.setname"); setname != "追悔の滓" {
		t.Fatalf("集名应干净（无前导空格）: %q", setname)
	}
	tags = ctx.CaptureAll("r1.tag")
	if len(tags) != 3 || tags[0].Value != "中国翻訳" || tags[1].Value != "無修正" || tags[2].Value != "DL版" {
		t.Fatalf("包裹重复段应剥括号 3 标签: %+v", tags)
	}

	// c) 填充让位（file）：标题让位，作者吞尾余段。
	ctx, ok = fileHit("{标题}_{作者}", "A_B_C")
	if !ok {
		t.Fatalf("{标题}_{作者} 对 A_B_C 应命中")
	}
	if title, _ := ctx.Capture("r1.title"); title != "A" {
		t.Fatalf("标题应让位取最短: %q", title)
	}
	if author, _ := ctx.Capture("r1.author"); author != "B_C" {
		t.Fatalf("作者应吞尾余段: %q", author)
	}

	// d) 类词钉界（file）：尾随数字类收界不被惰性化扰动。
	ctx, ok = fileHit("{标题}_{序号}", "A_B_123")
	if !ok {
		t.Fatalf("{标题}_{序号} 对 A_B_123 应命中")
	}
	if seq, _ := ctx.Capture("r1.seq"); seq != "123" {
		t.Fatalf("序号应类锚定取尾: %q", seq)
	}
	if title, _ := ctx.Capture("r1.title"); title != "A_B" {
		t.Fatalf("标题应回溯扩到数字类界前: %q", title)
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
		{
			name: "第1级与父目录模板互斥",
			c: Condx{Target: CondxTargetDir, Format: condxTpl("{标签}"), Attach: []CondxAttach{
				{Kind: CondxAttachDirLevel, MinCount: 1},
				{Kind: CondxAttachParentFormat, Tpl: "monster girl"},
			}},
			wantErr: "「位于第 1 级」与父目录条件不能同用：第 1 级目录的上面是库根本身，没有可匹配的父目录名",
		},
		{
			name: "第1级与父目录包含词互斥",
			c: Condx{Target: CondxTargetDir, Format: condxTpl("{标签}"), Attach: []CondxAttach{
				{Kind: CondxAttachDirLevel, MinCount: 1},
				{Kind: CondxAttachParentContains, Word: "pixiv"},
			}},
			wantErr: "与父目录条件不能同用：第 1 级目录的上面是库根本身",
		},
		{
			name: "第2级与父目录模板合法共存",
			c: Condx{Target: CondxTargetDir, Format: condxTpl("[{标签}]"), Attach: []CondxAttach{
				{Kind: CondxAttachDirLevel, MinCount: 2},
				{Kind: CondxAttachParentFormat, Tpl: "monster girl"},
			}},
		},
		{
			name:    "dirLevel 缺数量",
			c:       Condx{Target: CondxTargetDir, Format: condxTpl("{标签}"), Attach: []CondxAttach{{Kind: CondxAttachDirLevel}}},
			wantErr: "数量门槛",
		},
		{
			name:    "dirLevel 用于文件目标",
			c:       Condx{Target: CondxTargetFile, Format: condxTpl("{作者}"), Attach: []CondxAttach{{Kind: CondxAttachDirLevel, MinCount: 1}}},
			wantErr: "不适用于目标",
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

// TestDirSubjectDirLevelAssert 层级锚定谓词「位于第 N 级」（自识别根数起，
// 根直下=第 1 级）：全通配模板不再被更深层吞掉——仅第 N 级层可命中绑定
// （DirHit 记于 N 级层）；N 超深度（祖先链上无第 N 级）整行不中（严格不回退）；
// N≥2 与父目录谓词合取（决策4 场景丁的组合形态）。
func TestDirSubjectDirLevelAssert(t *testing.T) {
	c := &Condx{
		Target: CondxTargetDir,
		Format: condxTpl("{标签}"),
		Attach: []CondxAttach{{Kind: CondxAttachDirLevel, MinCount: 1}},
	}
	p, err := compileCondx(c, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	in := &MatchInput{FileName: "01.jpg", DirNames: []string{"monster girl", "album"}}
	ctx := &ParseContext{}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatalf("锚定第 1 级应命中")
	}
	tag, _ := ctx.Capture("r1.tag")
	if tag != "monster girl" {
		t.Fatalf("应只绑第 1 级目录名（不被更深层吞掉）: tag=%q", tag)
	}
	if len(ctx.DirHits) != 1 || ctx.DirHits[0].Name != "monster girl" || ctx.DirHits[0].Depth != 0 {
		t.Fatalf("DirHit 应记于第 1 级层: %+v", ctx.DirHits)
	}
	// N 超深度：两层夹具上要第 3 级——祖先链上无第 N 级，整行不中。
	over, err := compileCondx(&Condx{Target: CondxTargetDir, Format: condxTpl("{标签}"),
		Attach: []CondxAttach{{Kind: CondxAttachDirLevel, MinCount: 3}}}, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	if over.Evaluate(in, nil, &ParseContext{}) {
		t.Fatalf("N 超深度应整行不中（严格不回退）")
	}
	// dirLevel2 + parentFormat 合取（场景丁）：[{标签}] 目录位于第 2 级且父名整条匹配。
	dk, err := compileCondx(&Condx{Target: CondxTargetDir, Format: condxTpl("[{标签}]"),
		Attach: []CondxAttach{
			{Kind: CondxAttachDirLevel, MinCount: 2},
			{Kind: CondxAttachParentFormat, Tpl: "monster girl"},
		}}, 1)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	dctx := &ParseContext{}
	if !dk.Evaluate(&MatchInput{FileName: "01.jpg", DirNames: []string{"monster girl", "[cool]"}}, nil, dctx) {
		t.Fatalf("第 2 级且父为 monster girl 应命中")
	}
	if tag, _ := dctx.Capture("r1.tag"); tag != "cool" {
		t.Fatalf("第 2 级目录捕获不符: tag=%q", tag)
	}
	if len(dctx.DirHits) != 1 || dctx.DirHits[0].Depth != 1 {
		t.Fatalf("DirHit 应记于第 2 级层: %+v", dctx.DirHits)
	}
	if dk.Evaluate(&MatchInput{FileName: "01.jpg", DirNames: []string{"misc", "[cool]"}}, nil, &ParseContext{}) {
		t.Fatalf("父名不符时合取不应命中")
	}
}

// TestRowSubjectDirLevelHitMerge 行包装并回通道（声明5 第 1 路，AE 阶段5 核验）：
// 行内主体在私有子上下文求值后，DirHits 随行号前缀捕获一并并回主上下文——
// 并回是原值搬运、不改写深度：层级锚规则只在 N 级层记录 DirHit，深层同名形态
// 不产生第二命中；两行规则的捕获（r1/r2 前缀）同时在场。
func TestRowSubjectDirLevelHitMerge(t *testing.T) {
	rows := []CondxRow{
		{Condition: &Condx{Target: CondxTargetDir, Format: condxTpl("{标签}"),
			Attach: []CondxAttach{{Kind: CondxAttachDirLevel, MinCount: 1}}}},
		{Connector: "and", Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{标题}")}},
	}
	p, err := CompileRuleConditions(rows)
	if err != nil {
		t.Fatalf("编译失败: %v", err)
	}
	ctx := &ParseContext{}
	in := &MatchInput{FileName: "落日.jpg", DirNames: []string{"Top", "Inner"}}
	if !p.Evaluate(in, nil, ctx) {
		t.Fatalf("第 1 级目录行 + 文件行应命中（深层同名不吞锚定层）")
	}
	// 并回后的 DirHits 仅有第 1 级一条：锚定层=命中层口径在并回环节不被改写。
	if len(ctx.DirHits) != 1 || ctx.DirHits[0].Depth != 0 || ctx.DirHits[0].Name != "Top" {
		t.Fatalf("并回的 DirHits 应仅第 1 级一条: %+v", ctx.DirHits)
	}
	if tag, _ := ctx.Capture("r1.tag"); tag != "Top" {
		t.Fatalf("第 1 行捕获应绑第 1 级层: %q", tag)
	}
	if title, _ := ctx.Capture("r2.title"); title != "落日" {
		t.Fatalf("第 2 行捕获应并回主上下文: %q", title)
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
	// 2026-11-04 终审拍板（惰性化）：作者段取最短（首 _ 前），载体名由双
	// 下划线形变换为空格形变（fold 仍唯一命中 Jake_Jaccobus）。
	if !p.Evaluate(&MatchInput{FileName: "jake jaccobus_sunset.jpg"}, lib, ctx) {
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
	// 2026-11-04 终审拍板（惰性化）：分隔改空格字面——含 _ 的库名在 _
	// 分隔模板下已不可整名捕获（作者段止于首 _）。
	c2 := &Condx{Target: CondxTargetFile, Format: condxTpl("{作者} {标题}"),
		Badges: []CondxBadge{{Placeholder: "作者", Kind: CondxBadgeLibrary, Dim: "author"}}}
	p2, _ := compileCondx(c2, 1)
	if !p2.Evaluate(&MatchInput{FileName: "Jake_Jaccobus sunset.jpg"}, lib, &ParseContext{}) {
		t.Fatalf("精确名单应命中")
	}
	if p2.Evaluate(&MatchInput{FileName: "jake jaccobus sunset.jpg"}, lib, &ParseContext{}) {
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

// 占位词量词变体词法解析（任务A）：只测 parseCondxVariant 与触发标记判定，
// 不接入 deriveCondxTemplate 主流程（接入归任务B）。
func TestCondxVariantParse(t *testing.T) {
	green := []struct {
		name string
		word string
		want condxVariant
	}{
		{name: "标签·至少一次", word: "标签+", want: condxVariant{En: "tag", AtLeastOne: true}},
		{name: "标签·可零次", word: "标签*", want: condxVariant{En: "tag"}},
		{name: "标签·显式分隔带字面", word: "标签+;_", want: condxVariant{En: "tag", AtLeastOne: true, SepSpecified: true, Sep: ";_"}},
		{name: "标签·分隔仅为分号", word: "标签+;", want: condxVariant{En: "tag", AtLeastOne: true, SepSpecified: true, Sep: ";"}},
		{name: "作者·可零次", word: "作者*", want: condxVariant{En: "author"}},
		{name: "集名·至少一次", word: "集名+", want: condxVariant{En: "setname", AtLeastOne: true}},
	}
	for _, tc := range green {
		got, err := parseCondxVariant(tc.word)
		if err != nil {
			t.Fatalf("%s：期望解析成功，得到错误 %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s：期望 %+v，得到 %+v", tc.name, tc.want, got)
		}
	}
	red := []struct {
		name    string
		word    string
		wantErr string
	}{
		{name: "非可重复词位带量词", word: "标题+", wantErr: "仅 标签/作者/集名 可带量词"},
		{name: "吞位词带量词", word: "任意+", wantErr: "不可重复"},
		{name: "量词后残留非分号开头", word: "标签++", wantErr: "语法错误"},
		{name: "仅分隔无量词", word: "标签;_", wantErr: "语法错误"},
	}
	for _, tc := range red {
		if _, err := parseCondxVariant(tc.word); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Fatalf("%s：期望错误含 %q，得到 %v", tc.name, tc.wantErr, err)
		}
	}
	if hasCondxVariantMark("标签") || !hasCondxVariantMark("标签+") || !hasCondxVariantMark("标签;_") {
		t.Fatalf("变体标记触发判定错误：不含标记的词不应触发，含 +/*/; 的词应触发")
	}
}

// TestDeriveCondxTemplateRepeat 重复段表达（任务B）：量词变体占位词接入
// deriveCondxTemplate——整段恰一个位置捕获组（非命名，SubexpNames 组名为
// 空串、不进声明收集，即引用禁区词法根基）、单元值类为排除类、描述四字段
// （词根组名/位置组序号/单元子模式/排除集）；裸词无分隔、seq 主体行、
// 同词根重复段返回中文错误。
func TestDeriveCondxTemplateRepeat(t *testing.T) {
	type matchCase struct {
		input string
		want  []string // 各重复段整段位置组的期望匹配文本（按描述序），“<无>”=组未参与
	}
	green := []struct {
		name    string
		tpl     string
		opts    deriveOpts
		expr    string
		caps    []string
		reps    []condxRepeatDesc
		matches []matchCase
	}{
		{
			name:    "包裹1+·缺省分隔",
			tpl:     "[{标签+}]",
			expr:    `^\[([^]]+(?:\]\s*\[[^]]+)*)\]$`,
			reps:    []condxRepeatDesc{{En: "tag", GroupIndex: 1, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}},
			matches: []matchCase{{input: "[a] [b] [c]", want: []string{"a] [b] [c"}}, {input: "[x]", want: []string{"x"}}},
		},
		{
			name:    "包裹0+·整段含括号外套可缺形态",
			tpl:     "[{标签*}]",
			expr:    `^(?:\[([^]]+(?:\]\s*\[[^]]+)*)\])?$`,
			reps:    []condxRepeatDesc{{En: "tag", GroupIndex: 1, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}},
			matches: []matchCase{{input: "[a] [b]", want: []string{"a] [b"}}, {input: "", want: []string{"<无>"}}},
		},
		{
			name:    "包裹1+·显式分隔",
			tpl:     "[{标签+;_}]",
			expr:    `^\[([^]]+(?:\]_\[[^]]+)*)\]$`,
			reps:    []condxRepeatDesc{{En: "tag", GroupIndex: 1, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}},
			matches: []matchCase{{input: "[a]_[b]", want: []string{"a]_[b"}}},
		},
		{
			name:    "裸词1+·排除分隔字面全部字符",
			tpl:     "{标题}={标签+;_}",
			// 2026-11-04 终审拍板（惰性化）：标题段 .+ 翻新为 .+?（重复段位置组类内最大不变）。
			expr:    `^(?P<title>.+?)=([^_]+(?:_[^_]+)*)$`,
			caps:    []string{"title"},
			reps:    []condxRepeatDesc{{En: "tag", GroupIndex: 2, UnitPattern: `([^_]+)`, Exclude: "_"}},
			matches: []matchCase{{input: "标题甲=tag1_tag2", want: []string{"tag1_tag2"}}},
		},
		{
			name:    "裸词1+·前开括号不成包裹则原样回放",
			tpl:     "[{标签+;_} 第2卷]",
			expr:    `^\[([^_]+(?:_[^_]+)*) 第2卷\]$`,
			reps:    []condxRepeatDesc{{En: "tag", GroupIndex: 1, UnitPattern: `([^_]+)`, Exclude: "_"}},
			matches: []matchCase{{input: "[a_b 第2卷]", want: []string{"a_b"}}},
		},
		{
			name: "不同词根多重复段并存",
			tpl:  "[{标签+}]_[{作者*}]",
			expr: `^\[([^]]+(?:\]\s*\[[^]]+)*)\]_(?:\[([^]]+(?:\]\s*\[[^]]+)*)\])?$`,
			reps: []condxRepeatDesc{{En: "tag", GroupIndex: 1, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}, {En: "author", GroupIndex: 2, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}},
			matches: []matchCase{
				{input: "[a] [b]_[c] [d]", want: []string{"a] [b", "c] [d"}},
				{input: "[a]_", want: []string{"a", "<无>"}},
			},
		},
	}
	for _, tc := range green {
		t.Run(tc.name, func(t *testing.T) {
			expr, caps, reps, err := deriveCondxTemplate(tc.tpl, tc.opts)
			if err != nil {
				t.Fatalf("推导失败: %v", err)
			}
			if expr != tc.expr {
				t.Fatalf("表达式不符:\n得到 %q\n期望 %q", expr, tc.expr)
			}
			if !reflect.DeepEqual(caps, tc.caps) {
				t.Fatalf("捕获组不符: 得到 %v 期望 %v", caps, tc.caps)
			}
			if !reflect.DeepEqual(reps, tc.reps) {
				t.Fatalf("重复段描述不符:\n得到 %+v\n期望 %+v", reps, tc.reps)
			}
			re := regexp.MustCompile(expr)
			for _, m := range tc.matches {
				idx := re.FindStringSubmatchIndex(m.input)
				if idx == nil {
					t.Fatalf("%q 应命中 %q", m.input, expr)
				}
				for gi, want := range m.want {
					pos := reps[gi].GroupIndex
					lo, hi := idx[2*pos], idx[2*pos+1]
					got := "<无>"
					if lo >= 0 {
						got = m.input[lo:hi]
					}
					if got != want {
						t.Fatalf("%q 位置组 %d 不符: 得到 %q 期望 %q", m.input, pos, got, want)
					}
				}
			}
			// 引用禁区词法根基：整段为位置捕获组（非命名），组名为空串。
			for _, d := range reps {
				if re.SubexpNames()[d.GroupIndex] != "" {
					t.Fatalf("重复段整段应为位置捕获组（非命名），组 %d 得到名 %q", d.GroupIndex, re.SubexpNames()[d.GroupIndex])
				}
			}
		})
	}
	// 语义底线抽查：单元子模式对整段匹配文本可再拆（阶段2 绑定层再拆口径）。
	expr, _, reps, _ := deriveCondxTemplate("{标题}={标签+;_}", deriveOpts{})
	re := regexp.MustCompile(expr)
	m := re.FindStringSubmatchIndex("标题甲=tag1_tag2")
	lo, hi := m[2*reps[0].GroupIndex], m[2*reps[0].GroupIndex+1]
	units := regexp.MustCompile(reps[0].UnitPattern).FindAllStringSubmatch("标题甲=tag1_tag2"[lo:hi], -1)
	if len(units) != 2 || units[0][1] != "tag1" || units[1][1] != "tag2" {
		t.Fatalf("单元子模式再拆不符: %v", units)
	}
	red := []struct {
		name    string
		tpl     string
		opts    deriveOpts
		wantErr string
	}{
		{name: "裸词无分隔", tpl: "{标题}={标签+}", wantErr: "缺分隔字面"},
		{name: "seq主体行用重复词", tpl: "{公共段}_{标签+}_{序号}", opts: deriveOpts{allowKey: true, requireSeq: true}, wantErr: "序列成员格式不支持重复段"},
		{name: "同词根重复段并用", tpl: "[{标签+}]_[{标签*}]", wantErr: "重复"},
		{name: "重复词与单词同词根并用", tpl: "{标签}_[{标签+}]", wantErr: "重复"},
		{name: "裸词分隔字面为空", tpl: "{标题}={标签+;}", wantErr: "分隔字面不能为空"},
	}
	for _, tc := range red {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := deriveCondxTemplate(tc.tpl, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
			}
		})
	}
}

// TestDeriveCondxTemplateOptional 可选记号推导锚（阶段1，相似模板可选分隔表达
// 方案）：{字面?} 可选分隔字面编译 (?:字面)?（QuoteMeta，{what??} 双写表达可选
// what?）；{词?} 可选词位编译 (?:(?P<词>捕获类))?（捕获类沿用数字类/括号感知
// 类/惰性段三分支）；包裹词可选复用量词变体的暂扣开括号/闭括号去重机制——
// 开括号+词+闭括号整组同进退、不产「空括号命中」、后闭括号不配对时开括号按
// 字面回放走裸词可选；位置约束（决策4）=可选分隔两侧至少一侧非空必选字面段
// 或模板首/尾——0+ 包裹组吞闭括号后的空残留不作锚（风险5 已知边界）、右邻开
// 括号将被收进可缺形态时同理；拒绝面覆盖 {?}/{任意?}/{公共段?}{键?}/seq 词位/
// 两侧无锚/{标签+?} 既有拒绝/同词根重复。
func TestDeriveCondxTemplateOptional(t *testing.T) {
	type matchCase struct {
		input string
		want  map[string]string // 命名组→期望值；"<无>"=组未参与（词位缺席）
	}
	green := []struct {
		name    string
		tpl     string
		opts    deriveOpts
		expr    string
		caps    []string
		reps    []condxRepeatDesc
		matches []matchCase
		misses  []string
	}{
		{
			name: "可选空格分隔·双形态通吃（todo#100 实例）",
			tpl:  "[{作者}]{ ?}{集名}",
			expr: `^\[(?P<author>[^]]+)\](?: )?(?P<setname>.+?)$`,
			caps: []string{"author", "setname"},
			matches: []matchCase{
				{"[A]S", map[string]string{"author": "A", "setname": "S"}},
				{"[A] S", map[string]string{"author": "A", "setname": "S"}},
			},
		},
		{
			name: "多字符可选分隔（空格-连字符-空格）",
			tpl:  "[{作者}]{ - ?}{集名}",
			expr: `^\[(?P<author>[^]]+)\](?: - )?(?P<setname>.+?)$`,
			caps: []string{"author", "setname"},
			matches: []matchCase{
				{"[A]-S", map[string]string{"author": "A", "setname": "-S"}},
				{"[A] - S", map[string]string{"author": "A", "setname": "S"}},
			},
		},
		{
			name: "包裹词可选·整组同进退不产空括号命中",
			tpl:  "[{作者?}] {集名}",
			expr: `^(?:\[(?P<author>[^]]+)\])? (?P<setname>.+?)$`,
			caps: []string{"author", "setname"},
			matches: []matchCase{
				{"[Isel] Set", map[string]string{"author": "Isel", "setname": "Set"}},
				{" Set", map[string]string{"author": "<无>", "setname": "Set"}},
			},
			misses: []string{"[] Set", "Set"},
		},
		{
			name: "裸词可选·惰性段",
			tpl:  "{标题?}",
			expr: `^(?:(?P<title>.+?))?$`,
			caps: []string{"title"},
			matches: []matchCase{
				{"X", map[string]string{"title": "X"}},
				{"", map[string]string{"title": "<无>"}},
			},
		},
		{
			name: "裸词可选·数字类沿用",
			tpl:  "第{序号?}话",
			expr: `^第(?:(?P<seq>[0-9]+))?话$`,
			caps: []string{"seq"},
			matches: []matchCase{
				{"第12话", map[string]string{"seq": "12"}},
				{"第话", map[string]string{"seq": "<无>"}},
			},
		},
		{
			name: "前开括号不配对·回放开括号走裸词可选（括号感知类）",
			tpl:  "[{标题?}x",
			expr: `^\[(?:(?P<title>[^]]+))?x$`,
			caps: []string{"title"},
			matches: []matchCase{
				{"[abx", map[string]string{"title": "ab"}},
				{"[x", map[string]string{"title": "<无>"}},
			},
		},
		{
			name:    "可选字面自身以问号结尾·双写表达",
			tpl:     "{what??}",
			expr:    `^(?:what\?)?$`,
			matches: []matchCase{{"what?", nil}, {"", nil}},
			misses:  []string{"what"},
		},
		{
			name: "尾随可选分隔不进入取值（风险3 锚：leftmost-first 下惰性段先取满足位）",
			tpl:  "{标题}{ ?}",
			expr: `^(?P<title>.+?)(?: )?$`,
			caps: []string{"title"},
			matches: []matchCase{
				{"a ", map[string]string{"title": "a"}},
				{"a", map[string]string{"title": "a"}},
				{"a b", map[string]string{"title": "a b"}},
			},
		},
		{
			name: "首部可选分隔·模板首为锚",
			tpl:  "{ ?}{集名}",
			expr: `^(?: )?(?P<setname>.+?)$`,
			caps: []string{"setname"},
			matches: []matchCase{
				{" Set", map[string]string{"setname": "Set"}},
				{"Set", map[string]string{"setname": "Set"}},
			},
		},
		{
			name: "0+包裹重复组后接可选分隔·闭括号去重后非空残留作锚",
			tpl:  "[{标签*}]x{ ?}{标题}",
			expr: `^(?:\[([^]]+(?:\]\s*\[[^]]+)*)\])?x(?: )?(?P<title>.+?)$`,
			caps: []string{"title"},
			reps: []condxRepeatDesc{{En: "tag", GroupIndex: 1, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}},
			matches: []matchCase{
				{"x T", map[string]string{"title": "T"}},
				{"[a] [b]x T", map[string]string{"title": "T"}},
			},
		},
		{
			name: "右邻开括号由1+包裹重复段承载·仍为必选锚",
			tpl:  "{集名}{ ?}[{标签+}]",
			expr: `^(?P<setname>.+?)(?: )?\[([^]]+(?:\]\s*\[[^]]+)*)\]$`,
			caps: []string{"setname"},
			reps: []condxRepeatDesc{{En: "tag", GroupIndex: 2, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}},
			matches: []matchCase{
				{"a [b] [c]", map[string]string{"setname": "a"}},
				{"a[b]", map[string]string{"setname": "a"}},
			},
		},
		{
			name: "重复段与可选词位同模板并存（次数轴正交）",
			tpl:  "[{标签*}]_{标题?}",
			expr: `^(?:\[([^]]+(?:\]\s*\[[^]]+)*)\])?_(?:(?P<title>.+?))?$`,
			caps: []string{"title"},
			reps: []condxRepeatDesc{{En: "tag", GroupIndex: 1, UnitPattern: `\[([^]]+)\]`, Exclude: "]"}},
			matches: []matchCase{
				{"[a] [b]_T", map[string]string{"title": "T"}},
				{"_T", map[string]string{"title": "T"}},
			},
		},
		{
			name: "序列成员模板·可选分隔字面不受限",
			tpl:  "{公共段}_{序号}{ ?}",
			opts: deriveOpts{allowKey: true, requireSeq: true},
			expr: `^(?P<key>.+?)_(?P<seq>[0-9]+)(?: )?$`,
			caps: []string{"key", "seq"},
			matches: []matchCase{
				{"a_1", map[string]string{"key": "a", "seq": "1"}},
				{"a_1 ", map[string]string{"key": "a", "seq": "1"}},
			},
		},
	}
	for _, tc := range green {
		t.Run(tc.name, func(t *testing.T) {
			expr, caps, reps, err := deriveCondxTemplate(tc.tpl, tc.opts)
			if err != nil {
				t.Fatalf("推导失败: %v", err)
			}
			if expr != tc.expr {
				t.Fatalf("表达式不符:\n得到 %q\n期望 %q", expr, tc.expr)
			}
			if !reflect.DeepEqual(caps, tc.caps) {
				t.Fatalf("捕获组不符: 得到 %v 期望 %v", caps, tc.caps)
			}
			if !reflect.DeepEqual(reps, tc.reps) {
				t.Fatalf("重复段描述不符:\n得到 %+v\n期望 %+v", reps, tc.reps)
			}
			re := regexp.MustCompile(expr)
			for _, m := range tc.matches {
				idx := re.FindStringSubmatchIndex(m.input)
				if idx == nil {
					t.Fatalf("%q 应命中 %q", m.input, expr)
				}
				for name, want := range m.want {
					gi := re.SubexpIndex(name)
					if gi < 0 {
						t.Fatalf("命名组 %s 不在表达式内", name)
					}
					lo, hi := idx[2*gi], idx[2*gi+1]
					got := "<无>"
					if lo >= 0 {
						got = m.input[lo:hi]
					}
					if got != want {
						t.Fatalf("%q 命名组 %s 不符: 得到 %q 期望 %q", m.input, name, got, want)
					}
				}
			}
			for _, s := range tc.misses {
				if re.FindStringIndex(s) != nil {
					t.Fatalf("%q 不应命中 %q", s, expr)
				}
			}
		})
	}
	red := []struct {
		name    string
		tpl     string
		opts    deriveOpts
		wantErr string
	}{
		{name: "问号前无可选内容", tpl: "{?}", wantErr: "问号前无可选内容"},
		{name: "吞位词不可带问号", tpl: "{任意?}", wantErr: "不产出声明、无可选语义，不可带问号"},
		{name: "公共段不可带问号", tpl: "{公共段?}", wantErr: "成套的成群依据，不可带问号"},
		{name: "旧词键经别名归公共段同拒", tpl: "{键?}", wantErr: "成套的成群依据"},
		{name: "序列成员禁可选词位", tpl: "{公共段}_{标题?}_{序号}", opts: deriveOpts{allowKey: true, requireSeq: true}, wantErr: "序列成员格式不支持可选词位"},
		{name: "两取值词之间可选分隔无字面锚", tpl: "{作者}{ ?}{集名}", wantErr: "可选分隔两侧均无字面锚"},
		{name: "相邻两可选分隔天然被拒", tpl: "{ ?}{_?}", wantErr: "可选分隔两侧均无字面锚"},
		{name: "0+包裹重复组后接可选分隔·闭括号被吞无锚（风险5 已知边界）", tpl: "[{标签*}]{ ?}{标题}", wantErr: "可选分隔两侧均无字面锚"},
		{name: "可选包裹组后接可选分隔·闭括号被吞无锚（同机制）", tpl: "[{作者?}]{ ?}{集名}", wantErr: "可选分隔两侧均无字面锚"},
		{name: "右邻开括号将被0+包裹组收进可缺形态·无必选锚", tpl: "{集名}{ ?}[{标签*}]", wantErr: "可选分隔两侧均无字面锚"},
		{name: "右邻开括号将被包裹可选词位收进可缺形态·无必选锚", tpl: "{集名}{ ?}[{作者?}]", wantErr: "可选分隔两侧均无字面锚"},
		{name: "量词后问号·既有拒绝不回归", tpl: "{标签+?}", wantErr: "量词后只能跟分号引出的分隔字面"},
		{name: "可选词位与单词同词根重复", tpl: "{作者?}_{作者}", wantErr: "重复"},
		{name: "可选词位双写同词根重复", tpl: "{作者?}_{作者?}", wantErr: "重复"},
		{name: "重复段与可选词位同词根重复", tpl: "[{标签*}]_{标签?}", wantErr: "重复"},
		{name: "可选词位仍受相邻占位词约束", tpl: "{作者}{标题?}", wantErr: "缺少分隔"},
	}
	for _, tc := range red {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := deriveCondxTemplate(tc.tpl, tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("期望错误含 %q，得到 %v", tc.wantErr, err)
			}
		})
	}
}

// TestCondxOptionalEvaluate 可选记号求值锚（阶段1）：[{作者}]{ ?}{集名} 一条
// 规则通吃有无空格两形态并取值（todo#100 验收）；[{作者?}] {集名} 包裹组可选
// ——在场单值捕获、缺席仍命中、空括号不命中（缺席=无绑定的登记语义属阶段2
// 绑定层，此处只锚命中与在场取值）；{标题}{ ?} 尾随可选分隔不进入取值（风险3：
// leftmost-first 下惰性段先取满足位）。
func TestCondxOptionalEvaluate(t *testing.T) {
	fileHit := func(tpl, fileName string) (*ParseContext, bool) {
		p, err := compileCondx(&Condx{Target: CondxTargetFile, Format: condxTpl(tpl)}, 1)
		if err != nil {
			t.Fatalf("模板 %q 编译失败: %v", tpl, err)
		}
		ctx := &ParseContext{}
		ok := p.Evaluate(&MatchInput{FileName: fileName}, nil, ctx)
		return ctx, ok
	}

	// todo#100 实例：一条规则通吃 [{作者}]{集名} 与 [{作者}] {集名} 两形态。
	ctx, ok := fileHit("[{作者}]{ ?}{集名}", "[Isel]Set.jpg")
	if !ok {
		t.Fatalf("无分隔形态应命中")
	}
	if author, _ := ctx.Capture("r1.author"); author != "Isel" {
		t.Fatalf("无分隔形态作者捕获不符: %q", author)
	}
	if setname, _ := ctx.Capture("r1.setname"); setname != "Set" {
		t.Fatalf("无分隔形态集名捕获不符: %q", setname)
	}
	ctx, ok = fileHit("[{作者}]{ ?}{集名}", "[Isel] Set.jpg")
	if !ok {
		t.Fatalf("空格分隔形态应命中")
	}
	if author, _ := ctx.Capture("r1.author"); author != "Isel" {
		t.Fatalf("空格分隔形态作者捕获不符: %q", author)
	}
	if setname, _ := ctx.Capture("r1.setname"); setname != "Set" {
		t.Fatalf("空格分隔形态集名捕获不符: %q", setname)
	}

	// 包裹组可选：在场单值捕获；缺席仍命中（字面空格仍必选）；空括号不命中。
	ctx, ok = fileHit("[{作者?}] {集名}", "[Isel] Set.jpg")
	if !ok {
		t.Fatalf("包裹组在场形态应命中")
	}
	if author, _ := ctx.Capture("r1.author"); author != "Isel" {
		t.Fatalf("包裹组在场作者捕获不符: %q", author)
	}
	if setname, _ := ctx.Capture("r1.setname"); setname != "Set" {
		t.Fatalf("包裹组在场集名捕获不符: %q", setname)
	}
	ctx, ok = fileHit("[{作者?}] {集名}", " Set.jpg")
	if !ok {
		t.Fatalf("包裹组缺席形态应命中（缺席登记语义属阶段2 绑定层）")
	}
	if setname, _ := ctx.Capture("r1.setname"); setname != "Set" {
		t.Fatalf("包裹组缺席集名捕获不符: %q", setname)
	}
	if _, ok := fileHit("[{作者?}] {集名}", "[] Set.jpg"); ok {
		t.Fatalf("空括号不应命中（开括号+词+闭括号同进退）")
	}
	if _, ok := fileHit("[{作者?}] {集名}", "Set.jpg"); ok {
		t.Fatalf("缺字面空格的形态不应命中")
	}

	// 尾随可选分隔不进入取值（风险3 锚）。
	ctx, ok = fileHit("{标题}{ ?}", "name .jpg")
	if !ok {
		t.Fatalf("尾随可选分隔形态应命中")
	}
	if title, _ := ctx.Capture("r1.title"); title != "name" {
		t.Fatalf("尾随可选分隔不应进入取值: %q", title)
	}
	if _, ok := fileHit("{标题}{ ?}", "name.jpg"); !ok {
		t.Fatalf("无尾随分隔形态应命中")
	}
}
