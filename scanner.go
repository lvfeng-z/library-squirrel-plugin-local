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

// GroupFilesByParentDir 按直接父目录分组文件
// 同一目录下的文件属于同一个 parent task
func GroupFilesByParentDir(files []FileEntry) map[string][]FileEntry {
	groups := make(map[string][]FileEntry)
	for _, f := range files {
		dir := filepath.Dir(f.RelPath)
		if dir == "." {
			dir = ""
		}
		groups[dir] = append(groups[dir], f)
	}
	return groups
}
