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
	"filespace/internal/sync"
)

// Options 构建 Server 所需的依赖。
type Options struct {
	Config  *config.Config
	NodeID  string
	Version string
	Folders *share.Manager
	Monitor *monitor.Monitor
	Peers   *discovery.Cache
	// Hostname 本节点在局域网中显示的名称（默认系统主机名，可由 --hostname / 配置
	// hostname 自定义）。Monitor.Hostname() 仍返回真实系统主机名（节点 ID 依赖它），
	// 二者在自定义节点名称时不同。
	Hostname string
	// Persist 共享列表变更（添加/移除/修改密码）后的持久化回调，
	// 由外层提供（通常是把当前共享列表写回配置文件）。可为 nil。
	Persist func()
	// OnHostname 节点名称变更（本机管理页修改主机名）后的回调：外层据此
	// 同步 mDNS 宣告并写回配置文件。可为 nil；为 nil 时仅本次运行内存生效。
	OnHostname func(hostname string)
	// Syncs 后台同步任务管理器（-s/--sync 创建；本机已有后端运行时经
	// POST /api/sync/add 交接给它）。可为 nil；为 nil 时该端点返回 404。
	Syncs *sync.Manager
}

// Server HTTP API 服务。
type Server struct {
	cfg        *config.Config
	nodeID     string
	version    string
	folders    *share.Manager
	monitor    *monitor.Monitor
	hostname   string // 节点显示名称（默认系统主机名，可自定义）
	peers      *discovery.Cache
	persistFn  func()       // 共享列表变更后的持久化回调
	hostnameFn func(string) // 节点名称变更后的回调（mDNS + 配置持久化）
	auth       *auth.Tokens // 访问令牌管理（文件夹级密码认证）
	syncs      *sync.Manager
}

// NewServer 创建 API 服务。
func NewServer(opts Options) *Server {
	return &Server{
		cfg:        opts.Config,
		nodeID:     opts.NodeID,
		version:    opts.Version,
		folders:    opts.Folders,
		monitor:    opts.Monitor,
		hostname:   opts.Hostname,
		peers:      opts.Peers,
		persistFn:  opts.Persist,
		hostnameFn: opts.OnHostname,
		auth:       auth.NewTokens(),
		syncs:      opts.Syncs,
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
	mux.HandleFunc("POST /api/node/hostname", s.handleSetNodeHostname)
	mux.HandleFunc("POST /api/auth", s.handleAuth)
	mux.HandleFunc("GET /api/folders", s.handleFolders)
	mux.HandleFunc("POST /api/folders/add", s.handleAddFolders)
	mux.HandleFunc("POST /api/folders/remove", s.handleRemoveFolder)
	mux.HandleFunc("POST /api/folders/password", s.handleSetFolderPassword)
	mux.HandleFunc("POST /api/local/pick-directory", s.handlePickDirectory)
	mux.HandleFunc("POST /api/sync/add", s.handleSyncAdd)
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
