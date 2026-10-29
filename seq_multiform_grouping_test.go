package main

// 多行序列规则的定群回归锚（2026-10-29 开发环境实测缺陷修复）：两行 or 并列
// 的序列定群规则（qna-seq-たじ…，两种分隔形态各一行）此前不识别
// `[Isel]_[107319137]_アセンダー_[01]-[03]` 序列——isGroupingRule 的 v4 分支
// 曾要求「恰一行条件」，多行全 seq 规则被排除出分组段，decision.Group 只在
// 分组段回流（提取段命中虽经 absorbScoped 写 ctx.Group 但无人消费）——规则
// 命中计数可见而群卡/序列身份缺席、退化为结构群补位。修复后多行全 seq 规则
// 参与定群，序号原文逐主体试配。锚定两种形态成员的群身份/定群规则/序号/规模。
import (
	"testing"
)

func TestRealProfileSeqRuleMultiRowGrouping(t *testing.T) {
	profile, errs := ParseProfile([]byte(realComboProfile))
	if profile == nil {
		t.Fatalf("档案不可载入: %v", errs)
	}
	seqRuleID := "qna-seq-たじ-57719012-ボテ詰め合わせ-1793038910903"

	// 行1 形态（_[01] 下划线+括号序号）三成员。
	row1Files := []string{
		"[Isel]_[107319137]_アセンダー_[01].png",
		"[Isel]_[107319137]_アセンダー_[02].png",
		"[Isel]_[107319137]_アセンダー_[03].png",
	}
	// 行2 形态（空格分隔+裸序号）两成员。
	row2Files := []string{
		"[Rira] [99881] [夜の散歩]1.jpg",
		"[Rira] [99881] [夜の散歩]2.jpg",
	}

	siblings := append(append([]string{}, row1Files...), row2Files...)
	for _, tc := range []struct {
		file string
		seq  int
	}{
		{row1Files[0], 1}, {row1Files[1], 2}, {row1Files[2], 3},
		{row2Files[0], 1}, {row2Files[1], 2},
	} {
		in := &MatchInput{
			FileName: tc.file, DirNames: []string{"Artist[谁]"},
			RelPath: "Artist[谁]/" + tc.file, Siblings: siblings,
		}
		c := mergedFileConclusionOf(in, profile.Rules, nil, nil)
		if c.Group == nil {
			t.Fatalf("%s 应被两行序列规则定群，实际群组为空（matched=%v）", tc.file, c.MatchedRuleIDs)
		}
		if c.GroupRuleID != seqRuleID {
			t.Fatalf("%s 定群规则应为 %s，实际 %q", tc.file, seqRuleID, c.GroupRuleID)
		}
		if c.Group.Seq != tc.seq {
			t.Fatalf("%s 群内序号应为 %d，实际 %d", tc.file, tc.seq, c.Group.Seq)
		}
		wantSize := len(row1Files)
		if seqInFiles(row2Files, tc.file) {
			wantSize = len(row2Files)
		}
		if c.Group.Size != wantSize {
			t.Fatalf("%s 群规模应为 %d，实际 %d", tc.file, wantSize, c.Group.Size)
		}
		if c.GroupSeqText == "" {
			t.Fatalf("%s 序号原文应随结论携带", tc.file)
		}
	}
}

func seqInFiles(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
