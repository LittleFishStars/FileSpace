package main

import (
	"fmt"
	"log"
	"strings"

	"filespace/internal/config"
)

// saveSettingsToConfig 把本次命令行参数设置追加保存到配置文件（--save）：
//   - -d/--dir 目录：合并（追加 + 去重）到 shared_folders，保留配置中已有条目与
//     每个文件夹显式设置的密码哈希；新目录做存在性校验（复用 mergeShared）；
//   - -P/--passwd（显式给出，含空值清除）：覆盖配置顶层的默认访问密码；
//   - -p/--port：覆盖监听端口（后两者已由 loadConfig 应用到 cfg，此处直接随文件写回）。
//
// 与运行期写回（persistConfig）的规则一致：文件夹密码只存 sha256 哈希，不存明文。
// 默认密码不在此处固化进各文件夹（保持其显式密码为空），而是以顶层 passwd 明文保存，
// 由下次启动的 ApplyDefaultPasswdHash 统一应用——这样日后修改/清除默认密码不会
// 因旧目录残留哈希而失效。
func saveSettingsToConfig(cfg *config.Config, configPath string, opts *options) {
	if len(opts.dirs) == 0 && !opts.hasPasswd && opts.port == 0 {
		log.Fatalf("--save 需要至少一个可保存的参数（-d/--dir <目录>、-P/--passwd <密码>、-p/--port <端口>）")
	}

	saved := make([]string, 0, 3)
	if len(opts.dirs) > 0 {
		before := len(cfg.Shared)
		cfg.Shared = mergeShared(cfg.Shared, opts.dirs)
		saved = append(saved, fmt.Sprintf("共享目录追加 %d 个（当前共 %d 个）", len(cfg.Shared)-before, len(cfg.Shared)))
	}
	if opts.hasPasswd {
		if opts.passwd == "" {
			saved = append(saved, "默认访问密码已清除")
		} else {
			saved = append(saved, "默认访问密码已更新")
		}
	}
	if opts.port != 0 {
		saved = append(saved, fmt.Sprintf("监听端口设为 %d", cfg.ListenPort))
	}

	if err := config.Save(configPath, cfg); err != nil {
		log.Fatalf("保存配置失败: %v", err)
	}
	fmt.Printf("✅ 已把本次参数设置保存到配置文件 %s：%s\n", configPath, strings.Join(saved, "；"))
}
