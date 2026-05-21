package upload

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func signOSSV4(req *http.Request, ak, sk, bucketName, objectKey string) error {
	t := time.Now().UTC()
	date := t.Format("20060102T150405Z")
	shortDate := date[:8]
	region := extractRegionFromEndpoint(req.URL.Host)

	// Canonical request
	canonicalURI := "/" + objectKey
	canonicalQuery := ""
	canonicalHeaders := "content-md5:" + req.Header.Get("Content-MD5") + "\n" +
		"host:" + req.URL.Host + "\n" +
		"x-oss-security-token:" + req.Header.Get("x-oss-security-token") + "\n"
	signedHeaders := "content-md5;host;x-oss-security-token"

	payloadHash := "UNSIGNED-PAYLOAD"

	canonicalRequest := strings.Join([]string{
		req.Method, canonicalURI, canonicalQuery, canonicalHeaders, signedHeaders, payloadHash,
	}, "\n")

	// String to sign
	algorithm := "OSS4-HMAC-SHA1"
	scope := fmt.Sprintf("%s/%s/%s/oss4_request", shortDate, region, bucketName)
	stringToSign := strings.Join([]string{
		algorithm, date, scope, fmt.Sprintf("%x", sha1.Sum([]byte(canonicalRequest))),
	}, "\n")

	// Signing key
	dateKey := hmacSHA1([]byte("OSS4" + sk), []byte(shortDate))
	regionKey := hmacSHA1(dateKey, []byte(region))
	bucketKey := hmacSHA1(regionKey, []byte(bucketName))
	signingKey := hmacSHA1(bucketKey, []byte("oss4_request"))

	signature := base64.StdEncoding.EncodeToString(hmacSHA1(signingKey, []byte(stringToSign)))

	// Authorization header
	auth := fmt.Sprintf("%s Credential=%s/%s/%s/%s/oss4_request, SignedHeaders=%s, Signature=%s",
		algorithm, ak, shortDate, region, bucketName, signedHeaders, signature)
	req.Header.Set("Authorization", auth)
	req.Header.Set("x-oss-date", date)
	req.Header.Set("x-oss-content-sha256", payloadHash)

	return nil
}

func hmacSHA1(key, data []byte) []byte {
	mac := hmac.New(sha1.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func extractRegionFromEndpoint(endpoint string) string {
	if idx := strings.Index(endpoint, "-internal."); idx != -1 {
		start := strings.LastIndex(endpoint[:idx], ".")
		return endpoint[start+1 : idx]
	}
	if strings.Contains(endpoint, "oss-cn-") {
		parts := strings.SplitN(endpoint, ".", 3)
		if len(parts) > 1 && len(parts[0]) > len("oss-cn-") {
			return parts[0][len("oss-cn-"):]
		}
	}
	return "oss-cn-beijing"
}
