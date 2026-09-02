package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
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
	nodeID  string
	mon     *monitor.Monitor
	folders *share.Manager
	peers   *discovery.Cache
	httpSrv *http.Server
	cancel  context.CancelFunc
}

// runServer 启动纯后端 API 服务（无前端），等待退出信号后优雅关闭。
func runServer(cfg *config.Config) {
	a := &app{cfg: cfg}
	a.build()
	a.buildHTTPServer(nil) // nil → 纯 API，不托管静态文件
	a.startHTTP()
	a.startDiscovery()
	a.waitAndShutdown()
}

// build 组装各组件（监控、共享管理器、发现缓存）。
func (a *app) build() {
	a.mon = monitor.New()
	a.nodeID = config.NodeID(a.mon.Hostname())
	a.folders = share.NewManager(a.cfg.Shared)
	// 启动时后台预扫共享目录填充统计缓存（不阻塞启动，列表接口首次请求即可拿到统计）
	a.folders.WarmUp()
	a.peers = discovery.NewCache(a.nodeID)
}

// buildHTTPServer 创建 HTTP 服务。
// staticFS 不为 nil 时，组合 API 路由与静态文件服务器（--web 模式）；
// 为 nil 时，仅提供纯 API 路由。
func (a *app) buildHTTPServer(staticFS http.FileSystem) {
	srv := api.NewServer(api.Options{
		Config:  a.cfg,
		NodeID:  a.nodeID,
		Version: filespace.Version,
		Folders: a.folders,
		Monitor: a.mon,
		Peers:   a.peers,
	})
	var handler http.Handler
	if staticFS != nil {
		handler = srv.HandlerWithStatic(staticFS)
	} else {
		handler = srv.Handler()
	}
	a.httpSrv = &http.Server{Addr: fmt.Sprintf(":%d", a.cfg.ListenPort), Handler: handler}
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
	fmt.Printf("🌐 后端 API 已启动: http://%s:%d\n", a.mon.IP(), a.cfg.ListenPort)
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
	if err := discovery.Register(ctx, a.cfg.Discovery.ServiceName, a.cfg.Discovery.Domain, a.nodeID, a.cfg.ListenPort, txt); err != nil {
		log.Printf("mDNS 注册失败: %v", err)
	}
	go discovery.Watch(ctx, a.cfg.Discovery.ServiceName, a.cfg.Discovery.Domain, a.peers, 3*time.Second)
}

// waitAndShutdown 等待退出信号（Ctrl+C / kill / 终端关闭），记录共享目录后优雅关闭。
// 关闭顺序：先通知其他节点本节点已退出（让在线列表立即更新），
// 再取消 mDNS 注册（发送 goodbye 包）并关闭 HTTP 服务。
func (a *app) waitAndShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	<-quit
	fmt.Println("\n正在退出...")
	saveLastShared(a.folders)
	// 停止目录变更监听（释放 fsnotify 资源），统计缓存的扫描 goroutine 随进程退出自然结束
	a.folders.Close()
	// 通知其他节点本节点已退出：它们收到后立即把本节点从在线列表移除，
	// 无需等待离线超时（60s）。
	notifyCtx, notifyCancel := context.WithTimeout(context.Background(), 2*time.Second)
	discovery.NotifyExit(notifyCtx, a.peers, a.nodeID, 1*time.Second)
	notifyCancel()
	a.cancel()
	shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
	defer done()
	_ = a.httpSrv.Shutdown(shutdownCtx)
}

// saveLastShared 记录本次共享的目录（含运行中追加/设置的访问密码），
// 供下次未指定目录时恢复——重启后文件夹的密码不会丢失。
func saveLastShared(folders *share.Manager) {
	shared := folders.SharedSnapshot()
	if err := state.SaveLastShared(shared); err != nil {
		log.Printf("记录共享目录失败: %v", err)
		return
	}
	paths := make([]string, 0, len(shared))
	for _, f := range shared {
		paths = append(paths, f.Path)
	}
	fmt.Printf("已记录本次共享的目录: %s\n", strings.Join(paths, ", "))
}
