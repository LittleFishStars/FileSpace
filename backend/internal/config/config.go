// Package config 定义后端配置结构与加载逻辑。
package config

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
	// Passwd 明文访问密码（仅作输入语义：配置文件 / 命令行 / 旧版状态文件中的明文记录）。
	// 程序内部与持久化统一使用其 sha256 哈希（PasswdHash），明文不回存到任何状态文件。
	// 为空表示该文件夹开放（或密码由 PasswdHash 给出）。
	Passwd string `yaml:"passwd,omitempty" json:"passwd,omitempty"`
	// PasswdHash 访问密码的 sha256 十六进制哈希（程序内部与 last-shared.yaml 的持久化表示）。
	// 与 Passwd 同时存在时以 PasswdHash 为准；两者皆空表示对局域网开放。
	PasswdHash string `yaml:"passwd_hash,omitempty" json:"passwdHash,omitempty"`
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
	// Passwd 默认访问密码（-P/--passwd 或配置文件顶层 passwd）：
	// 应用于本节点所有未显式设置密码（shared_folders[].passwd 为空）的共享文件夹。
	// 为空表示默认不设密码（文件夹保持开放）。
	Passwd string `yaml:"passwd" json:"passwd"`
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
