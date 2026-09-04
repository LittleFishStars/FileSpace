// filespace 命令行入口：在某个文件夹下执行即可共享该文件夹。
package main

import (
	"errors"
	"log"

	"filespace/internal/state"
)

// main 解析参数、加载配置、获取运行锁（保持唯一后端）并启动服务。
func main() {
	opts, args := parseFlags()
	if opts.showHelp || (len(args) > 0 && args[0] == "help") {
		usage()
		return
	}
	// 位置参数已废弃：共享目录一律通过 -d/--dir 或配置文件 shared_folders 指定
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

	if opts.web {
		runServerWithWeb(cfg, configPath)
	} else {
		runServer(cfg, configPath)
	}
}
