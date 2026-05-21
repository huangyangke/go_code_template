package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
)

type tokenType string

const (
	tokenTypeAccess  tokenType = "access"
	tokenTypeRefresh tokenType = "refresh"
)

type jwtClaims struct {
	jwt.StandardClaims
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

// issueTokenBundle issues an access+refresh pair and wraps them in a TokenBundle.
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

	// Refresh tokens always get a JTI (needed for rotation/revocation).
	// Access tokens get a JTI only when UseJTI is set.
	jti := ""
	if typ == tokenTypeRefresh || m.cfg.UseJTI {
		jti = newJTI()
	}

	c := &jwtClaims{
		StandardClaims: jwt.StandardClaims{
			Subject:   result.UID,
			IssuedAt:  now.Unix(),
			ExpiresAt: now.Add(ttl).Unix(),
			Id:        jti,
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
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
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

// revocationKey returns the string key used for revocation callbacks.
// Uses JTI when available, otherwise falls back to sha256 of the raw token.
func revocationKey(c *jwtClaims, tokenStr string) string {
	if c.Id != "" {
		return c.Id
	}
	h := sha256.Sum256([]byte(tokenStr))
	return hex.EncodeToString(h[:])
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(authHeader string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(authHeader, prefix)
	return token, token != ""
}
