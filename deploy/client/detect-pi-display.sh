#!/bin/bash
set -euo pipefail

SYS_DRM_ROOT=${THINPI_DRM_SYSFS_ROOT:-/sys/class/drm}
DRI_ROOT=${THINPI_DRI_ROOT:-/dev/dri}
XORG_CONFIG=${THINPI_XORG_CONFIG:-/etc/X11/xorg.conf.d/99-thinpi-vc4.conf}
WAIT_ATTEMPTS=${THINPI_DISPLAY_WAIT_ATTEMPTS:-20}
WAIT_DELAY=${THINPI_DISPLAY_WAIT_DELAY:-0.25}
CARD=""

# Cold-boot DRM connector enumeration can lag service startup. Prefer the card
# with a connected HDMI output once enumeration settles.
for _ in $(seq 1 "$WAIT_ATTEMPTS"); do
  for STATUS in "$SYS_DRM_ROOT"/card[0-9]*-HDMI-A-*/status; do
    [ -e "$STATUS" ] || continue
    if [ "$(cat "$STATUS")" = connected ]; then
      CONNECTOR=$(basename "$(dirname "$STATUS")")
      CARD=${CONNECTOR%%-HDMI-A-*}
      break 2
    fi
  done
  sleep "$WAIT_DELAY"
done

# Permit headless provisioning by selecting the display controller that owns
# an HDMI connector rather than the render-only V3D device.
if [ -z "$CARD" ]; then
  for STATUS in "$SYS_DRM_ROOT"/card[0-9]*-HDMI-A-*/status; do
    [ -e "$STATUS" ] || continue
    CONNECTOR=$(basename "$(dirname "$STATUS")")
    CARD=${CONNECTOR%%-HDMI-A-*}
    break
  done
fi

[ -n "$CARD" ] || {
  echo "ThinPi: unable to identify HDMI-capable DRM device" >&2
  exit 1
}

DRM_DEVICE=$DRI_ROOT/$CARD
[ -e "$DRM_DEVICE" ] || {
  echo "ThinPi: selected display DRM device does not exist: $DRM_DEVICE" >&2
  exit 1
}

echo "ThinPi: using display DRM device $DRM_DEVICE"
install -d -m 0755 "$(dirname "$XORG_CONFIG")"

# Remove the filename used by the physically tested interim repair so ThinPi
# owns exactly one VC4 Device/Screen definition.
rm -f "$(dirname "$XORG_CONFIG")/99-vc4.conf"

CONFIG_TEMP=$(mktemp "$XORG_CONFIG.tmp.XXXXXX")
trap 'rm -f "$CONFIG_TEMP"' EXIT HUP INT TERM
cat > "$CONFIG_TEMP" <<XORG
Section "Device"
    Identifier "VC4"
    Driver "modesetting"
    Option "kmsdev" "$DRM_DEVICE"
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
XORG
chmod 0644 "$CONFIG_TEMP"
mv -f "$CONFIG_TEMP" "$XORG_CONFIG"
trap - EXIT HUP INT TERM
