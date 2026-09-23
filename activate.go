package main

import (
	"encoding/json"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// Activate 插件激活回调（扩展点注册与 URL 监听已由清单声明，宿主激活期派生）
func Activate(ctx sdkdto.PluginContext, handler *LocalImportWorkFetcher) {
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
