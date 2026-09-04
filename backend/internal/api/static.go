package api

import (
	"io"
	"net/http"
	"path"
	"strings"
)

// staticHandler 提供 Next.js 静态导出（output: 'export'）的文件服务：
// 依次尝试 <路径>、<路径>.html、<路径>/index.html（Next 把 /folders 导出为 folders.html），
// 全部未命中时回退 Next 的 404.html（not-found 页）。
// 供 --web 模式的后端托管前端静态资源（go:embed 嵌入）。
func staticHandler(fsys http.FileSystem) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		for _, name := range []string{p, p + ".html", p + "/index.html"} {
			if serveStaticFile(fsys, w, r, name, http.StatusOK) {
				return
			}
		}
		if !serveStaticFile(fsys, w, r, "404.html", http.StatusNotFound) {
			http.NotFound(w, r)
		}
	})
}

// serveStaticFile 从 fsys 打开 name 并返回其内容；文件不存在或为目录时返回 false。
func serveStaticFile(fsys http.FileSystem, w http.ResponseWriter, r *http.Request, name string, status int) bool {
	f, err := fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return false
	}
	if status != http.StatusOK {
		// 非 200（如 404 回退页）：手动写响应，避免 ServeContent 重复 WriteHeader 告警
		data, err := io.ReadAll(f)
		if err != nil {
			return false
		}
		w.WriteHeader(status)
		_, _ = w.Write(data)
		return true
	}
	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	return true
}
