package api

import (
	"net/http"
)

// handlePickDirectory 在本机弹出系统原生目录选择对话框（仅允许本机回环调用），
// 返回用户所选目录的绝对路径；用户取消时返回 cancelled。
// 浏览器出于安全限制不会向网页暴露所选文件夹的绝对路径，因此由后端在运行机器上
// 弹出系统对话框并直接取得路径（Linux 用 zenity/kdialog 等，Windows 用
// PowerShell FolderBrowserDialog，macOS 用 osascript choose folder）。
func (s *Server) handlePickDirectory(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, "仅允许本机调用")
		return
	}
	path, err := pickDirectoryDialog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "无法打开目录选择对话框: "+err.Error())
		return
	}
	if path == "" {
		writeJSON(w, map[string]any{"cancelled": true})
		return
	}
	writeJSON(w, map[string]any{"path": path})
}
