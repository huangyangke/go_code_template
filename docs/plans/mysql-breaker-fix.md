# Feature: MySQL 熔断器修复（真正阻止 SQL + half-open 限流）

The following plan should be complete, but it's important that you validate documentation and codebase patterns and task sanity before you start implementing.

Pay special attention to naming of existing utils, types and models. Import from the right files etc.

> 源讨论文档：[`docs/disc/2026-05-23-mysql-breaker-fix-disc.md`](../disc/2026-05-23-mysql-breaker-fix-disc.md)

## Feature Description

修复 `mysql.BreakerPlugin` 的两个核心 bug，让 SQL 熔断真正实现 fail-fast：

1. **熔断打开时仍然真发 SQL**（当前 plugin 在 after 阶段才调用 `breaker.Do`，统计准但保护无效）
2. **half-open 状态 `MaxRequests` 限流形同虚设**（`resilience.Breaker.Do` 把 `gobreaker` 的 `beforeRequest` 包在 Execute 内，调用方拿不到"判断/执行"的拆分点）

修复方式：在 `resilience.Breaker` 接口增加 `Allow() (done, err)` 方法（基于 `gobreaker.TwoStepCircuitBreaker`），`mysql.BreakerPlugin` 改用 `Allow` —— before 调 `Allow()` 被拒就 `db.AddError(ErrCircuitOpen)`，after 调 `done(success)`。`Do` 方法保留，httpclient 等已有调用方零改动。

## User Story

As a **后端服务运维者**
I want to **MySQL 故障时熔断器真正阻止 SQL 发出，并在恢复期通过 half-open 限流避免流量尖刺**
So that **DB 异常期间不再雪崩，恢复期不会因首批回流请求把刚起来的 DB 再次打挂**

## Problem Statement

`backend/pkg/aikit/database/mysql/breaker.go:62` 的 `beforeCallback` 仅设置一个 `aikit_sql_breaker_active=true` 哑标记，**不检查熔断状态、不拒绝请求**；`afterCallback` 才调用 `breaker.Do(...)`，但此时 SQL 已经发到 MySQL 并返回。结果：

- 熔断器观测到失败、状态机正确翻转，但**下一个请求仍然打到挂掉的 DB** —— 违背 fail-fast 基本语义
- half-open 恢复期，`gobreaker` 内部 `MaxRequests` 限流根本没机会执行 —— 100 个请求会全部放行到刚恢复的 DB，把它再次打挂

`resilience.Breaker` 当前只暴露 `Do(run, fallback)`，把 `gobreaker` 的 `beforeRequest`（含半开名额检查）和 `run()` 绑死在 `Execute` 内部，调用方无法在两者之间插入 GORM callback chain。

## Solution Statement

接口翻转："先发再统计" → "先判断再放行"：

1. **接口扩展**：`resilience.Breaker` 新增 `Allow() (done func(success bool), err error)`，底层换成 `gobreaker.TwoStepCircuitBreaker`，原生暴露半开名额限流
2. **plugin 改造**：`mysql.BreakerPlugin.beforeCallback` 调 `Allow()`，被拒则 `db.AddError(err)`（GORM 内置 `gorm:create/query/update/delete` 第一行 `if db.Error != nil { return }` 会跳过 SQL 发出）；`afterCallback` 调 `done(sqlAcceptable(db.Error))` 上报结果
3. **状态传递**：用 `db.InstanceSet(breakerCtxKey, done)` 把 `done` 闭包从 before 传到 after。`InstanceSet` key 内含 `fmt.Sprintf("%p", stmt)+key`（gorm.go:379），生命周期随 Statement，无残留
4. **Do 路径保留**：`Do(run, fallback)` 内部用 `Allow + run + done(success)` 重新实现，对外行为 100% 等价。httpclient 的 `BreakerMiddleware` 零改动
5. **metrics 收敛**：原本 `Do` 内部上报 success/failure/rejected 三态，现在统一到 `Allow` 返回值与 `done` 闭包内 —— 两条路径口径一致；`ErrTooManyRequests`（半开名额耗尽）会被 `IsCircuitOpen` 识别并计入 `rejected`，不会沉默

## Feature Metadata

**Feature Type**: Bug Fix（含小幅 API 扩展）
**Estimated Complexity**: Medium
**Primary Systems Affected**:
- `backend/pkg/aikit/resilience/`（接口 + 实现）
- `backend/pkg/aikit/database/mysql/`（plugin 改写 + 集成测试）

**Dependencies**:
- `github.com/sony/gobreaker v1.0.0`（已在 `go.mod`，无需升级；使用 `NewTwoStepCircuitBreaker` API）
- `gorm.io/gorm v1.31.1`（依赖其 callback 短路语义；不变更）
- `gorm.io/driver/sqlite`（已用于现有 mysql 包测试，无需新增）

## Assumptions

- GORM 内置 `gorm:create/query/update/delete` callback 在 `db.Error != nil` 时短路（已在 `gorm@v1.31.1/callbacks/create.go:41` 与 `callbacks/query.go:14` 验证）
- `gobreaker.TwoStepCircuitBreaker.Allow()` 与 `CircuitBreaker.Execute()` 共享同一份 `beforeRequest()`（`gobreaker.go:229` vs `gobreaker.go:266` 都调 `cb.beforeRequest()`），行为等价
- `db.InstanceSet` 存储的 `done` 闭包随 Statement GC 回收（key 含 `%p` Statement 指针，Statement 不走 sync.Pool）
- 仓库内 `resilience.Breaker` 仅有一个实现 `gobreakerBreaker`，无外部 mock/fake（已 grep 验证：`resilience.Breaker` 仅出现在 `mysql/breaker.go:15` 一处类型声明，加方法不会破坏外部代码）
- 项目使用 sqlite `:memory:` + GORM 跑集成测试（沿用 `timestamp_test.go:20`、`txdb_test.go:14` 模式），不引入 `sqlmock`

## Open Questions

无 —— disc 文档已收敛所有边界。如实现中遇到未列入 assumptions 的分歧，立刻回到 disc 文档核对，必要时停下问用户。

## Non-goals

- **不**修改 `Do(run, fallback)` 的对外签名或语义
- **不**改动 `httpclient.BreakerMiddleware`
- **不**覆盖 raw SQL 路径（`db.Raw`/`db.Exec` 走 ConnPool 绕过 callback；如未来需全覆盖，需独立 ConnPool wrapper 方案）
- **不**新增配置项（沿用 `resilience.Config`）
- **不**改 metrics 指标名（仍是 `success`/`failure`/`rejected` 三态）

---

## CONTEXT REFERENCES

### Relevant Codebase Files IMPORTANT: YOU MUST READ THESE FILES BEFORE IMPLEMENTING!

- `backend/pkg/aikit/resilience/breaker.go`（整个文件，~95 行）
  - Why: 本次改造主战场。`gobreakerBreaker.cb` 字段类型、`Do` 实现、`metrics` 上报点都要调整；`ErrCircuitOpen`/`IsCircuitOpen` 保持不变
- `backend/pkg/aikit/resilience/breaker_test.go`（整个文件，~63 行）
  - Why: 6 个现存测试是 `Do` 路径的回归基线，**保留不动**作为 TwoStep 切换后行为等价的证据；新增 `Allow` 路径测试追加在末尾
- `backend/pkg/aikit/database/mysql/breaker.go`（整个文件，~107 行）
  - Why: `BreakerPlugin` 的 `beforeCallback`（line 62）和 `afterCallback`（line 66）是改造目标；`sqlAcceptable`（line 89）保持不变
- `backend/pkg/aikit/database/mysql/metrics_test.go`（整个文件，已包含 `TestSqlAcceptable` 和 plugin 配置相关测试）
  - Why: 新增的 `mysql/breaker_test.go` 应**与该文件并列**，沿用其 import/断言风格（`testify/assert`）
- `backend/pkg/aikit/database/mysql/timestamp_test.go:20-31`（`openTestDB` helper）
  - Why: sqlite `:memory:` + GORM + plugin 注册的标准模式；新增的 `mysql/breaker_test.go` 直接 mirror 这个 helper
- `backend/pkg/aikit/database/mysql/txdb_test.go:13-22`（`openTxTestDB` helper）
  - Why: 另一个 sqlite + Database 包装的参考；如果集成测试需要"两个连同库 sqlite 句柄"（一个挂 plugin、一个旁路），可参考其 *Database 构造方式
- `backend/pkg/aikit/database/mysql/mysql.go:140-146`（`db.Use(NewBreakerPlugin(...))` 的注册位置）
  - Why: 验证改造后 plugin 注册不变（构造函数签名 `NewBreakerPlugin(cfg resilience.Config) *BreakerPlugin` 保持）
- `backend/pkg/aikit/app/httpclient/middleware_breaker.go:37-72`（`BreakerMiddleware`）
  - Why: 唯一另一个 `resilience.Breaker` 消费者，验证 `Do` 路径未被破坏。**本次不修改此文件**，但回归测试要覆盖

### What Already Exists (Reuse, Don't Rebuild)

- `resilience.IsCircuitOpen`（`breaker.go:74`）— 已覆盖 `gobreaker.ErrOpenState` 与 `gobreaker.ErrTooManyRequests`，`Allow()` 拒绝时直接复用判断逻辑
- `resilience.ErrCircuitOpen` = `gobreaker.ErrOpenState` 别名（`breaker.go:72`）— 保持不变，外部无人 import 也好、有 import 也兼容
- `metrics.ObserveCircuitBreakerCall(name, "rejected"|"failure"|"success")`（`metrics/predefined.go:228`）— 三态标签已固定，仅迁移上报位置
- `mysql.sqlAcceptable`（`breaker.go:89`）— 已被 `TestSqlAcceptable`（`metrics_test.go:22`）覆盖；nil / `gorm.ErrRecordNotFound` / MySQL 1062 三种情况算"非失败"，不动
- `mysql/timestamp_test.go` 的 `testModel` + `openTestDB` 模式 — 新建的 `breaker_test.go` 复用同一 `testModel` 类型即可

### New Files to Create

- `backend/pkg/aikit/database/mysql/breaker_test.go`
  - 集成测试：sqlite + GORM + `BreakerPlugin`，验证 open 状态真不发 SQL、`ErrRecordNotFound` 不计失败

### Files to Update

- `backend/pkg/aikit/resilience/breaker.go` — 加 `Allow` 方法到接口；底层切到 `TwoStepCircuitBreaker`；`Do` 改为 `Allow + run + done` 包装
- `backend/pkg/aikit/resilience/breaker_test.go` — **追加**（不替换）3 个 `Allow` 路径用例
- `backend/pkg/aikit/database/mysql/breaker.go` — `beforeCallback`/`afterCallback` 改写；新增 `breakerCtxKey` 常量

### Relevant Documentation YOU SHOULD READ THESE BEFORE IMPLEMENTING!

- gobreaker `TwoStepCircuitBreaker` 源码 `/root/go/pkg/mod/github.com/sony/gobreaker@v1.0.0/gobreaker.go:133-272`
  - 关键行号：
    - `:136` — `type TwoStepCircuitBreaker struct`
    - `:183` — `func NewTwoStepCircuitBreaker(st Settings) *TwoStepCircuitBreaker`
    - `:265` — `func (tscb *TwoStepCircuitBreaker) Allow() (done func(success bool), err error)`
    - `:276` — `func (cb *CircuitBreaker) beforeRequest()`（含 half-open `MaxRequests` 限流，line 285-287）
  - Why: 直接复刻 `Do` 等价语义需要确认 `Allow` 内部就是 `cb.beforeRequest()`（line 266），与 `Execute` 的 `beforeRequest()` 调用（line 229）是同一份代码
- gorm callback 短路语义 `/root/go/pkg/mod/gorm.io/gorm@v1.31.1/`
  - `callbacks/create.go:40-43` — `return func(db *gorm.DB) { if db.Error != nil { return } ... }`
  - `callbacks/query.go:14-15` — `func Query(db *gorm.DB) { if db.Error == nil { ... } }`
  - `callbacks.go:135-137` — dispatch 循环 `for _, f := range p.fns { f(db) }`，**无** `db.Error` 短路
  - Why: 证明 `db.AddError(ErrCircuitOpen)` 后真实 SQL 不会发出，但 after callback 仍会被调用（所以 after 必须能区分"before 拒绝"和"before 放行后失败"）
- gorm `InstanceSet`/`InstanceGet` 实现 `/root/go/pkg/mod/gorm.io/gorm@v1.31.1/gorm.go:377-385`
  - `:379` — `tx.Statement.Settings.Store(fmt.Sprintf("%p", tx.Statement)+key, value)`
  - Why: 证明 InstanceSet 是 per-Statement scoped；Statement 一次 SQL 一个，闭包随 Statement 回收
- 项目内 disc：`docs/disc/2026-05-23-mysql-breaker-fix-disc.md`
  - Why: 决策依据、对比表、所有"为什么不这样做"的回答

### Patterns to Follow

**接口扩展（最小破坏性）：**
```go
// 新增方法接在已有方法后
type Breaker interface {
    Do(run func() error, fallback func(error) error) error
    Allow() (done func(success bool), err error)
}
```
- `Do` 行为不变，签名不动；现有调用方零改动
- `Allow` 文档清晰列出两种返回组合：`(done, nil)` 或 `(nil, err)`，调用方据此选择路径

**InstanceSet/Get 状态传递（已是 metrics_plugin 同款模式）：**

参考 `mysql/metrics.go:54-78` 的 `before(op)`/`after(op)` 通过 `key := "aikit_sql_metrics_start_" + op` 在 callback 间传 `time.Time`：
```go
// before
_ = db.InstanceSet(breakerCtxKey, done)
// after
raw, ok := db.InstanceGet(breakerCtxKey)
if !ok {
    return  // before 没放行（被 Allow 拒了，或上游已有 db.Error）
}
done := raw.(func(success bool))
```

**metrics 上报点收敛：**

原 `Do` 路径 metrics 在 `breaker.go:84-89`（rejected/failure 分支）和 `:91`（success）三处。改造后**唯一**的上报位置：
- `Allow()` 返回 err 时，`IsCircuitOpen` 真 → `rejected`
- `done(true)` → `success`
- `done(false)` → `failure`

`Do` 内部直接调 `b.Allow()` 复用上述上报，不再自己上报。

**测试 helper 复用：**

`mysql/breaker_test.go` 直接 import `gorm.io/driver/sqlite` 并复刻 `timestamp_test.go:20` 的 `openTestDB` 模式（命名为 `openBreakerTestDB`，可选地接 `*BreakerPlugin` 参数）。**不要**新引入 `sqlmock`。

**错误处理（按现有约定）：**

- `db.AddError(err)` 用法已见 `gorm@v1.31.1/callbacks/update.go:23` 等多处；这是 GORM 推荐的 plugin 内累加错误方式
- 加防御早返：`if db.Error != nil { return }` —— 沿用 `callbacks/create.go:41` 的写法

**命名约定：**

- 包内常量小写：`breakerCtxKey = "aikit_sql_breaker_done"`
- 函数命名沿用 `beforeCallback`/`afterCallback`（不要重命名为 `before`/`after`，会与 `metrics.go:54` 的 `before(op)` 重名歧义）
- 测试函数前缀 `TestBreakerPlugin_` 与 `TestAllow_`，与现有测试一致

---

## FLOW DIAGRAM

### 主流程（修复后）

```text
db.Create(&row)
   |
   v
GORM dispatch loop (callbacks.go:135 — 无 db.Error 短路)
   |
   v
[before: aikit:breaker_before_create]
   |
   |--- db.Error != nil? ---YES---> return (不占用名额)
   |
   |--- breaker.Allow() ---ERR(Open/TooMany)---> db.AddError(err) -> return
   |                                                         |
   |                                                         v
   |                                          内置 callback 见 db.Error 跳过 SQL
   |                                                         |
   |                                                         v
   |                                          [after: aikit:breaker_after_create]
   |                                                         |
   |                                                         v
   |                                          InstanceGet(done) -> miss -> return
   |
   |--- (done, nil) ---> InstanceSet(breakerCtxKey, done)
                                |
                                v
                  [gorm:create] 真发 SQL
                                |
                                v
                  [after: aikit:breaker_after_create]
                                |
                                v
                  InstanceGet(done) -> hit -> done(sqlAcceptable(db.Error))
                                                  |
                                                  +--> success → metrics(success)
                                                  +--> failure → metrics(failure)
```

### Do 路径（httpclient 仍用此接口，行为不变）

```text
breaker.Do(run, fallback)
   |
   v
b.Allow()
   |
   |---ERR---> metrics(rejected) -> fallback(err) or return err
   |
   |--- (done, nil) ---> run()
                            |
                            |---err---> done(false) -> metrics(failure) -> fallback(err) or return err
                            |
                            +---nil---> done(true) -> metrics(success) -> return nil
```

---

## SYSTEM BOUNDARIES

| Boundary | Input Type | Required Validation |
| --- | --- | --- |
| `resilience.New(*Config)` 入参 | 用户传入的熔断配置 | `Name` 必填（已 panic）；其余字段零值 → 走默认值（已实现，`breaker.go:32-49`） |
| `breaker.Allow()` 内部 → gobreaker | `*TwoStepCircuitBreaker` 状态机 | gobreaker 自身保证：`ErrOpenState` / `ErrTooManyRequests` / `nil`（已通过 `IsCircuitOpen` 收口） |
| GORM callback 内 `db` | `*gorm.DB`，含 `Error`、`Statement`、`Statement.Settings` | `if db.Error != nil { return }` 早返；`InstanceGet` 返回 `(value, false)` 时不强转 |
| `done` 闭包类型断言 | `interface{}` from `InstanceGet` | 断言 `func(success bool)` 失败应安全跳过（用 `, ok :=` 形态，不要 panic） |
| 测试中"旁路连接" | sqlite `:memory:` 的 file:?mode 共享 | 同一进程内用 `file::memory:?cache=shared` 让两个 DSN 指向同一 DB；或直接用 `db.DB.WithContext(ctx)` 复用句柄不挂 plugin —— **优先后者，更简单** |

---

## IMPLEMENTATION PLAN

> 改动范围窄、跨包耦合明确，按"先底层接口、再消费方、再测试"自下而上推进。每个 phase 结束都能跑 `cd backend && go build ./...` 通过 —— 中间不留半成品状态。

### Phase 1: `resilience.Breaker` 接口扩展（底层）

**目标**：把 `gobreakerBreaker` 内部从 `*gobreaker.CircuitBreaker` 切到 `*gobreaker.TwoStepCircuitBreaker`，新增 `Allow()` 方法，`Do()` 重写为 `Allow + run + done` 包装；保持 `ErrCircuitOpen`/`IsCircuitOpen`/`Config` 不变。

**Tasks:**
- 修改 `Breaker` interface 定义（加 `Allow` 方法）
- 修改 `gobreakerBreaker` 字段类型 + `New()` 构造
- 实现 `(b *gobreakerBreaker) Allow()`（含 metrics 上报）
- 重写 `(b *gobreakerBreaker) Do()` 为 `Allow + run + done` 包装

**验证**：`cd backend && go build ./pkg/aikit/resilience/...` 通过；`go test ./pkg/aikit/resilience/... -count=1 -run "TestNew_Defaults|TestDo_Success|TestDo_Fallback|TestDo_OpenCircuit|TestIsCircuitOpen|TestNew_EmptyName_Panics" -v` 全部通过 → 证明内部切换 TwoStep 后 `Do` 路径行为等价。

### Phase 2: `mysql.BreakerPlugin` 改造（消费方）

**目标**：`beforeCallback` 检查熔断状态、被拒就 `db.AddError` 拦 SQL；`afterCallback` 调 `done` 上报。

**Tasks:**
- 新增 `breakerCtxKey` 常量
- 重写 `beforeCallback`：早返 + `Allow()` + `InstanceSet(done)` 或 `AddError`
- 重写 `afterCallback`：`InstanceGet(done)` + 类型断言 + `done(sqlAcceptable(db.Error))`

**验证**：`go build ./pkg/aikit/database/mysql/...` 通过；`go test ./pkg/aikit/database/mysql/... -count=1 -run "TestSqlAcceptable|TestMetricsPlugin_Name|TestNewMetricsPlugin|TestBreakerPlugin_Name|TestConfig_BreakerField|TestConfig_Fix_MaxIdleTime" -v` 全部通过 → 证明 `Config.Fix`/`sqlAcceptable` 等周边逻辑无回归。

### Phase 3: 测试补强

**目标**：覆盖本次修复的两个核心 bug 与一项行为契约：
- (A) open 状态真不发 SQL —— `TestBreakerPlugin_OpenBlocksSQL`（防止 GORM 升级后短路语义变化漏过）
- (B) half-open 名额限流 —— `TestAllow_HalfOpen_LimitsRequests`（**方案核心价值**）
- (C) `ErrRecordNotFound` 不计入失败 —— `TestBreakerPlugin_RecordNotFoundDoesntCount`（plugin 与 sqlAcceptable 真接通）

**Tasks:**
- `resilience/breaker_test.go` 末尾追加 3 个 `Allow` 路径测试
- 创建 `mysql/breaker_test.go`（sqlite `:memory:`，复用 `testModel`）
- 不引入 sqlmock；不删除现有测试

**验证**：`go test ./pkg/aikit/resilience/... ./pkg/aikit/database/mysql/... -count=1 -v` 全部通过；`go test ./... -count=1` 全仓库无回归。

### Phase 4: 全量回归

**目标**：确认 httpclient breaker、其他依赖 `resilience.Breaker` 的代码、整仓 build/lint 均无破坏。

**Tasks:**
- 执行 `./run.sh test`
- 执行 `./run.sh lint`
- 执行 `./run.sh build`
- 人工 review diff（重点：`resilience/breaker.go` 切到 TwoStep 后行为等价；plugin 改造后 callback 注册顺序未动）

---

## STEP-BY-STEP TASKS

IMPORTANT: Execute every task in order, top to bottom. Each task is atomic and independently testable.

> 实现前**必读**：`docs/disc/2026-05-23-mysql-breaker-fix-disc.md`（决策依据）+ 本 plan 的 CONTEXT REFERENCES 区块。如发现实际代码与 disc 描述不符，停下问用户。

### Task 1: UPDATE `backend/pkg/aikit/resilience/breaker.go` — 接口加 `Allow` 方法

- **IMPLEMENT**: 在 `Breaker` interface 内、`Do` 方法之后追加 `Allow() (done func(success bool), err error)` 方法定义。`Do` 签名保持不变。
- **PATTERN**: 接口定义当前在 `breaker.go:21-23`，扩展后形如：
  ```go
  type Breaker interface {
      Do(run func() error, fallback func(error) error) error
      Allow() (done func(success bool), err error)
  }
  ```
- **IMPORTS**: 不变。
- **GOTCHA**: 接口加方法对 Go 是破坏性变更，但仓库内 `resilience.Breaker` 仅在 `mysql/breaker.go:15` 一处作为字段类型出现（无 mock/fake 实现）—— 已 grep 确认。`httpclient/middleware_breaker.go:37` 用 `resilience.New(...)` 拿到 `Breaker` 后只调 `Do`，不会因为加方法编译失败。
- **VALIDATE**: `cd backend && go build ./pkg/aikit/resilience/...` —— 此时会编译失败（`gobreakerBreaker` 还没实现 `Allow`），是预期；进入 Task 2。

### Task 2: UPDATE `backend/pkg/aikit/resilience/breaker.go` — 切到 TwoStepCircuitBreaker

- **IMPLEMENT**:
  1. `gobreakerBreaker` 的字段从 `cb *gobreaker.CircuitBreaker` 改为 `cb *gobreaker.TwoStepCircuitBreaker`
  2. `New(c *Config) Breaker` 中把 `gobreaker.NewCircuitBreaker(...)` 改为 `gobreaker.NewTwoStepCircuitBreaker(...)`，**Settings 参数完全不变**（同名字段）
- **PATTERN**: 当前 `New` 在 `breaker.go:29-71`。`gobreaker.Settings` 字段（`Name`/`MaxRequests`/`Interval`/`Timeout`/`ReadyToTrip`/`OnStateChange`）对两种 breaker 共用 —— 见 `/root/go/pkg/mod/github.com/sony/gobreaker@v1.0.0/gobreaker.go:183-188`。
- **IMPORTS**: 不变（`github.com/sony/gobreaker` 已 import）。
- **GOTCHA**:
  - `TwoStepCircuitBreaker` 的 `Name()`/`State()`/`Counts()` 方法签名与 `CircuitBreaker` 相同（`gobreaker.go:248/253/258`），下游调用 `b.cb.Name()` 不需要改。
  - 不要保留旧 `CircuitBreaker` 字段做"双轨"切换，没必要。
- **VALIDATE**: 此时 `Do` 还引用 `b.cb.Execute`（TwoStep 没有这个方法）—— 编译会报 `b.cb.Execute undefined`。继续 Task 3。

### Task 3: UPDATE `backend/pkg/aikit/resilience/breaker.go` — 实现 `Allow()`

- **IMPLEMENT**: 在 `Do` 方法**之前**新增 `Allow` 方法（让阅读顺序与接口声明顺序一致更易读）：
  ```go
  func (b *gobreakerBreaker) Allow() (func(success bool), error) {
      done, err := b.cb.Allow()
      if err != nil {
          if IsCircuitOpen(err) {
              metrics.ObserveCircuitBreakerCall(b.cb.Name(), "rejected")
          }
          return nil, err
      }
      return func(success bool) {
          if success {
              metrics.ObserveCircuitBreakerCall(b.cb.Name(), "success")
          } else {
              metrics.ObserveCircuitBreakerCall(b.cb.Name(), "failure")
          }
          done(success)
      }, nil
  }
  ```
- **PATTERN**: `metrics.ObserveCircuitBreakerCall(name, result)` 来自 `pkg/aikit/metrics/predefined.go:228`，三态标签 `success`/`failure`/`rejected` 与现状保持一致。
- **IMPORTS**: 不变。
- **GOTCHA**:
  - `IsCircuitOpen` 涵盖 `ErrOpenState` 和 `ErrTooManyRequests` 两种；非熔断错误（理论上 gobreaker 不会返回，但写防御）不打 metrics —— 与原 `Do` 路径中"非 IsCircuitOpen 的 err 计入 failure"语义不同。**这是有意的差异**：`Allow()` 返回的 err **只可能**是 `ErrOpenState` 或 `ErrTooManyRequests`（gobreaker 内部 `beforeRequest()` 唯二返回值，见 `gobreaker.go:285-294`），所以不需要 failure 分支。原 `Do` 路径里看到的 failure 来自 `run()` 自身错误，会由 `done(false)` 走到。
  - 闭包捕获 `b.cb.Name()`：name 是 `Settings.Name` 不可变，每次调一下也无副作用 —— 也可以在外层取一次缓存到局部变量 `name := b.cb.Name()`，看个人偏好。
- **VALIDATE**: `cd backend && go vet ./pkg/aikit/resilience/...`（仍未通过，Do 方法等 Task 4）。

### Task 4: UPDATE `backend/pkg/aikit/resilience/breaker.go` — 重写 `Do()` 为 `Allow + run + done` 包装

- **IMPLEMENT**: 将原 `Do` 整体替换为：
  ```go
  func (b *gobreakerBreaker) Do(run func() error, fallback func(error) error) error {
      done, err := b.Allow()
      if err != nil {
          if fallback != nil {
              return fallback(err)
          }
          return err
      }
      if err := run(); err != nil {
          done(false)
          if fallback != nil {
              return fallback(err)
          }
          return err
      }
      done(true)
      return nil
  }
  ```
- **PATTERN**: 等价复刻 `gobreaker.CircuitBreaker.Execute`（`gobreaker.go:228-244`）的语义，但通过 `Allow + done` 拆开。
- **IMPORTS**: 不变。
- **GOTCHA**:
  - **不要**在 Do 内部再调 `metrics.ObserveCircuitBreakerCall`：metrics 已经全部由 `Allow`/`done` 内部上报，重复上报会导致 success/failure 计数翻倍。
  - run 返回 err 时**先** `done(false)` **再** fallback —— 这与原实现"先决定 fallback 返回，再上报 metrics"在外部观察上等价：metrics 计数在 fallback 执行前后无差别。
  - run panic 怎么办？原 `Execute` 内部用 `defer` 捕获并标 failure（`gobreaker.go:235-241`）。本重写**不复制这个 defer** —— 项目内 `Do` 当前调用方（httpclient `middleware_breaker.go:48` 的 `next(ctx, req)`）不期望 panic 通过 breaker，调用方上层的 gin recovery 会兜底。**这是与 disc 一致的取舍**，不要主动加 defer recover。
  - 验证回归测试 `TestDo_OpenCircuit`（`breaker_test.go:38-55`）能通过：`Allow()` 在 open 状态返回 `ErrOpenState`，`Do` 走"err != nil + 无 fallback → 直接 return err"，结果 err 仍是 `ErrOpenState`，`IsCircuitOpen(err)` 真。
- **VALIDATE**:
  ```bash
  cd backend && go build ./pkg/aikit/resilience/... && \
  go test ./pkg/aikit/resilience/... -count=1 -run "TestNew_Defaults|TestDo_Success|TestDo_Fallback|TestDo_OpenCircuit|TestIsCircuitOpen|TestNew_EmptyName_Panics" -v
  ```
  6 个原测试全部通过 —— 这是"内部切换 TwoStep 后 `Do` 行为等价"的核心证据。

### Task 5: UPDATE `backend/pkg/aikit/database/mysql/breaker.go` — 新增 `breakerCtxKey` 常量

- **IMPLEMENT**: 在文件 `package` 与 import 之后、`type BreakerPlugin struct` 之前，加：
  ```go
  const breakerCtxKey = "aikit_sql_breaker_done"
  ```
- **PATTERN**: 包内常量小写、`aikit_sql_*` 前缀（已用：`aikit_sql_breaker_active` 旧标记，`aikit_sql_metrics_start_*` 在 `metrics.go:55`）。
- **IMPORTS**: 不变。
- **GOTCHA**: 旧的 `"aikit_sql_breaker_active"` 字符串将在 Task 6/7 中删除，搜全文确认没有残留。
- **VALIDATE**: `cd backend && go build ./pkg/aikit/database/mysql/...` 通过。

### Task 6: UPDATE `backend/pkg/aikit/database/mysql/breaker.go` — 重写 `beforeCallback`

- **IMPLEMENT**: 替换 `beforeCallback`（当前 `breaker.go:62-64`）为：
  ```go
  func (p *BreakerPlugin) beforeCallback(db *gorm.DB) {
      if db.Error != nil {
          return
      }
      done, err := p.breaker.Allow()
      if err != nil {
          _ = db.AddError(err)
          return
      }
      _ = db.InstanceSet(breakerCtxKey, done)
  }
  ```
- **PATTERN**:
  - `db.Error != nil` 早返 —— 与 `gorm@v1.31.1/callbacks/create.go:41-43` 同款防御
  - `db.AddError` 是 GORM 推荐的 plugin 累加错误方式，已在 `gorm@v1.31.1/callbacks/update.go:23` 等多处使用
  - `InstanceSet` 用法见 `mysql/metrics.go:57`（已是同款"before 存、after 取"模式）
- **IMPORTS**: 不变。
- **GOTCHA**:
  - **绝对不能**忘 `if db.Error != nil { return }`：上游 plugin（如未来加的鉴权 plugin）可能已写入 `db.Error`，此时占用熔断名额会污染统计。
  - `_ = db.AddError(err)` 的下划线丢弃返回值是项目惯例（看 `mysql/breaker.go:30,33,...` 等所有 callback Register 都是这写法）。
  - 这一步同时**移除** `_ = db.InstanceSet("aikit_sql_breaker_active", true)` 旧逻辑。
- **VALIDATE**: `go build ./pkg/aikit/database/mysql/...` 通过；运行 `grep -rn "aikit_sql_breaker_active" backend/` 应**无任何输出**（旧 key 已全部清理）。

### Task 7: UPDATE `backend/pkg/aikit/database/mysql/breaker.go` — 重写 `afterCallback`

- **IMPLEMENT**: 替换 `afterCallback`（当前 `breaker.go:66-87`）为：
  ```go
  func (p *BreakerPlugin) afterCallback(db *gorm.DB) {
      raw, ok := db.InstanceGet(breakerCtxKey)
      if !ok {
          return
      }
      done, ok := raw.(func(success bool))
      if !ok {
          return
      }
      done(sqlAcceptable(db.Error))
  }
  ```
- **PATTERN**: `InstanceGet` 返回 `(value, found)`，与 `metrics.go:64-69` 相同的 ok-form 防御读取。
- **IMPORTS**: 不变。
- **GOTCHA**:
  - `done(sqlAcceptable(db.Error))`：`sqlAcceptable` 返回 `true` 表示"非失败"，对应 `done(true)` 上报 success；返回 `false` 对应 `done(false)` 上报 failure。**语义对齐确认**：
    - `db.Error == nil` → acceptable=true → success ✅
    - `db.Error == ErrRecordNotFound` → acceptable=true → success ✅（这就是不让 NotFound 拖垮熔断器的核心）
    - `db.Error == 1062 重复键` → acceptable=true → success ✅
    - `db.Error == 其他` → acceptable=false → failure ✅
  - **不要**像旧实现那样在 after 里再次 `db.AddError(err)`：`Allow` 阶段已经 AddError，after 这里只负责上报。
  - `InstanceGet` miss 的合法场景：(1) before 阶段 `db.Error != nil` 早返；(2) before 阶段 `Allow()` 拒了。两种场景都不应在 after 上报 done，直接 return。
- **VALIDATE**:
  ```bash
  cd backend && go build ./pkg/aikit/database/mysql/... && \
  go test ./pkg/aikit/database/mysql/... -count=1 -v
  ```
  现有 8 个测试（`TestSqlAcceptable`/`TestMetricsPlugin_*`/`TestNewMetricsPlugin`/`TestBreakerPlugin_Name`/`TestConfig_*`/`TestTimestampPlugin_*`/`TestTxDB_*`/`TestExecTx_*`/`TestCreateCtx_*`/`TestFindCtx_*`/`TestPing`）全部通过。

### Task 8: UPDATE `backend/pkg/aikit/resilience/breaker_test.go` — 追加 `TestAllow_Success_DoneSuccess`

- **IMPLEMENT**: 在文件末尾追加：
  ```go
  func TestAllow_Success_DoneSuccess(t *testing.T) {
      brk := New(&Config{Name: "test-allow-success"})
      done, err := brk.Allow()
      assert.NoError(t, err)
      assert.NotNil(t, done)
      done(true)

      // After successful done, breaker stays closed; next Allow still succeeds.
      done2, err2 := brk.Allow()
      assert.NoError(t, err2)
      assert.NotNil(t, done2)
      done2(true)
  }
  ```
- **PATTERN**: 沿用 `breaker_test.go:11-16` 的 `assert.NoError`/`assert.NotNil` 风格。
- **IMPORTS**: 不变（`testify/assert` 和 `errors`/`testing`/`time` 已 import）。
- **GOTCHA**: Name 用独立的 `"test-allow-success"`，避免与现有测试 metrics 计数冲突（gobreaker 内部 metrics 是按 Name 分桶的）。
- **VALIDATE**: `go test ./pkg/aikit/resilience/ -run TestAllow_Success_DoneSuccess -v -count=1` 通过。

### Task 9: UPDATE `backend/pkg/aikit/resilience/breaker_test.go` — 追加 `TestAllow_OpenReturnsError`

- **IMPLEMENT**:
  ```go
  func TestAllow_OpenReturnsError(t *testing.T) {
      brk := New(&Config{
          Name:                   "test-allow-open",
          RequestVolumeThreshold: 3,
          ErrorPercentThreshold:  50,
          SleepWindow:            60 * time.Second,
      })

      // Force failures to trip the circuit.
      for i := 0; i < 10; i++ {
          done, err := brk.Allow()
          if err == nil {
              done(false)
          }
      }

      // Now open: Allow should return ErrCircuitOpen and no done.
      done, err := brk.Allow()
      assert.Nil(t, done)
      assert.True(t, IsCircuitOpen(err))
  }
  ```
- **PATTERN**: 与 `TestDo_OpenCircuit`（`breaker_test.go:38-55`）平行 —— 同样 10 次失败推到 open。
- **IMPORTS**: 不变。
- **GOTCHA**:
  - 推到 open 阶段 `Allow()` 已经可能拒绝（gobreaker 内部计数是按 `RequestVolumeThreshold` + 错误率，不是固定 N 次），所以 for 循环里要 `if err == nil { done(false) }`，不能无脑调 `done`。
  - `SleepWindow: 60s` 确保测试不会触发 half-open 转换。
- **VALIDATE**: `go test ./pkg/aikit/resilience/ -run TestAllow_OpenReturnsError -v -count=1` 通过。

### Task 10: UPDATE `backend/pkg/aikit/resilience/breaker_test.go` — 追加 `TestAllow_HalfOpen_LimitsRequests`（**方案核心价值**）

- **IMPLEMENT**:
  ```go
  func TestAllow_HalfOpen_LimitsRequests(t *testing.T) {
      brk := New(&Config{
          Name:                   "test-allow-halfopen",
          MaxRequests:            2,
          RequestVolumeThreshold: 3,
          ErrorPercentThreshold:  50,
          SleepWindow:            50 * time.Millisecond,
      })

      // Trip circuit to open.
      for i := 0; i < 10; i++ {
          done, err := brk.Allow()
          if err == nil {
              done(false)
          }
      }

      // Confirm open.
      _, errOpen := brk.Allow()
      assert.True(t, IsCircuitOpen(errOpen))

      // Wait for SleepWindow → half-open.
      time.Sleep(80 * time.Millisecond)

      // First 2 Allow should succeed (MaxRequests=2). Don't call done yet — we
      // want to keep them in-flight to exhaust the half-open budget.
      done1, err1 := brk.Allow()
      assert.NoError(t, err1)
      assert.NotNil(t, done1)

      done2, err2 := brk.Allow()
      assert.NoError(t, err2)
      assert.NotNil(t, done2)

      // Third Allow within half-open with budget exhausted → ErrTooManyRequests.
      done3, err3 := brk.Allow()
      assert.Nil(t, done3)
      assert.True(t, IsCircuitOpen(err3)) // IsCircuitOpen covers ErrTooManyRequests too.

      // Cleanup: signal success on the in-flight tokens.
      done1(true)
      done2(true)
  }
  ```
- **PATTERN**: 这是 disc 文档中明确要求的"方案 F 核心价值"测试。
- **IMPORTS**: `time` 已 import；新增任何 import 都不需要。
- **GOTCHA**:
  - `MaxRequests: 2` 是 half-open 期间允许的并发名额上限（gobreaker `Settings.MaxRequests` 注释见 `gobreaker.go:69-71`）。
  - **必须先抢占 2 个名额且不调 done，再请求第 3 个**，否则 done 上报后名额释放、第 3 个会成功。
  - `SleepWindow: 50ms` + `time.Sleep(80ms)` 留出余量避免时序闪烁。
  - `IsCircuitOpen` 已覆盖 `ErrTooManyRequests`（`breaker.go:74-76` 的 `||` 分支），所以断言 `IsCircuitOpen(err3)` 是对的。
- **VALIDATE**: `go test ./pkg/aikit/resilience/ -run TestAllow_HalfOpen_LimitsRequests -v -count=1` 通过；建议加 `-count=5` 跑 5 次确认无时序闪烁。

### Task 11: CREATE `backend/pkg/aikit/database/mysql/breaker_test.go`

- **IMPLEMENT**: 新文件，包含 helper + 2 个集成测试用例：
  ```go
  package mysql

  import (
      "errors"
      "testing"
      "time"

      "github.com/stretchr/testify/assert"
      "gorm.io/driver/sqlite"
      "gorm.io/gorm"

      "github.com/example/go-template/pkg/aikit/resilience"
  )

  // openBreakerTestDB opens a sqlite :memory: DB with the breaker plugin attached.
  // Returns (db, plugin) so tests can introspect breaker state if needed.
  func openBreakerTestDB(t *testing.T, cfg resilience.Config) *gorm.DB {
      t.Helper()
      db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
      if err != nil {
          t.Fatalf("open sqlite: %v", err)
      }
      if err := db.AutoMigrate(&testModel{}); err != nil {
          t.Fatalf("auto migrate: %v", err)
      }
      if err := db.Use(NewBreakerPlugin(cfg)); err != nil {
          t.Fatalf("register breaker plugin: %v", err)
      }
      return db
  }

  // TestBreakerPlugin_OpenBlocksSQL is the headline regression test for this fix:
  // when the breaker is open, db.Create must NOT actually send SQL to the
  // database. We verify by counting rows via a separate gorm.DB handle (without
  // the plugin) on the *same* sqlite handle — i.e. we use the underlying *sql.DB
  // to bypass the plugin chain.
  func TestBreakerPlugin_OpenBlocksSQL(t *testing.T) {
      cfg := resilience.Config{
          Name:                   "test-mysql-breaker-open",
          RequestVolumeThreshold: 3,
          ErrorPercentThreshold:  50,
          SleepWindow:            60 * time.Second,
      }
      db := openBreakerTestDB(t, cfg)

      // Trip the breaker by issuing failing operations. We use Find against a
      // non-existent table to generate genuine non-acceptable errors.
      type doesNotExist struct{ ID uint }
      for i := 0; i < 10; i++ {
          var dst []doesNotExist
          _ = db.Find(&dst).Error
      }

      // Now the breaker should be open. Attempt a Create; it must fail with
      // ErrCircuitOpen and must NOT insert a row.
      err := db.Create(&testModel{Name: "should_not_insert"}).Error
      assert.True(t, resilience.IsCircuitOpen(err), "expected open circuit, got %v", err)

      // Bypass the plugin: use the raw *sql.DB to count rows in test_models.
      sqlDB, err2 := db.DB()
      assert.NoError(t, err2)
      var count int
      row := sqlDB.QueryRow("SELECT COUNT(*) FROM test_models WHERE name = ?", "should_not_insert")
      assert.NoError(t, row.Scan(&count))
      assert.Equal(t, 0, count, "open breaker must block SQL — no row should be inserted")
  }

  // TestBreakerPlugin_RecordNotFoundDoesntCount verifies that ErrRecordNotFound
  // is treated as acceptable by the plugin — a stream of NotFounds should not
  // open the breaker.
  func TestBreakerPlugin_RecordNotFoundDoesntCount(t *testing.T) {
      cfg := resilience.Config{
          Name:                   "test-mysql-breaker-notfound",
          RequestVolumeThreshold: 3,
          ErrorPercentThreshold:  50,
          SleepWindow:            60 * time.Second,
      }
      db := openBreakerTestDB(t, cfg)

      // 10 NotFounds.
      for i := 0; i < 10; i++ {
          var m testModel
          err := db.First(&m, "name = ?", "nonexistent").Error
          assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
      }

      // Breaker should still be closed: a normal Create must succeed.
      m := &testModel{Name: "after_notfounds"}
      assert.NoError(t, db.Create(m).Error)
      assert.NotZero(t, m.ID)
  }
  ```
- **PATTERN**:
  - sqlite `:memory:` + GORM helper → 直接 mirror `timestamp_test.go:20-31`
  - `testModel` 复用 `timestamp_test.go:11-16` 已定义的类型（同 package 直接可用）
  - `resilience.Config` 用法 → `breaker_test.go:38-43` 的字段组合
- **IMPORTS**:
  - `errors`、`testing`、`time` —— 标准库
  - `github.com/stretchr/testify/assert` —— 已是项目测试惯例
  - `gorm.io/driver/sqlite`、`gorm.io/gorm` —— `timestamp_test.go` 已有
  - `github.com/example/go-template/pkg/aikit/resilience` —— 用 `resilience.Config` / `resilience.IsCircuitOpen`
- **GOTCHA**:
  - **不要**引入 `sqlmock`：项目惯例就是 sqlite `:memory:`，引新依赖与 disc 决策不符。
  - "旁路连接"用 `db.DB()`（`*sql.DB`，绕过 plugin chain）查表 —— 这是最简单的选项，比 disc 文档原写的"另一个 sqlite 句柄"更省事，因为同一个 `*sql.DB` 对象等价同一个 sqlite memory DB，且 `*sql.DB.QueryRow` 不走 GORM callback。
  - SQL 注入：测试中表名 `test_models`（GORM 默认从 `testModel` 推导出来 —— 验证方式：临时加个 print 或先跑一次让 `db.AutoMigrate` 报错时观察）。如果不确定可以临时加一行 `t.Log(db.Statement.Table)` 打印验证，提交时删掉。
  - `db.Find(&dst).Error` 触发"表不存在"错误（sqlite 会报 `no such table`），这是非 acceptable 错误，会增加 breaker 失败计数。**不要**用 `db.First(&m)`（会返回 `ErrRecordNotFound`，被 sqlAcceptable 当成功）。
  - 测试名 prefix `TestBreakerPlugin_` 与现有 `mysql/metrics_test.go:53` 的 `TestBreakerPlugin_Name` 一致。
- **VALIDATE**:
  ```bash
  cd backend && go test ./pkg/aikit/database/mysql/ -run "TestBreakerPlugin_OpenBlocksSQL|TestBreakerPlugin_RecordNotFoundDoesntCount" -v -count=1
  ```
  两个测试通过；`TestBreakerPlugin_OpenBlocksSQL` 失败说明本次修复**没真正阻止 SQL** —— 这就是 disc 文档要解决的核心 bug 的回归保险栓。

### Task 12: 全量回归

- **IMPLEMENT**: 不写代码，按以下顺序跑命令、肉眼检查 diff：
  1. `cd backend && go build ./...` —— 整仓 build 通过
  2. `cd backend && go test ./... -count=1` —— 整仓测试通过（重点关注 `pkg/aikit/app/httpclient/` 下的测试，证明 `Do` 路径未破坏）
  3. `./run.sh lint` —— golangci-lint 通过
  4. `git diff --stat` —— 确认改动文件就是 `resilience/breaker.go`、`resilience/breaker_test.go`、`mysql/breaker.go`、`mysql/breaker_test.go`（新增），其他文件零改动
- **PATTERN**: 项目"小功能完整闭环迭代"（CLAUDE.md）—— build/test/lint 三件套必须齐全。
- **GOTCHA**: 如果 `httpclient` 测试有 metrics 断言失败，回头看 Task 4 的 GOTCHA 第一条（不要在 Do 内重复上报 metrics）。
- **VALIDATE**: 三条命令均退出码 0；diff 只动了上述 4 个文件。

---

## TESTING STRATEGY

### Unit Tests

**目标覆盖**：

| 用例 | 文件 | 覆盖点 |
|---|---|---|
| `TestNew_Defaults` | `resilience/breaker_test.go`（已存在） | `New` 默认值填充 |
| `TestDo_Success` | 同上（已存在） | `Do` 成功路径 → metrics(success) |
| `TestDo_Fallback` | 同上（已存在） | `Do` 失败路径走 fallback |
| `TestDo_OpenCircuit` | 同上（已存在） | `Do` 在 open 时返回 `ErrCircuitOpen` |
| `TestIsCircuitOpen` | 同上（已存在） | `IsCircuitOpen` 识别两种错误 |
| `TestNew_EmptyName_Panics` | 同上（已存在） | 配置校验 |
| `TestAllow_Success_DoneSuccess` | 同上（**新增**） | `Allow` + `done(true)` 闭环 |
| `TestAllow_OpenReturnsError` | 同上（**新增**） | `Allow` 在 open 时返回 err、不返回 done |
| `TestAllow_HalfOpen_LimitsRequests` | 同上（**新增**） | half-open `MaxRequests` 限流（**方案核心价值**） |
| `TestSqlAcceptable` | `mysql/metrics_test.go`（已存在） | `sqlAcceptable` 三种 acceptable 情况 |
| `TestBreakerPlugin_Name` 等周边 | 同上（已存在） | plugin 构造 + Config 字段 |

**测试框架**：`testing` + `github.com/stretchr/testify/assert`（项目惯例，见 `resilience/breaker_test.go:8`、`mysql/metrics_test.go:8`）。

### Integration Tests

| 用例 | 文件 | 覆盖点 |
|---|---|---|
| `TestBreakerPlugin_OpenBlocksSQL` | `mysql/breaker_test.go`（**新建**） | **本次修复的核心契约**：open 状态 `db.Create` 不真发 SQL —— 通过旁路 `*sql.DB` 查表行数为 0 验证 |
| `TestBreakerPlugin_RecordNotFoundDoesntCount` | 同上（**新建**） | 10 次 `ErrRecordNotFound` 后 breaker 仍 Closed，后续 Create 成功 |
| `TestBreakerMiddleware_5xxCountsAsFailure` | `httpclient/middleware_test.go:29`（已存在，**不动**） | 验证 `Do` 路径在 TwoStep 切换后行为不变 |
| `TestBreakerMiddleware_4xxDoesNotCount` | `httpclient/middleware_test.go:71`（已存在，**不动**） | 同上，fallback 路径不变 |

**测试环境**：sqlite `:memory:` + GORM（沿用 `timestamp_test.go:20-31`、`txdb_test.go:13-22` 模式）。**禁止引入** `sqlmock` 或真 MySQL 容器。

### Edge Cases

显式覆盖：

1. **`db.Error` 上游已写入** → before 早返，不占用熔断名额（防御逻辑写在 `beforeCallback` 第一行，**通过代码 review 验证**，不专门写测试 —— 没有便利的方式从外部注入"前置 plugin 写 db.Error"的场景，且代码极简肉眼可证）
2. **`InstanceGet` miss**（before 拒了 / 上游已有 error）→ after 直接 return，不 panic —— `TestBreakerPlugin_OpenBlocksSQL` 隐含覆盖：op 进入 after callback 时 done 是 nil，必须正常返回
3. **`InstanceGet` 类型断言失败** → after 直接 return（防御性 ok-form，理论上不会发生，因为同一 plugin 写入读取）—— 代码即文档，不专测
4. **并发场景** → gobreaker 自身保证（用 `sync.Mutex`，见 `gobreaker.go:142`）；plugin 层无共享状态，每次 SQL 一个 Statement —— 不专测
5. **`MaxRequests=0` 默认值** → `New` 已 fix 为 1（`breaker.go:32-34`），与现有 `TestNew_Defaults` 覆盖一致
6. **`ErrTooManyRequests` 的 metrics 归类** → `IsCircuitOpen` 真 → 计入 `rejected`（**通过 `TestAllow_HalfOpen_LimitsRequests` 覆盖**：第 3 次 `Allow` 返回 `ErrTooManyRequests`，`IsCircuitOpen` 为 true）

**不专测的边界**（已在代码层面消除，写测试是噪音）：
- run() panic 的处理 —— 见 Task 4 GOTCHA：项目不期望 panic 通过 breaker
- raw SQL 路径（`db.Raw`/`db.Exec`）—— disc 已明确不在范围内

## FAILURE MODES

| Failure Mode | Trigger | Expected Handling | Test Required |
|---|---|---|---|
| MySQL 完全不可达 | 网络分区 / DB 宕机 | 连续失败累积到 `RequestVolumeThreshold`，breaker 翻 Open；后续 SQL 直接被 `db.AddError(ErrCircuitOpen)` 拦截，错误传到 DAO/Service 层。Service 应识别 `resilience.IsCircuitOpen(err)` 或上层 `bcode` 转换（不在本次范围） | `TestBreakerPlugin_OpenBlocksSQL` ✅ |
| half-open 恢复期流量尖刺 | sleep window 到期，业务流量瞬时涌入 | gobreaker 内 `MaxRequests` 限流，前 N 个放行用于探测，第 N+1 个返回 `ErrTooManyRequests`，被 `IsCircuitOpen` 识别为 rejected（不雪崩刚恢复的 DB） | `TestAllow_HalfOpen_LimitsRequests` ✅ |
| half-open 探测请求失败 | DB 还没真恢复 | gobreaker 自动把状态转回 Open 并重置 timer | gobreaker 自身已测，本仓库不重复测 |
| `ErrRecordNotFound` 高频出现 | 业务正常查询无记录（如登录态校验） | `sqlAcceptable` 返回 true → `done(true)` → 不计入失败 → breaker 不会被这类正常业务推开 | `TestBreakerPlugin_RecordNotFoundDoesntCount` ✅ |
| MySQL 1062 重复键高频 | 业务幂等写场景 | 同上，1062 → `sqlAcceptable` true → 不计失败 | `TestSqlAcceptable`（已有单测） |
| 上游 plugin 已写 `db.Error` | 鉴权 plugin 失败 | before 早返，不占用熔断名额；不污染熔断统计 | 代码 review（见 Edge Cases #1） |
| GORM 升级破坏内置 callback 短路语义 | 升级 gorm 到新版本 | `TestBreakerPlugin_OpenBlocksSQL` 会失败 —— 这就是该测试的回归保险栓 | `TestBreakerPlugin_OpenBlocksSQL` ✅ |
| Statement 复用导致 `done` 串台 | 理论上 GORM 不会发生 | InstanceSet key 含 `%p` Statement 指针（gorm.go:379），每次 SQL 一个 Statement，物理隔离 | 不专测（库实现保证） |

---

## VALIDATION COMMANDS

> 全部命令在仓库根目录执行；`backend/` 是 Go module 根。

### Level 1: Syntax & Style

```bash
cd backend && go vet ./pkg/aikit/resilience/... ./pkg/aikit/database/mysql/...
./run.sh lint
```

通过标准：`go vet` 无输出；`golangci-lint` 退出码 0（如机器未装 lint，`run.sh` 会跳过并 warning —— **不算通过**，需在装好的环境复跑）。

### Level 2: Unit Tests

```bash
# 单独跑 resilience 包（验证 Phase 1 Do 路径等价 + Allow 新路径）
cd backend && go test ./pkg/aikit/resilience/ -count=1 -v

# 单独跑 mysql 包（验证 Phase 2 plugin 改造）
cd backend && go test ./pkg/aikit/database/mysql/ -count=1 -v
```

通过标准：全部 PASS。重点关注：
- `TestDo_OpenCircuit` —— 切到 TwoStep 后仍能正确返回 `ErrCircuitOpen`（行为等价）
- `TestAllow_HalfOpen_LimitsRequests` —— 半开名额限流真生效
- `TestBreakerPlugin_OpenBlocksSQL` —— SQL 真没发出（旁路查表 count=0）

### Level 3: Integration Tests

```bash
# 跨包验证：Do 路径在 httpclient 上未破坏
cd backend && go test ./pkg/aikit/app/httpclient/ -count=1 -v -run "TestBreakerMiddleware_"
```

通过标准：`TestBreakerMiddleware_5xxCountsAsFailure` 与 `TestBreakerMiddleware_4xxDoesNotCount`（`middleware_test.go:29,71`）均 PASS。

### Level 4: 全量回归

```bash
./run.sh test
./run.sh build
```

通过标准：
- `./run.sh test` → 整仓 `go test ./...` 退出码 0
- `./run.sh build` → 二进制产出，无编译错误

### Level 5: Diff 自审

```bash
git diff --stat backend/pkg/aikit/
```

通过标准：改动文件列表**严格等于**：
```
backend/pkg/aikit/resilience/breaker.go
backend/pkg/aikit/resilience/breaker_test.go
backend/pkg/aikit/database/mysql/breaker.go
backend/pkg/aikit/database/mysql/breaker_test.go  (new)
```

其他文件零改动。如有意外文件被动到，回滚并查清原因。

---

## ACCEPTANCE CRITERIA

- [ ] `resilience.Breaker` 接口暴露 `Allow() (func(success bool), error)`，对应实现走 `gobreaker.TwoStepCircuitBreaker`
- [ ] `resilience.Breaker.Do` 对外签名与语义不变，httpclient 的 `TestBreakerMiddleware_*` 全部通过
- [ ] `mysql.BreakerPlugin.beforeCallback` 在熔断打开时通过 `db.AddError` 阻止 SQL 真正发出
- [ ] `mysql.BreakerPlugin.afterCallback` 通过 `InstanceGet` 拿到 before 设置的 `done`，并按 `sqlAcceptable(db.Error)` 上报
- [ ] half-open 状态下 `MaxRequests` 名额限流真生效（`TestAllow_HalfOpen_LimitsRequests` PASS）
- [ ] open 状态下 SQL 真不发出（`TestBreakerPlugin_OpenBlocksSQL` 通过旁路 `*sql.DB` 验证表行数为 0）
- [ ] `ErrRecordNotFound` 高频不会推开熔断器（`TestBreakerPlugin_RecordNotFoundDoesntCount` PASS）
- [ ] metrics 三态（`success` / `failure` / `rejected`）标签名不变，上报点统一在 `resilience` 包内（不在 plugin 层）
- [ ] 旧的 `aikit_sql_breaker_active` 字符串 key 在仓库内已**完全清除**（`grep -rn aikit_sql_breaker_active backend/` 无输出）
- [ ] 整仓 `go build ./...` / `go test ./... -count=1` / `./run.sh lint` 全绿
- [ ] 改动文件严格限定在 4 个（见 Level 5 Diff 自审）

---

## COMPLETION CHECKLIST

- [ ] Task 1-12 按顺序全部完成
- [ ] 每个 Task 的 VALIDATE 命令在该 Task 完成时立刻跑通
- [ ] 5 个 Validation Levels 全部通过
- [ ] 全 Acceptance Criteria 勾选
- [ ] disc 文档（`docs/disc/2026-05-23-mysql-breaker-fix-disc.md`）"不做的事"清单条目没被违反（特别是：未改 Do 签名 / 未改 httpclient middleware / 未引入 sqlmock / 未新增 Config 字段 / 未改 metrics 指标名）
- [ ] commit message 包含本 plan 文件路径与 disc 文件路径

---

## NOTES

### 关键设计取舍（已锁定，实现时不要二次推翻）

1. **接口扩展 vs 新接口** — 选了"在现有 `Breaker` 加 `Allow`"。理由：仓库内仅一个实现一个跨包消费者，加方法的破坏面已 grep 验证为零；新建 `TwoStepBreaker` 接口反而引入两个并行抽象，徒增心智。

2. **`Allow` 还是 `Acquire`/`TryAcquire`** — 选了 `Allow` 对齐 gobreaker 命名（`TwoStepCircuitBreaker.Allow()`），降低读源码的认知阻抗。

3. **状态传递用 `InstanceSet` 而非 ctx** — GORM 的 `Statement.Context` 是请求范围 ctx，污染它需要构造新 ctx；`InstanceSet` 是 GORM plugin 间状态传递的官方推荐机制（见 `metrics.go:54-78` 同款用法），key 自带 Statement 指针做隔离，最干净。

4. **`Do` 内不重报 metrics** — 见 Task 4 GOTCHA 第一条。所有 success/failure/rejected 一律由 `Allow`/`done` 上报，单一上报点。

5. **不加 `defer recover()` 到 `Do`** — 与 `gobreaker.Execute` 行为有微小偏差。理由：项目内 `Do` 的两个调用方都不期望 panic 通过 breaker，gin recovery 兜底。如未来需要 panic 也计入 failure，单独提 issue 评估。

6. **测试不引 sqlmock** — disc 已明确决策。1062 重复键路径用 `TestSqlAcceptable` 单测覆盖（不在集成测里 reproduce sqlite 唯一约束 → MySQL 错误码的转换）。

### 实现 review 时的"嗅探信号"

如果实现 PR 出现以下任一信号，**回退重做**：
- ❌ `resilience.Breaker` 接口加了第三个方法（不在 plan 里的方法）
- ❌ `Do` 签名变了（参数 / 返回值）
- ❌ 任何 `httpclient/` 下的文件被改动
- ❌ `metrics.ObserveCircuitBreakerCall` 在 `mysql/breaker.go` 内被直接调用（应只在 `resilience/breaker.go` 内出现）
- ❌ 新增 `Config` 字段或新增 yaml key
- ❌ 引入了新的 Go module 依赖（`go.mod` / `go.sum` 被动）
- ❌ 集成测试用了 sqlmock、testcontainers 或真 MySQL
- ❌ 旧字符串 `aikit_sql_breaker_active` 仍残留在代码中

### 可选后续（不在本次范围）

- raw SQL 路径覆盖（`db.Raw`/`db.Exec`）→ 需要 ConnPool wrapper，单独立项
- breaker 配置热重载 → `resilience.New` 当前是一次性构造，热重载需要新接口
- per-statement breaker（按 SQL 分类做独立熔断）→ 业务收益尚未验证，不做预先设计
