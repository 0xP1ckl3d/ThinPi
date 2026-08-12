#!/bin/sh
set -eu
OUT=${1:-thinpi-benchmark-$(date -u +%Y%m%dT%H%M%SZ).txt}
{
  echo "ThinPi hardware benchmark"
  date -u
  uname -a
  cat /proc/device-tree/model 2>/dev/null || true; echo
  thinpi-agent version || true
  xfreerdp3 /version 2>/dev/null || xfreerdp /version 2>/dev/null || true
  moonlight-qt --version 2>/dev/null || true
  ip -brief address
  vcgencmd measure_temp 2>/dev/null || true
  vcgencmd get_throttled 2>/dev/null || true
  free -h
  systemctl --no-pager --full status thinpi-agent thinpi-ui || true
  journalctl -u thinpi-agent --since '30 minutes ago' --no-pager || true
} > "$OUT"
echo "Wrote $OUT. Add Moonlight session statistics and subjective input/audio results to docs/hardware-validation.md."
