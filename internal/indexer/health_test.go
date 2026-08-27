package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubCursorReader lets tests control cursor lag / errors without a real DB.
type stubCursorReader struct {
	cursor *Cursor
	err    error
}

func (s *stubCursorReader) GetCurrent(ctx context.Context) (*Cursor, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.cursor, nil
}

type stubRabbit struct{ alive bool }

func (s *stubRabbit) IsAlive() bool { return s.alive }

func newTestHandler(t *testing.T) (*HealthHandler, sqlmock.Sqlmock, func()) {
	t.Helper()

	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	db := sqlx.NewDb(mockDB, "sqlmock")

	mr, err := miniredis.Run()
	require.NoError(t, err)
	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	cursor := &stubCursorReader{cursor: &Cursor{Chain: "stellar", LastLedger: 100, LastProcessedAt: time.Now()}}
	rabbit := &stubRabbit{alive: true}

	h := NewHealthHandler(db, redisClient, rabbit, cursor, time.Minute)

	cleanup := func() {
		mockDB.Close()
		redisClient.Close()
		mr.Close()
	}

	return h, mock, cleanup
}

func TestHealthHandler_Health_AllHealthy(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "healthy", resp.Dependencies["postgres"].Status)
	assert.Equal(t, "healthy", resp.Dependencies["redis"].Status)
	assert.Equal(t, "healthy", resp.Dependencies["rabbitmq"].Status)
	assert.Equal(t, "healthy", resp.Dependencies["cursor"].Status)
}

func TestHealthHandler_Health_RabbitDown(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing()
	h.rabbit = &stubRabbit{alive: false}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "degraded", resp.Status)
	assert.Equal(t, "unhealthy", resp.Dependencies["rabbitmq"].Status)
	// other dependencies remain healthy — this is what the old handler missed
	assert.Equal(t, "healthy", resp.Dependencies["postgres"].Status)
}

func TestHealthHandler_Health_DatabaseDown(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing().WillReturnError(errors.New("connection refused"))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unhealthy", resp.Dependencies["postgres"].Status)
}

func TestHealthHandler_Health_RedisDown(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing()

	// Point at an address nothing is listening on so Ping fails fast.
	h.redis = redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unhealthy", resp.Dependencies["redis"].Status)
}

func TestHealthHandler_Health_CursorStale(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing()

	h.cursor = &stubCursorReader{cursor: &Cursor{
		Chain:           "stellar",
		LastLedger:      100,
		LastProcessedAt: time.Now().Add(-10 * time.Minute),
	}}
	h.maxCursorLag = time.Minute

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unhealthy", resp.Dependencies["cursor"].Status)
	assert.Contains(t, resp.Dependencies["cursor"].Message, "stale")
}

func TestHealthHandler_Health_CursorReadError(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing()

	h.cursor = &stubCursorReader{err: errors.New("no rows in result set")}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	h.Health(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "unhealthy", resp.Dependencies["cursor"].Status)
}

func TestHealthHandler_Ready_AllHealthy(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing()

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp.Status)
}

func TestHealthHandler_Ready_Unhealthy(t *testing.T) {
	h, mock, cleanup := newTestHandler(t)
	defer cleanup()
	mock.ExpectPing()
	h.rabbit = &stubRabbit{alive: false}

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	h.Ready(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "not ready", resp.Status)
}

func TestNewHealthHandler_DefaultsMaxCursorLag(t *testing.T) {
	h := NewHealthHandler(nil, nil, nil, nil, 0)
	assert.Equal(t, 2*time.Minute, h.maxCursorLag)
}
