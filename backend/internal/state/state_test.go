package state

import (
	"os"
	"path/filepath"
	"testing"

	"filespace/internal/config"
)

// TestLastSharedRoundtrip 新格式（路径 + 密码）读写一致：密码随记录持久化。
func TestLastSharedRoundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // Linux os.UserConfigDir 读取该变量
	in := []config.SharedFolder{
		{Path: "/home/u/docs", Passwd: "secret"},
		{Path: "/media/data", Name: "data"},
	}
	if err := SaveLastShared(in); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	got := LoadLastShared()
	if len(got) != 2 {
		t.Fatalf("读取数量 = %d，期望 2", len(got))
	}
	if got[0].Path != in[0].Path || got[0].Passwd != "secret" {
		t.Errorf("第一条记录 = %+v，期望含密码 secret", got[0])
	}
	if got[1].Path != "/media/data" || got[1].Passwd != "" {
		t.Errorf("第二条记录 = %+v，期望无密码", got[1])
	}
}

// TestLastSharedLegacyCompat 旧版仅路径列表（shared: [path...]）仍能读取，按无密码处理。
func TestLastSharedLegacyCompat(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	cfgDir := filepath.Join(cfgHome, "filespace")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "shared:\n    - /a\n    - /b\n"
	if err := os.WriteFile(filepath.Join(cfgDir, lastSharedFile), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadLastShared()
	if len(got) != 2 || got[0].Path != "/a" || got[0].Passwd != "" || got[1].Path != "/b" || got[1].Passwd != "" {
		t.Errorf("旧格式解析 = %+v，期望两条无密码路径 /a、/b", got)
	}
}
