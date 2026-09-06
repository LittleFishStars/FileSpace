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

// httpClient 复用于对远程节点的全部 HTTP 请求（抓取节点详情、心跳探测、退出通知）：
// 带连接池与统一超时，避免 discovery 频繁新建/释放连接，
// 也避免此前 notify 用无超时的 http.DefaultClient（可能挂起）。
// 调用方如需更短超时，通过请求 context 控制（http.Client 取两者更早者）。
var httpClient = &http.Client{Timeout: 5 * time.Second}

// Watch 持续监听局域网内的 filespace 服务，并周期性探测已发现节点，写入缓存。
//
// 两条刷新路径互为兜底：
//  1. mDNS Browse：负责发现新节点并持续刷新（周期重建，见 browseLoop）。
//  2. HTTP 心跳：对缓存中已知节点直连 GET /api/node 刷新在线状态——
//     部分网络（如 KVM 虚拟机 NAT 网桥）的 mDNS 组播不可靠，HTTP 直连更稳定。
func Watch(ctx context.Context, service, domain string, cache *Cache) {
	go browseLoop(ctx, service, domain, cache)
	go heartbeatLoop(ctx, cache)
}

// browseInterval 单次 Browse 会话的存活时长：到期后重建 Browse（重新发送 PTR 查询）。
// 原因：grandcat/zeroconf 的 Browse 在收到第一个匹配条目后会停止周期查询
// （mainloop 调用 disableProbing），而服务端（同为该库）只在启动时宣告一次、
// 不周期重发，两端互相发现后会进入"只听不说"的静默状态，缓存不再刷新、
// 节点在 offlineTimeout 后被误判离线。周期重建强制持续发送查询以恢复刷新。
const browseInterval = 30 * time.Second

// browseLoop 周期性地重建 mDNS Browse，保证查询持续发送、缓存持续刷新。
func browseLoop(ctx context.Context, service, domain string, cache *Cache) {
	for ctx.Err() == nil {
		if !runBrowse(ctx, service, domain, cache) {
			// 解析器创建失败等：稍等再试，避免忙循环。
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}
}

// runBrowse 执行一轮 mDNS Browse：存活 browseInterval 时长后返回。
// 返回 false 表示本轮未正常跑完（解析器创建失败、Browse 启动失败等环境性问题）。
func runBrowse(ctx context.Context, service, domain string, cache *Cache) bool {
	browCtx, cancel := context.WithTimeout(ctx, browseInterval)
	defer cancel()
	ifaces := listAllInterfaces()
	resolver, err := zeroconf.NewResolver(zeroconf.SelectIfaces(ifaces))
	if err != nil {
		log.Printf("mDNS 解析器创建失败: %v", err)
		return false
	}
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		for entry := range entries {
			handleEntry(ctx, entry, cache)
		}
	}()
	// grandcat/zeroconf 的 Browse 是异步启动的：它在内部起 goroutine 执行
	// mainloop（持续监听）与 periodicQuery（周期查询）后立即返回，并不阻塞
	// 到 browCtx 结束。若启动成功后此处不等待就返回，browseLoop 会陷入无间隔
	// 紧循环反复重建解析器：每次重建都重新 join IPv4/IPv6 组播组、发送 PTR
	// 查询并堆积 goroutine/socket（此前实测 goroutine 编号在数秒内飙至上万、
	// CPU 100%、mDNS 组播包达每秒数万，多网卡环境尤甚，形成组播风暴）。
	// 因此 Browse 成功后必须阻塞等待 browCtx 到期（约 browseInterval），
	// 让单轮 Browse 真正存活满一个周期，再由 browseLoop 按节奏重建；
	// browCtx 到期（或外层 ctx 取消）时库内部 goroutine 自行清理连接。
	if err := resolver.Browse(browCtx, service, domain, entries); err != nil {
		log.Printf("mDNS 监听失败: %v", err)
		return false
	}
	<-browCtx.Done()
	return true
}

// heartbeatInterval HTTP 心跳间隔：对已知节点直连探测刷新在线状态，
// 弥补 mDNS 组播在部分网络（如虚拟机 NAT 网桥）不可靠的问题。
// 需明显小于 offlineTimeout（60s），保证心跳间隔内至少一次成功刷新。
const heartbeatInterval = 20 * time.Second

// heartbeatLoop 周期性对缓存中的已知节点发起 HTTP 探测，
// 成功即刷新其在线时间戳（Touch），失败保持原状（lastSeen 过期后标记离线）。
func heartbeatLoop(ctx context.Context, cache *Cache) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, p := range cache.List() {
				if ctx.Err() != nil {
					return
				}
				if err := probePeer(ctx, p); err == nil {
					cache.Touch(p.Node.ID)
				}
			}
		}
	}
}

// probePeer 直连探测一个已知节点的存活：GET /api/node 成功即认为在线。
func probePeer(ctx context.Context, p model.PeerInfo) error {
	var node model.NodeInfo
	return getJSON(ctx, httpClient, "http://"+peerAddr(&p)+"/api/node", &node)
}

// handleEntry 处理一个 mDNS 服务条目，抓取节点详情后写入缓存。
//
// 节点可能公布多个 IP（默认网卡 + VPN/tun 等）。逐个尝试直到连通，
// 并用第一个成功连接的 IP 覆盖对方上报的默认网卡 IP（见 fetchPeer 注释），
// 保证前端展示与后续访问（HTTP 心跳、目录/下载）都走「实际连接到的地址」。
func handleEntry(ctx context.Context, entry *zeroconf.ServiceEntry, cache *Cache) {
	id := txtValue(entry.Text, "id")
	if id == "" || len(entry.AddrIPv4) == 0 || entry.Port == 0 {
		return
	}
	if cache.IsSelf(id) {
		return
	}
	for _, ip := range entry.AddrIPv4 {
		peer, err := fetchPeer(ctx, ip.String(), entry.Port)
		if err != nil {
			continue
		}
		cache.UpsertPeer(peer)
		return
	}
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
//
// ip 是本机实际连通的远程地址（mDNS 条目中各候选之一），用它覆盖对方
// 上报的默认网卡 IP：对方的 NodeInfo.IP/ListenAddr 是它自己的第一个非回环
// 地址，在跨网络组网（如经 tun0 VPN 互联）时不可达，直接展示/访问会走错网。
// 此前端展示与实际访问（HTTP 心跳、目录列表、下载）都以「实际连接到的地址」为准。
func fetchPeer(ctx context.Context, ip string, port int) (*model.PeerInfo, error) {
	base := fmt.Sprintf("http://%s:%d", ip, port)

	var node model.NodeInfo
	if err := getJSON(ctx, httpClient, base+"/api/node", &node); err != nil {
		return nil, err
	}
	var folders []model.FolderInfo
	if err := getJSON(ctx, httpClient, base+"/api/folders", &folders); err != nil {
		return nil, err
	}
	// 用实际连接地址覆盖对方上报的默认网卡 IP（保证展示与访问可达）。
	node.IP = ip
	node.ListenAddr = fmt.Sprintf("%s:%d", ip, port)
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
