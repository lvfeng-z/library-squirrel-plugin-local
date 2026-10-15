<template>
  <div class="li-wb">
    <header class="li-wb-header">
      <h2 class="li-wb-title">本地导入工作台</h2>
      <p class="li-wb-desc">
        解析会话按库根持久保留：可同时进行多个库根的适配，中断（关闭页面/重启应用）后恢复继续。
        进入解析视图后对库做全量识别呈现，由你驱动手动解析与任务创建。
      </p>
    </header>

    <el-alert
      v-if="sessions.length > 0"
      class="li-wb-restore"
      type="info"
      :closable="false"
      show-icon
      :title="`已从上次运行恢复 ${sessions.length} 个进行中的解析会话，可继续未完成的适配工作`"
    />

    <section class="li-wb-card">
      <h3 class="li-wb-card-title">新建解析会话</h3>
      <div class="li-wb-new-row">
        <el-input
          v-model="newRoot"
          class="li-wb-new-input"
          placeholder="库根路径（默认为资源库根目录，可输入其他本地目录）"
          clearable
        />
        <el-button type="primary" :loading="creating" @click="doCreate">新建解析会话</el-button>
      </div>
      <el-alert
        v-if="conflict"
        class="li-wb-conflict"
        type="warning"
        :closable="false"
        show-icon
        :title="`该库根已有进行中的解析会话（${conflict.rootPath}）`"
        :description="`同一库根至多一个活动会话：可恢复既有会话继续，或确认覆盖重建（既有会话的解析进度与赋值将被丢弃）。`"
      />
      <div v-if="conflict" class="li-wb-conflict-actions">
        <el-button type="primary" @click="enterParse(conflict.rootPath)">恢复既有会话</el-button>
        <el-button type="danger" :loading="replacing" @click="doReplace">覆盖重建</el-button>
        <el-button @click="conflict = null">取消</el-button>
      </div>
      <p v-if="createError" class="li-wb-error">{{ createError }}</p>
    </section>

    <section class="li-wb-card">
      <h3 class="li-wb-card-title">解析会话</h3>
      <div v-if="loading" class="li-wb-hint">正在读取会话…</div>
      <el-empty v-else-if="sessions.length === 0" description="暂无解析会话：在上方输入库根新建" />
      <ul v-else class="li-wb-list">
        <li v-for="s in sessions" :key="s.rootPath" class="li-wb-row">
          <div class="li-wb-row-main">
            <span class="li-wb-row-root">{{ s.rootPath }}</span>
            <el-tag class="li-wb-row-state" size="small">{{ stateLabel(s.state) }}</el-tag>
          </div>
          <div class="li-wb-row-side">
            <span class="li-wb-row-time">最近活动 {{ formatTime(s.updatedAtMs) }}</span>
            <el-button size="small" type="primary" @click="enterParse(s.rootPath)">进入解析</el-button>
            <el-button
              size="small"
              :type="pendingDelete === s.rootPath ? 'danger' : 'default'"
              @click="doDelete(s.rootPath)"
            >{{ pendingDelete === s.rootPath ? '确认删除' : '删除' }}</el-button>
          </div>
        </li>
      </ul>
      <p v-if="listError" class="li-wb-error">{{ listError }}</p>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Events } from '@wailsio/runtime'

// 工作台事件协议通道（与插件 Go 侧 session.go 约定一致）
const TOPIC_REQUEST = 'plugin:local-import:workbench:request'
const TOPIC_RESPONSE = 'plugin:local-import:workbench:response'

// 本插件 view 路由复合键前缀（publicId=插件 id，宿主按 publicId/extensionId 注册路由）
const PARSE_ROUTE = '#/com.lvfeng.localImport/workbench-parse'

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
  conflict?: SessionSummary
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

const sessions = ref<SessionSummary[]>([])
const loading = ref(false)
const listError = ref('')
const newRoot = ref('')
const creating = ref(false)
const createError = ref('')
const conflict = ref<SessionSummary | null>(null)
const replacing = ref(false)
const pendingDelete = ref('')
let deleteArmTimer: ReturnType<typeof setTimeout> | null = null
let seq = 0
let offResponse: (() => void) | null = null
const pending = new Map<number, (resp: WorkbenchResponse) => void>()

function stateLabel(state: string): string {
  if (state === 'recognized') return '已全量识别'
  return '进行中'
}

function formatTime(ms: number): string {
  if (!ms) return '—'
  return new Date(ms).toLocaleString()
}

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

async function refresh(): Promise<void> {
  loading.value = true
  listError.value = ''
  const resp = await request('listSessions')
  loading.value = false
  if (!resp.ok) {
    listError.value = resp.error || '读取会话失败'
    return
  }
  sessions.value = resp.sessions || []
  if (resp.defaultRoot && !newRoot.value) {
    newRoot.value = resp.defaultRoot
  }
}

async function doCreate(): Promise<void> {
  const root = newRoot.value.trim()
  if (!root) {
    createError.value = '请输入库根路径'
    return
  }
  creating.value = true
  createError.value = ''
  conflict.value = null
  const resp = await request('createSession', { root: root })
  creating.value = false
  if (!resp.ok) {
    if (resp.conflict) {
      conflict.value = resp.conflict
      return
    }
    createError.value = resp.error || '建立会话失败'
    return
  }
  newRoot.value = ''
  await refresh()
  if (resp.session) {
    enterParse(resp.session.rootPath)
  }
}

async function doReplace(): Promise<void> {
  if (!conflict.value) return
  replacing.value = true
  const root = conflict.value.rootPath
  const resp = await request('replaceSession', { root: root })
  replacing.value = false
  if (!resp.ok) {
    createError.value = resp.error || '覆盖重建失败'
    return
  }
  conflict.value = null
  await refresh()
  if (resp.session) {
    enterParse(resp.session.rootPath)
  }
}

function doDelete(root: string): void {
  if (pendingDelete.value !== root) {
    // 破坏性操作两段确认：首次点击武装，3 秒内再点执行
    pendingDelete.value = root
    if (deleteArmTimer) clearTimeout(deleteArmTimer)
    deleteArmTimer = setTimeout(() => {
      pendingDelete.value = ''
    }, 3000)
    return
  }
  if (deleteArmTimer) clearTimeout(deleteArmTimer)
  pendingDelete.value = ''
  void request('deleteSession', { root: root }).then(() => refresh())
}

// 编程导航至二级解析页面：hash 路由直改（插件 view 路由=publicId/extensionId，
// 锚点变更即普通路由跳转），库根经查询参数传递
function enterParse(root: string): void {
  location.hash = `${PARSE_ROUTE}?root=${encodeURIComponent(root)}`
}

// 进入时地址携 root 查询参数（创建流确认弹窗的落点形态）：取走参数并自动进入
// 对应会话。参数剥离发生在下钻导航之前——历史记录里本页条目不带查询串，从解析
// 视图后退（Alt+←）回到本页时不会再次自动进入
function consumeRootParam(): string | null {
  const hash = location.hash || ''
  const queryIdx = hash.indexOf('?')
  if (queryIdx < 0) return null
  const root = new URLSearchParams(hash.slice(queryIdx + 1)).get('root')
  if (!root) return null
  // 保留 vue-router 写入的 history.state（页面位置簿记），只改写 URL 剥掉查询串
  history.replaceState(history.state, '', `${location.pathname}${location.search}${hash.slice(0, queryIdx)}`)
  return root
}

onMounted(() => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  const rootParam = consumeRootParam()
  void refresh()
  if (rootParam) enterParse(rootParam)
})
</script>

<style scoped>
.li-wb {
  max-width: 860px;
  margin: 0 auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.li-wb-header {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-wb-title {
  margin: 0;
  font-size: 20px;
  color: var(--app-text-primary);
}

.li-wb-desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

.li-wb-card {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  padding: 16px;
  background: var(--app-bg-surface);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.li-wb-card-title {
  margin: 0;
  font-size: 15px;
  color: var(--app-text-primary);
}

.li-wb-new-row {
  display: flex;
  gap: 8px;
}

.li-wb-new-input {
  flex: 1;
}

.li-wb-conflict-actions {
  display: flex;
  gap: 8px;
}

.li-wb-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}

.li-wb-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 4px;
  border-bottom: 1px solid var(--app-border-color-lighter);
}

.li-wb-row:last-child {
  border-bottom: none;
}

.li-wb-row-main {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.li-wb-row-root {
  font-weight: 600;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-wb-row-side {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.li-wb-row-time {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-wb-hint {
  font-size: 13px;
  color: var(--app-text-secondary);
}

.li-wb-error {
  margin: 0;
  font-size: 13px;
  color: var(--app-status-fail-text);
}
</style>
