#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_root=${INIT_ROOT:-$(dirname "$script_dir")}
force=false
non_interactive=${INIT_NON_INTERACTIVE:-false}

usage() {
  cat <<'EOF'
Usage: make init [ARGS="--non-interactive|--force"]

Creates .env, config.yaml, data/, bin/, and local Go cache directories.
Existing configuration is preserved unless --force is supplied.

Environment overrides for automation:
  INIT_HOST_PORT         Host port exposed by Docker (default: 8084)
  INIT_REFRESH_SECONDS   Refresh interval in seconds (default: 600)
  INIT_MAX_CANDIDATES    Maximum proxies tested per refresh (default: 10000)
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --force) force=true ;;
    --non-interactive) non_interactive=true ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
  shift
done

prompt() {
  label=$1
  default=$2
  override=$3

  if [ -n "$override" ]; then
    REPLY=$override
  elif [ "$non_interactive" = true ] || [ ! -t 0 ]; then
    REPLY=$default
  else
    printf '%s [%s]: ' "$label" "$default" >&2
    IFS= read -r REPLY
    REPLY=${REPLY:-$default}
  fi
}

require_integer() {
  name=$1
  value=$2
  minimum=$3
  maximum=$4
  case "$value" in
    ''|*[!0-9]*) echo "$name must be an integer" >&2; exit 2 ;;
  esac
  if [ "$value" -lt "$minimum" ] || [ "$value" -gt "$maximum" ]; then
    echo "$name must be between $minimum and $maximum" >&2
    exit 2
  fi
}

set_env_value() {
  file=$1
  key=$2
  value=$3
  temporary="${file}.tmp.$$"
  awk -v key="$key" -v value="$value" '
    BEGIN { found = 0 }
    index($0, key "=") == 1 { print key "=" value; found = 1; next }
    { print }
    END { if (!found) print key "=" value }
  ' "$file" >"$temporary"
  mv "$temporary" "$file"
}

merge_missing_env() {
  source_file=$1
  target_file=$2
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      ''|'#'*) continue ;;
      *=*) key=${line%%=*} ;;
      *) continue ;;
    esac
    if ! grep -q "^${key}=" "$target_file"; then
      printf '\n%s\n' "$line" >>"$target_file"
    fi
  done <"$source_file"
}

ensure_management_token() {
  file=$1
  current=$(awk -F= '$1 == "MANAGEMENT_TOKEN" { print substr($0, index($0, "=") + 1); exit }' "$file")
  if [ -n "$current" ]; then
    return
  fi
  generated=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
  set_env_value "$file" MANAGEMENT_TOKEN "$generated"
  chmod 600 "$file"
  echo "Generated a management API token in .env"
}

migrate_legacy_env() {
  file=$1
  if ! grep -Eq '^(UVICORN_HOST|UVICORN_PORT|REDIS_HOST|PYTHONPATH)=' "$file"; then
    return
  fi
  set_env_value "$file" FETCH_TIMEOUT 20
  set_env_value "$file" TEST_TIMEOUT 6
  set_env_value "$file" XRAY_START_TIMEOUT 5
  set_env_value "$file" BATCH_SIZE 100
  set_env_value "$file" MAX_CONCURRENT_BATCHES 10
  set_env_value "$file" MAX_CONCURRENT_TESTS 100
  set_env_value "$file" TEST_ATTEMPTS 2
  set_env_value "$file" MAX_CANDIDATES 10000
  set_env_value "$file" MAX_DELAY_MS 10000
  set_env_value "$file" CACHE_INTERVAL_SECONDS 600
  set_env_value "$file" STATE_FILE_PATH /data/state.json
  echo "Migrated legacy Python-era runtime settings in .env"
}

env_file="$project_root/.env"
config_file="$project_root/config.yaml"
env_sample="$project_root/.env.sample"
config_sample="$project_root/config.yaml.sample"

[ -f "$env_sample" ] || { echo "Missing $env_sample" >&2; exit 1; }
[ -f "$config_sample" ] || { echo "Missing $config_sample" >&2; exit 1; }

if [ -f "$env_file" ] && [ "$force" != true ]; then
  migrate_legacy_env "$env_file"
  merge_missing_env "$env_sample" "$env_file"
  chmod 600 "$env_file"
  echo "Kept existing .env values and added missing Go defaults"
else
  prompt "Docker host port" "8084" "${INIT_HOST_PORT:-}"
  host_port=$REPLY
  prompt "Refresh interval in seconds" "600" "${INIT_REFRESH_SECONDS:-}"
  refresh_seconds=$REPLY
  prompt "Maximum candidates per refresh" "10000" "${INIT_MAX_CANDIDATES:-}"
  max_candidates=$REPLY

  require_integer "Docker host port" "$host_port" 1 65535
  require_integer "Refresh interval" "$refresh_seconds" 30 86400
  require_integer "Maximum candidates" "$max_candidates" 1 10000

  cp "$env_sample" "$env_file"
  set_env_value "$env_file" HOST_PORT "$host_port"
  set_env_value "$env_file" CACHE_INTERVAL_SECONDS "$refresh_seconds"
  set_env_value "$env_file" MAX_CANDIDATES "$max_candidates"
  chmod 600 "$env_file"
  echo "Created .env"
fi

ensure_management_token "$env_file"

if [ -f "$config_file" ] && [ "$force" != true ]; then
  echo "Keeping existing config.yaml"
else
  cp "$config_sample" "$config_file"
  echo "Created config.yaml"
fi

mkdir -p \
  "$project_root/data" \
  "$project_root/bin" \
  "$project_root/.cache/go-build" \
  "$project_root/.cache/go-mod"

echo
echo "Initialization complete."
echo "Start the service: docker compose up -d --build"
swagger_port=$(awk -F= '$1 == "HOST_PORT" { print $2; exit }' "$env_file")
swagger_port=${swagger_port:-8084}
echo "Open Swagger:     http://localhost:${swagger_port}/swagger"
