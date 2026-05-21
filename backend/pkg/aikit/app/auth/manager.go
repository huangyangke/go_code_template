package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/example/go-template/pkg/aikit/app/response"
)

// Config holds all configuration for Manager.
type Config struct {
	Secret string // JWT signing secret (required)

	// Token lifetimes
	AccessTokenTTL  time.Duration // default 15m
	RefreshTokenTTL time.Duration // default 7d

	// Route prefix (default "/auth")
	Prefix string

	// Callbacks — Authenticate is required; others are optional.
	Authenticate      func(ctx context.Context, username, password string) (*AuthResult, error)
	GetSubject        func(ctx context.Context, uid string) (map[string]any, error)
	RegisterUser      func(ctx context.Context, req RegisterRequest) (*AuthResult, error)
	OAuthAuthenticate func(ctx context.Context, identity OAuthIdentity) (*AuthResult, error)

	// Route toggles — auto-detected from callbacks when zero.
	EnableRegister          bool
	EnableRefresh           bool
	EnableLogout            bool
	EnableMe                bool
	EnableOAuth             bool
	AutoLoginAfterRegister  bool
	RequireTokenOnLogout    bool // default true
	DisableRefresh          bool
	DisableLogout           bool
	AllowLogoutWithoutToken bool

	// Token features
	UseJTI          bool
	UseSID          bool
	RefreshRotation bool // requires IsTokenRevoked + RevokeToken

	// Revocation callbacks (all optional)
	IsTokenRevoked  func(ctx context.Context, key string) (bool, error)
	RevokeToken     func(ctx context.Context, key string) error
	RevokeBySID     func(ctx context.Context, sid string) error
	GetTokenVersion func(ctx context.Context, uid string) (int64, error)

	// Lifecycle hooks (optional)
	OnLoginAttempt    func(c *gin.Context) error
	OnRegisterAttempt func(c *gin.Context) error
	OnTokenReuse      func(c *gin.Context, uid string)

	// Password policy applied during registration (nil = disabled)
	PasswordPolicy *PasswordPolicy

	// Set auth cookies in addition to returning tokens in body
	SetCookies bool

	// OAuth providers
	Providers        map[string]OAuthProviderConfig
	OAuthStateSecret string        // defaults to Secret
	OAuthStateMaxAge time.Duration // default 10m

	// Error message for wrong credentials
	InvalidCredentialsMsg string
}

// Manager handles JWT auth routes and middleware for a Gin application.
type Manager struct {
	cfg Config
}

// New creates a Manager from the given Config.
func New(cfg Config) (*Manager, error) {
	if cfg.Secret == "" {
		return nil, errors.New("auth: Secret is required")
	}
	if cfg.Authenticate == nil {
		return nil, errors.New("auth: Authenticate callback is required")
	}
	applyDefaults(&cfg)
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	return &Manager{cfg: cfg}, nil
}

// NewFromService builds a Manager from a UserService, auto-wiring callbacks.
// If svc also implements UserCreator, the /register route is enabled automatically.
func NewFromService(svc UserService, cfg Config) (*Manager, error) {
	hasher := BcryptHasher{}

	if cfg.Authenticate == nil {
		cfg.Authenticate = func(ctx context.Context, username, password string) (*AuthResult, error) {
			user, err := svc.GetUserByUsername(ctx, username)
			if err != nil {
				if errors.Is(err, ErrUserNotFound) {
					return nil, nil
				}
				return nil, err
			}
			if user == nil {
				return nil, nil
			}
			if !hasher.Verify(password, user.GetPasswordHash()) {
				return nil, nil
			}
			var tokenVersion int64
			if cfg.GetTokenVersion != nil {
				tokenVersion, err = cfg.GetTokenVersion(ctx, user.GetID())
				if err != nil {
					return nil, err
				}
			}
			return &AuthResult{
				UID:          user.GetID(),
				Scopes:       user.GetScopes(),
				TokenVersion: tokenVersion,
				User:         user.ToMap(),
				Fresh:        true,
			}, nil
		}
	}

	if cfg.GetSubject == nil {
		cfg.GetSubject = func(ctx context.Context, uid string) (map[string]any, error) {
			user, err := svc.GetUserByID(ctx, uid)
			if err != nil {
				return nil, err
			}
			if user == nil {
				return nil, ErrUserNotFound
			}
			return user.ToMap(), nil
		}
	}

	if creator, ok := svc.(UserCreator); ok && cfg.RegisterUser == nil {
		cfg.RegisterUser = func(ctx context.Context, req RegisterRequest) (*AuthResult, error) {
			extra := map[string]any{}
			if req.Email != nil {
				extra["email"] = *req.Email
			}
			if req.DisplayName != nil {
				extra["display_name"] = *req.DisplayName
			}
			user, err := creator.CreateUser(ctx, req.Username, req.Password, extra)
			if err != nil {
				return nil, err
			}
			var tokenVersion int64
			if cfg.GetTokenVersion != nil {
				tokenVersion, err = cfg.GetTokenVersion(ctx, user.GetID())
				if err != nil {
					return nil, err
				}
			}
			return &AuthResult{
				UID:          user.GetID(),
				Scopes:       user.GetScopes(),
				TokenVersion: tokenVersion,
				User:         user.ToMap(),
			}, nil
		}
	}

	return New(cfg)
}

func applyDefaults(cfg *Config) {
	if cfg.Prefix == "" {
		cfg.Prefix = "/auth"
	}
	if cfg.AccessTokenTTL == 0 {
		cfg.AccessTokenTTL = 15 * time.Minute
	}
	if cfg.RefreshTokenTTL == 0 {
		cfg.RefreshTokenTTL = 7 * 24 * time.Hour
	}
	if cfg.OAuthStateMaxAge == 0 {
		cfg.OAuthStateMaxAge = 10 * time.Minute
	}
	if cfg.OAuthStateSecret == "" {
		cfg.OAuthStateSecret = cfg.Secret
	}
	if cfg.InvalidCredentialsMsg == "" {
		cfg.InvalidCredentialsMsg = "用户名或密码错误"
	}
	// auto-enable routes based on provided callbacks
	if cfg.GetSubject != nil {
		cfg.EnableMe = true
	}
	if cfg.RegisterUser != nil {
		cfg.EnableRegister = true
	}
	if cfg.OAuthAuthenticate != nil && len(cfg.Providers) > 0 {
		cfg.EnableOAuth = true
	}
	if cfg.DisableRefresh {
		cfg.EnableRefresh = false
	} else if !cfg.EnableRefresh {
		cfg.EnableRefresh = true
	}
	if cfg.DisableLogout {
		cfg.EnableLogout = false
	} else if !cfg.EnableLogout {
		cfg.EnableLogout = true
	}
	if cfg.AllowLogoutWithoutToken {
		cfg.RequireTokenOnLogout = false
	} else if !cfg.RequireTokenOnLogout {
		cfg.RequireTokenOnLogout = true
	}
}

func validateConfig(cfg *Config) error {
	if cfg.RefreshRotation && (cfg.IsTokenRevoked == nil || cfg.RevokeToken == nil) {
		return errors.New("auth: RefreshRotation requires IsTokenRevoked and RevokeToken callbacks")
	}
	return nil
}

// Prefix returns the route prefix used by this manager.
func (m *Manager) Prefix() string {
	return m.cfg.Prefix
}

// RegisterRoutes registers all auth endpoints on the given router.
func (m *Manager) RegisterRoutes(r gin.IRouter) {
	g := r.Group(m.cfg.Prefix)
	g.POST("/login", m.handleLogin)
	if m.cfg.EnableRegister {
		g.POST("/register", m.handleRegister)
	}
	if m.cfg.EnableRefresh {
		g.POST("/refresh", m.handleRefresh)
	}
	if m.cfg.EnableLogout {
		g.POST("/logout", m.handleLogout)
	}
	if m.cfg.EnableMe {
		g.GET("/me", m.AuthRequired(), m.handleMe)
	}
	if m.cfg.EnableOAuth {
		for name, p := range m.cfg.Providers {
			p := p // capture
			n := name
			g.GET("/oauth/"+n, m.handleOAuthAuthorize(n, p))
			g.GET("/oauth/"+n+"/callback", m.handleOAuthCallback(n, p))
		}
	}
}

// AuthRequired returns a Gin middleware that validates the Bearer access token
// and sets ContextKeyUID, ContextKeyScopes, ContextKeyTokenData, ContextKeySID.
func (m *Manager) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			// fall back to cookie
			tokenStr, _ = c.Cookie("access_token")
		}
		if tokenStr == "" {
			response.Unauthorized(c, "missing access token")
			c.Abort()
			return
		}

		claims, err := m.parseToken(tokenStr)
		if err != nil || claims.Type != tokenTypeAccess {
			response.Unauthorized(c, "invalid access token")
			c.Abort()
			return
		}

		if err := m.checkRevocation(c.Request.Context(), claims, tokenStr); err != nil {
			response.Unauthorized(c, err.Error())
			c.Abort()
			return
		}

		// Check token version (matches Python behavior: version check on every verification)
		if m.cfg.GetTokenVersion != nil && claims.Ver > 0 {
			ver, err := m.cfg.GetTokenVersion(c.Request.Context(), claims.UID)
			if err != nil {
				response.Unauthorized(c, "token version check error")
				c.Abort()
				return
			}
			if ver != claims.Ver {
				response.Unauthorized(c, "token version mismatch, please re-login")
				c.Abort()
				return
			}
		}

		c.Set(ContextKeyUID, claims.UID)
		c.Set(ContextKeyScopes, claims.Scopes)
		c.Set(ContextKeyTokenData, claims.TokenData)
		c.Set(ContextKeySID, claims.SID)
		c.Next()
	}
}

// RequireScopes returns middleware that checks the user (set by AuthRequired) has all required scopes.
func (m *Manager) RequireScopes(required ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scopes := GetScopes(c)
		scopeSet := make(map[string]struct{}, len(scopes))
		for _, s := range scopes {
			scopeSet[s] = struct{}{}
		}
		for _, req := range required {
			if _, ok := scopeSet[req]; !ok {
				response.Forbidden(c, fmt.Sprintf("missing required scope: %s", req))
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// ── handlers ──────────────────────────────────────────────────────────────────

func (m *Manager) handleLogin(c *gin.Context) {
	if m.cfg.OnLoginAttempt != nil {
		if err := m.cfg.OnLoginAttempt(c); err != nil {
			return
		}
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, err.Error())
		return
	}

	result, err := m.cfg.Authenticate(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		response.InternalError(c, "authentication error")
		return
	}
	if result == nil {
		response.Unauthorized(c, m.cfg.InvalidCredentialsMsg)
		return
	}

	bundle, err := m.issueTokenBundle(result, true)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}
	if m.cfg.SetCookies {
		m.setCookies(c, bundle)
	}
	response.JSON(c, bundle, "")
}

func (m *Manager) handleRegister(c *gin.Context) {
	if m.cfg.OnRegisterAttempt != nil {
		if err := m.cfg.OnRegisterAttempt(c); err != nil {
			return
		}
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, err.Error())
		return
	}

	if m.cfg.PasswordPolicy != nil {
		if errs := m.cfg.PasswordPolicy.Validate(req.Password); len(errs) > 0 {
			response.ParamError(c, strings.Join(errs, "; "))
			return
		}
	}

	result, err := m.cfg.RegisterUser(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, ErrUserAlreadyExists) {
			response.Conflict(c, "用户已存在")
			return
		}
		response.InternalError(c, "registration failed")
		return
	}

	if m.cfg.AutoLoginAfterRegister {
		bundle, err := m.issueTokenBundle(result, true)
		if err != nil {
			response.InternalError(c, "failed to issue token")
			return
		}
		if m.cfg.SetCookies {
			m.setCookies(c, bundle)
		}
		response.JSON(c, bundle, "")
		return
	}

	response.JSON(c, map[string]any{"uid": result.UID, "user": result.User}, "")
}

func (m *Manager) handleRefresh(c *gin.Context) {
	tokenStr, ok := extractBearerToken(c.GetHeader("Authorization"))
	if !ok {
		tokenStr, _ = c.Cookie("refresh_token")
	}
	if tokenStr == "" {
		response.Unauthorized(c, "missing refresh token")
		return
	}

	claims, err := m.parseToken(tokenStr)
	if err != nil || claims.Type != tokenTypeRefresh {
		response.Unauthorized(c, "invalid refresh token")
		return
	}

	// Check revocation — a revoked refresh token under rotation signals reuse attack.
	if m.cfg.IsTokenRevoked != nil {
		key := revocationKey(claims, tokenStr)
		revoked, err := m.cfg.IsTokenRevoked(c.Request.Context(), key)
		if err != nil {
			response.InternalError(c, "token validation error")
			return
		}
		if revoked {
			if m.cfg.RefreshRotation && m.cfg.RevokeBySID != nil && claims.SID != "" {
				_ = m.cfg.RevokeBySID(c.Request.Context(), claims.SID)
			}
			if m.cfg.OnTokenReuse != nil {
				m.cfg.OnTokenReuse(c, claims.UID)
			}
			response.Unauthorized(c, "refresh token has been revoked")
			return
		}
	}

	// Check token version.
	if m.cfg.GetTokenVersion != nil && claims.Ver > 0 {
		ver, err := m.cfg.GetTokenVersion(c.Request.Context(), claims.UID)
		if err != nil {
			response.InternalError(c, "token version check error")
			return
		}
		if ver != claims.Ver {
			response.Unauthorized(c, "token version mismatch, please re-login")
			return
		}
	}

	// Revoke old refresh token before issuing new one (rotation).
	if m.cfg.RefreshRotation && m.cfg.RevokeToken != nil {
		_ = m.cfg.RevokeToken(c.Request.Context(), revocationKey(claims, tokenStr))
	}

	result := &AuthResult{
		UID:          claims.UID,
		Scopes:       claims.Scopes,
		TokenData:    claims.TokenData,
		TokenVersion: claims.Ver,
		SID:          claims.SID,
	}
	bundle, err := m.issueTokenBundle(result, false)
	if err != nil {
		response.InternalError(c, "failed to issue token")
		return
	}
	if m.cfg.SetCookies {
		m.setCookies(c, bundle)
	}
	response.JSON(c, bundle, "")
}

func (m *Manager) handleLogout(c *gin.Context) {
	tokenStr, ok := extractBearerToken(c.GetHeader("Authorization"))
	if !ok {
		tokenStr, _ = c.Cookie("access_token")
	}

	if m.cfg.RequireTokenOnLogout && tokenStr == "" {
		response.Unauthorized(c, "missing access token")
		return
	}

	if tokenStr != "" && m.cfg.RevokeToken != nil {
		if claims, err := m.parseToken(tokenStr); err == nil {
			_ = m.cfg.RevokeToken(c.Request.Context(), revocationKey(claims, tokenStr))
		}
	}

	if m.cfg.SetCookies {
		m.clearCookies(c)
	}
	response.JSON(c, nil, "")
}

func (m *Manager) handleMe(c *gin.Context) {
	uid := GetUID(c)
	if uid == "" {
		response.Unauthorized(c, "not authenticated")
		return
	}
	user, err := m.cfg.GetSubject(c.Request.Context(), uid)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.UserNotFound(c, "user not found")
			return
		}
		response.InternalError(c, "failed to fetch user")
		return
	}
	response.JSON(c, user, "")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (m *Manager) checkRevocation(ctx context.Context, claims *jwtClaims, tokenStr string) error {
	if m.cfg.IsTokenRevoked == nil {
		return nil
	}
	key := revocationKey(claims, tokenStr)
	revoked, err := m.cfg.IsTokenRevoked(ctx, key)
	if err != nil {
		return fmt.Errorf("token validation error")
	}
	if revoked {
		return fmt.Errorf("token has been revoked")
	}
	return nil
}

func (m *Manager) setCookies(c *gin.Context, bundle *TokenBundle) {
	secure := c.Request.TLS != nil
	c.SetCookie("access_token", bundle.AccessToken, int(m.cfg.AccessTokenTTL.Seconds()), "/", "", secure, true)
	if bundle.RefreshToken != "" {
		c.SetCookie("refresh_token", bundle.RefreshToken, int(m.cfg.RefreshTokenTTL.Seconds()), "/", "", secure, true)
	}
}

func (m *Manager) clearCookies(c *gin.Context) {
	c.SetCookie("access_token", "", -1, "/", "", false, true)
	c.SetCookie("refresh_token", "", -1, "/", "", false, true)
}

func fetchJSON(client *http.Client, url string, dest any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}
