package main

import (
	"log"

	"filespace/internal/config"
)

// loadConfig 按优先级加载配置：命令行 -c 指定的文件 > 默认值，-p 覆盖监听端口，-P 覆盖访问密码。
func loadConfig(opts *options) *config.Config {
	cfg := config.DefaultConfig()
	if opts.configPath != "" {
		loaded, err := config.Load(opts.configPath)
		if err != nil {
			log.Fatalf("读取配置失败: %v", err)
		}
		cfg = loaded
	}
	if opts.port != 0 {
		cfg.ListenPort = opts.port
	}
	if opts.passwd != "" {
		cfg.Passwd = opts.passwd
	}
	return cfg
}
