#!/bin/sh
set -eu

command -v git >/dev/null 2>&1 || { echo "git is required" >&2; exit 1; }
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || { echo "Run inside a Git checkout" >&2; exit 1; }

BAD=$(git ls-files | awk '
  /(^|\/)(bin|build|out|dist|\.thinpi-dev|\.docker-config|\.gocache|\.gomodcache)(\/|$)/ ||
  /(^|\/)\.env$/ ||
  /(^|\/)(agent|device)\.json$/ ||
  /(^|\/)ui\.env$/ ||
  /\.(db|db-shm|db-wal|sqlite|sqlite3|log|key|pem|p12|pfx|csr|srl|crt|deb|rpm|exe|dll)$/ { print }
')
if [ -n "$BAD" ]; then
  echo "Refusing release: generated, secret, or installation-specific files are tracked:" >&2
  printf '%s\n' "$BAD" >&2
  exit 1
fi

AUDIT_FILE=$(mktemp)
trap 'rm -f "$AUDIT_FILE"' EXIT INT TERM
if git grep -n -I -E -- '-----BEGIN ([A-Z ]+ )?PRIVATE KEY-----' >"$AUDIT_FILE" 2>/dev/null; then
  echo "Refusing release: a private key block is tracked:" >&2
  cat "$AUDIT_FILE" >&2
  exit 1
fi
echo "Repository audit passed: no tracked runtime state, build output, database, private key, certificate, or installation config."
