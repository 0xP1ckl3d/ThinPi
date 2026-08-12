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
command -v mktemp >/dev/null 2>&1 || { echo "mktemp is required on the workstation" >&2; exit 1; }

# Password-based SSH would otherwise prompt once for every ssh/rsync process.
# Keep one short-lived master connection for the complete deployment instead.
CONTROL_DIR=$(mktemp -d)
CONTROL_PATH=$CONTROL_DIR/control
cleanup() {
  ssh -o ControlPath="$CONTROL_PATH" -O exit "$TARGET" >/dev/null 2>&1 || true
  rmdir "$CONTROL_DIR" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM
SSH_REUSE="ssh -o ControlMaster=auto -o ControlPersist=300 -o ControlPath=$CONTROL_PATH"

echo "Preflighting $TARGET as platform $PLATFORM..."
echo "Opening one reusable SSH connection (password authentication prompts once)..."
ssh -o ControlMaster=yes -o ControlPersist=300 -o ControlPath="$CONTROL_PATH" \
  "$TARGET" 'mkdir -p /tmp/thinpi'
rsync -e "$SSH_REUSE" -av --delete "$ROOT/deploy/client/" "$TARGET:/tmp/thinpi/deploy-client/"
# PLATFORM is restricted to the fixed enum above.
# shellcheck disable=SC2029
ssh -o ControlMaster=auto -o ControlPersist=300 -o ControlPath="$CONTROL_PATH" \
  "$TARGET" "sh /tmp/thinpi/deploy-client/preflight.sh '$PLATFORM'"
rsync -e "$SSH_REUSE" -av --delete "$ROOT/agent/" "$TARGET:/tmp/thinpi/agent/"
rsync -e "$SSH_REUSE" -av --delete "$ROOT/launcher/" "$TARGET:/tmp/thinpi/launcher/"
# PLATFORM is restricted to the fixed enum above.
# shellcheck disable=SC2029
ssh -o ControlMaster=auto -o ControlPersist=300 -o ControlPath="$CONTROL_PATH" \
  -t "$TARGET" "sh /tmp/thinpi/deploy-client/stage.sh '$PLATFORM'"
