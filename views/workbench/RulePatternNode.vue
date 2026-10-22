<template>
  <div class="li-pn">
    <!-- 形态类型行：切换类型（按 RuleEditDialog 识别段的形态清单展开） -->
    <div class="li-pn-kind-line">
      <span class="li-pn-label">形态类型</span>
      <el-select :model-value="node.kind" size="small" class="li-pn-kind" @change="changeKind">
        <el-option v-for="k in PATTERN_KINDS" :key="k.value" :label="k.label" :value="k.value" />
      </el-select>
      <span class="li-pn-kind-note">{{ kindNote }}</span>
    </div>

    <!-- 文件名形态：段/分隔写进正则原文；捕获组在此声明 -->
    <template v-if="node.kind === 'regex'">
      <div class="li-pn-line">
        <span class="li-pn-label">匹配目标</span>
        <el-select v-model="node.target" size="small" class="li-pn-target">
          <el-option v-for="t in REGEX_TARGETS" :key="t.value" :label="t.label" :value="t.value" />
        </el-select>
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

    <!-- 谓词命中 -->
    <template v-else-if="node.kind === 'predicate'">
      <div class="li-pn-line">
        <span class="li-pn-label">谓词类别</span>
        <el-select v-model="node.predKind" size="small" class="li-pn-target">
          <el-option v-for="k in PREDICATE_KINDS" :key="k.value" :label="k.label" :value="k.value" />
        </el-select>
        <span class="li-pn-label li-pn-label-gap">取值来源</span>
        <el-input v-model="node.from" size="small" placeholder="捕获组名（空 = 文件名去扩展名）" class="li-pn-flex" />
      </div>
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
// 序列群组〔键/序号/成群数〕、组合〔子形态列表〕、谓词/取反补全。递归自引用
// （SFC 文件名自引用），就地改写传入的响应式节点对象；复杂树可切 JSON 高级
// 模式直通原文（见 RuleEditDialog）。
import { computed } from 'vue'
import {
  PATTERN_KINDS,
  REGEX_TARGETS,
  PREDICATE_KINDS,
  GROUP_OUTCOMES,
  defaultPatternNode,
  type PatternNode
} from './ruleFormModel'

const props = defineProps<{
  node: PatternNode
}>()

const kindNote = computed(
  () => (PATTERN_KINDS.find((k) => k.value === props.node.kind) || { note: '' }).note
)

const childList = computed(() => props.node.patterns || [])

// 切换形态类型：按新类型就地重建节点字段（保留 kind 之外全部重置为该类型默认）。
function changeKind(kind: string): void {
  const fresh = defaultPatternNode(kind)
  for (const key of Object.keys(props.node)) {
    delete (props.node as Record<string, unknown>)[key]
  }
  Object.assign(props.node, fresh)
}

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
</style>
