package api

import (
	"net/http"
	"os/exec"
	"runtime"
)

// handleOpenFile 用系统默认应用打开本机共享目录内的文件（?path=相对路径）。
// 这是本机专属能力（在运行后端的机器上唤起桌面应用），仅允许本机调用，
// 防止局域网内其他机器通过该接口在主机上执行打开操作。
func (s *Server) handleOpenFile(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, "仅允许本机调用")
		return
	}
	id := r.PathValue("id")
	rel := r.URL.Query().Get("path")
	full, err := s.folders.ResolveFile(id, rel)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	if err := openWithDefaultApp(full); err != nil {
		writeError(w, http.StatusInternalServerError, "打开失败: "+err.Error())
		return
	}
	writeJSON(w, map[string]any{"opened": full})
}

// openWithDefaultApp 调用系统默认应用打开文件（异步启动，不等待应用退出）。
//   - Linux:  xdg-open
//   - macOS:  open
//   - Windows: cmd /c start
func openWithDefaultApp(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
