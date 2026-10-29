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

    <div class="li-red-body">
      <!-- ================= 入口①：档案规则编辑（SubpageRules「编辑/新建」） ================= -->
      <template v-if="mode === 'rule'">
        <!-- 规则元信息：描述 + 启停 -->
        <div class="li-red-meta">
          <el-input v-model="descInput" placeholder="规则描述（人话，列表呈现用）" clearable />
          <el-switch v-model="enabledInput" active-text="启用" inactive-text="停用" />
        </div>

        <!-- 上半「识别规则」（v4 条件行 · 三步漏斗）：目标四卡（文件/目录/序列/
             引用规则）→ 格式模板 → 附加条件+值级小标；行前 且/或 + 行级非 + 括号
             子组（子组走高级 JSON）；引用行恒置条件行上方、独立编号「引用 N」、
             不占条件行「第 N 行」（W4 规则组合引用） -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">识别规则<span class="li-red-sec-note">上半 · 决定规则命中哪些文件</span></h4>
            <el-button size="small" text @click="toggleCondxJson">
              {{ condxJsonMode ? '切回结构化编辑' : '高级（JSON）' }}
            </el-button>
          </div>
          <p class="li-red-summary">{{ condxSummary }}</p>

          <template v-if="!condxJsonMode">
            <p v-if="condxError" class="li-red-error">{{ condxError }}</p>
            <!-- 引用行（W4 剧本一·帧4/帧5）：一卡一行一条成员规则；恒置条件行上方、
                 独立编号「引用 N」、不带且/或；候选＝档案里其余非组合规则且非本条自身 -->
            <div
              v-for="(r, i) in refRows"
              :key="`ref${i}`"
              class="li-condx-row li-ref-row"
              :class="{ 'is-invalid': !r.ruleId }"
            >
              <div class="li-condx-row-head">
                <span class="li-ref-no">引用 {{ i + 1 }}{{ r.ruleId ? '' : '（新行）' }}</span>
                <span class="li-ref-desc">{{ refRowViews[i].desc }}</span>
                <el-button size="small" text type="danger" class="li-condx-del" @click="refRows.splice(i, 1)">删行</el-button>
              </div>
              <el-select
                v-if="!r.ruleId"
                :model-value="r.ruleId"
                size="small"
                placeholder="选择要引用的规则"
                class="li-ref-select"
                @update:model-value="r.ruleId = String($event)"
              >
                <el-option v-for="m in refCandidates" :key="m.id" :label="m.description || m.id" :value="m.id">
                  <div class="li-ref-opt">
                    <span class="li-ref-opt-desc">{{ m.description || m.id }}</span>
                    <span class="li-ref-opt-id">{{ m.id }}</span>
                  </div>
                </el-option>
              </el-select>
              <p v-if="!r.ruleId" class="li-red-row-hint">须选择要引用的规则</p>
              <!-- 引用行透出成员目标类别与形态数（B3）+ 只读平铺成员条件行（F）+
                   静态告警（D：单行序列零提取成员标红 ✗ / 成员独立产出提示 ⚠） -->
              <template v-if="r.ruleId">
                <div v-if="refRowViews[i].flatLines.length" class="li-ref-flat">
                  <p v-for="(line, li) in refRowViews[i].flatLines" :key="li" class="li-ref-flat-line">{{ line }}</p>
                </div>
                <p v-if="refRowViews[i].alert" class="li-ref-alert">{{ refRowViews[i].alert }}</p>
                <p v-if="refRowViews[i].notice" class="li-ref-notice">{{ refRowViews[i].notice }}</p>
              </template>
            </div>
            <div v-if="refRows.length && condxRows.length" class="li-ref-divider"></div>
            <div v-for="(row, i) in condxRows" :key="i" class="li-condx-row" :class="{ 'is-invalid': !!condxRowHint(i) }">
              <div class="li-condx-row-head">
                <el-select v-if="i > 0" v-model="row.connector" size="small" class="li-condx-conn">
                  <el-option label="且（都命中）" value="and" />
                  <el-option label="或（任一命中）" value="or" />
                </el-select>
                <label class="li-condx-neg"><input v-model="row.negated" type="checkbox"><span>非</span></label>
                <!-- 括号子组：结构化模式只读占位（嵌套编辑走高级 JSON），保存原样 -->
                <div v-if="row.group" class="li-condx-groupnote">括号子组（{{ row.group.length }} 行）——完整嵌套编辑请用「高级（JSON）」</div>
                <el-button v-else size="small" text type="danger" class="li-condx-del" @click="condxRows.splice(i, 1)">删行</el-button>
              </div>
              <template v-if="!row.group">
                <!-- Step 1 目标四卡（一次点选；第四卡＝W4 引用规则，帧3） -->
                <div class="li-condx-targets">
                  <button
                    v-for="t in CONDX_TARGETS"
                    :key="t.value"
                    type="button"
                    class="li-condx-card"
                    :class="{ 'is-active': row.condition && row.condition.target === t.value }"
                    :title="t.note"
                    @click="setRowTarget(row, t.value)"
                  >
                    <span class="li-condx-card-label">{{ t.label }}</span>
                    <span class="li-condx-card-note">{{ t.note }}</span>
                  </button>
                  <button
                    type="button"
                    class="li-condx-card"
                    :title="CONDX_TARGET_REF_NOTE"
                    @click="setRowToRef(row)"
                  >
                    <span class="li-condx-card-label">引用规则</span>
                    <span class="li-condx-card-note">{{ CONDX_TARGET_REF_NOTE }}</span>
                  </button>
                </div>
                <template v-if="row.condition">
                  <!-- Step 2 格式模板：目录双问法 + 模板输入 + 词条点选 + 例子模板 -->
                  <div class="li-condx-fmt">
                    <el-select
                      v-if="row.condition.target === 'dir'"
                      :model-value="row.condition.format.mode"
                      size="small"
                      class="li-condx-mode"
                      @update:model-value="setRowFormatMode(row, $event)"
                    >
                      <el-option label="整条按模板" value="template" />
                      <el-option label="含这个词" value="contains" />
                    </el-select>
                    <el-input
                      v-if="row.condition.format.mode === 'contains'"
                      v-model="row.condition.format.word"
                      size="small"
                      placeholder="包含词（纯文字，不含占位词）"
                    />
                    <textarea
                      v-else
                      v-model="row.condition.format.tpl"
                      class="li-condx-tpl"
                      rows="2"
                      spellcheck="false"
                      :placeholder="'如 【{作者}】{标题} / {公共段}_{序号}（点下方词条插入光标处）'"
                      @blur="onTplSelection(i, $event.target.selectionStart)"
                      @keyup="onTplSelection(i, $event.target.selectionStart)"
                      @click="onTplSelection(i, $event.target.selectionStart)"
                    />
                  </div>
                  <div v-if="row.condition.format.mode === 'template'" class="li-condx-chips">
                    <button
                      v-for="v in CONDX_VOCAB"
                      :key="v.word"
                      type="button"
                      class="li-condx-chip"
                      :title="v.note"
                      @click="insertWordAtCursor(row, v.word)"
                    >{{ '{' + v.word + '}' }}</button>
                    <span class="li-condx-chips-sep">｜</span>
                    <button
                      v-for="ex in CONDX_EXAMPLES"
                      :key="ex.name"
                      type="button"
                      class="li-condx-chip is-example"
                      :title="`${ex.template}（${ex.description}）`"
                      @click="applyExample(row, ex)"
                    >{{ ex.name }}</button>
                  </div>
                  <!-- Step 3 附加条件（终结谓词，按目标封闭集）+ 值级小标 -->
                  <div class="li-condx-extra">
                    <div class="li-condx-extra-head">
                      <span class="li-condx-label">附加条件（终结谓词）</span>
                      <el-select
                        v-if="rowAttachOptions(row).length"
                        :model-value="''"
                        size="small"
                        class="li-condx-addsel"
                        placeholder="+ 加一条"
                        @update:model-value="addRowAttach(row, $event)"
                      >
                        <el-option v-for="o in rowAttachOptions(row)" :key="o.value" :label="o.label" :value="o.value" />
                      </el-select>
                    </div>
                    <div v-for="(a, ai) in row.condition.attach" :key="ai" class="li-condx-inline">
                      <span class="li-condx-inline-label">{{ attachLabelOf(row, a.kind) }}</span>
                      <textarea
                        v-if="attachNeeds(a, 'tpl')"
                        v-model="a.tpl"
                        class="li-condx-minitpl"
                        rows="1"
                        spellcheck="false"
                        placeholder="迷你模板（如 {序号}）"
                      />
                      <el-input v-if="attachNeeds(a, 'word')" v-model="a.word" size="small" placeholder="包含词" class="li-condx-mini" />
                      <el-input-number v-if="attachNeeds(a, 'count')" v-model="a.minCount" :min="1" size="small" controls-position="right" class="li-condx-num" />
                      <el-select v-if="attachNeeds(a, 'types')" v-model="a.types" size="small" multiple placeholder="类型" class="li-condx-mini">
                        <el-option v-for="t in CONDX_TYPE_OPTIONS" :key="t" :label="t" :value="t" />
                      </el-select>
                      <el-input v-if="attachNeeds(a, 'value')" v-model="a.value" size="small" placeholder=".jpg" class="li-condx-mini" />
                      <el-button size="small" text type="danger" @click="row.condition!.attach!.splice(ai, 1)">删</el-button>
                    </div>
                    <div class="li-condx-extra-head">
                      <span class="li-condx-label">值级小标（占位词上的断言）</span>
                      <el-select
                        v-if="condxRowPlaceholders(row.condition).length"
                        :model-value="''"
                        size="small"
                        class="li-condx-addsel"
                        placeholder="+ 加一枚"
                        @update:model-value="addRowBadge(row, $event)"
                      >
                        <el-option v-for="k in CONDX_BADGE_KINDS" :key="k.value" :label="k.label" :value="k.value" />
                      </el-select>
                    </div>
                    <div v-for="(b, bi) in row.condition.badges" :key="bi" class="li-condx-inline">
                      <el-select v-model="b.placeholder" size="small" class="li-condx-mini">
                        <el-option v-for="w in condxRowPlaceholders(row.condition)" :key="w" :label="`{${w}}`" :value="w" />
                      </el-select>
                      <span class="li-condx-dot">●</span>
                      <el-select :model-value="b.kind" size="small" class="li-condx-mini" @update:model-value="setBadgeKind(b, $event)">
                        <el-option v-for="k in CONDX_BADGE_KINDS" :key="k.value" :label="k.label" :value="k.value" />
                      </el-select>
                      <el-select v-if="badgeNeeds(b, 'dim')" v-model="b.dim" size="small" placeholder="名单维度" class="li-condx-mini">
                        <el-option v-for="d in CONDX_BADGE_DIMS" :key="d" :label="dimNameOf(d)" :value="d" />
                      </el-select>
                      <label v-if="badgeNeeds(b, 'dim')" class="li-condx-fold"><input v-model="b.fold" type="checkbox"><span>形变归一</span></label>
                      <template v-if="badgeNeeds(b, 'againstRow')">
                        <el-select v-model="b.againstRow" size="small" placeholder="对方行" class="li-condx-mini">
                          <el-option v-for="f in earlierFlatRows(i)" :key="f.row" :label="`第 ${f.row} 行`" :value="f.row" />
                        </el-select>
                        <el-select v-model="b.againstPh" size="small" placeholder="对方占位词" class="li-condx-mini">
                          <el-option v-for="w in againstWords(b)" :key="w" :label="`{${w}}`" :value="w" />
                        </el-select>
                      </template>
                      <el-input v-if="badgeNeeds(b, 'value')" v-model="b.value" size="small" placeholder="常量值" class="li-condx-mini" />
                      <el-button size="small" text type="danger" @click="row.condition!.badges!.splice(bi, 1)">删</el-button>
                    </div>
                  </div>
                </template>
              </template>
              <p v-if="condxRowHint(i)" class="li-red-row-hint">{{ condxRowHint(i) }}</p>
            </div>
            <el-button size="small" text @click="addCondxRow">+ 加一行</el-button>
            <!-- 占位词捕获清单（行号+词）：提取段引用的唯一来源 -->
            <div v-if="captureList.length" class="li-red-caps">
              <span class="li-red-label">占位词捕获清单</span>
              <div v-for="(c, i) in captureList" :key="`${c.row}/${c.word}/${i}`" class="li-red-cap-row">
                <span class="li-red-cap-name">第 {{ c.row }} 行 · {{ '{' + c.word + '}' }}</span>
              </div>
              <p class="li-red-hint">
                {占位词}＝命名捕获槽：只负责在匹配时捕获文本，本身不识别也不产生元数据——槽名与维度同字只是习惯叫法，无绑定
              </p>
            </div>
            <p v-else class="li-red-hint">
              尚无可引用的占位词——在行模板中写入 {作者}/{标题} 等词条后，提取段才能引用其捕获值
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

        <!-- 下半「提取规则」：逐维度行 = 维度 ← 来源 + 剥壳前后缀 + 条件勾选；行可增删 -->
        <section class="li-red-sec">
          <div class="li-red-sec-head">
            <h4 class="li-red-sec-title">提取规则<span class="li-red-sec-note">下半 · 命中后取值声明到维度</span></h4>
            <el-button size="small" text @click="toggleExtractionsJson">
              {{ extractionsJsonMode ? '切回结构化编辑' : '高级（JSON）' }}
            </el-button>
          </div>
          <p class="li-red-hint">
            元数据只在这里产生：把上半某个捕获槽的值「赋给维度」才是识别——上半匹配命中而下半没有赋值行时，不产出任何元数据
          </p>

          <template v-if="!extractionsJsonMode">
            <template v-for="(row, i) in extractionRows" :key="i">
              <!-- 「来自被引规则」分组头（F）：置于首个被引捕获行之前，转写提示
                   小字常驻（与镜像缺失 ⚠ 联动） -->
              <div v-if="isFirstRefCaptureRow(i)" class="li-red-ref-group">
                <div class="li-red-ref-group-title">── 来自被引规则 ──</div>
                <p class="li-red-ref-group-hint">每条「被引规则捕获」提取行转写成员一行的捕获值；成员多形态（多行）时须逐行各接一条，未接线的形态取不到值</p>
              </div>
              <div
                class="li-red-ex-row"
                :class="{ 'is-invalid': !!extractionRowHint(row) }"
              >
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
                  placeholder="成员规则·占位词"
                  class="li-red-ex-capture"
                  @update:model-value="setRowRefCapture(row, $event)"
                >
                  <el-option-group
                    v-for="g in refCaptureGroups"
                    :key="`${g.ruleId}/${g.index}`"
                    :label="`${g.label}（引用 ${g.index}）`"
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
                <template v-else>
                  <el-input v-model="row.stripPrefix" size="small" placeholder="剥壳前缀（可空）" class="li-red-ex-strip" />
                  <el-input v-model="row.stripSuffix" size="small" placeholder="剥壳后缀（可空）" class="li-red-ex-strip" />
                </template>
                <el-input v-model="row.roleName" size="small" placeholder="角色名（可空）" class="li-red-ex-role" />
                <label v-if="row.from !== 'scan'" class="li-red-cond">
                  <input v-model="row.seqOnly" type="checkbox">
                  <span>仅序列成员产出</span>
                </label>
                <span v-if="extractionRowHasOtherConds(row) && row.from !== 'scan'" class="li-red-other">含其他手写条件（高级 JSON 模式可编辑）</span>
              </div>
              <p v-if="extractionRowHint(row)" class="li-red-row-hint">{{ extractionRowHint(row) }}</p>
              <!-- 镜像缺失提示（D）：成员多行声明同组词而本行只接线部分行 -->
              <p v-if="refCaptureMirrorWarn(row, i)" class="li-red-mirror-warn">{{ refCaptureMirrorWarn(row, i) }}</p>
            </div>
            </template>
            <el-button size="small" text @click="addExtractionRow">+ 加一行</el-button>
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
            <p class="li-red-hint">JSON 数组（dimension / from / captureName / constant / roleName / strip 前后缀 / when 条件表达式）</p>
          </template>
        </section>

        <!-- 合成策略（随旧表单随迁）：角色绑定 + 维度策略 -->
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
//   ① rule（SubpageRules「编辑/新建」）：上半=识别规则（一行人话摘要+结构化形态
//      编辑〔文件名形态/目录锚定/序列群组/组合，RulePatternNode 递归展开〕+捕获组
//      声明清单+「高级（JSON）」切换双向同步），下半=提取规则（逐维度行=维度←
//      来源下拉〔捕获组只列上半声明的组/目录名/群组前缀/常量〕+剥壳前后缀+条件
//      勾选〔仅序列成员产出等，逐行〕；行可增删；行级就地校验）；合成策略随旧
//      表单随迁。保存载荷经 emit 交父组件走既有 saveRule RPC。
//   ② setAdjust（SetRulePanel「调整规则」）：上半=草稿识别形态（只读呈现），
//      下半=指派行——剧本E 全部语义随迁：线索三族（序列 seqKey/目录段/文件名段）
//      作为来源选项、默认指派预填、目录段多段连续勾选、互斥校验「一个维度只能有
//      一个来源」「目录段组合须为连续段」行级提示、单选目的地互斥挤占、还原默认
//      指派。保存载荷经 emit 交父组件走既有 saveWorkSetRuleAdjust RPC。
// 校验不过（组引用/维度来源唯一/JSON 解析）行级就地标红、弹窗不关；JSON 高级
// 模式解析错就地显示位置不丢输入；取消不保存（destroy-on-close，再开回上次保
// 存态——打开时从 props 重新取值）。
// W4 组合引用（规则组合引用剧本）：上半目标四卡新增「引用规则」卡——点选后该行
// 转为引用行（恒置条件行上方、独立编号「引用 N」、不带且/或、不占条件行行号）；
// 提取来源新增「被引规则捕获」＝成员规则·占位词（按引用行顺序分组）；引用行在
// 场时空条件表合法；保存载荷增 refs（成员 ID 列表随既有 saveRule 原样入档）。
import { ref, computed, watch } from 'vue'
import {
  DIMENSION_OPTIONS,
  FROM_OPTIONS,
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
  CONDX_TARGETS,
  CONDX_TARGET_REF_NOTE,
  CONDX_VOCAB,
  CONDX_ATTACH_OPTIONS,
  CONDX_TYPE_OPTIONS,
  CONDX_BADGE_KINDS,
  CONDX_BADGE_DIMS,
  CONDX_EXAMPLES,
  condxFlatRows,
  condxRowPlaceholders,
  condxTemplateError,
  condxRowsError,
  condxRowError,
  condxCaptures,
  condxNormalizeTpl,
  condxNormalizeRows,
  condxComboSentence,
  condxShapeSummary,
  type CondxRowForm,
  type CondxForm,
  type CondxBadgeForm,
  type CondxAttachForm,
  type CondxCaptureDecl,
  type ExtractionActionJSON
} from './ruleFormModel'

// —— 入口载荷形态（父组件组装的普通对象） ——

// 档案规则条目最小投影（W4 引用通路：引用行候选过滤与成员占位词声明取值用——
// SubpageRules 把完整规则列表经 allRules prop 传入；extractions 供引用行静态
// 告警判定〔成员独立产出/定群成员〕）。
interface RuleArchiveItem {
  id: string
  description: string
  refs?: string[] | null
  conditions?: unknown
  extractions?: unknown
}

// rule 模式初始档案条目（SubpageRules 的 RuleEntryView 投影；conditions 为
// v4 条件行列表——v4 档案条目唯一条件载体，模式树不再落档；refs 为 W4 组合
// 引用的成员规则 ID 列表——非组合规则缺省空）。
interface RuleInput {
  id: string
  description: string
  source: string
  enabled: boolean
  isNew: boolean
  refs?: string[] | null
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
  // 档案全量规则列表（W4：引用行候选与成员占位词声明的数据源，SubpageRules 传入）
  allRules?: RuleArchiveItem[] | null
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
// 为 v4 条件行，pattern 不再发送；refs 为 W4 引用行成员列表，随载荷原样走
// 既有 saveRule）。
interface RuleSavePayload {
  id: string
  description: string
  source: string
  enabled: boolean
  refs: string[]
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
  if (props.mode === 'rule') return props.rule && props.rule.isNew ? '新建规则' : '编辑规则'
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
// W4 refCapture 来源的成员规则 ID（refRule/row/captureName 三元寻址）。
interface ExtractionRow {
  dimension: string
  from: string
  captureName: string
  captureRow: number
  refRule: string
  constant: string
  scanScope: string
  roleName: string
  stripPrefix: string
  stripSuffix: string
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
// 引用行列表（W4：组合引用的成员规则——恒置条件行上方、独立编号「引用 N」，
// 不占条件行「第 N 行」；ruleId 空＝新行未选成员）
interface RefRowForm {
  ruleId: string
}
const refRows = ref<RefRowForm[]>([])
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
    stripPrefix: action.stripPrefix || '',
    stripSuffix: action.stripSuffix || '',
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

// refCapture 行的「成员规则·占位词」下拉取值拆装（值编码 refRule/row/captureName
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
// scan 行=范围 + 角色透传，不带 when/剥壳/捕获组/常量——内核校验拒绝面同构）
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
      // W4 寻址=成员规则+行号+占位词三元（refRule 往返不丢；行 1 缺省同口径）
      action.refRule = row.refRule
      action.captureName = row.captureName
      if (row.captureRow > 1) action.row = row.captureRow
    }
    if (row.from === 'constant' || row.from === 'adjudication') action.constant = row.constant
    if (row.roleName) action.roleName = row.roleName
    if (row.stripPrefix) action.stripPrefix = row.stripPrefix
    if (row.stripSuffix) action.stripSuffix = row.stripSuffix
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

// —— W4 引用行（规则组合引用剧本一·帧3-帧5 / 剧本三）：候选、成员占位词声明
// 与行编辑 ——

function refRuleById(ruleId: string): RuleArchiveItem | undefined {
  return (props.allRules || []).find((r) => r.id === ruleId)
}

// 成员候选（帧4）：档案里其余非组合规则（refs 空者——一期禁嵌套）且非本条自身
const refCandidates = computed<RuleArchiveItem[]>(() => {
  const selfId = props.rule && !props.rule.isNew ? props.rule.id : ''
  return (props.allRules || []).filter((r) => r.id !== selfId && !(r.refs && r.refs.length))
})

function refRuleLabel(ruleId: string): string {
  const hit = refRuleById(ruleId)
  return hit ? hit.description || hit.id : ruleId
}

// 成员·占位词分组（帧8）：按引用行顺序分组列出各成员上半声明的「第 N 行 · {词}」；
// 成员当前档案声明为真源——成员改过模板后旧引用自然不在列（剧本三·帧2）
interface RefCaptureGroup {
  ruleId: string
  label: string
  index: number
  captures: CondxCaptureDecl[]
}

const refCaptureGroups = computed<RefCaptureGroup[]>(() => {
  const out: RefCaptureGroup[] = []
  refRows.value.forEach((r, i) => {
    if (!r.ruleId) return
    const member = refRuleById(r.ruleId)
    if (!member) return
    const rows = Array.isArray(member.conditions) ? (member.conditions as CondxRowForm[]) : []
    out.push({ ruleId: r.ruleId, label: member.description || member.id, index: i + 1, captures: condxCaptures(rows) })
  })
  return out
})

// 已声明的「成员/行N/组名」寻址集合（refCapture 提取行引用校验）
const declaredRefCaptures = computed<Set<string>>(() =>
  new Set(refCaptureGroups.value.flatMap((g) => g.captures.map((c) => `${g.ruleId}/${c.row}/${c.group}`)))
)

// 英文组名 → 中文词（失效行残影呈现用；词表九词组名唯一）
function groupWordOf(group: string): string {
  const hit = CONDX_VOCAB.find((v) => v.group === group)
  return hit ? hit.word : group
}

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
    out.push({ value: key, label: `${refRuleLabel(row.refRule)} · 第 ${row.captureRow} 行 · {${groupWordOf(row.captureName)}}` })
  }
  return out
})

// —— 引用行呈现模型（B3/F/D）：成员描述·类别·形态数、成员条件行只读平铺、
// 单行序列零提取红色告警与成员独立产出提示。数据全部本地（成员现档经
// allRules 传入，静态判定不挂后端校验、不拦截保存）。 ——

// 引用行呈现（与 refRows 按下标一一对应）。
interface RefRowView {
  desc: string
  flatLines: string[]
  alert: string
  notice: string
}

// 单行序列零提取成员判定（与内核 isGroupingRule 同构：恰 1 行条件 + 目标序列
// + 提取动作空——只聚组不产生文件级命中，组合门段对其跳过=引用恒不产出；
// 两行零提取成员不触发）。
function isGroupingMember(member: RuleArchiveItem): boolean {
  const rows = Array.isArray(member.conditions) ? (member.conditions as CondxRowForm[]) : []
  const extractions = Array.isArray(member.extractions) ? member.extractions : []
  return extractions.length === 0 && rows.length === 1 &&
    rows[0].condition != null && rows[0].condition?.target === 'seq'
}

// 成员条件行只读平铺（F）：按行序列出「第N行 目标「模板」」（行号与占位词
// 寻址同口径——括号子组递归计行）；括号子组行显示「括号子组（N 行）」；模板
// 文本经旧词归一显示（{键} 显示为 {公共段}）。
function memberFlatLines(rows: unknown): string[] {
  const list = Array.isArray(rows) ? (rows as CondxRowForm[]) : []
  const out: string[] = []
  let n = 0
  const walk = (listRows: CondxRowForm[]): void => {
    for (const r of listRows) {
      if (r.group) {
        out.push(`括号子组（${r.group.length} 行）`)
        walk(r.group)
        continue
      }
      n += 1
      if (!r.condition) continue
      const target = CONDX_TARGETS.find((t) => t.value === r.condition?.target)
      const fmt = r.condition.format.mode === 'contains'
        ? `含「${r.condition.format.word || '?'}」`
        : condxNormalizeTpl(r.condition.format.tpl || '?')
      out.push(`第${n}行 ${target ? target.label : r.condition.target}「${fmt}」`)
    }
  }
  walk(list)
  return out
}

const refRowViews = computed<RefRowView[]>(() =>
  refRows.value.map((r) => {
    const member = r.ruleId ? refRuleById(r.ruleId) : undefined
    if (!r.ruleId) return { desc: '引用规则', flatLines: [], alert: '', notice: '' }
    const label = refRuleLabel(r.ruleId)
    if (!member) return { desc: label, flatLines: [], alert: '', notice: '' }
    const shape = condxShapeSummary(member.conditions)
    const desc = shape.count > 0 ? `${label} · ${shape.label} · ${shape.count} 形态` : label
    return {
      desc,
      flatLines: memberFlatLines(member.conditions),
      alert: isGroupingMember(member)
        ? '✗ 该成员只有一行序列条件、且不提取任何信息——它只用于把成套文件聚成一组、不会命中具体文件，组合引用它将永远不产出。出路：给成员加提取动作，或改成多行条件'
        : '',
      notice: Array.isArray(member.extractions) && member.extractions.length > 0
        ? '⚠ 成员自身将独立产出声明、命中照常计数；若要它只作引用条件，请在成员编辑中清空提取动作'
        : ''
    }
  })
)

// 首个「被引规则捕获」行位置（提取段「来自被引规则」分组头的插入点）。
function isFirstRefCaptureRow(index: number): boolean {
  if (extractionRows.value[index]?.from !== 'refCapture') return false
  return extractionRows.value.slice(0, index).every((r) => r.from !== 'refCapture')
}

// 镜像缺失提示（D）：按成员×占位词组名聚合——成员在多行声明同组词、本表被引
// 捕获行只接线其中部分行时，在该组首条接线行下提示（未接线行形态命中时取不到
// 值）；成员声明与接线数据均来自本地。
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
  return `⚠ {${groupWordOf(row.captureName)}}在成员第 ${list(declared)} 行都声明，你只接线了第 ${list(wiredUnique)} 行——第 ${list(missing)} 行形态命中时取不到值`
}

// 点「引用规则」卡（帧3）：该行转出条件行列表、追加为引用行——恒置条件行上方、
// 独立编号「引用 N」、不占条件行「第 N 行」（占位词寻址不受影响）、不带且/或
function setRowToRef(row: CondxRowForm): void {
  const idx = condxRows.value.indexOf(row)
  if (idx < 0) return
  condxRows.value.splice(idx, 1)
  refRows.value.push({ ruleId: '' })
}

// 条件行完整性（W4）：引用行在场时空条件表合法——组合可仅由成员门控（帧5→帧6
// 删光自身条件行后摘要为「引用 N 条成员规则；（无条件行）」且可保存）
function condxRowsErrorWithRefs(rows: CondxRowForm[]): string {
  if (!condxFlatRows(rows).length && refRows.value.length) return ''
  return condxRowsError(rows)
}

const condxSummary = computed((): string => {
  // 引用行在场：摘要=组合关系直译句（与规则列表组合行小字同函数生成）；
  // 自身还有条件行时尾接行数（成员全未选的新行保留原前缀呈现）
  const flat = condxFlatRows(activeCondxRows.value)
  const members = refRows.value
    .filter((r) => r.ruleId)
    .map((r) => ({ label: refRuleLabel(r.ruleId), rows: refRuleById(r.ruleId)?.conditions ?? [] }))
  if (members.length) {
    const core = condxComboSentence(members)
    return flat.length ? `${core}，且满足自身 ${flat.length} 行条件` : core
  }
  const refPrefix = refRows.value.length ? `引用 ${refRows.value.length} 条成员规则；` : ''
  if (!flat.length) return `${refPrefix}（无条件行）`
  const parts = flat.map((f) => {
    const c = f.condx
    const target = CONDX_TARGETS.find((t) => t.value === c.target)
    const fmt = c.format.mode === 'contains' ? `含「${c.format.word || '?'}」` : condxNormalizeTpl(c.format.tpl || '?')
    const badges = (c.badges || []).length ? ` ·${(c.badges || []).length} 枚小标` : ''
    const attach = (c.attach || []).length ? ` ·${(c.attach || []).length} 条附加` : ''
    return `第${f.row}行 ${target ? target.label : c.target}「${fmt}」${badges}${attach}`
  })
  return `${refPrefix}${parts.join('；')}`
})

// 结构化模式的行级校验提示（含 JSON 模式聚合错误的呈现位）
const condxRowHint = (i: number): string => {
  const rows = condxRows.value
  const row = rows[i]
  if (!row) return ''
  const flat = condxFlatRows(rows)
  if (row.group) return ''
  const hit = flat.find((f) => f.condx === row.condition)
  if (!hit) return '行内容不完整'
  return condxRowError(hit.row, row.condition, flat)
}

// —— 行编辑辅助（目标卡/格式/词条插入/例子模板/附加/小标） ——

function addCondxRow(): void {
  condxRows.value.push({ connector: condxRows.value.length ? 'and' : '', negated: false, condition: null, group: null })
}

// 目标三卡点选：首次进入带默认格式（template + 空模板），切换目标保留格式段
function setRowTarget(row: CondxRowForm, target: 'file' | 'dir' | 'seq'): void {
  if (!row.condition) {
    row.condition = { target, format: { mode: 'template', tpl: '' }, badges: [], attach: [] }
    return
  }
  row.condition.target = target
  if (target !== 'dir' && row.condition.format.mode === 'contains') {
    row.condition.format = { mode: 'template', tpl: '' }
  }
  // 序列默认形态一步到位（{公共段}_{序号}）
  if (target === 'seq' && row.condition.format.mode === 'template' && !(row.condition.format.tpl || '').trim()) {
    row.condition.format.tpl = '{公共段}_{序号}'
  }
  row.condition.attach = (row.condition.attach || []).filter((a) => rowAttachValues(target).indexOf(a.kind) >= 0)
}

function setRowFormatMode(row: CondxRowForm, mode: string): void {
  if (!row.condition) return
  row.condition.format = mode === 'contains' ? { mode: 'contains', word: '' } : { mode: 'template', tpl: '' }
}

// 词条点选插入光标处（记录 textarea 选区；无选区追加到末尾）
const tplSelections = new Map<number, number>()
function onTplSelection(rowIdx: number, sel: number): void {
  tplSelections.set(rowIdx, sel)
}
function insertWordAtCursor(row: CondxRowForm, word: string): void {
  if (!row.condition || row.condition.format.mode !== 'template') return
  const tpl = row.condition.format.tpl || ''
  const idx = condxRows.value.indexOf(row)
  const pos = tplSelections.get(idx)
  const piece = `{${word}}`
  if (pos == null || pos < 0 || pos > tpl.length) {
    row.condition.format.tpl = tpl + piece
    return
  }
  row.condition.format.tpl = tpl.slice(0, pos) + piece + tpl.slice(pos)
  tplSelections.set(idx, pos + piece.length)
}

function applyExample(row: CondxRowForm, ex: { template: string; target: 'file' | 'dir' | 'seq' }): void {
  setRowTarget(row, ex.target)
  if (row.condition) row.condition.format = { mode: 'template', tpl: ex.template }
}

// 附加谓词（按目标封闭集）
function rowAttachValues(target: string): string[] {
  return (CONDX_ATTACH_OPTIONS[target] || []).map((o) => o.value)
}
function rowAttachOptions(row: CondxRowForm): { value: string; label: string }[] {
  const target = row.condition ? row.condition.target : ''
  return (CONDX_ATTACH_OPTIONS[target] || []).map((o) => ({ value: o.value, label: o.label }))
}
function attachLabelOf(row: CondxRowForm, kind: string): string {
  const target = row.condition ? row.condition.target : ''
  const hit = (CONDX_ATTACH_OPTIONS[target] || []).find((o) => o.value === kind)
  return hit ? hit.label : kind
}
function attachNeeds(a: CondxAttachForm, need: string): boolean {
  const target = (condxRows.value.find((r) => r.condition && (r.condition.attach || []).indexOf(a) >= 0)?.condition?.target) || ''
  const hit = (CONDX_ATTACH_OPTIONS[target] || []).find((o) => o.value === a.kind)
  return !!hit && hit.needs.indexOf(need) >= 0
}
function addRowAttach(row: CondxRowForm, kind: string): void {
  if (!row.condition) return
  if (!row.condition.attach) row.condition.attach = []
  row.condition.attach.push({ kind })
}

// 值级小标
function addRowBadge(row: CondxRowForm, kind: string): void {
  if (!row.condition) return
  if (!row.condition.badges) row.condition.badges = []
  const first = condxRowPlaceholders(row.condition)[0] || ''
  row.condition.badges.push({ placeholder: first, kind: kind as CondxBadgeForm['kind'] })
}
function setBadgeKind(b: CondxBadgeForm, kind: string): void {
  b.kind = kind as CondxBadgeForm['kind']
  b.dim = undefined
  b.fold = undefined
  b.againstRow = undefined
  b.againstPh = undefined
  b.value = undefined
}
function badgeNeeds(b: CondxBadgeForm, need: string): boolean {
  const hit = CONDX_BADGE_KINDS.find((k) => k.value === b.kind)
  return !!hit && hit.needs.indexOf(need) >= 0
}
// equal 对方行候选（展平行号早于本行的 condition 行）
function earlierFlatRows(rowIndex: number): { row: number; condx: CondxForm }[] {
  const rows = condxRows.value
  const flat = condxFlatRows(rows)
  const row = rows[rowIndex]
  if (!row || !row.condition) return []
  const self = flat.find((f) => f.condx === row.condition)
  if (!self) return []
  return flat.filter((f) => f.row < self.row)
}
function againstWords(b: CondxBadgeForm): string[] {
  const hit = earlierFlatRowsForBadge(b)
  return hit ? condxRowPlaceholders(hit) : []
}
function earlierFlatRowsForBadge(b: CondxBadgeForm): CondxForm | null {
  const flat = condxFlatRows(condxRows.value)
  const hit = flat.find((f) => f.row === b.againstRow)
  return hit ? hit.condx : null
}
function dimNameOf(dim: string): string {
  const hit = DIMENSION_OPTIONS.find((d) => d.value === dim)
  return hit ? hit.label : dim
}

// JSON 模式下边输入边解析：合法即刷新清单/摘要，非法就地显示错误位置、不丢输入
watch(condxText, () => {
  if (!condxJsonMode.value) return
  const parsed = tryParseJSON(condxText.value)
  if (parsed.ok && Array.isArray(parsed.value)) {
    condxParsed.value = parsed.value as CondxRowForm[]
    condxError.value = condxRowsErrorWithRefs(condxParsed.value)
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
    // 切回结构化：旧词 {键} 书写归一显示为 {公共段}（保存即落档新词）
    condxRows.value = condxNormalizeRows(rows)
    condxParsed.value = null
    condxJsonMode.value = false
    condxError.value = condxRowsErrorWithRefs(condxRows.value)
    return
  }
  condxText.value = JSON.stringify(activeCondxRows.value, null, 2)
  condxJsonMode.value = true
  const parsed = tryParseJSON(condxText.value)
  if (parsed.ok && Array.isArray(parsed.value)) {
    condxParsed.value = parsed.value as CondxRowForm[]
    condxError.value = condxRowsErrorWithRefs(condxParsed.value)
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
    if (!row.refRule || !row.captureName) return '须选择成员规则·占位词'
    if (!refRows.value.some((r) => r.ruleId === row.refRule)) return '被引规则不在上方引用行中'
    // 失效＝成员改过模板（剧本三·帧1）/悬空成员——行级标红锚文案
    if (!declaredRefCaptures.value.has(`${row.refRule}/${row.captureRow}/${row.captureName}`)) {
      return '引用的占位词已不在被引规则声明'
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
    stripPrefix: '',
    stripSuffix: '',
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
    tplSelections.clear()
    // 引用行复位（W4）：档案 refs（组合规则成员）逐条转为引用行；坏档非字符串项滤除
    const initRefs = rule && Array.isArray(rule.refs) ? rule.refs : []
    refRows.value = initRefs.filter((id): id is string => typeof id === 'string').map((id) => ({ ruleId: id }))
    const defaultRows = (): CondxRowForm[] => [
      {
        connector: '',
        negated: false,
        condition: { target: 'file', format: { mode: 'template', tpl: '【{作者}】{标题}' }, badges: [], attach: [] },
        group: null
      }
    ]
    if (!rule || rule.isNew) {
      condxRows.value = defaultRows()
    } else if (Array.isArray(rule.conditions)) {
      // 旧档 {键} 模板归一显示为 {公共段}（与保存载荷构造同一归一函数——
      // 弹窗所见即保存所得）
      condxRows.value = condxNormalizeRows(cloneJSON(rule.conditions)) as CondxRowForm[]
    } else if (rule.conditions != null) {
      // 历史坏档（conditions 非数组）：原文进 JSON 高级模式，避免结构化缺省
      // 形态静默改写档案内容（旧词归一为语义等价换词，不属于形态改写）
      condxJsonMode.value = true
      condxText.value = JSON.stringify(condxNormalizeRows(rule.conditions), null, 2)
    } else {
      // 无 conditions 键的两义分流：组合规则（refs 在场）＝空条件表的落档形态
      //（内核 omitempty 序列化无键，剧本一·帧6 语义）——空行集起步，不得缺省
      // 补模板行（否则编辑再存会把缺省行写成组合自身条件、破坏成员门控求值，
      // 2026-10-26 live-test 抓出）；普通规则按 v3 残档处理，缺省模板行起步
      //（既有行为不变）。
      condxRows.value = refRows.value.length > 0 ? [] : defaultRows()
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
      condxError.value = condxRowsErrorWithRefs(condxRows.value)
    }
    extractionsJsonMode.value = false
    extractionsText.value = ''
    const rawExtractions: unknown = !rule || rule.isNew ? NEW_EXTRACTIONS_SAMPLE : rule.extractions
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
  },
  { immediate: true }
)

// —— 保存 / 取消 ——

function cancel(): void {
  emit('update:state', false)
}

// rule 模式保存载荷构造（含全部闸门：JSON 合法性/引用行完整/条件行完整/提取行
// 校验——不过就地标红返回 null）。保存与命中预览共用本构造路径：预览载荷即
// 归一后的保存草稿（{公共段} 归一在 conditions 构造层执行），非原始表单值——
// 预览=保存后口径。
function buildRulePayload(): RuleSavePayload | null {
  // JSON 合法性闸门：识别/提取任一停在 JSON 模式且解析不过 → 不关窗不保存
  if (condxJsonMode.value) {
    if (condxError.value || !condxParsed.value) return null
  }
  // 引用行完整性闸门（W4）：未选成员的引用行 → 行内已标红，此处拦住不保存
  if (refRows.value.some((r) => !r.ruleId)) return null
  // 条件行完整性闸门（v4）：行表校验不过（词表外/相邻占位/附加缺字段等）
  // → 行内已标红，此处拦住不保存；引用行在场时空条件表合法（组合可仅由成员门控）
  if (!condxJsonMode.value && condxRowsErrorWithRefs(condxRows.value)) {
    condxError.value = condxRowsErrorWithRefs(condxRows.value)
    return null
  }
  // 保存载荷构造层统一执行旧词归一（{键} → {公共段}，语义等价）：结构化与
  // JSON 高级模式同路径——JSON 手写 {键} 保存亦落档新词
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
    // 占位词引用校验（JSON 模式聚合呈现）：引用未声明「行N·{词}」→ 行号化
    // 错误就地列出
    const problems: string[] = []
    ;(parsed.value as ExtractionActionJSON[]).forEach((action, i) => {
      if (action && action.from === 'capture' && action.captureName) {
        const rowNo = action.row || 1
        if (!declaredCaptures.value.has(`${rowNo}/${action.captureName}`)) {
          problems.push(`第 ${i + 1} 行引用占位词「第 ${rowNo} 行·{${action.captureName}}」未在上半声明`)
        }
      }
      if (action && action.from === 'refCapture') {
        const rowNo = action.row || 1
        const inRefs = refRows.value.some((r) => r.ruleId === action.refRule)
        if (!action.refRule || !action.captureName || !inRefs || !declaredRefCaptures.value.has(`${action.refRule}/${rowNo}/${action.captureName}`)) {
          problems.push(`第 ${i + 1} 行：引用的占位词已不在被引规则声明`)
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
  return {
    id: rule && !rule.isNew ? rule.id : '',
    description: descInput.value,
    source: (rule && rule.source) || 'manual',
    enabled: enabledInput.value,
    refs: refRows.value.map((r) => r.ruleId),
    conditions: conditions,
    extractions: extractions,
    synthesis: {
      dimPolicies: dimRows.value.filter((d) => d.dimension && d.strategy),
      roles: roleRows.value.filter((r) => r.source && r.roleName)
    }
  }
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

// 预览触发（帧1）：载荷走与保存同一条草稿构造路径（同一闸门与 {公共段} 归一）；
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

/* 提取段行：主行（维度←来源）+ 副行（剥壳/角色/条件）+ 行级提示 */
.li-red-ex-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  /* 2026-10-28 行区域标识·左缘色条：提取条件行左缘 success 绿 3px 竖条（与识别行
     主色蓝条分野，见 .li-condx-row 注） */
  border-left: 3px solid var(--el-color-success);
  border-radius: var(--app-radius);
  padding: 8px 10px;
}

.li-red-ex-row.is-invalid {
  border-color: var(--app-color-danger);
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

.li-red-ex-strip {
  width: 130px;
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

/* 结果卡：弹窗底部整行、按钮行上方；只读呈现与表单互不干扰 */
.li-red-preview-card {
  flex-basis: 100%;
  display: flex;
  flex-direction: column;
  gap: 6px;
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 10px 12px;
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

/* v4 条件行（三步漏斗）：行卡片 + 目标三卡 + 词条 chips + 附加/小标行 */
.li-condx-row {
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  /* 2026-10-28 行区域标识·左缘色条（第三版，用户裁决「换方案」）：识别条件行
     左缘主色 3px 竖条——不动任何底色（行/分区透明复原，行内组件配色环境即原
     白底设计），逐行成块且与提取行（success 绿条）色彩分野；前两版（行级淡底/
     分区淡底+白卡行）均因观感被否 */
  border-left: 3px solid var(--el-color-primary);
  border-radius: var(--app-radius);
  padding: 10px;
}

.li-condx-row.is-invalid {
  border-color: var(--el-color-danger);
}

.li-condx-row-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-condx-conn {
  width: 130px;
}

.li-condx-neg {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-condx-groupnote {
  font-size: 12px;
  color: var(--app-text-secondary);
  flex: 1;
}

.li-condx-del {
  margin-left: auto;
}

/* W4 引用行（剧本一·帧4/帧5）：独立编号 + 引用规则描述 + 删行；候选下拉两行
   （描述 + ID 小字）；与条件行之间的分隔线 */
.li-ref-no {
  flex-shrink: 0;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-ref-desc {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--app-text-primary);
  word-break: break-all;
}

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

.li-ref-divider {
  border-top: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
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

/* F：提取段「来自被引规则」分组头（转写提示小字常驻） */
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

.li-condx-targets {
  display: flex;
  gap: 8px;
}

.li-condx-card {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  background: transparent;
  cursor: pointer;
  text-align: left;
}

.li-condx-card.is-active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
}

.li-condx-card-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-condx-card-note {
  font-size: 11px;
  color: var(--app-text-secondary);
  line-height: 1.5;
}

.li-condx-fmt {
  display: flex;
  gap: 8px;
  align-items: flex-start;
}

.li-condx-mode {
  width: 130px;
  flex: none;
}

.li-condx-tpl {
  flex: 1;
  resize: vertical;
  font-family: var(--app-font-mono, monospace);
  font-size: 13px;
  line-height: 1.6;
  padding: 6px 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  background: transparent;
  color: var(--app-text-primary);
}

.li-condx-tpl:focus {
  outline: none;
  border-color: var(--el-color-primary);
}

.li-condx-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.li-condx-chip {
  font-size: 12px;
  padding: 2px 8px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: 10px;
  background: transparent;
  cursor: pointer;
  color: var(--app-text-primary);
}

.li-condx-chip:hover {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}

.li-condx-chip.is-example {
  color: var(--app-text-secondary);
}

.li-condx-chips-sep {
  color: var(--app-border-color-lighter, var(--app-border-color-light));
}

.li-condx-extra {
  display: flex;
  flex-direction: column;
  gap: 6px;
  border-top: 1px dashed var(--app-border-color-lighter, var(--app-border-color-light));
  padding-top: 8px;
}

.li-condx-extra-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.li-condx-addsel {
  width: 180px;
}

.li-condx-inline {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-condx-inline-label {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-condx-minitpl {
  width: 160px;
  resize: none;
  font-family: var(--app-font-mono, monospace);
  font-size: 12px;
  padding: 4px 6px;
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  border-radius: var(--app-radius);
  background: transparent;
  color: var(--app-text-primary);
}

.li-condx-mini {
  width: 150px;
}

.li-condx-num {
  width: 110px;
}

.li-condx-dot {
  color: var(--el-color-primary);
  font-size: 10px;
}

.li-condx-fold {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: var(--app-text-secondary);
}
</style>
