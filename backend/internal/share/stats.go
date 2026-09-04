package share

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"filespace/internal/model"
)

// folderStats 一个共享目录的全量统计（文件数 / 总大小 / 最近更新）缓存。
type folderStats struct {
	count     int
	size      int64
	updated   time.Time
	scannedAt time.Time // 最近一次成功全量扫描的完成时间（零值表示尚未扫描）
	scanning  bool      // 是否已有后台扫描在进行（防止并发重复扫描）
	dirty     bool      // 扫描期间又有变更事件到达：本次扫描完成后需立即重扫
	// missing 最近一次扫描时目录不存在（被删除等，统计已清零）。
	// 目录整体被删除后其事件监听随目录消失而失效，重建的目录不会再产生事件，
	// List 据此标记检测目录重现：发现重现则补扫并重新挂载监听。
	missing bool
}

// List 返回共享文件夹列表（含全量统计：文件数 / 总大小 / 最近更新）。
// 统计来自缓存，由文件变更事件在后台刷新（见 dirWatcher），本方法不主动扫描，
// 仅兜底两种情况：① 从未扫描过 → 后台补扫；② 目录曾被删除（missing）而现已
// 重现（重建目录不在事件监听范围内）→ 后台补扫并重新监听。
// 无论哪种情况都立即返回当前缓存值，不会因大目录的全量遍历阻塞列表接口。
func (m *Manager) List() []model.FolderInfo {
	m.mu.RLock()
	folders := make([]Folder, len(m.folders))
	copy(folders, m.folders)
	m.mu.RUnlock()
	result := make([]model.FolderInfo, 0, len(folders))
	for _, f := range folders {
		m.statsMu.Lock()
		st, ok := m.stats[f.ID]
		if !ok {
			st = &folderStats{}
			m.stats[f.ID] = st
		}
		needScan := st.scannedAt.IsZero()
		if !needScan && st.missing {
			// 目录重现检测：仅当缓存标记缺失时 stat 一次（非常见路径，非轮询）
			m.statsMu.Unlock()
			if _, err := os.Stat(f.Path); err == nil {
				needScan = true
				if w := m.currentWatcher(); w != nil {
					w.watchFolder(f.ID, f.Path) // 重新挂载监听（此前随目录删除失效）
				}
			}
			m.statsMu.Lock()
		}
		count, size, updated := st.count, st.size, st.updated
		m.statsMu.Unlock()
		if needScan {
			go m.scanAsync(f.ID) // 本次仍返回缓存值，永不阻塞请求
		}
		result = append(result, model.FolderInfo{
			ID:        f.ID,
			Name:      f.Name,
			Path:      f.Path,
			FileCount: count,
			TotalSize: size,
			UpdatedAt: updated.Format(time.RFC3339),
			Auth:      !f.PasswdHash.IsEmpty(),
		})
	}
	return result
}

// WarmUp 启动时后台预扫所有共享目录，填充统计缓存，并开始监听目录变更事件
// （不阻塞调用方；目录多或体积大时并发扫描，扫描完成前 List 返回零值统计）。
func (m *Manager) WarmUp() {
	m.ensureWatcher()
	m.mu.RLock()
	ids := make([]string, len(m.folders))
	for i, f := range m.folders {
		ids[i] = f.ID
	}
	m.mu.RUnlock()
	for _, id := range ids {
		go m.scanAsync(id)
	}
}

// scanAsync 后台全量扫描一个共享目录并更新统计缓存。
// 并发安全：scanning 标志防止重复扫描；若扫描期间又有变更事件到达（dirty），
// 本次结果作废并立即重扫，保证统计不落后于磁盘；扫描期间不持有任何锁，不阻塞 List。
func (m *Manager) scanAsync(id string) {
	for {
		m.statsMu.Lock()
		st, ok := m.stats[id]
		if !ok {
			m.statsMu.Unlock()
			return
		}
		if st.scanning {
			st.dirty = true // 已有扫描在跑：标记待重扫，避免事件驱动下的变更丢失
			m.statsMu.Unlock()
			return
		}
		st.scanning = true
		st.dirty = false
		m.statsMu.Unlock()

		f, ok := m.Resolve(id) // 扫描期间目录可能已被移除
		if !ok {
			m.statsMu.Lock()
			st.scanning = false
			m.statsMu.Unlock()
			return
		}
		sc := scanFolder(f.Path)
		m.statsMu.Lock()
		if st.dirty {
			// 扫描期间目录又发生变化：本次结果作废，立即重扫
			st.scanning = false
			m.statsMu.Unlock()
			continue
		}
		if sc.missing {
			// 目录不存在（被删除等）：统计清零并标记缺失，避免目录列表出现异常数据
			st.count, st.size, st.updated = 0, 0, time.Time{}
			st.missing = true
		} else {
			st.count, st.size, st.updated, st.missing = sc.count, sc.size, sc.updated, false
		}
		st.scannedAt = time.Now()
		st.scanning = false
		m.statsMu.Unlock()
		return
	}
}

// folderScan 一次全量扫描的产物。
type folderScan struct {
	count   int
	size    int64
	updated time.Time
	missing bool // 目录不存在（被删除等）
}

// scanFolder 全量扫描一个共享目录，统计文件数、总大小与最近修改时间。
// 只在后台 goroutine 中调用（见 scanAsync），绝不直接在列表请求中同步执行。
func scanFolder(root string) (sc folderScan) {
	if _, err := os.Stat(root); err != nil {
		sc.missing = true
		return sc
	}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 单项不可访问（权限/竞态删除）时跳过，不中断整树扫描
		}
		if d.IsDir() {
			return nil
		}
		sc.count++
		if info, err := d.Info(); err == nil {
			sc.size += info.Size()
			if info.ModTime().After(sc.updated) {
				sc.updated = info.ModTime()
			}
		}
		return nil
	})
	return sc
}
