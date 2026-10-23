<template>
  <div class="li-pr">
    <!-- 首行无接词；其后每行行首 且/或（SQL WHERE 同构：优先级 非>且>或，
         括号子组最紧——帧-04 收敛形态） -->
    <div v-for="(row, i) in rows" :key="i" class="li-pr-row">
      <div class="li-pr-head">
        <el-select
          v-if="i > 0"
          :model-value="row.conn || 'and'"
          size="small"
          class="li-pr-conn"
          @update:model-value="(v: string) => (row.conn = v as 'and' | 'or')"
        >
          <el-option value="and" label="且" />
          <el-option value="or" label="或" />
        </el-select>
        <span v-else class="li-pr-first">（首行）</span>
        <label class="li-pr-neg" :class="{ 'is-on': row.negate }">
          <input type="checkbox" v-model="row.negate" />
          <span>非</span>
        </label>
        <el-button size="small" text type="danger" class="li-pr-del" @click="removeRow(i)">删</el-button>
      </div>
      <div class="li-pr-body">
        <template v-if="row.group">
          <button type="button" class="li-pr-group-toggle" @click="row.open = !row.open">
            {{ row.open ? '▾' : '▸' }} 括号子组 · {{ row.group.length }} 项（点{{ row.open ? '收' : '开' }}编辑）
          </button>
          <RulePatternRows v-if="row.open" :rows="row.group" />
          <p v-if="!row.group.length" class="li-pr-empty">（空子组——加子形态或删除本行）</p>
        </template>
        <RulePatternNode v-else-if="row.leaf" :node="row.leaf" leaf-only />
      </div>
    </div>
    <div class="li-pr-actions">
      <el-button size="small" text @click="addLeaf">+ 加子形态</el-button>
      <el-button size="small" text @click="addGroup">+ 加括号子组</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
// SQL WHERE 式行式接词渲染器（帧-04 收敛形态，RuleEditDialog 识别段根）：
// 行列表=首行无接词+行前 且/或 下拉+行级「非」勾选+叶子形态（RulePatternNode
// leaf-only）或括号子组（递归本组件）；行间优先级 非>且>或、子组最紧，解析
// 结果（含括号分组）由 RuleEditDialog 摘要行实时呈现；存档仍是内核
// And/Or/Not 树（行列表↔树互转见 ruleFormModel.treeToRows/rowsToTree）。
import RulePatternNode from './RulePatternNode.vue'
import { defaultPatternNode, type PatternRow } from './ruleFormModel'

const props = defineProps<{
  rows: PatternRow[]
}>()

function addLeaf(): void {
  props.rows.push({
    conn: props.rows.length ? 'and' : '',
    negate: false,
    leaf: defaultPatternNode('regex'),
    group: null
  })
}

function addGroup(): void {
  // 新子组默认展开并预置一个叶子，避免空壳（空子组保存闸门会拦）。
  props.rows.push({
    conn: props.rows.length ? 'and' : '',
    negate: false,
    leaf: null,
    group: [{ conn: '', negate: false, leaf: defaultPatternNode('regex'), group: null }],
    open: true
  })
}

function removeRow(index: number): void {
  props.rows.splice(index, 1)
  if (props.rows.length) props.rows[0].conn = ''
}
</script>

<style scoped>
/* 行式接词（帧-04）：每行=接词/非/删 头行 + 内容体；主题令牌呈现 */
.li-pr {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.li-pr-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  padding: 8px 10px;
  background: var(--app-bg-surface);
}

.li-pr-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-pr-conn {
  width: 72px;
}

.li-pr-first {
  font-size: 12px;
  color: var(--app-text-secondary);
  width: 72px;
  flex-shrink: 0;
}

.li-pr-neg {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--app-text-regular);
  cursor: pointer;
}

.li-pr-neg.is-on {
  color: var(--app-color-danger);
  font-weight: 600;
}

.li-pr-del {
  margin-left: auto;
}

.li-pr-body {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding-left: 12px;
  border-left: 2px solid var(--app-border-color-lighter, var(--app-border-color-light));
}

.li-pr-group-toggle {
  align-self: flex-start;
  border: none;
  background: none;
  padding: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
  cursor: pointer;
}

.li-pr-empty {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-danger);
}

.li-pr-actions {
  display: flex;
  gap: 8px;
}
</style>
