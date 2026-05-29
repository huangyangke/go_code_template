package async_queue

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huangyangke/go-aikit/internal/testutil"
	"github.com/huangyangke/go-aikit/log"
)

func TestProducerHandleEnqueueLogsBindError(t *testing.T) {
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
	router := testutil.NewGinRouter(t)
	router.POST("/task", p.handleEnqueue("/task"))

	req := httptest.NewRequest(http.MethodPost, "/task", strings.NewReader("{bad-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := testutil.ServeRequest(router, req)

	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	testutil.AssertStatus(t, rec, http.StatusUnprocessableEntity)
	assert.Contains(t, buf.String(), "[Producer][/task][request_bind_error]")
}
