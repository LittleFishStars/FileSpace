package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"filespace/internal/config"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
)

// newFoldersTestServer 构造带共享目录的测试 Server。
func newFoldersTestServer(t *testing.T, root string) (*Server, *share.Manager) {
	t.Helper()
	mgr := share.NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	srv := NewServer(Options{
		Config:  config.DefaultConfig(),
		NodeID:  "self",
		Version: "test",
		Folders: mgr,
		Monitor: monitor.New(),
		Peers:   discovery.NewCache("self"),
	})
	return srv, mgr
}

// setPasswordReq 调用 handleSetFolderPassword 并返回响应记录。
func setPasswordReq(srv *Server, remoteAddr, path, password string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"path": path, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/api/folders/password", bytes.NewReader(body))
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	srv.handleSetFolderPassword(w, req)
	return w
}

// TestHandleSetFolderPassword 修改密码接口：本机放行（设置/修改/移除），远程拒绝 403。
func TestHandleSetFolderPassword(t *testing.T) {
	root := t.TempDir()
	srv, mgr := newFoldersTestServer(t, root)
	id := folderIDOf(t, mgr, root)

	// 远程调用：403
	if w := setPasswordReq(srv, "192.168.1.10:54321", root, "secret"); w.Code != http.StatusForbidden {
		t.Fatalf("远程修改密码状态码 = %d，期望 403", w.Code)
	}

	// 本机设置密码
	if w := setPasswordReq(srv, "127.0.0.1:54321", root, "secret"); w.Code != http.StatusOK {
		t.Fatalf("本机设置密码状态码 = %d，期望 200", w.Code)
	}
	if pw, _ := mgr.FolderPasswd(id); pw != "secret" {
		t.Errorf("设置后 FolderPasswd = %q，期望 secret", pw)
	}

	// 本机移除密码（空值）
	if w := setPasswordReq(srv, "127.0.0.1:54321", root, ""); w.Code != http.StatusOK {
		t.Fatalf("本机移除密码状态码 = %d，期望 200", w.Code)
	}
	if pw, _ := mgr.FolderPasswd(id); pw != "" {
		t.Errorf("移除后 FolderPasswd = %q，期望空", pw)
	}

	// 未共享的目录：404
	missing := filepath.Join(t.TempDir(), "nope")
	if w := setPasswordReq(srv, "127.0.0.1:54321", missing, "secret"); w.Code != http.StatusNotFound {
		t.Fatalf("未共享目录状态码 = %d，期望 404", w.Code)
	}
}

// folderIDOf 返回 Manager 中 root 对应共享目录的 ID（用于断言密码状态）。
func folderIDOf(t *testing.T, mgr *share.Manager, root string) string {
	t.Helper()
	for _, f := range mgr.List() {
		if f.Path == root {
			return f.ID
		}
	}
	t.Fatalf("未找到共享目录 %s", root)
	return ""
}
