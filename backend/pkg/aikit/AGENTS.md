# CLAUDE.md

---

# 行为准则

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

---

# 项目规范

## 注释风格

**语言: 中文, 标点末尾: ASCII `.`

### 文件头

`// Package xxx` 写在 `package` 声明上方，这是 godoc 识别的唯一格式.

```go
// Package cache 多级缓存（本地 + Redis）.
// 提供带 TTL 的自动缓存、手动失效、Pub/Sub 跨节点同步.
package cache
```

### 导出类型/常量/变量

```go
// Consumer 处理异步任务的消费端.
type Consumer struct {
    Group     string // 消费组名称
    MaxRetry  int    // 最大重试次数
}

// DefaultTTL 默认缓存过期时间.
const DefaultTTL = 30 * time.Minute
```

### 导出函数

固定模板, 每行末尾句点:

```go
// CreateOrder 创建并提交新订单.
// 参数：items - 订单商品列表, userID - 下单用户ID.
// 返回值：orderID - 订单编号, err - 创建失败时的错误.
func CreateOrder(items []Item, userID string) (orderID string, err error) { ... }

// Lock 获取分布式锁.
// 参数：ctx - 上下文, key - 锁定的资源标识, ttl - 锁持有时长.
// 返回值：unlock - 释放锁的函数, err - 获取失败时的错误.
func Lock(ctx context.Context, key string, ttl time.Duration) (unlock func() error, err error) { ... }
```

### 函数内注释

纯中文, 不加句点, 只写 WHY 不写 WHAT. 一眼能看懂的代码不加注释.

```go
func CreateOrder(items []Item, userID string) (string, error) {
    // 库存校验必须在支付前完成，避免超卖后退款
    if err := checkStock(items); err != nil {
        return "", err
    }
    orderID = generateOrderID()
    return orderID, nil
}
```

## Testutil 设计规范

`internal/testutil/` 是测试基础设施，遵循以下约定.

### 职责边界

- **只放测试工具代码**：环境创建、数据生成、断言辅助、Mock 服务、时间控制、资源隔离.
- **不放业务逻辑**：任何与具体业务相关的测试属于 `_test.go` 文件，不属于 testutil.
- **不放特定测试用例**：testutil 只提供可复用的工具函数，不测试具体功能.

### 函数设计

```go
// NewXxx 带 *testing.T 的环境创建函数，必须调用三件套:
//   1. t.Helper()     — 错误指向调用者
//   2. t.Cleanup(fn)  — 测试结束自动清理资源
//   3. t.Fatalf(err)  — 失败直接终止，不返回 error
func NewMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
    t.Helper()
    mr := miniredis.RunT(t)
    client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
    t.Cleanup(func() { _ = client.Close() })
    return mr, client
}
```

### 设计原则

| 原则 | 做法 | 反例 |
|------|------|------|
| 自动清理 | `t.Cleanup()` 注册资源释放 | 要求调用者手动 `Close()` |
| 显式依赖 | 返回所有相关对象 | 隐藏 miniredis 只返回 client |
| 失败即崩溃 | `t.Fatal()` 不返回 error | `return nil, err` 让调用者判断 |
| 单一职责 | 一个函数做一件事 | `SetupEverything()` 返回 5 个对象 |
| t.Helper | 所有 `New*` 函数开头调用 | 省略，错误定位到 testutil 内部 |

### 抽取时机

当以下模式在 `_test.go` 中出现 3 次以上，考虑抽取到 testutil:

- 环境搭建样板（创建连接 + 配置 + cleanup）
- 数据断言重复（状态码检查 + JSON 解析 + 字段比较）
- 随机标识生成（`uuid.New()` 或 `rand` 组合）
- Mock 服务搭建（`httptest.NewServer` + handler）

新工具追加到 `testutil.go`，对应测试写入 `testutil_xxx_test.go`. 避免拆分成多文件（除非单文件超过 300 行）.