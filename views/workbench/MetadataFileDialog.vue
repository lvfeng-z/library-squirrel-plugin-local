<template>
  <!-- 元数据文件弹窗（元数据文件映射与聚合解耦·剧本A 帧2/帧3）：「元数据文件」
       产出的唯一指派面——勾选本规则条件行+为每行勾选内容去向（一行可多选，
       整段同时写入所有勾选维度）+声明配对方式（标识配对/目录配对，映射单用时
       必填）。行清单与编辑弹窗条件行区同源（父组件组装传入）；轨道随行标注随
       聚合在场才显示（并用形态=BO 剧本1 帧2 画面叠加配对方式区，其余不变；
       映射单用时行无轨概念）。帧3 失败路径=勾选行未选去向→确定被拦（正文只出
       ⚠ 行+[知道了]，回列表勾选保留）。壳=AutoHeightDialog（主程序全局注册，
       RuleEditDialog 同款、无需 import）。 -->
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
      <!-- 配对方式区（剧本A 帧2「配对方式：[标识配对▾]」）：映射单用时必选（保存
           闸门兜底）；并用时可清空=省略声明（分组随聚合并组）。标识配对下亮出
           配对键部分行（「配对：第 1 行 · {作品}+{序号}」形态，候选=全部条件行
           的命名捕获+序列行群键） -->
      <div class="li-mfd-pair">
        <span class="li-mfd-pair-title">配对方式：</span>
        <el-select
          v-model="pairingMode"
          size="small"
          class="li-mfd-pair-select"
          clearable
          :placeholder="hasAggregate ? '未声明（随聚合并组）' : '选择配对方式'"
          @change="onPairingModeChange"
        >
          <el-option v-for="m in PAIRING_MODE_OPTIONS" :key="m.value" :label="m.label" :value="m.value" />
        </el-select>
      </div>
      <div v-if="pairingMode === 'identity'" class="li-mfd-pair-line">
        <span class="li-mfd-pair-label">配对：</span>
        <template v-for="(part, pi) in pairingParts" :key="pi">
          <span v-if="pi > 0" class="li-mfd-pair-plus">+</span>
          <el-select
            :model-value="part.capture ? `${part.row}/${part.capture}` : ''"
            size="small"
            class="li-mfd-pair-part"
            placeholder="选占位词捕获"
            @update:model-value="setPairingPart(pi, $event)"
          >
            <el-option-group v-for="g in pairingGroups" :key="g.row" :label="`第 ${g.row} 行`">
              <el-option
                v-for="c in g.candidates"
                :key="`${g.row}/${c.group}`"
                :label="`第 ${g.row} 行 · {${c.word}}`"
                :value="`${g.row}/${c.group}`"
              />
            </el-option-group>
            <!-- 失效捕获残影选项（条件行模板改写后原引用不在声明中）：仅供呈现，
                 不可重选（保存闸门拦） -->
            <el-option
              v-for="o in pairingStaleOptions"
              :key="`stale-${o.value}`"
              :label="o.label"
              :value="o.value"
              disabled
            />
          </el-select>
          <el-button
            v-if="pairingParts.length > 1"
            size="small"
            text
            type="danger"
            @click="pairingParts.splice(pi, 1)"
          >删</el-button>
        </template>
        <el-button size="small" text @click="addPairingPart">+ 加一部分</el-button>
      </div>
      <p class="li-mfd-hint">ℹ 说明文件归进配对作品、不导入；若本规则另有聚合产出，命中行照常作为轨导入，内容来源叠加</p>
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
// 元数据文件弹窗（元数据文件映射与聚合解耦·阶段3 前端镜像）：「元数据文件」
// 产出的唯一指派面。行身份由两处指派合成——分轨入口（并用时轨道行=导入，随行
// 标注轨道词）与本弹窗勾选（内容产出元数据）；两者兼有=双重身份（照常导入）；
// 仅勾选未分配轨道=伴随说明文件处置（归进配对作品、无轨、不导入、不独立成
// 作品）。配对方式是「元数据文件」产出自己的声明——与聚合的作品标识各自独立、
// 并用互不牵连：标识配对=配对键部分（行号+捕获名）拼成复合标识，目录配对=所在
// 目录即配对键；映射单用（无聚合产出）时必选（保存闸门兜底，本弹窗不拦）、
// 并用时可清空省略（分组随聚合并组）。确定把绑定清单（一行多目标=同行多条目）
// 与配对键声明经 confirm 回写父组件（规则级 metadataFile 落档）；取消不保存。
import { ref, computed, watch } from 'vue'
import {
  CONTENT_BINDING_DIMENSIONS,
  PAIRING_MODE_OPTIONS,
  aggIdentityWordOf,
  cloneJSON,
  type ContentBindingForm,
  type IdentityPartForm,
  type PairingKeyForm,
  type PairingModeForm,
  type AggIdentityCandidate
} from './ruleFormModel'

const props = defineProps<{
  state: boolean
  // 弹窗标题后缀（规则描述，帧2「元数据文件 · 序列图集·每图带说明」形态）
  ruleTitle: string
  // 行清单（与编辑弹窗条件行区同源）：no=行号、text=模板形态（扩展名附加条件
  // 括注尾接）、track=轨道短词（空=未分配轨道）
  rows: { no: number; text: string; track: string }[]
  // 轨道随行标注的显示开关（聚合形态=true：BO 剧本1 帧2「（轨道：视频）」
  //「（未分配轨道）」；映射单用=false——行无轨概念，不渲染标注）
  showTracks: boolean
  // 已落档内容绑定（开窗预勾选+带入去向；一行多目标=同行多条目）
  bindings: ContentBindingForm[]
  // 已落档配对键声明（null=未声明——并用形态的省略态）
  pairing: PairingKeyForm | null
  // 聚合产出是否在场（并用=true：配对方式可清空省略；映射单用=false：必选，
  // 保存闸门兜底）
  hasAggregate: boolean
  // 配对键部分候选（全部条件行的命名捕获+序列行群键，父组件按当前条件行态
  // 组装传入）
  identityCandidates: AggIdentityCandidate[]
}>()

const emit = defineEmits<{
  (e: 'update:state', value: boolean): void
  (e: 'confirm', bindings: ContentBindingForm[], pairing: PairingKeyForm | null): void
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
// 配对方式草稿（''=未声明——并用形态的省略态；单用时保存闸门拦）
const pairingMode = ref<PairingModeForm | ''>('')
// 标识配对的配对键部分草稿（行号+捕获名；目录配对不携带）
const pairingParts = ref<IdentityPartForm[]>([])

const visible = computed({
  get: (): boolean => props.state,
  set: (value: boolean) => emit('update:state', value)
})

// 开窗取初值：行清单=父组件传入的当前条件行区态；已绑定行预勾选并带入去向
//（同行多条目=多目标并入同一行的勾选集）；配对键按落档声明带入
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
    pairingMode.value = props.pairing && props.pairing.mode ? props.pairing.mode : ''
    pairingParts.value =
      props.pairing && props.pairing.mode === 'identity' && Array.isArray(props.pairing.identity)
        ? cloneJSON(props.pairing.identity)
        : []
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

// 配对键候选分组（按行号）：文件名捕获/目录行捕获/序列行群键同列
const pairingGroups = computed<{ row: number; candidates: AggIdentityCandidate[] }[]>(() => {
  const out: { row: number; candidates: AggIdentityCandidate[] }[] = []
  for (const c of props.identityCandidates || []) {
    const last = out[out.length - 1]
    if (last && last.row === c.row) last.candidates.push(c)
    else out.push({ row: c.row, candidates: [c] })
  }
  return out
})

// 失效捕获残影选项（条件行模板改写后原引用不在声明中）：仅供行上呈现、不可重选
const pairingStaleOptions = computed<{ value: string; label: string }[]>(() => {
  const declared = new Set(
    pairingGroups.value.flatMap((g) => g.candidates.map((c) => `${g.row}/${c.group}`))
  )
  const out: { value: string; label: string }[] = []
  const seen = new Set<string>()
  for (const p of pairingParts.value) {
    const key = `${p.row}/${p.capture}`
    if (!p.capture || declared.has(key) || seen.has(key)) continue
    seen.add(key)
    out.push({ value: key, label: `第 ${p.row} 行 · {${aggIdentityWordOf(p.capture)}}` })
  }
  return out
})

// 配对方式切换：切到标识配对而无部分时缺省预置首个候选（无候选时空选、保存
// 闸门提示）；切到目录配对清空部分（目录配对不携带标识部分）
function onPairingModeChange(mode: PairingModeForm | ''): void {
  if (mode === 'identity' && !pairingParts.value.length) {
    const first = (props.identityCandidates || [])[0]
    pairingParts.value = [first ? { row: first.row, capture: first.group } : { row: 1, capture: '' }]
  }
  if (mode === 'directory') {
    pairingParts.value = []
  }
}

// 配对键部分取值拆装（值编码 行号/捕获名，从左拆一刀）
function setPairingPart(pi: number, value: string): void {
  const idx = value.indexOf('/')
  if (idx < 0) return
  const part = pairingParts.value[pi]
  if (!part) return
  part.row = Number(value.slice(0, idx)) || 1
  part.capture = value.slice(idx + 1)
}

// 加一部分：缺省选中首个候选（无候选时空选、保存闸门提示）
function addPairingPart(): void {
  const first = (props.identityCandidates || [])[0]
  pairingParts.value.push(first ? { row: first.row, capture: first.group } : { row: 1, capture: '' })
}

function cancel(): void {
  emit('update:state', false)
}

// 确定（帧2 手势=勾选→选去向→选配对方式→单次点）：每行勾选必须配至少一个
// 内容去向——首个未选行进帧3 拦截态（弹窗留在原地、勾选保留，补选后确定照常）；
// 全配齐则回写绑定清单（一行多目标=按枚举序展开为同行多条目；空清单=清除指派，
// 该规则行为回到现状）与配对键声明（未声明=null——并用省略态合法、单用由保存
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
  let pairing: PairingKeyForm | null = null
  if (pairingMode.value === 'identity') {
    pairing = { mode: 'identity', identity: cloneJSON(pairingParts.value) }
  } else if (pairingMode.value === 'directory') {
    pairing = { mode: 'directory' }
  }
  emit('confirm', bindings, pairing)
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

/* 配对方式区（剧本A 帧2）：「配对方式：[标识配对▾]」+ 标识配对下的配对键
   部分行（「配对：第 1 行 · {作品}+{序号}」） */
.li-mfd-pair {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 4px;
}

.li-mfd-pair-title {
  flex: none;
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-mfd-pair-select {
  width: 200px;
}

.li-mfd-pair-line {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-mfd-pair-label {
  flex: none;
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-mfd-pair-plus {
  flex: none;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-mfd-pair-part {
  width: 190px;
}

/* ℹ 提示（帧2：配对归并语义+并用叠加口径） */
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
