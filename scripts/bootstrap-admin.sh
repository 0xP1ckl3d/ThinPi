#!/bin/sh
set -eu
COMPOSE_FILE=${COMPOSE_FILE:-deploy/controller/compose.yml}
COMPOSE_ENV_FILE=${COMPOSE_ENV_FILE:-}
printf 'Initial administrator password: ' >&2
stty -echo
IFS= read -r PASSWORD
stty echo
printf '\n' >&2
trap 'stty echo 2>/dev/null || true; unset PASSWORD' EXIT INT TERM
if [ -n "$COMPOSE_ENV_FILE" ]; then
  printf '%s\n' "$PASSWORD" | docker compose --env-file "$COMPOSE_ENV_FILE" -f "$COMPOSE_FILE" run --rm --no-deps controller bootstrap-admin --password-stdin
else
  printf '%s\n' "$PASSWORD" | docker compose -f "$COMPOSE_FILE" run --rm --no-deps controller bootstrap-admin --password-stdin
fi
