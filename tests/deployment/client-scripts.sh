#!/bin/sh
set -eu

ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd)
FILES="
$ROOT/scripts/build-client.sh
$ROOT/scripts/build-arm64.sh
$ROOT/scripts/package-debs.sh
$ROOT/scripts/deploy-client.sh
$ROOT/scripts/deploy-pi.sh
$ROOT/deploy/client/lib/platform.sh
$ROOT/deploy/client/preflight.sh
$ROOT/deploy/client/provision.sh
$ROOT/deploy/client/stage.sh
$ROOT/deploy/client/update.sh
$ROOT/deploy/client/maintenance-session.sh
$ROOT/deploy/client/xinitrc
$ROOT/deploy/client/prepare-pi-audio.sh
$ROOT/deploy/client/detect-pi-display.sh
$ROOT/deploy/pi/provision.sh
$ROOT/deploy/pi/update.sh
"
for FILE in $FILES; do
  sh -n "$FILE"
done

grep -F 'amd64|arm64' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F '24.04|26.04' "$ROOT/deploy/client/lib/platform.sh" >/dev/null
grep -F 'Generic ThinPi clients require Lubuntu 24.04 or 26.04 LTS on amd64' "$ROOT/deploy/client/preflight.sh" >/dev/null
grep -F 'Raspberry Pi clients require Raspberry Pi OS Lite 64-bit' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'google-chrome-stable' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'THINPI_ADMIN_BROWSER' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'thinpi-browser-policy' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'URLBlocklist:["*"]' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
grep -F 'URLAllowlist:[$controller]' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
! grep -F '/*' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
grep -F '/home/thinpi/.local/share/pki/nssdb' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
grep -F 'certutil -A' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
grep -F 'CA_IMPORT_FILE=$(mktemp)' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
grep -F "CONTROLLER_URL=\$(sudo sed" "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F 'xhost +SI:localuser:thinpi' "$ROOT/deploy/client/xinitrc" >/dev/null
grep -F 'pulseaudio -n --start --file=/usr/local/libexec/thinpi-pulse.pa' "$ROOT/deploy/client/xinitrc" >/dev/null
grep -F 'pulseaudio -n --start --file="$PULSE_CONFIG"' "$ROOT/deploy/client/prepare-pi-audio.sh" >/dev/null
grep -F 'exec dbus-run-session -- "$0" --session-bus' "$ROOT/deploy/client/xinitrc" >/dev/null
grep -F 'pactl move-sink-input "$INPUT" thinpi_parking' "$ROOT/deploy/client/prepare-pi-audio.sh" >/dev/null
grep -F 'pactl unload-module "$MODULE_INDEX"' "$ROOT/deploy/client/prepare-pi-audio.sh" >/dev/null
grep -F 'pulseaudio --kill' "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F 'TTYPath=/dev/tty2' "$ROOT/deploy/client/thinpi-maintenance@.service" >/dev/null
grep -F 'pipewire-audio pipewire-alsa pipewire-pulse' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'bash /tmp/moonlight-repo.sh' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'install_moonlight_appimage_amd64' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'ControlMaster=yes' "$ROOT/scripts/deploy-client.sh" >/dev/null
grep -F 'ControlPath=' "$ROOT/scripts/deploy-client.sh" >/dev/null
grep -F -- '--disable-ssh-passwords' "$ROOT/deploy/client/provision.sh" >/dev/null
if grep -F 'PasswordAuthentication no' "$ROOT/deploy/client/hardening/99-thinpi-ssh.conf" >/dev/null; then
  echo "Default SSH hardening must preserve the host password-authentication setting" >&2
  exit 1
fi
grep -F 'Requires=multi-user.target' "$ROOT/deploy/client/thinpi.target" >/dev/null
! grep -F 'Alias=default.target' "$ROOT/deploy/client/thinpi.target" >/dev/null
grep -F 'systemctl set-default thinpi.target' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'sudo systemctl set-default thinpi.target' "$ROOT/deploy/client/stage.sh" >/dev/null
if grep -E 'systemctl enable .*thinpi\.target' "$ROOT/deploy/client/provision.sh" >/dev/null; then
  echo "thinpi.target must be selected with systemctl set-default, not enabled" >&2
  exit 1
fi
grep -F 'qt6-svg-plugins' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'xserver-xorg-input-libinput' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'needs_root_rights=auto' "$ROOT/deploy/client/hardening/Xwrapper.config" >/dev/null
grep -F "s/^needs_root_rights=auto\$/needs_root_rights=yes/" "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F "s/^needs_root_rights=auto\$/needs_root_rights=yes/" "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F '/usr/bin/xinit /usr/local/libexec/thinpi-xinitrc -- :0' "$ROOT/deploy/client/thinpi-ui.service" >/dev/null
grep -F 'ExecStartPre=/usr/local/libexec/thinpi-detect-display' "$ROOT/deploy/client/thinpi-ui-pi.service" >/dev/null
grep -F 'ExecStart=/usr/sbin/runuser -u thinpi -- /usr/bin/xinit' "$ROOT/deploy/client/thinpi-ui-pi.service" >/dev/null
grep -F 'ExecStartPost=/usr/bin/chvt 7' "$ROOT/deploy/client/thinpi-ui-pi.service" >/dev/null
grep -F 'xbindkeys -n -f /usr/local/libexec/thinpi-xbindkeysrc' "$ROOT/deploy/client/xinitrc" >/dev/null
grep -F 'Mod4 + l' "$ROOT/deploy/client/thinpi-xbindkeysrc" >/dev/null
grep -F 'Mod4 + m' "$ROOT/deploy/client/thinpi-xbindkeysrc" >/dev/null
grep -F 'xbindkeys xdotool procps' "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F 'xbindkeys xdotool procps' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'screen_sleep_minutes' "$ROOT/launcher/src/backend.cpp" >/dev/null
grep -F 'matchbox-window-manager -use_titlebar no &' "$ROOT/deploy/client/xinitrc" >/dev/null
! grep -F -- '-use_cursor no' "$ROOT/deploy/client/xinitrc" >/dev/null
! grep -F 'NoNewPrivileges=' "$ROOT/deploy/client/thinpi-ui-pi.service" >/dev/null
! grep -F 'PAMName=' "$ROOT/deploy/client/thinpi-ui-pi.service" >/dev/null
grep -F '[ -x /usr/lib/xorg/Xorg.wrap ]' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F '[ -x /usr/lib/xorg/Xorg.wrap ]' "$ROOT/deploy/client/stage.sh" >/dev/null
if grep -E '/dev/dri/card[01]([^0-9]|$)' "$ROOT/deploy/client/detect-pi-display.sh" >/dev/null; then
  echo "The Pi display detector must not hard-code a probe-order-dependent DRM card" >&2
  exit 1
fi
grep -F 'thinpi-detect-display' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'sudo /usr/local/libexec/thinpi-detect-display' "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F '99-vc4.conf' "$ROOT/deploy/client/detect-pi-display.sh" >/dev/null
grep -F 'NoNewPrivileges=true' "$ROOT/deploy/client/thinpi-ui.service" >/dev/null
grep -F 'thinpi-ui-pi.service' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'thinpi-ui-pi.service' "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F 'UI_RESTARTS_FIRST' "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F 'UI_RESTARTS_SECOND' "$ROOT/deploy/client/stage.sh" >/dev/null
grep -F './scripts/deploy-pi.sh pickle@localhost' "$ROOT/docs/pi-deployment.md" >/dev/null
grep -F -- '--platform raspberry-pi' "$ROOT/scripts/deploy-pi.sh" >/dev/null
grep -F 'deploy/client/provision.sh' "$ROOT/deploy/pi/provision.sh" >/dev/null
grep -F 'Architecture: %s' "$ROOT/scripts/package-debs.sh" >/dev/null

FIXTURE_DIR=$(mktemp -d)
trap 'rm -rf "$FIXTURE_DIR"' EXIT INT TERM
check_os() {
  NAME=$1 EXPECTED=$2
  RESULT=$(THINPI_OS_RELEASE_FILE="$FIXTURE_DIR/$NAME" sh -c '. "$1"; thinpi_load_supported_os; printf "%s:%s" "$THINPI_OS_FAMILY" "$THINPI_OS_VERSION"' sh "$ROOT/deploy/client/lib/platform.sh")
  [ "$RESULT" = "$EXPECTED" ] || { echo "$NAME produced $RESULT, expected $EXPECTED" >&2; exit 1; }
}
printf '%s\n' 'ID=debian' 'VERSION_ID="13"' 'VERSION_CODENAME=trixie' 'PRETTY_NAME="Debian GNU/Linux 13"' > "$FIXTURE_DIR/debian"
printf '%s\n' 'ID=raspbian' 'VERSION_ID="13"' 'VERSION_CODENAME=trixie' 'PRETTY_NAME="Raspberry Pi OS 13"' > "$FIXTURE_DIR/raspbian"
printf '%s\n' 'ID=ubuntu' 'VERSION_ID="24.04"' 'VERSION_CODENAME=noble' 'PRETTY_NAME="Ubuntu 24.04 LTS"' > "$FIXTURE_DIR/ubuntu-2404"
printf '%s\n' 'ID=ubuntu' 'VERSION_ID="26.04"' 'VERSION_CODENAME=resolute' 'VARIANT="Lubuntu"' 'PRETTY_NAME="Lubuntu 26.04 LTS"' > "$FIXTURE_DIR/lubuntu-2604"
printf '%s\n' 'ID=ubuntu' 'VERSION_ID="22.04"' 'VERSION_CODENAME=jammy' 'PRETTY_NAME="Ubuntu 22.04 LTS"' > "$FIXTURE_DIR/unsupported"
check_os debian debian:13
check_os raspbian debian:13
check_os ubuntu-2404 ubuntu:24.04
check_os lubuntu-2604 ubuntu:26.04
if THINPI_OS_RELEASE_FILE="$FIXTURE_DIR/unsupported" sh -c '. "$1"; thinpi_load_supported_os' sh "$ROOT/deploy/client/lib/platform.sh" >/dev/null 2>&1; then
  echo "Ubuntu 22.04 should be rejected" >&2
  exit 1
fi

mkdir -p "$FIXTURE_DIR/sys/class/drm/card0-HDMI-A-1" \
  "$FIXTURE_DIR/sys/class/drm/card1-HDMI-A-1" "$FIXTURE_DIR/dev/dri" \
  "$FIXTURE_DIR/etc/X11/xorg.conf.d"
printf '%s\n' disconnected > "$FIXTURE_DIR/sys/class/drm/card0-HDMI-A-1/status"
printf '%s\n' connected > "$FIXTURE_DIR/sys/class/drm/card1-HDMI-A-1/status"
: > "$FIXTURE_DIR/dev/dri/card0"
: > "$FIXTURE_DIR/dev/dri/card1"
: > "$FIXTURE_DIR/etc/X11/xorg.conf.d/99-vc4.conf"
PI_DISPLAY_SHELL=bash
if sh -c 'set -o pipefail' >/dev/null 2>&1; then PI_DISPLAY_SHELL=sh; fi
THINPI_DRM_SYSFS_ROOT="$FIXTURE_DIR/sys/class/drm" \
THINPI_DRI_ROOT="$FIXTURE_DIR/dev/dri" \
THINPI_XORG_CONFIG="$FIXTURE_DIR/etc/X11/xorg.conf.d/99-thinpi-vc4.conf" \
THINPI_DISPLAY_WAIT_ATTEMPTS=1 THINPI_DISPLAY_WAIT_DELAY=0 \
  "$PI_DISPLAY_SHELL" "$ROOT/deploy/client/detect-pi-display.sh"
grep -F "Option \"kmsdev\" \"$FIXTURE_DIR/dev/dri/card1\"" \
  "$FIXTURE_DIR/etc/X11/xorg.conf.d/99-thinpi-vc4.conf" >/dev/null
[ ! -e "$FIXTURE_DIR/etc/X11/xorg.conf.d/99-vc4.conf" ]

printf '%s\n' disconnected > "$FIXTURE_DIR/sys/class/drm/card1-HDMI-A-1/status"
THINPI_DRM_SYSFS_ROOT="$FIXTURE_DIR/sys/class/drm" \
THINPI_DRI_ROOT="$FIXTURE_DIR/dev/dri" \
THINPI_XORG_CONFIG="$FIXTURE_DIR/etc/X11/xorg.conf.d/99-thinpi-vc4.conf" \
THINPI_DISPLAY_WAIT_ATTEMPTS=1 THINPI_DISPLAY_WAIT_DELAY=0 \
  "$PI_DISPLAY_SHELL" "$ROOT/deploy/client/detect-pi-display.sh"
grep -F "Option \"kmsdev\" \"$FIXTURE_DIR/dev/dri/card0\"" \
  "$FIXTURE_DIR/etc/X11/xorg.conf.d/99-thinpi-vc4.conf" >/dev/null

echo "Client deployment script checks passed."
