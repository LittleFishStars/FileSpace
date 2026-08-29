package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

// webRoot 前端静态资源目录（web/），初始化时定位一次。
var webRoot string

// runWeb 启动前端 HTTP 服务：web/ 静态资源（SPA 回退 index.html）+
// /api/* 反向代理到后端端口；等待退出信号后优雅关闭并通知后端。
func runWeb(ln net.Listener, listenPort, backendPort int, spawned *os.Process) {
	webRoot = locateWebRoot()
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("127.0.0.1:%d", backendPort),
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/api", proxy)
	mux.HandleFunc("/", serveStatic)

	srv := &http.Server{Handler: mux}
	go func() {
		fmt.Printf("🌐 文件空间界面已启动: http://localhost:%d（后端 API 端口 %d）\n", listenPort, backendPort)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "前端服务启动失败: %v\n", err)
		}
	}()

	waitAndShutdown(srv, spawned)
}

// serveStatic 托管前端静态资源，未命中的路径回退到 index.html（SPA）。
func serveStatic(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(webRoot, filepath.Clean("/"+r.URL.Path))
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
}

// locateWebRoot 定位前端静态资源目录，优先级：
//  1. 二进制同级目录下的 web/（随包分发 / AppImage 布局）
//  2. 当前工作目录下的 web/（开发模式）
//  3. 系统安装布局 /usr/share/filespace/web（deb / pacman 包）
func locateWebRoot() string {
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

// waitAndShutdown 等待退出信号，通知后端（若由本进程拉起）退出并优雅关闭前端服务。
func waitAndShutdown(srv *http.Server, spawned *os.Process) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	<-quit
	fmt.Println("\n正在退出...")
	if spawned != nil {
		// SIGTERM 让后端优雅退出（记录共享目录并删除锁文件）；Windows 仅支持强制结束
		if runtime.GOOS == "windows" {
			_ = spawned.Kill()
		} else {
			_ = spawned.Signal(syscall.SIGTERM)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
