// 例子模板表：规则编辑面的 v4 条件行模板示例（内置规则退休后的形态——
// 不参与求值、不入档案，仅作「格式模板怎么写」的参考词条）。纯模型，无 IO。
package main

// exampleTemplate 为一条例子模板词条：模板原文 + 人话说明 + 适用主体。
type exampleTemplate struct {
	// Name 为词条短名（列表展示用）。
	Name string `json:"name"`
	// Template 为 v4 格式模板原文（词条表展示与插入编辑器的原文）。
	Template string `json:"template"`
	// Target 为主体类别（filename / dirname / seq——规则目标主体常量值）。
	Target string `json:"target"`
	// Description 为词条说明（命中形态与产出维度的中文说明）。
	Description string `json:"description"`
}

// exampleTemplates 返回例子模板表（v4 词条封闭词表演示：作者/标题/作品号/
// 序号/集名/键/任意）。顺序即展示顺序。
func exampleTemplates() []exampleTemplate {
	return []exampleTemplate{
		{
			Name:     "【作者】标题",
			Template: "【{作者}】{标题}",
			Target:   "filename",
			Description: "方括号命名惯例：【】内为作者、其后为标题。字面【】与模板词组合，" +
				"扩展名自动忽略（模板对去扩展名的文件名正文求值）",
		},
		{
			Name:     "作者 - 标题",
			Template: "{作者} - {标题}",
			Target:   "filename",
			Description: "空格包围连字符的分隔惯例：作者与标题各占一位模板词，" +
				"分隔原文（空格-连字符-空格）按字面匹配",
		},
		{
			Name:     "pixiv 页级文件名",
			Template: "{作品号}_p{序号}",
			Target:   "filename",
			Description: "pixiv 原图页级形态：作品号为纯数字类（[0-9]+），页号占序号词。" +
				"带补丁变体（作品号-十六进制_p页号）可在词间加字面段匹配",
		},
		{
			Name:     "序列成套",
			Template: "{键}_{序号}",
			Target:   "seq",
			Description: "序列主体行：同键文件 ≥2 件即成套归入同一作品集。" +
				"{键} 为公共前缀位，{序号} 为序号位（序号词数字类）",
		},
		{
			Name:     "目录层作者",
			Template: "Artist[{作者}]",
			Target:   "dirname",
			Description: "目录主体行：任意祖先目录名命中该模板即取作者。" +
				"剥壳惯例（前缀[内名]）由字面括号 + 模板词表达",
		},
		{
			Name:     "任意段吞位",
			Template: "{标题}_{任意}",
			Target:   "filename",
			Description: "{任意} 为不引用的吞位词（不产出声明、可重复出现）：" +
				"标题之后的一切（尾部码等）整体吞掉，不与引用词争词表",
		},
	}
}
