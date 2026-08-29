package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"filespace/internal/state"
)

// backendWaitTimeout 等待后端就绪的最长时间（spawn 的后端需完成锁文件写入与端口监听）。
const backendWaitTimeout = 15 * time.Second

// ensureBackend 定位后端并返回其端口；若后端未运行（锁文件缺失 / 端口无响应），
// 则自动拉起一个后端进程并等待其就绪。
//
// 返回值：
//   - port：后端监听端口（HTTP API 反代目标）
//   - spawned：非 nil 表示后端由本进程拉起（退出时应通知其结束）；nil 表示复用已有后端
//   - err：后端不可用且无法启动时返回错误
func ensureBackend() (int, *os.Process, error) {
	// 1) 锁文件优先：存在且端口存活 → 复用
	if p, ok := state.ReadLockPort(); ok && state.BackendAlive(p) {
		return p, nil, nil
	}

	// 2) 后端未运行：找一个空闲端口，拉起后端
	port, err := freePort()
	if err != nil {
		return 0, nil, fmt.Errorf("寻找空闲端口失败: %v", err)
	}
	exe, err := backendExecutable()
	if err != nil {
		return 0, nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, nil, err
	}
	cmd := exec.Command(exe, "-p", strconv.Itoa(port))
	cmd.Dir = cwd // 与直接执行 filespace 一致：默认共享当前目录
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf("启动后端进程 %s 失败: %v", exe, err)
	}

	// 3) 轮询等待后端就绪（正常情况后端会写入锁文件并开始监听）
	deadline := time.Now().Add(backendWaitTimeout)
	for time.Now().Before(deadline) {
		if state.BackendAlive(port) {
			return port, cmd.Process, nil
		}
		// 竞态兜底：若其他进程（如另一个 filespace-web）先拿到了锁，复用其端口
		if p, ok := state.ReadLockPort(); ok && p != port && state.BackendAlive(p) {
			return p, cmd.Process, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return 0, nil, fmt.Errorf("等待后端启动超时（端口 %d，%s），请检查后端日志", port, exe)
}

// backendExecutable 定位后端二进制：优先与 filespace-web 同目录的 filespace，
// 其次 PATH 中的 filespace（安装包布局：filespace-web 与 filespace 同级分发）。
func backendExecutable() (string, error) {
	name := "filespace"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("找不到后端程序 %s（应位于 %s 同级目录或 PATH 中）", name, exeDir())
}

// exeDir 返回本进程所在目录（仅用于报错提示）。
func exeDir() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// freePort 临时占用一个空闲端口后释放，返回端口号（存在微小竞态，可接受）。
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
