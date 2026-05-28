package log

import "strings"

// Level 日志级别.
type Level int8

const (
	_debugLevel Level = iota - 1
	_infoLevel
	_warnLevel
	_errorLevel
	_fatalLevel
)

var levelNames = map[Level]string{
	_debugLevel: "DEBUG",
	_infoLevel:  "INFO",
	_warnLevel:  "WARN",
	_errorLevel: "ERROR",
	_fatalLevel: "FATAL",
}

// String 返回级别的字符串表示.
// 返回值：级别名称 (DEBUG/INFO/WARN/ERROR/FATAL).
func (l Level) String() string {
	return levelNames[l]
}

// ParseLevel 将字符串解析为日志级别.
// 参数：s - 级别字符串 (DEBUG/INFO/WARN/ERROR/FATAL), defaultLevel - 解析失败时的默认级别.
// 返回值：解析后的日志级别.
func ParseLevel(s string, defaultLevel ...Level) Level {
	defLv := _infoLevel
	if len(defaultLevel) > 0 {
		defLv = defaultLevel[0]
	}
	switch strings.ToUpper(s) {
	case "DEBUG":
		return _debugLevel
	case "INFO":
		return _infoLevel
	case "WARN":
		return _warnLevel
	case "ERROR":
		return _errorLevel
	case "FATAL":
		return _fatalLevel
	}
	return defLv
}
