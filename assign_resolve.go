package main

// —— 赋值就近解析器（目录粒度表示的读路径，纯函数层）——
//
// 表示契约（plan/目录粒度赋值表示方案.md『表示设计』）：Assignments 单映射共居两类
// 键——文件键=相对路径原样（永不以 / 结尾）、目录键=目录路径+尾斜杠（库根目录键=
// "/"）；判别规则一条：键以 / 结尾⟺目录条目。目录条目对该目录整个子树生效，文件
// 条目=就近覆盖例外。尾斜杠让两类键在键空间即互斥：旧文件条目 X（文件已删=死路
// 径）不会在新目录 X/ 出现后被祖先链上溯误读为目录覆盖。
//
// 解析口径：查文件赋值=沿「文件键→自身目录→…→库根」逐维上溯，取最近指定该维的
// 条目（逐维而非整条就近——目录条目未指定的维度由上层继续供值，与旧形态下每文件
// 条目已含各层动作合并值的求值结果一致）；条目间只合值与线索出处（clueRef 各维随
// 维走，维度覆盖口径与 mergeDirAssignment 一致）。Generalized=任一供值条目带标记
// 即真、UpdatedAtMs=供值条目最近值——两者为呈现口径，不参与求值终选。
//
// 取用通路两条：目录全集在手（识别求值装载）用 newAssignmentDirIndex 按父链一次
// DP 预计算每目录解析结果、逐文件 O(1) 取用（构造后只读，并行区零锁共享）；无全集
// 的散点读取（创建链等）用 resolveFileAssignment 沿链现算（O(层深) 次 map 探测）。

import "strings"

// isDirAssignmentKey 赋值键判别：以 / 结尾=目录条目（库根目录键="/"），否则文件键。
func isDirAssignmentKey(key string) bool {
	return strings.HasSuffix(key, "/")
}

// assignmentDirKeyOf 目录寻址键（相对库根的 '/' 目录路径、库根=空串，同 DirRelKey
// 形态）→ 赋值映射内的目录条目键：加尾斜杠，库根映射为 "/"。
func assignmentDirKeyOf(dirRelKey string) string {
	if dirRelKey == "" {
		return "/"
	}
	return dirRelKey + "/"
}

// dirRelKeyOfAssignment 赋值映射内的目录条目键 → 目录寻址键：剥尾斜杠，库根 "/"
// 归于空串。仅供目录键（isDirAssignmentKey 为真）调用。
func dirRelKeyOfAssignment(dirKey string) string {
	if dirKey == "/" {
		return ""
	}
	return strings.TrimSuffix(dirKey, "/")
}

// parentDirRelKeyOf 目录寻址键的父目录键：剥最末一级（一级目录归于库根空串）。
func parentDirRelKeyOf(dirRelKey string) string {
	idx := strings.LastIndexByte(dirRelKey, '/')
	if idx < 0 {
		return ""
	}
	return dirRelKey[:idx]
}

// dimProvenance 单维度供值条目的呈现位记录：聚合 Generalized/UpdatedAtMs 的原料，
// 随维度就近重记——条目被更近端覆盖后即不再计入呈现。
type dimProvenance struct {
	generalized bool
	updatedAtMs int64
}

// resolvedAssign 就近解析的逐维载体：四个含义维度各自独立持有最近供值条目的值、
// 线索出处与呈现位。
type resolvedAssign struct {
	assign        SessionAssignment // 值与线索出处的逐维合并结果（呈现位由 finalized 聚合填）
	srcAuthor     dimProvenance     // 作者维（含 roleName 与出处成组）供值呈现位
	srcTitle      dimProvenance     // 作品名维供值呈现位
	srcTag        dimProvenance     // 标签维供值呈现位
	srcCollection dimProvenance     // 作品集维供值呈现位
}

// overlay 就近覆盖一层条目：entry 显式指定的维度整组盖入（值+线索出处成组、作者维
// 含 roleName、标签维含逐位出处——覆盖口径与 mergeDirAssignment 一致），未指定的
// 维度保持既有供值；供值呈现位随维重记。标签切片复制入载体，使解析结果与赋值
// 映射及预计算表互不别名。
func (r *resolvedAssign) overlay(entry SessionAssignment) {
	if entry.Author != "" {
		r.assign.Author = entry.Author
		r.assign.AuthorRole = entry.AuthorRole
		r.assign.AuthorClueRef = entry.AuthorClueRef
		r.srcAuthor = dimProvenance{generalized: entry.Generalized, updatedAtMs: entry.UpdatedAtMs}
	}
	if entry.Title != "" {
		r.assign.Title = entry.Title
		r.assign.TitleClueRef = entry.TitleClueRef
		r.srcTitle = dimProvenance{generalized: entry.Generalized, updatedAtMs: entry.UpdatedAtMs}
	}
	if len(entry.Tags) > 0 {
		r.assign.Tags = append([]string(nil), entry.Tags...)
		r.assign.TagClueRefs = append([]string(nil), entry.TagClueRefs...)
		r.srcTag = dimProvenance{generalized: entry.Generalized, updatedAtMs: entry.UpdatedAtMs}
	}
	if entry.Collection != "" {
		r.assign.Collection = entry.Collection
		r.assign.CollectionClueRef = entry.CollectionClueRef
		r.srcCollection = dimProvenance{generalized: entry.Generalized, updatedAtMs: entry.UpdatedAtMs}
	}
}

// finalized 聚合呈现口径并产出解析结果（值语义，不改载体）：Generalized=任一供值
// 条目带标记即真、UpdatedAtMs=供值条目最近值；全链无供值=零值赋值。
func (r resolvedAssign) finalized() SessionAssignment {
	out := r.assign
	out.Generalized = r.srcAuthor.generalized || r.srcTitle.generalized ||
		r.srcTag.generalized || r.srcCollection.generalized
	out.UpdatedAtMs = max(r.srcAuthor.updatedAtMs, r.srcTitle.updatedAtMs,
		r.srcTag.updatedAtMs, r.srcCollection.updatedAtMs)
	return out
}

// resolveDirCarrier 无记忆的目录级就近解析：沿父链递归（祖先供值为底、本目录条目
// 逐维覆盖）返回载体，供继续近端覆盖（文件例外）或呈现聚合——散点现算与预计算
// DP 共用的核心。
func resolveDirCarrier(assigns map[string]SessionAssignment, dirRelKey string) resolvedAssign {
	var r resolvedAssign
	if dirRelKey != "" {
		r = resolveDirCarrier(assigns, parentDirRelKeyOf(dirRelKey))
	}
	if entry, ok := assigns[assignmentDirKeyOf(dirRelKey)]; ok {
		r.overlay(entry)
	}
	return r
}

// resolveFileAssignment 文件赋值散点现算（无目录全集时的逐文件读法，创建链等取
// 用）：所属目录链就近解析为底、文件例外条目逐维覆盖——等价于沿「文件键→自身
// 目录→…→库根」逐维取最近指定该维的条目，O(层深) 次 map 探测；全链无供值=零值。
func resolveFileAssignment(assigns map[string]SessionAssignment, relPath string) SessionAssignment {
	r := resolveDirCarrier(assigns, dirRelKeyOf(relPath))
	if entry, ok := assigns[relPath]; ok {
		r.overlay(entry)
	}
	return r.finalized()
}

// assignmentDirIndex 目录粒度就近解析预计算表：对给定目录集合按父链一次 DP 出每
// 目录解析结果（resolveDirCarrier 的记忆化；祖先目录不在集合内亦被顺带解析，供值
// 链完整）。构造完成后只读，供求值并行区零锁共享——逐文件取用 O(1)，未预计算目
// 录回落现算（不写表）。
type assignmentDirIndex struct {
	assigns map[string]SessionAssignment // 底层赋值映射（只读共享，不复制；构造后不得再修改）
	dirs    map[string]resolvedAssign    // 目录寻址键（库根=空串）→ 载体（含每维呈现位）
}

// newAssignmentDirIndex 构建预计算表：dirRelKeys 为目录寻址键集合（识别快照的目录
// 集，库根空串可含可不含——根链在 DP 中恒被解析）。
func newAssignmentDirIndex(assigns map[string]SessionAssignment, dirRelKeys []string) *assignmentDirIndex {
	idx := &assignmentDirIndex{assigns: assigns, dirs: make(map[string]resolvedAssign, len(dirRelKeys))}
	for _, dirRelKey := range dirRelKeys {
		idx.resolveDirDP(dirRelKey)
	}
	return idx
}

// resolveDirDP 单目录 DP（记忆化递归）：父链载体先行、本目录条目逐维就近覆盖后入
// 表；表内存呈现前形态（呈现位在取用时聚合，供更深层 DP 继承重记）。
func (idx *assignmentDirIndex) resolveDirDP(dirRelKey string) resolvedAssign {
	if r, ok := idx.dirs[dirRelKey]; ok {
		return r
	}
	var r resolvedAssign
	if dirRelKey != "" {
		r = idx.resolveDirDP(parentDirRelKeyOf(dirRelKey))
	}
	if entry, ok := idx.assigns[assignmentDirKeyOf(dirRelKey)]; ok {
		r.overlay(entry)
	}
	idx.dirs[dirRelKey] = r
	return r
}

// resolveFile 预计算取用：所属目录的 DP 结果为底、文件例外条目逐维覆盖（O(1)）；
// 所属目录不在表内（目录集不全）时回落散点现算，不写表、保持零锁只读。
func (idx *assignmentDirIndex) resolveFile(relPath string) SessionAssignment {
	if base, ok := idx.dirs[dirRelKeyOf(relPath)]; ok {
		r := base
		if entry, ok := idx.assigns[relPath]; ok {
			r.overlay(entry)
		}
		return r.finalized()
	}
	return resolveFileAssignment(idx.assigns, relPath)
}

// resolveDir 目录级就近解析取值（目录面板三态判定/清除应答增补的读法）：本目录
// 条目盖父链供值；不在表内时现算（不写表）。
func (idx *assignmentDirIndex) resolveDir(dirRelKey string) SessionAssignment {
	if r, ok := idx.dirs[dirRelKey]; ok {
		return r.finalized()
	}
	return resolveDirCarrier(idx.assigns, dirRelKey).finalized()
}
