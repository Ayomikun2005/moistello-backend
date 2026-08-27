package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/moistello/backend/pkg/jobqueue"
)

func TestIntegration_JobQueueWorker_FullLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := jobqueue.NewJobQueue(nil)

	worker := jobqueue.NewWorker(jq, 10*time.Millisecond)

	var notifProcessed int32
	var webhookProcessed int32
	var webhookAttempts int32

	worker.RegisterHandler("notifications", func(ctx context.Context, payload json.RawMessage) error {
		atomic.AddInt32(&notifProcessed, 1)
		return nil
	})

	worker.RegisterHandler("webhooks", func(ctx context.Context, payload json.RawMessage) error {
		attempts := atomic.AddInt32(&webhookAttempts, 1)
		if attempts <= 1 {
			return errors.New("simulated webhook endpoint timeout")
		}
		atomic.AddInt32(&webhookProcessed, 1)
		return nil
	})

	for i := 0; i < 5; i++ {
		_, err := jq.Enqueue(ctx, "notifications", map[string]any{"id": i}, 3)
		require.NoError(t, err)
	}

	whJob, err := jq.Enqueue(ctx, "webhooks", map[string]string{"url": "https://example.com/webhook"}, 3)
	require.NoError(t, err)
	assert.NotEmpty(t, whJob.ID)

	worker.Start(ctx)

	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&notifProcessed) == 5
	}, 2*time.Second, 20*time.Millisecond)

	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&webhookAttempts) >= 1
	}, 2*time.Second, 20*time.Millisecond)

	time.Sleep(50 * time.Millisecond)
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&webhookProcessed) == 1
	}, 4*time.Second, 50*time.Millisecond)

	worker.Stop()
}

func TestIntegration_JobQueueWorker_DeadLetterAndAdminRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := jobqueue.NewJobQueue(nil)
	worker := jobqueue.NewWorker(jq, 10*time.Millisecond)

	var mu sync.Mutex
	shouldFail := true
	var processedSuccess int32

	worker.RegisterHandler("critical", func(ctx context.Context, payload json.RawMessage) error {
		mu.Lock()
		fail := shouldFail
		mu.Unlock()
		if fail {
			return errors.New("permanent failure until fix")
		}
		atomic.AddInt32(&processedSuccess, 1)
		return nil
	})

	job, err := jq.Enqueue(ctx, "critical", "important_payload", 2)
	require.NoError(t, err)

	worker.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	assert.Eventually(t, func() bool {
		deadJobs, err := jq.GetDeadLetterJobs(ctx)
		return err == nil && len(deadJobs) == 1 && deadJobs[0].ID == job.ID
	}, 3*time.Second, 20*time.Millisecond)

	deadJobs, err := jq.GetDeadLetterJobs(ctx)
	require.NoError(t, err)
	require.Len(t, deadJobs, 1)
	assert.Equal(t, jobqueue.StatusDeadLetter, deadJobs[0].Status)

	mu.Lock()
	shouldFail = false
	mu.Unlock()

	err = jq.RetryDeadLetterJob(ctx, job.ID)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&processedSuccess) == 1
	}, 2*time.Second, 20*time.Millisecond)

	worker.Stop()
}
