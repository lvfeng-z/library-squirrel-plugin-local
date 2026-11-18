// 配对归属消费面泛化的单元测试（阶段2）：映射单用作品（无聚合产出、配对段
// 归并）在选择域推导、详情投影、待裁决明细、创建链与执行期落库各消费面按
// 「配对归属」工作——说明成员归并到主导文件的作品（键=主导路径键）而不并
// 作品（主导维持独立形态、无轨、无作品类型），聚合场景行为与现状全等由既有
// 测试锚定（本文件只锚映射单用形态）。
package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// pairedSnap 映射单用场景的快照构造：说明成员携带配对归属（键=主导路径键+
// 说明标记），主导候选无归属（配对不并作品），另含孤立说明文件维持独立。
func pairedSnap() *RecognitionSnapshot {
	return &RecognitionSnapshot{
		Files: []SnapFile{
			// 主导先到形态：cover.jpg 建单文件作品，说明成员后到追加
			{RelPath: "无题/cover.jpg", FileName: "cover.jpg", DirRelKey: "无题", State: snapStateIdentified,
				Final: []SnapDecl{{Dimension: string(DimAuthor), Value: "Isel", Source: string(SourceFilename)}}},
			{RelPath: "无题/cover.txt", FileName: "cover.txt", DirRelKey: "无题", State: snapStatePlain,
				Final: []SnapDecl{{Dimension: string(DimDescription), Value: "cover 的说明内容", Source: string(SourceContent)}},
				AggWorkKey: "f:无题/cover.jpg", AggCompanion: true},
			// 说明成员先到形态：快照序 txt 在前、主导 jpg 殿后（域推导对位挪前）
			{RelPath: "无题/无题_01.txt", FileName: "无题_01.txt", DirRelKey: "无题", State: snapStatePlain,
				Final: []SnapDecl{{Dimension: string(DimDescription), Value: "无题_01 的说明内容", Source: string(SourceContent)}},
				AggWorkKey: "f:无题/无题_01.png", AggCompanion: true},
			{RelPath: "无题/无题_01.png", FileName: "无题_01.png", DirRelKey: "无题", State: snapStateIdentified,
				GroupKey: "无题", GroupSeq: 1},
			// 孤立说明文件（配对键取不到/守卫拒绝形态）：维持独立作品现状
			{RelPath: "无题/无题.txt", FileName: "无题.txt", DirRelKey: "无题", State: snapStatePlain},
		},
	}
}

// TestDeriveWorkDomainPairedWorksMerge 映射单用作品的选择域推导：说明成员并入
// 主导文件的作品（配对组键=主导路径键，与单文件作品键同值——两种到达序收敛
// 为同一组作品），Files 首位恒为主导（说明成员先到时对位挪前）、CompanionCount
// 数据位计数；主导候选不获归属（配对不并作品）也不因说明成员先到而丢作品键；
// 孤立说明文件维持单文件作品；集归属按主导文件群键（序列成员各自成件、各自
// 带说明——剧本B 帧1 形态的数据面）。
func TestDeriveWorkDomainPairedWorksMerge(t *testing.T) {
	domain := DeriveWorkDomain(pairedSnap(), nil)

	if len(domain.Works) != 3 {
		t.Fatalf("两件配对作品+孤立一件应共 3 件作品（说明成员不各自成件），实际 %d: %+v", len(domain.Works), domain.Works)
	}
	// 主导先到形态：Files=[主导, 说明]、键=主导路径键
	cover := domain.Work("f:无题/cover.jpg")
	if cover == nil {
		t.Fatalf("缺少配对作品 f:无题/cover.jpg（works: %+v）", domain.Works)
	}
	if !reflect.DeepEqual(cover.Files, []string{"无题/cover.jpg", "无题/cover.txt"}) {
		t.Fatalf("主导先到的配对作品 Files 应为主导在前双成员，实际 %v", cover.Files)
	}
	if cover.CompanionCount != 1 || cover.DirRelKey != "无题" {
		t.Fatalf("配对作品说明计数应为 1、目录键=主导目录键，实际 %+v", cover)
	}
	// 说明成员先到形态：对位挪前（Files 首位=主导）
	member := domain.Work("f:无题/无题_01.png")
	if member == nil {
		t.Fatalf("缺少配对作品 f:无题/无题_01.png（works: %+v）", domain.Works)
	}
	if !reflect.DeepEqual(member.Files, []string{"无题/无题_01.png", "无题/无题_01.txt"}) {
		t.Fatalf("说明成员先到的配对作品 Files 应对位挪前为主导首位，实际 %v", member.Files)
	}
	if member.CompanionCount != 1 || member.DirRelKey != "无题" {
		t.Fatalf("配对作品说明计数应为 1、目录键=后到主导目录键，实际 %+v", member)
	}
	// 作品键索引无冲突：两件配对作品与孤立作品各占一键
	if len(domain.byWork) != 3 {
		t.Fatalf("作品键索引应恰 3 键（配对组键与主导单文件键同值不分裂），实际 %d", len(domain.byWork))
	}
	// 说明成员 plain 不拉低作品：可选性按主导
	if !cover.Selectable || cover.State != workStateIdentified {
		t.Fatalf("配对作品应按主导文件可选（identified），实际 selectable=%v state=%s", cover.Selectable, cover.State)
	}
	// 孤立说明文件：维持单文件作品现状
	lone := domain.Work("f:无题/无题.txt")
	if lone == nil || len(lone.Files) != 1 || lone.CompanionCount != 0 || lone.Selectable {
		t.Fatalf("孤立说明文件应为不可选单文件作品（现状维持），实际 %+v", lone)
	}
	// 集归属按主导文件群键：序列成员（各自成作品）进集、整件为一集成员
	if len(domain.Sets) != 1 || len(domain.Sets[0].Members) != 1 || domain.Sets[0].Members[0] != member.WorkID {
		t.Fatalf("集视图应以配对作品为单成员（群键=主导文件群键），实际 sets: %+v", domain.Sets)
	}
}

// TestSelectionPairedLeadMergeProjection 映射单用作品的详情投影与并入冲突：
// 配对主导（无归属、配对组指向键在场）把说明成员的内容声明并入终选视图
//（描述补位合成——剧本B 帧2「来源：说明文件」数据面）；同维冲突挂主导文件
// 待裁决（明细经 workMergePendingOf 供给裁决面）；配对主导自身的声明只在创建
// 载荷口径收齐、投影口径只补其他成员；普通单文件零并入。
func TestSelectionPairedLeadMergeProjection(t *testing.T) {
	snap := pairedSnap()
	// 主导 cover 补一条与说明成员同维互异的声明（构造冲突面）
	snap.Files[0].Final = append(snap.Files[0].Final, SnapDecl{
		Dimension: string(DimDescription), Value: "主导侧描述", Source: string(SourceFilename)})

	contentDecls := workContentDeclsByWork(snap)
	// 投影口径：说明成员声明归集到配对组键、主导自身不收（cover 无内容声明、
	// 无题_01 主导亦无——两键各一条说明声明）
	if len(contentDecls) != 2 ||
		len(contentDecls["f:无题/cover.jpg"]) != 1 || len(contentDecls["f:无题/无题_01.png"]) != 1 {
		t.Fatalf("投影口径应按配对组键各收说明成员声明: %+v", contentDecls)
	}
	// 载荷口径：配对主导自身的内容声明同样收齐（无题_01 主导补内容声明后计入）
	snap.Files[3].Final = append(snap.Files[3].Final, SnapDecl{
		Dimension: string(DimTag), Value: "序列标签", Source: string(SourceContent)})
	payload := workPayloadContentDeclsByWork(snap)
	if len(payload["f:无题/无题_01.png"]) != 2 {
		t.Fatalf("载荷口径应收齐主导自身与说明成员的内容声明: %+v", payload["f:无题/无题_01.png"])
	}

	// 投影并入：无冲突主导（无题_01）描述补位合成
	merged := projectWorkMergedFile(snap.Files[3], contentDecls)
	found := false
	for _, d := range merged.Final {
		if d.Dimension == string(DimDescription) && d.Value == "无题_01 的说明内容" &&
			d.Source == string(SourceContent) && d.OriginName == "无题_01.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("配对主导投影应并入说明成员描述声明（整条保真），实际 %+v", merged.Final)
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
		t.Fatalf("并入冲突明细应挂配对主导文件（两值两来源），实际 %+v", pendings)
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

// TestEmitSelectedWorksPairedCompanionSingleChild 映射单用作品的创建链：哈希面
// 剔除说明成员（不哈希、不核对、不导子行——磁盘缺席不阻断创建），主导单条走
// 单文件子行（无轨道清单）而内容声明随主导子行固化进载荷；说明成员不独立成
// 件零子行（剧本B 帧3「3 个文件，另有说明文件 3」的创建数据面——单件口径）。
func TestEmitSelectedWorksPairedCompanionSingleChild(t *testing.T) {
	content := func(dim MetadataDimension, val string) SnapDecl {
		return SnapDecl{Dimension: string(dim), Value: val, Source: string(SourceContent)}
	}
	root, h, m, sess, works := aggEmitFixture(t, []SnapFile{
		{RelPath: "无题/无题_01.png", FileName: "无题_01.png", DirRelKey: "无题", State: snapStateIdentified,
			Final: []SnapDecl{{Dimension: string(DimAuthor), Value: "Isel", Source: string(SourceFilename)}}},
		{RelPath: "无题/无题_01.txt", FileName: "无题_01.txt", DirRelKey: "无题", State: snapStatePlain,
			Final:       []SnapDecl{content(DimDescription, "无题_01 的说明内容")},
			AggWorkKey:  "f:无题/无题_01.png",
			AggCompanion: true},
	})
	// 说明成员磁盘缺席：创建仍须成功（说明文件不 Stat 不哈希——若进哈希面即
	// 「磁盘对账未通过」中止）
	if err := os.Remove(root + "/无题/无题_01.txt"); err != nil {
		t.Fatalf("移除说明成员测试文件失败: %v", err)
	}

	if len(works) != 1 {
		t.Fatalf("两文件应并成一件配对作品，实际 %d: %+v", len(works), works)
	}
	work := works[0]
	if work.WorkID != "f:无题/无题_01.png" || len(work.Files) != 2 || work.CompanionCount != 1 {
		t.Fatalf("配对作品应为主导+说明双成员、计数 1，实际 %+v", work)
	}

	resps := emittedTasks(collectEmitted(t, h, root, sess, works, "纯映射批次"))
	if len(resps) != 1 {
		t.Fatalf("纯 local 批次应为单应答单父，实得 %d 个", len(resps))
	}
	children := resps[0].Children
	if len(children) != 1 {
		t.Fatalf("一件配对作品应恰一个子行（说明成员不导子行），实得 %d: %v", len(children), responseChildNames(resps[0]))
	}
	child := children[0]
	if child.TaskName != "无题_01.png" {
		t.Fatalf("子行应为哈希面主导文件，实际 %q", child.TaskName)
	}
	var fp FilePluginData
	if err := json.Unmarshal([]byte(child.PluginData), &fp); err != nil {
		t.Fatalf("解析子行 PluginData 失败: %v", err)
	}
	// 单文件子行形态：无轨道清单（区别于聚合子行的 Tracks）
	if len(fp.Tracks) != 0 {
		t.Fatalf("映射单用作品子行不应携带轨道清单（配对归并不落轨），实际 %+v", fp.Tracks)
	}
	// 内容声明随主导子行固化：说明成员的描述声明在场（任务自包含，执行期并入）
	if len(fp.ContentDecls) != 1 || fp.ContentDecls[0].Value != "无题_01 的说明内容" ||
		fp.ContentDecls[0].OriginName != "无题_01.txt" {
		t.Fatalf("说明成员内容声明应固化进主导子行载荷，实际 %+v", fp.ContentDecls)
	}
	// 批次摘要：单子行
	if summary := m.takeCreateBatchSummary(root); summary.total != 1 {
		t.Fatalf("批次摘要 total 应为 1（主导单子行），实际 %+v", summary)
	}
	_ = contract.StoreTypeSubtitle
}

// TestContentSourcePairedMappingFullChain 映射单用作品的识别全链（阶段2 退出
// 标准1 的前半链）：采集命中集（真实读盘）→ 内容注入 → 第二相整段取值产声明
//（描述=说明文件整段）→ 配对段说明成员归并（键=主导路径键+说明标记、无轨、
// 不落作品类型；主导候选不获归属）→ 快照落档 → 选择域推导（成员并入主导
// 作品、配对键取不到的说明文件维持独立——剧本B 帧1「无题.txt 独立成件」）。
func TestContentSourcePairedMappingFullChain(t *testing.T) {
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
			pairingFileRow("{作品}_{序号}", ".png"),
			pairingFileRow("{作品}_{序号}", ".txt"),
		},
		MetadataFile: &MetadataFileProduction{
			Pairing: &PairingKey{Mode: PairingModeIdentity, Identity: []IdentityPart{
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

	// 配对归属与第二相声明：说明成员带描述声明（整段）归并主导、无轨无类型。
	for i := 1; i <= 2; i++ {
		txt := aggConclusionOf(t, conclusions, "无题/无题_0"+string(rune('0'+i))+".txt")
		want := "f:无题/无题_0" + string(rune('0'+i)) + ".png"
		if txt.AggWorkKey != want || txt.AggTrackRole != "" || txt.AggWorkType != "" || !txt.AggCompanion {
			t.Fatalf("说明文件 %d 归属不符: key=%q role=%q type=%q companion=%v",
				i, txt.AggWorkKey, txt.AggTrackRole, txt.AggWorkType, txt.AggCompanion)
		}
		if !conclusionHasDecl(txt, DimDescription, "无题_0"+string(rune('0'+i))+" 的说明内容", SourceContent) {
			t.Fatalf("说明文件 %d 第二相应产整段描述声明: %+v", i, txt.Final)
		}
		png := aggConclusionOf(t, conclusions, "无题/无题_0"+string(rune('0'+i))+".png")
		if png.AggWorkKey != "" || png.AggCompanion {
			t.Fatalf("主导文件 %d 应不获归属（配对不并作品）: key=%q companion=%v", i, png.AggWorkKey, png.AggCompanion)
		}
	}
	// 组级说明文件：配对键取不到（无序号可解析）→ 维持独立作品现状。
	stray := aggConclusionOf(t, conclusions, "无题/无题.txt")
	if stray.AggWorkKey != "" || stray.AggCompanion {
		t.Fatalf("组级说明文件应维持独立作品: key=%q companion=%v", stray.AggWorkKey, stray.AggCompanion)
	}

	// 快照落档 → 选择域：两件配对作品+一件独立组级说明，成员并入主导作品。
	assignIdx := newAssignmentDirIndex(nil, nil)
	snap := &RecognitionSnapshot{}
	for i := range conclusions {
		snap.Files = append(snap.Files, snapFileOf(&conclusions[i], nil, assignIdx))
	}
	domain := DeriveWorkDomain(snap, nil)
	if len(domain.Works) != 3 {
		t.Fatalf("两件配对作品+组级说明一件应共 3 件作品，实际 %d: %+v", len(domain.Works), domain.Works)
	}
	for i := 1; i <= 2; i++ {
		work := domain.Work("f:无题/无题_0" + string(rune('0'+i)) + ".png")
		if work == nil || len(work.Files) != 2 || work.Files[0] != "无题/无题_0"+string(rune('0'+i))+".png" ||
			work.CompanionCount != 1 {
			t.Fatalf("配对作品 %d 应为主导+说明双成员（主导首位、计数 1），实际 %+v", i, work)
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

// TestMergeWorkPairedCreateWorkInfoDescription 映射单用作品的执行期落库：主导
// 子行载荷固化的说明成员内容声明经作品级并入合成——描述=说明文件整段内容落
// 库（SiteWorkDescription）；主导自身的结论（作者）照常在场（重算 ∪ 载荷）。
func TestMergeWorkPairedCreateWorkInfoDescription(t *testing.T) {
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
