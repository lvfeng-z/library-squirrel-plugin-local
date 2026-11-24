// 集序记录与作答端的单元测试：批内按序号数值序、跨批次乱序补导后全局序仍按
// 序号数值序（先导后段再导前段）、同作品重复导入合并不重复（首见条目保留）、
// 跨目录同名集按目录块首见拼接、作答序号偏移（集内序-1）、记录缺席空作答、
// 集名来源收口与归属形态守卫（提取元数据 collection 维度与含义链 workSet 项已
// 退役；产出四集名快照按条目供序、归属赋值（确认归集）照常供序、无集内序成员
// 不落记录）。
package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// orderTestFile 集序测试的单文件素材：集名声明（空=无集名声明）与集内序号
// （数值与原文）。
type orderTestFile struct {
	rel     string
	seq     int
	seqText string
	set     string
}

// buildOrderSnapshot 构造单目录单序列群的快照：文件行带归属赋值（确认归集）的
// 集维度声明与群组键（规则群成员形态——文件行与群卡双携带），群卡成员携带序号
// 数值与原文。集名来源取归属赋值通路（决策7 甲保留：它写赋值不是元数据提取），
// 使本组用例专注集序合并机制本身；产出四集名快照的供序见
// TestEmitSelectedWorksChildOrderFollowsGroupSeq/NaturalSeq 与
// TestCollectWorkSetOrder* 两组用例。
func buildOrderSnapshot(dir, group string, files []orderTestFile) *RecognitionSnapshot {
	snap := &RecognitionSnapshot{}
	groupMembers := make([]SnapGroupMember, 0, len(files))
	for _, f := range files {
		rel := dir + "/" + f.rel
		sf := SnapFile{
			RelPath: rel, FileName: f.rel, DirRelKey: dir,
			GroupKey: group, GroupSeq: f.seq,
		}
		if f.set != "" {
			sf.Final = append(sf.Final, SnapDecl{
				Dimension: string(DimCollection), Value: f.set, Source: string(SourceAssignment),
			})
		}
		snap.Files = append(snap.Files, sf)
		groupMembers = append(groupMembers, SnapGroupMember{RelPath: rel, Seq: f.seq, SeqText: f.seqText})
	}
	snap.Groups = append(snap.Groups, SnapGroup{DirRelKey: dir, Key: group, Members: groupMembers})
	return snap
}

// orderSeqEntryRule 构造一条以序列群键为归集依据的产出四规则（集序端到端用例的
// 条目集名来源）：单条序列行 `{集名}_{序号}`（图片成员）——分组绑定与集名来源
// 都取自这一行（分组绑定=序列群键保留名，须选自序列目标行；集名=该行的集名
// 捕获）。规则可经档案载入校验。
func orderSeqEntryRule(id string) RuleEntry {
	return RuleEntry{
		ID:          id,
		Description: "测试：集序条目集名来源",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{{
			Condition: &Condx{
				Target: CondxTargetSeq,
				Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: "{集名}_{序号}"},
				Attach: []CondxAttach{{Kind: CondxAttachSeqMemberType, Types: []string{"image"}}},
			},
		}},
		WorkSetAggregation: []WorkSetAggregationProduction{{
			Grouping: &GroupingBinding{
				Mode:     GroupingModeIdentity,
				Identity: []IdentityPart{{Row: 1, Capture: identityGroupKeyCapture}},
			},
			NameExtraction: &NameExtraction{Row: 1, Capture: condxVocab["集名"]},
		}},
	}
}

// orderRulesProfileKV 把规则序列化为档案 JSON 的 KV 桩（作品拉取器经上下文解析
// 流水线规则时载入；规则经生产载入校验管线）。
func orderRulesProfileKV(t *testing.T, rules ...RuleEntry) map[string]string {
	t.Helper()
	data, err := json.Marshal(&RuleProfile{SchemaVersion: CurrentSchemaVersion, Rules: rules})
	if err != nil {
		t.Fatalf("序列化集序测试档案失败: %v", err)
	}
	return map[string]string{RuleProfileKVKey: string(data)}
}

// buildOrderItems 从快照文件行构造批次子行素材：local 回退归属、作品身份键=
// 「hash-相对路径」的稳定假哈希（与子行身份键同源同值的测试投影）。
func buildOrderItems(snap *RecognitionSnapshot, metadata []PathMeaning) []batchFileItem {
	items := make([]batchFileItem, 0, len(snap.Files))
	for i := range snap.Files {
		f := &snap.Files[i]
		items = append(items, batchFileItem{
			af: attributedFile{entry: FileEntry{
				FullPath: "root/" + f.RelPath, RelPath: f.RelPath, Hash: "hash-" + f.RelPath,
			}},
			metadata: metadata,
		})
	}
	return items
}

// orderWorkIDOf 测试素材的期望作品身份键（local 回退=假哈希形态）。
func orderWorkIDOf(dir, rel string) string {
	return "hash-" + dir + "/" + rel
}

// mustQueryOrder 作答便捷封装：失败即终止用例。
func mustQueryOrder(t *testing.T, h *LocalImportWorkFetcher, setKey string) []*orderAnswerItem {
	t.Helper()
	entries, err := h.QueryWorkSetOrder(0, setKey)
	if err != nil {
		t.Fatalf("查询集序作答失败（键 %q）: %v", setKey, err)
	}
	out := make([]*orderAnswerItem, 0, len(entries))
	for _, e := range entries {
		out = append(out, &orderAnswerItem{siteWorkId: e.SiteWorkId, sortOrder: e.SortOrder})
	}
	return out
}

// orderAnswerItem 作答条目的断言投影。
type orderAnswerItem struct {
	siteWorkId string
	sortOrder  int64
}

// assertAnswerOrder 断言作答条目按给定身份键序列排列、序号=位次（集内序-1，
// 从 0 起）。
func assertAnswerOrder(t *testing.T, items []*orderAnswerItem, wantWorkIDs []string) {
	t.Helper()
	if len(items) != len(wantWorkIDs) {
		t.Fatalf("作答应含 %d 条，实得 %d 条: %v", len(wantWorkIDs), len(items), items)
	}
	for i, item := range items {
		if item.siteWorkId != wantWorkIDs[i] {
			t.Fatalf("作答第 %d 位应为 %s，实得 %s（全序: %v）", i+1, wantWorkIDs[i], item.siteWorkId, items)
		}
		if item.sortOrder != int64(i) {
			t.Fatalf("作答序号应为全序位次（集内序-1），第 %d 位成员序号实得 %d", i+1, item.sortOrder)
		}
	}
}

// TestWorkSetOrderWithinBatchNumericSeqOrder 批内全局序按序号数值升序：序号
// 10、2、1 乱序产出（文件名字典序 1<10<2 与数值序不同），作答仍按 1、2、10；
// 序号原文保留零填充形态、成员携带目录键与首见批次号。
func TestWorkSetOrderWithinBatchNumericSeqOrder(t *testing.T) {
	ctx := stubPluginCtx{kv: make(map[string]string)}
	h := &LocalImportWorkFetcher{ctx: ctx}
	snap := buildOrderSnapshot("目录", "集名", []orderTestFile{
		{"集名_10.jpg", 10, "010", "集名"},
		{"集名_2.jpg", 2, "2", "集名"},
		{"集名_1.jpg", 1, "1", "集名"},
	})
	h.recordWorkSetOrders(snap, buildOrderItems(snap, nil), 1000)

	assertAnswerOrder(t, mustQueryOrder(t, h, "workSet:集名"), []string{
		orderWorkIDOf("目录", "集名_1.jpg"),
		orderWorkIDOf("目录", "集名_2.jpg"),
		orderWorkIDOf("目录", "集名_10.jpg"),
	})

	// 记录载荷：存储键=集业务键原文，成员三件套（序号原文/目录键/首见批次号）
	raw, ok := ctx.kv["workSet:集名"]
	if !ok {
		t.Fatalf("集序记录应落键 %q，实际键集合: %v", "workSet:集名", ctx.kv)
	}
	var rec worksetOrderRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		t.Fatalf("集序记录应可解析: %v", err)
	}
	if rec.SchemaVersion != worksetOrderSchemaVersion || len(rec.Members) != 3 || len(rec.DirBlocks) != 1 {
		t.Fatalf("记录应为版本 %d 三成员单目录块，实际 %+v", worksetOrderSchemaVersion, rec)
	}
	if rec.DirBlocks[0] != "目录" {
		t.Fatalf("目录块应为成员所在目录键，实际 %v", rec.DirBlocks)
	}
	byText := map[string]worksetOrderMember{}
	for _, m := range rec.Members {
		byText[m.SiteWorkId] = m
	}
	member := byText[orderWorkIDOf("目录", "集名_10.jpg")]
	if member.Seq != 10 || member.SeqText != "010" || member.DirKey != "目录" || member.Batch != 1000 {
		t.Fatalf("序号 10 成员应携带数值/原文/目录键/首见批次号，实际 %+v", member)
	}
}

// TestWorkSetOrderCrossBatchReverseImportGlobalSeq 跨批次乱序补导：先导后段
// （序号 6-10）再导前段（序号 1-5），全局总序仍按序号数值升序（1..10 连续），
// 不受批次先后影响。
func TestWorkSetOrderCrossBatchReverseImportGlobalSeq(t *testing.T) {
	ctx := stubPluginCtx{kv: make(map[string]string)}
	h := &LocalImportWorkFetcher{ctx: ctx}

	lateFiles := make([]orderTestFile, 0, 5)
	for seq := 6; seq <= 10; seq++ {
		lateFiles = append(lateFiles, orderTestFile{
			rel: fmt.Sprintf("集名_%03d.jpg", seq), seq: seq, seqText: fmt.Sprintf("%03d", seq), set: "集名",
		})
	}
	lateSnap := buildOrderSnapshot("目录", "集名", lateFiles)
	h.recordWorkSetOrders(lateSnap, buildOrderItems(lateSnap, nil), 1000)

	earlyFiles := make([]orderTestFile, 0, 5)
	for seq := 1; seq <= 5; seq++ {
		earlyFiles = append(earlyFiles, orderTestFile{
			rel: fmt.Sprintf("集名_%03d.jpg", seq), seq: seq, seqText: fmt.Sprintf("%03d", seq), set: "集名",
		})
	}
	earlySnap := buildOrderSnapshot("目录", "集名", earlyFiles)
	h.recordWorkSetOrders(earlySnap, buildOrderItems(earlySnap, nil), 2000)

	want := make([]string, 0, 10)
	for seq := 1; seq <= 10; seq++ {
		want = append(want, orderWorkIDOf("目录", fmt.Sprintf("集名_%03d.jpg", seq)))
	}
	assertAnswerOrder(t, mustQueryOrder(t, h, "workSet:集名"), want)
}

// TestWorkSetOrderDuplicateImportMergesOnce 同作品重复导入合并不重复：同批
// 成员再次导入（批次号不同）不追加条目、首见批次号保留首写值；与重复成员同批
// 的新成员照常并入。
func TestWorkSetOrderDuplicateImportMergesOnce(t *testing.T) {
	ctx := stubPluginCtx{kv: make(map[string]string)}
	h := &LocalImportWorkFetcher{ctx: ctx}
	snap := buildOrderSnapshot("目录", "集名", []orderTestFile{
		{"集名_1.jpg", 1, "1", "集名"},
		{"集名_2.jpg", 2, "2", "集名"},
		{"集名_3.jpg", 3, "3", "集名"},
	})
	h.recordWorkSetOrders(snap, buildOrderItems(snap, nil), 1000)

	// 重复导入批次：旧成员 1、2 + 新成员 4
	reimport := buildOrderSnapshot("目录", "集名", []orderTestFile{
		{"集名_1.jpg", 1, "1", "集名"},
		{"集名_2.jpg", 2, "2", "集名"},
		{"集名_4.jpg", 4, "4", "集名"},
	})
	h.recordWorkSetOrders(reimport, buildOrderItems(reimport, nil), 2000)

	var rec worksetOrderRecord
	if err := json.Unmarshal([]byte(ctx.kv["workSet:集名"]), &rec); err != nil {
		t.Fatalf("集序记录应可解析: %v", err)
	}
	if len(rec.Members) != 4 {
		t.Fatalf("重复导入后应恰 4 成员（1/2/3/4 各一条），实际 %d: %+v", len(rec.Members), rec.Members)
	}
	for _, m := range rec.Members {
		wantBatch := int64(1000)
		if m.SiteWorkId == orderWorkIDOf("目录", "集名_4.jpg") {
			wantBatch = 2000
		}
		if m.Batch != wantBatch {
			t.Fatalf("成员 %s 首见批次号应为 %d，实际 %d", m.SiteWorkId, wantBatch, m.Batch)
		}
	}
	assertAnswerOrder(t, mustQueryOrder(t, h, "workSet:集名"), []string{
		orderWorkIDOf("目录", "集名_1.jpg"),
		orderWorkIDOf("目录", "集名_2.jpg"),
		orderWorkIDOf("目录", "集名_3.jpg"),
		orderWorkIDOf("目录", "集名_4.jpg"),
	})
}

// TestWorkSetOrderCrossDirBlocksFirstSeenOrder 跨目录同名集按目录块首见拼接：
// 两目录序号各自起算，先导入的目录块整体在前（不做跨块序号数值交错），块内
// 各自按数值升序。
func TestWorkSetOrderCrossDirBlocksFirstSeenOrder(t *testing.T) {
	ctx := stubPluginCtx{kv: make(map[string]string)}
	h := &LocalImportWorkFetcher{ctx: ctx}

	lateDir := buildOrderSnapshot("后导目录", "同名集", []orderTestFile{
		{"成员_1.jpg", 1, "1", "同名集"},
		{"成员_2.jpg", 2, "2", "同名集"},
	})
	h.recordWorkSetOrders(lateDir, buildOrderItems(lateDir, nil), 1000)

	earlyDir := buildOrderSnapshot("先名字典序目录", "同名集", []orderTestFile{
		{"成员_1.jpg", 1, "1", "同名集"},
		{"成员_2.jpg", 2, "2", "同名集"},
	})
	h.recordWorkSetOrders(earlyDir, buildOrderItems(earlyDir, nil), 2000)

	// 目录块按首见序拼接：后导目录块在前（首见），块内 1、2；先名字典序目录块
	//接续在后——目录名字典序不参与排序
	assertAnswerOrder(t, mustQueryOrder(t, h, "workSet:同名集"), []string{
		orderWorkIDOf("后导目录", "成员_1.jpg"),
		orderWorkIDOf("后导目录", "成员_2.jpg"),
		orderWorkIDOf("先名字典序目录", "成员_1.jpg"),
		orderWorkIDOf("先名字典序目录", "成员_2.jpg"),
	})
}

// TestQueryWorkSetOrderAbsentRecordEmptyAnswer 记录缺席空作答：未知集业务键
// 与空业务键均返回空切片零错误——宿主侧保持该集原站序现状。
func TestQueryWorkSetOrderAbsentRecordEmptyAnswer(t *testing.T) {
	h := &LocalImportWorkFetcher{ctx: stubPluginCtx{kv: make(map[string]string)}}
	for _, setKey := range []string{"workSet:无记录", ""} {
		entries, err := h.QueryWorkSetOrder(1, setKey)
		if err != nil {
			t.Fatalf("记录缺席应零错误（键 %q）: %v", setKey, err)
		}
		if len(entries) != 0 {
			t.Fatalf("记录缺席应空作答（键 %q），实得 %v", setKey, entries)
		}
	}
}

// TestCollectWorkSetOrderUpdatesSourcesAndGuards 集名来源收口与归属形态守卫（决策
// 7 甲）：①提取元数据 collection 维度的终选集声明已退役——快照里有也不作集名来
// 源；②目录含义链 workSet 项已退役——不再作集名来源（local 归属同样不落）；③产
// 出四集名快照（条目 NameExtraction 的捕获值）是唯一元数据出口：序列群键条目的
// 成员按群卡序号供序（成员未复现集维度声明亦可）；④归属赋值（确认归集）集维度
// 声明照常供序——它写的是赋值，不是元数据提取；⑤无群卡身份（无集内序）的成员
// 不落记录。
func TestCollectWorkSetOrderUpdatesSourcesAndGuards(t *testing.T) {
	rules := []RuleEntry{{ID: "ws-seq", WorkSetAggregation: []WorkSetAggregationProduction{{
		Grouping:       &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{{Capture: identityGroupKeyCapture}}},
		NameExtraction: &NameExtraction{Row: 1, Capture: "setname"},
	}}}}
	snap := &RecognitionSnapshot{}
	// ① 遗留 collection 提取声明（来源=文件名）+ 群卡序号：退役后不作集名来源。
	retiredRel := "混合/旧集_01.jpg"
	snap.Files = append(snap.Files, SnapFile{
		RelPath: retiredRel, FileName: "旧集_01.jpg", DirRelKey: "混合",
		Final: []SnapDecl{{Dimension: string(DimCollection), Value: "旧集", Source: string(SourceFilename)}},
	})
	// ④ 归属赋值（确认归集）集维度声明：照常供序。
	assignedRel := "混合/赋值_02.jpg"
	snap.Files = append(snap.Files, SnapFile{
		RelPath: assignedRel, FileName: "赋值_02.jpg", DirRelKey: "混合",
		Final: []SnapDecl{{Dimension: string(DimCollection), Value: "确认集", Source: string(SourceAssignment)}},
	})
	// ③ 产出四序列群键条目成员：集名快照在场（快照无集维度声明）、群卡序号 3。
	seqRel := "混合/序列_03.jpg"
	snap.Files = append(snap.Files, SnapFile{
		RelPath: seqRel, FileName: "序列_03.jpg", DirRelKey: "混合",
		SetGroupKey: "序列", SetName: "集甲", SetRuleID: "ws-seq",
	})
	// ⑤ 落单成员：集名快照在场但不在任何群卡、文件行无群组键（无集内序）。
	loneRel := "混合/落单.jpg"
	snap.Files = append(snap.Files, SnapFile{
		RelPath: loneRel, FileName: "落单.jpg", DirRelKey: "混合",
		SetGroupKey: "序列", SetName: "集甲", SetRuleID: "ws-seq",
	})
	snap.Groups = append(snap.Groups, SnapGroup{DirRelKey: "混合", Key: "序列", Members: []SnapGroupMember{
		{RelPath: retiredRel, Seq: 1, SeqText: "1"},
		{RelPath: assignedRel, Seq: 2, SeqText: "02"},
		{RelPath: seqRel, Seq: 3, SeqText: "03"},
	}})

	// 目录含义链 workSet 项：local 归属子行同样不再作集名来源（已退役）。
	metadata := []PathMeaning{{Type: string(PathTypeWorkSet), Name: "集乙"}}
	items := []batchFileItem{
		{af: attributedFile{
			entry: FileEntry{FullPath: "root/" + retiredRel, RelPath: retiredRel, Hash: "hash-retired"},
			attr:  workAttribution{SiteKey: identity.Local.Key},
		}, metadata: metadata},
		{af: attributedFile{
			entry: FileEntry{FullPath: "root/" + assignedRel, RelPath: assignedRel, Hash: "hash-assigned"},
			attr:  workAttribution{SiteKey: identity.Local.Key},
		}, metadata: metadata},
		{af: attributedFile{
			entry: FileEntry{FullPath: "root/" + seqRel, RelPath: seqRel, Hash: "hash-seq"},
			attr:  workAttribution{SiteKey: identity.Local.Key},
		}, metadata: metadata},
		{af: attributedFile{
			entry: FileEntry{FullPath: "root/" + loneRel, RelPath: loneRel, Hash: "hash-lone"},
			attr:  workAttribution{SiteKey: identity.Local.Key},
		}, metadata: metadata},
	}

	updates := collectWorkSetOrderUpdates(snap, items, rules)
	if len(updates) != 2 {
		t.Fatalf("应恰两个集键（确认集=归属赋值、集甲=产出四集名快照），实际 %v", updates)
	}
	if got := updates[workSetBizKey("集甲")]; len(got) != 1 || got[0].SiteWorkId != "hash-seq" || got[0].Seq != 3 {
		t.Fatalf("产出四集名快照应供序（落单无序号不落），实际 %+v", got)
	}
	if got := updates[workSetBizKey("确认集")]; len(got) != 1 || got[0].SiteWorkId != "hash-assigned" || got[0].Seq != 2 {
		t.Fatalf("归属赋值（确认归集）集应照常供序，实际 %+v", got)
	}
	for _, name := range []string{"旧集", "集乙"} {
		if got, ok := updates[workSetBizKey(name)]; ok {
			t.Fatalf("退役集名通路（%s）不应供序，实际 %+v", name, got)
		}
	}
}
