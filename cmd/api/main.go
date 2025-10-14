package main

//"log"

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/api"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/storage"
	"github.com/nats-io/nats.go"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var healthy int32 = 1

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	ctx := context.Background()
	database, err := db.New(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect db")
	}
	defer database.Close()

	// storage base path
	base := os.Getenv("STORAGE_PATH")
	if base == "" {
		base = "./data"
	}
	lf := storage.NewLocalFS(base)
	if err := lf.EnsureBasePath(base); err != nil {
		log.Fatal().Err(err).Msg("failed to create storage base path")
	}

	//Initialize NATS client
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL // "nats://127.0.0.1:4222"
	}
	nClient, err := queue.NewNatsClient(natsURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect nats")
	}
	defer nClient.Close()

	apiSvc := &api.API{
		DB:      database,
		Storage: lf,
		Queue:   nClient,
	}

	r := chi.NewRouter()
	r.Get("/health", healthHandler)
	r.Get("/ready", readyHandler)
	r.Post("/upload", apiSvc.UploadHandler)

	r.Route("/api", func(r chi.Router) {
		apiSvc.RegisterJobRoutes(r)
	})

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// graceful shutdown
	idleConnsClosed := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh

		log.Info().Msg("shutdown signal received")
		atomic.StoreInt32(&healthy, 0) // mark unhealthy

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("server shutdown error")
		}
		close(idleConnsClosed)
	}()

	log.Info().Msg("api server starting on :8080")
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("listen error")
	}
	<-idleConnsClosed
	log.Info().Msg("server stopped")
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
