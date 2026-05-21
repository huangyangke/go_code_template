package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/example/go-template/pkg/aikit/config"
	"github.com/example/go-template/pkg/aikit/utils/upload"
)

func main() {
	_, err := config.New("", config.WithEnvFile(".env"), config.WithOverrideEnv(true))
	if err != nil {
		fmt.Fprintf(os.Stderr, "load .env failed: %v\n", err)
		os.Exit(1)
	}

	addr := os.Getenv("UPLOAD_ADDR")
	if addr == "" {
		fmt.Fprintln(os.Stderr, "UPLOAD_ADDR not set, check .env file")
		os.Exit(1)
	}

	useCDN, _ := strconv.Atoi(os.Getenv("UPLOAD_USE_CDN"))
	u := upload.New(&upload.Config{
		Addr:     addr,
		PlatName: os.Getenv("UPLOAD_PLAT_NAME"),
		PlatKey:  os.Getenv("UPLOAD_PLAT_KEY"),
		Uploader: os.Getenv("UPLOAD_UPLOADER"),
		UseCDN:   useCDN,
	})

	// 上传文件：优先用命令行参数指定的文件路径，否则上传一段测试文本
	var res *upload.UploadResult
	if len(os.Args) > 1 {
		res, err = u.Upload(context.Background(), os.Args[1], nil, "")
	} else {
		res, err = u.Upload(context.Background(), "", []byte("hello from upload example"), "example.txt")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "upload failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("FileID: %d\nURL:    %s\n", res.FileID, res.URL)
}
