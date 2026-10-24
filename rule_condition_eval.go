// v4 行作用域与值级小标求值（条件构建语法硬化期1 第二步）：每条条件行在
// 独立子上下文求值，捕获绑定以行号前缀（rN.组名）并入主上下文——跨行值级
// 相等（T2 作者壳制：目录行的 {作者} 与文件行的 {作者} 相等）引用前缀绑定，
// 行序即求值序（校验层已保证引用早于本行）。库名单小标支持形变归一（折叠
// _ - 与大小写后仍唯一才命中，多义不填）。纯模型，无 IO 与日志。
package main

import (
	"fmt"
	"strings"
)

// rowCaptureName 行号前缀绑定名：rN.组名。
func rowCaptureName(row int, name string) string {
	return fmt.Sprintf("r%d.%s", row, name)
}

// RowSubjectPattern 行包装模式：行内主体在独立子上下文求值（规则作用域内的
// 第二层私有——行间同名捕获互不遮蔽，T2 的两行 {作者} 各自独立可引用），
// 命中后以行号前缀并回捕获，再对小标门（Gates）求值。DirHits 并回供
// FromAncestorDir 提取「取最深命中」语义；Group 由主体写入子上下文后并回。
type RowSubjectPattern struct {
	// Ordinal 为行号（先序展平，1 起）。
	Ordinal int `json:"ordinal"`
	// Inner 为行内主体（及随主体谓词）。
	Inner Pattern `json:"inner"`
	// Gates 为值级小标门（引用本行前缀绑定；跨行相等门另引他行前缀绑定）。
	Gates []Pattern `json:"gates,omitempty"`
}

// patternMarker 实现 Pattern 接口标记。
func (p *RowSubjectPattern) patternMarker() {}

// Evaluate 行内主体先在子上下文求值：未中即整行不中；命中则前缀并回捕获与
// 目录命中、并回群组产物，随后全部小标门对主上下文求值（任一门未中整行不中；
// 未绑定引用保守为假，与 CondExpr 的未绑定语义一致）。
func (p *RowSubjectPattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	sub := &ParseContext{DirNames: ctx.DirNames, Depth: ctx.Depth}
	if !p.Inner.Evaluate(in, lib, sub) {
		return false
	}
	prefix := fmt.Sprintf("r%d.", p.Ordinal)
	for _, binding := range sub.Captures {
		ctx.Captures = append(ctx.Captures, CaptureBinding{
			Name: prefix + binding.Name, Value: binding.Value, Segment: binding.Segment,
			Start: binding.Start, End: binding.End,
		})
	}
	ctx.DirHits = append(ctx.DirHits, sub.DirHits...)
	ctx.PredHits = append(ctx.PredHits, sub.PredHits...)
	if sub.Group != nil {
		ctx.Group = sub.Group
	}
	for _, gate := range p.Gates {
		if !gate.Evaluate(in, lib, ctx) {
			return false
		}
	}
	return true
}

// BadgeGatePattern 值级小标门：对某行的某占位词绑定值断言。
//   - library：值在库名单（dim 维度集合）；fold=true 时形变归一——折叠 _ - 与
//     大小写后在名单中恰有一个来源名才命中（多义不填，宁缺勿错），命中并回
//     规范名绑定（原前缀绑定保留，规范名以 {前缀}canon 形态附加，供提取消费）；
//   - equal：与另一行（须早于本行，校验层保证）的另一占位词绑定值相等；
//   - constant：与常量相等。
type BadgeGatePattern struct {
	// Row / Name 为左值行号与前缀绑定名。
	Row  int    `json:"row"`
	Name string `json:"name"`
	// Kind 为小标类别（library / equal / constant）。
	Kind string `json:"kind"`
	// Dim 为库名单维度（library）。
	Dim string `json:"dim,omitempty"`
	// Fold 为形变归一开关（library）。
	Fold bool `json:"fold,omitempty"`
	// RightRow / RightName 为相等门右值（equal）。
	RightRow  int    `json:"rightRow,omitempty"`
	RightName string `json:"rightName,omitempty"`
	// Constant 为常量（constant）。
	Constant string `json:"constant,omitempty"`
}

// patternMarker 实现 Pattern 接口标记。
func (p *BadgeGatePattern) patternMarker() {}

// Evaluate 左值未绑定时保守为假（行未命中即门无意义）。
func (p *BadgeGatePattern) Evaluate(in *MatchInput, lib *LibraryIndex, ctx *ParseContext) bool {
	value, ok := ctx.Capture(rowCaptureName(p.Row, p.Name))
	if !ok {
		return false
	}
	switch p.Kind {
	case CondxBadgeLibrary:
		names, foldable := libraryIndexSet(lib, p.Dim)
		if names == nil {
			return false
		}
		if !p.Fold {
			if _, hit := names[value]; !hit {
				return false
			}
			ctx.PredHits = append(ctx.PredHits, PredicateHit{Kind: p.Dim, Input: value, Name: value})
			return true
		}
		_ = foldable
		canonical, hit := foldLookupUnique(names, value)
		if !hit {
			return false
		}
		ctx.PredHits = append(ctx.PredHits, PredicateHit{Kind: p.Dim, Input: value, Name: canonical})
		ctx.Captures = append(ctx.Captures, CaptureBinding{Name: rowCaptureName(p.Row, p.Name) + "#canon", Value: canonical})
		return true
	case CondxBadgeEqual:
		right, ok := ctx.Capture(rowCaptureName(p.RightRow, p.RightName))
		if !ok {
			return false
		}
		return value == right
	case CondxBadgeConstant:
		return value == p.Constant
	default:
		return false
	}
}

// libraryIndexSet 按维度取库名单集合（author/tag/collection → AuthorNames/
// TagNames/WorkSetNames）。未知维度或库快照缺失返回 nil。
func libraryIndexSet(lib *LibraryIndex, dim string) (map[string]struct{}, bool) {
	if lib == nil {
		return nil, false
	}
	switch dim {
	case "author":
		return lib.AuthorNames, true
	case "tag":
		return lib.TagNames, true
	case "collection":
		return lib.WorkSetNames, true
	default:
		return nil, false
	}
}

// foldNameKey 形变归一键：小写并剔除 _ - 空格（全半角）后拼接。
func foldNameKey(s string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case '_', '-', ' ', '　', '‐', '‑', '–', '—':
			continue
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// foldLookupUnique 折叠归一查找：值在名单中恰有一个折叠同形来源名时返回
// （该规范名, true）；零个或多个（多义）均不命中。
func foldLookupUnique(names map[string]struct{}, value string) (string, bool) {
	if _, exact := names[value]; exact {
		return value, true
	}
	key := foldNameKey(value)
	if key == "" {
		return "", false
	}
	canonical := ""
	count := 0
	for name := range names {
		if foldNameKey(name) == key {
			canonical = name
			count++
			if count > 1 {
				return "", false
			}
		}
	}
	if count == 1 {
		return canonical, true
	}
	return "", false
}
