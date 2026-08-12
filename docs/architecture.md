# Architecture

ThinPi separates policy, presentation, and native-client execution.

```text
controller (HTTPS + SQLite)
  | user session: safe connection metadata and one-time ticket
  | device credential: ticket redemption and launch manifest
  v
launcher (Qt/QML) -- permission-restricted Unix socket --> agent (Go)
                                                         | direct argv/stdin
                                                         +--> FreeRDP --> RDP host
                                                         +--> TigerVNC --> VNC host
                                                         +--> Moonlight --> Sunshine host
                                                         +--> xterm --> locked OpenSSH --> SSH host
```

The controller enforces ACLs when connections are listed, when a launch ticket
is created, and again when it is redeemed. Tickets are random, hashed in the
database, device-bound, single-use, and expire after 30 seconds by default.
Reusable remote credentials are resolved per access rule (direct user first,
then group, then connection default), decrypted only during redemption, and
only the device-authenticated agent receives them. User policies are evaluated
when a ticket is issued; schedules and daily limits can deny the launch, while
the effective session cap travels in the signed workflow to the agent.

The controller is not an RDP/VNC/SSH/Moonlight proxy. The native client on each
appliance connects to the target address, so the appliance needs a direct or
routed path. When a
private target is reachable only at the controller's site, use the optional
network-layer subnet-router topology in [networking.md](networking.md). Routing
belongs on the Linux host or a gateway, not in the unprivileged controller
container.

The launcher holds its opaque user session in memory. It knows the public
device identifier but cannot read the device bearer credential. The agent
accepts four fixed local actions (`launch`, `status`, `cancel`, and
`maintenance`) and cannot execute administrator-supplied shell fragments. The
maintenance action accepts only a controller-issued, one-use, device-bound
administrator ticket and opens one preconfigured local administrator account on a
fixed virtual console; it cannot carry a command or username.

For an administrator, the launcher can request a short-lived, single-use
handoff and open the controller console in the platform's managed native browser. Redemption creates a
separate browser session; the administrator password and launcher token are
not copied into the browser.

## Session state

```text
idle -> redeeming_ticket -> preparing -> starting_client -> active -> stopping -> idle
                            \---------------- failure ----------------------/
```

Only one interactive session is admitted. FreeRDP gets a single, non-sensitive
`/args-from:stdin` argument; its structured connection parameters and password
are newline-delimited on stdin. TigerVNC receives an ephemeral mode-0600
password file produced by `vncpasswd -f` and deleted after the client exits.
Moonlight gets only `stream`, a validated host,
and a validated configured application. The agent is deliberately the only
place where upstream client syntax is encoded.

SSH gets a single-purpose xterm with OpenSSH as its child. The remote host key
is pinned in an ephemeral owner-only known-hosts file. OpenSSH ignores user
configuration and disables its escape command line, local commands, agent/X11/
TCP forwarding, proxy commands, connection sharing and host-key prompts. The
credential is never placed in argv. Exiting the remote shell destroys xterm.

SQLite uses foreign keys, a busy timeout, and WAL mode. The controller limits
SQLite to one open connection for predictable initial operation. Migrations
are ordered and embedded in the controller binary.

## Direct-console Moonlight seam

The process runner is an interface and Moonlight has its own command builder.
This permits a future runner to stop/yield Xorg, run Moonlight on DRM/TTY, and
restart the UI without changing controller policy or the local API. The change
must be based on the measurements in `hardware-validation.md`.
