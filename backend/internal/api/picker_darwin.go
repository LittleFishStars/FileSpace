//go:build darwin

package api

import (
	"os/exec"
	"strings"
)

// pickDirectoryDialog 在 macOS 上通过 osascript 的 choose folder 弹出原生目录选择对话框，
// 返回用户选择的绝对路径；用户取消时返回空字符串与 nil 错误。
func pickDirectoryDialog() (string, error) {
	out, err := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "选择要共享的文件夹")`).Output()
	if err != nil {
		if strings.TrimSpace(string(out)) == "" {
			return "", nil // 用户取消
		}
		return "", err
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", nil
	}
	return p, nil
}
