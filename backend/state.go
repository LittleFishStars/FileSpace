package filespace

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// lastSharedFile 状态文件名：记录最近一次退出前共享的目录，
// 供下次未指定共享目录时恢复。
const lastSharedFile = "last-shared.yaml"

// lastShared 状态文件内容：仅持久化目录路径，名称由路径派生。
type lastShared struct {
	Shared []string `yaml:"shared"`
}

// lastSharedPath 返回状态文件路径（用户配置目录下 filespace/ 子目录）。
func lastSharedPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "filespace", lastSharedFile), nil
}

// SaveLastShared 记录最近一次共享的目录列表（先写临时文件再改名，原子落盘）。
func SaveLastShared(paths []string) error {
	path, err := lastSharedPath()
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(lastShared{Shared: paths})
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

// LoadLastShared 读取上次共享的目录列表；无记录或文件损坏时返回空列表。
func LoadLastShared() []string {
	path, err := lastSharedPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var st lastShared
	if err := yaml.Unmarshal(data, &st); err != nil {
		return nil
	}
	return st.Shared
}
