// Package version exposes build-time metadata injected via -ldflags.
//
// Typical Makefile usage:
//
//	LDFLAGS = -X github.com/example/go-template/pkg/aikit/version.Version=$(VERSION) \
//	          -X github.com/example/go-template/pkg/aikit/version.Branch=$(BRANCH)   \
//	          -X github.com/example/go-template/pkg/aikit/version.Commit=$(COMMIT)   \
//	          -X github.com/example/go-template/pkg/aikit/version.Date=$(DATE)
//
//	go build -ldflags "$(LDFLAGS)" ./...
package version

import "fmt"

// These variables are set at compile time via -ldflags.
var (
	Version = "unknown"
	Branch  = "unknown"
	Commit  = "unknown"
	Date    = "unknown"
)

// Info returns a map of all build metadata fields.
func Info() map[string]string {
	return map[string]string{
		"version": Version,
		"branch":  Branch,
		"commit":  Commit,
		"date":    Date,
	}
}

// String returns all build metadata as a single-line string.
func String() string {
	return fmt.Sprintf("version=%s branch=%s commit=%s date=%s",
		Version, Branch, Commit, Date)
}
