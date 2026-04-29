VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: all build build-producer build-consumer test lint coverage clean proto migrate-up migrate-down up down logs

all: build

## Build -----------------------------------------------------------------------

build: build-producer build-consumer

build-producer:
	go build $(LDFLAGS) -o bin/producer ./cmd/producer

build-consumer:
	go build $(LDFLAGS) -o bin/consumer ./cmd/consumer

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

migrate-down:
	migrate -database "$(MIGRATE_DSN)" -path migrations down

## Docker ----------------------------------------------------------------------

up:
	docker compose -f deploy/docker-compose.yml up --build -d

down:
	docker compose -f deploy/docker-compose.yml down -v

logs:
	docker compose -f deploy/docker-compose.yml logs -f

## Cleanup ---------------------------------------------------------------------

clean:
	rm -rf bin/ coverage.out coverage.html
