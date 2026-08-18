#!/usr/bin/env bash
# Swaps in the standalone Next.js build staged next to this script as
# dashboard.new.tar.gz and restarts the unit. Run as root; the process itself
# runs as the unprivileged neurun user.
set -euo pipefail

APP_DIR=/var/apps/dashboard
STAGED_TAR="$APP_DIR/dashboard.new.tar.gz"
STAGING="$APP_DIR/staging"
LIVE="$APP_DIR/current"

if [ ! -f "$STAGED_TAR" ]; then
  echo "staged build not found: $STAGED_TAR" >&2
  exit 1
fi

rm -rf "$STAGING"
mkdir -p "$STAGING"
tar xzf "$STAGED_TAR" -C "$STAGING"
rm -f "$STAGED_TAR"

# Stopped before the swap rather than restarted after: the running node holds
# files under the directory that is about to be replaced.
systemctl stop neurun-dashboard || true

rm -rf "$LIVE"
mv "$STAGING" "$LIVE"
chown -R neurun:neurun "$LIVE"

systemctl start neurun-dashboard

for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:3001/" >/dev/null 2>&1; then
    echo "dashboard is serving"
    exit 0
  fi
  sleep 1
done

echo "dashboard did not become healthy — journalctl -u neurun-dashboard" >&2
exit 1
