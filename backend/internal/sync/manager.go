package sync

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
)

// 周期对账间隔：远端文件变化会在该时长内同步到本地（本地按固定周期轮询远端树）。
const (
	// pollInterval 两轮成功对账之间的间隔。
	pollInterval = 30 * time.Second
	// retryDelay 单轮对账失败后的首次重试间隔（指数退避的初始值，见 loop 中 backoff 配置）。
	retryDelay = 10 * time.Second
	// retryMaxInterval 失败重试的指数退避上限：避免远端长时间不可达时无限加长等待。
	retryMaxInterval = 5 * time.Minute
	// initialDelay 首次对账前等待：给远端节点留出启动/恢复时间。
	initialDelay = 2 * time.Second
)

// Task 一个后台同步任务（把远端某共享文件夹单向镜像到本地路径）。
type Task struct {
	spec   Spec
	status syncStatus
	done   chan struct{} // 对账循环退出后关闭（供 Stop 等待）
	hush   chan struct{} // close 后终止对账循环（本地切换停止态）

	snapMu sync.Mutex // 保护 snap（API 轮询与对账 goroutine 并发读写）
	snap   TaskSnapshot
}

// TaskSnapshot 同步任务的实时状态快照（供前端轮询 /api/sync/status）。
// phase 取值：listing（获取文件列表）/ downloading（下载）/ ""（空闲或结束）；
// done/total 当前阶段进度（获取列表阶段为已发现字节/文件夹总大小，
// 下载阶段为已下载字节/文件夹总大小），speed 下载速度（字节/秒）。
type TaskSnapshot struct {
	Remote string `json:"remote"` // 远端定位（ip:port:folderid）
	Local  string `json:"local"`  // 本地同步目录
	Status string `json:"status"` // 任务生命周期：等待启动/同步中/已停止
	Phase  string `json:"phase"`  // listing / downloading / ""
	Done   int64  `json:"done"`
	Total  int64  `json:"total"`
	Speed  int64  `json:"speed"` // 字节/秒，取整
}

// publishProgress 把当前进度快照同步到 Task（由 progress.update 回调调用，
// 运行在对账 goroutine 内；API 轮询通过 Snapshot 读取，锁保护并发）。
func (t *Task) publishProgress(p *progress) {
	t.snapMu.Lock()
	t.snap.Remote = t.Remote()
	t.snap.Local = t.spec.Local
	t.snap.Status = t.status.String()
	t.snap.Phase = p.phase
	if p.listing {
		t.snap.Done = p.listedBytes
	} else {
		t.snap.Done = p.done
	}
	t.snap.Total = p.total
	t.snap.Speed = int64(p.speed)
	t.snapMu.Unlock()
}

// Snapshot 返回当前任务快照的副本（并发安全，供 API 轮询）。
func (t *Task) Snapshot() TaskSnapshot {
	t.snapMu.Lock()
	defer t.snapMu.Unlock()
	return t.snap
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
// 启动时立即发布初始快照（phase=waiting），让前端在首次对账前的
// initialDelay 内就能显示浮动进度条，不留「点了同步没有反应」的空窗。
func (t *Task) start(ctx context.Context) {
	t.status = statusRunning
	t.done = make(chan struct{})
	t.hush = make(chan struct{})
	t.publishWaiting()
	go t.loop(ctx)
}

// publishWaiting 发布「等待首次对账」的初始快照（phase=waiting）。
func (t *Task) publishWaiting() {
	t.snapMu.Lock()
	t.snap.Remote = t.Remote()
	t.snap.Local = t.spec.Local
	t.snap.Status = t.status.String()
	t.snap.Phase = "waiting"
	t.snapMu.Unlock()
}

// loop 周期对账，直到 ctx 取消：每轮成功对账后等待 pollInterval，
// 失败后用指数退避（retryDelay 起步、retryMaxInterval 封顶）等待重进；
// 目标文件夹在远端消失（永久错误）时停止任务。
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

	// 失败重试的指数退避：复用 cenkalti/backoff（此前仅作为 zeroconf 的
	// 间接依赖，这里转正直接使用），避免远端短时不可达时按固定间隔反复打点。
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = retryDelay
	bo.Multiplier = 2
	bo.MaxInterval = retryMaxInterval
	bo.MaxElapsedTime = 0 // 不因累计时长放弃重试（对账会一直持续到远端恢复或任务停止）

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
			bo.Reset() // 成功一轮后重置退避，下次失败重新从 retryDelay 起步
			timer.Reset(pollInterval)
			continue
		}
		if isMissingFolder(err) {
			// 目标文件夹被远端移除：视为永久错误，任务停止（不再反复打印缺失告警）
			log.Printf("同步任务已停止（远端文件夹 %s 已不存在）: %s", t.spec.FolderID, t.spec.Address())
			t.stop(statusStopped)
			return
		}
		delay := bo.NextBackOff()
		log.Printf("同步 %s <- %s 失败: %v（%s 后重试）", t.spec.Local, t.Remote(), err, delay)
		timer.Reset(delay)
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

// List 返回全部同步任务的状态快照（供前端轮询 /api/sync/status）。
func (m *Manager) List() []TaskSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]TaskSnapshot, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t.Snapshot())
	}
	return out
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
