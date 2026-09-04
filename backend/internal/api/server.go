// Package api 提供 HTTP API 路由与处理函数。
// 本文件是服务装配与路由表；静态文件托管见 static.go，
// 响应/CORS 助手见 respond.go，各端点处理按资源分布在 node/folders/files/auth/peers/open 等文件。
package api

import (
	"net/http"

	"filespace/internal/auth"
	"filespace/internal/config"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
)

// Options 构建 Server 所需的依赖。
type Options struct {
	Config  *config.Config
	NodeID  string
	Version string
	Folders *share.Manager
	Monitor *monitor.Monitor
	Peers   *discovery.Cache
	// Persist 共享列表变更（添加/移除/修改密码）后的持久化回调，
	// 由外层提供（通常是把当前共享列表写回配置文件）。可为 nil。
	Persist func()
}

// Server HTTP API 服务。
type Server struct {
	cfg       *config.Config
	nodeID    string
	version   string
	folders   *share.Manager
	monitor   *monitor.Monitor
	peers     *discovery.Cache
	persistFn func()       // 共享列表变更后的持久化回调
	auth      *auth.Tokens // 访问令牌管理（文件夹级密码认证）
}

// NewServer 创建 API 服务。
func NewServer(opts Options) *Server {
	return &Server{
		cfg:       opts.Config,
		nodeID:    opts.NodeID,
		version:   opts.Version,
		folders:   opts.Folders,
		monitor:   opts.Monitor,
		peers:     opts.Peers,
		persistFn: opts.Persist,
		auth:      auth.NewTokens(),
	}
}

// persistChanged 触发共享列表变更后的持久化回调（外层写回配置文件）。
func (s *Server) persistChanged() {
	if s.persistFn != nil {
		s.persistFn()
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
