package share

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filespace/internal/config"
)

// TestSetPassword 验证 SetPassword 的设置 / 修改 / 移除密码与未共享报错。
func TestSetPassword(t *testing.T) {
	root := t.TempDir()
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	id := folderID(root)

	// 未共享的目录：返回 ErrFolderNotFound
	other := filepath.Join(t.TempDir(), "nope")
	if _, err := m.SetPassword(other, "secret"); !errors.Is(err, ErrFolderNotFound) {
		t.Fatalf("未共享目录应返回 ErrFolderNotFound，实际 %v", err)
	}

	// 设置密码
	f, err := m.SetPassword(root, "secret")
	if err != nil {
		t.Fatalf("设置密码失败: %v", err)
	}
	if f.Passwd != "secret" {
		t.Errorf("设置后 Passwd = %q，期望 secret", f.Passwd)
	}
	if pw, _ := m.FolderPasswd(id); pw != "secret" {
		t.Errorf("FolderPasswd = %q，期望 secret", pw)
	}

	// 修改密码
	if _, err := m.SetPassword(root, "newpass"); err != nil {
		t.Fatalf("修改密码失败: %v", err)
	}
	if pw, _ := m.FolderPasswd(id); pw != "newpass" {
		t.Errorf("修改后 FolderPasswd = %q，期望 newpass", pw)
	}

	// 空值移除密码（恢复开放）
	if _, err := m.SetPassword(root, ""); err != nil {
		t.Fatalf("移除密码失败: %v", err)
	}
	if pw, _ := m.FolderPasswd(id); pw != "" {
		t.Errorf("移除后 FolderPasswd = %q，期望空", pw)
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
	if pw, _ := m.FolderPasswd(folderID(real)); pw != "secret" {
		t.Errorf("FolderPasswd = %q，期望 secret（真实路径匹配生效）", pw)
	}
}
