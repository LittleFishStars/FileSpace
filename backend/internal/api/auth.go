package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// authRequest 认证请求体。
type authRequest struct {
	Password string `json:"password"`
}

// handleAuth 校验访问密码并签发访问令牌（POST /api/auth）。
// 密码须匹配本节点某个设置了密码的共享文件夹（错误密码立即返回 401）；
// 本节点没有任何需要密码的文件夹时返回 404，调用方据此判断无需输入密码。
// 令牌签发与「令牌↔密码哈希」绑定细节见 internal/auth.Tokens。
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	if !s.folders.HasPassword() {
		writeError(w, http.StatusNotFound, "本节点没有需要密码的共享文件夹")
		return
	}
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误: "+err.Error())
		return
	}
	if !s.folders.MatchPassword(req.Password) {
		writeError(w, http.StatusUnauthorized, "密码错误")
		return
	}
	token, ok := s.auth.Issue(req.Password)
	if !ok {
		writeError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	writeJSON(w, map[string]any{"token": token})
}

// authorizedFolderID 从路径参数取出共享文件夹 id 并完成访问鉴权：
// 通过时返回 id 与 true；未通过（需密码但无有效令牌）时已写出 401 响应，
// 返回 false，调用方应直接结束处理。供 tree/download 等远程内容端点复用，
// 避免各处理函数重复「取 id + 鉴权 + 写 401」三行样板。
func (s *Server) authorizedFolderID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !s.authorized(r, id) {
		writeError(w, http.StatusUnauthorized, "需要访问密码（未认证或令牌已过期）")
		return "", false
	}
	return id, true
}

// authorized 判断请求是否有权访问指定共享文件夹的内容：
// 回环请求（本机）放行；文件夹不存在或未设置密码时放行（不存在的由业务逻辑返回 404）；
// 设置了密码时校验访问令牌，且令牌须绑定该文件夹的密码哈希（同密码的文件夹可共用令牌）。
// 哈希以 auth.Hash 值类型直达令牌管理器，无需字符串来回编解码。
func (s *Server) authorized(r *http.Request, folderID string) bool {
	if isLoopbackRequest(r) {
		return true
	}
	passHash, ok := s.folders.FolderPasswdHash(folderID)
	if !ok || passHash.IsEmpty() {
		return true
	}
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	return s.auth.Validate(token, passHash)
}

// bearerToken 从 Authorization: Bearer <token> 取出令牌，格式不符返回空串。
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}
