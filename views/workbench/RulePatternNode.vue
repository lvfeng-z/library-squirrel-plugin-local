<template>
  <div class="li-pn">
    <!-- 形态类型行：切换类型（按 RuleEditDialog 识别段的形态清单展开；库内确认
         为 predicate 的顶层直达入口，按确认类别拆三项） -->
    <div class="li-pn-kind-line">
      <span class="li-pn-label">形态类型</span>
      <el-select :model-value="patternKindValue(node)" size="small" class="li-pn-kind" @change="changeKind">
        <el-option v-for="k in kindOptions" :key="k.value" :label="k.label" :value="k.value" />
      </el-select>
      <span class="li-pn-kind-note">{{ kindNote }}</span>
    </div>

    <!-- 文件名形态：格式模板行（帧-01，回车推导）+ 段/分隔写进正则原文；捕获组在此声明 -->
    <template v-if="node.kind === 'regex'">
      <div class="li-pn-line">
        <span class="li-pn-label">匹配目标</span>
        <el-select v-model="node.target" size="small" class="li-pn-target">
          <el-option v-for="t in REGEX_TARGETS" :key="t.value" :label="t.label" :value="t.value" />
        </el-select>
      </div>
      <div v-if="node.target === 'filename'" class="li-pn-line li-pn-stack">
        <span class="li-pn-label">
          格式模板
          <i class="li-pn-note">（占位词回车推导下方正则与捕获组声明，占位词外文字按原文）</i>
        </span>
        <div class="li-pn-tpl-row">
          <el-input
            v-model="tplText"
            size="small"
            placeholder="如 {作者}_{集名}_{序号} 或 Artist[{作者}]"
            class="li-pn-tpl"
            spellcheck="false"
            @keydown.enter="applyTemplate"
          />
          <el-button size="small" text @click="applyTemplate">推导</el-button>
        </div>
        <p v-if="tplError" class="li-pn-error">{{ tplError }}</p>
        <p v-else-if="tplStale" class="li-pn-tpl-stale">正则已手改，模板仅供参考；再次推导=覆盖正则与捕获组声明</p>
      </div>
      <div class="li-pn-line li-pn-stack">
        <span class="li-pn-label">正则表达式 <i class="li-pn-note">（段与分隔直接写原文；捕获组用 (?P&lt;组名&gt;…) 书写并在下方声明）</i></span>
        <el-input
          v-model="node.expr"
          type="textarea"
          :autosize="{ minRows: 2, maxRows: 8 }"
          class="li-pn-expr"
          spellcheck="false"
        />
      </div>
      <div class="li-pn-caps">
        <span class="li-pn-label">捕获组声明 <i class="li-pn-note">（提取动作与条件引用的组名；仅文件名/相对路径目标参与绑定）</i></span>
        <div v-for="(name, i) in node.captures" :key="i" class="li-pn-cap-row">
          <el-input
            :model-value="name"
            size="small"
            placeholder="组名"
            class="li-pn-cap-name"
            @update:model-value="(v: string) => setCapture(i, v)"
          />
          <span class="li-pn-cap-src" :title="node.expr">← 来自本节点正则</span>
          <el-button size="small" text type="danger" @click="removeCapture(i)">删</el-button>
        </div>
        <el-button size="small" text @click="addCapture">+ 加捕获组</el-button>
      </div>
    </template>

    <!-- 目录锚定：层深区间 + 单层目录名的子形态（格式段） -->
    <template v-else-if="node.kind === 'location'">
      <div class="li-pn-line">
        <span class="li-pn-label">层深区间</span>
        <el-input-number v-model="node.minDepth" size="small" :min="0" controls-position="right" class="li-pn-num" />
        <span class="li-pn-dash">..</span>
        <el-input-number v-model="node.maxDepth" size="small" :min="-1" controls-position="right" class="li-pn-num" />
        <span class="li-pn-note">（自库根起 0 计；上界 -1 = 不限）</span>
      </div>
      <div class="li-pn-sub">
        <span class="li-pn-label">格式段（作用于单层目录名的子形态）</span>
        <RulePatternNode :node="ensureChild('ancestor')" />
      </div>
    </template>

    <!-- 序列群组：键 / 序号 / 成群数 -->
    <template v-else-if="node.kind === 'sibling'">
      <div class="li-pn-line li-pn-stack">
        <span class="li-pn-label">键提取正则 <i class="li-pn-note">（空 = 去扩展名全名；含捕获组时取第一个分组为键）</i></span>
        <el-input v-model="node.keyExpr" size="small" spellcheck="false" placeholder="（空 = 去扩展名全名）" />
      </div>
      <div class="li-pn-line li-pn-stack">
        <span class="li-pn-label">序号提取正则 <i class="li-pn-note">（须至少含一个捕获组）</i></span>
        <el-input v-model="node.seqExpr" size="small" spellcheck="false" />
      </div>
      <div class="li-pn-line">
        <span class="li-pn-label">成群最少成员数</span>
        <el-input-number v-model="node.minGroupSize" size="small" :min="1" controls-position="right" class="li-pn-num" />
        <span class="li-pn-label li-pn-label-gap">命中后归属</span>
        <el-select v-model="node.outcome" size="small" class="li-pn-target">
          <el-option v-for="o in GROUP_OUTCOMES" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
      </div>
    </template>

    <!-- 库内确认（predicate）：确认类别 + 取值来源下拉（列上半声明的捕获组；
         引用不在声明集=行级红，当前值保底呈现「（未声明）」） -->
    <template v-else-if="node.kind === 'predicate'">
      <div class="li-pn-line">
        <span class="li-pn-label">确认类别</span>
        <el-select v-model="node.predKind" size="small" class="li-pn-target">
          <el-option v-for="k in PREDICATE_KINDS" :key="k.value" :label="k.label" :value="k.value" />
        </el-select>
        <span class="li-pn-label li-pn-label-gap">取值来源</span>
        <el-select v-model="node.from" size="small" class="li-pn-flex">
          <el-option value="" label="文件名去扩展名" />
          <el-option v-for="name in fromNameOptions" :key="name" :value="name" :label="name" />
          <el-option v-if="fromUndeclared" :value="node.from" :label="`${node.from}（未声明）`" />
        </el-select>
      </div>
      <p v-if="fromUndeclared" class="li-pn-error">{{ fromMissingHint }}</p>
    </template>

    <!-- 组合：子形态列表 -->
    <template v-else-if="node.kind === 'and' || node.kind === 'or'">
      <div class="li-pn-subs">
        <div v-for="(child, i) in childList" :key="i" class="li-pn-sub-item">
          <span class="li-pn-sub-index">#{{ i + 1 }}</span>
          <RulePatternNode :node="child" class="li-pn-sub-body" />
          <el-button size="small" text type="danger" class="li-pn-sub-del" @click="removeChild(i)">删</el-button>
        </div>
        <el-button size="small" text @click="addChild">+ 加子形态</el-button>
      </div>
    </template>

    <!-- 取反 -->
    <template v-else-if="node.kind === 'not'">
      <div class="li-pn-sub">
        <span class="li-pn-label">内层形态（不命中才命中）</span>
        <RulePatternNode :node="ensureChild('inner')" />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
// 识别形态的结构化编辑节点（RuleEditDialog 识别段用）：按形态类型展开字段——
// 文件名形态〔段/分隔写进正则原文 + 捕获组声明〕、目录锚定〔层深/格式段〕、
// 序列群组〔键/序号/成群数〕、库内确认〔确认类别+取值来源下拉〕、组合〔子形态
// 列表〕、取反补全。递归自引用（SFC 文件名自引用），就地改写传入的响应式节点
// 对象；复杂树可切 JSON 高级模式直通原文（见 RuleEditDialog）。
import { computed, inject, ref, type ComputedRef, type Ref } from 'vue'
import {
  PATTERN_KINDS,
  LEAF_PATTERN_KINDS,
  REGEX_TARGETS,
  PREDICATE_KINDS,
  GROUP_OUTCOMES,
  defaultPatternNode,
  derivePatternFromTemplate,
  patternKindValue,
  patternNodeOfKindValue,
  type PatternNode
} from './ruleFormModel'

const props = defineProps<{
  node: PatternNode
  // leaf-only：行式接词（帧-04）下的叶子——组合/取反由行结构承载，类型下拉
  // 不再含 and/or/not（RulePatternRows 的叶子以此约束；location 格式段等
  // 嵌套编辑仍用全量清单）。
  leafOnly?: boolean
}>()

const kindOptions = computed(() => (props.leafOnly ? LEAF_PATTERN_KINDS : PATTERN_KINDS))

const kindNote = computed(
  () => (kindOptions.value.find((k) => k.value === patternKindValue(props.node)) || { note: '' }).note
)

const childList = computed(() => props.node.patterns || [])

// 切换形态类型：按新类型（含库内确认复合值）就地重建节点字段（保留 kind 之外
// 全部重置为该类型默认）。
function changeKind(kind: string): void {
  const fresh = patternNodeOfKindValue(kind)
  for (const key of Object.keys(props.node)) {
    delete (props.node as Record<string, unknown>)[key]
  }
  Object.assign(props.node, fresh)
}

// —— 库内确认（predicate）取值来源：上半捕获组声明集由 RuleEditDialog 注入 ——

// 当前声明集（随上半形态编辑即时刷新）与历史声明集（区分「从未声明」与
// 「声明后被删」两级文案）。
const declaredNames = inject<ComputedRef<Set<string>> | null>('liPatternDeclaredNames', null)
const everNames = inject<Ref<Set<string>> | null>('liPatternEverNames', null)

const fromNameOptions = computed(() => (declaredNames ? [...declaredNames.value] : []))

const fromUndeclared = computed(
  () =>
    props.node.kind === 'predicate' &&
    !!props.node.from &&
    !(declaredNames ? declaredNames.value.has(props.node.from) : false)
)

const fromMissingHint = computed(() =>
  everNames && everNames.value.has(props.node.from || '')
    ? '引用的捕获组已不存在'
    : '取值来源引用的捕获组未声明'
)

// —— regex 捕获组声明 ——

function setCapture(index: number, value: string): void {
  const caps = props.node.captures || []
  caps[index] = value
}

function addCapture(): void {
  if (!Array.isArray(props.node.captures)) props.node.captures = []
  props.node.captures.push('')
}

function removeCapture(index: number): void {
  const caps = props.node.captures || []
  caps.splice(index, 1)
}

// —— location.ancestor / not.inner：确保子节点在场（缺省补默认 regex 节点） ——
function ensureChild(field: 'ancestor' | 'inner'): PatternNode {
  const child = props.node[field]
  if (child && typeof child === 'object' && typeof child.kind === 'string') return child
  const fresh = defaultPatternNode('regex')
  props.node[field] = fresh
  return fresh
}

// —— and / or 子形态列表 ——

function addChild(): void {
  if (!Array.isArray(props.node.patterns)) props.node.patterns = []
  props.node.patterns.push(defaultPatternNode('regex'))
}

function removeChild(index: number): void {
  const list = props.node.patterns || []
  list.splice(index, 1)
}

// —— 格式模板（帧-01 收敛）：占位词模板 → 正则 + 捕获组声明推导 ——
// 组件局部态（弹窗关即弃）：推导产物记录用于「手改灰注」（正则 ≠ 最近推导
// 产物时模板仅供参考；再推导=覆盖，灰注即预告）。
const tplText = ref('')
const tplError = ref('')
const tplAppliedExpr = ref('')

const tplStale = computed(
  () => tplAppliedExpr.value !== '' && (props.node.expr || '') !== tplAppliedExpr.value
)

function applyTemplate(): void {
  const r = derivePatternFromTemplate(tplText.value)
  if (!r.ok || !r.expr || !r.captures) {
    tplError.value = r.error || '模板推导失败'
    return
  }
  tplError.value = ''
  props.node.expr = r.expr
  props.node.captures = [...r.captures]
  tplAppliedExpr.value = r.expr
}
</script>

<style scoped>
/* 主题令牌呈现（--app-* 家族），结构化形态编辑节点 */
.li-pn {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  padding: 10px;
  background: var(--app-bg-surface);
}

/* 嵌套子形态缩进视觉：子节点留浅底 */
.li-pn .li-pn {
  background: var(--app-fill-color-light, var(--app-bg-surface));
}

.li-pn-kind-line,
.li-pn-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-pn-stack {
  flex-direction: column;
  align-items: stretch;
}

.li-pn-label {
  flex-shrink: 0;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-pn-label-gap {
  margin-left: 8px;
}

.li-pn-note {
  font-style: normal;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* 库内确认取值来源引用未声明的行级提示（与提取行行级红同款危险色） */
.li-pn-error {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-danger);
}

.li-pn-kind {
  width: 150px;
}

.li-pn-target {
  width: 200px;
}

.li-pn-num {
  width: 100px;
}

.li-pn-dash {
  color: var(--app-text-secondary);
}

.li-pn-flex {
  flex: 1;
  min-width: 160px;
}

.li-pn-kind-note {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pn-expr :deep(textarea) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
}

.li-pn-caps {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: flex-start;
}

.li-pn-cap-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.li-pn-cap-name {
  max-width: 200px;
}

.li-pn-cap-src {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.li-pn-sub,
.li-pn-subs {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: stretch;
}

.li-pn-sub-item {
  display: flex;
  align-items: flex-start;
  gap: 6px;
}

.li-pn-sub-index {
  flex-shrink: 0;
  padding-top: 12px;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
}

.li-pn-sub-body {
  flex: 1;
  min-width: 0;
}

.li-pn-sub-del {
  flex-shrink: 0;
  margin-top: 4px;
}

/* 格式模板行（帧-01）：输入+推导按钮一行；错误红、手改灰注 */
.li-pn-tpl-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-pn-tpl {
  max-width: 420px;
}

.li-pn-tpl-stale {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}
</style>
