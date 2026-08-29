package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"filespace/internal/state"
)

// handoffToExisting 已有后端在运行（运行锁被持有）：仅允许用目录参数或 -a 追加，
// 把目录交给它后退出；无追加内容时报错退出。
func handoffToExisting(port int, opts *options, args []string) {
	if opts.passwd != "" {
		log.Printf("警告: 已有后端在运行，-P/--passwd 仅在首次启动时生效，无法修改已运行后端的访问密码")
	}
	paths := appendPaths(args, opts.addCwd)
	if len(paths) == 0 {
		log.Fatalf("检测到已有 filespace 后端在运行（端口 %d），仅支持使用目录参数或 -a 追加共享目录（--this 等独占模式不适用）", port)
	}
	if err := sendAddFolders(port, paths); err != nil {
		log.Fatalf("向已有后端追加共享目录失败: %v", err)
	}
	fmt.Printf("已将 %d 个目录交给已运行的后端（端口 %d）共享: %s\n", len(paths), port, strings.Join(paths, ", "))
}

// appendPaths 汇总本次要追加的目录：目录参数 +（-a 时的）当前目录。
func appendPaths(args []string, addCwd bool) []string {
	paths := make([]string, 0, len(args)+1)
	paths = append(paths, args...)
	if addCwd {
		cwd, _ := os.Getwd()
		paths = append(paths, cwd)
	}
	return paths
}

// probeBackend 探测端口上是否已有 filespace 后端在运行：
//   - 返回 state.ErrBackendRunning：端口上是 filespace 后端（复用 state.BackendAlive 的探测逻辑）
//   - 返回其他错误：端口被占用但并非 filespace
//   - 返回 nil：端口空闲
func probeBackend(port int) error {
	if state.BackendAlive(port) {
		return state.ErrBackendRunning
	}
	// 非 filespace 但被占用：TCP 能连上说明有其他服务在监听
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 800*time.Millisecond)
	if err != nil {
		return nil // 连接失败视为无后端；若端口被占用，后续 ListenAndServe 会兜底报错
	}
	conn.Close()
	return fmt.Errorf("端口 %d 上有服务响应但非 filespace", port)
}

// sendAddFolders 把要追加的目录列表发给已运行的后端。
func sendAddFolders(port int, paths []string) error {
	body, err := json.Marshal(map[string]any{"paths": paths})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/api/folders/add", port), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, e.Error)
	}
	return nil
}
