<template>
  <AutoHeightDialog
    v-model:state="state"
    width="min(780px, 94vw)"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
  >
    <template #header>
      <div class="li-cred-header">
        <span>条件编辑 · {{ headerText }}</span>
      </div>
    </template>

    <div v-if="draft" class="li-cred-body">
      <!-- 新增态首步：条件种类选择（「添加哪种条件？」——文件名/目录名/序列/引用规则；
           选定后进入该种类编辑；编辑态直达内容（下方三步漏斗含目标卡可再切换）） -->
      <template v-if="mode === 'add' && !draft.condition">
        <p class="li-cred-q">添加哪种条件？</p>
        <div class="li-condx-targets">
          <button
            v-for="t in CONDX_TARGETS"
            :key="t.value"
            type="button"
            class="li-condx-card"
            :title="t.note"
            @click="setRowTarget(draft, t.value)"
          >
            <span class="li-condx-card-label">{{ t.label }}</span>
            <span class="li-condx-card-note">{{ t.note }}</span>
          </button>
          <!-- 引用规则卡：仅顶层条件行提供（引用是规则级字段+求值门段，结构上不可能
               成为括号内条件行） -->
          <button
            v-if="allowRefRow"
            type="button"
            class="li-condx-card"
            :title="CONDX_TARGET_REF_NOTE"
            @click="setRowToRef()"
          >
            <span class="li-condx-card-label">引用规则</span>
            <span class="li-condx-card-note">{{ CONDX_TARGET_REF_NOTE }}</span>
          </button>
        </div>
      </template>

      <!-- 三步漏斗（与原行内编辑同款：目标卡/格式模板/词条/例子模板/附加条件/值校验/
           位次角标与漂移提示）+行级「非」——编辑对象为开窗时的深拷贝草稿，确定才回写 -->
      <template v-else>
        <label class="li-condx-neg li-cred-neg">
          <input v-model="draft.negated" type="checkbox">
          <span>非（本行条件取反）</span>
        </label>
        <!-- Step 1 目标四卡（一次点选；第四卡＝W4 引用规则） -->
        <div class="li-condx-targets">
          <button
            v-for="t in CONDX_TARGETS"
            :key="t.value"
            type="button"
            class="li-condx-card"
            :class="{ 'is-active': condx && condx.target === t.value }"
            :title="t.note"
            @click="setRowTarget(draft, t.value)"
          >
            <span class="li-condx-card-label">{{ t.label }}</span>
            <span class="li-condx-card-note">{{ t.note }}</span>
          </button>
          <button
            v-if="allowRefRow"
            type="button"
            class="li-condx-card"
            :title="CONDX_TARGET_REF_NOTE"
            @click="setRowToRef()"
          >
            <span class="li-condx-card-label">引用规则</span>
            <span class="li-condx-card-note">{{ CONDX_TARGET_REF_NOTE }}</span>
          </button>
        </div>
        <template v-if="condx">
          <!-- Step 2 格式模板：目录双问法 + 模板输入 + 词条点选 + 例子模板 -->
          <div class="li-condx-fmt">
            <el-select
              v-if="condx.target === 'dir'"
              :model-value="condx.format.mode"
              size="small"
              class="li-condx-mode"
              @update:model-value="setRowFormatMode($event)"
            >
              <el-option label="整条按模板" value="template" />
              <el-option label="含这个词" value="contains" />
            </el-select>
            <el-input
              v-if="condx.format.mode === 'contains'"
              v-model="condx.format.word"
              size="small"
              placeholder="包含词（纯文字，不含占位词）"
            />
            <textarea
              v-else
              v-model="condx.format.tpl"
              class="li-condx-tpl"
              rows="2"
              spellcheck="false"
              :placeholder="'如 【{作者}】{标题} / {公共段}_{序号}（点下方词条插入光标处）'"
              @blur="onTplSelection($event.target.selectionStart)"
              @keyup="onTplSelection($event.target.selectionStart)"
              @click="onTplSelection($event.target.selectionStart)"
            />
          </div>
          <!-- 模板词元切分预览（位次角标）：同词根多位各自成 chip，位≥2 带
               「第N处」角标；悬浮注记位次与寻址组名、重复段位的多值禁引 -->
          <div v-if="tplTokChips.length" class="li-condx-toks">
            <template v-for="(t, ti) in tplTokChips" :key="ti">
              <span v-if="!t.ph" class="li-condx-tok-lit">{{ t.text }}</span>
              <span
                v-else
                class="li-condx-tok"
                :data-pos="t.occur && t.occur > 1 ? `第${t.occur}处` : null"
                :title="tplTokTitle(t) || undefined"
              >{{ '{' + t.text + '}' }}</span>
            </template>
          </div>
          <div v-if="condx.format.mode === 'template'" class="li-condx-chips">
            <button
              v-for="v in CONDX_VOCAB"
              :key="v.word"
              type="button"
              class="li-condx-chip"
              :title="v.note"
              @click="insertWordAtCursor(v.word)"
            >{{ '{' + v.word + '}' }}</button>
            <span class="li-condx-chips-sep">｜</span>
            <button
              v-for="ex in CONDX_EXAMPLES"
              :key="ex.name"
              type="button"
              :class="['li-condx-chip is-example', { 'is-optional': ex.template.indexOf('?') >= 0 }]"
              :title="`${ex.template}（${ex.description}）`"
              @click="applyExample(ex)"
            >{{ ex.name }}</button>
          </div>
          <!-- Step 3 附加条件（终结谓词，按目标封闭集）+ 值校验 -->
          <div class="li-condx-extra">
            <div class="li-condx-extra-head">
              <span class="li-condx-label">附加条件（终结谓词）</span>
              <el-select
                v-if="rowAttachOptions.length"
                :model-value="''"
                size="small"
                class="li-condx-addsel"
                placeholder="+ 加一条"
                @update:model-value="addRowAttach($event)"
              >
                <el-option v-for="o in rowAttachOptions" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
            </div>
            <div v-for="(a, ai) in condx.attach" :key="ai" class="li-condx-inline">
              <span class="li-condx-inline-label">{{ attachLabelOf(a.kind) }}</span>
              <textarea
                v-if="attachNeeds(a, 'tpl')"
                v-model="a.tpl"
                class="li-condx-minitpl"
                rows="1"
                spellcheck="false"
                placeholder="迷你模板（如 {序号}）"
              />
              <el-input v-if="attachNeeds(a, 'word')" v-model="a.word" size="small" placeholder="包含词" class="li-condx-mini" />
              <el-input-number v-if="attachNeeds(a, 'count')" v-model="a.minCount" :min="1" size="small" controls-position="right" class="li-condx-num" />
              <el-select v-if="attachNeeds(a, 'types')" v-model="a.types" size="small" multiple placeholder="类型" class="li-condx-mini">
                <el-option v-for="t in CONDX_TYPE_OPTIONS" :key="t" :label="t" :value="t" />
              </el-select>
              <el-input v-if="attachNeeds(a, 'value')" v-model="a.value" size="small" placeholder=".jpg" class="li-condx-mini" />
              <el-button size="small" text type="danger" @click="removeAttach(ai)">删</el-button>
            </div>
            <div class="li-condx-extra-head">
              <span class="li-condx-label">值校验（占位词上的断言）</span>
              <el-select
                v-if="condxRowPlaceholders(condx).length"
                :model-value="''"
                size="small"
                class="li-condx-addsel"
                placeholder="+ 加一枚"
                @update:model-value="addRowBadge($event)"
              >
                <el-option v-for="k in CONDX_BADGE_KINDS" :key="k.value" :label="k.label" :value="k.value" />
              </el-select>
            </div>
            <div v-for="(b, bi) in condx.badges" :key="bi" class="li-condx-inline">
              <!-- 词位下拉位次化：取值=位次化组名（行内唯一）、条目按位出
                   （位≥2 显示「词·第N处」）；选中回写词根+位次（位1 不落
                   occur，与内核 omitempty 同口径=存量档案零变化） -->
              <el-select
                :model-value="badgePhKey(b)"
                size="small"
                class="li-condx-mini"
                @update:model-value="setBadgePh(b, $event)"
              >
                <el-option v-for="p in condxRowPlaceholders(condx)" :key="p.group" :label="`{${p.word}}`" :value="p.group" />
              </el-select>
              <span class="li-condx-dot">●</span>
              <el-select :model-value="b.kind" size="small" class="li-condx-mini" @update:model-value="setBadgeKind(b, $event)">
                <el-option v-for="k in CONDX_BADGE_KINDS" :key="k.value" :label="k.label" :value="k.value" />
              </el-select>
              <el-select v-if="badgeNeeds(b, 'dim')" v-model="b.dim" size="small" placeholder="名单维度" class="li-condx-mini">
                <el-option v-for="d in CONDX_BADGE_DIMS" :key="d" :label="dimNameOf(d)" :value="d" />
              </el-select>
              <label v-if="badgeNeeds(b, 'dim')" class="li-condx-fold"><input v-model="b.fold" type="checkbox"><span>形变归一</span></label>
              <template v-if="badgeNeeds(b, 'againstRow')">
                <el-select v-model="b.againstRow" size="small" placeholder="对方行" class="li-condx-mini">
                  <el-option v-for="f in earlierFlatRows()" :key="f.row" :label="`第 ${f.row} 行`" :value="f.row" />
                </el-select>
                <!-- 相等类别的对方词下拉按对方行位次出条目（含「·第N处」） -->
                <el-select
                  :model-value="againstPhKey(b)"
                  size="small"
                  placeholder="对方占位词"
                  class="li-condx-mini"
                  @update:model-value="setBadgeAgainstPh(b, $event)"
                >
                  <el-option v-for="p in againstEntries(b)" :key="p.group" :label="`{${p.word}}`" :value="p.group" />
                </el-select>
              </template>
              <el-input v-if="badgeNeeds(b, 'value')" v-model="b.value" size="small" placeholder="常量值" class="li-condx-mini" />
              <el-button size="small" text type="danger" @click="removeBadge(bi)">删</el-button>
            </div>
            <!-- 位次漂移提示（值校验面）：占位词位次/相等对方词位次落在受
                 影响组名上时报出，请复核指向 -->
            <p v-if="badgeDriftText" class="li-ref-notice">{{ badgeDriftText }}</p>
          </div>
          <!-- 位次漂移提示（模板面）：编辑会话内模板变更前后位次表 diff 出
               「位次号变化/指向变化」的组名（主形态=在已有出现之前插入同
               词根新出现→后续位次整体 +1），报出受影响词位 -->
          <p v-if="tplDriftText" class="li-ref-notice">{{ tplDriftText }}</p>
        </template>
        <!-- 行级校验红字（与保存闸门同函数） -->
        <p v-if="rowHintText" class="li-red-row-hint">{{ rowHintText }}</p>
      </template>
    </div>

    <template #footer>
      <div class="li-cred-actions">
        <el-button @click="doCancel">取消</el-button>
        <el-button type="primary" :disabled="mode === 'add' && !draft?.condition" @click="doConfirm">确定</el-button>
      </div>
    </template>
  </AutoHeightDialog>
</template>

<script setup lang="ts">
// 条件编辑弹窗：规则编辑面卡片总览的逐条件编辑入口（卡片〔编辑〕/〔+ 加条件〕）。
// 承载原条件行内「三步漏斗」全部编辑能力（目标卡/格式模板/词条点选/例子模板/
// 附加条件/值校验/位次角标与漂移提示）+行级「非」。编辑对象=开窗时目标行的深拷贝
// 草稿——〔确定〕经 confirm 事件回写主视图（弹窗→主视图事件链，语义与行内就地
// 编辑等价）、〔取消〕不动（新增态由主视图移除开窗时追加的空行）。行转引用行
// （目标卡「引用规则」）经 toRef 事件上抛主视图处理。壳=主程序全局注册的
// AutoHeightDialog（EntryConfirmDialog 先例——无需 import；单弹窗确定关窗模式
// 沿用已退役的原括号组弹窗），常驻挂载、开关纯由 state 控制。
import { computed, ref, watch } from 'vue'
import {
  DIMENSION_OPTIONS,
  CONDX_TARGETS,
  CONDX_TARGET_REF_NOTE,
  CONDX_VOCAB,
  CONDX_ATTACH_OPTIONS,
  CONDX_TYPE_OPTIONS,
  CONDX_BADGE_KINDS,
  CONDX_BADGE_DIMS,
  CONDX_EXAMPLES,
  cloneJSON,
  condxFlatRows,
  condxRowPlaceholders,
  condxRowAllPlaceholders,
  condxOccurDrift,
  condxPosName,
  condxVocabOf,
  condxTplOccurChips,
  condxRowError,
  type CondxRowForm,
  type CondxForm,
  type CondxBadgeForm,
  type CondxAttachForm,
  type CondxOccEntry,
  type CondxTplTok
} from './ruleFormModel'

const state = defineModel<boolean>('state', { required: true })

const props = withDefaults(
  defineProps<{
    // 编辑目标行（主视图行树内的引用——confirm 回写、cancel 移除均按它定位）
    row: CondxRowForm | null
    // 目标行的全局先序行号（行级校验与值校验对方行候选的口径）
    no: number
    // add=新增态（首步条件种类选择）；edit=编辑态（直达内容）
    mode?: 'edit' | 'add'
    // 全树行表（顶层）：行级校验与值校验对方行候选按全局先序展平（对方行可跨组
    // 引用——全局行号+「须早于本行」约束）
    rootRows?: CondxRowForm[] | null
    // 位次漂移结果（全局行号→受影响组名+显示词，主视图随模板变更刷新）：本行
    // equal 值校验的对方行漂移提示读取；本行模板漂移在弹窗内就地 diff（基线=开窗
    // 快照）
    occurDrift?: Record<number, { group: string; word: string }[]>
    // 是否提供目标卡「引用规则」（顶层条件行=true；括号子组内条件行=false——引用行
    // 不进括号子组）
    allowRefRow?: boolean
  }>(),
  { mode: 'edit', rootRows: null, allowRefRow: true }
)

const emit = defineEmits<{
  // 〔确定〕：草稿回写主视图（主视图按行引用就地写回 negated/condition——行对象
  // 身份不变，行号引用不受影响）
  confirm: [row: CondxRowForm, draft: CondxRowForm]
  // 〔取消〕（含新增态）：主视图移除开窗时追加的空行（仅新增态）
  cancel: [row: CondxRowForm]
  // 行转引用行：主视图把该行从条件行表移入引用行表
  toRef: [row: CondxRowForm]
}>()

// 编辑草稿：开窗时深拷贝目标行，全部编辑只落在草稿上
const draft = ref<CondxRowForm | null>(null)
// 本行位次基线（开窗快照）：弹窗内模板漂移 diff 的「前」——弹窗期间行树不动，
// 基线与主视图位次快照中该行的条目一致
let baseEntries: CondxOccEntry[] = []

watch(state, (open) => {
  if (!open || !props.row) return
  draft.value = cloneJSON(props.row)
  baseEntries = draft.value.condition ? condxRowAllPlaceholders(draft.value.condition) : []
})

// 草稿条件对象（模板绑定用；目标切换/清空跟随草稿）
const condx = computed<CondxForm | null>(() => draft.value?.condition ?? null)

// 全局先序展平（行级校验与值校验对方行候选共用）：rootRows=顶层行表（对方行可
// 跨组引用——全局先序行号+「须早于本行」约束）
const flatRows = computed(() => condxFlatRows(props.rootRows || (props.row ? [props.row] : [])))

// 弹窗头文案：`N [目标词]`（未选目标时仅序号）
const headerText = computed(() => {
  const c = condx.value
  const target = c ? CONDX_TARGETS.find((t) => t.value === c.target) : undefined
  return target ? `${props.no} [${target.label}]` : `${props.no}`
})

// 行级校验红字（与保存闸门同函数；空行报「行内容不完整」）
const rowHintText = computed(() => {
  const c = condx.value
  if (!c) return '行内容不完整'
  return condxRowError(props.no, c, flatRows.value)
})

// 本行模板位次漂移（弹窗内就地 diff：基线=开窗快照 vs 草稿当前位次表）
const ownDrift = computed<{ group: string; word: string }[]>(() => {
  const c = condx.value
  if (!c) return []
  return condxOccurDrift(baseEntries, condxRowAllPlaceholders(c))
})

const tplDriftText = computed(() => {
  if (!ownDrift.value.length) return ''
  const words = ownDrift.value.map((d) => `{${d.word}}`).join('、')
  return `⚠ 模板位次已变化：${words} 的指向可能已漂移（插入同词根新出现会使后续位次整体 +1）——引用它的提取/生效条件/值校验/组合接线行需复核`
})

// 本行值校验的漂移提示：占位词位次（就地 diff）或相等对方词位次（主视图漂移结果）
// 落在受影响组名上。
const badgeDriftText = computed(() => {
  const c = condx.value
  if (!c || !(c.badges || []).length) return ''
  const hits: string[] = []
  for (const b of c.badges || []) {
    const v = condxVocabOf(b.placeholder || '')
    if (v && ownDrift.value.some((d) => d.group === condxPosName(v.group, b.occur || 1))) {
      hits.push(`{${b.placeholder || '?'}${b.occur && b.occur > 1 ? `·第${b.occur}处` : ''}}`)
    }
    if (b.kind === 'equal' && b.againstRow) {
      const av = condxVocabOf(b.againstPh || '')
      const againstDrift = (props.occurDrift || {})[b.againstRow] || []
      if (av && againstDrift.some((d) => d.group === condxPosName(av.group, b.againstOccur || 1))) {
        hits.push(`第 ${b.againstRow} 行 {${b.againstPh || '?'}${b.againstOccur && b.againstOccur > 1 ? `·第${b.againstOccur}处` : ''}}`)
      }
    }
  }
  if (!hits.length) return ''
  return `⚠ 值校验引用的占位词位次已漂移：${hits.join('、')}——请复核指向`
})

// —— 〔确定〕/〔取消〕/转引用行 ——

function doConfirm(): void {
  if (!props.row || !draft.value) return
  // 新增态未选种类：确定按钮已禁用，此处防御
  if (props.mode === 'add' && !draft.value.condition) return
  emit('confirm', props.row, draft.value)
  state.value = false
}

function doCancel(): void {
  if (!props.row) return
  emit('cancel', props.row)
  state.value = false
}

// 行转引用行（目标卡/种类选择「引用规则」）：上抛主视图移入引用行表并关窗
function setRowToRef(): void {
  if (!props.row) return
  emit('toRef', props.row)
  state.value = false
}

// —— 行编辑辅助（目标卡/格式/词条插入/例子模板/附加/值校验——与原行内编辑同款） ——

// 目标三卡点选：首次进入带默认格式（template + 空模板），切换目标保留格式段
function setRowTarget(row: CondxRowForm, target: 'file' | 'dir' | 'seq'): void {
  if (!row.condition) {
    row.condition = { target, format: { mode: 'template', tpl: '' }, badges: [], attach: [] }
    // 序列默认形态一步到位（{公共段}_{序号}）
    if (target === 'seq') row.condition.format.tpl = '{公共段}_{序号}'
    return
  }
  row.condition.target = target
  if (target !== 'dir' && row.condition.format.mode === 'contains') {
    row.condition.format = { mode: 'template', tpl: '' }
  }
  if (target === 'seq' && row.condition.format.mode === 'template' && !(row.condition.format.tpl || '').trim()) {
    row.condition.format.tpl = '{公共段}_{序号}'
  }
  row.condition.attach = (row.condition.attach || []).filter((a) => rowAttachValues(target).indexOf(a.kind) >= 0)
}

function setRowFormatMode(mode: string): void {
  const c = condx.value
  if (!c) return
  c.format = mode === 'contains' ? { mode: 'contains', word: '' } : { mode: 'template', tpl: '' }
}

// 词条点选插入光标处（记录 textarea 选区；无选区追加到末尾）——弹窗内单行编辑，
// 选区不再按行号分键
let tplSel: number | null = null
function onTplSelection(sel: number): void {
  tplSel = sel
}
function insertWordAtCursor(word: string): void {
  const c = condx.value
  if (!c || c.format.mode !== 'template') return
  const tpl = c.format.tpl || ''
  const piece = `{${word}}`
  if (tplSel == null || tplSel < 0 || tplSel > tpl.length) {
    c.format.tpl = tpl + piece
    return
  }
  c.format.tpl = tpl.slice(0, tplSel) + piece + tpl.slice(tplSel)
  tplSel = tplSel + piece.length
}

function applyExample(ex: { template: string; target: 'file' | 'dir' | 'seq' }): void {
  const row = draft.value
  if (!row) return
  setRowTarget(row, ex.target)
  if (row.condition) row.condition.format = { mode: 'template', tpl: ex.template }
}

// 附加谓词（按目标封闭集）
function rowAttachValues(target: string): string[] {
  return (CONDX_ATTACH_OPTIONS[target] || []).map((o) => o.value)
}
const rowAttachOptions = computed<{ value: string; label: string }[]>(() => {
  const c = condx.value
  return (CONDX_ATTACH_OPTIONS[c ? c.target : ''] || []).map((o) => ({ value: o.value, label: o.label }))
})
function attachLabelOf(kind: string): string {
  const c = condx.value
  const hit = (CONDX_ATTACH_OPTIONS[c ? c.target : ''] || []).find((o) => o.value === kind)
  return hit ? hit.label : kind
}
function attachNeeds(a: CondxAttachForm, need: string): boolean {
  const c = condx.value
  const hit = (CONDX_ATTACH_OPTIONS[c ? c.target : ''] || []).find((o) => o.value === a.kind)
  return !!hit && hit.needs.indexOf(need) >= 0
}
function addRowAttach(kind: string): void {
  const c = condx.value
  if (!c) return
  if (!c.attach) c.attach = []
  c.attach.push({ kind })
}
function removeAttach(ai: number): void {
  const c = condx.value
  if (c?.attach) c.attach.splice(ai, 1)
}

// 值校验
function addRowBadge(kind: string): void {
  const c = condx.value
  if (!c) return
  if (!c.badges) c.badges = []
  const first = condxRowPlaceholders(c)[0]
  // 词位缺省=首个可引用位；occur 仅在位≥2 落 JSON（位1 裸组名=存量档案零变化，
  // 与内核 omitempty 同口径）。
  c.badges.push(
    first
      ? { placeholder: first.baseWord, occur: first.occur > 1 ? first.occur : undefined, kind: kind as CondxBadgeForm['kind'] }
      : { placeholder: '', kind: kind as CondxBadgeForm['kind'] }
  )
}
function removeBadge(bi: number): void {
  const c = condx.value
  if (c?.badges) c.badges.splice(bi, 1)
}
// 模板词元切分 chips（位次角标呈现）：解析失败（未闭合 { 等）静默不显示——
// 行级校验已报错。
const tplTokChips = computed<CondxTplTok[]>(() => {
  const c = condx.value
  if (!c || c.format.mode !== 'template') return []
  return condxTplOccurChips(c.format.tpl || '').chips || []
})

// 占位词 chip 的悬浮说明：位≥2 注记位次与寻址组名；重复段位注记多值禁引。
function tplTokTitle(t: CondxTplTok): string {
  if (t.occur && t.occur > 1) {
    return `同词根第 ${t.occur} 处出现：独立产出与引用（寻址组名 ${t.posName}）${t.repeat ? '；重复段词（多值产出）仅提取段可引用' : ''}`
  }
  if (t.repeat) return '重复段词（多值产出）：提取段可引用，暂不能用于生效条件/值校验/组合接线'
  return ''
}

// equal 对方行候选（全局先序行号早于本行的 condition 行——括号子组内编辑时含组外
// 更早行）
function earlierFlatRows(): { row: number; condx: CondxForm }[] {
  return flatRows.value.filter((f) => f.row < props.no)
}
// 值校验词位选择的取值键：位次化组名（行内唯一）；旧档无 occur=位1 裸组名。
function badgePhKey(b: CondxBadgeForm): string {
  const v = condxVocabOf(b.placeholder || '')
  return v ? condxPosName(v.group, b.occur || 1) : b.placeholder || ''
}
function setBadgePh(b: CondxBadgeForm, group: string): void {
  const c = condx.value
  const hit = c ? condxRowPlaceholders(c).find((p) => p.group === group) : undefined
  if (!hit) return
  b.placeholder = hit.baseWord
  b.occur = hit.occur > 1 ? hit.occur : undefined
}
function setBadgeKind(b: CondxBadgeForm, kind: string): void {
  b.kind = kind as CondxBadgeForm['kind']
  b.dim = undefined
  b.fold = undefined
  b.againstRow = undefined
  b.againstPh = undefined
  b.againstOccur = undefined
  b.value = undefined
}
function badgeNeeds(b: CondxBadgeForm, need: string): boolean {
  const hit = CONDX_BADGE_KINDS.find((k) => k.value === b.kind)
  return !!hit && hit.needs.indexOf(need) >= 0
}
// equal 相等类别的对方词下拉：按对方行位次出条目（含「·第N处」）。
function earlierFlatRowsForBadge(b: CondxBadgeForm): CondxForm | null {
  const hit = flatRows.value.find((f) => f.row === b.againstRow)
  return hit ? hit.condx : null
}
function againstEntries(b: CondxBadgeForm): CondxOccEntry[] {
  const hit = earlierFlatRowsForBadge(b)
  return hit ? condxRowPlaceholders(hit) : []
}
function againstPhKey(b: CondxBadgeForm): string {
  const v = condxVocabOf(b.againstPh || '')
  return v ? condxPosName(v.group, b.againstOccur || 1) : b.againstPh || ''
}
function setBadgeAgainstPh(b: CondxBadgeForm, group: string): void {
  const target = earlierFlatRowsForBadge(b)
  const hit = target ? condxRowPlaceholders(target).find((p) => p.group === group) : undefined
  if (!hit) return
  b.againstPh = hit.baseWord
  b.againstOccur = hit.occur > 1 ? hit.occur : undefined
}
function dimNameOf(dim: string): string {
  const hit = DIMENSION_OPTIONS.find((d) => d.value === dim)
  return hit ? hit.label : dim
}
</script>

<style scoped>
/* 条件编辑弹窗（三步漏斗随行内编辑迁入，样式同款随迁） */
.li-cred-header {
  display: flex;
  align-items: center;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-cred-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 4px 0;
}

/* 新增态首步提问（「添加哪种条件？」） */
.li-cred-q {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

/* 行级「非」勾选（卡片上「非」以徽标呈现，开关住在弹窗内） */
.li-cred-neg {
  align-self: flex-start;
}

.li-cred-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.li-condx-neg {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-condx-targets {
  display: flex;
  gap: 8px;
}

.li-condx-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.li-condx-card.is-active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.li-condx-card-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-condx-card-note {
  font-size: 11px;
  color: var(--app-text-secondary);
  line-height: 1.5;
}

.li-condx-fmt {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.li-condx-mode {
  width: 130px;
  flex: none;
}

.li-condx-tpl {
  flex: 1;
  resize: vertical;
  font-family: var(--app-font-mono, monospace);
  font-size: 13px;
  line-height: 1.6;
  padding: 6px 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  background: transparent;
  color: var(--app-text-primary);
}

.li-condx-tpl:focus {
  outline: none;
  border-color: var(--el-color-primary);
}

.li-condx-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

/* 模板词元切分预览（位次角标）：字面段灰字 + 占位词 chip；位≥2 的 chip 带
   「第N处」角标（data-pos，复用可选 chip 的 ::after 角标样式先例）、主色描边 */
.li-condx-toks {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}

.li-condx-tok-lit {
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-condx-tok {
  font-size: 12px;
  padding: 1px 6px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: 10px;
  color: var(--app-text-primary);
}

.li-condx-tok[data-pos] {
  border-color: var(--el-color-primary);
}

.li-condx-tok[data-pos]::after {
  content: attr(data-pos);
  margin-left: 4px;
  padding: 0 3px;
  font-size: 10px;
  line-height: 1.4;
  border-radius: 4px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.li-condx-chip {
  font-size: 12px;
  padding: 2px 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: 10px;
  background: transparent;
  cursor: pointer;
  color: var(--app-text-primary);
}

.li-condx-chip:hover {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}

.li-condx-chip.is-example {
  color: var(--app-text-secondary);
}

/* 可选记号词条（模板含 ? 的示例）：虚化虚线边框 + 淡色「可选」角标 */
.li-condx-chip.is-optional {
  border-style: dashed;
  color: var(--app-text-secondary);
}

.li-condx-chip.is-optional::after {
  content: '可选';
  margin-left: 4px;
  padding: 0 3px;
  font-size: 10px;
  line-height: 1.4;
  border-radius: 4px;
  color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.li-condx-chips-sep {
  color: var(--app-border-color-lighter, var(--app-border-color-light));
}

.li-condx-extra {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-top: 1px dashed var(--app-border-color-lighter, var(--app-border-color-light));
  padding-top: 8px;
}

.li-condx-extra-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-condx-label {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-condx-addsel {
  width: 180px;
}

.li-condx-inline {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-condx-inline-label {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-condx-minitpl {
  width: 160px;
  resize: none;
  font-family: var(--app-font-mono, monospace);
  font-size: 12px;
  padding: 4px 6px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  background: transparent;
  color: var(--app-text-primary);
}

.li-condx-mini {
  width: 150px;
}

.li-condx-num {
  width: 110px;
}

.li-condx-dot {
  color: var(--el-color-primary);
  font-size: 10px;
}

.li-condx-fold {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* 行级校验红字（与主视图共用同名词，两份同值） */
.li-red-row-hint {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-danger);
  word-break: break-all;
}

/* 提示级 ⚠（位次漂移提示；与主视图共用同名词，两份同值） */
.li-ref-notice {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-color-warning, var(--app-text-secondary));
  word-break: break-all;
}
</style>
