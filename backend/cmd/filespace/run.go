package main

import (
	"context"
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
	"filespace/internal/config"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
	"filespace/internal/state"
)

// app 一次运行的组件集合。
type app struct {
	cfg     *config.Config
	webRoot string
	nodeID  string
	mon     *monitor.Monitor
	folders *share.Manager
	peers   *discovery.Cache
	httpSrv *http.Server
	cancel  context.CancelFunc
}

// runServer 启动 HTTP 服务与 mDNS 发现，等待退出信号后优雅关闭。
func runServer(cfg *config.Config) {
	a := &app{cfg: cfg, webRoot: webRoot()}
	a.build()
	a.startHTTP()
	a.startDiscovery()
	a.waitAndShutdown()
}

// build 组装各组件（监控、共享管理器、发现缓存、HTTP 服务）。
func (a *app) build() {
	a.mon = monitor.New()
	a.nodeID = config.NodeID(a.mon.Hostname())
	a.folders = share.NewManager(a.cfg.Shared)
	a.peers = discovery.NewCache(a.nodeID)
	srv := api.NewServer(api.Options{
		Config:  a.cfg,
		NodeID:  a.nodeID,
		Version: filespace.Version,
		Folders: a.folders,
		Monitor: a.mon,
		Peers:   a.peers,
		WebRoot: a.webRoot,
	})
	a.httpSrv = &http.Server{Addr: fmt.Sprintf(":%d", a.cfg.ListenPort), Handler: srv.Handler()}
}

// startHTTP 后台启动 HTTP 服务并打印启动信息。
func (a *app) startHTTP() {
	go func() {
		a.printStartup()
		if err := a.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务启动失败: %v", err)
		}
	}()
}

// printStartup 打印服务地址与共享目录列表。
func (a *app) printStartup() {
	fmt.Printf("🌐 服务已启动: http://%s:%d（前端目录 %s）\n", a.mon.IP(), a.cfg.ListenPort, a.webRoot)
	fmt.Printf("📂 共享 %d 个目录:\n", len(a.cfg.Shared))
	for _, f := range a.cfg.Shared {
		fmt.Printf("   - %s（%s）\n", f.Path, f.Name)
	}
}

// startDiscovery 注册 mDNS 服务并启动节点发现。
func (a *app) startDiscovery() {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	txt := map[string]string{"id": a.nodeID, "hostname": a.mon.Hostname(), "version": filespace.Version}
	if _, err := discovery.Register(ctx, a.cfg.Discovery.ServiceName, a.cfg.Discovery.Domain, a.nodeID, a.cfg.ListenPort, txt); err != nil {
		log.Printf("mDNS 注册失败: %v", err)
	}
	go discovery.Watch(ctx, a.cfg.Discovery.ServiceName, a.cfg.Discovery.Domain, a.peers, 3*time.Second)
}

// waitAndShutdown 等待退出信号（Ctrl+C / kill / 终端关闭），记录共享目录后优雅关闭。
func (a *app) waitAndShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	<-quit
	fmt.Println("\n正在退出...")
	saveLastShared(a.folders)
	a.cancel()
	shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	_ = a.httpSrv.Shutdown(shutdownCtx)
}

// saveLastShared 记录本次共享的目录（含运行中追加的），供下次未指定目录时恢复。
func saveLastShared(folders *share.Manager) {
	infos := folders.List()
	paths := make([]string, 0, len(infos))
	for _, f := range infos {
		paths = append(paths, f.Path)
	}
	if err := state.SaveLastShared(paths); err != nil {
		log.Printf("记录共享目录失败: %v", err)
		return
	}
	fmt.Printf("已记录本次共享的目录: %s\n", strings.Join(paths, ", "))
}

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
