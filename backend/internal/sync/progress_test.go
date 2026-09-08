package sync

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

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

// TestProgressRender 验证 TTY 模式下进度条的渲染：
//   - 每 add 一次输出一行以 \r 开头的进度（含文件数、字节、百分比）；
//   - finish 后输出一行 \r + 空格清行（长度足够覆盖上次内容）。
func TestProgressRender(t *testing.T) {
	var buf bytes.Buffer
	p := &progress{
		enabled: true,
		w:       &buf,
		id:      "192.168.1.5:8080:abcd1234",
		start:   time.Now(),
	}
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
	p := &progress{enabled: true, w: &buf, id: "x", total: 0, count: 0}
	p.render()
	p.finish()
	if buf.Len() != 0 {
		t.Errorf("total=0 时不应输出: %q", buf.String())
	}
}
