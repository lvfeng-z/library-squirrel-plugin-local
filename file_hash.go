package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// hashCancelCheckChunk 读循环取消位检查的分块粒度（甲3：4MB 分块处检查——
// 每读满该字节数查一次 ctx 取消，避免大文件读循环对取消位长盲区）。
const hashCancelCheckChunk = 4 << 20

// ComputeFileHash 流式计算文件 SHA-256 哈希（无取消/字节统计面；扫描期与既有
// 调用点沿用）。
func ComputeFileHash(filePath string) (string, error) {
	return computeFileHash(context.Background(), filePath, nil)
}

// ComputeFileHashCtx 带取消与读盘字节统计的流式哈希（verify 哈希循环用）：
// ctx 取消时读循环在 4MB 分块处中止并返回 ctx.Err()——Windows 对已进入内核的
// 挂起读无法强制中断，残余已知：读挂起的单个 goroutine 泄漏至进程重启（闸门
// 与终态不受影响，取消收流照常发生）；onBytes 按真实读盘字节逐块回调（爬坡
// 控制器窗口与进度速率字段的同一数据源；nil=不统计）。阶段5 哈希缓存命中的
// 文件不经本函数——零读盘字节计入控制器窗口（风险5 seam：缓存命中不产生读 IO）。
func ComputeFileHashCtx(ctx context.Context, filePath string, onBytes func(int64)) (string, error) {
	return computeFileHash(ctx, filePath, onBytes)
}

// computeFileHash 哈希实现：手动读循环支撑「取消位分块检查 + 逐块字节回调」
//（io.Copy 无这两处挂钩点）。
func computeFileHash(ctx context.Context, filePath string, onBytes func(int64)) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if ctx == nil {
		ctx = context.Background()
	}

	h := sha256.New()
	buf := make([]byte, 256<<10)
	var sinceCheck int64
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := h.Write(buf[:n]); werr != nil {
				return "", werr
			}
			if onBytes != nil {
				onBytes(int64(n))
			}
			sinceCheck += int64(n)
			if sinceCheck >= hashCancelCheckChunk {
				if err := ctx.Err(); err != nil {
					return "", err
				}
				sinceCheck = 0
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
