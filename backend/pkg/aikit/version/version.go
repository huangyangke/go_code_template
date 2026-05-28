// Package version 构建时元数据，通过 -ldflags 注入.
//
// 典型 Makefile 用法:
//
//	LDFLAGS = -X github.com/huangyangke/go-aikit/version.Version=$(VERSION) \
//	          -X github.com/huangyangke/go-aikit/version.Branch=$(BRANCH)   \
//	          -X github.com/huangyangke/go-aikit/version.Commit=$(COMMIT)   \
//	          -X github.com/huangyangke/go-aikit/version.Date=$(DATE)
//
//	go build -ldflags "$(LDFLAGS)" ./...
package version

import "fmt"

var (
	// Version 版本号，编译时通过 -ldflags 注入.
	Version = "unknown"
	// Branch 分支名，编译时通过 -ldflags 注入.
	Branch = "unknown"
	// Commit 提交哈希，编译时通过 -ldflags 注入.
	Commit = "unknown"
	// Date 构建日期，编译时通过 -ldflags 注入.
	Date = "unknown"
)

// Info 返回全部构建元数据的键值映射.
// 返回值：map - 包含 version/branch/commit/date 的映射.
func Info() map[string]string {
	return map[string]string{
		"version": Version,
		"branch":  Branch,
		"commit":  Commit,
		"date":    Date,
	}
}

// String 返回全部构建元数据的单行字符串.
// 返回值：str - 格式为 "version=... branch=... commit=... date=...".
func String() string {
	return fmt.Sprintf("version=%s branch=%s commit=%s date=%s",
		Version, Branch, Commit, Date)
}
