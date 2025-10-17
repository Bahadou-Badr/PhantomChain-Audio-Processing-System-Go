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
- `testdata/` — Sample audio files Dockerfile
- `deploy/` — Supporting Files
Supporting File
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

