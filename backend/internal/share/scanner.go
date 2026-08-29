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

// Manager 管理本节点共享的目录（支持运行中追加）。
type Manager struct {
	mu      sync.RWMutex
	folders []Folder
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
	return &Manager{folders: folders}
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
			return nil
		}
	}
	return ErrFolderNotFound
}

// List 返回共享文件夹列表（含实时统计的文件数 / 总大小 / 最近更新）。
func (m *Manager) List() []model.FolderInfo {
	m.mu.RLock()
	folders := make([]Folder, len(m.folders))
	copy(folders, m.folders)
	m.mu.RUnlock()
	result := make([]model.FolderInfo, 0, len(folders))
	for _, f := range folders {
		count, size, updated := scanFolder(f.Path)
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

// scanFolder 统计目录内文件数、总大小与最近修改时间。
func scanFolder(root string) (count int, size int64, updated time.Time) {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		count++
		if info, err := d.Info(); err == nil {
			size += info.Size()
			if info.ModTime().After(updated) {
				updated = info.ModTime()
			}
		}
		return nil
	})
	// 根目录不存在时（被删除等）返回零值，避免目录列表中出现异常数据
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, time.Time{}
	}
	return count, size, updated
}
