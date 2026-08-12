#!/bin/sh
set -eu
export THINPI_DEV_MODE=1 THINPI_API_URL=http://127.0.0.1:8080 THINPI_AGENT_SOCKET=/tmp/thinpi-agent-dev.sock
exec "$(dirname "$0")/../build/launcher/thinpi-launcher"
