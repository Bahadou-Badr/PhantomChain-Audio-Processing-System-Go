package server

import (
	"context"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/api"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/logging"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/metrics"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

var healthy int32 = 1

// APIServerConfig - configure the in-process API server used by tests.
type APIServerConfig struct {
	Addr        string // e.g. "127.0.0.1:8085"
	DatabaseDSN string // postgres DSN, will be set into env for db.New
	StorageBase string // local path for storage
	NatsURL     string // optional nats url
	DevLogging  bool   // true -> dev logger
}

// RunAPIServer starts the API server in-process. Caller must cancel ctx or call srv.Shutdown.
func RunAPIServer(ctx context.Context, cfg APIServerConfig) (*http.Server, error) {
	// ensure DSN available to db.New (which reads DATABASE_URL)
	if cfg.DatabaseDSN != "" {
		_ = os.Setenv("DATABASE_URL", cfg.DatabaseDSN)
	}

	// init logging
	if err := logging.Init(cfg.DevLogging); err != nil {
		return nil, err
	}
	// ensure logger sync on exit of this function goroutine (Shutdown will call Sync too)
	// (Do not call Sync here because server stays running; main/test will exit)
	logging.Logger.Info("server.RunAPIServer starting", zap.String("addr", cfg.Addr))

	// register metrics
	metrics.Register()

	// init DB
	dbConn, err := db.New(ctx)
	if err != nil {
		logging.Logger.Error("db.New failed", zap.Error(err))
		return nil, err
	}
	// do NOT close dbConn here; will be closed on context cancel after shutdown below

	// init storage (local dev)
	base := cfg.StorageBase
	if base == "" {
		base = "./data"
	}
	lf := storage.NewLocalFS(base)
	if err := lf.EnsureBasePath(base); err != nil {
		logging.Logger.Error("EnsureBasePath failed", zap.Error(err))
		dbConn.Close()
		return nil, err
	}

	// optional nats client
	var nClient *queue.NatsClient
	if cfg.NatsURL != "" {
		nc, err := queue.NewNatsClient(cfg.NatsURL)
		if err != nil {
			logging.Logger.Error("NewNatsClient failed", zap.Error(err))
			dbConn.Close()
			return nil, err
		}
		nClient = nc
	}

	// build API service and router
	apiSvc := &api.API{
		DB:      dbConn,
		Storage: lf,
		Queue:   nClient,
	}

	r := chi.NewRouter()
	r.Get("/health", healthHandler) // if you exported them; otherwise use inline handlers
	r.Get("/ready", readyHandler)
	r.Post("/upload", apiSvc.UploadHandler)
	apiSvc.RegisterJobRoutes(r)

	// metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// graceful shutdown when ctx canceled: shutdown server, close db and nats
	go func() {
		<-ctx.Done()
		logging.Logger.Info("server.RunAPIServer shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		dbConn.Close()
		if nClient != nil {
			nClient.Close()
		}
	}()

	// run server in goroutine (non-blocking)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Logger.Error("ListenAndServe error", zap.Error(err))
		}
	}()

	return srv, nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	if atomic.LoadInt32(&healthy) == 1 {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ready":true}`))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte(`{"ready":false}`))
}
