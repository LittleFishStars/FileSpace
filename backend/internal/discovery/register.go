// Package discovery 实现 mDNS 服务注册与发现。
package discovery

import (
	"context"
	"sort"
	"sync"

	"github.com/grandcat/zeroconf"
)

// Registrar 本节点 mDNS 服务注册句柄：持有 zeroconf Server，
// 支持运行中更新 TXT 记录（如节点名称变更后立即让其他节点看到新名字）。
type Registrar struct {
	mu     sync.Mutex
	server *zeroconf.Server // 为 nil 表示注册失败（服务不可用）
}

// Register 注册 mDNS 服务，使其他节点可以发现本节点，
// 返回注册句柄供运行中更新 TXT 记录；ctx 取消时自动关闭注册的 Server。
// 使用所有已启用的非回环接口（含点对点接口如 VPN/tun），
// 确保每个 UP 接口的 IP 地址都包含在 mDNS 注册信息中。
func Register(ctx context.Context, service, domain, instance string, port int, txt map[string]string) (*Registrar, error) {
	r := &Registrar{}
	if err := r.register(ctx, service, domain, instance, port, txt); err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		r.shutdown()
	}()
	return r, nil
}

// register 创建 zeroconf Server（不持锁：仅被 Register 调用一次）。
// 接口列表在注册时解析一次（当前网络环境下可用接口的快照）。
func (r *Registrar) register(ctx context.Context, service, domain, instance string, port int, txt map[string]string) error {
	server, err := zeroconf.Register(instance, service, domain, port, txtRecords(txt), listAllInterfaces())
	if err != nil {
		return err
	}
	r.server = server
	return nil
}

// SetHostname 更新节点名称（mDNS TXT 的 hostname 字段）并立即向局域网宣告，
// 使其他节点缓存中的本节点名称刷新；instance（mDNS 服务实例名）保持节点 ID 不变。
// 注册失败（server 为 nil）时静默忽略——名称仅影响展示，不影响节点被发现。
func (r *Registrar) SetHostname(hostname string, txt map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server == nil {
		return
	}
	if txt == nil {
		txt = map[string]string{}
	}
	txt["hostname"] = hostname
	r.server.SetText(txtRecords(txt))
}

// shutdown 关闭 zeroconf Server（发送 goodbye 反注册）。
func (r *Registrar) shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil {
		r.server.Shutdown()
		r.server = nil
	}
}

// txtRecords 把 TXT 键值映射转为 zeroconf 记录（形如 "key=value"），
// 排序保证输出稳定（mDNS 应答内容确定性，避免无意义抖动）。
func txtRecords(txt map[string]string) []string {
	keys := make([]string, 0, len(txt))
	for k := range txt {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	records := make([]string, 0, len(txt))
	for _, k := range keys {
		records = append(records, k+"="+txt[k])
	}
	return records
}
