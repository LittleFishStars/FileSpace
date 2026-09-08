package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"filespace/internal/sync"
)

// bindSyncLocals 解析 -s/--sync 与位置参数的配对：
//   - 逐个 -s 值经 sync.ParseRemote 解析为远程定位符
//     （<ip>:<文件夹id> 或 <ip>:<端口>:<文件夹id>，IPv6 方括号包裹），
//     若同时给出了位置参数（与 -s 个数一致），作为该同步项的本地路径；
//   - 未给位置参数时，本地路径默认派生自远程信息：<本地工作目录>/sync-<远程ip>-<文件夹id>，
//     避免用户必须显式写本地路径；
//   - -s 与位置参数个数不一致时报错；无 -s 时不允许出现任何位置参数（保持原有约束）。
//
// 结果写入 opts.syncSpecs（原始串）与 opts.syncLocal（本地路径）。
func bindSyncLocals(opts *options, args []string) error {
	n := len(opts.syncSpecs)
	if n == 0 {
		if len(args) > 0 {
			return fmt.Errorf("不支持位置参数，请改用 -d/--dir 指定要共享的文件夹，或用 -s/--sync <远程ip>:<文件夹id> <本地路径> 创建同步文件夹: %v", args)
		}
		return nil
	}
	if len(args) > n {
		return fmt.Errorf("位置参数个数（%d）超过 -s/--sync 个数（%d），多余的无法配对", len(args), n)
	}
	if len(args) != 0 && len(args) != n {
		return fmt.Errorf("-s/--sync 指定了 %d 个，但位置参数给出了 %d 个：需一一对应或全不给出", n, len(args))
	}
	// 逐个解析 -s 值（校验格式），本地路径为空时按默认规则派生
	opts.syncLocal = make([]string, n)
	for i, raw := range opts.syncSpecs {
		if _, err := sync.ParseRemote(raw); err != nil {
			return fmt.Errorf("同步定位符无效（第 %d 个）: %v", i+1, err)
		}
		if i < len(args) {
			opts.syncLocal[i] = args[i]
		} else {
			opts.syncLocal[i] = defaultSyncLocal(raw)
		}
	}
	return nil
}

// defaultSyncLocal 为未显式给出本地路径的 -s 派生默认本地同步目录：
// 当前工作目录下的 "sync-<远程ip>-<文件夹id>"（忽略端口差异，含端口时附加）。
func defaultSyncLocal(specStr string) string {
	parsed, err := sync.ParseRemote(specStr)
	if err != nil {
		return "" // 已在 bindSyncLocals 校验过，理论不可达
	}
	host := parsed.Host
	// IPv6 去掉冒号作为目录名一部分
	host = strings.ReplaceAll(host, ":", "-")
	name := fmt.Sprintf("sync-%s-%s", host, parsed.FolderID)
	wd, _ := os.Getwd()
	return filepath.Join(wd, name)
}

// buildSyncSpecs 把命令行 -s/--sync 转换为完整的 sync.Spec 列表（含默认密码
// 填充与本地路径），供启动阶段交由 sync.Manager 注册。
func buildSyncSpecs(opts *options, passwd string) []sync.Spec {
	specs := make([]sync.Spec, 0, len(opts.syncSpecs))
	for i, raw := range opts.syncSpecs {
		s, err := sync.ParseRemote(raw)
		if err != nil {
			log.Fatalf("同步定位符无效: %v", err) // bindSyncLocals 已校验，这里兜底
		}
		// 未显式给本地路径时用派生默认
		local := ""
		if i < len(opts.syncLocal) {
			local = opts.syncLocal[i]
		}
		if local == "" {
			local = defaultSyncLocal(raw)
		}
		// 同步访问密码：未单独指定时用整个进程的默认密码（-P/--passwd 或配置顶层 passwd），
		// 与共享目录共享同一套密码语义（该文件夹设置了密码时生效）。
		s.Local = local
		s.Passwd = passwd
		specs = append(specs, s)
	}
	return specs
}
