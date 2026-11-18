// 伴随说明文件内容采集通路的单元测试：采集算法（64KB 截断+二进制嗅探+编码
// 回退链）、命中集批量采集与求值输入面注入、快照游标铸造与增量失配拉入。
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// mustStatCursor 取文件当前 stat 的内容游标（失配测试造数用）。
func mustStatCursor(t *testing.T, fullPath string) companionContentCursor {
	t.Helper()
	fi, err := os.Stat(fullPath)
	if err != nil {
		t.Fatalf("stat %s 失败: %v", fullPath, err)
	}
	return companionContentCursorOf(fi)
}

// TestCompanionContentCollectEncodings 编码回退链：BOM 判定 → UTF-8 合法性
// 校验 → GBK 解码 → 不可解码字节按替换字符读入。
func TestCompanionContentCollectEncodings(t *testing.T) {
	root := t.TempDir()
	utf16Payload := func(text string, bigEndian bool) []byte {
		units := utf16.Encode([]rune(text))
		out := make([]byte, 0, 2+2*len(units))
		if bigEndian {
			out = append(out, 0xFE, 0xFF)
		} else {
			out = append(out, 0xFF, 0xFE)
		}
		for _, u := range units {
			if bigEndian {
				out = append(out, byte(u>>8), byte(u))
			} else {
				out = append(out, byte(u), byte(u>>8))
			}
		}
		return out
	}
	gbkPayload := func(text string) []byte {
		out, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(text))
		if err != nil {
			t.Fatalf("GBK 编码 %q 失败: %v", text, err)
		}
		return out
	}
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"无BOM的UTF-8", []byte("作者：甲\n简介内容"), "作者：甲\n简介内容"},
		{"UTF-8 BOM", append([]byte{0xEF, 0xBB, 0xBF}, []byte("带BOM的说明")...), "带BOM的说明"},
		{"UTF-16 LE BOM", utf16Payload("小端说明文本", false), "小端说明文本"},
		{"UTF-16 BE BOM", utf16Payload("大端说明文本", true), "大端说明文本"},
		{"UTF-16 奇数尾字节", append(utf16Payload("成对", false), 'X'), "成对"},
		{"GBK 中文", gbkPayload("中文说明内容：标签与作者"), "中文说明内容：标签与作者"},
		{"不可判别按替换字符读入", []byte{'o', 'k', 0xFF, 'f', 'i', 'n', 'e'}, "ok\ufffdfine"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			full := filepath.Join(root, strings.ReplaceAll(tc.name, "/", "_")+".txt")
			if err := os.WriteFile(full, tc.data, 0o644); err != nil {
				t.Fatalf("写入测试文件失败: %v", err)
			}
			sample, _, ok := collectCompanionContent(full)
			if !ok {
				t.Fatalf("文本文件不应被拒读: %s", full)
			}
			if sample.Truncated {
				t.Fatal("小文件不应截断")
			}
			if sample.Text != tc.want {
				t.Fatalf("解码结果 = %q, 期望 %q", sample.Text, tc.want)
			}
		})
	}
}

// TestCompanionContentCollectBinarySniff 二进制嗅探：首段含 NUL 字节即拒读
// （零内容零声明），游标仍然返回（拒读不是永久态）。
func TestCompanionContentCollectBinarySniff(t *testing.T) {
	root := t.TempDir()
	full := filepath.Join(root, "bin.dat")
	if err := os.WriteFile(full, []byte("ab\x00cd"), 0o644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}
	sample, cursor, ok := collectCompanionContent(full)
	if ok {
		t.Fatal("含 NUL 字节的文件应被二进制嗅探拒读")
	}
	if sample.Text != "" || sample.Truncated {
		t.Fatalf("拒读应零内容零截断标记: %+v", sample)
	}
	if cursor.Size != 5 || cursor.ModTime == 0 {
		t.Fatalf("拒读者的游标应仍然返回: %+v", cursor)
	}
}

// TestCompanionContentCollectTruncation 尺寸上限：恰满上限不截断，超限截断
// 且保留量等于上限。
func TestCompanionContentCollectTruncation(t *testing.T) {
	root := t.TempDir()
	exact := filepath.Join(root, "exact.txt")
	if err := os.WriteFile(exact, bytes.Repeat([]byte("a"), companionContentMaxBytes), 0o644); err != nil {
		t.Fatalf("写入恰满上限文件失败: %v", err)
	}
	sample, _, ok := collectCompanionContent(exact)
	if !ok || sample.Truncated || len(sample.Text) != companionContentMaxBytes {
		t.Fatalf("恰满上限不应截断: ok=%v truncated=%v len=%d", ok, sample.Truncated, len(sample.Text))
	}
	over := filepath.Join(root, "over.txt")
	if err := os.WriteFile(over, bytes.Repeat([]byte("b"), companionContentMaxBytes+1), 0o644); err != nil {
		t.Fatalf("写入超限文件失败: %v", err)
	}
	sample, cursor, ok := collectCompanionContent(over)
	if !ok || !sample.Truncated || len(sample.Text) != companionContentMaxBytes {
		t.Fatalf("超限应截断至上限: ok=%v truncated=%v len=%d", ok, sample.Truncated, len(sample.Text))
	}
	if cursor.Size != companionContentMaxBytes+1 {
		t.Fatalf("游标尺寸应为文件全量尺寸（截断只限片段）: %+v", cursor)
	}
}

// TestCompanionContentHitSetCollection 命中集批量采集：一次采齐、清单外路径
// 跳过、拒读者无片段但有游标。
func TestCompanionContentHitSetCollection(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "说明.txt"), "作品说明内容")
	writeTestFile(t, filepath.Join(root, "图.bin"), "x\x00y")
	allFiles := []FileEntry{
		{FullPath: filepath.Join(root, "说明.txt"), RelPath: "说明.txt"},
		{FullPath: filepath.Join(root, "图.bin"), RelPath: "图.bin"},
		{FullPath: filepath.Join(root, "别处.jpg"), RelPath: "别处.jpg"},
	}
	samples, cursors := collectCompanionContentSet([]string{"说明.txt", "图.bin", "已消失.txt", "说明.txt"}, allFiles)
	if s, ok := samples["说明.txt"]; !ok || s.Text != "作品说明内容" || s.Truncated {
		t.Fatalf("命中集内文本文件应采得片段: %+v ok=%v", s, ok)
	}
	if _, ok := samples["图.bin"]; ok {
		t.Fatal("二进制嗅探拒读不应有内容片段")
	}
	if _, ok := samples["已消失.txt"]; ok {
		t.Fatal("清单外路径不应有内容片段")
	}
	if len(samples) != 1 {
		t.Fatalf("片段表应恰一项（去重+清单过滤）: %+v", samples)
	}
	if _, ok := cursors["说明.txt"]; !ok {
		t.Fatal("文本文件应有游标")
	}
	if _, ok := cursors["图.bin"]; !ok {
		t.Fatal("拒读文件也应记游标（拒读不是永久态）")
	}
	if _, ok := cursors["已消失.txt"]; ok {
		t.Fatal("清单外路径不应有游标")
	}
	if len(cursors) != 2 {
		t.Fatalf("游标表应恰两项: %+v", cursors)
	}
}

// TestCompanionContentApplyToScanFile 采集产物注入求值输入面：命中条目得
// 内容片段与截断标记，缺席条目保持零值。
func TestCompanionContentApplyToScanFile(t *testing.T) {
	files := []ScanFile{{RelPath: "a.txt"}, {RelPath: "b.jpg"}}
	applyCompanionContent(files, map[string]CompanionContent{
		"a.txt": {Text: "片段", Truncated: true},
	})
	if files[0].CompanionText != "片段" || !files[0].CompanionTruncated {
		t.Fatalf("命中条目应注入内容面字段: %+v", files[0])
	}
	if files[1].CompanionText != "" || files[1].CompanionTruncated {
		t.Fatalf("缺席条目应保持零值: %+v", files[1])
	}
}

// TestCompanionContentHitSetSupplyEmpty 命中集供给点：绑定结构未落地时恒为
// 空集——零采集零效果。
func TestCompanionContentHitSetSupplyEmpty(t *testing.T) {
	rules := []RuleEntry{{ID: "r1"}}
	conclusions := []FileConclusion{{RelPath: "a.txt", FileName: "a.txt"}}
	if hits := companionContentHitSet(rules, conclusions); len(hits) != 0 {
		t.Fatalf("绑定结构未落地时供给应为空集: %+v", hits)
	}
	if hits := companionContentHitSet(nil, nil); len(hits) != 0 {
		t.Fatalf("空输入供给应为空集: %+v", hits)
	}
}

// TestCompanionContentCursorMismatchDirs 游标失配判定：跳过目录里被采集
// 文件的尺寸/修改时间失配入集；游标匹配、无游标、非跳过目录均不入集。
func TestCompanionContentCursorMismatchDirs(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "d1", "keep.txt"), "不变")
	writeTestFile(t, filepath.Join(root, "d2", "mtime.txt"), "内容")
	writeTestFile(t, filepath.Join(root, "d2", "size.txt"), "内容")
	writeTestFile(t, filepath.Join(root, "d3", "plain.txt"), "无游标")
	writeTestFile(t, filepath.Join(root, "d4", "rewritten.txt"), "非跳过")
	mtimeCur := mustStatCursor(t, filepath.Join(root, "d2", "mtime.txt"))
	mtimeCur.ModTime++
	sizeCur := mustStatCursor(t, filepath.Join(root, "d2", "size.txt"))
	sizeCur.Size++
	prev := &RecognitionSnapshot{Files: []SnapFile{
		{RelPath: "d1/keep.txt", DirRelKey: "d1", ContentSize: mustStatCursor(t, filepath.Join(root, "d1", "keep.txt")).Size, ContentModTime: mustStatCursor(t, filepath.Join(root, "d1", "keep.txt")).ModTime},
		{RelPath: "d2/mtime.txt", DirRelKey: "d2", ContentSize: mtimeCur.Size, ContentModTime: mtimeCur.ModTime},
		{RelPath: "d2/size.txt", DirRelKey: "d2", ContentSize: sizeCur.Size, ContentModTime: sizeCur.ModTime},
		{RelPath: "d3/plain.txt", DirRelKey: "d3"},
		{RelPath: "d4/rewritten.txt", DirRelKey: "d4", ContentSize: 1, ContentModTime: 1},
	}}
	got := contentCursorMismatchDirs(root, prev, map[string]bool{"d1": true, "d2": true, "d3": true})
	if !got["d2"] {
		t.Fatalf("尺寸/修改时间失配目录应入集: %+v", got)
	}
	if got["d1"] || got["d3"] || got["d4"] {
		t.Fatalf("游标匹配/无游标/非跳过目录不应入集: %+v", got)
	}
	if len(got) != 1 {
		t.Fatalf("失配集应恰一项: %+v", got)
	}
	if dirs := contentCursorMismatchDirs(root, nil, map[string]bool{"d1": true}); len(dirs) != 0 {
		t.Fatalf("无上轮快照应空集: %+v", dirs)
	}
}

// TestCompanionContentSkipCorrectionEvalFace 跳过集修正：forced 命中的跳过
// 目录整目录拉回求值面，并从复用集摘除。
func TestCompanionContentSkipCorrectionEvalFace(t *testing.T) {
	all := []FileEntry{
		{RelPath: "dirA/a1.txt"}, {RelPath: "dirA/a2.txt"},
		{RelPath: "dirB/b1.txt"},
		{RelPath: "dirC/c1.txt"},
	}
	prevSkipped := map[string]bool{"dirA": true, "dirB": true}
	evalFiles := skipCorrectionEvalFace([]FileEntry{{RelPath: "dirC/c1.txt"}}, all, prevSkipped, map[string]bool{"dirA": true})
	seen := make(map[string]bool, len(evalFiles))
	for _, f := range evalFiles {
		seen[f.RelPath] = true
	}
	for _, rel := range []string{"dirC/c1.txt", "dirA/a1.txt", "dirA/a2.txt"} {
		if !seen[rel] {
			t.Fatalf("求值面应含 %s: %+v", rel, evalFiles)
		}
	}
	if seen["dirB/b1.txt"] {
		t.Fatal("未命中强制重算的跳过目录不应拉回求值面")
	}
	if prevSkipped["dirA"] || !prevSkipped["dirB"] {
		t.Fatalf("复用集应只摘除强制重算目录: %+v", prevSkipped)
	}
}

// TestCompanionContentCursorMismatchFullRound 游标失配的整轮行为：目录被
// mtime 跳过时，被采集文件原地改内容（父目录 mtime 不变）→ 失配目录整目录
// 拉回求值面（记录重铸、游标清零）；游标匹配的文件维持整记录复用（游标
// 保留）。
func TestCompanionContentCursorMismatchFullRound(t *testing.T) {
	root := t.TempDir()
	m, _ := newRecognitionFixture(t, root, nil)
	recognitionSampleTree(t, root)
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("首轮识别失败: %v", err)
	}
	// 模拟上轮采集过两个文件：游标注入当前 stat 值（改动前）。快照属重载荷，
	// 变异走全保存入口。
	inject := map[string]companionContentCursor{
		"冲突/AB.jpg":  mustStatCursor(t, filepath.Join(root, "冲突", "AB.jpg")),
		"线索/任意名.jpg": mustStatCursor(t, filepath.Join(root, "线索", "任意名.jpg")),
	}
	if _, err := m.mutateSessionFull(root, func(sess *WorkbenchSession) error {
		for i := range sess.Snapshot.Files {
			if cur, ok := inject[sess.Snapshot.Files[i].RelPath]; ok {
				sess.Snapshot.Files[i].ContentSize, sess.Snapshot.Files[i].ContentModTime = cur.Size, cur.ModTime
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("注入游标失败: %v", err)
	}
	// AB.jpg 原地改内容：文件 mtime 前进、父目录 mtime 不变（目录仍被跳过）。
	time.Sleep(2 * time.Millisecond)
	writeTestFile(t, filepath.Join(root, "冲突", "AB.jpg"), "改写后的内容")
	if err := m.runFullRecognition(root); err != nil {
		t.Fatalf("增量识别失败: %v", err)
	}
	after, err := m.SessionOf(root)
	if err != nil {
		t.Fatalf("读取会话失败: %v", err)
	}
	byPath := make(map[string]SnapFile, len(after.Snapshot.Files))
	for _, f := range after.Snapshot.Files {
		byPath[f.RelPath] = f
	}
	pulled := byPath["冲突/AB.jpg"]
	if pulled.ContentSize != 0 || pulled.ContentModTime != 0 {
		t.Fatalf("失配拉回的文件应重铸记录（本轮未采集则游标清零）: %+v", pulled)
	}
	reused := byPath["线索/任意名.jpg"]
	cur := inject["线索/任意名.jpg"]
	if reused.ContentSize != cur.Size || reused.ContentModTime != cur.ModTime {
		t.Fatalf("游标匹配的文件应整记录复用（游标保留）: %+v 期望 %+v", reused, cur)
	}
}

// TestCompanionContentSnapshotCursorMint 快照铸造：本轮采集游标铸入重求值
// 文件的记录，未采集文件零值。
func TestCompanionContentSnapshotCursorMint(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "说明.txt"), "说明内容")
	writeTestFile(t, filepath.Join(root, "图.jpg"), "img")
	enumRes, err := NewScanner(root).Enumerate(nil)
	if err != nil {
		t.Fatalf("枚举失败: %v", err)
	}
	cur := mustStatCursor(t, filepath.Join(root, "说明.txt"))
	snap := aggregatePanorama(aggregateInput{
		root: root, enumRes: enumRes,
		conclusions: []FileConclusion{
			{RelPath: "说明.txt", FileName: "说明.txt"},
			{RelPath: "图.jpg", FileName: "图.jpg"},
		},
		prevSkipped:    map[string]bool{},
		contentCursors: map[string]companionContentCursor{"说明.txt": cur},
	})
	byPath := make(map[string]SnapFile, len(snap.Files))
	for _, f := range snap.Files {
		byPath[f.RelPath] = f
	}
	got := byPath["说明.txt"]
	if got.ContentSize != cur.Size || got.ContentModTime != cur.ModTime {
		t.Fatalf("被采集文件的游标应铸入记录: %+v 期望 %+v", got, cur)
	}
	plain := byPath["图.jpg"]
	if plain.ContentSize != 0 || plain.ContentModTime != 0 {
		t.Fatalf("未采集文件游标应零值: %+v", plain)
	}
}
