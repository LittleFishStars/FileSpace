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
	"syscall"
	"time"

	"filespace"
	"filespace/internal/api"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
)

// webRoot 定位前端静态资源，优先级：
//  1. 二进制同级目录下的 web/
//  2. 当前工作目录下的 web/
func webRoot() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if fi, err := os.Stat(filepath.Join(dir, "web")); err == nil && fi.IsDir() {
			return filepath.Join(dir, "web")
		}
	}
	return "./web"
}

func main() {
	var configPath string
	var port int
	flag.StringVar(&configPath, "config", "", "配置文件路径")
	flag.IntVar(&port, "port", 0, "监听端口（默认 8080）")
	flag.Parse()
	args := flag.Args()

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
	if len(args) > 0 {
		cfg.Shared = make([]filespace.SharedFolder, 0, len(args))
		for _, p := range args {
			cfg.Shared = append(cfg.Shared, filespace.SharedFolder{Path: p, Name: filepath.Base(p)})
		}
	}
	if len(cfg.Shared) == 0 {
		cwd, _ := os.Getwd()
		cfg.Shared = []filespace.SharedFolder{{Path: cwd, Name: filepath.Base(cwd)}}
		fmt.Printf("未指定共享目录，默认共享当前目录: %s\n", cwd)
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

	// 优雅退出（Ctrl+C）
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	fmt.Println("\n正在退出...")
	cancel()
	shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	_ = httpSrv.Shutdown(shutdownCtx)
}
