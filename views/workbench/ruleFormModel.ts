// 规则编辑弹窗（RuleEditDialog）的共享模型与纯函数：v4 条件行（三步漏斗）
// 模型 + 提取段选项与「仅序列成员产出」when 原子构造/判别/并入/移除逻辑。
// 纯前端模型，无 IO。


// 提取动作 JSON 的前端镜像（when 为原样透传的条件表达式树；row 为 v4 capture
// 来源行号——「行N·{词}」寻址，1 起缺省；refRule 为 W4 refCapture 来源的成员
// 规则 ID——refRule/row/captureName 三元寻址，与 refs 顶层字段配套）。
export interface ExtractionActionJSON {
  dimension?: string
  from?: string
  captureName?: string
  refRule?: string
  row?: number
  constant?: string
  scanScope?: string
  roleName?: string
  stripPrefix?: string
  stripSuffix?: string
  when?: unknown
}


// 深拷贝（弹窗打开时从档案条目复制工作副本，编辑不回写原条目）。
export function cloneJSON<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

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
  { value: 'adjudication', label: '裁决常量' },
  // W4 组合引用（规则组合引用剧本·帧7）：值取自被引成员规则上半声明的捕获槽；
  // 选中后出现「成员规则·占位词」下拉——选项按引用行顺序分组列出（帧8）
  { value: 'refCapture', label: '被引规则捕获' }
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
  { dimension: 'author', from: 'capture', captureName: 'author' },
  { dimension: 'title', from: 'capture', captureName: 'title' }
]


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
// note 为词条说明（chips 提示）。{公共段}=序列成套成员模板的公共相同部分
// （英文组名 key，与旧词 {键} 同组同义）。
export const CONDX_VOCAB: { word: string; group: string; digit: boolean; refable: boolean; note: string }[] = [
  { word: '作者', group: 'author', digit: false, refable: true, note: '贪婪取词（.+），相邻须有分隔文字' },
  { word: '集名', group: 'setname', digit: false, refable: true, note: '贪婪取词；{集名} 一位盖段组合' },
  { word: '标题', group: 'title', digit: false, refable: true, note: '贪婪取词' },
  { word: '标签', group: 'tag', digit: false, refable: true, note: '贪婪取词' },
  { word: '序号', group: 'seq', digit: true, refable: true, note: '数字类（[0-9]+）' },
  { word: '作品号', group: 'workid', digit: true, refable: true, note: '数字类（pixiv 作品号等）' },
  { word: '日期', group: 'date', digit: true, refable: true, note: '数字类（YYYYMMDD 等）' },
  { word: '公共段', group: 'key', digit: false, refable: true, note: '序列成套成员模板的公共相同部分' },
  { word: '任意', group: 'any', digit: false, refable: false, note: '吞位词：不产出声明、可重复出现' }
]

// 旧词别名（旧词 → 英文组名）：{键} 为 {公共段} 的旧书写词——旧档模板零迁移、
// 照常解析求值；仅供模板解析/校验路径消费（condxVocabOf 先查词表再查别名），
// chips 与显示永不出旧词（显示走组名反查=现行词条）。
export const CONDX_VOCAB_ALIASES: Record<string, string> = {
  键: 'key'
}

export function condxVocabOf(word: string): { word: string; group: string; digit: boolean; refable: boolean; note: string } | undefined {
  const hit = CONDX_VOCAB.find((v) => v.word === word)
  if (hit) return hit
  const aliasGroup = CONDX_VOCAB_ALIASES[word]
  if (!aliasGroup) return undefined
  return CONDX_VOCAB.find((v) => v.group === aliasGroup)
}

// 目标三分三卡（R3）：note=范围语义说明小字（卡片小字与悬停提示同源消费——
// 卡片与 title 用同一常量）。
export const CONDX_TARGETS: { value: 'file' | 'dir' | 'seq'; label: string; note: string }[] = [
  { value: 'file', label: '文件', note: '按格式匹配文件名(不含扩展名)命中的满足格式的每个文件' },
  { value: 'dir', label: '目录', note: '按格式匹配任一层祖先目录名——整个子树都算(不限直接父目录)' },
  { value: 'seq', label: '序列', note: '文件名按模板成套—非序号部分为{公共段}；公共段相同≥2才算、落单不算、不跨目录' }
]

// 第四卡「引用规则」的说明小字（组合引用的门槛与停用语义；卡片小字与悬停
// 提示同源）。
export const CONDX_TARGET_REF_NOTE = '引用另一条规则，仅当所有被引用规则命中时本规则才命中；被引用规则停用时本规则不可用'

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
  { name: '序列成套', template: '{公共段}_{序号}', target: 'seq', description: '{公共段}相同 ≥2 件成套归入同一作品集' },
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
// 返回现行词——旧词 {键} 书写的模板归一为 {公共段}（组名同 key，寻址不受
// 影响，显示不出旧词）。
export function condxRowPlaceholders(condx: CondxForm): string[] {
  if (!condx || condx.format.mode !== 'template') return []
  const parsed = condxTemplateChips(condx.format.tpl || '')
  if (!parsed.chips) return []
  const out: string[] = []
  for (const c of parsed.chips) {
    if (!c.ph) continue
    const v = condxVocabOf(c.text)
    if (v && v.refable && out.indexOf(v.word) < 0) out.push(v.word)
  }
  return out
}

// 模板轻校验（与内核 deriveCondxRules 主要拒绝面同口径，构造期校验兜底真源）：
// 词表外词（旧词别名合法）/ 引用词重复（按英文组名判重——{键} 与 {公共段}
// 同组，同模板并用即重复）/ 相邻占位词无分隔。返回中文错误（空串=通过）。
export function condxTemplateError(tpl: string): string {
  const parsed = condxTemplateChips(tpl)
  if (parsed.error) return parsed.error
  const chips = parsed.chips || []
  const seen = new Set<string>()
  for (const c of chips) {
    if (!c.ph) continue
    const v = condxVocabOf(c.text)
    if (!v) return `占位词「${c.text}」不在词表内（作者/集名/序号/标题/标签/作品号/日期/公共段/任意）`
    if (!v.refable) continue
    if (seen.has(v.group)) return `占位词「${c.text}」在同一模板内重复（引用词不可重复；吞位用 {任意}）`
    seen.add(v.group)
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
      if (!chips.some((c) => c.ph && c.text === '序号')) return '序列目标模板须含 {序号}（非序号部分为{公共段}）'
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
    if (!badgePlaceholderInRow(placeholders, b.placeholder)) {
      return `小标占位词「{${b.placeholder || '?'}}」不在本行模板中`
    }
    if (b.kind === 'library') {
      if (CONDX_BADGE_DIMS.indexOf(b.dim || '') < 0) return '在库名单小标须选择维度（作者/标签/作品集）'
    } else if (b.kind === 'equal') {
      const against = flat.find((f) => f.row === b.againstRow)
      if (!against || against.row >= rowNum) return '相等小标须引用更早的行'
      if (!badgePlaceholderInRow(condxRowPlaceholders(against.condx), b.againstPh)) {
        return `相等小标的对方占位词「{${b.againstPh || '?'}}」不在第 ${b.againstRow} 行模板中`
      }
    } else if (b.kind === 'constant') {
      if (!(b.value || '').trim()) return '常量小标的值为空'
    }
  }
  return ''
}

// 小标占位词是否在本行可引用词内：现行词精确命中，或与某词同英文组名
//（旧词 {键} 与 {公共段} 同组同义——JSON 手写旧词照常通过，保存时归一为新词）。
function badgePlaceholderInRow(placeholders: string[], placeholder: string | undefined): boolean {
  if (placeholders.indexOf(placeholder || '') >= 0) return true
  const own = condxVocabOf(placeholder || '')
  if (!own) return false
  return placeholders.some((w) => {
    const v = condxVocabOf(w)
    return v ? v.group === own.group : false
  })
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

// 捕获声明清单（v4：行号+占位词+英文组名）：提取段「行N·{词}」选项与清单
// 呈现用 word（现行词——旧词 {键} 书写归一显示为 {公共段}）；captureName 落
// JSON 用 group（内核寻址口径=英文组名）。
export interface CondxCaptureDecl {
  row: number
  word: string
  group: string
}

export function condxCaptures(rows: CondxRowForm[]): CondxCaptureDecl[] {
  const out: CondxCaptureDecl[] = []
  for (const f of condxFlatRows(rows)) {
    for (const word of condxRowPlaceholders(f.condx)) {
      const v = condxVocabOf(word)
      out.push({ row: f.row, word, group: v ? v.group : word })
    }
  }
  return out
}

// —— 旧词归一（{键} → {公共段}：同一英文组名 key 的同义词书写，语义等价） ——

// 模板文本归一：整位占位词 {键} 替换为 {公共段}（仅花括号整位匹配，字面
// 文本不动）。
export function condxNormalizeTpl(tpl: string): string {
  return (tpl || '').replace(/\{键\}/g, '{公共段}')
}

// 条件行树归一：行/括号子组递归遍历，格式模板（含附加条件迷你模板）整位
// 换词、值级小标的占位词字段（placeholder/againstPh）按词换词。编辑打开旧档
// 的显示归一与保存载荷构造共用本函数——结构化与 JSON 高级模式同路径，保存
// 即落档新词。
export function condxNormalizeRows<T>(rows: T): T {
  const walk = (value: unknown): unknown => {
    if (Array.isArray(value)) return value.map(walk)
    if (typeof value !== 'object' || value === null) return value
    const out: Record<string, unknown> = {}
    for (const key of Object.keys(value as Record<string, unknown>)) {
      const v = (value as Record<string, unknown>)[key]
      if (key === 'tpl' && typeof v === 'string') {
        out[key] = condxNormalizeTpl(v)
      } else if ((key === 'placeholder' || key === 'againstPh') && v === '键') {
        out[key] = '公共段'
      } else {
        out[key] = walk(v)
      }
    }
    return out
  }
  return walk(rows) as T
}

// —— 组合关系直译句与类别形态推导（弹窗摘要/引用行/列表小字同源） ——

// 成员规则在直译句与引用行摘要中的数据面：label=成员描述（缺省回退成员
// ID），rows=成员当前条件行（档案现档，组合求值对该形态的判定同源）。
export interface CondxComboMember {
  label: string
  rows: unknown
}

// 成员直译形态：全部条件行单一且同目标 → 按目标成段（dir/seq/file）；括号
// 子组、行级非或多行混合目标 → 该成员回退「被「A」命中」；任一析取（或）
// 连接 → 组合关系不可直译，整句回退通用措辞。
type CondxMemberSegKind = 'dir' | 'seq' | 'file' | 'fallback' | 'complex'

export interface CondxMemberShape {
  seg: CondxMemberSegKind
  label: string
}

export function condxMemberShape(member: CondxComboMember): CondxMemberShape {
  const rows = Array.isArray(member.rows) ? (member.rows as CondxRowForm[]) : []
  let rowCount = 0
  let hasGroup = false
  let hasNegated = false
  let hasOr = false
  const targets: string[] = []
  const walk = (list: CondxRowForm[]): void => {
    for (const r of list) {
      if (r.group) {
        hasGroup = true
        walk(r.group)
        continue
      }
      if (r.negated) hasNegated = true
      if (r.connector === 'or') hasOr = true
      if (r.condition) {
        rowCount += 1
        if (targets.indexOf(r.condition.target) < 0) targets.push(r.condition.target)
      }
    }
  }
  walk(rows)
  if (hasOr) return { seg: 'complex', label: member.label }
  if (hasGroup || hasNegated || rowCount === 0 || targets.length > 1) {
    return { seg: 'fallback', label: member.label }
  }
  return { seg: targets[0] as CondxMemberSegKind, label: member.label }
}

// 组合关系直译句：每成员一段、段间以「、」连接；dir 段=「位于「A」命中的
// 目录之下的」、seq 段=「属于「B」的成员文件（公共段相同≥2、落单不算）」
// （自带名词短语）、file/回退段=「被「C」命中」；末段非 seq 时整句尾接
// 「的文件」（段尾恰为「的」时只接「文件」）。任一成员含析取连接 → 整句
// 回退「命中=「A」与「B」都命中的文件」。
export function condxComboSentence(members: CondxComboMember[]): string {
  if (!members.length) return ''
  const shapes = members.map(condxMemberShape)
  if (shapes.some((s) => s.seg === 'complex')) {
    const names = members.map((m) => `「${m.label}」`)
    if (names.length === 1) return `命中=${names[0]}命中的文件`
    return `命中=${names.join('与')}都命中的文件`
  }
  const segs = shapes.map((s) => {
    if (s.seg === 'dir') return `位于「${s.label}」命中的目录之下的`
    if (s.seg === 'seq') return `属于「${s.label}」的成员文件（公共段相同≥2、落单不算）`
    return `被「${s.label}」命中`
  })
  const body = segs.join('、')
  if (shapes[shapes.length - 1].seg === 'seq') return `命中=${body}`
  return `命中=${body}${body.endsWith('的') ? '文件' : '的文件'}`
}

// 条件行的目标类别并集与形态数：label=按首次出现序去重并列（如「目录+文件」），
// count=条件行数（括号子组递归计行，与占位词寻址的行号口径一致）。
export interface CondxShapeSummary {
  label: string
  count: number
}

export function condxShapeSummary(rows: unknown): CondxShapeSummary {
  const list = Array.isArray(rows) ? (rows as CondxRowForm[]) : []
  const flat = condxFlatRows(list)
  const targets: string[] = []
  for (const f of flat) {
    if (targets.indexOf(f.condx.target) < 0) targets.push(f.condx.target)
  }
  const names = targets.map((t) => {
    const hit = CONDX_TARGETS.find((c) => c.value === t)
    return hit ? hit.label : t
  })
  return { label: names.join('+'), count: flat.length }
}
