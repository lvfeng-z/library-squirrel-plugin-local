// 目录问答分类器：目录层含义规则未覆盖时向前端发起问答（触发弹窗），答案即时
// 返回供本次运行消费，并吸收为新体系规则（Source=qna）持久化入规则档案——
// 档案是唯一学习载体，重启语义由 KV 落地承载。
package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// PathType 路径语义类型
type PathType string

const (
	PathTypeLocalAuthor PathType = "localAuthor"
	PathTypeSiteAuthor  PathType = "siteAuthor"
	PathTypeLocalTag    PathType = "localTag"
	PathTypeSiteTag     PathType = "siteTag"
	PathTypeWorkName    PathType = "workName"
	PathTypeWorkSet     PathType = "workSet"
	PathTypeSite        PathType = "site"
	PathTypeUnknown     PathType = "unknown"
)

// PathMeaning 单个路径段的含义
type PathMeaning struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ClassifyQuestion 发送给前端的分类问题
type ClassifyQuestion struct {
	Level   int      `json:"level"`
	DirName string   `json:"dirName"`
	Options []string `json:"options"`
}

// ClassifyResponse 前端返回的分类响应
type ClassifyResponse struct {
	Level    int           `json:"level"`
	DirName  string        `json:"dirName"`
	Meanings []PathMeaning `json:"meanings"`
	Cancel   bool          `json:"cancel,omitempty"`
}

// qnaDimensionOf 问答答案词汇 → 新体系含义维度：本地/站点作者同归作者维度，
// 本地/站点标签同归标签维度，作品名归标题，作品集归作品集；site 与 unknown
// 无对应维度（站点归属由归属裁决阶段消费、未知无含义），映射缺失即舍弃该答案。
var qnaDimensionOf = map[PathType]MetadataDimension{
	PathTypeLocalAuthor: DimAuthor,
	PathTypeSiteAuthor:  DimAuthor,
	PathTypeLocalTag:    DimTag,
	PathTypeSiteTag:     DimTag,
	PathTypeWorkName:    DimTitle,
	PathTypeWorkSet:     DimCollection,
}

// qnaAnswerOf 新体系含义维度 → 问答答案词汇（规则回放用语）：目录层声明的值
// 即目录名，统一落按名表达的本地域词汇；站点域身份不随目录层规则回放。
var qnaAnswerOf = map[MetadataDimension]PathType{
	DimAuthor:     PathTypeLocalAuthor,
	DimTag:        PathTypeLocalTag,
	DimTitle:      PathTypeWorkName,
	DimCollection: PathTypeWorkSet,
}

// qnaDimensionLabel 含义维度的人话标签，供生成规则的可读描述。
func qnaDimensionLabel(dim MetadataDimension) string {
	switch dim {
	case DimAuthor:
		return "作者"
	case DimTag:
		return "标签"
	case DimTitle:
		return "作品名"
	case DimCollection:
		return "作品集"
	default:
		return string(dim)
	}
}

// qnaRuleID 目录问答规则的唯一标识：层深 + 目录名锚定同一条问答。
func qnaRuleID(level int, dirName string) string {
	return fmt.Sprintf("qna-dir-%d-%s", level, dirName)
}

// qnaDirDepth 问答层深 → 规则档案目录深度（DirNames 索引）的换算：问答层深沿用
// 调用方编号——扫描根本身为 0、根下首层为 1；规则档案的 LocationPattern 深度是
// DirNames 的索引语义——DirNames 不含扫描根、根下首层为 0。两套编号恰差 1，
// 换算式 depth = level - 1；层深 0（扫描根本身）得 -1，表示无对应索引——扫描根
// 永不入 DirNames，位置规则无法锚定扫描根。
func qnaDirDepth(level int) int {
	return level - 1
}

// PathClassifier 路径分类器：与前端交互完成目录层含义问答，并把答案吸收为
// 规则档案内的 qna 规则。
type PathClassifier struct {
	ctx       sdkdto.PluginContext
	pendingCh chan *ClassifyResponse
}

// NewPathClassifier 创建路径分类器
func NewPathClassifier(ctx sdkdto.PluginContext) *PathClassifier {
	return &PathClassifier{
		ctx:       ctx,
		pendingCh: make(chan *ClassifyResponse, 1),
	}
}

// HandleResponse 处理前端分类响应
func (c *PathClassifier) HandleResponse(resp *ClassifyResponse) {
	select {
	case c.pendingCh <- resp:
	default:
	}
}

// ClassifyDir 对目录名进行分类，返回该目录的所有含义：规则档案（用户规则在前、
// 内置规则在后）已覆盖该层目录含义时依规则回放；未覆盖时向前端发起问答
// （弹窗行为不变），答案即时返回供本次运行消费，并吸收为一条 qna 规则持久化入档。
func (c *PathClassifier) ClassifyDir(level int, dirName string) ([]PathMeaning, error) {
	if meanings := c.meaningsFromRules(level, dirName); meanings != nil {
		c.ctx.Infof("目录分类命中规则档案: level=%d, dirName=%s, meanings=%+v", level, dirName, meanings)
		return meanings, nil
	}

	c.ctx.Infof("规则未覆盖该层目录含义，发起分类询问: level=%d, dirName=%s", level, dirName)

	question := &ClassifyQuestion{
		Level:   level,
		DirName: dirName,
		Options: []string{
			string(PathTypeLocalAuthor), string(PathTypeSiteAuthor),
			string(PathTypeLocalTag), string(PathTypeSiteTag),
			string(PathTypeWorkName), string(PathTypeWorkSet),
			string(PathTypeSite), string(PathTypeUnknown),
		},
	}
	data, err := json.Marshal(question)
	if err != nil {
		return nil, err
	}

	if err := c.ctx.PublishToFrontend("plugin:local-import:classify:request", data); err != nil {
		c.ctx.Warnf("发送分类请求失败: %v，使用默认规则", err)
		return []PathMeaning{{Type: string(PathTypeWorkName), Name: dirName}}, nil
	}
	c.ctx.Infof("分类请求已发送到前端，等待响应...")

	select {
	case resp := <-c.pendingCh:
		if resp.Cancel {
			return nil, fmt.Errorf("用户取消分类")
		}
		c.absorbResponse(level, dirName, resp)
		c.ctx.Infof("收到分类响应: level=%d, dirName=%s, meanings=%+v", level, dirName, resp.Meanings)
		return resp.Meanings, nil
	case <-time.After(5 * time.Minute):
		c.ctx.Warnf("分类超时（5分钟），使用默认规则")
		return []PathMeaning{{Type: string(PathTypeWorkName), Name: dirName}}, nil
	}
}

// meaningsFromRules 依规则档案回放该层目录含义：以启用规则序列调
// dirMeaningReplay 求值（档案载入在方法内完成）。问答通道只知层深与目录名，
// 祖先链以空占位构造（占位层不命中目录格式，组合规则的祖先依赖成员在门段
// 保守不命中——与文件级求值相比覆盖面收窄，仅影响本已退役的问答回放路径）。
func (c *PathClassifier) meaningsFromRules(level int, dirName string) []PathMeaning {
	rules := EffectiveRules(nil)
	if c.ctx != nil {
		profile, errs := LoadProfile(NewContextProfileStore(c.ctx))
		for _, err := range errs {
			c.ctx.Warnf("规则档案载入异常: %v", err)
		}
		if profile != nil {
			rules = EffectiveRules(profile)
		}
	}
	depth := qnaDirDepth(level)
	if depth < 0 {
		return nil
	}
	dirNames := make([]string, depth+1)
	dirNames[depth] = dirName
	meanings, _ := dirMeaningReplay(rules, dirNames, nil)
	return meanings
}

// dirMeaningReplay 目录层含义回放（纯求值，无 IO、不弹问）：识别全景元素1 的
// Meanings 与 MeaningRuleID 同源同序出自本实现。对给定目录链（dirNames = 库根
// 下各层目录名，目标层为末元素，须为真实目录名——祖先层供组合规则门段判定）
// 构造合成匹配输入（文件名留空使文件名规则不参与），逐条求值可表达目录层
// 含义的启用规则；组合规则（refs 非空）与提取段同语义地先过门段——被引成员
// 全体在同一合成输入上命中才参与（runRefGateStage，与 runExtractionStage 同一
// 实现），门过把成员捕获快照注入本规则作用域（refCapture 提取与 when 限定名
// 可用），且同受「之下不含其身」约束（自身目录行锚定层与被引目录成员命中层
// 重合时整体不中）；命中落点须恰为目标层目标名（祖先层命中不算覆盖），再执行
// 提取动作把声明转回路径含义并按（类型,名字）去重。返回（含义序列, 首个命中
// 规则 ID——不论该规则是否产出含义，与元素1「被哪条规则覆盖」口径一致）；
// 空链返回零值。
func dirMeaningReplay(rules []RuleEntry, dirNames []string, lib *LibraryIndex) ([]PathMeaning, string) {
	if len(dirNames) == 0 {
		return nil, ""
	}
	depth := len(dirNames) - 1
	dirName := dirNames[depth]
	in := &MatchInput{DirNames: dirNames}
	gate := runRefGateStage(rules, in, lib, &ParseContext{DirNames: dirNames, Depth: len(dirNames)})
	var meanings []PathMeaning
	var decls []MetadataDeclaration
	seen := make(map[PathMeaning]bool)
	firstHitRuleID := ""
	for _, rule := range rules {
		if !rule.Enabled || rule.Pattern == nil {
			continue
		}
		if isGroupingRule(&rule) {
			// 兄弟群组规则定群文件归属，不表达目录层含义
			continue
		}
		ctx := &ParseContext{DirNames: dirNames, Depth: len(dirNames)}
		if len(rule.Refs) > 0 {
			// 组合规则门段（与提取段同语义）：refs 全体成员在本回放输入上命中
			// 才参与；门过注入成员捕获快照（限定键通道）。
			if !gate.allHit(rule.Refs) {
				continue
			}
			injectRefSnapshots(ctx, gate, rule.Refs)
		}
		if !rule.Pattern.Evaluate(in, lib, ctx) {
			continue
		}
		if hit, ok := ctx.LastDirHit(); !ok || hit.Depth != depth || hit.Name != dirName {
			// 命中未落在本层目录名上（祖先层命中），不算覆盖
			continue
		}
		if len(rule.Refs) > 0 && gate.anchorCoincides(rule.Refs, depth) {
			// 之下不含其身（与提取段同判据）：目标目录即被引目录成员命中的
			// 目录 → 组合不覆盖本层。
			continue
		}
		if firstHitRuleID == "" {
			firstHitRuleID = rule.ID
		}
		for _, action := range rule.Extractions {
			// 条件动作对该回放语境下的解析上下文与已产出声明集求值（与提取段
			// 同语义），条件不满足的动作不产出含义
			if !action.When.Holds(ctx, decls) {
				continue
			}
			decl, ok := action.Apply(rule.ID, ctx)
			if !ok {
				continue
			}
			decls = append(decls, decl)
			answer, ok := qnaAnswerOf[decl.Dimension]
			if !ok {
				continue
			}
			meaning := PathMeaning{Type: string(answer), Name: decl.Value}
			if !seen[meaning] {
				seen[meaning] = true
				meanings = append(meanings, meaning)
			}
		}
	}
	return meanings, firstHitRuleID
}

// absorbResponse 把问答响应吸收为一条 qna 规则并持久化入档：答案词汇映射新体系
// 含义维度（无可映射维度的答案逐条告警后舍弃，全部不可映射时放弃生成），规则为
// v4 目录行（目标=目录、格式模板=目录名整名字面——任意祖先层命中），提取动作
// 按维度取 ancestorDir；层深 0（扫描根本身）无对应索引，不生成规则；持久化经
// LoadProfile→追加→SaveProfile，同层同目录锚点（同 ID）已有 qna 规则时原位替换，
// 新答案的维度集合整体取代旧规则。插件上下文不可用或档案不可载入时不落库（不
// 覆写既有档案），仅告警。
//
// 退役注记：此档案吸收写点随旧直接创建流程退役——Create 入口已改经工作台会话
// 分派（见 work_fetch.go CreateWithHeartbeat），ClassifyDir 弹问通道在全部创建
// 路径上结构性不可达，本写点随之不可达，保留为不可达历史代码（问答吸收机制
// 经单元测试直驱保活）；档案写一律经工作台会话管理器单点串行（泛化/裁决生成/
// 规则编辑面三入口，见 session.go UpdateProfile），本写点不纳入串行接入名单。
func (c *PathClassifier) absorbResponse(level int, dirName string, resp *ClassifyResponse) {
	if c.ctx == nil {
		return
	}
	depth := qnaDirDepth(level)
	if depth < 0 {
		// 层深 0 是扫描根本身：DirNames 不含扫描根，位置规则无从锚定，
		// 答案仅本次运行消费，不吸收为跨运行规则
		c.ctx.Warnf("扫描根本身不在规则层深编号内（DirNames 不含扫描根），问答答案不生成规则: level=%d, dirName=%s", level, dirName)
		return
	}
	var extractions []ExtractionAction
	var dims []MetadataDimension
	seenDim := make(map[MetadataDimension]bool)
	for _, m := range resp.Meanings {
		dim, ok := qnaDimensionOf[PathType(m.Type)]
		if !ok {
			c.ctx.Warnf("问答答案 %q 无对应含义维度，该答案不生成规则: level=%d, dirName=%s", m.Type, level, dirName)
			continue
		}
		if seenDim[dim] {
			continue
		}
		seenDim[dim] = true
		dims = append(dims, dim)
		extractions = append(extractions, ExtractionAction{Dimension: dim, From: FromAncestorDir})
	}
	if len(extractions) == 0 {
		c.ctx.Warnf("问答响应无可映射含义维度的答案，放弃生成规则: level=%d, dirName=%s", level, dirName)
		return
	}

	// v4 目录行：目录名整名字面模板（词法含花括号时放弃——占位词词法冲突）。
	if !condxTemplateBraceOK(dirName) {
		c.ctx.Warnf("目录名 %q 含花括号，与格式模板词法冲突，放弃生成规则", dirName)
		return
	}
	rows := []CondxRow{{Condition: &Condx{
		Target: CondxTargetDir,
		Format: CondxFormat{Mode: CondxFormatTemplate, Tpl: dirName},
	}}}
	pattern, err := CompileRuleConditions(rows)
	if err != nil {
		c.ctx.Warnf("构造问答规则目录条件失败，放弃生成规则: %v", err)
		return
	}
	labels := make([]string, 0, len(dims))
	for _, dim := range dims {
		labels = append(labels, qnaDimensionLabel(dim))
	}
	rule := RuleEntry{
		ID:          qnaRuleID(level, dirName),
		Description: fmt.Sprintf("问答生成：第 %d 层目录「%s」的含义为%s", level, dirName, strings.Join(labels, "、")),
		Source:      RuleSourceQna,
		Enabled:     true,
		Conditions:  rows,
		Pattern:     pattern,
		Extractions: extractions,
	}

	store := NewContextProfileStore(c.ctx)
	profile, errs := LoadProfile(store)
	for _, err := range errs {
		c.ctx.Warnf("规则档案载入异常: %v", err)
	}
	if profile == nil {
		c.ctx.Warnf("规则档案不可载入，问答规则不落库（不覆写既有档案）")
		return
	}
	replaced := false
	for i := range profile.Rules {
		if profile.Rules[i].Source == RuleSourceQna && profile.Rules[i].ID == rule.ID {
			profile.Rules[i] = rule
			replaced = true
			break
		}
	}
	if !replaced {
		profile.Rules = append(profile.Rules, rule)
	}
	if err := SaveProfile(store, profile); err != nil {
		c.ctx.Warnf("问答规则持久化失败: %v", err)
		return
	}
	action := "追加"
	if replaced {
		action = "替换既有规则"
	}
	c.ctx.Infof("问答规则已落库（%s）: id=%s, 含义维度=%v", action, rule.ID, dims)
}
