package api

import (
	"net/http"

	"filespace/internal/sync"
)

// syncAddRequest 「创建同步文件夹」的请求体（本机回环调用）：
// 远程定位符 + 本地同步路径 + 可选访问密码。
type syncAddRequest struct {
	sync.Spec
}

// handleSyncAdd 由命令行 -s/--sync 在本机已有后端运行时调用：把同步任务交给
// 已运行的后端注册并立即启动（等价于带 -s 直接启动该后端）。仅允许本机调用。
func (s *Server) handleSyncAdd(w http.ResponseWriter, r *http.Request) {
	if !requireLoopback(w, r) {
		return
	}
	var req syncAddRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.FolderID == "" || req.Local == "" || req.Host == "" {
		writeError(w, http.StatusBadRequest, "host/folder_id/local 不能为空")
		return
	}
	if s.syncs == nil {
		writeError(w, http.StatusServiceUnavailable, "同步功能不可用")
		return
	}
	s.syncs.Add(req.Spec)
	writeJSON(w, map[string]any{"added": req.Local})
}

// handleSyncStatus 返回全部后台同步任务的实时状态快照（供前端右下角浮动
// 进度条轮询：阶段 + 进度分数 + 下载速度）。仅允许本机调用。
func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if !requireLoopback(w, r) {
		return
	}
	if s.syncs == nil {
		writeJSON(w, map[string]any{"tasks": []any{}})
		return
	}
	writeJSON(w, map[string]any{"tasks": s.syncs.List()})
}
