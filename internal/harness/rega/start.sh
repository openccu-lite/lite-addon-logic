#!/bin/sh
# The clock is real time plus the offset in /clock/offset (libfaketime's "+<seconds>s x<rate>"),
# read once when the container starts: the harness writes that file, then starts (or restarts)
# the container. The multi-threaded build of libfaketime, and the offset from the environment
# rather than from a file read on every call: ReGaHss asks for the time from many threads, and
# with the plain library re-reading the file each time one call in some ten thousand answered
# the real time - a jump of months there and back after which ReGaHss no longer ran programs.
[ -f /clock/offset ] || echo "+0s" > /clock/offset
FAKETIME=$(cat /clock/offset)
export FAKETIME
export LD_PRELOAD=/usr/lib/x86_64-linux-gnu/faketime/libfaketimeMT.so.1
export FAKETIME_CACHE_DURATION=1000000000
exec /bin/ReGaHss -f /etc/rega.conf -c -l 0
