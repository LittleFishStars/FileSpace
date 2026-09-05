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
	// --save：先把本次命令行参数设置（目录/密码/端口）追加保存到配置文件，
	// 再按原流程运行（无运行后端则启动；已有后端则交给它），保证持久化与运行一致
	if opts.save {
		saveSettingsToConfig(cfg, configPath, opts)
	}

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

	startupDirs := opts.dirs
	if opts.save {
		// --save 已把 -d/--dir 目录合并进 cfg.Shared，启动路径无需重复合并
		startupDirs = nil
	}
	resolveSharedFolders(cfg, startupDirs)

	// 以是否托管前端界面（--web）为唯一差异启动后端服务
	runServer(cfg, configPath, opts.web)
}
