#!/bin/sh
set -eu
TARGET=${1:?Usage: pi-logs.sh user@host}
exec ssh "$TARGET" 'sudo journalctl -f -u thinpi-agent -u thinpi-ui'
