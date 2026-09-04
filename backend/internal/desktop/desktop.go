// Package desktop 提供「用系统默认应用打开本地目标」的跨平台能力：
// xdg-open（Linux）/ open（macOS）/ cmd start（Windows）对文件路径与 URL 通用，
// 故 api（本机打开共享文件）与 cmd（--web 打开浏览器）共用同一平台分发，
// 避免两处各自维护一份几乎相同的命令 switch。
package desktop

import (
	"os/exec"
	"runtime"
)

// Open 用系统默认应用打开 target（本地文件绝对路径或 URL），
// 异步启动子进程（open/xdg-open/cmd start）后立即返回，不等待目标应用退出。
// 返回启动错误（目标不存在、无可用默认应用等；启动成功后的行为由系统决定）。
func Open(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", target)
	default: // linux 等
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}
