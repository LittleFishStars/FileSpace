// Package discovery 实现 mDNS 服务注册与发现。
package discovery

import "net"

// listAllInterfaces 返回所有已启用的非回环网络接口（含点对点接口如 VPN/tun），
// 供 zeroconf 的 Register 与 NewResolver 使用。
//
// grandcat/zeroconf 默认的 listMulticastInterfaces 只选择同时有 UP 和 MULTICAST
// 标志的接口，排除了点对点接口（如 VPN 隧道 tun0）。这些接口虽然不支持组播，
// 但其 IP 地址需要被包含在 mDNS 注册信息中，以便其他节点通过该网络发现本节点。
// JoinGroup 在点对点接口上会失败，但库会优雅跳过（仅当全部接口失败才报错），
// 不影响其他接口的正常工作。
func listAllInterfaces() []net.Interface {
	var interfaces []net.Interface
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 {
			continue
		}
		// 排除回环接口
		if ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		interfaces = append(interfaces, ifi)
	}
	return interfaces
}