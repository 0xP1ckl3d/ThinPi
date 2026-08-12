#!/bin/sh
set -eu

SYS_DRM_ROOT=${THINPI_DRM_SYSFS_ROOT:-/sys/class/drm}
DRI_ROOT=${THINPI_DRI_ROOT:-/dev/dri}
XORG_CONFIG=${THINPI_XORG_CONFIG:-/etc/X11/xorg.conf.d/99-thinpi-vc4.conf}

CARD=""

# Prefer the display controller with an actively connected HDMI output.
for STATUS in "$SYS_DRM_ROOT"/card[0-9]*-HDMI-A-*/status; do
  [ -e "$STATUS" ] || continue
  if [ "$(cat "$STATUS")" = connected ]; then
    CONNECTOR=$(basename "$(dirname "$STATUS")")
    CARD=${CONNECTOR%%-HDMI-A-*}
    break
  fi
done

# A display may be disconnected while provisioning. In that case, use the DRM
# card that exposes an HDMI connector rather than the render-only V3D card.
if [ -z "$CARD" ]; then
  for STATUS in "$SYS_DRM_ROOT"/card[0-9]*-HDMI-A-*/status; do
    [ -e "$STATUS" ] || continue
    CONNECTOR=$(basename "$(dirname "$STATUS")")
    CARD=${CONNECTOR%%-HDMI-A-*}
    break
  done
fi

[ -n "$CARD" ] || {
  echo "Unable to identify the Raspberry Pi display DRM device" >&2
  exit 1
}

CARD_DEV=$DRI_ROOT/$CARD
[ -e "$CARD_DEV" ] || {
  echo "The selected Raspberry Pi display DRM device does not exist: $CARD_DEV" >&2
  exit 1
}
CARD_REAL=$(readlink -f "$CARD_DEV")
KMS_DEV=""
for PATH_ENTRY in "$DRI_ROOT"/by-path/*-card; do
  [ -e "$PATH_ENTRY" ] || continue
  if [ "$(readlink -f "$PATH_ENTRY")" = "$CARD_REAL" ]; then
    KMS_DEV=$PATH_ENTRY
    break
  fi
done

[ -n "$KMS_DEV" ] || {
  echo "No stable $DRI_ROOT/by-path mapping found for $CARD_DEV" >&2
  exit 1
}

install -d -m 0755 "$(dirname "$XORG_CONFIG")"
CONFIG_TEMP=$(mktemp "$XORG_CONFIG.tmp.XXXXXX")
trap 'rm -f "$CONFIG_TEMP"' EXIT HUP INT TERM
cat > "$CONFIG_TEMP" <<EOF
Section "Device"
    Identifier "VC4"
    Driver "modesetting"
    Option "kmsdev" "$KMS_DEV"
    Option "AccelMethod" "glamor"
    Option "PrimaryGPU" "true"
EndSection

Section "Screen"
    Identifier "Default Screen"
    Device "VC4"
EndSection

Section "ServerLayout"
    Identifier "Default Layout"
    Screen "Default Screen"
EndSection
EOF
chmod 0644 "$CONFIG_TEMP"
mv -f "$CONFIG_TEMP" "$XORG_CONFIG"
trap - EXIT HUP INT TERM
