// Package share 负责共享目录的扫描与索引。
package share

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"filespace/internal/config"
	"filespace/internal/model"
)

var (
	// ErrFolderNotFound 共享目录不存在。
	ErrFolderNotFound = errors.New("共享目录不存在")
	// ErrPathForbidden 路径超出共享目录范围。
	ErrPathForbidden = errors.New("路径超出共享目录范围")
	// ErrFolderExists 目录已在共享列表中。
	ErrFolderExists = errors.New("目录已在共享列表中")
	// ErrNotDirectory 路径不是目录。
	ErrNotDirectory = errors.New("路径不是目录")
)

// Folder 一个共享目录。
type Folder struct {
	ID   string
	Name string
	Path string
	// Passwd 该文件夹的访问密码：为空表示对局域网开放；
	// 设置后其他节点需先认证（换取访问令牌）才能查看/下载内容，本机回环访问不受影响。
	Passwd string
	// RealPath 解析符号链接后的真实路径，仅用于内部去重（识别指向同一目录的重复添加）。
	RealPath string
}

// statsCheckInterval List 中对目录树做修改时间指纹校验的最大频率：
// 校验只遍历各级目录（stat 目录自身的 mtime，不 stat 文件），用于判断自上次
// 全量扫描以来目录树是否发生过增删改；只有校验发现变化才在后台触发全量重扫。
const statsCheckInterval = 5 * time.Second

// folderStats 一个共享目录的全量统计（文件数 / 总大小 / 最近更新）缓存。
type folderStats struct {
	count     int
	size      int64
	updated   time.Time
	scannedAt time.Time // 最近一次成功全量扫描的完成时间（零值表示尚未扫描）
	scanning  bool      // 是否已有后台扫描在进行（防止并发重复扫描）
	// dirs 最近一次扫描时树内每个目录（含根 "."）的相对路径 → mtime（UnixNano）指纹：
	// 任意位置的目录项新建/删除/重命名/移动都会更新所在目录的 mtime，
	// 指纹校验据此判断缓存是否过期——有变化才重扫，无变化的缓存不过期。
	dirs map[string]int64
	// checkedAt 最近一次做指纹校验的时刻（节流用，见 statsCheckInterval；测试可调小）。
	checkedAt time.Time
	// missing 最近一次扫描时目录不存在（被删除等，统计已清零）：
	// 目录持续缺失时视为无变化，避免每次 List 反复触发重扫。
	missing bool
}

// Manager 管理本节点共享的目录（支持运行中追加）。
// 目录统计（文件数 / 总大小 / 最近更新）采用「缓存 + 后台异步扫描」：
// List 直接返回缓存；缓存是否过期由目录树修改时间指纹判定——指纹有变化（目录项
// 增删改）才在后台重扫，无变化的缓存不过期，避免列表接口同步全量遍历大目录导致卡顿。
type Manager struct {
	mu      sync.RWMutex
	folders []Folder
	statsMu sync.RWMutex
	stats   map[string]*folderStats
	// checkInterval 指纹校验的最大频率（测试可调小；生产为 statsCheckInterval 常量默认值）。
	checkInterval time.Duration
}

// NewManager 根据配置创建共享目录管理器，为每个目录生成稳定 ID。
func NewManager(shared []config.SharedFolder) *Manager {
	folders := make([]Folder, 0, len(shared))
	for _, sf := range shared {
		name := sf.Name
		if name == "" {
			name = filepath.Base(sf.Path)
		}
		folders = append(folders, Folder{
			ID:       folderID(sf.Path),
			Name:     name,
			Path:     sf.Path,
			Passwd:   sf.Passwd,
			RealPath: realPath(sf.Path),
		})
	}
	m := &Manager{folders: folders, stats: make(map[string]*folderStats, len(folders)), checkInterval: statsCheckInterval}
	// 预创建统计条目：WarmUp / List 触发 scanAsync 时能找到目标，立即开始后台扫描
	for _, f := range folders {
		m.stats[f.ID] = &folderStats{}
	}
	return m
}

// Add 运行中追加一个共享目录：校验路径存在且为目录。
// password 为可选访问密码（空表示开放）；去重仅针对指向同一目录的重复
// （精确路径或符号链接解析后的真实路径相同），不阻止同时共享父目录与其子目录。
func (m *Manager) Add(path, password string) (Folder, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Folder{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return Folder{}, err
	}
	if !fi.IsDir() {
		return Folder{}, ErrNotDirectory
	}
	real := realPath(abs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.folders {
		if f.Path == abs || (f.RealPath != "" && f.RealPath == real) {
			return f, ErrFolderExists
		}
	}
	f := Folder{ID: folderID(abs), Name: filepath.Base(abs), Path: abs, Passwd: password, RealPath: real}
	m.folders = append(m.folders, f)
	m.statsMu.Lock()
	m.stats[f.ID] = &folderStats{}
	m.statsMu.Unlock()
	go m.scanAsync(f.ID) // 后台预扫一次，尽快填充统计缓存
	return f, nil
}

// HasPassword 是否存在设置了访问密码的共享文件夹。
func (m *Manager) HasPassword() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if m.folders[i].Passwd != "" {
			return true
		}
	}
	return false
}

// MatchPassword 判断密码是否匹配任一设置了访问密码的共享文件夹（常量时间比较，不暴露明文）。
func (m *Manager) MatchPassword(password string) bool {
	if password == "" {
		return false
	}
	hash := sha256.Sum256([]byte(password))
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if m.folders[i].Passwd == "" {
			continue
		}
		folderHash := sha256.Sum256([]byte(m.folders[i].Passwd))
		if subtle.ConstantTimeCompare(hash[:], folderHash[:]) == 1 {
			return true
		}
	}
	return false
}

// realPath 返回路径解析符号链接后的真实路径；解析失败时原样返回（去重退化为精确匹配）。
func realPath(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// Remove 按 ID 移除共享目录；不存在返回 ErrFolderNotFound。
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.folders {
		if m.folders[i].ID == id {
			m.folders = append(m.folders[:i], m.folders[i+1:]...)
			m.statsMu.Lock()
			delete(m.stats, id)
			m.statsMu.Unlock()
			return nil
		}
	}
	return ErrFolderNotFound
}

// List 返回共享文件夹列表（含全量统计：文件数 / 总大小 / 最近更新）。
// 统计来自缓存，缓存是否过期由目录树修改时间指纹判定（见 folderChanged）：
// 未扫描过、或指纹校验发现目录树发生变化时在后台异步重扫，本方法立即返回当前
// 缓存值，不会因大目录的全量遍历阻塞列表接口（本机管理页 / 节点发现 / 页面轮询都走这里）。
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
		needScan := false
		switch {
		case st.scannedAt.IsZero():
			// 尚未扫描过：后台补扫
			needScan = true
		case time.Since(st.checkedAt) >= m.checkInterval:
			// 校验节流到期：锁外按目录 mtime 指纹判断缓存是否过期（不 stat 文件）
			st.checkedAt = time.Now()
			dirs := st.dirs
			missing := st.missing
			m.statsMu.Unlock()
			if folderChanged(f.Path, dirs, missing) {
				needScan = true // 目录树发生过变化：后台重扫
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
			Auth:      f.Passwd != "",
		})
	}
	return result
}

// WarmUp 启动时后台预扫所有共享目录，填充统计缓存（不阻塞调用方；
// 目录多或体积大时并发扫描，扫描完成前 List 返回零值统计）。
func (m *Manager) WarmUp() {
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
// 并发安全：scanning 标志防止重复扫描；扫描期间不持有任何锁，不阻塞 List。
func (m *Manager) scanAsync(id string) {
	m.statsMu.Lock()
	st, ok := m.stats[id]
	if !ok || st.scanning {
		m.statsMu.Unlock()
		return
	}
	st.scanning = true
	m.statsMu.Unlock()
	defer func() {
		m.statsMu.Lock()
		st.scanning = false
		m.statsMu.Unlock()
	}()
	f, ok := m.Resolve(id) // 扫描期间目录可能已被移除
	if !ok {
		return
	}
	sc := scanFolder(f.Path)
	now := time.Now()
	m.statsMu.Lock()
	st.count, st.size, st.updated = sc.count, sc.size, sc.updated
	st.dirs, st.missing = sc.dirs, sc.missing
	// 新指纹即刻生效；重置校验节流，让刚完成的重扫结果至少保留一个校验周期
	st.scannedAt, st.checkedAt = now, now
	m.statsMu.Unlock()
}

// Resolve 按 ID 查找共享目录。
func (m *Manager) Resolve(id string) (*Folder, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if m.folders[i].ID == id {
			return &m.folders[i], true
		}
	}
	return nil, false
}

// folderID 根据路径生成稳定的目录 ID。
func folderID(path string) string {
	h := fnv.New32a()
	h.Write([]byte(path))
	return hex.EncodeToString(h.Sum(nil))
}

// folderScan 一次全量扫描的产物：文件统计 + 目录树 mtime 指纹快照。
type folderScan struct {
	count   int
	size    int64
	updated time.Time
	missing bool             // 目录不存在（被删除等）
	dirs    map[string]int64 // 树内所有目录（含根 "."）相对路径 → mtime（UnixNano）
}

// scanFolder 全量扫描一个共享目录：统计文件数 / 总大小 / 最近修改时间，
// 并记录树内各级目录的 mtime 指纹（供 List 按修改时间判断统计缓存是否过期）。
// 只在后台 goroutine 中调用（见 scanAsync），绝不直接在列表请求中同步执行。
func scanFolder(root string) folderScan {
	rootInfo, err := os.Stat(root)
	if err != nil {
		// 根目录不存在（被删除等）：统计清零并标记缺失，避免目录列表出现异常数据
		return folderScan{missing: true, dirs: map[string]int64{}}
	}
	sc := folderScan{dirs: map[string]int64{dirKey(root, root): rootInfo.ModTime().UnixNano()}}
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 单项不可访问（权限/竞态删除）时跳过，不中断整树扫描
		}
		if d.IsDir() {
			if info, err := d.Info(); err == nil {
				sc.dirs[dirKey(root, path)] = info.ModTime().UnixNano()
			}
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

// dirKey 计算树内路径相对根目录的指纹键（统一 / 分隔，根目录为 "."）。
func dirKey(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// folderChanged 按目录树修改时间指纹判断目录自上次扫描（dirs 快照）后是否变化。
//
// 原理：树内任意位置的「新建 / 删除 / 重命名 / 移动目录项」都会更新所在目录的
// mtime，因此递归比较各级目录的 mtime 即可感知任意深度的结构变化，且无需 stat 文件。
// 局限：原地改写文件内容只更新该文件自身 mtime（父目录 mtime 不变），此处无法感知，
// 相关统计差异会在下一次结构变化或进程重启时通过重扫刷新。
func folderChanged(root string, dirs map[string]int64, missing bool) bool {
	if _, err := os.Stat(root); err != nil {
		// 目录消失/不可访问：缓存尚未反映缺失（!missing）才视为变化触发重扫清零；
		// 已反映缺失则持续缺失不算变化，避免每次 List 反复触发重扫。
		return !missing
	}
	if missing {
		return true // 目录重新出现：重扫恢复统计
	}
	if len(dirs) == 0 {
		return true // 无指纹快照的异常状态，保守视为有变化
	}
	seen := make(map[string]bool, len(dirs))
	changed := false
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel := dirKey(root, path)
		seen[rel] = true
		if prev, ok := dirs[rel]; ok {
			if info, err := d.Info(); err == nil && info.ModTime().UnixNano() != prev {
				changed = true // 该目录下发生过目录项增删改
			}
		} else {
			changed = true // 树内出现了快照之外的新目录
		}
		return nil
	})
	if changed {
		return true
	}
	for rel := range dirs {
		if !seen[rel] {
			return true // 快照中的目录已从树内消失
		}
	}
	return false
}
