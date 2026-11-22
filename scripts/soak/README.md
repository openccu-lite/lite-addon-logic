# The soak: a week on real hardware

Before a release that is meant to stay as it is, the addon runs for seven days on a small system
beside everything else such a system runs, with as many programs and variables as the largest
installations have, under a steady stream of writes. Memory, threads, open files and the bytes
written to storage must stay flat; after a hundred `kill -9` and ten resets without a sync the
programs, the variables' values and the files' history must be whole.

Nothing here runs in CI. `internal/soak` (with its test) makes the tree; the rest are small
scripts for the system under test and one for the workstation.

| File | Where | What |
| --- | --- | --- |
| `go run ./cmd/soakgen <folder>` | workstation | writes `programs/`, `variables/` (300 and 800 by default) and `inputs.txt`. `-read Interface/ADDRESS:CH/PARAM=value` adds programs that trigger on a real device's reports, `-write …=value` programs that write a datapoint - take a virtual key's `PRESS_SHORT`, so that nothing goes out on the radio |
| `box/sample.sh` | system, as root | one line a minute into `samples.csv`: the daemon's RSS, threads, open files, bytes written to storage; the size of `tree/` and `state/`; the system's available memory; the load's count |
| `box/load.sh` | system | writes a random value to a random input variable through the interface `Logic` on the loopback (`setValue var-<key>:1 VALUE`), twice a second; `load.log` has every refused write and every time the interface stopped answering |
| `box/start.sh`, `box/stop.sh` | system, as root | start and end the two in the background. **Nothing starts by itself: after every start of the system run `start.sh` again** |
| `box/kill.sh [times] [gap]` | system, as root | `kill -9` the daemon a hundred times and wait for the system's supervisor to start it again (after 90 seconds without an answer the script starts the unit itself and says so); `kills.csv` has the seconds until the interface answered, who started it, and the numbers of programs and variables it listed |
| `summary.py <folder>` | workstation | first, last, smallest, largest and the trend per day of every column; the kills; the load's log |

## A run

1. Install the addon's package on the system; stop its unit (`systemctl stop addon-logic`).
2. `soakgen` a tree, copy `programs/` and `variables/` into the addon's `tree/` folder
   (`/usr/local/etc/config/addons/logic/tree`, owned by the addon's user), `inputs.txt` and
   `box/*.sh` into `/usr/local/soak/`. Start the unit: the addon reads the folder and records the
   files in its history.
3. `/usr/local/soak/start.sh`. Look at `samples.csv` after a few minutes.
4. During the week: `kill.sh` once; ten resets without a sync (`echo b > /proc/sysrq-trigger` as
   root, the stand-in for a pulled plug), each followed by `start.sh` once the system is up, a
   look at the journal of that boot (the file system check, the addon's start) and at the
   interface (`kills.csv`'s numbers must be what they were); restarts of the system's own
   services under load.
5. At the end: `stop.sh`, copy `/usr/local/soak/` and the addon's `tree/` and `state/` to the
   workstation; `summary.py`; `git fsck --full` in the copy of `tree/`; the state file must be
   whole. Remove `/usr/local/soak/` and, unless the addon stays, the addon.

The daemon's goroutines are not sampled - nothing outside the daemon can count them. A leak of
them shows as memory and, when they block in a system call, as threads.

The programs never run in a circle: nothing a program writes is something the load writes, and
the only programs triggered by another program's output write an output nobody triggers on.
