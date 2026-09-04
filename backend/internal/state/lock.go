// Package state 负责本地运行锁的读写（锁文件存放于用户配置目录，标识已有后端在运行）。
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filespace/internal/config"
)

// ErrBackendRunning 标记已有 filespace 后端在运行（锁文件存在且端口存活）。
var ErrBackendRunning = errors.New("已有 filespace 后端在运行")

// lockFile 运行锁文件名：内容为运行中后端的端口。
const lockFile = "lock"

// RunningLock 一次持有的运行锁。
type RunningLock struct {
	path string
}

// AcquireRunningLock 用锁文件标识是否已有后端在运行：
//   - 锁文件存在且内容端口有活的 filespace 后端 → 返回 (nil, 端口, nil)，调用方移交后退出
//   - 锁文件存在但端口无响应（崩溃残留）→ 删除残留锁文件后重建
//   - 无锁文件 → 创建并写入本进程端口（退出时调用 Release 删除）
func AcquireRunningLock(port int) (*RunningLock, int, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return nil, 0, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, 0, err
	}
	path := filepath.Join(dir, lockFile)
	// 锁文件存在：读取内容端口，探测是否存活
	if _, err := os.Stat(path); err == nil {
		if p := readLockPort(path); p > 0 && BackendAlive(p) {
			return nil, p, nil
		}
		// 崩溃残留（端口无响应或内容无效）：删除后重建
		_ = os.Remove(path)
	}
	// 创建锁文件并写入本进程端口
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, 0, err
	}
	_, err = fmt.Fprintf(f, "%d\n", port)
	// 锁文件内容决定后续进程能否识别「已有后端在运行」，写入与关闭的错误都需上报
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, 0, err
	}
	return &RunningLock{path: path}, 0, nil
}

// Release 删除锁文件，释放"已有进程"标识。
func (l *RunningLock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	return os.Remove(l.path)
}

// BackendAlive 探测端口上是否运行着活的 filespace 后端：
// GET /api/node 成功且返回节点 ID 即认为存活（与 cmd 层移交探测同口径）。
func BackendAlive(port int) bool {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/node", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var info struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.ID == "" {
		return false
	}
	return true
}

// readLockPort 读取锁文件中的端口；容忍创建后写入的短暂窗口（重试），读不到返回 0。
func readLockPort(path string) int {
	for i := 0; i < 10; i++ {
		data, err := os.ReadFile(path)
		if err == nil {
			var p int
			if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &p); err == nil && p > 0 {
				return p
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return 0
}
