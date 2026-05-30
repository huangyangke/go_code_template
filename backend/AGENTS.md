# Go 后端项目规范

## 说明

- `internal/api/article.go` 是示例实现，参考该示例开发其他功能

---

## 一、分层架构

采用 **Handler → Service → DAO → Model** 四层架构：

```
internal/
├── api/        # Handler 层：HTTP 路由、参数解析、响应格式化
├── service/    # Service 层：业务逻辑、事务控制、bcode 错误转换
├── dao/        # DAO 层：SQL 操作 (CRUD)
├── model/      # Model 层：GORM 表结构、ORM 映射
│   └── constants/  # Redis Key 模板
├── schema/     # DTO：请求/响应结构体
└── errors/     # 业务错误码（AppError）
```

| 层级 | 职责 | 依赖方向 |
|------|------|----------|
| Handler | HTTP 路由、参数解析、响应格式化 | → Service |
| Service | 业务逻辑、事务控制、errors 转换 | → DAO |
| DAO | SQL 操作 (CRUD) | → Model |
| Model | 表结构、ORM 映射 | 无 |

---

## 二、各层规范

### Handler 层

- 使用 `response.JSONErr(c, data, err)` 统一响应，禁止直接调用 `c.JSON`
- 参数绑定失败用 `response.ParamError(c)` 返回；路径参数解析失败同样用 `response.ParamError(c)`
- 禁止在 Handler 里打错误日志——`response.JSONErr` 内部已对非业务错误自动记录日志
- 禁止在 Handler 里用 `errors.As` 判断错误类型——错误分发是 `response.JSONErr` 的职责
- 新路由在 `internal/api/router.go` 的 `RegisterRoutes` 中注册

```go
func (h *ArticleHandler) Get(c *gin.Context) {
    id, err := strconv.ParseUint(c.Param("id"), 10, 64)
    if err != nil {
        response.ParamError(c)
        return
    }
    article, err := h.svc.Get(c.Request.Context(), uint(id))
    response.JSONErr(c, article, err)
}
```

### Service 层

- 定义 `interface`（供测试 mock），实现绑定到该 interface
- 将 DAO 层错误（`gorm.ErrRecordNotFound` 等）转换为 `*apperrors.AppError`
- 禁止直接调用 GORM

```go
type articleRepository interface {
    GetByID(ctx context.Context, id uint) (*model.Article, error)
}

func (s *ArticleService) Get(ctx context.Context, id uint) (*model.Article, error) {
    article, err := s.repo.GetByID(ctx, id)
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, apperrors.ErrArticleNotFound
    }
    return article, err
}
```

#### 跨 DAO 事务

多个 DAO 写操作需原子提交时，用 `db.ExecTx` 包裹：注入 ctx 的事务会被 DAO（经泛型 Repository → `db.WithContext(ctx)`）自动复用，无需层层传 `*gorm.DB`。返回非 nil error 即整体回滚。

```go
// Service 持有 *dbmysql.Database 才能开启事务；DAO 仍只接收 ctx.
func (s *OrderService) Place(ctx context.Context, o *model.Order) error {
    return s.db.ExecTx(ctx, func(ctx context.Context) error {
        if err := s.orderDAO.Create(ctx, o); err != nil {
            return err // 回滚
        }
        return s.stockDAO.Decr(ctx, o.SkuID, o.Qty) // 任一步出错则全部回滚
    })
}
```

### DAO 层

- 只负责数据访问，不含业务逻辑
- 内嵌 aikit 泛型 `Repository[T]`，免写常规 CRUD：`List`/`Create` 签名一致直接提升；`GetByID`/`Update`/`Delete` 收窄主键为 `uint`，一行委托
- 经 Repository 走 `Database.WithContext(ctx)`，自动复用 `ExecTx` 注入的事务
- 泛型 Repository 已内置软删除与 `RowsAffected==0 → gorm.ErrRecordNotFound`，无需手写
- 仅当需要自定义查询（联表、聚合）时才手写方法，此时取 `database.WithContext(ctx)` 操作

```go
type ArticleDAO struct {
    *dbmysql.Repository[model.Article]
}

func NewArticleDAO(db *dbmysql.Database) *ArticleDAO {
    return &ArticleDAO{Repository: dbmysql.NewGenericRepository[model.Article](db)}
}

// 收窄主键类型，一行委托.
func (d *ArticleDAO) Delete(ctx context.Context, id uint) error {
    return d.Repository.Delete(ctx, id)
}
```

### Model 层

- 嵌入 `dbmysql.Model`（含 ID、CreatedAt、UpdatedAt、DeletedAt）
- 实现 `TableName() string`

```go
type Article struct {
    dbmysql.Model
    Title   string `gorm:"size:255;not null" json:"title"`
    Content string `gorm:"type:text;not null" json:"content"`
}

func (Article) TableName() string { return "articles" }
```

### Schema 层

- 请求结构体后缀 `Req`，响应后缀 `Resp`
- 使用 `binding` tag 做参数校验

```go
type CreateArticleReq struct {
    Title   string `json:"title"   binding:"required,max=255"`
    Content string `json:"content" binding:"required"`
}
```

---

## 三、业务错误码（AppError）

定义在 `internal/errors/errors.go`，按领域分段：

| 领域 | 码段 | 备注 |
|------|------|------|
| 框架保留（response 包） | 10000–10099 | 禁止在 errors 中使用 |
| Article | 10100–10199 | |
| User | 10200–10299 | |
| Order | 10300–10399 | |

```go
var ErrArticleNotFound = &AppError{http.StatusNotFound, 10100, "文章不存在"}
```

命名规范：`Err{Domain}{Reason}`，如 `ErrUserNotFound`、`ErrOrderExpired`。

---

## 四、Redis Key 规范

Key 模板定义在 `internal/model/constants/redis_key.go`，key 与 TTL 绑定：

```go
var KeyArticle = NewKey("go-template:article:%d", 5*time.Minute)

// 使用
key := constants.KeyArticle.Format(articleID)   // 生成 key 字符串
ttl := constants.KeyArticle.TTL                 // 取 TTL
```

---

## 五、数据库迁移

迁移文件放在 `cmd/migrate/migrations/`，格式：`{序号}_{描述}.up.sql` / `.down.sql`。

```bash
./run.sh migrate    # 执行迁移（golang-migrate）
```

---

## 六、配置管理

```
configs/
├── config.yaml     # 主配置（YAML 格式）
├── .env.dev        # 开发环境变量
└── .env.prod       # 生产环境变量
```

通过 `pkg/aikit/config` 加载，支持 YAML + 环境变量 + 热重载：

```go
loader.GetString("app.family", "go-template")
loader.GetInt("app.port", 8080)
loader.Scan("mysql", &mysqlCfg)
```

---

## 七、测试规范

### 测试分层

| 层级 | 范围 | 适用场景 |
|------|------|----------|
| 单元测试 | 单个函数/类，mock 依赖 | Service 逻辑、工具函数 |
| 集成测试 | DB + 完整链路 | DAO 操作、端到端接口 |

### 原则

- Service 层测试通过 interface mock DAO，不依赖真实 DB
- DAO 层测试使用 `pkg/aikit/database/mysql/txdb.go` 的事务回滚隔离（不污染数据）
- 每个测试只验证一个行为，命名描述预期：`TestGetArticle_NotFound`
- 使用 `testify` 断言

```go
func TestArticleService_Get_NotFound(t *testing.T) {
    repo := &mockRepo{}
    repo.On("GetByID", mock.Anything, uint(99)).Return(nil, gorm.ErrRecordNotFound)
    svc := NewArticleService(repo)
    _, err := svc.Get(context.Background(), 99)
    assert.ErrorAs(t, err, new(*apperrors.AppError))
}
```

---

## 八、注释风格

- 语言：中文，标点末尾 ASCII `.`
- 导出类型/函数必须有 godoc 注释
- 函数内注释只写 WHY，不写 WHAT

```go
// ArticleHandler 处理文章相关 HTTP 请求.
type ArticleHandler struct { ... }

// Get 按 ID 查询文章.
func (h *ArticleHandler) Get(c *gin.Context) { ... }
```
