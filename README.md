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

**Producer** generates tasks with random `type` (0–9) and `value` (0–99), persists them in PostgreSQL with state `received`, and sends them to the consumer via gRPC. Respects a configurable `max_backlog` limit — stops producing when unprocessed tasks reach the threshold. When gRPC fails or the consumer rejects a task, the producer marks it as `stale`. A periodic recovery loop re-submits `stale` tasks and resets them to `received` on success.

**Consumer** receives tasks via gRPC, transitions state to `processing`, sleeps for `value` milliseconds (simulating work), then marks `done`. Implements a token-bucket rate limiter with proportional refill (configurable tasks per time period). Bounds concurrency via a buffered-channel semaphore (`max_workers`). For each completed task, logs the task content and the cumulative value sum for that task's type.

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

# 3. Start consumer first (owns DB migrations, gRPC server must be up before producer)
./bin/consumer --config config.yaml

# 4. Start producer in another terminal
./bin/producer --config config.yaml
```

## Configuration

Configuration is loaded via three layers (highest priority wins):

1. **Embedded defaults** — sensible values baked into the binary via `//go:embed`
2. **YAML file** — `--config config.yaml` (merged on top of defaults)
3. **Environment variables** — prefixed with `TP_`, nested with underscores

```bash
# Override DB host and gRPC port via environment
TP_DB_HOST=mydb TP_GRPC_PORT=9999 ./bin/producer --config config.yaml
```

See [`config.yaml`](config.yaml) for all available options.

### Key configuration options

| Option | Default | Description |
|--------|---------|-------------|
| `producer.rate_per_second` | 2 | Tasks produced per second |
| `producer.max_backlog` | 100 | Stop producing when unprocessed tasks reach this count |
| `consumer.rate_limit` | 10 | Max tasks accepted per time period |
| `consumer.rate_period_ms` | 1000 | Time period for rate limiter (ms) |
| `consumer.max_workers` | 10 | Max concurrent processing goroutines |
| `log.level` | info | Log level: debug, info, warn, error |
| `log.format` | text | Log format: text (console) or json (structured) |

## Development

```bash
make build        # Build both binaries with -ldflags="-s -w" and version injection
make test         # Run tests with -race
make coverage     # Generate HTML coverage report
make lint         # Run golangci-lint
make bench        # Run benchmarks
make proto        # Regenerate gRPC code from .proto
make migrate-up   # Run DB migrations (requires running PostgreSQL)
make migrate-down # Rollback last migration (configurable: MIGRATE_STEPS=N)
make up           # Docker Compose up (build + start)
make down         # Docker Compose down (stop + remove volumes)
make logs         # Tail Docker Compose logs
make clean        # Remove bin/, profiles/, coverage files
```

## Project Structure

```
task-pipeline/
├── cmd/
│   ├── producer/main.go         # Producer entry point (flag-based CLI)
│   └── consumer/main.go         # Consumer entry point (flag-based CLI)
├── internal/
│   ├── app/                     # Shared resource setup (DB, gRPC, metrics, pprof)
│   ├── config/                  # Shared Viper config loading (embedded defaults + YAML + env)
│   ├── db/                      # Shared persistence (sqlc generated + connection + migrations)
│   ├── models/                  # Domain types (TaskState constants)
│   ├── metrics/                 # Prometheus metrics definitions + HTTP server
│   ├── logger/                  # slog wrapper (configurable level + format)
│   ├── producer/                # Producer logic (generation loop, backlog control, stale retry)
│   └── consumer/                # Consumer logic (gRPC handler, rate limiter, worker pool)
├── proto/                       # gRPC protobuf definitions + generated Go code
├── migrations/                  # gomigrate SQL files (embedded via embed package)
├── scripts/                     # Demo scripts (GOGC benchmarking)
├── deploy/
│   ├── prometheus.yml           # Prometheus scrape config
│   ├── helm/                    # Helm chart for Kind deployment
│   └── grafana/                 # Dashboard JSON provisioning (4 dashboards)
├── docker-compose.yml           # Full stack: postgres, producer, consumer, prometheus, grafana
├── Makefile                     # Build, test, lint, coverage, proto, migrate, docker targets
├── Dockerfile.producer          # Multi-stage build for producer
├── Dockerfile.consumer          # Multi-stage build for consumer
├── sqlc.yaml                    # sqlc configuration (PostgreSQL + pgx/v5)
├── .golangci.yml                # Linter configuration
├── config.yaml                  # Default config (local development)
└── config.docker.yaml           # Docker-specific config (container hostnames)
```

### Design decisions

- **`internal/db/`** is the shared persistence package — both services import the same sqlc-generated queries and connection logic, ensuring a single source of truth for the database schema.
- **`internal/config/`** provides a shared config struct with embedded defaults via `//go:embed`. Each service loads the full config but uses only its relevant section.
- **`migrations/`** lives at the project root with an `embed.go` that exposes `migrations.FS`. The consumer runs embedded migrations at startup — no external migration files needed at runtime. The consumer owns migrations to avoid race conditions when both services start simultaneously.
- **No `pkg/` directory** — there are no external consumers, so everything stays in `internal/`.

## Technical Choices

| Choice | Why |
|--------|-----|
| **gRPC** | Type-safe protobuf contracts with compile-time guarantees. HTTP/2 multiplexing outperforms REST for service-to-service communication. No Swagger docs needed — the `.proto` file is the contract. For a 1:1 producer-consumer, a direct RPC is simpler than a message broker (RabbitMQ/Kafka), which would add infrastructure complexity without benefit. Reliability is handled via DB-backed state and stale task retry. |
| **PostgreSQL** | Both services write concurrently — PostgreSQL handles this natively (unlike SQLite). gomigrate and sqlc have first-class support. Running in Docker adds zero extra complexity. |
| **slog (stdlib)** | Go 1.21+ structured logging from the standard library. Configurable levels and formats (text/json) via `TextHandler`/`JSONHandler`. No external dependencies. |
| **Viper** | Industry-standard configuration library. Supports YAML + environment variables + defaults. |
| **sqlc** | Generates type-safe Go code from SQL — no ORM overhead, compile-time query validation, native pgx/v5 support. |
| **pgx/v5** | The most performant Go PostgreSQL driver with built-in connection pooling (`pgxpool`). |
| **Token bucket rate limiter** | Proportional token refill based on elapsed periods, capped at max. Uses `sync.Mutex` because the state is shared across concurrent gRPC handler goroutines — a synchronous check, not a data pipeline. |
| **Buffered channel semaphore** | Bounds concurrent processing goroutines (`max_workers`). Standard library pattern — no external deps. |

### Concurrency design

- **Producer**: single goroutine with two `time.Ticker` loops — one for task generation at a configured rate, another for stale task recovery every 10s. Additional goroutines run Prometheus and pprof HTTP servers.
- **Consumer**: each gRPC call spawns a goroutine for async processing. Concurrency is bounded by a buffered-channel semaphore (`max_workers`). A `sync.WaitGroup` tracks in-flight goroutines for graceful shutdown. The rate limiter uses a `sync.Mutex` to coordinate token acquisition.
- **No channels for data flow**: the architecture is request/response (gRPC) with fire-and-forget processing. Channels would add unnecessary complexity — there is no fan-out/fan-in pattern.
- **Graceful shutdown**: the consumer stops accepting new RPCs via `GracefulStop()`, waits for in-flight tasks to complete via `wg.Wait()`, then shuts down HTTP servers with a 5-second timeout. The producer stops its loop on context cancellation and shuts down HTTP servers.

## Database Migrations

Migrations are embedded in the binary via Go's `embed` package and run automatically at consumer startup. For manual migration during the demo:

```bash
# Apply all pending migrations (while services are running)
make migrate-up

# Rollback the last migration
make migrate-down
```

Migration `000001` creates the `tasks` table with a `task_state` enum (`received`, `processing`, `done`, `stale`).

## Profiling

Both services expose pprof endpoints on configurable ports.

### CPU flame graph

```bash
# Collect 30s CPU profile and open interactive viewer
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/profile?seconds=30

# Or use Makefile targets
make flamegraph-producer    # Opens browser with flame graph
make flamegraph-consumer
```

### Memory profile

```bash
go tool pprof -http=:8080 http://localhost:6060/debug/pprof/heap

# Or use Makefile targets
make heap-producer
make heap-consumer
```

### Goroutine dump

```bash
curl http://localhost:6060/debug/pprof/goroutine?debug=2
```

### GOGC & GOMEMLIMIT

These Go runtime parameters control garbage collector behavior:

```bash
# GOGC=200: GC triggers at 2x live heap (default 100 = 1x)
# Fewer GC cycles → less CPU, but higher memory usage
GOGC=200 ./bin/consumer --config config.yaml

# GOMEMLIMIT=128MiB: hard memory cap
# GC runs more aggressively near the limit → prevents OOM
GOMEMLIMIT=128MiB ./bin/consumer --config config.yaml

# Combined: soft GC target + hard cap
# Best practice: set GOMEMLIMIT to ~90% of container memory limit
GOGC=200 GOMEMLIMIT=256MiB ./bin/consumer --config config.yaml
```

### Profile-Guided Optimization (PGO)

```bash
# 1. Run services under load, then collect CPU profiles
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

## Test Coverage

All business logic packages are at 100% coverage:

```bash
make test       # Run all tests with race detector
make coverage   # Generate HTML coverage report
make bench      # Run benchmarks
```

| Package | Coverage |
|---------|----------|
| `internal/config` | 100% |
| `internal/consumer` | 100% |
| `internal/producer` | 100% |
| `internal/logger` | 100% |
| `internal/metrics` | 100% |
| `internal/db` | 62% (remaining: sqlc-generated queries + real DB functions) |

## Grafana Dashboards

Four dashboards are auto-provisioned:

1. **Messages per State** — received, processing, done, and stale task counts over time
2. **Service Health** — producer/consumer up/down status + producer backlog gauge
3. **Value Sum per Type** — total sum of `value` field for each task type (0–9)
4. **Tasks per Type** — total processed tasks per task type (0–9)

## License

See [LICENSE](LICENSE).
