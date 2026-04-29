# Task Pipeline

Two Go microservices (producer/consumer) communicating via gRPC, with PostgreSQL persistence, Prometheus monitoring, and Grafana dashboards.

## Architecture

```
┌──────────┐    gRPC     ┌──────────┐
│ Producer │────────────▶│ Consumer │
│          │  SubmitTask │          │
└────┬─────┘             └────┬─────┘
     │                        │
     │   ┌──────────────┐     │
     └──▶│  PostgreSQL  │◀────┘
         │  (tasks DB)  │
         └──────────────┘
              ▲
     ┌────────┴────────┐
     │   Prometheus    │
     └────────┬────────┘
              ▼
     ┌─────────────────┐
     │     Grafana     │
     │  (4 dashboards) │
     └─────────────────┘
```

**Producer** generates tasks with random `type` (0–9) and `value` (0–99), persists them in PostgreSQL with state `received`, and sends them to the consumer via gRPC. Respects a configurable `max_backlog` limit — stops producing when the number of unprocessed tasks reaches the threshold.

**Consumer** receives tasks via gRPC, transitions state to `processing`, sleeps for `value` milliseconds (simulating work), then marks `done`. Implements a token-bucket rate limiter (configurable tasks per time period). For each completed task, logs the task content and the cumulative value sum for that task's type.

## Quick Start

### Prerequisites

- Go 1.26+
- Docker & Docker Compose
- Make
- protoc + Go plugins (for proto regeneration only)

### Run with Docker Compose

```bash
make up       # Build and start all services (postgres, consumer, producer, prometheus, grafana)
make logs     # Tail logs from all containers
make down     # Stop and clean up (removes volumes)
```

Services available at:

| Service | URL |
|---------|-----|
| Grafana | http://localhost:3000 (admin/admin) |
| Prometheus | http://localhost:9092 |
| Producer metrics | http://localhost:9090/metrics |
| Consumer metrics | http://localhost:9091/metrics |
| Producer pprof | http://localhost:6060/debug/pprof |
| Consumer pprof | http://localhost:6061/debug/pprof |

### Run Locally

```bash
# 1. Start PostgreSQL
docker run -d --name pg -e POSTGRES_USER=taskpipeline \
  -e POSTGRES_PASSWORD=taskpipeline -e POSTGRES_DB=taskpipeline \
  -p 5432:5432 postgres:17-alpine

# 2. Build both binaries
make build

# 3. Start consumer first (gRPC server must be up before producer connects)
./bin/consumer --config config.yaml

# 4. Start producer in another terminal
./bin/producer --config config.yaml
```

## Configuration

Configuration is loaded via three layers (highest priority wins):

1. **Defaults** — sensible values hardcoded in the application
2. **YAML file** — `--config config.yaml`
3. **Environment variables** — prefixed with `TP_`, nested with underscores

```bash
# Override DB host and gRPC port via environment
TP_DB_HOST=mydb TP_GRPC_PORT=9999 ./bin/producer --config config.yaml
```

See [`config.yaml`](config.yaml) for all available options with their defaults.

### Key configuration options

| Option | Default | Description |
|--------|---------|-------------|
| `producer.rate_per_second` | 2 | How many tasks per second to produce |
| `producer.max_backlog` | 100 | Stop producing when unprocessed tasks reach this count |
| `consumer.rate_limit` | 10 | Max tasks accepted per time period |
| `consumer.rate_period_ms` | 1000 | Time period for rate limiter (ms) |
| `log.level` | info | Log level: debug, info, warn, error |
| `log.format` | text | Log format: text (console) or json (structured) |

## Development

```bash
make build        # Build both binaries with -ldflags="-s -w" and version injection
make test         # Run tests with -race
make coverage     # Generate HTML coverage report
make lint         # Run golangci-lint
make proto        # Regenerate gRPC code from .proto
make migrate-up   # Run DB migrations (requires running PostgreSQL)
make migrate-down # Rollback migrations
make up           # Docker Compose up (build + start)
make down         # Docker Compose down (stop + remove volumes)
make logs         # Tail Docker Compose logs
```

## Project Structure

```
task-pipeline/
├── cmd/
│   ├── producer/main.go         # Producer CLI entry point (Cobra)
│   └── consumer/main.go         # Consumer CLI entry point (Cobra)
├── internal/
│   ├── config/                  # Shared Viper config loading (YAML + env + defaults)
│   ├── db/                      # Shared persistence (sqlc generated code + connection + migrations)
│   ├── models/                  # Domain types (TaskState constants) shared across services
│   ├── metrics/                 # Prometheus metrics definitions + HTTP server
│   ├── logger/                  # slog wrapper (configurable level + format)
│   ├── producer/                # Producer business logic (generation loop, backlog control)
│   └── consumer/                # Consumer business logic (gRPC handler, rate limiter, processing)
├── proto/                       # gRPC protobuf definitions + generated Go code
├── migrations/                  # gomigrate SQL files (embedded via embed package)
├── deploy/
│   ├── docker-compose.yml       # Full stack: postgres, producer, consumer, prometheus, grafana
│   ├── prometheus.yml           # Prometheus scrape config
│   └── grafana/                 # Dashboard JSON provisioning (4 dashboards)
├── Makefile                     # Build, test, lint, coverage, proto, migrate, docker targets
├── Dockerfile.producer          # Multi-stage build for producer
├── Dockerfile.consumer          # Multi-stage build for consumer
├── sqlc.yaml                    # sqlc configuration (PostgreSQL + pgx/v5)
├── config.yaml                  # Default config (local development)
└── config.docker.yaml           # Docker-specific config (container hostnames)
```

### Design decisions on shared code

- **`internal/models/`** defines domain types (`TaskState` constants) used across both services — single source of truth for state values.
- **`internal/db/`** is the shared persistence package — both services import the same sqlc-generated queries and connection logic. This ensures a single source of truth for the database schema and avoids drift between producer and consumer.
- **`internal/config/`** provides a shared config struct — each service loads the full config but uses only its relevant section.
- **`migrations/`** lives at the project root with an `embed.go` file that exposes `migrations.FS`. The `internal/db` package imports this to run embedded migrations at startup — no external migration files needed at runtime.
- **No `pkg/` directory** — there are no external consumers, so everything stays in `internal/`.

## Technical Choices

| Choice | Why |
|--------|-----|
| **gRPC (HTTP/2)** | Type-safe protobuf contracts with compile-time guarantees. HTTP/2 multiplexing is more scalable than REST for high-throughput service-to-service communication. No Swagger documentation needed — the `.proto` file is the contract. |
| **PostgreSQL** | Both services write to the same database concurrently — PostgreSQL handles concurrent connections natively (unlike SQLite). gomigrate and sqlc have first-class PostgreSQL support. Running in Docker adds zero extra complexity. |
| **slog (stdlib)** | Go 1.21+ standard library structured logging. Supports configurable levels (debug/info/warn/error) and formats (text/json) via `TextHandler`/`JSONHandler`. No external dependencies needed. |
| **Viper** | Industry-standard Go configuration library. Supports YAML files + environment variables + defaults. Listed in the spec's helpful materials. |
| **sqlc** | Generates type-safe Go code from SQL queries — no ORM overhead, compile-time query validation, and the generated code uses `pgx/v5` natively. |
| **pgx/v5** | The most performant Go PostgreSQL driver with built-in connection pooling (`pgxpool`). |
| **Token bucket rate limiter** | Simple, effective rate limiting with mutex protection. Justified use of `sync.Mutex`: the rate limiter state is shared across concurrent gRPC handler goroutines. No channels needed — the limiter is a synchronous check, not a data pipeline. |

### Concurrency design

- **Producer**: single goroutine with a `time.Ticker` loop. No concurrency needed — tasks are generated sequentially at a configured rate. Additional goroutines run the Prometheus and pprof HTTP servers.
- **Consumer**: each incoming gRPC call spawns a goroutine for async processing (`go c.process(...)`) so the gRPC response returns immediately. The rate limiter uses a `sync.Mutex` to safely coordinate token acquisition across concurrent handlers.
- **No channels**: the architecture is request/response (gRPC) with fire-and-forget processing. Channels would add unnecessary complexity — there's no data pipeline or fan-out/fan-in pattern.

## Database Migrations

Migrations are embedded in the binary via Go's `embed` package and run automatically at startup. For manual migration during the demo:

```bash
# Apply all pending migrations (while services are running)
make migrate-up

# Rollback the last migration (removes "comment" column)
make migrate-down
```

Migration 000001 creates the `tasks` table. Migration 000002 adds/removes a `comment` column — designed for demonstrating live migration up/down while services are running.

## Profiling

Both services expose pprof endpoints on configurable ports.

### Flame graph (CPU profile)

```bash
# Collect a 30-second CPU profile and open interactive viewer
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30
```

### Memory profile

```bash
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/heap
```

### Goroutine dump

```bash
curl http://localhost:6060/debug/pprof/goroutine?debug=2
```

### GOGC & GOMEMLIMIT

These Go runtime parameters control the garbage collector behavior:

```bash
# GOGC=200: GC triggers at 2x live heap (default 100 = 1x)
# Effect: fewer GC cycles → less CPU, but higher memory usage
GOGC=200 ./bin/consumer --config config.yaml

# GOMEMLIMIT=128MiB: hard memory cap
# Effect: GC runs more aggressively near the limit → prevents OOM
GOMEMLIMIT=128MiB ./bin/consumer --config config.yaml

# Combined: soft GC target via GOGC + hard cap via GOMEMLIMIT
# Best practice for production: set GOMEMLIMIT to ~90% of container memory limit
GOGC=200 GOMEMLIMIT=256MiB ./bin/consumer --config config.yaml
```

### Profile-Guided Optimization (PGO)

```bash
# 1. Run services, then collect CPU profiles
make pgo-collect    # Fetches 30s profiles from both services

# 2. Rebuild with PGO
make build-pgo      # Compiles with -pgo flag using collected profiles
```

PGO uses runtime CPU profiles to optimize hot paths at compile time, typically yielding 2–7% performance improvement.

## Build Flags

```bash
# Production build: stripped symbols (-s -w) + version injection
VERSION=v1.0.0 make build

# Verify version
./bin/producer -version   # prints: v1.0.0
./bin/consumer -version   # prints: v1.0.0
```

Build uses `-ldflags="-s -w -X main.version=$(VERSION)"`:
- `-s` strips the symbol table
- `-w` strips DWARF debug information
- `-X main.version=...` injects the build version at compile time

## Grafana Dashboards

Four dashboards are auto-provisioned:

1. **Messages per State** — received, processing, and done task counts over time
2. **Service Health** — producer/consumer up/down status + producer backlog gauge
3. **Value Sum per Type** — total sum of `value` field for each task type (0–9)
4. **Tasks per Type** — total processed tasks per task type (0–9)

## License

See [LICENSE](LICENSE).
