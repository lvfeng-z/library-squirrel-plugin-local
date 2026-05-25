<template>
  <teleport to="#dialog-mount-point">
    <el-dialog
      v-model="visible"
      title="目录分类"
      width="420px"
      :close-on-click-modal="false"
      :close-on-press-escape="false"
      :show-close="false"
    >
      <div v-if="question" class="classify-panel">
        <div class="classify-question">
          <span class="classify-label">目录「{{ question.dirName }}」代表：</span>
        </div>
        <div class="classify-options">
          <el-button
            v-for="opt in question.options"
            :key="opt"
            @click="selectType(opt)"
          >
            {{ optionLabels[opt] || opt }}
          </el-button>
          <el-button type="info" @click="cancel">取消</el-button>
        </div>
      </div>
    </el-dialog>
  </teleport>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { Events } from '@wailsio/runtime'

interface ClassifyQuestion {
  level: number
  dirName: string
  options: string[]
}

const visible = ref(false)
const question = ref<ClassifyQuestion | null>(null)

const optionLabels: Record<string, string> = {
  author: '作者',
  tag: '标签',
  workName: '作品名',
  workSet: '作品集',
}

function handleRequest(data: any) {
  question.value = data
  visible.value = true
}

function selectType(type: string) {
  if (!question.value) return
  Events.Emit('plugin:local-import:classify:response', {
    level: question.value.level,
    dirName: question.value.dirName,
    type,
  })
  visible.value = false
  question.value = null
}

function cancel() {
  if (!question.value) return
  Events.Emit('plugin:local-import:classify:response', {
    level: question.value.level,
    dirName: question.value.dirName,
    type: 'unknown',
    cancel: true,
  })
  visible.value = false
  question.value = null
}

let offFn: (() => void) | null = null

onMounted(() => {
  offFn = Events.On('plugin:local-import:classify:request', (event: any) => {
    handleRequest(event.data)
  })
})

onUnmounted(() => {
  if (offFn) offFn()
})
</script>

<style scoped>
.classify-question {
  margin-bottom: 16px;
}

.classify-label {
  font-size: 15px;
  color: #303133;
}

.classify-options {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
</style>
