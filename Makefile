.PHONY: build run-collector run-analyzer run-api run-alerter run-trader run-trader-skip infra-up infra-down tidy test lint

# Build all binaries
build: tidy
	go build -o bin/collector ./cmd/collector
	go build -o bin/analyzer ./cmd/analyzer
	go build -o bin/api ./cmd/api
	go build -o bin/alerter ./cmd/alerter
	go build -o bin/trader ./cmd/trader

# Run individual services
run-collector:
	go run ./cmd/collector -config configs/config.yaml

run-analyzer:
	go run ./cmd/analyzer -config configs/config.yaml

# Batch classification via Qwen-Plus API (8 workers, skip Ollama)
run-analyzer-batch:
	go run ./cmd/analyzer -config configs/config.yaml -batch -workers 8

run-api:
	go run ./cmd/api -config configs/config.yaml

run-alerter:
	go run ./cmd/alerter -config configs/config.yaml

run-trader:
	go run ./cmd/trader -config configs/config.yaml

# Run trader without startup backtest (skip backfill)
run-trader-skip:
	go run ./cmd/trader -config configs/config.yaml -skip-backtest

# Infrastructure
infra-up:
	docker compose -f deployments/docker-compose.yml up -d

infra-down:
	docker compose -f deployments/docker-compose.yml down

infra-reset:
	docker compose -f deployments/docker-compose.yml down -v
	docker compose -f deployments/docker-compose.yml up -d

# Development
tidy:
	go mod tidy

test:
	go test ./...

lint:
	golangci-lint run ./...
