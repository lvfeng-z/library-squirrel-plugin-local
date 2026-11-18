// 伴随说明内容绑定与声明产出通路的单元测试：绑定结构载入校验（行号范围/
// 维度合法/同行同维唯一/元数据规则内容取值拒绝/存量零值兼容）、两相提取的
// 命中集登记与汇流后整段取值产声明（空内容零声明、截断标注、赋值覆盖重放）。
package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// contentFileRow 构造文件目标条件行（模板 + 扩展名谓词，与预置种子轨行同形态）。
func contentFileRow(tpl, ext string) CondxRow {
	row := CondxRow{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}
	if ext != "" {
		row.Condition.Attach = []CondxAttach{{Kind: CondxAttachFileExt, Value: ext}}
	}
	return row
}

// contentAggregateEntryJSON 构造带内容绑定的聚合规则中间形态（两行：图片轨行
// + 文本行，绑定文本行内容→描述；绑定住规则级「元数据文件」产出落档位）。
func contentAggregateEntryJSON(bindings []ContentBinding) *ruleEntryJSON {
	return &ruleEntryJSON{
		ID:          "agg-content",
		Description: "图片+说明文本聚合（内容绑定）",
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			contentFileRow("{作品}", ".jpg"),
			contentFileRow("{作品}", ".txt"),
		},
		Identity: []IdentityPart{{Row: 1, Capture: "work"}},
		Aggregate: &AggregateProduction{
			WorkType: sdkdto.ResourceTypeArticle,
			Tracks: []TrackAssign{
				{Row: 1, Role: contract.StoreTypeImage},
				{Row: 2, Role: contract.StoreTypeDocument},
			},
		},
		MetadataFile: &MetadataFileProduction{ContentBindings: bindings},
	}
}

// TestContentSourceBindingLoadValidation 内容绑定的载入校验：合法形态载入并
// 往返保真；行号越界/维度非法/同行同维重复指派拒绝、同行多目标（同维互异）
// 放行；元数据规则的提取动作携带内容取值拒绝；无绑定键的旧档零值兼容。
func TestContentSourceBindingLoadValidation(t *testing.T) {
	ok := contentAggregateEntryJSON([]ContentBinding{{Row: 2, Dimension: DimDescription}})
	entry, err := ruleFromJSON(ok)
	if err != nil {
		t.Fatalf("合法内容绑定应载入: %v", err)
	}
	if len(entry.MetadataFile.ContentBindings) != 1 ||
		entry.MetadataFile.ContentBindings[0].Row != 2 ||
		entry.MetadataFile.ContentBindings[0].Dimension != DimDescription {
		t.Fatalf("内容绑定应原样载入: %+v", entry.MetadataFile.ContentBindings)
	}
	// 序列化往返：绑定结构不丢。
	rj2, err := ruleToJSON(*entry)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if !reflect.DeepEqual(rj2.MetadataFile.ContentBindings, ok.MetadataFile.ContentBindings) {
		t.Fatalf("内容绑定序列化往返应保真: %+v", rj2.MetadataFile.ContentBindings)
	}
	// 一行多目标：同一行整段同时映射多个维度（多目标=同行多条目），载入放行。
	multi := contentAggregateEntryJSON([]ContentBinding{
		{Row: 2, Dimension: DimDescription},
		{Row: 2, Dimension: DimTag},
	})
	multiEntry, err := ruleFromJSON(multi)
	if err != nil {
		t.Fatalf("一行多目标绑定应载入: %v", err)
	}
	if len(multiEntry.MetadataFile.ContentBindings) != 2 {
		t.Fatalf("一行多目标应保留两条绑定: %+v", multiEntry.MetadataFile.ContentBindings)
	}
	// 旧档零值兼容：无绑定键的聚合规则照常载入，无「元数据文件」产出。
	legacy := contentAggregateEntryJSON(nil)
	legacy.MetadataFile = nil
	legacyEntry, err := ruleFromJSON(legacy)
	if err != nil {
		t.Fatalf("无绑定的聚合规则应照常载入: %v", err)
	}
	if legacyEntry.MetadataFile != nil {
		t.Fatalf("无绑定键载入应无元数据文件产出: %+v", legacyEntry.MetadataFile)
	}

	cases := []struct {
		name     string
		bindings []ContentBinding
		wantSub  string
	}{
		{"行号越界", []ContentBinding{{Row: 3, Dimension: DimDescription}}, "越界"},
		{"行号零值", []ContentBinding{{Row: 0, Dimension: DimDescription}}, "越界"},
		{"维度非法", []ContentBinding{{Row: 2, Dimension: MetadataDimension("bogus")}}, "维度非法"},
		{"维度空串", []ContentBinding{{Row: 2}}, "维度非法"},
		{"同行同维重复指派", []ContentBinding{
			{Row: 2, Dimension: DimDescription},
			{Row: 2, Dimension: DimDescription},
		}, "同一行同一维度唯一"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bad := contentAggregateEntryJSON(tc.bindings)
			_, err := ruleFromJSON(bad)
			if err == nil {
				t.Fatal("非法内容绑定应载入拒绝")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("错误文案应含 %q: %v", tc.wantSub, err)
			}
		})
	}

	// 元数据规则限定：提取动作携带内容取值载入即拒（内容来源只经「元数据
	// 文件」产出绑定面成立）。
	meta := &ruleEntryJSON{
		ID:          "meta-content",
		Enabled:     true,
		Conditions:  []CondxRow{contentFileRow("{作品}", ".txt")},
		Extractions: []ExtractionAction{{Dimension: DimDescription, From: FromContent}},
	}
	if _, err := ruleFromJSON(meta); err == nil || !strings.Contains(err.Error(), "不得声明内容来源") {
		t.Fatalf("元数据规则的内容取值动作应拒绝: err=%v", err)
	}
	// 元数据规则不得携带聚合声明（旧有校验兜底）。
	metaWithAgg := contentAggregateEntryJSON([]ContentBinding{{Row: 2, Dimension: DimDescription}})
	metaWithAgg.Category = RuleCategoryMetadata
	if _, err := ruleFromJSON(metaWithAgg); err == nil {
		t.Fatal("元数据规则携带聚合声明应拒绝")
	}
}

// TestContentSourceHitSetSupply 命中集供给：绑定行命中的文件入集，未命中与
// 无绑定规则零供给；与第二相取值共用同一求值基座。
func TestContentSourceHitSetSupply(t *testing.T) {
	rules := []RuleEntry{{
		ID:          "agg-content",
		Enabled:     true,
		Category:    RuleCategoryAggregate,
		Conditions: []CondxRow{
			contentFileRow("{作品}", ".jpg"),
			contentFileRow("{作品}", ".txt"),
		},
		Identity: []IdentityPart{{Row: 1, Capture: "work"}},
		Aggregate: &AggregateProduction{
			WorkType: sdkdto.ResourceTypeArticle,
			Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}, {Row: 2, Role: contract.StoreTypeDocument}},
		},
		MetadataFile: &MetadataFileProduction{
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}, {
		ID:        "meta-plain",
		Enabled:   true,
		Conditions: []CondxRow{contentFileRow("{作品}", ".nfo")},
	}}
	conclusions := []FileConclusion{
		{RelPath: "图库/cover.jpg", FileName: "cover.jpg", DirRelKey: "图库"},
		{RelPath: "图库/说明.txt", FileName: "说明.txt", DirRelKey: "图库"},
		{RelPath: "图库/别处.nfo", FileName: "别处.nfo", DirRelKey: "图库"},
	}
	hits := companionContentHitSet(rules, conclusions)
	if !reflect.DeepEqual(hits, []string{"图库/说明.txt"}) {
		t.Fatalf("命中集应恰含文本行命中的说明文件: %+v", hits)
	}
	// 无绑定规则（禁用/元数据/空绑定）零供给。
	noBindings := append([]RuleEntry{}, rules...)
	noBindings[0].MetadataFile = nil
	if hits := companionContentHitSet(noBindings, conclusions); len(hits) != 0 {
		t.Fatalf("无内容绑定时应零供给: %+v", hits)
	}
	disabled := append([]RuleEntry{}, rules...)
	disabled[0].Enabled = false
	if hits := companionContentHitSet(disabled, conclusions); len(hits) != 0 {
		t.Fatalf("禁用规则不应供给命中集: %+v", hits)
	}
}

// TestContentSourceDeclarationProduction 第二相整段取值产声明：绑定行命中的
// 文件内容整段声明到绑定维度（来源=内容、截断标注）；空内容/全空白零声明；
// 未命中行零产出；会话赋值按维度覆盖重放。
func TestContentSourceDeclarationProduction(t *testing.T) {
	aggRule := func(binding ContentBinding) RuleEntry {
		return RuleEntry{
			ID:          "agg-content",
			Enabled:     true,
			Category:    RuleCategoryAggregate,
			Conditions: []CondxRow{
				contentFileRow("{作品}", ".jpg"),
				contentFileRow("{作品}", ".txt"),
			},
			Identity: []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{
				WorkType: sdkdto.ResourceTypeArticle,
				Tracks:   []TrackAssign{{Row: 1, Role: contract.StoreTypeImage}, {Row: 2, Role: contract.StoreTypeDocument}},
			},
			MetadataFile: &MetadataFileProduction{
				ContentBindings: []ContentBinding{binding},
			},
		}
	}
	newFace := func() ([]FileConclusion, []ScanFile) {
		conclusions := []FileConclusion{
			{RelPath: "图库/cover.jpg", FileName: "cover.jpg", DirRelKey: "图库"},
			{RelPath: "图库/说明.txt", FileName: "说明.txt", DirRelKey: "图库"},
		}
		files := []ScanFile{
			{RelPath: "图库/cover.jpg", FileName: "cover.jpg", DirNames: []string{"图库"}},
			{RelPath: "图库/说明.txt", FileName: "说明.txt", DirNames: []string{"图库"}},
		}
		return conclusions, files
	}

	t.Run("整段取值产声明", func(t *testing.T) {
		conclusions, files := newFace()
		files[1].CompanionText = "作品说明内容\n整段保留"
		applyCompanionContentDeclarations(conclusions, files, []RuleEntry{aggRule(ContentBinding{Row: 2, Dimension: DimDescription})}, nil)
		if len(conclusions[0].Final) != 0 {
			t.Fatalf("无内容文件不应产出声明: %+v", conclusions[0].Final)
		}
		if len(conclusions[1].Final) != 1 {
			t.Fatalf("绑定行命中的说明文件应产一条声明: %+v", conclusions[1].Final)
		}
		decl := conclusions[1].Final[0]
		if decl.Dimension != DimDescription || decl.Value != "作品说明内容\n整段保留" ||
			decl.Source != SourceContent || decl.RuleID != "agg-content" || decl.Truncated {
			t.Fatalf("声明字段不符: %+v", decl)
		}
	})

	t.Run("截断标注", func(t *testing.T) {
		conclusions, files := newFace()
		files[1].CompanionText = "超上限片段"
		files[1].CompanionTruncated = true
		applyCompanionContentDeclarations(conclusions, files, []RuleEntry{aggRule(ContentBinding{Row: 2, Dimension: DimDescription})}, nil)
		if len(conclusions[1].Final) != 1 || !conclusions[1].Final[0].Truncated {
			t.Fatalf("截断事实应随声明标注: %+v", conclusions[1].Final)
		}
	})

	t.Run("空内容与全空白零声明", func(t *testing.T) {
		conclusions, files := newFace()
		files[1].CompanionText = " \n\t "
		applyCompanionContentDeclarations(conclusions, files, []RuleEntry{aggRule(ContentBinding{Row: 2, Dimension: DimDescription})}, nil)
		if len(conclusions[1].Final) != 0 || len(conclusions[1].Pending) != 0 {
			t.Fatalf("全空白内容应零声明: %+v", conclusions[1].Final)
		}
	})

	t.Run("绑定行未命中零产出", func(t *testing.T) {
		conclusions, files := newFace()
		files[0].CompanionText = "图片不是说明文件"
		applyCompanionContentDeclarations(conclusions, files, []RuleEntry{aggRule(ContentBinding{Row: 2, Dimension: DimDescription})}, nil)
		if len(conclusions[0].Final) != 0 {
			t.Fatalf("未命中绑定行的文件应零声明: %+v", conclusions[0].Final)
		}
	})

	t.Run("赋值覆盖重放", func(t *testing.T) {
		conclusions, files := newFace()
		files[1].CompanionText = "说明文件写的作者"
		assigns := map[string]SessionAssignment{
			"图库/说明.txt": {Author: "手工作者"},
		}
		applyCompanionContentDeclarations(conclusions, files, []RuleEntry{aggRule(ContentBinding{Row: 2, Dimension: DimAuthor})}, assigns)
		if len(conclusions[1].Final) != 1 {
			t.Fatalf("赋值覆盖后应恰一条作者声明: %+v", conclusions[1].Final)
		}
		decl := conclusions[1].Final[0]
		if decl.Source != SourceAssignment || decl.Value != "手工作者" {
			t.Fatalf("会话赋值应压过内容声明: %+v", decl)
		}
	})

	t.Run("与提取段声明合成", func(t *testing.T) {
		conclusions, files := newFace()
		files[1].CompanionText = "说明文件写的标签"
		conclusions[1].rawDecls = []MetadataDeclaration{
			{Dimension: DimTag, Value: "文件名标签", Source: SourceFilename, RuleID: "meta-x"},
		}
		applyCompanionContentDeclarations(conclusions, files, []RuleEntry{aggRule(ContentBinding{Row: 2, Dimension: DimTag})}, nil)
		// 标签维度缺省并集：两条声明都保留（按值去重）。
		if len(conclusions[1].Final) != 2 {
			t.Fatalf("标签并集应保留文件名与内容两条声明: %+v", conclusions[1].Final)
		}
		got := map[string]SourceKind{}
		for _, decl := range conclusions[1].Final {
			got[decl.Value] = decl.Source
		}
		if got["文件名标签"] != SourceFilename || got["说明文件写的标签"] != SourceContent {
			t.Fatalf("并集声明来源不符: %+v", got)
		}
	})
}
