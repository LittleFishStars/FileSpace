package api

import (
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

// 验证 /api/node 的 local 字段反映请求来源：回环请求为 true（本机访问），
// 非回环请求为 false（远程访问时前端把本机节点当作局域网节点展示，
// 隐藏本机管理入口）。
func TestHandleNodeLocal(t *testing.T) {
	cfg := config.DefaultConfig()
	srv := NewServer(Options{
		Config:  cfg,
		NodeID:  "self",
		Version: "test",
		Folders: share.NewManager(nil),
		Monitor: monitor.New(),
		Peers:   discovery.NewCache("self"),
	})

	check := func(remoteAddr string, wantLocal bool) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/node", nil)
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		srv.handleNode(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("状态码 = %d，期望 200", w.Code)
		}
		var info model.NodeInfo
		if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if info.Local != wantLocal {
			t.Errorf("RemoteAddr=%s 时 local = %v，期望 %v", remoteAddr, info.Local, wantLocal)
		}
	}

	check("127.0.0.1:54321", true)
	check("[::1]:54321", true)
	check("192.168.1.10:54321", false)
}
