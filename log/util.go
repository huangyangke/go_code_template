package log

import (
	"math"
	"time"

	"github.com/huangyangke/go-aikit/log/internal/core"
)

func toMap(args ...D) map[string]interface{} {
	d := make(map[string]interface{}, 10+len(args))
	for _, arg := range args {
		switch arg.Type {
		case core.UintType, core.Uint64Type, core.IntType, core.Int64Type:
			d[arg.Key] = arg.Int64Val
		case core.StringType:
			d[arg.Key] = arg.StringVal
		case core.Float32Type:
			d[arg.Key] = math.Float32frombits(uint32(arg.Int64Val))
		case core.Float64Type:
			d[arg.Key] = math.Float64frombits(uint64(arg.Int64Val))
		case core.DurationType:
			d[arg.Key] = time.Duration(arg.Int64Val)
		default:
			d[arg.Key] = arg.Value
		}
	}
	return d
}
