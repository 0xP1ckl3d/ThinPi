# Troubleshooting

Start at the failing boundary. Do not disable TLS, certificate checking,
authentication, or kiosk hardening to make a symptom disappear.

## Admin browser

### `/admin` shows login or session expired

This is expected for a missing, expired, disabled, or demoted administrator
session. Sign in again at `/admin/login`. The server clears an invalid browser
cookie and redirects automatically.

Expected navigation:

| State | `/` | `/admin/login` | `/admin` |
|---|---|---|---|
| Signed out/expired | login | login | redirects to login |
| Signed-in administrator | redirects to admin | redirects to admin | admin console |

If the dashboard was open when the session expired, the first 401 admin API
response redirects the page to login.

### Admin console loads but tables are empty or shows API errors

Open browser developer tools and inspect the failing `/api/v1/admin/...`
request. On the controller:

```sh
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml logs --tail 200 controller
```

401 means reauthenticate. 403 `ADMIN_REQUIRED` means the account is no longer
an enabled administrator. 403 `CSRF_INVALID` usually means an old page was left
open across login/restart; reload it.

## Controller

### Container is unhealthy

```sh
cd /opt/thinpi
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml ps
docker compose --env-file deploy/controller/.env \
  -f deploy/controller/compose.yml logs --tail 200 controller
```

Common causes:

- TLS files or master key are missing;
- UID/GID 65532 cannot traverse/read the mounted path;
- port 8443 is already bound;
- SQLite volume ownership is wrong;
- certificate/key pair does not match.

Validate material:

```sh
sudo openssl pkey -in deploy/controller/tls/tls.key -check -noout
sudo openssl x509 -in deploy/controller/tls/tls.crt -noout -subject -dates
ss -lntp | grep 8443
```

### Browser/Pi reports certificate failure

Check all four:

1. `timedatectl status` on controller and Pi;
2. certificate SAN contains the exact controller hostname;
3. server sends the issuing chain;
4. private CA is installed in the Pi system trust store and referenced by the
   agent.

```sh
openssl s_client -connect thinpi.home.example:8443 \
  -servername thinpi.home.example -showcerts </dev/null
curl --cacert thinpi-ca.crt https://thinpi.home.example:8443/healthz
```

Never solve this with `--insecure` or a global FreeRDP certificate-ignore flag.

### Database works but stored credentials fail

The mounted master key does not match the key that encrypted the database, or
the encrypted row is damaged. Restore the database and its matching master key
as a pair. Generating a new key does not recover existing secrets.

## Client services and launcher

### Client boots to a console or blank screen

Connect over SSH:

```sh
systemctl get-default
systemctl status thinpi-agent thinpi-ui --no-pager --full
sudo journalctl -b -u thinpi-agent -u thinpi-ui --no-pager
```

Expected default target is `thinpi.target`. Verify `/usr/bin/thinpi-launcher`,
`/usr/local/libexec/thinpi-xinitrc`, Xorg packages, and tty7 availability. The
startup script must not be placed under the private `0750` `/etc/thinpi`
directory because the non-login kiosk identity cannot traverse it.

If Xorg reports `Only console users are allowed to run the X server`, the
distribution-generated `/etc/X11/Xwrapper.config` is still installed. The
ThinPi-managed file uses `allowed_users=anybody` because the kiosk starts from
a systemd/PAM session rather than an interactive getty. This does not provide
a login shell or sudo access to `thinpi`; the nologin account, masked getties,
Xorg escape-key restrictions, and systemd service boundary remain in force.
Pull the current repository and rerun `scripts/deploy-client.sh` to replace stale
ThinPi Chromium URL policy with the path-specific admin policy.

### Launcher cannot reach controller but agent can

The agent may trust `/etc/thinpi/controller-ca.pem` while Qt lacks system trust.
Check:

```sh
cat /etc/thinpi/ui.env
ls -l /usr/local/share/ca-certificates/thinpi-controller.crt
sudo update-ca-certificates
sudo systemctl restart thinpi-ui
```

Both `/etc/thinpi/agent.json` and `/etc/thinpi/ui.env` must use the same exact
controller origin.

### Launcher says local ThinPi service unavailable

```sh
systemctl status thinpi-agent --no-pager --full
sudo ls -l /run/thinpi/agent.sock
id thinpi
sudo journalctl -u thinpi-agent --since '10 minutes ago' --no-pager
```

If the log repeatedly reports that `chgrp` cannot find
`/run/thinpi/agent.sock`, an obsolete agent unit with a socket-startup race is
installed. Pull the current repository and rerun `scripts/deploy-client.sh`.
The current unit runs the root agent with primary group `thinpi`, so the agent
creates the mode `0660` socket as `root:thinpi`; its post-start check only waits
for the socket before allowing the UI to start. No new enrolment token is
needed.

The socket should be group `thinpi` with mode `0660`, and the agent must remain
active.

### Device does not appear online

```sh
sudo /usr/bin/thinpi-agent status --config /etc/thinpi/agent.json
sudo journalctl -u thinpi-agent --since '10 minutes ago' --no-pager
curl https://thinpi.home.example:8443/healthz
```

Check device revocation, `/etc/thinpi/device.json` mode/contents, DNS, NTP, TLS,
and firewall TCP 8443.

## Native sessions

### Locked SSH session refuses to start

Check the installed single-purpose stack and target reachability from the Pi:

```sh
command -v ssh xterm sshpass
nc -vz SSH_TARGET 22
sudo journalctl -u thinpi-agent --since '10 minutes ago' --no-pager
```

ThinPi records the remote host key automatically on first use. If it changes,
the dashboard shows the replacement fingerprint and asks whether to trust it.
Verify an unexpected change through a trusted console before accepting it. Do
not use `StrictHostKeyChecking=no`.

The SSH username and password/private key come from the assigned credential.
The secret uses an owner-only temporary file and never appears in process
arguments. Exiting the remote shell should close xterm and return to the launcher.

### Local maintenance is unavailable

Local maintenance requires all of the following:

- the launcher user is currently an enabled controller Administrator;
- the Pi device is enabled and has its device credential;
- `/etc/thinpi/agent.json` has a non-root `maintenance_user`;
- `/usr/local/libexec/thinpi-maintenance-session` and `chvt` exist;
- the controller is reachable while the one-use ticket is redeemed.

Check remotely:

```sh
sudo jq .maintenance_user /etc/thinpi/agent.json
command -v chvt
sudo test -x /usr/local/libexec/thinpi-maintenance-session && echo present
sudo journalctl -u thinpi-agent --since '10 minutes ago' --no-pager
```

The launcher signs out before switching consoles. Run `exit` in maintenance to
return to virtual terminal 7. Never unmask permanent getties as a workaround.

### A keyboard shortcut leaves the kiosk

This is a deployment failure. Check the installed Xorg policy and default
target:

```sh
cat /etc/X11/xorg.conf.d/10-thinpi-kiosk.conf
systemctl get-default
systemctl is-enabled getty@tty1.service getty@tty2.service getty@tty7.service
getent passwd thinpi
```

The Xorg file must set `DontVTSwitch`, `DontZap`, and `DontZoom`; default must
be `thinpi.target`; getties must be masked; and `thinpi` must use
`/usr/sbin/nologin`. Re-run the current provisioner if any are missing.

Windows/Command+L is the intentional exception: it must disconnect any active
native session and sign out to the ThinPi login screen. If it does nothing,
check and redeploy the shortcut configuration:

```sh
command -v xbindkeys
cat /usr/local/libexec/thinpi-xbindkeysrc
pgrep -a xbindkeys
sudo journalctl -b -u thinpi-ui --no-pager
```

### Display does not sleep or sleeps during a remote session

```sh
sudo -u thinpi env DISPLAY=:0 XAUTHORITY=/home/thinpi/.Xauthority xset q
sudo journalctl -b -u thinpi-ui --no-pager
```

Check **Kiosk settings** in the controller administration application. The
timeout must be an integer from `0` through `1440`; `0` disables sleep. Clients
poll this controller setting every minute. ThinPi configures Xorg DPMS while the
login or dashboard is visible and disables DPMS while a native session is
connecting or active.

### Clipboard, profile photo, palette or shell theme does not update

Confirm the controller is returning the current kiosk configuration:

```sh
curl -fsS https://controller.example/api/v1/login-users | jq .configuration
sudo journalctl -b -u thinpi-ui -u thinpi-agent --no-pager
```

The launcher checks the controller every minute. RDP and VNC clipboard
redirection is enforced by ThinPi, while SSH uses the Xorg text clipboard.
Sign-out intentionally clears it, including before the kiosk switches to the
protected local maintenance console. Profile photos must be PNG,
JPEG or WebP; the admin browser resizes them before upload. If keyboard focus is
unclear, press Tab: the focused control receives the selected palette's accent
outline, and Enter or Space activates it.

Moonlight is the protocol exception: Ctrl+Alt+Shift+V types the ThinPi clipboard
into the Sunshine host, but GameStream does not provide a host-to-client
clipboard channel. Copying content out of a Moonlight host therefore cannot be
made part of the shared clipboard without separate software on that host.

### FreeRDP unavailable or exits immediately

```sh
command -v xfreerdp3 || command -v xfreerdp
xfreerdp3 /help 2>/dev/null | head
sudo journalctl -u thinpi-agent --since '10 minutes ago' --no-pager
```

The installed client must support `/args-from:stdin`. In the connection editor,
set **RDP server certificate** to **Trust on first connection, then pin** for a
self-signed host; otherwise FreeRDP cannot ask a question inside the kiosk. Also
check target DNS/port, username format, and whether RDP is enabled at the host.

Verify the native client can access the kiosk X display:

```sh
sudo -u thinpi env DISPLAY=:0 xset q >/dev/null && echo display-ok
```

Current agents turn common display, credential, certificate, TLS and transport
failures into specific launcher messages and record the safe category in the
agent journal.

### VNC unavailable

```sh
command -v xtigervncviewer
command -v vncpasswd
nc -vz linux-host.home.example 5900
```

ThinPi expects TigerVNC Viewer/tools. Confirm the server authentication mode is
compatible and the assigned credential is correct.

### Moonlight unavailable, unpaired, or no audio/gamepad

```sh
command -v moonlight-qt
moonlight-qt --help
id thinpi
journalctl -b -u thinpi-agent -o cat --no-pager | tail -100
```

On amd64, `command -v` should return `/usr/local/bin/moonlight-qt`; the official
AppImage is extracted below `/opt/thinpi/moonlight`. On Raspberry Pi, use
`raspi-config` to select the audio output and confirm PulseAudio. On
Ubuntu/Lubuntu, confirm PipeWire and `pipewire-pulse` instead. In both cases,
the `thinpi` user must belong to the input, audio, video, and render groups.

If the host is unpaired, assign a password credential containing the Sunshine
Web UI administrator username and password to the connection. ThinPi performs
pairing automatically and stores it for the `thinpi` identity. Check that the
Sunshine Web UI is reachable on port `47990` (or the configured
`sunshine_api_port`) and that Sunshine permits PIN submission from the ThinPi
network. Do not run `moonlight-qt pair` manually.

### Target works from controller but not the client

This is expected when only the controller site has a route. The controller is
not a desktop proxy.

```sh
getent hosts target.home.example
ip route get TARGET_IP
ping -c 3 TARGET_IP
nc -vz TARGET_IP 3389
```

Configure the external routed VPN in [networking.md](networking.md) or make the
target directly reachable from the client.

### Routed session is laggy

Compare direct and routed path RTT/loss/MTU and gateway load. Moonlight is more
sensitive than RDP/VNC:

```sh
ping -c 20 TARGET_IP
tracepath TARGET_IP
vcgencmd measure_temp
vcgencmd get_throttled
```

Prefer a direct LAN route where available and complete
[hardware-validation.md](hardware-validation.md).

## Recovery and support bundle

Temporarily leave kiosk mode through SSH:

```sh
sudo systemctl isolate multi-user.target
```

Collect a hardware/service record:

```sh
sudo /tmp/thinpi/benchmark-pi.sh /tmp/thinpi-support.txt
```

Do not include `/etc/thinpi/device.json`, controller master keys, enrolment
tokens, remote passwords, or private TLS keys in support logs.
