#!/bin/sh
#
#   Kills the addon's daemon with SIGKILL, $1 times (100) with $2 seconds between two (60 - the
#   supervisor warns of a crash loop at 15 restarts in ten minutes),
#   while the load goes on, and waits for it to come back. The system's own supervisor is what
#   starts an addon's daemon again (the manifest says runtime.daemon); only when the interface
#   has not answered after $SOAK_WAIT seconds (90) does this script start the unit itself, and
#   says so. One line per kill into $SOAK_DIR/kills.csv: when, how many seconds until the
#   interface answered again, who started it (supervisor or fallback), and how many programs and
#   variables it listed then - the numbers must be the same every time.
#
#     epoch,seconds_until_back,by,programs,variables
#
DIR=${SOAK_DIR:-/usr/local/soak}
URL=${SOAK_URL:-http://127.0.0.1:31310/}
BIN=${SOAK_BIN:-/usr/local/addons/logic/bin/logic}
UNIT=${SOAK_UNIT:-addon-logic}
TIMES=${1:-100}
GAP=${2:-60}
WAIT=${SOAK_WAIT:-90}
OUT=$DIR/kills.csv
[ -f "$OUT" ] || echo "epoch,seconds_until_back,by,programs,variables" > "$OUT"
list='<?xml version="1.0"?><methodCall><methodName>listDevices</methodName><params></params></methodCall>'
i=0
while [ "$i" -lt "$TIMES" ]; do
    i=$((i + 1))
    for p in $(pidof "${BIN##*/}"); do
        [ "$(readlink "/proc/$p/exe")" = "$BIN" ] && kill -9 "$p"
    done
    start=$(date +%s)
    back=-1
    by=supervisor
    t=0
    while [ "$t" -lt $((WAIT + 120)) ]; do
        if [ "$t" = "$WAIT" ]; then
            by=fallback
            systemctl restart "$UNIT"
        fi
        if curl -s -m 10 -o /tmp/soak-kill.answer -H 'Content-Type: text/xml' -d "$list" "$URL" && grep -q 'LOGIC-PROGRAM' /tmp/soak-kill.answer; then
            back=$(($(date +%s) - start))
            break
        fi
        sleep 1
        t=$((t + 1))
    done
    programs=$(grep -o '<string>prg-[^<:]*</string>' /tmp/soak-kill.answer 2>/dev/null | sort -u | wc -l)
    variables=$(grep -o '<string>var-[^<:]*</string>' /tmp/soak-kill.answer 2>/dev/null | sort -u | wc -l)
    echo "$start,$back,$by,$programs,$variables" >> "$OUT"
    sleep "$GAP"
done
