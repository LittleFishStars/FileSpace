package api

import (
	"encoding/json"
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

// peerGoodbyeRequest 退出通知的请求体。
type peerGoodbyeRequest struct {
	// ID 退出节点的 ID（与 /api/node 的 id 一致）。
	ID string `json:"id"`
}

// handlePeerGoodbye 处理其他节点的退出通知：立即从缓存移除该节点，
// 使其马上从在线节点列表消失，无需等待 offlineTimeout 超时。
//
// 仅当节点确实在缓存中时才移除（无法凭空移除未知节点）；
// 伪造通知最多让某节点暂时从列表消失，mDNS 心跳会在几十秒内重新发现它，
// 危害有限，故不做更严格的来源校验（避免跨网络组网场景误伤）。
func (s *Server) handlePeerGoodbye(w http.ResponseWriter, r *http.Request) {
	var req peerGoodbyeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "缺少节点 id")
		return
	}
	s.peers.Remove(req.ID)
	w.WriteHeader(http.StatusOK)
}
