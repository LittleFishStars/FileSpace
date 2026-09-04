package share

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"filespace/internal/auth"
	"filespace/internal/config"
)

// Manager 管理本节点共享的目录（支持运行中追加）。
// 目录统计（文件数 / 总大小 / 最近更新）采用「缓存 + 后台异步扫描」，
// 由文件系统变更事件驱动刷新（见 dirWatcher）：List 只读缓存、永不因全量遍历阻塞，
// 具体的缓存与扫描逻辑在 stats.go，本文件聚焦注册表与密码查询。
type Manager struct {
	mu      sync.RWMutex
	folders []Folder
	statsMu sync.RWMutex
	stats   map[string]*folderStats

	lifeMu    sync.Mutex // 保护监听器启停
	watcher   *dirWatcher
	watchStop func()
	closed    bool // Close 后不再重启监听
	// debounce 目录变更事件的防抖窗口（测试可调小；生产默认 watchDebounce）。
	debounce time.Duration
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
			ID:         folderID(sf.Path),
			Name:       name,
			Path:       sf.Path,
			PasswdHash: passwdHashOf(sf),
			RealPath:   RealPath(sf.Path),
		})
	}
	m := &Manager{folders: folders, stats: make(map[string]*folderStats, len(folders)), debounce: watchDebounce}
	// 预创建统计条目：WarmUp / List 触发 scanAsync 时能找到目标，立即开始后台扫描
	for _, f := range folders {
		m.stats[f.ID] = &folderStats{}
	}
	return m
}

// Add 运行中追加一个共享目录：校验路径存在且为目录，随即开始监听其变更并后台预扫。
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
	real := RealPath(abs)
	m.mu.Lock()
	for _, f := range m.folders {
		if f.Path == abs || (f.RealPath != "" && f.RealPath == real) {
			m.mu.Unlock()
			return f, ErrFolderExists
		}
	}
	f := Folder{ID: folderID(abs), Name: filepath.Base(abs), Path: abs, PasswdHash: auth.OfPassword(password), RealPath: real}
	m.folders = append(m.folders, f)
	m.mu.Unlock()

	m.statsMu.Lock()
	m.stats[f.ID] = &folderStats{}
	m.statsMu.Unlock()

	if w := m.ensureWatcher(); w != nil {
		w.watchFolder(f.ID, abs)
	}
	go m.scanAsync(f.ID) // 后台预扫一次，尽快填充统计缓存
	return f, nil
}

// Remove 按 ID 移除共享目录（同时停止监听其变更）；不存在返回 ErrFolderNotFound。
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	idx := -1
	for i := range m.folders {
		if m.folders[i].ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		m.mu.Unlock()
		return ErrFolderNotFound
	}
	m.folders = append(m.folders[:idx], m.folders[idx+1:]...)
	m.mu.Unlock()

	m.statsMu.Lock()
	delete(m.stats, id)
	m.statsMu.Unlock()

	if w := m.currentWatcher(); w != nil {
		w.unwatchFolder(id)
	}
	return nil
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

// SharedSnapshot 返回当前共享目录列表（含密码哈希），供写回配置文件使用，
// 使运行中添加/移除/修改的共享目录与密码在重启后仍保持。
func (m *Manager) SharedSnapshot() []config.SharedFolder {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]config.SharedFolder, 0, len(m.folders))
	for i := range m.folders {
		out = append(out, config.SharedFolder{
			Path:       m.folders[i].Path,
			Name:       m.folders[i].Name,
			PasswdHash: m.folders[i].PasswdHash.Hex(),
		})
	}
	return out
}

// SetPassword 修改共享目录的访问密码（password 为空表示移除密码、恢复开放），
// 仅存储其哈希（见 auth.Hash），明文不驻留内存也不落盘。
// 按路径匹配：精确路径或符号链接解析后的真实路径相同即命中；未共享返回 ErrFolderNotFound。
func (m *Manager) SetPassword(path, password string) (Folder, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Folder{}, err
	}
	real := RealPath(abs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.folders {
		if m.folders[i].Path == abs || (m.folders[i].RealPath != "" && m.folders[i].RealPath == real) {
			m.folders[i].PasswdHash = auth.OfPassword(password)
			return m.folders[i], nil
		}
	}
	return Folder{}, ErrFolderNotFound
}

// HasPassword 是否存在设置了访问密码的共享文件夹。
func (m *Manager) HasPassword() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if !m.folders[i].PasswdHash.IsEmpty() {
			return true
		}
	}
	return false
}

// MatchPassword 判断密码是否匹配任一设置了访问密码的共享文件夹（常量时间比较，不暴露明文）。
func (m *Manager) MatchPassword(password string) bool {
	hash := auth.OfPassword(password)
	if hash.IsEmpty() {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if auth.Matches(m.folders[i].PasswdHash, hash) {
			return true
		}
	}
	return false
}

// FolderPasswdHash 加锁读取共享目录访问密码的哈希（供令牌校验使用，避免与运行中修改竞态）。
func (m *Manager) FolderPasswdHash(id string) (auth.Hash, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if m.folders[i].ID == id {
			return m.folders[i].PasswdHash, true
		}
	}
	return auth.Hash{}, false
}
