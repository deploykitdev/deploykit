# DeployKit

A self-hosted PaaS inspired by Railway for deploying applications on a single VM using Docker.

## Architecture

Follows the [WTF Dial](https://github.com/benbjohnson/wtf) pattern by Ben Johnson:

- **Root package (`deploykit`)** — Domain types and service interfaces. Zero dependencies on implementations.
- **`http`** — HTTP server, handlers, and routing. Depends on root domain types.
- **`sqlite`** — Database layer using SQLite. Implements service interfaces from root.
- **`docker`** — Docker client and container/network management. Implements provisioning interfaces.
- **`cmd/deploykit`** — Main entry point. Wires everything together.

Dependencies flow inward: implementation packages (`http`, `sqlite`, `docker`) depend on the root package, never on each other.

## Tech Stack

- **Language:** Go 1.25+
- **Module:** `github.com/heyjorgedev/deploykit`
- **Database:** SQLite (embedded, single-file)
- **Container runtime:** Docker (via Docker SDK for Go)
- **Frontend (planned):** Nuxt.js SPA

## Project Structure

```
deploykit.go       - Core domain types (Project, Resource, Network)
errors.go          - Domain error types
cmd/
  deploykit/       - Main binary entry point
http/              - HTTP server, routes, handlers
sqlite/            - SQLite service implementations, migrations
docker/            - Docker client, container/network management
```

## Commands

- `go build ./cmd/deploykitd` - Build the server binary
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
- Tests: table-driven tests, use `testing` package
- Naming: follow Go conventions (camelCase unexported, PascalCase exported)
