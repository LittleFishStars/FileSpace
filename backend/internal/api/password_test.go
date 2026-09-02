package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"filespace/internal/config"
	"filespace/internal/discovery"
	"filespace/internal/monitor"
	"filespace/internal/share"
	"filespace/internal/state"
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

// sha256Hex 计算明文的 sha256 十六进制（供断言内部只存哈希）。
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // 变更后即时落盘写临时目录，避免污染真实配置
	root := t.TempDir()
	srv, mgr := newFoldersTestServer(t, root)
	id := folderIDOf(t, mgr, root)

	// 远程调用：403
	if w := setPasswordReq(srv, "192.168.1.10:54321", root, "secret"); w.Code != http.StatusForbidden {
		t.Fatalf("远程修改密码状态码 = %d，期望 403", w.Code)
	}

	// 本机设置密码：内部存哈希
	if w := setPasswordReq(srv, "127.0.0.1:54321", root, "secret"); w.Code != http.StatusOK {
		t.Fatalf("本机设置密码状态码 = %d，期望 200", w.Code)
	}
	if pw, _ := mgr.FolderPasswdHash(id); pw != sha256Hex("secret") {
		t.Errorf("设置后 FolderPasswdHash = %q，期望 %q", pw, sha256Hex("secret"))
	}

	// 本机移除密码（空值）
	if w := setPasswordReq(srv, "127.0.0.1:54321", root, ""); w.Code != http.StatusOK {
		t.Fatalf("本机移除密码状态码 = %d，期望 200", w.Code)
	}
	if pw, _ := mgr.FolderPasswdHash(id); pw != "" {
		t.Errorf("移除后 FolderPasswdHash = %q，期望空", pw)
	}

	// 未共享的目录：404
	missing := filepath.Join(t.TempDir(), "nope")
	if w := setPasswordReq(srv, "127.0.0.1:54321", missing, "secret"); w.Code != http.StatusNotFound {
		t.Fatalf("未共享目录状态码 = %d，期望 404", w.Code)
	}
}

// TestPasswordPersistAndRestore 修改密码经 handler 落盘（仅存哈希）后，重启（按上次共享
// 记录重建 Manager）仍能恢复密码——回归验证"设置密码后重启丢失"问题。
func TestPasswordPersistAndRestore(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	srv, _ := newFoldersTestServer(t, root)

	// 设置密码（handler 内部立即持久化到 last-shared.yaml）
	if w := setPasswordReq(srv, "127.0.0.1:54321", root, "s3cret"); w.Code != http.StatusOK {
		t.Fatalf("设置密码状态码 = %d，期望 200", w.Code)
	}
	// 持久化文件不得含明文
	for _, sf := range state.LoadLastShared() {
		if sf.Passwd != "" {
			t.Error("last-shared.yaml 不应保存明文密码")
		}
		if sf.PasswdHash != sha256Hex("s3cret") {
			t.Errorf("持久化 PasswdHash = %q，期望 %q", sf.PasswdHash, sha256Hex("s3cret"))
		}
	}
	// 模拟重启：从上次共享记录重建 Manager，列表应仍带密码标记，且认证可用
	restored := share.NewManager(state.LoadLastShared())
	defer restored.Close()
	found := false
	for _, f := range restored.List() {
		if f.Path == root {
			found = true
			if !f.Auth {
				t.Error("重启恢复后该文件夹 auth = false，密码未持久化")
			}
		}
	}
	if !found {
		t.Errorf("重启恢复列表中未找到共享目录 %s", root)
	}
	if !restored.MatchPassword("s3cret") {
		t.Error("重启恢复后 MatchPassword(s3cret) 应为 true")
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
