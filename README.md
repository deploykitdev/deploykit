# DeployKit

> ⚠️ **Warning: This project is totally vibe-coded and has not been tested enough to be considered production-ready.**
> Do not run it against anything you care about. Expect breaking changes, sharp edges, and incomplete features.

A self-hosted PaaS for deploying applications on a single VM using Docker.

## What it does

DeployKit gives you a visual canvas for laying out projects, services, and their connections, then deploys them as Docker containers on a single host. It includes:

- A collaborative React Flow canvas with real-time WebSocket sync
- Project- and service-scoped environment variables
- Staged "pending changes" that you review and apply in one shot
- A reconciler loop that keeps Docker state in sync with the database
- Session + API key auth, bcrypt passwords, admin role gating
- An embedded SPA shipped inside a single Go binary

## Architecture

Follows the [WTF Dial](https://github.com/benbjohnson/wtf) layout by Ben Johnson. The root package holds domain types and service interfaces; implementation packages depend on the root, never on each other.

```
deploykit/          - Domain types and service interfaces
cmd/deploykitd/     - Main binary entry point
http/               - HTTP server, handlers, embedded SPA
sqlite/             - SQLite implementations of service interfaces
docker/             - Docker client, networks, log streaming
events/             - In-process pub/sub event bus
reconciler/         - Periodic DB <-> Docker reconciliation
sysinfo/            - Host/Docker/DB introspection
frontend/           - React Router SPA source
```

See [CLAUDE.md](CLAUDE.md) for a deeper walkthrough of the service interfaces, auth flow, pending-changes model, and reconciler.

## Running it

### Prerequisites

- Go 1.25+
- Node.js (for the frontend)
- A running Docker daemon

### Development

Run the backend and frontend in two terminals:

```bash
make dev           # Go backend on :8080 (no embedded SPA)
make dev-frontend  # Vite dev server on :5173, proxies API to :8080
```

Open `http://localhost:5173`. The first user you register becomes the admin.

### Production build

```bash
make build         # builds frontend, embeds it, outputs dist/deploykitd
./dist/deploykitd  # run the single binary
```

### Configuration

CLI flags:

| Flag                  | Default        | Description                            |
| --------------------- | -------------- | -------------------------------------- |
| `-addr`               | `:8080`        | HTTP listen address                    |
| `-db`                 | `deploykit.db` | SQLite database path                   |
| `-log-level`          | `info`         | `debug`, `info`, `warn`, `error`       |
| `-cors-origin`        | `*`            | Allowed CORS origin                    |
| `-reconcile-interval` | `30s`          | How often the reconciler loop runs     |

## Testing

```bash
go test ./...      # run all tests
go vet ./...       # static analysis
```

Tests use in-memory SQLite (`:memory:`) and the helpers in `sqlite/sqlite_test.go`.

## Status

Vibe-coded. Use at your own risk. Issues and PRs welcome, but don't expect a stable API yet.
