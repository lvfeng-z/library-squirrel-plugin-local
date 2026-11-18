<template>
  <!-- 元数据文件弹窗（伴随说明文件内容解析·剧本1 帧2/帧7、剧本2 帧2）：
       内容绑定的唯一指派面（决策1）——勾选本规则条件行+为每行选内容去向
      （一期=整段→单维度，决策2）。行清单与聚合弹窗同源（父组件按当前轨行区
       态组装传入，不重复书写识别条件）；轨道随行标注；帧7 失败路径=勾选行未选
       去向→确定被拦（正文只出 ⚠ 行+[知道了]，回列表勾选保留）。壳=
       AutoHeightDialog（主程序全局注册，RuleEditDialog 同款、无需 import）。 -->
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
    <!-- 帧7 失败态：正文只有 ⚠ 行（确定被拦、弹窗留在原地；知道了=回行清单，
         勾选保留） -->
    <div v-if="blockedRow > 0" class="li-mfd-body">
      <p class="li-mfd-blocked">⚠ 第 {{ blockedRow }} 行已勾选为元数据文件，请选择内容去向</p>
    </div>
    <div v-else class="li-mfd-body">
      <p class="li-mfd-lead">勾选哪些条件行的文件内容产出元数据：</p>
      <div v-for="r in drafts" :key="r.no" class="li-mfd-row">
        <el-checkbox v-model="r.checked" class="li-mfd-cb" />
        <span class="li-mfd-row-label">第 {{ r.no }} 行 · {{ r.text }}</span>
        <span class="li-mfd-row-track">{{ r.track ? `（轨道：${r.track}）` : '（未分配轨道）' }}</span>
      </div>
      <p class="li-mfd-dest-title">内容去向：</p>
      <div v-for="r in checkedDrafts" :key="`dest-${r.no}`" class="li-mfd-dest">
        <span class="li-mfd-dest-row">第 {{ r.no }} 行 →</span>
        <el-select v-model="r.dimension" size="small" class="li-mfd-dest-select" placeholder="未选">
          <el-option v-for="d in CONTENT_BINDING_DIMENSIONS" :key="d.value" :label="d.label" :value="d.value" />
        </el-select>
        <span class="li-mfd-dest-note">（整段）</span>
      </div>
      <p v-if="!checkedDrafts.length" class="li-mfd-dest-empty">（未勾选任何行——不指派时该规则行为与现状完全一致）</p>
      <p class="li-mfd-hint">ℹ 已分配轨道的行同样可勾选——文件照常导入，<br>内容同时作为元数据来源（见剧本2 帧2）</p>
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
// 元数据文件弹窗（伴随说明文件内容解析·阶段5 前端镜像）：聚合规则内容绑定的
// 唯一指派面。行身份由两处指派合成——分轨入口（轨道行=导入，随行标注轨道词）
// 与本弹窗勾选（内容产出元数据）；两者兼有=双重身份（照常导入）；仅勾选未分配
// 轨道=伴随说明文件处置（归进作品、无轨、不导入、不独立成作品）；两者皆无=
// 不参与成员判定。确定把绑定清单经 confirm 回写父组件（挂分轨入口
// contentBindings 落档）；取消不保存。
import { ref, computed, watch } from 'vue'
import { CONTENT_BINDING_DIMENSIONS, type ContentBindingForm } from './ruleFormModel'

const props = defineProps<{
  state: boolean
  // 弹窗标题后缀（规则描述，帧2「元数据文件 · 视频+字幕+说明」形态）
  ruleTitle: string
  // 行清单（与聚合弹窗轨行区同源）：no=行号、text=模板形态、track=轨道短词
  //（空=未分配轨道）
  rows: { no: number; text: string; track: string }[]
  // 已落档内容绑定（开窗预勾选+带入去向；预置「单图+说明文本」即预配形态）
  bindings: ContentBindingForm[]
}>()

const emit = defineEmits<{
  (e: 'update:state', value: boolean): void
  (e: 'confirm', bindings: ContentBindingForm[]): void
}>()

interface MetaDraftRow {
  no: number
  text: string
  track: string
  checked: boolean
  dimension: string
}

const drafts = ref<MetaDraftRow[]>([])
// 帧7 拦截态：首个「已勾选未选去向」的行号（0=正常行清单态）
const blockedRow = ref(0)

const visible = computed({
  get: (): boolean => props.state,
  set: (value: boolean) => emit('update:state', value)
})

// 开窗取初值：行清单=父组件传入的当前轨行区态；已绑定行预勾选并带入去向
watch(
  () => props.state,
  (open) => {
    if (!open) return
    const bound = new Map<number, string>()
    for (const b of props.bindings || []) {
      if (b) bound.set(b.row, b.dimension)
    }
    drafts.value = (props.rows || []).map((r) => ({
      no: r.no,
      text: r.text,
      track: r.track,
      checked: bound.has(r.no),
      dimension: bound.get(r.no) || ''
    }))
    blockedRow.value = 0
  },
  { immediate: true }
)

// 内容去向区=已勾选行（按行号序）
const checkedDrafts = computed<MetaDraftRow[]>(() => drafts.value.filter((r) => r.checked))

function cancel(): void {
  emit('update:state', false)
}

// 确定（帧2 手势=勾选→选去向→单次点）：每行勾选必须配一个内容去向——首个
// 未选行进帧7 拦截态（弹窗留在原地、勾选保留，补选后确定照常）；全配齐则
// 回写绑定清单（空清单=清除指派，该规则行为回到现状）并关窗
function confirm(): void {
  const first = checkedDrafts.value.find((r) => !r.dimension)
  if (first) {
    blockedRow.value = first.no
    return
  }
  emit(
    'confirm',
    checkedDrafts.value.map((r) => ({ row: r.no, dimension: r.dimension }))
  )
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

/* 行清单：勾选框 + 「第 N 行 · 模板」 + 轨道随行标注（剧本1 帧2） */
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

/* 内容去向行：「第 N 行 → [维度▾]（整段）」 */
.li-mfd-dest {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-mfd-dest-row {
  flex: none;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-mfd-dest-select {
  width: 130px;
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

/* ℹ 两行提示（帧2：轨道行双重身份说明） */
.li-mfd-hint {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

/* 帧7 拦截态 ⚠ 行 */
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
