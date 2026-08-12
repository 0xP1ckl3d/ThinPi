#!/bin/sh
set -eu
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
GENERIC_UPDATER="$SCRIPT_DIR/../client/update.sh"
[ -r "$GENERIC_UPDATER" ] || { echo "Use deploy/client/update.sh; the Pi wrapper requires the generic client deployment directory" >&2; exit 1; }
exec sh "$GENERIC_UPDATER" "$@"
