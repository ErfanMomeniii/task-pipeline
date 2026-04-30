VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: all build build-producer build-consumer build-pgo test lint coverage clean proto migrate-up migrate-down up down logs pgo-collect flamegraph-producer flamegraph-consumer bench build-docker kind-load helm-install helm-uninstall

all: build

## Build -----------------------------------------------------------------------

build: build-producer build-consumer

build-producer:
	go build $(LDFLAGS) -o bin/producer ./cmd/producer

build-consumer:
	go build $(LDFLAGS) -o bin/consumer ./cmd/consumer

## PGO (Profile-Guided Optimization) ------------------------------------------

PGO_DURATION ?= 30

pgo-collect:
	@echo "Collecting CPU profiles for $(PGO_DURATION)s..."
	@mkdir -p profiles
	curl -s "http://localhost:6060/debug/pprof/profile?seconds=$(PGO_DURATION)" -o profiles/producer.pprof &
	curl -s "http://localhost:6061/debug/pprof/profile?seconds=$(PGO_DURATION)" -o profiles/consumer.pprof &
	wait
	@echo "Profiles saved to profiles/"

build-pgo: pgo-collect
	go build -pgo=profiles/producer.pprof $(LDFLAGS) -o bin/producer ./cmd/producer
	go build -pgo=profiles/consumer.pprof $(LDFLAGS) -o bin/consumer ./cmd/consumer
	@echo "PGO-optimized binaries built in bin/"

## Quality ---------------------------------------------------------------------

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

coverage:
	go test ./... -race -coverprofile=coverage.out -covermode=atomic
	go tool cover -html=coverage.out -o coverage.html

## Protobuf --------------------------------------------------------------------

proto:
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/*.proto

## Database --------------------------------------------------------------------

MIGRATE_DSN ?= postgres://taskpipeline:taskpipeline@localhost:5432/taskpipeline?sslmode=disable

migrate-up:
	migrate -database "$(MIGRATE_DSN)" -path migrations up

MIGRATE_STEPS ?= 1

migrate-down:
	migrate -database "$(MIGRATE_DSN)" -path migrations down $(MIGRATE_STEPS)

## Docker ----------------------------------------------------------------------

up:
	docker compose up --build -d

down:
	docker compose down -v

logs:
	docker compose logs -f

## Profiling -------------------------------------------------------------------

PPROF_DURATION ?= 30

flamegraph-producer:
	@mkdir -p profiles
	@echo "Collecting $(PPROF_DURATION)s CPU profile from producer (localhost:6060)..."
	go tool pprof -http=:8090 "http://localhost:6060/debug/pprof/profile?seconds=$(PPROF_DURATION)"

flamegraph-consumer:
	@mkdir -p profiles
	@echo "Collecting $(PPROF_DURATION)s CPU profile from consumer (localhost:6061)..."
	go tool pprof -http=:8091 "http://localhost:6061/debug/pprof/profile?seconds=$(PPROF_DURATION)"

heap-producer:
	go tool pprof -http=:8090 "http://localhost:6060/debug/pprof/heap"

heap-consumer:
	go tool pprof -http=:8091 "http://localhost:6061/debug/pprof/heap"

## Benchmark -------------------------------------------------------------------

bench:
	go test -bench=. -benchmem ./...

## Kind + Helm -----------------------------------------------------------------

build-docker:
	docker build -f Dockerfile.producer -t task-pipeline-producer:latest .
	docker build -f Dockerfile.consumer -t task-pipeline-consumer:latest .

kind-load: build-docker
	kind load docker-image task-pipeline-producer:latest
	kind load docker-image task-pipeline-consumer:latest

helm-install:
	helm upgrade --install task-pipeline ./deploy/helm

helm-uninstall:
	helm uninstall task-pipeline

## Cleanup ---------------------------------------------------------------------

clean:
	rm -rf bin/ profiles/ coverage.out coverage.html
