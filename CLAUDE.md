# DeployKit

A self-hosted PaaS inspired by Railway for deploying applications on a single VM using Docker.

## Architecture

Follows the [WTF Dial](https://github.com/benbjohnson/wtf) pattern by Ben Johnson:

- **Root package (`deploykit`)** — Domain types and service interfaces. Zero dependencies on implementations.
- **`http`** — HTTP server, handlers, and routing. Depends on root domain types.
- **`sqlite`** — Database layer using SQLite. Implements service interfaces from root.
- **`docker`** — Docker client, network provisioning, and log streaming. Implements `Provisioner` and `LogStreamer`.
- **`events`** — In-process `EventBus` implementation (non-blocking pub/sub).
- **`reconciler`** — Periodic reconciliation loop syncing DB state with Docker.
- **`sysinfo`** — Host/Docker/DB introspection. Implements `SystemService`.
- **`cmd/deploykitd`** — Main entry point. Wires everything together.

Dependencies flow inward: implementation packages (`http`, `sqlite`, `docker`, `events`, `reconciler`, `sysinfo`) depend on the root package, never on each other.

## Tech Stack

- **Language:** Go 1.25+
- **Module:** `github.com/deploykitdev/deploykit`
- **Database:** SQLite (embedded, single-file)
- **Container runtime:** Docker (via Docker SDK for Go)
- **Frontend:** React Router v7 + Vite + TypeScript (SPA embedded into Go binary via `go:embed`)
- **Host metrics:** `github.com/shirou/gopsutil/v4` (used by `sysinfo/` for CPU/memory/disk)

## Project Structure

```
deploykit.go       - Package marker
project.go         - Project domain type and ProjectService interface
user.go            - User domain type and UserService interface
auth.go            - Session, APIKey types and AuthService interface
service.go         - Service domain type and ServiceService interface
deployment.go      - Deployment domain type and DeploymentService interface
container.go       - Container domain type and ContainerService interface
canvas.go          - CanvasNode, CanvasEdge types and CanvasService interface
system.go          - SystemAbout, SystemStatus types and SystemService interface
envvar.go          - EnvVar type and EnvVarService interface (project + service scope)
pending_change.go  - PendingChange types and PendingChangeService (staged-edit changelog)
bus.go             - EventBus interface and event payload types
logs.go            - LogLine type and LogStreamer interface
provisioner.go     - Provisioner interface (network management)
slug.go            - Slug generation utility
errors.go          - Domain error types and codes (ECONFLICT, EINTERNAL, etc.)
cmd/
  deploykitd/      - Main binary entry point, config, graceful shutdown
http/              - HTTP server, routes, handlers, middleware (auth, CORS)
  canvas_hub.go    - WebSocket hub for real-time canvas collaboration
  spa_prod.go      - Embedded SPA file server (production, go:embed)
  spa_dev.go       - Dev stub (returns message to use Vite dev server)
  spa_assets/dist/ - Vite build output (gitignored, embedded into binary)
sqlite/            - SQLite service implementations, migrations
docker/            - Docker client, network provisioning, container log streaming
events/            - In-process EventBus implementation (non-blocking pub/sub)
reconciler/        - Periodic reconciliation loop (desired DB state vs Docker state)
sysinfo/           - SystemService implementation (gopsutil + Docker daemon introspection)
frontend/          - React Router + Vite + TypeScript SPA
  app/             - React source (routes, components, lib)
    routes/settings/ - Admin screens (general, users, system, about)
  vite.config.ts   - Vite config (builds to ../http/spa_assets/dist)
docs/              - Project documentation
```

## Service Interfaces

All defined in the root `deploykit` package:

- **`ProjectService`** — CRUD + filtered listing for projects
- **`UserService`** — CRUD + filtered listing for users (bcrypt password hashing)
- **`AuthService`** — Login, token refresh, logout, API key management, first-user registration gate
- **`ServiceService`** — CRUD for services within projects
- **`DeploymentService`** — Create, list, get, rollback deployments for a service
- **`ContainerService`** — Create, get, list, update status, delete container records
- **`CanvasService`** — Get canvas state, upsert/delete nodes and edges, batch position updates
- **`SystemService`** — Host, Docker, and DB introspection (`About`, `Status`); tolerates unreachable Docker by returning partial data
- **`EnvVarService`** — CRUD for project- and service-scoped env vars; `ResolveForService` overlays project vars with service overrides
- **`PendingChangeService`** — Append-only changelog of staged edits per project; `Apply` mutates real state in one transaction and clears the log
- **`EventBus`** — Non-blocking in-process pub/sub for service/container/deployment events; slow subscribers drop messages rather than backpressure publishers
- **`LogStreamer`** — Stream container logs from the runtime (tail + follow) onto a caller-owned channel
- **`Provisioner`** — Network management (create/remove/list Docker networks)

## Authentication

- **Session tokens:** Access tokens (15 min TTL) + refresh tokens (7 day TTL) with rotation on refresh
- **API keys:** Long-lived tokens with optional expiration, tracked `last_used_at`
- **Security:** Plaintext tokens never stored — SHA-256 hashed in DB; passwords bcrypt-hashed
- **First-user gate:** `CanRegister()` returns true only when no users exist
- **Rate limiting:** 5 login attempts per 15 minutes per email
- **Middleware:** `authenticate()` checks Bearer token (tries session first, then API key)
- **Admin-only routes:** `adminOnly` middleware gates `/api/system/*` and other admin endpoints by checking the authenticated user's role

## Reconciler

The reconciler (`reconciler/`) runs a periodic loop (default 30s) that syncs desired state from the database with actual Docker state. It creates networks for new projects, reconciles containers from active deployments, and cleans up orphaned networks. It can also be triggered on-demand after project/deployment changes and prevents concurrent reconciliation cycles.

## Pending Changes

Deploy-impacting edits (project rename, service create/update/delete, env var CRUD) are not applied immediately — they are appended to a per-project `pending_changes` log via `PendingChangeService.Append`. The frontend renders the staged diff and the user clicks **Apply** to commit. `Apply` walks entries in sequence inside one transaction, resolves `target_temp_id` references for newly-created services, refreshes deployments for services whose env vars changed, clears the log on success, and rolls back on any error. Created deployments are returned in the result so callers can publish `EventDeploymentCreated` on the bus.

## Testing

- Table-driven tests in `sqlite/` package
- Test helpers in `sqlite/sqlite_test.go`: `MustOpenDB(t)`, `MustCreateProject()`, `MustCreateUser()`, `MustCreateAuthUser()`
- Tests use in-memory SQLite databases (`:memory:`)

## Configuration

CLI flags with defaults: `-addr` (`:8080`), `-db` (`deploykit.db`), `-log-level` (`info`), `-cors-origin` (`*`), `-reconcile-interval` (`30s`)

## API Routing

All backend routes are prefixed with `/api/` (e.g., `/api/auth/login`, `/api/projects`). The root path `/` serves the embedded SPA. This separation allows the SPA catch-all to handle client-side routing without conflicting with API endpoints.

## Frontend

- **Stack:** React Router v7 (SPA mode) + Vite + TypeScript
- **Source:** `frontend/app/` — routes, components, auth context, API client
- **Build output:** `http/spa_assets/dist/` — embedded into Go binary via `go:embed`
- **Dev workflow:** Two terminals — `make dev` (Go backend on :8080) + `make dev-frontend` (Vite on :5173 with API proxy)
- **Build tags:** `go run -tags dev` skips SPA embedding so the backend can run without building the frontend
- **Data fetching:** TanStack Query for declarative fetching and caching
- **Canvas:** React Flow-based collaborative canvas with real-time WebSocket sync, cursor tracking, connected users display, context menu, and Railway-style controls. Clicking a service node opens a side panel with details and live-streaming container logs.
- **Pages:** Login, Register, Projects list (with create dialog), Project detail with canvas + side panel, Project Settings (rename, env vars, pending changes), Profile, Settings (General, Users, System, About — admin-only)

## Commands

- `make build` - Build production binary (frontend + Go) to `dist/deploykitd`
- `make dev` - Run Go backend in dev mode (no embedded SPA)
- `make dev-frontend` - Run Vite dev server with API proxy to :8080
- `make frontend` - Build only the frontend
- `go test ./...` - Run all tests
- `go vet ./...` - Run static analysis
- `golangci-lint run` - Run linter (if installed)

## Conventions

- Use standard library where possible (`net/http`, `encoding/json`, `log/slog`)
- Domain types and service interfaces live in the root `deploykit` package
- Implementation packages import root, never each other
- Error handling: return errors, don't panic; wrap with `fmt.Errorf("context: %w", err)`
- Logging: use `log/slog` structured logging
- API responses: JSON, use consistent envelope format
- HTTP handlers return domain errors via `s.errorResponse(w, r, err)`, which maps error codes (`ECONFLICT`, `ENOTFOUND`, `EINVALID`, `EUNAUTHORIZED`, `EFORBIDDEN`, …) to HTTP status codes — don't write statuses directly
- Tests: table-driven tests, use `testing` package
- Naming: follow Go conventions (camelCase unexported, PascalCase exported)
- Build output: binaries go in `dist/` (gitignored), never in the project root
