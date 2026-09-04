package main

import (
	"path/filepath"
	"testing"
)

// TestCreateEmptyDirSetsReason 空目录创建任务时零任务返回并经 SetReason 声明用户可读原因；
// 路径不可访问仍走错误返回（经 SDK 的 error 块通道透传，不经 reason）
func TestCreateEmptyDirSetsReason(t *testing.T) {
	dir := t.TempDir()

	h := &LocalImportTaskHandler{}
	result, err := h.Create(dir)
	if err != nil {
		t.Fatalf("空目录 Create 不应报错: %v", err)
	}
	if result.IsStream() {
		t.Fatalf("空目录应返回批量结果而非流式")
	}
	if tasks := result.Array(); len(tasks) != 0 {
		t.Fatalf("空目录不应产出任务，实际产出 %d 个", len(tasks))
	}
	if result.Reason() == "" {
		t.Fatalf("空目录应经 SetReason 声明原因，Reason() 为空")
	}

	missing := filepath.Join(dir, "不存在的子目录")
	if _, err := h.Create(missing); err == nil {
		t.Fatalf("不可访问路径应保持错误返回，实际无错误")
	}
}
