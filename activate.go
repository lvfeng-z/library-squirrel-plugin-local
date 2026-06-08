package main

import (
	"encoding/json"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// Activate 插件激活回调，注册扩展点和 URL 监听器
func Activate(ctx sdkdto.PluginContext, handler *LocalImportTaskHandler) {
	// 注册 local site
	localName := "local"
	localDesc := "本地文件导入"
	if err := ctx.AddSite([]*sdkdto.SiteDTO{
		{SiteName: &localName, SiteDescription: &localDesc},
	}); err != nil {
		ctx.Warnf("注册 local site 失败（可能已存在）: %v", err)
	}

	// 注册任务处理器
	if err := ctx.RegisterTaskHandler("main", "本地导入", "从本地路径导入文件", handler); err != nil {
		ctx.Errorf("注册任务处理器失败: %v", err)
		return
	}

	// 注册 URL 监听器
	// local:// 自定义协议 + Windows 本地路径（C:\...、D:\...、\\server\share\...）
	listeners := []string{
		`^local://.*`,
		`^[A-Za-z]:\\.*`,
		`^\\\\[^\]+\\.*`,
	}
	if err := ctx.RegisterUrlListener("main", listeners); err != nil {
		ctx.Errorf("注册URL监听器失败: %v", err)
		return
	}

	// 订阅前端分类响应
	handler.ctx = ctx
	handler.classifier = NewPathClassifier(ctx)

	ch, err := ctx.SubscribeFrontend("plugin:local-import:classify:response")
	if err != nil {
		ctx.Errorf("订阅前端事件失败: %v", err)
		return
	}

	go func() {
		ctx.Infof("开始监听前端分类响应事件")
		for data := range ch {
			ctx.Infof("收到前端分类响应: %s", string(data))
			var resp ClassifyResponse
			if err := json.Unmarshal(data, &resp); err != nil {
				ctx.Warnf("解析分类响应失败: %v", err)
				continue
			}
			handler.classifier.HandleResponse(&resp)
		}
		ctx.Infof("前端分类响应事件通道已关闭")
	}()

	ctx.Infof("本地文件导入插件已激活")
}
