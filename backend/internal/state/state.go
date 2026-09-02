// Package state 负责本地状态文件的读写（上次共享记录、运行锁）。
package state

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"filespace/internal/config"
)

// lastSharedFile 状态文件名：记录最近一次退出前共享的目录（含访问密码），
// 供下次未指定共享目录时恢复，保证重启后文件夹密码不丢失。
const lastSharedFile = "last-shared.yaml"

// lastShared 状态文件内容：路径 + 访问密码（空表示开放），名称由路径派生。
type lastShared struct {
	Shared []config.SharedFolder `yaml:"shared"`
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

// SaveLastShared 记录最近一次共享的目录（含各目录访问密码，空表示开放）。
func SaveLastShared(shared []config.SharedFolder) error {
	path, err := path(lastSharedFile)
	if err != nil {
		return err
	}
	return write(path, lastShared{Shared: shared})
}

// LoadLastShared 读取上次共享的目录（含访问密码）；无记录或文件损坏时返回空列表。
// 兼容旧版仅存路径列表（shared: [path...]）的状态文件：读取后按无密码处理。
func LoadLastShared() []config.SharedFolder {
	path, err := path(lastSharedFile)
	if err != nil {
		return nil
	}
	var st lastShared
	if err := read(path, &st); err == nil {
		return st.Shared
	}
	var legacy struct {
		Shared []string `yaml:"shared"`
	}
	if err := read(path, &legacy); err == nil {
		out := make([]config.SharedFolder, 0, len(legacy.Shared))
		for _, p := range legacy.Shared {
			out = append(out, config.SharedFolder{Path: p})
		}
		return out
	}
	return nil
}
