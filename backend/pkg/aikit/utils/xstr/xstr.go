// Package xstr 字符串工具函数，提供 MD5、版本比较、真实 IP 提取等.
package xstr

import (
	"crypto/md5"
	"fmt"
	"net"
	"net/http"
	"strings"
)

// MD5 返回字符串的十六进制 MD5 摘要.
// 参数：s - 待计算字符串.
// 返回值：str - 十六进制 MD5 摘要.
func MD5(s string) string {
	h := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", h)
}

// CompareVersion 比较两个以点/横线/下划线分隔的版本字符串.
// 参数：v1 - 版本字符串1, v2 - 版本字符串2.
// 返回值：int - 相等返回0, v1<v2返回负数, v1>v2返回正数.
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

// 解析真实客户端 IP 时需跳过的私有 CIDR 地址块.
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

// GetRealIP 从请求中提取真实客户端 IP，依次检查 X-Forwarded-For、X-Real-Ip、RemoteAddr.
// 参数：r - HTTP 请求, resolvePrivate - 为 true 时保留 XFF 中的私有/回环地址.
// 返回值：str - 客户端 IP 字符串.
func GetRealIP(r *http.Request, resolvePrivate ...bool) string {
	skipPrivate := len(resolvePrivate) == 0 || !resolvePrivate[0]

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

	if xri := strings.TrimSpace(r.Header.Get("X-Real-Ip")); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
