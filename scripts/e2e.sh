#!/usr/bin/env bash
# End-to-end smoke test: serve a local HTTP file with `clowderd serve`,
# reach it through a second `clowderd dial`, and fetch it with curl.
#
# Requires network access to a DERP relay. Ports are fixed so the test is
# predictable; override with E2E_HTTP_PORT and E2E_LOCAL_PORT.
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${CLOWDERD:-$ROOT/bin/clowderd}"
HTTP_PORT="${E2E_HTTP_PORT:-18081}"
LOCAL_PORT="${E2E_LOCAL_PORT:-19001}"
DIR="$(mktemp -d)"

if [[ ! -x "$BIN" ]]; then
    echo "missing $BIN; run: just cli" >&2
    exit 1
fi

echo "clowder e2e: serving $HTTP_PORT, mapping local $LOCAL_PORT"

echo "hello-clowder-$$" >"$DIR/index.html"
python3 -m http.server "$HTTP_PORT" --bind 127.0.0.1 --directory "$DIR" >"$DIR/http.log" 2>&1 &
HPID=$!

CLOWDER_CONFIG_DIR="$DIR/cfg" "$BIN" serve -expose "$HTTP_PORT" >"$DIR/serve.log" 2>&1 &
SPID=$!
DPID=""

cleanup() {
    kill "$SPID" "$HPID" ${DPID:+"$DPID"} 2>/dev/null
    rm -rf "$DIR"
}
trap cleanup EXIT

ADDR=""
for _ in $(seq 1 60); do
    ADDR="$(grep -m1 '^tc' "$DIR/serve.log" 2>/dev/null || true)"
    [[ -n "$ADDR" ]] && break
    sleep 0.5
done
if [[ -z "$ADDR" ]]; then
    echo "the host never produced an address:" >&2
    tail -20 "$DIR/serve.log" >&2
    exit 1
fi

CLOWDER_CONFIG_DIR="$DIR/cfg" "$BIN" dial -addr "$ADDR" \
    -forward "$LOCAL_PORT:$HTTP_PORT" >"$DIR/dial.log" 2>&1 &
DPID=$!

for _ in $(seq 1 120); do
    grep -q "mapped local" "$DIR/dial.log" 2>/dev/null && break
    sleep 0.5
done
if ! grep -q "mapped local" "$DIR/dial.log"; then
    echo "the dial side never mapped the port:" >&2
    tail -20 "$DIR/dial.log" >&2
    exit 1
fi

BODY="$(curl -sS -m 15 "http://127.0.0.1:$LOCAL_PORT/" 2>&1)"
if [[ "$BODY" != hello-clowder-* ]]; then
    echo "unexpected response: $BODY" >&2
    exit 1
fi

echo "ok: tunnel carried the request ($BODY)"
