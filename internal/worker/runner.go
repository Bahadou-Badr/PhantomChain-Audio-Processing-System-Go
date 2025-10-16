package worker

import (
	"context"
	"encoding/json"
	"os"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/logging"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/metrics"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"
	"go.uber.org/zap"

	"github.com/nats-io/nats.go"
)

type WorkerConfig struct {
	DatabaseDSN string
	NatsURL     string
	Concurrency int
	QueueSize   int
	DevLogging  bool
}

// RunWorker starts a worker pool and subscribes to NATS subject "jobs".
// handler is the function executed for each job (it must respect ctx cancellation).
// Caller should cancel ctx to stop the worker.
func RunWorker(ctx context.Context, cfg WorkerConfig, handler func(ctx context.Context, jm queue.JobMessage) error) (*Pool, error) {
	// set DB env if provided
	if cfg.DatabaseDSN != "" {
		_ = os.Setenv("DATABASE_URL", cfg.DatabaseDSN)
	}

	// init logging (idempotent)
	if err := logging.Init(cfg.DevLogging); err != nil {
		return nil, err
	}

	metrics.Register()

	// init db
	database, err := db.New(ctx)
	if err != nil {
		logging.Logger.Error("db.New failed", zap.Error(err))
		return nil, err
	}

	// nats client
	natsURL := cfg.NatsURL
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}
	nc, err := queue.NewNatsClient(natsURL)
	if err != nil {
		database.Close()
		return nil, err
	}

	// construct pool (use handler signature expected by this package)
	p := NewPool(database, cfg.Concurrency, cfg.QueueSize, handler)
	p.Start(ctx)

	// subscribe to jobs subject and push messages into pool
	sub, err := nc.QueueSubscribe("jobs", "audio-workers", func(m *nats.Msg) {
		var jm queue.JobMessage
		if err := json.Unmarshal(m.Data, &jm); err != nil {
			logging.Logger.Error("bad job message", zap.Error(err))
			return
		}
		if err := p.Enqueue(jm); err != nil {
			logging.Logger.Warn("enqueue failed", zap.Error(err))
		}
	})
	if err != nil {
		// cleanup
		p.Stop()
		database.Close()
		nc.Close()
		return nil, err
	}

	// cleanup on context cancellation
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
		p.Stop()
		nc.Close()
		database.Close()
	}()

	return p, nil
}
