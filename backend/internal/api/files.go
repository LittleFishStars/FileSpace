package api

import (
	"net/http"
)

// handleDownload 下载共享目录内的文件（?path=相对路径，支持 Range 断点续传）。
// 该文件夹设置了访问密码时，远程请求需携带绑定该文件夹密码的有效访问令牌（本机回环放行）。
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorized(r, id) {
		writeError(w, http.StatusUnauthorized, "需要访问密码（未认证或令牌已过期）")
		return
	}
	rel := r.URL.Query().Get("path")
	full, err := s.folders.ResolveFile(id, rel)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	// http.ServeFile 自动处理 Range、Content-Disposition 等。
	http.ServeFile(w, r, full)
}
