~/ PhantomChain-Audio-Processing-System-Go 🕷️

# 🎧 PhantomChain — Distributed Audio Processing & Job Queue System (Go-based)

**PhantomChain** is a modular backend system written in **Go**, designed for **asynchronous audio processing** with a **scalable worker pool**, **NATS-based queue**, **structured logging (zap)**, **Prometheus metrics**, and **end-to-end integration testing** powered by Testcontainers.

Designed for scalability, it follows a microservice-style architecture separating the **API**, **Worker**, and **internal libraries** for clean maintainability.

---

## 🕸️ Architecture Overview

```pgsql
                      ┌───────────────────┐
                      │     Client App    │
                      │  (uploads audio)  │
                      └────────┬──────────┘
                               │ HTTP (JSON)
                               ▼
                ┌─────────────────────────────────┐
                │           API Server            │
                │─────────────────────────────────│
                │ • Receives upload requests      │
                │ • Stores job metadata (Postgres)│
                │ • Publishes jobs to NATS        │
                └────────────────┬────────────────┘
                                 │
                              Pub/Sub
                                 │
                                 ▼
                ┌────────────────────────────────┐
                │            Worker(s)           │
                │────────────────────────────────│
                │ • Subscribes to NATS jobs      │
                │ • Runs FFmpeg for processing   │
                │ • Reports job status updates   │
                │ • Exposes Prometheus metrics   │
                └────────────────┬───────────────┘
                                 │
                                 ▼
                           ┌─────────────┐
                           │ PostgreSQL  │
                           └─────────────┘
```


**Modules:**
- `cmd/api/` — API server entrypoint  
- `cmd/worker/` — Worker service entrypoint  
- `internal/api/` — HTTP handlers, routing, middleware  
- `internal/server/` — Helper to launch API in tests  
- `internal/worker/` — Worker pool, job handling (pool, runner)  
- `internal/db/` — Database connection, queries, migrations  
- `internal/queue/` — NATS client & JobMessage definitions 
- `internal/storage/` — Local file storage logic 
- `internal/logging/` — Zap logger initialization
- `internal/metrics/` — Prometheus metric definitions  
- `internal/audio/` — FFmpeg & analysis helpers (Probe, Transcode, Loudness, etc.)  
- `tools/analyze.py` — Python script to compute BPM / key using librosa  
- `test/integration/` — E2E tests (Testcontainers-based)
- `testdata/` — Sample audio files
- `deploy/` — Supporting Files
- `Dockerfile` — Dockerfile file example 
- `docker-compose.yml` — Full example includes logging, metrics exposure, network setup, and service dependencies (PhantomChain: API + Worker + NATS + PostgreSQL + Prometheus)

---

## ⚙️ Setup Instructions

### 1. Prerequisites
- Go 1.23+
- Docker  
- FFmpeg installed locally  
- Python + librosa (for BPM/key analysis, optional) 

### 2. Clone the repository
```bash
git clone https://github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go.git
cd PhantomChain-Audio-Processing-System-Go
go mod download
```

Set environment variables (example):
```bash
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/phantom?sslmode=disable"
export NATS_URL="nats://localhost:4222"
export STORAGE_PATH="./data"
```

### 3. Start dependencies
Start:
- PostgreSQL (port 5432)
- NATS Server (port 4222)
- Optional Grafana + Prometheus stack (if configured)

### 4. Run the API server
```bash
go run ./cmd/api
```
### 5. Run the Worker service
```bash
go run ./cmd/worker
```

```bash
curl -v -F "file=@C:\Users\dev\path\Test\SHORTSAMPLE1.mp3" http://localhost:8080/upload
```
`GET /uploads/{id}` to inspect uploads

Check ```/jobs/{id}``` via API should move from queued → running → processing → done, with logs

List jobs `curl http://localhost:8080/jobs`

Check DB and `GET /uploads/{id}/analysis` for results.

## 🔍 Observability

### **Logging (Zap)**

Structured JSON logs for API & Worker.  
Log levels: `INFO`, `ERROR`, `DEBUG`.

**Example:**
```json
{"level":"info","msg":"Job processed","job_id":42,"duration":"3.1s"}
```

### **Metrics (Prometheus)**

Metrics are exposed at `/metrics` on both API & Worker.
You can scrape them via *Prometheus* or view through *Grafana*.

#### Key Metrics :
| Metric                 | Type      | Description                       |
| ---------------------- | --------- | --------------------------------- |
| `jobs_processed_total` | Counter   | Total jobs successfully processed |
| `jobs_failed_total`    | Counter   | Failed job count                  |
| `job_duration_seconds` | Histogram | Job processing durations          |
| `worker_active_gauge`  | Gauge     | Current active workers            |
------

## 🧪 Testing
Integration and E2E tests use Testcontainers-Go to run isolated NATS and PostgreSQL containers automatically.

### Run all tests
```bash
go test ./test/integration -v
```
### Example test flow
1. Start PostgreSQL and NATS containers.

2. Start API and Worker inside the test environment.

3. Upload sample audio file via HTTP API.

4. Wait for worker to process the job and update DB status.

5. Assert successful job completion and processed output.

```bash
$ go test ./test/integration -v
=== RUN   TestE2E_UploadAndProcess_WithTestcontainers
2025/10/18 23:17:31 github.com/testcontainers/testcontainers-go - Connected to docker: 
  Server Version: 28.5.1
  API Version: 1.51
  Operating System: Docker Desktop
  Total Memory: 7802 MB
  Labels:
    com.docker.desktop.address=npipe://\\.\pipe\docker_cli
  Testcontainers for Go Version: v0.39.0
  Resolved Docker Host: npipe:////./pipe/docker_engine
  Resolved Docker Socket Path: //var/run/docker.sock
  Test SessionID: 448dbea7e23cce9d441ed41e7d198edbc5d81a29cbc802f7973b975fb0108c26
  Test ProcessID: 03574ec7-9c40-4257-a72d-178d7b53c8fa
2025/10/18 23:17:31 No image auth found for https://index.docker.io/v1/. Setting empty credentials for the image: postgres:15. This is expected for public images. Details: credentials not found in native keychain
2025/10/18 23:19:10 🐳 Creating container for image postgres:15
2025/10/18 23:19:11 No image auth found for https://index.docker.io/v1/. Setting empty credentials for the image: testcontainers/ryuk:0.13.0. This is expected for public images. Details: credentials not found in native keychain
2025/10/18 23:19:18 🐳 Creating container for image testcontainers/ryuk:0.13.0
2025/10/18 23:19:18 ✅ Container created: d6b0bf90f6f8
2025/10/18 23:19:18 🐳 Starting container: d6b0bf90f6f8
2025/10/18 23:19:19 ✅ Container started: d6b0bf90f6f8
2025/10/18 23:19:19 ⏳ Waiting for container id d6b0bf90f6f8 image: testcontainers/ryuk:0.13.0. Waiting for: &{Port:8080/tcp timeout:<nil> PollInterval:100ms skipInternalCheck:false skipExternalCheck:false}
2025/10/18 23:19:19 Shell not found in container
2025/10/18 23:19:19 🔔 Container is ready: d6b0bf90f6f8
2025/10/18 23:19:19 ✅ Container created: f90134fabe84
2025/10/18 23:19:19 🐳 Starting container: f90134fabe84
2025/10/18 23:19:19 ✅ Container started: f90134fabe84
2025/10/18 23:19:19 ⏳ Waiting for container id f90134fabe84 image: postgres:15. Waiting for: &{timeout:0xc000384150 URL:0x12c8120 Driver:postgres Port:5432/tcp startupTimeout:60000000000 PollInterval:100ms query:SELECT 1}
2025/10/18 23:19:22 🔔 Container is ready: f90134fabe84
2025/10/18 23:19:22 No image auth found for https://index.docker.io/v1/. Setting empty credentials for the image: nats:latest. This is expected for public images. Details: credentials not found in native keychain
2025/10/18 23:19:27 🐳 Creating container for image nats:latest
2025/10/18 23:19:27 ✅ Container created: eb9a541463f2
2025/10/18 23:19:27 🐳 Starting container: eb9a541463f2
2025/10/18 23:19:28 ✅ Container started: eb9a541463f2
2025/10/18 23:19:28 ⏳ Waiting for container id eb9a541463f2 image: nats:latest. Waiting for: &{Port:4222/tcp timeout:0xc0003bcaa0 PollInterval:100ms skipInternalCheck:false skipExternalCheck:false}
2025/10/18 23:19:28 Shell not found in container
2025/10/18 23:19:28 🔔 Container is ready: eb9a541463f2
2025-10-18T23:19:28.706+0100    INFO    server/server.go:46     server.RunAPIServer starting    {"addr": "127.0.0.1:8085"}
2025-10-18T23:19:29.236+0100    INFO    worker/pool.go:44       starting worker pool    {"concurrency": 2}
{"level":"info","job":1,"time":"2025-10-18T23:19:29+01:00","message":"published job to nats"}
{"level":"info","path":"20251018\\20251018-221929-short.mp3","size":4,"time":"2025-10-18T23:19:29+01:00","message":"uploaded file id=1 job=1"}
2025-10-18T23:19:29.570+0100    INFO    worker/pool.go:86       processing job  {"job": 1, "worker": 0}
2025-10-18T23:19:29.620+0100    INFO    worker/pool.go:134      job completed   {"job": 1, "duration_s": 0.0433098}
2025-10-18T23:19:29.870+0100    INFO    worker/pool.go:53       stopping worker pool
2025/10/18 23:19:30 🐳 Stopping container: eb9a541463f2
2025-10-18T23:19:30.070+0100    INFO    server/server.go:110    server.RunAPIServer shutdown requested
2025/10/18 23:19:30 ✅ Container stopped: eb9a541463f2
2025/10/18 23:19:30 🐳 Terminating container: eb9a541463f2
2025/10/18 23:19:30 🚫 Container terminated: eb9a541463f2
2025/10/18 23:19:30 🐳 Stopping container: f90134fabe84
2025/10/18 23:19:31 ✅ Container stopped: f90134fabe84
2025/10/18 23:19:31 🐳 Terminating container: f90134fabe84
2025/10/18 23:19:31 🚫 Container terminated: f90134fabe84
--- PASS: TestE2E_UploadAndProcess_WithTestcontainers (120.32s)
PASS
ok      github.com/Bahadou-Badr/PhantomChain-Audio-Processing-System-Go/test/integration        (120.32s)
```
--------

## 🧩 Technology Stack
| Layer | Technology | Advantages |
|-------|-------------|-------------|
| **Language** | Go (Golang) | Fast compilation, concurrency via goroutines, excellent tooling, strong standard library. |
| **Queue** | NATS | Lightweight, distributed, high-performance messaging system, perfect for job dispatch. |
| **Database** | PostgreSQL | Reliable ACID transactions, JSONB support for flexibility, rich ecosystem of tools. |
| **Processing** | FFmpeg | Proven, robust multimedia engine capable of transcoding, extracting, and analyzing audio. |
| **Observability** | Zap + Prometheus | Structured JSON logs and runtime metrics for visibility and performance tracking. |
| **Testing** | Testcontainers-Go | Real integration tests using real PostgreSQL and NATS containers. |
| **Routing** | Chi | Minimal, idiomatic HTTP router for Go — fast and composable. |
| **Configuration** | Environment Variables | Simple, flexible, and consistent across environments. |
| **Containerization** | Docker Compose | One-command environment orchestration for local dev and testing. |

## Why NATS for Job Queueing

NATS was chosen as the message broker because of its **simplicity, speed, and horizontal scalability**.  
It’s lightweight, extremely fast, and easy to integrate into Go projects without complex setup like Kafka or RabbitMQ.  

### Key Reasons:
- **Low latency & high throughput:** Ideal for real-time audio processing pipelines.  
-  **Go-native client:** Seamless integration with idiomatic Go patterns (channels, goroutines).  
-  **Automatic message persistence (JetStream):** Supports durable streams, replay, and acknowledgment.  
- **Scalable topology:** Multiple workers can process jobs concurrently across nodes.  
- **Pub/Sub + QueueGroup hybrid:** Simple fan-out or load-balancing via subject subscriptions.  

This combination gives PhantomChain a **fault-tolerant and distributed processing system** without introducing unnecessary complexity.

---

## ⚡ Design Notes
- Modular internal/ packages isolate logic for maintainability.

- Each service (API, Worker) runs independently — horizontally scalable.

- Worker pool uses goroutine-safe queue management and context cancellation for graceful shutdowns.

- Observability stack supports metrics-driven debugging.

- Integration tests simulate real-life workflows (upload → queue → process → store).



## 👾 Future Enhancements

| Enhancement                         | Description                                                                                                                       |
| ----------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| **MinIO Integration**               | Use **MinIO** as a high-performance, **S3-compatible object storage** for managing and storing processed audio files efficiently. |
| **Grafana Dashboards**              | Implement **Grafana** to visualize metrics collected via Prometheus (e.g., job durations, worker throughput, error rates).        |
| **Multi-Format Audio Transcoding**  | Extend FFmpeg worker logic to support multiple output formats (MP3, WAV, FLAC, AAC).                                              |
|**Authentication & access control for API**| Secure endpoints and restrict access based on roles or API keys
|Add `Configs/` folder | Centralize configuration for easier management and environment switching (for environment variables and other servers configuration)
