#!/bin/sh
set -eu

[ "$(id -u)" -eq 0 ] || { echo "Run as root" >&2; exit 1; }
[ "$#" -eq 2 ] || { echo "Usage: $0 thinpi-agent_VERSION_ARCH.deb thinpi-launcher_VERSION_ARCH.deb" >&2; exit 2; }

HOST_ARCH=$(dpkg --print-architecture)
AGENT_PACKAGE=""
LAUNCHER_PACKAGE=""
for PACKAGE in "$@"; do
  [ -r "$PACKAGE" ] || { echo "Package is not readable: $PACKAGE" >&2; exit 2; }
  PACKAGE=$(readlink -f -- "$PACKAGE")
  PACKAGE_NAME=$(dpkg-deb -f "$PACKAGE" Package)
  PACKAGE_ARCH=$(dpkg-deb -f "$PACKAGE" Architecture)
  case "$PACKAGE_ARCH" in "$HOST_ARCH"|all) ;; *) echo "$PACKAGE is for $PACKAGE_ARCH, not $HOST_ARCH" >&2; exit 2;; esac
  case "$PACKAGE_NAME" in
    thinpi-agent) [ -z "$AGENT_PACKAGE" ] || { echo "Duplicate thinpi-agent package" >&2; exit 2; }; AGENT_PACKAGE=$PACKAGE ;;
    thinpi-launcher) [ -z "$LAUNCHER_PACKAGE" ] || { echo "Duplicate thinpi-launcher package" >&2; exit 2; }; LAUNCHER_PACKAGE=$PACKAGE ;;
    *) echo "Refusing unexpected package $PACKAGE_NAME" >&2; exit 2 ;;
  esac
done
if [ -z "$AGENT_PACKAGE" ] || [ -z "$LAUNCHER_PACKAGE" ]; then echo "Provide one agent and one launcher package" >&2; exit 2; fi

AGENT_VERSION=$(dpkg-deb -f "$AGENT_PACKAGE" Version)
LAUNCHER_VERSION=$(dpkg-deb -f "$LAUNCHER_PACKAGE" Version)
[ "$AGENT_VERSION" = "$LAUNCHER_VERSION" ] || { echo "Agent and launcher versions must match" >&2; exit 2; }

apt-get install -y "$AGENT_PACKAGE" "$LAUNCHER_PACKAGE"
systemctl daemon-reload
systemctl restart thinpi-agent thinpi-ui
systemctl --no-pager --full status thinpi-agent thinpi-ui
