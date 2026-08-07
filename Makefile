.PHONY: dev run test build worker-image

dev:
	@command -v air >/dev/null 2>&1 || { echo "缺少 air，请先执行: go install github.com/air-verse/air@latest"; exit 1; }
	@air & server_pid=$$!; \
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
