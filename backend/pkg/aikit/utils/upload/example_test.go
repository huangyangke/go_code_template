package upload_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/example/go-template/pkg/aikit/config"
	"github.com/example/go-template/pkg/aikit/utils/upload"
)

func TestUpload_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, err := config.New("", config.WithEnvFile("../../.env"), config.WithOverrideEnv(true))
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}

	addr := os.Getenv("UPLOAD_ADDR")
	if addr == "" {
		t.Skip("UPLOAD_ADDR not set, skipping integration test")
	}

	useCDN, _ := strconv.Atoi(os.Getenv("UPLOAD_USE_CDN"))
	cfg := &upload.Config{
		Addr:     addr,
		PlatName: os.Getenv("UPLOAD_PLAT_NAME"),
		PlatKey:  os.Getenv("UPLOAD_PLAT_KEY"),
		Uploader: os.Getenv("UPLOAD_UPLOADER"),
		UseCDN:   useCDN,
	}
	u := upload.New(cfg)

	res, err := u.Upload(context.Background(), "", []byte("hello from upload integration test"), "test.txt")
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if res.FileID == 0 {
		t.Error("expected non-zero FileID")
	}
	if res.URL == "" {
		t.Error("expected non-empty URL")
	}

	fmt.Printf("upload ok: id=%d, url=%s\n", res.FileID, res.URL)
}
