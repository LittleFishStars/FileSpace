package sync

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// 周期对账间隔：远端文件变化会在该时长内同步到本地（本地按固定周期轮询远端树）。
const (
	// pollInterval 两轮成功对账之间的间隔。
	pollInterval = 30 * time.Second
	// retryDelay 单轮对账失败后的重试间隔（远端点暂时不可达时退避）。
	retryDelay = 10 * time.Second
	// initialDelay 首次对账前等待：给远端节点留出启动/恢复时间。
	initialDelay = 2 * time.Second
)

// Task 一个后台同步任务（把远端某共享文件夹单向镜像到本地路径）。
type Task struct {
	spec   Spec
	status syncStatus
	done   chan struct{} // 对账循环退出后关闭（供 Stop 等待）
	hush   chan struct{} // close 后终止对账循环（本地切换停止态）
}

// syncStatus 任务生命周期状态。
type syncStatus int

const (
	statusWaiting syncStatus = iota // 已注册，尚未开始对账
	statusRunning
	statusStopped
)

func (s syncStatus) String() string {
	switch s {
	case statusWaiting:
		return "等待启动"
	case statusRunning:
		return "同步中"
	default:
		return "已停止"
	}
}

// start 在共享 ctx 生命周期内启动任务对账循环（进程内仅调用一次）。
func (t *Task) start(ctx context.Context) {
	t.status = statusRunning
	t.done = make(chan struct{})
	t.hush = make(chan struct{})
	go t.loop(ctx)
}

// loop 周期对账，直到 ctx 取消：每轮成功对账后等待 pollInterval，
// 失败后等待 retryDelay 重进；目标文件夹在远端消失（永久错误）时停止任务。
func (t *Task) loop(ctx context.Context) {
	defer close(t.done)
	// 首次对账前等待 initialDelay
	select {
	case <-ctx.Done():
		t.stop(statusStopped)
		return
	case <-t.hush:
		t.stop(statusStopped)
		return
	case <-time.After(initialDelay):
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			t.stop(statusStopped)
			return
		case <-t.hush:
			t.stop(statusStopped)
			return
		case <-timer.C:
		}
		err := t.reconcileOnce(ctx, newRemoteClient(t.spec))
		if err == nil {
			timer.Reset(pollInterval)
			continue
		}
		if isMissingFolder(err) {
			// 目标文件夹被远端移除：视为永久错误，任务停止（不再反复打印缺失告警）
			log.Printf("同步任务已停止（远端文件夹 %s 已不存在）: %s", t.spec.FolderID, t.spec.Address())
			t.stop(statusStopped)
			return
		}
		log.Printf("同步 %s <- %s 失败: %v（%s 后重试）", t.spec.Local, t.Remote(), err, retryDelay)
		timer.Reset(retryDelay)
	}
}

// stop 将任务置于停止态。
func (t *Task) stop(s syncStatus) { t.status = s }

// Remote 返回远端描述（ip:port:folderid）。
func (t *Task) Remote() string {
	return fmt.Sprintf("%s:%s", t.spec.Address(), t.spec.FolderID)
}

// hushAndWait 关闭任务并等待其对账循环退出（进程退出时调用）。
func (t *Task) hushAndWait() {
	if t.hush != nil {
		close(t.hush)
	}
	if t.done != nil {
		<-t.done
	}
}

// Manager 管理一批后台同步任务。
type Manager struct {
	mu     sync.Mutex
	tasks  map[string]*Task // key: 本地同步路径
	cancel context.CancelFunc
	ctx    context.Context
}

// NewManager 创建同步任务管理器。
func NewManager() *Manager {
	return &Manager{tasks: make(map[string]*Task)}
}

// Add 注册一个同步任务（按本地同步路径去重；重复添加忽略）。调用方应先做路径校验。
// 若管理器已在运行（Start 已调用），新任务立即启动对账循环（供运行中交接用）。
func (m *Manager) Add(spec Spec) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tasks[spec.Local]; exists {
		return
	}
	t := &Task{spec: spec, status: statusWaiting}
	m.tasks[spec.Local] = t
	if m.ctx != nil {
		t.start(m.ctx)
	}
}

// Start 启动全部已注册任务（幂等；进程启动后调用一次）。
func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.ctx = ctx
	m.cancel = cancel
	for _, t := range m.tasks {
		t.start(ctx)
	}
}

// Stop 停止全部任务并等待退出（幂等；进程退出前调用）。
func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	m.ctx = nil
	m.cancel = nil
	m.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	for _, t := range m.tasks {
		t.hushAndWait()
	}
}
