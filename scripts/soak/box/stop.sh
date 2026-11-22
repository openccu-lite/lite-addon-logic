#!/bin/sh
#   Ends the sampler and the load start.sh started, by the process ids it wrote down.
for what in sample load; do
    if [ -f "/tmp/soak-$what.pid" ]; then
        kill "$(cat "/tmp/soak-$what.pid")" 2>/dev/null && echo "$what ended"
        rm -f "/tmp/soak-$what.pid"
    fi
done
