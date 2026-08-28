// Package monitor 采集系统状态（主机名 / 系统 / 运行时间 / 局域网 IP）。
// 跨平台能力由 github.com/shirou/gopsutil 提供，无需手写平台实现。
package monitor

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"
)

// Monitor 提供节点系统信息。
type Monitor struct {
	hostname string
	os       string
	ip       string
	started  time.Time
}

// New 创建 Monitor 并采集一次系统信息。
func New() *Monitor {
	hostname, _ := os.Hostname()
	return &Monitor{
		hostname: hostname,
		os:       platformName(),
		ip:       localIP(),
		started:  time.Now(),
	}
}

// Hostname 返回主机名。
func (m *Monitor) Hostname() string { return m.hostname }

// OS 返回操作系统名称（gopsutil 平台名 + 版本，如 "arch" / "darwin 24.0.0" / "windows 10.0.19045"）。
func (m *Monitor) OS() string { return m.os }

// IP 返回局域网 IPv4 地址。
func (m *Monitor) IP() string { return m.ip }

// Uptime 返回系统运行时间（如 "12 天 4 小时"），读取失败时回退到进程运行时间。
func (m *Monitor) Uptime() string {
	if secs, err := host.Uptime(); err == nil && secs > 0 {
		return formatDuration(time.Duration(secs) * time.Second)
	}
	return formatDuration(time.Since(m.started))
}

// platformName 通过 gopsutil host.Info 获取跨平台系统信息，
// 按 platform / os / platformVersion 拼接，各部分首字母大写。
func platformName() string {
	info, err := host.Info()
	if err != nil {
		return runtime.GOOS + " " + runtime.GOARCH
	}
	parts := make([]string, 0, 3)
	for _, p := range []string{info.Platform, info.OS, info.PlatformVersion} {
		if p != "" {
			parts = append(parts, capitalize(p))
		}
	}
	if len(parts) == 0 {
		return runtime.GOOS + " " + runtime.GOARCH
	}
	return strings.Join(parts, " ")
}

// capitalize 把首字母大写（平台名均为 ASCII）。
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// formatDuration 把时长格式化为 "X 天 Y 小时"。
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	case hours > 0:
		return fmt.Sprintf("%d 小时", hours)
	default:
		return fmt.Sprintf("%d 分钟", mins)
	}
}

// localIP 返回第一个非回环 IPv4 地址。
func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}
