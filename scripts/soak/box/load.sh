#!/bin/sh
#
#   The soak's load, on the system under test: writes a random value to a random input variable
#   through the interface "Logic" on the loopback, as other software would (setValue on
#   var-<key>:1 VALUE), $SOAK_RATE times a second on average (2 by default), for ever. What it
#   has written since it started is in /tmp/soak-load.count; the moments the interface stopped
#   and started answering, and every write the addon refused, are in $SOAK_DIR/load.log.
#
DIR=${SOAK_DIR:-/usr/local/soak}
URL=${SOAK_URL:-http://127.0.0.1:31310/}
PAUSE=${SOAK_PAUSE:-0.5}
INPUTS=$DIR/inputs.txt
n=$(wc -l < "$INPUTS")
[ "$n" -gt 0 ] || { echo "no inputs in $INPUTS" >&2; exit 1; }
count=0
up=1
echo "$(date +%s) load started, $n inputs" >> "$DIR/load.log"
while :; do
    r=$(od -An -N4 -tu4 /dev/urandom | tr -d ' ')
    set -- $(sed -n "$((r % n + 1))p" "$INPUTS")
    v=$(((r / 151) % 101))
    case $2 in
        bool) value="<boolean>$((v % 2))</boolean>" ;;
        number) value="<double>$v</double>" ;;
        list) case $((v % 3)) in 0) e=aus ;; 1) e=halb ;; *) e=voll ;; esac; value="<string>$e</string>" ;;
        *) value="<string>text $((v % 5))</string>" ;;
    esac
    body="<?xml version=\"1.0\"?><methodCall><methodName>setValue</methodName><params><param><value><string>var-$1:1</string></value></param><param><value><string>VALUE</string></value></param><param><value>$value</value></param></params></methodCall>"
    if curl -s -m 10 -o /tmp/soak-load.answer -H 'Content-Type: text/xml' -d "$body" "$URL"; then
        [ "$up" = 1 ] || { echo "$(date +%s) the interface answers again" >> "$DIR/load.log"; up=1; }
        if grep -q '<fault>' /tmp/soak-load.answer; then
            echo "$(date +%s) refused: $1 $value: $(tr -d '\n' < /tmp/soak-load.answer | sed 's/<[^>]*>/ /g' | tr -s ' ')" >> "$DIR/load.log"
        fi
        count=$((count + 1))
        echo "$count" > /tmp/soak-load.count
    else
        [ "$up" = 0 ] || { echo "$(date +%s) the interface does not answer" >> "$DIR/load.log"; up=0; }
        sleep 1
    fi
    sleep "$PAUSE"
done
