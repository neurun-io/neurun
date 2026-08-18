#!/usr/bin/env bash
# One-time setup for a bare Ubuntu VPS to host neurun: app user, /var/apps
# layout, Redis, nginx, firewall. Idempotent — safe to re-run after upgrades.
#
# Run as root: ./bootstrap.sh
set -euo pipefail

APP_DIR=/var/apps/neurun
DASHBOARD_DIR=/var/apps/dashboard
BIN_DIR=/var/apps/bin
CODENAME=$(. /etc/os-release && echo "$VERSION_CODENAME")

# --- app user & directories ---
# Both processes run as this unprivileged user; root (this script, and the CI
# deploy) only ever prepares files for it to run.
id -u neurun >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin neurun
mkdir -p "$APP_DIR/data" "$DASHBOARD_DIR" "$BIN_DIR"
touch "$APP_DIR/neurun.log" "$DASHBOARD_DIR/dashboard.log"
chown -R neurun:neurun "$APP_DIR" "$DASHBOARD_DIR" "$BIN_DIR"
chmod 750 "$APP_DIR" "$DASHBOARD_DIR" "$BIN_DIR"

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

# The API is not what a browser expects at the bare host, so it sits on its
# own port and the dashboard takes the default one — see dashboard.conf.
cat >/etc/nginx/conf.d/neurun.conf <<'EOF'
server {
    listen 8080;
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

# A separate origin on its own port, same split as local dev (localhost:1267
# API / localhost:3001 dashboard) — the session cookie is SameSite=Strict,
# which still rides between ports on the same host since site matching
# ignores port, so this needs CORS but not a shared origin.
cat >/etc/nginx/conf.d/dashboard.conf <<'EOF'
server {
    listen 80 default_server;
    server_name _;

    location / {
        proxy_pass http://127.0.0.1:3001;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF
rm -f /etc/nginx/conf.d/default.conf
nginx -t
systemctl enable --now nginx
systemctl reload nginx

# --- Node.js runtime for the dashboard's standalone server. CI builds it;
# this box only ever runs `node server.js`. ---
if ! command -v node >/dev/null; then
  curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
  apt-get install -y nodejs
fi

# --- Chrome + Xvfb for apps that drive a browser. Chrome for Testing is
# pinned by directory: fetched once at whatever version is current, then never
# touched again by this script or by apt, since it is a plain extracted
# tarball outside any package manager's reach. ---
apt-get install -y -qq \
  libatk1.0-0 libatk-bridge2.0-0 libcups2 libasound2t64 libgbm1 \
  libcairo2 libpango-1.0-0 libpangocairo-1.0-0 \
  libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libatspi2.0-0 \
  libxkbcommon0 libx11-xcb1 libxext6 libnss3 fonts-liberation \
  unzip jq xvfb

if [ ! -e "$BIN_DIR/chrome" ]; then
  CFT_JSON="https://googlechromelabs.github.io/chrome-for-testing/last-known-good-versions-with-downloads.json"
  CFT_VERSION=$(curl -fsSL "$CFT_JSON" | jq -r '.channels.Stable.version')
  CFT_URL=$(curl -fsSL "$CFT_JSON" | jq -r '.channels.Stable.downloads.chrome[] | select(.platform=="linux64") | .url')
  WORK=$(mktemp -d)
  curl -fsSL "$CFT_URL" -o "$WORK/chrome.zip"
  unzip -q "$WORK/chrome.zip" -d "$WORK/extracted"
  rm -rf "$BIN_DIR/chrome-$CFT_VERSION"
  mv "$WORK/extracted/chrome-linux64" "$BIN_DIR/chrome-$CFT_VERSION"
  rm -rf "$WORK"
  ln -sfn "$BIN_DIR/chrome-$CFT_VERSION/chrome" "$BIN_DIR/chrome"
  chown -R neurun:neurun "$BIN_DIR"
fi

cat >/etc/systemd/system/xvfb.service <<'EOF'
[Unit]
Description=Virtual framebuffer for headed browser automation
After=network.target

[Service]
ExecStart=/usr/bin/Xvfb :99 -screen 0 1920x1080x24 -ac -nolisten tcp
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable --now xvfb

# --- firewall: allow SSH before enabling, or this locks itself out ---
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 8080/tcp
ufw --force enable

# --- starter .env: written once, never overwritten by a later bootstrap run ---
if [ ! -f "$APP_DIR/.env" ]; then
  HOST_IP=$(curl -fsS -4 ifconfig.me || hostname -I | awk '{print $1}')
  cat >"$APP_DIR/.env" <<EOF
NEURUN_HTTP_ADDR=127.0.0.1:1267
NEURUN_PUBLIC_URL=http://$HOST_IP:8080
NEURUN_DATA_DIRECTORY=$APP_DIR/data
NEURUN_REDIS_URL=redis://localhost:6379/0
NEURUN_PYTHON_EXECUTABLE=python3
NEURUN_SESSION_COOKIE_SECURE=false
NEURUN_DEFAULT_PROJECT_ID=prj_default
# The dashboard's own origin — it calls this API cross-origin, with credentials.
NEURUN_ALLOWED_ORIGINS=http://$HOST_IP

# A pinned, non-auto-updating Chrome and a persistent virtual display for
# apps that drive a browser. NEURUN_BROWSER_SERVICE has nothing at that path
# until deploy/redeploy-browser-service.sh has shipped one.
DISPLAY=:99
NEURUN_CHROME_PATH=$BIN_DIR/chrome
NEURUN_BROWSER_SERVICE=$BIN_DIR/neurun-browser

# Required — neurun will not start until this is set.
# NEURUN_DATABASE_URL=postgres://user:password@host:5432/neurun?sslmode=disable
EOF
  chown neurun:neurun "$APP_DIR/.env"
  chmod 600 "$APP_DIR/.env"
fi

echo "bootstrap complete"
