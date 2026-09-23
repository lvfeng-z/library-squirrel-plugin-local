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

// currentPluginDataVersion 当前插件支持的 task PluginData 格式版本
// v2：FilePluginData 增加可选归属字段（siteKey/siteWorkId，扫描期归属裁决结果）；
// v0/v1 旧数据无归属字段，读侧按零值走 local 回退路径（作品身份=内容哈希）
const currentPluginDataVersion = 2

// checkPluginDataVersion 校验 task PluginData 格式版本：
// 0(旧数据，引入版本约定前写入)与当前支持版本走现有逻辑；高于当前=由更新版本插件写入，拒绝执行防静默数据损坏。
// 主程序不解析 PluginData 内容，版本兼容性由插件自负（见 doc/plugin-dev-guide.md「plugin_data 格式版本约定」）。
func checkPluginDataVersion(v int) error {
	if v > currentPluginDataVersion {
		return fmt.Errorf("任务 PluginData 由更高版本插件创建(schemaVersion=%d，当前支持 %d)，请升级插件或重建任务", v, currentPluginDataVersion)
	}
	return nil
}

// FilePluginData 文件级 PluginData
type FilePluginData struct {
	SchemaVersion int           `json:"schemaVersion,omitempty"` // plugin_data 格式版本；0=旧数据(引入版本约定前写入)，1=引入版本约定，2=增加归属字段
	FullPath      string        `json:"fullPath"`
	RelPath       string        `json:"relPath"`
	Hash          string        `json:"hash"`
	Size          int64         `json:"size"`
	Metadata      []PathMeaning `json:"metadata,omitempty"`
	// SiteKey/SiteWorkId 真实域归属（扫描期站点分类与文件名形态解析的裁决结果）：
	// SiteKey=归属站点键、SiteWorkId=页级作品 ID。两字段空=local 回退，作品身份=Hash
	SiteKey    string `json:"siteKey,omitempty"`
	SiteWorkId string `json:"siteWorkId,omitempty"`
}

// hasRealAttribution PluginData 是否携带真实域归属（裁决出真实站点与页级作品 ID）
func (fp *FilePluginData) hasRealAttribution() bool {
	return fp.SiteKey != "" && fp.SiteWorkId != ""
}

// DirPluginData 目录级 PluginData（用于 parent task）
type DirPluginData struct {
	SchemaVersion int           `json:"schemaVersion,omitempty"` // plugin_data 格式版本；0=旧数据(引入版本约定前写入)，1=当前
	DirRelPath    string        `json:"dirRelPath"`
	Metadata      []PathMeaning `json:"metadata"`
}

// LocalImportWorkFetcher 本地文件导入作品拉取扩展
type LocalImportWorkFetcher struct {
	ctx        sdkdto.PluginContext
	classifier *PathClassifier
	readers    sync.Map // taskID → *os.File

	siteKeyMu   sync.Mutex
	siteKeyByID map[int64]string // 站点 DB 行 id → 站点键（ListSites 注册表投影一次拉取的缓存）
}

// resolveSiteKeyByID 站点 DB 行 id → 站点键。面板站点下拉选择项的 value 是站点 DB 行 id，
// 而跨库身份是站点键，经 ListSites 注册表投影解析；不可解析（空 id/查询失败/查无此行）
// 一律按无站点分类处理，作品走 local 回退。
func (h *LocalImportWorkFetcher) resolveSiteKeyByID(idStr string) string {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return ""
	}
	h.siteKeyMu.Lock()
	defer h.siteKeyMu.Unlock()
	if h.siteKeyByID == nil {
		if h.ctx == nil {
			return ""
		}
		resp, err := h.ctx.ListSites()
		if err != nil {
			h.ctx.Warnf("查询站点列表失败，站点分类含义按无站点分类处理: %v", err)
			return ""
		}
		byID := make(map[int64]string, len(resp.Items))
		for _, s := range resp.Items {
			if s != nil && s.SiteKey != "" {
				byID[s.Id] = s.SiteKey
			}
		}
		h.siteKeyByID = byID
	}
	return h.siteKeyByID[id]
}

// Create 扫描本地路径，流式产出任务
func (h *LocalImportWorkFetcher) Create(url string) (*sdkdto.TaskCreateResult, error) {
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
		// 扫描器不按扩展名过滤（不识别的类型归 unknown 照常导入），
		// 零文件只可能是目录下无常规文件、或文件均因不可访问被跳过，SetReason 按此语义向用户说明
		result := sdkdto.BatchResult(nil)
		result.SetReason("未发现可导入的文件（目录为空或文件均不可读取）")
		return result, nil
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

			// 站点分类含义 → 子树归属站点键（site 含义 ID=站点 DB 行 id，经注册表投影解析）
			classifiedSiteKey := resolveClassifiedSiteKey(metadata, h.resolveSiteKeyByID)

			// 按归属裁决拆分：子任务站点继承父应答，同目录混合归属（真实域/local 回退）
			// 须拆为多个应答各自携带站点键
			localFiles, realFiles := partitionByAttribution(files, classifiedSiteKey)
			if resp := buildDirGroupResponse(path, dirRelPath, taskName, metadata, localFiles); resp != nil {
				ch <- resp
			}
			if resp := buildDirGroupResponse(path, dirRelPath, taskName, metadata, realFiles); resp != nil {
				ch <- resp
			}
		}
	}()

	return sdkdto.StreamResult(ch), nil
}

// buildDirGroupResponse 为同目录同归属的一组文件构造任务应答：
// 单文件=叶任务（身份=该文件的作品 ID），多文件=父容器+子任务。
// 组内归属一致（调用方按归属裁决拆分）；local 组维持既有任务身份形态
// （local-dir 前缀父任务/哈希身份），真实域组的父任务身份加站点键后缀，
// 区分同目录拆分后的两组任务（PluginTaskId 是同流应答的父任务合并键）。
// 全部文件不可访问（os.Stat 失败）时返回 nil。
func buildDirGroupResponse(path, dirRelPath, taskName string, metadata []PathMeaning, files []attributedFile) *sdkdto.TaskCreateResponse {
	if len(files) == 0 {
		return nil
	}
	siteKey := files[0].attr.SiteKey

	children := make([]*sdkdto.TaskCreateChildResponse, 0, len(files))
	for _, af := range files {
		fi, err := os.Stat(af.entry.FullPath)
		if err != nil {
			continue
		}

		// 作品身份：真实域=解析出的页级作品 ID，local 回退=文件内容哈希
		workID := af.entry.Hash
		fp := &FilePluginData{
			SchemaVersion: currentPluginDataVersion,
			FullPath:      af.entry.FullPath,
			RelPath:       af.entry.RelPath,
			Hash:          af.entry.Hash,
			Size:          fi.Size(),
			Metadata:      metadata,
		}
		if af.attr.isReal() {
			workID = af.attr.PageWorkID
			fp.SiteKey = af.attr.SiteKey
			fp.SiteWorkId = af.attr.PageWorkID
		}
		fpJSON, _ := json.Marshal(fp)

		children = append(children, &sdkdto.TaskCreateChildResponse{
			TaskName:     filepath.Base(af.entry.FullPath),
			SiteWorkId:   workID,
			Url:          "local://" + af.entry.FullPath,
			PluginData:   string(fpJSON),
			ResourceType: classifyResourceType(af.entry.FullPath),
		})
	}

	if len(children) == 0 {
		return nil
	}

	if len(children) == 1 {
		return &sdkdto.TaskCreateResponse{
			PluginTaskId: children[0].SiteWorkId,
			TaskName:     children[0].TaskName,
			SiteWorkId:   children[0].SiteWorkId,
			Url:          children[0].Url,
			PluginData:   children[0].PluginData,
			SiteKey:      siteKey,
			ResourceType: children[0].ResourceType,
		}
	}

	dp := &DirPluginData{
		SchemaVersion: currentPluginDataVersion,
		DirRelPath:    dirRelPath,
		Metadata:      metadata,
	}
	dpJSON, _ := json.Marshal(dp)

	parentID := fmt.Sprintf("local-dir-%s", dirRelPath)
	if files[0].attr.isReal() {
		parentID = parentID + "@" + siteKey
	}

	return &sdkdto.TaskCreateResponse{
		PluginTaskId: parentID,
		TaskName:     taskName,
		SiteWorkId:   parentID,
		Url:          "local://" + filepath.Join(path, dirRelPath),
		PluginData:   string(dpJSON),
		SiteKey:      siteKey,
		ResourceType: "", // 有 children 时由各 child 声明(parent 不声明)
		Children:     children,
	}
}

// CreateWorkInfo 从 PluginData 反序列化路径元数据，构建 WorkResponse
func (h *LocalImportWorkFetcher) CreateWorkInfo(task *sdkdto.TaskDTO) (*sdkdto.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}
	if err := checkPluginDataVersion(fp.SchemaVersion); err != nil {
		return nil, err
	}

	workName := filepath.Base(fp.FullPath)
	// 去除扩展名，扩展名由 Resource.Format 单独提供
	if ext := filepath.Ext(workName); ext != "" {
		workName = workName[:len(workName)-len(ext)]
	}

	// 作品身份：真实域归属（扫描期裁决进 PluginData）=页级作品 ID；local 回退=文件内容哈希
	siteWorkID := fp.Hash
	if fp.hasRealAttribution() {
		siteWorkID = fp.SiteWorkId
	}
	resp := &sdkdto.WorkResponse{
		Work: &sdkdto.WorkDTO{
			SiteWorkId:   &siteWorkID,
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
			// 用户供数只有名字：按名落本地域（ID=0 名称模式，宿主 find-or-create）。
			// 站点域行的身份只能来自站点侧 ID，名字不造站点域行
			if m.Name != "" {
				name := m.Name
				resp.LocalAuthors = append(resp.LocalAuthors, &sdkdto.LocalAuthorDTO{AuthorName: &name})
			}
		case "localTag":
			id, _ := strconv.ParseInt(m.ID, 10, 64)
			if id > 0 {
				resp.LocalTags = append(resp.LocalTags, &sdkdto.LocalTagDTO{Id: id})
			}
		case "siteTag":
			// 同 siteAuthor：按名落本地域
			if m.Name != "" {
				name := m.Name
				resp.LocalTags = append(resp.LocalTags, &sdkdto.LocalTagDTO{LocalTagName: &name})
			}
		case "workSet":
			// 作品集按名 upsert 落作品所属站点域：local 归属作品落 local 域
			// （workSet:{名} 即 local 站点的作品集 ID 约定形态）。真实域归属作品的
			// 站点侧作品集 ID 无法从名字派生（键即身份，无 ID 不造行），跳过该含义
			if fp.hasRealAttribution() {
				if h.ctx != nil {
					h.ctx.Infof("作品集含义按名仅支持 local 归属，真实域归属(%s)跳过: %s", fp.SiteKey, m.Name)
				}
				continue
			}
			resp.WorkSets = append(resp.WorkSets, &sdkdto.TaskWorkSetDTO{
				SiteWorkSetId: "workSet:" + m.Name,
				WorkSetName:   m.Name,
			})
		}
	}

	return resp, nil
}

// Start 打开文件,按 storeRoles 选择性返回 StoreSpec 流集合(主资源 downloaded + 缩略图 derived)与作品信息
func (h *LocalImportWorkFetcher) Start(ctx context.Context, task *sdkdto.TaskDTO, storeRoles []string) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	if task.PluginData == nil {
		return nil, nil, fmt.Errorf("pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*task.PluginData), &fp); err != nil {
		return nil, nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}
	if err := checkPluginDataVersion(fp.SchemaVersion); err != nil {
		return nil, nil, err
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
func (h *LocalImportWorkFetcher) Retry(task *sdkdto.TaskDTO) (*sdkdto.WorkResponse, error) {
	return nil, fmt.Errorf("retry 不支持，请使用 start")
}

// Pause 关闭文件句柄
func (h *LocalImportWorkFetcher) Pause(param *sdkdto.TaskResParam) error {
	return h.closeReader(param)
}

// Stop 关闭文件句柄
func (h *LocalImportWorkFetcher) Stop(param *sdkdto.TaskResParam) error {
	return h.closeReader(param)
}

// Resume 恢复下载:按 TaskResumeParam.StreamOffsets 续传未完成的主资源轨
// 缩略图(derived)为一次性产物,暂停的任务意味着其已完成,故 Resume 不再产出
func (h *LocalImportWorkFetcher) Resume(ctx context.Context, param *sdkdto.TaskResumeParam) ([]*sdkdto.StoreSpec, *sdkdto.WorkResponse, error) {
	if param.Task == nil || param.Task.PluginData == nil {
		return nil, nil, fmt.Errorf("task 或 pluginData 为空")
	}

	var fp FilePluginData
	if err := json.Unmarshal([]byte(*param.Task.PluginData), &fp); err != nil {
		return nil, nil, fmt.Errorf("解析 pluginData 失败: %w", err)
	}
	if err := checkPluginDataVersion(fp.SchemaVersion); err != nil {
		return nil, nil, err
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

func (h *LocalImportWorkFetcher) closeReader(param *sdkdto.TaskResParam) error {
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
