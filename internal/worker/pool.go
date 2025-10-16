package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/logging"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/metrics"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"
	"go.uber.org/zap"
)

// Handler defines the signature of a job processor.
type Handler func(ctx context.Context, jm queue.JobMessage) error

// Pool is a bounded worker pool that executes jobs concurrently with retry & backoff.
type Pool struct {
	db             *db.DB
	concurrency    int
	jobs           chan queue.JobMessage
	wg             sync.WaitGroup
	handler        Handler
	retryBaseDelay time.Duration
	maxRetries     int
}

// NewPool creates a worker pool with bounded concurrency and internal queue.
func NewPool(database *db.DB, concurrency int, queueSize int, handler Handler) *Pool {
	return &Pool{
		db:             database,
		concurrency:    concurrency,
		jobs:           make(chan queue.JobMessage, queueSize),
		handler:        handler,
		retryBaseDelay: 2 * time.Second,
		maxRetries:     3,
	}
}

// Start launches all worker goroutines.
func (p *Pool) Start(ctx context.Context) {
	logging.Logger.Info("starting worker pool", zap.Int("concurrency", p.concurrency))
	for i := 0; i < p.concurrency; i++ {
		p.wg.Add(1)
		go p.workerLoop(ctx, i)
	}
}

// Stop gracefully stops all workers and waits until they finish.
func (p *Pool) Stop() {
	logging.Logger.Info("stopping worker pool")
	close(p.jobs)
	p.wg.Wait()
}

// Enqueue pushes a job into the queue (non-blocking; returns error if full).
func (p *Pool) Enqueue(j queue.JobMessage) error {
	select {
	case p.jobs <- j:
		return nil
	default:
		return fmt.Errorf("job queue full")
	}
}

// workerLoop consumes jobs and executes them with retry and backoff.
func (p *Pool) workerLoop(ctx context.Context, id int) {
	defer p.wg.Done()
	for jm := range p.jobs {
		// Claim job atomically to avoid duplicates
		claimed, err := p.db.TryClaimJob(ctx, jm.JobID, fmt.Sprintf("worker-%d", id))
		if err != nil {
			_ = p.db.UpdateJobStatus(ctx, jm.JobID, "queued", 0, "claim error: "+err.Error())
			logging.Logger.Error("claim error", zap.Int64("job", jm.JobID), zap.Error(err))
			continue
		}
		if !claimed {
			continue // another worker already took it
		}

		// Instrument: increment gauges/counters
		metrics.CurrentJobs.Inc()
		start := time.Now()
		logging.Logger.Info("processing job", zap.Int64("job", jm.JobID), zap.Int("worker", id))

		jobCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		err = p.handler(jobCtx, jm)
		cancel()

		duration := time.Since(start).Seconds()
		metrics.JobDuration.WithLabelValues(jm.Type).Observe(duration)

		if err != nil {
			metrics.JobFailures.Inc()
			metrics.JobsProcessed.WithLabelValues("failed", jm.Type).Inc()
			metrics.CurrentJobs.Dec()
			// Record failure details
			_, _ = p.db.Pool.Exec(ctx,
				`UPDATE jobs SET retry_count = retry_count + 1, last_error = $1 WHERE id=$2`,
				err.Error(), jm.JobID)

			var retryCount, maxRetries int
			_ = p.db.Pool.QueryRow(ctx,
				`SELECT retry_count, max_retries FROM jobs WHERE id=$1`,
				jm.JobID).Scan(&retryCount, &maxRetries)

			if retryCount >= maxRetries {
				_ = p.db.UpdateJobStatus(ctx, jm.JobID, "failed", 0,
					fmt.Sprintf("job failed after %d retries: %s", retryCount, err.Error()))
				logging.Logger.Error("job failed permanently", zap.Int64("job", jm.JobID), zap.Int("retries", retryCount))
				continue
			}

			// Exponential backoff
			backoff := p.retryBaseDelay * time.Duration(1<<uint(retryCount-1))
			_, _ = p.db.Pool.Exec(ctx, `UPDATE jobs SET status='queued' WHERE id=$1`, jm.JobID)

			go func(jm queue.JobMessage, delay time.Duration) {
				time.Sleep(delay)
				_ = p.Enqueue(jm)
			}(jm, backoff)

			logging.Logger.Warn("job failed, will retry", zap.Int64("job", jm.JobID), zap.Int("retry", retryCount), zap.Error(err))
			continue
		}

		// Success
		metrics.JobsProcessed.WithLabelValues("done", jm.Type).Inc()
		metrics.CurrentJobs.Dec()
		_ = p.db.UpdateJobStatus(ctx, jm.JobID, "done", 100,
			fmt.Sprintf("completed in %s", time.Since(start)))
		logging.Logger.Info("job completed", zap.Int64("job", jm.JobID), zap.Float64("duration_s", duration))
	}
}
