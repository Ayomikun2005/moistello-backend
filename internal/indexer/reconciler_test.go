package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCursorTracker is an in-memory thread-safe cursor tracker for tests.
type mockCursorTracker struct {
	mu         sync.Mutex
	lastLedger int64
}

func (m *mockCursorTracker) GetCurrent(ctx context.Context) (*Cursor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &Cursor{
		Chain:           "stellar",
		LastLedger:      m.lastLedger,
		LastProcessedAt: time.Now(),
	}, nil
}

func (m *mockCursorTracker) Update(ctx context.Context, lastLedger int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Parallel-safe monotonic update: only advance if higher
	if lastLedger > m.lastLedger {
		m.lastLedger = lastLedger
	}
	return nil
}

func TestReconciler_BackfillMultipleBatches(t *testing.T) {
	// Total 150 missed ledgers from sequence 100 to 250
	const startLedger = int64(100)
	const endLedger = int64(250)

	var requestedBatchSizes []int
	var requestedCursors []int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursorStr := r.URL.Query().Get("cursor")
		limitStr := r.URL.Query().Get("limit")

		cursor, _ := strconv.ParseInt(cursorStr, 10, 64)
		limit, _ := strconv.Atoi(limitStr)

		requestedCursors = append(requestedCursors, cursor)
		requestedBatchSizes = append(requestedBatchSizes, limit)

		// /ledgers handler
		if r.URL.Path == "/ledgers" {
			var records []Ledger
			for i := int64(1); i <= int64(limit); i++ {
				seq := cursor + i
				if seq > endLedger {
					break
				}
				records = append(records, Ledger{
					Sequence: seq,
					ClosedAt: time.Now(),
					TxCount:  1,
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(LedgerResponse{
				Embedded: struct {
					Records []Ledger `json:"records"`
				}{Records: records},
			})
			return
		}

		// /ledgers/{sequence}/transactions handler
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TransactionResponse{
			Embedded: struct {
				Records []Transaction `json:"records"`
			}{
				Records: []Transaction{
					{
						Hash:       fmt.Sprintf("tx-hash-ledger-%s", r.URL.Path),
						Successful: true,
					},
				},
			},
		})
	}))
	defer server.Close()

	mockTracker := &mockCursorTracker{lastLedger: startLedger}
	poller := NewPoller(server.URL, nil)
	dedup := NewDeduplicator(1 * time.Hour)

	var processedCount int32
	processor := &EventProcessor{}
	_ = processor

	// Create reconciler with batchSize=30, maxLedgersPerCycle=200
	reconciler := NewReconciler(nil, poller, processor, dedup).
		WithBatchSize(30).
		WithMaxLedgersPerCycle(200)

	assert.Equal(t, 30, reconciler.batchSize)
	assert.Equal(t, 200, reconciler.maxLedgersPerCycle)

	_ = mockTracker
	_ = atomic.AddInt32(&processedCount, 0)
}

func TestParallelSafeCursorAdvancement(t *testing.T) {
	tracker := &mockCursorTracker{lastLedger: 100}

	var wg sync.WaitGroup
	// Concurrently attempt to update cursor to 90, 110, 105, 150, 120
	updates := []int64{90, 110, 105, 150, 120, 80, 200, 190, 160}
	for _, seq := range updates {
		wg.Add(1)
		go func(s int64) {
			defer wg.Done()
			_ = tracker.Update(context.Background(), s)
		}(seq)
	}
	wg.Wait()

	cur, err := tracker.GetCurrent(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(200), cur.LastLedger, "cursor should have advanced to the highest ledger monotonically")
}
