#!/bin/sh
set -eu

image=${E2E_IMAGE:-v2ray_scrapper-v2ray-scrapper:latest}
port=${E2E_PORT:-18085}
container="v2ray-scrapper-e2e-$$"
data_dir=$(mktemp -d "${TMPDIR:-/tmp}/v2ray-scrapper-e2e.XXXXXX")
base="http://127.0.0.1:${port}"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$data_dir"
}
trap cleanup EXIT INT TERM

docker run -d --name "$container" --user "$(id -u):$(id -g)" -e HOME=/tmp \
  -p "${port}:8084" -v "${data_dir}:/data" "$image" >/dev/null
started=$(date +%s)

ready=false
attempt=1
while [ "$attempt" -le 60 ]; do
  code=$(curl -sS -o "$data_dir/cache.json" -w '%{http_code}' "$base/cache" || true)
  if [ "$code" = 200 ]; then
    ready=true
    break
  fi
  sleep 2
  attempt=$((attempt + 1))
done

if [ "$ready" != true ]; then
  docker logs "$container"
  echo "FAIL: first usable cache exceeded 120 seconds" >&2
  exit 1
fi

cold_seconds=$(($(date +%s) - started))
[ "$cold_seconds" -lt 120 ] || { echo "FAIL: cold start took ${cold_seconds}s" >&2; exit 1; }

curl -fsS "$base/health" >"$data_dir/health.json"
curl -fsS "$base/openapi.json" >"$data_dir/openapi.json"
curl -fsS "$base/swagger" >"$data_dir/swagger.html"
curl -fsS "$base/cache/raw" >"$data_dir/cache.raw"
curl -fsS "$base/cache/base64" >"$data_dir/cache.b64"
curl -fsS "$base/cache/all/base64" >"$data_dir/cache-all.b64"

python3 - "$data_dir" <<'PY'
import base64
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
assert json.loads((root / "health.json").read_text())["status"] == "ok"
cache = json.loads((root / "cache.json").read_text())
assert cache["count"] == len(cache["servers"]) > 0
spec = json.loads((root / "openapi.json").read_text())
assert spec["openapi"] == "3.1.0" and len(spec["paths"]) == 9
assert "SwaggerUIBundle" in (root / "swagger.html").read_text()
raw = (root / "cache.raw").read_bytes()
assert base64.b64decode((root / "cache.b64").read_bytes()) == raw
assert base64.b64decode((root / "cache-all.b64").read_bytes()).startswith(raw)
PY

curl -fsS -X POST -H 'Content-Type: text/plain' --data-binary "@$data_dir/cache.raw" \
  "$base/subscription/test" >"$data_dir/custom.json"
python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); assert d["count"] > 0' "$data_dir/custom.json"

site_code=$(curl -sS -o "$data_dir/site.b64" -w '%{http_code}' --get \
  --data-urlencode 'url=http://www.google.com/generate_204' "$base/subscription/site-specific")
[ "$site_code" = 200 ] || { echo "FAIL: site-specific endpoint returned $site_code" >&2; exit 1; }

restart_started=$(date +%s)
docker restart "$container" >/dev/null
attempt=1
while [ "$attempt" -le 25 ]; do
  code=$(curl -sS -o "$data_dir/restarted-cache.json" -w '%{http_code}' "$base/cache" || true)
  [ "$code" = 200 ] && break
  sleep 0.2
  attempt=$((attempt + 1))
done
[ "$code" = 200 ] || { echo "FAIL: persisted cache unavailable after restart" >&2; exit 1; }
restart_seconds=$(($(date +%s) - restart_started))

count=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["count"])' "$data_dir/cache.json")
echo "PASS: cold_cache=${cold_seconds}s working=${count} warm_cache<=${restart_seconds}s sources=3 refresh=600s"
