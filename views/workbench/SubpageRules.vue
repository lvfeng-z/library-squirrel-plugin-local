<template>
  <div class="li-rules">
    <header class="li-rules-header">
      <h2 class="li-rules-title">规则管理</h2>
      <span v-if="session" class="li-rules-root">{{ session.rootPath }}</span>
      <span class="li-rules-header-side">
        <span class="li-rules-note">规则档案全库单份 · 编辑事务单点串行</span>
        <a class="li-rules-back" :href="parseRoute()">返回解析视图</a>
      </span>
    </header>

    <el-alert
      v-if="loadError"
      class="li-rules-banner"
      type="error"
      :closable="false"
      show-icon
      :title="loadError"
    >
      <a class="li-rules-back" :href="`${ROUTE_BASE}workbench`">返回本地导入工作台</a>
    </el-alert>
    <el-empty
      v-else-if="!session"
      description="未找到进行中的解析会话：请从工作台新建或恢复会话"
    >
      <a class="li-rules-back" :href="`${ROUTE_BASE}workbench`">返回本地导入工作台</a>
    </el-empty>

    <template v-else>
      <!-- 操作反馈区 -->
      <el-alert v-if="notice" class="li-rules-banner" type="success" :closable="false" show-icon :title="notice" />
      <el-alert v-if="actionError" class="li-rules-banner" type="error" :closable="false" show-icon :title="actionError" />

      <!-- 档案载入告警：坏条目被跳过等（核对档案的透出入口） -->
      <el-alert
        v-if="warnings.length"
        class="li-rules-banner"
        type="warning"
        :closable="false"
        show-icon
        :title="`档案载入告警 ${warnings.length} 条（对应规则已被跳过、不参与求值）`"
      >
        <ul class="li-rules-warnlist">
          <li v-for="(w, i) in warnings" :key="i">{{ w }}</li>
        </ul>
      </el-alert>

      <!-- 一、档案规则列表 -->
      <section class="li-rules-card">
        <div class="li-rules-card-head">
          <h3 class="li-rules-card-title">档案规则 <span class="li-rules-card-note">{{ rules.length }} 条</span></h3>
          <el-button type="primary" size="small" @click="openNew">新建规则…</el-button>
        </div>
        <p v-if="!rules.length" class="li-rules-empty">档案为空：可从手动解析页泛化生成，或在此新建手写规则</p>
        <div v-for="r in rules" :key="r.id" class="li-rules-row">
          <el-switch
            size="small"
            :model-value="r.enabled"
            :loading="acting"
            @change="doToggleRule(r, $event === true)"
          />
          <div class="li-rules-row-main">
            <div class="li-rules-row-desc" :title="r.description">{{ r.description || '（无描述）' }}</div>
            <div class="li-rules-row-id" :title="r.id">{{ r.id }}</div>
          </div>
          <el-tag size="small" :type="sourceTagType(r.source)">{{ sourceLabel(r.source) }}</el-tag>
          <span class="li-rules-row-side">
            <el-button size="small" @click="openEdit(r)">编辑</el-button>
            <el-button size="small" type="danger" @click="doDeleteRule(r)">删除</el-button>
          </span>
        </div>
        <p class="li-rules-hint">删除立即覆写档案（写前自动备份一代，误删可在下方「档案备份」回退）</p>
      </section>

      <!-- 二、单条编辑（两段式弹窗 · 剧本F）：列表行「编辑」/「新建规则…」挂共用
           组件 RuleEditDialog——上半=识别规则、下半=提取规则；旧内联 JSON 表单与
           「仅序列成员产出」开关由弹窗取代、不并存（开关化为提取段逐行条件勾选） -->
      <RuleEditDialog
        v-model:state="dialogVisible"
        mode="rule"
        :rule="dialogRule"
        :saving="savingRule"
        :save-error="dialogSaveError"
        @save="onDialogSave"
      />

      <!-- 三、例子模板（v4 词条写法参考——内置规则已退休，不参与求值不入档） -->
      <section class="li-rules-card">
        <div class="li-rules-card-head">
          <h3 class="li-rules-card-title">例子模板 <span class="li-rules-card-note">{{ examples.length }} 词条 · 格式写法参考</span></h3>
        </div>
        <p class="li-rules-hint">
          例子模板是 v4 条件格式的写法参考（不参与求值、不入档案）：新建/编辑规则时在格式步点词条区同名 chips
          可一步预填；占位词词表为封闭集（作者/集名/序号/标题/标签/作品号/日期/键/任意）
        </p>
        <div v-for="ex in examples" :key="ex.name" class="li-rules-row">
          <div class="li-rules-row-main">
            <div class="li-rules-row-desc" :title="ex.description">{{ ex.name }}</div>
            <div class="li-rules-row-id">{{ ex.template }}</div>
          </div>
          <el-tag size="small" type="info">{{ targetLabel(ex.target) }}</el-tag>
        </div>
      </section>

      <!-- 四、档案备份（写前单代备份 · 回退一代） -->
      <section class="li-rules-card">
        <div class="li-rules-card-head">
          <h3 class="li-rules-card-title">档案备份 <span class="li-rules-card-note">单代 · 每次覆写前自动留存</span></h3>
          <el-button size="small" :disabled="!backupAvailable || acting" @click="doRollback">回退一代</el-button>
        </div>
        <p class="li-rules-hint">
          每次覆写档案前，现行档案自动存入单代备份键（rule-profile.backup）——误删规则、禁用错内置或覆写中断均可回退一代；
          回退时当前档案换位进备份（再点一次即撤销回退）。回退后既有识别会话会在恢复时提示档案已变更，建议重新识别。
        </p>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'
import RuleEditDialog from './RuleEditDialog.vue'

// 工作台事件协议通道（与插件 Go 侧 session.go 约定一致）
const TOPIC_REQUEST = 'plugin:local-import:workbench:request'
const TOPIC_RESPONSE = 'plugin:local-import:workbench:response'

// 本插件 view 路由复合键前缀（publicId=插件 id）
const ROUTE_BASE = '#/com.lvfeng.localImport/'

interface SessionSummary {
  rootPath: string
  state: string
  createdAtMs: number
  updatedAtMs: number
}

interface RoleBindingView {
  source: string
  roleName: string
}

interface DimPolicyView {
  dimension: string
  strategy: string
}

// 档案规则条目（ruleEntryJSON 形态：conditions / extractions 即 JSON 片段源——
// v4 档案条件只落 conditions，模式树为编译缓存不落档）
interface RuleEntryView {
  id: string
  description: string
  source: string
  enabled: boolean
  conditions?: unknown
  extractions?: unknown
  synthesis?: { dimPolicies?: DimPolicyView[] | null; roles?: RoleBindingView[] | null }
}

// 例子模板词条（v4 写法参考，不参与求值不入档）。
interface ExampleTemplateView {
  name: string
  template: string
  target: string
  description: string
}

interface RulesAdminView {
  schemaVersion: number
  rules: RuleEntryView[]
  warnings?: string[]
  examples?: ExampleTemplateView[]
  backupAvailable: boolean
}

interface WorkbenchResponse {
  seq: number
  ok: boolean
  error?: string
  session?: SessionSummary
  rulesView?: RulesAdminView
  rolledBack?: boolean
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
const notice = ref('')
const actionError = ref('')
const acting = ref(false)

const rules = ref<RuleEntryView[]>([])
const examples = ref<ExampleTemplateView[]>([])
const warnings = ref<string[]>([])
const backupAvailable = ref(false)

function targetLabel(target: string): string {
  if (target === 'dir') return '目录'
  if (target === 'seq') return '序列'
  return '文件'
}

let seq = 0
let offResponse: (() => void) | null = null
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

// 进入时地址携 rule 查询参数（解析视图声明来源「直达编辑」的携参形态）：取走参数
// 并剥离——历史回退/刷新回到本页时不再重复触发编辑器打开（与工作台页 root 参数
// 同款取走即剥离语义；其余参数如 root 原样保留）
function consumeRuleParam(): string {
  const hash = location.hash || ''
  const queryIndex = hash.indexOf('?')
  if (queryIndex < 0) return ''
  const params = new URLSearchParams(hash.slice(queryIndex + 1))
  const rule = params.get('rule') || ''
  if (!rule) return ''
  params.delete('rule')
  const rest = params.toString()
  // 保留 vue-router 写入的 history.state（页面位置簿记），只改写 URL 剥掉该参数
  history.replaceState(history.state, '', `${location.pathname}${location.search}${hash.slice(0, queryIndex)}${rest ? `?${rest}` : ''}`)
  return rule
}

function parseRoute(): string {
  const root = session.value ? session.value.rootPath : rootFromHash()
  if (!root) return `${ROUTE_BASE}workbench-parse`
  return `${ROUTE_BASE}workbench-parse?root=${encodeURIComponent(root)}`
}

async function restoreSession(): Promise<boolean> {
  const root = rootFromHash()
  if (!root) return true
  const resp = await request('activateSession', { root: root })
  if (resp.ok && resp.session) {
    session.value = resp.session
    return true
  }
  loadError.value = resp.error || '恢复解析会话失败'
  return false
}

// 规则管理视图整份套用（每次动作后应答都带回新视图）
function applyRulesView(view: RulesAdminView | undefined | null): void {
  if (!view) return
  rules.value = view.rules || []
  examples.value = view.examples || []
  warnings.value = view.warnings || []
  backupAvailable.value = view.backupAvailable === true
}

async function loadRules(): Promise<void> {
  const resp = await request('getRules')
  if (!resp.ok) {
    actionError.value = resp.error || '读取规则档案失败'
    return
  }
  applyRulesView(resp.rulesView)
}

// 携规则 ID 进入（解析视图声明来源直达编辑）：规则档案载入后按 ID 打开该规则
// 编辑态（与列表行「编辑」按钮同一路径）；找不到时不阻断列表呈现，给出提示
function openRuleById(ruleId: string): void {
  const rule = rules.value.find((r) => r.id === ruleId)
  if (!rule) {
    actionError.value = `未在规则档案中找到规则「${ruleId}」（可能已被删除）`
    return
  }
  openEdit(rule)
}

// —— 一、列表操作：启停 / 删除 ——

async function doToggleRule(rule: RuleEntryView, enable: boolean): Promise<void> {
  acting.value = true
  actionError.value = ''
  const resp = await request('setRuleEnabled', { ruleId: rule.id, enabled: enable })
  acting.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '启停规则失败'
    return
  }
  applyRulesView(resp.rulesView)
  notice.value = `规则「${rule.description || rule.id}」已${enable ? '启用' : '停用'}`
}

async function doDeleteRule(rule: RuleEntryView): Promise<void> {
  acting.value = true
  actionError.value = ''
  const resp = await request('deleteRule', { ruleId: rule.id })
  acting.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '删除规则失败'
    return
  }
  applyRulesView(resp.rulesView)
  notice.value = `规则「${rule.description || rule.id}」已删除（可在档案备份回退一代找回）`
}

// —— 二、单条编辑（两段式弹窗 · 剧本F，共用组件 RuleEditDialog） ——

// 弹窗初始载荷（RuleEditDialog rule 模式入参的结构投影；conditions/extractions
// 即档案条目的 JSON 片段原文，由弹窗内结构化/JSON 双模编辑）
interface RuleDialogInput {
  id: string
  description: string
  source: string
  enabled: boolean
  isNew: boolean
  conditions?: unknown
  extractions: unknown
  synthesis: { dimPolicies: DimPolicyView[]; roles: RoleBindingView[] }
}

const dialogVisible = ref(false)
const dialogRule = ref<RuleDialogInput | null>(null)
const savingRule = ref(false)
// 保存失败文案（后端构造期校验拒绝等）：回填弹窗内呈现、不关窗
const dialogSaveError = ref('')

const sourceLabels: Record<string, string> = {
  builtin: '内置',
  manual: '手写',
  qna: '泛化',
  adjudication: '裁决'
}

function sourceLabel(source: string): string {
  return sourceLabels[source] || source
}

function sourceTagType(source: string): 'info' | 'success' | 'warning' | 'danger' {
  if (source === 'manual') return 'success'
  if (source === 'qna') return 'warning'
  if (source === 'adjudication') return 'danger'
  return 'info'
}

// 新建：空载荷入弹窗（v4 缺省样例〔【{作者}】{标题} 行 + 作者/标题提取行〕
// 由弹窗侧缺省带入）
function openNew(): void {
  dialogRule.value = {
    id: '',
    description: '',
    source: 'manual',
    enabled: true,
    isNew: true,
    conditions: null,
    extractions: null,
    synthesis: { dimPolicies: [], roles: [] }
  }
  dialogSaveError.value = ''
  dialogVisible.value = true
  actionError.value = ''
}

function openEdit(rule: RuleEntryView): void {
  const synth = rule.synthesis || {}
  dialogRule.value = {
    id: rule.id,
    description: rule.description,
    source: rule.source,
    enabled: rule.enabled,
    isNew: false,
    conditions: rule.conditions ?? null,
    extractions: rule.extractions ?? [],
    synthesis: {
      dimPolicies: ((synth.dimPolicies || []) as DimPolicyView[]).map((d) => ({ dimension: d.dimension, strategy: d.strategy })),
      roles: ((synth.roles || []) as RoleBindingView[]).map((r) => ({ source: r.source, roleName: r.roleName }))
    }
  }
  dialogSaveError.value = ''
  dialogVisible.value = true
  actionError.value = ''
}

// 弹窗保存（载荷已过弹窗侧行级校验——捕获组引用/JSON 合法）：走既有 saveRule
// RPC（构造期校验兜底）；失败文案回填弹窗不关窗，成功套用新视图并关窗
async function onDialogSave(payload: Record<string, unknown>): Promise<void> {
  savingRule.value = true
  dialogSaveError.value = ''
  const resp = await request('saveRule', { rule: payload })
  savingRule.value = false
  if (!resp.ok) {
    dialogSaveError.value = resp.error || '保存规则失败'
    return
  }
  applyRulesView(resp.rulesView)
  const ruleId = typeof payload.id === 'string' ? payload.id : ''
  notice.value = ruleId
    ? `规则「${ruleId}」已保存（可在识别全景页重新识别查看效果）`
    : '规则已新建并入档案（可在识别全景页重新识别查看效果）'
  dialogVisible.value = false
}

// —— 三、内置启停已随 v4 退休（例子模板为纯参考词条，无启停） ——

// —— 四、档案备份回退一代 ——

async function doRollback(): Promise<void> {
  acting.value = true
  actionError.value = ''
  const resp = await request('rollbackProfile')
  acting.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '档案回退失败'
    return
  }
  applyRulesView(resp.rulesView)
  notice.value = '档案已回退一代（当前档案换位进备份，再点一次即撤销回退）'
}

onMounted(() => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  // 取走即剥离：必须在任何导航发生前读取（进入时地址即携参形态）
  const ruleParam = consumeRuleParam()
  void (async () => {
    if (!(await restoreSession())) return
    await loadRules()
    if (ruleParam) openRuleById(ruleParam)
  })()
})

onUnmounted(() => {
  if (offResponse) offResponse()
})
</script>

<style scoped>
.li-rules {
  max-width: 860px;
  margin: 0 auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.li-rules-header {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.li-rules-title {
  margin: 0;
  font-size: 20px;
  color: var(--app-text-primary);
}

.li-rules-root {
  font-size: 13px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-rules-header-side {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 10px;
}

.li-rules-note {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-rules-back {
  font-size: 13px;
  color: var(--app-color-primary);
}

.li-rules-banner {
  width: 100%;
}

.li-rules-warnlist {
  margin: 4px 0 0;
  padding-left: 18px;
  font-size: 12px;
  line-height: 1.8;
  word-break: break-all;
}

.li-rules-card {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.li-rules-card-head {
  display: flex;
  align-items: center;
  gap: 10px;
}

.li-rules-card-title {
  margin: 0;
  font-size: 15px;
  color: var(--app-text-primary);
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-rules-card-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-rules-card-head .li-rules-card-title {
  margin-right: auto;
}

.li-rules-empty {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-regular);
}

.li-rules-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-text-secondary);
}

.li-rules-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 0;
  border-bottom: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
}

.li-rules-row:last-of-type {
  border-bottom: none;
}

.li-rules-row-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-rules-row-desc {
  font-size: 13px;
  color: var(--app-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.li-rules-row-id {
  font-size: 12px;
  color: var(--app-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.li-rules-row-side {
  display: flex;
  gap: 6px;
}

</style>
