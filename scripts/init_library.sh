#!/bin/bash
set -Eeuo pipefail
STATE_DIR="${APOFOCUS_STATE_DIR:-$HOME/Library/Application Support/ApoFocus}"
CONFIG_FILE="$STATE_DIR/apofocus.env"
if [[ ! -f "$CONFIG_FILE" ]]; then
  echo "ApoFocus is not configured; run scripts/install_macos.sh first." >&2
  exit 1
fi
# The installer owns this file and writes shell-escaped values with mode 600.
# shellcheck disable=SC1090
source "$CONFIG_FILE"
exec "$STATE_DIR/bin/apofocus-init-bin" "$@"
