.PHONY: dev run test build worker-image

dev:
	@go run ./cmd/server & server_pid=$$!; \
	trap 'kill $$server_pid 2>/dev/null || true' EXIT INT TERM; \
	npm run dev

run:
	npm run build
	go run ./cmd/server

test:
	go test ./cmd/... ./internal/...
	npm run typecheck
	npm run lint

build:
	npm run build
	@mkdir -p bin
	go build -o bin/aegis ./cmd/server

worker-image:
	./build-worker-image.sh
