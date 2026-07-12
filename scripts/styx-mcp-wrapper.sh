#!/bin/bash
# Parameterized stdio MCP wrapper for styx-mcp controller.
# Usage: ./styx-mcp-wrapper.sh
# Environment:
#   STYX_SECRET   - shared secret for agent authentication (default: secret)
#   STYX_LISTEN   - controller listen address for agents (default: 0.0.0.0:19137)
#   STYX_LOG      - stderr log path (default: /tmp/styx-mcp-controller.log)
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

SECRET="${STYX_SECRET:-secret}"
LISTEN="${STYX_LISTEN:-0.0.0.0:19137}"
LOG="${STYX_LOG:-/tmp/styx-mcp-controller.log}"

exec "${CONTROLLER}" -s "${SECRET}" -l "${LISTEN}" 2>>"${LOG}"
