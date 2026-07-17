#!/usr/bin/env bash
# Lab SOCKS smoke: controller + local agent + start_socks is covered by
#   go test ./pkg/controller/ -run 'TestE2E' -count=1
# This script is for the MCP-attached controller already listening (e.g. Grok).
#
# Usage:
#   export STYX_SECRET=...   # must match running controller
#   export STYX_LISTEN=127.0.0.1:19137
#   ./scripts/lab-e2e-socks.sh [lab_url] [socks_addr]
#
# Then, with agent online, call MCP start_socks(node_id, socks_addr) or rely on
# an existing SOCKS, and this script curls the lab through it.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LAB_URL="${1:-http://10.7.11.116/}"
SOCKS_ADDR="${2:-127.0.0.1:11080}"
SECRET="${STYX_SECRET:-}"
if [[ -z "${SECRET}" && -f "${ROOT}/.grok/styx.secret" ]]; then
  SECRET="$(tr -d '[:space:]' < "${ROOT}/.grok/styx.secret")"
fi
LISTEN="${STYX_LISTEN:-127.0.0.1:19137}"
BIN="${STYX_BIN_DIR:-${ROOT}/release/$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')}"

echo "[*] lab=${LAB_URL} socks=${SOCKS_ADDR} controller=${LISTEN}"

echo "[*] baseline (direct)"
curl -sS -m 15 -o /dev/null -w "direct_http=%{http_code} time=%{time_total}\n" "${LAB_URL}"

if [[ -n "${SECRET}" && -x "${BIN}/agent" ]]; then
  if ! pgrep -f "${BIN}/agent" >/dev/null 2>&1; then
    echo "[*] starting local agent -> ${LISTEN}"
    "${BIN}/agent" -s "${SECRET}" -c "${LISTEN}" >/tmp/styx-lab-agent.log 2>&1 &
    sleep 1
  fi
else
  echo "[!] no STYX_SECRET or agent binary; ensure an agent is already online"
fi

echo "[*] via SOCKS ${SOCKS_ADDR} (start_socks must already be ready)"
curl -sS -m 20 -o /tmp/lab-socks-e2e.html -w "socks_http=%{http_code} time=%{time_total}\n" \
  --socks5-hostname "${SOCKS_ADDR}" "${LAB_URL}"
if command -v rg >/dev/null 2>&1; then
  rg -o '<title>[^<]+</title>' /tmp/lab-socks-e2e.html || true
else
  head -c 200 /tmp/lab-socks-e2e.html; echo
fi
echo "[*] ok (see go test ./pkg/controller/ -run TestE2E for automated path)"
