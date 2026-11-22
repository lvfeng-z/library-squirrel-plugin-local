// 作品集聚合（产出四）求值内核：规则命中文件的集归属解析。汇流后的独立通道
// （与聚合段同相位、居其后）：不进逐文件求值循环、不读盘——输入=规则命中面
// （布尔门后的命中集，与聚合段同一命中集机制），输出=成员级集归属结论
// （组键+集名+规则ID+条目序）。
//
// 条目化求值（阶段2）：规则级「作品集聚合」为条目列表，每条 = 一个分组绑定 +
// 一个集名来源。规则命中面按规则求一次（同规则各条目共用），随后条目按声明序
// 逐条求值：每条按其绑定的分组求组键——复用统一分组求值器的库级取值源形态
// aggFileRowHits.groupingKeyOfWithLibrarySource（标识分组的序列群键保留名行内
// 序列行群产物优先、库级群卡键兜底；所在目录=目录聚合键），组内文件各成作品；
// 集名取该条目 NameExtraction（行号+捕获名）在组成员的命中行上取到的捕获值
// （不再读规则级集维度声明——集名的唯一出口是条目集名来源）。分组键或集名
// 不可得的组不入集（孤文件回退=独立作品），与伴随说明文件「组内无落点则独立
// 作品」的既有语义同款。
//
// 竞争（先到先得）：既有聚合归属优先——已被聚合收编或说明成员标记的文件不入集
// （聚合段在先、其归属不改判）；候选按「规则档案序 → 规则内条目声明序」逐条
// 归集，同一文件被在先条目归集后不再改判（条目序小的胜出，后续条目跳过该文件；
// 与聚合段首个命中定群同款取向）。归属只在实际落定（组键与集名皆可得）时生效，
// 未落定的文件仍可被后续条目/规则接纳。
package main

// workSetAssignment 单文件的集归属结论（产出四求值产物）：GroupKey 为按条目
// 归集依据算出的组键（身份复合键 NUL 连接／序列群键／目录聚合键），SetName 为
// 该组在该条目集名来源上取到的捕获值，RuleID 为产出该归属的规则，EntryIndex
// 为规则内的条目序（0 起，声明序——同文件多条目按此序先到先得，消费面据此
// 定位所绑分组与条目的供序形态）。GroupKey/SetName/RuleID 同空=不入集；消费面
// 按「GroupKey+SetName」识别一个集，按 RuleID 追溯产出规则。
type workSetAssignment struct {
	GroupKey   string
	SetName    string
	RuleID     string
	EntryIndex int
}

// workSetEntryPlan 单条「作品集聚合」条目的求值预案：index 为条目在规则内的
// 声明序（0 起，同文件多条目先到先得的凭据），grouping 为该条目的分组绑定
// （组键求值依据），name 为该条目的集名来源（行号+捕获名）。
type workSetEntryPlan struct {
	index    int
	grouping *GroupingBinding
	name     *NameExtraction
}

// workSetRulePlan 单条产出四规则的归集求值预案：条件行（先序展平叶子行）编译
// 为独立求值的行包装模式（规则命中面供该规则全部条目共用），布尔组合形态另
// 编译整条求值门（gate，与聚合段同一接线；纯平行行不设门=行命中池 OR 汇聚即
// 命中面）。seq 为规则在规则表中的下标（档案序先到先得的凭据）；entries 为按
// 声明序的条目预案（每条自带分组绑定与集名来源）。
type workSetRulePlan struct {
	rule    *RuleEntry
	seq     int
	rows    []Pattern
	gate    Pattern
	entries []workSetEntryPlan
}

// buildWorkSetPlans 过滤启用且携带产出四的规则并编译预案；行编译或布尔门编译
// 失败按宁缺勿错整条跳过（载入校验已拦的形态在此双保险，与聚合段同款防御）。
// 条目预案逐条收取：分组绑定与集名来源缺一不可（载入/保存校验已拦，此处按
// 缺项条目跳过、其余条目照常求值），条目序取声明下标（与界面卡片序一致）。
func buildWorkSetPlans(rules []RuleEntry) []*workSetRulePlan {
	var plans []*workSetRulePlan
	for i := range rules {
		rule := &rules[i]
		if !rule.Enabled || len(rule.WorkSetAggregation) == 0 || len(rule.Conditions) == 0 {
			continue
		}
		entries := make([]workSetEntryPlan, 0, len(rule.WorkSetAggregation))
		for j := range rule.WorkSetAggregation {
			entry := &rule.WorkSetAggregation[j]
			if entry.Grouping == nil || entry.NameExtraction == nil {
				continue
			}
			entries = append(entries, workSetEntryPlan{
				index:    j,
				grouping: entry.Grouping,
				name:     entry.NameExtraction,
			})
		}
		if len(entries) == 0 {
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
			rule:    rule,
			seq:     i,
			rows:    rows,
			gate:    gate,
			entries: entries,
		})
	}
	return plans
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

// workSetEntryNameOf 取该文件在本条目集名来源上的捕获值：行号越界、该行未命中
// 或捕获未绑定即空（该成员在本条目上无集名可用）。行号口径同 IdentityPart——
// 行命中面下标=行号-1，捕获以行号前缀（rN.组名）并入行命中上下文。
func (h aggFileRowHits) workSetEntryNameOf(name *NameExtraction) string {
	if name == nil || name.Row < 1 || name.Row > len(h) {
		return ""
	}
	ctx := h[name.Row-1]
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Capture(rowCaptureName(name.Row, name.Capture))
	return v
}

// workSetAggregation 产出四归集主算法（纯函数、输入序确定）：规则按档案序、
// 规则内条目按声明序逐条求值——条目按其分组把命中的未占用文件按组键归组，
// 键相等者成组；组内集名取该条目集名来源在首个可产出成员上的捕获值，集名
// 不可得时整组不落归属；落定成员写入结论并锁定（同文件多条目按条目序先到
// 先得）。surface 与 files 按下标对齐（调用方保证同一清单）。
func workSetAggregation(surface *aggEvalSurface, rules []RuleEntry, pred *LibraryIndex, groupKeys libraryGroupKeyIndex, taken []bool) []workSetAssignment {
	files := surface.files
	out := make([]workSetAssignment, len(files))
	plans := buildWorkSetPlans(rules)
	if len(plans) == 0 {
		return out
	}
	// 辅助表与求值面按下标对齐（调用方保证同源）；越界读取按「未占用」处理，
	// 防御不同源调用（正常路径恒等长）。
	takenOf := func(i int) bool { return i < len(taken) && taken[i] }
	claimed := make([]bool, len(files))
	// available 报告该文件是否仍可被本条目录取：既有归属与已落定归属皆不改判。
	available := func(i int) bool { return !takenOf(i) && !claimed[i] && out[i].GroupKey == "" }
	for _, plan := range plans {
		// 规则命中面（行命中池/布尔门）逐规则求一次，供该规则的全部条目共用：
		// 同规则各条目只差分组绑定与集名来源，命中集相同。
		hits := make([]aggFileRowHits, len(files))
		matched := make([]bool, len(files))
		for i := range files {
			if !available(i) {
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
				// 门过即命中：行命中面只供组键/集名取值（拒绝门内的否定行可令
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
		// 条目按声明序逐条求值：每条按其分组绑定求组键、取自己的集名来源；
		// 组内文件各成作品，先落定的条目占用文件——后续条目跳过该文件。
		for _, entry := range plan.entries {
			// 分组收集：组键可得者按组键归组，组序=组键首成员出现序（确定性）。
			type workSetGroup struct {
				members []int
			}
			groups := make(map[string]*workSetGroup)
			var order []string
			for i := range files {
				if !matched[i] || !available(i) {
					continue
				}
				key, ok := hits[i].groupingKeyOfWithLibrarySource(entry.grouping, files[i].DirNames, files[i].RelPath, groupKeys)
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
					if v := hits[i].workSetEntryNameOf(entry.name); v != "" {
						name = v
						break
					}
				}
				if name == "" {
					// 集名不可得（条目的集名来源行不在该组成员命中面上）→ 不落
					// 集归属，成员留给后续条目/规则（先到先得以实际归属为准）。
					continue
				}
				for _, i := range g.members {
					out[i] = workSetAssignment{GroupKey: key, SetName: name, RuleID: plan.rule.ID, EntryIndex: entry.index}
					claimed[i] = true
				}
			}
		}
	}
	return out
}
