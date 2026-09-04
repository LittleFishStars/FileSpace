package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filespace/internal/config"
	"filespace/internal/share"
)

// resolveSharedFolders 解析最终共享目录列表：
// 配置文件 shared_folders 为基，-d/--dir 指定的目录追加到其后（重复路径自动去重）；
// 最后用默认密码（cfg.Passwd，已合并 -P/--passwd 覆盖）填充未显式设置密码的文件夹。
func resolveSharedFolders(cfg *config.Config, dirs []string) {
	if len(dirs) > 0 {
		cfg.Shared = mergeShared(cfg.Shared, dirs)
	}
	share.ApplyDefaultPasswdHash(cfg.Shared, cfg.Passwd)
	if len(cfg.Shared) > 0 {
		paths := make([]string, 0, len(cfg.Shared))
		for _, f := range cfg.Shared {
			paths = append(paths, f.Path)
		}
		fmt.Printf("📂 共享 %d 个目录: %s\n", len(cfg.Shared), strings.Join(paths, ", "))
	} else {
		fmt.Println("📂 未配置共享目录，后端仅提供 API（用 -d/--dir 或配置文件 shared_folders 添加）")
	}
}

// sharedFromDir 校验单个 -d/--dir 目录并构造共享配置（名称取路径基名）。
func sharedFromDir(path string) (config.SharedFolder, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return config.SharedFolder{}, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return config.SharedFolder{}, fmt.Errorf("目录不可访问: %v", err)
	}
	if !fi.IsDir() {
		return config.SharedFolder{}, fmt.Errorf("不是目录: %s", abs)
	}
	return config.SharedFolder{Path: abs, Name: filepath.Base(abs)}, nil
}

// mergeShared 把 -d/--dir 目录追加到配置文件的共享列表并去重。
// 去重同时考虑精确路径与符号链接解析后的真实路径（复用 share.RealPath，与运行时 Add 同口径）；
// -d 指定的目录做存在性校验，配置文件中的路径不校验（允许临时失效的目录继续占位）。
func mergeShared(shared []config.SharedFolder, dirs []string) []config.SharedFolder {
	seen := make(map[string]bool, len(shared)+len(dirs))
	keep := func(p string) bool {
		key := share.RealPath(p)
		if seen[key] {
			return false
		}
		seen[key] = true
		return true
	}
	out := make([]config.SharedFolder, 0, len(shared)+len(dirs))
	for _, f := range shared {
		if keep(f.Path) {
			out = append(out, f)
		}
	}
	for _, p := range dirs {
		sf, err := sharedFromDir(p)
		if err != nil {
			log.Fatalf("共享目录无效: %v", err)
		}
		if !keep(sf.Path) {
			fmt.Printf("已跳过重复共享目录: %s\n", sf.Path)
			continue
		}
		out = append(out, sf)
	}
	return out
}

// canonicalSharedFolders 对共享列表按路径排序并返回其规范化快照，
// 供与当前配置对比（排序使 YAML 输出稳定，避免无意义抖动）。
func canonicalSharedFolders(shared []config.SharedFolder) []config.SharedFolder {
	out := make([]config.SharedFolder, len(shared))
	copy(out, shared)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
