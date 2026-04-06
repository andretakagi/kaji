VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: dev-frontend dev-backend build lint format docker-build docker-up docker-down clean

dev-frontend:
	cd frontend && bun dev

dev-backend:
	go run .

build:
	cd frontend && bun install && bun run build
	go build $(LDFLAGS) -o kaji .

lint:
	cd frontend && bunx @biomejs/biome check .
	gofmt -l . | grep . && exit 1 || true
	go vet ./...

format:
	cd frontend && bunx @biomejs/biome check --write .
	gofmt -w .

docker-build:
	docker build -t kaji .

docker-up:
	docker compose up -d

docker-down:
	docker compose down

clean:
	rm -rf dist kaji frontend/node_modules
