package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/internal/api/middleware"
	"github.com/moistello/backend/pkg/logger"
)

func TestGetRequestID_UsesCanonicalLoggerKey(t *testing.T) {
	// The middleware stores the request ID under logger.RequestIDKey; verify
	// GetRequestID reads from the same key so logs, errors and traces all
	// correlate (#229).
	ctx := context.WithValue(context.Background(), logger.RequestIDKey, "req-abc")
	require.Equal(t, "req-abc", middleware.GetRequestID(ctx))
	require.Equal(t, "unknown", middleware.GetRequestID(context.Background()))
}

func TestLoggingMiddleware_SetsRequestIDEverywhere(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var seenCtx string
	var seenGin string

	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.GET("/ping", func(c *gin.Context) {
		seenGin = c.GetString("requestID")
		seenCtx = middleware.GetRequestID(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ping", nil)
	req.Header.Set("X-Request-ID", "req-from-client")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "req-from-client", seenGin)
	require.Equal(t, "req-from-client", seenCtx)
	require.Equal(t, "req-from-client", w.Header().Get("X-Request-ID"))
}

func TestLoggingMiddleware_GeneratesRequestIDWhenAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var seenCtx string

	r := gin.New()
	r.Use(middleware.LoggingMiddleware())
	r.GET("/ping", func(c *gin.Context) {
		seenCtx = middleware.GetRequestID(c.Request.Context())
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ping", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.NotEmpty(t, seenCtx, "a request ID should be generated when none is supplied")
	require.Equal(t, seenCtx, w.Header().Get("X-Request-ID"))
}
