// 分组求值两段（键集 → 容器成员）的单元测试：所在目录分组、序列群键分组、
// 手写拼接三种依据的键集与成员判定；容器边界（键集之外的规则命中文件不是
// 成员、未命中规则的文件即使键落在键集内也不是成员）；产生行缺席的空键集；
// 成员面与文件输入序无关（键集先算全再判成员）；取值源授权向内核透传。
package main

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// —— 测试构造件 ——

// keySetFileRow 构造文件目标条件行（模板 + 可选扩展名谓词）。
func keySetFileRow(tpl, ext string) CondxRow {
	row := CondxRow{Condition: &Condx{
		Target: CondxTargetFile,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}
	if ext != "" {
		row.Condition.Attach = []CondxAttach{{Kind: CondxAttachFileExt, Value: ext}}
	}
	return row
}

// keySetDirRow 构造目录目标条件行（模板）。
func keySetDirRow(tpl string) CondxRow {
	return CondxRow{Condition: &Condx{
		Target: CondxTargetDir,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}
}

// keySetSeqRow 构造序列目标条件行（模板）。
func keySetSeqRow(tpl string) CondxRow {
	return CondxRow{Condition: &Condx{
		Target: CondxTargetSeq,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: tpl},
	}}
}

// keySetRule 构造启用规则（纯平行条件行）。
func keySetRule(id string, rows ...CondxRow) RuleEntry {
	return RuleEntry{ID: id, Enabled: true, Conditions: rows}
}

// keySetAssertLegalBinding 断言夹具的分组绑定是合法形态（走档案载入的同一
// 校验）——正例夹具不得靠非法形态通过。
func keySetAssertLegalBinding(t *testing.T, rule RuleEntry, binding *GroupingBinding) {
	t.Helper()
	if err := validateGroupingBindingShape(rule.ID, binding, rule.Conditions); err != nil {
		t.Fatalf("规则 %q 的夹具分组绑定形态非法: %v", rule.ID, err)
	}
}

// keySetEvalFace 对一条纯平行行规则在文件面上逐文件逐行求命中（与各产出的
// 命中面同一口径：每行独立作用域、行模式按先序展平行号编译），返回命中面与
// 规则命中集（行命中池任一行命中即命中规则）。
func keySetEvalFace(t *testing.T, rule RuleEntry, files []ScanFile) ([]aggFileRowHits, []bool) {
	t.Helper()
	if condxHasBooleanShape(rule.Conditions) {
		t.Fatalf("规则 %q 的夹具须为纯平行条件行（布尔组合形态的命中判据归调用方）", rule.ID)
	}
	leaves := condxLeafRowsOf(rule.Conditions)
	rows := make([]Pattern, 0, len(leaves))
	for _, leaf := range leaves {
		p, err := compileCondx(leaf.row.Condition, leaf.no)
		if err != nil {
			t.Fatalf("编译规则 %q 第 %d 行失败: %v", rule.ID, leaf.no, err)
		}
		rows = append(rows, p)
	}
	surface := newAggEvalSurface(files, files)
	hits := make([]aggFileRowHits, len(files))
	matched := make([]bool, len(files))
	for i, file := range files {
		in := surface.matchInputOf(file)
		rowHits := make(aggFileRowHits, len(rows))
		for j, row := range rows {
			ctx := &ParseContext{DirNames: file.DirNames, Depth: len(file.DirNames)}
			if row.Evaluate(in, nil, ctx) {
				rowHits[j] = ctx
				matched[i] = true
			}
		}
		hits[i] = rowHits
	}
	return hits, matched
}

// keySetHitRows 把单文件命中面渲染为命中的行号序列（如 "13"=第 1、3 行命中；
// 空=全部未命中），供夹具自证。
func keySetHitRows(h aggFileRowHits) string {
	var sb strings.Builder
	for row := 1; row <= len(h); row++ {
		if h[row-1] != nil {
			fmt.Fprintf(&sb, "%d", row)
		}
	}
	return sb.String()
}

// keySetIndexOf 取相对路径在文件面中的下标（缺席即测试失败）。
func keySetIndexOf(t *testing.T, files []ScanFile, relPath string) int {
	t.Helper()
	for i := range files {
		if files[i].RelPath == relPath {
			return i
		}
	}
	t.Fatalf("文件面中未找到 %q", relPath)
	return -1
}

// keySetMembersByPath 把成员结论投影为 组键 → 成员相对路径清单（求值面序）。
func keySetMembersByPath(files []ScanFile, g groupingGroups) map[string][]string {
	out := make(map[string][]string, len(g.groups))
	for key, members := range g.groups {
		paths := make([]string, 0, len(members))
		for _, i := range members {
			paths = append(paths, files[i].RelPath)
		}
		out[key] = paths
	}
	return out
}

// keySetKeyByPath 把成员结论投影为 相对路径 → 组键（与文件输入序无关的成员
// 面形态，供乱序比对）。
func keySetKeyByPath(files []ScanFile, g groupingGroups) map[string]string {
	out := make(map[string]string)
	for key, members := range g.groups {
		for _, i := range members {
			out[files[i].RelPath] = key
		}
	}
	return out
}

// keySetAscending 生成 0..n-1 的下标序列。
func keySetAscending(n int) []int {
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	return order
}

// keySetPermute 按下标序列重排文件面（返回新切片，不改原切片）。
func keySetPermute(files []ScanFile, order []int) []ScanFile {
	out := make([]ScanFile, len(order))
	for k, i := range order {
		out[k] = files[i]
	}
	return out
}

// keySetShuffledOrders 生成一组确定的输入序（名称 → 下标序列）：原序、逆序，
// 以及固定种子的多次洗牌（结果可复现）。
func keySetShuffledOrders(n int) map[string][]int {
	orders := map[string][]int{"原序": keySetAscending(n)}
	reversed := make([]int, n)
	for i := range reversed {
		reversed[i] = n - 1 - i
	}
	orders["逆序"] = reversed
	rng := rand.New(rand.NewSource(20261124))
	for k := 1; k <= 5; k++ {
		order := keySetAscending(n)
		rng.Shuffle(n, func(a, b int) { order[a], order[b] = order[b], order[a] })
		orders[fmt.Sprintf("洗牌%d", k)] = order
	}
	return orders
}

// —— 所在目录分组 ——

// TestGroupingKeySetDirectory 所在目录分组的两段求值锚：键集=产生行（目录行）
// 命中文件的所在目录；成员=键集内目录的全部规则命中文件，包括只命中目录行
// 自身的文件（容器成员不要求命中别的行）；另一条目录行命中的目录、以及没有
// 任何目录行命中的目录都不在键集内，其文件即使命中文件行也不是成员；不命中
// 任何条件行的文件不是成员。
func TestGroupingKeySetDirectory(t *testing.T) {
	rule := keySetRule("keyset-dir",
		keySetDirRow("v{作品}"),         // 第 1 行：产生行（目录名以 v 起头）
		keySetFileRow("{作品}", ".jpg"), // 第 2 行
		keySetFileRow("{作品}", ".txt"), // 第 3 行
		keySetDirRow("w{作品}"),         // 第 4 行：另一条目录行
	)
	binding := &GroupingBinding{Mode: GroupingModeDirectory, Row: 1}
	keySetAssertLegalBinding(t, rule, binding)
	files := aggScanFiles(
		"va/a.jpg",     // 第 1、2 行
		"va/a.txt",     // 第 1、3 行
		"va/sub/b.png", // 仅第 1 行（祖先目录 va 命中）——容器成员
		"wa/c.jpg",     // 第 2、4 行：目录 wa 不在键集内
		"other/d.jpg",  // 仅第 2 行：目录 other 不在键集内
		"other/e.pdf",  // 不命中任何条件行
	)
	hits, matched := keySetEvalFace(t, rule, files)
	wantRows := []string{"12", "13", "1", "24", "2", ""}
	for i, want := range wantRows {
		if got := keySetHitRows(hits[i]); got != want {
			t.Fatalf("夹具命中面不符：%s 命中行 %q（期望 %q）", files[i].RelPath, got, want)
		}
	}
	if want := []bool{true, true, true, true, true, false}; !reflect.DeepEqual(matched, want) {
		t.Fatalf("夹具命中集不符: %v（期望 %v）", matched, want)
	}

	// 第一段：键集=产生行命中文件的所在目录。
	keys, constrained := groupingKeySetOf(binding, files, hits)
	if !constrained {
		t.Fatalf("所在目录分组应带键集")
	}
	keyVa, keyVaSub := dirGroupKey([]string{"va"}), dirGroupKey([]string{"va", "sub"})
	if want := map[string]bool{keyVa: true, keyVaSub: true}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("键集应恰为产生行命中文件的所在目录: %v（期望 %v）", keys, want)
	}

	// 第二段：成员=键集内目录的全部规则命中文件；组序=组内首成员出现序。
	got := groupingMembersOf(binding, files, hits, matched, groupKeySourceInline, nil)
	want := map[string][]string{keyVa: {"va/a.jpg", "va/a.txt"}, keyVaSub: {"va/sub/b.png"}}
	if gotMembers := keySetMembersByPath(files, got); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("成员应为键集内目录的全部命中文件: %v（期望 %v）", gotMembers, want)
	}
	if wantOrder := []string{keyVa, keyVaSub}; !reflect.DeepEqual(got.order, wantOrder) {
		t.Fatalf("组序应为组内首成员出现序: %q（期望 %q）", got.order, wantOrder)
	}
}

// —— 序列群键分组 ——

// TestGroupingKeySetSequenceGroupKey 序列群键分组的两段求值锚：键集=产生行
// （序列行）命中文件产出的群键；成员含同键的其它行文件——另一条序列行以
// 另一种写法产出相同群键的文件不命中产生行，也并入同一组；产生行从未产出
// 的群键不在键集内，其文件不是成员；无群组键产出的规则命中文件（仅命中普通
// 文件行）自身键不可得，不是成员。
func TestGroupingKeySetSequenceGroupKey(t *testing.T) {
	rule := keySetRule("keyset-seq",
		keySetSeqRow("{公共段}_{序号}"),    // 第 1 行：产生行
		keySetFileRow("{作品}", ".txt"), // 第 2 行：普通文件行
		keySetSeqRow("{公共段}-{序号}"),    // 第 3 行：另一条序列行（另一种写法）
	)
	binding := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 1, Capture: identityGroupKeyCapture},
	}}
	keySetAssertLegalBinding(t, rule, binding)
	files := aggScanFiles(
		"d/p_1.jpg", "d/p_2.jpg", // 第 1 行：群 p
		"d/q_1.jpg", "d/q_2.jpg", // 第 1 行：群 q
		"d/p-1.png", "d/p-2.png", // 第 3 行：群 p（同键跨写法）——不命中产生行
		"d/z-1.png", "d/z-2.png", // 第 3 行：群 z（产生行从未产出）
		"d/p.txt", // 仅第 2 行：无群组键产出
	)
	hits, matched := keySetEvalFace(t, rule, files)
	wantRows := []string{"1", "1", "1", "1", "3", "3", "3", "3", "2"}
	for i, want := range wantRows {
		if got := keySetHitRows(hits[i]); got != want {
			t.Fatalf("夹具命中面不符：%s 命中行 %q（期望 %q）", files[i].RelPath, got, want)
		}
	}
	if want := []bool{true, true, true, true, true, true, true, true, true}; !reflect.DeepEqual(matched, want) {
		t.Fatalf("夹具命中集不符: %v（期望 %v）", matched, want)
	}

	// 夹具自证：第 3 行的 p 群与第 1 行的 p 群键相同、与 q 群键相异。
	keyP := hits[keySetIndexOf(t, files, "d/p_1.jpg")][0].Group.Key
	keyQ := hits[keySetIndexOf(t, files, "d/q_1.jpg")][0].Group.Key
	if keyP == "" || keyP == keyQ {
		t.Fatalf("夹具的 p、q 两群键应非空且相异: %q / %q", keyP, keyQ)
	}
	if got := hits[keySetIndexOf(t, files, "d/p-1.png")][2].Group.Key; got != keyP {
		t.Fatalf("夹具的另一种写法应产出相同群键: %q（期望 %q）", got, keyP)
	}

	// 第一段：键集=产生行命中文件产出的群键。
	keys, constrained := groupingKeySetOf(binding, files, hits)
	if !constrained {
		t.Fatalf("序列群键分组应带键集")
	}
	if want := map[string]bool{keyP: true, keyQ: true}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("键集应恰为产生行产出的群键: %v（期望 %v）", keys, want)
	}

	// 第二段：成员含同键的其它行文件；组序=组内首成员出现序。
	got := groupingMembersOf(binding, files, hits, matched, groupKeySourceInline, nil)
	want := map[string][]string{
		keyP: {"d/p_1.jpg", "d/p_2.jpg", "d/p-1.png", "d/p-2.png"},
		keyQ: {"d/q_1.jpg", "d/q_2.jpg"},
	}
	if gotMembers := keySetMembersByPath(files, got); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("成员应含同键的其它行文件、不含产生行未产出的群: %v（期望 %v）", gotMembers, want)
	}
	if wantOrder := []string{keyP, keyQ}; !reflect.DeepEqual(got.order, wantOrder) {
		t.Fatalf("组序应为组内首成员出现序: %q（期望 %q）", got.order, wantOrder)
	}
}

// —— 手写拼接 ——

// TestGroupingKeySetHandwrittenIdentity 手写拼接分组与逐文件算键的取值等价锚：
// 多部分复合键的标识分组无键集——自身键可得的规则命中文件同键成组，成员面、
// 组序、组内成员序与「逐文件算分组键直接归组」逐项一致；键不可得的命中文件
// 两边都不入组。
func TestGroupingKeySetHandwrittenIdentity(t *testing.T) {
	rule := keySetRule("keyset-manual",
		keySetFileRow("{作品}_{序号}", ".png"), // 第 1 行
		keySetFileRow("{作品}_{序号}", ".txt"), // 第 2 行：同名捕获跨行汇流，与 png 同键
		keySetFileRow("{任意}", ".md"),       // 第 3 行：不绑定键部分的捕获名——键不可得
	)
	binding := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 1, Capture: "work"},
		{Row: 1, Capture: "seq"},
	}}
	keySetAssertLegalBinding(t, rule, binding)
	files := aggScanFiles("集/集_01.png", "集/集_01.txt", "集/集_02.png", "集/说明.md")
	hits, matched := keySetEvalFace(t, rule, files)
	wantRows := []string{"1", "2", "1", "3"}
	for i, want := range wantRows {
		if got := keySetHitRows(hits[i]); got != want {
			t.Fatalf("夹具命中面不符：%s 命中行 %q（期望 %q）", files[i].RelPath, got, want)
		}
	}

	// 第一段：手写拼接无键集（成员各自产出键）。
	keys, constrained := groupingKeySetOf(binding, files, hits)
	if keys != nil || constrained {
		t.Fatalf("手写拼接应无键集: keys=%v constrained=%v", keys, constrained)
	}

	// 第三段：与逐文件算键直接归组逐项一致。
	got := groupingMembersOf(binding, files, hits, matched, groupKeySourceInline, nil)
	direct := groupingGroups{groups: make(map[string][]int)}
	for i := range files {
		if !matched[i] {
			continue
		}
		key, ok := groupingSelfKeyOf(hits[i], binding, files[i], groupKeySourceInline, nil)
		if !ok {
			continue
		}
		if _, seen := direct.groups[key]; !seen {
			direct.order = append(direct.order, key)
		}
		direct.groups[key] = append(direct.groups[key], i)
	}
	if !reflect.DeepEqual(got, direct) {
		t.Fatalf("手写拼接的成员面/组序/组内序应与逐文件算键直接归组一致: %+v（期望 %+v）", got, direct)
	}
	want := map[string][]string{
		"集\x0001": {"集/集_01.png", "集/集_01.txt"},
		"集\x0002": {"集/集_02.png"},
	}
	if gotMembers := keySetMembersByPath(files, got); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("成员应为同键成组、键不可得者不入组: %v（期望 %v）", gotMembers, want)
	}
}

// —— 成员面与输入序无关 ——

// keySetSinglePassReference 反例参照：把键集与成员写成一次遍历（边收键边判
// 成员、只认已收入的键）——成员面随文件输入序变化。只用于证明乱序用例确能
// 抓住单遍实现，不是任何求值面的实现。
func keySetSinglePassReference(binding *GroupingBinding, files []ScanFile, hits []aggFileRowHits, matched []bool) map[string]string {
	out := make(map[string]string)
	keys := make(map[string]bool)
	row, container := groupingProducingRowOf(binding)
	for i := range files {
		if container {
			if ctx := groupingRowHit(hits, i, row); ctx != nil {
				if binding.Mode == GroupingModeDirectory {
					keys[dirGroupKey(files[i].DirNames)] = true
				} else if ctx.Group != nil {
					keys[ctx.Group.Key] = true
				}
			}
		}
		if i >= len(matched) || !matched[i] {
			continue
		}
		key, ok := groupingSelfKeyOf(hits[i], binding, files[i], groupKeySourceInline, nil)
		if !ok || (container && !keys[key]) {
			continue
		}
		out[files[i].RelPath] = key
	}
	return out
}

// TestGroupingKeySetOrderIndependent 成员面与文件输入序无关锚：同一批文件按
// 多种输入序进入求值面（逆序会把不命中产生行的容器成员排在产出键的文件之
// 前；另有固定种子的多次洗牌），键集与「相对路径 → 组键」的成员面逐种完全
// 一致，组内成员恒按求值面下标升序——键集先一次遍历算全、成员另一次遍历
// 判定，先见到的文件不决定键集。所在目录分组与序列群键分组各自过一遍；每
// 种输入序的命中面在该序上重新求值。序列群键分组存在不命中产生行的容器成员
// （另一种写法的同键文件），逆序下反例参照（单遍实现）的成员面必须与两段
// 结果不同——证明本用例确能抓住单遍实现；所在目录分组的键集目录对目录下
// 全部文件成立，成员都命中产生行、各自产出键，不存在这类成员，不作反例参照。
func TestGroupingKeySetOrderIndependent(t *testing.T) {
	dirRule := keySetRule("keyset-order-dir",
		keySetDirRow("v{作品}"),         // 第 1 行：产生行
		keySetFileRow("{作品}", ".jpg"), // 第 2 行
	)
	dirBinding := &GroupingBinding{Mode: GroupingModeDirectory, Row: 1}
	seqRule := keySetRule("keyset-order-seq",
		keySetSeqRow("{公共段}_{序号}"), // 第 1 行：产生行
		keySetSeqRow("{公共段}-{序号}"), // 第 2 行：另一种写法
	)
	seqBinding := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 1, Capture: identityGroupKeyCapture},
	}}
	cases := []struct {
		name    string
		rule    RuleEntry
		binding *GroupingBinding
		paths   []string
		// containerOnlyMembers 报告该形态的夹具是否含不命中产生行的容器成员
		// （含则逆序下单遍实现必然漏收，可作反例参照）。
		containerOnlyMembers bool
	}{
		{"所在目录分组", dirRule, dirBinding, []string{
			"va/a.jpg", "va/a.txt", "va/sub/b.png", "wa/c.jpg", "other/d.jpg",
		}, false},
		{"序列群键分组", seqRule, seqBinding, []string{
			"d/p_1.jpg", "d/p_2.jpg", "d/q_1.jpg", "d/q_2.jpg",
			"d/p-1.png", "d/p-2.png", "d/z-1.png", "d/z-2.png",
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keySetAssertLegalBinding(t, tc.rule, tc.binding)
			files := aggScanFiles(tc.paths...)
			hits, matched := keySetEvalFace(t, tc.rule, files)
			wantKeys, constrained := groupingKeySetOf(tc.binding, files, hits)
			if !constrained || len(wantKeys) == 0 {
				t.Fatalf("夹具应带非空键集: constrained=%v keys=%v", constrained, wantKeys)
			}
			wantByKey := keySetKeyByPath(files, groupingMembersOf(tc.binding, files, hits, matched, groupKeySourceInline, nil))
			if len(wantByKey) == 0 {
				t.Fatalf("夹具应产出成员")
			}
			for name, order := range keySetShuffledOrders(len(files)) {
				shuffled := keySetPermute(files, order)
				shuffledHits, shuffledMatched := keySetEvalFace(t, tc.rule, shuffled)
				if gotKeys, _ := groupingKeySetOf(tc.binding, shuffled, shuffledHits); !reflect.DeepEqual(gotKeys, wantKeys) {
					t.Fatalf("输入序「%s」下键集应不变: %v（期望 %v）", name, gotKeys, wantKeys)
				}
				got := groupingMembersOf(tc.binding, shuffled, shuffledHits, shuffledMatched, groupKeySourceInline, nil)
				if gotByKey := keySetKeyByPath(shuffled, got); !reflect.DeepEqual(gotByKey, wantByKey) {
					t.Fatalf("输入序「%s」下成员面应不变: %v（期望 %v）", name, gotByKey, wantByKey)
				}
				for key, members := range got.groups {
					for k := 1; k < len(members); k++ {
						if members[k] <= members[k-1] {
							t.Fatalf("输入序「%s」下组 %q 的成员应按求值面下标升序: %v", name, key, members)
						}
					}
				}
			}
			// 反例参照：逆序下，单遍实现的成员面与两段结果不同。
			if !tc.containerOnlyMembers {
				return
			}
			reversed := keySetPermute(files, keySetShuffledOrders(len(files))["逆序"])
			reversedHits, reversedMatched := keySetEvalFace(t, tc.rule, reversed)
			if single := keySetSinglePassReference(tc.binding, reversed, reversedHits, reversedMatched); reflect.DeepEqual(single, wantByKey) {
				t.Fatalf("反例参照（单遍实现）在逆序下的成员面应与两段结果不同——否则本用例抓不住单遍实现: %v", single)
			}
		})
	}
}

// —— 产生行缺席 ——

// TestGroupingKeySetMissingProducingRow 产生行缺席锚：所在目录分组缺产生行
// 行号时键集为空、无成员，不回退到任何目录行——该形态被档案载入校验拒绝，
// 只会出现在未经载入校验的内存规则里；产生行行号或序列群键部分的行号超出
// 条件行范围同样空键集无成员；绑定缺席无分组可依、无成员。同一命中面换上
// 合法产生行即有成员（证明空结果源于产生行缺席而非夹具）。
func TestGroupingKeySetMissingProducingRow(t *testing.T) {
	rule := keySetRule("keyset-norow",
		keySetDirRow("v{作品}"),         // 第 1 行
		keySetFileRow("{作品}", ".jpg"), // 第 2 行
	)
	files := aggScanFiles("va/a.jpg", "va/b.png", "other/c.jpg")
	hits, matched := keySetEvalFace(t, rule, files)
	wantRows := []string{"12", "1", "2"}
	for i, want := range wantRows {
		if got := keySetHitRows(hits[i]); got != want {
			t.Fatalf("夹具命中面不符：%s 命中行 %q（期望 %q）", files[i].RelPath, got, want)
		}
	}

	assertNoMembers := func(name string, binding *GroupingBinding) {
		t.Helper()
		keys, constrained := groupingKeySetOf(binding, files, hits)
		if !constrained || len(keys) != 0 {
			t.Fatalf("%s应为空键集（键集在场、无键）: constrained=%v keys=%v", name, constrained, keys)
		}
		if got := groupingMembersOf(binding, files, hits, matched, groupKeySourceInline, nil); len(got.order) != 0 || len(got.groups) != 0 {
			t.Fatalf("%s应无成员: order=%q groups=%v", name, got.order, got.groups)
		}
	}
	missing := &GroupingBinding{Mode: GroupingModeDirectory}
	if err := validateGroupingBindingShape(rule.ID, missing, rule.Conditions); err == nil {
		t.Fatalf("缺产生行的所在目录分组应被载入校验拒绝（夹具前提）")
	}
	assertNoMembers("缺产生行行号的所在目录分组", missing)
	assertNoMembers("产生行行号越界的所在目录分组", &GroupingBinding{Mode: GroupingModeDirectory, Row: 9})

	seqRule := keySetRule("keyset-norow-seq", keySetSeqRow("{公共段}_{序号}"))
	seqFiles := aggScanFiles("d/p_1.jpg", "d/p_2.jpg")
	seqHits, seqMatched := keySetEvalFace(t, seqRule, seqFiles)
	seqBinding := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 5, Capture: identityGroupKeyCapture},
	}}
	if keys, constrained := groupingKeySetOf(seqBinding, seqFiles, seqHits); !constrained || len(keys) != 0 {
		t.Fatalf("行号越界的序列群键分组应为空键集: constrained=%v keys=%v", constrained, keys)
	}
	if got := groupingMembersOf(seqBinding, seqFiles, seqHits, seqMatched, groupKeySourceInline, nil); len(got.order) != 0 || len(got.groups) != 0 {
		t.Fatalf("行号越界的序列群键分组应无成员: order=%q groups=%v", got.order, got.groups)
	}

	// 绑定缺席：无分组可依。
	if keys, constrained := groupingKeySetOf(nil, files, hits); keys != nil || constrained {
		t.Fatalf("绑定缺席应无键集: keys=%v constrained=%v", keys, constrained)
	}
	if got := groupingMembersOf(nil, files, hits, matched, groupKeySourceInline, nil); len(got.order) != 0 || len(got.groups) != 0 {
		t.Fatalf("绑定缺席应无成员: order=%q groups=%v", got.order, got.groups)
	}

	// 对照：合法产生行之下，目录 va 的命中文件（含仅命中目录行的 b.png）成组；
	// 目录 other 不在键集内，c.jpg 不是成员。
	legal := &GroupingBinding{Mode: GroupingModeDirectory, Row: 1}
	keySetAssertLegalBinding(t, rule, legal)
	got := groupingMembersOf(legal, files, hits, matched, groupKeySourceInline, nil)
	want := map[string][]string{dirGroupKey([]string{"va"}): {"va/a.jpg", "va/b.png"}}
	if gotMembers := keySetMembersByPath(files, got); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("合法产生行下成员应仅含键集目录的命中文件: %v（期望 %v）", gotMembers, want)
	}
}

// —— 成员须规则命中 ——

// TestGroupingKeySetRequiresRuleHit 成员须规则命中锚：自身键落在键集内的文件
// 只有命中规则才是成员——命中集短于文件面时越界文件按未命中处理。布尔组合
// 形态下规则命中集可含行命中面全空的文件（整条求值门通过）：所在目录分组的
// 自身键恒可得、按目录键入组；序列群键分组的自身键不可得、不入组。
func TestGroupingKeySetRequiresRuleHit(t *testing.T) {
	files := aggScanFiles("va/a.jpg", "va/b.jpg", "va/c.jpg", "va/d.jpg")
	dirCtx := func() *ParseContext { return &ParseContext{DirNames: []string{"va"}} }
	// 命中面：a 命中第 1 行（产生行）；b 仅命中第 2 行；c 行命中面全空；d 未求值。
	// 命中集：a、b、c 命中规则；d 越界=未命中。
	dirHits := []aggFileRowHits{
		{dirCtx(), nil},
		{nil, dirCtx()},
		{nil, nil},
		nil,
	}
	matched := []bool{true, true, true}
	keyVa := dirGroupKey([]string{"va"})

	dirBinding := &GroupingBinding{Mode: GroupingModeDirectory, Row: 1}
	got := groupingMembersOf(dirBinding, files, dirHits, matched, groupKeySourceInline, nil)
	want := map[string][]string{keyVa: {"va/a.jpg", "va/b.jpg", "va/c.jpg"}}
	if gotMembers := keySetMembersByPath(files, got); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("所在目录分组的成员应为键集目录内的全部规则命中文件: %v（期望 %v）", gotMembers, want)
	}

	seqHits := []aggFileRowHits{
		{&ParseContext{Group: &GroupAnalysis{Key: "p"}}, nil},
		{nil, &ParseContext{Group: &GroupAnalysis{Key: "p"}}},
		{nil, nil},
		nil,
	}
	seqBinding := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 1, Capture: identityGroupKeyCapture},
	}}
	got = groupingMembersOf(seqBinding, files, seqHits, matched, groupKeySourceInline, nil)
	want = map[string][]string{"p": {"va/a.jpg", "va/b.jpg"}}
	if gotMembers := keySetMembersByPath(files, got); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("序列群键分组的成员应为自身键落在键集内的规则命中文件: %v（期望 %v）", gotMembers, want)
	}
}

// —— 取值源授权 ——

// TestGroupingKeySetLibrarySourceHandedThrough 取值源授权透传锚：序列群键保留
// 名的取值源由调用方按消费产出形态授权——库级取值源形态下，行内无群产物但
// 库级群卡键落在键集内的规则命中文件是容器成员；行内独占形态下同一文件不
// 入组。两种形态的键集相同（键集只由产生行的行内群产出构成）。
func TestGroupingKeySetLibrarySourceHandedThrough(t *testing.T) {
	binding := &GroupingBinding{Mode: GroupingModeIdentity, Identity: []IdentityPart{
		{Row: 1, Capture: identityGroupKeyCapture},
	}}
	files := aggScanFiles("d/p_1.jpg", "d/p_1.txt", "d/p_2.txt")
	// 命中面：p_1.jpg 命中产生行并产出群 p；两件 txt 仅命中另一行、行内无群产物。
	hits := []aggFileRowHits{
		{&ParseContext{Group: &GroupAnalysis{Key: "p"}}, nil},
		{nil, &ParseContext{DirNames: []string{"d"}}},
		{nil, &ParseContext{DirNames: []string{"d"}}},
	}
	matched := []bool{true, true, true}
	library := libraryGroupKeyIndex{"d/p_1.txt": "p", "d/p_2.txt": "r"}

	inline := groupingMembersOf(binding, files, hits, matched, groupKeySourceInline, nil)
	want := map[string][]string{"p": {"d/p_1.jpg"}}
	if gotMembers := keySetMembersByPath(files, inline); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("行内独占形态下无行内群产物的文件不应入组: %v（期望 %v）", gotMembers, want)
	}

	// 库级取值源：p_1.txt 的库级群卡键 p 落在键集内 → 并入群 p；p_2.txt 的库级
	// 群卡键 r 不在键集内 → 不入组。
	withLibrary := groupingMembersOf(binding, files, hits, matched, groupKeySourceLibraryFallback, library)
	want = map[string][]string{"p": {"d/p_1.jpg", "d/p_1.txt"}}
	if gotMembers := keySetMembersByPath(files, withLibrary); !reflect.DeepEqual(gotMembers, want) {
		t.Fatalf("库级取值源形态下库级群卡键落在键集内的文件应并入该组: %v（期望 %v）", gotMembers, want)
	}
}
