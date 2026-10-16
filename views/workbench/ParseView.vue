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

    <!-- 横幅区：加载失败 / 档案外变 / 会话恢复 -->
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
    <el-alert
      v-else-if="session"
      class="li-ws-banner"
      type="success"
      :closable="false"
      show-icon
      :title="`已恢复解析会话（${session.rootPath}）`"
      description="会话进度与赋值均从插件持久化存储恢复，可继续解析工作。"
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
        <!-- 区1 统计徽章行（含增量跳过目录统计） -->
        <section class="li-ws-stats">
          <span class="li-ws-stat">文件 {{ snapshot.stats.totalFiles }}</span>
          <span class="li-ws-stat li-ws-stat-ok">已识别 {{ snapshot.stats.identified }}</span>
          <span class="li-ws-stat li-ws-stat-clue">有线索 {{ snapshot.stats.clueful }}</span>
          <span class="li-ws-stat">无线索 {{ snapshot.stats.plain }}</span>
          <span class="li-ws-stat">序列群 {{ snapshot.stats.groupCount }}</span>
          <span class="li-ws-stat li-ws-stat-warn">待裁决 {{ snapshot.stats.pendingFiles }}</span>
          <span v-if="snapshot.stats.skippedDirs > 0" class="li-ws-stat">增量跳过目录 {{ snapshot.stats.skippedDirs }}</span>
        </section>

        <!-- 区3 重识别差异条（生成规则后的强制验证轮高亮：过窄＝仍有未覆盖、过泛＝错套） -->
        <section v-if="diff && diffTotal > 0" class="li-ws-diffbar">
          <span class="li-ws-diff-title">重识别差异（生成规则后的验证轮）：</span>
          <span class="li-ws-diff-chip is-changed">变更 {{ diff.changed ? diff.changed.length : 0 }}</span>
          <span class="li-ws-diff-chip is-added">新增 {{ diff.added ? diff.added.length : 0 }}</span>
          <span class="li-ws-diff-chip is-removed">消失 {{ diff.removed ? diff.removed.length : 0 }}</span>
          <span class="li-ws-diff-note">过窄＝差异里仍有未覆盖、过泛＝差异里错套——请在清单核对标色行</span>
        </section>

        <!-- 双栏主从主体：左栏自适应（保底 ~480px），右栏固定 ~400px 可收起 -->
        <section class="li-ws-main">
          <div class="li-ws-left">
            <!-- 区5 资源清单：过滤器行（状态/待裁决/已赋值/差异标）+ 组合选择 + 虚拟滚动清单 -->
            <section class="li-ws-card">
              <h3 class="li-ws-card-title">
                资源清单
                <span class="li-ws-card-note">{{ filteredFiles.length }} 项 · 虚拟滚动 · 点击行联动右栏解析面板</span>
              </h3>
              <div class="li-ws-combos">
                <div class="li-ws-filter">
                  <button
                    v-for="f in fileFilters"
                    :key="f.key"
                    class="li-ws-filter-btn"
                    :class="{ 'is-active': fileFilter === f.key }"
                    @click="fileFilter = f.key"
                  >{{ f.label }}</button>
                </div>
                <div class="li-ws-combo-actions">
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
              </div>
              <div ref="listViewport" class="li-ws-list" @scroll="onListScroll">
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
            </section>

            <!-- 区4 目录结构折叠区（层含义状态；组合①：按目录递选子树作品；默认收起、会话内记忆） -->
            <section class="li-ws-fold">
              <button type="button" class="li-ws-fold-head" :aria-expanded="!folded.dirs" @click="folded.dirs = !folded.dirs">
                <span class="li-ws-fold-arrow" :class="{ 'is-open': !folded.dirs }">▸</span>
                <span class="li-ws-fold-title">目录结构</span>
                <span class="li-ws-fold-note">{{ (snapshot.dirs || []).length }} 层 · 含义覆盖 {{ coveredDirCount }}/{{ dirNodes.length }} · 递选已选 {{ dirTreeSelectedText('') }}</span>
              </button>
              <div v-show="!folded.dirs" class="li-ws-fold-body">
                <div class="li-ws-dirlist">
                  <div v-for="dir in dirNodes" :key="dir.relKey" class="li-ws-dir-row" :style="{ paddingLeft: `${dir.depth * 16 + 8}px` }">
                    <el-checkbox
                      class="li-ws-dir-cb"
                      :model-value="dirTreeChecked(dir.relKey)"
                      :indeterminate="dirTreeIndeterminate(dir.relKey)"
                      @change="(v) => setDirTreeSelected(dir.relKey, !!v)"
                    >{{ dir.name }}</el-checkbox>
                    <span class="li-ws-dir-files">{{ dir.fileCount }} 文件</span>
                    <span class="li-ws-dir-pick">已选 {{ dirTreeSelectedText(dir.relKey) }}</span>
                    <template v-if="dir.meanings && dir.meanings.length">
                      <span v-for="(m, i) in dir.meanings" :key="i" class="li-ws-dir-meaning">{{ meaningLabel(m) }}</span>
                    </template>
                    <span v-else-if="dir.depth > 0" class="li-ws-dir-uncovered">未覆盖</span>
                  </div>
                </div>
              </div>
            </section>

            <!-- 区6 序列群折叠区（兄弟群：前缀/组大小/序号形态/成群落单） -->
            <section class="li-ws-fold">
              <button type="button" class="li-ws-fold-head" :aria-expanded="!folded.groups" @click="folded.groups = !folded.groups">
                <span class="li-ws-fold-arrow" :class="{ 'is-open': !folded.groups }">▸</span>
                <span class="li-ws-fold-title">序列群</span>
                <span class="li-ws-fold-note">{{ (snapshot.groups || []).length }} 群</span>
              </button>
              <div v-show="!folded.groups" class="li-ws-fold-body">
                <p v-if="!(snapshot.groups || []).length" class="li-ws-empty">无成群序列</p>
                <div v-for="g in snapshot.groups || []" :key="`${g.dirRelKey}/${g.key}`" class="li-ws-group">
                  <div class="li-ws-group-head">
                    <span class="li-ws-group-key">{{ g.key || '（空前缀）' }}</span>
                    <span class="li-ws-group-dir">{{ g.dirRelKey || '根目录' }}</span>
                    <span class="li-ws-group-size">{{ g.size }} 成员</span>
                    <span class="li-ws-group-rule">{{ g.ruleId || '结构' }}</span>
                  </div>
                  <div class="li-ws-group-members">
                    <span
                      v-for="m in g.members"
                      :key="m.relPath"
                      class="li-ws-group-member"
                      :class="{ 'is-selected': m.relPath === selectedRelPath }"
                      @click="selectFile(m.relPath)"
                    >#{{ m.seqText || m.seq }} {{ fileNameOf(m.relPath) }}</span>
                  </div>
                </div>
              </div>
            </section>

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
                      {{ dimLabel(c.dimension) }}={{ c.value }}（{{ sourceLabel(c.source) }}<template v-if="c.ruleId"> · {{ c.ruleId }}</template>）
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

            <!-- 区2 选择创建栏：作品级原子选择集 → 创建任务（含对账/创建结果横幅与悬空对账提示） -->
            <section class="li-ws-card">
              <div class="li-ws-createbar">
                <span class="li-ws-create-count">已选 {{ selectedWorks.size }} 件作品<span class="li-ws-create-sub">（可选 {{ selectableWorks.length }} 件 / 共 {{ allWorks.length }} 件）</span></span>
                <el-button
                  type="primary"
                  size="small"
                  :loading="creating"
                  :disabled="selectedWorks.size === 0"
                  @click="createTasks"
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
               赋值表单/泛化/裁决完整交互见 ParsePanel.vue（阶段3接入） -->
          <aside v-if="!rightCollapsed" class="li-ws-right">
            <div class="li-ws-panel-head">
              <span class="li-ws-panel-title">解析面板</span>
              <button type="button" class="li-ws-panel-btn" title="收起面板" @click="rightCollapsed = true">收起 »</button>
            </div>
            <div class="li-ws-panel-body">
              <!-- 边界：右栏空态引导 -->
              <div v-if="!selectedRelPath" class="li-ws-panel-guide">
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
                      <span class="li-ws-decl-src">{{ sourceLabel(d.source) }}<template v-if="d.ruleId"> · {{ d.ruleId }}</template></span>
                    </div>
                  </template>
                  <p v-else class="li-ws-empty">无终选声明</p>
                </div>
                <div class="li-ws-sec">
                  <h4 class="li-ws-sec-title">线索</h4>
                  <template v-if="selectedFile.clues && selectedFile.clues.length">
                    <div v-for="(c, i) in selectedFile.clues" :key="i" class="li-ws-clue">{{ clueLabel(c) }}</div>
                  </template>
                  <p v-else class="li-ws-empty">无线索</p>
                </div>
                <div class="li-ws-sec">
                  <h4 class="li-ws-sec-title">所属序列群</h4>
                  <div v-if="selectedGroup" class="li-ws-sec-block">
                    <p class="li-ws-sec-line">集 {{ selectedGroup.key || '（空前缀）' }} · {{ selectedGroup.dirRelKey || '根目录' }} · {{ selectedGroup.size }} 成员 · {{ selectedGroup.ruleId || '结构' }}</p>
                    <p class="li-ws-sec-note">本件序号：#{{ selectedGroupMember ? (selectedGroupMember.seqText || selectedGroupMember.seq) : '-' }}</p>
                  </div>
                  <p v-else class="li-ws-empty">不属于任何序列群</p>
                </div>
                <!-- 阶段3：待裁决就地双路裁决 + 赋值表单（五字段预填）/ 仅保存赋值 / 保存并泛化成规则（内联维度三选）/
                     忽略 / 清除赋值 / 泛化结果与差异提示；请求走本组件唯一通道，忽略后仅重取全景 -->
                <ParsePanel
                  :root="session ? session.rootPath : ''"
                  :file="selectedFile"
                  :assignments="assignmentList"
                  :ignored="selectedIgnored"
                  :pending-items="selectedPending"
                  :diff="diff"
                  :recognizing="recognizing"
                  :request="request"
                  @refresh="onPanelRefresh"
                />
              </template>
            </div>
          </aside>
        </section>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { Events } from '@wailsio/runtime'
import ParsePanel from './ParsePanel.vue'

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
  assigned?: boolean
}

interface SnapGroup {
  dirRelKey: string
  key: string
  size: number
  ruleId: string
  members: { relPath: string; seq: number; seqText: string }[]
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
  }[]
  files: SnapFile[]
  groups: SnapGroup[]
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
  // assignments 元素为后端 workbenchAssignmentView 全量投影（五字段供右栏表单预填）
  assignments?: { relPath: string; author?: string; authorRole?: string; title?: string; tags?: string[]; collection?: string; generalized?: boolean; updatedAtMs?: number }[]
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
// 右栏收起态与折叠区开合态：默认右栏展开、折叠区全收起
const rightCollapsed = ref(false)
const folded = ref({ dirs: true, groups: true, pending: true, rules: true, decisions: true })

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
const scrollTop = ref(0)
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

function selectFile(relPath: string): void {
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

// 悬空选择键（重识别后选择集引用了已不存在的作品）提示
const danglingSelection = computed<number>(() => {
  let count = 0
  for (const id of selectedWorks.value) {
    if (!workIndex.value.has(id)) count += 1
  }
  return count
})

// —— 创建任务：选择集整集落库 → 创建编排（对账→CreateTask→清理） —— //

async function createTasks(): Promise<void> {
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
    const resp = await request('createTasks', { root: session.value.rootPath })
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

const coveredDirCount = computed(() => dirNodes.value.filter((d) => d.meanings && d.meanings.length).length)
const dirNodes = computed(() => (snapshot.value ? snapshot.value.dirs : null) || [])

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

onMounted(async () => {
  offResponse = Events.On(TOPIC_RESPONSE, onResponse)
  offEvent = Events.On(TOPIC_EVENT, onEvent)
  const root = await restoreSession()
  if (root && session.value) {
    if (session.value.state === 'recognized') {
      await loadPanorama(root)
    }
    await nextTick()
    if (listViewport.value) {
      viewportH.value = listViewport.value.clientHeight || 420
    }
  }
})

onUnmounted(() => {
  if (offResponse) offResponse()
  if (offEvent) offEvent()
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

.li-ws-card-note {
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

/* —— 区1 统计徽章行 —— */

.li-ws-stats {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-ws-stat {
  font-size: 12px;
  padding: 3px 10px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-tag-neutral-text);
}

.li-ws-stat-ok {
  background: var(--app-tag-success-bg);
  color: var(--app-tag-success-text);
}

.li-ws-stat-clue {
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-ws-stat-warn {
  background: var(--app-tag-danger-bg);
  color: var(--app-tag-danger-text);
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

/* —— 区3 差异条 —— */

.li-ws-diffbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 8px 12px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
}

.li-ws-diff-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-primary);
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

.li-ws-diff-note {
  font-size: 12px;
  color: var(--app-text-secondary);
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

/* —— 过滤器与组合选择 —— */

.li-ws-combos {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.li-ws-filter {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
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

.li-ws-combo-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
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

.li-ws-list {
  position: relative;
  height: 420px;
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

/* —— 区4 目录结构 —— */

.li-ws-dirlist {
  max-height: 280px;
  overflow: auto;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 4px 0;
}

.li-ws-dir-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  padding: 2px 8px;
  white-space: nowrap;
}

.li-ws-dir-cb {
  margin-right: 0;
  height: auto;
}

.li-ws-dir-files {
  color: var(--app-text-secondary);
}

.li-ws-dir-pick {
  font-size: 11px;
  color: var(--app-text-regular);
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

/* —— 区6 序列群 —— */

.li-ws-group {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius-sm);
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-ws-group-head {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  flex-wrap: wrap;
}

.li-ws-group-key {
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ws-group-dir {
  color: var(--app-text-secondary);
}

.li-ws-group-size {
  padding: 1px 6px;
  border-radius: var(--app-radius-sm);
  background: var(--app-tag-neutral-bg);
  color: var(--app-text-regular);
}

.li-ws-group-rule {
  color: var(--app-text-secondary);
  font-size: 11px;
}

.li-ws-group-members {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.li-ws-group-member {
  font-size: 12px;
  padding: 1px 8px;
  border-radius: var(--app-radius-sm);
  background: var(--app-fill-color);
  color: var(--app-text-regular);
  cursor: pointer;
}

.li-ws-group-member.is-selected {
  box-shadow: inset 2px 0 0 var(--app-color-primary);
  background: var(--app-tag-neutral-bg);
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
