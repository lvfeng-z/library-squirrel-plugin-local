package main

// 作用域附件（refScope·阶段2）在「目录层含义回放」通道与文件级流水线的双通道
// 一致性锚（沿 AB 规则组合引用缺陷修复时代的真实档案锚改造）：开发环境实测
// 档案原文（refs 组合→作用域附件改写）锚定两侧——文件级流水线门控矩阵 +
// 目录层含义回放门控矩阵（回放与提取段同一门段入口 runRefGateStage 与层距
// 阶梯判据，裁决⑦双通道纪律）。
import (
	"strings"
	"testing"
)

// realScopeProfile 开发环境实测档案原文改造（2026-10-29 原档 refs 组合→阶段2
// 作用域附件）：一条序列定群规则、一条 Artist 壳作者目录规则（供给方——两行
// or 并列、纯目录）、一条其下紧邻层的 {集名} 目录规则（消费方——目录行挂
// refScope 附件，层距 1）。
const realScopeProfile = `{"schemaVersion":4,"rules":[
{"id":"qna-seq-たじ-57719012-ボテ詰め合わせ-1793038910903","description":"【作品集】序列型作品集","source":"qna","enabled":true,"conditions":[{"condition":{"target":"seq","format":{"mode":"template","tpl":"[{作者}]_[{作品号}]_{标题}_[{序号}]"},"attach":[{"kind":"seqMinLength","minCount":2}]}},{"connector":"or","condition":{"target":"seq","format":{"mode":"template","tpl":"[{作者}] [{作品号}] [{标题}]{序号}"},"attach":[{"kind":"seqMinLength","minCount":2}]}}],"extractions":[],"synthesis":{"dimPolicies":[],"roles":[]}},
{"id":"manual-作者-artist-作者结构-1793237147531","description":"【作者】Artist+作者结构","source":"manual","enabled":true,"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"Artist[{作者}]"}}},{"connector":"or","condition":{"target":"dir","format":{"mode":"template","tpl":"Artist {作者}"}}}],"extractions":[{"dimension":"author","from":"capture","captureName":"author","constant":"","roleName":""},{"dimension":"author","from":"capture","captureName":"author","row":2,"constant":"","roleName":""}],"synthesis":{"dimPolicies":[],"roles":[]}},
{"id":"manual-作品集-作者-目录-结构的-1793237671031","description":"【作品集】作者/目录 结构的","source":"manual","enabled":true,"conditions":[{"condition":{"target":"dir","format":{"mode":"template","tpl":"{集名}"},"attach":[{"kind":"refScope","value":"manual-作者-artist-作者结构-1793237147531","minCount":1}]}}],"extractions":[{"dimension":"collection","from":"capture","captureName":"setname","constant":"","roleName":""}],"synthesis":{"dimPolicies":[],"roles":[]}}]}`

// TestRealProfileComboGatePipeline 真实档案的文件级流水线门控基线：Artist 壳
// 下紧邻子目录识别为作品集、无 Artist 壳祖先的目录零作品集、Artist 壳直属
// 文件零作品集（其身在层距阶梯判据下天然不可成立——原「之下不含其身」重合
// 排除由裁决⑥ 的阶梯判据承接）。
func TestRealProfileComboGatePipeline(t *testing.T) {
	profile, errs := ParseProfile([]byte(realScopeProfile))
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
	// Artist 壳下紧邻子目录：应识别为作品集。
	if got := matchedCollectionOf(decisions[0]); got != "SetA" {
		t.Errorf("Artist[foo]/SetA 应识别作品集 SetA，实际 %q", got)
	}
	// 非 Artist 壳目录：供给方未命中 → 附件谓词不满足 → 不应识别作品集。
	if got := matchedCollectionOf(decisions[1]); got != "" {
		t.Errorf("OtherDir 不应识别作品集，实际 %q", got)
	}
	// Artist 壳直属文件：{集名} 的最深候选层=壳目录自身=供给方命中层（其身）
	// → 阶梯判据不可满足 → 零作品集。
	if got := matchedCollectionOf(decisions[2]); got != "" {
		t.Errorf("Artist[foo] 直属文件不应识别作品集（其身不可成立），实际 %q", got)
	}
	if len(decisions[2].MatchedRuleIDs) != 1 || decisions[2].MatchedRuleIDs[0] != "manual-作者-artist-作者结构-1793237147531" {
		t.Errorf("Artist[foo] 直属文件命中应仅供给方规则，实际 %v", decisions[2].MatchedRuleIDs)
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
// 门控矩阵（作用域附件版）：Artist 壳下紧邻子目录得作品集含义、无 Artist 壳
// 祖先的目录零作品集含义、Artist 壳目录自身零作品集含义（其身——作者含义仍
// 由供给方规则覆盖）、首个覆盖规则随门分叉；紧邻层距在中隔链上保守不覆盖
// （候选层须恰在供给方命中层下一级）、任意深度档（minCount=0 哨兵）在更深层
// 取最深 {集名} 层——回放与提取段同一门段入口与阶梯判据（双通道纪律）。
func TestDirMeaningReplayRefGateMatrix(t *testing.T) {
	profile, errs := ParseProfile([]byte(realScopeProfile))
	if profile == nil {
		t.Fatalf("档案不可载入: %v", errs)
	}
	consumerID := "manual-作品集-作者-目录-结构的-1793237671031"
	providerID := "manual-作者-artist-作者结构-1793237147531"

	// Artist 壳下紧邻子目录：供给方命中（祖先链含 Artist 壳）→ 紧邻层距成立
	// → 作品集含义=子目录名；供给方命中落祖先层不算覆盖本层 → 首个覆盖规则
	//=消费规则。
	meanings, firstID := dirMeaningReplay(profile.Rules, []string{"Artist[foo]", "SetA"}, nil)
	if !meaningContains(meanings, string(PathTypeWorkSet), "SetA") {
		t.Fatalf("Artist 壳下紧邻子目录应回放作品集含义 SetA: %+v", meanings)
	}
	if firstID != consumerID {
		t.Fatalf("首个覆盖规则应为消费规则（供给方命中落祖先层），实际 %q", firstID)
	}

	// 无 Artist 壳祖先的目录：供给方未命中 → 附件谓词不满足 → 零含义、零覆盖。
	meanings, firstID = dirMeaningReplay(profile.Rules, []string{"OtherDir"}, nil)
	for _, m := range meanings {
		if m.Type == string(PathTypeWorkSet) {
			t.Fatalf("无 Artist 壳祖先的目录不应回放出作品集含义: %+v", meanings)
		}
	}
	if firstID != "" {
		t.Fatalf("无规则覆盖的目录首个覆盖规则应为空，实际 %q", firstID)
	}

	// Artist 壳目录自身：供给方在本层命中 → 作者含义仍在；消费行 {集名} 候选
	// 层=供给方命中层（其身）→ 阶梯判据不可满足 → 零作品集含义。
	meanings, firstID = dirMeaningReplay(profile.Rules, []string{"Artist[foo]"}, nil)
	if !meaningContains(meanings, string(PathTypeLocalAuthor), "foo") {
		t.Fatalf("Artist 壳目录自身应回放作者含义 foo: %+v", meanings)
	}
	for _, m := range meanings {
		if m.Type == string(PathTypeWorkSet) {
			t.Fatalf("Artist 壳目录自身不应回放任何作品集含义（其身不可成立）: %+v", meanings)
		}
	}
	if firstID != providerID {
		t.Fatalf("首个覆盖规则应为供给方规则，实际 %q", firstID)
	}

	// 中隔链（紧邻档）：候选层 Page 不在供给方命中层下一级（Vol1 在其间）
	// → 保守不覆盖，零作品集含义。
	meanings, _ = dirMeaningReplay(profile.Rules, []string{"Artist foo", "Vol1", "Page"}, nil)
	for _, m := range meanings {
		if m.Type == string(PathTypeWorkSet) {
			t.Fatalf("中隔链在紧邻档下不应回放作品集含义（候选层须恰在下一级）: %+v", meanings)
		}
	}

	// 任意深度档（minCount=0 哨兵）：同一中隔链上更深的 Page 层可成立 → 取
	// 最深 {集名} 层回放该层含义。
	anyDepthProfile := strings.Replace(realScopeProfile, `"minCount":1}`, `"minCount":0}`, 1)
	profileAny, errs := ParseProfile([]byte(anyDepthProfile))
	if profileAny == nil {
		t.Fatalf("任意深度档案不可载入: %v", errs)
	}
	meanings, _ = dirMeaningReplay(profileAny.Rules, []string{"Artist foo", "Vol1", "Page"}, nil)
	if !meaningContains(meanings, string(PathTypeWorkSet), "Page") {
		t.Fatalf("任意深度档在更深层应回放该层作品集含义 Page: %+v", meanings)
	}
}

// 【已删除·决策6 改判】TestDirMeaningReplayDirLevelAnchor（目录含义回放通道的
// 层级锚用例，原声明5 第 4 路 AE 阶段5 核验）随 dirLevel 词条退役删除：词条
// 白名单已移除、携带即校验拒绝，回放按命中层对照的锚定语义失去构造入口；
// 多段目录模板的回放对照由后续阶段通道用例承接（DirHit 记落地段层——自决6）。

// meaningContains 判断含义序列是否含（类型, 名字）对。
func meaningContains(meanings []PathMeaning, typ, name string) bool {
	for _, m := range meanings {
		if m.Type == typ && m.Name == name {
			return true
		}
	}
	return false
}
