# Keep in sync with the `version:` pin in .github/workflows/ci.yml
GOLANGCI_LINT_VERSION := v2.13.2

.PHONY: generate build test lint up down dev install-hooks

# Regenerates Go and TypeScript types from schema/openapi.yaml and
# schema/directives.yaml (SDD.md §2.5). Until the schema and codegen
# pipeline exist (build-codegen-pipeline), this is a no-op so a fresh
# clone can still run every target successfully.
generate:
	@if [ -f schema/openapi.yaml ] && [ -x scripts/generate.sh ]; then \
		scripts/generate.sh; \
	else \
		echo "generate: schema/codegen pipeline not wired up yet, skipping"; \
	fi

build:
	go build ./...

test:
	go test ./...

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) config verify
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

# Builds and runs the one-container stack from docker-compose.yml: the API
# and the built PWA bundle served from a single origin, /vault bind-mounted
# and /config on a named volume (SDD.md §2.3). This is what "make up"
# means now that create-docker-compose has landed.
up:
	docker compose up --build

down:
	docker compose down

# Runs the daemon directly on the host, outside Docker: no image build, no
# volumes, just `go run`. Kept as the fastest inner loop for iterating on
# Go code — reach for this instead of `up` when you don't need the
# container topology.
dev:
	go run ./cmd/vaktd

# Points git at .githooks so pre-commit runs make test and make lint
# before every commit.
install-hooks:
	git config core.hooksPath .githooks
