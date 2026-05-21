package auth_test

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/example/go-template/pkg/aikit/app/auth"
)

func TestGetUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	assert.Equal(t, "", auth.GetUID(c))

	c.Set(auth.ContextKeyUID, "user-42")
	assert.Equal(t, "user-42", auth.GetUID(c))
}

func TestGetScopes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	assert.Nil(t, auth.GetScopes(c))

	c.Set(auth.ContextKeyScopes, []string{"read", "write"})
	assert.Equal(t, []string{"read", "write"}, auth.GetScopes(c))
}

func TestGetTokenData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	assert.Nil(t, auth.GetTokenData(c))

	c.Set(auth.ContextKeyTokenData, map[string]any{"role": "admin"})
	assert.Equal(t, map[string]any{"role": "admin"}, auth.GetTokenData(c))
}

func TestGetUID_WrongType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set(auth.ContextKeyUID, 42)
	assert.Equal(t, "", auth.GetUID(c))
}

func TestGetScopes_WrongType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set(auth.ContextKeyScopes, "not-a-slice")
	assert.Nil(t, auth.GetScopes(c))
}

func TestGetTokenData_WrongType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Set(auth.ContextKeyTokenData, "not-a-map")
	assert.Nil(t, auth.GetTokenData(c))
}
