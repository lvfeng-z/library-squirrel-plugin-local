// 卷序列号读取（Windows）：按盘记忆键的卷身份来源（决策1 第二层）。
package main

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

// volumeSerialOfPath 返回路径所在卷的卷序列号（GetVolumeInformationW，十六进制
// 八位）；路径无卷名或查询失败返回空串（调用方按无记忆面处理，回退纯自适应）。
func volumeSerialOfPath(path string) string {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return ""
	}
	rootPtr, err := syscall.UTF16PtrFromString(vol + `\`)
	if err != nil {
		return ""
	}
	var serial uint32
	r1, _, _ := procGetVolumeInformation.Call(
		uintptr(unsafe.Pointer(rootPtr)),
		0, 0, // 卷名缓冲区（不需要）
		uintptr(unsafe.Pointer(&serial)),
		0, 0, // 最大组件长度 / 文件系统标志（不需要）
		0, 0, // 文件系统名缓冲区（不需要）
	)
	if r1 == 0 {
		return ""
	}
	return fmt.Sprintf("%08X", serial)
}

var procGetVolumeInformation = syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumeInformationW")
