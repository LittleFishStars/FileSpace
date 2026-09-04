// Package share 管理本节点共享的目录：注册表（增删查）、访问密码、
// 以及「缓存 + 后台异步扫描」的目录统计。密码哈希与访问令牌契约委托给
// internal/auth 包（单一事实来源），本包只保存每个文件夹的密码哈希。
package share

import (
	"encoding/hex"
	"errors"
	"hash/fnv"
	"path/filepath"

	"filespace/internal/auth"
)

var (
	// ErrFolderNotFound 共享目录不存在。
	ErrFolderNotFound = errors.New("共享目录不存在")
	// ErrPathForbidden 路径超出共享目录范围。
	ErrPathForbidden = errors.New("路径超出共享目录范围")
	// ErrFolderExists 目录已在共享列表中。
	ErrFolderExists = errors.New("目录已在共享列表中")
	// ErrNotDirectory 路径不是目录。
	ErrNotDirectory = errors.New("路径不是目录")
)

// Folder 一个共享目录。
type Folder struct {
	ID   string
	Name string
	Path string
	// PasswdHash 该文件夹访问密码的 sha256 哈希（见 auth.Hash）：为零值表示对局域网开放；
	// 非零表示其他节点需先认证（换取访问令牌）才能查看/下载内容，本机回环访问不受影响。
	// 程序内不保存明文密码，设置密码的入口（配置 / CLI / API）统一先哈希再入库。
	PasswdHash auth.Hash
	// RealPath 解析符号链接后的真实路径，仅用于内部去重（识别指向同一目录的重复添加）。
	RealPath string
}

// RealPath 返回路径解析符号链接后的真实路径；解析失败时原样返回（去重退化为精确匹配）。
// 导出供启动阶段（cmd）对配置文件与 -d 目录做同样的去重口径。
func RealPath(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// folderID 根据路径生成稳定的目录 ID。
func folderID(path string) string {
	h := fnv.New32a()
	h.Write([]byte(path))
	return hex.EncodeToString(h.Sum(nil))
}
