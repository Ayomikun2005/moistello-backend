package jobqueue

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitFor polls cond until it returns true or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}

// jobStatus reads a job's status and retry count from the in-memory store
// under the queue's lock — the worker goroutine mutates the same map.
func jobStatus(jq *JobQueue, id string) (JobStatus, int) {
	jq.mu.Lock()
	defer jq.mu.Unlock()
	job, ok := jq.memory[id]
	if !ok {
		return "", 0
	}
	return job.Status, job.RetriesCount
}

// TestWorker_EnqueueDequeueComplete exercises the full happy path through the
// worker loop: enqueue → worker dequeues → handler runs → job completed.
func TestWorker_EnqueueDequeueComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := NewJobQueue(nil) // in-memory store
	w := NewWorker(jq, 10*time.Millisecond)

	var mu sync.Mutex
	processed := make(map[string]json.RawMessage)

	w.RegisterHandler("emails", func(ctx context.Context, payload json.RawMessage) error {
		mu.Lock()
		processed["emails"] = payload
		mu.Unlock()
		return nil
	})
	w.Start(ctx)
	defer w.Stop()

	job, err := jq.Enqueue(ctx, "emails", map[string]string{"to": "user@example.com"}, 3)
	require.NoError(t, err)

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(processed) > 0
	}, "handler to run")

	waitFor(t, 2*time.Second, func() bool {
		status, _ := jobStatus(jq, job.ID)
		return status == StatusCompleted
	}, "job to complete")

	mu.Lock()
	defer mu.Unlock()
	assert.JSONEq(t, `{"to":"user@example.com"}`, string(processed["emails"]))
}

// TestWorker_FailingJobDeadLetters verifies the retry ladder end to end through
// the worker: a handler that always fails exhausts its retries and lands in
// dead_letter, where the admin routes can see and retry it.
func TestWorker_FailingJobDeadLetters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := NewJobQueue(nil)
	w := NewWorker(jq, 10*time.Millisecond)

	w.RegisterHandler("tasks", func(ctx context.Context, payload json.RawMessage) error {
		return errors.New("boom")
	})
	w.Start(ctx)
	defer w.Stop()

	// maxRetries=1: the first failure immediately dead-letters (no backoff),
	// keeping the test fast. The backoff ladder is covered in jobqueue_test.go.
	job, err := jq.Enqueue(ctx, "tasks", "data", 1)
	require.NoError(t, err)

	waitFor(t, 2*time.Second, func() bool {
		status, _ := jobStatus(jq, job.ID)
		return status == StatusDeadLetter
	}, "job to reach dead_letter")

	dead, err := jq.GetDeadLetterJobs(ctx)
	require.NoError(t, err)
	require.Len(t, dead, 1)
	assert.Equal(t, job.ID, dead[0].ID)
	assert.True(t, dead[0].LastError.Valid)
	assert.Equal(t, "boom", dead[0].LastError.String)

	// Admin retry moves it back to pending; the worker picks it up again.
	err = jq.RetryDeadLetterJob(ctx, job.ID)
	require.NoError(t, err)

	waitFor(t, 2*time.Second, func() bool {
		status, retries := jobStatus(jq, job.ID)
		return status == StatusDeadLetter && retries == 1
	}, "retried job to fail again into dead_letter")
}

// TestWorker_MultipleQueuesIndependent verifies that one slow/failing queue
// does not block processing of another.
func TestWorker_MultipleQueuesIndependent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jq := NewJobQueue(nil)
	w := NewWorker(jq, 10*time.Millisecond)

	var mu sync.Mutex
	done := false

	w.RegisterHandler("fast", func(ctx context.Context, payload json.RawMessage) error {
		mu.Lock()
		done = true
		mu.Unlock()
		return nil
	})
	w.RegisterHandler("slow", func(ctx context.Context, payload json.RawMessage) error {
		time.Sleep(200 * time.Millisecond)
		return errors.New("eventual failure")
	})
	w.Start(ctx)
	defer w.Stop()

	_, err := jq.Enqueue(ctx, "slow", "data", 1)
	require.NoError(t, err)
	_, err = jq.Enqueue(ctx, "fast", "data", 1)
	require.NoError(t, err)

	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return done
	}, "fast queue job to complete")
}
