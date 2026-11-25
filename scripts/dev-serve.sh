#!/bin/sh
#
#   Runs the daemon against the fake system, for the page's development and the Playwright
#   tests: the fake system on 127.0.0.1:18081, the page on http://127.0.0.1:18301/addons/logic/
#   (no session check), the interface "Logic" on 127.0.0.1:31311. A fresh data directory each run
#   unless LOGIC_CONF_DIR names one. The page is served from ui/dist (npm run build first).
#
#   A second one beside it, for the time modules' tests: LOGIC_PORT, LOGIC_RPC_PORT and
#   LOGIC_SYSTEM_PORT name other ports, LOGIC_DEV_CLOCK (RFC 3339) starts the daemon on a clock
#   that stands still until /api/v1/dev/clock moves it, LOGIC_SEED names a directory whose
#   content becomes the tree (programs/, variables/), and TZ is the daemon's time zone.
#
set -e
cd "$(dirname "$0")/.."
PORT=${LOGIC_PORT:-18301}
RPC_PORT=${LOGIC_RPC_PORT:-31311}
SYSTEM_PORT=${LOGIC_SYSTEM_PORT:-18081}
DIR=${LOGIC_CONF_DIR:-$(mktemp -d)}
mkdir -p build "$DIR/addon"
# (two of these may start at once: one builds, the other waits)
flock build/.build.lock go build -o build/fakesystem ./cmd/fakesystem
flock build/.build.lock go build -o build/logic-dev ./cmd/logic
echo dev > "$DIR/token"
if [ -n "$LOGIC_SEED" ]; then
    mkdir -p "$DIR/tree"
    cp -r "$LOGIC_SEED"/. "$DIR/tree/"
fi
# (the fake system takes the daemon's Wake-on-LAN packets on a UDP port of the same number)
build/fakesystem -addr "127.0.0.1:$SYSTEM_PORT" -wake-port "$SYSTEM_PORT" &
FAKE=$!
sleep 0.5
build/logic-dev -system "http://127.0.0.1:$SYSTEM_PORT" -no-session-check -port "$PORT" -rpc-port "$RPC_PORT" -conf-dir "$DIR" -addon-dir "$DIR/addon" -www ui/dist -token-file "$DIR/token" -dev-wake-port "$SYSTEM_PORT" ${LOGIC_DEV_CLOCK:+-dev-clock "$LOGIC_DEV_CLOCK"} &
DAEMON=$!
trap 'kill $DAEMON $FAKE 2>/dev/null' EXIT INT TERM
wait $DAEMON
