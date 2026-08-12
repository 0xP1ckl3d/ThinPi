# ADR-004: Keep desktop traffic out of the controller application

## Status

Accepted architecture; external subnet-router setup is not yet automated by
ThinPi.

## Context

Some ThinPi targets are directly reachable from a Pi, while others are on a
private network reachable only at the controller's site. Sending every native
session through the controller would provide a pivot but would add an avoidable
hop for local targets and require the controller to proxy RDP, VNC, and
Moonlight's mixed TCP/UDP traffic.

## Decision

The controller remains an unprivileged HTTPS control plane. Native clients on
the Pi connect to the target address selected by the controller. Private target
networks are exposed with an optional Layer-3 subnet router on the controller
host or an adjacent Linux gateway, using Tailscale/WireGuard or an equivalent
routed VPN.

Route selection is delegated to the Pi operating system. Direct, more-specific
local routes should be preferred; VPN routes serve otherwise unreachable
private prefixes. The controller container will not receive `NET_ADMIN`, host
networking, or tunnel-device access.

## Consequences

- Direct sessions do not incur controller-site forwarding latency.
- Private targets gain a common pivot that supports TCP and UDP protocols.
- The gateway, route prefixes, VPN ACLs, MTU, and firewall become deployment
  responsibilities documented in `docs/networking.md`.
- The controller cannot make an unreachable target reachable by itself.
- A compromised Pi may attempt raw network access, so ThinPi application ACLs
  must be backed by VPN and network firewall policy.
- An application-level relay would be a separate future component only for
  environments where subnet routing is impossible.
