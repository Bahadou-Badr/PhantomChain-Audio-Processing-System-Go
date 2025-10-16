~/ PhantomChain-Audio-Processing-System-Go


## Architecture (Phase 1)
```
[Client] <-- HTTP --> [API service (:8080)]
                                |
                        ---------------
                        |             |
                    [Postgres]      [NATS]
```

-------------------------
-------------------------
#  Job queue
We went with NATS beacause is Minimal / cloud-native, Scalable and Designed for high performance and low latencyand , with features like server-side filtering

# Observability, metrics & tests

### Observability
Add structured logging (zap) and metrics (Prometheus client) :
- ```internal/logging/logger.go``` (zap init)
- ```internal/metrics/metrics.go``` (Prometheus metrics)
- ```internal/server/server.go``` to init logging, register metrics and expose /metrics
- ```internal/worker/runner.go``` (init logging + register metrics if desired)

Instrumentation snippets to add to ```internal/worker/pool.go``` so the pool updates metrics and logs (full file patch shown)

### Tests End2End (testcontainers)
**Testcontainers orchestrates containers programmatically** :
- testcontainers spins Postgres & NATS. The test waits for them to be ready.
- We use api.RunAPIServer to start the API in-process on 127.0.0.1:8085.
- We start an in-process worker via worker.RunWorker, passing a simHandler closure that performs a trivial "processing" (writes a .processed file and updates DB/job status). This avoids needing FFmpeg inside CI.
- We upload a sample file via HTTP POST /upload and poll the DB until the job becomes done.
- The test asserts uploads.output_path is set and the output file exists in the temp storage folder.

-----
-----
# my notes
## Test upload
```bash
curl -v -F "file=@C:\Users\bdr\Documents\Workspace\Test\SZN SAMPLE11.mp3" http://localhost:8080
/upload
```

## job Simulate
- Simulate progress:
```bash
curl -X PATCH http://localhost:8080/api/jobs/1 -H "Content-Type: application/json" -d "{\"status\":\"processing\",\"progress\":20,\"log\":\"started transcode\"}"
```

- Set done:
```bash
curl -X PATCH http://localhost:8080/api/jobs/1 -H "Content-Type: application/json" -d "{\"status\":\"done\",\"progress\":100,\"log\":\"finished transcode\"}"
```
## Analysis features (LUFS, BPM, key)
### Summary
 - Add DB columns ```bpm```, ```musical_key``` , (and ```integrated_lufs``` if not present).
 - Add ```tools/analyze.py``` and test it manually.
 - Add ```internal/audio/analyze.go``` wrapper to call Python.
 - Update worker ```handleJob``` to call ```audio.Loudness``` and ```audio.AnalyzeWithPython``` .
 Update API to expose ```/uploads/{id}/analysis``` .

## Tests & manual run (short) 

- Start API & worker & NATS.
- Upload a short MP3.
- Worker transcodes, saves output, runs ffmpeg loudnorm (LUFS), then runs python tools/analyze.py for BPM/key.
- Check DB and ```GET /uploads/{id}/analysis``` for results.

## End2end tests using (Testcontainers)
- Run _**Docker Desktop**_ ; testcontainers requires Docker to be available locally or in CI runner
Run ```go test ./...``` to build everything. Unit tests will run.
- Run integration tests (Testcontainers) with Docker available: ```go test ./test/integration -v``` (remove ```t.Skip```). 
- The test will start Postgres + NATS containers and start API & worker in-process; metrics endpoint will be mounted on the API server, and the worker will update metrics via the package-level counters.
