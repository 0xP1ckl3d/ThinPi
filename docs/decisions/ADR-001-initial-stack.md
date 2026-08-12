# ADR-001: Initial application and display stack

Status: accepted, 2026-08-11.

Use Go plus SQLite for the controller, Go for the local agent, and C++20/Qt 6
QML for the kiosk launcher. Use minimal Xorg/Matchbox on Raspberry Pi OS Lite
for the first stable FreeRDP deployment. Keep protocol invocation behind agent
adapters so Moonlight can later use a direct DRM/TTY runner. No browser gateway,
SPA framework, desktop environment, or protocol reimplementation is included.
