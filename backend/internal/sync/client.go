package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filespace/internal/model"
)

// 同步用 HTTP 客户端：统一超时，避免单个端点异常挂起整个对账。
var client = &http.Client{Timeout: 30 * time.Second}

// remoteClient 封装对远程 filespace 节点的 API 访问与访问密码认证。
type remoteClient struct {
	spec    Spec
	token   string // 通过 /api/auth 换取的访问令牌（该文件夹需要密码时）
	baseURL string
}

// newRemoteClient 基于 Spec 构造远程客户端（不发起网络请求）。
func newRemoteClient(spec Spec) *remoteClient {
	return &remoteClient{
		spec:    spec,
		baseURL: "http://" + spec.Address(),
	}
}

// authenticate 若远端该文件夹设置了访问密码，则用 Passwd 换取访问令牌并保存
// （后续 tree/download 请求附带 Bearer 令牌）。远端没有需要密码的文件夹时返回 nil。
func (c *remoteClient) authenticate(ctx context.Context) error {
	if c.token != "" {
		return nil
	}
	body, err := json.Marshal(map[string]string{"password": c.spec.Passwd})
	if err != nil {
		return err
	}
	resp, err := c.post(ctx, "/api/auth", body)
	if err != nil {
		return fmt.Errorf("远程节点认证失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		// 该节点没有需要密码的文件夹：无需认证，直接访问
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("远程节点认证失败（HTTP %d）: 密码可能错误", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("解析认证响应失败: %w", err)
	}
	if out.Token == "" {
		return fmt.Errorf("远程节点未返回访问令牌")
	}
	c.token = out.Token
	return nil
}

// post 发送带 JSON 体的 POST 请求。
// 超时由 client.Timeout（30s，覆盖连接/请求/响应体读取）统一兜底，不在此
// 额外包 context.WithTimeout：过早 cancel 会关闭底层连接，调用方返回后再读
// 响应体会报 use of closed network connection。
func (c *remoteClient) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return client.Do(req)
}

// get 构造带认证（令牌）的 GET 请求。超时语义同 post：由 client.Timeout 兜底，
// 不在此 cancel context（详见 post 注释——过早取消会令调用方读响应体时连接已关闭）。
func (c *remoteClient) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return client.Do(req)
}

// listFolders 拉取远端节点共享文件夹列表，并定位目标文件夹（按 id 匹配）。
func (c *remoteClient) findFolder(ctx context.Context) (*model.FolderInfo, error) {
	resp, err := c.get(ctx, "/api/folders", nil)
	if err != nil {
		return nil, fmt.Errorf("获取远程共享列表失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取远程共享列表失败（HTTP %d）", resp.StatusCode)
	}
	var folders []model.FolderInfo
	if err := json.NewDecoder(resp.Body).Decode(&folders); err != nil {
		return nil, fmt.Errorf("解析远程共享列表失败: %w", err)
	}
	for i := range folders {
		if folders[i].ID == c.spec.FolderID {
			return &folders[i], nil
		}
	}
	return nil, nil
}

// tree 拉取远端目录 rel 下的条目列表（懒加载，仅一层）。rel 为空表示共享根目录。
func (c *remoteClient) tree(ctx context.Context, rel string) ([]model.FileInfo, error) {
	q := url.Values{}
	if rel != "" {
		q.Set("path", rel)
	}
	resp, err := c.get(ctx, "/api/folders/"+c.spec.FolderID+"/tree", q)
	if err != nil {
		return nil, fmt.Errorf("获取远程目录列表 %q 失败: %w", rel, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("获取远程目录列表 %q 失败（HTTP %d）", rel, resp.StatusCode)
	}
	var files []model.FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, fmt.Errorf("解析远程目录列表 %q 失败: %w", rel, err)
	}
	return files, nil
}

// 断点续传临时产物：大文件下载可能因客户端 30s 超时或网络中断而中止，
// 中断时保留 .sync-tmp 已下载部分与 .sync-tmp.json 版本元数据，下次对账
// 凭元数据校验远端版本一致后从断点（Range）续传而非整包重下。远端
// handleDownload 用 http.ServeFile 原生支持 Range/If-Range，无需远端配合；
// 远端为旧版本/不支持 Range 时回退 200 全量，行为不变。
const (
	tmpSuffix  = ".sync-tmp"      // 未完成下载的临时文件后缀
	metaSuffix = ".sync-tmp.json" // 临时文件对应的远端版本元数据后缀
)

// tmpMeta 记录 .sync-tmp 对应哪个远端版本（下载开始时的 size+modTime）。
// 续传前必须与当轮远端条目逐一比对：版本不一致（中断期间远端已更新）时
// 从头下载，避免新旧内容拼接成损坏文件。
type tmpMeta struct {
	Size int64  `json:"size"` // 远端文件完整大小（字节）
	Mod  string `json:"mod"`  // 远端文件修改时间（RFC3339，秒精度）
}

// resumeOffset 计算 .sync-tmp 的可续传偏移：临时文件与元数据存在、元数据与
// 远端条目一致（同版本）、且已下载部分少于完整大小时，返回已下载字节数；
// 否则返回 0（从头下载）。临时文件大小达到或超过远端大小视为异常状态，
// 从头下载保证内容完整正确。
func resumeOffset(tmpPath, metaPath string, size int64, mod string) int64 {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return 0
	}
	var m tmpMeta
	if json.Unmarshal(data, &m) != nil {
		return 0
	}
	if m.Size != size || m.Mod != mod {
		return 0
	}
	fi, err := os.Stat(tmpPath)
	if err != nil || fi.IsDir() {
		return 0
	}
	if fi.Size() <= 0 || fi.Size() >= size {
		return 0
	}
	return fi.Size()
}

// openDownload 发起文件下载请求并返回响应体与实际写入起始偏移：
//   - offset>0 时携带 Range: bytes=offset- 与 If-Range（以远端版本 modTime
//     转 HTTP-date）。服务端支持且文件未变 → 206，body 从 offset 开始
//     （可追加续传）；不支持或文件已变 → 200，body 从头开始（须截断重写）。
//   - offset==0 时普通整包请求，恒 200。
//
// start 为实际应写入偏移（206 → offset，200 → 0），body 为响应流（调用方负责关闭）。
func (c *remoteClient) openDownload(ctx context.Context, rel string, offset int64, mod string) (start int64, body io.ReadCloser, err error) {
	q := url.Values{"path": {rel}}
	u := c.baseURL + "/api/folders/" + c.spec.FolderID + "/download?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		// If-Range：秒级一致才续传，否则服务端忽略 Range 返回 200 全量，
		// 兜底「远端文件在 tree 快照后被修改」的竞态窗口。
		if rt, perr := time.Parse(time.RFC3339, mod); perr == nil {
			req.Header.Set("If-Range", rt.UTC().Format(http.TimeFormat))
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("下载 %q 失败: %w", rel, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return 0, resp.Body, nil
	case http.StatusPartialContent:
		return offset, resp.Body, nil
	default:
		_ = resp.Body.Close()
		return 0, nil, fmt.Errorf("下载 %q 失败（HTTP %d）", rel, resp.StatusCode)
	}
}

// download 把远程文件 rel（远端大小 size、修改时间 mod）下载到本地磁盘
// localPath，支持断点续传：本地已有匹配元数据的 .sync-tmp 时从断点续传，
// 而非整包重下。下载失败（网络中断/超时）保留临时文件与元数据，供下一次
// 对账续传；成功则以临时文件原子改名落盘并删除元数据，避免半截文件被
// 下次对账误判为有效。是否跳过由对账层依据 tree 的 size/modTime 决定。
func (c *remoteClient) download(ctx context.Context, rel, localPath string, size int64, mod string) error {
	if err := ensureParentDir(localPath); err != nil {
		return err
	}
	tmp := localPath + tmpSuffix
	metaPath := localPath + metaSuffix
	offset := resumeOffset(tmp, metaPath, size, mod)
	start, body, err := c.openDownload(ctx, rel, offset, mod)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("创建本地文件 %q 失败: %w", localPath, err)
	}
	// 整包下载（start==0，含续传被服务端拒绝回退全量）时截断临时文件：
	// 其上可能遗留上次未完成/版本不符的部分数据。
	if start == 0 {
		if err := f.Truncate(0); err != nil {
			_ = f.Close()
			return fmt.Errorf("重置本地文件 %q 失败: %w", localPath, err)
		}
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		_ = f.Close()
		return fmt.Errorf("定位本地文件 %q 失败: %w", localPath, err)
	}
	// 先落元数据再写数据：中断后凭它判断 .sync-tmp 对应哪个远端版本。
	if err := saveTmpMeta(metaPath, size, mod); err != nil {
		_ = f.Close()
		return fmt.Errorf("写入下载元数据失败: %w", err)
	}

	_, copyErr := io.Copy(f, body)
	closeErr := f.Close()
	if copyErr != nil {
		// 保留 .sync-tmp 与元数据：下次对账从断点续传
		return fmt.Errorf("写入本地文件 %q 失败: %w", localPath, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭本地文件 %q 失败: %w", localPath, closeErr)
	}
	if err := os.Rename(tmp, localPath); err != nil {
		return fmt.Errorf("落盘本地文件 %q 失败: %w", localPath, err)
	}
	removeTmpMeta(metaPath)
	return nil
}

// saveTmpMeta 原子写入 .sync-tmp 对应的远端版本元数据（先写临时文件再改名，
// 避免中断留下半截 json）。
func saveTmpMeta(metaPath string, size int64, mod string) error {
	data, err := json.Marshal(tmpMeta{Size: size, Mod: mod})
	if err != nil {
		return err
	}
	tmp := metaPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, metaPath)
}

// removeTmpMeta 删除 .sync-tmp 对应的元数据（下载完成落盘后调用；失败仅残留
// 一个无引用 json，本地多余清理会豁免，不影响正确性）。
func removeTmpMeta(metaPath string) { _ = os.Remove(metaPath) }

// isTmpArtifact 判断相对路径是否为同步下载产生的临时产物（未完成下载的
// .sync-tmp 及其元数据 *.sync-tmp.json、*.sync-tmp.json.tmp）。它们是断点
// 续传的现场，清理本地多余条目时必须豁免，否则下一轮对账的续传现场被删。
func isTmpArtifact(rel string) bool {
	return strings.HasSuffix(rel, tmpSuffix) ||
		strings.HasSuffix(rel, metaSuffix) ||
		strings.HasSuffix(rel, metaSuffix+".tmp")
}

// ensureParentDir 确保文件所在父目录存在。
func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
