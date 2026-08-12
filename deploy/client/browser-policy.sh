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
POLICY_FILE=$(mktemp)
trap 'rm -f "$POLICY_FILE"' EXIT HUP INT TERM
jq -n \
  '{AllowDinosaurEasterEgg:false,AllowFileSelectionDialogs:false,BookmarkBarEnabled:false,BrowserAddPersonEnabled:false,BrowserGuestModeEnabled:false,BrowserSignin:0,DefaultBrowserSettingEnabled:false,DefaultPopupsSetting:2,DeveloperToolsAvailability:2,DownloadRestrictions:3,EditBookmarksEnabled:false,ExtensionInstallBlocklist:["*"],ExternalProtocolDialogShowAlwaysOpenCheckbox:false,IncognitoModeAvailability:1,PasswordManagerEnabled:false,PrintingEnabled:false,SavingBrowserHistoryDisabled:true,SyncDisabled:true}' > "$POLICY_FILE"

install -d -o root -g root -m 0755 \
  /etc/chromium/policies/managed /etc/opt/chrome/policies/managed
install -o root -g root -m 0644 "$POLICY_FILE" /etc/chromium/policies/managed/thinpi.json
install -o root -g root -m 0644 "$POLICY_FILE" /etc/opt/chrome/policies/managed/thinpi.json
