package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"

	"filespace/internal/share"
)

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

// writeError 输出 JSON 错误响应。
func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeFolderError 把 share 包的错误映射为 HTTP 状态码。
func writeFolderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, share.ErrFolderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, share.ErrPathForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

// isLoopbackRequest 判断请求是否来自本机（回环地址 127.0.0.1 / ::1）。
// 本机专属端点（增删共享、改密、打开文件、目录选择器）与访问密码豁免都以此为准。
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	return ip.IsLoopback()
}

// withCORS 允许浏览器跨域直连其他节点（局域网 P2P）。
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
