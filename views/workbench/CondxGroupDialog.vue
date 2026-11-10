<template>
  <AutoHeightDialog
    v-model:state="state"
    width="min(720px, 92vw)"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
  >
    <template #header>
      <div class="li-cgd-header">
        <span>括号子组编辑</span>
      </div>
    </template>

    <div class="li-cgd-body">
      <!-- 面包屑导航栈（帧3/帧4、剧本二帧1-帧2）：首段「顶层 ›」静态；其后每段=
           一次下钻（首段下钻标签=「第 N 行括号」（N=组行全局先序号）、更深层固定
           「嵌套括号」）；点段回退=内容切回该层（嵌套子组不开新弹窗，决策7） -->
      <nav v-if="current" class="li-cgd-crumbs">
        <span class="li-cgd-crumb is-root">顶层 ›</span>
        <template v-for="(lv, li) in stack" :key="li">
          <span v-if="li > 0" class="li-cgd-sep">›</span>
          <button
            v-if="li < stack.length - 1"
            type="button"
            class="li-cgd-crumb is-link"
            @click="popTo(li)"
          >{{ lv.label }}</button>
          <span v-else class="li-cgd-crumb is-current">{{ lv.label }}</span>
        </template>
      </nav>
      <!-- 组内条件行编辑器：与顶层同构复用（决策2 全套增删改查）；depth 逐层下传
           （第 3 层封顶禁开+提示）、baseNo=全局行号基数（行头「第 N 行」小字）、
           rootRows=顶层行表（对方行跨组候选）；recordEntry=false——下钻组行不写
           开窗入口侧信道（保入口组行引用供删空拆组定位）；编辑直改同一 reactive
           行树——实时生效、无取消语义，关窗即保留 -->
      <CondxRowsEditor
        v-if="current"
        :key="current"
        v-model:rows="current.rows"
        :depth="currentDepth"
        :base-no="current.base"
        :root-rows="topRows"
        :allow-ref-row="false"
        :record-entry="false"
        :extractions-json-mode="extractionsJsonMode"
        @edit-group="pushLevel"
        @moved="emit('moved', $event)"
      />
    </div>

    <template #footer>
      <div class="li-cgd-actions">
        <el-button type="primary" @click="close">完成</el-button>
      </div>
    </template>
  </AutoHeightDialog>
</template>

<script setup lang="ts">
// 括号子组编辑弹窗：主程序全局注册的 AutoHeightDialog 壳（EntryConfirmDialog
// 先例——无需 import）。壳形态=叠在规则编辑主弹窗之上的子组弹窗：ESC 与点遮罩不
// 关闭、显式〔完成〕按钮关窗。弹窗导航栈（技术要点3）：面包屑数组
// [{标签, 行容器引用, 行号基数}]——点嵌套组紧凑卡〔编辑〕push 下钻、点面包屑段
// pop 回退（同弹窗切层，不开新窗）；编辑直改同一 reactive 行树（无取消语义——
// 与顶层行编辑一致，实时生效、关窗即保留）。入口组行经 ruleFormModel 的
// condxGroupEditEntry 侧信道携带（RuleEditDialog 的 edit-group 接线只开窗不带行）。
import { computed, ref, watch } from 'vue'
import CondxRowsEditor from './CondxRowsEditor.vue'
import { condxFlatRows, condxGroupEditEntry, condxRemoveRow, type CondxRowForm, type CondxRowNoMap } from './ruleFormModel'

const state = defineModel<boolean>('state', { required: true })

// 提取区「高级（JSON）」模式（RuleEditDialog 下传）：透传组内行编辑器的移动
// 禁用态（自决2——子组内换位同样要改写主弹窗提取引用，禁用口径与顶层一致）
defineProps<{ extractionsJsonMode?: boolean }>()

// 组内行换位上抛透传（技术要点3、剧本三帧1-2）：子组行树与顶层同一 reactive 树
// （行表已在编辑器内落位），全局行号映射交 RuleEditDialog 改写主弹窗的提取行与
// 条件树引用
const emit = defineEmits<{ moved: [rowNoMap: CondxRowNoMap] }>()

// 导航栈的一层：label=面包屑段文案、rows=本层行容器（组内成员行表引用——同一
// reactive 行树，编辑直改）、base=本层全局行号基数（condxRowNos 的 base 参数）。
interface NavLevel {
  label: string
  rows: CondxRowForm[]
  base: number
}

const stack = ref<NavLevel[]>([])
// 顶层行表（开窗时从入口侧信道取）：组内行编辑器的全局展平基准（对方行跨组引用）
const topRows = ref<CondxRowForm[] | null>(null)

const current = computed<NavLevel | null>(() => (stack.value.length ? stack.value[stack.value.length - 1] : null))

// 本层深度（顶层=第 1 层，入口组内=第 2 层，逐层 +1）：达到 CONDX_GROUP_MAX_DEPTH
// 的层内行头「括号」开关禁用+封顶提示（剧本二帧3）
const currentDepth = computed(() => stack.value.length + 1)

// 开窗：按侧信道入口建导航栈首层——标签「第 N 行括号」（N=组行全局先序号=所在
// 行表内先序展开数+1）、行容器=组行 group 引用。
watch(state, (open) => {
  if (!open) return
  const entry = condxGroupEditEntry.value
  topRows.value = entry ? entry.rows : null
  stack.value = []
  if (!entry || !entry.row.group) return
  const idx = entry.rows.indexOf(entry.row)
  const base = idx > 0 ? condxFlatRows(entry.rows.slice(0, idx)).length : 0
  stack.value = [{ label: `第 ${base + 1} 行括号`, rows: entry.row.group, base }]
})

// 关窗（编辑实时生效，无取消语义——与顶层行编辑一致）
function close(): void {
  state.value = false
}

// 组内紧凑卡〔编辑〕：下钻一层（同弹窗切层，不开新窗——剧本二帧1→帧2）
function pushLevel(row: CondxRowForm): void {
  const cur = current.value
  if (!cur || !row.group) return
  const idx = cur.rows.indexOf(row)
  const base = cur.base + (idx > 0 ? condxFlatRows(cur.rows.slice(0, idx)).length : 0)
  stack.value.push({ label: '嵌套括号', rows: row.group, base })
}

// 点面包屑段：回退到该层（栈截到段序+1；嵌套层改动已实时生效、上层可见——剧本
// 二帧2 的「点『第 2 行括号』段」）
function popTo(levelIndex: number): void {
  if (levelIndex < 0 || levelIndex >= stack.value.length - 1) return
  stack.value.splice(levelIndex + 1)
}

// 删到 0 行自动拆组（自决）：当前层成员删空即拆掉括号、不产生空组——嵌套层=组行
// 从上层行表移除+回退上层（上层也删空则链式再拆）；入口层=组行从顶层行表移除+
// 关窗（清空操作零多余步骤）。
watch(
  () => (current.value ? current.value.rows.length : -1),
  (len) => {
    if (len !== 0 || !current.value) return
    if (stack.value.length > 1) {
      const parent = stack.value[stack.value.length - 2]
      const childRows = current.value.rows
      const gi = parent.rows.findIndex((r) => r.group === childRows)
      // 规整移除（condxRemoveRow）：组行居父表 0 位时新首行接词清零
      if (gi >= 0) condxRemoveRow(parent.rows, gi)
      stack.value.splice(stack.value.length - 1, 1)
      return
    }
    const entry = condxGroupEditEntry.value
    if (entry) {
      const gi = entry.rows.indexOf(entry.row)
      // 规整移除（condxRemoveRow）：组行居顶层 0 位时新首行接词清零
      if (gi >= 0) condxRemoveRow(entry.rows, gi)
    }
    stack.value = []
    state.value = false
  }
)
</script>

<style scoped>
.li-cgd-header {
  display: flex;
  align-items: center;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-cgd-body {
  padding: 4px 0;
}

/* 面包屑导航栈（帧3/剧本二帧1-帧2）：「顶层 › 」+ 下钻段；可点段回退 */
.li-cgd-crumbs {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-bottom: 10px;
  font-size: 13px;
}

.li-cgd-crumb {
  color: var(--app-text-secondary);
}

.li-cgd-crumb.is-current {
  color: var(--app-text-primary);
  font-weight: 600;
}

.li-cgd-crumb.is-link {
  padding: 0;
  border: none;
  background: transparent;
  cursor: pointer;
  color: var(--el-color-primary);
  font-size: 13px;
}

.li-cgd-crumb.is-link:hover {
  text-decoration: underline;
}

.li-cgd-sep {
  color: var(--app-text-secondary);
}

.li-cgd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
