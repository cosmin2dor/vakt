GOLANGCI_LINT_VERSION := v2.13.2

.PHONY: generate build test lint up

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
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

# Runs the daemon locally. docker-compose (create-docker-compose) will add
# the containerized path; this is the fastest loop until then.
up:
	go run ./cmd/vaktd
