package main

// 组合规则引用门在「目录层含义回放」通道的回归锚（AB 规则组合引用缺陷修复）：
// 开发环境实测发现组合规则（refs 门 + 自身 {集名} 目录行）在识别全景目录树上
// 令所有目录都呈现作品集含义——回放通道逐条孤立求值、未过门段（文件级流水线
// 门段正常）。本文件用开发环境真实档案原文锚定两侧：文件级流水线门控矩阵 +
// 目录层含义回放门控矩阵（修复后回放与提取段同语义过门，祖先链取真实目录名）。
import (
	"testing"
)

// realComboProfile 开发环境实测档案原文（2026-10-29，含一条序列定群规则、
// 一条 Artist 壳作者成员规则、一条引用它的 {集名} 组合规则）。
const realComboProfile = `{"schemaVersion":4,"rules":[
{"id":"qna-seq-たじ-57719012-ボテ詰め合わせ-1793038910903","description":"【作品集】序列型作品集","source":"qna","enabled":true,"conditions":[{"condition":{"target":"seq","format":{"mode":"template","tpl":"[{作者}]_[{作品号}]_{标题}_[{序号}]"},"attach":[{"kind":"seqMinLength","minCount":2}]}},{"connector":"or","condition":{"target":"seq","format":{"mode":"template","tpl":"[{作者}] [{作品号}] [{标题}]{序号}"},"attach":[{"kind":"seqMinLength","minCount":2}]}}],"extractions":[],"synthesis":{"dimPolicies":[],"roles":[]}},
{"id":"manual-作者-artist-作者结构-1793237147531","description":"【作者】Artist+作者结构","source":"manual","enabled":true,"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"Artist[{作者}]"}}},{"connector":"or","condition":{"target":"dir","format":{"mode":"template","tpl":"Artist {作者}"}}}],"extractions":[{"dimension":"author","from":"capture","captureName":"author","constant":"","roleName":""},{"dimension":"author","from":"capture","captureName":"author","row":2,"constant":"","roleName":""}],"synthesis":{"dimPolicies":[],"roles":[]}},
{"id":"manual-作品集-作者-目录-结构的-1793237671031","description":"【作品集】作者/目录 结构的","source":"manual","enabled":true,"refs":["manual-作者-artist-作者结构-1793237147531"],"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"{集名}"}}}],"extractions":[{"dimension":"collection","from":"capture","captureName":"setname","constant":"","roleName":""}],"synthesis":{"dimPolicies":[],"roles":[]}}]}`

// TestRealProfileComboGatePipeline 真实档案的文件级流水线门控基线：Artist 壳
// 下子目录识别为作品集、无 Artist 壳祖先的目录零作品集、Artist 壳直属文件零
// 作品集（之下不含其身：集名锚定不得落在被引规则命中的目录身上——2026-10-29
// 用户拍板收紧）。
func TestRealProfileComboGatePipeline(t *testing.T) {
	profile, errs := ParseProfile([]byte(realComboProfile))
	if profile == nil {
		t.Fatalf("档案不可载入: %v", errs)
	}
	for _, err := range errs {
		t.Logf("载入告警: %v", err)
	}
	files := []ScanFile{
		{FileName: "01.png", DirNames: []string{"Artist[foo]", "SetA"}, RelPath: "Artist[foo]/SetA/01.png"},
		{FileName: "02.png", DirNames: []string{"OtherDir"}, RelPath: "OtherDir/02.png"},
		{FileName: "03.png", DirNames: []string{"Artist[foo]"}, RelPath: "Artist[foo]/03.png"},
	}
	decisions := RunPipeline(files, profile.Rules, nil)
	// Artist 壳下子目录：应识别为作品集。
	if got := matchedCollectionOf(decisions[0]); got != "SetA" {
		t.Errorf("Artist[foo]/SetA 应识别作品集 SetA，实际 %q", got)
	}
	// 非 Artist 壳目录：不应识别作品集。
	if got := matchedCollectionOf(decisions[1]); got != "" {
		t.Errorf("OtherDir 不应识别作品集，实际 %q", got)
	}
	// Artist 壳直属文件：自身行 {集名} 的最深命中层=壳目录自身，与成员命中层
	// 重合 → 之下不含其身 → 零作品集。
	if got := matchedCollectionOf(decisions[2]); got != "" {
		t.Errorf("Artist[foo] 直属文件不应识别作品集（之下不含其身），实际 %q", got)
	}
	if len(decisions[2].MatchedRuleIDs) != 1 || decisions[2].MatchedRuleIDs[0] != "manual-作者-artist-作者结构-1793237147531" {
		t.Errorf("Artist[foo] 直属文件命中应仅成员规则，实际 %v", decisions[2].MatchedRuleIDs)
	}
}

// matchedCollectionOf 取结论中首个作品集维度声明值（无则空串）。
func matchedCollectionOf(d FileDecision) string {
	for _, decl := range d.Declarations {
		if decl.Dimension == DimCollection {
			return decl.Value
		}
	}
	return ""
}

// TestDirMeaningReplayRefGateMatrix 目录层含义回放（识别全景元素1 消费）的
// 门控矩阵：Artist 壳下子目录得作品集含义、无 Artist 壳祖先的目录零作品集
// 含义、Artist 壳目录自身零作品集含义（之下不含其身——作者含义仍由成员规则
// 覆盖）、首个覆盖规则随门分叉、门过链上更深层取最深 {集名} 层。
func TestDirMeaningReplayRefGateMatrix(t *testing.T) {
	profile, errs := ParseProfile([]byte(realComboProfile))
	if profile == nil {
		t.Fatalf("档案不可载入: %v", errs)
	}
	comboID := "manual-作品集-作者-目录-结构的-1793237671031"
	memberID := "manual-作者-artist-作者结构-1793237147531"

	// Artist 壳下子目录：门过（祖先链含 Artist 壳）→ 作品集含义=子目录名；
	// 成员规则命中落祖先层不算覆盖本层 → 首个覆盖规则=组合规则。
	meanings, firstID := dirMeaningReplay(profile.Rules, []string{"Artist[foo]", "SetA"}, nil)
	if !meaningContains(meanings, string(PathTypeWorkSet), "SetA") {
		t.Fatalf("Artist 壳下子目录应回放作品集含义 SetA: %+v", meanings)
	}
	if firstID != comboID {
		t.Fatalf("首个覆盖规则应为组合规则（成员命中落祖先层），实际 %q", firstID)
	}

	// 无 Artist 壳祖先的目录：门失败 → 组合规则零含义、零覆盖。
	meanings, firstID = dirMeaningReplay(profile.Rules, []string{"OtherDir"}, nil)
	for _, m := range meanings {
		if m.Type == string(PathTypeWorkSet) {
			t.Fatalf("无 Artist 壳祖先的目录不应回放出作品集含义: %+v", meanings)
		}
	}
	if firstID != "" {
		t.Fatalf("无规则覆盖的目录首个覆盖规则应为空，实际 %q", firstID)
	}

	// Artist 壳目录自身：成员在本层命中 → 作者含义仍在；组合 {集名} 锚定层与
	// 成员命中层重合（之下不含其身）→ 零作品集含义。
	meanings, firstID = dirMeaningReplay(profile.Rules, []string{"Artist[foo]"}, nil)
	if !meaningContains(meanings, string(PathTypeLocalAuthor), "foo") {
		t.Fatalf("Artist 壳目录自身应回放作者含义 foo: %+v", meanings)
	}
	for _, m := range meanings {
		if m.Type == string(PathTypeWorkSet) {
			t.Fatalf("Artist 壳目录自身不应回放任何作品集含义（之下不含其身）: %+v", meanings)
		}
	}
	if firstID != memberID {
		t.Fatalf("首个覆盖规则应为成员规则，实际 %q", firstID)
	}

	// 门过链上更深的子目录：仍取最深 {集名} 层。
	meanings, _ = dirMeaningReplay(profile.Rules, []string{"Artist foo", "Vol1", "Page"}, nil)
	if !meaningContains(meanings, string(PathTypeWorkSet), "Page") {
		t.Fatalf("门过链上更深层应回放该层作品集含义 Page: %+v", meanings)
	}
}

// TestDirMeaningReplayDirLevelAnchor 目录含义回放通道（声明5 第 4 路，AE 阶段5
// 核验）：回放要求命中落点恰为目标层（path_rule.go:240 的 hit.Depth/hit.Name
// 对照），与层级锚天然同向——锚定规则只在 N 级层记录 DirHit，故仅当目标目录
// 正是第 N 级时回放含义；目标在更深层时锚定祖先层的命中不算覆盖（严格不回退），
// 该层零含义、零首个覆盖规则。
func TestDirMeaningReplayDirLevelAnchor(t *testing.T) {
	mustRule := func(id string, level int) RuleEntry {
		t.Helper()
		rows := []CondxRow{{Condition: &Condx{
			Target: CondxTargetDir, Format: condxTpl("{标签}"),
			Attach: []CondxAttach{{Kind: CondxAttachDirLevel, MinCount: level}},
		}}}
		p, err := CompileRuleConditions(rows)
		if err != nil {
			t.Fatalf("编译失败（%s）: %v", id, err)
		}
		return RuleEntry{
			ID: id, Description: "测试：" + id, Source: RuleSourceManual, Enabled: true,
			Conditions: rows, Pattern: p,
			Extractions: []ExtractionAction{{Dimension: DimTag, From: FromCapture, CaptureName: "tag", Row: 1}},
		}
	}
	level1 := []RuleEntry{mustRule("level1-tag", 1)}
	// 目标=第 1 级：回放标签含义，首个覆盖规则即该规则。
	meanings, firstID := dirMeaningReplay(level1, []string{"[路灯后辈]"}, nil)
	if !meaningContains(meanings, string(PathTypeLocalTag), "[路灯后辈]") {
		t.Fatalf("第 1 级目标应回放标签含义: %+v", meanings)
	}
	if firstID != "level1-tag" {
		t.Fatalf("首个覆盖规则应为 level1-tag，实际 %q", firstID)
	}
	// 目标=第 2 级：规则在锚定祖先层（第 1 级）命中不算覆盖本层 → 零含义零覆盖。
	meanings, firstID = dirMeaningReplay(level1, []string{"[路灯后辈]", "子目录"}, nil)
	if len(meanings) != 0 || firstID != "" {
		t.Fatalf("目标在更深层时锚定祖先层命中不应回放覆盖: %+v firstID=%q", meanings, firstID)
	}

	// 第 2 级锚规则：两级链的目标层命中 → 回放；单级链不存在第 2 级 → 零。
	level2 := []RuleEntry{mustRule("level2-tag", 2)}
	meanings, firstID = dirMeaningReplay(level2, []string{"monster girl", "[cool]"}, nil)
	if !meaningContains(meanings, string(PathTypeLocalTag), "[cool]") || firstID != "level2-tag" {
		t.Fatalf("第 2 级目标应回放第 2 级标签含义: %+v firstID=%q", meanings, firstID)
	}
	meanings, firstID = dirMeaningReplay(level2, []string{"[cool]"}, nil)
	if len(meanings) != 0 || firstID != "" {
		t.Fatalf("祖先链无第 2 级时不应回放: %+v firstID=%q", meanings, firstID)
	}
}

// meaningContains 判断含义序列是否含（类型, 名字）对。
func meaningContains(meanings []PathMeaning, typ, name string) bool {
	for _, m := range meanings {
		if m.Type == typ && m.Name == name {
			return true
		}
	}
	return false
}
