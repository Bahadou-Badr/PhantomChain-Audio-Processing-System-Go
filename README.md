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