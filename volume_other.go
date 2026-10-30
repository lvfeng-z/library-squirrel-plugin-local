//go:build !windows

// 卷序列号读取的非 Windows 兜底（插件面向 Windows 发布；本文件仅保跨平台编译，
// 返回空串=无按盘记忆面）。
package main

// volumeSerialOfPath 非 Windows 恒返回空串（无卷序列号可取）。
func volumeSerialOfPath(string) string { return "" }
