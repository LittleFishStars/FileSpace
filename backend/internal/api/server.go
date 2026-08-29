// Package api 提供 HTTP API 路由与处理函数。
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"filespace"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
)

// Options 构建 Server 所需的依赖。
type Options struct {
	Config  *filespace.Config
	NodeID  string
	Version string
	Folders *share.Manager
	Monitor *monitor.Monitor
	Peers   *discovery.Cache
	WebRoot string
}

// Server HTTP API 服务。
type Server struct {
	cfg     *filespace.Config
	nodeID  string
	version string
	folders *share.Manager
	monitor *monitor.Monitor
	peers   *discovery.Cache
	webRoot string
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
		webRoot: opts.WebRoot,
	}
}

// Handler 返回根路由（含 CORS 与静态资源托管）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/node", s.handleNode)
	mux.HandleFunc("GET /api/folders", s.handleFolders)
	mux.HandleFunc("POST /api/folders/add", s.handleAddFolders)
	mux.HandleFunc("GET /api/folders/{id}/tree", s.handleTree)
	mux.HandleFunc("GET /api/folders/{id}/download", s.handleDownload)
	mux.HandleFunc("GET /api/peers", s.handlePeers)
	mux.HandleFunc("/", s.handleStatic)
	return withCORS(mux)
}

// withCORS 允许浏览器跨域直连其他节点（局域网 P2P）。
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleStatic 托管前端静态资源，未命中的路径回退到 index.html（SPA）。
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(s.webRoot, filepath.Clean("/"+r.URL.Path))
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.webRoot, "index.html"))
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
