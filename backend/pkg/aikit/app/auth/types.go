package auth

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
)

// Sentinel errors for UserService implementations.
var (
	ErrUserNotFound      = errors.New("auth: user not found")
	ErrUserAlreadyExists = errors.New("auth: user already exists")
)

// User is implemented by business user types to integrate with NewFromService.
type User interface {
	GetID() string
	GetPasswordHash() string
	GetScopes() []string
	ToMap() map[string]any
}

// UserService is the primary interface for adapting business user types.
// Implement UserCreator as well to automatically enable the register route.
type UserService interface {
	GetUserByUsername(ctx context.Context, username string) (User, error)
	GetUserByID(ctx context.Context, uid string) (User, error)
}

// UserCreator is an optional extension of UserService.
// When detected by NewFromService, the /register route is enabled automatically.
type UserCreator interface {
	CreateUser(ctx context.Context, username, password string, extra map[string]any) (User, error)
}

// AuthResult is returned by Authenticate, RegisterUser and OAuthAuthenticate callbacks.
type AuthResult struct {
	UID          string
	Scopes       []string
	TokenData    map[string]any
	TokenVersion int64
	User         map[string]any // returned to client in the token response
	SID          string         // pass existing SID through refresh to maintain session
	Fresh        bool
}

// LoginRequest is the JSON body for POST /auth/login.
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RegisterRequest is the JSON body for POST /auth/register.
type RegisterRequest struct {
	Username    string  `json:"username" binding:"required"`
	Password    string  `json:"password" binding:"required"`
	Email       *string `json:"email"`
	DisplayName *string `json:"display_name"`
}

// TokenBundle is the response body for login / register / refresh.
type TokenBundle struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token,omitempty"`
	TokenType    string         `json:"token_type"`
	ExpiresIn    int64          `json:"expires_in"` // seconds
	User         map[string]any `json:"user,omitempty"`
}

// OAuthIdentity is the normalised third-party identity passed to OAuthAuthenticate.
type OAuthIdentity struct {
	Provider       string
	ProviderUserID string
	Email          string
	EmailVerified  bool
	Name           string
	AvatarURL      string
	RawProfile     map[string]any
}

// OAuthProviderConfig configures a third-party OAuth 2.0 provider.
type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	AuthorizeURL string
	TokenURL     string
	UserinfoURL  string
	EmailsURL    string // GitHub-style secondary emails endpoint
	Scopes       []string
	RedirectURI  string // optional; derived from request if empty
	AccessType   string // e.g. "offline" for Google
	Prompt       string // e.g. "consent"
}

// Gin context keys set by AuthRequired middleware.
const (
	ContextKeyUID       = "auth_uid"
	ContextKeyScopes    = "auth_scopes"
	ContextKeyTokenData = "auth_token_data"
	ContextKeySID       = "auth_sid"
)

// GetUID returns the authenticated user's UID from the Gin context.
// Returns "" if AuthRequired middleware was not applied.
func GetUID(c *gin.Context) string {
	v, _ := c.Get(ContextKeyUID)
	s, _ := v.(string)
	return s
}

// GetScopes returns the authenticated user's scopes from the Gin context.
func GetScopes(c *gin.Context) []string {
	v, _ := c.Get(ContextKeyScopes)
	s, _ := v.([]string)
	return s
}

// GetTokenData returns the custom token_data map from the Gin context.
func GetTokenData(c *gin.Context) map[string]any {
	v, _ := c.Get(ContextKeyTokenData)
	m, _ := v.(map[string]any)
	return m
}
