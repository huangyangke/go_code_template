package xstr

import (
	"crypto/md5"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// MD5 returns the hex-encoded MD5 digest of s.
func MD5(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

// CompareVersion compares two dot/dash/underscore-delimited version strings.
// Returns 0 if equal, <0 if v1 < v2, >0 if v1 > v2.
func CompareVersion(v1, v2 string) int {
	if v1 == v2 {
		return 0
	}
	s1 := splitVersion(v1)
	s2 := splitVersion(v2)
	n := len(s1)
	if len(s2) < n {
		n = len(s2)
	}
	for i := 0; i < n; i++ {
		if d := len(s1[i]) - len(s2[i]); d != 0 {
			return d
		}
		if d := strings.Compare(s1[i], s2[i]); d != 0 {
			return d
		}
	}
	return len(s1) - len(s2)
}

func splitVersion(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == '.' || r == '_' || r == '-'
	})
}

// private CIDR blocks to skip when resolving the real client IP.
var privateNets []*net.IPNet

func init() {
	for _, cidr := range []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"127.0.0.0/8",
		"100.64.0.0/10",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"224.0.0.0/4",
		"255.255.255.255/32",
		"::1/128",
		"fe80::/10",
		"fc00::/7",
	} {
		_, n, _ := net.ParseCIDR(cidr)
		if n != nil {
			privateNets = append(privateNets, n)
		}
	}
}

func isPrivate(ip net.IP) bool {
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// GetRealIP extracts the real client IP from the request, checking
// X-Forwarded-For, X-Real-Ip, and finally RemoteAddr.
// When resolvePrivate is true, private/loopback addresses in X-Forwarded-For
// are not skipped. Default behaviour skips private IPs.
func GetRealIP(r *http.Request, resolvePrivate ...bool) string {
	skipPrivate := len(resolvePrivate) == 0 || !resolvePrivate[0]

	// X-Forwarded-For may contain a chain: "client, proxy1, proxy2"
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			ip := strings.TrimSpace(part)
			parsed := net.ParseIP(ip)
			if parsed == nil {
				continue
			}
			if skipPrivate && isPrivate(parsed) {
				continue
			}
			return ip
		}
	}

	// X-Real-Ip
	if xri := strings.TrimSpace(r.Header.Get("X-Real-Ip")); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	// RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
