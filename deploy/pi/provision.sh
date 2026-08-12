#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
GENERIC_PROVISIONER="$SCRIPT_DIR/../client/provision.sh"
[ -r "$GENERIC_PROVISIONER" ] || { echo "Use deploy/client/provision.sh; the Pi wrapper requires the generic client deployment directory" >&2; exit 1; }
exec sh "$GENERIC_PROVISIONER" --platform raspberry-pi "$@"
