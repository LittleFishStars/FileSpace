package main

import (
	"log"

	"filespace"
)

// loadConfig 按优先级加载配置：命令行 -c 指定的文件 > 默认值，-p 覆盖监听端口。
func loadConfig(opts *options) *filespace.Config {
	cfg := filespace.DefaultConfig()
	if opts.configPath != "" {
		loaded, err := filespace.Load(opts.configPath)
		if err != nil {
			log.Fatalf("读取配置失败: %v", err)
		}
		cfg = loaded
	}
	if opts.port != 0 {
		cfg.ListenPort = opts.port
	}
	return cfg
}
