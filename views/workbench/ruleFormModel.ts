// 规则编辑弹窗（RuleEditDialog）的共享模型与纯函数：v4 条件行（三步漏斗）
// 模型 + 提取段选项与「仅序列成员产出」when 原子构造/判别/并入/移除逻辑。
// 纯前端模型，无 IO（全部为纯函数与常量）。


// 提取动作 JSON 的前端镜像（when 为原样透传的条件表达式树；row 为 v4 capture
// 来源行号——「行N·{词}」寻址，1 起缺省；refRule 为 refCapture 来源的供给方
// 规则 ID——本规则目录行作用域附件（refScope）所引供给方；refRule/row/
// captureName 三元寻址）。
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

// 含义维度（提取动作 dimension）：description=作品描述（site_work_description，
// 创建期接线、多值取首个——与标题取首同语义）。
export const DIMENSION_OPTIONS: { value: string; label: string }[] = [
  { value: 'author', label: '作者' },
  { value: 'title', label: '作品名' },
  { value: 'tag', label: '标签' },
  { value: 'collection', label: '作品集' },
  { value: 'description', label: '描述' }
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
  // 作用域附件捕获（剧本2·裁决②）：值取自本规则目录行供给方（refScope 附件
  // 所引纯目录规则）上半声明的捕获槽；选中后出现「供给方·占位词」下拉——
  // 选项按供给方顺序分组列出
  { value: 'refCapture', label: '供给方捕获' }
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

// 占位词词表（与内核 condxVocab 同源十一词）：cls=捕获类三分——'lazy'=惰性段
// （类锚定外的缺省取词）、'digit'=数字类（[0-9]+，仅 {日期}——真实日期即数字，
// 维持纯数字）、'alnum'=字母数字类（[0-9A-Za-z]+，{序号}/{作品号}——编号类词
// 按真实编号形态取超集放宽，纯数字照常命中，BV 号一类字母混合编号整词可匹配）；
// refable=false 仅 {任意}（吞位词：非捕获、可重复、不可被提取引用）；
// note 为词条说明（chips 提示）。{公共段}=公共部分白段词（不接元数据维度、只圈
// 住公共部分文本；成套键=全部非序号必选捕获按序拼接；仅序列模板、单次、不可带
// 问号；英文组名 key，与旧词 {键} 同组同义）。{作品标识}=聚合标识专用中性取值
// 词根（机制零新增的普通命名捕获槽——干名如 ep01 挂标签/作者等元数据语义词别扭，
// 聚合轨行模板用它捕获标识值；英文组名 work，与旧词 {作品} 同组同义；与 {作品号}
// 同模板并用被拒——整词标识 vs 编号段语义近邻防混用）。{描述}=作品描述维度
// 取值词（提取动作接「描述」维度，创建期接线 site_work_description）。
// 量词变体（重复段，与内核 parseCondxVariant 同口径）：标签/作者/集名 三词位
// 可带量词——{词+}=至少一组、{词*}=可零组；分隔记号写作「;分隔」紧跟量词
// （如 {标签+;_}，分隔字面=首个分号起的全部、含分号本身）。变体词为多值产出，
// 仅提取段可引用，生效条件/值校验/组合接线三面禁引。
export const CONDX_VOCAB: { word: string; group: string; cls: 'lazy' | 'digit' | 'alnum'; refable: boolean; note: string }[] = [
  { word: '作者', group: 'author', cls: 'lazy', refable: true, note: '贪婪取词（.+），相邻须有分隔文字；可带量词 +/*（{作者+} 一类重复段）' },
  { word: '集名', group: 'setname', cls: 'lazy', refable: true, note: '贪婪取词；{集名} 一位盖段组合；可带量词 +/*（{集名*} 一类重复段）' },
  { word: '标题', group: 'title', cls: 'lazy', refable: true, note: '贪婪取词' },
  { word: '标签', group: 'tag', cls: 'lazy', refable: true, note: '贪婪取词；可带量词 +/*（{标签+} 一类重复段）' },
  { word: '作品标识', group: 'work', cls: 'lazy', refable: true, note: '聚合标识专用取值词根（普通命名捕获槽，旧词 {作品} 为别名书写）：聚合轨行模板用它捕获标识值，如 {作品标识}.{任意} 配扩展名谓词 .srt；不可与 {作品号} 同模板并用' },
  { word: '序号', group: 'seq', cls: 'alnum', refable: true, note: '字母数字类（[0-9A-Za-z]+）：纯数字照常命中，字母序号（1a/1b）整词命中' },
  { word: '作品号', group: 'workid', cls: 'alnum', refable: true, note: '字母数字类（[0-9A-Za-z]+）：pixiv 作品号/BV 号等编号段整词捕获' },
  { word: '日期', group: 'date', cls: 'digit', refable: true, note: '数字类（[0-9]+，YYYYMMDD 等）' },
  { word: '描述', group: 'description', cls: 'lazy', refable: true, note: '作品描述维度取值词（贪婪取词）：提取动作接「描述」维度（多值时创建期取首个）' },
  { word: '公共段', group: 'key', cls: 'lazy', refable: true, note: '公共部分白段词：不接元数据维度、只圈住公共部分文本；成套键=全部非序号必选捕获按序拼接；仅序列模板、单次、不可带问号' },
  { word: '任意', group: 'any', cls: 'lazy', refable: false, note: '吞位词：不产出声明、可重复出现' }
]

// 旧词别名（旧词 → 英文组名）：{键} 为 {公共段}、{作品} 为 {作品标识} 的旧
// 书写词——旧档模板零迁移、照常解析求值；仅供模板解析/校验路径消费
// （condxVocabOf 先查词表再查别名），chips 与显示永不出旧词（显示走组名反查=
// 现行词条；保存归一见 condxNormalizeTpl/condxNormalizeRows）。
export const CONDX_VOCAB_ALIASES: Record<string, string> = {
  键: 'key',
  作品: 'work'
}

export function condxVocabOf(word: string): { word: string; group: string; cls: 'lazy' | 'digit' | 'alnum'; refable: boolean; note: string } | undefined {
  const hit = CONDX_VOCAB.find((v) => v.word === word)
  if (hit) return hit
  const aliasGroup = CONDX_VOCAB_ALIASES[word]
  if (!aliasGroup) return undefined
  return CONDX_VOCAB.find((v) => v.group === aliasGroup)
}

// 目标三分三卡（R3）：note=范围语义说明小字（卡片小字与悬停提示同源消费——
// 卡片与 title 用同一常量）。
export const CONDX_TARGETS: { value: 'file' | 'dir' | 'seq'; label: string; note: string }[] = [
  { value: 'file', label: '文件名', note: '按格式匹配文件名(不含扩展名)命中的满足格式的每个文件' },
  { value: 'dir', label: '目录名', note: '按格式匹配任一层祖先目录名——整个子树都算(不限直接父目录)；层级表达属目录行：段间「/」=下一级目录、「//」=之下任意层、「/{2}」=隔2级（如 Artist[{作者}]/{集名} 自上而下逐级）' },
  { value: 'seq', label: '序列', note: '文件名按模板成套—成套键=全部非序号捕获按序拼接（可选词位不进键）；同键≥2才算、落单不算、不跨目录' }
]

// 终结谓词封闭集（与内核 attachAllowed 镜像）：needs 列出必填字段族。
// refScope（作用域附件·决策8 甲案）为行内特殊呈现——供给方规则选择+层距两
// 控件不走 needs 通用控件；MinCount 复用为层距（0=任意深度哨兵、1=紧邻缺省、
// N≥2 显式隔 N 级——与模板段距记号不同）；仅目录行可挂。
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
    { value: 'refScope', label: '位于此目录下', needs: ['refRule', 'gap'] }
  ],
  seq: [
    { value: 'seqMinLength', label: '序列长度 ≥N', needs: ['count'] },
    { value: 'seqMemberType', label: '成员类型是', needs: ['types'] },
    { value: 'parentFormat', label: '父目录名按模板', needs: ['tpl'] },
    { value: 'parentContains', label: '父目录名含词', needs: ['word'] }
  ]
}

// 作用域附件层距三选（剧本2 帧2）：「紧邻（下一级）／之下任意层／之下隔 N 级」
//（隔 N 级档配数字输入 N≥2）；MinCount 编码=0 任意深度、1 紧邻、N≥2 显式。
export const REFSCOPE_GAP_OPTIONS: { value: 'adjacent' | 'any' | 'n'; label: string }[] = [
  { value: 'adjacent', label: '紧邻（下一级）' },
  { value: 'any', label: '之下任意层' },
  { value: 'n', label: '之下隔 N 级' }
]

// 库根 sentinel（与内核 RefScopeLibraryRoot 同源）：来源=库的最顶层目录，
// 无供给方求值/悬空（门段恒注入命中层集 {-1}），层距即绝对层级——紧邻=根
// 直下第 1 级、隔 N=自根数起第 N 级、任意=恒真。保留串 $root 与规则 ID
// 形态不冲突（规则 ID 不以 $ 起首）；来源下拉「库根」恒在列、排首位、缺省
// 选中（帧2/帧4）。
export const REFSCOPE_LIBRARY_ROOT = '$root'

// 层距档判定：MinCount → 三选档位（缺省/1=紧邻、0=任意、≥2=隔 N 级）。
export function refScopeGapKeyOf(minCount: number | undefined): 'adjacent' | 'any' | 'n' {
  if (minCount === 0) return 'any'
  if (minCount != null && minCount >= 2) return 'n'
  return 'adjacent'
}

// 层距档取值：三选档位 → MinCount（隔 N 级档保留既有 ≥2 值、缺省 2）。
export function refScopeGapCountOf(key: 'adjacent' | 'any' | 'n', cur?: number): number {
  if (key === 'any') return 0
  if (key === 'n') return cur != null && cur >= 2 ? cur : 2
  return 1
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
  { name: 'pixiv 页级', template: '{作品号}_p{序号}', target: 'file', description: '作品号字母数字类（[0-9A-Za-z]+）+ _p 页号' },
  { name: '序列成套', template: '{公共段}_{序号}', target: 'seq', description: '{公共段} 为公共部分白段词（不接元数据维度，只圈住公共部分文本）；成套判定键=全部非序号捕获按序拼接，同键 ≥2 件即归入同一作品集。{序号} 为序号位（序号词字母数字类）' },
  { name: '序列成套·可选变化段', template: '{集名} - {序号}.{标题?}(Av{作品号},P{序号})', target: 'seq', description: '序列主体行的可选词位：{标题?} 逐件变化（如分P 标题）、不进成套键——成套键=集名+作品号（{序号} 两写时位2 也不进键）；该段缺席的成员照常入套（标题无声明）。须保留至少一个必选的非序号取值词（或 {公共段}），全部可选的序列模板会被拒绝' },
  { name: '目录层作者', template: 'Artist[{作者}]', target: 'dir', description: '剥壳惯例：字面括号 + 作者词' },
  { name: '首组内名', template: '[{标签}]{任意}', target: 'dir', description: '多组括号目录名取首组内名、尾段吞掉（单组整名同样命中）' },
  { name: '括号标签×N', template: '[{标签+}]', target: 'file', description: '方括号组连续出现一至多组、每组内名各取一标签（示例 [中国翻訳] [DL版] 两组双值；{标签*} 形态零组亦命中）。每组各取一值用本词条、只取第一组内名其余吞掉用「首组内名」' },
  { name: '连写标签×N（指定分隔）', template: '{标签+;_}', target: 'file', description: '下划线分隔连写标签各取一值（a_b_c 三值、ab 不拆）。方括号包裹的标签组请用『括号标签×N』词条' },
  { name: '任意段吞位', template: '{标题}_{任意}', target: 'file', description: '{任意} 吞掉标题之后的一切' },
  { name: '可选分隔', template: '[{作者}]{ ?}{集名}', target: 'file', description: '可选分隔字面 { ?} 表示可缺席的字面空格：一条规则通吃 [{作者}]{集名} 与 [{作者}] {集名} 两形态（两侧至少一侧须有字面锚）' },
  { name: '可选作者组', template: '[{作者?}] {集名}', target: 'file', description: '可选词位：{作者?} 的方括号组整组可缺席、在场时单值可引用；缺席形态需保留包裹组后的字面空格' },
  { name: '多处标签', template: '({标签})[{作者}]{集名}{ ?}[{标签+}]', target: 'file', description: '同占位词多次出现（位次分立）：圆括号内为位1「标签」、尾部方括号组为位2「标签·第2处」（重复段多值），各处独立产出与引用' },
  { name: '分P 双标题', template: '{标题} - {序号}.{标题}(Av{作品号},P{序号})', target: 'file', description: 'bilibili 分P 形态：投稿标题（位1 标题）与 P标题（位2 标题）各自独立取值、序号双写同理（位≥2 组名带 __2 后缀，如 title__2/seq__2，提取/值校验按位寻址）；作品号为字母数字类，Av 前缀后的编号整词捕获（BV 号形态同理，无需拆字面前缀）' },
  { name: '描述括号段', template: '{标题}({描述})', target: 'file', description: '{描述} 为作品描述维度取值词：括号内简介段整词捕获，提取动作接「描述」维度（多值时创建期取首个）' },
  { name: '聚合标识（带锚形态）', template: '{作品标识}.{任意}', target: 'file', description: '聚合规则的轨行示例：{作品标识} 为聚合标识专用取值词根（命名捕获槽可用词，旧词 {作品} 为其别名、书写合法），`.` 字面锚+{任意} 吞掉语言段——配扩展名谓词 .srt 时 ep01.zh.srt 的标识取 ep01。须写带锚形态而非裸 {作品标识}（无锚时语言段并进标识、同作不同名不可并组）；扩展名由扩展名谓词承载、不写进模板（{作品标识}.srt 一类把扩展名写进模板的形态不命中）' }
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

// —— 多段目录模板（决策1）：段切分前端镜像与直译句（决策7 前端镜像） ——

// 段距（两段之间的层距，裁决④）：any=true 为「之下任意层」（//）；否则 N≥1
// 显式层距（/{N}；「/」=1 紧邻缺省）。层距 0 不可书写。
export interface DirSegGapForm {
  any: boolean
  n: number
}

export interface DirTplSegmentsForm {
  segs: string[]
  gaps: DirSegGapForm[]
  // seps 为段间分隔符书写原文（'/'、'//'、'/{N}'——词元分组呈现的段间分隔）。
  seps: string[]
  error?: string
}

// splitDirTplSegments 把目录模板按分隔符切分为段序列与段距序列（决策1 形态，
// 镜像内核 rule_condition.go splitDirTplSegments、错误文案同源）：`/`=紧邻
//（N=1）、`//`=任意层距、`/{N}`（花括号纯数字，N≥1）=显式层距。边界：首段前/
// 末段后不得有分隔符；空段拒绝（连斜杠>2 同理）；非纯数字花括号不作层距记号
//（回到段内词法）。单段（无分隔符）返回单段、段距序列为空。
export function splitDirTplSegments(tpl: string): DirTplSegmentsForm {
  const text = tpl || ''
  const segs: string[] = []
  const gaps: DirSegGapForm[] = []
  const seps: string[] = []
  let start = 0
  let i = 0
  while (i < text.length) {
    if (text[i] !== '/') {
      i++
      continue
    }
    // 分隔符候选：吃掉连续斜杠（>2 即夹空段）。
    let j = i
    while (j < text.length && text[j] === '/') j++
    const slashes = j - i
    if (slashes > 2) {
      return { segs: [], gaps: [], seps: [], error: `目录模板在 ${slashes} 个连斜杠处存在空段：相邻分隔符之间没有目录名` }
    }
    let gap: DirSegGapForm = { any: false, n: 1 }
    let k = j
    if (slashes === 1 && j < text.length && text[j] === '{') {
      // /{N} 显式层距：花括号内纯数字才作层距记号，否则 `/` 按紧邻、花括号
      // 内容留给段内词法（词表校验照常处置）。
      let m = j + 1
      while (m < text.length && text[m] >= '0' && text[m] <= '9') m++
      if (m > j + 1 && m < text.length && text[m] === '}') {
        const digits = text.slice(j + 1, m)
        const n = Number(digits)
        if (!Number.isInteger(n) || n < 1) {
          return { segs: [], gaps: [], seps: [], error: `目录模板层距记号 /{${digits}} 无效：层距须为 ≥1 的整数（紧邻写 /、之下任意层写 //）` }
        }
        gap = { any: false, n }
        k = m + 1
      }
    }
    if (slashes === 2) gap = { any: true, n: 1 }
    if (i === 0) return { segs: [], gaps: [], seps: [], error: '目录模板不得以分隔符开头：首段之前没有目录名' }
    if (text.slice(start, i) === '') return { segs: [], gaps: [], seps: [], error: '目录模板存在空段：相邻分隔符之间没有目录名' }
    if (k >= text.length) return { segs: [], gaps: [], seps: [], error: '目录模板不得以分隔符结尾：末段之后没有目录名' }
    segs.push(text.slice(start, i))
    gaps.push(gap)
    seps.push(text.slice(i, k))
    start = k
    i = k
  }
  segs.push(text.slice(start))
  return { segs, gaps, seps }
}

// condxStripGapNotation 层距记号剥离（校验用）：/{N} 显式层距记号折回「/」
//——剥离后的模板可整体走既有词法校验（分隔符作字面、位次流跨段连续）。
export function condxStripGapNotation(tpl: string): string {
  return (tpl || '').replace(/\/\{([0-9]+)\}/g, '/')
}

// dirSegLevelPhrase 多段目录模板的人话句式（决策7 前端镜像，与后端
// parse_actions.go dirSegLevelPhrase 同源）：全段距紧邻时念「目录层级 `tpl`
//（自上而下逐级紧邻）」；含显式段距（// 任意、/{N} 跳层）时改为段间夹注形态
//——夹注词「（任意层）」「（隔N级）」落在段与分隔符之间，句尾缀「（自上而下）」。
// 切分失配（非多段原文）回落「目录层级 `tpl`（自上而下）」。
export function dirSegLevelPhrase(tpl: string, gaps: DirSegGapForm[]): string {
  const allAdjacent = gaps.every((g) => !g.any && g.n === 1)
  if (allAdjacent) return `目录层级 ${tpl}（自上而下逐级紧邻）`
  const split = splitDirTplSegments(tpl)
  if (split.error || split.gaps.length !== gaps.length || split.segs.length !== gaps.length + 1) {
    return `目录层级 ${tpl}（自上而下）`
  }
  let out = '目录层级 '
  split.segs.forEach((seg, i) => {
    out += seg
    if (i < gaps.length) {
      const g = gaps[i]
      if (g.any) out += '（任意层）'
      else if (g.n > 1) out += `（隔${g.n}级）`
      out += '/'
    }
  })
  return `${out}（自上而下）`
}

// refScopeGapWord 作用域附件层距词（决策7）：0=任意深度、1=紧邻、N≥2=隔 N 级。
export function refScopeGapWord(minCount: number | undefined): string {
  const n = minCount == null ? 1 : minCount
  if (n <= 0) return '任意深度'
  if (n >= 2) return `隔${n}级`
  return '紧邻'
}

// refScopeAssertPhrase 作用域附件谓词的人话句式（决策7 前端镜像）：库根来源
//「位于库根之下（紧邻/任意深度/隔N级）」（sentinel 分支不问 labelOf）；
// 规则来源完整描述形态「位于「〈供给方规则描述〉」之下（…）」（labelOf 解析
// 供给方 ID→规则描述）；labelOf 缺席时回落供给方 ID 直读形态
//「位于规则 X 之下（…）」——与后端 refScopeAssertPhrase 同源。
export function refScopeAssertPhrase(attach: CondxAttachForm, labelOf?: (ruleId: string) => string): string {
  const gap = refScopeGapWord(attach.minCount)
  if ((attach.value || '') === REFSCOPE_LIBRARY_ROOT) return `位于库根之下（${gap}）`
  const id = attach.value || '?'
  return labelOf ? `位于「${labelOf(id)}」之下（${gap}）` : `位于规则 ${id} 之下（${gap}）`
}

// 一条目录行的全部作用域附件直译句（按附加条件序以「 · 」并列；非目录行恒空）。
export function condxRefScopeNotes(condx: CondxForm | null | undefined, labelOf?: (ruleId: string) => string): string {
  if (!condx || condx.target !== 'dir') return ''
  const notes: string[] = []
  for (const a of condx.attach || []) {
    if (a && a.kind === 'refScope') notes.push(refScopeAssertPhrase(a, labelOf))
  }
  return notes.join(' · ')
}

// condxRefScopeProviders 条件行列表（含括号子组递归）引用的供给方规则 ID 集
//（refScope 行扫描，按出现序去重——镜像内核 refScopeProvidersOfRows）。
export function condxRefScopeProviders(rows: unknown): string[] {
  const out: string[] = []
  const walk = (list: CondxRowForm[]): void => {
    for (const r of list || []) {
      if (Array.isArray(r.group)) {
        walk(r.group)
        continue
      }
      for (const a of (r.condition && r.condition.attach) || []) {
        if (a && a.kind === 'refScope' && a.value && out.indexOf(a.value) < 0) out.push(a.value)
      }
    }
  }
  walk(Array.isArray(rows) ? (rows as CondxRowForm[]) : [])
  return out
}

// condxRowsAllDirTarget 纯目录规则判据（镜像内核 condxRowsAllDir，裁决③/自决2）：
// 全部条件行（含括号子组递归）目标=目录且至少一行；空表/非数组不满足
//（定群与聚合天然出局——判定只看行的目标类别，不猜布尔语义）。
export function condxRowsAllDirTarget(rows: unknown): boolean {
  if (!Array.isArray(rows)) return false
  let any = false
  const walk = (list: CondxRowForm[]): boolean => {
    for (const r of list) {
      if (Array.isArray(r.group) && r.group !== null) {
        any = true
        if (!walk(r.group)) return false
        continue
      }
      if (r.condition) {
        any = true
        if (r.condition.target !== 'dir') return false
      }
    }
    return true
  }
  if (!walk(rows as CondxRowForm[])) return false
  return any
}

// 模板词元切分 chips（位次角标供呈现）：字面段与占位词 chip 交替，同词根多位
// 各自成 chip；可多次出现词根（九词根全集）的位≥2 标注位次 occur 与寻址
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
  return { chips: condxApplyOccurs(parsed.chips || [], new Map<string, number>()) }
}

// chips 的出现序流（跨段连续编号——决策2：共享一次出现序计数，量词变体/
// 可选词位/普通词统一计数，{任意} 与可选分隔字面不计数）。
function condxApplyOccurs(chips: TemplateChip[], occ: Map<string, number>): CondxTplTok[] {
  return chips.map((c): CondxTplTok => {
    const hit = condxChipBase(c)
    if (!hit) return { ...c }
    const n = (occ.get(hit.base) || 0) + 1
    occ.set(hit.base, n)
    if (CONDX_REPEAT_GROUPS.indexOf(hit.base) < 0) return { ...c }
    return { ...c, occur: n, posName: condxPosName(hit.base, n), repeat: hit.repeat }
  })
}

// 模板词元的分段分组（剧本1 帧2）：多段目录模板按段分组（段内 chips + 段间
// 分隔符原文 sep），出现序流跨段共享（决策2——同词跨段按书写序连续编号，
// 位≥2 角标跨段连续）；无分隔符模板=单组（与既有 condxTplOccurChips 同构）。
// 段切分/词法失败返回 error——词元行静默不显示，行级校验红字承担报错。
export interface CondxTplSegGroup {
  toks: CondxTplTok[]
  // sep 为与后一段之间的分隔符书写原文（'/'、'//'、'/{N}'）；末段空串。
  sep: string
}

export function condxTplSegOccurChips(tpl: string): { groups?: CondxTplSegGroup[]; error?: string } {
  const split = splitDirTplSegments(tpl)
  if (split.error) return { error: split.error }
  if (split.segs.length <= 1) {
    const parsed = condxTemplateChips(tpl)
    if (parsed.error) return { error: parsed.error }
    return { groups: [{ toks: condxApplyOccurs(parsed.chips || [], new Map<string, number>()), sep: '' }] }
  }
  const occ = new Map<string, number>()
  const groups: CondxTplSegGroup[] = []
  for (let i = 0; i < split.segs.length; i++) {
    const parsed = condxTemplateChips(split.segs[i])
    if (parsed.error) return { error: parsed.error }
    groups.push({ toks: condxApplyOccurs(parsed.chips || [], occ), sep: i < split.seps.length ? split.seps[i] : '' })
  }
  return { groups }
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

// —— 括号子组结构化入口（行内开关/紧凑卡摘要/深度封顶/结构轻校验） ——

// 括号嵌套深度封顶：顶层=第 1 层，第 3 层内的行不再提供「括号」开关（更深的
// 组合走高级 JSON）。
export const CONDX_GROUP_MAX_DEPTH = 3

// 行表子树高度（本层=1）：叶子行=1、组行=1+组内最高；空表=0。深层怪态判定用。
export function condxRowsHeight(rows: CondxRowForm[]): number {
  let h = 0
  for (const row of rows) h = Math.max(h, row.group ? 1 + condxRowsHeight(row.group) : 1)
  return h
}

// 深层怪态判定（剧本三帧4）：组行所在层 depth（顶层=1）放进组内子树后的最深层
// 会超过封顶（depth + 组内高度 > CONDX_GROUP_MAX_DEPTH，即组内还套组套到第 4 层
// 及更深）——JSON 手写的合法形态，结构化模式不提供编辑：只读占位一行+可删，
// 保存原样带过（不跑结构归一，技术要点6）。
export function condxGroupBeyondDepth(group: CondxRowForm[], depth: number): boolean {
  return depth + condxRowsHeight(group) > CONDX_GROUP_MAX_DEPTH
}

// 接词显示词（且/或）；非 and/or 的值回退「；」——内核要求非首行接词必为
// and/or，回退仅防御 JSON 手写怪态。
export function condxConnLabel(connector?: string): string {
  return connector === 'and' ? '且' : connector === 'or' ? '或' : '；'
}

// 行表内每行的全局先序行号：叶子行=自身条件行序号；括号子组行=组内首行序号
//（空组=下一可用序号）——与 condxFlatRows 行号、内核 countCondxRows 先序累计
// 同口径（空叶子行占号与 condxFlatRows 一致）。
export function condxRowNos(rows: CondxRowForm[], base = 0): number[] {
  const out: number[] = []
  let n = base
  for (const row of rows) {
    out.push(n + 1)
    n += row.group ? condxFlatRows(row.group).length : 1
  }
  return out
}

// 行内容概要（摘要行共用的目标+格式段）：目标标签+「格式」（含这个词→含「词」；
// 模板→旧词归一后的模板文本）；目录行多段模板=层级直译句（决策7 前端镜像，
// 与后端 patternSummary 同源）；作用域附件句以「 · 」缀后（labelOf 供给方
// 完整描述解析，缺席时按供给方 ID 直读——后端同源形态）。
export function condxRowBrief(condx: CondxForm, labelOf?: (ruleId: string) => string): string {
  const scope = condxRefScopeNotes(condx, labelOf)
  if (condx.target === 'dir' && condx.format.mode === 'template' && (condx.format.tpl || '').indexOf('/') >= 0) {
    const tpl = condxNormalizeTpl(condx.format.tpl || '')
    const split = splitDirTplSegments(tpl)
    const base = split.error ? tpl : dirSegLevelPhrase(tpl, split.gaps)
    return scope ? `${base} · ${scope}` : base
  }
  const target = CONDX_TARGETS.find((t) => t.value === condx.target)
  const fmt = condx.format.mode === 'contains'
    ? `含「${condx.format.word || '?'}」`
    : condxNormalizeTpl(condx.format.tpl || '?')
  const body = `${target ? target.label : condx.target}「${fmt}」`
  return scope ? `${body} · ${scope}` : body
}

// 开关「开」（叶子行入组）：原行去接词入组（组内首行无接词）、组行继承原行接词
// 与「非」。就地改写传入行对象（行表为同一 reactive 数组，编辑实时生效）。
export function condxWrapRow(row: CondxRowForm): void {
  if (!row.condition) return
  row.group = [{ connector: '', negated: false, condition: row.condition }]
  row.condition = null
}

// 开关「关」（单成员组拆括号，入组的逆向展开）：还原行继承组行接词；行级「非」
// 双重否定相消（组行非与成员非异或——语义等价、成员内容原样保留）。
export function condxUnwrapRow(row: CondxRowForm): void {
  const group = row.group || []
  if (group.length !== 1) return
  const member = group[0]
  row.negated = !!row.negated !== !!member.negated
  row.condition = member.condition || null
  row.group = member.group || null
}

// 行表规整移除：splice 指定位后，删的是 0 位且仍有后行时清空新首行 connector——
// 首行接词控件不渲染（编辑器 v-if="i > 0"），残留接词在 UI 不可见、保存被内核
// validateCondxRowsAt 拒（「首行不能带行前连接词」，顶层与组内行表同规递归）。
// 所有条件行表移除路径统一经本函数：删普通行/删整组（CondxRowsEditor 卡与
// 组框头）、组内删空自动拆组的组行移除（CondxRowsEditor 组框 emptied 链）。
export function condxRemoveRow(rows: CondxRowForm[], idx: number): void {
  if (idx < 0 || idx >= rows.length) return
  rows.splice(idx, 1)
  if (idx === 0 && rows.length) rows[0].connector = ''
}

// 全树按引用同一性定位行所在的行表与下标（条件编辑弹窗「新增取消移除行」等跨层
// 行定位用；括号子组递归；找不到返回 null——行不在该树内）。
export function condxFindRowList(root: CondxRowForm[], row: CondxRowForm): { list: CondxRowForm[]; idx: number } | null {
  const idx = root.indexOf(row)
  if (idx >= 0) return { list: root, idx }
  for (const r of root) {
    if (Array.isArray(r.group)) {
      const hit = condxFindRowList(r.group, row)
      if (hit) return hit
    }
  }
  return null
}

// —— 条件行换位（上移/下移：内容交换 + 行号映射 + 引用重映射 + 相等门拦截） ——

// 换位方向（行头〔上移〕〔下移〕按钮语义）。
export type CondxMoveDir = 'up' | 'down'

// 「旧行号→新行号」全量映射（未受换位影响的行号为恒等项）。行号口径与
// condxFlatRows/condxRowNos 同源：先序展平、括号子组内行递归、空叶子行占号
//（草稿态显示语义，声明6——空行保存即拒、落档数据永无空行，落档形态下与
// 内核 countCondxRows 只数已填行等价）。查无键（引用行号越界/怪态）=不改写，
// 交由既有保存校验兜底。
export type CondxRowNoMap = Record<number, number | undefined>

// 换位落位（内容交换式，决策2）：rows[idx] 与相邻行（上移=前一行、下移=后一行）
// 交换除 connector 外的全部自身字段（negated/condition/group，及 JSON 手写可能
// 携带的其余字段——行间连接词留在原位，布尔骨架不动，首行规整天然无需）。
// 就地改写两行对象（行表为同一 reactive 数组，换位实时生效、不触发保存）。
// 边界越界（首行上移/末行下移/下标非法）返回 false 且不动任何行。
export function condxMoveSwapRows(rows: CondxRowForm[], idx: number, dir: CondxMoveDir): boolean {
  const j = dir === 'up' ? idx - 1 : idx + 1
  if (!Array.isArray(rows) || idx < 0 || idx >= rows.length || j < 0 || j >= rows.length) return false
  const a = rows[idx] as Record<string, unknown>
  const b = rows[j] as Record<string, unknown>
  for (const key of new Set([...Object.keys(a), ...Object.keys(b)])) {
    if (key === 'connector') continue
    const hasA = key in a
    const hasB = key in b
    const fromA = a[key]
    const fromB = b[key]
    if (hasB) a[key] = fromB
    else delete a[key]
    if (hasA) b[key] = fromA
    else delete b[key]
  }
  return true
}

// 行表的先序位数（换位映射的计数口径）：叶子行每位占一号（空行同占，与
// condxFlatRows 的行号递增一致——引用面行号即此口径）、括号子组行=组内递归
// 求和（空组占 0 位；条件/子组双占时组优先，与 condxFlatRows 的 group 分支
// 一致）。
function condxMoveRowsSpan(rows: CondxRowForm[]): number {
  let n = 0
  for (const row of rows) n += Array.isArray(row.group) ? condxMoveRowsSpan(row.group) : 1
  return n
}

function condxMoveRowSpan(row: CondxRowForm): number {
  return Array.isArray(row.group) ? condxMoveRowsSpan(row.group) : 1
}

// 在根行树内按引用同一性定位目标行表，返回其先序基位（该表首行之前累计的
// 叶子位数；表即根本身=0）。未找到（表不在根树内——调用方数据不同源）返回
// null，调用面按不可换位处理。
function condxMoveListBase(root: CondxRowForm[], target: CondxRowForm[], before: number): number | null {
  if (root === target) return before
  for (const row of root) {
    if (Array.isArray(row.group)) {
      const hit = condxMoveListBase(row.group, target, before)
      if (hit !== null) return hit
      before += condxMoveRowsSpan(row.group)
    } else {
      before += 1
    }
  }
  return null
}

// 全规则「旧行号→新行号」映射（决策3 的映射源）：list[idx] 与相邻行按内容
// 交换换位后，两行各自（括号子组整块、组内行递归）的新行号，其余行恒等。
// 须在 condxMoveSwapRows 落位**之前**按换位前形态调用（换位后同一调用得到的
// 是逆映射）。边界越界或 list 不在 root 树内（引用同一性定位失败）返回 null。
export function condxMoveRowNoMap(
  root: CondxRowForm[],
  list: CondxRowForm[],
  idx: number,
  dir: CondxMoveDir
): CondxRowNoMap | null {
  const j = dir === 'up' ? idx - 1 : idx + 1
  if (!Array.isArray(list) || idx < 0 || idx >= list.length || j < 0 || j >= list.length) return null
  const base = condxMoveListBase(root, list, 0)
  if (base === null) return null
  const first = Math.min(idx, j)
  const second = Math.max(idx, j)
  let start = base
  for (let k = 0; k < first; k++) start += condxMoveRowSpan(list[k])
  const headLen = condxMoveRowSpan(list[first])
  const tailLen = condxMoveRowSpan(list[second])
  const map: CondxRowNoMap = {}
  for (let n = 1; n <= condxMoveRowsSpan(root); n++) map[n] = n
  // 相邻两块整体对调：前块（list[first] 起占 headLen 位）后移 tailLen 位、后块
  //（相邻行）前移 headLen 位——组行与其组内行同块同移（剧本三：组内换位经
  // 全局行号映射改写主弹窗提取引用）。
  for (let n = 1; n <= headLen; n++) map[start + n] = start + tailLen + n
  for (let n = 1; n <= tailLen; n++) map[start + headLen + n] = start + n
  return map
}

// 三类行号引用重映射的消费面（提取行的结构子集——RuleEditDialog 的
// ExtractionRow 结构满足之；from/captureRow/when 三字段即重映射全部触点）。
export interface CondxMoveExtractionForm {
  from?: string
  captureRow?: number
  when?: unknown
}

// 三类行号引用重映射（决策3，换位后按映射就地改写草稿态——引用跟着行走）：
// ① 条件树值校验 againstRow（递归括号子组）；② 提取行 captureRow（仅
// from=capture——本规则行号寻址、缺省行号=1（声明3）；refCapture 的行号指向
// **被引成员规则**，不改写，声明9）；③ 提取行生效条件 when 树内的捕获操作数
//（行号前缀形态 rN.组名 改写 N——内核 rowCaptureName 同型、漂移扫描同源形态，
// 声明8；限定名 成员ID.rN.组名（三段）与裸组名等其余形态原样）。查无映射键的
// 越界引用保持原样，交由既有保存校验兜底。
export function condxMoveRemapRefs(
  rows: CondxRowForm[],
  extractions: CondxMoveExtractionForm[],
  rowNoMap: CondxRowNoMap
): void {
  const remapBadges = (list: CondxRowForm[]): void => {
    for (const row of list) {
      if (Array.isArray(row.group)) remapBadges(row.group)
      for (const b of (row.condition && row.condition.badges) || []) {
        if (!b || b.kind !== 'equal' || typeof b.againstRow !== 'number') continue
        const to = rowNoMap[b.againstRow]
        if (to !== undefined) b.againstRow = to
      }
    }
  }
  remapBadges(rows)
  for (const e of extractions || []) {
    if (!e) continue
    if (e.from === 'capture') {
      const eff = typeof e.captureRow === 'number' ? e.captureRow : 1
      const to = rowNoMap[eff]
      if (to !== undefined) e.captureRow = to
    }
    condxMoveRemapWhen(e.when, rowNoMap)
  }
}

// when 树递归改写捕获操作数：只认已知形态（kind=capture 且 captureName 为
// 字符串，与内核 ExprCapture 判别同源），其余节点/字段原样递归（结构化遍历，
// 手写怪态保守不动——风险2；漂移提示与保存期内核校验兜底）。
function condxMoveRemapWhen(node: unknown, rowNoMap: CondxRowNoMap): void {
  if (Array.isArray(node)) {
    for (const sub of node) condxMoveRemapWhen(sub, rowNoMap)
    return
  }
  if (!isCondNode(node)) return
  if (node['kind'] === 'capture' && typeof node['captureName'] === 'string') {
    const to = condxMoveRowPrefixNo(node['captureName'], rowNoMap)
    if (to !== undefined) node['captureName'] = to
  }
  for (const key of Object.keys(node)) condxMoveRemapWhen(node[key], rowNoMap)
}

// 行号前缀形态 rN.组名（恰两段：首段 rN——N≥1 无前导零，与内核
// isRowOrdinalPrefix 同口径；末段=组名/位次化组名，可带 #canon 规范名后缀）
// 改写 N 为映射后行号；其余形态（三段限定名 成员ID.rN.组名 指向被引成员、
// 裸组名、怪态）返回 undefined 不改写。
function condxMoveRowPrefixNo(name: string, rowNoMap: CondxRowNoMap): string | undefined {
  const parts = name.split('.')
  if (parts.length !== 2 || !parts[1]) return undefined
  if (!/^r[1-9][0-9]*$/.test(parts[0])) return undefined
  const to = rowNoMap[Number(parts[0].slice(1))]
  return to === undefined ? undefined : `r${to}.${parts[1]}`
}

// 相等门拦截判定（决策4）：模拟换位（按 condxMoveRowNoMap 重算先序号）后，
// 全树任一 equal 值校验的对方行新行号 ≥ 自身新行号即拦截——「与另一行相等须
// 引用更早行」约束会被破坏（判据与保存期校验同口径：前端 condxRowError 的
// against.row >= rowNum、内核 rule_condition.go 的 AgainstRow >= ordinal）。
// 返回拦截文案（空串=放行）：文案为悬停说明锚（剧本二帧1，上移/下移按方向），
// 行头按钮禁用态直接消费。边界越界/表不在树内不构成拦截（换位可行性由
// condxMoveSwapRows/condxMoveRowNoMap 兜）。
export function condxMoveBlockReason(root: CondxRowForm[], list: CondxRowForm[], idx: number, dir: CondxMoveDir): string {
  const map = condxMoveRowNoMap(root, list, idx, dir)
  if (!map) return ''
  for (const f of condxFlatRows(root)) {
    for (const b of f.condx.badges || []) {
      if (!b || b.kind !== 'equal' || typeof b.againstRow !== 'number') continue
      const refNo = map[b.againstRow]
      const selfNo = map[f.row]
      if (refNo !== undefined && selfNo !== undefined && refNo >= selfNo) {
        return dir === 'up'
          ? '上移会使"与另一行相等"校验引用的行晚于本行——请先调整该值校验'
          : '下移会使"与另一行相等"校验引用的行晚于本行——请先调整该值校验'
      }
    }
  }
  return ''
}

// 摘要段带计数的行概要（值校验/附加条数缀于内容概要后；内容概要内已含
// 多段直译句与作用域附件句）。
function condxSummaryBrief(condx: CondxForm, labelOf?: (ruleId: string) => string): string {
  const badges = (condx.badges || []).length ? ` ·${(condx.badges || []).length} 项值校验` : ''
  const attach = (condx.attach || []).length ? ` ·${(condx.attach || []).length} 条附加` : ''
  return `${condxRowBrief(condx, labelOf)}${badges}${attach}`
}

// 摘要行先序递归拼接（括号化）：组=圆括号包成员、接词随行、行号=全局先序号；
// 段间分隔——首段后为「；」，其后以接词（且/或）相连；空叶子行不进摘要。
// opts.refLabelOf 解析作用域附件供给方 ID→规则描述（完整描述形态直译句）。
export interface CondxRowsSummaryOpts {
  refLabelOf?: (ruleId: string) => string
}

export function condxRowsSummary(rows: CondxRowForm[], opts?: CondxRowsSummaryOpts): string {
  const labelOf = opts && opts.refLabelOf
  const sepFor = (emitted: number, top: boolean, connector?: string): string =>
    emitted === 0 ? '' : top && emitted === 1 ? '；' : ` ${condxConnLabel(connector)} `
  const build = (list: CondxRowForm[], base: number, top: boolean): string => {
    let out = ''
    let n = base
    let emitted = 0
    for (const row of list) {
      if (row.group) {
        out += sepFor(emitted, top, row.connector) + `（${build(row.group, n, false)}）`
        emitted += 1
        n += condxFlatRows(row.group).length
      } else if (row.condition) {
        out += sepFor(emitted, top, row.connector) + `第${n + 1}行 ${condxSummaryBrief(row.condition, labelOf)}`
        emitted += 1
        n += 1
      } else {
        n += 1
      }
    }
    return out
  }
  return build(rows, 0, true)
}

// 结构轻校验：显式空括号子组（group 数组在场且为空）与「条件/非空子组」双占为
// 不可存档形态——文案与内核 validateCondxRows 拒绝面逐字一致（空组行=「第 N 行
// 须为条件行或括号子组之一（非空）」、子组内错误以「第 N 行子组: 」前缀包装，
// 行号口径同内核：同级序号/组行全局先序号）。纯空叶子行（无条件且无 group 数组）
// 不在此列——行级「须选择目标与格式」提示与「至少需要一行条件」既有口径覆盖。
export function condxRowsShapeError(rows: CondxRowForm[]): string {
  return condxRowsShapeErrorAt(rows, 0)
}

function condxRowsShapeErrorAt(rows: CondxRowForm[], base: number): string {
  let prefix = 0
  for (let i = 0; i < rows.length; i++) {
    const row = rows[i]
    const group = Array.isArray(row.group) ? row.group : null
    if (group) {
      if (row.condition || group.length === 0) {
        return `第 ${i + 1} 行须为条件行或括号子组之一（非空）`
      }
      const inner = condxRowsShapeErrorAt(group, base + prefix)
      if (inner) return `第 ${base + prefix + 1} 行子组: ${inner}`
      prefix += condxFlatRows(group).length
    }
  }
  return ''
}

// —— 重复段词（占位词内部量词变体）前端轻量镜像（与内核 parseCondxVariant 同口径） ——

// 可带量词的词位（与内核 condxRepeatable 同源：仅 标签/作者/集名 三词位）。
export const CONDX_REPEAT_ROOTS = ['标签', '作者', '集名']

// 可多次出现（位次分立）的词根英文组名全集（与内核 condxRepeatableEn 同源：
// 标签/作者/集名/标题/序号/作品号/日期/作品标识/描述 九词根 → 全部取值词）——
// 同词根多位各自成条目、位次独立；{公共段}（key）维持单次，第二次出现维持
// 既有「重复」拒绝（轻校验镜像）；量词（+/*）仍限 CONDX_REPEAT_ROOTS 三词位，
// 与多次出现全集解耦。
export const CONDX_REPEAT_GROUPS = ['tag', 'author', 'setname', 'title', 'seq', 'workid', 'date', 'work', 'description']

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
    return { error: `模板占位词 {${word}} 的词根 {${root}} 不在词表（作者/集名/序号/标题/标签/作品标识/作品号/日期/描述/公共段/任意）` }
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
// 决策6），位 N≥2=「组名__N」（双下划线+位次；全组名均无下划线、后缀无碰撞）。
export function condxPosName(group: string, occur: number): string {
  return occur > 1 ? `${group}__${occur}` : group
}

// 位次表条目：模板内一个占位词出现（词元切分序）。group 为位次化组名=寻址键
// （captureName/限定名按此寻址）；sig 为记号签名（词根+记号形态归一——旧词
// {键} 与 {公共段} 同签名），供位次漂移 diff 判「指向变化」。
export interface CondxOccEntry {
  // word 为显示词：位1=词根现行词或变体书写原文；位≥2 追加「·第N处」。
  word: string
  // baseWord 为词根现行词（如 标签；旧词 {键}/{作品} 归一为 公共段/作品标识）。
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
// 同词根（可多次出现九词根全集）多位各自成条目、位次=出现序；{公共段} 第二次
// 出现由轻校验报错（条目仍按位产出，供行内标红下的呈现完整性）。可选词位
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
// 顺序=模板内出现序；旧词 {键}/{作品} 书写的模板归一为 {公共段}/{作品标识}
//（组名同 key/work，寻址
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
// {公共段}、{作品} 与 {作品标识} 同组，同模板并用即重复；可多次出现九词根
// 多次出现放行、位次分立，{公共段} 维持单次）/ {作品标识} 与 {作品号} 同模板
// 并用（语义近邻防混用）/ {公共段} 仅序列成员模板 / 相邻占位词无分隔 /
// 重复段词（量词变体）语法与形态（越权词位带量词、裸词无分隔、序列成员不支持
// 重复段）/ 可选记号拒绝面（{?} 空根、{任意?} 与 {公共段?}/{键?} 带问号、
// 可选分隔两侧无字面锚）。可选词位在序列模板解禁（可选捕获不进成套键——
// 全可选=空键守卫见 condxRowError）。target 传入时按主体镜像序列成员禁区；
// 文件名/序列行模板含「/」即拒（剧本1 帧3——层级表达属目录行，行校验与保存
// 闸同函数）；目录行模板的层距记号 /{N} 先折回「/」（段界合法性由
// splitDirTplSegments 在行级校验先行，此处只管段内词法——分隔符作字面、
// 位次流跨段连续）。返回中文错误（空串=通过）。
export function condxTemplateError(tpl: string, target?: 'file' | 'dir' | 'seq'): string {
  if ((target === 'file' || target === 'seq') && (tpl || '').indexOf('/') >= 0) {
    return '文件名模板不可含「/」——层级表达属目录行；文件名里没有这个字符，写了永不命中'
  }
  const checkTpl = target === 'dir' ? condxStripGapNotation(tpl || '') : tpl
  const parsed = condxTemplateChips(checkTpl)
  if (parsed.error) return parsed.error
  const chips = parsed.chips || []
  const seen = new Set<string>()
  // {作品标识} 与 {作品号} 同模板互斥登记（与内核 noteIDPair 同口径）：整词
  // 标识 vs 编号段语义近邻，并用易混用——普通词与可选词位两种书写面统一经此
  // 裁决（旧词 {作品} 经别名归组 work 同受约束）。
  let sawWork = false
  let sawWorkID = false
  const noteIDPair = (word: string, en: string): string => {
    if (en === 'work') {
      if (sawWorkID) {
        return `模板占位词 {${word}} 不可与 {作品号} 同模板并用：{作品标识} 取整词标识、{作品号} 取编号段，语义近邻易混用，请按文件名形态择一`
      }
      sawWork = true
    } else if (en === 'workid') {
      if (sawWork) {
        return `模板占位词 {${word}} 不可与 {作品标识} 同模板并用：{作品标识} 取整词标识、{作品号} 取编号段，语义近邻易混用，请按文件名形态择一`
      }
      sawWorkID = true
    }
    return ''
  }
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
        // 序列模板可选词位解禁（决策6）：可选捕获不进成套键、缺席成员照常入套
        // ——全可选=空键守卫在 condxRowError 序列分支拦截。
        const idErr = noteIDPair(c.text, v.group)
        if (idErr) return idErr
        // 位次分立：可多次出现九词根（标签/作者/集名/标题/序号/作品号/日期/
        // 作品标识/描述）多次出现放行、位次独立；{公共段} 第二次出现维持既有
        // 「重复」拒绝。
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
    if (!v) return `占位词「${c.text}」不在词表内（作者/集名/序号/标题/标签/作品标识/作品号/日期/描述/公共段/任意）`
    if (!v.refable) continue
    const idErr = noteIDPair(c.text, v.group)
    if (idErr) return idErr
    // {公共段} 仅序列成员模板（与内核 allowKey 同口径——target 缺省=迷你模板
    // 校验，后端按 allowKey:true 校验，此处同样放行）。
    if (target && target !== 'seq' && (c.text === '键' || c.text === '公共段')) {
      return `模板占位词 {${c.text}}：{公共段} 仅适用于序列成员格式`
    }
    // 位次分立：可多次出现九词根（标签/作者/集名/标题/序号/作品号/日期/
    // 作品标识/描述）多次出现放行、位次独立；{公共段} 重复维持既有报错文案
    //（{标签2} 词表外报错由上一行维持）。
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
  if (!CONDX_TARGETS.some((t) => t.value === target)) return '须选择目标（文件名/目录名/序列）'
  if (condx.format.mode === 'contains') {
    if (target !== 'dir') return '「含这个词」仅目录名目标可用'
    if (!(condx.format.word || '').trim()) return '包含词为空'
    if ((condx.format.word || '').indexOf('/') >= 0) return '包含词不可含「/」——目录名里没有这个字符，写了永不命中'
    if (condxTemplateChips(condx.format.word || '').chips) return '包含词须为纯文字（不含占位词）'
  } else {
    // 目录行多段模板：段界合法性（首尾分隔符/空段/层距记号值域）先行——
    // 与内核 splitDirTplSegments 同口径同文案。
    if (target === 'dir' && (condx.format.tpl || '').indexOf('/') >= 0) {
      const split = splitDirTplSegments(condx.format.tpl || '')
      if (split.error) return split.error
    }
    const err = condxTemplateError(condx.format.tpl || '', target)
    if (err) return err
    if (target === 'seq') {
      const chips = condxTemplateChips(condx.format.tpl || '').chips || []
      if (!chips.some((c) => c.ph && c.text === '序号')) return '序列目标模板须含 {序号}（成套键=其余捕获按序拼接）'
      // 全可选=空键守卫（新存档拦截，存量照旧）：与内核同口径——仅当模板含
      // 可选词位时启用（可选词位此前在序列模板整体被拒、存量档案不含此形态，
      // 无可选词位的既有书写不受新守卫影响）；可选捕获不进成套键，{序号} 之外
      // 再无必选取值词时键恒为空——同目录内一切命中文件同键并套。必选取值词=
      // 非可选词位/非可选分隔/非量词变体、词表内可引用且组名非 seq 的词
      //（{公共段} 计入；{任意} 吞位词不产捕获不计入）。
      const hasOptWord = chips.some((c) => c.ph && c.opt === 'word')
      if (hasOptWord) {
        const hasRequiredKeyWord = chips.some((c) => {
          if (!c.ph || c.opt || condxHasVariantMark(c.text)) return false
          const hit = condxVocabOf(c.text)
          return !!hit && hit.refable && hit.group !== 'seq'
        })
        if (!hasRequiredKeyWord) {
          return '序列成员格式的成套键为空：可选词位不参与成套键，{序号} 之外须至少保留一个必选取值词（或 {公共段}），否则同目录内所有命中文件会并成一套'
        }
      }
    }
  }
  const allowed = (CONDX_ATTACH_OPTIONS[target] || []).map((o) => o.value)
  const placeholders = condxRowPlaceholders(condx)
  for (const a of condx.attach || []) {
    if (allowed.indexOf(a.kind) < 0) return `终结谓词「${a.kind}」不适用于${target === 'file' ? '文件名' : target === 'dir' ? '目录名' : '序列'}目标`
    // 作用域附件（refScope）：来源值（供给方规则 ID 或库根 sentinel）非空 +
    // 层距值域（0=任意深度、1=紧邻、N≥2 显式隔 N 级——与内核 validateCondx
    // 同口径同文案方向）。缺省=库根后不再有未选态（sentinel 恒合法），空值
    // 仅为 JSON 手写兜底。
    if (a.kind === 'refScope') {
      if (!(a.value || '').trim()) return '附加条件「位于此目录下」须选择来源'
      const n = a.minCount == null ? 1 : a.minCount
      if (!(n === 0 || n === 1 || n >= 2)) return '附加条件 refScope 的层距无效（0=任意深度、1=紧邻、N≥2=隔 N 级）'
      continue
    }
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
// 与 {公共段}、{作品} 与 {作品标识} 同组同义——JSON 手写旧词照常通过，保存时
// 归一为新词）。
function badgePlaceholderInRow(placeholders: CondxOccEntry[], placeholder: string | undefined, occur: number | undefined): boolean {
  const own = condxVocabOf(placeholder || '')
  if (!own) return false
  return placeholders.some((p) => p.base === own.group && p.occur === (occur || 1))
}

// 全表校验：结构轻校验（空组/双占，文案与内核逐字一致）+ 行完整性（至少一行
// condition/Group 非空）+ 逐行 condxRowError。
export function condxRowsError(rows: CondxRowForm[]): string {
  const shapeErr = condxRowsShapeError(rows)
  if (shapeErr) return shapeErr
  const flat = condxFlatRows(rows)
  if (!flat.length) return '至少需要一行条件'
  for (const f of flat) {
    const err = condxRowError(f.row, f.condx, flat)
    if (err) return `第 ${f.row} 行：${err}`
  }
  return ''
}

// 捕获声明清单（v4：行号+占位词+英文组名）：提取段「行N·{词}」选项与清单
// 呈现用 word（现行词——旧词 {键}/{作品} 书写归一显示为 {公共段}/{作品标识}；
// 重复段词为模板书写原文如 标签+）；captureName 落 JSON 用 group（内核寻址
// 口径=英文组名）。
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

// —— 旧词归一（{键} → {公共段}、{作品} → {作品标识}：同一英文组名的同义词
// 书写，语义等价） ——

// 模板文本归一：整位占位词 {键} 替换为 {公共段}、{作品} 替换为 {作品标识}
//（仅花括号整位匹配，字面文本不动）。
export function condxNormalizeTpl(tpl: string): string {
  return (tpl || '').replace(/\{键\}/g, '{公共段}').replace(/\{作品\}/g, '{作品标识}')
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
      } else if ((key === 'placeholder' || key === 'againstPh') && v === '作品') {
        out[key] = '作品标识'
      } else {
        out[key] = walk(v)
      }
    }
    return out
  }
  return walk(rows) as T
}

// —— 类别形态推导（列表小字/行卡摘要同源） ——

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

// —— 卡片摘要派生（规则编辑面总览：条件卡/产出卡的一行摘要字段，纯函数） ——

// 条件卡模板摘要的截断长度（字符数，超出以「…」收尾）——卡片正文不随模板长度
// 增长，复杂度只进徽标计数（总览密度守恒）。
export const CONDX_CARD_TPL_MAX = 48

// 条件卡片摘要字段：序号+目标词+模板文本（截断）+复杂度徽标计数（附加×N/校验×N/
// 非/多值）+接词+作用域附件句。multiValue=模板含重复段词（多值产出）；levelNote
// 为目录行作用域附件（refScope）直译句（剧本2 帧2「{集名} · 位于「…」命中的
// 目录之下（紧邻）」式后缀）——字段名沿用既有卡片消费面（CondxRowsEditor 的
// 「· 」小字），语义已由层级锚（已退役）改为作用域句。opts.refLabelOf 解析
// 供给方 ID→规则描述（完整描述形态）；缺席时按供给方 ID 直读（后端同源形态）。
export interface CondxRowCardView {
  no: number
  targetLabel: string
  templateText: string
  attachCount: number
  badgeCount: number
  negated: boolean
  multiValue: boolean
  connectorLabel: string
  levelNote: string
}

// 卡片摘要选项：appendExt=模板文本尾接扩展名谓词取值（聚合轨行卡呈现——模板对
// 去扩展名的文件名正文求值、扩展名由 fileExt 谓词承载，拼接后卡片按轨行整体
// 形态呈现，如模板 {作品标识}.{任意} 配谓词 .srt 显示为 {作品标识}.{任意}.srt）；
// refLabelOf=作用域附件供给方 ID→规则描述的解析器（缺省按 ID 直读）。
export interface CondxRowCardOpts {
  appendExt?: boolean
  refLabelOf?: (ruleId: string) => string
}

// 行的扩展名谓词取值依序拼接（扩展名自带前导点；无扩展名谓词为空串）。
export function condxRowExtSuffix(condx: CondxForm): string {
  let out = ''
  for (const a of condx.attach || []) {
    if (a && a.kind === 'fileExt' && a.value) out += a.value
  }
  return out
}

export function condxRowCardView(row: CondxRowForm, no: number, opts?: CondxRowCardOpts): CondxRowCardView {
  const condx = row.condition
  if (!condx) {
    return {
      no,
      targetLabel: '—',
      templateText: '（未设条件）',
      attachCount: 0,
      badgeCount: 0,
      negated: !!row.negated,
      multiValue: false,
      connectorLabel: condxConnLabel(row.connector),
      levelNote: ''
    }
  }
  const target = CONDX_TARGETS.find((t) => t.value === condx.target)
  // 目录行多段模板：模板文本=层级直译句（决策7——卡片按直译句呈现）；其余
  // 形态维持模板原文（旧词归一）。
  let base: string
  if (condx.format.mode === 'contains') {
    base = `含「${condx.format.word || '?'}」`
  } else if (condx.target === 'dir' && (condx.format.tpl || '').indexOf('/') >= 0) {
    const tpl = condxNormalizeTpl(condx.format.tpl || '')
    const split = splitDirTplSegments(tpl)
    base = split.error ? tpl : dirSegLevelPhrase(tpl, split.gaps)
  } else {
    base = condxNormalizeTpl(condx.format.tpl || '')
  }
  const raw = opts && opts.appendExt ? `${base}${condxRowExtSuffix(condx)}` : base
  return {
    no,
    targetLabel: target ? target.label : String(condx.target),
    templateText: raw.length > CONDX_CARD_TPL_MAX ? `${raw.slice(0, CONDX_CARD_TPL_MAX)}…` : raw,
    attachCount: (condx.attach || []).length,
    badgeCount: (condx.badges || []).length,
    negated: !!row.negated,
    multiValue: condxRowAllPlaceholders(condx).some((p) => p.repeat),
    connectorLabel: condxConnLabel(row.connector),
    levelNote: condxRefScopeNotes(condx, opts && opts.refLabelOf)
  }
}

// 英文组名（含位次化 tag__2）→ 中文显示词（词表全词组名唯一；位次化组名按
// 词根中文+「·第N处」呈现）。
export function condxGroupWordOf(group: string): string {
  const m = /^(.+?)__(\d+)$/.exec(group)
  const base = m ? m[1] : group
  const hit = CONDX_VOCAB.find((v) => v.group === base)
  const word = hit ? hit.word : base
  return m ? `${word}·第${m[2]}处` : word
}

// 产出卡片摘要（单行）：维度 ← 来源摘要（capture/refCapture 携「第 N 行·{词}」
// 寻址、常量携值）+角色/仅序列成员小缀。refLabelOf 由调用方传入（供给方规则的
// 显示名解析，保持本函数纯——不挂档案数据源）。
export interface ExtractionCardSource {
  dimension: string
  from: string
  captureName?: string
  captureRow?: number
  refRule?: string
  constant?: string
  scanScope?: string
  roleName?: string
  seqOnly?: boolean
}

export interface ExtractionCardView {
  dimLabel: string
  fromText: string
  roleSuffix: string
}

export function extractionCardView(row: ExtractionCardSource, refLabelOf?: (ruleId: string) => string): ExtractionCardView {
  const dim = DIMENSION_OPTIONS.find((d) => d.value === row.dimension)
  const from = FROM_OPTIONS.find((f) => f.value === row.from)
  const rowNo = row.captureRow || 1
  let fromText = from ? from.label : row.from
  if (row.from === 'capture' && row.captureName) {
    fromText = `第 ${rowNo} 行·{${condxGroupWordOf(row.captureName)}}`
  } else if (row.from === 'refCapture' && row.captureName) {
    const label = refLabelOf && row.refRule ? refLabelOf(row.refRule) : row.refRule || '?'
    fromText = `${label}·第 ${rowNo} 行·{${condxGroupWordOf(row.captureName)}}`
  } else if ((row.from === 'constant' || row.from === 'adjudication') && (row.constant || '').trim()) {
    fromText = `${fromText} ${row.constant}`
  }
  const suffix: string[] = []
  if (row.roleName) suffix.push(`角色 ${row.roleName}`)
  if (row.seqOnly) suffix.push('仅序列成员')
  return { dimLabel: dim ? dim.label : row.dimension, fromText, roleSuffix: suffix.length ? ` · ${suffix.join(' · ')}` : '' }
}

// —— 聚合规则表单模型（两级类别·聚合声明镜像）——
// 纯函数与常量：作品标识声明（IdentityPart）、分轨入口（AggregateProduction/
// TrackAssign）的表单形态、类型投影消费（类型标签/角色标签/可用轨清单）与保存
// 闸门校验。载入侧校验在插件 Go 侧 validateAggregateEntry（前后双校验的另一面）。

// 序列群键保留捕获名（与内核 identityGroupKeyCapture 同值）：标识声明的专用取值
// ——取该行序列求值写入的群组键，只在序列目标行上可声明。
export const AGG_GROUP_KEY_CAPTURE = 'groupKey'

// 作品标识声明的一个部分（表单形态；落 JSON 字段 row/capture 与内核同形）。
export interface IdentityPartForm {
  row: number
  capture: string
}

// 分轨入口的单行角色分配（落 JSON 字段 row/role 与内核同形）。
export interface TrackAssignForm {
  row: number
  role: string
}

// 内容绑定（ContentBinding 表单形态；落 JSON 字段 row/dimension 与内核同形）：
// 把本规则一条条件行命中文件的伴随说明内容整段声明到某含义维度——一期零参数
//（整段→维度），一行内容可整段同时映射多个维度（多目标=同行多条目），同一行
// 同一维度唯一（重复勾选无效果）。绑定行可为轨行（并用时照常导入，内容叠加
// 产出）或说明行（不导入——处置归配对/聚合段）；「元数据文件」弹窗
//（MetadataFileDialog）是唯一指派面。
export interface ContentBindingForm {
  row: number
  dimension: string
}

// 配对方式取值（与内核 PairingMode 同集）：identity=标识配对（配对键部分=行号+
// 捕获名按声明序拼成复合标识，捕获值相等即一组——说明文件与资源文件各自从
// 自己的命中行取同名捕获）；directory=目录配对（同目录主导候选唯一时任意的
// 说明文件归并给该主导，不携带标识部分）。
export type PairingModeForm = 'identity' | 'directory'

// 配对方式选项（「配对方式：」选择器条目，剧本A 帧2 逐字词）。
export const PAIRING_MODE_OPTIONS: { value: PairingModeForm; label: string }[] = [
  { value: 'identity', label: '标识配对' },
  { value: 'directory', label: '目录配对' }
]

// 配对键声明（PairingKey 表单形态；落 JSON 字段 mode/identity 与内核同形）：
// 「元数据文件」产出自己的字段——与聚合规则的作品标识各自独立、并用互不牵连
//（规则级 identity 维持聚合专属）。映射单用（本规则无聚合产出）时必填，否则
// 说明成员无从归属；并用时可省略（省略=分组随聚合并组）。
export interface PairingKeyForm {
  mode: PairingModeForm
  identity?: IdentityPartForm[]
}

// 「元数据文件」产出表单形态（规则级独立落档位，与「聚合成作品」平级、各自可
// 单独添加或同规则并用）：内容绑定列表+配对键声明。元数据规则与聚合规则均可
// 携带；null=未加该产出（保存载荷不落键）。
export interface MetadataFileProductionForm {
  pairing?: PairingKeyForm
  contentBindings?: ContentBindingForm[]
}

// 分轨入口表单形态。companionUse 为规则级历史字段——内核口径=规则级仅 track
// 可写（空串载入归一 track；metadata 载入即拒并指路行级指派——处置语义由分轨
// 入口（轨道行=导入）+内容绑定勾选（内容产出元数据）合成表达，见插件 Go 侧
// validateAggregateEntry 的拒载文案）；前端表单不出现 metadata 选项，保存载荷
// 恒归一 track 落档。内容绑定已迁至规则级「元数据文件」产出
//（MetadataFileProductionForm），聚合产出不再携带。
export interface AggregateProductionForm {
  workType: string
  tracks: TrackAssignForm[]
  companionUse: string
}

// 类型投影条目（ListResourceTypes 应答条目的前端投影：结构角色与基数）。投影缺席
// （查询失败/无会话）时清单为空——分轨入口类型步呈现不可用态。
export interface AggRoleSpec {
  storeType: string
  min: number
  max: number
}

// 角色文件标准（类型投影 storeStandards 条目的前端投影：formats 为该角色的
// 标准扩展名清单，小写带点形——文件轨行尾部扩展名白名单的并集来源之一）。
export interface AggStandardSpec {
  formats?: string[]
}

export interface AggTypeSpec {
  resourceType: string
  roles: AggRoleSpec[]
  storeStandards?: Record<string, AggStandardSpec>
}

// 内置类型标签（分轨入口第一步的类型按钮文案）；unknown 无结构约束（roles 空）
// 不参与聚合、不出按钮，自定义类型照类型值原样呈现。
export const AGG_TYPE_LABELS: Record<string, string> = {
  image: '图片',
  video: '视频',
  article: '图文',
  document: '文档',
  audio: '音频'
}

// 分轨入口的角色标签（未知角色值照原样呈现，不虚构中文）。
export const AGG_ROLE_LABELS: Record<string, string> = {
  videoMain: '视频主体',
  videoTrack: '视频轨',
  audioTrack: '音频轨',
  audioMain: '音频主体',
  subtitle: '字幕轨',
  image: '图片',
  document: '正文',
  thumbnail: '缩略图'
}

export function aggTypeLabel(type: string): string {
  return AGG_TYPE_LABELS[type] || type
}

export function aggRoleLabel(role: string): string {
  return AGG_ROLE_LABELS[role] || role
}

// 角色短词（行标去「轨」形态，与 ParseView 确认框轨别摘要短名同口径）：元数据
// 文件弹窗的轨道随行标注用——剧本1 帧2「（轨道：视频）」「（轨道：字幕）」、
// 剧本2 帧2「（轨道：图片）」「（轨道：正文）」；未分配轨道行不取词（弹窗侧标
// 注「（未分配轨道）」）。未知角色值照原样呈现。
export const AGG_ROLE_SHORT_LABELS: Record<string, string> = {
  videoMain: '视频',
  subtitle: '字幕',
  image: '图片',
  document: '正文',
  thumbnail: '缩略图',
  videoTrack: '视频原料',
  audioTrack: '音频原料',
  audioMain: '音频主体'
}

export function aggRoleShortLabel(role: string): string {
  return AGG_ROLE_SHORT_LABELS[role] || role
}

// 内容去向维度枚举（与内核 knownBindingDimensions 同集=五维度全值；一行内容
// 整段可同时映射多个维度，各维独立走既有合成与冲突判定）。描述为本批主用形态
// 列首；顺序即弹窗勾选组呈现序（剧本A 帧2「☑描述 ☐标签 ☐标题 ☐作者 ☐作品集」）。
export const CONTENT_BINDING_DIMENSIONS: { value: string; label: string }[] = [
  { value: 'description', label: '描述' },
  { value: 'tag', label: '标签' },
  { value: 'title', label: '标题' },
  { value: 'author', label: '作者' },
  { value: 'collection', label: '作品集' }
]

export function contentBindingDimensionLabel(dim: string): string {
  const hit = CONTENT_BINDING_DIMENSIONS.find((d) => d.value === dim)
  return hit ? hit.label : dim
}

// 产出区「元数据文件」入口摘要（剧本A 帧1「元数据文件（0）」/帧2 系统回应
//「元数据文件（1）：第 2 行 → 描述」，多目标摘要连写「描述+标签」）：绑定按
// 行号升序、行内维度按枚举序以「+」连写、行间以「、」连排；计数=被指派行数
//（同行多维度合并为一段）。SubpageRules 规则列表摘要同词复用。
export function aggMetadataFileSummary(bindings: ContentBindingForm[] | null | undefined): string {
  const list = (bindings || []).filter(Boolean).slice().sort((a, b) => a.row - b.row)
  const byRow = new Map<number, string[]>()
  for (const b of list) {
    const dims = byRow.get(b.row) || []
    dims.push(b.dimension)
    byRow.set(b.row, dims)
  }
  if (!byRow.size) return '元数据文件（0）'
  const segs: string[] = []
  for (const [row, dims] of byRow) {
    const sorted = dims
      .slice()
      .sort((x, y) => CONTENT_BINDING_DIMENSIONS.findIndex((d) => d.value === x) - CONTENT_BINDING_DIMENSIONS.findIndex((d) => d.value === y))
      .map((v) => contentBindingDimensionLabel(v))
    segs.push(`第 ${row} 行 → ${sorted.join('+')}`)
  }
  return `元数据文件（${byRow.size}）：${segs.join('、')}`
}

// 类型清单（分轨入口第一步的按钮清单）：滤除无结构角色的类型（unknown），返回
// 标签与规约；投影缺席返回空清单。
export function aggTypeOptions(types: AggTypeSpec[]): { value: string; label: string; spec: AggTypeSpec }[] {
  const out: { value: string; label: string; spec: AggTypeSpec }[] = []
  for (const t of types || []) {
    if (!t || !t.resourceType || !(t.roles || []).length) continue
    out.push({ value: t.resourceType, label: aggTypeLabel(t.resourceType), spec: t })
  }
  return out
}

export function aggTypeSpecOf(types: AggTypeSpec[], type: string): AggTypeSpec | null {
  return (types || []).find((t) => t && t.resourceType === type) || null
}

// 角色是否必含（类型规约 Min≥1）：必含轨所在行承载聚合作品的主导文件与作品级
// 结论，并组判定要求该行每组恰一命中。
export function aggRoleRequired(spec: AggTypeSpec, role: string): boolean {
  for (const r of (spec && spec.roles) || []) {
    if (r && r.storeType === role) return r.min >= 1
  }
  return false
}

// 角色呈现（可用轨清单与下拉条目共用）：标签+必含标记。
export function aggRoleDisplay(spec: AggTypeSpec, role: string): string {
  return aggRoleRequired(spec, role) ? `${aggRoleLabel(role)}（必含）` : aggRoleLabel(role)
}

// 类型可用轨清单行：「类型「X」的可用轨：a（必含）· b · c」（角色按规约序）。
export function aggAvailableTracksText(spec: AggTypeSpec): string {
  const roles = (spec.roles || []).map((r) => (r ? aggRoleDisplay(spec, r.storeType) : '')).filter((s) => s)
  return `类型「${aggTypeLabel(spec.resourceType)}」的可用轨：${roles.join(' · ')}`
}

// 产出卡摘要：「聚合成作品 · 视频（主体+字幕轨）」——必含角色的轨行归并为「主体」，
// 其余角色按首现序各一条（多轨行同角色 ×N 缀尾）。类型缺席（悬空）按类型值原样。
export function aggProductionSummary(types: AggTypeSpec[], prod: AggregateProductionForm | null): string {
  if (!prod || !prod.workType) return '聚合成作品 · （未选类型）'
  const spec = aggTypeSpecOf(types, prod.workType)
  const segs: string[] = []
  const seen = new Set<string>()
  let hasRequired = false
  for (const ta of prod.tracks || []) {
    if (!ta || !ta.role) continue
    if (spec && aggRoleRequired(spec, ta.role)) {
      hasRequired = true
      continue
    }
    if (seen.has(ta.role)) continue
    seen.add(ta.role)
    const n = (prod.tracks || []).filter((t) => t && t.role === ta.role).length
    segs.push(n > 1 ? `${aggRoleLabel(ta.role)}×${n}` : aggRoleLabel(ta.role))
  }
  if (hasRequired) segs.unshift('主体')
  return `聚合成作品 · ${aggTypeLabel(prod.workType)}（${segs.join('+')}）`
}

// 标识候选（各轨行捕获汇集）：全部条件行的命名捕获（按行号序；重复段位多值
// 禁引剔除）+ 序列目标行的群键保留名。目录轨行的目录捕获即目录行的普通捕获，
// 天然在列。
export interface AggIdentityCandidate {
  row: number
  word: string
  group: string
}

export function aggIdentityCandidates(rows: CondxRowForm[]): AggIdentityCandidate[] {
  const out: AggIdentityCandidate[] = []
  for (const f of condxFlatRows(rows)) {
    for (const p of condxRowPlaceholders(f.condx)) {
      out.push({ row: f.row, word: p.word, group: p.group })
    }
    if (f.condx.target === 'seq') {
      out.push({ row: f.row, word: '群键', group: AGG_GROUP_KEY_CAPTURE })
    }
  }
  return out
}

// 标识部分的下拉显示词：群键保留名显示「群键」，其余按组名反查词条（含位次化
// 组名「词·第N处」）。
export function aggIdentityWordOf(capture: string): string {
  return capture === AGG_GROUP_KEY_CAPTURE ? '群键' : condxGroupWordOf(capture)
}

// 常见扩展名集（小写带点形）：文件轨行「模板尾部误写扩展名」识别白名单的基底
// ——与插件 Go 侧 knownFileExtsForTplTail 同源清单，前端另并入类型投影各角色
// 文件标准（storeStandards）的扩展名并集。
const COMMON_FILE_EXTS: string[] = [
  '.mp4', '.mkv', '.avi', '.mov', '.wmv', '.m4v', '.webm', '.flv', '.ts', '.m2ts',
  '.srt', '.ass', '.ssa', '.sub', '.vtt', '.txt', '.md', '.xml', '.nfo', '.rtf',
  '.odt', '.pdf', '.doc', '.docx', '.epub', '.jpg', '.jpeg', '.png', '.webp',
  '.gif', '.bmp', '.avif', '.heic', '.tiff', '.mp3', '.flac', '.wav', '.m4a',
  '.ogg', '.aac', '.mka', '.opus', '.zip', '.rar', '.7z', '.tar', '.gz', '.bz2',
  '.cbz', '.cbr'
]

// 扩展名记号归一（小写带点形）：容忍无点写法（"jpg"→".jpg"）；非「字母数字
// 串」记号（空串等）返回空串不入集。
function normalizeFileExtToken(raw: string): string {
  const m = /^\.?([A-Za-z0-9]+)$/.exec((raw || '').trim())
  return m ? `.${m[1].toLowerCase()}` : ''
}

// 文件轨行尾部扩展名识别白名单：常见扩展名集 ∪ 类型投影各角色文件标准
//（storeStandards）的扩展名并集。投影缺席（查询失败）时退化为常见扩展名集。
function aggFileExtWhitelist(types: AggTypeSpec[]): Set<string> {
  const out = new Set(COMMON_FILE_EXTS)
  for (const t of types || []) {
    const standards = (t && t.storeStandards) || {}
    for (const key of Object.keys(standards)) {
      for (const f of (standards[key] && standards[key].formats) || []) {
        const ext = normalizeFileExtToken(f)
        if (ext) out.add(ext)
      }
    }
  }
  return out
}

// 模板尾字面段（最后一个占位词之后的字面文本；无占位词即整条模板）以白名单
// 扩展名收尾时返回该扩展名（小写带点形），否则空串。文件主体模板对去扩展名的
// 文件名求值，尾字面 .ext 恒不命中——白名单外的点段（如 .part1）是文件名正文
// 内的合法点段，不拦。chips 解析失败（空模板等，行级校验另报）返回空串。
function condxTplTailKnownExt(tpl: string, known: Set<string>): string {
  const chips = condxTemplateChips(tpl).chips
  if (!chips || !chips.length) return ''
  const last = chips[chips.length - 1]
  if (last.ph) return ''
  const dot = last.text.lastIndexOf('.')
  if (dot < 0 || dot === last.text.length - 1) return ''
  const ext = last.text.slice(dot).toLowerCase()
  return known.has(ext) ? ext : ''
}

// 元数据类别行式底盘的模板尾内联扩展名守卫（决策8 甲案·与内核 ruleFromJSON
// 同口径同文案方向）：文件名行模板对去扩展名的文件名求值，尾字面白名单扩展
// 名恒不命中——此前元数据类别无守卫（静默死行），保存闸门补齐拦截+指路
// 「扩展名是」附加条件；dir/seq 目标行不受影响（名不去扩展名）。白名单基底=
// 常见扩展名集（内核 knownFileExtsForTplTail 同源清单；聚合面按类型投影并集、
// 元数据行无轨类型语境，取基底集）。聚合类别行不经本函数（聚合面闸门
// aggFormErrors「文件轨行」措辞先行，聚合路径零变化）。
export function condxFileRowInlineExtError(rows: CondxRowForm[]): string {
  const known = new Set(COMMON_FILE_EXTS)
  for (const f of condxFlatRows(rows)) {
    const c = f.condx
    if (!c || c.target !== 'file' || c.format.mode === 'contains') continue
    const ext = condxTplTailKnownExt(c.format.tpl || '', known)
    if (ext) {
      return `第 ${f.row} 行模板尾部误写扩展名 ${ext}：文件名行模板只匹配去扩展名的文件名，该形态恒不命中，请删除模板尾部的 ${ext}，改用「扩展名是」附加条件承载扩展名`
    }
  }
  return ''
}

// 聚合表单保存闸门（纯函数）：轨行结构限制（平行选择器——括号子组/行级非/跨行
// 相等均拒绝，与内核 validateAggregateEntry 同口径）+ 文件轨行模板尾部误写
// 扩展名拒绝（模板只匹配去扩展名的文件名，该形态恒不命中——与内核同口径）+
// 标识声明（行号界内、捕获在轨行声明、群键只选自序列行）+ 分轨入口（类型
// 在册、角色∈类型规约、行号界内、必含角色被至少一轨行覆盖）。内容绑定已迁至
// 规则级「元数据文件」产出（metadataFileFormErrors）。companionUse 处置维度不
// 进本闸门：内核口径=规则级仅 track 可写（metadata 载入即拒并指路行级指派——
// 分轨入口+内容绑定勾选合成表达），前端表单不出 metadata 选项、载荷恒归一
// track。返回错误清单（空=通过）；文案为编辑面锚文案。
export function aggFormErrors(
  rows: CondxRowForm[],
  identity: IdentityPartForm[],
  prod: AggregateProductionForm | null,
  types: AggTypeSpec[]
): string[] {
  const errs: string[] = []
  const flat = condxFlatRows(rows)
  const total = flat.length
  if (!flat.length) {
    errs.push('轨行区条件行列表不能为空（聚合规则须至少一行轨行）')
  }
  const knownExts = aggFileExtWhitelist(types)
  let n = 0
  const walkShape = (list: CondxRowForm[]): void => {
    for (const row of list) {
      if (row.group) {
        walkShape(row.group)
        continue
      }
      n += 1
      if (!row.condition) continue
      if (row.negated) {
        errs.push(`第 ${n} 行带行级非：轨行不支持行级非`)
      }
      for (const b of row.condition.badges || []) {
        if (b && b.kind === 'equal') {
          errs.push(`第 ${n} 行携带相等值校验：轨行独立求值，不支持跨行相等`)
        }
      }
      // 文件轨行模板尾部误写扩展名（仅 file 目标行；dir/seq 目标名不去扩展名，
      // 模板带点尾合法）：拒绝并指路 fileExt 谓词（「扩展名是」）。
      if (row.condition.target === 'file' && row.condition.format.mode !== 'contains') {
        const ext = condxTplTailKnownExt(row.condition.format.tpl || '', knownExts)
        if (ext) {
          errs.push(
            `第 ${n} 行模板尾部误写扩展名 ${ext}：文件轨行模板只匹配去扩展名的文件名，该形态恒不命中，请删除模板尾部的 ${ext}，改用「扩展名是」附加条件承载扩展名`
          )
        }
      }
    }
  }
  walkShape(rows)
  // 括号子组按最外层组计一次错（组行行号=组内首行序号，与行号口径一致）。
  let m = 0
  for (const row of rows) {
    if (row.group) {
      errs.push(`第 ${m + 1} 行为括号子组：轨行须为普通条件行（平行选择器，无且/或组合）`)
      m += condxFlatRows(row.group).length
      continue
    }
    m += 1
  }
  if (!identity.length) {
    errs.push('缺少作品标识声明——标识值相等即并组，请从各轨行捕获中选择标识')
  }
  const candidates = new Set(aggIdentityCandidates(rows).map((c) => `${c.row}/${c.group}`))
  const anyRowCaptures = new Set(aggIdentityCandidates(rows).map((c) => c.group))
  const rowByNo = new Map(flat.map((f) => [f.row, f.condx]))
  for (let i = 0; i < (identity || []).length; i++) {
    const part = identity[i]
    if (!part) continue
    if (part.row < 1 || part.row > total) {
      errs.push(`标识第 ${i + 1} 部分行号 ${part.row} 越界（共 ${total} 行）`)
      continue
    }
    if (!part.capture) {
      errs.push(`标识第 ${i + 1} 部分缺少捕获名`)
      continue
    }
    if (part.capture === AGG_GROUP_KEY_CAPTURE) {
      const target = rowByNo.get(part.row)
      if (!target || target.target !== 'seq') {
        errs.push(`标识第 ${i + 1} 部分：序列群键只能选自序列目标行`)
      }
      continue
    }
    if (!candidates.has(`${part.row}/${part.capture}`)) {
      errs.push(
        anyRowCaptures.has(part.capture)
          ? `标识第 ${i + 1} 部分引用的 {${aggIdentityWordOf(part.capture)}} 已不在第 ${part.row} 行声明（可改选轨行汇集候选中的同词位）`
          : `标识第 ${i + 1} 部分引用的 {${aggIdentityWordOf(part.capture)}} 未在任何轨行声明`
      )
    }
  }
  if (!prod) {
    errs.push('缺少分轨入口——产出区请加「聚合成作品」并选类型分配轨道')
  } else {
    if (!prod.workType) {
      errs.push('作品类型未选择（类型先行：类型规约决定可用轨与角色基数）')
    } else {
      const spec = aggTypeSpecOf(types, prod.workType)
      if (!spec) {
        errs.push(`作品类型「${aggTypeLabel(prod.workType)}」不在资源类型注册表（可能已随插件停用或卸载注销）`)
      } else {
        const legalRoles = new Set((spec.roles || []).map((r) => (r ? r.storeType : '')))
        for (let i = 0; i < (prod.tracks || []).length; i++) {
          const ta = prod.tracks[i]
          if (!ta) continue
          if (ta.row < 1 || ta.row > total) {
            errs.push(`分轨第 ${i + 1} 项行号 ${ta.row} 越界（共 ${total} 行）`)
            continue
          }
          if (!ta.role) {
            errs.push(`分轨第 ${i + 1} 项缺少角色`)
            continue
          }
          if (!legalRoles.has(ta.role)) {
            errs.push(`分轨第 ${i + 1} 项角色「${aggRoleLabel(ta.role)}」不在类型「${aggTypeLabel(prod.workType)}」的规约（下拉只出该类型可用轨）`)
          }
        }
        const covered = (prod.tracks || []).some((ta) => ta && legalRoles.has(ta.role) && aggRoleRequired(spec, ta.role))
        if (!covered) {
          errs.push('分轨入口未覆盖必含角色（类型规约 Min≥1 的角色）——主导文件无从判定，请把某轨行分到必含轨')
        }
      }
    }
  }
  return errs
}

// 「元数据文件」产出保存闸门（纯函数，与内核 validateMetadataFileProduction/
// validatePairingKey 同口径）：内容绑定（行号界内、维度合法枚举、同一行同一
// 维度唯一——一行内容整段可同时映射多个维度；勾选未选去向的行级拦截在元数据
// 文件弹窗内〔剧本A 帧3 ⚠ 行〕，此处兜保存面）+ 配对键（形态枚举合法；标识
// 配对=配对键部分的行号界内、捕获名引用存在、群键只选自序列目标行；目录配对
// 不携带标识部分）。映射单用（hasAggregate=false，本规则无聚合产出）时配对键
// 必填——否则说明成员无从归属；并用时可省略。mf 为 null（未加产出）恒通过。
export function metadataFileFormErrors(
  rows: CondxRowForm[],
  mf: MetadataFileProductionForm | null,
  hasAggregate: boolean
): string[] {
  const errs: string[] = []
  if (!mf) return errs
  const total = condxFlatRows(rows).length
  const bindings = mf.contentBindings || []
  if (!bindings.length) {
    errs.push('元数据文件产出缺少内容绑定（打开「元数据文件」弹窗勾选条件行并选内容去向）')
  }
  const dims = new Set(CONTENT_BINDING_DIMENSIONS.map((d) => d.value))
  const seenRowDims = new Set<string>()
  for (let i = 0; i < bindings.length; i++) {
    const b = bindings[i]
    if (!b) continue
    if (b.row < 1 || b.row > total) {
      errs.push(`内容绑定第 ${i + 1} 项行号 ${b.row} 越界（共 ${total} 行）`)
      continue
    }
    const key = `${b.row}/${b.dimension}`
    if (seenRowDims.has(key)) {
      errs.push(`内容绑定第 ${i + 1} 项与在先项重复指派第 ${b.row} 行的同一维度（同一行同一维度唯一）`)
      continue
    }
    seenRowDims.add(key)
    if (!b.dimension || !dims.has(b.dimension)) {
      errs.push(`内容绑定第 ${i + 1} 项维度非法: "${b.dimension}"（内容去向只出 描述/标签/标题/作者/作品集 枚举）`)
    }
  }
  // 配对键校验（与内核 validatePairingKey 同口径）
  const pairing = mf.pairing
  if (!pairing || !pairing.mode) {
    if (!hasAggregate) {
      errs.push('元数据文件产出缺少配对键声明（无聚合产出时须声明配对方式：标识配对或目录配对）')
    }
    return errs
  }
  if (pairing.mode === 'directory') {
    if ((pairing.identity || []).length) {
      errs.push('配对键为目录配对，不得携带标识配对部分（所在目录即配对键）')
    }
    return errs
  }
  if (pairing.mode !== 'identity') {
    errs.push(`配对方式非法: "${pairing.mode}"（identity=标识配对 / directory=目录配对）`)
    return errs
  }
  const parts = pairing.identity || []
  if (!parts.length) {
    errs.push('配对键为标识配对但缺少配对键部分（行·捕获名）')
    return errs
  }
  const candidates = new Set(aggIdentityCandidates(rows).map((c) => `${c.row}/${c.group}`))
  const anyRowCaptures = new Set(aggIdentityCandidates(rows).map((c) => c.group))
  const rowByNo = new Map(condxFlatRows(rows).map((f) => [f.row, f.condx] as const))
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i]
    if (!part) continue
    if (part.row < 1 || part.row > total) {
      errs.push(`配对键第 ${i + 1} 部分行号 ${part.row} 越界（共 ${total} 行）`)
      continue
    }
    if (!part.capture) {
      errs.push(`配对键第 ${i + 1} 部分缺少捕获名`)
      continue
    }
    if (part.capture === AGG_GROUP_KEY_CAPTURE) {
      const target = rowByNo.get(part.row)
      if (!target || target.target !== 'seq') {
        errs.push(`配对键第 ${i + 1} 部分：序列群键只能选自序列目标行`)
      }
      continue
    }
    if (!candidates.has(`${part.row}/${part.capture}`)) {
      errs.push(
        anyRowCaptures.has(part.capture)
          ? `配对键第 ${i + 1} 部分引用的 {${aggIdentityWordOf(part.capture)}} 已不在第 ${part.row} 行声明（可改选条件行汇集候选中的同词位）`
          : `配对键第 ${i + 1} 部分引用的 {${aggIdentityWordOf(part.capture)}} 未在任何条件行声明`
      )
    }
  }
  return errs
}
