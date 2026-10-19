<template>
  <div class="li-pp">
    <!-- 操作反馈：识别中状态条（复用全景进度事件驱动的 recognizing）与动作通知/错误 -->
    <div v-if="busy" class="li-pp-busy">重识别进行中……完成后本面板与全景自动刷新</div>
    <el-alert v-if="notice" class="li-pp-alert" type="success" :closable="false" show-icon :title="notice" />
    <el-alert v-if="actionError" class="li-pp-alert" type="error" :closable="false" show-icon :title="actionError" />

    <!-- 泛化失败半程态：赋值已保存、规则未生成——赋值不回滚，可原维度重试或保留赋值 -->
    <div v-if="halfState" class="li-pp-half">
      <div class="li-pp-half-title">赋值已保存、规则未生成（{{ genModeLabel(halfState.mode) }}）</div>
      <p class="li-pp-half-reason">{{ halfState.reason }}</p>
      <div class="li-pp-row">
        <el-button size="small" type="primary" :loading="generalizing" @click="retryGeneralize">重试泛化</el-button>
        <el-button size="small" @click="halfState = null">保留赋值，暂不泛化</el-button>
      </div>
      <p class="li-pp-hint">赋值不回滚：本次赋值的会话态覆盖已生效，重识别后按赋值呈现</p>
    </div>

    <!-- 剧本J1 目录面板头（目录模式）：最上目录名 + 两三行信息——层级/相对路径、
         含义覆盖（meanings+meaningRuleId，无则「未覆盖」）、子树文件统计 -->
    <div v-if="dir" class="li-pp-sec">
      <div class="li-pp-dir-head">
        <span class="li-pp-dir-name" :title="dir.relKey || '（库根）'">{{ dir.name }}</span>
        <button type="button" class="li-pp-dir-back" @click="exitDirPanel">返回文件面板</button>
      </div>
      <p class="li-pp-hint">层级：{{ dirLevelText }} · 相对路径 {{ dir.relKey || '（库根）' }}</p>
      <p class="li-pp-hint">含义覆盖：{{ dirMeaningText }}</p>
      <p class="li-pp-hint">子树 {{ dir.subtreeFileCount }} 个文件（含子目录 {{ dir.subdirCount }} 个）</p>
    </div>

    <!-- 待裁决：选中文件冲突维度就地双路裁决（左栏待裁决折叠区保留只读总览）；目录模式无此项 -->
    <div v-if="!dir && pendingItems.length" class="li-pp-sec">
      <h4 class="li-pp-sec-title">待裁决冲突 <span class="li-pp-sec-note">{{ pendingItems.length }} 维 · 就地双路裁决</span></h4>
      <div v-for="p in pendingItems" :key="p.dimension" class="li-pp-pending">
        <div class="li-pp-pending-head">
          <span class="li-pp-pending-dim">{{ dimLabel(p.dimension) }} 冲突</span>
          <span class="li-pp-pending-reason" :title="p.reason">{{ p.reason }}</span>
        </div>
        <div class="li-pp-cands">
          <button
            v-for="(c, j) in p.candidates || []"
            :key="j"
            type="button"
            class="li-pp-cand"
            :class="{ 'is-active': pickByDim[p.dimension] === j }"
            @click="pickByDim[p.dimension] = j"
          >
            <span class="li-pp-cand-value">{{ dimLabel(c.dimension) }}={{ c.value }}</span>
            <span class="li-pp-cand-src">{{ sourceLabel(c.source) }}<template v-if="c.ruleId"> · {{ c.ruleId }}</template></span>
          </button>
        </div>
        <div class="li-pp-row">
          <el-button type="primary" size="small" :loading="adjudicating" @click="doAdjudicatePick(p)">裁决采纳所选（双命中）</el-button>
          <el-button size="small" @click="toggleConditional(p)">按条件改绑（单侧缺席）…</el-button>
        </div>
        <!-- 条件改绑面板：谓词条件策略（在场/缺席各采纳一候选） -->
        <div v-if="condOpenByDim[p.dimension]" class="li-pp-cond">
          <div class="li-pp-cond-row">
            <span class="li-pp-cond-label">条件谓词（何来源在何维度上的声明）：</span>
            <el-select v-model="predSourceByDim[p.dimension]" class="li-pp-cond-select" size="small">
              <el-option v-for="s in predSources" :key="s.key" :label="s.label" :value="s.key" />
            </el-select>
            <el-select v-model="predDimensionByDim[p.dimension]" class="li-pp-cond-select" size="small">
              <el-option v-for="d in predDims" :key="d.key" :label="d.label" :value="d.key" />
            </el-select>
          </div>
          <div class="li-pp-cond-row">
            <span class="li-pp-cond-label">谓词在场时采纳：</span>
            <button
              v-for="(c, j) in p.candidates || []"
              :key="`p${j}`"
              type="button"
              class="li-pp-cand is-mini"
              :class="{ 'is-active': presentByDim[p.dimension] === j }"
              @click="presentByDim[p.dimension] = j"
            >{{ c.value }}</button>
          </div>
          <div class="li-pp-cond-row">
            <span class="li-pp-cond-label">谓词缺席时采纳：</span>
            <button
              v-for="(c, j) in p.candidates || []"
              :key="`a${j}`"
              type="button"
              class="li-pp-cand is-mini"
              :class="{ 'is-active': absentByDim[p.dimension] === j }"
              @click="absentByDim[p.dimension] = j"
            >{{ c.value }}</button>
          </div>
          <div class="li-pp-row">
            <el-button type="primary" size="small" :loading="adjudicating" @click="doAdjudicateConditional(p)">生成条件策略</el-button>
          </div>
          <p class="li-pp-hint">生成「在场时用甲、缺席时用乙」的最简谓词条件策略（来源=裁决）</p>
        </div>
      </div>
    </div>

    <!-- 剧本G1 线索区（赋值表单上方）：结构分段线索集合的可点胶囊陈列
         （未用=灰、已用=亮）；剧本J2 目录模式复用同通路——胶囊来自该目录
         structClues（库根名/各级目录名/N级目录剥壳，无文件名段）；
         库名命中/pixiv 形态等既有线索若存在也一并陈列（只读） -->
    <div v-if="structClueList.length || legacyClues.length" class="li-pp-sec">
      <h4 class="li-pp-sec-title">
        线索
        <span class="li-pp-sec-note">点胶囊选给任意维度；灰=未用、亮=已用</span>
      </h4>
      <div class="li-pp-clues">
        <button
          v-for="c in structClueList"
          :key="c.ref"
          type="button"
          class="li-pp-clue"
          :class="{ 'is-used': isClueUsed(c.ref) }"
          :title="`${c.label}：${c.value}`"
          @click="openClueMenu(c.ref)"
        >
          <span class="li-pp-clue-label">{{ c.label }}</span>
          <span class="li-pp-clue-value">{{ c.value }}</span>
        </button>
        <span
          v-for="(c, i) in legacyClues"
          :key="`legacy-${i}`"
          class="li-pp-clue is-legacy"
          :title="`既有线索（不支持选给）：${legacyClueLabel(c)}`"
        >{{ legacyClueLabel(c) }}</span>
      </div>
    </div>

    <!-- 剧本G2-G3 线索选给的维度选择弹层：作者/作者角色/作品名/标签/作品集 -->
    <div v-if="menuClue" class="li-pp-cluemenu-mask" @click.self="clueMenu = null">
      <div class="li-pp-cluemenu">
        <div class="li-pp-cluemenu-head">
          把「<span class="li-pp-cluemenu-ref">{{ menuClue.label }}：{{ menuClue.value }}</span>」选给维度
        </div>
        <button
          v-for="d in clueDims"
          :key="d.key"
          type="button"
          class="li-pp-cluemenu-item"
          @click="applyClueToDim(d.key)"
        >{{ d.label }}</button>
        <button type="button" class="li-pp-cluemenu-item is-cancel" @click="clueMenu = null">取消</button>
      </div>
    </div>

    <!-- 赋值表单：五字段，已赋值预填现值 + 已赋值/已泛化标记 -->
    <div class="li-pp-sec">
      <h4 class="li-pp-sec-title">
        赋值
        <el-tag v-if="currentAssignment" size="small">已赋值</el-tag>
        <el-tag v-if="currentAssignment && currentAssignment.generalized" size="small" type="success">已泛化</el-tag>
        <span v-if="ignored" class="li-pp-sec-note">已忽略</span>
      </h4>
      <!-- 剧本G2-G3：选给填值 + 来源小字（「自 一级目录(剥壳)」式；作者双来源标「与目录互证」）；
           手填兼容——直接打字即清该字段出处，无出处手填值照常提交（后端总结器兜底匹配） -->
      <div class="li-pp-form">
        <div class="li-pp-field">
          <el-input v-model="formAuthor" placeholder="作者" size="small" clearable @input="onAuthorInput" />
          <span v-if="fieldSourceText('author')" class="li-pp-field-src">{{ fieldSourceText('author') }}</span>
        </div>
        <div class="li-pp-field">
          <el-input v-model="formAuthorRole" placeholder="作者角色（如 原画，可空）" size="small" clearable @input="onAuthorRoleInput" />
          <span v-if="fieldSourceText('authorRole')" class="li-pp-field-src">{{ fieldSourceText('authorRole') }}</span>
        </div>
        <div class="li-pp-field">
          <el-input v-model="formTitle" placeholder="作品名" size="small" clearable @input="onTitleInput" />
          <span v-if="fieldSourceText('title')" class="li-pp-field-src">{{ fieldSourceText('title') }}</span>
        </div>
        <div class="li-pp-field">
          <el-input v-model="formTagsText" placeholder="标签（逗号分隔多个，可空）" size="small" clearable @input="onTagsInput" />
          <span v-if="tagsSourceText()" class="li-pp-field-src">{{ tagsSourceText() }}</span>
        </div>
        <div class="li-pp-field">
          <el-input v-model="formCollection" placeholder="作品集（集归属，可空）" size="small" clearable @input="onCollectionInput" />
          <span v-if="fieldSourceText('collection')" class="li-pp-field-src">{{ fieldSourceText('collection') }}</span>
        </div>
      </div>
      <!-- 剧本J3：目录模式表单下作用域提示小字（N=子树文件数；0 文件=保存禁用并提示「该目录下没有文件」） -->
      <p v-if="dir" class="li-pp-hint">
        <template v-if="dir.subtreeFileCount > 0">保存将作用于 {{ dir.relKey || '库根' }} 下全部 {{ dir.subtreeFileCount }} 个文件（含子目录）</template>
        <template v-else>该目录下没有文件</template>
      </p>
      <!-- 剧本J4：目录模式底部两钮——保存（assignDir 子树合并写）+ 清除目录赋值
           （clearDirAssignments）；文件模式维持单「保存」（G4：赋值入会话后紧随
           「规则建议」卡，泛化维度由线索出处总结器推导，泛化确认=generalize mode=struct） -->
      <div class="li-pp-row">
        <el-button
          v-if="dir"
          type="primary"
          size="small"
          :disabled="dir.subtreeFileCount === 0"
          :loading="assigning || summarizing"
          @click="doSave"
        >保存</el-button>
        <el-button
          v-else
          type="primary"
          size="small"
          :loading="assigning || summarizing"
          @click="doSave"
        >保存</el-button>
        <el-button v-if="dir" size="small" :loading="assigning" @click="doClearDirAssign">清除目录赋值</el-button>
      </div>
      <div v-if="!dir" class="li-pp-row">
        <el-button size="small" :loading="ignoring" @click="doIgnoreAction">{{ ignored ? '取消忽略' : '忽略此文件' }}</el-button>
        <el-button size="small" :loading="assigning" @click="doClearAssign">清除赋值</el-button>
      </div>
      <p v-if="!dir" class="li-pp-hint">空维度不覆盖档案结论；全部留空即清除赋值。保存赋值后自动总结规则建议（手填值由后端按「值=某段」兜底匹配），确认泛化才入档；保存后后端自动排定增量重识别，完成后本面板与全景自动刷新</p>
    </div>

    <!-- 剧本G4 规则建议卡：保存赋值后按选给出处总结（人话规则+命中预览+样例应用），
         未入档；「泛化成规则」=确认入档（G5），「仅保留本次赋值」=收起卡（G6） -->
    <div v-if="suggestion" class="li-pp-sec">
      <h4 class="li-pp-sec-title">
        规则建议
        <span class="li-pp-sec-note">按本次选给总结 · 未入档</span>
      </h4>
      <p class="li-pp-sugg-summary">{{ suggestion.summary }}</p>
      <div v-if="suggestion.mappings && suggestion.mappings.length" class="li-pp-sugg-maps">
        <div v-for="(m, i) in suggestion.mappings" :key="i" class="li-pp-sugg-map">
          <span class="li-pp-sugg-dim">{{ dimLabel(m.dimension) }}</span>
          <span class="li-pp-sugg-arrow">←</span>
          <span class="li-pp-sugg-from">{{ m.from }}</span>
        </div>
      </div>
      <p v-if="suggestion.manualDims && suggestion.manualDims.length" class="li-pp-hint">
        手工指定（不参与泛化）：{{ manualDimsText }}
      </p>
      <!-- 命中预览（文件与目录两路共用）：全库口径；样例行带目录键时文件名前呈目录键 -->
      <p class="li-pp-hint">
        命中预览：将命中全库 {{ suggestion.hitCount }} 个文件（全库共 {{ suggestion.hitTotal }} 个）<template v-if="suggestion.dropsTailCode">；样例尾部码将被丢弃</template>
      </p>
      <div v-if="suggestion.samples && suggestion.samples.length" class="li-pp-sugg-samples">
        <div v-for="(s, i) in suggestion.samples" :key="i" class="li-pp-sugg-sample">
          <span class="li-pp-sugg-file" :title="s.dirRelKey ? `${s.dirRelKey}/${s.fileName}` : s.fileName">{{ s.dirRelKey ? `${s.dirRelKey}/` : '' }}{{ s.fileName }}</span>
          <span class="li-pp-sugg-arrow">→</span>
          <span class="li-pp-sugg-result">{{ s.result }}</span>
        </div>
      </div>
      <div class="li-pp-row">
        <el-button type="primary" size="small" :loading="generalizing" @click="doGeneralizeStruct">泛化成规则</el-button>
        <el-button size="small" @click="dismissSuggestion">仅保留本次赋值</el-button>
      </div>
    </div>

    <!-- 泛化/裁决生成物与验证轮差异提示（完整差异见左栏差异条） -->
    <div v-if="lastGenRule || lastGenRules.length || lastCondition || diffTotal" class="li-pp-sec">
      <h4 class="li-pp-sec-title">
        生成规则与验证差异
        <span class="li-pp-sec-note">变更 {{ diffChangedCount }} · 新增 {{ diffAddedCount }} · 消失 {{ diffRemovedCount }}</span>
      </h4>
      <div v-if="lastGenRule" class="li-pp-gen">
        <p class="li-pp-gen-line"><span class="li-pp-gen-id">{{ lastGenRule.id }}</span>{{ lastGenRule.description }}</p>
        <p v-if="lastGenRule.pattern" class="li-pp-gen-pattern">{{ lastGenRule.pattern }}</p>
      </div>
      <!-- 剧本G5：结构映射确认入档的组合规则序列（generalize mode=struct 的 Rules） -->
      <div v-for="r in lastGenRules" :key="r.id" class="li-pp-gen">
        <p class="li-pp-gen-line"><span class="li-pp-gen-id">{{ r.id }}</span>{{ r.description }}</p>
        <p v-if="r.pattern" class="li-pp-gen-pattern">{{ r.pattern }}</p>
      </div>
      <div v-if="lastCondition" class="li-pp-condjson">
        <div class="li-pp-condjson-head">条件策略生成物（条件 JSON，与规则档案同构的表达式片段）</div>
        <pre class="li-pp-condjson-body">{{ lastCondition }}</pre>
      </div>
      <p v-if="diffTotal" class="li-pp-hint">生成规则后自动增量重识别——过窄＝差异里仍有未覆盖，过泛＝差异里出现错套，请核对</p>
      <div v-if="diffEntries.length" class="li-pp-difflist">
        <div
          v-for="e in diffEntries"
          :key="`${e.kind}/${e.relPath}`"
          class="li-pp-diff"
          :class="`is-${e.kind}`"
        >
          <span class="li-pp-diff-kind">{{ diffKindLabel(e.kind) }}</span>
          <span class="li-pp-diff-path" :title="e.relPath">{{ e.relPath }}</span>
          <span class="li-pp-diff-cols">
            <span class="li-pp-diff-side is-before">{{ conclusionSummary(e.before) }}</span>
            <span class="li-pp-diff-arrow">→</span>
            <span class="li-pp-diff-side is-after">{{ conclusionSummary(e.after) }}</span>
          </span>
        </div>
        <p v-if="diffTotal > DIFF_CAP" class="li-pp-hint">仅显示前 {{ DIFF_CAP }} 条，完整差异见左栏差异条</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'

// 右栏解析面板（阶段3 · 剧本G v2 赋值即映射 + 阶段4 · 剧本J 目录模式）：
// 文件模式——线索集胶囊陈列（G1 未用灰/已用亮，含既有库名命中/pixiv 线索）→
// 点线索弹维度选择选给五字段+来源小字（G2-G3，作者双来源标「与目录互证」，
// 互证仅在用户显式赋第二条跨族等值线索时点亮——剧本G修订；出处 clueRef 随
// assign 载荷入会话）→ 保存后「规则建议」卡（G4 summarizeRule：人话规则+
// 全库命中预览+样例应用）→ 泛化成规则（G5 generalize mode=struct 入档+
// 验证轮）/ 仅保留本次赋值（G6 收起卡）；手填兼容=打字清出处、无出处值照常
// 提交（后端按「值=某段」兜底匹配）。目录模式（剧本J，dir 上下文由 ParseView
// 组装传入）——目录头信息（J1）+ 目录线索胶囊（J2）+ 五字段表单与子树作用域
// 提示（J3）+ 保存 assignDir / 清除目录赋值 clearDirAssignments（J4）+ 建议
// 卡泛化 generalize 目录寻址（J5）；与文件面板互斥使用、互不联动（J7）。
// 其余：忽略/清除、待裁决就地双路裁决、生成规则与差异提示。
// 请求经父组件 ParseView 的既有 workbench 请求通道（request 以 prop 注入，
// 不另建事件通道，避免双通道序号错配）；识别事件（recognitionProgress/
// recognitionDone）也由父组件统一监听并重取全景+决策态，本组件只消费
// recognizing/diff/assignments 等投影——赋值/清除应答后前端不再发识别请求。

interface PanelFile {
  relPath: string
  fileName: string
  state: string
  assigned?: boolean
  // 剧本G1：结构分段线索集（getPanorama 每文件投影，ref 空间与 assign 出处共用）
  structClues?: PanelStructClue[]
  // 既有疑似线索（库名命中/pixiv 形态等；只读陈列，不支持选给）
  clues?: { kind: string; detail: string }[]
}

// 剧本J 目录模式上下文（ParseView 组装传入）：relKey 空串=库根（全树寻址）；
// structClues 为该目录级结构分段线索集（库根名/各级目录名/剥壳变体，无文件名段）；
// subtreeFileCount 按 snapshot.files 的 dirRelKey 前缀统计、subdirCount 按
// snapshot.dirs 前缀统计（不含自身）——树节点聚合受清单过滤影响不可用
interface PanelDirContext {
  relKey: string
  name: string
  depth: number
  meanings?: { type: string; id?: string; name?: string }[]
  meaningRuleId?: string
  structClues?: PanelStructClue[]
  subtreeFileCount: number
  subdirCount: number
}

// 结构分段线索（后端 SnapStructClue：ref/kind/label/value）
interface PanelStructClue {
  ref: string
  kind: string
  label: string
  value: string
}

interface PanelDecl {
  dimension: string
  value: string
  source: string
  roleName?: string
  ruleId?: string
}

interface PanelPending {
  relPath: string
  fileName: string
  dimension: string
  reason: string
  candidates: PanelDecl[]
}

interface PanelRule {
  id: string
  description: string
  pattern?: string
  condition?: string
}

interface PanelDiffConclusion {
  state?: string
  final?: PanelDecl[]
  pendingDims?: string[]
}

interface PanelDiffEntry {
  relPath: string
  before?: PanelDiffConclusion
  after?: PanelDiffConclusion
}

interface PanelDiff {
  changed?: PanelDiffEntry[]
  added?: PanelDiffEntry[]
  removed?: PanelDiffEntry[]
}

interface PanelAssignment {
  relPath: string
  author?: string
  authorRole?: string
  title?: string
  tags?: string[]
  collection?: string
  generalized?: boolean
  updatedAtMs?: number
  // 剧本G2-G3：各维度赋值的线索出处引用（assignment/decisions.assignments 回透，
  // 供表单预填时恢复「自 一级目录(剥壳)」来源小字；TagClueRefs 与 Tags 逐位平行）
  authorClueRef?: string
  titleClueRef?: string
  collectionClueRef?: string
  tagClueRefs?: string[]
}

// 应答窄投影：父通道应答结构是其超集（结构兼容，见 request prop 类型）
interface PanelResponse {
  ok: boolean
  error?: string
  rule?: PanelRule
  // 剧本G5：generalize mode=struct 入档的组合规则摘要序列
  rules?: PanelRule[]
  // 剧本G4：summarizeRule 的总结器产物（建议卡载荷；未入档）
  ruleSuggestion?: PanelRuleSuggestion
  // 剧本J4：assignDir/clearDirAssignments 的生效文件计数（子树合并写触及数/清除数）
  affected?: number
}

// 剧本G4 规则建议（后端 RuleSuggestion：人话规则+逐维度映射+命中预览+样例应用）
interface PanelRuleSuggestionMapping {
  dimension: string
  from: string
  clueRef?: string
}
interface PanelRuleSuggestionSample {
  fileName: string
  // 样例文件所在目录键（跨目录样例呈现：文件名前呈目录键；空=库根直下不呈前缀）
  dirRelKey?: string
  result: string
}
interface PanelRuleSuggestion {
  relPath: string
  summary: string
  mappings?: PanelRuleSuggestionMapping[]
  manualDims?: string[]
  rules?: PanelRule[]
  hitCount: number
  hitTotal: number
  samples?: PanelRuleSuggestionSample[]
  dropsTailCode?: boolean
}

const props = defineProps<{
  root: string
  // 文件模式上下文（目录模式实例缺省；两模式互斥使用）
  file?: PanelFile | null
  // 剧本J 目录模式上下文（文件模式实例缺省）
  dir?: PanelDirContext | null
  assignments: PanelAssignment[]
  ignored: boolean
  pendingItems: PanelPending[]
  diff: PanelDiff | null
  recognizing: boolean
  request: (action: string, payload: Record<string, unknown>) => Promise<PanelResponse>
}>()

const emit = defineEmits<{
  (e: 'refresh'): void
  // 剧本J7：目录面板「返回文件面板」（清目录选中态，互斥切回文件面板）
  (e: 'exitDir'): void
}>()

// —— 反馈态 ——
const notice = ref('')
const actionError = ref('')
// 泛化/裁决成功后验证轮在途的乐观标记：与父组件 recognizing 合成忙碌态，
// recognitionDone（recognizing 转 false）后清除
const verifyStarted = ref(false)
const busy = computed(() => props.recognizing || verifyStarted.value)
watch(
  () => props.recognizing,
  (v) => {
    if (!v) verifyStarted.value = false
  }
)

// 泛化失败半程态（赋值已保存、规则未生成）：记失败原因与当时的泛化维度，可原维度重试
const halfState = ref<{ reason: string; mode: string } | null>(null)

// —— 赋值表单（五字段，与 SubpageManualParse 字段定义一致） ——
const formAuthor = ref('')
const formAuthorRole = ref('')
const formTitle = ref('')
const formTagsText = ref('')
const formCollection = ref('')
const assigning = ref(false)
const ignoring = ref(false)
// 用户是否改动过表单：改动后自动刷新（assignments 投影更新）不再覆盖预填
let formDirty = false

function markDirty(): void {
  formDirty = true
}

const currentAssignment = computed<PanelAssignment | null>(() => {
  const rel = props.file ? props.file.relPath : ''
  if (!rel) return null
  return props.assignments.find((a) => a.relPath === rel) || null
})

function setForm(a: PanelAssignment | null): void {
  formAuthor.value = (a && a.author) || ''
  formAuthorRole.value = (a && a.authorRole) || ''
  formTitle.value = (a && a.title) || ''
  formTagsText.value = (a && a.tags ? a.tags.join('，') : '') || ''
  formCollection.value = (a && a.collection) || ''
  // 剧本G2-G3：出处随赋值回透恢复（作者角色无后端出处字段，仅会话内呈现）
  srcAuthor.value = (a && a.authorClueRef) || ''
  srcTitle.value = (a && a.titleClueRef) || ''
  srcCollection.value = (a && a.collectionClueRef) || ''
  srcAuthorRole.value = ''
  srcTags.value = (a && a.tagClueRefs ? a.tagClueRefs.slice() : [])
  crossAuthorRefs.value = []
  formDirty = false
}

// —— 剧本G1-G3 + 剧本J2：线索集投影与赋值出处（选给=值+clueRef 同时进表单与 assign 载荷；
//     目录模式取该目录 structClues，文件模式取该文件的——ref 空间两路共用） ——
const structClueList = computed<PanelStructClue[]>(() =>
  (props.dir ? props.dir.structClues : props.file && props.file.structClues) || []
)
const legacyClues = computed(() => (props.file && props.file.clues) || [])
const clueByRef = computed<Map<string, PanelStructClue>>(() => {
  const m = new Map<string, PanelStructClue>()
  for (const c of structClueList.value) m.set(c.ref, c)
  return m
})
// 各字段当前出处（线索 ref）；作者角色无后端 clueRef 字段，仅本地小字呈现
const srcAuthor = ref('')
const srcAuthorRole = ref('')
const srcTitle = ref('')
const srcCollection = ref('')
// 标签出处：与 tags 逐位平行（手编破坏平行时整体退无出处）
const srcTags = ref<string[]>([])
// 作者互证来源（剧本G3「与目录互证」；呈现态，载荷仍只发主出处）
const crossAuthorRefs = ref<string[]>([])
// 目录族线索类别（互证判定：主出处与佐证分属目录族/文件名族）
const dirKindSet = new Set(['rootName', 'dirLevel', 'dirLevelStripped'])

// 与后端 normalizeNameHint 同规则：小写 + '-'/' ' 折叠为 '_'（互证判定的值等价）
function normalizeHint(v: string): string {
  return v.toLowerCase().replace(/[- ]/g, '_')
}

// 剧本G修订（互证时机）：互证仅当用户把第二条跨族等值线索显式赋给作者
// （crossAuthorRefs 非空，由 applyClueToDim 的作者双来源分支登记）时为真；
// 线索集自身的值等价不再自动点亮——单来源赋作者小字只有「自 <来源>」
const authorCorroborated = computed(() => crossAuthorRefs.value.length > 0)

function isClueUsed(ref: string): boolean {
  return (
    srcAuthor.value === ref ||
    srcAuthorRole.value === ref ||
    srcTitle.value === ref ||
    srcCollection.value === ref ||
    srcTags.value.indexOf(ref) >= 0 ||
    crossAuthorRefs.value.indexOf(ref) >= 0
  )
}

function clueLabelOf(ref: string): string {
  const c = clueByRef.value.get(ref)
  return c ? c.label : ref
}

// 来源小字：「自 一级目录(剥壳)」式；作者双来源时追加「与目录互证」
function fieldSourceText(dim: 'author' | 'authorRole' | 'title' | 'collection'): string {
  const ref =
    dim === 'author' ? srcAuthor.value
      : dim === 'authorRole' ? srcAuthorRole.value
        : dim === 'title' ? srcTitle.value
          : srcCollection.value
  if (!ref) return ''
  let text = `自 ${clueLabelOf(ref)}`
  if (dim === 'author' && authorCorroborated.value) text += '；与目录互证'
  return text
}

function tagsSourceText(): string {
  const labels: string[] = []
  for (const r of srcTags.value) {
    if (r && labels.indexOf(clueLabelOf(r)) < 0) labels.push(clueLabelOf(r))
  }
  return labels.length ? `自 ${labels.join('、')}` : ''
}

// 手填兼容：打字即清该字段出处（无出处手填值照常提交，后端按「值=某段」兜底匹配）
function onAuthorInput(): void {
  markDirty()
  srcAuthor.value = ''
  crossAuthorRefs.value = []
}
function onAuthorRoleInput(): void {
  markDirty()
  srcAuthorRole.value = ''
}
function onTitleInput(): void {
  markDirty()
  srcTitle.value = ''
}
function onCollectionInput(): void {
  markDirty()
  srcCollection.value = ''
}
function onTagsInput(): void {
  markDirty()
  srcTags.value = []
}

// —— 剧本G2-G3：点线索 → 维度选择弹层 → 选给 ——
const clueMenu = ref<string | null>(null)
const menuClue = computed(() => (clueMenu.value ? clueByRef.value.get(clueMenu.value) || null : null))
const clueDims = [
  { key: 'author', label: '作者' },
  { key: 'authorRole', label: '作者角色' },
  { key: 'title', label: '作品名' },
  { key: 'tag', label: '标签' },
  { key: 'collection', label: '作品集' }
]

function openClueMenu(ref: string): void {
  clueMenu.value = ref
  notice.value = ''
  actionError.value = ''
}

function applyClueToDim(dim: string): void {
  const clue = menuClue.value
  clueMenu.value = null
  if (!clue) return
  notice.value = ''
  actionError.value = ''
  if (dim === 'author') {
    const cur = srcAuthor.value ? clueByRef.value.get(srcAuthor.value) : null
    if (cur && normalizeHint(cur.value) === normalizeHint(clue.value) && dirKindSet.has(cur.kind) !== dirKindSet.has(clue.kind)) {
      // 剧本G3 作者双来源：值保持主出处值，新线索记为互证（呈现态）
      if (crossAuthorRefs.value.indexOf(clue.ref) < 0) crossAuthorRefs.value = [...crossAuthorRefs.value, clue.ref]
    } else {
      formAuthor.value = clue.value
      srcAuthor.value = clue.ref
      crossAuthorRefs.value = []
    }
  } else if (dim === 'authorRole') {
    formAuthorRole.value = clue.value
    srcAuthorRole.value = clue.ref
  } else if (dim === 'title') {
    formTitle.value = clue.value
    srcTitle.value = clue.ref
  } else if (dim === 'collection') {
    formCollection.value = clue.value
    srcCollection.value = clue.ref
  } else if (dim === 'tag') {
    // 标签追加：文本追加一段，出处数组与解析后 tags 保持逐位平行
    const parts = formTagsText.value
      .split(/[,，、]/)
      .map((t) => t.trim())
      .filter((t) => t.length > 0)
    const refs = srcTags.value.slice(0, parts.length)
    while (refs.length < parts.length) refs.push('')
    refs.push(clue.ref)
    srcTags.value = refs
    formTagsText.value = parts.length ? `${parts.join('，')}，${clue.value}` : clue.value
  }
  formDirty = true
}

// —— 剧本G4-G6：规则建议与泛化确认（泛化维度由总结器按线索出处推导，前端不再三选） ——
const generalizing = ref(false)
// 建议卡（summarizeRule 产物；保存赋值后出现，双路按钮=泛化成规则/仅保留本次赋值）
const suggestion = ref<PanelRuleSuggestion | null>(null)
const summarizing = ref(false)
// 结构映射确认入档的组合规则序列（generalize mode=struct 应答 Rules）
const lastGenRules = ref<PanelRule[]>([])

function genModeLabel(mode: string): string {
  return mode === 'struct' ? '结构映射' : mode
}

// —— 待裁决就地双路（维度键控的裁决选择态） ——
const adjudicating = ref(false)
const pickByDim = ref<Record<string, number>>({})
const condOpenByDim = ref<Record<string, boolean>>({})
const predSourceByDim = ref<Record<string, string>>({})
const predDimensionByDim = ref<Record<string, string>>({})
const presentByDim = ref<Record<string, number>>({})
const absentByDim = ref<Record<string, number>>({})

const predSources = [
  { key: 'dir', label: '目录源' },
  { key: 'filename', label: '文件名源' },
  { key: 'predicate', label: '库内命中源' },
  { key: 'group', label: '群组前缀源' },
  { key: 'constant', label: '常量' }
]
const predDims = [
  { key: 'author', label: '作者' },
  { key: 'title', label: '作品名' },
  { key: 'tag', label: '标签' },
  { key: 'collection', label: '作品集' }
]

// —— 生成物与差异呈现 ——
const lastGenRule = ref<PanelRule | null>(null)
const lastCondition = ref('')
const DIFF_CAP = 8
const diffChangedCount = computed(() => (props.diff && props.diff.changed ? props.diff.changed.length : 0))
const diffAddedCount = computed(() => (props.diff && props.diff.added ? props.diff.added.length : 0))
const diffRemovedCount = computed(() => (props.diff && props.diff.removed ? props.diff.removed.length : 0))
const diffTotal = computed(() => diffChangedCount.value + diffAddedCount.value + diffRemovedCount.value)
const diffEntries = computed<{ kind: string; relPath: string; before?: PanelDiffConclusion; after?: PanelDiffConclusion }[]>(() => {
  const d = props.diff
  if (!d) return []
  const cap = (arr?: PanelDiffEntry[], kind = '') => (arr || []).slice(0, DIFF_CAP).map((e) => ({ kind, ...e }))
  return [...cap(d.changed, 'changed'), ...cap(d.added, 'added'), ...cap(d.removed, 'removed')]
})

// —— 面板复位：点行切换文件/点目录名切换目录时表单与裁决/反馈态跟随重置
//    （文件模式按现值预填；目录模式无单点赋值存储，从空表单起） ——
function resetPanelState(): void {
  setForm(currentAssignment.value)
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  lastGenRule.value = null
  lastCondition.value = ''
  lastGenRules.value = []
  suggestion.value = null
  clueMenu.value = null
  verifyStarted.value = false
  pickByDim.value = {}
  condOpenByDim.value = {}
  predSourceByDim.value = {}
  predDimensionByDim.value = {}
  presentByDim.value = {}
  absentByDim.value = {}
}

watch(
  () => props.file && props.file.relPath,
  () => {
    resetPanelState()
  }
)

// 剧本J7：点目录名切换目录面板目标（relKey 变化才复位；快照刷新重建 dir 上下文对象不触发）
watch(
  () => props.dir && props.dir.relKey,
  () => {
    resetPanelState()
  }
)

// 自动刷新落盘（assignments 投影更新）：表单未被用户改动时跟随预填现值（含已泛化标记随标记位刷新）；
// 目录模式无赋值预填源，刷新不覆写目录表单（保存后表单内容保留）
watch(
  () => props.assignments,
  () => {
    if (!formDirty && !props.dir) setForm(currentAssignment.value)
  }
)

// 初挂载即按现值预填（选中态先于组件挂载就位）
resetPanelState()

// —— 赋值动作（动作协议零改动，经父通道串行调用） ——

async function runAssign(): Promise<boolean> {
  if (!props.root || !props.file) {
    actionError.value = '会话或目标文件缺失，无法保存赋值'
    return false
  }
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  const tags = formTagsText.value
    .split(/[,，、]/)
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
  // 剧本G2-G3：选给出的处随赋值入会话（authorClueRef 等）；手填字段无出处照常
  // 提交（后端总结器按「值=某段」兜底匹配）。tagClueRefs 仅在与 tags 逐位平行时携带。
  const payload: Record<string, unknown> = {
    root: props.root,
    relPath: props.file.relPath,
    author: formAuthor.value.trim(),
    authorRole: formAuthorRole.value.trim(),
    title: formTitle.value.trim(),
    tags: tags,
    collection: formCollection.value.trim()
  }
  if (srcAuthor.value) payload.authorClueRef = srcAuthor.value
  if (srcTitle.value) payload.titleClueRef = srcTitle.value
  if (srcCollection.value) payload.collectionClueRef = srcCollection.value
  if (tags.length && srcTags.value.length === tags.length && srcTags.value.some((r) => r)) {
    payload.tagClueRefs = srcTags.value.slice()
  }
  const resp = await props.request('assign', payload)
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '保存赋值失败'
    return false
  }
  return true
}

// 剧本G4 保存（文件）/ 剧本J4 保存（目录）：赋值入会话 → 紧随「规则建议」卡
//（summarizeRule：人话规则+全库命中预览+样例）
async function doSave(): Promise<void> {
  if (props.dir) {
    await doSaveDir()
    return
  }
  const ok = await runAssign()
  if (!ok) return
  formDirty = false
  notice.value = '已保存赋值（重识别后生效，识别完成本面板自动刷新）'
  suggestion.value = null
  // 全部留空=清除赋值路径：无建议卡
  const tags = formTagsText.value
    .split(/[,，、]/)
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
  if (!formAuthor.value.trim() && !formAuthorRole.value.trim() && !formTitle.value.trim() && !formCollection.value.trim() && !tags.length) {
    return
  }
  summarizing.value = true
  actionError.value = ''
  const resp = await props.request('summarizeRule', {
    root: props.root,
    relPath: props.file ? props.file.relPath : ''
  })
  summarizing.value = false
  if (!resp.ok || !resp.ruleSuggestion) {
    // 纯手工指定（无可结构映射维度）：无建议卡，单次赋值语义（剧本G 手填兼容）
    notice.value = `已保存赋值；未产生规则建议（${resp.error || '无可泛化内容'}）`
    return
  }
  suggestion.value = resp.ruleSuggestion
}

// 剧本J4 保存（目录）：assignDir 子树合并写（非空维度覆盖、空维度不动）→
// 成功通知用 affected（生效文件数）→ summarizeRule 目录寻址（relPath 缺省+
// dirRelKey 在场，样本=子树首个有赋值文件）出建议卡；后端已排自动增量重识别，
// 前端仅重取全景（不发识别请求——与 assign 同约定）。失败时动作 error 上浮
// actionError，表单内容保留。
async function doSaveDir(): Promise<void> {
  const dir = props.dir
  if (!props.root || !dir) {
    actionError.value = '会话或目标目录缺失，无法保存赋值'
    return
  }
  // 双保险：按钮已按 0 文件禁用（J3）
  if (dir.subtreeFileCount <= 0) {
    actionError.value = '该目录下没有文件'
    return
  }
  const tags = formTagsText.value
    .split(/[,，、]/)
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
  if (!formAuthor.value.trim() && !formAuthorRole.value.trim() && !formTitle.value.trim() && !formCollection.value.trim() && !tags.length) {
    // 目录赋值无「全部留空=清除」语义（清除走「清除目录赋值」钮）；文案与后端拦截一致
    actionError.value = '目录赋值未指定任何维度，无法应用'
    return
  }
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  const payload: Record<string, unknown> = {
    root: props.root,
    dirRelKey: dir.relKey,
    author: formAuthor.value.trim(),
    authorRole: formAuthorRole.value.trim(),
    title: formTitle.value.trim(),
    tags: tags,
    collection: formCollection.value.trim()
  }
  if (srcAuthor.value) payload.authorClueRef = srcAuthor.value
  if (srcTitle.value) payload.titleClueRef = srcTitle.value
  if (srcCollection.value) payload.collectionClueRef = srcCollection.value
  if (tags.length && srcTags.value.length === tags.length && srcTags.value.some((r) => r)) {
    payload.tagClueRefs = srcTags.value.slice()
  }
  const resp = await props.request('assignDir', payload)
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '保存目录赋值失败'
    return
  }
  formDirty = false
  const affected = resp.affected || 0
  notice.value = `已对 ${affected} 个文件保存赋值（重识别后生效）`
  emit('refresh')
  suggestion.value = null
  summarizing.value = true
  actionError.value = ''
  const sresp = await props.request('summarizeRule', {
    root: props.root,
    dirRelKey: dir.relKey
  })
  summarizing.value = false
  if (!sresp.ok || !sresp.ruleSuggestion) {
    notice.value = `已对 ${affected} 个文件保存赋值；未产生规则建议（${sresp.error || '无可泛化内容'}）`
    return
  }
  suggestion.value = sresp.ruleSuggestion
}

// 剧本G6 仅保留本次赋值：卡收起，现状单次赋值语义（不泛化=不发请求）
function dismissSuggestion(): void {
  suggestion.value = null
  notice.value = '已保留本次赋值（未泛化）'
}

// 剧本G5 泛化成规则：generalize mode=struct 确认入档 + 自动验证轮（复用现有通知通路）
async function doGeneralizeStruct(): Promise<void> {
  await runGeneralize('struct')
}

async function runGeneralize(mode: string): Promise<void> {
  if (!props.root) return
  // 剧本J5：目录模式=目录寻址（relPath 缺省+dirRelKey 在场，样本=子树首个有
  // 赋值文件）；验证轮流程（verifyStarted/差异）两路共用不变
  const target: Record<string, unknown> | null = props.dir
    ? { dirRelKey: props.dir.relKey }
    : props.file
      ? { relPath: props.file.relPath }
      : null
  if (!target) return
  generalizing.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('generalize', {
    root: props.root,
    generalizeMode: mode,
    ...target
  })
  generalizing.value = false
  const rules = resp.rules && resp.rules.length ? resp.rules : resp.rule ? [resp.rule] : []
  if (!resp.ok || !rules.length) {
    // 半程态：赋值已保存、规则未生成——保留失败原因与泛化维度供重试，赋值不回滚
    halfState.value = { reason: resp.error || '泛化生成失败', mode: mode }
    return
  }
  halfState.value = null
  // struct 为组合规则序列（建议卡收起，结论转「生成规则与验证差异」区呈现）
  lastGenRules.value = rules
  lastGenRule.value = null
  lastCondition.value = ''
  suggestion.value = null
  const ids = rules.map((r) => r.id).join('、')
  notice.value = `已生成规则 ${ids}；验证轮重识别已启动，差异见下方`
  verifyStarted.value = true
}

async function retryGeneralize(): Promise<void> {
  if (!halfState.value) return
  await runGeneralize(halfState.value.mode)
}

// 清除赋值：后端 ApplyAssignment 已排自动刷新，应答后无需前端再发识别请求
async function doClearAssign(): Promise<void> {
  if (!props.root || !props.file) return
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  const resp = await props.request('clearAssignment', {
    root: props.root,
    relPath: props.file.relPath
  })
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '清除赋值失败'
    return
  }
  setForm(null)
  lastGenRule.value = null
  lastCondition.value = ''
  lastGenRules.value = []
  suggestion.value = null
  notice.value = '已清除赋值（重识别后生效）'
}

// 剧本J4 清除目录赋值：clearDirAssignments 删子树全部文件赋值（含已泛化标记，
// affected=清除数）→ 通知+重取全景；后端已排自动重识别，前端不发识别请求
async function doClearDirAssign(): Promise<void> {
  const dir = props.dir
  if (!props.root || !dir) return
  assigning.value = true
  notice.value = ''
  actionError.value = ''
  halfState.value = null
  const resp = await props.request('clearDirAssignments', {
    root: props.root,
    dirRelKey: dir.relKey
  })
  assigning.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '清除目录赋值失败'
    return
  }
  setForm(null)
  lastGenRule.value = null
  lastCondition.value = ''
  lastGenRules.value = []
  suggestion.value = null
  notice.value = `已清除目录下 ${resp.affected || 0} 个文件的赋值（重识别后生效）`
  emit('refresh')
}

// 忽略/取消忽略：忽略不触发识别，应答后仅请父组件重取全景（含决策态）
async function doIgnoreAction(): Promise<void> {
  if (!props.root || !props.file) return
  ignoring.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('setIgnored', {
    root: props.root,
    relPath: props.file.relPath,
    ignored: !props.ignored
  })
  ignoring.value = false
  if (!resp.ok) {
    actionError.value = resp.error || '忽略项登记失败'
    return
  }
  notice.value = props.ignored ? '已取消忽略' : '已忽略此文件'
  emit('refresh')
}

// —— 就地裁决双路 ——

function toggleConditional(p: PanelPending): void {
  const k = p.dimension
  condOpenByDim.value[k] = !condOpenByDim.value[k]
  if (condOpenByDim.value[k]) {
    // 缺省：谓词=目录源作者、在场=首个候选、缺席=次候选
    if (predSourceByDim.value[k] === undefined) predSourceByDim.value[k] = 'dir'
    if (predDimensionByDim.value[k] === undefined) predDimensionByDim.value[k] = 'author'
    if (presentByDim.value[k] === undefined) presentByDim.value[k] = 0
    if (absentByDim.value[k] === undefined) absentByDim.value[k] = 1
  }
  notice.value = ''
  actionError.value = ''
}

// 双路一：双命中择一（pick）——两来源声明均在场，采纳所选候选生成组合规则
async function doAdjudicatePick(p: PanelPending): Promise<void> {
  if (!props.root) return
  if (pickByDim.value[p.dimension] === undefined) {
    actionError.value = '请先点选要采纳的候选'
    return
  }
  adjudicating.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('adjudicate', {
    root: props.root,
    relPath: p.relPath,
    dimension: p.dimension,
    adjudicateMode: 'pick',
    pickIndex: pickByDim.value[p.dimension]
  })
  adjudicating.value = false
  if (!resp.ok || !resp.rule) {
    actionError.value = resp.error || '裁决生成失败'
    return
  }
  lastGenRule.value = resp.rule
  lastCondition.value = resp.rule.condition || ''
  lastGenRules.value = []
  notice.value = `已生成组合规则 ${resp.rule.id}；验证轮重识别已启动，差异见下方`
  verifyStarted.value = true
}

// 双路二：单侧缺席条件改绑（conditional）——生成「在场用甲、缺席用乙」的谓词条件策略
async function doAdjudicateConditional(p: PanelPending): Promise<void> {
  if (!props.root) return
  adjudicating.value = true
  notice.value = ''
  actionError.value = ''
  const resp = await props.request('adjudicate', {
    root: props.root,
    relPath: p.relPath,
    dimension: p.dimension,
    adjudicateMode: 'conditional',
    predicateSource: predSourceByDim.value[p.dimension] || 'dir',
    predicateDimension: predDimensionByDim.value[p.dimension] || 'author',
    presentIndex: presentByDim.value[p.dimension] || 0,
    absentIndex: absentByDim.value[p.dimension] === undefined ? 1 : absentByDim.value[p.dimension]
  })
  adjudicating.value = false
  if (!resp.ok || !resp.rule) {
    actionError.value = resp.error || '条件策略生成失败'
    return
  }
  lastGenRule.value = resp.rule
  lastCondition.value = resp.rule.condition || ''
  lastGenRules.value = []
  condOpenByDim.value[p.dimension] = false
  notice.value = `已生成条件策略 ${resp.rule.id}；验证轮重识别已启动，差异见下方`
  verifyStarted.value = true
}

// —— 呈现辅助（与 SubpageManualParse 同源词条） ——

const dimLabel = (dim: string): string => {
  switch (dim) {
    case 'author': return '作者'
    case 'authorRole': return '作者角色'
    case 'title': return '作品名'
    case 'tag': return '标签'
    case 'collection': return '作品集'
    default: return dim
  }
}

// —— 剧本J1 目录面板头呈现辅助 ——

// 层级标签（depth 0=库根；n≥1 与后端 dirLevelLabel 的 1 基序数一致）
const dirLevelText = computed(() => {
  const d = props.dir
  if (!d) return ''
  if (d.depth <= 0) return '库根'
  const words = ['', '一', '二', '三', '四', '五', '六', '七', '八', '九', '十']
  if (d.depth < words.length) return `${words[d.depth]}级目录`
  return `第 ${d.depth} 级目录`
})

// 含义覆盖行：meanings 逐条人话 + 来源规则名；无则「未覆盖」（与 ParseView meaningLabel 同源）
const dirMeaningText = computed(() => {
  const d = props.dir
  if (!d) return ''
  const list = d.meanings || []
  if (!list.length) return '未覆盖'
  let text = list.map(meaningLabel).join('、')
  if (d.meaningRuleId) text += `（来源规则 ${d.meaningRuleId}）`
  return text
})

function meaningLabel(m: { type: string; name?: string }): string {
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

// 剧本J7：目录面板「返回文件面板」（互斥切回，由 ParseView 清目录选中态）
function exitDirPanel(): void {
  emit('exitDir')
}

// 剧本G1：既有线索（库名命中/pixiv 形态等）的陈列词条（与 ParseView clueLabel 同源）
const legacyClueLabel = (c: { kind: string; detail: string }): string => {
  switch (c.kind) {
    case 'predicate-author': return `库内作者：${c.detail}`
    case 'predicate-tag': return `库内标签：${c.detail}`
    case 'pixiv-id': return `pixiv 形态：${c.detail}`
    case 'group-prefix': return `集名候选：${c.detail}`
    case 'set-name-match': return `集名互证：${c.detail}`
    default: return c.detail
  }
}
// 建议卡的手工指定维度说明（值不对应任何段，不参与泛化）
const manualDimsText = computed(() =>
  ((suggestion.value && suggestion.value.manualDims) || []).map((d) => dimLabel(d)).join('、')
)
const sourceLabel = (source: string): string => {
  switch (source) {
    case 'dir': return '目录'
    case 'filename': return '文件名'
    case 'constant': return '常量'
    case 'group': return '群组前缀'
    case 'predicate': return '库内命中'
    case 'assignment': return '手工赋值'
    case 'adjudication': return '裁决'
    default: return source
  }
}
const diffKindLabel = (kind: string): string => {
  switch (kind) {
    case 'changed': return 'Δ变更'
    case 'added': return '＋新增'
    case 'removed': return '－消失'
    default: return kind
  }
}
const conclusionSummary = (c?: PanelDiffConclusion | null): string => {
  if (!c) return '（无）'
  const parts: string[] = []
  for (const d of c.final || []) parts.push(`${dimLabel(d.dimension)}=${d.value}`)
  if (c.pendingDims && c.pendingDims.length) parts.push(`待裁决:${c.pendingDims.join('/')}`)
  if (!parts.length) return c.state || '（无）'
  return `${c.state || ''}（${parts.join('、')}）`
}
</script>

<style scoped>
.li-pp {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.li-pp-alert {
  border-radius: var(--app-radius);
}

.li-pp-busy {
  font-size: 12px;
  padding: 5px 10px;
  border-radius: var(--app-radius);
  background: var(--app-tag-warning-bg);
  color: var(--app-tag-warning-text);
}

.li-pp-sec {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.li-pp-sec-title {
  margin: 0;
  font-size: 13px;
  color: var(--app-text-primary);
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pp-sec-note {
  font-size: 12px;
  font-weight: normal;
  color: var(--app-text-secondary);
}

.li-pp-hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-secondary);
}

.li-pp-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

/* 半程态：泛化失败的半程提示块 */
.li-pp-half {
  border: 1px solid var(--app-tag-warning-bg);
  border-radius: var(--app-radius);
  background: var(--app-tag-warning-bg);
  padding: 10px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pp-half-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--app-tag-warning-text);
}

.li-pp-half-reason {
  margin: 0;
  font-size: 12px;
  color: var(--app-tag-warning-text);
  word-break: break-all;
}

/* 赋值表单（窄栏纵排） */
.li-pp-form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

/* 剧本J1 目录面板头：目录名 + 返回文件面板 */
.li-pp-dir-head {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.li-pp-dir-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-pp-dir-back {
  margin-left: auto;
  flex-shrink: 0;
  border: none;
  background: none;
  padding: 0;
  font-size: 12px;
  cursor: pointer;
  color: var(--app-color-primary);
}

/* 剧本G1 线索胶囊（未用=灰、已用=亮） */
.li-pp-clues {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.li-pp-clue {
  border: 1px solid var(--app-border-color-light);
  border-radius: 999px;
  background: var(--app-bg-surface);
  padding: 2px 10px;
  cursor: pointer;
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  max-width: 100%;
  min-width: 0;
  color: var(--app-text-secondary);
}

.li-pp-clue:hover {
  border-color: var(--app-color-primary);
}

.li-pp-clue.is-used {
  border-color: var(--app-color-primary);
  color: var(--app-color-primary);
  background: var(--app-color-primary-light, rgba(64, 158, 255, 0.08));
}

.li-pp-clue.is-legacy {
  cursor: default;
  border-style: dashed;
  font-size: 11px;
}

.li-pp-clue-label {
  flex-shrink: 0;
  font-size: 11px;
}

.li-pp-clue-value {
  font-size: 12px;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-pp-clue.is-used .li-pp-clue-value {
  color: var(--app-color-primary);
}

/* 剧本G2-G3 线索选给的维度选择弹层 */
.li-pp-cluemenu-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.25);
  z-index: 50;
  display: flex;
  align-items: center;
  justify-content: center;
}

.li-pp-cluemenu {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 10px 12px;
  min-width: 220px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
}

.li-pp-cluemenu-head {
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-pp-cluemenu-ref {
  color: var(--app-text-primary);
}

.li-pp-cluemenu-item {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 5px 10px;
  cursor: pointer;
  text-align: left;
  font-size: 12px;
  color: var(--app-text-regular);
}

.li-pp-cluemenu-item:hover {
  border-color: var(--app-color-primary);
  color: var(--app-color-primary);
}

.li-pp-cluemenu-item.is-cancel {
  text-align: center;
  color: var(--app-text-secondary);
}

/* 剧本G2-G3 字段来源小字 */
.li-pp-field {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-pp-field-src {
  font-size: 11px;
  color: var(--app-text-secondary);
  padding-left: 2px;
}

/* 剧本G4 规则建议卡 */
.li-pp-sugg-summary {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: var(--app-text-primary);
  word-break: break-all;
}

.li-pp-sugg-maps {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-pp-sugg-map {
  font-size: 12px;
  color: var(--app-text-regular);
  display: flex;
  align-items: baseline;
  gap: 4px;
}

.li-pp-sugg-dim {
  flex-shrink: 0;
  font-weight: 600;
  color: var(--app-text-primary);
}

.li-pp-sugg-arrow {
  color: var(--app-text-secondary);
}

.li-pp-sugg-from {
  word-break: break-all;
}

.li-pp-sugg-samples {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.li-pp-sugg-sample {
  font-size: 11px;
  color: var(--app-text-secondary);
  display: flex;
  align-items: baseline;
  gap: 4px;
  min-width: 0;
}

.li-pp-sugg-file {
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 45%;
}

.li-pp-sugg-result {
  word-break: break-all;
}

/* 待裁决双路 */
.li-pp-pending {
  border-top: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  padding-top: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pp-pending-head {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.li-pp-pending-dim {
  font-size: 12px;
  font-weight: 600;
  color: var(--app-text-primary);
  flex-shrink: 0;
}

.li-pp-pending-reason {
  font-size: 12px;
  color: var(--app-text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-pp-cands {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-pp-cand {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  background: var(--app-bg-surface);
  padding: 4px 8px;
  cursor: pointer;
  text-align: left;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  min-width: 0;
}

.li-pp-cand:hover {
  border-color: var(--app-color-primary);
}

.li-pp-cand.is-active {
  border-color: var(--app-color-primary);
  background: var(--app-color-primary-light, rgba(64, 158, 255, 0.08));
}

.li-pp-cand.is-mini {
  padding: 2px 8px;
  font-size: 12px;
  justify-content: flex-start;
}

.li-pp-cand-value {
  font-size: 12px;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-pp-cand-src {
  font-size: 11px;
  color: var(--app-text-secondary);
  flex-shrink: 0;
}

/* 条件改绑面板 */
.li-pp-cond {
  border: 1px solid var(--app-border-color-light);
  border-radius: var(--app-radius);
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.li-pp-cond-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.li-pp-cond-label {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pp-cond-select {
  width: 110px;
}

/* 生成物与差异 */
.li-pp-gen-line {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-primary);
  display: flex;
  align-items: baseline;
  gap: 6px;
  flex-wrap: wrap;
  word-break: break-all;
}

.li-pp-gen-id {
  font-weight: 600;
  color: var(--app-color-primary);
}

.li-pp-gen-pattern {
  margin: 0;
  font-size: 12px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-pp-condjson {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-pp-condjson-head {
  font-size: 12px;
  color: var(--app-text-secondary);
}

.li-pp-condjson-body {
  margin: 0;
  padding: 6px 8px;
  border-radius: var(--app-radius);
  background: var(--app-bg-page, #f5f7fa);
  border: 1px solid var(--app-border-color-lighter, var(--app-border-color-light));
  font-size: 11px;
  line-height: 1.5;
  max-height: 140px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

.li-pp-difflist {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.li-pp-diff {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  font-size: 12px;
}

.li-pp-diff-kind {
  flex-shrink: 0;
  font-size: 11px;
}

.li-pp-diff.is-changed .li-pp-diff-kind {
  color: var(--app-color-warning, #e6a23c);
}

.li-pp-diff.is-added .li-pp-diff-kind {
  color: var(--app-color-success, #67c23a);
}

.li-pp-diff.is-removed .li-pp-diff-kind {
  color: var(--app-color-danger, #f56c6c);
}

.li-pp-diff-path {
  flex-shrink: 0;
  max-width: 40%;
  color: var(--app-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.li-pp-diff-cols {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 4px;
  flex-wrap: wrap;
}

.li-pp-diff-side {
  font-size: 11px;
  color: var(--app-text-secondary);
  word-break: break-all;
}

.li-pp-diff-side.is-before {
  text-decoration: line-through;
  opacity: 0.7;
}

.li-pp-diff-arrow {
  color: var(--app-text-secondary);
}
</style>
