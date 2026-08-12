# Controller reference

> Do not start here for a first deployment. Follow
> [Deploy ThinPi from zero](deployment.md); it tells you exactly when to use
> this reference. This page is for alternate controller operating systems,
> certificate arrangements and ongoing operations.

The worked example uses:

| Setting | Example |
|---|---|
| Source directory | `/opt/thinpi` |
| Controller host | `controller01.home.example` / `10.20.0.10` |
| HTTPS name | `thinpi.home.example` |
| Browser/API URL | `https://thinpi.home.example:8443` |

Every hostname in this table is an example, not something ThinPi creates. For
the simplest deployment use a reserved controller IP such as
`https://10.20.0.10:8443`; the complete guide generates a certificate with that
IP in its SAN and requires no DNS. Use the friendly-name example only after you
create the matching DNS record.

## 1. Prepare the Linux controller host

Recommended minimum:

- Debian or Ubuntu VM, 1–2 vCPU, 1 GiB RAM, 8 GiB disk;
- static address or DHCP reservation;
- working DNS and NTP;
- Docker Engine and the Docker Compose plugin;
- inbound TCP 8443 restricted to administrator and client networks.

On Proxmox, prefer a small VM. Do not make an LXC privileged just to run
Docker.

Install ordinary host tools:

```sh
sudo apt update
sudo apt install -y ca-certificates curl git openssl rsync
```

Install Docker Engine from Docker's official Debian/Ubuntu repository, including
the Compose plugin. Then verify:

```sh
docker version
docker compose version
docker run --rm hello-world
```

Official installation references:

- [Docker Engine installation](https://docs.docker.com/engine/install/)
- [Docker Compose plugin](https://docs.docker.com/compose/install/linux/)

All remaining commands assume your user can run Docker. Use a dedicated
operations account rather than making the controller generally multi-user.

## 2. Put this source tree on the controller host

If your repository has a remote, clone it into `/opt/thinpi`. Otherwise copy
the working tree from the administrator workstation:

```sh
sudo install -d -m 0755 /opt/thinpi
sudo chown "$USER":"$USER" /opt/thinpi
```

Run this from the repository root on the workstation:

```sh
rsync -a --delete \
  --exclude .thinpi-dev --exclude build --exclude bin --exclude .docker-config \
  ./ thinpiops@controller01.home.example:/opt/thinpi/
```

On the controller host:

```sh
cd /opt/thinpi
test -f deploy/controller/compose.yml
test -f controller/Dockerfile
```

Do not copy an existing development Docker volume or `.thinpi-dev` directory.

## 3. Choose IP-only or configure DNS before the certificate

For an IP-only deployment, skip DNS and ensure the controller address has a
DHCP reservation. Generate a certificate containing that IP and use the same
IP URL everywhere.

For a friendly name, create the DNS record yourself on the router, Pi-hole,
AdGuard Home, Active Directory DNS, or other DNS server. For example:

```text
thinpi.home.arpa A 10.20.0.10
```

Verify from both an administrator workstation and every client network:

```sh
getent hosts thinpi.home.arpa
```

The certificate must contain whichever DNS name or IP appears in the URL as a
Subject Alternative Name. The launcher, agent and browser must use the same
origin.

## 4. Install TLS

Create the destination first:

```sh
cd /opt/thinpi
install -d -m 0750 deploy/controller/tls
```

### Option A: certificate from an existing/public CA

Install the full certificate chain and unencrypted server key:

```sh
sudo install -o root -g 65532 -m 0640 /secure/source/fullchain.pem \
  deploy/controller/tls/tls.crt
sudo install -o root -g 65532 -m 0640 /secure/source/server.key \
  deploy/controller/tls/tls.key
```

### Option B: private home CA

Use your existing private CA if you have one. For the simplest two-server
deployment, the helper may run directly on the controller; keep its CA key mode
`0600` and never mount it into the container. Moving that key to encrypted
offline storage is recommended hardening, not a third-server requirement.
Issue a server certificate containing `DNS:thinpi.home.example`; include an IP
SAN only if clients will deliberately use the IP address.

For maximum separation, run this from the source tree on a secure Linux/WSL
administrator workstation. For direct controller generation, run the same
command from `/opt/thinpi` and install `tls.crt`/`tls.key` locally as shown in
the worked [Ubuntu/Lubuntu guide](ubuntu-lubuntu-deployment.md):

```sh
sh scripts/generate-controller-pki.sh \
  thinpi.home.example 10.20.0.10 "$HOME/thinpi-pki"
```

The helper refuses to overwrite an existing CA, verifies the issued
certificate, and prints its SAN. Protect `$HOME/thinpi-pki/thinpi-ca.key`
offline; that key can issue trusted certificates and must never be copied to
the controller or Pi.

Copy only these files to the controller:

```text
tls.crt       server certificate plus issuing chain
tls.key       unencrypted server private key
thinpi-ca.crt public CA certificate (also copied to each Pi)
```

From the administrator workstation:

```sh
scp "$HOME/thinpi-pki/tls.crt" "$HOME/thinpi-pki/tls.key" \
  thinpiops@controller01.home.example:/tmp/
```

On the controller host, install the server pair and remove the transfer copy:

```sh
sudo install -o root -g 65532 -m 0640 /tmp/tls.crt \
  deploy/controller/tls/tls.crt
sudo install -o root -g 65532 -m 0640 /tmp/tls.key \
  deploy/controller/tls/tls.key
sudo rm -f /tmp/tls.crt /tmp/tls.key
```

Keep `thinpi-ca.crt` available on the administrator workstation for the Pi
provisioning step. Never copy the CA private key to a Pi.

Validate the installed certificate before starting:

```sh
openssl x509 -in deploy/controller/tls/tls.crt -noout \
  -subject -issuer -dates -ext subjectAltName
openssl pkey -in deploy/controller/tls/tls.key -check -noout
```

## 5. Create the credential-encryption master key

This key encrypts every stored remote password. Losing it makes those passwords
unrecoverable even if the database survives.

```sh
cd /opt/thinpi
install -d -m 0750 deploy/controller/secrets
umask 0077
openssl rand -base64 32 > deploy/controller/secrets/thinpi_master_key
sudo chown root:65532 deploy/controller/secrets/thinpi_master_key
sudo chmod 0640 deploy/controller/secrets/thinpi_master_key
```

Immediately copy this file to a separate encrypted/offline backup. Do not store
it in Git, chat, the SQLite volume, or the same only disk as the controller.

## 6. Configure Compose

```sh
cd /opt/thinpi
cp deploy/controller/.env.example deploy/controller/.env
```

Edit `deploy/controller/.env`:

```dotenv
THINPI_VERSION=0.1.0
THINPI_BIND_ADDRESS=10.20.0.10
THINPI_MASTER_KEY_FILE=./secrets/thinpi_master_key
```

`THINPI_BIND_ADDRESS` is the controller host address, not the Pi address and
not the DNS name. Use `0.0.0.0` only if host firewall policy deliberately
restricts every interface.

The shipped container runs as UID/GID 65532. Confirm the mounted files are
readable by that identity:

```sh
sudo namei -l deploy/controller/tls/tls.crt
sudo namei -l deploy/controller/tls/tls.key
sudo namei -l deploy/controller/secrets/thinpi_master_key
```

Rootless Docker and user-namespace remapping require different host ownership.
Resolve that mapping before continuing; do not make secrets world-readable.

## 7. Validate and start the controller

```sh
cd /opt/thinpi
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml config
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml up -d --build
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml ps
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml logs --tail 100 controller
```

Wait until `ps` reports the controller as `healthy`. Then test the exact public
URL:

```sh
curl --fail --silent --show-error \
  https://thinpi.home.example:8443/healthz
```

For a private CA, use this before the CA is installed system-wide:

```sh
curl --fail --show-error --cacert /secure/path/thinpi-ca.crt \
  https://thinpi.home.example:8443/healthz
```

Expected response:

```json
{"status":"ok","version":"0.1.0"}
```

Do not use `curl --insecure` as a solution. Fix DNS, certificate SAN, dates, or
the CA chain.

## 8. Create the first administrator

The production database starts empty. The bootstrap command works only while
no administrator exists and prompts without echoing the password:

```sh
cd /opt/thinpi
COMPOSE_FILE=deploy/controller/compose.yml \
COMPOSE_ENV_FILE=deploy/controller/.env \
  scripts/bootstrap-admin.sh
```

Use at least eight characters and store the password in your password manager.
Expected output is `Administrator created.`

Open:

```text
https://thinpi.home.example:8443/
```

The root redirects to `/admin/login`. After login, `/` and `/admin/login`
redirect to `/admin`. An expired browser session redirects back to login.

## 9. Configure the first production objects

In the admin console, use this order:

1. create a non-administrator test person;
2. create a stored credential for one real target;
3. create the real RDP, VNC, locked SSH, or Moonlight connection;
4. create an Access rule that assigns both connection and credential;
5. add Restrictions if required;
6. confirm the connection appears only for the intended user.

Do not create a client enrolment token until that client is ready to consume it.

## 10. Create a one-time client enrolment token

Run immediately before provisioning one Pi:

```sh
cd /opt/thinpi
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml exec controller \
  /usr/bin/thinpi-controller create-enrolment-token \
  --name "Living room Pi" --ttl 30m
```

The printed value is a one-time bearer secret. Enter it only into the Pi
provisioner's hidden prompt. Create a different token for every Pi.

## 11. Firewall rules

Allow only:

- administrator network → controller TCP 8443;
- client networks → controller TCP 8443;
- explicit monitoring/backup management as required;
- controller host → DNS, NTP, and update repositories.

The controller needs no inbound RDP, VNC, SSH-target, or Moonlight ports and normally needs
no route to target desktops.

## 12. Back up before enrolling a Pi

The simple consistent backup pauses the controller briefly:

```sh
cd /opt/thinpi
mkdir -p backups
backup="thinpi-$(date -u +%Y%m%dT%H%M%SZ).db"
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml stop controller
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml run --rm --no-deps \
  --entrypoint sh -v "$PWD/backups:/backup" initialise-data \
  -c "cp /var/lib/thinpi/thinpi.db /backup/$backup"
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml start controller
ls -l "backups/$backup"
```

Copy the database backup and matching master key off-host. A database restored
with the wrong master key retains users and rules but cannot decrypt stored
credentials.

### Restore

Stop the controller and preserve the current volume before restoring:

```sh
cd /opt/thinpi
restore=thinpi-20260812T120000Z.db
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml stop controller
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml run --rm --no-deps \
  --entrypoint sh -e RESTORE_FILE="$restore" \
  -v "$PWD/backups:/backup:ro" initialise-data \
  -c 'cp "/backup/$RESTORE_FILE" /var/lib/thinpi/thinpi.db && chown 65532:65532 /var/lib/thinpi/thinpi.db && chmod 0600 /var/lib/thinpi/thinpi.db'
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml start controller
```

The mounted master key must be the one backed up with that database.

## 13. Updates and certificate renewal

Take a backup, update the source, then rebuild only the controller:

```sh
cd /opt/thinpi
git pull --ff-only
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml build --pull controller
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml up -d
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml ps
curl --fail https://thinpi.home.example:8443/healthz
```

If source is transferred with `rsync`, transfer the reviewed release instead
of running `git pull`.

Database migrations run at controller startup. Do not downgrade across a
schema migration without restoring the matching database and application
version.

For TLS renewal, replace `tls.crt` and `tls.key`, preserve ownership/mode,
restart the controller, and validate from a client before the old certificate
expires.

## 14. Controller troubleshooting commands

```sh
cd /opt/thinpi
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml ps
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml logs --tail 200 controller
ss -lntp | grep 8443
openssl s_client -connect thinpi.home.example:8443 \
  -servername thinpi.home.example -showcerts </dev/null
```

See [troubleshooting.md](troubleshooting.md) for symptom-based diagnosis.
