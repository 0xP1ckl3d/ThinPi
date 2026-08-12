#!/bin/sh
set -eu
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=${VERSION:-$(cat "$ROOT/VERSION" 2>/dev/null || printf '0.1.0')}
mkdir -p "$ROOT/bin/arm64"
(cd "$ROOT/controller" && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$ROOT/bin/arm64/thinpi-controller" ./cmd/thinpi-controller)
VERSION=$VERSION sh "$ROOT/scripts/build-client.sh" arm64
