package state

import (
	"os"
	"path/filepath"
	"testing"

	"filespace/internal/config"
)

// TestLastSharedRoundtrip 新格式（路径 + 密码哈希）读写一致：哈希随记录持久化。
func TestLastSharedRoundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // Linux os.UserConfigDir 读取该变量
	in := []config.SharedFolder{
		{Path: "/home/u/docs", PasswdHash: "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"},
		{Path: "/media/data", Name: "data"},
	}
	if err := SaveLastShared(in); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	got := LoadLastShared()
	if len(got) != 2 {
		t.Fatalf("读取数量 = %d，期望 2", len(got))
	}
	if got[0].Path != in[0].Path || got[0].PasswdHash != in[0].PasswdHash {
		t.Errorf("第一条记录 = %+v，期望含密码哈希 %q", got[0], in[0].PasswdHash)
	}
	if got[1].Path != "/media/data" || got[1].PasswdHash != "" {
		t.Errorf("第二条记录 = %+v，期望无密码", got[1])
	}
}

// TestLastSharedLegacyCompat 兼容读取：旧版仅路径列表与上版明文密码记录均能读入。
func TestLastSharedLegacyCompat(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	cfgDir := filepath.Join(cfgHome, "filespace")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// ① 最旧格式：shared 为纯字符串列表
	legacy := "shared:\n    - /a\n    - /b\n"
	if err := os.WriteFile(filepath.Join(cfgDir, lastSharedFile), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadLastShared()
	if len(got) != 2 || got[0].Path != "/a" || got[0].PasswdHash != "" || got[1].Path != "/b" {
		t.Errorf("纯路径格式解析 = %+v，期望两条无密码路径 /a、/b", got)
	}
	// ② 上版明文记录（程序侧会现场转哈希，此处仅验证字段读入保留）
	plain := "shared:\n    - path: /a\n      passwd: secret\n"
	if err := os.WriteFile(filepath.Join(cfgDir, lastSharedFile), []byte(plain), 0o644); err != nil {
		t.Fatal(err)
	}
	got = LoadLastShared()
	if len(got) != 1 || got[0].Path != "/a" || got[0].Passwd != "secret" || got[0].PasswdHash != "" {
		t.Errorf("明文记录格式解析 = %+v，期望保留明文 passwd=secret 供转换", got)
	}
}
