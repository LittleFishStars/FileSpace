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
// 使下一次 add 必然触发渲染（用于验证渲染内容的测试）。
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

// TestProgressListing 验证「获取文件列表」阶段：beginListing 立即渲染首行，
// scanAdd 累计已发现字节并按总大小显示百分比（无文件数量）；endListing
// 先渲染最终列表结果再切换到下载模式。
func TestProgressListing(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.total = 1024 * 1024 // 文件夹总大小 1 MB
	p.beginListing()
	first := buf.String()
	if !strings.HasPrefix(first, "\r同步 ") || !strings.Contains(first, "获取文件列表") {
		t.Errorf("beginListing 应立即渲染首行: %q", first)
	}
	if !strings.Contains(first, "0 B / 1.0 MB") || !strings.Contains(first, "0%") {
		t.Errorf("beginListing 应显示 0 B / 总大小（0%%）: %q", first)
	}
	if strings.Contains(first, "个文件") {
		t.Errorf("获取文件列表行不应显示文件数量: %q", first)
	}

	// 发现 256 KB 的文件
	buf.Reset()
	expireThrottle(p)
	p.scanAdd(256 * 1024)
	mid := buf.String()
	if !strings.Contains(mid, "256.0 KB / 1.0 MB") || !strings.Contains(mid, "25%") {
		t.Errorf("scanAdd 后应按总大小显示百分比: %q", mid)
	}

	// endListing 先强制渲染最终结果（覆盖节流落下的最后一段），再离开列表模式
	buf.Reset()
	p.scanAdd(768 * 1024) // 累计 1 MB = 100%
	buf.Reset()
	p.endListing()
	end := buf.String()
	if !strings.Contains(end, "1.0 MB / 1.0 MB") || !strings.Contains(end, "100%") {
		t.Errorf("endListing 应强制渲染最终列表结果: %q", end)
	}
	if p.listing {
		t.Error("endListing 后应退出列表模式")
	}
}

// TestProgressListingNoTotal 验证文件夹总大小尚未统计到（total<=0）时，
// 获取文件列表阶段只显示已发现字节、不显示百分比（避免除以 0）。
func TestProgressListingNoTotal(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.beginListing()
	buf.Reset()
	expireThrottle(p)
	p.scanAdd(512)
	line := buf.String()
	if !strings.Contains(line, "获取文件列表") || !strings.Contains(line, "512 B") {
		t.Errorf("total 不可用时应显示已发现字节: %q", line)
	}
	if strings.Contains(line, "%") {
		t.Errorf("total 不可用时不显示百分比: %q", line)
	}
}

// TestProgressListingThrottle 验证列表阶段的节流：连续快速 scanAdd 不会每次写
// 终端；endListing 强制渲染最终结果（与下载阶段的节流语义一致）。
func TestProgressListingThrottle(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.total = 9990
	p.beginListing()
	buf.Reset()
	for i := 0; i < 999; i++ {
		p.scanAdd(10)
	}
	if got := countCR(buf.String()); got > 4 {
		t.Errorf("999 次快速 scanAdd 的刷新次数应被节流（<=4），实际 %d 次", got)
	}
	buf.Reset()
	p.endListing()
	if !strings.Contains(buf.String(), "9.8 KB / 9.8 KB") || !strings.Contains(buf.String(), "100%") {
		t.Errorf("endListing 应强制渲染最终列表结果: %q", buf.String())
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
	p.total = 40
	p.count = 4
	p.add(10)
	p.add(10)
	p.finish()
	if buf.Len() != 0 {
		t.Errorf("非 TTY 进度条不应输出内容，得到: %q", buf.String())
	}
}

// TestProgressRender 验证 TTY 模式下下载阶段的渲染：
//   - 每 add 一次输出一行以 \r 开头的进度，按「已下载字节 / 总字节（百分比）」展示，
//     不显示文件数量；
//   - finish 后输出一行 \r + 空格清行（长度足够覆盖上次内容）。
func TestProgressRender(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.total = 40
	p.count = 4
	p.add(10)
	line1 := buf.String()
	if !strings.HasPrefix(line1, "\r同步 ") || !strings.Contains(line1, "下载 10 B / 40 B") {
		t.Errorf("第一次 add 输出不符合预期: %q", line1)
	}
	if !strings.Contains(line1, "25%") {
		t.Errorf("第一次 add 应显示 25%%: %q", line1)
	}
	if strings.Contains(line1, "个文件") {
		t.Errorf("进度行不应显示文件数量: %q", line1)
	}
	buf.Reset()
	expireThrottle(p)
	p.add(10)
	line2 := buf.String()
	if !strings.Contains(line2, "20 B / 40 B") || !strings.Contains(line2, "50%") {
		t.Errorf("第二次 add 输出不符合预期: %q", line2)
	}
	buf.Reset()
	p.finish()
	fin := buf.String()
	if !strings.HasPrefix(fin, "\r") || !strings.HasSuffix(fin, "\r") {
		t.Errorf("finish 应以 \\r 开头和结尾清理进度行: %q", fin)
	}
}

// TestProgressTotalFromFolderSize 验证用文件夹总大小做分母（大于本轮待下载量）时
// 百分比按总大小计算，即使本轮只下载其中一部分（增量场景）。
func TestProgressTotalFromFolderSize(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.total = 1000 // 文件夹总大小（远端 stats 的 TotalSize）
	p.count = 2    // 本轮只有 2 个文件待下载
	p.add(200)
	line := buf.String()
	if !strings.Contains(line, "200 B / 1000 B") || !strings.Contains(line, "20%") {
		t.Errorf("应按文件夹总大小算百分比: %q", line)
	}
	if strings.Contains(line, "个文件") {
		t.Errorf("进度行不应显示文件数量: %q", line)
	}
}

// TestProgressThrottle 验证节流：连续快速 add（间隔小于 renderInterval）
// 不会每次写终端（实际刷新次数远小于调用次数）；最后一个文件强制刷新最终结果。
func TestProgressThrottle(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	// 先结算一次（lastRender 变为 now），随后快速 add
	p.total = 10000
	p.count = 1000
	p.add(10)

	buf.Reset()
	for i := 0; i < 998; i++ {
		p.add(10)
	}
	if got := countCR(buf.String()); got > 4 {
		t.Errorf("999 次快速 add 的刷新次数应被节流（<=4），实际 %d 次", got)
	}

	// 最后一个文件强制渲染最终结果
	buf.Reset()
	p.add(10)
	if !strings.Contains(buf.String(), "9.8 KB / 9.8 KB") || !strings.Contains(buf.String(), "100%") {
		t.Errorf("最后一个文件应强制渲染 100%%: %q", buf.String())
	}
}

// TestProgressZeroTotal 验证 total=0（不应发生，防御）时不渲染也不清行。
func TestProgressZeroTotal(t *testing.T) {
	var buf bytes.Buffer
	p := newTestProgress(&buf)
	p.render()
	p.finish()
	if buf.Len() != 0 {
		t.Errorf("total=0 时不应输出: %q", buf.String())
	}
}
