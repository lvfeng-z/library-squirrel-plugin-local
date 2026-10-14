<template>
  <div class="li-sub">
    <header class="li-sub-header">
      <h2 class="li-sub-title">解析观测</h2>
      <span class="li-sub-state li-sub-state-wip">建设中</span>
    </header>
    <p class="li-sub-desc">
      资源与元数据关联的可视化与利用率观测（解析观测面）规划中，本页为其保留的
      挂载位——功能建设期间呈现「建设中」占位。
    </p>
    <a class="li-sub-back" href="#/com.lvfeng.localImport/workbench-parse">返回解析视图</a>
  </div>
</template>

<script setup lang="ts">
// 解析观测占位子页：谱系延后分支 H（解析观测面任务）的挂载位，建设期渲染「建设中」
</script>

<style scoped>
.li-sub {
  max-width: 720px;
  margin: 0 auto;
  padding: 32px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  align-items: flex-start;
}

.li-sub-header {
  display: flex;
  align-items: center;
  gap: 10px;
}

.li-sub-title {
  margin: 0;
  font-size: 20px;
  color: var(--app-text-primary);
}

.li-sub-state {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-tag-neutral-text);
}

.li-sub-state-wip {
  background: var(--app-tag-orange-bg);
  color: var(--app-tag-orange-text);
}

.li-sub-desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--app-text-regular);
}

.li-sub-back {
  font-size: 13px;
  color: var(--app-color-primary);
}
</style>
