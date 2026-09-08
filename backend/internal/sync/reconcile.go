package sync

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filespace/internal/pathutil"
)

// errFolderMissing 目标文件夹在远端已不存在（被移除或 id 有误）的错误哨兵。
// 对账语义上属「永久」错误：任务不重试（重新对账只会反复打印缺失告警）。
var errFolderMissing = errors.New("远程文件夹不存在")

// isMissingFolder 判断错误是否为「远端目标文件夹不存在」。
func isMissingFolder(err error) bool { return errors.Is(err, errFolderMissing) }

// remoteEntry 远端目录树中的一个条目。
type remoteEntry struct {
	rel     string // 相对共享根的前斜杠路径（目录以 "/" 结尾）
	isDir   bool
	size    int64
	modTime string // RFC3339（秒精度）
}

// reconcileOnce 对单个同步任务执行一轮完整对账（在任务 goroutine 内单线程执行）：
//  1. 拉取远端共享列表定位目标文件夹，确认存在且 id 匹配；
//  2. 若该文件夹设置了访问密码，用 Passwd 换取访问令牌；
//  3. 深度遍历远端目录树（懒加载），汇总全部条目；
//  4. 对账到本地：
//     a. 远端条目与本地「大小+修改时间一致」→ 跳过（增量）；
//     b. 否则下载（新增或远端变更），下载后把远端修改时间戳打到本地文件；
//     c. 目录/文件类型冲突 → 按远端类型重建；
//     d. 本地存在但远端没有的多余条目 → 清理（本地是远端镜像）。
//
// 语意：远端为事实源，本地为单向镜像；本地改动会被远端覆盖/清理。
func (t *Task) reconcileOnce(ctx context.Context, c *remoteClient) error {
	// 1. 定位远程文件夹（id 匹配），确认其仍被共享
	folder, err := c.findFolder(ctx)
	if err != nil {
		return err
	}
	if folder == nil {
		return fmt.Errorf("%w: 远程节点上找不到文件夹 id %s（可能已被移除或 id 有误）", errFolderMissing, t.spec.FolderID)
	}
	// 2. 认证：仅当目标文件夹设置了访问密码时才需要用 Passwd 换令牌
	//（开放文件夹直接访问；不按节点整体判断——节点上其他密码文件夹的存在
	// 不应阻塞对开放文件夹的同步，见 /api/auth 的节点级校验语义）。
	if folder.Auth {
		if err := c.authenticate(ctx); err != nil {
			return err
		}
	}

	// 3. 深度收集远端条目。进度条从这一步开始显示：从远端逐层拉取文件清单
	// 是下载的前提（TotalSize 只是总大小，给不了文件清单），对 .git/objects
	// 这类深层目录会耗时较久，必须一开始就有「获取文件列表」的大小百分比反馈，
	// 而不是干等到下载阶段才有动静。
	prog := newProgress(os.Stdout, t.Remote())
	prog.total = folder.TotalSize // 分母：目标文件夹总大小（stats 后台扫描）
	prog.beginListing()
	remote, err := collectRemote(ctx, c, "", prog)
	if err != nil {
		prog.finish()
		return err
	}
	prog.endListing()

	// 4. 确保本地同步目录存在
	if err := os.MkdirAll(t.spec.Local, 0o755); err != nil {
		return fmt.Errorf("创建本地同步目录失败: %w", err)
	}

	// 对账：先处理远端条目（建目录/下载/类型重建），再清理本地多余条目
	toDelete := t.reconcilePulls(ctx, c, remote, prog)
	if toDelete != nil {
		t.removeExtras(remote, toDelete)
	}
	return nil
}

// collectRemote 深度遍历远端目录树，返回全部条目（逐层调用 tree 懒加载）。
// prog 非空时每发现一个文件即上报字节（目录无大小不计入），刷新列表进度。
func collectRemote(ctx context.Context, c *remoteClient, dir string, prog *progress) ([]remoteEntry, error) {
	entries, err := c.tree(ctx, dir)
	if err != nil {
		return nil, err
	}
	var out []remoteEntry
	for _, e := range entries {
		rel := e.Path
		if e.IsDir {
			children, err := collectRemote(ctx, c, rel, prog)
			if err != nil {
				return nil, err
			}
			out = append(out, remoteEntry{rel: rel + "/", isDir: true})
			out = append(out, children...)
		} else {
			out = append(out, remoteEntry{rel: rel, size: e.Size, modTime: e.ModTime})
			if prog != nil {
				prog.scanAdd(e.Size)
			}
		}
	}
	return out, nil
}

// reconcilePulls 逐远端条目对账到本地（增量下载/类型冲突重建），
// 返回本地需要清理的多余条目（rel → 是否为目录；目录以 "/" 结尾）。
// 分两阶段执行：先规划（处理目录结构、判定待下载文件），再下载并刷新终端
// 进度条（分母在扫描阶段已设为文件夹总大小；不可用（<=0，如远端尚未统计完）
// 时回退为本轮待下载字节和，保证百分比仍反映下载进度）。
func (t *Task) reconcilePulls(ctx context.Context, c *remoteClient, remote []remoteEntry, prog *progress) map[string]bool {
	// 待下载列表：rel（远端相对路径）+ 本地绝对路径 + 大小 + 远端修改时间。
	type pullTask struct {
		rel   string
		local string
		size  int64
		mod   string
	}
	var pending []pullTask
	var pendingBytes int64

	// 阶段一：规划——处理目录结构（含类型冲突重建），收集需要下载的文件。
	for _, re := range remote {
		local := t.localPath(re.rel)
		if re.isDir {
			// 远端是目录：本地若是文件则先删除，再确保目录存在
			if fi, err := os.Lstat(local); err == nil && !fi.IsDir() {
				if err := os.RemoveAll(local); err != nil {
					fmt.Printf("同步重建 %q（远端为目录，本地为文件）失败: %v\n", re.rel, err)
					continue
				}
			}
			if err := os.MkdirAll(local, 0o755); err != nil {
				fmt.Printf("创建本地目录 %q 失败: %v\n", local, err)
			}
			continue
		}

		// 远端是文件：本地是目录则删除重建；本地文件「大小+修改时间」一致则跳过（增量命中）
		fi, statErr := os.Stat(local)
		switch {
		case statErr == nil && fi.IsDir():
			if err := os.RemoveAll(local); err != nil {
				fmt.Printf("同步重建 %q（远端为文件，本地为目录）失败: %v\n", re.rel, err)
				continue
			}
		case statErr == nil:
			if _, unchanged := fileUnchanged(fi, re); unchanged {
				continue
			}
		}
		pending = append(pending, pullTask{rel: re.rel, local: local, size: re.size, mod: re.modTime})
		pendingBytes += re.size
	}

	// 阶段二：执行下载。无待下载（纯增量命中）时只清掉「获取文件列表」的进度行，
	// 本地冗余清理（collectExtras）随之进行。
	if len(pending) > 0 {
		prog.count = len(pending)
		if prog.total <= 0 {
			prog.total = pendingBytes // 文件夹总大小不可用：回退为本轮待下载量
		}
		for _, p := range pending {
			if err := c.download(ctx, p.rel, p.local); err != nil {
				// 先清掉进度行再打印错误，避免错误与进度条串行；后续 add 会重新渲染进度
				prog.finish()
				fmt.Printf("同步下载 %q 失败: %v\n", p.rel, err)
				continue
			}
			// 打上远端修改时间戳：保证「大小+修改时间」判定在下次对账稳定命中（真正增量）
			applyRemoteModTime(p.local, p.mod)
			prog.add(p.size)
		}
		prog.finish()
		// 本轮汇总（非 TTY 也会打印，方便重定向到文件时观察进度结果）
		fmt.Printf("✅ 同步 %s 完成：本轮下载 %s，文件夹总大小 %s\n",
			t.Remote(), formatBytes(prog.done), formatBytes(prog.total))
	} else {
		// 本轮无待下载：清掉「获取文件列表」的进度行
		prog.finish()
	}
	return t.collectExtras(remote)
}

// fileUnchanged 判断本地文件与远端条目是否「大小+修改时间」一致（增量命中：
// 一致返回 unchanged=true，跳过下载）。远端时间格式异常时视为不一致（需重新下载）。
func fileUnchanged(fi os.FileInfo, re remoteEntry) (rt time.Time, unchanged bool) {
	rt, err := time.Parse(time.RFC3339, re.modTime)
	if err != nil {
		return rt, false
	}
	if fi.Size() != re.size {
		return rt, false
	}
	return rt, fi.ModTime().Truncate(time.Second).Equal(rt.Truncate(time.Second))
}

// applyRemoteModTime 把远端修改时间戳（RFC3339）打到本地文件，
// 使下次对账的大小+时间判定稳定；解析失败或 Chtimes 失败时静默跳过。
func applyRemoteModTime(path, rfc string) {
	rt, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return
	}
	_ = os.Chtimes(path, rt, rt)
}

// collectExtras 找出本地存在但远端不存在的条目（需清理）。
// 目录以其 "/" 结尾的 rel 记录，扫描时对整棵多余子树做 SkipDir 加速。
func (t *Task) collectExtras(remote []remoteEntry) map[string]bool {
	remoteSet := make(map[string]struct{}, len(remote))
	for _, re := range remote {
		remoteSet[re.rel] = struct{}{}
	}
	toDelete := make(map[string]bool)
	_ = filepath.WalkDir(t.spec.Local, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 单项不可访问：跳过
		}
		if filepath.Clean(path) == filepath.Clean(t.spec.Local) {
			return nil // 跳过同步根本身
		}
		rel, relErr := filepath.Rel(t.spec.Local, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			rel = rel + "/"
		}
		if _, ok := remoteSet[rel]; ok {
			return nil // 远端存在：保留
		}
		toDelete[rel] = d.IsDir()
		if d.IsDir() {
			return filepath.SkipDir // 目录整体多余：其后代随 RemoveAll 删除，无需逐个标记
		}
		return nil
	})
	return toDelete
}

// removeExtras 清理本地多余条目（远端不存在的）。目录深→浅先删（RemoveAll），
// 再删文件；删除前二次核对远端集合，避免误删本轮刚下载的条目。
func (t *Task) removeExtras(remote []remoteEntry, toDelete map[string]bool) {
	remoteSet := make(map[string]struct{}, len(remote))
	for _, re := range remote {
		remoteSet[re.rel] = struct{}{}
	}
	// 目录：按深度降序排列，先删最深的（父目录删了子目录一并消失）
	dirs := make([]string, 0, len(toDelete))
	for rel := range toDelete {
		if strings.HasSuffix(rel, "/") {
			dirs = append(dirs, rel)
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], "/") > strings.Count(dirs[j], "/")
	})
	for _, rel := range dirs {
		if _, ok := remoteSet[rel]; ok {
			continue
		}
		if err := os.RemoveAll(t.localPath(rel)); err != nil {
			fmt.Printf("同步清理本地多余目录 %q 失败: %v\n", rel, err)
		} else {
			fmt.Printf("🗑 同步清理本地多余目录: %s\n", rel)
		}
	}
	for rel, isDir := range toDelete {
		if isDir {
			continue
		}
		if _, ok := remoteSet[rel]; ok {
			continue
		}
		if err := os.Remove(t.localPath(rel)); err != nil {
			fmt.Printf("同步清理本地多余文件 %q 失败: %v\n", rel, err)
		} else {
			fmt.Printf("🗑 同步清理本地多余文件: %s\n", rel)
		}
	}
}

// localPath 把远端相对路径映射为本地同步目录下的绝对路径，并校验不越界。
// 越界（".." 逃逸等）时退回同步根，绝不允许写出同步目录。
func (t *Task) localPath(rel string) string {
	root := filepath.Clean(t.spec.Local)
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "/"))
	p := filepath.Join(root, filepath.FromSlash(rel))
	if !pathutil.Within(root, p) {
		return root
	}
	return p
}
