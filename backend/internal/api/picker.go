package api

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ncruces/zenity"
)

// handlePickDirectory 在本机弹出系统原生目录选择对话框（仅允许本机回环调用），
// 返回用户所选目录的绝对路径；用户取消时返回 cancelled。
// 浏览器出于安全限制不会向网页暴露所选文件夹的绝对路径，因此由后端在运行机器上
// 弹出系统对话框并直接取得路径。
func (s *Server) handlePickDirectory(w http.ResponseWriter, r *http.Request) {
	if !requireLoopback(w, r) {
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

// pickDirectoryDialog 弹出系统原生目录选择对话框，返回用户所选目录的绝对路径；
// 用户取消时返回空字符串与 nil 错误。
// 跨平台统一委托给 ncruces/zenity（无 cgo、可交叉编译）：Windows 用原生
// 文件对话框、macOS 走 osascript、Linux 依赖已安装的 zenity / qarma /
// matedialog 之一（替代此前按平台各自手写 PowerShell / osascript / 多工具探测）。
func pickDirectoryDialog() (string, error) {
	opts := []zenity.Option{zenity.Directory(), zenity.Title("选择要共享的文件夹")}
	// 默认起始位置取用户主目录（各平台均有），避免对话框从任意目录开始
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		opts = append(opts, zenity.Filename(filepath.Join(home, string(filepath.Separator))))
	}
	path, err := zenity.SelectFile(opts...)
	if err != nil {
		if errors.Is(err, zenity.ErrCanceled) {
			return "", nil // 用户取消
		}
		return "", err
	}
	return path, nil
}
