package share

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filespace/internal/config"
)

// TestSetPassword 验证 SetPassword 的设置 / 修改 / 移除密码（内部仅存哈希）与未共享报错。
func TestSetPassword(t *testing.T) {
	root := t.TempDir()
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	id := folderID(root)

	// 未共享的目录：返回 ErrFolderNotFound
	other := filepath.Join(t.TempDir(), "nope")
	if _, err := m.SetPassword(other, "secret"); !errors.Is(err, ErrFolderNotFound) {
		t.Fatalf("未共享目录应返回 ErrFolderNotFound，实际 %v", err)
	}

	// 设置密码：内部只存 sha256 哈希，不存明文
	want := hashPassword("secret")
	f, err := m.SetPassword(root, "secret")
	if err != nil {
		t.Fatalf("设置密码失败: %v", err)
	}
	if f.PasswdHash != want {
		t.Errorf("设置后 PasswdHash = %q，期望 %q（明文 secret 的哈希）", f.PasswdHash, want)
	}
	if pw, _ := m.FolderPasswdHash(id); pw != want {
		t.Errorf("FolderPasswdHash = %q，期望 %q", pw, want)
	}

	// 修改密码
	wantNew := hashPassword("newpass")
	if _, err := m.SetPassword(root, "newpass"); err != nil {
		t.Fatalf("修改密码失败: %v", err)
	}
	if pw, _ := m.FolderPasswdHash(id); pw != wantNew {
		t.Errorf("修改后 FolderPasswdHash = %q，期望 %q", pw, wantNew)
	}

	// 空值移除密码（恢复开放）
	if _, err := m.SetPassword(root, ""); err != nil {
		t.Fatalf("移除密码失败: %v", err)
	}
	if pw, _ := m.FolderPasswdHash(id); pw != "" {
		t.Errorf("移除后 FolderPasswdHash = %q，期望空", pw)
	}
}

// TestPasswordFromConfig 明文配置（配置文件 / 命令行输入）在 NewManager 时统一转哈希。
func TestPasswordFromConfig(t *testing.T) {
	root := t.TempDir()
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root", Passwd: "secret"}})
	id := folderID(root)
	if pw, _ := m.FolderPasswdHash(id); pw != hashPassword("secret") {
		t.Errorf("明文配置应转哈希存储，实际 %q", pw)
	}
	if !m.MatchPassword("secret") {
		t.Error("MatchPassword(secret) 应为 true")
	}
	if m.MatchPassword("wrong") {
		t.Error("MatchPassword(wrong) 应为 false")
	}
	// 持久化快照只含哈希、不含明文
	for _, sf := range m.SharedSnapshot() {
		if sf.Passwd != "" {
			t.Error("SharedSnapshot 不应携带明文密码")
		}
		if sf.PasswdHash != hashPassword("secret") {
			t.Errorf("SharedSnapshot.PasswdHash = %q，期望 %q", sf.PasswdHash, hashPassword("secret"))
		}
	}
}

// TestSetPasswordByRealPath 符号链接指向共享目录时，按真实路径匹配也能修改密码。
func TestSetPasswordByRealPath(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("当前环境不支持符号链接: %v", err)
	}
	m := NewManager([]config.SharedFolder{{Path: real, Name: "root"}})
	if _, err := m.SetPassword(link, "secret"); err != nil {
		t.Fatalf("按符号链接路径修改密码失败: %v", err)
	}
	if pw, _ := m.FolderPasswdHash(folderID(real)); pw != hashPassword("secret") {
		t.Errorf("FolderPasswdHash = %q，期望 %q（真实路径匹配生效）", pw, hashPassword("secret"))
	}
}
