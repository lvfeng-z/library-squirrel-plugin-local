// 赋值压实迁移（目录粒度表示的数据层迁移，plan/目录粒度赋值表示方案.md
// 『迁移设计与回滚』）：v1 旧形态（逐文件全量条目）会话在首次全解码变异时
// 懒压实为目录粒度（开放决策④），压实前现行信封原文存入单代备份键（宁可不
// 压也不失备），回滚=备份原文写回信封键。压实只写备份键与信封键两个单键
// 原子写，不存在半压实形态（崩溃窗口与恢复语义见函数注释）。
package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// workbenchSessionMigrateBackupKeyPrefix 赋值压实迁移的单代备份键前缀（后接
// 归一化库根，键形态循 workbenchSessionKey 先例）：压实前现行信封原文存入
// 本键（单代备份，rules_store.go 的 RuleProfileBackupKVKey 先例形态）。前缀
// 与会话/分片/索引/创建标记键前缀互不重叠——ListSessions 的回退扫描
// （scanSessionRootsFromValues 按 "workbench-session:" 前缀识别）不会误读。
const workbenchSessionMigrateBackupKeyPrefix = "workbench-session-migrate-backup:"

// workbenchSessionMigrateBackupKey 库根 → 赋值压实迁移备份 KV 键（归一与会话
// 键同规则）。
func workbenchSessionMigrateBackupKey(root string) string {
	return workbenchSessionMigrateBackupKeyPrefix + normalizeLibraryRoot(root)
}

// assignmentFullyEqual 两赋值条目全字段相等：含全部值与线索出处维度，含
// Generalized/UpdatedAtMs 呈现位——压实按全字段相等归并，塌缩条目的呈现
// 口径与被消费的逐文件条目逐字段相同，就近解析的求值与呈现天然全等。
func assignmentFullyEqual(a, b SessionAssignment) bool {
	if a.Author != b.Author || a.AuthorRole != b.AuthorRole || a.AuthorClueRef != b.AuthorClueRef ||
		a.Title != b.Title || a.TitleClueRef != b.TitleClueRef ||
		a.Collection != b.Collection || a.CollectionClueRef != b.CollectionClueRef ||
		a.Generalized != b.Generalized || a.UpdatedAtMs != b.UpdatedAtMs ||
		len(a.Tags) != len(b.Tags) || len(a.TagClueRefs) != len(b.TagClueRefs) {
		return false
	}
	for i := range a.Tags {
		if a.Tags[i] != b.Tags[i] {
			return false
		}
	}
	for i := range a.TagClueRefs {
		if a.TagClueRefs[i] != b.TagClueRefs[i] {
			return false
		}
	}
	return true
}

// compactLegacyAssignments v1 旧形态赋值映射（逐文件全量条目）→ 目录粒度压实
// （确定性、无信息损失）。以快照文件集为底：按目录后序遍历（深先于浅），
// 目录子树内全部文件都有条目且值全字段相等 → 该目录可塌缩（最大塌缩：塌缩
// 链顶产一个目录条目，子树内全部文件条目与子目录产出被消费）；否则不产条目，
// 子目录已塌缩条目保留、直属异值文件保持文件条目。无条目文件视作异值（方案
// 风险2：否则目录条目会越权覆盖未赋值文件，迁移全等锚破坏）。死路径键（不在
// 快照文件集）原样保持文件条目、不参与压实；输入中已有的目录条目键（防御：
// 野生 v1 会话无目录键）原样保持并钉住其目录不塌缩。AssignRevision 由调用方
// 保持不递增（开放决策③：压实是纯表示变换，保增量识别资格）。
func compactLegacyAssignments(assigns map[string]SessionAssignment, files []SnapFile) map[string]SessionAssignment {
	live := make(map[string]bool, len(files))
	direct := make(map[string][]string) // 目录寻址键 → 直属文件相对路径（快照序）
	inTree := make(map[string]bool)     // 快照目录树（文件路径祖先链全量，含库根）
	for _, f := range files {
		live[f.RelPath] = true
		dir := dirRelKeyOf(f.RelPath)
		direct[dir] = append(direct[dir], f.RelPath)
		for ; ; dir = parentDirRelKeyOf(dir) {
			inTree[dir] = true
			if dir == "" {
				break
			}
		}
	}

	// 目录后序清单：按层深降序（子女恒先于父目录处理；同层目录互独立），
	// 子目录清单按同序装配。
	dirs := make([]string, 0, len(inTree))
	for dir := range inTree {
		dirs = append(dirs, dir)
	}
	dirDepth := func(dir string) int {
		if dir == "" {
			return 0
		}
		return strings.Count(dir, "/") + 1
	}
	sort.Slice(dirs, func(i, j int) bool { return dirDepth(dirs[i]) > dirDepth(dirs[j]) })
	children := make(map[string][]string, len(dirs))
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		parent := parentDirRelKeyOf(dir)
		children[parent] = append(children[parent], dir)
	}

	// 后序 DP：collapsed[dir] 非 nil=该目录子树整体可塌缩为该值（全部直属文件
	// 有条目且全字段相等 + 全部子目录已塌缩且值相等）；既有目录条目键钉住
	// 其目录不可塌缩（避免与钉住条目键冲突）。
	collapsed := make(map[string]*SessionAssignment, len(inTree))
	for _, dir := range dirs {
		if _, pinned := assigns[assignmentDirKeyOf(dir)]; pinned {
			continue
		}
		var uniform *SessionAssignment
		ok := true
		for _, rel := range direct[dir] {
			entry, has := assigns[rel]
			if !has {
				ok = false // 无条目文件=异值：本目录及其祖先链均不可归并
				break
			}
			if uniform == nil {
				v := entry
				uniform = &v
			} else if !assignmentFullyEqual(*uniform, entry) {
				ok = false
				break
			}
		}
		if ok {
			for _, child := range children[dir] {
				childValue := collapsed[child]
				if childValue == nil {
					ok = false
					break
				}
				if uniform == nil {
					v := *childValue
					uniform = &v
				} else if !assignmentFullyEqual(*uniform, *childValue) {
					ok = false
					break
				}
			}
		}
		if ok && uniform != nil {
			collapsed[dir] = uniform
		}
	}

	out := make(map[string]SessionAssignment, len(assigns))
	// 死路径键与既有目录条目键原样保持（不参与压实）
	for key, entry := range assigns {
		if isDirAssignmentKey(key) || !live[key] {
			out[key] = entry
		}
	}
	// 塌缩链顶产目录条目：父目录亦可塌缩时由父承载（最大塌缩；父不可塌缩
	// 意味着本目录是子树内最高的可塌缩层——条目落在本目录，供值范围自明）
	for dir, value := range collapsed {
		if dir != "" {
			if _, parentCollapsed := collapsed[parentDirRelKeyOf(dir)]; parentCollapsed {
				continue
			}
		}
		out[assignmentDirKeyOf(dir)] = *value
	}
	// 未被塌缩覆盖的活文件保持文件条目（其所属目录不可塌缩=子树内存在异值
	// 或缺条目文件，上级亦不可能已塌缩——可塌缩性沿树自上而下传递）
	for rel := range live {
		if _, covered := collapsed[dirRelKeyOf(rel)]; covered {
			continue
		}
		if entry, has := assigns[rel]; has {
			out[rel] = entry
		}
	}
	return out
}

// maybeCompactLegacyAssignmentsLocked 懒压实挂钩（调用方须持 mu；开放决策④：
// 首次全解码变异时触发，非首载强转）：会话结构版本=v1 旧形态、赋值非空且
// 快照在手 → ①现行信封原文（v1）存入单代备份键——备份失败即中止变异返回
// 错误（宁可不压也不失备）；②压实赋值；③结构版本置 2。挂钩**不写会话键**
//（只写备份键一次，压实当日一次性成本），会话保存由调用方各自负责、恰好
// 一次，两处调用点：
//   - 轻变异通路 mutateSessionFullDecodeLocked（任何赋值动作；快照=存储
//     现值）→ saveSession 轻保存落笔：分片零触碰；
//   - 识别轮完成通路 mutateSessionFull（挂钩置于 saveSessionFull 之前；
//     快照=变异闭包刚写入的新快照，压实以落库快照的文件集为底）→
//     saveSessionFull 全量落库：分片按全保存自身语义重写（**非分片零触碰**），
//     版本盖 2 与赋值压实随同一次信封写完成，无「版本已 2 而赋值未压」残留
//     窗口，亦无「轻存→再全存」双写（挂钩不带保存）。
// 崩溃窗口只此两个键（各为单键原子写）：备份写成而信封未回（下次变异重走
// 迁移，备份键被同内容覆写=幂等）；信封写回后（迁移已完成，备份在场供回
// 滚）。信封级轻变异（无快照在手）不压实——维持旧形态直到首个全解码变异。
func (m *SessionManager) maybeCompactLegacyAssignmentsLocked(root string, sess *WorkbenchSession) error {
	if sess.SchemaVersion != legacyWorkbenchSessionSchemaVersion ||
		len(sess.Assignments) == 0 || sess.Snapshot == nil {
		return nil
	}
	_, raw, err := m.readSessionEnvelope(root)
	if err != nil {
		return fmt.Errorf("读取现行会话信封（赋值迁移备份，键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	if raw == "" {
		return errors.New("现行会话信封缺席，赋值迁移前备份失败，本次操作未生效")
	}
	if err := m.store.SetValue(workbenchSessionMigrateBackupKey(root), raw); err != nil {
		return fmt.Errorf("赋值迁移前备份失败，本次操作未生效: %w", err)
	}
	sess.Assignments = compactLegacyAssignments(sess.Assignments, sess.Snapshot.Files)
	sess.SchemaVersion = workbenchSessionSchemaVersion
	m.logf("赋值已压实为目录粒度并升级会话结构版本 %d（root=%s）", workbenchSessionSchemaVersion, root)
	return nil
}

// RollbackAssignMigrateBackup 赋值压实迁移回滚（测试/降级用）：把单代备份键中
// 的压实前信封原文写回会话信封键——回写后即 v1 旧形态，新代码原生兼容读取
// （无只读破面），旧版插件亦可正常读取（降级场景可用）。备份缺席（该库根
// 从未压实）返回错误；回写后逐出会话缓存（缓存内的压实态实例不再服务）。
// 备份键保留（回滚本身可重复执行，同原文幂等覆写）。
func (m *SessionManager) RollbackAssignMigrateBackup(root string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := workbenchSessionMigrateBackupKey(root)
	backup, err := m.store.GetValue(key)
	if err != nil {
		return fmt.Errorf("读取赋值迁移备份（键 %q）失败: %w", key, err)
	}
	if backup == nil || backup.Value == "" {
		return errors.New("无可回退的赋值迁移备份（该库根未曾压实）")
	}
	if err := m.store.SetValue(workbenchSessionKey(root), backup.Value); err != nil {
		return fmt.Errorf("回写会话信封（键 %q）失败: %w", workbenchSessionKey(root), err)
	}
	m.evictSessionCache(normalizeLibraryRoot(root))
	return nil
}
