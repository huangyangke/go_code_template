package xjob

import (
	"errors"
	"net"
)

// Config defines XXL-Job executor configuration.
type Config struct {
	Family       string            `yaml:"family"`        // Executor name (auto-filled by FastApp)
	ServerAddr   string            `yaml:"server_addr"`   // XXL-Job admin address (required)
	AccessToken  string            `yaml:"access_token"`  // Request token
	ExecutorIp   string            `yaml:"executor_ip"`   // Local IP (auto-detected)
	ExecutorPort string            `yaml:"executor_port"` // Local port, default "9999"
	LogDir       string            `yaml:"log_dir"`       // Job log directory, default "logs/xjob"
	MaxAge       int               `yaml:"max_age"`       // Log retention days, default 7
	JobDisabled  bool              `yaml:"job_disabled"`  // Disable job execution
	Extra        map[string]string `yaml:"extra"`         // Extra config
}

// Fix fills default values for zero/empty fields.
func (c *Config) Fix() {
	if c.ExecutorIp == "" {
		c.ExecutorIp = getLocalIP()
	}
	if c.ExecutorPort == "" {
		c.ExecutorPort = "9999"
	}
	if c.MaxAge <= 0 {
		c.MaxAge = 7
	}
	if c.LogDir == "" {
		c.LogDir = "logs/xjob"
	}
}

// Validate checks required fields.
func (c *Config) Validate() error {
	if c.ServerAddr == "" {
		return errors.New("xjob: server_addr is required")
	}
	if c.Family == "" {
		return errors.New("xjob: family is required")
	}
	return nil
}

// GetExtra returns an extra config value by key.
func (c *Config) GetExtra(key string) string {
	if c == nil || c.Extra == nil {
		return ""
	}
	return c.Extra[key]
}

// getLocalIP returns the first non-loopback IPv4 address.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return "127.0.0.1"
}
