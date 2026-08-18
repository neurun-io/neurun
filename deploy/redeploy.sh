#!/usr/bin/env bash
# Terminates the running neurun and re-serves the binary staged next to this
# script as neurun.new. Run as root on the target host; the server process
# itself runs as the unprivileged neurun user.
set -euo pipefail

APP_DIR=/var/apps/neurun
BIN="$APP_DIR/neurun"
STAGED="$APP_DIR/neurun.new"
PIDFILE="$APP_DIR/neurun.pid"
LOG="$APP_DIR/neurun.log"

if [ ! -x "$STAGED" ]; then
  echo "staged binary not found or not executable: $STAGED" >&2
  exit 1
fi

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

mv -f "$STAGED" "$BIN"
chown neurun:neurun "$BIN"
chmod 755 "$BIN"
touch "$LOG"
chown neurun:neurun "$LOG"

cd "$APP_DIR"
setsid runuser -u neurun -- "$BIN" serve >>"$LOG" 2>&1 </dev/null &
NEW_PID=$!
disown
echo "$NEW_PID" >"$PIDFILE"

for _ in $(seq 1 120); do
  if curl -fsS "http://127.0.0.1:1267/healthz" >/dev/null 2>&1; then
    echo "neurun is serving (pid $NEW_PID)"
    exit 0
  fi
  # Migrations run on every start; against a remote database they can take
  # the better part of a minute, so this waits rather than declaring an early
  # failure while they are still running.
  sleep 1
done

echo "neurun did not become healthy after restart — check $LOG" >&2
exit 1
