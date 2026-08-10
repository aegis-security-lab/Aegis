.PHONY: dev dev-stop run test build worker-image

dev:
	@test -n "$$AEGIS_PASSWORD" || { echo "请先设置 AEGIS_PASSWORD"; exit 1; }
	@command -v air >/dev/null 2>&1 || { echo "缺少 air，请先执行: go install github.com/air-verse/air@latest"; exit 1; }
	@mkdir -p tmp; \
	air & air_pid=$$!; \
	npm run dev & vite_pid=$$!; \
	echo $$air_pid > tmp/air.pid; \
	echo $$vite_pid > tmp/vite.pid; \
	cleanup() { \
		trap - EXIT INT TERM HUP; \
		kill -TERM $$vite_pid $$air_pid 2>/dev/null || true; \
		wait $$vite_pid $$air_pid 2>/dev/null || true; \
		rm -f tmp/air.pid tmp/vite.pid; \
	}; \
	trap cleanup EXIT INT TERM HUP; \
	wait $$vite_pid

dev-stop:
	@for pid_file in tmp/air.pid tmp/vite.pid; do \
		if test -f "$$pid_file"; then \
			pid=$$(cat "$$pid_file"); \
			case "$$pid" in (*[!0-9]*|'') ;; (*) kill -TERM "$$pid" 2>/dev/null || true ;; esac; \
		fi; \
	done; \
	rm -f tmp/air.pid tmp/vite.pid

run:
	@test -n "$$AEGIS_PASSWORD" || { echo "请先设置 AEGIS_PASSWORD"; exit 1; }
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
