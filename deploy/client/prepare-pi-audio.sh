#!/bin/sh
set -eu

ACTION=${1:?Audio action is required}
case "$ACTION" in
  prepare)
    DEVICE=${2:?ALSA playback device is required}
    SINK_NAME=${3:?PulseAudio sink name is required}
    ;;
  release)
    SINK_NAME=${2:?PulseAudio sink name is required}
    ;;
  resume)
    DEVICE=${2:?ALSA playback device is required}
    SINK_NAME=${3:?PulseAudio sink name is required}
    CLIENT_PID=${4:?Moonlight process ID is required}
    case "$CLIENT_PID" in *[!0-9]*|"") echo "Invalid Moonlight process ID" >&2; exit 2 ;; esac
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
    prepare|resume) pulseaudio -n --start --file="$PULSE_CONFIG" --exit-idle-time=-1 ;;
    release) exit 0 ;;
  esac
fi

ATTEMPT=0
until pactl info >/dev/null 2>&1; do
  ATTEMPT=$((ATTEMPT + 1))
  [ "$ATTEMPT" -lt 40 ] || { echo "PulseAudio did not become ready" >&2; exit 1; }
  sleep 0.05
done

sink_index() {
  pactl list short sinks | awk -v sink="$1" '$2 == sink { print $1; exit }'
}

module_index() {
  pactl list short modules | awk -v needle="sink_name=$1" \
    '$2 == "module-alsa-sink" && index($0, needle) { print $1; exit }'
}

ensure_parking_sink() {
  if ! pactl list short sinks | awk '$2 == "thinpi_parking" { found=1 } END { exit !found }'; then
    pactl load-module module-null-sink sink_name=thinpi_parking \
      rate=48000 channels=2 >/dev/null
  fi
}

release_physical_sink() {
  SOURCE_INDEX=$(sink_index "$SINK_NAME")
  [ -n "$SOURCE_INDEX" ] || return 0
  ensure_parking_sink
  for INPUT in $(pactl list short sink-inputs | awk -v sink="$SOURCE_INDEX" '$2 == sink { print $1 }'); do
    pactl move-sink-input "$INPUT" thinpi_parking
  done
  MODULE_INDEX=$(module_index "$SINK_NAME")
  [ -z "$MODULE_INDEX" ] || pactl unload-module "$MODULE_INDEX"
}

ensure_physical_sink() {
  if pactl list short sinks | awk -v sink="$SINK_NAME" '$2 == sink { found=1 } END { exit !found }'; then
    pactl suspend-sink "$SINK_NAME" 0 >/dev/null 2>&1 || release_physical_sink
  fi
  if ! pactl list short sinks | awk -v sink="$SINK_NAME" '$2 == sink { found=1 } END { exit !found }'; then
    pactl load-module module-alsa-sink \
      "device=$DEVICE" "sink_name=$SINK_NAME" rate=48000 channels=2 tsched=0 >/dev/null
  fi
  pactl set-default-sink "$SINK_NAME"
}

case "$ACTION" in
  prepare)
    ensure_physical_sink
    ;;
  release)
    release_physical_sink
    ;;
  resume)
    ensure_physical_sink
    pactl -f json list sink-inputs | \
      jq -r --arg pid "$CLIENT_PID" '.[] | select(.properties["application.process.id"] == $pid) | .index' | \
      while IFS= read -r INPUT; do
        [ -z "$INPUT" ] || pactl move-sink-input "$INPUT" "$SINK_NAME"
      done
    ;;
esac
