<template>
  <div class="li-ws">
    <!-- 头部：解析工作台 · 库根 · 识别于 · 重新识别 · 规则管理/解析观测子页链接 -->
    <header class="li-ws-header">
      <h2 class="li-ws-title">解析工作台</h2>
      <span v-if="session" class="li-ws-root">{{ session.rootPath }}</span>
      <span v-if="snapshot" class="li-ws-time">识别于 {{ formatTime(snapshot.generatedAtMs) }}</span>
      <span class="li-ws-header-actions">
        <el-button
          v-if="snapshot"
          size="small"
          :loading="starting"
          @click="startRecognition"
        >重新识别</el-button>
        <a class="li-ws-header-link" :href="subRoute('workbench-rules')">规则管理</a>
        <a class="li-ws-header-link" :href="subRoute('workbench-observation')">解析观测</a>
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
      description="既有识别结论可能与最新规则不一致，建议重新识别。"
    />

    <!-- 空会话态 -->
    <el-empty
      v-if="!session && !loadError"
      description="未找到进行中的解析会话：请从工作台新建或恢复会话"
    >
      <a class="li-ws-back" :href="`${ROUTE_BASE}workbench`">返回本地导入工作台</a>
    </el-empty>

    <template v-if="session">
      <!-- 边界：未识别会话（无快照）→ 开始全量识别入口（干跑 = 纯求值，零任务产出、零弹问） -->
      <section v-if="!snapshot && !recognizing" class="li-ws-card">
        <p class="li-ws-desc">
          对库根做全量识别干跑：按「规则档案 ∪ 会话赋值」合并求值全部文件，产出
          识别全景（目录结构含义、已识别资源、有价值未识别、序列群、待裁决项、
          规则命中与决策态）。识别不创建任何任务。
        </p>
        <el-button type="primary" :loading="starting" @click="startRecognition">
          开始全量识别
        </el-button>
      </section>

      <!-- 识别中：流式进度（阶段跃迁 + 计数）；赋值变更后的自动增量重识别共用此状态条 -->
      <section v-if="recognizing" class="li-ws-card">
        <div class="li-ws-progress-head">
          <span class="li-ws-progress-phase">{{ phaseLabel(progress.phase) }}</span>
          <span v-if="progress.total > 0" class="li-ws-progress-count">{{ progress.done }} / {{ progress.total }}</span>
        </div>
        <div class="li-ws-progress-track">
          <div class="li-ws-progress-bar" :style="{ width: progressPercent }"></div>
        </div>
      </section>

      <!-- 识别失败提示 -->
      <el-alert
        v-if="recognError"
        class="li-ws-banner"
        type="error"
        :closable="false"
        show-icon
        :title="recognError"
      />

      <template v-if="snapshot">
        <!-- 区3 重识别差异行（H5：单行小字，验证轮提示语并入同行，不占独立卡片） -->
        <p v-if="diff && diffTotal > 0" class="li-ws-diffline">
          重识别差异（生成规则后的验证轮）：
          <span class="li-ws-diff-chip is-changed">变更 {{ diff.changed ? diff.changed.length : 0 }}</span>
          <span class="li-ws-diff-chip is-added">新增 {{ diff.added ? diff.added.length : 0 }}</span>
          <span class="li-ws-diff-chip is-removed">消失 {{ diff.removed ? diff.removed.length : 0 }}</span>
          <span>过窄＝差异里仍有未覆盖、过泛＝差异里错套——请在清单核对标色行</span>
        </p>

        <!-- 双栏主从主体：左栏自适应（保底 ~480px），右栏固定 ~400px 可收起 -->
        <section class="li-ws-main">
          <div class="li-ws-left">
            <!-- 区5 资源清单：统计并入标题行（H2）+ 过滤与组合操作单行（H3）+ 虚拟滚动/树体（H4 体高自适应） -->
            <section class="li-ws-card">
              <h3 class="li-ws-card-title">
                资源清单
                <!-- 剧本F F1：清单浏览模式切换「▦ 清单 / ⛁ 目录树」，默认目录树（决策2），会话内记忆不持久化 -->
                <span class="li-ws-mode-toggle">
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
                  <!-- 剧本v2 第三标签：疑似作品集候选卡墙（目录候选/序列群/A 档库内集比对合流；
                       原「序列群」折叠区能力并入此处退役） -->
                  <button
                    type="button"
                    class="li-ws-filter-btn li-ws-mode-btn"
                    :class="{ 'is-active': listMode === 'sets' }"
                    title="候选卡墙：目录候选 / 序列群 / 库内集比对（确认归集或忽略）"
                    @click="listMode = 'sets'"
                  >疑似作品集</button>
                </span>
                <!-- H2：识别统计随快照刷新；增量跳过目录>0 时串内追加 -->
                <span class="li-ws-card-stats">文件 {{ snapshot.stats.totalFiles }} · 已识别 {{ snapshot.stats.identified }} · 有线索 {{ snapshot.stats.clueful }} · 无线索 {{ snapshot.stats.plain }} · 序列群 {{ snapshot.stats.groupCount }} · 待裁决 {{ snapshot.stats.pendingFiles }}<template v-if="snapshot.stats.skippedDirs > 0"> · 增量跳过目录 {{ snapshot.stats.skippedDirs }}</template></span>
              </h3>
              <!-- H3：过滤器与批量/组合操作合并单行（flex-wrap，窄窗自然折行） -->
              <div class="li-ws-combos">
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
                  <option v-for="r in ruleOptions" :key="r.id" :value="r.id">{{ r.id }}（命中 {{ r.hitCount }}）</option>
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
              <!-- 平铺模式（虚拟滚动切片渲染；H4：体高随视口剩余空间自适应，切片窗口随容器尺寸持续同步；
                    模式切换时过滤与选择集保持） -->
              <div
                v-show="listMode === 'flat'"
                ref="listViewport"
                class="li-ws-list"
                :style="{ height: `${listBodyH}px` }"
                @scroll="onListScroll"
              >
                <div class="li-ws-list-spacer" :style="{ height: `${filteredFiles.length * ROW_H}px` }"></div>
                <!-- 行 = 选择复选（作品级）+ 状态点 + 路径 + 声明/线索摘要 + 差异标 + 已赋值标；
                     行点击 = 选中联动右栏（选中键 = 文件相对路径），与复选不互斥 -->
                <div
                  v-for="row in visibleFiles"
                  :key="row.relPath"
                  class="li-ws-row"
                  :class="row.relPath === selectedRelPath ? 'is-selected' : (diffMark(row.relPath) ? `is-diff-${diffMark(row.relPath)}` : '')"
                  :style="{ transform: `translateY(${row.index * ROW_H}px)` }"
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
                <p v-if="filteredFiles.length === 0" class="li-ws-empty">当前过滤条件下无文件</p>
              </div>
              <!-- 树模式（剧本F F1/F2：库根 → 目录 → 文件全量渲染，246 行级数据量不虚拟化；H4：体高与平铺同享自适应）；
                   目录行 = 展开箭头 + 复选框（子树递选，按作品聚合进现有选择集）+ 目录名（点 = 右栏目录解析面板，剧本J）
                   + 文件数 + 含义覆盖标记（复用旧「目录结构」折叠区数据源）；
                   文件行 = 行首作品级复选（H6：勾选/禁用语义同平铺，目录行全选/半选态由选择集派生）+ 平铺同款元数据，点行 = 现有联动 -->
              <div
                v-show="listMode === 'tree'"
                ref="treeViewport"
                class="li-ws-tree"
                :style="{ height: `${listBodyH}px` }"
              >
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
              </div>
              <!-- 剧本v2 疑似作品集候选卡墙（第三标签体）：三类候选合流出卡——目录候选（整目录像一集）/
                   序列群（结构/规则/页级）/A 档库内集比对（集名互证徽标 + 作品号命中→归入既有集）；
                   成员文件名序列块底色（区间=后端形态表单一事实源）；两段式归集（预填可改）；
                   已归集/已忽略收纳分组（会话内，忽略可恢复）；确认走集维度批量赋值+既有建议卡通路 -->
              <div v-show="listMode === 'sets'" class="li-ws-sets" :style="{ height: `${listBodyH}px` }">
                <p
                  v-if="candidates.length === 0 && !(snapshot.ignoredCandidates || []).length && !(snapshot.confirmedCandidates || []).length"
                  class="li-ws-empty"
                >无疑似作品集候选</p>
                <div
                  v-for="c in candidates"
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
                      v-for="m in c.members"
                      :key="m.relPath"
                      class="li-ws-cand-member"
                      :class="{ 'is-selected': m.relPath === selectedRelPath }"
                      :title="m.relPath"
                      @click="selectFile(m.relPath)"
                    ><template v-if="m.seqText"><span class="li-ws-seg-seq">#{{ m.seqText }} </span></template><span
                        v-for="(s, si) in elSpans(fileNameOf(m.relPath), m.elements).spans"
                        :key="si"
                        :class="s.kind ? `li-ws-seg-${s.kind}` : ''"
                      >{{ s.text }}</span><span class="li-ws-cand-ext">{{ elSpans(fileNameOf(m.relPath), m.elements).ext }}</span></span>
                  </div>
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
                          v-for="m in c.members"
                          :key="m.relPath"
                          class="li-ws-cand-member"
                          :class="{ 'is-selected': m.relPath === selectedRelPath }"
                          :title="m.relPath"
                          @click="selectFile(m.relPath)"
                        ><template v-if="m.seqText"><span class="li-ws-seg-seq">#{{ m.seqText }} </span></template><span
                          v-for="(s, si) in elSpans(fileNameOf(m.relPath), m.elements).spans"
                          :key="si"
                          :class="s.kind ? `li-ws-seg-${s.kind}` : ''"
                        >{{ s.text }}</span><span class="li-ws-cand-ext">{{ elSpans(fileNameOf(m.relPath), m.elements).ext }}</span></span>
                      </div>
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
            </section>

            <!-- 区4 目录结构折叠区已退役（剧本F：层含义/递选/覆盖标记能力已被清单树模式吸收） -->
            <!-- 区6 序列群折叠区已退役（剧本v2：能力并入「疑似作品集」第三标签候选卡墙——群全量出卡，动作与收纳在卡墙） -->

            <!-- 区7 待裁决项折叠区（合成层声明冲突全量清单；单文件就地裁决在右栏·阶段3接入） -->
            <section class="li-ws-fold">
              <button type="button" class="li-ws-fold-head" :aria-expanded="!folded.pending" @click="folded.pending = !folded.pending">
                <span class="li-ws-fold-arrow" :class="{ 'is-open': !folded.pending }">▸</span>
                <span class="li-ws-fold-title">待裁决项</span>
                <span class="li-ws-fold-note">{{ snapshot.pending ? snapshot.pending.length : 0 }} 项</span>
              </button>
              <div v-show="!folded.pending" class="li-ws-fold-body">
                <p v-if="!snapshot.pending || snapshot.pending.length === 0" class="li-ws-empty">无声明冲突</p>
                <div v-for="(p, i) in snapshot.pending || []" :key="i" class="li-ws-pending">
                  <div class="li-ws-pending-head">
                    <span
                      class="li-ws-pending-file"
                      :class="{ 'is-selected': p.relPath === selectedRelPath }"
                      @click="selectFile(p.relPath)"
                    >{{ p.relPath }}</span>
                    <span class="li-ws-pending-dim">{{ dimLabel(p.dimension) }}</span>
                  </div>
                  <div class="li-ws-pending-cands">
                    <span v-for="(c, j) in p.candidates || []" :key="j" class="li-ws-pending-cand">
                      {{ dimLabel(c.dimension) }}={{ c.value }}（{{ sourceLabel(c.source) }}<template v-if="c.ruleId"> · <a class="li-ws-rule-link" :href="ruleEditRoute(c.ruleId)" :title="`跳规则管理子页并打开「${c.ruleId}」编辑`">{{ c.ruleId }}</a></template>）
                    </span>
                  </div>
                  <p class="li-ws-pending-reason">{{ p.reason }}</p>
                </div>
              </div>
            </section>

            <!-- 区8 规则档案态折叠区（来源/启用态/命中数） -->
            <section class="li-ws-fold">
              <button type="button" class="li-ws-fold-head" :aria-expanded="!folded.rules" @click="folded.rules = !folded.rules">
                <span class="li-ws-fold-arrow" :class="{ 'is-open': !folded.rules }">▸</span>
                <span class="li-ws-fold-title">规则档案态</span>
                <span class="li-ws-fold-note">{{ (snapshot.ruleStats || []).length }} 条</span>
              </button>
              <div v-show="!folded.rules" class="li-ws-fold-body">
                <div class="li-ws-rules">
                  <div class="li-ws-rule-row li-ws-rule-head">
                    <span class="li-ws-rule-id">规则</span>
                    <span class="li-ws-rule-src">来源</span>
                    <span class="li-ws-rule-enabled">启用</span>
                    <span class="li-ws-rule-hits">命中</span>
                  </div>
                  <div v-for="r in snapshot.ruleStats || []" :key="r.id" class="li-ws-rule-row" :title="r.description">
                    <span class="li-ws-rule-id">{{ r.id }}</span>
                    <span class="li-ws-rule-src">{{ sourceLabel(r.source) }}</span>
                    <span class="li-ws-rule-enabled" :class="{ 'is-off': !r.enabled }">{{ r.enabled ? '启用' : '停用' }}</span>
                    <span class="li-ws-rule-hits">{{ r.hitCount }}</span>
                  </div>
                </div>
              </div>
            </section>

            <!-- 区9 会话决策态折叠区（赋值/忽略/选择集计数 + 失效赋值提示） -->
            <section class="li-ws-fold">
              <button type="button" class="li-ws-fold-head" :aria-expanded="!folded.decisions" @click="folded.decisions = !folded.decisions">
                <span class="li-ws-fold-arrow" :class="{ 'is-open': !folded.decisions }">▸</span>
                <span class="li-ws-fold-title">会话决策态</span>
                <span class="li-ws-fold-note">
                  已解析 {{ decisions && decisions.assignments ? decisions.assignments.length : 0 }} · 已忽略 {{ decisions && decisions.ignored ? decisions.ignored.length : 0 }} · 已选 {{ selectedWorks.size }}
                  <template v-if="snapshot.staleAssignments && snapshot.staleAssignments.length"> · 失效赋值 {{ snapshot.staleAssignments.length }}</template>
                </span>
              </button>
              <div v-show="!folded.decisions" class="li-ws-fold-body">
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
                <p class="li-ws-empty">手动解析（单次赋值/泛化/裁决）在右栏解析面板操作（点清单行进入）；资源选择与任务创建用下方创建栏（创建后选择集自动清理）。</p>
                <div v-if="snapshot.staleAssignments && snapshot.staleAssignments.length" class="li-ws-stale">
                  <p class="li-ws-stale-title">失效赋值（引用的路径已不存在）：</p>
                  <span v-for="s in snapshot.staleAssignments" :key="s" class="li-ws-stale-item">{{ s }}</span>
                </div>
              </div>
            </section>

            <!-- 区2 选择创建栏：作品级原子选择集 → 创建任务（剧本K：点按钮先弹名称确认
                 对话框，确认后才走创建链路；含对账/创建结果横幅与悬空对账提示） -->
            <section class="li-ws-card">
              <div class="li-ws-createbar">
                <span class="li-ws-create-count">已选 {{ selectedWorks.size }} 件作品<span class="li-ws-create-sub">（可选 {{ selectableWorks.length }} 件 / 共 {{ allWorks.length }} 件）</span></span>
                <el-button
                  type="primary"
                  size="small"
                  :loading="creating"
                  :disabled="selectedWorks.size === 0"
                  @click="openCreateDialog"
                >创建任务</el-button>
                <span class="li-ws-create-note">创建前经磁盘对账核验；创建后任务为 Created 状态，由任务管理手动开始</span>
              </div>
              <el-alert
                v-if="createError"
                class="li-ws-banner"
                type="error"
                :closable="false"
                show-icon
                :title="createError"
              />
              <el-alert
                v-else-if="createSummary"
                class="li-ws-banner"
                type="success"
                :closable="false"
                show-icon
                :title="createSummary"
              />
              <p v-if="danglingSelection > 0" class="li-ws-stale-hint">
                选择集中 {{ danglingSelection }} 项已不在当前识别全景内（可能已重识别），创建前请刷新重选
              </p>
            </section>
          </div>

          <!-- 右栏解析面板（固定 ~400px、可收起）：随选中联动显示文件路径与状态，
               赋值表单/泛化/裁决完整交互见 ParsePanel.vue（阶段3接入）；目录解析
               面板（阶段4 剧本J：点树中目录名进入）亦由 ParsePanel 目录模式承载 -->
          <aside v-if="!rightCollapsed" class="li-ws-right">
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
                :pending-items="[]"
                :diff="diff"
                :recognizing="recognizing"
                :request="request"
                @refresh="onPanelRefresh"
                @exit-dir="exitDirPanel"
                @open-rule="openRuleEdit"
              />
              <!-- 边界：右栏空态引导 -->
              <div v-else-if="!selectedRelPath" class="li-ws-panel-guide">
                <p class="li-ws-panel-guide-title">未选中文件</p>
                <p class="li-ws-panel-guide-text">
                  点击左栏清单中任意文件行，此处将呈现该文件的解析面板：
                  终选声明、线索、所属序列群与赋值表单。
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
                  <h4 class="li-ws-sec-title">终选声明</h4>
                  <template v-if="selectedFile.final && selectedFile.final.length">
                    <div v-for="(d, i) in selectedFile.final" :key="i" class="li-ws-decl">
                      <span class="li-ws-decl-main">{{ declLabel(d) }}</span>
                      <span class="li-ws-decl-src">{{ sourceLabel(d.source) }}<template v-if="d.ruleId"> · <a class="li-ws-rule-link" :href="ruleEditRoute(d.ruleId)" :title="`跳规则管理子页并打开「${d.ruleId}」编辑`">{{ d.ruleId }}</a></template></span>
                    </div>
                  </template>
                  <p v-else class="li-ws-empty">无终选声明</p>
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
                  :pending-items="selectedPending"
                  :diff="diff"
                  :recognizing="recognizing"
                  :request="request"
                  @refresh="onPanelRefresh"
                  @open-rule="openRuleEdit"
                />
              </template>
            </div>
          </aside>
        </section>
      </template>
    </template>

    <!-- 剧本K 创建任务名称确认对话框：任务名预填默认名（空=用默认，后端同规则兜底），
         确认「创建」才走既有创建链路；「取消」= 关闭弹窗、无副作用 -->
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
        一次创建为一个父任务，包含全部 {{ selectedWorks.size }} 件作品（{{ selectedFileCount }} 个文件）；识别出站点归属的文件按站点分组为额外父行（罕见）
      </p>
      <template #footer>
        <el-button @click="createDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="createTasks">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { Events } from '@wailsio/runtime'
import ParsePanel from './ParsePanel.vue'
import SetRulePanel from './SetRulePanel.vue'

// 双栏主从解析工作区（识别全景 × 手动解析合一）：
// 左栏 = 清单全景（过滤器 + 资源清单 + 目录树/序列群/待裁决/规则态/决策态折叠区 + 选择创建栏），
// 右栏 = 解析面板（本阶段占位：点行联动显示文件路径与状态；赋值/泛化/裁决交互属阶段3）。
// 九区呈现场自 SubpagePan.vue 逐区迁入，事件协议零改动（request/response/event 三通道原样）。

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
  domain?: WorkDomainView
  created?: number
  createMsg?: string
}

interface WorkbenchEvent {
  kind: string
  root?: string
  phase?: string
  done?: number
  total?: number
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
// 右栏收起态与折叠区开合态：默认右栏展开、折叠区全收起（目录结构区已退役，能力并入清单树模式）
const rightCollapsed = ref(false)
const folded = ref({ pending: true, rules: true, decisions: true })

// —— 清单浏览模式（剧本F：平铺 ↔ 目录树；默认=目录树〔决策2〕，会话内记忆不持久化。
// 剧本v2 增第三标签 sets=疑似作品集候选卡墙） ——
const listMode = ref<'tree' | 'flat' | 'sets'>('tree')

// —— 剧本v2 疑似作品集候选卡墙（第三标签体） ——
// 候选/两分仓清单来自快照（后端 buildSnapshotCandidates 三类合流）；动作走本组件唯一
// workbench 请求通道，应答先回、分仓随自动重识别的快照事件刷新（既有事件通路联动）
const candidates = computed<SnapCandidate[]>(() => (snapshot.value && snapshot.value.candidates) || [])
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

// 群卡定群来源标注：结构形态群=「结构·形态名」（形态表命中形态透出），规则定群
// 群=定群规则 ID，兜底「结构」（群卡列表与右栏「所属序列群」共用）
const groupBadge = (g: SnapGroup): string => {
  if (g.formName) return `结构·${g.formName}`
  return g.ruleId || '结构'
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
const createSummary = ref('')
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

// 带序号配对的请求（识别全景载荷较大，超时放宽到 30 秒）
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
    }, 30000)
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

// 装载全景（两段拉取：概览先渲染，文件级清单大库单独分节拉取）；
// 重装载后选中键不重置——selectedRelPath 凭相对路径跨快照保持
async function loadPanorama(root: string): Promise<void> {
  const summary = await request('getPanorama', { root: root })
  if (!summary.ok || !summary.panorama) {
    // 未识别（created）：保留空态呈现识别入口
    if (summary.error && summary.error.indexOf('尚未完成') < 0) {
      loadError.value = summary.error
    }
    return
  }
  snapshot.value = summary.panorama
  decisions.value = summary.decisions || null
  diff.value = summary.diff || null
  // 选择域（唯一结构源）与会话选择集（作品键）随全景回读
  domain.value = summary.domain || null
  selectedWorks.value = new Set((summary.decisions && summary.decisions.selection) || [])
  const files = await request('getPanorama', { root: root, section: 'files' })
  if (files.ok && files.panorama) {
    const base = snapshot.value
    if (base) {
      snapshot.value = {
        ...base,
        files: files.panorama.files,
        groups: files.panorama.groups,
        pending: files.panorama.pending
      }
    }
    if (files.diff) diff.value = files.diff
  }
  // 自动刷新后选中行滚回可视区（文件仍在当前全景时）
  await nextTick()
  scrollSelectedIntoView()
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
  progress.value = { phase: 'scan', done: 0, total: 0 }
}

// 进度与终态事件（识别事件外的其他工作台事件忽略）：
// recognitionDone → 重取全景与决策态（赋值变更后的自动增量重识别也经此通路刷新）
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
  }
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

const listViewport = ref<HTMLDivElement | null>(null)
const treeViewport = ref<HTMLDivElement | null>(null)
const scrollTop = ref(0)
const viewportH = ref(420)
// H4：清单/树体高度（视口剩余空间实测，保底 240px；初值 420 供首帧渲染，挂载后同步）
const listBodyH = ref(420)

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

const visibleFiles = computed(() => {
  const start = Math.max(0, Math.floor(scrollTop.value / ROW_H) - 8)
  const end = Math.min(filteredFiles.value.length, start + Math.ceil(viewportH.value / ROW_H) + 16)
  return filteredFiles.value.slice(start, end).map((f, i) => ({ ...f, index: start + i }))
})

function onListScroll(e: Event): void {
  const target = e.target as HTMLDivElement
  scrollTop.value = target.scrollTop
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

// 自动刷新（重识别）后清单虚拟滚动重排：选中行滚回可视区
function scrollSelectedIntoView(): void {
  if (!selectedRelPath.value || !listViewport.value) return
  const idx = filteredFiles.value.findIndex((f) => f.relPath === selectedRelPath.value)
  if (idx < 0) return
  const top = idx * ROW_H
  const view = listViewport.value
  const bottomEdge = view.scrollTop + view.clientHeight
  if (top < view.scrollTop || top + ROW_H > bottomEdge) {
    view.scrollTop = Math.max(0, top - Math.floor(view.clientHeight / 2))
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

const selectedCountOf = (works: SelectableWorkView[]): number => works.filter((w) => selectedWorks.value.has(w.workId)).length
const selectableCountOf = (works: SelectableWorkView[]): number => works.filter((w) => w.selectable).length

const dirTreeChecked = (relKey: string): boolean => {
  const works = worksUnderDirTree(relKey)
  const selectable = selectableCountOf(works)
  return selectable > 0 && selectedCountOf(works) === selectable
}

const dirTreeIndeterminate = (relKey: string): boolean => {
  const works = worksUnderDirTree(relKey)
  const count = selectedCountOf(works)
  return count > 0 && count < selectableCountOf(works)
}

const dirTreeSelectedText = (relKey: string): string => {
  const works = worksUnderDirTree(relKey)
  return `${selectedCountOf(works)}/${works.length}`
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
    const cut = node.relKey.lastIndexOf('/')
    const parent = cut >= 0 ? nodeByRel.get(node.relKey.slice(0, cut)) : undefined
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

// 树可见行序列：展开目录的内容行（子目录在前、文件在后）；全量渲染不虚拟化（246 行级数据量）
const treeRows = computed<TreeRow[]>(() => {
  const rows: TreeRow[] = []
  for (const f of rootLevelFiles.value) rows.push({ kind: 'file', key: `f:${f.relPath}`, depth: 0, file: f })
  const emit = (node: TreeDirNode): void => {
    rows.push({ kind: 'dir', key: `d:${node.relKey}`, depth: node.depth, dir: node })
    if (treeExpanded.value.has(node.relKey)) {
      for (const child of node.children) emit(child)
      for (const f of node.files) rows.push({ kind: 'file', key: `f:${f.relPath}`, depth: node.depth + 1, file: f })
    }
  }
  for (const root of dirRoots.value) emit(root)
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

// —— 创建任务：选择集整集落库 → 创建编排（对账→CreateTask→清理） —— //

// 剧本K2：确认「创建」→ 弹窗关闭、创建按钮转 loading、走既有链路；taskName 随输入
// 原样上送（清空/全空白由后端默认名兜底）；失败态（K2b）弹窗已关、红横幅语义与
// 现状一致、选择集保留可重试
async function createTasks(): Promise<void> {
  createDialogVisible.value = false
  if (!session.value) return
  if (selectedWorks.value.size === 0) {
    createError.value = '未选择任何作品：请在清单勾选要导入的作品后再创建'
    return
  }
  creating.value = true
  createError.value = ''
  createSummary.value = ''
  try {
    const sel = await request('setSelection', { root: session.value.rootPath, works: [...selectedWorks.value] })
    if (!sel.ok) {
      createError.value = sel.error || '选择集写入失败'
      return
    }
    if (sel.decisions) decisions.value = sel.decisions
    const resp = await request('createTasks', { root: session.value.rootPath, taskName: createTaskName.value })
    if (!resp.ok) {
      createError.value = resp.error || '任务创建失败'
      return
    }
    createSummary.value = resp.createMsg || `已提交 ${resp.created || 0} 件作品的导入任务`
    if (resp.decisions) decisions.value = resp.decisions
    selectedWorks.value = new Set((resp.decisions && resp.decisions.selection) || [])
  } finally {
    creating.value = false
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

// —— H4 清单体高自适应与切片窗口持续同步 —— //

const LIST_MIN_H = 240
const LIST_BOTTOM_PAD = 16

let viewportRO: ResizeObserver | null = null
let layoutRO: ResizeObserver | null = null

// 平铺隐藏（树模式）时树体在同一位置，测可见者即可
function visibleListEl(): HTMLElement | null {
  const flat = listViewport.value
  if (flat && flat.offsetParent !== null) return flat
  return treeViewport.value
}

function syncListBodyH(): void {
  const el = visibleListEl()
  if (!el || el.offsetParent === null) return
  // offsetTop 链求文档级顶部偏移：与滚动位置无关，高度不随滚动抖动
  let top = 0
  let node: HTMLElement | null = el
  while (node) {
    top += node.offsetTop
    node = node.offsetParent as HTMLElement | null
  }
  listBodyH.value = Math.max(LIST_MIN_H, window.innerHeight - top - LIST_BOTTOM_PAD)
}

// 容器尺寸变化 → viewportH 同步（窗口缩放/体高调整/模式切换后切片窗口正确）；元素重建时重挂观察
function bindViewportRO(): void {
  const el = listViewport.value
  if (!el) return
  if (viewportRO) viewportRO.disconnect()
  viewportRO = new ResizeObserver(() => {
    if (listViewport.value) viewportH.value = listViewport.value.clientHeight || 420
  })
  viewportRO.observe(el)
}

// 快照晚到（挂载后才识别）时清单容器才出现，经模板 ref 变化接入体高同步与观察
watch(listViewport, (el) => {
  if (!el) return
  syncListBodyH()
  bindViewportRO()
})

onMounted(async () => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  offEvent = Events.On(TOPIC_EVENT, onEvent)
  // 体高同步三触发：窗口缩放、上游内容增减（body 尺寸变化改变清单顶部偏移）、清单容器出现
  window.addEventListener('resize', syncListBodyH)
  // 光标追踪（剧本步2′ chips 插入用）：事件流维护选区快照
  document.addEventListener('selectionchange', onNameSelChange)
  for (const t of ['click', 'keyup', 'select', 'input'] as const) {
    document.addEventListener(t, onNameSelDelegate, true)
  }
  layoutRO = new ResizeObserver(syncListBodyH)
  layoutRO.observe(document.body)
  const root = await restoreSession()
  if (root && session.value) {
    if (session.value.state === 'recognized') {
      await loadPanorama(root)
    }
    await nextTick()
    syncListBodyH()
    bindViewportRO()
  }
})

onUnmounted(() => {
  if (offResponse) offResponse()
  if (offEvent) offEvent()
  // 断开观察与监听，防卸载后回调触达已失效节点
  viewportRO?.disconnect()
  layoutRO?.disconnect()
  window.removeEventListener('resize', syncListBodyH)
  document.removeEventListener('selectionchange', onNameSelChange)
  for (const t of ['click', 'keyup', 'select', 'input'] as const) {
    document.removeEventListener(t, onNameSelDelegate, true)
  }
})
</script>

<style scoped>
.li-ws {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
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
  font-size: 20px;
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

.li-ws-header-link {
  font-size: 13px;
  color: var(--app-color-primary);
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

.li-ws-card {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
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

/* —— 识别中状态条 —— */

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

/* —— 双栏主从主体：左栏自适应（保底 ~480px），右栏固定 ~400px —— */

.li-ws-main {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}

.li-ws-left {
  flex: 1 1 auto;
  min-width: 480px;
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

/* —— 区5 资源清单（虚拟滚动） —— */

/* H4：高度由 listBodyH 内联注入（视口剩余空间实测），此处只兜底最小高度 */
.li-ws-list {
  position: relative;
  min-height: 240px;
  overflow: auto;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
}

.li-ws-list-spacer {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  pointer-events: none;
}

.li-ws-row {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
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

/* —— 折叠区（默认收起，会话内记忆不持久化） —— */

.li-ws-fold {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
}

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

.li-ws-fold-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-ws-fold-body {
  padding: 0 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

/* —— 区5 清单模式切换（剧本F：▦ 清单 ↔ ⛁ 目录树） —— */

.li-ws-mode-toggle {
  display: inline-flex;
  gap: 4px;
}

/* —— 区5 目录树模式（剧本F：全量渲染）；目录行复用旧目录结构区的含义覆盖标记样式 —— */

/* H4：体高与平铺清单同享 listBodyH 内联注入，此处只兜底最小高度 */
.li-ws-tree {
  min-height: 240px;
  overflow: auto;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 4px 0;
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

.li-ws-dir-meaning {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-ws-dir-uncovered {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-tag-neutral-text);
}

/* 区6 序列群样式已退役（剧本v2：折叠区并入「疑似作品集」第三标签候选卡墙，见下节） */

/* —— 剧本v2 疑似作品集候选卡墙（第三标签；区6 序列群折叠区能力并入） —— */

.li-ws-sets {
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px 10px;
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

/* —— 区7 待裁决项 —— */

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

/* —— 区8 规则档案态 —— */

.li-ws-rules {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  overflow: hidden;
}

.li-ws-rule-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 90px 60px 60px;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  padding: 4px 10px;
}

.li-ws-rule-row:nth-child(odd) {
  background: var(--app-fill-color);
}

.li-ws-rule-head {
  font-weight: 600;
  color: var(--app-text-secondary);
}

.li-ws-rule-id {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--app-text-primary);
}

.li-ws-rule-src,
.li-ws-rule-enabled,
.li-ws-rule-hits {
  color: var(--app-text-regular);
}

.li-ws-rule-enabled.is-off {
  color: var(--app-text-secondary);
}

/* —— 区9 会话决策态 —— */

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

/* —— 区2 选择创建栏 —— */

.li-ws-createbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

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

.li-ws-create-note {
  font-size: 12px;
  color: var(--app-text-regular);
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

/* —— 右栏解析面板 —— */

.li-ws-right {
  flex: 0 0 400px;
  width: 400px;
  position: sticky;
  top: 12px;
  max-height: calc(100vh - 24px);
  overflow-y: auto;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  display: flex;
  flex-direction: column;
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
  font-size: 11px;
  color: var(--app-text-secondary);
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
