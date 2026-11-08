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

// —— v4 条件行（收敛档 R1-R11：目标三分 + 格式模板 + 附加终结谓词 + 值校验） ——

// 格式段：template（整条按模板）/ contains（含这个词，仅目录）。
export interface CondxFormatForm {
  mode: 'template' | 'contains'
  tpl?: string
  word?: string
}

// 值校验（R6/R7 封闭三选一，JSON 字段名不动）：
// library 在库名单 / equal 与另一行占位词相等 / constant =常量。
export interface CondxBadgeForm {
  placeholder: string
  // occur 为值校验作用占位词的位次（同词根多次出现按出现序 1 起；缺省=位1
  // 裸组名——存量档案零变化，与内核 CondxBadge.Occur 同口径）。
  occur?: number
  kind: 'library' | 'equal' | 'constant'
  dim?: string
  fold?: boolean
  againstRow?: number
  againstPh?: string
  // againstOccur 为 equal 对方占位词的位次（语义同 occur，作用于 againstPh）。
  againstOccur?: number
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
// 量词变体（重复段，与内核 parseCondxVariant 同口径）：标签/作者/集名 三词位
// 可带量词——{词+}=至少一组、{词*}=可零组；分隔记号写作「;分隔」紧跟量词
// （如 {标签+;_}，分隔字面=首个分号起的全部、含分号本身）。变体词为多值产出，
// 仅提取段可引用，生效条件/值校验/组合接线三面禁引。
export const CONDX_VOCAB: { word: string; group: string; digit: boolean; refable: boolean; note: string }[] = [
  { word: '作者', group: 'author', digit: false, refable: true, note: '贪婪取词（.+），相邻须有分隔文字；可带量词 +/*（{作者+} 一类重复段）' },
  { word: '集名', group: 'setname', digit: false, refable: true, note: '贪婪取词；{集名} 一位盖段组合；可带量词 +/*（{集名*} 一类重复段）' },
  { word: '标题', group: 'title', digit: false, refable: true, note: '贪婪取词' },
  { word: '标签', group: 'tag', digit: false, refable: true, note: '贪婪取词；可带量词 +/*（{标签+} 一类重复段）' },
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
  { value: 'dir', label: '目录', note: '按格式匹配任一层祖先目录名——整个子树都算(不限直接父目录)；可用「位于第 N 级」限定层级（自识别根数起）' },
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
    { value: 'parentContains', label: '父目录名含词', needs: ['word'] },
    { value: 'dirLevel', label: '位于第 N 级', needs: ['count'] }
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

// 值校验类别（R6 三选一）与 library 名单维度。
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
  { name: '作者 - 标题', template: '{作者} - {标题}', target: 'file', description: '连字符分隔（惰性取首个分隔位）' },
  { name: 'pixiv 页级', template: '{作品号}_p{序号}', target: 'file', description: '作品号数字类 + _p 页号' },
  { name: '序列成套', template: '{公共段}_{序号}', target: 'seq', description: '{公共段}相同 ≥2 件成套归入同一作品集' },
  { name: '目录层作者', template: 'Artist[{作者}]', target: 'dir', description: '剥壳惯例：字面括号 + 作者词' },
  { name: '首组内名', template: '[{标签}]{任意}', target: 'dir', description: '多组括号目录名取首组内名、尾段吞掉（单组整名同样命中）' },
  { name: '括号标签×N', template: '[{标签+}]', target: 'file', description: '方括号组连续出现一至多组、每组内名各取一标签（示例 [中国翻訳] [DL版] 两组双值；{标签*} 形态零组亦命中）。每组各取一值用本词条、只取第一组内名其余吞掉用「首组内名」' },
  { name: '连写标签×N（指定分隔）', template: '{标签+;_}', target: 'file', description: '下划线分隔连写标签各取一值（a_b_c 三值、ab 不拆）。方括号包裹的标签组请用『括号标签×N』词条' },
  { name: '任意段吞位', template: '{标题}_{任意}', target: 'file', description: '{任意} 吞掉标题之后的一切' },
  { name: '可选分隔', template: '[{作者}]{ ?}{集名}', target: 'file', description: '可选分隔字面 { ?} 表示可缺席的字面空格：一条规则通吃 [{作者}]{集名} 与 [{作者}] {集名} 两形态（两侧至少一侧须有字面锚）' },
  { name: '可选作者组', template: '[{作者?}] {集名}', target: 'file', description: '可选词位：{作者?} 的方括号组整组可缺席、在场时单值可引用；缺席形态需保留包裹组后的字面空格' },
  { name: '多处标签', template: '({标签})[{作者}]{集名}{ ?}[{标签+}]', target: 'file', description: '同占位词多次出现（位次分立）：圆括号内为位1「标签」、尾部方括号组为位2「标签·第2处」（重复段多值），各处独立产出与引用' }
]

// 模板解析为 chips（literal/word 交替）：未闭合 { 报错（与内核同口径）。
// 可选记号（阶段3 镜像）：{词?} 为可选词位 chip（opt='word'）、{字面?} 为可选
// 分隔字面 chip（opt='sep'）——text 保留书写原文（含结尾 ?）；包裹组可选沿用
// 既有「字面开括号 + 词 chip + 字面闭括号」形态，chip 上叠加可选标记。
export interface TemplateChip {
  ph: boolean
  text: string
  // 可选记号类别：'word'=可选词位 {作者?}；'sep'=可选分隔字面 { ?}（空根 {?}
  // 也归此类，拒绝面由轻校验报「问号前无可选内容」）。
  opt?: 'word' | 'sep'
  // 可选记号的词根（剥去一个结尾 ?；opt='sep' 时即分隔字面本身，可为空）。
  root?: string
}

// ? 后缀记号的词根（剥去一个结尾 ?）；非 ? 后缀返回 undefined。
export function condxOptRootOfWord(word: string): string | undefined {
  return word.endsWith('?') ? word.slice(0, -1) : undefined
}

// 可选词位词根判定：词根非空且在词表（含旧词别名 {键}）——与内核
// condxOptWordEn 同口径；空根与词表外词根返回 undefined。
export function condxOptWordRootOfWord(word: string): string | undefined {
  const root = condxOptRootOfWord(word)
  if (!root) return undefined
  return condxVocabOf(root) ? root : undefined
}

// 可选分隔字面判定（与内核 condxOptSepPlaceholder 同口径）：非量词变体、
// ? 后缀、剥壳后非空且不在词表。空根 {?} 不在此列（轻校验另报文案）。
export function condxOptSepWord(word: string): boolean {
  if (condxHasVariantMark(word)) return false
  const root = condxOptRootOfWord(word)
  return root !== undefined && root !== '' && !condxVocabOf(root)
}

// 占位词 chip 归类：量词变体优先（与内核解析顺序一致，{标签+?} 走既有变体
// 报错），其次 ? 后缀（词表命中=可选词位、否则可选分隔字面），其余普通占位词。
function condxChipOfPlaceholder(word: string): TemplateChip {
  if (condxHasVariantMark(word)) return { ph: true, text: word }
  const root = condxOptRootOfWord(word)
  if (root === undefined) return { ph: true, text: word }
  return { ph: true, text: word, opt: condxVocabOf(root) ? 'word' : 'sep', root }
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
      chips.push(condxChipOfPlaceholder(text.slice(i + 1, close)))
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

// 模板词元切分 chips（位次角标供呈现）：字面段与占位词 chip 交替，同词根多位
// 各自成 chip；可多次出现词根（标签/作者/集名）的位≥2 标注位次 occur 与寻址
// 组名 posName（呈现层据此出「第N处」角标——复用可选 chip 的角标样式先例）。
// 与内核 condxPosOccur 同口径：量词变体/可选词位/普通词统一计数，{任意} 与
// 可选分隔字面不计数。
export interface CondxTplTok extends TemplateChip {
  occur?: number
  posName?: string
  repeat?: boolean
}

export function condxTplOccurChips(tpl: string): { chips?: CondxTplTok[]; error?: string } {
  const parsed = condxTemplateChips(tpl)
  if (parsed.error) return { error: parsed.error }
  const occ = new Map<string, number>()
  const chips = (parsed.chips || []).map((c): CondxTplTok => {
    const hit = condxChipBase(c)
    if (!hit) return { ...c }
    const n = (occ.get(hit.base) || 0) + 1
    occ.set(hit.base, n)
    if (CONDX_REPEAT_GROUPS.indexOf(hit.base) < 0) return { ...c }
    return { ...c, occur: n, posName: condxPosName(hit.base, n), repeat: hit.repeat }
  })
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

// —— 重复段词（占位词内部量词变体）前端轻量镜像（与内核 parseCondxVariant 同口径） ——

// 可带量词的词位（与内核 condxRepeatable 同源：仅 标签/作者/集名 三词位）。
export const CONDX_REPEAT_ROOTS = ['标签', '作者', '集名']

// 可多次出现（位次分立）的词根英文组名（与内核 condxRepeatableEn 同源：
// 标签/作者/集名 → tag/author/setname）——同词根多位各自成条目、位次独立；
// 其余词根第二次出现维持既有「重复」拒绝（轻校验镜像）。
export const CONDX_REPEAT_GROUPS = ['tag', 'author', 'setname']

// 组界六对同型括号的开→闭映射（包裹单元判定用；与内核 condxOpenBrackets 同源）。
const CONDX_OPEN_BRACKETS: Record<string, string> = {
  '[': ']', '【': '】', '（': '）', '(': ')', '「': '」', '『': '』'
}

// 重复段量词变体：root=词根、group=词根英文组名、atLeastOne=量词档（+=true、
// *=false）、sepSpecified/sep=分隔字面（自首个分号起、含分号本身）。
export interface CondxRepeatVariant {
  word: string
  root: string
  group: string
  atLeastOne: boolean
  sepSpecified: boolean
  sep: string
}

// 占位词内部是否含量词变体标记（+、* 或 ;）：与内核 hasCondxVariantMark 同口径。
export function condxHasVariantMark(word: string): boolean {
  return /[+*;]/.test(word)
}

// 解析量词变体：返回 {variant} 或 {error}（中文文案与后端同口径）；非变体词
// 返回空对象（走既有单词路径，既有行为零扰动）。
export function condxVariantOfWord(word: string): { variant?: CondxRepeatVariant; error?: string } {
  if (!condxHasVariantMark(word)) return {}
  const q = word.search(/[+*]/)
  const semi = word.indexOf(';')
  if (q < 0 || (semi >= 0 && semi < q)) {
    return { error: `模板占位词 {${word}} 语法错误：分隔字面须跟在量词（+ 或 *）之后` }
  }
  const root = word.slice(0, q)
  const rest = word.slice(q + 1)
  if (rest !== '' && rest[0] !== ';') {
    return { error: `模板占位词 {${word}} 语法错误：量词后只能跟分号引出的分隔字面` }
  }
  const v = condxVocabOf(root)
  if (!v) {
    return { error: `模板占位词 {${word}} 的词根 {${root}} 不在词表（作者/集名/序号/标题/标签/作品号/日期/公共段/任意）` }
  }
  if (CONDX_REPEAT_ROOTS.indexOf(root) < 0) {
    return { error: `模板占位词 {${word}} 的词根 {${root}} 不可重复：仅 标签/作者/集名 可带量词（+ 或 *）` }
  }
  return { variant: { word, root, group: v.group, atLeastOne: word[q] === '+', sepSpecified: rest !== '', sep: rest } }
}

// 后邻占位词是否需暂扣前字面段收尾开括号（包裹组可选的 chip 形态来源）：
// 量词变体与可选词位两类；可选分隔字面不包裹任何词（与内核 condxHoldsTrailingOpen
// 同口径）。
function condxHoldsTrailingOpen(word: string): boolean {
  return condxHasVariantMark(word) || !!condxOptWordRootOfWord(word)
}

// 字面段 chips[j] 的必选（非可缺）内容是否非空——可选分隔的右邻锚判定（与
// 内核 condxLitMandatoryAnchor 同口径）：普通字面段恒非空；收尾开括号将被后邻
// 0+ 包裹重复段或包裹可选词位收进可缺形态时，必选残留=去开括号（空则不作锚）。
function condxLitMandatoryAnchor(chips: TemplateChip[], j: number): boolean {
  const lit = chips[j].text
  if (j + 1 >= chips.length || !lit) return true
  const openCh = lit.slice(-1)
  const closeCh = CONDX_OPEN_BRACKETS[openCh]
  if (!closeCh) return true
  const next = chips[j + 1]
  const nextNext = j + 2 < chips.length && !chips[j + 2].ph ? chips[j + 2].text : ''
  const wrapped = nextNext.startsWith(closeCh)
  if (next.ph && condxHasVariantMark(next.text)) {
    const pv = condxVariantOfWord(next.text)
    if (pv.error || !pv.variant || pv.variant.atLeastOne || !wrapped) {
      // 1+ 档与裸词档：暂扣开括号原样回放，仍必选；解析错误由变体分支报错。
      return true
    }
    return Array.from(lit).length > 1 // 0+ 包裹档收进可缺形态：必选残留=去开括号。
  }
  if (next.ph && condxOptWordRootOfWord(next.text) && wrapped) {
    return Array.from(lit).length > 1 // 包裹可选词位收进可缺形态：必选残留=去开括号。
  }
  return true // 非包裹形态：开括号按字面回放，仍必选。
}

// 可选分隔字面 chips[idx] 的位置约束（决策4）：左邻与右邻至少一侧须为非空
// 必选字面段或模板首/尾，占位词不作锚；相邻两可选分隔互不作锚（{ ?}{_?}
// 天然被拒）。左邻取前一字面段的必选残留（0+ 包裹重复段/包裹可选词位把起首
// 闭括号收进可缺形态后残留为空，风险5 已知边界）；右邻取后一字面段的必选残留
// 前瞻。与内核 condxOptSepAnchored 同口径。
function condxOptSepAnchored(chips: TemplateChip[], idx: number, lastLitMandatory: string): boolean {
  if (idx > 0 && chips[idx - 1].ph && condxOptSepWord(chips[idx - 1].text)) return false
  if (idx + 1 < chips.length && chips[idx + 1].ph && condxOptSepWord(chips[idx + 1].text)) return false
  const leftOK = idx === 0 || (!chips[idx - 1].ph && chips[idx - 1].text !== '' && lastLitMandatory !== '')
  const rightOK =
    idx === chips.length - 1 ||
    (!chips[idx + 1].ph && chips[idx + 1].text !== '' && condxLitMandatoryAnchor(chips, idx + 1))
  return leftOK || rightOK
}

// —— 同占位词多次出现（位次分立）前端镜像：位次化组名、位次表条目 ——

// 位次化组名（与内核 condxPosName 同口径）：位1=裸组名（存量单出现模板零变化，
// 决策6），位 N≥2=「组名__N」（双下划线+位次；九组名全无下划线、后缀无碰撞）。
export function condxPosName(group: string, occur: number): string {
  return occur > 1 ? `${group}__${occur}` : group
}

// 位次表条目：模板内一个占位词出现（词元切分序）。group 为位次化组名=寻址键
// （captureName/限定名按此寻址）；sig 为记号签名（词根+记号形态归一——旧词
// {键} 与 {公共段} 同签名），供位次漂移 diff 判「指向变化」。
export interface CondxOccEntry {
  // word 为显示词：位1=词根现行词或变体书写原文；位≥2 追加「·第N处」。
  word: string
  // baseWord 为词根现行词（标签——旧词 {键} 归一为 公共段）。
  baseWord: string
  // base 为词根英文组名（位次化前的裸组名，如 tag）。
  base: string
  // group 为位次化组名（tag / tag__2）——寻址与下拉取值口径。
  group: string
  // occur 为位次（同词根从左到右出现序 1 起，量词变体/可选词位/普通词统一计数）。
  occur: number
  // repeat 为该位是否重复段（多值产出，三面禁引；提取段可引用）。
  repeat: boolean
  // sig 为记号签名（普通词=词根、可选词位=词根?、重复段=词根+量词+分隔）。
  sig: string
}

// 占位词 chip 的词根归类（供出现计数）：可选词位/量词变体/普通词 → 词根组名、
// 现行词、是否重复段与显示词；{任意}（非引用）、可选分隔字面、词表外/变体
// 解析错词返回 null（不计数——与内核 任意 提前返回、词表外报错同口径）。
function condxChipBase(c: TemplateChip): { base: string; baseWord: string; repeat: boolean; display: string; sig: string } | null {
  if (!c.ph || c.opt === 'sep') return null
  if (c.opt === 'word') {
    const v = condxVocabOf(c.root || '')
    if (!v || !v.refable) return null
    return { base: v.group, baseWord: v.word, repeat: false, display: v.word, sig: `${v.word}?` }
  }
  const pv = condxVariantOfWord(c.text)
  if (pv.variant) {
    const v = pv.variant
    return {
      base: v.group,
      baseWord: v.root,
      repeat: true,
      display: c.text,
      sig: `${v.root}${v.atLeastOne ? '+' : '*'}${v.sepSpecified ? v.sep : ''}`
    }
  }
  const v = condxVocabOf(c.text)
  if (!v || !v.refable) return null
  return { base: v.group, baseWord: v.word, repeat: false, display: v.word, sig: v.word }
}

// 一行的全部占位词出现（含重复段位——多值产出，仅提取段可引用；含各位次）：
// 同词根（仅 标签/作者/集名 可多次出现）多位各自成条目、位次=出现序；其余词根
// 第二次出现由轻校验报错（条目仍按位产出，供行内标红下的呈现完整性）。可选词位
// {作者?} 按现行词登记——引用权由书写记号静态决定；可选分隔字面与 {任意}
//（非引用吞位词）不产出条目、不计数（与内核 condxPosOccur 同口径）。
export function condxRowAllPlaceholders(condx: CondxForm): CondxOccEntry[] {
  if (!condx || condx.format.mode !== 'template') return []
  const parsed = condxTemplateChips(condx.format.tpl || '')
  if (!parsed.chips) return []
  const out: CondxOccEntry[] = []
  const occ = new Map<string, number>()
  for (const c of parsed.chips) {
    const hit = condxChipBase(c)
    if (!hit) continue
    const n = (occ.get(hit.base) || 0) + 1
    occ.set(hit.base, n)
    out.push({
      word: n > 1 ? `${hit.display}·第${n}处` : hit.display,
      baseWord: hit.baseWord,
      base: hit.base,
      group: condxPosName(hit.base, n),
      occur: n,
      repeat: hit.repeat,
      sig: hit.sig
    })
  }
  return out
}

// 一行的可引用占位词条目（剔除重复段位）：生效条件/值校验/组合接线三面禁引的
// 按位镜像——位为重复段（多值产出）禁引、位为单值词（含可选词位）可引用。
// 顺序=模板内出现序；旧词 {键} 书写的模板归一为 {公共段}（组名同 key，寻址
// 不受影响，显示不出旧词）。
export function condxRowPlaceholders(condx: CondxForm): CondxOccEntry[] {
  return condxRowAllPlaceholders(condx).filter((p) => !p.repeat)
}

// —— 位次漂移 diff（风险1 缓解）：编辑会话内模板变更前后位次表对照 ——

// 位次漂移判定：按词根分组做尾对齐贪心匹配（每个先前出现对齐其后最晚的同
// 签名出现），报出「位次号变化/指向变化」的组名——主形态=在已有出现之前插入
// 同词根新出现（含可选词位/重复段变体），后续位次整体 +1（旧 tag__2 仍存在
// 但指向新插入处、存在性检查不报警）；尾部追加新出现对齐不变、不报；先前位次
// 消失由既有「已不在上半声明」校验兜底，不算漂移。返回受影响组名与显示词
//（配置行按组名反查提示需复核——提取/生效条件/值校验/组合接线）。
export function condxOccurDrift(before: CondxOccEntry[], after: CondxOccEntry[]): { group: string; word: string }[] {
  const out: { group: string; word: string }[] = []
  const seen = new Set<string>()
  const roots = new Set<string>()
  for (const e of before) roots.add(e.base)
  for (const e of after) roots.add(e.base)
  for (const base of roots) {
    const b = before.filter((e) => e.base === base)
    const a = after.filter((e) => e.base === base)
    // 尾对齐：先前出现从后往前，各找其后最晚的同签名出现（新出现=after 中
    // 未对齐项；先前位次在 after 中无同签名出现=已删除/改写形态）。
    const pairs: Array<[number, number]> = []
    let j = a.length - 1
    for (let i = b.length - 1; i >= 0; i--) {
      let jj = j
      while (jj >= 0 && a[jj].sig !== b[i].sig) jj--
      if (jj >= 0) {
        pairs.push([i, jj])
        j = jj - 1
      }
    }
    const hit: Array<{ group: string; word: string }> = []
    for (const [bi, ai] of pairs) {
      if (bi !== ai) hit.push({ group: b[bi].group, word: b[bi].word })
    }
    // 未对齐的新出现落在先前位次号域内=改写形态（原记号被换成别的记号），
    // 该位次指向已变。
    const matched = new Set(pairs.map((p) => p[1]))
    for (let ai = 0; ai < a.length && ai < b.length; ai++) {
      if (!matched.has(ai)) hit.push({ group: a[ai].group, word: a[ai].word })
    }
    for (const h of hit) {
      if (seen.has(h.group)) continue
      seen.add(h.group)
      out.push(h)
    }
  }
  return out
}

// 模板轻校验（与内核 deriveCondxTemplate 主要拒绝面同口径，构造期校验兜底
// 真源）：词表外词（旧词别名合法）/ 引用词重复（按英文组名判重——{键} 与
// {公共段} 同组，同模板并用即重复；标签/作者/集名 三词位多次出现放行、位次
// 分立）/ 相邻占位词无分隔 / 重复段词（量词变体）
// 语法与形态（越权词位带量词、裸词无分隔、序列成员不支持重复段）/ 可选记号
// 拒绝面（{?} 空根、{任意?} 与 {公共段?}/{键?} 带问号、序列成员可选词位、
// 可选分隔两侧无字面锚）。target 传入时按主体镜像序列成员禁区。返回中文错误
//（空串=通过）。
export function condxTemplateError(tpl: string, target?: 'file' | 'dir' | 'seq'): string {
  const parsed = condxTemplateChips(tpl)
  if (parsed.error) return parsed.error
  const chips = parsed.chips || []
  const seen = new Set<string>()
  let prevPlaceholder = false
  let skipCloserHead = false
  let lastLitMandatory = ''
  for (let ci = 0; ci < chips.length; ci++) {
    const c = chips[ci]
    if (!c.ph) {
      // 字面段：复刻内核编译期的「闭括号去重」状态（0+ 包裹重复段/包裹可选
      // 词位把后字面段起首配对闭括号收进可缺形态）——仅供可选分隔的位置约束
      // 判定左邻必选字面残留。
      let lit = c.text
      if (skipCloserHead) {
        lit = lit.slice(1)
        skipCloserHead = false
      }
      lastLitMandatory = lit
      prevPlaceholder = false
      continue
    }
    // 可选分隔字面自身即分隔：豁免相邻占位词检查（改受位置约束），也不作
    // 后邻锚；其余占位词（含可选词位）照常受检。
    const isOptSep = condxOptSepWord(c.text)
    if (!isOptSep) {
      if (prevPlaceholder) return '相邻占位词之间缺少分隔文字'
      prevPlaceholder = true
    } else {
      prevPlaceholder = false
    }
    const pv = condxVariantOfWord(c.text)
    if (pv.error) return pv.error
    if (pv.variant) {
      const v = pv.variant
      if (target === 'seq') return `模板占位词 {${c.text}} 不可重复：序列成员格式不支持重复段`
      // 位次分立：重复段词根恒为三词位（condxVariantOfWord 已拒绝越权词位），
      // 同词根多次出现（含与普通词/可选词位混排）放行、位次独立——查重不再
      // 按词根拒绝。
      // 包裹判定：前字面段收尾开括号且后字面段起首为其配对闭括号（六对同型
      // 括号映射）→ 包裹单元；否则裸词（裸词必须指定分隔字面）。
      const prev = chips[ci - 1]
      const next = chips[ci + 1]
      const open = prev && !prev.ph ? prev.text.slice(-1) : ''
      const close = next && !next.ph ? next.text.slice(0, 1) : ''
      const wrapped = !!open && !!close && CONDX_OPEN_BRACKETS[open] === close
      if (!wrapped) {
        if (!v.sepSpecified) {
          return `模板占位词 {${c.text}} 缺分隔字面：无包裹括号的重复词没有单元边界（贪婪并合），请用分号指定分隔（如 {${c.text};_}）或改用包裹括号成段（如 [{${c.text}}]）`
        }
        if (v.sep === ';') return `模板占位词 {${c.text}} 语法错误：分号引出的分隔字面不能为空`
      } else if (!v.atLeastOne) {
        // 0+ 包裹档把配对闭括号收进可缺形态：后一字面段起首闭括号去重。
        skipCloserHead = true
      }
      continue
    }
    // 量词变体判定在前（{标签+?} 走既有「量词后只能跟分号引出的分隔字面」报错），
    // 其后才是 ? 后缀两形态（词表命中=可选词位、未命中且非空=可选分隔字面）。
    const optRoot = condxHasVariantMark(c.text) ? undefined : condxOptRootOfWord(c.text)
    if (optRoot !== undefined) {
      if (optRoot === '') return `模板占位词 {${c.text}}：问号前无可选内容`
      const v = condxVocabOf(optRoot)
      if (v) {
        // 可选词位 {词?}：走普通命名组通路（引用权由书写记号静态决定，与
        // {词*} 多值通路正交）；缺席=无绑定（绑定层由内核处理）。
        if (optRoot === '任意') {
          return `模板占位词 {${c.text}}：吞位词 {任意} 不产出声明、无可选语义，不可带问号`
        }
        if (optRoot === '键' || optRoot === '公共段') {
          return `模板占位词 {${c.text}}：{公共段} 是序列成套的成群依据，不可带问号`
        }
        if (target === 'seq') {
          return `模板占位词 {${c.text}}：序列成员格式不支持可选词位（可选分隔字面不受限）`
        }
        // 位次分立：三词位（标签/作者/集名）多次出现放行、位次独立；其余词根
        // 第二次出现维持既有「重复」拒绝。
        if (CONDX_REPEAT_GROUPS.indexOf(v.group) < 0) {
          if (seen.has(v.group)) return `模板占位词 {${c.text}} 重复`
          seen.add(v.group)
        }
        // 包裹组可选：可选作用域=整组（开括号+词+配对闭括号同进退），后一
        // 字面段起首配对闭括号随组收进可缺形态（去重，同 0+ 包裹档机制）。
        const prev = chips[ci - 1]
        const next = chips[ci + 1]
        const open = prev && !prev.ph ? prev.text.slice(-1) : ''
        const close = next && !next.ph ? next.text.slice(0, 1) : ''
        if (open && close && CONDX_OPEN_BRACKETS[open] === close) skipCloserHead = true
        continue
      }
      // 可选分隔字面（{ ?}/{_?}/{ - ?}/{what??}）：位置约束=决策4（左邻与
      // 右邻至少一侧须为非空必选字面段或模板首/尾，占位词不作锚；相邻两
      // 可选分隔互不作锚）。空根 {?} 已在上方报错。
      if (!condxOptSepAnchored(chips, ci, lastLitMandatory)) {
        return '可选分隔两侧均无字面锚：两取值词之间的可选分隔在无分隔形态下切分位置任意，请将一侧落在字面（如括号或固定前后缀）后使用'
      }
      continue
    }
    const v = condxVocabOf(c.text)
    if (!v) return `占位词「${c.text}」不在词表内（作者/集名/序号/标题/标签/作品号/日期/公共段/任意）`
    if (!v.refable) continue
    // 位次分立：三词位（标签/作者/集名）多次出现放行、位次独立；其余词根重复
    // 维持既有报错文案（{标签2} 词表外报错由上一行维持）。
    if (CONDX_REPEAT_GROUPS.indexOf(v.group) < 0) {
      if (seen.has(v.group)) {
        return `占位词「${c.text}」在同一模板内重复（引用词不可重复；同一词根重复取值请用重复段记号 {${v.word}+}，吞位用 {任意}）`
      }
      seen.add(v.group)
    }
  }
  return ''
}

// 单行校验（含附加/值校验字段族与跨行引用）：row 为该行展平行号，flat 为全表
// 展平（equal 对方行须早于本行且对方行模板含该词位）。
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
    const err = condxTemplateError(condx.format.tpl || '', target)
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
  // 「位于第 1 级」与父目录谓词互斥（N≥2 合法共存）：第 1 级目录的上面是
  // 识别根（库根）本身，没有可匹配的父目录名——行内提示与保存闸门同函数拦。
  if (target === 'dir') {
    const level1 = (condx.attach || []).some((a) => a.kind === 'dirLevel' && a.minCount === 1)
    const hasParent = (condx.attach || []).some((a) => a.kind === 'parentFormat' || a.kind === 'parentContains')
    if (level1 && hasParent) {
      return '「位于第 1 级」与父目录条件不能同用：第 1 级目录的上面是库根本身，没有可匹配的父目录名'
    }
  }
  for (const b of condx.badges || []) {
    if (!badgePlaceholderInRow(placeholders, b.placeholder, b.occur)) {
      return `值校验占位词「{${b.placeholder || '?'}${b.occur && b.occur > 1 ? `·第${b.occur}处` : ''}}」不在本行模板中`
    }
    if (b.kind === 'library') {
      if (CONDX_BADGE_DIMS.indexOf(b.dim || '') < 0) return '在库名单值校验须选择维度（作者/标签/作品集）'
    } else if (b.kind === 'equal') {
      const against = flat.find((f) => f.row === b.againstRow)
      if (!against || against.row >= rowNum) return '相等值校验须引用更早的行'
      if (!badgePlaceholderInRow(condxRowPlaceholders(against.condx), b.againstPh, b.againstOccur)) {
        return `相等值校验的对方占位词「{${b.againstPh || '?'}${b.againstOccur && b.againstOccur > 1 ? `·第${b.againstOccur}处` : ''}}」不在第 ${b.againstRow} 行模板中`
      }
    } else if (b.kind === 'constant') {
      if (!(b.value || '').trim()) return '常量值校验的值为空'
    }
  }
  return ''
}

// 值校验占位词+位次是否在本行可引用词内：按词根组名+位次配对命中（旧词 {键}
// 与 {公共段} 同组同义——JSON 手写旧词照常通过，保存时归一为新词）。
function badgePlaceholderInRow(placeholders: CondxOccEntry[], placeholder: string | undefined, occur: number | undefined): boolean {
  const own = condxVocabOf(placeholder || '')
  if (!own) return false
  return placeholders.some((p) => p.base === own.group && p.occur === (occur || 1))
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
// 呈现用 word（现行词——旧词 {键} 书写归一显示为 {公共段}；重复段词为模板
// 书写原文如 标签+）；captureName 落 JSON 用 group（内核寻址口径=英文组名）。
// repeat=true 为重复段词（多值产出）：提取段可引用，三面（生效条件/值校验/
// 组合接线）禁引——opts.includeRepeat=false 时剔除（组合接线的成员下拉口径）。
export interface CondxCaptureDecl {
  row: number
  word: string
  group: string
  repeat?: boolean
}

export function condxCaptures(rows: CondxRowForm[], opts?: { includeRepeat?: boolean }): CondxCaptureDecl[] {
  const includeRepeat = !opts || opts.includeRepeat !== false
  const out: CondxCaptureDecl[] = []
  for (const f of condxFlatRows(rows)) {
    for (const p of condxRowAllPlaceholders(f.condx)) {
      if (p.repeat && !includeRepeat) continue
      out.push({ row: f.row, word: p.word, group: p.group, repeat: p.repeat })
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
// 换词、值校验的占位词字段（placeholder/againstPh）按词换词。编辑打开旧档
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
// count=条件行数（括号子组递归计行，与占位词寻址的行号口径一致）；
// levelNote=目录行层级锚（附加条件 dirLevel，N 走 count 字段）的小字并列
// （如「第 1 级」，多行锚按行序以「 · 」连接），供各摘要面缀于「N 形态」后。
export interface CondxShapeSummary {
  label: string
  count: number
  levelNote: string
}

// 单行的层级锚小字：目录行附加条件含 dirLevel 时按 N 值生成「第 N 级」，
// 一行多条锚按附加条件序以「 · 」并列；非目录行或无锚返回空串。
export function condxDirLevelNote(condx: CondxForm): string {
  if (!condx || condx.target !== 'dir') return ''
  const notes: string[] = []
  for (const a of condx.attach || []) {
    if (a.kind === 'dirLevel' && a.minCount && a.minCount > 0) notes.push(`第 ${a.minCount} 级`)
  }
  return notes.join(' · ')
}

export function condxShapeSummary(rows: unknown): CondxShapeSummary {
  const list = Array.isArray(rows) ? (rows as CondxRowForm[]) : []
  const flat = condxFlatRows(list)
  const targets: string[] = []
  const notes: string[] = []
  for (const f of flat) {
    if (targets.indexOf(f.condx.target) < 0) targets.push(f.condx.target)
    const note = condxDirLevelNote(f.condx)
    if (note && notes.indexOf(note) < 0) notes.push(note)
  }
  const names = targets.map((t) => {
    const hit = CONDX_TARGETS.find((c) => c.value === t)
    return hit ? hit.label : t
  })
  return { label: names.join('+'), count: flat.length, levelNote: notes.join(' · ') }
}
