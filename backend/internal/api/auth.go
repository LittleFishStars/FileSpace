package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

// authTTL 访问令牌有效期（到期后需重新输入密码）。
const authTTL = 24 * time.Hour

// tokenInfo 一个访问令牌的认证信息：绑定密码哈希（同一密码的多个文件夹可共用同一令牌）。
type tokenInfo struct {
	passHash [32]byte
	exp      time.Time
}

// authManager 管理已签发的访问令牌。
// 密码本身不驻留在此：授权时按目标文件夹的密码哈希与令牌绑定的密码哈希比较。
type authManager struct {
	mu     sync.Mutex
	tokens map[string]tokenInfo
}

// newAuthManager 创建认证管理器。
func newAuthManager() *authManager {
	return &authManager{tokens: make(map[string]tokenInfo)}
}

// issue 校验密码（空密码不签发），正确则签发一个访问令牌（有效期 authTTL）。
func (a *authManager) issue(password string) (string, bool) {
	if password == "" {
		return "", false
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false
	}
	token := hex.EncodeToString(buf)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.purgeLocked()
	a.tokens[token] = tokenInfo{
		passHash: sha256.Sum256([]byte(password)),
		exp:      time.Now().Add(authTTL),
	}
	return token, true
}

// valid 判断令牌是否适用于指定密码哈希（绑定相同密码的令牌才放行）。
func (a *authManager) valid(token string, passHash [32]byte) bool {
	if token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	info, exists := a.tokens[token]
	if !exists {
		return false
	}
	if time.Now().After(info.exp) {
		delete(a.tokens, token)
		return false
	}
	return subtle.ConstantTimeCompare(info.passHash[:], passHash[:]) == 1
}

// purgeLocked 清理已过期的令牌（调用方须持有锁）。
func (a *authManager) purgeLocked() {
	now := time.Now()
	for tok, info := range a.tokens {
		if now.After(info.exp) {
			delete(a.tokens, tok)
		}
	}
}

// bearerToken 从 Authorization: Bearer <token> 取出令牌，格式不符返回空串。
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// authRequest 认证请求体。
type authRequest struct {
	Password string `json:"password"`
}

// handleAuth 校验访问密码并签发访问令牌（POST /api/auth）。
// 密码须匹配本节点某个设置了密码的共享文件夹（错误密码立即返回 401）；
// 本节点没有任何需要密码的文件夹时返回 404，调用方据此判断无需输入密码。
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
	token, ok := s.auth.issue(req.Password)
	if !ok {
		writeError(w, http.StatusInternalServerError, "签发令牌失败")
		return
	}
	writeJSON(w, map[string]any{"token": token})
}

// authorized 判断请求是否有权访问指定共享文件夹的内容：
// 回环请求（本机）放行；文件夹未设置密码时放行；
// 设置了密码时校验访问令牌，且令牌须绑定该文件夹的密码（同密码的文件夹可共用令牌）。
func (s *Server) authorized(r *http.Request, folderID string) bool {
	if isLoopbackRequest(r) {
		return true
	}
	folder, ok := s.folders.Resolve(folderID)
	if !ok {
		return true // 文件夹不存在由业务逻辑返回 404
	}
	if folder.Passwd == "" {
		return true // 该文件夹未设置密码，开放访问
	}
	token := bearerToken(r)
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	return s.auth.valid(token, sha256.Sum256([]byte(folder.Passwd)))
}
