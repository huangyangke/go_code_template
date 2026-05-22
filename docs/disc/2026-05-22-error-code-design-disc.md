# 错误码与错误信息返回设计

**日期**: 2026-05-22  
**状态**: 待实现  
**范围**: `backend/internal/errors/`, `backend/pkg/aikit/app/response/`, `backend/internal/api/`, `backend/internal/service/`

---

## 背景与问题

当前模板的错误处理存在以下问题：

1. **`(nil, nil)` 歧义语义**：Service 层用 `(nil, nil)` 表示"未找到"，与"成功返回空"在类型系统上无法区分，依赖约定和注释。
2. **Handler 层职责过重**：Handler 既要处理 nil 判断，又要维护业务码映射，还要记住每个 service 方法的返回约定，导致每个 handler 方法都有重复的 if/else 分支。
3. **运行时 map 查找**：`response.RegisterCode` + `LookupMsg` 在每次响应时做 map 查找，消息本可在编译期确定。
4. **反向依赖**：`internal/errors` import `pkg/aikit/app/response` 以获取框架码常量，业务层依赖基础设施层，方向错误。

---

## 决策

### 核心原则

- **错误消息静态化**：消息在编译期定好，不需要运行时格式化。
- **Service 层做错误翻译**：将基础设施错误（`gorm.ErrRecordNotFound`）翻译为业务错误，Handler 不感知 gorm。
- **`internal/errors` 零依赖**：不 import 任何项目内部包，成为纯业务错误定义层。
- **Handler 统一一行**：所有错误路径通过 `response.JSONErr` 处理，消除重复判断。

### 依赖方向

```
internal/errors          (零依赖)
        ↑
internal/service         (import errors，做错误翻译)
        ↑
internal/api             (import response，调用 JSONErr)

pkg/aikit/app/response   (通过 bizError interface duck typing 消费 AppError)
```

---

## 设计

### 1. `internal/errors/errors.go`

定义 `AppError` 类型和所有业务错误变量。消息直接编码在值上，无 map，无 init 注册。字段设为不可导出，防止外部修改全局变量。

```go
package errors

import "net/http"

type AppError struct {
    httpStatus int
    code       int
    msg        string
}

func (e *AppError) Error() string      { return e.msg }
func (e *AppError) BizCode() int       { return e.code }
func (e *AppError) BizHTTPStatus() int { return e.httpStatus }

// 框架层码变量（数值与 response 包常量一致；通过编译期断言 var _ bizShape = (*AppError)(nil) 保证接口匹配）
// 注意：框架层码的数值必须与 pkg/aikit/app/response 中的 CodeXxx 常量保持一致。
// 若修改任一方，需同步修改另一方，并在 response_test.go 中有数值对比测试。
var (
    ErrBadRequest   = &AppError{http.StatusBadRequest,          10000, "请求错误"}
    ErrParamError   = &AppError{http.StatusUnprocessableEntity, 10001, "参数校验错误"}
    ErrMethodDenied = &AppError{http.StatusMethodNotAllowed,    10002, "请求方法错误"}
    ErrNotFound     = &AppError{http.StatusNotFound,            10003, "请求路径错误"}
    ErrRateLimited  = &AppError{http.StatusTooManyRequests,     10004, "请求过于频繁，请稍后重试"}
    ErrInternal     = &AppError{http.StatusInternalServerError, 10005, "服务器内部错误"}
    ErrUserNotFound = &AppError{http.StatusNotFound,            10006, "用户不存在"}
    ErrUnauthorized = &AppError{http.StatusUnauthorized,        10007, "未登录或登录已失效"}
    ErrForbidden    = &AppError{http.StatusForbidden,           10008, "无权限访问"}
    ErrConflict     = &AppError{http.StatusConflict,            10009, "资源已存在"}
)

// Article errors (10100–10199)
var (
    ErrArticleNotFound = &AppError{http.StatusNotFound, 10100, "文章不存在"}
    ErrArticleDeleted  = &AppError{http.StatusGone,     10101, "文章已删除"}
)
```

**关于框架码的两份定义**：`response` 包保留 `CodeXxx` int 常量供直接调用 `Fail` 的场景（aikit 内部 helper），`internal/errors` 定义对应的 `ErrXxx` 变量供业务层使用。两者数值必须一致，通过测试断言保证（见下文测试设计）。

**命名规范**：`Err{Domain}{Reason}`，框架级用 `Err{Reason}`。  
**码段规范**：

| 域 | 范围 | 备注 |
|---|---|---|
| 框架保留 | 10000–10099 | 与 response.CodeXxx 对应 |
| Article | 10100–10199 | |
| User | 10200–10299 | 注意：10006 是框架级"用户不存在"，10200+ 是业务级用户错误，不重叠 |
| Order | 10300–10399 | |

### 2. `pkg/aikit/app/response/response.go` 变更

移除 `RegisterCode`、`LookupMsg`、`codeMessages` map。`Fail` 直接使用传入的 `msg` 参数（其他逻辑如 `WithConvertCode`、`WithStatusCode`、`TaskID` 提取保持不变）。

`bizError` interface 定义保持不变：

```go
type bizError interface {
    error  // 内嵌 error，即 Error() string
    BizCode() int
    BizHTTPStatus() int
}
```

`JSONErr` 调整说明：

- `bizError` 分支：直接取 `biz.Error()` 作为消息，无 map 查找
- `gorm.ErrRecordNotFound` 兜底分支：**保留**作为 Service 层未翻译时的安全网，但这是异常路径（正常情况 Service 已翻译）。消息统一为中文"记录不存在"（修正原来的英文）
- 其他 error：返回 500。**由调用方（Service）负责在返回前用 `log.Error` 记录原始错误**；`JSONErr` 本身不做日志（response 包不依赖 log 包）
- `err == nil` 分支：返回成功，`data` 可为 nil（前端收到 `"data": null`）

`response` 包的 helper 函数（`BadRequest`、`Unauthorized` 等）保持现有实现，它们直接 hardcode 消息字符串，与 `internal/errors` 中的消息保持一致。两处消息字符串通过测试断言对齐（见测试设计）。

### 3. Service 层：错误翻译

Service 负责将 DAO 的基础设施错误（`gorm.ErrRecordNotFound`）转换为业务错误。**所有返回 `ErrRecordNotFound` 的 DAO 调用都需要翻译**，包括 Get、Update、Delete。

```go
// Get
func (s *ArticleService) Get(ctx context.Context, id uint) (*model.Article, error) {
    article, err := s.repo.GetByID(ctx, id)
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, apperrors.ErrArticleNotFound
    }
    return article, err
}

// Update
func (s *ArticleService) Update(ctx context.Context, id uint, req *schema.UpdateArticleReq) (*model.Article, error) {
    // ... 构建 updates map ...
    article, err := s.repo.Update(ctx, id, updates)
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, apperrors.ErrArticleNotFound
    }
    return article, err
}

// Delete：签名从 (bool, error) 改为 error，未找到时返回 ErrArticleNotFound
// DAO 层需通过 RowsAffected == 0 判断未找到并返回 gorm.ErrRecordNotFound
func (s *ArticleService) Delete(ctx context.Context, id uint) error {
    err := s.repo.Delete(ctx, id)
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return apperrors.ErrArticleNotFound
    }
    return err
}
```

**DAO 的 Delete** 需要检查 `RowsAffected`。`model.Article` 嵌入了 `dbmysql.Model`，其中包含 `DeletedAt gorm.DeletedAt`，因此 GORM 执行软删除（设置 `deleted_at`）。软删除情况下：

- 记录存在时：`RowsAffected == 1`，成功
- 记录不存在（或已被软删除）时：`RowsAffected == 0`，返回 `gorm.ErrRecordNotFound`

```go
func (d *ArticleDAO) Delete(ctx context.Context, id uint) error {
    result := d.db.WithContext(ctx).Delete(&model.Article{}, id)
    if result.Error != nil {
        return result.Error
    }
    if result.RowsAffected == 0 {
        return gorm.ErrRecordNotFound
    }
    return nil
}
```

对于未翻译的 DAO 错误（DB 连接失败等），Service 直接透传，Handler 负责在调用 `response.JSONErr` 前记录日志。

### 4. Handler 层：统一一行

消除所有 `if article == nil` 判断，统一用 `JSONErr`。Get、Update、Delete handler 均适用：

```go
func (h *ArticleHandler) Get(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        response.BadRequest(c)
        return
    }
    article, err := h.svc.Get(c.Request.Context(), uint(id))
    if err != nil && !errors.As(err, new(*apperrors.AppError)) {
        log.Errorf("get article %d: %v", id, err)  // 仅记录非业务错误（DB 故障等）
    }
    response.JSONErr(c, article, err)
}
```

业务错误（`*AppError`，如 `ErrArticleNotFound`）是预期行为，不记录日志；非业务错误（DB 连接失败等）才记录。

### 5. 测试设计

在 `response_test.go` 或独立测试文件中增加断言，防止两处定义漂移：

```go
// 确保 AppError 实现了 bizError 接口（编译期检查）
// 放在 internal/errors/errors_test.go
var _ interface {
    Error() string
    BizCode() int
    BizHTTPStatus() int
} = (*AppError)(nil)

// 确保框架码数值一致（运行期断言，放在 response_test.go）
func TestFrameworkCodeConsistency(t *testing.T) {
    require.Equal(t, response.CodeBadRequest, apperrors.ErrBadRequest.BizCode())
    require.Equal(t, response.CodeUnauthorized, apperrors.ErrUnauthorized.BizCode())
    // ... 其他框架码 ...
}
```

---

## 性能对比

| 维度 | 现方案 | 新方案 |
|------|--------|--------|
| 消息查找 | 运行时 map 查找 | 直接字段访问（零开销） |
| 内存 | 全局 map 常驻 | 无额外内存 |
| 并发安全 | map 只在 init 写入（安全，但结构上有隐患） | 无共享可变状态 |
| 编译期保证 | 码与消息分离，可能漂移 | 码与消息绑定在同一变量 |

---

## 不在范围内

- `pkg/aikit/` 内部的 helper（`Unauthorized`、`RateLimited` 等）调用方式不变，它们直接调用 `response.Unauthorized(c)` 等，不受此次改动影响，wire output（code + msg）保持一致。
- 多语言 i18n 支持。
- 错误码文档自动生成。

---

## 迁移路径

1. 更新 `internal/errors/errors.go`：改为 `AppError` 变量，字段不可导出，移除 `init()`
2. 更新 `pkg/aikit/app/response/response.go`：移除 `RegisterCode`、`LookupMsg`、`codeMessages`；`gorm.ErrRecordNotFound` 兜底消息改为中文
3. 更新 `internal/dao/article.go`：`Delete` 增加 `RowsAffected` 检查
4. 更新 `internal/service/article.go`：所有方法做错误翻译，`Delete` 签名从 `(bool, error)` 改为 `error`
5. 更新 `internal/api/article.go`：移除所有 `if article == nil` 分支（Get、Update、Delete handler），改用 `response.JSONErr(c, article, err)`；日志只记录非业务错误
6. 新增测试：框架码一致性断言 + `AppError` 接口编译期检查
