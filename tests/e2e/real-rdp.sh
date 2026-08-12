#!/bin/sh
set -eu
: "${THINPI_RDP_TEST_HOST:?Set THINPI_RDP_TEST_HOST to an authorised Windows test VM}"
echo "Run the documented UI flow against $THINPI_RDP_TEST_HOST, then complete docs/hardware-validation.md."
echo "This test intentionally requires human verification of fullscreen, audio, input, certificate validation, process-list secrecy, and dashboard return."
