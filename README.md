# ThinPi

ThinPi turns a Debian 13, Ubuntu/Lubuntu LTS, mini PC, VM, ARM64 device, or Raspberry Pi 4/5 into a locked-down thin client. A user signs
into the full-screen launcher and selects an assigned Windows/Linux RDP, Linux
VNC, locked remote SSH, or Moonlight/Sunshine session. The controller stores users,
policies, connection definitions, and encrypted remote credentials.

This README contains the two workflows most people need:

1. [Run the complete test environment on Windows](#run-the-complete-test-environment-on-windows).
2. [Deploy a production controller and Linux client](#deploy-a-production-client).

## Understand the two machines

ThinPi is not one program copied to a Pi:

| Part | Runs on | Purpose |
|---|---|---|
| Controller | Linux VM/server with Docker | HTTPS API, admin console, users, policies, encrypted credentials, audit data |
| Launcher | Dedicated supported Linux client display | Full-screen login and connection selection UI |
| Agent | Same client | Redeems one-time tickets and starts FreeRDP, TigerVNC, locked OpenSSH, or Moonlight |

The controller does **not** relay the remote desktop stream. Each client must be able
to reach the RDP, VNC, or Sunshine host directly or through a separately
configured routed VPN. See [Networking](docs/networking.md) before deploying
targets on a different private network.

## Run the complete test environment on Windows

This is the supported local workflow. It runs:

- the controller in Docker Desktop at `http://127.0.0.1:8080`;
- a native Windows agent using safe mock clients;
- the real compiled Qt launcher;
- a persistent development database in a Docker volume.

The default test environment never contacts a real desktop. A launch displays
the simulated session for 12 seconds, then returns to the launcher dashboard.

### 1. Install the Windows prerequisites once

Install:

- Docker Desktop using Linux containers;
- Go 1.25 or newer;
- Qt 6.5 or newer with the **MinGW 64-bit** desktop kit;
- the Qt installer components **CMake**, **Ninja**, and **MinGW**;
- PowerShell 7 is recommended, although Windows PowerShell also works.

The scripts automatically discover a normal Qt installation under `C:\Qt`.
If Qt is elsewhere, set `QT_ROOT` to the kit directory containing
`bin\Qt6Core.dll`.

From PowerShell, verify the prerequisites:

```powershell
docker version
docker compose version
go version
Get-ChildItem C:\Qt -Recurse -Filter Qt6Core.dll | Select-Object -First 1
```

Docker Desktop must be running before continuing. If PowerShell blocks local
scripts, use this only for the current terminal:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
```

### 2. Start and build everything

Open PowerShell in the repository root—the directory containing this README—
and run exactly:

```powershell
.\scripts\dev-up.ps1
```

The first run downloads container dependencies and can take several minutes.
The script then:

1. builds and starts the controller container;
2. waits for `/healthz`;
3. builds the Windows agent;
4. builds the Qt launcher and launcher tests;
5. starts the agent in the background;
6. writes only development state under `.thinpi-dev`.

Successful output ends with:

```text
ThinPi development environment is ready.
Controller: http://127.0.0.1:8080
Admin UI:  http://127.0.0.1:8080/admin/login
Users:     admin, wife, daughter
Password:  thinpi-dev
Mode:      Safe 12-second demo sessions
```

If it fails, do not continue. Read the first reported error, then use the
[local troubleshooting table](#local-test-troubleshooting).

### 3. Run the automated end-to-end check

```powershell
.\scripts\dev-test.ps1
```

The check logs in as Daughter, requests an authorised connection, redeems the
device-bound ticket through the real local agent API, observes the active mock
session, and confirms the agent returns to idle. It must end with:

```text
End-to-end launch flow passed.
```

### 4. Test the administration console

Open either URL; `/` now redirects naturally:

- `http://127.0.0.1:8080/`
- `http://127.0.0.1:8080/admin/login`

Sign in with:

```text
Username: admin
Password: thinpi-dev
```

Verify the following before testing the launcher:

1. **People** shows account enable/disable actions and Restrictions.
2. **Connections** offers RDP, Linux VNC, locked SSH, Moonlight, and a development-only
   demo type.
3. **Credentials** stores a remote username/password by label.
4. **Access rules** assigns a connection and credential to a person or group.
5. **Devices** shows the Development Pi heartbeat.

An expired admin browser session redirects to the login screen. Visiting `/`
or `/admin/login` while already signed in redirects to `/admin`.

### 5. Test the real launcher

```powershell
.\scripts\dev-client.ps1
```

Use either account:

| Username | Password | What to verify |
|---|---|---|
| `daughter` | `thinpi-dev` | Assigned connections, parental limits, 12-second session, return to dashboard |
| `admin` | `thinpi-dev` | Same launcher plus the **Administration** button |

The Administration button requests a 45-second, single-use handoff and opens
the admin console without asking for the password again. Non-administrators
cannot create that handoff.

### 6. Inspect or stop the environment

```powershell
.\scripts\dev-status.ps1
.\scripts\dev-down.ps1
```

`dev-down.ps1` stops the controller and agent but keeps the test database. To
delete all test data and reseed it on the next start:

```powershell
.\scripts\dev-down.ps1 -ResetData
.\scripts\dev-up.ps1
```

### Optional: test installed native clients on Windows

Only after creating connections that point at reachable real hosts, restart in
real-client mode:

```powershell
.\scripts\dev-down.ps1
.\scripts\dev-up.ps1 -RealClients
.\scripts\dev-client.ps1
```

FreeRDP, TigerVNC Viewer, or Moonlight must already be installed and on `PATH`.
The seeded `*.invalid` development targets are deliberately unreachable, so
replace them in the admin console first.

### Local test troubleshooting

| Symptom | Check |
|---|---|
| Docker pipe/config access error | Start Docker Desktop, wait for “Engine running,” then run `docker version` |
| Qt not found | Install the MinGW 64-bit Qt kit under `C:\Qt`, or set `QT_ROOT` |
| `cmake` compiler exits with no message | Ensure the Qt MinGW `bin` directory is installed; `dev-up.ps1` adds it to `PATH` |
| Port 8080 already in use | Stop the other service or run `.\scripts\dev-down.ps1` |
| Agent named-pipe error | Run `.\scripts\dev-status.ps1`, then restart with `dev-down.ps1` and `dev-up.ps1` |
| Browser shows old UI | Rebuild with `dev-up.ps1`, then hard-refresh the page |
| Reset still shows old data | Use `dev-down.ps1 -ResetData`; ordinary shutdown intentionally preserves the volume |

## Deploy a production client

Do not copy the Windows development environment to a client. Production deployment
has this order:

1. prepare a Linux Docker host for the controller;
2. choose stable controller DNS and TLS;
3. start the production controller and create its first administrator;
4. install Debian 13, Ubuntu/Lubuntu 24.04 or 26.04 LTS, or Raspberry Pi OS Lite 64-bit on a Pi;
5. enable SSH public-key authentication and verify it works;
6. build and stage matching amd64 or arm64 agent/launcher binaries;
7. create a one-time device enrolment token;
8. run the generic client provisioner;
9. reboot and complete the acceptance checklist;
10. configure users, credentials, connections, and access rules.

### Production requirements

You need:

- a Linux VM/server running Docker Engine and the Compose plugin;
- a reserved controller LAN IP; friendly DNS is optional and the end-to-end
  guide explains exactly how to add it;
- an HTTPS certificate for that exact name;
- an amd64 client running Debian 13/Trixie or Ubuntu/Lubuntu 24.04 or 26.04
  LTS; Debian 13 and Raspberry Pi OS also support arm64; Raspberry Pi OS Lite
  64-bit is supported on Pi 4/5;
- wired Ethernet where practical;
- an administrator workstation with this source tree, `ssh`, and `rsync`;
- at least one reachable RDP, VNC, or Sunshine target.

Windows users should run the client deployment script from WSL. It is a POSIX
`ssh`/`rsync` script, not a native PowerShell script.

### Start with the single end-to-end production guide

Follow [Deploy ThinPi from zero](docs/deployment.md) from top to bottom. It
explains what runs on each machine, recommends an IP-based first deployment
that requires no DNS, shows every command and expected checkpoint, and links
to component reference pages only when they are useful.

For the exact Ubuntu Server controller plus Lubuntu 26.04 VM combination, use
the shorter [worked Ubuntu/Lubuntu install](docs/ubuntu-lubuntu-deployment.md).
It uses controller IP `10.10.10.60`, requires no DNS, and includes every command
and checkpoint from SSH preparation through kiosk acceptance.

Then use [Client deployment](docs/client-deployment.md) for a VM, mini PC, ARM
device, or Pi. Do not start with component reference pages; they assume you are
at the corresponding step of the end-to-end guide.

If you already have an Ubuntu controller and a Lubuntu 26.04 amd64 VM, that is
a supported combination. The client guide has a labelled Lubuntu command path;
provisioning disables SDDM and makes the systemd ThinPi kiosk the boot target.
The Lubuntu desktop is not exposed to ordinary ThinPi users.

### Production is intentionally different from development

Production does not seed `admin`, `wife`, or `daughter`; does not accept
`thinpi-dev`; does not display the demo card; rejects the mock protocol; and
does not run simulated desktops. Production sessions start real native clients
and return safe, useful connection failures.

The client becomes a kiosk, but it remains maintainable over the administrator SSH
key created before provisioning. An authenticated ThinPi administrator also
gets a one-use, device-bound **Local maintenance** action that opens the fixed
local administrator console and signs the launcher out. Normal users cannot
switch virtual terminals or request that console. Updates can run there or,
preferably, from a separate workstation with `scripts/deploy-client.sh`.

## Important current boundary: routed network pivot

Direct client-to-target connections are implemented. Automated Tailscale/WireGuard
enrolment is not. If a target exists only behind the controller site's deeper
network, configure the controller host or a nearby Linux host as a subnet
router and install the corresponding VPN client on each appliance before acceptance.
The controller container itself is not a network proxy.

## Repository map

| Path | Contents |
|---|---|
| `controller/` | Go HTTPS controller, SQLite database, admin UI |
| `agent/` | Go local agent and native-client command builders |
| `launcher/` | C++20/Qt 6/QML kiosk launcher |
| `deploy/controller/` | Production and development Compose files |
| `deploy/client/` | Supported Linux provisioner, systemd units and kiosk policy |
| `deploy/pi/` | Raspberry Pi compatibility wrappers and references |
| `scripts/` | Local environment, build, deployment, logs, packaging |
| `docs/` | Production operations, networking, security, API, acceptance |

## Security material you must not lose

Back up these separately:

- the controller SQLite database;
- `deploy/controller/secrets/thinpi_master_key`;
- the private CA and its private key if you operate your own CA;
- the administrator SSH private key used to maintain each client.

Losing the controller master key makes stored remote credentials
unrecoverable. Losing client SSH access may require offline disk or SD-card recovery.
