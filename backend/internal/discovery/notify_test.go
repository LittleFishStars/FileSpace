package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"filespace/internal/model"
)

// 临时验证：NotifyExit 向在线节点发送退出通知（POST /api/peers/goodbye，body 携带本机 id），
// 跳过离线节点；Cache.Remove 移除指定节点。
func TestNotifyExitAndRemove(t *testing.T) {
	cache := NewCache("self")
	offline := &model.PeerInfo{Node: model.NodeInfo{ID: "offline", Hostname: "offline-host", IP: "192.168.1.9", ListenAddr: "192.168.1.9:8080"}}
	cache.UpsertPeer(offline)
	// 手动标记 offline 节点为离线：直接改 lastSeen 为很久以前
	cache.mu.Lock()
	cache.lastSeen["offline"] = time.Now().Add(-10 * time.Minute)
	cache.mu.Unlock()

	var received atomic.Int32
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/peers/goodbye" {
			t.Errorf("请求不匹配: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("解析 body 失败: %v", err)
		}
		gotBody = body["id"]
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, port := splitHostPort(t, srv.URL)
	online := &model.PeerInfo{Node: model.NodeInfo{ID: "online", Hostname: "online-host", IP: host, ListenAddr: host + ":" + port}}
	cache.UpsertPeer(online)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	NotifyExit(ctx, cache, "self", 1*time.Second)

	if received.Load() != 1 {
		t.Errorf("应只通知 1 个在线节点，实际 %d", received.Load())
	}
	if gotBody != "self" {
		t.Errorf("通知 body id = %q，期望 self", gotBody)
	}

	// Remove 后 List 不再包含
	cache.Remove("online")
	for _, p := range cache.List() {
		if p.Node.ID == "online" {
			t.Error("Remove 后仍包含 online 节点")
		}
	}
}

func splitHostPort(t *testing.T, raw string) (string, string) {
	t.Helper()
	u := raw[len("http://"):]
	for i := len(u) - 1; i >= 0; i-- {
		if u[i] == ':' {
			return u[:i], u[i+1:]
		}
	}
	t.Fatalf("无法解析 %s", raw)
	return "", ""
}
