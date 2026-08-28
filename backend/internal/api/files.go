package api

import (
	"net/http"
)

// handleDownload 下载共享目录内的文件（?path=相对路径，支持 Range 断点续传）。
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rel := r.URL.Query().Get("path")
	full, err := s.folders.ResolveFile(id, rel)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	// http.ServeFile 自动处理 Range、Content-Disposition 等。
	http.ServeFile(w, r, full)
}
