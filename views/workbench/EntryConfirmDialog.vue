<template>
  <AutoHeightDialog
    v-model:state="visible"
    width="min(480px, 90vw)"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
  >
    <template #header>
      <div class="li-confirm-header">
        <span>开始解析确认</span>
      </div>
    </template>
    <div class="li-confirm-body">
      <p class="li-confirm-text">
        即将对以下库根建立解析会话并进入解析视图（全量识别、手动解析与任务创建
        均在工作台完成，本次不直接创建任务）：
      </p>
      <p class="li-confirm-root">{{ question ? question.root : '' }}</p>
      <p class="li-confirm-note">
        确认后创建流程即结束（零任务）；如需直接导入，可稍后在解析视图中选择资源
        创建任务。弹窗静置超过 5 分钟将自动取消。
      </p>
    </div>
    <template #footer>
      <div class="li-confirm-actions">
        <el-button type="primary" autofocus @click="answer(true)">开始解析</el-button>
        <el-button @click="answer(false)">取消</el-button>
      </div>
    </template>
  </AutoHeightDialog>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

// 工作台事件协议通道（与插件 Go 侧 work_fetch.go 约定一致）
const TOPIC_REQUEST = 'plugin:local-import:workbench:request'
const TOPIC_EVENT = 'plugin:local-import:workbench:event'

// 本插件 view 路由复合键前缀（publicId=插件 id）
const PARSE_ROUTE = '#/com.lvfeng.localImport/workbench-parse'

interface WorkbenchEvent {
  kind: string
  seq?: number
  root?: string
}

// Wails 将 []byte 载荷以 base64 或 JSON 文本送达，双路解码
function decodePayload(raw: unknown): WorkbenchEvent | null {
  if (raw == null) return null
  if (typeof raw === 'object') return raw as WorkbenchEvent
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw) as WorkbenchEvent
    } catch {
      /* 尝试 base64 */
    }
    try {
      return JSON.parse(atob(raw)) as WorkbenchEvent
    } catch {
      /* 放弃 */
    }
  }
  return null
}

const visible = ref(false)
const question = ref<WorkbenchEvent | null>(null)
let offEvent: (() => void) | null = null
let emitSeq = 0

function onEvent(event: { data?: unknown }) {
  const evt = decodePayload(event && event.data)
  if (!evt) return
  if (evt.kind === 'entryConfirm' && evt.seq) {
    question.value = evt
    visible.value = true
  } else if (evt.kind === 'entryConfirmDismiss' && question.value && evt.seq === question.value.seq) {
    // 等待超时的关窗指令：超时判定在插件侧完成，此处只关窗不补发答复
    visible.value = false
    question.value = null
  }
}

// 回答复核确认（采纳/取消）并关窗；采纳时导航至解析视图（带库根），会话由插件侧
// 在收到答复后建立——解析视图对刚建立的会话做短暂重试，无需在此等待应答
function answer(accepted: boolean): void {
  const q = question.value
  visible.value = false
  question.value = null
  if (!q || !q.seq) return
  emitSeq += 1
  Events.Emit(
    TOPIC_REQUEST,
    JSON.stringify({ seq: emitSeq, action: 'confirmAnswer', confirmSeq: q.seq, accepted: accepted })
  )
  if (accepted && q.root) {
    location.hash = `${PARSE_ROUTE}?root=${encodeURIComponent(q.root)}`
  }
}

onMounted(() => {
  offEvent = Events.On(TOPIC_EVENT, onEvent)
})

onUnmounted(() => {
  if (offEvent) offEvent()
})
</script>

<style scoped>
.li-confirm-header {
  display: flex;
  align-items: center;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-confirm-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 4px 0;
}

.li-confirm-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--app-text-regular);
}

.li-confirm-root {
  margin: 0;
  padding: 8px 10px;
  border-radius: var(--app-radius-sm);
  background: var(--app-fill-color-light);
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-confirm-note {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

.li-confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
