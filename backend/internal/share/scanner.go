// Package share 负责共享目录的扫描与索引。
package share

import (
	"encoding/hex"
	"errors"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"filespace"
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
}

// Manager 管理本节点共享的目录（支持运行中追加）。
type Manager struct {
	mu      sync.RWMutex
	folders []Folder
}

// NewManager 根据配置创建共享目录管理器，为每个目录生成稳定 ID。
func NewManager(shared []filespace.SharedFolder) *Manager {
	folders := make([]Folder, 0, len(shared))
	for _, sf := range shared {
		name := sf.Name
		if name == "" {
			name = filepath.Base(sf.Path)
		}
		folders = append(folders, Folder{
			ID:   folderID(sf.Path),
			Name: name,
			Path: sf.Path,
		})
	}
	return &Manager{folders: folders}
}

// Add 运行中追加一个共享目录：校验路径存在且为目录，已在列表中则返回 ErrFolderExists。
func (m *Manager) Add(path string) (Folder, error) {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, f := range m.folders {
		if f.Path == abs {
			return f, ErrFolderExists
		}
	}
	f := Folder{ID: folderID(abs), Name: filepath.Base(abs), Path: abs}
	m.folders = append(m.folders, f)
	return f, nil
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
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
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
	if _, err := os.Stat(root); err != nil {
		return 0, 0, time.Time{}
	}
	return count, size, updated
}
