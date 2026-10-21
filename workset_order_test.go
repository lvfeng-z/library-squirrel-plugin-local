// 集序记录与作答端的单元测试：批内按序号数值序、跨批次乱序补导后全局序仍按
// 序号数值序（先导后段再导前段）、同作品重复导入合并不重复（首见条目保留）、
// 跨目录同名集按目录块首见拼接、作答序号偏移（集内序-1）、记录缺席空作答、
// 集归属两来源与归属形态守卫（真实域作品身份键=页级作品 ID、目录含义链集名仅
// local 归属生效、无集内序成员不落记录）。
package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// orderTestFile 集序测试的单文件素材：集名声明（空=无集维度终选声明）与集内
// 序号（数值与原文）。
type orderTestFile struct {
	rel     string
	seq     int
	seqText string
	set     string
}

// buildOrderSnapshot 构造单目录单序列群的快照：文件行带集维度终选声明与群组键
// （规则群成员形态——文件行与群卡双携带），群卡成员携带序号数值与原文。
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
			sf.Final = append(sf.Final, SnapDecl{Dimension: string(DimCollection), Value: f.set})
		}
		snap.Files = append(snap.Files, sf)
		groupMembers = append(groupMembers, SnapGroupMember{RelPath: rel, Seq: f.seq, SeqText: f.seqText})
	}
	snap.Groups = append(snap.Groups, SnapGroup{DirRelKey: dir, Key: group, Members: groupMembers})
	return snap
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

// TestCollectWorkSetOrderUpdatesSourcesAndGuards 集归属两来源与归属形态守卫：
// ①快照终选集维度声明对真实域归属成员照常记录，身份键=页级作品 ID（与子行
// 同值）；②目录含义链 workSet 项仅对 local 归属子行生效（真实域跳过——与执行
// 期作品应答的跳过口径一致）；③不在任何群卡且文件行无群组键的成员（无集内
// 序）不落记录；④已识别成员经群卡反查取得序号（文件行无群组键亦可记录）。
func TestCollectWorkSetOrderUpdatesSourcesAndGuards(t *testing.T) {
	snap := &RecognitionSnapshot{}
	// 真实域归属成员：集维度终选声明 + 群卡序号；文件行无群组键（已识别形态）
	realRel := "混合/11111111_p0.jpg"
	snap.Files = append(snap.Files, SnapFile{
		RelPath: realRel, FileName: "11111111_p0.jpg", DirRelKey: "混合",
		Final: []SnapDecl{{Dimension: string(DimCollection), Value: "集甲"}},
	})
	// local 归属成员：无集维度声明，靠目录含义链 workSet 项归集；群卡携带序号
	localRel := "混合/普通_02.jpg"
	snap.Files = append(snap.Files, SnapFile{
		RelPath: localRel, FileName: "普通_02.jpg", DirRelKey: "混合",
	})
	// 落单成员：目录含义链指向同名集但不在任何群、文件行无群组键（无集内序）
	loneRel := "混合/落单.jpg"
	snap.Files = append(snap.Files, SnapFile{
		RelPath: loneRel, FileName: "落单.jpg", DirRelKey: "混合",
	})
	snap.Groups = append(snap.Groups, SnapGroup{DirRelKey: "混合", Key: "序列", Members: []SnapGroupMember{
		{RelPath: realRel, Seq: 1, SeqText: "1"},
		{RelPath: localRel, Seq: 2, SeqText: "02"},
	}})

	// 目录含义链：集名「集乙」（对真实域成员无效、对 local 成员有效）
	metadata := []PathMeaning{{Type: string(PathTypeWorkSet), Name: "集乙"}}
	items := []batchFileItem{
		{af: attributedFile{
			entry: FileEntry{FullPath: "root/" + realRel, RelPath: realRel, Hash: "hash-real"},
			attr:  workAttribution{SiteKey: identity.Pixiv.Key, PageWorkID: "11111111_p0"},
		}, metadata: metadata},
		{af: attributedFile{
			entry: FileEntry{FullPath: "root/" + localRel, RelPath: localRel, Hash: "hash-local"},
			attr:  workAttribution{SiteKey: identity.Local.Key},
		}, metadata: metadata},
		{af: attributedFile{
			entry: FileEntry{FullPath: "root/" + loneRel, RelPath: loneRel, Hash: "hash-lone"},
			attr:  workAttribution{SiteKey: identity.Local.Key},
		}, metadata: metadata},
	}

	updates := collectWorkSetOrderUpdates(snap, items)
	if len(updates) != 2 {
		t.Fatalf("应恰两个集键（集甲=真实域声明、集乙=含义链 local 归属），实际 %v", updates)
	}
	if got := updates["workSet:集甲"]; len(got) != 1 || got[0].SiteWorkId != "11111111_p0" || got[0].Seq != 1 || got[0].DirKey != "混合" {
		t.Fatalf("真实域成员应按页级作品 ID 记录进声明集，实际 %+v", got)
	}
	if got := updates["workSet:集乙"]; len(got) != 1 || got[0].SiteWorkId != "hash-local" || got[0].Seq != 2 || got[0].SeqText != "02" {
		t.Fatalf("local 成员应经含义链归集（真实域跳过、落单无序号不落），实际 %+v", got)
	}
}
