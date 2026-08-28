package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"

	"filespace/internal/model"
)

// Watch 持续监听局域网内的 filespace 服务，
// 对每个新发现的节点请求其 /api/node 与 /api/folders，写入缓存。
func Watch(ctx context.Context, service, domain string, cache *Cache, fetchTimeout time.Duration) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Printf("mDNS 解析器创建失败: %v", err)
		return
	}
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for entry := range entries {
			handleEntry(ctx, entry, cache, fetchTimeout)
		}
	}()
	if err := resolver.Browse(ctx, service, domain, entries); err != nil {
		log.Printf("mDNS 监听失败: %v", err)
	}
}

// handleEntry 处理一个 mDNS 服务条目，抓取节点详情后写入缓存。
func handleEntry(ctx context.Context, entry *zeroconf.ServiceEntry, cache *Cache, timeout time.Duration) {
	id := txtValue(entry.Text, "id")
	if id == "" || len(entry.AddrIPv4) == 0 || entry.Port == 0 {
		return
	}
	if cache.IsSelf(id) {
		return
	}
	peer, err := fetchPeer(ctx, entry.AddrIPv4[0].String(), entry.Port, timeout)
	if err != nil {
		return
	}
	cache.UpsertPeer(peer)
}

// txtValue 从 TXT 记录中取出指定 key 的值（记录形如 "key=value"）。
func txtValue(records []string, key string) string {
	prefix := key + "="
	for _, rec := range records {
		if strings.HasPrefix(rec, prefix) {
			return strings.TrimPrefix(rec, prefix)
		}
	}
	return ""
}

// fetchPeer 请求远程节点的 /api/node 与 /api/folders 构建 PeerInfo。
func fetchPeer(ctx context.Context, ip string, port int, timeout time.Duration) (*model.PeerInfo, error) {
	base := fmt.Sprintf("http://%s:%d", ip, port)
	client := &http.Client{Timeout: timeout}

	var node model.NodeInfo
	if err := getJSON(ctx, client, base+"/api/node", &node); err != nil {
		return nil, err
	}
	var folders []model.FolderInfo
	if err := getJSON(ctx, client, base+"/api/folders", &folders); err != nil {
		return nil, err
	}
	return &model.PeerInfo{
		Node:     node,
		Folders:  folders,
		Online:   true,
		LastSeen: time.Now().Format(time.RFC3339),
	}, nil
}

// getJSON 发起 GET 请求并解码 JSON 响应。
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求 %s 返回 %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
