package api

import (
	"fmt"
	"net/http"

	"filespace/internal/model"
)

// handleNode 返回本节点信息。
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	ip := s.monitor.IP()
	info := model.NodeInfo{
		ID:              s.nodeID,
		Hostname:        s.monitor.Hostname(),
		IP:              ip,
		OS:              s.monitor.OS(),
		SoftwareVersion: s.version,
		Status:          "online",
		Uptime:          s.monitor.Uptime(),
		ListenAddr:      fmt.Sprintf("%s:%d", ip, s.cfg.ListenPort),
		// 节点级标记：本节点是否存在需要访问密码的共享文件夹（具体看每个文件夹的 auth 字段）
		Auth: s.folders.HasPassword(),
		// 请求是否来自本机回环：远程设备访问时前端把本机节点当作局域网节点展示
		Local: isLoopbackRequest(r),
	}
	writeJSON(w, info)
}
