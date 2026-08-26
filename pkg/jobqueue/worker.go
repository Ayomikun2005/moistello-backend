package jobqueue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// JobHandler is a function that processes a single dequeued job.
type JobHandler func(ctx context.Context, job *Job) error

// WorkerOptions configures the worker consumer.
type WorkerOptions struct {
	Concurrency  int           // Number of concurrent worker goroutines (default: 5)
	PollInterval time.Duration // Interval to wait when no jobs are available (default: 500ms)
	Queues       []string      // Queue names to poll from (default: ["default"])
	MaxRetries   int           // Default max retries for failed jobs (default: 3)
}

// Worker is a concurrent background worker that consumes and processes jobs from the JobQueue.
type Worker struct {
	queue      *JobQueue
	options    WorkerOptions
	handlers   map[string]JobHandler
	handlersMu sync.RWMutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
	running    int32
	mu         sync.Mutex
}

// NewWorker initializes a new job consumer worker with the given options.
func NewWorker(jq *JobQueue, opts WorkerOptions) *Worker {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 5
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = 500 * time.Millisecond
	}
	if len(opts.Queues) == 0 {
		opts.Queues = []string{"default"}
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 3
	}

	return &Worker{
		queue:    jq,
		options:  opts,
		handlers: make(map[string]JobHandler),
		stopCh:   make(chan struct{}),
	}
}

// RegisterHandler registers a processing handler for a specific queue name.
func (w *Worker) RegisterHandler(queueName string, handler JobHandler) {
	w.handlersMu.Lock()
	defer w.handlersMu.Unlock()
	w.handlers[queueName] = handler
}

// Start launches the consumer worker pool in background goroutines.
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if atomic.LoadInt32(&w.running) == 1 {
		return errors.New("worker is already running")
	}

	w.stopCh = make(chan struct{})
	atomic.StoreInt32(&w.running, 1)

	log.Info().
		Int("concurrency", w.options.Concurrency).
		Strs("queues", w.options.Queues).
		Dur("poll_interval", w.options.PollInterval).
		Msg("starting job queue consumer worker")

	for i := 0; i < w.options.Concurrency; i++ {
		w.wg.Add(1)
		go w.workerLoop(ctx, i)
	}

	return nil
}

// Stop gracefully signals all worker routines to stop and waits for active jobs to finish.
func (w *Worker) Stop() {
	w.mu.Lock()
	if atomic.LoadInt32(&w.running) == 0 {
		w.mu.Unlock()
		return
	}
	atomic.StoreInt32(&w.running, 0)
	close(w.stopCh)
	w.mu.Unlock()

	log.Info().Msg("stopping job queue consumer worker...")
	w.wg.Wait()
	log.Info().Msg("job queue consumer worker stopped")
}

// IsRunning returns whether the worker is currently running.
func (w *Worker) IsRunning() bool {
	return atomic.LoadInt32(&w.running) == 1
}

func (w *Worker) workerLoop(ctx context.Context, workerID int) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		processedAny := false

		for _, qName := range w.options.Queues {
			select {
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			default:
			}

			job, err := w.queue.Dequeue(ctx, qName)
			if err != nil {
				log.Warn().Err(err).Str("queue", qName).Int("worker", workerID).Msg("error dequeuing job")
				continue
			}

			if job != nil {
				processedAny = true
				w.processJob(ctx, job)
			}
		}

		if !processedAny {
			select {
			case <-ctx.Done():
				return
			case <-w.stopCh:
				return
			case <-time.After(w.options.PollInterval):
			}
		}
	}
}

func (w *Worker) processJob(ctx context.Context, job *Job) {
	w.handlersMu.RLock()
	handler, exists := w.handlers[job.QueueName]
	w.handlersMu.RUnlock()

	if !exists {
		// Default handler if none registered: log warning and mark complete
		log.Warn().
			Str("job_id", job.ID).
			Str("queue", job.QueueName).
			Msg("no handler registered for job queue; completing job")
		_ = w.queue.Complete(ctx, job.ID)
		return
	}

	execErr := handler(ctx, job)
	if execErr != nil {
		log.Error().
			Err(execErr).
			Str("job_id", job.ID).
			Str("queue", job.QueueName).
			Int("retries", job.RetriesCount+1).
			Int("max_retries", job.MaxRetries).
			Msg("job execution failed")

		if failErr := w.queue.Fail(ctx, job.ID, execErr); failErr != nil {
			log.Error().Err(failErr).Str("job_id", job.ID).Msg("failed to update job failure status")
		}
		return
	}

	if completeErr := w.queue.Complete(ctx, job.ID); completeErr != nil {
		log.Error().Err(completeErr).Str("job_id", job.ID).Msg("failed to mark job completed")
	}
}
