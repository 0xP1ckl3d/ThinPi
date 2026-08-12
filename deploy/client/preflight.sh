#!/bin/sh
set -eu

PLATFORM=${1:-auto}
case "$PLATFORM" in auto|generic|raspberry-pi) ;; *) echo "Invalid platform" >&2; exit 2;; esac
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib/platform.sh"
thinpi_load_supported_os

export PATH=/usr/local/go/bin:"$PATH"
ARCH=$(dpkg --print-architecture)
case "$ARCH" in amd64|arm64) ;; *) echo "amd64 or arm64 is required; found $ARCH" >&2; exit 1;; esac
if [ "$THINPI_OS_FAMILY" = ubuntu ] && [ "$ARCH" != amd64 ]; then
  echo "Ubuntu/Lubuntu ThinPi clients currently require amd64; use Debian 13 for arm64 clients" >&2
  exit 1
fi

IS_PI=false
if [ -r /proc/device-tree/model ] && grep -q "Raspberry Pi" /proc/device-tree/model; then IS_PI=true; fi
if [ "$PLATFORM" = raspberry-pi ]; then
  [ "$ARCH" = arm64 ] && [ "$IS_PI" = true ] || { echo "The Raspberry Pi platform requires arm64 Raspberry Pi hardware" >&2; exit 1; }
fi

for tool in rsync go cmake ninja qmake6 g++; do
  command -v "$tool" >/dev/null 2>&1 || { echo "$tool is missing; install the client build prerequisites" >&2; exit 1; }
done
GO_VERSION=$(go env GOVERSION | sed 's/^go//')
dpkg --compare-versions "$GO_VERSION" ge 1.25 || { echo "Go 1.25 or newer is required; found $GO_VERSION" >&2; exit 1; }
QT_VERSION=$(qmake6 -query QT_VERSION)
dpkg --compare-versions "$QT_VERSION" ge 6.4 || { echo "Qt 6.4 or newer is required; found $QT_VERSION" >&2; exit 1; }
printf 'Preflight passed: %s %s, Go %s, Qt %s, %s, Pi=%s\n' "$THINPI_OS_ID" "$THINPI_OS_VERSION" "$GO_VERSION" "$QT_VERSION" "$ARCH" "$IS_PI"
