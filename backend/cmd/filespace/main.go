// filespace 命令行入口：在某个文件夹下执行即可共享该文件夹。
package main

import (
	"context"
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
  -a, --add               直接共享当前文件夹（不恢复上次共享的目录）
  -c, --config <文件>     配置文件路径（YAML）
  -p, --port <端口>       监听端口（默认 8080）
  -h, --help              显示本帮助信息

命令:
  help                    显示本帮助信息

示例:
  filespace                        恢复上次共享的目录（无记录时共享当前目录）
  filespace -a                     直接共享当前目录
  filespace ~/docs /mnt/data       共享多个目录
  filespace -p 9000 ~/docs         指定端口
  filespace -c config.yaml         使用配置文件

配置优先级: 命令行 -p > 配置文件 > 默认值; 目录参数覆盖配置文件中的 shared_folders; 两者都未指定时恢复上次退出前共享的目录，-a 可改为直接共享当前目录。
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
	flag.BoolVar(&addCwd, "add", false, "直接共享当前文件夹")
	flag.BoolVar(&addCwd, "a", false, "直接共享当前文件夹（-a, --add 简写）")
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
	// 共享目录解析：目录参数 > 配置文件 shared_folders > 上次共享记录 > 当前目录；
	// -a/--add 表示直接共享当前目录，与目录参数 / 配置中的 shared_folders 互斥
	if addCwd {
		if len(args) > 0 {
			log.Fatalf("不能同时使用 -a/--add 与目录参数")
		}
		if len(cfg.Shared) > 0 {
			log.Fatalf("不能同时使用 -a/--add 与配置文件中的 shared_folders")
		}
	}
	if len(args) > 0 {
		cfg.Shared = make([]filespace.SharedFolder, 0, len(args))
		for _, p := range args {
			cfg.Shared = append(cfg.Shared, filespace.SharedFolder{Path: p, Name: filepath.Base(p)})
		}
	}
	if len(cfg.Shared) == 0 {
		if addCwd {
			// -a：直接共享当前目录，不恢复上次共享的目录
			cwd, _ := os.Getwd()
			cfg.Shared = []filespace.SharedFolder{{Path: cwd, Name: filepath.Base(cwd)}}
			fmt.Printf("已指定 -a，直接共享当前目录: %s\n", cwd)
		} else if last := filespace.LoadLastShared(); len(last) > 0 {
			// 未指定共享目录：优先恢复上次退出前共享的目录
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
	// 记录本次共享的目录，供下次未指定目录时恢复
	paths := make([]string, 0, len(cfg.Shared))
	for _, f := range cfg.Shared {
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
