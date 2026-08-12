#!/bin/sh

# Shared operating-system validation for the client preflight and provisioner.
# Keep this file POSIX sh: it runs on a newly installed client before ThinPi is
# present.
thinpi_load_supported_os() {
  OS_RELEASE_FILE=${THINPI_OS_RELEASE_FILE:-/etc/os-release}
  [ -r "$OS_RELEASE_FILE" ] || {
    echo "Cannot identify the operating system: $OS_RELEASE_FILE is unreadable" >&2
    return 1
  }

  # Do not inherit similarly named values from the caller's environment.
  ID= ID_LIKE= VERSION_ID= VERSION_CODENAME= PRETTY_NAME=
  # shellcheck disable=SC1090
  . "$OS_RELEASE_FILE"

  case "${ID:-}" in
    debian|raspbian)
      [ "${VERSION_CODENAME:-}" = trixie ] || {
        echo "ThinPi Debian clients require Debian 13 (Trixie); found ${PRETTY_NAME:-unknown}" >&2
        return 1
      }
      THINPI_OS_FAMILY=debian
      ;;
    ubuntu)
      case "${VERSION_ID:-}" in
        24.04|26.04) ;;
        *)
          echo "ThinPi Ubuntu clients require Ubuntu/Lubuntu 24.04 or 26.04 LTS; found ${PRETTY_NAME:-unknown}" >&2
          return 1
          ;;
      esac
      THINPI_OS_FAMILY=ubuntu
      ;;
    *)
      echo "ThinPi clients require Debian 13, Raspberry Pi OS based on Debian 13, or Ubuntu/Lubuntu 24.04/26.04 LTS; found ${PRETTY_NAME:-unknown}" >&2
      return 1
      ;;
  esac

  THINPI_OS_ID=${ID:-unknown}
  THINPI_OS_VERSION=${VERSION_ID:-${VERSION_CODENAME:-unknown}}
  THINPI_OS_CODENAME=${VERSION_CODENAME:-unknown}
  export THINPI_OS_FAMILY THINPI_OS_ID THINPI_OS_VERSION THINPI_OS_CODENAME
}
