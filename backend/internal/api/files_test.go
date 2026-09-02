package api

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// 验证下载响应的 Content-Disposition：ASCII 文件名直接可用，
// 非 ASCII（中文等）文件名通过 filename*（RFC 5987）携带原始名称，
// 同时提供 ASCII 兜底 filename，且兜底值不含头注入风险字符。
func TestSetDownloadDisposition(t *testing.T) {
	cases := []struct {
		name     string
		wantStar bool // 是否应出现 filename*
	}{
		{"report.pdf", false},
		{"archive.tar.gz", false},
		{"中文文档 说明.txt", true},
		{"纯文本.txt", true},
		{`we"ird;name.csv`, false},
		{"a\nb.txt", false},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		setDownloadDisposition(w, tc.name)
		got := w.Header().Get("Content-Disposition")
		if got == "" {
			t.Fatalf("文件名 %q：Content-Disposition 为空", tc.name)
		}
		if !strings.HasPrefix(got, "attachment; filename=") {
			t.Errorf("文件名 %q：Content-Disposition 缺少 attachment; filename=，实际 %q", tc.name, got)
		}
		hasStar := strings.Contains(got, "filename*=UTF-8''")
		if hasStar != tc.wantStar {
			t.Errorf("文件名 %q：filename* 出现 = %v，期望 %v，实际 %q", tc.name, hasStar, tc.wantStar, got)
		}
		// 非 ASCII 文件名：filename* 解码后必须还原原始文件名
		if tc.wantStar {
			raw := got[strings.Index(got, "filename*=UTF-8''")+len("filename*=UTF-8''"):]
			dec, err := url.QueryUnescape(raw)
			if err != nil {
				t.Fatalf("文件名 %q：filename* 解码失败 %v", tc.name, err)
			}
			if dec != tc.name {
				t.Errorf("文件名 %q：filename* 解码 = %q，期望还原原文件名", tc.name, dec)
			}
		}
		// ASCII 兜底值不得含引号、反斜杠、分号、控制字符
		rest := got[len("attachment; filename="):]
		if idx := strings.Index(rest, ";"); idx >= 0 {
			rest = rest[:idx]
		}
		fallback := strings.Trim(rest, `"`)
		for _, r := range fallback {
			if r < 0x20 || r == '"' || r == '\\' || r == ';' || r == ',' {
				t.Errorf("文件名 %q：兜底值 %q 含非法字符 %q", tc.name, fallback, r)
			}
		}
	}
}

// 验证 encodeRFC5987 的百分号编码结果。
func TestEncodeRFC5987(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"report.pdf", "report.pdf"},
		{"中文 文档.txt", "%E4%B8%AD%E6%96%87%20%E6%96%87%E6%A1%A3.txt"},
		{"a'b*c", "a%27b%2Ac"},
	}
	for _, tc := range cases {
		if got := encodeRFC5987(tc.in); got != tc.want {
			t.Errorf("encodeRFC5987(%q) = %q，期望 %q", tc.in, got, tc.want)
		}
	}
}
