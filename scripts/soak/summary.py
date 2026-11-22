#!/usr/bin/env python3
"""What the soak's samples say: for each column the first and the last value, the smallest and
the largest, and the trend per day (a least-squares line through the samples of the daemon that
ran longest without a restart). Reads samples.csv and, when they are beside it, kills.csv and
load.log.

    summary.py <folder with samples.csv>
"""
import csv
import os
import sys


def trend(points):
    """The slope of the least-squares line through (x, y) points, per unit of x."""
    n = len(points)
    if n < 2:
        return 0.0
    mx = sum(x for x, _ in points) / n
    my = sum(y for _, y in points) / n
    den = sum((x - mx) ** 2 for x, _ in points)
    return sum((x - mx) * (y - my) for x, y in points) / den if den else 0.0


def main(folder):
    # a reset without a sync can leave a line of zero bytes where the sample of that minute
    # was being written: such a line is counted and left out
    text = open(os.path.join(folder, "samples.csv"), errors="replace").read()
    damaged = text.count("\0") > 0
    lines = [line.replace("\0", "").strip() for line in text.splitlines()]
    rows, broken = [], 0
    for r in csv.DictReader(line for line in lines if line):
        try:
            rows.append({k: int(v) for k, v in r.items()})
        except (TypeError, ValueError):
            broken += 1
    if damaged or broken:
        print(f"samples.csv: {broken} lines that are no sample left out" + (", zero bytes removed" if damaged else "") + " (a reset while a line was written)")
    if not rows:
        print("no samples")
        return 1
    span = rows[-1]["epoch"] - rows[0]["epoch"]
    down = sum(1 for r in rows if r["pid"] == 0)
    pids = []
    for r in rows:
        if r["pid"] and (not pids or pids[-1] != r["pid"]):
            pids.append(r["pid"])
    print(f"{len(rows)} samples over {span / 3600:.1f} h; {len(pids)} daemon processes; {down} samples without one")
    # the longest stretch of one process: what a leak would show in
    best, cur = [], []
    for r in rows:
        if cur and r["pid"] != cur[-1]["pid"]:
            cur = []
        if r["pid"]:
            cur.append(r)
            if len(cur) > len(best):
                best = list(cur)
    print(f"longest run of one process: {len(best)} samples, {(best[-1]['epoch'] - best[0]['epoch']) / 3600:.1f} h" if best else "no process seen")
    for col in ("rss_kb", "threads", "fds", "write_bytes", "tree_kb", "state_kb", "mem_available_kb"):
        vals = [r[col] for r in rows if r["pid"] or col in ("tree_kb", "state_kb", "mem_available_kb")]
        if not vals:
            continue
        per_day = trend([(r["epoch"] / 86400, r[col]) for r in best]) if best else 0.0
        print(f"{col:17} first {vals[0]:>12} last {vals[-1]:>12} min {min(vals):>12} max {max(vals):>12} trend/day {per_day:>+14.1f}")
    writes = rows[-1]["load_writes"]
    print(f"load: {writes} writes counted since the load last started")
    kills = os.path.join(folder, "kills.csv")
    if os.path.exists(kills):
        k = list(csv.DictReader(open(kills)))
        back = [int(r["seconds_until_back"]) for r in k]
        shapes = {(r["programs"], r["variables"]) for r in k}
        by = sorted({r["by"] for r in k})
        print(f"kills: {len(k)}; back after {min(back)}..{max(back)} s; never back: {sum(1 for b in back if b < 0)}; started again by: {by}; (programs, variables) seen: {sorted(shapes)}")
    log = os.path.join(folder, "load.log")
    if os.path.exists(log):
        lines = open(log).read().splitlines()
        print(f"load.log: {sum('refused' in line for line in lines)} refused writes, {sum('does not answer' in line for line in lines)} times the interface stopped answering")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "."))
