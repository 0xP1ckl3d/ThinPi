# Security

## Implemented controls

- Passwords use Argon2id with a unique 16-byte salt, 64 MiB memory, three
  iterations, and parallelism two. Login is limited to eight failures per
  username/source in ten minutes and uses a generic failure response.
- User sessions, device credentials, enrolment tokens, and launch tickets use
  cryptographically random opaque values. Only SHA-256 token hashes are stored.
- Remote passwords use AES-256-GCM with a random nonce and associated format
  data. The 32-byte master key is outside SQLite.
- ACL and access-policy checks are server-side. Disabled users/devices/connections,
  disallowed schedules, and exhausted daily allowances are denied.
- Launch tickets are short-lived, device-bound, and atomically marked used.
- Launcher-to-admin browser handoffs are administrator-only, expire after 45
  seconds, are stored hashed, and are atomically redeemed once into a separate
  HttpOnly browser session.
- The launcher never receives reusable connection secrets or the device token.
- Protocol fields are validated and translated to direct process arguments.
  There is no shell interpolation and no `extra_args` field.
- FreeRDP secrets use `/args-from:stdin`. RDP certificate handling is explicit
  and non-interactive: TOFU is the default, strict trust is available, and the
  deliberately labelled unsafe ignore mode requires an administrator choice.
- VNC passwords use an ephemeral owner-only TigerVNC password file and are
  never placed in the process arguments. The file is removed after the viewer exits.
- SSH uses xterm only as a single-child display for OpenSSH. There are no tabs
  or local shell child. Host keys are mandatory and pinned; user config,
  escape/local commands, proxying and every forwarding mode are disabled.
  Password or private-key material uses an ephemeral owner-only file and never
  appears in argv.
- The kiosk account has `nologin`, no sudo rights, no display manager and no
  getty. Xorg blocks VT switching, server termination and mode-switch key
  sequences. The Xorg wrapper permits service launch by a non-console identity,
  but does not grant a shell, sudo, or additional application capability;
  systemd restarts the launcher if it exits.
- Client provisioning preserves the host's existing administrator SSH
  password-authentication setting. Root login and forwarding remain disabled;
  key-only SSH is available only through `--disable-ssh-passwords`.
- The native admin browser runs only after an administrator handoff, maximized
  with a throwaway profile and visible controls so the administrator can close it.
  Browser navigation is not treated as a security boundary: anyone authorized to open it is
  already a ThinPi administrator with access to the separately audited local
  maintenance path.
  Its **Return to ThinPi** control signs out and closes that browser process.
- Local maintenance requires a short-lived single-use controller ticket bound
  to the administrator and device. The root agent can only switch to the fixed
  preconfigured maintenance account; the local API cannot carry commands.
- Browser admin mutations have CSRF protection. Responses set CSP,
  `X-Frame-Options`, no-sniff, and referrer headers.
- Mock clients and the mock protocol are restricted to an HTTP controller on a
  loopback address; production APIs reject mock connections.
- Services use dedicated filesystem paths, no-new-privileges, private temporary
  directories, restricted address families, and read-only system paths.

## Master-key backup and rotation

Generate once with `thinpi-controller generate-master-key --out master.key`.
Store a protected offline backup separately from database backups. Production
mode refuses to start without the key and TLS certificate/key. Loss of the key
makes stored remote credentials unrecoverable; users, ACLs, and connections
remain recoverable.

Rotation requires decrypting each credential using the old key and re-encrypting
under the new key. This release intentionally provides no unsafe blind rotation
command. Back up the database and both keys before a controlled migration.

## Threat review

| Threat | Main controls | Residual risk/action |
|---|---|---|
| Curious local user escapes UI | no desktop/shell, kiosk account, agent allow-list | physically accessible boot media still needs bootloader/SD controls |
| User launches unassigned VM | list/create/redeem ACL checks | firewall should also restrict client VLAN destinations |
| Stolen client storage | no user/remote passwords, revocable device token | bearer device token can be used until revoked; consider encrypted storage/TPM |
| Process-list credential theft | FreeRDP stdin only, log redaction | validate installed FreeRDP `/args-from:stdin` during provisioning |
| Controller compromise | network restriction, TLS, encrypted secrets, minimal stack | high impact; patch, back up, and consider admin MFA in a later release |
| Malicious admin field | structured schema, host/range validation, no arbitrary args | Moonlight settings not supported by installed CLI remain ignored safely |

Place the controller admin UI only on an administrator LAN, tailnet, or explicit
management network. Do not publish it to the Internet.

The remote-SSH/local-maintenance separation is recorded in
[ADR-005](decisions/ADR-005-kiosk-ssh-and-maintenance.md).

If the controller host also operates as a subnet router, keep that routing
service outside the controller container. Do not add `NET_ADMIN`, host
networking, or `/dev/net/tun` to the controller service. Restrict routed client
traffic with VPN policy and network firewalls; application access rules do not
prevent a compromised/rooted client from opening raw network connections.
