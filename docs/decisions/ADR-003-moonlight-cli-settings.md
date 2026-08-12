# ADR-003: Moonlight command-line compatibility

Status: accepted, 2026-08-11.

Current upstream Moonlight Qt source (checked 2026-08-11) defines structured
`stream HOST APP` options for resolution, FPS, bitrate, display mode, HDR,
codec, decoder, frame pacing, gamepad background input and performance overlay.
The adapter validates and supplies those options, forces fullscreen hardware
decode, and never accepts a free-form argument. The installed client's help
output remains authoritative and provisioning records it for compatibility.
Moonlight exposes host-audio playback rather than client audio/gamepad disable
switches. The adapter therefore rejects configurations that request disabled
client audio or gamepad instead of silently failing to enforce policy. Direct
console/TTY operation remains a measured optimisation seam.
