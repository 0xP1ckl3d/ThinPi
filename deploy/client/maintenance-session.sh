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

THEME=$(cat /run/thinpi/maintenance-theme 2>/dev/null || printf dark)
case "$THEME" in
  light) FOREGROUND='#172033'; BACKGROUND='#f4f6fa' ;;
  dark) FOREGROUND='#e8edf4'; BACKGROUND='#07111e' ;;
  *) exit 1 ;;
esac

exec /usr/bin/xterm \
  -title 'ThinPi local maintenance' \
  -fullscreen \
  -xrm 'XTerm*selectToClipboard: true' \
  -xrm "XTerm*foreground: $FOREGROUND" \
  -xrm "XTerm*background: $BACKGROUND" \
  -xrm 'XTerm*scrollBar: false' \
  -xrm 'XTerm*toolBar: false' \
  -e "$SHELL_VALUE" -l
