#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
VERSION=${VERSION:-$(cat "$ROOT/VERSION" 2>/dev/null || printf '0.1.0')}
REQUESTED_ARCH=${1:-}
if [ -z "$REQUESTED_ARCH" ]; then
  if command -v dpkg >/dev/null 2>&1; then REQUESTED_ARCH=$(dpkg --print-architecture); else REQUESTED_ARCH=$(uname -m); fi
fi
case "$REQUESTED_ARCH" in
  amd64|x86_64) ARCH=amd64; GOARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64; GOARCH=arm64 ;;
  *) echo "Usage: build-client.sh [amd64|arm64]" >&2; exit 2 ;;
esac

HOST_ARCH=$(uname -m)
case "$HOST_ARCH" in x86_64) HOST_ARCH=amd64;; aarch64) HOST_ARCH=arm64;; esac
mkdir -p "$ROOT/bin/$ARCH"
(cd "$ROOT/agent" && CGO_ENABLED=0 GOOS=linux GOARCH=$GOARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o "$ROOT/bin/$ARCH/thinpi-agent" ./cmd/thinpi-agent)

BUILD_DIR="$ROOT/build/launcher-$ARCH"
if [ "$HOST_ARCH" = "$ARCH" ]; then
  cmake -S "$ROOT/launcher" -B "$BUILD_DIR" -G Ninja -DCMAKE_BUILD_TYPE=Release
  cmake --build "$BUILD_DIR"
elif [ -n "${CMAKE_TOOLCHAIN_FILE:-}" ]; then
  cmake -S "$ROOT/launcher" -B "$BUILD_DIR" -G Ninja -DCMAKE_BUILD_TYPE=Release -DCMAKE_TOOLCHAIN_FILE="$CMAKE_TOOLCHAIN_FILE"
  cmake --build "$BUILD_DIR"
else
  echo "Agent built for $ARCH. Set CMAKE_TOOLCHAIN_FILE to cross-build the Qt launcher, or build natively on the target." >&2
  exit 0
fi
printf 'Client binaries built for %s (version %s).\n' "$ARCH" "$VERSION"
