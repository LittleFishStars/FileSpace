// Package config 定义后端配置结构与加载逻辑。
package config

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SharedFolder 一个共享目录的配置。
type SharedFolder struct {
	Path string `yaml:"path" json:"path"`
	Name string `yaml:"name" json:"name"`
	// Passwd 明文访问密码（仅作输入语义：配置文件 / 命令行中的明文记录）。
	// 程序内部与持久化统一使用其 sha256 哈希（PasswdHash），明文不回存到任何文件。
	// 为空表示该文件夹开放（或密码由 PasswdHash 给出）。
	Passwd string `yaml:"passwd,omitempty" json:"passwd,omitempty"`
	// PasswdHash 访问密码的 sha256 十六进制哈希（程序内部与配置文件的持久化表示）。
	// 与 Passwd 同时存在时以 PasswdHash 为准；两者皆空表示对局域网开放。
	PasswdHash string `yaml:"passwd_hash,omitempty" json:"passwdHash,omitempty"`
}

// DiscoveryConfig mDNS 发现配置。
type DiscoveryConfig struct {
	ServiceName string `yaml:"service_name" json:"serviceName"`
	Domain      string `yaml:"domain" json:"domain"`
}

// Config 后端配置。
type Config struct {
	ListenPort int             `yaml:"port" json:"port"`
	Shared     []SharedFolder  `yaml:"shared_folders" json:"sharedFolders"`
	Discovery  DiscoveryConfig `yaml:"discovery" json:"discovery"`
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

// ConfigDir 返回用户配置目录下的 filespace/ 子目录
// （默认配置文件与运行锁文件所在处，如 Linux ~/.config/filespace/）。
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "filespace"), nil
}

// DefaultConfigPath 返回默认配置文件路径（用户配置目录下 filespace/config.yaml）。
func DefaultConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// defaultConfigBody 默认配置文件模板：首次无参数运行生成。
// 后续程序写回配置（本机管理页增删共享/改密、命令行 --save）时全量重写该文件，手工注释会丢失。
const defaultConfigBody = `# filespace 配置文件
# 首次无参数运行自动生成；修改后重启生效。
# 本机管理页（/local）中添加/移除共享、修改密码时程序会同步更新本文件。
# 命令行可用 -d/--dir <目录> 临时追加共享文件夹、-P/--passwd 提供默认访问密码；
# 加 --save 可把本次命令行参数设置（目录追加/默认密码/端口）保存到本文件（下次启动即生效）。

# HTTP 监听端口
port: 8080

# 默认访问密码：应用于未显式设置密码（shared_folders[].passwd）的共享文件夹。
# 为空表示默认不设密码（文件夹保持开放）
passwd: ""

# 共享目录列表：在此列出要共享的文件夹；也可用命令行 -d/--dir 追加
shared_folders: []

# mDNS 发现配置
discovery:
  service_name: "_filespace._tcp"
  domain: "local."
`

// EnsureDefaultConfig 确保默认配置文件存在：已存在则原样保留，
// 不存在则创建带注释的默认配置模板。
func EnsureDefaultConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigBody), 0o644)
}

// Save 把配置原子写回文件（先写临时文件再改名）。
// 供运行中共享列表变更（UI 增删/改密）后持久化使用；shared_folders 仅存密码哈希，
// 明文密码在写回前统一转为哈希（见 cmd/filespace 的 persistConfig）。
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NodeID 根据主机名生成稳定的节点 ID（重启不变，供 mDNS 去重识别）。
func NodeID(hostname string) string {
	sum := sha1.Sum([]byte(hostname))
	return hex.EncodeToString(sum[:6])
}
