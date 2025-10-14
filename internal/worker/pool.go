package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"
)

// Handler is the function that processes a job; it must respect ctx cancellation.
type Handler func(ctx context.Context, jm queue.JobMessage) error

type Pool struct {
	db          *db.DB
	concurrency int
	jobs        chan queue.JobMessage
	wg          sync.WaitGroup
	handler     Handler

	// controls
	retryBaseDelay time.Duration
	maxRetries     int
}

// NewPool creates a bounded worker pool.
func NewPool(db *db.DB, concurrency int, queueSize int, handler Handler) *Pool {
	return &Pool{
		db:             db,
		concurrency:    concurrency,
		jobs:           make(chan queue.JobMessage, queueSize),
		handler:        handler,
		retryBaseDelay: 2 * time.Second,
		maxRetries:     3,
	}
}

// Start launches worker goroutines that will consume jobs channel.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.workerLoop(ctx, i)
	}
}

// Stop waits for workers to finish (jobs channel should be closed by caller)
func (p *Pool) Stop() {
	close(p.jobs)
	p.wg.Wait()
}

// Enqueue pushes a job into the queue (non-blocking with drop policy returning error)
func (p *Pool) Enqueue(j queue.JobMessage) error {
	select {
	case p.jobs <- j:
		return nil
	default:
		return fmt.Errorf("job queue full")
	}
}

func (p *Pool) workerLoop(ctx context.Context, id int) {
	defer p.wg.Done()
	for jm := range p.jobs {
		// For each job, try to claim atomically
		claimed, err := p.db.TryClaimJob(ctx, jm.JobID, fmt.Sprintf("worker-%d", id))
		if err != nil {
			// database error; optionally sleep and retry
			_ = p.db.UpdateJobStatus(ctx, jm.JobID, "queued", 0, "claim-db-error:"+err.Error())
			continue
		}
		if !claimed {
			// not ours or already being processed
			continue
		}

		// Build job-specific context with cancellation and time limit
		jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute) // tune timeout
		start := time.Now()
		err = p.handler(jobCtx, jm)
		cancel()

		if err != nil {
			// increment retry_count, set last_error
			_, _ = p.db.Pool.Exec(ctx, `UPDATE jobs SET retry_count = retry_count + 1, last_error = $1 WHERE id=$2`, err.Error(), jm.JobID)

			// fetch current retry_count, max_retries
			var retryCount, maxRetries int
			_ = p.db.Pool.QueryRow(ctx, `SELECT retry_count, max_retries FROM jobs WHERE id=$1`, jm.JobID).Scan(&retryCount, &maxRetries)

			if retryCount >= maxRetries {
				// mark as failed / dead-letter
				_ = p.db.UpdateJobStatus(ctx, jm.JobID, "failed", 0, fmt.Sprintf("job failed after %d retries: %s", retryCount, err.Error()))
			} else {
				// schedule requeue after backoff (simple sleep here)
				backoff := p.retryBaseDelay * time.Duration(1<<uint(retryCount-1)) // exp backoff
				// set status back to queued so another worker can pick it later
				_, _ = p.db.Pool.Exec(ctx, `UPDATE jobs SET status='queued' WHERE id=$1`, jm.JobID)

				// crude re-enqueue: sleep then enqueue (in goroutine so worker loop continues)
				go func(jm queue.JobMessage, delay time.Duration) {
					time.Sleep(delay)
					_ = p.Enqueue(jm)
				}(jm, backoff)
			}
			continue
		}

		// success: mark done (handler should update progress/logs too)
		_ = p.db.UpdateJobStatus(ctx, jm.JobID, "done", 100, fmt.Sprintf("completed in %s", time.Since(start)))
	}
}
