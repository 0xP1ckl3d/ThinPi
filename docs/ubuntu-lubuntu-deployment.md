# Worked install: Ubuntu controller at 10.10.10.60 and Lubuntu 26.04 client

This is the shortest complete production runbook for the exact two-VM layout
used for release testing. It creates a real controller—no seeded demo users,
mock sessions, or development password—and converts one Lubuntu VM into a
locked ThinPi appliance.

Read the replacement values once before copying commands:

| Text in commands | Replace with |
|---|---|
| `<CONTROLLER_USER>` | Your Ubuntu Server SSH username |
| `<CLIENT_IP>` | The Lubuntu VM address shown by `ip -br address` |
| `<CLIENT_USER>` | Your Lubuntu administrator username |

Do not type the angle brackets. `10.10.10.60` is already the real controller
address. This first deployment deliberately uses the IP address, so no local
DNS setup is required. The generated TLS certificate includes that IP.

## Part A — prepare both SSH logins

From your normal administrator workstation, verify both VMs are reachable:

```sh
ssh <CONTROLLER_USER>@10.10.10.60
ssh <CLIENT_USER>@<CLIENT_IP>
```

On the Lubuntu VM, enable SSH if the second command does not connect:

```sh
sudo apt update
sudo apt install -y openssh-server
sudo systemctl enable --now ssh
ip -br address
```

Public keys are optional. If you want them, install them with:

```sh
ssh-copy-id <CONTROLLER_USER>@10.10.10.60
ssh-copy-id <CLIENT_USER>@<CLIENT_IP>
```

Whether you use passwords or keys, open a fresh SSH session and verify `sudo`
before provisioning. ThinPi preserves the host's existing SSH password setting.

## Part B — install the controller on Ubuntu Server

SSH to the controller and install Docker Engine from Docker's signed Ubuntu
repository:

```sh
sudo apt update
sudo apt full-upgrade -y
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
sudo usermod -aG docker "$USER"
newgrp docker
```

The `docker` group has root-equivalent control of this dedicated server.
`newgrp docker` starts a new shell with that membership immediately, so you do
not need to sign out and reconnect. Then verify:

```sh
docker version
docker compose version
docker run --rm hello-world
```

Clone the published source:

```sh
sudo install -d -m 0755 /opt/thinpi
sudo chown "$USER":"$USER" /opt/thinpi
git clone https://github.com/0xP1ckl3d/ThinPi.git /opt/thinpi
cd /opt/thinpi
git status --short
```

`git status --short` must print nothing.

## Part C — create TLS without DNS

Do this directly on the controller. A third server or separate Linux
workstation is not required. The CA key is used only when issuing or renewing
certificates; it is not mounted into the running controller container.

```sh
cd /opt/thinpi
sh scripts/generate-controller-pki.sh \
  thinpi-controller 10.10.10.60 "$HOME/thinpi-pki-production"
sudo install -d -o root -g root -m 0750 deploy/controller/tls
sudo chown root:65532 deploy/controller/tls
sudo install -o root -g root -m 0640 \
  "$HOME/thinpi-pki-production/tls.crt" \
  deploy/controller/tls/tls.crt
sudo chown root:65532 deploy/controller/tls/tls.crt
sudo install -o root -g root -m 0640 \
  "$HOME/thinpi-pki-production/tls.key" \
  deploy/controller/tls/tls.key
sudo chown root:65532 deploy/controller/tls/tls.key
sudo openssl x509 -in deploy/controller/tls/tls.crt -noout \
  -subject -issuer -dates -ext subjectAltName
```

The final output must contain `IP Address:10.10.10.60`.

Still on the controller, copy the public CA certificate to Lubuntu:

```sh
scp "$HOME/thinpi-pki-production/thinpi-ca.crt" \
  <CLIENT_USER>@<CLIENT_IP>:/tmp/thinpi-ca.crt
```

The controller can retain `$HOME/thinpi-pki-production/thinpi-ca.key` with
mode `0600` for later certificate renewal. For stronger security, optionally
copy that one file to encrypted removable/offline storage and remove the
controller copy. That is hardening, not a deployment requirement.

To trust the public CA in a Windows admin browser, run this from Windows
PowerShell to download it:

```powershell
scp <CONTROLLER_USER>@10.10.10.60:/home/<CONTROLLER_USER>/thinpi-pki-production/thinpi-ca.crt `
  "$env:USERPROFILE\Downloads\thinpi-ca.crt"
```

Then open PowerShell as Administrator and run:

```powershell
Import-Certificate `
  -FilePath "$env:USERPROFILE\Downloads\thinpi-ca.crt" `
  -CertStoreLocation Cert:\LocalMachine\Root
```

## Part D — configure and start the real controller

Create the credential encryption key and production environment file:

```sh
cd /opt/thinpi
sudo install -d -o root -g root -m 0750 deploy/controller/secrets
sudo chown root:65532 deploy/controller/secrets
sudo sh -c 'umask 0077; openssl rand -base64 32 > deploy/controller/secrets/thinpi_master_key'
sudo chown root:65532 deploy/controller/secrets/thinpi_master_key
sudo chmod 0640 deploy/controller/secrets/thinpi_master_key
cp deploy/controller/.env.example deploy/controller/.env
sed -i 's/^THINPI_BIND_ADDRESS=.*/THINPI_BIND_ADDRESS=10.10.10.60/' \
  deploy/controller/.env
```

Back up `thinpi_master_key` separately. Losing it makes stored target
credentials unrecoverable. Confirm the configuration and start:

```sh
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml config
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml up -d --build
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml ps
```

Wait for `controller` to show `healthy`, then test using the private CA:

```sh
curl --fail --show-error \
  --cacert "$HOME/thinpi-pki-production/thinpi-ca.crt" \
  https://10.10.10.60:8443/healthz
```

Do not point ordinary `curl` at `deploy/controller/tls/tls.crt`. That directory
is intentionally readable only by root and the controller container's numeric
GID, so the shell user cannot traverse it. The public CA file under `$HOME` is
the correct readable trust file for this health check.

If UFW is active, permit SSH and TCP 8443 only from the local client/admin
network. This example assumes the two VMs are on `10.10.10.0/24`; change the
subnet if yours differs:

```sh
sudo ufw status
sudo ufw allow OpenSSH
sudo ufw allow from 10.10.10.0/24 to any port 8443 proto tcp
sudo ufw status numbered
```

Do not expose port 8443 directly to the public Internet.

Create the first real administrator:

```sh
COMPOSE_FILE=deploy/controller/compose.yml \
COMPOSE_ENV_FILE=deploy/controller/.env \
  scripts/bootstrap-admin.sh
```

Open `https://10.10.10.60:8443/` in a browser that trusts
`thinpi-ca.crt`. Production starts empty: create a test user, one credential,
one real connection, and an access rule linking all three.

## Part E — prepare Lubuntu 26.04

Run on the Lubuntu client before converting it to a kiosk:

```sh
sudo apt update
sudo apt full-upgrade -y
sudo apt install -y git rsync curl ca-certificates build-essential \
  cmake ninja-build pkg-config qt6-base-dev qt6-declarative-dev \
  golang-go openssh-server
uname -m
dpkg --print-architecture
grep -E '^(PRETTY_NAME|VERSION_ID|VERSION_CODENAME)=' /etc/os-release
go version
qmake6 -query QT_VERSION
curl --fail --show-error --cacert /tmp/thinpi-ca.crt \
  https://10.10.10.60:8443/healthz
```

Required results are `x86_64`, `amd64`, Lubuntu/Ubuntu `26.04`, Go 1.25 or
newer, Qt 6.4 or newer, a working administrator SSH login, and controller health JSON.
Do not continue if any check fails.

## Part F — build, enrol, and provision the Lubuntu client

From a Linux/WSL checkout of ThinPi on the administrator workstation:

```sh
cd ThinPi
sh scripts/deploy-client.sh <CLIENT_USER>@<CLIENT_IP>
```

The command builds the amd64 agent and launcher natively on Lubuntu and ends
with `Binaries staged in /usr/local/bin` on a first install.

On the controller, create a one-use token:

```sh
cd /opt/thinpi
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml exec controller \
  /usr/bin/thinpi-controller create-enrolment-token \
  --name 'Lubuntu canary VM' --ttl 30m
```

SSH to Lubuntu and run the final conversion. Paste the token only at the hidden
prompt:

```sh
sudo sh /tmp/thinpi/deploy-client/provision.sh \
  --platform generic \
  --server https://10.10.10.60:8443 \
  --device-id lubuntu-canary-01 \
  --name 'Lubuntu canary VM' \
  --ca-certificate /tmp/thinpi-ca.crt \
  --moonlight no
```

Use `--moonlight yes` only when this VM needs Moonlight and its GPU/driver is
ready. RDP, VNC, and locked SSH do not require Moonlight. Success ends with:

```text
Provisioning complete for generic/amd64 on ubuntu 26.04. Reboot to start the ThinPi kiosk.
```

Reboot only after that line appears:

```sh
sudo reboot
```

## Part G — acceptance checks

The VM console must show the ThinPi login, not SDDM or the Lubuntu desktop.
From the administrator workstation:

```sh
ssh <CLIENT_USER>@<CLIENT_IP>
systemctl get-default
systemctl is-active thinpi-agent thinpi-ui
getent passwd thinpi
sudo /usr/bin/thinpi-agent status --config /etc/thinpi/agent.json
sudo journalctl -b -u thinpi-agent -u thinpi-ui --no-pager
```

Required: `thinpi.target`, both services `active`, `thinpi` uses
`/usr/sbin/nologin`, and the device ID is `lubuntu-canary-01`. Then verify a
real assigned connection, administrator dashboard handoff, and authorised
local maintenance. Normal users must not reach a local shell, another browser
origin, a second SSH tab, virtual terminals, downloads, or developer tools.

If the kiosk fails, do not reinstall blindly. SSH still works with the key:

```sh
sudo systemctl isolate multi-user.target
sudo journalctl -b -u thinpi-agent -u thinpi-ui --no-pager
```

Return to the kiosk with `sudo systemctl isolate thinpi.target`.
