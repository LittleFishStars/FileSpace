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

// download 把远程文件 rel 下载到本地磁盘 localPath：
// 整包下载 + 临时文件原子改名，避免半截文件被下次对账误判为有效。
// 是否跳过由对账层依据 tree 的 size/modTime 决定，本方法只负责「下载并落盘」。
func (c *remoteClient) download(ctx context.Context, rel, localPath string) error {
	if err := ensureParentDir(localPath); err != nil {
		return err
	}
	q := url.Values{"path": {rel}}
	resp, err := c.get(ctx, "/api/folders/"+c.spec.FolderID+"/download", q)
	if err != nil {
		return fmt.Errorf("下载 %q 失败: %w", rel, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载 %q 失败（HTTP %d）", rel, resp.StatusCode)
	}
	tmp := localPath + ".sync-tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("创建本地文件 %q 失败: %w", localPath, err)
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("写入本地文件 %q 失败: %w", localPath, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("关闭本地文件 %q 失败: %w", localPath, closeErr)
	}
	if err := os.Rename(tmp, localPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("落盘本地文件 %q 失败: %w", localPath, err)
	}
	return nil
}

// ensureParentDir 确保文件所在父目录存在。
func ensureParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}
