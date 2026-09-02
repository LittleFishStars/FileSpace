// Package api 提供 HTTP API 路由与处理函数。
package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path"
	"strings"

	"filespace/internal/config"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
	"filespace/internal/state"
)

// Options 构建 Server 所需的依赖。
type Options struct {
	Config  *config.Config
	NodeID  string
	Version string
	Folders *share.Manager
	Monitor *monitor.Monitor
	Peers   *discovery.Cache
}

// Server HTTP API 服务。
type Server struct {
	cfg     *config.Config
	nodeID  string
	version string
	folders *share.Manager
	monitor *monitor.Monitor
	peers   *discovery.Cache
	auth    *authManager // 访问令牌管理（文件夹级密码认证）
}

// NewServer 创建 API 服务。
func NewServer(opts Options) *Server {
	return &Server{
		cfg:     opts.Config,
		nodeID:  opts.NodeID,
		version: opts.Version,
		folders: opts.Folders,
		monitor: opts.Monitor,
		peers:   opts.Peers,
		auth:    newAuthManager(),
	}
}

// persistLastShared 把当前共享目录（含访问密码）立即写入上次共享记录。
// 在添加/移除共享、设置/修改/移除密码等运行中变更后调用，使密码与列表在
// 进程被强杀（无法走优雅退出落盘）时也不会丢失；优雅退出时仍会再写一次兜底。
func (s *Server) persistLastShared() {
	if err := state.SaveLastShared(s.folders.SharedSnapshot()); err != nil {
		log.Printf("记录共享目录失败: %v", err)
	}
}

// apiMux 构建纯 API 路由（不含 CORS 外层）。
func (s *Server) apiMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/node", s.handleNode)
	mux.HandleFunc("POST /api/auth", s.handleAuth)
	mux.HandleFunc("GET /api/folders", s.handleFolders)
	mux.HandleFunc("POST /api/folders/add", s.handleAddFolders)
	mux.HandleFunc("POST /api/folders/remove", s.handleRemoveFolder)
	mux.HandleFunc("POST /api/folders/password", s.handleSetFolderPassword)
	mux.HandleFunc("POST /api/local/pick-directory", s.handlePickDirectory)
	mux.HandleFunc("GET /api/folders/{id}/tree", s.handleTree)
	mux.HandleFunc("GET /api/folders/{id}/download", s.handleDownload)
	mux.HandleFunc("POST /api/folders/{id}/open", s.handleOpenFile)
	mux.HandleFunc("GET /api/peers", s.handlePeers)
	mux.HandleFunc("POST /api/peers/goodbye", s.handlePeerGoodbye)
	return mux
}

// Handler 返回纯 API 路由（含 CORS，供浏览器跨节点直连下载）。
// 不带静态文件托管，用于 filespace 后端独立运行（无 --web）的场景。
func (s *Server) Handler() http.Handler {
	return withCORS(s.apiMux())
}

// HandlerWithStatic 返回组合路由：/api/* 走 API，其余走静态文件服务器。
// 用于 filespace --web 模式：后端直接托管前端静态资源（go:embed 嵌入）。
func (s *Server) HandlerWithStatic(staticFS http.FileSystem) http.Handler {
	root := http.NewServeMux()
	root.Handle("/api/", withCORS(s.apiMux()))
	root.Handle("/", staticHandler(staticFS))
	return root
}

// staticHandler 提供 Next.js 静态导出（output: 'export'）的文件服务：
// 依次尝试 <路径>、<路径>.html、<路径>/index.html（Next 把 /folders 导出为 folders.html），
// 全部未命中时回退 Next 的 404.html（not-found 页）。
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

// withCORS 允许浏览器跨域直连其他节点（局域网 P2P）。
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// writeError 输出 JSON 错误响应。
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
