// filespace 命令行入口：在某个文件夹下执行即可共享该文件夹。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"filespace"
	"filespace/internal/api"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
)

// webRoot 定位前端静态资源，优先级：
//  1. 二进制同级目录下的 web/（随包分发 / AppImage 布局）
//  2. 当前工作目录下的 web/（开发模式）
//  3. 系统安装布局 /usr/share/filespace/web（deb / pacman 包）
func webRoot() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if fi, err := os.Stat(filepath.Join(dir, "web")); err == nil && fi.IsDir() {
			return filepath.Join(dir, "web")
		}
	}
	if fi, err := os.Stat("./web"); err == nil && fi.IsDir() {
		return "./web"
	}
	const sysWeb = "/usr/share/filespace/web"
	if fi, err := os.Stat(sysWeb); err == nil && fi.IsDir() {
		return sysWeb
	}
	return "./web"
}

// usage 打印命令行帮助信息（help 命令与 -h 共用）。
func usage() {
	fmt.Print(`filespace — 局域网文件共享工具

用法:
  filespace [选项] [目录...]

在任意文件夹执行 filespace 即可共享该文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

参数:
  [目录...]               要共享的文件夹（可多个）；缺省恢复上次退出前共享的目录，无记录时共享当前目录
  -a, --add               额外共享当前文件夹（在解析出的共享目录之外追加）
  --this                  仅共享当前目录（不恢复上次共享的目录）
  -c, --config <文件>     配置文件路径（YAML）
  -p, --port <端口>       监听端口（默认 8080）
  -h, --help              显示本帮助信息

命令:
  help                    显示本帮助信息

示例:
  filespace                        恢复上次共享的目录（无记录时共享当前目录）
  filespace -a                     恢复上次共享的目录，并额外共享当前目录
  filespace --this                 仅共享当前目录
  filespace ~/docs /mnt/data       共享多个目录
  filespace -p 9000 ~/docs         指定端口
  filespace -c config.yaml         使用配置文件

配置优先级: 命令行 -p > 配置文件 > 默认值; 目录参数覆盖配置文件中的 shared_folders; 两者都未指定时恢复上次退出前共享的目录，-a 可在任何情况下额外共享当前目录，--this 可仅共享当前目录。
检测到同端口已有后端在运行时，本进程仅支持用目录参数或 -a 追加，把目录交给已运行的后端后自动退出。
`)
}

func main() {
	flag.Usage = usage
	var configPath string
	var port int
	var showHelp bool
	// 长名与短名别名指向同一变量，-x 与 --x 由 flag 包等价解析
	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.StringVar(&configPath, "c", "", "配置文件路径（-c, --config 简写）")
	flag.IntVar(&port, "port", 0, "监听端口（默认 8080）")
	flag.IntVar(&port, "p", 0, "监听端口（-p, --port 简写）")
	flag.BoolVar(&showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&showHelp, "h", false, "显示帮助信息（-h, --help 简写）")
	var addCwd bool
	flag.BoolVar(&addCwd, "add", false, "额外共享当前文件夹")
	flag.BoolVar(&addCwd, "a", false, "额外共享当前文件夹（-a, --add 简写）")
	var thisOnly bool
	flag.BoolVar(&thisOnly, "this", false, "仅共享当前目录")
	flag.Parse()
	args := flag.Args()

	// -h / --help 或 help 命令：显示帮助信息后退出
	if showHelp || (len(args) > 0 && args[0] == "help") {
		usage()
		return
	}

	// 配置优先级：命令行路径 > 配置文件 > 默认（共享当前目录）
	cfg := filespace.DefaultConfig()
	if configPath != "" {
		loaded, err := filespace.Load(configPath)
		if err != nil {
			log.Fatalf("读取配置失败: %v", err)
		}
		cfg = loaded
	}
	if port != 0 {
		cfg.ListenPort = port
	}

	// 探测是否已有 filespace 后端在运行：若有，仅支持用目录参数或 -a 追加共享目录，
	// 把目录交给已有后端后自身退出，避免端口冲突
	if err := probeBackend(cfg.ListenPort); err != nil {
		switch {
		case errors.Is(err, errBackendRunning):
			paths := make([]string, 0, len(args)+1)
			paths = append(paths, args...)
			if addCwd {
				cwd, _ := os.Getwd()
				paths = append(paths, cwd)
			}
			if len(paths) == 0 {
				log.Fatalf("检测到已有 filespace 后端在运行（端口 %d），仅支持使用目录参数或 -a 追加共享目录（--this 等独占模式不适用）", cfg.ListenPort)
			}
			if err := sendAddFolders(cfg.ListenPort, paths); err != nil {
				log.Fatalf("向已有后端追加共享目录失败: %v", err)
			}
			fmt.Printf("已将 %d 个目录交给已运行的后端（端口 %d）共享: %s\n", len(paths), cfg.ListenPort, strings.Join(paths, ", "))
			return
		default:
			log.Fatalf("端口 %d 不可用: %v", cfg.ListenPort, err)
		}
	}

	// 共享目录解析：目录参数 > 配置文件 shared_folders > 上次共享记录 > 当前目录；
	// --this 仅共享当前目录（跳过恢复），与目录参数 / 配置中的 shared_folders 互斥
	if thisOnly {
		if len(args) > 0 {
			log.Fatalf("不能同时使用 --this 与目录参数")
		}
		if len(cfg.Shared) > 0 {
			log.Fatalf("不能同时使用 --this 与配置文件中的 shared_folders")
		}
		cwd, _ := os.Getwd()
		cfg.Shared = []filespace.SharedFolder{{Path: cwd, Name: filepath.Base(cwd)}}
		fmt.Printf("已指定 --this，仅共享当前目录: %s\n", cwd)
	}
	if len(args) > 0 {
		cfg.Shared = make([]filespace.SharedFolder, 0, len(args))
		for _, p := range args {
			cfg.Shared = append(cfg.Shared, filespace.SharedFolder{Path: p, Name: filepath.Base(p)})
		}
	}
	if len(cfg.Shared) == 0 {
		// 未指定共享目录：优先恢复上次退出前共享的目录，无记录时回退到当前目录
		if last := filespace.LoadLastShared(); len(last) > 0 {
			cfg.Shared = make([]filespace.SharedFolder, 0, len(last))
			for _, p := range last {
				cfg.Shared = append(cfg.Shared, filespace.SharedFolder{Path: p, Name: filepath.Base(p)})
			}
			fmt.Printf("未指定共享目录，恢复上次共享的目录: %s\n", strings.Join(last, ", "))
		} else {
			cwd, _ := os.Getwd()
			cfg.Shared = []filespace.SharedFolder{{Path: cwd, Name: filepath.Base(cwd)}}
			fmt.Printf("未指定共享目录，默认共享当前目录: %s\n", cwd)
		}
	}
	// -a/--add：在解析结果基础上额外共享当前目录（已在列表中则跳过）
	if addCwd {
		cwd, _ := os.Getwd()
		if !sharedContains(cfg.Shared, cwd) {
			cfg.Shared = append(cfg.Shared, filespace.SharedFolder{Path: cwd, Name: filepath.Base(cwd)})
			fmt.Printf("已指定 -a，额外共享当前目录: %s\n", cwd)
		} else {
			fmt.Printf("已指定 -a，当前目录已在共享列表中: %s\n", cwd)
		}
	}

	// 组件
	mon := monitor.New()
	nodeID := filespace.NodeID(mon.Hostname())
	folders := share.NewManager(cfg.Shared)
	cache := discovery.NewCache(nodeID)

	// HTTP 服务（API + 前端静态资源）
	srv := api.NewServer(api.Options{
		Config:  cfg,
		NodeID:  nodeID,
		Version: filespace.Version,
		Folders: folders,
		Monitor: mon,
		Peers:   cache,
		WebRoot: webRoot(),
	})
	httpSrv := &http.Server{Addr: fmt.Sprintf(":%d", cfg.ListenPort), Handler: srv.Handler()}

	// mDNS 注册与发现
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	txt := map[string]string{"id": nodeID, "hostname": mon.Hostname(), "version": filespace.Version}
	if _, err := discovery.Register(ctx, cfg.Discovery.ServiceName, cfg.Discovery.Domain, nodeID, cfg.ListenPort, txt); err != nil {
		log.Printf("mDNS 注册失败: %v", err)
	}
	go discovery.Watch(ctx, cfg.Discovery.ServiceName, cfg.Discovery.Domain, cache, 3*time.Second)

	// 启动 HTTP 服务
	go func() {
		fmt.Printf("🌐 服务已启动: http://%s:%d（前端目录 %s）\n", mon.IP(), cfg.ListenPort, webRoot())
		fmt.Printf("📂 共享 %d 个目录:\n", len(cfg.Shared))
		for _, f := range cfg.Shared {
			fmt.Printf("   - %s（%s）\n", f.Path, f.Name)
		}
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务启动失败: %v", err)
		}
	}()

	// 优雅退出（Ctrl+C / kill / 终端关闭）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	<-quit
	fmt.Println("\n正在退出...")
	// 记录本次共享的目录（含运行中追加的），供下次未指定目录时恢复
	infos := folders.List()
	paths := make([]string, 0, len(infos))
	for _, f := range infos {
		paths = append(paths, f.Path)
	}
	if err := filespace.SaveLastShared(paths); err != nil {
		log.Printf("记录共享目录失败: %v", err)
	} else {
		fmt.Printf("已记录本次共享的目录: %s\n", strings.Join(paths, ", "))
	}
	cancel()
	shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	_ = httpSrv.Shutdown(shutdownCtx)
}

// sharedContains 判断共享列表中是否已包含指定路径。
func sharedContains(shared []filespace.SharedFolder, path string) bool {
	for _, f := range shared {
		if f.Path == path {
			return true
		}
	}
	return false
}

// errBackendRunning 标记端口上已探测到 filespace 后端在运行。
var errBackendRunning = errors.New("已有 filespace 后端在运行")

// probeBackend 探测端口上是否已有 filespace 后端在运行：
//   - 返回 errBackendRunning：端口上是 filespace 后端
//   - 返回其他错误：端口被占用但并非 filespace
//   - 返回 nil：端口空闲（无后端），可正常启动
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
	return errBackendRunning
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
