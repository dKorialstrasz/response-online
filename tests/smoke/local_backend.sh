#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
log_file="$(mktemp)"
response_file="$(mktemp)"
server_pid=""

cleanup() {
  if [[ -n "${server_pid}" ]]; then
    kill -TERM "${server_pid}" 2>/dev/null || true
    wait "${server_pid}" 2>/dev/null || true
  fi
  rm -f "${log_file}" "${response_file}"
}
trap cleanup EXIT

(
  cd "${repo_root}/backend"
  RESPONSE_HTTP_ADDR=127.0.0.1:18080 go run ./cmd/server >"${log_file}" 2>&1
) &
server_pid=$!

health_ready=false
for _ in {1..300}; do
  if curl --fail --silent http://127.0.0.1:18080/healthz >"${response_file}"; then
    health_ready=true
    break
  fi
  sleep 0.1
done

if [[ "${health_ready}" != true ]]; then
  cat "${log_file}" >&2
  echo 'Local backend did not become ready within 30 seconds.' >&2
  exit 1
fi

python3 - "${response_file}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    payload = json.load(source)
if payload != {"status": "ok"}:
    raise SystemExit(f"unexpected health response: {payload!r}")
PY

curl --fail --silent http://127.0.0.1:18080/api/v1/build >"${response_file}"
python3 - "${response_file}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    payload = json.load(source)
if payload.get("rulesVersion") != "response-rules-2026-10-17":
    raise SystemExit("unexpected core rules version")
if payload.get("contentVersion") != "base-cards-v1":
    raise SystemExit("unexpected content version")
if payload.get("engineVersion") != "deterministic-core-v1":
    raise SystemExit("unexpected deterministic engine version")
PY

echo 'Local backend smoke test passed.'

