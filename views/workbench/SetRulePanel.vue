<template>
  <div class="li-sr">
    <!-- 面板头：集名 + 返回（退出集面板回空态；未保存指派时走父级确认） -->
    <div class="li-sr-head">
      <span class="li-sr-title">集规则泛化「{{ setName }}」</span>
      <button type="button" class="li-sr-back" @click="emit('exit')">返回 »</button>
    </div>

    <!-- 剧本C 步1：进入面板即推导 -->
    <p v-if="loading" class="li-sr-loading">推导中…</p>

    <!-- 剧本C 步5c：集不在已归集清单内（可能已被识别结构变化移除）——面板不动 -->
    <el-alert
      v-else-if="loadError"
      class="li-sr-error"
      type="error"
      :closable="false"
      show-icon
      :title="loadError"
    />

    <template v-else-if="suggestion">
      <!-- 步1 集卡：集名 + N 成员；成员列表点行跳该文件的表单（#N 高亮） -->
      <div class="li-sr-sec">
        <p class="li-sr-line">
          作品集「{{ suggestion.setName }}」 · {{ suggestion.memberCount }} 成员
          <span class="li-sr-note">泛化主体=本集整体（多成员共同形态），非单个样本</span>
        </p>
        <div class="li-sr-members">
          <button
            v-for="m in suggestion.members"
            :key="m.relPath"
            type="button"
            class="li-sr-member"
            :title="`打开 ${m.relPath} 的文件表单`"
            @click="emit('selectMember', m.relPath)"
          >
            <span v-if="m.seqText" class="li-sr-member-seq">#{{ m.seqText }}</span>
            <span class="li-sr-member-name">{{ m.fileName }}</span>
          </button>
        </div>
      </div>

      <!-- 步3 结果态：泛化/调整保存成功后的生成规则与差异（镜像 ParsePanel 建议卡形态） -->
      <div v-if="genResult" class="li-sr-sec">
        <h4 class="li-sr-sec-title">
          生成规则与差异
          <span class="li-sr-sec-note">变更 {{ diffChangedCount }} · 新增 {{ diffAddedCount }} · 消失 {{ diffRemovedCount }}</span>
        </h4>
        <p class="li-sr-hint">已生成规则 {{ firstGenId }}；验证轮重识别已启动</p>
        <div v-for="r in genRules" :key="r.id" class="li-sr-gen">
          <p class="li-sr-gen-line"><span class="li-sr-gen-id">{{ r.id }}</span>{{ r.description }}</p>
          <p v-if="r.pattern" class="li-sr-gen-pattern">{{ r.pattern }}</p>
        </div>
        <p v-if="diffTotal" class="li-sr-hint">生成规则后自动增量重识别——过窄＝差异里仍有未覆盖，过泛＝差异里出现错套，请核对</p>
        <div v-if="diffEntries.length" class="li-sr-difflist">
          <div v-for="e in diffEntries" :key="`${e.kind}/${e.relPath}`" class="li-sr-diff" :class="`is-${e.kind}`">
            <span class="li-sr-diff-kind">{{ diffKindLabel(e.kind) }}</span>
            <span class="li-sr-diff-path" :title="e.relPath">{{ e.relPath }}</span>
          </div>
          <p v-if="diffTotal > DIFF_CAP" class="li-sr-hint">仅显示前 {{ DIFF_CAP }} 条，完整差异见左栏差异条</p>
        </div>
      </div>

      <!-- 步1 失败态（5a）：如实呈现推导不出的原因；泛化置灰、调整可用 -->
      <div v-if="suggestion.failed && !genResult" class="li-sr-sec">
        <el-alert
          class="li-sr-fail"
          type="warning"
          :closable="false"
          show-icon
          :title="suggestion.failReason"
          description="自动推导未成——可在下方「调整规则」中手动指派线索后保存规则。"
        />
      </div>

      <!-- 步1 规则草稿（成功态；泛化/保存成功后由结果态接替）：人话摘要 + 逐维度
           映射 + 表达式 + 仅序列成员产出已勾 -->
      <div v-else-if="!genResult" class="li-sr-sec">
        <h4 class="li-sr-sec-title">
          规则草稿
          <span class="li-sr-sec-note">集整体推导 · 未入档</span>
        </h4>
        <p class="li-sr-shape">共同形态：{{ suggestion.shape }}</p>
        <p class="li-sr-summary">{{ suggestion.summary }}</p>
        <div v-if="suggestion.mappings && suggestion.mappings.length" class="li-sr-maps">
          <div v-for="(m, i) in suggestion.mappings" :key="i" class="li-sr-map">
            <span class="li-sr-map-dim">{{ dimLabel(m.dimension) }}</span>
            <span class="li-sr-arrow">←</span>
            <span class="li-sr-map-from">{{ m.from }}</span>
          </div>
        </div>
        <div v-if="suggestion.rules && suggestion.rules.length" class="li-sr-exprs">
          <p v-for="r in suggestion.rules" :key="r.id" class="li-sr-expr">
            <span class="li-sr-expr-id">{{ r.id }}</span>
            <span class="li-sr-expr-pattern">{{ r.pattern }}</span>
          </p>
        </div>
        <label class="li-sr-cond">
          <input type="checkbox" :checked="!!suggestion.memberOnly" disabled>
          <span>仅序列成员产出</span>
        </label>
        <!-- 命中预览：全库口径（含本集成员），样例偏好跨目录 -->
        <p class="li-sr-hint">
          命中预览：将命中全库 {{ suggestion.hitCount }} 个文件（全库共 {{ suggestion.hitTotal }} 个）
        </p>
        <div v-if="suggestion.samples && suggestion.samples.length" class="li-sr-samples">
          <div v-for="(s, i) in suggestion.samples" :key="i" class="li-sr-sample">
            <span class="li-sr-sample-file" :title="s.dirRelKey ? `${s.dirRelKey}/${s.fileName}` : s.fileName">{{ s.dirRelKey ? `${s.dirRelKey}/` : '' }}{{ s.fileName }}</span>
            <span class="li-sr-arrow">→</span>
            <span class="li-sr-sample-result">{{ s.result }}</span>
          </div>
        </div>
      </div>

      <!-- 步5b 半程态：入档写失败——重试泛化 / 放弃，草稿不丢 -->
      <div v-if="halfState" class="li-sr-half">
        <p class="li-sr-half-title">泛化未完成（规则未入档）</p>
        <p class="li-sr-half-reason">{{ halfState.reason }}</p>
        <div class="li-sr-row">
          <el-button type="primary" size="small" :loading="generalizing" @click="doGeneralize">重试泛化</el-button>
          <el-button size="small" @click="halfState = null">放弃</el-button>
        </div>
      </div>

      <!-- 步1 动作行：泛化成规则 / 调整规则（失败态泛化置灰、调整可用） -->
      <div v-if="!genResult" class="li-sr-row li-sr-actions">
        <el-button
          type="primary"
          size="small"
          :disabled="suggestion.failed"
          :loading="generalizing"
          @click="doGeneralize"
        >{{ generalizing ? '泛化中…' : '泛化成规则' }}</el-button>
        <el-button size="small" @click="adjustDialogOpen = true">调整规则</el-button>
      </div>
    </template>

    <!-- 步4 调整规则（剧本F 两段式弹窗 · 共用组件 RuleEditDialog——与规则管理子页
         「编辑/新建」同一弹窗）：上半=草稿识别形态（只读呈现）、下半=指派行；剧本E
         全部语义随迁（线索三族来源/默认指派预填/目录段多段连续组合/互斥行级提示/
         仅序列成员/还原默认指派）；保存走既有 saveWorkSetRuleAdjust RPC，效果与
         原调整面一致；取消不保存、再开回上次保存态 -->
    <RuleEditDialog
      v-model:state="adjustDialogOpen"
      mode="setAdjust"
      :suggestion="suggestion"
      :initial-assigns="assigns"
      :initial-member-only="memberOnly"
      :saving="savingAdjust"
      :save-error="adjustError"
      @save="onAdjustSave"
      @dirty-change="onAdjustDirty"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import RuleEditDialog from './RuleEditDialog.vue'

// 集规则面板（迭代三 · 剧本C + 迭代四 · 剧本D + 迭代五 · 剧本E）：已归集
// 集卡「泛化集规则」进入——泛化主体=集整体（后端对 confirmed 分仓卡按 target
// 寻址推导多成员共同形态），不再绕道集内单个作品的表单。面板态机：
//   推导中… → 草稿态（集卡+成员列表+共同形态+规则草稿〔人话/表达式/仅序列
//   成员产出已勾——文件名共同形态优先；无共同形态时给目录格式草稿（剧本D：
//   第 N 层目录段格式→作品集=末非数字段、每目录各自成集，默认不勾仅序列
//   成员）〕+命中预览〔全库含本集成员，样例偏好跨目录〕+泛化成规则/
//   调整规则）→ 泛化中… → 生成规则与差异（验证轮重识别已启动，差异随 props）；
//   失败三分支——a) 推导失败如实呈现原因（含「无法从目录名推导稳定格式
//   （原因）」；泛化置灰、调整可用）；b) 入档写失败半程态（重试泛化/放弃，
//   草稿不丢）；c) 集不在已归集清单内（面板不动）。
// 调整规则（步4，剧本F 起为两段式弹窗——与规则管理子页「编辑/新建」共用
// RuleEditDialog 组件）：上半=草稿识别形态（只读呈现）、下半=指派行。线索清单
// （目录各层〔含剥壳变体〕+集所在目录名分段〔setDirSegN——可勾多段连续组合〕
// +序列〔seqKey，剧本E——序列形态在场才列，值=群键/公共前缀〕+成员文件名段
// +序号段+库根名）逐项指派（维度←线索/集名捕获段组合〔序列单指｜文件名段/
// 目录段连续组合，三族互斥〕/可变/目录锚定/不用；末段序号默认已指；集名默认
// 跟随识别形态——序列形态=序列行、目录名形态=目录标题段，默认指派呈现跟随应
// 答）→ 保存规则（入档+验证轮，与泛化同终点——走既有 saveWorkSetRuleAdjust，
// 效果与原内联调整面一致）；同维度互斥多来源/目录段不连续行级就地提示（保存
// 校验照旧后端拒绝）。弹窗取消不保存、再开回上次保存态（面板 assigns/memberOnly
// 持已应用指派作弹窗初值）。请求经父组件 ParseView 的唯一 workbench 请求通道
// （request prop 注入）；成员点行 emit selectMember 由父组件切到该文件的表单
// （集面板让出）。

interface SetRuleMember {
  relPath: string
  fileName: string
  seqText?: string
}

interface SetRuleClue {
  ref: string
  kind: string
  label: string
  value: string
  ordinal?: number
  shelled?: boolean
  inner?: string
}

interface SetRuleGenRule {
  id: string
  description: string
  pattern?: string
}

interface SetRuleSuggestion {
  target: string
  setName: string
  memberCount: number
  members: SetRuleMember[]
  shape?: string
  summary?: string
  mappings?: { dimension: string; from: string }[]
  rules?: SetRuleGenRule[]
  memberOnly?: boolean
  hitCount?: number
  hitTotal?: number
  samples?: { fileName: string; dirRelKey?: string; result: string }[]
  failed?: boolean
  failReason?: string
  clues?: SetRuleClue[]
  defaultCollectionSegRefs?: string[]
  defaultVariableSegRefs?: string[]
  defaultSeqSegRef?: string
  defaultMemberOnly?: boolean
  defaultAuthorClueRef?: string
}

interface SetRuleDiffConclusion {
  state?: string
  final?: { dimension: string; value: string; source: string }[]
  pendingDims?: string[]
}

interface SetRuleDiffEntry {
  relPath: string
  before?: SetRuleDiffConclusion
  after?: SetRuleDiffConclusion
}

interface SetRuleDiff {
  changed?: SetRuleDiffEntry[]
  added?: SetRuleDiffEntry[]
  removed?: SetRuleDiffEntry[]
}

// 应答窄投影（父通道应答结构的子集）
interface SetRuleResponse {
  ok: boolean
  error?: string
  started?: boolean
  rule?: SetRuleGenRule
  rules?: SetRuleGenRule[]
  setRuleSuggestion?: SetRuleSuggestion
}

const props = defineProps<{
  root: string
  // 集寻址（与后端 target() 同式：source+NUL+dir+NUL+key）
  target: string
  // 面板头集名兜底（卡面确认集名；推导应答到达后以应答为准）
  setName: string
  request: (action: string, payload: Record<string, unknown>) => Promise<SetRuleResponse>
  diff: SetRuleDiff | null
  recognizing: boolean
}>()

const emit = defineEmits<{
  // 泛化/保存成功后重取全景+决策态（父组件既有通路）
  (e: 'refresh'): void
  // 成员点行：父组件切到该文件的表单（集面板让出）
  (e: 'selectMember', relPath: string): void
  // 面板头「返回」：让出集面板（父组件清 setPanelTarget；未保存指派先确认）
  (e: 'exit'): void
}>()

const DIFF_CAP = 6

const loading = ref(true)
const loadError = ref('')
const suggestion = ref<SetRuleSuggestion | null>(null)
const generalizing = ref(false)
const savingAdjust = ref(false)
// 调整规则弹窗（剧本F 两段式共用组件）开关态；指派编辑态在弹窗内部（取消即弃、
// 再开回上次保存态——initialAssigns 取面板当前已应用指派）
const adjustDialogOpen = ref(false)
// 弹窗内指派改动标志（dirty-change 上行）：供父组件切换选中目标前的未保存确认
const adjustDirty = ref(false)
const adjustError = ref('')
const halfState = ref<{ reason: string } | null>(null)
// 泛化/调整保存成功后的生成结果态（生成规则与差异；面板保持挂载——动作行
// 隐藏、结果与差异常驻，验证轮进度由父组件 recognizing 呈现于面板外）
const genResult = ref<{ rule?: SetRuleGenRule; rules?: SetRuleGenRule[] } | null>(null)

const genRules = computed<SetRuleGenRule[]>(() => {
  const r = genResult.value
  if (!r) return []
  if (r.rules && r.rules.length) return r.rules
  return r.rule ? [r.rule] : []
})
const firstGenId = computed<string>(() => {
  const first = genRules.value[0]
  return first ? first.id : ''
})

// —— 调整面指派态（步4）：线索 ref → 指派目的地 ——
// ''=不用；author/title/tag=维度（author/title/anchor 单选互斥，tag 可多条，
// 集 README 组序即标签序）；collection=集名捕获段组合（可多选=连续段组合）；
// variable=可变段；seq=序号（仅序号段线索可指，默认已指）；anchor=目录锚定
//（仅目录层线索可指）。行内指派交互与校验（剧本E 语义）由 RuleEditDialog 承载，
// 面板只持「当前已应用指派」作为弹窗初值与保存落点。
const assigns = ref<Record<string, string>>({})
const memberOnly = ref(true)
let assignsBaseline = ''

function assignStateText(): string {
  const parts: string[] = [memberOnly.value ? '1' : '0']
  for (const c of (suggestion.value && suggestion.value.clues) || []) {
    parts.push(`${c.ref}=${assigns.value[c.ref] || ''}`)
  }
  return parts.join('|')
}

function applyDefaultAssigns(): void {
  const sugg = suggestion.value
  if (!sugg) return
  const next: Record<string, string> = {}
  for (const c of sugg.clues || []) next[c.ref] = ''
  for (const ref of sugg.defaultCollectionSegRefs || []) next[ref] = 'collection'
  for (const ref of sugg.defaultVariableSegRefs || []) next[ref] = 'variable'
  if (sugg.defaultSeqSegRef) next[sugg.defaultSeqSegRef] = 'seq'
  // 目录格式草稿的默认作者指派（一级目录 Artist 壳在场时=dirLevelStripped1）
  if (sugg.defaultAuthorClueRef) next[sugg.defaultAuthorClueRef] = 'author'
  assigns.value = next
  // 目录格式草稿不带「仅序列成员产出」默认（集的载体是目录名格式）——严格
  // 按应答携带值取 true，缺席即不勾。
  memberOnly.value = sugg.defaultMemberOnly === true
  assignsBaseline = assignStateText()
}

// 弹窗内指派改动上行（dirty-change）：关窗（取消/保存后）即复位
function onAdjustDirty(dirty: boolean): void {
  adjustDirty.value = dirty
}

watch(adjustDialogOpen, (open) => {
  if (!open) adjustDirty.value = false
})

// 供父组件在切换选中目标前读取：调整面是否有未保存指派改动
defineExpose({
  isFormDirty: (): boolean => adjustDialogOpen.value && adjustDirty.value
})

function dimLabel(dim: string): string {
  switch (dim) {
    case 'author': return '作者'
    case 'title': return '作品名'
    case 'collection': return '作品集'
    case 'tag': return '标签'
    default: return dim
  }
}

function diffKindLabel(kind: string): string {
  switch (kind) {
    case 'changed': return '变更'
    case 'added': return '新增'
    case 'removed': return '消失'
    default: return kind
  }
}

const diffEntries = computed(() => {
  const d = props.diff
  if (!d) return []
  const out: { kind: string; relPath: string; before?: SetRuleDiffConclusion; after?: SetRuleDiffConclusion }[] = []
  for (const e of d.changed || []) out.push({ kind: 'changed', ...e })
  for (const e of d.added || []) out.push({ kind: 'added', ...e })
  for (const e of d.removed || []) out.push({ kind: 'removed', ...e })
  return out.slice(0, DIFF_CAP)
})
const diffTotal = computed(() => {
  const d = props.diff
  if (!d) return 0
  return (d.changed ? d.changed.length : 0) + (d.added ? d.added.length : 0) + (d.removed ? d.removed.length : 0)
})
const diffChangedCount = computed(() => (props.diff && props.diff.changed ? props.diff.changed.length : 0))
const diffAddedCount = computed(() => (props.diff && props.diff.added ? props.diff.added.length : 0))
const diffRemovedCount = computed(() => (props.diff && props.diff.removed ? props.diff.removed.length : 0))

// —— 动作 ——

// 步1：进入面板即推导（失败态/5c 错误态均在此分叉）
async function loadSuggestion(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const resp = await props.request('summarizeWorkSetRule', { root: props.root, target: props.target })
    if (!resp.ok) {
      loadError.value = resp.error || '集规则草稿推导失败'
      return
    }
    if (!resp.setRuleSuggestion) {
      loadError.value = '插件应答缺少集规则草稿'
      return
    }
    suggestion.value = resp.setRuleSuggestion
    applyDefaultAssigns()
  } finally {
    loading.value = false
  }
}

// 步3：泛化成规则（草稿即所见即所入——后端复跑同一推导）；失败走半程态（5b）
async function doGeneralize(): Promise<void> {
  if (!props.root || generalizing.value) return
  generalizing.value = true
  halfState.value = null
  try {
    const resp = await props.request('generalizeWorkSetRule', { root: props.root, target: props.target })
    if (!resp.ok) {
      halfState.value = { reason: resp.error || '泛化入档失败' }
      return
    }
    genResult.value = { rule: resp.rule, rules: resp.rules }
    emit('refresh')
  } finally {
    generalizing.value = false
  }
}

// 步4：调整规则弹窗保存（弹窗上行指派结果→既有 saveWorkSetRuleAdjust 组装入
// 档，效果与原调整面一致；失败文案回填弹窗内呈现、指派不丢不关窗）
async function onAdjustSave(payload: { assigns: Record<string, string>; memberOnly: boolean }): Promise<void> {
  const sugg = suggestion.value
  if (!sugg || !props.root || savingAdjust.value) return
  savingAdjust.value = true
  adjustError.value = ''
  const savedAssigns = payload.assigns
  const savedMemberOnly = payload.memberOnly
  const byDest = (dest: string): string[] => {
    const refs: string[] = []
    for (const c of sugg.clues || []) {
      if ((savedAssigns[c.ref] || '') === dest) refs.push(c.ref)
    }
    return refs
  }
  try {
    const resp = await props.request('saveWorkSetRuleAdjust', {
      root: props.root,
      target: props.target,
      authorClueRef: byDest('author')[0] || '',
      titleClueRef: byDest('title')[0] || '',
      tagClueRefs: byDest('tag'),
      collectionSegRefs: byDest('collection'),
      variableSegRefs: byDest('variable'),
      seqSegRef: byDest('seq')[0] || '',
      dirAnchorRef: byDest('anchor')[0] || '',
      setRuleMemberOnlyOff: !savedMemberOnly
    })
    if (!resp.ok) {
      adjustError.value = resp.error || '调整规则保存失败'
      return
    }
    // 保存落点：面板指派态更新为已保存值（弹窗再开即以此为初值=上次保存态）
    assigns.value = savedAssigns
    memberOnly.value = savedMemberOnly
    assignsBaseline = assignStateText()
    adjustDialogOpen.value = false
    genResult.value = { rule: resp.rule, rules: resp.rules }
    emit('refresh')
  } finally {
    savingAdjust.value = false
  }
}

onMounted(() => {
  void loadSuggestion()
})
</script>

<style scoped>
/* 集规则面板（剧本C）：主题令牌呈现（--app-* 家族），与 ParsePanel 同构的分节/胶囊/差异样式语汇 */
.li-sr {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.li-sr-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.li-sr-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-sr-back {
  margin-left: auto;
  flex-shrink: 0;
  border: none;
  background: none;
  padding: 0;
  font-size: 12px;
  cursor: pointer;
  color: var(--app-color-primary);
}

.li-sr-loading {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-secondary);
}

.li-sr-sec {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-top: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  padding-top: 10px;
}

.li-sr-sec-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-sr-sec-note {
  margin-left: 6px;
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-sr-line {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-sr-note {
  display: block;
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-sr-members {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 168px;
  overflow-y: auto;
}

.li-sr-member {
  display: flex;
  align-items: baseline;
  gap: 6px;
  border: none;
  background: none;
  padding: 2px 6px;
  text-align: left;
  cursor: pointer;
  border-radius: var(--app-radius);
  color: var(--app-text-regular);
  min-width: 0;
}

.li-sr-member:hover {
  background: var(--app-bg-page, var(--app-bg-surface));
  color: var(--app-color-primary);
}

.li-sr-member-seq {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
}

.li-sr-member-name {
  font-size: 12px;
  word-break: break-all;
}

.li-sr-shape {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-sr-summary {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-regular);
  word-break: break-all;
}

.li-sr-maps {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-sr-map {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 12px;
}

.li-sr-map-dim {
  color: var(--app-text-primary);
  font-weight: 600;
}

.li-sr-map-from {
  color: var(--app-text-secondary);
}

.li-sr-arrow {
  color: var(--app-text-secondary);
}

.li-sr-exprs {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-sr-expr {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.li-sr-expr-id {
  color: var(--app-color-primary);
  word-break: break-all;
}

.li-sr-expr-pattern {
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-sr-cond {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--app-text-regular);
  cursor: pointer;
}

.li-sr-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

.li-sr-samples {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-sr-sample {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 12px;
  min-width: 0;
}

.li-sr-sample-file {
  color: var(--app-text-regular);
  word-break: break-all;
}

.li-sr-sample-result {
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-sr-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.li-sr-actions {
  padding-top: 2px;
}

/* 半程态（5b）：入档写失败——重试/放弃，草稿不丢 */
.li-sr-half {
  border: 1px solid var(--app-tag-warning-bg);
  border-radius: var(--app-radius);
  background: var(--app-tag-warning-bg);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-sr-half-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-tag-warning-text);
}

.li-sr-half-reason {
  margin: 0;
  font-size: 12px;
  color: var(--app-tag-warning-text);
  word-break: break-all;
}

/* 生成结果（步3） */
.li-sr-gen {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-sr-gen-line {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-sr-gen-id {
  color: var(--app-color-primary);
  margin-right: 6px;
  word-break: break-all;
}

.li-sr-gen-pattern {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-sr-difflist {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-sr-diff {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 12px;
}

.li-sr-diff.is-changed .li-sr-diff-kind {
  color: var(--app-color-primary);
}

.li-sr-diff.is-added .li-sr-diff-kind {
  color: var(--app-color-success);
}

.li-sr-diff.is-removed .li-sr-diff-kind {
  color: var(--app-text-secondary);
}

.li-sr-diff-kind {
  flex-shrink: 0;
  font-weight: 600;
}

.li-sr-diff-path {
  color: var(--app-text-regular);
  word-break: break-all;
}

/* 调整面线索表与行级提示样式已随调整规则弹窗迁入 RuleEditDialog（li-red-*） */

.li-sr-fail {
  width: 100%;
}

.li-sr-error {
  width: 100%;
}
</style>
