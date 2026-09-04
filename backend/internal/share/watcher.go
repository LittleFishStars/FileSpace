package share

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchDebounce 目录变更事件的防抖窗口：窗口内到达的事件合并为一次触发，
// 避免复制 / 解压 / 编译等瞬间海量事件把每次变更都变成一次全量重扫。
const watchDebounce = 300 * time.Millisecond

// dirWatcher 用 fsnotify 递归监听共享目录树：磁盘上任意文件 / 目录发生变化
// （含原地改写内容、任意深度的增删改）都会经防抖合并后触发一次后台重扫，
// 统计缓存因此始终跟随磁盘，无需轮询比对（这是 Manager 统计缓存的过期信号源）。
type dirWatcher struct {
	mu sync.Mutex
	w  *fsnotify.Watcher
	// watched 共享目录 ID → 已注册 watch 的目录路径集合（相对共享根的绝对路径）。
	watched map[string]map[string]struct{}
	// pending 共享目录 ID → 防抖定时器（存在表示该目录的变化正在合并窗口中）。
	pending map[string]*time.Timer
	// scan 由 Manager 注入：防抖到期后触发对应目录的后台全量重扫。
	scan  func(id string)
	delay time.Duration
}

// newDirWatcher 创建目录变更监听器；scan 在防抖后触发，delay 为防抖窗口。
func newDirWatcher(delay time.Duration, scan func(id string)) (*dirWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &dirWatcher{
		w:       w,
		watched: make(map[string]map[string]struct{}),
		pending: make(map[string]*time.Timer),
		scan:    scan,
		delay:   delay,
	}, nil
}

// watchFolder 为共享目录 root 递归挂载 watch（含现存全部子目录）。
// 目录不存在时静默跳过（共享配置允许指向已删除的目录）。
func (dw *dirWatcher) watchFolder(id, root string) {
	root = filepath.Clean(root)
	if _, err := os.Stat(root); err != nil {
		return
	}
	dw.mu.Lock()
	defer dw.mu.Unlock()
	dw.addTreeLocked(id, root)
}

// addTreeLocked 递归注册 root 及其现存子目录的 watch（调用方须持有 dw.mu）。
// fsnotify 只监听单层目录，后续新建的子目录由事件循环收到 Create 事件后补挂。
func (dw *dirWatcher) addTreeLocked(id, root string) {
	if _, err := os.Stat(root); err != nil {
		return // 目录刚被删除等：静默跳过
	}
	set, ok := dw.watched[id]
	if !ok {
		set = make(map[string]struct{})
		dw.watched[id] = set
	}
	warnedSpace := false
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil // 单项不可访问（权限 / 竞态删除）时跳过
		}
		if _, dup := set[path]; dup {
			return nil // 已注册（一般不会出现，除非重复挂载）
		}
		if err := dw.w.Add(path); err != nil {
			// inotify watch 数量达到内核上限：整体放弃该目录树并提示用户调大上限
			if errors.Is(err, syscall.ENOSPC) && !warnedSpace {
				warnedSpace = true
				log.Printf("监听目录失败：文件变更监听数量已达系统上限（可调大 fs.inotify.max_user_watches），%s 将不随文件变化自动刷新: %v", root, err)
				return filepath.SkipAll
			}
			return nil // 其余错误（权限等）静默跳过
		}
		set[path] = struct{}{}
		return nil
	})
	if len(set) == 0 {
		delete(dw.watched, id)
	}
}

// unwatchFolder 移除共享目录 id 的全部 watch（目录被移出共享列表时调用）。
func (dw *dirWatcher) unwatchFolder(id string) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	if t, ok := dw.pending[id]; ok {
		t.Stop()
		delete(dw.pending, id)
	}
	set := dw.watched[id]
	if set == nil {
		return
	}
	for p := range set {
		_ = dw.w.Remove(p) // 目录已删除时 fsnotify 会报错，忽略
	}
	delete(dw.watched, id)
}

// discardPathLocked 目录 / 文件被删除后清理其已注册的 watch 记录（含子目录）。
// fsnotify 会在被删目录上自动移除对应 watch，这里只需清理内部索引，
// 使同名目录日后重建时能重新挂载监听（调用方须持有 dw.mu）。
func (dw *dirWatcher) discardPathLocked(id, path string) {
	set := dw.watched[id]
	if set == nil {
		return
	}
	prefix := path + string(filepath.Separator)
	for p := range set {
		if p == path || strings.HasPrefix(p, prefix) {
			delete(set, p)
		}
	}
	if len(set) == 0 {
		delete(dw.watched, id)
	}
}

// handle 处理一个文件系统事件：定位受影响的共享目录并合并触发重扫，
// 对新建目录补挂 watch、对删除目录清理内部索引。
func (dw *dirWatcher) handle(ev fsnotify.Event) {
	dw.mu.Lock()
	parent := filepath.Dir(ev.Name)
	ids := make([]string, 0, 2)
	for id, set := range dw.watched {
		if _, ok := set[ev.Name]; ok {
			ids = append(ids, id) // 被 watch 的目录自身被删 / 改名等
			continue
		}
		if _, ok := set[parent]; ok {
			ids = append(ids, id) // 目录内的子项变化：其父目录必在 watch 中
		}
	}
	if ev.Op&fsnotify.Remove != 0 {
		for _, id := range ids {
			dw.discardPathLocked(id, ev.Name)
		}
	}
	if ev.Op&fsnotify.Create != 0 {
		// 新建目录：为其及现存子目录补挂 watch（fsnotify 不递归）
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			for _, id := range ids {
				dw.addTreeLocked(id, filepath.Clean(ev.Name))
			}
		}
	}
	dw.mu.Unlock()
	for _, id := range ids {
		dw.trigger(id)
	}
}

// trigger 为共享目录 id 安排一次防抖后的重扫：窗口内事件合并，只扫一次。
func (dw *dirWatcher) trigger(id string) {
	dw.mu.Lock()
	defer dw.mu.Unlock()
	if _, ok := dw.pending[id]; ok {
		return // 已有防抖窗口在等待，合并本次事件
	}
	t := time.AfterFunc(dw.delay, func() {
		dw.mu.Lock()
		delete(dw.pending, id)
		dw.mu.Unlock()
		dw.scan(id)
	})
	dw.pending[id] = t
}

// run 运行事件循环直到 ctx 取消或监听器被关闭。
// 事件队列溢出（短时间内变化过多）时对所有共享目录做一次全量重扫兜底，
// 保证统计最终一致（fsnotify 官方对 ErrEventOverflow 的推荐处理）。
func (dw *dirWatcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-dw.w.Errors:
			if !ok {
				return
			}
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				dw.mu.Lock()
				ids := make([]string, 0, len(dw.watched))
				for id := range dw.watched {
					ids = append(ids, id)
				}
				dw.mu.Unlock()
				log.Printf("文件变更事件过多发生溢出，重新扫描全部共享目录以保证统计准确")
				for _, id := range ids {
					dw.scan(id)
				}
			} else {
				log.Printf("文件监听错误: %v", err)
			}
		case ev, ok := <-dw.w.Events:
			if !ok {
				return
			}
			dw.handle(ev)
		}
	}
}

// close 停止监听：取消未触发的防抖定时器并关闭底层 watcher（幂等）。
func (dw *dirWatcher) close() {
	dw.mu.Lock()
	for id, t := range dw.pending {
		t.Stop()
		delete(dw.pending, id)
	}
	dw.mu.Unlock()
	_ = dw.w.Close()
}

// ensureWatcher 确保目录变更监听已启动，返回当前监听器。
// 启动失败（平台不支持 / 内核资源不足等）时仅告警并返回 nil：
// 统计缓存退化为启动与添加共享目录时的一次扫描（事件不可用场景较少见）。
func (m *Manager) ensureWatcher() *dirWatcher {
	m.lifeMu.Lock()
	defer m.lifeMu.Unlock()
	if m.closed {
		return nil
	}
	if m.watcher != nil {
		return m.watcher
	}
	w, err := newDirWatcher(watchDebounce, m.scanAsync)
	if err != nil {
		log.Printf("文件变更监听不可用（%v）：统计缓存将不会随文件变化自动刷新", err)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.watcher = w
	m.watchStop = cancel
	go w.run(ctx)
	// 为当前所有共享目录递归挂载 watch
	m.mu.RLock()
	folders := append([]Folder(nil), m.folders...)
	m.mu.RUnlock()
	for _, f := range folders {
		w.watchFolder(f.ID, f.Path)
	}
	return w
}

// currentWatcher 返回已启动的监听器（未启动或已关闭时为 nil）。
func (m *Manager) currentWatcher() *dirWatcher {
	m.lifeMu.Lock()
	defer m.lifeMu.Unlock()
	return m.watcher
}

// Close 停止目录变更监听并释放资源（幂等；进程优雅退出时调用）。
func (m *Manager) Close() {
	m.lifeMu.Lock()
	defer m.lifeMu.Unlock()
	if m.watcher == nil {
		m.closed = true
		return
	}
	m.watcher.close()
	m.watcher = nil
	if m.watchStop != nil {
		m.watchStop()
		m.watchStop = nil
	}
	m.closed = true
}
