package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huangyangke/go-aikit/app/response"
)

// Config Manager 的全部配置.
type Config struct {
	Secret string // JWT 签名密钥（必填）

	// 令牌有效期
	AccessTokenTTL  time.Duration // 缺省 15m
	RefreshTokenTTL time.Duration // 缺省 7d

	// 路由前缀（缺省 "/auth")
	Prefix string

	// 回调函数 — Authenticate 必填，其余可选
	Authenticate      func(ctx context.Context, username, password string) (*AuthResult, error)
	GetSubject        func(ctx context.Context, uid string) (map[string]any, error)
	RegisterUser      func(ctx context.Context, req RegisterRequest) (*AuthResult, error)
	OAuthAuthenticate func(ctx context.Context, identity OAuthIdentity) (*AuthResult, error)

	// 路由开关 — 填零时从回调自动检测
	EnableRegister          bool
	EnableRefresh           bool
	EnableLogout            bool
	EnableMe                bool
	EnableOAuth             bool
	AutoLoginAfterRegister  bool
	RequireTokenOnLogout    bool // 缺省 true
	DisableRefresh          bool
	DisableLogout           bool
	AllowLogoutWithoutToken bool

	// 令牌特性
	UseJTI          bool
	UseSID          bool
	RefreshRotation bool // 需要 IsTokenRevoked + RevokeToken

	// 撤销回调（均可选）
	IsTokenRevoked  func(ctx context.Context, key string) (bool, error)
	RevokeToken     func(ctx context.Context, key string) error
	RevokeBySID     func(ctx context.Context, sid string) error
	GetTokenVersion func(ctx context.Context, uid string) (int64, error)

	// 生命周期钩子（可选）
	OnLoginAttempt    func(c *gin.Context) error
	OnRegisterAttempt func(c *gin.Context) error
	OnTokenReuse      func(c *gin.Context, uid string)

	// 注册时的密码策略（nil 表示禁用）
	PasswordPolicy *PasswordPolicy

	// 除在响应体返回令牌外，还设置 Cookie
	SetCookies bool

	// OAuth 提供方
	Providers        map[string]OAuthProviderConfig
	OAuthStateSecret string        // 缺省等于 Secret
	OAuthStateMaxAge time.Duration // 缺省 10m

	// 认证失败的提示消息
	InvalidCredentialsMsg string
}

// Manager 管理 Gin 应用的 JWT 认证路由和中间件.
type Manager struct {
	cfg Config
}

// New 根据配置创建 Manager.
// 参数：cfg - 认证配置.
// 返回值：mgr - 认证管理器, err - 配置校验失败时的错误.
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

// NewFromService 根据 UserService 自动装配回调并创建 Manager.
// 若 svc 同时实现 UserCreator，自动启用注册路由.
// 参数：svc - 业务用户服务, cfg - 认证配置.
// 返回值：mgr - 认证管理器, err - 配置校验失败时的错误.
func NewFromService(svc UserService, cfg Config) (*Manager, error) {
	hasher := BcryptHasher{}

	if cfg.Authenticate == nil {
		// dummyHash 是合法 bcrypt 哈希，用于掩盖"用户不存在"与"密码错误"的时序差异，防止枚举攻击
		dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

		cfg.Authenticate = func(ctx context.Context, username, password string) (*AuthResult, error) {
			user, err := svc.GetUserByUsername(ctx, username)

			var userHash string
			switch {
			case err != nil && !errors.Is(err, ErrUserNotFound):
				// 先做 bcrypt 校验掩盖时序，再返回错误
				hasher.Verify(password, dummyHash)
				return nil, err
			case err != nil:
				// 用户不存在时使用 dummy hash 掩盖时序
				userHash = dummyHash
			case user == nil:
				userHash = dummyHash
			default:
				userHash = user.GetPasswordHash()
			}

			// 始终执行 bcrypt 以掩盖用户是否存在
			if !hasher.Verify(password, userHash) {
				return nil, nil
			}

			// 使用了 dummy hash 表示实际为用户不存在
			if user == nil || errors.Is(err, ErrUserNotFound) {
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
	// 根据回调自动启用路由
	if cfg.GetSubject != nil {
		cfg.EnableMe = true
	}
	if cfg.RegisterUser != nil {
		cfg.EnableRegister = true
	}
	if cfg.OAuthAuthenticate != nil && len(cfg.Providers) > 0 {
		cfg.EnableOAuth = true
	}
	// DisableXxx 强制关闭；其余默认启用
	if cfg.DisableRefresh {
		cfg.EnableRefresh = false
	} else if !cfg.EnableRefresh {
		cfg.EnableRefresh = true // 未显式禁用时默认启用
	}
	if cfg.DisableLogout {
		cfg.EnableLogout = false
	} else if !cfg.EnableLogout {
		cfg.EnableLogout = true // 默认启用
	}
	if cfg.AllowLogoutWithoutToken {
		cfg.RequireTokenOnLogout = false
	} else if !cfg.RequireTokenOnLogout {
		cfg.RequireTokenOnLogout = true // 默认启用
	}
}

func validateConfig(cfg *Config) error {
	if cfg.RefreshRotation && (cfg.IsTokenRevoked == nil || cfg.RevokeToken == nil) {
		return errors.New("auth: RefreshRotation requires IsTokenRevoked and RevokeToken callbacks")
	}
	return nil
}

// Prefix 返回路由前缀.
// 返回值：prefix - 路由前缀字符串.
func (m *Manager) Prefix() string {
	return m.cfg.Prefix
}

// RegisterRoutes 在路由器上注册全部认证端点.
// 参数：r - Gin 路由器.
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
			p := p // 捕获循环变量
			n := name
			g.GET("/oauth/"+n, m.handleOAuthAuthorize(n, p))
			g.GET("/oauth/"+n+"/callback", m.handleOAuthCallback(n, p))
		}
	}
}

// AuthRequired 返回 Bearer 访问令牌验证中间件.
// 验证后设置 ContextKeyUID、ContextKeyScopes、ContextKeyTokenData、ContextKeySID.
// 参数：无.
// 返回值：gin 中间件 HandlerFunc.
func (m *Manager) AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			// 回退到 Cookie
			tokenStr, _ = c.Cookie("access_token")
		}
		if tokenStr == "" {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		claims, err := m.parseToken(tokenStr)
		if err != nil || claims.Type != tokenTypeAccess {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		if err := m.checkRevocation(c.Request.Context(), claims, tokenStr); err != nil {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		// 检查令牌版本（即使 version=0 也检查，防止新令牌绕过）
		if m.cfg.GetTokenVersion != nil {
			ver, err := m.cfg.GetTokenVersion(c.Request.Context(), claims.UID)
			if err != nil {
				response.Unauthorized(c)
				c.Abort()
				return
			}
			if ver != claims.Ver {
				response.Unauthorized(c)
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

// RequireScopes 返回权限范围校验中间件，要求用户具备所有指定 scope.
// 参数：required - 必需权限列表.
// 返回值：gin 中间件 HandlerFunc.
func (m *Manager) RequireScopes(required ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		scopes := GetScopes(c)
		scopeSet := make(map[string]struct{}, len(scopes))
		for _, s := range scopes {
			scopeSet[s] = struct{}{}
		}
		for _, req := range required {
			if _, ok := scopeSet[req]; !ok {
				response.Forbidden(c)
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// ── handlers ──────────────────────────────────────────────────────────────────.

func (m *Manager) handleLogin(c *gin.Context) {
	if m.cfg.OnLoginAttempt != nil {
		if err := m.cfg.OnLoginAttempt(c); err != nil {
			return
		}
	}

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c)
		return
	}

	result, err := m.cfg.Authenticate(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		response.InternalError(c)
		return
	}
	if result == nil {
		response.Unauthorized(c)
		return
	}

	bundle, err := m.issueTokenBundle(result, true)
	if err != nil {
		response.InternalError(c)
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
		response.ParamError(c)
		return
	}

	if m.cfg.PasswordPolicy != nil {
		if errs := m.cfg.PasswordPolicy.Validate(req.Password); len(errs) > 0 {
			response.ParamError(c)
			return
		}
	}

	result, err := m.cfg.RegisterUser(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, ErrUserAlreadyExists) {
			response.Conflict(c)
			return
		}
		response.InternalError(c)
		return
	}

	if m.cfg.AutoLoginAfterRegister {
		bundle, err := m.issueTokenBundle(result, true)
		if err != nil {
			response.InternalError(c)
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
		response.Unauthorized(c)
		return
	}

	claims, err := m.parseToken(tokenStr)
	if err != nil || claims.Type != tokenTypeRefresh {
		response.Unauthorized(c)
		return
	}

	// 检查撤销 — 轮换模式下已撤销的刷新令牌表明重用攻击
	if m.cfg.IsTokenRevoked != nil {
		key := revocationKey(claims, tokenStr)
		revoked, err := m.cfg.IsTokenRevoked(c.Request.Context(), key)
		if err != nil {
			response.InternalError(c)
			return
		}
		if revoked {
			if m.cfg.RefreshRotation && m.cfg.RevokeBySID != nil && claims.SID != "" {
				_ = m.cfg.RevokeBySID(c.Request.Context(), claims.SID)
			}
			if m.cfg.OnTokenReuse != nil {
				m.cfg.OnTokenReuse(c, claims.UID)
			}
			response.Unauthorized(c)
			return
		}
	}

	// 检查令牌版本（与 AuthRequired 一致，version=0 也检查）
	if m.cfg.GetTokenVersion != nil {
		ver, err := m.cfg.GetTokenVersion(c.Request.Context(), claims.UID)
		if err != nil {
			response.InternalError(c)
			return
		}
		if ver != claims.Ver {
			response.Unauthorized(c)
			return
		}
	}

	result := &AuthResult{
		UID:          claims.UID,
		Scopes:       claims.Scopes,
		TokenData:    claims.TokenData,
		TokenVersion: claims.Ver,
		SID:          claims.SID,
	}
	// 先签发新令牌再撤销旧令牌，避免签发失败导致用户锁定
	bundle, err := m.issueTokenBundle(result, false)
	if err != nil {
		response.InternalError(c)
		return
	}
	// 新令牌签发成功后撤销旧令牌
	if m.cfg.RefreshRotation && m.cfg.RevokeToken != nil {
		_ = m.cfg.RevokeToken(c.Request.Context(), revocationKey(claims, tokenStr))
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
		response.Unauthorized(c)
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
		response.Unauthorized(c)
		return
	}
	user, err := m.cfg.GetSubject(c.Request.Context(), uid)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response.UserNotFound(c)
			return
		}
		response.InternalError(c)
		return
	}
	response.JSON(c, user, "")
}

// ── helpers ───────────────────────────────────────────────────────────────────.

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
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", bundle.AccessToken, int(m.cfg.AccessTokenTTL.Seconds()), "/", "", secure, true)
	if bundle.RefreshToken != "" {
		c.SetCookie("refresh_token", bundle.RefreshToken, int(m.cfg.RefreshTokenTTL.Seconds()), "/", "", secure, true)
	}
}

func (m *Manager) clearCookies(c *gin.Context) {
	secure := c.Request.TLS != nil
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("access_token", "", -1, "/", "", secure, true)
	c.SetCookie("refresh_token", "", -1, "/", "", secure, true)
}

func fetchJSON(client *http.Client, url string, dest any) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned %d", resp.StatusCode)
	}
	// 限制响应体大小为 1MB，防止恶意 OAuth 提供方 DoS
	const maxBodySize = 1 << 20 // 1MB
	return json.NewDecoder(io.LimitReader(resp.Body, maxBodySize)).Decode(dest)
}
