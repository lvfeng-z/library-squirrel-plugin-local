package main

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/lvfeng-z/library-squirrel-sdk/contract"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// 存量档案的回放锚与并用形态的消费面回归锚（成员化语义退役后的行为定锚）：
//
//   - 既有成功配对形态（映射单用的「一 txt 一作品」单作品组、并用成员）：旧档
//     原文（含 metadataFile.pairing / aggregate.contentBindings 键）经档案载入
//     归一后重识别——结论、快照、投影三向与成员化退役前的数据形态（说明成员
//     按归并目标键携带归属、扇出键集字段不存在）经同一批消费面回放全等，
//     差异零噪音；
//   - 既有守卫拒绝形态（分组内多作品——旧指向集唯一守卫整组拒绝的形态）：
//     差异=预期行为——目标作品出现内容声明、说明文件不再独立成件；测试对
//     新旧两形态各自落锚（差异即验收，勿当回归修复）；
//   - 并用形态消费面零改动：绑定行成员仍由聚合段赋聚合作品键（归进作品、
//     无轨）、哈希面剔除、创建载荷全收、投影并入照常；聚合侧守卫（必含轨
//     每组恰一）不动——歧义组成员维持独立作品。

// fanoutReplayFixture 按相对路径→内容构造磁盘夹具（相对路径排序=求值面文件
// 序，确定序），返回扫描条目清单。
func fanoutReplayFixture(t *testing.T, root string, contents map[string]string) []FileEntry {
	t.Helper()
	rels := make([]string, 0, len(contents))
	for rel := range contents {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	entries := make([]FileEntry, 0, len(rels))
	for _, rel := range rels {
		full := root + "/" + rel
		writeTestFile(t, full, contents[rel])
		entries = append(entries, FileEntry{FullPath: full, RelPath: rel})
	}
	return entries
}

// fanoutReplayRecognize 识别全链驱动（回放锚共用的求值面）：合并求值 → 聚合
// 段与扇出归属段 → 内容采集命中集 → 片段注入 → 第二相整段取值产声明 → 快照
// 落档。资源类型投影与库内谓词索引取测试形态（纯求值、无 IO 面）。
func fanoutReplayRecognize(t *testing.T, entries []FileEntry, rules []RuleEntry) (*RecognitionSnapshot, []FileConclusion) {
	t.Helper()
	files := buildScanFiles(entries)
	conclusions := MergedRecognition(files, rules, nil, nil)
	runAggregationOnConclusions(conclusions, files, files, rules, nil, testAggTypeIndex(), nil)
	hits := companionContentHitSet(rules, conclusions)
	samples, _ := collectCompanionContentSet(hits, entries)
	applyCompanionContent(files, samples)
	applyCompanionContentDeclarations(conclusions, files, rules, nil)
	assignIdx := newAssignmentDirIndex(nil, nil)
	snap := &RecognitionSnapshot{}
	for i := range conclusions {
		snap.Files = append(snap.Files, snapFileOf(&conclusions[i], nil, assignIdx))
	}
	return snap, conclusions
}

// fanoutLegacyShapeSnapshot 成员化退役前的快照数据形态：pairedKeys 给出的
// 说明成员按归并目标键携带归属（退役前配对成立的成员）、全库无扇出键集
// （字段尚不存在），其余字段原样保留——供新旧数据形态经同一批消费面的
// 同形回放比对（快照行切片浅拷贝：消费面只读不改写输入行）。
func fanoutLegacyShapeSnapshot(snap *RecognitionSnapshot, pairedKeys map[string]string) *RecognitionSnapshot {
	legacy := &RecognitionSnapshot{Files: append([]SnapFile(nil), snap.Files...)}
	for i := range legacy.Files {
		f := &legacy.Files[i]
		f.FanoutWorkKeys = nil
		if key, ok := pairedKeys[f.RelPath]; ok {
			f.AggWorkKey = key
		}
	}
	return legacy
}

// fanoutConsumerFingerprint 消费指纹（回放全等的比对面）：选择域作品序列
// （作品键|成员清单|说明计数|可选性/呈现态）与内容声明归集的投影/载荷两
// 口径（作品键→「维度=值@来源成员文件名」序列）。
type fanoutConsumerFingerprint struct {
	works   []string
	proj    map[string][]string
	payload map[string][]string
}

// fanoutConsumerFingerprintOf 从快照推导消费指纹：选择域推导、投影口径内容
// 声明归集、创建载荷口径归集三消费面各落一维。
func fanoutConsumerFingerprintOf(snap *RecognitionSnapshot) fanoutConsumerFingerprint {
	domain := DeriveWorkDomain(snap, nil)
	fp := fanoutConsumerFingerprint{
		proj:    fanoutDeclsFingerprint(workContentDeclsByWork(snap)),
		payload: fanoutDeclsFingerprint(workPayloadContentDeclsByWork(snap)),
	}
	for _, w := range domain.Works {
		fp.works = append(fp.works, fmt.Sprintf("%s|%s|说明%d|%v/%s",
			w.WorkID, strings.Join(w.Files, ","), w.CompanionCount, w.Selectable, w.State))
	}
	return fp
}

// fanoutDeclsFingerprint 内容声明归集表的指纹形态（nil 归一为空表——两侧
// 同函数推导，比对面一致）。
func fanoutDeclsFingerprint(m map[string][]SnapDecl) map[string][]string {
	out := make(map[string][]string, len(m))
	for key, decls := range m {
		items := make([]string, 0, len(decls))
		for _, d := range decls {
			items = append(items, fmt.Sprintf("%s=%s@%s", d.Dimension, d.Value, d.OriginName))
		}
		out[key] = items
	}
	return out
}

// —— 回放锚（一）：既有成功配对形态 ——

// fanoutReplaySingleWorkGroupJSON 旧档原文：映射单用规则携带旧档配对键
// （metadataFile.pairing·标识分组），命中「一 txt 一作品」的单作品组——退役前
// 指向集唯一守卫放行的成功配对面。
const fanoutReplaySingleWorkGroupJSON = `{"schemaVersion":5,"rules":[
{"id":"replay-single","description":"回放锚：单作品组一 txt 一作品","source":"manual","enabled":true,
 "conditions":[
  {"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}_{序号}"},"attach":[{"kind":"fileExt","value":".png"}]}},
  {"connector":"and","condition":{"target":"file","format":{"mode":"template","tpl":"{作品}_{序号}"},"attach":[{"kind":"fileExt","value":".txt"}]}}],
 "metadataFile":{"pairing":{"mode":"identity","identity":[{"row":1,"capture":"work"},{"row":1,"capture":"seq"}]},
  "contentBindings":[{"row":2,"dimension":"description"}]}}]}`

// TestFanoutReplayAnchorSingleWorkGroup 单作品组形态的回放锚：旧档经载入归一
// 后重识别——结论向（说明文件=元数据源：零归属键、说明成员标记、扇出键集=
// 组内唯一作品；目标文件独立成件不获归属）、快照/投影向（与退役前数据形态
// 的消费指纹全等：作品合并、说明计数、内容声明到达两口径逐位一致——差异
// 零噪音）。
func TestFanoutReplayAnchorSingleWorkGroup(t *testing.T) {
	profile, errs := ParseProfile([]byte(fanoutReplaySingleWorkGroupJSON))
	if len(errs) != 0 {
		t.Fatalf("旧档单作品组档案应零拒绝载入（配对键归一）: %v", errs)
	}
	root := t.TempDir()
	entries := fanoutReplayFixture(t, root, map[string]string{
		"无题/无题_01.png": "png",
		"无题/无题_01.txt": "无题_01 的说明内容",
	})
	snap, conclusions := fanoutReplayRecognize(t, entries, profile.Rules)

	// 结论向：说明文件=元数据源（零归属键+说明成员标记+内容扇出到组内唯一
	// 作品），归并目标与退役前同值（该作品的路径键）；目标文件独立成件。
	txt := aggConclusionOf(t, conclusions, "无题/无题_01.txt")
	if txt.AggWorkKey != "" || txt.AggTrackRole != "" || txt.AggWorkType != "" || !txt.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(txt), []string{"f:无题/无题_01.png"}) {
		t.Fatalf("说明文件应为元数据源且扇出目标与退役前归并目标同键: key=%q role=%q type=%q companion=%v fanout=%v",
			txt.AggWorkKey, txt.AggTrackRole, txt.AggWorkType, txt.AggCompanion, txt.FanoutWorkKeys)
	}
	if !conclusionHasDecl(txt, DimDescription, "无题_01 的说明内容", SourceContent) {
		t.Fatalf("说明文件第二相应产整段描述声明: %+v", txt.Final)
	}
	png := aggConclusionOf(t, conclusions, "无题/无题_01.png")
	if png.AggWorkKey != "" || png.AggCompanion || len(fanoutKeysOf(png)) != 0 {
		t.Fatalf("目标文件应独立成件（扇出不并作品）: key=%q companion=%v fanout=%v",
			png.AggWorkKey, png.AggCompanion, png.FanoutWorkKeys)
	}

	// 快照/投影向：退役前数据形态（说明成员按归并目标键携带归属、无扇出键
	// 集）与本轮识别快照经三消费面的指纹全等。
	legacy := fanoutLegacyShapeSnapshot(snap, map[string]string{"无题/无题_01.txt": "f:无题/无题_01.png"})
	got, want := fanoutConsumerFingerprintOf(snap), fanoutConsumerFingerprintOf(legacy)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("单作品组形态的回放指纹应全等（差异零噪音）:\n新形态=%+v\n退役前形态=%+v", got, want)
	}
	if len(got.works) != 1 || !strings.Contains(got.works[0], "f:无题/无题_01.png|无题/无题_01.png,无题/无题_01.txt|说明1|") {
		t.Fatalf("回放指纹应含恰一件双成员作品（目标首位、说明计数 1）: %+v", got.works)
	}
	if len(got.proj["f:无题/无题_01.png"]) != 1 || len(got.payload["f:无题/无题_01.png"]) != 1 {
		t.Fatalf("内容声明应到达唯一作品（投影与载荷两口径各一份）: %+v / %+v", got.proj, got.payload)
	}
	// 投影并入：目标文件详情并入说明成员描述（来源标注=说明文件名）。
	contentDecls := workContentDeclsByWork(snap)
	merged := projectWorkMergedFile(snapFileOfRow(snap, "无题/无题_01.png"), contentDecls)
	if !finalHasDecl(&merged, DimDescription, "无题_01 的说明内容", SourceContent) {
		t.Fatalf("目标文件投影应并入说明成员描述: %+v", merged.Final)
	}
}

// snapFileOfRow 按相对路径取快照文件行（缺行即测试失败）。
func snapFileOfRow(snap *RecognitionSnapshot, relPath string) SnapFile {
	for i := range snap.Files {
		if snap.Files[i].RelPath == relPath {
			return snap.Files[i]
		}
	}
	return SnapFile{}
}

// fanoutReplayCombinedMemberJSON 旧档原文：并用规则（聚合产出）携带旧档绑定
// 键（aggregate.contentBindings·分组绑定缺省——退役前并用成员的落档形态）。
const fanoutReplayCombinedMemberJSON = `{"schemaVersion":5,"rules":[
{"id":"replay-combined","description":"回放锚：并用成员视频+字幕+说明","source":"manual","enabled":true,"category":"aggregate",
 "conditions":[
  {"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}"},"attach":[{"kind":"fileExt","value":".mp4"}]}},
  {"connector":"and","condition":{"target":"file","format":{"mode":"template","tpl":"{作品}.{任意}"},"attach":[{"kind":"fileExt","value":".srt"}]}},
  {"connector":"and","condition":{"target":"file","format":{"mode":"template","tpl":"{作品}"},"attach":[{"kind":"fileExt","value":".nfo"}]}}],
 "identity":[{"row":1,"capture":"work"}],
 "aggregate":{"workType":"video","tracks":[{"row":1,"role":"videoMain"},{"row":2,"role":"subtitle"}],
  "contentBindings":[{"row":3,"dimension":"description"}]}}]}`

// TestFanoutReplayAnchorCombinedMember 并用成员形态的回放锚：旧档绑定键归一
// 后重识别——结论向与退役前逐字段同值（绑定行成员仍由聚合段赋聚合作品键、
// 无轨、说明成员标记、作品类型随作品——退役前并用桥接同值），扇出键集为
// 纯增量数据位；快照/投影向与退役前数据形态（无扇出键集）的消费指纹全等。
func TestFanoutReplayAnchorCombinedMember(t *testing.T) {
	profile, errs := ParseProfile([]byte(fanoutReplayCombinedMemberJSON))
	if len(errs) != 0 {
		t.Fatalf("旧档并用档案应零拒绝载入（绑定键归一）: %v", errs)
	}
	root := t.TempDir()
	entries := fanoutReplayFixture(t, root, map[string]string{
		"d/ep01.mp4":    "mp4",
		"d/ep01.zh.srt": "srt",
		"d/ep01.nfo":    "ep01 的简介整段",
	})
	snap, conclusions := fanoutReplayRecognize(t, entries, profile.Rules)

	// 结论向：三成员归并进一件聚合作品（主导=必含轨命中、字幕落轨、说明
	// 成员无轨），绑定行成员照常由聚合段赋聚合作品键——退役前桥接同值；
	// 扇出键集=该聚合作品键（同规则并用的缺省分组=聚合成作品所绑分组）。
	video := aggConclusionOf(t, conclusions, "d/ep01.mp4")
	if video.AggWorkKey != "f:d/ep01.mp4" || video.AggTrackRole != sdkdto.StoreRoleVideoMain || video.AggCompanion {
		t.Fatalf("视频主导的归属不符: key=%q role=%q companion=%v",
			video.AggWorkKey, video.AggTrackRole, video.AggCompanion)
	}
	sub := aggConclusionOf(t, conclusions, "d/ep01.zh.srt")
	if sub.AggWorkKey != "f:d/ep01.mp4" || sub.AggTrackRole != contract.StoreTypeSubtitle || sub.AggCompanion {
		t.Fatalf("字幕轨成员的归属不符: key=%q role=%q companion=%v",
			sub.AggWorkKey, sub.AggTrackRole, sub.AggCompanion)
	}
	nfo := aggConclusionOf(t, conclusions, "d/ep01.nfo")
	if nfo.AggWorkKey != "f:d/ep01.mp4" || nfo.AggTrackRole != "" || nfo.AggWorkType != sdkdto.ResourceTypeVideo ||
		!nfo.AggCompanion || !reflect.DeepEqual(fanoutKeysOf(nfo), []string{"f:d/ep01.mp4"}) {
		t.Fatalf("说明成员应照常归进聚合作品（聚合段赋键）且内容扇出: key=%q role=%q type=%q companion=%v fanout=%v",
			nfo.AggWorkKey, nfo.AggTrackRole, nfo.AggWorkType, nfo.AggCompanion, nfo.FanoutWorkKeys)
	}
	if !conclusionHasDecl(nfo, DimDescription, "ep01 的简介整段", SourceContent) {
		t.Fatalf("说明成员第二相应产整段描述声明: %+v", nfo.Final)
	}

	// 快照/投影向：退役前数据形态（同字段结论、无扇出键集）的消费指纹全等。
	legacy := fanoutLegacyShapeSnapshot(snap, nil)
	got, want := fanoutConsumerFingerprintOf(snap), fanoutConsumerFingerprintOf(legacy)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("并用成员形态的回放指纹应全等（差异零噪音）:\n新形态=%+v\n退役前形态=%+v", got, want)
	}
	if len(got.works) != 1 || !strings.Contains(got.works[0], "f:d/ep01.mp4|d/ep01.mp4,d/ep01.nfo,d/ep01.zh.srt|说明1|") {
		t.Fatalf("回放指纹应含恰一件三成员聚合作品（主导首位、说明计数 1）: %+v", got.works)
	}
}

// —— 回放锚（二）：既有守卫拒绝形态（差异=预期行为）——

// fanoutReplayMultiWorkGroupJSON 旧档原文：映射单用规则、分组键取 {作品}
// 前缀——同前缀两张 png 与一份总说明 txt 编入同组（退役前指向集唯一守卫
// 整组拒绝的形态：主导跨多作品）。
const fanoutReplayMultiWorkGroupJSON = `{"schemaVersion":5,"rules":[
{"id":"replay-multi","description":"回放锚：分组内多作品（旧守卫拒绝形态）","source":"manual","enabled":true,
 "conditions":[
  {"condition":{"target":"file","format":{"mode":"template","tpl":"{作品}_{序号}"},"attach":[{"kind":"fileExt","value":".png"}]}},
  {"connector":"and","condition":{"target":"file","format":{"mode":"template","tpl":"{作品}"},"attach":[{"kind":"fileExt","value":".txt"}]}}],
 "metadataFile":{"pairing":{"mode":"identity","identity":[{"row":1,"capture":"work"}]},
  "contentBindings":[{"row":2,"dimension":"description"}]}}]}`

// TestFanoutReplayAnchorGuardRejectedMultiWorkGroup 多作品组形态的两分锚：
// 退役前该形态被守卫整组拒绝——说明文件维持独立作品（自带描述声明、可被
// 误导入为作品）、两张 png 不获任何内容；守卫随成员化语义退役后差异=预期
// 行为——说明文件为元数据源（不独立成件、不导入），其内容声明扇出给组内
// 两件作品（各带一份、详情可见）。测试对两形态各自落锚，差异即验收。
func TestFanoutReplayAnchorGuardRejectedMultiWorkGroup(t *testing.T) {
	profile, errs := ParseProfile([]byte(fanoutReplayMultiWorkGroupJSON))
	if len(errs) != 0 {
		t.Fatalf("旧档多作品组档案应零拒绝载入（配对键归一）: %v", errs)
	}
	root := t.TempDir()
	entries := fanoutReplayFixture(t, root, map[string]string{
		"无题/a_01.png": "png",
		"无题/a_02.png": "png",
		"无题/a.txt":    "总说明整段",
	})
	snap, conclusions := fanoutReplayRecognize(t, entries, profile.Rules)

	// 结论向：两张 png 各自成件（扇出不并作品），说明文件=元数据源（零归属
	// 键、说明成员标记、扇出键集=组内两件作品）。
	for _, png := range []string{"无题/a_01.png", "无题/a_02.png"} {
		got := aggConclusionOf(t, conclusions, png)
		if got.AggWorkKey != "" || got.AggCompanion || len(fanoutKeysOf(got)) != 0 {
			t.Fatalf("目标文件 %s 应独立成件: key=%q companion=%v fanout=%v",
				png, got.AggWorkKey, got.AggCompanion, got.FanoutWorkKeys)
		}
	}
	txt := aggConclusionOf(t, conclusions, "无题/a.txt")
	if txt.AggWorkKey != "" || !txt.AggCompanion ||
		!reflect.DeepEqual(fanoutKeysOf(txt), []string{"f:无题/a_01.png", "f:无题/a_02.png"}) {
		t.Fatalf("说明文件应为元数据源且内容扇出到组内两件作品: key=%q companion=%v fanout=%v",
			txt.AggWorkKey, txt.AggCompanion, txt.FanoutWorkKeys)
	}

	// 投影向（新形态）：说明文件不再独立成件——并入两件目标作品（各计说明
	// 一次），内容声明到达两键（投影与载荷两口径），目标详情并入描述。
	got := fanoutConsumerFingerprintOf(snap)
	if len(got.works) != 2 {
		t.Fatalf("新形态应恰两件目标作品（说明文件不独立成件）: %+v", got.works)
	}
	for _, work := range got.works {
		if !strings.Contains(work, ",无题/a.txt|说明1|") {
			t.Fatalf("每件目标作品应并入说明文件且各计说明一次: %+v", got.works)
		}
	}
	for _, key := range []string{"f:无题/a_01.png", "f:无题/a_02.png"} {
		if len(got.proj[key]) != 1 || len(got.payload[key]) != 1 {
			t.Fatalf("内容声明应到达每件目标作品（两口径各一份）: %+v / %+v", got.proj, got.payload)
		}
		merged := projectWorkMergedFile(snapFileOfRow(snap, strings.TrimPrefix(key, "f:")), workContentDeclsByWork(snap))
		if !finalHasDecl(&merged, DimDescription, "总说明整段", SourceContent) {
			t.Fatalf("目标作品 %s 的详情应并入说明文件描述（差异即验收）: %+v", key, merged.Final)
		}
	}

	// 退役前形态基线（守卫整组拒绝：说明文件维持独立作品、无归属无标记，
	// 两张 png 零内容到达）——与新形态的两点差异即预期行为。
	legacy := fanoutLegacyShapeSnapshot(snap, nil)
	for i := range legacy.Files {
		if legacy.Files[i].RelPath == "无题/a.txt" {
			legacy.Files[i].AggCompanion = false
		}
	}
	old := fanoutConsumerFingerprintOf(legacy)
	if len(old.works) != 3 {
		t.Fatalf("退役前形态应为三件独立作品（说明文件独立成件）: %+v", old.works)
	}
	for _, work := range old.works {
		if strings.Contains(work, "说明1") {
			t.Fatalf("退役前形态不应有说明并入: %+v", old.works)
		}
	}
	if len(old.proj) != 0 || len(old.payload) != 0 {
		t.Fatalf("退役前形态内容声明应零到达（守卫拒绝）: %+v / %+v", old.proj, old.payload)
	}
}

// —— 并用形态消费面零改动锚 ——

// TestFanoutCombinedConsumerSurfaceAnchor 并用形态（聚合+元数据文件两产出
// 并存）的消费面回归锚：绑定行成员仍由聚合段赋聚合作品键（消费面零改动——
// 选择联动、哈希剔除、载荷收发照常），内容声明经扇出面到达所属作品；主导
// 轨行兼内容绑定的双重身份照常导入（哈希面保留）、自身内容声明只在创建
// 载荷口径收齐（执行期重算不重读内容）；聚合侧守卫不动——必含轨非恰一的
// 歧义组不成立，成员维持独立作品、扇出面在守卫拒绝后的求值面上照常工作。
func TestFanoutCombinedConsumerSurfaceAnchor(t *testing.T) {
	t.Run("视频+字幕+说明文件（并用缺省分组）", func(t *testing.T) {
		rule := RuleEntry{
			ID: "combined-video", Description: "测试：并用视频+字幕+说明", Source: RuleSourceManual, Enabled: true,
			Category: RuleCategoryAggregate,
			Conditions: []CondxRow{
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".mp4"}}}},
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}.{任意}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".srt"}}}},
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".nfo"}}}},
			},
			Identity: []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{WorkType: sdkdto.ResourceTypeVideo, Tracks: []TrackAssign{
				{Row: 1, Role: sdkdto.StoreRoleVideoMain},
				{Row: 2, Role: contract.StoreTypeSubtitle},
			}},
			// 分组绑定缺省=聚合成作品所绑分组（求值期取用、档案不物化）。
			MetadataFile: &MetadataFileProduction{
				ContentBindings: []ContentBinding{{Row: 3, Dimension: DimDescription}},
			},
		}
		root := t.TempDir()
		entries := fanoutReplayFixture(t, root, map[string]string{
			"d/ep01.mp4":    "mp4",
			"d/ep01.zh.srt": "srt",
			"d/ep01.nfo":    "ep01 的简介整段",
		})
		snap, conclusions := fanoutReplayRecognize(t, entries, []RuleEntry{rule})

		// 结论：绑定行成员仍由聚合段赋聚合作品键（归进作品、无轨、说明
		// 成员标记），扇出键集=该作品键（同规则并用两通道落点一致）。
		nfo := aggConclusionOf(t, conclusions, "d/ep01.nfo")
		if nfo.AggWorkKey != "f:d/ep01.mp4" || nfo.AggTrackRole != "" || !nfo.AggCompanion ||
			!reflect.DeepEqual(fanoutKeysOf(nfo), []string{"f:d/ep01.mp4"}) {
			t.Fatalf("绑定行成员应照常由聚合段赋键且内容扇出: key=%q role=%q companion=%v fanout=%v",
				nfo.AggWorkKey, nfo.AggTrackRole, nfo.AggCompanion, nfo.FanoutWorkKeys)
		}

		// 选择域：一件聚合作品三成员（主导首位）、说明计数 1。
		domain := DeriveWorkDomain(snap, nil)
		if len(domain.Works) != 1 {
			t.Fatalf("应并成一件聚合作品: %+v", domain.Works)
		}
		work := domain.Works[0]
		if work.WorkID != "f:d/ep01.mp4" ||
			!reflect.DeepEqual(work.Files, []string{"d/ep01.mp4", "d/ep01.nfo", "d/ep01.zh.srt"}) ||
			work.CompanionCount != 1 {
			t.Fatalf("聚合作品的成员面应为轨行∪绑定行（主导首位、说明计数 1）: %+v", work)
		}

		// 内容声明：投影口径并入主导详情（来源=说明成员）、载荷口径全收。
		contentDecls := workContentDeclsByWork(snap)
		if len(contentDecls["f:d/ep01.mp4"]) != 1 {
			t.Fatalf("投影口径应收说明成员的内容声明: %+v", contentDecls)
		}
		merged := projectWorkMergedFile(snapFileOfRow(snap, "d/ep01.mp4"), contentDecls)
		if !finalHasDecl(&merged, DimDescription, "ep01 的简介整段", SourceContent) {
			t.Fatalf("主导详情应并入说明成员描述: %+v", merged.Final)
		}
		payload := workPayloadContentDeclsByWork(snap)
		if len(payload["f:d/ep01.mp4"]) != 1 || payload["f:d/ep01.mp4"][0].OriginName != "ep01.nfo" {
			t.Fatalf("载荷口径应收齐说明成员内容声明（携来源文件名）: %+v", payload)
		}

		// 哈希面：说明成员剔除（不哈希、不核对、不导子行），轨道成员全保留。
		fileByPath := make(map[string]*SnapFile, len(snap.Files))
		for i := range snap.Files {
			fileByPath[snap.Files[i].RelPath] = &snap.Files[i]
		}
		hashFace := workHashFaceOf(fileByPath, work)
		if !reflect.DeepEqual(hashFace.Files, []string{"d/ep01.mp4", "d/ep01.zh.srt"}) {
			t.Fatalf("哈希面应只含轨道成员（说明成员剔除）: %+v", hashFace.Files)
		}
	})

	t.Run("图文正文轨双身份（主导轨行兼内容绑定）", func(t *testing.T) {
		rule := RuleEntry{
			ID: "combined-article", Description: "测试：图文正文轨双身份", Source: RuleSourceManual, Enabled: true,
			Category: RuleCategoryAggregate,
			Conditions: []CondxRow{
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".txt"}}}},
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}_{序号}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".jpg"}}}},
			},
			Identity: []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{WorkType: sdkdto.ResourceTypeArticle, Tracks: []TrackAssign{
				{Row: 1, Role: sdkdto.StoreRoleDocument},
				{Row: 2, Role: contract.StoreTypeImage},
			}},
			MetadataFile: &MetadataFileProduction{
				ContentBindings: []ContentBinding{{Row: 1, Dimension: DimDescription}},
			},
		}
		root := t.TempDir()
		entries := fanoutReplayFixture(t, root, map[string]string{
			"t/手记.txt":    "手记正文整段",
			"t/手记_01.jpg": "jpg",
		})
		snap, conclusions := fanoutReplayRecognize(t, entries, []RuleEntry{rule})

		// 结论：主导轨行兼内容绑定=双重身份——照常落轨导入（不带说明成员
		// 标记），自身内容随扇出面承载（键集含其所属作品键）。
		lead := aggConclusionOf(t, conclusions, "t/手记.txt")
		if lead.AggWorkKey != "f:t/手记.txt" || lead.AggTrackRole != sdkdto.StoreRoleDocument || lead.AggCompanion ||
			!reflect.DeepEqual(fanoutKeysOf(lead), []string{"f:t/手记.txt"}) {
			t.Fatalf("双重身份主导应照常落轨且内容随扇出面承载: key=%q role=%q companion=%v fanout=%v",
				lead.AggWorkKey, lead.AggTrackRole, lead.AggCompanion, lead.FanoutWorkKeys)
		}
		img := aggConclusionOf(t, conclusions, "t/手记_01.jpg")
		if img.AggWorkKey != "f:t/手记.txt" || img.AggTrackRole != contract.StoreTypeImage || img.AggCompanion {
			t.Fatalf("图片轨成员的归属不符: key=%q role=%q companion=%v",
				img.AggWorkKey, img.AggTrackRole, img.AggCompanion)
		}

		// 选择域：双轨作品、零说明计数（双身份不是说明成员）。
		domain := DeriveWorkDomain(snap, nil)
		if len(domain.Works) != 1 || len(domain.Works[0].Files) != 2 || domain.Works[0].CompanionCount != 0 {
			t.Fatalf("应为双轨零说明计数作品: %+v", domain.Works)
		}
		work := domain.Works[0]

		// 内容声明：投影口径零归集（主导自身的终选已在主导位呈现，不双收），
		// 载荷口径收齐（执行期单文件重算经载荷复原——重算不重读内容）。
		if proj := workContentDeclsByWork(snap); len(proj) != 0 {
			t.Fatalf("双重身份主导的投影口径不应双收自身声明: %+v", proj)
		}
		payload := workPayloadContentDeclsByWork(snap)
		if len(payload["f:t/手记.txt"]) != 1 || payload["f:t/手记.txt"][0].Value != "手记正文整段" {
			t.Fatalf("载荷口径应收齐主导自身的内容声明: %+v", payload)
		}

		// 哈希面：双身份照常导入（两轨全保留）。
		fileByPath := make(map[string]*SnapFile, len(snap.Files))
		for i := range snap.Files {
			fileByPath[snap.Files[i].RelPath] = &snap.Files[i]
		}
		hashFace := workHashFaceOf(fileByPath, work)
		if !reflect.DeepEqual(hashFace.Files, []string{"t/手记.txt", "t/手记_01.jpg"}) {
			t.Fatalf("双身份轨行应照常保留在哈希面: %+v", hashFace.Files)
		}
	})

	t.Run("聚合侧守卫不动（必含轨非恰一歧义组不成立）", func(t *testing.T) {
		rule := RuleEntry{
			ID: "combined-guard", Description: "测试：必含轨歧义组守卫", Source: RuleSourceManual, Enabled: true,
			Category: RuleCategoryAggregate,
			Conditions: []CondxRow{
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}_{任意}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".mp4"}}}},
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}.{任意}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".srt"}}}},
				{Condition: &Condx{Target: CondxTargetFile, Format: condxTpl("{作品}"),
					Attach: []CondxAttach{{Kind: CondxAttachFileExt, Value: ".nfo"}}}},
			},
			Identity: []IdentityPart{{Row: 1, Capture: "work"}},
			Aggregate: &AggregateProduction{WorkType: sdkdto.ResourceTypeVideo, Tracks: []TrackAssign{
				{Row: 1, Role: sdkdto.StoreRoleVideoMain},
				{Row: 2, Role: contract.StoreTypeSubtitle},
			}},
			MetadataFile: &MetadataFileProduction{
				ContentBindings: []ContentBinding{{Row: 3, Dimension: DimDescription}},
			},
		}
		root := t.TempDir()
		entries := fanoutReplayFixture(t, root, map[string]string{
			"g/a_1.mp4":  "mp4",
			"g/a_2.mp4":  "mp4",
			"g/a.zh.srt": "srt",
			"g/a.nfo":    "a 组总说明",
		})
		snap, conclusions := fanoutReplayRecognize(t, entries, []RuleEntry{rule})

		// 守卫不动：同标识两单位命中必含轨（两个视频文件）→歧义、该组不
		// 成立——全部成员维持独立作品（零归属键、零轨道角色）。
		for _, rel := range []string{"g/a_1.mp4", "g/a_2.mp4", "g/a.zh.srt"} {
			got := aggConclusionOf(t, conclusions, rel)
			if got.AggWorkKey != "" || got.AggTrackRole != "" || got.AggCompanion {
				t.Fatalf("歧义组成员 %s 应维持独立作品（守卫不动）: key=%q role=%q companion=%v",
					rel, got.AggWorkKey, got.AggTrackRole, got.AggCompanion)
			}
		}
		// 扇出面在守卫拒绝后的求值面上照常工作：说明文件=元数据源，内容
		// 扇出到组内三件独立作品。
		nfo := aggConclusionOf(t, conclusions, "g/a.nfo")
		want := []string{"f:g/a.zh.srt", "f:g/a_1.mp4", "f:g/a_2.mp4"}
		if nfo.AggWorkKey != "" || !nfo.AggCompanion || !reflect.DeepEqual(fanoutKeysOf(nfo), want) {
			t.Fatalf("守卫拒绝组的说明文件应为元数据源且扇出到组内全部独立作品: key=%q companion=%v fanout=%v",
				nfo.AggWorkKey, nfo.AggCompanion, nfo.FanoutWorkKeys)
		}
		// 消费面：三件独立作品各并入说明文件（各计说明一次）。
		domain := DeriveWorkDomain(snap, nil)
		if len(domain.Works) != 3 {
			t.Fatalf("守卫拒绝组应维持三件独立作品: %+v", domain.Works)
		}
		for _, w := range domain.Works {
			if w.CompanionCount != 1 {
				t.Fatalf("每件独立作品应各计说明一次: %+v", w)
			}
		}
	})
}
