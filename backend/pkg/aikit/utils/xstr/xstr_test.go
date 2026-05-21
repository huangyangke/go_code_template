package xstr_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/example/go-template/pkg/aikit/utils/xstr"
)

// ── MD5 ───────────────────────────────────────────────────────────────────────

func TestMD5_KnownValue(t *testing.T) {
	// echo -n "hello" | md5sum  →  5d41402abc4b2a76b9719d911017c592
	assert.Equal(t, "5d41402abc4b2a76b9719d911017c592", xstr.MD5("hello"))
}

func TestMD5_Empty(t *testing.T) {
	// echo -n "" | md5sum  →  d41d8cd98f00b204e9800998ecf8427e
	assert.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", xstr.MD5(""))
}

func TestMD5_LengthIs32(t *testing.T) {
	assert.Len(t, xstr.MD5("anything"), 32)
}

// ── CompareVersion ────────────────────────────────────────────────────────────

func TestCompareVersion_Equal(t *testing.T) {
	assert.Equal(t, 0, xstr.CompareVersion("1.2.3", "1.2.3"))
}

func TestCompareVersion_LessThan(t *testing.T) {
	assert.Less(t, xstr.CompareVersion("1.2.3", "1.2.4"), 0)
	assert.Less(t, xstr.CompareVersion("1.0.0", "2.0.0"), 0)
}

func TestCompareVersion_GreaterThan(t *testing.T) {
	assert.Greater(t, xstr.CompareVersion("1.2.4", "1.2.3"), 0)
	assert.Greater(t, xstr.CompareVersion("2.0.0", "1.9.9"), 0)
}

func TestCompareVersion_DifferentLengths(t *testing.T) {
	assert.Less(t, xstr.CompareVersion("1.0", "1.0.1"), 0)
	assert.Greater(t, xstr.CompareVersion("1.0.1", "1.0"), 0)
}

func TestCompareVersion_Same(t *testing.T) {
	assert.Equal(t, 0, xstr.CompareVersion("v1.0.0", "v1.0.0"))
}

// ── GetRealIP ─────────────────────────────────────────────────────────────────

func reqWithHeaders(headers map[string]string, remoteAddr string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func TestGetRealIP_XForwardedFor_PublicIP(t *testing.T) {
	req := reqWithHeaders(map[string]string{
		"X-Forwarded-For": "1.2.3.4, 10.0.0.1",
	}, "192.168.1.1:1234")
	assert.Equal(t, "1.2.3.4", xstr.GetRealIP(req))
}

func TestGetRealIP_XRealIP(t *testing.T) {
	req := reqWithHeaders(map[string]string{
		"X-Real-Ip": "5.6.7.8",
	}, "192.168.1.1:1234")
	assert.Equal(t, "5.6.7.8", xstr.GetRealIP(req))
}

func TestGetRealIP_FallsBackToRemoteAddr(t *testing.T) {
	req := reqWithHeaders(nil, "9.10.11.12:5678")
	assert.Equal(t, "9.10.11.12", xstr.GetRealIP(req))
}

func TestGetRealIP_SkipsPrivateInXFF(t *testing.T) {
	// first IP is private, second is public
	req := reqWithHeaders(map[string]string{
		"X-Forwarded-For": "10.0.0.1, 1.2.3.4",
	}, "192.168.1.1:1234")
	assert.Equal(t, "1.2.3.4", xstr.GetRealIP(req))
}
