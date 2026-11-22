<template>
  <!-- 元数据文件弹窗（元数据文件映射与聚合解耦·剧本A 帧2/帧3·彻底解耦轮）：
       「元数据文件」产出的唯一指派面——勾选本规则条件行+为每行勾选内容去向（一行
       可多选，整段同时写入所有勾选维度）+分组绑定（分组区自动项+声明并列下拉，
       映射单用时必填）。行清单与编辑弹窗条件行区同源（父组件组装传入）；
       轨道随行标注随聚合产出在场才显示。帧3 失败路径=勾选行未选去向→确定被拦
      （正文只出 ⚠ 行+[知道了]，回列表勾选保留）。壳=AutoHeightDialog（主程序
       全局注册，RuleEditDialog 同款、无需 import）。 -->
  <AutoHeightDialog
    v-model:state="visible"
    width="min(620px, 94vw)"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    :show-close="false"
  >
    <template #header>
      <div class="li-mfd-header">
        <span class="li-mfd-title">元数据文件 · {{ ruleTitle }}</span>
      </div>
    </template>
    <!-- 帧3 失败态：正文只有 ⚠ 行（确定被拦、弹窗留在原地；知道了=回行清单，
         勾选保留） -->
    <div v-if="blockedRow > 0" class="li-mfd-body">
      <p class="li-mfd-blocked">⚠ 第 {{ blockedRow }} 行已勾选为元数据文件，请选择内容去向</p>
    </div>
    <div v-else class="li-mfd-body">
      <p class="li-mfd-lead">勾选哪些条件行的文件内容产出元数据：</p>
      <div v-for="r in drafts" :key="r.no" class="li-mfd-row">
        <el-checkbox v-model="r.checked" class="li-mfd-cb" />
        <span class="li-mfd-row-label">第 {{ r.no }} 行 · {{ r.text }}</span>
        <span v-if="showTracks" class="li-mfd-row-track">{{ r.track ? `（轨道：${r.track}）` : '（未分配轨道）' }}</span>
      </div>
      <p class="li-mfd-dest-title">内容去向（一行可多选）：</p>
      <div v-for="r in checkedDrafts" :key="`dest-${r.no}`" class="li-mfd-dest">
        <span class="li-mfd-dest-row">第 {{ r.no }} 行 →</span>
        <label v-for="d in CONTENT_BINDING_DIMENSIONS" :key="d.value" class="li-mfd-dim">
          <input
            type="checkbox"
            :checked="r.dims.indexOf(d.value) >= 0"
            @change="toggleDraftDim(r, d.value)"
          >
          <span>{{ d.label }}</span>
        </label>
        <span class="li-mfd-dest-note">（整段）</span>
      </div>
      <p v-if="!checkedDrafts.length" class="li-mfd-dest-empty">（未勾选任何行——不指派时该规则行为与现状完全一致）</p>
      <!-- 分组绑定区（剧本A 帧2「分组：[按 {作品}+{序号} 分组▾]」·三子句定稿轮）：
           绑定分组区自动项与显式声明并列（同形显式声明在场时自动项不重复
           出），另含「目录分组 · 所在目录」虚拟项（选定即物化为 directory
           声明）；映射单用时必选（保存闸门兜底）、并用时可清空=缺省（=聚合成
           作品所绑分组，后端求值期取用；编辑面装载并用规则时物化为显式引用） -->
      <div class="li-mfd-group">
        <span class="li-mfd-group-title">分组：</span>
        <el-select
          v-model="groupingValue"
          size="small"
          class="li-mfd-group-select"
          clearable
          :placeholder="hasAggregate ? '未声明（缺省=聚合成作品所绑分组）' : '选择分组'"
        >
          <el-option v-for="o in groupOptions" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
      </div>
      <!-- ℹ 信息文案（A 帧2 逐字——扇出语义、孤文件回退与并用叠加口径；换行按
           帧内排版自然呈现，字符串内容逐字） -->
      <p class="li-mfd-hint">ℹ 说明文件内容并入分组内全部作品（每件各带一份、说明文件自身不导入）；组内无其他文件时说明文件维持独立作品；若本规则另有聚合成作品产出，命中行照常作为轨导入，内容来源叠加</p>
    </div>
    <template #footer>
      <div class="li-mfd-actions">
        <template v-if="blockedRow > 0">
          <el-button type="primary" @click="blockedRow = 0">知道了</el-button>
        </template>
        <template v-else>
          <el-button @click="cancel">取消</el-button>
          <el-button type="primary" @click="confirm">确定</el-button>
        </template>
      </div>
    </template>
  </AutoHeightDialog>
</template>

<script setup lang="ts">
// 元数据文件弹窗（元数据文件映射与聚合解耦·彻底解耦轮）：「元数据文件」
// 产出的唯一指派面。行身份由两处指派合成——分轨入口（并用时轨道行=导入，随行
// 标注轨道词）与本弹窗勾选（内容产出元数据）；两者兼有=双重身份（照常导入）；
// 仅勾选未分配轨道=元数据源文件（仅绑定行命中且组内有落点：无轨、不导入、
// 内容扇出给组内全部作品）。分组绑定是「元数据文件」产出自己的声明绑定——与
// 聚合产出的分组绑定各自独立、并用互不牵连（序列化映射：capture/seqKey 声明
// →grouping identity 形态、directory 声明→grouping directory 形态，父组件
// 物化与序列化，只落 grouping 新键）；映射单用（无聚合产出）时必选（保存闸门
// 兜底，本弹窗不拦）、并用时可清空缺省（=聚合成作品所绑分组，后端求值期取
// 用——编辑面装载时物化为显式引用）。确定把绑定清单（一行多目标=同行多条目）
// 与分组绑定值经 confirm 回写父组件；取消不保存。
import { ref, computed, watch } from 'vue'
import {
  CONTENT_BINDING_DIMENSIONS,
  type ContentBindingForm
} from './ruleFormModel'

const props = defineProps<{
  state: boolean
  // 弹窗标题后缀（规则描述，帧2「元数据文件 · 序列图集·每图带说明」形态）
  ruleTitle: string
  // 行清单（与编辑弹窗条件行区同源）：no=行号、text=模板形态（扩展名附加条件
  // 括注尾接）、track=轨道短词（空=未分配轨道）
  rows: { no: number; text: string; track: string }[]
  // 轨道随行标注的显示开关（聚合产出在场=true：BO 剧本1 帧2「（轨道：视频）」
  //「（未分配轨道）」；映射单用=false——行无轨概念，不渲染标注）
  showTracks: boolean
  // 已落档内容绑定（开窗预勾选+带入去向；一行多目标=同行多条目）
  bindings: ContentBindingForm[]
  // 已绑定的分组声明/自动项下拉取值（decl:uid / auto:seqKey:N；''=未绑定——
  // 并用形态的缺省态）
  boundGroupValue: string
  // 分组绑定下拉条目（父组件按分组区声明态组装传入：decl:uid 项+auto:seqKey:N
  // 自动项+按所在目录虚拟项 dir:）
  groupOptions: { value: string; label: string }[]
  // 聚合产出是否在场（并用=true：分组绑定可清空缺省；映射单用=false：必选，
  // 保存闸门兜底）
  hasAggregate: boolean
}>()

const emit = defineEmits<{
  (e: 'update:state', value: boolean): void
  (e: 'confirm', bindings: ContentBindingForm[], groupingValue: string): void
}>()

interface MetaDraftRow {
  no: number
  text: string
  track: string
  checked: boolean
  // 已勾选内容去向维度（值集=CONTENT_BINDING_DIMENSIONS 的 value）
  dims: string[]
}

const drafts = ref<MetaDraftRow[]>([])
// 帧3 拦截态：首个「已勾选未选去向」的行号（0=正常行清单态）
const blockedRow = ref(0)
// 分组绑定草稿（''=未声明——并用形态的缺省态；单用时保存闸门拦）
const groupingValue = ref('')

const visible = computed({
  get: (): boolean => props.state,
  set: (value: boolean) => emit('update:state', value)
})

// 开窗取初值：行清单=父组件传入的当前条件行区态；已绑定行预勾选并带入去向
//（同行多条目=多目标并入同一行的勾选集）；分组绑定按落档绑定值带入
watch(
  () => props.state,
  (open) => {
    if (!open) return
    const bound = new Map<number, string[]>()
    for (const b of props.bindings || []) {
      if (!b) continue
      const dims = bound.get(b.row) || []
      if (dims.indexOf(b.dimension) < 0) dims.push(b.dimension)
      bound.set(b.row, dims)
    }
    drafts.value = (props.rows || []).map((r) => ({
      no: r.no,
      text: r.text,
      track: r.track,
      checked: (bound.get(r.no) || []).length > 0,
      dims: bound.get(r.no) || []
    }))
    groupingValue.value = props.boundGroupValue || ''
    blockedRow.value = 0
  },
  { immediate: true }
)

// 内容去向区=已勾选行（按行号序）
const checkedDrafts = computed<MetaDraftRow[]>(() => drafts.value.filter((r) => r.checked))

// 维度勾选切换（同一维度重复勾选无效果=取消勾选）
function toggleDraftDim(r: MetaDraftRow, dim: string): void {
  const idx = r.dims.indexOf(dim)
  if (idx >= 0) r.dims.splice(idx, 1)
  else r.dims.push(dim)
}

function cancel(): void {
  emit('update:state', false)
}

// 确定（帧2 手势=勾选→选去向→绑分组→单次点）：每行勾选必须配至少一个
// 内容去向——首个未选行进帧3 拦截态（弹窗留在原地、勾选保留，补选后确定照常）；
// 全配齐则回写绑定清单（一行多目标=按枚举序展开为同行多条目；空清单=清除指派，
// 该规则行为回到现状）与分组绑定值（''=未声明——并用缺省态合法、单用由保存
// 闸门拦）并关窗
function confirm(): void {
  const first = checkedDrafts.value.find((r) => !r.dims.length)
  if (first) {
    blockedRow.value = first.no
    return
  }
  const bindings: ContentBindingForm[] = []
  for (const r of checkedDrafts.value) {
    for (const d of CONTENT_BINDING_DIMENSIONS) {
      if (r.dims.indexOf(d.value) >= 0) bindings.push({ row: r.no, dimension: d.value })
    }
  }
  emit('confirm', bindings, groupingValue.value || '')
  emit('update:state', false)
}
</script>

<style scoped>
/* 元数据文件弹窗：主题令牌呈现（--app-* 家族），形态对齐 RuleEditDialog 弹窗族 */
.li-mfd-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
  flex-wrap: wrap;
}

.li-mfd-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-mfd-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 2px 4px;
  max-height: 60vh;
  overflow-y: auto;
}

.li-mfd-lead {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-primary);
}

/* 行清单：勾选框 + 「第 N 行 · 模板」 + 轨道随行标注（聚合形态） */
.li-mfd-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.li-mfd-cb {
  flex: none;
  height: auto;
}

.li-mfd-row-label {
  flex: 0 1 auto;
  min-width: 0;
  font-size: 13px;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-mfd-row-track {
  flex: none;
  margin-left: auto;
  font-size: 11px;
  color: var(--app-text-secondary);
}

.li-mfd-dest-title {
  margin: 4px 0 0;
  font-size: 13px;
  color: var(--app-text-primary);
}

/* 内容去向行：「第 N 行 → ☑描述 ☐标签 ☐标题 ☐作者 ☐作品集（整段）」 */
.li-mfd-dest {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-mfd-dest-row {
  flex: none;
  font-size: 12px;
  color: var(--app-text-regular);
}

/* 维度勾选（一行可多选：整段同时写入所有勾选维度，各维独立走合成与冲突判定） */
.li-mfd-dim {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 12px;
  color: var(--app-text-primary);
  cursor: pointer;
}

.li-mfd-dim input {
  margin: 0;
  accent-color: var(--app-color-primary);
}

.li-mfd-dest-note {
  flex: none;
  font-size: 11px;
  color: var(--app-text-secondary);
}

.li-mfd-dest-empty {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* 分组绑定区（剧本A 帧2） */
.li-mfd-group {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
}

.li-mfd-group-title {
  flex: none;
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-mfd-group-select {
  width: 260px;
}

/* ℹ 提示（帧2：分组归并语义+并用叠加口径） */
.li-mfd-hint {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

/* 帧3 拦截态 ⚠ 行 */
.li-mfd-blocked {
  margin: 0;
  font-size: 13px;
  color: var(--app-color-warning, var(--app-text-primary));
}

.li-mfd-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
