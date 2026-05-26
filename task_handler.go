package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	pluginsdk "github.com/lvfeng-z/library-squirrel-plugin-sdk"
)

const siteName = "local"

// FilePluginData 文件级 PluginData
type FilePluginData struct {
	FullPath string        `json:"fullPath"`
	RelPath  string        `json:"relPath"`
	Hash     string        `json:"hash"`
	Size     int64         `json:"size"`
	Metadata []PathMeaning `json:"metadata,omitempty"`
}

// DirPluginData 目录级 PluginData（用于 parent task）
type DirPluginData struct {
	DirRelPath string        `json:"dirRelPath"`
	Metadata   []PathMeaning `json:"metadata"`
}

// LocalImportTaskHandler 本地文件导入任务处理器
type LocalImportTaskHandler struct {
	ctx        pluginsdk.PluginContext
	classifier *PathClassifier
	readers    sync.Map // taskID → *os.File
}

// Create 扫描本地路径，流式产出任务
func (h *LocalImportTaskHandler) Create(url string) (*pluginsdk.TaskCreateResult, error) {
	path := url
	if len(path) >= 8 && path[:8] == "local://" {
		path = path[8:]
	}

	scanner := NewScanner(path)
	scanResult, err := scanner.Scan()
	if err != nil {
		return nil, fmt.Errorf("扫描路径失败: %w", err)
	}

	if len(scanResult.Files) == 0 {
		return pluginsdk.BatchResult(nil), nil
	}

	ch := make(chan *pluginsdk.TaskCreateResponse, 16)

	go func() {
		defer close(ch)

		pathInfo, _ := os.Stat(path)
		isDir := pathInfo != nil && pathInfo.IsDir()
		var rootMeanings []PathMeaning
		rootDirName := filepath.Base(path)
		if isDir {
			var err error
			rootMeanings, err = h.classifier.ClassifyDir(0, rootDirName)
			if err != nil {
				return
			}
		}

		groups := GroupFilesByParentDir(scanResult.Files)

		for dirRelPath, files := range groups {
			var metadata []PathMeaning

			if isDir {
				metadata = append(metadata, rootMeanings...)
			}

			if len(files) > 0 {
				levelsSlice := ExtractDirLevels(files[0].RelPath)
				levelOffset := 0
				if isDir {
					levelOffset = 1
				}
				for i, dirName := range levelsSlice {
					meanings, err := h.classifier.ClassifyDir(i+levelOffset, dirName)
					if err != nil {
						return
					}
					metadata = append(metadata, meanings...)
				}
			}

			taskName := fmt.Sprintf("导入【%s】", path)

			children := make([]*pluginsdk.TaskCreateChildResponse, 0, len(files))
			for _, f := range files {
				fi, err := os.Stat(f.FullPath)
				if err != nil {
					continue
				}

				fp := &FilePluginData{
					FullPath: f.FullPath,
					RelPath:  f.RelPath,
					Hash:     f.Hash,
					Size:     fi.Size(),
					Metadata: metadata,
				}
				fpJSON, _ := json.Marshal(fp)

				children = append(children, &pluginsdk.TaskCreateChildResponse{
					TaskName:   filepath.Base(f.FullPath),
					SiteWorkID: f.Hash,
					URL:        "local://" + f.FullPath,
					PluginData: string(fpJSON),
					SiteName:   siteName,
				})
			}

			dp := &DirPluginData{
				DirRelPath: dirRelPath,
				Metadata:   metadata,
			}
			dpJSON, _ := json.Marshal(dp)

			if len(children) == 1 {
				ch <- &pluginsdk.TaskCreateResponse{
					PluginTaskID: children[0].SiteWorkID,
					TaskName:     children[0].TaskName,
					SiteWorkID:   children[0].SiteWorkID,
					URL:          children[0].URL,
					PluginData:   children[0].PluginData,
					SiteName:     siteName,
				}
			} else {
				ch <- &pluginsdk.TaskCreateResponse{
					PluginTaskID: fmt.Sprintf("local-dir-%s", dirRelPath),
					TaskName:     taskName,
					SiteWorkID:   fmt.Sprintf("local-dir-%s", dirRelPath),
					URL:          "local://" + filepath.Join(path, dirRelPath),
					PluginData:   string(dpJSON),
					SiteName:     siteName,
					Children:     children,
				}
			}
		}
	}()

	return pluginsdk.StreamResult(ch), nil
}

// CreateWorkInfo 从 PluginData 反序列化路径元数据，构建 WorkResponse
func (h *LocalImportTaskHandler) CreateWorkInfo(task *pluginsdk.Task) (*pluginsdk.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}

	workName := filepath.Base(fp.FullPath)
	resp := &pluginsdk.WorkResponse{
		Work: &pluginsdk.Work{
			SiteWorkID:   &fp.Hash,
			SiteWorkName: &workName,
		},
	}

	for _, m := range fp.Metadata {
		switch m.Type {
		case "localAuthor", "siteAuthor":
			siteAuthorID := m.Type + ":" + m.Name
			resp.SiteAuthors = append(resp.SiteAuthors, &pluginsdk.TaskSiteAuthorDTO{
				SiteAuthorID: siteAuthorID,
				AuthorName:   m.Name,
			})
		case "localTag", "siteTag":
			siteTagID := m.Type + ":" + m.Name
			resp.SiteTags = append(resp.SiteTags, &pluginsdk.TaskSiteTagDTO{
				SiteTagID: siteTagID,
				TagName:   m.Name,
			})
		case "workSet":
			resp.WorkSets = append(resp.WorkSets, &pluginsdk.TaskWorkSetDTO{
				SiteWorkSetID: "workSet:" + m.Name,
				WorkSetName:   m.Name,
			})
		}
	}

	return resp, nil
}

// Start 打开文件并返回 ReadCloser + WorkResponse
func (h *LocalImportTaskHandler) Start(task *pluginsdk.Task) (io.ReadCloser, *pluginsdk.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}

	f, err := os.Open(fp.FullPath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开文件失败: %w", err)
	}

	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	taskID := fmt.Sprintf("%d", task.ID)
	h.readers.Store(taskID, f)

	workName := filepath.Base(fp.FullPath)
	format := filepath.Ext(fp.FullPath)
	if len(format) > 0 {
		format = format[1:]
	}

	resp := &pluginsdk.WorkResponse{
		Work: &pluginsdk.Work{
			SiteWorkName: &workName,
		},
		Resource: &pluginsdk.TaskResourceDTO{
			URL:       "local://" + fp.FullPath,
			LocalPath: fp.RelPath,
			Size:      fi.Size(),
			Format:    format,
		},
	}

	return f, resp, nil
}

// Retry 委托到 Start
func (h *LocalImportTaskHandler) Retry(task *pluginsdk.Task) (*pluginsdk.WorkResponse, error) {
	return nil, fmt.Errorf("retry 不支持，请使用 start")
}

// Pause 关闭文件句柄
func (h *LocalImportTaskHandler) Pause(param *pluginsdk.TaskResParam) error {
	return h.closeReader(param)
}

// Stop 关闭文件句柄
func (h *LocalImportTaskHandler) Stop(param *pluginsdk.TaskResParam) error {
	return h.closeReader(param)
}

// Resume 重新打开文件（从偏移量继续）
func (h *LocalImportTaskHandler) Resume(param *pluginsdk.TaskResParam) (*pluginsdk.WorkResponse, error) {
	return nil, fmt.Errorf("resume 暂不支持")
}

func (h *LocalImportTaskHandler) closeReader(param *pluginsdk.TaskResParam) error {
	if param.Task == nil {
		return nil
	}
	taskID := fmt.Sprintf("%d", param.Task.ID)
	if v, ok := h.readers.LoadAndDelete(taskID); ok {
		return v.(*os.File).Close()
	}
	return nil
}
