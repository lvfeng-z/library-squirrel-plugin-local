package main

import (
	"testing"
)

// TestEditedRuleOmittedRowStillExtracts 用户实测缺陷回归锚（期3-I，2026-10-25）：
// 前端规则编辑保存的提取动作第 1 行捕获引用合法省略 row 字段（rowsToActions
// 仅行>1 才带 row）——档案载入后 Row=0，Apply 求值期曾按裸名直取捕获（v4
// 上下文捕获键恒为 rN. 前缀）恒落空，规则「命中但零声明」。修复=Row≤0 按
// 第 1 行兜底（与 validateExtractions rowPrefixed 校验口径一致）。
func TestEditedRuleOmittedRowStillExtracts(t *testing.T) {
	root := t.TempDir()
	m, _ := dirAssignFixture(t, root)
	writeTestFile(t, root+"/Artist[GlukMaster69]/burnice/pic_01.jpg", "g1")
	writeTestFile(t, root+"/Artist[Isel]/set/pic_01.jpg", "i1")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("识别失败: %v", err)
	}

	// 手动赋值→泛化（产物 Row:1 显式）——验证轮后删除会话前的规则行为基线
	if _, _, err := m.ApplyDirAssignment(root, "Artist[GlukMaster69]", SessionAssignment{Author: "GlukMaster69"}); err != nil {
		t.Fatalf("目录赋值失败: %v", err)
	}
	if _, err := m.ApplyDirGeneralization(root, "Artist[GlukMaster69]", generalizeModeStruct); err != nil {
		t.Fatalf("泛化失败: %v", err)
	}
	profile, perrs := LoadProfile(m.store)
	if profile == nil || len(perrs) > 0 {
		t.Fatalf("泛化后档案不可载入: %v", perrs)
	}
	var id string
	for i := range profile.Rules {
		r := &profile.Rules[i]
		if len(r.Conditions) == 1 && r.Conditions[0].Condition != nil &&
			r.Conditions[0].Condition.Format.Tpl == "Artist[{作者}]" {
			id = r.ID
		}
	}
	if id == "" {
		t.Fatal("档案中无 Artist[{{作者}}] 模板规则")
	}
	if err := m.UpdateProfile(ArchiveWriteRuleEdit, func(p *RuleProfile) error {
		for i := range p.Rules {
			if p.Rules[i].ID == id {
				for j := range p.Rules[i].Extractions {
					p.Rules[i].Extractions[j].Row = 0
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("编辑保存失败: %v", err)
	}

	// 赋值清除（会话重建口径）后重识别：规则必须独立产出 author（含跨目录
	// 推广）——修复前此处全部落空（命中但零声明）
	if _, _, err := m.ClearDirAssignments(root, "Artist[GlukMaster69]"); err != nil {
		t.Fatalf("清除赋值失败: %v", err)
	}
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("重识别失败: %v", err)
	}
	sess, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	for rel, want := range map[string]string{
		"Artist[GlukMaster69]/burnice/pic_01.jpg": "GlukMaster69",
		"Artist[Isel]/set/pic_01.jpg":             "Isel",
	} {
		f := snapshotFileOf(sess.Snapshot, rel)
		if f == nil {
			t.Fatalf("快照内无文件 %s", rel)
		}
		var author string
		matched := false
		for _, mid := range f.MatchedRuleIDs {
			if mid == id {
				matched = true
			}
		}
		for _, d := range f.Final {
			if string(d.Dimension) == string(DimAuthor) {
				author = d.Value
			}
		}
		if !matched {
			t.Errorf("%s 应命中规则 %s（matched=%v）", rel, id, f.MatchedRuleIDs)
		}
		if author != want {
			t.Errorf("%s 终选 author=%q，期望 %q（省略 row 的提取动作必须兜底第 1 行取值）", rel, author, want)
		}
	}
}
