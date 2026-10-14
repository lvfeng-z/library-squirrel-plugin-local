<template>
  <div class="li-manual">
    <header class="li-manual-header">
      <h2 class="li-manual-title">手动解析</h2>
      <span v-if="session" class="li-manual-root">{{ session.rootPath }}</span>
    </header>

    <el-alert
      v-if="loadError"
      class="li-manual-banner"
      type="error"
      :closable="false"
      show-icon
      :title="loadError"
    >
      <a class="li-manual-back" :href="parseRoute()">返回解析视图</a>
    </el-alert>
    <el-empty
      v-else-if="!session"
      description="未找到进行中的解析会话：请从工作台新建或恢复会话"
    >
      <a class="li-manual-back" :href="`${ROUTE_BASE}workbench`">返回本地导入工作台</a>
    </el-empty>

    <template v-else>
      <!-- 识别前置：全景快照不在场时无法定位待解析对象 -->
      <el-alert
        v-if="!snapshot"
        class="li-manual-banner"
        type="info"
        :closable="false"
        show-icon
        title="该会话尚未完成全量识别"
        description="手动解析（赋值/泛化/裁决）依赖识别全景提供文件上下文与待裁决清单，请先在识别全景页完成识别。"
      >
        <a class="li-manual-back" :href="panoramaRoute()">前往识别全景</a>
      </el-alert>

      <template v-else>
        <!-- 操作反馈区 -->
        <el-alert v-if="notice" class="li-manual-banner" type="success" :closable="false" show-icon :title="notice" />
        <el-alert v-if="actionError" class="li-manual-banner" type="error" :closable="false" show-icon :title="actionError" />
        <div v-if="recognizing" class="li-manual-busy">验证轮重识别进行中……完成后下方差异区自动刷新</div>

        <!-- 条件策略生成物透出：条件 JSON（档案 v3 表达式树，进阶手写参照） -->
        <div v-if="lastCondition" class="li-manual-condjson">
          <div class="li-manual-condjson-head">条件策略生成物（条件 JSON，与规则档案同构的表达式片段）</div>
          <pre class="li-manual-condjson-body">{{ lastCondition }}</pre>
        </div>

        <!-- 一、单次赋值（会话态知识，不升格规则） -->
        <section class="li-manual-card">
          <h3 class="li-manual-card-title">单次赋值 <span class="li-manual-card-note">特异性结果留会话 · 跨重识别持续生效</span></h3>
          <div class="li-manual-picker">
            <el-input v-model="filterText" placeholder="按文件名/路径过滤待解析候选（未识别与有线索）" clearable />
            <div v-if="formRel" class="li-manual-picker-target">目标：{{ formRel }}</div>
          </div>
          <div v-if="candidateFiles.length" class="li-manual-candidates">
            <button
              v-for="f in candidateFiles"
              :key="f.relPath"
              class="li-manual-candidate"
              :class="{ 'is-active': f.relPath === formRel }"
              @click="pickCandidate(f)"
            >
              <span class="li-manual-candidate-state" :class="`is-${f.state}`">{{ stateDot(f.state) }}</span>
              <span class="li-manual-candidate-path">{{ f.relPath }}</span>
              <span v-if="f.assigned" class="li-manual-candidate-flag">已赋值</span>
            </button>
            <p v-if="candidateFiles.length >= CANDIDATE_CAP" class="li-manual-hint">
              仅显示前 {{ CANDIDATE_CAP }} 项候选，可细化过滤或直接在下方输入相对路径
            </p>
          </div>
          <div class="li-manual-form">
            <el-input v-model="formRel" class="li-manual-form-wide" placeholder="相对库根路径（如 作者甲/画集/01.jpg）" />
            <el-input v-model="formAuthor" placeholder="作者" clearable />
            <el-input v-model="formAuthorRole" placeholder="作者角色（如 原画，可空）" clearable />
            <el-input v-model="formTitle" placeholder="作品名" clearable />
            <el-input v-model="formTagsText" placeholder="标签（逗号分隔多个，可空）" clearable />
            <el-input v-model="formCollection" placeholder="作品集（集归属，可空）" clearable />
          </div>
          <div class="li-manual-actions">
            <el-button type="primary" :loading="assigning" @click="doAssign">保存赋值</el-button>
            <el-button :loading="assigning" @click="doClearAssign">清除该文件赋值</el-button>
            <el-button :loading="assigning" @click="doIgnore(true)">忽略此文件</el-button>
          </div>
          <p class="li-manual-hint">空维度不覆盖档案结论；全部留空即清除赋值。已忽略项：{{ ignoredCount }} 个</p>
        </section>

        <!-- 二、已有赋值与泛化成规则 -->
        <section class="li-manual-card">
          <h3 class="li-manual-card-title">已有赋值 <span class="li-manual-card-note">{{ assignments.length }} 项</span></h3>
          <p v-if="!assignments.length" class="li-manual-empty">尚无手工解析赋值</p>
          <div v-for="a in assignments" :key="a.relPath" class="li-manual-assign-row">
            <span class="li-manual-assign-path" :title="a.relPath">{{ a.relPath }}</span>
            <span class="li-manual-assign-dims">{{ assignmentSummary(a) }}</span>
            <el-tag v-if="a.generalized" size="small" type="success">已泛化</el-tag>
            <span class="li-manual-assign-side">
              <el-button size="small" @click="editAssignment(a)">编辑</el-button>
              <el-button v-if="!a.generalized" size="small" type="primary" @click="openGeneralize(a)">泛化成规则…</el-button>
              <el-button size="small" @click="doIgnore(false, a.relPath)">取消忽略</el-button>
            </span>
          </div>

          <!-- 泛化维度选择面板 -->
          <div v-if="genTarget" class="li-manual-genpanel">
            <div class="li-manual-genpanel-head">
              <span>泛化「{{ genTarget }}」的赋值：</span>
              <el-button size="small" text @click="genTarget = ''">收起</el-button>
            </div>
            <div class="li-manual-genmodes">
              <button
                v-for="mode in genModes"
                :key="mode.key"
                class="li-manual-genmode"
                :class="{ 'is-active': genMode === mode.key }"
                @click="genMode = mode.key"
              >
                <span class="li-manual-genmode-title">{{ mode.label }}</span>
                <span class="li-manual-genmode-desc">{{ mode.desc }}</span>
              </button>
            </div>
            <div class="li-manual-actions">
              <el-button type="primary" :loading="generalizing" @click="doGeneralize">生成规则并入档案</el-button>
            </div>
            <p class="li-manual-hint">泛化生成的规则来源标注为「泛化」（qna）；生成后立即增量重识别并高亮差异</p>
          </div>
        </section>

        <!-- 三、待裁决项（冲突裁决生成） -->
        <section class="li-manual-card">
          <h3 class="li-manual-card-title">待裁决项 <span class="li-manual-card-note">{{ pendingItems.length }} 项 · 双方声明对照</span></h3>
          <p v-if="!pendingItems.length" class="li-manual-empty">无声明冲突</p>
          <div v-for="(p, i) in pendingItems" :key="`${p.relPath}/${p.dimension}`" class="li-manual-pending">
            <div class="li-manual-pending-head">
              <span class="li-manual-pending-file" :title="p.relPath">{{ p.relPath }}</span>
              <span class="li-manual-pending-dim">{{ dimLabel(p.dimension) }} 冲突</span>
            </div>
            <div class="li-manual-pending-cands">
              <button
                v-for="(c, j) in p.candidates || []"
                :key="j"
                class="li-manual-cand"
                :class="{ 'is-active': pickIndex[i] === j }"
                @click="pickIndex[i] = j"
              >
                <span class="li-manual-cand-value">{{ dimLabel(c.dimension) }}={{ c.value }}</span>
                <span class="li-manual-cand-src">{{ sourceLabel(c.source) }}<template v-if="c.ruleId"> · {{ c.ruleId }}</template></span>
              </button>
            </div>
            <div class="li-manual-actions">
              <el-button type="primary" size="small" :loading="adjudicating" @click="doAdjudicatePick(p, i)">裁决采纳所选（双命中）</el-button>
              <el-button size="small" @click="toggleConditional(i)">按条件改绑（单侧缺席）…</el-button>
            </div>

            <!-- 条件改绑面板：谓词条件策略（在场/缺席各采纳一候选） -->
            <div v-if="condOpen[i]" class="li-manual-condpanel">
              <div class="li-manual-cond-row">
                <span class="li-manual-cond-label">条件谓词（何来源在何维度上的声明）：</span>
                <el-select v-model="predSource[i]" class="li-manual-cond-select" size="small">
                  <el-option v-for="s in predSources" :key="s.key" :label="s.label" :value="s.key" />
                </el-select>
                <el-select v-model="predDimension[i]" class="li-manual-cond-select" size="small">
                  <el-option v-for="d in predDims" :key="d.key" :label="d.label" :value="d.key" />
                </el-select>
              </div>
              <div class="li-manual-cond-row">
                <span class="li-manual-cond-label">谓词在场时采纳：</span>
                <button
                  v-for="(c, j) in p.candidates || []"
                  :key="`p${j}`"
                  class="li-manual-cand li-manual-cand-mini"
                  :class="{ 'is-active': presentIndex[i] === j }"
                  @click="presentIndex[i] = j"
                >{{ c.value }}</button>
              </div>
              <div class="li-manual-cond-row">
                <span class="li-manual-cond-label">谓词缺席时采纳：</span>
                <button
                  v-for="(c, j) in p.candidates || []"
                  :key="`a${j}`"
                  class="li-manual-cand li-manual-cand-mini"
                  :class="{ 'is-active': absentIndex[i] === j }"
                  @click="absentIndex[i] = j"
                >{{ c.value }}</button>
              </div>
              <div class="li-manual-actions">
                <el-button type="primary" size="small" :loading="adjudicating" @click="doAdjudicateConditional(p, i)">生成条件策略</el-button>
              </div>
              <p class="li-manual-hint">生成「在场时用甲、缺席时用乙」的最简谓词条件策略（来源=裁决）</p>
            </div>
          </div>
        </section>

        <!-- 四、验证轮差异（生成规则后的强制重识别高亮） -->
        <section class="li-manual-card">
          <h3 class="li-manual-card-title">重识别差异 <span class="li-manual-card-note">变更 {{ diffChanged.length }} · 新增 {{ diffAdded.length }} · 消失 {{ diffRemoved.length }}</span></h3>
          <p v-if="!diff || diffTotal === 0" class="li-manual-empty">暂无差异：尚未生成规则或最近一轮重识别无结论变化</p>
          <template v-else>
            <p class="li-manual-hint">每次生成规则后自动增量重识别——过窄＝差异里仍有未覆盖，过泛＝差异里出现错套，请核对</p>
            <div class="li-manual-difflist">
              <div
                v-for="entry in diffEntries"
                :key="`${entry.kind}/${entry.relPath}`"
                class="li-manual-diff-row"
                :class="`is-${entry.kind}`"
              >
                <span class="li-manual-diff-kind">{{ diffKindLabel(entry.kind) }}</span>
                <span class="li-manual-diff-path" :title="entry.relPath">{{ entry.relPath }}</span>
                <span class="li-manual-diff-cols">
                  <span class="li-manual-diff-side is-before">{{ conclusionSummary(entry.before) }}</span>
                  <span class="li-manual-diff-arrow">→</span>
                  <span class="li-manual-diff-side is-after">{{ conclusionSummary(entry.after) }}</span>
                </span>
              </div>
            </div>
          </template>
        </section>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

// 工作台事件协议通道（与插件 Go 侧 session.go 约定一致）
const TOPIC_REQUEST = 'plugin:local-import:workbench:request'
const TOPIC_RESPONSE = 'plugin:local-import:workbench:response'
const TOPIC_EVENT = 'plugin:local-import:workbench:event'

// 本插件 view 路由复合键前缀（publicId=插件 id）
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
  state: string
  pendingDims?: string[]
  clues?: { kind: string; detail: string }[]
  assigned?: boolean
}

interface SnapPending {
  relPath: string
  fileName: string
  dimension: string
  reason: string
  candidates: SnapDecl[]
}

interface AssignmentView {
  relPath: string
  author?: string
  authorRole?: string
  title?: string
  tags?: string[]
  collection?: string
  generalized?: boolean
  updatedAtMs?: number
}

interface DecisionsView {
  assignments?: AssignmentView[]
  ignored?: string[]
  selection?: string[]
}

interface DiffConclusion {
  state: string
  final?: SnapDecl[]
  pendingDims?: string[]
}

interface DiffEntry {
  relPath: string
  before?: DiffConclusion
  after?: DiffConclusion
}

interface SnapDiff {
  changed: DiffEntry[]
  added: DiffEntry[]
  removed: DiffEntry[]
}

interface GeneratedRuleView {
  id: string
  description: string
  source: string
  pattern: string
  condition?: string
}

interface WorkbenchResponse {
  seq: number
  ok: boolean
  error?: string
  session?: SessionSummary
  panorama?: { files?: SnapFile[]; pending?: SnapPending[]; stats?: { totalFiles: number } }
  decisions?: DecisionsView
  diff?: SnapDiff
  assignment?: AssignmentView | null
  rule?: GeneratedRuleView
  started?: boolean
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
const loadError = ref('')
const snapshot = ref<{ pending?: SnapPending[] } | null>(null)
const decisions = ref<DecisionsView | null>(null)
const diff = ref<SnapDiff | null>(null)
const notice = ref('')
const actionError = ref('')
const recognizing = ref(false)
// 最近一次条件策略生成的条件 JSON（档案 v3 表达式树，生成物透出）
const lastCondition = ref('')

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

// 带序号配对的请求
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

function panoramaRoute(): string {
  const root = session.value ? session.value.rootPath : rootFromHash()
  if (!root) return `${ROUTE_BASE}workbench-panorama`
  return `${ROUTE_BASE}workbench-panorama?root=${encodeURIComponent(root)}`
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
  return ''
}

// 拉取手动解析所需数据（getPanorama files 分节）：待裁决清单 + 决策态 + 差异集
// + 待解析候选（未识别与有线索态文件）；候选清单单独一份便于扩展分页
async function loadParseData(root: string): Promise<void> {
  const resp = await request('getPanorama', { root: root, section: 'files' })
  if (!resp.ok) {
    if (resp.error && resp.error.indexOf('尚未完成') < 0) {
      actionError.value = resp.error
    }
    return
  }
  const pano = resp.panorama
  snapshot.value = { pending: (pano && pano.pending) || [] }
  decisions.value = resp.decisions || null
  diff.value = resp.diff || null
  allFilesRef.value = ((pano && pano.files) || []).filter((f) => f.state !== 'identified')
}

// —— 一、单次赋值 ——

const CANDIDATE_CAP = 200
const filterText = ref('')
const formRel = ref('')
const formAuthor = ref('')
const formAuthorRole = ref('')
const formTitle = ref('')
const formTagsText = ref('')
const formCollection = ref('')
const assigning = ref(false)

// 待解析候选清单（仅未识别与有线索态；由 getPanorama files 分节填充）
const allFilesRef = ref<SnapFile[]>([])

const candidateFiles = computed<SnapFile[]>(() => {
  const files = allFilesRef.value
  const text = filterText.value.trim().toLowerCase()
  const filtered = text
    ? files.filter((f) => f.relPath.toLowerCase().indexOf(text) >= 0)
    : files
  return filtered.slice(0, CANDIDATE_CAP)
})

function pickCandidate(f: SnapFile): void {
  formRel.value = f.relPath
}

function resetForm(): void {
  formAuthor.value = ''
  formAuthorRole.value = ''
  formTitle.value = ''
  formTagsText.value = ''
  formCollection.value = ''
}

async function doAssign(): Promise<void> {
  if (!session.value || !formRel.value.trim()) {
    actionError.value = '请先选定或输入目标文件的相对路径'
    return
  }
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  const tags = formTagsText.value
    .split(/[,，、]/)
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
  const resp = await request('assign', {
    root: session.value.rootPath,
    relPath: formRel.value.trim(),
    author: formAuthor.value.trim(),
    authorRole: formAuthorRole.value.trim(),
    title: formTitle.value.trim(),
    tags: tags,
    collection: formCollection.value.trim()
  })
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '保存赋值失败'
    return
  }
  notice.value = `已保存赋值：${formRel.value.trim()}（重识别后生效，可在识别全景查看覆盖结果）`
  resetForm()
  await reloadAll()
}

async function doClearAssign(): Promise<void> {
  if (!session.value || !formRel.value.trim()) {
    actionError.value = '请先选定或输入目标文件的相对路径'
    return
  }
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await request('clearAssignment', {
    root: session.value.rootPath,
    relPath: formRel.value.trim()
  })
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '清除赋值失败'
    return
  }
  notice.value = `已清除赋值：${formRel.value.trim()}`
  await reloadAll()
}

async function doIgnore(ignore: boolean, relPath?: string): Promise<void> {
  const target = relPath || formRel.value.trim()
  if (!session.value || !target) {
    actionError.value = '请先选定或输入目标文件的相对路径'
    return
  }
  const resp = await request('setIgnored', {
    root: session.value.rootPath,
    relPath: target,
    ignored: ignore
  })
  if (!resp.ok) {
    actionError.value = resp.error || '忽略项登记失败'
    return
  }
  notice.value = ignore ? `已忽略：${target}` : `已取消忽略：${target}`
  await reloadAll()
}

// —— 二、已有赋值与泛化 ——

const assignments = computed<AssignmentView[]>(() => (decisions.value && decisions.value.assignments) || [])
const ignoredCount = computed(() => (decisions.value && decisions.value.ignored ? decisions.value.ignored.length : 0))

const genModes = [
  { key: 'filename', label: '按文件名形态', desc: '赋值值位升格为捕获组（如方括号作者）' },
  { key: 'dirLayer', label: '按目录层', desc: '父目录名锚定目录含义规则' },
  { key: 'sequence', label: '按序列形态', desc: '所属序列群前缀锚定定群规则' }
]
const genTarget = ref('')
const genMode = ref('filename')
const generalizing = ref(false)

function openGeneralize(a: AssignmentView): void {
  genTarget.value = a.relPath
  genMode.value = 'filename'
  notice.value = ''
  actionError.value = ''
}

function editAssignment(a: AssignmentView): void {
  formRel.value = a.relPath
  formAuthor.value = a.author || ''
  formAuthorRole.value = a.authorRole || ''
  formTitle.value = a.title || ''
  formTagsText.value = (a.tags || []).join('，')
  formCollection.value = a.collection || ''
}

async function doGeneralize(): Promise<void> {
  if (!session.value || !genTarget.value) return
  generalizing.value = true
  notice.value = ''
  actionError.value = ''
  lastCondition.value = ''
  const resp = await request('generalize', {
    root: session.value.rootPath,
    relPath: genTarget.value,
    generalizeMode: genMode.value
  })
  generalizing.value = false
  if (!resp.ok || !resp.rule) {
    actionError.value = resp.error || '泛化生成失败'
    return
  }
  notice.value = `已生成规则 ${resp.rule.id}（${resp.rule.description}）；验证轮重识别已启动，差异见下方`
  genTarget.value = ''
  recognizing.value = true
}

// —— 三、待裁决与条件改绑 ——

const pendingItems = computed<SnapPending[]>(() => (snapshot.value && snapshot.value.pending) || [])
const adjudicating = ref(false)

// 逐待裁决项的裁决选择状态（下标键控）
const pickIndex = ref<Record<number, number>>({})
const condOpen = ref<Record<number, boolean>>({})
const predSource = ref<Record<number, string>>({})
const predDimension = ref<Record<number, string>>({})
const presentIndex = ref<Record<number, number>>({})
const absentIndex = ref<Record<number, number>>({})

const predSources = [
  { key: 'dir', label: '目录源' },
  { key: 'filename', label: '文件名源' },
  { key: 'predicate', label: '库内命中源' },
  { key: 'group', label: '群组前缀源' },
  { key: 'constant', label: '常量' }
]
const predDims = [
  { key: 'author', label: '作者' },
  { key: 'title', label: '作品名' },
  { key: 'tag', label: '标签' },
  { key: 'collection', label: '作品集' }
]

function toggleConditional(i: number): void {
  condOpen.value[i] = !condOpen.value[i]
  if (condOpen.value[i]) {
    // 缺省：谓词=目录源作者、在场=首个候选、缺席=次候选
    if (!predSource.value[i]) predSource.value[i] = 'dir'
    if (!predDimension.value[i]) predDimension.value[i] = 'author'
    if (presentIndex.value[i] === undefined) presentIndex.value[i] = 0
    if (absentIndex.value[i] === undefined) absentIndex.value[i] = 1
  }
  notice.value = ''
  actionError.value = ''
  lastCondition.value = ''
}

async function doAdjudicatePick(p: SnapPending, i: number): Promise<void> {
  if (!session.value) return
  if (pickIndex.value[i] === undefined) {
    actionError.value = '请先点选要采纳的候选'
    return
  }
  adjudicating.value = true
  notice.value = ''
  actionError.value = ''
  lastCondition.value = ''
  const resp = await request('adjudicate', {
    root: session.value.rootPath,
    relPath: p.relPath,
    dimension: p.dimension,
    adjudicateMode: 'pick',
    pickIndex: pickIndex.value[i]
  })
  adjudicating.value = false
  if (!resp.ok || !resp.rule) {
    actionError.value = resp.error || '裁决生成失败'
    return
  }
  notice.value = `已生成组合规则 ${resp.rule.id}（${resp.rule.pattern}）；验证轮重识别已启动，差异见下方`
  recognizing.value = true
}

async function doAdjudicateConditional(p: SnapPending, i: number): Promise<void> {
  if (!session.value) return
  adjudicating.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await request('adjudicate', {
    root: session.value.rootPath,
    relPath: p.relPath,
    dimension: p.dimension,
    adjudicateMode: 'conditional',
    predicateSource: predSource.value[i] || 'dir',
    predicateDimension: predDimension.value[i] || 'author',
    presentIndex: presentIndex.value[i] || 0,
    absentIndex: absentIndex.value[i] === undefined ? 1 : absentIndex.value[i]
  })
  adjudicating.value = false
  if (!resp.ok || !resp.rule) {
    actionError.value = resp.error || '条件策略生成失败'
    return
  }
  notice.value = `已生成条件策略 ${resp.rule.id}（${resp.rule.description}）；验证轮重识别已启动，差异见下方`
  lastCondition.value = resp.rule.condition || ''
  condOpen.value[i] = false
  recognizing.value = true
}

// —— 四、验证轮差异 ——

const diffChanged = computed<DiffEntry[]>(() => (diff.value && diff.value.changed) || [])
const diffAdded = computed<DiffEntry[]>(() => (diff.value && diff.value.added) || [])
const diffRemoved = computed<DiffEntry[]>(() => (diff.value && diff.value.removed) || [])
const diffTotal = computed(() => diffChanged.value.length + diffAdded.value.length + diffRemoved.value.length)

const diffEntries = computed<{ kind: string; relPath: string; before?: DiffConclusion; after?: DiffConclusion }[]>(() => [
  ...diffChanged.value.map((e) => ({ kind: 'changed', ...e })),
  ...diffAdded.value.map((e) => ({ kind: 'added', ...e })),
  ...diffRemoved.value.map((e) => ({ kind: 'removed', ...e }))
])

const diffKindLabel = (kind: string): string => {
  switch (kind) {
    case 'changed': return 'Δ变更'
    case 'added': return '＋新增'
    case 'removed': return '－消失'
    default: return kind
  }
}

const conclusionSummary = (c?: DiffConclusion | null): string => {
  if (!c) return '（无）'
  const parts: string[] = []
  for (const d of c.final || []) parts.push(`${dimLabel(d.dimension)}=${d.value}`)
  if (c.pendingDims && c.pendingDims.length) parts.push(`待裁决:${c.pendingDims.join('/')}`)
  const state = stateLabel(c.state)
  return parts.length ? `${state}（${parts.join('、')}）` : state
}

// —— 呈现辅助 ——

const stateLabel = (state: string): string => {
  switch (state) {
    case 'identified': return '已识别'
    case 'clueful': return '有线索'
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
    default: return source
  }
}
const assignmentSummary = (a: AssignmentView): string => {
  const parts: string[] = []
  if (a.author) parts.push(`作者=${a.author}${a.authorRole ? `（${a.authorRole}）` : ''}`)
  if (a.title) parts.push(`作品名=${a.title}`)
  if (a.tags && a.tags.length) parts.push(`标签=${a.tags.join('、')}`)
  if (a.collection) parts.push(`作品集=${a.collection}`)
  return parts.join('、') || '（空）'
}

async function reloadAll(): Promise<void> {
  if (!session.value) return
  await loadParseData(session.value.rootPath)
}

// 识别事件：验证轮完成后自动刷新（含差异集）
function onEvent(event: { data?: unknown }) {
  const evt = decodePayload<{ kind?: string; root?: string; success?: boolean; message?: string }>(event && event.data)
  if (!evt) return
  const root = session.value ? session.value.rootPath : ''
  if (evt.root && root && evt.root !== root) return
  if (evt.kind === 'recognitionProgress') {
    recognizing.value = true
  } else if (evt.kind === 'recognitionDone') {
    recognizing.value = false
    if (evt.success) {
      void reloadAll()
    } else {
      actionError.value = evt.message || '验证轮重识别失败'
    }
  }
}

onMounted(async () => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  offEvent = Events.On(TOPIC_EVENT, onEvent)
  const root = await restoreSession()
  if (root) {
    await loadParseData(root)
  }
})

onUnmounted(() => {
  if (offResponse) offResponse()
  if (offEvent) offEvent()
})
</script>

<style scoped>
.li-manual {
  max-width: 880px;
  margin: 0 auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.li-manual-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  flex-wrap: wrap;
}

.li-manual-title {
  margin: 0;
  font-size: 20px;
  color: var(--app-text-primary);
}

.li-manual-root {
  font-size: 13px;
  color: var(--app-text-secondary);
}

.li-manual-banner {
  border-radius: var(--app-radius);
}

.li-manual-back {
  font-size: 13px;
  color: var(--app-color-primary);
}

.li-manual-busy {
  font-size: 13px;
  padding: 6px 12px;
  border-radius: var(--app-radius);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-manual-card {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.li-manual-card-title {
  margin: 0;
  font-size: 15px;
  color: var(--app-text-primary);
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

.li-manual-card-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-manual-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

.li-manual-empty {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-regular);
}

.li-manual-picker {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-manual-picker-target {
  font-size: 12px;
  color: var(--app-color-primary);
}

.li-manual-candidates {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 220px;
  overflow-y: auto;
}

.li-manual-candidate {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  background: transparent;
  cursor: pointer;
  text-align: left;
  font-size: 12px;
}

.li-manual-candidate:hover {
  border-color: var(--app-color-primary);
}

.li-manual-candidate.is-active {
  border-color: var(--app-color-primary);
  background: var(--app-tag-neutral-bg);
}

.li-manual-candidate-state {
  flex: none;
  width: 14px;
  text-align: center;
}

.li-manual-candidate-state.is-identified {
  color: var(--app-color-success);
}

.li-manual-candidate-state.is-clueful {
  color: var(--app-color-warning);
}

.li-manual-candidate-state.is-plain {
  color: var(--app-text-secondary);
}

.li-manual-candidate-path {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-primary);
}

.li-manual-candidate-flag {
  flex: none;
  font-size: 11px;
  padding: 0 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-manual-form {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
}

.li-manual-form-wide {
  grid-column: 1 / -1;
}

.li-manual-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-manual-assign-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid var(--app-border-color-light);
  font-size: 12px;
  flex-wrap: wrap;
}

.li-manual-assign-row:last-of-type {
  border-bottom: none;
}

.li-manual-assign-path {
  max-width: 34%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-primary);
}

.li-manual-assign-dims {
  flex: 1;
  color: var(--app-text-regular);
}

.li-manual-assign-side {
  display: flex;
  gap: 6px;
  align-items: center;
}

.li-manual-genpanel {
  border: 1px dashed var(--app-border-color-light);
  border-radius: var(--app-radius);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.li-manual-genpanel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-manual-genmodes {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
}

.li-manual-genmode {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.li-manual-genmode:hover {
  border-color: var(--app-color-primary);
}

.li-manual-genmode.is-active {
  border-color: var(--app-color-primary);
  background: var(--app-tag-neutral-bg);
}

.li-manual-genmode-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-manual-genmode-desc {
  font-size: 11px;
  line-height: 1.5;
  color: var(--app-text-secondary);
}

.li-manual-pending {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.li-manual-pending-head {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-manual-pending-file {
  font-size: 12px;
  color: var(--app-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 60%;
}

.li-manual-pending-dim {
  font-size: 11px;
  padding: 0 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-manual-pending-cands {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.li-manual-cand {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 12px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.li-manual-cand:hover {
  border-color: var(--app-color-primary);
}

.li-manual-cand.is-active {
  border-color: var(--app-color-primary);
  background: var(--app-tag-neutral-bg);
}

.li-manual-cand-mini {
  flex-direction: row;
  align-items: center;
  padding: 2px 10px;
  font-size: 12px;
}

.li-manual-cand-value {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-manual-cand-src {
  font-size: 11px;
  color: var(--app-text-secondary);
}

.li-manual-condpanel {
  border-top: 1px dashed var(--app-border-color-light);
  padding-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.li-manual-condjson {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-manual-condjson-head {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-manual-condjson-body {
  margin: 0;
  font-family: var(--app-font-mono, monospace);
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-primary);
  white-space: pre-wrap;
  word-break: break-all;
}

.li-manual-cond-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-manual-cond-label {
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-manual-cond-select {
  width: 140px;
}

.li-manual-difflist {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 320px;
  overflow-y: auto;
}

.li-manual-diff-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 8px;
  border-radius: var(--app-radius-sm);
  font-size: 12px;
  flex-wrap: wrap;
}

.li-manual-diff-row.is-changed {
  background: var(--app-tag-warning-bg);
}

.li-manual-diff-row.is-added {
  background: var(--app-tag-success-bg);
}

.li-manual-diff-row.is-removed {
  background: var(--app-tag-danger-bg);
}

.li-manual-diff-kind {
  flex: none;
  font-weight: 600;
}

.li-manual-diff-row.is-changed .li-manual-diff-kind {
  color: var(--app-tag-warning-text);
}

.li-manual-diff-row.is-added .li-manual-diff-kind {
  color: var(--app-tag-success-text);
}

.li-manual-diff-row.is-removed .li-manual-diff-kind {
  color: var(--app-tag-danger-text);
}

.li-manual-diff-path {
  flex: none;
  max-width: 32%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-primary);
}

.li-manual-diff-cols {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 240px;
  flex-wrap: wrap;
}

.li-manual-diff-side {
  color: var(--app-text-regular);
}

.li-manual-diff-side.is-after {
  color: var(--app-text-primary);
  font-weight: 600;
}

.li-manual-diff-arrow {
  color: var(--app-text-secondary);
}
</style>
