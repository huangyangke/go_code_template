# MySQL 熔断器修复：真正阻止 SQL 执行 + half-open 限流

**日期**: 2026-05-23
**状态**: 待实现
**范围**: `backend/pkg/aikit/resilience/breaker.go`, `backend/pkg/aikit/database/mysql/breaker.go`, 配套测试

---

## 背景与问题

当前 `mysql.BreakerPlugin`（`backend/pkg/aikit/database/mysql/breaker.go`）的实现存在一个核心 bug：**熔断器打开时仍然会执行 SQL**。

现状：

1. `beforeCallback` 只设一个 `aikit_sql_breaker_active=true` 的标记，**不检查熔断状态**，不做任何拒绝动作
2. `afterCallback` 才调 `breaker.Do(...)`，此时 SQL 已经发到 MySQL 并返回了
3. 结果：熔断器观测到失败、统计到状态，但下一个请求仍然会被打到挂掉的 DB 上 —— 违反 fail-fast 的基本语义

第二个相关问题：half-open 状态下 `gobreaker` 配置的 `MaxRequests` 形同虚设。熔断恢复期来 100 个请求，全部会被放行到刚恢复的 DB，形成"流量尖刺打回 open"的反模式。这是因为 `resilience.Breaker` 只暴露 `Do(run, fallback)` 接口，没有"放行前置判断 + 名额占用"的语义。

---

## 决策

### 核心思路

熔断保护从"先发 SQL 再统计"翻转为"先判断再放行"。具体：

- **`resilience.Breaker` 接口新增 `Allow() (done, err)`**，基于 `gobreaker.TwoStepCircuitBreaker`，原生支持 half-open 名额限流
- **`mysql.BreakerPlugin` 改用 `Allow`**：`beforeCallback` 调 `Allow()`，被拒就 `db.AddError(ErrCircuitOpen)` 让 GORM 跳过 SQL；`afterCallback` 调 `done(success)` 上报结果
- **`Do` 接口保留**，httpclient 等已有调用方零改动
- **状态传递用 `db.InstanceSet`** 把 `done` 闭包从 before 传到 after（GORM 推荐机制，请求结束即释放）

### 为什么 `AddError` 能阻止 SQL 执行

GORM 的 callback chain 本身不会因 `db.Error != nil` 短路（`callbacks.go:135-137` 是无条件遍历）。但**内置的 `gorm:create/query/update/delete` callback 自己第一行就 `if db.Error != nil { return }`**（见 `gorm@v1.31.1/callbacks/create.go:41`、`callbacks/query.go:15`），所以 before 里 `AddError` 后真正的 SQL 不会发出。

### 为什么用 `TwoStepCircuitBreaker`

`gobreaker.CircuitBreaker.Execute` 的 `beforeRequest` 已经包含 half-open `MaxRequests` 限流（`gobreaker.go:285-287`）。但它是包在 Execute 内的，调用方无法在"判断"和"执行 run()"之间插入 GORM 的 callback 机制。`TwoStepCircuitBreaker` 把这个 `beforeRequest`/`afterRequest` 拆成 `Allow()` 和 `done()` 两步，正是 plugin 场景需要的形态。

内部把 `Breaker` 实现从 `CircuitBreaker` 换成 `TwoStepCircuitBreaker` 后，`Do()` 可以无损实现为 `Allow() + run + done(success)` 的封装，对外行为完全等价。

---

## 设计

### `resilience.Breaker` 接口扩展

```go
// breaker.go
type Breaker interface {
    // Do: run() 决定 success/failure,fallback 在拒绝或失败时调用。
    // 适合一次性"包住整个操作"的场景(如 httpclient middleware)。
    Do(run func() error, fallback func(error) error) error

    // Allow: 前置放行判断。
    //   (done, nil)  → 已占用一个名额,调用方必须在操作完成后调 done(success)
    //   (nil, err)   → 拒绝(ErrCircuitOpen / ErrTooManyRequests),不要执行操作
    // 适合"判断"和"执行"分离的场景(如 GORM plugin)。
    Allow() (done func(success bool), err error)
}
```

实现切换：

```go
type gobreakerBreaker struct {
    cb *gobreaker.TwoStepCircuitBreaker  // was: *gobreaker.CircuitBreaker
}

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

`ErrCircuitOpen`、`IsCircuitOpen` 不变。metrics 上报点从原来分散在 `Do` 内部，统一收敛到 `Allow` 返回值和 `done` 闭包里 —— 两条路径口径完全一致。

### `mysql.BreakerPlugin` 改造

```go
// breaker.go
const breakerCtxKey = "aikit_sql_breaker_done"

func (p *BreakerPlugin) beforeCallback(db *gorm.DB) {
    if db.Error != nil {
        return  // 上游已有错误,不占用名额
    }
    done, err := p.breaker.Allow()
    if err != nil {
        _ = db.AddError(err)  // 内置 SQL callback 检测到 db.Error 非 nil 会跳过
        return
    }
    _ = db.InstanceSet(breakerCtxKey, done)
}

func (p *BreakerPlugin) afterCallback(db *gorm.DB) {
    raw, ok := db.InstanceGet(breakerCtxKey)
    if !ok {
        return  // before 拒绝过 / 上游已有错误,无 done 可调
    }
    done := raw.(func(success bool))
    done(sqlAcceptable(db.Error))
}
```

`sqlAcceptable` 保持原样（nil / `gorm.ErrRecordNotFound` / MySQL 1062 → 不计入失败）。

### 对比当前实现

| 维度 | 当前实现 | 修复后 |
|---|---|---|
| open 状态下是否真发 SQL | **是**（bug） | 否，before 直接 AddError 拦截 |
| half-open `MaxRequests` 限流 | **不生效** | 生效（TwoStep 内部 beforeRequest 保证） |
| before/after 状态传递 | `aikit_sql_breaker_active=true` 哑标记 | `done` 闭包，语义即"放行名额" |
| metrics 上报点 | afterCallback 内的 `breaker.Do` | `resilience.Allow`/`done` 内部，统一 |

---

## 实现要点

### 接口变更影响范围

`resilience.Breaker` 加方法是破坏性变更，但项目内调用方只有两处：

- `backend/pkg/aikit/database/mysql/breaker.go` — 本次重点改造
- `backend/pkg/aikit/app/httpclient/middleware_breaker.go` — 无需改动（仍用 `Do`）

`resilience.Breaker` 是内部包，无外部消费者，编译期就能发现遗漏。

### before 的"上游已有错误"分支

加 `if db.Error != nil { return }` 防御性早返。如果别的 plugin 在更早的 callback 里设了 `db.Error`，不应占用熔断名额，否则会污染熔断计数。开销几乎为零。

### InstanceSet 不是"内存泄漏"

之前评审讨论时质疑过 `InstanceSet` 的清理。实际验证（`gorm@v1.31.1/gorm.go:377-381`、`statement.go:540+`）：

- `InstanceSet` 的 key 是 `fmt.Sprintf("%p", stmt) + key`，存到 `stmt.Settings`（sync.Map）
- `Statement` **不走 sync.Pool**，`getInstance` 每次新建或 clone；一次 SQL 一个 Statement，执行完整个对象随 DB 实例 GC

存的 `done` 闭包随 Statement 生命周期回收，无残留。

### Raw SQL 路径不在本次范围

`db.Raw().Scan()` 和 `db.Exec("...")` 绕过 ORM callback 直接走 ConnPool，本方案（GORM plugin）覆盖不到。当前 CLAUDE.md 规定数据库操作在 DAO 层，DAO 内极少用 raw SQL，覆盖收益低、改造代价高（要 wrap ConnPool 的 4 个方法 + Tx），本次不做。如未来需要全覆盖，可单独评估"ConnPool wrapper 方案"。

---

## 测试策略

### `resilience/breaker_test.go`

**保留**现有 6 个测试作为 `Do` 路径的回归（验证内部切到 TwoStep 后行为不变）：
- `TestNew_Defaults`、`TestDo_Success`、`TestDo_Fallback`、`TestDo_OpenCircuit`、`TestIsCircuitOpen`、`TestNew_EmptyName_Panics`

**新增** `Allow` 路径覆盖：
- `TestAllow_Success_DoneSuccess` — `Allow()` 返回 done，调 `done(true)` 后状态保持 Closed
- `TestAllow_OpenReturnsError` — 推到 open 状态后，`Allow()` 返回 `ErrCircuitOpen`，且不返回 done
- `TestAllow_HalfOpen_LimitsRequests` — 配 `MaxRequests=2`，过 sleep_window 后连续调 `Allow()`，前 2 次成功、第 3 次返回 `ErrTooManyRequests`（**方案 F 的核心价值，必须有这条**）

### `mysql/breaker_test.go`

沿用现有 mysql 测试模式（`timestamp_test.go:20`、`txdb_test.go:14`），用 sqlite `:memory:` + GORM 跑真实 SQL，不引入 sqlmock。

- `TestSqlAcceptable` — 保留（已有，`metrics_test.go:22`）
- `TestBreakerPlugin_OpenBlocksSQL` — **本次修复的核心断言**。构造连续失败把 breaker 推到 open，再调 `db.Create(&row)`，断言：
  - 返回错误满足 `resilience.IsCircuitOpen`
  - 通过旁路连接（不挂 BreakerPlugin 的同库 sqlite 句柄）查询，确认表中**没有新记录** —— 证明 SQL 真没发出
- `TestBreakerPlugin_RecordNotFoundDoesntCount` — 连续 N+1 次 `First` 返回 `ErrRecordNotFound`，breaker 仍 Closed（后续正常查询应成功）
- ~~`TestBreakerPlugin_DuplicateKeyDoesntCount`~~ — 跳过。sqlite 唯一约束错误不是 MySQL 1062，`sqlAcceptable` 的 1062 分支已在 `TestSqlAcceptable` 单测覆盖，集成测里跳过避免引入 sqlmock 只为测一行

---

## 风险与回滚

**风险点**：

1. `resilience.Breaker` 加方法 → 任何外部实现（mock/fake）需要补 `Allow`。仓库内 grep 确认无外部实现，仅 `gobreakerBreaker` 一个实现
2. `TwoStepCircuitBreaker` 与 `CircuitBreaker` 的 `beforeRequest`/`afterRequest` 是同一份代码，行为等价 —— `Do` 路径回归测试覆盖这点
3. `db.AddError` 后 GORM 内置 callback 不发 SQL 的假设依赖具体版本（v1.31.1 验证通过），后续升级 GORM 需要回归 `TestBreakerPlugin_OpenBlocksSQL`

**回滚**：单文件改动，git revert 即可。配套测试独立，删除新增测试不影响其他模块。

---

## 不做的事

- **不**改 `Do` 接口签名或行为
- **不**改 httpclient breaker middleware
- **不**覆盖 raw SQL 路径（`db.Raw`/`db.Exec` 走 ConnPool，需要独立方案）
- **不**新增配置项（沿用现有 `resilience.Config`）
- **不**改 metrics 指标名（`success`/`failure`/`rejected` 三态保持，仅上报位置内聚到 resilience 包内）
