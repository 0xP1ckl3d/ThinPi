#!/bin/sh
set -eu

USER_NAME=${1:-}
case "$USER_NAME" in
  ""|root|thinpi|*[!a-z0-9_-]*) exit 1 ;;
esac
ENTRY=$(getent passwd "$USER_NAME")
[ -n "$ENTRY" ] || exit 1
UID_VALUE=$(printf '%s\n' "$ENTRY" | cut -d: -f3)
SHELL_VALUE=$(printf '%s\n' "$ENTRY" | cut -d: -f7)
[ "$UID_VALUE" -ge 1000 ] || exit 1
case "$SHELL_VALUE" in */nologin|*/false|"") exit 1 ;; esac

return_to_kiosk() {
  clear 2>/dev/null || true
  chvt 7 2>/dev/null || true
}
trap return_to_kiosk EXIT INT TERM HUP

clear
printf '%s\n' \
  'ThinPi local maintenance' \
  '========================' \
  "Controller administrator access authorised for OS account: $USER_NAME" \
  'This is the local ThinPi appliance, not a remote SSH connection.' \
  'Run exit when finished. The kiosk will return to its locked login screen.' \
  ''
/usr/sbin/runuser --login "$USER_NAME"
