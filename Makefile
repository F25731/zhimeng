.PHONY: backend-api backend-agent frontend dev-db test build

backend-api:
	cd backend && go run ./cmd/api

backend-agent:
	cd backend && go run ./cmd/agent

frontend:
	cd frontend && npm run serve

dev-db:
	docker compose -f deploy/docker-compose.dev.yml up -d postgres migrate minio

test:
	cd backend && go test ./...

build:
	cd backend && go build ./cmd/api ./cmd/agent ./cmd/reporter ./cmd/init-admin
	cd frontend && npm ci && npm run build
