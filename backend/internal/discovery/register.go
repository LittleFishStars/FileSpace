// Package discovery 实现 mDNS 服务注册与发现。
package discovery

import (
	"context"

	"github.com/grandcat/zeroconf"
)

// Register 注册 mDNS 服务，使其他节点可以发现本节点。
// 使用所有已启用的非回环接口（含点对点接口如 VPN/tun），
// 确保每个 UP 接口的 IP 地址都包含在 mDNS 注册信息中。
// ctx 取消时自动关闭注册的 Server。
func Register(ctx context.Context, service, domain, instance string, port int, txt map[string]string) error {
	records := make([]string, 0, len(txt))
	for k, v := range txt {
		records = append(records, k+"="+v)
	}
	ifaces := listAllInterfaces()
	server, err := zeroconf.Register(instance, service, domain, port, records, ifaces)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()
	return nil
}
