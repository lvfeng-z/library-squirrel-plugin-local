// 条件构建语法 v4（收敛档 R1-R11，2026-10-24）：目标三分（file/dir/seq）+ 格式
// 模板（占位词点选）+ 附加条件（节点级终结谓词集）+ 值级小标（名单/相等/常量）。
// 本文件为纯语法层：类型定义、模板→正则推导、静态校验、行列表→模式树编译。
// 求值扩展（目录子女终结谓词、形变归一名单、跨行相等门）在后续硬化步接入；
// 未实现的附加/小标在编译期显式报错（绝不静默视为恒真）。
package main

import (
	"fmt"
	"regexp"
	"strings"
)

// CondxTargetKind 为条件的目标类别（三级词表·目标三分）。
type CondxTargetKind string

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

// CondxBadge 为值级小标：附着在占位词上的封闭三选一断言（R6/R7）。
type CondxBadge struct {
	// Placeholder 为小标作用的占位词（词表原文，如 "作者"）。
	Placeholder string `json:"placeholder"`
	// Kind 为小标类别："library"（在库名单）| "equal"（与另一占位词相等）| "constant"（=常量）。
	Kind string `json:"kind"`
	// Dim 为 library 的名单维度（author / tag / collection）。
	Dim string `json:"dim,omitempty"`
	// Fold 为 library 的形变归一开关（_ - 与大小写折叠，折叠后仍唯一才命中）。
	Fold bool `json:"fold,omitempty"`
	// AgainstRow 为 equal 的对方行号（1 起，按行先序展平计数；须早于本行）。
	AgainstRow int `json:"againstRow,omitempty"`
	// AgainstPh 为 equal 的对方占位词（词表原文）。
	AgainstPh string `json:"againstPh,omitempty"`
	// Value 为 constant 的常量值。
	Value string `json:"value,omitempty"`
}

// 值级小标类别常量。
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
		CondxAttachParentFormat: true, CondxAttachParentContains: true,
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

// Condx 为一条条件（行内主体声明）：目标 + 格式 + 值级小标 + 终结谓词。
type Condx struct {
	// Target 为目标类别（file / dir / seq）。
	Target CondxTargetKind `json:"target"`
	// Format 为格式段（两种问法）。
	Format CondxFormat `json:"format"`
	// Badges 为值级小标列表（可空）。
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
// {键} 仅用于序列成员模板（标记跨成员恒同部分，产出走群组键）。
var condxVocab = map[string]string{
	"作者": "author", "集名": "setname", "序号": "seq", "标题": "title",
	"标签": "tag", "作品号": "workid", "日期": "date", "键": "key",
}

// condxDigits 为数字类占位词（推导为 [0-9]+）。
var condxDigits = map[string]bool{"序号": true, "作品号": true, "日期": true}

// condxVocabHint 为报错提示用的词表清单（稳定顺序）。
const condxVocabHint = "作者/集名/序号/标题/标签/作品号/日期/键"

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

// deriveCondxTemplate 把占位词模板推导为锚定正则：字面段转义；数字类占位词
// （序号/作品号/日期）取 [0-9]+；其余占位词一律取贪婪段 .+（回溯由锚定边界与
// 相邻字面段约束，见函数体内注释）。返回表达式原文与捕获组名列表；词表外/
// 重复/相邻占位词返回中文错误。
func deriveCondxTemplate(tpl string, opts deriveOpts) (string, []string, error) {
	if strings.TrimSpace(tpl) == "" {
		return "", nil, fmt.Errorf("格式模板不能为空")
	}
	tokens, err := tokenizeCondx(tpl)
	if err != nil {
		return "", nil, err
	}
	seen := make(map[string]bool)
	var captures []string
	var sb strings.Builder
	sb.WriteString("^")
	hasSeq := false
	prevPlaceholder := false
	for _, tok := range tokens {
		if tok.literal != "" {
			sb.WriteString(regexp.QuoteMeta(tok.literal))
			prevPlaceholder = false
			continue
		}
		if prevPlaceholder {
			return "", nil, fmt.Errorf("相邻占位词 {%s} 与前一个占位词之间缺少分隔文字", tok.placeholder)
		}
		prevPlaceholder = true
		word := tok.placeholder
		en, known := condxVocab[word]
		if !known {
			return "", nil, fmt.Errorf("模板占位词 {%s} 不在词表（%s）", word, condxVocabHint)
		}
		if word == "键" && !opts.allowKey {
			return "", nil, fmt.Errorf("模板占位词 {键} 仅适用于序列成员格式")
		}
		if seen[en] {
			return "", nil, fmt.Errorf("模板占位词 {%s} 重复", word)
		}
		seen[en] = true
		captures = append(captures, en)
		if word == "序号" {
			hasSeq = true
		}
		sb.WriteString("(?P<" + en + ">")
		if condxDigits[word] {
			sb.WriteString("[0-9]+")
		} else {
			// 贪婪段 .+：锚定边界与相邻字面段经回溯决定切分——值可含下划线等
			// 分隔符（如 kpop_dancer_wg）；贪婪取最长、尾随数字类取最短，恰好
			// 落在「键吞尽、序号取尾」的期望切分上。不采用首段排除启发式——
			// 排除类会误杀含分隔符的值，且回溯已保证字面段完整。
			sb.WriteString(".+")
		}
		sb.WriteString(")")
	}
	if opts.requireSeq && !hasSeq {
		return "", nil, fmt.Errorf("序列成员格式必须包含 {序号}")
	}
	sb.WriteString("$")
	return sb.String(), captures, nil
}

// deriveCondxContains 把包含词推导为锚定正则（含这个词：任意位置字面包含）。
func deriveCondxContains(word string) (string, error) {
	if word == "" {
		return "", fmt.Errorf("包含词不能为空")
	}
	return "^.*" + regexp.QuoteMeta(word) + ".*$", nil
}

// condxCapturesOf 返回条件格式段推导出的捕获组英文名列表。
func condxCapturesOf(c *Condx) ([]string, error) {
	if c.Format.Mode == CondxFormatContains {
		return nil, nil
	}
	_, captures, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{allowKey: c.Target == CondxTargetSeq})
	return captures, err
}

// validateCondx 校验单条条件：目标合法、格式可推导、小标引用存在、终结谓词
// 属目标封闭集且必填字段齐备、主体锚为 nil。
func validateCondx(c *Condx, ownOrdinal int) error {
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
		if _, _, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{
			allowKey:   c.Target == CondxTargetSeq,
			requireSeq: c.Target == CondxTargetSeq,
		}); err != nil {
			return fmt.Errorf("格式模板无效: %w", err)
		}
	case CondxFormatContains:
		if c.Target == CondxTargetSeq {
			return fmt.Errorf("序列目标只支持整条按模板（contains 问法不适用）")
		}
		if c.Format.Word == "" {
			return fmt.Errorf("包含词不能为空")
		}
	default:
		return fmt.Errorf("未知的格式模式: %q（template / contains）", c.Format.Mode)
	}
	captures, _ := condxCapturesOf(c)
	captureSet := make(map[string]bool, len(captures))
	for _, en := range captures {
		captureSet[en] = true
	}
	for i, b := range c.Badges {
		en, known := condxVocab[b.Placeholder]
		if !known {
			return fmt.Errorf("第 %d 个值级小标的占位词 {%s} 不在词表（%s）", i+1, b.Placeholder, condxVocabHint)
		}
		if !captureSet[en] {
			return fmt.Errorf("值级小标 {%s} 引用的占位词不在本行格式模板内", b.Placeholder)
		}
		switch b.Kind {
		case CondxBadgeLibrary:
			switch b.Dim {
			case "author", "tag", "collection":
			default:
				return fmt.Errorf("值级小标 {%s} 的名单维度无效: %q（author / tag / collection）", b.Placeholder, b.Dim)
			}
		case CondxBadgeEqual:
			if b.AgainstPh == "" {
				return fmt.Errorf("值级小标 {%s} 的相等对象占位词不能为空", b.Placeholder)
			}
			if _, ok := condxVocab[b.AgainstPh]; !ok {
				return fmt.Errorf("值级小标 {%s} 的相等对象 {%s} 不在词表", b.Placeholder, b.AgainstPh)
			}
		case CondxBadgeConstant:
			if b.Value == "" {
				return fmt.Errorf("值级小标 {%s} 的常量值不能为空", b.Placeholder)
			}
		default:
			return fmt.Errorf("未知的值级小标类别: %q（library / equal / constant）", b.Kind)
		}
	}
	allowed := attachAllowed[c.Target]
	for i, a := range c.Attach {
		if !allowed[a.Kind] {
			return fmt.Errorf("第 %d 个附加条件 %q 不适用于目标 %q", i+1, a.Kind, string(c.Target))
		}
		if attachNeedsTpl[a.Kind] {
			if _, _, err := deriveCondxTemplate(a.Tpl, deriveOpts{allowKey: true}); err != nil {
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
	_ = ownOrdinal
	return nil
}

// validateCondxRows 校验行列表：至少一行、首行无连接词、后续行连接词合法、
// 行内条件与子组二选一且其一非空、跨行值级相等的行号引用合法（先序展平编号，
// 对方行须早于本行——求值顺序依赖，R6）。
func validateCondxRows(rows []CondxRow) error {
	if len(rows) == 0 {
		return fmt.Errorf("条件行列表不能为空")
	}
	total := countCondxRows(rows)
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
			ordinal := condxRowOrdinal(rows, i)
			if err := validateCondx(row.Condition, ordinal); err != nil {
				return fmt.Errorf("第 %d 行: %w", ordinal, err)
			}
			for _, b := range row.Condition.Badges {
				if b.Kind != CondxBadgeEqual {
					continue
				}
				if b.AgainstRow < 1 || b.AgainstRow > total {
					return fmt.Errorf("第 %d 行: 值级小标 {%s} 的对方行号 %d 越界（共 %d 行）",
						ordinal, b.Placeholder, b.AgainstRow, total)
				}
				if b.AgainstRow >= ordinal {
					return fmt.Errorf("第 %d 行: 值级小标 {%s} 的对方行号 %d 须早于本行", ordinal, b.Placeholder, b.AgainstRow)
				}
			}
		}
		if len(row.Group) > 0 {
			if err := validateCondxRows(row.Group); err != nil {
				return fmt.Errorf("第 %d 行子组: %w", i+1, err)
			}
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

// condxRowOrdinal 返回顶层第 idx 行的先序展平行号（1 起；条件行编号=其前全部
// 条件行数+1，子组在其父行位置整体计入——跨行相等引用以该编号为准）。
func condxRowOrdinal(rows []CondxRow, idx int) int {
	n := 0
	for i := 0; i < idx; i++ {
		if rows[i].Condition != nil {
			n++
		}
		if len(rows[i].Group) > 0 {
			n += countCondxRows(rows[i].Group)
		}
	}
	return n + 1
}

// compileCondxRows 把行列表编译为模式树：且连接紧于或连接（或分支=按 or 切段，
// 段内 and 链），括号子组最紧，单子归一，行级非包裹整行——与前端行式底盘的
// 组合语义一致（前端 rowsToTree 的内核侧锚定实现）。
func compileCondxRows(rows []CondxRow) (Pattern, error) {
	if err := validateCondxRows(rows); err != nil {
		return nil, err
	}
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
	compileChunk := func(c chunk) (Pattern, error) {
		var subs []Pattern
		for i := range c.rows {
			p, err := compileCondxRow(&c.rows[i])
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
	var branches []Pattern
	for _, c := range chunks {
		p, err := compileChunk(c)
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

// compileCondxRow 编译单行：条件或子组 → 模式；行级非包裹。
func compileCondxRow(row *CondxRow) (Pattern, error) {
	var p Pattern
	var err error
	if row.Condition != nil {
		p, err = compileCondx(row.Condition)
	} else {
		p, err = compileCondxRows(row.Group)
	}
	if err != nil {
		return nil, err
	}
	if row.Negated {
		return Not{Inner: p}, nil
	}
	return p, nil
}

// compileCondx 编译单条条件为目标主体模式。本期实现格式段主体（文件/目录/
// 序列模板）与 seqMinLength；其余终结谓词与值级小标的求值在后续硬化步接入，
// 编译期显式报错（不静默恒真）。
func compileCondx(c *Condx) (Pattern, error) {
	if err := validateCondx(c, 0); err != nil {
		return nil, err
	}
	minSeq := 2
	for _, a := range c.Attach {
		switch a.Kind {
		case CondxAttachSeqMinLength:
			if a.MinCount > 0 {
				minSeq = a.MinCount
			}
		case CondxAttachParentFormat, CondxAttachParentContains:
			// 上级断言可与主体共存：本期主体编译先行，上级断言下一硬化步并入。
			return nil, fmt.Errorf("附加条件 %q 的求值将在下一硬化步接入（本期语法已收口，先拒后接）", a.Kind)
		default:
			return nil, fmt.Errorf("附加条件 %q 的求值将在下一硬化步接入（本期语法已收口，先拒后接）", a.Kind)
		}
	}
	if len(c.Badges) > 0 {
		return nil, fmt.Errorf("值级小标的求值将在下一硬化步接入（本期语法已收口，先拒后接）")
	}
	switch c.Target {
	case CondxTargetFile:
		expr, captures, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{})
		if err != nil {
			return nil, err
		}
		return newFileSubjectPattern(expr, captures)
	case CondxTargetDir:
		if c.Format.Mode == CondxFormatContains {
			expr, err := deriveCondxContains(c.Format.Word)
			if err != nil {
				return nil, err
			}
			return newDirSubjectPattern(expr, nil)
		}
		expr, captures, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{})
		if err != nil {
			return nil, err
		}
		return newDirSubjectPattern(expr, captures)
	case CondxTargetSeq:
		expr, captures, err := deriveCondxTemplate(c.Format.Tpl, deriveOpts{allowKey: true, requireSeq: true})
		if err != nil {
			return nil, err
		}
		return newSeqSubjectPattern(expr, captures, minSeq)
	default:
		return nil, fmt.Errorf("未知的目标类别: %q", string(c.Target))
	}
}
