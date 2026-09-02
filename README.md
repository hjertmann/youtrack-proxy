# youtrack-proxy

An HTTP proxy that presents a Jira REST API surface backed by YouTrack. Built so [Apache DevLake](https://devlake.apache.org/) (and its Jira plugin) can ingest issues, projects, statuses, and changelogs from a YouTrack instance without any DevLake code changes.

## How It Works

DevLake's Jira plugin talks to this proxy as if it were a Jira Server. The proxy translates every request into the equivalent YouTrack API call and reshapes the response back into Jira's JSON format.

```
DevLake Jira Plugin  ──HTTP──▶  youtrack-proxy  ──HTTP──▶  YouTrack Cloud/Server
                     (Jira API)                  (YT REST API)
```

## Quick Start

### Run the published image (GHCR)

Prebuilt images are published to the GitHub Container Registry on every push to `main` and on version tags.

```sh
docker run -p 8080:8080 \
  -e YOUTRACK_URL=https://your-instance.youtrack.cloud \
  ghcr.io/hjertmann/youtrack-proxy:latest
```

Available tags:

- `latest` — tracks the current `main` HEAD
- `YYYY.MM.DD` (e.g. `2026.08.30`) — automatic CalVer datestamp, published on every `main` build
- `<major>.<minor>.<patch>` (e.g. `1.2.3`), plus `<major>.<minor>` and `<major>` — published from `v*` git tags (optional, for explicit releases)
- `sha-<short-sha>` — immutable, one per commit; use this to pin an exact build

See [Environment Variables](#environment-variables) for all configuration options.

### Docker Compose (recommended)

```sh
export YOUTRACK_URL=https://your-instance.youtrack.cloud
docker compose up --build
```

### Binary

```sh
export YOUTRACK_URL=https://your-instance.youtrack.cloud
export PORT=8080
go run main.go
```

The proxy listens on `http://localhost:8080`. Point DevLake's Jira connection at this address.

### Authentication

The proxy expects HTTP Basic Auth on **every endpoint except `/health`**. This includes `/rest/api/2/serverInfo` and every other `/rest/...` route across the v2, v3, and Agile API groups — they all sit behind the Basic Auth middleware and return HTTP 401 without valid credentials.

- **Username**: any value by default, or a specific username if `AUTH_USERNAME` is set
- **Password**: your YouTrack permanent token

The proxy extracts the token from the Basic Auth password field and forwards it as a `Bearer` token to YouTrack.

If `AUTH_USERNAME` is set, the proxy rejects requests whose Basic Auth username does not match (HTTP 401). This lets you lock the proxy to a single known user. When unset or empty, any username is accepted (the previous default behavior).

The only unauthenticated route is the health check below.

### Health Check

`GET /health` is the sole endpoint that requires no authentication and makes no YouTrack call. It returns `200 OK` with the plain-text body `OK` and a `Cache-Control: no-cache` header, so it is safe to use as a liveness/readiness probe (Docker healthcheck, Kubernetes probe, load-balancer check).

```sh
curl http://localhost:8080/health
# => OK
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `AUTH_USERNAME` | _(empty)_ | If set, only this Basic Auth username is accepted. Whitespace-only values are treated as empty. |
| `PORT` | `8080` | Listen port |
| `YOUTRACK_URL` | `https://example.youtrack.cloud` | YouTrack base URL |
| `YT_MAX_CONCURRENCY` | `10` | Max parallel requests to YouTrack (1-100) |
| `YT_QUEUE_TIMEOUT_SECONDS` | `30` | Timeout waiting for a concurrency slot (1-300) |
| `YT_REQUEST_TIMEOUT_SECONDS` | `30` | Upstream HTTP request timeout (1-300) |

## Key Concept Mappings

### Boards = Projects

Jira's Agile API has boards; YouTrack does not. The proxy synthesizes one board per YouTrack project. When DevLake asks for boards, it gets back the full project list. The board ID is a deterministic numeric encoding of the YouTrack project ID (see ID Mapping below), so the same project always produces the same board.

Board configuration and filter endpoints follow the same pattern: a filter ID decodes back to a project, and the returned JQL is simply `project = <KEY>`.

Sprints are stubbed as empty lists since there is no direct sprint equivalent mapped.

### State Mapping

YouTrack uses free-form state names per project. Jira groups every status into one of three categories: To Do, In Progress, or Done. The proxy maps YouTrack states to Jira status categories using two inputs:

1. **YouTrack's `isResolved` flag** on each bundle value. Any state marked `isResolved` in YouTrack maps to **Done** (category ID 3, key `done`).
2. **A hardcoded "new" set** for states that should be To Do: `open`, `submitted`, `incomplete`, `new`, `reopened`, `to do`, `backlog`. These map to **To Do** (category ID 1, key `new`).
3. **Everything else** defaults to **In Progress** (category ID 4, key `indeterminate`).

The resolved state set is fetched per-project from YouTrack's State field bundle and cached for one hour.

### Issue Type and Priority

YouTrack Type and Priority custom field bundle values are returned as Jira issue types and priorities. Their IDs are deterministic numeric encodings of the YouTrack bundle value IDs.

### Users

YouTrack `login` maps to Jira `key`, `name`, and `accountId`. YouTrack `fullName` maps to `displayName`. The `banned` flag is inverted to Jira's `active`.

### Changelog

YouTrack activity items are converted to Jira changelog history entries. Activities with the same timestamp and author are grouped into a single history entry, matching Jira's behavior. Field names are mapped (e.g. `State` becomes `status`, `Type` becomes `issuetype`).

Resolution date is derived from the last activity that transitioned an issue into a done-category state.

## ID Mapping

Jira uses numeric IDs everywhere. YouTrack uses string IDs like `0-4` or `0-0.9-60`. The proxy deterministically encodes YouTrack IDs into 63-bit integers using a bitfield layout, with no database or state file.

Two modes cover all YouTrack ID formats:

**Base entity** (`<typeId>-<seqId>`, e.g. `0-4`): flag bit 0, 8-bit type, 54-bit sequence.

**Activity stream** (`<typeId>-<seqId>.<catId>-<eventOffset>`, e.g. `0-0.9-60`): flag bit 1, 7-bit type, 24-bit sequence, 6-bit category, 25-bit event offset.

The encoding is reversible: every board ID, filter ID, issue ID, status ID, and issue type ID can be decoded back to the original YouTrack string ID. This is how board/filter lookups work without any mapping table.

**Limitation**: the bit widths impose upper bounds on each field. Base entity sequence IDs can go up to ~18 quadrillion, which is effectively unlimited. Activity stream IDs are more constrained (sequence max ~16M, event offset max ~33M). If a YouTrack instance has IDs exceeding these bounds, the encoding will fail and the entity will be skipped with a warning log.

### Issue Creation

When creating issues (`POST /rest/api/2/issue`), the proxy fetches the target project's custom field configuration from YouTrack to resolve the field IDs for Type, Priority, and Assignee dynamically. No manual field mapping configuration is needed.

## API Surface

### Core (used by DevLake)

| Method | Path | Description |
|---|---|---|
| GET | `/rest/api/2/serverInfo` | Server discovery (requires auth) |
| GET | `/rest/api/2/search` | Issue search with JQL translation |
| GET | `/rest/api/2/search/jql` | Same as above (alternate path) |
| GET | `/rest/api/3/search/jql` | v3 variant of search |
| GET | `/rest/api/2/issue/:id` | Single issue |
| GET | `/rest/api/2/issue/:id/changelog` | Issue change history |
| GET | `/rest/api/2/issue/:id/comment` | Issue comments |
| GET | `/rest/api/2/issue/:id/editmeta` | Editable fields metadata |
| GET | `/rest/api/2/project` | List all projects |
| GET | `/rest/api/2/project/:id` | Single project |
| GET | `/rest/api/2/status` | List all statuses with categories |
| GET | `/rest/api/2/issuetype` | List all issue types |
| GET | `/rest/api/2/field` | Field metadata (static) |
| GET | `/rest/agile/1.0/board` | List boards (1 per project) |
| GET | `/rest/agile/1.0/board/:id/configuration` | Board config |
| GET | `/rest/agile/1.0/board/:id/sprint` | Sprints (empty stub) |
| GET | `/rest/api/2/filter/:id` | Synthetic filter for a project |
| GET | `/rest/api/2/myself` | Current user |
| GET | `/rest/api/2/user` | User lookup |
| POST | `/rest/api/2/issue` | Create issue |

### Stubs (return empty responses to prevent DevLake errors)

| Method | Path |
|---|---|
| GET | `/rest/api/2/issue/:id/worklog` |
| GET | `/rest/api/2/issue/:id/remotelink` |
| GET | `/rest/api/2/filter/search` |
| GET | `/rest/api/2/project/recent` |

## Not Supported

- Jira workflows and transitions
- Sprints / sprint planning
- Worklogs and time tracking
- Remote links
- Attachments
- Webhooks
- Issue updates (PUT/PATCH) and deletions
- Jira Cloud (accountId-based) authentication

## Releasing

Images are published automatically by the CI workflow (`.github/workflows/build-and-push.yml`); you never push images by hand.

**On every merge to `main`**, CI builds and pushes:

- `latest` — updated to the new `main` HEAD
- `YYYY.MM.DD` — an automatic CalVer datestamp (UTC) of the build
- `sha-<short-sha>` — immutable, pinned to that exact commit

This gives you an automatic, human-readable version on every build with no manual step. Two builds on the same day reuse the same datestamp tag; use `sha-<short-sha>` when you need to pin an exact commit.

**To cut an explicit semver release** (optional — only needed when you want a compatibility-versioned tag), push a semver git tag (`vMAJOR.MINOR.PATCH`). CI derives the image version from the tag:

```sh
git checkout main
git pull

# tag a commit that is already on main and passing CI
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

A tag of `v1.2.3` produces the image tags `1.2.3`, `1.2`, and `1` (plus the usual `sha-<short-sha>`). The `1.2` and `1` tags roll forward to the newest matching release, so consumers can pin to whatever precision they want.

Notes:

- The tag **must** be valid semver (`v1.2.3`), or CI only emits a `sha-` tag — no version tags.
- The version image is built from the tagged commit's tree, independent of where `main` has moved since.
- This publishes the container image only. To also create a GitHub Release with notes: `gh release create v1.2.3 --generate-notes`.

## License

[MIT](LICENSE)
