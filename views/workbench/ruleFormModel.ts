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

// 提取动作 JSON 的前端镜像（when 为原样透传的条件表达式树）。
export interface ExtractionActionJSON {
  dimension?: string
  from?: string
  captureName?: string
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

// 新建规则的最小可改样例（pattern.captures 与 extractions.captureName 同名呼应）。
export const NEW_PATTERN_SAMPLE: PatternNode = {
  kind: 'regex',
  target: 'filename',
  expr: '^【(?P<author>[^】]+)】',
  captures: ['author']
}

export const NEW_EXTRACTIONS_SAMPLE: ExtractionActionJSON[] = [
  { dimension: 'author', from: 'capture', captureName: 'author' }
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
