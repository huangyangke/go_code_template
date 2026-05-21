package log

import (
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// parseDSN parses log agent dsn.
// udp://host:port?chan=1024&timeout=100ms
func parseDSN(rawdsn string) *AgentConfig {
	u, err := url.Parse(rawdsn)
	if err != nil {
		panic(fmt.Sprintf("log: invalid dsn: %s: %v", rawdsn, err))
	}

	values := u.Query()
	chanSize, _ := strconv.Atoi(values.Get("chan"))

	var timeout time.Duration
	if ts := values.Get("timeout"); ts != "" {
		timeout, _ = time.ParseDuration(ts)
	}

	priority, _ := strconv.Atoi(values.Get("priority"))
	return &AgentConfig{
		Proto:    u.Scheme,
		Addr:     u.Host,
		Chan:     chanSize,
		Timeout:  timeout,
		Priority: priority,
	}
}
