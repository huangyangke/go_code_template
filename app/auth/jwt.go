package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type tokenType string

const (
	tokenTypeAccess  tokenType = "access"
	tokenTypeRefresh tokenType = "refresh"
)

type jwtClaims struct {
	jwt.RegisteredClaims
	UID       string         `json:"uid"`
	Scopes    []string       `json:"scopes,omitempty"`
	TokenData map[string]any `json:"td,omitempty"`
	SID       string         `json:"sid,omitempty"`
	Type      tokenType      `json:"type"`
	Ver       int64          `json:"ver,omitempty"`
}

func newJTI() string {
	return uuid.New().String()
}

// issueTokenBundle 签发 access+refresh 令牌对.
func (m *Manager) issueTokenBundle(result *AuthResult, newSession bool) (*TokenBundle, error) {
	sid := result.SID
	if m.cfg.UseSID && newSession {
		sid = newJTI()
	}

	accessToken, _, err := m.issueToken(tokenTypeAccess, result, sid)
	if err != nil {
		return nil, err
	}

	bundle := &TokenBundle{
		AccessToken: accessToken,
		TokenType:   "bearer",
		ExpiresIn:   int64(m.cfg.AccessTokenTTL.Seconds()),
		User:        result.User,
	}

	if m.cfg.EnableRefresh {
		refreshToken, _, err := m.issueToken(tokenTypeRefresh, result, sid)
		if err != nil {
			return nil, err
		}
		bundle.RefreshToken = refreshToken
	}

	return bundle, nil
}

func (m *Manager) issueToken(typ tokenType, result *AuthResult, sid string) (string, string, error) {
	now := time.Now()

	ttl := m.cfg.AccessTokenTTL
	if typ == tokenTypeRefresh {
		ttl = m.cfg.RefreshTokenTTL
	}

	// 刷新令牌始终包含 JTI（轮换/撤销时需要），访问令牌仅在 UseJTI 时包含
	jti := ""
	if typ == tokenTypeRefresh || m.cfg.UseJTI {
		jti = newJTI()
	}

	c := &jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   result.UID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			ID:        jti,
		},
		UID:       result.UID,
		Scopes:    result.Scopes,
		TokenData: result.TokenData,
		SID:       sid,
		Type:      typ,
		Ver:       result.TokenVersion,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, err := token.SignedString([]byte(m.cfg.Secret))
	return signed, jti, err
}

func (m *Manager) parseToken(tokenStr string) (*jwtClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &jwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		// 锁定 HS256 防止算法混淆攻击
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(m.cfg.Secret), nil
	})
	if err != nil {
		return nil, err
	}
	c, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return c, nil
}

// revocationKey 返回撤销回调使用的键，有 JTI 时用 JTI，否则用令牌 sha256.
func revocationKey(c *jwtClaims, tokenStr string) string {
	if c.ID != "" {
		return c.ID
	}
	h := sha256.Sum256([]byte(tokenStr))
	return hex.EncodeToString(h[:])
}

// extractBearerToken 从 Authorization 头提取 Bearer 令牌.
func extractBearerToken(authHeader string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(authHeader, prefix)
	return token, token != ""
}
