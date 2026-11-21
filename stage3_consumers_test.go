// 作品集聚合声明式收编·阶段3 消费面锚：产出四集归属事实的选择域集键、创建链
// 落集第三源、集序供序形态裁定、疑似作品集页目录卡归集成员与「隐藏已识别」
// 数据位口径。夹具为合成识别快照（消费面输入的稳定形态）与真实识别链路两式：
// 既有序列群/落单/聚合行为在产出四缺席时零漂移，产出四在场时集可整选、集名
// 按名落集、非序列群集不供集内序。
package main

import (
	"encoding/json"
	"strings"
	"testing"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
)

// —— 消费面1：选择域集键扩展 —— //

// TestDeriveWorkDomainSetAttribution 选择域集键扩展：产出四集归属在场时按
// 「组键+集名」识别一个集——成员（含文件行无群组键的已识别结构群成员）整体
// 入集、整集可整选；集内序有群卡身份取群卡序（快照序不同也按序号）、无群卡
// 身份按快照序稳定排列；集键为归属键（不与既有序列群集重复归集）；所在目录
// 集（组键=目录路径）与根目录集（组键空）同样成集；无集归属的既有序列群集
// 与落单文件行为零漂移。
func TestDeriveWorkDomainSetAttribution(t *testing.T) {
	snap := &RecognitionSnapshot{
		Files: []SnapFile{
			// 序列群集：快照序 B、A、C 打乱，群卡序号 A=1/B=2/C=3 决定集内序；
			// A 为规则群成员（文件行群组键），B/C 已识别且文件行无群组键（群归属
			// 只在群卡）——产出四集须把三者整体纳入。
			{RelPath: "集/路灯_02.jpg", FileName: "路灯_02.jpg", DirRelKey: "集", State: snapStateIdentified,
				SetGroupKey: "路灯", SetName: "路灯", SetRuleID: "ws-seq"},
			{RelPath: "集/路灯_01.jpg", FileName: "路灯_01.jpg", DirRelKey: "集", State: snapStateIdentified,
				GroupKey: "路灯", GroupSeq: 1, SetGroupKey: "路灯", SetName: "路灯", SetRuleID: "ws-seq"},
			{RelPath: "集/路灯_03.jpg", FileName: "路灯_03.jpg", DirRelKey: "集", State: snapStateIdentified,
				SetGroupKey: "路灯", SetName: "路灯", SetRuleID: "ws-seq"},
			// 所在目录集：无群卡身份，成员按快照序稳定排列；集名来自目录名捕获。
			{RelPath: "目录集/d1.jpg", FileName: "d1.jpg", DirRelKey: "目录集", State: snapStateIdentified,
				SetGroupKey: "目录集", SetName: "目录名集", SetRuleID: "ws-dir"},
			{RelPath: "目录集/d2.jpg", FileName: "d2.jpg", DirRelKey: "目录集", State: snapStateIdentified,
				SetGroupKey: "目录集", SetName: "目录名集", SetRuleID: "ws-dir"},
			// 根目录集：目录聚合键为空串，集仍成（集名非空即归属在场）。
			{RelPath: "r1.jpg", FileName: "r1.jpg", DirRelKey: "", State: snapStateIdentified,
				SetName: "根集", SetRuleID: "ws-dir"},
			{RelPath: "r2.jpg", FileName: "r2.jpg", DirRelKey: "", State: snapStateIdentified,
				SetName: "根集", SetRuleID: "ws-dir"},
			// 既有序列群集（无产出四）：行为零漂移。
			{RelPath: "旧集/x_01.jpg", FileName: "x_01.jpg", DirRelKey: "旧集", State: snapStateIdentified,
				GroupKey: "旧群", GroupSeq: 1},
			{RelPath: "旧集/x_02.jpg", FileName: "x_02.jpg", DirRelKey: "旧集", State: snapStateIdentified,
				GroupKey: "旧群", GroupSeq: 2},
			// 落单文件：不属任何集。
			{RelPath: "落单/z.jpg", FileName: "z.jpg", DirRelKey: "落单", State: snapStateIdentified},
		},
		Groups: []SnapGroup{
			{DirRelKey: "集", Key: "路灯", Size: 3, RuleID: "sel-sibling-set", Members: []SnapGroupMember{
				{RelPath: "集/路灯_01.jpg", Seq: 1, SeqText: "01"},
				{RelPath: "集/路灯_02.jpg", Seq: 2, SeqText: "02"},
				{RelPath: "集/路灯_03.jpg", Seq: 3, SeqText: "03"},
			}},
		},
	}
	domain := DeriveWorkDomain(snap, nil)

	// 序列群集：键=组键+集名（归属集键），成员按群卡序 A/B/C。
	seqSetKey := workSetKeyOfAttribution("路灯", "路灯")
	seqSet := setViewOf(domain, seqSetKey)
	if seqSet == nil {
		t.Fatalf("产出四序列群集缺席（sets: %+v）", domain.Sets)
	}
	if seqSet.Key != "路灯" || seqSet.DirRelKey != "集" {
		t.Fatalf("产出四集呈现键/目录键不符: %+v", seqSet)
	}
	wantMemberIDs := []string{"f:集/路灯_01.jpg", "f:集/路灯_02.jpg", "f:集/路灯_03.jpg"}
	if strings.Join(seqSet.Members, ",") != strings.Join(wantMemberIDs, ",") {
		t.Fatalf("产出四集成员应按群卡序号排列: got=%v want=%v", seqSet.Members, wantMemberIDs)
	}
	for _, id := range wantMemberIDs {
		work := domain.Work(id)
		if work == nil || work.SetKey != seqSetKey {
			t.Fatalf("成员 %s 应属产出四集 %q（实际 %+v）", id, seqSetKey, work)
		}
	}
	// 归属集优先于既有群组键：不得再以「目录+群组键」重复归集
	if legacy := setViewOf(domain, workSetKey("集", "路灯")); legacy != nil {
		t.Fatalf("产出四集成员不应重复进入既有序列群集: %+v", legacy)
	}

	// 所在目录集：成员无群卡身份，按快照序稳定排列。
	dirSet := setViewOf(domain, workSetKeyOfAttribution("目录集", "目录名集"))
	if dirSet == nil || strings.Join(dirSet.Members, ",") != "f:目录集/d1.jpg,f:目录集/d2.jpg" {
		t.Fatalf("所在目录集成员应=快照序两成员: %+v", dirSet)
	}
	if dirSet.Key != "目录名集" {
		t.Fatalf("所在目录集呈现键应为集名: %+v", dirSet)
	}

	// 根目录集：组键空串仍成集（集名非空=归属在场）。
	rootSet := setViewOf(domain, workSetKeyOfAttribution("", "根集"))
	if rootSet == nil || len(rootSet.Members) != 2 || rootSet.Key != "根集" {
		t.Fatalf("根目录集应按集名成集: %+v", rootSet)
	}

	// 既有序列群集与落单文件零漂移。
	legacy := setViewOf(domain, workSetKey("旧集", "旧群"))
	if legacy == nil || strings.Join(legacy.Members, ",") != "f:旧集/x_01.jpg,f:旧集/x_02.jpg" {
		t.Fatalf("无产出四的既有序列群集行为应零漂移: %+v", legacy)
	}
	if lone := domain.Work("f:落单/z.jpg"); lone == nil || lone.SetKey != "" {
		t.Fatalf("无集归属的落单文件不应属集: %+v", lone)
	}
}

// setViewOf 按集键取集合视图（未命中 nil）。
func setViewOf(domain *WorkDomain, setKey string) *WorkSetView {
	for _, s := range domain.Sets {
		if s.SetKey == setKey {
			return s
		}
	}
	return nil
}

// —— 消费面2：创建链落集第三源 —— //

// TestCreateWorkInfoSetAttributionSource 创建链落集第三源：任务载荷携带产出四
// 集名快照（setName）时按名声明作品集（宿主按业务键 upsert 的同名同集通路），
// 与既有来源同名只声明一次；成员未在重算结论中复现集维度声明也能落集；真实域
// 归属成员同权（按名声明落作品所属站点域，与终选声明来源一致）。
func TestCreateWorkInfoSetAttributionSource(t *testing.T) {
	h := &LocalImportWorkFetcher{}
	// ① 无任何终选/含义链来源：第三源按名声明
	pd := `{"schemaVersion":5,"fullPath":"C:/lib/photo.png","relPath":"photo.png","hash":"setname1","size":10,` +
		`"setName":"集丙"}`
	resp, err := h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &pd})
	if err != nil {
		t.Fatalf("CreateWorkInfo 不应报错: %v", err)
	}
	if len(resp.WorkSets) != 1 || resp.WorkSets[0].SiteWorkSetId != workSetBizKey("集丙") ||
		resp.WorkSets[0].WorkSetName != "集丙" {
		t.Fatalf("第三源应按集名快照声明作品集，实得 %+v", resp.WorkSets)
	}

	// ② 与含义链来源同名：只声明一次（同名同集幂等）
	pd = `{"schemaVersion":5,"fullPath":"C:/lib/photo.png","relPath":"photo.png","hash":"setname2","size":10,` +
		`"setName":"合集","metadata":[{"type":"workSet","name":"合集"}]}`
	resp, err = h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &pd})
	if err != nil {
		t.Fatalf("CreateWorkInfo 不应报错: %v", err)
	}
	if len(resp.WorkSets) != 1 || resp.WorkSets[0].WorkSetName != "合集" {
		t.Fatalf("第三源与既有来源同名应只声明一次，实得 %+v", resp.WorkSets)
	}

	// ③ 真实域归属：含义链来源跳过（站点侧集 ID 不可派生），第三源按名声明
	//    不受该跳过约束（与终选声明来源同形态，落作品所属站点域）。
	pd = `{"schemaVersion":5,"fullPath":"C:/lib/12345678-abcdef_p0.jpg","relPath":"12345678-abcdef_p0.jpg",` +
		`"hash":"setname3","size":10,"siteKey":"pixiv","siteWorkId":"12345678-abcdef_p0","setName":"集丁",` +
		`"metadata":[{"type":"workSet","name":"含义链集"}]}`
	resp, err = h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &pd})
	if err != nil {
		t.Fatalf("CreateWorkInfo 不应报错: %v", err)
	}
	if len(resp.WorkSets) != 1 || resp.WorkSets[0].WorkSetName != "集丁" {
		t.Fatalf("真实域成员第三源应按名声明（含义链来源仍跳过），实得 %+v", resp.WorkSets)
	}

	// ④ 无集名快照：既有两源行为零漂移（含义链 local 作品集合照落）
	pd = `{"schemaVersion":5,"fullPath":"C:/lib/photo.png","relPath":"photo.png","hash":"setname4","size":10,` +
		`"metadata":[{"type":"workSet","name":"既有集"}]}`
	resp, err = h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &pd})
	if err != nil {
		t.Fatalf("CreateWorkInfo 不应报错: %v", err)
	}
	if len(resp.WorkSets) != 1 || resp.WorkSets[0].WorkSetName != "既有集" {
		t.Fatalf("无集名快照时既有集名来源应零漂移，实得 %+v", resp.WorkSets)
	}
}

// TestEmitSelectedWorksCarriesSetName 创建子行载荷固化集名快照：工作台创建时
// 识别快照的文件行集名（产出四集归属）随子行 PluginData 固化（任务自包含，
// 执行期重算无集维度声明时据此按名挂集）；聚合子行取主导文件集名。无归属
// 文件不携带（omitempty，既有载荷零漂移）。
func TestEmitSelectedWorksCarriesSetName(t *testing.T) {
	root, h, _, sess := emitFixture(t, []string{"集/路灯_01.jpg", "集/路灯_02.jpg", "落单/z.jpg"})
	sess.Snapshot = &RecognitionSnapshot{Files: []SnapFile{
		{RelPath: "集/路灯_01.jpg", FileName: "路灯_01.jpg", DirRelKey: "集", State: snapStateIdentified,
			SetGroupKey: "路灯", SetName: "路灯", SetRuleID: "ws-seq"},
		{RelPath: "集/路灯_02.jpg", FileName: "路灯_02.jpg", DirRelKey: "集", State: snapStateIdentified,
			SetGroupKey: "路灯", SetName: "路灯", SetRuleID: "ws-seq"},
		{RelPath: "落单/z.jpg", FileName: "z.jpg", DirRelKey: "落单", State: snapStateIdentified},
	}}
	works := []*SelectableWork{
		{WorkID: "f:集/路灯_01.jpg", DirRelKey: "集", Files: []string{"集/路灯_01.jpg"},
			SetKey: workSetKeyOfAttribution("路灯", "路灯"), Selectable: true},
		{WorkID: "f:集/路灯_02.jpg", DirRelKey: "集", Files: []string{"集/路灯_02.jpg"},
			SetKey: workSetKeyOfAttribution("路灯", "路灯"), Selectable: true},
		{WorkID: "f:落单/z.jpg", DirRelKey: "落单", Files: []string{"落单/z.jpg"}, Selectable: true},
	}
	items := collectEmitted(t, h, root, sess, works, "集名快照")
	tasks := emittedTasks(items)
	if len(tasks) != 1 {
		t.Fatalf("应单批父行，实得 %d", len(tasks))
	}
	if len(tasks[0].Children) != 3 {
		t.Fatalf("子行数应为 3，实得 %d", len(tasks[0].Children))
	}
	byName := make(map[string]FilePluginData, len(tasks[0].Children))
	for _, child := range tasks[0].Children {
		var fp FilePluginData
		if err := json.Unmarshal([]byte(child.PluginData), &fp); err != nil {
			t.Fatalf("解析子行 PluginData 失败: %v", err)
		}
		if fp.SchemaVersion != currentPluginDataVersion {
			t.Fatalf("子行 PluginData 版本应=当前支持版本 %d，实得 %d", currentPluginDataVersion, fp.SchemaVersion)
		}
		byName[child.TaskName] = fp
	}
	if fp := byName["路灯_01.jpg"]; fp.SetName != "路灯" {
		t.Fatalf("集成员子行应固化集名快照，实得 %+v", fp)
	}
	if fp := byName["路灯_02.jpg"]; fp.SetName != "路灯" {
		t.Fatalf("集成员子行应固化集名快照，实得 %+v", fp)
	}
	if fp := byName["z.jpg"]; fp.SetName != "" {
		t.Fatalf("无集归属文件不应携带集名快照，实得 %+v", fp)
	}
}

// TestBuildAggregatedChildResponseCarriesSetName 聚合作品子行（一件一子行多轨）
// 载荷同样固化主导文件的集名快照——产出四并聚合产出时集归属随子行落集。
func TestBuildAggregatedChildResponseCarriesSetName(t *testing.T) {
	dir := t.TempDir()
	lead := dir + "/video.mp4"
	sub := dir + "/video.srt"
	writeTestFile(t, lead, "v")
	writeTestFile(t, sub, "s")
	child := buildAggregatedChildResponse(
		[]aggTrackMember{
			{entry: FileEntry{FullPath: lead, RelPath: "作品集/video.mp4", Hash: "h1"}, role: "videoMain"},
			{entry: FileEntry{FullPath: sub, RelPath: "作品集/video.srt", Hash: "h2"}, role: "subtitle"},
		},
		workAttribution{SiteKey: identity.Local.Key}, nil, nil, "video", nil, "聚合集")
	if child == nil {
		t.Fatal("聚合作品子行构造不应失败")
	}
	var fp FilePluginData
	if err := json.Unmarshal([]byte(child.PluginData), &fp); err != nil {
		t.Fatalf("解析聚合作品子行 PluginData 失败: %v", err)
	}
	if fp.SetName != "聚合集" || len(fp.Tracks) != 2 || fp.SchemaVersion != currentPluginDataVersion {
		t.Fatalf("聚合作品子行应固化集名快照与轨道清单，实得 %+v", fp)
	}
}

// —— 消费面3：集序供序形态裁定 —— //

// TestCollectWorkSetOrderUpdatesSetAttribution 产出四集序供序裁定：序列群键
// 归集依据的集照常供集内序（成员可能未在本文件复现集维度声明——按集名快照
// 补位记录）；行捕获复合/所在目录集的集名不供集内序（宿主创建序兜底，即便
// 成员带群卡身份与终选集维度声明）；无群卡身份的序列群集成员不落记录；无
// 产出四集归属时既有两源行为零漂移。
func TestCollectWorkSetOrderUpdatesSetAttribution(t *testing.T) {
	rules := []RuleEntry{
		{ID: "ws-seq", WorkSetAggregation: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeIdentity, Identity: []IdentityPart{{Capture: identityGroupKeyCapture}},
		}}},
		{ID: "ws-cap", WorkSetAggregation: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeIdentity, Identity: []IdentityPart{{Capture: "甲"}, {Capture: "乙"}},
		}}},
		{ID: "ws-dir", WorkSetAggregation: &WorkSetAggregationProduction{Grouping: &GroupingBinding{
			Mode: GroupingModeDirectory,
		}}},
	}
	snap := &RecognitionSnapshot{}
	addFile := func(f SnapFile) { snap.Files = append(snap.Files, f) }
	// 序列群集成员：无终选集维度声明，靠集名快照补位；群卡序号 2。
	addFile(SnapFile{RelPath: "混合/序列_02.jpg", DirRelKey: "混合",
		SetGroupKey: "序列", SetName: "集甲", SetRuleID: "ws-seq"})
	// 目录集成员：有群卡身份与终选集维度声明，但归集依据=所在目录 → 不供序。
	addFile(SnapFile{RelPath: "混合/目录_01.jpg", DirRelKey: "混合",
		SetGroupKey: "混合", SetName: "集乙", SetRuleID: "ws-dir",
		Final: []SnapDecl{{Dimension: string(DimCollection), Value: "集乙"}}})
	// 行捕获复合集成员：有群卡身份与终选集维度声明 → 不供序。
	addFile(SnapFile{RelPath: "混合/捕获_03.jpg", DirRelKey: "混合",
		SetGroupKey: "甲\x00乙", SetName: "集丙", SetRuleID: "ws-cap",
		Final: []SnapDecl{{Dimension: string(DimCollection), Value: "集丙"}}})
	// 既有集成员（无产出四集归属）：终选声明照常供序。
	addFile(SnapFile{RelPath: "混合/既有_04.jpg", DirRelKey: "混合",
		Final: []SnapDecl{{Dimension: string(DimCollection), Value: "集丁"}}})
	// 序列群集但无群卡身份：无集内序不落记录。
	addFile(SnapFile{RelPath: "混合/无群_05.jpg", DirRelKey: "混合",
		SetGroupKey: "序列", SetName: "集甲", SetRuleID: "ws-seq"})
	snap.Groups = append(snap.Groups, SnapGroup{DirRelKey: "混合", Key: "序列", Size: 2, RuleID: "ws-seq",
		Members: []SnapGroupMember{
			{RelPath: "混合/序列_02.jpg", Seq: 2, SeqText: "02"},
		}})
	// 非序列群集成员的群卡身份（各自成组，证明供序裁定只看归集依据形态）。
	snap.Groups = append(snap.Groups,
		SnapGroup{DirRelKey: "混合", Key: "目录群", Members: []SnapGroupMember{
			{RelPath: "混合/目录_01.jpg", Seq: 1, SeqText: "01"},
		}},
		SnapGroup{DirRelKey: "混合", Key: "捕获群", Members: []SnapGroupMember{
			{RelPath: "混合/捕获_03.jpg", Seq: 3, SeqText: "03"},
		}},
		SnapGroup{DirRelKey: "混合", Key: "既有群", Members: []SnapGroupMember{
			{RelPath: "混合/既有_04.jpg", Seq: 4, SeqText: "04"},
		}},
	)

	items := []batchFileItem{
		{af: attributedFile{entry: FileEntry{FullPath: "root/混合/序列_02.jpg", RelPath: "混合/序列_02.jpg", Hash: "h-seq"}, attr: workAttribution{SiteKey: identity.Local.Key}}},
		{af: attributedFile{entry: FileEntry{FullPath: "root/混合/目录_01.jpg", RelPath: "混合/目录_01.jpg", Hash: "h-dir"}, attr: workAttribution{SiteKey: identity.Local.Key}}},
		{af: attributedFile{entry: FileEntry{FullPath: "root/混合/捕获_03.jpg", RelPath: "混合/捕获_03.jpg", Hash: "h-cap"}, attr: workAttribution{SiteKey: identity.Local.Key}}},
		{af: attributedFile{entry: FileEntry{FullPath: "root/混合/既有_04.jpg", RelPath: "混合/既有_04.jpg", Hash: "h-legacy"}, attr: workAttribution{SiteKey: identity.Local.Key}}},
		{af: attributedFile{entry: FileEntry{FullPath: "root/混合/无群_05.jpg", RelPath: "混合/无群_05.jpg", Hash: "h-noSeq"}, attr: workAttribution{SiteKey: identity.Local.Key}}},
	}
	updates := collectWorkSetOrderUpdates(snap, items, rules)

	if len(updates) != 2 {
		t.Fatalf("应恰两个集键（序列群集=集甲、既有集=集丁），实得 %v", updates)
	}
	if got := updates[workSetBizKey("集甲")]; len(got) != 1 || got[0].SiteWorkId != "h-seq" || got[0].Seq != 2 {
		t.Fatalf("序列群键集成员应按集名快照补位供序（无终选声明亦可），实得 %+v", got)
	}
	if got := updates[workSetBizKey("集丁")]; len(got) != 1 || got[0].SiteWorkId != "h-legacy" || got[0].Seq != 4 {
		t.Fatalf("无产出四集归属的既有集名来源应零漂移，实得 %+v", got)
	}
	for _, name := range []string{"集乙", "集丙"} {
		if got, ok := updates[workSetBizKey(name)]; ok {
			t.Fatalf("行捕获复合/所在目录集不应供集内序（%s），实得 %+v", name, got)
		}
	}
}

// —— 消费面4：疑似作品集页目录卡与归集成员从分组层来 —— //

// TestWorkSetCandidateActionsDirGroupMembers 目录卡与归集成员从分组层来：识别
// 产出的目录来源候选卡成员=目录分组分节成员（同源逐字段），确认归集按卡成员
// 逐文件写集维度赋值（写法不变=决策2）、动作对账返回成员数=分组层成员数。
func TestWorkSetCandidateActionsDirGroupMembers(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	// 作品号目录：两个直接子文件无群归属、无规则命中 → 目录分组出卡。
	writeTestFile(t, root+"/100423498/a.txt", "a")
	writeTestFile(t, root+"/100423498/b.txt", "b")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("全量识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	snap := sess.Snapshot
	if snap == nil {
		t.Fatal("识别完成后快照不在场")
	}
	// 分组层分节唯一一条（作品号目录），候选墙目录卡成员与之逐字段同源。
	if len(snap.DirGroups) != 1 {
		t.Fatalf("目录分组分节应恰 1 条，实得 %+v", snap.DirGroups)
	}
	var card *SnapCandidate
	for i := range snap.Candidates {
		if snap.Candidates[i].Source == candSourceDir {
			card = &snap.Candidates[i]
			break
		}
	}
	if card == nil {
		t.Fatalf("候选墙缺少目录来源卡: %+v", snap.Candidates)
	}
	group := snap.DirGroups[0]
	if card.Dir != group.Dir || card.Key != group.Key || card.Size != group.Size ||
		len(card.Members) != len(group.Members) {
		t.Fatalf("目录候选卡应从分组层分节投影: card=%+v group=%+v", *card, group)
	}
	for i := range card.Members {
		if card.Members[i].RelPath != group.Members[i].RelPath {
			t.Fatalf("目录候选卡成员应与分节逐字段同源: card=%+v group=%+v", card.Members, group.Members)
		}
	}
	// 确认归集：按卡成员逐文件写集维度赋值（写法不变），返回受影响成员数。
	confirmedSess, affected, err := m.ConfirmWorkSetCandidate(root, card.target(), "目录作品集", 0)
	if err != nil {
		t.Fatalf("目录候选确认归集失败: %v", err)
	}
	if affected != len(group.Members) {
		t.Fatalf("确认归集受影响成员数 = %d，期望分组层成员数 %d", affected, len(group.Members))
	}
	for _, mem := range group.Members {
		if a := confirmedSess.Assignments[mem.RelPath]; a.Collection != "目录作品集" {
			t.Fatalf("成员 %s 应写入集维度赋值，实际 %+v", mem.RelPath, a)
		}
	}
}

// —— 风险4：增量轮集归属复原（跳过目录整组复用旧文件记录） —— //

// TestIncrementalReuseKeepsSetAttribution 增量轮集归属复原锚：目录级跳过复用
// 按「整条旧 SnapFile 记录」拷贝（跳过目录整组复原机制），集归属三字段为记录
// 内字段，天然随复用保留——跳过目录不因未重求值丢失集归属；重求值目录的集
// 归属由本轮汇流段重算（另见工作集聚合测试）。
func TestIncrementalReuseKeepsSetAttribution(t *testing.T) {
	root := t.TempDir()
	prev := &RecognitionSnapshot{
		RootPath: root,
		Dirs: []SnapDir{
			{RelKey: "", Name: "库", Depth: 0},
			{RelKey: "甲", Name: "甲", Depth: 1, FileCount: 1},
		},
		Files: []SnapFile{
			{RelPath: "甲/a.jpg", FileName: "a.jpg", DirRelKey: "甲", State: snapStateIdentified,
				SetGroupKey: "序列", SetName: "集甲", SetRuleID: "ws-seq",
				Final: []SnapDecl{{Dimension: string(DimCollection), Value: "集甲"}}},
		},
	}
	enumRes := enumerateResultOfSnapshot(prev)
	enumRes.SkippedDirKeys = []string{"甲"}
	snap := aggregatePanorama(aggregateInput{
		root:        root,
		enumRes:     enumRes,
		prev:        prev,
		prevSkipped: map[string]bool{"甲": true},
	})
	if len(snap.Files) != 1 {
		t.Fatalf("跳过目录复用应保留唯一文件记录，实得 %d: %+v", len(snap.Files), snap.Files)
	}
	f := snap.Files[0]
	if f.SetGroupKey != "序列" || f.SetName != "集甲" || f.SetRuleID != "ws-seq" {
		t.Fatalf("跳过目录复用应保留集归属三字段（增量轮集归属复原）: %+v", f)
	}
	if len(f.Final) == 0 || f.Final[0].Dimension != string(DimCollection) {
		t.Fatalf("跳过目录复用应保留既有终选声明: %+v", f.Final)
	}
	if snap.Stats.SkippedDirs != 1 {
		t.Fatalf("跳过目录计数应为 1，实得 %d", snap.Stats.SkippedDirs)
	}
}

// —— 消费面5：「隐藏已识别」数据位（后端投影断言；前端派生口径见 ParseView） —— //

// TestSnapshotSetNameProjection 快照投影保留集归属字段：产出四集名随 panorama
// files 分节（panoramaViewOf/projectWorkMergedFile）出线，前端「隐藏已识别」
// 可据文件的集名数据位判定成员已带集维度结论（不依赖本文件终选声明）。
func TestSnapshotSetNameProjection(t *testing.T) {
	snap := &RecognitionSnapshot{RootPath: "G:/库", Files: []SnapFile{
		{RelPath: "集/a.jpg", FileName: "a.jpg", DirRelKey: "集", State: snapStateIdentified,
			SetGroupKey: "集", SetName: "集名", SetRuleID: "ws"},
		{RelPath: "集/b.jpg", FileName: "b.jpg", DirRelKey: "集", State: snapStateIdentified},
	}}
	view := panoramaViewOf(snap)
	if len(view.Files) != 2 {
		t.Fatalf("投影文件数不符: %+v", view.Files)
	}
	if view.Files[0].SetName != "集名" || view.Files[0].SetGroupKey != "集" || view.Files[0].SetRuleID != "ws" {
		t.Fatalf("panorama files 分节应保留集归属三字段: %+v", view.Files[0])
	}
	if view.Files[1].SetName != "" {
		t.Fatalf("无集归属文件不应携带集名: %+v", view.Files[1])
	}
	// overview 投影不携带文件明细（既有分页契约），集归属随 files 分节出线。
	if overview := panoramaOverviewViewOf(snap); overview.Files != nil {
		t.Fatalf("overview 投影不应携带文件明细: %+v", overview.Files)
	}
}
