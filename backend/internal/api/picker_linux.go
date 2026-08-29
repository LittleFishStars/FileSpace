//go:build linux

package api

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// pickDirectoryDialog 在 Linux 上调用桌面环境的原生目录选择对话框
// （按优先级探测 zenity / kdialog / qarma / yad / Xdialog），返回用户选择的绝对路径；
// 用户取消时返回空字符串与 nil 错误。
func pickDirectoryDialog() (string, error) {
	home, _ := os.UserHomeDir()
	candidates := []struct {
		bin  string
		args []string
	}{
		{"zenity", []string{"--file-selection", "--directory", "--title=选择要共享的文件夹", "--filename=" + home + "/"}},
		{"kdialog", []string{"--getexistingdirectory", home}},
		{"qarma", []string{"--file-selection", "--directory", "--title=选择要共享的文件夹", "--filename=" + home + "/"}},
		{"yad", []string{"--file", "--directory", "--title=选择要共享的文件夹", "--filename=" + home + "/"}},
		{"Xdialog", []string{"--title", "选择要共享的文件夹", "--dselect", home + "/", "0", "0"}},
	}
	var lastErr error
	for _, c := range candidates {
		if _, err := exec.LookPath(c.bin); err != nil {
			continue // 未安装，尝试下一个工具
		}
		out, err := exec.Command(c.bin, c.args...).Output()
		if err != nil {
			// 取消时（退出码非 0）stdout 为空；工具报错则记录并继续尝试下一个
			if len(bytes.TrimSpace(out)) == 0 {
				return "", nil
			}
			lastErr = fmt.Errorf("%s: %w", c.bin, err)
			continue
		}
		p := strings.TrimSpace(string(out))
		if p == "" {
			return "", nil
		}
		return p, nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("未找到可用的目录选择工具（zenity / kdialog / qarma / yad / Xdialog），请安装其一后重试")
}
