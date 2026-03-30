# DeployKit

A self-hosted PaaS inspired by Railway for deploying applications on a single VM using Docker.

## Architecture

Follows the [WTF Dial](https://github.com/benbjohnson/wtf) pattern by Ben Johnson:

- **Root package (`deploykit`)** — Domain types and service interfaces. Zero dependencies on implementations.
- **`http`** — HTTP server, handlers, and routing. Depends on root domain types.
- **`sqlite`** — Database layer using SQLite. Implements service interfaces from root.
- **`docker`** — Docker client and container/network management. Implements provisioning interfaces.
- **`cmd/deploykitd`** — Main entry point. Wires everything together.

Dependencies flow inward: implementation packages (`http`, `sqlite`, `docker`) depend on the root package, never on each other.

## Tech Stack

- **Language:** Go 1.25+
- **Module:** `github.com/heyjorgedev/deploykit`
- **Database:** SQLite (embedded, single-file)
- **Container runtime:** Docker (via Docker SDK for Go)
- **Frontend (planned):** Nuxt.js SPA

## Project Structure

```
deploykit.go       - Package marker
project.go         - Project domain type and ProjectService interface
user.go            - User domain type and UserService interface
auth.go            - Session, APIKey types and AuthService interface
errors.go          - Domain error types and codes (ECONFLICT, EINTERNAL, etc.)
cmd/
  deploykitd/      - Main binary entry point, config, graceful shutdown
http/              - HTTP server, routes, handlers, middleware (auth, CORS)
sqlite/            - SQLite service implementations, migrations
docker/            - Docker client, container/network management (planned)
```

## Service Interfaces

All defined in the root `deploykit` package:

- **`ProjectService`** — CRUD + filtered listing for projects
- **`UserService`** — CRUD + filtered listing for users (bcrypt password hashing)
- **`AuthService`** — Login, token refresh, logout, API key management, first-user registration gate

## Authentication

- **Session tokens:** Access tokens (15 min TTL) + refresh tokens (7 day TTL) with rotation on refresh
- **API keys:** Long-lived tokens with optional expiration, tracked `last_used_at`
- **Security:** Plaintext tokens never stored — SHA-256 hashed in DB; passwords bcrypt-hashed
- **First-user gate:** `CanRegister()` returns true only when no users exist
- **Rate limiting:** 5 login attempts per 15 minutes per email
- **Middleware:** `authenticate()` checks Bearer token (tries session first, then API key)

## Testing

- Table-driven tests in `sqlite/` package
- Test helpers in `sqlite/sqlite_test.go`: `MustOpenDB(t)`, `MustCreateProject()`, `MustCreateUser()`, `MustCreateAuthUser()`
- Tests use in-memory SQLite databases (`:memory:`)

## Configuration

CLI flags with defaults: `-addr` (`:8080`), `-db` (`deploykit.db`), `-log-level` (`info`), `-cors-origin` (`*`)

## Commands

- `go build -o dist/deploykitd ./cmd/deploykitd` - Build the server binary (binaries go in `dist/`)
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
- Build output: binaries go in `dist/` (gitignored), never in the project root
