#!/usr/bin/env bash
#
# Create or replace an operator account in .env, so the dashboard has a login.
#
# The password is hashed with `neurun hash-password` and only the hash is
# written; the plaintext never lands in .env, and passing it on stdin keeps it
# out of shell history too.
#
# Usage:
#   scripts/create-operator.sh                              # prompts; user=admin, role=admin
#   scripts/create-operator.sh alice operator               # prompts for the password
#   printf '%s' 'a-long-dev-password' | \
#     scripts/create-operator.sh admin admin                # non-interactive
#
# Roles: admin (all scopes), operator (read + submit/cancel), viewer (read only).
# Passwords must be at least 12 characters.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT_DIR/.env"
VAR="NEURUN_OPERATOR_ACCOUNTS"

USERNAME="${1:-admin}"
ROLE="${2:-admin}"

log()  { printf '\033[2m[operator]\033[0m %s\n' "$*"; }
fail() { printf '\033[1m[operator] %s\033[0m\n' "$*" >&2; exit 1; }

case "$ROLE" in
  admin|operator|viewer) ;;
  *) fail "unknown role '$ROLE' (expected admin, operator, or viewer)" ;;
esac

command -v go >/dev/null 2>&1 || fail "go is not on PATH; it is needed to hash the password"

# ---------------------------------------------------------------------------
# .env
# ---------------------------------------------------------------------------

if [ ! -f "$ENV_FILE" ]; then
  log "creating .env from .env.example"
  cp "$ROOT_DIR/.env.example" "$ENV_FILE"
fi

# ---------------------------------------------------------------------------
# Password
# ---------------------------------------------------------------------------

if [ -t 0 ]; then
  printf 'Password for %s (at least 12 characters, input is visible): ' "$USERNAME" >&2
  IFS= read -r PASSWORD
  printf '\n' >&2
else
  IFS= read -r PASSWORD || true
fi

[ -n "${PASSWORD:-}" ] || fail "no password supplied"

log "hashing password"
HASH="$(printf '%s' "$PASSWORD" | go run "$ROOT_DIR/cmd/neurun" hash-password 2>/dev/null)" ||
  fail "hashing failed — the password is probably shorter than 12 characters"
unset PASSWORD

[ -n "$HASH" ] || fail "hash-password produced no output"

# ---------------------------------------------------------------------------
# Merge into NEURUN_OPERATOR_ACCOUNTS
#
# The variable holds every account as `username:role:hash`, separated by `;`.
# Replacing one account therefore means rewriting the line, keeping the others.
# ---------------------------------------------------------------------------

EXISTING="$(sed -n "s/^${VAR}=//p" "$ENV_FILE" | tail -1 | sed "s/^['\"]//; s/['\"]$//")"

MERGED=""
if [ -n "$EXISTING" ]; then
  OLD_IFS="$IFS"
  IFS=';'
  for entry in $EXISTING; do
    entry="$(printf '%s' "$entry" | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
    [ -n "$entry" ] || continue
    entry_user="${entry%%:*}"
    # Drop any prior entry for this username so we replace rather than duplicate.
    if [ "$entry_user" = "$USERNAME" ]; then
      log "replacing the existing '$USERNAME' account"
      continue
    fi
    MERGED="${MERGED:+$MERGED;}$entry"
  done
  IFS="$OLD_IFS"
fi
MERGED="${MERGED:+$MERGED;}${USERNAME}:${ROLE}:${HASH}"

# Rewrite in place: drop every prior definition, then append one.
TMP_FILE="$(mktemp)"
grep -v "^${VAR}=" "$ENV_FILE" > "$TMP_FILE" || true
printf "%s='%s'\n" "$VAR" "$MERGED" >> "$TMP_FILE"
mv "$TMP_FILE" "$ENV_FILE"

log "wrote $VAR to .env"
cat <<SUMMARY

  Username  ${USERNAME}
  Role      ${ROLE}
  Stored    .env (hash only — the password is not saved)

  Start the stack with:
    docker compose --env-file .env up --build     # control plane only, :1267
    make dev                                     # control plane + dashboard, :3001

SUMMARY
