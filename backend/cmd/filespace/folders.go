package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"filespace"
)

// resolveSharedFolders 解析最终共享目录列表，优先级：
// 目录参数 > 配置文件 shared_folders > 上次共享记录 > 当前目录；
// --this 仅共享当前目录（跳过恢复），-a 在结果上额外追加当前目录。
func resolveSharedFolders(cfg *filespace.Config, opts *options, args []string) {
	if opts.thisOnly {
		useThisOnly(cfg, args)
	}
	if len(args) > 0 {
		cfg.Shared = sharedFromPaths(args)
	}
	if len(cfg.Shared) == 0 {
		restoreOrCurrent(cfg)
	}
	if opts.addCwd {
		addCurrentDir(cfg)
	}
}

// useThisOnly 处理 --this：仅共享当前目录，与目录参数 / 配置文件中的 shared_folders 互斥。
func useThisOnly(cfg *filespace.Config, args []string) {
	if len(args) > 0 {
		log.Fatalf("不能同时使用 --this 与目录参数")
	}
	if len(cfg.Shared) > 0 {
		log.Fatalf("不能同时使用 --this 与配置文件中的 shared_folders")
	}
	cwd, _ := os.Getwd()
	cfg.Shared = sharedFromPaths([]string{cwd})
	fmt.Printf("已指定 --this，仅共享当前目录: %s\n", cwd)
}

// sharedFromPaths 由路径列表构造共享目录配置（名称取路径基名）。
func sharedFromPaths(paths []string) []filespace.SharedFolder {
	shared := make([]filespace.SharedFolder, 0, len(paths))
	for _, p := range paths {
		shared = append(shared, filespace.SharedFolder{Path: p, Name: filepath.Base(p)})
	}
	return shared
}

// restoreOrCurrent 未指定任何共享目录：优先恢复上次退出前共享的目录，无记录时回退到当前目录。
func restoreOrCurrent(cfg *filespace.Config) {
	if last := filespace.LoadLastShared(); len(last) > 0 {
		cfg.Shared = sharedFromPaths(last)
		fmt.Printf("未指定共享目录，恢复上次共享的目录: %s\n", strings.Join(last, ", "))
		return
	}
	cwd, _ := os.Getwd()
	cfg.Shared = sharedFromPaths([]string{cwd})
	fmt.Printf("未指定共享目录，默认共享当前目录: %s\n", cwd)
}

// addCurrentDir 处理 -a/--add：在解析结果基础上额外共享当前目录（已在列表中则跳过）。
func addCurrentDir(cfg *filespace.Config) {
	cwd, _ := os.Getwd()
	if sharedContains(cfg.Shared, cwd) {
		fmt.Printf("已指定 -a，当前目录已在共享列表中: %s\n", cwd)
		return
	}
	cfg.Shared = append(cfg.Shared, filespace.SharedFolder{Path: cwd, Name: filepath.Base(cwd)})
	fmt.Printf("已指定 -a，额外共享当前目录: %s\n", cwd)
}

// sharedContains 判断共享列表中是否已包含指定路径。
func sharedContains(shared []filespace.SharedFolder, path string) bool {
	for _, f := range shared {
		if f.Path == path {
			return true
		}
	}
	return false
}
