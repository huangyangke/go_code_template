# auth — JWT + OAuth2 认证

完整的认证体系：JWT 令牌签发/验证、OAuth2（Google/GitHub/自定义）、密码哈希策略、路由自动注册。

## 快速开始

```go
manager, _ := auth.NewFromService(userService, auth.Config{
    Secret:         "your-secret",
    AccessTokenTTL: 15 * time.Minute,
    RefreshTokenTTL: 7 * 24 * time.Hour,
    Prefix:         "/auth",
    EnableRefresh:  true,
    EnableLogout:   true,
    EnableMe:       true,
})

// 自动注册路由：POST /auth/login, /auth/refresh, /auth/logout, GET /auth/me
manager.RegisterRoutes(router)

// 路由保护
api := router.Group("/api", manager.AuthRequired())
api.GET("/admin", manager.RequireScopes("admin"), adminHandler)
```

## 自动注册的路由

| 路由 | 条件 |
|---|---|
| `POST /login` | 始终开启 |
| `POST /register` | `EnableRegister` 且 UserService 实现 `UserCreator` |
| `POST /refresh` | `EnableRefresh` |
| `POST /logout` | `EnableLogout` |
| `GET /me` | `EnableMe` |
| `GET /oauth/:provider/authorize` | `EnableOAuth` |
| `POST /oauth/:provider/callback` | `EnableOAuth` |

## OAuth2

内置 Google 和 GitHub provider 预设，也可自定义：

```go
auth.Config{
    EnableOAuth: true,
    Providers: map[string]auth.OAuthProviderConfig{
        "google": {ClientID: "...", ClientSecret: "..."},
        "github": {ClientID: "...", ClientSecret: "..."},
        "custom": {
            ClientID: "...", ClientSecret: "...",
            AuthorizeURL: "https://...", TokenURL: "https://...", UserinfoURL: "https://...",
        },
    },
    OAuthStateSecret: "state-secret",
}
```

## 密码策略

```go
policy := auth.DefaultPasswordPolicy() // 长度 8+，大小写+数字+特殊字符
failures := policy.Validate("weak")    // 返回不满足的规则列表

hasher := auth.BcryptHasher{Cost: 12}
hash, _ := hasher.Hash("password")
ok := hasher.Verify("password", hash)
```

## 接口

实现 `UserService` 接口接入你的用户存储：

```go
type UserService interface {
    GetUserByUsername(ctx context.Context, username string) (User, error)
    GetUserByID(ctx context.Context, uid string) (User, error)
}

// 可选：实现 UserCreator 自动启用注册功能
type UserCreator interface {
    CreateUser(ctx context.Context, username, password string, extra map[string]any) (User, error)
}
```
