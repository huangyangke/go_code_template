# config — 配置加载

支持 YAML 文件、环境变量、Nacos 远程配置、热重载。

## 用法

```go
// 基础加载
loader, err := config.New("config.yaml")

// 多文件 + 环境变量
loader, err := config.NewFromPaths([]string{"base.yaml", "local.yaml"},
    config.WithEnvFile(".env"),
    config.WithOverrideEnv(true),
    config.WithEnableSubstitution(true),
)

// 读取配置
name := loader.GetString("app.name", "default")
port := loader.GetInt("app.port", 8080)
debug := loader.GetBool("app.debug", false)

// 结构化绑定
var cfg MyConfig
loader.Scan("database", &cfg)

// 热重载
loader.Watch(func() {
    log.Info("config reloaded")
})
```

## Nacos 远程配置

```go
loader, err := config.New("config.yaml",
    config.WithNacosConfig(&config.NacosConfig{
        ServerAddr: "nacos:8848",
        Namespace:  "dev",
        ConfigList: []config.NacosConfigItem{
            {Group: "DEFAULT_GROUP", DataID: "my-service.yaml"},
        },
        AutoUpdate: true,
    }),
)
```

## 方法

| 方法 | 说明 |
|---|---|
| `Get(key, default...)` | 获取任意类型 |
| `GetString/GetInt/GetBool/GetFloat` | 类型化获取 |
| `GetStringSlice(key, default...)` | 获取字符串切片 |
| `GetMap(key, default...)` | 获取 map[string]interface{} |
| `GetDuration(key, default...)` | 获取 time.Duration（支持 "5s"、"100ms" 或整数秒） |
| `Scan(key, v)` | 绑定到结构体 |
| `Reload()` | 手动重载 |
| `Watch(callback)` | 文件变更监听 |
| `Raw()` | 获取原始 map |
| `Dump(redactKeys...)` | 导出配置（脱敏） |
