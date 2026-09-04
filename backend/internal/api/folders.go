package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"

	"filespace/internal/model"
	"filespace/internal/share"
)

// handleFolders 返回本节点共享的文件夹列表。
func (s *Server) handleFolders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.folders.List())
}

// addFoldersRequest 追加共享目录的请求体。
type addFoldersRequest struct {
	Paths []string `json:"paths"`
	// Password 新文件夹的可选访问密码（空表示开放）；仅对本机调用有效。
	Password string `json:"password"`
}

// handleAddFolders 追加共享目录（本机另一个 filespace 进程探测到本后端后，把目录交过来；
// 本机管理页添加共享文件夹时亦可指定该文件夹的访问密码）。
// 仅允许本机（回环地址）调用，防止局域网内其他机器随意向本机追加共享目录。
func (s *Server) handleAddFolders(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, "仅允许本机调用")
		return
	}
	var req addFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	if len(req.Paths) == 0 {
		writeError(w, http.StatusBadRequest, "paths 不能为空")
		return
	}
	var added []model.FolderInfo
	for _, p := range req.Paths {
		f, err := s.folders.Add(p, req.Password)
		if errors.Is(err, share.ErrFolderExists) {
			continue // 已在共享列表中，视为成功
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "追加失败: "+err.Error())
			return
		}
		added = append(added, model.FolderInfo{ID: f.ID, Name: f.Name, Path: f.Path, Auth: f.PasswdHash != ""})
	}
	s.persistChanged() // 立即写回配置文件，避免进程被强杀时新增共享（含密码）丢失
	writeJSON(w, map[string]any{"added": added})
}

// removeFolderRequest 移除共享目录的请求体。
type removeFolderRequest struct {
	ID string `json:"id"`
}

// handleRemoveFolder 移除共享目录（仅允许本机调用，与添加共享目录同等安全约束）。
func (s *Server) handleRemoveFolder(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, "仅允许本机调用")
		return
	}
	var req removeFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id 不能为空")
		return
	}
	if err := s.folders.Remove(req.ID); err != nil {
		writeFolderError(w, err)
		return
	}
	s.persistChanged() // 立即写回配置文件，避免进程被强杀后已移除的目录重新复活
	writeJSON(w, map[string]any{"removed": req.ID})
}

// setPasswordRequest 修改共享文件夹访问密码的请求体。
type setPasswordRequest struct {
	// Path 目标文件夹的绝对路径（后端按精确路径或真实路径匹配）。
	Path string `json:"path"`
	// Password 新访问密码：为空表示移除密码（该文件夹恢复开放）。
	Password string `json:"password"`
}

// handleSetFolderPassword 修改本机共享文件夹的访问密码（仅允许本机调用，
// 与添加/移除共享目录同等安全约束）。
// password 为空表示移除密码：文件夹恢复开放，此前签发的访问令牌自动失效
// （令牌绑定密码哈希，移除后授权路径不再校验）。
func (s *Server) handleSetFolderPassword(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		writeError(w, http.StatusForbidden, "仅允许本机调用")
		return
	}
	var req setPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, "path 不能为空")
		return
	}
	f, err := s.folders.SetPassword(req.Path, req.Password)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	s.persistChanged() // 立即写回配置文件（仅存哈希），重启后仍按新密码生效
	writeJSON(w, map[string]any{"updated": f.ID, "name": f.Name, "auth": f.PasswdHash != ""})
}

// isLoopbackRequest 判断请求是否来自本机（回环地址 127.0.0.1 / ::1）。
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

// handleTree 懒加载返回共享目录内的文件列表（?path=相对路径）。
// 该文件夹设置了访问密码时，远程请求需携带绑定该文件夹密码的有效访问令牌（本机回环放行）。
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.authorized(r, id) {
		writeError(w, http.StatusUnauthorized, "需要访问密码（未认证或令牌已过期）")
		return
	}
	rel := r.URL.Query().Get("path")
	entries, err := s.folders.Tree(id, rel)
	if err != nil {
		writeFolderError(w, err)
		return
	}
	writeJSON(w, entries)
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
