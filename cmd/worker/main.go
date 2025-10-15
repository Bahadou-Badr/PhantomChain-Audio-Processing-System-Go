package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/audio"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

var running int32 = 1

func main() {
	ctx := context.Background()

	// DB connect (reuse db.New)
	database, err := db.New(ctx)
	if err != nil {
		log.Fatal().Err(err).Msg("db connect failed")
	}
	defer database.Close()

	// NATS client
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}
	nc, err := queue.NewNatsClient(natsURL)
	if err != nil {
		log.Fatal().Err(err).Msg("nats connect failed")
	}
	defer nc.Close()

	// subscribe to subject "jobs" with queue group "audio-workers"
	subj := "jobs"
	queueGroup := "audio-workers"

	sub, err := nc.QueueSubscribe(subj, queueGroup, func(m *nats.Msg) {
		var jm queue.JobMessage
		if err := json.Unmarshal(m.Data, &jm); err != nil {
			log.Error().Err(err).Msg("invalid job message")
			return
		}
		go handleJob(context.Background(), database, jm) // process concurrently
	})
	if err != nil {
		log.Fatal().Err(err).Msg("subscribe failed")
	}
	defer sub.Unsubscribe()

	// graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	log.Info().Msg("shutdown requested")
	atomic.StoreInt32(&running, 0)
	time.Sleep(1 * time.Second) // wait for current go routines or handle elegantly
	log.Info().Msg("worker stopped")
}

func handleJob(ctx context.Context, db *db.DB, jm queue.JobMessage) {
	jobID := jm.JobID
	uploadID := jm.UploadID

	_ = db.UpdateJobStatus(ctx, jobID, "running", 1, "worker: started")

	// fetch upload path
	var relPath string
	if err := db.Pool.QueryRow(ctx, `SELECT path FROM uploads WHERE id=$1`, uploadID).Scan(&relPath); err != nil {
		_ = db.UpdateJobStatus(ctx, jobID, "failed", 0, "upload not found")
		return
	}

	base := "./data" // must match your storage base
	inputFull := filepath.Join(base, relPath)

	// 1) Probe
	info, err := audio.Probe(ctx, inputFull)
	if err != nil {
		_ = db.UpdateJobStatus(ctx, jobID, "failed", 0, "probe failed: "+err.Error())
		return
	}
	// update duration in uploads table
	_, _ = db.Pool.Exec(ctx, `UPDATE uploads SET duration_seconds=$1 WHERE id=$2`, info.Duration.Seconds(), uploadID)
	_ = db.UpdateJobStatus(ctx, jobID, "processing", 10, fmt.Sprintf("probe ok (dur=%v)", info.Duration))

	// 2) Transcode -> create output path
	outputRel := relPath + ".mp3" // simple suffix; adjust later
	outputFull := filepath.Join(base, outputRel)

	// ensure dir exists
	if err := os.MkdirAll(filepath.Dir(outputFull), 0o755); err != nil {
		_ = db.UpdateJobStatus(ctx, jobID, "failed", 0, "mkdir failed: "+err.Error())
		return
	}

	// Transcode with timeout context
	trCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := audio.Transcode(trCtx, inputFull, outputFull); err != nil {
		_ = db.UpdateJobStatus(ctx, jobID, "failed", 0, "transcode failed: "+err.Error())
		return
	}
	_ = db.UpdateJobStatus(ctx, jobID, "processing", 60, "transcode done")

	// 1) LUFS (you may already compute it)
	lufs, err := audio.Loudness(ctx, outputFull)
	if err == nil {
		_, _ = db.Pool.Exec(ctx, `UPDATE uploads SET integrated_lufs=$1 WHERE id=$2`, lufs, jm.UploadID)
		_ = db.UpdateJobStatus(ctx, jm.JobID, "processing", 75, fmt.Sprintf("loudness=%.2f", lufs))
	}

	// 2) BPM + Key via python analyzer
	// set python executable path and script path (config or env)
	pythonExe := "python" // or full path like ".\\.venv\\Scripts\\python.exe"
	scriptPath := "./tools/analyze.py"
	analysisCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	res, err := audio.AnalyzeWithPython(analysisCtx, pythonExe, scriptPath, outputFull, 60*time.Second)
	if err == nil && res != nil {
		_, _ = db.Pool.Exec(ctx, `UPDATE uploads SET bpm=$1, musical_key=$2 WHERE id=$3`, res.BPM, res.Key, jm.UploadID)
		_ = db.UpdateJobStatus(ctx, jm.JobID, "processing", 90, fmt.Sprintf("bpm=%.2f key=%s", res.BPM, res.Key))
	} else {
		_ = db.UpdateJobStatus(ctx, jm.JobID, "processing", 90, "analysis failed: "+err.Error())
	}

	// 3) Loudness analysis
	if err == nil {
		// save to uploads table
		_, _ = db.Pool.Exec(ctx, `UPDATE uploads SET output_path=$1, integrated_lufs=$2 WHERE id=$3`, outputRel, lufs, uploadID)
		_ = db.UpdateJobStatus(ctx, jobID, "processing", 80, fmt.Sprintf("loudness=%.2f LUFS", lufs))
	} else {
		// not fatal — record absence
		_, _ = db.Pool.Exec(ctx, `UPDATE uploads SET output_path=$1 WHERE id=$2`, outputRel, uploadID)
		_ = db.UpdateJobStatus(ctx, jobID, "processing", 80, "loudness analysis failed: "+err.Error())
	}

	// 4) Waveform generation (PNG)
	wavePath := strings.TrimSuffix(outputRel, filepath.Ext(outputRel)) + "-wave.png"
	waveFull := filepath.Join(base, wavePath)
	if err := audio.GenerateWaveform(ctx, outputFull, waveFull, 800, 160); err == nil {
		// optionally save path to DB, you can create column for waveform if needed
		_, _ = db.Pool.Exec(ctx, `UPDATE uploads SET output_path=$1 WHERE id=$2`, outputRel, uploadID)
		_ = db.UpdateJobStatus(ctx, jobID, "processing", 95, "waveform generated")
	} else {
		_ = db.UpdateJobStatus(ctx, jobID, "processing", 95, "waveform failed: "+err.Error())
	}

	// 5) finish
	_ = db.UpdateJobStatus(ctx, jobID, "done", 100, "processing finished")
}
