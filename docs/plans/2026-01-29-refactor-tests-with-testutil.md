# 实现计划：替换测试代码重复为 testutil 封装

## 概览

**目标**：将项目中 62 个测试文件的 2000+ 行重复代码替换为 `internal/testutil` 包的封装调用，减少冗余并统一测试模式。

**当前状态**：
- ✅ `testutil` 包已完整实现并测试
- ✅ 包含 16 个工具函数（原有 8 个 + 新增 8 个）
- ❌ **0 个外部测试文件使用 testutil**（完全未被采用）
- 📊 重复代码分布：
  - Gin router 创建：**50+ 处**
  - JSON 请求构造：**30+ 处**
  - HTTP 响应记录：**50+ 处**
  - 状态码断言：**80+ 处**
  - JSON 响应解析：**40+ 处**
  - Context 创建：**100+ 处**
  - 数据库/Redis 创建：**5 处**

**预计收益**：
- 减少约 1500 行重复代码
- 统一测试模式，降低维护成本
- 提高测试一致性和可读性

---

## 问题陈述

项目中存在 8 种重复的测试代码模式，分布在 62 个测试文件中。这种重复导致：
1. **维护成本高**：修改测试模式需要改所有相关文件
2. **不一致风险**：不同文件可能有细微差异（如 `gin.SetMode` 的调用位置）
3. **开发效率低**：新测试需要复制粘贴样板代码
4. **可读性差**：测试逻辑被重复的基础设施代码淹没

---

## 目标陈述

将所有可安全替换的重复代码统一改为 `testutil` 封装调用，具体包括：

### Phase 1：Gin 相关（优先级：高）
- `gin.SetMode(gin.TestMode)` + `gin.New()` → `testutil.NewGinRouter(t)`
- `httptest.NewRequest` + `json.Marshal` + `Header.Set` → `testutil.NewJSONRequest(t, method, path, body)`
- `httptest.NewRecorder()` + `r.ServeHTTP(w, req)` → `testutil.ServeRequest(r, req)`
- `assert.Equal(t, 200, w.Code)` → `testutil.AssertStatus(t, w, 200)`
- `json.Unmarshal(w.Body.Bytes(), &resp)` → `testutil.ParseJSONResponse(t, w, &resp)`

### Phase 2：Context 相关（优先级：中）
- `ctx, cancel := context.WithCancel(context.Background())` + `defer cancel()` → `ctx, cancel := testutil.NewTestContext(t)`
- `context.WithTimeout(ctx, 5*time.Second)` → `testutil.NewContextWithTimeout(t, 5*time.Second)`

### Phase 3：Database/Redis 相关（优先级：低）
- `miniredis.RunT(t)` + `redis.NewClient` + `defer Close()` → `testutil.NewMiniRedis(t)`
- `redis.NewClient` + `dbredis.New` + `defer Close()` → `testutil.NewRedis(t)`
- `gorm.Open(sqlite.Open(":memory:"))` + `defer db.DB().Close()` → `testutil.NewSQLiteDB(t)`

### Phase 4：时间控制（优先级：低）
- `clock.NewMock()` + `mock.Set(now)` → `testutil.NewClock(t, now)`  *(仅在需要时间控制的测试中)*

---

## 文件结构

### ✅ 需要修改的文件（34 个）

这些文件包含可替换的重复模式：

#### Phase 2：高优先级 - Gin 相关（8 个文件，50+ 重复）
```
app/
├── auth/auth_test.go                    # 29 gin.New, 102 recorder, 35 jsonReq
├── auth/context_test.go                 # 6 gin.SetMode
├── middleware/
│   ├── middleware_test.go               # 16 gin.New, 32 recorder
│   ├── cors_test.go                     # 5 gin.New, 10 recorder
│   ├── rate_limit_test.go               # 4 gin.New, 10 recorder
│   ├── rate_limit_lua_test.go           # 3 gin.New, 8 recorder
│   └── request_id_test.go               # 5 gin.New, 10 recorder
├── async_queue/
│   ├── producer_test.go                 # 6 gin.New, 34 recorder
│   └── producer_log_test.go             # 1 gin.New, 2 recorder
├── fastapp_test.go                      # 3 gin.SetMode, 28 recorder
├── response/
│   ├── response_test.go                 # 2 gin.SetMode, 2 recorder
│   └── convenience_test.go             # 1 gin.SetMode, 1 recorder
```

#### Phase 3：中优先级 - Context 相关（16 个文件，100+ 重复）
```
agent/eino_plus/embedding/embedding_test.go  # 10 context.*
app/async_queue/
├── context_test.go                          # 4 context.*
├── concurrency_limiter_test.go              # 14 context.*
├── consumer_test.go                         # 21 context.*
├── deadletter_test.go                       # 5 context.*
├── status_test.go                           # 38 context.* (含 1 jsonReq)
database/redis/
├── lock_test.go                             # 9 context.*, 1 miniredis
└── cmd_test.go                              # 29 context.*, 1 miniredis
app/httpclient/
├── client_test.go                           # 4 context.*, 4 httptest.NewServer
├── middleware_test.go                       # 14 context.*, 5 httptest.NewServer
└── middleware_extra_test.go                 # 26 context.*, 15 httptest.NewServer
cache/cache_test.go                          # 12 context.*
log/handler_test.go                          # 4 context.*
utils/gopool/pool_internal_test.go          # 4 context.*
utils/gopool/pool_test.go                   # 3 context.*
```

#### Phase 4：低优先级 - Database/Redis（3 个文件，3 重复）
```
database/mysql/
├── breaker_test.go                          # 1 gorm.Open(sqlite)
├── txdb_test.go                             # 1 gorm.Open(sqlite)
└── timestamp_test.go                        # 1 gorm.Open(sqlite)
```

---

### ❌ 不需要修改的文件（28 个）

这些文件是纯单元测试，没有可替换的模式：

```
# Config 验证测试（仅 assert）
app/async_queue/config_test.go
app/httpclient/config_test.go
cache/cache_config_test.go
config/config_test.go
config/config_extra_test.go
database/mysql/mysql_config_test.go
database/redis/redis_config_test.go

# 数据结构测试（仅 assert）
app/async_queue/helpers_test.go
app/async_queue/pending_test.go

# 其他纯单元测试
agent/eino_plus/vectordb/compat_test.go
agent/eino_plus/vectordb/vectordb_test.go
app/fastapp_extra_test.go
app/xjob/logger_test.go
app/xjob/log_handler_test.go
app/xjob/xjob_test.go
cache/stats_handler_test.go
database/mysql/metrics_test.go
database/mysql/migrate_test.go
database/pulsar/pulsar_test.go
database/redis/redis_test.go
log/field_test.go
log/log_test.go
metrics/metrics_test.go
metrics/predefined_test.go
resilience/breaker_test.go
resilience/retry_test.go
utils/upload/upload_test.go
utils/upload/example_test.go
utils/xstr/xstr_test.go
version/version_test.go
app/health/health_test.go (仅有 2 ctx，可忽略)
```

**总计：62 个测试文件 = 34 需要替换 + 28 无需修改**

---

## 替换规则

### 规则 1：Gin Router 创建
```go
// Before
func init() {
    gin.SetMode(gin.TestMode)
}

func TestXxx(t *testing.T) {
    r := gin.New()
    r.Use(auth.Middleware)
    r.GET("/test", handler)
    // ...
}

// After
func TestXxx(t *testing.T) {
    r := testutil.NewGinRouter(t)
    r.Use(auth.Middleware)
    r.GET("/test", handler)
    // ...
}
```

**注意**：
- 移除 `init()` 中的 `gin.SetMode(gin.TestMode)`（如果存在）
- `NewGinRouter` 已经包含 `Recovery()` 中间件，如果原代码有 `r.Use(gin.Recovery())` 需要删除

---

### 规则 2：JSON 请求构造
```go
// Before
func TestPost(t *testing.T) {
    body := map[string]string{"name": "test"}
    bodyBytes, _ := json.Marshal(body)
    req, _ := http.NewRequest("POST", "/api/test", bytes.NewReader(bodyBytes))
    req.Header.Set("Content-Type", "application/json")
    // ...
}

// After
func TestPost(t *testing.T) {
    req := testutil.NewJSONRequest(t, "POST", "/api/test", map[string]string{"name": "test"})
    // ...
}
```

**参数映射**：
- 第 1 个参数：`t`
- 第 2 个参数：HTTP 方法（字符串）
- 第 3 个参数：请求路径（字符串）
- 第 4 个参数：请求体（可以是 struct、map，或 `nil`）

---

### 规则 3：HTTP 响应记录
```go
// Before
w := httptest.NewRecorder()
r.ServeHTTP(w, req)
assert.Equal(t, 200, w.Code)

// After
w := testutil.ServeRequest(r, req)
testutil.AssertStatus(t, w, 200)
```

---

### 规则 4：JSON 响应解析
```go
// Before
var resp Response
err := json.Unmarshal(w.Body.Bytes(), &resp)
require.NoError(t, err)

// After
var resp Response
testutil.ParseJSONResponse(t, w, &resp)
```

**注意**：`ParseJSONResponse` 已经包含错误检查，无需额外的 `require.NoError`

---

### 规则 5：Context 创建
```go
// Before
func TestWithCancel(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    // ...
}

// After
func TestWithCancel(t *testing.T) {
    ctx, cancel := testutil.NewTestContext(t)
    // 无需 defer，t.Cleanup 已自动注册
    // ...
}
```

```go
// Before
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// After
ctx, cancel := testutil.NewContextWithTimeout(t, 5*time.Second)
// 无需 defer
```

---

### 规则 6：MiniRedis 创建
```go
// Before
func TestRedis(t *testing.T) {
    mr := miniredis.RunT(t)
    client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    defer client.Close()
    // ...
}

// After
func TestRedis(t *testing.T) {
    mr, client := testutil.NewMiniRedis(t)
    // 无需 defer，t.Cleanup 已自动注册
    // ...
}
```

**注意**：如果原代码需要访问 `mr.FastForward()` 或 `mr.Exists()`，`NewMiniRedis` 已返回 `mr`，可以直接使用

---

### 规则 7：SQLite DB 创建
```go
// Before
func TestDB(t *testing.T) {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    require.NoError(t, err)
    defer func() {
        if sqlDB, err := db.DB(); err == nil {
            _ = sqlDB.Close()
        }
    }()
    // ...
}

// After
func TestDB(t *testing.T) {
    db := testutil.NewSQLiteDB(t)
    // 无需 defer 和错误检查
    // ...
}
```

---

## 实现步骤

### Phase 1：基础设施验证（已完成）

- [x] **Step 1.1**：验证 testutil 包实现完整性
  - ✅ 运行 `go test ./internal/testutil/...` 确认所有 16 个工具函数测试通过
  - ✅ 确认 `go build ./...` 无编译错误

✅ **Phase 1 验证通过** — 可以开始大规模替换

---

### Phase 2：高优先级替换（Gin 相关）

#### Step 2.1：替换 `app/auth/auth_test.go`（33 处重复）

**操作**：
1. 在文件顶部添加 import：`"github.com/huangyangke/go-aikit/internal/testutil"`
2. 删除 `init()` 函数中的 `gin.SetMode(gin.TestMode)`
3. 将所有 `setupRouter()` 中的 `gin.New()` 替换为 `testutil.NewGinRouter(t)`
4. 将所有 `post()` 函数中的 JSON 请求构造替换为 `testutil.NewJSONRequest`
5. 将所有 `httptest.NewRecorder() + r.ServeHTTP` 替换为 `testutil.ServeRequest`
6. 将所有 `assert.Equal(t, 200, w.Code)` 替换为 `testutil.AssertStatus(t, w, 200)`
7. 将所有 `json.Unmarshal(w.Body.Bytes(), &resp)` 替换为 `testutil.ParseJSONResponse(t, w, &resp)`

**验证**：
```bash
go test ./app/auth/... -v
```

**预期结果**：
- 测试全部通过
- 代码减少约 80 行
- 无需手动管理 `gin.SetMode` 和资源清理

---

#### Step 2.2：替换 `app/middleware/middleware_test.go`（21 处重复）

**操作**：
1. 添加 testutil import
2. 将所有 `gin.SetMode(gin.TestMode)` + `gin.New()` 替换为 `testutil.NewGinRouter(t)`
3. 替换所有 HTTP 请求构造和响应处理

**验证**：
```bash
go test ./app/middleware/... -v
```

---

#### Step 2.3：替换 `app/middleware/cors_test.go`

**操作**：类似 Step 2.2

**验证**：
```bash
go test ./app/middleware/cors_test.go -v
```

---

#### Step 2.4：替换 `app/middleware/rate_limit_test.go`

**操作**：类似 Step 2.2

**验证**：
```bash
go test ./app/middleware/rate_limit_test.go -v
```

---

#### Step 2.5：替换 `app/middleware/rate_limit_lua_test.go`

**操作**：类似 Step 2.2

**验证**：
```bash
go test ./app/middleware/rate_limit_lua_test.go -v
```

---

#### Step 2.6：替换 `app/async_queue/producer_test.go`

**操作**：类似 Step 2.1

**验证**：
```bash
go test ./app/async_queue/... -v
```

---

#### Step 2.7：替换 `app/async_queue/producer_log_test.go`

**操作**：类似 Step 2.1

**验证**：
```bash
go test ./app/async_queue/producer_log_test.go -v
```

---

#### Step 2.8：替换 `app/fastapp_test.go`

**操作**：类似 Step 2.1

**验证**：
```bash
go test ./app/... -v
```

---

### Phase 3：中优先级替换（Context 相关）

#### Step 3.1：替换 `agent/eino_plus/embedding/embedding_test.go`

**操作**：
1. 添加 testutil import
2. 替换所有 `context.WithCancel(context.Background())` + `defer cancel()` 为 `testutil.NewTestContext(t)`
3. 替换所有 `context.WithTimeout` 为 `testutil.NewContextWithTimeout`

**验证**：
```bash
go test ./agent/eino_plus/... -v
```

---

#### Step 3.2：替换 `agent/eino_plus/vectordb/vectordb_test.go`

**操作**：类似 Step 3.1

**验证**：
```bash
go test ./agent/eino_plus/vectordb/... -v
```

---

#### Step 3.3：替换 `agent/eino_plus/vectordb/compat_test.go`

**操作**：类似 Step 3.1

**验证**：
```bash
go test ./agent/eino_plus/vectordb/compat_test.go -v
```

---

#### Step 3.4：替换 `app/async_queue/context_test.go`

**操作**：类似 Step 3.1

**验证**：
```bash
go test ./app/async_queue/context_test.go -v
```

---

### Phase 4：低优先级替换（Database/Redis 相关）

#### Step 4.1：替换 `database/mysql/breaker_test.go`

**操作**：
1. 添加 testutil import
2. 替换 `gorm.Open(sqlite.Open(":memory:"))` 为 `testutil.NewSQLiteDB(t)`
3. 移除相关的错误检查和 defer

**验证**：
```bash
go test ./database/mysql/... -v
```

---

#### Step 4.2：替换 `database/mysql/txdb_test.go`

**操作**：类似 Step 4.1

**验证**：
```bash
go test ./database/mysql/txdb_test.go -v
```

---

#### Step 4.3：替换 `database/mysql/timestamp_test.go`

**操作**：类似 Step 4.1

**验证**：
```bash
go test ./database/mysql/timestamp_test.go -v
```

---

#### Step 4.4：替换 `database/redis/lock_test.go`

**操作**：
1. 添加 testutil import
2. 替换 `miniredis.RunT(t)` + `redis.NewClient` 为 `testutil.NewMiniRedis(t)`
3. 移除相关的 defer

**验证**：
```bash
go test ./database/redis/lock_test.go -v
```

---

#### Step 4.5：替换 `database/redis/cmd_test.go`

**操作**：类似 Step 4.4

**验证**：
```bash
go test ./database/redis/cmd_test.go -v
```

---

### Phase 5：全局验证

#### Step 5.1：运行完整测试套件

**操作**：
```bash
go test ./... -v
```

**预期**：所有测试通过，无失败

---

#### Step 5.2：代码统计分析

**操作**：
```bash
# 统计替换后的改进
echo "=== 替换前 ==="
git diff HEAD~1 --stat
git log --oneline -1

echo "=== 替换后 ==="
grep -r "testutil\." --include="*_test.go" | wc -l
grep -r "gin.New()" --include="*_test.go" | wc -l
grep -r "httptest.NewRecorder()" --include="*_test.go" | wc -l
```

**预期**：
- testutil 使用次数：> 200
- gin.New() 剩余：< 10
- httptest.NewRecorder() 剩余：< 10

---

#### Step 5.3：更新 AGENTS.md 中的 testutil 部分

**操作**：
1. 在 `AGENTS.md` 中增加"替换模式"部分
2. 添加具体的 Before/After 示例
3. 说明何时应该使用 testutil

**内容示例**：
```markdown
## 测试代码替换模式

当遇到以下模式时，应使用 `internal/testutil` 包：

### Gin Router
```go
// ❌ Before
gin.SetMode(gin.TestMode)
r := gin.New()

// ✅ After
r := testutil.NewGinRouter(t)
```

### JSON 请求
```go
// ❌ Before
body, _ := json.Marshal(data)
req, _ := http.NewRequest("POST", "/api", bytes.NewReader(body))
req.Header.Set("Content-Type", "application/json")

// ✅ After
req := testutil.NewJSONRequest(t, "POST", "/api", data)
```

### HTTP 响应
```go
// ❌ Before
w := httptest.NewRecorder()
r.ServeHTTP(w, req)
assert.Equal(t, 200, w.Code)

// ✅ After
w := testutil.ServeRequest(r, req)
testutil.AssertStatus(t, w, 200)
```

### Context
```go
// ❌ Before
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// ✅ After
ctx, cancel := testutil.NewTestContext(t)
```
```

---

#### Step 5.4：创建测试替换检查清单文档

**操作**：
创建 `docs/testing-testutil-guide.md`，包含：
1. testutil 完整 API 列表
2. 常见替换模式
3. 何时使用 testutil 的决策指南
4. 常见问题解答

---

## 验收标准

- [ ] 所有 62 个测试文件中的重复代码已替换（可安全替换的部分）
- [ ] 全量测试通过：`go test ./... -v`
- [ ] 代码减少约 1500 行（通过 `git diff --stat` 验证）
- [ ] testutil 使用次数 > 200（通过 `grep -r "testutil\." --include="*_test.go" | wc -l` 验证）
- [ ] 无编译错误：`go build ./...`
- [ ] 无 lint 错误：`golangci-lint run`

---

## 风险与缓解

### 风险 1：某些测试有特殊的 Gin 配置
**缓解**：逐个文件检查，保留特殊的中间件配置（如自定义中间件链），只替换基础的 `gin.New()` + `gin.SetMode`。

### 风险 2：某些测试需要自定义的 HTTP Client
**缓解**：`testutil.NewMockHTTPServer` 返回 `*httptest.Server`，可以自行配置 Client，不影响使用。

### 风险 3：某些测试对 Context 有特殊要求
**缓解**：`testutil.NewTestContext` 和 `testutil.NewContextWithTimeout` 已经覆盖了最常见的场景。如有特殊需求（如需要传递值），保留原有代码。

### 风险 4：替换后某些测试失败
**缓解**：每个 Phase 完成后立即运行全量测试，及时发现问题。保留 git 历史以便回滚。

---

## 工具

### 自动化检查
```bash
# 检查 testutil 使用情况
grep -r "testutil\." --include="*_test.go" . | wc -l

# 检查剩余重复
grep -r "gin.New()" --include="*_test.go" . | wc -l
grep -r "httptest.NewRecorder()" --include="*_test.go" . | wc -l
grep -r "context.WithCancel(context.Background())" --include="*_test.go" . | wc -l
```

### 测试命令
```bash
# 单文件测试
go test ./path/to/test.go -v

# 包测试
go test ./app/auth/... -v

# 全量测试
go test ./... -v
```

---

## 总结

此计划将系统性地替换项目中 62 个测试文件的 2000+ 行重复代码，全部使用已实现并测试的 `testutil` 包。预计减少 1500 行代码，统一测试模式，提高代码质量和开发效率。

**下一步**：执行 Phase 2 的高优先级替换任务。
