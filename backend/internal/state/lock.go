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
)

// ErrBackendRunning 标记已有 filespace 后端在运行（锁文件存在且端口存活）。
var ErrBackendRunning = errors.New("已有 filespace 后端在运行")

// 运行锁文件名：running-<端口>.lock，端口编码在文件名中。
const lockPrefix = "running-"

// RunningLock 一次持有的运行锁。
type RunningLock struct {
	path string
}

// AcquireRunningLock 用锁文件标识是否已有后端在运行：
//   - 锁文件存在且对应端口有活的 filespace 后端 → 返回 (nil, 端口, nil)，调用方移交后退出
//   - 锁文件存在但端口无响应（崩溃残留）→ 删除残留锁文件后重建
//   - 无锁文件 → 创建并持有（退出时调用 Release 删除）
func AcquireRunningLock(port int) (*RunningLock, int, error) {
	dir, err := dir()
	if err != nil {
		return nil, 0, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, 0, err
	}
	// 1. 扫描所有锁文件：存在且端口存活 → 已有后端；存在但端口无响应 → 崩溃残留，清理
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if !isLockName(e.Name()) {
				continue
			}
			p, err := lockPort(e.Name())
			if err != nil {
				continue
			}
			if backendAlive(p) {
				return nil, p, nil
			}
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	// 2. 创建自己端口的锁文件（内容记录端口，便于排查）
	path := filepath.Join(dir, lockName(port))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, 0, err
	}
	_, err = fmt.Fprintf(f, "%d\n", port)
	f.Close()
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

// backendAlive 探测端口上是否运行着活的 filespace 后端。
func backendAlive(port int) bool {
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

// lockName 由端口生成锁文件名。
func lockName(port int) string {
	return fmt.Sprintf("%s%d.lock", lockPrefix, port)
}

// isLockName 判断文件名是否为运行锁。
func isLockName(name string) bool {
	return strings.HasPrefix(name, lockPrefix) && strings.HasSuffix(name, ".lock")
}

// lockPort 从锁文件名解析端口。
func lockPort(name string) (int, error) {
	s := strings.TrimSuffix(strings.TrimPrefix(name, lockPrefix), ".lock")
	var p int
	if _, err := fmt.Sscanf(s, "%d", &p); err != nil || p <= 0 {
		return 0, errors.New("无效端口")
	}
	return p, nil
}
