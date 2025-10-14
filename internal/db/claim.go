package db

import (
	"context"
	"fmt"
	"time"
)

// TryClaimJob attempts to transition a queued job to running and returns true if claimed.
func (d *DB) TryClaimJob(ctx context.Context, jobID int64, workerName string) (bool, error) {
	// We change status only if currently queued
	// Also set updated timestamp or last_error cleared
	tag, err := d.Pool.Exec(ctx,
		`UPDATE jobs
		 SET status='running', last_error='', progress=1
		 WHERE id=$1 AND status='queued'`, jobID)
	if err != nil {
		return false, fmt.Errorf("claim update error: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// not claimed (already running or done)
		return false, nil
	}
	// optionally insert an audit row in another table here
	_ = time.Now()
	return true, nil
}
