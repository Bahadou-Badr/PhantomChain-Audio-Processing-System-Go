package db

import (
	"context"
	"time"
)

type JobModel struct {
	ID        int64     `db:"id"`
	UploadID  int64     `db:"upload_id"`
	Type      string    `db:"type"`
	Status    string    `db:"status"`
	Progress  int       `db:"progress"`
	Logs      string    `db:"logs"`
	CreatedAt time.Time `db:"created_at"`
}

func (d *DB) CreateJob(ctx context.Context, uploadID int64, jtype string) (int64, error) {
	var id int64
	err := d.Pool.QueryRow(ctx,
		`INSERT INTO jobs (upload_id, type, status) VALUES ($1,$2,'queued') RETURNING id`,
		uploadID, jtype).Scan(&id)
	return id, err
}

func (d *DB) GetJob(ctx context.Context, id int64) (*JobModel, error) {
	j := &JobModel{}
	row := d.Pool.QueryRow(ctx, `SELECT id, upload_id, type, status, progress, logs, created_at FROM jobs WHERE id=$1`, id)
	if err := row.Scan(&j.ID, &j.UploadID, &j.Type, &j.Status, &j.Progress, &j.Logs, &j.CreatedAt); err != nil {
		return nil, err
	}
	return j, nil
}

func (d *DB) ListJobs(ctx context.Context, limit, offset int) ([]*JobModel, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id, upload_id, type, status, progress, logs, created_at FROM jobs ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*JobModel
	for rows.Next() {
		j := &JobModel{}
		if err := rows.Scan(&j.ID, &j.UploadID, &j.Type, &j.Status, &j.Progress, &j.Logs, &j.CreatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (d *DB) UpdateJobStatus(ctx context.Context, id int64, status string, progress int, appendLog string) error {
	// Append log if provided
	if appendLog != "" {
		_, err := d.Pool.Exec(ctx, `UPDATE jobs SET status=$1, progress=$2, logs = COALESCE(logs,'') || E'\n' || $3 WHERE id=$4`, status, progress, appendLog, id)
		return err
	}
	_, err := d.Pool.Exec(ctx, `UPDATE jobs SET status=$1, progress=$2 WHERE id=$3`, status, progress, id)
	return err
}
