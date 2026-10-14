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

	// 工作台会话管理器装配：会话 KV 读写、档案写单点串行与前端事件协议处理；
	// Create 入口三分支按会话分派（确认弹窗等待期心跳保活 + 插件侧 5 分钟超时）
	handler.sessions = NewSessionManager(ctx, ctx.GetLogger().Named("Workbench"))
	handler.entryConfirm = newEntryConfirmCoordinator(ctx, ctx.GetLogger().Named("Workbench"), entryConfirmWaitLimit)
	handler.sessions.SetConfirmAnswerHandler(handler.entryConfirm.HandleAnswer)

	// 激活期读档预热与校验观测：坏规则逐条告警（错误携带规则 ID），不阻塞激活；
	// 加载结果不驻留，Create/CreateWorkInfo 按需各自重读档案
	profile, errs := LoadProfile(NewContextProfileStore(ctx))
	for _, err := range errs {
		ctx.Warnf("规则档案载入异常: %v", err)
	}
	if profile != nil {
		ctx.Infof("规则档案预热完成: 用户规则 %d 条", len(profile.Rules))
	}

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

	// 工作台前端事件协议：订阅请求通道，逐条处理并按序号回发应答
	workbenchCh, err := ctx.SubscribeFrontend(workbenchTopicRequest)
	if err != nil {
		ctx.Errorf("订阅工作台请求事件失败: %v", err)
		return
	}
	go func() {
		ctx.Infof("开始监听工作台请求事件")
		for data := range workbenchCh {
			handler.sessions.HandleFrontendRequest(data)
		}
		ctx.Infof("工作台请求事件通道已关闭")
	}()

	ctx.Infof("本地文件导入插件已激活")
}
