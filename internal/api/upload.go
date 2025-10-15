package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/storage"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/pkg/utils"
	"github.com/go-chi/chi/v5"

	"github.com/rs/zerolog/log"
)

type API struct {
	DB      *db.DB
	Storage storage.Storage
	Queue   *queue.NatsClient
}

type uploadResponse struct {
	UploadID int64  `json:"upload_id"`
	Status   string `json:"status"`
	Path     string `json:"path"`
}

func (a *API) UploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// 1. Parse multipart form (limit size e.g., 100MB)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "field 'file' is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	dest := storage.BuildPath(filename)

	// Save file via storage
	n, err := a.Storage.Save(file, dest)
	if err != nil {
		log.Error().Err(err).Msg("save file failed")
		http.Error(w, "failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Persist record in DB
	var uploadID int64
	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO uploads (filename, path, content_type, size) VALUES ($1,$2,$3,$4) RETURNING id`,
		filename, dest, header.Header.Get("Content-Type"), n,
	).Scan(&uploadID)

	if err != nil {
		log.Error().Err(err).Msg("db insert failed")
		http.Error(w, "db insert failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// create a job record (queued) - basic
	var jobID int64
	err = a.DB.Pool.QueryRow(ctx,
		`INSERT INTO jobs (upload_id, type, status) VALUES ($1,$2,'queued') RETURNING id`,
		uploadID, "transcode",
	).Scan(&jobID)
	if err != nil {
		// not fatal: still return upload id
		log.Error().Err(err).Msg("job insert failed")
	}

	// publish job message (non-blocking)
	if a.Queue != nil {
		jm := queue.JobMessage{
			JobID:    jobID,
			UploadID: uploadID,
			Type:     "transcode",
		}
		// context.Background() used for quick publish; you can pass r.Context()
		if err := a.Queue.PublishJob(r.Context(), "jobs", jm); err != nil {
			log.Error().Err(err).Msg("failed to publish job to nats")
		} else {
			log.Info().Int64("job", jobID).Msg("published job to nats")
		}
	}

	resp := uploadResponse{
		UploadID: uploadID,
		Status:   "queued",
		Path:     dest,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)

	// optional: log
	log.Info().Str("path", dest).Int64("size", n).Msgf("uploaded file id=%d job=%d", uploadID, jobID)
}

func (a *API) GetUploadAnalysisHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	row := a.DB.Pool.QueryRow(ctx, `SELECT duration_seconds, integrated_lufs, bpm, musical_key, output_path FROM uploads WHERE id=$1`, id)
	var dur sql.NullFloat64
	var lufs sql.NullFloat64
	var bpm sql.NullFloat64
	var key sql.NullString
	var out sql.NullString
	if err := row.Scan(&dur, &lufs, &bpm, &key, &out); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	resp := map[string]interface{}{
		"duration_seconds": utils.NilIfNullFloat(dur),
		"integrated_lufs":  utils.NilIfNullFloat(lufs),
		"bpm":              utils.NilIfNullFloat(bpm),
		"musical_key":      utils.NilIfNullString(key),
		"output_path":      utils.NilIfNullString(out),
	}
	writeJSON(w, resp)
}
