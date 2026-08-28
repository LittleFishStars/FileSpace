package filespace

import (
	"crypto/sha1"
	"encoding/hex"
	"os"

	"gopkg.in/yaml.v3"
)

// SharedFolder 一个共享目录的配置。
type SharedFolder struct {
	Path string `yaml:"path" json:"path"`
	Name string `yaml:"name" json:"name"`
}

// DiscoveryConfig mDNS 发现配置。
type DiscoveryConfig struct {
	ServiceName string `yaml:"service_name" json:"serviceName"`
	Domain      string `yaml:"domain" json:"domain"`
}

// MonitorConfig 系统状态采集配置。
type MonitorConfig struct {
	IntervalSec int `yaml:"interval_sec" json:"intervalSec"`
}

// Config 后端配置。
type Config struct {
	ListenPort int             `yaml:"port" json:"port"`
	NodeName   string          `yaml:"name" json:"name"`
	Shared     []SharedFolder  `yaml:"shared_folders" json:"sharedFolders"`
	Discovery  DiscoveryConfig `yaml:"discovery" json:"discovery"`
	Monitor    MonitorConfig   `yaml:"monitor" json:"monitor"`
}

// DefaultConfig 返回默认配置。
func DefaultConfig() *Config {
	return &Config{
		ListenPort: 8080,
		Discovery: DiscoveryConfig{
			ServiceName: "_filespace._tcp",
			Domain:      "local.",
		},
		Monitor: MonitorConfig{IntervalSec: 10},
	}
}

// Load 从 YAML 文件读取配置，缺失字段回退到默认值。
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 8080
	}
	if cfg.Discovery.ServiceName == "" {
		cfg.Discovery.ServiceName = "_filespace._tcp"
	}
	if cfg.Discovery.Domain == "" {
		cfg.Discovery.Domain = "local."
	}
	return cfg, nil
}

// NodeID 根据主机名生成稳定的节点 ID（重启不变，供 mDNS 去重识别）。
func NodeID(hostname string) string {
	sum := sha1.Sum([]byte(hostname))
	return hex.EncodeToString(sum[:6])
}
