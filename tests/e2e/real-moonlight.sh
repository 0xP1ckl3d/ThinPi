#!/bin/sh
set -eu
: "${THINPI_SUNSHINE_TEST_HOST:?Set THINPI_SUNSHINE_TEST_HOST to a paired Sunshine host}"
echo "Run the documented UI flow against $THINPI_SUNSHINE_TEST_HOST, then complete docs/hardware-validation.md."
echo "Capture 1080p60 frame, decode, network and input latency statistics plus gamepad and HDMI audio results."
