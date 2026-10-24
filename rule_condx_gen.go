// v4 泛化构造底座：占位词位分配器与「元素数组 → 模板」拼装器，供结构映射/
// 序列集名捕获/集规则调整等机器规则生产方共用（用户裁决 2026-10-24：机器流
// 与档案装载同一来源，v4 条件行为唯一规则形态）。
package main

import (
	"fmt"
	"strings"
)

// condxWordPool 占位词位分配器：引用位（提取槽/语义位）显式占用——同词重
// 复占用=词法禁止；泛化消费位不占池（一律 {任意} 吞位词，见 condxVocab）。
type condxWordPool struct {
	used map[string]bool
}

// newCondxWordPool 建池（空占用集）。
func newCondxWordPool() *condxWordPool {
	return &condxWordPool{used: map[string]bool{}}
}

// Take 显式占用 word；已被占用返回假（调用方降级或报错——词表每位一词）。
func (p *condxWordPool) Take(word string) bool {
	if p.used[word] {
		return false
	}
	p.used[word] = true
	return true
}

// condxTplFromEls 元素数组 → v4 模板拼装（字节级走查）：
//   - span [cs,ce) 为显式捕获区间（含剥壳内名边界调整），spanWord 一个占位词
//     整体覆盖（段间分隔与边界括号一并入位——与捕获组覆盖语义同构）；无区间
//     传 cs>=ce 且 spanWord 为空。
//   - wordByEl 为逐元素显式占位词（捕获位）；其余非分隔元素泛化消费：
//     数字段（elKindSeq）首取 {序号}（已占用则借数字类词），普通段按池序借词；
//   - 剥壳元素=开括号字面 + 占位词（盖内名）+ 闭括号字面（随后续间隙落位）；
//   - 分隔 run 按原文字面。
//
// 字面段含花括号返回错误（词法冲突，调用方降级）；相邻占位词不在本层报错
// （由条件编译层的 derive 校验承接，调用方以编译失败降级）。
func condxTplFromEls(base string, els []SnapEl, off []int, cs, ce int, spanWord string, wordByEl map[int]string, pool *condxWordPool) (string, error) {
	var b strings.Builder
	writeLiteral := func(s string) error {
		if !condxTemplateBraceOK(s) {
			return fmt.Errorf("字面段 %q 含花括号，与格式模板词法冲突", s)
		}
		b.WriteString(s)
		return nil
	}
	pos := 0
	// emitEl 单元素落位（泛化消费位词选择 + 剥壳括号留字面）。
	emitEl := func(i int) error {
		s, e := off[i], off[i+1]
		if e <= pos {
			return nil
		}
		el := els[i]
		if el.Kind == elKindSep {
			// 分隔 run 与其前残余间隙（前元素内名之后的闭括号等）一并字面。
			if err := writeLiteral(base[pos:e]); err != nil {
				return err
			}
			pos = e
			return nil
		}
		word, ok := wordByEl[i]
		if !ok {
			// 泛化消费位词选择：语义元素种类优先取同名占位词（author→作者、
			// workId→作品号——数字位保持数字类语义），序号位取 {序号}/数字
			// 类词，普通位一律 {任意}（非引用吞位词，不占词池）；同名词已
			// 被占用时退 {任意}（消费位不与引用位争词）。
			var w string
			if sem, known := condxWordOfElKind(el.Kind); known && !pool.used[sem] {
				w = sem
				pool.Take(w)
			} else if el.Kind == elKindSeq {
				if !pool.used["序号"] {
					w = "序号"
					pool.Take(w)
				} else if !pool.used["作品号"] {
					w = "作品号"
					pool.Take(w)
				} else if !pool.used["日期"] {
					w = "日期"
					pool.Take(w)
				} else {
					w = "任意"
				}
			} else {
				w = "任意"
			}
			word = w
		}
		// 词覆盖区：剥壳元素=内名（括号留字面）；其余=整元素。
		covS, covE := s, e
		if el.Shelled && el.Inner != "" {
			if io := strings.Index(base[s:e], el.Inner); io >= 0 {
				covS, covE = s+io, s+io+len(el.Inner)
			}
		}
		if covS < pos {
			covS = pos
		}
		if err := writeLiteral(base[pos:covS]); err != nil {
			return err
		}
		b.WriteString("{" + word + "}")
		pos = covE
		return nil
	}
	if spanWord != "" && cs < ce {
		// span 之前：元素逐一泛化走查（span 首元素的剥壳开括号等残余由
		// base[pos:cs] 字面段承接）。
		for i := range els {
			if off[i+1] <= cs {
				if err := emitEl(i); err != nil {
					return "", err
				}
			}
		}
		if err := writeLiteral(base[pos:cs]); err != nil {
			return "", err
		}
		b.WriteString("{" + spanWord + "}")
		pos = ce
	}
	for i := range els {
		if err := emitEl(i); err != nil {
			return "", err
		}
	}
	if err := writeLiteral(base[pos:]); err != nil {
		return "", err
	}
	return b.String(), nil
}

// condxWordOfSlot 显式捕获槽位 → 占位词（维度词表封闭；标签仅一位——多标签
// 位返回假，调用方降级手工）。
func condxWordOfSlot(dim MetadataDimension, tagIndex int) (string, bool) {
	if dim == DimTag && tagIndex > 1 {
		return "", false
	}
	return condxWordOfDim(dim)
}

// condxWordOfElKind 语义元素种类 → 同名占位词（泛化消费位优先按元素语义
// 落词——author 位落 {作者}、workId 位落 {作品号}，跨成员形态保真）。
func condxWordOfElKind(kind string) (string, bool) {
	switch kind {
	case elKindAuthor:
		return "作者", true
	case elKindWorkId:
		return "作品号", true
	case elKindTitle:
		return "标题", true
	}
	return "", false
}
