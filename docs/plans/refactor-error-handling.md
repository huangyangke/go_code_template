# Feature: 重构错误码与错误信息返回机制

> 本计划在实现前，请先验证文件路径、接口签名、字段名是否与当前代码一致。

## Feature Description

将当前基于运行时 map 注册的错误码机制，重构为编译期静态绑定的 `AppError` 变量方案。消除 Service 层的 `(nil, nil)` 歧义语义，Handler 层统一使用 `response.JSONErr` 处理错误，移除反向依赖（`internal/errors` → `pkg/aikit/app/response`）。

## User Story

作为一个使用此模板的开发者，
我希望错误码、HTTP 状态码、错误消息绑定在同一个变量上，Service 层负责翻译基础设施错误，Handler 层无需 nil 判断和码映射，
从而减少新增业务错误时需要改动的文件数量，并消除运行时 map 查找开销。

## Problem Statement

1. `(nil, nil)` 语义歧义：Service 用 `(nil, nil)` 表示"未找到"，调用方靠注释区分。
2. Handler 层知识过重：每个 handler 方法都有 nil 判断 + 码映射的重复 if/else。
3. 运行时 map 查找：`RegisterCode`/`LookupMsg` 每次响应多一次 map 查找，消息可在编译期确定。
4. 反向依赖：`internal/errors` import `pkg/aikit/app/response` 以拿框架码常量，业务层依赖基础设施层。

## Solution Statement

- 定义 `AppError{httpStatus, code, msg}` 类型，字段不可导出，错误值为包级变量。
- `internal/errors` 零依赖，移除 `init()` 注册。
- `response` 包移除 `RegisterCode`/`LookupMsg`/`codeMessages`，`Fail` 直接用传入 msg。
- Service 层在所有 DAO 调用后做 `gorm.ErrRecordNotFound` → `AppError` 翻译。
- DAO `Delete` 增加 `RowsAffected` 检查以正确检测软删除未找到。
- Handler 层统一 `response.JSONErr(c, data, err)`，只对非业务错误记日志。
- 新增编译期接口断言 + 运行期框架码一致性测试。

## Feature Metadata

**Feature Type**: Refactor  
**Estimated Complexity**: Medium  
**Primary Systems Affected**: `internal/errors`, `internal/dao`, `internal/service`, `internal/api`, `pkg/aikit/app/response`  
**Dependencies**: 无新增外部依赖

## Assumptions

- `model.Article` 嵌入 `dbmysql.Model`（含 `DeletedAt gorm.DeletedAt`），使用 GORM 软删除，`RowsAffected` 检查在软删除场景下行为正确。
- `pkg/aikit/` 内部的 `auth`、`async_queue` 等模块调用 `response.Unauthorized(c)` 等 helper，不调用 `RegisterCode`/`LookupMsg`，不受本次改动影响。
- `response.Fail` 的 `WithConvertCode`、`WithStatusCode`、`getTaskID` 逻辑保持不变，只移除 map 查找。

## Open Questions

- 无。所有设计决策已在 disc doc 中确认。

## Non-goals

- `pkg/aikit/` 内部 helper 函数的调用方式不变。
- 多语言 i18n。
- 错误码文档自动生成。
- 对 `auth`、`async_queue`、`middleware` 等模块的错误处理重构。

---

## CONTEXT REFERENCES

### 必读文件（实现前必须读取）

- `backend/internal/errors/errors.go` — 当前实现：`const` + `init()` 注册模式，全部替换
- `backend/pkg/aikit/app/response/response.go` (L1–L50) — `codeMessages` map、`RegisterCode`、`LookupMsg` 定义，需移除；`Fail`(L101–L117) 的 `LookupMsg` 调用需改为直接用 msg；`bizError` interface(L121–L125) 保持不变
- `backend/internal/dao/article.go` (L54–L56) — `Delete` 当前实现，需增加 `RowsAffected` 检查
- `backend/internal/service/article.go` — 全文，所有方法需增加错误翻译；`Delete` 签名从 `(bool, error)` 改为 `error`
- `backend/internal/api/article.go` — 全文，Get/Update/Delete handler 的 nil 判断分支需移除
- `backend/cmd/server/main.go` (L15) — blank import `_ "github.com/example/go-template/internal/errors"` 需移除
- `backend/pkg/aikit/app/response/response_test.go` (L120–L131) — `TestJSONErr_RecordNotFound` 断言 msg 为 `"record not found"`（英文），需更新为中文
- `backend/internal/service/article_test.go` — 现有测试：`mockRepo.Delete` 返回 `error`（已匹配新签名），需补充 not-found 场景

### What Already Exists

- `backend/pkg/aikit/app/response/response.go` — `bizError` interface duck typing 机制保留，`JSONErr` 的三个分支逻辑保留（只改 msg 来源和 gorm 兜底消息）
- `backend/internal/service/article_test.go` — mock repo 模式复用，补充新测试用例
- `backend/pkg/aikit/app/response/convenience_test.go` — helper 函数测试模式，补充框架码一致性测试时复用 `testContext()`

### New Files to Create

- `backend/internal/errors/errors_test.go` — 编译期接口断言 + 框架码一致性测试

### Patterns to Follow

**错误变量声明模式**（`internal/errors/errors.go`）：
```go
var ErrArticleNotFound = &AppError{http.StatusNotFound, 10100, "文章不存在"}
```

**Service 错误翻译模式**：
```go
article, err := s.repo.GetByID(ctx, id)
if errors.Is(err, gorm.ErrRecordNotFound) {
    return nil, apperrors.ErrArticleNotFound
}
return article, err
```

**Handler 统一响应模式**：
```go
result, err := h.svc.Get(c.Request.Context(), id)
if err != nil && !errors.As(err, new(*apperrors.AppError)) {
    log.Errorf("get article %d: %v", id, err)
}
response.JSONErr(c, result, err)
```

**测试中 mock Delete 的调用方式**（`article_test.go` L32–L34）：
```go
func (m *mockRepo) Delete(ctx context.Context, id uint) error {
    return m.Called(ctx, id).Error(0)
}
```
注意：`mockRepo.Delete` 已经返回 `error`，与新 Service 签名匹配，无需修改 mock。

---

## FLOW DIAGRAM

```text
HTTP 请求
   |
   v
Handler
   |-- 解析参数失败 → response.BadRequest(c)  [立即返回]
   |
   v
Service.Get/Update/Delete(ctx, id)
   |-- DAO 返回 gorm.ErrRecordNotFound → 翻译为 apperrors.ErrArticleNotFound
   |-- DAO 返回其他 error → 透传
   |-- 成功 → (data, nil)
   |
   v
Handler 收到 (data, err)
   |-- err != nil && !bizError → log.Errorf(...)
   v
response.JSONErr(c, data, err)
   |-- err == nil         → 200 + data
   |-- err 是 bizError    → biz.BizHTTPStatus() + biz.BizCode() + biz.Error()
   |-- gorm.ErrRecordNotFound (兜底) → 404 + 10003 + "记录不存在"
   |-- 其他 error          → 500 + 10005 + "服务器内部错误"
```

## SYSTEM BOUNDARIES

| Boundary | Input Type | Required Validation |
|---|---|---|
| Handler 路径参数 `id` | string → uint64 | `strconv.ParseUint` 失败 → `BadRequest`（现有，不变） |
| Handler 请求体 | JSON | `ShouldBindJSON` 失败 → `ParamError`（现有，不变） |

---

## IMPLEMENTATION PLAN

### Phase 1: 基础类型重构

重写 `internal/errors/errors.go`，移除 `cmd/server/main.go` 的 blank import。

### Phase 2: response 包简化

移除 map 机制，更新 `Fail`，修正 gorm 兜底消息，更新相关测试。

### Phase 3: DAO 层修正

`ArticleDAO.Delete` 增加 `RowsAffected` 检查。

### Phase 4: Service 层错误翻译

所有方法增加 `gorm.ErrRecordNotFound` 翻译，`Delete` 签名变更。

### Phase 5: Handler 层简化

移除 nil 判断，统一 `JSONErr`，补充日志。

### Phase 6: 测试补全

新增编译期接口断言、框架码一致性测试、Service not-found 场景测试。

---

## STEP-BY-STEP TASKS

### TASK 1: UPDATE `backend/internal/errors/errors.go`

- **REMOVE**: 所有 `const` 声明（框架码和业务码）
- **REMOVE**: `import "github.com/example/go-template/pkg/aikit/app/response"`
- **REMOVE**: `func init()` 及其全部 `RegisterCode` 调用
- **ADD**: `AppError` struct，字段全部小写不可导出：`httpStatus int`, `code int`, `msg string`
- **ADD**: 三个方法：`Error() string`、`BizCode() int`、`BizHTTPStatus() int`
- **ADD**: `import "net/http"`
- **ADD**: 框架层错误变量（`ErrBadRequest` 到 `ErrConflict`，10000–10009，数值与 `response.CodeXxx` 一致）
- **ADD**: Article 业务错误变量（`ErrArticleNotFound = &AppError{http.StatusNotFound, 10100, "文章不存在"}`，`ErrArticleDeleted = &AppError{http.StatusGone, 10101, "文章已删除"}`）
- **ADD**: 注释说明框架码数值必须与 `response.CodeXxx` 保持一致
- **GOTCHA**: `ErrUserNotFound`（10006）是框架级变量，User 业务域从 10200 开始，不冲突
- **VALIDATE**: `cd backend && go build ./internal/errors/...`

### TASK 2: UPDATE `backend/cmd/server/main.go`

- **REMOVE**: `_ "github.com/example/go-template/internal/errors" // register error codes`（L15）
- **VALIDATE**: `cd backend && go build ./cmd/server/...`

### TASK 3: UPDATE `backend/pkg/aikit/app/response/response.go`

- **REMOVE**: `var codeMessages = map[int]string{}`（L27）
- **REMOVE**: `func RegisterCode(code int, msg string)` 整个函数（L29–L33）
- **REMOVE**: `func LookupMsg(code int, fallback string) string` 整个函数（L35–L40）
- **UPDATE**: `Fail` 函数体中 `LookupMsg(code, msg)` → 直接用 `msg`（L113）
- **UPDATE**: `JSONErr` 中 `gorm.ErrRecordNotFound` 分支的消息从 `"record not found"` 改为 `"记录不存在"`
- **GOTCHA**: `WithConvertCode`、`WithStatusCode`、`getTaskID`、`Data: []any{}` 等逻辑**不动**
- **VALIDATE**: `cd backend && go build ./pkg/aikit/app/response/...`

### TASK 4: UPDATE `backend/pkg/aikit/app/response/response_test.go`

- **UPDATE**: `TestJSONErr_RecordNotFound`（L120–L131）中断言 msg 从 `"record not found"` 改为 `"记录不存在"`
- **VALIDATE**: `cd backend && go test ./pkg/aikit/app/response/...`

### TASK 5: UPDATE `backend/internal/dao/article.go`

- **UPDATE**: `Delete` 方法（L54–L56）改为：
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
- **ADD**: `import "gorm.io/gorm"` 如不存在（检查现有 import）
- **GOTCHA**: `model.Article` 使用软删除（`DeletedAt gorm.DeletedAt`），软删除时 `RowsAffected == 1`；已软删除或不存在时 `RowsAffected == 0`
- **VALIDATE**: `cd backend && go build ./internal/dao/...`

### TASK 6: UPDATE `backend/internal/service/article.go`

- **ADD**: import `apperrors "github.com/example/go-template/internal/errors"` 和 `"gorm.io/gorm"`（如不存在）
- **UPDATE**: `Get` 方法：在 `s.repo.GetByID` 调用后增加：
  ```go
  if errors.Is(err, gorm.ErrRecordNotFound) {
      return nil, apperrors.ErrArticleNotFound
  }
  ```
- **UPDATE**: `Update` 方法：在 `s.repo.Update` 调用后增加同样翻译
- **UPDATE**: `Delete` 方法签名从 `(bool, error)` 改为 `error`，内部翻译 `gorm.ErrRecordNotFound` → `apperrors.ErrArticleNotFound`，直接 `return err`（不再返回 bool）
- **UPDATE**: `articleRepository` interface 中 `Delete` 签名已是 `error`（确认 L14–L16，当前已是 `Delete(ctx context.Context, id uint) error`，无需改动 interface）
- **VALIDATE**: `cd backend && go build ./internal/service/...`

### TASK 7: UPDATE `backend/internal/api/article.go`

- **UPDATE**: import：将 `apperrors "github.com/example/go-template/internal/errors"` 保留（用于日志过滤），移除 `"net/http"`（如不再使用）
- **ADD**: import `"github.com/example/go-template/pkg/aikit/log"`（如不存在）
- **UPDATE**: `Get` handler：移除 `if article == nil { response.Fail(...) }` 分支，改为：
  ```go
  article, err := h.svc.Get(c.Request.Context(), uint(id))
  if err != nil && !errors.As(err, new(*apperrors.AppError)) {
      log.Errorf("get article %d: %v", id, err)
  }
  response.JSONErr(c, article, err)
  ```
- **UPDATE**: `Update` handler：同样移除 nil 判断，改用 `response.JSONErr`
- **UPDATE**: `Delete` handler：原来调用 `h.svc.Delete` 返回 `(bool, error)`，改为只返回 `error`；移除 `if !ok` 分支，改用 `response.JSONErr`
- **GOTCHA**: `errors.As(err, new(*apperrors.AppError))` 需要 import `"errors"`（标准库）
- **VALIDATE**: `cd backend && go build ./internal/api/...`

### TASK 8: CREATE `backend/internal/errors/errors_test.go`

- **ADD**: 编译期接口断言（包级 `var _`）：
  ```go
  var _ interface {
      Error() string
      BizCode() int
      BizHTTPStatus() int
  } = (*AppError)(nil)
  ```
- **ADD**: `TestFrameworkCodeConsistency`，验证框架层 `AppError` 变量的 `BizCode()` 与 `response.CodeXxx` 数值一致：
  ```go
  import (
      "testing"
      "github.com/stretchr/testify/assert"
      apperrors "github.com/example/go-template/internal/errors"
      "github.com/example/go-template/pkg/aikit/app/response"
  )

  func TestFrameworkCodeConsistency(t *testing.T) {
      assert.Equal(t, response.CodeBadRequest,    apperrors.ErrBadRequest.BizCode())
      assert.Equal(t, response.CodeParamError,    apperrors.ErrParamError.BizCode())
      assert.Equal(t, response.CodeMethodDenied,  apperrors.ErrMethodDenied.BizCode())
      assert.Equal(t, response.CodeNotFound,      apperrors.ErrNotFound.BizCode())
      assert.Equal(t, response.CodeRateLimited,   apperrors.ErrRateLimited.BizCode())
      assert.Equal(t, response.CodeInternalError, apperrors.ErrInternal.BizCode())
      assert.Equal(t, response.CodeUserNotFound,  apperrors.ErrUserNotFound.BizCode())
      assert.Equal(t, response.CodeUnauthorized,  apperrors.ErrUnauthorized.BizCode())
      assert.Equal(t, response.CodeForbidden,     apperrors.ErrForbidden.BizCode())
      assert.Equal(t, response.CodeConflict,      apperrors.ErrConflict.BizCode())
  }
  ```
- **GOTCHA**: `errors_test.go` 的包名用 `package errors_test`（外部测试包），需要 import `apperrors`
- **VALIDATE**: `cd backend && go test ./internal/errors/...`

### TASK 9: UPDATE `backend/internal/service/article_test.go`

- **ADD**: `TestArticleService_Get_NotFound`：mock `GetByID` 返回 `((*model.Article)(nil), gorm.ErrRecordNotFound)`，断言 service 返回 `(nil, apperrors.ErrArticleNotFound)`
- **ADD**: `TestArticleService_Delete_NotFound`：mock `Delete` 返回 `gorm.ErrRecordNotFound`，断言 service 返回 `apperrors.ErrArticleNotFound`
- **ADD**: 需要 import `"gorm.io/gorm"` 和 `apperrors "github.com/example/go-template/internal/errors"`
- **GOTCHA**: `mockRepo.Delete` 已返回 `error`（L32–L34），与新 Service 接口匹配，无需修改 mock
- **VALIDATE**: `cd backend && go test ./internal/service/...`

---

## TESTING STRATEGY

### Unit Tests

- `internal/errors/errors_test.go`：编译期接口断言 + 框架码一致性
- `internal/service/article_test.go`：补充 Get not-found、Delete not-found 场景
- `pkg/aikit/app/response/response_test.go`：更新 `TestJSONErr_RecordNotFound` 消息断言

### Edge Cases

- `Delete` 软删除：记录已被软删除时 `RowsAffected == 0`，返回 `ErrArticleNotFound`
- `JSONErr` 收到 `nil` data + `nil` err：正常返回 200，`data` 字段为 `null`
- `JSONErr` 收到 `bizError`：直接用 `biz.Error()` 作为消息，不走 map

## FAILURE MODES

| Failure Mode | Trigger | Expected Handling | Test Required |
|---|---|---|---|
| DAO 返回非 gorm 错误（如 DB 连接失败） | MySQL 不可达 | Service 透传，Handler 记日志，JSONErr 返回 500 | 否（集成测试范畴） |
| Article 记录已软删除，再次 Delete | RowsAffected == 0 | DAO 返回 gorm.ErrRecordNotFound，Service 翻译为 ErrArticleNotFound，404 | 是（unit test mock） |
| 框架码数值漂移（两处定义不一致） | 手动修改一处忘记同步 | `TestFrameworkCodeConsistency` 失败，CI 拦截 | 是 |

---

## VALIDATION COMMANDS

### Level 1: 编译检查

```bash
cd /data/13_claude/go_code_template/backend && go build ./...
```

### Level 2: 单元测试

```bash
cd /data/13_claude/go_code_template/backend && go test ./internal/errors/... ./internal/service/... ./pkg/aikit/app/response/...
```

### Level 3: 全量测试

```bash
cd /data/13_claude/go_code_template/backend && go test ./...
```

### Level 4: 代码检查

```bash
cd /data/13_claude/go_code_template && ./run.sh lint
```

---

## ACCEPTANCE CRITERIA

- [ ] `internal/errors` 包无任何 import（零依赖）
- [ ] `internal/errors/errors.go` 无 `init()` 函数，无 `const` 声明
- [ ] `response.go` 无 `RegisterCode`、`LookupMsg`、`codeMessages`
- [ ] `response.Fail` 直接使用传入 `msg`，不做 map 查找
- [ ] `ArticleDAO.Delete` 检查 `RowsAffected == 0` 并返回 `gorm.ErrRecordNotFound`
- [ ] `ArticleService.Delete` 签名为 `(ctx, id) error`
- [ ] `ArticleService.Get`/`Update`/`Delete` 均翻译 `gorm.ErrRecordNotFound`
- [ ] `ArticleHandler.Get`/`Update`/`Delete` 无 `if article == nil` 分支，统一 `response.JSONErr`
- [ ] `cmd/server/main.go` 无 blank import `internal/errors`
- [ ] `TestJSONErr_RecordNotFound` 断言消息为中文"记录不存在"
- [ ] `TestFrameworkCodeConsistency` 全部通过
- [ ] `go build ./...` 零错误
- [ ] `go test ./...` 全部通过

---

## NOTES

- **`pkg/aikit/` 的 helper 调用不变**：`auth`、`async_queue` 等调用 `response.Unauthorized(c)` 等，这些 helper 内部 hardcode 了消息字符串，与新的 `AppError` 消息保持一致，不需要改动。
- **`response.CodeXxx` 常量保留**：`pkg/aikit/` 内部代码和直接调用 `Fail` 的场景仍需要这些常量，不删除。
- **disc doc 路径**：`docs/disc/2026-05-22-error-code-design-disc.md`
