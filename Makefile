# ==============================================================================
# Super Sparrow - Developer Automation
# ==============================================================================

.PHONY: help install dev dev-server dev-web dev-desktop build build-server build-web check check-go check-web check-desktop test test-go test-web format format-go format-web format-check clean env-file

# The server and the web dev server read their settings from the repository .env.
LOAD_ENV := set -a && . ./.env && set +a

help:
	@echo "Available commands:"
	@echo ""
	@echo "  --- Setup ---"
	@echo "  make install        Install Bun workspace dependencies and download Go modules"
	@echo ""
	@echo "  --- Development ---"
	@echo "  make dev            Run the Go server and the web dev server together"
	@echo "  make dev-server     Build and run the Go server with the settings in .env"
	@echo "  make dev-web        Run the web dev server; it forwards /api to the Go server"
	@echo "  make dev-desktop    Run the Tauri desktop app in development mode"
	@echo ""
	@echo "  --- Build ---"
	@echo "  make build          Build the web app and the server binary"
	@echo ""
	@echo "  --- Checks and tests ---"
	@echo "  make check          Run every check (Go and web)"
	@echo "  make check-go       gofmt, go vet, and staticcheck"
	@echo "  make check-web      Route generation, TypeScript, and Prettier"
	@echo "  make check-desktop  cargo check of the Tauri app (needs the Linux WebKit libraries)"
	@echo "  make test           Run every test suite (Go and web)"
	@echo "  make test-go        Go tests with the race detector"
	@echo "  make test-web       Web tests with bun test"
	@echo ""
	@echo "  --- Formatting ---"
	@echo "  make format         Format the whole repository (gofmt and Prettier)"
	@echo "  make format-check   Check formatting without changing files"
	@echo ""
	@echo "  make clean          Remove build output"

# --- Setup ---
install:
	bun install --frozen-lockfile
	cd server && go mod download

env-file:
	@test -f .env || { echo ".env is missing: run cp .env.example .env"; exit 1; }

# --- Development ---
dev: env-file
	@$(MAKE) --no-print-directory -j2 dev-server dev-web

dev-server: env-file
	cd server && go build -o bin/sparrow ./cmd/sparrow
	@$(LOAD_ENV) && mkdir -p "$$(dirname "$$SPARROW_DATABASE_PATH")" && ./server/bin/sparrow serve

dev-web: env-file
	bun --filter @super-sparrow/web dev

dev-desktop:
	bun --filter @super-sparrow/desktop dev

# --- Build ---
build: build-web build-server

build-web:
	bun --filter @super-sparrow/web build

build-server:
	cd server && go build -trimpath -o bin/sparrow ./cmd/sparrow

# --- Checks ---
check:
	@$(MAKE) --no-print-directory --keep-going check-go check-web

check-go:
	@unformatted="$$(cd server && gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	cd server && go vet ./...
	cd server && go tool staticcheck ./...

check-web:
	bun --filter @super-sparrow/web typecheck
	bun --bun prettier --check "web/**/*.{ts,tsx,css,json}"

check-desktop:
	cd desktop/src-tauri && cargo check --locked

# --- Tests ---
test:
	@$(MAKE) --no-print-directory --keep-going test-go test-web

test-go:
	cd server && go test -race -count=1 ./...

test-web:
	bun --filter @super-sparrow/web test

# --- Formatting ---
format: format-go format-web

format-go:
	cd server && gofmt -w .

format-web:
	bun run format

format-check:
	@unformatted="$$(cd server && gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	bun run format:check

# --- Cleanup ---
clean:
	rm -rf server/bin web/dist web/.tanstack desktop/src-tauri/target
