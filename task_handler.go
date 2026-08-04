package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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
	ctx        sdkdto.PluginContext
	classifier *PathClassifier
	readers    sync.Map // taskID → *os.File
}

// Create 扫描本地路径，流式产出任务
func (h *LocalImportTaskHandler) Create(url string) (*sdkdto.TaskCreateResult, error) {
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
		return sdkdto.BatchResult(nil), nil
	}

	ch := make(chan *sdkdto.TaskCreateResponse, 16)

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

			children := make([]*sdkdto.TaskCreateChildResponse, 0, len(files))
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

				children = append(children, &sdkdto.TaskCreateChildResponse{
					TaskName:     filepath.Base(f.FullPath),
					SiteWorkId:   f.Hash,
					Url:          "local://" + f.FullPath,
					PluginData:   string(fpJSON),
					SiteName:     siteName,
					ResourceType: classifyResourceType(f.FullPath),
				})
			}

			dp := &DirPluginData{
				DirRelPath: dirRelPath,
				Metadata:   metadata,
			}
			dpJSON, _ := json.Marshal(dp)

			if len(children) == 1 {
				ch <- &sdkdto.TaskCreateResponse{
					PluginTaskId: children[0].SiteWorkId,
					TaskName:     children[0].TaskName,
					SiteWorkId:   children[0].SiteWorkId,
					Url:          children[0].Url,
					PluginData:   children[0].PluginData,
					SiteName:     siteName,
					ResourceType: children[0].ResourceType,
				}
			} else {
				ch <- &sdkdto.TaskCreateResponse{
					PluginTaskId: fmt.Sprintf("local-dir-%s", dirRelPath),
					TaskName:     taskName,
					SiteWorkId:   fmt.Sprintf("local-dir-%s", dirRelPath),
					Url:          "local://" + filepath.Join(path, dirRelPath),
					PluginData:   string(dpJSON),
					SiteName:     siteName,
					ResourceType: "", // 有 children 时由各 child 声明(parent 不声明)
					Children:     children,
				}
			}
		}
	}()

	return sdkdto.StreamResult(ch), nil
}

// CreateWorkInfo 从 PluginData 反序列化路径元数据，构建 WorkResponse
func (h *LocalImportTaskHandler) CreateWorkInfo(task *sdkdto.TaskDTO) (*sdkdto.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}

	workName := filepath.Base(fp.FullPath)
	// 去除扩展名，扩展名由 Resource.Format 单独提供
	if ext := filepath.Ext(workName); ext != "" {
		workName = workName[:len(workName)-len(ext)]
	}
	resp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkId:   &fp.Hash,
			SiteWorkName: &workName,
		},
	}

	for _, m := range fp.Metadata {
		switch m.Type {
		case "localAuthor":
			id, _ := strconv.ParseInt(m.ID, 10, 64)
			if id > 0 {
				resp.LocalAuthors = append(resp.LocalAuthors, &sdkdto.LocalAuthorDTO{Id: id})
			}
		case "siteAuthor":
			siteAuthorID := "siteAuthor:" + m.Name
			resp.SiteAuthors = append(resp.SiteAuthors, &sdkdto.TaskSiteAuthorDTO{
				SiteAuthorId: siteAuthorID,
				AuthorName:   m.Name,
			})
		case "localTag":
			id, _ := strconv.ParseInt(m.ID, 10, 64)
			if id > 0 {
				resp.LocalTags = append(resp.LocalTags, &sdkdto.LocalTagDTO{Id: id})
			}
		case "siteTag":
			siteTagID := "siteTag:" + m.Name
			resp.SiteTags = append(resp.SiteTags, &sdkdto.TaskSiteTagDTO{
				SiteTagId: siteTagID,
				TagName:   m.Name,
			})
		case "workSet":
			resp.WorkSets = append(resp.WorkSets, &sdkdto.TaskWorkSetDTO{
				SiteWorkSetId: "workSet:" + m.Name,
				WorkSetName:   m.Name,
			})
		}
	}

	return resp, nil
}

// Start 打开文件,按 storeRoles 选择性返回 StoreSpec 流集合(主资源 downloaded + 缩略图 derived)与作品信息
func (h *LocalImportTaskHandler) Start(ctx context.Context, task *sdkdto.TaskDTO, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}

	ext := filepath.Ext(fp.FullPath)
	workName := stripExt(filepath.Base(fp.FullPath))
	taskID := fmt.Sprintf("%d", task.Id)

	// 主资源 role 按文件类型派生:video→videoMain(可播放主体,downloaded);image/document 对应;unknown→image 兜底
	mainRole := mainStoreRole(classifyResourceType(fp.FullPath))

	var specs []*sdkdto.StoreSpec

	// 主资源(downloaded):本地文件流,可断点续传
	if wantsRole(storeRoles, mainRole) {
		f, err := os.Open(fp.FullPath)
		if err != nil {
			return nil, nil, fmt.Errorf("打开文件失败: %w", err)
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("获取文件信息失败: %w", err)
		}
		h.readers.Store(taskID, f)
		specs = append(specs, &sdkdto.StoreSpec{
			Role:        mainRole,
			Generation:  sdkdto.GenerationDownloaded,
			ReadCloser:  f,
			Format:      ext,
			Size:        fi.Size(),
			Continuable: boolPtr(true),
		})
	}

	// 缩略图(derived):按扩展名生成;无生成器或不适用则不产出该轨
	if wantsRole(storeRoles, sdkdto.StoreRoleThumbnail) {
		if thumbData, thumbFormat, _ := generateThumbnail(fp.FullPath); len(thumbData) > 0 {
			if thumbFormat == "" {
				thumbFormat = "jpg"
			}
			specs = append(specs, &sdkdto.StoreSpec{
				Role:        sdkdto.StoreRoleThumbnail,
				Generation:  sdkdto.GenerationDerived,
				ReadCloser:  io.NopCloser(bytes.NewReader(thumbData)),
				Format:      thumbFormat,
				Size:        int64(len(thumbData)),
				Continuable: boolPtr(false),
			})
		}
	}

	if len(specs) == 0 {
		return nil, nil, fmt.Errorf("所选资源角色 %v 均无可产出 store", storeRoles)
	}

	resp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkName: &workName,
		},
	}

	return specs, resp, nil
}

// wantsRole storeRoles 为空(全量)或包含 role 时返回 true
func wantsRole(storeRoles []string, role string) bool {
	if len(storeRoles) == 0 {
		return true
	}
	for _, r := range storeRoles {
		if r == role {
			return true
		}
	}
	return false
}

// Retry 委托到 Start
func (h *LocalImportTaskHandler) Retry(task *sdkdto.TaskDTO) (*sdkdto.WorkResponse, error) {
	return nil, fmt.Errorf("retry 不支持，请使用 start")
}

// Pause 关闭文件句柄
func (h *LocalImportTaskHandler) Pause(param *sdkdto.TaskResParam) error {
	return h.closeReader(param)
}

// Stop 关闭文件句柄
func (h *LocalImportTaskHandler) Stop(param *sdkdto.TaskResParam) error {
	return h.closeReader(param)
}

// Resume 恢复下载:按 TaskResumeParam.StreamOffsets 续传未完成的主资源轨
// 缩略图(derived)为一次性产物,暂停的任务意味着其已完成,故 Resume 不再产出
func (h *LocalImportTaskHandler) Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	if param.Task == nil || param.Task.PluginData == nil {
		return nil, nil, fmt.Errorf("task 或 pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*param.Task.PluginData), &fp); err != nil {
		return nil, nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}

	// 主资源 role 按文件类型派生(与 Start 一致)
	mainRole := mainStoreRole(classifyResourceType(fp.FullPath))

	offset, hasMain := param.OffsetForRole(mainRole)
	// 主资源已完成或未选:无需续传
	if !hasMain {
		return nil, nil, nil
	}

	f, err := os.Open(fp.FullPath)
	if err != nil {
		return nil, nil, fmt.Errorf("打开文件失败: %w", err)
	}

	// 从已下载位置继续
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			f.Close()
			return nil, nil, fmt.Errorf("seek 到偏移量 %d 失败: %w", offset, err)
		}
	}

	taskID := fmt.Sprintf("%d", param.Task.Id)
	h.readers.Store(taskID, f)

	ext := filepath.Ext(fp.FullPath)
	workName := stripExt(filepath.Base(fp.FullPath))

	spec := &sdkdto.StoreSpec{
		Role:        mainRole,
		Generation:  sdkdto.GenerationDownloaded,
		ReadCloser:  f,
		Format:      ext,
		Size:        fp.Size,
		Continuable: boolPtr(true),
	}

	resp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkName: &workName,
		},
	}

	return []*sdkdto.StoreSpec{spec}, resp, nil
}

func (h *LocalImportTaskHandler) closeReader(param *sdkdto.TaskResParam) error {
	if param.Task == nil {
		return nil
	}
	taskID := fmt.Sprintf("%d", param.Task.Id)
	if v, ok := h.readers.LoadAndDelete(taskID); ok {
		return v.(*os.File).Close()
	}
	return nil
}

// boolPtr 返回 bool 值的指针
func boolPtr(b bool) *bool { return &b }

// stripExt 去除文件名扩展名(扩展名由 StoreSpec.Format 单独提供)
func stripExt(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		return strings.TrimSuffix(name, ext)
	}
	return name
}
