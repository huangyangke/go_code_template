package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRedis(t *testing.T) (*Redis, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	r := MustNew(&Config{
		Name:  "cmdtest",
		Type:  StandaloneType,
		Addrs: []string{mr.Addr()},
	})
	t.Cleanup(func() { r.Close() })
	return r, mr
}

// --- String / Key ---

func TestSetXx(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	// SETXX only sets if key exists — should fail
	ok, err := r.SetXx(ctx, "xxkey", "v1", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok)

	// Create the key first
	require.NoError(t, r.SetEx(ctx, "xxkey", "v0", time.Minute))

	// Now SETXX should succeed
	ok, err = r.SetXx(ctx, "xxkey", "v1", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)

	val, err := r.Get(ctx, "xxkey")
	require.NoError(t, err)
	assert.Equal(t, "v1", val)
}

func TestDecr(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	require.NoError(t, r.SetEx(ctx, "counter", "10", time.Hour))
	v, err := r.Decr(ctx, "counter")
	require.NoError(t, err)
	assert.Equal(t, int64(9), v)
}

func TestDecrBy(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	require.NoError(t, r.SetEx(ctx, "counter", "10", time.Hour))
	v, err := r.DecrBy(ctx, "counter", 3)
	require.NoError(t, err)
	assert.Equal(t, int64(7), v)
}

func TestTTL(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	require.NoError(t, r.SetEx(ctx, "ttlkey", "v", 60*time.Second))
	ttl, err := r.TTL(ctx, "ttlkey")
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 60*time.Second)
}

func TestTTL_Missing(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	ttl, err := r.TTL(ctx, "doesnotexist")
	require.NoError(t, err)
	assert.Equal(t, time.Duration(-2), ttl)
}

// --- Hash ---

func TestHGetNoNil(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	val, err := r.HGetNoNil(ctx, "h1", "f1")
	require.NoError(t, err)
	assert.Equal(t, "", val)

	require.NoError(t, r.HSet(ctx, "h1", "f1", "hello"))
	val, err = r.HGetNoNil(ctx, "h1", "f1")
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
}

func TestHExists(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	ok, err := r.HExists(ctx, "h1", "f1")
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, r.HSet(ctx, "h1", "f1", "v"))
	ok, err = r.HExists(ctx, "h1", "f1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestHSetNx(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	ok, err := r.HSetNx(ctx, "h1", "f1", "v1")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = r.HSetNx(ctx, "h1", "f1", "v2")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHMSet_HMGet(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	err := r.HMSet(ctx, "h1", map[string]interface{}{"f1": "v1", "f2": "v2"})
	require.NoError(t, err)

	vals, err := r.HMGet(ctx, "h1", "f1", "f2", "f3")
	require.NoError(t, err)
	assert.Equal(t, []string{"v1", "v2", ""}, vals)
}

func TestHVals_HKeys_HLen(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	require.NoError(t, r.HSet(ctx, "h1", "a", "1"))
	require.NoError(t, r.HSet(ctx, "h1", "b", "2"))

	vals, err := r.HVals(ctx, "h1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1", "2"}, vals)

	keys, err := r.HKeys(ctx, "h1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, keys)

	n, err := r.HLen(ctx, "h1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestHIncrBy(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	require.NoError(t, r.HSet(ctx, "h1", "count", "10"))
	v, err := r.HIncrBy(ctx, "h1", "count", 5)
	require.NoError(t, err)
	assert.Equal(t, 15, v)
}

// --- List ---

func TestLPush_RPush_LPop_RPop(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	n, err := r.RPush(ctx, "list", "a", "b", "c")
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	n, err = r.LPush(ctx, "list", "z")
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	v, err := r.LPop(ctx, "list")
	require.NoError(t, err)
	assert.Equal(t, "z", v)

	v, err = r.RPop(ctx, "list")
	require.NoError(t, err)
	assert.Equal(t, "c", v)
}

func TestLRange_LLen(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.RPush(ctx, "list", "a", "b", "c")

	vals, err := r.LRange(ctx, "list", 0, -1)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, vals)

	n, err := r.LLen(ctx, "list")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestLRem_LIndex_LTrim(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.RPush(ctx, "list", "a", "b", "a", "c")
	n, err := r.LRem(ctx, "list", -2, "a")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// After removing 2 "a"s, list = [b, c]
	v, err := r.LIndex(ctx, "list", 0)
	require.NoError(t, err)
	assert.Equal(t, "b", v)

	err = r.LTrim(ctx, "list", 0, 0)
	require.NoError(t, err)

	vals, _ := r.LRange(ctx, "list", 0, -1)
	assert.Equal(t, []string{"b"}, vals)
}

// --- Set ---

func TestSAdd_SCard_SMembers_SRem(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	n, err := r.SAdd(ctx, "s1", "a", "b", "c")
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	n, err = r.SCard(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	members, err := r.SMembers(ctx, "s1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b", "c"}, members)

	n, err = r.SRem(ctx, "s1", "a")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	n, err = r.SCard(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestSPop_SIsMember(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.SAdd(ctx, "s1", "a", "b")

	v, err := r.SPop(ctx, "s1")
	require.NoError(t, err)
	assert.Contains(t, []string{"a", "b"}, v)

	n, err := r.SCard(ctx, "s1")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	ok, err := r.SIsMember(ctx, "s1", v)
	require.NoError(t, err)
	assert.False(t, ok, "popped member should not be a set member anymore")
}

// --- Sorted Set ---

func TestZAdd_ZScore_ZCard_ZRank(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	ok, err := r.ZAdd(ctx, "z1", 100.5, "alice")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = r.ZAdd(ctx, "z1", 200, "bob")
	require.NoError(t, err)
	assert.True(t, ok)

	score, err := r.ZScore(ctx, "z1", "alice")
	require.NoError(t, err)
	assert.Equal(t, 100.5, score)

	n, err := r.ZCard(ctx, "z1")
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	rank, err := r.ZRank(ctx, "z1", "alice")
	require.NoError(t, err)
	assert.Equal(t, int64(0), rank)
}

func TestZIncrBy(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	v, err := r.ZIncrBy(ctx, "z1", 50.25, "alice")
	require.NoError(t, err)
	assert.Equal(t, 150.25, v)
}

func TestZRevRank(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")

	rank, err := r.ZRevRank(ctx, "z1", "alice")
	require.NoError(t, err)
	assert.Equal(t, int64(1), rank) // alice is rank 1 in reverse (bob=0)
}

func TestZRange_ZRevRange(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")
	_, _ = r.ZAdd(ctx, "z1", 300, "carol")

	vals, err := r.ZRange(ctx, "z1", 0, -1)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "bob", "carol"}, vals)

	vals, err = r.ZRevRange(ctx, "z1", 0, -1)
	require.NoError(t, err)
	assert.Equal(t, []string{"carol", "bob", "alice"}, vals)
}

func TestZRangeWithScores(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")

	zs, err := r.ZRangeWithScores(ctx, "z1", 0, -1)
	require.NoError(t, err)
	require.Len(t, zs, 2)
	assert.Equal(t, float64(100), zs[0].Score)
	assert.Equal(t, "alice", zs[0].Member)
}

func TestZRevRangeWithScores(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")

	zs, err := r.ZRevRangeWithScores(ctx, "z1", 0, -1)
	require.NoError(t, err)
	require.Len(t, zs, 2)
	assert.Equal(t, float64(200), zs[0].Score)
	assert.Equal(t, "bob", zs[0].Member)
}

func TestZRangeByScoreWithScores(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")
	_, _ = r.ZAdd(ctx, "z1", 300, "carol")

	// float64 params — common case, just pass numbers
	zs, err := r.ZRangeByScoreWithScores(ctx, "z1", 100, 200)
	require.NoError(t, err)
	require.Len(t, zs, 2)
	assert.Equal(t, "alice", zs[0].Member)
	assert.Equal(t, "bob", zs[1].Member)
}

func TestZRangeByScoreWithScoresRaw(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")
	_, _ = r.ZAdd(ctx, "z1", 300, "carol")

	// Open interval: (100, 200] should include only bob
	zs, err := r.ZRangeByScoreWithScoresRaw(ctx, "z1", ExclusiveScore(100), "200")
	require.NoError(t, err)
	require.Len(t, zs, 1)
	assert.Equal(t, "bob", zs[0].Member)

	// All elements with -inf / +inf
	zs, err = r.ZRangeByScoreWithScoresRaw(ctx, "z1", MinScore, MaxScore)
	require.NoError(t, err)
	require.Len(t, zs, 3)
}

func TestZRevRangeByScore(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")
	_, _ = r.ZAdd(ctx, "z1", 300, "carol")

	vals, err := r.ZRevRangeByScore(ctx, "z1", 100, 200)
	require.NoError(t, err)
	assert.Equal(t, []string{"bob", "alice"}, vals)
}

func TestZRevRangeByScoreWithScores(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")

	zs, err := r.ZRevRangeByScoreWithScores(ctx, "z1", 100, 200)
	require.NoError(t, err)
	require.Len(t, zs, 2)
	assert.Equal(t, float64(200), zs[0].Score)
}

func TestZCount(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")
	_, _ = r.ZAdd(ctx, "z1", 300, "carol")

	n, err := r.ZCount(ctx, "z1", 100, 200)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestZRem_ZRemRangeByScore_ZRemRangeByRank(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	_, _ = r.ZAdd(ctx, "z1", 100, "alice")
	_, _ = r.ZAdd(ctx, "z1", 200, "bob")
	_, _ = r.ZAdd(ctx, "z1", 300, "carol")

	n, err := r.ZRem(ctx, "z1", "alice")
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	n, err = r.ZRemRangeByScore(ctx, "z1", 200, 300)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	n, err = r.ZCard(ctx, "z1")
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// Re-add and test ZRemRangeByRank
	_, _ = r.ZAdd(ctx, "z1", 100, "a")
	_, _ = r.ZAdd(ctx, "z1", 200, "b")
	_, _ = r.ZAdd(ctx, "z1", 300, "c")

	n, err = r.ZRemRangeByRank(ctx, "z1", 0, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestZAdds(t *testing.T) {
	r, _ := newTestRedis(t)
	ctx := context.Background()

	n, err := r.ZAdds(ctx, "z1",
		Z{Score: 100, Member: "alice"},
		Z{Score: 200, Member: "bob"},
	)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

func TestExclusiveScore(t *testing.T) {
	assert.Equal(t, "(100", ExclusiveScore(100))
	assert.Equal(t, "(0.5", ExclusiveScore(0.5))
}

func TestMinScoreMaxScore(t *testing.T) {
	assert.Equal(t, "-inf", MinScore)
	assert.Equal(t, "+inf", MaxScore)
}
