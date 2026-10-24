package main

// 「形态即元数据含义」表达力锚定（终审复看反馈⑥-B）：用户实例形态——序列所在
// 目录为 {作者}_{作品集合名称}_{序号} 且上级目录为 Artist[{作者}] 的双层目录
// 结构，触发作者/集名提取，且两层正则各自捕获的作者要求相等（跨层一致性）。
// 本测试把该形态写成真实的手写规则条目 JSON（与编辑弹窗「高级（JSON）」模式
// 手写产物同构），经 ParseProfile 载入（含构造期校验）后在提取段求值：
//   - 双层 LocationPattern 组合锚定（第 2 层集目录格式 + 第 1 层 Artist 壳格式）
//     各自带命名捕获组，And 组合下全部命中规则才命中；
//   - 跨层一致性经提取动作 When 条件承载：equal(capture setauthor, capture
//     shellauthor)——v3 条件表达式树的捕获组相等比较，两层作者不一致时该规则
//     零产出（规则命中计数仍在，声明不产出）；
//   - 捕获组名须为 ASCII 标识符（Go RE2 拒绝 CJK 组名，锚定该事实——编辑弹窗
//     教学文案的 (?P<组名>…) 占位符易误导手写者写中文名）。
import (
	"encoding/json"
	"strings"
	"testing"
)

// artistShellSetDirRuleJSON 用户实例形态的手写规则条目 JSON：上半识别规则=
// 双层目录锚定组合（第 2 层 {作者}_{集名}_{序号} + 第 1 层 Artist[{作者}]），
// 下半提取规则=作者←集目录作者捕获、集名←集目录集名捕获，两动作均挂跨层
// 作者相等条件。
const artistShellSetDirRuleJSON = `{
  "schemaVersion": 4,
  "rules": [
    {
      "id": "manual-artist-set-dir",
      "description": "Artist壳/{作者}_{集名}_{序号} 双层目录形态：跨层作者一致才产出作者与集名",
      "source": "manual",
      "enabled": true,
      "pattern": {
        "kind": "and",
        "patterns": [
          {
            "kind": "location",
            "minDepth": 1,
            "maxDepth": 1,
            "ancestor": {
              "kind": "regex",
              "target": "filename",
              "expr": "^(?P<setauthor>[^_]+)_(?P<setname>.+)_(?P<seqno>[0-9]+)$",
              "captures": ["setauthor", "setname", "seqno"]
            }
          },
          {
            "kind": "location",
            "minDepth": 0,
            "maxDepth": 0,
            "ancestor": {
              "kind": "regex",
              "target": "filename",
              "expr": "^Artist\\[(?P<shellauthor>[^\\]]+)\\]$",
              "captures": ["shellauthor"]
            }
          }
        ]
      },
      "extractions": [
        {
          "dimension": "author",
          "from": "capture",
          "captureName": "setauthor",
          "when": {
            "kind": "equal",
            "left": { "kind": "capture", "captureName": "setauthor" },
            "right": { "kind": "capture", "captureName": "shellauthor" }
          }
        },
        {
          "dimension": "collection",
          "from": "capture",
          "captureName": "setname",
          "when": {
            "kind": "equal",
            "left": { "kind": "capture", "captureName": "setauthor" },
            "right": { "kind": "capture", "captureName": "shellauthor" }
          }
        }
      ],
      "synthesis": {
        "dimPolicies": [
          { "dimension": "author", "strategy": "union" },
          { "dimension": "collection", "strategy": "union" }
        ],
        "roles": []
      }
    }
  ]
}`

// runArtistShellRule 载入手写规则 JSON 并在单文件输入上跑提取段，返回决策
// （命中规则 ID 集 + 产出声明）。
func runArtistShellRule(t *testing.T, in *MatchInput) (*FileDecision, RuleEntry) {
	t.Helper()
	profile, errs := ParseProfile([]byte(artistShellSetDirRuleJSON))
	if len(errs) > 0 {
		t.Fatalf("手写规则 JSON 载入失败: %v", errs)
	}
	if len(profile.Rules) != 1 {
		t.Fatalf("载入后规则数应为 1，实际 %d", len(profile.Rules))
	}
	rule := profile.Rules[0]
	decision := &FileDecision{RelPath: in.RelPath, FileName: in.FileName}
	ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
	runExtractionStage(decision, profile.Rules, in, nil, ctx)
	return decision, rule
}

// declOf 取决策内某规则在某维度上的声明值。
func declOf(decision *FileDecision, ruleID string, dim MetadataDimension) (string, bool) {
	for i := range decision.Declarations {
		if decision.Declarations[i].RuleID == ruleID && decision.Declarations[i].Dimension == dim {
			return decision.Declarations[i].Value, true
		}
	}
	return "", false
}

// TestArtistShellSetDirFormConsistentHit 一致命中：两层目录形态与跨层作者一致
// 均成立 → 规则命中且产出作者=Isel、集名=瞬間的に絡み合う。
func TestArtistShellSetDirFormConsistentHit(t *testing.T) {
	in := &MatchInput{
		FileName: "表紙.png",
		DirNames: []string{"Artist[Isel]", "Isel_瞬間的に絡み合う_01"},
		RelPath:  "Artist[Isel]/Isel_瞬間的に絡み合う_01/表紙.png",
	}
	decision, rule := runArtistShellRule(t, in)
	if len(decision.MatchedRuleIDs) != 1 || decision.MatchedRuleIDs[0] != rule.ID {
		t.Fatalf("规则应命中：matched=%v", decision.MatchedRuleIDs)
	}
	if v, ok := declOf(decision, rule.ID, DimAuthor); !ok || v != "Isel" {
		t.Fatalf("作者声明应为 Isel：got %q ok=%v", v, ok)
	}
	if v, ok := declOf(decision, rule.ID, DimCollection); !ok || v != "瞬間的に絡み合う" {
		t.Fatalf("集名声明应为 瞬間的に絡み合う：got %q ok=%v", v, ok)
	}
}

// TestArtistShellSetDirFormCrossLayerMismatch 跨层不一致：两层形态各自成立但
// 作者不等（壳=Beko、集目录=Isel）→ 规则仍命中（形态组合成立），但 When 相等
// 条件不成立 → 该规则零声明产出（不产出错误作者/集名）。
func TestArtistShellSetDirFormCrossLayerMismatch(t *testing.T) {
	in := &MatchInput{
		FileName: "表紙.png",
		DirNames: []string{"Artist[Beko]", "Isel_別冊_02"},
		RelPath:  "Artist[Beko]/Isel_別冊_02/表紙.png",
	}
	decision, _ := runArtistShellRule(t, in)
	if len(decision.MatchedRuleIDs) != 1 {
		t.Fatalf("形态组合成立时规则应命中（命中计数）：matched=%v", decision.MatchedRuleIDs)
	}
	if len(decision.Declarations) != 0 {
		t.Fatalf("跨层作者不一致时应零产出：decls=%+v", decision.Declarations)
	}
}

// TestArtistShellSetDirFormAnchorMisses 锚定不达：无 Artist 壳（单层）、壳在
// 场但集目录层异形——两种情况下规则不命中。
func TestArtistShellSetDirFormAnchorMisses(t *testing.T) {
	cases := []struct {
		name     string
		dirNames []string
		relPath  string
	}{
		{"无壳单层", []string{"Isel_瞬間的に絡み合う_01"}, "Isel_瞬間的に絡み合う_01/表紙.png"},
		{"集目录层异形", []string{"Artist[Isel]", "manual"}, "Artist[Isel]/manual/表紙.png"},
		{"壳形不符", []string{"Isel", "Isel_別集_01"}, "Isel/Isel_別集_01/表紙.png"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := &MatchInput{FileName: "表紙.png", DirNames: c.dirNames, RelPath: c.relPath}
			decision, rule := runArtistShellRule(t, in)
			if len(decision.MatchedRuleIDs) != 0 {
				t.Fatalf("规则不应命中：matched=%v", decision.MatchedRuleIDs)
			}
			for i := range decision.Declarations {
				if decision.Declarations[i].RuleID == rule.ID {
					t.Fatalf("不应有该规则的声明：%+v", decision.Declarations[i])
				}
			}
		})
	}
}

// TestArtistShellSetDirFormNestedFile 成员目录内更深层文件：集成员目录下带
// 子目录时（文件隔层）双层锚定按层深仍命中且产出一致——锚定语义=按深度区间
// 的目录层形态，非「文件直接父目录」。
func TestArtistShellSetDirFormNestedFile(t *testing.T) {
	in := &MatchInput{
		FileName: "內頁.png",
		DirNames: []string{"Artist[Isel]", "Isel_瞬間的に絡み合う_01", "sub"},
		RelPath:  "Artist[Isel]/Isel_瞬間的に絡み合う_01/sub/內頁.png",
	}
	decision, rule := runArtistShellRule(t, in)
	if len(decision.MatchedRuleIDs) != 1 {
		t.Fatalf("隔层文件按层深锚定应命中：matched=%v", decision.MatchedRuleIDs)
	}
	if v, ok := declOf(decision, rule.ID, DimAuthor); !ok || v != "Isel" {
		t.Fatalf("作者声明应为 Isel：got %q ok=%v", v, ok)
	}
	if v, ok := declOf(decision, rule.ID, DimCollection); !ok || v != "瞬間的に絡み合う" {
		t.Fatalf("集名声明应为 瞬間的に絡み合う：got %q ok=%v", v, ok)
	}
}

// TestArtistShellSetDirFormCJKCaptureNameRejected CJK 捕获组名被 Go RE2 拒绝
// （锚定事实）：编辑弹窗教学文案的 (?P<组名>…) 占位符易误导手写者写中文名，
// 构造期以中文错误拦截（NewRegexPattern → 编译失败）。
func TestArtistShellSetDirFormCJKCaptureNameRejected(t *testing.T) {
	_, err := NewRegexPattern("filename", `^(?P<作者>[^_]+)_`, []string{"作者"})
	if err == nil {
		t.Fatal("CJK 捕获组名应被正则编译拒绝")
	}
	if !strings.Contains(err.Error(), "invalid named capture") && !strings.Contains(err.Error(), "编译失败") {
		t.Fatalf("应为编译失败类错误：got %v", err)
	}
	// 对照：同规则换 ASCII 组名可载入（整条档案 JSON 级对照）。
	swapped := strings.ReplaceAll(artistShellSetDirRuleJSON, "setauthor", "dirAuthor")
	swapped = strings.ReplaceAll(swapped, "setname", "dirTitle")
	if !strings.Contains(swapped, "dirAuthor") {
		t.Fatal("替换失败：规则 JSON 内应含替换后的组名")
	}
	var probe struct {
		Rules []json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal([]byte(swapped), &probe); err != nil || len(probe.Rules) != 1 {
		t.Fatalf("替换后 JSON 应仍可解析：err=%v rules=%d", err, len(probe.Rules))
	}
}

// artistLibPredicateRuleJSON 「{作者} = 已被识别出的作者」的库内互证落法：
// 集目录形态正则捕获作者，And 内接 author 谓词（Kind=author、From=捕获组名）
// ——捕获值必须命中库内作者集合（LibraryIndex.AuthorNames，宿主侧已入库作品
// 的作者名集）规则才命中。与壳层无关：不写上级 Artist 壳正则，作者域=库内
// 已有作者全集（任何途径入库的作者都算「已被识别出」）。
const artistLibPredicateRuleJSON = `{
  "schemaVersion": 4,
  "rules": [
    {
      "id": "manual-setdir-author-lib-predicate",
      "description": "集目录 {作者}_{集名}_{序号} 且作者命中库内作者集：作者/集名提取",
      "source": "manual",
      "enabled": true,
      "pattern": {
        "kind": "and",
        "patterns": [
          {
            "kind": "location",
            "minDepth": 1,
            "maxDepth": 1,
            "ancestor": {
              "kind": "regex",
              "target": "filename",
              "expr": "^(?P<setauthor>[^_]+)_(?P<setname>.+)_(?P<seqno>[0-9]+)$",
              "captures": ["setauthor", "setname", "seqno"]
            }
          },
          { "kind": "predicate", "predKind": "author", "from": "setauthor" }
        ]
      },
      "extractions": [
        { "dimension": "author", "from": "capture", "captureName": "setauthor" },
        { "dimension": "collection", "from": "capture", "captureName": "setname" }
      ],
      "synthesis": {
        "dimPolicies": [
          { "dimension": "author", "strategy": "union" },
          { "dimension": "collection", "strategy": "union" }
        ],
        "roles": []
      }
    }
  ]
}`

// TestSetDirFormAuthorLibraryPredicate 「{作者}=已被识别出的作者」的库内互证
// 落法：谓词在命中级（And 组合内）拦截——作者不在库内作者集时规则整体不命中
// （对照 When 条件的提取级门：不产出但命中计数仍在）；作者在库内时命中并产出。
// 边界：首轮导入的全新作者尚未入库，谓词不命中、该轮不产出（库内互证的固有
// 边界，与集名互证同族）。
func TestSetDirFormAuthorLibraryPredicate(t *testing.T) {
	run := func(lib *LibraryIndex, in *MatchInput) *FileDecision {
		t.Helper()
		profile, errs := ParseProfile([]byte(artistLibPredicateRuleJSON))
		if len(errs) > 0 {
			t.Fatalf("库内互证规则 JSON 载入失败: %v", errs)
		}
		decision := &FileDecision{RelPath: in.RelPath, FileName: in.FileName}
		ctx := &ParseContext{DirNames: in.DirNames, Depth: len(in.DirNames)}
		runExtractionStage(decision, profile.Rules, in, lib, ctx)
		return decision
	}
	in := &MatchInput{
		FileName: "表紙.png",
		DirNames: []string{"Artist[Isel]", "Isel_瞬間的に絡み合う_01"},
		RelPath:  "Artist[Isel]/Isel_瞬間的に絡み合う_01/表紙.png",
	}
	libWithIsel := &LibraryIndex{AuthorNames: map[string]struct{}{"Isel": {}, "Beko": {}}}
	decision := run(libWithIsel, in)
	if len(decision.MatchedRuleIDs) != 1 {
		t.Fatalf("作者在库内作者集时规则应命中：matched=%v", decision.MatchedRuleIDs)
	}
	if v, ok := declOf(decision, "manual-setdir-author-lib-predicate", DimAuthor); !ok || v != "Isel" {
		t.Fatalf("作者声明应为 Isel：got %q ok=%v", v, ok)
	}
	// 作者不在库内（全新作者首轮）：And 内谓词失败 → 规则整体不命中（命中级拦截）。
	libWithout := &LibraryIndex{AuthorNames: map[string]struct{}{"Beko": {}}}
	decision2 := run(libWithout, in)
	if len(decision2.MatchedRuleIDs) != 0 || len(decision2.Declarations) != 0 {
		t.Fatalf("作者不在库内时规则不应命中：matched=%v decls=%+v", decision2.MatchedRuleIDs, decision2.Declarations)
	}
}
