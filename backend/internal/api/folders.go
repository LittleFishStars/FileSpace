package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"filespace/internal/model"
	"filespace/internal/share"
)

// handleFolders 返回本节点共享的文件夹列表。
func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.folders.List())
}

// addFoldersRequest 追加共享目录的请求体。
type addFoldersRequest struct {
	Paths []string `json:"paths"`
}

// handleAddFolders 追加共享目录（本机另一个 filespace 进程探测到本后端后，把目录交过来）。
func (s *Server) handleAddFolders(w http.ResponseWriter, r *http.Request) {
	var req addFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "paths 不能为空")
		return
	}
	var added []model.FolderInfo
	for _, p := range req.Paths {
		f, err := s.folders.Add(p)
		if errors.Is(err, share.ErrFolderExists) {
			continue // 已在共享列表中，视为成功
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "追加失败: "+err.Error())
			return
		}
		added = append(added, model.FolderInfo{ID: f.ID, Name: f.Name, Path: f.Path})
	}
	writeJSON(w, map[string]any{"added": added})
}

// handleTree 懒加载返回共享目录内的文件列表（?path=相对路径）。
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rel := r.URL.Query().Get("path")
	entries, err := s.folders.Tree(id, rel)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	writeJSON(w, entries)
}

// writeFolderError 把 share 包的错误映射为 HTTP 状态码。
func writeFolderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, share.ErrFolderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, share.ErrPathForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}
