<template>
  <div class="li-sub">
    <header class="li-sub-header">
      <h2 class="li-sub-title">规则管理</h2>
      <span class="li-sub-state">骨架占位</span>
    </header>
    <p class="li-sub-desc">
      规则档案的列表、启停、删除与编辑（列表+表单+JSON 混合形态），全部在工作台
      完成——规则维护不再需要停库直写。编辑事务经会话管理器单点串行。属后续阶段。
    </p>
    <a class="li-sub-back" href="#/com.lvfeng.localImport/workbench-parse">返回解析视图</a>
  </div>
</template>

<script setup lang="ts">
// 规则管理子页（骨架占位）：阶段6 填充档案维护面
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
