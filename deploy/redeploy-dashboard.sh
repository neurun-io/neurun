#!/usr/bin/env bash
# Terminates the running dashboard and re-serves the standalone Next.js build
# staged next to this script as dashboard.new.tar.gz. Run as root; the process
# itself runs as the unprivileged neurun user.
set -euo pipefail

APP_DIR=/var/apps/dashboard
STAGED_TAR="$APP_DIR/dashboard.new.tar.gz"
STAGING="$APP_DIR/staging"
LIVE="$APP_DIR/current"
PIDFILE="$APP_DIR/dashboard.pid"
LOG="$APP_DIR/dashboard.log"

if [ ! -f "$STAGED_TAR" ]; then
  echo "staged build not found: $STAGED_TAR" >&2
  exit 1
fi

rm -rf "$STAGING"
mkdir -p "$STAGING"
tar xzf "$STAGED_TAR" -C "$STAGING"
rm -f "$STAGED_TAR"

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

rm -rf "$LIVE"
mv "$STAGING" "$LIVE"
chown -R neurun:neurun "$LIVE"
touch "$LOG"
chown neurun:neurun "$LOG"

cd "$LIVE"
setsid runuser -u neurun -- env PORT=3001 HOSTNAME=127.0.0.1 NODE_ENV=production \
  node server.js >>"$LOG" 2>&1 </dev/null &
NEW_PID=$!
disown
echo "$NEW_PID" >"$PIDFILE"

for _ in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:3001/" >/dev/null 2>&1; then
    echo "dashboard is serving (pid $NEW_PID)"
    exit 0
  fi
  sleep 0.5
done

echo "dashboard did not become healthy after restart — check $LOG" >&2
exit 1
