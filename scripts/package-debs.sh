#!/bin/sh
set -eu
VERSION=${1:?Usage: package-debs.sh VERSION [amd64|arm64]}
ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
printf '%s\n' "$VERSION" | grep -Eq '^[0-9][0-9A-Za-z.+:~-]*$' || { echo "VERSION is not a safe Debian package version" >&2; exit 2; }
ARCH=${2:-}
if [ -z "$ARCH" ]; then
  if command -v dpkg >/dev/null 2>&1; then ARCH=$(dpkg --print-architecture); else ARCH=$(uname -m); fi
fi
case "$ARCH" in amd64|x86_64) ARCH=amd64;; arm64|aarch64) ARCH=arm64;; *) echo "Architecture must be amd64 or arm64" >&2; exit 2;; esac
OUT="$ROOT/bin/packages"; mkdir -p "$OUT"
[ -x "$ROOT/bin/$ARCH/thinpi-agent" ] || { echo "Run scripts/build-client.sh $ARCH first" >&2; exit 1; }
[ -x "$ROOT/build/launcher-$ARCH/thinpi-launcher" ] || { echo "$ARCH launcher build missing" >&2; exit 1; }
for package in agent launcher; do
  STAGE=$(mktemp -d)
  mkdir -p "$STAGE/DEBIAN" "$STAGE/usr/bin"
  if [ "$package" = agent ]; then BIN="$ROOT/bin/$ARCH/thinpi-agent"; NAME=thinpi-agent; DESC="ThinPi secure native-client agent"; DEPENDS="ca-certificates"; else BIN="$ROOT/build/launcher-$ARCH/thinpi-launcher"; NAME=thinpi-launcher; DESC="ThinPi Qt kiosk launcher"; DEPENDS="jq, libnss3-tools, libqt6core6t64 | libqt6core6, libqt6gui6, libqt6network6, libqt6qml6, libqt6quick6, libqt6quickcontrols2-6, qml6-module-qtquick, qml6-module-qtquick-controls, qml6-module-qtquick-layouts, qml6-module-qtquick-window, xinit, matchbox-window-manager"; fi
  install -m 0755 "$BIN" "$STAGE/usr/bin/$NAME"
  if [ "$package" = agent ]; then
    mkdir -p "$STAGE/lib/systemd/system" "$STAGE/usr/share/thinpi"
    install -m 0644 "$ROOT/deploy/client/thinpi-agent.service" "$STAGE/lib/systemd/system/"
    install -m 0644 "$ROOT/deploy/client/agent.json.example" "$STAGE/usr/share/thinpi/agent.json.example"
  else
    mkdir -p "$STAGE/lib/systemd/system" "$STAGE/etc/thinpi" "$STAGE/etc/X11/xorg.conf.d" "$STAGE/usr/local/libexec"
    install -m 0644 "$ROOT/deploy/client/thinpi-ui.service" "$STAGE/lib/systemd/system/"
    install -m 0644 "$ROOT/deploy/client/thinpi-maintenance@.service" "$STAGE/lib/systemd/system/"
    install -m 0644 "$ROOT/deploy/client/thinpi.target" "$STAGE/lib/systemd/system/"
    install -m 0755 "$ROOT/deploy/client/xinitrc" "$STAGE/usr/local/libexec/thinpi-xinitrc"
    install -m 0755 "$ROOT/deploy/client/maintenance-session.sh" "$STAGE/usr/local/libexec/thinpi-maintenance-session"
    install -m 0755 "$ROOT/deploy/client/browser-policy.sh" "$STAGE/usr/local/libexec/thinpi-browser-policy"
    install -m 0644 "$ROOT/deploy/client/hardening/Xwrapper.config" "$STAGE/etc/X11/Xwrapper.config"
    install -m 0644 "$ROOT/deploy/client/hardening/10-thinpi-kiosk.conf" "$STAGE/etc/X11/xorg.conf.d/10-thinpi-kiosk.conf"
  fi
  printf 'Package: %s\nVersion: %s\nSection: net\nPriority: optional\nArchitecture: %s\nMaintainer: ThinPi\nDepends: %s\nDescription: %s\n' "$NAME" "$VERSION" "$ARCH" "$DEPENDS" "$DESC" > "$STAGE/DEBIAN/control"
  printf '#!/bin/sh\nset -e\nsystemctl daemon-reload 2>/dev/null || true\n' > "$STAGE/DEBIAN/postinst"
  chmod 0755 "$STAGE/DEBIAN/postinst"
  dpkg-deb --root-owner-group --build "$STAGE" "$OUT/${NAME}_${VERSION}_${ARCH}.deb"
  rm -rf "$STAGE"
done
