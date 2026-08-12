#!/bin/sh
set -eu

usage() {
  echo "Usage: deploy-client.sh [--platform auto|generic|raspberry-pi] user@host" >&2
}

PLATFORM=auto
if [ "${1:-}" = --platform ]; then PLATFORM=${2:-}; shift 2; fi
case "$PLATFORM" in auto|generic|raspberry-pi) ;; *) usage; exit 2;; esac
TARGET=${1:-}
if [ -z "$TARGET" ] || [ "$#" -ne 1 ]; then usage; exit 2; fi
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
command -v ssh >/dev/null 2>&1 || { echo "ssh is required on the workstation" >&2; exit 1; }
command -v rsync >/dev/null 2>&1 || { echo "rsync is required on the workstation" >&2; exit 1; }

echo "Preflighting $TARGET as platform $PLATFORM..."
ssh "$TARGET" 'mkdir -p /tmp/thinpi'
rsync -av --delete "$ROOT/deploy/client/" "$TARGET:/tmp/thinpi/deploy-client/"
# PLATFORM is restricted to the fixed enum above.
# shellcheck disable=SC2029
ssh "$TARGET" "sh /tmp/thinpi/deploy-client/preflight.sh '$PLATFORM'"
rsync -av --delete "$ROOT/agent/" "$TARGET:/tmp/thinpi/agent/"
rsync -av --delete "$ROOT/launcher/" "$TARGET:/tmp/thinpi/launcher/"
# PLATFORM is restricted to the fixed enum above.
# shellcheck disable=SC2029
ssh -t "$TARGET" "sh /tmp/thinpi/deploy-client/stage.sh '$PLATFORM'"
