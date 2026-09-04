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
	// PasswdHash 该文件夹访问密码的 sha256 十六进制哈希：为空表示对局域网开放；
	// 非空表示其他节点需先认证（换取访问令牌）才能查看/下载内容，本机回环访问不受影响。
	// 程序内不保存明文密码，设置密码的入口（配置 / CLI / API）统一先哈希再入库。
	PasswdHash string
	// RealPath 解析符号链接后的真实路径，仅用于内部去重（识别指向同一目录的重复添加）。
	RealPath string
}

// hashPassword 计算访问密码的 sha256 十六进制哈希（空密码返回空串，表示开放）。
// 认证令牌本就绑定密码哈希（见 api authManager），内部统一存哈希、永不存明文。
func hashPassword(password string) string {
	if password == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

// HashPassword 导出密码哈希计算：供启动解析共享配置时（cmd/filespace）将
// 明文默认密码直接转为哈希填入，保证明文只出现在输入入口。
func HashPassword(password string) string {
	return hashPassword(password)
}

// ApplyDefaultPasswdHash 用默认密码填充未显式设置密码（passwd 与 passwd_hash 均为空）
// 的共享目录：直接写入密码哈希（与持久化表示一致），
// 不覆盖配置文件中为单个文件夹指定的独立密码（含其哈希表示）。
func ApplyDefaultPasswdHash(shared []config.SharedFolder, passwd string) {
	if passwd == "" {
		return
	}
	hash := hashPassword(passwd)
	for i := range shared {
		if shared[i].Passwd == "" && shared[i].PasswdHash == "" {
			shared[i].PasswdHash = hash
		}
	}
}

// folderPassHash 取共享配置的密码哈希：PasswdHash 优先（持久化/内部表示），
// 否则对明文 Passwd（配置文件中的历史输入）现场哈希，不回存明文。
func folderPassHash(sf config.SharedFolder) string {
	if sf.PasswdHash != "" {
		return sf.PasswdHash
	}
	return hashPassword(sf.Passwd)
}

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

// Manager 管理本节点共享的目录（支持运行中追加）。
// 目录统计（文件数 / 总大小 / 最近更新）采用「缓存 + 后台异步扫描」：
// 缓存是否过期由文件系统变更事件驱动（见 dirWatcher）——磁盘上任意文件/目录
// 变化（含原地改写内容、任意深度增删改）都会触发一次后台重扫，无变化时零开销；
// List 只读缓存，永不因全量遍历阻塞（本机管理页 / 节点发现 / 页面轮询都走这里）。
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
			PasswdHash: folderPassHash(sf),
			RealPath:   realPath(sf.Path),
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
	real := realPath(abs)
	m.mu.Lock()
	for _, f := range m.folders {
		if f.Path == abs || (f.RealPath != "" && f.RealPath == real) {
			m.mu.Unlock()
			return f, ErrFolderExists
		}
	}
	f := Folder{ID: folderID(abs), Name: filepath.Base(abs), Path: abs, PasswdHash: hashPassword(password), RealPath: real}
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

// HasPassword 是否存在设置了访问密码的共享文件夹。
func (m *Manager) HasPassword() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if m.folders[i].PasswdHash != "" {
			return true
		}
	}
	return false
}

// MatchPassword 判断密码是否匹配任一设置了访问密码的共享文件夹（常量时间比较，不暴露明文）。
func (m *Manager) MatchPassword(password string) bool {
	hash := hashPassword(password)
	if hash == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if m.folders[i].PasswdHash == "" {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(m.folders[i].PasswdHash), []byte(hash)) == 1 {
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
			PasswdHash: m.folders[i].PasswdHash,
		})
	}
	return out
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
			Auth:      f.PasswdHash != "",
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

// FolderPasswdHash 加锁读取共享目录访问密码的哈希（供认证校验使用，避免与运行中修改竞态）。
func (m *Manager) FolderPasswdHash(id string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.folders {
		if m.folders[i].ID == id {
			return m.folders[i].PasswdHash, true
		}
	}
	return "", false
}

// SetPassword 修改共享目录的访问密码（password 为空表示移除密码、恢复开放），
// 仅存储其 sha256 哈希，明文不驻留内存也不落盘。
// 按路径匹配：精确路径或符号链接解析后的真实路径相同即命中；未共享返回 ErrFolderNotFound。
func (m *Manager) SetPassword(path, password string) (Folder, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Folder{}, err
	}
	real := realPath(abs)
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.folders {
		if m.folders[i].Path == abs || (m.folders[i].RealPath != "" && m.folders[i].RealPath == real) {
			m.folders[i].PasswdHash = hashPassword(password)
			return m.folders[i], nil
		}
	}
	return Folder{}, ErrFolderNotFound
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
