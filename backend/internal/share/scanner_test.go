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
// List 返回的是加锁保护的快照；目录变化时 List 的目录 mtime 指纹校验会在后台触发
// 重扫，而轮询间隔（20ms）大于测试设定的短校验节流，每次调用都会做校验，天然无竞态。
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

// TestStatsRefreshOnDirChange 验证目录内容变化（根目录新增文件，根目录 mtime 变化）后，
// List 的 mtime 指纹校验发现变化并触发后台重扫，统计最终更新为新值。
func TestStatsRefreshOnDirChange(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.checkInterval = 5 * time.Millisecond // 测试用短校验节流
	m.WarmUp()
	if f := waitFileCount(t, m, folderID(root), 1, 3*time.Second); f.TotalSize != 100 {
		t.Fatalf("初始总大小 = %d，期望 100", f.TotalSize)
	}

	// 目录新增文件后，指纹校验发现根目录 mtime 变化 → 后台重扫，统计更新为 2
	writeFile(t, filepath.Join(root, "b.txt"), 50)
	f := waitFileCount(t, m, folderID(root), 2, 3*time.Second)
	if f.TotalSize != 150 {
		t.Errorf("重扫后总大小 = %d，期望 150", f.TotalSize)
	}
}

// TestStatsRefreshOnDeepChange 验证深层子目录新增文件（根目录 mtime 不变）也能被
// 目录级 mtime 指纹感知并触发重扫，证明指纹覆盖整棵目录树而非只看根目录。
func TestStatsRefreshOnDeepChange(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.checkInterval = 5 * time.Millisecond
	m.WarmUp()
	if f := waitFileCount(t, m, folderID(root), 1, 3*time.Second); f.TotalSize != 100 {
		t.Fatalf("初始总大小 = %d，期望 100", f.TotalSize)
	}

	// 在 sub/deep 深处新增文件：只更新 deep 目录的 mtime，根目录 mtime 不变
	writeFile(t, filepath.Join(root, "sub", "deep", "x.txt"), 30)
	f := waitFileCount(t, m, folderID(root), 2, 3*time.Second)
	if f.TotalSize != 130 {
		t.Errorf("深层新增后总大小 = %d，期望 130", f.TotalSize)
	}
}

// TestStatsKeptFreshWithoutChange 目录无任何变化时，多次 List 的指纹校验不发现变化，
// 不应触发重扫（缓存不过期，scannedAt 保持不变）。
func TestStatsKeptFreshWithoutChange(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.checkInterval = time.Millisecond
	id := folderID(root)
	m.WarmUp()
	waitFileCount(t, m, id, 1, 3*time.Second)

	m.statsMu.RLock()
	before := m.stats[id].scannedAt
	m.statsMu.RUnlock()
	// 无变化时反复 List：每次指纹校验都不发现变化，不应安排重扫
	for i := 0; i < 5; i++ {
		_ = m.List()
		time.Sleep(5 * time.Millisecond)
	}
	m.statsMu.RLock()
	after := m.stats[id].scannedAt
	m.statsMu.RUnlock()
	if !after.Equal(before) {
		t.Errorf("目录无变化时不应重扫：scannedAt 从 %v 变为 %v", before, after)
	}
}

// TestStatsNotRefreshedOnInPlaceRewrite 原地改写文件内容（仅文件 mtime 变化、父目录
// mtime 不变）不会被指纹感知 → 不触发重扫。这是「仅按目录修改时间判定」的已知取舍：
// 大多数编辑器保存是先写临时文件再 rename 覆盖（会更新目录 mtime 而被感知），
// 直接 truncate 覆盖的场景统计会在下一次结构变化或重启时刷新。
func TestStatsNotRefreshedOnInPlaceRewrite(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.checkInterval = time.Millisecond
	id := folderID(root)
	m.WarmUp()
	waitFileCount(t, m, id, 1, 3*time.Second)

	rootInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	rootMtime := rootInfo.ModTime()
	// 原地覆盖文件内容（大小也变）：os.WriteFile 走 truncate，不改变父目录 mtime
	if err := os.WriteFile(filepath.Join(root, "a.txt"), make([]byte, 300), 0o644); err != nil {
		t.Fatalf("覆盖文件失败: %v", err)
	}
	rootInfo, err = os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if !rootInfo.ModTime().Equal(rootMtime) {
		t.Skip("当前文件系统原地写文件会更新目录 mtime，跳过本局限用例")
	}

	m.statsMu.RLock()
	before := m.stats[id].scannedAt
	m.statsMu.RUnlock()
	for i := 0; i < 5; i++ {
		_ = m.List()
		time.Sleep(5 * time.Millisecond)
	}
	m.statsMu.RLock()
	after := m.stats[id].scannedAt
	m.statsMu.RUnlock()
	if !after.Equal(before) {
		t.Errorf("原地改写文件不应触发重扫：scannedAt 从 %v 变为 %v", before, after)
	}
}

// TestStatsHandlesRemovedFolder 共享目录本体被删除后统计清零且不反复重扫；
// 目录恢复后指纹校验发现重现，重扫恢复统计。
func TestStatsHandlesRemovedFolder(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), 100)
	m := NewManager([]config.SharedFolder{{Path: root, Name: "root"}})
	m.checkInterval = 5 * time.Millisecond
	id := folderID(root)
	m.WarmUp()
	waitFileCount(t, m, id, 1, 3*time.Second)

	// 删除目录本体 → 指纹校验发现目录消失，后台重扫把统计清零
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("删除目录失败: %v", err)
	}
	if f := waitFileCount(t, m, id, 0, 3*time.Second); f.TotalSize != 0 {
		t.Errorf("目录删除后总大小 = %d，期望清零", f.TotalSize)
	}
	// 目录持续缺失：已反映缺失，后续 List 不应反复触发重扫
	m.statsMu.RLock()
	before := m.stats[id].scannedAt
	m.statsMu.RUnlock()
	for i := 0; i < 3; i++ {
		_ = m.List()
		time.Sleep(10 * time.Millisecond)
	}
	m.statsMu.RLock()
	after := m.stats[id].scannedAt
	m.statsMu.RUnlock()
	if !after.Equal(before) {
		t.Errorf("目录持续缺失时不应反复重扫：scannedAt 从 %v 变为 %v", before, after)
	}

	// 目录恢复并放入新文件 → 指纹校验发现目录重现，重扫恢复统计
	writeFile(t, filepath.Join(root, "b.txt"), 200)
	f := waitFileCount(t, m, id, 1, 3*time.Second)
	if f.TotalSize != 200 {
		t.Errorf("目录恢复后总大小 = %d，期望 200", f.TotalSize)
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
