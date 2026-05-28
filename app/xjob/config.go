package xjob

import (
	"errors"
	"net"
)

// Config XXL-Job 执行器配置.
type Config struct {
	Family       string            `yaml:"family"`        // 执行器名称（由 FastApp 自动填充）
	ServerAddr   string            `yaml:"server_addr"`   // XXL-Job 调度中心地址（必填）
	AccessToken  string            `yaml:"access_token"`  // 请求令牌
	ExecutorIp   string            `yaml:"executor_ip"`   // 本机 IP（自动检测）
	ExecutorPort string            `yaml:"executor_port"` // 本机端口，缺省 "9999"
	LogDir       string            `yaml:"log_dir"`       // 任务日志目录，缺省 "logs/xjob"
	MaxAge       int               `yaml:"max_age"`       // 日志保留天数，缺省 7
	JobDisabled  bool              `yaml:"job_disabled"`  // 禁用任务执行
	Extra        map[string]string `yaml:"extra"`         // 扩展配置
}

// Fix 填充零值/空值字段的默认值.
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

// Validate 校验必填字段.
// 返回值：err - 字段缺失时的错误.
func (c *Config) Validate() error {
	if c.ServerAddr == "" {
		return errors.New("xjob: server_addr is required")
	}
	if c.Family == "" {
		return errors.New("xjob: family is required")
	}
	return nil
}

// GetExtra 返回扩展配置中指定键的值.
// 参数：key - 配置键.
// 返回值：value - 配置值，键不存在或 Config 为 nil 时返回空字符串.
func (c *Config) GetExtra(key string) string {
	if c == nil || c.Extra == nil {
		return ""
	}
	return c.Extra[key]
}

// getLocalIP 返回首个非回环 IPv4 地址.
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
