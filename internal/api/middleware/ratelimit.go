package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/moistello/backend/config"
	"github.com/moistello/backend/internal/infrastructure/ratelimit"
)

// errRedisUnavailable is returned when Redis cannot be reached during a rate
// limit check. The middleware falls back to an in-memory rate limiter rather
// than failing closed with 503.
var errRedisUnavailable = errors.New("rate limiter: Redis unavailable")

var inMemoryLimiter = ratelimit.NewInMemoryRateLimiter()

func RateLimitMiddleware(redisClient *redis.Client, cfg config.RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "ratelimit:g:" + c.ClientIP()
		limit := cfg.Global
		if _, exists := c.Get("userID"); exists {
			key = "ratelimit:u:" + GetUserID(c)
			limit = cfg.Authenticated
		}

		allowed, remaining, ttl, err := checkLimit(c, redisClient, key, limit)
		if err != nil {
			log.Warn().Err(err).Str("key", key).Msg("rate limit check failed: falling back to in-memory limiter")
			allowed, remaining, ttl, _ = inMemoryLimiter.Check(c.Request.Context(), key, limit, 1*time.Minute)
		}

		setRateLimitHeaders(c, limit, remaining, ttl)

		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success":    false,
				"error":      "rate limit exceeded",
				"retryAfter": ttl.Seconds(),
			})
			return
		}
		c.Next()
	}
}

func AuthRateLimitMiddleware(redisClient *redis.Client, cfg config.RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := "ratelimit:a:" + c.ClientIP()
		limit := cfg.Auth

		allowed, remaining, ttl, err := checkLimit(c, redisClient, key, limit)
		if err != nil {
			log.Warn().Err(err).Str("key", key).Msg("rate limit check failed: falling back to in-memory limiter")
			allowed, remaining, ttl, _ = inMemoryLimiter.Check(c.Request.Context(), key, limit, 1*time.Minute)
		}

		setRateLimitHeaders(c, limit, remaining, ttl)

		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success":    false,
				"error":      "too many auth attempts",
				"retryAfter": ttl.Seconds(),
			})
			return
		}
		c.Next()
	}
}

func PerResourceRateLimitMiddleware(redisClient *redis.Client, resource string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := fmt.Sprintf("ratelimit:r:%s:%s", resource, c.ClientIP())
		allowed, remaining, ttl, err := checkLimitWithWindow(c, redisClient, key, limit, window)
		if err != nil {
			log.Warn().Err(err).Str("key", key).Msg("rate limit check failed: falling back to in-memory limiter")
			allowed, remaining, ttl, _ = inMemoryLimiter.Check(c.Request.Context(), key, limit, window)
		}

		setRateLimitHeaders(c, limit, remaining, ttl)

		if !allowed {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success":    false,
				"error":      fmt.Sprintf("rate limit exceeded for %s", resource),
				"retryAfter": ttl.Seconds(),
			})
			return
		}
		c.Next()
	}
}

func checkLimit(c *gin.Context, redisClient *redis.Client, key string, limit int) (bool, int, time.Duration, error) {
	return checkLimitWithWindow(c, redisClient, key, limit, 1*time.Minute)
}

// checkLimitWithWindow implements a Redis-backed sliding window rate limiter
// using a sorted set with timestamps as scores. It returns (allowed, remaining, ttl, err).
// err is non-nil only when Redis is unreachable — callers fall back to an
// in-memory rate limiter rather than failing the request.
func checkLimitWithWindow(c *gin.Context, redisClient *redis.Client, key string, limit int, window time.Duration) (bool, int, time.Duration, error) {
	reqCtx := c.Request.Context()
	redisKey := fmt.Sprintf("ratelimit:%s", key)

	now := time.Now().UnixNano()
	windowStart := now - window.Nanoseconds()

	pipe := redisClient.Pipeline()
	pipe.ZRemRangeByScore(reqCtx, redisKey, "-inf", fmt.Sprintf("%d", windowStart))
	pipe.ZAdd(reqCtx, redisKey, &redis.Z{Score: float64(now), Member: now})
	pipe.ZCard(reqCtx, redisKey)
	pipe.Expire(reqCtx, redisKey, window)

	results, err := pipe.Exec(reqCtx)
	if err != nil {
		log.Error().Err(err).Str("key", key).Msg("rate limit sliding window failed: Redis unreachable")
		return false, 0, 0, errRedisUnavailable
	}

	count := results[2].(*redis.IntCmd).Val()
	if count > int64(limit) {
		ttl, err := redisClient.TTL(reqCtx, redisKey).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return false, 0, ttl, nil
	}

	remaining := limit - int(count)
	return true, remaining, window, nil
}

func setRateLimitHeaders(c *gin.Context, limit, remaining int, ttl time.Duration) {
	c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
	c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(ttl).Unix(), 10))
}
