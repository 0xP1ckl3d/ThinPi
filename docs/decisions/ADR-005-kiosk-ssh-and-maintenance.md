# ADR-005: separate remote SSH from local maintenance

Status: accepted and implemented.

## Decision

An SSH connection is a managed remote session, not a general terminal-emulator
feature. The agent starts one full-screen xterm whose sole child is OpenSSH.
The command is built from validated fields; a pinned host public key is
mandatory; no shell interpolation or arbitrary SSH options exist. OpenSSH
ignores user configuration and disables its escape command line, local
commands, proxy commands, connection sharing, X11/agent/TCP forwarding and
interactive host trust. Password material is passed through an owner-only
ephemeral file and never argv. xterm has no tabs and exits with SSH.

Local Pi maintenance is a different workflow. An authenticated controller
administrator requests a random 30-second ticket bound to one enabled device.
The root agent redeems it exactly once, then asks systemd to start the fixed
`thinpi-maintenance@<configured-user>.service`. Neither the local request nor
ticket can contain a command or username. The maintenance unit switches to a
fixed VT and existing non-root OS administrator; `exit` switches back to the
signed-out kiosk.

## Kiosk boundary

Normal boot starts systemd services directly as the non-login `thinpi` identity;
there is no passwordless desktop account. Ordinary getties are masked, Xorg
disables VT/Zap/zoom escape sequences, the launcher is restarted by systemd,
and `thinpi` has neither a login shell nor sudo. The administrator Chromium is
controller-origin-only by policy, kiosk-mode, and uses a disposable profile.

## Consequences

- Assigned users can use a remote command line without gaining a local Pi shell.
- Physical administrator maintenance requires live controller authorisation;
  SSH-key maintenance remains the independent recovery path.
- A compromised root account or writable boot media remains outside the kiosk
  UI threat boundary and requires physical/platform controls.
- xterm, OpenSSH, sshpass, systemd/openvt and Chromium policy behaviour must be
  verified on each physical Pi release using `hardware-validation.md`.
