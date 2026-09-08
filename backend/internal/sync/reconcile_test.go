package sync

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 复刻 share.folderID：把路径 fnv32a 哈希后 hex 编码为 8 位十六进制。
func folderIDFor(path string) string {
	h := fnv.New32a()
	h.Write([]byte(path))
	return hex.EncodeToString(h.Sum(nil))
}

// fakeRemote 用 httptest.Server 模拟一个远端 filespace 节点：
//   - /api/folders 固定返回一个共享文件夹（id = folderIDFor("share")）；
//   - /api/folders/{id}/tree?path=... 从 remoteRoot 返回目录条目（懒加载）；
//   - /api/folders/{id}/download?path=... 返回文件内容（记录调用次数）；
//   - password 非空时提供 /api/auth（正确密码为 "secret"）——模拟「节点上存在需要
//     密码的文件夹」；目标文件夹自身的访问要求由 folderAuth 独立控制（可与节点
//     密码解耦，用于验证「节点有密码但目标文件夹开放」的同步场景）。
type fakeRemote struct {
	srv          *httptest.Server
	spec         Spec
	remoteRoot   string
	password     string
	folderAuth   bool
	downloadCall int
}

// newFakeRemote 创建模拟远端（目标文件夹是否设密码与节点是否有密码一致）。
func newFakeRemote(remoteRoot, password string) *fakeRemote {
	return newFakeRemoteAuth(remoteRoot, password, password != "")
}

// newFakeRemoteAuth 创建模拟远端，节点密码与目标文件夹 auth 解耦设置。
func newFakeRemoteAuth(remoteRoot, password string, folderAuth bool) *fakeRemote {
	fr := &fakeRemote{
		remoteRoot: remoteRoot,
		password:   password,
		folderAuth: folderAuth,
		spec:       Spec{Host: "127.0.0.1", FolderID: folderIDFor("share")},
	}
	mux := http.NewServeMux()

	if password != "" {
		mux.HandleFunc("/api/auth", func(w http.ResponseWriter, r *http.Request) {
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req["password"] != "secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "test-token"})
		})
	}

	mux.HandleFunc("/api/folders", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": fr.spec.FolderID, "name": "share", "auth": fr.folderAuth},
		})
	})

	prefix := "/api/folders/" + fr.spec.FolderID + "/"
	mux.HandleFunc("/api/folders/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, prefix)
		switch {
		case rest == "tree" || strings.HasPrefix(rest, "tree?"):
			fr.handleTree(w, r, r.URL.Query().Get("path"))
		case rest == "download" || strings.HasPrefix(rest, "download?"):
			fr.handleDownload(w, r, r.URL.Query().Get("path"))
		default:
			http.NotFound(w, r)
		}
	})

	fr.srv = httptest.NewServer(mux)
	u := strings.TrimPrefix(fr.srv.URL, "http://")
	host, portStr, _ := strings.Cut(u, ":")
	fr.spec.Host = host
	fmt.Sscanf(portStr, "%d", &fr.spec.Port)
	return fr
}

// handleTree 返回远端目录条目（懒加载一层）。
func (fr *fakeRemote) handleTree(w http.ResponseWriter, r *http.Request, rel string) {
	base := fr.remoteRoot
	if rel != "" {
		base = filepath.Join(fr.remoteRoot, filepath.FromSlash(rel))
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		relPath := filepath.ToSlash(filepath.Join(rel, e.Name()))
		out = append(out, map[string]any{
			"name":    e.Name(),
			"path":    relPath,
			"isDir":   e.IsDir(),
			"size":    info.Size(),
			"modTime": info.ModTime().Format(time.RFC3339),
		})
	}
	_ = json.NewEncoder(w).Encode(out)
}

// handleDownload 返回远端文件内容并记录调用次数。
func (fr *fakeRemote) handleDownload(w http.ResponseWriter, r *http.Request, rel string) {
	fr.downloadCall++
	full := filepath.Join(fr.remoteRoot, filepath.FromSlash(rel))
	http.ServeFile(w, r, full)
}

// Close 关闭模拟服务器。
func (fr *fakeRemote) Close() { fr.srv.Close() }

// writeTestFile 写文件并（可选）固定修改时间（UTC）。
func writeTestFile(t *testing.T, path, content, modTimeRFC string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if modTimeRFC != "" {
		mt, err := time.Parse(time.RFC3339, modTimeRFC)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
}

// readLocal 读取本地文件内容。
func readLocal(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(data)
}

// mustNotExist 断言路径不存在。
func mustNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("路径 %s 应不存在，实际仍存在", path)
	}
}

// TestReconcileMirrorAndIncremental 验证：
//  1. 首次对账完整镜像远端目录；
//  2. 无变化时第二次对账不重复下载（增量跳过）；
//  3. 远端新增文件后第三次对账只下载新增文件；
//  4. 远端删除后本地被清理。
func TestReconcileMirrorAndIncremental(t *testing.T) {
	remoteRoot := t.TempDir()
	writeTestFile(t, filepath.Join(remoteRoot, "a.txt"), "hello", "2024-01-01T10:00:00Z")
	if err := os.MkdirAll(filepath.Join(remoteRoot, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(remoteRoot, "sub", "b.txt"), "nested", "2024-01-02T10:00:00Z")

	fr := newFakeRemote(remoteRoot, "")
	defer fr.Close()
	spec := fr.spec
	spec.Local = filepath.Join(t.TempDir(), "mirror")
	task := &Task{spec: spec}
	ctx := context.Background()

	// 首次对账：全量镜像
	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("首次 reconcileOnce 失败: %v", err)
	}
	if got := readLocal(t, filepath.Join(spec.Local, "a.txt")); got != "hello" {
		t.Errorf("a.txt = %q，期望 hello", got)
	}
	if got := readLocal(t, filepath.Join(spec.Local, "sub", "b.txt")); got != "nested" {
		t.Errorf("sub/b.txt = %q，期望 nested", got)
	}

	// 无变化：第二次对账不下载任何文件
	before := fr.downloadCall
	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("第二次 reconcileOnce 失败: %v", err)
	}
	if fr.downloadCall != before {
		t.Errorf("无变化时不应重复下载: before=%d after=%d", before, fr.downloadCall)
	}

	// 远端新增 c.txt：第三次对账只下载它
	writeTestFile(t, filepath.Join(remoteRoot, "c.txt"), "new file", "2024-01-03T10:00:00Z")
	before = fr.downloadCall
	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("第三次 reconcileOnce 失败: %v", err)
	}
	if fr.downloadCall != before+1 {
		t.Errorf("新增一个文件应只下载 1 次: before=%d after=%d", before, fr.downloadCall)
	}
	if got := readLocal(t, filepath.Join(spec.Local, "c.txt")); got != "new file" {
		t.Errorf("c.txt = %q，期望 new file", got)
	}

	// 远端删除 a.txt：本地同步清理
	if err := os.Remove(filepath.Join(remoteRoot, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("删除后 reconcileOnce 失败: %v", err)
	}
	mustNotExist(t, filepath.Join(spec.Local, "a.txt"))
	// 其余文件仍在
	if got := readLocal(t, filepath.Join(spec.Local, "sub", "b.txt")); got != "nested" {
		t.Errorf("sub/b.txt 被误删: %q", got)
	}
}

// TestReconcileAuth 验证访问密码认证：错误密码同步失败，正确密码成功。
func TestReconcileAuth(t *testing.T) {
	remoteRoot := t.TempDir()
	writeTestFile(t, filepath.Join(remoteRoot, "secret.txt"), "top secret", "")

	fr := newFakeRemote(remoteRoot, "on")
	defer fr.Close()
	spec := fr.spec
	spec.Local = filepath.Join(t.TempDir(), "mirror")

	// 错误密码：认证应失败
	c := newRemoteClient(spec)
	if err := c.authenticate(context.Background()); err == nil {
		t.Fatalf("错误密码 authenticate 应失败")
	}

	// 正确密码：同步成功
	spec.Passwd = "secret"
	task := &Task{spec: spec}
	if err := task.reconcileOnce(context.Background(), newRemoteClient(spec)); err != nil {
		t.Fatalf("正确密码 reconcileOnce 失败: %v", err)
	}
	if got := readLocal(t, filepath.Join(spec.Local, "secret.txt")); got != "top secret" {
		t.Errorf("secret.txt = %q，期望 top secret", got)
	}
}

// TestReconcileOpenFolderUnauthed 验证开放文件夹在「节点上存在其他密码文件夹」时
// 仍能直接同步：认证判定按目标文件夹自身是否设密码（folder.Auth），而非节点整体。
// 回归：曾无条件调用 /api/auth（节点级校验），节点上有密码文件夹时开放文件夹
// 用空密码认证被拒（HTTP 401 密码可能错误），同步不断重试。
func TestReconcileOpenFolderUnauthed(t *testing.T) {
	remoteRoot := t.TempDir()
	writeTestFile(t, filepath.Join(remoteRoot, "open.txt"), "open content", "")

	// 节点有密码文件夹（password="on" 提供 /api/auth），但目标文件夹开放（folderAuth=false）
	fr := newFakeRemoteAuth(remoteRoot, "on", false)
	defer fr.Close()
	spec := fr.spec
	spec.Local = filepath.Join(t.TempDir(), "mirror")

	task := &Task{spec: spec}
	if err := task.reconcileOnce(context.Background(), newRemoteClient(spec)); err != nil {
		t.Fatalf("开放文件夹不应因节点存在密码文件夹而认证失败: %v", err)
	}
	if got := readLocal(t, filepath.Join(spec.Local, "open.txt")); got != "open content" {
		t.Errorf("open.txt = %q，期望 open content", got)
	}
}

// TestLocalExtrasRemoved 验证镜像语义：本地多余条目（远端不存在）被清理，
// 远端存在的文件不被误删。
func TestLocalExtrasRemoved(t *testing.T) {
	remoteRoot := t.TempDir()
	writeTestFile(t, filepath.Join(remoteRoot, "keep.txt"), "keep", "")
	fr := newFakeRemote(remoteRoot, "")
	defer fr.Close()
	spec := fr.spec
	spec.Local = filepath.Join(t.TempDir(), "mirror")
	task := &Task{spec: spec}
	ctx := context.Background()

	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("首次 reconcileOnce 失败: %v", err)
	}
	// 本地造多余文件与目录
	extra := filepath.Join(spec.Local, "extra.txt")
	extraDir := filepath.Join(spec.Local, "extra-dir")
	if err := os.WriteFile(extra, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(extraDir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extraDir, "nested", "f"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("第二次 reconcileOnce 失败: %v", err)
	}
	mustNotExist(t, extra)
	mustNotExist(t, extraDir)
	if got := readLocal(t, filepath.Join(spec.Local, "keep.txt")); got != "keep" {
		t.Errorf("keep.txt 被误删: %q", got)
	}
}

// TestMissingFolderStops 验证远端文件夹不存在时返回可识别的哨兵错误。
func TestMissingFolderStops(t *testing.T) {
	remoteRoot := t.TempDir()
	fr := newFakeRemote(remoteRoot, "")
	defer fr.Close()
	// 把 id 改成远端不存在的
	spec := fr.spec
	spec.FolderID = folderIDFor("other-share")
	spec.Local = filepath.Join(t.TempDir(), "mirror")
	task := &Task{spec: spec}
	err := task.reconcileOnce(context.Background(), newRemoteClient(spec))
	if err == nil {
		t.Fatalf("远端文件夹不存在时 reconcileOnce 应报错")
	}
	if !isMissingFolder(err) {
		t.Errorf("应识别为文件夹缺失错误，得到: %v", err)
	}
}
