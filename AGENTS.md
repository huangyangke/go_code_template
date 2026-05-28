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