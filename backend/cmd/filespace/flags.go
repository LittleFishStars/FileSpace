package main

import (
	"flag"
	"fmt"
)

// options 命令行选项。
type options struct {
	configPath string
	port       int
	addCwd     bool // -a/--add：在解析出的共享目录之外额外共享当前目录
	thisOnly   bool // --this：仅共享当前目录
	showHelp   bool
}

// parseFlags 解析命令行参数（长名与短名别名指向同一变量，-x 与 --x 由 flag 包等价解析）。
func parseFlags() (*options, []string) {
	flag.Usage = usage
	opts := &options{}
	flag.StringVar(&opts.configPath, "config", "", "配置文件路径")
	flag.StringVar(&opts.configPath, "c", "", "配置文件路径（-c, --config 简写）")
	flag.IntVar(&opts.port, "port", 0, "监听端口（默认 8080）")
	flag.IntVar(&opts.port, "p", 0, "监听端口（-p, --port 简写）")
	flag.BoolVar(&opts.showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&opts.showHelp, "h", false, "显示帮助信息（-h, --help 简写）")
	flag.BoolVar(&opts.addCwd, "add", false, "额外共享当前文件夹")
	flag.BoolVar(&opts.addCwd, "a", false, "额外共享当前文件夹（-a, --add 简写）")
	flag.BoolVar(&opts.thisOnly, "this", false, "仅共享当前目录")
	flag.Parse()
	return opts, flag.Args()
}

// usage 打印命令行帮助信息（help 命令与 -h 共用）。
func usage() {
	fmt.Print(`filespace — 局域网文件共享工具

用法:
  filespace [选项] [目录...]

在任意文件夹执行 filespace 即可共享该文件夹，打开浏览器即可查看局域网内所有已共享的文件夹。

参数:
  [目录...]               要共享的文件夹（可多个）；缺省恢复上次退出前共享的目录，无记录时共享当前目录
  -a, --add               额外共享当前文件夹（在解析出的共享目录之外追加）
  --this                  仅共享当前目录（不恢复上次共享的目录）
  -c, --config <文件>     配置文件路径（YAML）
  -p, --port <端口>       监听端口（默认 8080）
  -h, --help              显示本帮助信息

命令:
  help                    显示本帮助信息

示例:
  filespace                        恢复上次共享的目录（无记录时共享当前目录）
  filespace -a                     恢复上次共享的目录，并额外共享当前目录
  filespace --this                 仅共享当前目录
  filespace ~/docs /mnt/data       共享多个目录
  filespace -p 9000 ~/docs         指定端口
  filespace -c config.yaml         使用配置文件

配置优先级: 命令行 -p > 配置文件 > 默认值; 目录参数覆盖配置文件中的 shared_folders; 两者都未指定时恢复上次退出前共享的目录，-a 可在任何情况下额外共享当前目录，--this 可仅共享当前目录。
检测到同端口已有后端在运行时，本进程仅支持用目录参数或 -a 追加，把目录交给已运行的后端后自动退出。
`)
}
