# xstr — 字符串工具

常用字符串处理函数。

## 函数

```go
// MD5 哈希
hash := xstr.MD5("hello")

// 语义化版本比较（返回 -1, 0, 1）
cmp := xstr.CompareVersion("1.2.3", "1.3.0") // -1

// 获取真实 IP（支持 X-Forwarded-For, X-Real-IP）
ip := xstr.GetRealIP(r)
ip := xstr.GetRealIP(r, true) // 解析私有 IP
```
