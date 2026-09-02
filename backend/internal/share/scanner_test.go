package share

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"filespace/internal/config"
	"filespace/internal/model"
)

// writeFile 创建文件（目录自动创建）并返回其字节大小。
func writeFile(t *testing.T, path string, n int) int64 {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	return int64(n)
}

// waitFileCount 通过 List 轮询等待某共享目录的统计 fileCount 达到目标值（超时报错）。
// List 返回的是加锁保护的快照，且每次调用会触发过期缓存的后台重扫，天然无竞态。
func waitFileCount(t *testing.T, m *Manager, id string, want int, timeout time.Duration) model.FolderInfo {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		for _, f := range m.List() {
			if f.ID == id {
				if f.FileCount == want {
					return f
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 fileCount=%d 超时（%s），最近列表: %+v", want, timeout, m.List())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestScanStatsFullTree 验证统计是全量递归的（含所有层级子目录），且与文件系统一致。
func TestScanStatsFullTree(t *testing.T) {
	root := t.TempDir()
	wantSize := int64(0)
	wantSize += writeFile(t, filepath.Join(root, "a.txt"), 100)
	wantSize += writeFile(t, filepath.Join(root, "b.txt"), 200)
	wantSize += writeFile(t, filepath.Join(root, "sub", "x1.txt"), 10)
	wantSize += writeFile(t, filepath.Join(root, "sub", "deep", "x2.txt"), 30)
	wantSize += writeFile(t, filepath.Join(root, "sub", "deep", "x3.txt"), 40)

	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.WarmUp()
	f := waitFileCount(t, m, folderID(root), 5, 3*time.Second)
	if f.TotalSize != wantSize {
		t.Errorf("总大小 = %d，期望 %d（递归统计所有层级）", f.TotalSize, wantSize)
	}
	if f.UpdatedAt == "" {
		t.Error("最近更新时间不应为空")
	}
}

// TestStatsRefreshAfterTTL 验证缓存过期后 List 触发后台重扫，统计最终更新为新值。
func TestStatsRefreshAfterTTL(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.statsTTL = 50 * time.Millisecond // 测试用短 TTL
	m.WarmUp()
	if f := waitFileCount(t, m, folderID(root), 1, 3*time.Second); f.TotalSize != 100 {
		t.Fatalf("初始总大小 = %d，期望 100", f.TotalSize)
	}

	// 目录新增文件后，TTL 过期前 List 仍返回缓存旧值（不阻塞、不立即重扫）
	writeFile(t, filepath.Join(root, "b.txt"), 50)
	if f := m.List(); f[0].FileCount != 1 {
		t.Fatalf("TTL 内应返回缓存旧值，实际 fileCount = %d", f[0].FileCount)
	}

	// 等待 TTL 过期后 List 触发后台重扫，统计应更新为 2
	f := waitFileCount(t, m, folderID(root), 2, 3*time.Second)
	if f.TotalSize != 150 {
		t.Errorf("重扫后总大小 = %d，期望 150", f.TotalSize)
	}
}

// TestListNotBlockingOnMissingStats 未预热时 List 立即返回（零值统计），不等待后台扫描。
func TestListNotBlockingOnMissingStats(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	// 不调用 WarmUp：List 应立即返回（零值），并在后台触发扫描
	start := time.Now()
	infos := m.List()
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("未预热时 List 不应阻塞，耗时 %v", elapsed)
	}
	if infos[0].FileCount != 0 {
		t.Errorf("未预热时统计应为零值，实际 %d", infos[0].FileCount)
	}
	// 后台扫描最终就绪
	waitFileCount(t, m, folderID(root), 1, 3*time.Second)
}

// TestRemoveClearsStats 移除共享目录后，残留的后台扫描不应写回或 panic。
func TestRemoveClearsStats(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	id := folderID(root)
	m.WarmUp()
	waitFileCount(t, m, id, 1, 3*time.Second)

	if err := m.Remove(id); err != nil {
		t.Fatalf("移除失败: %v", err)
	}
	m.statsMu.RLock()
	_, ok := m.stats[id]
	m.statsMu.RUnlock()
	if ok {
		t.Error("移除后统计缓存应被清理")
	}
	// 触发一次 List 不应 panic（scanAsync 中 Resolve 失败即返回）
	_ = m.List()
}
