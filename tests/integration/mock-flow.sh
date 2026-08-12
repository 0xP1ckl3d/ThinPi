#!/bin/sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
WORK=$(mktemp -d)
CONTROLLER_PID=""; AGENT_PID=""
cleanup(){ [ -z "$AGENT_PID" ] || kill "$AGENT_PID" 2>/dev/null || true; [ -z "$CONTROLLER_PID" ] || kill "$CONTROLLER_PID" 2>/dev/null || true; rm -rf "$WORK"; }
trap cleanup EXIT INT TERM
(cd "$ROOT/controller" && go run ./cmd/thinpi-controller serve --dev --listen 127.0.0.1:18080 --database "$WORK/test.db") & CONTROLLER_PID=$!
for _ in $(seq 1 50); do curl -fsS http://127.0.0.1:18080/healthz >/dev/null 2>&1 && break; sleep .2; done
printf '{"controller_url":"http://127.0.0.1:18080","device_file":"%s/missing.json","socket":"%s/agent.sock","mock_clients":true,"mock_duration_seconds":1}\n' "$WORK" "$WORK" > "$WORK/agent.json"
THINPI_AGENT_MOCK_CLIENTS=1 "$ROOT/bin/thinpi-agent" serve --config "$WORK/agent.json" & AGENT_PID=$!
for _ in $(seq 1 50); do [ -S "$WORK/agent.sock" ] && break; sleep .2; done
LOGIN=$(curl -fsS -H 'Content-Type: application/json' -d '{"username":"daughter","password":"thinpi-dev"}' http://127.0.0.1:18080/api/v1/auth/login)
USER_TOKEN=$(printf '%s' "$LOGIN" | jq -r .token)
CONNECTION_ID=$(curl -fsS -H "Authorization: Bearer $USER_TOKEN" http://127.0.0.1:18080/api/v1/connections | jq -r '.items[0].id')
TICKET=$(curl -fsS -H "Authorization: Bearer $USER_TOKEN" -H 'Content-Type: application/json' -d '{"device_identifier":"dev-device"}' "http://127.0.0.1:18080/api/v1/connections/$CONNECTION_ID/launch" | jq -r .ticket)
RESPONSE=$(printf '{"action":"launch","ticket":"%s"}\n' "$TICKET" | socat - UNIX-CONNECT:"$WORK/agent.sock")
printf '%s' "$RESPONSE" | jq -e '.accepted == true' >/dev/null
echo "Mock login/ACL/ticket/agent/session flow passed"
