#!/usr/bin/env bash
# One-time setup for a bare Debian or Ubuntu host to run neurun: app user, /var/apps
# layout, Redis, nginx, the two units and the firewall. Idempotent — safe to
# re-run after upgrades.
#
# Run as root: ./bootstrap.sh
set -euo pipefail

APP_DIR=/var/apps/neurun
DASHBOARD_DIR=/var/apps/dashboard
CODENAME=$(. /etc/os-release && echo "$VERSION_CODENAME")
# Only the vendor repo path differs between them; every package here is
# named the same on both.
DISTRO=$(. /etc/os-release && echo "$ID")


# --- prerequisites ---
# A minimal cloud image ships none of these, and the vendor repos below are
# fetched and verified with them.
apt-get update -qq
apt-get install -y -qq ca-certificates curl gnupg ufw

# --- root login by key ---
# The CI deploy connects as root to swap binaries and bounce units. Cloud images
# ship PermitRootLogin no; this restores key-only root, never password — which is
# what the deploy key already relies on.
install -d -m 755 /etc/ssh/sshd_config.d
cat >/etc/ssh/sshd_config.d/10-neurun-deploy.conf <<'EOF'
PermitRootLogin prohibit-password
EOF
systemctl reload ssh 2>/dev/null || systemctl reload sshd 2>/dev/null || true

# --- app user & directories ---
# Both processes run as this unprivileged user; root (this script, and the CI
# deploy) only ever prepares files for it to run.
id -u neurun >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin neurun
# Chrome writes under $HOME regardless of --user-data-dir, and dies without it —
# a browser session got as far as launching and then never opened its debug
# port. The account is created with no home, so the directory is made here.
install -d -o neurun -g neurun -m 700 "$(getent passwd neurun | cut -d: -f6)"
mkdir -p "$APP_DIR/data" "$DASHBOARD_DIR"
touch "$APP_DIR/neurun.log" "$DASHBOARD_DIR/dashboard.log"
chown -R neurun:neurun "$APP_DIR" "$DASHBOARD_DIR"
chmod 750 "$APP_DIR" "$DASHBOARD_DIR"

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
  echo "deb [signed-by=/usr/share/keyrings/nginx-archive-keyring.gpg] http://nginx.org/packages/$DISTRO $CODENAME nginx" \
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
# "upgrade" only when the client asked for one, so ordinary requests are still
# answered on a keep-alive connection.
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 8080;
    server_name _;

    client_max_body_size 64m;

    location / {
        proxy_pass http://127.0.0.1:1267;
        proxy_http_version 1.1;
        # $http_host, not $host: $host drops the port, which would leave the API
        # looking like the dashboard's own origin. CORS then reads the preflight
        # as same-origin, declines to answer it, and OPTIONS 404s.
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # A display is streamed over a websocket, which does not survive a proxy
        # that drops the upgrade — the browser reports it as the display having
        # stopped responding.
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        # And it stays open as long as somebody is watching, so it must not be
        # read-timed out the way a request is.
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
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
        # $http_host, not $host: $host drops the port, which would leave the API
        # looking like the dashboard's own origin. CORS then reads the preflight
        # as same-origin, declines to answer it, and OPTIONS 404s.
        proxy_set_header Host $http_host;
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
# this box only ever runs `node server.js`. It doubles as the toolchain the
# deployer builds Node apps with, since that needs npm and nothing more. ---
if ! command -v node >/dev/null; then
  curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
  apt-get install -y nodejs
fi

# --- toolchains the deployer builds tenant apps with ---
# A runtime is available when its toolchain is on this host. Nothing checks at
# boot: a missing one fails the deployment that asked for it, which is why an
# absent cargo surfaced as build_failed on a Rust app rather than at startup.
#
# cargo shells out to a linker and links against system OpenSSL, so the
# compiler alone is not enough. Python is the system one and Node is above; Go
# and Ruby are deliberately absent, and deployments naming them will fail until
# they are added here.
# protoc is here because a crate's build.rs may compile .proto files — prost
# shells out to it, and cargo reports its absence as a failed build script
# rather than as a missing tool.
apt-get install -y -qq build-essential pkg-config libssl-dev protobuf-compiler

# Rust comes from rustup, not apt: the distro's is whatever that release froze,
# and Debian 13 ships 1.85 where tonic already requires 1.88. Installed into
# /usr/local so every user has it, and the toolchain's own binaries are linked
# rather than rustup's shims — a shim resolves its toolchain through RUSTUP_HOME,
# which the build does not carry.
if [ ! -x /usr/local/rustup/toolchains/stable-x86_64-unknown-linux-gnu/bin/cargo ]; then
  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs |
    RUSTUP_HOME=/usr/local/rustup CARGO_HOME=/usr/local/cargo     sh -s -- -y --no-modify-path --profile minimal --default-toolchain stable
fi
for tool in cargo rustc rustdoc; do
  ln -sfn "/usr/local/rustup/toolchains/stable-x86_64-unknown-linux-gnu/bin/$tool" "/usr/local/bin/$tool"
done

# --- units ---
# systemd owns both processes: they come back after a reboot and after a crash,
# which a backgrounded process does not. Neither is Type=notify — nothing here
# signals readiness — so a restart returns long before the port answers, and the
# deploy scripts poll for health themselves.
cat >/etc/systemd/system/neurun.service <<'EOF'
[Unit]
Description=Neurun control plane
Wants=network-online.target
After=network-online.target redis-server.service

[Service]
User=neurun
Group=neurun
# The server reads .env out of its working directory, so this is what
# configures it. EnvironmentFile would parse those values by different rules.
WorkingDirectory=/var/apps/neurun
ExecStart=/var/apps/neurun/neurun serve
Restart=always
RestartSec=2
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF

cat >/etc/systemd/system/neurun-dashboard.service <<'EOF'
[Unit]
Description=Neurun dashboard
Wants=network-online.target
After=network-online.target

[Service]
User=neurun
Group=neurun
WorkingDirectory=/var/apps/dashboard/current
Environment=NODE_ENV=production PORT=3001 HOSTNAME=127.0.0.1
ExecStart=/usr/bin/node server.js
Restart=always
RestartSec=2
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF

# Whatever an earlier deploy backgrounded is not ours to leave running: it holds
# the port the unit is about to want.
pkill -f '/var/apps/neurun/neurun serve' 2>/dev/null || true
pkill -f 'node server.js' 2>/dev/null || true
rm -f "$APP_DIR/neurun.pid" "$DASHBOARD_DIR/dashboard.pid"

systemctl daemon-reload
systemctl enable neurun neurun-dashboard
# Started only once there is something to start: a fresh box has neither until
# the first deploy lands.
if [ -x "$APP_DIR/neurun" ]; then
  systemctl restart neurun
fi
if [ -f "$DASHBOARD_DIR/current/server.js" ]; then
  systemctl restart neurun-dashboard
fi

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
# A cold cargo or npm build is minutes, and the default 5m cut them off.
NEURUN_DEPLOYMENT_BUILD_TIMEOUT=15m
# The dashboard's own origin — it calls this API cross-origin, with credentials.
NEURUN_ALLOWED_ORIGINS=http://$HOST_IP

# Required — neurun will not start until this is set.
# NEURUN_DATABASE_URL=postgres://user:password@host:5432/neurun?sslmode=disable
EOF
  chown neurun:neurun "$APP_DIR/.env"
  chmod 600 "$APP_DIR/.env"
fi

echo "bootstrap complete"
