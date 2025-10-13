package api

// "context"
import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func (a *API) RegisterJobRoutes(r chi.Router) {
	r.Get("/jobs", a.ListJobsHandler)
	r.Get("/jobs/{id}", a.GetJobHandler)
	r.Patch("/jobs/{id}", a.UpdateJobHandler) // e.g., update status/progress
	r.Get("/uploads/{id}", a.GetUploadHandler)
}

func (a *API) ListJobsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := 50
	offset := 0
	jobs, err := a.DB.ListJobs(ctx, limit, offset)
	if err != nil {
		log.Error().Err(err).Msg("list jobs failed")
		http.Error(w, "list jobs failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, jobs)
}

func (a *API) GetJobHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	j, err := a.DB.GetJob(ctx, id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	writeJSON(w, j)
}

type updateJobReq struct {
	Status   string `json:"status,omitempty"`
	Progress *int   `json:"progress,omitempty"`
	Log      string `json:"log,omitempty"`
}

func (a *API) UpdateJobHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var req updateJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	progress := 0
	if req.Progress != nil {
		progress = *req.Progress
	}
	if req.Status == "" {
		req.Status = "running"
	}
	if err := a.DB.UpdateJobStatus(ctx, id, req.Status, progress, req.Log); err != nil {
		log.Error().Err(err).Msg("update job failed")
		http.Error(w, "update failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) GetUploadHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	row := a.DB.Pool.QueryRow(ctx, `SELECT id, filename, path, content_type, size, status, created_at FROM uploads WHERE id=$1`, id)
	var idOut int64
	var filename, path, contentType, status string
	var size int64
	var createdAt string
	if err := row.Scan(&idOut, &filename, &path, &contentType, &size, &status, &createdAt); err != nil {
		http.Error(w, "upload not found", http.StatusNotFound)
		return
	}
	resp := map[string]interface{}{
		"id":           idOut,
		"filename":     filename,
		"path":         path,
		"content_type": contentType,
		"size":         size,
		"status":       status,
		"created_at":   createdAt,
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
