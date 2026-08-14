#!/bin/sh
set -eu

ACTION=${1:?Audio action is required}
case "$ACTION" in
  prepare)
    DEVICE=${2:?ALSA playback device is required}
    SINK_NAME=${3:?PulseAudio sink name is required}
    ;;
  suspend|resume)
    SINK_NAME=${2:?PulseAudio sink name is required}
    ;;
  *)
    echo "Invalid ThinPi audio action" >&2
    exit 2
    ;;
esac
case "$SINK_NAME" in
  thinpi_[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f]) ;;
  *) echo "Invalid ThinPi audio sink name" >&2; exit 2 ;;
esac

PULSE_CONFIG=/usr/local/libexec/thinpi-pulse.pa
if ! pulseaudio --check >/dev/null 2>&1; then
  case "$ACTION" in
    prepare) pulseaudio --start --file="$PULSE_CONFIG" --exit-idle-time=-1 ;;
    suspend) exit 0 ;;
    resume) echo "PulseAudio is not running" >&2; exit 1 ;;
  esac
fi

ATTEMPT=0
until pactl info >/dev/null 2>&1; do
  ATTEMPT=$((ATTEMPT + 1))
  [ "$ATTEMPT" -lt 40 ] || { echo "PulseAudio did not become ready" >&2; exit 1; }
  sleep 0.05
done

case "$ACTION" in
  prepare)
    if ! pactl list short sinks | awk -v sink="$SINK_NAME" '$2 == sink { found=1 } END { exit !found }'; then
      pactl load-module module-alsa-sink \
        "device=$DEVICE" "sink_name=$SINK_NAME" rate=48000 channels=2 tsched=0 >/dev/null
    fi
    pactl suspend-sink "$SINK_NAME" 0
    pactl set-default-sink "$SINK_NAME"
    ;;
  suspend)
    pactl suspend-sink "$SINK_NAME" 1
    ;;
  resume)
    pactl suspend-sink "$SINK_NAME" 0
    pactl set-default-sink "$SINK_NAME"
    ;;
esac
