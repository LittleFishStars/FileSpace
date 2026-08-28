// Package monitor 采集系统状态（主机名 / 系统 / 运行时间 / 局域网 IP）。
package monitor

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"
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
		os:       detectOS(),
		ip:       localIP(),
		started:  time.Now(),
	}
}

// Hostname 返回主机名。
func (m *Monitor) Hostname() string { return m.hostname }

// OS 返回操作系统名称（如 "Arch Linux"）。
func (m *Monitor) OS() string { return m.os }

// IP 返回局域网 IPv4 地址。
func (m *Monitor) IP() string { return m.ip }

// Uptime 返回运行时间（如 "12 天 4 小时"）。
func (m *Monitor) Uptime() string {
	if secs, ok := linuxUptime(); ok {
		return formatDuration(time.Duration(secs) * time.Second)
	}
	return formatDuration(time.Since(m.started))
}

// linuxUptime 读取 /proc/uptime（Linux）。
func linuxUptime() (int64, bool) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, false
	}
	var secs float64
	if _, err := fmt.Sscanf(string(data), "%f", &secs); err != nil {
		return 0, false
	}
	return int64(secs), true
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

// detectOS 优先读取 /etc/os-release 的 PRETTY_NAME，否则回退到 GOOS+GOARCH。
func detectOS() string {
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
			}
		}
	}
	return runtime.GOOS + " " + runtime.GOARCH
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
