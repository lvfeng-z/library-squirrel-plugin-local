<template>
  <div class="li-sub">
    <header class="li-sub-header">
      <h2 class="li-sub-title">手动解析</h2>
      <span class="li-sub-state">骨架占位</span>
    </header>
    <p class="li-sub-desc">
      对未识别或识别错的资源手动指定结果：单次赋值（特异性，留会话）、泛化成规则
      （普适，入档案）、冲突裁决（生成组合规则）。三动作与生成后的增量重识别差异
      高亮属后续阶段。
    </p>
    <a class="li-sub-back" href="#/com.lvfeng.localImport/workbench-parse">返回解析视图</a>
  </div>
</template>

<script setup lang="ts">
// 手动解析子页（骨架占位）：阶段3 填充赋值/泛化/裁决三动作
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
