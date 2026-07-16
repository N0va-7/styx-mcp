#!/bin/bash
# Parameterized stdio MCP wrapper for styx-mcp controller.
# Usage: ./styx-mcp-wrapper.sh
# Environment:
#   STYX_SECRET   - shared secret for agent authentication (required; no weak default)
#   STYX_LISTEN   - controller listen address for agents (default: 127.0.0.1:19137)
#   STYX_LOG      - controller stderr log path (default: /tmp/styx-mcp-controller.log)
#   STYX_MCP_LOG  - optional path to log raw MCP stdio (off by default; may contain secrets)
#   STYX_BIN_DIR  - override the directory containing built binaries

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Resolve OS/arch for binary selection
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: ${ARCH}" >&2; exit 1 ;;
esac

BIN_DIR="${STYX_BIN_DIR:-${PROJECT_ROOT}/release/${OS}-${ARCH}}"
CONTROLLER="${BIN_DIR}/controller"

if [[ ! -x "${CONTROLLER}" ]]; then
    echo "Controller binary not found: ${CONTROLLER}" >&2
    echo "Run 'make build' or 'make build-all' in ${PROJECT_ROOT}" >&2
    exit 1
fi

# Prefer env; fall back to repo-local lab secret (gitignored .grok/styx.secret).
if [[ -z "${STYX_SECRET:-}" && -f "${PROJECT_ROOT}/.grok/styx.secret" ]]; then
    STYX_SECRET="$(tr -d '[:space:]' < "${PROJECT_ROOT}/.grok/styx.secret")"
fi

if [[ -z "${STYX_SECRET:-}" ]]; then
    echo "STYX_SECRET is required (do not use a weak default)." >&2
    echo "  export STYX_SECRET=\"\$(openssl rand -hex 16)\"" >&2
    echo "  # or write it to ${PROJECT_ROOT}/.grok/styx.secret (gitignored)" >&2
    exit 1
fi

SECRET="${STYX_SECRET}"
# Default to loopback; set STYX_LISTEN=0.0.0.0:19137 only when agents are remote.
LISTEN="${STYX_LISTEN:-127.0.0.1:19137}"
LOG="${STYX_LOG:-/tmp/styx-mcp-controller.log}"

# Early port-busy hint (controller still fails hard if bind races after this check).
if command -v lsof >/dev/null 2>&1; then
    PORT="${LISTEN##*:}"
    if [[ -n "${PORT}" ]] && lsof -nP -iTCP:"${PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
        echo "styx-mcp: listen address ${LISTEN} appears busy (port ${PORT} already in use)." >&2
        echo "  Free the port or set STYX_LISTEN to another host:port." >&2
        lsof -nP -iTCP:"${PORT}" -sTCP:LISTEN 2>/dev/null | head -5 >&2 || true
        exit 1
    fi
fi

exec "${CONTROLLER}" -s "${SECRET}" -l "${LISTEN}" 2>>"${LOG}"
