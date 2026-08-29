# Project: youtrack-proxy

## Overview

This is a Go HTTP proxy server that accepts Jira REST API v2 requests and translates them into YouTrack API calls. It allows tools/integrations that only speak Jira's API to transparently create issues in YouTrack.

## Architecture

```
internal/
  client/     - HTTP client for outbound YouTrack API calls + concurrency semaphore
  config/     - Configuration loading (env vars)
  handler/    - Echo HTTP route handlers (one file per resource area)
  idmap/      - Deterministic, reversible YouTrack ID ↔ int64 encoding (see below)
  middleware/ - Echo middleware (Basic Auth extraction)
  model/      - Shared request/response structs (models.go, jira_read.go, youtrack_read.go)
  service/    - Business logic: converters, JQL translation, status mapping, caching
main.go       - Server bootstrap, route registration, concurrency init
```

## Tech Stack

- **Language**: Go 1.25+
- **HTTP Framework**: Echo v4 (`github.com/labstack/echo/v4`)
- **Logging**: zerolog (`github.com/rs/zerolog`)
- **Concurrency**: `golang.org/x/sync/semaphore` (bounded outbound request concurrency)
- **Testing**: `pgregory.net/rapid` (property-based testing)
- **Deployment**: Docker (multi-stage build, scratch base image)
- **CI**: GitHub Actions (build, test, push to GHCR)

## Coding Conventions

- Follow standard Go project layout (`internal/` for private packages).
- Use zerolog for all structured logging. Do not use `fmt.Println` for runtime logs.
- Errors should be returned, not logged-and-swallowed. Let the handler layer decide the HTTP response.
- Keep handler functions thin — delegate logic to `service/` and I/O to `client/`. Handlers are split by resource area: `handler.go` (issue creation), `issues.go`, `projects.go`, `boards.go`, `pickers.go`, `filters.go`, `fields.go`, `serverinfo.go`, `stubs.go`, `timeout.go`.
- Structs live in `internal/model/` across three files: `models.go` (write-direction), `jira_read.go` (Jira response types), `youtrack_read.go` (YouTrack response types).
- Configuration is loaded once at startup via `config.LoadConfig()` and passed by pointer.
- Use `echo.Context` only in handler layer. Service and client layers accept plain Go types.

## Naming

- Files: lowercase, underscores for multi-word.
- Go: standard camelCase/PascalCase per Go conventions.
- Packages: single lowercase word where possible.

## Error Handling

- Wrap errors with `fmt.Errorf("context: %v", err)` or `fmt.Errorf("context: %w", err)` for unwrapping.
- Return structured JSON error responses from handlers: `{"error": "message"}`.
- Use appropriate HTTP status codes (400 for bad input, 401 for auth, 500 for upstream failures).

## ID Encoding

Jira clients expect numeric IDs. YouTrack uses string IDs like `"0-4"` or `"0-0.9-60"`. The `internal/idmap` package provides deterministic, reversible bitfield encoding between these formats — no state, no I/O, pure functions:

- `idmap.Encode(youtrackID) (int64, error)` — pack a YT string ID into a 63-bit int64.
- `idmap.Decode(numericID) (string, bool)` — unpack back to the original YT string ID.
- `idmap.FormatID(id) string` — decimal string representation.

Used throughout handlers, service converters, boards, and filters. When encoding fails, handlers return 500; converters fall back to the raw YT ID with a warning log.

## Authentication

- Incoming requests use HTTP Basic Auth where the password is the YouTrack permanent token.
- The `internal/middleware/` package extracts the token and stores it in `model.RequestContext` on the echo context.
- Client functions forward it as a Bearer token to YouTrack.

## Adding New Endpoints

1. Define request/response structs in the appropriate `internal/model/` file (`models.go` for write, `jira_read.go` / `youtrack_read.go` for read).
2. Add conversion logic in `internal/service/` (write: `converter.go`, read: `converter_read.go`).
3. Add YouTrack API call in `internal/client/youtrack.go` (update field constants if new fields are needed).
4. Add the handler in `internal/handler/` — use the existing file for that resource area or create a new one.
5. Register the route in `main.go` under the appropriate API group.
6. If the endpoint returns Jira-style numeric IDs, use `idmap.Encode` / `idmap.Decode`.
