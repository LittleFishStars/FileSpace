// filespace-web 前端程序：托管前端界面（web/ 静态资源），并负责后端生命周期。
//
// 启动流程：
//  1. 读取后端锁文件（用户配置目录 filespace/lock）获取后端端口；
//  2. 锁文件缺失、内容无效或端口无响应（崩溃残留）→ 自动拉起一个后端
//     （filespace -p <端口>，工作目录为当前目录，与直接执行 filespace 行为一致），
//     轮询等待其就绪；
//  3. 监听前端端口（默认 8080，被占用时自动顺延），把 /api/* 请求反向代理给后端。
//
// 若本进程拉起了后端，退出时（Ctrl+C / kill）会一并通知后端优雅退出；
// 复用已有后端时不影响其生命周期。
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
)

// options 命令行选项。
type options struct {
	port     int // 前端监听端口（默认 8080）
	showHelp bool
}

// parseFlags 解析命令行参数（-x 与 --x 由 flag 包等价解析）。
func parseFlags() *options {
	flag.Usage = usage
	opts := &options{}
	flag.IntVar(&opts.port, "port", 0, "前端监听端口（默认 8080，被占用时自动顺延）")
	flag.IntVar(&opts.port, "p", 0, "前端监听端口（-p, --port 简写）")
	flag.BoolVar(&opts.showHelp, "help", false, "显示帮助信息")
	flag.BoolVar(&opts.showHelp, "h", false, "显示帮助信息（-h, --help 简写）")
	flag.Parse()
	return opts
}

// usage 打印命令行帮助信息。
func usage() {
	fmt.Print(`filespace-web — 文件空间前端程序（界面托管 + 后端自动拉起）

用法:
  filespace-web [选项]

在任意文件夹执行 filespace-web 即可共享该文件夹：程序读取后端锁文件获取端口，
若后端尚未启动则自动拉起一个后端，并在浏览器中打开前端界面。

参数:
  -p, --port <端口>       前端监听端口（默认 8080；被占用时自动顺延）
  -h, --help              显示本帮助信息

示例:
  filespace-web                   在 8080 端口打开界面（必要时自动启动后端）
  filespace-web -p 9000           指定前端端口

后端程序 filespace（P2P + mDNS + 文件共享 API）可单独运行；已手动运行后端时，
filespace-web 会通过锁文件自动复用其端口，不会重复启动。
` + "\n")
}

func main() {
	opts := parseFlags()
	if opts.showHelp {
		usage()
		return
	}

	// 1) 定位后端：读锁文件，未启动则拉起
	backendPort, spawned, err := ensureBackend()
	if err != nil {
		log.Fatalf("启动后端失败: %v", err)
	}

	// 2) 前端监听：默认 8080，被占用（含后端已占用）时顺延找空闲端口
	ln, listenPort, err := listenAt(opts.port)
	if err != nil {
		log.Fatalf("前端端口不可用: %v", err)
	}
	if opts.port != 0 && listenPort != opts.port {
		fmt.Printf("⚠️ 端口 %d 已被占用，前端改听端口 %d\n", opts.port, listenPort)
	}
	fmt.Printf("后端端口: %d%s\n", backendPort, noteSpawned(spawned))

	// 3) 启动前端服务（静态资源 + /api 反代），等待退出信号
	runWeb(ln, listenPort, backendPort, spawned)
}

// noteSpawned 返回后端来源说明（本进程拉起 / 复用已有）。
func noteSpawned(spawned *os.Process) string {
	if spawned != nil {
		return "（本进程自动拉起）"
	}
	return "（复用已有实例，见锁文件）"
}

// listenAt 从候选端口（0 表示默认 8080）开始寻找空闲端口并返回监听器；
// 端口被占用时依次顺延（最多 20 个），全部被占时报错。
func listenAt(prefer int) (net.Listener, int, error) {
	start := prefer
	if start == 0 {
		start = 8080
	}
	for port := start; port < start+20; port++ {
		ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err == nil {
			return ln, port, nil
		}
	}
	return nil, 0, fmt.Errorf("端口 %d-%d 均被占用", start, start+19)
}
