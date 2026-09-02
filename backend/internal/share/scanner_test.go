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

// newTestManager 构造共享 root 的管理器：缩短事件防抖窗口加速测试，
// 并在测试结束时关闭监听器（释放 fsnotify 资源）。
func newTestManager(t *testing.T, root string) (*Manager, string) {
	t.Helper()
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.debounce = 15 * time.Millisecond // 测试用短防抖
	t.Cleanup(m.Close)
	return m, folderID(root)
}

// waitFolderStat 通过 List 轮询等待某共享目录的统计达到目标值（超时报错）。
// 统计由文件系统变更事件驱动刷新（防抖窗口内合并、后台扫描），轮询只做观察。
func waitFolderStat(t *testing.T, m *Manager, id string, wantCount int, wantSize int64) model.FolderInfo {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		for _, f := range m.List() {
			if f.ID == id {
				if f.FileCount == wantCount && f.TotalSize == wantSize {
					return f
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("等待 fileCount=%d totalSize=%d 超时，最近列表: %+v", wantCount, wantSize, m.List())
		}
		time.Sleep(15 * time.Millisecond)
	}
}

// scannedAt 读取某目录最近一次扫描完成时间（供断言"未触发重扫"）。
func scannedAt(m *Manager, id string) time.Time {
	m.statsMu.RLock()
	defer m.statsMu.RUnlock()
	return m.stats[id].scannedAt
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

	m, id := newTestManager(t, root)
	m.WarmUp()
	f := waitFolderStat(t, m, id, 5, wantSize)
	if f.UpdatedAt == "" {
		t.Error("最近更新时间不应为空")
	}
}

// TestStatsRefreshOnNewFile 根目录新增文件 → 变更事件触发后台重扫，统计更新为新值。
func TestStatsRefreshOnNewFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m, id := newTestManager(t, root)
	m.WarmUp()
	waitFolderStat(t, m, id, 1, 100)

	writeFile(t, filepath.Join(root, "b.txt"), 50)
	f := waitFolderStat(t, m, id, 2, 150)
	if f.TotalSize != 150 {
		t.Errorf("新增后总大小 = %d，期望 150", f.TotalSize)
	}
}

// TestStatsRefreshOnInPlaceRewrite 原地改写已有文件内容（目录结构不变，大小变化）
// 也能通过 Write 事件被感知——这是事件通知相对目录 mtime 指纹方案的增强点。
func TestStatsRefreshOnInPlaceRewrite(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m, id := newTestManager(t, root)
	m.WarmUp()
	waitFolderStat(t, m, id, 1, 100)

	// 原地覆盖 a.txt（truncate 直写，不新增/删除目录项）：Write 事件 → 重扫
	writeFile(t, filepath.Join(root, "a.txt"), 300)
	f := waitFolderStat(t, m, id, 1, 300)
	if f.FileCount != 1 {
		t.Errorf("原地改写不应改变文件数，实际 %d", f.FileCount)
	}
	if f.TotalSize != 300 {
		t.Errorf("原地改写后总大小 = %d，期望 300", f.TotalSize)
	}
}

// TestStatsRefreshOnDeepCreate 深层新建目录与文件都能感知，且新建子目录的监听
// 会被补挂：第一次写入触发重扫后，同一新目录里的后续写入仍能触发更新。
func TestStatsRefreshOnDeepCreate(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m, id := newTestManager(t, root)
	m.WarmUp()
	waitFolderStat(t, m, id, 1, 100)

	// 一次建出多层新目录并写文件：fsnotify 不递归，靠 Create 事件逐层补挂 watch
	writeFile(t, filepath.Join(root, "sub", "deep", "x.txt"), 30)
	waitFolderStat(t, m, id, 2, 130)

	// 补挂的 watch 应继续生效：同一深处目录再写一个文件
	writeFile(t, filepath.Join(root, "sub", "deep", "y.txt"), 20)
	f := waitFolderStat(t, m, id, 3, 150)
	if f.TotalSize != 150 {
		t.Errorf("深层第二次写入后总大小 = %d，期望 150", f.TotalSize)
	}
}

// TestStatsRefreshOnDelete 删除文件 → 事件触发重扫，统计下降。
func TestStatsRefreshOnDelete(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	writeFile(t, filepath.Join(root, "b.txt"), 50)
	m, id := newTestManager(t, root)
	m.WarmUp()
	waitFolderStat(t, m, id, 2, 150)

	if err := os.Remove(filepath.Join(root, "b.txt")); err != nil {
		t.Fatalf("删除文件失败: %v", err)
	}
	f := waitFolderStat(t, m, id, 1, 100)
	if f.TotalSize != 100 {
		t.Errorf("删除后总大小 = %d，期望 100", f.TotalSize)
	}
}

// TestStatsKeptFreshWithoutChange 目录无任何变化时，多次 List 不触发重扫
// （事件驱动下无事件即无扫描，scannedAt 保持不变）。
func TestStatsKeptFreshWithoutChange(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m, id := newTestManager(t, root)
	m.WarmUp()
	waitFolderStat(t, m, id, 1, 100)

	before := scannedAt(m, id)
	// 无变化时反复 List + 等待，不应有任何重扫
	for i := 0; i < 10; i++ {
		_ = m.List()
		time.Sleep(10 * time.Millisecond)
	}
	if after := scannedAt(m, id); !after.Equal(before) {
		t.Errorf("目录无变化时不应重扫：scannedAt 从 %v 变为 %v", before, after)
	}
}

// TestStatsHandlesRemovedFolder 共享目录本体被删除后统计清零且不反复重扫；
// 目录重建后 List 兜底检测重现（重建目录不在监听范围），补扫恢复并重新挂载监听，
// 之后的写入再次由事件驱动刷新。
func TestStatsHandlesRemovedFolder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m, id := newTestManager(t, root)
	m.WarmUp()
	waitFolderStat(t, m, id, 1, 100)

	// 删除目录本体 → Remove 事件触发重扫，统计清零
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("删除目录失败: %v", err)
	}
	waitFolderStat(t, m, id, 0, 0)

	// 目录持续缺失：无事件、无重现检测命中，不应反复重扫
	before := scannedAt(m, id)
	for i := 0; i < 5; i++ {
		_ = m.List()
		time.Sleep(10 * time.Millisecond)
	}
	if after := scannedAt(m, id); !after.Equal(before) {
		t.Errorf("目录持续缺失时不应反复重扫：scannedAt 从 %v 变为 %v", before, after)
	}

	// 重建目录并放入文件 → List 兜底检测到重现，补扫并重新挂载监听
	writeFile(t, filepath.Join(root, "b.txt"), 200)
	waitFolderStat(t, m, id, 1, 200)

	// 重新挂载的监听应生效：再写入文件由事件驱动更新
	writeFile(t, filepath.Join(root, "c.txt"), 50)
	f := waitFolderStat(t, m, id, 2, 250)
	if f.TotalSize != 250 {
		t.Errorf("目录恢复后再写入总大小 = %d，期望 250", f.TotalSize)
	}
}

// TestListNotBlockingOnMissingStats 未预热时 List 立即返回（零值统计），不等待后台扫描。
func TestListNotBlockingOnMissingStats(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m, id := newTestManager(t, root)
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
	waitFolderStat(t, m, id, 1, 100)
}

// TestRemoveClearsStats 移除共享目录后，残留的后台扫描不应写回或 panic，监听同步停止。
func TestRemoveClearsStats(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m, id := newTestManager(t, root)
	m.WarmUp()
	waitFolderStat(t, m, id, 1, 100)

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
