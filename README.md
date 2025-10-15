~/ PhantomChain-Audio-Processing-System-Go

# go-audio-queue — Phase 1

## Goal (Phase 1)
Initialize project skeleton:
- Basic API service exposing `/health` and `/ready`.
- Docker compose with Postgres + MinIO + API container.
- README and basic repo structure.

## Quickstart (local)
1. Build & start:
```bash
docker-compose up --build
```
2. Test endpoints
```bash
# health
curl http://localhost:8080/health
# ready
curl http://localhost:8080/ready
```

## Architecture (Phase 1)
```
[Client] <-- HTTP --> [API service (:8080)]
                                |
                        ---------------
                        |             |
                    [Postgres]      [MinIO]
```



-----
# my note
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
### Tests & manual run (short) 

- Start API & worker & NATS.
- Upload a short MP3.
- Worker transcodes, saves output, runs ffmpeg loudnorm (LUFS), then runs python tools/analyze.py for BPM/key.
- Check DB and ```GET /uploads/{id}/analysis``` for results.

#  Job queue
We went with NATS beacause is Minimal / cloud-native, Scalable and Designed for high performance and low latencyand , with features like server-side filtering