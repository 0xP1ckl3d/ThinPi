# Raspberry Pi deployment reference

> Do not start here for a first deployment. Follow
> [Deploy ThinPi from zero](deployment.md). That end-to-end guide explains the
> controller, address, certificate, DNS choice and enrolment token before it
> sends you to the Pi.

This Pi-specific supplement converts a dedicated Raspberry Pi into the ThinPi
kiosk. Shared build, provision, hardening and update code lives under
`deploy/client`; see [client-deployment.md](client-deployment.md) for VMs, mini
PCs and generic ARM64 devices. Complete the
[controller runbook](controller-deployment.md) first.

Provisioning is intentionally invasive. It installs native clients, creates a
non-login kiosk user, disables password SSH and ordinary virtual-console
switching, disables desktop display managers, and changes the default systemd
target. It also installs the controller-ticketed administrator maintenance
console. Use a recoverable SD card and prove SSH public-key access first.

The values below are examples only. ThinPi does not create these names or make
them resolve. The start-to-finish guide recommends an IP URL for the first
deployment so DNS is optional.

| Setting | Example |
|---|---|
| Pi hostname | `thinpi-living-room` |
| Pi administrator | `piadmin` |
| Pi device ID | `pi-living-room` |
| Pi display name | `Living room` |
| Controller URL | `https://thinpi.home.example:8443` |
| Private CA file | `thinpi-ca.crt` |

Replace these with the values in [deployment.md](deployment.md).

## 1. Hardware and workstation requirements

Use:

- Raspberry Pi 4 or 5;
- reliable power supply and cooling;
- Raspberry Pi OS Lite **64-bit**, Debian Trixie or later;
- wired Ethernet where practical, especially for Moonlight;
- display, keyboard/mouse, and any gamepad required for acceptance testing;
- 16 GiB or larger high-quality SD card/SSD;
- administrator workstation with this source tree, `ssh`, and `rsync`.

The stock OS must provide Qt 6.5 or newer. The source currently requires Go
1.25 or newer.

Windows users must run `scripts/deploy-pi.sh` from WSL. In WSL, the repository
will normally be under `/mnt/c/...`.

Official references:

- [Raspberry Pi Imager/getting started](https://www.raspberrypi.com/documentation/computers/getting-started.html)
- [Raspberry Pi OS images](https://www.raspberrypi.com/software/operating-systems/)
- [Go Linux installation](https://go.dev/doc/install)
- [Moonlight Qt on Raspberry Pi](https://github.com/moonlight-stream/moonlight-docs/wiki/Installing-Moonlight-Qt-on-Raspberry-Pi-4)

## 2. Flash Raspberry Pi OS correctly

In Raspberry Pi Imager:

1. choose your Pi model;
2. select **Raspberry Pi OS (other)**;
3. select **Raspberry Pi OS Lite (64-bit)** based on Debian Trixie;
4. set hostname `thinpi-living-room`;
5. create administrator `piadmin` with a strong recovery password;
6. set timezone/keyboard/network;
7. enable SSH;
8. select **public-key authentication only** and install your administrator
   public key;
9. write and verify the media.

Do not use the Desktop image. ThinPi installs its own Xorg kiosk environment.

Boot the Pi, wait for networking, then connect from the workstation:

```sh
ssh piadmin@thinpi-living-room
```

If this key-based login does not work, stop. Fix it before provisioning.

## 3. Verify OS, architecture, time, and SSH recovery

Run on the Pi:

```sh
uname -m
dpkg --print-architecture
grep -E '^(PRETTY_NAME|VERSION_CODENAME)=' /etc/os-release
timedatectl status
test -s "$HOME/.ssh/authorized_keys" && echo "SSH key present"
sudo -n true || sudo true
```

Required results:

```text
aarch64
arm64
VERSION_CODENAME=trixie
SSH key present
```

`timedatectl` must show synchronized time. TLS and 30-second launch tickets fail
when clocks are wrong.

## 4. Prove controller and target reachability before lock-down

On the Pi, test the exact controller name:

```sh
getent hosts thinpi.home.example
curl --fail --show-error https://thinpi.home.example:8443/healthz
```

For a private CA, copy its **public certificate** and test with it:

```sh
scp thinpi-ca.crt piadmin@thinpi-living-room:/tmp/thinpi-ca.crt
ssh piadmin@thinpi-living-room
curl --fail --show-error --cacert /tmp/thinpi-ca.crt \
  https://thinpi.home.example:8443/healthz
```

Expected response:

```json
{"status":"ok","version":"0.1.0"}
```

Test at least one real target from the Pi:

```sh
sudo apt install -y netcat-openbsd
ip route get 10.30.0.25
nc -vz 10.30.0.25 3389
```

Substitute the actual target and port. If the target is reachable only from
the controller site, configure the external subnet router described in
[networking.md](networking.md) first. Controller reachability alone is not
enough.

## 5. Install build prerequisites on the Pi

The first deployment builds native ARM64 binaries on the Pi, avoiding an
untested cross-compilation setup.

```sh
sudo apt update
sudo apt full-upgrade -y
sudo apt install -y git rsync curl ca-certificates build-essential golang \
  cmake ninja-build pkg-config qt6-base-dev qt6-declarative-dev
sudo reboot
```

Reconnect and verify:

```sh
go version
cmake --version
ninja --version
qmake6 -query QT_VERSION
```

Required:

- Go 1.25 or newer;
- CMake 3.21 or newer;
- Qt 6.5 or newer.

If `go version` is older than 1.25, install the current official Linux ARM64
archive from [go.dev/dl](https://go.dev/dl/) according to the official Go
installation instructions. After installation:

```sh
/usr/local/go/bin/go version
echo 'export PATH=/usr/local/go/bin:$PATH' >> "$HOME/.profile"
. "$HOME/.profile"
go version
```

Do not continue until all version checks pass.

## 6. Build and stage ThinPi from the workstation

From the repository root on Linux, macOS, or WSL:

```sh
scripts/deploy-pi.sh piadmin@thinpi-living-room
```

The script:

1. copies `agent/`, `launcher/`, and `deploy/client/` to `/tmp/thinpi`;
2. builds the Go agent for the Pi;
3. builds the Qt launcher against the Pi's installed Qt;
4. stages both binaries under `/usr/local/bin`;
5. on later deployments, updates `/usr/bin`, unit files, CA trust, and restarts
   the services.

For the first deployment, successful output ends with:

```text
Binaries staged in /usr/local/bin. Run /tmp/thinpi/deploy-client/provision.sh next.
```

Verify on the Pi:

```sh
ls -l /usr/local/bin/thinpi-agent /usr/local/bin/thinpi-launcher
/usr/local/bin/thinpi-agent version
```

If building fails, fix the reported Go/Qt/CMake error. Do not substitute the
Windows x86-64 launcher executable.

## 7. Create one enrolment token for this Pi

Use either the production admin console **Devices → Create enrolment token** or
run this on the controller host:

```sh
cd /opt/thinpi
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml exec controller \
  /usr/bin/thinpi-controller create-enrolment-token \
  --name "Living room Pi" --ttl 30m
```

The token is shown once. Do not put it in shell history, source control, chat,
or a permanent config file. The provisioner prompts for it without echo.

## 8. Run the provisioner

If using a private CA, ensure `/tmp/thinpi-ca.crt` exists on the Pi. Then SSH to
the Pi and run:

```sh
sudo sh /tmp/thinpi/deploy-client/provision.sh \
  --platform raspberry-pi \
  --server https://thinpi.home.example:8443 \
  --device-id pi-living-room \
  --name 'Living room' \
  --ca-certificate /tmp/thinpi-ca.crt
```

For a publicly trusted certificate, omit the final `--ca-certificate` line.

At this prompt:

```text
One-time enrolment token:
```

paste the token and press Enter. Nothing is displayed while typing. The token
is passed to the agent on stdin, exchanged once for the device credential, and
not stored in process arguments.

Provisioning can take several minutes because it installs Xorg, Qt runtime
libraries, Chromium, FreeRDP, TigerVNC, PulseAudio, and Moonlight Qt from its
official repository.

It then:

- creates the non-login `thinpi` kiosk identity;
- installs binaries into `/usr/bin`;
- writes `/etc/thinpi/agent.json` and `/etc/thinpi/ui.env`;
- installs a private CA into both agent and system trust stores;
- writes `/etc/thinpi/device.json` mode `0600`;
- installs hardened agent/UI systemd services;
- installs the locked xterm/OpenSSH client and pinned-host-key enforcement;
- masks ordinary local getties and blocks kiosk VT escape shortcuts;
- installs the administrator-only, one-use-ticket maintenance console;
- restricts Chromium to the controller origin with managed policy;
- disables desktop display managers;
- disables SSH password, root, forwarding, and X11 access;
- selects `thinpi.target` as the default boot target.

Successful output ends with:

```text
Provisioning complete for raspberry-pi/arm64. Reboot to start the ThinPi kiosk.
```

If provisioning fails, do not reboot. Fix the reported error while the normal
administrator environment is still active.

## 9. Configure audio and Moonlight before final acceptance

RDP/VNC do not require Moonlight pairing. If using Moonlight, first configure
the Raspberry Pi OS Lite audio output:

```sh
sudo raspi-config
```

Select the PulseAudio/audio output options appropriate to the attached display,
then reboot if requested.

Moonlight pairing is stored per Linux identity. ThinPi launches Moonlight as
the `thinpi` user, so pairing performed only as `piadmin` is not sufficient.
After the first ThinPi boot, pair using the `thinpi` home and running display:

```sh
sudo -u thinpi env \
  HOME=/home/thinpi USER=thinpi LOGNAME=thinpi \
  DISPLAY=:0 XAUTHORITY=/home/thinpi/.Xauthority \
  moonlight-qt pair sunshine-host.home.example
```

Complete the displayed PIN on the Sunshine host. Then verify the pairing as the
same identity before assigning the production connection.

## 10. Reboot into ThinPi

```sh
sudo reboot
```

Expected screen: the full-screen ThinPi login, with no desktop, taskbar, or
terminal.

Reconnect over SSH and run:

```sh
systemctl is-active thinpi-agent thinpi-ui
systemctl get-default
sudo /usr/bin/thinpi-agent status --config /etc/thinpi/agent.json
sudo ls -l /run/thinpi/agent.sock /etc/thinpi/device.json
sudo journalctl -b -u thinpi-agent -u thinpi-ui --no-pager
```

Expected:

- both services print `active`;
- default target is `thinpi.target`;
- agent reports the correct device ID and detected clients;
- device file is root-owned mode `0600`;
- local socket is accessible to group `thinpi`;
- controller Devices screen reports a recent heartbeat.

## 11. Configure and test through the admin console

Use this order:

1. Credentials;
2. Connections;
3. People/Groups;
4. Access rules with the correct credential override;
5. Restrictions;
6. Devices heartbeat verification.

Then test:

- restricted user sees only assigned connections;
- stored password is not requested or displayed in the launcher;
- disabled user is denied;
- real RDP/VNC/SSH/Moonlight failure messages return to the dashboard;
- closing a native client returns to the launcher;
- administrator launcher login shows **Administration** and opens Chromium;
- session time limit ends the client and reports the reason.

## 12. Production configuration files

`/etc/thinpi/agent.json`:

| Field | Purpose |
|---|---|
| `controller_url` | Exact production HTTPS origin |
| `device_file` | Root-only device ID/bearer credential |
| `socket` | Local launcher/agent Unix socket |
| `ca_certificate` | Optional private controller CA |
| `freerdp_binary` | `auto` or explicit FreeRDP executable |
| `vnc_binary` | `auto` or explicit TigerVNC executable |
| `moonlight_binary` | `auto` or explicit Moonlight executable |
| `ssh_binary` | `auto` or explicit OpenSSH client executable |
| `terminal_binary` | `auto` or explicit `xterm` executable |
| `sshpass_binary` | `auto` or explicit SSH password helper |
| `maintenance_user` | Existing non-root Pi administrator opened by a valid maintenance ticket |

`/etc/thinpi/ui.env` contains `THINPI_API_URL` and may contain
`THINPI_ADMIN_BROWSER` if Chromium has a non-standard executable name.

Never add `mock_clients`, `THINPI_DEV_MODE`, a user token, a remote password,
or the device token to the launcher environment.

## 13. Updating a locked Pi

Prefer updating from the separate administrator workstation over its SSH key:

```sh
scripts/deploy-pi.sh piadmin@thinpi-living-room
```

On an already provisioned Pi, the script builds both components, replaces the
installed pair, refreshes units and CA trust, and restarts the services. This
interrupts an active session, so schedule it outside use hours.

There is currently no controller-driven fleet updater or unattended agent
self-update feature.

For local work, sign into the launcher as a controller Administrator and choose
**Local maintenance**. After a one-use device-bound ticket is redeemed, the
launcher signs out and switches to the configured Pi administrator console.
Run `exit` to close it and return to the locked launcher. A normal ThinPi user
cannot switch console or request this path.

For versioned `.deb` packages:

```sh
sudo sh /tmp/thinpi/deploy-client/update.sh \
  /tmp/thinpi-agent_0.2.0_arm64.deb \
  /tmp/thinpi-launcher_0.2.0_arm64.deb
```

Keep the previous two matched agent/launcher versions for rollback. Update the
controller first when release notes mention an API change.

## 14. Recovery

Follow logs remotely from the workstation:

```sh
scripts/pi-logs.sh piadmin@thinpi-living-room
```

Temporarily stop the kiosk and return to multi-user mode:

```sh
ssh piadmin@thinpi-living-room
sudo systemctl isolate multi-user.target
```

Return to ThinPi:

```sh
sudo systemctl isolate thinpi.target
```

If a device was revoked, create a new one-time token and re-enrol it. Preserve
the old root-only device file until replacement enrolment and heartbeat have
succeeded.

If SSH is unavailable, recover through the SD card/offline OS rather than
trying to escape the kiosk as an ordinary user.

## 15. Hardware acceptance is still required

Before calling the appliance production-ready, complete and record the tests in
[hardware-validation.md](hardware-validation.md): repeated cold boots, display
mode, real RDP/VNC, Moonlight decode/audio/gamepad, power interruption, thermal
behaviour, and secret inspection. These require your physical Pi and targets.
