package log

import "strings"

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

func (l Level) String() string {
	return levelNames[l]
}

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
