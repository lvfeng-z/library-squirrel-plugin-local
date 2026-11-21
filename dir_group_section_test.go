// 目录分组分节（分组层产物）锚：探测判定迁入快照新分节 detectDirGroups（原目录
// 候选探测 detectDirCandidates），集名候选墙的目录来源由分节投影
// dirGroupCandidates——迁移前后候选逐字段等值（含探测条件、键、成员清单、理由
// 与键元素池）；分节独立于序列群卡分节（不污染群卡消费面）；分节随快照 JSON
// 往返保真、旧快照无本字段零值降级。
package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// dirGroupSectionFixture 目录分组分节锚夹具：作品号目录、剥壳目录、裸括号目录、
// 作品号+单一群键双信号目录、单一群键目录、规则定群覆盖目录（不出）、无信号/
// 单文件/异群键目录（不出）、库根（不出）。
func dirGroupSectionFixture() *RecognitionSnapshot {
	plainFile := func(relPath, dirKey, name string) SnapFile {
		return SnapFile{RelPath: relPath, FileName: name, DirRelKey: dirKey, State: snapStatePlain}
	}
	return &RecognitionSnapshot{
		RootPath: "G:/inner world/picture",
		Dirs: []SnapDir{
			{RelKey: "", Name: "picture", Depth: 0},
			{RelKey: "100423498", Name: "100423498", Depth: 1},
			{RelKey: "100423498/sub", Name: "sub", Depth: 2},
			{RelKey: "Artist[100423501]", Name: "Artist[100423501]", Depth: 1},
			{RelKey: "[100423502]", Name: "[100423502]", Depth: 1},
			{RelKey: "100423600", Name: "100423600", Depth: 1},
			{RelKey: "结构群目录", Name: "结构群目录", Depth: 1},
			{RelKey: "100423700", Name: "100423700", Depth: 1},
			{RelKey: "无信号目录", Name: "无信号目录", Depth: 1},
			{RelKey: "100423499", Name: "100423499", Depth: 1},
			{RelKey: "异群键目录", Name: "异群键目录", Depth: 1},
		},
		Files: []SnapFile{
			// 作品号目录：4 个散文件（无群归属）→ 命中
			plainFile("100423498/a.jpg", "100423498", "a.jpg"),
			plainFile("100423498/b_01.jpg", "100423498", "b_01.jpg"),
			plainFile("100423498/c_02.jpg", "100423498", "c_02.jpg"),
			plainFile("100423498/d.jpg", "100423498", "d.jpg"),
			// 子目录文件不计入父目录分组成员（成员=目录直接子文件）
			plainFile("100423498/sub/inner.jpg", "100423498/sub", "inner.jpg"),
			// 剥壳目录名含作品号：分组键取内名
			plainFile("Artist[100423501]/x.jpg", "Artist[100423501]", "x.jpg"),
			plainFile("Artist[100423501]/y.jpg", "Artist[100423501]", "y.jpg"),
			// 裸括号作品号：分组键取内名
			plainFile("[100423502]/无题_01.jpg", "[100423502]", "无题_01.jpg"),
			plainFile("[100423502]/无题_02.jpg", "[100423502]", "无题_02.jpg"),
			// 作品号 + 整目录单一群键：双信号齐
			{RelPath: "100423600/g_01.jpg", FileName: "g_01.jpg", DirRelKey: "100423600", State: snapStatePlain, GroupKey: "集键", GroupSeq: 1},
			{RelPath: "100423600/g_02.jpg", FileName: "g_02.jpg", DirRelKey: "100423600", State: snapStatePlain, GroupKey: "集键", GroupSeq: 2},
			// 单一结构群键成集：出目录分组
			{RelPath: "结构群目录/散集 01.jpg", FileName: "散集 01.jpg", DirRelKey: "结构群目录", State: snapStatePlain, GroupKey: "散集", GroupSeq: 1},
			{RelPath: "结构群目录/散集 02.jpg", FileName: "散集 02.jpg", DirRelKey: "结构群目录", State: snapStatePlain, GroupKey: "散集", GroupSeq: 2},
			// 规则定群覆盖：不重复出（群来源已有卡）
			{RelPath: "100423700/r_1.jpg", FileName: "r_1.jpg", DirRelKey: "100423700", State: snapStatePlain, GroupKey: "rg", GroupSeq: 1},
			{RelPath: "100423700/r_2.jpg", FileName: "r_2.jpg", DirRelKey: "100423700", State: snapStatePlain, GroupKey: "rg", GroupSeq: 2},
			// 无信号目录：不出
			plainFile("无信号目录/x.jpg", "无信号目录", "x.jpg"),
			plainFile("无信号目录/y.jpg", "无信号目录", "y.jpg"),
			// 单文件目录：不出
			plainFile("100423499/only.jpg", "100423499", "only.jpg"),
			// 异群键目录：全部有群键但非单一群键 → 无作品号信号，不出
			{RelPath: "异群键目录/m_1.jpg", FileName: "m_1.jpg", DirRelKey: "异群键目录", State: snapStatePlain, GroupKey: "甲", GroupSeq: 1},
			{RelPath: "异群键目录/m_2.jpg", FileName: "m_2.jpg", DirRelKey: "异群键目录", State: snapStatePlain, GroupKey: "乙", GroupSeq: 1},
		},
		Groups: []SnapGroup{
			{DirRelKey: "100423700", Key: "rg", Size: 2, RuleID: "test-sibling-set"},
			{DirRelKey: "结构群目录", Key: "散集", Size: 2, FormName: "尾缀数字"},
			{DirRelKey: "100423600", Key: "集键", Size: 2},
		},
	}
}

// legacyDetectDirCandidateCardsRef 迁移前目录候选探测实现快照（逐字段等值参考
// 实现）：判定与成员计算与迁移前的 detectDirCandidates 同文，仅作迁移对照锚，
// 生产路径已改由 detectDirGroups（分组层分节）+ dirGroupCandidates（投影）。
func legacyDetectDirCandidateCardsRef(snap *RecognitionSnapshot) []SnapCandidate {
	ruleGroupKeys := make(map[string]bool, len(snap.Groups))
	for _, g := range snap.Groups {
		if g.RuleID != "" {
			ruleGroupKeys[g.DirRelKey+"\x00"+g.Key] = true
		}
	}
	filesByDir := make(map[string][]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		filesByDir[f.DirRelKey] = append(filesByDir[f.DirRelKey], f)
	}
	var cands []SnapCandidate
	for _, dir := range snap.Dirs {
		if dir.RelKey == "" {
			continue
		}
		members := filesByDir[dir.RelKey]
		if len(members) < 2 {
			continue
		}
		ruleCovered := false
		groupKeys := make(map[string]bool, len(members))
		allGrouped := true
		for _, f := range members {
			if f.GroupKey == "" {
				allGrouped = false
				continue
			}
			groupKeys[f.GroupKey] = true
			if ruleGroupKeys[f.DirRelKey+"\x00"+f.GroupKey] {
				ruleCovered = true
				break
			}
		}
		if ruleCovered {
			continue
		}
		workNo := workNumberOf(dir.Name)
		singleGroup := allGrouped && len(groupKeys) == 1
		if workNo == "" && !singleGroup {
			continue
		}
		key := dir.Name
		if _, inner, ok := stripShell(dir.Name); ok {
			key = inner
		}
		var parts []string
		if workNo != "" {
			parts = append(parts, "目录名含作品号 "+workNo)
		}
		if singleGroup {
			parts = append(parts, fmt.Sprintf("目录内 %d 个文件已成一集", len(members)))
		} else {
			parts = append(parts, fmt.Sprintf("目录内 %d 个文件", len(members)))
		}
		cand := SnapCandidate{
			Key: key, Dir: dir.RelKey, Source: candSourceDir, Size: len(members),
			Reason:      strings.Join(parts, "，"),
			Members:     make([]SnapCandMember, 0, len(members)),
			KeyElements: dirNameElements(key),
		}
		for _, f := range members {
			mem := SnapCandMember{RelPath: f.RelPath}
			if base := BaseNameWithoutExt(f.FileName); base != "" {
				els := splitFilenameElements(base, elementHints(snap.RootPath, f.RelPath))
				for i := len(els) - 1; i >= 0; i-- {
					if els[i].Kind == elKindSeq {
						mem.SeqText = elValue(els[i])
						break
					}
				}
				mem.Elements = els
			}
			cand.Members = append(cand.Members, mem)
		}
		cands = append(cands, cand)
	}
	return cands
}

// TestDirGroupSectionMigrationEquivalence 迁移对照锚：分节探测（detectDirGroups）
// 经候选投影（dirGroupCandidates）产出的目录来源候选与迁移前探测实现逐字段
// 全等——探测条件（作品号/单一群键）、键（剥壳目录名）、成员清单（路径/序号
// 原文/元素数组）、理由文本与键元素池一致；规则定群覆盖/无信号/单文件/异群键
// 目录与库根均不出。
func TestDirGroupSectionMigrationEquivalence(t *testing.T) {
	snap := dirGroupSectionFixture()
	want := legacyDetectDirCandidateCardsRef(snap)
	groups := detectDirGroups(snap)
	got := dirGroupCandidates(groups)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("目录分组分节投影与迁移前候选探测不全等：\ngot:  %+v\nwant: %+v", got, want)
	}
	if len(groups) != 5 {
		t.Fatalf("目录分组数 = %d，期望 5（作品号/剥壳/裸括号/双信号/单一群键）：%+v", len(groups), groups)
	}
	byDir := make(map[string]SnapDirGroup, len(groups))
	for _, g := range groups {
		byDir[g.Dir] = g
	}
	wantByDir := make(map[string]SnapCandidate, len(want))
	for _, c := range want {
		wantByDir[c.Dir] = c
	}
	for dir, g := range byDir {
		c := wantByDir[dir]
		if g.Key != c.Key || g.Size != c.Size || !reflect.DeepEqual(g.Members, c.Members) ||
			!reflect.DeepEqual(g.KeyElements, c.KeyElements) {
			t.Fatalf("目录分组 %s 的键/成员/键元素与迁移前候选不等：\ngroup: %+v\ncand:  %+v", dir, g, c)
		}
	}
	// 探测条件入分节：作品号信号与整目录单一群键分列可判（理由由两条件现拼）
	double := byDir["100423600"]
	if double.WorkNo != "100423600" || !double.SingleGroup {
		t.Fatalf("双信号目录应同时携带作品号与单一群键条件: %+v", double)
	}
	if cand := wantByDir["100423600"]; !strings.Contains(cand.Reason, "，") {
		t.Fatalf("双信号目录理由应由两个条件以逗号拼接: %q", cand.Reason)
	}
	if single := byDir["结构群目录"]; single.WorkNo != "" || !single.SingleGroup {
		t.Fatalf("单一群键目录应只携带群键条件: %+v", single)
	}
	// 成员清单形态：目录直接子文件全部（子目录文件不计），序号原文与元素数组
	// 现算（b_01 序号 01）
	workNo := byDir["100423498"]
	if len(workNo.Members) != 4 || workNo.Size != 4 {
		t.Fatalf("作品号目录成员应=4 个直接子文件（子目录文件不计）: size=%d members=%+v", workNo.Size, workNo.Members)
	}
	var seqText string
	var hasSeqEl bool
	for _, mem := range workNo.Members {
		if mem.RelPath != "100423498/b_01.jpg" {
			continue
		}
		seqText = mem.SeqText
		for _, el := range mem.Elements {
			if el.Kind == elKindSeq {
				hasSeqEl = true
			}
		}
	}
	if seqText != "01" || !hasSeqEl {
		t.Fatalf("目录分组成员元素应现算（b_01 序号 01），实际 seqText=%q hasSeqEl=%v", seqText, hasSeqEl)
	}
}

// TestDirGroupSectionIndependentFromGroupCards 独立分节锚：分节产出不
// 触碰序列群卡分节与文件行记录；候选墙目录来源卡由分节逐条投影（目录卡数=
// 分节数），群卡候选仍只由 Groups 产出——群卡消费面（读 Groups 的选择域/集序/
// 命中统计）不受影响。
func TestDirGroupSectionIndependentFromGroupCards(t *testing.T) {
	snap := dirGroupSectionFixture()
	groupsBefore := append([]SnapGroup(nil), snap.Groups...)
	filesBefore := append([]SnapFile(nil), snap.Files...)
	if len(snap.DirGroups) != 0 {
		t.Fatalf("候选装配前不应预置目录分组: %+v", snap.DirGroups)
	}
	buildSnapshotCandidates(snap, nil, aggregateInput{})
	if !reflect.DeepEqual(snap.Groups, groupsBefore) {
		t.Fatalf("目录分组产出改动了序列群卡分节：\nbefore: %+v\nafter:  %+v", groupsBefore, snap.Groups)
	}
	if !reflect.DeepEqual(snap.Files, filesBefore) {
		t.Fatalf("目录分组产出改动了文件行记录")
	}
	if len(snap.DirGroups) != 5 {
		t.Fatalf("快照目录分组分节数 = %d，期望 5", len(snap.DirGroups))
	}
	var groupCands, dirCands int
	for _, c := range snap.Candidates {
		switch c.Source {
		case candSourceGroup:
			groupCands++
		case candSourceDir:
			dirCands++
		default:
			t.Fatalf("候选墙出现未知来源 %q: %+v", c.Source, c)
		}
	}
	if groupCands != len(snap.Groups) || dirCands != len(snap.DirGroups) {
		t.Fatalf("候选墙来源分账不符：群卡候选=%d（群卡 %d）目录候选=%d（分节 %d）",
			groupCands, len(snap.Groups), dirCands, len(snap.DirGroups))
	}
	// 分节投影与群卡互不混入：目录分组键不出现在序列群卡分节
	dirKeys := make(map[string]bool)
	for _, g := range snap.DirGroups {
		dirKeys[g.Dir+"\x00"+g.Key] = true
	}
	for _, g := range snap.Groups {
		if dirKeys[g.DirRelKey+"\x00"+g.Key] {
			t.Fatalf("序列群卡分节混入了目录分组事实: %+v", g)
		}
	}
}

// TestDirGroupSectionJSONRoundTrip 分节持久化锚：目录分组随快照 JSON 往返保真
// （探测条件/键/成员清单/键元素池）；旧快照（无 dirGroups 字段）零值降级为空
// 分节（识别重跑重建）。
func TestDirGroupSectionJSONRoundTrip(t *testing.T) {
	snap := dirGroupSectionFixture()
	buildSnapshotCandidates(snap, nil, aggregateInput{})
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("快照序列化失败: %v", err)
	}
	var back RecognitionSnapshot
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("快照反序列化失败: %v", err)
	}
	if !reflect.DeepEqual(back.DirGroups, snap.DirGroups) {
		t.Fatalf("目录分组分节未随快照往返保真：\ngot:  %+v\nwant: %+v", back.DirGroups, snap.DirGroups)
	}
	if !reflect.DeepEqual(back.Candidates, snap.Candidates) {
		t.Fatalf("候选墙未随快照往返保真")
	}
	legacy := []byte(`{"schemaVersion":3,"dirs":[{"relKey":"d","name":"d","depth":1}],"files":[{"relPath":"d/a.jpg","fileName":"a.jpg","dirRelKey":"d","state":"plain"}]}`)
	var old RecognitionSnapshot
	if err := json.Unmarshal(legacy, &old); err != nil {
		t.Fatalf("旧快照读入失败: %v", err)
	}
	if len(old.DirGroups) != 0 {
		t.Fatalf("旧快照无目录分节时应零值降级，实际: %+v", old.DirGroups)
	}
}

// TestDirGroupSectionNotInOverviewWire overview 应答投影锚：目录分组分节为重载荷
// 成员清单，overview 整答不携带（与文件明细同口径，避免每页重复整节导致 12MB
// 页界与装载增量风险）；全量投影 panoramaViewOf 保留分节（服务端读取面）。
func TestDirGroupSectionNotInOverviewWire(t *testing.T) {
	snap := dirGroupSectionFixture()
	buildSnapshotCandidates(snap, nil, aggregateInput{})
	if len(snap.DirGroups) == 0 {
		t.Fatalf("夹具缺损：快照应含目录分组分节")
	}
	if view := panoramaOverviewViewOf(snap); view.DirGroups != nil {
		t.Fatalf("overview 应答投影不应携带目录分组分节：%+v", view.DirGroups)
	}
	if view := panoramaViewOf(snap); len(view.DirGroups) != len(snap.DirGroups) {
		t.Fatalf("全量投影应保留目录分组分节（服务端读取面）：%d != %d", len(view.DirGroups), len(snap.DirGroups))
	}
}

// TestDirGroupSectionInRecognitionSnapshot 识别链落节锚：全量识别产出的快照含
// 目录分组分节（探测条件/键/成员与候选墙目录来源卡一致）——分节生产挂在识别
// 装配内，非仅单元入口可达。
func TestDirGroupSectionInRecognitionSnapshot(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	writeTestFile(t, root+"/100423498/a.jpg", "a")
	writeTestFile(t, root+"/100423498/b_01.jpg", "b")
	writeTestFile(t, root+"/noise/x.jpg", "x")
	writeTestFile(t, root+"/noise/y.jpg", "y")
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
	if len(snap.DirGroups) != 1 {
		t.Fatalf("快照目录分组分节 = %d 条，期望 1（作品号目录；无信号目录不出）：%+v", len(snap.DirGroups), snap.DirGroups)
	}
	g := snap.DirGroups[0]
	if g.Dir != "100423498" || g.Key != "100423498" || g.WorkNo != "100423498" || g.SingleGroup || g.Size != 2 {
		t.Fatalf("目录分组分节字段不符: %+v", g)
	}
	if len(g.Members) != 2 || g.Members[0].RelPath != "100423498/a.jpg" || g.Members[1].RelPath != "100423498/b_01.jpg" {
		t.Fatalf("目录分组成员清单不符（目录直接子文件全部、快照文件序）: %+v", g.Members)
	}
	// 候选墙目录来源卡与分节逐字段一致（分节是唯一事实源）
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
	if card.Key != g.Key || card.Dir != g.Dir || card.Size != g.Size ||
		!reflect.DeepEqual(card.Members, g.Members) || !reflect.DeepEqual(card.KeyElements, g.KeyElements) {
		t.Fatalf("目录来源卡与分节字段不一致：card=%+v group=%+v", *card, g)
	}
	if !strings.Contains(card.Reason, "目录名含作品号 100423498") {
		t.Fatalf("目录来源卡理由应按探测条件现拼: %q", card.Reason)
	}
}
