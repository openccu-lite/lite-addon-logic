#!/bin/sh
#
#   Starts the sampler and the load in the background (after the installation, and again after
#   every start of the system: nothing here starts by itself). Their process ids are in
#   /tmp/soak-sample.pid and /tmp/soak-load.pid; stop.sh ends them.
#
DIR=${SOAK_DIR:-/usr/local/soak}
for what in sample load; do
    if [ -f "/tmp/soak-$what.pid" ] && kill -0 "$(cat "/tmp/soak-$what.pid")" 2>/dev/null; then
        echo "$what runs already ($(cat "/tmp/soak-$what.pid"))"
        continue
    fi
    setsid nohup "$DIR/$what.sh" > "/tmp/soak-$what.out" 2>&1 < /dev/null &
    echo $! > "/tmp/soak-$what.pid"
    echo "$what started ($!)"
done
