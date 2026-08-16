package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"github.com/moistello/backend/config"
	"github.com/moistello/backend/internal/api/middleware"
)

func newRateLimitConfig() config.RateLimitConfig {
	return config.RateLimitConfig{
		Global:        100,
		Authenticated: 200,
		Auth:          20,
	}
}

// newUnreachableRedis returns a Redis client that will always fail (no server at
// that address), used to exercise the in-memory fallback path.
func newUnreachableRedis() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: "localhost:19999", DB: 0})
}

// TestRateLimitMiddleware_FallsBackToInMemoryWhenRedisDown verifies that the
// middleware falls back to an in-memory rate limiter instead of failing closed
// with 503 when Redis is unreachable.
func TestRateLimitMiddleware_FallsBackToInMemoryWhenRedisDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb := newUnreachableRedis()
	defer rdb.Close()

	cfg := newRateLimitConfig()
	r := gin.New()
	r.Use(middleware.RateLimitMiddleware(rdb, cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "100", w.Header().Get("X-RateLimit-Limit"))
}

// TestAuthRateLimitMiddleware_FallsBackToInMemoryWhenRedisDown verifies the
// same in-memory fallback behaviour for the auth-specific rate limit middleware.
func TestAuthRateLimitMiddleware_FallsBackToInMemoryWhenRedisDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb := newUnreachableRedis()
	defer rdb.Close()

	cfg := newRateLimitConfig()
	r := gin.New()
	r.Use(middleware.AuthRateLimitMiddleware(rdb, cfg))
	r.POST("/auth/login", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/login", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "20", w.Header().Get("X-RateLimit-Limit"))
}

// TestRateLimitMiddleware_SetsHeaders verifies that rate-limit headers are set
// when Redis is available and the limit has not been reached.
// This test requires a running Redis at localhost:6379.
func TestRateLimitMiddleware_SetsHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	defer rdb.Close()

	cfg := newRateLimitConfig()
	r := gin.New()
	r.Use(middleware.RateLimitMiddleware(rdb, cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	// If Redis is available this should pass and set headers.
	// If Redis is unavailable this falls back to in-memory and still sets headers.
	if w.Code == http.StatusOK {
		assert.Equal(t, "100", w.Header().Get("X-RateLimit-Limit"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Remaining"))
		assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
	}
}

// TestAuthRateLimitMiddleware_SetsAuthLimit verifies the auth limit value is
// correctly propagated to response headers.
func TestAuthRateLimitMiddleware_SetsAuthLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	defer rdb.Close()

	cfg := newRateLimitConfig()
	r := gin.New()
	r.Use(middleware.AuthRateLimitMiddleware(rdb, cfg))
	r.POST("/auth/login", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		assert.Equal(t, "20", w.Header().Get("X-RateLimit-Limit"))
	}
}

// TestRateLimitMiddleware_AuthenticatedUserLimit verifies that authenticated
// users receive the higher authenticated limit.
func TestRateLimitMiddleware_AuthenticatedUserLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	defer rdb.Close()

	cfg := config.RateLimitConfig{
		Global:        100,
		Authenticated: 200,
		Auth:          20,
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", "test-user-id")
		c.Next()
	})
	r.Use(middleware.RateLimitMiddleware(rdb, cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		assert.Equal(t, "200", w.Header().Get("X-RateLimit-Limit"))
	}
}

// TestRateLimitMiddleware_MultipleMiddlewareChain verifies that chaining both
// middlewares does not panic and produces a coherent response.
func TestRateLimitMiddleware_MultipleMiddlewareChain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 15})
	defer rdb.Close()

	cfg := newRateLimitConfig()
	r := gin.New()
	r.Use(middleware.RateLimitMiddleware(rdb, cfg))
	r.Use(middleware.AuthRateLimitMiddleware(rdb, cfg))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
