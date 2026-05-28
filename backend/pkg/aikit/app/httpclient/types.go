package httpclient

import "net/http"

// Request 携带客户端级元数据的 HTTP 请求封装.
type Request struct {
	*http.Request
}

// Response 供中间件使用的 HTTP 响应封装.
type Response struct {
	*http.Response
}
