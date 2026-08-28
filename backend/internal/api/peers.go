package api

import (
	"net/http"

	"filespace/internal/model"
)

// handlePeers 返回 mDNS 发现的其他节点（含其共享文件夹）。
func (s *Server) handlePeers(w http.ResponseWriter, r *http.Request) {
	peers := s.peers.List()
	if peers == nil {
		peers = []model.PeerInfo{}
	}
	writeJSON(w, peers)
}
