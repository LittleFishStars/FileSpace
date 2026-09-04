package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"

	"filespace/internal/desktop"
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

// openBrowser 在默认浏览器中打开 url（跨平台：内部统一用系统默认应用打开）。
func openBrowser(url string) {
	if err := desktop.Open(url); err != nil {
		log.Printf("打开浏览器失败: %v（请手动访问 %s）", err, url)
	}
}
