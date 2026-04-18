#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
IDS_FILE="${SCRIPT_DIR}/.tool_ids.env"
CEP="${CEP:-70100000}"
CALL_ID="${CALL_ID:-call_viacep}"

if [[ ! -f "${IDS_FILE}" ]]; then
  echo "Arquivo ${IDS_FILE} nao encontrado. Rode 001__cadastro-todas-apis.sh antes." >&2
  exit 1
fi

# shellcheck disable=SC1090
source "${IDS_FILE}"
TOOL_ID="${TOOL_ID:-${VIACEP_TOOL_ID:-}}"

if [[ -z "${TOOL_ID}" ]]; then
  echo "VIACEP_TOOL_ID nao encontrado em ${IDS_FILE}. Rode o cadastro novamente." >&2
  exit 1
fi

curl --silent --show-error --fail \
  -X POST "${API_BASE_URL}/tool/execute/${TOOL_ID}" \
  -H 'Content-Type: application/json' \
  -d @- <<JSON
{
  "call_id": "${CALL_ID}",
  "arguments": {
    "cep": "${CEP}",
    "include_geodata": true
  }
}
JSON
