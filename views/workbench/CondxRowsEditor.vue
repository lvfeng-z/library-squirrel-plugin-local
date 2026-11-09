<template>
  <div v-for="(row, i) in rows" :key="i" class="li-condx-row" :class="{ 'is-invalid': !!condxRowHint(i) }">
    <div class="li-condx-row-head">
      <!-- 弹窗内行头全局行号小字（帧3/帧4）：「第 N 行」=condxRowNos 全局先序号
           （组行=组内首行序号），与捕获清单「第 N 行·{词}」对齐；顶层不带（baseNo
           缺省） -->
      <span v-if="rowNoLabel(i)" class="li-condx-rowno">{{ rowNoLabel(i) }}</span>
      <el-select v-if="i > 0" v-model="row.connector" size="small" class="li-condx-conn">
        <el-option label="且（都命中）" value="and" />
        <el-option label="或（任一命中）" value="or" />
      </el-select>
      <label class="li-condx-neg"><input v-model="row.negated" type="checkbox"><span>非</span></label>
      <!-- 叶子行「括号」开关：开=本行收进括号子组；空行与深度封顶层（第 3 层）禁用 -->
      <label v-if="!row.group" class="li-condx-neg" :class="{ 'is-locked': bracketLocked(i) }">
        <input type="checkbox" :checked="false" :disabled="bracketLocked(i)" @change="onBracketOn(i)">
        <span>括号</span>
      </label>
      <!-- 深层怪态（>3 层，JSON 手写合法形态）：只读占位一行（剧本三帧4）——完整
           内容请用「高级（JSON）」，可〔删行〕（删整组带确认）；保存原样带过 -->
      <template v-else-if="groupBeyondDepth(i)">
        <span class="li-condx-neg is-locked">—</span>
        <div class="li-condx-groupcard">括号子组（深层嵌套）——完整内容请用「高级（JSON）」</div>
        <el-button size="small" text type="danger" class="li-condx-del" @click="askDeleteGroup(i)">删行</el-button>
      </template>
      <!-- 括号子组紧凑卡：☑括号（单成员可拆、多成员禁用）+成员摘要+〔编辑〕+〔删行〕（删整组带确认） -->
      <template v-else>
        <label class="li-condx-neg" :class="{ 'is-locked': row.group.length !== 1 }">
          <input type="checkbox" :checked="true" :disabled="row.group.length !== 1" @change="onBracketOff(i)">
          <span>括号</span>
        </label>
        <div class="li-condx-groupcard">{{ groupCardText(i) }}</div>
        <el-button size="small" text @click="onEditGroup(row)">编辑</el-button>
        <el-button size="small" text type="danger" class="li-condx-del" @click="askDeleteGroup(i)">删行</el-button>
      </template>
      <el-button v-if="!row.group" size="small" text type="danger" class="li-condx-del" @click="condxRemoveRow(rows, i)">删行</el-button>
    </div>
    <!-- 「括号」开关禁用态提示：深度封顶 / 多成员组减行引导 -->
    <p v-if="bracketLockHint(i)" class="li-ref-notice">{{ bracketLockHint(i) }}</p>
    <template v-if="!row.group">
      <!-- Step 1 目标四卡（一次点选；第四卡＝W4 引用规则，帧3） -->
      <div class="li-condx-targets">
        <button
          v-for="t in CONDX_TARGETS"
          :key="t.value"
          type="button"
          class="li-condx-card"
          :class="{ 'is-active': row.condition && row.condition.target === t.value }"
          :title="t.note"
          @click="setRowTarget(row, t.value)"
        >
          <span class="li-condx-card-label">{{ t.label }}</span>
          <span class="li-condx-card-note">{{ t.note }}</span>
        </button>
        <!-- 引用规则卡：弹窗内复用时隐藏（引用是规则级字段+求值门段，结构上不可能
             成为括号内条件行——决策4核验关闭） -->
        <button
          v-if="allowRefRow"
          type="button"
          class="li-condx-card"
          :title="CONDX_TARGET_REF_NOTE"
          @click="setRowToRef(row)"
        >
          <span class="li-condx-card-label">引用规则</span>
          <span class="li-condx-card-note">{{ CONDX_TARGET_REF_NOTE }}</span>
        </button>
      </div>
      <template v-if="row.condition">
        <!-- Step 2 格式模板：目录双问法 + 模板输入 + 词条点选 + 例子模板 -->
        <div class="li-condx-fmt">
          <el-select
            v-if="row.condition.target === 'dir'"
            :model-value="row.condition.format.mode"
            size="small"
            class="li-condx-mode"
            @update:model-value="setRowFormatMode(row, $event)"
          >
            <el-option label="整条按模板" value="template" />
            <el-option label="含这个词" value="contains" />
          </el-select>
          <el-input
            v-if="row.condition.format.mode === 'contains'"
            v-model="row.condition.format.word"
            size="small"
            placeholder="包含词（纯文字，不含占位词）"
          />
          <textarea
            v-else
            v-model="row.condition.format.tpl"
            class="li-condx-tpl"
            rows="2"
            spellcheck="false"
            :placeholder="'如 【{作者}】{标题} / {公共段}_{序号}（点下方词条插入光标处）'"
            @blur="onTplSelection(i, $event.target.selectionStart)"
            @keyup="onTplSelection(i, $event.target.selectionStart)"
            @click="onTplSelection(i, $event.target.selectionStart)"
          />
        </div>
        <!-- 模板词元切分预览（位次角标）：同词根多位各自成 chip，位≥2 带
             「第N处」角标（data-pos，复用可选 chip 的 ::after 角标样式
             先例）；悬浮注记位次与寻址组名、重复段位的多值禁引 -->
        <div v-if="tplTokChips(row.condition).length" class="li-condx-toks">
          <template v-for="(t, ti) in tplTokChips(row.condition)" :key="ti">
            <span v-if="!t.ph" class="li-condx-tok-lit">{{ t.text }}</span>
            <span
              v-else
              class="li-condx-tok"
              :data-pos="t.occur && t.occur > 1 ? `第${t.occur}处` : null"
              :title="tplTokTitle(t) || undefined"
            >{{ '{' + t.text + '}' }}</span>
          </template>
        </div>
        <div v-if="row.condition.format.mode === 'template'" class="li-condx-chips">
          <button
            v-for="v in CONDX_VOCAB"
            :key="v.word"
            type="button"
            class="li-condx-chip"
            :title="v.note"
            @click="insertWordAtCursor(row, v.word)"
          >{{ '{' + v.word + '}' }}</button>
          <span class="li-condx-chips-sep">｜</span>
          <button
            v-for="ex in CONDX_EXAMPLES"
            :key="ex.name"
            type="button"
            :class="['li-condx-chip is-example', { 'is-optional': ex.template.indexOf('?') >= 0 }]"
            :title="`${ex.template}（${ex.description}）`"
            @click="applyExample(row, ex)"
          >{{ ex.name }}</button>
        </div>
        <!-- Step 3 附加条件（终结谓词，按目标封闭集）+ 值校验 -->
        <div class="li-condx-extra">
          <div class="li-condx-extra-head">
            <span class="li-condx-label">附加条件（终结谓词）</span>
            <el-select
              v-if="rowAttachOptions(row).length"
              :model-value="''"
              size="small"
              class="li-condx-addsel"
              placeholder="+ 加一条"
              @update:model-value="addRowAttach(row, $event)"
            >
              <el-option v-for="o in rowAttachOptions(row)" :key="o.value" :label="o.label" :value="o.value" />
            </el-select>
          </div>
          <div v-for="(a, ai) in row.condition.attach" :key="ai" class="li-condx-inline">
            <span class="li-condx-inline-label">{{ attachLabelOf(row, a.kind) }}</span>
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
            <el-button size="small" text type="danger" @click="row.condition!.attach!.splice(ai, 1)">删</el-button>
          </div>
          <div class="li-condx-extra-head">
            <span class="li-condx-label">值校验（占位词上的断言）</span>
            <el-select
              v-if="condxRowPlaceholders(row.condition).length"
              :model-value="''"
              size="small"
              class="li-condx-addsel"
              placeholder="+ 加一枚"
              @update:model-value="addRowBadge(row, $event)"
            >
              <el-option v-for="k in CONDX_BADGE_KINDS" :key="k.value" :label="k.label" :value="k.value" />
            </el-select>
          </div>
          <div v-for="(b, bi) in row.condition.badges" :key="bi" class="li-condx-inline">
            <!-- 词位下拉位次化：取值=位次化组名（行内唯一）、条目按位出
                 （位≥2 显示「词·第N处」）；选中回写词根+位次（位1 不落
                 occur，与内核 omitempty 同口径=存量档案零变化） -->
            <el-select
              :model-value="badgePhKey(row.condition, b)"
              size="small"
              class="li-condx-mini"
              @update:model-value="setBadgePh(row.condition, b, $event)"
            >
              <el-option v-for="p in condxRowPlaceholders(row.condition)" :key="p.group" :label="`{${p.word}}`" :value="p.group" />
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
                <el-option v-for="f in earlierFlatRows(i)" :key="f.row" :label="`第 ${f.row} 行`" :value="f.row" />
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
            <el-button size="small" text type="danger" @click="row.condition!.badges!.splice(bi, 1)">删</el-button>
          </div>
          <!-- 位次漂移提示（值校验面）：占位词位次/相等对方词位次落在受
               影响组名上时报出，请复核指向 -->
          <p v-if="badgeDriftNotice(i)" class="li-ref-notice">{{ badgeDriftNotice(i) }}</p>
        </div>
        <!-- 位次漂移提示（模板面）：编辑会话内模板变更前后位次表 diff 出
             「位次号变化/指向变化」的组名（主形态=在已有出现之前插入同
             词根新出现→后续位次整体 +1），报出受影响词位 -->
        <p v-if="rowOccurDriftNotice(i)" class="li-ref-notice">{{ rowOccurDriftNotice(i) }}</p>
      </template>
    </template>
    <p v-if="condxRowHint(i)" class="li-red-row-hint">{{ condxRowHint(i) }}</p>
  </div>
  <el-button size="small" text @click="addCondxRow">+ 加一行</el-button>
  <!-- 删整组确认（带确认弹窗）：〔取消〕=什么都不变；〔删除〕=组与组内全部条件行消失 -->
  <AutoHeightDialog
    v-model:state="delGroupState"
    width="min(420px, 92vw)"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
  >
    <div class="li-condx-delbody">{{ delGroupText }}</div>
    <template #footer>
      <div class="li-condx-delrow">
        <el-button @click="delGroupState = false">取消</el-button>
        <el-button type="danger" @click="doDeleteGroup">删除</el-button>
      </div>
    </template>
  </AutoHeightDialog>
</template>

<script setup lang="ts">
// 条件行编辑器：规则编辑弹窗「识别规则」上半的条件行区块（行渲染模板+行级校验+
// 词条点选+位次角标等全部行为）。行表经 v-model:rows 由父组件传入同一 reactive
// 数组——就地编辑、无拷贝（顶层与括号子组弹窗复用同一实现，编辑实时生效）。
// 行转引用行（目标卡「引用规则」）涉引用行表，经 toRef 事件上抛父组件处理；
// 位次漂移快照与全量漂移查询留在父组件（提取行漂移提示共用），行内漂移提示经
// occurDrift prop 读父组件刷新结果。
import { ref } from 'vue'
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
  CONDX_GROUP_MAX_DEPTH,
  condxFlatRows,
  condxRowPlaceholders,
  condxPosName,
  condxVocabOf,
  condxTplOccurChips,
  condxRowError,
  condxGroupBeyondDepth,
  condxGroupBrief,
  condxRowNos,
  condxWrapRow,
  condxUnwrapRow,
  condxRemoveRow,
  condxGroupEditEntry,
  type CondxRowForm,
  type CondxForm,
  type CondxBadgeForm,
  type CondxAttachForm,
  type CondxOccEntry,
  type CondxTplTok
} from './ruleFormModel'

const rows = defineModel<CondxRowForm[]>('rows', { required: true })

// 弹窗内复用形态（顶层=不传 baseNo/rootRows/depth，与 RuleEditDialog 既有接线一致）
const props = withDefaults(
  defineProps<{
    // 位次漂移结果（全局行号→受影响组名+显示词），父组件随模板变更刷新；弹窗内
    // 复用时不传（漂移快照与刷新在 RuleEditDialog，行内漂移提示留顶层呈现）
    occurDrift?: Record<number, { group: string; word: string }[]>
    // 所在括号嵌套层（顶层=1，弹窗内逐层下传）；达到 CONDX_GROUP_MAX_DEPTH 的层内
    // 行不再提供「括号」开关（封顶禁用+提示，剧本二帧3）
    depth?: number
    // 全局行号基数（弹窗内复用时传入）：行头带「第 N 行」小字、紧凑卡成员行号
    // 按全局先序号（condxRowNos/condxGroupBrief 的 base）；顶层缺省不带
    baseNo?: number
    // 全树行表（弹窗内复用时传顶层行表）：行级校验与值校验对方行候选按全局先序
    // 展平（对方行可跨组引用——全局行号+「须早于本行」约束，边界条件表零改动
    // 口径）；顶层缺省=本容器即全树
    rootRows?: CondxRowForm[] | null
    // 是否提供目标卡「引用规则」（缺省提供；弹窗内复用时传 false——引用行不进
    // 括号子组，决策4核验关闭）
    allowRefRow?: boolean
    // 是否把紧凑卡〔编辑〕的组行写入 condxGroupEditEntry 侧信道（缺省写——顶层
    // 编辑器开窗需要；弹窗内复用时传 false，下钻由 CondxGroupDialog 接 editGroup
    // 事件自行 push，不覆写开窗入口的组行引用）
    recordEntry?: boolean
  }>(),
  { allowRefRow: true, rootRows: null, recordEntry: true }
)

const emit = defineEmits<{
  // 行转引用行：父组件把该行从条件行表移入引用行表
  toRef: [row: CondxRowForm]
  // 紧凑卡〔编辑〕上抛（组行同时记入 condxGroupEditEntry 侧信道）：顶层由
  // RuleEditDialog 接住开窗；弹窗内由 CondxGroupDialog 接住下钻切层
  editGroup: [row: CondxRowForm]
}>()

// 结构化模式的行级校验提示
const condxRowHint = (i: number): string => {
  const list = rows.value
  const row = list[i]
  if (!row) return ''
  const flat = flatRowsGlobal()
  if (row.group) return ''
  const hit = flat.find((f) => f.condx === row.condition)
  if (!hit) return '行内容不完整'
  return condxRowError(hit.row, row.condition, flat)
}

// 全局先序展平（行号=全局口径，行级校验与值校验对方行候选共用）：顶层=本容器
// 即全树；弹窗内经 rootRows 传顶层行表（对方行可跨组引用——全局先序行号+「须
// 早于本行」约束自动覆盖，边界条件表零改动口径）。
function flatRowsGlobal(): { row: number; condx: CondxForm }[] {
  return condxFlatRows(props.rootRows || rows.value)
}

// —— 「括号」开关与括号子组紧凑卡（帧1/帧2/帧5、剧本三帧1-帧3） ——

// 叶子行开关禁用态：空行（无可入组的内容）或所在层达到深度封顶（顶层=第 1 层）。
// 组行开关禁用态：多成员（拆括号须先在括号编辑里减到一行）。
function bracketLocked(i: number): boolean {
  const row = rows.value[i]
  if (!row) return true
  if (row.group) return row.group.length !== 1
  return !row.condition || (props.depth || 1) >= CONDX_GROUP_MAX_DEPTH
}

// 开关禁用态提示：封顶层（更深的组合走高级 JSON）/多成员组（减行引导）。
function bracketLockHint(i: number): string {
  const row = rows.value[i]
  if (!row) return ''
  if (row.group && row.group.length > 1) return '↳ 括号内多于一行——在括号编辑里减到一行才能拆'
  if (!row.group && (props.depth || 1) >= CONDX_GROUP_MAX_DEPTH) {
    return '↳ 最多套 3 层——更深的组合请用「高级（JSON）」'
  }
  return ''
}

// 开关「开」：本行收进括号子组（原行去接词入组、组行继承原接词与「非」；外层
// 其余行不动）。
function onBracketOn(i: number): void {
  const row = rows.value[i]
  if (!row || !row.condition) return
  condxWrapRow(row)
}

// 开关「关」（单成员组）：拆括号恢复普通行，内容原样。
function onBracketOff(i: number): void {
  const row = rows.value[i]
  if (!row || !row.group || row.group.length !== 1) return
  condxUnwrapRow(row)
}

// 紧凑卡文案：「括号子组 · N 行：…」——N=组内条件行数（递归计）；概要=单成员
// 内容概要、多成员全局行号以接词相连（帧2/帧5；弹窗内复用时 baseNo 平移到全局
// 行号口径）。
function groupCardText(i: number): string {
  const list = rows.value
  const row = list[i]
  if (!row || !row.group) return ''
  const count = condxFlatRows(row.group).length
  const base = condxFlatRows(list.slice(0, i)).length + (props.baseNo || 0)
  return `括号子组 · ${count} 行：${condxGroupBrief(row.group, base)}`
}

// 弹窗内行头全局行号小字（帧3/帧4）：「第 N 行」——N=condxRowNos 先序号（叶子行=
// 自身序号、组行=组内首行序号）；顶层（baseNo 缺省）不带行号。
function rowNoLabel(i: number): string {
  if (props.baseNo == null) return ''
  const no = condxRowNos(rows.value, props.baseNo)[i]
  return no == null ? '' : `第 ${no} 行`
}

// 深层怪态（>3 层，JSON 手写合法形态）：组内子树放进所在层会超深度封顶——
// 结构化不提供编辑，只读占位一行（剧本三帧4）。
function groupBeyondDepth(i: number): boolean {
  const row = rows.value[i]
  return !!row && !!row.group && condxGroupBeyondDepth(row.group, props.depth || 1)
}

// 紧凑卡〔编辑〕：上抛（弹窗内由 CondxGroupDialog 接住下钻切层——同窗不开新窗，
// 剧本二帧1→帧2）；顶层编辑器（recordEntry）同时把组行与所在行表记入侧信道——
// RuleEditDialog 的 edit-group 接线只开窗不带行，CondxGroupDialog 开窗时经侧信道
// 取组行建导航栈首层。
function onEditGroup(row: CondxRowForm): void {
  if (props.recordEntry) condxGroupEditEntry.value = { row, rows: rows.value }
  emit('editGroup', row)
}

// 删整组（带确认）：确认文案行数=组内条件行数（递归计）；〔取消〕什么都不变。
const delGroupState = ref(false)
const delGroupIndex = ref(-1)
const delGroupText = ref('')

function askDeleteGroup(i: number): void {
  const row = rows.value[i]
  if (!row || !row.group) return
  delGroupText.value = `确定删除整个括号子组（连同括号内 ${condxFlatRows(row.group).length} 行条件）？`
  delGroupIndex.value = i
  delGroupState.value = true
}

function doDeleteGroup(): void {
  const idx = delGroupIndex.value
  // 规整移除（condxRemoveRow）：删 0 位组行后新首行接词清零——残留会被内核拒保存
  if (idx >= 0 && idx < rows.value.length) condxRemoveRow(rows.value, idx)
  delGroupState.value = false
  delGroupIndex.value = -1
}

// —— 行编辑辅助（目标卡/格式/词条插入/例子模板/附加/值校验） ——

function addCondxRow(): void {
  rows.value.push({ connector: rows.value.length ? 'and' : '', negated: false, condition: null, group: null })
}

// 目标三卡点选：首次进入带默认格式（template + 空模板），切换目标保留格式段
function setRowTarget(row: CondxRowForm, target: 'file' | 'dir' | 'seq'): void {
  if (!row.condition) {
    row.condition = { target, format: { mode: 'template', tpl: '' }, badges: [], attach: [] }
    return
  }
  row.condition.target = target
  if (target !== 'dir' && row.condition.format.mode === 'contains') {
    row.condition.format = { mode: 'template', tpl: '' }
  }
  // 序列默认形态一步到位（{公共段}_{序号}）
  if (target === 'seq' && row.condition.format.mode === 'template' && !(row.condition.format.tpl || '').trim()) {
    row.condition.format.tpl = '{公共段}_{序号}'
  }
  row.condition.attach = (row.condition.attach || []).filter((a) => rowAttachValues(target).indexOf(a.kind) >= 0)
}

function setRowFormatMode(row: CondxRowForm, mode: string): void {
  if (!row.condition) return
  row.condition.format = mode === 'contains' ? { mode: 'contains', word: '' } : { mode: 'template', tpl: '' }
}

// 行转引用行（目标卡「引用规则」）：涉引用行表，上抛父组件处理
function setRowToRef(row: CondxRowForm): void {
  emit('toRef', row)
}

// 词条点选插入光标处（记录 textarea 选区；无选区追加到末尾）
const tplSelections = new Map<number, number>()
function onTplSelection(rowIdx: number, sel: number): void {
  tplSelections.set(rowIdx, sel)
}
function insertWordAtCursor(row: CondxRowForm, word: string): void {
  if (!row.condition || row.condition.format.mode !== 'template') return
  const tpl = row.condition.format.tpl || ''
  const idx = rows.value.indexOf(row)
  const pos = tplSelections.get(idx)
  const piece = `{${word}}`
  if (pos == null || pos < 0 || pos > tpl.length) {
    row.condition.format.tpl = tpl + piece
    return
  }
  row.condition.format.tpl = tpl.slice(0, pos) + piece + tpl.slice(pos)
  tplSelections.set(idx, pos + piece.length)
}

function applyExample(row: CondxRowForm, ex: { template: string; target: 'file' | 'dir' | 'seq' }): void {
  setRowTarget(row, ex.target)
  if (row.condition) row.condition.format = { mode: 'template', tpl: ex.template }
}

// 附加谓词（按目标封闭集）
function rowAttachValues(target: string): string[] {
  return (CONDX_ATTACH_OPTIONS[target] || []).map((o) => o.value)
}
function rowAttachOptions(row: CondxRowForm): { value: string; label: string }[] {
  const target = row.condition ? row.condition.target : ''
  return (CONDX_ATTACH_OPTIONS[target] || []).map((o) => ({ value: o.value, label: o.label }))
}
function attachLabelOf(row: CondxRowForm, kind: string): string {
  const target = row.condition ? row.condition.target : ''
  const hit = (CONDX_ATTACH_OPTIONS[target] || []).find((o) => o.value === kind)
  return hit ? hit.label : kind
}
function attachNeeds(a: CondxAttachForm, need: string): boolean {
  const target = (rows.value.find((r) => r.condition && (r.condition.attach || []).indexOf(a) >= 0)?.condition?.target) || ''
  const hit = (CONDX_ATTACH_OPTIONS[target] || []).find((o) => o.value === a.kind)
  return !!hit && hit.needs.indexOf(need) >= 0
}
function addRowAttach(row: CondxRowForm, kind: string): void {
  if (!row.condition) return
  if (!row.condition.attach) row.condition.attach = []
  row.condition.attach.push({ kind })
}

// 值校验
function addRowBadge(row: CondxRowForm, kind: string): void {
  if (!row.condition) return
  if (!row.condition.badges) row.condition.badges = []
  const first = condxRowPlaceholders(row.condition)[0]
  // 词位缺省=首个可引用位；occur 仅在位≥2 落 JSON（位1 裸组名=存量档案零变化，
  // 与内核 omitempty 同口径）。
  row.condition.badges.push(
    first
      ? { placeholder: first.baseWord, occur: first.occur > 1 ? first.occur : undefined, kind: kind as CondxBadgeForm['kind'] }
      : { placeholder: '', kind: kind as CondxBadgeForm['kind'] }
  )
}
// 模板词元切分 chips（位次角标呈现）：解析失败（未闭合 { 等）静默不显示——
// 行级校验已报错。
function tplTokChips(condx: CondxForm): CondxTplTok[] {
  if (!condx || condx.format.mode !== 'template') return []
  return condxTplOccurChips(condx.format.tpl || '').chips || []
}

// 占位词 chip 的悬浮说明：位≥2 注记位次与寻址组名；重复段位注记多值禁引。
function tplTokTitle(t: CondxTplTok): string {
  if (t.occur && t.occur > 1) {
    return `同词根第 ${t.occur} 处出现：独立产出与引用（寻址组名 ${t.posName}）${t.repeat ? '；重复段词（多值产出）仅提取段可引用' : ''}`
  }
  if (t.repeat) return '重复段词（多值产出）：提取段可引用，暂不能用于生效条件/值校验/组合接线'
  return ''
}

// equal 对方行候选（全局先序行号早于本行的 condition 行——弹窗内含组外更早行）
function earlierFlatRows(rowIndex: number): { row: number; condx: CondxForm }[] {
  const list = rows.value
  const flat = flatRowsGlobal()
  const row = list[rowIndex]
  if (!row || !row.condition) return []
  const self = flat.find((f) => f.condx === row.condition)
  if (!self) return []
  return flat.filter((f) => f.row < self.row)
}
// 值校验词位选择的取值键：位次化组名（行内唯一）；旧档无 occur=位1 裸组名。
function badgePhKey(condx: CondxForm, b: CondxBadgeForm): string {
  const v = condxVocabOf(b.placeholder || '')
  return v ? condxPosName(v.group, b.occur || 1) : b.placeholder || ''
}
function setBadgePh(condx: CondxForm, b: CondxBadgeForm, group: string): void {
  const hit = condxRowPlaceholders(condx).find((p) => p.group === group)
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
  const flat = flatRowsGlobal()
  const hit = flat.find((f) => f.row === b.againstRow)
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
  const condx = earlierFlatRowsForBadge(b)
  const hit = condx ? condxRowPlaceholders(condx).find((p) => p.group === group) : undefined
  if (!hit) return
  b.againstPh = hit.baseWord
  b.againstOccur = hit.occur > 1 ? hit.occur : undefined
}
function dimNameOf(dim: string): string {
  const hit = DIMENSION_OPTIONS.find((d) => d.value === dim)
  return hit ? hit.label : dim
}

// —— 位次漂移提示（行内呈现；快照与刷新在父组件，经 occurDrift prop 读结果） ——

// 本行模板的位次漂移提示：受影响词位按行报出，指向复核（提取/生效条件/值校验/
// 组合接线）。
function rowOccurDriftNotice(i: number): string {
  const row = rows.value[i]
  if (!row || !row.condition) return ''
  const flat = flatRowsGlobal()
  const self = flat.find((f) => f.condx === row.condition)
  if (!self) return ''
  const drift = (props.occurDrift || {})[self.row]
  if (!drift || !drift.length) return ''
  const words = drift.map((d) => `{${d.word}}`).join('、')
  return `⚠ 模板位次已变化：${words} 的指向可能已漂移（插入同词根新出现会使后续位次整体 +1）——引用它的提取/生效条件/值校验/组合接线行需复核`
}

// 本行值校验的漂移提示：占位词位次或相等对方词位次落在受影响组名上。
function badgeDriftNotice(i: number): string {
  const row = rows.value[i]
  if (!row || !row.condition || !(row.condition.badges || []).length) return ''
  const flat = flatRowsGlobal()
  const self = flat.find((f) => f.condx === row.condition)
  if (!self) return ''
  const ownDrift = (props.occurDrift || {})[self.row] || []
  const hits: string[] = []
  for (const b of row.condition.badges || []) {
    const v = condxVocabOf(b.placeholder || '')
    if (v && ownDrift.some((d) => d.group === condxPosName(v.group, b.occur || 1))) {
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
}
</script>

<style scoped>
/* 行容器与行头：与父组件引用行区块共用同名词（引用行侧留父级样式，此处条件行
   自带一份同声明——scoped 作用域互不渗透，两份保持同值） */
.li-condx-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  /* 2026-10-28 行区域标识·左缘色条（第三版，用户裁决「换方案」）：识别条件行
     左缘主色 3px 竖条——不动任何底色（行/分区透明复原，行内组件配色环境即原
     白底设计），逐行成块且与提取行（success 绿条）色彩分野；前两版（行级淡底/
     分区淡底+白卡行）均因观感被否 */
  border-left: 3px solid var(--el-color-primary);
  border-radius: var(--app-radius);
  padding: 10px;
}

.li-condx-row.is-invalid {
  border-color: var(--el-color-danger);
}

.li-condx-row-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-condx-conn {
  width: 130px;
}

/* 弹窗内行头全局行号小字（帧3/帧4）：「第 N 行」——次级文字、不换行 */
.li-condx-rowno {
  font-size: 12px;
  color: var(--app-text-secondary);
  white-space: nowrap;
}

.li-condx-neg {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* 「非」/「括号」开关禁用态（封顶层/多成员组）：整体降透明度呈灰 */
.li-condx-neg.is-locked {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 括号子组紧凑卡成员摘要（帧2/帧5）：「括号子组 · N 行：…」单行可换行 */
.li-condx-groupcard {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-text-primary);
  word-break: break-all;
}

/* 删整组确认弹窗：正文与底部按钮行 */
.li-condx-delbody {
  padding: 4px 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-condx-delrow {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.li-condx-del {
  margin-left: auto;
}

/* 行级校验红字（与父组件引用行/提取行共用同名词，两份同值） */
.li-red-row-hint {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-danger);
  word-break: break-all;
}

/* 提示级 ⚠（位次漂移提示；与父组件引用行提示共用同名词，两份同值） */
.li-ref-notice {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-color-warning, var(--app-text-secondary));
  word-break: break-all;
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

/* 可选记号词条（模板含 ? 的示例）：虚化虚线边框 + 淡色「可选」角标——阶段3
   可选 chip 样式（无既有可选类先例，不阻塞语义的小额呈现设计）。 */
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
</style>
