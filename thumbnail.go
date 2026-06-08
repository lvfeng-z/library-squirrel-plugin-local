package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// thumbnailGenerator 缩略图生成器接口
// 不同资源类型（视频、动图、文章等）实现此接口以支持缩略图生成。
// 扩展步骤：
//  1. 实现此接口（参考 videoThumbnailGenerator）
//  2. 定义该类型的扩展名集合
//  3. 在 init() 中注册到 extensionGenerators
type thumbnailGenerator interface {
	// generate 从指定文件生成缩略图
	// 返回 nil 表示该文件不需要或无法生成缩略图
	generate(filePath string) (*sdkdto.ThumbnailResponse, error)
}

// ---- 缩略图分派 ----

// extensionGenerators 扩展名到生成器的注册表
var extensionGenerators = map[string]thumbnailGenerator{}

func init() {
	registerVideoExtensions()
	// 预留：registerAnimatedImageExtensions()
	// 预留：registerDocumentExtensions()
}

// generateThumbnail 根据文件扩展名分派到对应的缩略图生成器
func generateThumbnail(filePath string) (*sdkdto.ThumbnailResponse, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	gen, ok := extensionGenerators[ext]
	if !ok {
		return nil, nil
	}
	return gen.generate(filePath)
}

// ---- 视频缩略图生成器 ----

// videoThumbnailGenerator 通过 FFmpeg 从视频文件提取帧作为缩略图
type videoThumbnailGenerator struct{}

// videoExtensions 支持缩略图生成的视频格式扩展名（小写，含点号）
var videoExtensions = map[string]bool{
	".mp4":  true,
	".avi":  true,
	".mkv":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
	".m4v":  true,
	".mpg":  true,
	".mpeg": true,
	".3gp":  true,
	".ts":   true,
}

func registerVideoExtensions() {
	gen := &videoThumbnailGenerator{}
	for ext := range videoExtensions {
		extensionGenerators[ext] = gen
	}
}

// ffmpegAvailable 缓存 FFmpeg 可用性检测结果
var (
	ffmpegAvailable bool
	ffmpegChecked   bool
)

// checkFFmpegAvailable 检查系统中是否安装了 FFmpeg
func checkFFmpegAvailable() bool {
	if !ffmpegChecked {
		_, err := exec.LookPath("ffmpeg")
		ffmpegAvailable = err == nil
		ffmpegChecked = true
	}
	return ffmpegAvailable
}

func (g *videoThumbnailGenerator) generate(filePath string) (*sdkdto.ThumbnailResponse, error) {
	if !checkFFmpegAvailable() {
		return nil, nil
	}

	// 从视频第一帧提取 JPEG 缩略图
	args := []string{
		"-i", filePath,
		"-vframes", "1",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "5",
		"pipe:1",
	}

	cmd := exec.Command("ffmpeg", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg 提取视频帧失败: %w", err)
	}

	if stdout.Len() == 0 {
		return nil, nil
	}

	return &sdkdto.ThumbnailResponse{
		Data:   stdout.Bytes(),
		Format: "jpg",
	}, nil
}
