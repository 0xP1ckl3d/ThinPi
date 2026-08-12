# Client deployment: Lubuntu, Ubuntu, Debian, mini PC, VM, or Raspberry Pi

This runbook turns a clean Debian machine into a dedicated ThinPi appliance
after the production controller is healthy. The result boots directly into the
ThinPi login screen; ordinary users never receive a Linux desktop or local OS
login.

Provisioning is intentionally invasive. It changes the default systemd target,
masks local getties, disables display managers and SSH passwords, and installs
the controller-authorised maintenance console. Use a dedicated or recoverable
machine and prove SSH public-key access before continuing.

## 1. Choose supported hardware and OS

| Client | Install | Minimum practical allocation | Platform value |
|---|---|---:|---|
| x86-64 VM | Lubuntu 26.04 LTS amd64 | 2 vCPU, 2 GiB RAM, 16 GiB disk, virtual GPU | `generic` |
| x86-64 VM/mini PC | Ubuntu or Lubuntu 24.04/26.04 LTS amd64 | 2 cores, 2 GiB RAM, 16 GiB storage | `generic` |
| x86-64 VM | Debian 13 amd64 netinst, no desktop | 2 vCPU, 2 GiB RAM, 16 GiB disk, virtual GPU | `generic` |
| Intel/AMD mini PC | Debian 13 amd64 netinst, no desktop | 2 cores, 2 GiB RAM, 16 GiB storage | `generic` |
| ARM64 VM/device | Debian 13 arm64, no desktop | 2 cores, 2 GiB RAM, 16 GiB storage | `generic` |
| Raspberry Pi 4/5 | Raspberry Pi OS Lite 64-bit, Trixie | 2 GiB RAM, 16 GiB storage | `raspberry-pi` |

Supported client operating systems are Debian 13/Trixie on `amd64` or `arm64`,
Raspberry Pi OS based on Debian 13 on `arm64`, and Ubuntu/Lubuntu 24.04 or
26.04 LTS on `amd64`. Other releases are refused before the system is changed.

On Ubuntu/Lubuntu, the installer does not use Ubuntu's transitional Chromium
Snap. It installs native Google Chrome from Google's signed Linux repository,
sets mandatory machine policy under `/etc/opt/chrome/policies/managed`, and
gives the launcher the fixed executable path. This preserves the locked admin
browser when the kiosk runs under the non-login `thinpi` identity.

For a VM, configure a visible virtual display adapter such as VirtIO-GPU, QXL,
or the hypervisor default. Do not create a headless server with no display
device. Disable hypervisor clipboard sharing, drag-and-drop, shared folders and
automatic USB redirection. Hypervisor console access is administrator access.

## 2. Install or prepare the OS

During the Debian installer:

1. create a non-root administrator, for example `thinpiadmin`;
2. select **SSH server** and **standard system utilities** only;
3. do not select GNOME, KDE, Xfce or another desktop;
4. use wired networking where possible;
5. set the correct timezone and enable time synchronisation;
6. give the client a DHCP reservation or stable address.

Install the workstation's public SSH key, then prove a new key-only session
works:

```sh
ssh-copy-id thinpiadmin@thinpi-canary
ssh thinpiadmin@thinpi-canary
test -s "$HOME/.ssh/authorized_keys" && echo 'SSH recovery key present'
sudo true
```

Do not provision until this succeeds. Provisioning disables SSH password login.

For an existing Lubuntu 26.04 VM, keep the administrator account created by the
installer and enable SSH now:

```sh
sudo apt update
sudo apt install -y openssh-server
sudo systemctl enable --now ssh
ip -br address
```

From the machine that will run `scripts/deploy-client.sh`, install and prove
the key exactly as shown above. A successful provision disables SDDM and boots
`thinpi.target`; it does not depend on Lubuntu desktop auto-login. The desktop
packages remain available only after an administrator deliberately isolates
`graphical.target` from SSH or authorised local maintenance.

## 3. Verify the platform

Run on the client:

```sh
uname -m
dpkg --print-architecture
grep -E '^(ID|ID_LIKE|PRETTY_NAME|VERSION_ID|VERSION_CODENAME)=' /etc/os-release
timedatectl status
```

Required results are a supported architecture, a synchronised clock, and one
of: Debian 13/Trixie, Ubuntu/Lubuntu 24.04 LTS, or Ubuntu/Lubuntu 26.04 LTS.
Ubuntu/Lubuntu clients must be `amd64`; use Debian 13 for generic arm64.

## 4. Install build prerequisites

The simple deployment command builds natively on the client:

```sh
sudo apt update
sudo apt full-upgrade -y
sudo apt install -y git rsync curl ca-certificates build-essential \
  cmake ninja-build pkg-config qt6-base-dev qt6-declarative-dev
```

On Lubuntu 26.04, install its supported Go toolchain too:

```sh
sudo apt install -y golang-go
```

ThinPi requires Go 1.25 or newer. Check every tool:

```sh
go version
qmake6 -query QT_VERSION
cmake --version
ninja --version
```

If Go is missing or older than 1.25, install the correct `linux-amd64` or
`linux-arm64` archive using the official [Go installation
instructions](https://go.dev/doc/install), then make `/usr/local/go/bin`
available in `PATH`. Qt must be 6.4 or newer. Lubuntu 26.04 currently supplies
Go 1.26 and Qt 6.10, so no external Go archive is needed there.

## 5. Prove network and TLS before kiosk lock-down

Copy the public controller CA when using a private CA:

```sh
scp thinpi-ca.crt thinpiadmin@thinpi-canary:/tmp/thinpi-ca.crt
```

On the client, test the exact production URL and at least one target:

```sh
curl --fail --show-error --cacert /tmp/thinpi-ca.crt \
  https://10.20.0.10:8443/healthz
sudo apt install -y netcat-openbsd
nc -vz 10.30.0.25 3389
```

Replace the addresses and port. The controller does not relay native sessions;
the client needs a direct route or the routed VPN described in
[networking.md](networking.md).

## 6. Build and stage the client

Run from the repository root on Linux, macOS or WSL, not on the client:

```sh
sh scripts/deploy-client.sh thinpiadmin@thinpi-canary
```

The script detects `amd64` or `arm64`, builds the matching Go agent and Qt
launcher on the target, and stages them under `/usr/local/bin`. Use the explicit
Pi compatibility command only for a Pi:

```sh
sh scripts/deploy-pi.sh piadmin@thinpi-living-room
```

The first deployment ends with:

```text
Binaries staged in /usr/local/bin. Run /tmp/thinpi/deploy-client/provision.sh next.
```

## 7. Create a unique enrolment token

On the controller:

```sh
cd /opt/thinpi
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml exec controller \
  /usr/bin/thinpi-controller create-enrolment-token \
  --name 'Canary VM' --ttl 30m
```

Use a different token and device ID for every client. Never clone an enrolled
VM; its root-only device credential would also be cloned.

## 8. Provision the appliance

Run on a VM, mini PC, Lubuntu/Ubuntu machine, or other generic client:

```sh
sudo sh /tmp/thinpi/deploy-client/provision.sh \
  --platform generic \
  --server https://10.20.0.10:8443 \
  --device-id vm-canary-01 \
  --name 'Canary VM' \
  --ca-certificate /tmp/thinpi-ca.crt
```

On a Raspberry Pi, use `--platform raspberry-pi`, or omit `--platform` and let
hardware detection choose. Paste the one-time token at the hidden prompt.

The provisioner installs Xorg/Matchbox, Qt runtime, a native managed browser,
FreeRDP, TigerVNC, locked OpenSSH, the non-login `thinpi` identity, the systemd
kiosk, browser/SSH/Xorg policy, and local maintenance. It enrols the unique
device, disables SDDM/LightDM/GDM, and changes the default boot target.

Moonlight is installed automatically on supported ARM packages. On generic
amd64 it remains unavailable unless an administrator has already installed an
official `moonlight-qt` or `moonlight` executable. Use `--moonlight yes` to
require installation and fail closed, or `--moonlight no` when that client will
not use Moonlight. Other protocols are unaffected.

Do not reboot if provisioning reports an error. A successful run ends with:

```text
Provisioning complete for generic/amd64 on ubuntu 26.04. Reboot to start the ThinPi kiosk.
```

## 9. Reboot and verify

```sh
sudo reboot
```

The physical or virtual display must show the ThinPi login without a Linux
desktop or login prompt. From the administrator workstation:

```sh
ssh thinpiadmin@thinpi-canary
systemctl get-default
systemctl is-active thinpi-agent thinpi-ui
getent passwd thinpi
sudo /usr/bin/thinpi-agent status --config /etc/thinpi/agent.json
sudo journalctl -b -u thinpi-agent -u thinpi-ui --no-pager
```

Required results: `thinpi.target`, both services `active`, the `thinpi` shell
set to `/usr/sbin/nologin`, the correct device ID, and expected native-client
detection.

## 10. Test the appliance boundary

Before assigning the device to a user, verify:

- power-on always returns to the ThinPi login;
- normal users cannot switch virtual terminals or open maintenance;
- closing RDP, VNC or locked SSH returns to the launcher;
- remote SSH cannot open a local shell, second tab or forwarding channel;
- the managed browser cannot leave the controller origin, download files or open developer tools;
- only a controller administrator can request the one-use local maintenance console;
- leaving maintenance with `exit` returns to the signed-out launcher;
- the hypervisor does not inject shared clipboard, folders or host shortcuts.

A VM validates application and amd64 upgrades. It does not validate Pi ARM64
packages, HDMI/audio, VideoCore decoding, physical keyboard firmware or Pi
thermal behaviour. Promote releases through an amd64 canary and then a staging
Pi before the production Pi.

## 11. Updates and matched packages

The easiest source update rebuilds on the client and restarts both services:

```sh
sh scripts/deploy-client.sh thinpiadmin@thinpi-canary
```

For versioned release packages, build on the matching architecture:

```sh
VERSION=0.2.0 sh scripts/build-client.sh amd64
sh scripts/package-debs.sh 0.2.0 amd64
```

Install exactly one matching agent and launcher package:

```sh
sudo sh /tmp/thinpi/deploy-client/update.sh \
  /tmp/thinpi-agent_0.2.0_amd64.deb \
  /tmp/thinpi-launcher_0.2.0_amd64.deb
```

The updater refuses the wrong architecture, unexpected package names,
duplicates, or mismatched agent/launcher versions. Keep the previous matched
pair for rollback. There is no controller-driven unattended fleet updater in
this release.

## 12. Recovery

Use the administrator SSH key, or the controller-authorised local maintenance
action. To temporarily leave kiosk mode through SSH:

```sh
sudo systemctl isolate multi-user.target
```

Return to the appliance:

```sh
sudo systemctl isolate thinpi.target
```

If the display stack fails, diagnose it over SSH with `journalctl`; do not
enable an ordinary getty as a kiosk workaround.
