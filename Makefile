.PHONY: help up down migrate-up migrate-down sqlc run-api run-worker test lint clean

help:
	@echo "Available BioVerse commands:"
	@echo "  make up           - Start PostgreSQL and MinIO containers via Docker Compose"
	@echo "  make down         - Stop Docker Compose containers"
	@echo "  make migrate-up   - Apply database migrations using goose"
	@echo "  make migrate-down - Rollback the last migration using goose"
	@echo "  make sqlc         - Generate typed Go code from SQL queries"
	@echo "  make run-api      - Run HTTP API server"
	@echo "  make run-worker   - Run background worker"
	@echo "  make test         - Run all unit and integration tests"
	@echo "  make lint         - Run go vet and code validation"
	@echo "  make clean        - Clean binary and temporary files"

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

migrate-up:
	goose -dir db/migrations postgres "$${DATABASE_URL:-postgres://bioverse:bioverse_dev@127.0.0.1:5432/bioverse?sslmode=disable}" up

migrate-down:
	goose -dir db/migrations postgres "$${DATABASE_URL:-postgres://bioverse:bioverse_dev@127.0.0.1:5432/bioverse?sslmode=disable}" down

sqlc:
	sqlc generate

run-api:
	go run cmd/api/main.go

run-worker:
	go run cmd/worker/main.go

test:
	go test -v -race ./...

lint:
	go vet ./...

clean:
	rm -rf bin/ tmp/ coverage.out coverage.html
