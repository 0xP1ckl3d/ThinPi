# Deploy ThinPi from zero: complete operator guide

This is the **start here** production guide. Follow it from top to bottom. It
does not assume that you already understand Docker, DNS, TLS certificates,
systemd, or Linux kiosk setup.

Do not begin with `controller-deployment.md`, `client-deployment.md`, or
`pi-deployment.md`. Those are
reference pages linked from the relevant steps below.

## What you are building

You need three computers:

| Computer | Example used below | What it does |
|---|---|---|
| Your workstation | Windows PC with WSL | Holds this source, prepares certificates, deploys updates |
| Controller | Ubuntu Server VM at `10.20.0.10` | Runs Docker, the web admin panel, database, users and secrets |
| ThinPi client | Debian VM/mini PC or Raspberry Pi | Boots directly into the locked ThinPi login screen |

The client connects to remote desktops directly. The controller authorises the
launch and supplies the assigned credential, but does not carry the desktop
video/audio stream.

## Before touching anything: choose and write down your values

The examples below are not magic names that already exist on your network.
Replace them with your values.

| Meaning | Example in this guide | Your value |
|---|---|---|
| Controller reserved LAN IP | `10.20.0.10` | |
| Controller Linux login | `thinpiops` | |
| Controller URL | `https://10.20.0.10:8443` | |
| Optional friendly controller name | `thinpi.home.arpa` | |
| Client hostname | `thinpi-living-room` | |
| Client Linux administrator | `piadmin` | |
| Client device ID | `pi-living-room` | |
| Client display name | `Living room` | |
| First remote target | `10.30.0.25:3389` | |

### The recommended first deployment does not need DNS

Use the controller's reserved IP in the URL:

```text
https://10.20.0.10:8443
```

The certificate-generation command below puts that IP into the certificate.
The browser and Pi can then validate HTTPS without inventing a hostname or
editing DNS.

You must reserve `10.20.0.10` in your router/DHCP server so another machine
cannot receive it later. Router interfaces differ, but the setting is normally
called **DHCP reservation**, **static lease**, or **address reservation**. It
maps the controller VM's MAC address to `10.20.0.10`.

### Optional: use a friendly name

Only do this if you want `https://thinpi.home.arpa:8443` instead of the IP.
Create a local DNS record on your router, Pi-hole, AdGuard Home, or other DNS
server:

```text
Name:    thinpi.home.arpa
Type:    A
Address: 10.20.0.10
```

Make sure both your workstation and Pi use that DNS server. Test with:

```sh
getent hosts thinpi.home.arpa
```

It must return `10.20.0.10`. `.home.arpa` is the reserved home-network suffix.
Do not copy `.internal`, `.example`, or another sample suffix unless you have
actually configured it. The certificate helper includes both the friendly name
and IP, so either URL can work.

## Phase 1 — create the Ubuntu controller VM

These exact instructions target **Ubuntu Server 24.04 LTS**. A physical Ubuntu
server also works. Debian works, but package-repository commands differ; use
[controller-deployment.md](controller-deployment.md) if you choose Debian.

1. Create a VM with 2 CPU cores, 2 GiB RAM and 16 GiB disk.
2. Install Ubuntu Server 24.04 LTS.
3. Create the user `thinpiops` during installation.
4. Enable OpenSSH when the installer asks.
5. Give the VM the DHCP reservation recorded above.
6. From your workstation, open PowerShell and connect:

   ```powershell
   ssh thinpiops@10.20.0.10
   ```

7. On the controller, confirm its address and clock:

   ```sh
   hostnamectl
   ip -br address
   timedatectl status
   ```

Stop if the address is not the reserved address or the clock is not
synchronised.

## Phase 2 — install Docker on the controller

Every command in this phase runs **inside the controller SSH session**.

```sh
sudo apt update
sudo apt install -y ca-certificates curl git openssl rsync
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
  -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
. /etc/os-release
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu ${UBUNTU_CODENAME:-$VERSION_CODENAME} stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io \
  docker-buildx-plugin docker-compose-plugin
sudo docker run --rm hello-world
sudo docker compose version
```

Checkpoint: `hello-world` must finish successfully and Compose must print a
version. If not, stop and use Docker's official Ubuntu troubleshooting before
continuing.

## Phase 3 — copy ThinPi source to the controller

### If this project is in a Git repository

Run on the controller, replacing the URL:

```sh
sudo install -d -m 0755 /opt/thinpi
sudo chown thinpiops:thinpiops /opt/thinpi
git clone YOUR_GIT_REPOSITORY_URL /opt/thinpi
cd /opt/thinpi
```

### If the files only exist on your Windows workstation

First install WSL Ubuntu if it is not present:

```powershell
wsl --install -d Ubuntu
```

Restart Windows if requested. Open **Ubuntu** from the Start menu. Find the
project under `/mnt/c`; for this workspace it is:

```sh
cd /mnt/c/Users/brad/TOOLS/Projects/ThinPi
```

Prepare the destination on the controller:

```sh
ssh thinpiops@10.20.0.10 \
  'sudo install -d -o thinpiops -g thinpiops -m 0755 /opt/thinpi'
```

Copy only source files:

```sh
rsync -av --delete \
  --exclude .git --exclude .thinpi-dev --exclude build --exclude bin \
  --exclude .docker-config --exclude .gocache --exclude .gomodcache \
  ./ thinpiops@10.20.0.10:/opt/thinpi/
```

Then reconnect and verify:

```sh
ssh thinpiops@10.20.0.10
cd /opt/thinpi
test -f deploy/controller/compose.yml && echo 'ThinPi source found'
```

## Phase 4 — create HTTPS certificates

These steps use a private ThinPi certificate authority. Four files are made:

| File | Where it goes | Secret? |
|---|---|---|
| `tls.crt` | Controller | No |
| `tls.key` | Controller | **Yes** |
| `thinpi-ca.crt` | Each Pi and administrator browser | No |
| `thinpi-ca.key` | Offline backup only | **Yes — never copy to controller or Pi** |

The simplest two-server deployment can generate these files directly on the
controller from `/opt/thinpi`; no third server is required. Running the helper
from WSL on a separate workstation is optional hardening that keeps the CA key
off the controller. The worked [Ubuntu/Lubuntu guide](ubuntu-lubuntu-deployment.md)
shows the direct-on-controller commands.

For the optional separate-workstation method, run:

```sh
sudo apt update
sudo apt install -y openssl
sh scripts/generate-controller-pki.sh \
  thinpi.home.arpa \
  10.20.0.10 \
  thinpi-pki-production
```

Even if you are using only the IP, keep `thinpi.home.arpa` as the certificate's
descriptive DNS name. The IP SAN is what makes the IP URL valid.

Expected final output includes:

```text
Generated thinpi-pki-production
```

Copy the server certificate and key to the controller:

```sh
scp thinpi-pki-production/tls.crt \
  thinpi-pki-production/tls.key \
  thinpiops@10.20.0.10:/tmp/
```

On the controller:

```sh
cd /opt/thinpi
sudo install -d -o root -g root -m 0750 deploy/controller/tls
sudo chown root:65532 deploy/controller/tls
sudo install -o root -g root -m 0640 /tmp/tls.crt \
  deploy/controller/tls/tls.crt
sudo chown root:65532 deploy/controller/tls/tls.crt
sudo install -o root -g root -m 0640 /tmp/tls.key \
  deploy/controller/tls/tls.key
sudo chown root:65532 deploy/controller/tls/tls.key
sudo rm -f /tmp/tls.crt /tmp/tls.key
```

Keep `thinpi-pki-production/thinpi-ca.key` in encrypted offline storage. Keep
`thinpi-ca.crt` available; you will copy that public file to the Pi later.

## Phase 5 — create the controller encryption key and configuration

Run on the controller:

```sh
cd /opt/thinpi
sudo install -d -o root -g root -m 0750 deploy/controller/secrets
sudo chown root:65532 deploy/controller/secrets
sudo sh -c 'umask 0077; openssl rand -base64 32 > deploy/controller/secrets/thinpi_master_key'
sudo chown root:65532 deploy/controller/secrets/thinpi_master_key
sudo chmod 0640 deploy/controller/secrets/thinpi_master_key
cp deploy/controller/.env.example deploy/controller/.env
nano deploy/controller/.env
```

Replace the file contents with your controller IP:

```text
THINPI_VERSION=0.1.0
THINPI_BIND_ADDRESS=10.20.0.10
THINPI_MASTER_KEY_FILE=./secrets/thinpi_master_key
```

Save in Nano with `Ctrl+O`, Enter, then `Ctrl+X`.

Make two protected offline backups now:

- `deploy/controller/secrets/thinpi_master_key`;
- `thinpi-pki-production/thinpi-ca.key` from the workstation.

The database and master key must be restored as a matched pair. A new master
key cannot decrypt old credentials.

## Phase 6 — start and verify the controller

Run on the controller:

```sh
cd /opt/thinpi
sudo docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml up -d --build
sudo docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml ps
```

Wait up to one minute. The `controller` row must say `healthy`.

If it does not:

```sh
sudo docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml logs --tail 200 controller
```

From WSL on the workstation, use the public CA to test the exact production
URL:

```sh
curl --fail --show-error \
  --cacert thinpi-pki-production/thinpi-ca.crt \
  https://10.20.0.10:8443/healthz
```

Expected response:

```json
{"status":"ok","version":"0.1.0"}
```

Never use `curl --insecure`. A failure means the IP, certificate, clock, port,
or firewall is wrong.

## Phase 7 — create the first ThinPi administrator

Run on the controller:

```sh
cd /opt/thinpi
sudo env COMPOSE_ENV_FILE=deploy/controller/.env \
  sh scripts/bootstrap-admin.sh
```

Enter a new password of at least eight characters twice. It does not echo, and
the script creates nothing if the entries differ. Successful output explicitly
reports `Username: admin`; `Administrator` is the display name, not the login
username.

Install the public CA in Windows so Edge and Chrome trust it:

1. Open `thinpi-pki-production` in File Explorer.
2. Double-click `thinpi-ca.crt`.
3. Select **Install Certificate**.
4. Select **Local Machine** and approve the administrator prompt.
5. Select **Place all certificates in the following store**.
6. Browse to **Trusted Root Certification Authorities**.
7. Finish and confirm the security warning only after checking that this is the
   CA you just generated.
8. Fully close and reopen the browser.

Firefox may use its own certificate store. In Firefox, open Settings, search
for **certificates**, select **View Certificates → Authorities → Import**, and
import `thinpi-ca.crt` for website identification.

Then open:

```text
https://10.20.0.10:8443/admin/login
```

Sign in as `admin`. If you do not want to install a private CA in the browser,
use a certificate from a CA already trusted by your organisation instead; do
not click through certificate warnings for production administration.

Checkpoint:

- the page has no certificate warning;
- login succeeds;
- logout returns to login;
- `/` redirects to login when signed out and `/admin` when signed in.

If Ubuntu's firewall is already active, allow only your administrator and Pi
subnets before proceeding. Check first:

```sh
sudo ufw status
```

For example, if both are inside `10.20.0.0/24`:

```sh
sudo ufw allow OpenSSH
sudo ufw allow from 10.20.0.0/24 to any port 8443 proto tcp
```

Do not expose TCP 8443 directly to the public Internet.

## Phase 8 — choose and install the client

ThinPi supports Debian 13/Trixie on amd64 and arm64, Ubuntu/Lubuntu 24.04 or
26.04 LTS on amd64, and Raspberry Pi OS based on Debian 13 on arm64. This
includes VMs, Intel/AMD mini PCs, generic ARM64 devices, and Raspberry Pi 4/5.

- For a VM, mini PC, Lubuntu/Ubuntu client, or generic ARM device, follow
  [client deployment](client-deployment.md) sections 1–9, then return to
  Phase 15 below to configure users and connections.
- For a Raspberry Pi, continue with the Pi-specific phases below. The same
  generic provisioner and security policy are used, plus Pi platform checks
  and Pi Moonlight packages.

### Raspberry Pi path — flash the operating system

On the workstation, open Raspberry Pi Imager.

1. Choose the actual Pi model.
2. Choose **Raspberry Pi OS (other)**.
3. Choose **Raspberry Pi OS Lite (64-bit)** based on Debian Trixie.
4. Set hostname to `thinpi-living-room`.
5. Set username to `piadmin`.
6. Set a strong password. It is needed for `sudo` in local maintenance.
7. Set locale, keyboard, timezone and Wi-Fi if Ethernet is unavailable.
8. Enable SSH.
9. Select password or public-key SSH authentication according to your preference.
10. Write and verify the SD card/SSD.

Boot the Pi and wait two minutes. From WSL:

```sh
ssh piadmin@thinpi-living-room
```

If that name does not resolve, find the Pi's address in your router and use:

```sh
ssh piadmin@PI_ADDRESS
```

Do not provision until a fresh administrator SSH login works. Provisioning
preserves the host's existing password-authentication setting unless you
explicitly pass `--disable-ssh-passwords`.

## Phase 9 — verify and prepare the Raspberry Pi

Run inside the Pi SSH session:

```sh
uname -m
dpkg --print-architecture
grep -E '^(PRETTY_NAME|VERSION_CODENAME)=' /etc/os-release
timedatectl status
sudo true
```

Required results include `aarch64`, `arm64`, `VERSION_CODENAME=trixie`, a
synchronised clock, and a working administrator SSH login.

Install build tools:

```sh
sudo apt update
sudo apt full-upgrade -y
sudo apt install -y git rsync curl ca-certificates build-essential \
  cmake ninja-build pkg-config qt6-base-dev qt6-declarative-dev golang
sudo reboot
```

Reconnect and check:

```sh
go version
qmake6 -query QT_VERSION
cmake --version
ninja --version
```

ThinPi requires Go 1.25 or newer and Qt 6.5 or newer. If the packaged Go is
older, follow the official Go ARM64 installation linked from
[pi-deployment.md](pi-deployment.md), then rerun `go version`.

## Phase 10 — prove network reachability before kiosk lock-down

Copy the public controller CA from WSL to the Pi:

```sh
scp thinpi-pki-production/thinpi-ca.crt \
  piadmin@thinpi-living-room:/tmp/thinpi-ca.crt
```

On the Pi:

```sh
curl --fail --show-error --cacert /tmp/thinpi-ca.crt \
  https://10.20.0.10:8443/healthz
sudo apt install -y netcat-openbsd
nc -vz 10.30.0.25 3389
```

Replace `10.30.0.25` and `3389` with a real target and port. Common ports:

| Connection | Default port |
|---|---:|
| SSH | TCP 22 |
| RDP | TCP 3389 |
| VNC | TCP 5900 |
| Moonlight/Sunshine | several TCP/UDP ports; see `networking.md` |

If the controller health check works but the target check fails, the Pi has no
route/firewall access to that target. Fix that now. If the target exists behind
another site, follow [networking.md](networking.md); the controller container is
not a network proxy.

## Phase 11 — build and stage the Raspberry Pi software

Run from the project root in WSL on the workstation:

```sh
sh scripts/deploy-pi.sh piadmin@thinpi-living-room
```

The first run should end with:

```text
Binaries staged in /usr/local/bin. Run /tmp/thinpi/deploy-client/provision.sh next.
```

If it reports a missing Go, Qt, CMake, Ninja, compiler, or wrong CPU
architecture, correct that exact prerequisite and rerun the command.

## Phase 12 — create a one-time client enrolment token

Run on the controller:

```sh
cd /opt/thinpi
sudo docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml exec controller \
  /usr/bin/thinpi-controller create-enrolment-token \
  --name 'Living room Pi' --ttl 30m
```

Copy the displayed token temporarily. It works once and expires after 30
minutes. Do not save it in Git, notes, shell scripts, or chat.

## Phase 13 — provision the Raspberry Pi as a locked appliance

SSH to the Pi and run:

```sh
sudo sh /tmp/thinpi/deploy-client/provision.sh \
  --platform raspberry-pi \
  --server https://10.20.0.10:8443 \
  --device-id pi-living-room \
  --name 'Living room' \
  --ca-certificate /tmp/thinpi-ca.crt
```

Paste the one-time token when prompted. Nothing appears while pasting.

The provisioner will:

- create `thinpi`, a dedicated system account with `/usr/sbin/nologin` and no
  sudo rights;
- install the full-screen launcher on virtual terminal 7;
- start it automatically at every boot without exposing an OS login;
- install the root agent and its small fixed local API;
- install RDP, VNC, Moonlight and the locked single-purpose SSH terminal;
- disable ordinary virtual-terminal switching from the kiosk;
- mask local getty login screens;
- disable root/forwarding/X11 access to the Pi's SSH server while preserving
  its existing password-authentication setting;
- restrict the managed admin browser to the controller origin;
- configure `piadmin` as the one allowed maintenance console account;
- set `thinpi.target` as the default boot target.

Expected final line:

```text
Provisioning complete for raspberry-pi/arm64. Reboot to start the ThinPi kiosk.
```

Do not reboot if provisioning reports an error.

## Phase 14 — reboot and prove automatic Raspberry Pi kiosk startup

On the Pi:

```sh
sudo reboot
```

The display must show the ThinPi login screen automatically. There is no
desktop login and no passwordless Linux login. Systemd starts the launcher as
the locked `thinpi` identity; the login shown on screen is your custom ThinPi
controller authentication.

Reconnect over SSH from the workstation and verify:

```sh
systemctl get-default
systemctl is-active thinpi-agent thinpi-ui
getent passwd thinpi
sudo /usr/bin/thinpi-agent status --config /etc/thinpi/agent.json
sudo systemctl cat thinpi-ui thinpi-agent
```

Required:

- default target: `thinpi.target`;
- both services: `active`;
- `thinpi` shell: `/usr/sbin/nologin`;
- status reports RDP, VNC, Moonlight and SSH client detection.

## Phase 15 — configure users and connections

Open the controller admin panel. Work in this exact order:

1. **Credentials** — create the remote username and password by a descriptive
   label. Users never see the secret.
2. **Connections** — create RDP, VNC, SSH, or Moonlight target and optionally
   choose a default credential.
3. **People** — create each ThinPi login. Mark only trusted operators as
   Administrator.
4. **Groups** — optional shared roles.
5. **Access rules** — assign a connection to a person/group and select its
   credential override.
6. **Restrictions** — set days, hours, daily limit, session limit and timezone.
7. **Devices** — confirm the Pi has a recent heartbeat.

### Creating a locked SSH connection

ThinPi SSH intentionally gives the user a shell on the **remote SSH server**.
It never opens a shell on the Pi.

On the remote SSH server, obtain its Ed25519 public host key through a trusted
administrator session or console:

```sh
sudo cat /etc/ssh/ssh_host_ed25519_key.pub
sudo ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

Verify the fingerprint out of band. Copy only the first two fields, for
example:

```text
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...
```

In ThinPi:

1. create a credential containing the remote SSH username and password;
2. create a **Linux command line (locked SSH)** connection;
3. set host and port;
4. replace the example key in Advanced settings:

   ```json
   {
     "host_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...",
     "terminal_title": "School Linux server"
   }
   ```

5. assign it to the intended person with that credential.

ThinPi refuses an SSH connection without a pinned host key. It also disables
local commands, escape commands, agent/X11/TCP forwarding, user SSH config,
host-key prompts, tabs, toolbars, terminal logging and window-control escape
sequences. Closing or exiting SSH destroys the terminal and returns to ThinPi.

## Phase 16 — use administrator-only local maintenance

There are two supported maintenance paths.

### Recommended: maintain from another computer over SSH

From WSL/workstation:

```sh
ssh piadmin@thinpi-living-room
sudo apt update
sudo apt full-upgrade
exit
```

Deploy a new ThinPi build from the project root:

```sh
sh scripts/deploy-pi.sh piadmin@thinpi-living-room
```

### On the Pi display: controller administrator maintenance console

1. Sign into the ThinPi launcher with a user marked Administrator.
2. Select **Local maintenance**.
3. Read and accept the confirmation.
4. The controller issues a 30-second one-use ticket bound to this Pi.
5. The root agent validates the ticket and switches to console 2.
6. ThinPi signs the launcher out before the switch.
7. The console opens as the configured `piadmin` OS account.
8. Use `sudo` when an OS update requires root.
9. Type `exit` when finished.
10. Console 2 closes and the locked ThinPi login returns on console 7.

No local command text is sent through the agent API. The only privileged local
action implemented is “redeem this maintenance ticket and open the fixed
maintenance console for the configured account.”

## Phase 17 — kiosk escape and acceptance tests

Test these physically before deployment sign-off:

- boot and power-cycle always return to the ThinPi login;
- `Ctrl+Alt+F1` through `F6` does not leave the kiosk for a normal user;
- `Ctrl+Alt+Backspace`, window close shortcuts and launcher crashes do not
  expose a desktop or shell;
- normal users do not see Administration or Local maintenance;
- an administrator maintenance ticket cannot be reused;
- leaving maintenance with `exit` returns to the signed-out launcher;
- remote SSH exits back to the launcher and cannot open a second/local tab;
- the SSH remote host-key mismatch fails closed;
- the managed admin browser cannot browse away from the controller, download files,
  open developer tools, use guest/incognito mode, or access `file://` paths;
- an expired launcher/controller session returns to login;
- the `thinpi` account has `nologin`, no sudo membership and no writable system
  paths;
- RDP drive, clipboard, printer and smart-card sharing are disabled unless an
  administrator explicitly enabled the relevant structured setting;
- VNC clipboard is disabled unless explicitly enabled;
- disconnecting every native client returns to ThinPi.

For every platform, complete the appliance-boundary checklist in
[client-deployment.md](client-deployment.md#10-test-the-appliance-boundary).
For Raspberry Pi, additionally record the hardware-specific results in
[hardware-validation.md](hardware-validation.md).

## Phase 18 — backup before calling it deployed

Back up all of the following to protected storage:

1. controller SQLite volume/data backup;
2. `deploy/controller/secrets/thinpi_master_key`;
3. `thinpi-ca.key` and `thinpi-ca.crt`;
4. controller `tls.key` and `tls.crt`;
5. Pi administrator SSH private key;
6. this completed values worksheet.

Create a consistent database backup on the controller. This pauses the web
controller briefly:

```sh
cd /opt/thinpi
mkdir -p backups
backup="thinpi-$(date -u +%Y%m%dT%H%M%SZ).db"
sudo docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml stop controller
sudo docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml run --rm --no-deps \
  --entrypoint sh -v "$PWD/backups:/backup" initialise-data \
  -c "cp /var/lib/thinpi/thinpi.db /backup/$backup"
sudo docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml start controller
ls -lh "backups/$backup"
```

Copy the resulting database and matching master key off the controller. Test a
restore using [Controller backup and restore](controller-deployment.md#12-back-up-before-enrolling-a-pi) before discarding the original system.

Never commit these files. Run the repository check before pushing:

```sh
sh scripts/repo-audit.sh
```

## Recovery commands

If the kiosk is broken but SSH works:

```sh
ssh piadmin@thinpi-living-room
sudo systemctl status thinpi-agent thinpi-ui --no-pager --full
sudo journalctl -b -u thinpi-agent -u thinpi-ui --no-pager
sudo systemctl restart thinpi-agent thinpi-ui
```

Temporarily boot ordinary multi-user mode remotely:

```sh
sudo systemctl isolate multi-user.target
```

Return to the appliance:

```sh
sudo systemctl isolate thinpi.target
```

If SSH and the administrator maintenance path both fail, power down and mount
the SD card on another computer or re-image it. There is intentionally no
normal-user kiosk escape.

For symptom-specific help, use [troubleshooting.md](troubleshooting.md).
