#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
project_root=$(dirname "$script_dir")
test_root=$(mktemp -d "${TMPDIR:-/tmp}/v2ray-init-test.XXXXXX")

cleanup() {
  rm -rf "$test_root"
}
trap cleanup EXIT INT TERM

cp "$project_root/.env.sample" "$test_root/.env.sample"
cp "$project_root/config.yaml.sample" "$test_root/config.yaml.sample"

INIT_ROOT="$test_root" \
INIT_NON_INTERACTIVE=true \
INIT_HOST_PORT=19084 \
INIT_REFRESH_SECONDS=300 \
INIT_MAX_CANDIDATES=40 \
  sh "$script_dir/init.sh" >/dev/null

grep -qx 'HOST_PORT=19084' "$test_root/.env"
grep -qx 'CACHE_INTERVAL_SECONDS=300' "$test_root/.env"
grep -qx 'MAX_CANDIDATES=40' "$test_root/.env"
grep -Eq '^MANAGEMENT_TOKEN=[0-9a-f]{48}$' "$test_root/.env"
[ -f "$test_root/config.yaml" ]
[ -d "$test_root/data" ]
[ -d "$test_root/bin" ]
[ -d "$test_root/.cache/go-build" ]
[ -d "$test_root/.cache/go-mod" ]
case $(uname -s) in
  Darwin) env_mode=$(stat -f '%Lp' "$test_root/.env") ;;
  *) env_mode=$(stat -c '%a' "$test_root/.env") ;;
esac
[ "$env_mode" = 600 ]

printf '\nUSER_VALUE=preserved\n' >>"$test_root/.env"
INIT_ROOT="$test_root" INIT_NON_INTERACTIVE=true sh "$script_dir/init.sh" >/dev/null
grep -qx 'USER_VALUE=preserved' "$test_root/.env"

INIT_ROOT="$test_root" \
INIT_NON_INTERACTIVE=true \
INIT_HOST_PORT=18084 \
  sh "$script_dir/init.sh" --force >/dev/null
grep -qx 'HOST_PORT=18084' "$test_root/.env"
if grep -q 'USER_VALUE=preserved' "$test_root/.env"; then
  echo "--force did not replace .env" >&2
  exit 1
fi

if INIT_ROOT="$test_root" INIT_NON_INTERACTIVE=true INIT_HOST_PORT=70000 \
  sh "$script_dir/init.sh" --force >/dev/null 2>&1; then
  echo "invalid port was accepted" >&2
  exit 1
fi

legacy_root="$test_root/legacy"
mkdir -p "$legacy_root"
cp "$project_root/.env.sample" "$legacy_root/.env.sample"
cp "$project_root/config.yaml.sample" "$legacy_root/config.yaml.sample"
cat >"$legacy_root/.env" <<'EOF'
UVICORN_HOST=0.0.0.0
REDIS_HOST=redis
BATCH_SIZE=500
TEST_TIMEOUT=10
CACHE_INTERVAL_SECONDS=900
SUB_URLS=https://example.test/custom-subscription
GITHUB_TOKEN=test
EOF
INIT_ROOT="$legacy_root" INIT_NON_INTERACTIVE=true sh "$script_dir/init.sh" >/dev/null
grep -qx 'BATCH_SIZE=100' "$legacy_root/.env"
grep -qx 'TEST_TIMEOUT=6' "$legacy_root/.env"
grep -qx 'CACHE_INTERVAL_SECONDS=600' "$legacy_root/.env"
grep -qx 'MAX_CONCURRENT_BATCHES=10' "$legacy_root/.env"
grep -qx 'MAX_CONCURRENT_TESTS=100' "$legacy_root/.env"
grep -qx 'TEST_ATTEMPTS=2' "$legacy_root/.env"
grep -qx 'MAX_CANDIDATES=10000' "$legacy_root/.env"
grep -qx 'STATE_FILE_PATH=/data/state.json' "$legacy_root/.env"
grep -qx 'SUB_URLS=https://example.test/custom-subscription' "$legacy_root/.env"
grep -qx 'GITHUB_TOKEN=test' "$legacy_root/.env"

echo "init tests passed"
