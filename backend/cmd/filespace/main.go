// filespace 命令行入口：在某个文件夹下执行即可共享该文件夹。
package main

// main 解析参数、加载配置、解析共享目录并启动服务。
func main() {
	opts, args := parseFlags()
	if opts.showHelp || (len(args) > 0 && args[0] == "help") {
		usage()
		return
	}
	cfg := loadConfig(opts)
	if maybeHandoffToExisting(cfg, opts, args) {
		return
	}
	resolveSharedFolders(cfg, opts, args)
	runServer(cfg)
}
