# upload — COS 文件上传

腾讯云 COS 文件上传，支持 CRC64 校验。

## 用法

```go
uploader := upload.New(&upload.Config{
    Addr:     "https://upload.example.com",
    PlatName: "my-plat",
    PlatKey:  "secret-key",
})

result, err := uploader.Upload(ctx, "path/to/file.jpg", fileBytes, "file.jpg")
// result.FileID, result.URL

url, err := uploader.GetURL(ctx, fileID)
err = uploader.SetACL(ctx, fileID, "public-read")
```

## 工具函数

```go
crc := upload.CalcCRC64(data)     // CRC64 校验值
ext := upload.DetectExt(data)     // 根据文件头检测扩展名
```
