# response — 统一 API 响应

统一的 JSON 响应结构，含标准错误码和 task_id 透传。

## 响应格式

```json
{"code": 200, "msg": "ok", "data": {...}, "task_id": "uuid"}
```

## 用法

```go
// 成功
response.JSON(c, data, middleware.GetTaskID(c.Request.Context()))

// 错误（自动映射）
response.JSONErr(c, nil, err)

// 指定错误
response.BadRequest(c, "参数错误")
response.Unauthorized(c, "未授权")
response.NotFound(c, "资源不存在")
response.InternalError(c, "服务异常")
response.RateLimited(c)
```

## 错误码

| 常量 | 值 | 说明 |
|---|---|---|
| `CodeSuccess` | 200 | 成功 |
| `CodeBadRequest` | 10000 | 请求错误 |
| `CodeParamError` | 10001 | 参数错误 |
| `CodeMethodDenied` | 10002 | 方法不允许 |
| `CodeNotFound` | 10003 | 未找到 |
| `CodeRateLimited` | 10004 | 限流 |
| `CodeInternalError` | 10005 | 内部错误 |
| `CodeUserNotFound` | 10006 | 用户不存在 |
| `CodeUnauthorized` | 10007 | 未授权 |
| `CodeForbidden` | 10008 | 禁止 |
| `CodeConflict` | 10009 | 冲突 |
