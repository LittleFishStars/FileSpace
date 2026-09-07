package api

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

// setHostnameRequest 修改节点名称的请求体。
type setHostnameRequest struct {
	// Hostname 新的节点显示名称：非空为自定义名称，
	// 空字符串表示恢复系统主机名（与启动配置语义一致）。
	Hostname string `json:"hostname"`
}

// maxHostnameLen 节点名称长度上限（UTF-8 字符数）。
// mDNS TXT 记录每段不超过 255 字节，名称过长会超出协议限制（截断或注册失败）；
// 展示场景（卡片标题/窗口标题）也无需过长名称。
const maxHostnameLen = 64

// handleSetNodeHostname 修改本节点在局域网中显示的名称（仅允许本机调用，
// 与增删共享/改密同等安全约束）。修改后立即生效：
//   - /api/node 的 hostname（本机卡片与其他节点探测到的新名字，无需重启）；
//   - mDNS TXT 宣告（经外层 OnHostname 回调，其他节点列表即时刷新）；
//   - 写回配置文件（经同一回调持久化，重启后仍保留）。
//
// 空字符串表示恢复系统主机名（与 --hostname ” 语义一致）；此时 /api/node
// 向调用方返回系统主机名，同时把「用户输入为空」传给外层回调（其内部清空
// 配置 hostname 并恢复 mDNS 宣告）。
// 与命令行 --hostname 不同：该参数是启动型、运行中不生效；此接口为运行中修改。
func (s *Server) handleSetNodeHostname(w http.ResponseWriter, r *http.Request) {
	if !requireLoopback(w, r) {
		return
	}
	var req setHostnameRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	input := strings.TrimSpace(req.Hostname)
	if utf8.RuneCountInString(input) > maxHostnameLen {
		writeError(w, http.StatusBadRequest, "节点名称过长（最多 "+strconv.Itoa(maxHostnameLen)+" 个字符）")
		return
	}
	display := input
	if display == "" {
		display = s.monitor.Hostname() // 恢复系统主机名
	}
	s.hostname = display // 修改内存显示名：/api/node 立即返回新名称
	if s.hostnameFn != nil {
		s.hostnameFn(input) // 传用户原始输入（空=恢复系统主机名），外层负责 mDNS 与配置持久化
	}
	writeJSON(w, map[string]string{"hostname": display})
}
