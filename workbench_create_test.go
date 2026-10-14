// Create 入口三分支的单元测试：工作台发起占位过滤、库根命中既有会话的意图信号
// 收尾、无会话时的入口确认弹窗流（采纳建会话/取消/超时=取消）与等待期心跳上报。
package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// waitForEntryConfirmEvent 轮询等待指定种类的工作台事件发布并解出载荷。
func waitForEntryConfirmEvent(t *testing.T, ctx *workbenchTestCtx, kind string) workbenchFrontendEvent {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ctx.mu.Lock()
		published := append([]publishedEvent(nil), ctx.published...)
		ctx.mu.Unlock()
		for i := len(published) - 1; i >= 0; i-- {
			if published[i].topic != workbenchTopicEvent {
				continue
			}
			var evt workbenchFrontendEvent
			if err := json.Unmarshal([]byte(published[i].data), &evt); err != nil || evt.Kind != kind {
				continue
			}
			return evt
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待工作台事件 %q 发布超时", kind)
	return workbenchFrontendEvent{}
}

// newTestFetcher 构造装配了会话管理器与确认协调器的拉取器（协调器等待上限与
// 心跳节拍可注入短值，事件经上下文桩记录）。
func newTestFetcher(t *testing.T, waitLimit, tick time.Duration) (*LocalImportWorkFetcher, *SessionManager, *entryConfirmCoordinator, *workbenchTestCtx) {
	t.Helper()
	store := newFakeSessionStore()
	mgr, ctx := newTestManager(t, store)
	var coord *entryConfirmCoordinator
	if tick > 0 {
		coord = newEntryConfirmCoordinatorWithTick(ctx, nil, waitLimit, tick)
	} else {
		coord = newEntryConfirmCoordinator(ctx, nil, waitLimit)
	}
	mgr.SetConfirmAnswerHandler(coord.HandleAnswer)
	h := &LocalImportWorkFetcher{sessions: mgr, entryConfirm: coord}
	return h, mgr, coord, ctx
}

// TestCreateRequiresSessionManager 会话管理器未装配时 Create 直接报错（装配遗漏
// 显性失败，不静默走入旧扫描路径）。
func TestCreateRequiresSessionManager(t *testing.T) {
	h := &LocalImportWorkFetcher{}
	if _, err := h.Create(`D:\库`); err == nil || !strings.Contains(err.Error(), "会话管理器未装配") {
		t.Fatalf("未装配会话管理器应报错，实际: %v", err)
	}
}

// TestCreateBranchWorkbenchInitiatedPlaceholder 分支 1（工作台发起）：会话带发起
// 标记的创建进占位过滤路径——零任务、按 reason 说明占位（选择集过滤属后续阶段），
// 不弹确认、不建会话、标记被消费。
func TestCreateBranchWorkbenchInitiatedPlaceholder(t *testing.T) {
	h, mgr, _, ctx := newTestFetcher(t, time.Second, 0)
	if _, err := mgr.CreateSession(`D:\库甲`); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}
	if err := mgr.BeginWorkbenchCreate(`D:\库甲`); err != nil {
		t.Fatalf("置位发起标记失败: %v", err)
	}

	result, err := h.Create(`local://D:\库甲`)
	if err != nil {
		t.Fatalf("工作台发起创建不应报错: %v", err)
	}
	if result.IsStream() || len(result.Array()) != 0 {
		t.Fatalf("占位过滤路径应零任务，实际: 流=%v 任务=%d", result.IsStream(), len(result.Array()))
	}
	if result.Reason() == "" {
		t.Fatalf("占位过滤路径应经 SetReason 说明占位")
	}
	if kind, _ := result.Intent(); kind != sdkdto.CreateIntentNone {
		t.Fatalf("占位过滤路径不应携带意图信号，实际 %v", kind)
	}
	if mgr.ConsumeWorkbenchCreateMarker(`D:\库甲`) {
		t.Fatalf("发起标记应已被本次创建消费")
	}
	ctx.mu.Lock()
	published := len(ctx.published)
	ctx.mu.Unlock()
	if published != 0 {
		t.Fatalf("工作台发起创建不应发布任何前端事件，实际 %d 条", published)
	}
}

// TestCreateBranchSessionHitIntentToParse 分支 2（库根命中既有会话）：不重问、
// 零任务、意图信号「转入解析」收尾，URL 的 local:// 前缀与库根书写形态不影响命中。
func TestCreateBranchSessionHitIntentToParse(t *testing.T) {
	h, mgr, _, ctx := newTestFetcher(t, time.Second, 0)
	if _, err := mgr.CreateSession(`D:\库乙`); err != nil {
		t.Fatalf("建立会话失败: %v", err)
	}

	result, err := h.Create(`local://d:/库乙/`)
	if err != nil {
		t.Fatalf("命中既有会话的创建不应报错: %v", err)
	}
	if result.IsStream() || len(result.Array()) != 0 {
		t.Fatalf("命中既有会话应零任务，实际: 流=%v 任务=%d", result.IsStream(), len(result.Array()))
	}
	kind, message := result.Intent()
	if kind != sdkdto.CreateIntentToParse {
		t.Fatalf("应携带「转入解析」意图信号，实际 %v", kind)
	}
	if message == "" {
		t.Fatalf("意图信号应携带用户可读文案")
	}
	ctx.mu.Lock()
	published := len(ctx.published)
	ctx.mu.Unlock()
	if published != 0 {
		t.Fatalf("命中既有会话不应发布任何前端事件，实际 %d 条", published)
	}
}

// TestCreateBranchConfirmAccept 分支 3（无会话→确认弹窗）·采纳：拉起事件携带库根
// 与配对序号；答复采纳后建立会话并意图信号「转入解析」收尾（零任务）。
func TestCreateBranchConfirmAccept(t *testing.T) {
	h, mgr, coord, ctx := newTestFetcher(t, 5*time.Second, 0)

	type outcome struct {
		result *sdkdto.TaskCreateResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := h.Create(`D:\库丙`)
		done <- outcome{result, err}
	}()

	evt := waitForEntryConfirmEvent(t, ctx, workbenchEventEntryConfirm)
	if evt.Root != `D:\库丙` || evt.Seq == 0 {
		t.Fatalf("拉起事件应携带库根与确认序号: %+v", evt)
	}
	if !coord.HandleAnswer(evt.Seq, true) {
		t.Fatalf("在等确认的答复应命中")
	}

	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("采纳后的创建不应报错: %v", out.err)
		}
		if len(out.result.Array()) != 0 {
			t.Fatalf("采纳后应零任务，实际 %d 个", len(out.result.Array()))
		}
		if kind, _ := out.result.Intent(); kind != sdkdto.CreateIntentToParse {
			t.Fatalf("采纳后应以「转入解析」意图收尾，实际 %v", kind)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("采纳后创建未收尾")
	}

	if _, err := mgr.SessionOf(`D:\库丙`); err != nil {
		t.Fatalf("采纳后应建立会话: %v", err)
	}
}

// TestCreateBranchConfirmCancel 分支 3·取消：答复取消后零任务、意图信号「已取消」
// 收尾、不建会话。
func TestCreateBranchConfirmCancel(t *testing.T) {
	h, mgr, coord, ctx := newTestFetcher(t, 5*time.Second, 0)

	done := make(chan *sdkdto.TaskCreateResult, 1)
	go func() {
		result, err := h.Create(`D:\库丁`)
		if err != nil {
			t.Errorf("取消路径的创建不应报错: %v", err)
		}
		done <- result
	}()

	evt := waitForEntryConfirmEvent(t, ctx, workbenchEventEntryConfirm)
	coord.HandleAnswer(evt.Seq, false)

	select {
	case result := <-done:
		if len(result.Array()) != 0 {
			t.Fatalf("取消路径应零任务，实际 %d 个", len(result.Array()))
		}
		kind, message := result.Intent()
		if kind != sdkdto.CreateIntentCancelled {
			t.Fatalf("取消路径应以「已取消」意图收尾，实际 %v", kind)
		}
		if message == "" {
			t.Fatalf("取消意图应携带用户可读文案")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("取消后创建未收尾")
	}

	if _, err := mgr.SessionOf(`D:\库丁`); err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("取消路径不应建立会话，实际: %v", err)
	}
}

// TestCreateBranchConfirmTimeout 分支 3·超时=取消：等待上限（注入短值）内无答复
// 即自动取消——意图信号「已取消」（超时文案）、零任务、不建会话，并向前端发布
// 关窗指令；同一确认的迟到答复不再命中。
func TestCreateBranchConfirmTimeout(t *testing.T) {
	h, mgr, coord, ctx := newTestFetcher(t, 40*time.Millisecond, 0)

	done := make(chan *sdkdto.TaskCreateResult, 1)
	go func() {
		result, err := h.Create(`D:\库戊`)
		if err != nil {
			t.Errorf("超时路径的创建不应报错: %v", err)
		}
		done <- result
	}()

	evt := waitForEntryConfirmEvent(t, ctx, workbenchEventEntryConfirm)
	select {
	case result := <-done:
		if len(result.Array()) != 0 {
			t.Fatalf("超时路径应零任务，实际 %d 个", len(result.Array()))
		}
		kind, message := result.Intent()
		if kind != sdkdto.CreateIntentCancelled || !strings.Contains(message, "超时") {
			t.Fatalf("超时路径应以「已取消」（超时文案）意图收尾，实际 %v %q", kind, message)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("超时后创建未收尾")
	}

	waitForEntryConfirmEvent(t, ctx, workbenchEventEntryConfirmDismiss)
	if coord.HandleAnswer(evt.Seq, true) {
		t.Fatalf("超时解除后的迟到答复不应命中")
	}
	if _, err := mgr.SessionOf(`D:\库戊`); err == nil || !strings.Contains(err.Error(), "未找到") {
		t.Fatalf("超时路径不应建立会话，实际: %v", err)
	}
}

// TestCreateConfirmHeartbeatDuringWait 分支 3 等待期心跳：等待期（注入短节拍与
// 短上限）内按节拍周期上报心跳维持宿主空闲窗。
func TestCreateConfirmHeartbeatDuringWait(t *testing.T) {
	h, _, _, ctx := newTestFetcher(t, 120*time.Millisecond, 5*time.Millisecond)
	reporter := &countingStubReporter{}

	go func() {
		_, _ = h.CreateWithHeartbeat(`D:\库己`, reporter)
	}()

	waitForEntryConfirmEvent(t, ctx, workbenchEventEntryConfirm)
	// 等待超时自然收尾（不答复），期间应已多次上报心跳
	time.Sleep(200 * time.Millisecond)
	if got := reporter.count.Load(); got < 2 {
		t.Fatalf("等待期应按节拍多次上报心跳，实际 %d 次", got)
	}
}
