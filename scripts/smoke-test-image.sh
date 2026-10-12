#!/usr/bin/env bash
# Starts a built container image and checks that it really works: the health
# endpoint answers, the embedded UI is served, and the admin API is locked.
# CI runs this before an image is published; you can run it locally too:
#
#   scripts/smoke-test-image.sh ghcr.io/nimdanitro/go-sonos-kids:main
set -euo pipefail

image="${1:?usage: $0 <image>}"
port="${PORT:-18080}"
name="sonoskids-smoke-$$"
data="$(mktemp -d)"
base="http://127.0.0.1:${port}"

cleanup() {
  docker rm -f "$name" >/dev/null 2>&1 || true
  rm -rf -- "$data"
}
trap cleanup EXIT

fail() {
  echo "::error::smoke test failed: $*" >&2
  echo "--- container log:" >&2
  docker logs "$name" >&2 || true
  exit 1
}

chmod 777 "$data" # the image runs as a non-root user; :Z relabels the mount on SELinux hosts
docker run -d --name "$name" --network host \
  -v "$data:/data:Z" \
  -e SONOSKIDS_DB=/data/sonoskids.db \
  -e SONOSKIDS_ADDR="127.0.0.1:${port}" \
  -e SONOSKIDS_ADMIN_TOKEN=smoke \
  "$image" serve >/dev/null

for _ in $(seq 1 30); do
  curl -fsS "$base/healthz" >/dev/null 2>&1 && break
  sleep 1
done

curl -fsS "$base/healthz" | grep -q '"status":"ok"' || fail "/healthz did not answer ok"
curl -fsS "$base/" | grep -q 'id="root"' || fail "the embedded UI was not served (was the UI built before the image?)"
[ "$(curl -s -o /dev/null -w '%{http_code}' "$base/api/admin/profiles")" = 401 ] || fail "the admin API is not locked without a token"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer smoke' "$base/api/admin/profiles")" = 200 ] || fail "the admin API rejects the configured token"
[ -s "$data/sonoskids.db" ] || fail "the database was not created on the mounted volume"

echo "smoke test passed for $image"
