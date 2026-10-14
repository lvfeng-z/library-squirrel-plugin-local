<template>
  <div class="li-parse">
    <header class="li-parse-header">
      <h2 class="li-parse-title">解析视图</h2>
      <p v-if="session" class="li-parse-root">库根：{{ session.rootPath }}</p>
    </header>

    <el-alert
      v-if="profileChanged"
      class="li-parse-banner"
      type="warning"
      :closable="false"
      show-icon
      title="规则档案在会话外被变更"
      description="既有识别结论可能与最新规则不一致，建议重新识别（重新识别入口属后续阶段）。"
    />
    <el-alert
      v-else-if="session"
      class="li-parse-banner"
      type="success"
      :closable="false"
      show-icon
      :title="`已恢复解析会话（${session.rootPath}）`"
      description="会话进度与赋值均从插件持久化存储恢复，可继续解析工作。"
    />
    <el-alert
      v-else-if="loadError"
      class="li-parse-banner"
      type="error"
      :closable="false"
      show-icon
      :title="loadError"
    />

    <nav v-if="session" class="li-parse-nav">
      <a
        v-for="sub in subpages"
        :key="sub.href"
        class="li-parse-nav-item"
        :href="sub.href"
      >{{ sub.title }}<span class="li-parse-nav-note">{{ sub.note }}</span></a>
    </nav>

    <section class="li-parse-body">
      <template v-if="session">
        <p class="li-parse-desc">
          解析视图已就绪：识别全景（全量识别干跑与七类元素呈现）与手动解析
          （单次赋值/泛化成规则/冲突裁决生成）可用，资源选择与任务创建将在
          后续阶段填充。
        </p>
      </template>
      <el-empty
        v-else-if="!loadError"
        description="未找到进行中的解析会话：请从工作台新建或恢复会话"
      >
        <a class="li-parse-back" :href="`#/com.lvfeng.localImport/workbench`">返回本地导入工作台</a>
      </el-empty>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

// 工作台事件协议通道（与插件 Go 侧 session.go 约定一致）
const TOPIC_REQUEST = 'plugin:local-import:workbench:request'
const TOPIC_RESPONSE = 'plugin:local-import:workbench:response'

interface SessionSummary {
  rootPath: string
  state: string
  createdAtMs: number
  updatedAtMs: number
}

interface WorkbenchResponse {
  seq: number
  ok: boolean
  error?: string
  sessions?: SessionSummary[]
  defaultRoot?: string
  session?: SessionSummary
  profileChanged?: boolean
}

// Wails 将 []byte 载荷以 base64 或 JSON 文本送达，双路解码
function decodePayload(raw: unknown): WorkbenchResponse | null {
  if (raw == null) return null
  if (typeof raw === 'object') return raw as WorkbenchResponse
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw) as WorkbenchResponse
    } catch {
      /* 尝试 base64 */
    }
    try {
      // base64 → 字节 → UTF-8 文本：atob 直出 Latin-1 串会使多字节中文乱码
      const bin = atob(raw)
      const bytes = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
      return JSON.parse(new TextDecoder().decode(bytes)) as WorkbenchResponse
    } catch {
      /* 放弃 */
    }
  }
  return null
}

// 本插件 view 路由复合键前缀（publicId=插件 id）
const ROUTE_BASE = '#/com.lvfeng.localImport/'

// 子页导航（携带库根，子页免二次寻址）
const subpageDefs = [
  { key: 'workbench-panorama', title: '识别全景', note: '全量识别与七类元素' },
  { key: 'workbench-manual-parse', title: '手动解析', note: '赋值·泛化·裁决' },
  { key: 'workbench-rules', title: '规则管理', note: '列表·编辑·内置启停·备份回退' },
  { key: 'workbench-observation', title: '解析观测', note: '建设中' }
]
const subpages = computed(() => {
  const rootSuffix = session.value ? `?root=${encodeURIComponent(session.value.rootPath)}` : ''
  return subpageDefs.map((sub) => ({
    href: `${ROUTE_BASE}${sub.key}${rootSuffix}`,
    title: sub.title,
    note: sub.note
  }))
})

const session = ref<SessionSummary | null>(null)
const profileChanged = ref(false)
const loadError = ref('')
let seq = 0
let offResponse: (() => void) | null = null
const pending = new Map<number, (resp: WorkbenchResponse) => void>()

function onResponse(event: { data?: unknown }) {
  const resp = decodePayload(event && event.data)
  if (!resp || !pending.has(resp.seq)) return
  pending.get(resp.seq)?.(resp)
  pending.delete(resp.seq)
}

// 发起一次带序号配对的请求；Go 侧按 seq 回发应答
function request(action: string, payload: Record<string, unknown> = {}): Promise<WorkbenchResponse> {
  seq += 1
  const reqSeq = seq
  return new Promise((resolve) => {
    pending.set(reqSeq, resolve)
    Events.Emit(TOPIC_REQUEST, JSON.stringify({ seq: reqSeq, action: action, ...payload }))
    setTimeout(() => {
      if (pending.has(reqSeq)) {
        pending.delete(reqSeq)
        resolve({ seq: reqSeq, ok: false, error: '插件响应超时（插件进程未运行或通道异常）' })
      }
    }, 8000)
  })
}

// 从 hash 查询参数读取进入本页时携带的库根
function rootFromHash(): string {
  const hash = location.hash || ''
  const queryIndex = hash.indexOf('?')
  if (queryIndex < 0) return ''
  return new URLSearchParams(hash.slice(queryIndex + 1)).get('root') || ''
}

// 恢复会话：按 hash 库根激活（含档案对账）；库根缺席时若恰一个会话则直接恢复；
// 会话可能刚经入口确认建立（导航先于建会话落库到达），短暂重试
async function restoreSession(attempt = 0): Promise<void> {
  const root = rootFromHash()
  if (root) {
    const resp = await request('activateSession', { root: root })
    if (resp.ok && resp.session) {
      session.value = resp.session
      profileChanged.value = resp.profileChanged === true
      return
    }
    if (resp.session === undefined && attempt < 3) {
      setTimeout(() => void restoreSession(attempt + 1), 300)
      return
    }
    loadError.value = resp.error || '恢复解析会话失败'
    return
  }
  const list = await request('listSessions')
  if (!list.ok) {
    loadError.value = list.error || '读取会话失败'
    return
  }
  const sessions = list.sessions || []
  if (sessions.length === 1) {
    session.value = sessions[0]
    const resp = await request('activateSession', { root: sessions[0].rootPath })
    if (resp.ok && resp.session) {
      session.value = resp.session
      profileChanged.value = resp.profileChanged === true
    }
  }
}

onMounted(() => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  void restoreSession()
})

onUnmounted(() => {
  if (offResponse) offResponse()
})
</script>

<style scoped>
.li-parse {
  max-width: 860px;
  margin: 0 auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.li-parse-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-parse-title {
  margin: 0;
  font-size: 20px;
  color: var(--app-text-primary);
}

.li-parse-root {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-parse-banner {
  width: 100%;
}

.li-parse-nav {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.li-parse-nav-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  text-decoration: none;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-parse-nav-item:hover {
  border-color: var(--app-color-primary);
}

.li-parse-nav-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-parse-body {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 16px;
}

.li-parse-desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--app-text-regular);
}

.li-parse-back {
  color: var(--app-color-primary);
}
</style>
