# Networking and remote-network access

> **Implementation status:** direct client-to-target connections are implemented.
> The hybrid subnet-router design below is an accepted deployment architecture,
> but ThinPi does not install, enrol, or configure Tailscale/WireGuard today.
> Until an operator configures that routing separately, each client must have an
> existing direct route to every target. The controller application itself is
> not a pivot or proxy.

## What ThinPi does today

The ThinPi controller is a **control plane**, not a remote-desktop proxy.

```text
                         HTTPS control traffic
Client launcher + agent  -------------------------->  ThinPi controller
       |
       | native RDP, VNC, SSH, or Moonlight traffic
       v
Remote desktop host
```

The controller authenticates users, evaluates access rules and time policies,
stores encrypted credentials, issues device-bound launch tickets, and records
session events. After ticket redemption, the agent on the client starts FreeRDP,
TigerVNC, locked OpenSSH, or Moonlight. Session traffic goes
from the client directly to the configured connection host.

Therefore, in the default topology, the client must be able to resolve and reach
the configured target address. Giving the controller host access to a deeper
network does not automatically give that access to the client.

## Recommended hybrid design

Use direct routing whenever the client and target already share a reachable
network. For targets that are only reachable from the controller's site, make
the **controller host or an adjacent Linux gateway a subnet router**. Do not add
routing privileges to the controller container and do not proxy desktop
protocols in the Go web application.

```text
Direct target:
Client ------------------------- LAN ----------------------------> target

Private target:
Client == encrypted routed tunnel ===== gateway/controller host --> target
        controller API remains separate HTTPS control traffic
```

This provides the desired pivot without putting the controller application in
the media path. It also supports Moonlight's UDP streams, which makes a TCP-only
HTTP or SSH relay a poor general solution.

This is currently a deployment recipe, not controller/agent functionality. A
future ThinPi network bootstrap could automate peer enrolment and route checks,
but that work has not been implemented and no UI switch can enable it.

### Route selection and latency

No ThinPi connection flag is required. The connection keeps its real target IP
or DNS name and the client's operating-system route table chooses the path:

- A directly connected, more-specific route should win when the target is local.
- An advertised VPN route is used when that is the only route to the private subnet.
- Avoid identical subnets at different sites. If overlap is unavoidable, design
  route prefixes and metrics deliberately and verify them with `ip route get`.

A routed tunnel adds the network path to the gateway plus encryption and
forwarding overhead. It does not add controller/database processing to each
frame. For interactive RDP and VNC this is usually modest when the gateway is
near the target. Moonlight is more sensitive to RTT, loss, MTU problems, and
gateway throughput, so prefer a direct LAN route whenever one exists.

## Tailscale subnet-router example

Tailscale is the simplest managed option for a small deployment. Its Linux
kernel-mode subnet router performs Layer-3 forwarding and supports the TCP and
UDP traffic required by all ThinPi protocols. Run it on the Proxmox host, a
small gateway VM, or the controller VM host—not inside the controller container.

Assume the private desktop network is `10.40.0.0/16`.

On the gateway:

```sh
sudo tee /etc/sysctl.d/99-thinpi-routing.conf >/dev/null <<'EOF'
net.ipv4.ip_forward = 1
net.ipv6.conf.all.forwarding = 1
EOF
sudo sysctl -p /etc/sysctl.d/99-thinpi-routing.conf
sudo tailscale set --advertise-routes=10.40.0.0/16
```

Approve that advertised route in the Tailscale admin console. On each client:

```sh
sudo tailscale set --accept-routes=true
```

Then verify the selected path and target ports from the client:

```sh
ip route get 10.40.1.25
nc -vz 10.40.1.25 3389   # RDP example
nc -vz 10.40.1.30 5900   # VNC example
```

Use Tailscale grants/ACLs and the gateway firewall to allow only the required
client devices, destination hosts, and ports. ThinPi's UI permissions control what
the launcher can request; they are not a replacement for network firewall rules.

Relevant upstream guidance:

- [Configure a Tailscale subnet router](https://tailscale.com/docs/features/subnet-routers/how-to/setup)
- [Linux route acceptance](https://tailscale.com/docs/features/client/manage-preferences)
- [Kernel versus userspace routing performance](https://tailscale.com/kb/1177/kernel-vs-userspace-routers)
- [Overlapping LAN route behaviour](https://tailscale.com/docs/reference/troubleshooting/network-configuration/lan-traffic-overlapping-subnets)

## Plain WireGuard alternative

WireGuard provides the same network-layer architecture without a hosted
coordination plane. Configure each client peer's `AllowedIPs` with only the remote
desktop subnets, enable forwarding on the site gateway, and add firewall/NAT or
return routes as appropriate. It is operationally more manual but avoids an
additional service dependency. See the [official WireGuard quick start](https://www.wireguard.com/quickstart/).

## Firewall flows

| Source | Destination | Purpose |
|---|---|---|
| Administrator browser | controller TCP 8443 | administration UI/API |
| Client | controller TCP 8443 | login, connection list, tickets, heartbeat, audit |
| Client | SSH host TCP 22 (or configured port) | Locked remote command-line session |
| Client | RDP host TCP 3389 (or configured port) | Windows/Linux RDP session |
| Client | VNC host TCP 5900 (or configured port) | Linux VNC session |
| Client | Sunshine TCP 47984, 47989, 48010 | Moonlight HTTPS/HTTP/RTSP defaults |
| Client | Sunshine UDP 47998–48000 | Moonlight video, control, and audio defaults |
| Client | DNS/NTP | name resolution and certificate-valid system time |

If routed through a gateway, the same target flows must be allowed on the
gateway and destination firewall. Do not expose the controller admin port to
the public Internet. Sunshine custom base ports offset this port family; verify
the selected values against the
[Sunshine network configuration](https://docs.lizardbyte.dev/projects/sunshine/latest/md_docs_2configuration.html).

## Why there is no application-level relay

An in-controller relay would require protocol-specific TCP and UDP forwarding,
connection lifecycle management, rate limits, backpressure, additional secrets,
and a much larger attack surface. It would also make the controller a bandwidth
and availability bottleneck. A routed VPN solves the reachability problem below
the application layer and leaves native clients unchanged.

An application relay can be reconsidered only if a deployment cannot install a
subnet router. It should be a separate, tightly scoped component rather than a
feature of the controller HTTP process.

This boundary is recorded in
[ADR-004](decisions/ADR-004-network-routing.md).
