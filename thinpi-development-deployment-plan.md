# ThinPi Development and Deployment Plan

**Status:** Initial implementation specification
**Target:** Codex / autonomous development agent
**Working project name:** ThinPi
**Primary deployment target:** Raspberry Pi 5, Raspberry Pi OS Lite 64-bit
**Secondary target:** Raspberry Pi 4 where performance is acceptable
**Controller target:** Proxmox-hosted Linux VM/LXC/Docker environment
**Document date:** 2026-08-11

---

## 1. Project Objective

Build a centrally managed, native thin-client appliance for Raspberry Pi.

The Raspberry Pi must not present a normal Linux desktop to the user. After boot, it should present only a purpose-built fullscreen login/dashboard application. Users sign in and see only the remote systems that an administrator has assigned to them.

Remote sessions must use native remote-display clients rather than browser-based gateways.

Primary protocols:

- **RDP via FreeRDP** for normal Windows desktop use.
- **Moonlight via Sunshine** for gaming, multimedia-heavy desktops, or other latency-sensitive workloads.
- **VNC via TigerVNC** for Linux desktops.
- **SSH via a single-purpose xterm/OpenSSH wrapper** for assigned remote command-line access without a local Pi shell.

Future/optional protocols:

- SPICE.

The user experience should feel as close as practical to sitting at a directly attached computer.

Do **not** implement or proxy the RDP, GameStream, VNC, SPICE, or video protocols. ThinPi is only responsible for:

1. Authentication.
2. Authorisation.
3. Connection presentation.
4. Secure launch configuration.
5. Starting/stopping native client software.
6. Returning the user to the ThinPi dashboard after the remote session exits.
7. Centralised administration.

---

## 2. Core Requirements

### 2.1 User Experience

At power-on:

```text
Raspberry Pi boots
    ↓
Minimal Linux userspace
    ↓
ThinPi session starts automatically
    ↓
Fullscreen ThinPi login screen
```

After login:

```text
User authenticates
    ↓
Controller returns authorised connections only
    ↓
ThinPi renders connection tiles
```

Example:

```text
┌──────────────────────────────────────────────┐
│                  ThinPi                      │
│                                              │
│  Welcome, Alice                             │
│                                              │
│  ┌────────────────┐  ┌────────────────┐     │
│  │   School PC    │  │    Game PC     │     │
│  │      RDP       │  │   Moonlight    │     │
│  └────────────────┘  └────────────────┘     │
│                                              │
│                                  Log out     │
└──────────────────────────────────────────────┘
```

Selecting an RDP machine:

```text
ThinPi dashboard
    ↓
Authorised launch request
    ↓
Local ThinPi agent
    ↓
FreeRDP fullscreen
    ↓
Remote session ends
    ↓
ThinPi dashboard returns
```

Selecting a gaming machine:

```text
ThinPi dashboard
    ↓
Authorised launch request
    ↓
Local ThinPi agent
    ↓
Moonlight fullscreen
    ↓
Sunshine host
    ↓
Remote session ends
    ↓
ThinPi dashboard returns
```

The user must never need to interact with:

- A Linux desktop.
- A browser.
- A local Pi terminal. An explicitly assigned SSH connection may expose only
  the remote host's shell inside the locked single-purpose terminal.
- A file manager.
- A taskbar.
- A package manager.
- The FreeRDP command line.
- The Moonlight host selection UI during normal use.
- ThinPi configuration files.

---

## 3. Users and Authorisation

The system must support multiple ThinPi users.

Example:

```text
Brad
 ├── Pentest-WS01
 ├── Pentest-WS02
 ├── Kali-Lab
 ├── Windows-Admin
 ├── Family-PC
 └── Gaming-PC

Wife
 ├── Wife-PC
 └── Family-PC

Daughter
 ├── School-PC
 └── Gaming-PC
```

Only an administrator can:

- Create users.
- Disable users.
- Reset passwords.
- Create groups.
- Add/remove users from groups.
- Create connection definitions.
- Edit connection definitions.
- Assign users/groups to connections.
- Add/remove ThinPi devices.
- Revoke devices.
- Configure stored remote credentials.
- Configure protocol-specific options.

Normal users must not be able to modify their connection list or connection parameters.

The launcher must request authorised connections from the controller. It must **not** download every configured connection and hide unauthorised ones locally.

Authorisation must be enforced server-side.

---

## 4. High-Level Architecture

```text
                         PROXMOX
┌──────────────────────────────────────────────────────┐
│                                                      │
│   ┌──────────────────────────────────────────────┐   │
│   │ ThinPi Controller                            │   │
│   │                                              │   │
│   │ REST API                                     │   │
│   │ Admin Web UI                                 │   │
│   │ Authentication                              │   │
│   │ ACL engine                                   │   │
│   │ Device registry                              │   │
│   │ Connection registry                          │   │
│   │ Encrypted secret store                       │   │
│   │ Audit log                                    │   │
│   │ SQLite database initially                    │   │
│   └──────────────┬───────────────────────────────┘   │
│                  │ HTTPS                             │
└──────────────────┼───────────────────────────────────┘
                   │
                   │
         ┌─────────▼─────────┐
         │ Raspberry Pi      │
         │                   │
         │ ThinPi Launcher   │
         │      │            │
         │      │ Unix socket│
         │      ▼            │
         │ ThinPi Agent      │
         │   │        │      │
         │   ▼        ▼      │
         │ FreeRDP  Moonlight│
         └───┬────────┬──────┘
             │        │
             │        │
          RDP│        │GameStream
             │        │
         ┌───▼───┐ ┌──▼─────────┐
         │Win VM │ │Sunshine VM │
         └───────┘ └────────────┘
```

---

## 5. Repository Layout

Use a monorepo unless there is a compelling technical reason not to.

Recommended structure:

```text
thinpi/
├── README.md
├── LICENSE
├── Makefile
├── docs/
│   ├── architecture.md
│   ├── api.md
│   ├── security.md
│   ├── deployment.md
│   ├── troubleshooting.md
│   └── decisions/
│       └── ADR-*.md
│
├── controller/
│   ├── cmd/
│   │   └── thinpi-controller/
│   ├── internal/
│   │   ├── auth/
│   │   ├── users/
│   │   ├── groups/
│   │   ├── connections/
│   │   ├── permissions/
│   │   ├── devices/
│   │   ├── launch/
│   │   ├── secrets/
│   │   ├── audit/
│   │   ├── database/
│   │   └── web/
│   ├── migrations/
│   ├── static/
│   ├── templates/
│   ├── Dockerfile
│   └── go.mod
│
├── agent/
│   ├── cmd/
│   │   └── thinpi-agent/
│   ├── internal/
│   │   ├── api/
│   │   ├── launch/
│   │   ├── freerdp/
│   │   ├── moonlight/
│   │   ├── process/
│   │   ├── session/
│   │   └── config/
│   └── go.mod
│
├── launcher/
│   ├── CMakeLists.txt
│   ├── src/
│   ├── qml/
│   │   ├── Login.qml
│   │   ├── Dashboard.qml
│   │   ├── ConnectionTile.qml
│   │   ├── ErrorDialog.qml
│   │   └── SessionOverlay.qml
│   ├── assets/
│   └── tests/
│
├── deploy/
│   ├── controller/
│   │   ├── compose.yml
│   │   └── .env.example
│   ├── pi/
│   │   ├── provision.sh
│   │   ├── update.sh
│   │   ├── thinpi-agent.service
│   │   ├── thinpi-ui.service
│   │   ├── xinitrc
│   │   └── hardening/
│   └── systemd/
│
├── scripts/
│   ├── dev-controller.sh
│   ├── dev-launcher.sh
│   ├── deploy-pi.sh
│   ├── build-arm64.sh
│   └── create-enrolment-token.sh
│
└── tests/
    ├── integration/
    └── e2e/
```

---

## 6. Technology Decisions

### 6.1 Controller

Use **Go**.

Reasons:

- Single deployable binary.
- Good HTTP/TLS support.
- Easy containerisation.
- Low resource usage.
- Straightforward concurrency.
- Suitable for an LXC, VM, or Docker deployment.
- Avoids a large runtime dependency tree.

Use:

- Go standard library wherever practical.
- A small router only if it materially simplifies routing.
- SQLite for the initial database.
- Server-side HTML templates for the administrative interface.
- Minimal vanilla JavaScript only where necessary.

Do not introduce React, Vue, Node.js, or a separate SPA build unless there is a demonstrated requirement.

### 6.2 Launcher

Use **C++20 + Qt 6 + QML**.

The launcher should contain only:

- Login UI.
- Authenticated user state.
- Connection list.
- Connection tiles.
- Session launch requests.
- Error handling.
- Logout.
- Optional shutdown/reboot controls where authorised.

It must not contain:

- Remote desktop protocol implementation.
- Remote passwords in persistent storage.
- Administrator configuration functions.
- Arbitrary shell command functionality.

### 6.3 Pi Agent

Use **Go**.

The agent is a local daemon responsible for privileged or security-sensitive operations:

- Device enrolment.
- Secure communication with controller.
- Launch-ticket redemption.
- Native client discovery.
- Native client invocation.
- Session process monitoring.
- Session termination.
- Return-to-dashboard behaviour.
- Protocol-specific argument construction.
- Optional VT/display management.
- Local health information.

The launcher must communicate with the agent over a local Unix domain socket.

Example:

```text
/run/thinpi/agent.sock
```

Do not expose the local agent on a TCP listener.

### 6.4 Display Stack

Initial implementation:

- Raspberry Pi OS Lite 64-bit.
- Minimal Xorg stack.
- A very small window manager if required for reliable fullscreen behaviour.
- No desktop environment.

The primary reason to use Xorg for the first stable build is FreeRDP client compatibility and predictable fullscreen behaviour.

Do not install:

- GNOME.
- KDE Plasma.
- LXDE.
- XFCE.
- A conventional Raspberry Pi desktop session.

The display stack is an implementation detail, not a user-facing environment.

Future optimisation may migrate to Wayland or direct DRM/TTY where that improves performance.

---

## 7. Protocol Strategy

### 7.1 RDP

Use the installed **FreeRDP** client.

Do not assume one binary name across every distribution release.

At agent start-up, detect supported clients in an ordered preference list, for example:

```text
xfreerdp3
xfreerdp
sdl-freerdp3
sdl-freerdp
```

For the initial Xorg deployment, prefer the X11 FreeRDP client where available.

The agent must query the installed client help/version during provisioning or startup and log the selected binary/version.

Do not place the remote password directly on the process command line.

Current FreeRDP 3 supports `/args-from:stdin` / `/args-from:<source>` functionality. Prefer passing sensitive arguments over stdin so passwords do not appear in the process list.

Conceptual invocation:

```text
xfreerdp /args-from:stdin
```

Then supply one argument per line over stdin:

```text
/v:10.10.10.41
/u:alice
/p:<secret>
/f
+auto-reconnect
...
```

Codex must confirm exact syntax against the installed FreeRDP build using its current help output.

Never automatically use `/cert:ignore` in production.

Certificate behaviour must be configurable and safe by default.

Recommended connection options should be represented as structured fields, not arbitrary user-provided command fragments.

Example model:

```json
{
  "protocol": "rdp",
  "host": "10.10.10.41",
  "port": 3389,
  "username": "alice",
  "fullscreen": true,
  "clipboard": false,
  "audio": true,
  "microphone": false,
  "drives": false,
  "auto_reconnect": true
}
```

Do not allow an administrator-supplied `extra_args` string in the first version. This creates avoidable command-injection and policy problems.

If advanced arguments are required later, implement an explicit allow-list.

### 7.2 Moonlight

Use the official Raspberry Pi build of **Moonlight Qt**.

Moonlight supports direct command-line streaming to a specific host/application.

Conceptual command:

```text
moonlight-qt stream <host> <application>
```

Typical application for a general remote desktop connection:

```text
Desktop
```

Actual installed syntax must be verified using the currently installed Moonlight build.

The Moonlight host should run **Sunshine**.

Sunshine pairing is device-oriented. ThinPi should not attempt to reimplement Moonlight/Sunshine pairing.

Device pairing should be treated as an administrative provisioning operation.

The normal child/family user must not see or interact with pairing screens.

### 7.3 Gaming Performance

Gaming performance is a primary requirement.

The final Pi deployment must benchmark:

- 1080p60.
- Controller input.
- Keyboard/mouse input.
- HDMI audio.
- Decode latency.
- Frame drops.
- Network latency.

Where possible also benchmark:

- 1440p60.
- 1080p120 if the target display and Pi support it.
- 4K only if there is a practical requirement.

The initial Moonlight implementation may run under the minimal graphical session for simplicity.

However, upstream Moonlight documentation recommends direct console/TTY execution for best Raspberry Pi performance.

Therefore implement the session architecture so that Moonlight can later be changed to:

```text
ThinPi UI
    ↓
Request gaming session
    ↓
UI/display session temporarily yields
    ↓
Moonlight runs directly on console/DRM
    ↓
Moonlight exits
    ↓
ThinPi UI/display session restarts
```

This optimisation should be adopted if benchmarking shows a meaningful latency or frame-delivery benefit.

Do not lock the design so tightly to Xorg that direct-console Moonlight becomes difficult later.

---

## 8. Controller Data Model

Minimum required entities:

### User

```text
id
username
display_name
password_hash
is_admin
enabled
created_at
updated_at
last_login_at
```

### Group

```text
id
name
description
created_at
updated_at
```

### UserGroup

```text
user_id
group_id
```

### Connection

```text
id
name
description
protocol
host
port
enabled
icon
sort_order
protocol_config_json
credential_id nullable
created_at
updated_at
```

### UserConnectionPermission

```text
user_id
connection_id
can_launch
```

### GroupConnectionPermission

```text
group_id
connection_id
can_launch
```

### Credential

```text
id
username nullable
encrypted_secret
secret_type
created_at
updated_at
```

### Device

```text
id
name
device_identifier
token_hash
enabled
last_seen_at
last_ip
created_at
updated_at
```

### LaunchTicket

```text
id
token_hash
user_id
device_id
connection_id
expires_at
redeemed_at nullable
created_at
```

### AuditEvent

```text
id
timestamp
actor_user_id nullable
device_id nullable
event_type
connection_id nullable
source_ip
result
metadata_json
```

---

## 9. Authentication Model

### 9.1 User Authentication

Initial version:

- Username + password.
- Password hashes must use Argon2id.
- Unique salt per user.
- Rate-limit failed authentication.
- Do not reveal whether username or password was incorrect.
- Configurable idle logout.
- Admin can disable accounts.

Potential future authentication:

- Short numeric child PIN.
- TOTP.
- LDAP.
- OIDC.
- Passkeys.

Do not add these to MVP unless they are straightforward and do not delay core functionality.

### 9.2 Launcher Session

After successful authentication, the launcher receives an authenticated session token.

Prefer an opaque bearer token rather than implementing complex JWT logic without a requirement.

Requirements:

- Random, high entropy.
- Short-lived.
- Stored in launcher memory only.
- Revocable server-side.
- Invalidated on explicit logout.
- Invalidated when user is disabled.

### 9.3 Device Authentication

Each Pi must be separately enrolled.

Proposed flow:

```text
Administrator creates one-time enrolment token
    ↓
Pi runs:
thinpi-agent enroll --server https://thinpi.example --token <token>
    ↓
Controller validates one-time token
    ↓
Controller creates device
    ↓
Agent receives device credential
    ↓
Credential stored root-only
```

Store at:

```text
/etc/thinpi/device.json
```

Permissions:

```text
root:root
0600
```

The launcher must never be able to read the device credential directly.

The administrator must be able to revoke a device from the controller.

---

## 10. Secure Launch Flow

Do not send reusable connection credentials directly to the QML launcher.

Use this workflow:

```text
1. User clicks a connection tile.

2. Launcher:
   POST /api/v1/connections/{id}/launch

3. Controller:
   - validates user session
   - validates connection exists and is enabled
   - evaluates user/group ACL
   - validates requesting device
   - creates one-time launch ticket
   - ticket expires after approximately 30 seconds

4. Controller returns opaque launch ticket to launcher.

5. Launcher sends ticket to:
   /run/thinpi/agent.sock

6. Agent authenticates to controller using device credential and redeems ticket.

7. Controller:
   - verifies ticket
   - verifies device binding
   - verifies expiry
   - verifies it has not already been redeemed
   - marks ticket redeemed
   - returns structured launch manifest

8. Agent launches native client.

9. Agent waits for native client exit.

10. Agent records session result and returns control to launcher.
```

The launch manifest may include an RDP password because it is delivered only to the trusted agent over HTTPS after an authorised one-time redemption.

The launcher must never receive the password.

---

## 11. Credential Storage

Remote credentials are optional.

Supported use cases:

1. Require the user to authenticate to the remote VM manually.
2. Store a dedicated VM username/password centrally for seamless launch.
3. Store a username but prompt for password.

Stored secrets must be encrypted at rest.

Do not store plaintext RDP passwords in SQLite.

Use authenticated encryption such as AES-256-GCM or ChaCha20-Poly1305.

The encryption master key must not live in the SQLite database.

For Docker deployment:

```text
/run/secrets/thinpi_master_key
```

or environment/file mounted from the host with strict permissions.

For non-container deployment:

```text
/etc/thinpi/master.key
```

Permissions:

```text
root:thinpi-controller
0640
```

Provide a documented key-backup process.

If the key is lost, encrypted remote credentials should be considered unrecoverable and must be re-entered.

---

## 12. Local Agent API

Use a Unix socket.

Possible JSON protocol:

### Launch

Request:

```json
{
  "action": "launch",
  "ticket": "<opaque launch ticket>"
}
```

Response:

```json
{
  "accepted": true,
  "session_id": "..."
}
```

### Status

```json
{
  "action": "status"
}
```

Response:

```json
{
  "state": "idle",
  "active_session": null,
  "controller_reachable": true,
  "freerdp": {
    "available": true,
    "binary": "/usr/bin/xfreerdp3",
    "version": "..."
  },
  "moonlight": {
    "available": true,
    "binary": "/usr/bin/moonlight-qt",
    "version": "..."
  }
}
```

### Cancel

An administrator-facing local cancel operation may be added later.

The local socket must be permission-restricted so only the ThinPi launcher service account can use it.

---

## 13. Launcher UI Requirements

### Login Screen

Must contain:

- ThinPi branding/title.
- Username.
- Password.
- Login button.
- Friendly authentication error.
- Network/controller unavailable indicator.
- Optional shutdown button.

Must not display:

- IP addresses.
- Internal stack traces.
- Controller URLs.
- Linux usernames.
- Shell errors.

### Dashboard

Must show:

- User display name.
- Authorised connection tiles only.
- Connection name.
- Optional icon.
- Optional protocol badge.
- Connection availability state if implemented.
- Logout.
- Optional shutdown/reboot.

Connection tile states:

```text
Available
Starting
Unavailable
Disabled
Error
```

### Remote Session Transition

When launching:

```text
Connecting to School PC…
```

Then native client takes focus fullscreen.

When client exits normally:

```text
Return directly to dashboard.
```

If client fails:

```text
Unable to connect to School PC.
[Try Again] [Back]
```

Do not expose raw command output to normal users.

Detailed errors may be written to the local log.

---

## 14. Administrative Web UI

The controller must expose a simple administrator-only web interface.

Sections:

### Dashboard

Show:

- Devices.
- Online/offline status.
- Recent logins.
- Recent connection launches.
- Failed authentication attempts.
- Recent errors.

### Users

Functions:

- Create.
- Edit.
- Enable/disable.
- Reset password.
- Set/remove administrator.
- Add/remove group memberships.
- View assigned connections.

### Groups

Functions:

- Create.
- Edit.
- Delete.
- Add/remove users.
- Assign/remove connections.

### Connections

Functions:

- Create.
- Edit.
- Enable/disable.
- Set protocol.
- Host.
- Port.
- Display name.
- Icon.
- Protocol options.
- Credential association.
- Assign users/groups.

### Credentials

Functions:

- Create.
- Replace.
- Delete.
- Associate with connection.

Never display stored passwords after creation.

### Devices

Functions:

- Create enrolment token.
- View enrolled Pi devices.
- Rename.
- Revoke.
- View last-seen.
- View selected native client versions.

### Audit

Filterable audit events including:

- Login success/failure.
- Logout.
- Connection launch approved.
- Connection launch denied.
- Launch ticket redeemed.
- Native client exit.
- Device enrolment.
- Device revocation.
- Admin configuration changes.

---

## 15. Controller API

Use `/api/v1`.

Minimum routes:

```text
POST   /api/v1/auth/login
POST   /api/v1/auth/logout
GET    /api/v1/me

GET    /api/v1/connections
POST   /api/v1/connections/{id}/launch

POST   /api/v1/devices/enrol
POST   /api/v1/agent/redeem-launch
POST   /api/v1/agent/heartbeat
POST   /api/v1/agent/session-event
```

Administrative API endpoints may sit under:

```text
/api/v1/admin/*
```

All endpoints must enforce authentication and role checks server-side.

Do not rely on hidden buttons in the admin UI for authorisation.

---

## 16. Connection Availability

For MVP, a connection may simply be displayed if authorised and enabled.

Later enhancement:

Controller or agent can determine basic availability.

RDP:

- TCP reachability to host:port.

Moonlight/Sunshine:

- Host reachability.
- Prefer a supported Moonlight/Sunshine discovery/status method rather than inventing a custom probe.

Do not block dashboard rendering for long availability checks.

Use cached asynchronous status.

---

## 17. Development Modes

The complete system must be developable on a normal Linux workstation without a Raspberry Pi.

### 17.1 Local Controller

Run:

```text
docker compose up
```

Controller available locally.

Use a development database and development encryption key.

Seed test users:

```text
admin
wife
daughter
```

Seed test connections:

```text
Mock School PC
Mock Gaming PC
Mock Admin PC
```

### 17.2 Launcher Development Mode

Support:

```text
THINPI_DEV_MODE=1
THINPI_API_URL=http://127.0.0.1:8080
```

In dev mode, protocol launches can use a mock agent.

Clicking a connection should display a mock session window such as:

```text
MOCK REMOTE SESSION

Protocol: RDP
Connection: School PC
Target ID: <id>

[End Session]
```

The UI must be fully testable without FreeRDP or Moonlight installed.

### 17.3 Agent Development Mode

Provide a mock transport mode.

Example:

```text
THINPI_AGENT_MOCK_CLIENTS=1
```

Instead of launching FreeRDP/Moonlight, run a deterministic dummy process for several seconds and generate normal session lifecycle events.

---

## 18. Testing Requirements

### 18.1 Controller Unit Tests

Cover:

- Password hashing/verification.
- Login rate limiting.
- Session creation.
- Session expiry.
- User disabled handling.
- Direct user ACLs.
- Group ACLs.
- Combined ACL behaviour.
- Connection disabled handling.
- Launch ticket creation.
- Launch ticket expiry.
- One-time redemption.
- Wrong-device redemption.
- Credential encryption/decryption.
- Device revocation.
- Admin permission checks.

### 18.2 API Integration Tests

Test full sequences:

```text
login
→ list connections
→ request launch
→ redeem as device
→ verify ticket cannot be reused
```

Also test denied cases.

### 18.3 Agent Tests

Mock external processes.

Test:

- Binary discovery.
- Safe argument generation.
- Password not placed in argv.
- Process start failure.
- Client non-zero exit.
- Client normal exit.
- Controller unavailable.
- Launch ticket invalid.
- Launch ticket expired.
- Concurrent launch prevention.

Only one interactive remote session should run at once on a Pi by default.

### 18.4 Launcher Tests

Qt tests should cover:

- Login form behaviour.
- Authentication error.
- Connection list rendering.
- No unauthorised connection rendering.
- Launch transition.
- Agent failure.
- Logout.
- Session return.

### 18.5 End-to-End Tests

At minimum:

1. Local mock E2E.
2. Real RDP E2E against a test Windows VM.
3. Real Moonlight E2E against a Sunshine test host.
4. Pi reboot-to-login test.
5. Pi session-exit-to-dashboard test.

---

## 19. Security Requirements

### 19.1 General

- Default deny.
- No arbitrary command execution through controller data.
- No shell interpolation when starting native clients.
- Use direct argv/process APIs.
- Validate hosts, ports, usernames, and protocol options.
- Treat all data from the controller database as potentially malformed.
- TLS required outside explicit development mode.
- Secrets never logged.
- Passwords never placed in URL parameters.
- Passwords never placed in FreeRDP argv where avoidable.
- Session tokens never written to normal logs.
- Device tokens never returned by admin API after enrolment.
- Administrative state-changing operations require CSRF protection if cookie-based sessions are used.

### 19.2 Pi Hardening

Production Pi:

- Dedicated `thinpi` launcher user.
- No login shell for kiosk user.
- Root SSH login disabled.
- SSH key authentication for administrator.
- Password SSH authentication disabled after provisioning.
- Disable unnecessary services.
- No general-purpose browser. The administrator-only controller browser is
  origin-restricted, policy-locked, kiosk-mode, and uses a throwaway profile.
- No conventional desktop environment.
- Disable ordinary VT switching; allow only the controller-ticketed fixed
  administrator maintenance console.
- Disable local shell escape paths.
- Agent Unix socket permissions restricted.
- Device credentials root-readable only.
- Configure automatic UI restart.
- Configure watchdog/restart behaviour.
- Consider Raspberry Pi overlay/read-only filesystem after stable deployment.

### 19.3 Network Policy

Recommended:

```text
Pi → Controller HTTPS          ALLOW
Pi → Assigned RDP networks    ALLOW as required
Pi → Sunshine networks        ALLOW as required
Pi → DNS/NTP                  ALLOW
Pi → Management networks      DENY unless specifically required
```

Where possible enforce network policy at pfSense as well as locally.

Do not rely exclusively on UI ACLs.

### 19.4 Controller Exposure

Controller admin UI should preferably be reachable only from:

- Administrator LAN.
- Tailnet.
- Explicit management network.

It does not need to be Internet-accessible.

The Pi-facing API may be reachable from the Pi VLAN/network.

---

## 20. Logging

### Controller

Structured logs.

Log:

- Request ID.
- Event type.
- User ID.
- Device ID.
- Connection ID.
- Outcome.
- Non-sensitive error.

Never log:

- User passwords.
- RDP passwords.
- Device bearer tokens.
- Session bearer tokens.
- Raw launch tickets.

### Agent

Use journald.

Examples:

```text
thinpi-agent: controller connected
thinpi-agent: selected FreeRDP binary /usr/bin/xfreerdp3
thinpi-agent: launching connection id=42 protocol=rdp
thinpi-agent: session id=... exited code=0
```

### Launcher

Log UI-level state transitions but no secrets.

---

## 21. Deployment Strategy

### Phase A — Local Development

Development machine:

```text
Controller → Docker
Launcher   → Native x86_64 Qt build
Agent      → Native x86_64 Go build in mock mode
```

Target outcome:

```text
login
→ connection dashboard
→ click tile
→ mock native session
→ return to dashboard
```

### Phase B — First Pi Deployment

Use a stock Raspberry Pi OS Lite 64-bit installation.

Provision with:

```text
deploy/pi/provision.sh
```

The script should:

1. Verify supported architecture.
2. Update packages.
3. Install minimum display stack.
4. Install Qt runtime dependencies.
5. Install FreeRDP.
6. Install Moonlight Qt from its official Raspberry Pi repository.
7. Install audio requirements.
8. Create ThinPi system user(s).
9. Install agent.
10. Install launcher.
11. Install systemd units.
12. Configure boot into ThinPi session.
13. Configure permissions.
14. Enrol device.
15. Reboot.

### Phase C — Native Pi Build

During early development it is acceptable to:

```text
rsync source
→ build launcher natively on Pi
→ restart service
```

Provide:

```text
scripts/deploy-pi.sh
```

Expected workflow:

```bash
./scripts/deploy-pi.sh thinpi@thinpi-desk.local
```

It should:

- Build controller/agent locally where architecture-compatible.
- Sync launcher source if native Pi compilation is being used.
- Compile launcher.
- Install binaries/assets.
- Restart ThinPi services.
- Show service status.

### Phase D — Package-Based Deployment

Once stable, replace ad-hoc deployment with ARM64 packages.

Preferred artefacts:

```text
thinpi-agent_<version>_arm64.deb
thinpi-launcher_<version>_arm64.deb
```

Controller should be published as:

```text
container image
and/or
linux amd64/arm64 binary
```

### Phase E — Reproducible Appliance

After the software is stable, create a reproducible Pi provisioning/image process.

Do not begin by building a custom OS image.

First prove application behaviour on stock Raspberry Pi OS Lite.

Later options:

- Automated Raspberry Pi Imager customisation.
- Image build script.
- Ansible.
- `rpi-image-gen`.
- Read-only overlay filesystem.

---

## 22. Systemd Layout

Suggested units:

```text
thinpi-agent.service
thinpi-ui.service
```

Agent starts before UI.

Conceptual:

```ini
[Unit]
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/bin/thinpi-agent
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

UI service should:

- Start Xorg/minimal graphical session.
- Start the ThinPi launcher.
- Restart automatically on crash.
- Not expose a desktop if launcher exits.

If the launcher repeatedly crashes, show a controlled recovery screen or restart rather than dropping to a shell.

---

## 23. Native Client Lifecycle

The agent must track a session state machine:

```text
IDLE
 ↓
REDEEMING_TICKET
 ↓
PREPARING
 ↓
STARTING_CLIENT
 ↓
ACTIVE
 ↓
STOPPING
 ↓
IDLE
```

Failure paths return to:

```text
IDLE
```

The launcher should subscribe/poll agent state and present a clean status.

Do not permit:

```text
ACTIVE → second ACTIVE session
```

unless multi-session support is explicitly added later.

---

## 24. RDP Defaults

Provide sane configurable defaults.

Recommended starting point:

- Fullscreen: enabled.
- Dynamic resolution: enabled where supported.
- Audio playback: enabled.
- Microphone redirection: disabled by default.
- Clipboard: configurable, disabled for locked-down profiles.
- Drive redirection: disabled.
- Printer redirection: disabled.
- Smartcard redirection: disabled.
- Auto-reconnect: enabled.
- Certificate validation: enabled.

A per-connection security profile can later provide:

```text
Locked Down
Standard
Trusted/Admin
```

Do not implement insecure convenience defaults globally.

---

## 25. Moonlight Defaults

Store per-connection settings such as:

```text
host
application
resolution
fps
bitrate
codec preference
hdr
audio
gamepad enabled
```

Do not assume all Pis/displays should use the same stream settings.

Recommended initial target for testing:

```text
1920x1080
60 FPS
hardware decode
wired Ethernet
```

Allow later tuning after measuring actual latency and frame loss.

---

## 26. Sunshine Host Assumptions

ThinPi does not install Sunshine on the remote VM automatically in MVP.

Document host preparation:

1. Install Sunshine using supported upstream package.
2. Ensure VM has suitable GPU acceleration where gaming performance is required.
3. Configure Sunshine.
4. Add a `Desktop` application or desired game/application entries.
5. Pair the ThinPi device administratively.
6. Confirm Moonlight can stream outside ThinPi.
7. Only then create the corresponding ThinPi connection.

For gaming VMs, GPU passthrough/vGPU configuration is outside the ThinPi codebase.

ThinPi documentation should state that poor VM rendering or software-only encoding cannot be fixed by the ThinPi client.

---

## 27. Offline Behaviour

If the controller cannot be reached:

- Show a clear network/controller unavailable screen.
- Do not expose cached remote credentials.
- Do not permit new remote launches by default.
- Provide retry.
- Permit shutdown/reboot if configured.

Future enhancement may support cryptographically protected cached assignments, but this is not required for MVP.

---

## 28. Updates

Initial update model:

```text
administrator SSHs to Pi
→ deploy/update script
```

Production update model:

```text
signed package repository or controlled package delivery
```

Do not implement unattended auto-updates until rollback is solved.

Controller should display client versions reported by each device.

---

## 29. Failure Recovery

Handle:

### Controller unavailable

- Login screen shows service unavailable.
- Automatic periodic retry.
- No crash loop.

### Agent unavailable

- Launcher shows local service error.
- systemd restarts agent.

### FreeRDP missing

- Connection tile may show unavailable.
- Admin-visible health warning.

### Moonlight missing

- Moonlight connections unavailable.
- RDP remains functional.

### Remote VM offline

- Display clean connection error.
- Return to dashboard.

### Client crashes

- Agent records exit.
- Launcher returns to dashboard.
- No shell is exposed.

### Pi loses power

- Device must boot cleanly back to ThinPi.
- Later enable read-only/overlay filesystem to reduce SD corruption risk.

---

## 30. Development Milestones

### Milestone 0 — Repository and Tooling

Deliver:

- Monorepo.
- README.
- Build instructions.
- Controller skeleton.
- Agent skeleton.
- Qt launcher skeleton.
- Formatting/linting.
- CI.
- Architecture documentation.

Acceptance:

```text
make test
make build
```

works on supported developer Linux host.

---

### Milestone 1 — Controller Authentication and ACLs

Deliver:

- SQLite migrations.
- Users.
- Password hashing.
- Login/logout.
- Groups.
- Connections.
- User/group permissions.
- `/api/v1/connections`.
- Admin bootstrap account.
- Unit tests.

Acceptance:

- Daughter account receives only daughter-assigned connections.
- Wife receives only wife-assigned connections.
- Admin can receive all assigned/admin connections.
- Disabled user cannot log in.

---

### Milestone 2 — Launcher MVP

Deliver:

- Qt/QML login.
- Controller authentication.
- Dashboard.
- Connection tiles.
- Logout.
- Dev-mode mock session.

Acceptance:

```text
launch launcher
→ log in
→ assigned tiles render
→ click tile
→ mock session
→ close mock session
→ dashboard returns
```

No Pi required.

---

### Milestone 3 — Agent and Launch Tickets

Deliver:

- Unix socket service.
- Device enrolment.
- Device authentication.
- Launch-ticket creation.
- Launch-ticket redemption.
- Single-use tickets.
- Mock protocol process.
- Session lifecycle.

Acceptance:

- Launcher cannot start an arbitrary binary.
- Expired ticket fails.
- Reused ticket fails.
- Ticket created for another device fails.
- Valid ticket launches mock process and returns.

---

### Milestone 4 — Real FreeRDP

Deliver:

- FreeRDP binary detection.
- Structured argument builder.
- Secure password delivery.
- Fullscreen launch.
- Process lifecycle.
- RDP error handling.

Acceptance on Linux developer workstation:

```text
ThinPi login
→ click Windows VM
→ FreeRDP opens fullscreen
→ authenticate/connect
→ exit FreeRDP
→ ThinPi dashboard returns
```

Verify no RDP password is visible in:

```text
ps
/proc/<pid>/cmdline
controller logs
agent logs
launcher logs
```

---

### Milestone 5 — First Raspberry Pi Appliance

Deliver:

- Raspberry Pi OS Lite provisioning.
- Minimal display stack.
- Autostart.
- No conventional desktop.
- Agent.
- Launcher.
- Real RDP.

Acceptance:

```text
power on Pi
→ ThinPi login visible
→ no Linux desktop appears
→ log in
→ start RDP
→ usable remote desktop
→ exit
→ dashboard returns
```

Reboot test must pass repeatedly.

---

### Milestone 6 — Moonlight/Sunshine

Deliver:

- Moonlight detection.
- Moonlight connection type.
- Direct stream to configured host/application.
- Pairing documentation.
- Session lifecycle.
- Controller/gamepad testing.

Acceptance:

```text
ThinPi login
→ Gaming PC
→ direct Moonlight stream
→ no host/application picker
→ controller works
→ audio works
→ terminate stream
→ dashboard returns
```

Benchmark 1080p60 over wired Ethernet.

---

### Milestone 7 — Administrator Web UI

Deliver all required CRUD functions for:

- Users.
- Groups.
- Connections.
- Credentials.
- Devices.
- ACL assignments.
- Audit events.

Acceptance:

An administrator can create a new daughter test account, assign one RDP VM and one Moonlight VM, then the Pi immediately reflects those assignments on next login/refresh without editing the Pi.

---

### Milestone 8 — Security Hardening

Deliver:

- TLS-only production mode.
- Argon2id parameters documented.
- Login rate limiting.
- Secret encryption.
- SSH hardening.
- Systemd hardening.
- Device revocation.
- CSRF protection.
- Input validation.
- Security tests.
- No arbitrary protocol arguments.
- Log redaction.
- Restricted service accounts.

Perform a basic threat-model review and document findings.

---

### Milestone 9 — Packaging and Reproducible Deployment

Deliver:

- `.deb` packages for launcher and agent.
- Versioning.
- Controller container.
- Automated Pi provision/update script.
- Rollback documentation.
- Reproducible setup instructions.

---

### Milestone 10 — Performance Optimisation

Benchmark:

- RDP responsiveness.
- Moonlight 1080p60.
- Moonlight direct TTY vs graphical session.
- Input latency.
- Decode latency.
- CPU.
- GPU.
- Memory.
- Network throughput.
- Audio stability.

If direct-console Moonlight is materially better, implement automatic UI handoff.

Do not optimise based on assumption. Record measurements.

---

## 31. Acceptance Criteria for Version 1.0

Version 1.0 is complete only when all of the following are true.

### Boot

- Pi boots directly to ThinPi.
- No conventional desktop is exposed.
- No local terminal is exposed during normal use; assigned SSH sessions reach
  only the remote host.
- Launcher automatically restarts after crash.

### Authentication

- Multiple users supported.
- Passwords securely hashed.
- Disabled accounts cannot log in.
- Logout works.

### ACLs

- Each user sees only explicitly authorised connections.
- ACL enforcement occurs on controller.
- Agent cannot redeem unauthorised launch tickets.
- Admin can modify assignments centrally.

### RDP

- Native FreeRDP session.
- Fullscreen.
- Audio works.
- Keyboard/mouse work.
- Session returns to dashboard.
- Credential is not exposed in command-line arguments.

### Moonlight

- Native Moonlight/Sunshine stream.
- Direct configured host/application launch.
- Hardware decode active.
- Gamepad works.
- Audio works.
- 1080p60 stable on wired LAN under expected household conditions.
- Session returns to dashboard.

### Administration

- Web UI manages users.
- Web UI manages groups.
- Web UI manages connections.
- Web UI manages ACLs.
- Web UI manages devices.
- Remote credentials can be securely stored.
- Audit events visible.

### Deployment

- Fresh Pi can be provisioned using documented automated steps.
- Controller can be deployed in Proxmox.
- Client update process documented.
- Full rebuild does not require manually recreating ACL/configuration on the Pi.

---

## 32. Non-Goals for Version 1.0

Do not expand scope into:

- Browser-based remote desktops.
- Guacamole integration.
- Reimplementation of RDP.
- Reimplementation of GameStream.
- Cloud multi-tenancy.
- Public SaaS hosting.
- Internet-facing controller.
- Active Directory integration.
- Kubernetes.
- Mobile app.
- Full MDM.
- Arbitrary application launcher.
- General-purpose Pi desktop.
- VM creation or Proxmox lifecycle management.
- Automatic GPU passthrough configuration.
- Complex HA controller deployment.

These can be considered later.

---

## 33. Coding Standards

### Go

- `gofmt`.
- `go vet`.
- Static analysis in CI.
- Clear package boundaries.
- Context-aware HTTP operations.
- Timeouts on outbound HTTP clients.
- No global mutable state where avoidable.
- Table-driven tests.

### C++/Qt

- C++20.
- RAII.
- Avoid raw owning pointers.
- Separate QML presentation from controller/API logic.
- Avoid blocking the UI thread.
- Network requests asynchronous.
- Qt logging categories.
- Unit-test backend logic independently of QML where possible.

### General

- No secrets committed.
- `.env.example`, never real `.env`.
- Migrations committed.
- Every security-sensitive behaviour tested.
- Comments explain why, not obvious syntax.
- Use Australian English in user-facing text/documentation.

---

## 34. API Error Model

Use consistent JSON errors.

Example:

```json
{
  "error": {
    "code": "CONNECTION_NOT_AUTHORISED",
    "message": "You are not authorised to launch this connection.",
    "request_id": "..."
  }
}
```

Normal users should receive safe messages.

Detailed diagnostics belong in logs.

---

## 35. Configuration

Controller example:

```yaml
server:
  listen: "0.0.0.0:8443"
  public_url: "https://thinpi.internal"

database:
  path: "/var/lib/thinpi/thinpi.db"

security:
  master_key_file: "/run/secrets/thinpi_master_key"
  session_idle_timeout: "30m"
  launch_ticket_ttl: "30s"

logging:
  level: "info"
```

Agent example:

```yaml
controller_url: "https://thinpi.internal"
device_file: "/etc/thinpi/device.json"
socket: "/run/thinpi/agent.sock"

clients:
  freerdp:
    binary: "auto"
  moonlight:
    binary: "auto"
```

Launcher example:

```yaml
agent_socket: "/run/thinpi/agent.sock"
controller_url: "https://thinpi.internal"
fullscreen: true
```

Do not duplicate secrets across configuration files.

---

## 36. Local Developer Workflow

Expected workflow:

```bash
git clone <repo>
cd thinpi

make dev-controller
make dev-agent
make dev-launcher
```

or:

```bash
docker compose up controller
./bin/thinpi-agent --mock
./build/thinpi-launcher
```

A developer should be able to exercise the complete login/ACL/launch flow without a Pi.

Add seed tooling:

```bash
make seed-dev
```

Add reset tooling:

```bash
make reset-dev
```

---

## 37. Pi Developer Workflow

Initial iteration:

```bash
./scripts/deploy-pi.sh thinpi@<pi-address>
```

The script should:

1. Build/sync code.
2. Install changed artefacts.
3. Restart services.
4. Print:
   - `systemctl status thinpi-agent`
   - `systemctl status thinpi-ui`
5. Fail non-zero if deployment fails.

Provide optional:

```bash
./scripts/pi-logs.sh thinpi@<pi-address>
```

to tail relevant journal logs.

---

## 38. Controller Deployment to Proxmox

Initial recommended controller deployment:

```text
Small Debian/Ubuntu VM or LXC
    ↓
Docker/Podman
    ↓
ThinPi Controller
```

Suggested resources:

```text
1–2 vCPU
512 MB–1 GB RAM
small persistent disk
```

Exact resource usage should be measured.

Persistent volumes:

```text
/var/lib/thinpi
/run/secrets or equivalent
```

Back up:

- SQLite database.
- Master encryption key separately.
- TLS key/certificates.
- Configuration.

The master key must not be stored only inside a disposable container layer.

---

## 39. Network Integration

ThinPi should work whether the Pi reaches the VM subnet directly or through existing routing.

Do not hardcode a specific subnet.

Connection host values can be:

- IP address.
- Internal DNS name.
- Tailnet DNS name if intentionally used.

Prefer direct LAN routing for low-latency Moonlight where available.

Do not route gaming traffic through browser gateways or application proxies.

---

## 40. Performance Targets

These are goals, not synthetic guarantees.

### UI

- Login screen interactive immediately after display starts.
- Dashboard connection list appears quickly on LAN.
- No animation-heavy UI.
- Low idle CPU usage.

### RDP

- Normal desktop interaction should feel comparable to running FreeRDP manually on the same Pi.
- ThinPi must add effectively no latency to an active RDP session after launch.

### Moonlight

ThinPi must add no realtime translation layer.

Data path:

```text
VM GPU
→ Sunshine encode
→ LAN
→ Moonlight hardware decode
→ HDMI
```

ThinPi controller is not in the media path.

---

## 41. Threat Model Summary

Threats to consider:

### Malicious or curious local child user

Goals:

- Escape launcher.
- Open terminal.
- Launch unassigned VM.
- Alter local config.
- Recover stored remote passwords.

Controls:

- No desktop.
- No shell.
- Restricted kiosk user.
- Local agent controls launches.
- Server-side ACL.
- Launch tickets.
- Remote credentials never delivered to launcher.
- Filesystem hardening.
- Network policy.

### Stolen Pi/SD card

Controls:

- No reusable user passwords stored.
- Device credential revocable.
- Remote credentials stored centrally, not on SD.
- Root-only device credential.
- Future optional full-disk/secret hardware protection if required.

### Compromised normal ThinPi account

Controls:

- Only assigned connections.
- No admin endpoints.
- Rate limits.
- Server-side ACL.
- Session expiry.
- Audit trail.

### Compromised controller

Impact is high.

Controls:

- Restricted network exposure.
- Regular patching.
- Secret encryption.
- Backups.
- Strong admin authentication.
- Minimal dependencies.
- Logs/audit.
- Future optional MFA.

---

## 42. Codex Working Instructions

Implement incrementally.

For each milestone:

1. Read this document.
2. Inspect existing repository state.
3. Do not silently replace architectural decisions.
4. If a decision must change, create an ADR under `docs/decisions/`.
5. Implement the smallest complete slice.
6. Add tests with the implementation.
7. Run all relevant tests.
8. Update documentation.
9. Ensure no secrets or local deployment credentials are committed.
10. Commit logical changes separately where repository workflow permits.

When uncertain about current third-party command syntax:

- Consult current upstream documentation/source.
- Prefer inspecting the installed binary's `--help`, `/help`, or equivalent.
- Do not copy stale command examples without verification.
- Keep third-party client invocation isolated behind protocol adapters.

Do not attempt to solve protocol problems by adding a browser/Guacamole layer.

The primary design principle is:

> **ThinPi controls access and launches native clients; it does not sit in the media/desktop data path.**

---

## 43. Questions Codex Should Resolve During Implementation

These are implementation questions, not reasons to stop development.

Codex should make a sensible default, document it, and continue.

1. Exact minimal Xorg/window-manager package set on the selected Raspberry Pi OS release.
2. Exact FreeRDP executable name packaged by that release.
3. Exact FreeRDP argument syntax for the installed version.
4. Exact Moonlight Raspberry Pi package/version and command-line syntax.
5. Whether Moonlight inside minimal Xorg has acceptable latency versus direct TTY.
6. Best Qt platform integration for the Pi image.
7. Whether PulseAudio or the current Raspberry Pi OS audio stack should be used for the selected image.
8. Best `.deb` build mechanism for the final project.
9. Whether SQLite WAL mode provides operational benefit for the controller.
10. Exact systemd hardening flags that remain compatible with FreeRDP/Moonlight input/video access.

Resolve these experimentally and record the result.

---

## 44. Upstream References

The implementation should verify current upstream documentation at development time.

### FreeRDP

- Project: https://github.com/FreeRDP/FreeRDP
- Wiki: https://github.com/FreeRDP/FreeRDP/wiki
- Command-line documentation notes that the application's own help output should be treated as authoritative when wiki content is stale.

As of this plan's creation, FreeRDP 3.x is current and `/args-from` functionality is available for supplying command arguments via a file/stdin/file descriptor. Verify against the installed client before finalising the adapter.

### Moonlight Qt

- Project: https://github.com/moonlight-stream/moonlight-qt
- Raspberry Pi documentation: https://github.com/moonlight-stream/moonlight-docs/wiki/Installing-Moonlight-Qt-on-Raspberry-Pi-4

Current upstream Raspberry Pi guidance supports Raspberry Pi 4 or later and Raspberry Pi OS Bookworm or later. Upstream recommends running directly from console/TTY for best performance.

### Sunshine

- Documentation: https://docs.lizardbyte.dev/projects/sunshine/latest/

Sunshine is the intended self-hosted streaming host for Moonlight and supports hardware encoding on supported AMD, Intel, and NVIDIA GPUs.

### Raspberry Pi

- Kiosk guidance: https://www.raspberrypi.com/tutorials/how-to-use-a-raspberry-pi-in-kiosk-mode/

Raspberry Pi documents booting directly into a fullscreen application without requiring a conventional desktop environment and also documents an overlay/read-only filesystem option useful for appliance deployments.

### Qt

- Embedded Linux: https://doc.qt.io/qt-6/embedded-linux.html
- Embedded device configuration: https://doc.qt.io/qt-6/configure-linux-device.html

---

## 45. Final Deliverable

The finished project should allow the following real-world workflow:

```text
Administrator:
    opens ThinPi admin UI
    creates daughter account
    creates School PC RDP connection
    creates Gaming PC Moonlight connection
    assigns both connections to daughter
    closes admin UI

Daughter:
    sits at Raspberry Pi
    powers it on
    sees ThinPi login
    signs in
    sees only:
        School PC
        Gaming PC

    selects School PC
        → native FreeRDP session
        → exits
        → returns to ThinPi

    selects Gaming PC
        → native Moonlight stream
        → plays with low latency
        → exits
        → returns to ThinPi

Daughter:
    cannot see Brad's VMs
    cannot change connection definitions
    cannot open a Linux desktop
    cannot open a terminal
    cannot retrieve stored RDP credentials

Administrator:
    can add/remove users, VMs, permissions and Pi devices centrally
    without editing the Pi manually
```

That is the required Version 1.0 product.
