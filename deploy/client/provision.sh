#!/bin/sh
set -eu

usage() {
  cat <<'EOF'
Usage: provision.sh --server HTTPS_URL --device-id ID [options]

Options:
  --token TOKEN                  One-time enrolment token (prompted if omitted)
  --name NAME                    Display name for this client
  --ca-certificate FILE          Private controller CA certificate
  --platform auto|generic|raspberry-pi
  --moonlight auto|yes|no        Install supported Moonlight package (default auto)
EOF
}

SERVER="" TOKEN="" DEVICE_ID="" DEVICE_NAME="ThinPi" CA_FILE=""
PLATFORM=auto MOONLIGHT=auto
while [ "$#" -gt 0 ]; do
  case "$1" in
    --server) SERVER=${2:-}; shift 2;;
    --token) TOKEN=${2:-}; shift 2;;
    --device-id) DEVICE_ID=${2:-}; shift 2;;
    --name) DEVICE_NAME=${2:-}; shift 2;;
    --ca-certificate) CA_FILE=${2:-}; shift 2;;
    --platform) PLATFORM=${2:-}; shift 2;;
    --moonlight) MOONLIGHT=${2:-}; shift 2;;
    --help|-h) usage; exit 0;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2;;
  esac
done

[ "$(id -u)" -eq 0 ] || { echo "Run as root" >&2; exit 1; }
if [ -z "$SERVER" ] || [ -z "$DEVICE_ID" ]; then usage >&2; exit 2; fi
case "$SERVER" in https://*) ;; *) echo "--server must be an HTTPS URL" >&2; exit 2;; esac
case "$SERVER" in *[[:space:]]*) echo "--server cannot contain whitespace" >&2; exit 2;; esac
case "$PLATFORM" in auto|generic|raspberry-pi) ;; *) echo "Invalid --platform value" >&2; exit 2;; esac
case "$MOONLIGHT" in auto|yes|no) ;; *) echo "Invalid --moonlight value" >&2; exit 2;; esac
[ -z "$CA_FILE" ] || [ -r "$CA_FILE" ] || { echo "--ca-certificate is not readable" >&2; exit 2; }

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib/platform.sh"
thinpi_load_supported_os

ARCH=$(dpkg --print-architecture)
case "$ARCH" in amd64|arm64) ;; *) echo "ThinPi supports amd64 and arm64 clients; got $ARCH" >&2; exit 1;; esac
if [ "$THINPI_OS_FAMILY" = ubuntu ] && [ "$ARCH" != amd64 ]; then
  echo "Ubuntu/Lubuntu ThinPi clients currently require amd64; use Debian 13 for arm64 clients" >&2
  exit 1
fi
IS_PI=false
if [ -r /proc/device-tree/model ] && grep -q "Raspberry Pi" /proc/device-tree/model; then IS_PI=true; fi
if [ "$PLATFORM" = auto ]; then
  if [ "$IS_PI" = true ]; then PLATFORM=raspberry-pi; else PLATFORM=generic; fi
fi
if [ "$PLATFORM" = raspberry-pi ]; then
  [ "$ARCH" = arm64 ] || { echo "Raspberry Pi clients require the arm64 OS" >&2; exit 1; }
  [ "$IS_PI" = true ] || { echo "--platform raspberry-pi was selected but Raspberry Pi hardware was not detected" >&2; exit 1; }
fi

if [ -x /usr/local/bin/thinpi-agent ] && [ -x /usr/local/bin/thinpi-launcher ]; then
  STAGED_BINARIES=true
elif [ -x /usr/bin/thinpi-agent ] && [ -x /usr/bin/thinpi-launcher ]; then
  STAGED_BINARIES=false
else
  echo "Stage matching $ARCH thinpi-agent and thinpi-launcher binaries before provisioning" >&2
  exit 1
fi

ADMIN_USER=${SUDO_USER:-}
[ -n "$ADMIN_USER" ] && [ "$ADMIN_USER" != root ] || ADMIN_USER=$(awk -F: '$3 >= 1000 && $7 !~ /(nologin|false)$/ {print $1; exit}' /etc/passwd)
[ -n "$ADMIN_USER" ] || { echo "No administrator account with a login shell was found" >&2; exit 1; }
case "$ADMIN_USER" in root|thinpi|*[!a-z0-9_-]*) echo "Administrator account '$ADMIN_USER' is not valid for the fixed maintenance console" >&2; exit 1;; esac
ADMIN_HOME=$(getent passwd "$ADMIN_USER" | cut -d: -f6)
[ -s "$ADMIN_HOME/.ssh/authorized_keys" ] || { echo "Refusing to disable SSH passwords until an administrator public key is installed" >&2; exit 1; }

export DEBIAN_FRONTEND=noninteractive
apt-get update

first_available_package() {
  for PACKAGE_NAME in "$@"; do
    if apt-cache show "$PACKAGE_NAME" 2>/dev/null | grep -q '^Package:'; then
      printf '%s\n' "$PACKAGE_NAME"
      return 0
    fi
  done
  echo "None of the required packages are available: $*" >&2
  return 1
}

QT_CORE_PACKAGE=$(first_available_package libqt6core6t64 libqt6core6)
QT_GUI_PACKAGE=$(first_available_package libqt6gui6t64 libqt6gui6)
QT_NETWORK_PACKAGE=$(first_available_package libqt6network6t64 libqt6network6)
apt-get install -y --no-install-recommends \
  xserver-xorg-core xserver-xorg-legacy xinit x11-xserver-utils \
  matchbox-window-manager "$QT_CORE_PACKAGE" "$QT_GUI_PACKAGE" "$QT_NETWORK_PACKAGE" libqt6qml6 \
  libqt6quick6 libqt6quickcontrols2-6 qml6-module-qtquick \
  qml6-module-qtquick-controls qml6-module-qtquick-layouts \
  qml6-module-qtquick-window pulseaudio curl ca-certificates jq \
  tigervnc-viewer tigervnc-tools openssh-client openssh-server \
  sshpass xterm kbd util-linux
if [ "$PLATFORM" = generic ]; then
  apt-get install -y --no-install-recommends xserver-xorg-video-all libgl1-mesa-dri
fi
QT_VERSION=$(dpkg-query -W -f='${Version}' "$QT_CORE_PACKAGE")
dpkg --compare-versions "$QT_VERSION" ge 6.4 || { echo "ThinPi requires Qt 6.4 or newer; installed $QT_CORE_PACKAGE is $QT_VERSION" >&2; exit 1; }
if apt-cache show freerdp3-x11 >/dev/null 2>&1; then apt-get install -y freerdp3-x11; else apt-get install -y freerdp2-x11; fi

install_google_chrome() {
  apt-get install -y --no-install-recommends gnupg
  install -d -o root -g root -m 0755 /usr/share/keyrings
  curl -fsSL https://dl.google.com/linux/linux_signing_key.pub -o /tmp/google-linux-signing-key.pub
  gpg --batch --yes --dearmor -o /usr/share/keyrings/google-chrome.gpg /tmp/google-linux-signing-key.pub
  rm -f /tmp/google-linux-signing-key.pub
  printf '%s\n' 'deb [arch=amd64 signed-by=/usr/share/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main' > /etc/apt/sources.list.d/google-chrome.list
  apt-get update
  apt-get install -y --no-install-recommends google-chrome-stable
}

if [ "$THINPI_OS_FAMILY" = ubuntu ]; then
  # Ubuntu's chromium-browser package is a Snap transition. A native managed
  # browser is required because the kiosk runs as a locked system identity.
  command -v google-chrome-stable >/dev/null 2>&1 || install_google_chrome
  ADMIN_BROWSER=/usr/bin/google-chrome-stable
else
  apt-get install -y --no-install-recommends chromium
  ADMIN_BROWSER=/usr/bin/chromium
fi
[ -x "$ADMIN_BROWSER" ] || { echo "The managed administration browser was not installed" >&2; exit 1; }

install_moonlight_repo() {
  curl -1sLf 'https://dl.cloudsmith.io/public/moonlight-game-streaming/moonlight-qt/setup.deb.sh' -o /tmp/moonlight-repo.sh
  if [ "$PLATFORM" = raspberry-pi ]; then
    distro=raspbian codename="$VERSION_CODENAME" sh /tmp/moonlight-repo.sh
  else
    sh /tmp/moonlight-repo.sh
  fi
  apt-get update
  apt-get install -y moonlight-qt
}

if command -v moonlight-qt >/dev/null 2>&1 || command -v moonlight >/dev/null 2>&1; then
  echo "Moonlight client already installed."
elif [ "$MOONLIGHT" = yes ]; then
  install_moonlight_repo
elif [ "$MOONLIGHT" = auto ] && { [ "$PLATFORM" = raspberry-pi ] || [ "$ARCH" = arm64 ]; }; then
  if ! install_moonlight_repo; then
    echo "Moonlight installation failed; rerun with --moonlight no if this hardware will not use Moonlight" >&2
    exit 1
  fi
else
  echo "Moonlight was not installed automatically on generic $ARCH. Other protocols remain available."
fi

getent group thinpi >/dev/null || groupadd --system thinpi
EXTRA_GROUPS=""
for GROUP_NAME in audio video render input; do
  if getent group "$GROUP_NAME" >/dev/null; then
    if [ -n "$EXTRA_GROUPS" ]; then EXTRA_GROUPS="$EXTRA_GROUPS,$GROUP_NAME"; else EXTRA_GROUPS=$GROUP_NAME; fi
  fi
done
if ! id thinpi >/dev/null 2>&1; then
  if [ -n "$EXTRA_GROUPS" ]; then
    useradd --system --create-home --home-dir /home/thinpi --shell /usr/sbin/nologin --gid thinpi --groups "$EXTRA_GROUPS" thinpi
  else
    useradd --system --create-home --home-dir /home/thinpi --shell /usr/sbin/nologin --gid thinpi thinpi
  fi
fi
if [ -n "$EXTRA_GROUPS" ]; then
  usermod --gid thinpi --groups "$EXTRA_GROUPS" --home /home/thinpi --shell /usr/sbin/nologin thinpi
else
  usermod --gid thinpi --groups '' --home /home/thinpi --shell /usr/sbin/nologin thinpi
fi
passwd --lock thinpi >/dev/null 2>&1 || true
install -d -o thinpi -g thinpi -m 0700 /home/thinpi/.cache /home/thinpi/.config /home/thinpi/.local
install -d -o root -g root -m 0750 /etc/thinpi
if [ "$STAGED_BINARIES" = true ]; then
  install -m 0755 /usr/local/bin/thinpi-agent /usr/bin/thinpi-agent
  install -m 0755 /usr/local/bin/thinpi-launcher /usr/bin/thinpi-launcher
fi
install -m 0644 "$SCRIPT_DIR/thinpi-agent.service" /etc/systemd/system/thinpi-agent.service
install -m 0644 "$SCRIPT_DIR/thinpi-ui.service" /etc/systemd/system/thinpi-ui.service
install -m 0644 "$SCRIPT_DIR/thinpi-maintenance@.service" /etc/systemd/system/thinpi-maintenance@.service
install -m 0644 "$SCRIPT_DIR/thinpi.target" /etc/systemd/system/thinpi.target
install -m 0755 "$SCRIPT_DIR/xinitrc" /etc/thinpi/xinitrc
install -d -o root -g root -m 0755 /usr/local/libexec /etc/X11/xorg.conf.d \
  /etc/chromium/policies/managed /etc/opt/chrome/policies/managed
install -m 0755 "$SCRIPT_DIR/maintenance-session.sh" /usr/local/libexec/thinpi-maintenance-session
install -m 0644 "$SCRIPT_DIR/hardening/10-thinpi-kiosk.conf" /etc/X11/xorg.conf.d/10-thinpi-kiosk.conf
install -m 0644 "$SCRIPT_DIR/hardening/99-thinpi-ssh.conf" /etc/ssh/sshd_config.d/99-thinpi.conf
[ -z "$CA_FILE" ] || {
  install -m 0644 "$CA_FILE" /etc/thinpi/controller-ca.pem
  install -m 0644 "$CA_FILE" /usr/local/share/ca-certificates/thinpi-controller.crt
  update-ca-certificates
}
CA_PATH=""
[ -z "$CA_FILE" ] || CA_PATH=/etc/thinpi/controller-ca.pem
jq -n --arg controller "$SERVER" --arg maintenance_user "$ADMIN_USER" --arg ca "$CA_PATH" \
  '{controller_url:$controller,device_file:"/etc/thinpi/device.json",socket:"/run/thinpi/agent.sock",freerdp_binary:"auto",moonlight_binary:"auto",vnc_binary:"auto",ssh_binary:"auto",terminal_binary:"auto",sshpass_binary:"auto",maintenance_user:$maintenance_user} + (if $ca == "" then {} else {ca_certificate:$ca} end)' \
  > /etc/thinpi/agent.json
chmod 0640 /etc/thinpi/agent.json
printf 'THINPI_API_URL=%s\nTHINPI_ADMIN_BROWSER=%s\n' "$SERVER" "$ADMIN_BROWSER" > /etc/thinpi/ui.env
chmod 0644 /etc/thinpi/ui.env
POLICY_FILE=/tmp/thinpi-browser-policy.json
jq -n --arg controller "${SERVER%/}" \
  '{URLBlocklist:["*"],URLAllowlist:[($controller+"/*")],AllowDinosaurEasterEgg:false,AllowFileSelectionDialogs:false,BookmarkBarEnabled:false,BrowserAddPersonEnabled:false,BrowserGuestModeEnabled:false,BrowserSignin:0,DefaultBrowserSettingEnabled:false,DefaultPopupsSetting:2,DeveloperToolsAvailability:2,DownloadRestrictions:3,EditBookmarksEnabled:false,ExtensionInstallBlocklist:["*"],ExternalProtocolDialogShowAlwaysOpenCheckbox:false,IncognitoModeAvailability:1,PasswordManagerEnabled:false,PrintingEnabled:false,SavingBrowserHistoryDisabled:true,SyncDisabled:true}' > "$POLICY_FILE"
install -o root -g root -m 0644 "$POLICY_FILE" /etc/chromium/policies/managed/thinpi.json
install -o root -g root -m 0644 "$POLICY_FILE" /etc/opt/chrome/policies/managed/thinpi.json
rm -f "$POLICY_FILE"
if [ -z "$TOKEN" ]; then
  printf 'One-time enrolment token: ' >/dev/tty
  stty -echo </dev/tty
  trap 'stty echo </dev/tty 2>/dev/null || true' EXIT INT TERM
  IFS= read -r TOKEN </dev/tty
  stty echo </dev/tty
  printf '\n' >/dev/tty
  trap - EXIT INT TERM
fi
[ -n "$TOKEN" ] || { echo "An enrolment token is required" >&2; exit 2; }
if [ -n "$CA_PATH" ]; then
  printf '%s\n' "$TOKEN" | /usr/bin/thinpi-agent enrol --server "$SERVER" --token-stdin --device-id "$DEVICE_ID" --name "$DEVICE_NAME" --device-file /etc/thinpi/device.json --ca-certificate "$CA_PATH"
else
  printf '%s\n' "$TOKEN" | /usr/bin/thinpi-agent enrol --server "$SERVER" --token-stdin --device-id "$DEVICE_ID" --name "$DEVICE_NAME" --device-file /etc/thinpi/device.json
fi
unset TOKEN
chmod 0600 /etc/thinpi/device.json

systemctl disable --now display-manager.service lightdm.service gdm.service sddm.service 2>/dev/null || true
systemctl mask getty@tty1.service getty@tty2.service getty@tty3.service getty@tty4.service getty@tty5.service getty@tty6.service getty@tty7.service
systemctl daemon-reload
systemctl enable thinpi-agent.service thinpi-ui.service thinpi.target
systemctl set-default thinpi.target
sshd -t
xfreerdp3 /help >/var/log/thinpi-freerdp-help.txt 2>&1 || xfreerdp /help >/var/log/thinpi-freerdp-help.txt 2>&1 || true
moonlight-qt --help >/var/log/thinpi-moonlight-help.txt 2>&1 || moonlight --help >/var/log/thinpi-moonlight-help.txt 2>&1 || true
xtigervncviewer -h >/var/log/thinpi-vncviewer-help.txt 2>&1 || true
xterm -version >/var/log/thinpi-xterm-version.txt 2>&1
ssh -V >/var/log/thinpi-ssh-version.txt 2>&1
sshpass -V >/var/log/thinpi-sshpass-version.txt 2>&1
"$ADMIN_BROWSER" --version >/var/log/thinpi-admin-browser-version.txt 2>&1
echo "Provisioning complete for $PLATFORM/$ARCH on $THINPI_OS_ID $THINPI_OS_VERSION. Reboot to start the ThinPi kiosk."
