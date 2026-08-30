package discovery

import (
	"sync"
	"time"

	"filespace/internal/model"
)

// offlineTimeout 超过该时长未刷新则标记为离线。
const offlineTimeout = 60 * time.Second

// Cache 缓存 mDNS 发现的其他节点。
type Cache struct {
	mu       sync.RWMutex
	selfID   string
	peers    map[string]*model.PeerInfo
	lastSeen map[string]time.Time
}

// NewCache 创建节点缓存，selfID 用于排除自身。
func NewCache(selfID string) *Cache {
	return &Cache{
		selfID:   selfID,
		peers:    map[string]*model.PeerInfo{},
		lastSeen: map[string]time.Time{},
	}
}

// IsSelf 判断节点 ID 是否为自身。
func (c *Cache) IsSelf(id string) bool { return id == c.selfID }

// UpsertPeer 记录或刷新一个节点。
func (c *Cache) UpsertPeer(p *model.PeerInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	p.Online = true
	p.LastSeen = now.Format(time.RFC3339)
	c.peers[p.Node.ID] = p
	c.lastSeen[p.Node.ID] = now
}

// Touch 记录一次成功的活跃探测（HTTP 心跳），刷新该节点的在线时间戳。
// 仅对已知节点生效；探测失败不应调用，让 lastSeen 自然过期以标记离线。
func (c *Cache) Touch(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.peers[id]; ok {
		c.lastSeen[id] = time.Now()
	}
}

// List 返回已知节点列表（附在线状态，超时未刷新标记离线）。
func (c *Cache) List() []model.PeerInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]model.PeerInfo, 0, len(c.peers))
	now := time.Now()
	for id, p := range c.peers {
		cp := *p
		cp.Online = now.Sub(c.lastSeen[id]) < offlineTimeout
		out = append(out, cp)
	}
	return out
}
