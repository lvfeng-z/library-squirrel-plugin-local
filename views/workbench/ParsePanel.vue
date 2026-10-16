<template>
  <div class="li-pp">
    <!-- 操作反馈：识别中状态条（复用全景进度事件驱动的 recognizing）与动作通知/错误 -->
    <div v-if="busy" class="li-pp-busy">重识别进行中……完成后本面板与全景自动刷新</div>
    <el-alert v-if="notice" class="li-pp-alert" type="success" :closable="false" show-icon :title="notice" />
    <el-alert v-if="actionError" class="li-pp-alert" type="error" :closable="false" show-icon :title="actionError" />

    <!-- 泛化失败半程态：赋值已保存、规则未生成——赋值不回滚，可原维度重试或保留赋值 -->
    <div v-if="halfState" class="li-pp-half">
      <div class="li-pp-half-title">赋值已保存、规则未生成（{{ genModeLabel(halfState.mode) }}）</div>
      <p class="li-pp-half-reason">{{ halfState.reason }}</p>
      <div class="li-pp-row">
        <el-button size="small" type="primary" :loading="generalizing" @click="retryGeneralize">重试泛化</el-button>
        <el-button size="small" @click="halfState = null">保留赋值，暂不泛化</el-button>
      </div>
      <p class="li-pp-hint">赋值不回滚：该文件的会话态覆盖已生效，重识别后按赋值呈现</p>
    </div>

    <!-- 待裁决：选中文件冲突维度就地双路裁决（左栏待裁决折叠区保留只读总览） -->
    <div v-if="pendingItems.length" class="li-pp-sec">
      <h4 class="li-pp-sec-title">待裁决冲突 <span class="li-pp-sec-note">{{ pendingItems.length }} 维 · 就地双路裁决</span></h4>
      <div v-for="p in pendingItems" :key="p.dimension" class="li-pp-pending">
        <div class="li-pp-pending-head">
          <span class="li-pp-pending-dim">{{ dimLabel(p.dimension) }} 冲突</span>
          <span class="li-pp-pending-reason" :title="p.reason">{{ p.reason }}</span>
        </div>
        <div class="li-pp-cands">
          <button
            v-for="(c, j) in p.candidates || []"
            :key="j"
            type="button"
            class="li-pp-cand"
            :class="{ 'is-active': pickByDim[p.dimension] === j }"
            @click="pickByDim[p.dimension] = j"
          >
            <span class="li-pp-cand-value">{{ dimLabel(c.dimension) }}={{ c.value }}</span>
            <span class="li-pp-cand-src">{{ sourceLabel(c.source) }}<template v-if="c.ruleId"> · {{ c.ruleId }}</template></span>
          </button>
        </div>
        <div class="li-pp-row">
          <el-button type="primary" size="small" :loading="adjudicating" @click="doAdjudicatePick(p)">裁决采纳所选（双命中）</el-button>
          <el-button size="small" @click="toggleConditional(p)">按条件改绑（单侧缺席）…</el-button>
        </div>
        <!-- 条件改绑面板：谓词条件策略（在场/缺席各采纳一候选） -->
        <div v-if="condOpenByDim[p.dimension]" class="li-pp-cond">
          <div class="li-pp-cond-row">
            <span class="li-pp-cond-label">条件谓词（何来源在何维度上的声明）：</span>
            <el-select v-model="predSourceByDim[p.dimension]" class="li-pp-cond-select" size="small">
              <el-option v-for="s in predSources" :key="s.key" :label="s.label" :value="s.key" />
            </el-select>
            <el-select v-model="predDimensionByDim[p.dimension]" class="li-pp-cond-select" size="small">
              <el-option v-for="d in predDims" :key="d.key" :label="d.label" :value="d.key" />
            </el-select>
          </div>
          <div class="li-pp-cond-row">
            <span class="li-pp-cond-label">谓词在场时采纳：</span>
            <button
              v-for="(c, j) in p.candidates || []"
              :key="`p${j}`"
              type="button"
              class="li-pp-cand is-mini"
              :class="{ 'is-active': presentByDim[p.dimension] === j }"
              @click="presentByDim[p.dimension] = j"
            >{{ c.value }}</button>
          </div>
          <div class="li-pp-cond-row">
            <span class="li-pp-cond-label">谓词缺席时采纳：</span>
            <button
              v-for="(c, j) in p.candidates || []"
              :key="`a${j}`"
              type="button"
              class="li-pp-cand is-mini"
              :class="{ 'is-active': absentByDim[p.dimension] === j }"
              @click="absentByDim[p.dimension] = j"
            >{{ c.value }}</button>
          </div>
          <div class="li-pp-row">
            <el-button type="primary" size="small" :loading="adjudicating" @click="doAdjudicateConditional(p)">生成条件策略</el-button>
          </div>
          <p class="li-pp-hint">生成「在场时用甲、缺席时用乙」的最简谓词条件策略（来源=裁决）</p>
        </div>
      </div>
    </div>

    <!-- 赋值表单：五字段，已赋值预填现值 + 已赋值/已泛化标记 -->
    <div class="li-pp-sec">
      <h4 class="li-pp-sec-title">
        赋值
        <el-tag v-if="currentAssignment" size="small">已赋值</el-tag>
        <el-tag v-if="currentAssignment && currentAssignment.generalized" size="small" type="success">已泛化</el-tag>
        <span v-if="ignored" class="li-pp-sec-note">已忽略</span>
      </h4>
      <div class="li-pp-form">
        <el-input v-model="formAuthor" placeholder="作者" size="small" clearable @input="markDirty" />
        <el-input v-model="formAuthorRole" placeholder="作者角色（如 原画，可空）" size="small" clearable @input="markDirty" />
        <el-input v-model="formTitle" placeholder="作品名" size="small" clearable @input="markDirty" />
        <el-input v-model="formTagsText" placeholder="标签（逗号分隔多个，可空）" size="small" clearable @input="markDirty" />
        <el-input v-model="formCollection" placeholder="作品集（集归属，可空）" size="small" clearable @input="markDirty" />
      </div>
      <!-- 合并流：泛化维度内联三选 + 主按钮组（仅保存 / 保存并泛化成规则） -->
      <div class="li-pp-merge">
        <div class="li-pp-genmodes">
          <button
            v-for="m in genModes"
            :key="m.key"
            type="button"
            class="li-pp-genmode"
            :class="{ 'is-active': genMode === m.key }"
            :title="m.desc"
            @click="genMode = m.key"
          >
            <span class="li-pp-genmode-label">{{ m.label }}</span>
          </button>
        </div>
        <div class="li-pp-row">
          <el-button type="primary" size="small" :loading="assigning" @click="doAssignOnly">仅保存赋值</el-button>
          <el-button type="primary" size="small" :loading="assigning || generalizing" @click="doAssignAndGeneralize">保存并泛化成规则</el-button>
        </div>
      </div>
      <div class="li-pp-row">
        <el-button size="small" :loading="ignoring" @click="doIgnoreAction">{{ ignored ? '取消忽略' : '忽略此文件' }}</el-button>
        <el-button size="small" :loading="assigning" @click="doClearAssign">清除赋值</el-button>
      </div>
      <p class="li-pp-hint">空维度不覆盖档案结论；全部留空即清除赋值。保存后后端自动排定增量重识别，完成后本面板与全景自动刷新</p>
    </div>

    <!-- 泛化/裁决生成物与验证轮差异提示（完整差异见左栏差异条） -->
    <div v-if="lastGenRule || lastCondition || diffTotal" class="li-pp-sec">
      <h4 class="li-pp-sec-title">
        生成规则与验证差异
        <span class="li-pp-sec-note">变更 {{ diffChangedCount }} · 新增 {{ diffAddedCount }} · 消失 {{ diffRemovedCount }}</span>
      </h4>
      <div v-if="lastGenRule" class="li-pp-gen">
        <p class="li-pp-gen-line"><span class="li-pp-gen-id">{{ lastGenRule.id }}</span>{{ lastGenRule.description }}</p>
        <p v-if="lastGenRule.pattern" class="li-pp-gen-pattern">{{ lastGenRule.pattern }}</p>
      </div>
      <div v-if="lastCondition" class="li-pp-condjson">
        <div class="li-pp-condjson-head">条件策略生成物（条件 JSON，与规则档案同构的表达式片段）</div>
        <pre class="li-pp-condjson-body">{{ lastCondition }}</pre>
      </div>
      <p v-if="diffTotal" class="li-pp-hint">生成规则后自动增量重识别——过窄＝差异里仍有未覆盖，过泛＝差异里出现错套，请核对</p>
      <div v-if="diffEntries.length" class="li-pp-difflist">
        <div
          v-for="e in diffEntries"
          :key="`${e.kind}/${e.relPath}`"
          class="li-pp-diff"
          :class="`is-${e.kind}`"
        >
          <span class="li-pp-diff-kind">{{ diffKindLabel(e.kind) }}</span>
          <span class="li-pp-diff-path" :title="e.relPath">{{ e.relPath }}</span>
          <span class="li-pp-diff-cols">
            <span class="li-pp-diff-side is-before">{{ conclusionSummary(e.before) }}</span>
            <span class="li-pp-diff-arrow">→</span>
            <span class="li-pp-diff-side is-after">{{ conclusionSummary(e.after) }}</span>
          </span>
        </div>
        <p v-if="diffTotal > DIFF_CAP" class="li-pp-hint">仅显示前 {{ DIFF_CAP }} 条，完整差异见左栏差异条</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'

// 右栏解析面板（阶段3）：文件详情区之外的交互面——赋值表单（五字段预填）、
// 仅保存赋值 / 保存并泛化成规则（泛化维度内联三选）、忽略/清除次级按钮、
// 待裁决就地双路裁决、泛化结果与差异提示。
// 请求经父组件 ParseView 的既有 workbench 请求通道（request 以 prop 注入，
// 不另建事件通道，避免双通道序号错配）；识别事件（recognitionProgress/
// recognitionDone）也由父组件统一监听并重取全景+决策态，本组件只消费
// recognizing/diff/assignments 等投影——赋值/清除应答后前端不再发识别请求。

interface PanelFile {
  relPath: string
  fileName: string
  state: string
  assigned?: boolean
}

interface PanelDecl {
  dimension: string
  value: string
  source: string
  roleName?: string
  ruleId?: string
}

interface PanelPending {
  relPath: string
  fileName: string
  dimension: string
  reason: string
  candidates: PanelDecl[]
}

interface PanelRule {
  id: string
  description: string
  pattern?: string
  condition?: string
}

interface PanelDiffConclusion {
  state?: string
  final?: PanelDecl[]
  pendingDims?: string[]
}

interface PanelDiffEntry {
  relPath: string
  before?: PanelDiffConclusion
  after?: PanelDiffConclusion
}

interface PanelDiff {
  changed?: PanelDiffEntry[]
  added?: PanelDiffEntry[]
  removed?: PanelDiffEntry[]
}

interface PanelAssignment {
  relPath: string
  author?: string
  authorRole?: string
  title?: string
  tags?: string[]
  collection?: string
  generalized?: boolean
  updatedAtMs?: number
}

// 应答窄投影：父通道应答结构是其超集（结构兼容，见 request prop 类型）
interface PanelResponse {
  ok: boolean
  error?: string
  rule?: PanelRule
}

const props = defineProps<{
  root: string
  file: PanelFile
  assignments: PanelAssignment[]
  ignored: boolean
  pendingItems: PanelPending[]
  diff: PanelDiff | null
  recognizing: boolean
  request: (action: string, payload: Record<string, unknown>) => Promise<PanelResponse>
}>()

const emit = defineEmits<{ (e: 'refresh'): void }>()

// —— 反馈态 ——
const notice = ref('')
const actionError = ref('')
// 泛化/裁决成功后验证轮在途的乐观标记：与父组件 recognizing 合成忙碌态，
// recognitionDone（recognizing 转 false）后清除
const verifyStarted = ref(false)
const busy = computed(() => props.recognizing || verifyStarted.value)
watch(
  () => props.recognizing,
  (v) => {
    if (!v) verifyStarted.value = false
  }
)

// 泛化失败半程态（赋值已保存、规则未生成）：记失败原因与当时的泛化维度，可原维度重试
const halfState = ref<{ reason: string; mode: string } | null>(null)

// —— 赋值表单（五字段，与 SubpageManualParse 字段定义一致） ——
const formAuthor = ref('')
const formAuthorRole = ref('')
const formTitle = ref('')
const formTagsText = ref('')
const formCollection = ref('')
const assigning = ref(false)
const ignoring = ref(false)
// 用户是否改动过表单：改动后自动刷新（assignments 投影更新）不再覆盖预填
let formDirty = false

function markDirty(): void {
  formDirty = true
}

const currentAssignment = computed<PanelAssignment | null>(() => {
  const rel = props.file ? props.file.relPath : ''
  if (!rel) return null
  return props.assignments.find((a) => a.relPath === rel) || null
})

function setForm(a: PanelAssignment | null): void {
  formAuthor.value = (a && a.author) || ''
  formAuthorRole.value = (a && a.authorRole) || ''
  formTitle.value = (a && a.title) || ''
  formTagsText.value = (a && a.tags ? a.tags.join('，') : '') || ''
  formCollection.value = (a && a.collection) || ''
  formDirty = false
}

// —— 合并流：泛化维度三选（复用 genModes 常量，默认文件名形态） ——
const genModes = [
  { key: 'filename', label: '按文件名形态', desc: '赋值值位升格为捕获组（如方括号作者）' },
  { key: 'dirLayer', label: '按目录层', desc: '父目录名锚定目录含义规则' },
  { key: 'sequence', label: '按序列形态', desc: '所属序列群前缀锚定定群规则' }
]
const genMode = ref('filename')
const generalizing = ref(false)

function genModeLabel(mode: string): string {
  const hit = genModes.find((m) => m.key === mode)
  return hit ? hit.label : mode
}

// —— 待裁决就地双路（维度键控的裁决选择态） ——
const adjudicating = ref(false)
const pickByDim = ref<Record<string, number>>({})
const condOpenByDim = ref<Record<string, boolean>>({})
const predSourceByDim = ref<Record<string, string>>({})
const predDimensionByDim = ref<Record<string, string>>({})
const presentByDim = ref<Record<string, number>>({})
const absentByDim = ref<Record<string, number>>({})

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

// —— 生成物与差异呈现 ——
const lastGenRule = ref<PanelRule | null>(null)
const lastCondition = ref('')
const DIFF_CAP = 8
const diffChangedCount = computed(() => (props.diff && props.diff.changed ? props.diff.changed.length : 0))
const diffAddedCount = computed(() => (props.diff && props.diff.added ? props.diff.added.length : 0))
const diffRemovedCount = computed(() => (props.diff && props.diff.removed ? props.diff.removed.length : 0))
const diffTotal = computed(() => diffChangedCount.value + diffAddedCount.value + diffRemovedCount.value)
const diffEntries = computed<{ kind: string; relPath: string; before?: PanelDiffConclusion; after?: PanelDiffConclusion }[]>(() => {
  const d = props.diff
  if (!d) return []
  const cap = (arr?: PanelDiffEntry[], kind = '') => (arr || []).slice(0, DIFF_CAP).map((e) => ({ kind, ...e }))
  return [...cap(d.changed, 'changed'), ...cap(d.added, 'added'), ...cap(d.removed, 'removed')]
})

// —— 面板复位：点行切换文件时表单与裁决/反馈态跟随重置并按现值预填 ——
function resetPanelState(): void {
  setForm(currentAssignment.value)
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  lastGenRule.value = null
  lastCondition.value = ''
  verifyStarted.value = false
  pickByDim.value = {}
  condOpenByDim.value = {}
  predSourceByDim.value = {}
  predDimensionByDim.value = {}
  presentByDim.value = {}
  absentByDim.value = {}
}

watch(
  () => props.file && props.file.relPath,
  () => {
    resetPanelState()
  }
)

// 自动刷新落盘（assignments 投影更新）：表单未被用户改动时跟随预填现值（含已泛化标记随标记位刷新）
watch(
  () => props.assignments,
  () => {
    if (!formDirty) setForm(currentAssignment.value)
  }
)

// 初挂载即按现值预填（选中态先于组件挂载就位）
resetPanelState()

// —— 赋值动作（动作协议零改动，经父通道串行调用） ——

async function runAssign(): Promise<boolean> {
  if (!props.root || !props.file) {
    actionError.value = '会话或目标文件缺失，无法保存赋值'
    return false
  }
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  const tags = formTagsText.value
    .split(/[,，、]/)
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
  const resp = await props.request('assign', {
    root: props.root,
    relPath: props.file.relPath,
    author: formAuthor.value.trim(),
    authorRole: formAuthorRole.value.trim(),
    title: formTitle.value.trim(),
    tags: tags,
    collection: formCollection.value.trim()
  })
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '保存赋值失败'
    return false
  }
  return true
}

// 仅保存赋值：应答即终态呈现（后端已排自动增量重识别，无需前端再发识别请求）
async function doAssignOnly(): Promise<void> {
  const ok = await runAssign()
  if (!ok) return
  formDirty = false
  notice.value = '已保存赋值（重识别后生效，识别完成本面板自动刷新）'
}

// 保存并泛化成规则：前端串行 assign → generalize(mode)；泛化失败进入半程态（赋值不回滚）
async function doAssignAndGeneralize(): Promise<void> {
  const ok = await runAssign()
  if (!ok) return
  formDirty = false
  await runGeneralize(genMode.value)
}

async function runGeneralize(mode: string): Promise<void> {
  if (!props.root || !props.file) return
  generalizing.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('generalize', {
    root: props.root,
    relPath: props.file.relPath,
    generalizeMode: mode
  })
  generalizing.value = false
  if (!resp.ok || !resp.rule) {
    // 半程态：赋值已保存、规则未生成——保留失败原因与泛化维度供重试，赋值不回滚
    halfState.value = { reason: resp.error || '泛化生成失败', mode: mode }
    return
  }
  halfState.value = null
  lastGenRule.value = resp.rule
  lastCondition.value = resp.rule.condition || ''
  notice.value = `已生成规则 ${resp.rule.id}；验证轮重识别已启动，差异见下方`
  verifyStarted.value = true
}

async function retryGeneralize(): Promise<void> {
  if (!halfState.value) return
  await runGeneralize(halfState.value.mode)
}

// 清除赋值：后端 ApplyAssignment 已排自动刷新，应答后无需前端再发识别请求
async function doClearAssign(): Promise<void> {
  if (!props.root || !props.file) return
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  const resp = await props.request('clearAssignment', {
    root: props.root,
    relPath: props.file.relPath
  })
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '清除赋值失败'
    return
  }
  setForm(null)
  lastGenRule.value = null
  lastCondition.value = ''
  notice.value = '已清除赋值（重识别后生效）'
}

// 忽略/取消忽略：忽略不触发识别，应答后仅请父组件重取全景（含决策态）
async function doIgnoreAction(): Promise<void> {
  if (!props.root || !props.file) return
  ignoring.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('setIgnored', {
    root: props.root,
    relPath: props.file.relPath,
    ignored: !props.ignored
  })
  ignoring.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '忽略项登记失败'
    return
  }
  notice.value = props.ignored ? '已取消忽略' : '已忽略此文件'
  emit('refresh')
}

// —— 就地裁决双路 ——

function toggleConditional(p: PanelPending): void {
  const k = p.dimension
  condOpenByDim.value[k] = !condOpenByDim.value[k]
  if (condOpenByDim.value[k]) {
    // 缺省：谓词=目录源作者、在场=首个候选、缺席=次候选
    if (predSourceByDim.value[k] === undefined) predSourceByDim.value[k] = 'dir'
    if (predDimensionByDim.value[k] === undefined) predDimensionByDim.value[k] = 'author'
    if (presentByDim.value[k] === undefined) presentByDim.value[k] = 0
    if (absentByDim.value[k] === undefined) absentByDim.value[k] = 1
  }
  notice.value = ''
  actionError.value = ''
}

// 双路一：双命中择一（pick）——两来源声明均在场，采纳所选候选生成组合规则
async function doAdjudicatePick(p: PanelPending): Promise<void> {
  if (!props.root) return
  if (pickByDim.value[p.dimension] === undefined) {
    actionError.value = '请先点选要采纳的候选'
    return
  }
  adjudicating.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('adjudicate', {
    root: props.root,
    relPath: p.relPath,
    dimension: p.dimension,
    adjudicateMode: 'pick',
    pickIndex: pickByDim.value[p.dimension]
  })
  adjudicating.value = false
  if (!resp.ok || !resp.rule) {
    actionError.value = resp.error || '裁决生成失败'
    return
  }
  lastGenRule.value = resp.rule
  lastCondition.value = resp.rule.condition || ''
  notice.value = `已生成组合规则 ${resp.rule.id}；验证轮重识别已启动，差异见下方`
  verifyStarted.value = true
}

// 双路二：单侧缺席条件改绑（conditional）——生成「在场用甲、缺席用乙」的谓词条件策略
async function doAdjudicateConditional(p: PanelPending): Promise<void> {
  if (!props.root) return
  adjudicating.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('adjudicate', {
    root: props.root,
    relPath: p.relPath,
    dimension: p.dimension,
    adjudicateMode: 'conditional',
    predicateSource: predSourceByDim.value[p.dimension] || 'dir',
    predicateDimension: predDimensionByDim.value[p.dimension] || 'author',
    presentIndex: presentByDim.value[p.dimension] || 0,
    absentIndex: absentByDim.value[p.dimension] === undefined ? 1 : absentByDim.value[p.dimension]
  })
  adjudicating.value = false
  if (!resp.ok || !resp.rule) {
    actionError.value = resp.error || '条件策略生成失败'
    return
  }
  lastGenRule.value = resp.rule
  lastCondition.value = resp.rule.condition || ''
  condOpenByDim.value[p.dimension] = false
  notice.value = `已生成条件策略 ${resp.rule.id}；验证轮重识别已启动，差异见下方`
  verifyStarted.value = true
}

// —— 呈现辅助（与 SubpageManualParse 同源词条） ——

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
const diffKindLabel = (kind: string): string => {
  switch (kind) {
    case 'changed': return 'Δ变更'
    case 'added': return '＋新增'
    case 'removed': return '－消失'
    default: return kind
  }
}
const conclusionSummary = (c?: PanelDiffConclusion | null): string => {
  if (!c) return '（无）'
  const parts: string[] = []
  for (const d of c.final || []) parts.push(`${dimLabel(d.dimension)}=${d.value}`)
  if (c.pendingDims && c.pendingDims.length) parts.push(`待裁决:${c.pendingDims.join('/')}`)
  if (!parts.length) return c.state || '（无）'
  return `${c.state || ''}（${parts.join('、')}）`
}
</script>

<style scoped>
.li-pp {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.li-pp-alert {
  border-radius: var(--app-radius);
}

.li-pp-busy {
  font-size: 12px;
  padding: 5px 10px;
  border-radius: var(--app-radius);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-pp-sec {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.li-pp-sec-title {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-primary);
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pp-sec-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-pp-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

.li-pp-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* 半程态：泛化失败的半程提示块 */
.li-pp-half {
  border: 1px solid var(--app-tag-warning-bg);
  border-radius: var(--app-radius);
  background: var(--app-tag-warning-bg);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pp-half-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-tag-warning-text);
}

.li-pp-half-reason {
  margin: 0;
  font-size: 12px;
  color: var(--app-tag-warning-text);
  word-break: break-all;
}

/* 赋值表单（窄栏纵排） */
.li-pp-form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 合并流：泛化维度内联三选 + 主按钮组 */
.li-pp-merge {
  border: 1px dashed var(--app-border-color-light);
  border-radius: var(--app-radius);
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.li-pp-genmodes {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pp-genmode {
  flex: 1 1 auto;
  min-width: 0;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 4px 8px;
  cursor: pointer;
  text-align: center;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-pp-genmode:hover {
  border-color: var(--app-color-primary);
}

.li-pp-genmode.is-active {
  border-color: var(--app-color-primary);
  color: var(--app-color-primary);
  background: var(--app-color-primary-light, rgba(64, 158, 255, 0.08));
}

.li-pp-genmode-label {
  display: block;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 待裁决双路 */
.li-pp-pending {
  border-top: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  padding-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pp-pending-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.li-pp-pending-dim {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-primary);
  flex-shrink: 0;
}

.li-pp-pending-reason {
  font-size: 12px;
  color: var(--app-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-pp-cands {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-pp-cand {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 4px 8px;
  cursor: pointer;
  text-align: left;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.li-pp-cand:hover {
  border-color: var(--app-color-primary);
}

.li-pp-cand.is-active {
  border-color: var(--app-color-primary);
  background: var(--app-color-primary-light, rgba(64, 158, 255, 0.08));
}

.li-pp-cand.is-mini {
  padding: 2px 8px;
  font-size: 12px;
  justify-content: flex-start;
}

.li-pp-cand-value {
  font-size: 12px;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-pp-cand-src {
  font-size: 11px;
  color: var(--app-text-secondary);
  flex-shrink: 0;
}

/* 条件改绑面板 */
.li-pp-cond {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pp-cond-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pp-cond-label {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pp-cond-select {
  width: 110px;
}

/* 生成物与差异 */
.li-pp-gen-line {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-primary);
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
  word-break: break-all;
}

.li-pp-gen-id {
  font-weight: 600;
  color: var(--app-color-primary);
}

.li-pp-gen-pattern {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-pp-condjson {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-pp-condjson-head {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pp-condjson-body {
  margin: 0;
  padding: 6px 8px;
  border-radius: var(--app-radius);
  background: var(--app-bg-page, #f5f7fa);
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  font-size: 11px;
  line-height: 1.5;
  max-height: 140px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.li-pp-difflist {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-pp-diff {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  font-size: 12px;
}

.li-pp-diff-kind {
  flex-shrink: 0;
  font-size: 11px;
}

.li-pp-diff.is-changed .li-pp-diff-kind {
  color: var(--app-color-warning, #e6a23c);
}

.li-pp-diff.is-added .li-pp-diff-kind {
  color: var(--app-color-success, #67c23a);
}

.li-pp-diff.is-removed .li-pp-diff-kind {
  color: var(--app-color-danger, #f56c6c);
}

.li-pp-diff-path {
  flex-shrink: 0;
  max-width: 40%;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-pp-diff-cols {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 4px;
  flex-wrap: wrap;
}

.li-pp-diff-side {
  font-size: 11px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-pp-diff-side.is-before {
  text-decoration: line-through;
  opacity: 0.7;
}

.li-pp-diff-arrow {
  color: var(--app-text-secondary);
}
</style>
