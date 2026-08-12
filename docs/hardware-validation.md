# Raspberry Pi production acceptance record

This is the final release gate for a real Pi. Source builds and x86 mock tests
cannot certify display, decoder, audio, input, power, thermal, or network
behaviour.

Copy this file for each Pi/model/site and record actual results.

## Test identity

| Field | Value |
|---|---|
| Date/operator | |
| ThinPi version/commit | |
| Pi model/revision/RAM | |
| Storage/power supply | |
| Raspberry Pi OS/kernel | |
| Qt/FreeRDP/TigerVNC/OpenSSH/xterm/Moonlight versions | |
| Display/resolution/refresh/HDR | |
| Network wired/Wi-Fi/VPN | |
| Controller URL/version | |
| RDP/VNC/Sunshine host OS/GPU/version | |

Collect the baseline:

```sh
sudo /tmp/thinpi/benchmark-pi.sh /tmp/thinpi-baseline.txt
sudo /usr/bin/thinpi-agent status --config /etc/thinpi/agent.json
systemctl is-active thinpi-agent thinpi-ui
vcgencmd measure_temp
vcgencmd get_throttled
```

## Mandatory tests

| Test | Pass criteria | Result/evidence |
|---|---|---|
| 25 cold boots | login appears every time; no console/desktop; services active | Pending hardware |
| Network loss/recovery | clear error; UI remains usable; reconnects after network returns | Pending hardware |
| Controller restart | launcher recovers without Pi reboot | Pending hardware |
| Power interruption | filesystem/services recover and login returns | Pending hardware |
| Admin SSO | admin button opens console; non-admin has no button | Pending hardware |
| Session expiry | `/admin` and open dashboard return to login naturally | Pending hardware |
| User disable | disabled launcher account denied immediately | Pending hardware |
| Policy schedule | disallowed time/day denied with correct message | Pending hardware |
| Daily limit | exhausted allowance denies new launch | Pending hardware |
| Session cap | native client ends at cap and launcher explains why | Pending hardware |
| RDP fullscreen | display/input/audio usable; close returns to launcher | Pending host |
| RDP credential secrecy | password absent from launcher, `ps`, `/proc/*/cmdline`, logs | Pending host |
| Linux VNC | authentication/display/input work; close returns | Pending host |
| Locked remote SSH | pinned host accepted; remote shell works; exit returns to launcher | Pending host |
| SSH wrong host key | connection fails closed with no trust prompt | Pending host |
| SSH local escape | no tabs/local shell, OpenSSH escapes, forwarding, logs or argv password | Pending hardware |
| Device revocation | revoked Pi cannot redeem new launch tickets | Pending hardware |
| SSH maintenance | configured admin password/key login still works; root/forwarding denied | Pending hardware |
| Kiosk key escape | Ctrl+Alt+F1-F6, Ctrl+Alt+Backspace and window keys expose no console/desktop | Pending hardware |
| Kiosk OS identity | `thinpi` is nologin, non-sudo; getties masked; system paths read-only | Pending hardware |
| Admin local maintenance | admin-only one-use ticket opens fixed account; `exit` returns signed-out kiosk | Pending hardware |
| Locked admin browser | controller only; no devtools/downloads/guest/incognito/file URLs | Pending hardware |
| Update/rollback | matched agent/launcher update and rollback succeed | Pending hardware |

## Moonlight tests (mandatory when Moonlight is deployed)

| Test | Pass criteria | Result/measurement |
|---|---|---|
| Pairing identity | pairing exists for Linux user `thinpi` after reboot | Pending |
| 1080p60 wired | stable hardware decode, audio, input/gamepad | Pending |
| 30-minute soak | no disconnect, thermal throttle, audio drift, or major frame loss | Pending |
| Latency/statistics | record RTT, decode/render/network latency, dropped frames | Pending |
| 1440p60 | optional; record drops/temperature | Pending |
| 1080p120 | optional; record display/decoder support | Pending |
| Xorg suitability | acceptable performance through current Xorg kiosk | Pending |

Moonlight's Raspberry Pi guidance notes that Pi 4 desktop/window scaling above
1080p can materially reduce performance and recommends console/TTY operation
for best performance. ThinPi currently launches from its Xorg kiosk. Do not
claim Moonlight performance readiness until the physical measurements are
acceptable; a direct-console runner remains a future optimisation seam.

## Network-pivot tests (mandatory when routed VPN is used)

Record:

```sh
ip route get TARGET_IP
ping -c 20 TARGET_IP
tracepath TARGET_IP
```

Verify direct local targets remain direct and remote private targets use the
expected subnet router. Test Moonlight UDP flows, MTU, loss, and gateway CPU;
TCP-only port tests are not sufficient for Moonlight.

## Sign-off

- [ ] Every deployed protocol passed on the actual target.
- [ ] Mandatory resilience/security tests passed.
- [ ] Controller database and master key backup/restore were tested.
- [ ] Known limitations are recorded and accepted.
- [ ] Failed/untested rows are not represented as production-ready.

Operator/signature/date:
