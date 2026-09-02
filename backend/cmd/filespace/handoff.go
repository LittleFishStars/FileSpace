package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"filespace/internal/state"
)

// handoffToExisting 已有后端在运行（运行锁被持有）时，把本次操作交给它：
//   - 无 -P：仅支持用目录参数或 -a 追加共享目录；
//   - 带 -P（含空值）：需与目录参数配合，为这些目录设置/修改/移除访问密码——
//     目录已共享则修改其密码（空值移除），未共享则作为新增共享设置密码。
func handoffToExisting(port int, opts *options, args []string) {
	paths := appendPaths(args, opts.addCwd)
	if len(paths) == 0 {
		if opts.hasPasswd {
			log.Fatalf("检测到已有 filespace 后端在运行（端口 %d）：修改/移除密码需要指定目标目录，例如 filespace <目录> -P <密码>（-P '' 移除密码）", port)
		}
		log.Fatalf("检测到已有 filespace 后端在运行（端口 %d），仅支持使用目录参数或 -a 追加共享目录（--this 等独占模式不适用）", port)
	}
	if opts.hasPasswd {
		handoffSetPasswords(port, paths, opts.passwd)
		return
	}
	if err := sendAddFolders(port, paths, ""); err != nil {
		log.Fatalf("向已有后端追加共享目录失败: %v", err)
	}
	fmt.Printf("已将 %d 个目录交给已运行的后端（端口 %d）共享: %s\n", len(paths), port, strings.Join(paths, ", "))
}

// handoffSetPasswords 为每个目标目录设置/修改/移除访问密码。
// 目录已共享 → 修改其密码（空值移除）；未共享 → 新增共享并设置密码。
func handoffSetPasswords(port int, paths []string, passwd string) {
	action := "设置"
	if passwd == "" {
		action = "移除"
	}
	ok := true
	for _, p := range paths {
		changed, err := setOrAddFolder(port, p, passwd)
		if err != nil {
			ok = false
			log.Printf("处理目录 %s 的访问密码失败: %v", p, err)
			continue
		}
		if changed {
			if passwd == "" {
				fmt.Printf("已移除共享目录 %s 的访问密码（恢复开放）\n", p)
			} else {
				fmt.Printf("已修改共享目录 %s 的访问密码\n", p)
			}
		} else if passwd == "" {
			fmt.Printf("已将目录 %s 新增为共享（未设置访问密码）\n", p)
		} else {
			fmt.Printf("已将目录 %s 新增为共享，并%s访问密码\n", p, action)
		}
	}
	if !ok {
		log.Fatalf("部分目录的密码操作失败，见上方错误")
	}
}

// setOrAddFolder 对单个目录执行「修改密码或新增共享并设密码」：
// 后端已有该目录 → 修改其密码并返回 changed=true；
// 后端没有该目录 → 新增共享（带密码）并返回 changed=false。
func setOrAddFolder(port int, path, passwd string) (bool, error) {
	err := sendSetFolderPassword(port, path, passwd)
	if err == nil {
		return true, nil // 目录已共享，密码已更新（或移除）
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.status != http.StatusNotFound {
		return false, err // 非「目录未共享」错误：直接上报
	}
	if err := sendAddFolders(port, []string{path}, passwd); err != nil {
		return false, err
	}
	return false, nil
}

// apiError HTTP API 返回的错误（状态码 + 后端 error 消息）。
type apiError struct {
	status int
	msg    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.status, e.msg)
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

// sendAddFolders 把要追加的目录列表发给已运行的后端（password 为该批目录的访问密码，空为开放）。
func sendAddFolders(port int, paths []string, password string) error {
	body, err := json.Marshal(map[string]any{"paths": paths, "password": password})
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
		return &apiError{status: resp.StatusCode, msg: e.Error}
	}
	return nil
}

// sendSetFolderPassword 请求已运行的后端修改/移除某目录的访问密码
// （password 为空表示移除）。目录未共享时后端返回 404。
func sendSetFolderPassword(port int, path, password string) error {
	body, err := json.Marshal(map[string]any{"path": path, "password": password})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/api/folders/password", port), "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return &apiError{status: resp.StatusCode, msg: e.Error}
	}
	return nil
}
