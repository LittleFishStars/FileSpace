package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"filespace/internal/model"
)

// goodbyePath 其他节点的退出通知端点：收到后立即从缓存移除本节点。
const goodbyePath = "/api/peers/goodbye"

// NotifyExit 在退出前向所有已发现节点发送"本节点已退出"通知，
// 让对方立即把本节点从在线列表移除，无需等待 offlineTimeout（60s）超时。
//
// 通知为尽力而为：任意节点发送失败（网络抖动、对方已离线等）不阻塞退出，
// 退化为等待对方离线超时。
func NotifyExit(ctx context.Context, cache *Cache, selfID string, timeout time.Duration) {
	peers := cache.List()
	var wg sync.WaitGroup
	for i := range peers {
		p := &peers[i]
		if !p.Online {
			continue // 已离线的节点收不到通知，跳过
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sendGoodbye(ctx, p, selfID, timeout); err != nil && ctx.Err() == nil {
				log.Printf("通知节点 %s（%s）退出失败: %v", p.Node.Hostname, p.Node.IP, err)
			}
		}()
	}
	wg.Wait()
}

// sendGoodbye 向单个节点发送退出通知（POST /api/peers/goodbye）。
func sendGoodbye(ctx context.Context, p *model.PeerInfo, selfID string, timeout time.Duration) error {
	body, err := json.Marshal(map[string]string{"id": selfID})
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, "http://"+peerAddr(p)+goodbyePath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("返回 %d", resp.StatusCode)
	}
	return nil
}
