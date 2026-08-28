package api

import (
	"errors"
	"net/http"

	"filespace/internal/share"
)

// handleFolders 返回本节点共享的文件夹列表。
func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.folders.List())
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
