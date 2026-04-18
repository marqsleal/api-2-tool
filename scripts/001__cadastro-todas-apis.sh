#!/usr/bin/env bash
set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
IDS_FILE="${SCRIPT_DIR}/.tool_ids.env"
PAYLOADS_DIR="${SCRIPT_DIR}/payloads"

post_tool() {
  local payload_file="$1"
  curl --silent --show-error --fail \
    -X POST "${API_BASE_URL}/tool" \
    -H 'Content-Type: application/json' \
    --data-binary "@${payload_file}"
}

extract_id() {
  local response="$1"
  echo "${response}" \
    | tr -d '\n' \
    | grep -oE '"id"[[:space:]]*:[[:space:]]*"[^"]+"' \
    | head -n1 \
    | cut -d'"' -f4
}

print_response() {
  local response="$1"
  if command -v jq >/dev/null 2>&1; then
    echo "${response}" | jq
  else
    echo "${response}"
  fi
}

echo "Cadastrando ViaCEP..."
VIACEP_RESPONSE="$(post_tool "${PAYLOADS_DIR}/001__viacep.json")"
VIACEP_TOOL_ID="$(extract_id "${VIACEP_RESPONSE}")"
print_response "${VIACEP_RESPONSE}"
echo

echo "Cadastrando REST Countries..."
RESTCOUNTRIES_RESPONSE="$(post_tool "${PAYLOADS_DIR}/002__restcountries.json")"
RESTCOUNTRIES_TOOL_ID="$(extract_id "${RESTCOUNTRIES_RESPONSE}")"
print_response "${RESTCOUNTRIES_RESPONSE}"
echo

echo "Cadastrando Nager.Date..."
NAGERDATE_RESPONSE="$(post_tool "${PAYLOADS_DIR}/003__nagerdate.json")"
NAGERDATE_TOOL_ID="$(extract_id "${NAGERDATE_RESPONSE}")"
print_response "${NAGERDATE_RESPONSE}"
echo

if [[ -z "${VIACEP_TOOL_ID}" || -z "${RESTCOUNTRIES_TOOL_ID}" || -z "${NAGERDATE_TOOL_ID}" ]]; then
  echo "Falha ao extrair um ou mais IDs. Verifique as respostas acima." >&2
  exit 1
fi

cat > "${IDS_FILE}" <<ENVVARS
VIACEP_TOOL_ID=${VIACEP_TOOL_ID}
RESTCOUNTRIES_TOOL_ID=${RESTCOUNTRIES_TOOL_ID}
NAGERDATE_TOOL_ID=${NAGERDATE_TOOL_ID}
ENVVARS

echo "Cadastro concluido. IDs salvos em: ${IDS_FILE}"
