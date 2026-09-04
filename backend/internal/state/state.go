// Package state 负责本地运行锁的读写。
package state

import (
	"os"
	"path/filepath"
)

// dir 返回用户配置目录下 filespace/ 子目录。
func dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "filespace"), nil
}
