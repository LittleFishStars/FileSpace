// Package discovery 实现 mDNS 服务注册与发现。
package discovery

import (
	"context"

	"github.com/grandcat/zeroconf"
)

// Register 注册 mDNS 服务，使其他节点可以发现本节点。
// 返回的 Server 在 ctx 取消时自动关闭。
func Register(ctx context.Context, service, domain, instance string, port int, txt map[string]string) (*zeroconf.Server, error) {
	records := make([]string, 0, len(txt))
	for k, v := range txt {
		records = append(records, k+"="+v)
	}
	server, err := zeroconf.Register(instance, service, domain, port, records, nil)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		server.Shutdown()
	}()
	return server, nil
}
