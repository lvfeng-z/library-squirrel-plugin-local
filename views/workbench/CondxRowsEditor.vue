<template>
  <template v-for="(row, i) in rows" :key="i">
    <!-- 叶子行卡（恒定一行高：序号+目标词+模板摘要+复杂度徽标+接词+行操作） -->
    <div v-if="!row.group" class="li-condx-card" :class="{ 'is-invalid': !!cardErrText(i) }">
      <div class="li-condx-card-line">
        <span class="li-condx-card-no">{{ cardNos[i] }}</span>
        <!-- 目标词+模板摘要+复杂度徽标（附加×N/校验×N/非/多值——复杂度只进
             徽标不进正文，总览密度不随规则复杂度退化） -->
        <span class="li-condx-card-target">[{{ cardView(i).targetLabel }}]</span>
        <span class="li-condx-card-tpl" :title="cardView(i).templateText">{{ cardView(i).templateText }}</span>
        <span v-if="cardView(i).levelNote" class="li-condx-card-note">· {{ cardView(i).levelNote }}</span>
        <span v-if="cardView(i).attachCount" class="li-condx-card-note">· 附加×{{ cardView(i).attachCount }}</span>
        <span v-if="cardView(i).badgeCount" class="li-condx-card-note">· 校验×{{ cardView(i).badgeCount }}</span>
        <span v-if="cardView(i).negated" class="li-condx-card-flag">非</span>
        <span v-if="cardView(i).multiValue" class="li-condx-card-flag">多值</span>
        <!-- 位次漂移徽标（详情与复核提示在条件编辑弹窗内呈现） -->
        <span v-if="cardDriftText(i)" class="li-condx-card-flag is-warn" :title="cardDriftText(i)">⚠</span>
        <!-- 行级校验徽标（红边+✗；完整文案在条件编辑弹窗内/保存闸门呈现） -->
        <span v-if="cardErrText(i)" class="li-condx-card-flag is-err" :title="cardErrText(i)">✗</span>
        <!-- 接词（首行无）：与行操作一起收在卡片行尾 -->
        <el-select v-if="i > 0" v-model="row.connector" size="small" class="li-condx-conn">
          <el-option label="且（都命中）" value="and" />
          <el-option label="或（任一命中）" value="or" />
        </el-select>
        <!-- 「括号」开关：开=本行收进括号子组；空行与深度封顶层（第 3 层）禁用
             （禁用态提示经 title 悬浮——卡片高度恒定不落第二行） -->
        <label class="li-condx-neg" :class="{ 'is-locked': bracketLocked(i) }" :title="bracketLockHint(i) || '收进括号子组'">
          <input type="checkbox" :checked="false" :disabled="bracketLocked(i)" @change="onBracketOn(i)">
          <span>括号</span>
        </label>
        <!-- 换位组（靠右成组）：上移/下移图标按钮（SortUp/SortDown，宿主全局注册按名
             直用）——首行上移/末行下移禁用（单行列表双禁用，无提示，BC 同款）；相等门
             阻断方向禁用+悬停说明（文案=condxMoveBlockReason 返回值）；提取区
             「高级（JSON）」模式整体禁用+悬停提示。禁用按钮外包一层 span 接管悬浮 -->
        <span class="li-condx-move">
          <el-tooltip :disabled="!moveTip(i, 'up')" :content="moveTip(i, 'up')" placement="top">
            <span class="li-condx-move-btn">
              <el-button size="small" text :disabled="moveLocked(i, 'up')" aria-label="上移" @click="onMoveRow(i, 'up')">
                <el-icon><SortUp /></el-icon>
              </el-button>
            </span>
          </el-tooltip>
          <el-tooltip :disabled="!moveTip(i, 'down')" :content="moveTip(i, 'down')" placement="top">
            <span class="li-condx-move-btn">
              <el-button size="small" text :disabled="moveLocked(i, 'down')" aria-label="下移" @click="onMoveRow(i, 'down')">
                <el-icon><SortDown /></el-icon>
              </el-button>
            </span>
          </el-tooltip>
        </span>
        <el-divider direction="vertical" class="li-condx-move-sep" />
        <!-- 叶子行〔编辑〕=开条件编辑弹窗（组框内成员卡同一弹窗） -->
        <el-button size="small" text @click="emitEditRow(row, i)">[编辑]</el-button>
        <el-button size="small" text type="danger" class="li-condx-del" @click="condxRemoveRow(rows, i)">删行</el-button>
      </div>
    </div>
    <!-- 深层怪态组卡（>3 层，JSON 手写合法形态）：只读占位一行（可删整组带确认、
         可移动；结构化不提供编辑，保存原样带过） -->
    <div v-else-if="groupBeyondDepth(i)" class="li-condx-card">
      <div class="li-condx-card-line">
        <span class="li-condx-card-no">{{ cardNos[i] }}</span>
        <span class="li-condx-card-target">[括号]</span>
        <span class="li-condx-card-tpl">括号子组（深层嵌套）——完整内容请用「高级（JSON）」</span>
        <el-select v-if="i > 0" v-model="row.connector" size="small" class="li-condx-conn">
          <el-option label="且（都命中）" value="and" />
          <el-option label="或（任一命中）" value="or" />
        </el-select>
        <span class="li-condx-neg is-locked" title="括号子组嵌套超过 3 层——完整内容请用「高级（JSON）」">—</span>
        <span class="li-condx-move">
          <el-tooltip :disabled="!moveTip(i, 'up')" :content="moveTip(i, 'up')" placement="top">
            <span class="li-condx-move-btn">
              <el-button size="small" text :disabled="moveLocked(i, 'up')" aria-label="上移" @click="onMoveRow(i, 'up')">
                <el-icon><SortUp /></el-icon>
              </el-button>
            </span>
          </el-tooltip>
          <el-tooltip :disabled="!moveTip(i, 'down')" :content="moveTip(i, 'down')" placement="top">
            <span class="li-condx-move-btn">
              <el-button size="small" text :disabled="moveLocked(i, 'down')" aria-label="下移" @click="onMoveRow(i, 'down')">
                <el-icon><SortDown /></el-icon>
              </el-button>
            </span>
          </el-tooltip>
        </span>
        <el-divider direction="vertical" class="li-condx-move-sep" />
        <el-button size="small" text type="danger" class="li-condx-del" @click="askDeleteGroup(i)">删行</el-button>
      </div>
    </div>
    <!-- 括号子组组框（页内直显）：组框头=左缘「（」+组内接词语义标注+组级操作
         （接词切换/拆组/换位/删整组）；成员卡缩进于框内、与顶层同款恒高卡；嵌套
         组=组框再缩进一层（封顶 3 层维持，超出走高级 JSON 的只读占位卡） -->
    <div v-else class="li-condx-group">
      <div class="li-condx-group-head">
        <span class="li-condx-group-mark">（</span>
        <span v-if="groupWordText(i)" class="li-condx-group-word" title="括号内成员行间的接词">{{ groupWordText(i) }}</span>
        <span v-if="row.negated" class="li-condx-card-flag" title="括号子组整体取非（「高级（JSON）」模式可改）">非</span>
        <el-select v-if="i > 0" v-model="row.connector" size="small" class="li-condx-conn">
          <el-option label="且（都命中）" value="and" />
          <el-option label="或（任一命中）" value="or" />
        </el-select>
        <!-- 组行「括号」开关：单成员组可拆（☑ 状态）、多成员禁用（拆括号须先减到
             一行——组内成员卡〔删行〕） -->
        <label class="li-condx-neg" :class="{ 'is-locked': row.group.length !== 1 }" :title="bracketLockHint(i) || '拆掉括号恢复普通行'">
          <input type="checkbox" :checked="true" :disabled="row.group.length !== 1" @change="onBracketOff(i)">
          <span>括号</span>
        </label>
        <span class="li-condx-move">
          <el-tooltip :disabled="!moveTip(i, 'up')" :content="moveTip(i, 'up')" placement="top">
            <span class="li-condx-move-btn">
              <el-button size="small" text :disabled="moveLocked(i, 'up')" aria-label="上移" @click="onMoveRow(i, 'up')">
                <el-icon><SortUp /></el-icon>
              </el-button>
            </span>
          </el-tooltip>
          <el-tooltip :disabled="!moveTip(i, 'down')" :content="moveTip(i, 'down')" placement="top">
            <span class="li-condx-move-btn">
              <el-button size="small" text :disabled="moveLocked(i, 'down')" aria-label="下移" @click="onMoveRow(i, 'down')">
                <el-icon><SortDown /></el-icon>
              </el-button>
            </span>
          </el-tooltip>
        </span>
        <el-divider direction="vertical" class="li-condx-move-sep" />
        <el-button size="small" text type="danger" class="li-condx-del" @click="askDeleteGroup(i)">删行</el-button>
      </div>
      <!-- 组内成员：本组件递归（成员卡与顶层同款恒高卡、点〔编辑〕开同一条件编辑
           弹窗）；事件逐层上浮主视图（editRow/moved 同一消费），组内删空经
           emptied 拆掉本组框（链式再拆由各层自行接住） -->
      <CondxRowsEditor
        v-model:rows="row.group"
        :depth="(depth || 1) + 1"
        :base-no="groupBaseNo(i)"
        :root-rows="rootRows || rows"
        :occur-drift="occurDrift"
        :extractions-json-mode="extractionsJsonMode"
        :add-label="addLabel"
        :agg-mode="aggMode"
        @edit-row="(r, no) => emit('editRow', r, no)"
        @moved="emit('moved', $event)"
        @emptied="onGroupEmptied(row)"
      />
    </div>
  </template>
  <div class="li-condx-cards-foot">
    <!-- 新增条件亦经条件编辑弹窗（新增态首步为条件种类选择）——先追加空行再上抛
         editRow（空行在树内=行号/校验上下文真实；取消由主视图移除该行）；
         聚合轨行区经 addLabel 换「+ 加轨」文案（同一弹窗同一链路） -->
    <el-button size="small" text @click="addCondxRow">{{ addLabel }}</el-button>
  </div>
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
// 条件卡片列表（规则编辑面总览）：每条件一行恒定高度摘要卡（序号+目标词+模板摘要+
// 复杂度徽标+接词）。三步漏斗编辑（目标卡/格式模板/词条/附加条件/值校验/漂移提示）
// 整体迁入条件编辑弹窗（CondxRowEditDialog）——叶子卡〔编辑〕/〔+ 加条件〕经
// editRow 事件上抛主视图（RuleEditDialog）开窗。括号子组页内直显：组行渲染为
// 组框（组框头=接词语义标注+组级操作），成员卡由本组件递归渲染（与顶层同款
// 恒高卡，嵌套组=组框再缩进一层、封顶 3 层），无面包屑下钻弹窗。行表经
// v-model:rows 由父组件传入同一 reactive 数组——结构性操作（接词/括号开关/
// 换位/删行）仍在本组件就地生效；组内成员的 editRow/moved 逐层上浮（事件源
// 收敛到主视图，RuleEditDialog 同一消费）。
import { computed, ref, watch } from 'vue'
import {
  CONDX_GROUP_MAX_DEPTH,
  condxFlatRows,
  condxGroupBeyondDepth,
  condxConnLabel,
  condxWrapRow,
  condxUnwrapRow,
  condxRemoveRow,
  condxMoveSwapRows,
  condxMoveRowNoMap,
  condxMoveBlockReason,
  condxRowCardView,
  condxRowError,
  condxRowNos,
  type CondxRowForm,
  type CondxMoveDir,
  type CondxRowNoMap
} from './ruleFormModel'

const rows = defineModel<CondxRowForm[]>('rows', { required: true })

// 组框内复用形态（顶层=不传 baseNo/rootRows/depth，与 RuleEditDialog 既有接线一致）
const props = withDefaults(
  defineProps<{
    // 位次漂移结果（全局行号→受影响组名+显示词），父组件随模板变更刷新；卡片「⚠」
    // 徽标读取（详情在条件编辑弹窗内呈现）
    occurDrift?: Record<number, { group: string; word: string }[]>
    // 所在括号嵌套层（顶层=1，组框内逐层 +1 下传）；达到 CONDX_GROUP_MAX_DEPTH 的层内
    // 行不再提供「括号」开关（封顶禁用+悬浮提示）
    depth?: number
    // 全局行号基数（组框内复用时传入）：卡片序号按全局先序号（condxRowNos 的 base）
    baseNo?: number
    // 全树行表（组框内复用时传顶层行表）：行级校验按全局先序展平（对方行可跨组
    // 引用——全局行号+「须早于本行」约束）；顶层缺省=本容器即全树
    rootRows?: CondxRowForm[] | null
    // 提取区「高级（JSON）」模式（RuleEditDialog 下传、组框内透传）：移动按钮整体
    // 禁用+悬停提示——JSON 文本里的行号引用无法可靠重映射，放行会静默失同步；
    // 组内换位同样要改写主弹窗提取引用，禁用口径与顶层一致
    extractionsJsonMode?: boolean
    // 底部新增按钮文案（聚合轨行区换「+ 加轨」，与元数据规则同一弹窗链路）
    addLabel?: string
    // 聚合轨行区形态：卡片模板文本尾接扩展名谓词取值（模板对去扩展名的文件名
    // 正文求值，轨行整体形态=模板+扩展名，如 {作品}.{任意}+.srt）
    aggMode?: boolean
  }>(),
  { rootRows: null, extractionsJsonMode: false, addLabel: '+ 加条件', aggMode: false }
)

const emit = defineEmits<{
  // 叶子卡〔编辑〕/〔+ 加条件〕上抛（携行对象与全局先序行号）：主视图接住开条件
  // 编辑弹窗；组框内成员卡逐层透传至主视图（同一弹窗）
  editRow: [row: CondxRowForm, no: number]
  // 行换位落地上抛（携「旧行号→新行号」全量映射）：行表已在编辑器内就地内容
  // 交换，RuleEditDialog 接住按映射改写提取行与条件树草稿态的三类行号引用；
  // 组框内换位逐层透传（同一消费）
  moved: [rowNoMap: CondxRowNoMap]
  // 组内行删空（组框内复用形态=depth>1）：渲染组框的父层接住移除组行（自动拆组）
  emptied: []
}>()

// 卡片序号（全局先序行号，空叶子行占号——与捕获清单「第 N 行」同口径；顶层基数 0）
const cardNos = computed(() => condxRowNos(rows.value, props.baseNo ?? 0))

// 卡片摘要字段（纯函数派生：序号+目标词+模板文本截断+复杂度徽标计数+接词）；
// 聚合形态（aggMode）模板文本尾接扩展名谓词取值
function cardView(i: number) {
  const row = rows.value[i]
  return condxRowCardView(row || { condition: null }, cardNos.value[i] ?? i + 1, { appendExt: props.aggMode })
}

// 全局先序展平（行级校验用）：顶层=本容器即全树；组框内经 rootRows 传顶层行表
function flatRowsGlobal(): { row: number; condx: NonNullable<CondxRowForm['condition']> }[] {
  return condxFlatRows(props.rootRows || rows.value)
}

// 行级校验文案（卡片红边+✗ 徽标与保存闸门同函数；完整提示在条件编辑弹窗内呈现）
function cardErrText(i: number): string {
  const row = rows.value[i]
  if (!row || row.group) return ''
  if (!row.condition) return '行内容不完整'
  return condxRowError(cardNos.value[i] ?? i + 1, row.condition, flatRowsGlobal())
}

// 位次漂移徽标文案（详情在条件编辑弹窗内呈现）
function cardDriftText(i: number): string {
  const row = rows.value[i]
  if (!row || row.group) return ''
  const drift = (props.occurDrift || {})[cardNos.value[i] ?? i + 1]
  if (!drift || !drift.length) return ''
  const words = drift.map((d) => `{${d.word}}`).join('、')
  return `⚠ 模板位次已变化：${words} 的指向可能已漂移——点〔编辑〕查看详情与复核指向`
}

// —— 「括号」开关与括号子组组框（页内直显） ——

// 叶子行开关禁用态：空行（无可入组的内容）或所在层达到深度封顶（顶层=第 1 层）。
// 组行开关禁用态：多成员（拆括号须先在组框内减到一行）。
function bracketLocked(i: number): boolean {
  const row = rows.value[i]
  if (!row) return true
  if (row.group) return row.group.length !== 1
  return !row.condition || (props.depth || 1) >= CONDX_GROUP_MAX_DEPTH
}

// 开关禁用态悬浮提示：封顶层（更深的组合走高级 JSON）/多成员组（减行引导）。
function bracketLockHint(i: number): string {
  const row = rows.value[i]
  if (!row) return ''
  if (row.group && row.group.length > 1) return '↳ 括号内多于一行——在组框内减到一行才能拆'
  if (!row.group && (props.depth || 1) >= CONDX_GROUP_MAX_DEPTH) {
    return '↳ 最多套 3 层——更深的组合请用「高级（JSON）」'
  }
  return ''
}

// 开关「开」：本行收进括号子组（原行去接词入组、组行继承原接词；外层其余行不动）。
function onBracketOn(i: number): void {
  const row = rows.value[i]
  if (!row || !row.condition) return
  condxWrapRow(row)
}

// 开关「关」（单成员组）：拆括号恢复普通行，内容原样（行级「非」双重否定相消）。
function onBracketOff(i: number): void {
  const row = rows.value[i]
  if (!row || !row.group || row.group.length !== 1) return
  condxUnwrapRow(row)
}

// 组框头接词语义标注：组内成员行间接词统一时的语义词（且/或）、混合=「且/或」、
// 单成员组无组内接词（空串不渲染）。
function groupWordText(i: number): string {
  const group = rows.value[i]?.group
  if (!group) return ''
  const words = new Set(group.filter((_, idx) => idx > 0).map((m) => condxConnLabel(m.connector)))
  if (!words.size) return ''
  return words.size === 1 ? [...words][0] : '且/或'
}

// 组框内递归编辑器的全局行号基数：组行的全局先序号 - 1（组行行号=组内首行序号，
// 与 condxRowNos 的组行占号口径一致）。
function groupBaseNo(i: number): number {
  return (cardNos.value[i] ?? i + 1) - 1
}

// 组内删空自动拆组：组框内成员删到 0 行（删行/条件编辑弹窗取消移除等任一路径）
// 时移除所在组行（规整移除——组行居 0 位时新首行接词清零）；父层若因此也删空则
// 经 emptied 链式再拆。按行对象引用定位，重复触发时行已不在表内即无操作。
function onGroupEmptied(row: CondxRowForm): void {
  const idx = rows.value.indexOf(row)
  if (idx < 0) return
  const target = rows.value[idx]
  if (!target.group || target.group.length) return
  condxRemoveRow(rows.value, idx)
}

// 组内行表长度监听（组框内复用形态=depth>1）：删空即上抛 emptied（渲染前触发，
// 空组框不落地渲染）；顶层空行表合法（引用行在场组合/保存闸门口径），不上抛。
watch(
  () => rows.value.length,
  (len) => {
    if (len === 0 && (props.depth || 1) > 1) emit('emptied')
  }
)

// 深层怪态（>3 层，JSON 手写合法形态）：组内子树放进所在层会超深度封顶——结构化
// 不提供编辑，卡片只读占位（可删整组带确认、可移动；保存原样带过）。
function groupBeyondDepth(i: number): boolean {
  const row = rows.value[i]
  return !!row && !!row.group && condxGroupBeyondDepth(row.group, props.depth || 1)
}

// —— 行头换位（上移/下移与相邻行内容交换，连接词留位） ——

// 提取区「高级（JSON）」模式的移动禁用提示（固定文案）：JSON 文本里的行号引用
// 无法可靠重映射（手工改写会毁掉用户手排格式），放行则引用静默失同步。
const MOVE_JSON_TIP = '提取区为「高级（JSON）」模式，不能移动——请切回结构化编辑'

// 换位全树（行号映射与相等门判定的基准）：顶层=本容器行表；组框内=rootRows 顶层表
function moveRootRows(): CondxRowForm[] {
  return props.rootRows || rows.value
}

// 换位边界（首行上移/末行下移，含单行列表双向）
function moveAtEdge(i: number, dir: CondxMoveDir): boolean {
  return dir === 'up' ? i <= 0 : i >= rows.value.length - 1
}

// 换位禁用态：① 边界（无提示，BC 同款）；② 相等门阻断方向（模拟换位后「与另一行
// 相等」校验引用的行会晚于本行）；③ 提取区 JSON 模式整体禁用。
function moveLocked(i: number, dir: CondxMoveDir): boolean {
  if (moveAtEdge(i, dir)) return true
  if (props.extractionsJsonMode) return true
  return condxMoveBlockReason(moveRootRows(), rows.value, i, dir) !== ''
}

// 禁用态悬停说明（空串=不弹）：边界禁用自明无提示；相等门=拦截文案；提取区 JSON
// 模式=固定提示。
function moveTip(i: number, dir: CondxMoveDir): string {
  if (moveAtEdge(i, dir)) return ''
  if (props.extractionsJsonMode) return MOVE_JSON_TIP
  return condxMoveBlockReason(moveRootRows(), rows.value, i, dir)
}

// 换位落地：先按换位前形态取「旧行号→新行号」全量映射（调用顺序契约——映射必须在
// SwapRows 落位之前），再内容交换换位（连接词留位、布尔骨架不动）；成功上抛 moved
// 携映射（组框内换位逐层上浮，顶层由 RuleEditDialog 改写提取行与条件树的三类行号
// 引用——组行整块与组内行都由同一映射覆盖）。阻断方向在禁用态已拦，此处再核一次
// 防御。移动不触发保存、无确认弹窗。
function onMoveRow(i: number, dir: CondxMoveDir): void {
  if (moveLocked(i, dir)) return
  const map = condxMoveRowNoMap(moveRootRows(), rows.value, i, dir)
  if (!map) return
  if (!condxMoveSwapRows(rows.value, i, dir)) return
  emit('moved', map)
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

// —— 新增与编辑入口（均经条件编辑弹窗） ——

// 叶子卡〔编辑〕：上抛行对象与全局先序行号（主视图接住开条件编辑弹窗）
function emitEditRow(row: CondxRowForm, i: number): void {
  emit('editRow', row, cardNos.value[i] ?? i + 1)
}

// 〔+ 加条件〕：先追加空行（行号/校验上下文真实），随即上抛 editRow（主视图以
// 新增态开条件编辑弹窗——首步条件种类选择；取消时移除该行）；组框内=向本组追加
// 成员行
function addCondxRow(): void {
  rows.value.push({ connector: rows.value.length ? 'and' : '', negated: false, condition: null, group: null })
  const idx = rows.value.length - 1
  emit('editRow', rows.value[idx], cardNos.value[idx] ?? idx + 1)
}
</script>

<style scoped>
/* 条件卡片（总览）：容器纵向承载恒定高度的主行；卡片主行显式固定高——复杂度只进
   徽标计数，卡片高度不随条件内部复杂度增长（总览密度守恒） */
.li-condx-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  /* 识别条件行左缘主色 3px 竖条（与提取行 success 绿条色彩分野，行区域标识） */
  border-left: 3px solid var(--el-color-primary);
  border-radius: var(--app-radius);
  padding: 0 10px;
}

.li-condx-card.is-invalid {
  border-color: var(--el-color-danger);
}

/* 卡片主行：恒定一行高（高度显式约束）+单行截断——徽标/按钮不换行 */
.li-condx-card-line {
  height: 40px;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: none;
}

.li-condx-card-no {
  flex: none;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-secondary);
}

.li-condx-card-target {
  flex: none;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
}

/* 模板摘要：单行截断（悬浮 title 看全文） */
.li-condx-card-tpl {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 复杂度徽标段（「· 附加×N」「· 校验×N」「· 第N级」）：正文外收纳 */
.li-condx-card-note {
  flex: none;
  font-size: 12px;
  color: var(--app-text-secondary);
  white-space: nowrap;
}

/* 独立徽标（非/多值/⚠/✗）：小色块 */
.li-condx-card-flag {
  flex: none;
  font-size: 11px;
  line-height: 1;
  padding: 3px 5px;
  border-radius: var(--app-radius);
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  color: var(--app-text-secondary);
  white-space: nowrap;
}

.li-condx-card-flag.is-warn {
  color: var(--app-color-warning, var(--app-text-secondary));
  border-color: var(--app-color-warning, var(--app-border-color-light));
}

.li-condx-card-flag.is-err {
  color: var(--app-color-danger);
  border-color: var(--app-color-danger);
}

/* 括号子组组框（页内直显）：框内边距=成员卡相对顶层的缩进，嵌套组=框中框自然
   再缩进一层；左缘竖条与条件卡同款（识别条件行区域标识） */
.li-condx-group {
  margin: 4px 0;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-left: 3px solid var(--el-color-primary);
  border-radius: var(--app-radius);
  padding: 0 10px 8px;
  min-width: 0;
}

/* 组框头：恒定一行高（比成员卡矮一档，框界与内容分野）；「（」+组内接词语义
   标注在左、组级操作（接词/拆组/换位/删整组）靠右 */
.li-condx-group-head {
  height: 32px;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.li-condx-group-mark {
  flex: none;
  font-size: 16px;
  font-weight: 600;
  color: var(--app-color-primary);
}

/* 组内接词语义标注（成员行间接词统一=该词；混合=「且/或」） */
.li-condx-group-word {
  flex: none;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
}

.li-condx-conn {
  width: 130px;
  flex: none;
}

.li-condx-neg {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
  white-space: nowrap;
}

/* 「非」/「括号」开关禁用态（封顶层/多成员组）：整体降透明度呈灰 */
.li-condx-neg.is-locked {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 换位组：靠右成组（模板摘要 flex:1 已占满剩余空间）；tooltip 悬停由内层 span 接管 */
.li-condx-move {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  flex: none;
}

.li-condx-move-btn {
  display: inline-flex;
}

/* 换位组与行操作按钮的竖分隔线：间距交由主行 gap 统一（清零 divider 自带 margin） */
.li-condx-move-sep {
  margin: 0;
  height: 14px;
}

/* 删行按钮：随换位组之后成组靠右 */
.li-condx-del {
  flex: none;
}

/* 卡片列表底部动作区（〔+ 加条件〕靠右） */
.li-condx-cards-foot {
  display: flex;
  justify-content: flex-end;
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
</style>
