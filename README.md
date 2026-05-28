# go-template

基于内置 aikit（`pkg/aikit/`）的 Go 后端服务项目模板。aikit 已随仓库内置，无外部模块依赖，可按需直接修改。

## 快速开始

```bash
# 1. 克隆并重命名模块
NEW=github.com/your-org/your-project
grep -rl "github.com/example/go-template" --include="*.go" . | xargs sed -i "s|github.com/example/go-template|$NEW|g"
sed -i "s|^module github.com/example/go-template|module $NEW|" go.mod

# 2. 配置环境变量
cp configs/.env.dev configs/.env.dev.local
# 编辑 .env.dev 填入 MYSQL_DSN, REDIS_ADDR 等

# 3. 启动服务
./run.sh start

# 4. 访问 API
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/swagger/index.html
```

## 命令

```bash
./run.sh install    # 检查 Go 环境并下载依赖
./run.sh start      # 编译并启动服务
./run.sh stop       # 停止服务
./run.sh restart    # 重启服务
./run.sh build      # 仅编译二进制
./run.sh test       # 运行测试
./run.sh lint       # 代码检查
./run.sh swagger    # 生成 Swagger 文档
./run.sh status     # 查看服务状态
```

## 项目结构

```
.
├── cmd/server/main.go          # 应用入口
├── internal/
│   ├── api/                    # HTTP 层（路由绑定、参数解析、响应）
│   │   ├── router.go           # 路由注册
│   │   └── article.go          # 示例 handler
│   ├── service/                # 业务逻辑层
│   │   └── article.go
│   ├── dao/                    # 数据访问层（GORM 操作）
│   │   └── article.go
│   ├── model/                  # 数据库模型
│   │   └── article.go
│   └── schema/                 # 请求/响应 DTO
│       └── article.go
├── pkg/
│   └── aikit/                  # 内置工具包（FastApp、config、log、cache、metrics 等）
├── configs/
│   ├── config.yaml             # 主配置文件
│   ├── .env.dev                # 开发环境变量
│   └── .env.prod               # 生产环境变量
├── docs/                       # Swagger 生成文档（./run.sh swagger）
├── Dockerfile
├── run.sh
└── CLAUDE.md
```

## 开发新功能

参考 `internal/api/article.go` 示例，按以下步骤：

1. 在 `internal/model/` 新增数据库模型
2. 在 `internal/dao/` 新增 DAO（实现 service 层接口）
3. 在 `internal/service/` 新增 Service（定义接口 + 实现）
4. 在 `internal/schema/` 新增请求/响应 DTO
5. 在 `internal/api/` 新增 Handler
6. 在 `internal/api/router.go` 注册路由
