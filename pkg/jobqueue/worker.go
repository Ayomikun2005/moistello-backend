package jobqueue

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// JobHandler processes a single dequeued job payload. Returning an error moves
// the job through the retry/dead-letter ladder (see JobQueue.Fail).
type JobHandler func(ctx context.Context, payload json.RawMessage) error

// Worker polls registered queues and dispatches dequeued jobs to their
// handlers. It is the runtime half of the admin job-queue: pkg/jobqueue holds
// the storage, Worker holds the execution loop. Issue #162 wired this into the
// API server (cmd/api-server/main.go) so background work has a place to run
// instead of living as inline stub routes.
type Worker struct {
	queue    *JobQueue
	handlers map[string]JobHandler
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewWorker creates a worker that polls every queue with a registered handler
// on the given interval.
func NewWorker(queue *JobQueue, pollInterval time.Duration) *Worker {
	return &Worker{
		queue:    queue,
		handlers: make(map[string]JobHandler),
		interval: pollInterval,
		stopCh:   make(chan struct{}),
	}
}

// RegisterHandler binds a handler to a queue name. Only queues with a
// registered handler are polled; a job enqueued to a queue with no handler
// stays pending until a handler is registered (or an operator moves it).
func (w *Worker) RegisterHandler(queueName string, h JobHandler) {
	w.handlers[queueName] = h
}

// Start launches the poll loop in the background. The loop stops when ctx is
// cancelled or Stop is called.
func (w *Worker) Start(ctx context.Context) {
	w.wg.Add(1)
	go w.run(ctx)
}

// Stop signals the worker to stop after the current poll cycle completes.
func (w *Worker) Stop() {
	close(w.stopCh)
	w.wg.Wait()
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	log.Info().Dur("interval", w.interval).Msg("job queue worker started")

	for {
		select {
		case <-ticker.C:
			w.processQueues(ctx)
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		}
	}
}

// processQueues drains every registered queue once: dequeue, dispatch, and
// mark complete/failed per job.
func (w *Worker) processQueues(ctx context.Context) {
	for queueName, handler := range w.handlers {
		if err := w.drainQueue(ctx, queueName, handler); err != nil {
			log.Error().Err(err).Str("queue", queueName).Msg("job queue drain failed")
		}
	}
}

func (w *Worker) drainQueue(ctx context.Context, queueName string, handler JobHandler) error {
	for {
		job, err := w.queue.Dequeue(ctx, queueName)
		if err != nil {
			return err
		}
		if job == nil {
			return nil // queue drained
		}
		w.process(ctx, job, handler)
	}
}

func (w *Worker) process(ctx context.Context, job *Job, handler JobHandler) {
	if err := handler(ctx, job.Payload); err != nil {
		log.Error().Err(err).Str("job", job.ID).Str("queue", job.QueueName).Msg("job handler failed")
		if err := w.queue.Fail(ctx, job.ID, err); err != nil {
		log.Error().Err(err).Str("job", job.ID).Msg("failed to record job failure")
		}
		return
	}

	if err := w.queue.Complete(ctx, job.ID); err != nil {
		log.Error().Err(err).Str("job", job.ID).Msg("failed to mark job completed")
	}
}
