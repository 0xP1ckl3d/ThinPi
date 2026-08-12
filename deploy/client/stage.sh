#!/bin/sh
set -eu

PLATFORM=${1:-auto}
case "$PLATFORM" in auto|generic|raspberry-pi) ;; *) echo "Invalid platform" >&2; exit 2;; esac
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
  sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi-ui.service /etc/systemd/system/thinpi-ui.service
  sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi-maintenance@.service /etc/systemd/system/thinpi-maintenance@.service
  sudo install -m 0644 /tmp/thinpi/deploy-client/thinpi.target /etc/systemd/system/thinpi.target
  sudo install -d -o root -g root -m 0755 /usr/local/libexec /etc/X11/xorg.conf.d /etc/ssh/sshd_config.d
  sudo install -m 0755 /tmp/thinpi/deploy-client/xinitrc /usr/local/libexec/thinpi-xinitrc
  sudo install -m 0755 /tmp/thinpi/deploy-client/maintenance-session.sh /usr/local/libexec/thinpi-maintenance-session
  sudo install -m 0755 /tmp/thinpi/deploy-client/browser-policy.sh /usr/local/libexec/thinpi-browser-policy
  sudo install -m 0644 /tmp/thinpi/deploy-client/hardening/Xwrapper.config /etc/X11/Xwrapper.config
  sudo install -m 0644 /tmp/thinpi/deploy-client/hardening/10-thinpi-kiosk.conf /etc/X11/xorg.conf.d/10-thinpi-kiosk.conf
  sudo install -m 0644 /tmp/thinpi/deploy-client/hardening/99-thinpi-ssh.conf /etc/ssh/sshd_config.d/99-thinpi.conf
  sudo systemctl daemon-reload
  sudo sshd -t
  command -v certutil >/dev/null 2>&1 || {
    sudo apt-get update
    sudo apt-get install -y libnss3-tools
  }
  # /etc/thinpi is intentionally inaccessible to the SSH deployment user.
  # Read the controller URL through sudo; do not hide a failed read behind a
  # pipeline whose final command exits successfully.
  CONTROLLER_URL=$(sudo sed -n 's/^THINPI_API_URL=//p' /etc/thinpi/ui.env)
  [ -z "$CONTROLLER_URL" ] || sudo /usr/local/libexec/thinpi-browser-policy "$CONTROLLER_URL"
  sudo pkill -u thinpi -x chrome 2>/dev/null || true
  sudo pkill -u thinpi -x chromium 2>/dev/null || true
  sudo systemctl restart thinpi-agent thinpi-ui
  sudo systemctl --no-pager --full status thinpi-agent thinpi-ui
else
  echo "Binaries staged in /usr/local/bin. Run /tmp/thinpi/deploy-client/provision.sh next."
fi
