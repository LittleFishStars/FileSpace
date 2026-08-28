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
	}
	writeJSON(w, info)
}
