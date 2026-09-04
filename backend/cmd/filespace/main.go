// filespace 命令行入口：启动局域网文件共享后端（默认读取用户配置文件）。
package main

import (
	"errors"
	"log"

	"filespace/internal/state"
)

// main 解析参数、加载配置、获取运行锁（保持唯一后端）并启动服务。
func main() {
	opts, args := parseFlags()
	if opts.showHelp {
		usage()
		return
	}
	// 共享目录只能通过 -d/--dir 或配置文件 shared_folders 指定，不接受位置参数
	if len(args) > 0 {
		log.Fatalf("不支持位置参数，请改用 -d/--dir 指定要共享的文件夹: %v", args)
	}
	cfg, configPath := loadConfig(opts)

	// 运行锁：用锁文件标识是否已有后端在运行（含运行在其他端口的实例）
	lock, existing, err := state.AcquireRunningLock(cfg.ListenPort)
	if err != nil {
		log.Fatalf("获取运行锁失败: %v", err)
	}
	if existing > 0 {
		handoffToExisting(existing, opts)
		return
	}
	defer lock.Release()

	// 兜底：锁文件缺失但同端口已有后端（如锁文件被误删）
	if err := probeBackend(cfg.ListenPort); err != nil {
		if errors.Is(err, state.ErrBackendRunning) {
			handoffToExisting(cfg.ListenPort, opts)
			return
		}
		log.Fatalf("端口 %d 不可用: %v", cfg.ListenPort, err)
	}

	resolveSharedFolders(cfg, opts.dirs)

	// 以是否托管前端界面（--web）为唯一差异启动后端服务
	runServer(cfg, configPath, opts.web)
}
