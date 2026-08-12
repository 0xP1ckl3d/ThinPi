#!/bin/sh
set -eu
cd "$(dirname "$0")/../controller"
exec go run ./cmd/thinpi-controller serve --dev --listen 127.0.0.1:8080 --database ../thinpi-dev.db
