<template>
  <teleport to="#dialog-mount-point">
    <el-dialog
      v-model="visible"
      title="解释路径含义"
      width="520px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
    >
      <div class="explain-panel">
        <el-text class="explain-path-text">{{ question?.dirName }}</el-text>
        <el-scrollbar class="explain-scroll">
          <div v-for="(meaning, index) in meanings" :key="index" class="explain-row">
            <el-select v-model="meaning.type" class="explain-type-select" @change="resetName(meaning)">
              <el-option v-for="opt in meaningTypes" :key="opt.value" :value="opt.value" :label="opt.label" />
            </el-select>
            <el-input v-model="meaning.name" class="explain-name-input" clearable />
            <el-button icon="Remove" @click="removeMeaning(index)" />
          </div>
        </el-scrollbar>
      </div>
      <template #footer>
        <el-button type="success" icon="CirclePlus" @click="addMeaning">新增</el-button>
        <el-button type="primary" @click="confirm">确定</el-button>
        <el-button @click="cancel">取消</el-button>
      </template>
    </el-dialog>
  </teleport>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

interface ClassifyQuestion {
  level: number
  dirName: string
}

interface PathMeaning {
  type: string
  name: string
}

const meaningTypes = [
  { value: 'author', label: '作者' },
  { value: 'tag', label: '标签' },
  { value: 'workName', label: '作品名称' },
  { value: 'workSet', label: '作品集名称' },
  { value: 'site', label: '站点名称' },
  { value: 'unknown', label: '未知/无含义' }
]

const visible = ref(false)
const question = ref<ClassifyQuestion | null>(null)
const meanings = ref<PathMeaning[]>([])

function addMeaning() {
  meanings.value.push({ type: 'unknown', name: question.value?.dirName || '' })
}

function removeMeaning(index: number) {
  meanings.value.splice(index, 1)
}

function resetName(meaning: PathMeaning) {
  meaning.name = question.value?.dirName || ''
}

function handleRequest(data: ClassifyQuestion) {
  question.value = data
  meanings.value = [{ type: 'unknown', name: data.dirName }]
  visible.value = true
}

function confirm() {
  if (!question.value) return
  Events.Emit('plugin:local-import:classify:response', {
    level: question.value.level,
    dirName: question.value.dirName,
    meanings: meanings.value
  })
  visible.value = false
  question.value = null
}

function cancel() {
  if (!question.value) return
  Events.Emit('plugin:local-import:classify:response', {
    level: question.value.level,
    dirName: question.value.dirName,
    cancel: true
  })
  visible.value = false
  question.value = null
}

let offFn: (() => void) | null = null

onMounted(() => {
  console.log('[ClassifyPanel] 已挂载，开始监听 classify:request 事件')
  offFn = Events.On('plugin:local-import:classify:request', (event: any) => {
    console.log('[ClassifyPanel] 收到分类请求:', event.data)
    handleRequest(event.data)
  })
})

onUnmounted(() => {
  if (offFn) offFn()
})
</script>

<style scoped>
.explain-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.explain-path-text {
  font-size: 14px;
  color: #606266;
  word-break: break-all;
}
.explain-scroll {
  max-height: 300px;
}
.explain-row {
  display: flex;
  gap: 8px;
  align-items: center;
  margin-bottom: 8px;
}
.explain-type-select {
  width: 140px;
  flex-shrink: 0;
}
.explain-name-input {
  flex: 1;
}
</style>
