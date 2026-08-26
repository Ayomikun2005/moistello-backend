package jobqueue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorker_ProcessJobsSuccessfully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := NewJobQueue(nil)
	worker := NewWorker(jq, WorkerOptions{
		Concurrency:  3,
		PollInterval: 10 * time.Millisecond,
		Queues:       []string{"emails"},
	})

	var processedCount int32
	worker.RegisterHandler("emails", func(ctx context.Context, job *Job) error {
		atomic.AddInt32(&processedCount, 1)
		return nil
	})

	// Enqueue 5 jobs
	for i := 0; i < 5; i++ {
		_, err := jq.Enqueue(ctx, "emails", map[string]int{"idx": i}, 3)
		require.NoError(t, err)
	}

	err := worker.Start(ctx)
	require.NoError(t, err)
	assert.True(t, worker.IsRunning())

	// Wait for processing
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&processedCount) == 5
	}, 2*time.Second, 20*time.Millisecond)

	worker.Stop()
	assert.False(t, worker.IsRunning())

	// Verify all jobs in memory are completed
	for _, j := range jq.memory {
		assert.Equal(t, StatusCompleted, j.Status)
	}
}

func TestWorker_RetryAndDeadLetter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := NewJobQueue(nil)
	worker := NewWorker(jq, WorkerOptions{
		Concurrency:  2,
		PollInterval: 10 * time.Millisecond,
		Queues:       []string{"tasks"},
	})

	var attemptCount int32
	worker.RegisterHandler("tasks", func(ctx context.Context, job *Job) error {
		atomic.AddInt32(&attemptCount, 1)
		return errors.New("simulated transient failure")
	})

	// Enqueue a job with max_retries = 2
	job, err := jq.Enqueue(ctx, "tasks", "payload", 2)
	require.NoError(t, err)

	err = worker.Start(ctx)
	require.NoError(t, err)

	// First attempt fails -> status pending, scheduled in future
	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(&attemptCount) >= 1
	}, 1*time.Second, 10*time.Millisecond)

	// Manually fast-forward scheduled_at to test second attempt immediately
	jq.mu.Lock()
	jq.memory[job.ID].ScheduledAt = time.Now().Add(-1 * time.Second)
	jq.mu.Unlock()

	// Second attempt fails -> moves to dead_letter (since retriesCount >= maxRetries)
	assert.Eventually(t, func() bool {
		jq.mu.Lock()
		defer jq.mu.Unlock()
		return jq.memory[job.ID].Status == StatusDeadLetter
	}, 2*time.Second, 10*time.Millisecond)

	deadJobs, err := jq.GetDeadLetterJobs(ctx)
	require.NoError(t, err)
	assert.Len(t, deadJobs, 1)
	assert.Equal(t, job.ID, deadJobs[0].ID)

	worker.Stop()
}

func TestWorker_ConcurrentExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := NewJobQueue(nil)
	concurrency := 4
	worker := NewWorker(jq, WorkerOptions{
		Concurrency:  concurrency,
		PollInterval: 10 * time.Millisecond,
		Queues:       []string{"parallel"},
	})

	var currentActive int32
	var maxActive int32
	var mu sync.Mutex

	worker.RegisterHandler("parallel", func(ctx context.Context, job *Job) error {
		active := atomic.AddInt32(&currentActive, 1)
		mu.Lock()
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&currentActive, -1)
		return nil
	})

	for i := 0; i < 8; i++ {
		_, err := jq.Enqueue(ctx, "parallel", i, 3)
		require.NoError(t, err)
	}

	err := worker.Start(ctx)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return maxActive > 1
	}, 2*time.Second, 20*time.Millisecond)

	worker.Stop()
}

func TestWorker_UnregisteredHandlerFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := NewJobQueue(nil)
	worker := NewWorker(jq, WorkerOptions{
		Concurrency:  1,
		PollInterval: 10 * time.Millisecond,
		Queues:       []string{"unhandled"},
	})

	job, err := jq.Enqueue(ctx, "unhandled", "data", 3)
	require.NoError(t, err)

	err = worker.Start(ctx)
	require.NoError(t, err)

	assert.Eventually(t, func() bool {
		jq.mu.Lock()
		defer jq.mu.Unlock()
		return jq.memory[job.ID].Status == StatusCompleted
	}, 1*time.Second, 10*time.Millisecond)

	worker.Stop()
}
