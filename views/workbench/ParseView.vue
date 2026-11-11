<template>
  <BaseView>
    <div class="li-ws">
    <!-- 头部：解析工作台 · 库根 · 识别于 · 重新识别 · 待裁决·决策态融合弹窗入口 · 子页链接 -->
    <header class="li-ws-header">
      <h2 class="li-ws-title">解析工作台</h2>
      <span v-if="session" class="li-ws-root">{{ session.rootPath }}</span>
      <span v-if="snapshot" class="li-ws-time">识别于 {{ formatTime(snapshot.generatedAtMs) }}</span>
      <span class="li-ws-header-actions">
        <!-- 创建任务/开始全量识别移页头（2026-10-27 裁决A：统计行并入卡头行，
             卡头动作钮让位；与重新识别同排，全部 el-button small 统一款式） -->
        <el-button
          v-if="snapshot"
          type="primary"
          size="small"
          :loading="creating"
          :disabled="selectedWorks.size === 0"
          @click="openCreateDialog"
        >创建任务</el-button>
        <el-button
          v-else-if="!sessionRestoring && !recognizing && !panoLoading && !panoLoadFailed"
          type="primary"
          size="small"
          :loading="starting"
          @click="startRecognition"
        >开始全量识别</el-button>
        <el-button
          v-if="snapshot"
          size="small"
          :loading="starting"
          @click="startRecognition"
        >重新识别</el-button>
        <!-- 待裁决·决策态融合弹窗入口：徽标计数=待裁决项数，弹窗承载只读总览与清单行跳转 -->
        <el-button
          v-if="snapshot"
          size="small"
          @click="fusionDialogVisible = true"
        >⚑ 待裁决·决策态 {{ pendingCount }}</el-button>
        <el-button size="small" @click="goSub('workbench-rules')">规则管理</el-button>
        <el-button size="small" @click="goSub('workbench-observation')">解析观测</el-button>
      </span>
    </header>

    <!-- 横幅区：加载失败 / 档案外变（仅异常态占位，常态不占行） -->
    <el-alert
      v-if="loadError"
      class="li-ws-banner"
      type="error"
      :closable="false"
      show-icon
      :title="loadError"
    >
      <a class="li-ws-back" :href="`${ROUTE_BASE}workbench`">返回本地导入工作台</a>
    </el-alert>
    <el-alert
      v-else-if="profileChanged && session"
      class="li-ws-banner"
      type="warning"
      :closable="false"
      show-icon
      title="规则档案在会话外被变更"
      description="既有识别结果可能与最新规则不一致，建议重新识别。"
    />
    <!-- 识别失败横幅（页头下方，常态不占行） -->
    <el-alert
      v-if="recognError"
      class="li-ws-banner"
      type="error"
      :closable="false"
      show-icon
      :title="recognError"
    />
    <!-- 缺陷①修复（动作通知存活层）：解析面板动作通知在面板不在场期由此横幅呈现
         ——动作应答后 emit('refresh') 触发 loadPanorama 同步 snapshot=null，右栏
         v-if 同批卸载面板，通知随实例消亡曾永不进 DOM（帧D-2）；面板回场（快照
         就绪）后交还面板内 alert 呈现（noticeSeed 回填），本横幅即隐去不双呈 -->
    <el-alert
      v-if="session && panelNotice && !(snapshot && !rightCollapsed)"
      class="li-ws-banner"
      type="success"
      :closable="false"
      show-icon
      :title="panelNotice"
    />

    <!-- 空会话态 -->
    <el-empty
      v-if="!session && !loadError && !sessionRestoring"
      description="未找到进行中的解析会话：请从工作台新建或恢复会话"
    >
      <a class="li-ws-back" :href="`${ROUTE_BASE}workbench`">返回本地导入工作台</a>
    </el-empty>

    <template v-if="session || sessionRestoring">
      <!-- 重识别差异行（单行小字，验证轮提示语并入同行，不占独立卡片） -->
      <p v-if="snapshot && diff && diffTotal > 0" class="li-ws-diffline">
        重识别差异（生成规则后的验证轮）：
        <span class="li-ws-diff-chip is-changed">变更 {{ diff.changed ? diff.changed.length : 0 }}</span>
        <span class="li-ws-diff-chip is-added">新增 {{ diff.added ? diff.added.length : 0 }}</span>
        <span class="li-ws-diff-chip is-removed">消失 {{ diff.removed ? diff.removed.length : 0 }}</span>
        <span>过窄＝差异里仍有未覆盖、过泛＝差异里错套——请在清单核对标色行</span>
      </p>

      <!-- 双栏主从主体（视口锁定）：左=资源清单卡 flex 自适应（min-width:0），右=解析面板卡
           固定 400px；align-items:stretch 令双卡等高填满——各自卡内滚动，页面级滚动归零 -->
      <section class="li-ws-main">
        <div class="li-ws-left">
          <!-- 资源清单卡（唯一左卡）：卡头=已选计数+创建任务入口（原选择创建栏融入，无快照时
                为开始全量识别入口）；卡体在无快照时承载引导/识别进度/空态占位，有快照时=
                过滤器行+统计行+模式切换+三标签滚动体+卡底创建反馈条 -->
          <section class="li-ws-card">
            <!-- 卡头行（2026-10-27 裁决A）：标题+已选计数+统计行同排（flex-wrap 窄窗折行）；
                 创建任务/开始全量识别已移页头 -->
            <h3 class="li-ws-card-title">
              资源清单
              <template v-if="snapshot"><span class="li-ws-create-count">已选 {{ selectedWorks.size }} 件作品<span class="li-ws-create-sub">（可选 {{ selectableWorks.length }} / 共 {{ allWorks.length }}）</span></span></template>
              <span v-if="snapshot" class="li-ws-card-stats">文件 {{ snapshot.stats.totalFiles }} · 已识别 {{ snapshot.stats.identified }} · 有线索 {{ snapshot.stats.clueful }} · 无线索 {{ snapshot.stats.plain }} · 序列群 {{ snapshot.stats.groupCount }} · 待裁决 {{ snapshot.stats.pendingFiles }}<template v-if="snapshot.stats.skippedDirs > 0"> · 增量跳过目录 {{ snapshot.stats.skippedDirs }}</template></span>
            </h3>
            <!-- 识别中：流式进度（阶段跃迁 + 计数）；赋值变更后的自动增量重识别共用此状态条 -->
            <div v-if="recognizing" class="li-ws-progress">
              <div class="li-ws-progress-head">
                <span class="li-ws-progress-phase">{{ phaseLabel(progress.phase) }}</span>
                <span v-if="progress.total > 0" class="li-ws-progress-count">{{ progress.done }} / {{ progress.total }}</span>
              </div>
              <div class="li-ws-progress-track">
                <div class="li-ws-progress-bar" :style="{ width: progressPercent }"></div>
              </div>
            </div>
            <template v-if="snapshot">
              <!-- 标签页行（2026-10-27 重整：三项置顶，其下=按标签页适配的筛选工具栏）；
                   会话内记忆不持久化 -->
              <div class="li-ws-mode-toggle">
                <button
                  type="button"
                  class="li-ws-filter-btn li-ws-mode-btn"
                  :class="{ 'is-active': listMode === 'tree' }"
                  title="目录树模式：库根 → 目录 → 文件层级归属"
                  @click="listMode = 'tree'"
                >⛁ 目录树</button>
                <button
                  type="button"
                  class="li-ws-filter-btn li-ws-mode-btn"
                  :class="{ 'is-active': listMode === 'flat' }"
                  title="平铺清单模式：全量文件平铺"
                  @click="listMode = 'flat'"
                >▦ 清单</button>
                <button
                  type="button"
                  class="li-ws-filter-btn li-ws-mode-btn"
                  :class="{ 'is-active': listMode === 'sets' }"
                  title="候选卡墙：目录候选 / 序列群 / 库内集比对（确认归集或忽略）"
                  @click="listMode = 'sets'"
                >疑似作品集</button>
              </div>
              <!-- 筛选工具栏（目录树/清单）：文件过滤器与批量/组合操作合并单行
                   （flex-wrap，窄窗自然折行）；对疑似作品集卡墙无作用故在该标签页下隐藏 -->
              <div v-show="listMode !== 'sets'" class="li-ws-combos">
                <button
                  v-for="f in fileFilters"
                  :key="f.key"
                  class="li-ws-filter-btn"
                  :class="{ 'is-active': fileFilter === f.key }"
                  @click="fileFilter = f.key"
                >{{ f.label }}</button>
                <button class="li-ws-filter-btn" @click="selectAllVisible">全选可见</button>
                <button class="li-ws-filter-btn" @click="invertVisible">反选可见</button>
                <select v-model="rulePick" class="li-ws-combo-select" @change="onRulePick">
                  <option value="">按规则命中选…</option>
                  <option v-for="r in ruleOptions" :key="r.id" :value="r.id">{{ ruleNameOf(r.id) }}（命中 {{ r.hitCount }}）</option>
                </select>
                <select v-model="authorPick" class="li-ws-combo-select" @change="onAuthorPick">
                  <option value="">按作者声明选…</option>
                  <option v-for="a in authorOptions" :key="a" :value="a">{{ a }}</option>
                </select>
                <button class="li-ws-filter-btn" @click="clearSelection">清空选择</button>
                <button
                  v-if="rightCollapsed"
                  class="li-ws-filter-btn li-ws-panel-reopen"
                  @click="rightCollapsed = false"
                >« 展开解析面板</button>
              </div>
              <!-- 筛选工具栏（疑似作品集·适配）：隐藏已识别上移自卡墙顶部过滤条
                   （口径=候选成员全部已带集维度最终结论=已被识别为作品集） -->
              <div v-show="listMode === 'sets'" class="li-ws-combos">
                <button
                  type="button"
                  class="li-ws-filter-btn"
                  :class="{ 'is-active': hideIdentifiedCandidates }"
                  :title="hideIdentifiedCandidates ? `已隐藏 ${identifiedCandidateCount} 条成员均已识别出作品集（集维度结论在档）的候选` : '隐藏成员均已识别出作品集（集维度结论在档）的候选'"
                  @click="hideIdentifiedCandidates = !hideIdentifiedCandidates"
                >隐藏已识别{{ identifiedCandidateCount ? ` ${identifiedCandidateCount}` : '' }}</button>
                <button
                  v-if="rightCollapsed"
                  class="li-ws-filter-btn li-ws-panel-reopen"
                  @click="rightCollapsed = false"
                >« 展开解析面板</button>
              </div>
              <!-- 平铺模式（虚拟滚动切片渲染；滚动承载=el-scrollbar 宿主样式条，高度由卡内
                    flex 链分配，切片窗口随 wrap 实测高度持续同步；模式切换时过滤与选择集保持）。
                    2026-10-27 滚轮缺陷修复：虚拟化改为「头/尾静态占位条夹切片行」，行回归常规流
                    （原绝对定位+translateY 会被 WebView2 合成器滚轮命中链切断，滚轮失效）。
                     三标签体（平铺/树/卡墙）v-if 惰性挂载：未打开标签零 DOM 零布局；
                     重挂=全新滚动体，切片基准 scrollTop 与卡墙渲染上限随重挂重置（见 script） -->
              <el-scrollbar
                v-if="listMode === 'flat'"
                ref="listViewport"
                class="li-ws-list"
                @scroll="onListScroll"
              >
                <div class="li-ws-list-spacer" :style="{ height: `${visibleRange.start * ROW_H}px` }"></div>
                <!-- 行 = 选择复选（作品级）+ 状态点 + 路径 + 声明/线索摘要 + 差异标 + 已赋值标；
                     行点击 = 选中联动右栏（选中键 = 文件相对路径），与复选不互斥 -->
                <div
                  v-for="row in visibleFiles"
                  :key="row.relPath"
                  class="li-ws-row"
                  :class="row.relPath === selectedRelPath ? 'is-selected' : (diffMark(row.relPath) ? `is-diff-${diffMark(row.relPath)}` : '')"
                  @click="selectFile(row.relPath)"
                >
                  <el-checkbox
                    class="li-ws-cb"
                    :model-value="isWorkSelected(workIdOf(row.relPath))"
                    :disabled="!isWorkSelectable(row.relPath)"
                    :title="workHintOf(row.relPath)"
                    @click.stop
                    @change="() => toggleWorkAt(row.relPath)"
                  />
                  <span class="li-ws-row-state" :class="`is-${row.state}`" :title="stateLabel(row.state)">{{ stateDot(row.state) }}</span>
                  <span class="li-ws-row-path" :title="row.relPath">{{ row.relPath }}</span>
                  <span v-if="diffMark(row.relPath)" class="li-ws-row-diff" :class="`is-${diffMark(row.relPath)}`">{{ diffMark(row.relPath) === 'changed' ? 'Δ变更' : diffMark(row.relPath) === 'added' ? '＋新增' : '－消失' }}</span>
                  <span class="li-ws-row-meta">
                    <template v-if="row.final && row.final.length">
                      <span v-for="(d, i) in row.final" :key="i" class="li-ws-row-decl">{{ declLabel(d) }}</span>
                    </template>
                    <template v-if="row.clues && row.clues.length">
                      <span v-for="(c, i) in row.clues" :key="i" class="li-ws-row-clue">{{ clueLabel(c) }}</span>
                    </template>
                    <span v-if="row.pendingDims && row.pendingDims.length" class="li-ws-row-pending">待裁决:{{ row.pendingDims.join('/') }}</span>
                    <span v-if="row.assigned" class="li-ws-row-assigned">已赋值</span>
                  </span>
                </div>
                <!-- 尾部占位条：与头部占位条相加=全列表高度（见 visibleRange 注释） -->
                <div class="li-ws-list-spacer" :style="{ height: `${Math.max(0, (filteredFiles.length - visibleRange.end) * ROW_H)}px` }"></div>
                <p v-if="filteredFiles.length === 0" class="li-ws-empty">当前过滤条件下无文件</p>
              </el-scrollbar>
              <!-- 树模式（库根 → 目录 → 文件全量渲染，行级数据量不虚拟化；滚动承载=el-scrollbar）；
                   目录行 = 展开箭头 + 复选框（子树递选，按作品聚合进现有选择集）+ 目录名（点 = 右栏目录解析面板）
                   + 文件数 + 含义覆盖标记；
                   文件行 = 行首作品级复选（勾选/禁用语义同平铺，目录行全选/半选态由选择集派生）+ 平铺同款元数据，点行 = 现有联动 -->
              <el-scrollbar v-if="listMode === 'tree'" ref="treeViewport" class="li-ws-tree">
                <template v-for="row in treeRows" :key="row.key">
                  <div v-if="row.kind === 'dir' && row.dir" class="li-ws-tree-dir" :style="{ paddingLeft: `${row.depth * 16 + 4}px` }">
                    <button
                      type="button"
                      class="li-ws-tree-arrow"
                      :class="{ 'is-open': treeExpanded.has(row.dir.relKey) }"
                      :title="treeExpanded.has(row.dir.relKey) ? '收起目录' : '展开目录'"
                      @click="toggleTreeDir(row.dir.relKey)"
                    >▸</button>
                    <el-checkbox
                      class="li-ws-dir-cb"
                      :model-value="dirTreeChecked(row.dir.relKey)"
                      :indeterminate="dirTreeIndeterminate(row.dir.relKey)"
                      :title="`子树递选（按作品聚合）· 已选 ${dirTreeSelectedText(row.dir.relKey)}`"
                      @change="(v) => setDirTreeSelected(row.dir.relKey, !!v)"
                    />
                    <span class="li-ws-tree-name" :title="`${row.dir.relKey || '（库根）'}（点开目录解析面板）`" @click="showDirCard(row.dir.relKey)">{{ row.dir.name }}</span>
                    <span class="li-ws-dir-files">{{ row.dir.fileCount }} 文件</span>
                    <template v-if="row.dir.meanings && row.dir.meanings.length">
                      <span v-for="(m, mi) in row.dir.meanings" :key="mi" class="li-ws-dir-meaning">{{ meaningLabel(m) }}</span>
                    </template>
                    <span v-else-if="row.depth > 0" class="li-ws-dir-uncovered">未覆盖</span>
                  </div>
                  <div
                    v-else-if="row.kind === 'file' && row.file"
                    class="li-ws-tree-file"
                    :class="row.file.relPath === selectedRelPath ? 'is-selected' : (diffMark(row.file.relPath) ? `is-diff-${diffMark(row.file.relPath)}` : '')"
                    :style="{ paddingLeft: `${row.depth * 16 + 30}px` }"
                    @click="selectFile(row.file.relPath)"
                  >
                    <!-- H6：行首复选走作品级选择集通路（与平铺行同款），阻止冒泡以免触发行选中联动 -->
                    <el-checkbox
                      class="li-ws-cb"
                      :model-value="isWorkSelected(workIdOf(row.file.relPath))"
                      :disabled="!isWorkSelectable(row.file.relPath)"
                      :title="workHintOf(row.file.relPath)"
                      @click.stop
                      @change="() => toggleWorkAt(row.file.relPath)"
                    />
                    <span class="li-ws-row-state" :class="`is-${row.file.state}`" :title="stateLabel(row.file.state)">{{ stateDot(row.file.state) }}</span>
                    <span class="li-ws-tree-fname" :title="row.file.relPath">{{ row.file.fileName }}</span>
                    <span v-if="diffMark(row.file.relPath)" class="li-ws-row-diff" :class="`is-${diffMark(row.file.relPath)}`">{{ diffMark(row.file.relPath) === 'changed' ? 'Δ变更' : diffMark(row.file.relPath) === 'added' ? '＋新增' : '－消失' }}</span>
                    <span class="li-ws-row-meta">
                      <template v-if="row.file.final && row.file.final.length">
                        <span v-for="(d, di) in row.file.final" :key="di" class="li-ws-row-decl">{{ declLabel(d) }}</span>
                      </template>
                      <template v-if="row.file.clues && row.file.clues.length">
                        <span v-for="(c, ci) in row.file.clues" :key="ci" class="li-ws-row-clue">{{ clueLabel(c) }}</span>
                      </template>
                      <span v-if="row.file.pendingDims && row.file.pendingDims.length" class="li-ws-row-pending">待裁决:{{ row.file.pendingDims.join('/') }}</span>
                      <span v-if="row.file.assigned" class="li-ws-row-assigned">已赋值</span>
                    </span>
                  </div>
                </template>
                <p v-if="treeRows.length === 0" class="li-ws-empty">当前过滤条件下无文件</p>
              </el-scrollbar>
              <!-- 疑似作品集候选卡墙（第三标签体；滚动承载=el-scrollbar）：三类候选合流出卡——
                   目录候选（整目录像一集）/序列群（结构/规则/页级）/库内集比对（集名互证徽标 +
                   作品号命中→归入既有集）；成员文件名序列块底色（区间=后端形态表单一事实源）；
                   两段式归集（预填可改）；已归集/已忽略收纳分组（会话内，忽略可恢复）；
                   确认走集维度批量赋值+既有建议卡通路 -->
              <el-scrollbar v-if="listMode === 'sets'" class="li-ws-sets">
                <div class="li-ws-sets-inner">
                <!-- 已识别候选过滤条已上移工具栏位（2026-10-27 重整：口径=候选成员
                     全部已带集维度最终结论〔规则识别或赋值存档〕=已被识别为作品集） -->
                <p
                  v-if="visibleCandidates.length === 0 && !(snapshot.ignoredCandidates || []).length && !(snapshot.confirmedCandidates || []).length"
                  class="li-ws-empty"
                >无疑似作品集候选{{ hideIdentifiedCandidates && identifiedCandidateCount ? `（已隐藏已识别 ${identifiedCandidateCount} 条）` : '' }}</p>
                <div
                  v-for="c in renderedCandidates"
                  :key="candKey(c)"
                  class="li-ws-cand"
                  :class="{ 'is-confirming': confirming === candTarget(c) }"
                >
                  <div class="li-ws-cand-head">
                    <span class="li-ws-cand-key">{{ c.key || '（空键）' }}</span>
                    <span class="li-ws-cand-dir">{{ c.dir || '根目录' }}</span>
                    <span class="li-ws-cand-size">{{ c.size }} 成员</span>
                    <span class="li-ws-cand-src">{{ candSourceLabel(c) }}</span>
                    <span v-if="c.crossMatch" class="li-ws-cand-cross" title="库内已有同名作品集（集名互证）">集名互证</span>
                  </div>
                  <p v-if="c.reason" class="li-ws-cand-reason">{{ c.reason }}</p>
                  <div class="li-ws-cand-members">
                    <span
                      v-for="mv in candMembers(c)"
                      :key="mv.m.relPath"
                      class="li-ws-cand-member"
                      :class="{ 'is-selected': mv.m.relPath === selectedRelPath }"
                      :title="mv.m.relPath"
                      @click="selectFile(mv.m.relPath)"
                    ><template v-if="mv.m.seqText"><span class="li-ws-seg-seq">#{{ mv.m.seqText }} </span></template><span
                        v-for="(s, si) in mv.parts.spans"
                        :key="si"
                        :class="s.kind ? `li-ws-seg-${s.kind}` : ''"
                      >{{ s.text }}</span><span class="li-ws-cand-ext">{{ mv.parts.ext }}</span></span>
                  </div>
                  <!-- 成员预览截断说明：后端投影层截前 panoramaMemberPreviewLimit=200 个成员
                       （size 恒为真总数），被截成员不可点击定位=可接受展示降级；
                       确认归集/忽略动作按目标键全量生效，不受截断影响 -->
                  <p v-if="c.size > (c.members || []).length" class="li-ws-cand-preview">仅预览前 {{ (c.members || []).length }} 个成员（共 {{ c.size }} 个）</p>
                  <div v-if="confirming === candTarget(c)" class="li-ws-cand-confirm">
                    <span class="li-ws-cand-namewrap">
                      <input v-model="confirmName" class="li-ws-cand-name" type="text" maxlength="200" placeholder="集名" :style="{ width: nameInputWidth }">
                      <button v-if="confirmName" type="button" class="li-ws-cand-clear" title="清空集名" :disabled="candBusy" @click="clearConfirmName">✕</button>
                      <span class="li-ws-cand-name-mirror" aria-hidden="true">{{ confirmName || '集名' }}</span>
                    </span>
                    <span v-if="c.existingSetId" class="li-ws-cand-existing">将归入既有集</span>
                    <button type="button" class="li-ws-filter-btn li-ws-cand-ok" :disabled="candBusy || !confirmName.trim()" @click="confirmCandidate(c)">确认归集</button>
                    <button type="button" class="li-ws-filter-btn" :disabled="candBusy" @click="confirming = ''">取消</button>
                    <div v-if="chipVariants(c).length" class="li-ws-cand-chips">
                      <button
                        v-for="(chip, chi) in chipVariants(c)"
                        :key="chi"
                        type="button"
                        class="li-ws-chip"
                        :class="{ 'is-shelled': chip.shelled }"
                        :title="chip.shelled ? '插入带壳形态' : '插入剥壳形态'"
                        :disabled="candBusy"
                        @click="insertChip(chip)"
                      >{{ chip.label }}</button>
                    </div>
                  </div>
                  <div v-else class="li-ws-cand-actions">
                    <button type="button" class="li-ws-filter-btn" :disabled="candBusy" @click="startConfirm(c)">归入一集</button>
                    <button type="button" class="li-ws-filter-btn" :disabled="candBusy" @click="ignoreCandidate(c)">忽略</button>
                  </div>
                </div>
                <!-- 增量渲染哨兵（性能修复）：候选卡墙 v-for 只渲染前缀切片
                     （renderedCandidates），本哨兵由 IntersectionObserver 盯守（含 400px
                     预取带），触底自动追加一批；清单见底即撤哨。空态判据仍用
                     visibleCandidates 全量清单，语义不变 -->
                <div
                  v-if="renderedCandidates.length < visibleCandidates.length"
                  ref="cardsSentinel"
                  class="li-ws-cands-more"
                >已显示 {{ renderedCandidates.length }}/{{ visibleCandidates.length }} 条候选，滚动载入更多…</div>
                <!-- 收纳分组：已归集（与待归集卡同构〔剧本1〕：键名=所在目录的目录解析
                     面板入口、成员文件名块联动右栏；右上「撤销归集」〔剧本2〕；确认集名随卡回带） -->
                <div v-if="(snapshot.confirmedCandidates || []).length" class="li-ws-cand-bin">
                  <button type="button" class="li-ws-fold-head" @click="binConfirmed = !binConfirmed">
                    <span class="li-ws-fold-arrow" :class="{ 'is-open': !binConfirmed }">▸</span>
                    <span class="li-ws-fold-title">已归集 {{ (snapshot.confirmedCandidates || []).length }}</span>
                  </button>
                  <div v-show="!binConfirmed" class="li-ws-cand-binbody">
                    <el-alert
                      v-if="candError"
                      class="li-ws-cand-error"
                      type="error"
                      :closable="false"
                      show-icon
                      :title="candError"
                    />
                    <div v-for="c in snapshot.confirmedCandidates || []" :key="`cf-${candKey(c)}`" class="li-ws-cand is-done">
                      <div class="li-ws-cand-head">
                        <span
                          class="li-ws-cand-key is-link"
                          title="打开所在目录的目录解析面板"
                          @click="showDirCard(c.dir)"
                        >{{ c.key || '（空键）' }}</span>
                        <span class="li-ws-cand-size">{{ c.size }} 成员</span>
                        <span class="li-ws-cand-src">{{ candSourceLabel(c) }}</span>
                        <span class="li-ws-cand-done">已归集「{{ c.confirmedSetName || c.key }}」</span>
                        <!-- 剧本C：已归集卡的集规则泛化入口——右栏切集规则面板（泛化主体=本集整体） -->
                        <button
                          type="button"
                          class="li-ws-filter-btn li-ws-cand-setrule"
                          :disabled="candBusy"
                          @click="openSetRule(c)"
                        >泛化集规则</button>
                        <button
                          type="button"
                          class="li-ws-filter-btn li-ws-cand-undo"
                          :disabled="candBusy"
                          @click="undoConfirm(c)"
                        >{{ undoing === candTarget(c) ? '撤销中…' : '撤销归集' }}</button>
                      </div>
                      <div class="li-ws-cand-members">
                        <span
                          v-for="mv in candMembers(c)"
                          :key="mv.m.relPath"
                          class="li-ws-cand-member"
                          :class="{ 'is-selected': mv.m.relPath === selectedRelPath }"
                          :title="mv.m.relPath"
                          @click="selectFile(mv.m.relPath)"
                        ><template v-if="mv.m.seqText"><span class="li-ws-seg-seq">#{{ mv.m.seqText }} </span></template><span
                          v-for="(s, si) in mv.parts.spans"
                          :key="si"
                          :class="s.kind ? `li-ws-seg-${s.kind}` : ''"
                        >{{ s.text }}</span><span class="li-ws-cand-ext">{{ mv.parts.ext }}</span></span>
                      </div>
                      <!-- 成员预览截断说明（已归集卡同待归集卡：size=真总数，动作全量生效） -->
                      <p v-if="c.size > (c.members || []).length" class="li-ws-cand-preview">仅预览前 {{ (c.members || []).length }} 个成员（共 {{ c.size }} 个）</p>
                    </div>
                  </div>
                </div>
                <!-- 收纳分组：已忽略（可恢复回候选区；会话内记忆不写档案） -->
                <div v-if="(snapshot.ignoredCandidates || []).length" class="li-ws-cand-bin">
                  <button type="button" class="li-ws-fold-head" @click="binIgnored = !binIgnored">
                    <span class="li-ws-fold-arrow" :class="{ 'is-open': !binIgnored }">▸</span>
                    <span class="li-ws-fold-title">已忽略 {{ (snapshot.ignoredCandidates || []).length }}</span>
                  </button>
                  <div v-show="!binIgnored" class="li-ws-cand-binbody">
                    <div v-for="c in snapshot.ignoredCandidates || []" :key="`ig-${candKey(c)}`" class="li-ws-cand is-done">
                      <div class="li-ws-cand-head">
                        <span class="li-ws-cand-key">{{ c.key || '（空键）' }}</span>
                        <span class="li-ws-cand-size">{{ c.size }} 成员</span>
                        <span class="li-ws-cand-src">{{ candSourceLabel(c) }}</span>
                        <button type="button" class="li-ws-filter-btn li-ws-cand-restore" :disabled="candBusy" @click="restoreCandidate(c)">恢复</button>
                      </div>
                    </div>
                  </div>
                </div>
                </div>
              </el-scrollbar>
              <!-- 创建反馈条落位区（卡底、常态不占行）：进度横幅（异步创建期，帧1/帧2
                   双行双计数+取消入口、帧3→帧4 清理中转场）、终态横幅（帧4 取消✔/
                   帧5 失败✖/帧6 完成✔）、黄字=悬空选择提示（重识别后选择集引用了
                   已不存在的作品） -->
              <!-- 创建异步进度横幅（剧本1·帧1/帧2）：createProgress 事件按 phase 分道
                   双计数（乙3 重叠期双行并存——核对行核对完成即消失、发射行自首批发射
                   起出现至终值）；核对行携带读盘速率与前端估算预计剩余（甲1）；取消
                   入口双行期恒在（甲3）。创建在插件侧独立 goroutine 执行，期间清单/
                   目录树可继续浏览（不阻塞） -->
              <div v-if="creating" class="li-ws-create-progress">
                <template v-if="createCancelling">
                  <!-- 帧3→帧4 转场：取消已受理，等待在途回滚清理与终态事件 -->
                  <p class="li-ws-progress-phase">正在清理在途任务…</p>
                </template>
                <template v-else>
                  <div v-if="createVerify && !createVerifyFinished" class="li-ws-progress-head">
                    <span class="li-ws-progress-phase">正在核对文件…{{ createVerifyCount }}</span>
                    <span class="li-ws-progress-count">{{ createVerifyPercent }}</span>
                  </div>
                  <p v-if="createVerifyMeta" class="li-ws-progress-meta">{{ createVerifyMeta }}</p>
                  <div v-if="createVerify && !createVerifyFinished && createVerify.total > 0" class="li-ws-progress-track">
                    <div class="li-ws-progress-bar" :style="{ width: createVerifyPercent }"></div>
                  </div>
                  <div v-if="createEmit" class="li-ws-progress-head">
                    <span class="li-ws-progress-phase">正在创建子任务… {{ fmtCount(createEmit.done) }} 个</span>
                  </div>
                  <p v-if="createCancelRaceNote" class="li-ws-progress-note">{{ createCancelRaceNote }}</p>
                  <el-button size="small" class="li-ws-cancel-create" @click="openCancelConfirm">取消创建</el-button>
                </template>
              </div>
              <!-- 创建终态横幅（剧本1·帧4/帧5/帧6）：文案优先消费后端 createDone Message
                   ——帧6 完成（✔）「已创建任务「名」：N 件作品，共 M 个子任务，用时
                   X 分钟」（含归属附注）、帧4 取消（✔）「已取消创建：在途任务已清理
                   （已删 N 个子任务）…」、帧5 失败（✖）「任务创建失败：…；在途任务
                   已清理…，可重试」；失败/取消后选择集保留（重试走帧1） -->
              <el-alert
                v-if="createDoneMessage"
                class="li-ws-banner"
                :type="createDoneKind === 'failed' ? 'error' : 'success'"
                :closable="false"
                show-icon
                :title="createDoneMessage"
              />
              <el-alert
                v-else-if="createError"
                class="li-ws-banner"
                type="error"
                :closable="false"
                show-icon
                :title="createError"
              />
              <p v-if="danglingSelection > 0" class="li-ws-stale-hint">
                选择集中 {{ danglingSelection }} 项已不在当前识别全景内（可能已重识别），创建前请刷新重选
              </p>
            </template>
            <!-- 载入态（大载荷分页方案·剧本1·帧1/帧2）：分节分页拉取进行中/中断——
                 专用载入态替换「尚未生成识别全景」误导空态（决策3=整体装载：全部
                 分节就绪前不呈现任何清单数据，解析面板等其余区域空置） -->
            <div v-else-if="panoLoading || panoLoadFailed" class="li-ws-pano-load">
              <template v-if="panoLoading">
                <p class="li-ws-pano-load-title">正在载入识别全景…</p>
                <div class="li-ws-progress-track">
                  <div class="li-ws-progress-bar" :style="{ width: panoProgressPercent }"></div>
                </div>
                <p class="li-ws-pano-load-count">
                  <span>{{ panoProgressPercent }}</span>
                  <span>已接收 {{ panoReceivedCount }}/{{ panoTotalCount }} 项</span>
                </p>
              </template>
              <template v-else>
                <p class="li-ws-pano-load-title is-failed">载入中断</p>
                <p class="li-ws-pano-load-count">第 {{ panoFailedPage }}/{{ panoEstPages }} 页失败</p>
                <el-button
                  size="small"
                  type="primary"
                  :loading="panoResuming"
                  @click="resumePanoLoad"
                >从断点继续</el-button>
              </template>
            </div>
            <!-- 会话恢复中（2026-11-06 BA / 样式修订）：占位落在资源清单卡内——与随后
                 panoLoading 载入态同卡、同位置、同底色，恢复完成时只换文案与进度条，
                 不再从「无卡页面」跳到「白底卡」造成观感跳变 -->
            <div v-else-if="sessionRestoring" class="li-ws-pano-load">
              <p class="li-ws-pano-load-title">正在载入解析会话…</p>
              <p class="li-ws-pano-load-count">正在读取会话与识别快照</p>
            </div>
            <!-- 边界：未识别会话（无快照）→ 引导说明 + 空态占位（干跑 = 纯求值，零任务产出、零弹问） -->
            <div v-else-if="!recognizing" class="li-ws-nosnap">
              <p class="li-ws-desc">
                对库根做全量识别干跑：按「规则档案 ∪ 会话赋值」合并求值全部文件，产出
                识别全景（目录结构含义、已识别资源、有价值未识别、序列群、待裁决项、
                规则命中与决策态）。识别不创建任何任务。
              </p>
              <el-empty description="尚未生成识别全景" />
            </div>
          </section>
        </div>

          <!-- 右栏解析面板卡（固定 ~400px、可收起；高度由主体 stretch 链锁定，卡内滚动）：
               随选中联动显示文件路径与状态，赋值表单/泛化/裁决完整交互见 ParsePanel.vue；
               目录解析面板（点树中目录名进入）亦由 ParsePanel 目录模式承载 -->
          <aside v-if="snapshot && !rightCollapsed" class="li-ws-right">
            <div class="li-ws-panel-head">
              <span class="li-ws-panel-title">解析面板</span>
              <button type="button" class="li-ws-panel-btn" title="收起面板" @click="rightCollapsed = true">收起 »</button>
            </div>
            <!-- 未保存切换确认条：表单有未保存改动时切换选中目标先暂存，二选一后放行 -->
            <div v-if="pendingSwitch" class="li-ws-switch-confirm">
              <span>解析面板有未保存的改动，切换将丢弃</span>
              <span class="li-ws-switch-confirm-actions">
                <button type="button" class="li-ws-panel-btn is-primary" @click="applyPendingSwitch">丢弃并切换</button>
                <button type="button" class="li-ws-panel-btn" @click="pendingSwitch = null">留在当前去保存</button>
              </span>
            </div>
            <!-- 面板内容滚动体：卡内 flex 链分配高度，宿主样式条内滚 -->
            <el-scrollbar class="li-ws-panel-scroll">
              <div class="li-ws-panel-body">
              <!-- 剧本C 集规则面板（已归集卡「泛化集规则」进入）：泛化主体=集整体
                   （target 寻址 confirmed 分仓卡），推导/命中预览/泛化入档/调整面全由
                   SetRulePanel 承载；与文件/目录面板互斥——点文件行/目录名让出、点
                   成员行（面板内）切到该文件的表单 -->
              <SetRulePanel
                v-if="setPanelTarget"
                :key="setPanelTarget"
                ref="setPanelRef"
                :root="session ? session.rootPath : ''"
                :target="setPanelTarget"
                :set-name="setPanelName"
                :diff="diff"
                :recognizing="recognizing"
                :request="request"
                @refresh="onPanelRefresh"
                @select-member="onSetMemberSelected"
                @exit="exitSetPanel"
              />
              <!-- 剧本J 目录解析面板（卡+解析合一，保存作用子树）：目录头信息+线索胶囊+
                   五字段表单+目录保存/清除+建议卡，全部由 ParsePanel 目录模式承载
                  （dir 上下文由本组件组装传入）；与文件面板互斥——点树中文件行切回
                   文件面板、点目录名回来，不联动文件表单（J7） -->
              <ParsePanel
                v-else-if="dirPanelContext"
                ref="dirPanelRef"
                :root="session ? session.rootPath : ''"
                :dir="dirPanelContext"
                :assignments="assignmentList"
                :ignored="false"
                :rule-names="ruleNameMap"
                :pending-items="[]"
                :diff="diff"
                :recognizing="recognizing"
                :request="request"
                :notice-seed="panelNotice"
                :suggestion-seed="panelSuggestion"
                @notice="onPanelNotice"
                @suggestion="onPanelSuggestion"
                @refresh="onPanelRefresh"
                @exit-dir="exitDirPanel"
                @open-rule="openRuleEdit"
              />
              <!-- 边界：右栏空态引导 -->
              <div v-else-if="!selectedRelPath" class="li-ws-panel-guide">
                <p class="li-ws-panel-guide-title">未选中文件</p>
                <p class="li-ws-panel-guide-text">
                  点击左栏清单中任意文件行，此处将呈现该文件的解析面板：
                  识别结果、线索、所属序列群与赋值表单。
                </p>
              </div>
              <!-- 边界：选中文件已从当前快照消失（重识别后） -->
              <div v-else-if="!selectedFile" class="li-ws-panel-missing">
                <p class="li-ws-panel-path">{{ selectedRelPath }}</p>
                <p class="li-ws-panel-missing-text">文件已不在当前全景（可能已被重识别移除），请重新选择。</p>
                <el-button size="small" @click="selectedRelPath = ''">清除选中</el-button>
              </div>
              <template v-else>
                <!-- 文件头：状态点 + 相对路径（选中键）+ 状态文案 -->
                <div class="li-ws-file-head">
                  <span class="li-ws-row-state" :class="`is-${selectedFile.state}`" :title="stateLabel(selectedFile.state)">{{ stateDot(selectedFile.state) }}</span>
                  <span class="li-ws-panel-path" :title="selectedFile.relPath">{{ selectedFile.relPath }}</span>
                  <span class="li-ws-file-state">{{ stateLabel(selectedFile.state) }}</span>
                </div>
                <div class="li-ws-sec">
                  <h4 class="li-ws-sec-title">识别结果</h4>
                  <template v-if="selectedFile.final && selectedFile.final.length">
                    <div v-for="(d, i) in selectedFile.final" :key="i" class="li-ws-decl">
                      <span class="li-ws-decl-main">{{ declLabel(d) }}</span>
                      <span class="li-ws-decl-src">
                        <span class="li-ws-decl-src-text">{{ sourceLabel(d.source) }}<template v-if="d.ruleId"> · <a class="li-ws-rule-link" :href="ruleEditRoute(d.ruleId)" :title="`跳规则管理子页并打开「${ruleNameOf(d.ruleId)}」编辑`">{{ ruleNameOf(d.ruleId) }}</a></template></span>
                        <button
                          v-if="(d.alsoRuleIds || []).length"
                          type="button"
                          class="li-ws-same-toggle"
                          @click="toggleSameExpand(i)"
                        >{{ sameToggleLabel((d.alsoRuleIds || []).length, isSameExpanded(i)) }}</button>
                      </span>
                      <div v-if="(d.alsoRuleIds || []).length && isSameExpanded(i)" class="li-ws-same-list">
                        <p v-for="rid in d.alsoRuleIds" :key="rid" class="li-ws-same-item">· <a class="li-ws-rule-link" :href="ruleEditRoute(rid)" :title="`跳规则管理子页并打开「${ruleNameOf(rid)}」编辑`">{{ ruleNameOf(rid) }}</a></p>
                      </div>
                    </div>
                  </template>
                  <p v-else class="li-ws-empty">无识别结果</p>
                </div>
                <div class="li-ws-sec">
                  <h4 class="li-ws-sec-title">{{ hitRuleCount ? `命中规则（${hitRuleCount}）` : '命中规则' }}</h4>
                  <div v-if="hitRuleCount" class="li-ws-hit-rules">
                    <a
                      v-for="rid in selectedFile.matchedRuleIds || []"
                      :key="rid"
                      class="li-ws-hit-pill"
                      :href="ruleEditRoute(rid)"
                      :title="`跳规则管理子页并打开「${ruleNameOf(rid)}」编辑`"
                    >{{ ruleNameOf(rid) }}</a>
                  </div>
                  <p v-else class="li-ws-empty">无命中规则</p>
                </div>
                <div class="li-ws-sec">
                  <h4 class="li-ws-sec-title">所属序列群</h4>
                  <div v-if="selectedGroup" class="li-ws-sec-block">
                    <p class="li-ws-sec-line">集 {{ selectedGroup.key || '（空前缀）' }} · {{ selectedGroup.dirRelKey || '根目录' }} · {{ selectedGroup.size }} 成员 · {{ groupBadge(selectedGroup) }}</p>
                    <p class="li-ws-sec-note">本件序号：#{{ selectedGroupMember ? (selectedGroupMember.seqText || selectedGroupMember.seq) : '-' }}</p>
                  </div>
                  <p v-else class="li-ws-empty">不属于任何序列群</p>
                </div>
                <!-- 阶段3 · 剧本G：线索集胶囊陈列（结构分段+既有库名命中/pixiv，含旧「线索」区职能）+
                     选给五字段+出处小字 → 保存 → 规则建议卡（泛化成规则/仅保留本次赋值）+
                     待裁决就地双路裁决 / 忽略 / 清除赋值 / 生成规则与差异提示；请求走本组件唯一通道 -->
                <ParsePanel
                  ref="filePanelRef"
                  :root="session ? session.rootPath : ''"
                  :file="selectedFile"
                  :assignments="assignmentList"
                  :ignored="selectedIgnored"
                  :rule-names="ruleNameMap"
                  :pending-items="selectedPending"
                  :diff="diff"
                  :recognizing="recognizing"
                  :request="request"
                  :notice-seed="panelNotice"
                  @notice="onPanelNotice"
                  @refresh="onPanelRefresh"
                  @open-rule="openRuleEdit"
                />
              </template>
              </div>
            </el-scrollbar>
          </aside>
        </section>
    </template>

    <!-- 创建任务名称确认对话框：任务名预填默认名（空=用默认，后端同规则兜底），
         确认「创建」才走既有创建链路（磁盘对账核验→创建→选择集清理）；
         「取消」= 关闭弹窗、无副作用 -->
    <el-dialog
      v-model="createDialogVisible"
      title="创建导入任务"
      width="420px"
      append-to-body
    >
      <el-input
        v-model="createTaskName"
        maxlength="200"
        clearable
        :placeholder="`默认：${defaultTaskName}`"
      />
      <p class="li-ws-create-dialog-note">
        一次创建为一个父任务，包含全部 {{ selectedWorks.size }} 件作品（{{ selectedFileCount }} 个文件）；识别出站点归属的文件按站点分组为额外父行（罕见）。创建前经磁盘对账核验；创建后任务为 Created 状态，由任务管理手动开始
      </p>
      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="createTasks">创建</el-button>
      </template>
    </el-dialog>

    <!-- 取消创建确认对话框（剧本1·帧3 逐字文案）：「继续创建」= 关窗回到进行中
         形态（无副作用）；「确定取消」= cancelCreate 请求 → cancelling（横幅转
         「正在清理在途任务…」帧3→帧4）/ completed（刚好已完成，未执行取消——
         风险10，照常收帧6 完成通知）/ idle（无在途创建，终态横幅已先行呈现） -->
    <el-dialog
      v-model="cancelDialogVisible"
      title="取消创建"
      width="420px"
      append-to-body
    >
      <p class="li-ws-cancel-dialog-text">将删除本次已创建的全部在途任务，选择集保留、可重新发起。确定取消？</p>
      <template #footer>
        <el-button @click="cancelDialogVisible = false">继续创建</el-button>
        <el-button type="danger" :loading="createCancelPending" @click="confirmCancelCreate">确定取消</el-button>
      </template>
    </el-dialog>

    <!-- 待裁决·决策态融合弹窗（只读总览）：决策计数三格 + 失效赋值提示 + 待裁决全量清单
         （长内容经 AutoHeightDialog 内置 el-scrollbar 弹窗内滚）；点文件名关弹窗并定位
         清单行、右栏走既有 selectFile 链路；裁决动作仍在右栏解析面板就地完成 -->
    <AutoHeightDialog v-model:state="fusionDialogVisible" width="min(720px, 94vw)">
      <template #header>
        <span class="li-ws-fusion-title">待裁决 · 决策态</span>
      </template>
      <div class="li-ws-fusion">
        <div class="li-ws-decisions">
          <div class="li-ws-decision">
            <span class="li-ws-decision-count">{{ decisions ? decisions.assignments ? decisions.assignments.length : 0 : 0 }}</span>
            <span class="li-ws-decision-label">已手动解析</span>
          </div>
          <div class="li-ws-decision">
            <span class="li-ws-decision-count">{{ decisions ? (decisions.ignored ? decisions.ignored.length : 0) : 0 }}</span>
            <span class="li-ws-decision-label">已忽略</span>
          </div>
          <div class="li-ws-decision">
            <span class="li-ws-decision-count">{{ selectedWorks.size }}</span>
            <span class="li-ws-decision-label">已选定为任务</span>
          </div>
        </div>
        <div v-if="snapshot && snapshot.staleAssignments && snapshot.staleAssignments.length" class="li-ws-stale">
          <p class="li-ws-stale-title">失效赋值 {{ snapshot.staleAssignments.length }}（引用的路径已不存在）：</p>
          <span v-for="s in snapshot.staleAssignments" :key="s" class="li-ws-stale-item">{{ s }}</span>
        </div>
        <h4 class="li-ws-fusion-sec">待裁决项（{{ pendingCount }}）</h4>
        <p v-if="pendingCount === 0" class="li-ws-empty">无声明冲突</p>
        <div v-for="(p, i) in (snapshot && snapshot.pending) || []" :key="i" class="li-ws-pending">
          <div class="li-ws-pending-head">
            <span
              class="li-ws-pending-file"
              :class="{ 'is-selected': p.relPath === selectedRelPath }"
              title="定位清单行并联动右栏解析面板"
              @click="jumpToPendingFile(p.relPath)"
            >{{ p.relPath }}</span>
            <span class="li-ws-pending-dim">{{ dimLabel(p.dimension) }}</span>
          </div>
          <div class="li-ws-pending-cands">
            <span v-for="(c, j) in p.candidates || []" :key="j" class="li-ws-pending-cand">
              {{ dimLabel(c.dimension) }}={{ c.value }}（{{ sourceLabel(c.source) }}<template v-if="c.ruleId"> · <a class="li-ws-rule-link" :href="ruleEditRoute(c.ruleId)" :title="`跳规则管理子页并打开「${ruleNameOf(c.ruleId)}」编辑`">{{ ruleNameOf(c.ruleId) }}</a></template>）
            </span>
          </div>
          <p class="li-ws-pending-reason">{{ p.reason }}</p>
        </div>
        <p class="li-ws-fusion-note">弹窗只读总览；裁决在右栏解析面板就地操作</p>
      </div>
    </AutoHeightDialog>
  </div>
  </BaseView>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, onActivated, onDeactivated, nextTick } from 'vue'
import { Events } from '@wailsio/runtime'
import ParsePanel from './ParsePanel.vue'
import SetRulePanel from './SetRulePanel.vue'
import { requestTimeoutMs } from './requestTimeout'

// 双栏主从解析工作区（识别全景 × 手动解析合一，视口锁定：双卡等高填满、各自卡内滚动）：
// 左卡 = 资源清单（过滤器 + 统计 + 目录树/平铺/疑似作品集三标签滚动体 + 卡头创建任务入口 +
// 卡底创建反馈条；无快照时卡体承载引导/识别进度/空态占位），
// 右卡 = 解析面板（文件/目录/集规则三模式互斥挂载，点清单行联动选中）。
// 待裁决项与会话决策态融合为头部入口弹窗（只读总览+清单行定位）；事件协议零改动
// （request/response/event 三通道原样）。

// 工作台事件协议通道（与插件 Go 侧 session.go 约定一致）
const TOPIC_REQUEST = 'plugin:local-import:workbench:request'
const TOPIC_RESPONSE = 'plugin:local-import:workbench:response'
const TOPIC_EVENT = 'plugin:local-import:workbench:event'

// 本插件 view 路由复合键前缀（publicId=插件 id）
const ROUTE_BASE = '#/com.lvfeng.localImport/'

interface SessionSummary {
  rootPath: string
  state: string
  createdAtMs: number
  updatedAtMs: number
}

interface SnapDecl {
  dimension: string
  value: string
  source: string
  roleName?: string
  ruleId?: string
  // 同值伴生来源：与本声明产出相同值的其他规则（终选合成同值去重的伴生记录，
  // 保序；缺省=无同值伴随）
  alsoRuleIds?: string[]
}

interface SnapFile {
  relPath: string
  fileName: string
  dirRelKey: string
  state: string
  final?: SnapDecl[]
  pendingDims?: string[]
  matchedRuleIds?: string[]
  groupKey?: string
  groupSeq?: number
  clues?: { kind: string; detail: string }[]
  // 剧本G：结构分段线索集（getPanorama 每文件投影；ParsePanel 线索胶囊区消费）
  structClues?: { ref: string; kind: string; label: string; value: string }[]
  assigned?: boolean
}

interface SnapGroup {
  dirRelKey: string
  key: string
  size: number
  ruleId: string
  // 结构形态群携带的命中形态名（如「括号序号」）；规则定群群为空
  formName?: string
  members: { relPath: string; seq: number; seqText: string }[]
}

// 剧本步1′：文件名元素（后端 splitFilenameElements 单一事实源；按序拼接 text
// 即还原去扩展名正文，无偏移量纲问题）。kind=author 作者括号块/workId 作品号
// 数字流/seq 序号/title 群公共元素/plain 非公共/sep 分隔 run（后两者无底色）；
// shelled=括号组双态——text 含括号原文、inner 剥壳内名
interface SnapEl {
  text: string
  kind: string
  shelled?: boolean
  inner?: string
}

// 剧本步1′：候选卡成员（群成员/目录直接子文件；elements 供元素级块底色渲染，
// 旧快照 v1 无该字段时退整段无底色）
interface SnapCandMember {
  relPath: string
  seqText?: string
  elements?: SnapEl[]
}

// 剧本v2 疑似作品集候选卡（三类合流）：source=dir 目录候选 / group 序列群（含页级）；
// crossMatch=库内同名集互证（批次二 set-name-match 同源）；existingSet*=A 档作品号唯一
// 命中的库内既有集；confirmedSetName=已归集分仓回带的确认集名
interface SnapCandidate {
  key: string
  dir: string
  source: string
  formName?: string
  size: number
  reason?: string
  crossMatch?: boolean
  existingSetId?: number
  existingSetName?: string
  confirmedSetName?: string
  members: SnapCandMember[]
  // 剧本步2′：键元素池（决策2 A——群来源=键前缀元素、目录来源=剥壳内名元素），
  // 确认区插入 chips 的数据源（括号组双态成对出 chip）
  keyElements?: SnapEl[]
}

interface SnapPending {
  relPath: string
  fileName: string
  dimension: string
  reason: string
  candidates: SnapDecl[]
}

interface PanoramaSnapshot {
  schemaVersion: number
  generatedAtMs: number
  rootPath: string
  dirs: {
    relKey: string
    name: string
    depth: number
    fileCount: number
    meanings?: { type: string; id?: string; name?: string }[]
    meaningRuleId?: string
    // 阶段4 剧本J：目录级结构分段线索集（库根名/各级目录名/剥壳变体，无文件名段；
    // getPanorama 投影时展开的呈现态数据）——目录面板赋值表单的出处引用数据源
    structClues?: { ref: string; kind: string; label: string; value: string }[]
  }[]
  files: SnapFile[]
  groups: SnapGroup[]
  // 剧本v2 疑似作品集候选三清单（主列表恒数组；两分仓 omitempty——动作应答先回、
  // 分仓数据随自动重识别的快照事件到达）
  candidates: SnapCandidate[]
  ignoredCandidates?: SnapCandidate[]
  confirmedCandidates?: SnapCandidate[]
  pending?: SnapPending[]
  ruleStats: { id: string; description: string; source: string; enabled: boolean; hitCount: number }[]
  staleAssignments?: string[]
  stats: {
    totalFiles: number
    identified: number
    clueful: number
    plain: number
    groupCount: number
    pendingFiles: number
    skippedDirs: number
    staleAssignments: number
  }
}

interface DecisionsView {
  // assignments 元素为后端 workbenchAssignmentView 全量投影（五字段供右栏表单预填；
  // 剧本G 各维度线索出处引用随赋值回透，供来源小字恢复——tagClueRefs 与 tags 逐位平行）
  assignments?: { relPath: string; author?: string; authorRole?: string; title?: string; tags?: string[]; collection?: string; generalized?: boolean; updatedAtMs?: number; authorClueRef?: string; titleClueRef?: string; collectionClueRef?: string; tagClueRefs?: string[] }[]
  ignored?: string[]
  selection?: string[]
}

// 选择域（服务端从识别快照派生的唯一结构源，键 ≡ 创建基面）：
// 作品 = 集成员单文件作品（kind=setMember）或目录聚合多轨作品（kind=dirTracks），
// 键形态为 f:{relPath} / d:{dirRelKey}——无文件级键，半作品结构性不可表达
interface SelectableWorkView {
  workId: string
  kind: string
  dirRelKey: string
  files: string[]
  setKey?: string
  selectable: boolean
  state: string
}

interface WorkDomainView {
  works: SelectableWorkView[]
  // 选择域集视图（大载荷分页方案·设计 A：domain 分节分页随答；作品序列在前、
  // 集序列在后共用一个下标空间，前端逐页拼接）
  sets?: WorkSetRowView[]
}

// 选择域集（序列群）的呈现视图（后端 WorkSetView：成员作品键按集内序）
interface WorkSetRowView {
  setKey: string
  dirRelKey: string
  key: string
  members: string[]
}

// getPanorama 分节分页回执（后端 workbenchPageReceipt，json 驼峰逐字对齐）：
// section=本页分节（overview/files/domain）、offset/count=分节序列起点与条数、
// total=序列总条数（overview=全景六段+决策态三清单+差异集三清单共用下标空间
// 合计）、nextCursor=续拉游标（末页空）、done=本页即末页、generation=快照世代
//（GeneratedAtMs）
interface PageReceipt {
  section: string
  offset: number
  count: number
  total: number
  nextCursor?: string
  done: boolean
  generation: number
}

// 重识别差异集（生成规则后的强制验证轮高亮载荷）
interface SnapDiff {
  changed: { relPath: string; before?: DiffConclusion; after?: DiffConclusion }[]
  added: { relPath: string; before?: DiffConclusion; after?: DiffConclusion }[]
  removed: { relPath: string; before?: DiffConclusion; after?: DiffConclusion }[]
}

interface DiffConclusion {
  state: string
  final?: SnapDecl[]
  pendingDims?: string[]
}

interface WorkbenchResponse {
  seq: number
  ok: boolean
  error?: string
  sessions?: SessionSummary[]
  session?: SessionSummary
  profileChanged?: boolean
  started?: boolean
  running?: boolean
  panorama?: PanoramaSnapshot
  decisions?: DecisionsView
  diff?: SnapDiff
  // setSelection/createTasks 的选择集轻量回执（后端 decisions 全量投影已从
  // 变更类动作应答剥离——大载荷断路器收口；选择族以本回执回读选择态）
  selection?: string[]
  domain?: WorkDomainView
  // getPanorama 分节分页回执（与 panorama/domain 分节数据同答）
  page?: PageReceipt
  // createTasks 立即应答「已开始创建」（大载荷分页方案·设计 C：进度与终态经
  // createProgress/createDone 事件推送，应答不再携带 created/createMsg）
  started?: boolean
  // cancelCreate 受理结论（甲3/决策3）：cancelling=取消已受理（终态经 createDone
  // 事件）/ completed=恰好已完成（原子提交点已过、未执行取消，风险10）；idle 走
  // ok=false（无在途创建可取消）
  cancelState?: string
}

interface WorkbenchEvent {
  kind: string
  root?: string
  phase?: string
  done?: number
  total?: number
  // 核对相当前窗口读盘字节/秒（甲1 速率字段：verify 相携带、发射相与首事件不带）
  bytesPerSec?: number
  success?: boolean
  message?: string
}

// Wails 将 []byte 载荷以 base64 或 JSON 文本送达，双路解码
function decodePayload<T>(raw: unknown): T | null {
  if (raw == null) return null
  if (typeof raw === 'object') return raw as T
  if (typeof raw === 'string') {
    try {
      return JSON.parse(raw) as T
    } catch {
      /* 尝试 base64 */
    }
    try {
      // base64 → 字节 → UTF-8 文本：atob 直出 Latin-1 串会使多字节中文乱码
      const bin = atob(raw)
      const bytes = new Uint8Array(bin.length)
      for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
      return JSON.parse(new TextDecoder().decode(bytes)) as T
    } catch {
      /* 放弃 */
    }
  }
  return null
}

const session = ref<SessionSummary | null>(null)
// 会话恢复中初态（2026-11-06 BA）：activateSession 对大会话做全载荷解码（session.go
// SessionOf「全会话解码、秒级」），此前页面无「恢复中」态——页头按 snapshot 为空
// 直显「开始全量识别」、主体直显「未找到进行中的解析会话」/「尚未生成识别全景」，
// 大库约 1s 的窗口会诱导用户点击全量识别。恢复完成（成功或失败）即置 false。
const sessionRestoring = ref(true)
const snapshot = ref<PanoramaSnapshot | null>(null)
const decisions = ref<DecisionsView | null>(null)
const diff = ref<SnapDiff | null>(null)
const loadError = ref('')
const profileChanged = ref(false)
const recognizing = ref(false)
const starting = ref(false)
const recognError = ref('')
const progress = ref({ phase: '', done: 0, total: 0 })

// —— 双栏布局态（会话内记忆，不持久化） ——
// 右栏收起态：默认展开；收起后左卡独占整行宽度
const rightCollapsed = ref(false)

// —— 缺陷①修复（动作通知跨刷新存活）——
// 解析面板动作通知的父组件托管态：动作应答后 emit('refresh') → loadPanorama
// 同步 snapshot=null → 右栏 v-if 同批卸载面板，通知随实例消亡（MutationObserver
// 全程旁录 0 命中已证）。改为 ParsePanel notice 每次变更上浮此处托管：面板不在
// 场（全景刷新载入期/右栏收起）由页头存活层横幅呈现，面板回场后经 noticeSeed
// 回填交还面板内呈现。切换面板目标（换文件/换目录/文件↔目录互斥切）即弃管旧
// 通知——新目标不该复活旧动作的反馈；刷新周期内的卸载-重挂不属切换（选中键
// 与目录键均不变、不触发本守卫，通知得以跨刷新续读）。
const panelNotice = ref('')

function onPanelNotice(text: string): void {
  panelNotice.value = text
}

// —— 缺陷②修复（建议卡跨刷新存活）——
// 解析面板规则建议的父组件托管态：目录保存（doSaveDir）在摘要应答后仍
// emit('refresh') → loadPanorama 置 snapshot=null → dirPanelContext 失值、
// 目录 ParsePanel 同批卸载，重挂后建议若只存实例内即随实例消亡（建议卡
// 永不渲染，实机 DOM 标记已证重挂）。改为 ParsePanel suggestion 每次变更
// 上浮此处托管，重挂后经 suggestionSeed 回填交还面板呈现。切换面板目标即
// 弃管旧建议（同 panelNotice 守卫）。类型与 ParsePanel 的
// PanelRuleSuggestion/PanelRule 同构（SFC 间类型不互导，先例 DirPanelContext）
interface PanelSuggestionRule {
  id: string
  description: string
  pattern?: string
  condition?: string
}
interface PanelSuggestionSeed {
  relPath: string
  summary: string
  mappings?: { dimension: string; from: string; clueRef?: string }[]
  manualDims?: string[]
  notes?: string[]
  rules?: PanelSuggestionRule[]
  hitCount: number
  hitTotal: number
  samples?: { fileName: string; dirRelKey?: string; result: string }[]
  dropsTailCode?: boolean
}
const panelSuggestion = ref<PanelSuggestionSeed | null>(null)

function onPanelSuggestion(s: PanelSuggestionSeed | null): void {
  panelSuggestion.value = s
}

// —— 待裁决·决策态融合弹窗（头部「重新识别」右侧入口；只读总览 + 待裁决文件定位清单行） ——
const fusionDialogVisible = ref(false)
const pendingCount = computed(() => ((snapshot.value && snapshot.value.pending) || []).length)

// —— 清单浏览模式（剧本F：平铺 ↔ 目录树；默认=目录树〔决策2〕，会话内记忆不持久化。
// 剧本v2 增第三标签 sets=疑似作品集候选卡墙） ——
const listMode = ref<'tree' | 'flat' | 'sets'>('tree')

// —— 剧本v2 疑似作品集候选卡墙（第三标签体） ——
// 候选/两分仓清单来自快照（后端 buildSnapshotCandidates 三类合流）；动作走本组件唯一
// workbench 请求通道，应答先回、分仓随自动重识别的快照事件刷新（既有事件通路联动）
const candidates = computed<SnapCandidate[]>(() => (snapshot.value && snapshot.value.candidates) || [])
// 已识别候选过滤（2026-10-27 用户二拍板改口径）：候选成员**全部**已带集维度
// 最终结论（snapshot.files 的 final 含 dimension=collection——规则识别与赋值
// 存档同源计入，后端 applyAssignmentToConclusion 合并）= 已被识别为作品集 →
// 默认隐藏可关；existingSetId（库内作品号比对）/crossMatch（同名互证）不再
// 作隐藏依据；部分成员未识别的候选保留（残员仍需人工处理）。已归集/已忽略
// 收纳分组不受此过滤影响。
const hideIdentifiedCandidates = ref(true)
const fileHasCollection = computed(() => {
  const m = new Map<string, boolean>()
  for (const f of (snapshot.value && snapshot.value.files) || []) {
    m.set(f.relPath, !!(f.final && f.final.some(d => d.dimension === 'collection')))
  }
  return m
})
function candidateIdentified(c: SnapCandidate): boolean {
  if (!c.members.length) return false
  return c.members.every(m => fileHasCollection.value.get(m.relPath) === true)
}
const identifiedCandidateCount = computed(() => candidates.value.filter(candidateIdentified).length)
const visibleCandidates = computed(() =>
  hideIdentifiedCandidates.value ? candidates.value.filter(c => !candidateIdentified(c)) : candidates.value)
// —— 卡墙增量分批渲染（性能修复）：真实库候选量 ~2.8 万卡，一次性 v-for 全量
// 渲染=百万级 DOM 节点（切标签 display 翻转整片 layout/paint ~10 秒；翻「隐藏
// 已识别」跳全量瞬间建卡曾致 WebView2 渲染进程 OOM 崩溃重启）。v-for 改遍历
// visibleCandidates 的前缀切片（renderedCandidates），卡墙尾部哨兵元素（模板
// ref cardsSentinel）由 IntersectionObserver 盯守触底自动追加一批；底层清单
// 变更（隐藏开关翻转/快照刷新）与离开卡墙标签时上限重置回首批，从 100 张
// 起步随滚动续批（接线见 bindCardsIO 一节） ——
const CARD_BATCH = 100
const cardRenderLimit = ref(CARD_BATCH)
const renderedCandidates = computed(() => visibleCandidates.value.slice(0, cardRenderLimit.value))
const confirming = ref('')
const confirmName = ref('')
const candBusy = ref(false)
const binConfirmed = ref(true)
const binIgnored = ref(true)
const candKey = (c: SnapCandidate): string => `${c.source}/${c.dir}/${c.key}`
// 动作寻址串（与后端 target() 同式：source+NUL+dir+NUL+key）
const candTarget = (c: SnapCandidate): string => `${c.source}\u0000${c.dir}\u0000${c.key}`
const candSourceLabel = (c: SnapCandidate): string =>
  c.source === 'dir' ? '目录候选' : (c.formName ? `序列·${c.formName}` : '序列群')
// 剧本步1′ 元素级块底色：元素数组按序拼接即去扩展名正文（后端单一事实源，
// 无偏移换算），扩展名尾巴以弱化样式缀尾；plain/sep 不加底色类。旧快照 v1
// （无 elements 字段）退整段无底色
const elSpans = (fileName: string, els?: SnapEl[]): { spans: { text: string; kind?: string }[]; ext: string } => {
  const dot = fileName.lastIndexOf('.')
  const ext = dot > 0 ? fileName.slice(dot) : ''
  const spans: { text: string; kind?: string }[] = []
  for (const el of els || []) {
    if (!el.text) continue
    spans.push({ text: el.text, kind: el.kind === 'author' || el.kind === 'workId' || el.kind === 'seq' || el.kind === 'title' ? el.kind : undefined })
  }
  if (!spans.length) {
    const base = dot > 0 ? fileName.slice(0, dot) : fileName
    if (base) spans.push({ text: base })
  }
  return { spans, ext }
}
// elSpans 单次计算（性能修复）：模板原先每成员两处绑定（.spans 与 .ext）各调
// 一次 elSpans（成员级 ×2 切分与数组分配放大）。改为按候选对象弱缓存整卡成员
// 预装配视图 { m, parts }——elSpans 每成员只算一次，同候选重复渲染（确认态
// 翻转/滚动续批重排）零重算；候选对象随快照整体更换，弱键条目自动回收不泄漏
// （快照成员数据不可变，缓存安全）。渲染输出与原模板逐字等价（#seqText 前缀、
// 形态块 class、扩展名尾巴全保持）
interface CandMemberView {
  m: SnapCandMember
  parts: { spans: { text: string; kind?: string }[]; ext: string }
}
const candMemberViews = new WeakMap<SnapCandidate, CandMemberView[]>()
const candMembers = (c: SnapCandidate): CandMemberView[] => {
  let views = candMemberViews.get(c)
  if (!views) {
    views = c.members.map(m => ({ m, parts: elSpans(fileNameOf(m.relPath), m.elements) }))
    candMemberViews.set(c, views)
  }
  return views
}
// 剧本步2′：确认区元素插入池（决策2 A=键元素+目录名元素，后端 keyElements）；
// 括号组元素双态成对出 chip（带壳 `[Isel]` / 剥壳 `Isel`），点击插入集名输入框
// 光标处
const chipVariants = (c: SnapCandidate): { label: string; insert: string; shelled: boolean }[] => {
  const out: { label: string; insert: string; shelled: boolean }[] = []
  for (const el of c.keyElements || []) {
    if (!el.text) continue
    if (el.shelled && el.inner) {
      out.push({ label: el.text, insert: el.text, shelled: true })
      out.push({ label: el.inner, insert: el.inner, shelled: false })
    } else {
      out.push({ label: el.text, insert: el.text, shelled: false })
    }
  }
  return out
}
// 剧本步2′ 光标追踪：确认区输入框的最近选区由事件流持续维护（点击 chip 时
// 输入框通常已失焦、且焦点/选区读取时序在后台窗口不可依赖——快照式维护不
// 依赖点击瞬间的读取）。selectionchange 覆盖程序化与拖选，click/keyup/select/
// input 事件委托覆盖用户键鼠置位（不依赖单一事件的浏览器覆盖面）。
let nameSelSnapshot = { start: 0, end: 0 }
const snapNameSel = (el: EventTarget | null) => {
  if (el instanceof HTMLInputElement && el.classList.contains('li-ws-cand-name')) {
    nameSelSnapshot = { start: el.selectionStart ?? 0, end: el.selectionEnd ?? 0 }
  }
}
// 后台/失焦窗口中 document.activeElement 不可靠（恒为 body），selectionchange
// 分支直接定位确认区输入框读选区（同时刻全页仅一个确认区输入框）
const onNameSelChange = () => {
  const el = document.querySelector<HTMLInputElement>('input.li-ws-cand-name')
  if (el) snapNameSel(el)
}
const onNameSelDelegate = (ev: Event) => snapNameSel(ev.target)
const insertChip = (chip: { insert: string }) => {
  const name = confirmName.value
  const start = Math.min(nameSelSnapshot.start, name.length)
  const end = Math.min(nameSelSnapshot.end, name.length)
  confirmName.value = name.slice(0, start) + chip.insert + name.slice(Math.max(start, end))
  void nextTick(() => {
    const input = [...document.querySelectorAll<HTMLInputElement>('input.li-ws-cand-name')].find(i => i.offsetParent !== null)
    if (input) {
      input.focus()
      const caret = start + chip.insert.length
      input.setSelectionRange(caret, caret)
    }
  })
}
// 两段式归集：预填=A 档既有集名（命中时）或候选键（剥壳段/群键），可改
const startConfirm = (c: SnapCandidate) => {
  confirming.value = candTarget(c)
  confirmName.value = c.existingSetName || c.key
  // 新确认区开启：预填后光标语义=键尾，快照重置防上一轮残留选区串位
  nameSelSnapshot = { start: confirmName.value.length, end: confirmName.value.length }
}
// 用户反馈优化①：集名输入框一键清空（顺带重置选区快照到键首）
const clearConfirmName = () => {
  confirmName.value = ''
  nameSelSnapshot = { start: 0, end: 0 }
  void nextTick(() => { document.querySelector<HTMLInputElement>('input.li-ws-cand-name')?.focus() })
}
// 用户反馈优化①：输入框宽度自适应内容（镜像 span 量宽——v-for 内不可用模板 ref，
// 与插入选区同因，DOM 直查；首帧 immediate 触发一次量宽）
const nameInputWidth = ref('160px')
const syncNameInputWidth = () => {
  void nextTick(() => {
    const mirror = document.querySelector<HTMLElement>('.li-ws-cand-name-mirror')
    if (!mirror) return
    const w = mirror.offsetWidth
    if (w > 0) nameInputWidth.value = Math.min(Math.max(w + 34, 96), 360) + 'px'
  })
}
watch(confirmName, syncNameInputWidth, { immediate: true })
const confirmCandidate = async (c: SnapCandidate) => {
  if (!session.value || !confirmName.value.trim()) return
  candBusy.value = true
  try {
    await request('confirmWorkSetCandidate', {
      root: session.value.rootPath,
      target: candTarget(c),
      setName: confirmName.value.trim(),
      existingSetId: c.existingSetId || 0
    })
    confirming.value = ''
  } finally {
    candBusy.value = false
  }
}
const ignoreCandidate = async (c: SnapCandidate) => {
  if (!session.value) return
  candBusy.value = true
  try {
    await request('ignoreCandidate', { root: session.value.rootPath, target: candTarget(c) })
  } finally {
    candBusy.value = false
  }
}
const restoreCandidate = async (c: SnapCandidate) => {
  if (!session.value) return
  candBusy.value = true
  try {
    await request('restoreCandidate', { root: session.value.rootPath, target: candTarget(c) })
  } finally {
    candBusy.value = false
  }
}
// 剧本2 撤销归集：在途态按钮「撤销中…」（undoing 寻址 + candBusy 全墙禁用）；
// 失败回错误提示、卡片不动；成功后卡随自动重识别的快照事件移回上方候选清单
// （成员集声明先保持旧值、刷新完成后消失——与忽略/恢复同通路）
const undoing = ref('')
const candError = ref('')
const undoConfirm = async (c: SnapCandidate) => {
  if (!session.value) return
  undoing.value = candTarget(c)
  candBusy.value = true
  candError.value = ''
  try {
    const resp = await request('undoWorkSetCandidate', { root: session.value.rootPath, target: candTarget(c) })
    if (!resp.ok) candError.value = resp.error || '撤销归集失败'
  } finally {
    undoing.value = ''
    candBusy.value = false
  }
}

// —— 剧本C 集规则面板（已归集卡的泛化集规则入口；与文件/目录面板互斥） ——
// target 寻址 confirmed 分仓卡（后端按集整体推导）；面板头集名用卡面确认集名
// 兜底（推导应答到达后以应答为准）。面板不随快照自动重推导（5c 面板不动），
// 重进由用户再点入口达成
const setPanelTarget = ref('')
const setPanelName = ref('')

const openSetRule = (c: SnapCandidate) => {
  const target = candTarget(c)
  const setName = c.confirmedSetName || c.key || ''
  // 面板有未保存改动时暂存切换目标待确认（含集面板间切换）
  if (panelDirty() && (target !== setPanelTarget.value || selectedDirKey.value !== null || selectedRelPath.value !== '')) {
    pendingSwitch.value = { kind: 'set', target: target, setName: setName }
    return
  }
  selectedDirKey.value = null
  selectedRelPath.value = ''
  setPanelTarget.value = target
  setPanelName.value = setName
}

// 集面板「返回」：让出集面板会卸载调整面指派、未保存改动随之丢失，故同样先确认
const exitSetPanel = (): void => {
  if (panelDirty()) {
    pendingSwitch.value = { kind: 'exitSet' }
    return
  }
  setPanelTarget.value = ''
}

// 集面板成员点行：切到该文件的表单（集面板让出，与点清单文件行同语义——走
// 同一未保存确认通路）
const onSetMemberSelected = (relPath: string): void => {
  selectFile(relPath)
}

// —— 右栏选中态（主从联动） ——
// 选中键 = 文件相对路径：自动刷新（重识别）后快照重排，选中态凭键保持；
// 该路径从快照消失时右栏转「文件已不在当前全景」提示态
const selectedRelPath = ref('')
const selectedFile = computed<SnapFile | null>(() => {
  if (!selectedRelPath.value) return null
  return fileByRel.value.get(selectedRelPath.value) || null
})
const selectedGroup = computed<SnapGroup | null>(() => {
  const file = selectedFile.value
  if (!file) return null
  for (const g of (snapshot.value ? snapshot.value.groups : null) || []) {
    if (g.members.some((m) => m.relPath === file.relPath)) return g
  }
  return null
})
const selectedGroupMember = computed(() => {
  const file = selectedFile.value
  const group = selectedGroup.value
  if (!file || !group) return null
  return group.members.find((m) => m.relPath === file.relPath) || null
})

// 选中文件的提取段命中规则数（matchedRuleIds 为档案序；定群规则不入提取段命中）
const hitRuleCount = computed<number>(() => {
  const file = selectedFile.value
  return file ? (file.matchedRuleIds || []).length : 0
})

// 终选声明同值折叠展开态（键=声明行下标，仅所点行切换；换选中文件收起全部）
const sameExpandedIndexes = ref(new Set<number>())
const isSameExpanded = (i: number): boolean => sameExpandedIndexes.value.has(i)
function toggleSameExpand(i: number): void {
  const next = new Set(sameExpandedIndexes.value)
  if (next.has(i)) next.delete(i)
  else next.add(i)
  sameExpandedIndexes.value = next
}
watch(selectedRelPath, () => {
  sameExpandedIndexes.value = new Set()
})

// 同值折叠标签文案：[同值 +N]，展开态 [同值 +N ▴]
function sameToggleLabel(n: number, expanded: boolean): string {
  return `[同值 +${n}${expanded ? ' ▴' : ''}]`
}

// 群定群来源标注（右栏「所属序列群」区消费）：结构形态群=「结构·形态名」
// （形态表命中形态透出），规则定群群=定群规则名称（无名称/已删回退规则编码），
// 兜底「结构」
const groupBadge = (g: SnapGroup): string => {
  if (g.formName) return `结构·${g.formName}`
  return g.ruleId ? ruleNameOf(g.ruleId) : '结构'
}

// —— 右栏解析面板数据投影（ParsePanel 消费；请求仍走本组件唯一 workbench 通道） ——
// 已忽略判定与选中文件的待裁决项（左栏待裁决折叠区保留只读总览，双路裁决在右栏就地）
const assignmentList = computed(() => (decisions.value && decisions.value.assignments) || [])
const selectedIgnored = computed(() => {
  const list = (decisions.value && decisions.value.ignored) || []
  return list.indexOf(selectedRelPath.value) >= 0
})
const selectedPending = computed(() =>
  ((snapshot.value && snapshot.value.pending) || []).filter((p) => p.relPath === selectedRelPath.value)
)

// 面板 refresh 事件（忽略动作应答后仅重取全景+决策态，不触发识别）
function onPanelRefresh(): void {
  if (session.value) void loadPanorama(session.value.rootPath)
}

// —— 资源选择与任务创建（作品级原子选择；域结构来自服务端 getPanorama 的 domain 投影） ——
const domain = ref<WorkDomainView | null>(null)
const selectedWorks = ref<Set<string>>(new Set())
const creating = ref(false)
const createError = ref('')
// —— 创建异步呈现态（大载荷分页方案·设计 C → 剧本1 六帧升级：双计数分道+取消）——
// 双计数模型（乙3 重叠期语义）：verify 道=核对行（done/total=作品数、bytesPerSec=
// 读盘速率；done 到达 total 即核对完成、核对行消失）、emit 道=发射行（done=已发射
// 子行数，自首批发射起出现至终值）；两道按 createProgress 事件 phase 维度各自
// 更新、可并存（null=该道未出现）。创建发起即预点亮 verify 道（计数待首事件 0/N）
const createVerify = ref<{ done: number; total: number; bytesPerSec: number } | null>(null)
const createEmit = ref<{ done: number } | null>(null)
// 取消受理态（甲3/剧本帧3→帧4）：cancelling=横幅转「正在清理在途任务…」待终态；
// raceNote=取消×完成竞态附注（应答 completed=风险10「刚好已完成，未执行取消」）
const createCancelling = ref(false)
const createCancelRaceNote = ref('')
const createCancelPending = ref(false)
const cancelDialogVisible = ref(false)
// 终态横幅（帧4/帧5/帧6）：文案优先消费后端 createDone Message（前端自建摘要与
// partial 结构化两行随帧5/帧6 收口退役）；kind 只决定横幅色（帧4 取消=✔、帧5
// 失败=✖、帧6 完成=✔）
const createDoneKind = ref<'cancelled' | 'failed' | 'success'>('success')
const createDoneMessage = ref('')
// 终态粘滞位：createDone 到达后置位，下一次创建发起时复位——终态后的迟到
// createProgress 事件（发射侧收流前的残余推送）不再把横幅拉回进行态
const createTerminalReached = ref(false)
// 本次创建的任务名（成功通知兜底名；事件文案携带名为准）
const createTaskLabel = ref('')
// verify 道 ETA 估算（速率=事件间 Δdone/Δt 的指数平滑；首个有效样本后可估）
const verifyEmaWorksPerSec = ref(0)
let verifySample: { done: number; at: number } | null = null
// 千分位计数（帧1/帧2「1,240 / 4,528」「41,200 个」形态）
function fmtCount(n: number): string {
  return n.toLocaleString('en-US')
}
// 读盘速率文案（帧1/帧2「读取 96 MB/s」形态）：≥10 MB/s 取整、以下保留 1 位小数
function formatRateMB(bytesPerSec: number): string {
  const mb = bytesPerSec / 1048576
  return `${mb >= 10 ? Math.round(mb) : mb.toFixed(1)} MB/s`
}
const createVerifyFinished = computed(() => {
  const v = createVerify.value
  return !!v && v.total > 0 && v.done >= v.total
})
// 核对行计数段（total 未到时不显示数字——不呈现误导性 0 / 0，真分母由首事件到达）
const createVerifyCount = computed(() => {
  const v = createVerify.value
  if (!v || v.total <= 0) return ''
  return ` ${fmtCount(v.done)} / ${fmtCount(v.total)}`
})
const createVerifyPercent = computed(() => {
  const v = createVerify.value
  if (!v || v.total <= 0) return '0%'
  return `${Math.min(100, Math.round((v.done / v.total) * 100))}%`
})
// 核对行速率/预计剩余行（帧1/帧2「读取 96 MB/s · 预计剩余 9 分钟」）：速率=事件
// bytesPerSec、ETA=平滑速率外推（不可估段隐藏；剩余不足 1 分钟按「预计剩余
// 不足 1 分钟」呈现）；核对完成后整行隐藏
const createVerifyMeta = computed(() => {
  const v = createVerify.value
  if (!v || createVerifyFinished.value) return ''
  const parts: string[] = []
  if (v.bytesPerSec > 0) parts.push(`读取 ${formatRateMB(v.bytesPerSec)}`)
  if (verifyEmaWorksPerSec.value > 0 && v.total > v.done) {
    const remainSec = (v.total - v.done) / verifyEmaWorksPerSec.value
    parts.push(remainSec < 60 ? '预计剩余不足 1 分钟' : `预计剩余 ${Math.max(1, Math.round(remainSec / 60))} 分钟`)
  }
  return parts.join(' · ')
})
// 剧本K 名称确认对话框：开窗态 + 任务名输入（每次打开预填默认名，清空=用默认）
const createDialogVisible = ref(false)
const createTaskName = ref('')
const rulePick = ref('')
const authorPick = ref('')

let seq = 0
let offResponse: (() => void) | null = null
let offEvent: (() => void) | null = null
const pending = new Map<number, (resp: WorkbenchResponse) => void>()

function onResponse(event: { data?: unknown }) {
  const resp = decodePayload<WorkbenchResponse>(event && event.data)
  if (!resp || !pending.has(resp.seq)) return
  pending.get(resp.seq)?.(resp)
  pending.delete(resp.seq)
}

// 带序号配对的请求（超时口径经 requestTimeoutMs：预览类动作 summarizeRule/
// summarizeWorkSetRule 全库求值 180 秒——大库线性增长分钟级；识别全景等其余动作 30 秒）
function request(action: string, payload: Record<string, unknown> = {}): Promise<WorkbenchResponse> {
  seq += 1
  const reqSeq = seq
  return new Promise((resolve) => {
    pending.set(reqSeq, resolve)
    Events.Emit(TOPIC_REQUEST, JSON.stringify({ seq: reqSeq, action: action, ...payload }))
    setTimeout(() => {
      if (pending.has(reqSeq)) {
        pending.delete(reqSeq)
        resolve({ seq: reqSeq, ok: false, error: '插件响应超时（插件进程未运行或通道异常）' })
      }
    }, requestTimeoutMs(action))
  })
}

// 从 hash 查询参数读取进入本页时携带的库根
function rootFromHash(): string {
  const hash = location.hash || ''
  const queryIndex = hash.indexOf('?')
  if (queryIndex < 0) return ''
  return new URLSearchParams(hash.slice(queryIndex + 1)).get('root') || ''
}

// 头部子页链接（规则管理/解析观测：parent 不变，携带库根免二次寻址）
function subRoute(key: string): string {
  const root = session.value ? session.value.rootPath : rootFromHash()
  if (!root) return `${ROUTE_BASE}${key}`
  return `${ROUTE_BASE}${key}?root=${encodeURIComponent(root)}`
}

// 头部子页跳转动作（2026-10-27 按钮统一：原文字链接改 el-button，hash 导航保留）
function goSub(key: string): void {
  location.hash = subRoute(key)
}

// 规则名称映射（快照 ruleStats 的 id→description）：工作台呈现面统一按规则名称
// 渲染；description 空（无名称规则）或映射未命中（规则已删）回退规则编码。
// ParsePanel 无快照数据，经 prop 注入该映射。
const ruleNameMap = computed<Record<string, string>>(() => {
  const map: Record<string, string> = {}
  for (const r of (snapshot.value ? snapshot.value.ruleStats : null) || []) {
    map[r.id] = r.description
  }
  return map
})
const ruleNameOf = (ruleId: string): string => ruleNameMap.value[ruleId] || ruleId

// 声明来源规则名直达编辑：跳规则管理子页并携规则 ID——子页载入规则档案后按 ID
// 打开该规则编辑态。规则名出现在待裁决候选/终选声明的来源小字处（含泛化规则），
// 点击即达修改口，消除「泛化规则无修改口」的认知缺口
function ruleEditRoute(ruleId: string): string {
  const base = subRoute('workbench-rules')
  return `${base}${base.indexOf('?') >= 0 ? '&' : '?'}rule=${encodeURIComponent(ruleId)}`
}

// 面板声明来源规则名点击（待裁决候选内的规则 ID）：路由上下文在本组件，统一由此跳转
function openRuleEdit(ruleId: string): void {
  location.hash = ruleEditRoute(ruleId)
}

// 恢复会话：按 hash 库根激活（含档案对账）；库根缺席时若恰一个会话则直接恢复；
// 会话可能刚经入口确认建立（导航先于建会话落库到达），短暂重试
async function restoreSession(attempt = 0): Promise<string> {
  const root = rootFromHash()
  if (root) {
    const resp = await request('activateSession', { root: root })
    if (resp.ok && resp.session) {
      session.value = resp.session
      profileChanged.value = resp.profileChanged === true
      return resp.session.rootPath
    }
    if (resp.session === undefined && attempt < 3) {
      await new Promise((r) => setTimeout(r, 300))
      return restoreSession(attempt + 1)
    }
    loadError.value = resp.error || '恢复解析会话失败'
    return ''
  }
  const list = await request('listSessions')
  if (!list.ok) {
    loadError.value = list.error || '读取会话失败'
    return ''
  }
  const sessions = list.sessions || []
  if (sessions.length === 1) {
    const resp = await request('activateSession', { root: sessions[0].rootPath })
    if (resp.ok && resp.session) {
      session.value = resp.session
      profileChanged.value = resp.profileChanged === true
      return resp.session.rootPath
    }
  }
  return ''
}

// —— 全景分节分页装载（大载荷分页方案·设计 A/剧本1）—— //
// 拉取链：overview 逐页（重载荷清单共用下标空间——目录树/序列群/候选三仓/
// 待裁决明细六段 + 决策态三清单（赋值/忽略/选择集）+ 差异集三清单（变更/
// 新增/消失）逐页拼接，统计与规则档案态等小字段及决策态/差异集容器每页
// 随答）→ files 逐页（cursor 串行续拉）→ domain 逐页 → 全部就绪一次性进入
// 现有呈现（决策3=整体装载，就绪前清单区只显示专用载入态）；每页一请求，
// 30s 超时按页适用。进度计数=各分节 Page 回执 total/累计 count 汇总（overview
// 清单条目计入——全景六段+决策态+差异集同口径）。页失败（含超时）→ 暂停出
// 「载入中断」+「第 X/Y 页失败」断点重试（已收页不重拉）；「世代变更/游标
// 无效」→ 整轮重拉（回到载入态从头，连续自动重拉至多 2 次后转中断态人工
// 重启）。

// 载入态呈现面（响应式小量；累积的大数组归 PanoLoadRun 普通对象承载，避免
// 数十万条的深层响应式开销）
const panoLoading = ref(false)
const panoLoadFailed = ref(false)
const panoResuming = ref(false)
const panoReceivedCount = ref(0)
const panoTotalCount = ref(0)
const panoFailedPage = ref(0)
const panoEstPages = ref(0)

// 一轮装载的累积数据与簿记（非响应式；跨断点暂停保留，供「从断点继续」续拉）
interface PanoLoadRun {
  // overview 各页拼接的重载荷清单（首页另携小字段；files 明细恒不在场）
  overview: PanoramaSnapshot | null
  // decisions/diff：overview 各页拼接的决策态三清单/差异集三清单（与全景
  // 六段同一预算装配分页，容器每页随答、页片逐页追加——大载荷赋值档案与
  // 差异清单全量拼接可达）
  decisions: DecisionsView | null
  diff: SnapDiff | null
  files: SnapFile[]
  filesTotal: number
  filesFirstCount: number
  domainWorks: SelectableWorkView[]
  domainSets: WorkSetRowView[]
  domainTotal: number
  domainFirstCount: number
  overviewTotal: number
  overviewFirstCount: number
  // 全局页序计数（三分节统一逐页）：失败页号 = pagesDone + 1
  pagesDone: number
}
let panoRun: PanoLoadRun | null = null
// 断点续拉位（失败页的 section+cursor）；null=须整轮重拉（世代变更耗尽自动重拉额度）
const panoResumeFrom = ref<{ section: string; cursor: string } | null>(null)

// 世代失效/游标无效的整轮重拉错误锚（后端错误应答文案前缀，逐字对齐 session.go）
const PANO_RESTART_ERROR = '整轮重拉'

function freshPanoRun(): PanoLoadRun {
  return {
    overview: null,
    decisions: null,
    diff: null,
    files: [],
    filesTotal: 0,
    filesFirstCount: 0,
    domainWorks: [],
    domainSets: [],
    domainTotal: 0,
    domainFirstCount: 0,
    overviewTotal: 0,
    overviewFirstCount: 0,
    pagesDone: 0
  }
}

function resetPanoRun(run: PanoLoadRun): void {
  const fresh = freshPanoRun()
  Object.assign(run, fresh)
}

// overview 已收清单条目数（全景六段合计；进度计数用）
function overviewListCount(o: PanoramaSnapshot | null): number {
  if (!o) return 0
  return (
    (o.dirs || []).length +
    (o.groups || []).length +
    (o.candidates || []).length +
    (o.ignoredCandidates || []).length +
    (o.confirmedCandidates || []).length +
    (o.pending || []).length
  )
}

// 决策态已收清单条目数（赋值/忽略/选择集三清单合计；进度计数用——后端
// overview 的 total 含这三段，接收计数须同口径才能收敛到 100%）
function decisionsListCount(d: DecisionsView | null): number {
  if (!d) return 0
  return ((d.assignments && d.assignments.length) || 0) + ((d.ignored && d.ignored.length) || 0) + ((d.selection && d.selection.length) || 0)
}

// 差异集已收清单条目数（变更/新增/消失三清单合计；进度计数用，口径同上）
function diffListCount(d: SnapDiff | null): number {
  if (!d) return 0
  return ((d.changed && d.changed.length) || 0) + ((d.added && d.added.length) || 0) + ((d.removed && d.removed.length) || 0)
}

// 载入进度同步：已接收=各分节累计条目数（overview 清单条目计入——全景六段+
// 决策态三清单+差异集三清单）；总量=已知分节 total 之和；估算总页数=各分节
// ceil(total/首页条数) 之和（overview 至少 1 页；未知分节不计），且不低于
// 失败页号（首页即败时保证「第 X/Y 页」分母可读）
function syncPanoProgress(run: PanoLoadRun): void {
  panoReceivedCount.value =
    overviewListCount(run.overview) +
    decisionsListCount(run.decisions) +
    diffListCount(run.diff) +
    run.files.length +
    run.domainWorks.length +
    run.domainSets.length
  panoTotalCount.value = run.overviewTotal + run.filesTotal + run.domainTotal
  const overviewEst =
    run.overviewTotal > 0 && run.overviewFirstCount > 0
      ? Math.max(1, Math.ceil(run.overviewTotal / run.overviewFirstCount))
      : 1
  const filesEst = run.filesTotal > 0 && run.filesFirstCount > 0 ? Math.ceil(run.filesTotal / run.filesFirstCount) : 0
  const domainEst = run.domainTotal > 0 && run.domainFirstCount > 0 ? Math.ceil(run.domainTotal / run.domainFirstCount) : 0
  panoEstPages.value = Math.max(overviewEst + filesEst + domainEst, run.pagesDone + 1)
}

const panoProgressPercent = computed(() => {
  if (panoTotalCount.value <= 0) return '0%'
  return `${Math.min(100, Math.round((panoReceivedCount.value / panoTotalCount.value) * 100))}%`
})

// 单页拉取结果：done=全部分节就绪；nosnap=会话未识别（保留空态呈现识别入口）；
// restart=世代失效/游标无效（整轮重拉）；failed=页失败（resume=失败页续拉位）
type PanoPagesOutcome =
  | { outcome: 'done' }
  | { outcome: 'nosnap' }
  | { outcome: 'restart' }
  | { outcome: 'failed'; resume: { section: string; cursor: string } }

// 分节序列游走：overview → files → domain；页成功则累积数据并按回执续拉/换节
async function runPanoPages(
  root: string,
  run: PanoLoadRun,
  from: { section: string; cursor: string }
): Promise<PanoPagesOutcome> {
  let section = from.section
  let cursor = from.cursor
  while (section) {
    const resp = await request('getPanorama', {
      root: root,
      section: section,
      cursor: cursor || undefined
    })
    if (!resp.ok || !resp.page) {
      const errorText = resp.error || ''
      // 未识别会话：非载入失败，退出呈现空态（识别入口）
      if (section === 'overview' && errorText.indexOf('尚未完成') >= 0) {
        return { outcome: 'nosnap' }
      }
      // 世代变更/游标无效：旧游标作废，整轮重拉
      if (errorText.indexOf(PANO_RESTART_ERROR) >= 0) {
        return { outcome: 'restart' }
      }
      return { outcome: 'failed', resume: { section: section, cursor: cursor } }
    }
    const page = resp.page
    if (section === 'overview') {
      // overview 逐页拼接：首页整体入账（含小字段），后续页只追加六段清单；
      // 决策态/差异集容器每页随答、三清单页片逐页追加（与全景六段同一预算
      // 装配分页——大载荷赋值档案/差异清单全量拼接可达）
      const pan = resp.panorama || null
      if (pan) {
        if (!run.overview) {
          run.overview = pan
        } else {
          const o = run.overview
          o.dirs = (o.dirs || []).concat(pan.dirs || [])
          o.groups = (o.groups || []).concat(pan.groups || [])
          o.candidates = (o.candidates || []).concat(pan.candidates || [])
          o.ignoredCandidates = (o.ignoredCandidates || []).concat(pan.ignoredCandidates || [])
          o.confirmedCandidates = (o.confirmedCandidates || []).concat(pan.confirmedCandidates || [])
          o.pending = (o.pending || []).concat(pan.pending || [])
        }
        run.overviewTotal = page.total
        if (run.overviewFirstCount === 0) run.overviewFirstCount = page.count
      }
      const dec = resp.decisions
      if (dec) {
        if (!run.decisions) {
          run.decisions = dec
        } else {
          const d = run.decisions
          d.assignments = (d.assignments || []).concat(dec.assignments || [])
          d.ignored = (d.ignored || []).concat(dec.ignored || [])
          d.selection = (d.selection || []).concat(dec.selection || [])
        }
      }
      const df = resp.diff
      if (df) {
        if (!run.diff) {
          run.diff = df
        } else {
          const d = run.diff
          d.changed = (d.changed || []).concat(df.changed || [])
          d.added = (d.added || []).concat(df.added || [])
          d.removed = (d.removed || []).concat(df.removed || [])
        }
      }
    } else if (section === 'files') {
      const pageFiles = (resp.panorama && resp.panorama.files) || []
      for (const f of pageFiles) run.files.push(f)
      run.filesTotal = page.total
      if (run.filesFirstCount === 0) run.filesFirstCount = page.count
    } else {
      const works = (resp.domain && resp.domain.works) || []
      const sets = (resp.domain && resp.domain.sets) || []
      for (const w of works) run.domainWorks.push(w)
      for (const s of sets) run.domainSets.push(s)
      run.domainTotal = page.total
      if (run.domainFirstCount === 0) run.domainFirstCount = page.count
    }
    run.pagesDone += 1
    syncPanoProgress(run)
    if (page.done) {
      section = section === 'overview' ? 'files' : section === 'files' ? 'domain' : ''
      cursor = ''
    } else {
      cursor = page.nextCursor || ''
    }
  }
  return { outcome: 'done' }
}

// 一轮装载的编排：驱动分页游走并收束三种终局（就绪呈现 / 整轮重拉循环 / 中断
// 暂停）。from=null 时重置累积数据从头拉（含世代变更的整轮重拉）。
async function drivePanoLoad(
  root: string,
  run: PanoLoadRun,
  from: { section: string; cursor: string } | null,
  restartsLeft: number
): Promise<void> {
  if (!from) {
    resetPanoRun(run)
    syncPanoProgress(run)
  }
  panoLoading.value = true
  panoLoadFailed.value = false
  let next = from
  for (;;) {
    const result = await runPanoPages(root, run, next || { section: 'overview', cursor: '' })
    // 并发守卫：期间新一轮 loadPanorama 已接管时，本轮静默废弃
    if (panoRun !== run) return
    if (result.outcome === 'done') {
      finishPanoLoad(run)
      await nextTick()
      scrollSelectedIntoView()
      return
    }
    if (result.outcome === 'nosnap') {
      // 未识别会话：清载入态走空态引导（与旧两段拉取的「保留空态」语义一致）
      panoLoading.value = false
      panoLoadFailed.value = false
      panoRun = null
      panoResumeFrom.value = null
      return
    }
    if (result.outcome === 'restart') {
      if (restartsLeft > 0) {
        // 世代变更：整轮重拉（回到载入态从头）
        restartsLeft -= 1
        next = null
        resetPanoRun(run)
        syncPanoProgress(run)
        continue
      }
      panoResumeFrom.value = null
    } else {
      panoResumeFrom.value = result.resume
    }
    panoFailedPage.value = run.pagesDone + 1
    syncPanoProgress(run)
    panoLoading.value = false
    panoLoadFailed.value = true
    return
  }
}

// 全部就绪：一次性进入现有呈现（快照合体 overview+files、选择域合体两分页、
// 选择集随决策态回读；选中键凭相对路径跨快照保持）
function finishPanoLoad(run: PanoLoadRun): void {
  if (run.overview) {
    snapshot.value = { ...run.overview, files: run.files }
  }
  decisions.value = run.decisions
  diff.value = run.diff
  domain.value = { works: run.domainWorks, sets: run.domainSets }
  selectedWorks.value = new Set((run.decisions && run.decisions.selection) || [])
  panoLoading.value = false
  panoLoadFailed.value = false
  panoRun = null
  panoResumeFrom.value = null
}

// 装载全景（分节分页整轮：overview → files 逐页 → domain 逐页 → 一次性呈现）；
// 重装载后选中键不重置——selectedRelPath 凭相对路径跨快照保持
async function loadPanorama(root: string): Promise<void> {
  const run = freshPanoRun()
  panoRun = run
  // 载入态先行：清旧呈现（决策3——就绪前不呈现部分数据，清单区转专用载入态）
  snapshot.value = null
  decisions.value = null
  diff.value = null
  domain.value = null
  await drivePanoLoad(root, run, null, 2)
}

// 断点继续：从失败页续拉（已收页不重拉）；世代变更耗尽自动重拉额度后的中断态
// 无有效游标（panoResumeFrom=null），按钮走整轮重拉
async function resumePanoLoad(): Promise<void> {
  const root = session.value ? session.value.rootPath : ''
  const run = panoRun
  if (!root || !run || panoResuming.value) return
  panoResuming.value = true
  try {
    await drivePanoLoad(root, run, panoResumeFrom.value, 2)
  } finally {
    panoResuming.value = false
  }
}

async function startRecognition(): Promise<void> {
  if (!session.value) return
  starting.value = true
  recognError.value = ''
  const resp = await request('startRecognition', { root: session.value.rootPath })
  starting.value = false
  if (!resp.ok) {
    recognError.value = resp.error || '启动全量识别失败'
    return
  }
  recognizing.value = true
  // 在途接续态（护栏三/AW）：后端在途去重应答携带 running=true——在途轮已持有
  // 进度，此处仅置「识别中」、不动 progress（下一拍进度事件自然接续刷新；若上一轮
  // 恰已收尾则进度保持空态「准备中」，待首拍）。started=true 维持原有「scan 相
  // 0/0 起步」行为，不因本次意图重来而把在途进度抹回扫描起点。
  if (resp.running) return
  progress.value = { phase: 'scan', done: 0, total: 0 }
}

// 进度与终态事件（识别事件外的其他工作台事件忽略）：
// recognitionDone → 重取全景与决策态（赋值变更后的自动增量重识别也经此通路刷新）；
// createProgress/createDone → 创建异步进度两相与终态（大载荷分页方案·设计 C，
// createTasks 立即应答 started 后全程经事件感知）
function onEvent(event: { data?: unknown }) {
  const evt = decodePayload<WorkbenchEvent>(event && event.data)
  if (!evt) return
  const root = session.value ? session.value.rootPath : ''
  if (evt.root && root && evt.root !== root) return
  if (evt.kind === 'recognitionProgress') {
    recognizing.value = true
    progress.value = {
      phase: evt.phase || '',
      done: evt.done || 0,
      total: evt.total || 0,
    }
  } else if (evt.kind === 'recognitionDone') {
    recognizing.value = false
    if (evt.success) {
      progress.value = { phase: '', done: 0, total: 0 }
      if (session.value) void loadPanorama(session.value.rootPath)
    } else {
      recognError.value = evt.message || '全量识别失败'
    }
  } else if (evt.kind === 'createProgress') {
    // 终态粘滞：createDone 已到达后的迟到进度事件不回拉横幅（防御层——终态后
    // 插件侧核对/发射应收流停摆，残余推送不改变已定格的终态呈现）
    if (createTerminalReached.value) return
    // 双计数分道（乙3 重叠期语义，phase 词汇对齐 selection.go）：verify=核对行
    //（作品数+读盘速率，终值 done=total 即核对完成）、create=发射行（已发射子行数）
    creating.value = true
    if (evt.phase === 'verify') {
      createVerify.value = { done: evt.done || 0, total: evt.total || 0, bytesPerSec: evt.bytesPerSec || 0 }
      // ETA 样本：事件间 Δdone/Δt 指数平滑（首事件只记样本、第二样本起可估）
      const now = Date.now()
      if (verifySample && evt.done > verifySample.done) {
        const perSec = (evt.done - verifySample.done) / Math.max(1, (now - verifySample.at) / 1000)
        verifyEmaWorksPerSec.value = verifyEmaWorksPerSec.value > 0 ? verifyEmaWorksPerSec.value * 0.6 + perSec * 0.4 : perSec
      }
      verifySample = { done: evt.done || 0, at: now }
    } else if (evt.phase === 'create') {
      createEmit.value = { done: evt.done || 0 }
    }
  } else if (evt.kind === 'createDone') {
    // 终态（帧4 取消/帧5 失败/帧6 完成）：文案优先消费后端 Message；取消终态按
    // 「已取消创建」前缀归类（✔ 呈现，区别于 ✖ 失败）；成功时选择集已被插件侧
    // 清空，本地同步镜像（已选计数归零）。终态粘滞位置位——后续迟到进度事件
    // 不再把横幅拉回进行态（复位=下一次创建发起）
    createTerminalReached.value = true
    creating.value = false
    createCancelling.value = false
    createCancelRaceNote.value = ''
    cancelDialogVisible.value = false
    createVerify.value = null
    createEmit.value = null
    verifySample = null
    verifyEmaWorksPerSec.value = 0
    if (evt.success) {
      createDoneKind.value = 'success'
      const name = taskNameOfMessage(evt.message) || createTaskLabel.value
      createDoneMessage.value = evt.message || `已创建任务「${name}」`
      selectedWorks.value = new Set()
    } else if ((evt.message || '').startsWith('已取消创建')) {
      createDoneKind.value = 'cancelled'
      createDoneMessage.value = evt.message || '已取消创建：选择集已保留，可重新发起'
    } else {
      createDoneKind.value = 'failed'
      createDoneMessage.value = evt.message || '任务创建失败（选择集已保留，可重试）'
    }
  }
}

// 事件摘要文案中的任务名（「已创建任务「名」：…」首对引号内；缺省回退提交名）
function taskNameOfMessage(message?: string): string {
  if (!message) return ''
  const m = message.match(/「([^」]+)」/)
  return m ? m[1] : ''
}

const phaseLabel = (phase: string): string => {
  switch (phase) {
    case 'scan': return '扫描目录结构'
    case 'index': return '构造库内谓词索引'
    case 'evaluate': return '合并求值（档案 ∪ 会话赋值）'
    case 'save': return '写入识别快照'
    default: return '准备中'
  }
}

const progressPercent = computed(() => {
  if (progress.value.total <= 0) return '12%'
  return `${Math.min(100, Math.round((progress.value.done / progress.value.total) * 100))}%`
})

// —— 资源级清单（虚拟滚动，切片渲染，千级/万级行不卡顿） —— //

const ROW_H = 30

// 过滤器行：状态 / 待裁决 / 已赋值 / 差异标
const fileFilters = [
  { key: 'all', label: '全部' },
  { key: 'identified', label: '已识别' },
  { key: 'clueful', label: '有线索' },
  { key: 'plain', label: '无线索' },
  { key: 'pending', label: '待裁决' },
  { key: 'assigned', label: '已赋值' },
  { key: 'diff', label: '差异标' }
]
const fileFilter = ref('all')

// el-scrollbar 组件实例公开面（宿主全局组件，零 import）：wrapRef = 滚动承载元素
// （隐藏原生条的实际滚动容器），虚拟切片窗口与选中行回滚均经它读写
interface ScrollbarInst {
  wrapRef?: HTMLDivElement | null
}
const listViewport = ref<ScrollbarInst | null>(null)
const treeViewport = ref<ScrollbarInst | null>(null)
const scrollTop = ref(0)
// 虚拟切片窗口高度：初值 420 供首帧渲染，挂载后随 wrap 元素实测高度持续同步
const viewportH = ref(420)

const pendingRelSet = computed<Set<string>>(() => {
  const set = new Set<string>()
  for (const p of (snapshot.value ? snapshot.value.pending : null) || []) set.add(p.relPath)
  return set
})

const filteredFiles = computed<SnapFile[]>(() => {
  const files = (snapshot.value ? snapshot.value.files : null) || []
  const key = fileFilter.value
  if (key === 'all') return files
  if (key === 'pending') return files.filter((f) => (f.pendingDims || []).length > 0 || pendingRelSet.value.has(f.relPath))
  if (key === 'assigned') return files.filter((f) => f.assigned === true)
  if (key === 'diff') return files.filter((f) => !!diffMark(f.relPath))
  return files.filter((f) => f.state === key)
})

// 可见切片窗口（含上下过扫）：2026-10-27 滚轮缺陷修复——虚拟行由「绝对定位+
// translateY」改为「静态占位条+常规流行」，占位条高度由本窗口派生（头条=start*行高、
// 尾条=总行数-end*行高，两者夹住切片行即还原全列表高度）。
// 根因（live 实测定位）：WebView2 合成器滚轮命中链遇到「绝对定位元素垫在滚动容器
// 与命中目标之间」即断链——滚轮落在虚拟行上时不再滚 wrap（规则页/树页常规流行正常，
// 同一 canvas 仅改 position:absolute 即复现失效）。故弃绝对定位，全部回归常规流。
const visibleRange = computed(() => {
  const start = Math.max(0, Math.floor(scrollTop.value / ROW_H) - 8)
  const end = Math.min(filteredFiles.value.length, start + Math.ceil(viewportH.value / ROW_H) + 16)
  return { start, end }
})

const visibleFiles = computed(() => {
  const { start, end } = visibleRange.value
  return filteredFiles.value.slice(start, end).map((f, i) => ({ ...f, index: start + i }))
})

// el-scrollbar 的 @scroll 载荷为 { scrollTop, scrollLeft }（宿主组件转发的滚动承载元素位移）
function onListScroll(pos: { scrollTop: number }): void {
  scrollTop.value = pos.scrollTop
}

// —— 主从联动：点清单行 → 选中键落右栏（与复选不互斥：行选中不改选择集，
//    复选改选择集不改行选中） —— //

// 右栏解析面板实例引用（文件/目录/集规则三实例互斥挂载，活动实例非空）：切换选中
// 目标前读面板的未保存标志（面板表单改动标志，见 ParsePanel / SetRulePanel）
const filePanelRef = ref<{ isFormDirty: () => boolean } | null>(null)
const dirPanelRef = ref<{ isFormDirty: () => boolean } | null>(null)
const setPanelRef = ref<{ isFormDirty: () => boolean } | null>(null)

function panelDirty(): boolean {
  const filePanel = filePanelRef.value
  if (filePanel && filePanel.isFormDirty()) return true
  const dirPanel = dirPanelRef.value
  if (dirPanel && dirPanel.isFormDirty()) return true
  const setPanel = setPanelRef.value
  return setPanel ? setPanel.isFormDirty() : false
}

// 未保存切换确认：表单有未保存改动时切换选中目标先暂存于此——「丢弃并切换」
// 才落选中键（面板随选中变化的复位逻辑在此之后才触发），「留在当前去保存」丢弃
// 暂存、保持现选中，用户先保存再切换即无确认直通
type PendingSwitch =
  | { kind: 'file'; relPath: string }
  | { kind: 'dir'; relKey: string }
  | { kind: 'exitDir' }
  | { kind: 'set'; target: string; setName: string }
  | { kind: 'exitSet' }
const pendingSwitch = ref<PendingSwitch | null>(null)

function applyPendingSwitch(): void {
  const pending = pendingSwitch.value
  pendingSwitch.value = null
  if (!pending) return
  if (pending.kind === 'file') {
    selectedDirKey.value = null
    setPanelTarget.value = ''
    selectedRelPath.value = pending.relPath
  } else if (pending.kind === 'dir') {
    setPanelTarget.value = ''
    selectedDirKey.value = pending.relKey
  } else if (pending.kind === 'set') {
    selectedDirKey.value = null
    selectedRelPath.value = ''
    setPanelTarget.value = pending.target
    setPanelName.value = pending.setName
  } else if (pending.kind === 'exitSet') {
    setPanelTarget.value = ''
  } else {
    selectedDirKey.value = null
  }
}

function selectFile(relPath: string): void {
  // 点文件行 = 现有联动（F5/J7）；同时让出目录/集规则面板（三面板互斥，文件优先）。
  // 面板有未保存改动时暂存切换目标待确认；点同一文件仅让出其他面板也属切换，同样确认
  if (panelDirty() && (relPath !== selectedRelPath.value || selectedDirKey.value !== null || setPanelTarget.value !== '')) {
    pendingSwitch.value = { kind: 'file', relPath: relPath }
    return
  }
  selectedDirKey.value = null
  setPanelTarget.value = ''
  selectedRelPath.value = relPath
}

// 自动刷新（重识别）后清单虚拟滚动重排：选中行滚回可视区（平铺虚拟清单专用；
// 滚动承载=el-scrollbar 的 wrap 元素）
function scrollSelectedIntoView(): void {
  if (!selectedRelPath.value || listMode.value !== 'flat') return
  const view = listViewport.value ? listViewport.value.wrapRef : null
  if (!view) return
  const idx = filteredFiles.value.findIndex((f) => f.relPath === selectedRelPath.value)
  if (idx < 0) return
  const top = idx * ROW_H
  const bottomEdge = view.scrollTop + view.clientHeight
  if (top < view.scrollTop || top + ROW_H > bottomEdge) {
    view.scrollTop = Math.max(0, top - Math.floor(view.clientHeight / 2))
  }
}

// 融合弹窗点待裁决文件名：关弹窗 → 清单定位该行 → 既有 selectFile 链路联动右栏。
// 定位含三步兜底：文件被当前过滤器隐藏时回全量过滤；候选卡墙标签下切到平铺（行级
// 呈现面）；树模式下展开该文件目录链（库根 → 各级目录）后滚到选中行
function jumpToPendingFile(relPath: string): void {
  fusionDialogVisible.value = false
  if (!filteredFiles.value.some((f) => f.relPath === relPath)) fileFilter.value = 'all'
  if (listMode.value === 'sets') listMode.value = 'flat'
  if (listMode.value === 'tree') {
    const file = fileByRel.value.get(relPath)
    if (file && file.dirRelKey) {
      const next = new Set(treeExpanded.value)
      let key = file.dirRelKey
      while (key) {
        next.add(key)
        const cut = key.lastIndexOf('/')
        key = cut >= 0 ? key.slice(0, cut) : ''
      }
      treeExpanded.value = next
    }
    selectFile(relPath)
    void nextTick(() => {
      const row = treeViewport.value && treeViewport.value.wrapRef
        ? treeViewport.value.wrapRef.querySelector('.li-ws-tree-file.is-selected')
        : null
      if (row) row.scrollIntoView({ block: 'center' })
    })
  } else {
    selectFile(relPath)
    // v-if 惰性挂载：自卡墙切到平铺后 el-scrollbar 与其内部 wrapRef 需经挂载
    // 补齐（内部结构跨组件层级，双 nextTick 确保滚动 ref 就绪再定位；平铺本就
    // 打开时多等一拍无可感差异）
    void nextTick(() => void nextTick(() => scrollSelectedIntoView()))
  }
}

// —— 作品级选择集（清单行复选 = 该文件所属作品的原子选择） —— //

const allWorks = computed<SelectableWorkView[]>(() => (domain.value ? domain.value.works : null) || [])
const selectableWorks = computed<SelectableWorkView[]>(() => allWorks.value.filter((w) => w.selectable))

const workIndex = computed<Map<string, SelectableWorkView>>(() => {
  const index = new Map<string, SelectableWorkView>()
  for (const w of allWorks.value) index.set(w.workId, w)
  return index
})

// 文件相对路径 → 所属作品（域内每文件恰属一作品；集成员=单文件键，目录聚合=目录键）
const workByRel = computed<Map<string, SelectableWorkView>>(() => {
  const index = new Map<string, SelectableWorkView>()
  for (const w of allWorks.value) {
    for (const rel of w.files) index.set(rel, w)
  }
  return index
})

const workIdOf = (rel: string): string => {
  const w = workByRel.value.get(rel)
  return w ? w.workId : ''
}
const isWorkSelectable = (rel: string): boolean => {
  const w = workByRel.value.get(rel)
  return !!w && w.selectable
}
const workHintOf = (rel: string): string => {
  const w = workByRel.value.get(rel)
  if (!w) return '不在可选作品域'
  return w.selectable ? '' : `${workStateLabel(w.state)}（整体不可选）`
}

const workStateLabel = (state: string): string => {
  switch (state) {
    case 'identified': return '已识别'
    case 'clueful': return '有价值未识别'
    case 'plain': return '未识别'
    case 'pending': return '待裁决'
    case 'ignored': return '已忽略'
    default: return state
  }
}

const fileByRel = computed<Map<string, SnapFile>>(() => {
  const index = new Map<string, SnapFile>()
  for (const f of (snapshot.value ? snapshot.value.files : null) || []) index.set(f.relPath, f)
  return index
})

const isWorkSelected = (id: string): boolean => selectedWorks.value.has(id)

function toggleWork(id: string): void {
  const work = workIndex.value.get(id)
  if (!work || !work.selectable) return
  const next = new Set(selectedWorks.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedWorks.value = next
}

function toggleWorkAt(rel: string): void {
  const id = workIdOf(rel)
  if (id) toggleWork(id)
}

function addWorks(works: SelectableWorkView[], on: boolean): void {
  const next = new Set(selectedWorks.value)
  for (const w of works) {
    if (!w.selectable) continue
    if (on) next.add(w.workId)
    else next.delete(w.workId)
  }
  selectedWorks.value = next
}

// 当前过滤条件命中的作品集（清单行复选的可见域，去重保序）
const filteredWorks = computed<SelectableWorkView[]>(() => {
  const seen = new Set<string>()
  const out: SelectableWorkView[] = []
  for (const f of filteredFiles.value) {
    const w = workByRel.value.get(f.relPath)
    if (w && !seen.has(w.workId)) {
      seen.add(w.workId)
      out.push(w)
    }
  }
  return out
})

// —— 组合选择：全选/反选可见 + 按规则/声明命中（命中集=任一文件带该命中痕迹的作品） —— //

function selectAllVisible(): void {
  addWorks(filteredWorks.value, true)
}

function invertVisible(): void {
  // 反选仅作用于当前可见集的可选项（选中→去、未选→进）
  const next = new Set(selectedWorks.value)
  for (const w of filteredWorks.value) {
    if (!w.selectable) continue
    if (next.has(w.workId)) next.delete(w.workId)
    else next.add(w.workId)
  }
  selectedWorks.value = next
}

function clearSelection(): void {
  selectedWorks.value = new Set()
}

const ruleOptions = computed(() =>
  ((snapshot.value ? snapshot.value.ruleStats : null) || []).filter((r) => r.hitCount > 0)
)

function onRulePick(): void {
  const ruleId = rulePick.value
  rulePick.value = ''
  if (!ruleId) return
  const hit = allWorks.value.filter((w) =>
    w.files.some((rel) => {
      const f = fileByRel.value.get(rel)
      return !!f && (f.matchedRuleIds || []).indexOf(ruleId) >= 0
    })
  )
  addWorks(hit, true)
}

const authorOptions = computed<string[]>(() => {
  const authors = new Set<string>()
  for (const f of (snapshot.value ? snapshot.value.files : null) || []) {
    for (const d of f.final || []) {
      if (d.dimension === 'author' && d.value) authors.add(d.value)
    }
  }
  return [...authors].sort()
})

function onAuthorPick(): void {
  const author = authorPick.value
  authorPick.value = ''
  if (!author) return
  const hit = allWorks.value.filter((w) =>
    w.files.some((rel) => {
      const f = fileByRel.value.get(rel)
      return !!f && (f.final || []).some((d) => d.dimension === 'author' && d.value === author)
    })
  )
  addWorks(hit, true)
}

// —— 目录树递选（子树语义：前缀匹配，库根=全部；折叠区目录行复用） —— //

const worksUnderDirTree = (relKey: string): SelectableWorkView[] =>
  allWorks.value.filter((w) => relKey === '' || w.dirRelKey === relKey || w.dirRelKey.startsWith(relKey + '/'))

// 三态聚合映射：单遍遍历全部作品，把 {selected, selectable, total} 计数累加到各作品
// dirRelKey 及其全部祖先前缀键（含库根 ''）——前缀语义与 worksUnderDirTree 逐字等价
// （relKey==='' 匹配全部作品 / dirRelKey 等值 / startsWith(relKey+'/') 即祖先链逐级）。
// 仅依赖 allWorks 与 selectedWorks（写点全部为 Set 整体替换，无原地变异），载入完成
// 或选择集变更时重算一次。旧三函数每次渲染逐行各跑一遍全库 filter（可见目录行 ×3 次
// O(作品数) 扫描，真实库 4,528 作品展开一次 ≈ 2 亿次比对），现改 O(1) 查表
interface DirTreeAgg {
  selected: number
  selectable: number
  total: number
}

const dirTreeAgg = computed<Map<string, DirTreeAgg>>(() => {
  const agg = new Map<string, DirTreeAgg>()
  const bucketOf = (key: string): DirTreeAgg => {
    let b = agg.get(key)
    if (!b) {
      b = { selected: 0, selectable: 0, total: 0 }
      agg.set(key, b)
    }
    return b
  }
  for (const w of allWorks.value) {
    const selected = selectedWorks.value.has(w.workId)
    const selectable = w.selectable
    const count = (key: string): void => {
      const b = bucketOf(key)
      if (selected) b.selected++
      if (selectable) b.selectable++
      b.total++
    }
    let key = w.dirRelKey
    while (key) {
      count(key)
      const cut = key.lastIndexOf('/')
      key = cut >= 0 ? key.slice(0, cut) : ''
    }
    count('') // 库根聚合全部作品（worksUnderDirTree 的 relKey==='' 分支）
  }
  return agg
})

// 未命中键（空子树/防御）→ 全零三态：checked/indeterminate false、文案 0/0，与空子树现状一致
const dirTreeChecked = (relKey: string): boolean => {
  const b = dirTreeAgg.value.get(relKey)
  return !!b && b.selectable > 0 && b.selected === b.selectable
}

const dirTreeIndeterminate = (relKey: string): boolean => {
  const b = dirTreeAgg.value.get(relKey)
  return !!b && b.selected > 0 && b.selected < b.selectable
}

const dirTreeSelectedText = (relKey: string): string => {
  const b = dirTreeAgg.value.get(relKey)
  return `${b ? b.selected : 0}/${b ? b.total : 0}`
}

function setDirTreeSelected(relKey: string, on: boolean): void {
  addWorks(worksUnderDirTree(relKey), on)
}

// —— 清单树模式（剧本F：库根 → 目录 → 文件；骨架复用 snapshot.dirs，递选复用上方子树语义） —— //

interface TreeDirNode {
  relKey: string
  name: string
  depth: number
  fileCount: number
  meanings: { type: string; id?: string; name?: string }[]
  meaningRuleId?: string
  // 剧本J2：目录级结构分段线索集（snapshot.dirs 投影携带；防御性补全节点无值→空集）
  structClues: { ref: string; kind: string; label: string; value: string }[]
  files: SnapFile[]
  children: TreeDirNode[]
}

// 树行（拍平渲染模型）：kind 判别 + 预生成 key；目录/文件字段按 kind 二选一存在
interface TreeRow {
  kind: 'dir' | 'file'
  key: string
  depth: number
  dir?: TreeDirNode
  file?: SnapFile
}

// 目录树骨架：snapshot.dirs（旧「目录结构」折叠区同一数据源）按 relKey 前缀挂接父子；
// 文件按当前过滤条件分桶进各目录（模式切换时过滤保持）；快照目录清单缺位的路径防御性补全
const dirRoots = computed<TreeDirNode[]>(() => {
  const filesByDir = new Map<string, SnapFile[]>()
  for (const f of filteredFiles.value) {
    const list = filesByDir.get(f.dirRelKey)
    if (list) list.push(f)
    else filesByDir.set(f.dirRelKey, [f])
  }
  type DirRow = PanoramaSnapshot['dirs'][number]
  const nodeByRel = new Map<string, TreeDirNode>()
  const ensure = (relKey: string, row?: DirRow): TreeDirNode => {
    const found = nodeByRel.get(relKey)
    if (found) {
      if (row) {
        found.name = row.name
        found.depth = row.depth
        found.fileCount = row.fileCount
        found.meanings = row.meanings || []
        found.meaningRuleId = row.meaningRuleId
        found.structClues = row.structClues || []
      }
      return found
    }
    const segs = relKey.split('/')
    const node: TreeDirNode = {
      relKey: relKey,
      name: row ? row.name : segs[segs.length - 1],
      depth: row ? row.depth : segs.length,
      fileCount: row ? row.fileCount : 0,
      meanings: (row && row.meanings) || [],
      meaningRuleId: row ? row.meaningRuleId : undefined,
      structClues: (row && row.structClues) || [],
      files: [],
      children: []
    }
    nodeByRel.set(relKey, node)
    return node
  }
  for (const d of (snapshot.value ? snapshot.value.dirs : null) || []) ensure(d.relKey, d)
  for (const key of filesByDir.keys()) {
    let cur = key
    while (cur) {
      ensure(cur)
      const cut = cur.lastIndexOf('/')
      cur = cut >= 0 ? cur.slice(0, cut) : ''
    }
  }
  const roots: TreeDirNode[] = []
  for (const node of nodeByRel.values()) {
    node.files = filesByDir.get(node.relKey) || []
    // 一级目录（键不含 '/'）的父节点=库根（relKey 空串行，快照目录清单恒含）；
    // 库根自身无父、挂树根
    const cut = node.relKey.lastIndexOf('/')
    const parent = node.relKey === '' ? undefined : nodeByRel.get(cut >= 0 ? node.relKey.slice(0, cut) : '')
    if (parent) parent.children.push(node)
    else roots.push(node)
  }
  return roots
})

const dirByKey = computed<Map<string, TreeDirNode>>(() => {
  const index = new Map<string, TreeDirNode>()
  const walk = (nodes: TreeDirNode[]): void => {
    for (const n of nodes) {
      index.set(n.relKey, n)
      walk(n.children)
    }
  }
  walk(dirRoots.value)
  return index
})

// 展开态（会话内记忆不持久化；默认全收起，进入只见一级目录行）
const treeExpanded = ref<Set<string>>(new Set())

function toggleTreeDir(relKey: string): void {
  const next = new Set(treeExpanded.value)
  if (next.has(relKey)) next.delete(relKey)
  else next.add(relKey)
  treeExpanded.value = next
}

// 库根直挂文件（dirRelKey 为空）：置顶平铺，避免树外丢失
const rootLevelFiles = computed<SnapFile[]>(() => filteredFiles.value.filter((f) => !f.dirRelKey))

// 树可见行序列：展开目录的内容行（子目录在前、文件在后）；全量渲染不虚拟化（246 行级数据量）。
// 库根节点（relKey 空串）为隐式容器：不出目录行、子目录恒可见（树入口即一级目录行），
// 其直挂文件已由置顶平铺承载、不再随展开重复出
const treeRows = computed<TreeRow[]>(() => {
  const rows: TreeRow[] = []
  for (const f of rootLevelFiles.value) rows.push({ kind: 'file', key: `f:${f.relPath}`, depth: 0, file: f })
  const emit = (node: TreeDirNode, implicitRoot: boolean): void => {
    if (!implicitRoot) rows.push({ kind: 'dir', key: `d:${node.relKey}`, depth: node.depth, dir: node })
    if (implicitRoot || treeExpanded.value.has(node.relKey)) {
      for (const child of node.children) emit(child, false)
      if (!implicitRoot) {
        for (const f of node.files) rows.push({ kind: 'file', key: `f:${f.relPath}`, depth: node.depth + 1, file: f })
      }
    }
  }
  for (const root of dirRoots.value) emit(root, root.relKey === '')
  return rows
})

// —— 右栏目录解析面板（剧本J：卡+解析合一，保存作用子树） —— //
// 选中键三态：null=文件面板；''=库根目录（assignDir/clearDirAssignments 的
// 全树寻址）；非空=子目录键。点目录名进入、点文件行/「返回文件面板」让出（J7）
const selectedDirKey = ref<string | null>(null)
const selectedDirNode = computed<TreeDirNode | null>(() =>
  selectedDirKey.value === null ? null : dirByKey.value.get(selectedDirKey.value) || null
)

function showDirCard(relKey: string): void {
  // 点目录名进入目录面板（同时让出集规则面板）；面板有未保存改动时暂存切换
  // 目标待确认（含目录间切换）
  if (panelDirty() && (relKey !== selectedDirKey.value || setPanelTarget.value !== '')) {
    pendingSwitch.value = { kind: 'dir', relKey: relKey }
    return
  }
  setPanelTarget.value = ''
  selectedDirKey.value = relKey
}

// 目录面板「返回文件面板」：让出目录面板会卸载目录表单实例、未保存改动随之
// 丢失，故同样先确认
function exitDirPanel(): void {
  if (panelDirty()) {
    pendingSwitch.value = { kind: 'exitDir' }
    return
  }
  selectedDirKey.value = null
}

// 缺陷①②修复配套守卫：切换面板目标（换文件/换目录/文件↔目录互斥切）即弃管
// 托管的动作通知与规则建议——新目标不该复活旧动作的反馈；全景刷新周期内的
// 卸载-重挂不属切换（选中文件键与目录键均不变、不触发本守卫），通知/建议
// 得以跨刷新续读。
watch(
  [() => selectedRelPath.value, () => selectedDirKey.value],
  () => {
    panelNotice.value = ''
    panelSuggestion.value = null
  }
)

// 剧本J 目录模式上下文（与 ParsePanel PanelDirContext 同构；ParsePanel 目录模式消费）
interface DirPanelContext {
  relKey: string
  name: string
  depth: number
  meanings: { type: string; id?: string; name?: string }[]
  meaningRuleId?: string
  structClues: { ref: string; kind: string; label: string; value: string }[]
  subtreeFileCount: number
  subdirCount: number
}

// 剧本J1/J3：子树文件数从 snapshot.files 按 dirRelKey 前缀统计（''=全库）——
// 树节点的文件聚合受清单过滤影响，不可作计数源
function subtreeFileCountOf(relKey: string): number {
  const files = (snapshot.value ? snapshot.value.files : null) || []
  if (!relKey) return files.length
  const prefix = relKey + '/'
  let count = 0
  for (const f of files) {
    if (f.dirRelKey === relKey || f.dirRelKey.startsWith(prefix)) count += 1
  }
  return count
}

// 子目录数从 snapshot.dirs 前缀统计（不含自身；库根=全部目录减根条目）
function subdirCountOf(relKey: string): number {
  const dirs = (snapshot.value ? snapshot.value.dirs : null) || []
  if (!relKey) return dirs.filter((d) => d.relKey !== '').length
  const prefix = relKey + '/'
  return dirs.filter((d) => d.relKey.startsWith(prefix)).length
}

// 目录面板上下文组装：随快照/选中目录重算；对象每次重建但 relKey 稳定，
// ParsePanel 侧仅凭 relKey 变化复位（刷新不清目录表单）
const dirPanelContext = computed<DirPanelContext | null>(() => {
  const node = selectedDirNode.value
  if (!node) return null
  return {
    relKey: node.relKey,
    name: node.name,
    depth: node.depth,
    meanings: node.meanings || [],
    meaningRuleId: node.meaningRuleId,
    structClues: node.structClues || [],
    subtreeFileCount: subtreeFileCountOf(node.relKey),
    subdirCount: subdirCountOf(node.relKey)
  }
})

// 悬空选择键（重识别后选择集引用了已不存在的作品）提示
const danglingSelection = computed<number>(() => {
  let count = 0
  for (const id of selectedWorks.value) {
    if (!workIndex.value.has(id)) count += 1
  }
  return count
})

// —— 剧本K 创建任务名称确认：预填默认名 → 确认后走既有创建链路 —— //

// 库根名 = rootPath 末段（兼容 / 与 \ 分隔、吞尾部分隔符，对齐后端 filepath.Base）
const libraryRootName = computed<string>(() => {
  const root = (session.value && session.value.rootPath) || ''
  const parts = root.split(/[\\/]+/).filter(Boolean)
  return parts.length > 0 ? parts[parts.length - 1] : root
})

// 默认任务名「本地导入 · <库根名> · N 件作品」——与后端空名兜底同规则（方案 §3.2），
// 前端仅作输入框预填，最终裁决以后端 resolveCreateTaskName 为准
const defaultTaskName = computed<string>(
  () => `本地导入 · ${libraryRootName.value} · ${selectedWorks.value.size} 件作品`
)

// 框下小字的 M：选择集作品文件总数（按选择集聚合各作品 files 数求和；
// 悬空选择键不在 workIndex 内、无文件数可计按 0——悬空本身另有创建栏提示拦阻）
const selectedFileCount = computed<number>(() => {
  let count = 0
  for (const id of selectedWorks.value) {
    const work = workIndex.value.get(id)
    if (work) count += (work.files && work.files.length) || 0
  }
  return count
})

// 剧本K1：点「创建任务」→ 弹名称确认对话框（预填默认名，可编辑，空=用默认）；
// 空选择守卫与原直驱路径一致（按钮 disabled 之外的二道防线）
function openCreateDialog(): void {
  if (!session.value) return
  if (selectedWorks.value.size === 0) {
    createError.value = '未选择任何作品：请在清单勾选要导入的作品后再创建'
    return
  }
  createTaskName.value = defaultTaskName.value
  createDialogVisible.value = true
}

// —— 创建任务：选择集整集落库 → 创建编排（立即应答→插件侧 goroutine 创建→事件终态） —— //

// 剧本K2/剧本2·帧1（大载荷分页方案·设计 C）：确认「创建」→ 弹窗立即关闭（不等
// 创建完成）；taskName 随输入原样上送（清空/全空白由后端默认名兜底）。createTasks
// 立即应答 started（守卫链失败=红横幅+选择集保留，语义与现状一致）；已开始则两相
// 进度横幅亮起，进度与终态经 createProgress/createDone 事件推送（卡底横幅呈现，
// 清单/目录树浏览不阻塞），成功转绿色通知、失败转中断横幅（选择集保留可重试）
async function createTasks(): Promise<void> {
  createDialogVisible.value = false
  if (!session.value) return
  if (selectedWorks.value.size === 0) {
    createError.value = '未选择任何作品：请在清单勾选要导入的作品后再创建'
    return
  }
  creating.value = true
  createError.value = ''
  createDoneMessage.value = ''
  createDoneKind.value = 'success'
  // 新一次创建发起：解除终态粘滞（上一轮终态后的迟到事件防线随新轮重置）
  createTerminalReached.value = false
  createCancelling.value = false
  createCancelRaceNote.value = ''
  // 通知名预备：成功事件文案携带名为准，空输入回退前端同规则默认名
  createTaskLabel.value = createTaskName.value.trim() || defaultTaskName.value
  // 横幅先行亮起（立即应答与首条 createProgress 之间的空窗也有呈现）：verify 道
  // 无计数形态待首事件 0/N 到达真分母（甲1），发射道待首批发射事件出现（帧2 起）
  createVerify.value = { done: 0, total: 0, bytesPerSec: 0 }
  createEmit.value = null
  verifySample = null
  verifyEmaWorksPerSec.value = 0
  let started = false
  try {
    const sel = await request('setSelection', { root: session.value.rootPath, works: [...selectedWorks.value] })
    if (!sel.ok) {
      createError.value = sel.error || '选择集写入失败'
      return
    }
    // 选择集轻量回执：本地决策态镜像只同步 selection 子集（全量 decisions 不
    // 随动作应答整答——大载荷断路器收口）
    if (sel.selection && decisions.value) decisions.value.selection = sel.selection
    const resp = await request('createTasks', { root: session.value.rootPath, taskName: createTaskName.value })
    if (!resp.ok) {
      createError.value = resp.error || '任务创建失败'
      return
    }
    started = resp.started === true
    if (!started) {
      createError.value = resp.error || '任务创建未能开始'
      return
    }
    // 已开始创建：立即应答携选择集轻量回执（此刻选择集尚未被清空）——回读
    // 选择态；清空随 createDone 成功态发生
    if (resp.selection && decisions.value) decisions.value.selection = resp.selection
    selectedWorks.value = new Set(resp.selection || [])
  } finally {
    if (!started) {
      // 未进入异步创建（守卫失败/超时）：横幅即刻收起
      creating.value = false
      createVerify.value = null
      createEmit.value = null
    }
    // started=true 保持进度横幅，收尾归 createDone 事件
  }
}

// —— 取消通路（甲3/决策3，剧本1·帧3→帧4）—— //

// 帧3：取消确认对话框（逐字文案过审）——「继续创建」= 关窗回到进行中形态（无副作用）
function openCancelConfirm(): void {
  createCancelRaceNote.value = ''
  cancelDialogVisible.value = true
}

// 帧3「确定取消」：cancelCreate 三态应答——cancelling=已受理（横幅转「正在清理
// 在途任务…」，帧4 终态经 createDone 事件）；completed=恰好已完成（原子提交点
// 已过、未执行取消——风险10，附注呈现并照常收帧6 完成通知）；idle（ok=false 无
// 在途创建）=终态已先行到达，界面随终态横幅呈现
async function confirmCancelCreate(): Promise<void> {
  if (!session.value || createCancelPending.value) return
  createCancelPending.value = true
  try {
    const resp = await request('cancelCreate', { root: session.value.rootPath })
    if (resp.cancelState === 'cancelling') {
      createCancelling.value = true
    } else if (resp.cancelState === 'completed') {
      createCancelRaceNote.value = '刚好已完成，未执行取消'
    }
  } finally {
    createCancelPending.value = false
    cancelDialogVisible.value = false
  }
}

// —— 呈现辅助 —— //

const stateLabel = (state: string): string => {
  switch (state) {
    case 'identified': return '已识别'
    case 'clueful': return '有价值未识别'
    case 'pending': return '待裁决'
    case 'ignored': return '已忽略'
    default: return '未识别'
  }
}
const stateDot = (state: string): string => {
  switch (state) {
    case 'identified': return '●'
    case 'clueful': return '◐'
    default: return '○'
  }
}
const dimLabel = (dim: string): string => {
  switch (dim) {
    case 'author': return '作者'
    case 'title': return '作品名'
    case 'tag': return '标签'
    case 'collection': return '作品集'
    default: return dim
  }
}
const sourceLabel = (source: string): string => {
  switch (source) {
    case 'dir': return '目录'
    case 'filename': return '文件名'
    case 'constant': return '常量'
    case 'group': return '群组前缀'
    case 'predicate': return '库内命中'
    case 'assignment': return '手工赋值'
    case 'adjudication': return '裁决'
    case 'qna': return '泛化'
    case 'builtin': return '内置'
    case 'manual': return '手写'
    default: return source
  }
}
const clueLabel = (clue: { kind: string; detail: string }): string => {
  switch (clue.kind) {
    case 'predicate-author': return `库内作者：${clue.detail}`
    case 'predicate-tag': return `库内标签：${clue.detail}`
    case 'pixiv-id': return `pixiv 形态：${clue.detail}`
    case 'group-prefix': return `集名候选：${clue.detail}`
    case 'set-name-match': return `集名互证：${clue.detail}`
    default: return clue.detail
  }
}
const meaningLabel = (m: { type: string; name?: string }): string => {
  const labels: Record<string, string> = {
    localAuthor: '作者',
    siteAuthor: '站点作者',
    localTag: '标签',
    siteTag: '站点标签',
    workName: '作品名',
    workSet: '作品集',
    site: '站点',
    unknown: '未知'
  }
  const label = labels[m.type] || m.type
  return m.name ? `${label}:${m.name}` : label
}
const declLabel = (d: SnapDecl): string => {
  let text = `${dimLabel(d.dimension)}=${d.value}`
  if (d.roleName) text += `（${d.roleName}）`
  return text
}
const fileNameOf = (relPath: string): string => {
  const idx = relPath.lastIndexOf('/')
  return idx >= 0 ? relPath.slice(idx + 1) : relPath
}

// —— 重识别差异高亮（生成规则后的强制验证轮：过窄=仍有未覆盖、过泛=错套） —— //

const diffIndex = computed<Map<string, string>>(() => {
  const index = new Map<string, string>()
  const d = diff.value
  if (!d) return index
  for (const entry of d.changed || []) index.set(entry.relPath, 'changed')
  for (const entry of d.added || []) index.set(entry.relPath, 'added')
  for (const entry of d.removed || []) if (!index.has(entry.relPath)) index.set(entry.relPath, 'removed')
  return index
})

const diffMark = (relPath: string): string => diffIndex.value.get(relPath) || ''

const diffTotal = computed(() => {
  const d = diff.value
  if (!d) return 0
  return (d.changed ? d.changed.length : 0) + (d.added ? d.added.length : 0) + (d.removed ? d.removed.length : 0)
})

const formatTime = (ms: number): string => {
  const date = new Date(ms)
  const pad = (n: number) => `${n}`.padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

// —— 清单滚动体与切片窗口持续同步（清单高度由卡内 flex 链分配，无需 JS 注入） —— //

let viewportRO: ResizeObserver | null = null

// 滚动承载元素尺寸变化 → viewportH 同步（窗口缩放/右栏收起/模式切换后切片窗口正确）
// 保活切回（activated）亦直接调用做一次即时补测——保活期窗口尺寸变化观察器断线
// 不回传，重挂后立即重测防切片窗口陈旧
function syncViewportH(): void {
  const el = listViewport.value ? listViewport.value.wrapRef : null
  if (el) viewportH.value = el.clientHeight || 420
}

function bindViewportRO(): void {
  const wrap = listViewport.value ? listViewport.value.wrapRef : null
  if (!wrap) return
  if (viewportRO) viewportRO.disconnect()
  viewportRO = new ResizeObserver(syncViewportH)
  viewportRO.observe(wrap)
}

// 快照晚到（挂载后才识别）/ 标签体 v-if 惰性挂载时清单滚动体才出现，经模板
// ref 变化接入观察。v-if 重挂=全新滚动体（实际滚动位归零），切片窗口基准
// scrollTop 同步归零，防残留值与新 DOM 滚动位错位（切走再切回见空白墙）
watch(listViewport, (inst) => {
  if (!inst) return
  scrollTop.value = 0
  void nextTick(bindViewportRO)
})

// —— 卡墙增量渲染哨兵接线（IntersectionObserver；哨兵随卡墙标签 v-if 挂载/卸载，
//    组件失活/卸载由 deactivated/onUnmounted 统一断线） —— //

let cardsIO: IntersectionObserver | null = null
const cardsSentinel = ref<HTMLElement | null>(null)

// 重新挂起观察=按当前相交状态重发回调：用于①触底追加一批后哨兵仍在视口内
// （一批不足以填屏）时继续追加，②底层清单重置上限后哨兵本就可见时立即续批
// （IntersectionObserver 只在相交态变化时回调，静默 re-observe 迫使其重估）
function rearmCardsIO(): void {
  const node = cardsSentinel.value
  if (!node || !cardsIO) return
  cardsIO.unobserve(node)
  cardsIO.observe(node)
}

function bindCardsIO(): void {
  const el = cardsSentinel.value
  if (!el) return
  if (cardsIO) cardsIO.disconnect()
  // 默认根（视口）+400px 预取带：哨兵接近可视区即续批，滚动衔接无停顿
  cardsIO = new IntersectionObserver((entries) => {
    if (!entries.some((e) => e.isIntersecting)) return
    if (cardRenderLimit.value >= visibleCandidates.value.length) return
    cardRenderLimit.value += CARD_BATCH
    rearmCardsIO()
  }, { rootMargin: '400px' })
  cardsIO.observe(el)
}

// 哨兵挂载（卡墙标签打开且候选未渲染完）→ 接线；卸载（见底撤哨/离开卡墙）→ 断线
watch(cardsSentinel, (el) => {
  if (el) {
    void nextTick(bindCardsIO)
  } else if (cardsIO) {
    cardsIO.disconnect()
    cardsIO = null
  }
})

// 底层清单变更（「隐藏已识别」翻转/快照刷新重灌候选）：上限重置回首批，防翻
// 全量瞬间再炸；哨兵若仍在视口（或重置后被推回可视区）经 rearm 立即续批
watch(visibleCandidates, () => {
  cardRenderLimit.value = CARD_BATCH
  rearmCardsIO()
})

// 离开卡墙标签：v-if 卸载整墙 DOM（滚动位随之丢失），上限同步重置回首批，
// 重进从 100 张起步（与滚动归零一致，防重进瞬间重建巨量卡片）
watch(listMode, (mode) => {
  if (mode !== 'sets') cardRenderLimit.value = CARD_BATCH
})

// —— 保活生命周期迁移（本页声明 keepAlive；契约见「视图切换保活方案·插件侧生命
//    周期契约」）—— //
// document 级监听与观察器随 deactivated 断开、activated 重挂（契约1/2：保活态不在
// 其他页面的事件链上运行；切回重挂并立即重测，防保活期窗口/右栏尺寸变化陈旧）；
// Events.On 保持长活（契约3）：仅 mounted 注册、unmounted 断开、失活不断——保活
// 期后台继续收识别进度/完成/创建链事件（recognitionDone 自动重载全景在后台完成，
// 切回即最新，创建链两相横幅进度连续）。

const NAME_SEL_EVENTS = ['click', 'keyup', 'select', 'input'] as const

// 光标追踪（集名输入框 chips 插入用）：事件流维护选区快照
function bindDocumentListeners(): void {
  document.addEventListener('selectionchange', onNameSelChange)
  for (const t of NAME_SEL_EVENTS) {
    document.addEventListener(t, onNameSelDelegate, true)
  }
}

function unbindDocumentListeners(): void {
  document.removeEventListener('selectionchange', onNameSelChange)
  for (const t of NAME_SEL_EVENTS) {
    document.removeEventListener(t, onNameSelDelegate, true)
  }
}

// activated 对账并发簿记：快速往返切换时仅最新一轮生效，迟到应答静默废弃
let reconcileSeq = 0
// 挂载路径恢复是否已落定（成功或失败）——区分「首挂恢复在途」（activated 紧随
// mounted 触发，由挂载路径接管）与「此前恢复失败/无会话」（重试整条恢复路径）
let initialRestoreSettled = false

// activated 轻对账（契约4，幂等——首挂后的 activated 也要过）：比对地址库根与当前
// 会话——不同（会话切换）重走等价挂载路径（restoreSession + 按态装载全景）；相同
// 只发一次 activateSession 轻请求做档案对账（刷新 session 状态与 profileChanged
// 横幅），不重载全景（快照未变时重载零收益，保活目的所在）
async function reconcileOnActivated(): Promise<void> {
  const seq = ++reconcileSeq
  const hashRoot = rootFromHash()
  if (!session.value && !initialRestoreSettled) return
  if (session.value && (!hashRoot || hashRoot === session.value.rootPath)) {
    const resp = await request('activateSession', { root: session.value.rootPath })
    if (seq !== reconcileSeq) return
    if (resp.ok && resp.session) {
      session.value = resp.session
      profileChanged.value = resp.profileChanged === true
      loadError.value = ''
    } else {
      loadError.value = resp.error || '恢复解析会话失败'
    }
    return
  }
  const root = await restoreSession()
  if (seq !== reconcileSeq) return
  if (root && session.value) {
    if (session.value.state === 'recognized') {
      await loadPanorama(root)
    } else {
      // 新会话未识别：清旧会话呈现，回到引导/识别进度面（等价首挂载的空态）
      snapshot.value = null
      decisions.value = null
      diff.value = null
      domain.value = null
    }
    await nextTick()
    bindViewportRO()
  }
}

onMounted(async () => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  offEvent = Events.On(TOPIC_EVENT, onEvent)
  bindDocumentListeners()
  try {
    const root = await restoreSession()
    // 恢复落定即撤「恢复中」初态（成功→按态装载全景/空态；失败→loadError 横幅），
    // 避免占位与后续 panoLoading 载入态重叠
    sessionRestoring.value = false
    if (root && session.value) {
      if (session.value.state === 'recognized') {
        await loadPanorama(root)
      }
      await nextTick()
      bindViewportRO()
    }
  } finally {
    sessionRestoring.value = false
    initialRestoreSettled = true
  }
})

onActivated(() => {
  // document 监听重挂（契约1；首挂与 mounted 的重复注册由浏览器同引用去重吸收）+
  // 观察器重挂并立即重测一次（契约2：viewportH 即时补测，哨兵 re-observe 重估）+
  // 会话/档案轻对账（契约4）
  bindDocumentListeners()
  syncViewportH()
  bindViewportRO()
  if (cardsSentinel.value) bindCardsIO()
  void reconcileOnActivated()
})

onDeactivated(() => {
  // 契约1/2：document 级监听与观察器全部断开（Events.On 不断，见上方契约3注记）
  unbindDocumentListeners()
  viewportRO?.disconnect()
  viewportRO = null
  cardsIO?.disconnect()
  cardsIO = null
})

// 契约5：onUnmounted 清理保留——缓存修剪/应用卸载（include 变化或整页 reload 触
// 达）时的最终清理，保活期不触达
onUnmounted(() => {
  if (offResponse) offResponse()
  if (offEvent) offEvent()
  // 断开观察与监听，防卸载后回调触达已失效节点
  viewportRO?.disconnect()
  cardsIO?.disconnect()
  unbindDocumentListeners()
})
</script>

<style scoped>
/* 工作台式骨架：宿主推荐边距惯例（10px），高度链自 BaseView 闭合——页级滚动归零，
   各分区卡内部滚动 */
.li-ws {
  width: calc(100% - 20px);
  height: calc(100% - 20px);
  margin: 10px;
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* —— 头部 —— */

.li-ws-header {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.li-ws-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ws-root {
  font-size: 13px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-ws-time {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-ws-header-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 12px;
}



/* 声明来源规则名直达编辑链接（待裁决候选/终选声明的来源小字内） */
.li-ws-rule-link {
  color: var(--app-color-primary);
  text-decoration: underline dotted;
}

.li-ws-banner {
  border-radius: var(--app-radius);
}

.li-ws-back {
  font-size: 13px;
  color: var(--app-color-primary);
}

/* 资源清单卡（唯一左卡）：flex 填满主体高度，卡内分区各自伸缩 */
.li-ws-card {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 12px 14px;
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.li-ws-card-title {
  margin: 0;
  font-size: 15px;
  color: var(--app-text-primary);
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

/* 卡头右簇：创建任务/开始全量识别入口（已选计数在左、动作按钮在右） */


/* H2：识别统计并入卡片标题行的小字串 */
.li-ws-card-stats {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-ws-desc {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--app-text-regular);
}

.li-ws-empty {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-secondary);
}

/* 未识别会话（无快照）卡体：引导说明 + 空态占位 */
.li-ws-nosnap {
  flex: 1 1 auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* —— 分页载入态（剧本1·帧1/帧2）：清单卡体专用载入/中断占位 —— */

.li-ws-pano-load {
  flex: 1 1 auto;
  min-height: 180px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 12px;
  padding: 0 10px;
}

.li-ws-pano-load-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
  /* 2026-11-07 用户反馈：载入态文案在卡内水平居中（垂直居中由容器 justify-content 承担） */
  text-align: center;
}

.li-ws-pano-load-title.is-failed {
  color: var(--app-color-danger, #f56c6c);
}

.li-ws-pano-load-count {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  display: flex;
  align-items: baseline;
  gap: 14px;
  flex-wrap: wrap;
  justify-content: center;
}

/* —— 创建异步横幅（剧本1·帧1/帧2/帧3→帧4）：卡底双行双计数+速率/ETA+取消入口 —— */

.li-ws-create-progress {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 转场行（「正在清理在途任务…」在 progress-head 之外，自带字号与配色） */
.li-ws-create-progress .li-ws-progress-phase {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-regular);
}

/* 核对行速率/ETA 行与取消×完成竞态附注（小字次级色） */
.li-ws-progress-meta,
.li-ws-progress-note {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-ws-cancel-create {
  align-self: flex-start;
}

/* 帧3 取消确认对话框正文 */
.li-ws-cancel-dialog-text {
  margin: 0;
  font-size: 13px;
  line-height: 1.7;
  color: var(--app-text-regular);
}

/* —— 识别中状态条（清单卡体内，无快照/增量重识别共用） —— */

.li-ws-progress {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-ws-progress-head {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--app-text-regular);
}

.li-ws-progress-count {
  color: var(--app-text-secondary);
}

.li-ws-progress-track {
  height: 6px;
  border-radius: 3px;
  background: var(--app-border-color-light);
  overflow: hidden;
}

.li-ws-progress-bar {
  height: 100%;
  border-radius: 3px;
  background: var(--app-color-primary);
  transition: width 0.2s ease;
}

/* —— 区3 重识别差异行（H5：单行小字，无卡片外壳） —— */

.li-ws-diffline {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-ws-diff-chip {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
}

.li-ws-diff-chip.is-changed {
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-ws-diff-chip.is-added {
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-ws-diff-chip.is-removed {
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

/* —— 双栏主从主体（视口锁定）：左卡 flex 自适应（min-width:0），右卡固定 400px；
      stretch 令双卡等高填满，各自卡内滚动 —— */

.li-ws-main {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  gap: 10px;
  align-items: stretch;
}

.li-ws-left {
  flex: 1 1 auto;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

/* —— 过滤与组合选择（H3：合并单行，窄窗自然折行） —— */

.li-ws-combos {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.li-ws-filter-btn {
  font-size: 12px;
  padding: 3px 12px;
  border-radius: var(--app-radius-sm);
  border: 1px solid var(--app-border-color-light);
  background: var(--app-bg-surface);
  color: var(--app-text-regular);
  cursor: pointer;
}

.li-ws-filter-btn.is-active {
  border-color: var(--app-color-primary);
  color: var(--app-color-primary);
}

.li-ws-combo-select {
  font-size: 12px;
  padding: 3px 8px;
  border-radius: var(--app-radius-sm);
  border: 1px solid var(--app-border-color-light);
  background: var(--app-bg-surface);
  color: var(--app-text-regular);
  max-width: 240px;
}

/* —— 平铺虚拟清单（滚动承载=el-scrollbar 宿主样式条；高度由卡内 flex 链分配） —— */

.li-ws-list {
  flex: 1 1 auto;
  min-height: 0;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
}

/* 虚拟滚动画布：总高=行数×行高撑起滚动区，行绝对定位在画布内按序号平移 */
/* 虚拟化占位条（头/尾各一）：高度内联由 visibleRange 派生，仅撑高度不承载内容 */
.li-ws-list-spacer {
  width: 100%;
}

.li-ws-row {
  /* 常规流行（2026-10-27 滚轮缺陷修复：绝对定位行会切断 WebView2 合成器滚轮命中链，
     改由头/尾静态占位条撑起虚拟化高度；行高固定 30px 与 ROW_H 对齐） */
  height: 30px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 8px;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  cursor: pointer;
}

.li-ws-row:hover {
  background: rgba(128, 128, 128, 0.08);
}

/* 选中态（主从联动的可视反馈：左侧主色描边；差异底色优先级更高，选中行靠描边区分） */
.li-ws-row.is-selected {
  box-shadow: inset 2px 0 0 var(--app-color-primary);
  background: var(--app-fill-color);
}

.li-ws-cb {
  flex: none;
  height: auto;
  margin-right: 0;
}

.li-ws-row-state {
  flex: none;
  width: 14px;
  text-align: center;
}

.li-ws-row-state.is-identified {
  color: var(--app-color-success);
}

.li-ws-row-state.is-clueful {
  color: var(--app-color-warning);
}

.li-ws-row-state.is-plain {
  color: var(--app-text-secondary);
}

.li-ws-row-path {
  flex: none;
  max-width: 34%;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--app-text-primary);
}

/* 重识别差异高亮：变更/新增行底色标记（消失行不在当前清单，经差异条计数呈现） */
.li-ws-row.is-diff-changed {
  background: var(--app-tag-warning-bg);
}

.li-ws-row.is-diff-added {
  background: var(--app-tag-success-bg);
}

.li-ws-row.is-diff-removed {
  background: var(--app-tag-danger-bg);
}

.li-ws-row-diff {
  flex: none;
  font-size: 11px;
  padding: 0 6px;
  border-radius: var(--app-radius-sm);
}

.li-ws-row-diff.is-changed {
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-ws-row-diff.is-added {
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-ws-row-diff.is-removed {
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-ws-row-meta {
  display: flex;
  gap: 6px;
  overflow: hidden;
}

.li-ws-row-decl {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-text-regular);
}

.li-ws-row-clue {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-ws-row-pending {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-ws-row-assigned {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

/* —— 收纳分组折叠头（候选卡墙已归集/已忽略分仓） —— */

.li-ws-fold-head {
  width: 100%;
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 8px 12px;
  border: none;
  background: transparent;
  font-size: 13px;
  color: var(--app-text-primary);
  cursor: pointer;
  text-align: left;
}

.li-ws-fold-arrow {
  display: inline-block;
  font-size: 11px;
  color: var(--app-text-secondary);
  transition: transform 0.15s ease;
}

.li-ws-fold-arrow.is-open {
  transform: rotate(90deg);
}

.li-ws-fold-title {
  font-weight: 600;
}

/* —— 清单模式切换（目录树 / 平铺清单 / 疑似作品集） —— */

.li-ws-mode-toggle {
  display: inline-flex;
  gap: 4px;
}

/* —— 区5 目录树模式（剧本F：全量渲染）；目录行复用旧目录结构区的含义覆盖标记样式 —— */

/* H4：体高与平铺清单同享卡内 flex 链 */
.li-ws-tree {
  flex: 1 1 auto;
  min-height: 0;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
}

.li-ws-tree :deep(.el-scrollbar__view) {
  padding: 4px 0;
  /* 行宽随最宽行展开：超宽行由横向滚动承载查看；窄内容时行背景仍铺满可视宽 */
  width: max-content;
  min-width: 100%;
  box-sizing: border-box;
}

.li-ws-tree-dir,
.li-ws-tree-file {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  padding: 2px 8px;
  white-space: nowrap;
  overflow: hidden;
}

.li-ws-tree-arrow {
  flex: none;
  width: 18px;
  padding: 0;
  border: none;
  background: transparent;
  font-size: 11px;
  color: var(--app-text-secondary);
  cursor: pointer;
  text-align: center;
  transition: transform 0.15s ease;
}

.li-ws-tree-arrow.is-open {
  transform: rotate(90deg);
}

.li-ws-tree-name {
  color: var(--app-text-primary);
  font-weight: 600;
  cursor: pointer;
}

.li-ws-tree-name:hover {
  color: var(--app-color-primary);
}

.li-ws-tree-file {
  height: 30px;
  box-sizing: border-box;
  cursor: pointer;
}

.li-ws-tree-file:hover {
  background: rgba(128, 128, 128, 0.08);
}

.li-ws-tree-file.is-selected {
  box-shadow: inset 2px 0 0 var(--app-color-primary);
  background: var(--app-fill-color);
}

.li-ws-tree-file.is-diff-changed {
  background: var(--app-tag-warning-bg);
}

.li-ws-tree-file.is-diff-added {
  background: var(--app-tag-success-bg);
}

.li-ws-tree-file.is-diff-removed {
  background: var(--app-tag-danger-bg);
}

.li-ws-tree-fname {
  flex: none;
  max-width: 30%;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--app-text-primary);
}

/* —— 右栏目录解析面板（剧本J：呈现主体在 ParsePanel 目录模式） —— */

.li-ws-dir-cb {
  margin-right: 0;
  height: auto;
}

.li-ws-dir-files {
  color: var(--app-text-secondary);
}

/* 含义胶囊与文件行声明胶囊同款（同为识别元数据提示，统一中性形）；
   未覆盖=该层无结论的缺席态，降为次要文字、不占胶囊形 */
.li-ws-dir-meaning {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-text-regular);
}

.li-ws-dir-uncovered {
  color: var(--app-text-secondary);
}

/* 区6 序列群样式已退役（剧本v2：折叠区并入「疑似作品集」第三标签候选卡墙，见下节） */

/* —— 剧本v2 疑似作品集候选卡墙（第三标签；区6 序列群折叠区能力并入） —— */

.li-ws-sets {
  flex: 1 1 auto;
  min-height: 0;
}

.li-ws-sets-inner {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px 10px;
}

/* 增量渲染哨兵（触底续批提示行；见底撤哨不占位） */
.li-ws-cands-more {
  padding: 2px 0;
  text-align: center;
  font-size: 12px;
  color: var(--app-text-secondary);
}



.li-ws-cand {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  background: var(--app-bg-color, transparent);
}

.li-ws-cand.is-confirming {
  border-color: var(--app-color-primary);
}

.li-ws-cand.is-done {
  opacity: 0.72;
}

.li-ws-cand-head {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  flex-wrap: wrap;
}

.li-ws-cand-key {
  font-weight: 600;
  color: var(--app-text-primary);
}

/* 剧本1：已归集卡键名=所在目录的目录解析面板入口（showDirCard(c.dir)，空目录键
   =库根目录的既有处理） */
.li-ws-cand-key.is-link {
  cursor: pointer;
}

.li-ws-cand-key.is-link:hover {
  color: var(--app-color-primary);
}

.li-ws-cand-dir {
  color: var(--app-text-secondary);
}

.li-ws-cand-size {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-text-regular);
}

.li-ws-cand-src {
  color: var(--app-text-secondary);
  font-size: 11px;
}

.li-ws-cand-cross {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: rgba(29, 122, 69, 0.14);
  color: #1d7a45;
  font-size: 11px;
}

.li-ws-cand-reason {
  margin: 0;
  color: var(--app-text-secondary);
  font-size: 12px;
}

.li-ws-cand-members {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

/* 成员预览截断说明（size > len(members) 时呈现：投影层截前 200 个成员，size=真总数） */
.li-ws-cand-preview {
  margin: 0;
  color: var(--app-text-secondary);
  font-size: 11px;
}

.li-ws-cand-member {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-fill-color);
  color: var(--app-text-regular);
  cursor: pointer;
  word-break: break-all;
}

.li-ws-cand-member.is-selected {
  box-shadow: inset 2px 0 0 var(--app-color-primary);
}

/* 元素级块底色（剧本步1′：作者括号=琥珀 / 作品号=蓝 / 序号=绿 / 群公共元素=
   浅灰；plain/sep 无底色；半透明覆盖面兼容明暗主题） */
.li-ws-seg-author {
  background: rgba(255, 193, 7, 0.35);
  border-radius: 3px;
}

.li-ws-seg-workId {
  background: rgba(64, 148, 250, 0.32);
  border-radius: 3px;
}

.li-ws-seg-seq {
  background: rgba(102, 187, 106, 0.36);
  border-radius: 3px;
}

.li-ws-seg-title {
  background: rgba(130, 130, 130, 0.22);
  border-radius: 3px;
}

.li-ws-cand-ext {
  opacity: 0.55;
}

.li-ws-cand-actions,
.li-ws-cand-confirm {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-ws-cand-name {
  /* 宽度由镜像量宽自适应（:style 绑定），此处兜底 */
  min-width: 96px;
  max-width: 360px;
  padding: 2px 24px 2px 8px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  background: var(--app-bg-color, transparent);
  color: var(--app-text-primary);
  font-size: 12px;
  font-family: inherit;
}

/* 用户反馈优化①：集名输入框包裹层（一键清除按钮定位 + 镜像量宽载体） */
.li-ws-cand-namewrap {
  position: relative;
  display: inline-flex;
  align-items: center;
}

.li-ws-cand-clear {
  position: absolute;
  right: 4px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: var(--app-text-secondary, #888);
  font-size: 10px;
  line-height: 1;
  cursor: pointer;
}

.li-ws-cand-clear:hover {
  color: var(--app-text-primary, #333);
  background: rgba(128, 128, 128, 0.15);
}

/* 镜像 span：与输入框同字体量宽，绝对定位移出视口不占位 */
.li-ws-cand-name-mirror {
  position: absolute;
  left: -9999px;
  top: 0;
  visibility: hidden;
  white-space: pre;
  font-size: 12px;
  font-family: inherit;
  padding: 2px 8px;
}


.li-ws-cand-name:focus {
  outline: none;
  border-color: var(--app-color-primary);
}

.li-ws-cand-existing {
  color: #1d7a45;
  font-size: 12px;
}

/* 剧本步2′：确认区元素插入 chips（决策2 A 池——键元素+目录名元素；括号组
   双态成对：带壳 chip 加壳色描边，点击插入集名输入框光标处） */
.li-ws-cand-chips {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  flex-basis: 100%;
}

.li-ws-chip {
  padding: 1px 8px;
  border: 1px solid var(--app-border-color-light);
  border-radius: 10px;
  background: var(--app-bg-color, transparent);
  color: var(--app-text-primary);
  font-size: 11px;
  cursor: pointer;
}

.li-ws-chip:hover:not(:disabled) {
  border-color: var(--app-color-primary);
}

.li-ws-chip.is-shelled {
  border-color: rgba(255, 193, 7, 0.6);
}

.li-ws-cand-bin {
  border-top: 1px dashed var(--app-border-color-light);
  padding-top: 6px;
  margin-top: 2px;
}

.li-ws-cand-binbody {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}

.li-ws-cand-done {
  color: #1d7a45;
  font-size: 12px;
  margin-left: auto;
}

/* 剧本2：已归集卡右上「撤销归集」按钮（已归集徽标 margin-left:auto 已推至行尾，
   按钮紧随其后即右上位；在途态文案切换见 undoConfirm） */
.li-ws-cand-undo {
  flex: none;
}

/* 剧本2：撤销归集失败提示（错误路径：卡片不动） */
.li-ws-cand-error {
  margin-bottom: 2px;
}

.li-ws-cand-restore {
  margin-left: auto;
}

/* 剧本C：已归集卡「泛化集规则」入口（撤销归集左侧；进入右栏集规则面板） */
.li-ws-cand-setrule {
  flex: none;
}

/* —— 待裁决项卡（融合弹窗内；文件名点按定位清单行，候选含规则直达链接） —— */

.li-ws-pending {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-ws-pending-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
}

.li-ws-pending-file {
  color: var(--app-text-primary);
  font-weight: 600;
  cursor: pointer;
  word-break: break-all;
}

.li-ws-pending-file.is-selected {
  color: var(--app-color-primary);
}

.li-ws-pending-dim {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
}

.li-ws-pending-cands {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.li-ws-pending-cand {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-fill-color);
  color: var(--app-text-regular);
}

.li-ws-pending-reason {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* —— 待裁决·决策态融合弹窗（决策计数 + 失效赋值 + 待裁决全量清单，弹窗内滚动） —— */

.li-ws-fusion {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.li-ws-fusion-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ws-fusion-sec {
  margin: 0;
  padding-top: 10px;
  border-top: 1px dashed var(--app-border-color-light);
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ws-fusion-note {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* —— 决策计数三格（融合弹窗内：已手动解析/已忽略/已选定为任务） —— */

.li-ws-decisions {
  display: flex;
  gap: 12px;
}

.li-ws-decision {
  flex: 1;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 10px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.li-ws-decision-count {
  font-size: 20px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ws-decision-label {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-ws-stale {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-ws-stale-title {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-warning);
}

.li-ws-stale-item {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
  word-break: break-all;
}

/* —— 已选计数（清单卡头，随选择集同步）与悬空选择提示（卡底反馈条） —— */

.li-ws-create-count {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ws-create-sub {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-ws-stale-hint {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-warning);
}

/* 剧本K 名称确认对话框：输入框下说明小字（append-to-body 后 scoped 属性仍随模板元素携带） */
.li-ws-create-dialog-note {
  margin: 8px 0 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

/* —— 右栏解析面板卡（高度由主体 stretch 链锁定，卡内滚动） —— */

.li-ws-right {
  flex: 0 0 400px;
  width: 400px;
  min-height: 0;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  display: flex;
  flex-direction: column;
}

/* 面板内容滚动体：宿主样式条内滚 */
.li-ws-panel-scroll {
  flex: 1 1 auto;
  min-height: 0;
}

.li-ws-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 12px;
  border-bottom: 1px solid var(--app-border-color-light);
}

.li-ws-panel-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ws-panel-btn {
  font-size: 12px;
  padding: 2px 10px;
  border-radius: var(--app-radius-sm);
  border: 1px solid var(--app-border-color-light);
  background: var(--app-bg-surface);
  color: var(--app-text-regular);
  cursor: pointer;
}

/* 未保存切换确认条（解析面板表单有未保存改动时切换文件/目录目标的轻量确认） */
.li-ws-switch-confirm {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 12px;
  border-bottom: 1px solid var(--app-border-color-light);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
  font-size: 12px;
}

.li-ws-switch-confirm-actions {
  display: flex;
  gap: 6px;
}

.li-ws-switch-confirm .li-ws-panel-btn.is-primary {
  border-color: var(--app-color-primary);
  color: var(--app-color-primary);
}

.li-ws-panel-body {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.li-ws-panel-guide {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 24px 8px;
}

.li-ws-panel-guide-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-secondary);
  text-align: center;
}

.li-ws-panel-guide-text {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-text-secondary);
  text-align: center;
}

.li-ws-panel-missing {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-start;
}

.li-ws-panel-missing-text {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-warning);
}

.li-ws-panel-path {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-primary);
  font-weight: 600;
  word-break: break-all;
}

.li-ws-file-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-ws-file-state {
  flex: none;
  font-size: 12px;
  color: var(--app-text-secondary);
  margin-left: auto;
}

.li-ws-sec {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-ws-sec-title {
  margin: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-secondary);
}

.li-ws-sec-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-ws-sec-line {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-ws-sec-note {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-ws-decl {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 4px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
}

.li-ws-decl-main {
  font-size: 12px;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-ws-decl-src {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 11px;
  color: var(--app-text-secondary);
}

.li-ws-decl-src-text {
  word-break: break-all;
}

/* 同值折叠标签：终选声明来源行尾 [同值 +N]（展开态 [同值 +N ▴]），点击展开/收起 */
.li-ws-same-toggle {
  margin-left: auto;
  flex: none;
  font-size: 11px;
  line-height: 1.5;
  padding: 0 6px;
  border: none;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-color-primary);
  cursor: pointer;
}

/* 同值展开列表：所属声明行下的同值规则名链接（逐条直达规则编辑） */
.li-ws-same-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-left: 12px;
}

.li-ws-same-item {
  margin: 0;
  font-size: 11px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

/* 命中规则区：提取段命中规则的名称胶囊（逐条直达规则编辑） */
.li-ws-hit-rules {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.li-ws-hit-pill {
  font-size: 11px;
  line-height: 1.5;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-color-primary);
  text-decoration: none;
  word-break: break-all;
}

.li-ws-hit-pill:hover {
  text-decoration: underline dotted;
}

.li-ws-clue {
  font-size: 12px;
  padding: 2px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
  word-break: break-all;
}
</style>
