#!/usr/bin/env bash
# One-time setup for a bare Ubuntu VPS to host neurun: app user, /var/apps
# layout, Redis, nginx, firewall. Idempotent — safe to re-run after upgrades.
#
# Run as root: ./bootstrap.sh
set -euo pipefail

APP_DIR=/var/apps/neurun
CODENAME=$(. /etc/os-release && echo "$VERSION_CODENAME")

# --- app user & directories ---
# The server itself runs as this unprivileged user; root (this script, and the
# CI deploy) only ever prepares files for it to run.
id -u neurun >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin neurun
mkdir -p "$APP_DIR/data"
touch "$APP_DIR/neurun.log"
chown -R neurun:neurun "$APP_DIR"
chmod 750 "$APP_DIR"

# --- Redis: official repo for the latest release, sessions cache only ---
if ! command -v redis-server >/dev/null; then
  curl -fsSL https://packages.redis.io/gpg | gpg --dearmor -o /usr/share/keyrings/redis-archive-keyring.gpg
  echo "deb [signed-by=/usr/share/keyrings/redis-archive-keyring.gpg] https://packages.redis.io/deb $CODENAME main" \
    >/etc/apt/sources.list.d/redis.list
  cat >/etc/apt/preferences.d/redis <<'EOF'
Package: *
Pin: origin packages.redis.io
Pin-Priority: 900
EOF
  apt-get update -qq
  apt-get install -y redis
fi
# Sessions only: losing it just signs everybody out, so it is not persisted —
# matches compose.yaml's local redis service.
sed -i 's/^save .*/save ""/' /etc/redis/redis.conf
sed -i 's/^appendonly .*/appendonly no/' /etc/redis/redis.conf
systemctl enable --now redis-server

# --- nginx: official repo for the latest release ---
if ! command -v nginx >/dev/null; then
  curl -fsSL https://nginx.org/keys/nginx_signing.key | gpg --dearmor -o /usr/share/keyrings/nginx-archive-keyring.gpg
  echo "deb [signed-by=/usr/share/keyrings/nginx-archive-keyring.gpg] http://nginx.org/packages/ubuntu $CODENAME nginx" \
    >/etc/apt/sources.list.d/nginx.list
  cat >/etc/apt/preferences.d/nginx <<'EOF'
Package: *
Pin: origin nginx.org
Pin-Priority: 900
EOF
  apt-get update -qq
  apt-get install -y nginx
fi

cat >/etc/nginx/conf.d/neurun.conf <<'EOF'
server {
    listen 80 default_server;
    server_name _;

    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:1267;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # A deployment build can run for minutes; the connection that is
        # following its logs has to survive at least that long.
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }
}
EOF
rm -f /etc/nginx/conf.d/default.conf
nginx -t
systemctl enable --now nginx
systemctl reload nginx

# --- firewall: allow SSH before enabling, or this locks itself out ---
ufw allow OpenSSH
ufw allow 80/tcp
ufw --force enable

# --- starter .env: written once, never overwritten by a later bootstrap run ---
if [ ! -f "$APP_DIR/.env" ]; then
  cat >"$APP_DIR/.env" <<EOF
NEURUN_HTTP_ADDR=127.0.0.1:1267
NEURUN_PUBLIC_URL=http://$(curl -fsS -4 ifconfig.me || hostname -I | awk '{print $1}')
NEURUN_DATA_DIRECTORY=$APP_DIR/data
NEURUN_REDIS_URL=redis://localhost:6379/0
NEURUN_PYTHON_EXECUTABLE=python3
NEURUN_SESSION_COOKIE_SECURE=false
NEURUN_DEFAULT_PROJECT_ID=prj_default

# Required — neurun will not start until this is set.
# NEURUN_DATABASE_URL=postgres://user:password@host:5432/neurun?sslmode=disable
EOF
  chown neurun:neurun "$APP_DIR/.env"
  chmod 600 "$APP_DIR/.env"
fi

echo "bootstrap complete"
