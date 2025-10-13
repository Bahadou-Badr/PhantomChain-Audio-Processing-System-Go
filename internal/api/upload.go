package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/storage"

	"github.com/rs/zerolog/log"
)

type API struct {
	DB      *db.DB
	Storage storage.Storage
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
