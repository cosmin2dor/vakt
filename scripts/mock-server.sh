#!/usr/bin/env bash
# Mocks the v1 API straight from schema/openapi.yaml (validate-contract-against-mock).
# Prism generates the server from the spec's schemas/examples — no hand-written
# mock handlers or fixtures, same "generated, never hand-written" rule as
# scripts/generate.sh. Pinned via `npx package@version`, that script's own
# convention, so the tool doesn't need to land in web/package.json.
#
# Prism serves paths exactly as declared in the spec (no /api/v1 prefix), so
# `web/vite.config.ts` proxies /api/v1/* here with the prefix stripped.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

PRISM_VERSION="5.16.0"
PORT="${1:-4010}"

echo "mock-server: Prism ${PRISM_VERSION} serving schema/openapi.yaml on :${PORT}"
npx --yes "@stoplight/prism-cli@${PRISM_VERSION}" mock schema/openapi.yaml -p "${PORT}" --dynamic
