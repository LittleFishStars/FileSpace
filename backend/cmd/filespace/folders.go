package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"filespace/internal/config"
	"filespace/internal/state"
)

// resolveSharedFolders 解析最终共享目录列表，优先级：
// 目录参数 > 配置文件 shared_folders > 上次共享记录 > 当前目录；
// --this 仅共享当前目录（跳过恢复），-a 在结果上额外追加当前目录。
// 最后用默认密码（-P/--passwd 或配置 passwd）填充未显式设置密码的文件夹。
func resolveSharedFolders(cfg *config.Config, opts *options, args []string) {
	if opts.thisOnly {
		useThisOnly(cfg, args, opts.passwd)
	}
	if len(args) > 0 {
		cfg.Shared = sharedFromPaths(args, opts.passwd)
	}
	if len(cfg.Shared) == 0 {
		restoreOrCurrent(cfg, opts.passwd)
	}
	if opts.addCwd {
		addCurrentDir(cfg, opts.passwd)
	}
	applyDefaultPasswd(cfg, opts.passwd)
}

// useThisOnly 处理 --this：仅共享当前目录，与目录参数 / 配置文件中的 shared_folders 互斥。
func useThisOnly(cfg *config.Config, args []string, passwd string) {
	if len(args) > 0 {
		log.Fatalf("不能同时使用 --this 与目录参数")
	}
	if len(cfg.Shared) > 0 {
		log.Fatalf("不能同时使用 --this 与配置文件中的 shared_folders")
	}
	cwd, _ := os.Getwd()
	cfg.Shared = sharedFromPaths([]string{cwd}, passwd)
	fmt.Printf("已指定 --this，仅共享当前目录: %s\n", cwd)
}

// sharedFromPaths 由路径列表构造共享目录配置（名称取路径基名，passwd 为默认访问密码）。
func sharedFromPaths(paths []string, passwd string) []config.SharedFolder {
	shared := make([]config.SharedFolder, 0, len(paths))
	for _, p := range paths {
		shared = append(shared, config.SharedFolder{Path: p, Name: filepath.Base(p), Passwd: passwd})
	}
	return shared
}

// restoreOrCurrent 未指定任何共享目录：优先恢复上次退出前共享的目录（含各自访问密码），
// 无记录时回退到当前目录。本次 -P/--passwd 作为默认密码仅覆盖恢复记录中未显式设密的文件夹。
func restoreOrCurrent(cfg *config.Config, passwd string) {
	if last := state.LoadLastShared(); len(last) > 0 {
		cfg.Shared = last
		applyDefaultPasswd(cfg, passwd)
		paths := make([]string, 0, len(last))
		for _, f := range last {
			paths = append(paths, f.Path)
		}
		fmt.Printf("未指定共享目录，恢复上次共享的目录: %s\n", strings.Join(paths, ", "))
		return
	}
	cwd, _ := os.Getwd()
	cfg.Shared = sharedFromPaths([]string{cwd}, passwd)
	fmt.Printf("未指定共享目录，默认共享当前目录: %s\n", cwd)
}

// addCurrentDir 处理 -a/--add：在解析结果基础上额外共享当前目录（已在列表中则跳过）。
func addCurrentDir(cfg *config.Config, passwd string) {
	cwd, _ := os.Getwd()
	if sharedContains(cfg.Shared, cwd) {
		fmt.Printf("已指定 -a，当前目录已在共享列表中: %s\n", cwd)
		return
	}
	cfg.Shared = append(cfg.Shared, config.SharedFolder{Path: cwd, Name: filepath.Base(cwd), Passwd: passwd})
	fmt.Printf("已指定 -a，额外共享当前目录: %s\n", cwd)
}

// applyDefaultPasswd 用默认密码填充未显式设置密码（shared_folders[].passwd 为空）的文件夹，
// 不覆盖配置文件中为单个文件夹指定的独立密码。
func applyDefaultPasswd(cfg *config.Config, passwd string) {
	if passwd == "" {
		return
	}
	for i := range cfg.Shared {
		if cfg.Shared[i].Passwd == "" {
			cfg.Shared[i].Passwd = passwd
		}
	}
}

// sharedContains 判断共享列表中是否已包含指定路径。
func sharedContains(shared []config.SharedFolder, path string) bool {
	for _, f := range shared {
		if f.Path == path {
			return true
		}
	}
	return false
}
