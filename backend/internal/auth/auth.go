// Package auth 提供文件夹级访问密码的核心契约：
//   - Hash：明文密码的 sha256 摘要值类型（内部统一存哈希，永不存明文）；
//   - Tokens：绑定密码哈希的访问令牌签发与校验（内存态、带 TTL）。
//
// 该包只依赖标准库，不感知共享文件夹 / HTTP 传输等业务概念，
// 因此可被 share（保存文件夹密码哈希、匹配登录密码）与
// api（签发 / 校验访问令牌）共同作为单一事实来源依赖，
// 避免「密码→哈希」的算法知识散落在多个包里造成隐式契约。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

// Hash 明文密码的 sha256 摘要。零值表示未设置密码（文件夹对局域网开放）。
// 值类型语义清晰：可空、可按指针取切片做常量时间比较，
// 且直接作为 map/slice 字段存储时无需额外编码。
type Hash [sha256.Size]byte

// OfPassword 计算明文密码的哈希。空密码返回零值 Hash（表示开放）。
func OfPassword(password string) Hash {
	if password == "" {
		return Hash{}
	}
	return Hash(sha256.Sum256([]byte(password)))
}

// IsEmpty 报告哈希是否对应「未设置密码」。
func (h Hash) IsEmpty() bool { return h == Hash{} }

// Hex 返回哈希的十六进制字符串表示，供配置持久化与需要字符串形态的场合使用。
func (h Hash) Hex() string {
	if h.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(h[:])
}

// ParseHex 把十六进制字符串还原为 Hash；输入非法（长度/字符）时返回零值与 false。
// 主要供 share 包从配置文件读取持久化的 passwd_hash 时还原为 Hash；
// 令牌签发/校验一律直接传 Hash 值类型，无需经字符串来回编解码。
func ParseHex(s string) (Hash, bool) {
	if s == "" {
		return Hash{}, false
	}
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != sha256.Size {
		return Hash{}, false
	}
	var h Hash
	copy(h[:], b)
	return h, true
}

// Matches 常量时间比较两个哈希，避免因时序差异泄漏密码信息。
func Matches(a, b Hash) bool {
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// tokenTTL 访问令牌有效期（到期后需重新输入密码）。
const tokenTTL = 24 * time.Hour

// tokenRecord 一个已签发令牌的记录：绑定密码哈希 + 到期时间。
// 同密码的多个文件夹可共用一枚令牌。
type tokenRecord struct {
	passHash Hash
	exp      time.Time
}

// Tokens 内存态访问令牌管理：签发时把明文密码哈希后与令牌绑定；
// 校验时按目标文件夹的哈希比较——只信哈希，明文不驻留。
type Tokens struct {
	mu     sync.Mutex
	tokens map[string]tokenRecord
}

// NewTokens 创建令牌管理器。
func NewTokens() *Tokens {
	return &Tokens{tokens: make(map[string]tokenRecord)}
}

// Issue 用明文密码哈希签发一枚新令牌（有效期 tokenTTL）。
// 空密码不签发（视为「本节点该文件夹不需要密码」）。
func (t *Tokens) Issue(password string) (string, bool) {
	if password == "" {
		return "", false
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false
	}
	token := hex.EncodeToString(buf)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.purgeLocked()
	t.tokens[token] = tokenRecord{passHash: OfPassword(password), exp: time.Now().Add(tokenTTL)}
	return token, true
}

// Validate 判断给定令牌是否对目标哈希有效：存在、未过期且哈希匹配。
// 命中过期令牌时顺手清理，防止长期内存累积。
func (t *Tokens) Validate(token string, target Hash) bool {
	if token == "" || target.IsEmpty() {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	rec, ok := t.tokens[token]
	if !ok {
		return false
	}
	if time.Now().After(rec.exp) {
		delete(t.tokens, token)
		return false
	}
	return Matches(rec.passHash, target)
}

// purgeLocked 清理所有已过期令牌（调用方须持有 t.mu）。
// 签发路径顺带触发，避免只有请求路径才回收的漏洞。
func (t *Tokens) purgeLocked() {
	now := time.Now()
	for tok, rec := range t.tokens {
		if now.After(rec.exp) {
			delete(t.tokens, tok)
		}
	}
}
