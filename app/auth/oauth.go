package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"github.com/huangyangke/go-aikit/app/response"
)

// builtinProviders 内置 OAuth 端点配置.
var builtinProviders = map[string]OAuthProviderConfig{
	"google": {
		AuthorizeURL: "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserinfoURL:  "https://openidconnect.googleapis.com/v1/userinfo",
		Scopes:       []string{"openid", "email", "profile"},
		AccessType:   "offline",
		Prompt:       "consent",
	},
	"github": {
		AuthorizeURL: "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserinfoURL:  "https://api.github.com/user",
		EmailsURL:    "https://api.github.com/user/emails",
		Scopes:       []string{"read:user", "user:email"},
	},
}

func (m *Manager) resolveProvider(name string, p OAuthProviderConfig) (OAuthProviderConfig, error) {
	builtin, ok := builtinProviders[name]
	if ok {
		if p.AuthorizeURL == "" {
			p.AuthorizeURL = builtin.AuthorizeURL
		}
		if p.TokenURL == "" {
			p.TokenURL = builtin.TokenURL
		}
		if p.UserinfoURL == "" {
			p.UserinfoURL = builtin.UserinfoURL
		}
		if p.EmailsURL == "" {
			p.EmailsURL = builtin.EmailsURL
		}
		if len(p.Scopes) == 0 {
			p.Scopes = builtin.Scopes
		}
		if p.AccessType == "" {
			p.AccessType = builtin.AccessType
		}
		if p.Prompt == "" {
			p.Prompt = builtin.Prompt
		}
	}
	if p.AuthorizeURL == "" || p.TokenURL == "" {
		return p, fmt.Errorf("auth: OAuth provider %q is missing AuthorizeURL or TokenURL", name)
	}
	return p, nil
}

// ── state 管理 ──────────────────────────────────────────────────────────────.

type oauthState struct {
	Provider string `json:"p"`
	Nonce    string `json:"n"`
	IssuedAt int64  `json:"t"`
}

func (m *Manager) buildState(provider string) (string, error) {
	s := oauthState{
		Provider: provider,
		Nonce:    newJTI(),
		IssuedAt: time.Now().Unix(),
	}
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	sig := m.signState(encoded)
	return encoded + "." + sig, nil
}

func (m *Manager) verifyState(state, expectedProvider string) error {
	dot := strings.LastIndexByte(state, '.')
	if dot <= 0 {
		return fmt.Errorf("auth: invalid state format")
	}
	encoded, sig := state[:dot], state[dot+1:]
	// 常量时间比较防止时序攻击
	if !hmac.Equal([]byte(m.signState(encoded)), []byte(sig)) {
		return fmt.Errorf("auth: state signature mismatch")
	}

	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("auth: state decode error")
	}
	var s oauthState
	if err := json.Unmarshal(payload, &s); err != nil {
		return fmt.Errorf("auth: state parse error")
	}
	if s.Provider != expectedProvider {
		return fmt.Errorf("auth: state provider mismatch")
	}
	age := time.Since(time.Unix(s.IssuedAt, 0))
	if age > m.cfg.OAuthStateMaxAge {
		return fmt.Errorf("auth: state expired")
	}
	return nil
}

func (m *Manager) signState(payload string) string {
	mac := hmac.New(sha256.New, []byte(m.cfg.OAuthStateSecret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// ── 处理器 ──────────────────────────────────────────────────────────────────.

func (m *Manager) handleOAuthAuthorize(name string, p OAuthProviderConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if m.cfg.OnLoginAttempt != nil {
			if err := m.cfg.OnLoginAttempt(c); err != nil {
				return
			}
		}

		resolved, err := m.resolveProvider(name, p)
		if err != nil {
			response.InternalError(c)
			return
		}

		redirectURI := resolved.RedirectURI
		if redirectURI == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "oauth redirect_uri not configured"})
			return
		}

		oauthCfg := &oauth2.Config{
			ClientID:     resolved.ClientID,
			ClientSecret: resolved.ClientSecret,
			Scopes:       resolved.Scopes,
			Endpoint:     oauth2.Endpoint{AuthURL: resolved.AuthorizeURL, TokenURL: resolved.TokenURL},
			RedirectURL:  redirectURI,
		}

		state, err := m.buildState(name)
		if err != nil {
			response.InternalError(c)
			return
		}

		var opts []oauth2.AuthCodeOption
		if resolved.AccessType != "" {
			opts = append(opts, oauth2.SetAuthURLParam("access_type", resolved.AccessType))
		}
		if resolved.Prompt != "" {
			opts = append(opts, oauth2.SetAuthURLParam("prompt", resolved.Prompt))
		}

		c.Redirect(http.StatusFound, oauthCfg.AuthCodeURL(state, opts...))
	}
}

func (m *Manager) handleOAuthCallback(name string, p OAuthProviderConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if errParam := c.Query("error"); errParam != "" {
			response.BadRequest(c)
			return
		}

		code := c.Query("code")
		state := c.Query("state")
		if code == "" || state == "" {
			response.BadRequest(c)
			return
		}

		if err := m.verifyState(state, name); err != nil {
			response.BadRequest(c)
			return
		}

		resolved, err := m.resolveProvider(name, p)
		if err != nil {
			response.InternalError(c)
			return
		}

		redirectURI := resolved.RedirectURI
		if redirectURI == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "oauth redirect_uri not configured"})
			return
		}

		oauthCfg := &oauth2.Config{
			ClientID:     resolved.ClientID,
			ClientSecret: resolved.ClientSecret,
			Scopes:       resolved.Scopes,
			Endpoint:     oauth2.Endpoint{AuthURL: resolved.AuthorizeURL, TokenURL: resolved.TokenURL},
			RedirectURL:  redirectURI,
		}

		oauthToken, err := oauthCfg.Exchange(c.Request.Context(), code)
		if err != nil {
			response.BadRequest(c)
			return
		}

		httpClient := oauthCfg.Client(c.Request.Context(), oauthToken)

		identity, err := m.fetchOAuthIdentity(httpClient, name, resolved)
		if err != nil {
			response.InternalError(c)
			return
		}

		authResult, err := m.cfg.OAuthAuthenticate(c.Request.Context(), identity)
		if err != nil {
			response.InternalError(c)
			return
		}
		if authResult == nil {
			response.Unauthorized(c)
			return
		}

		bundle, err := m.issueTokenBundle(authResult, true)
		if err != nil {
			response.InternalError(c)
			return
		}
		if m.cfg.SetCookies {
			m.setCookies(c, bundle)
		}
		response.JSON(c, bundle, "")
	}
}

// ── 身份标准化 ──────────────────────────────────────────────────────────────.

func (m *Manager) fetchOAuthIdentity(client *http.Client, provider string, p OAuthProviderConfig) (OAuthIdentity, error) {
	var raw map[string]any
	if err := fetchJSON(client, p.UserinfoURL, &raw); err != nil {
		return OAuthIdentity{}, fmt.Errorf("userinfo fetch: %w", err)
	}

	switch provider {
	case "github":
		return normalizeGitHub(client, raw, p.EmailsURL)
	case "google":
		return normalizeGoogle(raw), nil
	default:
		return normalizeGeneric(provider, raw), nil
	}
}

func normalizeGoogle(raw map[string]any) OAuthIdentity {
	id, _ := raw["sub"].(string)
	email, _ := raw["email"].(string)
	verified, _ := raw["email_verified"].(bool)
	name, _ := raw["name"].(string)
	avatar, _ := raw["picture"].(string)
	return OAuthIdentity{
		Provider:       "google",
		ProviderUserID: id,
		Email:          email,
		EmailVerified:  verified,
		Name:           name,
		AvatarURL:      avatar,
		RawProfile:     raw,
	}
}

func normalizeGitHub(client *http.Client, raw map[string]any, emailsURL string) (OAuthIdentity, error) {
	id := fmt.Sprintf("%v", raw["id"])
	name, _ := raw["name"].(string)
	if name == "" {
		name, _ = raw["login"].(string)
	}
	avatar, _ := raw["avatar_url"].(string)

	// 从专用端点获取主要已验证邮箱
	email := ""
	verified := false
	if emailsURL != "" {
		var emails []map[string]any
		if err := fetchJSON(client, emailsURL, &emails); err == nil {
			for _, e := range emails {
				primary, _ := e["primary"].(bool)
				v, _ := e["verified"].(bool)
				if primary && v {
					email, _ = e["email"].(string)
					verified = v
					break
				}
			}
		}
	}
	if email == "" {
		email, _ = raw["email"].(string)
	}

	return OAuthIdentity{
		Provider:       "github",
		ProviderUserID: id,
		Email:          email,
		EmailVerified:  verified,
		Name:           name,
		AvatarURL:      avatar,
		RawProfile:     raw,
	}, nil
}

func normalizeGeneric(provider string, raw map[string]any) OAuthIdentity {
	id, _ := raw["id"].(string)
	if id == "" {
		id, _ = raw["sub"].(string)
	}
	email, _ := raw["email"].(string)
	name, _ := raw["name"].(string)
	avatar, _ := raw["picture"].(string)
	if avatar == "" {
		avatar, _ = raw["avatar_url"].(string)
	}
	return OAuthIdentity{
		Provider:       provider,
		ProviderUserID: id,
		Email:          email,
		Name:           name,
		AvatarURL:      avatar,
		RawProfile:     raw,
	}
}
