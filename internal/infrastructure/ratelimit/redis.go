package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisRateLimiter struct {
	client *redis.Client
}

func NewRedisRateLimiter(client *redis.Client) *RedisRateLimiter {
	return &RedisRateLimiter{client: client}
}

// slidingWindowRedis checks the rate limit using a sorted set with timestamps
// as scores. Entries outside the window are removed before counting.
func (l *RedisRateLimiter) slidingWindowRedis(ctx context.Context, key string, maxRequests int, window time.Duration) (bool, int, time.Duration, error) {
	redisKey := fmt.Sprintf("ratelimit:%s", key)
	now := time.Now().UnixNano()
	windowStart := now - window.Nanoseconds()

	pipe := l.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, redisKey, "-inf", fmt.Sprintf("%d", windowStart))
	pipe.ZAdd(ctx, redisKey, redis.Z{Score: float64(now), Member: now})
	pipe.ZCard(ctx, redisKey)
	pipe.Expire(ctx, redisKey, window)

	results, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, 0, fmt.Errorf("checking sliding window rate limit: %w", err)
	}

	count := results[2].(*redis.IntCmd).Val()
	if count > int64(maxRequests) {
		ttl, err := l.client.TTL(ctx, redisKey).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return false, 0, ttl, nil
	}

	remaining := maxRequests - int(count)
	return true, remaining, window, nil
}

func (l *RedisRateLimiter) Check(ctx context.Context, key string, maxRequests int, window time.Duration) (bool, time.Duration, error) {
	allowed, _, ttl, err := l.slidingWindowRedis(ctx, key, maxRequests, window)
	return allowed, ttl, err
}

func (l *RedisRateLimiter) Reset(ctx context.Context, key string) error {
	redisKey := fmt.Sprintf("ratelimit:%s", key)
	if err := l.client.Del(ctx, redisKey).Err(); err != nil {
		return fmt.Errorf("resetting rate limit: %w", err)
	}
	return nil
}

// InMemoryRateLimiter provides a fallback when Redis is unavailable.
type InMemoryRateLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
}

func NewInMemoryRateLimiter() *InMemoryRateLimiter {
	return &InMemoryRateLimiter{windows: make(map[string][]time.Time)}
}

func (l *InMemoryRateLimiter) Check(ctx context.Context, key string, maxRequests int, window time.Duration) (bool, int, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-window)

	var recent []time.Time
	for _, t := range l.windows[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	l.windows[key] = recent

	if len(recent) >= maxRequests {
		if len(recent) > 0 {
			ttl := recent[0].Add(window).Sub(now)
			if ttl < 0 {
				ttl = 0
			}
			return false, 0, ttl, nil
		}
		return false, 0, window, nil
	}

	l.windows[key] = append(l.windows[key], now)
	remaining := maxRequests - len(l.windows[key])
	if remaining < 0 {
		remaining = 0
	}
	return true, remaining, window, nil
}
