package main

import "os"

// Shutdown 插件关闭回调，清理所有打开的文件句柄
func Shutdown(handler *LocalImportTaskHandler) {
	handler.readers.Range(func(key, value any) bool {
		handler.readers.Delete(key)
		value.(*os.File).Close()
		return true
	})
}
