# version — 构建版本信息

通过 `-ldflags` 注入构建版本信息。

## 用法

```go
fmt.Println(version.String())
// my-service v1.0.0 (branch: main, commit: abc1234, date: 2026-01-01)

info := version.Info()
// map[version:v1.0.0 branch:main commit:abc1234 date:2026-01-01]
```

## 构建注入

```bash
go build -ldflags "\
  -X github.com/huangyangke/go-aikit/version.Version=v1.0.0 \
  -X github.com/huangyangke/go-aikit/version.Branch=$(git rev-parse --abbrev-ref HEAD) \
  -X github.com/huangyangke/go-aikit/version.Commit=$(git rev-parse --short HEAD) \
  -X github.com/huangyangke/go-aikit/version.Date=$(date -u +%Y-%m-%d)" \
  ./cmd/myapp
```
