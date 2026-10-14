<template>
  <div class="li-pano">
    <header class="li-pano-header">
      <h2 class="li-pano-title">识别全景</h2>
      <span v-if="session" class="li-pano-root">{{ session.rootPath }}</span>
      <span v-if="snapshot" class="li-pano-time">识别于 {{ formatTime(snapshot.generatedAtMs) }}</span>
    </header>

    <el-alert
      v-if="loadError"
      class="li-pano-banner"
      type="error"
      :closable="false"
      show-icon
      :title="loadError"
    >
      <a class="li-pano-back" :href="parseRoute()">返回解析视图</a>
    </el-alert>
    <el-empty
      v-else-if="!session"
      description="未找到进行中的解析会话：请从工作台新建或恢复会话"
    >
      <a class="li-pano-back" :href="`${ROUTE_BASE}workbench`">返回本地导入工作台</a>
    </el-empty>

    <template v-else>
      <!-- 未识别态：全量识别入口（干跑 = 纯求值，零任务产出、零弹问） -->
      <section v-if="!snapshot && !recognizing" class="li-pano-card">
        <p class="li-pano-desc">
          对库根做全量识别干跑：按「规则档案 ∪ 会话赋值」合并求值全部文件，产出
          识别全景（目录结构含义、已识别资源、有价值未识别、序列群、待裁决项、
          规则命中与决策态）。识别不创建任何任务。
        </p>
        <el-button type="primary" :loading="starting" @click="startRecognition">
          开始全量识别
        </el-button>
      </section>

      <!-- 识别中：流式进度（阶段跃迁 + 计数） -->
      <section v-if="recognizing" class="li-pano-card">
        <div class="li-pano-progress-head">
          <span class="li-pano-progress-phase">{{ phaseLabel(progress.phase) }}</span>
          <span v-if="progress.total > 0" class="li-pano-progress-count">{{ progress.done }} / {{ progress.total }}</span>
        </div>
        <div class="li-pano-progress-track">
          <div class="li-pano-progress-bar" :style="{ width: progressPercent }"></div>
        </div>
      </section>

      <!-- 识别失败提示 -->
      <el-alert
        v-if="recognError"
        class="li-pano-banner"
        type="error"
        :closable="false"
        show-icon
        :title="recognError"
      />

      <template v-if="snapshot">
        <!-- 汇总统计徽章 -->
        <section class="li-pano-stats">
          <span class="li-pano-stat">文件 {{ snapshot.stats.totalFiles }}</span>
          <span class="li-pano-stat li-pano-stat-ok">已识别 {{ snapshot.stats.identified }}</span>
          <span class="li-pano-stat li-pano-stat-clue">有线索 {{ snapshot.stats.clueful }}</span>
          <span class="li-pano-stat">无线索 {{ snapshot.stats.plain }}</span>
          <span class="li-pano-stat">序列群 {{ snapshot.stats.groupCount }}</span>
          <span class="li-pano-stat li-pano-stat-warn">待裁决 {{ snapshot.stats.pendingFiles }}</span>
          <span v-if="snapshot.stats.skippedDirs > 0" class="li-pano-stat">增量跳过目录 {{ snapshot.stats.skippedDirs }}</span>
          <el-button size="small" :loading="starting" @click="startRecognition">重新识别</el-button>
        </section>

        <!-- 重识别差异（生成规则后的强制验证轮高亮：变更行标记） -->
        <section v-if="diff && diffTotal > 0" class="li-pano-diffbar">
          <span class="li-pano-diff-title">重识别差异（生成规则后的验证轮）：</span>
          <span class="li-pano-diff-chip is-changed">变更 {{ diff.changed ? diff.changed.length : 0 }}</span>
          <span class="li-pano-diff-chip is-added">新增 {{ diff.added ? diff.added.length : 0 }}</span>
          <span class="li-pano-diff-chip is-removed">消失 {{ diff.removed ? diff.removed.length : 0 }}</span>
          <span class="li-pano-diff-note">过窄＝差异里仍有未覆盖、过泛＝差异里错套——请在下方清单核对标色行</span>
        </section>

        <!-- 元素1：目录结构（层含义状态） -->
        <section class="li-pano-card">
          <h3 class="li-pano-card-title">目录结构 <span class="li-pano-card-note">{{ (snapshot.dirs || []).length }} 层 · 含义覆盖 {{ coveredDirCount }}/{{ dirNodes.length }}</span></h3>
          <div class="li-pano-dirlist">
            <div v-for="dir in dirNodes" :key="dir.relKey" class="li-pano-dir-row" :style="{ paddingLeft: `${dir.depth * 16 + 8}px` }">
              <span class="li-pano-dir-name">{{ dir.name }}</span>
              <span class="li-pano-dir-files">{{ dir.fileCount }} 文件</span>
              <template v-if="dir.meanings && dir.meanings.length">
                <span v-for="(m, i) in dir.meanings" :key="i" class="li-pano-dir-meaning">{{ meaningLabel(m) }}</span>
              </template>
              <span v-else-if="dir.depth > 0" class="li-pano-dir-uncovered">未覆盖</span>
            </div>
          </div>
        </section>

        <!-- 元素2/3：全库资源级清单（虚拟滚动；状态过滤） -->
        <section class="li-pano-card">
          <h3 class="li-pano-card-title">资源清单 <span class="li-pano-card-note">{{ filteredFiles.length }} 项 · 虚拟滚动</span></h3>
          <div class="li-pano-filter">
            <button
              v-for="f in fileFilters"
              :key="f.key"
              class="li-pano-filter-btn"
              :class="{ 'is-active': fileFilter === f.key }"
              @click="fileFilter = f.key"
            >{{ f.label }}</button>
          </div>
          <div ref="listViewport" class="li-pano-list" @scroll="onListScroll">
            <div class="li-pano-list-spacer" :style="{ height: `${filteredFiles.length * ROW_H}px` }"></div>
            <div
              v-for="row in visibleFiles"
              :key="row.relPath"
              class="li-pano-row"
              :class="diffMark(row.relPath) ? `is-diff-${diffMark(row.relPath)}` : ''"
              :style="{ transform: `translateY(${row.index * ROW_H}px)` }"
            >
              <span class="li-pano-row-state" :class="`is-${row.state}`" :title="stateLabel(row.state)">{{ stateDot(row.state) }}</span>
              <span class="li-pano-row-path" :title="row.relPath">{{ row.relPath }}</span>
              <span v-if="diffMark(row.relPath)" class="li-pano-row-diff" :class="`is-${diffMark(row.relPath)}`">{{ diffMark(row.relPath) === 'changed' ? 'Δ变更' : diffMark(row.relPath) === 'added' ? '＋新增' : '－消失' }}</span>
              <span class="li-pano-row-meta">
                <template v-if="row.final && row.final.length">
                  <span v-for="(d, i) in row.final" :key="i" class="li-pano-row-decl">{{ declLabel(d) }}</span>
                </template>
                <template v-if="row.clues && row.clues.length">
                  <span v-for="(c, i) in row.clues" :key="i" class="li-pano-row-clue">{{ clueLabel(c) }}</span>
                </template>
                <span v-if="row.pendingDims && row.pendingDims.length" class="li-pano-row-pending">待裁决:{{ row.pendingDims.join('/') }}</span>
                <span v-if="row.assigned" class="li-pano-row-assigned">已赋值</span>
              </span>
            </div>
          </div>
        </section>

        <!-- 元素4：序列（兄弟群：前缀/组大小/序号形态/成群落单） -->
        <section class="li-pano-card">
          <h3 class="li-pano-card-title">序列群 <span class="li-pano-card-note">{{ (snapshot.groups || []).length }} 群</span></h3>
          <p v-if="!(snapshot.groups || []).length" class="li-pano-empty">无成群序列</p>
          <div v-for="g in snapshot.groups || []" :key="`${g.dirRelKey}/${g.key}`" class="li-pano-group">
            <div class="li-pano-group-head">
              <span class="li-pano-group-key">{{ g.key || '（空前缀）' }}</span>
              <span class="li-pano-group-dir">{{ g.dirRelKey || '根目录' }}</span>
              <span class="li-pano-group-size">{{ g.size }} 成员</span>
              <span class="li-pano-group-rule">{{ g.ruleId || '结构' }}</span>
            </div>
            <div class="li-pano-group-members">
              <span v-for="m in g.members" :key="m.relPath" class="li-pano-group-member">
                #{{ m.seqText || m.seq }} {{ fileNameOf(m.relPath) }}
              </span>
            </div>
          </div>
        </section>

        <!-- 元素5：待裁决项（合成层声明冲突） -->
        <section class="li-pano-card">
          <h3 class="li-pano-card-title">待裁决项 <span class="li-pano-card-note">{{ snapshot.pending ? snapshot.pending.length : 0 }} 项</span></h3>
          <p v-if="!snapshot.pending || snapshot.pending.length === 0" class="li-pano-empty">无声明冲突</p>
          <div v-for="(p, i) in snapshot.pending || []" :key="i" class="li-pano-pending">
            <div class="li-pano-pending-head">
              <span class="li-pano-pending-file">{{ p.relPath }}</span>
              <span class="li-pano-pending-dim">{{ dimLabel(p.dimension) }}</span>
            </div>
            <div class="li-pano-pending-cands">
              <span v-for="(c, j) in p.candidates || []" :key="j" class="li-pano-pending-cand">
                {{ dimLabel(c.dimension) }}={{ c.value }}（{{ sourceLabel(c.source) }}<template v-if="c.ruleId"> · {{ c.ruleId }}</template>）
              </span>
            </div>
            <p class="li-pano-pending-reason">{{ p.reason }}</p>
          </div>
        </section>

        <!-- 元素6：规则档案态（来源/启用态/命中数） -->
        <section class="li-pano-card">
          <h3 class="li-pano-card-title">规则档案态 <span class="li-pano-card-note">{{ (snapshot.ruleStats || []).length }} 条</span></h3>
          <div class="li-pano-rules">
            <div class="li-pano-rule-row li-pano-rule-head">
              <span class="li-pano-rule-id">规则</span>
              <span class="li-pano-rule-src">来源</span>
              <span class="li-pano-rule-enabled">启用</span>
              <span class="li-pano-rule-hits">命中</span>
            </div>
            <div v-for="r in snapshot.ruleStats || []" :key="r.id" class="li-pano-rule-row" :title="r.description">
              <span class="li-pano-rule-id">{{ r.id }}</span>
              <span class="li-pano-rule-src">{{ sourceLabel(r.source) }}</span>
              <span class="li-pano-rule-enabled" :class="{ 'is-off': !r.enabled }">{{ r.enabled ? '启用' : '停用' }}</span>
              <span class="li-pano-rule-hits">{{ r.hitCount }}</span>
            </div>
          </div>
        </section>

        <!-- 元素7：会话决策态（骨架：赋值/忽略/选择集） -->
        <section class="li-pano-card">
          <h3 class="li-pano-card-title">会话决策态</h3>
          <div class="li-pano-decisions">
            <div class="li-pano-decision">
              <span class="li-pano-decision-count">{{ decisions ? decisions.assignments ? decisions.assignments.length : 0 : 0 }}</span>
              <span class="li-pano-decision-label">已手动解析</span>
            </div>
            <div class="li-pano-decision">
              <span class="li-pano-decision-count">{{ decisions ? (decisions.ignored ? decisions.ignored.length : 0) : 0 }}</span>
              <span class="li-pano-decision-label">已忽略</span>
            </div>
            <div class="li-pano-decision">
              <span class="li-pano-decision-count">{{ decisions ? (decisions.selection ? decisions.selection.length : 0) : 0 }}</span>
              <span class="li-pano-decision-label">已选定为任务</span>
            </div>
          </div>
          <p class="li-pano-empty">手动解析（单次赋值/泛化/裁决）与资源选择在对应子页操作，本节随决策推进填充。</p>
          <div v-if="snapshot.staleAssignments && snapshot.staleAssignments.length" class="li-pano-stale">
            <p class="li-pano-stale-title">失效赋值（引用的路径已不存在）：</p>
            <span v-for="s in snapshot.staleAssignments" :key="s" class="li-pano-stale-item">{{ s }}</span>
          </div>
        </section>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { Events } from '@wailsio/runtime'

// 工作台事件协议通道（与插件 Go 侧 session.go 约定一致）
const TOPIC_REQUEST = 'plugin:local-import:workbench:request'
const TOPIC_RESPONSE = 'plugin:local-import:workbench:response'
const TOPIC_EVENT = 'plugin:local-import:workbench:event'

// 本插件 view 路由复合键前缀
const ROUTE_BASE = '#/com.lvfeng.localImport/'

interface SessionSummary {
  rootPath: string
  state: string
  createdAtMs: number
  updatedAtMs: number
}

interface SnapDecl {
  dimension: string
  value: string
  source: string
  roleName?: string
  ruleId?: string
}

interface SnapFile {
  relPath: string
  fileName: string
  dirRelKey: string
  state: string
  final?: SnapDecl[]
  pendingDims?: string[]
  matchedRuleIds?: string[]
  groupKey?: string
  groupSeq?: number
  clues?: { kind: string; detail: string }[]
  assigned?: boolean
}

interface SnapGroup {
  dirRelKey: string
  key: string
  size: number
  ruleId: string
  members: { relPath: string; seq: number; seqText: string }[]
}

interface SnapPending {
  relPath: string
  fileName: string
  dimension: string
  reason: string
  candidates: SnapDecl[]
}

interface PanoramaSnapshot {
  schemaVersion: number
  generatedAtMs: number
  rootPath: string
  dirs: {
    relKey: string
    name: string
    depth: number
    fileCount: number
    meanings?: { type: string; id?: string; name?: string }[]
    meaningRuleId?: string
  }[]
  files: SnapFile[]
  groups: SnapGroup[]
  pending?: SnapPending[]
  ruleStats: { id: string; description: string; source: string; enabled: boolean; hitCount: number }[]
  staleAssignments?: string[]
  stats: {
    totalFiles: number
    identified: number
    clueful: number
    plain: number
    groupCount: number
    pendingFiles: number
    skippedDirs: number
    staleAssignments: number
  }
}

interface DecisionsView {
  assignments?: { relPath: string; generalized?: boolean; updatedAtMs?: number }[]
  ignored?: string[]
  selection?: string[]
}

// 重识别差异集（生成规则后的强制验证轮高亮载荷）
interface SnapDiff {
  changed: { relPath: string; before?: DiffConclusion; after?: DiffConclusion }[]
  added: { relPath: string; before?: DiffConclusion; after?: DiffConclusion }[]
  removed: { relPath: string; before?: DiffConclusion; after?: DiffConclusion }[]
}

interface DiffConclusion {
  state: string
  final?: SnapDecl[]
  pendingDims?: string[]
}

interface WorkbenchResponse {
  seq: number
  ok: boolean
  error?: string
  sessions?: SessionSummary[]
  session?: SessionSummary
  profileChanged?: boolean
  started?: boolean
  running?: boolean
  panorama?: PanoramaSnapshot
  decisions?: DecisionsView
  diff?: SnapDiff
}

interface WorkbenchEvent {
  kind: string
  root?: string
  phase?: string
  done?: number
  total?: number
  success?: boolean
  message?: string
}

// Wails 将 []byte 载荷以 base64 或 JSON 文本送达，双路解码
function decodePayload<T>(raw: unknown): T | null {
  if (raw == null) return null
  if (typeof raw === 'object') return raw as T
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw) as T
    } catch {
      /* 尝试 base64 */
    }
    try {
      // base64 → 字节 → UTF-8 文本：atob 直出 Latin-1 串会使多字节中文乱码
      const bin = atob(raw)
      const bytes = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
      return JSON.parse(new TextDecoder().decode(bytes)) as T
    } catch {
      /* 放弃 */
    }
  }
  return null
}

const session = ref<SessionSummary | null>(null)
const snapshot = ref<PanoramaSnapshot | null>(null)
const decisions = ref<DecisionsView | null>(null)
const diff = ref<SnapDiff | null>(null)
const loadError = ref('')
const recognizing = ref(false)
const starting = ref(false)
const recognError = ref('')
const progress = ref({ phase: '', done: 0, total: 0 })

let seq = 0
let offResponse: (() => void) | null = null
let offEvent: (() => void) | null = null
const pending = new Map<number, (resp: WorkbenchResponse) => void>()

function onResponse(event: { data?: unknown }) {
  const resp = decodePayload<WorkbenchResponse>(event && event.data)
  if (!resp || !pending.has(resp.seq)) return
  pending.get(resp.seq)?.(resp)
  pending.delete(resp.seq)
}

// 带序号配对的请求（识别全景载荷较大，超时放宽到 30 秒）
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
    }, 30000)
  })
}

// 从 hash 查询参数读取库根
function rootFromHash(): string {
  const hash = location.hash || ''
  const queryIndex = hash.indexOf('?')
  if (queryIndex < 0) return ''
  return new URLSearchParams(hash.slice(queryIndex + 1)).get('root') || ''
}

function parseRoute(): string {
  const root = session.value ? session.value.rootPath : rootFromHash()
  if (!root) return `${ROUTE_BASE}workbench-parse`
  return `${ROUTE_BASE}workbench-parse?root=${encodeURIComponent(root)}`
}

async function restoreSession(): Promise<string> {
  const root = rootFromHash()
  if (root) {
    const resp = await request('activateSession', { root: root })
    if (resp.ok && resp.session) {
      session.value = resp.session
      return resp.session.rootPath
    }
    loadError.value = resp.error || '恢复解析会话失败'
    return ''
  }
  const list = await request('listSessions')
  if (!list.ok) {
    loadError.value = list.error || '读取会话失败'
    return ''
  }
  const sessions = list.sessions || []
  if (sessions.length === 1) {
    const resp = await request('activateSession', { root: sessions[0].rootPath })
    if (resp.ok && resp.session) {
      session.value = resp.session
      return resp.session.rootPath
    }
  }
  return ''
}

async function loadPanorama(root: string): Promise<void> {
  const summary = await request('getPanorama', { root: root })
  if (!summary.ok || !summary.panorama) {
    // 未识别（created）：保留空态呈现识别入口
    if (summary.error && summary.error.indexOf('尚未完成') < 0) {
      loadError.value = summary.error
    }
    return
  }
  snapshot.value = summary.panorama
  decisions.value = summary.decisions || null
  diff.value = summary.diff || null
  // 文件级清单大库单独分节拉取（概览先渲染）
  const files = await request('getPanorama', { root: root, section: 'files' })
  if (files.ok && files.panorama) {
    snapshot.value = { ...snapshot.value, files: files.panorama.files, groups: files.panorama.groups, pending: files.panorama.pending }
    if (files.diff) diff.value = files.diff
  }
}

async function startRecognition(): Promise<void> {
  if (!session.value) return
  starting.value = true
  recognError.value = ''
  const resp = await request('startRecognition', { root: session.value.rootPath })
  starting.value = false
  if (!resp.ok) {
    recognError.value = resp.error || '启动全量识别失败'
    return
  }
  if (resp.running) {
    recognizing.value = true
    return
  }
  recognizing.value = true
  progress.value = { phase: 'scan', done: 0, total: 0 }
}

// 进度与终态事件（识别事件外的其他工作台事件忽略）
function onEvent(event: { data?: unknown }) {
  const evt = decodePayload<WorkbenchEvent>(event && event.data)
  if (!evt) return
  const root = session.value ? session.value.rootPath : ''
  if (evt.root && root && evt.root !== root) return
  if (evt.kind === 'recognitionProgress') {
    recognizing.value = true
    progress.value = {
      phase: evt.phase || '',
      done: evt.done || 0,
      total: evt.total || 0,
    }
  } else if (evt.kind === 'recognitionDone') {
    recognizing.value = false
    if (evt.success) {
      progress.value = { phase: '', done: 0, total: 0 }
      if (session.value) void loadPanorama(session.value.rootPath)
    } else {
      recognError.value = evt.message || '全量识别失败'
    }
  }
}

const phaseLabel = (phase: string): string => {
  switch (phase) {
    case 'scan': return '扫描目录结构'
    case 'index': return '构造库内谓词索引'
    case 'evaluate': return '合并求值（档案 ∪ 会话赋值）'
    case 'save': return '写入识别快照'
    default: return '准备中'
  }
}

const progressPercent = computed(() => {
  if (progress.value.total <= 0) return '12%'
  return `${Math.min(100, Math.round((progress.value.done / progress.value.total) * 100))}%`
})

// —— 资源级清单的虚拟滚动（切片渲染，千级/万级行不卡顿） ——

const ROW_H = 30

const fileFilters = [
  { key: 'all', label: '全部' },
  { key: 'identified', label: '已识别' },
  { key: 'clueful', label: '有线索' },
  { key: 'plain', label: '无线索' }
]
const fileFilter = ref('all')

const listViewport = ref<HTMLDivElement | null>(null)
const scrollTop = ref(0)
const viewportH = ref(420)

const filteredFiles = computed<SnapFile[]>(() => {
  const files = (snapshot.value ? snapshot.value.files : null) || []
  if (fileFilter.value === 'all') return files
  return files.filter((f) => f.state === fileFilter.value)
})

const visibleFiles = computed(() => {
  const start = Math.max(0, Math.floor(scrollTop.value / ROW_H) - 8)
  const end = Math.min(filteredFiles.value.length, start + Math.ceil(viewportH.value / ROW_H) + 16)
  return filteredFiles.value.slice(start, end).map((f, i) => ({ ...f, index: start + i }))
})

function onListScroll(e: Event): void {
  const target = e.target as HTMLDivElement
  scrollTop.value = target.scrollTop
}

// —— 呈现辅助 ——

const coveredDirCount = computed(() => dirNodes.value.filter((d) => d.meanings && d.meanings.length).length)
const dirNodes = computed(() => (snapshot.value ? snapshot.value.dirs : null) || [])

const stateLabel = (state: string): string => {
  switch (state) {
    case 'identified': return '已识别'
    case 'clueful': return '有价值未识别'
    default: return '未识别'
  }
}
const stateDot = (state: string): string => {
  switch (state) {
    case 'identified': return '●'
    case 'clueful': return '◐'
    default: return '○'
  }
}
const dimLabel = (dim: string): string => {
  switch (dim) {
    case 'author': return '作者'
    case 'title': return '作品名'
    case 'tag': return '标签'
    case 'collection': return '作品集'
    default: return dim
  }
}
const sourceLabel = (source: string): string => {
  switch (source) {
    case 'dir': return '目录'
    case 'filename': return '文件名'
    case 'constant': return '常量'
    case 'group': return '群组前缀'
    case 'predicate': return '库内命中'
    case 'assignment': return '手工赋值'
    case 'adjudication': return '裁决'
    case 'qna': return '泛化'
    case 'builtin': return '内置'
    case 'manual': return '手写'
    default: return source
  }
}
const clueLabel = (clue: { kind: string; detail: string }): string => {
  switch (clue.kind) {
    case 'predicate-author': return `库内作者：${clue.detail}`
    case 'predicate-tag': return `库内标签：${clue.detail}`
    case 'pixiv-id': return `pixiv 形态：${clue.detail}`
    case 'group-prefix': return `集名候选：${clue.detail}`
    default: return clue.detail
  }
}
const meaningLabel = (m: { type: string; name?: string }): string => {
  const labels: Record<string, string> = {
    localAuthor: '作者',
    siteAuthor: '站点作者',
    localTag: '标签',
    siteTag: '站点标签',
    workName: '作品名',
    workSet: '作品集',
    site: '站点',
    unknown: '未知'
  }
  const label = labels[m.type] || m.type
  return m.name ? `${label}:${m.name}` : label
}
const declLabel = (d: SnapDecl): string => {
  let text = `${dimLabel(d.dimension)}=${d.value}`
  if (d.roleName) text += `（${d.roleName}）`
  return text
}
const fileNameOf = (relPath: string): string => {
  const idx = relPath.lastIndexOf('/')
  return idx >= 0 ? relPath.slice(idx + 1) : relPath
}

// —— 重识别差异高亮（生成规则后的强制验证轮：过窄=仍有未覆盖、过泛=错套） ——

const diffIndex = computed<Map<string, string>>(() => {
  const index = new Map<string, string>()
  const d = diff.value
  if (!d) return index
  for (const entry of d.changed || []) index.set(entry.relPath, 'changed')
  for (const entry of d.added || []) index.set(entry.relPath, 'added')
  for (const entry of d.removed || []) if (!index.has(entry.relPath)) index.set(entry.relPath, 'removed')
  return index
})

const diffMark = (relPath: string): string => diffIndex.value.get(relPath) || ''

const diffTotal = computed(() => {
  const d = diff.value
  if (!d) return 0
  return (d.changed ? d.changed.length : 0) + (d.added ? d.added.length : 0) + (d.removed ? d.removed.length : 0)
})
const formatTime = (ms: number): string => {
  const date = new Date(ms)
  const pad = (n: number) => `${n}`.padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

onMounted(async () => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  offEvent = Events.On(TOPIC_EVENT, onEvent)
  const root = await restoreSession()
  if (root) {
    if (session.value && session.value.state === 'recognized') {
      await loadPanorama(root)
    }
    await nextTick()
    if (listViewport.value) {
      viewportH.value = listViewport.value.clientHeight || 420
    }
  }
})

onUnmounted(() => {
  if (offResponse) offResponse()
  if (offEvent) offEvent()
})
</script>

<style scoped>
.li-pano {
  max-width: 960px;
  margin: 0 auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.li-pano-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  flex-wrap: wrap;
}

.li-pano-title {
  margin: 0;
  font-size: 20px;
  color: var(--app-text-primary);
}

.li-pano-root {
  font-size: 13px;
  color: var(--app-text-secondary);
}

.li-pano-time {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pano-banner {
  border-radius: var(--app-radius);
}

.li-pano-back {
  font-size: 13px;
  color: var(--app-color-primary);
}

.li-pano-card {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.li-pano-card-title {
  margin: 0;
  font-size: 15px;
  color: var(--app-text-primary);
  display: flex;
  align-items: baseline;
  gap: 8px;
}

.li-pano-card-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-pano-desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--app-text-regular);
}

.li-pano-empty {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-secondary);
}

.li-pano-stats {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-pano-stat {
  font-size: 12px;
  padding: 3px 10px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-tag-neutral-text);
}

.li-pano-stat-ok {
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-pano-stat-clue {
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-pano-stat-warn {
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-pano-progress-head {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--app-text-regular);
}

.li-pano-progress-count {
  color: var(--app-text-secondary);
}

.li-pano-progress-track {
  height: 6px;
  border-radius: 3px;
  background: var(--app-border-color-light);
  overflow: hidden;
}

.li-pano-progress-bar {
  height: 100%;
  border-radius: 3px;
  background: var(--app-color-primary);
  transition: width 0.2s ease;
}

.li-pano-dirlist {
  max-height: 280px;
  overflow: auto;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 4px 0;
}

.li-pano-dir-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  padding: 2px 8px;
  white-space: nowrap;
}

.li-pano-dir-name {
  color: var(--app-text-primary);
  font-weight: 600;
}

.li-pano-dir-files {
  color: var(--app-text-secondary);
}

.li-pano-dir-meaning {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-pano-dir-uncovered {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-tag-neutral-text);
}

.li-pano-filter {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pano-filter-btn {
  font-size: 12px;
  padding: 3px 12px;
  border-radius: var(--app-radius-sm);
  border: 1px solid var(--app-border-color-light);
  background: var(--app-bg-surface);
  color: var(--app-text-regular);
  cursor: pointer;
}

.li-pano-filter-btn.is-active {
  border-color: var(--app-color-primary);
  color: var(--app-color-primary);
}

.li-pano-list {
  position: relative;
  height: 420px;
  overflow: auto;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
}

.li-pano-list-spacer {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  pointer-events: none;
}

.li-pano-row {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 30px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
}

.li-pano-row-state {
  flex: none;
  width: 14px;
  text-align: center;
}

.li-pano-row-state.is-identified {
  color: var(--app-color-success);
}

.li-pano-row-state.is-clueful {
  color: var(--app-color-warning);
}

.li-pano-row-state.is-plain {
  color: var(--app-text-secondary);
}

.li-pano-row-path {
  flex: none;
  max-width: 40%;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--app-text-primary);
}

/* 重识别差异高亮：变更/新增行底色标记（消失行不在当前清单，经差异条计数与手动解析页明细呈现） */
.li-pano-row.is-diff-changed {
  background: var(--app-tag-warning-bg);
}

.li-pano-row.is-diff-added {
  background: var(--app-tag-success-bg);
}

.li-pano-row.is-diff-removed {
  background: var(--app-tag-danger-bg);
}

.li-pano-row-diff {
  flex: none;
  font-size: 11px;
  padding: 0 6px;
  border-radius: var(--app-radius-sm);
}

.li-pano-row-diff.is-changed {
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-pano-row-diff.is-added {
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-pano-row-diff.is-removed {
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

/* 差异摘要条（验证轮高亮入口） */
.li-pano-diffbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 12px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
}

.li-pano-diff-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-pano-diff-chip {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
}

.li-pano-diff-chip.is-changed {
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-pano-diff-chip.is-added {
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-pano-diff-chip.is-removed {
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-pano-diff-note {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pano-row-meta {
  display: flex;
  gap: 6px;
  overflow: hidden;
}

.li-pano-row-decl {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-text-regular);
}

.li-pano-row-clue {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-pano-row-pending {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-pano-row-assigned {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-pano-group {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pano-group-head {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  flex-wrap: wrap;
}

.li-pano-group-key {
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-pano-group-dir {
  color: var(--app-text-secondary);
}

.li-pano-group-size {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-text-regular);
}

.li-pano-group-rule {
  color: var(--app-text-secondary);
  font-size: 11px;
}

.li-pano-group-members {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pano-group-member {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-fill-color);
  color: var(--app-text-regular);
}

.li-pano-pending {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pano-pending-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}

.li-pano-pending-file {
  color: var(--app-text-primary);
  font-weight: 600;
}

.li-pano-pending-dim {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-pano-pending-cands {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pano-pending-cand {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-fill-color);
  color: var(--app-text-regular);
}

.li-pano-pending-reason {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pano-rules {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  overflow: hidden;
}

.li-pano-rule-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 90px 60px 60px;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  padding: 4px 10px;
}

.li-pano-rule-row:nth-child(odd) {
  background: var(--app-fill-color);
}

.li-pano-rule-head {
  font-weight: 600;
  color: var(--app-text-secondary);
}

.li-pano-rule-id {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-primary);
}

.li-pano-rule-src,
.li-pano-rule-enabled,
.li-pano-rule-hits {
  color: var(--app-text-regular);
}

.li-pano-rule-enabled.is-off {
  color: var(--app-text-secondary);
}

.li-pano-decisions {
  display: flex;
  gap: 12px;
}

.li-pano-decision {
  flex: 1;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.li-pano-decision-count {
  font-size: 20px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-pano-decision-label {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pano-stale {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pano-stale-title {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-warning);
}

.li-pano-stale-item {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}
</style>
