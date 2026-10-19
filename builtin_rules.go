// 内置惯例规则表：高置信文件名惯例（形态求值/提取）与站点惯例定群（兄弟群组）
// 的规则声明与构造，供规则档案以内置来源装载；纯模型，无 IO 与日志。
package main

import "fmt"

// 内置规则的捕获组名：正则命名捕获组与提取动作的取值键共用同一组名字。
const (
	// builtinPageWorkIDCapture 为 pixiv 页级作品 ID 的捕获组名，取值即作品身份键。
	builtinPageWorkIDCapture = "pageWorkID"
	// builtinAuthorCapture 为作者段的捕获组名。
	builtinAuthorCapture = "author"
	// builtinTitleCapture 为标题段的捕获组名。
	builtinTitleCapture = "title"
)

// 内置规则 ID：固定且互不相同，规则档案反复装载时保持稳定。
const (
	// builtinRuleIDPixivPageWorkID 为 pixiv 页级作品 ID 形态规则的 ID。
	builtinRuleIDPixivPageWorkID = "builtin-pixiv-page-work-id"
	// builtinRuleIDPixivPageGroup 为 pixiv 页级作品定群规则的 ID。
	builtinRuleIDPixivPageGroup = "builtin-pixiv-page-group"
	// builtinRuleIDBracketAuthorTitle 为「【作者】标题」分隔惯例规则的 ID。
	builtinRuleIDBracketAuthorTitle = "builtin-bracket-author-title"
	// builtinRuleIDDashAuthorTitle 为「作者 - 标题」分隔惯例规则的 ID。
	builtinRuleIDDashAuthorTitle = "builtin-dash-author-title"
)

// builtinTrailingExtExpr 为文件名末尾扩展名段：至多一段不含点号的末段扩展名，
// 命中即被丢弃——规则只解析扩展名之前的正文，与去末段扩展名的语义一致。
const builtinTrailingExtExpr = `(?:\.[^.]*)?$`

// builtinTitleSegmentExpr 为分隔惯例规则的标题段：以非点号字符起头，故「作者 + 点号
// + 扩展名」这类无标题的文件名不命中（点号起头一律视为扩展名段）；其后的字符不再
// 限形，标题内的点号（如版本号）照常保留。
const builtinTitleSegmentExpr = `[^.].*?`

// builtinAuthorSegmentExpr 为「作者 - 标题」规则的作者段：一到多个以空白分隔的词，
// 词内不得含连字符分隔符，也不得含括号 / 引号 / 竖线 / 斜杠等噪音字符。作者段起头
// 亦不得为空白，故「 - 标题」这类缺少作者的文件名不命中。
const builtinAuthorSegmentExpr = `[^\s\-【】\[\]()（）<>《》|/\\]+(?:\s+[^\s\-【】\[\]()（）<>《》|/\\]+)*`

// builtinPixivPageWorkIDExpr 为 pixiv 原图文件名的页级作品 ID 形态，作用于含扩展名的
// 文件名：去掉末段扩展名后的正文须整体等于 {作品号}[-{十六进制补丁}]_p{页号}。
// 锚定全匹配——带前后缀的形似文件名（如 master 规格名）不命中。
const builtinPixivPageWorkIDExpr = `^(?P<` + builtinPageWorkIDCapture + `>\d+(?:-[0-9a-f]+)?_p\d+)` + builtinTrailingExtExpr

// builtinPixivPageGroupKeyExpr 为页级定群规则的分组键提取式：捕获组1 = 作品 ID，
// 容忍可选的流水号前缀（{数字}_）。十六进制补丁变体（{作品号}-{补丁}_p{页号}）
// 不属本形态（键提取不成立即不参与定群，求值规则仍透出其 ID 线索）。
const builtinPixivPageGroupKeyExpr = `(?:\d+_)?(\d+)_p\d+`

// builtinPixivPageGroupSeqExpr 为页级定群规则的序号提取式：捕获组1 = 页号，
// 容忍 `_master{N}` 缩图规格尾缀；作用于含扩展名的文件名，末段扩展名不计入。
const builtinPixivPageGroupSeqExpr = `_p(\d+)(?:_master\d+)?` + builtinTrailingExtExpr

// builtinBracketAuthorTitleExpr 为「【作者】标题」形态：作者为方括号内的非空文本
// （方括号不参与嵌套），标题为紧随其后的非空正文，末段扩展名不计入标题。
const builtinBracketAuthorTitleExpr = `^【(?P<` + builtinAuthorCapture + `>[^】]+)】(?P<` + builtinTitleCapture + `>` + builtinTitleSegmentExpr + `)` + builtinTrailingExtExpr

// builtinDashAuthorTitleExpr 为「作者 - 标题」形态：分隔符为两侧带空白的连字符，
// 标题段允许再含连字符（只按首个干净的分隔处切分）。
const builtinDashAuthorTitleExpr = `^(?P<` + builtinAuthorCapture + `>` + builtinAuthorSegmentExpr + `)\s+-\s+(?P<` + builtinTitleCapture + `>` + builtinTitleSegmentExpr + `)` + builtinTrailingExtExpr

// builtinSiblingSpec 为内置兄弟群组定群模式的声明：按其键/序号提取式构造
// SiblingPattern（顶层 SiblingPattern 规则参与流水线分组段，不参与提取段）。
type builtinSiblingSpec struct {
	// KeyExpr 为分组键提取正则，捕获组1 = 分群键。
	KeyExpr string
	// SeqExpr 为序号提取正则，捕获组1 = 序号原文。
	SeqExpr string
	// MinGroupSize 为成群的最少成员数。
	MinGroupSize int
	// Outcome 为命中群组后的归属结果类别。
	Outcome GroupOutcome
}

// builtinRuleSpec 为一条内置规则的声明：文件名正则或兄弟群组定群模式的构造
// 参数与命中后的提取动作，由 buildBuiltinRules 统一编译装配为完整规则。
type builtinRuleSpec struct {
	// ID 为规则唯一标识。
	ID string
	// Description 为规则的人类可读描述。
	Description string
	// Expr 为文件名匹配正则原文（Sibling 在场时不参与）。
	Expr string
	// Captures 为需要绑定的命名捕获组名列表（Sibling 在场时不参与）。
	Captures []string
	// Extractions 为命中后依序执行的提取动作；为空表示只做形态求值或定群。
	Extractions []ExtractionAction
	// Sibling 为兄弟群组定群模式声明；在场时按其提取式构造 SiblingPattern，
	// Expr/Captures 不参与（定群规则不走正则模式构造）。
	Sibling *builtinSiblingSpec
}

// builtinRuleSpecs 返回内置规则声明表，切片顺序即规则表顺序（EffectiveRules
// 的内置段保持此序；用户规则恒先于全部内置规则）。
func builtinRuleSpecs() []builtinRuleSpec {
	return []builtinRuleSpec{
		{
			ID:          builtinRuleIDPixivPageWorkID,
			Description: "pixiv 原图文件名页级作品 ID 形态：{作品号}[-{十六进制补丁}]_p{页号}，锚定全匹配，命中即认定页级作品形态",
			Expr:        builtinPixivPageWorkIDExpr,
			Captures:    []string{builtinPageWorkIDCapture},
		},
		{
			ID:          builtinRuleIDPixivPageGroup,
			Description: "pixiv 页级作品定群：[流水号_]{作品号}_p{页号}[._master{N}] 形态按作品 ID 分组、页号为序号，同作品 ≥2 页归入同一作品集",
			Sibling: &builtinSiblingSpec{
				KeyExpr:      builtinPixivPageGroupKeyExpr,
				SeqExpr:      builtinPixivPageGroupSeqExpr,
				MinGroupSize: 2,
				Outcome:      OutcomeCollection,
			},
		},
		{
			ID:          builtinRuleIDBracketAuthorTitle,
			Description: "「【作者】标题」命名惯例：方括号内为作者、其后为标题，末段扩展名不计入标题",
			Expr:        builtinBracketAuthorTitleExpr,
			Captures:    []string{builtinAuthorCapture, builtinTitleCapture},
			Extractions: []ExtractionAction{
				{Dimension: DimAuthor, From: FromCapture, CaptureName: builtinAuthorCapture},
				{Dimension: DimTitle, From: FromCapture, CaptureName: builtinTitleCapture},
			},
		},
		{
			ID:          builtinRuleIDDashAuthorTitle,
			Description: "「作者 - 标题」命名惯例：空格包围的连字符分隔，作者段不含分隔符与括号噪音，末段扩展名不计入标题",
			Expr:        builtinDashAuthorTitleExpr,
			Captures:    []string{builtinAuthorCapture, builtinTitleCapture},
			Extractions: []ExtractionAction{
				{Dimension: DimAuthor, From: FromCapture, CaptureName: builtinAuthorCapture},
				{Dimension: DimTitle, From: FromCapture, CaptureName: builtinTitleCapture},
			},
		},
	}
}

// buildBuiltinRules 按声明表构造内置规则：来源统一为内置、启用统一为真；
// 正则条目与兄弟群组条目各按其构造参数编译装配。任一正则编译失败时返回中文
// 错误与 nil。声明表的表达式均为固定常量，构造失败不属于运行期可达路径。
func buildBuiltinRules() ([]RuleEntry, error) {
	specs := builtinRuleSpecs()
	rules := make([]RuleEntry, 0, len(specs))
	for _, spec := range specs {
		entry := RuleEntry{
			ID:          spec.ID,
			Description: spec.Description,
			Source:      RuleSourceBuiltin,
			Enabled:     true,
			Extractions: spec.Extractions,
		}
		if spec.Sibling != nil {
			sibling, err := NewSiblingPattern(spec.Sibling.KeyExpr, spec.Sibling.SeqExpr, spec.Sibling.MinGroupSize, spec.Sibling.Outcome)
			if err != nil {
				return nil, fmt.Errorf("内置规则 %q 构造失败: %w", spec.ID, err)
			}
			entry.Pattern = sibling
		} else {
			pattern, err := NewRegexPattern(string(TargetFileName), spec.Expr, spec.Captures)
			if err != nil {
				return nil, fmt.Errorf("内置规则 %q 构造失败: %w", spec.ID, err)
			}
			entry.Pattern = pattern
		}
		rules = append(rules, entry)
	}
	return rules, nil
}

// BuiltinRules 返回内置高置信规则表：全部条目来源为内置、启用状态，ID 固定唯一。
// 声明表恒可构造；构造失败时返回 nil，使缺陷在测试与装配首帧暴露而非静默降级。
func BuiltinRules() []RuleEntry {
	rules, err := buildBuiltinRules()
	if err != nil {
		return nil
	}
	return rules
}
