#!/bin/sh
set -eu

CONTROLLER_URL=${1:?Usage: thinpi-browser-policy https://controller.example:8443}
CONTROLLER_URL=${CONTROLLER_URL%/}
case "$CONTROLLER_URL" in
  https://*) ;;
  *) echo "Controller URL must start with https://" >&2; exit 2 ;;
esac
case "$CONTROLLER_URL" in
  *[[:space:]]*) echo "Controller URL must not contain whitespace" >&2; exit 2 ;;
esac

# Allow the configured controller origin and nothing else.  Using the origin
# (no path component) covers the handoff redirect, admin pages, static assets
# and API calls without relying on a fragile list of individual URL prefixes.
POLICY_FILE=$(mktemp)
trap 'rm -f "$POLICY_FILE"' EXIT HUP INT TERM
jq -n --arg controller "$CONTROLLER_URL" \
  '{URLBlocklist:["*"],URLAllowlist:[$controller],AllowDinosaurEasterEgg:false,AllowFileSelectionDialogs:false,BookmarkBarEnabled:false,BrowserAddPersonEnabled:false,BrowserGuestModeEnabled:false,BrowserSignin:0,DefaultBrowserSettingEnabled:false,DefaultPopupsSetting:2,DeveloperToolsAvailability:2,DownloadRestrictions:3,EditBookmarksEnabled:false,ExtensionInstallBlocklist:["*"],ExternalProtocolDialogShowAlwaysOpenCheckbox:false,IncognitoModeAvailability:1,PasswordManagerEnabled:false,PrintingEnabled:false,SavingBrowserHistoryDisabled:true,SyncDisabled:true}' > "$POLICY_FILE"

install -d -o root -g root -m 0755 \
  /etc/chromium/policies/managed /etc/opt/chrome/policies/managed
install -o root -g root -m 0644 "$POLICY_FILE" /etc/chromium/policies/managed/thinpi.json
install -o root -g root -m 0644 "$POLICY_FILE" /etc/opt/chrome/policies/managed/thinpi.json

# Chrome 146+ uses the NSS shared database below rather than Ubuntu's PEM
# bundle for locally-added private roots. Import the controller CA into the
# locked kiosk identity's database so the managed browser and the agent trust
# the same controller certificate.
CA_FILE=/etc/thinpi/controller-ca.pem
if [ -r "$CA_FILE" ]; then
  command -v certutil >/dev/null 2>&1 || {
    echo "certutil is required to install the controller CA for Chrome" >&2
    exit 1
  }
  NSS_DIR=/home/thinpi/.local/share/pki/nssdb
  if [ -d /home/thinpi/.pki/nssdb ]; then
    NSS_DIR=/home/thinpi/.pki/nssdb
  fi
  install -d -o thinpi -g thinpi -m 0700 "$NSS_DIR"
  if [ ! -f "$NSS_DIR/cert9.db" ]; then
    runuser -u thinpi -- certutil -N --empty-password -d "sql:$NSS_DIR"
  fi
  CA_IMPORT_FILE=$(mktemp)
  trap 'rm -f "$POLICY_FILE" "$CA_IMPORT_FILE"' EXIT HUP INT TERM
  install -o thinpi -g thinpi -m 0400 "$CA_FILE" "$CA_IMPORT_FILE"
  runuser -u thinpi -- certutil -D -d "sql:$NSS_DIR" -n "ThinPi Controller CA" >/dev/null 2>&1 || true
  runuser -u thinpi -- certutil -A -d "sql:$NSS_DIR" -t "C,," -n "ThinPi Controller CA" -i "$CA_IMPORT_FILE"
  runuser -u thinpi -- certutil -L -d "sql:$NSS_DIR" -n "ThinPi Controller CA" >/dev/null
fi
