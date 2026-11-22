#!/bin/sh
#
#   The soak's sampler, on the system under test, as root: one line a minute into
#   $SOAK_DIR/samples.csv about the addon's daemon - memory, threads, open files, bytes written
#   to storage - and about the system beside it. A daemon that is not running gives a line
#   with pid 0.
#
#     epoch,pid,rss_kb,threads,fds,write_bytes,tree_kb,state_kb,mem_available_kb,load_writes
#
DIR=${SOAK_DIR:-/usr/local/soak}
CONF=${SOAK_CONF:-/usr/local/etc/config/addons/logic}
BIN=${SOAK_BIN:-/usr/local/addons/logic/bin/logic}
EVERY=${SOAK_EVERY:-60}
OUT=$DIR/samples.csv
[ -f "$OUT" ] || echo "epoch,pid,rss_kb,threads,fds,write_bytes,tree_kb,state_kb,mem_available_kb,load_writes" > "$OUT"
while :; do
    pid=0
    for p in $(pidof "${BIN##*/}"); do
        [ "$(readlink "/proc/$p/exe")" = "$BIN" ] && pid=$p
    done
    rss=0; threads=0; fds=0; wb=0
    if [ "$pid" != 0 ]; then
        rss=$(awk '/^VmRSS:/ {print $2}' "/proc/$pid/status")
        threads=$(awk '/^Threads:/ {print $2}' "/proc/$pid/status")
        fds=$(ls "/proc/$pid/fd" 2>/dev/null | wc -l)
        wb=$(awk '/^write_bytes:/ {print $2}' "/proc/$pid/io")
    fi
    tree=$(du -sk "$CONF/tree" 2>/dev/null | cut -f1)
    state=$(du -sk "$CONF/state" 2>/dev/null | cut -f1)
    avail=$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)
    writes=$(cat /tmp/soak-load.count 2>/dev/null || echo 0)
    echo "$(date +%s),$pid,${rss:-0},${threads:-0},$fds,${wb:-0},${tree:-0},${state:-0},$avail,$writes" >> "$OUT"
    sleep "$EVERY"
done
