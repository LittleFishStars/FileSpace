package main

import (
	"flag"
	"fmt"
)

// options 命令行选项。
type options struct {
	configPath string // -c/--config：配置文件路径（默认取用户配置目录下的 config.yaml）
	dirs       []string
	port       int
	passwd     string // -P/--passwd：共享访问密码
	hasPasswd  bool   // 是否显式给出了 -P/--passwd（含空值，空值表示移除密码）
	web        bool   // --web：同时启动前端界面（静态导出）并在浏览器中打开
	showHelp   bool
}

// dirFlags 多次出现的 -d/--dir 收集器。
type dirFlags []string

func (d *dirFlags) String() string { return fmt.Sprint([]string(*d)) }

func (d *dirFlags) Set(v string) error {
	*d = append(*d, v)
	return nil
}

// parseFlags 解析命令行参数（长名与短名别名指向同一变量，-x 与 --x 由 flag 包等价解析）。
func parseFlags() (*options, []string) {
	flag.Usage = usage
	opts := &options{}
	flag.StringVar(&opts.configPath, "config", "", "配置文件路径（默认 <用户配置目录>/filespace/config.yaml）")
	flag.StringVar(&opts.configPath, "c", "", "配置文件路径（-c, --config 简写）")
	flag.Var((*dirFlags)(&opts.dirs), "dir", "要共享的文件夹，可多次指定（-d, --dir）")
	flag.Var((*dirFlags)(&opts.dirs), "d", "要共享的文件夹（-d, --dir 简写），可多次指定")
	flag.IntVar(&opts.port, "port", 0, "监听端口（默认 8080）")
	flag.IntVar(&opts.port, "p", 0, "监听端口（-p, --port 简写）")
	flag.StringVar(&opts.passwd, "passwd", "", "共享访问密码")
	flag.StringVar(&opts.passwd, "P", "", "共享访问密码（-P, --passwd 简写）")
	flag.BoolVar(&opts.showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&opts.showHelp, "h", false, "显示帮助信息（-h, --help 简写）")
	flag.BoolVar(&opts.web, "web", false, "同时启动前端界面并在浏览器中打开")
	flag.Parse()
	// 检测是否显式给出 -P/--passwd：空值（-P ''）表示「移除密码」，需与「未提供」区分
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "passwd" || f.Name == "P" {
			opts.hasPasswd = true
		}
	})
	return opts, flag.Args()
}

// usage 打印命令行帮助信息（help 命令与 -h 共用）。
func usage() {
	fmt.Print(`filespace — 文件空间后端（局域网文件共享 API + 前端托管）

用法:
  filespace [选项]

无参数运行：读取用户配置目录下的默认配置文件（filespace/config.yaml，
不存在则自动创建带注释的模板），按其 shared_folders 共享文件夹，不共享当前目录。
用 -d/--dir 可在配置之外临时追加共享文件夹。

参数:
  -d, --dir <目录>        要共享的文件夹，可多次指定（在配置文件 shared_folders 之外追加）
  --web                   同时启动前端界面并在浏览器中打开（默认只启动后端 API）
  -c, --config <文件>     配置文件路径（YAML，默认 <用户配置目录>/filespace/config.yaml）
  -p, --port <端口>       监听端口（默认 8080）
  -P, --passwd <密码>     访问密码。两种情形：
                           · 本进程作为后端启动时：作为默认密码，应用于本次共享的文件夹
                             （未显式设置密码的），其他节点需输入密码才能查看/下载；本机不受影响
                           · 已有后端在运行时：需与 -d/--dir 配合，修改该目录的访问密码
                             （传空值 -P '' 表示移除密码）；目录未共享时按「新增共享并设密码」处理
                           也可在 web 端添加/管理共享时按文件夹单独设置密码
  -h, --help              显示本帮助信息

命令:
  help                    显示本帮助信息

示例:
  filespace                        读取/创建默认配置文件并按配置共享（无共享目录则不共享）
  filespace --web                  启动后端 + 前端界面，并在浏览器中打开
  filespace --web -p 9000          指定端口并启动前端
  filespace -d ~/docs              按默认配置共享，并额外共享 ~/docs
  filespace -d ~/docs -d /mnt/data 额外共享多个目录
  filespace -c config.yaml         使用指定配置文件
  filespace -P secret -d ~/docs    设置共享访问密码，并共享 ~/docs
  filespace -d ~/docs -P newpass   已有后端运行时：修改共享目录 ~/docs 的访问密码
  filespace -d ~/docs -P ''        已有后端运行时：移除 ~/docs 的访问密码（恢复开放）

配置优先级: 命令行 -p > 配置文件 > 默认值; -d 指定的目录追加到配置文件的 shared_folders
之后（重复路径自动去重）。无参数且配置未列共享目录时，后端启动但不共享任何文件夹。
检测到本机已有 filespace 后端在运行时（含运行在其他端口），本进程仅支持用 -d 追加、
或与 -P 配合修改目录密码，把操作交给已运行的后端后自动退出。
`)
}
