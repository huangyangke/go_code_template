# go-template

基于内置 aikit（`pkg/aikit/`）的 Go 后端服务项目模板。aikit 已随仓库内置，无外部模块依赖，可按需直接修改。

## 开发新功能

参考 `internal/api/article.go` 示例，按以下步骤：

1. 在 `internal/model/` 新增数据库模型
2. 在 `cmd/migrate/migrations/` 新增迁移文件，执行 `./run.sh migrate`
3. 在 `internal/dao/` 新增 DAO（实现 service 层接口）及对应测试
4. 在 `internal/service/` 新增 Service（定义接口 + 实现）及对应测试
5. 在 `internal/schema/` 新增请求/响应 DTO
6. 在 `internal/api/` 新增 Handler
7. 在 `internal/api/router.go` 注册路由