#!/bin/sh
set -eu

PLATFORM=${1:-auto}
case "$PLATFORM" in auto|generic|raspberry-pi) ;; *) echo "Invalid platform" >&2; exit 2;; esac
if [ "$PLATFORM" = auto ]; then
  if [ -r /proc/device-tree/model ] && grep -q "Raspberry Pi" /proc/device-tree/model; then
    PLATFORM=raspberry-pi
  else
    PLATFORM=generic
  fi
fi
export PATH=/usr/local/go/bin:"$PATH"

cd /tmp/thinpi/agent
go build -trimpath -o /tmp/thinpi-agent ./cmd/thinpi-agent
cmake -S /tmp/thinpi/launcher -B /tmp/thinpi/launcher-build -G Ninja -DCMAKE_BUILD_TYPE=Release
cmake --build /tmp/thinpi/launcher-build
sudo install -m 0755 /tmp/thinpi-agent /usr/local/bin/thinpi-agent
sudo install -m 0755 /tmp/thinpi/launcher-build/thinpi-launcher /usr/local/bin/thinpi-launcher

if systemctl list-unit-files thinpi-agent.service --no-legend 2>/dev/null | grep -q thinpi-agent; then
  sudo install -m 0755 /tmp/thinpi-agent /usr/bin/thinpi-agent
  sudo install -m 0755 /tmp/thinpi/launcher-build/thinpi-launcher /usr/bin/thinpi-launcher
  sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi-agent.service /etc/systemd/system/thinpi-agent.service
  if [ "$PLATFORM" = raspberry-pi ]; then
    sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi-ui-pi.service /etc/systemd/system/thinpi-ui.service
  else
    sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi-ui.service /etc/systemd/system/thinpi-ui.service
  fi
  sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi-maintenance@.service /etc/systemd/system/thinpi-maintenance@.service
  sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi.target /etc/systemd/system/thinpi.target
  sudo install -d -o root -g root -m 0755 /usr/local/libexec /etc/X11/xorg.conf.d /etc/ssh/sshd_config.d
  sudo install -m 0755 /tmp/thinpi/deploy-client/xinitrc /usr/local/libexec/thinpi-xinitrc
  sudo install -m 0755 /tmp/thinpi/deploy-client/maintenance-session.sh /usr/local/libexec/thinpi-maintenance-session
  sudo install -m 0755 /tmp/thinpi/deploy-client/browser-policy.sh /usr/local/libexec/thinpi-browser-policy
  sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi-xbindkeysrc /usr/local/libexec/thinpi-xbindkeysrc
  sudo install -m 0644 /tmp/thinpi/deploy-client/hardening/Xwrapper.config /etc/X11/Xwrapper.config
  sudo install -m 0644 /tmp/thinpi/deploy-client/hardening/10-thinpi-kiosk.conf /etc/X11/xorg.conf.d/10-thinpi-kiosk.conf
  sudo install -m 0644 /tmp/thinpi/deploy-client/hardening/99-thinpi-ssh.conf /etc/ssh/sshd_config.d/99-thinpi.conf
  if [ "$PLATFORM" = raspberry-pi ]; then
    [ -x /usr/lib/xorg/Xorg.wrap ] || {
      echo "The installed client is missing /usr/lib/xorg/Xorg.wrap; rerun provisioning to install xserver-xorg-legacy" >&2
      exit 1
    }
    sudo sed -i 's/^needs_root_rights=auto$/needs_root_rights=yes/' /etc/X11/Xwrapper.config
    sudo install -m 0755 /tmp/thinpi/deploy-client/detect-pi-display.sh /usr/local/libexec/thinpi-detect-display
    sudo rm -f /usr/local/libexec/thinpi-configure-pi-xorg
    sudo rm -f /etc/systemd/system/thinpi-ui.service.d/raspberry-pi.conf
    sudo rmdir /etc/systemd/system/thinpi-ui.service.d 2>/dev/null || true
    sudo /usr/local/libexec/thinpi-detect-display
  fi
  sudo systemctl daemon-reload
  sudo systemctl set-default thinpi.target
  sudo sshd -t
  command -v certutil >/dev/null 2>&1 || {
    sudo apt-get update
    sudo apt-get install -y libnss3-tools
  }
  if ! command -v xbindkeys >/dev/null 2>&1 || ! command -v xdotool >/dev/null 2>&1 || ! command -v pkill >/dev/null 2>&1; then
    sudo apt-get update
    sudo apt-get install -y --no-install-recommends xbindkeys xdotool procps
  fi
  # /etc/thinpi is intentionally inaccessible to the SSH deployment user.
  # Read the controller URL through sudo; do not hide a failed read behind a
  # pipeline whose final command exits successfully.
  CONTROLLER_URL=$(sudo sed -n 's/^THINPI_API_URL=//p' /etc/thinpi/ui.env)
  [ -z "$CONTROLLER_URL" ] || sudo /usr/local/libexec/thinpi-browser-policy "$CONTROLLER_URL"
  sudo pkill -u thinpi -x chrome 2>/dev/null || true
  sudo pkill -u thinpi -x chromium 2>/dev/null || true
  sudo systemctl restart thinpi-agent thinpi-ui
  sleep 8
  UI_STATE_FIRST=$(sudo systemctl show thinpi-ui -p ActiveState -p SubState -p NRestarts -p MainPID -p ExecMainStatus)
  UI_RESTARTS_FIRST=$(printf '%s\n' "$UI_STATE_FIRST" | sed -n 's/^NRestarts=//p')
  sleep 3
  UI_STATE_SECOND=$(sudo systemctl show thinpi-ui -p ActiveState -p SubState -p NRestarts -p MainPID -p ExecMainStatus)
  UI_ACTIVE=$(printf '%s\n' "$UI_STATE_SECOND" | sed -n 's/^ActiveState=//p')
  UI_SUBSTATE=$(printf '%s\n' "$UI_STATE_SECOND" | sed -n 's/^SubState=//p')
  UI_RESTARTS_SECOND=$(printf '%s\n' "$UI_STATE_SECOND" | sed -n 's/^NRestarts=//p')
  UI_MAIN_PID=$(printf '%s\n' "$UI_STATE_SECOND" | sed -n 's/^MainPID=//p')
  UI_EXIT_STATUS=$(printf '%s\n' "$UI_STATE_SECOND" | sed -n 's/^ExecMainStatus=//p')
  case "$UI_MAIN_PID" in ''|0|*[!0-9]*) UI_PID_VALID=false;; *) UI_PID_VALID=true;; esac
  if [ "$UI_ACTIVE" != active ] || [ "$UI_SUBSTATE" != running ] || \
     [ "$UI_PID_VALID" != true ] || [ "$UI_EXIT_STATUS" != 0 ] || \
     [ "$UI_RESTARTS_FIRST" != "$UI_RESTARTS_SECOND" ]; then
    echo "thinpi-ui did not remain stable after the update" >&2
    printf '%s\n' "$UI_STATE_FIRST" "$UI_STATE_SECOND" >&2
    sudo systemctl --no-pager --full status thinpi-ui || true
    exit 1
  fi
  sudo systemctl --no-pager --full status thinpi-agent thinpi-ui
else
  echo "Binaries staged in /usr/local/bin. Run /tmp/thinpi/deploy-client/provision.sh next."
fi
