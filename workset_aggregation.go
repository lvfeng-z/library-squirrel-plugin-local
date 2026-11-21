// 作品集聚合（产出四）求值内核：规则命中文件的集归属解析。汇流后的独立通道
// （与聚合段同相位、居其后）：不进逐文件求值循环、不读盘——输入=规则命中面
// （布尔门后的命中集，与聚合段同一命中集机制），输出=成员级集归属结论
// （组键+集名+规则ID）。
//
// 组键：统一分组求值器的库级取值源形态（aggFileRowHits.groupingKeyOfWithLibrarySource）
// ——标识分组的序列群键保留名行内序列行群产物优先、库级群卡键兜底；所在目录=
// 目录聚合键。分组键不可得的文件不入集（孤文件回退=独立作品），与伴随说明
// 文件「组内无落点则独立作品」的既有语义同款。
//
// 集名：规则匹配层模板产出的作品集维度声明（产出四自身不携带集名来源，档案
// 校验已拦无集名产出的条目）；按规则 ID 归位，组内取首个能产出的成员（扫描序
// ——同组成员经同一规则模板/群键产出同名值）。组内无集名可用（如生效条件不
// 成立）时整组不落集归属，成员保持独立作品并留给后续规则。
//
// 竞争（档案序先到先得）：既有聚合归属优先——已被聚合收编或说明成员标记的
// 文件不入集（聚合段在先、其归属不改判）；作品集聚合规则之间按规则档案中的
// 声明顺序先到先得，同一文件被在先规则归集后不再改判（与聚合段首个命中定群
// 同款取向）。归属只在实际落定（组键与集名皆可得）时生效，未落定的文件仍可
// 被后续规则接纳。
package main

// workSetAssignment 单文件的集归属结论（产出四求值产物）：GroupKey 为按归集
// 依据算出的组键（身份复合键 NUL 连接／序列群键／目录聚合键），SetName 为该
// 组命中成员的集名（规则匹配层模板产出），RuleID 为产出该归属的规则。三字段
// 同空=不入集；消费面按「GroupKey+SetName」识别一个集，按 RuleID 追溯产出规则。
type workSetAssignment struct {
	GroupKey string
	SetName  string
	RuleID   string
}

// workSetRulePlan 单条产出四规则的归集求值预案：条件行（先序展平叶子行）编译
// 为独立求值的行包装模式，布尔组合形态另编译整条求值门（gate，与聚合段同一
// 接线；纯平行行不设门=行命中池 OR 汇聚即命中面）。seq 为规则在规则表中的
// 下标（档案序先到先得的凭据）。
type workSetRulePlan struct {
	rule     *RuleEntry
	seq      int
	rows     []Pattern
	gate     Pattern
	grouping *GroupingBinding
}

// buildWorkSetPlans 过滤启用且携带产出四的规则并编译预案；行编译或布尔门编译
// 失败按宁缺勿错整条跳过（载入校验已拦的形态在此双保险，与聚合段同款防御）。
func buildWorkSetPlans(rules []RuleEntry) []*workSetRulePlan {
	var plans []*workSetRulePlan
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || rule.WorkSetAggregation == nil || rule.WorkSetAggregation.Grouping == nil || len(rule.Conditions) == 0 {
			continue
		}
		leaves := condxLeafRowsOf(rule.Conditions)
		if len(leaves) == 0 {
			continue
		}
		rows := make([]Pattern, 0, len(leaves))
		valid := true
		for _, leaf := range leaves {
			p, err := compileCondx(leaf.row.Condition, leaf.no)
			if err != nil {
				valid = false
				break
			}
			if leaf.row.Negated {
				p = Not{Inner: p}
			}
			rows = append(rows, p)
		}
		if !valid {
			continue
		}
		gate, err := aggregateBooleanGate(rule)
		if err != nil {
			continue
		}
		plans = append(plans, &workSetRulePlan{
			rule:     rule,
			seq:      i,
			rows:     rows,
			gate:     gate,
			grouping: rule.WorkSetAggregation.Grouping,
		})
	}
	return plans
}

// workSetSetNames 集名来源面：各文件按规则 ID 索引的作品集维度声明值（规则
// 匹配层模板产出的集名）。nil 条目=该文件无任何规则产出的集名。
type workSetSetNames []map[string]string

// workSetSetNamesOfDecls 从声明集提取集名（同规则同维度取首个非空值）：只认
// 归属规则 ID 非空的声明——会话赋值覆盖的作品集声明（RuleID 空）不是匹配层
// 模板产出，不参与产出四的集名归位。
func workSetSetNamesOfDecls(decls []MetadataDeclaration) map[string]string {
	var out map[string]string
	for i := range decls {
		decl := &decls[i]
		if decl.Dimension != DimCollection || decl.RuleID == "" || decl.Value == "" {
			continue
		}
		if out == nil {
			out = make(map[string]string)
		}
		if _, seen := out[decl.RuleID]; !seen {
			out[decl.RuleID] = decl.Value
		}
	}
	return out
}

// workSetSetNamesOfConclusions 汇流结论形态的集名来源面：终选声明按规则 ID
// 归位（合成后同值吞并不影响本规则自身的声明值）。
func workSetSetNamesOfConclusions(conclusions []FileConclusion) workSetSetNames {
	names := make(workSetSetNames, len(conclusions))
	for i := range conclusions {
		names[i] = workSetSetNamesOfDecls(conclusions[i].Final)
	}
	return names
}

// workSetSetNamesOfDecisions 流水线结论形态的集名来源面：提取段原始声明按规则
// ID 归位（流水线路径无合成终选，声明即产出面）。
func workSetSetNamesOfDecisions(decisions []FileDecision) workSetSetNames {
	names := make(workSetSetNames, len(decisions))
	for i := range decisions {
		names[i] = workSetSetNamesOfDecls(decisions[i].Declarations)
	}
	return names
}

// workSetTakenOfConclusions 既有归属占用面（汇流结论形态）：已被聚合段收编
// （聚合归属键非空）或已标记为说明成员的文件不再进入作品集聚合。
func workSetTakenOfConclusions(conclusions []FileConclusion) []bool {
	taken := make([]bool, len(conclusions))
	for i := range conclusions {
		taken[i] = conclusions[i].AggWorkKey != "" || conclusions[i].AggCompanion
	}
	return taken
}

// workSetTakenOfDecisions 既有归属占用面（流水线结论形态），口径同上。
func workSetTakenOfDecisions(decisions []FileDecision) []bool {
	taken := make([]bool, len(decisions))
	for i := range decisions {
		taken[i] = decisions[i].AggWorkKey != "" || decisions[i].AggCompanion
	}
	return taken
}

// libraryGroupKeysOfDecisions 库级序列群键反查索引（流水线结论形态；口径同
// recognition.go 的 libraryGroupKeysOfConclusions：由分组归属按成员路径反查
// 群卡键）。
func libraryGroupKeysOfDecisions(decisions []FileDecision) libraryGroupKeyIndex {
	idx := make(libraryGroupKeyIndex)
	for i := range decisions {
		if g := decisions[i].Group; g != nil {
			idx.put(decisions[i].RelPath, g.Key)
		}
	}
	return idx
}

// workSetAggregation 产出四归集主算法（纯函数、输入序确定）：规则按档案序逐条
// 求值，对命中的未占用文件按归集依据分组，键相等者成组；组内集名取首个可产出
// 成员（扫描序），集名不可得时整组不落归属；落定成员写入结论并锁定（先到先得）。
// surface 与 files 按下标对齐（调用方保证同一清单）。
func workSetAggregation(surface *aggEvalSurface, rules []RuleEntry, pred *LibraryIndex, groupKeys libraryGroupKeyIndex, setNames workSetSetNames, taken []bool) []workSetAssignment {
	files := surface.files
	out := make([]workSetAssignment, len(files))
	plans := buildWorkSetPlans(rules)
	if len(plans) == 0 {
		return out
	}
	// 辅助表与求值面按下标对齐（调用方保证同源）；越界读取按「未占用/无集名」
	// 处理，防御不同源调用（正常路径恒等长）。
	takenOf := func(i int) bool { return i < len(taken) && taken[i] }
	setNameOf := func(i int, ruleID string) string {
		if i >= len(setNames) {
			return ""
		}
		return setNames[i][ruleID]
	}
	claimed := make([]bool, len(files))
	for _, plan := range plans {
		hits := make([]aggFileRowHits, len(files))
		matched := make([]bool, len(files))
		for i := range files {
			if takenOf(i) || claimed[i] || out[i].GroupKey != "" {
				continue
			}
			file := files[i]
			in := surface.matchInputOf(file)
			if plan.gate != nil {
				// 布尔组合形态：门不过=文件不在本规则命中集（行求值跳过）。
				gateCtx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
				if !plan.gate.Evaluate(in, pred, gateCtx) {
					continue
				}
			}
			rowHits := make(aggFileRowHits, len(plan.rows))
			for j, row := range plan.rows {
				ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
				if row.Evaluate(in, pred, ctx) {
					rowHits[j] = ctx
				}
			}
			if plan.gate != nil {
				// 门过即命中：行命中面只供组键取值（拒绝门内的否定行可令
				// 全部行不中而文件仍属命中集）。
				matched[i] = true
			} else {
				// 纯平行行：行命中池 OR 汇聚即命中面（连接词不参与成员判定，
				// 与聚合段同语义）。
				for j := range rowHits {
					if rowHits[j] != nil {
						matched[i] = true
						break
					}
				}
			}
			if matched[i] {
				hits[i] = rowHits
			}
		}
		// 分组收集：组键可得者按组键归组，组序=组键首成员出现序（确定性）。
		type workSetGroup struct {
			members []int
		}
		groups := make(map[string]*workSetGroup)
		var order []string
		for i := range files {
			if !matched[i] {
				continue
			}
			key, ok := hits[i].groupingKeyOfWithLibrarySource(plan.grouping, files[i].DirNames, files[i].RelPath, groupKeys)
			if !ok {
				continue
			}
			g := groups[key]
			if g == nil {
				g = &workSetGroup{}
				groups[key] = g
				order = append(order, key)
			}
			g.members = append(g.members, i)
		}
		for _, key := range order {
			g := groups[key]
			name := ""
			for _, i := range g.members {
				if v := setNameOf(i, plan.rule.ID); v != "" {
					name = v
					break
				}
			}
			if name == "" {
				// 集名不可得（规则匹配层模板未在该组成员上产出）→ 不落集
				// 归属，成员留给后续规则（先到先得以实际归属为准）。
				continue
			}
			for _, i := range g.members {
				out[i] = workSetAssignment{GroupKey: key, SetName: name, RuleID: plan.rule.ID}
				claimed[i] = true
			}
		}
	}
	return out
}
