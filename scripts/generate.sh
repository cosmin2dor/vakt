#!/usr/bin/env bash
# Regenerates Go and TypeScript types from schema/openapi.yaml (SDD.md
# §2.5). Called by `make generate`; not meant to be run standalone from
# an arbitrary working directory.
#
# Go types: oapi-codegen (github.com/oapi-codegen/oapi-codegen/v2),
# `types` only, into internal/model — config in schema/oapi-codegen.yaml.
# TypeScript types: openapi-typescript, into web/src/lib/api-types.ts.
#
# Both tools are invoked at a pinned version via `go run .../pkg@version`
# and `npx package@version`, the same convention `make lint` already uses
# for golangci-lint, so no tool dependency needs to land in go.mod or
# package.json.
#
# schema/directives.yaml (the registry) is out of scope here — the
# registry-constants generator is separate, later work
# (generate-registry-constants).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

OAPI_CODEGEN_VERSION="v2.8.0"
OPENAPI_TYPESCRIPT_VERSION="7.13.0"

echo "generate: Go types (oapi-codegen ${OAPI_CODEGEN_VERSION}) -> internal/model/"
go run "github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@${OAPI_CODEGEN_VERSION}" \
	-config schema/oapi-codegen.yaml schema/openapi.yaml

echo "generate: TypeScript types (openapi-typescript ${OPENAPI_TYPESCRIPT_VERSION}) -> web/src/lib/api-types.ts"
(cd web && npx --yes "openapi-typescript@${OPENAPI_TYPESCRIPT_VERSION}" \
	../schema/openapi.yaml -o src/lib/api-types.ts)

# openapi-typescript's own output doesn't match this repo's Prettier
# style (enforce-web-ci's `npm run format:check` gates on it), so run it
# through web/'s own installed Prettier (not a separately pinned version)
# before committing it — requires `npm --prefix web ci` to have run.
(cd web && npx prettier --write src/lib/api-types.ts)
