// 规则匹配阶段的输入快照与解析上下文：纯数据模型与无副作用方法，无 IO。
package main

import (
	"path/filepath"
	"strings"
)

// MatchInput 为单条待解析条目的匹配输入。
type MatchInput struct {
	// FileName 为文件名（含扩展名），不含目录部分。
	FileName string
	// DirNames 为从库根到直接父目录的各层目录名，索引即深度。
	DirNames []string
	// RelPath 为以 '/' 分隔的相对路径（相对库根）。
	RelPath string
	// Siblings 为同目录全部兄弟文件名（含自身）。
	Siblings []string
	// AncestorFiles 为各层祖先目录的**直接子文件名**列表，索引与 DirNames 对齐
	//（深度 d 的元素 = 目录链 DirNames[0..d] 的直接子文件，不含更深层内容）。
	// 由批级流水线从同批条目聚合派生；nil（构造点未提供）或对应层无元素时，
	// 目录终结谓词（子女全部/至少N/子女类型）对该层保守不命中。
	AncestorFiles [][]string
	// AncestorChildDirCounts 为各层祖先目录的直接子目录数（仅目录计数，不含
	// 文件），索引与 DirNames 对齐；未提供时子目录数谓词保守不命中。
	AncestorChildDirCounts []int
}

// ancestorFilesAt 返回深度 d 祖先目录的直接子文件名（true=数据可用且非空）。
func (in *MatchInput) ancestorFilesAt(depth int) ([]string, bool) {
	if depth < 0 || depth >= len(in.AncestorFiles) {
		return nil, false
	}
	files := in.AncestorFiles[depth]
	if len(files) == 0 {
		return nil, false
	}
	return files, true
}

// ancestorChildDirCountAt 返回深度 d 祖先目录的直接子目录数（false=数据未提供）。
func (in *MatchInput) ancestorChildDirCountAt(depth int) (int, bool) {
	if depth < 0 || depth >= len(in.AncestorChildDirCounts) {
		return 0, false
	}
	return in.AncestorChildDirCounts[depth], true
}

// LibraryIndex 为谓词求值时注入的库内数据快照。
type LibraryIndex struct {
	// AuthorNames 为库内已有作者名集合。
	AuthorNames map[string]struct{}
	// TagNames 为库内已有标签名集合。
	TagNames map[string]struct{}
	// WorkSetNames 为库内作品集名集合——集名互证索引。
	WorkSetNames map[string]struct{}
	// WorkSetIDs 为库内作品集名 → 集行 id（第五轨同一次拉取填充）：A 档
	// 「归入既有集」比对的既有 id 来源。同名多行时取后拉到的行（互证按名
	// 判定不受影响，id 仅呈现与旁路簿记用）。
	WorkSetIDs map[string]int64
}

// GroupAnalysis 为群组分析产物，描述条目所属的连续序列群组。
type GroupAnalysis struct {
	// Key 为分组键，同一群组内所有成员相同。
	Key string
	// Prefix 为群组公共前缀。
	Prefix string
	// Seq 为本条目在群组内的序号，从 1 起。
	Seq int
	// Size 为群组成员总数。
	Size int
}

// CaptureBinding 为一条正则命名捕获组的命中绑定。
type CaptureBinding struct {
	// Name 为捕获组名。
	Name string
	// Value 为该次命中的捕获文本。
	Value string
	// Segment 为本次匹配所作用的目标文本（文件名 / 相对路径或某层目录名）：
	// 供贪婪识别「未覆盖片段」范围做同段区间比对；空 = 旧绑定未记录（视为
	// 不可比对，不参与排除）。
	Segment string
	// Start / End 为 Value 在 Segment 内的字节区间 [Start, End)；Segment 为空
	// 或该组未参与本次匹配时无意义（未参与时为零值）。
	Start int
	End   int
}

// PredicateHit 为一次谓词命中记录。
type PredicateHit struct {
	// Kind 为谓词类别，取值 "author"、"tag" 或 "workSet"；谓词类型枚举定义在 pattern 文件中。
	Kind string
	// Input 为参与求值的输入文本。
	Input string
	// Name 为命中的作者名或标签名。
	Name string
}

// DirHit 为一次目录层命中记录。
type DirHit struct {
	// Depth 为命中目录的深度，与 DirNames 的索引对应。
	Depth int
	// Name 为命中的目录名。
	Name string
}

// ParseContext 为解析上下文：匹配过程产出的全部绑定；非群组成员时 Group 为 nil。
type ParseContext struct {
	// Captures 为全部命名捕获组绑定，按命中顺序追加。
	Captures []CaptureBinding
	// Group 为所属群组的分析产物，非群组成员时为 nil。
	Group *GroupAnalysis
	// DirNames 为从库根到直接父目录的各层目录名，索引即深度。
	DirNames []string
	// Depth 为条目所在目录的深度。
	Depth int
	// PredHits 为全部谓词命中记录，按命中顺序追加。
	PredHits []PredicateHit
	// DirHits 为全部目录层命中记录，按命中顺序追加。
	DirHits []DirHit
}

// Capture 返回组名为 name 的最后一条绑定值；不存在时返回 ("", false)。
func (c *ParseContext) Capture(name string) (string, bool) {
	value := ""
	found := false
	for _, binding := range c.Captures {
		if binding.Name == name {
			value = binding.Value
			found = true
		}
	}
	return value, found
}

// LastDirHit 返回 Depth 最大的一条目录命中（Depth 相同时取最后一条）；无记录时返回零值与 false。
func (c *ParseContext) LastDirHit() (DirHit, bool) {
	var best DirHit
	found := false
	for _, hit := range c.DirHits {
		if !found || hit.Depth >= best.Depth {
			best = hit
			found = true
		}
	}
	return best, found
}

// ruleScopedContext 派生规则私有求值作用域（提取段逐规则取用）：捕获绑定与
// 目录/谓词命中都从空开始——同名捕获组跨规则互不遮蔽，FromAncestorDir/
// FromPredicate 只读自身规则的锚定/谓词命中（跨规则不再共享累积：共享时代
// 后求值的规则会读到先求值规则更深/更晚的命中，产出他目录剥壳的垃圾值，
// 发现6）；「取最深/取最后」语义保留在规则作用域内（规则内多模式锚定取该
// 规则内最深命中）。群组与目录名基座共享。
func (c *ParseContext) ruleScopedContext() *ParseContext {
	return &ParseContext{
		Group:    c.Group,
		DirNames: c.DirNames,
		Depth:    c.Depth,
	}
}

// absorbScoped 并回规则私有作用域期间变更的共享态：群组字段（定群规则求值
// 写入）。捕获绑定与目录/谓词命中是规则私有的，不并回。
func (c *ParseContext) absorbScoped(scoped *ParseContext) {
	c.Group = scoped.Group
}

// BaseNameWithoutExt 去掉文件名的最后一段扩展名（filepath.Ext 语义）：
// 多点文件（如 "a.tar.gz"）只去最后一段得 "a.tar"；无扩展名时原样返回。
func BaseNameWithoutExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}
