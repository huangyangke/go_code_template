package async_queue

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/example/go-template/pkg/aikit/log"
)

func TestProducerHandleEnqueueLogsBindError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
		log.Init(&log.Config{Stdout: true, Level: "info"})
	})

	log.Init(&log.Config{Stdout: true, Level: "debug"})

	p := &Producer{namespace: "test"}
	router := gin.New()
	router.POST("/task", p.handleEnqueue("/task"))

	req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader("{bad-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, buf.String(), "[Producer][/task][request_bind_error]")
}
