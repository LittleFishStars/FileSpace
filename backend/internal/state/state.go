// Package state 负责本地状态文件的读写（上次共享记录、运行锁）。
package state

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// lastSharedFile 状态文件名：记录最近一次退出前共享的目录，供下次未指定共享目录时恢复。
const lastSharedFile = "last-shared.yaml"

// lastShared 状态文件内容：仅持久化目录路径，名称由路径派生。
type lastShared struct {
	Shared []string `yaml:"shared"`
}

// dir 返回用户配置目录下 filespace/ 子目录。
func dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "filespace"), nil
}

// path 返回用户配置目录下 filespace/ 子目录中的状态文件路径。
func path(name string) (string, error) {
	dir, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// write 原子写入状态文件（先写临时文件再改名）。
func write(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// read 读取并解析状态文件。
func read(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, v)
}

// SaveLastShared 记录最近一次共享的目录列表。
func SaveLastShared(paths []string) error {
	path, err := path(lastSharedFile)
	if err != nil {
		return err
	}
	return write(path, lastShared{Shared: paths})
}

// LoadLastShared 读取上次共享的目录列表；无记录或文件损坏时返回空列表。
func LoadLastShared() []string {
	path, err := path(lastSharedFile)
	if err != nil {
		return nil
	}
	var st lastShared
	if err := read(path, &st); err != nil {
		return nil
	}
	return st.Shared
}
