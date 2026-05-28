// Package auth JWT 认证与 OAuth 登录，提供路由注册和中间件.
package auth

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
)

// ErrUserNotFound 用户不存在.
// ErrUserAlreadyExists 用户已存在.
var (
	ErrUserNotFound      = errors.New("auth: user not found")
	ErrUserAlreadyExists = errors.New("auth: user already exists")
)

// User 业务用户类型接口，用于 NewFromService 自动装配.
type User interface {
	GetID() string
	GetPasswordHash() string
	GetScopes() []string
	ToMap() map[string]any
}

// UserService 业务用户查询接口，配合 NewFromService 使用.
// 同时实现 UserCreator 可自动启用注册路由.
type UserService interface {
	GetUserByUsername(ctx context.Context, username string) (User, error)
	GetUserByID(ctx context.Context, uid string) (User, error)
}

// UserCreator UserService 的可选扩展，实现后自动启用注册路由.
type UserCreator interface {
	CreateUser(ctx context.Context, username, password string, extra map[string]any) (User, error)
}

// AuthResult 认证结果，由 Authenticate、RegisterUser、OAuthAuthenticate 回调返回.
type AuthResult struct {
	UID          string
	Scopes       []string
	TokenData    map[string]any
	TokenVersion int64
	User         map[string]any // 令牌响应中返回给客户端的用户信息
	SID          string         // 刷新时传入已有 SID 以保持会话
	Fresh        bool
}

// LoginRequest 登录请求体，用于 POST /auth/login.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RegisterRequest 注册请求体，用于 POST /auth/register.
type RegisterRequest struct {
	Username    string  `json:"username" binding:"required"`
	Password    string  `json:"password" binding:"required"`
	Email       *string `json:"email"`
	DisplayName *string `json:"display_name"`
}

// TokenBundle 令牌响应体，用于登录/注册/刷新.
type TokenBundle struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	TokenType    string         `json:"token_type"`
	ExpiresIn    int64          `json:"expires_in"` // 秒
	User         map[string]any `json:"user,omitempty"`
}

// OAuthIdentity 第三方 OAuth 标准化身份信息，传入 OAuthAuthenticate 回调.
type OAuthIdentity struct {
	Provider       string
	ProviderUserID string
	Email          string
	EmailVerified  bool
	Name           string
	AvatarURL      string
	RawProfile     map[string]any
}

// OAuthProviderConfig 第三方 OAuth 2.0 提供方配置.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	AuthorizeURL string
	TokenURL     string
	UserinfoURL  string
	EmailsURL    string // GitHub 式补充邮箱端点
	Scopes       []string
	RedirectURI  string // 可选，缺省从请求推导
	AccessType   string // 如 "offline"（Google）
	Prompt       string // 如 "consent"
}

// ContextKeyUID AuthRequired 中间件设置的 Gin context 键.
const (
	ContextKeyUID       = "auth_uid"
	ContextKeyScopes    = "auth_scopes"
	ContextKeyTokenData = "auth_token_data"
	ContextKeySID       = "auth_sid"
)

// GetUID 从 Gin context 获取已认证用户 UID.
// 参数：c - Gin 上下文.
// 返回值：uid - 用户 ID，未认证时返回空字符串.
func GetUID(c *gin.Context) string {
	v, _ := c.Get(ContextKeyUID)
	s, _ := v.(string)
	return s
}

// GetScopes 从 Gin context 获取已认证用户权限范围.
// 参数：c - Gin 上下文.
// 返回值：scopes - 权限列表.
func GetScopes(c *gin.Context) []string {
	v, _ := c.Get(ContextKeyScopes)
	s, _ := v.([]string)
	return s
}

// GetTokenData 从 Gin context 获取自定义 token_data 映射.
// 参数：c - Gin 上下文.
// 返回值：tokenData - 自定义令牌数据.
func GetTokenData(c *gin.Context) map[string]any {
	v, _ := c.Get(ContextKeyTokenData)
	m, _ := v.(map[string]any)
	return m
}
