package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// FileEntry 扫描到的文件条目
type FileEntry struct {
	FullPath string // 文件完整路径
	RelPath  string // 相对于输入路径的相对路径
	Hash     string // SHA-256 哈希
}

// ScanResult 扫描结果
type ScanResult struct {
	Files     []FileEntry // 所有文件
	DirLevels []string    // 从输入路径开始的目录层级名
}

// Scanner 目录扫描器
type Scanner struct {
	rootPath string
}

// NewScanner 创建扫描器
func NewScanner(rootPath string) *Scanner {
	return &Scanner{rootPath: rootPath}
}

// Scan 扫描路径，收集所有文件及其相对路径
func (s *Scanner) Scan() (*ScanResult, error) {
	info, err := os.Stat(s.rootPath)
	if err != nil {
		return nil, fmt.Errorf("路径不可访问: %w", err)
	}

	result := &ScanResult{}

	if !info.IsDir() {
		// 单文件
		hash, err := ComputeFileHash(s.rootPath)
		if err != nil {
			return nil, fmt.Errorf("计算文件哈希失败: %w", err)
		}
		result.Files = append(result.Files, FileEntry{
			FullPath: s.rootPath,
			RelPath:  filepath.Base(s.rootPath),
			Hash:     hash,
		})
		return result, nil
	}

	// 目录扫描
	err = filepath.Walk(s.rootPath, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			// 跳过不可访问的文件/目录
			return nil
		}
		if fi.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(s.rootPath, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		hash, err := ComputeFileHash(path)
		if err != nil {
			return nil
		}

		result.Files = append(result.Files, FileEntry{
			FullPath: path,
			RelPath:  rel,
			Hash:     hash,
		})
		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// ExtractDirLevels 从相对路径中提取目录层级名（不含文件名）
// 例如 "author/tag/work/file.jpg" → ["author", "tag", "work"]
func ExtractDirLevels(relPath string) []string {
	dir := filepath.Dir(relPath)
	if dir == "." {
		return nil
	}
	return strings.Split(filepath.ToSlash(dir), "/")
}

// —— 识别干跑的轻量枚举（零内容哈希 + 目录 mtime 增量）——

// EnumerateResult 轻量枚举结果：识别干跑是纯求值（求值输入=原始路径+档案，
// 文件内容不参与），不计算内容哈希；FileEntry.Hash 恒为空。
type EnumerateResult struct {
	// Files 为需要（重新）求值的常规文件条目：mtime 未变目录的直接文件不在此列
	// （其结论由调用方从上次识别快照复用）。
	Files []FileEntry
	// AllFiles 为全部常规文件条目（含未变目录复用者），供死路径赋值判定与
	// 全量统计使用。
	AllFiles []FileEntry
	// DirMtimes 为本次实际下钻的全部目录（含根与被跳过求值者）的 mtime 游标：
	// 键为相对根的 '/' 目录路径（根为空串），值为 mtime 的 UnixNano。
	DirMtimes map[string]int64
	// SkippedDirKeys 为 mtime 未变而跳过求值的目录键（结论复用上次快照）。
	SkippedDirKeys []string
	// Dirs 为目录树节点（含根），层深编号与目录问答一致：根为 0、根下首层为 1。
	Dirs []EnumerateDir
}

// EnumerateDir 单个目录的枚举产物。
type EnumerateDir struct {
	// RelKey 为相对根的 '/' 目录路径（根为空串）。
	RelKey string
	// Name 为目录名（根节点为库根的基名）。
	Name string
	// Depth 为层深（根为 0）。
	Depth int
	// FileCount 为该目录直接子文件数（含未变目录复用者）。
	FileCount int
}

// dirRelKeyOf 相对文件路径 → 所属目录键：取 '/' 路径的目录段（根直下为空串）。
func dirRelKeyOf(relPath string) string {
	idx := strings.LastIndexByte(relPath, '/')
	if idx < 0 {
		return ""
	}
	return relPath[:idx]
}

// Enumerate 轻量枚举库根：不读文件内容、不计算哈希，逐目录收集 mtime 游标并
// 按 prevDirMtimes 做目录级增量过滤——目录 mtime 反映其直接子项（文件与子目录）
// 的增删改，mtime 未变即该目录的直接文件集合与名字未变，求值结论（仅由原始
// 路径与规则决定）不变，跳过其文件求值；子目录仍继续下钻（子目录内部变化只
// 改子目录自身 mtime，跳过父目录整棵子树会漏检）。prevDirMtimes 为 nil 时全量。
func (s *Scanner) Enumerate(prevDirMtimes map[string]int64) (*EnumerateResult, error) {
	info, err := os.Stat(s.rootPath)
	if err != nil {
		return nil, fmt.Errorf("路径不可访问: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("识别干跑的库根须为目录: %s", s.rootPath)
	}
	result := &EnumerateResult{DirMtimes: make(map[string]int64)}
	var walk func(dirPath, relKey string, depth int) error
	walk = func(dirPath, relKey string, depth int) error {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			// 不可读目录跳过（与 Scan 的容错语义一致），不影响其余目录
			return nil
		}
		var mtime int64
		if fi, err := os.Stat(dirPath); err == nil {
			mtime = fi.ModTime().UnixNano()
			result.DirMtimes[relKey] = mtime
		}
		result.Dirs = append(result.Dirs, EnumerateDir{
			RelKey: relKey, Name: filepath.Base(dirPath), Depth: depth,
		})
		// 目录级增量判定：游标在表且 mtime 相等 → 直接文件集合未变，跳过求值
		unchanged := false
		if prev, ok := prevDirMtimes[relKey]; ok && prev == mtime {
			unchanged = true
			result.SkippedDirKeys = append(result.SkippedDirKeys, relKey)
		}
		var subDirs []os.DirEntry
		for _, entry := range entries {
			if entry.IsDir() {
				subDirs = append(subDirs, entry)
				continue
			}
			rel := entry.Name()
			if relKey != "" {
				rel = relKey + "/" + entry.Name()
			}
			fileEntry := FileEntry{
				FullPath: filepath.Join(dirPath, entry.Name()),
				RelPath:  rel,
			}
			result.AllFiles = append(result.AllFiles, fileEntry)
			result.Dirs[len(result.Dirs)-1].FileCount++
			if unchanged {
				continue
			}
			result.Files = append(result.Files, fileEntry)
		}
		// 字典序下钻（ReadDir 已按名字典序返回，与 Scan 的 Walk 顺序同构）
		for _, sub := range subDirs {
			subRel := sub.Name()
			if relKey != "" {
				subRel = relKey + "/" + sub.Name()
			}
			if err := walk(filepath.Join(dirPath, sub.Name()), subRel, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(s.rootPath, "", 0); err != nil {
		return nil, err
	}
	return result, nil
}

// —— 伴随说明文件的内容片段采集（识别轮按规则命中集的一次性读盘）——

// companionContentMaxBytes 单文件内容片段的采集上限：命中集文件读盘量的
// 安全界，超出截断。
const companionContentMaxBytes = 64 << 10

// CompanionContent 单个伴随说明文件的内容片段采集产物。
type CompanionContent struct {
	// Text 为解码后的内容片段：编码回退链的终态文本，不可解码字节以替换
	// 字符在场（替换字符可见即回退事实的呈现）。
	Text string
	// Truncated 标记内容超出采集上限被截断（内容声明来源标注截断事实的
	// 数据位）。
	Truncated bool
}

// companionContentCursor 文件级内容游标：尺寸+修改时间（UnixNano）。内容
// 原地修改不改父目录 mtime，目录级增量跳过检不到内容变化，被采集文件以此
// 逐文件比对新鲜度（失配即拉回求值面，接线见识别轮）。
type companionContentCursor struct {
	Size    int64
	ModTime int64
}

// companionContentCursorOf 从文件信息取内容游标。
func companionContentCursorOf(fi os.FileInfo) companionContentCursor {
	return companionContentCursor{Size: fi.Size(), ModTime: fi.ModTime().UnixNano()}
}

// collectCompanionContent 采集单文件的内容片段：上限读入（多读一字节判定
// 截断）→ 二进制嗅探（首段含 NUL 字节即拒读：零内容零声明，按来源缺失
// 呈现）→ 编码回退链解码。ok=false 为拒读（二进制嗅探命中或读盘失败）；
// stat 成功时游标仍然返回——拒读不是永久态，文件再编辑后游标失配照常拉回
// 求值面。
func collectCompanionContent(fullPath string) (sample CompanionContent, cursor companionContentCursor, ok bool) {
	f, err := os.Open(fullPath)
	if err != nil {
		return CompanionContent{}, companionContentCursor{}, false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return CompanionContent{}, companionContentCursor{}, false
	}
	data, err := io.ReadAll(io.LimitReader(f, companionContentMaxBytes+1))
	if err != nil {
		return CompanionContent{}, companionContentCursorOf(fi), false
	}
	if len(data) > companionContentMaxBytes {
		data = data[:companionContentMaxBytes]
		sample.Truncated = true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return CompanionContent{}, companionContentCursorOf(fi), false
	}
	sample.Text = decodeCompanionText(data)
	return sample, companionContentCursorOf(fi), true
}

// collectCompanionContentSet 按命中集一次采齐（不逐目录流式）：hitRels 为
// 内容绑定行命中的文件相对路径，allFiles 为枚举全量清单（相对路径 → 完整
// 路径解析）。返回内容片段表（拒读者缺席）与游标表（stat 成功的命中文件，
// 含拒读者）。清单外路径静默跳过——文件增删由下轮枚举对账。
func collectCompanionContentSet(hitRels []string, allFiles []FileEntry) (samples map[string]CompanionContent, cursors map[string]companionContentCursor) {
	if len(hitRels) == 0 {
		return nil, nil
	}
	fullByRel := make(map[string]string, len(allFiles))
	for _, f := range allFiles {
		fullByRel[f.RelPath] = f.FullPath
	}
	samples = make(map[string]CompanionContent, len(hitRels))
	cursors = make(map[string]companionContentCursor, len(hitRels))
	seen := make(map[string]bool, len(hitRels))
	for _, rel := range hitRels {
		if seen[rel] {
			continue
		}
		seen[rel] = true
		full, listed := fullByRel[rel]
		if !listed {
			continue
		}
		sample, cursor, ok := collectCompanionContent(full)
		if cursor.ModTime != 0 || cursor.Size != 0 {
			cursors[rel] = cursor
		}
		if ok {
			samples[rel] = sample
		}
	}
	return samples, cursors
}

// applyCompanionContent 把采集产物注入求值输入面：命中集文件的内容片段与
// 截断标记写入对应条目（缺席者保持零值——纯求值只消费注入的数据字段，
// 读盘不进求值函数）。
func applyCompanionContent(files []ScanFile, samples map[string]CompanionContent) {
	if len(samples) == 0 {
		return
	}
	for i := range files {
		if s, ok := samples[files[i].RelPath]; ok {
			files[i].CompanionText = s.Text
			files[i].CompanionTruncated = s.Truncated
		}
	}
}

// decodeCompanionText 内容片段的编码回退链：BOM 判定 → UTF-8 合法性校验 →
// GBK 解码 → 不可解码字节按替换字符读入（GBK 解码器对失配序列自置替换
// 字符，链尾天然以替换字符收尾）。
func decodeCompanionText(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		return decodeUTF8WithReplacement(data[3:])
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		return decodeUTF16WithReplacement(data[2:], false)
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		return decodeUTF16WithReplacement(data[2:], true)
	}
	if utf8.Valid(data) {
		return string(data)
	}
	out, err := simplifiedchinese.GBK.NewDecoder().Bytes(data)
	if err != nil {
		return strings.ToValidUTF8(string(data), string(utf8.RuneError))
	}
	return string(out)
}

// decodeUTF8WithReplacement UTF-8 解码（BOM 已剥）：失配字节按替换字符读入。
func decodeUTF8WithReplacement(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	return strings.ToValidUTF8(string(data), string(utf8.RuneError))
}

// decodeUTF16WithReplacement UTF-16 解码（BOM 已剥）：奇数尾字节丢弃，落单
// 代位由标准库按替换字符置换。
func decodeUTF16WithReplacement(data []byte, bigEndian bool) string {
	if len(data)%2 != 0 {
		data = data[:len(data)-1]
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		lo, hi := data[2*i], data[2*i+1]
		if bigEndian {
			lo, hi = hi, lo
		}
		units[i] = uint16(lo) | uint16(hi)<<8
	}
	return string(utf16.Decode(units))
}
