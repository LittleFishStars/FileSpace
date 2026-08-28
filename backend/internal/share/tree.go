package share

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filespace/internal/model"
)

// Tree 返回共享目录内相对路径 rel 下的文件列表（懒加载，仅一层）。
func (m *Manager) Tree(id, rel string) ([]model.FileInfo, error) {
	folder, ok := m.Resolve(id)
	if !ok {
		return nil, ErrFolderNotFound
	}
	full, err := resolvePath(folder.Path, rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err
	}
	files := make([]model.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, model.FileInfo{
			Name:    e.Name(),
			Path:    filepath.ToSlash(filepath.Join(rel, e.Name())),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
			IsDir:   e.IsDir(),
		})
	}
	// 目录在前，名称升序
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return files[i].Name < files[j].Name
	})
	return files, nil
}

// ResolveFile 把共享目录内的相对路径解析为磁盘绝对路径（供下载）。
func (m *Manager) ResolveFile(id, rel string) (string, error) {
	folder, ok := m.Resolve(id)
	if !ok {
		return "", ErrFolderNotFound
	}
	return resolvePath(folder.Path, rel)
}

// resolvePath 拼接共享目录根与相对路径，并校验不越界。
func resolvePath(root, rel string) (string, error) {
	root = filepath.Clean(root)
	rel = cleanRel(rel)
	full := filepath.Join(root, filepath.FromSlash(rel))
	if !within(root, full) {
		return "", ErrPathForbidden
	}
	return full, nil
}

// cleanRel 规范化相对路径：去首斜杠、清理 "." 与 ".."。
func cleanRel(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	return filepath.ToSlash(filepath.Clean(rel))
}

// within 判断 child 是否位于 parent 目录内。
func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
