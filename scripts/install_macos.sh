#!/bin/bash

set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CURRENT_STEP="initial checks"

YES=0
SKIP_MODEL_DOWNLOAD=0
NO_START=0
CHECK_ONLY=0

STATE_DIR="${APOFOCUS_STATE_DIR:-$HOME/Library/Application Support/ApoFocus}"
LIBRARY_ROOT="${APOFOCUS_LIBRARY_ROOT:-$HOME/Pictures/ApoFocus Library}"
INBOX_ROOT="${APOFOCUS_INBOX_ROOT:-$HOME/Pictures/ApoFocus Inbox}"
IMPORT_ROOTS="${APOFOCUS_IMPORT_ROOTS:-}"
BACKUP_ROOT="${APOFOCUS_BACKUP_ROOT:-}"
POSTGRES_PORT="${APOFOCUS_POSTGRES_PORT:-55432}"
ADDR="${APOFOCUS_ADDR:-127.0.0.1:8080}"

usage() {
  cat <<'EOF'
Usage: bash scripts/install_macos.sh [options]

Options:
  --yes                    Do not ask for confirmation.
  --library-root PATH      Managed photo/video/audio library.
  --inbox-root PATH        MCP and batch import inbox.
  --import-roots PATHS     Colon-separated import allowlist.
  --state-dir PATH         App binaries, PostgreSQL, venv, and model cache.
  --backup-root PATH       External APFS volume directory for PostgreSQL backups.
  --postgres-port PORT     Dedicated local PostgreSQL port (default: 55432).
  --addr HOST:PORT         Local Web listen address (default: 127.0.0.1:8080).
  --skip-model-download    Install Python packages but download weights later.
  --no-start               Install and migrate, but leave all services stopped.
  --check-only             Read-only prerequisite and configuration check.
  -h, --help               Show this help.

The same values can be supplied with APOFOCUS_LIBRARY_ROOT,
APOFOCUS_INBOX_ROOT, APOFOCUS_IMPORT_ROOTS, APOFOCUS_STATE_DIR,
APOFOCUS_BACKUP_ROOT, APOFOCUS_POSTGRES_PORT, and APOFOCUS_ADDR.
EOF
}

fail() {
  echo "[ApoFocus] ERROR: $*" >&2
  exit 1
}

on_error() {
  local exit_code=$?
  echo "[ApoFocus] Installation failed during: $CURRENT_STEP" >&2
  echo "[ApoFocus] Fix the error above and run this installer again; completed steps are reusable." >&2
  exit "$exit_code"
}
trap on_error ERR

log() {
  echo
  echo "==> $*"
}

wait_for_port_release() {
  local port="$1" released=0
  for _ in $(seq 1 40); do
    if ! lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      released=1
      break
    fi
    sleep 0.25
  done
  (( released == 1 ))
}

expand_path() {
  case "$1" in
    "~") printf '%s\n' "$HOME" ;;
    "~/"*) printf '%s/%s\n' "$HOME" "${1#\~/}" ;;
    /*) printf '%s\n' "$1" ;;
    *) printf '%s/%s\n' "$PWD" "$1" ;;
  esac
}

normalize_roots() {
  local value result="" root
  local old_ifs="$IFS"
  local -a values
  IFS=':'
  read -r -a values <<< "$1"
  IFS="$old_ifs"
  for value in "${values[@]}"; do
    [[ -n "$value" ]] || continue
    root="$(expand_path "$value")"
    if [[ -z "$result" ]]; then
      result="$root"
    else
      result="$result:$root"
    fi
  done
  printf '%s\n' "$result"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --yes) YES=1; shift ;;
    --skip-model-download) SKIP_MODEL_DOWNLOAD=1; shift ;;
    --no-start) NO_START=1; shift ;;
    --check-only) CHECK_ONLY=1; shift ;;
    --library-root) [[ $# -ge 2 ]] || fail "$1 requires a path"; LIBRARY_ROOT="$2"; shift 2 ;;
    --inbox-root) [[ $# -ge 2 ]] || fail "$1 requires a path"; INBOX_ROOT="$2"; shift 2 ;;
    --import-roots) [[ $# -ge 2 ]] || fail "$1 requires a value"; IMPORT_ROOTS="$2"; shift 2 ;;
    --state-dir) [[ $# -ge 2 ]] || fail "$1 requires a path"; STATE_DIR="$2"; shift 2 ;;
    --backup-root) [[ $# -ge 2 ]] || fail "$1 requires a path"; BACKUP_ROOT="$2"; shift 2 ;;
    --postgres-port) [[ $# -ge 2 ]] || fail "$1 requires a port"; POSTGRES_PORT="$2"; shift 2 ;;
    --addr) [[ $# -ge 2 ]] || fail "$1 requires HOST:PORT"; ADDR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "unknown option: $1" ;;
  esac
done

STATE_DIR="$(expand_path "$STATE_DIR")"
LIBRARY_ROOT="$(expand_path "$LIBRARY_ROOT")"
INBOX_ROOT="$(expand_path "$INBOX_ROOT")"
if [[ -n "$BACKUP_ROOT" ]]; then
  BACKUP_ROOT="$(expand_path "$BACKUP_ROOT")"
fi
if [[ -z "$IMPORT_ROOTS" ]]; then
  IMPORT_ROOTS="$INBOX_ROOT:/Volumes"
fi
IMPORT_ROOTS="$(normalize_roots "$IMPORT_ROOTS")"

[[ "$POSTGRES_PORT" =~ ^[0-9]+$ ]] || fail "PostgreSQL port must be numeric"
POSTGRES_PORT=$((10#$POSTGRES_PORT))
(( POSTGRES_PORT >= 1024 && POSTGRES_PORT <= 65535 )) || fail "PostgreSQL port must be between 1024 and 65535"
[[ "$ADDR" == *:* ]] || fail "--addr must be HOST:PORT"
[[ "$ADDR" == 127.0.0.1:* ]] || fail "ApoFocus has no remote authentication yet; --addr must use 127.0.0.1"
WEB_PORT="${ADDR##*:}"
[[ "$WEB_PORT" =~ ^[0-9]+$ ]] || fail "Web port must be numeric"
WEB_PORT=$((10#$WEB_PORT))
(( WEB_PORT >= 1024 && WEB_PORT <= 65535 )) || fail "Web port must be between 1024 and 65535"
(( POSTGRES_PORT != WEB_PORT && POSTGRES_PORT != 8090 && WEB_PORT != 8090 )) || fail "PostgreSQL, Web, and embedding ports must be different"
ADDR="127.0.0.1:$WEB_PORT"
[[ "$LIBRARY_ROOT" != *:* ]] || fail "library path cannot contain ':'"
[[ "$INBOX_ROOT" != *:* ]] || fail "inbox path cannot contain ':'"
[[ "$STATE_DIR" != *:* ]] || fail "state path cannot contain ':'"
[[ "$BACKUP_ROOT" != *:* ]] || fail "backup path cannot contain ':'"
if [[ -n "$BACKUP_ROOT" && "$BACKUP_ROOT" != /Volumes/* ]]; then
  fail "--backup-root must be on a mounted external volume under /Volumes"
fi

APP_URL="http://$ADDR"
BIN_DIR="$STATE_DIR/bin"
SERVICE_DIR="$STATE_DIR/services/embedding"
VENV_DIR="$STATE_DIR/venv"
MODEL_DIR="$STATE_DIR/models"
POSTGRES_DATA="$STATE_DIR/postgres"
LOG_DIR="$HOME/Library/Logs/ApoFocus"
LAUNCH_AGENT_DIR="$HOME/Library/LaunchAgents"
CONFIG_FILE="$STATE_DIR/apofocus.env"
PASSWORD_FILE="$STATE_DIR/postgres.password"
BACKUP_STATUS="$STATE_DIR/backup-status.json"
BACKUP_VOLUME_UUID=""
DOMAIN="gui/$(id -u)"

[[ "$(uname -s)" == "Darwin" ]] || fail "this installer only supports macOS"
(( EUID != 0 )) || fail "do not run this installer with sudo"

echo "ApoFocus macOS installation"
echo "  Repository:      $PROJECT_ROOT"
echo "  App state:       $STATE_DIR"
echo "  Managed library: $LIBRARY_ROOT"
echo "  Import roots:    $IMPORT_ROOTS"
echo "  Web:             $APP_URL"
echo "  PostgreSQL:      127.0.0.1:$POSTGRES_PORT (dedicated cluster)"
if [[ -n "$BACKUP_ROOT" ]]; then
  echo "  Backup root:     $BACKUP_ROOT"
else
  echo "  Backup root:     disabled (use --backup-root /Volumes/<volume>/ApoFocusBackup)"
fi
if (( SKIP_MODEL_DOWNLOAD == 0 )); then
  echo "  Models:          OpenCLIP + Whisper base + LAION-CLAP (several GB)"
else
  echo "  Models:          deferred until first use"
fi

if (( CHECK_ONLY == 1 )); then
  missing=0
  for command_name in xcode-select curl launchctl; do
    if command -v "$command_name" >/dev/null 2>&1; then
      echo "[ok] $command_name"
    else
      echo "[missing] $command_name"
      missing=1
    fi
  done
  check_brew=""
  if command -v brew >/dev/null 2>&1; then
    check_brew="$(command -v brew)"
  elif [[ -x /opt/homebrew/bin/brew ]]; then
    check_brew=/opt/homebrew/bin/brew
  elif [[ -x /usr/local/bin/brew ]]; then
    check_brew=/usr/local/bin/brew
  fi
  if [[ -n "$check_brew" ]]; then
    echo "[ok] Homebrew"
    for formula in go python@3.12 postgresql@18 pgvector ffmpeg libraw libsndfile libomp cmake pkgconf; do
      if "$check_brew" list --versions "$formula" >/dev/null 2>&1; then
        echo "[ok] Homebrew formula: $formula"
      else
        echo "[missing] Homebrew formula: $formula"
        missing=1
      fi
    done
  else
    echo "[missing] Homebrew"
    missing=1
  fi
  if xcode-select -p >/dev/null 2>&1; then
    echo "[ok] Xcode Command Line Tools"
  else
    echo "[missing] Xcode Command Line Tools"
    missing=1
  fi
  exit "$missing"
fi

if (( YES == 0 )) && [[ -t 0 ]]; then
  read -r -p "Continue with installation? [y/N] " answer
  [[ "$answer" == "y" || "$answer" == "Y" ]] || exit 0
fi

CURRENT_STEP="Xcode Command Line Tools"
if ! xcode-select -p >/dev/null 2>&1; then
  xcode-select --install >/dev/null 2>&1 || true
  fail "Xcode Command Line Tools installation was requested. Finish the macOS dialog, then run this script again."
fi

CURRENT_STEP="Homebrew installation"
if command -v brew >/dev/null 2>&1; then
  BREW="$(command -v brew)"
elif [[ -x /opt/homebrew/bin/brew ]]; then
  BREW=/opt/homebrew/bin/brew
elif [[ -x /usr/local/bin/brew ]]; then
  BREW=/usr/local/bin/brew
else
  log "Installing Homebrew"
  /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  if [[ -x /opt/homebrew/bin/brew ]]; then
    BREW=/opt/homebrew/bin/brew
  elif [[ -x /usr/local/bin/brew ]]; then
    BREW=/usr/local/bin/brew
  else
    fail "Homebrew installation finished but brew was not found"
  fi
fi
BREW_PREFIX="$($BREW --prefix)"
eval "$($BREW shellenv)"

CURRENT_STEP="Homebrew dependencies"
log "Installing Go, PostgreSQL, pgvector, Python, FFmpeg, and media libraries"
"$BREW" install go python@3.12 postgresql@18 pgvector ffmpeg libraw libsndfile libomp cmake pkgconf

GO_BIN="$BREW_PREFIX/bin/go"
PYTHON_BIN="$BREW_PREFIX/bin/python3.12"
POSTGRES_BIN="$($BREW --prefix postgresql@18)/bin"
[[ -x "$GO_BIN" ]] || GO_BIN="$(command -v go)"
[[ -x "$PYTHON_BIN" ]] || fail "Homebrew python3.12 was not found"
[[ -x "$POSTGRES_BIN/postgres" ]] || fail "Homebrew PostgreSQL 18 was not found"
[[ -f "$($POSTGRES_BIN/pg_config --sharedir)/extension/vector.control" ]] || fail "pgvector is not installed for PostgreSQL 18"

CURRENT_STEP="installation directories"
if [[ "$LIBRARY_ROOT" == /Volumes/* ]]; then
  volume_name="${LIBRARY_ROOT#/Volumes/}"
  volume_name="${volume_name%%/*}"
  [[ -d "/Volumes/$volume_name" ]] || fail "external volume /Volumes/$volume_name is not mounted"
fi
if [[ -n "$BACKUP_ROOT" ]]; then
  backup_volume_root="${BACKUP_ROOT#/Volumes/}"
  backup_volume_root="/Volumes/${backup_volume_root%%/*}"
  [[ -d "$backup_volume_root" ]] || fail "external backup volume $backup_volume_root is not mounted"
  backup_filesystem="$(/usr/sbin/diskutil info -plist "$backup_volume_root" | /usr/bin/plutil -extract FilesystemName raw -o - -)"
  [[ "$backup_filesystem" == "APFS" ]] || fail "external backup volume must use APFS; found $backup_filesystem"
  BACKUP_VOLUME_UUID="$(/usr/sbin/diskutil info -plist "$backup_volume_root" | /usr/bin/plutil -extract VolumeUUID raw -o - -)"
  [[ -n "$BACKUP_VOLUME_UUID" ]] || fail "could not read Volume UUID for $backup_volume_root"
fi
mkdir_paths=("$STATE_DIR" "$BIN_DIR" "$MODEL_DIR" "$LOG_DIR" "$LAUNCH_AGENT_DIR" "$LIBRARY_ROOT" "$INBOX_ROOT")
if [[ -n "$BACKUP_ROOT" ]]; then
  mkdir_paths+=("$BACKUP_ROOT" "$BACKUP_ROOT/postgres")
fi
mkdir -p "${mkdir_paths[@]}"
chmod 700 "$STATE_DIR" "$MODEL_DIR"
chmod 750 "$BIN_DIR" "$LIBRARY_ROOT" "$INBOX_ROOT" "$LOG_DIR"
if [[ -n "$BACKUP_ROOT" ]]; then
  chmod 700 "$BACKUP_ROOT" "$BACKUP_ROOT/postgres"
fi
old_ifs="$IFS"
IFS=':'
read -r -a import_root_values <<< "$IMPORT_ROOTS"
IFS="$old_ifs"
for import_root in "${import_root_values[@]}"; do
  [[ -d "$import_root" ]] || fail "import root does not exist: $import_root"
done

CURRENT_STEP="Go binaries"
log "Building ApoFocus binaries"
cd "$PROJECT_ROOT"
mkdir -p "$STATE_DIR/cache/go-build"
env GOCACHE="$STATE_DIR/cache/go-build" "$GO_BIN" build -trimpath -o "$BIN_DIR/apofocus" ./cmd/apofocus
env GOCACHE="$STATE_DIR/cache/go-build" "$GO_BIN" build -trimpath -o "$BIN_DIR/apofocus-mcp" ./cmd/apofocus-mcp
env GOCACHE="$STATE_DIR/cache/go-build" "$GO_BIN" build -trimpath -o "$BIN_DIR/apofocus-batch" ./cmd/apofocus-batch
env GOCACHE="$STATE_DIR/cache/go-build" "$GO_BIN" build -trimpath -o "$BIN_DIR/apofocus-backup" ./cmd/apofocus-backup
install -m 0755 "$SCRIPT_DIR/apofocusctl" "$BIN_DIR/apofocusctl"

CURRENT_STEP="Python environment"
log "Installing Python analysis service"
mkdir -p "$SERVICE_DIR"
install -m 0644 "$PROJECT_ROOT/services/embedding/app.py" "$SERVICE_DIR/app.py"
install -m 0644 "$PROJECT_ROOT/services/embedding/preload.py" "$SERVICE_DIR/preload.py"
install -m 0644 "$PROJECT_ROOT/services/embedding/requirements.txt" "$SERVICE_DIR/requirements.txt"
install -m 0644 "$PROJECT_ROOT/services/embedding/worker.py" "$SERVICE_DIR/worker.py"
if [[ -x "$VENV_DIR/bin/python" ]]; then
  venv_version="$($VENV_DIR/bin/python -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')"
  if [[ "$venv_version" != "3.12" ]]; then
    mv "$VENV_DIR" "$VENV_DIR.backup.$(date +%Y%m%d%H%M%S)"
  fi
fi
if [[ ! -x "$VENV_DIR/bin/python" ]]; then
  "$PYTHON_BIN" -m venv "$VENV_DIR"
fi
"$VENV_DIR/bin/python" -m pip install --upgrade pip setuptools wheel
"$VENV_DIR/bin/python" -m pip install --requirement "$SERVICE_DIR/requirements.txt"

CURRENT_STEP="PostgreSQL credentials and cluster"
if [[ -f "$PASSWORD_FILE" ]]; then
  DB_PASSWORD="$(tr -d '\r\n' < "$PASSWORD_FILE")"
  [[ -n "$DB_PASSWORD" ]] || fail "$PASSWORD_FILE is empty"
else
  umask 077
  DB_PASSWORD="$(openssl rand -hex 24)"
  printf '%s\n' "$DB_PASSWORD" > "$PASSWORD_FILE"
fi
chmod 600 "$PASSWORD_FILE"
DATABASE_URL="postgresql://apofocus:$DB_PASSWORD@127.0.0.1:$POSTGRES_PORT/apofocus?sslmode=disable"

if [[ ! -f "$POSTGRES_DATA/PG_VERSION" ]]; then
  mkdir -p "$POSTGRES_DATA"
  chmod 700 "$POSTGRES_DATA"
  "$POSTGRES_BIN/initdb" -D "$POSTGRES_DATA" --encoding=UTF8 --locale=C --username=apofocus \
    --auth-local=trust --auth-host=scram-sha-256 --pwfile="$PASSWORD_FILE"
else
  pg_major="$(cut -d. -f1 < "$POSTGRES_DATA/PG_VERSION")"
  [[ "$pg_major" == "18" ]] || fail "existing ApoFocus cluster uses PostgreSQL $pg_major; expected 18"
fi

CURRENT_STEP="model weights"
if (( SKIP_MODEL_DOWNLOAD == 0 )); then
  log "Downloading and validating local model weights"
  env \
    PHOTO_ROOTS="$IMPORT_ROOTS" \
    THUMBNAIL_ROOTS="$LIBRARY_ROOT" \
    PHOTO_LIBRARY_ROOT="$LIBRARY_ROOT" \
    HF_HOME="$MODEL_DIR/huggingface" \
    TORCH_HOME="$MODEL_DIR/torch" \
    XDG_CACHE_HOME="$MODEL_DIR/xdg" \
    WHISPER_DOWNLOAD_ROOT="$MODEL_DIR/whisper" \
    "$VENV_DIR/bin/python" "$SERVICE_DIR/preload.py"
fi

CURRENT_STEP="configuration"
umask 077
config_temporary="$CONFIG_FILE.tmp.$$"
{
  printf 'DATABASE_URL=%q\n' "$DATABASE_URL"
  printf 'POSTGRES_BIN=%q\n' "$POSTGRES_BIN"
  printf 'POSTGRES_DATA=%q\n' "$POSTGRES_DATA"
  printf 'POSTGRES_PORT=%q\n' "$POSTGRES_PORT"
  printf 'PHOTO_LIBRARY_ROOT=%q\n' "$LIBRARY_ROOT"
  printf 'APOFOCUS_IMPORT_ROOTS=%q\n' "$IMPORT_ROOTS"
  printf 'EMBEDDING_SERVICE_URL=%q\n' "http://127.0.0.1:8090"
  printf 'APOFOCUS_APP_URL=%q\n' "$APP_URL"
  printf 'APOFOCUS_BACKUP_ROOT=%q\n' "$BACKUP_ROOT"
  printf 'APOFOCUS_BACKUP_STATUS=%q\n' "$BACKUP_STATUS"
  printf 'APOFOCUS_BACKUP_VOLUME_UUID=%q\n' "$BACKUP_VOLUME_UUID"
  printf 'ADDR=%q\n' "$ADDR"
} > "$config_temporary"
mv "$config_temporary" "$CONFIG_FILE"
chmod 600 "$CONFIG_FILE"

"$PYTHON_BIN" "$SCRIPT_DIR/generate_launchd_plists.py" \
  --output-dir "$LAUNCH_AGENT_DIR" \
  --state-dir "$STATE_DIR" \
  --logs-dir "$LOG_DIR" \
  --postgres-bin "$POSTGRES_BIN" \
  --postgres-data "$POSTGRES_DATA" \
  --postgres-port "$POSTGRES_PORT" \
  --app-bin "$BIN_DIR/apofocus" \
  --mcp-bin "$BIN_DIR/apofocus-mcp" \
  --backup-bin "$BIN_DIR/apofocus-backup" \
  --python-bin "$VENV_DIR/bin/python" \
  --embedding-dir "$SERVICE_DIR" \
  --database-url "$DATABASE_URL" \
  --addr "$ADDR" \
  --app-url "$APP_URL" \
  --library-root "$LIBRARY_ROOT" \
  --import-roots "$IMPORT_ROOTS" \
  --brew-prefix "$BREW_PREFIX" \
  --backup-root "$BACKUP_ROOT" \
  --backup-status "$BACKUP_STATUS" \
  --backup-volume-uuid "$BACKUP_VOLUME_UUID"

for label in com.apofocus.postgres com.apofocus.embedding com.apofocus.web; do
  plutil -lint "$LAUNCH_AGENT_DIR/$label.plist" >/dev/null
done
if [[ -n "$BACKUP_ROOT" ]]; then
  for label in com.apofocus.backup com.apofocus.backup-verify; do
    plutil -lint "$LAUNCH_AGENT_DIR/$label.plist" >/dev/null
  done
fi

CURRENT_STEP="LaunchAgent restart"
launchctl print "$DOMAIN" >/dev/null 2>&1 || fail "no macOS GUI login session is available for LaunchAgents"
for label in com.apofocus.backup-verify com.apofocus.backup; do
  launchctl bootout "$DOMAIN/$label" >/dev/null 2>&1 || true
done
for label in com.apofocus.web com.apofocus.embedding com.apofocus.postgres; do
  launchctl bootout "$DOMAIN/$label" >/dev/null 2>&1 || true
done
wait_for_port_release "$POSTGRES_PORT" || fail "PostgreSQL port $POSTGRES_PORT is already in use; choose another --postgres-port"
wait_for_port_release 8090 || fail "embedding port 8090 is already in use"
wait_for_port_release "$WEB_PORT" || fail "Web port $WEB_PORT is already in use; choose another --addr"
launchctl bootstrap "$DOMAIN" "$LAUNCH_AGENT_DIR/com.apofocus.postgres.plist"

CURRENT_STEP="PostgreSQL readiness"
ready=0
for _ in $(seq 1 60); do
  if PGPASSWORD="$DB_PASSWORD" "$POSTGRES_BIN/pg_isready" -h 127.0.0.1 -p "$POSTGRES_PORT" -U apofocus -d postgres >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.5
done
(( ready == 1 )) || fail "PostgreSQL did not become ready; see $LOG_DIR/postgres.error.log"

CURRENT_STEP="database creation and migrations"
PSQL=("$POSTGRES_BIN/psql" -v ON_ERROR_STOP=1 -h 127.0.0.1 -p "$POSTGRES_PORT" -U apofocus)
if ! PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d postgres -Atqc "SELECT 1 FROM pg_database WHERE datname='apofocus'" | grep -qx 1; then
  PGPASSWORD="$DB_PASSWORD" "$POSTGRES_BIN/createdb" -h 127.0.0.1 -p "$POSTGRES_PORT" -U apofocus apofocus
fi

relation_exists() {
  PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d apofocus -Atqc "SELECT to_regclass('public.$1') IS NOT NULL" | grep -qx t
}

if ! relation_exists photos; then
  PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d apofocus -f "$PROJECT_ROOT/migrations/000001_init.sql"
fi
PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d apofocus -f "$PROJECT_ROOT/migrations/000002_ingest.sql"
if ! relation_exists batch_jobs; then
  PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d apofocus -f "$PROJECT_ROOT/migrations/000003_folders_and_batch.sql"
fi
if ! relation_exists media_assets; then
  PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d apofocus -f "$PROJECT_ROOT/migrations/000004_multimedia.sql"
fi
if ! relation_exists storage_roots; then
  PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d apofocus -f "$PROJECT_ROOT/migrations/000005_storage_tracking.sql"
fi

vector_version="$(PGPASSWORD="$DB_PASSWORD" "${PSQL[@]}" -d apofocus -Atqc "SELECT extversion FROM pg_extension WHERE extname='vector'")"
[[ -n "$vector_version" ]] || fail "pgvector extension was not created"

CURRENT_STEP="ApoFocus services"
if (( NO_START == 0 )); then
  launchctl bootstrap "$DOMAIN" "$LAUNCH_AGENT_DIR/com.apofocus.embedding.plist"
  launchctl bootstrap "$DOMAIN" "$LAUNCH_AGENT_DIR/com.apofocus.web.plist"
  if [[ -n "$BACKUP_ROOT" ]]; then
    launchctl bootstrap "$DOMAIN" "$LAUNCH_AGENT_DIR/com.apofocus.backup.plist"
    launchctl bootstrap "$DOMAIN" "$LAUNCH_AGENT_DIR/com.apofocus.backup-verify.plist"
  fi

  embedding_ready=0
  web_ready=0
  for _ in $(seq 1 120); do
    if (( embedding_ready == 0 )) && curl -fsS http://127.0.0.1:8090/healthz >/dev/null 2>&1; then
      embedding_ready=1
    fi
    if (( web_ready == 0 )) && curl -fsS "$APP_URL/api/v1/photos?limit=1" >/dev/null 2>&1; then
      web_ready=1
    fi
    if (( embedding_ready == 1 && web_ready == 1 )); then
      break
    fi
    sleep 0.5
  done
  (( embedding_ready == 1 )) || fail "embedding service did not become ready; see $LOG_DIR/embedding.error.log"
  (( web_ready == 1 )) || fail "web service did not become ready; see $LOG_DIR/web.error.log"
else
  launchctl bootout "$DOMAIN/com.apofocus.postgres" >/dev/null 2>&1 || true
fi

CURRENT_STEP="complete"
echo
echo "ApoFocus installation complete."
echo "  Web:      $APP_URL"
echo "  Library:  $LIBRARY_ROOT"
echo "  Inbox:    $INBOX_ROOT"
echo "  Control:  $BIN_DIR/apofocusctl"
echo "  Config:   $CONFIG_FILE"
echo "  MCP:      $STATE_DIR/mcp-server.json"
echo "  Logs:     $LOG_DIR"
if [[ -n "$BACKUP_ROOT" ]]; then
  echo "  Backups:  $BACKUP_ROOT/postgres"
fi
if (( NO_START == 1 )); then
  echo "  Services are stopped; run: $BIN_DIR/apofocusctl start"
else
  echo "  Health:   $BIN_DIR/apofocusctl doctor"
fi
echo
echo "For external drives, macOS may ask for Files and Folders access."
echo "If access is denied, allow ApoFocus or your terminal in System Settings > Privacy & Security."
