package filespace

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
