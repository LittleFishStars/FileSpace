package main

import (
	"log"

	"filespace/internal/config"
)

// loadConfig 加载配置并返回其文件路径：
//   - 未指定 -c/--config 时使用默认配置文件（用户配置目录 filespace/config.yaml），
//     文件不存在则自动创建带注释的模板（无参数启动完成初始化）；
//   - -p 覆盖监听端口；-P/--passwd（含空值清除）覆盖配置文件顶层的默认密码。
//
// 返回的 configPath 供运行中共享列表变更后写回（见 api persistConfig）。
func loadConfig(opts *options) (*config.Config, string) {
	path := opts.configPath
	if path == "" {
		dp, err := config.DefaultConfigPath()
		if err != nil {
			log.Fatalf("获取默认配置路径失败: %v", err)
		}
		path = dp
		// 默认配置文件不存在时创建模板；显式 -c 指定的文件缺失则由 Load 报错
		if err := config.EnsureDefaultConfig(path); err != nil {
			log.Fatalf("创建默认配置文件失败: %v", err)
		}
	}
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	if opts.port != 0 {
		cfg.ListenPort = opts.port
	}
	// -P/--passwd 显式给出（含空值）时覆盖配置文件顶层的默认密码
	if opts.hasPasswd {
		cfg.Passwd = opts.passwd
	}
	return cfg, path
}
