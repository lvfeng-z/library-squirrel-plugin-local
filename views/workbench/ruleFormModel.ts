// 两段式规则编辑弹窗的共享模型与纯函数（RuleEditDialog / RulePatternNode 共用）。
// 含自 SubpageRules 随迁的「仅序列成员产出」when 原子构造/判别/并入/移除逻辑
//（原全局开关改为提取段逐行勾选，机制不变）。纯前端模型，无 IO。

// 模式 JSON 节点（主程序 patternJSON 的前端镜像：kind 判别 + 各具体模式字段
// 并集，嵌套子模式 ancestor / patterns / inner 递归同构）。
export interface PatternNode {
  kind: string
  target?: string
  expr?: string
  captures?: string[]
  minDepth?: number
  maxDepth?: number
  ancestor?: PatternNode | null
  keyExpr?: string
  seqExpr?: string
  minGroupSize?: number
  outcome?: string
  predKind?: string
  from?: string
  patterns?: PatternNode[]
  inner?: PatternNode | null
}

// 提取动作 JSON 的前端镜像（when 为原样透传的条件表达式树；row 为 v4 capture
// 来源行号——「行N·{词}」寻址，1 起缺省）。
export interface ExtractionActionJSON {
  dimension?: string
  from?: string
  captureName?: string
  row?: number
  constant?: string
  scanScope?: string
  roleName?: string
  stripPrefix?: string
  stripSuffix?: string
  when?: unknown
}

// 捕获组声明条目：名称 + 来源正则（声明它的 regex 节点 expr）。
export interface CaptureDecl {
  name: string
  expr: string
  target: string
}

// 深拷贝（弹窗打开时从档案条目复制工作副本，编辑不回写原条目）。
export function cloneJSON<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

// 按形态类型构造默认节点（新建/切换类型/子形态追加的初值）。
export function defaultPatternNode(kind: string): PatternNode {
  switch (kind) {
    case 'location':
      return { kind: 'location', minDepth: 0, maxDepth: -1, ancestor: defaultPatternNode('regex') }
    case 'sibling':
      return { kind: 'sibling', keyExpr: '', seqExpr: '([0-9]+)$', minGroupSize: 2, outcome: 'collection' }
    case 'predicate':
      return { kind: 'predicate', predKind: 'author', from: '' }
    case 'and':
      return { kind: 'and', patterns: [defaultPatternNode('regex')] }
    case 'or':
      return { kind: 'or', patterns: [defaultPatternNode('regex')] }
    case 'not':
      return { kind: 'not', inner: defaultPatternNode('regex') }
    default:
      return { kind: 'regex', target: 'filename', expr: '', captures: [] }
  }
}

// 形态类型清单（kind 判别值与主程序规则模型一致）。库内确认为直达入口：同一
// predicate 能力按确认类别拆为三项顶层选项（复合值 predicate:<类别>），顶层
// 可见「作者/标签/作品集」字样、选中即预选类别。
export const PATTERN_KINDS: { value: string; label: string; note: string }[] = [
  { value: 'regex', label: '文件名形态', note: '正则匹配文件名/目录名/路径，可声明捕获组' },
  { value: 'location', label: '目录锚定', note: '在目录深度区间内逐层匹配目录名' },
  { value: 'sibling', label: '序列群组', note: '同目录同键兄弟达数成群' },
  { value: 'predicate:author', label: '库内确认·作者', note: '按名字查询库内作者集合，查不到则整个规则不命中' },
  { value: 'predicate:tag', label: '库内确认·标签', note: '按名字查询库内标签集合，查不到则整个规则不命中' },
  { value: 'predicate:workSet', label: '库内确认·作品集', note: '按名字查询库内作品集集合，查不到则整个规则不命中' },
  { value: 'and', label: '组合·全部命中', note: '子形态全部命中才命中' },
  { value: 'or', label: '组合·任一命中', note: '子形态任一命中即命中' },
  { value: 'not', label: '取反', note: '内层形态不命中才命中' }
]

// 形态类型下拉的当前值（节点 → 复合值）：predicate 节点按 predKind 映射到
// predicate:<类别> 顶层直达项（缺类别回落作者），其余 kind 原样。
export function patternKindValue(node: { kind?: string; predKind?: string }): string {
  if (node.kind === 'predicate') return 'predicate:' + (node.predKind || 'author')
  return node.kind || 'regex'
}

// 复合值 → 该类型的默认节点（切换形态类型用）：库内确认三项映射 kind=predicate
// + 预选确认类别（取值来源置空=文件名去扩展名），其余走 defaultPatternNode。
export function patternNodeOfKindValue(value: string): PatternNode {
  if (value.startsWith('predicate:')) {
    return { kind: 'predicate', predKind: value.slice('predicate:'.length), from: '' }
  }
  return defaultPatternNode(value)
}

// 正则匹配目标（regex.target）。
export const REGEX_TARGETS: { value: string; label: string }[] = [
  { value: 'filename', label: '文件名（含扩展名）' },
  { value: 'dirname', label: '目录名（逐层）' },
  { value: 'relpath', label: '相对路径（/ 分隔）' }
]

// 谓词类别（predicate.predKind）。
export const PREDICATE_KINDS: { value: string; label: string }[] = [
  { value: 'author', label: '作者' },
  { value: 'tag', label: '标签' },
  { value: 'workSet', label: '作品集' }
]

// 序列群组命中后的归属类别（sibling.outcome）。
export const GROUP_OUTCOMES: { value: string; label: string }[] = [
  { value: 'collection', label: '归入同一作品集' },
  { value: 'independent', label: '落单条目作为独立作品' },
  { value: 'multiStoreWork', label: '单作品多 store（预留）' }
]

// 含义维度（提取动作 dimension）。
export const DIMENSION_OPTIONS: { value: string; label: string }[] = [
  { value: 'author', label: '作者' },
  { value: 'title', label: '作品名' },
  { value: 'tag', label: '标签' },
  { value: 'collection', label: '作品集' }
]

// 提取动作取值来源（from）——剧本F 提取段的来源下拉：捕获组／目录名／群组前缀
// ／常量四类为主，谓词/裁决为既有模型的补全项（JSON 高级模式可手写的一切，
// 结构化行不丢字段）。贪婪识别（帧-05）：扫描候选分段×库内集合精确比对，
// 查不到不提取（确认非线索）。
export const FROM_OPTIONS: { value: string; label: string }[] = [
  { value: 'capture', label: '捕获组' },
  { value: 'scan', label: '贪婪识别（库内确认）' },
  { value: 'parentDir', label: '目录名（父目录）' },
  { value: 'ancestorDir', label: '目录名（锚定命中层）' },
  { value: 'groupPrefix', label: '群组前缀' },
  { value: 'constant', label: '常量' },
  { value: 'predicate', label: '谓词命中' },
  { value: 'adjudication', label: '裁决常量' }
]

// 贪婪识别（帧-05 收敛）：可扫描维度（LibraryIndex 有集合者）与扫描范围。
export const SCAN_DIMENSIONS = ['author', 'tag', 'collection']
export const SCAN_SCOPE_OPTIONS: { value: string; label: string }[] = [
  { value: 'path', label: '文件名+各级目录名' },
  { value: 'uncovered', label: '仅上半未覆盖片段' }
]

// 合成策略：角色绑定的来源类别（sourceKinds）与维度策略。
export const SOURCE_KIND_OPTIONS: { value: string; label: string }[] = [
  { value: 'dir', label: '目录策展' },
  { value: 'filename', label: '文件名解析' },
  { value: 'constant', label: '常量声明' },
  { value: 'group', label: '群组分析' },
  { value: 'predicate', label: '谓词命中' },
  { value: 'assignment', label: '会话赋值' },
  { value: 'adjudication', label: '裁决声明' }
]

export const STRATEGY_OPTIONS: { value: string; label: string }[] = [
  { value: 'union', label: '并集' },
  { value: 'highTrust', label: '高可信择一' }
]

// 新建规则的最小可改样例（v4：缺省条件行【{作者}】{标题} + 作者/标题提取行，
// 占位词寻址=行 1）。
export const NEW_EXTRACTIONS_SAMPLE: ExtractionActionJSON[] = [
  { dimension: 'author', from: 'capture', captureName: '作者' },
  { dimension: 'title', from: 'capture', captureName: '标题' }
]

// 收集会绑入解析上下文的捕获组声明（镜像后端 declaredPatternCaptures：仅
// target=filename / relpath 的 regex 节点声明参与绑定——dirname 目标只记目录层
// 命中不绑定；location.ancestor 子树同样按 target 判定）。提取段下拉与捕获组
// 声明清单的数据源，上半改形态后即时跟随刷新。
export function collectPatternCaptures(node: unknown, out: CaptureDecl[] = []): CaptureDecl[] {
  if (node == null || typeof node !== 'object') return out
  const n = node as PatternNode
  if (n.kind === 'regex' && (n.target === 'filename' || n.target === 'relpath')) {
    for (const name of n.captures || []) {
      out.push({ name, expr: n.expr || '', target: n.target || '' })
    }
  }
  if (n.kind === 'location') collectPatternCaptures(n.ancestor, out)
  if (n.kind === 'and' || n.kind === 'or') {
    for (const sub of n.patterns || []) collectPatternCaptures(sub, out)
  }
  if (n.kind === 'not') collectPatternCaptures(n.inner, out)
  return out
}

// 收集形态树内全部库内确认（predicate）节点的取值来源引用（空=文件名去扩展名，
// 不计）：保存闸门与行级红的数据源——引用不在上半捕获组声明集即非法。
export function collectPredicateFroms(node: unknown, out: string[] = []): string[] {
  if (node == null || typeof node !== 'object') return out
  const n = node as PatternNode
  if (n.kind === 'predicate' && n.from) out.push(n.from)
  if (n.ancestor) collectPredicateFroms(n.ancestor, out)
  if (n.inner) collectPredicateFroms(n.inner, out)
  for (const sub of n.patterns || []) collectPredicateFroms(sub, out)
  return out
}

// 人话摘要（识别段首行，按规则类型生成）。
// 组合摘要递归拼接子形态摘要（与目录锚定递归格式段同取向）——子形态的语义
//（如库内确认的取值来源句）在首行可见，不折叠为「N 个子形态」计数。
export function patternSummaryOf(node: unknown): string {
  if (node == null || typeof node !== 'object') return '（无模式）'
  const n = node as PatternNode
  switch (n.kind) {
    case 'regex': {
      const target = n.target === 'dirname' ? '目录名' : n.target === 'relpath' ? '相对路径' : '文件名'
      const caps = n.captures || []
      const capText = caps.length ? `，声明 ${caps.length} 个捕获组（${caps.join('、')}）` : ''
      return `${target}正则匹配${n.expr ? `「${n.expr}」` : '（表达式为空）'}${capText}`
    }
    case 'location': {
      const max = n.maxDepth == null || n.maxDepth < 0 ? '不限' : String(n.maxDepth)
      return `目录锚定：第 ${n.minDepth ?? 0}..${max} 层目录名匹配（${patternSummaryOf(n.ancestor)}）`
    }
    case 'sibling':
      return `序列群组：键 ${n.keyExpr ? `「${n.keyExpr}」` : '（去扩展名全名）'} / 序号「${n.seqExpr || ''}」，同键 ≥${n.minGroupSize ?? 2} 成群`
    case 'predicate': {
      const kind = (PREDICATE_KINDS.find((k) => k.value === n.predKind) || { label: n.predKind || '?' }).label
      const fromText = n.from ? `${n.from} 捕获值` : '文件名去扩展名'
      return `库内确认·${kind}：${fromText}在库内${kind}集合，查不到则不命中`
    }
    case 'and':
      return `组合（全部命中）：${summarizeSubs(n.patterns)}`
    case 'or':
      return `组合（任一命中）：${summarizeSubs(n.patterns)}`
    case 'not':
      return `取反：内层形态不命中才命中`
    default:
      return `形态类型 ${n.kind}`
  }
}

// 组合摘要的子形态拼接（空列表如实呈现）。
function summarizeSubs(subs?: PatternNode[]): string {
  const list = subs || []
  if (!list.length) return '（无子形态）'
  return list.map((s) => patternSummaryOf(s)).join('；')
}

// 行式接词（帧-04）下的叶子形态类型清单：组合/取反由行结构承载（行前接词 +
// 行级非 + 括号子组），不出现在叶子类型下拉（JSON 高级模式仍可写一切）。
export const LEAF_PATTERN_KINDS: { value: string; label: string; note: string }[] = PATTERN_KINDS.filter(
  (k) => k.value !== 'and' && k.value !== 'or' && k.value !== 'not'
)

// —— 格式模板（帧-01/收敛档 A）：{占位词} 模板 → 正则 + 捕获组声明推导 ——

// 占位词词表：中文占位词 → 英文组名（Go RE2 拒绝中文组名）；numeric=数字段。
export const TEMPLATE_VOCAB: { words: string[]; group: string; numeric: boolean }[] = [
  { words: ['作者'], group: 'author', numeric: false },
  { words: ['集名', '作品集', '作品集合名称'], group: 'setname', numeric: false },
  { words: ['序号'], group: 'seq', numeric: true },
  { words: ['标题', '作品名'], group: 'title', numeric: false },
  { words: ['标签'], group: 'tag', numeric: false }
]

// 正则字面转义（模板的字面部分原样进正则）。
export function escapeRegexLiteral(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function escapeForCharClass(ch: string): string {
  return ch === ']' || ch === '\\' || ch === '^' ? '\\' + ch : ch
}

// 首/尾占位段的取值字符类：排除相邻字面首字符（无相邻字面取 '_'，收敛档 A）。
function charClassOf(lit: string): string {
  const ch = lit ? lit[0] : '_'
  if (ch === '_') return '[^_]+'
  return '[^' + escapeForCharClass(ch) + ']+'
}

export interface TemplateResult {
  ok: boolean
  expr?: string
  captures?: string[]
  error?: string
}

// derivePatternFromTemplate 把 {作者}_{集名}_{序号} 类模板推导为锚定正则与
// 捕获组声明：词表外中文词/非法英文组名/重复占位/相邻占位无分隔 → 错误文案。
export function derivePatternFromTemplate(tpl: string): TemplateResult {
  const text = tpl.trim()
  if (!text) return { ok: false, error: '模板为空' }
  const segs: { ph: boolean; text: string }[] = []
  let buf = ''
  for (let i = 0; i < text.length; ) {
    if (text[i] === '{') {
      const close = text.indexOf('}', i + 1)
      if (close < 0) return { ok: false, error: '占位词缺少「}」' }
      if (buf) {
        segs.push({ ph: false, text: buf })
        buf = ''
      }
      segs.push({ ph: true, text: text.slice(i + 1, close) })
      i = close + 1
    } else {
      buf += text[i]
      i++
    }
  }
  if (buf) segs.push({ ph: false, text: buf })
  if (!segs.some((s) => s.ph)) return { ok: false, error: '模板内没有占位词（如 {作者}）' }
  const groups: { group: string; numeric: boolean }[] = []
  for (const s of segs) {
    if (!s.ph) continue
    const hit = TEMPLATE_VOCAB.find((v) => v.words.indexOf(s.text) >= 0)
    if (hit) {
      groups.push({ group: hit.group, numeric: hit.numeric })
      continue
    }
    if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(s.text)) {
      groups.push({ group: s.text, numeric: s.text === 'seq' })
      continue
    }
    return {
      ok: false,
      error: `占位词「${s.text}」仅支持：作者/集名/序号/标题/标签（或直接写英文组名，如 {publisher}）`
    }
  }
  const names = groups.map((g) => g.group)
  if (new Set(names).size !== names.length) return { ok: false, error: '同一模板内占位词不能重复' }
  for (let i = 1; i < segs.length; i++) {
    if (segs[i].ph && segs[i - 1].ph) return { ok: false, error: '相邻占位词之间需要至少一个分隔字符' }
  }
  let expr = '^'
  let gi = 0
  for (let i = 0; i < segs.length; i++) {
    const s = segs[i]
    if (!s.ph) {
      expr += escapeRegexLiteral(s.text)
      continue
    }
    const g = groups[gi++]
    const prevLit = i > 0 && !segs[i - 1].ph ? segs[i - 1].text : ''
    const nextLit = i < segs.length - 1 && !segs[i + 1].ph ? segs[i + 1].text : ''
    let body: string
    if (g.numeric) body = '[0-9]+'
    else if (i === 0) body = charClassOf(nextLit)
    else if (i === segs.length - 1) body = charClassOf(prevLit)
    else body = '.+'
    expr += `(?P<${g.group}>${body})`
  }
  expr += '$'
  return { ok: true, expr, captures: names }
}

// —— SQL WHERE 式行式接词（帧-04/收敛档 B）：行列表 ↔ And/Or/Not 树互转 ——

// 行模型：conn=且/或（首行无）；negate=行级非；leaf=叶子形态；group=括号子组
//（递归行列表，open=展开态）；leaf 与 group 互斥（group 非空即子组行）。
export interface PatternRow {
  conn: '' | 'and' | 'or'
  negate: boolean
  leaf: PatternNode | null
  group: PatternRow[] | null
  open?: boolean
}

// 单子组合解包（And/Or 单子 ≡ 子自身；往返规范化）。
function unwrapSingle(node: PatternNode): PatternNode {
  let n = node
  while ((n.kind === 'and' || n.kind === 'or') && (n.patterns || []).length === 1) {
    n = (n.patterns || [])[0]
  }
  return n
}

// Not 链 → 行：叶子取反一行；组合体取反=子组行带非（Not(Not(x)) 翻转归并）。
function leafRowsOf(node: PatternNode): PatternRow[] {
  if (node.kind === 'not') {
    let negate = true
    let inner: PatternNode | null = node.inner || null
    while (inner && inner.kind === 'not') {
      negate = !negate
      inner = inner.inner
    }
    if (inner == null) return [{ conn: '', negate: false, leaf: defaultPatternNode('regex'), group: null }]
    if (inner.kind === 'and' || inner.kind === 'or') {
      return [{ conn: '', negate, leaf: null, group: treeToRows(inner), open: false }]
    }
    return [{ conn: '', negate, leaf: inner, group: null }]
  }
  return [{ conn: '', negate: false, leaf: node, group: null }]
}

// treeToRows 树 → 行列表：同类 And/Or 子树展平为连续行（统一接词）；异类组合
// 子树成括号子组行；Not 叶子成行级非。首行 conn 置空。
export function treeToRows(node: PatternNode | null | undefined): PatternRow[] {
  if (node == null || typeof node !== 'object') return []
  const n = unwrapSingle(node)
  if (n.kind === 'and' || n.kind === 'or') {
    const rows: PatternRow[] = []
    for (const child of n.patterns || []) {
      const c = unwrapSingle(child)
      if (c.kind === n.kind) {
        rows.push(...treeToRows(c))
      } else if (c.kind === 'and' || c.kind === 'or') {
        rows.push({ conn: '', negate: false, leaf: null, group: treeToRows(c), open: false })
      } else {
        rows.push(...leafRowsOf(c))
      }
    }
    rows.forEach((r, i) => {
      r.conn = i === 0 ? '' : n.kind === 'and' ? 'and' : 'or'
    })
    return rows
  }
  return leafRowsOf(n)
}

// rowsToTree 行列表 → 树：优先级与 SQL 相同（非>且>或；子组最紧）——且-连续段
// 归并为 And，或-分段为 Or 的分支；单行/单分支归一为裸节点。
export function rowsToTree(rows: PatternRow[]): PatternNode | null {
  if (!rows.length) return null
  const nodeOf = (row: PatternRow): PatternNode | null => {
    const body = row.group ? rowsToTree(row.group) : row.leaf
    if (!body) return null
    return row.negate ? { kind: 'not', inner: body } : body
  }
  if (rows.length === 1) return nodeOf(rows[0])
  const orTerms: (PatternNode | null)[][] = []
  let current: (PatternNode | null)[] = []
  rows.forEach((row, i) => {
    if (i > 0 && row.conn === 'or') {
      orTerms.push(current)
      current = []
    }
    current.push(nodeOf(row))
  })
  orTerms.push(current)
  const termNodes: PatternNode[] = []
  for (const term of orTerms) {
    const nodes = term.filter((x): x is PatternNode => x != null)
    if (!nodes.length) continue
    termNodes.push(nodes.length === 1 ? nodes[0] : { kind: 'and', patterns: nodes })
  }
  if (!termNodes.length) return null
  return termNodes.length === 1 ? termNodes[0] : { kind: 'or', patterns: termNodes }
}

// rowsComplete 行列表完整性（保存闸门）：每行有叶子或有非空且完整的子组。
export function rowsComplete(rows: PatternRow[] | null | undefined): boolean {
  if (!rows || !rows.length) return false
  return rows.every((r) =>
    r.group && r.group.length ? rowsComplete(r.group) : !!r.leaf && typeof r.leaf === 'object'
  )
}

// rowsSummaryOf 行列表人话摘要（帧-04：实时显示按优先级解析出的括号分组）：
// 且-连续段为项、或-分段为支；多行且段加括号；行级非前缀「非」；子组括号。
export function rowsSummaryOf(rows: PatternRow[] | null | undefined): string {
  if (!rows || !rows.length) return '（无条件——至少一行）'
  const termOf = (row: PatternRow): string => {
    const body = row.group && row.group.length ? `（${rowsSummaryOf(row.group)}）` : patternSummaryOf(row.leaf)
    return (row.negate ? '非' : '') + body
  }
  const orTerms: string[][] = []
  let current: string[] = []
  rows.forEach((row, i) => {
    if (i > 0 && row.conn === 'or') {
      orTerms.push(current)
      current = []
    }
    current.push(termOf(row))
  })
  orTerms.push(current)
  const terms = orTerms.map((t) => (t.length > 1 ? `（${t.join(' 且 ')}）` : t.join(' 且 ')))
  return terms.join(' 或 ')
}

// —— 「仅序列成员产出」when 原子（自 SubpageRules 随迁，机制不变；由全局开关
// 改为提取段逐行勾选）——
// 群组序号从 1 起，落单文件（不属于任何群组）取值不可用、含它的比较为假，
// 故该原子成立即「属于任意一个序列」（规则定群或结构群成员），不绑定群组来源。

// 条件表达式节点的松散形态：只按判别字段读写，动作/表达式其余字段一律透传不动
export type CondNode = Record<string, unknown>

export function isCondNode(e: unknown): e is CondNode {
  return typeof e === 'object' && e !== null && !Array.isArray(e)
}

// 「群组序号 ≥ 1」原子（数值区间式，操作数=群组序号）：与手写 JSON 可表达的形态一致
export function buildSeqAtom(): CondNode {
  return { kind: 'range', operand: { kind: 'groupSeq' }, min: 1 }
}

// 原子判别：数值区间式、操作数=群组序号、下界恰为 1、无上界（带上界的更严区间
// 是手写条件，不算本原子）
export function checkSeqAtom(e: unknown): boolean {
  if (!isCondNode(e) || e['kind'] !== 'range') return false
  const operand = e['operand']
  if (!isCondNode(operand) || operand['kind'] !== 'groupSeq') return false
  return e['min'] === 1 && (e['max'] === undefined || e['max'] === null)
}

// 原子是否为整棵表达式的必要合取项（从根到原子只经过 and 节点）：or 分支或 not
// 内层的原子不构成「仅序列成员」限定——落单文件仍可能经析取分支产出，故不参与
// 派生勾选，也不会被勾选移除
export function checkRequiredConjunct(e: unknown): boolean {
  if (!isCondNode(e)) return false
  if (checkSeqAtom(e)) return true
  if (e['kind'] === 'and') {
    const subs = e['subs']
    if (Array.isArray(subs)) return subs.some((sub) => checkRequiredConjunct(sub))
  }
  return false
}

// 原子并入单条 when：已含必要合取则幂等返回；既有 and 节点直接追加子项（避免
// 层层包裹）；其余既有条件以 and 合取；原本无条件则以原子为整棵条件
export function buildWhenWithSeqAtom(when: unknown): unknown {
  if (when == null) return buildSeqAtom()
  if (checkRequiredConjunct(when)) return when
  if (isCondNode(when) && when['kind'] === 'and') {
    const subs = when['subs']
    if (Array.isArray(subs)) return { ...when, subs: [...subs, buildSeqAtom()] }
  }
  return { kind: 'and', subs: [when, buildSeqAtom()] }
}

// 从 when 中移除必要合取位置上的全部原子：and 剔除后仅剩一项则收敛为该项、
// 剔除殆尽则该动作无条件（when 置空）；or / not 子树与叶子节点原样保留
export function buildWhenWithoutSeqAtom(e: unknown): unknown {
  if (!isCondNode(e)) return e
  if (checkSeqAtom(e)) return null
  if (e['kind'] === 'and') {
    const subs = e['subs']
    if (Array.isArray(subs)) {
      const kept = subs
        .map((sub) => buildWhenWithoutSeqAtom(sub))
        .filter((sub) => sub !== null && sub !== undefined)
      if (kept.length === 0) return null
      if (kept.length === 1) return kept[0]
      return { ...e, subs: kept }
    }
  }
  return e
}

// 该 when 除「仅序列成员」原子外是否还含其他手写条件（行内提示用）。
export function hasOtherConds(when: unknown): boolean {
  return buildWhenWithoutSeqAtom(when) != null
}

// —— v4 条件行（收敛档 R1-R11：目标三分 + 格式模板 + 附加终结谓词 + 值级小标） ——

// 格式段：template（整条按模板）/ contains（含这个词，仅目录）。
export interface CondxFormatForm {
  mode: 'template' | 'contains'
  tpl?: string
  word?: string
}

// 值级小标（R6/R7 封闭三选一）：library 在库名单 / equal 与另一行占位词相等 /
// constant =常量。
export interface CondxBadgeForm {
  placeholder: string
  kind: 'library' | 'equal' | 'constant'
  dim?: string
  fold?: boolean
  againstRow?: number
  againstPh?: string
  value?: string
}

// 节点级终结谓词（R5 封闭集，按目标分化）。
export interface CondxAttachForm {
  kind: string
  tpl?: string
  minCount?: number
  word?: string
  types?: string[]
  value?: string
}

// 一条条件（行内主体声明）。
export interface CondxForm {
  target: 'file' | 'dir' | 'seq'
  format: CondxFormatForm
  badges?: CondxBadgeForm[]
  attach?: CondxAttachForm[]
  anchor?: unknown
}

// 行式底盘的一行（R1：行前接词 + 行级非 + 条件或括号子组）。
export interface CondxRowForm {
  connector?: string
  negated?: boolean
  condition?: CondxForm | null
  group?: CondxRowForm[] | null
}

// 占位词词表（与内核 condxVocab 同源九词）：digit=数字类（[0-9]+）；
// refable=false 仅 {任意}（吞位词：非捕获、可重复、不可被提取引用）；
// note 为词条说明（chips 提示）。
export const CONDX_VOCAB: { word: string; digit: boolean; refable: boolean; note: string }[] = [
  { word: '作者', digit: false, refable: true, note: '贪婪取词（.+），相邻须有分隔文字' },
  { word: '集名', digit: false, refable: true, note: '贪婪取词；{集名} 一位盖段组合' },
  { word: '标题', digit: false, refable: true, note: '贪婪取词' },
  { word: '标签', digit: false, refable: true, note: '贪婪取词' },
  { word: '序号', digit: true, refable: true, note: '数字类（[0-9]+）' },
  { word: '作品号', digit: true, refable: true, note: '数字类（pixiv 作品号等）' },
  { word: '日期', digit: true, refable: true, note: '数字类（YYYYMMDD 等）' },
  { word: '键', digit: false, refable: true, note: '序列成员模板的公共前缀位' },
  { word: '任意', digit: false, refable: false, note: '吞位词：不产出声明、可重复出现' }
]

export function condxVocabOf(word: string): { word: string; digit: boolean; refable: boolean; note: string } | undefined {
  return CONDX_VOCAB.find((v) => v.word === word)
}

// 目标三分三卡（R3）：目录命中=整棵子树包含（任意深度）、主体回联内建。
export const CONDX_TARGETS: { value: 'file' | 'dir' | 'seq'; label: string; note: string }[] = [
  { value: 'file', label: '文件', note: '按格式命中单个文件（匹配去扩展名的文件名）' },
  { value: 'dir', label: '目录', note: '按格式命中目录（命中=该目录位于祖先链上，整棵子树包含）；命中即集主体' },
  { value: 'seq', label: '序列', note: '成员按模板成套（非{序号}部分跨成员相同为键）' }
]

// 终结谓词封闭集（与内核 attachAllowed 镜像）：needs 列出必填字段族。
export const CONDX_ATTACH_OPTIONS: Record<string, { value: string; label: string; needs: string[] }[]> = {
  file: [
    { value: 'fileExt', label: '扩展名是', needs: ['value'] },
    { value: 'fileSizeMin', label: '文件大小 ≥N KB', needs: ['count'] },
    { value: 'parentFormat', label: '父目录名按模板', needs: ['tpl'] },
    { value: 'parentContains', label: '父目录名含词', needs: ['word'] }
  ],
  dir: [
    { value: 'childrenAllFormat', label: '里面全部子女按模板', needs: ['tpl', 'count'] },
    { value: 'childrenAtLeastN', label: '里面至少 N 个子女按模板', needs: ['tpl', 'count'] },
    { value: 'childDirCount', label: '子目录数 ≥N', needs: ['count'] },
    { value: 'childTypeIs', label: '子女类型是', needs: ['types'] },
    { value: 'parentFormat', label: '父目录名按模板', needs: ['tpl'] },
    { value: 'parentContains', label: '父目录名含词', needs: ['word'] }
  ],
  seq: [
    { value: 'seqMinLength', label: '序列长度 ≥N', needs: ['count'] },
    { value: 'seqMemberType', label: '成员类型是', needs: ['types'] },
    { value: 'parentFormat', label: '父目录名按模板', needs: ['tpl'] },
    { value: 'parentContains', label: '父目录名含词', needs: ['word'] }
  ]
}

// 类型列表谓词（childTypeIs / seqMemberType）的封闭类型集。
export const CONDX_TYPE_OPTIONS = ['image', 'video', 'document', 'archive']

// 值级小标类别（R6 三选一）与 library 名单维度。
export const CONDX_BADGE_KINDS: { value: 'library' | 'equal' | 'constant'; label: string; needs: string[] }[] = [
  { value: 'library', label: '在库名单', needs: ['dim'] },
  { value: 'equal', label: '与另一行的占位词相等', needs: ['againstRow', 'againstPh'] },
  { value: 'constant', label: '= 常量', needs: ['value'] }
]
export const CONDX_BADGE_DIMS = ['author', 'tag', 'collection']

// 例子模板（与内核 exampleTemplates 同源语义的静态镜像：参考词条不参与求值，
// 点击预填当前行目标与模板）。
export const CONDX_EXAMPLES: { name: string; template: string; target: 'file' | 'dir' | 'seq'; description: string }[] = [
  { name: '【作者】标题', template: '【{作者}】{标题}', target: 'file', description: '方括号命名惯例' },
  { name: '作者 - 标题', template: '{作者} - {标题}', target: 'file', description: '连字符分隔（贪婪取最后分隔位）' },
  { name: 'pixiv 页级', template: '{作品号}_p{序号}', target: 'file', description: '作品号数字类 + _p 页号' },
  { name: '序列成套', template: '{键}_{序号}', target: 'seq', description: '同键 ≥2 件成套归入同一作品集' },
  { name: '目录层作者', template: 'Artist[{作者}]', target: 'dir', description: '剥壳惯例：字面括号 + 作者词' },
  { name: '任意段吞位', template: '{标题}_{任意}', target: 'file', description: '{任意} 吞掉标题之后的一切' }
]

// 模板解析为 chips（literal/word 交替）：未闭合 { 报错（与内核同口径）。
export interface TemplateChip {
  ph: boolean
  text: string
}

export function condxTemplateChips(tpl: string): { chips?: TemplateChip[]; error?: string } {
  const text = (tpl || '').trim()
  if (!text) return { error: '模板为空' }
  const chips: TemplateChip[] = []
  let buf = ''
  for (let i = 0; i < text.length; ) {
    if (text[i] === '{') {
      const close = text.indexOf('}', i + 1)
      if (close < 0) return { error: '占位词缺少「}」' }
      if (buf) {
        chips.push({ ph: false, text: buf })
        buf = ''
      }
      chips.push({ ph: true, text: text.slice(i + 1, close) })
      i = close + 1
    } else {
      buf += text[i]
      i++
    }
  }
  if (buf) chips.push({ ph: false, text: buf })
  if (!chips.some((c) => c.ph)) return { error: '模板内没有占位词（如 {作者}）' }
  return { chips }
}

// 先序展平（Group 递归）：行号按内核口径 1 起连续计数（equal 的 againstRow、
// 提取动作 Row 同源）。只数 condition 行。
export function condxFlatRows(rows: CondxRowForm[]): { row: number; condx: CondxForm }[] {
  const out: { row: number; condx: CondxForm }[] = []
  let n = 0
  const walk = (list: CondxRowForm[]): void => {
    for (const r of list) {
      if (r.group) {
        walk(r.group)
        continue
      }
      n += 1
      if (r.condition) out.push({ row: n, condx: r.condition })
    }
  }
  walk(rows)
  return out
}

// 一行的可引用占位词（模板中出现且 refable）：提取段「行N·{词}」下拉与
// 小标占位词下拉的数据源；顺序=模板内出现序（引用词不可重复，天然唯一）。
export function condxRowPlaceholders(condx: CondxForm): string[] {
  if (!condx || condx.format.mode !== 'template') return []
  const parsed = condxTemplateChips(condx.format.tpl || '')
  if (!parsed.chips) return []
  const out: string[] = []
  for (const c of parsed.chips) {
    if (!c.ph) continue
    const v = condxVocabOf(c.text)
    if (v && v.refable) out.push(c.text)
  }
  return out
}

// 模板轻校验（与内核 deriveCondxRules 主要拒绝面同口径，构造期校验兜底真源）：
// 词表外词 / 引用词重复 / 相邻占位词无分隔。返回中文错误（空串=通过）。
export function condxTemplateError(tpl: string): string {
  const parsed = condxTemplateChips(tpl)
  if (parsed.error) return parsed.error
  const chips = parsed.chips || []
  const seen = new Set<string>()
  for (const c of chips) {
    if (!c.ph) continue
    const v = condxVocabOf(c.text)
    if (!v) return `占位词「${c.text}」不在词表内（作者/集名/序号/标题/标签/作品号/日期/键/任意）`
    if (!v.refable) continue
    if (seen.has(c.text)) return `占位词「${c.text}」在同一模板内重复（引用词不可重复；吞位用 {任意}）`
    seen.add(c.text)
  }
  for (let i = 1; i < chips.length; i++) {
    if (chips[i].ph && chips[i - 1].ph) return '相邻占位词之间缺少分隔文字'
  }
  return ''
}

// 单行校验（含附加/小标字段族与跨行引用）：row 为该行展平行号，flat 为全表
// 展平（equal 对方行须早于本行且对方行模板含该词）。
export function condxRowError(rowNum: number, condx: CondxForm | null | undefined, flat: { row: number; condx: CondxForm }[]): string {
  if (!condx) return '须选择目标与格式'
  if (condx.anchor != null) return '主体锚（ref）为二期能力，v4 不支持'
  const target = condx.target
  if (!CONDX_TARGETS.some((t) => t.value === target)) return '须选择目标（文件/目录/序列）'
  if (condx.format.mode === 'contains') {
    if (target !== 'dir') return '「含这个词」仅目录目标可用'
    if (!(condx.format.word || '').trim()) return '包含词为空'
    if (condxTemplateChips(condx.format.word || '').chips) return '包含词须为纯文字（不含占位词）'
  } else {
    const err = condxTemplateError(condx.format.tpl || '')
    if (err) return err
    if (target === 'seq') {
      const chips = condxTemplateChips(condx.format.tpl || '').chips || []
      if (!chips.some((c) => c.ph && c.text === '序号')) return '序列目标模板须含 {序号}（非序号部分为键）'
    }
  }
  const allowed = (CONDX_ATTACH_OPTIONS[target] || []).map((o) => o.value)
  const placeholders = condxRowPlaceholders(condx)
  for (const a of condx.attach || []) {
    if (allowed.indexOf(a.kind) < 0) return `终结谓词「${a.kind}」不适用于${target === 'file' ? '文件' : target === 'dir' ? '目录' : '序列'}目标`
    const opt = (CONDX_ATTACH_OPTIONS[target] || []).find((o) => o.value === a.kind)
    for (const need of opt ? opt.needs : []) {
      if (need === 'tpl' && condxTemplateError(a.tpl || '')) return `附加条件的迷你模板不完整`
      if (need === 'word' && !(a.word || '').trim()) return `附加条件的包含词为空`
      if (need === 'count' && !(a.minCount && a.minCount > 0)) return `附加条件的数量门槛须 ≥1`
      if (need === 'types' && !(a.types || []).length) return `附加条件须选择至少一个类型`
      if (need === 'value' && !(a.value || '').trim()) return `附加条件的扩展名为空`
    }
  }
  for (const b of condx.badges || []) {
    if (placeholders.indexOf(b.placeholder) < 0) return `小标占位词「{${b.placeholder || '?'}}」不在本行模板中`
    if (b.kind === 'library') {
      if (CONDX_BADGE_DIMS.indexOf(b.dim || '') < 0) return '在库名单小标须选择维度（作者/标签/作品集）'
    } else if (b.kind === 'equal') {
      const against = flat.find((f) => f.row === b.againstRow)
      if (!against || against.row >= rowNum) return '相等小标须引用更早的行'
      if (condxRowPlaceholders(against.condx).indexOf(b.againstPh || '') < 0) {
        return `相等小标的对方占位词「{${b.againstPh || '?'}}」不在第 ${b.againstRow} 行模板中`
      }
    } else if (b.kind === 'constant') {
      if (!(b.value || '').trim()) return '常量小标的值为空'
    }
  }
  return ''
}

// 全表校验：行完整性（至少一行 condition/Group 非空）+ 逐行 condxRowError。
export function condxRowsError(rows: CondxRowForm[]): string {
  const flat = condxFlatRows(rows)
  if (!flat.length) return '至少需要一行条件'
  for (const f of flat) {
    const err = condxRowError(f.row, f.condx, flat)
    if (err) return `第 ${f.row} 行：${err}`
  }
  return ''
}

// 捕获声明清单（v4：行号+占位词）：提取段「行N·{词}」选项与清单呈现。
export interface CondxCaptureDecl {
  row: number
  word: string
}

export function condxCaptures(rows: CondxRowForm[]): CondxCaptureDecl[] {
  const out: CondxCaptureDecl[] = []
  for (const f of condxFlatRows(rows)) {
    for (const word of condxRowPlaceholders(f.condx)) out.push({ row: f.row, word })
  }
  return out
}
