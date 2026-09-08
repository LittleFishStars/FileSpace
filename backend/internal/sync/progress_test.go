package sync

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// newTestProgress 构造一个 TTY 模式（enabled=true）的进度条，写入 buf。
// lastRender 置为过去，绕过节流（渲染内容测试关注的是输出格式，节流单独测）。
func newTestProgress(buf *bytes.Buffer) *progress {
	return &progress{
		enabled:    true,
		w:          buf,
		id:         "192.168.1.5:8080:abcd1234",
		start:      time.Now(),
		lastRender: time.Time{}, // 零值：距上次刷新远超 renderInterval，必然触发渲染
	}
}

// expireThrottle 把 lastRender 置为过去，模拟「距上次刷新已超过 renderInterval」，
// 使下一次 add/scanAdd 必然触发渲染（用于验证渲染内容的测试）。
func expireThrottle(p *progress) {
	p.lastRender = time.Time{}
}

// countCR 统计文本中 \r 的个数（即进度条实际写入终端的刷新次数）。
func countCR(s string) int {
	return strings.Count(s, "\r")
}

// TestProgressFormatting 验证格式化函数的输出。
func TestProgressFormatting(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{3 * 1024 * 1024, "3.0 MB"},
		{5 * 1024 * 1024 * 1024, "5.0 GB"},
	}
	for _, c := range cases {
		if got := formatBytes(c.n); got != c.want {
			t.Errorf("formatBytes(%d) = %q，期望 %q", c.n, got, c.want)
		}
	}

	durationCases := []struct {
		sec  int
		want string
	}{
		{5, "5s"},
		{65, "1m5s"},
		{3600, "1h0m"},
		{90000, "1d1h"},
	}
	for _, c := range durationCases {
		if got := humanDuration(time.Duration(c.sec) * time.Second); got != c.want {
			t.Errorf("humanDuration(%ds) = %q，期望 %q", c.sec, got, c.want)
		}
	}
}

// TestProgressNonTTYSilent 验证非 TTY（管道/重定向，如写入 bytes.Buffer 或普通文件）
// 时进度条完全静默：newProgress 返回 enabled=false，render/add/finish 不产生任何输出。
func TestProgressNonTTYSilent(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, "192.168.1.5:8080:abcd1234")
	if p.enabled {
		t.Fatal("写入非 TTY 目标时 progress.enabled 应为 false")
	}
	p.count = 10
	p.total = 100
	p.add(10)
	p.add(20)
	p.finish()
	if buf.Len() != 0 {
		t.Errorf("非 TTY 进度条不应输出内容，得到: %q", buf.String())
	}
}

// TestProgressScanRender 验证扫描阶段的渲染：
//   - beginScan 立即输出一行「扫描远端目录…」；
//   - scanAdd 累加条目数（文件累加字节、目录不计字节）；
//   - endScan 强制渲染最终扫描结果（条目数/字节）。
func TestProgressScanRender(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.beginScan()
	line := buf.String()
	if !strings.HasPrefix(line, "\r同步 ") || !strings.Contains(line, "扫描远端目录") {
		t.Fatalf("beginScan 输出不符合预期: %q", line)
	}
	if !strings.Contains(line, "0 个条目") {
		t.Errorf("beginScan 初始应显示 0 个条目: %q", line)
	}

	// 文件：累加字节；目录：仅计入条数（此处快速连续调用，靠 endScan 看最终值）
	buf.Reset()
	p.scanAdd(false, 1024)
	p.scanAdd(true, 0)
	p.scanAdd(false, 2048)
	buf.Reset()
	p.endScan() // 强制渲染最终扫描结果
	line = buf.String()
	if !strings.Contains(line, "3 个条目") || !strings.Contains(line, "3.0 KB") {
		t.Errorf("扫描后应显示 3 个条目 3.0 KB: %q", line)
	}
}

// TestProgressDownloadAfterScan 验证扫描结束后无缝切换到下载阶段：
// 设置 count/total 后 add 输出下载行（不再显示「扫描」字样）。
func TestProgressDownloadAfterScan(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.beginScan()
	p.scanAdd(false, 10)
	p.endScan()

	buf.Reset()
	expireThrottle(p)
	p.count = 4
	p.total = 40
	p.add(10)
	line := buf.String()
	if !strings.Contains(line, "1/4") || !strings.Contains(line, "25%") {
		t.Errorf("扫描后下载行不符合预期: %q", line)
	}
	if strings.Contains(line, "扫描远端目录") {
		t.Errorf("下载阶段不应再显示扫描字样: %q", line)
	}
}

// TestProgressThrottle 验证节流：连续快速 scanAdd（间隔小于 renderInterval）
// 不会每次写终端（实际刷新次数远小于调用次数）；endScan 强制刷新最终结果。
func TestProgressThrottle(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.beginScan() // 第一次渲染（lastRender 从零值变为 now）

	buf.Reset()
	// 连续快速 scanAdd：全部落在 renderInterval 内，应被节流合并
	for i := 0; i < 1000; i++ {
		p.scanAdd(false, 10)
	}
	if got := countCR(buf.String()); got > 4 {
		t.Errorf("1000 次快速 scanAdd 的刷新次数应被节流（<=4），实际 %d 次", got)
	}

	// endScan 强制渲染最终结果
	buf.Reset()
	p.endScan()
	if !strings.Contains(buf.String(), "1000 个条目") {
		t.Errorf("endScan 应强制渲染最终条目数: %q", buf.String())
	}
}

// TestProgressRender 验证 TTY 模式下下载阶段的渲染：
//   - 每 add 一次输出一行以 \r 开头的进度（含文件数、字节、百分比）；
//   - finish 后输出一行 \r + 空格清行（长度足够覆盖上次内容）。
func TestProgressRender(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.count = 4
	p.total = 40
	p.add(10)
	line1 := buf.String()
	if !strings.HasPrefix(line1, "\r同步 ") || !strings.Contains(line1, "1/4") {
		t.Errorf("第一次 add 输出不符合预期: %q", line1)
	}
	if !strings.Contains(line1, "25%") {
		t.Errorf("第一次 add 应显示 25%%: %q", line1)
	}
	buf.Reset()
	expireThrottle(p)
	p.add(10)
	line2 := buf.String()
	if !strings.Contains(line2, "2/4") || !strings.Contains(line2, "50%") {
		t.Errorf("第二次 add 输出不符合预期: %q", line2)
	}
	buf.Reset()
	p.finish()
	fin := buf.String()
	if !strings.HasPrefix(fin, "\r") || !strings.HasSuffix(fin, "\r") {
		t.Errorf("finish 应以 \\r 开头和结尾清理进度行: %q", fin)
	}
}

// TestProgressZeroTotal 验证待下载总数为 0（不应发生，防御）时不渲染也不清行。
func TestProgressZeroTotal(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.render()
	p.finish()
	if buf.Len() != 0 {
		t.Errorf("total=0 时不应输出: %q", buf.String())
	}
}
