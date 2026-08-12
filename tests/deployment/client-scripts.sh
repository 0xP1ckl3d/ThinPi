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
$ROOT/deploy/pi/provision.sh
$ROOT/deploy/pi/update.sh
"
for FILE in $FILES; do
  sh -n "$FILE"
done

grep -F 'amd64|arm64' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F '24.04|26.04' "$ROOT/deploy/client/lib/platform.sh" >/dev/null
grep -F 'google-chrome-stable' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'THINPI_ADMIN_BROWSER' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'thinpi-browser-policy' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'URLAllowlist:[$controller]' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
! grep -F '($controller+"/*")' "$ROOT/deploy/client/browser-policy.sh" >/dev/null
grep -F 'xhost +SI:localuser:thinpi' "$ROOT/deploy/client/xinitrc" >/dev/null
grep -F 'TTYPath=/dev/tty2' "$ROOT/deploy/client/thinpi-maintenance@.service" >/dev/null
grep -F 'pipewire-audio pipewire-alsa pipewire-pulse' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'bash /tmp/moonlight-repo.sh' "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F "THINPI_OS_VERSION\" = 26.04" "$ROOT/deploy/client/provision.sh" >/dev/null
grep -F 'ControlMaster=yes' "$ROOT/scripts/deploy-client.sh" >/dev/null
grep -F 'ControlPath=' "$ROOT/scripts/deploy-client.sh" >/dev/null
grep -F -- '--disable-ssh-passwords' "$ROOT/deploy/client/provision.sh" >/dev/null
if grep -F 'PasswordAuthentication no' "$ROOT/deploy/client/hardening/99-thinpi-ssh.conf" >/dev/null; then
  echo "Default SSH hardening must preserve the host password-authentication setting" >&2
  exit 1
fi
grep -F 'Requires=multi-user.target' "$ROOT/deploy/client/thinpi.target" >/dev/null
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

echo "Client deployment script checks passed."
