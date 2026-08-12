#!/bin/sh
set -eu
NAME=${1:-ThinPi}
exec "$(dirname "$0")/../bin/thinpi-controller" create-enrolment-token --name "$NAME"
