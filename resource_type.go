package main

import (
	"path/filepath"
	"strings"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// imageExtensions 图片格式扩展名(小写,含点号);与主程序 image 规约 Formats 对齐
var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
	".gif": true, ".bmp": true, ".tiff": true, ".tif": true,
}

// documentExtensions 文档格式扩展名(小写,含点号);与主程序 document 规约 Formats 对齐
var documentExtensions = map[string]bool{
	".pdf": true, ".docx": true, ".doc": true, ".txt": true, ".rtf": true,
}

// audioExtensions 音频格式扩展名(小写,含点号);与主程序 audio 规约 Formats 对齐
var audioExtensions = map[string]bool{
	".mp3": true, ".m4a": true, ".aac": true, ".flac": true, ".wav": true, ".ogg": true,
}

// classifyResourceType 按文件扩展名分派预定义资源类型(image/video/audio/document/unknown)。
// 与 thumbnail.go 的 videoExtensions 同源(视频集合复用),确保类型识别与缩略图分派一致。
func classifyResourceType(fullPath string) string {
	ext := strings.ToLower(filepath.Ext(fullPath))
	switch {
	case imageExtensions[ext]:
		return sdkdto.ResourceTypeImage
	case videoExtensions[ext]:
		return sdkdto.ResourceTypeVideo
	case audioExtensions[ext]:
		return sdkdto.ResourceTypeAudio
	case documentExtensions[ext]:
		return sdkdto.ResourceTypeDocument
	default:
		return sdkdto.ResourceTypeUnknown
	}
}

// mainStoreRole 返回资源类型对应的主资源 store role。
// video→videoMain(可播放主体,本地封装文件直接作 downloaded 主轨);
// audio→audioMain(音频可播放主体);image→image;document→document;unknown→image(通用兜底,unknown 无结构约束)。
func mainStoreRole(resourceType string) string {
	switch resourceType {
	case sdkdto.ResourceTypeVideo:
		return sdkdto.StoreRoleVideoMain
	case sdkdto.ResourceTypeAudio:
		return sdkdto.StoreRoleAudioMain
	case sdkdto.ResourceTypeDocument:
		return sdkdto.StoreRoleDocument
	default:
		return sdkdto.StoreRoleImage
	}
}
