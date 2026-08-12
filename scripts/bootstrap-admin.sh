#!/bin/sh
set -eu
COMPOSE_FILE=${COMPOSE_FILE:-deploy/controller/compose.yml}
COMPOSE_ENV_FILE=${COMPOSE_ENV_FILE:-}
PASSWORD=""
CONFIRM_PASSWORD=""
trap 'stty echo 2>/dev/null || true; unset PASSWORD CONFIRM_PASSWORD' EXIT HUP INT TERM

printf 'Initial administrator password: ' >&2
stty -echo
IFS= read -r PASSWORD
printf '\nConfirm administrator password: ' >&2
IFS= read -r CONFIRM_PASSWORD
stty echo
printf '\n' >&2

if [ "$PASSWORD" != "$CONFIRM_PASSWORD" ]; then
  echo "Passwords do not match. No administrator was created." >&2
  exit 1
fi
unset CONFIRM_PASSWORD

if [ -n "$COMPOSE_ENV_FILE" ]; then
  printf '%s\n' "$PASSWORD" | docker compose --env-file "$COMPOSE_ENV_FILE" -f "$COMPOSE_FILE" run --rm --no-deps controller bootstrap-admin --password-stdin
else
  printf '%s\n' "$PASSWORD" | docker compose -f "$COMPOSE_FILE" run --rm --no-deps controller bootstrap-admin --password-stdin
fi
