#!/bin/bash

set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
if [[ "$(uname -s)" == "Darwin" ]]; then
  DEFAULT_STATE_DIR="$HOME/Library/Application Support/ApoFocus"
else
  DEFAULT_STATE_DIR="$HOME/.local/share/apofocus"
fi
STATE_DIR="${APOFOCUS_STATE_DIR:-$DEFAULT_STATE_DIR}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
SKIP_MODEL_DOWNLOAD=0

usage() {
  printf '%s\n' \
    "Usage: bash scripts/install_deep_analysis.sh [options]" \
    "" \
    "Options:" \
    "  --state-dir PATH          Installation and model-cache directory." \
    "  --python PATH             Python 3.11+ executable." \
    "  --skip-model-download     Install code and packages without downloading Qwen." \
    "  -h, --help                Show this help." \
    "" \
    "The installer does not start or register a service. After installation, use" \
    "the printed command in systemd, Docker, launchd, or your process supervisor."
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --state-dir) [[ $# -ge 2 ]] || { echo "$1 requires a path" >&2; exit 2; }; STATE_DIR="$2"; shift 2 ;;
    --python) [[ $# -ge 2 ]] || { echo "$1 requires a path" >&2; exit 2; }; PYTHON_BIN="$2"; shift 2 ;;
    --skip-model-download) SKIP_MODEL_DOWNLOAD=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$STATE_DIR" in
  /*) ;;
  ~/*) STATE_DIR="$HOME/${STATE_DIR#\~/}" ;;
  *) STATE_DIR="$PWD/$STATE_DIR" ;;
esac

"$PYTHON_BIN" -c 'import sys; assert sys.version_info >= (3, 11), "Python 3.11+ is required"'

SERVICE_DIR="$STATE_DIR/services/deep-analysis"
VENV_DIR="$STATE_DIR/deep-analysis-venv"
MODEL_DIR="$STATE_DIR/models"
mkdir -p "$SERVICE_DIR" "$MODEL_DIR"
chmod 700 "$STATE_DIR" "$MODEL_DIR"
install -m 0644 "$PROJECT_ROOT/services/deep_analysis/app.py" "$SERVICE_DIR/app.py"
install -m 0644 "$PROJECT_ROOT/services/deep_analysis/preload.py" "$SERVICE_DIR/preload.py"
install -m 0644 "$PROJECT_ROOT/services/deep_analysis/requirements.txt" "$SERVICE_DIR/requirements.txt"

if [[ ! -x "$VENV_DIR/bin/python" ]]; then
  "$PYTHON_BIN" -m venv "$VENV_DIR"
fi
"$VENV_DIR/bin/python" -m pip install --upgrade pip setuptools wheel
"$VENV_DIR/bin/python" -m pip install --requirement "$SERVICE_DIR/requirements.txt"

if (( SKIP_MODEL_DOWNLOAD == 0 )); then
  env HF_HOME="$MODEL_DIR/huggingface" "$VENV_DIR/bin/python" "$SERVICE_DIR/preload.py"
fi

printf '\n%s\n' "Deep analysis service installed. Configure PHOTO_ROOTS and run:"
printf 'HF_HOME=%q PHOTO_ROOTS=%q THUMBNAIL_ROOTS=%q %q -m uvicorn app:app --app-dir %q --host 127.0.0.1 --port 8091\n' \
  "$MODEL_DIR/huggingface" "/path/to/import-roots" "/path/to/apofocus-library" "$VENV_DIR/bin/python" "$SERVICE_DIR"
