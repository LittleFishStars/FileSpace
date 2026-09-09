// Package pathutil 提供跨内部包复用的路径工具。
// 目前只有目录包含（越界）校验 Within：share（目录树路径解析）与 sync
// （本地镜像路径映射）都需要「拼接后不越出根目录」的判定，此前各自维护一份
// 完全相同的实现，现统一收敛到此基础包（只依赖标准库，稳定基础层）。
package pathutil

import (
	"path/filepath"
	"strings"
)

// Within 判断 child 是否位于 parent 目录内。
func Within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
