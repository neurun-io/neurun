#!/usr/bin/env bash
# Swaps in the binary staged next to this script as neurun.new and restarts the
# unit. Run as root on the target host; the server itself runs as neurun.
set -euo pipefail

APP_DIR=/var/apps/neurun
BIN="$APP_DIR/neurun"
STAGED="$APP_DIR/neurun.new"

if [ ! -x "$STAGED" ]; then
  echo "staged binary not found or not executable: $STAGED" >&2
  exit 1
fi

mv -f "$STAGED" "$BIN"
chown neurun:neurun "$BIN"
chmod 755 "$BIN"

systemctl restart neurun

# The unit is Type=simple, so systemd calls it started the moment it forks —
# well before migrations finish, which against a remote database can take the
# better part of a minute. Health is the only thing worth reporting.
for _ in $(seq 1 120); do
  if curl -fsS "http://127.0.0.1:1267/healthz" >/dev/null 2>&1; then
    echo "neurun is serving"
    exit 0
  fi
  sleep 1
done

echo "neurun did not become healthy after restart — journalctl -u neurun" >&2
exit 1
