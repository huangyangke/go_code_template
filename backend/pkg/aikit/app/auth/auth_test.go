package auth_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/go-template/pkg/aikit/app/auth"
	"github.com/example/go-template/pkg/aikit/app/response"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── fixtures ──────────────────────────────────────────────────────────────────

type fakeUser struct {
	id   string
	hash string
}

func (u *fakeUser) GetID() string           { return u.id }
func (u *fakeUser) GetPasswordHash() string { return u.hash }
func (u *fakeUser) GetScopes() []string     { return []string{"read"} }
func (u *fakeUser) ToMap() map[string]any   { return map[string]any{"id": u.id} }

type fakeUserService struct {
	users map[string]*fakeUser
}

func (s *fakeUserService) GetUserByUsername(_ context.Context, username string) (auth.User, error) {
	u, ok := s.users[username]
	if !ok {
		return nil, auth.ErrUserNotFound
	}
	return u, nil
}

func (s *fakeUserService) GetUserByID(_ context.Context, uid string) (auth.User, error) {
	for _, u := range s.users {
		if u.id == uid {
			return u, nil
		}
	}
	return nil, auth.ErrUserNotFound
}

func newTestManager(t *testing.T, opts ...func(*auth.Config)) *auth.Manager {
	t.Helper()
	hasher := auth.BcryptHasher{}
	hash, err := hasher.Hash("TestPass1")
	require.NoError(t, err)

	svc := &fakeUserService{users: map[string]*fakeUser{
		"alice": {id: "1", hash: hash},
	}}

	cfg := auth.Config{
		Secret: "test-secret-key-32-bytes-long!!!",
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	m, err := auth.NewFromService(svc, cfg)
	require.NoError(t, err)
	return m
}

func setupRouter(m *auth.Manager) *gin.Engine {
	r := gin.New()
	m.RegisterRoutes(r)
	return r
}

func post(t *testing.T, r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func get(t *testing.T, r *gin.Engine, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) response.ApiResponse {
	t.Helper()
	var resp response.ApiResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func dataMap(t *testing.T, resp response.ApiResponse) map[string]any {
	t.Helper()
	b, _ := json.Marshal(resp.Data)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestLogin_Success(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeBody(t, w)
	assert.Equal(t, response.CodeSuccess, resp.Code)
	data := dataMap(t, resp)
	assert.NotEmpty(t, data["access_token"])
	assert.NotEmpty(t, data["refresh_token"])
}

func TestLogin_InvalidCredentials(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "wrongpass"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	resp := decodeBody(t, w)
	assert.Equal(t, response.CodeUnauthorized, resp.Code)
}

func TestLogin_UserNotFound(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "nobody", Password: "TestPass1"})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMe_WithValidToken(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	require.Equal(t, http.StatusOK, w.Code)
	data := dataMap(t, decodeBody(t, w))
	token := data["access_token"].(string)

	w2 := get(t, r, "/auth/me", token)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestMe_NoToken(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)
	w := get(t, r, "/auth/me", "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRefresh_Success(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	data := dataMap(t, decodeBody(t, w))
	refreshToken := data["refresh_token"].(string)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusOK, w2.Code)
	resp := decodeBody(t, w2)
	data2 := dataMap(t, resp)
	assert.NotEmpty(t, data2["access_token"])
}

func TestRefresh_Disabled(t *testing.T) {
	m := newTestManager(t, func(cfg *auth.Config) {
		cfg.DisableRefresh = true
	})
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	require.Equal(t, http.StatusOK, w.Code)
	data := dataMap(t, decodeBody(t, w))
	assert.Empty(t, data["refresh_token"])

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusNotFound, w2.Code)
}

func TestRefresh_TokenVersionMismatch(t *testing.T) {
	m, err := auth.New(auth.Config{
		Secret: "test-secret-key-32-bytes-long!!!",
		Authenticate: func(ctx context.Context, username, password string) (*auth.AuthResult, error) {
			return &auth.AuthResult{
				UID:          "1",
				Scopes:       []string{"read"},
				TokenVersion: 1,
			}, nil
		},
		GetTokenVersion: func(ctx context.Context, uid string) (int64, error) {
			return 2, nil
		},
	})
	require.NoError(t, err)
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	require.Equal(t, http.StatusOK, w.Code)
	data := dataMap(t, decodeBody(t, w))
	refreshToken := data["refresh_token"].(string)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestLogout(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	data := dataMap(t, decodeBody(t, w))
	token := data["access_token"].(string)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestRegister_Disabled(t *testing.T) {
	// No UserCreator → register route not mounted
	m := newTestManager(t)
	r := setupRouter(m)
	w := post(t, r, "/auth/register", auth.RegisterRequest{Username: "bob", Password: "TestPass1"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── PasswordPolicy ─────────────────────────────────────────────────────────────

func TestPasswordPolicy_Default(t *testing.T) {
	p := auth.DefaultPasswordPolicy()
	assert.Empty(t, p.Validate("TestPass1"))
	assert.NotEmpty(t, p.Validate("short"))
	assert.NotEmpty(t, p.Validate("alllowercase1"))
	assert.NotEmpty(t, p.Validate("ALLUPPERCASE1"))
	assert.NotEmpty(t, p.Validate("NoDigitsHere"))
}

func TestPasswordPolicy_Disallowed(t *testing.T) {
	p := auth.DefaultPasswordPolicy()
	p.DisallowedPasswords = []string{"Password1"}
	errs := p.Validate("password1") // case-insensitive
	assert.NotEmpty(t, errs)
	assert.Contains(t, strings.Join(errs, " "), "弱密码")
}

func TestPasswordPolicy_Special(t *testing.T) {
	p := auth.DefaultPasswordPolicy()
	p.RequireSpecial = true
	assert.NotEmpty(t, p.Validate("TestPass1")) // missing special
	assert.Empty(t, p.Validate("TestPass1!"))
}

// ── AuthRequired + RequireScopes ──────────────────────────────────────────────

func TestRequireScopes_Allowed(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)
	r.GET("/secret", m.AuthRequired(), m.RequireScopes("read"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	data := dataMap(t, decodeBody(t, w))
	token := data["access_token"].(string)

	w2 := get(t, r, "/secret", token)
	assert.Equal(t, http.StatusOK, w2.Code)
}

func TestRequireScopes_Forbidden(t *testing.T) {
	m := newTestManager(t)
	r := setupRouter(m)
	r.GET("/admin", m.AuthRequired(), m.RequireScopes("admin"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := post(t, r, "/auth/login", auth.LoginRequest{Username: "alice", Password: "TestPass1"})
	data := dataMap(t, decodeBody(t, w))
	token := data["access_token"].(string)

	w2 := get(t, r, "/admin", token)
	assert.Equal(t, http.StatusForbidden, w2.Code)
}

// ── BcryptHasher ──────────────────────────────────────────────────────────────

func TestBcryptHasher(t *testing.T) {
	h := auth.BcryptHasher{}
	hash, err := h.Hash("MySecret1")
	require.NoError(t, err)
	assert.True(t, h.Verify("MySecret1", hash))
	assert.False(t, h.Verify("wrong", hash))
}
