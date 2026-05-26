package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Pipeliner = redis.Pipeliner
type Script = redis.Script
type Z = redis.Z

func (r *Redis) SetEx(ctx context.Context, key string, value interface{}, expire time.Duration) error {
	return r.client.SetEx(ctx, r.key(key), value, expire).Err()
}

func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, r.key(key)).Result()
}

func (r *Redis) GetNoNil(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, r.key(key)).Result()
	if err == redis.Nil {
		return val, nil
	}
	return val, err
}

func (r *Redis) MGet(ctx context.Context, keys ...string) ([]string, error) {
	vals, err := r.client.MGet(ctx, r.keys(keys...)...).Result()
	if err != nil {
		return nil, err
	}
	result := make([]string, len(vals))
	for i, v := range vals {
		if s, ok := v.(string); ok {
			result[i] = s
		}
	}
	return result, nil
}

func (r *Redis) Del(ctx context.Context, key string) (int, error) {
	v, err := r.client.Del(ctx, r.key(key)).Result()
	return int(v), err
}

func (r *Redis) Exists(ctx context.Context, key string) (bool, error) {
	v, err := r.client.Exists(ctx, r.key(key)).Result()
	return v == 1, err
}

func (r *Redis) Expire(ctx context.Context, key string, expire time.Duration) error {
	return r.client.Expire(ctx, r.key(key), expire).Err()
}

func (r *Redis) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, r.key(key)).Result()
}

func (r *Redis) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return r.client.IncrBy(ctx, r.key(key), n).Result()
}

func (r *Redis) SetNx(ctx context.Context, key string, value interface{}, expire time.Duration) (bool, error) {
	return r.client.SetNX(ctx, r.key(key), value, expire).Result()
}

func (r *Redis) HSet(ctx context.Context, key, field string, value interface{}) error {
	return r.client.HSet(ctx, r.key(key), field, value).Err()
}

func (r *Redis) HGet(ctx context.Context, key, field string) (string, error) {
	return r.client.HGet(ctx, r.key(key), field).Result()
}

func (r *Redis) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, r.key(key)).Result()
}

func (r *Redis) HDel(ctx context.Context, key string, fields ...string) (bool, error) {
	v, err := r.client.HDel(ctx, r.key(key), fields...).Result()
	return v > 0, err
}

func (r *Redis) Publish(ctx context.Context, channel string, message any) (int, error) {
	v, err := r.client.Publish(ctx, r.key(channel), message).Result()
	return int(v), err
}

func (r *Redis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	switch c := r.client.(type) {
	case *redis.Client:
		return c.Subscribe(ctx, r.keys(channels...)...)
	case *redis.ClusterClient:
		return c.Subscribe(ctx, r.keys(channels...)...)
	}
	panic("redis: Subscribe called on unsupported client type")
}

func (r *Redis) Pipeline() Pipeliner {
	return r.client.Pipeline()
}

func NewScript(script string) *Script {
	return redis.NewScript(script)
}

func (r *Redis) ScriptRun(ctx context.Context, script *Script, keys []string, args ...any) (any, error) {
	return script.Run(ctx, r.client, r.keys(keys...), args...).Result()
}

func (r *Redis) Ping(ctx context.Context) bool {
	v, err := r.client.Ping(ctx).Result()
	return err == nil && v == "PONG"
}

// --- String / Key commands ---

func (r *Redis) SetXx(ctx context.Context, key string, value interface{}, expire time.Duration) (bool, error) {
	return r.client.SetXX(ctx, r.key(key), value, expire).Result()
}

func (r *Redis) Decr(ctx context.Context, key string) (int64, error) {
	return r.client.Decr(ctx, r.key(key)).Result()
}

func (r *Redis) DecrBy(ctx context.Context, key string, decrement int64) (int64, error) {
	return r.client.DecrBy(ctx, r.key(key), decrement).Result()
}

// TTL returns the remaining time-to-live of a key as time.Duration.
// Special values from Redis are preserved:
//   -1 * time.Second: key exists but has no expiration
//   -2 * time.Second: key does not exist
func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, r.key(key)).Result()
}

// --- Hash commands ---

func (r *Redis) HGetNoNil(ctx context.Context, key, field string) (string, error) {
	val, err := r.client.HGet(ctx, r.key(key), field).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (r *Redis) HExists(ctx context.Context, key, field string) (bool, error) {
	return r.client.HExists(ctx, r.key(key), field).Result()
}

func (r *Redis) HSetNx(ctx context.Context, key, field string, value interface{}) (bool, error) {
	return r.client.HSetNX(ctx, r.key(key), field, value).Result()
}

func (r *Redis) HMSet(ctx context.Context, key string, fieldsAndValues map[string]interface{}) error {
	return r.client.HMSet(ctx, r.key(key), fieldsAndValues).Err()
}

func (r *Redis) HMGet(ctx context.Context, key string, fields ...string) ([]string, error) {
	vals, err := r.client.HMGet(ctx, r.key(key), fields...).Result()
	if err != nil {
		return nil, err
	}
	return toStrings(vals), nil
}

func (r *Redis) HVals(ctx context.Context, key string) ([]string, error) {
	return r.client.HVals(ctx, r.key(key)).Result()
}

func (r *Redis) HKeys(ctx context.Context, key string) ([]string, error) {
	return r.client.HKeys(ctx, r.key(key)).Result()
}

func (r *Redis) HLen(ctx context.Context, key string) (int, error) {
	v, err := r.client.HLen(ctx, r.key(key)).Result()
	return int(v), err
}

func (r *Redis) HIncrBy(ctx context.Context, key, field string, incr int) (int, error) {
	v, err := r.client.HIncrBy(ctx, r.key(key), field, int64(incr)).Result()
	return int(v), err
}

// --- List commands ---

func (r *Redis) LPush(ctx context.Context, key string, values ...interface{}) (int, error) {
	v, err := r.client.LPush(ctx, r.key(key), values...).Result()
	return int(v), err
}

func (r *Redis) RPush(ctx context.Context, key string, values ...interface{}) (int, error) {
	v, err := r.client.RPush(ctx, r.key(key), values...).Result()
	return int(v), err
}

func (r *Redis) LPop(ctx context.Context, key string) (string, error) {
	return r.client.LPop(ctx, r.key(key)).Result()
}

func (r *Redis) RPop(ctx context.Context, key string) (string, error) {
	return r.client.RPop(ctx, r.key(key)).Result()
}

func (r *Redis) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, r.key(key), start, stop).Result()
}

func (r *Redis) LLen(ctx context.Context, key string) (int, error) {
	v, err := r.client.LLen(ctx, r.key(key)).Result()
	return int(v), err
}

func (r *Redis) LRem(ctx context.Context, key string, count int64, value interface{}) (int, error) {
	v, err := r.client.LRem(ctx, r.key(key), count, value).Result()
	return int(v), err
}

func (r *Redis) LIndex(ctx context.Context, key string, idx int64) (string, error) {
	return r.client.LIndex(ctx, r.key(key), idx).Result()
}

func (r *Redis) LTrim(ctx context.Context, key string, start, stop int64) error {
	return r.client.LTrim(ctx, r.key(key), start, stop).Err()
}

// --- Set commands ---

func (r *Redis) SAdd(ctx context.Context, key string, members ...interface{}) (int, error) {
	v, err := r.client.SAdd(ctx, r.key(key), members...).Result()
	return int(v), err
}

func (r *Redis) SCard(ctx context.Context, key string) (int, error) {
	v, err := r.client.SCard(ctx, r.key(key)).Result()
	return int(v), err
}

func (r *Redis) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.client.SMembers(ctx, r.key(key)).Result()
}

func (r *Redis) SRem(ctx context.Context, key string, members ...interface{}) (int, error) {
	v, err := r.client.SRem(ctx, r.key(key), members...).Result()
	return int(v), err
}

func (r *Redis) SPop(ctx context.Context, key string) (string, error) {
	return r.client.SPop(ctx, r.key(key)).Result()
}

func (r *Redis) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.client.SIsMember(ctx, r.key(key), member).Result()
}

// --- Sorted Set commands ---

func (r *Redis) ZAdd(ctx context.Context, key string, score float64, member string) (bool, error) {
	v, err := r.client.ZAdd(ctx, r.key(key), redis.Z{Score: score, Member: member}).Result()
	return v == 1, err
}

func (r *Redis) ZAdds(ctx context.Context, key string, members ...redis.Z) (int, error) {
	v, err := r.client.ZAdd(ctx, r.key(key), members...).Result()
	return int(v), err
}

func (r *Redis) ZScore(ctx context.Context, key, member string) (float64, error) {
	return r.client.ZScore(ctx, r.key(key), member).Result()
}

func (r *Redis) ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error) {
	return r.client.ZIncrBy(ctx, r.key(key), increment, member).Result()
}

func (r *Redis) ZRank(ctx context.Context, key, member string) (int64, error) {
	return r.client.ZRank(ctx, r.key(key), member).Result()
}

func (r *Redis) ZRevRank(ctx context.Context, key, member string) (int64, error) {
	return r.client.ZRevRank(ctx, r.key(key), member).Result()
}

func (r *Redis) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRange(ctx, r.key(key), start, stop).Result()
}

func (r *Redis) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRevRange(ctx, r.key(key), start, stop).Result()
}

func (r *Redis) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.client.ZRangeWithScores(ctx, r.key(key), start, stop).Result()
}

func (r *Redis) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.client.ZRevRangeWithScores(ctx, r.key(key), start, stop).Result()
}

// ZRangeByScoreWithScores returns members with scores between min and max (inclusive).
func (r *Redis) ZRangeByScoreWithScores(ctx context.Context, key string, min, max float64) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: formatScore(min), Max: formatScore(max)}
	return r.client.ZRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

// ZRangeByScoreWithScoresRaw accepts Redis-style score strings for -inf/+inf/(score.
func (r *Redis) ZRangeByScoreWithScoresRaw(ctx context.Context, key, min, max string) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: min, Max: max}
	return r.client.ZRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

func (r *Redis) ZRevRangeByScore(ctx context.Context, key string, min, max float64) ([]string, error) {
	opt := &redis.ZRangeBy{Min: formatScore(min), Max: formatScore(max)}
	return r.client.ZRevRangeByScore(ctx, r.key(key), opt).Result()
}

func (r *Redis) ZRevRangeByScoreRaw(ctx context.Context, key, min, max string) ([]string, error) {
	opt := &redis.ZRangeBy{Min: min, Max: max}
	return r.client.ZRevRangeByScore(ctx, r.key(key), opt).Result()
}

func (r *Redis) ZRevRangeByScoreWithScores(ctx context.Context, key string, min, max float64) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: formatScore(min), Max: formatScore(max)}
	return r.client.ZRevRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

func (r *Redis) ZRevRangeByScoreWithScoresRaw(ctx context.Context, key, min, max string) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: min, Max: max}
	return r.client.ZRevRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

func (r *Redis) ZCount(ctx context.Context, key string, min, max float64) (int, error) {
	v, err := r.client.ZCount(ctx, r.key(key), formatScore(min), formatScore(max)).Result()
	return int(v), err
}

func (r *Redis) ZCountRaw(ctx context.Context, key, min, max string) (int, error) {
	v, err := r.client.ZCount(ctx, r.key(key), min, max).Result()
	return int(v), err
}

func (r *Redis) ZCard(ctx context.Context, key string) (int, error) {
	v, err := r.client.ZCard(ctx, r.key(key)).Result()
	return int(v), err
}

func (r *Redis) ZRem(ctx context.Context, key string, members ...interface{}) (int, error) {
	v, err := r.client.ZRem(ctx, r.key(key), members...).Result()
	return int(v), err
}

func (r *Redis) ZRemRangeByScore(ctx context.Context, key string, min, max float64) (int, error) {
	v, err := r.client.ZRemRangeByScore(ctx, r.key(key), formatScore(min), formatScore(max)).Result()
	return int(v), err
}

func (r *Redis) ZRemRangeByScoreRaw(ctx context.Context, key, min, max string) (int, error) {
	v, err := r.client.ZRemRangeByScore(ctx, r.key(key), min, max).Result()
	return int(v), err
}

func (r *Redis) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) (int, error) {
	v, err := r.client.ZRemRangeByRank(ctx, r.key(key), start, stop).Result()
	return int(v), err
}

// --- helpers ---

const (
	MinScore = "-inf"
	MaxScore = "+inf"
)

// ExclusiveScore renders a float64 score as an exclusive Redis ZSet range bound,
// e.g. ExclusiveScore(100) == "(100". Use with *Raw methods.
func ExclusiveScore(score float64) string {
	return "(" + formatScore(score)
}

func formatScore(score float64) string {
	return strconv.FormatFloat(score, 'f', -1, 64)
}

func toStrings(vals []interface{}) []string {
	ret := make([]string, len(vals))
	for i, v := range vals {
		if v != nil {
			ret[i], _ = v.(string)
		}
	}
	return ret
}

