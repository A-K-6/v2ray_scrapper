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
  INIT_MAX_CANDIDATES    Maximum proxies tested per refresh (default: 60)
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

env_file="$project_root/.env"
config_file="$project_root/config.yaml"
env_sample="$project_root/.env.sample"
config_sample="$project_root/config.yaml.sample"

[ -f "$env_sample" ] || { echo "Missing $env_sample" >&2; exit 1; }
[ -f "$config_sample" ] || { echo "Missing $config_sample" >&2; exit 1; }

if [ -f "$env_file" ] && [ "$force" != true ]; then
  echo "Keeping existing .env"
else
  prompt "Docker host port" "8084" "${INIT_HOST_PORT:-}"
  host_port=$REPLY
  prompt "Refresh interval in seconds" "600" "${INIT_REFRESH_SECONDS:-}"
  refresh_seconds=$REPLY
  prompt "Maximum candidates per refresh" "60" "${INIT_MAX_CANDIDATES:-}"
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
