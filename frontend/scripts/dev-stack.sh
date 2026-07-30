#!/usr/bin/env bash
#
# Start every app needed to use the operator dashboard locally:
#
#   1. the Neurun control plane (cmd/neurun) on :8080
#   2. this dashboard's Next.js server on :3000
#
# The compose `dependencies` profile (postgres, nats, minio) is deliberately not
# started. The current all-in-one server uses process-local job, invocation,
# outbox and queue adapters, so those services are unused until the persistent
# adapters land.
#
# Ctrl-C stops both processes.
#
# Usage:
#   scripts/dev-stack.sh                 # dev servers, hot reload
#   scripts/dev-stack.sh --prod          # production build, then serve
#   scripts/dev-stack.sh --no-backend    # dashboard only; you run the server
#   scripts/dev-stack.sh --port 4000     # dashboard port

set -euo pipefail

FRONTEND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ROOT_DIR="$(cd "$FRONTEND_DIR/.." && pwd)"

MODE="dev"
START_BACKEND=1
WEB_PORT="${PORT:-3000}"

while [ $# -gt 0 ]; do
  case "$1" in
    --prod)       MODE="prod"; shift ;;
    --no-backend) START_BACKEND=0; shift ;;
    --port)       WEB_PORT="${2:?--port needs a value}"; shift 2 ;;
    # Print the header comment, stopping at the first line that is not one.
    -h|--help)    awk 'NR>1 && !/^#/{exit} NR>1{sub(/^# ?/,""); print}' "${BASH_SOURCE[0]}"; exit 0 ;;
    *)            echo "unknown option: $1" >&2; exit 2 ;;
  esac
done

log()  { printf '\033[2m[stack]\033[0m %s\n' "$*"; }
fail() { printf '\033[1m[stack] %s\033[0m\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------------------
# Toolchain
# ---------------------------------------------------------------------------

command -v node >/dev/null 2>&1 || fail "node is not on PATH. Next.js 16 needs Node 20.9 or newer."
NODE_MAJOR="$(node -p 'process.versions.node.split(".")[0]')"
[ "$NODE_MAJOR" -ge 20 ] || fail "Node $(node -v) is too old. Next.js 16 needs Node 20.9 or newer."

if [ "$START_BACKEND" -eq 1 ]; then
  command -v go >/dev/null 2>&1 ||
    fail "go is not on PATH. Install Go, or run with --no-backend and start the control plane yourself."
fi

# ---------------------------------------------------------------------------
# Configuration
#
# The control plane's own env comes from the repo's .env when present and
# .env.example otherwise. This script never writes outside frontend/, so it
# reads those rather than creating one.
# ---------------------------------------------------------------------------

if [ -f "$ROOT_DIR/.env" ]; then
  ENV_FILE="$ROOT_DIR/.env"
else
  ENV_FILE="$ROOT_DIR/.env.example"
  log "no .env found — using .env.example (copy it to .env to customise)"
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

NEURUN_HTTP_ADDR="${NEURUN_HTTP_ADDR:-:8080}"
API_PORT="${NEURUN_HTTP_ADDR##*:}"
API_BASE_URL="http://localhost:${API_PORT}"

# Async job routes return 503 durable_backend_unavailable unless volatile jobs
# are explicitly enabled. Leaving it off is a valid thing to test — the
# dashboard disables only the async control and keeps sync working — but for a
# default local run, on is more useful.
export NEURUN_ALLOW_VOLATILE_JOBS="${NEURUN_ALLOW_VOLATILE_JOBS:-true}"

# The dashboard proxies same-origin to the control plane; the browser never
# calls it directly, because the server ships no CORS middleware.
if [ ! -f "$FRONTEND_DIR/.env.local" ]; then
  log "writing frontend/.env.local → $API_BASE_URL"
  printf 'NEURUN_API_BASE_URL=%s\n' "$API_BASE_URL" > "$FRONTEND_DIR/.env.local"
fi

if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
  log "installing dashboard dependencies"
  (cd "$FRONTEND_DIR" && npm install)
fi

# ---------------------------------------------------------------------------
# Shutdown
# ---------------------------------------------------------------------------

BACKEND_PID=""
cleanup() {
  trap - EXIT INT TERM
  if [ -n "$BACKEND_PID" ] && kill -0 "$BACKEND_PID" 2>/dev/null; then
    log "stopping control plane"
    kill "$BACKEND_PID" 2>/dev/null || true
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

# ---------------------------------------------------------------------------
# Control plane
# ---------------------------------------------------------------------------

if [ "$START_BACKEND" -eq 1 ]; then
  # Build first, then run the binary: `go run` spawns a child, and killing the
  # wrapper can leave that child holding the port.
  log "building control plane"
  BIN="$ROOT_DIR/bin/neurun"
  (cd "$ROOT_DIR" && go build -trimpath -o "$BIN" ./cmd/neurun)
  [ -f "$BIN" ] || BIN="$BIN.exe"

  log "starting control plane on $API_BASE_URL"
  "$BIN" > "$FRONTEND_DIR/.neurun-server.log" 2>&1 &
  BACKEND_PID=$!

  # Wait for readiness rather than racing the dashboard against it.
  READY=0
  for _ in $(seq 1 50); do
    if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
      echo "--- control plane output ---" >&2
      cat "$FRONTEND_DIR/.neurun-server.log" >&2
      fail "control plane exited during startup"
    fi
    if node -e "fetch('$API_BASE_URL/healthz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))" 2>/dev/null; then
      READY=1
      break
    fi
    sleep 0.2
  done
  [ "$READY" -eq 1 ] || fail "control plane did not become healthy — see frontend/.neurun-server.log"
  log "control plane healthy"
fi

# ---------------------------------------------------------------------------
# Dashboard
# ---------------------------------------------------------------------------

cat <<BANNER

  Dashboard   http://localhost:${WEB_PORT}
  API         ${API_BASE_URL}
  API key     ${NEURUN_API_KEY:-<set NEURUN_API_KEY>}
  Async jobs  ${NEURUN_ALLOW_VOLATILE_JOBS} (process_local — queued jobs are lost on restart)

  Paste the base URL and API key into the connection screen.

BANNER

cd "$FRONTEND_DIR"
if [ "$MODE" = "prod" ]; then
  log "building dashboard"
  npm run build
  npm run start -- --port "$WEB_PORT"
else
  npm run dev -- --port "$WEB_PORT"
fi
