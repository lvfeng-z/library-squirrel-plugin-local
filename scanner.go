package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
