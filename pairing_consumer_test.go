// 扇出归属消费面的单元测试：映射单用作品（无聚合产出、扇出归属段标记）在
// 选择域推导、详情投影、待裁决明细、创建链与执行期落库各消费面按「内容
// 扇出键集」工作——元数据源文件不导入不独立成件、按扇出键集并入各目标
// 作品（键=目标路径键），目标文件维持独立形态（无轨、无作品类型），聚合
// 场景行为与现状全等由既有测试锚定（本文件只锚映射单用形态）。
package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// fanoutSnap 映射单用场景的快照构造：元数据源文件携带内容扇出键集（键集=
// 组内作品的键）与说明成员标记、零聚合归属，目标文件维持独立形态，另含
// 孤立说明文件维持独立（孤文件回退）。
func fanoutSnap() *RecognitionSnapshot {
	return &RecognitionSnapshot{
		Files: []SnapFile{
			// 目标先到形态：cover.jpg 建单文件作品，元数据源文件后到并入
			{RelPath: "无题/cover.jpg", FileName: "cover.jpg", DirRelKey: "无题", State: snapStateIdentified,
				Final: []SnapDecl{{Dimension: string(DimAuthor), Value: "Isel", Source: string(SourceFilename)}}},
			{RelPath: "无题/cover.txt", FileName: "cover.txt", DirRelKey: "无题", State: snapStatePlain,
				Final:        []SnapDecl{{Dimension: string(DimDescription), Value: "cover 的说明内容", Source: string(SourceContent)}},
				AggCompanion: true, FanoutWorkKeys: []string{"f:无题/cover.jpg"}},
			// 元数据源先到形态：快照序 txt 在前、目标 png 殿后（扇出并入后
			// Files 首位恒为目标）
			{RelPath: "无题/无题_01.txt", FileName: "无题_01.txt", DirRelKey: "无题", State: snapStatePlain,
				Final:        []SnapDecl{{Dimension: string(DimDescription), Value: "无题_01 的说明内容", Source: string(SourceContent)}},
				AggCompanion: true, FanoutWorkKeys: []string{"f:无题/无题_01.png"}},
			{RelPath: "无题/无题_01.png", FileName: "无题_01.png", DirRelKey: "无题", State: snapStateIdentified,
				GroupKey: "无题", GroupSeq: 1},
			// 孤立说明文件（分组键取不到/无落点形态）：按独立作品导入
			{RelPath: "无题/无题.txt", FileName: "无题.txt", DirRelKey: "无题", State: snapStatePlain},
		},
	}
}

// TestDeriveWorkDomainFanoutWorksMerge 映射单用作品的选择域推导：元数据源
// 文件并入扇出键集内各目标作品（作品键=目标文件路径键——两种到达序收敛为
// 同一组作品），Files 首位恒为目标文件、CompanionCount 数据位计数；目标文件
// 不获归属（扇出不并作品）也不因元数据源先到而丢作品键；孤立说明文件维持
// 单文件作品；集归属按目标文件群键（序列成员各自成件、各自带说明——剧本B
// 帧1 形态的数据面）。
func TestDeriveWorkDomainFanoutWorksMerge(t *testing.T) {
	domain := DeriveWorkDomain(fanoutSnap(), nil)

	if len(domain.Works) != 3 {
		t.Fatalf("两件扇出作品+孤立一件应共 3 件作品（元数据源不各自成件），实际 %d: %+v", len(domain.Works), domain.Works)
	}
	// 目标先到形态：Files=[目标, 元数据源]、键=目标路径键
	cover := domain.Work("f:无题/cover.jpg")
	if cover == nil {
		t.Fatalf("缺少扇出目标作品 f:无题/cover.jpg（works: %+v）", domain.Works)
	}
	if !reflect.DeepEqual(cover.Files, []string{"无题/cover.jpg", "无题/cover.txt"}) {
		t.Fatalf("目标先到的扇出作品 Files 应为目标在前双成员，实际 %v", cover.Files)
	}
	if cover.CompanionCount != 1 || cover.DirRelKey != "无题" {
		t.Fatalf("扇出作品说明计数应为 1、目录键=目标目录键，实际 %+v", cover)
	}
	// 元数据源先到形态：Files 首位恒为目标
	member := domain.Work("f:无题/无题_01.png")
	if member == nil {
		t.Fatalf("缺少扇出目标作品 f:无题/无题_01.png（works: %+v）", domain.Works)
	}
	if !reflect.DeepEqual(member.Files, []string{"无题/无题_01.png", "无题/无题_01.txt"}) {
		t.Fatalf("元数据源先到的扇出作品 Files 应目标居首，实际 %v", member.Files)
	}
	if member.CompanionCount != 1 || member.DirRelKey != "无题" {
		t.Fatalf("扇出作品说明计数应为 1、目录键=目标目录键，实际 %+v", member)
	}
	// 作品键索引无冲突：两件扇出作品与孤立作品各占一键
	if len(domain.byWork) != 3 {
		t.Fatalf("作品键索引应恰 3 键，实际 %d", len(domain.byWork))
	}
	// 元数据源 plain 不拉低作品：可选性按目标
	if !cover.Selectable || cover.State != workStateIdentified {
		t.Fatalf("扇出作品应按目标文件可选（identified），实际 selectable=%v state=%s", cover.Selectable, cover.State)
	}
	// 孤立说明文件：维持单文件作品现状
	lone := domain.Work("f:无题/无题.txt")
	if lone == nil || len(lone.Files) != 1 || lone.CompanionCount != 0 || lone.Selectable {
		t.Fatalf("孤立说明文件应为不可选单文件作品（现状维持），实际 %+v", lone)
	}
	// 集归属按目标文件群键：序列成员（各自成作品）进集、整件为一集成员
	if len(domain.Sets) != 1 || len(domain.Sets[0].Members) != 1 || domain.Sets[0].Members[0] != member.WorkID {
		t.Fatalf("集视图应以扇出作品为单成员（群键=目标文件群键），实际 sets: %+v", domain.Sets)
	}
}

// TestSelectionFanoutLeadMergeProjection 映射单用作品的详情投影与并入冲突：
// 扇出目标文件（零归属、其作品键在元数据源的扇出键集内）把说明成员的内容
// 声明并入终选视图（描述补位合成——剧本B 帧2「来源：说明文件」数据面）；
// 同维冲突挂目标文件待裁决（明细经 workMergePendingOf 供给裁决面）；双重
// 身份目标（自身携内容声明+扇出键集）的声明只在创建载荷口径收齐、投影口径
// 只补其他成员；普通单文件零并入。
func TestSelectionFanoutLeadMergeProjection(t *testing.T) {
	snap := fanoutSnap()
	// 目标 cover 补一条与说明成员同维互异的声明（构造冲突面）
	snap.Files[0].Final = append(snap.Files[0].Final, SnapDecl{
		Dimension: string(DimDescription), Value: "主导侧描述", Source: string(SourceFilename)})

	contentDecls := workContentDeclsByWork(snap)
	// 投影口径：元数据源声明归集到扇出键集内各键、目标自身不收（cover 无
	// 内容声明、无题_01 目标亦无——两键各一条说明声明）
	if len(contentDecls) != 2 ||
		len(contentDecls["f:无题/cover.jpg"]) != 1 || len(contentDecls["f:无题/无题_01.png"]) != 1 {
		t.Fatalf("投影口径应按扇出键集各收说明成员声明: %+v", contentDecls)
	}
	// 载荷口径：双重身份目标（无题_01 自身的内容声明随扇出键集收齐）计入
	//（载荷构造只在快照上临时挂扇出键集，投影口径恢复零键集形态）
	snap.Files[3].FanoutWorkKeys = []string{"f:无题/无题_01.png"}
	snap.Files[3].Final = append(snap.Files[3].Final, SnapDecl{
		Dimension: string(DimTag), Value: "序列标签", Source: string(SourceContent)})
	payload := workPayloadContentDeclsByWork(snap)
	if len(payload["f:无题/无题_01.png"]) != 2 {
		t.Fatalf("载荷口径应收齐双重身份目标自身与说明成员的内容声明: %+v", payload["f:无题/无题_01.png"])
	}
	snap.Files[3].FanoutWorkKeys = nil

	// 投影并入：无冲突目标（无题_01）描述补位合成
	merged := projectWorkMergedFile(snap.Files[3], contentDecls)
	found := false
	for _, d := range merged.Final {
		if d.Dimension == string(DimDescription) && d.Value == "无题_01 的说明内容" &&
			d.Source == string(SourceContent) && d.OriginName == "无题_01.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("扇出目标投影应并入说明成员描述声明（整条保真），实际 %+v", merged.Final)
	}
	if len(merged.PendingDims) != 0 {
		t.Fatalf("无冲突并入不应落待裁决维度，实际 %+v", merged.PendingDims)
	}
	// 投影并入：同维冲突主导（cover）——冲突维移出终选、挂待裁决维度
	conflicted := projectWorkMergedFile(snap.Files[0], contentDecls)
	for _, d := range conflicted.Final {
		if d.Dimension == string(DimDescription) {
			t.Fatalf("冲突维应移出终选呈现（维度进待裁决即不双呈值），实际 %+v", conflicted.Final)
		}
	}
	if !containsDim(conflicted.PendingDims, string(DimDescription)) {
		t.Fatalf("冲突维应挂主导待裁决维度标记，实际 %+v", conflicted.PendingDims)
	}
	// 冲突明细：挂主导文件、候选两值两来源（裁决面供给）
	pendings := workMergePendingOf(snap, contentDecls)
	if len(pendings) != 1 || pendings[0].RelPath != "无题/cover.jpg" ||
		pendings[0].Dimension != string(DimDescription) || len(pendings[0].Candidates) != 2 {
		t.Fatalf("并入冲突明细应挂扇出目标文件（两值两来源），实际 %+v", pendings)
	}
	// 普通单文件（孤立说明文件）零并入：原样返回
	lone := projectWorkMergedFile(snap.Files[4], contentDecls)
	if !reflect.DeepEqual(lone.Final, snap.Files[4].Final) || len(lone.PendingDims) != 0 {
		t.Fatalf("普通单文件投影应零并入，实际 %+v", lone)
	}
	// 说明成员行只补内容采集状态位、终选原样
	companionRow := projectWorkMergedFile(snap.Files[1], contentDecls)
	if !reflect.DeepEqual(companionRow.Final, snap.Files[1].Final) {
		t.Fatalf("说明成员行投影应只补状态位不改终选，实际 %+v", companionRow.Final)
	}
}

// containsDim 维度字符串清单包含判定（测试断言助手）。
func containsDim(dims []string, want string) bool {
	for _, d := range dims {
		if d == want {
			return true
		}
	}
	return false
}

// TestEmitSelectedWorksFanoutCompanionSingleChild 映射单用作品的创建链：哈希面
// 剔除元数据源文件（不哈希、不核对、不导子行——磁盘缺席不阻断创建），目标单条走
// 单文件子行（无轨道清单）而内容声明随目标子行固化进载荷；元数据源不独立成
// 件零子行（剧本B 帧3「3 个文件，另有说明文件 3」的创建数据面——单件口径）。
func TestEmitSelectedWorksFanoutCompanionSingleChild(t *testing.T) {
	content := func(dim MetadataDimension, val string) SnapDecl {
		return SnapDecl{Dimension: string(dim), Value: val, Source: string(SourceContent)}
	}
	root, h, m, sess, works := aggEmitFixture(t, []SnapFile{
		{RelPath: "无题/无题_01.png", FileName: "无题_01.png", DirRelKey: "无题", State: snapStateIdentified,
			Final: []SnapDecl{{Dimension: string(DimAuthor), Value: "Isel", Source: string(SourceFilename)}}},
		{RelPath: "无题/无题_01.txt", FileName: "无题_01.txt", DirRelKey: "无题", State: snapStatePlain,
			Final:          []SnapDecl{content(DimDescription, "无题_01 的说明内容")},
			AggCompanion:   true,
			FanoutWorkKeys: []string{"f:无题/无题_01.png"}},
	})
	// 元数据源磁盘缺席：创建仍须成功（说明文件不 Stat 不哈希——若进哈希面即
	// 「磁盘对账未通过」中止）
	if err := os.Remove(root + "/无题/无题_01.txt"); err != nil {
		t.Fatalf("移除元数据源测试文件失败: %v", err)
	}

	if len(works) != 1 {
		t.Fatalf("两文件应并成一件扇出目标作品，实际 %d: %+v", len(works), works)
	}
	work := works[0]
	if work.WorkID != "f:无题/无题_01.png" || len(work.Files) != 2 || work.CompanionCount != 1 {
		t.Fatalf("扇出作品应为目标+元数据源双成员、计数 1，实际 %+v", work)
	}

	resps := emittedTasks(collectEmitted(t, h, root, sess, works, "纯映射批次"))
	if len(resps) != 1 {
		t.Fatalf("纯 local 批次应为单应答单父，实得 %d 个", len(resps))
	}
	children := resps[0].Children
	if len(children) != 1 {
		t.Fatalf("一件扇出作品应恰一个子行（元数据源不导子行），实得 %d: %v", len(children), responseChildNames(resps[0]))
	}
	child := children[0]
	if child.TaskName != "无题_01.png" {
		t.Fatalf("子行应为哈希面目标文件，实际 %q", child.TaskName)
	}
	var fp FilePluginData
	if err := json.Unmarshal([]byte(child.PluginData), &fp); err != nil {
		t.Fatalf("解析子行 PluginData 失败: %v", err)
	}
	// 单文件子行形态：无轨道清单（区别于聚合子行的 Tracks）
	if len(fp.Tracks) != 0 {
		t.Fatalf("映射单用作品子行不应携带轨道清单（扇出不落轨），实际 %+v", fp.Tracks)
	}
	// 内容声明随目标子行固化：元数据源的描述声明在场（任务自包含，执行期并入）
	if len(fp.ContentDecls) != 1 || fp.ContentDecls[0].Value != "无题_01 的说明内容" ||
		fp.ContentDecls[0].OriginName != "无题_01.txt" {
		t.Fatalf("元数据源内容声明应固化进目标子行载荷，实际 %+v", fp.ContentDecls)
	}
	// 批次摘要：单子行
	if summary := m.takeCreateBatchSummary(root); summary.total != 1 {
		t.Fatalf("批次摘要 total 应为 1（目标单子行），实际 %+v", summary)
	}
	_ = contract.StoreTypeSubtitle
}

// TestContentSourceFanoutMappingFullChain 映射单用作品的识别全链（阶段2 退出
// 标准1 的前半链）：采集命中集（真实读盘）→ 内容注入 → 第二相整段取值产声明
// （描述=说明文件整段）→ 扇出归属段标记（元数据源=零归属键+说明成员标记+
// 扇出键集、无轨、不落作品类型；目标文件不获归属）→ 快照落档 → 选择域推导
// （元数据源并入目标作品、不命中条件行的说明文件按独立作品导入——剧本B
// 帧1「无题.txt 独立成件」）。
func TestContentSourceFanoutMappingFullChain(t *testing.T) {
	root := t.TempDir()
	entries := []FileEntry{
		{FullPath: root + "/无题/无题_01.png", RelPath: "无题/无题_01.png"},
		{FullPath: root + "/无题/无题_02.png", RelPath: "无题/无题_02.png"},
		{FullPath: root + "/无题/无题_01.txt", RelPath: "无题/无题_01.txt"},
		{FullPath: root + "/无题/无题_02.txt", RelPath: "无题/无题_02.txt"},
		{FullPath: root + "/无题/无题.txt", RelPath: "无题/无题.txt"},
	}
	writeTestFile(t, entries[0].FullPath, "png")
	writeTestFile(t, entries[1].FullPath, "png")
	writeTestFile(t, entries[2].FullPath, "无题_01 的说明内容")
	writeTestFile(t, entries[3].FullPath, "无题_02 的说明内容")
	writeTestFile(t, entries[4].FullPath, "组级说明")
	rule := RuleEntry{
		ID:          "map-seq-full",
		Description: "测试：序列各自成作品+各自说明",
		Source:      RuleSourceManual,
		Enabled:     true,
		Conditions: []CondxRow{
			fanoutFileRow("{作品}_{序号}", ".png"),
			fanoutFileRow("{作品}_{序号}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Grouping: &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
				{Row: 1, Capture: "work"},
				{Row: 1, Capture: "seq"},
			}},
			ContentBindings: []ContentBinding{{Row: 2, Dimension: DimDescription}},
		},
	}
	rules := []RuleEntry{rule}
	files := buildScanFiles(entries)
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex())

	// 采集命中集：绑定行命中的两个序号说明文件（组级「无题.txt」不命中条件行）。
	hits := companionContentHitSet(rules, conclusions)
	if !reflect.DeepEqual(hits, []string{"无题/无题_01.txt", "无题/无题_02.txt"}) {
		t.Fatalf("采集命中集应恰含两个序号说明文件: %+v", hits)
	}
	samples, _ := collectCompanionContentSet(hits, entries)
	applyCompanionContent(files, samples)
	applyCompanionContentDeclarations(conclusions, files, rules, nil)

	// 扇出归属与第二相声明：元数据源带描述声明（整段）扇出到组内唯一作品
	//（零归属键+说明成员标记；目标不获归属——扇出不并作品）。
	for i := 1; i <= 2; i++ {
		txt := aggConclusionOf(t, conclusions, "无题/无题_0"+string(rune('0'+i))+".txt")
		want := []string{"f:无题/无题_0" + string(rune('0'+i)) + ".png"}
		if txt.AggWorkKey != "" || txt.AggTrackRole != "" || txt.AggWorkType != "" || !txt.AggCompanion ||
			!reflect.DeepEqual(txt.FanoutWorkKeys, want) {
			t.Fatalf("说明文件 %d 归属不符: key=%q role=%q type=%q companion=%v fanout=%v",
				i, txt.AggWorkKey, txt.AggTrackRole, txt.AggWorkType, txt.AggCompanion, txt.FanoutWorkKeys)
		}
		if !conclusionHasDecl(txt, DimDescription, "无题_0"+string(rune('0'+i))+" 的说明内容", SourceContent) {
			t.Fatalf("说明文件 %d 第二相应产整段描述声明: %+v", i, txt.Final)
		}
		png := aggConclusionOf(t, conclusions, "无题/无题_0"+string(rune('0'+i))+".png")
		if png.AggWorkKey != "" || png.AggCompanion {
			t.Fatalf("目标文件 %d 应不获归属（扇出不并作品）: key=%q companion=%v", i, png.AggWorkKey, png.AggCompanion)
		}
	}
	// 组级说明文件：不命中任何条件行（分组键不可得）→ 按独立作品导入。
	stray := aggConclusionOf(t, conclusions, "无题/无题.txt")
	if stray.AggWorkKey != "" || stray.AggCompanion {
		t.Fatalf("组级说明文件应按独立作品导入: key=%q companion=%v", stray.AggWorkKey, stray.AggCompanion)
	}

	// 快照落档 → 选择域：两件扇出作品+一件独立组级说明，元数据源并入目标作品。
	assignIdx := newAssignmentDirIndex(nil, nil)
	snap := &RecognitionSnapshot{}
	for i := range conclusions {
		snap.Files = append(snap.Files, snapFileOf(&conclusions[i], nil, assignIdx))
	}
	domain := DeriveWorkDomain(snap, nil)
	if len(domain.Works) != 3 {
		t.Fatalf("两件扇出作品+组级说明一件应共 3 件作品，实际 %d: %+v", len(domain.Works), domain.Works)
	}
	for i := 1; i <= 2; i++ {
		work := domain.Work("f:无题/无题_0" + string(rune('0'+i)) + ".png")
		if work == nil || len(work.Files) != 2 || work.Files[0] != "无题/无题_0"+string(rune('0'+i))+".png" ||
			work.CompanionCount != 1 {
			t.Fatalf("扇出作品 %d 应为目标+说明双成员（目标首位、计数 1），实际 %+v", i, work)
		}
	}
	if lone := domain.Work("f:无题/无题.txt"); lone == nil || len(lone.Files) != 1 {
		t.Fatalf("组级说明文件应为独立单文件作品，实际 %+v", lone)
	}
}

// conclusionHasDecl 结论终选声明包含判定（维度+值+来源三对齐；测试断言
// 助手——finalHasDecl 为快照行形态的既有助手，此处为求值结论形态）。
func conclusionHasDecl(c *FileConclusion, dim MetadataDimension, val string, src SourceKind) bool {
	for _, d := range c.Final {
		if d.Dimension == dim && d.Value == val && d.Source == src {
			return true
		}
	}
	return false
}

// TestMergeWorkFanoutCreateWorkInfoDescription 映射单用作品的执行期落库：目标
// 子行载荷固化的元数据源内容声明经作品级并入合成——描述=说明文件整段内容落
// 库（SiteWorkDescription）；主导自身的结论（作者）照常在场（重算 ∪ 载荷）。
func TestMergeWorkFanoutCreateWorkInfoDescription(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root+"/无题/无题_01.png", "图片内容")
	writeTestFile(t, root+"/无题/无题_01.txt", "无题_01 的说明内容")
	hash, err := ComputeFileHash(root + "/无题/无题_01.png")
	if err != nil {
		t.Fatalf("计算测试文件哈希失败: %v", err)
	}
	fp := &FilePluginData{
		SchemaVersion: currentPluginDataVersion,
		FullPath:      root + "/无题/无题_01.png",
		RelPath:       "无题/无题_01.png",
		Hash:          hash,
		Metadata: []PathMeaning{
			{Type: string(PathTypeSiteAuthor), Name: "Isel"},
		},
		ContentDecls: []SnapDecl{{
			Dimension: string(DimDescription), Value: "无题_01 的说明内容",
			Source: string(SourceContent), OriginName: "无题_01.txt",
		}},
	}
	fpJSON, _ := json.Marshal(fp)
	h := &LocalImportWorkFetcher{}
	taskData := string(fpJSON)
	resp, err := h.CreateWorkInfo(&sdkdto.TaskDTO{PluginData: &taskData})
	if err != nil {
		t.Fatalf("CreateWorkInfo 不应报错: %v", err)
	}
	if resp.Work.SiteWorkDescription == nil || *resp.Work.SiteWorkDescription != "无题_01 的说明内容" {
		t.Fatalf("描述应=说明文件整段内容（载荷声明并入落库），实际 %+v", resp.Work.SiteWorkDescription)
	}
	if len(resp.LocalAuthors) != 1 || resp.LocalAuthors[0].AuthorName == nil || *resp.LocalAuthors[0].AuthorName != "Isel" {
		t.Fatalf("主导结论（目录含义链作者）应照常落本地作者，实际 %+v", resp.LocalAuthors)
	}
}
