#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"

if command -v jq >/dev/null 2>&1; then
  curl --silent --show-error --fail "${API_BASE_URL}/tool/definitions" \
    | jq '.items | map({id, active, name: .tool.function.name, method: .upstream.method, url: .upstream.url})'
else
  curl --silent --show-error --fail "${API_BASE_URL}/tool/definitions"
fi
