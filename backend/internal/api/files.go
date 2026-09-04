package api

import (
	"net/http"
	"path/filepath"
	"strings"
)

// handleDownload 下载共享目录内的文件（?path=相对路径，支持 Range 断点续传）。
// 该文件夹设置了访问密码时，远程请求需携带绑定该文件夹密码的有效访问令牌（本机回环放行）。
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorized(r, id) {
		writeError(w, http.StatusUnauthorized, "需要访问密码（未认证或令牌已过期）")
		return
	}
	rel := r.URL.Query().Get("path")
	full, err := s.folders.ResolveFile(id, rel)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	// 显式设置 Content-Disposition：下载 URL 以 /api/folders/{id}/download?path=... 结尾，
	// 没有文件名段，http.ServeFile 也不会设置该头；缺省时浏览器无法解析文件名，
	// 远程下载会回退为占位名 unresolved-filename，二进制文件还会被命名为 .bin。
	setDownloadDisposition(w, filepath.Base(full))
	// http.ServeFile 自动处理 Range、Content-Type 等。
	http.ServeFile(w, r, full)
}

// setDownloadDisposition 设置 attachment 下载头（RFC 6266）：
// filename 提供 ASCII 兜底文件名，filename*（RFC 5987）携带原始文件名，
// 支持中文等非 ASCII 文件名；不支持 filename* 的客户端回退使用 filename。
func setDownloadDisposition(w http.ResponseWriter, name string) {
	disposition := `attachment; filename="` + sanitizeASCII(name) + `"`
	if !isASCII(name) {
		disposition += "; filename*=UTF-8''" + encodeRFC5987(name)
	}
	w.Header().Set("Content-Disposition", disposition)
}

// sanitizeASCII 生成可放进引号内的 ASCII 兜底文件名：
// 非 ASCII 字符与控制字符（含引号、反斜杠、分号等头注入风险字符）替换为 _。
func sanitizeASCII(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 0x20 && r < 0x7f && r != '"' && r != '\\' && r != ';' && r != ',' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

// isASCII 判断字符串是否全部为 ASCII 字节。
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// encodeRFC5987 按 RFC 5987 attr-char 白名单做百分号编码，
// 供 filename* 参数携带原始文件名。
func encodeRFC5987(name string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			strings.ContainsRune("!#$&+-.^_`|~", rune(c)) {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0xf])
		}
	}
	return b.String()
}
