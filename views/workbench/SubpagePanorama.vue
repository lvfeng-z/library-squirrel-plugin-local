<template>
  <div class="li-sub">
    <header class="li-sub-header">
      <h2 class="li-sub-title">识别全景</h2>
      <span class="li-sub-state">骨架占位</span>
    </header>
    <p class="li-sub-desc">
      对库根做全量识别干跑后呈现识别全景：目录结构含义、已识别资源、有价值未识别
      资源、兄弟序列、待裁决项与规则覆盖统计。全量识别引擎与全景呈现属后续阶段。
    </p>
    <a class="li-sub-back" href="#/com.lvfeng.localImport/workbench-parse">返回解析视图</a>
  </div>
</template>

<script setup lang="ts">
// 识别全景子页（骨架占位）：阶段2 填充全量识别与七类元素呈现
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
