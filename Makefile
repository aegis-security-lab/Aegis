.PHONY: dev dev-stop dev-local dev-local-stop up down logs run test build app-image worker-image

dev:
	docker compose -f compose.yaml -f compose.dev.yaml up --build

dev-stop:
	docker compose -f compose.yaml -f compose.dev.yaml down --remove-orphans

dev-local:
	@test -n "$$AEGIS_PASSWORD" || { echo "请先设置 AEGIS_PASSWORD"; exit 1; }
	@command -v air >/dev/null 2>&1 || { echo "缺少 air，请先执行: go install github.com/air-verse/air@latest"; exit 1; }
	@mkdir -p tmp; \
	air & air_pid=$$!; \
	npm run dev -- --host 0.0.0.0 & vite_pid=$$!; \
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

dev-local-stop:
	@for pid_file in tmp/air.pid tmp/vite.pid; do \
		if test -f "$$pid_file"; then \
			pid=$$(cat "$$pid_file"); \
			case "$$pid" in (*[!0-9]*|'') ;; (*) kill -TERM "$$pid" 2>/dev/null || true ;; esac; \
		fi; \
	done; \
	rm -f tmp/air.pid tmp/vite.pid

up:
	docker compose up --build --detach

down:
	docker compose down --remove-orphans

logs:
	docker compose logs --follow aegis

run:
	@test -n "$$AEGIS_PASSWORD" || { echo "请先设置 AEGIS_PASSWORD"; exit 1; }
	npm run build
	go run ./cmd/server

test:
	go test $$(go list ./... | grep -v '/node_modules/')
	npm run typecheck
	npm run lint -- --max-warnings=0

build:
	npm run build
	@rm -rf internal/webui/dist
	@mkdir -p internal/webui/dist
	@cp -R dist/. internal/webui/dist/
	@mkdir -p bin
	go build -o bin/aegis ./cmd/server

app-image:
	docker build --file Dockerfile.app --target production --tag "$${AEGIS_IMAGE:-aegis:latest}" .

worker-image:
	./build-worker-image.sh
