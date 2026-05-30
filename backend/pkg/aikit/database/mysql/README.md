# mysql — GORM v2 数据库封装

GORM v2 封装，内置熔断插件、Prometheus 指标插件（可选）、时间戳自动填充、事务辅助、泛型 Repository。

> **指标按入口区分**：裸 `New()` 客户端默认 **不** 采集 Prometheus 指标（适合 CLI/脚本/迁移等非服务调用）；通过 `FastApp.RegisterMySQL` 注册的实例自动启用。需手动开启时设 `EnableMetrics: true`（启用后 `Name` 必填，作为指标 datasource 标签）。熔断器始终 opt-in（`Breaker` 默认 nil）。

## 用法

```go
db := dbmysql.New(&dbmysql.Config{
    DSN:          "user:pass@tcp(localhost:3306)/db?parseTime=true",
    MaxOpenConns: 20,
    MaxIdleConns: 5,
    Breaker:      &resilience.Config{Name: "mysql-main"},
})
defer db.Close()

// CRUD
db.Create(&user)
db.First(&user, "id = ?", 1)
db.Where("name = ?", "test").Find(&users)
db.Updates(&user)
db.Delete(&user)

// 事务
db.ExecTx(ctx, func(ctx context.Context) error {
    db.CreateCtx(ctx, &order)
    db.UpdatesCtx(ctx, &inventory)
    return nil
})
```

## 泛型 Repository

消除每个模型重复的 CRUD 样板。方法均 ctx-aware：若 ctx 由 `ExecTx` 注入了事务，则自动在该事务内执行，因此可无缝组合到事务中。

```go
repo := dbmysql.NewGenericRepository[User](db) // 传 *Database，非 *gorm.DB

user, _ := repo.GetByID(ctx, 1)
users, _ := repo.FindWhere(ctx, "age > ?", 18)
items, total, _ := repo.List(ctx, 0, 20)        // 分页：offset, limit
count, _ := repo.Count(ctx, "active = ?", true) // 条件为 nil 时统计全部
repo.Create(ctx, &user)
updated, _ := repo.Update(ctx, 1, map[string]any{"name": "new"})
repo.Delete(ctx, 1)                              // 模型含 gorm.DeletedAt 时为软删除

// 与事务组合：Repository 自动复用 ctx 中的事务
db.ExecTx(ctx, func(ctx context.Context) error {
    repo.Create(ctx, &order)
    repo.Update(ctx, invID, map[string]any{"stock": gorm.Expr("stock - 1")})
    return nil // 返回非 nil 则整体回滚
})
```

未找到记录时 `GetByID`/`Update`/`Delete` 返回 `gorm.ErrRecordNotFound`。

## 配置

```yaml
dsn: user:pass@tcp(localhost:3306)/db?parseTime=true
max_open_conns: 20          # 默认
max_idle_conns: 5           # 默认
max_lifetime: 600s          # 默认
debug: false
name: main                  # 指标 datasource 标签（EnableMetrics 时必填）
breaker:                    # nil 则不启用
  name: mysql-main
  sleep_window: 5s
  error_percent_threshold: 50
enable_metrics: false       # 默认关：裸 New() 不采集指标；FastApp 注册时自动置 true
```

## 数据库迁移

基于 [golang-migrate](https://github.com/golang-migrate/migrate) 的版本化 migration 支持，使用 SQL 文件 + `embed.FS`：

```go
import "embed"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// 一步完成迁移
db.MigrateUp(ctx, migrationsFS, "migrations")

// 或使用 Migrator 做精细控制
mg, err := db.NewMigrator(migrationsFS, "migrations")
defer mg.Close()

mg.Up()                    // 全部升级
mg.Down()                  // 全部回滚
mg.Steps(1)                // 升一步
mg.Steps(-1)               // 回滚一步
mg.MigrateTo(3)            // 迁移到指定版本
v, dirty, _ := mg.Version()  // 查询当前版本
mg.Force(3)                // 强制设置版本（修复 dirty 状态）
```

SQL 文件命名规范：`{version}_{description}.{up|down}.sql`，例如：
```
migrations/
  000001_create_users.up.sql
  000001_create_users.down.sql
  000002_add_email.up.sql
  000002_add_email.down.sql
```

## 熔断策略

- **触发**：GORM 查询错误（非 `RecordNotFound`、非 MySQL 1062 Duplicate entry）
- **不触发**：`gorm.ErrRecordNotFound`、Duplicate entry（业务错误）
- **开路后**：`db.Error` 被设为 hystrix open-circuit 错误

## 指标

| 指标名 | 类型 | Labels |
|---|---|---|
| `mysql_requests_total` | counter | `family, datasource, operation, success` |
| `mysql_request_duration_seconds` | histogram | `family, datasource, operation` |
