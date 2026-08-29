# Build, Test & Run

## Prerequisites

- Go 1.25+ installed
- Docker (optional, for containerized builds)

## Build

```sh
go build ./...
```

## Run Locally

```sh
# Set required env vars
export YOUTRACK_URL=https://your-instance.youtrack.cloud
export PORT=8080

go run main.go
```

## Test

```sh
go test ./...
```

Run with verbose output and coverage:

```sh
go test -v -cover ./...
```

## Lint / Vet

```sh
go vet ./...
```

## Docker

Build and run:

```sh
docker compose up --build
```

Or standalone:

```sh
docker build -t youtrack-proxy .
docker run -p 8080:8080 \
  -e YOUTRACK_URL=https://your-instance.youtrack.cloud \
  youtrack-proxy
```

## CI

GitHub Actions workflow (`.github/workflows/build-and-push.yml`) runs on push to main and PRs:
1. `go build -v ./...`
2. `go test -v ./...`
3. Docker build + push to GHCR (on main only)

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server listen port |
| `YOUTRACK_URL` | `https://example.youtrack.cloud` | YouTrack instance base URL |
| `YT_MAX_CONCURRENCY` | `10` | Max concurrent outbound requests to YouTrack (range 1–100) |
| `YT_QUEUE_TIMEOUT_SECONDS` | `30` | How long a request waits for a concurrency slot before 503 (range 1–300) |
| `YT_REQUEST_TIMEOUT_SECONDS` | `30` | Per-request timeout for YouTrack API calls (range 1–300) |

## Health Check

```sh
curl http://localhost:8080/health
# => OK
```
