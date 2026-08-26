package integration_test

import (
	"context"
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

	worker := jobqueue.NewWorker(jq, jobqueue.WorkerOptions{
		Concurrency:  4,
		PollInterval: 10 * time.Millisecond,
		Queues:       []string{"notifications", "webhooks"},
		MaxRetries:   2,
	})

	var notifProcessed int32
	var webhookProcessed int32
	var webhookAttempts int32

	worker.RegisterHandler("notifications", func(ctx context.Context, job *jobqueue.Job) error {
		atomic.AddInt32(&notifProcessed, 1)
		return nil
	})

	worker.RegisterHandler("webhooks", func(ctx context.Context, job *jobqueue.Job) error {
		attempts := atomic.AddInt32(&webhookAttempts, 1)
		if attempts <= 1 {
			return errors.New("simulated webhook endpoint timeout")
		}
		atomic.AddInt32(&webhookProcessed, 1)
		return nil
	})

	// 1. Enqueue 5 notifications
	for i := 0; i < 5; i++ {
		_, err := jq.Enqueue(ctx, "notifications", map[string]any{"id": i}, 3)
		require.NoError(t, err)
	}

	// 2. Enqueue 1 webhook that fails once then succeeds
	whJob, err := jq.Enqueue(ctx, "webhooks", map[string]string{"url": "https://example.com/webhook"}, 3)
	require.NoError(t, err)
	assert.NotEmpty(t, whJob.ID)

	err = worker.Start(ctx)
	require.NoError(t, err)
	assert.True(t, worker.IsRunning())

	// Verify all notifications succeed
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&notifProcessed) == 5
	}, 2*time.Second, 20*time.Millisecond)

	// Webhook should fail once
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&webhookAttempts) >= 1
	}, 2*time.Second, 20*time.Millisecond)

	// Fast-forward scheduled_at for retry
	time.Sleep(50 * time.Millisecond)
	// After backoff or retry, webhook succeeds
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&webhookProcessed) == 1
	}, 4*time.Second, 50*time.Millisecond)

	worker.Stop()
	assert.False(t, worker.IsRunning())
}

func TestIntegration_JobQueueWorker_DeadLetterAndAdminRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := jobqueue.NewJobQueue(nil)
	worker := jobqueue.NewWorker(jq, jobqueue.WorkerOptions{
		Concurrency:  2,
		PollInterval: 10 * time.Millisecond,
		Queues:       []string{"critical"},
		MaxRetries:   2,
	})

	var mu sync.Mutex
	shouldFail := true
	var processedSuccess int32

	worker.RegisterHandler("critical", func(ctx context.Context, job *jobqueue.Job) error {
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

	err = worker.Start(ctx)
	require.NoError(t, err)

	// Allow first attempt to fail
	time.Sleep(100 * time.Millisecond)

	// Allow second attempt to fail -> should become dead_letter
	assert.Eventually(t, func() bool {
		deadJobs, err := jq.GetDeadLetterJobs(ctx)
		return err == nil && len(deadJobs) == 1 && deadJobs[0].ID == job.ID
	}, 3*time.Second, 20*time.Millisecond)

	// Verify dead letter inspection
	deadJobs, err := jq.GetDeadLetterJobs(ctx)
	require.NoError(t, err)
	require.Len(t, deadJobs, 1)
	assert.Equal(t, jobqueue.StatusDeadLetter, deadJobs[0].Status)

	// Admin fixes the issue and retries the dead-letter job
	mu.Lock()
	shouldFail = false
	mu.Unlock()

	err = jq.RetryDeadLetterJob(ctx, job.ID)
	require.NoError(t, err)

	// Verify worker picks up the retried job and succeeds
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&processedSuccess) == 1
	}, 2*time.Second, 20*time.Millisecond)

	worker.Stop()
}
