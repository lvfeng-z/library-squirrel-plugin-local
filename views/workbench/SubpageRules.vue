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

      <!-- 二、单条编辑（表单化常用字段 + JSON 片段） -->
      <section v-if="editOpen" class="li-rules-card li-rules-editor">
        <div class="li-rules-card-head">
          <h3 class="li-rules-card-title">
            {{ editIsNew ? '新建规则' : '编辑规则' }}
            <span v-if="!editIsNew" class="li-rules-card-note">{{ editId }} · 来源：{{ sourceLabel(editSource) }}（不可改）</span>
            <span v-else class="li-rules-card-note">来源：手写（manual）· 保存时自动生成 ID</span>
          </h3>
          <el-button size="small" text @click="closeEdit">收起</el-button>
        </div>

        <!-- 捕获组约束提示：漏声明 captures → 规则命中零声明 -->
        <el-alert
          class="li-rules-banner"
          type="info"
          :closable="false"
          show-icon
          title="捕获组约束：提取动作与条件引用的捕获组名，必须在模式 JSON 的 regex 节点 captures 数组内显式声明，且该 regex 的 target 须为 filename / relpath（dirname 目标与目录层 location.ancestor 子树的捕获组不绑定）——漏声明的引用不会产出任何声明（规则照常命中但零声明），保存时会被拒绝。"
        />

        <div class="li-rules-form">
          <el-input v-model="editDescription" placeholder="规则描述（人话，列表呈现用）" clearable />
          <div class="li-rules-switch-line">
            <el-switch v-model="editEnabled" active-text="启用" inactive-text="停用" />
          </div>
        </div>

        <!-- 合成策略：角色绑定 + 维度策略（表单化） -->
        <div class="li-rules-subform">
          <div class="li-rules-subform-head">
            <span>合成策略 · 角色绑定（来自该来源的声明一律附加角色）</span>
            <el-button size="small" text @click="addRole">加一行</el-button>
          </div>
          <div v-for="(role, i) in editRoles" :key="`role${i}`" class="li-rules-inline-row">
            <el-select v-model="role.source" size="small" placeholder="来源">
              <el-option v-for="s in sourceKinds" :key="s.key" :label="s.label" :value="s.key" />
            </el-select>
            <el-input v-model="role.roleName" size="small" placeholder="角色名（如 原画）" />
            <el-button size="small" text type="danger" @click="editRoles.splice(i, 1)">删</el-button>
          </div>
          <div class="li-rules-subform-head">
            <span>合成策略 · 维度策略（同维度多来源候选的合成方式）</span>
            <el-button size="small" text @click="addDim">加一行</el-button>
          </div>
          <div v-for="(dim, i) in editDims" :key="`dim${i}`" class="li-rules-inline-row">
            <el-select v-model="dim.dimension" size="small" placeholder="维度">
              <el-option v-for="d in dimensions" :key="d.key" :label="d.label" :value="d.key" />
            </el-select>
            <el-select v-model="dim.strategy" size="small" placeholder="策略">
              <el-option v-for="s in strategies" :key="s.key" :label="s.label" :value="s.key" />
            </el-select>
            <el-button size="small" text type="danger" @click="editDims.splice(i, 1)">删</el-button>
          </div>
        </div>

        <!-- 模式与提取动作：JSON 片段编辑 -->
        <div class="li-rules-json">
          <label class="li-rules-json-label">模式 pattern（JSON 对象：regex / location / sibling / predicate / and / or / not）</label>
          <el-input
            v-model="editPatternText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 14 }"
            class="li-rules-json-area"
            spellcheck="false"
          />
          <label class="li-rules-json-label">提取动作 extractions（JSON 数组：dimension / from / captureName / constant / roleName / when 条件表达式）</label>
          <el-input
            v-model="editExtractionsText"
            type="textarea"
            :autosize="{ minRows: 4, maxRows: 14 }"
            class="li-rules-json-area"
            spellcheck="false"
          />
          <!-- 仅序列成员产出：保存时对全部提取动作的 when 条件统一并入/移除「群组序号 ≥ 1」原子 -->
          <div class="li-rules-seq-line">
            <el-checkbox v-model="editIsSeqOnly">仅序列成员产出</el-checkbox>
            <span class="li-rules-seq-note">
              勾选后保存时对上方全部提取动作的 when 条件并入「群组序号 ≥ 1」原子（数值区间式，操作数=群组序号）：
              只有序列成员（规则定群或文件名元素对齐的结构群）才产出声明，落单文件不产出；取消勾选则移除该原子，
              其余手写条件原样保留。重开编辑器时，全部动作均含该原子则复选保持勾选；该原子也可直接手写进 JSON 表达。
            </span>
          </div>
        </div>

        <div class="li-rules-actions">
          <el-button type="primary" :loading="saving" @click="doSaveRule">保存入档案</el-button>
          <el-button :disabled="saving" @click="closeEdit">取消</el-button>
        </div>
        <p class="li-rules-hint">保存经构造期校验（正则编译、捕获组存在性、条件表达式引用与类型）；非法条目即时报错、不落盘</p>
      </section>

      <!-- 三、内置规则（代码内置表 · 禁用态存档案禁用清单） -->
      <section class="li-rules-card">
        <div class="li-rules-card-head">
          <h3 class="li-rules-card-title">内置规则 <span class="li-rules-card-note">{{ builtinActiveCount }}/{{ builtins.length }} 参与求值</span></h3>
        </div>
        <p class="li-rules-hint">内置规则在代码内置表、不占档案条目；禁用态记入档案「内置禁用清单」，合并求值时排除——禁用等价于从求值中移除，可随时恢复</p>
        <div v-for="b in builtins" :key="b.id" class="li-rules-row">
          <el-switch
            size="small"
            :model-value="!b.disabled"
            :loading="acting"
            @change="doToggleBuiltin(b, $event === true)"
          />
          <div class="li-rules-row-main">
            <div class="li-rules-row-desc" :title="b.description">{{ b.description }}</div>
            <div class="li-rules-row-id" :title="b.id">{{ b.id }}</div>
          </div>
          <el-tag size="small" type="info">内置</el-tag>
          <el-tag v-if="b.disabled" size="small" type="warning">已禁用</el-tag>
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

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

// 档案规则条目（ruleEntryJSON 形态：pattern / extractions 即 JSON 片段源）
interface RuleEntryView {
  id: string
  description: string
  source: string
  enabled: boolean
  pattern: unknown
  extractions?: unknown
  synthesis?: { dimPolicies?: DimPolicyView[] | null; roles?: RoleBindingView[] | null }
}

interface BuiltinRuleView {
  id: string
  description: string
  disabled: boolean
}

interface RulesAdminView {
  schemaVersion: number
  rules: RuleEntryView[]
  warnings?: string[]
  disabledBuiltinIDs?: string[]
  builtins: BuiltinRuleView[]
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
const builtins = ref<BuiltinRuleView[]>([])
const warnings = ref<string[]>([])
const backupAvailable = ref(false)
const builtinActiveCount = computed(() => builtins.value.filter((b) => !b.disabled).length)

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
  builtins.value = view.builtins || []
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
    actionError.value = `未在规则档案中找到规则「${ruleId}」（可能已被删除；内置规则不在档案条目内）`
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

// —— 二、单条编辑（表单化常用字段 + JSON 片段） ——

const editOpen = ref(false)
const editIsNew = ref(false)
const editId = ref('')
const editSource = ref('manual')
const editDescription = ref('')
const editEnabled = ref(true)
const editRoles = ref<RoleBindingView[]>([])
const editDims = ref<DimPolicyView[]>([])
const editPatternText = ref('')
const editExtractionsText = ref('')
// 仅序列成员产出（一等开关）：作用于全部提取动作 when 条件的「群组序号 ≥ 1」原子开关
const editIsSeqOnly = ref(false)
const saving = ref(false)

// 新建时的最小可改样例：pattern.captures 与 extractions.captureName 同名呼应
const NEW_PATTERN_SAMPLE = '{\n  "kind": "regex",\n  "target": "filename",\n  "expr": "^【(?P<author>[^】]+)】",\n  "captures": ["author"]\n}'
const NEW_EXTRACTIONS_SAMPLE = '[\n  {"dimension": "author", "from": "capture", "captureName": "author"}\n]'

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

const sourceKinds = [
  { key: 'dir', label: '目录策展' },
  { key: 'filename', label: '文件名解析' },
  { key: 'constant', label: '常量声明' },
  { key: 'group', label: '群组分析' },
  { key: 'predicate', label: '谓词命中' },
  { key: 'assignment', label: '会话赋值' },
  { key: 'adjudication', label: '裁决声明' }
]

const dimensions = [
  { key: 'author', label: '作者' },
  { key: 'title', label: '作品名' },
  { key: 'tag', label: '标签' },
  { key: 'collection', label: '作品集' }
]

const strategies = [
  { key: 'union', label: '并集' },
  { key: 'highTrust', label: '高可信择一' }
]

function openNew(): void {
  editIsNew.value = true
  editId.value = ''
  editSource.value = 'manual'
  editDescription.value = ''
  editEnabled.value = true
  editRoles.value = []
  editDims.value = []
  editPatternText.value = NEW_PATTERN_SAMPLE
  editExtractionsText.value = NEW_EXTRACTIONS_SAMPLE
  editIsSeqOnly.value = false
  editOpen.value = true
  actionError.value = ''
}

function openEdit(rule: RuleEntryView): void {
  editIsNew.value = false
  editId.value = rule.id
  editSource.value = rule.source
  editDescription.value = rule.description
  editEnabled.value = rule.enabled
  const synth = rule.synthesis || {}
  editRoles.value = ((synth.roles || []) as RoleBindingView[]).map((r) => ({ source: r.source, roleName: r.roleName }))
  editDims.value = ((synth.dimPolicies || []) as DimPolicyView[]).map((d) => ({ dimension: d.dimension, strategy: d.strategy }))
  editPatternText.value = JSON.stringify(rule.pattern, null, 2)
  editExtractionsText.value = JSON.stringify(rule.extractions ?? [], null, 2)
  // 派生态由档案动作的 when 内容决定：全部动作均含「群组序号 ≥ 1」原子才勾选
  editIsSeqOnly.value = checkExtractionsSeqOnly(rule.extractions)
  editOpen.value = true
  actionError.value = ''
}

function closeEdit(): void {
  editOpen.value = false
}

function addRole(): void {
  editRoles.value.push({ source: 'filename', roleName: '' })
}

function addDim(): void {
  editDims.value.push({ dimension: 'title', strategy: 'union' })
}

// —— 仅序列成员产出开关：「群组序号 ≥ 1」原子的构造 / 判别 / 并入 / 移除 ——
// 群组序号从 1 起，落单文件（不属于任何群组）取值不可用、含它的比较为假，
// 故该原子成立即「属于任意一个序列」（规则定群或结构群成员），不绑定群组来源。

// 条件表达式节点的松散形态：只按判别字段读写，动作/表达式其余字段一律透传不动
type CondNode = Record<string, unknown>

function isCondNode(e: unknown): e is CondNode {
  return typeof e === 'object' && e !== null && !Array.isArray(e)
}

// 「群组序号 ≥ 1」原子（数值区间式，操作数=群组序号）：与手写 JSON 可表达的形态一致
function buildSeqAtom(): CondNode {
  return { kind: 'range', operand: { kind: 'groupSeq' }, min: 1 }
}

// 原子判别：数值区间式、操作数=群组序号、下界恰为 1、无上界（带上界的更严区间是手写条件，不算本原子）
function checkSeqAtom(e: unknown): boolean {
  if (!isCondNode(e) || e['kind'] !== 'range') return false
  const operand = e['operand']
  if (!isCondNode(operand) || operand['kind'] !== 'groupSeq') return false
  return e['min'] === 1 && (e['max'] === undefined || e['max'] === null)
}

// 原子是否为整棵表达式的必要合取项（从根到原子只经过 and 节点）：or 分支或 not
// 内层的原子不构成「仅序列成员」限定——落单文件仍可能经析取分支产出，故不参与
// 派生勾选，也不会被开关移除
function checkRequiredConjunct(e: unknown): boolean {
  if (!isCondNode(e)) return false
  if (checkSeqAtom(e)) return true
  if (e['kind'] === 'and') {
    const subs = e['subs']
    if (Array.isArray(subs)) return subs.some((sub) => checkRequiredConjunct(sub))
  }
  return false
}

// 原子并入单条 when：已含必要合取则幂等返回；既有 and 节点直接追加子项（避免
// 层层包裹）；其余既有条件以 and 合取；原本无条件则以原子为整棵条件
function buildWhenWithSeqAtom(when: unknown): unknown {
  if (when == null) return buildSeqAtom()
  if (checkRequiredConjunct(when)) return when
  if (isCondNode(when) && when['kind'] === 'and') {
    const subs = when['subs']
    if (Array.isArray(subs)) return { ...when, subs: [...subs, buildSeqAtom()] }
  }
  return { kind: 'and', subs: [when, buildSeqAtom()] }
}

// 从 when 中移除必要合取位置上的全部原子：and 剔除后仅剩一项则收敛为该项、
// 剔除殆尽则该动作无条件（when 置空）；or / not 子树与叶子节点原样保留
function buildWhenWithoutSeqAtom(e: unknown): unknown {
  if (!isCondNode(e)) return e
  if (checkSeqAtom(e)) return null
  if (e['kind'] === 'and') {
    const subs = e['subs']
    if (Array.isArray(subs)) {
      const kept = subs
        .map((sub) => buildWhenWithoutSeqAtom(sub))
        .filter((sub) => sub !== null && sub !== undefined)
      if (kept.length === 0) return null
      if (kept.length === 1) return kept[0]
      return { ...e, subs: kept }
    }
  }
  return e
}

// 派生态：全部提取动作的 when 均含必要合取位置的原子才勾选（任一动作不含即不
// 勾选）；无提取动作时无产出可限定，不勾选
function checkExtractionsSeqOnly(extractions: unknown): boolean {
  if (!Array.isArray(extractions) || extractions.length === 0) return false
  return extractions.every((action) => isCondNode(action) && checkRequiredConjunct(action['when']))
}

// 开关落到已解析的提取动作数组（保存时调用）：勾选逐动作并入原子、取消逐动作
// 移除原子；仅改写 when 字段，非对象动作与不含 when 的动作原样透传
function applySeqSwitch(extractions: unknown[], seqOnly: boolean): unknown[] {
  return extractions.map((action) => {
    if (!isCondNode(action)) return action
    const when = action['when']
    const nextWhen = seqOnly ? buildWhenWithSeqAtom(when) : buildWhenWithoutSeqAtom(when)
    if (nextWhen === when) return action
    if (nextWhen == null) {
      const rest: CondNode = { ...action }
      delete rest['when']
      return rest
    }
    return { ...action, when: nextWhen }
  })
}

// 前端侧先解析两段 JSON 片段（拼装完整规则对象）；语义校验由后端构造期管线兜底
function parseFragment<T>(text: string, label: string): T | null {
  try {
    return JSON.parse(text) as T
  } catch (err) {
    actionError.value = `${label} 不是合法 JSON：${err instanceof Error ? err.message : String(err)}`
    return null
  }
}

async function doSaveRule(): Promise<void> {
  const pattern = parseFragment<unknown>(editPatternText.value, '模式 pattern')
  if (pattern === null) return
  const extractions = parseFragment<unknown[]>(editExtractionsText.value, '提取动作 extractions')
  if (extractions === null) return
  if (!Array.isArray(extractions)) {
    actionError.value = '提取动作 extractions 须为 JSON 数组'
    return
  }
  saving.value = true
  actionError.value = ''
  const rule = {
    id: editIsNew.value ? '' : editId.value,
    description: editDescription.value,
    source: editSource.value,
    enabled: editEnabled.value,
    pattern: pattern,
    // 「仅序列成员产出」在保存时统一落到各动作的 when 条件（并入/移除原子）
    extractions: applySeqSwitch(extractions, editIsSeqOnly.value),
    synthesis: {
      dimPolicies: editDims.value.filter((d) => d.dimension && d.strategy),
      roles: editRoles.value.filter((r) => r.source && r.roleName)
    }
  }
  const resp = await request('saveRule', { rule: rule })
  saving.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '保存规则失败'
    return
  }
  applyRulesView(resp.rulesView)
  notice.value = editIsNew.value
    ? '规则已新建并入档案（可在识别全景页重新识别查看效果）'
    : `规则「${editId.value}」已保存（可在识别全景页重新识别查看效果）`
  editOpen.value = false
}

// —— 三、内置规则启停（档案禁用清单承载） ——

async function doToggleBuiltin(builtin: BuiltinRuleView, enable: boolean): Promise<void> {
  acting.value = true
  actionError.value = ''
  const resp = await request('setBuiltinDisabled', { builtinId: builtin.id, disabled: !enable })
  acting.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '内置规则启停失败'
    return
  }
  applyRulesView(resp.rulesView)
  notice.value = `内置规则「${builtin.id}」已${enable ? '恢复参与求值' : '禁用（合并求值排除）'}`
}

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

.li-rules-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.li-rules-switch-line {
  display: flex;
  align-items: center;
}

.li-rules-subform {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-rules-subform-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--app-text-regular);
}

.li-rules-subform-head .li-rules-subform-head {
  margin-top: 6px;
}

.li-rules-subform-head > span {
  margin-right: auto;
}

.li-rules-inline-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-rules-inline-row .el-select {
  width: 160px;
}

.li-rules-inline-row .el-input {
  flex: 1;
}

.li-rules-json {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-rules-json-label {
  font-size: 13px;
  color: var(--app-text-regular);
}

.li-rules-json-area :deep(textarea) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
}

.li-rules-seq-line {
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

.li-rules-seq-note {
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-text-secondary);
}

.li-rules-actions {
  display: flex;
  gap: 8px;
}
</style>
