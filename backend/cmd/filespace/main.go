// filespace 命令行入口：在某个文件夹下执行即可共享该文件夹。
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// webRoot 定位前端静态资源，优先级：
//  1. 二进制同级目录下 web/
//  2. 当前工作目录下的 web/
func webRoot() string {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		if fi, err := os.Stat(filepath.Join(dir, "web")); err == nil && fi.IsDir() {
			return filepath.Join(dir, "web")
		}
	}
	return "./web"
}

func main() {
	fmt.Println("filespace: 局域网文件共享工具（骨架，待实现）")
	fmt.Printf("前端静态目录: %s\n", webRoot())
	// TODO: 启动 HTTP 服务，/api/* 提供后端接口，/ 提供 webRoot() 下的静态资源
}
