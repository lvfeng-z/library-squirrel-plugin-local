// 条件构建语法 v4（收敛档 R1-R11，2026-10-24）：目标三分（file/dir/seq）+ 格式
// 模板（占位词点选）+ 附加条件（节点级终结谓词集）+ 值校验（名单/相等/常量）。
// 本文件为纯语法层：类型定义、模板→正则推导、静态校验、行列表→模式树编译。
// 求值扩展（目录子女终结谓词、形变归一名单、跨行相等门）在后续硬化步接入；
// 未实现的附加/值校验在编译期显式报错（绝不静默视为恒真）。
package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CondxTargetKind 为条件的目标类别（三级词表·目标三分）。
type CondxTargetKind string

// 三型有效范围：file 命中单个文件（匹配去扩展名的文件名）；dir 命中目录整个
// 子树与求值面的交集（任一层祖先，不限直接父目录）；seq 命中同目录同键成套
// 成员（计数≥下限，落单出局、不跨目录）。
const (
	// CondxTargetFile 表示目标=文件：按格式命中单个文件（匹配去扩展名的文件名）。
	CondxTargetFile CondxTargetKind = "file"
	// CondxTargetDir 表示目标=目录：按格式命中目录；对条目而言命中=该目录位于
	// 其祖先链上（子树包含，任意深度），终结谓词作用于该目录的子女/父目录。
	CondxTargetDir CondxTargetKind = "dir"
	// CondxTargetSeq 表示目标=序列：成员按模板成套（非{序号}部分跨成员相同为键）。
	CondxTargetSeq CondxTargetKind = "seq"
)

// CondxFormat 为格式段：目录名的两种问法（含这个词 / 整条按模板），
// 文件与序列只用 template 模式。
type CondxFormat struct {
	// Mode 为格式模式："template"（整条按模板，默认）或 "contains"（含这个词）。
	Mode string `json:"mode"`
	// Tpl 为 template 模式的占位词模板原文（如 "[{作者}][{作品号}][{标题}]"）。
	Tpl string `json:"tpl,omitempty"`
	// Word 为 contains 模式的包含词（按字面包含，不含占位词）。
	Word string `json:"word,omitempty"`
}

// 格式模式常量。
const (
	CondxFormatTemplate = "template"
	CondxFormatContains = "contains"
)

// CondxBadge 为值校验：附着在占位词上的封闭三选一断言（R6/R7）。
type CondxBadge struct {
	// Placeholder 为值校验作用的占位词（词表原文，如 "作者"）。
	Placeholder string `json:"placeholder"`
	// Occur 为值校验作用占位词的位次（同词根多次出现按从左到右出现序 1 起；
	// 0/1=位1 裸组名——存量单出现档案零变化；N≥2 按带 __N 后缀的组名寻址）。
	Occur int `json:"occur,omitempty"`
	// Kind 为值校验类别："library"（在库名单）| "equal"（与另一占位词相等）| "constant"（=常量）。
	Kind string `json:"kind"`
	// Dim 为 library 的名单维度（author / tag / collection）。
	Dim string `json:"dim,omitempty"`
	// Fold 为 library 的形变归一开关（_ - 与大小写折叠，折叠后仍唯一才命中）。
	Fold bool `json:"fold,omitempty"`
	// AgainstRow 为 equal 的对方行号（1 起，按行先序展平计数；须早于本行）。
	AgainstRow int `json:"againstRow,omitempty"`
	// AgainstPh 为 equal 的对方占位词（词表原文）。
	AgainstPh string `json:"againstPh,omitempty"`
	// AgainstOccur 为 equal 对方占位词的位次（语义同 Occur，作用于 AgainstPh）。
	AgainstOccur int `json:"againstOccur,omitempty"`
	// Value 为 constant 的常量值。
	Value string `json:"value,omitempty"`
}

// 值校验类别常量。
const (
	CondxBadgeLibrary  = "library"
	CondxBadgeEqual    = "equal"
	CondxBadgeConstant = "constant"
)

// CondxAttach 为节点级终结谓词（封闭集，按目标分化；R5 收口：迷你格式为终结
// 格式，不可再挂任何条件）。Kind 取封闭集常量之一。
type CondxAttach struct {
	// Kind 为终结谓词类别（见 CondxAttach* 常量）。
	Kind string `json:"kind"`
	// Tpl 为子女/父目录迷你格式的模板原文（{序号} 等占位词模板）。
	Tpl string `json:"tpl,omitempty"`
	// MinCount 为数量门槛（≥N；子女全部/至少N/子目录数/序列长度/文件大小KB）。
	MinCount int `json:"minCount,omitempty"`
	// Word 为 parentContains 的包含词。
	Word string `json:"word,omitempty"`
	// Types 为类型列表（childTypeIs / seqMemberType：image / video / document / archive）。
	Types []string `json:"types,omitempty"`
	// Value 为 fileExt 的扩展名（如 ".jpg"）。
	Value string `json:"value,omitempty"`
}

// 终结谓词类别常量（封闭集；按目标分化见 attachAllowed）。
const (
	// 目录：里面全部子女按迷你格式 ≥N（迷你格式为终结格式）。
	CondxAttachChildrenAllFormat = "childrenAllFormat"
	// 目录：里面至少 N 个子女按迷你格式。
	CondxAttachChildrenAtLeastN = "childrenAtLeastN"
	// 目录：子目录数 ≥N。
	CondxAttachChildDirCount = "childDirCount"
	// 目录：子女类型是指定类型之一。
	CondxAttachChildTypeIs = "childTypeIs"
	// 目录：位于此目录下（作用域附件，目录行专属——阶段2；来源=库根或某条
	// 供给方规则）。Value 复用为来源：供给方规则 ID（须为纯目录规则，保存期
	// 校验）或库根 sentinel RefScopeLibraryRoot；MinCount 复用为层距：
	// 1=紧邻（下一级，UI 缺省档）、N≥2=显式隔 N 级、0=任意深度哨兵值（与
	// 多段模板段距记号不同——模板段距不可写 0，附件的 0 是合法哨兵）。
	// 层距阶梯判据（裁决⑥）：候选层 c 合格 ⟺ 供给方命中且 ∃D∈供给方命中层集
	// s.t. 层距=0 时 c>D、层距=N 时 c−D=N——其身（c=D）与 above-form 天然
	// 不可成立。库根来源的命中层集恒为 {-1}（门段注入，无供给方求值），同一
	// 判据零改动即得绝对层级语义：紧邻=自根数起第 1 级（c=0）、隔 N=第 N 级
	//（c=N−1）、任意=恒真。
	CondxAttachRefScope = "refScope"

	// RefScopeLibraryRoot 作用域附件（refScope）的库根 sentinel：CondxAttach.
	// Value 取此值时来源=库的最顶层目录（非供给方规则），门段为其注入命中层集
	// {-1}（库根之上的虚拟层），层距阶梯谓词按同一算术得出绝对层级语义——
	// 紧邻=根直下第 1 级、隔 N=自根数起第 N 级、任意=恒真。保留串形态 $root
	// 与规则 ID 形态不冲突：规则 ID 由系统生成（手工=「manual-描述-时间戳」、
	// 预置=「preset-…」前缀族），不以 $ 起首，且保存期不会接受以 $ 起首的
	// 手写供给方 ID（除非恰等于本 sentinel——此时即库根语义本身）。sentinel
	// 无供给方求值、无悬空/自引用/链式/纯目录合格性问题（校验放行）。
	RefScopeLibraryRoot = "$root"

	// 目录/文件/序列：父目录名整条按迷你模板（上级断言，1 级）。
	CondxAttachParentFormat = "parentFormat"
	// 目录/文件/序列：父目录名含某词（上级断言，1 级）。
	CondxAttachParentContains = "parentContains"
	// 文件：扩展名匹配（如 ".jpg" / "jpg"）。
	CondxAttachFileExt = "fileExt"
	// 文件：文件大小 ≥N KB。
	CondxAttachFileSizeMin = "fileSizeMin"
	// 序列：序列长度 ≥N。
	CondxAttachSeqMinLength = "seqMinLength"
	// 序列：成员类型是指定类型之一。
	CondxAttachSeqMemberType = "seqMemberType"
)

// attachAllowed 为各目标允许的终结谓词类别封闭集。
var attachAllowed = map[CondxTargetKind]map[string]bool{
	CondxTargetFile: {
		CondxAttachFileExt: true, CondxAttachFileSizeMin: true,
		CondxAttachParentFormat: true, CondxAttachParentContains: true,
	},
	CondxTargetDir: {
		CondxAttachChildrenAllFormat: true, CondxAttachChildrenAtLeastN: true,
		CondxAttachChildDirCount: true, CondxAttachChildTypeIs: true,
		CondxAttachParentFormat:  true, CondxAttachParentContains: true,
		CondxAttachRefScope:      true,
	},
	CondxTargetSeq: {
		CondxAttachSeqMinLength: true, CondxAttachSeqMemberType: true,
		CondxAttachParentFormat: true, CondxAttachParentContains: true,
	},
}

// attachNeedsTpl / attachNeedsWord / attachNeedsCount 为各谓词的必填字段族。
var attachNeedsTpl = map[string]bool{
	CondxAttachChildrenAllFormat: true, CondxAttachChildrenAtLeastN: true, CondxAttachParentFormat: true,
}
var attachNeedsWord = map[string]bool{CondxAttachParentContains: true}
var attachNeedsCount = map[string]bool{
	CondxAttachChildrenAllFormat: true, CondxAttachChildrenAtLeastN: true,
	CondxAttachChildDirCount: true, CondxAttachFileSizeMin: true, CondxAttachSeqMinLength: true,
}

// CondxAnchor 为主体锚：v4 仅接受 nil（「基于其他条件」为二期能力，R8——
// JSON 形态预留 ref 槽，载入即拒绝，反例实例出现时加法式启用）。
type CondxAnchor struct {
	// Kind 为锚类别：当前唯一预留值 "ref"。
	Kind string `json:"kind"`
	// Row 为 ref 的对方行号（1 起；预留字段，不校验语义）。
	Row int `json:"row,omitempty"`
}

// CondxAnchorRef 为锚类别常量（预留）。
const CondxAnchorRef = "ref"

// Condx 为一条条件（行内主体声明）：目标 + 格式 + 值校验 + 终结谓词。
type Condx struct {
	// Target 为目标类别（file / dir / seq）。
	Target CondxTargetKind `json:"target"`
	// Format 为格式段（两种问法）。
	Format CondxFormat `json:"format"`
	// Badges 为值校验列表（可空）。
	Badges []CondxBadge `json:"badges,omitempty"`
	// Attach 为节点级终结谓词列表（可空）。
	Attach []CondxAttach `json:"attach,omitempty"`
	// Anchor 为主体锚（v4 必须为 nil；ref 为二期预留）。
	Anchor *CondxAnchor `json:"anchor,omitempty"`
}

// CondxRow 为行式底盘的一行：行前连接词 + 行级非 + 条件或括号子组（R1）。
type CondxRow struct {
	// Connector 为行前连接词：首行为空串，后续行 "and" / "or"。
	Connector string `json:"connector,omitempty"`
	// Negated 为行级非。
	Negated bool `json:"negated,omitempty"`
	// Condition 为行内条件；与 Group 二选一。
	Condition *Condx `json:"condition,omitempty"`
	// Group 为括号子组（递归行列表，至少一行）；与 Condition 二选一。
	Group []CondxRow `json:"group,omitempty"`
}

// condxVocab 为占位词词表（中文词 → 捕获组英文名，RE2 不支持 CJK 组名）。
// {键} 仅用于序列成员模板（标记跨成员恒同部分，产出走群组键）；
// {任意} 为非引用吞位词（派生非捕获吞位段、末位可空、可重复出现、不可被
// 提取引用）——泛化消费位（尾部码丢弃、未引用段等）专用，不与引用位争抢词表。
// 同组双词：{公共段}（新词）与 {键}（旧词）同组 key、{作品标识}（新词）与
// {作品}（旧词）同组 work——旧词为别名合法书写、同组同义（存量档案零迁移）；
// {描述} 为作品描述维度的取值词（描述声明接线见 work_fetch 终选）。
var condxVocab = map[string]string{
	"作者": "author", "集名": "setname", "序号": "seq", "标题": "title",
	"标签": "tag", "作品号": "workid", "日期": "date",
	"键": "key", "公共段": "key", "任意": "any",
	"作品标识": "work", "作品": "work", "描述": "description",
}

// condxDigits 为数字类占位词（推导为 [0-9]+）：真实日期即数字，维持纯数字类。
var condxDigits = map[string]bool{"日期": true}

// condxAlnum 为字母数字类占位词（推导为 [0-9A-Za-z]+）：编号类词按真实编号
// 形态取超集放宽——纯数字照常命中，BV 号一类字母混合编号整词可匹配。
var condxAlnum = map[string]bool{"序号": true, "作品号": true}

// condxOpenBrackets 为组界六对同型括号的开→闭映射（与组界扫描共用同一括号
// 词汇）：占位词前字面段收尾的开括号决定捕获类要排除的配对闭括号。
var condxOpenBrackets = map[rune]rune{
	'[': ']', '【': '】', '（': '）', '(': ')', '「': '」', '『': '』',
}

// tailClosingBracket 返回字面段收尾开括号的配对闭括号（字面段为空或收尾
// 不是开括号时返回 0）。
func tailClosingBracket(literal string) rune {
	r := []rune(literal)
	if len(r) == 0 {
		return 0
	}
	return condxOpenBrackets[r[len(r)-1]]
}

// condxVocabHint 为报错提示用的词表清单（稳定顺序）；旧词「键」「作品」合法
// 但不再鼓励书写，不入提示。{作品标识}（旧词 {作品}）为聚合标识专用中性取值
// 词根（命名捕获槽可用词，机制零新增——干名如 ep01 挂标签/作者等元数据语义
// 词别扭）。
const condxVocabHint = "作者/集名/序号/标题/标签/作品标识/作品号/日期/描述/公共段/任意"

// deriveOpts 为模板推导选项。
type deriveOpts struct {
	// allowKey 是否允许 {键}（仅序列成员模板）。
	allowKey bool
	// requireSeq 是否必须含 {序号}（序列成员模板）。
	requireSeq bool
}

// condxToken 为模板的词法单元：literal（字面段）或 placeholder（占位词）。
type condxToken struct {
	literal     string
	placeholder string
}

// tokenizeCondx 把模板原文切为字面段与占位词的交替序列。
func tokenizeCondx(tpl string) ([]condxToken, error) {
	var tokens []condxToken
	re := regexp.MustCompile(`\{([^{}]+)\}`)
	for len(tpl) > 0 {
		loc := re.FindStringSubmatchIndex(tpl)
		if loc == nil {
			tokens = append(tokens, condxToken{literal: tpl})
			break
		}
		if loc[0] > 0 {
			tokens = append(tokens, condxToken{literal: tpl[:loc[0]]})
		}
		tokens = append(tokens, condxToken{placeholder: tpl[loc[2]:loc[3]]})
		tpl = tpl[loc[1]:]
	}
	return tokens, nil
}

// condxRepeatable 为可带量词（+ / *）的词位集合（重复段表达方案；词法解析
// 在本文件，正则生成由任务B 接入主流程时实现）。
var condxRepeatable = map[string]bool{"标签": true, "作者": true, "集名": true}

// condxRepeatableHint 为可带量词词位的报错清单（稳定顺序，供文案点名）。
const condxRepeatableHint = "标签/作者/集名"

// condxRepeatableEn 为允许多次出现（位次分立）的词根英文组名全集：标签/作者/
// 集名/标题/序号/作品号/日期/作品标识/描述——全部取值词各处独立取值（组名
// en__N 按位寻址）；{公共段}（key）与 {键} 维持单次出现拒绝；量词（+/*）仍
// 限 condxRepeatable 三词位，与多次出现全集解耦。
var condxRepeatableEn = map[string]bool{
	"tag": true, "author": true, "setname": true, "title": true,
	"seq": true, "workid": true, "date": true, "work": true, "description": true,
}

// condxPosName 位次化组名：位1 保持裸名 en（存量单出现零变化，决策6）；位 N≥2
// 为 en__N（双下划线+位次；组名合法字符 字母/数字/下划线，全组名均无下划线，
// 后缀无碰撞）。occur≤1 一律归位1（0=存量档案缺省）。
func condxPosName(en string, occur int) string {
	if occur <= 1 {
		return en
	}
	return fmt.Sprintf("%s__%d", en, occur)
}

// condxPosOccur 登记同词根出现并返回位次化组名（occ 为模板内出现计数，按
// tokens 从左到右出现序递增，跨 普通词/可选词位/重复段变体 统一计数）：可多次
// 出现全集（condxRepeatableEn）第 N 次出现即位 N；{公共段}/{键} 第二次出现
// 维持既有拒绝文案不变。
func condxPosOccur(occ map[string]int, word, en string) (string, error) {
	occ[en]++
	if occ[en] > 1 && !condxRepeatableEn[en] {
		return "", fmt.Errorf("模板占位词 {%s} 重复", word)
	}
	return condxPosName(en, occ[en]), nil
}

// condxRepeatWordLabels 为重复段词根英文组名到词表中文的映射（三面引用禁区
// ——生效条件 / 值校验 / 组合接线——的报错文案统一按词根中文渲染）。
var condxRepeatWordLabels = map[string]string{"tag": "标签", "author": "作者", "集名": "集名"}

// condxRepeatRefError 为重复段占位词引用禁区的统一中文文案：en 为词根英文
// 组名（标签→tag 一类），三面共用同一形态，保证文案口径一致。
func condxRepeatRefError(en string) error {
	label, ok := condxRepeatWordLabels[en]
	if !ok {
		label = en
	}
	return fmt.Errorf("「%s」为重复段占位词（多值产出），暂不能用于生效条件/值校验/组合接线", label)
}

// condxRepeatEnOfCapture 从行号前缀形态或裸名捕获名取词根英文组名
// （r1.tag → tag、r1.tag#canon → tag、r1.tag__2 → tag），供引用禁区文案渲染。
func condxRepeatEnOfCapture(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, "#canon")
	// 位次后缀剥离（tag__2 → tag）：位次分立后重复段捕获名可带 __N，文案仍按
	// 词根中文渲染。
	if i := strings.LastIndex(name, "__"); i >= 0 {
		name = name[:i]
	}
	return name
}

// condxVariant 为占位词内部量词变体（如 标签+、标签*、标签+;_）的词法解析
// 结果；包内可见，供任务B 消费生成正则。
type condxVariant struct {
	// En 为词根对应的英文捕获组名（标签→tag、作者→author、集名→setname）。
	En string
	// AtLeastOne 为量词档：true=至少一次（+），false=可零次（*）。
	AtLeastOne bool
	// SepSpecified 是否显式指定分隔字面（量词后以 ; 引出）。
	SepSpecified bool
	// Sep 为分隔字面原文：自首个分号起含分号本身（标签+; → ";"，标签+;_ → ";_"）。
	Sep string
}

// hasCondxVariantMark 报告占位词内部是否含变体标记（+、* 或 ;）：含标记才走
// 变体解析，否则维持既有单词路径，既有行为零扰动。
func hasCondxVariantMark(word string) bool {
	return strings.ContainsAny(word, "+*;")
}

// condxOptRoot 报告占位词内容是否 ? 后缀形态：是则返回剥去一个结尾 ? 的词根
// （{作者?} → 作者、{what??} → what?——可选字面自身以 ? 结尾用双写表达）。
// 词法无转义语法且不引入（2026-11-07 终审澄清1）：花括号外的 \ 与 ? 皆为
// 普通字面，存量模板含 \ 的含义不变；成对花括号文本无法按字面表达属既有边界。
func condxOptRoot(word string) (string, bool) {
	if !strings.HasSuffix(word, "?") {
		return "", false
	}
	return strings.TrimSuffix(word, "?"), true
}

// condxOptWordEn 返回 ? 后缀占位词的可选词位词根英文组名（词根在词表——含
// 任意/键/公共段，三者随后由各自拒绝面拦下）；可选分隔字面与空词根返回 false。
func condxOptWordEn(word string) (string, bool) {
	root, ok := condxOptRoot(word)
	if !ok || root == "" {
		return "", false
	}
	en, known := condxVocab[root]
	return en, known
}

// condxOptSepPlaceholder 报告占位词内容是否可选分隔字面形态（{字面?}）：非
// 变体、剥去一个结尾 ? 后非空且不在词表（2026-11-07 终审澄清2 的词根判定
// 顺序：先查词表命中=可选词位，未命中且非空=可选分隔字面）。
func condxOptSepPlaceholder(word string) bool {
	if hasCondxVariantMark(word) {
		return false
	}
	root, ok := condxOptRoot(word)
	if !ok || root == "" {
		return false
	}
	_, known := condxVocab[root]
	return !known
}

// condxHoldsTrailingOpen 报告后邻占位词是否需要暂扣前字面段收尾的开括号：
// 量词变体（既有）与可选词位（包裹组可选——开括号随词进退）两类；可选分隔
// 字面不包裹任何词，不暂扣。
func condxHoldsTrailingOpen(next string) bool {
	if hasCondxVariantMark(next) {
		return true
	}
	_, ok := condxOptWordEn(next)
	return ok
}

// condxOptSepAnchored 校验可选分隔字面（tokens[idx]，占位词内容 {字面?} 形态）
// 的位置约束（决策4）：左邻与右邻中至少一侧须为非空必选字面段或模板首/尾，
// 占位词不作锚。左邻取前一字面段的必选残留（0+ 包裹重复段/包裹可选词位把
// 起首闭括号收进可缺形态后残留为空，风险5 已知边界）；右邻取后一字面段的
// 必选残留前瞻（condxLitMandatoryAnchor）。相邻两可选分隔互不作锚——
// { ?}{_?} 一类天然被拒。
func condxOptSepAnchored(tokens []condxToken, idx int, lastLitMandatory string) bool {
	if idx > 0 && condxOptSepPlaceholder(tokens[idx-1].placeholder) {
		return false
	}
	if idx+1 < len(tokens) && condxOptSepPlaceholder(tokens[idx+1].placeholder) {
		return false
	}
	leftOK := idx == 0 || (tokens[idx-1].literal != "" && lastLitMandatory != "")
	rightOK := idx == len(tokens)-1 || (tokens[idx+1].literal != "" && condxLitMandatoryAnchor(tokens, idx+1))
	return leftOK || rightOK
}

// condxLitMandatoryAnchor 报告字面段 tokens[j] 的必选（非可缺）内容是否非空
// （作可选分隔的右邻锚）：普通字面段恒非空；收尾开括号将被后邻 0+ 包裹重复
// 段或包裹可选词位收进可缺形态时，必选残留=去开括号（空则不作锚）；1+ 档与
// 裸词形态的开括号按字面回放、仍必选。变体解析失败时保守作锚（由变体分支
// 自身报错）。
func condxLitMandatoryAnchor(tokens []condxToken, j int) bool {
	rs := []rune(tokens[j].literal)
	if j+1 >= len(tokens) || len(rs) == 0 || condxOpenBrackets[rs[len(rs)-1]] == 0 {
		return true
	}
	openCh := rs[len(rs)-1]
	closeCh := condxOpenBrackets[openCh]
	nextNext := ""
	if j+2 < len(tokens) {
		nextNext = tokens[j+2].literal
	}
	wrapped := strings.HasPrefix(nextNext, string(closeCh))
	if hasCondxVariantMark(tokens[j+1].placeholder) {
		v, verr := parseCondxVariant(tokens[j+1].placeholder)
		if verr != nil || v.AtLeastOne || !wrapped {
			// 1+ 档与裸词档：暂扣开括号原样回放，仍必选；解析错误由变体分支报错。
			return true
		}
		return len(rs) > 1 // 0+ 包裹档收进可缺形态：必选残留=去开括号。
	}
	if _, ok := condxOptWordEn(tokens[j+1].placeholder); ok && wrapped {
		return len(rs) > 1 // 包裹可选词位收进可缺形态：必选残留=去开括号。
	}
	return true // 非包裹形态：开括号按字面回放，仍必选。
}

// parseCondxVariant 解析占位词内部的量词变体：词根 + 量词（+ 或 *）+ 可选
// 分隔字面（; 引出，取首个分号起的全部、含分号本身）。仅 标签/作者/集名
// 三词位可带量词；仅分隔无量词、量词后残留非分号开头、未知词根与越权词位
// 均返回中文报错。
func parseCondxVariant(word string) (condxVariant, error) {
	var v condxVariant
	q := strings.IndexAny(word, "+*")
	semi := strings.Index(word, ";")
	if q < 0 || (semi >= 0 && semi < q) {
		// 含 ; 触发了变体解析，但量词缺失（标签;_）或位于分号之后（标签;+）。
		return v, fmt.Errorf("模板占位词 {%s} 语法错误：分隔字面须跟在量词（+ 或 *）之后", word)
	}
	root, rest := word[:q], word[q+1:]
	if rest != "" && !strings.HasPrefix(rest, ";") {
		// 量词后残留非分号开头（如 标签++、标签+x）。
		return v, fmt.Errorf("模板占位词 {%s} 语法错误：量词后只能跟分号引出的分隔字面", word)
	}
	en, known := condxVocab[root]
	if !known {
		return v, fmt.Errorf("模板占位词 {%s} 的词根 {%s} 不在词表（%s）", word, root, condxVocabHint)
	}
	if !condxRepeatable[root] {
		return v, fmt.Errorf("模板占位词 {%s} 的词根 {%s} 不可重复：仅 %s 可带量词（+ 或 *）", word, root, condxRepeatableHint)
	}
	v.En = en
	v.AtLeastOne = word[q] == '+'
	if rest != "" {
		v.SepSpecified = true
		v.Sep = rest
	}
	return v, nil
}

// condxRepeatDesc 为重复段（{标签+} 一类量词占位词）的推导描述：整段在主正则
// 中是恰一个位置捕获组（非命名——SubexpNames 该组名为空串，不进推导出的命名
// 捕获组名清单；声明层由 declaredConditionRepeatCaptures 显式补入并打标，供
// 三面引用禁区识别）；绑定层（阶段2）按 GroupIndex 取整段匹配文本，再用
// UnitPattern 再拆出各单元值。
type condxRepeatDesc struct {
	// En 为词根英文组名（标签→tag）。仅作描述标识，不进推导命名组清单
	//（声明层显式补入）。位次分立后可带 __N 后缀（同词根第 N 次出现的
	// 重复段，如 tag__2）——扇出绑定与三面禁引声明名随之位次化。
	En string
	// GroupIndex 为整段位置捕获组在模板正则中的序号（1 起，含全部命名组）。
	GroupIndex int
	// UnitPattern 为单元子模式原文：含恰一个捕获组圈住单元值（带包裹为
	// 「开括号(值类)闭括号」，裸词为「(值类)」），供 FindAllStringSubmatchIndex 再拆。
	UnitPattern string
	// Exclude 为单元值类排除集原文（未转义）：带包裹=配对闭括号，裸词=分隔
	// 字面的全部字符（去重、按首现顺序）。
	Exclude string
}

// condxDistinctRunes 返回字符串的去重 rune 序列（按首现顺序）。
func condxDistinctRunes(s string) string {
	var sb strings.Builder
	seen := make(map[rune]bool)
	for _, r := range s {
		if seen[r] {
			continue
		}
		seen[r] = true
		sb.WriteRune(r)
	}
	return sb.String()
}

// condxClassEscape 转义字符在方括号字符类中的特殊字符（\ ] ^ -），使分隔
// 字面字符可安全并入排除类。
func condxClassEscape(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\]^-`, r) {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// deriveState 为模板推导的跨段共享状态（多段目录模板）：出现计数（位次分立，
// 决策2 跨段连续编号——整条模板一次出现序流）与 {作品标识}×{作品号} 互斥登记
// 均按整条模板计；单段推导不共享（每次新建），行为与既有完全一致。
type deriveState struct {
	occ        map[string]int
	sawWork    bool
	sawWorkID  bool
}

// deriveCondxTemplate 把占位词模板推导为锚定正则：字面段转义；数字类 {日期}
// 取 [0-9]+、字母数字类（序号/作品号）取 [0-9A-Za-z]+；前字面段以开括号收尾
// 的占位词取排除配对
// 闭括号的非空类；其余占位词取惰性段 .+?（2026-11-04 终审拍板：填充段
// 让位一切占位词——leftmost-first 下惰性段取最短，后邻类词/重复段先取
// 满足位，见函数体内注释）；末位 {任意} 取可空吞位段，括号包裹的 {任意}
// 取排除配对闭括号的非空类（2026-11-04 终审修复批：括号字面为锚，通配段
// 不跨括号组吞并——前邻惰性 span 赖此收界）。量词变体占位词（{标签+} 一类）
// 推导为重复段：整段为恰一个位置捕获组（非命名、组内全非捕获）、单元值类为
// 排除类（带包裹排除配对闭括号、裸词排除分隔字面全部字符）、分隔缺省为可选
// 空白（仅限带包裹）、0+ 档整段含包裹括号外套可缺形态。? 后缀记号（相似
// 模板可选分隔表达方案）：可选分隔字面 {字面?} 编译 (?:字面)?（QuoteMeta，
// 受位置约束——两侧至少一侧非空必选字面段或模板首/尾）、可选词位 {词?} 编译
// (?:(?P<组名>捕获类))?（捕获类沿用三分支；包裹判定复用量词变体的暂扣开
// 括号/闭括号去重机制，开括号+词+闭括号整组同进退）。同词根多次出现（位次
// 分立，2026-11-08 方案）：可多次出现全集（标签/作者/集名/标题/序号/作品号/
// 日期/作品标识/描述）第 N 次出现即位 N（tokens 从左到右
// 出现序，跨 普通词/可选词位/重复段变体 统一计数）——位1 保持裸组名（存量单
// 出现零变化）、位 N≥2 组名 en__N；{公共段}/{键} 第二次出现维持既有拒绝；
// {作品标识} 与 {作品号} 同模板并用拒绝（语义近邻防混用）。返回表达式
// 原文、捕获组名列表、重复段描述列表与可选词位捕获组名列表；词表外/重复/
// 相邻占位词与非法重复段返回中文错误。
func deriveCondxTemplate(tpl string, opts deriveOpts) (string, []string, []condxRepeatDesc, []string, error) {
	return deriveCondxTemplateState(tpl, opts, nil)
}

// deriveCondxTemplateState 为带共享推导状态的推导入口：st 非 nil 时出现计数
// 与 {作品标识}×{作品号} 互斥登记跨段延续（多段目录模板逐段推导共用一址——
// 同一占位词跨段出现按整条模板出现序连续编号，决策2）；st 为 nil 时新建，
// 单段行为与既有完全一致。
func deriveCondxTemplateState(tpl string, opts deriveOpts, st *deriveState) (string, []string, []condxRepeatDesc, []string, error) {
	if strings.TrimSpace(tpl) == "" {
		return "", nil, nil, nil, fmt.Errorf("格式模板不能为空")
	}
	tokens, err := tokenizeCondx(tpl)
	if err != nil {
		return "", nil, nil, nil, err
	}
	if st == nil {
		st = &deriveState{occ: make(map[string]int)}
	}
	// 同词根出现计数（位次分立）：可多次出现全集放行、组名按位次生成；
	// {公共段}/{键} 第二次出现维持既有拒绝（condxPosOccur）。occ 住共享
	// 状态——多段共享时跨段连续编号。
	occ := st.occ
	// {作品标识} 与 {作品号} 同模板互斥登记：整词标识 vs 编号段语义近邻，并用
	// 易混用——普通词与可选词位两种书写面统一经此裁决（多段共享时按整条
	// 模板计）。
	noteIDPair := func(word, en string) error {
		switch en {
		case "work":
			if st.sawWorkID {
				return fmt.Errorf("模板占位词 {%s} 不可与 {作品号} 同模板并用：{作品标识} 取整词标识、{作品号} 取编号段，语义近邻易混用，请按文件名形态择一", word)
			}
			st.sawWork = true
		case "workid":
			if st.sawWork {
				return fmt.Errorf("模板占位词 {%s} 不可与 {作品标识} 同模板并用：{作品标识} 取整词标识、{作品号} 取编号段，语义近邻易混用，请按文件名形态择一", word)
			}
			st.sawWorkID = true
		}
		return nil
	}
	var captures []string
	var optCaptures []string // 可选词位捕获组名（序列模板成套键的排除集）
	var repeats []condxRepeatDesc
	var sb strings.Builder
	sb.WriteString("^")
	hasSeq := false
	prevPlaceholder := false
	pendingCloser := rune(0)
	captureIdx := 0
	heldOpen := rune(0)
	skipCloserHead := false
	lastLitMandatory := ""
	for i, tok := range tokens {
		if tok.literal != "" {
			lit := tok.literal
			if skipCloserHead {
				// 0+ 包裹重复段/包裹可选词位已把后字面段起首的配对闭括号收进
				// 可缺形态，去重。
				lit = string([]rune(lit)[1:])
				skipCloserHead = false
			}
			heldOpen = rune(0)
			if i+1 < len(tokens) && condxHoldsTrailingOpen(tokens[i+1].placeholder) {
				// 后邻量词变体/可选词位：暂扣字面段收尾开括号——0+ 包裹重复档
				// 与包裹可选词位把它收进可缺形态（整组同进退），其余形态原样回放。
				if rs := []rune(lit); len(rs) > 0 && condxOpenBrackets[rs[len(rs)-1]] != 0 {
					heldOpen = rs[len(rs)-1]
					lit = string(rs[:len(rs)-1])
				}
			}
			if lit != "" {
				sb.WriteString(regexp.QuoteMeta(lit))
			}
			lastLitMandatory = lit
			prevPlaceholder = false
			pendingCloser = tailClosingBracket(lit)
			continue
		}
		word := tok.placeholder
		// 可选分隔字面（{字面?}）自身即分隔：豁免相邻占位词检查（改受位置
		// 约束 condxOptSepAnchored）；其余占位词（含可选词位）照常受检。
		if !condxOptSepPlaceholder(word) {
			if prevPlaceholder {
				return "", nil, nil, nil, fmt.Errorf("相邻占位词 {%s} 与前一个占位词之间缺少分隔文字", word)
			}
			prevPlaceholder = true
		}
		if hasCondxVariantMark(word) {
			// 重复段表达（任务B）：量词变体占位词不走词表单词路径。整段为恰
			// 一个位置捕获组（非命名——SubexpNames 该组名为空串，不进推导出的
			// 命名捕获组名清单；声明层另有显式补入与打标），词根组名只进重复段
			// 描述、不进命名捕获组名清单。
			v, verr := parseCondxVariant(word)
			if verr != nil {
				return "", nil, nil, nil, verr
			}
			if opts.requireSeq {
				return "", nil, nil, nil, fmt.Errorf("模板占位词 {%s} 不可重复：序列成员格式不支持重复段", word)
			}
			// 位次分立：重复段变体计入出现序（词根恒为三词位——parseCondxVariant
			// 已拒绝越权词位，多次出现恒放行）。
			posName, perr := condxPosOccur(occ, word, v.En)
			if perr != nil {
				return "", nil, nil, nil, perr
			}
			// 包裹判定：前字面段收尾开括号且后字面段起首为其配对闭括号（六对
			// 同型括号映射）→ 包裹单元；否则裸词。
			nextLit := ""
			if i+1 < len(tokens) {
				nextLit = tokens[i+1].literal
			}
			nextRs := []rune(nextLit)
			wrapped := heldOpen != 0 && len(nextRs) > 0 && nextRs[0] == condxOpenBrackets[heldOpen]
			var desc condxRepeatDesc
			// 位次写入单点构造点：desc.En 带位次后缀，扇出绑定/值校验拒绝集/
			// 三面禁引声明名随组名后缀自动生效（rule_condition_subject.go 零改）。
			desc.En = posName
			if wrapped {
				openCh, closeCh := heldOpen, condxOpenBrackets[heldOpen]
				openRe, closeRe := regexp.QuoteMeta(string(openCh)), regexp.QuoteMeta(string(closeCh))
				valueClass := "[^" + string(closeCh) + "]+"
				sepRe := `\s*`
				if v.SepSpecified {
					sepRe = regexp.QuoteMeta(v.Sep[1:])
				}
				captureIdx++
				desc.GroupIndex = captureIdx
				desc.UnitPattern = openRe + "(" + valueClass + ")" + closeRe
				desc.Exclude = string(closeCh)
				// 单元核：首单元值 +（闭括号+分隔+开括号+后续值）*——中间单元
				// 括号进组内；1+ 档首开括号由前字面段承载、末闭括号由后字面段
				// 承载（括号留在组外字面段），0+ 档整段含包裹括号外套可缺形态。
				group := "(" + valueClass + "(?:" + closeRe + sepRe + openRe + valueClass + ")*)"
				if v.AtLeastOne {
					sb.WriteString(openRe + group)
				} else {
					sb.WriteString("(?:" + openRe + group + closeRe + ")?")
					skipCloserHead = true
				}
			} else {
				if heldOpen != 0 {
					// 暂扣的开括号不属裸词重复段，原样回放。
					sb.WriteString(regexp.QuoteMeta(string(heldOpen)))
				}
				if !v.SepSpecified {
					return "", nil, nil, nil, fmt.Errorf("模板占位词 {%s} 缺分隔字面：无包裹括号的重复词没有单元边界（贪婪并合），请用分号指定分隔（如 {%s;_}）或改用包裹括号成段（如 [{%s}]）", word, word, word)
				}
				sepLit := v.Sep[1:]
				if sepLit == "" {
					return "", nil, nil, nil, fmt.Errorf("模板占位词 {%s} 语法错误：分号引出的分隔字面不能为空", word)
				}
				sepRe := regexp.QuoteMeta(sepLit)
				exclude := condxDistinctRunes(sepLit)
				valueClass := "[^" + condxClassEscape(exclude) + "]+"
				captureIdx++
				desc.GroupIndex = captureIdx
				desc.UnitPattern = "(" + valueClass + ")"
				desc.Exclude = exclude
				group := "(" + valueClass + "(?:" + sepRe + valueClass + ")*)"
				if v.AtLeastOne {
					sb.WriteString(group)
				} else {
					sb.WriteString("(?:" + group + ")?")
				}
			}
			heldOpen = rune(0)
			repeats = append(repeats, desc)
			pendingCloser = rune(0)
			continue
		}
		if optRoot, isOpt := condxOptRoot(word); isOpt {
			// ? 后缀记号（相似模板可选分隔表达方案，阶段1）：判定居量词变体
			// 之后；词根先查词表（命中=可选词位），未命中且非空=可选分隔字面。
			if optRoot == "" {
				return "", nil, nil, nil, fmt.Errorf("模板占位词 {%s}：问号前无可选内容", word)
			}
			if en, known := condxVocab[optRoot]; known {
				// 可选词位 {词?}：走普通命名组通路（三面引用可用，引用权由
				// 书写记号静态决定，与 {词*} 多值通路正交）；文件/目录/序列
				// 模板同款可用——序列模板的可选捕获不进成套键（进键则在场/
				// 缺席成员键不同必拆套，空键拦截见函数尾守卫）；缺席=无绑定。
				if optRoot == "任意" {
					return "", nil, nil, nil, fmt.Errorf("模板占位词 {%s}：吞位词 {任意} 不产出声明、无可选语义，不可带问号", word)
				}
				if optRoot == "键" || optRoot == "公共段" {
					return "", nil, nil, nil, fmt.Errorf("模板占位词 {%s}：{公共段} 是序列成套的成群依据，不可带问号", word)
				}
				if err := noteIDPair(word, en); err != nil {
					return "", nil, nil, nil, err
				}
				posName, perr := condxPosOccur(occ, word, en)
				if perr != nil {
					return "", nil, nil, nil, perr
				}
				captures = append(captures, posName)
				optCaptures = append(optCaptures, posName)
				captureIdx++
				nextLit := ""
				if i+1 < len(tokens) {
					nextLit = tokens[i+1].literal
				}
				nextRs := []rune(nextLit)
				if heldOpen != 0 && len(nextRs) > 0 && nextRs[0] == condxOpenBrackets[heldOpen] {
					// 包裹词可选：开括号+词+闭括号整组同进退（复用量词变体的
					// 暂扣开括号与闭括号去重机制），编译 (?:开(?P<词>[^闭]+)闭)?
					// ——括号随词同缺席，不产「空括号命中」。
					closeCh := condxOpenBrackets[heldOpen]
					sb.WriteString("(?:" + regexp.QuoteMeta(string(heldOpen)) + "(?P<" + posName + ">[^" + string(closeCh) + "]+)" + regexp.QuoteMeta(string(closeCh)) + ")?")
					skipCloserHead = true
				} else {
					if heldOpen != 0 {
						// 暂扣的开括号不配对（后字面段非配对闭括号起首）：按字面
						// 回放（同量词变体的开括号回放逻辑），词按裸词可选。
						sb.WriteString(regexp.QuoteMeta(string(heldOpen)))
					}
					cls := ".+?"
					switch {
					case condxAlnum[optRoot]:
						cls = "[0-9A-Za-z]+"
					case condxDigits[optRoot]:
						cls = "[0-9]+"
					case heldOpen != 0:
						cls = "[^" + string(condxOpenBrackets[heldOpen]) + "]+"
					case pendingCloser != 0:
						cls = "[^" + string(pendingCloser) + "]+"
					}
					sb.WriteString("(?:(?P<" + posName + ">" + cls + "))?")
				}
				heldOpen = rune(0)
				pendingCloser = rune(0)
				continue
			}
			// 可选分隔字面（{ ?} 可选空格、{_?} 可选下划线、{what??} 可选
			// what?）：编译 (?:字面)?（QuoteMeta）；位置约束=决策4（两侧至少
			// 一侧字面锚）。
			if !condxOptSepAnchored(tokens, i, lastLitMandatory) {
				return "", nil, nil, nil, fmt.Errorf("可选分隔两侧均无字面锚：两取值词之间的可选分隔在无分隔形态下切分位置任意，请将一侧落在字面（如括号或固定前后缀）后使用")
			}
			sb.WriteString("(?:" + regexp.QuoteMeta(optRoot) + ")?")
			prevPlaceholder = false
			continue
		}
		en, known := condxVocab[word]
		if !known {
			return "", nil, nil, nil, fmt.Errorf("模板占位词 {%s} 不在词表（%s）", word, condxVocabHint)
		}
		if err := noteIDPair(word, en); err != nil {
			return "", nil, nil, nil, err
		}
		if (word == "键" || word == "公共段") && !opts.allowKey {
			return "", nil, nil, nil, fmt.Errorf("模板占位词 {公共段} 仅适用于序列成员格式")
		}
		if word == "任意" {
			// 非引用吞位词：非捕获吞位段、可重复、不进捕获组清单（提取
			// 引用 any 会被未声明捕获名校验拒绝——词法天然保证只作消费）。
			// 末位吞位段可空——[{词}]{任意} 一类模板须同时命中「首组+尾」
			// 与「单组整名（尾空）」两类形态；中位吞位段保持非空。
			// 中位吞位段惰性化（2026-11-04 终审拍板）：贪婪版会抢占后邻
			// 类词/重复段的满足位，与 default 填充段让位原则同源。
			// 括号包裹的 {任意}（前字面段以开括号收尾且后字面段以其配对
			// 闭括号起首）编译为排除该闭括号的非空类（2026-11-04 终审
			// 修复批）：旧通配段可跨括号组吞并，与填充段抢占同族缺陷；
			// 类化后前邻惰性 span 经回溯扩到预期区间（workset 目录段
			// 组合「{集名} [{任意}]」形态依赖此锚定收界）。同对嵌套在
			// 首个闭括号处截断，与括号排除捕获类同口径（已知词法边界）。
			nextLit := ""
			if i+1 < len(tokens) {
				nextLit = tokens[i+1].literal
			}
			if pendingCloser != 0 && strings.HasPrefix(nextLit, string(pendingCloser)) {
				sb.WriteString("(?:[^" + string(pendingCloser) + "]+)")
				continue
			}
			if i == len(tokens)-1 {
				sb.WriteString("(?:.*)")
			} else {
				sb.WriteString("(?:.+?)")
			}
			continue
		}
		posName, perr := condxPosOccur(occ, word, en)
		if perr != nil {
			return "", nil, nil, nil, perr
		}
		captures = append(captures, posName)
		captureIdx++
		if word == "序号" {
			hasSeq = true
		}
		sb.WriteString("(?P<" + posName + ">")
		switch {
		case condxAlnum[word]:
			sb.WriteString("[0-9A-Za-z]+")
		case condxDigits[word]:
			sb.WriteString("[0-9]+")
		case pendingCloser != 0:
			// 括号感知捕获类：前字面段以开括号收尾时排除配对闭括号——括号内
			// 名不跨界吞并后续括号组，多组名对 [{词}] 宁缺不命中，首组表达
			// 由末位 {任意} 吞尾承载（[{词}]{任意}）。字符级排除无计数能力：
			// 同对嵌套（如 [a[b]]）在首个闭括号处截断、捕获 a[b，与组界扫描
			// 的最外层组界分歧，属已知词法边界。
			sb.WriteString("[^" + string(pendingCloser) + "]+")
		default:
			// 惰性段 .+?（2026-11-04 终审拍板：填充段让位一切占位词）：
			// leftmost-first 下取最短，后邻字面段/类词/重复段先取满足位——
			// 值可含下划线等分隔符（如 kpop_dancer_wg），多值并合时由后邻
			// 结构吞余段；尾随类词（字母数字类/日期数字类，类锚定不惰性化）
			// 仍经回溯取到尾数（{标题}_{序号} 对 A_B_123 → 序号=123）。
			sb.WriteString(".+?")
		}
		sb.WriteString(")")
	}
	if opts.requireSeq && !hasSeq {
		return "", nil, nil, nil, fmt.Errorf("序列成员格式必须包含 {序号}")
	}
	// 序列模板空键守卫：可选词位捕获不进成套键，{序号} 之外再无必选取值词时
	// 键恒为空串——同目录内一切命中文件同键并套。可选词位此前在序列模板整体
	// 被拒，存量档案不含此形态，守卫只拦含可选词位的新书写；{公共段} 一类
	// 必选键段词照常满足守卫。
	if opts.requireSeq && len(optCaptures) > 0 {
		optSet := make(map[string]bool, len(optCaptures))
		for _, name := range optCaptures {
			optSet[name] = true
		}
		hasMandatoryKeyWord := false
		for _, name := range captures {
			if !optSet[name] && condxRepeatEnOfCapture(name) != "seq" {
				hasMandatoryKeyWord = true
				break
			}
		}
		if !hasMandatoryKeyWord {
			return "", nil, nil, nil, fmt.Errorf("序列成员格式的成套键为空：可选词位不参与成套键，{序号} 之外须至少保留一个必选取值词（或 {公共段}），否则同目录内所有命中文件会并成一套")
		}
	}
	sb.WriteString("$")
	return sb.String(), captures, repeats, optCaptures, nil
}

// dirTplSegment 多段目录模板的单段推导产物：段锚定正则（^…$，匹配单个目录
// 名）与该段捕获/重复段——绑定按段进行，各段命中各自目录名后逐段
// bindCondxCaptures。
type dirTplSegment struct {
	expr     string
	captures []string
	repeats  []condxRepeatDesc
}

// dirSegGap 段距（两段之间的层距，裁决④）：Any=true 为「之下任意层」（//）；
// 否则 N≥1 显式层距（/{N}；「/」=1 紧邻，缺省形态）。层距 0 不可书写。
type dirSegGap struct {
	Any bool `json:"any,omitempty"`
	N   int  `json:"n,omitempty"`
}

// splitDirTplSegments 把目录模板按分隔符切分为段序列与段距序列（决策1 形态：
// `/`=紧邻〔N=1〕、`//`=任意层距、`/{N}`〔花括号纯数字，N≥1〕=显式层距）。
// 边界条件：首段前/末段后不得有分隔符；空段拒绝（相邻分隔符之间没有目录段，
// 三个及以上连斜杠同理）；层距 0 不可书写、非纯数字花括号不作层距记号（回到
// 段内词法，由既有词表校验处置）。单段（无分隔符）返回单段切片、段距序列
// 为空——调用方走现行单段路径零变化。
func splitDirTplSegments(tpl string) ([]string, []dirSegGap, error) {
	var segs []string
	var gaps []dirSegGap
	start := 0
	i := 0
	for i < len(tpl) {
		if tpl[i] != '/' {
			i++
			continue
		}
		// 分隔符候选：吃掉连续斜杠（>2 即夹空段）。
		j := i
		for j < len(tpl) && tpl[j] == '/' {
			j++
		}
		slashes := j - i
		if slashes > 2 {
			return nil, nil, fmt.Errorf("目录模板在 %d 个连斜杠处存在空段：相邻分隔符之间没有目录名", slashes)
		}
		gap := dirSegGap{N: 1}
		k := j
		if slashes == 1 && j < len(tpl) && tpl[j] == '{' {
			// /{N} 显式层距：花括号内纯数字才作层距记号，否则 `/` 按紧邻、
			// 花括号内容留给段内词法（词表校验照常处置）。
			m := j + 1
			for m < len(tpl) && tpl[m] >= '0' && tpl[m] <= '9' {
				m++
			}
			if m > j+1 && m < len(tpl) && tpl[m] == '}' {
				n, aerr := strconv.Atoi(tpl[j+1 : m])
				if aerr != nil || n < 1 {
					return nil, nil, fmt.Errorf("目录模板层距记号 /{%s} 无效：层距须为 ≥1 的整数（紧邻写 /、之下任意层写 //）", tpl[j+1:m])
				}
				gap.N = n
				k = m + 1
			}
		}
		if slashes == 2 {
			gap = dirSegGap{Any: true}
		}
		if i == 0 {
			return nil, nil, fmt.Errorf("目录模板不得以分隔符开头：首段之前没有目录名")
		}
		if tpl[start:i] == "" {
			return nil, nil, fmt.Errorf("目录模板存在空段：相邻分隔符之间没有目录名")
		}
		if k >= len(tpl) {
			return nil, nil, fmt.Errorf("目录模板不得以分隔符结尾：末段之后没有目录名")
		}
		segs = append(segs, tpl[start:i])
		gaps = append(gaps, gap)
		start = k
		i = k
	}
	last := tpl[start:]
	if len(segs) == 0 {
		return []string{last}, nil, nil
	}
	segs = append(segs, last)
	return segs, gaps, nil
}

// deriveDirTplSegments 多段目录模板逐段推导（段内词表/量词/可选词位/惰性段
// 语义原样）：共享一次出现序流（决策2——同一占位词跨段出现按整条模板出现序
// 连续编号，段内出现序与段序拼接即整条书写序），返回段模式序列+段距序列。
// 单段模板等价于 deriveCondxTemplate。
func deriveDirTplSegments(tpl string) ([]dirTplSegment, []dirSegGap, error) {
	segs, gaps, err := splitDirTplSegments(tpl)
	if err != nil {
		return nil, nil, err
	}
	st := &deriveState{occ: make(map[string]int)}
	out := make([]dirTplSegment, 0, len(segs))
	for _, seg := range segs {
		expr, captures, repeats, _, err := deriveCondxTemplateState(seg, deriveOpts{}, st)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, dirTplSegment{expr: expr, captures: captures, repeats: repeats})
	}
	return out, gaps, nil
}

// deriveCondxContains 把包含词推导为锚定正则（含这个词：任意位置字面包含）。
func deriveCondxContains(word string) (string, error) {
	if word == "" {
		return "", fmt.Errorf("包含词不能为空")
	}
	return "^.*" + regexp.QuoteMeta(word) + ".*$", nil
}

// condxCapturesOf 返回条件格式段推导出的捕获组英文名列表与重复段描述列表
// （重复段机械透传给调用方，语义消费在阶段2 绑定层）。目录目标的模板可多段
// （决策1）：捕获名与重复段按段序拼接（跨段出现序连续编号——决策2），供
// 提取/when/值校验按整条模板口径引用（如 r1.tag__2 取第 2 段的第 2 处）。
func condxCapturesOf(c *Condx) ([]string, []condxRepeatDesc, error) {
	if c.Format.Mode == CondxFormatContains {
		return nil, nil, nil
	}
	if c.Target == CondxTargetDir {
		segs, _, err := deriveDirTplSegments(c.Format.Tpl)
		if err != nil {
			return nil, nil, err
		}
		var captures []string
		var repeats []condxRepeatDesc
		for _, s := range segs {
			captures = append(captures, s.captures...)
			repeats = append(repeats, s.repeats...)
		}
		return captures, repeats, nil
	}
	_, captures, repeats, _, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{allowKey: c.Target == CondxTargetSeq})
	return captures, repeats, err
}

// validateCondx 校验单条条件：目标合法、格式可推导、值校验引用存在、终结谓词
// 属目标封闭集且必填字段齐备、主体锚为 nil。
func validateCondx(c *Condx) error {
	switch c.Target {
	case CondxTargetFile, CondxTargetDir, CondxTargetSeq:
	default:
		return fmt.Errorf("未知的目标类别: %q（file / dir / seq）", string(c.Target))
	}
	if c.Anchor != nil {
		return fmt.Errorf("主体锚（基于其他条件）为二期能力，当前档案不可用")
	}
	if c.Format.Mode == "" {
		c.Format.Mode = CondxFormatTemplate
	}
	switch c.Format.Mode {
	case CondxFormatTemplate:
		if c.Target == CondxTargetDir {
			// 目录目标：多段模板（决策1）——段界合法性（首尾/空段/层距值域）
			// 与逐段推导共用一址（单段时与既有校验等价，零变化）。
			if _, _, err := deriveDirTplSegments(c.Format.Tpl); err != nil {
				return fmt.Errorf("格式模板无效: %w", err)
			}
		} else {
			// 文件/序列目标：文件名不含「/」，含之即恒不命中的死写法——
			// 保存期拒绝（自决3，静默死形态升级校验期拒绝）。
			if strings.Contains(c.Format.Tpl, "/") {
				return fmt.Errorf("格式模板不可含「/」：层级表达属目录行，文件名里没有这个字符，写了永不命中")
			}
			if _, _, _, _, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{
				allowKey:   c.Target == CondxTargetSeq,
				requireSeq: c.Target == CondxTargetSeq,
			}); err != nil {
				return fmt.Errorf("格式模板无效: %w", err)
			}
		}
	case CondxFormatContains:
		if c.Target == CondxTargetSeq {
			return fmt.Errorf("序列目标只支持整条按模板（contains 问法不适用）")
		}
		if c.Format.Word == "" {
			return fmt.Errorf("包含词不能为空")
		}
		// 包含词按字面匹配单个目录名/文件名，含「/」恒不命中——保存期拒绝
		//（自决3：「含这个词」问法不多段）。
		if strings.Contains(c.Format.Word, "/") {
			return fmt.Errorf("包含词不可含「/」：目录名与文件名里都没有这个字符，写了永不命中")
		}
	default:
		return fmt.Errorf("未知的格式模式: %q（template / contains）", string(c.Format.Mode))
	}
	captures, repeats, _ := condxCapturesOf(c)
	captureSet := make(map[string]bool, len(captures))
	for _, en := range captures {
		captureSet[en] = true
	}
	repeatSet := make(map[string]bool, len(repeats))
	for _, rp := range repeats {
		repeatSet[rp.En] = true
	}
	for i, b := range c.Badges {
		en, known := condxVocab[b.Placeholder]
		if !known {
			return fmt.Errorf("第 %d 个值校验的占位词 {%s} 不在词表（%s）", i+1, b.Placeholder, condxVocabHint)
		}
		if b.Occur < 0 {
			return fmt.Errorf("第 %d 个值校验的位次 %d 无效（位次从 1 起）", i+1, b.Occur)
		}
		// 值校验位次化：按位次生成寻址名（位1 裸组名=存量零变化；位 N≥2 带
		// __N 后缀），captureSet/repeatSet 键同为位次名、按位判定。
		name := condxPosName(en, b.Occur)
		// 重复段占位词多值产出（单元值序列），值校验（库名单/相等/常量）
		// 的单一值断言无定义——三断言的共同构造入口在此统一拒绝（按位）。
		if repeatSet[name] {
			return fmt.Errorf("第 %d 个值校验: %w", i+1, condxRepeatRefError(condxRepeatEnOfCapture(name)))
		}
		if !captureSet[name] {
			return fmt.Errorf("值校验 {%s} 引用的占位词不在本行格式模板内", b.Placeholder)
		}
		switch b.Kind {
		case CondxBadgeLibrary:
			switch b.Dim {
			case "author", "tag", "collection":
			default:
				return fmt.Errorf("值校验 {%s} 的名单维度无效: %q（author / tag / collection）", b.Placeholder, b.Dim)
			}
		case CondxBadgeEqual:
			if b.AgainstPh == "" {
				return fmt.Errorf("值校验 {%s} 的相等对象占位词不能为空", b.Placeholder)
			}
			if _, ok := condxVocab[b.AgainstPh]; !ok {
				return fmt.Errorf("值校验 {%s} 的相等对象 {%s} 不在词表", b.Placeholder, b.AgainstPh)
			}
			if b.AgainstOccur < 0 {
				return fmt.Errorf("值校验 {%s} 的相等对象位次 %d 无效（位次从 1 起）", b.Placeholder, b.AgainstOccur)
			}
		case CondxBadgeConstant:
			if b.Value == "" {
				return fmt.Errorf("值校验 {%s} 的常量值不能为空", b.Placeholder)
			}
		default:
			return fmt.Errorf("未知的值校验类别: %q（library / equal / constant）", string(b.Kind))
		}
	}
	allowed := attachAllowed[c.Target]
	for i, a := range c.Attach {
		if !allowed[a.Kind] {
			return fmt.Errorf("第 %d 个附加条件 %q 不适用于目标 %q", i+1, a.Kind, string(c.Target))
		}
		if attachNeedsTpl[a.Kind] {
			if _, _, _, _, err := deriveCondxTemplate(a.Tpl, deriveOpts{allowKey: true}); err != nil {
				return fmt.Errorf("附加条件 %q 的迷你模板无效: %w", a.Kind, err)
			}
		}
		if attachNeedsWord[a.Kind] && a.Word == "" {
			return fmt.Errorf("附加条件 %q 的包含词不能为空", a.Kind)
		}
		if attachNeedsCount[a.Kind] && a.MinCount < 1 {
			return fmt.Errorf("附加条件 %q 的数量门槛须 ≥1", a.Kind)
		}
		if a.Kind == CondxAttachFileExt && a.Value == "" {
			return fmt.Errorf("附加条件 fileExt 的扩展名不能为空")
		}
		if a.Kind == CondxAttachRefScope {
			// 字段族：Value=供给方规则 ID 或库根 sentinel（必填，见
			// RefScopeLibraryRoot）；MinCount=层距（0=任意深度哨兵、1=紧邻、
			// N≥2 显式——负数无义拒绝）。供给方的存在性/合格性（悬空/自引用/
			// 链式/纯目录）属跨规则校验，见 rule_profile.go 的
			// validateRefScopeWiring（第二遍；库根 sentinel 放行不判）。
			if a.Value == "" {
				return fmt.Errorf("附加条件 refScope 的供给方规则 ID 不能为空")
			}
			if a.MinCount < 0 {
				return fmt.Errorf("附加条件 refScope 的层距无效（0=任意深度、1=紧邻、N≥2=隔 N 级），得到 %d", a.MinCount)
			}
		}
		if a.Kind == CondxAttachChildTypeIs || a.Kind == CondxAttachSeqMemberType {
			if len(a.Types) == 0 {
				return fmt.Errorf("附加条件 %q 的类型列表不能为空", a.Kind)
			}
			for _, t := range a.Types {
				switch t {
				case "image", "video", "document", "archive":
				default:
					return fmt.Errorf("附加条件 %q 的类型无效: %q（image / video / document / archive）", a.Kind, t)
				}
			}
		}
	}
	return nil
}

// validateCondxRows 校验行列表：至少一行、首行无连接词、后续行连接词合法、
// 行内条件与子组二选一且其一非空、跨行值级相等的行号引用合法（**全局先序
// 展平编号**，对方行须早于本行——求值顺序依赖，R6；子组内行的编号接续父级
// 前缀，跨组引用按全局编号裁决）。
func validateCondxRows(rows []CondxRow) error {
	return validateCondxRowsAt(rows, 0)
}

// validateCondxRowsAt 在全局行号偏移 base 上递归校验（子组传入其先序前缀）。
func validateCondxRowsAt(rows []CondxRow, base int) error {
	if len(rows) == 0 {
		return fmt.Errorf("条件行列表不能为空")
	}
	total := base + countCondxRows(rows)
	prefix := 0
	for i := range rows {
		row := &rows[i]
		if i == 0 {
			if row.Connector != "" {
				return fmt.Errorf("首行不能带行前连接词: %q", row.Connector)
			}
		} else if row.Connector != "and" && row.Connector != "or" {
			return fmt.Errorf("第 %d 行的行前连接词无效: %q（and / or）", i+1, row.Connector)
		}
		if (row.Condition == nil) == (len(row.Group) == 0) {
			return fmt.Errorf("第 %d 行须为条件行或括号子组之一（非空）", i+1)
		}
		if row.Condition != nil {
			ordinal := base + prefix + 1
			if err := validateCondx(row.Condition); err != nil {
				return fmt.Errorf("第 %d 行: %w", ordinal, err)
			}
			for _, b := range row.Condition.Badges {
				if b.Kind != CondxBadgeEqual {
					continue
				}
				if b.AgainstRow < 1 || b.AgainstRow > total {
					return fmt.Errorf("第 %d 行: 值校验 {%s} 的对方行号 %d 越界（共 %d 行）",
						ordinal, b.Placeholder, b.AgainstRow, total)
				}
				if b.AgainstRow >= ordinal {
					return fmt.Errorf("第 %d 行: 值校验 {%s} 的对方行号 %d 须早于本行", ordinal, b.Placeholder, b.AgainstRow)
				}
			}
			prefix++
		}
		if len(row.Group) > 0 {
			if err := validateCondxRowsAt(row.Group, base+prefix); err != nil {
				return fmt.Errorf("第 %d 行子组: %w", base+prefix+1, err)
			}
			prefix += countCondxRows(row.Group)
		}
	}
	return nil
}

// countCondxRows 统计行列表的先序展平行数（含子组内各行）。
func countCondxRows(rows []CondxRow) int {
	n := 0
	for _, row := range rows {
		if row.Condition != nil {
			n++
		}
		if len(row.Group) > 0 {
			n += countCondxRows(row.Group)
		}
	}
	return n
}

// condxLeafRow 条件行树的先序展平叶子行（括号子组递归内层；行级非行照收）：
// row 为叶子行引用（含行级非标记），no 为全局行号（1 起先序编号，与
// compileCondxRow 的 counter 推进一致——组行占其组内首行号，后续行接续组内
// 展平计数；countCondxRows 即叶子行总数）。
type condxLeafRow struct {
	row *CondxRow
	no  int
}

// condxLeafRowsOf 条件行列表的先序展平叶子行序列（行号 1..N 连续）：括号
// 子组递归下潜、组结构不产叶子行（布尔组合由 aggregateBooleanGate 消费整条
// 求值）；供聚合/配对/内容绑定段的行级命中寻址（决策10：行级命中照常按行）。
func condxLeafRowsOf(rows []CondxRow) []condxLeafRow {
	var out []condxLeafRow
	var walk func(list []CondxRow)
	walk = func(list []CondxRow) {
		for i := range list {
			row := &list[i]
			if row.Condition != nil {
				out = append(out, condxLeafRow{row: row, no: len(out) + 1})
				continue
			}
			walk(row.Group)
		}
	}
	walk(rows)
	return out
}

// condxHasBooleanShape 报告条件行树是否含括号子组或行级非（决策10 放开的
// 布尔组合形态）：在场时聚合/配对段的成员判定在行命中池之上加整条布尔求值
// 门（命中集消费）；纯平行行（无括号无行级非——存量聚合/配对规则的现状
// 形态）不设门，维持行命中池 OR 汇聚的既有语义（连接词不参与成员判定）。
func condxHasBooleanShape(rows []CondxRow) bool {
	for i := range rows {
		if rows[i].Negated || len(rows[i].Group) > 0 {
			return true
		}
	}
	return false
}

// compileCondxRows 把行列表编译为模式树：且连接紧于或连接（或分支=按 or 切段，
// 段内 and 链），括号子组最紧，单子归一，行级非包裹整行——与前端行式底盘的
// 组合语义一致（前端 rowsToTree 的内核侧锚定实现）。行号（先序展平，1 起）
// 供行包装与值校验门引用；校验先行。
func compileCondxRows(rows []CondxRow) (Pattern, error) {
	if err := validateCondxRows(rows); err != nil {
		return nil, err
	}
	return compileCondxRowsAt(rows, 0)
}

// compileCondxRowsAt 在全局行号偏移 base 上递归编译。
func compileCondxRowsAt(rows []CondxRow, base int) (Pattern, error) {
	type chunk struct{ rows []CondxRow }
	var chunks []chunk
	current := chunk{}
	for i := range rows {
		if rows[i].Connector == "or" {
			chunks = append(chunks, current)
			current = chunk{}
		}
		current.rows = append(current.rows, rows[i])
	}
	chunks = append(chunks, current)
	compileChunk := func(c chunk, counter *int) (Pattern, error) {
		var subs []Pattern
		for i := range c.rows {
			p, err := compileCondxRow(&c.rows[i], counter)
			if err != nil {
				return nil, err
			}
			subs = append(subs, p)
		}
		if len(subs) == 1 {
			return subs[0], nil
		}
		return And(subs), nil
	}
	counter := base
	var branches []Pattern
	for _, c := range chunks {
		p, err := compileChunk(c, &counter)
		if err != nil {
			return nil, err
		}
		branches = append(branches, p)
	}
	if len(branches) == 1 {
		return branches[0], nil
	}
	return Or(branches), nil
}

// compileCondxRow 编译单行：条件或子组 → 模式（条件行为行包装主体）；行级非
// 包裹整行。counter 为全局行号游标（先序推进，与校验编号一致）。
func compileCondxRow(row *CondxRow, counter *int) (Pattern, error) {
	var p Pattern
	var err error
	if row.Condition != nil {
		*counter++
		p, err = compileCondx(row.Condition, *counter)
	} else {
		nested := countCondxRows(row.Group)
		p, err = compileCondxRowsAt(row.Group, *counter)
		if err == nil {
			*counter += nested
		}
	}
	if err != nil {
		return nil, err
	}
	if row.Negated {
		return Not{Inner: p}, nil
	}
	return p, nil
}

// compileCondx 编译单条条件为行包装主体（RowSubjectPattern）：行内主体（含随
// 主体终结谓词）+ 值校验门。fileSizeMin 因扫描快照暂无文件尺寸（数据缺口）
// 显式拒绝；其余谓词与值校验全部接入求值。
func compileCondx(c *Condx, ordinal int) (Pattern, error) {
	if err := validateCondx(c); err != nil {
		return nil, err
	}
	var gates []Pattern
	for _, b := range c.Badges {
		// 值校验位次化：编译名带位次后缀（位1 裸名），求值寻址与 #canon 键随
		// 之（BadgeGatePattern 以全名寻址，求值面零特判）。
		gate := &BadgeGatePattern{
			Row: ordinal, Name: condxPosName(condxVocab[b.Placeholder], b.Occur),
			Kind: b.Kind, Dim: b.Dim, Fold: b.Fold, Constant: b.Value,
		}
		if b.Kind == CondxBadgeEqual {
			gate.RightRow = b.AgainstRow
			gate.RightName = condxPosName(condxVocab[b.AgainstPh], b.AgainstOccur)
		}
		gates = append(gates, gate)
	}
	var inner Pattern
	var err error
	switch c.Target {
	case CondxTargetFile:
		inner, err = compileFileCondx(c)
	case CondxTargetDir:
		inner, err = compileDirCondx(c)
	case CondxTargetSeq:
		inner, err = compileSeqCondx(c)
	default:
		return nil, fmt.Errorf("未知的目标类别: %q", string(c.Target))
	}
	if err != nil {
		return nil, err
	}
	return &RowSubjectPattern{Ordinal: ordinal, Inner: inner, Gates: gates}, nil
}

// compileFileCondx 编译文件主体：格式模板 + fileExt（并入主体）+ 父目录两式
// （并列子式）；fileSizeMin 显式拒绝（扫描快照数据缺口）。
func compileFileCondx(c *Condx) (Pattern, error) {
	expr, captures, repeats, _, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{})
	if err != nil {
		return nil, err
	}
	subject, err := newFileSubjectPattern(expr, captures, repeats)
	if err != nil {
		return nil, err
	}
	subs := []Pattern{subject}
	for _, a := range c.Attach {
		switch a.Kind {
		case CondxAttachFileExt:
			ext := strings.ToLower(a.Value)
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			subject.Ext = ext
		case CondxAttachParentFormat:
			expr, _, _, _, err := deriveCondxTemplate(a.Tpl, deriveOpts{allowKey: true})
			if err != nil {
				return nil, fmt.Errorf("附加条件 parentFormat 的迷你模板无效: %w", err)
			}
			p, err := newParentNamePattern(expr)
			if err != nil {
				return nil, err
			}
			subs = append(subs, p)
		case CondxAttachParentContains:
			expr, err := deriveCondxContains(a.Word)
			if err != nil {
				return nil, err
			}
			p, err := newParentNamePattern(expr)
			if err != nil {
				return nil, err
			}
			subs = append(subs, p)
		case CondxAttachFileSizeMin:
			return nil, fmt.Errorf("附加条件 fileSizeMin 待扫描快照携带文件尺寸后接入（数据缺口，显式拒绝不静默）")
		}
	}
	if len(subs) == 1 {
		return subs[0], nil
	}
	return And(subs), nil
}

// compileDirCondx 编译目录主体：格式 + 终结谓词集（子女全部/至少N按迷你格式、
// 子目录数、子女类型、父目录两式——父目录谓词作用于**命中层目录**的父；多段
// 模板的命中层=落地段层，谓词随落地段求值，自决6）。模板可多段（决策1）：
// 单段走现行 DirSubjectPattern 路径零变化；多段走 Segments 求值路径（段模式
// 序列+段距序列）。
func compileDirCondx(c *Condx) (Pattern, error) {
	var subject *DirSubjectPattern
	if c.Format.Mode == CondxFormatContains {
		expr, err := deriveCondxContains(c.Format.Word)
		if err != nil {
			return nil, err
		}
		subject, err = newDirSubjectPattern(expr, nil, nil)
		if err != nil {
			return nil, err
		}
	} else {
		segs, gaps, err := deriveDirTplSegments(c.Format.Tpl)
		if err != nil {
			return nil, err
		}
		if len(segs) == 1 {
			subject, err = newDirSubjectPattern(segs[0].expr, segs[0].captures, segs[0].repeats)
		} else {
			subject, err = newMultiDirSubjectPattern(c.Format.Tpl, segs, gaps)
		}
		if err != nil {
			return nil, err
		}
	}
	for _, a := range c.Attach {
		assert := DirAssert{Kind: a.Kind, MinCount: a.MinCount, Types: a.Types, Word: a.Word, Value: a.Value}
		switch a.Kind {
		case CondxAttachChildrenAllFormat, CondxAttachChildrenAtLeastN, CondxAttachParentFormat:
			mexpr, _, _, _, err := deriveCondxTemplate(a.Tpl, deriveOpts{allowKey: true})
			if err != nil {
				return nil, fmt.Errorf("附加条件 %s 的迷你模板无效: %w", a.Kind, err)
			}
			re, cerr := regexp.Compile(mexpr)
			if cerr != nil {
				return nil, fmt.Errorf("附加条件 %s 的迷你正则编译失败: %w", a.Kind, cerr)
			}
			assert.re = re
		}
		subject.Asserts = append(subject.Asserts, assert)
	}
	return subject, nil
}

// compileSeqCondx 编译序列主体：成员格式模板 + seqMinLength（并入主体门槛）+
// seqMemberType（并入成员计数限定）+ 父目录两式（并列子式）。
func compileSeqCondx(c *Condx) (Pattern, error) {
	expr, captures, repeats, optCaptures, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{allowKey: true, requireSeq: true})
	if err != nil {
		return nil, err
	}
	subject, err := newSeqSubjectPattern(expr, captures, repeats, optCaptures, 2)
	if err != nil {
		return nil, err
	}
	subs := []Pattern{subject}
	for _, a := range c.Attach {
		switch a.Kind {
		case CondxAttachSeqMinLength:
			if a.MinCount > 0 {
				subject.MinSize = a.MinCount
			}
		case CondxAttachSeqMemberType:
			subject.MemberTypes = append(subject.MemberTypes, a.Types...)
		case CondxAttachParentFormat:
			expr, _, _, _, err := deriveCondxTemplate(a.Tpl, deriveOpts{allowKey: true})
			if err != nil {
				return nil, fmt.Errorf("附加条件 parentFormat 的迷你模板无效: %w", err)
			}
			p, err := newParentNamePattern(expr)
			if err != nil {
				return nil, err
			}
			subs = append(subs, p)
		case CondxAttachParentContains:
			expr, err := deriveCondxContains(a.Word)
			if err != nil {
				return nil, err
			}
			p, err := newParentNamePattern(expr)
			if err != nil {
				return nil, err
			}
			subs = append(subs, p)
		}
	}
	if len(subs) == 1 {
		return subs[0], nil
	}
	return And(subs), nil
}

// CompileRuleConditions 校验并编译 v4 条件行列表为模式树（规则构造入口：
// 问答/泛化/裁决等机器规则生产方与档案装载共用——v4 档案仅认 conditions，
// 机器流同样以条件行产出，模式树只是编译缓存）。
func CompileRuleConditions(rows []CondxRow) (Pattern, error) {
	return compileCondxRows(rows)
}

// declaredConditionCaptures 收集条件行列表会绑定进解析上下文的捕获组名
// （行号前缀形态 rN.组名；库名单值校验另产出 rN.组名#canon 规范名绑定），为
// **提取引用口径**：含重复段词（rN.tag 一类多值产出词根）——提取动作可引用
// 重复段词（值由绑定层按单元值展开），故并入。供提取动作的捕获引用做构造期
// 存在性校验。行号=先序展平，与编译一致。
func declaredConditionCaptures(rows []CondxRow) map[string]bool {
	all, _, _ := declaredConditionCaptureSets(rows)
	return all
}

// declaredConditionOnlyCaptures 收集**条件可引用口径**的捕获名：与提取口径
// 同源但剔除重复段词（多值产出坐标量单一值，生效条件/组合接线暂不允许引用）。
func declaredConditionOnlyCaptures(rows []CondxRow) map[string]bool {
	_, cond, _ := declaredConditionCaptureSets(rows)
	return cond
}

// declaredConditionRepeatCaptures 收集条件行列表中的重复段词捕获名（行号
// 前缀形态，来源=condxRepeatDesc.En）：阶段1 的位置组设计使 En 不进捕获组名
// 清单，此处在声明层显式补入并打标，供三面引用禁区识别。
func declaredConditionRepeatCaptures(rows []CondxRow) map[string]bool {
	_, _, repeat := declaredConditionCaptureSets(rows)
	return repeat
}

// declaredConditionCaptureSets 单遍收集三口径集合：all=提取可引用（含重复段
// 词）、cond=条件可引用（剔除重复段词）、repeat=重复段词标记集。
func declaredConditionCaptureSets(rows []CondxRow) (all, cond, repeat map[string]bool) {
	all, cond, repeat = map[string]bool{}, map[string]bool{}, map[string]bool{}
	collectConditionCaptureSetsAt(rows, 0, all, cond, repeat)
	return all, cond, repeat
}

// collectConditionCaptureSetsAt 在全局行号偏移 base 上递归收集三口径集合。
func collectConditionCaptureSetsAt(rows []CondxRow, base int, all, cond, repeat map[string]bool) {
	prefix := 0
	for i := range rows {
		row := &rows[i]
		if row.Condition != nil {
			ordinal := base + prefix + 1
			captures, repeats, err := condxCapturesOf(row.Condition)
			if err == nil {
				for _, en := range captures {
					name := rowCaptureName(ordinal, en)
					all[name] = true
					cond[name] = true
				}
				for _, rp := range repeats {
					name := rowCaptureName(ordinal, rp.En)
					all[name] = true
					repeat[name] = true
				}
			}
			for _, b := range row.Condition.Badges {
				if b.Kind != CondxBadgeLibrary {
					continue
				}
				if en, ok := condxVocab[b.Placeholder]; ok {
					// #canon 声明名随位次名（位1 裸名零变化；位 N≥2 键位次隔离）。
					name := rowCaptureName(ordinal, condxPosName(en, b.Occur)) + "#canon"
					all[name] = true
					cond[name] = true
				}
			}
			prefix++
		}
		if len(row.Group) > 0 {
			collectConditionCaptureSetsAt(row.Group, base+prefix, all, cond, repeat)
			prefix += countCondxRows(row.Group)
		}
	}
}
