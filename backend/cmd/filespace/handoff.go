package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"filespace/internal/state"
)

// handoffToExisting 已有后端在运行（运行锁被持有）：仅允许用目录参数或 -a 追加，
// 把目录交给它后退出；无追加内容时报错退出。
func handoffToExisting(port int, opts *options, args []string) {
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
//   - 返回 state.ErrBackendRunning：端口上是 filespace 后端
//   - 返回其他错误：端口被占用但并非 filespace
//   - 返回 nil：端口空闲
func probeBackend(port int) error {
	client := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/node", port))
	if err != nil {
		return nil // 连接失败视为无后端；若端口被占用，后续 ListenAndServe 会兜底报错
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("端口 %d 上有服务响应但非 filespace（HTTP %d）", port, resp.StatusCode)
	}
	var info struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.ID == "" {
		return fmt.Errorf("端口 %d 上的服务响应不符合 filespace 格式", port)
	}
	return state.ErrBackendRunning
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
