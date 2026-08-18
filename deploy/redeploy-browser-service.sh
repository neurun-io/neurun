#!/usr/bin/env bash
# Swaps in a new neurun-browser executable staged as neurun-browser.new, then
# restarts neurun so its supervisor spawns the new one on next use.
# neurun-browser has no server of its own to terminate here — it is neurun's
# child, started on first use and killed when neurun stops
# (browsergrpc.Supervisor.Close). Run as root.
set -euo pipefail

BIN_DIR=/var/apps/bin
STAGED="$BIN_DIR/neurun-browser.new"
LIVE="$BIN_DIR/neurun-browser"

APP_DIR=/var/apps/neurun
PIDFILE="$APP_DIR/neurun.pid"
LOG="$APP_DIR/neurun.log"

if [ ! -f "$STAGED" ]; then
  echo "staged executable not found: $STAGED" >&2
  exit 1
fi

mv -f "$STAGED" "$LIVE"
chown neurun:neurun "$LIVE"
chmod 755 "$LIVE"

if [ -f "$PIDFILE" ]; then
  OLD_PID=$(cat "$PIDFILE")
  if kill -0 "$OLD_PID" 2>/dev/null; then
    kill -TERM "$OLD_PID"
    for _ in $(seq 1 20); do
      kill -0 "$OLD_PID" 2>/dev/null || break
      sleep 0.5
    done
    kill -0 "$OLD_PID" 2>/dev/null && kill -KILL "$OLD_PID"
  fi
  rm -f "$PIDFILE"
fi

cd "$APP_DIR"
setsid runuser -u neurun -- "$APP_DIR/neurun" serve >>"$LOG" 2>&1 </dev/null &
NEW_PID=$!
disown
echo "$NEW_PID" >"$PIDFILE"

for _ in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:1267/healthz" >/dev/null 2>&1; then
    echo "neurun-browser swapped in, neurun is serving (pid $NEW_PID)"
    exit 0
  fi
  sleep 0.5
done

echo "neurun did not become healthy after restart — check $LOG" >&2
exit 1
