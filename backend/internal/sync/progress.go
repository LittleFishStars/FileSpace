package sync

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// 终端进度条：在同步任务运行时用 \r 覆盖单行显示进度，覆盖两个阶段——
//   - 获取文件列表（collectRemote 逐层拉取远端目录树，需先知道远端有哪些文件
//     才能下载，无法跳过）：显示「获取文件列表 已发现 / 总大小（%）」；
//   - 下载阶段（逐文件下载）：显示「下载 已下载 / 总大小（%） 速度 剩余时间」。
//
// 两个阶段都是纯大小百分比、不显示文件数量；分母取目标文件夹总大小
// （远端 stats 后台扫描的 FolderInfo.TotalSize），不可用时回退。
// 仅当 stdout 是真实终端（TTY）时渲染；管道/重定向（如重定向到日志文件）时静默，
// 避免把 \r 控制字符写进日志。多个同步任务并发时用包级互斥串行化刷新，
// 防止各任务进度行互相打断。

// progressMu 串行化所有进度条的一次刷新：多任务并发时，每次整行 \r 覆盖
// 在锁内完成，行内容不会被交错（视觉上本次显示哪个任务由最后一次刷新决定）。
var progressMu sync.Mutex

// renderInterval 两次实际刷新（写终端）的最小间隔：获取列表时条目数可达
// 数万，逐条刷新会刷屏并争用输出锁拖慢对账，按此间隔节流；最终态强制刷新。
const renderInterval = 120 * time.Millisecond

// progress 一个同步任务当前阶段的进度状态。
type progress struct {
	enabled  bool // 非 TTY 时为 false，render/finish 均为空操作
	rendered bool // 是否真正输出过进度行（finish 仅需清理渲染过的行）
	w        io.Writer
	id       string // 远程定位 + 文件夹 id（如 192.168.1.5:8080:abcd1234）

	// 获取文件列表阶段
	listing     bool  // 是否处于「获取文件列表」阶段
	listedBytes int64 // 已从远端清单中发现的文件字节和

	// 下载阶段
	total int64 // 进度分母：目标文件夹总大小（字节）
	count int   // 本轮待下载文件数（内部用于判定是否全部处理完，不参与显示）
	done  int64 // 进度分子：已下载字节
	doneN int   // 已下载文件数（内部计数）

	start      time.Time // 阶段开始时间（速度用）
	lastRender time.Time // 上次实际写终端的时刻（节流用）
}

// newProgress 创建进度条。stdout 非终端时返回 enabled=false 的空进度条（调用方
// 无需区分，scanAdd/add/render/finish 均安全）。
func newProgress(w io.Writer, id string) *progress {
	return &progress{
		enabled: isTTY(w),
		w:       w,
		id:      id,
		start:   time.Now(),
	}
}

// isTTY 判断写入目标是否为字符设备（真实终端）。管道、文件、NUL 均非 TTY。
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// beginListing 进入「获取文件列表」阶段（collectRemote 前调用），立即渲染
// 首行进度，让用户点完同步马上看到反馈（大文件夹遍历耗时较长，不能干等）。
func (p *progress) beginListing() {
	p.listing = true
	p.start = time.Now()
	p.render()
}

// scanAdd 记录从远端清单中发现的字节（目录无大小不计入）。
// 按 renderInterval 节流，避免大目录数万条目逐条刷新刷屏。
func (p *progress) scanAdd(size int64) {
	p.listedBytes += size
	p.maybeRender()
}

// endListing 结束「获取文件列表」阶段（collectRemote 完成后调用）：先强制渲染
// 最终列表结果（覆盖节流可能落下的最后一段），再切换到下载/空闲模式。
func (p *progress) endListing() {
	p.render()
	p.listing = false
}

// add 累加一条下载完成（bytes 为该文件实际下载字节数），并刷新进度行。
// 最后一个文件（doneN==count）强制渲染，保证 100% 完整显示。
func (p *progress) add(bytes int64) {
	p.done += bytes
	p.doneN++
	if p.doneN >= p.count {
		p.render() // 完成态强制
	} else {
		p.maybeRender()
	}
}

// maybeRender 节流版渲染：距上次实际写终端不足 renderInterval 时跳过。
func (p *progress) maybeRender() {
	if !p.enabled {
		return
	}
	if time.Since(p.lastRender) < renderInterval {
		return
	}
	p.render()
}

// render 刷新单行进度。格式：
//
//	获取列表：  同步 <id>：获取文件列表 3.0 MB / 15.0 MB（20%）
//	下载：      同步 <id>：下载 4.2 MB / 15.0 MB（25%） 2.3 MB/s 剩余 1m20s
//
// 用空格补齐行尾，避免上次更长的内容残留。
func (p *progress) render() {
	if !p.enabled {
		return
	}
	if !p.listing && p.total == 0 {
		return
	}
	var line string
	if p.listing {
		if p.total > 0 {
			line = fmt.Sprintf("\r同步 %s：获取文件列表 %s / %s（%d%%）",
				p.id, formatBytes(p.listedBytes), formatBytes(p.total),
				int(p.listedBytes*100/p.total))
		} else {
			// 总大小尚未统计到：只显示已发现字节
			line = fmt.Sprintf("\r同步 %s：获取文件列表… 已发现 %s",
				p.id, formatBytes(p.listedBytes))
		}
	} else {
		elapsed := time.Since(p.start).Seconds()
		var speed, eta string
		if elapsed > 0 && p.done > 0 {
			rate := float64(p.done) / elapsed
			speed = fmt.Sprintf(" %.1f MB/s", rate/(1024*1024))
			if p.done < p.total {
				remain := float64(p.total-p.done) / rate
				eta = fmt.Sprintf(" 剩余 %s", humanDuration(time.Duration(remain*float64(time.Second))))
			}
		}
		line = fmt.Sprintf("\r同步 %s：下载 %s / %s（%d%%）%s%s",
			p.id,
			formatBytes(p.done), formatBytes(p.total),
			int(p.done*100/p.total),
			speed, eta,
		)
	}
	// 预留行宽：补齐到 80 列，覆盖上一次更长内容
	if len(line) < 80 {
		line += spaces(80 - len(line))
	}
	progressMu.Lock()
	_, _ = io.WriteString(p.w, line)
	p.rendered = true
	p.lastRender = time.Now()
	progressMu.Unlock()
}

// finish 清除进度行（回到行首并擦除整行），随后可打印摘要等完整行。
// 仅当本进度条真正渲染过进度行时才清理，避免产生空清行噪音。
func (p *progress) finish() {
	if !p.enabled || !p.rendered {
		return
	}
	progressMu.Lock()
	_, _ = io.WriteString(p.w, "\r"+spaces(80)+"\r")
	progressMu.Unlock()
}

// spaces 返回 n 个空格组成的字符串（n<=0 时返回空串）。
func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return string(b)
}

// formatBytes 把字节数格式化为人类可读（B/KB/MB/GB，带一位小数）。
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// humanDuration 把时长格式化为「XdHhMmSs」或分段（不足 1 分钟显示秒）。
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	switch {
	case s >= 86400:
		return fmt.Sprintf("%dd%dh", s/86400, s%86400/3600)
	case s >= 3600:
		return fmt.Sprintf("%dh%dm", s/3600, s%3600/60)
	case s >= 60:
		return fmt.Sprintf("%dm%ds", s/60, s%60)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
