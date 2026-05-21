package httpclient

import "net/http"

// Request wraps http.Request with client-level metadata.
type Request struct {
	*http.Request
}

// Response wraps http.Response for middleware use.
type Response struct {
	*http.Response
}
