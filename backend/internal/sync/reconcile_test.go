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

	"filespace/internal/model"
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
	lastRange    string // 最近一次下载请求的 Range 头（验证断点续传）
	cutFirst     bool   // 首次下载只回一半内容并截断连接（模拟大文件中途网络中断）
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

// handleDownload 返回远端文件内容并记录调用次数与最近一次 Range 头。
// cutFirst 开启时首次请求只写一半内容（Content-Length 声明完整大小但未写满，
// 服务端返回后强制截断连接 → 客户端读到 unexpected EOF），模拟大文件下载
// 中途网络中断；后续请求走正常路径（http.ServeFile 原生处理 Range 续传）。
func (fr *fakeRemote) handleDownload(w http.ResponseWriter, r *http.Request, rel string) {
	fr.downloadCall++
	fr.lastRange = r.Header.Get("Range")
	full := filepath.Join(fr.remoteRoot, filepath.FromSlash(rel))
	if fr.cutFirst && fr.downloadCall == 1 {
		data, err := os.ReadFile(full)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data[:len(data)/2])
		return
	}
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

// TestTreeLargeResponse 验证大体积目录列表（超出 http.Transport 预读缓冲，
// 必须真正从网络读取响应体）能完整解析。
// 回归：get() 曾在返回前 defer cancel() 请求 context，导致调用方读取响应体时
// 底层连接已被关闭（use of closed network connection）——小响应恰好落在预读
// 缓冲内不触发，大目录（如 .git/objects 的深层遍历）必现。
func TestTreeLargeResponse(t *testing.T) {
	count := 800 // 条目足够多，JSON 序列化后远超 transport 预读缓冲（默认 4KB）
	entries := make([]model.FileInfo, 0, count)
	for i := 0; i < count; i++ {
		entries = append(entries, model.FileInfo{
			Name:    fmt.Sprintf("file-%04d.bin", i),
			Path:    fmt.Sprintf("file-%04d.bin", i),
			Size:    100,
			ModTime: "2024-01-01T10:00:00Z",
		})
	}
	body, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 4096 {
		t.Fatalf("测试数据不足 4KB（实际 %d），无法触发预读缓冲之外的真实读取", len(body))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/folders/"+folderIDFor("share")+"/tree", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	host, portStr, _ := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")
	spec := Spec{Host: host, FolderID: folderIDFor("share")}
	fmt.Sscanf(portStr, "%d", &spec.Port)

	got, err := newRemoteClient(spec).tree(context.Background(), "")
	if err != nil {
		t.Fatalf("大响应 tree 读取失败: %v", err)
	}
	if len(got) != count {
		t.Errorf("解析条目数 = %d，期望 %d", len(got), count)
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

// TestDownloadResume 验证断点续传：大文件下载中断后保留 .sync-tmp 与元数据，
// 第二轮对账从断点（Range）续传完成，最终文件完整且内容正确。
func TestDownloadResume(t *testing.T) {
	remoteRoot := t.TempDir()
	content := strings.Repeat("sync-resume-data-", 1000) // 17000 字节
	writeTestFile(t, filepath.Join(remoteRoot, "big.bin"), content, "2024-01-01T10:00:00Z")

	fr := newFakeRemote(remoteRoot, "")
	fr.cutFirst = true // 首次下载只回一半（模拟网络中断）
	defer fr.Close()
	spec := fr.spec
	spec.Local = filepath.Join(t.TempDir(), "mirror")
	task := &Task{spec: spec}
	ctx := context.Background()

	local := filepath.Join(spec.Local, "big.bin")
	tmp := local + ".sync-tmp"
	meta := local + ".sync-tmp.json"

	// 第一轮：下载中断（只拿到一半），临时文件与元数据必须保留
	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("首次 reconcileOnce 失败: %v", err)
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("中断下载后不应有最终文件 %s", local)
	}
	fi, err := os.Stat(tmp)
	if err != nil {
		t.Fatalf("中断下载后应保留临时文件 %s: %v", tmp, err)
	}
	if fi.Size() != int64(len(content)/2) {
		t.Errorf("临时文件大小 = %d，期望 %d（一半）", fi.Size(), len(content)/2)
	}
	if _, err := os.Stat(meta); err != nil {
		t.Fatalf("应保留断点元数据 %s: %v", meta, err)
	}

	// 第二轮：从断点续传，最终文件完整；下载请求携带 Range 头
	fr.lastRange = ""
	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("续传 reconcileOnce 失败: %v", err)
	}
	if got := readLocal(t, local); got != content {
		t.Errorf("续传后 big.bin 内容不完整（len=%d，期望 %d）", len(got), len(content))
	}
	if !strings.HasPrefix(fr.lastRange, "bytes=") {
		t.Errorf("续传请求应携带 Range 头，实际: %q", fr.lastRange)
	}
	mustNotExist(t, tmp)
	mustNotExist(t, meta)
}

// TestDownloadResumeVersionChanged 验证断点失效场景：中断期间远端文件更新
// （修改时间变化），第二轮必须从头下载新版本，不得拼接出新旧混合文件。
func TestDownloadResumeVersionChanged(t *testing.T) {
	remoteRoot := t.TempDir()
	contentV1 := "version-one-" + strings.Repeat("x", 1000)
	contentV2 := "version-two-" + strings.Repeat("y", 1000)
	writeTestFile(t, filepath.Join(remoteRoot, "big.bin"), contentV1, "2024-01-01T10:00:00Z")

	fr := newFakeRemote(remoteRoot, "")
	fr.cutFirst = true // 首次下载中断
	defer fr.Close()
	spec := fr.spec
	spec.Local = filepath.Join(t.TempDir(), "mirror")
	task := &Task{spec: spec}

	if err := task.reconcileOnce(context.Background(), newRemoteClient(spec)); err != nil {
		t.Fatalf("首次 reconcileOnce 失败: %v", err)
	}
	// 远端在中断后更新（大小相同、修改时间变化）：续传必须从头下载新版本
	writeTestFile(t, filepath.Join(remoteRoot, "big.bin"), contentV2, "2024-02-01T10:00:00Z")

	if err := task.reconcileOnce(context.Background(), newRemoteClient(spec)); err != nil {
		t.Fatalf("版本变化后 reconcileOnce 失败: %v", err)
	}
	if got := readLocal(t, filepath.Join(spec.Local, "big.bin")); got != contentV2 {
		t.Errorf("版本变化后应得到新内容（len=%d，期望 %d）", len(got), len(contentV2))
	}
	if fr.lastRange != "" {
		t.Errorf("版本变化后应从头下载（无 Range），实际: %q", fr.lastRange)
	}
}

// TestTmpArtifactExemptFromCleanup 验证断点续传现场（.sync-tmp 与元数据）
// 不会被「本地多余条目清理」误删：镜像清理只针对远端不存在的普通文件。
func TestTmpArtifactExemptFromCleanup(t *testing.T) {
	remoteRoot := t.TempDir()
	writeTestFile(t, filepath.Join(remoteRoot, "a.txt"), "keep", "")
	fr := newFakeRemote(remoteRoot, "")
	defer fr.Close()
	spec := fr.spec
	spec.Local = filepath.Join(t.TempDir(), "mirror")
	task := &Task{spec: spec}
	ctx := context.Background()

	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("首次 reconcileOnce 失败: %v", err)
	}
	// 伪造一个中断下载现场（目标文件已就绪但临时文件残留）
	tmp := filepath.Join(spec.Local, "a.txt.sync-tmp")
	meta := filepath.Join(spec.Local, "a.txt.sync-tmp.json")
	if err := os.WriteFile(tmp, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(meta, []byte(`{"size":4,"mod":"2024-01-01T10:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// 第二轮对账（远端无变化）：增量命中 + 多余清理，续传现场必须豁免保留
	if err := task.reconcileOnce(ctx, newRemoteClient(spec)); err != nil {
		t.Fatalf("第二次 reconcileOnce 失败: %v", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Errorf("断点续传现场 %s 不应被多余清理删除: %v", tmp, err)
	}
	if _, err := os.Stat(meta); err != nil {
		t.Errorf("断点元数据 %s 不应被多余清理删除: %v", meta, err)
	}
}
