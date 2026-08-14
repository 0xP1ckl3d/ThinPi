#!/bin/sh
set -eu

DEVICE=${1:?ALSA playback device is required}
SINK_NAME=${2:?PulseAudio sink name is required}
case "$SINK_NAME" in
  thinpi_[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]) ;;
  *) echo "Invalid ThinPi audio sink name" >&2; exit 2 ;;
esac

PULSE_CONFIG=/usr/local/libexec/thinpi-pulse.pa
if ! pulseaudio --check >/dev/null 2>&1; then
  pulseaudio --start --file="$PULSE_CONFIG" --exit-idle-time=-1
fi

ATTEMPT=0
until pactl info >/dev/null 2>&1; do
  ATTEMPT=$((ATTEMPT + 1))
  [ "$ATTEMPT" -lt 40 ] || { echo "PulseAudio did not become ready" >&2; exit 1; }
  sleep 0.05
done

if ! pactl list short sinks | awk -v sink="$SINK_NAME" '$2 == sink { found=1 } END { exit !found }'; then
  pactl load-module module-alsa-sink \
    "device=$DEVICE" "sink_name=$SINK_NAME" rate=48000 channels=2 tsched=0 >/dev/null
fi
pactl set-default-sink "$SINK_NAME"
