<template>
  <AutoHeightDialog
    v-model:state="visible"
    width="min(880px, 94vw)"
    :close-on-click-modal="false"
  >
    <template #header>
      <div class="li-red-header">
        <span class="li-red-title">{{ headerTitle }}</span>
        <span class="li-red-header-note">{{ headerNote }}</span>
      </div>
    </template>

    <div class="li-red-body">
      <!-- ================= 入口①：档案规则编辑（SubpageRules「编辑/新建」） ================= -->
      <template v-if="mode === 'rule'">
        <!-- 规则元信息：描述 + 启停 -->
        <div class="li-red-meta">
          <el-input v-model="descInput" placeholder="规则描述（人话，列表呈现用）" clearable />
          <el-switch v-model="enabledInput" active-text="启用" inactive-text="停用" />
        </div>

        <!-- 上半「识别规则」：人话摘要 + 结构化形态编辑 + 捕获组声明清单 + JSON 高级切换 -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">识别规则<span class="li-red-sec-note">上半 · 决定规则命中哪些文件</span></h4>
            <el-button size="small" text @click="togglePatternJson">
              {{ patternJsonMode ? '切回结构化编辑' : '高级（JSON）' }}
            </el-button>
          </div>
          <p class="li-red-summary">{{ patternSummary }}</p>

          <template v-if="!patternJsonMode">
            <!-- 捕获组声明清单（名称 + 来源正则）：提取段下拉的唯一来源 -->
            <div v-if="captureDecls.length" class="li-red-caps">
              <span class="li-red-label">捕获组声明清单</span>
              <div v-for="(c, i) in captureDecls" :key="`${c.name}/${i}`" class="li-red-cap-row">
                <span class="li-red-cap-name">{{ c.name }}</span>
                <span class="li-red-cap-expr" :title="c.expr">← {{ c.expr }}</span>
              </div>
            </div>
            <p v-else class="li-red-hint">
              未声明捕获组——提取动作引用捕获组前，须先在下方形态内声明（仅文件名 / 相对路径目标的正则参与绑定，
              目录名目标不绑定）
            </p>
            <RulePatternNode :node="patternRoot" />
          </template>

          <template v-else>
            <el-input
              v-model="patternText"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 16 }"
              class="li-red-json-area"
              spellcheck="false"
            />
            <p v-if="patternError" class="li-red-error">模式 pattern JSON：{{ patternError }}</p>
            <p class="li-red-hint">
              JSON 对象（regex / location / sibling / predicate / and / or / not，复杂树直通原文编辑）；
              切回结构化编辑前须通过解析，解析错不丢输入
            </p>
          </template>
        </section>

        <!-- 下半「提取规则」：逐维度行 = 维度 ← 来源 + 剥壳前后缀 + 条件勾选；行可增删 -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">提取规则<span class="li-red-sec-note">下半 · 命中后取值声明到维度</span></h4>
            <el-button size="small" text @click="toggleExtractionsJson">
              {{ extractionsJsonMode ? '切回结构化编辑' : '高级（JSON）' }}
            </el-button>
          </div>

          <template v-if="!extractionsJsonMode">
            <div
              v-for="(row, i) in extractionRows"
              :key="i"
              class="li-red-ex-row"
              :class="{ 'is-invalid': !!extractionRowHint(row) }"
            >
              <div class="li-red-ex-main">
                <el-select v-model="row.dimension" size="small" placeholder="维度" class="li-red-ex-dim">
                  <el-option v-for="d in DIMENSION_OPTIONS" :key="d.value" :label="d.label" :value="d.value" />
                </el-select>
                <span class="li-red-arrow">←</span>
                <el-select v-model="row.from" size="small" placeholder="来源" class="li-red-ex-from">
                  <el-option v-for="f in FROM_OPTIONS" :key="f.value" :label="f.label" :value="f.value" />
                </el-select>
                <el-select
                  v-if="row.from === 'capture'"
                  v-model="row.captureName"
                  size="small"
                  placeholder="捕获组（只列上半声明的组）"
                  class="li-red-ex-capture"
                >
                  <el-option v-for="c in captureDecls" :key="c.name" :label="c.name" :value="c.name" />
                </el-select>
                <el-input
                  v-else-if="row.from === 'constant' || row.from === 'adjudication'"
                  v-model="row.constant"
                  size="small"
                  placeholder="常量值"
                  class="li-red-ex-const"
                />
                <el-button size="small" text type="danger" @click="removeExtractionRow(i)">删行</el-button>
              </div>
              <div class="li-red-ex-sub">
                <el-input v-model="row.stripPrefix" size="small" placeholder="剥壳前缀（可空）" class="li-red-ex-strip" />
                <el-input v-model="row.stripSuffix" size="small" placeholder="剥壳后缀（可空）" class="li-red-ex-strip" />
                <el-input v-model="row.roleName" size="small" placeholder="角色名（可空）" class="li-red-ex-role" />
                <label class="li-red-cond">
                  <input v-model="row.seqOnly" type="checkbox">
                  <span>仅序列成员产出</span>
                </label>
                <span v-if="extractionRowHasOtherConds(row)" class="li-red-other">含其他手写条件（高级 JSON 模式可编辑）</span>
              </div>
              <p v-if="extractionRowHint(row)" class="li-red-row-hint">{{ extractionRowHint(row) }}</p>
            </div>
            <el-button size="small" text @click="addExtractionRow">+ 加一行</el-button>
          </template>

          <template v-else>
            <el-input
              v-model="extractionsText"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 16 }"
              class="li-red-json-area"
              spellcheck="false"
            />
            <p v-if="extractionsError" class="li-red-error">提取动作 extractions JSON：{{ extractionsError }}</p>
            <p class="li-red-hint">JSON 数组（dimension / from / captureName / constant / roleName / strip 前后缀 / when 条件表达式）</p>
          </template>
        </section>

        <!-- 合成策略（随旧表单随迁）：角色绑定 + 维度策略 -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">合成策略<span class="li-red-sec-note">该规则产出声明的合成方式</span></h4>
          </div>
          <div class="li-red-synth-head">
            <span>角色绑定（来自该来源的声明一律附加角色）</span>
            <el-button size="small" text @click="roleRows.push({ source: 'filename', roleName: '' })">加一行</el-button>
          </div>
          <div v-for="(role, i) in roleRows" :key="`role${i}`" class="li-red-inline-row">
            <el-select v-model="role.source" size="small" placeholder="来源" class="li-red-inline-select">
              <el-option v-for="s in SOURCE_KIND_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
            <el-input v-model="role.roleName" size="small" placeholder="角色名（如 原画）" class="li-red-inline-input" />
            <el-button size="small" text type="danger" @click="roleRows.splice(i, 1)">删</el-button>
          </div>
          <div class="li-red-synth-head">
            <span>维度策略（同维度多来源候选的合成方式）</span>
            <el-button size="small" text @click="dimRows.push({ dimension: 'title', strategy: 'union' })">加一行</el-button>
          </div>
          <div v-for="(dim, i) in dimRows" :key="`dim${i}`" class="li-red-inline-row">
            <el-select v-model="dim.dimension" size="small" placeholder="维度" class="li-red-inline-select">
              <el-option v-for="d in DIMENSION_OPTIONS" :key="d.value" :label="d.label" :value="d.value" />
            </el-select>
            <el-select v-model="dim.strategy" size="small" placeholder="策略" class="li-red-inline-select">
              <el-option v-for="s in STRATEGY_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
            <el-button size="small" text type="danger" @click="dimRows.splice(i, 1)">删</el-button>
          </div>
        </section>
      </template>

      <!-- ================= 入口②：集规则调整（SetRulePanel「调整规则」，剧本E 语义随迁） ================= -->
      <template v-else-if="suggestion">
        <!-- 上半「识别规则」：草稿识别形态（只读呈现——保存走 saveWorkSetRuleAdjust，形态由指派重推导） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">识别规则<span class="li-red-sec-note">上半 · 草稿识别形态（只读）</span></h4>
          </div>
          <p class="li-red-summary">{{ suggestion.shape || '（推导未成，无共同形态）' }}</p>
          <p v-if="suggestion.failed" class="li-red-error">{{ suggestion.failReason }}</p>
          <p v-if="suggestion.summary" class="li-red-hint">{{ suggestion.summary }}</p>
          <div v-if="suggestion.mappings && suggestion.mappings.length" class="li-red-maps">
            <div v-for="(m, i) in suggestion.mappings" :key="i" class="li-red-map">
              <span class="li-red-map-dim">{{ dimLabel(m.dimension) }}</span>
              <span class="li-red-arrow">←</span>
              <span class="li-red-map-from">{{ m.from }}</span>
            </div>
          </div>
          <div v-if="suggestion.rules && suggestion.rules.length" class="li-red-drafts">
            <p v-for="r in suggestion.rules" :key="r.id" class="li-red-draft">
              <span class="li-red-draft-id">{{ r.id }}</span>
              <span class="li-red-draft-desc">{{ r.description }}</span>
            </p>
          </div>
        </section>

        <!-- 下半「提取规则」：线索逐项指派行（剧本E 全部语义：三族来源 / 默认跟随 /
             目录段多段连续组合 / 互斥行级提示） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">提取规则<span class="li-red-sec-note">下半 · 线索逐项指派 · 保存即入档</span></h4>
          </div>
          <p class="li-red-hint">
            每条线索指派到维度/集名组合/可变/目录锚定/不用之一；集名默认跟随识别形态（序列形态识别=「序列」行
            〔群键/公共前缀〕、目录名形态识别=目录标题段）；集名组合可勾选连续多段——文件名段与目录段同语义
            （目录段组合同样须连续、段间原文保留）；末段序号默认已指。
          </p>
          <div class="li-red-clue-table">
            <div v-for="c in suggestion.clues || []" :key="c.ref" class="li-red-clue-row">
              <span class="li-red-clue-label" :title="c.ref">{{ c.label }}</span>
              <span class="li-red-clue-value" :title="c.value">{{ clueValueText(c) }}</span>
              <select v-model="assignMap[c.ref]" class="li-red-clue-select" @change="onAssignChange(c.ref)">
                <option v-for="opt in destOptionsOf(c)" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
              </select>
              <span v-if="clueRowHint(c)" class="li-red-clue-hint">{{ clueRowHint(c) }}</span>
            </div>
          </div>
          <label class="li-red-cond">
            <input v-model="memberOnly" type="checkbox">
            <span>仅序列成员产出</span>
          </label>
        </section>
      </template>
    </div>

    <template #footer>
      <div class="li-red-footer">
        <span v-if="saveError" class="li-red-save-error" :title="saveError">{{ saveError }}</span>
        <span class="li-red-footer-actions">
          <el-button v-if="mode === 'setAdjust'" size="small" @click="resetAssignDefaults">还原默认指派</el-button>
          <el-button :disabled="saving" @click="cancel">取消</el-button>
          <el-button type="primary" :loading="saving" @click="save">
            {{ mode === 'rule' ? '保存入档案' : '保存规则' }}
          </el-button>
        </span>
      </div>
    </template>
  </AutoHeightDialog>
</template>

<script setup lang="ts">
// 两段式规则编辑弹窗（剧本F）：主程序全局注册的 AutoHeightDialog 壳（内容随窗口
// 高度封顶内滚，EntryConfirmDialog 先例——无需 import）。所有规则编辑处共用本
// 组件，两入口挂载、按 mode 分形：
//   ① rule（SubpageRules「编辑/新建」）：上半=识别规则（一行人话摘要+结构化形态
//      编辑〔文件名形态/目录锚定/序列群组/组合，RulePatternNode 递归展开〕+捕获组
//      声明清单+「高级（JSON）」切换双向同步），下半=提取规则（逐维度行=维度←
//      来源下拉〔捕获组只列上半声明的组/目录名/群组前缀/常量〕+剥壳前后缀+条件
//      勾选〔仅序列成员产出等，逐行〕；行可增删；行级就地校验）；合成策略随旧
//      表单随迁。保存载荷经 emit 交父组件走既有 saveRule RPC。
//   ② setAdjust（SetRulePanel「调整规则」）：上半=草稿识别形态（只读呈现），
//      下半=指派行——剧本E 全部语义随迁：线索三族（序列 seqKey/目录段/文件名段）
//      作为来源选项、默认指派预填、目录段多段连续勾选、互斥校验「一个维度只能有
//      一个来源」「目录段组合须为连续段」行级提示、单选目的地互斥挤占、还原默认
//      指派。保存载荷经 emit 交父组件走既有 saveWorkSetRuleAdjust RPC。
// 校验不过（组引用/维度来源唯一/JSON 解析）行级就地标红、弹窗不关；JSON 高级
// 模式解析错就地显示位置不丢输入；取消不保存（destroy-on-close，再开回上次保
// 存态——打开时从 props 重新取值）。
import { ref, computed, watch } from 'vue'
import RulePatternNode from './RulePatternNode.vue'
import {
  DIMENSION_OPTIONS,
  FROM_OPTIONS,
  SOURCE_KIND_OPTIONS,
  STRATEGY_OPTIONS,
  NEW_PATTERN_SAMPLE,
  NEW_EXTRACTIONS_SAMPLE,
  cloneJSON,
  collectPatternCaptures,
  patternSummaryOf,
  defaultPatternNode,
  checkRequiredConjunct,
  buildWhenWithSeqAtom,
  buildWhenWithoutSeqAtom,
  hasOtherConds,
  type PatternNode,
  type ExtractionActionJSON,
  type CaptureDecl
} from './ruleFormModel'

// —— 入口载荷形态（父组件组装的普通对象） ——

// rule 模式初始档案条目（SubpageRules 的 RuleEntryView 投影）。
interface RuleInput {
  id: string
  description: string
  source: string
  enabled: boolean
  isNew: boolean
  pattern: unknown
  extractions: unknown
  synthesis?: { dimPolicies?: { dimension: string; strategy: string }[] | null; roles?: { source: string; roleName: string }[] | null }
}

// setAdjust 模式的集规则草稿（SetRulePanel 的 SetRuleSuggestion 结构投影）。
interface ClueView {
  ref: string
  kind: string
  label: string
  value: string
  ordinal?: number
}

interface SuggestionInput {
  setName?: string
  shape?: string
  summary?: string
  mappings?: { dimension: string; from: string }[]
  rules?: { id: string; description: string; pattern?: string }[]
  failed?: boolean
  failReason?: string
  clues?: ClueView[]
  defaultCollectionSegRefs?: string[]
  defaultVariableSegRefs?: string[]
  defaultSeqSegRef?: string
  defaultMemberOnly?: boolean
  defaultAuthorClueRef?: string
}

const props = defineProps<{
  state: boolean
  mode: 'rule' | 'setAdjust'
  rule?: RuleInput | null
  suggestion?: SuggestionInput | null
  initialAssigns?: Record<string, string>
  initialMemberOnly?: boolean
  // 父组件回填的保存失败文案（后端构造期校验拒绝等）：弹窗内呈现、不关窗
  saveError?: string
  saving?: boolean
}>()

// rule 模式保存载荷（完整规则条目 JSON——父组件原样进 saveRule）。
interface RuleSavePayload {
  id: string
  description: string
  source: string
  enabled: boolean
  pattern: unknown
  extractions: unknown
  synthesis: { dimPolicies: { dimension: string; strategy: string }[]; roles: { source: string; roleName: string }[] }
}

// setAdjust 模式保存载荷（指派结果——父组件换算为 saveWorkSetRuleAdjust 载荷）。
interface SetAdjustSavePayload {
  assigns: Record<string, string>
  memberOnly: boolean
}

const emit = defineEmits<{
  (e: 'update:state', value: boolean): void
  (e: 'save', payload: RuleSavePayload | SetAdjustSavePayload): void
  (e: 'dirty-change', dirty: boolean): void
}>()

const visible = computed({
  get: (): boolean => props.state,
  set: (value: boolean) => emit('update:state', value)
})

const sourceLabels: Record<string, string> = {
  builtin: '内置',
  manual: '手写',
  qna: '泛化',
  adjudication: '裁决'
}

const headerTitle = computed((): string => {
  if (props.mode === 'rule') return props.rule && props.rule.isNew ? '新建规则' : '编辑规则'
  return '调整规则'
})

const headerNote = computed((): string => {
  if (props.mode === 'rule') {
    if (!props.rule || props.rule.isNew) return '来源：手写（manual）· 保存时自动生成 ID'
    return `${props.rule.id} · 来源：${sourceLabels[props.rule.source] || props.rule.source}（不可改）`
  }
  return `线索逐项指派 · 保存即入档${props.suggestion && props.suggestion.setName ? ` · ${props.suggestion.setName}` : ''}`
})

// —— rule 模式编辑态 ——

// 提取段行模型：when 为原样透传的条件树，seqOnly 为「仅序列成员产出」勾选
//（保存时并入/移除「群组序号 ≥ 1」原子，其余手写条件原样保留）。
interface ExtractionRow {
  dimension: string
  from: string
  captureName: string
  constant: string
  roleName: string
  stripPrefix: string
  stripSuffix: string
  seqOnly: boolean
  when: unknown
}

const descInput = ref('')
const enabledInput = ref(true)
const roleRows = ref<{ source: string; roleName: string }[]>([])
const dimRows = ref<{ dimension: string; strategy: string }[]>([])
const patternRoot = ref<PatternNode>(defaultPatternNode('regex'))
const patternText = ref('')
const patternJsonMode = ref(false)
const patternError = ref('')
// JSON 模式下最近一次成功解析的结构（捕获组下拉与摘要跟随它；解析失败保留旧值）
const patternParsed = ref<PatternNode | null>(null)
const extractionRows = ref<ExtractionRow[]>([])
const extractionsText = ref('')
const extractionsJsonMode = ref(false)
const extractionsError = ref('')

// JSON 解析（成功/失败带原文错误信息——V8 报文自带行/列位置）
function tryParseJSON(text: string): { ok: boolean; value?: unknown; error?: string } {
  try {
    return { ok: true, value: JSON.parse(text) }
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
}

// 任意 JSON → 形态节点（缺 kind 的非法形态回落默认 regex 节点，不丢打开机会）
function normalizePattern(raw: unknown): PatternNode {
  if (raw != null && typeof raw === 'object' && typeof (raw as PatternNode).kind === 'string') {
    return raw as PatternNode
  }
  return defaultPatternNode('regex')
}

// 提取动作 JSON → 行模型（seqOnly 由 when 内容派生）
function actionToRow(action: ExtractionActionJSON): ExtractionRow {
  const when = action.when ?? null
  return {
    dimension: action.dimension || '',
    from: action.from || 'capture',
    captureName: action.captureName || '',
    constant: action.constant || '',
    roleName: action.roleName || '',
    stripPrefix: action.stripPrefix || '',
    stripSuffix: action.stripSuffix || '',
    seqOnly: checkRequiredConjunct(when),
    when: when
  }
}

// 行模型 → 提取动作 JSON（seqOnly 落到该行 when 的并入/移除；仅携带有值字段）
function rowsToActions(): ExtractionActionJSON[] {
  return extractionRows.value.map((row) => {
    const action: ExtractionActionJSON = { dimension: row.dimension, from: row.from }
    if (row.from === 'capture') action.captureName = row.captureName
    if (row.from === 'constant' || row.from === 'adjudication') action.constant = row.constant
    if (row.roleName) action.roleName = row.roleName
    if (row.stripPrefix) action.stripPrefix = row.stripPrefix
    if (row.stripSuffix) action.stripSuffix = row.stripSuffix
    const when = row.seqOnly ? buildWhenWithSeqAtom(row.when) : buildWhenWithoutSeqAtom(row.when)
    if (when != null) action.when = when
    return action
  })
}

// —— 识别段联动：上半改形态/删捕获组 → 捕获组清单与提取段下拉即时跟随 ——

// 当前生效的形态结构（JSON 模式=最近一次成功解析；结构化模式=编辑树）
const activePattern = computed<PatternNode | null>(() =>
  patternJsonMode.value ? patternParsed.value : patternRoot.value
)

const captureDecls = computed<CaptureDecl[]>(() => collectPatternCaptures(activePattern.value))

const declaredNames = computed<Set<string>>(() => new Set(captureDecls.value.map((c) => c.name)))

const patternSummary = computed((): string => {
  if (patternJsonMode.value && !patternParsed.value) return '（JSON 待解析为合法对象后呈现摘要）'
  return patternSummaryOf(activePattern.value)
})

// JSON 模式下边输入边解析：合法即刷新下拉/摘要，非法就地显示错误位置、不丢输入
watch(patternText, () => {
  if (!patternJsonMode.value) return
  const parsed = tryParseJSON(patternText.value)
  if (parsed.ok && parsed.value != null && typeof parsed.value === 'object') {
    patternParsed.value = parsed.value as PatternNode
    patternError.value = ''
  } else if (parsed.ok) {
    patternError.value = '顶层须为 JSON 对象'
  } else {
    patternError.value = parsed.error || '解析失败'
  }
})

// 进入 JSON 高级模式：现行结构序列化为原文（双向同步——结构化改动能带进 JSON）
function togglePatternJson(): void {
  if (patternJsonMode.value) {
    // 切回结构化：须先通过解析（失败留在 JSON 模式、显示错误位置、不丢输入）
    const parsed = tryParseJSON(patternText.value)
    if (!parsed.ok || parsed.value == null || typeof parsed.value !== 'object') {
      patternError.value = parsed.ok ? '顶层须为 JSON 对象' : parsed.error || '解析失败'
      return
    }
    patternRoot.value = normalizePattern(parsed.value)
    patternParsed.value = patternRoot.value
    patternJsonMode.value = false
    patternError.value = ''
    return
  }
  patternText.value = JSON.stringify(activePattern.value ?? {}, null, 2)
  patternJsonMode.value = true
  const parsed = tryParseJSON(patternText.value)
  if (parsed.ok && parsed.value != null && typeof parsed.value === 'object') {
    patternParsed.value = parsed.value as PatternNode
    patternError.value = ''
  }
}

// —— 提取段行：增删与行级就地校验（组引用存在性随上半联动） ——

function extractionRowHint(row: ExtractionRow): string {
  if (!row.dimension) return '须选择维度'
  if (row.from === 'capture') {
    if (!row.captureName) return '须选择捕获组'
    if (!declaredNames.value.has(row.captureName)) return '引用的捕获组已不存在'
  }
  if ((row.from === 'constant' || row.from === 'adjudication') && !row.constant.trim()) {
    return '常量来源须填写常量值'
  }
  return ''
}

function extractionRowHasOtherConds(row: ExtractionRow): boolean {
  return hasOtherConds(row.when)
}

function addExtractionRow(): void {
  extractionRows.value.push({
    dimension: 'author',
    from: 'capture',
    captureName: '',
    constant: '',
    roleName: '',
    stripPrefix: '',
    stripSuffix: '',
    seqOnly: false,
    when: null
  })
}

function removeExtractionRow(index: number): void {
  extractionRows.value.splice(index, 1)
}

// 提取段 JSON 高级模式（复杂 when 树直通原文）：进入=行序列化（seqOnly 已并入），
// 切回=解析重建行（seqOnly 从 when 重新派生，往返不丢）
function toggleExtractionsJson(): void {
  if (extractionsJsonMode.value) {
    const parsed = tryParseJSON(extractionsText.value)
    if (!parsed.ok) {
      extractionsError.value = parsed.error || '解析失败'
      return
    }
    if (!Array.isArray(parsed.value)) {
      extractionsError.value = '顶层须为 JSON 数组'
      return
    }
    extractionRows.value = (parsed.value as ExtractionActionJSON[]).map(actionToRow)
    extractionsJsonMode.value = false
    extractionsError.value = ''
    return
  }
  extractionsText.value = JSON.stringify(rowsToActions(), null, 2)
  extractionsJsonMode.value = true
  extractionsError.value = ''
}

// —— setAdjust 模式指派态（剧本E 语义随迁） ——

type Dest = '' | 'author' | 'title' | 'tag' | 'collection' | 'variable' | 'anchor' | 'seq'

const assignMap = ref<Record<string, string>>({})
const memberOnly = ref(true)
let adjustBaseline = ''

function dimLabel(dim: string): string {
  switch (dim) {
    case 'author': return '作者'
    case 'title': return '作品名'
    case 'collection': return '作品集'
    case 'tag': return '标签'
    default: return dim
  }
}

function clueValueText(c: ClueView): string {
  return c.kind === 'dirLevelStripped' ? `${c.label}（剥壳 → ${c.value}）` : c.value
}

// 线索指派可选目的地（按线索类别）——与原调整面同款：序号段→序号/不用；序列
//（setSeqKey，剧本E——值=群键/公共前缀，与文件名段行同权限，指集名即「集名=
// 群键」）→集名/作者/作品名/标签/不用；文件名段→集名组合/可变/作者/作品名/
// 标签/不用；目录名分段→集名组合/可变/作者/不用（可勾多段连续组合）；目录层
//（含剥壳变体）→作者/作品名/标签/目录锚定/不用；库根名/其余→维度/不用。
function destOptionsOf(c: ClueView): { value: Dest; label: string }[] {
  if (c.kind === 'setSeq') {
    return [
      { value: 'seq', label: '序号' },
      { value: '', label: '不用' }
    ]
  }
  if (c.kind === 'setSeqKey') {
    return [
      { value: '', label: '不用' },
      { value: 'collection', label: '集名' },
      { value: 'author', label: '作者' },
      { value: 'title', label: '作品名' },
      { value: 'tag', label: '标签' }
    ]
  }
  if (c.kind === 'setSeg') {
    return [
      { value: '', label: '不用' },
      { value: 'collection', label: '集名组合' },
      { value: 'variable', label: '可变' },
      { value: 'author', label: '作者' },
      { value: 'title', label: '作品名' },
      { value: 'tag', label: '标签' }
    ]
  }
  if (c.kind === 'setDirSeg') {
    return [
      { value: '', label: '不用' },
      { value: 'collection', label: '集名组合' },
      { value: 'variable', label: '可变' },
      { value: 'author', label: '作者' }
    ]
  }
  if (c.kind === 'dirLevel' || c.kind === 'dirLevelStripped') {
    return [
      { value: '', label: '不用' },
      { value: 'author', label: '作者' },
      { value: 'title', label: '作品名' },
      { value: 'tag', label: '标签' },
      { value: 'anchor', label: '目录锚定' }
    ]
  }
  return [
    { value: '', label: '不用' },
    { value: 'author', label: '作者' },
    { value: 'title', label: '作品名' },
    { value: 'tag', label: '标签' }
  ]
}

// 单选目的地互斥：作者/作品名/目录锚定/序号各至多一条线索持有——新指派挤掉
// 旧持有（集名组合/标签/可变允许多条）。
const SINGLE_DESTS: Dest[] = ['author', 'title', 'anchor', 'seq']

function onAssignChange(clueRef: string): void {
  const dest = assignMap.value[clueRef]
  if (!dest || SINGLE_DESTS.indexOf(dest as Dest) < 0) return
  for (const c of (props.suggestion && props.suggestion.clues) || []) {
    if (c.ref !== clueRef && assignMap.value[c.ref] === dest) assignMap.value[c.ref] = ''
  }
}

// 行级就地提示（剧本E 失败路径；保存校验照旧由后端拒绝）：①同一维度（集名）指了
// 互斥多来源（序列/文件名段/目录段混指）→「一个维度只能有一个来源」；②目录段
// 组合不连续 →「目录段组合须为连续段」。
function collectionFamilyOf(c: ClueView): string {
  if (c.kind === 'setSeqKey') return 'seq'
  if (c.kind === 'setDirSeg') return 'dir'
  if (c.kind === 'setSeg') return 'file'
  return 'other'
}

function clueRowHint(c: ClueView): string {
  const sugg = props.suggestion
  if (!sugg || (assignMap.value[c.ref] || '') !== 'collection') return ''
  const holders = (sugg.clues || []).filter((x) => (assignMap.value[x.ref] || '') === 'collection')
  const families = new Set(holders.map(collectionFamilyOf))
  if (families.size > 1) return '一个维度只能有一个来源'
  if (collectionFamilyOf(c) === 'dir') {
    const ords = holders
      .filter((x) => x.kind === 'setDirSeg')
      .map((x) => x.ordinal || 0)
      .sort((a, b) => a - b)
    for (let i = 1; i < ords.length; i++) {
      if (ords[i] !== ords[i - 1] + 1) return '目录段组合须为连续段'
    }
  }
  return ''
}

// 还原默认指派（剧本E 默认跟随识别形态的应答预填值）
function resetAssignDefaults(): void {
  const sugg = props.suggestion
  if (!sugg) return
  const next: Record<string, string> = {}
  for (const c of sugg.clues || []) next[c.ref] = ''
  for (const ref of sugg.defaultCollectionSegRefs || []) next[ref] = 'collection'
  for (const ref of sugg.defaultVariableSegRefs || []) next[ref] = 'variable'
  if (sugg.defaultSeqSegRef) next[sugg.defaultSeqSegRef] = 'seq'
  if (sugg.defaultAuthorClueRef) next[sugg.defaultAuthorClueRef] = 'author'
  assignMap.value = next
  memberOnly.value = sugg.defaultMemberOnly === true
}

// 调整面脏标志（供父组件切换选中目标前的未保存确认）
const adjustDirty = computed((): boolean => {
  if (props.mode !== 'setAdjust') return false
  return JSON.stringify({ a: assignMap.value, m: memberOnly.value }) !== adjustBaseline
})

watch(adjustDirty, (dirty) => emit('dirty-change', dirty))

// —— 打开时取初始值（destroy-on-close：每次打开重建，取消后再开回上次保存态） ——

function initFromProps(): void {
  if (props.mode === 'rule') {
    const rule = props.rule
    descInput.value = (rule && rule.description) || ''
    enabledInput.value = rule ? rule.enabled !== false : true
    const synth = (rule && rule.synthesis) || {}
    roleRows.value = ((synth.roles || []) as { source: string; roleName: string }[]).map((r) => ({
      source: r.source || 'filename',
      roleName: r.roleName || ''
    }))
    dimRows.value = ((synth.dimPolicies || []) as { dimension: string; strategy: string }[]).map((d) => ({
      dimension: d.dimension || 'title',
      strategy: d.strategy || 'union'
    }))
    // 显式复位跨开关存留的编辑态（AutoHeightDialog destroy-on-close 只销毁插槽
    // 内容，本组件 setup 态跨开关存留——每个字段都须重置后再按初值分支）
    patternJsonMode.value = false
    patternText.value = ''
    patternRoot.value = defaultPatternNode('regex')
    if (!rule || rule.isNew) {
      patternRoot.value = cloneJSON(NEW_PATTERN_SAMPLE)
    } else if (rule.pattern != null && typeof rule.pattern === 'object' && typeof (rule.pattern as PatternNode).kind === 'string') {
      patternRoot.value = cloneJSON(rule.pattern) as PatternNode
    } else if (rule.pattern != null) {
      // 历史坏档（pattern 非对象/缺 kind）：原文进 JSON 高级模式，避免结构化
      // 缺省形态静默改写档案内容
      patternJsonMode.value = true
      patternText.value = JSON.stringify(rule.pattern, null, 2)
    }
    patternError.value = ''
    patternParsed.value = null
    if (patternJsonMode.value) {
      const parsed = tryParseJSON(patternText.value)
      if (parsed.ok && parsed.value != null && typeof parsed.value === 'object') {
        patternParsed.value = parsed.value as PatternNode
      } else {
        patternError.value = parsed.ok ? '顶层须为 JSON 对象' : parsed.error || '解析失败'
      }
    }
    extractionsJsonMode.value = false
    extractionsText.value = ''
    const rawExtractions: unknown = !rule || rule.isNew ? NEW_EXTRACTIONS_SAMPLE : rule.extractions
    if (Array.isArray(rawExtractions)) {
      extractionRows.value = (cloneJSON(rawExtractions) as ExtractionActionJSON[]).map(actionToRow)
    } else {
      // 非数组历史坏档：保原文进 JSON 高级模式（缺省=空规则，结构化空行集）
      extractionRows.value = []
      if (rawExtractions != null) {
        extractionsJsonMode.value = true
        extractionsText.value = JSON.stringify(rawExtractions, null, 2)
      }
    }
    extractionsError.value = ''
    return
  }
  assignMap.value = cloneJSON(props.initialAssigns || {})
  memberOnly.value = props.initialMemberOnly === true
  adjustBaseline = JSON.stringify({ a: assignMap.value, m: memberOnly.value })
  emit('dirty-change', false)
}

watch(
  () => props.state,
  (open) => {
    if (open) initFromProps()
  },
  { immediate: true }
)

// —— 保存 / 取消 ——

function cancel(): void {
  emit('update:state', false)
}

function save(): void {
  if (props.mode === 'rule') {
    // JSON 合法性闸门：识别/提取任一停在 JSON 模式且解析不过 → 不关窗不保存
    if (patternJsonMode.value) {
      if (patternError.value || !patternParsed.value) return
    }
    const pattern = patternJsonMode.value ? patternParsed.value : patternRoot.value
    let extractions: unknown
    if (extractionsJsonMode.value) {
      const parsed = tryParseJSON(extractionsText.value)
      if (!parsed.ok) {
        extractionsError.value = parsed.error || '解析失败'
        return
      }
      if (!Array.isArray(parsed.value)) {
        extractionsError.value = '顶层须为 JSON 数组'
        return
      }
      extractionsError.value = ''
      // 组引用校验（JSON 模式聚合呈现）：引用未声明组 → 行号化错误就地列出
      const problems: string[] = []
      ;(parsed.value as ExtractionActionJSON[]).forEach((action, i) => {
        if (action && action.from === 'capture' && action.captureName && !declaredNames.value.has(action.captureName)) {
          problems.push(`第 ${i + 1} 行引用捕获组「${action.captureName}」未在上半声明`)
        }
      })
      if (problems.length) {
        extractionsError.value = problems.join('；')
        return
      }
      extractions = parsed.value
    } else {
      // 结构化行：任一行级校验不过 → 行内已标红，此处拦住不保存
      if (extractionRows.value.some((row) => extractionRowHint(row))) return
      extractions = rowsToActions()
    }
    const rule = props.rule
    emit('save', {
      id: rule && !rule.isNew ? rule.id : '',
      description: descInput.value,
      source: (rule && rule.source) || 'manual',
      enabled: enabledInput.value,
      pattern: pattern,
      extractions: extractions,
      synthesis: {
        dimPolicies: dimRows.value.filter((d) => d.dimension && d.strategy),
        roles: roleRows.value.filter((r) => r.source && r.roleName)
      }
    })
    return
  }
  // setAdjust：任一指派行级提示在场 → 拦住不保存（行内已标红）
  const sugg = props.suggestion
  if (sugg && (sugg.clues || []).some((c) => clueRowHint(c))) return
  emit('save', { assigns: assignMap.value, memberOnly: memberOnly.value })
}
</script>

<style scoped>
/* 两段式规则编辑弹窗（剧本F）：主题令牌呈现（--app-* 家族） */
.li-red-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
  flex-wrap: wrap;
}

.li-red-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-red-header-note {
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-red-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 2px 4px;
}

.li-red-meta {
  display: flex;
  align-items: center;
  gap: 12px;
}

.li-red-meta .el-input {
  flex: 1;
}

.li-red-sec {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  padding: 12px;
}

.li-red-sec-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-red-sec-head .li-red-sec-title {
  margin-right: auto;
}

.li-red-sec-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-sec-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-red-summary {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-red-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-text-secondary);
}

.li-red-error {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-color-danger);
  word-break: break-all;
}

.li-red-label {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* 捕获组声明清单（名称 + 来源正则） */
.li-red-caps {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: 1px dashed var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  padding: 8px 10px;
}

.li-red-cap-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.li-red-cap-name {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
}

.li-red-cap-expr {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: Consolas, 'Courier New', monospace;
}

.li-red-json-area :deep(textarea) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
}

/* 提取段行：主行（维度←来源）+ 副行（剥壳/角色/条件）+ 行级提示 */
.li-red-ex-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  padding: 8px 10px;
}

.li-red-ex-row.is-invalid {
  border-color: var(--app-color-danger);
}

.li-red-ex-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-ex-dim {
  width: 110px;
}

.li-red-arrow {
  color: var(--app-text-secondary);
}

.li-red-ex-from {
  width: 170px;
}

.li-red-ex-capture {
  width: 200px;
}

.li-red-ex-const {
  width: 200px;
}

.li-red-ex-sub {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-ex-strip {
  width: 130px;
}

.li-red-ex-role {
  width: 130px;
}

.li-red-cond {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--app-text-regular);
  cursor: pointer;
}

.li-red-other {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-red-row-hint {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-danger);
  word-break: break-all;
}

/* 合成策略（随旧表单随迁） */
.li-red-synth-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-red-synth-head > span {
  margin-right: auto;
}

.li-red-inline-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-red-inline-select {
  width: 140px;
}

.li-red-inline-input {
  flex: 1;
}

/* setAdjust：草稿映射与表达式呈现 */
.li-red-maps {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-red-map {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 12px;
}

.li-red-map-dim {
  color: var(--app-text-primary);
  font-weight: 600;
}

.li-red-map-from {
  color: var(--app-text-secondary);
}

.li-red-drafts {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-red-draft {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.li-red-draft-id {
  color: var(--app-color-primary);
  word-break: break-all;
}

.li-red-draft-desc {
  color: var(--app-text-secondary);
  word-break: break-all;
}

/* setAdjust：线索指派行（剧本E） */
.li-red-clue-table {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-red-clue-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex-wrap: wrap;
}

.li-red-clue-hint {
  flex-basis: 100%;
  font-size: 12px;
  color: var(--app-color-danger);
  word-break: break-all;
}

.li-red-clue-label {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-red-clue-value {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.li-red-clue-select {
  flex-shrink: 0;
  max-width: 110px;
  font-size: 12px;
  color: var(--app-text-regular);
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 1px 4px;
}

/* 底部动作区：保存错误文案（父组件回填）+ 动作按钮 */
.li-red-footer {
  display: flex;
  align-items: center;
  gap: 10px;
}

.li-red-save-error {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-color-danger);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.li-red-footer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
</style>
