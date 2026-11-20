<template>
  <AutoHeightDialog
    v-model:state="visible"
    width="min(880px, 94vw)"
    :close-on-click-modal="false"
  >
    <template #header>
      <div class="li-red-header">
        <span class="li-red-title">{{ headerTitle }}</span>
        <span class="li-red-header-note">{{ headerNote }}</span>
      </div>
    </template>

    <!-- 结果卡在场时表单区同步收窄（配套 footer 结果卡封顶）：弹窗总高压回视口内，
         见样式 .li-red-body--preview 注释 -->
    <div class="li-red-body" :class="{ 'li-red-body--preview': mode === 'rule' && previewResult }">
      <!-- ================= 入口①：档案规则编辑（SubpageRules「编辑/新建」） ================= -->
      <template v-if="mode === 'rule'">
        <!-- 规则元信息：描述 + 启停 -->
        <div class="li-red-meta">
          <el-input v-model="descInput" placeholder="规则描述（人话，列表呈现用）" clearable />
          <el-switch v-model="enabledInput" active-text="启用" inactive-text="停用" />
        </div>

        <!-- 三子句形态（决策9·终审重梳轮）：一条规则=匹配（谓词行布尔自由+属性
             派生）/分组（属性选键声明）/产出（三产出零互斥）——类别步与「元数据
             规则｜聚合规则」两分概念退役（落档类别=推导细节：有聚合成作品产出=
             aggregate）；条件行服务一切产出、「轨行」词汇退役 -->

        <!-- 匹配区（三子句之一）：卡片总览——条件卡（CondxRowsEditor，序号/目标/
              模板摘要/复杂度徽标/接词，恒定高度）；谓词行布尔自由恒开（聚合产出
              在场不再隐藏/清空提取与布尔能力）；逐条件编辑经条件编辑弹窗（三步
              漏斗=目标三卡〔文件名/目录名/序列〕→格式模板→附加条件+值校验）；
              括号子组页内直显（组框缩进承载成员卡，嵌套封顶 3 层） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">匹配<span class="li-red-sec-note">谓词行布尔自由 · 决定规则命中哪些文件</span></h4>
            <el-button size="small" text @click="toggleCondxJson">
              {{ condxJsonMode ? '切回结构化编辑' : '高级（JSON）' }}
            </el-button>
          </div>
          <p class="li-red-summary">{{ condxSummary }}</p>

          <template v-if="!condxJsonMode">
            <p v-if="condxError" class="li-red-error">{{ condxError }}</p>
            <!-- 条件卡片区（总览）：条件卡列表（CondxRowsEditor，括号子组=组框页内
                 直显、成员卡与顶层同款）；位次漂移结果经 occurDrift 下传（快照与
                 刷新留本组件，提取行漂移提示与卡片⚠徽标共用）、行换位经 moved 上抛
                 （onCondxRowsMoved 改写三类行号引用；组框内换位逐层上浮同一消费）、
                 逐条件编辑经条件编辑弹窗（editRow 开窗，组内成员卡同一链路）、
                 提取区 JSON 模式经 extractionsJsonMode 下传（移动按钮禁用） -->
            <div class="li-red-zone-head">
              <span class="li-red-zone-title">匹配（{{ condxCardCount }}）</span>
            </div>
            <CondxRowsEditor
              v-model:rows="condxRows"
              :occur-drift="occurDrift"
              :extractions-json-mode="extractionsJsonMode"
              add-label="+ 加条件"
              :agg-mode="!!aggProduction"
              :ref-label-of="refRuleLabel"
              @edit-row="openCondxRowEdit"
              @moved="onCondxRowsMoved"
            />
            <!-- 占位词捕获清单（行号+词）：提取段/分组声明引用的唯一来源 -->
            <div v-if="captureList.length" class="li-red-caps">
              <span class="li-red-label">占位词捕获清单</span>
              <div v-for="(c, i) in captureList" :key="`${c.row}/${c.word}/${i}`" class="li-red-cap-row">
                <span class="li-red-cap-name">第 {{ c.row }} 行 · {{ '{' + c.word + '}' }}</span>
                <span v-if="c.repeat" class="li-red-cap-multi" title="重复段词（多值产出）：提取段可引用，暂不能用于生效条件/值校验/组合接线">多值</span>
              </div>
              <p class="li-red-hint">
                {占位词}＝命名捕获槽：只负责在匹配时捕获文本，本身不识别也不产生元数据——槽名与维度同字只是习惯叫法，无绑定
              </p>
            </div>
            <p v-else class="li-red-hint">
              尚无可引用的占位词——在行模板中写入 {作者}/{标题} 等词条后，提取与分组才能引用其捕获值
            </p>
          </template>

          <template v-else>
            <el-input
              v-model="condxText"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 16 }"
              class="li-red-json-area"
              spellcheck="false"
            />
            <p v-if="condxError" class="li-red-error">条件行 conditions JSON：{{ condxError }}</p>
            <p class="li-red-hint">
              JSON 数组（行对象：connector/negated/condition 或 group 括号子组递归）；切回结构化编辑前须通过解析，
              解析错不丢输入
            </p>
          </template>
        </section>

        <!-- 分组区（三子句之二·决策9）：分组声明=属性选键（匹配行捕获/序列群键/
             所在目录，单一或拼接），可多条——分组只在此声明、不参与布尔运算、
             作用于匹配求值后的命中集；产出绑定消费（聚合成作品/元数据文件各绑
             一条声明——序列化映射到既有键字段，内核 schema 零改动） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">分组<span class="li-red-sec-note">属性选键声明 · 作用于匹配求值后的命中集</span></h4>
          </div>
          <div class="li-red-zone-head">
            <span class="li-red-zone-title">分组（{{ groupDecls.length }}）</span>
          </div>
          <div v-for="(d, di) in groupDecls" :key="d.uid" class="li-out-card">
            <div class="li-condx-card-line">
              <span class="li-out-mark">◇</span>
              <span class="li-condx-card-tpl" :title="groupDeclBlockLabel(d)">{{ groupDeclBlockLabel(d) }}</span>
              <el-button size="small" text @click="editGroupDecl(di)">[ 编辑 ]</el-button>
              <el-button size="small" text type="danger" @click="removeGroupDecl(di)">[删]</el-button>
            </div>
          </div>
          <el-button size="small" text @click="addGroupDecl">+ 加分组</el-button>
          <p class="li-red-hint">
            分组键=逐文件属性（匹配行捕获/序列行群键/所在目录，单一或拼接）；未被产出绑定的声明保存时拦截
          </p>
          <p v-for="e in groupDeclErrors" :key="`gde-${e}`" class="li-red-row-hint">{{ e }}</p>
        </section>

        <!-- 产出区（三子句之三）：三产出零互斥并存——提取元数据/元数据文件/聚合成
             作品（绑定分组）；条目符号统一 ◇；gating：无分组声明且无序列行时
             「+ 聚合成作品」置灰附因（D 帧4） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">产出<span class="li-red-sec-note">三产出自由组合 · 命中后取值声明到维度</span></h4>
            <el-button size="small" text @click="toggleExtractionsJson">
              {{ extractionsJsonMode ? '切回结构化编辑' : '高级（JSON）' }}
            </el-button>
          </div>
          <p class="li-red-hint">
            元数据只在这里产生：把匹配区某个捕获槽的值「赋给维度」才是识别——匹配命中而产出没有赋值行时，不产出任何元数据
          </p>

          <template v-if="!extractionsJsonMode">
            <div class="li-red-zone-head">
              <!-- 产出计数=提取行数+「元数据文件」条目数+「聚合成作品」条目数（平级产出各计一条） -->
              <span class="li-red-zone-title">产出（{{ productionCount }}）</span>
            </div>
            <!-- 提取元数据汇总卡（D 帧3「◇ 提取元数据（2）：作者←{作者} 标题←{标题}」） -->
            <div v-if="extractionRows.length" class="li-out-card li-red-metadata-card">
              <div class="li-condx-card-line">
                <span class="li-out-mark">◇</span>
                <span class="li-condx-card-tpl" :title="extractionSummaryText">提取元数据（{{ extractionRows.length }}）：{{ extractionSummaryText }}</span>
              </div>
            </div>
            <template v-for="(row, i) in extractionRows" :key="i">
              <!-- 「来自供给方规则」分组头：置于首个供给方捕获行之前，转写提示
                   小字常驻（与镜像缺失 ⚠ 联动） -->
              <div v-if="isFirstRefCaptureRow(i)" class="li-red-ref-group">
                <div class="li-red-ref-group-title">── 来自供给方规则 ──</div>
                <p class="li-red-ref-group-hint">每条「供给方捕获」提取行转写供给方规则（本规则目录行作用域附件所引）一行声明的捕获值；供给方多形态（多行）时须逐行各接一条，未接线的形态取不到值</p>
              </div>
              <!-- 产出卡（总览）：摘要行恒定一行高（维度 ← 来源摘要+角色/仅序列
                   成员小缀，纯函数派生）；〔编辑〕展开本行完整编辑器——行级校验
                   不过的行恒展开（保存闸门呈现不因收起而不可见） -->
              <div class="li-out-card" :class="{ 'is-invalid': !!extractionRowHint(row) }">
                <div class="li-condx-card-line">
                  <span class="li-out-mark">◇</span>
                  <span class="li-condx-card-tpl" :title="outCardText(row)">{{ outCardText(row) }}</span>
                  <span
                    v-if="extractionDriftWarn(row) || refCaptureMirrorWarn(row, i)"
                    class="li-condx-card-flag is-warn"
                    :title="extractionDriftWarn(row) || refCaptureMirrorWarn(row, i)"
                  >⚠</span>
                  <el-button size="small" text @click="toggleOutOpen(i)">[编辑]</el-button>
                </div>
                <div v-if="outIsOpen(i)" class="li-out-editor">
                <div class="li-red-ex-main">
                <el-select v-model="row.dimension" size="small" placeholder="维度" class="li-red-ex-dim">
                  <el-option v-for="d in dimOptionsOf(row)" :key="d.value" :label="d.label" :value="d.value" />
                </el-select>
                <span class="li-red-arrow">←</span>
                <el-select v-model="row.from" size="small" placeholder="来源" class="li-red-ex-from">
                  <el-option v-for="f in FROM_OPTIONS" :key="f.value" :label="f.label" :value="f.value" />
                </el-select>
                <el-select
                  v-if="row.from === 'scan'"
                  v-model="row.scanScope"
                  size="small"
                  placeholder="扫描范围"
                  class="li-red-ex-scan"
                >
                  <el-option v-for="s in SCAN_SCOPE_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
                </el-select>
                <el-select
                  v-if="row.from === 'capture'"
                  :model-value="`${row.captureRow}/${row.captureName}`"
                  size="small"
                  placeholder="占位词（只列上半声明的 行N·{词}）"
                  class="li-red-ex-capture"
                  @update:model-value="setRowCapture(row, $event)"
                >
                  <el-option
                    v-for="c in captureList"
                    :key="`${c.row}/${c.group}`"
                    :label="`第 ${c.row} 行 · {${c.word}}`"
                    :value="`${c.row}/${c.group}`"
                  />
                </el-select>
                <el-select
                  v-else-if="row.from === 'refCapture'"
                  :model-value="`${row.refRule}/${row.captureRow}/${row.captureName}`"
                  size="small"
                  placeholder="供给方·占位词"
                  class="li-red-ex-capture"
                  @update:model-value="setRowRefCapture(row, $event)"
                >
                  <el-option-group
                    v-for="g in refCaptureGroups"
                    :key="`${g.ruleId}/${g.index}`"
                    :label="`${g.label}（供给方 ${g.index}）`"
                  >
                    <el-option
                      v-for="c in g.captures"
                      :key="`${g.ruleId}/${c.row}/${c.group}`"
                      :label="`${g.label} · 第 ${c.row} 行 · {${c.word}}`"
                      :value="`${g.ruleId}/${c.row}/${c.group}`"
                    />
                  </el-option-group>
                  <!-- 失效占位词残影选项（剧本三·帧1）：仅供行上呈现，不可重选 -->
                  <el-option
                    v-for="o in refCaptureStaleOptions"
                    :key="`stale-${o.value}`"
                    :label="o.label"
                    :value="o.value"
                    disabled
                  />
                </el-select>
                <el-input
                  v-else-if="row.from === 'constant' || row.from === 'adjudication'"
                  v-model="row.constant"
                  size="small"
                  placeholder="常量值"
                  class="li-red-ex-const"
                />
                <el-button size="small" text type="danger" @click="removeExtractionRow(i)">删行</el-button>
              </div>
              <div class="li-red-ex-sub">
                <template v-if="row.from === 'scan'">
                  <span class="li-red-scan-note">
                    贪婪识别=扫描候选片段（括号内/分隔符切分/整段）×库内{{ scanDimLabel(row.dimension) }}集合精确比对，查不到不提取；
                    标签全收，作者/作品集歧义不填
                  </span>
                </template>
                <span v-if="row.from !== 'scan' && row.from !== 'constant' && row.from !== 'adjudication'" class="li-red-strip-note">
                  剥壳由字面括号模板承载：首组内名（[{标签}]{任意}）即多组目录名取首组内名、尾段吞掉
                </span>
                <el-input v-model="row.roleName" size="small" placeholder="角色名（可空）" class="li-red-ex-role" />
                <label v-if="row.from !== 'scan'" class="li-red-cond">
                  <input v-model="row.seqOnly" type="checkbox">
                  <span>仅序列成员产出</span>
                </label>
                <span v-if="extractionRowHasOtherConds(row) && row.from !== 'scan'" class="li-red-other">含其他手写条件（高级 JSON 模式可编辑）</span>
              </div>
              <p v-if="extractionRowHint(row)" class="li-red-row-hint">{{ extractionRowHint(row) }}</p>
              <!-- 位次漂移提示（提取面）：capture 引用受影响组名、或 when 条件
                   （含组合接线限定名）文本命中受影响组名时报出 -->
              <p v-if="extractionDriftWarn(row)" class="li-red-mirror-warn">{{ extractionDriftWarn(row) }}</p>
              <!-- 镜像缺失提示（D）：成员多行声明同组词而本行只接线部分行 -->
              <p v-if="refCaptureMirrorWarn(row, i)" class="li-red-mirror-warn">{{ refCaptureMirrorWarn(row, i) }}</p>
                </div>
              </div>
            </template>
            <el-button size="small" text @click="addExtractionRow">+ 提取元数据</el-button>
          </template>

          <template v-else>
            <el-input
              v-model="extractionsText"
              type="textarea"
              :autosize="{ minRows: 6, maxRows: 16 }"
              class="li-red-json-area"
              spellcheck="false"
            />
            <p v-if="extractionsError" class="li-red-error">提取动作 extractions JSON：{{ extractionsError }}</p>
            <p class="li-red-hint">JSON 数组（dimension / from / captureName / constant / roleName / when 条件表达式）</p>
          </template>

          <!-- 元数据文件条目（产出区平级条目·决策1，与「聚合成作品」平级、可各自
               单独添加或并存）：未加=「+ 元数据文件」添加入口（剧本A 帧1）；加了
               =◇ 条目卡（[打开]进指派弹窗、[移除]去产出） -->
          <div v-if="metadataFileProduction" class="li-out-card li-red-metadata-card">
            <div class="li-condx-card-line">
              <span class="li-out-mark">◇</span>
              <span class="li-condx-card-tpl" :title="metadataFileSummary">{{ metadataFileSummary }}</span>
              <el-button size="small" text @click="openMetadataDialog">[打开]</el-button>
              <el-button size="small" text type="danger" @click="removeMetadataFile">[移除]</el-button>
            </div>
          </div>
          <el-button v-else size="small" text @click="openMetadataDialog">+ 元数据文件</el-button>
          <!-- 聚合成作品条目（D 帧3「◇ 聚合成作品 · 图文（article）  [ 编辑 ] [移除]」）：
               分组绑定声明（作品标识=绑定分组）+类型+轨道分配（两步弹窗） -->
          <div v-if="aggProduction" class="li-out-card" :class="{ 'is-invalid': aggErrors.length > 0 }">
            <div class="li-condx-card-line">
              <span class="li-out-mark">◇</span>
              <span class="li-condx-card-tpl" :title="aggCardText">{{ aggCardText }}</span>
              <el-button size="small" text @click="openTrackDialog">[ 编辑 ]</el-button>
              <el-button size="small" text type="danger" @click="removeAggregateProduction">[移除]</el-button>
            </div>
          </div>
          <template v-else>
            <el-button size="small" text :disabled="aggAdditionGated" @click="openTrackDialog">+ 聚合成作品</el-button>
            <!-- gating 附注（D 帧4「（需分组声明或序列行，暂不可加）」）：置灰有因，
                 加分组声明（帧1）即解 -->
            <span v-if="aggAdditionGated" class="li-red-hint">（{{ AGG_ADDITION_GATING_NOTE }}）</span>
          </template>
          <!-- 「元数据文件」产出保存闸门错误清单 -->
          <p v-for="(e, ei) in metadataFileErrors" :key="`mfm-${ei}`" class="li-red-row-hint">{{ e }}</p>
          <!-- 聚合面保存闸门错误清单（保存被拦时逐条在场） -->
          <p v-for="(e, ei) in aggErrors" :key="`agg-${ei}`" class="li-red-row-hint">{{ e }}</p>
        </section>

        <!-- 合成策略（随旧表单随迁）：角色绑定 + 维度策略（提取元数据产出在场即相关，
             聚合并存已放开——决策10） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">合成策略<span class="li-red-sec-note">该规则产出声明的合成方式</span></h4>
          </div>
          <div class="li-red-synth-head">
            <span>角色绑定（来自该来源的声明一律附加角色）</span>
            <el-button size="small" text @click="roleRows.push({ source: 'filename', roleName: '' })">加一行</el-button>
          </div>
          <div v-for="(role, i) in roleRows" :key="`role${i}`" class="li-red-inline-row">
            <el-select v-model="role.source" size="small" placeholder="来源" class="li-red-inline-select">
              <el-option v-for="s in SOURCE_KIND_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
            <el-input v-model="role.roleName" size="small" placeholder="角色名（如 原画）" class="li-red-inline-input" />
            <el-button size="small" text type="danger" @click="roleRows.splice(i, 1)">删</el-button>
          </div>
          <div class="li-red-synth-head">
            <span>维度策略（同维度多来源候选的合成方式）</span>
            <el-button size="small" text @click="dimRows.push({ dimension: 'title', strategy: 'union' })">加一行</el-button>
          </div>
          <div v-for="(dim, i) in dimRows" :key="`dim${i}`" class="li-red-inline-row">
            <el-select v-model="dim.dimension" size="small" placeholder="维度" class="li-red-inline-select">
              <el-option v-for="d in DIMENSION_OPTIONS" :key="d.value" :label="d.label" :value="d.value" />
            </el-select>
            <el-select v-model="dim.strategy" size="small" placeholder="策略" class="li-red-inline-select">
              <el-option v-for="s in STRATEGY_OPTIONS" :key="s.value" :label="s.label" :value="s.value" />
            </el-select>
            <el-button size="small" text type="danger" @click="dimRows.splice(i, 1)">删</el-button>
          </div>
        </section>

        <!-- 条件编辑弹窗（卡片总览的逐条件编辑入口）：常驻挂载、开关纯由 state
             控制（先挂载后开窗，保证壳组件 @open 高度测量正常触发）；草稿〔确定〕
             经 confirm 就地回写行对象（negated/condition——行对象身份不变，行号
             引用与换位链不受影响；行级校验红字在场时弹窗内拦住不回写）、〔取消〕
             新增态移除开窗时追加的空行；occurDrift 下传供相等值校验的对方行漂移
             提示（本行模板漂移在弹窗内就地 diff）；refScopeCandidates 下传作用域
             附件的供给方候选（纯目录规则清单，剧本2 帧2/帧3） -->
        <CondxRowEditDialog
          v-model:state="condxRowDialogState"
          :row="condxRowEditTarget"
          :no="condxRowEditNo"
          :mode="condxRowEditMode"
          :root-rows="condxRows"
          :occur-drift="occurDrift"
          :ref-scope-candidates="refScopeCandidates"
          @confirm="onCondxRowEditConfirm"
          @cancel="onCondxRowEditCancel"
        />

        <!-- 聚合成作品弹窗（D 帧2·三子句定稿轮）：分组绑定行（作品标识=绑定的分组
             ——下拉可换绑分组区声明）+ 两步（第一步·作品类型下拉〔清单含插件自
             定义类型〕→第二步·轨道分配：条件行→角色，角色下拉按类型规约过滤）。
             深拷贝草稿编辑、〔确定〕就地回写产出卡；footer 恒两按钮（高度静态）；
             常驻挂载、开关纯由 state 控制 -->
        <AutoHeightDialog
          v-model:state="trackDialogState"
          width="min(680px, 94vw)"
          :close-on-click-modal="false"
          :close-on-press-escape="false"
          :show-close="false"
        >
          <template #header>
            <div class="li-red-header">
              <span class="li-red-title">聚合成作品 · {{ descInput }}</span>
            </div>
          </template>
          <div v-if="trackDraft" class="li-red-body li-red-track-body">
            <!-- 分组绑定行（D 帧2「分组：[◇ 按 {作品}+{序号} 分组▾]」）：作品标识=
                 绑定的分组，不再单独声明、不悬于产出区上方；可换绑分组区声明或
                 虚拟序列群键项（选定即物化为声明） -->
            <div class="li-red-agg-track li-red-agg-grouping">
              <span class="li-red-label">分组：</span>
              <el-select
                :model-value="trackGroupingValue"
                size="small"
                class="li-red-agg-role"
                placeholder="选择分组声明"
                @update:model-value="setTrackGrouping"
              >
                <el-option
                  v-for="o in bindableGroupOptions"
                  :key="o.value"
                  :label="o.label"
                  :value="o.value"
                />
              </el-select>
            </div>
            <p class="li-red-agg-step">第一步·作品类型:</p>
            <el-select
              v-if="trackTypeOptions.length"
              v-model="trackDraft.workType"
              size="small"
              class="li-red-agg-role"
              placeholder="选择作品类型"
              @change="onTrackTypeChange"
            >
              <el-option
                v-for="t in trackTypeOptions"
                :key="t.value"
                :label="`${t.label}（${t.value}）`"
                :value="t.value"
              />
            </el-select>
            <p v-else class="li-red-error">
              类型清单不可用（资源类型注册表查询失败或为空）——无法分配轨道；可关闭弹窗稍后重试
            </p>
            <p class="li-red-hint">
              选定类型后亮出该类型的可用轨清单（如：类型「视频」的可用轨：视频主体（必含）· 视频轨 · 音频轨 · 字幕轨 · 缩略图），再把各条件行分到其中的角色
            </p>
            <template v-if="trackSpec">
              <p class="li-red-summary">{{ aggAvailableTracksText(trackSpec) }}</p>
              <p class="li-red-agg-step">第二步·轨道分配:</p>
              <div v-for="t in trackAssignRows" :key="t.no" class="li-red-agg-track">
                <span class="li-condx-card-no">第 {{ t.no }} 行</span>
                <span class="li-condx-card-tpl" :title="t.text">· {{ t.text }}</span>
                <span class="li-red-arrow">→</span>
                <el-select
                  :model-value="trackRoleOf(t.no)"
                  size="small"
                  class="li-red-agg-role"
                  placeholder="未分配"
                  clearable
                  @update:model-value="setTrackRole(t.no, $event)"
                  @clear="setTrackRole(t.no, '')"
                >
                  <el-option
                    v-for="r in trackRoleOptions"
                    :key="r"
                    :label="aggRoleDisplay(trackSpec, r)"
                    :value="r"
                  />
                </el-select>
              </div>
              <p class="li-red-hint">
                必含轨所在行承载聚合作品的主导文件（每组恰一命中，否则该组不成立）；未分配的条件行只作分组键源、其命中文件不入轨
              </p>
            </template>
            <p v-if="trackDialogError" class="li-red-row-hint">{{ trackDialogError }}</p>
          </div>
          <template #footer>
            <div class="li-cred-actions">
              <el-button @click="trackDialogState = false">取消</el-button>
              <el-button type="primary" @click="confirmTrackDialog">确定</el-button>
            </div>
          </template>
        </AutoHeightDialog>

        <!-- 分组声明弹窗（D 帧1·三子句定稿轮）：键源（匹配行捕获/序列群键/所在
             目录）+键（捕获可拼接；序列群键=选序列行；所在目录=无键部分）。
             深拷贝草稿编辑、〔确定〕就地回写声明块；常驻挂载、开关纯由 state 控制 -->
        <AutoHeightDialog
          v-model:state="groupDialogState"
          width="min(560px, 94vw)"
          :close-on-click-modal="false"
          :close-on-press-escape="false"
          :show-close="false"
        >
          <template #header>
            <div class="li-red-header">
              <span class="li-red-title">分组声明</span>
            </div>
          </template>
          <div v-if="groupDraft" class="li-red-body">
            <div class="li-red-inline-row">
              <span class="li-red-label">键源：</span>
              <el-select v-model="groupDraft.keySource" size="small" class="li-red-agg-role" @change="onGroupKeySourceChange">
                <el-option v-for="o in GROUP_KEY_SOURCE_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
              </el-select>
              <span class="li-red-hint">（另可选：序列群键/所在目录）</span>
            </div>
            <template v-if="groupDraft.keySource === 'capture'">
              <div class="li-red-agg-identity">
                <span class="li-red-label">键：</span>
                <template v-for="(part, pi) in groupDraft.parts" :key="pi">
                  <el-select
                    :model-value="part.capture ? `${part.row}/${part.capture}` : ''"
                    size="small"
                    class="li-red-agg-ident"
                    placeholder="选占位词捕获"
                    @update:model-value="setGroupDraftPart(pi, $event)"
                  >
                    <el-option-group v-for="g in identityGroups" :key="g.row" :label="`第${g.row}行`">
                      <el-option
                        v-for="c in g.candidates"
                        :key="`${g.row}/${c.group}`"
                        :label="`{${c.word}}`"
                        :value="`${g.row}/${c.group}`"
                      />
                    </el-option-group>
                  </el-select>
                  <el-button
                    v-if="groupDraft.parts.length > 1"
                    size="small"
                    text
                    type="danger"
                    @click="groupDraft.parts.splice(pi, 1)"
                  >删</el-button>
                </template>
                <el-button size="small" text @click="addGroupDraftPart">+ 加一部分</el-button>
              </div>
              <p class="li-red-hint">
                分组键=逐文件属性，可拼接（如 第1行{作品}+第1行{序号}）；分组只在此声明、作用于匹配求值后的命中集
              </p>
            </template>
            <template v-else-if="groupDraft.keySource === 'seqKey'">
              <div class="li-red-agg-identity">
                <span class="li-red-label">键：</span>
                <el-select
                  :model-value="groupDraft.parts[0] && groupDraft.parts[0].capture ? String(groupDraft.parts[0].row) : ''"
                  size="small"
                  class="li-red-agg-ident"
                  placeholder="选序列行"
                  @update:model-value="setGroupDraftSeqRow($event)"
                >
                  <el-option v-for="s in seqRowOptions" :key="s.row" :label="`第${s.row}行 · ${s.text}`" :value="String(s.row)" />
                </el-select>
              </div>
              <p class="li-red-hint">序列群键：同一序列组的成员共享群键，每序列组一件作品</p>
            </template>
            <p v-else class="li-red-hint">所在目录即分组键——同目录的文件归为一组（映射产出的配对形态之一）</p>
          </div>
          <template #footer>
            <div class="li-cred-actions">
              <el-button @click="groupDialogState = false">取消</el-button>
              <el-button type="primary" @click="confirmGroupDecl">确定</el-button>
            </div>
          </template>
        </AutoHeightDialog>

        <!-- 元数据文件弹窗（元数据文件映射与聚合解耦·剧本A 帧2/帧3·三子句定稿轮）：
             「元数据文件」产出唯一指派面——勾选本规则条件行+内容去向（一行可
             多选）+分组绑定（A 帧2「分组：[◇ 按 {作品}+{序号} 分组▾]」+按所在
             目录选项；映射单用必选、并用可省略）；
             确定回写产出（勾选未选去向=帧3 拦截态：⚠ 行+[知道了]、勾选保留）；
             轨道随行标注随聚合产出在场才显示；
             常驻挂载、开关纯由 state 控制 -->
        <MetadataFileDialog
          v-model:state="metadataDialogState"
          :rule-title="descInput"
          :rows="metadataDialogRows"
          :show-tracks="!!aggProduction"
          :bindings="metadataFileProduction && metadataFileProduction.contentBindings ? metadataFileProduction.contentBindings : []"
          :bound-group-value="mfBoundGroupValue"
          :group-options="metadataGroupOptions"
          :has-aggregate="!!aggProduction"
          @confirm="onMetadataConfirm"
        />
      </template>

      <!-- ================= 入口②：集规则调整（SetRulePanel「调整规则」，剧本E 语义随迁） ================= -->
      <template v-else-if="suggestion">
        <!-- 上半「识别规则」：草稿识别形态（只读呈现——保存走 saveWorkSetRuleAdjust，形态由指派重推导） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">识别规则<span class="li-red-sec-note">上半 · 草稿识别形态（只读）</span></h4>
          </div>
          <p class="li-red-summary">{{ suggestion.shape || '（推导未成，无共同形态）' }}</p>
          <p v-if="suggestion.failed" class="li-red-error">{{ suggestion.failReason }}</p>
          <p v-if="suggestion.summary" class="li-red-hint">{{ suggestion.summary }}</p>
          <div v-if="suggestion.mappings && suggestion.mappings.length" class="li-red-maps">
            <div v-for="(m, i) in suggestion.mappings" :key="i" class="li-red-map">
              <span class="li-red-map-dim">{{ dimLabel(m.dimension) }}</span>
              <span class="li-red-arrow">←</span>
              <span class="li-red-map-from">{{ m.from }}</span>
            </div>
          </div>
          <div v-if="suggestion.rules && suggestion.rules.length" class="li-red-drafts">
            <p v-for="r in suggestion.rules" :key="r.id" class="li-red-draft">
              <span class="li-red-draft-id">{{ r.id }}</span>
              <span class="li-red-draft-desc">{{ r.description }}</span>
            </p>
          </div>
        </section>

        <!-- 下半「提取规则」：线索逐项指派行（剧本E 全部语义：三族来源 / 默认跟随 /
             目录段多段连续组合 / 互斥行级提示） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">提取规则<span class="li-red-sec-note">下半 · 线索逐项指派 · 保存即入档</span></h4>
          </div>
          <p class="li-red-hint">
            每条线索指派到维度/集名组合/可变/目录锚定/不用之一；集名默认跟随识别形态（序列形态识别=「序列」行
            〔群键/公共前缀〕、目录名形态识别=目录标题段）；集名组合可勾选连续多段——文件名段与目录段同语义
            （目录段组合同样须连续、段间原文保留）；末段序号默认已指。
          </p>
          <div class="li-red-clue-table">
            <div v-for="c in suggestion.clues || []" :key="c.ref" class="li-red-clue-row">
              <span class="li-red-clue-label" :title="c.ref">{{ c.label }}</span>
              <span class="li-red-clue-value" :title="c.value">{{ clueValueText(c) }}</span>
              <select v-model="assignMap[c.ref]" class="li-red-clue-select" @change="onAssignChange(c.ref)">
                <option v-for="opt in destOptionsOf(c)" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
              </select>
              <span v-if="clueRowHint(c)" class="li-red-clue-hint">{{ clueRowHint(c) }}</span>
            </div>
          </div>
          <label class="li-red-cond">
            <input v-model="memberOnly" type="checkbox">
            <span>仅序列成员产出</span>
          </label>
        </section>
      </template>
    </div>

    <template #footer>
      <div class="li-red-footer">
        <!-- 命中预览结果卡（E 件·剧本三帧2）：命中数/样例至多 3 条/口径小字；
             [×] 关闭后可改表单重试，呈现与表单互不干扰 -->
        <div v-if="mode === 'rule' && previewResult" class="li-red-preview-card">
          <div class="li-red-preview-head">
            <span class="li-red-preview-title">预览结果</span>
            <el-button size="small" text class="li-red-preview-close" @click="closePreview">×</el-button>
          </div>
          <p class="li-red-preview-stat">命中 {{ previewResult.hitCount }} / 全库 {{ previewResult.hitTotal }} 个文件</p>
          <div class="li-red-preview-samples">
            <p class="li-red-preview-samples-label">样例：</p>
            <p v-for="(s, i) in previewResult.samples" :key="i" class="li-red-preview-sample">{{ previewSampleLine(s) }}</p>
          </div>
          <p class="li-red-preview-note">按最近一次识别的文件清单与现行档案（含本条草稿）求值；保存后全量识别结果与此一致</p>
        </div>
        <span v-if="saveError" class="li-red-save-error" :title="saveError">{{ saveError }}</span>
        <span class="li-red-footer-actions">
          <!-- 预览命中（E 件·剧本三帧1/帧3）：置于保存/取消旁、按钮触发不随表单
               实时联动；等待态「预览中…」不可再点；无会话/无识别快照禁用态+下方小字 -->
          <el-button
            v-if="mode === 'rule'"
            size="small"
            :disabled="!previewEnabled || previewing"
            @click="preview"
          >
            {{ previewButtonLabel }}
          </el-button>
          <el-button v-if="mode === 'setAdjust'" size="small" @click="resetAssignDefaults">还原默认指派</el-button>
          <el-button :disabled="saving" @click="cancel">取消</el-button>
          <el-button type="primary" :loading="saving" @click="save">
            {{ mode === 'rule' ? '保存入档案' : '保存规则' }}
          </el-button>
        </span>
        <span v-if="mode === 'rule' && previewError" class="li-red-preview-error" :title="previewError">{{ previewError }}</span>
        <p v-if="mode === 'rule' && !previewEnabled" class="li-red-preview-unavailable">
          预览需要最近一次识别的文件清单——请先在解析视图执行识别
        </p>
      </div>
    </template>
  </AutoHeightDialog>
</template>

<script setup lang="ts">
// 两段式规则编辑弹窗（剧本F）：主程序全局注册的 AutoHeightDialog 壳（内容随窗口
// 高度封顶内滚，EntryConfirmDialog 先例——无需 import）。所有规则编辑处共用本
// 组件，两入口挂载、按 mode 分形：
//   ① rule（SubpageRules「编辑/新建」）——三子句形态（决策9/10·终审重梳轮）：
//      匹配区=条件行（CondxRowsEditor 条件卡，谓词行布尔自由恒开+属性派生——
//      聚合产出在场不再隐藏/清空提取与布尔能力）、分组区=分组声明（属性选键：
//      匹配行捕获/序列群键/所在目录，可多条；声明为编辑面形态，保存时序列化映射
//      到既有键字段——聚合产出绑定→identity、元数据文件产出绑定→pairing，内核
//      schema 零改动）、产出区=三产出零互斥并存（提取元数据/元数据文件/聚合成
//      作品；聚合产出卡显示绑定分组；无分组声明且无序列行时「+ 聚合成作品」
//      置灰附因〔D 帧4〕）。类别步退役：保存时推导落档类别（有聚合成作品产出=
//      aggregate）；加载既有档案（两类）全部内容装进统一表单。聚合成作品弹窗
//      （D 帧2）=分组绑定行+两步（第一步·作品类型下拉〔运行时注册表投影〕→
//      第二步·轨道分配：条件行→角色，角色按类型规约过滤）；元数据文件弹窗
//      （A 帧2）=勾选行+内容去向〔一行可多选〕+分组绑定〔分组区声明下拉+按所在
//      目录〕。保存校验=ruleFormModel 纯函数闸门（aggFormErrors+
//      metadataFileFormErrors+未绑定声明拦截），载入校验在插件 Go 侧
//      validateAggregateEntry/validateMetadataFileProduction。
//   ② setAdjust（SetRulePanel「调整规则」）：上半=草稿识别形态（只读呈现），
//      下半=指派行——剧本E 全部语义随迁：线索三族（序列 seqKey/目录段/文件名段）
//      作为来源选项、默认指派预填、目录段多段连续勾选、互斥校验「一个维度只能有
//      一个来源」「目录段组合须为连续段」行级提示、单选目的地互斥挤占、还原默认
//      指派。保存载荷经 emit 交父组件走既有 saveWorkSetRuleAdjust RPC。
// 校验不过（组引用/维度来源唯一/JSON 解析）行级就地标红、弹窗不关；JSON 高级
// 模式解析错就地显示位置不丢输入；取消不保存（destroy-on-close，再开回上次保
// 存态——打开时从 props 重新取值）。
// 目录层级表达与作用域附件（BK·剧本1/剧本2）：条件行目标三分（文件名/目录名/
// 序列）；目录行模板可多段（段间「/」=下一级、「//」=之下任意层、「/{N}」=隔 N
// 级——层级表达属目录行，file/seq 模板含「/」拒绝）；目录行附加条件可挂作用域
// 附件（refScope）：供给方规则选择（只列纯目录规则）+层距三选（0=任意深度、
// 1=紧邻、N≥2 显式隔 N 级）；提取来源「供给方捕获」＝供给方规则·占位词（候选
// =本规则目录行 refScope 供给方，按供给方序分组）。规则级 refs 已退役——引用
// 行卡与组合类别随之消亡，编辑面回三分。
import { ref, computed, watch } from 'vue'
import CondxRowEditDialog from './CondxRowEditDialog.vue'
import CondxRowsEditor from './CondxRowsEditor.vue'
import MetadataFileDialog from './MetadataFileDialog.vue'
import {
  DIMENSION_OPTIONS,
  FROM_OPTIONS,
  REFSCOPE_LIBRARY_ROOT,
  SCAN_DIMENSIONS,
  SCAN_SCOPE_OPTIONS,
  SOURCE_KIND_OPTIONS,
  STRATEGY_OPTIONS,
  NEW_EXTRACTIONS_SAMPLE,
  cloneJSON,
  checkRequiredConjunct,
  buildWhenWithSeqAtom,
  buildWhenWithoutSeqAtom,
  hasOtherConds,
  condxFlatRows,
  condxRowsSummary,
  condxRowAllPlaceholders,
  condxOccurDrift,
  condxRowsError,
  condxFileRowInlineExtError,
  condxRefScopeProviders,
  condxRowsAllDirTarget,
  condxCaptures,
  condxNormalizeTpl,
  condxNormalizeRows,
  condxRemoveRow,
  condxFindRowList,
  condxGroupWordOf,
  condxMoveRemapRefs,
  extractionCardView,
  condxRowExtSuffix,
  aggTypeOptions,
  aggTypeSpecOf,
  aggAvailableTracksText,
  aggRoleDisplay,
  aggRoleRequired,
  aggRoleShortLabel,
  aggProductionSummary,
  aggMetadataFileSummary,
  aggIdentityCandidates,
  aggIdentityWordOf,
  aggFormErrors,
  metadataFileFormErrors,
  GROUP_KEY_SOURCE_OPTIONS,
  AGG_ADDITION_GATING_NOTE,
  GROUP_DECL_UNBOUND_ERROR,
  aggAdditionGated as aggAdditionGatedOf,
  groupDeclBlockLabel,
  groupDeclOptionLabel,
  groupDeclIdentityParts,
  groupDeclPairing,
  groupDeclFromIdentity,
  groupDeclFromPairing,
  type CondxRowForm,
  type CondxRowNoMap,
  type CondxCaptureDecl,
  type CondxOccEntry,
  type ExtractionActionJSON,
  type IdentityPartForm,
  type AggregateProductionForm,
  type ContentBindingForm,
  type PairingKeyForm,
  type MetadataFileProductionForm,
  type GroupDeclForm,
  type GroupKeySource,
  type AggTypeSpec,
  type AggIdentityCandidate
} from './ruleFormModel'

// —— 入口载荷形态（父组件组装的普通对象） ——

// 档案规则条目最小投影（作用域附件通路：供给方候选过滤与供给方占位词声明取值
// 用——SubpageRules 把完整规则列表经 allRules prop 传入）。
interface RuleArchiveItem {
  id: string
  description: string
  enabled?: boolean
  category?: string
  conditions?: unknown
  extractions?: unknown
}

// rule 模式初始档案条目（SubpageRules 的 RuleEntryView 投影；conditions 为
// v4 条件行列表——v4 档案条目唯一条件载体，模式树不再落档；category/identity/
// aggregate 为聚合规则的三件声明（两级类别·阶段6 编辑面镜像的对端）；
// metadataFile 为规则级「元数据文件」产出（与聚合产出平级，元数据规则与聚合
// 规则均可携带——内容绑定已从 aggregate 迁出，读写一律走本键）。
interface RuleInput {
  id: string
  description: string
  source: string
  enabled: boolean
  isNew: boolean
  category?: string
  identity?: { row: number; capture: string }[] | null
  aggregate?: {
    workType: string
    tracks: { row: number; role: string }[]
    companionUse?: string
  } | null
  metadataFile?: {
    pairing?: { mode: string; identity?: { row: number; capture: string }[] } | null
    contentBindings?: { row: number; dimension: string }[]
  } | null
  conditions?: unknown
  extractions: unknown
  synthesis?: { dimPolicies?: { dimension: string; strategy: string }[] | null; roles?: { source: string; roleName: string }[] | null }
}

// setAdjust 模式的集规则草稿（SetRulePanel 的 SetRuleSuggestion 结构投影）。
interface ClueView {
  ref: string
  kind: string
  label: string
  value: string
  ordinal?: number
}

interface SuggestionInput {
  setName?: string
  shape?: string
  summary?: string
  mappings?: { dimension: string; from: string }[]
  rules?: { id: string; description: string; pattern?: string }[]
  failed?: boolean
  failReason?: string
  clues?: ClueView[]
  defaultCollectionSegRefs?: string[]
  defaultVariableSegRefs?: string[]
  defaultSeqSegRef?: string
  defaultMemberOnly?: boolean
  defaultAuthorClueRef?: string
}

const props = defineProps<{
  state: boolean
  mode: 'rule' | 'setAdjust'
  rule?: RuleInput | null
  suggestion?: SuggestionInput | null
  initialAssigns?: Record<string, string>
  initialMemberOnly?: boolean
  // 档案全量规则列表（作用域附件通路：供给方候选与供给方占位词声明的数据源，
  // SubpageRules 传入）
  allRules?: RuleArchiveItem[] | null
  // 资源类型注册表投影（聚合规则分轨入口的类型清单：内置 6 类+插件自定义类型，
  // SubpageRules 自规则管理视图透传；缺席=类型步不可用）
  aggTypes?: AggTypeSpec[] | null
  // 父组件回填的保存失败文案（后端构造期校验拒绝等）：弹窗内呈现、不关窗
  saveError?: string
  saving?: boolean
  // —— 命中预览（E 件·剧本三）：父组件持有的预览通路状态 ——
  // previewAvailable 预览可用性（父组件按库根会话在场性与识别快照态判定，
  // false＝按钮禁用态+按钮行下方说明小字；打开弹窗时即已可判定帧3）；
  // previewing 等待态（按钮文案转「预览中…」不可再点）；previewResult 预览
  // 应答（null＝不显示结果卡）；previewError 失败文案（后端同源降级文案等）。
  previewAvailable?: boolean
  previewing?: boolean
  previewResult?: RulePreviewView | null
  previewError?: string
}>()

// rule 模式保存载荷（完整规则条目 JSON——父组件原样进 saveRule；conditions
// 为 v4 条件行（目录行可多段/挂作用域附件），pattern 不再发送；category/
// identity/aggregate 为聚合规则的类别与两件声明（元数据规则三者缺省——
// omitempty 落档零残留）。metadataFile=规则级「元数据文件」产出（内容绑定+
// 配对键声明；元数据规则与聚合规则均可携带，未加产出=undefined 键不落档=
// omitempty 镜像）。
interface RuleSavePayload {
  id: string
  description: string
  source: string
  enabled: boolean
  category?: string
  identity?: { row: number; capture: string }[]
  aggregate?: {
    workType: string
    tracks: { row: number; role: string }[]
    companionUse?: string
  } | null
  metadataFile?: {
    pairing?: { mode: string; identity?: { row: number; capture: string }[] }
    contentBindings?: { row: number; dimension: string }[]
  }
  conditions: unknown
  extractions: unknown
  synthesis: { dimPolicies: { dimension: string; strategy: string }[]; roles: { source: string; roleName: string }[] }
}

// setAdjust 模式保存载荷（指派结果——父组件换算为 saveWorkSetRuleAdjust 载荷）。
interface SetAdjustSavePayload {
  assigns: Record<string, string>
  memberOnly: boolean
}

// previewRule 应答的命中样例（rulesPreviewSample 形态）：文件名+所在目录+该草稿
// 规则产出的声明；result 缺省＝纯门组合退化形态——仅文件名+目录。
interface RulePreviewSampleView {
  fileName: string
  dirRelKey?: string
  result?: string
}

// previewRule 应答的命中预览（rulesPreviewResult 形态）：命中数/求值面文件总数/
// 样例（至多 3 条，父组件回填、结果卡呈现）。
interface RulePreviewView {
  hitCount: number
  hitTotal: number
  samples: RulePreviewSampleView[]
}

const emit = defineEmits<{
  (e: 'update:state', value: boolean): void
  (e: 'save', payload: RuleSavePayload | SetAdjustSavePayload): void
  // 命中预览请求（payload 由与保存同一条草稿构造路径产出）与结果卡 [×] 关闭
  (e: 'preview', payload: RuleSavePayload): void
  (e: 'preview-close'): void
  (e: 'dirty-change', dirty: boolean): void
}>()

const visible = computed({
  get: (): boolean => props.state,
  set: (value: boolean) => emit('update:state', value)
})

const sourceLabels: Record<string, string> = {
  builtin: '内置',
  manual: '手写',
  qna: '泛化',
  adjudication: '裁决'
}

const headerTitle = computed((): string => {
  if (props.mode === 'rule') {
    if (props.rule && props.rule.isNew) return '新建规则'
    return '编辑规则'
  }
  return '调整规则'
})

const headerNote = computed((): string => {
  if (props.mode === 'rule') {
    if (!props.rule || props.rule.isNew) return '来源：手写（manual）· 保存时自动生成 ID'
    return `${props.rule.id} · 来源：${sourceLabels[props.rule.source] || props.rule.source}（不可改）`
  }
  return `线索逐项指派 · 保存即入档${props.suggestion && props.suggestion.setName ? ` · ${props.suggestion.setName}` : ''}`
})

// —— rule 模式编辑态 ——

// 提取段行模型：when 为原样透传的条件树，seqOnly 为「仅序列成员产出」勾选
//（保存时并入/移除「群组序号 ≥ 1」原子，其余手写条件原样保留）；row 为
// capture 来源行号（v4：占位词捕获按「行N·{词}」寻址，1 起）；refRule 为
// refCapture 来源的供给方规则 ID（本规则目录行作用域附件所引供给方；
// refRule/row/captureName 三元寻址）。
interface ExtractionRow {
  dimension: string
  from: string
  captureName: string
  captureRow: number
  refRule: string
  constant: string
  scanScope: string
  roleName: string
  seqOnly: boolean
  when: unknown
}

const descInput = ref('')
const enabledInput = ref(true)
const roleRows = ref<{ source: string; roleName: string }[]>([])
const dimRows = ref<{ dimension: string; strategy: string }[]>([])
// 识别段结构化编辑模型（v4 条件行）：行列表为编辑态（行前接词+行级非+Condx
// 或括号子组占位），保存时整表直落 conditions（内核按行序编译为模式树）。
const condxRows = ref<CondxRowForm[]>([])
// 条件编辑弹窗态：目标行（行树内引用）、全局先序行号、edit/add（空行=新增态——
// 首步条件种类选择）
const condxRowDialogState = ref(false)
const condxRowEditTarget = ref<CondxRowForm | null>(null)
const condxRowEditNo = ref(1)
const condxRowEditMode = ref<'edit' | 'add'>('edit')

// —— 三子句编辑态（决策9/10·终审重梳轮）：分组声明与三产出（类别步退役——
//    落档类别=保存时推导：有聚合成作品产出=aggregate） ——

// 分组声明（分组区编辑态；声明=编辑面形态，保存时映射到既有键字段——聚合产出
// 绑定→identity、元数据文件产出绑定→pairing，内核 schema 零改动）
const groupDecls = ref<GroupDeclForm[]>([])
let groupDeclUidSeq = 1

// 产出的分组绑定（uid → groupDecls 条目；null=未绑定——聚合成作品未绑=保存闸门
// 拦；元数据文件未绑且无聚合产出=保存闸门拦）
const aggGroupingUid = ref<number | null>(null)
const mfGroupingUid = ref<number | null>(null)

// 分轨入口（AggregateProduction 表单形态；null=尚未加「聚合成作品」产出）
const aggProduction = ref<AggregateProductionForm | null>(null)

// 「元数据文件」产出（规则级独立落档位，与「聚合成作品」平级；null=尚未加该
// 产出——「+ 元数据文件」入口）
const metadataFileProduction = ref<MetadataFileProductionForm | null>(null)

// 按声明 uid 取声明（缺席=null）
function groupDeclByUid(uid: number | null): GroupDeclForm | null {
  if (uid == null) return null
  return groupDecls.value.find((d) => d.uid === uid) || null
}

// 聚合产出的分组绑定候选（非 directory 形态——所在目录对聚合产出不可绑：聚合
// 作品键=标识/群键，目录成组走目录捕获）+ 序列行虚拟项（选定即物化为声明，
// D 帧4 gating 的序列行解锁路径）
const bindableGroupOptions = computed<{ value: string; label: string }[]>(() => {
  const out: { value: string; label: string }[] = []
  for (const d of groupDecls.value) {
    if (d.keySource === 'directory') continue
    out.push({ value: `decl:${d.uid}`, label: groupDeclOptionLabel(d) })
  }
  const declaredSeqRows = new Set(
    groupDecls.value.filter((d) => d.keySource === 'seqKey').map((d) => (d.parts[0] ? d.parts[0].row : -1))
  )
  for (const s of seqRowOptions.value) {
    if (declaredSeqRows.has(s.row)) continue
    out.push({ value: `seq:${s.row}`, label: `◇ 按序列群键分组` })
  }
  return out
})

// 序列行清单（序列群键声明与虚拟项的候选源）
const seqRowOptions = computed<{ row: number; text: string }[]>(() => {
  const out: { row: number; text: string }[] = []
  for (const f of condxFlatRows(activeCondxRows.value)) {
    if (f.condx && f.condx.target === 'seq') {
      const tpl = f.condx.format.mode === 'contains'
        ? `含「${f.condx.format.word || '?'}」`
        : condxNormalizeTpl(f.condx.format.tpl || '')
      out.push({ row: f.row, text: `${tpl}${condxRowExtSuffix(f.condx)}` })
    }
  }
  return out
})

// 「+ 聚合成作品」gating（D 帧4：无分组声明且无序列行→置灰附因）
const aggAdditionGated = computed((): boolean =>
  aggAdditionGatedOf(groupDecls.value, activeCondxRows.value)
)

// 未绑定声明的保存拦截（微型闸门·未锚微文案）
const groupDeclErrors = computed((): string[] => {
  const errs: string[] = []
  const bound = new Set<number>()
  if (aggProduction.value && aggGroupingUid.value != null) bound.add(aggGroupingUid.value)
  if (metadataFileProduction.value && mfGroupingUid.value != null) bound.add(mfGroupingUid.value)
  // 并用形态（聚合在场）下元数据文件可省略分组绑定（分组随聚合并组）——
  // 未绑定的声明仍一律拦截
  if (groupDecls.value.some((d) => !bound.has(d.uid))) errs.push(GROUP_DECL_UNBOUND_ERROR)
  return errs
})

// 产出计数（三产出各计一条——提取行数+元数据文件+聚合成作品）
const productionCount = computed((): number =>
  extractionRows.value.length + (metadataFileProduction.value ? 1 : 0) + (aggProduction.value ? 1 : 0)
)

// 提取元数据汇总卡文案（D 帧3「作者←{作者} 标题←{标题}」）
const extractionSummaryText = computed((): string =>
  extractionRows.value.map((r) => extractionSummarySeg(r)).join(' ')
)

// 单条提取行的汇总段：「维度←来源摘要」（capture={词}、constant=常量值、其余=来源短词）
function extractionSummarySeg(row: ExtractionRow): string {
  const dim = row.dimension || '?'
  let src = row.from || '?'
  if (row.from === 'capture') src = `{${aggIdentityWordOf(row.captureName) || row.captureName}}`
  else if (row.from === 'constant' || row.from === 'adjudication') src = row.constant || '常量'
  return `${dim}←${src}`
}

// 聚合绑定的分组声明展开为 identity 部分（保存序列化与闸门共用）
const aggIdentityParts = computed((): IdentityPartForm[] => {
  const parts = groupDeclIdentityParts(groupDeclByUid(aggGroupingUid.value))
  return parts || []
})

// 标识/分组键候选分组（按条件行号）：文件名捕获/目录行捕获/序列行群键同列
// （分组声明弹窗的键部分选择与聚合闸门候选同源）
const identityGroups = computed<{ row: number; candidates: AggIdentityCandidate[] }[]>(() => {
  const out: { row: number; candidates: AggIdentityCandidate[] }[] = []
  for (const c of aggIdentityCandidates(activeCondxRows.value)) {
    const last = out[out.length - 1]
    if (last && last.row === c.row) last.candidates.push(c)
    else out.push({ row: c.row, candidates: [c] })
  }
  return out
})

// 聚合面保存闸门错误（纯函数派生；保存与预览共用——identity=绑定声明的展开）
const aggErrors = computed((): string[] =>
  aggFormErrors(activeCondxRows.value, aggIdentityParts.value, aggProduction.value, props.aggTypes || [])
)

// 产出卡摘要（「聚合成作品 · 视频（主体+字幕轨）」）
const aggCardText = computed((): string => aggProductionSummary(props.aggTypes || [], aggProduction.value))

// —— 聚合成作品弹窗（分组绑定+两步：类型→轨道分配）与分组声明弹窗 ——

const trackDialogState = ref(false)
const trackDraft = ref<AggregateProductionForm | null>(null)
// 草稿的分组绑定（uid 声明引用；虚拟序列项选定即物化为声明）
const trackDraftGroupingUid = ref<number | null>(null)

// 类型下拉清单（滤除无结构角色类型；投影缺席=空清单→类型步不可用提示）
const trackTypeOptions = computed(() => aggTypeOptions(props.aggTypes || []))

// 草稿当前类型的规约（未选类型=null）
const trackSpec = computed((): AggTypeSpec | null =>
  trackDraft.value && trackDraft.value.workType ? aggTypeSpecOf(props.aggTypes || [], trackDraft.value.workType) : null
)

// 角色下拉条目（类型规约序——可用轨清单同序呈现）
const trackRoleOptions = computed((): string[] => {
  const spec = trackSpec.value
  if (!spec) return []
  return (spec.roles || []).map((r) => (r ? r.storeType : '')).filter((s) => s)
})

// 轨道分配行：各条件行（模板+扩展名谓词的整体形态）与其当前角色
const trackAssignRows = computed<{ no: number; text: string }[]>(() => {
  const out: { no: number; text: string }[] = []
  for (const f of condxFlatRows(activeCondxRows.value)) {
    const tpl = f.condx.format.mode === 'contains'
      ? `含「${f.condx.format.word || '?'}」`
      : condxNormalizeTpl(f.condx.format.tpl || '')
    out.push({ no: f.row, text: `${tpl}${condxRowExtSuffix(f.condx)}` })
  }
  return out
})

// 草稿分组绑定的下拉取值（decl:uid=既有声明；seq:N=虚拟序列群键项）
const trackGroupingValue = computed((): string => {
  const d = groupDeclByUid(trackDraftGroupingUid.value)
  if (d) return `decl:${d.uid}`
  return ''
})

// 分组绑定改选（虚拟序列项=物化为 seqKey 声明后绑定）
function setTrackGrouping(value: string): void {
  if (!value) {
    trackDraftGroupingUid.value = null
    return
  }
  if (value.startsWith('decl:')) {
    trackDraftGroupingUid.value = Number(value.slice(5)) || null
    return
  }
  if (value.startsWith('seq:')) {
    const row = Number(value.slice(4)) || 1
    const decl: GroupDeclForm = { uid: groupDeclUidSeq++, keySource: 'seqKey', parts: [{ row, capture: 'groupKey' }] }
    groupDecls.value.push(decl)
    trackDraftGroupingUid.value = decl.uid
  }
}

// 弹窗级错误（分组未绑/类型未选/未覆盖必含角色——确定闸门呈现）
const trackDialogError = computed((): string => {
  const draft = trackDraft.value
  if (!draft) return ''
  if (!groupDeclByUid(trackDraftGroupingUid.value)) return '分组未绑定——作品标识=绑定的分组，请先在分组区加分组声明（或在分组下拉选序列群键）'
  if (!draft.workType) return '作品类型未选择（类型先行：类型规约决定可用轨与角色基数）'
  const spec = aggTypeSpecOf(props.aggTypes || [], draft.workType)
  if (!spec) return `作品类型「${draft.workType}」不在资源类型注册表（可能已随插件停用或卸载注销）`
  const covered = (draft.tracks || []).some((ta) => ta && aggRoleRequired(spec, ta.role))
  if (!covered) return '分轨入口未覆盖必含角色（类型规约 Min≥1 的角色）——主导文件无从判定，请把某条件行分到必含轨'
  return ''
})

// 开窗（产出卡〔编辑〕/「+ 聚合成作品」）：深拷贝草稿编辑，确定才回写
function openTrackDialog(): void {
  if (aggAdditionGated.value && !aggProduction.value) return
  trackDraft.value = aggProduction.value
    ? cloneJSON(aggProduction.value)
    : { workType: '', tracks: [], companionUse: 'track' }
  trackDraftGroupingUid.value = aggGroupingUid.value
  trackDialogState.value = true
}

// 类型选定（下拉）：清轨道分配并把首个必含角色自动落给行 1（用户随后可逐行
// 改选——角色下拉只出该类型可用轨）
function onTrackTypeChange(type: string): void {
  const draft = trackDraft.value
  if (!draft) return
  draft.workType = type
  const spec = aggTypeSpecOf(props.aggTypes || [], type)
  const required = (spec && spec.roles ? spec.roles : []).find((r) => r && r.min >= 1)
  draft.tracks = required ? [{ row: 1, role: required.storeType }] : []
}

// 单条件行角色改选（清空=移除分配：该行只作分组键源、不入轨）
function trackRoleOf(no: number): string {
  const draft = trackDraft.value
  const hit = draft && draft.tracks ? draft.tracks.find((t) => t && t.row === no) : undefined
  return hit ? hit.role : ''
}

function setTrackRole(no: number, role: string): void {
  const draft = trackDraft.value
  if (!draft) return
  if (!draft.tracks) draft.tracks = []
  const idx = draft.tracks.findIndex((t) => t && t.row === no)
  if (!role) {
    if (idx >= 0) draft.tracks.splice(idx, 1)
    return
  }
  if (idx >= 0) draft.tracks[idx] = { row: no, role }
  else draft.tracks.push({ row: no, role })
}

// 〔确定〕：草稿过分组/类型/必含闸门后就地回写产出卡与分组绑定；不过不关窗
//（错误行内在场）
function confirmTrackDialog(): void {
  if (trackDialogError.value) return
  if (!trackDraft.value) return
  aggProduction.value = cloneJSON(trackDraft.value)
  aggGroupingUid.value = trackDraftGroupingUid.value
  trackDialogState.value = false
}

// 产出卡〔移除〕：去聚合产出（分组声明不动——其他产出可能仍绑定）
function removeAggregateProduction(): void {
  aggProduction.value = null
  aggGroupingUid.value = null
  trackDialogState.value = false
  trackDraft.value = null
}

// —— 分组声明弹窗（键源+键，D 帧1） ——

const groupDialogState = ref(false)
const groupDialogEditIdx = ref(-1)
const groupDraft = ref<{ keySource: GroupKeySource; parts: IdentityPartForm[] } | null>(null)

// 「+ 加分组」：缺省键源=匹配行捕获、预置首个候选部分
function addGroupDecl(): void {
  const first = aggIdentityCandidates(activeCondxRows.value)[0]
  groupDraft.value = {
    keySource: 'capture',
    parts: [first ? { row: first.row, capture: first.group } : { row: 1, capture: '' }]
  }
  groupDialogEditIdx.value = -1
  groupDialogState.value = true
}

// 声明块〔编辑〕：草稿深拷贝（uid 不动——绑定引用稳定）
function editGroupDecl(idx: number): void {
  const d = groupDecls.value[idx]
  if (!d) return
  groupDraft.value = { keySource: d.keySource, parts: cloneJSON(d.parts || []) }
  groupDialogEditIdx.value = idx
  groupDialogState.value = true
}

// 声明块〔删〕：解除引用它的产出绑定（聚合产出绑定被解=其闸门拦补绑）
function removeGroupDecl(idx: number): void {
  const d = groupDecls.value[idx]
  if (!d) return
  if (aggGroupingUid.value === d.uid) {
    aggGroupingUid.value = null
    removeAggregateProduction()
  }
  if (mfGroupingUid.value === d.uid) mfGroupingUid.value = null
  groupDecls.value.splice(idx, 1)
}

// 键源切换：按源形态重置键部分（capture=首个候选；seqKey=首个序列行；directory=空）
function onGroupKeySourceChange(): void {
  const draft = groupDraft.value
  if (!draft) return
  if (draft.keySource === 'directory') {
    draft.parts = []
    return
  }
  if (draft.keySource === 'seqKey') {
    const first = seqRowOptions.value[0]
    draft.parts = [first ? { row: first.row, capture: 'groupKey' } : { row: 1, capture: 'groupKey' }]
    return
  }
  const cand = aggIdentityCandidates(activeCondxRows.value)[0]
  draft.parts = [cand ? { row: cand.row, capture: cand.group } : { row: 1, capture: '' }]
}

// 键部分取值拆装（值编码 行号/捕获名，从左拆一刀）
function setGroupDraftPart(pi: number, value: string): void {
  const idx = value.indexOf('/')
  if (idx < 0) return
  const part = groupDraft.value && groupDraft.value.parts[pi]
  if (!part) return
  part.row = Number(value.slice(0, idx)) || 1
  part.capture = value.slice(idx + 1)
}

// 加一部分：缺省选中首个候选
function addGroupDraftPart(): void {
  const draft = groupDraft.value
  if (!draft) return
  const first = aggIdentityCandidates(activeCondxRows.value)[0]
  draft.parts.push(first ? { row: first.row, capture: first.group } : { row: 1, capture: '' })
}

// 序列群键行选定
function setGroupDraftSeqRow(value: string): void {
  const draft = groupDraft.value
  if (!draft) return
  const row = Number(value) || 1
  draft.parts = [{ row, capture: 'groupKey' }]
}

// 〔确定〕：键合法（capture=至少一部分且已选捕获；seqKey=已选序列行）才回写
function confirmGroupDecl(): void {
  const draft = groupDraft.value
  if (!draft) return
  if (draft.keySource === 'capture') {
    const parts = draft.parts.filter((p) => p && p.capture)
    if (!parts.length) return
    draft.parts = parts
  } else if (draft.keySource === 'seqKey') {
    if (!draft.parts.length || !draft.parts[0].capture) return
  }
  if (groupDialogEditIdx.value >= 0) {
    const d = groupDecls.value[groupDialogEditIdx.value]
    if (d) {
      d.keySource = draft.keySource
      d.parts = cloneJSON(draft.parts)
    }
  } else {
    groupDecls.value.push({ uid: groupDeclUidSeq++, keySource: draft.keySource, parts: cloneJSON(draft.parts) })
  }
  groupDialogState.value = false
}

// —— 元数据文件弹窗（元数据文件映射与聚合解耦·剧本A 帧1/帧2·三子句定稿轮）：
//    「元数据文件」产出的唯一指派面（内容绑定+分组绑定，无聚合产出也可开窗指派） ——

const metadataDialogState = ref(false)

// 弹窗行清单（与条件行区同源）：行号+模板形态（剧本A 帧2 行文案「第 N 行 ·
// {作品}_{序号}（.png）」——扩展名附加条件括注尾接、模板字面扩展名直拼）+
// 轨道短词（按当前分轨入口的角色分配；空=未分配轨道；聚合产出在场才随行标注）
const metadataDialogRows = computed<{ no: number; text: string; track: string }[]>(() => {
  const roleByRow = new Map<number, string>()
  if (aggProduction.value && aggProduction.value.tracks) {
    for (const t of aggProduction.value.tracks) {
      if (t) roleByRow.set(t.row, t.role)
    }
  }
  return trackAssignRows.value.map((t) => ({
    no: t.no,
    text: metadataRowText(t.no),
    track: roleByRow.has(t.no) ? aggRoleShortLabel(roleByRow.get(t.no) || '') : ''
  }))
})

// 弹窗行模板形态：模板直拼+扩展名附加条件括注（fileExt 谓词承载的扩展名与
// 模板字面扩展名在行文案上以括注区分）
function metadataRowText(no: number): string {
  for (const f of condxFlatRows(activeCondxRows.value)) {
    if (f.row !== no) continue
    const tpl = f.condx.format.mode === 'contains'
      ? `含「${f.condx.format.word || '?'}」`
      : condxNormalizeTpl(f.condx.format.tpl || '')
    const ext = condxRowExtSuffix(f.condx)
    return ext ? `${tpl}（${ext}）` : tpl
  }
  return ''
}

// 分组绑定下拉条目（A 帧2「分组：[◇ 按 {作品}+{序号} 分组▾]」+按所在目录选项）：
// 分组区声明（capture/seqKey/directory 全列）+ 无 directory 声明时的虚拟
//「按所在目录」项（选定即物化为 directory 声明）
const metadataGroupOptions = computed<{ value: string; label: string }[]>(() => {
  const out: { value: string; label: string }[] = []
  const hasDirectory = groupDecls.value.some((d) => d.keySource === 'directory')
  for (const d of groupDecls.value) {
    out.push({ value: `decl:${d.uid}`, label: groupDeclOptionLabel(d) })
  }
  if (!hasDirectory) out.push({ value: 'dir:', label: '◇ 按所在目录分组' })
  return out
})

// 当前分组绑定的下拉取值（decl:uid；未绑定=''——并用形态可省略）
const mfBoundGroupValue = computed((): string => {
  const d = groupDeclByUid(mfGroupingUid.value)
  return d ? `decl:${d.uid}` : ''
})

// 产出区入口摘要（帧1「元数据文件（0）」/帧2 系统回应「元数据文件（1）：第 2 行
// → 描述」，多目标连写「描述+标签」）
const metadataFileSummary = computed((): string =>
  aggMetadataFileSummary(metadataFileProduction.value ? metadataFileProduction.value.contentBindings : null)
)

// 「元数据文件」产出保存闸门（纯函数派生；保存与预览共用——与内核
// validateMetadataFileProduction/validatePairingKey 同口径；pairing=绑定声明的展开，
// 未绑定且聚合在场=undefined〔并用可省略——分组随聚合并组〕）
const metadataFileErrors = computed((): string[] => {
  const mf = metadataFileProduction.value
  const pairingDerived = mf ? groupDeclPairing(groupDeclByUid(mfGroupingUid.value)) : undefined
  return metadataFileFormErrors(
    activeCondxRows.value,
    mf ? { pairing: pairingDerived, contentBindings: mf.contentBindings } : null,
    !!aggProduction.value
  )
})

function openMetadataDialog(): void {
  metadataDialogState.value = true
}

// 条目卡〔移除〕（产出移除入口——开窗清空勾选确定即移除）
function removeMetadataFile(): void {
  metadataFileProduction.value = null
}

// 弹窗〔确定〕：绑定清单与分组绑定回写「元数据文件」产出（空清单=移除该
// 产出——该规则行为回到现状；非空=落规则级 metadataFile 键——pairing 由绑定
// 声明序列化映射）；虚拟「按所在目录」选定即物化为 directory 声明
function onMetadataConfirm(bindings: ContentBindingForm[], groupingValue: string): void {
  if (groupingValue === 'dir:') {
    const decl: GroupDeclForm = { uid: groupDeclUidSeq++, keySource: 'directory', parts: [] }
    groupDecls.value.push(decl)
    mfGroupingUid.value = decl.uid
  } else if (groupingValue.startsWith('decl:')) {
    mfGroupingUid.value = Number(groupingValue.slice(5)) || null
  } else {
    mfGroupingUid.value = null
  }
  metadataFileProduction.value = bindings.length
    ? { contentBindings: cloneJSON(bindings) }
    : null
}

// 条件卡〔编辑〕/〔+ 加条件〕开窗（顶层卡与组框内成员卡同一链路——成员卡经组框
// 内编辑器逐层上浮至此）：空行按新增态开（首步条件种类选择）、已设内容按编辑态直达
function openCondxRowEdit(row: CondxRowForm, no: number): void {
  condxRowEditTarget.value = row
  condxRowEditNo.value = no
  condxRowEditMode.value = row.condition ? 'edit' : 'add'
  condxRowDialogState.value = true
}

// 条件编辑弹窗〔确定〕：草稿就地回写行对象（negated/condition——行对象身份
// 不变，行号引用与换位链不受影响；行树深侦听触发位次漂移刷新与摘要重算）
function onCondxRowEditConfirm(row: CondxRowForm, draft: CondxRowForm): void {
  row.negated = draft.negated
  row.condition = draft.condition
}

// 条件编辑弹窗〔取消〕：新增态移除开窗时追加的空行（全树定位——组内追加的行
// 同链移除；规整移除=删 0 位行后新首行接词清零）；编辑态不动（草稿即弃）
function onCondxRowEditCancel(row: CondxRowForm): void {
  if (condxRowEditMode.value !== 'add') return
  const hit = condxFindRowList(condxRows.value, row)
  if (hit) condxRemoveRow(hit.list, hit.idx)
}
const condxText = ref('')
const condxJsonMode = ref(false)
const condxError = ref('')
// JSON 模式下最近一次成功解析的结构（占位词清单与摘要跟随它；解析失败保留旧值）
const condxParsed = ref<CondxRowForm[] | null>(null)
const extractionRows = ref<ExtractionRow[]>([])
const extractionsText = ref('')
const extractionsJsonMode = ref(false)
const extractionsError = ref('')

// JSON 解析（成功/失败带原文错误信息——V8 报文自带行/列位置）
function tryParseJSON(text: string): { ok: boolean; value?: unknown; error?: string } {
  try {
    return { ok: true, value: JSON.parse(text) }
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
}

// 任意 JSON → 条件行列表（缺 condition/group 的坏形态保结构不丢：原样进
// JSON 高级模式，避免结构化缺省静默改写档案内容）
function normalizeCondxRows(raw: unknown): CondxRowForm[] | null {
  if (Array.isArray(raw)) return cloneJSON(raw) as CondxRowForm[]
  return null
}

// 提取动作 JSON → 行模型（seqOnly 由 when 内容派生；scan 行的范围默认 path；
// capture 行的行号缺省 1——v4 前期档案单行即行 1）
function actionToRow(action: ExtractionActionJSON): ExtractionRow {
  const when = action.when ?? null
  return {
    dimension: action.dimension || '',
    from: action.from || 'capture',
    captureName: action.captureName || '',
    captureRow: action.row || 1,
    refRule: action.refRule || '',
    constant: action.constant || '',
    scanScope: action.scanScope || 'path',
    roleName: action.roleName || '',
    seqOnly: checkRequiredConjunct(when),
    when: when
  }
}

// capture 行的「行N·{词}」下拉取值拆装
function setRowCapture(row: ExtractionRow, value: string): void {
  const idx = value.indexOf('/')
  if (idx < 0) return
  row.captureRow = Number(value.slice(0, idx)) || 1
  row.captureName = value.slice(idx + 1)
}

// refCapture 行的「供给方·占位词」下拉取值拆装（值编码 refRule/row/captureName
// 三元——从右侧拆两刀，兼容含「/」的规则 ID）
function setRowRefCapture(row: ExtractionRow, value: string): void {
  const rowIdx = value.lastIndexOf('/')
  if (rowIdx < 0) return
  const nameIdx = value.lastIndexOf('/', rowIdx - 1)
  if (nameIdx < 0) return
  row.refRule = value.slice(0, nameIdx)
  row.captureRow = Number(value.slice(nameIdx + 1, rowIdx)) || 1
  row.captureName = value.slice(rowIdx + 1)
}

// 行模型 → 提取动作 JSON（seqOnly 落到该行 when 的并入/移除；仅携带有值字段；
// scan 行=范围 + 角色透传，不带 when/捕获组/常量——内核校验拒绝面同构）
function rowsToActions(): ExtractionActionJSON[] {
  return extractionRows.value.map((row) => {
    if (row.from === 'scan') {
      const scanAction: ExtractionActionJSON = {
        dimension: row.dimension,
        from: 'scan',
        scanScope: row.scanScope || 'path'
      }
      if (row.roleName) scanAction.roleName = row.roleName
      return scanAction
    }
    const action: ExtractionActionJSON = { dimension: row.dimension, from: row.from }
    if (row.from === 'capture') {
      action.captureName = row.captureName
      // v4 寻址=行号+占位词（行 1 缺省不落 JSON，与内核 omitempty 同口径）
      if (row.captureRow > 1) action.row = row.captureRow
    }
    if (row.from === 'refCapture') {
      // refCapture 寻址=供给方规则+行号+占位词三元（refRule 往返不丢；行 1 缺省同口径）
      action.refRule = row.refRule
      action.captureName = row.captureName
      if (row.captureRow > 1) action.row = row.captureRow
    }
    if (row.from === 'constant' || row.from === 'adjudication') action.constant = row.constant
    if (row.roleName) action.roleName = row.roleName
    const when = row.seqOnly ? buildWhenWithSeqAtom(row.when) : buildWhenWithoutSeqAtom(row.when)
    if (when != null) action.when = when
    return action
  })
}

// —— 识别段联动：上半改行/改模板 → 占位词清单与提取段下拉即时跟随 ——

// 当前生效的条件行表（JSON 模式=最近一次成功解析；结构化模式=行列表）
const activeCondxRows = computed<CondxRowForm[]>(() =>
  condxJsonMode.value && condxParsed.value ? condxParsed.value : condxRows.value
)

const captureList = computed(() => condxCaptures(activeCondxRows.value))

// 已声明的「行N·组名」寻址集合（提取行引用校验；captureName=英文组名——
// 内核寻址口径）
const declaredCaptures = computed<Set<string>>(() => new Set(captureList.value.map((c) => `${c.row}/${c.group}`)))

// —— 作用域附件通路（剧本2）：供给方候选、供给方占位词声明与提取行接线校验 ——

function refRuleById(ruleId: string): RuleArchiveItem | undefined {
  return (props.allRules || []).find((r) => r.id === ruleId)
}

function refRuleLabel(ruleId: string): string {
  const hit = refRuleById(ruleId)
  return hit ? hit.description || hit.id : ruleId
}

// 本规则条件行引用的供给方规则 ID 集（refScope 行扫描——按出现序去重；JSON
// 模式=最近一次成功解析的行表）
const refScopeProviderIds = computed<string[]>(() => condxRefScopeProviders(activeCondxRows.value))

// 供给方候选（剧本2 帧2 / 剧本A 帧3-4）：「库根」恒在列、排首位、缺省选中
//（sentinel，无供给方求值/悬空）；其后只列纯目录规则（只含目录行的规则——
// 定群/聚合/混合目标天然出局）、启用、非本条自身；停用/悬空（conditions
// 缺席）排除。候选可携带任意作用域附件（规则来源或库根，嵌套多层皆入列）；
// 环不在前端拦截，保存期由后端构造校验报错兜底（弹窗保留、可改选重存）。
// 规则项文本=规则描述；无纯目录规则时仅库根一项（帧4，无空态红字）。
const refScopeCandidates = computed<{ id: string; label: string }[]>(() => {
  const selfId = props.rule && !props.rule.isNew ? props.rule.id : ''
  const rules = (props.allRules || [])
    .filter((r) => {
      if (!r || r.id === selfId || r.enabled === false || r.category === 'aggregate') return false
      if (!Array.isArray(r.conditions) || !r.conditions.length) return false
      return condxRowsAllDirTarget(r.conditions)
    })
    .map((r) => ({ id: r.id, label: r.description || r.id }))
  return [{ id: REFSCOPE_LIBRARY_ROOT, label: '库根' }, ...rules]
})

// 供给方·占位词分组：按供给方顺序分组列出各供给方上半声明的「第 N 行 · {词}」；
// 供给方当前档案声明为真源——供给方改过模板后旧引用自然不在列。
// 重复段词（多值产出）不进本下拉：接线禁引，与后端口径一致。
interface RefCaptureGroup {
  ruleId: string
  label: string
  index: number
  captures: CondxCaptureDecl[]
}

const refCaptureGroups = computed<RefCaptureGroup[]>(() => {
  const out: RefCaptureGroup[] = []
  refScopeProviderIds.value.forEach((ruleId, i) => {
    const member = refRuleById(ruleId)
    if (!member) return
    const rows = Array.isArray(member.conditions) ? (member.conditions as CondxRowForm[]) : []
    out.push({ ruleId, label: member.description || member.id, index: i + 1, captures: condxCaptures(rows, { includeRepeat: false }) })
  })
  return out
})

// 已声明的「成员/行N/组名」寻址集合（refCapture 提取行引用校验）
const declaredRefCaptures = computed<Set<string>>(() =>
  new Set(refCaptureGroups.value.flatMap((g) => g.captures.map((c) => `${g.ruleId}/${c.row}/${c.group}`)))
)

// 英文组名 → 中文显示词（含位次化组名）统一走 ruleFormModel 的 condxGroupWordOf
// （失效行残影与产出卡摘要共用同一呈现口径）。

// 失效占位词残影选项（剧本三·帧1）：行值不在成员现声明中时补一枚禁用选项，
// 让行上仍按「成员 · 第 N 行 · {词}」呈现且不可重选
const refCaptureStaleOptions = computed<{ value: string; label: string }[]>(() => {
  const declared = declaredRefCaptures.value
  const out: { value: string; label: string }[] = []
  const seen = new Set<string>()
  for (const row of extractionRows.value) {
    if (row.from !== 'refCapture' || !row.refRule || !row.captureName) continue
    const key = `${row.refRule}/${row.captureRow}/${row.captureName}`
    if (declared.has(key) || seen.has(key)) continue
    seen.add(key)
    out.push({ value: key, label: `${refRuleLabel(row.refRule)} · 第 ${row.captureRow} 行 · {${condxGroupWordOf(row.captureName)}}` })
  }
  return out
})

// 首个「供给方捕获」行位置（提取段「来自供给方规则」分组头的插入点）。
function isFirstRefCaptureRow(index: number): boolean {
  if (extractionRows.value[index]?.from !== 'refCapture') return false
  return extractionRows.value.slice(0, index).every((r) => r.from !== 'refCapture')
}

// 镜像缺失提示：按供给方×占位词组名聚合——供给方在多行声明同组词、本表供给方
// 捕获行只接线其中部分行时，在该组首条接线行下提示（未接线行形态命中时取不到
// 值）；供给方声明与接线数据均来自本地。
function refCaptureMirrorWarn(row: ExtractionRow, index: number): string {
  if (row.from !== 'refCapture' || !row.refRule || !row.captureName) return ''
  const group = refCaptureGroups.value.find((g) => g.ruleId === row.refRule)
  if (!group) return ''
  const declared = [...new Set(group.captures.filter((c) => c.group === row.captureName).map((c) => c.row))].sort((a, b) => a - b)
  if (declared.length < 2) return ''
  const wired = extractionRows.value
    .filter((r) => r.from === 'refCapture' && r.refRule === row.refRule && r.captureName === row.captureName)
    .map((r) => r.captureRow)
  const firstWired = extractionRows.value.findIndex(
    (r) => r.from === 'refCapture' && r.refRule === row.refRule && r.captureName === row.captureName
  )
  if (firstWired >= 0 && firstWired < index) return ''
  const wiredUnique = [...new Set(wired)].sort((a, b) => a - b)
  const missing = declared.filter((n) => wiredUnique.indexOf(n) < 0)
  if (!wiredUnique.length || !missing.length) return ''
  const list = (nums: number[]): string => nums.join('、')
  return `⚠ {${condxGroupWordOf(row.captureName)}}在供给方第 ${list(declared)} 行都声明，你只接线了第 ${list(wiredUnique)} 行——第 ${list(missing)} 行形态命中时取不到值`
}

// —— 产出卡展开态与摘要 ——

// 产出卡展开态：〔编辑〕切换；行级校验不过的行缺省展开（保存闸门呈现不因收起
// 而不可见），显式收起后不再自动展开
const outOpenMap = ref<Record<number, boolean>>({})
function outIsOpen(i: number): boolean {
  if (outOpenMap.value[i] !== undefined) return outOpenMap.value[i]
  const row = extractionRows.value[i]
  return !!row && !!extractionRowHint(row)
}
function toggleOutOpen(i: number): void {
  outOpenMap.value[i] = !outIsOpen(i)
}

// 产出卡摘要文本（纯函数派生：维度 ← 来源摘要+角色/仅序列成员小缀）
function outCardText(row: ExtractionRow): string {
  const v = extractionCardView(row, refRuleLabel)
  return `${v.dimLabel} ← ${v.fromText}${v.roleSuffix}`
}

// 条件卡片区计数（条件行数——括号子组递归计）
const condxCardCount = computed(() => condxFlatRows(condxRows.value).length)

// 条件行换位落地（moved 事件，顶层编辑器与组框内编辑器逐层上浮两路）：行表已在编辑器内
// 就地内容交换，此处按随行上抛的全局行号映射改写三类行号引用（决策3）——提取行
// captureRow 与生效条件 when 树捕获操作数、条件树值校验 againstRow（递归子组）；
// refCapture 行号指向供给方规则不改写（供给方行号与消费方行表无关）。提取区
// JSON 模式下移动按钮整体禁用（自决2），无 JSON 文本失同步面。
function onCondxRowsMoved(map: CondxRowNoMap): void {
  condxMoveRemapRefs(condxRows.value, extractionRows.value, map)
  // 位次漂移基线随行换位重挂（行号键改写）：换位属「行号重排」，不在漂移检测
  // 「行集稳定、行内模板变更」覆盖面内——基线与既有漂移项按映射改键，避免合法
  // 换位被误报为「模板位次已变化」（引用已由上方重映射跟随，无需用户复核）。
  const snapshot = new Map<number, CondxOccEntry[]>()
  for (const [no, entries] of occurSnapshot) {
    const to = map[no]
    snapshot.set(to === undefined ? no : to, entries)
  }
  occurSnapshot = snapshot
  const drift: Record<number, { group: string; word: string }[]> = {}
  for (const no of Object.keys(occurDrift.value)) {
    const to = map[Number(no)]
    if (to !== undefined) drift[to] = occurDrift.value[Number(no)]
  }
  occurDrift.value = drift
}

// 摘要行（一行人话）：先序递归拼接（组=圆括号包成员、接词随行、行号=全局先序
// 号）；目录行多段模板与作用域附件按直译句呈现（refLabelOf 解析供给方完整
// 描述——决策7 前端镜像，与弹窗行卡/后端 patternSummary 同源）。
const condxSummary = computed((): string => {
  const flat = condxFlatRows(activeCondxRows.value)
  if (!flat.length) return '（无条件行）'
  return condxRowsSummary(activeCondxRows.value, { refLabelOf: refRuleLabel })
})

// —— 位次漂移 diff（风险1 缓解）：编辑会话内模板变更前后位次表对照 ——

// 上一快照（按展平行号存位次表）与漂移结果（行号→受影响组名+显示词）；弹窗
// 打开时复位基线，模板每变一次即 diff+刷新。
let occurSnapshot = new Map<number, CondxOccEntry[]>()
const occurDrift = ref<Record<number, { group: string; word: string }[]>>({})

function refreshOccurDrift(rows: CondxRowForm[]): void {
  const flat = condxFlatRows(rows)
  const next = new Map<number, CondxOccEntry[]>()
  for (const f of flat) next.set(f.row, condxRowAllPlaceholders(f.condx))
  // 行集结构变化（加/删行导致展平行号重排）时放弃对照、重建基线——位次漂移
  // 检测只覆盖「行集稳定、行内模板变更」的编辑形态（风险1 主形态）；行号重排
  // 的引用失效由既有「已不在上半声明」校验兜底。
  const stable = flat.length === occurSnapshot.size && flat.every((f) => occurSnapshot.has(f.row))
  const drift: Record<number, { group: string; word: string }[]> = {}
  if (stable) {
    for (const f of flat) {
      const before = occurSnapshot.get(f.row)
      if (!before) continue
      const after = next.get(f.row) || []
      if (condxOccurEntriesEqual(before, after)) {
        // 本行位次表未变：沿用既有漂移提示——跨多次无关编辑保留，直至该行
        // 模板再变（重算）或漂移被修复（重算为空）。
        if (occurDrift.value[f.row]) drift[f.row] = occurDrift.value[f.row]
        continue
      }
      const hit = condxOccurDrift(before, after)
      if (hit.length) drift[f.row] = hit
    }
  }
  occurSnapshot = next
  occurDrift.value = drift
}

// 位次表是否未变（签名序列逐一相等——位次表由模板确定性地推导，签名同序即同表）。
function condxOccurEntriesEqual(a: CondxOccEntry[], b: CondxOccEntry[]): boolean {
  return a.length === b.length && a.every((e, i) => e.sig === b[i].sig)
}

watch(
  () => [condxRows.value, condxParsed.value] as const,
  () => refreshOccurDrift(activeCondxRows.value),
  { deep: true }
)

// 全部漂移项展平（提取行 when/接线文本反查用）。
function allOccurDrift(): { row: number; group: string; word: string }[] {
  const out: { row: number; group: string; word: string }[] = []
  for (const rowNo of Object.keys(occurDrift.value)) {
    for (const d of occurDrift.value[Number(rowNo)] || []) {
      out.push({ row: Number(rowNo), group: d.group, word: d.word })
    }
  }
  return out
}

// 提取行的漂移提示：capture 引用受影响组名，或 when 条件（含组合接线限定名
// rN.组名）文本命中受影响组名（词边界防 tag 命中 tag__2）。
// 行内漂移提示（模板面/值校验面）随条件行区块迁入 CondxRowsEditor，经
// occurDrift prop 读取本组件刷新结果。
function extractionDriftWarn(row: ExtractionRow): string {
  const drift = allOccurDrift()
  if (!drift.length) return ''
  if (row.from === 'capture' && row.captureName) {
    const hit = drift.find((d) => d.row === row.captureRow && d.group === row.captureName)
    if (hit) return `⚠ 第 ${hit.row} 行模板位次已变化：{${hit.word}} 的指向可能已漂移，请复核本行引用`
  }
  if (row.when != null) {
    const text = JSON.stringify(row.when)
    for (const d of drift) {
      const re = new RegExp(`(^|[^A-Za-z0-9_])${d.group.replace(/[$()*+.?[\\\]^{|}]/g, (s) => `\\${s}`)}(?![A-Za-z0-9_])`)
      if (re.test(text)) {
        return `⚠ 生效条件/接线引用的占位词位次已漂移（第 ${d.row} 行 {${d.word}}），请复核本行条件`
      }
    }
  }
  return ''
}

// JSON 模式下边输入边解析：合法即刷新清单/摘要，非法就地显示错误位置、不丢输入
watch(condxText, () => {
  if (!condxJsonMode.value) return
  const parsed = tryParseJSON(condxText.value)
  if (parsed.ok && Array.isArray(parsed.value)) {
    condxParsed.value = parsed.value as CondxRowForm[]
    condxError.value = condxRowsError(condxParsed.value)
  } else if (parsed.ok) {
    condxError.value = '顶层须为 JSON 数组（条件行列表）'
  } else {
    condxError.value = parsed.error || '解析失败'
  }
})

// 进入 JSON 高级模式：现行行表序列化为原文（双向同步——结构化改动能带进 JSON）
function toggleCondxJson(): void {
  if (condxJsonMode.value) {
    // 切回结构化：须先通过解析（失败留在 JSON 模式、显示错误位置、不丢输入）
    const parsed = tryParseJSON(condxText.value)
    if (!parsed.ok || !Array.isArray(parsed.value)) {
      condxError.value = parsed.ok ? '顶层须为 JSON 数组（条件行列表）' : parsed.error || '解析失败'
      return
    }
    const rows = normalizeCondxRows(parsed.value)
    if (!rows) {
      condxError.value = '顶层须为 JSON 数组（条件行列表）'
      return
    }
    // 切回结构化：旧词 {键}/{作品} 书写归一显示为 {公共段}/{作品标识}（保存即落档新词）
    condxRows.value = condxNormalizeRows(rows)
    condxParsed.value = null
    condxJsonMode.value = false
    condxError.value = condxRowsError(condxRows.value)
    return
  }
  condxText.value = JSON.stringify(activeCondxRows.value, null, 2)
  condxJsonMode.value = true
  const parsed = tryParseJSON(condxText.value)
  if (parsed.ok && Array.isArray(parsed.value)) {
    condxParsed.value = parsed.value as CondxRowForm[]
    condxError.value = condxRowsError(condxParsed.value)
  }
}

// —— 提取段行：增删与行级就地校验（组引用存在性随上半联动） ——

function extractionRowHint(row: ExtractionRow): string {
  if (!row.dimension) return '须选择维度'
  if (row.from === 'capture') {
    if (!row.captureName) return '须选择占位词'
    if (!declaredCaptures.value.has(`${row.captureRow}/${row.captureName}`)) return '引用的占位词已不在上半声明'
  }
  if (row.from === 'refCapture') {
    if (!row.refRule || !row.captureName) return '须选择供给方·占位词'
    if (refScopeProviderIds.value.indexOf(row.refRule) < 0) return '供给方规则不在本规则目录行的作用域附件中'
    // 失效＝供给方改过模板/供给方悬空——行级标红锚文案
    if (!declaredRefCaptures.value.has(`${row.refRule}/${row.captureRow}/${row.captureName}`)) {
      return '引用的占位词已不在供给方规则声明'
    }
  }
  if (row.from === 'scan') {
    // 帧五可裁点①：仅库内集合维度可扫描（作者/标签/作品集）。
    if (!SCAN_DIMENSIONS.includes(row.dimension)) return '贪婪识别仅支持 作者/标签/作品集（无库内集合的维度不适用）'
    if (!SCAN_SCOPE_OPTIONS.some((s) => s.value === (row.scanScope || 'path'))) return '须选择扫描范围'
  }
  if ((row.from === 'constant' || row.from === 'adjudication') && !row.constant.trim()) {
    return '常量来源须填写常量值'
  }
  return ''
}

// scan 行的维度下拉只列可扫描维度（作者/标签/作品集）。
function dimOptionsOf(row: ExtractionRow): { value: string; label: string }[] {
  if (row.from === 'scan') return DIMENSION_OPTIONS.filter((d) => SCAN_DIMENSIONS.includes(d.value))
  return DIMENSION_OPTIONS
}

// scan 行说明文案里的库内集合名（作者/标签/作品集）。
function scanDimLabel(dim: string): string {
  const hit = DIMENSION_OPTIONS.find((d) => d.value === dim)
  return hit ? hit.label : dim
}

function extractionRowHasOtherConds(row: ExtractionRow): boolean {
  return hasOtherConds(row.when)
}

function addExtractionRow(): void {
  extractionRows.value.push({
    dimension: 'author',
    from: 'capture',
    captureName: '',
    captureRow: 1,
    refRule: '',
    constant: '',
    scanScope: 'path',
    roleName: '',
    seqOnly: false,
    when: null
  })
}

function removeExtractionRow(index: number): void {
  extractionRows.value.splice(index, 1)
}

// 提取段 JSON 高级模式（复杂 when 树直通原文）：进入=行序列化（seqOnly 已并入），
// 切回=解析重建行（seqOnly 从 when 重新派生，往返不丢）
function toggleExtractionsJson(): void {
  if (extractionsJsonMode.value) {
    const parsed = tryParseJSON(extractionsText.value)
    if (!parsed.ok) {
      extractionsError.value = parsed.error || '解析失败'
      return
    }
    if (!Array.isArray(parsed.value)) {
      extractionsError.value = '顶层须为 JSON 数组'
      return
    }
    extractionRows.value = (parsed.value as ExtractionActionJSON[]).map(actionToRow)
    extractionsJsonMode.value = false
    extractionsError.value = ''
    return
  }
  extractionsText.value = JSON.stringify(rowsToActions(), null, 2)
  extractionsJsonMode.value = true
  extractionsError.value = ''
}

// —— setAdjust 模式指派态（剧本E 语义随迁） ——

type Dest = '' | 'author' | 'title' | 'tag' | 'collection' | 'variable' | 'anchor' | 'seq'

const assignMap = ref<Record<string, string>>({})
const memberOnly = ref(true)
let adjustBaseline = ''

function dimLabel(dim: string): string {
  switch (dim) {
    case 'author': return '作者'
    case 'title': return '作品名'
    case 'collection': return '作品集'
    case 'tag': return '标签'
    default: return dim
  }
}

function clueValueText(c: ClueView): string {
  return c.kind === 'dirLevelStripped' ? `${c.label}（剥壳 → ${c.value}）` : c.value
}

// 线索指派可选目的地（按线索类别）——与原调整面同款：序号段→序号/不用；序列
//（setSeqKey，剧本E——值=群键/公共前缀，与文件名段行同权限，指集名即「集名=
// 群键」）→集名/作者/作品名/标签/不用；文件名段→集名组合/可变/作者/作品名/
// 标签/不用；目录名分段→集名组合/可变/作者/不用（可勾多段连续组合）；目录层
//（含剥壳变体）→作者/作品名/标签/目录锚定/不用；库根名/其余→维度/不用。
function destOptionsOf(c: ClueView): { value: Dest; label: string }[] {
  if (c.kind === 'setSeq') {
    return [
      { value: 'seq', label: '序号' },
      { value: '', label: '不用' }
    ]
  }
  if (c.kind === 'setSeqKey') {
    return [
      { value: '', label: '不用' },
      { value: 'collection', label: '集名' },
      { value: 'author', label: '作者' },
      { value: 'title', label: '作品名' },
      { value: 'tag', label: '标签' }
    ]
  }
  if (c.kind === 'setSeg') {
    return [
      { value: '', label: '不用' },
      { value: 'collection', label: '集名组合' },
      { value: 'variable', label: '可变' },
      { value: 'author', label: '作者' },
      { value: 'title', label: '作品名' },
      { value: 'tag', label: '标签' }
    ]
  }
  if (c.kind === 'setDirSeg') {
    return [
      { value: '', label: '不用' },
      { value: 'collection', label: '集名组合' },
      { value: 'variable', label: '可变' },
      { value: 'author', label: '作者' }
    ]
  }
  if (c.kind === 'dirLevel' || c.kind === 'dirLevelStripped') {
    return [
      { value: '', label: '不用' },
      { value: 'author', label: '作者' },
      { value: 'title', label: '作品名' },
      { value: 'tag', label: '标签' },
      { value: 'anchor', label: '目录锚定' }
    ]
  }
  return [
    { value: '', label: '不用' },
    { value: 'author', label: '作者' },
    { value: 'title', label: '作品名' },
    { value: 'tag', label: '标签' }
  ]
}

// 单选目的地互斥：作者/作品名/目录锚定/序号各至多一条线索持有——新指派挤掉
// 旧持有（集名组合/标签/可变允许多条）。
const SINGLE_DESTS: Dest[] = ['author', 'title', 'anchor', 'seq']

function onAssignChange(clueRef: string): void {
  const dest = assignMap.value[clueRef]
  if (!dest || SINGLE_DESTS.indexOf(dest as Dest) < 0) return
  for (const c of (props.suggestion && props.suggestion.clues) || []) {
    if (c.ref !== clueRef && assignMap.value[c.ref] === dest) assignMap.value[c.ref] = ''
  }
}

// 行级就地提示（剧本E 失败路径；保存校验照旧由后端拒绝）：①同一维度（集名）指了
// 互斥多来源（序列/文件名段/目录段混指）→「一个维度只能有一个来源」；②目录段
// 组合不连续 →「目录段组合须为连续段」。
function collectionFamilyOf(c: ClueView): string {
  if (c.kind === 'setSeqKey') return 'seq'
  if (c.kind === 'setDirSeg') return 'dir'
  if (c.kind === 'setSeg') return 'file'
  return 'other'
}

function clueRowHint(c: ClueView): string {
  const sugg = props.suggestion
  if (!sugg || (assignMap.value[c.ref] || '') !== 'collection') return ''
  const holders = (sugg.clues || []).filter((x) => (assignMap.value[x.ref] || '') === 'collection')
  const families = new Set(holders.map(collectionFamilyOf))
  if (families.size > 1) return '一个维度只能有一个来源'
  if (collectionFamilyOf(c) === 'dir') {
    const ords = holders
      .filter((x) => x.kind === 'setDirSeg')
      .map((x) => x.ordinal || 0)
      .sort((a, b) => a - b)
    for (let i = 1; i < ords.length; i++) {
      if (ords[i] !== ords[i - 1] + 1) return '目录段组合须为连续段'
    }
  }
  return ''
}

// 还原默认指派（剧本E 默认跟随识别形态的应答预填值）
function resetAssignDefaults(): void {
  const sugg = props.suggestion
  if (!sugg) return
  const next: Record<string, string> = {}
  for (const c of sugg.clues || []) next[c.ref] = ''
  for (const ref of sugg.defaultCollectionSegRefs || []) next[ref] = 'collection'
  for (const ref of sugg.defaultVariableSegRefs || []) next[ref] = 'variable'
  if (sugg.defaultSeqSegRef) next[sugg.defaultSeqSegRef] = 'seq'
  if (sugg.defaultAuthorClueRef) next[sugg.defaultAuthorClueRef] = 'author'
  assignMap.value = next
  memberOnly.value = sugg.defaultMemberOnly === true
}

// 调整面脏标志（供父组件切换选中目标前的未保存确认）
const adjustDirty = computed((): boolean => {
  if (props.mode !== 'setAdjust') return false
  return JSON.stringify({ a: assignMap.value, m: memberOnly.value }) !== adjustBaseline
})

watch(adjustDirty, (dirty) => emit('dirty-change', dirty))

// —— 打开时取初始值（destroy-on-close：每次打开重建，取消后再开回上次保存态） ——

function initFromProps(): void {
  if (props.mode === 'rule') {
    const rule = props.rule
    descInput.value = (rule && rule.description) || ''
    enabledInput.value = rule ? rule.enabled !== false : true
    const synth = (rule && rule.synthesis) || {}
    roleRows.value = ((synth.roles || []) as { source: string; roleName: string }[]).map((r) => ({
      source: r.source || 'filename',
      roleName: r.roleName || ''
    }))
    dimRows.value = ((synth.dimPolicies || []) as { dimension: string; strategy: string }[]).map((d) => ({
      dimension: d.dimension || 'title',
      strategy: d.strategy || 'union'
    }))
    // 显式复位跨开关存留的编辑态（AutoHeightDialog destroy-on-close 只销毁插槽
    // 内容，本组件 setup 态跨开关存留——每个字段都须重置后再按初值分支）
    condxJsonMode.value = false
    condxText.value = ''
    // 三子句编辑态复位：分组声明与产出绑定从档案键字段还原为声明（聚合
    // identity→声明绑定聚合产出；映射 pairing→声明绑定元数据文件产出——两类
    // 类别全部内容装进统一表单，类别步退役）
    groupDecls.value = []
    groupDeclUidSeq = 1
    aggGroupingUid.value = null
    mfGroupingUid.value = null
    const initIdentity = rule && Array.isArray(rule.identity) ? rule.identity : []
    const initIdentityParts = initIdentity
      .filter((p): p is { row: number; capture: string } => !!p && typeof p.row === 'number' && typeof p.capture === 'string')
      .map((p) => ({ row: p.row || 1, capture: p.capture }))
    const initAgg = rule && rule.aggregate ? rule.aggregate : null
    aggProduction.value = initAgg
      ? {
          workType: initAgg.workType || '',
          tracks: (Array.isArray(initAgg.tracks) ? initAgg.tracks : [])
            .filter((t): t is { row: number; role: string } => !!t && typeof t.row === 'number' && typeof t.role === 'string')
            .map((t) => ({ row: t.row, role: t.role })),
          companionUse: initAgg.companionUse || 'track'
        }
      : null
    if (aggProduction.value) {
      const decl = groupDeclFromIdentity(initIdentityParts, groupDeclUidSeq++)
      if (decl) {
        groupDecls.value.push(decl)
        aggGroupingUid.value = decl.uid
      }
    }
    // 「元数据文件」产出装载（规则级 metadataFile 键——内容绑定已从 aggregate
    // 迁出，读写一律走本键；旧档 aggregate.contentBindings 由内核载入归一到
    // 本键后经 rulesView 投影，前端不读旧键；pairing 还原为分组声明并绑定）
    const initMf = rule && rule.metadataFile ? rule.metadataFile : null
    metadataFileProduction.value = initMf
      ? {
          contentBindings: Array.isArray(initMf.contentBindings)
            ? initMf.contentBindings
                .filter(
                  (b): b is { row: number; dimension: string } =>
                    !!b && typeof b.row === 'number' && typeof b.dimension === 'string'
                )
                .map((b) => ({ row: b.row, dimension: b.dimension }))
            : undefined
        }
      : null
    if (metadataFileProduction.value && initMf && initMf.pairing &&
        (initMf.pairing.mode === 'identity' || initMf.pairing.mode === 'directory')) {
      const parts = initMf.pairing.mode === 'identity' && Array.isArray(initMf.pairing.identity)
        ? initMf.pairing.identity
            .filter((p): p is { row: number; capture: string } => !!p && typeof p.row === 'number' && typeof p.capture === 'string')
            .map((p) => ({ row: p.row || 1, capture: p.capture }))
        : []
      const decl = groupDeclFromPairing(
        { mode: initMf.pairing.mode as PairingKeyForm['mode'], identity: parts.length ? parts : undefined },
        groupDeclUidSeq++
      )
      if (decl) {
        groupDecls.value.push(decl)
        mfGroupingUid.value = decl.uid
      }
    }
    trackDialogState.value = false
    trackDraft.value = null
    metadataDialogState.value = false
    const defaultRows = (): CondxRowForm[] => [
      {
        connector: '',
        negated: false,
        condition: { target: 'file', format: { mode: 'template', tpl: '【{作者}】{标题}' }, badges: [], attach: [] },
        group: null
      }
    ]
    if (!rule || (rule.isNew && !Array.isArray(rule.conditions))) {
      condxRows.value = defaultRows()
    } else if (Array.isArray(rule.conditions)) {
      // 旧档 {键}/{作品} 模板归一显示为 {公共段}/{作品标识}（与保存载荷构造同一归一函数——
      // 弹窗所见即保存所得）；复制条目（isNew+携内容）同路装载
      condxRows.value = condxNormalizeRows(cloneJSON(rule.conditions)) as CondxRowForm[]
    } else if (rule.conditions != null) {
      // 历史坏档（conditions 非数组）：原文进 JSON 高级模式，避免结构化缺省
      // 形态静默改写档案内容（旧词归一为语义等价换词，不属于形态改写）
      condxJsonMode.value = true
      condxText.value = JSON.stringify(condxNormalizeRows(rule.conditions), null, 2)
    } else {
      // 无 conditions 键的历史残档：缺省模板行起步（既有行为不变）
      condxRows.value = defaultRows()
    }
    condxError.value = ''
    condxParsed.value = null
    if (condxJsonMode.value) {
      const parsed = tryParseJSON(condxText.value)
      if (parsed.ok && Array.isArray(parsed.value)) {
        condxParsed.value = parsed.value as CondxRowForm[]
      } else {
        condxError.value = parsed.ok ? '顶层须为 JSON 数组（条件行列表）' : parsed.error || '解析失败'
      }
    } else {
      condxError.value = condxRowsError(condxRows.value)
    }
    // 位次漂移基线复位（编辑会话内模板变更前后对照的「前」快照——打开弹窗
    // 取值不算变更，不产生漂移提示）
    occurSnapshot = new Map(condxFlatRows(activeCondxRows.value).map((f) => [f.row, condxRowAllPlaceholders(f.condx)] as const))
    occurDrift.value = {}
    extractionsJsonMode.value = false
    extractionsText.value = ''
    // 复制条目（isNew+携提取数组）装载其内容；新建缺省样例起步
    const rawExtractions: unknown = !rule
      ? NEW_EXTRACTIONS_SAMPLE
      : rule.isNew && !Array.isArray(rule.extractions)
        ? NEW_EXTRACTIONS_SAMPLE
        : rule.extractions
    if (Array.isArray(rawExtractions)) {
      extractionRows.value = (cloneJSON(rawExtractions) as ExtractionActionJSON[]).map(actionToRow)
    } else {
      // 非数组历史坏档：保原文进 JSON 高级模式（缺省=空规则，结构化空行集）
      extractionRows.value = []
      if (rawExtractions != null) {
        extractionsJsonMode.value = true
        extractionsText.value = JSON.stringify(rawExtractions, null, 2)
      }
    }
    extractionsError.value = ''
    return
  }
  assignMap.value = cloneJSON(props.initialAssigns || {})
  memberOnly.value = props.initialMemberOnly === true
  adjustBaseline = JSON.stringify({ a: assignMap.value, m: memberOnly.value })
  emit('dirty-change', false)
}

watch(
  () => props.state,
  (open) => {
    if (open) initFromProps()
    // 主弹窗关闭时随关条件编辑弹窗（避免对已不在场行树的陈旧草稿会话做回写）
    else condxRowDialogState.value = false
  },
  { immediate: true }
)

// —— 保存 / 取消 ——

function cancel(): void {
  emit('update:state', false)
}

// rule 模式保存载荷构造（含全部闸门：JSON 合法性/条件行完整/提取行校验——
// 不过就地标红返回 null）。保存与命中预览共用本构造路径：预览载荷即
// 归一后的保存草稿（{公共段}/{作品标识} 旧词归一在 conditions 构造层执行），非原始表单值——
// 预览=保存后口径。
function buildRulePayload(): RuleSavePayload | null {
  // JSON 合法性闸门：识别/提取任一停在 JSON 模式且解析不过 → 不关窗不保存
  if (condxJsonMode.value) {
    if (condxError.value || !condxParsed.value) return null
  }
  // 条件行完整性闸门（v4·全类别统一）：行表校验不过 → 行内已标红，此处拦住
  if (!condxJsonMode.value) {
    const rowErr = condxRowsError(condxRows.value)
    if (rowErr) {
      condxError.value = rowErr
      return null
    }
  }
  // 聚合面闸门（聚合产出在场才适用）：分组绑定（标识=绑定声明展开）/分轨
  // 校验不过 → 产出区已逐条标红，此处拦住不保存
  if (aggProduction.value && aggErrors.value.length) return null
  // 「元数据文件」产出闸门：分组绑定/绑定行非法 → 产出区已逐条标红
  if (metadataFileErrors.value.length) return null
  // 未绑定分组声明拦截（微型闸门·三子句定稿轮）
  if (groupDeclErrors.value.length) return null
  // 模板尾内联扩展名守卫（决策8 甲案·与内核同口径）：文件名行模板对去扩展名
  // 的文件名求值，尾字面白名单扩展名恒不命中——保存拦截并指路「扩展名是」
  // 附加条件（结构化与 JSON 高级模式同拦）；聚合产出在场时由聚合面闸门
  //（aggFormErrors「文件条件行」措辞）先行，此处不覆盖。
  if (!aggProduction.value) {
    const metaTailExtErr = condxFileRowInlineExtError(
      condxJsonMode.value ? condxParsed.value ?? [] : condxRows.value
    )
    if (metaTailExtErr) {
      condxError.value = metaTailExtErr
      return null
    }
  }
  // 保存载荷构造层统一执行旧词归一（{键} → {公共段}、{作品} → {作品标识}，
  // 语义等价）：结构化与
  // JSON 高级模式同路径——JSON 手写 {键}/{作品} 保存亦落档新词
  const conditions = condxNormalizeRows(condxJsonMode.value ? condxParsed.value : condxRows.value)
  let extractions: unknown
  if (extractionsJsonMode.value) {
    const parsed = tryParseJSON(extractionsText.value)
    if (!parsed.ok) {
      extractionsError.value = parsed.error || '解析失败'
      return null
    }
    if (!Array.isArray(parsed.value)) {
      extractionsError.value = '顶层须为 JSON 数组'
      return null
    }
    extractionsError.value = ''
    // 占位词引用校验（JSON 模式）：引用未声明「行N·{词}」→ 行号化错误就地列出
    const problems: string[] = []
    ;(parsed.value as ExtractionActionJSON[]).forEach((action, i) => {
      if (action && action.from === 'capture' && action.captureName) {
        const rowNo = action.row || 1
        if (!declaredCaptures.value.has(`${rowNo}/${action.captureName}`)) {
          problems.push(`第 ${i + 1} 行引用占位词「第 ${rowNo} 行·{${action.captureName}}」未在匹配区声明`)
        }
      }
      if (action && action.from === 'refCapture') {
        const rowNo = action.row || 1
        const inProviders = refScopeProviderIds.value.indexOf(action.refRule || '') >= 0
        if (!action.refRule || !action.captureName || !inProviders || !declaredRefCaptures.value.has(`${action.refRule}/${rowNo}/${action.captureName}`)) {
          problems.push(`第 ${i + 1} 行：引用的占位词已不在供给方规则声明`)
        }
      }
    })
    if (problems.length) {
      extractionsError.value = problems.join('；')
      return null
    }
    extractions = parsed.value
  } else {
    // 结构化行：任一行级校验不过 → 行内已标红，此处拦住不保存
    if (extractionRows.value.some((row) => extractionRowHint(row))) return null
    extractions = rowsToActions()
  }
  const rule = props.rule
  // 落档类别推导（决策9：类别步退役——有「聚合成作品」产出=aggregate，否则
  // metadata 键不落档）；聚合声明三件（category/identity/aggregate）随推导落档，
  // identity=绑定分组声明的展开；提取动作与聚合并存（决策10 解除互斥）
  return {
    id: rule && !rule.isNew ? rule.id : '',
    description: descInput.value,
    source: (rule && rule.source) || 'manual',
    enabled: enabledInput.value,
    category: aggProduction.value ? 'aggregate' : undefined,
    identity: aggProduction.value
      ? aggIdentityParts.value.map((p) => ({ row: p.row, capture: p.capture }))
      : undefined,
    aggregate: aggProduction.value
      ? {
          workType: aggProduction.value.workType,
          tracks: aggProduction.value.tracks.map((t) => ({ row: t.row, role: t.role })),
          // 规则级处置维度恒归一 track（metadata 由行级指派合成表达——内核载入
          // 对规则级 metadata 拒载；前端表单不出该值）
          companionUse: 'track'
        }
      : undefined,
    metadataFile: buildMetadataFilePayload(),
    conditions: conditions,
    extractions: extractions,
    synthesis: {
      dimPolicies: dimRows.value.filter((d) => d.dimension && d.strategy),
      roles: roleRows.value.filter((r) => r.source && r.roleName)
    }
  }
}

// 「元数据文件」产出的保存载荷构造：未加产出=undefined 键不落档（omitempty
// 镜像）；配对键=绑定分组声明的序列化映射（capture/seqKey→identity 形态、
// directory→directory 形态、未绑定且聚合在场=不落键〔并用可省略〕）；绑定空
// 数组不落键（该形态由闸门拦，构造层兜底不产出空键）
function buildMetadataFilePayload(): RuleSavePayload['metadataFile'] {
  const mf = metadataFileProduction.value
  if (!mf) return undefined
  const pairing = groupDeclPairing(groupDeclByUid(mfGroupingUid.value))
  const contentBindings = (mf.contentBindings || []).map((b) => ({ row: b.row, dimension: b.dimension }))
  return { pairing, contentBindings }
}

function save(): void {
  if (props.mode === 'rule') {
    const payload = buildRulePayload()
    if (!payload) return
    emit('save', payload)
    return
  }
  // setAdjust：任一指派行级提示在场 → 拦住不保存（行内已标红）
  const sugg = props.suggestion
  if (sugg && (sugg.clues || []).some((c) => clueRowHint(c))) return
  emit('save', { assigns: assignMap.value, memberOnly: memberOnly.value })
}

// —— 命中预览（E 件·剧本三帧1-3）：按钮触发、不随表单实时联动；等待态/结果卡
// 与表单互不干扰，等待期间保存/取消行为保持既有 ——

// 预览可用性（帧3 判定）：父组件按库根会话在场性与识别快照态传入；缺省视为
// 可用（setAdjust 模式不渲染按钮，无消费面）。
const previewEnabled = computed((): boolean => props.previewAvailable !== false)

// 预览按钮文案（帧1/帧3 逐字）：等待态「预览中…」；禁用态「预览命中（不可用）」；
// 常态「预览命中」。
const previewButtonLabel = computed((): string => {
  if (props.previewing) return '预览中…'
  return previewEnabled.value ? '预览命中' : '预览命中（不可用）'
})

// 预览触发（帧1）：载荷走与保存同一条草稿构造路径（同一闸门与 {公共段}/{作品标识} 旧词归一）；
// 校验不过时不发请求（行内已标红、就地呈现）；等待/禁用态不可再点。
function preview(): void {
  if (props.previewing || !previewEnabled.value) return
  const payload = buildRulePayload()
  if (!payload) return
  emit('preview', payload)
}

// 结果卡关闭（帧2：可 [×] 关闭后改表单重试）——结果态归父组件，清空经事件上抛
function closePreview(): void {
  emit('preview-close')
}

// 样例行文案（帧2 模板「· {fileName}（{dirRelKey}）→ {result}」）：result 缺省
// （纯门组合退化形态）时仅文件名+目录，不渲染「→ 」尾巴。
function previewSampleLine(s: RulePreviewSampleView): string {
  const base = `· ${s.fileName}（${s.dirRelKey ?? ''}）`
  return s.result ? `${base}→ ${s.result}` : base
}
</script>

<style scoped>
/* 两段式规则编辑弹窗（剧本F）：主题令牌呈现（--app-* 家族） */
.li-red-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
  flex-wrap: wrap;
}

.li-red-title {
  font-size: 16px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-red-header-note {
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-red-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 2px 4px;
}

/* 用户终审反馈批（预览结果卡撑高弹窗→页面级原生滚动条）：结果卡在 footer 槽，
   而壳 AutoHeightDialog 的表单区封顶测量发生在 @open 时刻（卡未在场、footer 仅
   按钮行高），卡出现后 footer 增高不会反馈进壳的测量——弹窗被整体撑出视口、
   .el-overlay 出原生滚动条。此处卡在场时表单区同步收窄让位：弹窗总高 ≈
   头部+表单+按钮行+卡 ≤ 头部+（76vh-170px）+按钮行+min(24vh,220px) ≤ 100vh
   （170px≈头部+按钮行+弹窗内衬固定高度，含窄窗换行余量）。 */
.li-red-body--preview {
  max-height: calc(76vh - 170px);
}

.li-red-meta {
  display: flex;
  align-items: center;
  gap: 12px;
}

.li-red-meta .el-input {
  flex: 1;
}

.li-red-sec {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  padding: 12px;
}

.li-red-sec-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-red-sec-head .li-red-sec-title {
  margin-right: auto;
}

.li-red-sec-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
  display: flex;
  align-items: baseline;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-sec-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-red-summary {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-red-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-text-secondary);
}

.li-red-error {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-color-danger);
  word-break: break-all;
}

.li-red-label {
  font-size: 12px;
  color: var(--app-text-regular);
}

/* 捕获组声明清单（名称 + 来源正则） */
.li-red-caps {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: 1px dashed var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  padding: 8px 10px;
}

.li-red-cap-row {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.li-red-cap-name {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-color-primary);
}

/* 重复段词标注（多值产出）：提取段可引用、三面禁引的清单提示 */
.li-red-cap-multi {
  flex-shrink: 0;
  font-size: 11px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: var(--app-radius);
  border: 1px solid var(--app-border-color-light);
  color: var(--app-text-secondary);
}

.li-red-cap-expr {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: Consolas, 'Courier New', monospace;
}

.li-red-json-area :deep(textarea) {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
}

/* 产出卡（总览）：容器纵向承载恒定高度摘要行+可选展开编辑器；左缘 success 绿
   3px 竖条（与识别条件行主色蓝条色彩分野，行区域标识沿用） */
.li-out-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-left: 3px solid var(--el-color-success);
  border-radius: var(--app-radius);
  padding: 0 10px;
}

.li-out-card.is-invalid {
  border-color: var(--app-color-danger);
}

/* 产出卡展开编辑区：主行（维度←来源）+ 副行（角色/条件/剥壳提示）+ 行级提示 */
.li-out-editor {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-top: 1px dashed var(--app-border-color-lighter, var(--app-border-color-light));
  padding: 8px 0;
}

/* 产出卡摘要行首的「◇」记号（条目符号统一 ◇·D 帧3） */
.li-out-mark {
  flex: none;
  color: var(--app-text-secondary);
}

.li-red-ex-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-ex-dim {
  width: 110px;
}

.li-red-arrow {
  color: var(--app-text-secondary);
}

.li-red-ex-from {
  width: 170px;
}

.li-red-ex-capture {
  width: 200px;
}

.li-red-ex-const {
  width: 200px;
}

.li-red-ex-sub {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-strip-note {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-red-ex-scan {
  width: 150px;
}

.li-red-scan-note {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-red-ex-role {
  width: 130px;
}

.li-red-cond {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--app-text-regular);
  cursor: pointer;
}

.li-red-other {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-red-row-hint {
  margin: 0;
  font-size: 12px;
  color: var(--app-color-danger);
  word-break: break-all;
}

/* 合成策略（随旧表单随迁） */
.li-red-synth-head {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-red-synth-head > span {
  margin-right: auto;
}

.li-red-inline-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-red-inline-select {
  width: 140px;
}

.li-red-inline-input {
  flex: 1;
}

/* setAdjust：草稿映射与表达式呈现 */
.li-red-maps {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-red-map {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-size: 12px;
}

.li-red-map-dim {
  color: var(--app-text-primary);
  font-weight: 600;
}

.li-red-map-from {
  color: var(--app-text-secondary);
}

.li-red-drafts {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-red-draft {
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.li-red-draft-id {
  color: var(--app-color-primary);
  word-break: break-all;
}

.li-red-draft-desc {
  color: var(--app-text-secondary);
  word-break: break-all;
}

/* setAdjust：线索指派行（剧本E） */
.li-red-clue-table {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-red-clue-row {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex-wrap: wrap;
}

.li-red-clue-hint {
  flex-basis: 100%;
  font-size: 12px;
  color: var(--app-color-danger);
  word-break: break-all;
}

.li-red-clue-label {
  flex-shrink: 0;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-red-clue-value {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.li-red-clue-select {
  flex-shrink: 0;
  max-width: 110px;
  font-size: 12px;
  color: var(--app-text-regular);
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 1px 4px;
}

/* 底部动作区：保存错误文案（父组件回填）+ 动作按钮；换行承载命中预览结果卡与
   说明小字（整行元素 flex-basis 100%） */
.li-red-footer {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.li-red-save-error {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  color: var(--app-color-danger);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.li-red-footer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}

/* —— 命中预览（E 件·剧本三）：结果卡（帧2）+ 失败文案/禁用态说明小字（帧3） —— */

/* 结果卡：弹窗底部整行、按钮行上方；只读呈现与表单互不干扰；高度封顶
   min(24vh, 220px)（超高样例在卡内样例区滚动，不再撑高 footer/弹窗——
   与 .li-red-body--preview 表单区收窄配套，见其注释） */
.li-red-preview-card {
  flex-basis: 100%;
  display: flex;
  flex-direction: column;
  gap: 6px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 10px 12px;
  max-height: min(24vh, 220px);
}

/* 样例区自身内滚（SetRulePanel .li-sr-members 同款形态）；min-height:0 允许在
   卡封顶时收缩（flex 子项默认不缩破内容高） */
.li-red-preview-samples {
  min-height: 0;
  overflow-y: auto;
}

.li-red-preview-head {
  display: flex;
  align-items: center;
}

.li-red-preview-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-red-preview-close {
  margin-left: auto;
}

.li-red-preview-stat {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-red-preview-samples-label,
.li-red-preview-sample {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-text-regular);
  word-break: break-all;
}

.li-red-preview-note {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* 预览失败文案（RPC 失败/后端降级）与帧3 禁用态说明小字：按钮行下方整行 */
.li-red-preview-error {
  flex-basis: 100%;
  font-size: 12px;
  color: var(--app-color-danger);
  word-break: break-all;
}

.li-red-preview-unavailable {
  flex-basis: 100%;
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* 卡片总览区标题（「条件（N）」「产出（N）」） */
.li-red-zone-head {
  display: flex;
  align-items: center;
}

.li-red-zone-title {
  margin-right: auto;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

/* 引用行卡（与条件卡同款）：容器纵向承载恒定高度主行+可选展开区；左缘主色
   3px 竖条（识别条件行行区域标识沿用） */
.li-condx-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-left: 3px solid var(--el-color-primary);
  border-radius: var(--app-radius);
  padding: 0 10px;
}

.li-condx-card.is-invalid {
  border-color: var(--app-color-danger);
}

/* 卡片主行：恒定一行高（高度显式约束——卡片高度不随条件内部复杂度增长） */
.li-condx-card-line {
  height: 40px;
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex: none;
}

.li-condx-card-no {
  flex: none;
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-secondary);
}

/* 卡片摘要正文：单行截断（悬浮 title 看全文） */
.li-condx-card-tpl {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 卡片徽标（引用/✗/⚠ 等）：小色块 */
.li-condx-card-flag {
  flex: none;
  font-size: 11px;
  line-height: 1;
  padding: 3px 5px;
  border-radius: var(--app-radius);
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  color: var(--app-text-secondary);
  white-space: nowrap;
}

.li-condx-card-flag.is-ref {
  color: var(--app-color-primary);
  border-color: var(--app-color-primary);
}

.li-condx-card-flag.is-warn {
  color: var(--app-color-warning, var(--app-text-secondary));
  border-color: var(--app-color-warning, var(--app-border-color-light));
}

.li-condx-card-flag.is-err {
  color: var(--app-color-danger);
  border-color: var(--app-color-danger);
}

/* 卡片展开区（引用行卡成员明细/产出卡编辑器入口共用） */
.li-condx-card-more {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: flex-start;
  border-top: 1px dashed var(--app-border-color-lighter, var(--app-border-color-light));
  padding: 6px 0 8px;
}

.li-condx-del {
  margin-left: auto;
}

/* W4 引用行卡展开区：候选下拉两行（描述 + ID 小字） */
.li-ref-select {
  width: 100%;
  max-width: 420px;
}

.li-ref-opt {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.4;
  padding: 2px 0;
}

.li-ref-opt-desc {
  font-size: 13px;
  color: var(--app-text-primary);
}

.li-ref-opt-id {
  font-size: 11px;
  color: var(--app-text-secondary);
}

/* F：引用行下只读平铺成员条件行 */
.li-ref-flat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-left: 14px;
  padding-left: 10px;
  border-left: 2px solid var(--app-border-color-lighter, var(--app-border-color-light));
}

.li-ref-flat-line {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

/* D：引用行静态告警（✗ 单行序列零提取成员=红色警示）与提示（⚠ 成员独立产出） */
.li-ref-alert {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-color-danger);
  word-break: break-all;
}

/* D：提示级 ⚠（成员独立产出 / 镜像缺失） */
.li-ref-notice,
.li-red-mirror-warn {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-color-warning, var(--app-text-secondary));
  word-break: break-all;
}

/* F：提取段「来自供给方规则」分组头（转写提示小字常驻） */
.li-red-ref-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 4px 0;
}

.li-red-ref-group-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-regular);
}

.li-red-ref-group-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.7;
  color: var(--app-text-secondary);
}

/* —— 聚合规则编辑面（两级类别·剧本3）—— */

/* 类别选择行（新建首步）：[元数据规则] [聚合规则] 两枚可选按钮+类别说明小字 */
.li-red-cat {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-cat-note {
  font-size: 12px;
  color: var(--app-text-secondary);
}

/* 作品标识声明行：「作品标识:」标签+各部分下拉+删/加按钮（帧3 单行形态） */
.li-red-agg-identity {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-agg-ident {
  width: 200px;
}

/* 分轨入口弹窗：步骤标签（第一步·作品类型:/第二步·轨道分配:） */
.li-red-agg-step {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

/* 类型按钮区（第一步）：等宽卡片按钮，选中态主色描边（条件编辑弹窗目标卡同款
   形态——本组件 scoped 样式不跨组件，故本地同名再造） */
.li-red-types {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.li-red-type-card {
  flex: 1;
  min-width: 88px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 8px 10px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-red-type-card.is-active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

/* 轨道分配行：「第 N 行 · 模板+扩展名 → 角色下拉」（D 帧2） */
.li-red-agg-track {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

/* 分组绑定行（D 帧2「分组：[◇ 按 {作品}+{序号} 分组▾]」） */
.li-red-agg-grouping {
  margin-bottom: 4px;
}

.li-red-agg-role {
  width: 180px;
  flex: none;
}

/* 分轨入口弹窗体：纵向承载两步内容（弹窗壳高度封顶内滚） */
.li-red-track-body {
  max-height: 60vh;
  overflow-y: auto;
}

/* 分轨入口弹窗 footer 按钮行（恒两按钮、高度静态） */
.li-cred-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
