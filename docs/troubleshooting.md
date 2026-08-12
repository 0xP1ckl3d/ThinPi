# Troubleshooting

Start at the failing boundary. Do not disable TLS, certificate checking,
authentication, or kiosk hardening to make a symptom disappear.

## Local Windows environment

### `dev-up.ps1` cannot connect to Docker

1. Start Docker Desktop and wait for the engine to report running.
2. Run:

   ```powershell
   docker version
   docker compose version
   ```

3. Restart with:

   ```powershell
   .\scripts\dev-down.ps1
   .\scripts\dev-up.ps1
   ```

The development scripts use a repository-local Docker CLI configuration under
`.docker-config` to avoid unrelated user-config permission failures.

### Qt, CMake, Ninja, or MinGW is missing

Install the Qt 6.5+ MinGW 64-bit kit plus Qt's CMake, Ninja, and MinGW tools.
Normal installations under `C:\Qt` are discovered automatically. For a custom
location:

```powershell
$env:QT_ROOT = 'D:\Qt\6.10.1\mingw_64'
.\scripts\dev-up.ps1
```

### Local agent/named-pipe failure

```powershell
.\scripts\dev-status.ps1
Get-Content .\.thinpi-dev\agent.err.log
Get-Content .\.thinpi-dev\agent.out.log
```

Then perform a clean process restart without deleting data:

```powershell
.\scripts\dev-down.ps1
.\scripts\dev-up.ps1
```

### Old development users or stale UI

Hard-refresh the browser after rebuilding. To remove the persistent test
database:

```powershell
.\scripts\dev-down.ps1 -ResetData
.\scripts\dev-up.ps1
```

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
`/etc/thinpi/xinitrc`, Xorg packages, and tty7 availability.

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

The connection's Advanced settings must contain the verified remote host public
key as `host_key`, for example `ssh-ed25519 AAAA...`. A missing key fails
closed. If the server host key changed, verify the new fingerprint through a
trusted console before updating ThinPi. Do not use `StrictHostKeyChecking=no`.

The SSH username comes from the assigned credential. The password uses an
owner-only temporary file and never appears in process arguments. Exiting the
remote shell should close xterm and return to the launcher.

### Local maintenance is unavailable

Local maintenance requires all of the following:

- the launcher user is currently an enabled controller Administrator;
- the Pi device is enabled and has its device credential;
- `/etc/thinpi/agent.json` has a non-root `maintenance_user`;
- `/usr/local/libexec/thinpi-maintenance-session` and `openvt` exist;
- the controller is reachable while the one-use ticket is redeemed.

Check remotely:

```sh
sudo jq .maintenance_user /etc/thinpi/agent.json
command -v openvt
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

### FreeRDP unavailable or exits immediately

```sh
command -v xfreerdp3 || command -v xfreerdp
xfreerdp3 /help 2>/dev/null | head
sudo journalctl -u thinpi-agent --since '10 minutes ago' --no-pager
```

The installed client must support `/args-from:stdin`. Check target DNS/port,
remote certificate name, username format, and whether RDP is enabled at the
host.

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
sudo -u thinpi env HOME=/home/thinpi DISPLAY=:0 \
  XAUTHORITY=/home/thinpi/.Xauthority moonlight-qt pair sunshine-host
id thinpi
```

The pairing must exist for the `thinpi` identity. Use `raspi-config` to select
the audio output, confirm PulseAudio, and ensure `thinpi` belongs to input,
audio, video, and render groups.

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
