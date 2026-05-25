package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// ComputeFileHash 流式计算文件 SHA-256 哈希
func ComputeFileHash(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
