#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
temp_dir="$(mktemp -d)"
log_file="${temp_dir}/server.log"
response_file="${temp_dir}/response"
cookie_file="${temp_dir}/cookies"
server_pid=""

cleanup() {
  if [[ -n "${server_pid}" ]]; then
    kill -TERM "${server_pid}" 2>/dev/null || true
    wait "${server_pid}" 2>/dev/null || true
  fi
  rm -rf "${temp_dir}"
}
trap cleanup EXIT

(cd "${repo_root}/backend" && go build -o "${temp_dir}/response-server" ./cmd/server)

start_backend() {
  RESPONSE_HTTP_ADDR=127.0.0.1:18080 RESPONSE_REPOSITORY_ROOT="${repo_root}" "${temp_dir}/response-server" >>"${log_file}" 2>&1 &
  server_pid=$!
}

wait_until_ready() {
  local health_ready=false
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
}

start_backend
wait_until_ready

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
if payload.get("engineVersion") != "base-abilities-v2":
    raise SystemExit("unexpected deterministic engine version")
PY

curl --fail --silent http://127.0.0.1:18080/metrics >"${response_file}"
for metric in response_active_rooms response_active_matches response_http_request_duration_seconds_bucket response_ws_command_duration_seconds_bucket response_rejects_total response_reconnects_total response_technical_aborts_total; do
  grep -q "${metric}" "${response_file}"
done

# A crash intentionally loses the process-local session/room. The restart must
# come up cleanly and must not pretend to recover the old state.
curl --fail --silent --cookie-jar "${cookie_file}" \
  -H 'Origin: http://127.0.0.1:5173' -H 'Content-Type: application/json' \
  --data '{}' http://127.0.0.1:18080/api/v1/sessions >"${response_file}"
csrf_token="$(python3 - "${response_file}" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as source:
    print(json.load(source)["csrfToken"])
PY
)"
curl --fail --silent --cookie "${cookie_file}" \
  -H 'Origin: http://127.0.0.1:5173' -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: smoke-room' -H "X-CSRF-Token: ${csrf_token}" \
  --data '{}' http://127.0.0.1:18080/api/v1/rooms >"${response_file}"
room_id="$(python3 - "${response_file}" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as source:
    print(json.load(source)["room"]["roomId"])
PY
)"

kill -KILL "${server_pid}"
wait "${server_pid}" 2>/dev/null || true
server_pid=""
start_backend
wait_until_ready
restart_status="$(curl --silent --output "${response_file}" --write-out '%{http_code}' \
  --cookie "${cookie_file}" -H 'Origin: http://127.0.0.1:5173' \
  "http://127.0.0.1:18080/api/v1/rooms/${room_id}")"
if [[ "${restart_status}" != 401 ]]; then
  echo "Restart unexpectedly recovered process-local state (HTTP ${restart_status})." >&2
  exit 1
fi

kill -TERM "${server_pid}"
wait "${server_pid}"
server_pid=""
python3 - "${log_file}" <<'PY'
import json
import sys

events = []
with open(sys.argv[1], encoding="utf-8") as source:
    for line in source:
        payload = json.loads(line)
        encoded = json.dumps(payload)
        for forbidden in ("response_session", "ticket", "seed", "hand"):
            if forbidden in encoded.lower():
                raise SystemExit(f"sensitive field in structured log: {forbidden}")
        events.append(payload.get("msg"))
for required in ("local server started", "controlled shutdown initiated", "local server stopped"):
    if required not in events:
        raise SystemExit(f"missing structured log event: {required}")
PY

echo 'Local backend smoke test passed.'

