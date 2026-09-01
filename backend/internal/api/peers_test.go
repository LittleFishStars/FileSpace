package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"filespace/internal/config"
	"filespace/internal/discovery"
	"filespace/internal/model"
	"filespace/internal/monitor"
	"filespace/internal/share"
)

// 临时验证：POST /api/peers/goodbye 收到退出通知后立即从缓存移除该节点；
// 缺少 id 返回 400。
func TestHandlePeerGoodbye(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := NewServer(Options{
		Config:  cfg,
		NodeID:  "self",
		Version: "test",
		Folders: share.NewManager(nil),
		Monitor: monitor.New(),
		Peers:   discovery.NewCache("self"),
	})
	srv.peers.UpsertPeer(&model.PeerInfo{Node: model.NodeInfo{ID: "peer-a", Hostname: "a", IP: "192.168.1.5", ListenAddr: "192.168.1.5:8080"}})
	srv.peers.UpsertPeer(&model.PeerInfo{Node: model.NodeInfo{ID: "peer-b", Hostname: "b", IP: "192.168.1.6", ListenAddr: "192.168.1.6:8080"}})

	// 正常通知：移除 peer-a
	body, _ := json.Marshal(map[string]string{"id": "peer-a"})
	req := httptest.NewRequest(http.MethodPost, "/api/peers/goodbye", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handlePeerGoodbye(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", w.Code)
	}
	for _, p := range srv.peers.List() {
		if p.Node.ID == "peer-a" {
			t.Error("goodbye 后缓存仍包含 peer-a")
		}
	}

	// 未知 id：幂等移除，不影响其他节点
	body, _ = json.Marshal(map[string]string{"id": "unknown"})
	req = httptest.NewRequest(http.MethodPost, "/api/peers/goodbye", bytes.NewReader(body))
	w = httptest.NewRecorder()
	srv.handlePeerGoodbye(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("未知 id 状态码 = %d，期望 200", w.Code)
	}
	found := false
	for _, p := range srv.peers.List() {
		if p.Node.ID == "peer-b" {
			found = true
		}
	}
	if !found {
		t.Error("goodbye 误删了 peer-b")
	}

	// 缺少 id：400
	req = httptest.NewRequest(http.MethodPost, "/api/peers/goodbye", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	srv.handlePeerGoodbye(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("空 body 状态码 = %d，期望 400", w.Code)
	}
}
