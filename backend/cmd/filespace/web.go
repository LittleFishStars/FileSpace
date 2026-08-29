package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"runtime"

	"filespace/internal/config"
)

//go:embed all:web
var webAssets embed.FS

// webFS 返回以嵌入静态资源根为根的 http.FileSystem。
// go:embed 的根是 pattern 目录 "web"（路径带 "web/" 前缀），
// 需用 fs.Sub 剥掉前缀，否则 http.FileServer 找不到 index.html 等文件。
func webFS() (http.FileSystem, error) {
	sub, err := fs.Sub(webAssets, "web")
	if err != nil {
		return nil, err
	}
	return http.FS(sub), nil
}

// runServerWithWeb 启动后端 API 服务并托管前端静态资源，然后在浏览器中打开界面。
// 前端静态资源由构建脚本从 web/out/ 拷贝到 backend/cmd/filespace/web/，通过 go:embed 嵌入。
func runServerWithWeb(cfg *config.Config) {
	a := &app{cfg: cfg}
	a.build()
	staticFS, err := webFS()
	if err != nil {
		log.Fatalf("初始化前端静态资源失败: %v", err)
	}
	a.buildHTTPServer(staticFS) // 组合 API 路由与静态文件服务器
	a.startHTTP()
	a.startDiscovery()

	url := fmt.Sprintf("http://%s:%d", a.mon.IP(), a.cfg.ListenPort)
	fmt.Printf("🌐 文件空间界面已启动: %s\n", url)

	openBrowser(url)

	a.waitAndShutdown()
}

// openBrowser 在默认浏览器中打开 url（跨平台）。
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux 等
		cmd = "xdg-open"
		args = []string{url}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("打开浏览器失败: %v（请手动访问 %s）", err, url)
	}
}
