# ADR-006: Generic supported Linux client appliances

Status: accepted and implemented

## Context

The original deployment scripts treated Raspberry Pi hardware and ARM64 as a
product requirement even though the launcher, agent, native clients and kiosk
controls are ordinary Linux components. This prevented supported deployment on
amd64 VMs, mini PCs and other Debian hardware, and prevented an x86 canary from
testing client upgrades.

## Decision

Support Debian 13/Trixie client appliances on `amd64` and `arm64`, plus
Ubuntu/Lubuntu 24.04 and 26.04 LTS on `amd64`. Keep the
systemd/Xorg/Matchbox kiosk, non-login identity, locked SSH client, managed browser
policy and controller-ticketed maintenance broker common to every platform.

Detect Raspberry Pi only to select the Pi platform adapter: ARM64 validation,
Pi-specific Moonlight repository and Pi hardware acceptance guidance. The
generic provisioner refuses unvalidated distributions rather than weakening or
guessing at kiosk controls. Ubuntu's Chromium Snap is not used for the locked
system identity; the provisioner installs native Chrome from Google's signed
repository and applies the equivalent managed browser policy.

Produce architecture-labelled, matched agent/launcher Debian packages. The
updater must reject foreign architectures, unexpected packages and mismatched
versions.

## Consequences

- VMs and Intel/AMD mini PCs are supported production clients and can serve as
  amd64 release canaries.
- Raspberry Pi remains supported through the same common deployment code and a
  compatibility command.
- An amd64 canary does not replace ARM64/Pi hardware acceptance for display,
  decoding, audio, input, boot and thermal behaviour.
- Moonlight availability remains hardware/package dependent; RDP, VNC and
  locked SSH do not depend on Moonlight.
