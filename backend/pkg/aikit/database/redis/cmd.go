package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Pipeliner Redis 管道接口别名.
type Pipeliner = redis.Pipeliner

// Script Redis Lua 脚本封装.
type Script = redis.Script

// Z Redis 有序集合元素（分数 + 成员）.
type Z = redis.Z

// SetEx 设置带过期时间的键值.
// 参数：ctx - 上下文, key - 键名, value - 值, expire - 过期时间.
// 返回值：err - 操作失败时的错误.
func (r *Redis) SetEx(ctx context.Context, key string, value interface{}, expire time.Duration) error {
	return r.client.SetEx(ctx, r.key(key), value, expire).Err()
}

// Get 获取键的字符串值.
// 参数：ctx - 上下文, key - 键名.
// 返回值：val - 键值, err - 键不存在或操作失败时的错误.
func (r *Redis) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, r.key(key)).Result()
}

// GetNoNil 获取键值，键不存在时返回空字符串而非错误.
// 参数：ctx - 上下文, key - 键名.
// 返回值：val - 键值, err - 非 Nil 错误.
func (r *Redis) GetNoNil(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, r.key(key)).Result()
	if err == redis.Nil {
		return val, nil
	}
	return val, err
}

// MGet 批量获取多个键的字符串值.
// 参数：ctx - 上下文, keys - 键名列表.
// 返回值：vals - 键值列表, err - 操作失败时的错误.
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

// Del 删除键.
// 参数：ctx - 上下文, key - 键名.
// 返回值：n - 删除的键数量, err - 操作失败时的错误.
func (r *Redis) Del(ctx context.Context, key string) (int, error) {
	v, err := r.client.Del(ctx, r.key(key)).Result()
	return int(v), err
}

// Exists 检查键是否存在.
// 参数：ctx - 上下文, key - 键名.
// 返回值：ok - 是否存在, err - 操作失败时的错误.
func (r *Redis) Exists(ctx context.Context, key string) (bool, error) {
	v, err := r.client.Exists(ctx, r.key(key)).Result()
	return v == 1, err
}

// Expire 设置键的过期时间.
// 参数：ctx - 上下文, key - 键名, expire - 过期时间.
// 返回值：err - 操作失败时的错误.
func (r *Redis) Expire(ctx context.Context, key string, expire time.Duration) error {
	return r.client.Expire(ctx, r.key(key), expire).Err()
}

// Incr 对键自增 1.
// 参数：ctx - 上下文, key - 键名.
// 返回值：n - 自增后的值, err - 操作失败时的错误.
func (r *Redis) Incr(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, r.key(key)).Result()
}

// IncrBy 对键自增指定数值.
// 参数：ctx - 上下文, key - 键名, n - 自增量.
// 返回值：n - 自增后的值, err - 操作失败时的错误.
func (r *Redis) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return r.client.IncrBy(ctx, r.key(key), n).Result()
}

// SetNx 仅当键不存在时设置值.
// 参数：ctx - 上下文, key - 键名, value - 值, expire - 过期时间.
// 返回值：ok - 是否设置成功, err - 操作失败时的错误.
func (r *Redis) SetNx(ctx context.Context, key string, value interface{}, expire time.Duration) (bool, error) {
	return r.client.SetNX(ctx, r.key(key), value, expire).Result()
}

// HSet 设置哈希字段值.
// 参数：ctx - 上下文, key - 键名, field - 字段名, value - 值.
// 返回值：err - 操作失败时的错误.
func (r *Redis) HSet(ctx context.Context, key, field string, value interface{}) error {
	return r.client.HSet(ctx, r.key(key), field, value).Err()
}

// HGet 获取哈希字段值.
// 参数：ctx - 上下文, key - 键名, field - 字段名.
// 返回值：val - 字段值, err - 字段不存在或操作失败时的错误.
func (r *Redis) HGet(ctx context.Context, key, field string) (string, error) {
	return r.client.HGet(ctx, r.key(key), field).Result()
}

// HGetAll 获取哈希所有字段与值.
// 参数：ctx - 上下文, key - 键名.
// 返回值：m - 字段值映射, err - 操作失败时的错误.
func (r *Redis) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, r.key(key)).Result()
}

// HDel 删除哈希字段.
// 参数：ctx - 上下文, key - 键名, fields - 字段名列表.
// 返回值：ok - 是否有字段被删除, err - 操作失败时的错误.
func (r *Redis) HDel(ctx context.Context, key string, fields ...string) (bool, error) {
	v, err := r.client.HDel(ctx, r.key(key), fields...).Result()
	return v > 0, err
}

// Publish 向频道发布消息.
// 参数：ctx - 上下文, channel - 频道名, message - 消息内容.
// 返回值：n - 收到消息的订阅者数量, err - 操作失败时的错误.
func (r *Redis) Publish(ctx context.Context, channel string, message any) (int, error) {
	v, err := r.client.Publish(ctx, r.key(channel), message).Result()
	return int(v), err
}

// Subscribe 订阅频道.
// 参数：ctx - 上下文, channels - 频道名列表.
// 返回值：*redis.PubSub - 订阅连接.
func (r *Redis) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	switch c := r.client.(type) {
	case *redis.Client:
		return c.Subscribe(ctx, r.keys(channels...)...)
	case *redis.ClusterClient:
		return c.Subscribe(ctx, r.keys(channels...)...)
	}
	panic("redis: Subscribe called on unsupported client type")
}

// Pipeline 创建 Redis 管道.
// 返回值：Pipeliner - 管道接口.
func (r *Redis) Pipeline() Pipeliner {
	return r.client.Pipeline()
}

// NewScript 创建 Lua 脚本封装.
// 参数：script - Lua 脚本内容.
// 返回值：*Script - 脚本实例.
func NewScript(script string) *Script {
	return redis.NewScript(script)
}

// ScriptRun 执行 Lua 脚本.
// 参数：ctx - 上下文, script - 脚本实例, keys - 键名列表, args - 附加参数.
// 返回值：any - 脚本返回值, err - 执行失败时的错误.
func (r *Redis) ScriptRun(ctx context.Context, script *Script, keys []string, args ...any) (any, error) {
	return script.Run(ctx, r.client, r.keys(keys...), args...).Result()
}

// Ping 检测 Redis 连通性.
// 参数：ctx - 上下文.
// 返回值：ok - 连接是否正常.
func (r *Redis) Ping(ctx context.Context) bool {
	v, err := r.client.Ping(ctx).Result()
	return err == nil && v == "PONG"
}

// --- String / Key commands ---.

// SetXx 仅当键已存在时设置值.
// 参数：ctx - 上下文, key - 键名, value - 值, expire - 过期时间.
// 返回值：ok - 是否设置成功, err - 操作失败时的错误.
func (r *Redis) SetXx(ctx context.Context, key string, value interface{}, expire time.Duration) (bool, error) {
	return r.client.SetXX(ctx, r.key(key), value, expire).Result()
}

// Decr 对键自减 1.
// 参数：ctx - 上下文, key - 键名.
// 返回值：n - 自减后的值, err - 操作失败时的错误.
func (r *Redis) Decr(ctx context.Context, key string) (int64, error) {
	return r.client.Decr(ctx, r.key(key)).Result()
}

// DecrBy 对键自减指定数值.
// 参数：ctx - 上下文, key - 键名, decrement - 自减量.
// 返回值：n - 自减后的值, err - 操作失败时的错误.
func (r *Redis) DecrBy(ctx context.Context, key string, decrement int64) (int64, error) {
	return r.client.DecrBy(ctx, r.key(key), decrement).Result()
}

// TTL 返回键的剩余过期时间.
// 参数：ctx - 上下文, key - 键名.
// 返回值：ttl - 剩余时间, err - 操作失败时的错误.
// 特殊值：-1 表示无过期，-2 表示键不存在.
func (r *Redis) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, r.key(key)).Result()
}

// --- Hash commands ---.

// HGetNoNil 获取哈希字段值，字段不存在时返回空字符串而非错误.
// 参数：ctx - 上下文, key - 键名, field - 字段名.
// 返回值：val - 字段值, err - 非 Nil 错误.
func (r *Redis) HGetNoNil(ctx context.Context, key, field string) (string, error) {
	val, err := r.client.HGet(ctx, r.key(key), field).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// HExists 检查哈希字段是否存在.
// 参数：ctx - 上下文, key - 键名, field - 字段名.
// 返回值：ok - 是否存在, err - 操作失败时的错误.
func (r *Redis) HExists(ctx context.Context, key, field string) (bool, error) {
	return r.client.HExists(ctx, r.key(key), field).Result()
}

// HSetNx 仅当哈希字段不存在时设置值.
// 参数：ctx - 上下文, key - 键名, field - 字段名, value - 值.
// 返回值：ok - 是否设置成功, err - 操作失败时的错误.
func (r *Redis) HSetNx(ctx context.Context, key, field string, value interface{}) (bool, error) {
	return r.client.HSetNX(ctx, r.key(key), field, value).Result()
}

// HMSet 批量设置哈希字段值.
// 参数：ctx - 上下文, key - 键名, fieldsAndValues - 字段值映射.
// 返回值：err - 操作失败时的错误.
func (r *Redis) HMSet(ctx context.Context, key string, fieldsAndValues map[string]interface{}) error {
	return r.client.HMSet(ctx, r.key(key), fieldsAndValues).Err()
}

// HMGet 批量获取哈希字段值.
// 参数：ctx - 上下文, key - 键名, fields - 字段名列表.
// 返回值：vals - 字段值列表, err - 操作失败时的错误.
func (r *Redis) HMGet(ctx context.Context, key string, fields ...string) ([]string, error) {
	vals, err := r.client.HMGet(ctx, r.key(key), fields...).Result()
	if err != nil {
		return nil, err
	}
	return toStrings(vals), nil
}

// HVals 获取哈希所有字段值.
// 参数：ctx - 上下文, key - 键名.
// 返回值：vals - 字段值列表, err - 操作失败时的错误.
func (r *Redis) HVals(ctx context.Context, key string) ([]string, error) {
	return r.client.HVals(ctx, r.key(key)).Result()
}

// HKeys 获取哈希所有字段名.
// 参数：ctx - 上下文, key - 键名.
// 返回值：keys - 字段名列表, err - 操作失败时的错误.
func (r *Redis) HKeys(ctx context.Context, key string) ([]string, error) {
	return r.client.HKeys(ctx, r.key(key)).Result()
}

// HLen 获取哈希字段数量.
// 参数：ctx - 上下文, key - 键名.
// 返回值：n - 字段数量, err - 操作失败时的错误.
func (r *Redis) HLen(ctx context.Context, key string) (int, error) {
	v, err := r.client.HLen(ctx, r.key(key)).Result()
	return int(v), err
}

// HIncrBy 对哈希字段自增指定整数.
// 参数：ctx - 上下文, key - 键名, field - 字段名, incr - 自增量.
// 返回值：n - 自增后的值, err - 操作失败时的错误.
func (r *Redis) HIncrBy(ctx context.Context, key, field string, incr int) (int, error) {
	v, err := r.client.HIncrBy(ctx, r.key(key), field, int64(incr)).Result()
	return int(v), err
}

// --- List commands ---.

// LPush 从列表左端插入元素.
// 参数：ctx - 上下文, key - 键名, values - 元素列表.
// 返回值：n - 列表长度, err - 操作失败时的错误.
func (r *Redis) LPush(ctx context.Context, key string, values ...interface{}) (int, error) {
	v, err := r.client.LPush(ctx, r.key(key), values...).Result()
	return int(v), err
}

// RPush 从列表右端插入元素.
// 参数：ctx - 上下文, key - 键名, values - 元素列表.
// 返回值：n - 列表长度, err - 操作失败时的错误.
func (r *Redis) RPush(ctx context.Context, key string, values ...interface{}) (int, error) {
	v, err := r.client.RPush(ctx, r.key(key), values...).Result()
	return int(v), err
}

// LPop 从列表左端弹出元素.
// 参数：ctx - 上下文, key - 键名.
// 返回值：val - 弹出的元素, err - 列表为空或操作失败时的错误.
func (r *Redis) LPop(ctx context.Context, key string) (string, error) {
	return r.client.LPop(ctx, r.key(key)).Result()
}

// RPop 从列表右端弹出元素.
// 参数：ctx - 上下文, key - 键名.
// 返回值：val - 弹出的元素, err - 列表为空或操作失败时的错误.
func (r *Redis) RPop(ctx context.Context, key string) (string, error) {
	return r.client.RPop(ctx, r.key(key)).Result()
}

// LRange 获取列表指定范围的元素.
// 参数：ctx - 上下文, key - 键名, start - 起始索引, stop - 结束索引.
// 返回值：vals - 元素列表, err - 操作失败时的错误.
func (r *Redis) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.LRange(ctx, r.key(key), start, stop).Result()
}

// LLen 获取列表长度.
// 参数：ctx - 上下文, key - 键名.
// 返回值：n - 列表长度, err - 操作失败时的错误.
func (r *Redis) LLen(ctx context.Context, key string) (int, error) {
	v, err := r.client.LLen(ctx, r.key(key)).Result()
	return int(v), err
}

// LRem 从列表中移除指定数量的匹配元素.
// 参数：ctx - 上下文, key - 键名, count - 移除数量, value - 匹配值.
// 返回值：n - 实际移除数量, err - 操作失败时的错误.
func (r *Redis) LRem(ctx context.Context, key string, count int64, value interface{}) (int, error) {
	v, err := r.client.LRem(ctx, r.key(key), count, value).Result()
	return int(v), err
}

// LIndex 获取列表指定索引的元素.
// 参数：ctx - 上下文, key - 键名, idx - 索引.
// 返回值：val - 元素值, err - 操作失败时的错误.
func (r *Redis) LIndex(ctx context.Context, key string, idx int64) (string, error) {
	return r.client.LIndex(ctx, r.key(key), idx).Result()
}

// LTrim 保留列表指定范围，其余删除.
// 参数：ctx - 上下文, key - 键名, start - 起始索引, stop - 结束索引.
// 返回值：err - 操作失败时的错误.
func (r *Redis) LTrim(ctx context.Context, key string, start, stop int64) error {
	return r.client.LTrim(ctx, r.key(key), start, stop).Err()
}

// --- Set commands ---.

// SAdd 向集合添加成员.
// 参数：ctx - 上下文, key - 键名, members - 成员列表.
// 返回值：n - 新增成员数量, err - 操作失败时的错误.
func (r *Redis) SAdd(ctx context.Context, key string, members ...interface{}) (int, error) {
	v, err := r.client.SAdd(ctx, r.key(key), members...).Result()
	return int(v), err
}

// SCard 获取集合成员数量.
// 参数：ctx - 上下文, key - 键名.
// 返回值：n - 成员数量, err - 操作失败时的错误.
func (r *Redis) SCard(ctx context.Context, key string) (int, error) {
	v, err := r.client.SCard(ctx, r.key(key)).Result()
	return int(v), err
}

// SMembers 获取集合所有成员.
// 参数：ctx - 上下文, key - 键名.
// 返回值：members - 成员列表, err - 操作失败时的错误.
func (r *Redis) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.client.SMembers(ctx, r.key(key)).Result()
}

// SRem 从集合移除成员.
// 参数：ctx - 上下文, key - 键名, members - 待移除成员列表.
// 返回值：n - 实际移除数量, err - 操作失败时的错误.
func (r *Redis) SRem(ctx context.Context, key string, members ...interface{}) (int, error) {
	v, err := r.client.SRem(ctx, r.key(key), members...).Result()
	return int(v), err
}

// SPop 随机弹出集合中的一个成员.
// 参数：ctx - 上下文, key - 键名.
// 返回值：val - 弹出的成员, err - 操作失败时的错误.
func (r *Redis) SPop(ctx context.Context, key string) (string, error) {
	return r.client.SPop(ctx, r.key(key)).Result()
}

// SIsMember 检查成员是否在集合中.
// 参数：ctx - 上下文, key - 键名, member - 待检查成员.
// 返回值：ok - 是否存在, err - 操作失败时的错误.
func (r *Redis) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	return r.client.SIsMember(ctx, r.key(key), member).Result()
}

// --- Sorted Set commands ---.

// ZAdd 向有序集合添加成员.
// 参数：ctx - 上下文, key - 键名, score - 分数, member - 成员.
// 返回值：added - 是否新增, err - 操作失败时的错误.
func (r *Redis) ZAdd(ctx context.Context, key string, score float64, member string) (bool, error) {
	v, err := r.client.ZAdd(ctx, r.key(key), redis.Z{Score: score, Member: member}).Result()
	return v == 1, err
}

// ZAdds 批量向有序集合添加成员.
// 参数：ctx - 上下文, key - 键名, members - 元素列表.
// 返回值：n - 新增成员数量, err - 操作失败时的错误.
func (r *Redis) ZAdds(ctx context.Context, key string, members ...redis.Z) (int, error) {
	v, err := r.client.ZAdd(ctx, r.key(key), members...).Result()
	return int(v), err
}

// ZScore 获取有序集合成员的分数.
// 参数：ctx - 上下文, key - 键名, member - 成员.
// 返回值：score - 分数, err - 成员不存在或操作失败时的错误.
func (r *Redis) ZScore(ctx context.Context, key, member string) (float64, error) {
	return r.client.ZScore(ctx, r.key(key), member).Result()
}

// ZIncrBy 对有序集合成员的分数自增.
// 参数：ctx - 上下文, key - 键名, increment - 自增量, member - 成员.
// 返回值：score - 自增后的分数, err - 操作失败时的错误.
func (r *Redis) ZIncrBy(ctx context.Context, key string, increment float64, member string) (float64, error) {
	return r.client.ZIncrBy(ctx, r.key(key), increment, member).Result()
}

// ZRank 获取有序集合成员的正序排名.
// 参数：ctx - 上下文, key - 键名, member - 成员.
// 返回值：rank - 排名, err - 成员不存在或操作失败时的错误.
func (r *Redis) ZRank(ctx context.Context, key, member string) (int64, error) {
	return r.client.ZRank(ctx, r.key(key), member).Result()
}

// ZRevRank 获取有序集合成员的逆序排名.
// 参数：ctx - 上下文, key - 键名, member - 成员.
// 返回值：rank - 排名, err - 成员不存在或操作失败时的错误.
func (r *Redis) ZRevRank(ctx context.Context, key, member string) (int64, error) {
	return r.client.ZRevRank(ctx, r.key(key), member).Result()
}

// ZRange 按正序获取有序集合指定范围的成员.
// 参数：ctx - 上下文, key - 键名, start - 起始索引, stop - 结束索引.
// 返回值：members - 成员列表, err - 操作失败时的错误.
func (r *Redis) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRange(ctx, r.key(key), start, stop).Result()
}

// ZRevRange 按逆序获取有序集合指定范围的成员.
// 参数：ctx - 上下文, key - 键名, start - 起始索引, stop - 结束索引.
// 返回值：members - 成员列表, err - 操作失败时的错误.
func (r *Redis) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return r.client.ZRevRange(ctx, r.key(key), start, stop).Result()
}

// ZRangeWithScores 按正序获取有序集合指定范围的成员与分数.
// 参数：ctx - 上下文, key - 键名, start - 起始索引, stop - 结束索引.
// 返回值：zs - 元素列表, err - 操作失败时的错误.
func (r *Redis) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.client.ZRangeWithScores(ctx, r.key(key), start, stop).Result()
}

// ZRevRangeWithScores 按逆序获取有序集合指定范围的成员与分数.
// 参数：ctx - 上下文, key - 键名, start - 起始索引, stop - 结束索引.
// 返回值：zs - 元素列表, err - 操作失败时的错误.
func (r *Redis) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) ([]redis.Z, error) {
	return r.client.ZRevRangeWithScores(ctx, r.key(key), start, stop).Result()
}

// ZRangeByScoreWithScores 按分数范围获取有序集合成员与分数（数值型边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数, max - 最大分数.
// 返回值：zs - 元素列表, err - 操作失败时的错误.
func (r *Redis) ZRangeByScoreWithScores(ctx context.Context, key string, min, max float64) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: formatScore(min), Max: formatScore(max)}
	return r.client.ZRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

// ZRangeByScoreWithScoresRaw 按分数范围获取有序集合成员与分数（字符串边界，支持 -inf/+inf/(score）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数字符串, max - 最大分数字符串.
// 返回值：zs - 元素列表, err - 操作失败时的错误.
func (r *Redis) ZRangeByScoreWithScoresRaw(ctx context.Context, key, min, max string) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: min, Max: max}
	return r.client.ZRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

// ZRevRangeByScore 按逆序分数范围获取有序集合成员（数值型边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数, max - 最大分数.
// 返回值：members - 成员列表, err - 操作失败时的错误.
func (r *Redis) ZRevRangeByScore(ctx context.Context, key string, min, max float64) ([]string, error) {
	opt := &redis.ZRangeBy{Min: formatScore(min), Max: formatScore(max)}
	return r.client.ZRevRangeByScore(ctx, r.key(key), opt).Result()
}

// ZRevRangeByScoreRaw 按逆序分数范围获取有序集合成员（字符串边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数字符串, max - 最大分数字符串.
// 返回值：members - 成员列表, err - 操作失败时的错误.
func (r *Redis) ZRevRangeByScoreRaw(ctx context.Context, key, min, max string) ([]string, error) {
	opt := &redis.ZRangeBy{Min: min, Max: max}
	return r.client.ZRevRangeByScore(ctx, r.key(key), opt).Result()
}

// ZRevRangeByScoreWithScores 按逆序分数范围获取有序集合成员与分数（数值型边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数, max - 最大分数.
// 返回值：zs - 元素列表, err - 操作失败时的错误.
func (r *Redis) ZRevRangeByScoreWithScores(ctx context.Context, key string, min, max float64) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: formatScore(min), Max: formatScore(max)}
	return r.client.ZRevRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

// ZRevRangeByScoreWithScoresRaw 按逆序分数范围获取有序集合成员与分数（字符串边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数字符串, max - 最大分数字符串.
// 返回值：zs - 元素列表, err - 操作失败时的错误.
func (r *Redis) ZRevRangeByScoreWithScoresRaw(ctx context.Context, key, min, max string) ([]redis.Z, error) {
	opt := &redis.ZRangeBy{Min: min, Max: max}
	return r.client.ZRevRangeByScoreWithScores(ctx, r.key(key), opt).Result()
}

// ZCount 按分数范围统计有序集合成员数量（数值型边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数, max - 最大分数.
// 返回值：n - 成员数量, err - 操作失败时的错误.
func (r *Redis) ZCount(ctx context.Context, key string, min, max float64) (int, error) {
	v, err := r.client.ZCount(ctx, r.key(key), formatScore(min), formatScore(max)).Result()
	return int(v), err
}

// ZCountRaw 按分数范围统计有序集合成员数量（字符串边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数字符串, max - 最大分数字符串.
// 返回值：n - 成员数量, err - 操作失败时的错误.
func (r *Redis) ZCountRaw(ctx context.Context, key, min, max string) (int, error) {
	v, err := r.client.ZCount(ctx, r.key(key), min, max).Result()
	return int(v), err
}

// ZCard 获取有序集合成员数量.
// 参数：ctx - 上下文, key - 键名.
// 返回值：n - 成员数量, err - 操作失败时的错误.
func (r *Redis) ZCard(ctx context.Context, key string) (int, error) {
	v, err := r.client.ZCard(ctx, r.key(key)).Result()
	return int(v), err
}

// ZRem 移除有序集合成员.
// 参数：ctx - 上下文, key - 键名, members - 待移除成员列表.
// 返回值：n - 实际移除数量, err - 操作失败时的错误.
func (r *Redis) ZRem(ctx context.Context, key string, members ...interface{}) (int, error) {
	v, err := r.client.ZRem(ctx, r.key(key), members...).Result()
	return int(v), err
}

// ZRemRangeByScore 按分数范围移除有序集合成员（数值型边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数, max - 最大分数.
// 返回值：n - 实际移除数量, err - 操作失败时的错误.
func (r *Redis) ZRemRangeByScore(ctx context.Context, key string, min, max float64) (int, error) {
	v, err := r.client.ZRemRangeByScore(ctx, r.key(key), formatScore(min), formatScore(max)).Result()
	return int(v), err
}

// ZRemRangeByScoreRaw 按分数范围移除有序集合成员（字符串边界）.
// 参数：ctx - 上下文, key - 键名, min - 最小分数字符串, max - 最大分数字符串.
// 返回值：n - 实际移除数量, err - 操作失败时的错误.
func (r *Redis) ZRemRangeByScoreRaw(ctx context.Context, key, min, max string) (int, error) {
	v, err := r.client.ZRemRangeByScore(ctx, r.key(key), min, max).Result()
	return int(v), err
}

// ZRemRangeByRank 按排名范围移除有序集合成员.
// 参数：ctx - 上下文, key - 键名, start - 起始排名, stop - 结束排名.
// 返回值：n - 实际移除数量, err - 操作失败时的错误.
func (r *Redis) ZRemRangeByRank(ctx context.Context, key string, start, stop int64) (int, error) {
	v, err := r.client.ZRemRangeByRank(ctx, r.key(key), start, stop).Result()
	return int(v), err
}

// --- helpers ---.

// MinScore 有序集合最小分数标识 "-inf".
const MinScore = "-inf"

// MaxScore 有序集合最大分数标识 "+inf".
const MaxScore = "+inf"

// ExclusiveScore 将分数转换为 Redis 排他边界格式，用于 *Raw 方法.
// 参数：score - 分数值.
// 返回值：string - 排他边界字符串，如 "(100".
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
