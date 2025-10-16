package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	q "github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/queue"
	serverpkg "github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/server"
	workerpkg "github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/worker"
	_ "github.com/lib/pq"

	"github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/internal/db"
	nat "github.com/docker/go-connections/nat" // put this in imports at file top
	"github.com/stretchr/testify/require"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestE2E_UploadAndProcess_WithTestcontainers(t *testing.T) {
	ctx := context.Background()

	// -------------------------------
	// 1) Start Postgres container
	//---------------------------

	pgReq := testcontainers.ContainerRequest{
		Image: "postgres:15",
		Env: map[string]string{
			"POSTGRES_DB":       "goaudio",
			"POSTGRES_USER":     "postgres",
			"POSTGRES_PASSWORD": "postgres",
		},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor: wait.ForSQL("5432/tcp", "postgres", func(host string, port nat.Port) string {
			// Host comes from the container runtime (often "127.0.0.1" or mapped host)
			// Use host + mapped port to build a DSN that will work from the test process.
			return fmt.Sprintf("postgres://postgres:postgres@%s:%s/goaudio?sslmode=disable", host, port.Port())
		}).WithStartupTimeout(90 * time.Second),
	}

	pgC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: pgReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer func() { _ = pgC.Terminate(ctx) }()

	// Build DSN to use when opening sql.DB in test. Prefer using localhost (127.0.0.1)
	// and mapped port to be safe on Windows, but host from mapped port also works.
	pgPort, err := pgC.MappedPort(ctx, "5432")
	require.NoError(t, err)
	dsn := fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:%s/goaudio?sslmode=disable", pgPort.Port())

	// -------------------------------
	// Wait for Postgres to be ready (poll + ping)
	// -------------------------------
	maxWait := 180 * time.Second
	deadline := time.Now().Add(maxWait)
	dbReady := false
	for time.Now().Before(deadline) {
		conn, err := sql.Open("postgres", dsn)
		if err == nil {
			pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err = conn.PingContext(pingCtx)
			cancel()
			_ = conn.Close()
			if err == nil {
				dbReady = true
				break
			}
		}
		time.Sleep(1 * time.Second)
	}
	if !dbReady {
		// attempt to read container logs to help debugging
		if logsRC, err := pgC.Logs(ctx); err == nil && logsRC != nil {
			if buf, rerr := io.ReadAll(logsRC); rerr == nil {
				_ = logsRC.Close()
				t.Logf("Postgres container logs:\n%s", string(buf))
			}
		}
		t.Fatalf("Postgres did not become ready within %v", maxWait)
	}

	// -------------------------------
	// 2) Create DB schema inline
	// -------------------------------
	conn, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer conn.Close()

	schema := []string{
		`CREATE TABLE IF NOT EXISTS uploads (
			id SERIAL PRIMARY KEY,
			filename TEXT,
			path TEXT,
			output_path TEXT,
			content_type TEXT,
			size BIGINT,
			status TEXT DEFAULT 'queued',
			duration_seconds DOUBLE PRECISION,
			integrated_lufs DOUBLE PRECISION,
			bpm DOUBLE PRECISION,
			musical_key TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
		);`,
		`CREATE TABLE IF NOT EXISTS jobs (
			id SERIAL PRIMARY KEY,
			upload_id INT REFERENCES uploads(id),
			type TEXT,
			status TEXT DEFAULT 'queued',
			progress INT DEFAULT 0,
			logs TEXT DEFAULT '',
			retry_count INT DEFAULT 0,
			max_retries INT DEFAULT 3,
			last_error TEXT DEFAULT '',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT now()
		);`,
	}
	for _, s := range schema {
		_, err := conn.Exec(s)
		require.NoError(t, err)
	}

	// -------------------------------
	// 3) Start NATS container
	// -------------------------------
	natsReq := testcontainers.ContainerRequest{
		Image:        "nats:latest",
		ExposedPorts: []string{"4222/tcp"},
		WaitingFor:   wait.ForListeningPort("4222/tcp").WithStartupTimeout(60 * time.Second),
	}
	natsC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: natsReq,
		Started:          true,
	})
	require.NoError(t, err)
	defer func() { _ = natsC.Terminate(ctx) }()

	natsHost, err := natsC.Host(ctx)
	require.NoError(t, err)
	natsPort, err := natsC.MappedPort(ctx, "4222")
	require.NoError(t, err)
	natsURL := "nats://" + natsHost + ":" + natsPort.Port()

	// -------------------------------
	// 4) Prepare environment & storage
	// -------------------------------
	storageDir := t.TempDir()
	require.NoError(t, os.Setenv("DATABASE_URL", dsn))
	require.NoError(t, os.Setenv("STORAGE_PATH", storageDir))
	require.NoError(t, os.Setenv("NATS_URL", natsURL))

	// -------------------------------
	// 5) Start API server (in-process)
	// -------------------------------
	apiCfg := serverpkg.APIServerConfig{
		Addr:        "127.0.0.1:8085",
		DatabaseDSN: dsn,
		StorageBase: storageDir,
		NatsURL:     natsURL,
		DevLogging:  true,
	}
	apiCtx, apiCancel := context.WithCancel(ctx)
	defer apiCancel()

	srv, err := serverpkg.RunAPIServer(apiCtx, apiCfg)
	require.NoError(t, err)

	// give server a moment to start listening
	time.Sleep(500 * time.Millisecond)

	// -------------------------------
	// 6) Start worker (in-process) with a simulated handler
	// -------------------------------
	workerCfg := workerpkg.WorkerConfig{
		DatabaseDSN: dsn,
		NatsURL:     natsURL,
		Concurrency: 2,
		QueueSize:   10,
		DevLogging:  true,
	}
	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	// simulated processing handler (uses db.New to mirror runtime)
	simHandler := func(ctx context.Context, jm q.JobMessage) error {
		d, err := db.New(ctx)
		if err != nil {
			return err
		}
		defer d.Close()

		var relPath string
		if err := d.Pool.QueryRow(ctx, `SELECT path FROM uploads WHERE id=$1`, jm.UploadID).Scan(&relPath); err != nil {
			_ = d.UpdateJobStatus(ctx, jm.JobID, "failed", 0, "upload not found")
			return err
		}

		base := os.Getenv("STORAGE_PATH")
		if base == "" {
			base = storageDir
		}
		// inputFull := filepath.Join(base, relPath)
		outRel := relPath + ".processed"
		outFull := filepath.Join(base, outRel)

		if err := os.MkdirAll(filepath.Dir(outFull), 0o755); err != nil {
			_ = d.UpdateJobStatus(ctx, jm.JobID, "failed", 0, "mkdir failed: "+err.Error())
			return err
		}
		if err := os.WriteFile(outFull, []byte("processed:"+relPath), 0o644); err != nil {
			_ = d.UpdateJobStatus(ctx, jm.JobID, "failed", 0, "write failed: "+err.Error())
			return err
		}

		_, _ = d.Pool.Exec(ctx, `UPDATE uploads SET output_path=$1 WHERE id=$2`, outRel, jm.UploadID)
		_ = d.UpdateJobStatus(ctx, jm.JobID, "done", 100, "simulated processing done")
		return nil
	}

	pool, err := workerpkg.RunWorker(workerCtx, workerCfg, simHandler)
	require.NoError(t, err)
	_ = pool

	// small pause for NATS subscription to be ready
	time.Sleep(300 * time.Millisecond)

	// -------------------------------
	// 7) Upload sample file to API
	// -------------------------------
	sampleSrc := filepath.Join("testdata", "short.mp3")
	if _, err := os.Stat(sampleSrc); os.IsNotExist(err) {
		_ = os.MkdirAll("testdata", 0o755)
		_ = os.WriteFile(sampleSrc, []byte("tiny"), 0o644)
	}

	f, err := os.Open(sampleSrc)
	require.NoError(t, err)
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(sampleSrc))
	require.NoError(t, err)
	_, err = io.Copy(part, f)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest("POST", "http://127.0.0.1:8085/upload", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var up struct {
		UploadID int64  `json:"upload_id"`
		Status   string `json:"status"`
		Path     string `json:"path"`
	}
	err = json.NewDecoder(resp.Body).Decode(&up)
	require.NoError(t, err)
	require.Greater(t, up.UploadID, int64(0))

	// -------------------------------
	// 8) Poll DB for job completion
	// -------------------------------
	deadline2 := time.Now().Add(30 * time.Second)
	var jobID int64
	for time.Now().Before(deadline2) {
		row := conn.QueryRow(`SELECT id, status FROM jobs WHERE upload_id=$1`, up.UploadID)
		var status string
		err := row.Scan(&jobID, &status)
		if err == sql.ErrNoRows || err != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if err != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if status == "done" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	require.Greater(t, jobID, int64(0), "job not found or not completed in time")

	// Check uploads.output_path
	var outPath sql.NullString
	err = conn.QueryRow(`SELECT output_path FROM uploads WHERE id=$1`, up.UploadID).Scan(&outPath)
	require.NoError(t, err)
	require.True(t, outPath.Valid, "output_path should be set")

	outFull := filepath.Join(storageDir, outPath.String)
	_, err = os.Stat(outFull)
	require.NoError(t, err, "output file must exist")

	// -------------------------------
	// 9) Shutdown and cleanup
	// -------------------------------
	_ = srv.Shutdown(context.Background())
	workerCancel()
	time.Sleep(200 * time.Millisecond)
}
