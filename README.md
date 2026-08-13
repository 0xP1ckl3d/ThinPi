# ThinPi

ThinPi turns a dedicated Lubuntu x86-64 machine or Raspberry Pi 4/5 into a
locked-down thin client. Users sign into the full-screen launcher and open
assigned RDP, VNC, locked SSH, or Moonlight/Sunshine sessions. A separate Linux
controller runs the HTTPS administration console, policy engine, encrypted
credential store, and audit database.

## Supported production deployments

| Component | Supported platform |
|---|---|
| Controller | Linux server or VM with Docker Compose |
| Lubuntu client | Lubuntu 24.04 or 26.04 LTS, amd64 |
| Raspberry Pi client | Raspberry Pi OS Lite 64-bit, Debian Trixie, Pi 4 or 5 |

The controller authorises sessions but does not relay their display traffic.
Each thin client must reach its RDP, VNC, SSH, or Sunshine target directly or
through a routed VPN.

## Deployment guides

- [Controller reference](docs/controller-deployment.md)
- [Worked Ubuntu controller and Lubuntu client install](docs/ubuntu-lubuntu-deployment.md)
- [Raspberry Pi deployment](docs/pi-deployment.md)
- [Networking and routed targets](docs/networking.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Raspberry Pi hardware acceptance](docs/hardware-validation.md)

Use `scripts/deploy-client.sh` for an existing Lubuntu client and
`scripts/deploy-pi.sh` for a Raspberry Pi. Both copy a fresh source snapshot to
the target, build there, install updates, restart the services, and verify that
the kiosk remains running.

## Kiosk behaviour

- The display powers down after 15 minutes without local keyboard or mouse
  activity by default. Administrators control the timeout from **Kiosk
  settings** in the controller; use `0` to disable display sleep. Clients pick
  up changes within one minute without reprovisioning.
- Administrators can show the most recently used accounts on the sign-in screen
  or require everyone to enter their username manually.
- **Kiosk settings** also provides Ocean, Graphite, Forest, Sunset and High
  Contrast client palettes plus dark/light SSH shell appearance. Clients apply
  palette changes within one minute.
- Administrators can add or replace each person's profile photo from **People**.
  Photos are resized in the browser and displayed on the kiosk login and
  dashboard screens.
- The signed-in user has one text clipboard across RDP, VNC, SSH and the
  controller administration browser. It survives changing connections and is
  cleared at ThinPi sign-out. Local maintenance signs out and switches to its
  protected administrator console.
- Moonlight can type client clipboard text into its host with its
  Ctrl+Alt+Shift+V shortcut, but the upstream GameStream protocol cannot copy
  host clipboard content back to ThinPi.
- The login and dashboard can be operated with Tab/Shift+Tab, Enter, Space and
  Escape. A normal pointer remains available whenever a mouse is attached,
  including for selecting and copying text in shells.
- Windows/Command+L ends any active remote connection, closes the managed
  administration browser, revokes the dashboard session, and returns to login.
- Active connections have a VMware-style control strip at the top edge. Move
  the pointer to its position to reveal it, drag it along the top, optionally
  pin it, or use it to minimise or close the connection. A minimised connection
  is marked on its card; selecting that card restores the same live session.
- Display sleep is suspended while a remote session is active.
- User inactivity logout remains independently configurable in each user's
  controller policy.

## Updating production

Update the controller checkout and container:

```sh
cd /opt/thinpi
git pull --ff-only origin main
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml up -d --build
```

Then deploy the matching client code from that checkout:

```sh
# Lubuntu
sh scripts/deploy-client.sh thinpiadmin@lubuntu-client

# Raspberry Pi
sh scripts/deploy-pi.sh piadmin@thinpi-pi
```

## Repository map

| Path | Purpose |
|---|---|
| `controller/` | Go HTTPS controller and administration UI |
| `agent/` | Linux device agent and native-session launchers |
| `launcher/` | Qt/QML kiosk interface |
| `deploy/controller/` | Production Compose deployment |
| `deploy/client/` | Shared Lubuntu and Raspberry Pi provisioning |
| `deploy/pi/` | Raspberry Pi compatibility wrappers |
| `scripts/` | Production build, deployment, and diagnostics |
| `docs/` | Production operations, security, networking, and acceptance |

## Protect these files

Back up the controller SQLite volume and
`deploy/controller/secrets/thinpi_master_key` together. Losing the master key
makes stored remote credentials unrecoverable. Also retain the private CA key
when using a private controller CA and the administrator SSH key used to recover
each thin client.
