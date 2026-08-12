#!/bin/sh
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
exec sh "$ROOT/scripts/deploy-client.sh" --platform raspberry-pi "$@"
